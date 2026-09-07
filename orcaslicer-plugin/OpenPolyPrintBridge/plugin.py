# /// script
# requires-python = ">=3.10"
# dependencies = []
#
# [tool.orcaslicer.plugin]
# name = "OpenPolyPrint Bridge"
# description = "Syncs sliced G-code, presets and settings to an OpenPolyPrint server, where they can be scanned and AI-reviewed (Gemini key required on the server)."
# author = "OpenPolyPrint"
# version = "1.1.0"
# ///
"""OpenPolyPrint Bridge — OrcaSlicer plugin.

Pushes slicer data to an OpenPolyPrint server so it can be scanned and
reviewed there:

- Heartbeat: announces this OrcaSlicer instance every N seconds so the
  OpenPolyPrint UI shows it as online (config: heartbeat).
- "G-code Sync" (slicing pipeline): after each slice, uploads the exported
  G-code file. OpenPolyPrint parses the embedded `; key = value` config
  block, so the full resolved settings travel with the file
  (config: auto_upload_gcode).
- "Sync Now" (script): pushes a snapshot of the current preset bundle.

The plugin talks to OpenPolyPrint's HTTP API. If the server has a passcode
set, configure it in the plugin settings and the plugin will log in and use
the session token. AI review happens on the server and only works when a
Gemini API key is configured in OpenPolyPrint (Settings or GEMINI_API_KEY).

Only the Python standard library is used. The first network operation will
trigger OrcaSlicer's permission dialog — allow it so heartbeats and uploads
can reach the server.
"""

import json
import os
import platform
import socket
import ssl
import threading
import time
import urllib.error
import urllib.request
import uuid

import orca

PLUGIN_VERSION = "1.1.0"
CAPABILITIES = ["heartbeat", "gcode-sync", "presets-sync"]

DEFAULT_CONFIG = {
    "server_url": "http://localhost",
    "passcode": "",
    "auto_upload_gcode": True,
    "heartbeat": True,
    "heartbeat_interval_sec": 30,
    "verify_tls": False,
}

# Auth token cache and the heartbeat thread live at module level: capabilities
# are instantiated once per load, and the heartbeat must survive across them.
_TOKEN = {"value": None}
_TOKEN_LOCK = threading.Lock()
_HEARTBEAT_STOP = threading.Event()
_HEARTBEAT_THREAD = None
# Config snapshot taken on load / execute — the heartbeat thread must not call
# into the host API, so it reuses whatever config was last seen on the main
# thread. Reload the plugin after changing settings for them to apply.
_CONFIG_SNAPSHOT = {}


def _log(msg):
    print(f"[openpolyprint-bridge] {msg}")


def _safe_call(fn, *args, **kwargs):
    try:
        return fn(*args, **kwargs)
    except Exception:
        return None


# ─── Identity ─────────────────────────────────────────────────────────────


def _machine_id():
    host = _safe_call(socket.gethostname) or "unknown"
    return f"{host}:{platform.system()}:{platform.machine()}"


def instance_id():
    """Stable per-machine ID derived from hostname + platform."""
    return str(uuid.uuid5(uuid.NAMESPACE_DNS, "orca-openpolyprint-bridge:" + _machine_id()))


def hostname():
    return _safe_call(socket.gethostname) or "unknown"


def _orca_version():
    for getter in (lambda: orca.__version__, lambda: orca.version()):
        v = _safe_call(getter)
        if v:
            return str(v)
    return ""


# ─── HTTP client ──────────────────────────────────────────────────────────


def _merged_config(cfg):
    merged = dict(DEFAULT_CONFIG)
    merged.update(cfg or {})
    return merged


def _base_url(cfg):
    # Accept bare host[:port] input (e.g. "raspberrypi" or "192.168.1.50:80")
    # and normalize it to an http URL — the plugin only ever talks to the
    # OpenPolyPrint server over the network, scheme-less input defaults to http.
    url = str(cfg.get("server_url", "")).strip().rstrip("/")
    if url and "://" not in url:
        url = "http://" + url
    return url


def _ssl_context(cfg):
    # OpenPolyPrint serves HTTPS with a self-signed local CA by default; leave
    # verification off unless that CA was installed on this machine.
    if not cfg.get("verify_tls", False):
        ctx = ssl.create_default_context()
        ctx.check_hostname = False
        ctx.verify_mode = ssl.CERT_NONE
        return ctx
    return ssl.create_default_context()


def _request_once(cfg, method, path, data=None, headers=None, timeout=60):
    url = _base_url(cfg) + path
    hdrs = dict(headers or {})
    if data is not None and "Content-Type" not in hdrs:
        hdrs["Content-Type"] = "application/json"
    req = urllib.request.Request(url, data=data, headers=hdrs, method=method)
    kwargs = {"timeout": timeout}
    if url.lower().startswith("https"):
        kwargs["context"] = _ssl_context(cfg)
    with urllib.request.urlopen(req, **kwargs) as resp:
        return resp.status, resp.read()


