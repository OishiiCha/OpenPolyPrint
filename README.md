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
./openpolyprint
```

The server serves the built frontend from `frontend/dist/`. It listens on both:
- **HTTP** on port `80` → http://localhost
- **HTTPS** on port `443` → https://localhost

HTTPS uses a local CA to sign the server certificate (like mkcert).
To remove browser security warnings, install the CA certificate:

1. Download the CA cert from `http://localhost/api/tls/ca` (or the Settings page)
2. Install it in your system/browser trust store:
   - **Windows:** Double-click `openpolyprint-ca.pem` → "Install Certificate" → "Local Machine" → "Place all certificates in: Trusted Root Certification Authorities"
   - **macOS:** Double-click → Keychain Access → Find "OpenPolyPrint Local CA" → Right-click → "Get Info" → Trust → "Always Trust"
   - **Linux:** `sudo cp openpolyprint-ca.pem /usr/local/share/ca-certificates/ && sudo update-ca-certificates`

The CA is stored in `<settings-dir>/tls/ca.pem` (valid 10 years).
The server certificate is stored in `cert.pem` and `key.pem` (valid 1 year).
Both are auto-regenerated on startup if expired or if the hostname/IP addresses have changed.

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

- `-addr` — HTTPS listen address (default `:443`; HTTP always on `:80` when TLS enabled)
- `-tls` — enable HTTPS with auto-generated self-signed certificate (default `true`)
- `-data-dir` — optional Anker config data directory

Environment variables used by the Docker compose files:

- `OPENPOLYPRINT_HOST` — bind host, e.g. `0.0.0.0`
- `OPENPOLYPRINT_PORT` — bind port, e.g. `443`
- `OPENPOLYPRINT_DATA_DIR` — runtime data directory, e.g. `/data`

### Secrets / API keys (.env)

Copy `.env.example` to `.env` and fill in your values. The `.env` file is
loaded automatically at startup (both for local dev and Docker). Docker
Compose uses `env_file: .env` to inject the variables.

Supported env vars (all optional — see `.env.example` for the full list):

- `GEMINI_API_KEY` — Google Gemini API key for AI print analysis
- `GEMINI_ENABLED` — enable AI analysis on startup (`true`/`false`)
- `ANKER_EMAIL` / `ANKER_PASSWORD` / `ANKER_REGION` — auto-login to Anker cloud
- `VAPID_PUBLIC_KEY` / `VAPID_PRIVATE_KEY` — push notification keys (auto-generated if blank)
- `TELEGRAM_BOT_TOKEN` / `TELEGRAM_CHAT_ID` — Telegram notifications
- `DISCORD_WEBHOOK_URL` — Discord notifications
- `N8N_WEBHOOK_URL` — n8n/Zapier webhook
- `OBICO_TOKEN` — Obico integration

Env vars take precedence over values saved in settings.json via the UI.

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
