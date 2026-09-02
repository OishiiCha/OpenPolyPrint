# EufyMake APK Decompilation — Findings Log

## Decompilation Setup

- **APK source**: APKMirror — `com.oceanwing.FDMPrint_v4.2.0_9393-9393_2arch_24lang_1feat_97c4582cf7b1cd63dbb98adf96cb5745_apkmirror.com.apkm` (stored at `\\wsl.localhost\kali-linux\home\lucas\apk\`, unpacked to `~/apk/decomp/` in Kali)
- **APK version**: v4.2.0 (build 9393)
- **APK hash (SHA256)**: [not computed yet — run `sha256sum ~/apk/*.apkm` in Kali]
- **Tools used**:
  - [x] jadx (Java decompiler — stub only; Debian wrapper requires absolute paths)
  - [x] unzip (split `.apkm` bundle → base.apk + splits)
  - [x] strings (Dart string/symbol pool mining of `libapp.so`)
  - [x] **blutter `--no-analysis`** — works (full analysis segfaults in `FunctionAnalyzer::handleArgumentsDescriptorTypeArguments` — crash location identified via gdb, unpatched)
  - [x] capstone AOT disassembly + object-pool resolution (repeatable recipe, see Notes)
  - [x] whole-`.text` code-reference scanning (BL-target scan + pool-load scan)
  - [ ] frida (runtime) — `blutter_frida.js` pre-generated
  - [ ] BlackDex / FRIDA-DEXDump — for the Ijiami native dex

### Key structural finding

**This is NOT a normal Java app:**

1. **Ijiami-packed Android shell** — `base.apk` has a 47KB stub `classes.dex`; real Java code encrypted in `assets/ijiami.dat` (9.5MB, entropy 7.98 = fully encrypted; header `03 00 00 00 d4 4d f3 01 "356a007ba"`) + `assets/ijiami.ajm` (2.9MB, header `"indl01"`, entropy 7.99). Decrypted at runtime by `assets/ijm_lib/*/libexec.so` (`ijiami SecLLVM compiler 1.7.4.20`). **Static unpacking = dead end** (would need serious RE of the SecLLVM-obfuscated libexec.so).
2. **The real app is Flutter** — Dart 3.5.4, snapshot `80a49c7111088100a233b2ae788e1f48`, target android arm64. Note: the armeabi-v7a split exists but **blutter does not support arm32**.
3. Java side = Pigeon bridges only, under `com.zhixin.flutter_fdmprint_module.*`.

### Quick start commands (what actually worked)
```bash
# In Kali WSL
cd ~/apk/decomp
unzip ../*.apkm
unzip split_config.arm64_v8a.apk 'lib/arm64-v8a/libapp.so' 'lib/arm64-v8a/libflutter.so' -d flat

# blutter — MUST use --no-analysis (full analysis segfaults)
python3 /opt/blutter/blutter.py ~/apk/decomp/flat ~/apk/decomp/blutter_out --no-analysis
# → asm/ (every class+method+address), pp.txt (object pool), objs.txt, blutter_frida.js
```

Artifacts (Kali): `~/apk/decomp/blutter_out/` (asm/, pp.txt 15MB, objs.txt, blutter_frida.js), `libapp_strings.txt`, `flat/`, `out_res/` (jadx incl. manifest/resources).

---

## 1. BLE Service & Characteristic UUIDs

> **CORRECTION:** the three UUID-shaped strings in the pool are **NOT GATT UUIDs** — code-ref scanning shows they're hardcoded MQTT `message_id` values in `devOnline`/`devOffline` synthetic payloads (adjacent keys: entity/station/feature/device_info/message_id/station_sn/timestamp).

### Findings
- [x] GATT UUIDs live **only in the encrypted native dex** — Dart never references them
- [ ] Actual UUIDs: need runtime dump / Ijiami unpack / HCI capture
- [x] Scan filtering: by **BLE name** (`bleName`/`devBleName`) + `bleType`; `BleDevice` carries `scanRecord` + `mBleAddress` + `rssi`

---

## 2. GATT Communication Protocol

> **FULLY RECONSTRUCTED at the Dart↔native boundary (static).** The Dart layer
> builds `CommandSend` objects; the native layer does GATT + final byte framing.

### 2a. Wire objects (exact field lists from decode/encode disassembly)

**`CommandSend` (Dart → native, channel `…DeviceSendFunctionApi.sendCommand`):**
```
{ commandCode: Byte,    // the command (see table below)
  dataBytes: Uint8List, // TLV payload (see 2c)
  timeout: int = 30000, // 30 s default (bleControl takes explicit timeout)
  onlyBle: bool,        // force BLE transport (skip MQTT)
  bleType, mqttConnType, deviceType, sn, mtuSize }
```

**`CommandReply` (native → Dart):** `{ command, code, bleBytes, isWriteFail, cmdSource }`

**`BleDevice` (scan result, native → Dart):**
`{ mConnectionState, mBleAddress, bleName, deviceType, rssi, scanRecord, connectFailType, disconnectStatus }`

**Native → Dart events (via `AKBleManager.handleNativeCall`):**
`onStateNotify{state}` (BCBLEManagerState) · `onBleSendStateNotify{state, bleName}` (BLESendState) · `onWifiStateNotify{state}` (WifiManagerState)

### 2b. Command codes (Byte constants from object pool — CONFIRMED)

| code | Dart function (BleCommandUtils) | step |
|------|--------------------------------|------|
| **0x42** | `commandSendWifiList` | request Wi-Fi scan/list |
| **0x43** | `commandSendWifiConnect` | send credentials & connect |
| **0x44** | `commandSendActivate` | activation (timezone/domain/userId…) |
| **0x46** | `bleControl` | generic control (explicit timeout param) |
| **0x48** | `commandSendPinCode` | PIN code entry |
| **0x4A** | `commandDeviceConfirm` | physical-button confirm |
| **0x4B** | `commandSetFactory` | factory-state init / re-init |

Also: `commandRequestDeivceLog` / `newCommandRequestDeivceLog` (log upload), `mqttControl` (MQTT variant).

### 2c. dataBytes payload format — **TLV: `[TAG][LEN][BYTES]`** (CONFIRMED)

Strings encoded with `stringToBytes()` = `toUtf8()` (dartx `StringToUtf8Extension.toUtf8` @0xfbf720).

| command | payload |
|---------|---------|
| 0x48 PinCode | `A1 <len> <pin UTF-8>` |
| 0x42 WifiList | `A1 01 <1-byte arg>` `A2 02 <2 bytes>` (abCode-derived) |
| 0x44 Activate | `A1 <timezone (fallback "")>` `A2 <domain>` `A3 <timezone>` `A4 <userId>` — A3 is a second copy of the timezone bytes (from `_setActivateData` register tracking) |
| 0x46 Control | **NOT TLV** — `dataBytes` = UTF-8 of `JsonEncoder.convert(args)` (generic JSON control) |
| 0x4A Confirm / 0x4B SetFactory | no payload (empty dataBytes) |

Command table is **complete**: only 7 `Byte` constants exist in the entire object pool (0x46/0x4B/0x43/0x44/0x48/0x42/0x4A) — no undiscovered command codes.

(0x43's builder at 0x1824560 passes the `wifiInfo` map through without embedding TLV tags itself — native reads ssid/auth/pwd; see §5.)

### 2d. Provisioning Flow (page-level, confirmed)

```
1.  Permissions (bleGetPermissionStatus / blePermission)
2.  SearchDeviceApi.startSearch → onFindDevice(BleDevice{bleName,scanRecord,rssi,…})
3.  BluetoothConnectApi.connectDevice (Pigeon, retried ×2) — sends the **encoded BleDevice map**
      `{mConnectionState, mBleAddress, bleName, deviceType, rssi, scanRecord, connectFailType, disconnectStatus}`
      → MTU via bleGet0x0105Bytes {"slave_id":2,"bleMTU":N}
4.  FACTORY STATE: "connectDevice success, but isFactory so do commandSetFactory"
      → sendCommand{commandCode 0x4B, no payload} → success route "nativePageTestProduct"
5.  commandSendActivate (0x44: A1 timezone | A2 domain | A3 ? | A4 userId)
6.  checkUsePinCode? → commandSendPinCode (0x48: A1 <pin>) — UI: ble_test/input_pin_page.dart
7.  commandDeviceConfirm (0x4A) — "press button to confirm" — add_device_action_confirm_page.dart
8.  bleGetWifis {"isChangeWifi":false} → printer-side scan → List of {ssid, auth, rssi,…}
9.  User picks SSID → wifiInfo built → wifiConnection {wifiInfo, isChangeWifi, mainSn}
10. WifiManagerState result: 0 succ … 5 wifiPwError (wrong password) … 7 deviceCommunicateFail
11. wifiBind {wifiInfo, mainSn} / bindActive (→ compares result to "true") → cloud/MQTT → optional P2P
```

Production-test flow: `lib_device/device_trash/ble_test/` (production test, connect_wifi, input_pin, input_device_name, select_wifi pages).

### 2e. Native method vocabulary (PageEventHelper.notifyNative)

`devSearch{bindInfo}` · `scanStop{}` · `bleDisconnect{bleName}` · `devBleReConnect{bleName,changeWifi,needWifiList}` · `bleGetWifis{isChangeWifi}→List<Map>` · `wifiConnection{wifiInfo,isChangeWifi,mainSn}` · `wifiBind{wifiInfo,mainSn}` · `wifiConnectinAndBind{needBind,wifiInfo}` · `bindActive→bool` · `bleGet0x0103Bytes{slave_id:2,…}→List<Uint8List>` · `bleGet0x0105Bytes{slave_id:2,bleMTU}` · `bleGet0x0233Bytes{slave_id:2}` · `mqttGet0x1029Bytes{sn,cmd:1029,value}` · `checkUsePinCode/devBleIsUsePinCode→bool` · `deviceStatusCheck` · `bleOpen` · `wifiEnableStatus` · `uploadBleLog` · `bigButtonVer`

Callers of byte-builders (BL-scan): `get0x0233Bytes` ← `ak_bind_p2p_conn_model.bindActive` · `get1029Bytes` ← firmware `getConfigList` · `get0x0105Bytes` ← MTU path.

### 2f. State machines (codes from enum pool objects)

**WifiManagerState**: 0 succ, 1 fail, 2 timeOut, 3 scanWifiFail, 4 connect-fail variant, **5 wifiPwError**, 6 getIpFail, 7 deviceCommunicateFail (4/5 exact order tentative)
**BCBLEManagerState** (17): 0 unknown, 1 unsupported, 2 unauthorized, 3 poweredOff, 4 poweredOn, 7 scanErr, 8 scanTimeout, 0xb connectTimeout, 0xe disConnectErr
**BLESendState**: 0 timeOutError, 3 otherError

### 2g. Frame-level (0x0103/0x0105/0x0233/0x1527, slave_id=2)

Built by **native** on request (`bleGet0xNNNNBytes` returns `List<Uint8List>`). Modbus-style `slave_id=2`. Register/transport framing unknown → in encrypted dex.

---

## 3. Encryption / Key Exchange

- [ ] BLE payload encryption: unknown (native). PIN is plain UTF-8 inside the TLV at the Dart→native boundary; native may encrypt before air.
- [x] Ijiami payloads fully encrypted (entropy ≈7.98/7.99) — static unpack dead end; runtime dump needed.

---

## 4. Device Information Exchange

- [x] Model detection via `bleName`; **T5216 = M5C**; scan gives `BleDevice{bleName, mBleAddress, rssi, scanRecord, deviceType}`
- [x] SN keys: `sn`, `devSN`, `mainSn`, `station_sn`
- [ ] SN/DUID byte formats: inside 0x0103/0x0105 responses (native)

---

## 5. Wi-Fi Configuration Protocol

- [x] **`FDMWifiItemModel.toMap()` = `{"wifiInfo": …, "ssid": …, "auth": …, "pwd": …}`** (confirmed via code-ref scan; builder sits right before `wifiConnectinAndBind`)
- [x] Wi-Fi list entries from `bleGetWifis`: `{ssid, auth, rssi, …}` (parse refs at 0x19006b8/0x1900708); UI model adds `encryptionType`
- [x] Security: **WPA/WPA2 only** (explicit no-WPA3 strings)
- [x] Wrong password → WifiManagerState 5 (`wifiPwError`) / `fdm_bind_wificonnect_pwderr`
- [ ] 0x43 wire payload: `wifiInfo` passes through to native (native reads ssid/auth/pwd)

---

## 6. PPPP / LAN Transition

- [x] `AKP2PManager`/`AKBindP2PConnModel`; BLE = "wired-equivalent" transport for P2P stack (`"changed access mode from P2P wired to BLE wired (mode 2 -> 1)"`)
- [x] `bindActive` (ak_bind_p2p_conn_model) uses `get0x0233Bytes` — 0x0233 is part of the bind/activate sequence
- [ ] DUID/P2P keys: unknown

---

## 7. Cloud / MQTT Registration

- [x] `bindActive` result compared to `"true"`; `sendMqttCommand` channel; `CommandMqttSend` wire class; `getCMDInfo` channel `…DeviceSendFunctionApi.getCMDInfo`
- [x] MQTT status JSON `{"commandType":1000/1068/1085/1192/1601,…}`; error codes 0xFB…/0xFD…
- [x] devOnline/devOffline synthesize payloads with hardcoded message_id UUIDs (the debunked "UUIDs")
- [x] **Community cross-reference**: [Ankermgmt/ankermake-m5-protocol](https://github.com/Ankermgmt/ankermake-m5-protocol) (ankerctl) documents the **cloud-side** MQTT/PPPP/HTTPS APIs (pppp keys fetched from cloud after credential import) — but has **no BLE provisioning coverage**. Our Dart-level BLE findings are new territory; after cloud bind, their MQTT/PPPP docs should slot in.

---

## 8. Native Libraries (.so files)

| File | Size | Purpose | Status |
|------|------|---------|--------|
| libapp.so | 31MB | Dart AOT — all app logic | ✅ decompiled (blutter no-analysis + capstone) |
| libflutter.so | 10MB | engine 3.5.4 | version only |
| libakai.so | 10MB | OpenCV vision | identified |
| ijm libexec.so + ijiami.dat/ajm | ~12.5MB | Ijiami packer + **encrypted native dex (GATT + framing)** | ❌ static dead end; runtime dump needed |

---

## 9. Class Map (Dart)

Key files:
- `fdm_base/native_channel/utils/ble_command_utils.dart` — **all provisioning commands** (0x42–0x4B) + payload builders
- `fdm_base/native_channel/utils/byte_utils.dart` — `stringToBytes`, `bleControl` helpers
- `fdm_base/native_channel/channel/flutter_channel_ble.dart` — `BluetoothConnectApi`, `SearchDeviceApi`, `BleDevice`, `CommandReply`, `CommandSend`, `CommandMqttSend`, `ScanFailType`
- `fdm_base/native_channel/channel/flutter_channel_device_send_fun.dart` — `DeviceSendFunctionApi.sendCommand/sendMqttCommand/getCMDInfo`
- `fdm_base/native_channel/fdmchannel/fdm_page_event_helper.dart` — `notifyNative` + bind/wifi/scan calls
- `fdm_base/communication/ble/ak_ble_manager.dart` + `ak_ble_enum.dart`
- `lib_device/device_trash/add_device/*` (factory/confirm pages), `lib_device/device_trash/ble_test/*` (production test)
- `lib_device_uv_bind/bind/*`, `uv_t5216/*`

Key addresses (arm64 libapp.so):
```
BleCommandUtils.commandSetFactory 0x170dc2c   commandSendWifiConnect 0x18244bc
commandSendActivate 0x182603c  _setActivateData 0x18260e0
commandSendPinCode 0x18279d0  _setSendPinCodeData 0x1827a74
commandSendWifiList 0x1828c80 _setRequestWifiData 0x1828d2c
commandDeviceConfirm 0x19d8804  bleControl 0xfbf1dc
CommandSend.ctor 0xfbf284  CommandSend.decode 0x13f1d70  CommandReply.decode 0x13f0ed8
BleDevice.decode 0x13f12e4  connectDevice 0x170d4dc  startSearch 0x170e408
onFindDevice 0xf0f438  handleNativeCall 0xff427c  toUtf8 0xfbf720
stringToBytes 0x182567c  get0x0103Bytes 0xffa054  get0x0105Bytes 0xff99b0
get0x0233Bytes 0x18f8cec  get1029Bytes 0x18e0eb0  wifiConnection 0x18ffa84  bleGetWifis 0x1900528
FDMWifiItemModel.toMap 0x18f0bb8  wifiConnectinAndBind 0x18f0ae0  bindActive 0x18f7fec
```

---

## 10. Raw Captures

- none yet. When possible: HCI snoop; each `sendCommand{commandCode}` ≙ one GATT write sequence; `CommandReply.bleBytes` = response frame.

---

## 11. Open Questions

- [ ] GATT service/characteristic UUIDs (native dex only)
- [ ] Native framing of CommandSend → BLE bytes (header/seq/checksum/encryption; slave_id=2 register protocol)
- [ ] Confirm WifiManagerState 4/5 naming order
- [ ] PIN transport security (native may encrypt the TLV)
- [ ] Fix blutter crash (`FunctionAnalyzer::handleArgumentsDescriptorTypeArguments` via `handlePrologue`) → full IL for all functions
- [ ] `_dataFormat` (0xff9b00): appends parsed k/v pairs to the 0x0103 map (partial: iterates a string char-by-char, builds Uint8List via closure)

---

## 12. Progress Tracker

| Task | Status | Notes |
|------|--------|-------|
| Unpack .apkm / architecture | done | Flutter 3.5.4 + Ijiami shell |
| blutter decompile | done (--no-analysis) | full-analysis crash located, unpatched |
| Command codes 0x42–0x4B | **done** | §2b |
| TLV payload format + per-command payloads | **done** | §2c |
| Wire objects (CommandSend/Reply/BleDevice) | **done** | §2a |
| Native→Dart events | done | §2a |
| State-machine codes | done | §2f |
| wifiInfo structure {ssid,auth,pwd} | done | §5 |
| UUID candidates | eliminated | MQTT message_ids; real UUIDs in native dex |
| Ijiami static unpack | **dead end** | entropy ≈8, SecLLVM; needs runtime dump |
| GATT UUIDs / native framing | pending | requires Android runtime (emulator/device) |
| Live BLE capture | pending | |
| Implement in Go | pending | Dart-level protocol now implementable except native framing |

---

## Notes

- **Reusable recipe** (capstone + pool resolution): disassemble function → track `add xN, x27, #hi, lsl #12` + `ldr xN, [xN, #lo]` → look up `pp.txt` at `pp+hi+lo`. Whole-`.text` scans: (a) BL-opcode scan for callers of a known address; (b) pool-load scan for code referencing a known pool string. Both used repeatedly this session.
- **blutter crash**: gdb backtrace = `FunctionAnalyzer::handleArgumentsDescriptorTypeArguments` ← `processPrologueParametersInstr` ← `handlePrologue` ← `asm2il`. Fixable in `/opt/blutter/blutter/src/CodeAnalyzer_arm64.cpp` if needed.
- Byte-constant trick: command codes are `Obj!Byte@…` objects in pp.txt with `off_8: int(0xNN)` values.
- `_setSendPinCodeData` disassembly proves TLV: `strb 0xA1; strb len; memcpy(pin utf8)`.
- `timeout` default 30000 ms in `CommandSend` (bleControl exposes explicit timeout).
- Ijiami headers for reference: `ijiami.dat` = `03 00 00 00 d4 4d f3 01 "356a007ba"`; `ijiami.ajm` = `"indl01" 00 00 18 00 00 00 d0 45 01 00`.
- The `commandType` JSON (1000/1068/…) is cloud/MQTT — separate from the BLE binary protocol.
