# Desarrollo en Windows · Etapa 9E

Guía para desarrollar, probar y empaquetar AD-EL POS en Windows 10 u 11 de 64 bits. Es el equivalente de
[DEVELOPMENT_MACOS.md](DEVELOPMENT_MACOS.md), que sigue siendo la guía de la Mac: `scripts/dev.sh` y los scripts de
Python no cambiaron. Lo ejecutado y sus resultados están en [docs/ETAPA_9E_AVANCE.md](docs/ETAPA_9E_AVANCE.md).

## Herramientas

Igual que en macOS, Go, Node y Rust viven **dentro del proyecto** (`.tools\`, excluida de git) para no depender ni
alterar las versiones del equipo. `scripts\env.ps1` las antepone al `PATH` solo en la sesión que lo carga.

| Herramienta                    | Versión           | Origen y lugar                                                                                                                                                             |
| ------------------------------ | ----------------- | -------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Go                             | 1.27.3            | zip oficial `go1.27.3.windows-amd64.zip` (go.dev) en `.tools\go`                                                                                                           |
| Node / npm                     | 24.22.2 / 11.19.1 | zip oficial `node-v24.22.2-win-x64.zip` (nodejs.org) en `.tools\node`                                                                                                      |
| Rust                           | 1.99.1 (MSVC)     | `rustup-init.exe` oficial con `RUSTUP_HOME=.tools\rustup` y `CARGO_HOME=.tools\cargo`, `--no-modify-path --default-toolchain 1.99.1 --profile minimal --component rustfmt` |
| Visual Studio Build Tools 2022 | 17.14             | `winget install --id Microsoft.VisualStudio.2022.BuildTools` con la carga `Microsoft.VisualStudio.Workload.VCTools` (por equipo: pide permiso de administrador una vez)    |
| PostgreSQL                     | 18.6              | binarios oficiales para Windows (`postgresql-18.6-1-windows-x64-binaries.zip` de EDB) en `.tools\pgsql`, sin instalador ni servicio                                        |
| WebView2                       | Evergreen         | ya viene con Windows 11; el instalador incluye su instalador de arranque                                                                                                   |

Comprobar los SHA-256 de los zip con los que publica cada sitio (registrados en el avance). Las dependencias de npm,
Go y Rust salen de los lockfiles (`npm ci`, `go mod download`, `cargo … --locked`).

## Comandos

Todos desde la raíz del repositorio en PowerShell:

```powershell
powershell -ExecutionPolicy Bypass -File scripts\dev.ps1 <comando>
```

`-ExecutionPolicy Bypass` solo afecta a ese proceso; no cambia la directiva del equipo. La lógica está en
`scripts\windows\dev.mjs` (Node), que reproduce las reglas de `dev.sh`, `database.py`, `server-process.py`,
`browser-tests.py` y `restore-check.py`.

| Comando                                                                     | Efecto                                                                                                                                                                                               |
| --------------------------------------------------------------------------- | ---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `install`                                                                   | `npm ci`, sincroniza metadatos y `go mod download` (requiere red).                                                                                                                                   |
| `sync`, `format`, `format-check`                                            | Como en macOS.                                                                                                                                                                                       |
| `db-init` / `db-start` / `db-stop`                                          | Clúster exclusivo `.local\postgres` (puerto 55432, solo `127.0.0.1`, SCRAM, UTC, `locale=C`, sin socket de Unix), rol `adel_app`, base `adel_pos` y `.local\server.json`.                            |
| `backup`, `restore-check backups\NOMBRE.dump`                               | Respaldo `pg_dump` custom y ensayo de restauración en una base temporal con conteo de todas las tablas.                                                                                              |
| `build-server`                                                              | `bin\ad-el-pos-server.exe`.                                                                                                                                                                          |
| `migrate`, `check-config`, `prepare-setup`, `tls-init`, `create-enrollment` | Como en macOS, con `.local\server.json`.                                                                                                                                                             |
| `provision-sa`, `reset-sa`                                                  | **Solo el propietario**, en una consola interactiva (usa `CONIN$`/`CONOUT$`).                                                                                                                        |
| `server` / `server-start` / `server-stop` / `status`                        | Go en primer plano o como proceso oculto independiente (bitácora `.local\server.log`, PID `.local\server.pid`). La parada usa `ad-el-pos-server stop --pid`, que solo detiene a este mismo programa. |
| `desktop`                                                                   | `tauri dev` con Vite en 127.0.0.1:1420 (no lanza Go: hay que iniciarlo aparte, como en macOS).                                                                                                       |
| `test`                                                                      | `format-check`, `sync --check`, `go test ./...`, `go vet`, Vitest, `npm run build` y `cargo test --locked`.                                                                                          |
| `test-integration`                                                          | Pruebas `-tags=integration` con una base desechable `adel_pos_test_*` por paquete en el clúster del proyecto.                                                                                        |
| `test-browser`                                                              | Go temporal en 8183 y Playwright Chromium y WebKit (`ADEL_BROWSER_ENGINES=chromium` para uno solo).                                                                                                  |
| `build-windows`                                                             | Instalador NSIS de prueba (ver abajo).                                                                                                                                                               |

Primera vez:

```powershell
powershell -ExecutionPolicy Bypass -File scripts\dev.ps1 install
powershell -ExecutionPolicy Bypass -File scripts\dev.ps1 db-init
powershell -ExecutionPolicy Bypass -File scripts\dev.ps1 migrate
powershell -ExecutionPolicy Bypass -File scripts\dev.ps1 prepare-setup
powershell -ExecutionPolicy Bypass -File scripts\dev.ps1 server-start
powershell -ExecutionPolicy Bypass -File scripts\dev.ps1 desktop
```

## Diferencias con macOS en el desarrollo

- **Sin `-race`**: el detector de carreras de Go exige cgo y un compilador de C (gcc) en Windows. `test` y
  `test-integration` lo usan solo si encuentran `gcc` (por ejemplo en `.tools\mingw64\bin`); si no, avisan y corren sin
  él. La Mac sigue ejecutándolos con `-race`.
- **Procesos**: Windows no tiene `SIGTERM` ni `ps`. El servidor crea un evento de parada con nombre propio de su
  PID y `stop --pid` lo activa (`cmd/ad-el-pos-server/stop_windows.go`); los procesos ocultos se lanzan sin heredar
  las tuberías de la consola (si las heredaran, la consola esperaría a que el servidor terminara).
- **Permisos**: los bits 0600/0700 no protegen en Windows; los archivos privados reciben una ACL propia (cuenta
  actual, SYSTEM y Administradores, sin herencia) con `internal/fileacl`.
- **Fin de línea**: `.gitattributes` fija LF para todo el texto (Git para Windows extraería CRLF y cambiaría el
  SHA-256 de las migraciones). Los `.ps1` y `.nsh` se guardan en UTF-8 **con BOM** (Windows PowerShell 5.1 y makensis
  leen mal los acentos sin él). No reescribir archivos UTF-8 con `Get-Content`/`Set-Content` de PowerShell 5.1.
- **Antivirus**: Microsoft Defender analiza cada compilación; la primera de Go o Rust tarda varios minutos. No se
  agregaron exclusiones (cambiar la seguridad de Windows queda fuera del proyecto).
- **Mismo puerto**: el clúster de desarrollo y el de la aplicación instalada usan 55432 y Go 8181; no pueden correr a
  la vez. Detener el de desarrollo (`db-stop`, `server-stop`) antes de abrir la aplicación instalada.

## Instalador de prueba

```powershell
powershell -ExecutionPolicy Bypass -File scripts\build-windows.ps1
```

Comprueba las herramientas, sincroniza metadatos, compila el servidor como binario externo de Tauri
(`desktop\src-tauri\binaries\ad-el-pos-server-x86_64-pc-windows-msvc.exe`), prepara solo los binarios de
PostgreSQL que se usan con el runtime de Visual C++ (`desktop\src-tauri\target\windows-bundle\postgresql`), comprueba
con `initdb` que funcionan y genera el instalador NSIS con `desktop\src-tauri\tauri.windows-installer.conf.json`. El
resultado queda en `desktop\src-tauri\target\release\bundle\nsis\AD-EL POS_<versión>_x64-setup.exe`. Detalle de la
instalación y de lo que hace en el equipo: [docs/INSTALAR_EN_WINDOWS.md](docs/INSTALAR_EN_WINDOWS.md) (para el
propietario) y `CLAUDE.md` §17.
