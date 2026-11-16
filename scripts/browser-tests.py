#!/usr/bin/env python3
"""Aceptación visual con Go y PostgreSQL reales en una base desechable."""
import json
import os
from pathlib import Path
import runpy
import secrets
import shutil
import subprocess
import time
import urllib.request

ROOT = Path(__file__).resolve().parent.parent

# Servidor temporal que comparten las especificaciones y, desde la etapa 9H (fase D), un segundo servidor con su
# propia base limpia para la importación completa del sistema anterior (solo lo usa
# workflow-stage9h-migration.spec.ts, por ADEL_BROWSER_CLEAN_SERVER_URL).
SHARED_PORT = 8183
CLEAN_PORT = 8185


def prepare_server(port):
    """Crea la base y la carpeta temporal de un servidor y escribe su configuración de pruebas."""
    name = "adel_pos_test_browser_" + secrets.token_hex(6)
    directory = ROOT / ".local" / name
    directory.mkdir(mode=0o700)
    config = json.loads((ROOT / ".local/server.json").read_text())
    config["database"]["name"] = name
    config["listen_address"] = f"127.0.0.1:{port}"
    config["allow_lan"] = False
    # Etapa 9F: modo de pruebas (el servidor se niega a arrancar si la base no es adel_pos_test_* y lo
    # informa a native-driver.ts) y respaldos de «Quitar datos de prueba» dentro de la carpeta temporal.
    config["test_mode"] = True
    config["backup_directory"] = str(directory / "backups")
    filename = directory / "server.json"
    with open(filename, "x", opener=lambda p, f: os.open(p, f, 0o600)) as output:
        json.dump(config, output)
    return {"name": name, "directory": directory, "config": filename, "port": port}


def start_server(server):
    for command in ("migrate", "prepare-setup"):
        subprocess.run(
            [str(ROOT / "bin/ad-el-pos-server"), command, "--config", str(server["config"])],
            cwd=ROOT,
            check=True,
        )
    log = (server["directory"] / "server.log").open("w")
    server["log"] = log
    server["process"] = subprocess.Popen(
        [str(ROOT / "bin/ad-el-pos-server"), "serve", "--config", str(server["config"])],
        cwd=ROOT,
        stdout=log,
        stderr=log,
    )
    for attempt in range(100):
        try:
            with urllib.request.urlopen(
                f"http://127.0.0.1:{server['port']}/api/v1/health/ready", timeout=1
            ) as response:
                if response.status == 200:
                    return
        except OSError:
            time.sleep(0.1)
    raise RuntimeError("El servidor temporal no quedó listo")


def run_browser(engine):
    database = runpy.run_path(str(ROOT / "scripts/database.py"))
    database["ensure_owned"]()
    servers = []
    try:
        for port in (SHARED_PORT, CLEAN_PORT):
            server = prepare_server(port)
            servers.append(server)
            database["run"](
                [database["PG_BIN"] / "createdb", "--owner=adel_app", server["name"]],
                env=database["admin_env"](),
            )
            server["created"] = True
        subprocess.run([str(ROOT / "scripts/dev.sh"), "build-server"], cwd=ROOT, check=True)
        for server in servers:
            start_server(server)
        shared, clean = servers
        environment = dict(
            os.environ,
            ADEL_BROWSER_ENGINE=engine,
            ADEL_BROWSER_PROVISIONING=str(shared["directory"] / "provisioning-token"),
            ADEL_BROWSER_CLEAN_SERVER_URL=f"http://127.0.0.1:{CLEAN_PORT}",
            ADEL_BROWSER_CLEAN_PROVISIONING=str(clean["directory"] / "provisioning-token"),
            ADEL_BROWSER_SERVER_BINARY=str(ROOT / "bin/ad-el-pos-server"),
            PLAYWRIGHT_BROWSERS_PATH=str(ROOT / ".cache/playwright"),
        )
        subprocess.run(
            ["npm", "run", "test:browser", "--", "--project", engine],
            cwd=ROOT / "desktop",
            env=environment,
            check=True,
        )
    finally:
        for server in servers:
            process = server.get("process")
            if process is not None:
                process.terminate()
                process.wait(timeout=15)
            if server.get("log") is not None:
                server["log"].close()
            if server.get("created"):
                database["run"](
                    [database["PG_BIN"] / "dropdb", server["name"]],
                    env=database["admin_env"](),
                )
            shutil.rmtree(server["directory"])


for engine in ("chromium", "webkit"):
    run_browser(engine)
