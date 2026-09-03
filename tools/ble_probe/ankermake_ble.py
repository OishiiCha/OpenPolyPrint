#!/usr/bin/env python3
"""AnkerMake M5/M5C BLE research probe.

Companion tool for planning/apk-decompilation-findings.md.
Recovers the pieces static analysis couldn't: GATT UUIDs, characteristic roles,
and the native framing of the provisioning commands (codes 0x42-0x4B, TLV payloads).

Subcommands:
  scan    [-f FILTER] [-d SECONDS]     list nearby BLE devices, highlight AnkerMake ones
  enum    ADDRESS [--out FILE]         connect, dump all services/characteristics, save JSON
  monitor ADDRESS [options]            subscribe to notifications, interactive send/decode

Run with the Python that has bleak installed, e.g.:
  py -3.10 tools/ble_probe/ankermake_ble.py scan -f anker
"""

from __future__ import annotations

import argparse
import asyncio
import datetime as _dt
import json
import re
import string
import sys
from pathlib import Path

try:
    from bleak import BleakClient, BleakScanner
    from bleak.backends.characteristic import BleakGATTCharacteristic
    from bleak.backends.device import BLEDevice
    from bleak.backends.scanner import AdvertisementData
except ImportError:
    sys.exit("bleak not installed for this interpreter. Try: py -3.10 -m pip install bleak")

# Name fragments the AnkerMake app itself looks for (findings §1: scan filters by bleName).
NAME_HINTS = ("anker", "make", "m5", "m5c", "eufy", "t5216")

# Command codes recovered from the APK (findings §2b).
COMMANDS = {
    "wifi_list": 0x42,
    "wifi_connect": 0x43,
    "activate": 0x44,
    "control": 0x46,
    "pin": 0x48,
    "confirm": 0x4A,
    "set_factory": 0x4B,
}


def ts() -> str:
    return _dt.datetime.now().strftime("%H:%M:%S.%f")[:-3]


def hx(data: bytes) -> str:
    return data.hex()


def printable(data: bytes) -> str:
    out = "".join(c if 32 <= ord(c) < 127 else "." for c in data.decode("latin1"))
    return out.strip(".")


def looks_anker(name: str | None) -> bool:
    return bool(name) and any(h in name.lower() for h in NAME_HINTS)


# ---------------------------------------------------------------- scan -----

async def cmd_scan(args: argparse.Namespace) -> None:
    found: dict[str, tuple[BLEDevice, AdvertisementData]] = {}

    def on_detection(device: BLEDevice, adv: AdvertisementData) -> None:
        found[device.address] = (device, adv)

    print(f"[*] Scanning for {args.duration}s (ctrl-c to stop early)...")
    scanner = BleakScanner(detection_callback=on_detection)
    async with scanner:
        await asyncio.sleep(args.duration)

    rows = []
    for addr, (dev, adv) in found.items():
        name = adv.local_name or dev.name or ""
        if args.filter and args.filter.lower() not in name.lower():
            continue
        rows.append((looks_anker(name), adv.rssi if adv.rssi is not None else -999, addr, name, adv))

    rows.sort(key=lambda r: (not r[0], -r[1]))
    if not rows:
        print("[!] No matching devices. Try without --filter to see everything.")
        return

    print(f"\n{'ANKER?':<7}{'RSSI':>5}  {'ADDRESS':<20}NAME")
    print("-" * 78)
    for is_anker, rssi, addr, name, adv in rows:
        flag = "**" if is_anker else "  "
        print(f"{flag:<7}{rssi:>5}  {addr:<20}{name!r}")
        if adv.manufacturer_data:
            for mfr_id, mfr_bytes in adv.manufacturer_data.items():
                print(f"{'':13}mfr 0x{mfr_id:04x}: {hx(mfr_bytes)}")
        if adv.service_uuids:
            print(f"{'':13}svc: {', '.join(adv.service_uuids)}")
    print("\n[*] Use an address above with: enum <ADDRESS>   (Windows address format is fine as-is)")


# ---------------------------------------------------------------- enum -----

def char_props(c: BleakGATTCharacteristic) -> str:
    return ",".join(p.lower() for p in c.properties)


