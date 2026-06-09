#!/usr/bin/env bash
set -euo pipefail

INSTALL_DIR="/opt/timelapse"
SERVICE_NAME="timelapse.service"
SERVICE_FILE="/etc/systemd/system/${SERVICE_NAME}"

if [ "$EUID" -ne 0 ]; then
  echo "Please run as root (e.g., sudo ./install.sh)"
  exit 1
fi

# Use already built binary
if [ ! -f "./bin/server" ]; then
  echo "Error: ./bin/server not found. Please build the binary first."
  exit 1
fi

# Create install directory
mkdir -p "${INSTALL_DIR}"

# Stop the service if it's running so the binary can be replaced
if systemctl is-active --quiet "${SERVICE_NAME}"; then
  systemctl stop "${SERVICE_NAME}"
fi

# Copy binary
cp ./bin/server "${INSTALL_DIR}/server"

# Copy config with override prompt
if [ -f "${INSTALL_DIR}/config.yaml" ]; then
  read -p "config.yaml already exists in ${INSTALL_DIR}. Override? [y/N]: " answer
  if [[ "$answer" =~ ^[Yy]$ ]]; then
    cp ./config.yaml "${INSTALL_DIR}/config.yaml"
    echo "config.yaml updated."
  else
    echo "config.yaml kept."
  fi
else
  cp ./config.yaml "${INSTALL_DIR}/config.yaml"
  echo "config.yaml copied."
fi

# Create working subdirectories
mkdir -p "${INSTALL_DIR}/timelapse"
mkdir -p "${INSTALL_DIR}/packages"
mkdir -p "${INSTALL_DIR}/videos"

# Ensure pi user owns the directory so relative paths work and files are accessible
chown -R pi:pi "${INSTALL_DIR}"

# Create systemd service
cat >"${SERVICE_FILE}" <<EOF
[Unit]
Description=Timelapse Camera Server
After=network.target

[Service]
Type=simple
User=pi
Group=pi
WorkingDirectory=${INSTALL_DIR}
ExecStart=${INSTALL_DIR}/server --cfg ${INSTALL_DIR}/config.yaml
Restart=always
RestartSec=15
AmbientCapabilities=CAP_NET_BIND_SERVICE

[Install]
WantedBy=multi-user.target
EOF

# Reload systemd and enable service
systemctl daemon-reload
systemctl enable "${SERVICE_NAME}"
if systemctl is-active --quiet "${SERVICE_NAME}"; then
  systemctl restart "${SERVICE_NAME}"
else
  systemctl start "${SERVICE_NAME}"
fi

echo "Installed and started ${SERVICE_NAME}"
echo "Status:"
systemctl status "${SERVICE_NAME}" --no-pager
