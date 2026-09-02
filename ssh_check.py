import paramiko, time

client = paramiko.SSHClient()
client.set_missing_host_key_policy(paramiko.AutoAddPolicy())
client.connect('192.168.1.39', username='mrit', password='161211', timeout=15)

def run(cmd, timeout=300):
    stdin, stdout, stderr = client.exec_command(cmd, timeout=timeout)
    out = stdout.read().decode('utf-8', errors='replace')
    safe = out.encode('ascii', errors='replace').decode('ascii')
    if safe.strip():
        print(safe)

# Check if build is still running
print("=== Checking if docker build is still running ===\n")
run("ps aux | grep 'docker build' | grep -v grep | head -3", timeout=10)

# Check container status
print("\n=== Container status ===\n")
run("echo '161211' | sudo -S docker ps -a --format '{{.Names}} {{.Status}}' 2>/dev/null", timeout=15)

# If build finished, restart container
print("\n=== Restarting container ===\n")
run("cd /home/mrit/OpenPolyPrint && echo '161211' | sudo -S docker compose -f docker-compose.pi.yaml up -d 2>&1", timeout=60)

print("\n=== Container status after restart ===\n")
run("echo '161211' | sudo -S docker ps --format '{{.Names}} {{.Status}}' 2>/dev/null", timeout=15)

# Check logs
print("\n=== Recent logs ===\n")
run("echo '161211' | sudo -S docker logs openpolyprint --tail 10 2>&1", timeout=15)

client.close()
print("DONE")
