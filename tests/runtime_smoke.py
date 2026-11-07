#!/usr/bin/env python3
"""Prueba opt-in sobre los procesos de desarrollo propios; deja DB/Go/app activos.

Interrumpe temporalmente DB y Go. No ejecutar durante trabajo con datos reales.
"""
import json
import os
from pathlib import Path
import subprocess
import sys
import time
import urllib.error
import urllib.request

ROOT = Path(__file__).resolve().parent.parent
LOCAL = ROOT / '.local'
APP = ROOT / 'desktop/src-tauri/target/release/bundle/macos/AD-EL POS.app'
REPORT = {}

def dev(action):
    subprocess.run([str(ROOT / 'scripts/dev.sh'), action], cwd=ROOT, check=True)

def probe(route='health/ready', port=8181):
    try:
        with urllib.request.urlopen(f'http://127.0.0.1:{port}/api/v1/{route}', timeout=4) as response:
            return response.status, json.load(response)
    except urllib.error.HTTPError as error:
        return error.code, json.load(error)
    except OSError:
        return 0, {}

def ready(port=8181):
    for _ in range(40):
        if probe(port=port)[0] == 200: return
        time.sleep(.25)
    raise AssertionError('No se recuperó la disponibilidad')

def pid():
    return int((LOCAL / 'server.pid').read_text())

def app_pids():
    executable = str(APP / 'Contents/MacOS/ad-el-pos-desktop')
    rows = subprocess.check_output(['ps', '-axo', 'pid=,comm='], text=True).splitlines()
    return [int(row.strip().split(None, 1)[0]) for row in rows if row.strip().split(None, 1)[-1] == executable]

def requests():
    return sum(1 for line in (LOCAL / 'server.log').read_text().splitlines() if '"msg":"http_request"' in line)

def wait_polling():
    before = requests()
    for _ in range(30):
        if requests() >= before + 3: return
        time.sleep(.5)
    raise AssertionError('La interfaz no emitió el ciclo de consultas esperado')

def terminal_mode():
    ready()
    original_pid = pid()
    subprocess.run(['osascript', '-e', 'tell application id "local.adel.pos.dev" to quit'], check=True)
    for _ in range(30):
        if not app_pids(): break
        time.sleep(.2)
    assert not app_pids()
    filename = LOCAL / 'terminal-only.json'
    filename.write_text(json.dumps({'installation_mode': 'terminal-only', 'server_url': 'http://127.0.0.1:8181'}))
    with (LOCAL / 'terminal-only.log').open('wb') as log:
        process = subprocess.Popen([str(APP / 'Contents/MacOS/ad-el-pos-desktop')], env=dict(os.environ, ADEL_TERMINAL_CONFIG=str(filename)), stdin=subprocess.DEVNULL, stdout=log, stderr=log, start_new_session=True)
    try:
        wait_polling()
        assert process.poll() is None and pid() == original_pid
        result = {'mode': 'terminal-only', 'server_pid_unchanged': original_pid, 'result': 'El mismo .app usó configuración externa y consultó Go existente.'}
        (LOCAL / 'terminal-mode-verification.json').write_text(json.dumps(result, indent=2, ensure_ascii=False) + '\n')
        print(json.dumps(result, ensure_ascii=False))
    finally:
        subprocess.run(['osascript', '-e', 'tell application id "local.adel.pos.dev" to quit'], check=True)
        process.wait(timeout=7)
        subprocess.run(['open', '-n', str(APP)], check=True)
        wait_polling()

def main():
    config = json.loads((LOCAL / 'server.json').read_text())
    assert config['environment'] == 'development' and config['listen_address'] == '127.0.0.1:8181'
    assert config['database']['host'] == '127.0.0.1' and config['database']['port'] == 55432
    assert APP.is_dir(), 'Construye el .app antes de esta prueba'
    ready()
    original_pid = pid()
    dev('db-stop')
    try:
        assert probe('health/live')[0] == 200
        code, body = probe()
        assert code == 503 and body['checks']['postgres'] == 'unavailable'
        assert pid() == original_pid
        REPORT['database_outage'] = 'live=200, ready=503, Go conservó su PID'
    finally:
        dev('db-start')
    ready()
    REPORT['database_recovery'] = 'ready=200 sin reiniciar Go'

    config['installation_mode'] = 'server-only'
    config['listen_address'] = '127.0.0.1:8182'
    mode_file = LOCAL / 'server-only-test.json'
    with open(mode_file, 'w', opener=lambda p, f: os.open(p, f, 0o600)) as file:
        json.dump(config, file)
    with (LOCAL / 'server-only-test.log').open('wb') as log:
        process = subprocess.Popen([str(ROOT / 'bin/ad-el-pos-server'), 'serve', '--config', str(mode_file)], stdout=log, stderr=log)
        try:
            ready(8182)
            assert process.poll() is None
            REPORT['server_only'] = 'API y PostgreSQL disponibles en proceso separado :8182'
        finally:
            process.terminate()
            assert process.wait(timeout=7) == 0
            mode_file.unlink()

    if not app_pids():
        subprocess.run(['open', '-n', str(APP)], check=True)
    wait_polling()
    before_close_pid = pid()
    subprocess.run(['osascript', '-e', 'tell application id "local.adel.pos.dev" to quit'], check=True)
    for _ in range(30):
        if not app_pids(): break
        time.sleep(.2)
    assert not app_pids(), 'La aplicación no se cerró'
    assert pid() == before_close_pid and probe()[0] == 200
    REPORT['desktop_closed'] = f'Tauri terminó; Go PID {before_close_pid} continuó y ready=200'
    subprocess.run(['open', '-n', str(APP)], check=True)
    wait_polling()
    assert app_pids() and pid() == before_close_pid
    REPORT['desktop_reopened'] = 'El .app volvió a consultar tres endpoints con el mismo proceso Go'

    dev('server-stop')
    try:
        assert probe('health/live')[0] == 0
        time.sleep(6)
        assert app_pids(), 'Tauri no debe terminar cuando Go está desconectado'
        REPORT['server_outage'] = 'Go detenido; aplicación nativa siguió abierta'
    finally:
        dev('server-start')
    ready()
    wait_polling()
    REPORT['server_recovery'] = 'El .app recuperó consultas sin reiniciarse'
    REPORT['final_server_pid'] = pid()
    REPORT['final_app_pids'] = app_pids()
    (LOCAL / 'runtime-verification.json').write_text(json.dumps(REPORT, indent=2, ensure_ascii=False) + '\n')
    print(json.dumps(REPORT, indent=2, ensure_ascii=False))

if __name__ == '__main__':
    if '--terminal-only' in sys.argv: terminal_mode()
    else: main()
