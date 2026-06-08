#!/usr/bin/env bash
set -euo pipefail

SERVICE_NAME="timelapse.service"

if [ "$EUID" -ne 0 ]; then
    echo "Please run as root (e.g., sudo ./restart.sh)"
    exit 1
fi

systemctl restart "${SERVICE_NAME}"
echo "Restarted ${SERVICE_NAME}"
systemctl status "${SERVICE_NAME}" --no-pager
