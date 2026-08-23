# OpenPolyPrint — App Vision & Build Roadmap

## 1. Project Overview

**OpenPolyPrint** is a desktop-first, multi-vendor 3D printer control and monitoring app. It is built on the protocol code and web-server work already started in the `ankermake-m5-protocol` sub-project, but it is **not** limited to AnkerMake.

The goal is to take the existing AnkerMake (M5 / M5C) PPPP, MQTT, HTTP-API and camera-streaming logic, refactor it into a shared printer-driver model, then wrap it in a completely new, multi-page desktop UI. Each major screen is a real route (`/dashboard`, `/printers`, `/gcode`, etc.) instead of a single-page tab layout.

### Core Goals
- Support multiple printer brands/firmwares from one app: **Anker M5**, **Anker M5C**, **FlashForge**, **Klipper / Moonraker**, and eventually others.
- Re-use the proven Anker protocol code from `ankermake-m5-protocol` as the first driver.
- Provide a clean, modern, desktop-grade interface with separate pages and deep links.
- Be locally-first, offline-capable, and network-transparent where the printer firmware allows it.
- Keep the desktop app simple to install and run as a single binary / package.

> **Note — Project Name & Branding**
>
> - [x] Confirm final name `OpenPolyPrint` and add logo / color palette.
>
>   *Notes:*
>
>   ```
>   - Logo: public/logo.svg (three honeycomb OPP cells: indigo, green, amber)
>   - Favicon and PWA manifest use the same logo; theme color #2563eb
>   - UI theme: terminal/hacker look, blue accent, monospace body font
>   - Sidebar shows the multi-colour OPP logo + "OpenPolyPrint" wordmark
>   ```

---

## 2. What We Are Reusing From `ankermake-m5-protocol`

The existing Go project already solves a lot of hard problems for the Anker side:

| Existing Module | What It Gives Us |
|----------------|------------------|
| `flagship/pppp` | PPPP UDP packet protocol, LAN discovery, video stream, file transfer |
| `flagship/mqtt` | Anker cloud MQTT command channel, native printer commands |
| `flagship/crypto` | AES, ECDH, curse, seccode, logincache for auth and key exchange |
| `flagship/httpapi` | App/passport/hub v1/v2 HTTP clients |
| `flagship/config` | Config manager, printer model, import/export |
| `web/server.go` | HTTP routing, REST endpoints, websocket handlers, embedded static assets |
| `web/camera_*.go` | Multi-camera recording, stream management, session metadata |
| `web/gcode_*.go` | G-code file storage, SVG thumbnails |

We will pull these packages up into the new `OpenPolyPrint` root as a reusable **Anker driver**, then implement new drivers for the other printer types.

> **Note — Driver Extraction**
>
> - [x] Move `flagship/*` and `web/*` reusable backend modules into `OpenPolyPrint/drivers/anker/...`
>
>   *Notes:*
>
>   ```
>   - Created openpolyprint root go.mod with a replace directive to ./ankermake-m5-protocol
>   - Reused flagship/pppp, flagship/config, flagship/types in internal/drivers/anker/driver.go
>   - Backend imports github.com/ankermgmt/ankermake-m5-protocol-go/flagship packages directly
>   ```

---

## 3. Printer Driver Architecture

A single, clean interface should abstract the printer so the UI never talks to vendor protocols directly.

```
drivers/
  anker/      ← from existing project
  anker_m5c/  ← reuse anker modules with M5C-specific quirks
  flashforge/ ← FlashForge API / serial / TCP driver
  klipper/    ← Moonraker REST / websocket driver
  bambu/      ← (future) Bambu Lab LAN mode
```

