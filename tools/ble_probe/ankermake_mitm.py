"""mitmproxy addon: capture AnkerMake/EufyMake API traffic.

Run with:
  mitmdump -s tools/ble_probe/ankermake_mitm.py
  (or: mitmweb -s tools/ble_probe/ankermake_mitm.py for the web UI)

Then configure your phone's WiFi proxy to point at this PC on port 8080,
install the mitmproxy CA cert on the phone, and open the EufyMake app.

This script logs:
- All requests to *.ankermake.com / *.eufymake.com
- Full headers (including the signed X-Signature headers)
- OTA API requests and responses (firmware download URLs)
- Any CDN/download URLs that look like firmware packages
"""

import json
import re
from datetime import datetime
from pathlib import Path
from mitmproxy import http

# Hosts we care about
ANKER_HOSTS = re.compile(r".*ankermake\.com$|.*eufymake\.com$|.*anker\.com$", re.I)
# CDN/firmware URL patterns
FIRMWARE_PATTERNS = re.compile(r"ota|firmware|rom|update|\.bin|\.img|\.tar|\.gz|\.zip", re.I)

LOG_FILE = Path("tools/ble_probe/captures/ankermake_api_capture.log")
LOG_FILE.parent.mkdir(parents=True, exist_ok=True)

# Collect all captured data for a summary at the end
captured_requests = []


def log(msg: str) -> None:
    ts = datetime.now().strftime("%H:%M:%S")
    line = f"[{ts}] {msg}"
    print(line)
    with open(LOG_FILE, "a", encoding="utf-8") as f:
        f.write(line + "\n")


def request(flow: http.HTTPFlow) -> None:
    """Log every request to AnkerMake/EufyMake servers."""
    host = flow.request.pretty_host
    if not ANKER_HOSTS.match(host):
        return

    log(f"\n{'='*72}")
    log(f"REQUEST {flow.request.method} {flow.request.pretty_url}")
    log(f"  Host: {host}")

    # Log all headers — the signed ones are what we need
    interesting_headers = [
        "X-Signature", "X-Key-Ident", "X-Request-Ts", "X-Request-Once",
        "X-Encoding", "Authorization", "app_version", "timezone",
        "platform", "language", "Content-Type", "User-Agent",
    ]
    for h in interesting_headers:
        val = flow.request.headers.get(h)
        if val:
            log(f"  {h}: {val}")

    # Log ALL headers for completeness
    log(f"  --- All headers ---")
    for k, v in flow.request.headers.items():
        if k not in interesting_headers:
            log(f"  {k}: {v}")

    # Log request body
    if flow.request.content:
        body = flow.request.content.decode("utf-8", errors="replace")
        log(f"  Body: {body[:2000]}")

    # Check if this is an OTA request
    if FIRMWARE_PATTERNS.search(flow.request.pretty_url):
        log(f"  *** OTA/FIRMWARE REQUEST DETECTED ***")

    captured_requests.append({
        "url": flow.request.pretty_url,
        "method": flow.request.method,
        "headers": dict(flow.request.headers),
        "body": flow.request.content.decode("utf-8", errors="replace") if flow.request.content else "",
    })


def response(flow: http.HTTPFlow) -> None:
    """Log responses from AnkerMake/EufyMake servers."""
    host = flow.request.pretty_host
    if not ANKER_HOSTS.match(host):
        return

    log(f"\nRESPONSE {flow.response.status_code} {flow.request.pretty_url}")

    # Log response body
    if flow.response.content:
        body = flow.response.content.decode("utf-8", errors="replace")
        log(f"  Body: {body[:5000]}")

        # Try to parse JSON and look for firmware URLs
        try:
            data = json.loads(body)
            # Recursively search for URLs in the JSON
            urls = find_urls_in_json(data)
            if urls:
                log(f"  *** URLs found in response JSON ***")
                for url in urls:
                    log(f"    {url}")
                    if FIRMWARE_PATTERNS.search(url):
                        log(f"    *** FIRMWARE URL: {url} ***")
        except (json.JSONDecodeError, TypeError):
            pass

    # Check for redirect to CDN
    location = flow.response.headers.get("Location")
    if location:
        log(f"  Redirect: {location}")
        if FIRMWARE_PATTERNS.search(location):
            log(f"  *** FIRMWARE REDIRECT: {location} ***")


def find_urls_in_json(obj, urls=None):
    """Recursively find all URL-like strings in a JSON structure."""
    if urls is None:
        urls = []
    if isinstance(obj, dict):
        for k, v in obj.items():
            find_urls_in_json(v, urls)
            # Key names that suggest firmware/download
            if k.lower() in ("url", "download_url", "file_url", "firmware_url",
                             "ota_url", "package_url", "bin_url", "rom_url"):
                if isinstance(v, str):
                    urls.append(f"{k}: {v}")
    elif isinstance(obj, list):
        for item in obj:
            find_urls_in_json(item, urls)
    elif isinstance(obj, str):
        # Look for URLs in string values
        if re.search(r"https?://", obj):
            if FIRMWARE_PATTERNS.search(obj) or ANKER_HOSTS.search(obj):
                urls.append(obj)
    return urls


def done():
    """Print summary when mitmproxy shuts down."""
    log(f"\n{'='*72}")
    log(f"CAPTURE COMPLETE — {len(captured_requests)} AnkerMake API requests captured")
    log(f"Log saved to: {LOG_FILE}")

    # Save all captured data as JSON
    json_file = LOG_FILE.with_suffix(".json")
    with open(json_file, "w", encoding="utf-8") as f:
        json.dump(captured_requests, f, indent=2, ensure_ascii=False)
    log(f"Full JSON data: {json_file}")
