# AGENTS.md — rspi-cam-interface

## Project Overview

Go web server providing a REST API to capture photos via Raspberry Pi camera module (`rpicam-still`). Built with [Fiber v3](https://docs.gofiber.io/). Includes a CLI client for downloading timelapse packages and generating videos.

## Commands

| Command | Purpose |
|---------|---------|
| `go run ./cmd/server` | Run server locally (requires Go 1.26) |
| `go run ./cmd/client --server <url>` | Run client locally |
| `go run ./cmd/client --server <url> --keep` | Run client and keep working dir for resume |
| `go build -o ./bin/server ./cmd/server` | Build server binary |
| `go build -o ./bin/client ./cmd/client` | Build client binary |
| `go mod tidy` | Sync dependencies |

No tests, Makefile, or CI config exist yet.

> **Note**: This project is developed on Fedora but runs on Raspberry Pi OS, where the `rpicam-still` tool is available. Camera endpoints will not function during local Fedora development.

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

1. The background worker captures an image periodically using `rpicam-still` with `--output -` (stdout).
2. The image bytes are stored in memory.
3. Return the JPEG bytes with the correct `Content-Type: image/jpeg`.

Key difference between endpoints:
- **Preview**: `--timeout 2000 --width 640 --height 480`
- **Photo**: no extra flags (full sensor resolution)

### Client Flags

| Flag | Default | Description |
|------|---------|-------------|
| `-server` | `http://raspberrypi.local/` | Server base URL |
| `-output-dir` | `.` | Output directory for generated videos |
| `-work-dir` | `./timelapse_work` | Working directory for downloads and frames |
| `-fps` | `60` | Frames per second for output video |
| `-keep` | `false` | Keep working directory after encoding |
| `-state` | `timelapse_state.json` | Path to state file tracking processed packages |

### Client TUI

The client now uses a bubbletea v2 + bubbles v2 TUI (Terminal User Interface) with three screens:

1. **Group selection** (`bubbles/list`): A rich list with filtering, pagination, and help. Each group shows a checkmark (`[✓]`) when selected, package count, and how many are new. Groups with new packages are pre-selected. Navigate with `up/down` (or `k`/`j`), `space` to toggle selection, `/` to filter, `enter` to confirm.
2. **FPS selection** (`bubbles/textinput`): A text input field to enter any FPS value. Defaults to the `-fps` flag value (60 if not specified). `enter` to confirm, `esc` to go back.
3. **Processing** (`bubbles/spinner`): A spinner animates while the current group is being processed. Shows completed count, current group, and any errors. Press `q` or `ctrl+c` to quit at any time.

All `log` output during the TUI is redirected to `client.log` so it doesn't interfere with the terminal rendering.

### Timelapse Flow

1. Background goroutine captures photos every configured period using `rpicam-still --output -` (stdout to memory).
2. The in-memory image is written to `./timelapse` for packaging.
3. Background packager runs every minute, bundling photos older than 5s into `.tar.gz` archives in `./packages`.
4. Client queries `/api/v1/timelapse`, downloads archives, groups them by name prefix, unpacks them, and uses `ffmpeg` to generate a video per group.

#### Grouping

Packages are grouped based on the filename prefix before the last underscore:
- `timelapse_window_122.tar.gz` → `window.mp4`
- `timelapse_plants_02.tar.gz`, `timelapse_plants_03.tar.gz` → `plants.mp4`

#### Resume / State Tracking

The client maintains a state file (`timelapse_state.json` by default) that records which packages have already been downloaded and extracted. On subsequent runs, already-processed packages are skipped. Use `-keep` to preserve the working directory (frames) between runs so that resumed sessions include all historical frames in the final video.

## Code Style & Conventions

- Standard Go formatting (`gofmt`).
- Use Fiber's `fiber.Ctx` for handlers.
- `log.Fatal` for fatal server startup errors.

## Gotchas

- **Port 8080**: Server listens on `:8080`.
- **Hardware dependency**: `rpicam-still` must be installed and the user must have camera permissions. This only works on Raspberry Pi OS or compatible systems with a connected camera module.
- **No tests**: There are zero tests in the repo. Add them before making significant changes.
- **Cross-environment**: Developed on Fedora, deployed/run on Raspberry Pi OS. `rpicam-still` is not available on Fedora, so camera endpoints will fail there.
