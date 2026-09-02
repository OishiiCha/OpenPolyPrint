# EufyMake Shutdown Contingency Plan

## What Happens If EufyMake Shuts Down?

If Anker/Eufy shuts down the EufyMake cloud service, the following features break:

| Feature | Dependency | Impact |
|---------|-----------|--------|
| Remote control (MQTT) | EufyMake MQTT broker | **Broken** — no remote commands |
| Account auth | EufyMake HTTP API | **Broken** — can't log in |
| Printer registration | EufyMake HTTP API | **Broken** — can't add new printers |
| Firmware updates | EufyMake CDN | **Broken** — no new firmware |
| Push notifications | EufyMake push service | **Broken** — no alerts |
| Camera stream (PPPP) | None (LAN only) | **Works** — if DUID/keys are saved |
| File upload (PPPP) | None (LAN only) | **Works** — if DUID/keys are saved |
| Print control (PPPP) | None (LAN only) | **Works** — if DUID/keys are saved |

## What Still Works (LAN-only mode)

If we have the printer's DUID and encryption keys saved locally:
- PPPP LAN connection (direct UDP to printer IP)
- Camera streaming
- G-code file upload
- Print start/pause/stop
- Temperature monitoring
- All movement controls

## What We Need to Do NOW (Before Shutdown)

1. **Save DUID and keys for every printer**
   - Export from OpenPolyPrint settings
   - Store in a local backup file
   - These are the critical pieces — without them, no LAN connection

2. **Reverse engineer Bluetooth pairing** (see ankermake-m5c-bluetooth-re.md)
   - So we can set up new printers without the app
   - This is the hardest part

3. **Cache firmware versions**
   - Download and store current firmware files
   - In case we need to reflash a printer

4. **Document the PPPP protocol fully**
   - The protocol code is already in OpenPolyPrint
   - Ensure it works without any cloud dependency

5. **Implement LAN-only mode**
   - Skip MQTT entirely
   - Use PPPP for everything (commands, status, camera, file transfer)
   - This is mostly working already

## Priority Order

1. **Critical**: Save DUID/keys for all existing printers
2. **High**: Bluetooth RE for new printer setup
3. **Medium**: LAN-only command path (PPPP instead of MQTT)
4. **Low**: Firmware caching
