# EufyMake APK Decompilation — Findings Log

## Decompilation Setup

- **APK source**: [where did you get it — APKMirror, APKPure, pulled from device?]
- **APK version**: [e.g. v2.5.1]
- **APK hash (SHA256)**: [run `certutil -hashfile eufymake.apk SHA256`]
- **Tools used**:
  - [ ] jadx (Java decompiler)
  - [ ] apktool (resource extraction)
  - [ ] frida (runtime hooking)
  - [ ] mitmproxy (network capture)
  - [ ] Other: [list]

### Quick start commands
```bash
# Decompile with jadx (GUI or CLI)
jadx -d eufymake_decompiled eufymake.apk

# Or with apktool for resources
apktool d eufymake.apk -o eufymake_apktool

# Search for BLE-related classes
grep -r "BluetoothGatt" eufymake_decompiled/sources/ --include="*.java" -l
grep -r "writeCharacteristic" eufymake_decompiled/sources/ --include="*.java" -l
grep -r "UUID.fromString" eufymake_decompiled/sources/ --include="*.java" -l
```

---

## 1. BLE Service & Characteristic UUIDs

> These are the most critical pieces. The printer advertises a service UUID, and the app connects to specific characteristics to read/write data.

### Service UUIDs
| UUID | Name/Purpose | Notes |
|------|-------------|-------|
| [e.g. 0000xxxx-0000-1000-8000-00805f9b34fb] | [e.g. Device Information] | [standard or custom?] |
| [e.g. xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx] | [e.g. Anker Setup Service] | [custom — this is the one we need] |

### Characteristic UUIDs
| UUID | Service | Properties | Direction | Purpose |
|------|---------|-----------|-----------|---------|
| [UUID] | [service UUID] | Read | Phone → Printer | [e.g. Get device info] |
| [UUID] | [service UUID] | Write | Phone → Printer | [e.g. Send Wi-Fi credentials] |
| [UUID] | [service UUID] | Write+Notify | Bidirectional | [e.g. Key exchange] |
| [UUID] | [service UUID] | Notify | Printer → Phone | [e.g. Status updates] |

### Where to find these in the APK
Search for:
```
grep -r "UUID.fromString" eufymake_decompiled/sources/ --include="*.java"
grep -r "0000" eufymake_decompiled/sources/ --include="*.java" | grep -i uuid
grep -r "service" eufymake_decompiled/sources/ --include="*.java" | grep -i uuid
```
Look in classes like:
- `BluetoothLeService.java` or similar
- `BleManager.java`
- `PrinterSetupActivity.java`
- `DevicePairingHelper.java`
- Any class with `Gatt` in the name

### Findings
- [ ] Service UUIDs found: [list]
- [ ] Characteristic UUIDs found: [list]
- [ ] Advertising data format: [describe]
- [ ] Scan filter: [what does the app filter on?]

---

## 2. GATT Communication Protocol

> Document the exact sequence of reads/writes/notifications during pairing.

### Connection Flow
```
1. App scans for printer advertising [service UUID]
2. App connects to GATT server
3. App discovers services
4. App [reads/writes] characteristic [UUID] → [what data?]
5. Printer [responds via notify/read] on [UUID] → [what data?]
6. ...
7. Pairing complete, printer connects to Wi-Fi
```

### Message Format

> What does the payload look like? Is it raw JSON, binary, protobuf, or encrypted?

- [ ] Payload encoding: [JSON / Protobuf / raw bytes / encrypted]
- [ ] Byte order: [little-endian / big-endian]
- [ ] Header format: [if binary, describe header bytes]
- [ ] Checksum: [any CRC/checksum?]

#### Example: Wi-Fi Credential Exchange
```
Phone writes to [UUID]:
  [hex dump or structure description]
  - Bytes 0-3: [magic header?]
  - Bytes 4-N: [SSID]
  - Bytes N+1-M: [password]
  - Bytes M+1: [checksum?]

Printer responds on [notify UUID]:
  [hex dump or structure description]
  - Byte 0: status code (0 = success)
  - ...
```

