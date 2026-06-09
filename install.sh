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

# Copy binary
if [ -f "./bin/server" ]; then
    cp ./bin/server "${INSTALL_DIR}/server"
else
    echo "Error: ./bin/server not found. Build failed?"
    exit 1
fi

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
cat > "${SERVICE_FILE}" <<EOF
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
RestartSec=5

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