def _login(cfg):
    body = json.dumps({"passcode": cfg.get("passcode", "")}).encode("utf-8")
    status, resp = _request_once(
        cfg, "POST", "/api/auth/login", body,
        {"Content-Type": "application/json"}, timeout=15,
    )
    if status != 200:
        raise RuntimeError("OpenPolyPrint rejected the passcode (check the plugin settings)")
    with _TOKEN_LOCK:
        _TOKEN["value"] = json.loads(resp.decode("utf-8")).get("token") or None


def _api_request(cfg, method, path, payload=None, headers=None, timeout=60):
    """Request to the OpenPolyPrint API with bearer auth.

    On 401, logs in with the configured passcode once and retries.
    payload is a JSON-serializable object, or raw bytes when a Content-Type
    header is supplied (multipart uploads).
    """
    for attempt in (0, 1):
        hdrs = dict(headers or {})
        with _TOKEN_LOCK:
            token = _TOKEN["value"]
        if token:
            hdrs["Authorization"] = "Bearer " + token
        data = payload
        if payload is not None and not isinstance(payload, (bytes, bytearray)) \
                and "Content-Type" not in hdrs:
            data = json.dumps(payload).encode("utf-8")
            hdrs["Content-Type"] = "application/json"
        try:
            return _request_once(cfg, method, path, data, hdrs, timeout)
        except urllib.error.HTTPError as e:
            if e.code == 401 and attempt == 0:
                _login(cfg)
                continue
            raise
    raise RuntimeError("unreachable")


def _post_multipart(cfg, path, fields, file_field, filename, content, timeout=300):
    boundary = "----oppb-" + uuid.uuid4().hex
    parts = []
    for key, value in fields.items():
        parts.append(
            (
                f"--{boundary}\r\n"
                f'Content-Disposition: form-data; name="{key}"\r\n\r\n'
                f"{value}\r\n"
            ).encode("utf-8")
        )
    parts.append(
        (
            f"--{boundary}\r\n"
            f'Content-Disposition: form-data; name="{file_field}"; filename="{filename}"\r\n'
            f"Content-Type: application/octet-stream\r\n\r\n"
        ).encode("utf-8")
    )
    parts.append(content)
    parts.append(f"\r\n--{boundary}--\r\n".encode("utf-8"))
    body = b"".join(parts)
    return _api_request(
        cfg, "POST", path, body,
        {"Content-Type": f"multipart/form-data; boundary={boundary}"},
        timeout,
    )


# ─── Server sync ──────────────────────────────────────────────────────────


def _announce(cfg):
    payload = {
        "instanceId": instance_id(),
        "hostname": hostname(),
        "platform": f"{platform.system()} {platform.release()}",
        "orcaVersion": _orca_version(),
        "pluginVersion": PLUGIN_VERSION,
        "capabilities": CAPABILITIES,
    }
    return _api_request(cfg, "POST", "/api/orca/announce", payload, timeout=15)


def _describe_preset(item):
    entry = {}
    for field in ("name", "key", "type", "vendor", "version", "inherits", "notes"):
        v = _safe_call(getattr, item, field)
        if v not in (None, ""):
            entry[field] = v if isinstance(v, (str, int, float, bool)) else str(v)
    if not entry:
        entry["name"] = str(item)
    return entry


def _collect_collection(bundle, attr):
    """Snapshot one preset collection (printers/prints/filaments/...).

    The host API surface may differ between OrcaSlicer builds, so every
    access is probed defensively and missing collections are skipped.
    """
    col = _safe_call(getattr, bundle, attr)
    if col is None:
        return None
    try:
        items = list(col)
    except Exception:
        return None
    out = {"count": len(items), "presets": [_describe_preset(i) for i in items]}
    selected = _safe_call(getattr, col, "get_selected_preset")
    if selected is not None:
        name = _safe_call(getattr, selected, "name") or _safe_call(getattr, selected, "key")
        if name:
            out["selected"] = str(name)
    return out


def collect_presets():
    """Best-effort snapshot of the current preset bundle via orca.host."""
    bundle = _safe_call(orca.host.preset_bundle)
    if bundle is None:
        return {"error": "preset bundle unavailable (is the GUI ready?)"}
    out = {}
    for attr in ("printers", "prints", "filaments", "sla_prints", "sla_materials"):
        data = _collect_collection(bundle, attr)
        if data:
            out[attr] = data
    for attr in ("printer_settings_id", "print_settings_id", "filament_settings_id"):
        v = _safe_call(getattr, bundle, attr)
        if v:
            out["selected_" + attr.replace("_settings_id", "")] = (
                v if isinstance(v, list) else str(v)
            )
    if not out:
        return {"error": "no preset data exposed by this OrcaSlicer build"}
    return out