async def cmd_enum(args: argparse.Namespace) -> None:
    print(f"[*] Connecting to {args.address} (retrying twice, like the app)...")
    client = BleakClient(args.address)
    last_err = None
    for attempt in (1, 2, 3):
        try:
            await client.connect()
            break
        except Exception as e:  # noqa: BLE001 - report and retry, same as app's ×2 retry
            last_err = e
            print(f"[!] attempt {attempt} failed: {e}")
            await asyncio.sleep(1.5)
    else:
        sys.exit(f"[!] connect failed: {last_err}")

    print(f"[+] Connected. MTU (OS-reported): {client.mtu_size}")
    dump: dict = {"address": args.address, "timestamp": _dt.datetime.now().isoformat(),
                  "mtu": client.mtu_size, "services": []}

    interesting = []
    for service in client.services:
        print(f"\nSERVICE {service.uuid}  ({service.description})")
        svc = {"uuid": service.uuid, "description": service.description, "characteristics": []}
        for c in service.characteristics:
            props = char_props(c)
            print(f"  CHAR {c.uuid}  handle={c.handle:<4}[{props}]")
            svc["characteristics"].append(
                {"uuid": c.uuid, "handle": c.handle, "properties": props,
                 "descriptors": [{"uuid": d.uuid, "description": d.description} for d in c.descriptors]})
            if "write" in props or ("notify" in props) or "indicate" in props:
                interesting.append((c.uuid, props))
        dump["services"].append(svc)

    print("\n[*] Setup-relevant characteristics (write and/or notify):")
    for uuid, props in interesting:
        print(f"    {uuid}  [{props}]")

    out = Path(args.out)
    out.parent.mkdir(parents=True, exist_ok=True)
    out.write_text(json.dumps(dump, indent=2))
    print(f"\n[+] Saved GATT dump to {out}")
    print("[*] Compare against findings §1, then run: monitor <ADDRESS> --all-notify")
    await client.disconnect()


# --------------------------------------------------------------- probe -----

# Built-in probe sequence for framing discovery (findings §2g/§2b).
PROBE_FRAMES = [
    ("replay-heartbeat", bytes.fromhex("4d4119000501010546c001008462cc2fbb0900000000000025")),
    ("minimal-set_factory", bytes.fromhex("4d410900050101054b4e")),
    ("full-set_factory", bytes.fromhex("4d411900050101054bc0010000000000000000000000009f")),
    ("full-wifi_list", bytes.fromhex("4d4119000501010542c0010000000000000000000000008d")),
    ("raw-4b", bytes.fromhex("4b")),
    ("raw-02-4b-00", bytes.fromhex("024b00")),
]


def xor8(data: bytes) -> int:
    x = 0
    for b in data:
        x ^= b
    return x


def build_frame(cmd: int, payload: bytes = b"", flags: bytes = b"\xc0\x01\x00") -> bytes:
    """Build an MA frame: magic + len + 05010105 + cmd + flags + payload + pad + xor8."""
    body = b"\x4d\x41" + (0).to_bytes(2, "little") + b"\x05\x01\x01\x05" + bytes([cmd]) + flags + payload
    body = body[:2] + len(body).to_bytes(2, "little") + body[4:]
    return body + bytes([xor8(body)])


async def cmd_probe(args: argparse.Namespace) -> None:
    log_path = Path(args.log)
    log_path.parent.mkdir(parents=True, exist_ok=True)
    log_lines: list[str] = []

    def log(msg: str) -> None:
        print(msg)
        log_lines.append(msg)

    def on_notify(c: BleakGATTCharacteristic, data: bytearray) -> None:
        try:
            note = decode_guess(bytes(data))
        except Exception:  # noqa: BLE001
            note = ""
        log(f"[{ts()}] NOTIFY ({len(data)}B): {hx(data)}" + (f"  |  {note}" if note else ""))

    frames = [(n, bytes.fromhex(h)) for n, h in args.frames] if args.frames else PROBE_FRAMES
    gap = args.gap

    async with BleakClient(args.address) as client:
        log(f"[+] Connected {args.address}, MTU {client.mtu_size}")
        chars = [c for s in client.services for c in s.characteristics]
        write_chars = [c for c in chars if "write" in char_props(c)]
        target = write_chars[0] if write_chars else None
        if not target:
            sys.exit("[!] no writable characteristic")
        for c in chars:
            if "notify" in char_props(c) or "indicate" in char_props(c):
                await client.start_notify(c, on_notify)
        log(f"[*] write target {target.uuid}; listening {args.baseline}s for baseline heartbeat...")
        await asyncio.sleep(args.baseline)

        for name, frame in frames:
            log(f"\n=== PROBE {name}: {hx(frame)}  (xor8={'ok' if xor8(frame[:-1]) == frame[-1] else 'n/a'})")
            await client.write_gatt_char(target, frame, response=False)
            await asyncio.sleep(gap)

        log(f"\n[*] done; log saved to {log_path}")
    log_path.write_text("\n".join(log_lines) + "\n")


