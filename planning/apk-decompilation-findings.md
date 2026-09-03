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
- [x] **Advertising format (captured live 2026-09-02, M5C fresh/unbound):**
  - Name: `M5C_E8:EE:CC:9F:E8:57` = `<model>_<MAC>` where MAC = advertiser address + 1 (E8:EE:CC:9F:E8:56 adv) — likely WiFi MAC
  - Advertised service UUID: `fb349b5f-8044-3380-7210-656b4d416e41` (tail = ASCII `AnAMke`) — **advertising beacon only, not in the GATT table**
  - Manufacturer data: `0xffff: 0101` (possible factory/unbound status flags — re-check after binding)
- [x] **GATT table (enumerated live via `tools/ble_probe`):**
  - Service **`0000414d-0000-1000-8000-00805f9b34fb`** (`0x414d` = ASCII "AM" = AnkerMake)
  - Single characteristic **`00003344-0000-1000-8000-00805f9b34fb`** (handle 41, `0x3344` = ASCII "3D"), properties **write-without-response + notify** — one pipe for commands AND replies
  - Plus standard GAP/GATT (0x1800/0x1801) with read-only chars; nothing else custom
  - Default MTU 23 at connect → explains the app's early MTU negotiation (`bleGet0x0105Bytes {slave_id:2, bleMTU:N}`) before sending SSID/password TLVs
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

### 2g. Frame format on the wire — **DECODED (live capture 2026-09-02)**

The native framing around the command codes, captured from a fresh M5C via `tools/ble_probe monitor`:

```
[0:2]   4d 41        magic "MA" (ASCII — AnkerMake)
[2:4]   len LE16     total frame length (heartbeat = 0x0019 = 25)
[4:8]   05 01 01 05  constant header (version/flags — meaning TBD)
[8]     CMD          command code — SAME numbering as the APK table (§2b)!
[9:12]  c0 01 00     constant (flags/subcode — meaning TBD)
[12:18] 6 bytes      varies every frame (nonce/counter/encrypted status — TBD)
[18:24] 00 x6        zero padding
[24]    XOR-8        checksum: XOR of ALL preceding bytes (verified on 4+ frames)
```

- Printer pushes an unsolicited **0x46 (bleControl) status heartbeat every ~3 s** immediately after GATT connect — before any write. This is how the app learns device state (likely incl. the factory/unbound flag).
- MTU 23 default → heartbeat is exactly 25 bytes (fits ATT payload); larger commands need the app's MTU negotiation first.

### 2h. Transport (from live enum)

Single characteristic `00003344-…` (handle 41) — write-without-response for commands, notify for replies/heartbeat. See §1.

### 2j. THE HANDSHAKE CAPTURED (official app, HCI snoop 2026-09-02 23:27) — protocol fully decoded

**Why all Pi probes were dropped:** app→printer frames use header bytes `01 01 01 02` at [4:8]; the printer's own heartbeats use `05 01 01 05`. We mirrored the printer's header — wrong direction byte = silent drop. No crypto needed to be *heard*, just the right header.

**App frame layout (64-byte prologue):**
```
[0:2]   4d 41       magic "MA"
[2:4]   len LE16    total incl. xor byte
[4:8]   01 01 01 02  client header (heartbeat uses 05 01 01 05)
[8]     CMD
[9:12]  flags       c0 01 00 (simple cmds) | c1 01 00 + c3 02 00 (0x46 control, sent in pairs)
[12:14] seq LE16    increments across session (0x40b5, 0x40b8, … 0x410a)
[14:32] session id  18 bytes, e.g. 98 6a 63 35 63 66 61 37 35 35 64 63 63 34 34 61 32 35 ("…c5cfa755dcc4a25") — source TBD (app session? account?)
[32:64] zeros
[64:-1] payload     (encrypted after handshake; first frames plaintext-ish TLV)
[-1]    XOR-8       verified xor-ok on all app frames
```

**Printer replies** arrive as notifications **fragmented into 20-byte ATT chunks** (MTU 23, no MTU exchange ever) — reassembly by len field required. Observed reply sizes: 252B (to 0x45), 41B (0x49/0x4A/0x43/0x44 acks), 57B/73B (wifi-list entries), 425B/345B (0x46 control pairs), 137B (status).

