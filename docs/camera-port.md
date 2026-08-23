# Anker camera module port to OpenPolyPrint

## Goal

Port the local-camera subsystem from `ankermake-m5-protocol/web/camera_*.go` so that OpenPolyPrint can:

1. Discover and attach USB cameras to the server.
2. Discover and attach a Raspberry Pi MIPI / CSI camera.
3. Expose those cameras in the existing `Cameras` page.
4. Provide a live MJPEG preview for USB and MIPI cameras.
5. (Optional later) Record camera streams to files.

## Source reference files

- `ankermake-m5-protocol/web/camera_store.go` — `CameraStore`, `CameraConfig`, `CameraSettings`, `Recording*`.
- `ankermake-m5-protocol/web/usb_streamer.go` — `UsbCameraStreamer`, `runFfmpeg`, MJPEG broadcast.
- `ankermake-m5-protocol/web/camera_handlers.go` — HTTP handlers for `/api/cameras/*`.
- `ankermake-m5-protocol/web/static/ankersrv.js` — frontend camera UI, WebSocket preview, USB enumeration.

## Phases

### Phase 1 — USB cameras (first)

- Create `internal/cameras` package.
  - `config.go`: `CameraConfig`, `CameraSettings`, `RecordingSettings`.
  - `store.go`: JSON-backed `CameraStore` persisted to `data/cameras.json`.
  - `usb.go`: `UsbCameraStreamer` that spawns `ffmpeg` and broadcasts JPEG frames to subscribers.
- Add backend HTTP handlers to `cmd/openpolyprint/main.go`:
  - `GET /api/cameras/settings`
  - `POST /api/cameras/settings`
  - `GET /api/cameras`
  - `POST /api/cameras`
  - `PUT /api/cameras`
  - `DELETE /api/cameras`
  - `GET /api/cameras/status`
  - `GET /api/cameras/usb/list`
  - `GET /api/cameras/usb/preview` (MJPEG stream or WebSocket)
- Update frontend `Cameras` page and `AddCameraModal`:
  - Allow adding a `usb` or `stream` camera.
  - Show a list of available USB devices and preview.
  - Persist new camera in `localStorage` via `useCameras` and sync to backend.

### Phase 2 — MIPI / Raspberry Pi camera

- Extend `CameraConfig.Type` with `rpicam`.
- Add a Raspberry Pi MIPI streamer that uses `rpicam-vid` / `libcamera-vid` (Linux only) with fallback to `ffmpeg /dev/video*`.
- Add `GET /api/cameras/mipi/list` and `GET /api/cameras/mipi/preview`.
- Ensure only one MIPI camera can be added (matches anker project logic).

### Phase 3 — Recording and polish

- Recording endpoints (`/api/cameras/recording/*`).
- Stream/record toggle in UI.
- Camera layout and active camera selection.

## Implementation notes

- The existing `Camera` type in `frontend/src/types.ts` currently has `type: 'built-in' | 'usb' | 'stream'`. Add `'mipi'` as a new option.
- `CameraConfig` should map to the frontend `Camera` type; the backend is the source of truth once the module is wired.
- `ffmpeg` must be installed on the host for USB streaming.
- MIPI support is Linux / Raspberry Pi only. On Windows it can be a no-op or hidden.
