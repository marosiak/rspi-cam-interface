#!/usr/bin/env bash
set -euo pipefail

SERVICE_NAME="timelapse.service"

if [ "$EUID" -ne 0 ]; then
    echo "Please run as root (e.g., sudo ./stop.sh)"
    exit 1
fi

systemctl stop "${SERVICE_NAME}"
echo "Stopped ${SERVICE_NAME}"
