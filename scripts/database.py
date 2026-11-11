#!/usr/bin/env python3
"""Clúster de desarrollo exclusivo. Nunca modifica un clúster ajeno."""
import json
import os
from pathlib import Path
import secrets
import subprocess
import sys
from datetime import datetime, timezone
from urllib.parse import quote

ROOT = Path(__file__).resolve().parent.parent
LOCAL = ROOT / ".local"
DATA = LOCAL / "postgres"
PRODUCT = json.loads((ROOT / "config/product.json").read_text())
DB_NAME = PRODUCT["database_name"]
PG_BIN = Path(
    os.environ.get(
        "ADEL_PG_BIN",
        (
            "/opt/homebrew/opt/postgresql@18/bin"
            if os.uname().machine == "arm64"
            else "/usr/local/opt/postgresql@18/bin"
        ),
    )
)
PORT = "55432"


def run(command, **kwargs):
    return subprocess.run([str(x) for x in command], check=True, **kwargs)


def secret_file(filename):
    if not filename.exists():
        with open(filename, "x", opener=lambda p, f: os.open(p, f, 0o600)) as file:
            file.write(secrets.token_hex(24))
    return filename.read_text().strip()


def admin_env():
    return dict(
        os.environ,
        PGHOST="127.0.0.1",
        PGPORT=PORT,
        PGUSER="adel_dev_admin",
        PGDATABASE="postgres",
        PGPASSWORD=(LOCAL / "admin-password").read_text().strip(),
        PGCONNECT_TIMEOUT="3",
    )


def ensure_owned():
    if not (LOCAL / "cluster-owner.json").exists():
        raise SystemExit(
            "No existe el marcador del clúster exclusivo. No se operará sobre otros datos; ejecuta db-init."
        )
    marker = json.loads((LOCAL / "cluster-owner.json").read_text())
    if marker.get("data_directory") != str(DATA) or marker.get("port") != PORT:
        raise SystemExit("El marcador no coincide con este clúster. Revisión manual necesaria.")


def start():
    ensure_owned()
    if (
        subprocess.run(
            [str(PG_BIN / "pg_ctl"), "-D", str(DATA), "status"], stdout=subprocess.DEVNULL
        ).returncode
        == 0
    ):
        print("PostgreSQL del proyecto ya está activo.")
        return
    run([PG_BIN / "pg_ctl", "-D", DATA, "-l", LOCAL / "postgres.log", "-w", "start"])


def init():
    if not (PG_BIN / "initdb").exists():
        raise SystemExit(
            "Falta PostgreSQL. Instala postgresql@18 o define ADEL_PG_BIN; consulta DEVELOPMENT_MACOS.md."
        )
    LOCAL.mkdir(mode=0o700, exist_ok=True)
    if DATA.exists():
        ensure_owned()
        if not (DATA / "PG_VERSION").exists():
            raise SystemExit(
                "Inicialización anterior incompleta. No se borrará el directorio; revisa .local/postgres."
            )
    else:
        secret_file(LOCAL / "admin-password")
        (LOCAL / "cluster-owner.json").write_text(
            json.dumps({"data_directory": str(DATA), "port": PORT})
        )
        run(
            [
                PG_BIN / "initdb",
                "-D",
                DATA,
                "--username=adel_dev_admin",
                "--auth=scram-sha-256",
                "--encoding=UTF8",
                "--locale=C",
                "--pwfile",
                LOCAL / "admin-password",
            ]
        )
        socket_dir = LOCAL / "socket"
        socket_dir.mkdir(mode=0o700, exist_ok=True)
        escaped = str(socket_dir).replace("'", "''")
        with (DATA / "postgresql.conf").open("a") as file:
            file.write(
                f"\nlisten_addresses = '127.0.0.1'\nport = {PORT}\nunix_socket_directories = '{escaped}'\ntimezone = 'UTC'\npassword_encryption = 'scram-sha-256'\n"
            )
    start()
    password = secret_file(LOCAL / "app-password")
    # Variables psql citadas: no se interpolan secretos en shell ni argumentos.
    sql = f"""\set app_password '{password}'
\set database_name '{DB_NAME}'
SELECT format('CREATE ROLE adel_app LOGIN PASSWORD %L NOSUPERUSER NOCREATEDB NOCREATEROLE', :'app_password') WHERE NOT EXISTS (SELECT FROM pg_roles WHERE rolname='adel_app')\gexec
SELECT format('CREATE DATABASE %I OWNER adel_app', :'database_name') WHERE NOT EXISTS (SELECT FROM pg_database WHERE datname=:'database_name')\gexec
"""
    run(
        [PG_BIN / "psql", "-X", "-q", "-v", "ON_ERROR_STOP=1"],
        input=sql,
        text=True,
        env=admin_env(),
    )
    config_file = LOCAL / "server.json"
    if not config_file.exists():
        config = json.loads((ROOT / "config/server.development.example.json").read_text())
        config["database"]["password"] = password
        config["database"]["name"] = DB_NAME
        with open(config_file, "x", opener=lambda p, f: os.open(p, f, 0o600)) as file:
            json.dump(config, file, indent=2)
            file.write("\n")
    print(
        "Base de desarrollo preparada. Configuración privada en .local/server.json; secretos no mostrados."
    )