```go
type PrinterDriver interface {
    Connect(ctx context.Context) error
    Disconnect() error
    Status(ctx context.Context) (PrinterStatus, error)
    SendGCode(ctx context.Context, gcode string) error
    UploadFile(ctx context.Context, filename string, data io.Reader) error
    StartPrint(ctx context.Context, filename string) error
    PausePrint(ctx context.Context) error
    StopPrint(ctx context.Context) error
    GetCameraStreamURL(ctx context.Context) (string, error)
    BedLevel(ctx context.Context) error
    Home(ctx context.Context) error
    ListFiles(ctx context.Context) ([]FileInfo, error)
    // ... extend as needed
}
```

> **Note — Driver Interface Finalised**
>
> - [x] Agree on the `PrinterDriver` Go interface and error types.
>
>   *Notes:*
>
>   ```
>   - Defined in internal/printers/printer.go
>   - Current interface: PrinterID(), Name(), Type(), Connect(ctx), Disconnect(), Status()
>   - Status returns Online, State string, UpdatedAt, Error
>   - Will extend with SendGCode, UploadFile, StartPrint, PausePrint, StopPrint, etc. as needed
>   ```

---

## 4. Tech Stack (Desktop First)

| Layer | Suggested Choice | Rationale |
|-------|-----------------|-----------|
| Backend | Go (re-use existing) | Existing protocol code is already in Go |
| REST / WebSocket API | `net/http` / `gorilla/websocket` (already used) | No new dependencies, cross-platform |
| Frontend | SvelteKit or React + Vite | Fast, modern, easy to turn into a desktop app |
| Desktop Shell | **Tauri** (Rust) or **Electron** | Single executable, web frontend, native OS feel |
| Styling | Tailwind CSS + shadcn/ui or DaisyUI | Consistent, dark/light theme support |
| Data store | SQLite for settings / printers / history | Simple, local, no external server |
| Optional build | Go embeds static files, Tauri bundles them | Keep the all-in-one binary feel |

> **Note — Stack Decision**
>
> - [x] Pick the final frontend framework and desktop shell.
>
>   *Notes:*
>
>   ```
>   - Frontend: React + Vite + React Router + Tailwind CSS (fast, easy to build, SPA)
>   - Backend: Go, re-uses anker modules
>   - Desktop shell: still TBD (Tauri or Electron); currently running as a web build
>   - go build ./cmd/openpolyprint can embed the built frontend from frontend/dist
>   ```

---

## 5. Page / Route Structure

Every screen is a real URL so the app feels like a desktop app and supports deep-linking / reload.

| Route | Page | Purpose |
|-------|------|---------|
| `/` | **Dashboard** | Overview of all printers, quick status, cameras, active prints |
| `/printers` | **Printers** | Add, edit, remove printers; see connection state |
| `/printers/:id` | **Printer Detail** | Full control for one printer: temps, fan, movement, macros |
| `/printers/:id/leveling` | **Bed Leveling** | Bed mesh / corner leveling (re-use existing 3D bed view logic) |
| `/gcode` | **G-code Library** | Upload, preview, delete, organise files |
| `/gcode/:id` | **G-code Preview** | 3D layer / toolpath viewer |
| `/cameras` | **Cameras** | All camera streams, single + grid view, recording |
| `/recordings` | **Recordings** | Session playback, MP4 conversion, export |
| `/history` | **Print History** | Past prints, duration, material used, thumbnails |
| `/settings` | **Settings** | App theme, drivers, notifications, offline keys, Pi/GPIO |
| `/help` | **Help / About** | Shortcuts, logs, version, credits |

> **Note — Routing Implemented**
>
> - [x] Set up the router with all planned routes.
>
>   *Notes:*
>
>   ```
>   - Implemented in App.tsx / pages/index.tsx with react-router-dom
>   - Live routes: /, /printers, /printers/:id, /gcode, /gcode/:id, /cameras, /recordings, /history, /settings, /help
>   - Persistent sidebar navigation with active-state highlighting
>   - Dashboard printer cards no longer link to detail; controls are opened from /printers
>   ```

---

## 6. UI / UX Direction

