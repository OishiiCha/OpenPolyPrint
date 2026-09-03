#!/usr/bin/env python3
"""Test Tuya-style and other KDF patterns against known AnkerMake M5C tokens.

We have two known tokens:
1. Device-constant pre-handshake token (no ECDH, derived from static device data):
   9c46cc9dff7c323a1259c0bdc6fcd236

2. Session token (post-ECDH, derived from shared secret + maybe DSK):
   14d2e781aaa0150996f5cc34354e7fc9
   (shared secret: 3c058b30...92f1 — only partial, need full from Pi)

Known device data:
- DSK (old, stale): xdgfZrMLEVuRBTOs4aAi
- DSK (new, current): mc6FEc6X79zOzcPhL8GE
- Device SN: AK75D7D345000049
- WiFi MAC: E8EECC9FE857
- DUID: EUPRAKM-012822-SMKXB

The device-constant token was used BEFORE the ECDH handshake (0x40-error era),
so it must be derivable from static device data alone. If we crack this, we
learn the KDF pattern and can apply it to the session token.

Tuya BLE KDF pattern (from ESP-TuyaBLE source):
  session_key = MD5(local_key[:6] + srand[:6])

Tuya LAN v3.4 pattern:
  session_key = AES_ECB_128(XOR_bytes(client_random, gw_random), local_key)
"""

import hashlib
import base64
from Crypto.Cipher import AES

# ── Known values ──────────────────────────────────────────────────────

DEVICE_CONSTANT_TOKEN = bytes.fromhex("9c46cc9dff7c323a1259c0bdc6fcd236")
SESSION_TOKEN = bytes.fromhex("14d2e781aaa0150996f5cc34354e7fc9")

DSK_OLD = "xdgfZrMLEVuRBTOs4aAi"
DSK_NEW = "mc6FEc6X79zOzcPhL8GE"
SN = "AK75D7D345000049"
MAC = "E8EECC9FE857"
DUID = "EUPRAKM-012822-SMKXB"

# Partial shared secret (first 4 + last 2 bytes known)
# Full 32 bytes needed — run on Pi with maclient3+ to get full
SHARED_PARTIAL = "3c058b30"
SHARED_TAIL = "92f1"

# ── Helpers ───────────────────────────────────────────────────────────

def md5(data: bytes) -> bytes:
    return hashlib.md5(data).digest()

def sha256(data: bytes) -> bytes:
    return hashlib.sha256(data).digest()

def sha1(data: bytes) -> bytes:
    return hashlib.sha1(data).digest()

def aes_ecb_encrypt(key: bytes, data: bytes) -> bytes:
    cipher = AES.new(key[:16].ljust(16, b'\x00'), AES.MODE_ECB)
    return cipher.encrypt(data.ljust(16, b'\x00')[:16])

def aes_ecb_decrypt(key: bytes, data: bytes) -> bytes:
    cipher = AES.new(key[:16].ljust(16, b'\x00'), AES.MODE_ECB)
    return cipher.decrypt(data[:16])

def xor_bytes(a: bytes, b: bytes) -> bytes:
    return bytes(x ^ y for x, y in zip(a, b))

def try_decode_dsk(dsk: str) -> list[bytes]:
    """Try various interpretations of the DSK string."""
    results = []
    # Raw ASCII bytes
    results.append(("ascii", dsk.encode()))
    # Base64 decode (may need padding)
    try:
        padded = dsk + "=" * (4 - len(dsk) % 4) if len(dsk) % 4 else dsk
        results.append(("b64", base64.b64decode(padded)))
    except Exception:
        pass
    # Hex decode (if it were hex)
    try:
        results.append(("hex", bytes.fromhex(dsk)))
    except Exception:
        pass
    return results

# ── Test the device-constant token (no ECDH needed) ──────────────────

print("=" * 72)
print("PHASE 1: Device-constant token (pre-handshake, static device data only)")
print(f"  Target: {DEVICE_CONSTANT_TOKEN.hex()}")
print("=" * 72)

candidates = []

