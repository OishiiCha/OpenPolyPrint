import paramiko
import sys

client = paramiko.SSHClient()
client.set_missing_host_key_policy(paramiko.AutoAddPolicy())
client.connect('192.168.1.39', username='mrit', password='161211', timeout=15)

def run(cmd, timeout=300):
    print(f">>> {cmd}")
    stdin, stdout, stderr = client.exec_command(cmd, timeout=timeout)
    out = stdout.read().decode('utf-8', errors='replace')
    err = stderr.read().decode('utf-8', errors='replace')
    # Replace problematic unicode chars
    out = out.encode('ascii', errors='replace').decode('ascii')
    err = err.encode('ascii', errors='replace').decode('ascii')
    if out:
        print(out)
    if err:
        print(err)
    print()

# Check if the build completed and start the container
run("cd /home/mrit/OpenPolyPrint && echo '161211' | sudo -S docker compose -f docker-compose.pi.yaml up -d 2>&1", timeout=120)

import time
time.sleep(8)

run("echo '161211' | sudo -S docker ps -a --format 'table {{.Names}}\\t{{.Status}}' 2>&1", timeout=30)
run("echo '161211' | sudo -S docker logs --tail 20 openpolyprint 2>&1", timeout=30)

client.close()
print("DONE")
