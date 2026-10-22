#!/usr/bin/env bash
# Upgrade only an existing RegionHop panel; leave tunnel configs/core intact.
set -euo pipefail
umask 077

repo=valisheidaei/regionhop
panel=/opt/psi-panel/panel/psi-panel
if [[ ${EUID:-$(id -u)} != 0 ]]; then echo 'Run as root (sudo bash install-pin.sh).' >&2; exit 1; fi
if [[ $(uname -s) != Linux || ! -f "$panel" ]]; then echo 'An existing Linux RegionHop installation is required.' >&2; exit 1; fi
for command in curl sha256sum systemctl install mktemp flock; do
  command -v "$command" >/dev/null || { echo "Missing command: $command" >&2; exit 1; }
done
exec 9>/run/regionhop-pin-install.lock
flock -n 9 || { echo 'Another pin-panel install is running.' >&2; exit 1; }
case $(uname -m) in
  x86_64) arch=amd64 ;;
  aarch64|arm64) arch=arm64 ;;
  armv7l) arch=armv7 ;;
  *) echo 'Unsupported CPU architecture.' >&2; exit 1 ;;
esac

work=$(mktemp -d /tmp/regionhop-pin.XXXXXXXX)
staged=''
cleanup() {
  [[ -z "$staged" ]] || rm -f -- "$staged"
  # mktemp created this exact directory, never a caller-provided path.
  rm -rf -- "$work"
}
trap cleanup EXIT

tag=${REGIONHOP_PIN_TAG:-}
if [[ -z "$tag" ]]; then
  latest=$(curl -fLsS --proto '=https' --proto-redir '=https' -o /dev/null -w '%{url_effective}' "https://github.com/$repo/releases/latest")
  prefix="https://github.com/$repo/releases/tag/"
  [[ "$latest" == "$prefix"* ]] || { echo 'No pin release is available yet. Check the GitHub Actions build.' >&2; exit 1; }
  tag=${latest#"$prefix"}
fi
[[ "$tag" =~ ^pin-[a-f0-9]{12}$ ]] || { echo 'Refusing an unexpected release tag.' >&2; exit 1; }
asset="psi-panel-linux-$arch"
base="https://github.com/$repo/releases/download/$tag"
curl -fLsS --proto '=https' --proto-redir '=https' "$base/$asset" -o "$work/$asset"
curl -fLsS --proto '=https' --proto-redir '=https' "$base/SHA256SUMS.txt" -o "$work/SHA256SUMS.txt"
expected=$(awk -v name="$asset" '$2 == name { print $1 }' "$work/SHA256SUMS.txt")
[[ "$expected" =~ ^[a-f0-9]{64}$ ]] || { echo 'Missing or invalid release checksum.' >&2; exit 1; }
(cd "$work"; printf '%s  %s\n' "$expected" "$asset" | sha256sum -c -)

backup=$(mktemp -d /opt/psi-panel/panel-before-pin.XXXXXXXX)
chmod 700 "$backup"
cp -a -- "$panel" "$backup/psi-panel"
# Record the original service state for install/rollback.
was_active=false
if systemctl is-active --quiet psi-panel.service; then was_active=true; fi
staged=$(mktemp /opt/psi-panel/panel/.psi-panel-pin.XXXXXXXX)
install -o root -g root -m 0755 "$work/$asset" "$staged"

echo "Installing release $tag. Backup: $backup/psi-panel"
echo 'Only the management panel is restarted; the tunnel services keep running.'
systemctl stop psi-panel.service
if ! mv -f -- "$staged" "$panel"; then
  if "$was_active"; then systemctl start psi-panel.service; fi
  exit 1
fi
staged=''
if "$was_active"; then
  if ! systemctl start psi-panel.service; then
    systemctl stop psi-panel.service || true
    cp -a -- "$backup/psi-panel" "$panel"
    systemctl start psi-panel.service || true
    echo 'Panel failed to start; previous executable restored.' >&2
    exit 1
  fi
  sleep 2
  if ! systemctl is-active --quiet psi-panel.service; then
    systemctl stop psi-panel.service || true
    cp -a -- "$backup/psi-panel" "$panel"
    systemctl start psi-panel.service || true
    echo 'Panel exited after startup; previous executable restored.' >&2
    exit 1
  fi
fi
echo "Installed $tag. Open the panel and try Pin server on one test tunnel."
echo "To roll back: unpin tunnels first, then restore $backup/psi-panel while psi-panel is stopped."