### Where to find in APK
Search for:
```
grep -r "writeCharacteristic" eufymake_decompiled/sources/ --include="*.java"
grep -r "onCharacteristicChanged" eufymake_decompiled/sources/ --include="*.java"
grep -r "onCharacteristicWrite" eufymake_decompiled/sources/ --include="*.java"
grep -r "setValue" eufymake_decompiled/sources/ --include="*.java" | grep -i char
```

### Findings
- [ ] Write sequence documented: [yes/no]
- [ ] Payload format identified: [describe]
- [ ] Notify/response format identified: [describe]
- [ ] Full pairing sequence mapped: [yes/no]

---

## 3. Encryption / Key Exchange

> If the BLE payloads are encrypted, we need to understand the encryption to reproduce it.

### Encryption Details
- [ ] Is payload encrypted? [Yes / No / Partially]
- [ ] Algorithm: [AES-128 / AES-256 / ChaCha20 / custom / unknown]
- [ ] Key source: [hardcoded / derived from device / ECDH exchange / user input]
- [ ] Key length: [128 / 256 / other]
- [ ] Mode: [ECB / CBC / GCM / CTR]
- [ ] IV handling: [static / random / counter]

### Key Exchange Flow
```
1. [What happens first?]
2. [ECDH? Pre-shared key? Derived from device serial?]
3. [How is the session key established?]
```

### Where to find in APK
Search for:
```
grep -r "Cipher" eufymake_decompiled/sources/ --include="*.java" | grep -i "AES\|encrypt\|decrypt"
grep -r "KeyGenerator" eufymake_decompiled/sources/ --include="*.java"
grep -r "ECDH\|ECKey\|KeyAgreement" eufymake_decompiled/sources/ --include="*.java"
grep -r "SecretKeySpec" eufymake_decompiled/sources/ --include="*.java"
grep -r "encrypt" eufymake_decompiled/sources/ --include="*.java" -l
```

Also check for native libraries:
```
# Check for .so files that might contain crypto
ls eufymake_apktool/lib/*/ 
grep -r "encrypt" eufymake_apktool/lib/ --include="*.so"  # may need strings command
strings eufymake_apktool/lib/arm64-v8a/lib*.so | grep -i "aes\|encrypt\|key"
```

### Findings
- [ ] Encryption algorithm identified: [which?]
- [ ] Key derivation/exchange documented: [describe]
- [ ] Hardcoded keys found: [list if any — WARNING: do not commit actual keys to public repo]
- [ ] Can we reproduce encryption in Go? [yes/no/partial]

---

## 4. Device Information Exchange

> What info does the printer provide over BLE during setup?

### Data Obtained
| Field | How (read/notify) | Characteristic UUID | Format |
|-------|-------------------|---------------------|--------|
| Serial number | [read/notify] | [UUID] | [string/hex/int] |
| DUID | [read/notify] | [UUID] | [string/hex] |
| Firmware version | [read/notify] | [UUID] | [string] |
| Hardware version | [read/notify] | [UUID] | [string] |
| Device name | [read/notify] | [UUID] | [string] |
| Model (M5/M5C) | [read/notify] | [UUID] | [enum/string] |
| MAC address | [read/notify] | [UUID] | [6 bytes] |

### Where to find in APK
Search for:
```
grep -r "serial\|duid\|firmware\|deviceInfo\|getDeviceName" eufymake_decompiled/sources/ --include="*.java" | grep -i "ble\|bluetooth\|gatt"
grep -r "DEVICE_INFO\|DEVICE_NAME\|FIRMWARE" eufymake_decompiled/sources/ --include="*.java"
```

### Findings
- [ ] Serial number format: [describe]
- [ ] DUID format: [describe — how many bytes? hex string?]
- [ ] Firmware version format: [describe]
- [ ] How DUID maps to PPPP DUID: [same? transformed?]

---

## 5. Wi-Fi Configuration Protocol

