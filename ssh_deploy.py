import paramiko
import sys
import time

client = paramiko.SSHClient()
client.set_missing_host_key_policy(paramiko.AutoAddPolicy())
client.connect('192.168.1.39', username='mrit', password='161211', timeout=15)

def run(cmd, timeout=600):
    print(f">>> {cmd}", flush=True)
    stdin, stdout, stderr = client.exec_command(cmd, timeout=timeout, get_pty=True)
    # Stream output
    while True:
        chunk = stdout.channel.recv(4096)
        if not chunk:
            break
        try:
            text = chunk.decode('utf-8', errors='replace')
        except:
            text = chunk.decode('ascii', errors='replace')
        # Filter out sudo password echo
        for line in text.split('\n'):
            if line.strip() and '161211' not in line:
                print(line, flush=True)
    print(flush=True)

# 1. Pull latest code
run("cd /home/mrit/OpenPolyPrint && git pull origin main 2>&1", timeout=60)

# 2. Stop the container
run("echo '161211' | sudo -S docker stop openpolyprint 2>&1", timeout=30)

# 3. Rebuild the image (no cache to ensure latest code)
print(">>> Building Docker image (this may take several minutes)...", flush=True)
run("cd /home/mrit/OpenPolyPrint && echo '161211' | sudo -S docker compose -f docker-compose.pi.yaml build --no-cache 2>&1", timeout=900)

# 4. Start the container
run("cd /home/mrit/OpenPolyPrint && echo '161211' | sudo -S docker compose -f docker-compose.pi.yaml up -d 2>&1", timeout=60)

# 5. Wait and check status
time.sleep(8)
run("echo '161211' | sudo -S docker ps -a --format 'table {{.Names}}\\t{{.Status}}' 2>&1", timeout=30)
run("echo '161211' | sudo -S docker logs --tail 15 openpolyprint 2>&1", timeout=30)

client.close()
print("DONE", flush=True)