**Session flow (captured):**
```
0x45 (136B): TLV A1[65] = 0x04 || X || Y  → app's P-256 ECDH PUBLIC KEY (uncompressed)
             TLV A2[2] = 05 02 (curve/params id)
   ← 252B reply (printer key/cert, encrypted from 0x49 on)
0x49 (113B): challenge/verify (encrypted payload)
0x4A confirm (81B, 16B encrypted payload) ← the button-press step (user confirms on printer)
0x42 wifi_list → 6 × (57B/73B) fragmented entries
0x43 wifi_connect (129B) ← 41B ack
0x44 activate (177B) ← 41B ack
0x46 control pairs (c101:180B + c302:110B, same session) — ongoing status/printing channel
```

**Crypto**: A1[65]=0x04-prefixed EC point = P-256 ECDH — same family as Anker PPPP crypto already implemented in `internal/anker/proto/crypto/ecdh.go`. Post-handshake payloads are encrypted (high-entropy), key presumably ECDH-derived. Heartbeat "random" 6 bytes = likely encrypted/rolling session state.

### 2k. Our own sessions (2026-09-02/03, Pi as client) — handshake REPRODUCED

- **Header fix**: app frames = `01 01 01 02` at [4:8]; **len field includes the XOR byte**. Both facts were the entire "silent drop" mystery.
- **0x45 handshake works from our own client**: send TLV `A1[65]=secp256k1 uncompressed pubkey` + `A2[2]=0502` → printer replies 252B with ITS ephemeral k1 pubkey + plaintext metadata:
  - `A3` `{"marlin_hw_version":"V2.0.0","junzheng_hw_version":"V8110_HW_V2.0"}`
  - `A4` `{"device_sn":"AK75D7D345000049","nozzle_hw_version":"0.0.4"}`
  - `A5` `"V3.1.56"` (firmware)
- **Curve: secp256k1** (verified: printer point valid on k1, invalid on r1/SM2). Keys are **ephemeral** (printer rotates per connection). Client keys are apparently not curve-checked (our r1 key was accepted).
- **ECDH shared secret computed in 3 own sessions** (working code in `tools/ble_probe/maclient3+`). Reference pair: shared `3c058b30…92f1` → session token `14d2e781aaa0150996f5cc34354e7fc9`.
- **ACK semantics**: post-handshake, commands get 41B cmd-matched ACKs: `[4B nonce][8B zeros][16B session token][xor]`. Token = constant within a session, changes per session; pre-handshake (0x40-error era) token was device-constant `9c46cc9dff7c323a1259c0bdc6fcd236` across connections.
- **Session id field is client-generated**: the app invented `986a…25` per session and the printer accepts arbitrary values (ours worked).
- **Commands do NOT execute without an encrypted payload**: every app command carries one (0x4A/0x42: 16B = 1 AES block; 0x49: 48B; 0x43: 64B; 0x44: 112B; 0x46 pairs: 116B/45B). Empty/garbage payloads → generic ACK, no execution. 0x49 is optional (2nd app round skipped it before 0x4A/0x42).
- **Failed derivations (ruled out)**: session token ≠ any of {md5/sha1/sha256/hmac/AES-ECB combos of shared, pubs, dev_const, MAC, SN, hex-strings} (30+ candidates × 3 known pairs). Command-payload key ≠ {md5(s), sha(s) halves, s-halves} × {zeros, token, md5(s)} × {ECB, CBC-0} (24 candidates live-tested — all generic ACKs).
- **Replay experiment (decisive)**: the app's exact captured frames (0x45 → 0x4A → 0x42 ×2 → 0x43 → 0x44, machine-verified bytes) sent from our own connection → handshake answered with the printer's NEW ephemeral key, all commands ACK'd, **zero execution, no confirm beep**. ⇒ **Command ciphertexts are bound to the session's ephemeral ECDH; the key is NOT device-static.** The DSK (cloud device key `xdgfZrMLEVuRBTOs4aAi`, confirmed present in OpenPolyPrint config) may contribute to the KDF (e.g. f(DSK, session-secret)) but is not sufficient alone.
- **Connection model (tested)**: the M5C accepts **one BLE connection at a time** — while the Pi holds a connection, the phone app cannot connect (and vice versa). No concurrent observation possible; MITM at GATT level is structurally impossible without dedicated sniffer hardware.
- Anomaly for future work: the 0x44 (activate) replay ACK returned a DIFFERENT session token (`e0ba83f4…`) than all other ACKs in that session (`aec5b40c…`) — first observed mid-session state change.
- **Campaign totals**: ~55 offline derivations + ~84 live candidates (AES/curse/simple × key families × plaintexts) + full-sequence replay — all negative on command execution. Handshake, framing, identity, and ACK layer: fully working from the Pi.
- **The remaining unknown (final)**: the KDF from (session ECDH secret [, DSK]) → command-encryption key, and the command plaintext envelope. This exists only in the Ijiami-encrypted native dex. **Path to finish: BlackDex (or FRIDA-DEXDump) on any rooted Android → dump decrypted dex → jadx → locate BleCommandUtils/CommandSend native implementations → extract KDF + cipher.** Everything else needed to implement local provisioning is already documented in this file and proven live.

