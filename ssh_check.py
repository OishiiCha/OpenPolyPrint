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

# Get the presets section
run("curl -sk https://localhost/api/profile-files/pf_1788083988828603412 2>&1 | grep -A 20 '^\\[presets\\]'", timeout=30)

print("\n=== FULL FILE FIRST/LAST LINES ===\n")
run("curl -sk https://localhost/api/profile-files/pf_1788083988828603412 2>&1 | head -5", timeout=30)
run("curl -sk https://localhost/api/profile-files/pf_1788083988828603412 2>&1 | tail -20", timeout=30)

print("\n=== AI OPTIMIZED FILE (extracted) ===\n")
# Check what the AI optimized file looks like
run("curl -sk https://localhost/api/profile-files/pf_1788257621793410642 2>&1 | head -30", timeout=30)
run("curl -sk https://localhost/api/profile-files/pf_1788257621793410642 2>&1 | tail -10", timeout=30)

client.close()
print("DONE")
