# AnkerMake M5 / M5C — Bluetooth Initial Connection Reverse Engineering

## Motivation

EufyMake (Anker's cloud service) could shut down at any time. If it does, we lose:
- MQTT cloud relay (remote commands)
- HTTP API (printer registration, file list, account auth)
- Push notifications
- Firmware update delivery

The **Bluetooth initial connection** is the critical first step that pairs a phone to the printer. If we can replicate this, we can:
- Set up printers without the EufyMake app
- Establish a direct local connection that doesn't depend on the cloud
- Get the P2P DUID and encryption keys needed for PPPP LAN communication
- Future-proof against EufyMake shutdown

## Goal

Reverse engineer the Bluetooth pairing protocol used by the AnkerMake M5 / M5C so that OpenPolyPrint can perform the initial setup without the EufyMake mobile app.

---

## What We Know

### Bluetooth Role
- The M5/M5C has a Bluetooth Low Energy (BLE) module used for **initial setup only**
- After pairing, the app switches to Wi-Fi/LAN (PPPP) or cloud (MQTT)
- Bluetooth is used to:
  1. Discover the printer advertising as a BLE peripheral
  2. Exchange Wi-Fi credentials (SSID + password)
  3. Exchange encryption keys / device pairing info
  4. Get the printer's serial number, DUID, and firmware version
  5. Trigger the printer to connect to Wi-Fi

### Hardware
- M5 and M5C both have BLE chips (likely ESP32 or similar)
- The printer advertises on BLE after being put into "setup mode" (hold the knob/button)
- Service UUIDs and characteristic UUIDs are unknown — need to capture

---

## Plan: Phone-Based Bluetooth Log Capture

### Phase 1: Capture Bluetooth Traffic

**Tools needed:**
- Android phone with **Wireshark + btatt** or **nRF Connect for Mobile** (Nordic)
- Alternatively: **Bluetooth HCI snoop log** via Android Developer Options
- AnkerMake M5 or M5C printer in setup mode
- EufyMake app installed and logged in

**Steps:**
1. Enable Bluetooth HCI snoop logging on Android:
   - Settings → Developer Options → Enable Bluetooth HCI snoop log
   - Or: `adb shell setprop persist.bluetooth.btsnooplog true`
2. Put printer in Bluetooth setup mode (hold knob/button until LED flashes)
3. Open nRF Connect and scan — note the device name, MAC, and advertised service UUIDs
4. Connect to the printer in nRF Connect
5. Enumerate all services and characteristics
6. **Screenshot** the full service/characteristic tree
7. Disconnect from nRF Connect
8. Now use the **EufyMake app** to do the actual pairing:
   - Start HCI snoop capture: `adb shell bugreport bugreport.zip` (contains btsnoop_hci.log)
   - Or use Wireshark with live HCI capture
   - Open EufyMake app → Add Printer → follow full setup flow
   - Wait for pairing to complete (printer connects to Wi-Fi)
9. Stop capture and pull the snoop log

### Phase 2: Analyse the Capture

1. Open `btsnoop_hci.log` in Wireshark
2. Filter for `btatt` (Bluetooth ATT protocol) to see GATT reads/writes
3. Map out the conversation:
   - Which characteristics are written to?
   - What data is sent? (Wi-Fi SSID, password, keys?)
   - What data is read back? (DUID, serial, firmware version?)
   - Are there any notifications/indications?
4. Look for:
   - **Service UUIDs** — identify the custom Anker service
   - **Write characteristics** — where the phone sends data
   - **Notify characteristics** — where the printer sends responses
   - **Encryption** — is the payload plaintext or encrypted? If encrypted, what algorithm?
5. Document the full protocol flow as a sequence diagram

### Phase 3: Reproduce in OpenPolyPrint

1. Implement a BLE scanner in Go (use `github.com/go-ble/ble` or similar)
2. Replicate the GATT service discovery
3. Replicate the write/read sequence captured in Phase 2
4. Test: can we pair a fresh printer without the EufyMake app?
5. Extract: DUID, encryption keys, serial number, firmware version
6. Feed these into the existing PPPP LAN connection flow

---

## Key Questions to Answer

- [ ] What BLE service UUID does the M5/M5C advertise?
- [ ] What are the characteristic UUIDs for write/notify?
- [ ] Is the Wi-Fi credential exchange encrypted or plaintext?
- [ ] What encryption keys are exchanged during pairing?
- [ ] Does the printer return its DUID over BLE, or is that fetched later via Wi-Fi?
- [ ] Is there a pairing PIN / OOB exchange?
- [ ] Does the protocol differ between M5 and M5C?
- [ ] Can we trigger setup mode programmatically, or is the button press required?

---

## Risks & Considerations

- **Encryption**: If the BLE payload is encrypted with a key derived from the app, we need to reverse the app too
- **Firmware updates**: Anker could change the BLE protocol in a firmware update
- **Legal**: Reverse engineering for interoperability is generally legal, but check local laws
- **BLE stack differences**: iOS and Android may behave differently; capture from Android first (easier HCI access)
- **Alternative**: If BLE RE proves too hard, we can pre-pair printers with the EufyMake app now and save the DUID/keys for later use

---

## Alternative Approach: App Decompilation

If Bluetooth capture is insufficient, we can also decompile the EufyMake APK:
1. Download the EufyMake APK from APKMirror or similar
2. Decompile with `jadx` or `apktool`
3. Search for BLE-related classes (`BluetoothGatt`, `writeCharacteristic`, service UUIDs)
4. Find the encryption logic if payloads are encrypted
5. This gives us the protocol without needing a live capture

**Status: IN PROGRESS** — see [APK Decompilation Findings](apk-decompilation-findings.md) for the detailed template and log of what we've found.

---

## Timeline

| Phase | Effort | Status |
|-------|--------|--------|
| Phase 1: Capture | 1-2 hours (need physical printer + phone) | Not started |
| Phase 2: Analyse | 2-4 hours (Wireshark analysis) | Not started |
| Phase 3: Implement | 1-2 days (Go BLE implementation) | Not started |
| App decompilation | 1-2 hours (parallel with Phase 1) | Not started |

---

## Notes

- The M5C may use a different BLE chip than the M5 — capture both if possible
- Save the HCI snoop log to this repo (or a private fork) for reference
- If we can get the BLE pairing working, OpenPolyPrint becomes fully cloud-independent for initial setup
- This is the single most important step for long-term viability if EufyMake shuts down
