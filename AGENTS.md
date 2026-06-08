# AGENTS.md — rspi-cam-interface

## Project Overview

Single-file Go web server providing a REST API to capture photos via Raspberry Pi camera module (`rpicam-jpeg`). Built with [Fiber v3](https://docs.gofiber.io/).

## Commands

| Command | Purpose                                                                                              |
|---------|------------------------------------------------------------------------------------------------------|
| `go run main.go` | Run locally (requires Go 1.26)                                                                       |
| `go build` | Build binary (works on Fedora but will be run on raspberry pi so it doesnt need to use rpicam tools) |
| `go mod tidy` | Sync dependencies                                                                                    |

No tests, Makefile, or CI config exist yet.

> **Note**: This project is developed on Fedora but runs on Raspberry Pi OS, where the `rpicam-jpeg` tool is available. Camera endpoints will not function during local Fedora development.

## Architecture & Control Flow

- **Entry point**: `main.go` only.
- **Server**: Fiber v3 instance listening on `:80`.
- **Endpoints**:
  - `GET /` — HTML links to preview and photo endpoints.
  - `GET /api/v1/preview` — low-res capture (640×480, 2s timeout).
  - `GET /api/v1/photo` — full-res capture.

### Camera Endpoint Pattern

Both `/preview` and `/photo` are intended to follow the same flow (documented in inline comments):

1. Generate a random ID for the output filename.
2. Shell out to `rpicam-jpeg` with `--output <filename>.jpg`.
3. Read the resulting JPEG into memory.
4. Delete the file immediately.
5. Return the JPEG bytes with the correct `Content-Type: image/jpeg`.

Key difference between endpoints:
- **Preview**: `--timeout 2000 --width 640 --height 480`
- **Photo**: no extra flags (full sensor resolution)

## Code Style & Conventions

- Standard Go formatting (`gofmt`).
- Use Fiber's `fiber.Ctx` for handlers.
- `log.Fatal` for fatal server startup errors.

## Gotchas

- **Port 80**: Running locally may require root/admin privileges. Use `:8080` for local dev if needed.
- **Hardware dependency**: `rpicam-jpeg` must be installed and the user must have camera permissions. This only works on Raspberry Pi OS or compatible systems with a connected camera module.
- **No tests**: There are zero tests in the repo. Add them before making significant changes.
- **Cross-environment**: Developed on Fedora, deployed/run on Raspberry Pi OS. `rpicam-jpeg` is not available on Fedora, so camera endpoints will fail there.
