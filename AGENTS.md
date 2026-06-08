# AGENTS.md — rspi-cam-interface

## Project Overview

Go web server providing a REST API to capture photos via Raspberry Pi camera module (`rpicam-jpeg`). Built with [Fiber v3](https://docs.gofiber.io/). Includes a CLI client for downloading timelapse packages and generating videos.

## Commands

| Command | Purpose |
|---------|---------|
| `go run ./cmd/server` | Run server locally (requires Go 1.26) |
| `go run ./cmd/client --server <url>` | Run client locally |
| `go build -o ./bin/server ./cmd/server` | Build server binary |
| `go build -o ./bin/client ./cmd/client` | Build client binary |
| `go mod tidy` | Sync dependencies |

No tests, Makefile, or CI config exist yet.

> **Note**: This project is developed on Fedora but runs on Raspberry Pi OS, where the `rpicam-jpeg` tool is available. Camera endpoints will not function during local Fedora development.

## Architecture & Control Flow

- **Server entry point**: `./cmd/server/main.go`
- **Client entry point**: `./cmd/client/main.go`
- **Server**: Fiber v3 instance listening on `:8080`.
- **Endpoints**:
  - `GET /` — HTML links to preview and photo endpoints.
  - `GET /api/v1/preview` — low-res capture (640×480, 2s timeout).
  - `GET /api/v1/photo` — full-res capture.
  - `GET /api/v1/timelapse` — JSON list of available timelapse package URLs.
  - `GET /static/*` — serves packaged timelapse `.tar.gz` files.

### Camera Endpoint Pattern

Both `/preview` and `/photo` follow the same flow:

1. Generate a random ID for the output filename.
2. Shell out to `rpicam-jpeg` with `--output <filename>.jpg`.
3. Read the resulting JPEG into memory.
4. Delete the file immediately.
5. Return the JPEG bytes with the correct `Content-Type: image/jpeg`.

Key difference between endpoints:
- **Preview**: `--timeout 2000 --width 640 --height 480`
- **Photo**: no extra flags (full sensor resolution)

### Timelapse Flow

1. Background goroutine captures photos every configured period to `./timelapse`.
2. Background packager runs every minute, bundling photos older than 5s into `.tar.gz` archives in `./packages`.
3. Client queries `/api/v1/timelapse`, downloads archives, unpacks them, and uses `ffmpeg` to generate a video.

## Code Style & Conventions

- Standard Go formatting (`gofmt`).
- Use Fiber's `fiber.Ctx` for handlers.
- `log.Fatal` for fatal server startup errors.

## Gotchas

- **Port 8080**: Server listens on `:8080`.
- **Hardware dependency**: `rpicam-jpeg` must be installed and the user must have camera permissions. This only works on Raspberry Pi OS or compatible systems with a connected camera module.
- **No tests**: There are zero tests in the repo. Add them before making significant changes.
- **Cross-environment**: Developed on Fedora, deployed/run on Raspberry Pi OS. `rpicam-jpeg` is not available on Fedora, so camera endpoints will fail there.
