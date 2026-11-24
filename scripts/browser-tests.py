#!/usr/bin/env python3
"""Aceptación visual con Go y PostgreSQL reales en una base desechable."""
import json
import os
from pathlib import Path
import runpy
import secrets
import shutil
import subprocess
import tempfile
import time
import urllib.request

ROOT = Path(__file__).resolve().parent.parent

# Servidor temporal que comparten las especificaciones y, desde la etapa 9H (fase D), un segundo servidor con su
# propia base limpia para la importación completa del sistema anterior (solo lo usa
# workflow-stage9h-migration.spec.ts, por ADEL_BROWSER_CLEAN_SERVER_URL).
SHARED_PORT = 8183
CLEAN_PORT = 8185
# Etapa L1 (Fase E): un tercer servidor con su base limpia y **sin activar** (solo lo usa
# workflow-stagel1-activation.spec.ts, por ADEL_BROWSER_UNLICENSED_SERVER_URL) y el servidor de licencias simulado de
# la orden de pruebas license-test (Fase B), con TLS y solo en loopback. Nunca un servidor de licencias real: los tres
# servidores temporales apuntan al simulado y confían solo en las llaves de prueba de vectors.json.
UNLICENSED_PORT = 8187
LICENSE_PORT = 8189
LICENSE_TRUST = ROOT / "contracts/licencias/v1/fixtures/vectors.json"


def prepare_server(port, license_url):
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
    # Etapa L1: el servidor de licencias es el simulado (en test_mode, el cliente rechaza cualquier dirección que no
    # sea de loopback).
    config["license_server_url"] = license_url
    filename = directory / "server.json"
    with open(filename, "x", opener=lambda p, f: os.open(p, f, 0o600)) as output:
        json.dump(config, output)
    return {"name": name, "directory": directory, "config": filename, "port": port}


def start_server(server, license_ca):
    for command in ("migrate", "prepare-setup"):
        subprocess.run(
            [str(ROOT / "bin/ad-el-pos-server"), command, "--config", str(server["config"])],
            cwd=ROOT,
            check=True,
        )
    # Etapa L1: ayudas de prueba de la licencia (solo con test_mode y en una compilación de desarrollo): confiar en las
    # llaves de vectors.json (las licencias de prueba de license-test), el certificado del simulador y un reloj
    # simulado por servidor (un desfase en un archivo; nunca se cambia la hora de la computadora).
    server["clock"] = server["directory"] / "reloj"
    server["clock"].write_text("")
    environment = dict(
        os.environ,
        ADEL_TEST_LICENSE_TRUST=str(LICENSE_TRUST),
        ADEL_TEST_LICENSE_CA=str(license_ca),
        ADEL_TEST_CLOCK_FILE=str(server["clock"]),
    )
    log = (server["directory"] / "server.log").open("w")
    server["log"] = log
    server["process"] = subprocess.Popen(
        [str(ROOT / "bin/ad-el-pos-server"), "serve", "--config", str(server["config"])],
        cwd=ROOT,
        env=environment,
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


def start_license_simulator(tools):
    """Compila license-test (solo de pruebas, en una carpeta temporal) y arranca su servidor de licencias simulado."""
    tool = tools / "license-test"
    subprocess.run(
        ["go", "build", "-trimpath", "-o", str(tool), "./cmd/license-test"],
        cwd=ROOT / "backend",
        check=True,
    )
    certificate = tools / "licencias-ca.pem"
    log_path = tools / "license-server.log"
    log = log_path.open("w")
    process = subprocess.Popen(
        [
            str(tool),
            "server",
            "--listen",
            f"127.0.0.1:{LICENSE_PORT}",
            "--ca-out",
            str(certificate),
            "--fixtures",
            str(LICENSE_TRUST),
        ],
        cwd=ROOT,
        env=dict(os.environ, ADEL_TEST_MODE="1"),
        stdout=log,
        stderr=log,
    )
    simulator = {"tool": tool, "certificate": certificate, "process": process, "log": log}
    for attempt in range(150):
        if '"event":"ready"' in log_path.read_text():
            simulator["url"] = f"https://127.0.0.1:{LICENSE_PORT}"
            return simulator
        if process.poll() is not None:
            break
        time.sleep(0.1)
    raise RuntimeError("El servidor de licencias simulado no quedó listo")


def run_browser(engine):
    database = runpy.run_path(str(ROOT / "scripts/database.py"))
    database["ensure_owned"]()
    servers = []
    simulator = None
    tools = Path(tempfile.mkdtemp(prefix="adel-license-test-"))
    license_url = f"https://127.0.0.1:{LICENSE_PORT}"
    try:
        for port in (SHARED_PORT, CLEAN_PORT, UNLICENSED_PORT):
            server = prepare_server(port, license_url)
            servers.append(server)
            database["run"](
                [database["PG_BIN"] / "createdb", "--owner=adel_app", server["name"]],
                env=database["admin_env"](),
            )
            server["created"] = True
        subprocess.run([str(ROOT / "scripts/dev.sh"), "build-server"], cwd=ROOT, check=True)
        # El simulador va antes que los servidores: necesitan su certificado al arrancar.
        simulator = start_license_simulator(tools)
        for server in servers:
            start_server(server, simulator["certificate"])
        shared, clean, unlicensed = servers
        environment = dict(
            os.environ,
            ADEL_BROWSER_ENGINE=engine,
            ADEL_BROWSER_PROVISIONING=str(shared["directory"] / "provisioning-token"),
            ADEL_BROWSER_CLEAN_SERVER_URL=f"http://127.0.0.1:{CLEAN_PORT}",
            ADEL_BROWSER_CLEAN_PROVISIONING=str(clean["directory"] / "provisioning-token"),
            ADEL_BROWSER_SERVER_BINARY=str(ROOT / "bin/ad-el-pos-server"),
            # Etapa L1 (Fase E): native-driver.ts activa cada base nueva con una licencia de prueba de license-test;
            # las especificaciones de la etapa usan además el simulador, el reloj simulado del servidor compartido y
            # el servidor sin activar.
            ADEL_BROWSER_LICENSE_TOOL=str(simulator["tool"]),
            ADEL_BROWSER_LICENSE_SERVER_URL=simulator["url"],
            ADEL_BROWSER_LICENSE_CA=str(simulator["certificate"]),
            ADEL_BROWSER_CLOCK_FILE=str(shared["clock"]),
            ADEL_BROWSER_UNLICENSED_SERVER_URL=f"http://127.0.0.1:{UNLICENSED_PORT}",
            ADEL_BROWSER_UNLICENSED_PROVISIONING=str(unlicensed["directory"] / "provisioning-token"),
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
        if simulator is not None:
            simulator["process"].terminate()
            simulator["process"].wait(timeout=15)
            simulator["log"].close()
        shutil.rmtree(tools, ignore_errors=True)


for engine in ("chromium", "webkit"):
    run_browser(engine)