# ─── Heartbeat ────────────────────────────────────────────────────────────


def _start_heartbeat(cfg):
    global _HEARTBEAT_THREAD
    if _HEARTBEAT_THREAD is not None and _HEARTBEAT_THREAD.is_alive():
        return
    interval = max(10, int(cfg.get("heartbeat_interval_sec", 30)))
    _HEARTBEAT_STOP.clear()

    def loop():
        while not _HEARTBEAT_STOP.is_set():
            snapshot = _CONFIG_SNAPSHOT or cfg
            if snapshot.get("heartbeat", True):
                try:
                    _announce(snapshot)
                except Exception as e:
                    _log(f"heartbeat failed: {e}")
            _HEARTBEAT_STOP.wait(interval)

    _HEARTBEAT_THREAD = threading.Thread(target=loop, name="oppb-heartbeat", daemon=True)
    _HEARTBEAT_THREAD.start()
    _log(f"heartbeat started (every {interval}s → {_base_url(cfg)})")


# ─── Capabilities ─────────────────────────────────────────────────────────


CONFIG_UI_HTML = """<div style="font-family: sans-serif; color: var(--orca-fg, #222);
  display: grid; gap: 12px; max-width: 480px; font-size: 13px;">
  <label>OpenPolyPrint server URL
    <input id="server_url" placeholder="http://openpolyprint:80"
      style="width:100%; padding:6px; margin-top:4px; box-sizing:border-box;
        background:var(--orca-bg,#fff); color:var(--orca-fg,#222);
        border:1px solid var(--orca-accent,#888); border-radius:4px;">
  </label>
  <label>Server passcode (only if OpenPolyPrint has one set)
    <input id="passcode" type="password"
      style="width:100%; padding:6px; margin-top:4px; box-sizing:border-box;
        background:var(--orca-bg,#fff); color:var(--orca-fg,#222);
        border:1px solid var(--orca-accent,#888); border-radius:4px;">
  </label>
  <label style="display:flex; align-items:center; gap:8px;">
    <input id="auto_upload_gcode" type="checkbox"> Upload sliced G-code automatically after each slice
  </label>
  <label style="display:flex; align-items:center; gap:8px;">
    <input id="heartbeat" type="checkbox"> Announce this slicer (shows as online in OpenPolyPrint)
  </label>
  <label>Heartbeat interval (seconds)
    <input id="heartbeat_interval_sec" type="number" min="10" step="5"
      style="width:100px; padding:6px; margin-top:4px; background:var(--orca-bg,#fff);
        color:var(--orca-fg,#222); border:1px solid var(--orca-accent,#888); border-radius:4px;">
  </label>
  <label style="display:flex; align-items:center; gap:8px;">
    <input id="verify_tls" type="checkbox"> Verify TLS certificate
    <span style="opacity:0.7;">(needs the OpenPolyPrint CA installed)</span>
  </label>
  <p style="opacity:0.7; margin:0;">
    The AI review runs on the server and needs a Gemini API key configured in
    OpenPolyPrint Settings. Reload the plugin after changing these settings.
  </p>
  <div>
    <button id="save" style="padding:8px 16px; background:var(--orca-accent,#2563eb);
      color:#fff; border:none; border-radius:4px; cursor:pointer;">Save</button>
    <span id="status" style="margin-left:10px;"></span>
  </div>
</div>
<script>
  const raw = window.orca.getConfig();
  const cfg = typeof raw === 'string' ? JSON.parse(raw || '{}') : (raw || {});
  const $ = (id) => document.getElementById(id);
  const bool = (k, dflt) => cfg[k] === undefined ? dflt : !!cfg[k];
  $('server_url').value = cfg.server_url || 'http://localhost';
  $('passcode').value = cfg.passcode || '';
  $('auto_upload_gcode').checked = bool('auto_upload_gcode', true);
  $('heartbeat').checked = bool('heartbeat', true);
  $('heartbeat_interval_sec').value = cfg.heartbeat_interval_sec || 30;
  $('verify_tls').checked = bool('verify_tls', false);
  $('save').onclick = () => {
    const next = {
      server_url: $('server_url').value.trim(),
      passcode: $('passcode').value,
      auto_upload_gcode: $('auto_upload_gcode').checked,
      heartbeat: $('heartbeat').checked,
      heartbeat_interval_sec: parseInt($('heartbeat_interval_sec').value, 10) || 30,
      verify_tls: $('verify_tls').checked,
    };
    window.orca.saveConfig(typeof raw === 'string' ? JSON.stringify(next) : next);
    $('status').textContent = 'Saved';
    setTimeout(() => { $('status').textContent = ''; }, 2000);
  };
</script>"""


