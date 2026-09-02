import paramiko

client = paramiko.SSHClient()
client.set_missing_host_key_policy(paramiko.AutoAddPolicy())
client.connect('192.168.1.39', username='mrit', password='161211', timeout=15)

def run(cmd, timeout=30):
    stdin, stdout, stderr = client.exec_command(cmd, timeout=timeout)
    out = stdout.read().decode('utf-8', errors='replace')
    safe = out.encode('ascii', errors='replace').decode('ascii')
    if safe.strip():
        print(safe)

ORIG_ID = "pf_1788245236563261456"

# Check printer_notes and related fields
print("=== Printer notes field ===\n")
run(f"curl -sk https://localhost/api/profile-files/{ORIG_ID} 2>&1 | grep -A 80 '^\\[printer:0.4mm Text\\]' | grep -E 'printer_notes|printer_model|printer_variant|inherits|preset|name =|model'", timeout=30)

print("\n=== Full printer section (first 30 lines) ===\n")
run(f"curl -sk https://localhost/api/profile-files/{ORIG_ID} 2>&1 | grep -A 30 '^\\[printer:0.4mm Text\\]'", timeout=30)

print("\n=== Print profile notes field ===\n")
run(f"curl -sk https://localhost/api/profile-files/{ORIG_ID} 2>&1 | grep -B2 -A2 'notes = PRINT'", timeout=30)

print("\n=== Filament notes/compatible fields ===\n")
run(f"curl -sk https://localhost/api/profile-files/{ORIG_ID} 2>&1 | grep -A 80 '^\\[filament:Lucas PETG\\]' | grep -E 'notes|compatible|inherits|filament_type|temperature|bed_temperature' | head -15", timeout=30)

# Check what the extracted file looks like
print("\n=== Extracted PETG AI file - sections and key fields ===\n")
run("curl -sk https://localhost/api/profile-files/pf_1788330176443528649 2>&1 | grep -E '^\\[|inherits|compatible_printers|printer_notes|notes =|settings_id'", timeout=30)

client.close()
print("DONE")