for dsk_name, dsk in [("DSK_OLD", DSK_OLD), ("DSK_NEW", DSK_NEW)]:
    for dsk_form, dsk_bytes in try_decode_dsk(dsk):
        candidates.extend([
            (f"md5({dsk_name}_{dsk_form})", md5(dsk_bytes)),
            (f"md5({dsk_name}_{dsk_form}+SN)", md5(dsk_bytes + SN.encode())),
            (f"md5(SN+{dsk_name}_{dsk_form})", md5(SN.encode() + dsk_bytes)),
            (f"md5({dsk_name}_{dsk_form}+MAC)", md5(dsk_bytes + MAC.encode())),
            (f"md5(MAC+{dsk_name}_{dsk_form})", md5(MAC.encode() + dsk_bytes)),
            (f"md5({dsk_name}_{dsk_form}+DUID)", md5(dsk_bytes + DUID.encode())),
            (f"md5(DUID+{dsk_name}_{dsk_form})", md5(DUID.encode() + dsk_bytes)),
            # Tuya BLE style: MD5(key[:6] + something[:6])
            (f"md5({dsk_name}_{dsk_form}[:6])", md5(dsk_bytes[:6])),
            (f"md5({dsk_name}_{dsk_form}[:6]+SN[:6])", md5(dsk_bytes[:6] + SN.encode()[:6])),
            (f"md5({dsk_name}_{dsk_form}[:6]+MAC[:6])", md5(dsk_bytes[:6] + MAC.encode()[:6])),
            # SHA256 truncated to 16
            (f"sha256({dsk_name}_{dsk_form})[:16]", sha256(dsk_bytes)[:16]),
            (f"sha256({dsk_name}_{dsk_form}+SN)[:16]", sha256(dsk_bytes + SN.encode())[:16]),
            (f"sha256(SN+{dsk_name}_{dsk_form})[:16]", sha256(SN.encode() + dsk_bytes)[:16]),
            # AES-ECB with DSK as key, encrypting known plaintext
            (f"aes_ecb({dsk_name}_{dsk_form},zeros)", aes_ecb_encrypt(dsk_bytes, b'\x00' * 16)),
            (f"aes_ecb({dsk_name}_{dsk_form},SN)", aes_ecb_encrypt(dsk_bytes, SN.encode())),
            (f"aes_ecb({dsk_name}_{dsk_form},MAC)", aes_ecb_encrypt(dsk_bytes, MAC.encode())),
        ])

# Also try with SN/MAC/DUID alone
candidates.extend([
    ("md5(SN)", md5(SN.encode())),
    ("md5(MAC)", md5(MAC.encode())),
    ("md5(SN+MAC)", md5(SN.encode() + MAC.encode())),
    ("md5(MAC+SN)", md5(MAC.encode() + SN.encode())),
    ("md5(DUID)", md5(DUID.encode())),
    ("md5(DUID+SN)", md5(DUID.encode() + SN.encode())),
    ("md5(SN+DUID)", md5(SN.encode() + DUID.encode())),
    ("md5(DUID+MAC)", md5(DUID.encode() + MAC.encode())),
    ("md5(MAC+DUID)", md5(MAC.encode() + DUID.encode())),
    # SN lower
    ("md5(SN_lower)", md5(SN.lower().encode())),
    ("md5(MAC_lower)", md5(MAC.lower().encode())),
    ("md5(SN_lower+MAC_lower)", md5(SN.lower().encode() + MAC.lower().encode())),
    # MAC as bytes
    ("md5(MAC_bytes)", md5(bytes.fromhex(MAC))),
    ("md5(SN+MAC_bytes)", md5(SN.encode() + bytes.fromhex(MAC))),
    ("md5(MAC_bytes+SN)", md5(bytes.fromhex(MAC) + SN.encode())),
    # Tuya-style with SN as "local_key"
    ("md5(SN[:6])", md5(SN.encode()[:6])),
    ("md5(SN[:6]+MAC[:6])", md5(SN.encode()[:6] + MAC.encode()[:6])),
    ("md5(MAC[:6]+SN[:6])", md5(MAC.encode()[:6] + SN.encode()[:6])),
    # SHA variants
    ("sha256(SN)[:16]", sha256(SN.encode())[:16]),
    ("sha256(MAC)[:16]", sha256(MAC.encode())[:16]),
    ("sha256(SN+MAC)[:16]", sha256(SN.encode() + MAC.encode())[:16]),
    ("sha256(MAC+SN)[:16]", sha256(MAC.encode() + SN.encode())[:16]),
    ("sha1(SN)[:16]", sha1(SN.encode())[:16]),
    ("sha1(MAC)[:16]", sha1(MAC.encode())[:16]),
    ("sha1(SN+MAC)[:16]", sha1(SN.encode() + MAC.encode())[:16]),
])