> How does the app send Wi-Fi credentials to the printer?

### Wi-Fi Credential Format
- [ ] SSID encoding: [UTF-8 / ASCII / length-prefixed]
- [ ] Password encoding: [UTF-8 / ASCII / length-prefixed]
- [ ] Security type sent: [WPA2 / WPA3 / WEP / open / auto]
- [ ] Hidden network support: [yes/no]
- [ ] Multiple credentials: [yes/no]

### Wi-Fi Connection Flow
```
1. App writes SSID to [UUID]
2. App writes password to [UUID]
3. App writes security type to [UUID] (or combined with above)
4. App sends "connect" command to [UUID]
5. Printer attempts Wi-Fi connection
6. Printer notifies status on [notify UUID]:
   - [status code for success]
   - [status code for wrong password]
   - [status code for network not found]
   - [status code for DHCP failure]
7. On success, printer sends [what? IP address? DUID?]
```

### Where to find in APK
Search for:
```
grep -r "ssid\|wifi\|password\|wpa\|WLAN" eufymake_decompiled/sources/ --include="*.java" | grep -i "ble\|bluetooth\|gatt\|setup\|pair"
grep -r "WifiConfiguration\|ScanResult" eufymake_decompiled/sources/ --include="*.java"
```

### Findings
- [ ] Wi-Fi credential format documented: [describe]
- [ ] Status codes mapped: [list]
- [ ] Can we send Wi-Fi config via our own BLE client? [yes/no]

---

## 6. PPPP / LAN Transition

> After BLE pairing, how does the app transition to PPPP/LAN?

### Transition Flow
```
1. BLE pairing complete → printer has Wi-Fi credentials
2. Printer connects to Wi-Fi → gets IP address
3. Printer [broadcasts? notifies app via BLE? sends to cloud?]
4. App discovers printer on LAN via [PPPP LAN search / mDNS / UDP broadcast]
5. App uses DUID + keys from BLE to establish PPPP connection
```

### Key Data Needed for PPPP
- [ ] DUID: [obtained from BLE? from cloud? hardcoded?]
- [ ] PPPP encryption key: [same as BLE key? different? derived?]
- [ ] AES key: [where does it come from?]
- [ ] License key / auth token: [needed?]

### Where to find in APK
Search for:
```
grep -r "pppp\|PPPP\|lan_search\|LanSearch\|duid\|DUID" eufymake_decompiled/sources/ --include="*.java"
grep -r "PPPPApi\|P2PApi\|p2p" eufymake_decompiled/sources/ --include="*.java"
```

### Findings
- [ ] DUID source after BLE: [describe]
- [ ] PPPP key relationship to BLE: [same/different/derived]
- [ ] Can we go straight from BLE to PPPP without cloud? [yes/no]

---

## 7. Cloud / MQTT Registration

> Does the app need to register the printer with the cloud after BLE pairing?

### Registration Flow
```
1. After BLE + Wi-Fi, app calls [which API endpoint?]
2. Sends: [what data? DUID? serial? user token?]
3. Cloud returns: [what? MQTT credentials? encryption keys?]
4. App stores: [what locally?]
```

### Where to find in APK
Search for:
```
grep -r "register\|bindDevice\|addPrinter\|passport" eufymake_decompiled/sources/ --include="*.java"
grep -r "mqtt\|MQTT" eufymake_decompiled/sources/ --include="*.java" -l
grep -r "api.*v1\|api.*v2\|eufy" eufymake_decompiled/sources/ --include="*.java" | grep -i "http\|url\|endpoint"
```

### Findings
- [ ] Cloud registration required for PPPP? [yes/no]
- [ ] Can we skip cloud and go BLE → PPPP directly? [yes/no]
- [ ] What data does cloud give us that BLE doesn't? [list]

---

## 8. Native Libraries (.so files)

> Some crypto or protocol logic may be in native code.

