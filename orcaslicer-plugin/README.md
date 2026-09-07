# OpenPolyPrint Bridge — OrcaSlicer plugin

An OrcaSlicer (Python) plugin that syncs your slicer to an [OpenPolyPrint](../)
server so it can scan your G-code, presets and settings — and run an AI review
of them (the AI check only works when a **Gemini API key** is configured in
OpenPolyPrint → Settings).

## Requirements

- **OrcaSlicer on any platform**: Windows, macOS, or Linux; x64 or ARM — the
  plugin is pure Python standard library (no dependencies, no compiled code),
  so it runs anywhere OrcaSlicer's embedded CPython runs.
- **OrcaSlicer with the plugin system** (File → Plugins). Older builds without
  it are not supported.
- Network access from the OrcaSlicer machine to the OpenPolyPrint server
  (outbound only — no inbound ports or firewall rules needed).

OrcaSlicer usually runs on a **different device** than the OpenPolyPrint
server (e.g. OrcaSlicer on your desktop, OpenPolyPrint on a Pi). That's the
supported setup: the plugin pushes everything to the server over HTTP.

## What it does

| Capability | Type | What happens |
|---|---|---|
| Heartbeat | background | Announces this OrcaSlicer instance every 30 s, so it shows as **online** on the OpenPolyPrint *Slicer Bridge* page |
| G-code Sync | slicing pipeline | After each slice, uploads the exported G-code file. OpenPolyPrint parses the embedded `; key = value` config block, so the **full resolved settings** arrive with it |
| Sync Now | script | Pushes a snapshot of the current **preset bundle** (printers / print / filament presets + current selections) |

## Install (on the OrcaSlicer machine)

**Easiest** — download an installer from the Slicer Bridge page (or directly
below), run it on the OrcaSlicer machine, and it downloads the plugin from
your server into the right folder automatically:

- Windows: `http://<openpolyprint-host>/api/orca/install/windows` (`.bat`)
- macOS: `http://<openpolyprint-host>/api/orca/install/mac` (`.sh`)
- Linux: `http://<openpolyprint-host>/api/orca/install/linux` (`.sh`)

**Manual** — download the plugin from your OpenPolyPrint server
(`http://<openpolyprint-host>/api/orca/plugin`, also linked from the Slicer
Bridge page), then create the plugin folder inside OrcaSlicer's data
directory and put the file in it, so you end up with:

- **Windows:** `%APPDATA%\OrcaSlicer\orca_plugins\OpenPolyPrintBridge\plugin.py`
- **macOS:** `~/Library/Application Support/OrcaSlicer/orca_plugins/OpenPolyPrintBridge/plugin.py`
- **Linux:** `~/.config/OrcaSlicer/orca_plugins/OpenPolyPrintBridge/plugin.py`

Then:

1. Restart OrcaSlicer (or reload plugins from the Plugins dialog:
   File → Plugins).
2. Open File → Plugins → *OpenPolyPrint Bridge* → configure:
   - **Server URL** — the network address of OpenPolyPrint as seen from this
     machine, e.g. `http://raspberrypi:80` or `http://192.168.1.50`. A bare
     `host:port` works too (http is assumed). The Slicer Bridge page shows the
     exact URL to use, with a copy button. Plain HTTP avoids TLS friction;
     HTTPS works with *Verify TLS certificate* off (self-signed local CA).
   - **Passcode** — only if OpenPolyPrint has a passcode set. The plugin logs
     in and uses the session token automatically.
5. The first network operation triggers OrcaSlicer's permission dialog —
   allow it so heartbeats/uploads can reach the server.
6. Click **Run** on *Sync Now* once to verify the connection, or just slice
   something — the G-code uploads automatically.
7. Reload the plugin after changing settings for them to take effect.

If you have a checkout of this repo instead, copy the `OpenPolyPrintBridge`
folder from `orcaslicer-plugin/` into `orca_plugins/` — same result.

## Viewing the data in OpenPolyPrint

Open the **Slicer Bridge** page in OpenPolyPrint (Files → Slicer Bridge):

- **Connect OrcaSlicer** — the exact server URL for the plugin config (with
  copy button), a plugin download button, per-platform one-click installers,
  and full install steps.
- **Connected Slicers** — your OrcaSlicer instances with online status,
  OrcaSlicer/plugin versions.
- **Synced Artifacts** — uploaded G-code (with parsed settings), preset
  snapshots, settings snapshots. Select one to inspect its full settings
  table and presets.
- **AI Check** — runs a Gemini review of the artifact's G-code, settings and
  presets (issues with severity, concrete recommendations). The result is
  stored on the artifact. This button only works when a Gemini API key is set
  in OpenPolyPrint Settings (or `GEMINI_API_KEY` env var).

## API the plugin uses

| Method | Path | Auth | Purpose |
|---|---|---|---|
| POST | `/api/orca/announce` | passcode token | Heartbeat; response includes whether AI review is enabled |
| POST | `/api/orca/artifacts` (multipart) | passcode token | G-code upload |
| POST | `/api/orca/artifacts` (JSON) | passcode token | Presets / settings snapshot |
| POST | `/api/auth/login` | public | Exchange passcode for session token |
| GET | `/api/orca/plugin` | public | Download the plugin script itself |
| GET | `/api/orca/install/{windows\|mac\|linux}` | public | One-click installer scripts (download the plugin into `orca_plugins/`) |

Read/analyze endpoints for the UI: `GET /api/orca/instances`,
`GET /api/orca/artifacts[?type=]`, `GET/DELETE /api/orca/artifacts/{id}`,
`GET /api/orca/artifacts/{id}/content`,
`POST /api/orca/artifacts/{id}/analyze`.

## Notes

- G-code artifacts are capped at 100 on the server; the oldest are pruned
  automatically.
- The preset bundle snapshot probes the OrcaSlicer host API defensively —
  depending on the OrcaSlicer build it may contain names/metadata only. The
  richest settings source is the G-code upload, which carries the full
  resolved config block.
- If uploads fail, check `log/python_*.log` in the OrcaSlicer data directory
  for `[openpolyprint-bridge]` lines. Common causes: wrong server URL
  (remember: the server's address, not localhost), server down, or the
  network permission prompt was denied (reload the plugin to retrigger it).