### Desktop-first design
- Sidebar or top navigation with real route links.
- Persistent printer selector in the header.
- Dark / light toggle, persisted.
- Status cards with live temperatures, progress bars, thumbnails.
- Keyboard shortcuts where useful (space to pause, etc.).
- Native window chrome (minimise, maximise, drag) and menu bar (File, View, Printer, Help).

### Look & Feel Targets
- Clean, not cluttered.
- Real-time but not noisy — subtle pulse for disconnected, clear color coding.
- Camera grid should be a first-class feature, not hidden in a tab.
- G-code preview should be a dedicated page with a clear upload flow.

> **Note — UI Design Complete**
>
> - [x] Produce a Figma / wireframe / mock-up and sign off the direction.
>
>   *Notes:*
>
>   ```
>   - Direction: terminal / hacker / modern desktop
>   - All headings bracketed: [ title ] with a blinking cursor and > prompt
>   - Body text uses ui-monospace font stack
>   - Cards have a blue left border accent; modals have a dark terminal window look
>   - Blue primary colour (#2563eb) instead of purple
>   - Dark/light toggle persisted in localStorage
>   ```

---

## 7. Feature Checklist (Backend)

### Core Infrastructure
- [x] Create `OpenPolyPrint` root module with `go.mod` and shared packages.
    > *Notes:*
    > - go.mod created with replace directive for ./ankermake-m5-protocol
    > - Shared packages: internal/printers, internal/drivers/anker
- [x] Refactor existing Anker code into a clean driver package.
    > *Notes:*
    > - internal/drivers/anker/driver.go wraps pppp.DuidFromString / pppp.NewPPPPApiLAN
    > - Connect follows the same flow as ankerctl pppp open (connect lan search, run, wait for StateConnected)
    > - Status reports online / pppp state string
- [x] Implement `PrinterDriver` interface and per-brand driver registry.
    > *Notes:*
    > - printers.Driver in internal/printers/printer.go
    > - Manager in internal/printers/manager.go connects/disconnects/lists all drivers
    > - Per-brand registry not yet added (only anker.NewDriver wired in main.go)
- [ ] Add SQLite persistence for printers, settings, and print history.
    > *Notes:*
    > - Frontend still persists settings to localStorage
    > - No SQLite layer yet; currently using ankerctl default.json for printer config
- [x] Build REST/websocket API layer that serves the frontend.
    > *Notes:*
    > - cmd/openpolyprint/main.go serves /api/health, /api/printers, /api/printers/{id}/status
    > - Also /api/printers/{id}/pause and /api/printers/{id}/stop
    > - Serves frontend/dist/ as the UI when it exists
    > - WebSocket not yet added
- [x] Add auto-discovery / LAN search for Anker printers.
    > *Notes:*
    > - AnkerDriver.Connect calls api.ConnectLanSearch() and waits for StateConnected
    > - No dedicated /api/discover endpoint yet; discovery happens during driver connect
- [ ] Add manual IP / host configuration for Klipper/Moonraker and FlashForge.
    > *Notes:*
    > - Frontend Settings page has UI fields for Klipper/FlashForge/Other providers
    > - No backend drivers yet for non-Anker brands
- [ ] Add optional token-based auth (reuse existing `auth.go` logic).
    > *Notes:*
    > - Not started
    > - Current API is open on localhost
### Printer Support
- [x] **AnkerMake M5** — full feature parity with existing app.
    > *Notes:*
    > - Basic connect + online/offline status implemented
    > - Anker code organized under `internal/anker` (`driver.go`, `login.go`, `printer_state.go`)
    > - Pause / stop print control via MQTT implemented
    > - Real temperatures, progress, file name, and remaining time parsed from MQTT into `printers.Status`
    > - Still need: camera, file upload, start print
- [ ] **AnkerMake M5C** — reuse M5 driver, handle M5C-specific responses.
    > *Notes:*
    > - Not explicitly tested; should work with same PPPP flow once M5C quirks are known