### 2i. Live probe results (2026-09-02, pre-discovery)

- Heartbeat cadence: precisely ~3.015 s — makes it easy to distinguish replies from scheduled frames. XOR-8 checksum verified live on hundreds of frames (`[xor-ok]`).
- **All rounds SILENTLY DROPPED** (heartbeats stayed on the 3.015s grid after every write):
  - raw bytes, `[slave_id][cmd]`, replayed heartbeat
  - minimal + full MA frames, all 7 command codes, with/without TLV payloads
  - write-with-response vs without; flags variants (`c00100/c00101/000100`); header variant (`0101→0000`)
  - token-echo frames (printer's last 6-byte token embedded, auto-copied)
  - **v3: byte-perfect 25B frames — len=0x19 (incl. checksum), valid XOR, structure identical to genuine frames — still dropped**
- One apparent grid deviation (22:33 session) did not reproduce on clean repeats — radio hiccup, not a signal.
- **Conclusion: the command channel is session-gated (encrypted or handshake-protected). Plaintext probing is exhausted.**
- Remaining gates tested next: LE pairing (`bluetoothctl pair` + retry).
- **Pairing result: REFUSED** — `bluetoothctl pair` connects and resolves services, but `Paired: no / Bonded: no`. The M5C has **no LE link-layer security**; the command gate is purely application-layer crypto in the app's first writes.
- Peripheral details from BlueZ `info`: address type **public**, advertising flags `0x06` (BR/EDR not supported, general discoverable), ManufacturerData `0xffff: 0101`, handle of `3344` char value = **0x002A** (decl 0x29).
- **Decisive next step: HCI snoop capture of one official-app setup** — decode with `tools/ble_probe/hci_decode.py`. Note: OpenPolyPrint's `internal/anker/proto/crypto` already implements Anker's PPPP crypto (ECDH, seccode, "curse") — the BLE session handshake may reuse the same family; compare the first app→printer writes in the capture against those implementations before assuming something new.

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
| GATT UUIDs | **done (live)** | service `414d` ("AM"), char `3344` ("3D") w/wo-response+notify, handle 41 |
| Native framing | **done** | MA frames, direction headers, XOR-8, len-incl-xor, 64B client prologue, seq+session |
| Handshake (0x45, secp256k1 ECDH) | **done (own client)** | shared secret extracted 3×; printer SN/firmware read |
| Command execution | **blocked on KDF** | needs session-key derivation for the encrypted command payloads |
| Unpack Ijiami dex (BlackDex) | pending | fastest route to the KDF |
| Live BLE capture | **done** | official app HCI capture decoded |
| Implement in Go | pending | Dart-level protocol now implementable except native framing |

---

## 14. Key rotation & bound-state discovery (2026-09-03)

- **DSK and mqtt_key ROTATE on every factory-reset + re-bind** — proven by log forensics: OpenPolyPrint's MQTT decryption worked until the exact second of the re-bind (15:00:44 container time), then failed permanently (PKCS7 errors); after re-login the cloud issued a new `p2p_key` (`xdgfZrMLEVuRBTOs4aAi` → `mc6FEc6X79zOzcPhL8GE`) and MQTT healed. **All DSK cipher batteries run on 2026-09-02 used the STALE key** — re-run with the fresh key: still negative (curse/AES × ascii/b64/case/md5 × heartbeats/app-payloads/tokens/137B frames).
- **Heartbeat is state-dependent**: unbound printer → 25B minimal frames; **bound printer → 137B rich status frames** every 3s (matches app-capture post-binding frames). Structure: `[12B hdr][4B nonce][4B counter][4B zeros][80B constant blob][32B varying tail][xor]`. Constant blob stable within a session — encrypted state/config; not opened by DSK-family ciphers.
- **MQTT status stream confirmed**: `/phone/maker/{sn}/notice` carries printer status every 3s, AES-encrypted with the (rotated on re-bind) mqtt_key; OpenPolyPrint decrypts it fine with a current key — a working local status channel already.
- **Server-down implication**: keys held at any moment remain valid indefinitely; a factory reset AFTER server loss is unrecoverable (no way to fetch the new DSK). OpenPolyPrint must treat cached keys as critical persistent state.
- **OTA API verdict (final)**: `/v1/app/ota/get_rom_version` + `T5216_Model` returns `32000` uniformly — even with no check_code, any version, any derivation ⇒ **32000 = request-signature verification failure**. The signed headers (`X-Signature` etc.) are computed in the NATIVE layer (`getV3Headers`/`getAiotHeaders` in Dart are thin `notifyNative` wrappers — disassembled and confirmed). Token is short-lived (401 after ~minutes). The app-API road ends at the Ijiami dex, same as the BLE crypto.
- Remaining viable routes: (1) rooted Android + BlackDex → dump dex → extract BOTH the BLE KDF and the header signer; (2) MQTT-observed OTA push (requires an actual update event); (3) printer firmware via other physical means (flash dump); (4) **MITM proxy on non-rooted phone** — see §15.

---

## 15. Tuya KDF comparison & MITM proxy avenue (2026-09-03)

### 15a. Tuya BLE SDK KDF comparison — NEGATIVE

Tested Tuya-style KDF patterns against the device-constant pre-handshake token (`9c46cc9dff7c323a1259c0bdc6fcd236`). Script: `tools/ble_probe/tuya_kdf_test.py`.

**Tuya BLE session key derivation** (from ESP-TuyaBLE source code):
```
session_key = MD5(local_key[:6] + srand[:6])   // srand = 6B nonce from device info response
```
Security flags: 0x04 = MD5(local_key[:6]), 0x05 = session_key, 0x06 = no key.

**Tuya LAN v3.4 session key**:
```
session_key = AES_ECB_128(XOR_bytes(client_random, gw_random), local_key)
```

**89 candidates tested** against the device-constant token, using DSK (old+new, ascii+base64 forms), SN, MAC, DUID in all combinations of MD5/SHA1/SHA256/AES-ECB:
- `MD5(dsk + sn)`, `MD5(sn + dsk)`, `MD5(dsk + mac)`, `MD5(mac + dsk)` — all forms
- `MD5(dsk[:6] + sn[:6])` — Tuya BLE style with DSK as "local key"
- `SHA256(dsk)[:16]`, `SHA256(sn+mac)[:16]` — truncated hashes
- `AES_ECB(dsk, zeros/sn/mac)` — Tuya LAN style
- `AES_ECB_decrypt(dsk, token)` — reverse lookup (all produced garbage)
- SN/MAC/DUID alone and combined

**Result: ZERO matches.** The device-constant token is NOT derivable from DSK, SN, MAC, or DUID using any standard pattern. This confirms the KDF uses a **device-internal factory key** that lives only in the junzheng module's flash — not in any cloud-provided credential.

**Conclusion**: The Tuya crypto family is related (ECDH + AES-ECB) but the AnkerMake KDF uses an additional secret not available outside the device. Tuya's SDK assumes the `local_key` is provisioned at factory time and known to both sides — AnkerMake's equivalent is a factory-internal key never exposed via cloud APIs.

### 15b. MITM proxy on non-rooted phone — NEW ROUTE (no root needed)

**Insight**: We don't need to crack the Ijiami-signed headers to get the OTA firmware. We just need to **capture** them from a live app session.

**Setup (no root required)**:
1. Install mitmproxy (or Proxyman/Charles) on the PC
2. Install the proxy's CA certificate on the phone (Settings → Security → Install certificate)
3. Configure the phone's WiFi proxy to point at the PC
4. Open the EufyMake app → Settings → Firmware → Check for Update

**What we capture**:
- The full OTA API request including signed headers (`X-Signature`, `X-Key-Ident`, `X-Request-Ts`, `X-Request-Once`, `X-Encoding`)
- The OTA API response — if an update is available, this contains the **firmware download URL** (CDN link, likely unsigned)
- Even if no update is available, the response tells us the current firmware version and may contain CDN URLs for the current firmware

**Why this works**:
- The signed headers are generated by the native Ijiami layer, but we don't need to understand them — we just capture and replay them
- The token is short-lived (~minutes), but we can immediately use it to fetch the firmware
- The CDN hosting the firmware binary likely doesn't require authentication — just the URL
- Even if the CDN requires auth, we capture the auth from the same session

**What we get**: the OTA firmware package → extract the junzheng BLE module binary → static ARM RE → the session KDF (no Ijiami obfuscation on the junzheng firmware, per §13).

**Risk**: Ijiami may implement certificate pinning. If so, the app will refuse to connect through the proxy. Bypass options:
- `frida-gadget` patched APK (objection patchapk) to disable pinning — works on non-rooted phones
- Or: use a VPN-based MITM (like PCAPdroid) that captures raw traffic without proxy settings — won't decrypt TLS but may reveal the CDN hostname, which we can then try to access directly

**If no update is available**: The OTA API still returns the current version info. More importantly, the **CDN URL pattern** may be predictable (e.g. `https://cdn.ankermake.com/ota/T5216/V3.1.56/firmware.bin`). Once we see the URL structure from any response, we can try other version numbers.

### 15c. Option 5 (MQTT OTA observation) — REVISED ASSESSMENT

**Original idea**: Watch MQTT for OTA push notifications.
**Problem**: The OTA check is app→cloud (HTTPS), not over MQTT. The printer doesn't initiate OTA checks. Without Anker actively pushing an update, there's nothing to observe on MQTT.
**Verdict**: Dead unless Anker pushes an OTA update to this specific printer. The MITM proxy approach (§15b) is strictly better — it works on demand, doesn't require waiting for an update push, and captures the signed headers directly.

## 13. Firmware / OTA avenue (2026-09-03 online sweep)

- **Marlin GPL source** (github.com/eufymake/eufyMake-Marlin, cloned): NO BLE code — `queue.cpp:672` ("only response uart1 from junzheng") confirms the BLE+WiFi stack lives on the **junzheng companion module** (V8110), fed to Marlin over UART1. The junzheng firmware ships in **OTA packages** — plain binaries, no Ijiami.
- **Tuya SDK** (hjytry/tuya-ble-sdk): crypto family public (ECDH/session-key modes in compiled libs) but M5C GATT/framing is Anker-custom.
- **Anker OTA API**: `POST https://make-app-eu.ankermake.com/v1/app/ota/get_rom_version` (EU region; token in `/data/ankerctl/default.json`), body `{sn, check_code, device_type, current_version_name}`.
  - **device_type for M5C = `T5216_Model`** (20008→32000 progress vs other values)
  - check_code = `md5(duid + "+" + duid[-4:] + "+" + mac)` (duid `EUPRAKM-012822-SMKXB`, wifi mac `E8EECC9FE857`) → but 32000 persists across check_code variants ⇒ **gated on app-signed headers**
  - Required headers (from APK pool): `X-Key-Ident`, `X-Request-Ts`, `X-Request-Once`, `X-Signature`, `X-Encoding`, (+ `app_version`, `timezone`, `platform`, `language`). Signing algorithm is in Dart: `getV3Headers` @0x15799ac, `getAiotHeaders` @0x10b452c — extractable with the same capstone+pool pipeline used for §2.
- **Zero-code alternative**: OpenPolyPrint's MQTT client (decrypts with stored mqtt_key, subscribes `/phone/maker/{sn}/notice|command/reply|query/reply`) may observe the OTA notify when the app checks firmware — watch `docker logs -f openpolyprint` during an app firmware-check.
- Firmware package goal: extract junzheng BLE firmware binary → static ARM RE → the session KDF (no obfuscation expected).

## Notes

- **Reusable recipe** (capstone + pool resolution): disassemble function → track `add xN, x27, #hi, lsl #12` + `ldr xN, [xN, #lo]` → look up `pp.txt` at `pp+hi+lo`. Whole-`.text` scans: (a) BL-opcode scan for callers of a known address; (b) pool-load scan for code referencing a known pool string. Both used repeatedly this session.
- **blutter crash**: gdb backtrace = `FunctionAnalyzer::handleArgumentsDescriptorTypeArguments` ← `processPrologueParametersInstr` ← `handlePrologue` ← `asm2il`. Fixable in `/opt/blutter/blutter/src/CodeAnalyzer_arm64.cpp` if needed.
- Byte-constant trick: command codes are `Obj!Byte@…` objects in pp.txt with `off_8: int(0xNN)` values.
- `_setSendPinCodeData` disassembly proves TLV: `strb 0xA1; strb len; memcpy(pin utf8)`.
- `timeout` default 30000 ms in `CommandSend` (bleControl exposes explicit timeout).
- Ijiami headers for reference: `ijiami.dat` = `03 00 00 00 d4 4d f3 01 "356a007ba"`; `ijiami.ajm` = `"indl01" 00 00 18 00 00 00 d0 45 01 00`.
- The `commandType` JSON (1000/1068/…) is cloud/MQTT — separate from the BLE binary protocol.
