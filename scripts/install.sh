#!/usr/bin/env bash
set -euo pipefail

VERSION="${GRUPO_VERSION:-latest}"
INSTALL_DIR="${INSTALL_DIR:-/usr/local/bin}"
DATA_DIR="${DATA_DIR:-/var/lib/grupo}"
CONFIG_DIR="${CONFIG_DIR:-/etc/grupo}"

echo "Installing Grupo ${VERSION}..."

if ! command -v docker >/dev/null 2>&1; then
  echo "Docker is required. Install Docker Engine first." >&2
  exit 1
fi

id -u grupo >/dev/null 2>&1 || useradd --system --home "$DATA_DIR" --shell /usr/sbin/nologin grupo
mkdir -p "$DATA_DIR" "$CONFIG_DIR"
chown -R grupo:grupo "$DATA_DIR"

if [ -f bin/grupo ]; then
  install -m 0755 bin/grupo "$INSTALL_DIR/grupo"
else
  echo "Place the grupo binary at bin/grupo before running install.sh" >&2
  exit 1
fi

install -m 0644 deploy/grupo.service /etc/systemd/system/grupo.service
if [ ! -f "$CONFIG_DIR/config.yaml" ]; then
  install -m 0644 deploy/config.example.yaml "$CONFIG_DIR/config.yaml"
  echo "Created $CONFIG_DIR/config.yaml — edit before starting."
fi

usermod -aG docker grupo 2>/dev/null || true
systemctl daemon-reload
systemctl enable grupo.service
echo "Installed. Configure $CONFIG_DIR/config.yaml then: systemctl start grupo"
