# AnkerCTL API Integration Guide

Complete reference for building a third-party app that integrates with AnkerMake M5/M5C printers via the AnkerCTL web server.

## Table of Contents

1. [Architecture Overview](#architecture-overview)
2. [Authentication](#authentication)
3. [Configuration & Login](#configuration--login)
4. [Printer Status & Data](#printer-status--data)
5. [WebSocket Streams](#websocket-streams)
6. [Print Controls](#print-controls)
7. [GCode Commands](#gcode-commands)
8. [Movement Controls](#movement-controls)
9. [Temperature Controls](#temperature-controls)
10. [File Management](#file-management)
11. [Camera & Video](#camera--video)
12. [Recording & Timelapse](#recording--timelapse)
13. [Pi GPIO & Sensors](#pi-gpio--sensors)
14. [MQTT Command Reference](#mqtt-command-reference)
15. [PPPP Protocol Reference](#pppp-protocol-reference)
16. [OctoPrint-Compatible API](#octoprint-compatible-api)
17. [Server Settings](#server-settings)
18. [History](#history)

---

## Architecture Overview

AnkerCTL acts as a bridge between your app and the AnkerMake printer:

```
Your App  ←─ HTTP/WebSocket ─→  AnkerCTL Server  ←─ MQTT  ─→  AnkerMake Cloud
                                                         ←─ PPPP  ─→  Printer (LAN)
```

- **MQTT**: Used for commands (pause, resume, home, temps, gcode) and status updates (print progress, temps, layers). Goes through AnkerMake cloud servers.
- **PPPP**: Peer-to-peer LAN protocol for video streaming and file transfers. Direct UDP connection to the printer.
- **REST API**: All endpoints return JSON unless noted otherwise.
- **WebSockets**: Real-time streams for MQTT messages, video frames, and control messages.

### Base URL

```
http://<host>:<port>
```

Default port is `8080`. All endpoints are relative to this base URL.

---

## Authentication

If the server has an auth token configured, all API requests must include it:

**Header:**
```
Authorization: Bearer <token>
```

**Or query parameter:**
```
?token=<token>
```

If no token is configured, no authentication is required.

---

## Configuration & Login

Before using printer endpoints, the server must have a valid config loaded. There are several ways to get one:

### Direct Login (Email + Password)

```
POST /api/ankerctl/config/login
```

**Request:**
```json
{
  "email": "user@example.com",
  "password": "yourpassword",
  "region": "eu"
}
```

- `region`: `"eu"` or `"us"` (defaults to `"eu"`)

**Response (success):**
```json
{
  "success": true,
  "message": "Login successful. Config imported."
}
```

**Response (CAPTCHA required):**
```json
{
  "success": false,
  "message": "CAPTCHA required",
  "code": 100032,
  "captcha_id": "xxx",
  "captcha_img": "base64-encoded-image"
}
```

If CAPTCHA is required, retry with `captcha_id` and `captcha_answer` fields.

**Response (failure):**
```json
{
  "success": false,
  "message": "Login failed (code XXX): error message",
  "code": 100032
}
```

### Upload login.json (from AnkerMake Slicer / eufyMake Studio)

```
POST /api/ankerctl/config/upload
```

Multipart form upload of the `login.json` file found in the AnkerMake Slicer or eufyMake Studio app data directory.

### Auto-detect login.json on the server

```
GET /api/ankerctl/config/detect
```

Returns paths where `login.json` was found on the server's filesystem.

### Auto-import from detected login.json

```
POST /api/ankerctl/config/auto-import
```

Automatically finds and imports `login.json` from known application data paths.

### WebView2 detection (eufyMake Studio)

```
GET /api/ankerctl/config/webview2/detect
POST /api/ankerctl/config/webview2/import
```

### LAN Search (discover printers on local network)

```
GET /api/ankerctl/config/lan-search
```

Returns a list of printers discovered on the LAN via mDNS/PPPP broadcast.

### Offline Setup (manual printer config without cloud)

```
POST /api/ankerctl/config/offline-setup
```

Allows configuring a printer manually with IP address and DUID without logging in to AnkerMake cloud.

### Stored Login Management

| Method | Endpoint | Description |
|--------|----------|-------------|
| `GET` | `/api/ankerctl/config/stored-login` | Check if login.json is stored |
| `POST` | `/api/ankerctl/config/stored-login/reload` | Reload config from stored login.json |
| `DELETE` | `/api/ankerctl/config/stored-login` | Permanently delete stored login.json |
| `GET` | `/api/ankerctl/config/export-file` | Download login.json backup |
| `GET` | `/api/ankerctl/config/export-keys` | Export encryption keys |

### Logout

```
POST /api/ankerctl/config/logout
```

Deletes the active config and stops all services. Stored `login.json` is preserved.

### Select Printer

```
POST /api/ankerctl/printer/select
```

**Request:**
```json
{
  "index": 0
}
```

Switches the active printer if multiple are configured.

### Server Reload

```
GET /api/ankerctl/server/reload
```

Reloads config and restarts all services.

---

## Printer Status & Data

### Full Printer Status

```
GET /api/printer/status
```

**Response:**
```json
{
  "connected": true,
  "state": "printing",
  "fileName": "benchy.gcode",
  "progress": 45.2,
  "timeElapsed": 1234,
  "timeRemaining": 5678,
  "totalTime": 6912,
  "nozzleTemp": 210.5,
  "setNozzleTemp": 210.0,
  "bedTemp": 60.0,
  "setBedTemp": 60.0,
  "printSpeed": 100,
  "layerNum": 45,
  "layerCount": 100,
  "lastUpdate": "2026-08-21T20:00:00Z",
  "serverUptime": 3600,
  "serverStarted": "2026-08-21T19:00:00Z"
}
```

### Temperatures Only

```
GET /api/printer/temps
```

```json
{
  "nozzleTemp": 210.5,
  "setNozzleTemp": 210.0,
  "bedTemp": 60.0,
  "setBedTemp": 60.0,
  "lastUpdate": "2026-08-21T20:00:00Z"
}
```

### Print Progress

```
GET /api/printer/progress
```

```json
{
  "fileName": "benchy.gcode",
  "progress": 45.2,
  "timeElapsed": 1234,
  "timeRemaining": 5678,
  "totalTime": 6912,
  "printSpeed": 100,
  "layerNum": 45,
  "layerCount": 100,
  "state": "printing"
}
```

### Server Info

```
GET /api/server/info
```

```json
{
  "serverUptime": 3600,
  "serverStarted": "2026-08-21T19:00:00Z",
  "version": "1.9.0",
  "host": "0.0.0.0",
  "port": 8080,
  "localIPs": ["192.168.1.100"],
  "printer": {
    "name": "My M5C",
    "model": "M5C",
    "sn": "V8110XXXXXXXXX"
  },
  "printerCount": 1,
  "configured": true,
  "printerState": "printing",
  "printerConnected": true
}
```

### State Values

| State | Description |
|-------|-------------|
| `idle` | Printer is idle, not printing |
| `printing` | Print in progress |
| `paused` | Print paused |

---

## WebSocket Streams

### MQTT Stream (real-time printer data)

```
GET /ws/mqtt  (WebSocket upgrade)
```

Receives real-time MQTT messages from the printer. Each message is a JSON object. The server sends a ping every 30s; if no pong is received within 60s, the connection is closed.

**Message types** (identified by `commandType` field):

| commandType | Name | Fields |
|-------------|------|--------|
| `1001` | Print Schedule | `name`, `progress`, `totalTime`, `time` (remaining), `state` |
| `1003` | Temperatures | `nozzleTemp`, `setNozzleTemp`, `bedTemp`, `setBedTemp` |
| `1005` | Print Speed/Layers | `printSpeed`, `layerNum`, `layerCount` |

**Example message:**
```json
{
  "commandType": 1003,
  "nozzleTemp": 210.5,
  "setNozzleTemp": 210.0,
  "bedTemp": 60.0,
  "setBedTemp": 60.0
}
```

### Video Stream (built-in camera via PPPP)

```
GET /ws/video  (WebSocket upgrade)
```

- First message is a text frame: `{"stream":"pppp","ready":true}`
- Subsequent messages are **binary** frames containing raw H.264 video data
- Feed binary frames to a video decoder (JMuxer, WebCodecs, or your own H.264 decoder)
- A ping/pong keepalive runs every 30s
- If no frames arrive for 15s, the frontend auto-reconnects

### USB Camera Video Stream

```
GET /ws/video/usb/{cameraID}  (WebSocket upgrade)
```

- First message is text: `{"stream":"usb","camera":"<cameraID>"}`
- Subsequent messages are **binary** JPEG frames
- Render frames on a `<canvas>` or `<img>` element

### Control Socket (light/quality control)

```
GET /ws/ctrl  (WebSocket upgrade)
```

- First message is text: `{"ankerctl":1}`
- **Send** JSON messages to control the printer camera:

```json
{ "light": true }          // Turn camera light on/off
{ "quality": 0 }           // Set video quality (0=low, 1=high)
```

---

## Print Controls

### Pause Print

```
POST /api/printer/pause
```

No request body needed. Sends MQTT `CmdPrintControl` (0x03f0) with value `0`.

```json
{ "success": true }
```

### Resume Print

```
POST /api/printer/resume
```

Sends MQTT `CmdPrintControl` with value `1`.

### Stop Print

```
POST /api/printer/stop
```

Sends MQTT `CmdPrintControl` with value `2`.

### Auto Bed Leveling

```
POST /api/printer/auto-level
```

Sends MQTT `CmdAutoLeveling` (0x03ef = 1007) with value `1`.

```json
{ "success": true, "command": "auto_leveling" }
```

### Home All Axes

```
POST /api/printer/home
```

Sends MQTT `CmdMoveZero` (0x0402 = 1026) with value `0`.

```json
{ "success": true, "command": "home" }
```

### Z-Axis Recoup/Calibration

```
POST /api/printer/z-axis-recoup
```

Sends MQTT `CmdZAxisRecoup` (0x03fd = 1021) with value `1`.

```json
{ "success": true, "command": "z_axis_recoup" }
```

---

## GCode Commands

### Send Raw GCode

```
POST /api/printer/gcode/send
```

**Request:**
```json
{
  "command": "G28\nG0 X10 Y10 F3000"
}
```

Multiple lines are supported (separated by `\n`). Lines starting with `;` are treated as comments and skipped. Each line is sent as a separate MQTT `CmdGcodeCommand` (0x0413) message.

**Response:**
```json
{ "success": true, "sent": 2 }
```

### Get Current GCode (for 3D viewer)

```
GET /api/printer/gcode/current
```

Returns the currently loaded GCode file content as plain text.

---

## Movement Controls

Movement is done by sending GCode commands via the `/api/printer/gcode/send` endpoint:

### Relative Movement (Jog)

```
POST /api/printer/gcode/send
```

```json
{ "command": "G91\nG0 X10 F3000\nG90" }
```

- `G91`: Set relative positioning mode
- `G0 X10`: Move X axis by +10mm at 3000mm/min feedrate
- `G90`: Return to absolute positioning mode

### Z-Axis Movement

```json
{ "command": "G91\nG0 Z5 F600\nG90" }
```

Z-axis typically uses a slower feedrate (F600 vs F3000).

### Homing Individual Axes

```json
{ "command": "G28 X Y" }   // Home X and Y only
{ "command": "G28 Z" }     // Home Z only
{ "command": "G28" }       // Home all axes
```

---

## Temperature Controls

Set temperatures by sending GCode commands:

### Set Nozzle Temperature

```json
{ "command": "M104 S210" }
```

### Set Bed Temperature

```json
{ "command": "M140 S60" }
```

### Wait for Temperature

```json
{ "command": "M109 S210" }   // Wait for nozzle
{ "command": "M190 S60" }    // Wait for bed
```

Alternatively, you can use the native MQTT commands by sending a custom command via the MQTT client (see [MQTT Command Reference](#mqtt-command-reference)):

- `CmdNozzleTemp` (0x03eb = 1003) — set nozzle temp
- `CmdHotbedTemp` (0x03ec = 1004) — set bed temp
- `CmdFanSpeed` (0x03ed = 1005) — set fan speed
- `CmdPrintSpeed` (0x03ee = 1006) — set print speed

---

## File Management

### Upload GCode File

```
POST /api/gcode/files
```

Multipart form upload with field name `file`.

**Response:**
```json
{
  "success": true,
  "id": "abc123",
  "name": "Benchy",
  "layers": 100,
  "size": 1234567,
  "thumbnail": "data:image/png;base64,..."
}
```

### List GCode Files

```
GET /api/gcode/files
```

```json
{
  "files": [
    {
      "id": "abc123",
      "name": "Benchy",
      "filename": "benchy.gcode",
      "size": 1234567,
      "layers": 100,
      "estTime": 3600,
      "filament": "PLA",
      "uploaded": "2026-08-21T19:00:00Z",
      "thumbnail": "data:image/png;base64,..."
    }
  ],
  "count": 1
}
```

### Get GCode File

```
GET /api/gcode/files/{id}
```

Returns file metadata + content. Add `?content=1` for raw GCode text.

### Get Thumbnail

```
GET /api/gcode/files/{id}/thumbnail
```

Returns the thumbnail image.

### Delete GCode File

```
DELETE /api/gcode/files/{id}
```

### Set Active GCode File

```
POST /api/gcode/set-active
```

**Request:**
```json
{ "id": "abc123" }
```

### Clear Active GCode

```
POST /api/gcode/clear
```

### OctoPrint-Compatible File Upload

```
POST /api/files/local
```

Multipart form upload (OctoPrint-compatible format).

### Select File for Printing (OctoPrint-compatible)

```
POST /api/files/local/{path}
```

```json
{ "command": "select", "print": true }
```

---

## Camera & Video

### Get Camera Settings

```
GET /api/cameras/settings
```

### Set Camera Settings

```
POST /api/cameras/settings
```

### List Cameras

```
GET /api/cameras
```

### Add Camera

```
POST /api/cameras
```

### Update Camera

```
PUT /api/cameras
```

### Remove Camera

```
DELETE /api/cameras
```

### Camera Status

```
GET /api/cameras/status
```

### List USB Cameras

```
GET /api/cameras/usb/list
```

Returns available USB cameras detected on the system.

### USB Camera Preview

```
GET /api/cameras/usb/preview
```

Returns a single JPEG snapshot from a USB camera.

### Video Download

```
GET /video
```

Downloads the live video stream as a raw H.264 file (for testing).

---

## Recording & Timelapse

### List Recordings

```
GET /api/timelapses
```

```json
{
  "videos": [
    {
      "id": 1,
      "session_id": "abc123",
      "camera_name": "Built-in",
      "filename": "rec_001.mp4",
      "print_file": "benchy.gcode",
      "created_at": "2026-08-21T19:00:00Z",
      "duration": "01:23:45",
      "size": 123456789,
      "status": "ready"
    }
  ],
  "count": 1
}
```

### Delete Recording

```
DELETE /api/timelapses
```

```json
{ "id": 1 }
```

### Get Timelapse Settings

```
GET /api/timelapse/settings
```

```json
{
  "storageType": "local",
  "savePath": "/path/to/videos",
  "autoCreateDir": true,
  "interval": 10,
  "ffmpegPath": "ffmpeg"
}
```

### Set Timelapse Settings

```
POST /api/timelapse/settings
```

### Download Timelapse

```
GET /api/timelapse/download?id=1
```

Returns the video file (MP4/MKV/WebM).

### Get Thumbnail

```
GET /api/timelapse/thumbnail?id=1
```

Returns a JPEG thumbnail extracted from the video (at 50% duration).

### Session Metadata

```
GET /api/timelapse/session?session=abc123
```

Returns metadata about a recording session including printer state, cameras, and events.

### Get GCode from Recording

```
GET /api/timelapse/gcode?filename=benchy.gcode
```

### Convert Video

```
POST /api/timelapse/convert
```

Multipart upload, converts to MP4 H.265 via ffmpeg.

### Recording Control

| Method | Endpoint | Description |
|--------|----------|-------------|
| `GET` | `/api/cameras/recording/settings` | Get recording settings |
| `POST` | `/api/cameras/recording/settings` | Set recording settings |
| `POST` | `/api/cameras/recording/start` | Start recording |
| `POST` | `/api/cameras/recording/stop` | Stop recording |
| `GET` | `/api/cameras/recording/status` | Get recording status |
| `POST` | `/api/cameras/recording/convert` | Convert recording format |
| `POST` | `/api/cameras/recording/upload` | Upload recording |

### List Storage Drives

```
GET /api/drives
```

Returns available storage locations for saving recordings.

---

## Pi GPIO & Sensors

### Get Pi Settings

```
GET /api/pi/settings
```

```json
{
  "lightRelayEnabled": true,
  "lightRelayGpio": 18,
  "lightRelayOn": false,
  "filamentSensors": [
    {
      "id": 1,
      "enabled": true,
      "name": "Sensor 1",
      "gpioPin": 23,
      "filamentType": "PLA",
      "color": "#FF0000"
    }
  ],
  "filamentSensorCount": 1,
  "filamentSensorMax": 5,
  "sensorManagerRunning": true,
  "gpioAvailable": true,
  "os": "linux"
}
```

### Set Pi Settings

```
POST /api/pi/settings
```

```json
{
  "lightRelayEnabled": true,
  "lightRelayGpio": 18,
  "filamentSensors": [
    {
      "id": 1,
      "enabled": true,
      "name": "Sensor 1",
      "gpioPin": 23,
      "filamentType": "PLA",
      "color": "#FF0000"
    }
  ]
}
```

### Toggle Light Relay

```
POST /api/pi/light/toggle
```

```json
{ "on": true }
```

**Response:**
```json
{
  "success": true,
  "on": true
}
```

### Get Filament Sensor Readings

```
GET /api/pi/sensors/readings
```

```json
{
  "sensors": [
    {
      "id": 1,
      "enabled": true,
      "name": "Sensor 1",
      "gpioPin": 23,
      "filamentType": "PLA",
      "color": "#FF0000",
      "temp": 22.5,
      "humidity": 45.0,
      "hasReading": true,
      "updatedAt": 1692646400
    }
  ],
  "count": 1,
  "sensorManagerRunning": true,
  "lightRelayEnabled": true,
  "lightRelayGpio": 18,
  "lightRelayOn": false,
  "gpioAvailable": true,
  "os": "linux"
}
```

---

## MQTT Command Reference

The printer communicates via encrypted MQTT messages. Each command has a `commandType` (hex value) and optional fields. The AnkerCTL server handles encryption/decryption automatically.

### Command Types (Sent TO Printer)

| commandType | Hex | Name | Fields |
|-------------|-----|------|--------|
| 1000 | 0x03E8 | Event Notify | `event_type` |
| 1001 | 0x03E9 | Print Schedule | `schedule_type` |
| 1002 | 0x03EA | Firmware Version | — |
| 1003 | 0x03EB | Nozzle Temp | `value` (°C) |
| 1004 | 0x03EC | Hotbed Temp | `value` (°C) |
| 1005 | 0x03ED | Fan Speed | `value` (0-255) |
| 1006 | 0x03EE | Print Speed | `value` (%) |
| 1007 | 0x03EF | Auto Leveling | `value` (1=start) |
| 1008 | 0x03F0 | Print Control | `control` (0=pause, 1=resume, 2=stop) |
| 1009 | 0x03F1 | File List Request | — |
| 1010 | 0x03F2 | Gcode File Request | `filename` |
| 1011 | 0x03F3 | Allow Firmware Update | — |
| 1012 | 0x03FC | Gcode File Download | `filename` |
| 1013 | 0x03FD | Z-Axis Recoup | `value` (1=start) |
| 1014 | 0x03FE | Extrusion Step | `value` |
| 1015 | 0x03FF | Enter/Quit Material | `value` |
| 1016 | 0x0400 | Move Step | `axis`, `step`, `speed` |
| 1017 | 0x0401 | Move Direction | `axis`, `direction` |
| 1018 | 0x0402 | Move Zero (Home) | `value` (0=all) |
| 1019 | 0x0403 | App Query Status | — |
| 1020 | 0x0404 | Online Notify | — |
| 1021 | 0x0405 | Recover Factory | — |
| 1023 | 0x0407 | BLE On/Off | `value` (0=off, 1=on) |
| 1024 | 0x0408 | Delete Gcode File | `filename` |
| 1025 | 0x0409 | Reset Gcode Param | — |
| 1026 | 0x040A | Device Name Set | `name` |
| 1027 | 0x040B | Device Log Upload | — |
| 1028 | 0x040C | On/Off Modal | `value` |
| 1029 | 0x040D | Motor Lock | `value` |
| 1030 | 0x040E | Preheat Config | `nozzle_temp`, `bed_temp` |
| 1031 | 0x040F | Break Point | — |
| 1032 | 0x0410 | AI Calibration | — |
| 1033 | 0x0411 | Video On/Off | `value` (0=off, 1=on) |
| 1034 | 0x0412 | Advanced Parameters | — |
| 1035 | 0x0413 | Gcode Command | `gcode` (string) |
| 1036 | 0x0414 | Preview Image URL | `url` |
| 1041 | 0x0419 | System Check | — |
| 1042 | 0x041A | AI Switch | `value` |
| 1043 | 0x041B | AI Info Check | — |
| 1044 | 0x041C | Model Layer | — |
| 1045 | 0x041D | Model DL Process | — |
| 1047 | 0x041F | Print Max Speed | `value` |

### Response Types (Received FROM Printer via WebSocket)

| commandType | Name | Fields |
|-------------|------|--------|
| 1001 | Print Status | `name`, `progress` (0-100), `totalTime`, `time` (remaining sec) |
| 1003 | Temperatures | `nozzleTemp`, `setNozzleTemp`, `bedTemp`, `setBedTemp` |
| 1005 | Print Speed/Layers | `printSpeed`, `layerNum`, `layerCount` |

### Sending Commands

Commands are sent via the MQTT service. The server exposes this through REST endpoints (see above sections) and through the WebSocket control socket. Internally, the Go MQTT client (`AnkerMQTTClient.Command()`) handles:

1. JSON encoding the command
2. Encrypting with AES-CTR using the printer's key
3. Packing into MQTT packet format (header + encrypted payload)
4. Publishing to the printer's MQTT topic

### MQTT Packet Format

```
[1 byte: packet type] [2 bytes: payload length] [N bytes: encrypted payload]
```

- Packet types: `0xC0` (single), `0xC1` (multi-begin), `0xC2` (multi-append), `0xC3` (multi-finish)
- Multi-packet messages are used when payload exceeds MTU
- Payload is AES-CTR encrypted with a per-printer key derived from the printer's DUID

### M5C Header Format

The M5C uses a shorter 24-byte MQTT header (no timestamp or GUID fields), while the M5 uses a longer header with timestamp and GUID.

---

## PPPP Protocol Reference

PPPP (Peer-to-Peer Protocol) is a UDP-based LAN protocol used for video streaming and file transfers. It operates on ports 32108 (LAN) and 32100 (WAN).

### Connection Flow

```
Client                          Printer
  │                                │
  │── PktLanSearch (0x30) ───────→│
  │←─ PktLanNotify (0x31) ────────│
  │── PktLanNotifyAck (0x32) ────→│
  │                                │
  │── PktP2PReq (0x43) ──────────→│
  │←─ PktP2PReqAck (0x44) ────────│
  │── PktP2PRdy (0x42) ──────────→│
  │←─ PktP2PRdyAck (0x43) ────────│
  │                                │
  │══ Connected ══════════════════│
  │                                │
  │── PktDrw (data) ─────────────→│  (commands)
  │←─ PktDrw (data) ──────────────│  (responses)
  │                                │
  │── PktAlive (0x3F) ───────────→│  (keepalive)
  │←─ PktAliveAck (0x40) ─────────│
```

### Key Packet Types

| Type | Hex | Direction | Description |
|------|-----|-----------|-------------|
| LanSearch | 0x30 | → | Discover printers on LAN |
| LanNotify | 0x31 | ← | Printer announces itself |
| LanNotifyAck | 0x32 | → | Acknowledge printer found |
| P2PReq | 0x43 | → | Request P2P connection |
| P2PReqAck | 0x44 | ← | Acknowledge P2P request |
| P2PRdy | 0x42 | → | Connection ready |
| P2PRdyAck | 0x43 | ← | Acknowledge ready |
| Alive | 0x3F | → | Keepalive ping |
| AliveAck | 0x40 | ← | Keepalive pong |
| Drw | 0x44 | ↔ | Data read/write (carries XZYH/AABB) |
| Close | 0x41 | → | Close connection |

### XZYH Data Format

XZYH packets carry JSON commands and binary data over PPPP channels:

```
[4 bytes: "XZYH"] [4 bytes: payload length] [8 bytes: reserved] [N bytes: JSON payload]
```

The JSON payload contains `commandType` and command-specific fields, similar to MQTT commands.

### Channels

PPPP supports up to 8 channels (0-7):
- **Channel 0**: Control commands (JSON via XZYH)
- **Channel 1**: Video stream (H.264 frames via XZYH)
- **Channel 2-7**: File transfers and other data

### Video Service Commands (via PPPP XZYH)

| SubCmd | Description |
|--------|-------------|
| `SubCmdStartLive` | Start live video stream |
| `SubCmdCloseLive` | Stop live video stream |
| `SubCmdLightStateSwitch` | Toggle camera light (`{"open": true/false}`) |
| `SubCmdLiveModeSet` | Set video quality (`{"mode": 0/1}`) |

### File Transfer (via PPPP AABB)

File transfers use the AABB protocol on PPPP channels. The `FileTransferService` handles:
- Initiating file upload to printer
- Chunking large files
- Handling ACK/retry logic

---

## OctoPrint-Compatible API

AnkerCTL provides OctoPrint-compatible endpoints for slicer integration (e.g., PrusaSlicer, Cura):

### Version

```
GET /api/version
```

```json
{
  "api": "0.1",
  "server": "1.9.0",
  "text": "OctoPrint 1.9.0"
}
```

### Connection

```
GET /api/connection
POST /api/connection
```

GET returns connection state. POST accepts connect/disconnect commands (no-op, always succeeds).

```json
{
  "current": { "state": "Operational" },
  "options": {
    "ports": ["VIRTUAL"],
    "baudrates": [0],
    "printerProfiles": [{ "id": "default", "name": "AnkerMake M5" }],
    "autoconnect": false
  }
}
```

State mapping: `idle` → `Operational`, `printing` → `Printing`, `paused` → `Paused`, disconnected → `Offline`.

### Job

```
GET /api/job
```

```json
{
  "job": {
    "file": { "name": "benchy.gcode", "origin": "local", "size": 0, "date": 0 },
    "estimatedPrintTime": 6912
  },
  "progress": {
    "completion": 45.2,
    "filepos": 0,
    "printTime": 1234,
    "printTimeLeft": 5678
  },
  "state": "Printing"
}
```

### Printer

```
GET /api/printer
```

```json
{
  "state": {
    "text": "Printing",
    "flags": {
      "operational": true,
      "printing": true,
      "closedOrError": false,
      "error": false
    }
  },
  "temperature": {
    "tool0": { "actual": 210.5, "target": 210.0, "offset": 0 },
    "bed": { "actual": 60.0, "target": 60.0, "offset": 0 }
  }
}
```

### File Upload (OctoPrint format)

```
POST /api/files/local
```

Multipart form with `file` field and optional `print` parameter.

### File Select (OctoPrint format)

```
POST /api/files/local/{path}
```

```json
{ "command": "select", "print": true }
```

---

## Server Settings

### Get Server Settings

```
GET /api/ankerctl/server/settings
```

```json
{
  "host": "0.0.0.0",
  "port": 8080
}
```

### Set Server Settings

```
POST /api/ankerctl/server/settings
```

```json
{
  "host": "0.0.0.0",
  "port": 8080
}
```

### Debug Stream Log

```
GET /api/debug/streamlog
```

Returns server-side debug log entries (for camera debugging).

---

## History

### List Print History

```
GET /api/history
```

```json
{
  "entries": [
    {
      "id": 1,
      "printer": "My M5C",
      "filename": "benchy.gcode",
      "status": "completed",
      "started_at": "2026-08-21T19:00:00Z",
      "duration": "01:23:45",
      "ended_at": "2026-08-21T20:23:45Z"
    }
  ],
  "count": 1
}
```

### Add History Entry

```
POST /api/history
```

```json
{
  "printer": "My M5C",
  "filename": "benchy.gcode",
  "status": "completed",
  "duration": "01:23:45"
}
```

### Delete History Entry

```
DELETE /api/history
```

```json
{ "id": 1 }
```

### Clear All History

```
POST /api/history/clear
```

---

## Simulation Mode

For testing without a real printer:

| Method | Endpoint | Description |
|--------|----------|-------------|
| `GET` | `/api/sim/state` | Get simulated print state |
| `POST` | `/api/sim/start` | Start simulated print |
| `POST` | `/api/sim/pause` | Pause simulated print |
| `POST` | `/api/sim/resume` | Resume simulated print |
| `POST` | `/api/sim/stop` | Stop simulated print |

---

## Quick Start Example (Python)

```python
import requests
import websocket
import json

BASE = "http://192.168.1.100:8080"

# 1. Login
resp = requests.post(f"{BASE}/api/ankerctl/config/login", json={
    "email": "user@example.com",
    "password": "yourpassword",
    "region": "eu"
})
print(resp.json())

# 2. Get printer status
status = requests.get(f"{BASE}/api/printer/status").json()
print(f"State: {status['state']}, Progress: {status['progress']}%")

# 3. Send GCode
requests.post(f"{BASE}/api/printer/gcode/send", json={
    "command": "G28\nM104 S210\nM140 S60"
})

# 4. Start a print
requests.post(f"{BASE}/api/printer/pause")   # pause
requests.post(f"{BASE}/api/printer/resume")  # resume
requests.post(f"{BASE}/api/printer/stop")    # stop

# 5. Home all axes
requests.post(f"{BASE}/api/printer/home")

# 6. Toggle light (Pi GPIO)
requests.post(f"{BASE}/api/pi/light/toggle", json={"on": True})

# 7. WebSocket for real-time updates
def on_message(ws, message):
    data = json.loads(message)
    ct = data.get("commandType")
    if ct == 1001:
        print(f"Print: {data['progress']}% - {data.get('name', '')}")
    elif ct == 1003:
        print(f"Temps: nozzle={data['nozzleTemp']}°C bed={data['bedTemp']}°C")
    elif ct == 1005:
        print(f"Layer: {data['layerNum']}/{data['layerCount']} speed={data['printSpeed']}%")

ws = websocket.WebSocketApp(
    f"ws://192.168.1.100:8080/ws/mqtt",
    on_message=on_message
)
ws.run_forever(ping_interval=30)
```

---

## Error Handling

All error responses follow this format:

```json
{ "error": "descriptive error message" }
```

Common HTTP status codes:

| Code | Meaning |
|------|---------|
| 200 | Success |
| 400 | Bad request (missing/invalid parameters) |
| 401 | Unauthorized (auth token required/invalid) |
| 404 | Not found (file, printer, config) |
| 500 | Internal server error |
| 502 | Bad gateway (upstream API failure) |
| 503 | Service unavailable (MQTT/PPPP not connected) |

When you get `503`, it means the printer connection isn't established yet. Wait a few seconds and retry.
