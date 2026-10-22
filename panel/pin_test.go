package main

import (
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	bolt "go.etcd.io/bbolt"
)

func TestPinUsesActiveTunnelAndPreservesSignedFields(t *testing.T) {
	out := `{"noticeType":"ActiveTunnel","timestamp":"2026-01-02","data":{"diagnosticID":"abcdefgh"}}` + "\n" +
		`{"noticeType":"ConnectedServer","timestamp":"2026-01-03","data":{"diagnosticID":"wrongone"}}`
	id := activeDiagnosticID(out)
	if id != "abcdefgh" {
		t.Fatal(id)
	}
	raw := []byte(`{"ipAddress":"203.0.113.1","tag":"abcdefghMORE","signature":"preserve","futureField":true}`)
	entry, ip, err := encodedEntry(raw, id)
	if err != nil || ip != "203.0.113.1" {
		t.Fatalf("%s %v", ip, err)
	}
	decoded, _ := hex.DecodeString(entry)
	if string(decoded) != "0 0 0 0 "+string(raw) {
		t.Fatal(string(decoded))
	}
	if e, _, _ := encodedEntry(raw, "different"); e != "" {
		t.Fatal("matched a different server")
	}
}

func TestPinDatastoreMissingAndAmbiguous(t *testing.T) {
	root := t.TempDir()
	if _, _, err := readPinnedEntry(root, "abcdefgh"); err == nil {
		t.Fatal("missing datastore accepted")
	}
	dir := filepath.Join(root, "ca.psiphon.PsiphonTunnel.tunnel-core", "datastore")
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	db, err := bolt.Open(filepath.Join(dir, "psiphon.boltdb"), 0600, nil)
	if err != nil {
		t.Fatal(err)
	}
	err = db.Update(func(tx *bolt.Tx) error {
		b, err := tx.CreateBucket([]byte("serverEntries"))
		if err != nil {
			return err
		}
		return b.Put([]byte("a"), []byte(`{"ipAddress":"203.0.113.1","tag":"abcdefgh1"}`))
	})
	if err != nil {
		t.Fatal(err)
	}
	db.Close()
	entry, ip, err := readPinnedEntry(root, "abcdefgh")
	if err != nil || entry == "" || ip != "203.0.113.1" {
		t.Fatalf("%s %v", ip, err)
	}
	db, err = bolt.Open(filepath.Join(dir, "psiphon.boltdb"), 0600, nil)
	if err != nil {
		t.Fatal(err)
	}
	err = db.Update(func(tx *bolt.Tx) error {
		return tx.Bucket([]byte("serverEntries")).Put([]byte("b"), []byte(`{"ipAddress":"203.0.113.2","tag":"abcdefgh2"}`))
	})
	if err != nil {
		t.Fatal(err)
	}
	db.Close()
	if _, _, err = readPinnedEntry(root, "abcdefgh"); err == nil || !strings.Contains(err.Error(), "ambiguous") {
		t.Fatal(err)
	}
}

func TestPinnedConfigSurvivesRegenerationAndRegistry(t *testing.T) {
	dir := t.TempDir()
	tunnel := Tunnel{Name: "test-pin", Region: "US", SocksPort: 19000, TargetServerEntry: "encoded-entry", PinnedServerIP: "203.0.113.1"}
	path := filepath.Join(dir, "registry.json")
	if err := saveRegistry(path, []Tunnel{tunnel}); err != nil {
		t.Fatal(err)
	}
	list, err := loadRegistry(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := writeTunnelConfig(dir, dir, list[0], psiphonCreds{ConfigJSON: `{"TargetServerEntry":"wrong","TunnelPoolSize":8,"ListenInterface":"any"}`}); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(filepath.Join(dir, "test-pin.json"))
	if err != nil {
		t.Fatal(err)
	}
	var cfg map[string]any
	if err := json.Unmarshal(b, &cfg); err != nil {
		t.Fatal(err)
	}
	if cfg["TargetServerEntry"] != "encoded-entry" || cfg["TunnelPoolSize"] != float64(1) || cfg["ListenInterface"] != "lo" {
		t.Fatal(cfg)
	}
}
