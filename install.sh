#!/usr/bin/env bash
set -euo pipefail

INSTALL_DIR="/opt/timelapse"
SERVICE_NAME="timelapse.service"
SERVICE_FILE="/etc/systemd/system/${SERVICE_NAME}"

if [ "$EUID" -ne 0 ]; then
    echo "Please run as root (e.g., sudo ./install.sh)"
    exit 1
fi

# Build binary for Raspberry Pi
if [ -f "build-rspi.sh" ]; then
    ./build-rspi.sh
else
    GOOS=linux GOARCH=arm GOARM=6 go build -o ./bin/server ./cmd/server
fi

# Create install directory
mkdir -p "${INSTALL_DIR}"

# Copy binary and config
cp ./bin/server "${INSTALL_DIR}/server"
cp ./config.yaml "${INSTALL_DIR}/config.yaml"

# Create working subdirectories
mkdir -p "${INSTALL_DIR}/timelapse"
mkdir -p "${INSTALL_DIR}/packages"
mkdir -p "${INSTALL_DIR}/videos"

# Ensure pi user owns the directory so relative paths work and files are accessible
chown -R pi:pi "${INSTALL_DIR}"

# Create systemd service
cat > "${SERVICE_FILE}" <<EOF
[Unit]
Description=Timelapse Camera Server
After=network.target

[Service]
Type=simple
User=pi
Group=pi
WorkingDirectory=${INSTALL_DIR}
ExecStart=${INSTALL_DIR}/server
Restart=always
RestartSec=5

[Install]
WantedBy=multi-user.target
EOF

# Reload systemd and enable service
systemctl daemon-reload
systemctl enable "${SERVICE_NAME}"
systemctl start "${SERVICE_NAME}"

echo "Installed and started ${SERVICE_NAME}"
echo "Status:"
systemctl status "${SERVICE_NAME}" --no-pager
