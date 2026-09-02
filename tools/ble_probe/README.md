# AnkerMake BLE Probe

PC-side research client for the AnkerMake M5/M5C Bluetooth provisioning protocol.
Goal: recover the three things static APK analysis couldn't reach (see
[`planning/apk-decompilation-findings.md`](../../planning/apk-decompilation-findings.md)):

1. **GATT service/characteristic UUIDs** (§1 — they live only in the app's encrypted native layer)
2. **Native byte framing** (§2g — how `CommandSend` becomes BLE writes)
3. **PIN/wifi credentials on-air encoding** (§3 — may be encrypted by the native layer)

## Setup

Windows PC with Bluetooth (works with the built-in adapter). bleak is required:

```powershell
py -3.10 -m pip install bleak     # bleak is installed for 3.10; 3.14 has no pip
```

## Usage

```powershell
cd C:\Users\Lucas\Documents\GitHub\OpenPolyPrint

# 1) Find the printer (highlights AnkerMake-looking names, shows manufacturer data)
py -3.10 tools\ble_probe\ankermake_ble.py scan -f anker
py -3.10 tools\ble_probe\ankermake_ble.py scan            # everything, sorted by RSSI

# 2) Dump its GATT table -> captures\gatt_dump.json  (this alone answers the UUID question
#    if the printer doesn't require bonding first)
py -3.10 tools\ble_probe\ankermake_ble.py enum <ADDRESS>

# 3) Live monitor: subscribe to every notify characteristic, then poke it
py -3.10 tools\ble_probe\ankermake_ble.py monitor <ADDRESS>
```

Inside `monitor`, interactive commands:

```
s  <uuid|handle> <hex>    write without response
sr <uuid|handle> <hex>    write with response
tlv <tag> <text>          build TLV bytes [tag][len][utf8] and print hex
frame <cmd> [hex]         build candidate [cmd][len][payload] frame from a known code
q                         quit
```

Known command codes (findings §2b) are preloaded: `frame set_factory` → `4b`,
`frame pin` + a TLV payload, etc. `frame wifi_connect`, `frame activate`, …

## What to try, in order

1. `scan` — note the printer's advertised name format and manufacturer-data bytes
   (the app matches devices by **BLE name**, findings §1).
2. `enum` — save the dump; note which characteristic is write vs notify
   (expect one write + one notify pair for the setup service).
3. `monitor` — subscribe, then try candidate frames on the write characteristic:
   - raw command: `frame set_factory` → `4b00`
   - TLV under a command: `frame pin 4141...`
   - with Modbus-style prefix (findings §2g, slave_id=2): `s <char> 02 4b00`
4. Watch notifications — every packet is auto-annotated: detected TLV tags (A0–AF),
   printable ASCII, and any known command codes seen in the data.
5. Record what works back into the findings doc (§2g native framing) — then the
   protocol is fully implementable outside the app.

If the printer ignores unknown clients, fall back to capturing the official app once
with an HCI snoop log and match frames against this table.