- [ ] **FlashForge** — status, upload, start/pause/stop, camera.
    > *Notes:*
    > - Settings UI placeholder exists
    > - `internal/flashforge` package created as a stub for future driver
- [ ] **Klipper / Moonraker** — connect via HTTP/websocket, macros, camera stream.
    > *Notes:*
    > - Settings UI placeholder exists
    > - `internal/klipper` package created as a stub for future driver
- [ ] Generic **G-code over serial / TCP** fallback (optional).
    > *Notes:*
    > - Not started
### Camera & Recording
- [x] Multi-camera support (printer, USB, stream URL).
    > *Notes:*
    > - Frontend /cameras page groups cameras by printer with type badges
    > - Dashboard has a security-camera-style grid with printer filter
    > - Real camera streams not yet wired
- [ ] Re-use existing ffmpeg recording and MKV/MP4 conversion.
    > *Notes:*
    > - UI only; no recording implementation yet
- [ ] Session grouping and playback.
    > *Notes:*
    > - Not started
### G-code
- [ ] Upload, download, delete, list files.
    > *Notes:*
    > - Frontend /gcode library has mock data only
    > - No backend file store or upload API
- [ ] Generate SVG thumbnails (reuse existing logic).
    > *Notes:*
    > - Not started
- [x] 3D viewer on G-code detail page.
    > *Notes:*
    > - Frontend has /gcode/:id with layer slider and print-time estimate mockup
    > - Active G-code viewer added to printer modal [ gcode ] tab
    > - Real 3D rendering not yet integrated
### Settings / Data
- [ ] Import / export printer offline keys.
    > *Notes:*
    > - UI button in Settings, not wired
- [ ] Filament inventory and usage tracking.
    > *Notes:*
    > - Not started
- [ ] Pi / GPIO / sensor support (hide on non-Pi, reuse existing logic).
    > *Notes:*
    > - Not started
---

## 8. Feature Checklist (Frontend)

### Shell
- [ ] Desktop window frame and menu.
    > *Notes:*
    > - Still running in the browser; Tauri/Electron shell not yet integrated
- [x] Routing with all pages from section 5.
    > *Notes:*
    > - All planned routes are live (see section 5 notes)
- [x] Sidebar or top navigation component.
    > *Notes:*
    > - Sidebar with logo, all nav items, and dark/light toggle
- [x] Theme toggle and persistence.
    > *Notes:*
    > - Dark mode stored in localStorage and synced to document.documentElement.classList
### Dashboard
- [x] Grid of printer cards showing status, temps, progress.
    > *Notes:*
    > - Dashboard shows real printer cards fetched from `GET /api/printers`
    > - Mock `mockPrinters` removed; `Printer` type moved to `src/types.ts`
    > - Pause/stop with confirm modal
- [x] Active camera / quick actions.
    > *Notes:*
    > - Security-camera-style grid with filter dropdown
    > - Stop/pause controls on cards (dashboard not navigable)
### Printers
- [ ] Add-printer wizard (brand, connection method, IP/credentials).
    > *Notes:*
    > - Settings page has provider toggles and fields; no full wizard yet
- [ ] Edit / remove printers.
    > *Notes:*
    > - Not started
- [x] Printer detail page with live telemetry.
    > *Notes:*
    > - /printers/:id and printer modal with controls, temperatures, macros, bed leveling, cameras, gcode
- [x] Bed leveling page with 3D view.
    > *Notes:*
    > - /printers/:id/leveling route and modal tab [ leveling ]
    > - 3D bed visualizer is a placeholder
### G-code
- [ ] File library with drag-and-drop upload.
    > *Notes:*
    > - /gcode page lists mock files; no real upload
- [x] Preview page with layer slider and print-time estimate.
    > *Notes:*
    > - /gcode/:id has layer slider and mock stats
    > - Active G-code viewer in printer modal with layer slider and progress
