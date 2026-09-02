# EufyMake APK Decompilation — Findings Log

## Decompilation Setup

- **APK source**: APKMirror — `com.oceanwing.FDMPrint_v4.2.0_9393-9393_2arch_24lang_1feat_97c4582cf7b1cd63dbb98adf96cb5745_apkmirror.com.apkm` (stored at `\\wsl.localhost\kali-linux\home\lucas\apk\`, unpacked to `~/apk/decomp/` in Kali)
- **APK version**: v4.2.0 (build 9393)
- **APK hash (SHA256)**: [not computed yet — run `sha256sum ~/apk/*.apkm` in Kali]
- **Tools used**:
  - [x] jadx (Java decompiler — stub only; Debian wrapper requires absolute paths)
  - [x] unzip (split `.apkm` bundle → base.apk + splits)
  - [x] strings (Dart string/symbol pool mining of `libapp.so`)
  - [x] **blutter with `--no-analysis`** — ✅ WORKS (full-analysis build segfaults; no-analysis mode dumps every class/method + addresses + object pool + a ready-to-use frida script)
  - [x] capstone (manual AOT disassembly of key functions with pool-reference resolution)
  - [ ] frida (runtime hooking) — `blutter_frida.js` is pre-generated and ready
  - [ ] BlackDex / FRIDA-DEXDump — to unpack Ijiami (native layer)

### Key structural finding (changes the whole approach)

**This is NOT a normal Java app.** It is:

1. **Ijiami-packed Android shell** — `base.apk` contains only a 47KB stub `classes.dex`. The real Java/Kotlin code is encrypted in `assets/ijiami.dat` (9.5MB) + `assets/ijiami.ajm` (2.9MB) and decrypted at runtime by `assets/ijm_lib/*/libexec.so` (`ijiami SecLLVM compiler 1.7.4.20`).
2. **The real app is Flutter** — Dart 3.5.4 (stable), snapshot `80a49c7111088100a233b2ae788e1f48`, target android arm64, compressed pointers. All setup/pairing logic lives in `libapp.so` (31MB, from `split_config.arm64_v8a.apk`). Note: the armeabi-v7a split exists but **blutter does not support arm32**.
3. Java side is a bridge only: Pigeon-style channels under `com.zhixin.flutter_fdmprint_module.*`.

### Quick start commands (what actually worked)
```bash
# In Kali WSL
cd ~/apk/decomp
unzip ../*.apkm                                   # split bundle
unzip split_config.arm64_v8a.apk 'lib/arm64-v8a/libapp.so' 'lib/arm64-v8a/libflutter.so' -d flat

# blutter — MUST use --no-analysis (full analysis segfaults on this snapshot)
# deps: jadx unzip binutils cmake ninja-build libicu-dev libcapstone-dev python3-capstone
#       pip: pyelftools requests bs4 tqdm   | blutter cloned to /opt/blutter
python3 /opt/blutter/blutter.py ~/apk/decomp/flat ~/apk/decomp/blutter_out --no-analysis
# → asm/ (every class+method+address), pp.txt (object pool: strings/constants/enums),
#   objs.txt (class list), blutter_frida.js (hook script with all offsets)

# Example: find BLE code
grep -rn "notifyNative\|sendCommand" blutter_out/asm/fdm_base/native_channel -r

# Dart version
strings flat/libflutter.so | grep -E '\(stable\)'   # → 3.5.4
```

Artifacts (Kali): `~/apk/decomp/blutter_out/` (asm/, pp.txt 15MB, objs.txt, blutter_frida.js), `~/apk/decomp/libapp_strings.txt`, `~/apk/decomp/flat/`, `~/apk/decomp/out_res/` (jadx incl. resources/manifest).

---

## 1. BLE Service & Characteristic UUIDs

> **CORRECTION (previously wrong lead):** the three UUID-looking strings found in the string pool
> (`3b04351f-130b-4ed8-a45d-24bd9d310b60`, `fc7da1cf-8d3c-4d71-af0f-644766f4bbb3`,
> `cfde7cb9-acd9-42ed-a581-1c22db1aa764`) are **NOT GATT UUIDs**.
> Code-reference scanning (capstone over all of `.text`) shows they are used only in
> `devOnline`/`devOffline` of the MQTT message receiver as hardcoded **`message_id` values**
> inside a synthetic MQTT payload (adjacent keys: entity/station/feature/device_info/message_id/station_sn/timestamp).

### Findings
- [x] GATT UUID location: **in the encrypted native (Ijiami dex) layer** — the Dart side never touches UUIDs; it calls named native methods (see §2) that do the GATT work
- [ ] Actual service/characteristic UUIDs: unknown → need runtime dump (frida hook on `BluetoothGattService`/`addService`, or BlackDex-unpack the native dex, or HCI snoop capture)
- [x] Advertising/scan filtering: by **BLE name** (`bleName`/`devBleName`), plus `bleType`, `bleAddress`, `bleMacFormat`; scan callback `SearchDeviceCallbackApi.onFindDevice` carries `scanRecord`

---

## 2. GATT Communication Protocol

> Fully reconstructed at the Dart↔native boundary. The Dart side is orchestration only:
> it calls **named native methods with argument maps**; the native (encrypted) side owns
> GATT + byte framing.

### 2a. Native method vocabulary (exact, from disassembly + pool resolution)

Called via `PageEventHelper.notifyNative(method, arguments: {...})`:

| Native method | Arguments | Returns | Purpose |
|---|---|---|---|
| `devSearch` | `{"bindInfo": ...}` | void (events) | start scan/bind session |
| `scanStop` | — | void | stop scan |
| `bleDisconnect` | `{"bleName": String}` | void | drop BLE link |
| `devBleReConnect` | `{"bleName", "changeWifi", "needWifiList"}` | void | reconnect (change-WiFi path) |
| `bleGetWifis` | `{"isChangeWifi": bool}` | `List<Map<String,dynamic>>` | **printer-side Wi-Fi scan** |
| `wifiConnection` | `{"wifiInfo", "isChangeWifi", "mainSn"}` | WifiManagerState | send credentials & connect |
| `wifiBind` | `{"wifiInfo", "mainSn"}` | — | bind after connect |
| `wifiConnectinAndBind` | `{"needBind": bool, "wifiInfo"}` | bool | combined connect+bind |
| `bindActive` | (sn) | bool (compared to `"true"`) | cloud activation/bind |
| `bleGet0x0103Bytes` | `{"slave_id": 2, ...}` | `List<Uint8List>` | build 0x0103 frame(s) |
| `bleGet0x0105Bytes` | `{"slave_id": 2, "bleMTU": int}` | `List<Uint8List>` | build 0x0105 frame (MTU) |
| `bleGet0x0233Bytes` | `{"slave_id": 2}` | `List<Uint8List>` | build 0x0233 frame |
| `mqttGet0x1029Bytes` | `{"sn", "cmd": 1029, "value"}` | `List<Uint8List>` | build MQTT 0x1029 frame |
| `checkUsePinCode` / `devBleIsUsePinCode` | — | bool | whether this device needs PIN |
| `deviceStatusCheck`, `bleOpen`, `wifiEnableStatus`, `uploadBleLog`, `bigButtonVer` | — | — | aux |

Command channel (separate from above): `ChannelManager.instance.deviceSendFunctionApi.sendCommand(map)` →
Pigeon channel **`com.zhixin.flutter_fdmprint_module.DeviceSendFunctionApi.sendCommand`** returning a
`CommandReply`. The command bean's JSON keys (found in pool as a fromJson cluster):
**`commandCode`, `mtuSize`, `dataMap`, `command`, `bleBytes`, `scanRecord`, `sendType`, `keepCmdSending`, `sendingStatusRollbackTime`**.

### 2b. State machines (codes from enum object dumps in pp.txt)

**WifiManagerState** (returned by `wifiConnection`):
| code | name |
|---|---|
| 0 | succ |
| 1 | fail |
| 2 | timeOut |
| 3 | scanWifiFail |
| 4 | connect-fail variant (see note) |
| 5 | wifiPwError — **wrong password** |
| 6 | getIpFail |
| 7 | deviceCommunicateFail |

(4/5 exact naming order between "wifiConnectOtherFail"/"wifiPwError" is tentative; the wrong-password state is confirmed distinct.)

**BCBLEManagerState** (BLE adapter/connection, 17 values incl.): 0 unknown, 1 unsupported, 2 unauthorized, 3 poweredOff, 4 poweredOn, 7 scanErr, 8 scanTimeout, 0xb connectTimeout, 0xe disConnectErr.
**BLESendState**: 0 timeOutError, 3 otherError (4 values).

### 2c. Provisioning Flow (page-level, confirmed from asm)

```
1.  Permissions (bleGetPermissionStatus / blePermission)
2.  devSearch {bindInfo}            → scan; onFindDevice carries bleName/scanRecord
3.  connectDevice (BluetoothConnectApi, retried ×2; then MTU via bleGet0x0105Bytes {"slave_id":2,"bleMTU":N})
4.  FACTORY STATE: "connectDevice success, but isFactory so do commandSetFactory"
      commandSetFactory  — impl: lib_device/device_trash/add_device/add_device_scan_qr_page.dart
      sends via DeviceSendFunctionApi.sendCommand; success → route "nativePageTestProduct"
5.  commandSendActivate / commandSendPinCode
      PIN UI: lib_device/device_trash/ble_test/input_pin_page.dart (CommandReply callback)
      checkUsePinCode gates whether PIN needed
6.  commandDeviceConfirm — impl: add_device_action_confirm_page.dart ("press button to confirm")
7.  bleGetWifis {"isChangeWifi":false} → List of wifi maps (ssid/rssi/auth...)
8.  User picks SSID + password → wifiInfo = {"ssid", "auth"/"encryptionType", "pwd"}
9.  wifiConnection {wifiInfo, isChangeWifi, mainSn} → WifiManagerState (5 = wrong password)
10. wifiBind {wifiInfo, mainSn} / bindActive → cloud bind (result compared to "true")
11. MQTT listener (_setupMqttListener); P2P optional (AKP2PManager)
```

Production-test flow exists separately: `lib_device/device_trash/ble_test/`
(`ble_production_test_page`, `connect_wifi_page`, `input_pin_page`, `input_device_name_page`, `select_wifi_page`).

### Message Format
- [x] Dart↔native: named methods + maps (documented above)
- [x] Frame builders: **native-side** — Dart asks native for `List<Uint8List>` frames for cmd 0x0103/0x0105/0x0233/0x1029. `slave_id = 2` constant ⇒ register/Modbus-style addressing. **The encoder itself is in the encrypted dex.**
- [ ] Raw BLE byte layout (header/seq/checksum/encryption): unknown — dump native dex or capture

### 2d. Command Codes (CRACKED — from ble_command_utils.dart disassembly)

> Each BLE command references a distinct `Byte` constant object in the Dart pool.
> Disassembly of `commandSetFactory`, `commandSendWifiConnect`, `commandSendActivate`,
> `commandSendPinCode`, `commandSendWifiList`, `commandDeviceConfirm`, `bleControl`
> revealed the command code each one loads.

| Command | Code (hex) | Code (dec) | Dart function |
|---------|-----------|-----------|---------------|
| WifiList | `0x42` | 66 | `commandSendWifiList` @0x1828c80 |
| WifiConnect | `0x43` | 67 | `commandSendWifiConnect` @0x18244bc |
| Activate | `0x44` | 68 | `commandSendActivate` @0x182603c |
| bleControl | `0x46` | 70 | `bleControl` @0xfbf1dc |
| PinCode | `0x48` | 72 | `commandSendPinCode` @0x18279d0 |
| DeviceConfirm | `0x4A` | 74 | `commandDeviceConfirm` @0x19d8804 |
| SetFactory | `0x4B` | 75 | `commandSetFactory` @0x170dc2c |

### 2e. Payload Builders (from ble_command_utils.dart)

> Found via caller scan: built a function index from blutter's asm files, mapped
> each call site to its containing function. `ble_command_utils.dart` contains
> `_setSendPinCodeData` and `_setRequestWifiData` — the exact payload builders.

| Function | Address | Size | Purpose |
|----------|---------|------|---------|
| `_setSendPinCodeData` | 0x1827a74 | 0x21c | Builds PIN code payload (fields → bytes) |
| `_setRequestWifiData` | 0x1828d2c | 0xd4 | Builds Wi-Fi request payload |
| `_setActivateData` | 0x18260e0 | 0x9a0 | Builds activate payload (largest — most fields) |
| `stringToBytes` | 0x182567c | 0x200 | Core encoder: converts string fields to UTF-8 byte lists |
| `commandSetFactory` | 0x170dc2c | 0xb8 | Sends 0x4B |
| `commandSendWifiConnect` | 0x18244bc | 0xa4 | Sends 0x43 |
| `commandSendActivate` | 0x182603c | 0xa4 | Sends 0x44 |
| `commandSendPinCode` | 0x18279d0 | 0xa4 | Sends 0x48 |
| `commandSendWifiList` | 0x1828c80 | 0xac | Sends 0x42 |
| `commandDeviceConfirm` | 0x19d8804 | 0xb8 | Sends 0x4A |
| `bleControl` | 0xfbf1dc | 0xa8 | Sends 0x46 |
| `commonMtuSize` | 0xfbf084 | 0x4c | MTU size getter |

**Payload encoding**: Fields are converted via `stringToBytes()` → UTF-8 byte lists.
The core encoder at 0xfbf720 handles length-prefix/padding rules (details still being
disassembled).

### 2f. Wire Format Objects (decoded from Dart pool)

> Disassembled `CommandSend.ctor`, `CommandSend.decode`, `BleDevice.decode`,
> `connectDevice`, `CommandReply.decode` — these contain every field name on the wire.

**CommandSend** (the wire object sent to native):
- Constructor @0xfbf284 (0x2b4 bytes)
- Decode @0x13f1d70 (0x4d8 bytes)
- Fields extracted from pool: `commandCode`, `mtuSize`, `dataMap`, `command`, `bleBytes`, `scanRecord`, `sendType`, `keepCmdSending`, `sendingStatusRollbackTime`

**CommandReply** (response from native/printer):
- Decode @0x13f0ed8 (0x2d8 bytes)
- Contains: command code echo, status, data fields

**BleDevice** (discovered device):
- Decode @0x13f12e4 (0x3b8 bytes)
- Fields: `bleName`, `bleAddress`, `bleMacFormat`, `bleType`, `scanRecord`, `devType`, `rssi`

**connectDevice** @0x170d4dc (0x10c bytes):
- Takes bleName, connects via native, retried ×2 on failure

---

## 3. Encryption / Key Exchange

- [ ] BLE payload encryption: unknown (native side). PIN code is verified during bind (`commandSendPinCode` + `checkUsePinCode`); transport unknown.
- [x] Ijiami `libijmDataEncryption*.so` encrypts the app dex (not necessarily BLE traffic).
- [x] Payload field encoding: **UTF-8 via `stringToBytes()`** — confirmed by disassembling the core encoder at 0xfbf720 (calls `toUtf8`)
- [x] Command codes are single-byte (0x42-0x4B) — no encryption on the command code itself
- [ ] Whether the *data payload* (SSID, password, PIN) is encrypted before the UTF-8 conversion or after: need full `_setSendPinCodeData` / `_setActivateData` body analysis
- [ ] Anker P2P layer AES: not reached (post-provisioning).
- Hooks ready: `javax.crypto.Cipher` + GATT write via frida once app runs.

---

## 4. Device Information Exchange

- [x] Model detection via `bleName`; **T5216 = M5C** (`uv_t5216/*` package tree); M5 separate DeviceCtrl classes
- [x] SN exchanged during bind (`sn`, `devSN`, `mainSn`, `station_sn` keys; `findT5216AccessoryBySN`)
- [x] Scan result carries `scanRecord` + `ble_rssi`; `deviceInfo`/`devType`/`bleType` keys exist
- [ ] SN/DUID byte formats: inside 0x0103/0x0105 responses (native)

---

## 5. Wi-Fi Configuration Protocol

- [x] `wifiInfo` map = **`{"ssid": ..., "auth"/"encryptionType": ..., "pwd": ...}`** (keys confirmed in pool adjacent to the wifi native methods: `"auth"`, `"pwd"`, `"ssid"`; `"encryptionType"` in scan-result model)
- [x] Security: **WPA/WPA2 only** (explicit "printer doesn't support WPA3" in 10+ languages)
- [x] Printer-side scan (`bleGetWifis`) — phone never scans for the printer's sake; result entries include `ssid`/`rssi`/auth type (`wifi_ssid`, `wifi_mac` keys exist in device-info models)
- [x] Wrong-password reporting: WifiManagerState code 5 (`wifiPwError`) / UI key `fdm_bind_wificonnect_pwderr`
- [ ] Byte-level encoding of wifiInfo into the frame: native side

---

## 6. PPPP / LAN Transition

- [x] `AKP2PManager` / `AKBindP2PConnModel` in Dart (`ak/nb/p2p`, `app/get_p2p_conn`); access-mode log `"changed access mode from P2P wired to BLE wired (mode 2 -> 1)"` ⇒ BLE is a wired-equivalent transport for the P2P stack
- [x] BLE↔P2P bridge commands: `p2pSendBleCmd0x0103/0x0105/0x1527`
- [ ] DUID source / P2P keys: unknown (likely from cloud bind step)

---

## 7. Cloud / MQTT Registration

- [x] `bindActive` result compared against string `"true"`; `sendMqttCommand` channel exists (`DeviceSendFunctionApi.sendMqttCommand`)
- [x] MQTT device-status JSON: `{"commandType":1000/1068/1085/1192/1601,...}` with error codes `0xFB…/0xFD…`
- [x] `mqtt_message_active_receiver.dart`: devOnline/devOffline synthesize payloads with hardcoded message_id UUIDs (see §1 correction)
- [ ] Full REST bind sequence: not mapped

---

## 8. Native Libraries (.so files)

| File | Size | Purpose | Reversed? |
|------|------|---------|-----------|
| libapp.so | 31MB | Flutter Dart AOT — all app logic | ✅ blutter --no-analysis + capstone |
| libflutter.so | 10MB | Flutter engine 3.5.4 | version only |
| libakai.so | 10MB | OpenCV vision | identified, not BLE |
| assets/ijm_lib/libexec.so | 0.8MB | Ijiami packer runtime | no |
| assets/libijmDataEncryption*.so | 0.4MB | dex encryption | no |

**The BLE GATT + frame encoder lives in the Ijiami-encrypted dex (ijiami.dat)** — the single highest-value remaining target.

---

## 9. Class Map (Dart, from blutter)

| Dart file | Purpose |
|---|---|
| `fdm_base/communication/ble/ak_ble_manager.dart` | AKBleManager: connect/disconnect, get0x0103/0x0105/0x0233/1029Bytes, _dataFormat, state listeners |
| `fdm_base/communication/ble/ak_ble_enum.dart` | WifiManagerState / BCBLEManagerState / BLESendState enums (+fromCode) |
| `fdm_base/communication/ble/ble_command_utils.dart` | **Command payload builders**: _setSendPinCodeData, _setRequestWifiData, _setActivateData, stringToBytes, all command* functions |
| `fdm_base/native_channel/channel/flutter_channel_device_send_fun.dart` | DeviceSendFunctionApi: sendCommand / sendMqttCommand / getCMDInfo |
| `fdm_base/native_channel/fdmchannel/fdm_page_event_helper.dart` | notifyNative + all bind/wifi/scan native calls |
| `lib_device/device_trash/add_device/add_device_scan_qr_page.dart` | commandSetFactory (from-factory entry) |
| `lib_device/device_trash/add_device/add_device_action_confirm_page.dart` | commandDeviceConfirm → goWiFiPage |
| `lib_device/device_trash/ble_test/*` | production-test flow incl. PIN + wifi pages |
| `lib_device_uv_bind/bind/*` | bind UI + models (ak_find_dev_view_model, ak_p2p_manager, ak_ble_error_listener, wifi_security_type) |
| `uv_t5216/*` | M5C-specific controllers/notifiers |

Key function addresses (arm64 libapp.so, for frida/Ghidra):
```
AKBleManager.get0x0105Bytes  0xff99b0      AKBleManager.get0x0103Bytes  0xffa054
AKBleManager._dataFormat     0xff9b00      AKBleManager.get0x0233Bytes  0x18f8cec
AKBleManager.get1029Bytes    0x18e0eb0     AKBleManager.handleNativeCall 0xff427c
commandSetFactory            0x170dadc     commandDeviceConfirm        0x19d86cc
DeviceSendFunctionApi.sendCommand 0xfbf0d0 / sendMqttCommand 0xfbeb30
PageEventHelper.notifyNative 0xf05174      wifiConnection 0x18ffa84   wifiBind 0x18ff4a4
bleGetWifis 0x1900528        bindActive 0x18f7fec   devBleReConnect 0xff3bfc   devSearch 0xffbc50
```
(Complete map in `blutter_out/asm/**`; hooking scaffold in `blutter_out/blutter_frida.js`.)

---

## 10. Raw Captures

- none yet. **Recommended**: HCI snoop during real setup; correlate writes with the native methods above (each notifyNative → one or more GATT writes). Filter `btatt`.

---

## 11. Open Questions

- [ ] GATT service/characteristic UUIDs (in native dex — BlackDex/FRIDA-DEXDump or runtime hook)
- [ ] Byte layout of 0x0103/0x0105/0x0233/0x1527 frames (native `bleGet0xNNNNBytes` implementations)
- [ ] What `_dataFormat` appends to the 0x0103 map (parses an argument string into extra k/v pairs)
- [ ] Confirm exact name↔code for WifiManagerState 4/5
- [ ] PIN: source (device sticker/QR/display?) and whether hashed in transit
- [x] ~~Why full-analysis blutter segfaults~~ — **diagnosed**: crash in blutter's prologue analyzer (`handleArgumentsDescriptorTypeArguments`); fixable upstream but manual capstone pipeline covers targeted functions
- [ ] Ijiami anti-frida/anti-root checks (test on rooted device)
- [ ] Full body of `_setActivateData` (0x9a0 bytes — largest payload builder, most fields)
- [ ] Length-prefix/padding rules in core encoder 0xfbf720
- [ ] Which fields go into each command payload (final detail pass in progress)

---

## 12. Progress Tracker

| Task | Status | Notes |
|------|--------|-------|
| Unpack .apkm / identify architecture | done | Flutter 3.5.4 + Ijiami shell |
| String pool extraction | done | `libapp_strings.txt` |
| **blutter decompile** | **done (--no-analysis)** | asm/ + pp.txt + objs.txt + frida script |
| blutter full-analysis crash | **diagnosed** | SIGSEGV in `handleArgumentsDescriptorTypeArguments`; gdb backtrace captured |
| Manual AOT disasm of key funcs | done | frame builders, command senders, enums |
| Native method vocabulary + args | **done** | see §2a table |
| State-machine codes | done | §2b |
| wifiInfo payload keys | done | ssid/auth/pwd |
| UUID candidates | **eliminated (wrong)** | they are MQTT message_ids; real UUIDs in native dex |
| **Command codes cracked** | **done** | §2d — SetFactory=0x4B, WifiConnect=0x43, Activate=0x44, bleControl=0x46, PinCode=0x48, DeviceConfirm=0x4A, WifiList=0x42 |
| **Payload builders located** | **done** | §2e — ble_command_utils.dart: _setSendPinCodeData, _setRequestWifiData, _setActivateData, stringToBytes |
| **Wire format objects decoded** | **done** | §2f — CommandSend, CommandReply, BleDevice field names extracted |
| **Payload encoding confirmed** | **done** | UTF-8 via stringToBytes() → toUtf8 |
| Caller scan (who sends which frame) | **done** | function index built from asm, call sites mapped |
| Full payload field mapping | **in progress** | disassembling _setSendPinCodeData, _setRequestWifiData, _setActivateData bodies |
| GATT UUIDs / frame bytes | pending | need Ijiami dex dump (BlackDex) or runtime hook |
| Live BLE capture | pending | |
| Implement in Go | pending | |
| Test pairing without app | pending | |

---

## 13. Next Steps (Prioritized)

### Step 1: Finish payload field mapping (IN PROGRESS)
**What**: Complete the disassembly of `_setSendPinCodeData` (0x21c bytes), `_setRequestWifiData` (0xd4 bytes), and `_setActivateData` (0x9a0 bytes) to extract exactly which string fields go into each command payload.

**How**: Continue the capstone disassembly pipeline with pool resolution:
```bash
# In Kali WSL — full raw disassembly of the three payload builders
wsl -d kali-linux -e bash -c "python3 - <<'EOF'
from capstone import *
import re
so = open('/home/lucas/apk/decomp/flat/libapp.so','rb').read()
md = Cs(CS_ARCH_ARM64, CS_MODE_LITTLE_ENDIAN)
pool = {}
for line in open('/home/lucas/apk/decomp/blutter_out/pp.txt'):
    m = re.match(r'\[pp\+0x([0-9a-f]+)\] (.*)', line.rstrip())
    if m: pool[int(m.group(1),16)] = m.group(2)[:140]
for name,start,size in [('_setSendPinCodeData',0x1827a74,0x21c),
                         ('_setRequestWifiData',0x1828d2c,0xd4),
                         ('_setActivateData',0x18260e0,0x9a0)]:
    print('='*14, name)
    regs={}
    for i in md.disasm(so[start:start+size], start):
        s=f'{i.mnemonic} {i.op_str}'
        # ... (same pool-resolution pattern as before)
EOF"
```
**Goal**: For each command, know the exact list of fields and their order in the payload.

### Step 2: Dump the Ijiami-encrypted native dex
**What**: Extract the real Java/Kotlin dex from the Ijiami packer to find GATT UUIDs and the native frame encoder.

**How (option A — BlackDex)**:
```bash
# On a rooted Android device with BlackDex installed
# BlackDex hooks the Ijiami libexec.so and dumps the decrypted dex
# Output: /sdcard/Android/data/com.googlecode.android.bb.blackdex/...
```

**How (option B — FRIDA-DEXDump)**:
```bash
# With frida server running on device
pip install frida-tools
frida-dexdump -U -f com.oceanwing.FDMPrint
# Dumps all dex files at runtime, including the Ijiami-decrypted one
```

**How (option C — static probe)**:
```bash
# Probe ijiami.dat statically — it may be XOR/AES encrypted
# Check libexec.so for the decryption routine
strings assets/ijm_lib/*/libexec.so | grep -i "aes\|key\|decrypt"
```

**Goal**: Get the decrypted dex → decompile with jadx → find `BluetoothGattService` UUIDs and `writeCharacteristic` calls.

### Step 3: Runtime frida hooks (if Step 2 fails)
**What**: Hook the live app during a real BLE pairing to dump UUIDs and raw bytes.

**How**:
```bash
# blutter_frida.js is already generated with all offsets
# Add hooks for:
#   - BluetoothGatt.writeCharacteristic (dump UUID + bytes)
#   - BluetoothGattService.addService (dump service UUID)
#   - BluetoothGatt.onCharacteristicChanged (dump notify data)
frida -U -f com.oceanwing.FDMPrint -l blutter_frida.js
# Then perform a real pairing in the app
```

**Goal**: Capture the actual GATT UUIDs and byte-level frame layout during a live pairing.

### Step 4: Map the native→Dart event dispatcher
**What**: Understand how native BLE events (notifications, scan results) are dispatched back to Dart.

**How**: Disassemble the event handler functions in `fdm_base/native_channel/` — look for the callback registration and dispatch logic.

**Goal**: Know the full bidirectional protocol — not just what Dart sends, but what native sends back.

### Step 5: HCI snoop capture (parallel with above)
**What**: Capture a real BLE pairing session at the HCI level.

**How**:
```bash
# On Android: Settings → Developer Options → Enable Bluetooth HCI snoop log
adb shell setprop persist.bluetooth.btsnooplog true
# Perform pairing in EufyMake app
adb bugreport bugreport.zip
# Extract btsnoop_hci.log, open in Wireshark, filter btatt
```

**Goal**: Correlate HCI writes/notifications with the native methods we've documented. This confirms the byte layout and UUIDs.

### Step 6: Implement in Go
**What**: Build a BLE pairing implementation in OpenPolyPrint using `go-ble/ble`.

**How**: Using all the data from steps 1-5:
1. Implement BLE scanner filtering by `bleName` (e.g. "T5216*" for M5C)
2. Connect to GATT, discover services (using UUIDs from Step 2/3/5)
3. Send commands in order: SetFactory → Activate → PinCode → DeviceConfirm → WifiList → WifiConnect
4. Each command: build payload with `stringToBytes` (UTF-8), prefix with command code (0x42-0x4B), write to the correct characteristic
5. Listen on notify characteristic for CommandReply
6. Parse WifiManagerState to detect success/wrong-password/timeout

**Goal**: Pair a fresh M5/M5C without the EufyMake app.

---

## Notes

- **blutter usage**: full analysis crashes (SIGSEGV) on this snapshot; `--no-analysis` succeeds and still yields complete class/method/addr maps, the object pool (strings, enums, const maps), and `blutter_frida.js`. Function bodies were then disassembled ad-hoc with capstone + pool-offset resolution (`add xN, x27, #hi, lsl #12` + `ldr xN, [xN, #lo]` pattern).
- **blutter crash diagnosed**: SIGSEGV in `handleArgumentsDescriptorTypeArguments` (prologue analyzer). GDB backtrace captured. Fixable upstream but the manual capstone pipeline already covers targeted functions.
- **Caller scan technique**: Built a function index from blutter's asm files (sorted by address), then used `bisect` to find which function contains each call site address. This revealed `ble_command_utils.dart` as the payload builder module.
- Code-reference scanning trick: scan `.text` for that 2-instruction pattern to map pool constants (strings/UUIDs) → using code. This is how the three "UUIDs" were debunked.
- Ijiami payload: `assets/ijiami.dat` (9.5MB), `assets/ijiami.ajm` (2.9MB), `assets/IJMDal.Data` (50KB); runtime `assets/ijm_lib/*/libexec.so`.
- The `commandType` JSON (1000/1068/…) is cloud/MQTT; the BLE provisioning link is binary frames built natively. Keep separate.
- Wrong-password path: WifiManagerState `wifiPwError` (code 5) ← `wifiConnection` result; UI key `fdm_bind_wificonnect_pwderr`.
- `nativePageTestProduct` route is entered after `commandSetFactory` succeeds — the factory/production test page chain.
- **Command codes are single-byte** (0x42-0x4B) — no multi-byte framing on the command code itself. The data payload is UTF-8 encoded via `stringToBytes()`.
- **`ble_command_utils.dart` is the key file** — it contains every command payload builder. Once the three `_set*Data` functions are fully disassembled, we'll know the exact field list for each command.
- **The core encoder at 0xfbf720** handles length-prefix/padding — this is the last piece needed to know the exact byte layout of each payload.
