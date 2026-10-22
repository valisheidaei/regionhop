package main

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	bolt "go.etcd.io/bbolt"
)

// Use the last ActiveTunnel from this invocation, not an exit-IP lookup:
// a server's ingress address need not equal the public egress address.
func activeDiagnosticID(out string) string {
	var latest, id string
	for _, line := range strings.Split(out, "\n") {
		i := strings.IndexByte(line, '{')
		if i < 0 {
			continue
		}
		var n struct {
			Type      string `json:"noticeType"`
			Timestamp string `json:"timestamp"`
			Data      struct {
				ID string `json:"diagnosticID"`
			} `json:"data"`
		}
		if json.Unmarshal([]byte(line[i:]), &n) == nil && n.Type == "ActiveTunnel" && n.Timestamp >= latest {
			latest, id = n.Timestamp, n.Data.ID
		}
	}
	return id
}

func encodedEntry(raw []byte, diagnosticID string) (encoded, ip string, err error) {
	var fields map[string]any
	if err = json.Unmarshal(raw, &fields); err != nil {
		return
	}
	get := func(key string) string { s, _ := fields[key].(string); return s }
	ip = get("ipAddress")
	if net.ParseIP(ip) == nil {
		return "", "", errors.New("invalid server address")
	}
	tag := get("tag")
	if tag == "" {
		mac := hmac.New(sha256.New, []byte(get("webServerSecret")))
		mac.Write([]byte(ip))
		tag = base64.StdEncoding.EncodeToString(mac.Sum(nil))
	}
	if len(diagnosticID) != 8 || len(tag) < 8 || tag[:8] != diagnosticID {
		return "", "", nil
	}
	// Preserve all fields, including signatures and fields unknown to this panel.
	cert := get("webServerCertificate")
	prefix := "0 0 0 0 "
	if cert != "" {
		prefix = fmt.Sprintf("%s %s %s %s ", ip, get("webServerPort"), get("webServerSecret"), cert)
	}
	return hex.EncodeToString(append([]byte(prefix), raw...)), ip, nil
}

func readPinnedEntry(root, diagnosticID string) (entry, ip string, err error) {
	// The service must be stopped before calling this. Never bypass its lock
	// or reset/modify its database. Unsupported formats fail explicitly.
	path := filepath.Join(root, "ca.psiphon.PsiphonTunnel.tunnel-core", "datastore", "psiphon.boltdb")
	if _, err = os.Stat(path); err != nil {
		return "", "", fmt.Errorf("supported Bolt datastore unavailable: %w", err)
	}
	db, err := bolt.Open(path, 0600, &bolt.Options{ReadOnly: true, Timeout: time.Second})
	if err != nil {
		return "", "", err
	}
	defer db.Close()
	err = db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte("serverEntries"))
		if b == nil {
			return errors.New("serverEntries bucket missing")
		}
		return b.ForEach(func(_, v []byte) error {
			if v == nil {
				return nil
			}
			e, addr, eerr := encodedEntry(v, diagnosticID)
			if eerr != nil {
				return eerr
			}
			if e == "" {
				return nil
			}
			if entry != "" {
				return errors.New("ambiguous diagnostic ID; refusing to pin")
			}
			entry, ip = e, addr
			return nil
		})
	})
	if err == nil && entry == "" {
		err = errors.New("active server not found; reconnect and try again")
	}
	return
}

func (a *app) handlePin(w http.ResponseWriter, r *http.Request, name string, pin bool) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", 405)
		return
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	list, err := loadRegistry(registryPath)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	i := -1
	for j := range list {
		if list[j].Name == name {
			i = j
			break
		}
	}
	if i < 0 {
		http.NotFound(w, r)
		return
	}
	old := list[i]
	if pin && old.TargetServerEntry != "" {
		http.Redirect(w, r, urlFor("/"), 303)
		return
	}
	// Pinning needs a fresh invocation ID; the dashboard's three-second
	// status cache could otherwise select the run before a recent restart.
	stateOut, stateErr := runSystemctl("show", "-p", "Id", "-p", "ActiveState", "-p", "InvocationID", unitName(name))
	if stateErr != nil {
		http.Error(w, "cannot read service state", 500)
		return
	}
	st := parseUnitStates(stateOut)[name]
	if pin && st.Active != "active" {
		http.Error(w, "connect the tunnel before pinning", 409)
		return
	}
	// Preserve the exact existing configuration, including settings added by
	// the installer. Unpin changes only the target and pool size we saved.
	path := filepath.Join(configsDir, name+".json")
	before, err := os.ReadFile(path)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	var cfg map[string]any
	if json.Unmarshal(before, &cfg) != nil || cfg == nil {
		http.Error(w, "invalid tunnel config", 500)
		return
	}
	if pin && cfg["TargetServerEntry"] != nil && cfg["TargetServerEntry"] != "" {
		http.Error(w, "a TargetServerEntry already exists in the config", 409)
		return
	}
	if !pin && old.TargetServerEntry == "" {
		http.Error(w, "tunnel is not pinned", 409)
		return
	}
	// Brief outage is intentional: read a consistent, closed datastore.
	wasRunning := st.Active == "active"
	completed := false
	defer func() {
		if wasRunning {
			if e := startTunnel(name); e != nil {
				if completed {
					http.Error(w, "configuration saved, but restart failed: "+e.Error(), 500)
				} else {
					fmt.Printf("resume %s failed: %v\n", name, e)
				}
				return
			}
		}
		if completed {
			http.Redirect(w, r, urlFor("/"), http.StatusSeeOther)
		}
	}()
	if wasRunning {
		if err = stopTunnel(name); err != nil {
			wasRunning = false
			http.Error(w, err.Error(), 500)
			return
		}
	}
	if pin {
		if st.InvocationID == "" {
			http.Error(w, "missing service invocation ID", 409)
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
		defer cancel()
		out, e := exec.CommandContext(ctx, "journalctl", "_SYSTEMD_INVOCATION_ID="+st.InvocationID, "-n", "10000", "-o", "cat", "--no-pager").Output()
		if e != nil {
			http.Error(w, "cannot read current tunnel notices", 500)
			return
		}
		id := activeDiagnosticID(string(out))
		if id == "" {
			http.Error(w, "ActiveTunnel notice unavailable; reconnect with diagnostic notices enabled", 409)
			return
		}
		entry, ip, e := readPinnedEntry(filepath.Join(dataDir, name), id)
		if e != nil {
			http.Error(w, e.Error(), 409)
			return
		}
		list[i].TargetServerEntry, list[i].PinnedServerIP = entry, ip
		list[i].PrePinPoolSize = cfg["TunnelPoolSize"]
		cfg["TargetServerEntry"], cfg["TunnelPoolSize"] = entry, 1
	} else {
		delete(cfg, "TargetServerEntry")
		delete(cfg, "TunnelPoolSize")
		if old.PrePinPoolSize != nil {
			cfg["TunnelPoolSize"] = old.PrePinPoolSize
		}
		list[i].TargetServerEntry, list[i].PinnedServerIP, list[i].PrePinPoolSize = "", "", nil
	}
	body, err := json.MarshalIndent(cfg, "", "  ")
	if err == nil {
		err = atomicPrivateWrite(path, body)
	}
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	if err = saveRegistry(registryPath, list); err != nil {
		rollback := atomicPrivateWrite(path, before)
		http.Error(w, fmt.Sprintf("registry write failed: %v; config rollback: %v", err, rollback), 500)
		return
	}
	invalidateRows()
	completed = true
}
