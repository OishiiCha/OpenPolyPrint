import paramiko
import sys
import time

client = paramiko.SSHClient()
client.set_missing_host_key_policy(paramiko.AutoAddPolicy())
client.connect('192.168.1.39', username='mrit', password='161211', timeout=15)

def run(cmd, timeout=300):
    stdin, stdout, stderr = client.exec_command(cmd, timeout=timeout)
    out = stdout.read().decode('utf-8', errors='replace')
    err = stderr.read().decode('utf-8', errors='replace')
    # Force ASCII-safe output
    safe_out = out.encode('ascii', errors='replace').decode('ascii')
    safe_err = err.encode('ascii', errors='replace').decode('ascii')
    if safe_out.strip():
        print(safe_out)
    if safe_err.strip():
        print(safe_err)

# Check if build is still running
run("echo '161211' | sudo -S docker ps -a --format '{{.Names}} {{.Status}}' 2>&1", timeout=30)

# Try to start the container (if build finished, it'll use the new image)
run("cd /home/mrit/OpenPolyPrint && echo '161211' | sudo -S docker compose -f docker-compose.pi.yaml up -d 2>&1", timeout=120)

time.sleep(8)
run("echo '161211' | sudo -S docker ps -a --format 'table {{.Names}}\\t{{.Status}}' 2>&1", timeout=30)
run("echo '161211' | sudo -S docker logs --tail 15 openpolyprint 2>&1", timeout=30)

client.close()
print("DONE")