### Native Libraries Found
| File | Architecture | Purpose | Reversed? |
|------|-------------|---------|-----------|
| [e.g. libanker.so] | arm64-v8a | [crypto / BLE / PPPP] | [yes/no] |
| [e.g. libpppp.so] | armeabi-v7a | [PPPP protocol] | [yes/no] |

### Analysis
```bash
# List exported symbols
nm -D eufymake_apktool/lib/arm64-v8a/lib*.so | grep -i "encrypt\|key\|ble\|pair"

# Extract strings
strings eufymake_apktool/lib/arm64-v8a/lib*.so | grep -i "uuid\|service\|char"

# Disassemble with Ghidra or IDA
# [notes from disassembly]
```

### Findings
- [ ] Native libs identified: [list]
- [ ] Crypto functions found: [describe]
- [ ] BLE protocol in native or Java? [which?]

---

## 9. Class Map

> Key classes from the decompiled APK and what they do.

| Class | Package | Purpose |
|-------|---------|---------|
| [e.g. BleManager.java] | com.eufy.ble | BLE connection management |
| [e.g. GattHelper.java] | com.eufy.ble | GATT operations wrapper |
| [e.g. PrinterSetupActivity.java] | com.eufy.printer | Setup/pairing flow UI |
| [e.g. CryptoHelper.java] | com.eufy.crypto | Encryption/decryption |
| [e.g. PpppApi.java] | com.eufy.p2p | PPPP LAN connection |
| [e.g. MqttClient.java] | com.eufy.mqtt | MQTT cloud communication |

### Key Methods to Document
```
BleManager:
  - connect(deviceAddress) → [what happens]
  - writeCharacteristic(uuid, data) → [what format]
  - onCharacteristicChanged(uuid, data) → [what responses]

CryptoHelper:
  - encrypt(plaintext, key) → [algorithm used]
  - decrypt(ciphertext, key) → [algorithm used]
  - deriveKey(...) → [how keys are derived]
```

---

## 10. Raw Captures

> If you've done a live BLE capture (HCI snoop log), document it here.

### Capture Info
- **Date**: [when captured]
- **Phone**: [Android model/version]
- **Printer**: [M5 or M5C, firmware version]
- **Capture file**: [filename — don't commit to public repo]

### Wireshark Filters Used
```
# Filter for ATT (GATT) operations
btatt

# Filter for specific opcode (write request = 0x12, write command = 0x52)
btatt.opcode == 0x12

# Filter for specific handle
btatt.handle == 0x002a
```

### Key Packets
| Packet # | Direction | Handle | Data (hex) | Interpretation |
|-----------|-----------|--------|------------|----------------|
| [123] | Phone→Printer | [0x002a] | [hex] | [Wi-Fi SSID write] |
| [456] | Printer→Phone | [0x003b] | [hex] | [Status notify: success] |

---

## 11. Open Questions

- [ ] Is the BLE protocol the same for M5 and M5C?
- [ ] Does firmware version change the BLE protocol?
- [ ] Are there any anti-tampering / root detection checks in the app?
- [ ] Is there certificate pinning on the HTTP API calls?
- [ ] Can we use frida to hook the live app and dump BLE payloads?
- [ ] Does the app check for a signed/verified BLE pairing state?

---

## 12. Progress Tracker

| Task | Status | Notes |
|------|--------|-------|
| Download APK | [done/pending] | [version] |
| Decompile with jadx | [done/pending] | |
| Find BLE service UUIDs | [done/pending] | |
| Find characteristic UUIDs | [done/pending] | |
| Document GATT flow | [done/pending] | |
| Identify encryption | [done/pending] | |
| Document Wi-Fi exchange | [done/pending] | |
| Document device info exchange | [done/pending] | |
| Map BLE→PPPP transition | [done/pending] | |
| Analyse native libs | [done/pending] | |
| Live BLE capture | [done/pending] | |
| Implement in Go | [done/pending] | |
| Test pairing without app | [done/pending] | |

---

## Notes

[Add any additional notes, observations, or dead ends here.]
