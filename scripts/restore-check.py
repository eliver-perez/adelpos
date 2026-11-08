#!/usr/bin/env python3
"""Restaura un respaldo en una base temporal y compara conteos de todas las tablas."""
import json
import os
from pathlib import Path
import runpy
import secrets
import subprocess
import sys

ROOT = Path(__file__).resolve().parent.parent
database = runpy.run_path(str(ROOT / "scripts/database.py"))
database["ensure_owned"]()
dump = Path(sys.argv[1]).resolve()
if dump.parent != ROOT / "backups" or not dump.is_file():
    raise SystemExit("Selecciona un respaldo existente dentro de backups del proyecto")
config = json.loads((ROOT / ".local/server.json").read_text())["database"]
if config["host"] != "127.0.0.1" or config["port"] != 55432:
    raise SystemExit("El ensayo está limitado al clúster exclusivo de desarrollo")


def counts(name):
    environment = dict(database["admin_env"](), PGDATABASE=name)

    def query(sql):
        return subprocess.check_output(
            [str(database["PG_BIN"] / "psql"), "-XAt", "-v", "ON_ERROR_STOP=1", "-c", sql],
            env=environment,
            text=True,
        ).strip()

    tables = query(
        "SELECT tablename FROM pg_tables WHERE schemaname='app' ORDER BY tablename"
    ).splitlines()
    result = {}
    for table in tables:
        identifier = table.replace('"', '""')
        result[table] = int(query(f'SELECT count(*) FROM app."{identifier}"'))
    return result


name = "adel_pos_test_restore_" + secrets.token_hex(6)
before = counts(config["name"])
database["run"](
    [database["PG_BIN"] / "createdb", "--owner=adel_app", name],
    env=database["admin_env"](),
)
try:
    database["run"](
        [database["PG_BIN"] / "pg_restore", "--exit-on-error", "--no-owner", "--no-acl",
         "--role=adel_app", "--dbname", name, dump],
        env=database["admin_env"](),
    )
    restored = counts(name)
    if before != restored:
        raise RuntimeError("Los conteos restaurados difieren; no continuar con la migración")
    report = {
        "backup": dump.name,
        "restored_in_temporary_database": True,
        "counts_equal": True,
        "tables": restored,
    }
    stage = json.loads((ROOT / "config/product.json").read_text())["schema_version"]
    (ROOT / f".local/stage{stage}-restore-verification.json").write_text(
        json.dumps(report, indent=2) + "\n"
    )
    print(f"Restauración completa verificada: {len(restored)} tablas con conteos idénticos.")
finally:
    database["run"]([database["PG_BIN"] / "dropdb", name], env=database["admin_env"]())
