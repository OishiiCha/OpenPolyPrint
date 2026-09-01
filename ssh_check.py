import paramiko
import sys

client = paramiko.SSHClient()
client.set_missing_host_key_policy(paramiko.AutoAddPolicy())

try:
    client.connect('192.168.1.39', username='mrit', password='161211', timeout=15)
except Exception as e:
    print(f"SSH connection failed: {e}")
    sys.exit(1)

commands = [
    "echo '161211' | sudo -S docker ps -a --format 'table {{.Names}}\\t{{.Status}}\\t{{.Image}}' 2>&1",
    "echo '=== CONTAINER LOGS (last 80 lines) ==='",
    "echo '161211' | sudo -S docker logs --tail 80 openpolyprint 2>&1",
]

for cmd in commands:
    stdin, stdout, stderr = client.exec_command(cmd, timeout=30)
    out = stdout.read().decode('utf-8', errors='replace')
    err = stderr.read().decode('utf-8', errors='replace')
    if out:
        print(out)
    if err:
        print(err)

client.close()