# ------------------------------------------------------------- monitor -----

TLV_RE = re.compile(rb"(?:( [\xa0-\xaf]) )", re.VERBOSE)


def decode_guess(data: bytes) -> str:
    """Best-effort annotation of a notification: TLV tags + printable runs."""
    notes = []
    tags = [(i, b) for i, b in enumerate(data) if 0xA0 <= b <= 0xAF and i + 1 < len(data)]
    if tags:
        parts = []
        for pos, tag in tags:
            ln = data[pos + 1]
            chunk = data[pos + 2 : pos + 2 + ln]
            if chunk:
                parts.append(f"{tag:02x}[{ln}]={printable(chunk) or hx(chunk)}")
        if parts:
            notes.append("tlv: " + " ".join(parts))
    txt = printable(data)
    if len(txt) >= 4:
        notes.append(f'ascii: "{txt}"')
    known = [f"cmd=0x{code:02x}({name})" for name, code in COMMANDS.items() if code in data]
    if known:
        notes.append("cmds-seen: " + ", ".join(known))
    return "  |  ".join(notes)


async def cmd_monitor(args: argparse.Namespace) -> None:
    char_filter = [u.lower() for u in args.chars.split(",")] if args.chars else None

    def on_notify(c: BleakGATTCharacteristic, data: bytearray) -> None:
        try:
            note = decode_guess(bytes(data))
        except Exception as e:  # noqa: BLE001 - never lose a packet to a decoder bug
            note = f"(decode error: {e})"
        print(f"[{ts()}] NOTIFY {c.uuid} ({len(data)}B): {hx(data)}" + (f"  {note}" if note else ""))

    async with BleakClient(args.address) as client:
        print(f"[+] Connected to {args.address}, MTU {client.mtu_size}")
        subscribed = []
        for service in client.services:
            for c in service.characteristics:
                props = char_props(c)
                if "notify" not in props and "indicate" not in props:
                    continue
                if char_filter and c.uuid.lower() not in char_filter and str(c.handle) not in char_filter:
                    continue
                try:
                    await client.start_notify(c, on_notify)
                    subscribed.append(c)
                    print(f"[*] subscribed {c.uuid} [{props}]")
                except Exception as e:  # noqa: BLE001
                    print(f"[!] could not subscribe {c.uuid}: {e}")
        if not subscribed:
            sys.exit("[!] No notify characteristics subscribed (bad --chars filter?)")

        write_targets = [c for s in client.services for c in s.characteristics if "write" in char_props(c)]
        print("\n[*] Write targets:")
        for c in write_targets:
            print(f"    {c.uuid}  handle={c.handle}  [{char_props(c)}]")

        help_text = """
  Commands (interactive):
    s <uuid|handle> <hex>      write WITHOUT response   (e.g. s 0x0025 4b)
    sr <uuid|handle> <hex>     write WITH response
    tlv <tag> <text>           build TLV bytes -> prints hex (paste into s/sr)
    frame <cmd-name|hexcode> [hex-payload]   build <cmd> <len> <payload> candidate frame
    q                          quit"""
        print(help_text if sys.stdin.isatty() else "[*] stdin not a tty; --send one-shot mode")

        if args.send:
            target, hexstr = args.send.split(":", 1)
            await do_write(client, resolve_char(write_targets, target), bytes.fromhex(hexstr), response=False)
            await asyncio.sleep(args.wait)
            return

        loop = asyncio.get_running_loop()
        while True:
            line = await loop.run_in_executor(None, input)
            line = line.strip()
            if not line:
                continue
            try:
                if line == "q":
                    break
                if line == "?":
                    print(help_text)
                elif line.startswith("tlv "):
                    _, tag_s, text = line.split(maxsplit=2)
                    tag = int(tag_s, 16)
                    payload = text.encode()
                    print(f"    -> {tag:02x}{len(payload):02x}{hx(payload)}")
                elif line.startswith("frame "):
                    parts = line.split()
                    code = COMMANDS.get(parts[1]) if not parts[1].startswith(("0x", "4")) else int(parts[1], 16)
                    if code is None:
                        print(f"    unknown cmd {parts[1]!r}; known: {', '.join(COMMANDS)}")
                        continue
                    payload = bytes.fromhex(parts[2]) if len(parts) > 2 else b""
                    frame = bytes([code, len(payload)]) + payload
                    print(f"    -> {hx(frame)}   (candidate: [cmd][len][tlv...])")
                elif line.startswith(("s ", "sr ")):
                    resp = line.startswith("sr")
                    _, target, hexstr = line.split(maxsplit=2)
                    c = resolve_char(write_targets, target)
                    await do_write(client, c, bytes.fromhex(hexstr), response=resp)
            except ValueError as e:
                print(f"[!] bad command: {e}  (type ? for help)")
            except Exception as e:  # noqa: BLE001
                print(f"[!] error: {e}")