print(f"\nTesting {len(candidates)} candidates...\n")

matches = 0
for name, result in candidates:
    if result[:16] == DEVICE_CONSTANT_TOKEN:
        print(f"  *** MATCH! {name} = {result.hex()}")
        matches += 1
    elif result[:4] == DEVICE_CONSTANT_TOKEN[:4]:
        print(f"  [partial 4B] {name} = {result.hex()[:8]}...")

if matches == 0:
    print("  No matches found for device-constant token.")
    print("  The derivation may use data we don't have (e.g. a device-internal key).")
else:
    print(f"\n  {matches} match(es) found!")

# ── Test the session token (needs full shared secret) ─────────────────

print("\n" + "=" * 72)
print("PHASE 2: Session token (post-ECDH, needs full shared secret)")
print(f"  Target: {SESSION_TOKEN.hex()}")
print(f"  Shared secret: {SHARED_PARTIAL}...{SHARED_TAIL} (PARTIAL — need full 32B from Pi)")
print("=" * 72)

print("""
To test session-token derivations, run this script on the Pi where maclient3+
computed the full shared secret. Add the full 32-byte shared secret and
uncomment the session-key tests below.

Tuya-style patterns to try (once you have the full shared secret):
  key = md5(shared[:6] + dsk_bytes[:6])         # Tuya BLE style
  key = md5(shared[:6] + printer_nonce[:6])      # Tuya BLE with device nonce
  key = md5(shared[:16])                          # Simple MD5 of first half
  key = md5(shared)                               # MD5 of full secret
  key = sha256(shared)[:16]                       # SHA256 truncated
  key = aes_ecb(shared[:16], dsk_padded)          # Tuya LAN v3.4 style
  key = aes_ecb(dsk[:16], shared[:16])            # Reversed
  key = md5(shared + dsk_bytes)                   # Secret + DSK
  key = md5(dsk_bytes + shared)                   # DSK + secret
  key = md5(shared[:6] + sn[:6])                  # Secret prefix + SN prefix
  key = md5(shared[:6] + mac_bytes[:6])           # Secret prefix + MAC prefix

Also compare the 252B printer reply to 0x45 — it may contain a 6-byte
srand/nonce (like Tuya's device info response) that feeds into the KDF.
Look for bytes in the reply that aren't part of the ECDH pubkey or the
A3/A4/A5 TLV metadata.
""")

# ── Also check: is the device-constant token related to the DSK via AES? ─

print("=" * 72)
print("PHASE 3: AES relationship between DSK and device-constant token")
print("=" * 72)

for dsk_name, dsk in [("DSK_OLD", DSK_OLD), ("DSK_NEW", DSK_NEW)]:
    for dsk_form, dsk_bytes in try_decode_dsk(dsk):
        if len(dsk_bytes) < 16:
            dsk_key = dsk_bytes.ljust(16, b'\x00')
        else:
            dsk_key = dsk_bytes[:16]

        # If token = AES_ECB(DSK, plaintext), what's the plaintext?
        pt = aes_ecb_decrypt(dsk_key, DEVICE_CONSTANT_TOKEN)
        print(f"  aes_ecb_decrypt({dsk_name}_{dsk_form}, token) = {pt.hex()}")
        try:
            txt = pt.decode('ascii', errors='replace')
            if any(c.isalpha() for c in txt):
                print(f"    ascii: {txt!r}")
        except Exception:
            pass

        # If token = AES_ECB(plaintext, DSK), what key would produce it?
        # Can't reverse this way, but check if encrypting known data with DSK gives token
        for pt_name, pt_data in [("zeros", b'\x00'*16), ("SN", SN.encode()), ("MAC", MAC.encode())]:
            ct = aes_ecb_encrypt(dsk_key, pt_data)
            if ct == DEVICE_CONSTANT_TOKEN:
                print(f"  *** MATCH! aes_ecb({dsk_name}_{dsk_form}, {pt_name}) = token!")

print("\nDone. If no matches, the device-constant token likely uses a")
print("device-internal key not available in cloud config (e.g. junzheng")
print("module's factory-flashed secret). In that case, the SPI flash dump")
print("or Ijiami dex dump is the only path to the KDF.")