class SyncNow(orca.script.ScriptPluginCapabilityBase):
    """Pushes a preset bundle snapshot and announces this slicer instance."""

    def get_name(self):
        return "Sync Now"

    def has_config_ui(self):
        return True

    def get_config_ui(self):
        return CONFIG_UI_HTML

    def get_default_config(self):
        return dict(DEFAULT_CONFIG)

    def on_load(self):
        global _CONFIG_SNAPSHOT
        cfg = _merged_config(json.loads(self.get_config() or "{}"))
        _CONFIG_SNAPSHOT = cfg
        _start_heartbeat(cfg)

    def on_unload(self):
        _HEARTBEAT_STOP.set()

    def execute(self):
        global _CONFIG_SNAPSHOT
        cfg = _merged_config(json.loads(self.get_config() or "{}"))
        _CONFIG_SNAPSHOT = cfg
        server = _base_url(cfg)
        try:
            _, body = _announce(cfg)
            ai_enabled = json.loads(body.decode("utf-8")).get("aiEnabled", False)
        except Exception as e:
            return orca.ExecutionResult.failure(
                orca.PluginResult.RecoverableError,
                f"could not reach OpenPolyPrint at {server}: {e}",
            )
        presets = collect_presets()
        payload = {
            "instanceId": instance_id(),
            "hostname": hostname(),
            "type": "presets",
            "name": "Presets — " + time.strftime("%Y-%m-%d %H:%M"),
            "presets": presets,
        }
        try:
            _api_request(cfg, "POST", "/api/orca/artifacts", payload, timeout=60)
        except Exception as e:
            return orca.ExecutionResult.failure(
                orca.PluginResult.RecoverableError,
                f"preset sync to {server} failed: {e}",
            )
        ai_note = (
            "AI review is available on the server"
            if ai_enabled
            else "AI review is DISABLED on the server (set a Gemini API key in OpenPolyPrint Settings)"
        )
        return orca.ExecutionResult.success(
            f"Synced preset bundle to {server}. {ai_note}."
        )


class GCodeSync(orca.slicing.SlicingPipelineCapabilityBase):
    """Uploads the exported G-code after each slice."""

    def get_name(self):
        return "G-code Sync"

    def get_default_config(self):
        return dict(DEFAULT_CONFIG)

    def on_load(self):
        # The heartbeat is managed by SyncNow; just refresh the config snapshot.
        global _CONFIG_SNAPSHOT
        _CONFIG_SNAPSHOT = _merged_config(json.loads(self.get_config() or "{}"))

    def execute(self, ctx):
        if ctx.step != orca.slicing.Step.psGCodePostProcess:
            return orca.ExecutionResult.success()
        global _CONFIG_SNAPSHOT
        cfg = _merged_config(json.loads(self.get_config() or "{}"))
        _CONFIG_SNAPSHOT = cfg
        if not cfg.get("auto_upload_gcode", True):
            return orca.ExecutionResult.skipped("auto upload disabled in plugin settings")
        path = getattr(ctx, "gcode_path", None)
        if not path:
            return orca.ExecutionResult.skipped("no G-code path in context")

        try:
            with open(path, "rb") as f:
                content = f.read()
        except PermissionError as e:
            return orca.ExecutionResult.failure(
                orca.PluginResult.RecoverableError,
                f"reading the G-code was blocked: {e}",
            )
        except Exception as e:
            return orca.ExecutionResult.failure(
                orca.PluginResult.RecoverableError,
                f"could not read {path}: {e}",
            )

        output_name = getattr(ctx, "output_name", None) or ""
        filename = os.path.basename(path) or (output_name + ".gcode") or "slice.gcode"
        name = output_name or os.path.splitext(filename)[0]
        fields = {
            "instanceId": instance_id(),
            "hostname": hostname(),
            "type": "gcode",
            "name": name,
        }
        try:
            _post_multipart(cfg, "/api/orca/artifacts", fields, "file", filename, content)
        except Exception as e:
            # Never block the user's export because the bridge is down.
            return orca.ExecutionResult.failure(
                orca.PluginResult.RecoverableError,
                f"upload to {_base_url(cfg)} failed: {e} "
                f"(G-code was still exported to {path})",
            )
        return orca.ExecutionResult.success(
            f"uploaded {filename} ({len(content) / 1e6:.1f} MB) to OpenPolyPrint"
        )


@orca.plugin
class OpenPolyPrintBridge(orca.base):
    def register_capabilities(self):
        orca.register_capability(SyncNow)
        orca.register_capability(GCodeSync)