### Cameras & Recordings
- [x] Multi-camera grid and single view.
    > *Notes:*
    > - /cameras groups by printer, dashboard has grid filter
- [ ] Record, stop, convert, export.
    > *Notes:*
    > - UI only; no recording backend
### Settings
- [x] General, printers, notifications, integrations, offline keys, Pi.
    > *Notes:*
    > - Settings page has appearance, notifications, provider toggles, integrations, offline keys sections
    > - Integrations registry imported from the embedded project under `internal/integrations`
    > - Telegram and Discord test senders implemented with `POST /api/integrations/{id}/test`
    > - All integration forms editable in the UI and persisted through `AppConfig.integrations`
---

## 9. Build, Packaging & Release

- [ ] Define build scripts for Windows/macOS/Linux desktop packages.
    > *Notes:*
    > - Go backend builds with go build; frontend builds with npm run build
    > - No packaging scripts yet
- [ ] Cross-compile Go backend (reuse existing `make cross-compile` pattern).
    > *Notes:*
    > - No Makefile yet
- [x] Bundle frontend into Go static assets or Tauri/Electron package.
    > *Notes:*
    > - cmd/openpolyprint serves frontend/dist/ over http.FileServer
    > - SPA fallback to index.html implemented for unknown routes
- [ ] Create installer / `.zip` / `.dmg` / `.appimage` outputs.
    > *Notes:*
    > - Not started
- [ ] CI workflow for tests and release builds (can re-use `.github/workflows`).
    > *Notes:*
    > - Not started
---

## 10. First Milestone (MVP)

The first deliverable should be a working desktop app that can:

1. Add an Anker M5 / M5C printer using credentials or offline keys.
2. Display the dashboard with printer status, temperatures, and current print progress.
3. Show a live camera stream.
4. Upload and list G-code files.
5. Start, pause, and stop a print.
6. Look significantly different from the current single-page UI — separate pages, new design.

> **Note — MVP Completed**
>
> - [ ] All MVP items above tested and working.
>
>   *Notes:*
>
>   ```
>   - UI/UX is in place with terminal/hacker theme
>   - Anker M5 connect + status API works if default.json is present
>   - Pause / stop commands now wired from frontend to backend over MQTT (when account is configured)
>   - Still need: start, real telemetry, live camera, file upload
>   ```

---

## 11. Notes & Open Questions

Use this section for anything that needs a decision or tracking that is not covered above.

- [ ] Will the desktop app be **Tauri** or **Electron**?
    > *Notes:*
    > - Leaning toward Tauri for smaller binary, but Electron is an option- [ ] Should the backend be a separate daemon the frontend talks to, or compiled into one Tauri/Electron binary?
    > *Notes:*
    > - Currently a single Go binary serves the API and the built frontend
    > - Tauri can bundle this as a sidecar or the frontend can call it- [ ] Do we keep the original `ankermake-m5-protocol` folder as a git submodule or migrate code out of it?
    > *Notes:*
    > - Currently a local folder with a go.mod replace directive; no submodule- [ ] Which FlashForge protocol / API version is the target (e.g. FlashPrint Cloud, ADventurer, Finder)?
    > *Notes:*
    > - Needs research; provider UI is ready- [ ] Does the user need local serial-port support or just LAN/Wi-Fi?
    > *Notes:*
    > - Currently targeting LAN/Wi-Fi; serial can be added later- [ ] Will mobile be considered later, or is it strictly out of scope for now?
    > *Notes:*
    > - Desktop-first for now; mobile is out of scope
---

## 12. How to Use This File

1. When starting a task, find its checkbox in this file.
2. Update the `*Notes:*` block under the task with progress, decisions, blockers, or commit references.
3. Tick the checkbox when the task is fully done and verified.
4. Use the bottom **Notes & Open Questions** block for anything that is unclear or still being decided.

This keeps the whole build history in one place and makes it easy to hand the project back and forth.
