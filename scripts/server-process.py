#!/usr/bin/env python3
"""Control de proceso solo para desarrollo, independiente de la ventana Tauri."""
import json
import os
from pathlib import Path
import signal
import ssl
import subprocess
import sys
import time
import urllib.request

ROOT = Path(__file__).resolve().parent.parent
LOCAL = ROOT / ".local"
PIDFILE = LOCAL / "server.pid"
BINARY = ROOT / "bin/ad-el-pos-server"


def owned_pid():
    if not PIDFILE.exists():
        return None
    pid = int(PIDFILE.read_text())
    result = subprocess.run(
        ["ps", "-p", str(pid), "-o", "stat=,comm="], capture_output=True, text=True
    )
    if result.returncode or not result.stdout.strip():
        return None
    state, command = result.stdout.strip().split(None, 1)
    # macOS puede conservar brevemente un zombie tras SIGTERM; ya no es un proceso activo.
    if state.startswith("Z") or "E" in state:
        return None
    if command != str(BINARY):
        raise SystemExit("El PID registrado pertenece a otro proceso. No se enviará ninguna señal.")
    return pid


def start():
    if owned_pid():
        print("El servidor del proyecto ya está activo.")
        return
    LOCAL.mkdir(mode=0o700, exist_ok=True)
    filename = Path(os.environ.get("ADEL_SERVER_CONFIG", LOCAL / "server.json")).resolve()
    config = json.loads(filename.read_text())
    address = config["listen_address"]
    scheme = "https" if config.get("tls_certificate") else "http"
    trust_context = None
    if scheme == "https":
        local_ca = filename.parent / "tls" / "ca.pem"
        trust_context = ssl.create_default_context(
            cafile=str(local_ca) if local_ca.exists() else None
        )
    with (LOCAL / "server.log").open("ab") as log:
        process = subprocess.Popen(
            [str(BINARY), "serve", "--config", str(filename)],
            cwd=ROOT,
            stdin=subprocess.DEVNULL,
            stdout=log,
            stderr=log,
            start_new_session=True,
        )
    PIDFILE.write_text(str(process.pid))
    for _ in range(40):
        if process.poll() is not None:
            PIDFILE.unlink(missing_ok=True)
            raise SystemExit("Go no pudo iniciar. Revisa .local/server.log.")
        try:
            with urllib.request.urlopen(
                f"{scheme}://{address}/api/v1/health/live", timeout=0.3, context=trust_context
            ) as response:
                if response.status == 200:
                    time.sleep(0.15)
                    if process.poll() is not None:
                        continue
                    print(
                        f"Servidor independiente iniciado (PID {process.pid}). Log: .local/server.log"
                    )
                    return
        except (OSError, ValueError):
            pass
        time.sleep(0.1)
    raise SystemExit(
        "El proceso sigue activo, pero no se pudo verificar HTTP. Revisa .local/server.log."
    )


def stop():
    pid = owned_pid()
    if pid is None:
        print("No hay un servidor activo registrado por este script.")
        return
    try:
        os.kill(pid, signal.SIGTERM)
    except ProcessLookupError:
        PIDFILE.unlink(missing_ok=True)
        print("El servidor ya terminó.")
        return
    for _ in range(60):
        # Tras validar y señalar el proceso no volvemos a enviar señales.
        # Durante la salida macOS puede perder la ruta de comm antes de liberar el PID.
        state = subprocess.run(
            ["ps", "-p", str(pid), "-o", "stat="], capture_output=True, text=True
        )
        status = state.stdout.strip()
        if state.returncode or not status or status.startswith("Z") or "E" in status:
            PIDFILE.unlink(missing_ok=True)
            print("Servidor detenido de forma ordenada.")
            return
        time.sleep(0.1)
    raise SystemExit("No se confirmó el cierre. No se forzará la terminación; revisa el proceso.")


if __name__ == "__main__":
    if sys.argv[1] == "start":
        start()
    elif sys.argv[1] == "stop":
        stop()
    elif sys.argv[1] == "status":
        print(f'Servidor: {owned_pid() or "detenido"}')
    else:
        raise SystemExit("Comando no válido.")
