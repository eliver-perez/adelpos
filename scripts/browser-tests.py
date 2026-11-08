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


def run_browser(engine):
    database = runpy.run_path(str(ROOT / "scripts/database.py"))
    database["ensure_owned"]()
    name = "adel_pos_test_browser_" + secrets.token_hex(6)
    directory = ROOT / ".local" / name
    directory.mkdir(mode=0o700)
    config = json.loads((ROOT / ".local/server.json").read_text())
    config["database"]["name"] = name
    config["listen_address"] = "127.0.0.1:8183"
    config["allow_lan"] = False
    filename = directory / "server.json"
    with open(filename, "x", opener=lambda p, f: os.open(p, f, 0o600)) as output:
        json.dump(config, output)
    server = None
    created = False
    try:
        database["run"](
            [database["PG_BIN"] / "createdb", "--owner=adel_app", name],
            env=database["admin_env"](),
        )
        created = True
        subprocess.run([str(ROOT / "scripts/dev.sh"), "build-server"], cwd=ROOT, check=True)
        for command in ("migrate", "prepare-setup"):
            subprocess.run(
                [str(ROOT / "bin/ad-el-pos-server"), command, "--config", str(filename)],
                cwd=ROOT,
                check=True,
            )
        with (directory / "server.log").open("w") as log:
            server = subprocess.Popen(
                [str(ROOT / "bin/ad-el-pos-server"), "serve", "--config", str(filename)],
                cwd=ROOT,
                stdout=log,
                stderr=log,
            )
            for attempt in range(100):
                try:
                    with urllib.request.urlopen(
                        "http://127.0.0.1:8183/api/v1/health/ready", timeout=1
                    ) as response:
                        if response.status == 200:
                            break
                except OSError:
                    time.sleep(0.1)
            else:
                raise RuntimeError("El servidor temporal no quedó listo")
            environment = dict(
                os.environ,
                ADEL_BROWSER_ENGINE=engine,
                ADEL_BROWSER_PROVISIONING=str(directory / "provisioning-token"),
                PLAYWRIGHT_BROWSERS_PATH=str(ROOT / ".cache/playwright"),
            )
            subprocess.run(
                ["npm", "run", "test:browser", "--", "--project", engine],
                cwd=ROOT / "desktop",
                env=environment,
                check=True,
            )
    finally:
        if server is not None:
            server.terminate()
            server.wait(timeout=15)
        if created:
            database["run"](
                [database["PG_BIN"] / "dropdb", name], env=database["admin_env"]()
            )
        shutil.rmtree(directory)


for engine in ("chromium", "webkit"):
    run_browser(engine)
