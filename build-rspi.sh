#!/usr/bin/env bash
set -euo pipefail

mkdir -p ./bin

GOOS=linux GOARCH=arm GOARM=6 go build -o ./bin/server ./cmd/server

echo "Built for Raspberry Pi Zero W: ./bin/server"