def resolve_char(chars: list[BleakGATTCharacteristic], target: str) -> BleakGATTCharacteristic:
    t = target.lower()
    for c in chars:
        if c.uuid.lower() == t or str(c.handle) == t.lstrip("0x"):
            return c
    raise ValueError(f"no writable characteristic matches {target!r}")


async def do_write(client: BleakClient, c: BleakGATTCharacteristic, data: bytes, response: bool) -> None:
    print(f"[{ts()}] WRITE {'(resp)' if response else '(no-resp)'} {c.uuid}: {hx(data)}")
    await client.write_gatt_char(c, data, response=response)


# ---------------------------------------------------------------- main -----

def main() -> None:
    p = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    sub = p.add_subparsers(dest="cmd", required=True)

    ps = sub.add_parser("scan", help="scan for nearby BLE devices")
    ps.add_argument("-f", "--filter", default="", help="case-insensitive name substring (e.g. anker)")
    ps.add_argument("-d", "--duration", type=int, default=10, help="scan seconds (default 10)")
    ps.set_defaults(fn=cmd_scan)

    pe = sub.add_parser("enum", help="connect and dump GATT services/characteristics")
    pe.add_argument("address")
    pe.add_argument("--out", default=str(Path(__file__).parent / "captures" / "gatt_dump.json"))
    pe.set_defaults(fn=cmd_enum)

    pm = sub.add_parser("monitor", help="subscribe to notifications; interactive send/decode")
    pm.add_argument("address")
    pm.add_argument("--chars", default="", help="comma list of UUIDs/handles to subscribe (default: all notify/indicate)")
    pm.add_argument("--send", default="", help="one-shot: '<uuid|handle>:<hex>' then wait")
    pm.add_argument("--wait", type=float, default=5.0, help="seconds to listen in --send mode (default 5)")
    pm.set_defaults(fn=cmd_monitor)

    pp = sub.add_parser("probe", help="non-interactive: send built-in framing probes, log all notifications")
    pp.add_argument("address")
    pp.add_argument("--baseline", type=float, default=6.0, help="seconds to observe heartbeat before probing")
    pp.add_argument("--gap", type=float, default=6.0, help="seconds between probes (default 6, > heartbeat period)")
    pp.add_argument("--frames", default="", help="custom probes as 'name:hex,name:hex,...' (else built-in set)")
    pp.add_argument("--log", default=str(Path(__file__).parent / "captures" / "probe_session.log"))
    pp.set_defaults(fn=cmd_probe)

    args = p.parse_args()
    try:
        asyncio.run(args.fn(args))
    except KeyboardInterrupt:
        print("\n[*] interrupted")


if __name__ == "__main__":
    main()
