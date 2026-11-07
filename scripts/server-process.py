#!/usr/bin/env python3
"""Control de proceso solo para desarrollo, independiente de la ventana Tauri."""
import json
import os
from pathlib import Path
import signal
import subprocess
import sys
import time
import urllib.request

ROOT = Path(__file__).resolve().parent.parent
LOCAL = ROOT / '.local'
PIDFILE = LOCAL / 'server.pid'
BINARY = ROOT / 'bin/ad-el-pos-server'

def owned_pid():
    if not PIDFILE.exists(): return None
    pid = int(PIDFILE.read_text())
    result = subprocess.run(['ps', '-p', str(pid), '-o', 'comm='], capture_output=True, text=True)
    if result.returncode or not result.stdout.strip(): return None
    if result.stdout.strip() != str(BINARY):
        raise SystemExit('El PID registrado pertenece a otro proceso. No se enviará ninguna señal.')
    return pid

def start():
    if owned_pid():
        print('El servidor del proyecto ya está activo.'); return
    LOCAL.mkdir(mode=0o700, exist_ok=True)
    filename = Path(os.environ.get('ADEL_SERVER_CONFIG', LOCAL / 'server.json')).resolve()
    config = json.loads(filename.read_text())
    address = config['listen_address']
    with (LOCAL / 'server.log').open('ab') as log:
        process = subprocess.Popen([str(BINARY), 'serve', '--config', str(filename)], cwd=ROOT, stdin=subprocess.DEVNULL, stdout=log, stderr=log, start_new_session=True)
    PIDFILE.write_text(str(process.pid))
    for _ in range(40):
        if process.poll() is not None:
            PIDFILE.unlink(missing_ok=True)
            raise SystemExit('Go no pudo iniciar. Revisa .local/server.log.')
        try:
            with urllib.request.urlopen(f'http://{address}/api/v1/health/live', timeout=.3) as response:
                if response.status == 200:
                    time.sleep(.15)
                    if process.poll() is not None: continue
                    print(f'Servidor independiente iniciado (PID {process.pid}). Log: .local/server.log'); return
        except (OSError, ValueError): pass
        time.sleep(.1)
    raise SystemExit('El proceso sigue activo, pero no se pudo verificar HTTP. Revisa .local/server.log.')

def stop():
    pid = owned_pid()
    if pid is None:
        print('No hay un servidor activo registrado por este script.'); return
    os.kill(pid, signal.SIGTERM)
    for _ in range(60):
        if owned_pid() is None:
            PIDFILE.unlink(missing_ok=True)
            print('Servidor detenido de forma ordenada.'); return
        time.sleep(.1)
    raise SystemExit('No se confirmó el cierre. No se forzará la terminación; revisa el proceso.')

if __name__ == '__main__':
    if sys.argv[1] == 'start': start()
    elif sys.argv[1] == 'stop': stop()
    elif sys.argv[1] == 'status': print(f'Servidor: {owned_pid() or "detenido"}')
    else: raise SystemExit('Comando no válido.')
