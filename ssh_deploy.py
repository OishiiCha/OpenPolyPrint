import paramiko
import sys
import time

client = paramiko.SSHClient()
client.set_missing_host_key_policy(paramiko.AutoAddPolicy())
client.connect('192.168.1.39', username='mrit', password='161211', timeout=15)

def run(cmd, timeout=600):
    stdin, stdout, stderr = client.exec_command(cmd, timeout=timeout, get_pty=True)
    # Read all output, filter problematic chars
    all_out = b""
    while True:
        chunk = stdout.channel.recv(4096)
        if not chunk:
            break
        all_out += chunk
    text = all_out.decode('utf-8', errors='replace')
    # Force ASCII-safe
    safe = text.encode('ascii', errors='replace').decode('ascii')
    # Filter out password lines
    for line in safe.split('\n'):
        if line.strip() and '161211' not in line:
            print(line, flush=True)
    print(flush=True)

# 1. Pull latest code
run("cd /home/mrit/OpenPolyPrint && git pull origin main 2>&1", timeout=60)

# 2. Stop the container
run("echo '161211' | sudo -S docker stop openpolyprint 2>&1", timeout=30)

# 3. Rebuild the image
print(">>> Building Docker image (may take several minutes)...", flush=True)
run("cd /home/mrit/OpenPolyPrint && echo '161211' | sudo -S docker compose -f docker-compose.pi.yaml build --no-cache 2>&1", timeout=900)

# 4. Start the container
run("cd /home/mrit/OpenPolyPrint && echo '161211' | sudo -S docker compose -f docker-compose.pi.yaml up -d 2>&1", timeout=60)

# 5. Wait and check
time.sleep(8)
run("echo '161211' | sudo -S docker ps -a --format 'table {{.Names}}\\t{{.Status}}' 2>&1", timeout=30)
run("echo '161211' | sudo -S docker logs --tail 10 openpolyprint 2>&1", timeout=30)

client.close()
print("DONE", flush=True)
