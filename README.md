# OpenPolyPrint

Multi-vendor 3D printer control built from the AnkerMake M5/M5C protocol stack. Designed as a desktop-first web app for monitoring, recording, and managing one or more 3D printers.

## Features

- **Dashboard** — live printer status, temperatures, progress, pause/stop controls, and camera cards with recording.
- **Printers** — printer list, detailed printer view, and provider configuration.
- **Cameras** — add and manage USB, MIPI (Pi), and stream cameras.
- **Recordings** — video and timelapse recordings with thumbnails, renaming, download, and grid layout.
- **Auto Record** — automatically start recording when a print begins, with video or timelapse mode.
- **Print History** — tracks finished prints and results.
- **G-code** — upload and manage G-code files.
- **Terminal** — full log viewer and a color-coded mini terminal in the sidebar.
- **Settings** — integrations, Pi GPIO, provider accounts, and app preferences.
- **Offline keys export** — export printer P2P/MQTT keys for manual offline setup.

## Tech Stack

- **Backend:** Go 1.26, standard `net/http` mux, with the Anker protocol stack in `internal/anker/proto`.
- **Frontend:** React 19, TypeScript, Vite, Tailwind CSS, `lucide-react`, `react-router-dom`.
- **Hardware (Pi):** `rpicam-apps` / `libcamera-apps`, `pigpiod`, GPIO via `sysfs` and udev.

## Quick Start

### Build

```bash
# Backend
go build -o openpolyprint ./cmd/openpolyprint

# Frontend
cd frontend
npm install
npm run build
```

### Run

```bash
./openpolyprint -addr :8080
```

The server serves the built frontend from `frontend/dist/`. Open http://localhost:8080.

### Development

```bash
# Terminal 1 — backend
go run ./cmd/openpolyprint

# Terminal 2 — frontend dev server with API proxy
cd frontend
npm run dev
```

### Docker

```bash
# x86 / regular build
docker compose up --build

# Raspberry Pi build with camera and GPIO support
docker compose -f docker-compose.pi.yaml up --build
```

The Pi compose uses `network_mode: host` (required for PPPP LAN discovery), maps `/dev`, `/run/udev`, and GPIO `sysfs` paths, and runs the container as `privileged`.

## Configuration

Settings are stored in the platform config directory (`~/.config/openpolyprint` on Linux, `%APPDATA%\openpolyprint` on Windows) in `settings.json`.

CLI flags:

- `-addr` — HTTP listen address (default `:8080`)
- `-data-dir` — optional Anker config data directory

Environment variables used by the Docker compose files:

- `OPENPOLYPRINT_HOST` — bind host, e.g. `0.0.0.0`
- `OPENPOLYPRINT_PORT` — bind port, e.g. `8080`
- `OPENPOLYPRINT_DATA_DIR` — runtime data directory, e.g. `/data`

## Lint / CI

```bash
# Go
gofmt -l cmd internal
go vet ./...
go build ./...

# Frontend
cd frontend
npm run lint   # tsc --noEmit
npm run build  # tsc && vite build
```

A GitHub Actions workflow (`.github/workflows/lint.yml`) runs the above on every push and PR.

## Supported Printers

- **Anker M5 / M5C** — full status, MQTT control (pause/resume/stop), PPPP LAN, and MQTT cloud support.
- **Flashforge / Klipper / Other** — provider settings UI exists but the backend drivers are not yet implemented.

## Recording

- **Video** — continuous MKV recording.
- **Timelapse** — frame-captured MKV at a selectable interval.
- **Auto Record** — starts a recording for every camera assigned to a printer when the printer begins a print, then stops and logs the result to History.

## Keyboard Shortcuts

| Shortcut | Page      |
| -------- | --------- |
| Ctrl + 1 | Dashboard |
| Ctrl + 2 | Printers  |
| Ctrl + 3 | G-code    |

## Notes

- `network_mode: host` is required for PPPP printer discovery on the local network.
- The Pi runtime installs `rpicam-apps` and `libcamera-apps` from the Raspberry Pi APT repository, not upstream Debian.
