#!/usr/bin/env python3
"""Decode an Android btsnoop_hci.log into an AnkerMake BLE transcript.

Filters ATT traffic (L2CAP CID 0x0004), prints every write/notification with
its handle, and annotates frames that carry the AnkerMake "MA" protocol
(magic, len, cmd, XOR-8 check, TLV payloads, known command names).

Usage: python3 hci_decode.py btsnoop_hci.log [ > transcript.txt ]
Works on the bug-report-extracted FS/data/misc/bluetooth/logs/btsnoop_hci.log.
"""

from __future__ import annotations

import datetime as _dt
import struct
import sys

CMD_NAMES = {0x42: "wifi_list", 0x43: "wifi_connect", 0x44: "activate",
             0x46: "control", 0x48: "pin", 0x4A: "confirm", 0x4B: "set_factory"}

ATT_NAMES = {0x52: "WriteCmd", 0x12: "WriteReq", 0x13: "WriteRsp",
             0x1B: "Notify", 0x1D: "Indicate", 0x0A: "ReadReq", 0x0B: "ReadRsp",
             0x1E: "HandleValCfm", 0x05: "MTUReq", 0x06: "MTURsp"}

# btsnoop timestamps are microseconds since year 0 (63-epoch offset)
SNOOP_EPOCH = _dt.datetime(1, 1, 1) if False else _dt.datetime(1970, 1, 1)
USEC_YEAR0_TO_UNIX = 0x00E03AB2A2800000


def xor8(d: bytes) -> int:
    x = 0
    for b in d:
        x ^= b
    return x


def dec_ma(d: bytes) -> str:
    """Annotate an AnkerMake MA frame (findings §2g)."""
    if len(d) < 13 or d[:2] != b"MA":
        return ""
    flen = int.from_bytes(d[2:4], "little")
    cmd = d[8] if len(d) > 8 else 0
    name = CMD_NAMES.get(cmd, "?")
    ok = "xor-ok" if xor8(d[:-1]) == d[-1] else "xor-BAD"
    notes = [f"MA len={flen} cmd=0x{cmd:02x}({name}) flags={d[9:12].hex()} {ok}"]
    body = d[12:-1]
    # TLV scan: [tag 0xA0-0xAF][len][bytes]
    i, tlvs = 0, []
    while i + 1 < len(body):
        tag, ln = body[i], body[i + 1]
        if 0xA0 <= tag <= 0xAF and i + 2 + ln <= len(body):
            chunk = body[i + 2 : i + 2 + ln]
            txt = "".join(chr(c) if 32 <= c < 127 else "." for c in chunk)
            tlvs.append(f"{tag:02x}[{ln}]={chunk.hex()}{(' ' + repr(txt)) if txt.strip('.') else ''}")
            i += 2 + ln
        else:
            i += 1
    if tlvs:
        notes.append("tlv: " + " ".join(tlvs))
    else:
        notes.append(f"payload={body[:12].hex()}")
    return "  |  " + "  ".join(notes)


def parse(path: str):
    data = open(path, "rb").read()
    if data[:8] != b"btsnoop\x00":
        sys.exit("[!] not a btsnoop file")
    pos, n = 16, 0
    while pos + 24 <= len(data):
        orig_len, incl_len, flags, drops, ts = struct.unpack_from(">IIIIQ", data, pos)
        pos += 24
        pkt = data[pos : pos + incl_len]
        pos += incl_len
        n += 1
        when = _dt.datetime.utcfromtimestamp((ts - USEC_YEAR0_TO_UNIX) / 1e6)
        # H4 packet type byte
        if not pkt:
            continue
        ptype = pkt[0]
        if ptype != 0x02:  # ACL only
            continue
        handle_flags, acl_len = struct.unpack_from("<HH", pkt, 1)
        handle = handle_flags & 0x0FFF
        pb = (handle_flags >> 12) & 0x3
        acl = pkt[5 : 5 + acl_len]
        # reassembly for continuation fragments is not needed for small ATT PDUs;
        # skip continuation fragments (PB == 0x01)
        if pb == 0x01 or len(acl) < 4:
            continue
        l2_len, cid = struct.unpack_from("<HH", acl, 0)
        if cid != 0x0004:  # ATT
            continue
        att = acl[4 : 4 + l2_len]
        if not att:
            continue
        op = att[0]
        nm = ATT_NAMES.get(op, f"ATT-0x{op:02x}")
        if op in (0x52, 0x12):          # write cmd/req: handle + data
            if len(att) < 3:
                continue
            h = int.from_bytes(att[1:3], "little")
            payload = att[3:]
            direction = "APP->PRN"
            print(f"[{when.strftime('%H:%M:%S.%f')[:-3]}] {direction} handle=0x{h:04X} {nm:9s} {len(payload):3d}B {payload.hex()}{dec_ma(payload)}")
        elif op in (0x1B, 0x1D):        # notification/indication
            if len(att) < 3:
                continue
            h = int.from_bytes(att[1:3], "little")
            payload = att[3:]
            print(f"[{when.strftime('%H:%M:%S.%f')[:-3]}] PRN->APP handle=0x{h:04X} {nm:9s} {len(payload):3d}B {payload.hex()}{dec_ma(payload)}")
        elif op in (0x05, 0x06):        # MTU exchange - notable for provisioning
            print(f"[{when.strftime('%H:%M:%S.%f')[:-3]}] {'APP->PRN' if op==5 else 'PRN->APP'} {nm:9s} {att[1:].hex()}")
    print(f"\n[*] {n} snoop records processed", file=sys.stderr)


if __name__ == "__main__":
    if len(sys.argv) < 2:
        sys.exit(__doc__)
    parse(sys.argv[1])
