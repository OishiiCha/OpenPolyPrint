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

# Check the remaining file's structure
print("=== PETG (AI Optimized) - section headers ===\n")
run("curl -sk https://localhost/api/profile-files/pf_1788305515179945415 2>&1 | grep -E '^#|^\\['", timeout=30)

print("\n=== First 5 lines ===\n")
run("curl -sk https://localhost/api/profile-files/pf_1788305515179945415 2>&1 | head -5", timeout=30)

print("\n=== Last 10 lines ===\n")
run("curl -sk https://localhost/api/profile-files/pf_1788305515179945415 2>&1 | tail -10", timeout=30)

print("\n=== Presets section ===\n")
run("curl -sk https://localhost/api/profile-files/pf_1788305515179945415 2>&1 | grep -A 6 '^\\[presets\\]'", timeout=30)

client.close()
print("DONE")
