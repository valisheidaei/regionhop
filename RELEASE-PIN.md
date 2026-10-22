Custom RegionHop 1.24.0 panel with per-tunnel server pinning.

- Pin the active Psiphon server; unpin to restore normal selection.
- No automatic fallback to a different server while pinned.
- This does not reserve or guarantee a static public exit IP.
- Preserves the existing Psiphon core and local SOCKS ports.
- Bolt datastore support only; missing notices/entries fail explicitly.
- Unit tests run on Linux before binaries are published. A live Psiphon connection on your server still needs testing.

For an existing installation, download and review `install-pin.sh` from this repository, then run it as root. It validates the release checksum and backs up the existing panel executable. See `PINNING.fa.md` for Persian instructions and limitations.

The original `install.sh` is the upstream full installer. Use `install-pin.sh` to test this custom panel.