def integration():
    ensure_owned()
    password = (LOCAL / "app-password").read_text().strip()
    packages = ("./internal/database", "./internal/administration", "./internal/catalog", "./internal/operations", "./internal/payments", "./internal/reports", "./internal/reports/operational")
    selected = os.environ.get("ADEL_TEST_PACKAGE")
    if selected and selected not in packages:
        raise SystemExit("El paquete de integración solicitado no está permitido.")
    for package in ((selected,) if selected else packages):
        name = "adel_pos_test_" + secrets.token_hex(6)
        run([PG_BIN / "createdb", "--owner=adel_app", name], env=admin_env())
        try:
            connection_url = (
                f'postgresql://adel_app:{quote(password, safe="")}@127.0.0.1:'
                f"{PORT}/{name}?sslmode=disable"
            )
            run(
                ["go", "test", "-race", "-tags=integration", package, "-count=1", "-v"] + (["-run", os.environ["ADEL_TEST_RUN"]] if os.environ.get("ADEL_TEST_RUN") else []),
                cwd=ROOT / "backend",
                env=dict(os.environ, ADEL_TEST_DATABASE_URL=connection_url),
            )
        finally:
            run([PG_BIN / "dropdb", name], env=admin_env())


def backup():
    config = json.loads(
        Path(os.environ.get("ADEL_SERVER_CONFIG", LOCAL / "server.json")).read_text()
    )["database"]
    env = dict(
        os.environ,
        PGHOST=config["host"],
        PGPORT=str(config["port"]),
        PGUSER=config["user"],
        PGDATABASE=config["name"],
        PGPASSWORD=os.environ.get("ADEL_DB_PASSWORD", config["password"]),
        PGSSLMODE=config["ssl_mode"],
        PGCONNECT_TIMEOUT="3",
    )
    if config.get("ssl_root_cert"):
        env["PGSSLROOTCERT"] = config["ssl_root_cert"]
    destination = ROOT / "backups"
    destination.mkdir(mode=0o700, exist_ok=True)
    filename = (
        destination
        / f"{config['name']}-{datetime.now(timezone.utc).strftime('%Y%m%dT%H%M%SZ')}-{secrets.token_hex(3)}.dump"
    )
    try:
        with open(filename, "xb", opener=lambda p, f: os.open(p, f, 0o600)) as output:
            run(
                [PG_BIN / "pg_dump", "--format=custom", "--no-owner", "--no-acl"],
                env=env,
                stdout=output,
            )
        run([PG_BIN / "pg_restore", "--list", filename], stdout=subprocess.DEVNULL)
    except Exception:
        filename.unlink(missing_ok=True)
        raise
    print(
        f"Respaldo generado y catálogo legible: {filename.name}. Aún debe probarse su restauración."
    )


if __name__ == "__main__":
    action = sys.argv[1]
    if action == "db-init":
        init()
    elif action == "db-start":
        start()
    elif action == "db-stop":
        ensure_owned()
        run([PG_BIN / "pg_ctl", "-D", DATA, "-m", "fast", "-w", "stop"])
    elif action == "test-integration":
        integration()
    elif action == "backup":
        backup()
    else:
        raise SystemExit("Comando no válido.")
