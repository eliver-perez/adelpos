# Etapa 9E · Avance (Windows 11: compilación, pruebas e instalador)

Especificación del propietario: [docs/etapas/ETAPA_9E_WINDOWS.md](etapas/ETAPA_9E_WINDOWS.md) (solo lectura). Rama
`etapa-9e-windows`, creada desde `main` (`6763379`, etapa 9D con la versión 0.9.3 y el esquema 21, comprobados en
`config/product.json` y `CLAUDE.md`). Se trabaja en una computadora con Windows 11 sobre un clon recién hecho del
repositorio. Este archivo registra solo lo que realmente se ejecutó.

## Fase 0 · Verificación del entorno y del código (12/11/2026)

### 1. Equipo

| Dato             | Valor                                                                                       |
| ---------------- | ------------------------------------------------------------------------------------------- |
| Windows          | Windows 11 Pro 25H2, versión 10.0.26200, compilación 26200.9168                              |
| Arquitectura     | x64 (64 bits); procesador Intel Core i7-1355U (10 núcleos, 12 hilos)                         |
| Memoria          | 31.7 GB (16.7 GB libres al iniciar)                                                          |
| Disco C:         | 1 331 GB libres (530 GB usados)                                                              |
| Usuario          | `Expediente`, perfil `C:\Users\Expediente` (sin espacios ni acentos); la sesión **no** está elevada |
| Directivas       | `ExecutionPolicy` del equipo `RemoteSigned`; la del proceso de esta sesión, `Bypass`         |

### 2. Herramientas encontradas

| Herramienta                                    | Estado                                                                                                            |
| ---------------------------------------------- | ----------------------------------------------------------------------------------------------------------------- |
| Git                                            | 2.56.0.windows.4 (`C:\Program Files\Git`); `core.autocrlf=true` en el `gitconfig` del sistema                     |
| Go                                             | **no instalado** (el proyecto usa 1.27.3; `go.mod` declara 1.27.0)                                                |
| Node / npm                                     | 20.19.6 / 10.8.2 instalados en todo el equipo (`C:\Program Files\nodejs`); el proyecto usa **24.22.2**           |
| Gestor de paquetes del proyecto                | npm con `package-lock.json` (`npm ci`)                                                                            |
| Rust (`rustup`, `x86_64-pc-windows-msvc`)      | **no instalado** (el proyecto usa 1.99.1, `Cargo.toml` `rust-version = "1.99"`)                                    |
| Visual Studio Build Tools («Desarrollo para el escritorio con C++») | **no instalado** (no existe `vswhere.exe`)                                         |
| WebView2                                       | 157.0.4341.46 (Evergreen, en todo el equipo)                                                                      |
| CLI de Tauri                                   | se instala con `npm ci` (`@tauri-apps/cli` 2.12.4 fijado en `package.json`)                                       |
| PostgreSQL                                     | **no instalado** (el proyecto usa 18.6)                                                                           |
| Python                                         | solo el alias de Microsoft Store (no hay intérprete)                                                              |
| Compilador de C para `cgo` (`go test -race`)   | no instalado                                                                                                      |
| winget                                         | 1.29.380                                                                                                          |

Red: se comprobó acceso a go.dev, nodejs.org, static.rust-lang.org, get.enterprisedb.com (existe
`postgresql-18.6-1-windows-x64-binaries.zip`), github.com, registry.npmjs.org, proxy.golang.org e index.crates.io.
No hay archivos `.nvmrc` ni `rust-toolchain`: las versiones salen de [DEPENDENCIES.md](DEPENDENCIES.md) y los
lockfiles.

### 3. Cómo arranca hoy la aplicación en macOS

Según [NOTAS_WINDOWS.md](NOTAS_WINDOWS.md) y el código (`scripts/dev.sh`, `scripts/*.py`,
`backend/cmd/ad-el-pos-server/main.go`, `desktop/src-tauri/src/main.rs`):

- **Inclusión e inicio de Go y PostgreSQL**: no son sidecars ni recursos de Tauri. `tauri.conf.json` no declara
  `externalBin` ni `resources` y `bundle.targets` es solo `["app"]`. Son tres procesos independientes: PostgreSQL de
  Homebrew (`scripts/database.py:17-26`) iniciado con `pg_ctl` desde Python, Go iniciado con
  `scripts/server-process.py` (PID en `.local/server.pid`, `start_new_session=True`) y la aplicación. Regla vigente
  (`CLAUDE.md` §3): «Tauri no inicia ni detiene Go ni PostgreSQL».
- **Puertos**: Go 8181 en `127.0.0.1` (`config/server.development.example.json`), PostgreSQL 55432 solo en
  `127.0.0.1`; Vite 1420, vista previa 1421 y Go temporal de Playwright 8183. La LAN exige `allow_lan`, IP privada y
  TLS, y Go la rechaza fuera de desarrollo.
- **Carpeta de datos**: todo dentro del repositorio, en `.local/` (clúster `.local/postgres`, `server.json` con la
  contraseña de `adel_app`, contraseñas, marcador del clúster, token de aprovisionamiento, código de enrolamiento,
  `tls/`, `server.log`, `server.pid`) y `backups/`.
- **Creación de la base en el primer arranque**: no es automática. `dev.sh db-init` (`database.py:init`) ejecuta
  `initdb --locale=C --auth=scram-sha-256 --encoding=UTF8`, agrega `listen_addresses`, `port`,
  `unix_socket_directories`, `timezone='UTC'` y SCRAM, crea el rol `adel_app` y la base `adel_pos`, y escribe
  `.local/server.json`.
- **Migraciones**: embebidas en el binario (`backend/migrations/embed.go`), aplicadas a mano con `dev.sh migrate`;
  cada una se identifica por el **SHA-256 de sus bytes** (`internal/database/migrate.go`), y `health/ready` exige que el
  historial coincida.
- **Secretos**: el servidor usa archivos 0600 en `.local/`; la terminal guarda identificador, token de terminal y token
  de sesión en Keychain con el crate `keyring` (`vault.rs`), que ya tiene activado el backend de Windows
  (`features = ["apple-native", "windows-native"]`).
- **Respaldos**: `dev.sh backup` (`database.py:backup`) con `pg_dump --format=custom` y `pg_restore --list`; el ensayo
  de restauración es `scripts/restore-check.py` (base temporal y conteo de todas las tablas de `app`).
- **Bitácoras**: Go escribe JSON por la salida estándar (`main.go:31`), que `server-process.py` redirige a
  `.local/server.log`; PostgreSQL a `.local/postgres.log`.
- **Impresión**: `kitchen.Transport` con dos transportes, CUPS (`/usr/bin/lp`, `lpstat`, `lpoptions`) y ESC/POS por TCP
  (puertos 9100–9109, IP privada). El valor `agent` está reservado y no implementado.
- **Cajón de dinero**: pulso ESC/POS por TCP o, desde la 7.1, por una cola CUPS en modo raw (detección de solo lectura
  con `lpstat`/`lpoptions`).

### 4. Todo lo que depende de macOS (archivo y línea)

**Rutas y comandos de macOS en Go**

- `backend/internal/kitchen/transport.go:55, 67, 100, 110, 139, 319`: `/usr/bin/lpstat`, `/usr/bin/lpoptions` y
  `/usr/bin/lp` (descubrir colas, opción de corte, texto y pulso del cajón).
- `backend/internal/kitchen/transport_formatted.go:138`: `/usr/bin/lp -o raw` para documentos con formato.
- `backend/internal/kitchen/cups_raw.go:86, 89`: detección de colas raw con `lpstat -p` y `lpoptions -p`.
- Usuarios de lo anterior: `kitchen/reads.go:20` (`DiscoverPrinters`), `kitchen/worker.go:226` (`HostTransport`),
  `payments/cash_drawer.go:129` y `designer/samples.go:131` (`HostQueueInspector`), `cmd/ad-el-pos-server/main.go:93`.
- `backend/internal/kitchen/printers.go:14`: `queuePattern` solo admite nombres de cola CUPS
  (`[A-Za-z0-9_-]`); los nombres de impresoras de Windows suelen llevar espacios (por ejemplo «POS-80 Series»).
- `backend/cmd/ad-el-pos-server/local.go:38`: `/dev/tty` para `provision-sa` y `reset-sa` (no existe en Windows).

**Señales y procesos**

- `backend/cmd/ad-el-pos-server/main.go:82`: `signal.NotifyContext(…, os.Interrupt, syscall.SIGTERM)`; Windows no envía
  `SIGTERM` y un proceso sin consola no recibe Ctrl+C.
- `scripts/server-process.py:24, 92, 101`: `ps` y `os.kill(…, SIGTERM)`; `:59` `start_new_session=True`.
- `tests/runtime_smoke.py:17, 51, 88, 104, 138, 189`: `.app`, `ps -axo` y `osascript`.

**Permisos de archivo**

- `backend/cmd/ad-el-pos-server/local.go:25, 77, 82` y `backend/internal/localsetup/tls.go:28, 96`: 0600/0700, que en
  Windows no protegen (solo el atributo de solo lectura). La protección real es la ACL de la carpeta.
- `backend/internal/localsetup/tls_test.go:57`: comprueba `Perm() == 0600` y falla en Windows (Go informa 0666).
- `scripts/database.py:36, 81, 107, 132, 181, 187`, `scripts/browser-tests.py:21, 27`, `scripts/server-process.py:41`.

**Keychain y textos de macOS**

- `desktop/src-tauri/src/main.rs:33`: «Revisa el diálogo de macOS».
- `desktop/src/App.tsx:281`: «Revisa su diálogo de acceso al Llavero».
- `desktop/src-tauri/src/vault.rs:66`: la prueba real del almacén seguro solo existe para macOS.
- Textos con «CUPS» o «macOS» en la configuración de impresión: `pages/kitchen/PrintersAdmin.tsx:68-69, 82, 92, 119,
  166, 176, 185, 284`, `pages/cash/CashHardwareSettings.tsx:132, 142`, `pages/documents/IdentityPanel.tsx:113`,
  `pages/documents/designer/BlockProperties.tsx:190`, `pages/documents/designer/SampleDialog.tsx:24`,
  `pages/kitchen/PrintQueue.tsx:46`, `services/cash.ts:461`.

**Escritorio**

- `desktop/src-tauri/tauri.conf.json`: `bundle.targets: ["app"]`, solo `icons/icon.png` (Windows exige
  `icons/icon.ico` para el recurso del ejecutable), `bundle.macOS`.
- `desktop/src-tauri/src/menu.rs:197`: submenú «AD-EL POS» solo en macOS (el resto del menú nunca se compiló en
  Windows).
- `desktop/src-tauri/src/export.rs:307, 338`: temporal `.adel-export-….part` (en Windows el punto no lo oculta) y
  `fs::rename` (falla si el destino está abierto en otro programa).

**Scripts que solo funcionan en bash o con Python de macOS**

- `scripts/dev.sh` y `scripts/env.sh` (bash; `.tools/*/bin` con binarios de macOS).
- `scripts/database.py:21-23` (`os.uname()`, Homebrew), `:111` (`unix_socket_directories`).
- `scripts/browser-tests.py:37, 40, 46` (`scripts/dev.sh build-server`, `bin/ad-el-pos-server` sin `.exe`).
- `scripts/restore-check.py` (depende de `database.py`).
- `desktop/package.json` y `playwright.config.ts` no usan comandos propios de bash (`&&` también funciona en `cmd.exe`).

**Nombres de archivo**: ninguno con caracteres no válidos en Windows (`: * ? " < > |`), ninguna pareja que difiera
solo en mayúsculas y ninguna ruta de más de 150 caracteres. Las rutas de Go usan `path/filepath`.

**Zona horaria**: `backend/internal/administration/setup.go:10` ya importa `time/tzdata`, de modo que el binario de Go
trae su propia base de zonas horarias (Windows no tiene `zoneinfo`).

### 5. Fin de línea

- No existe `.gitattributes`. `core.autocrlf=true` (configuración del sistema de Git para Windows) dejó **1 006
  archivos de texto con CRLF** en el área de trabajo (todos con LF en el índice) y 5 binarios (`*.png`, el `.bin` de
  prueba de CUPS).
- Consecuencias si no se corrige: Prettier (`endOfLine: "lf"`) y `gofmt -l` marcan todos los archivos; el
  **SHA-256 de cada migración** cambiaría (las migraciones embebidas tendrían otra huella que la instalación de macOS y
  un respaldo de la Mac sería «historial incompatible»); las pruebas byte a byte de plantillas, `testdata` y la
  lectura de `api.rs`/`menu.rs`/`export.rs` con expresiones regulares en `native-driver.ts` se alteran.
- La especificación `docs/etapas/ETAPA_9E_WINDOWS.md` está sin versionar y con LF (md5
  `f147a8869c1853f1e584b69150c69673`; las de las etapas anteriores se registraron con md5 para comprobarlas al final).

### 6. Plan de las fases C y D

**Decisión de instalación (0-B)**: instalador NSIS **por usuario** (`installMode: currentUser`). Es el más simple y
estable con PostgreSQL incluido: no pide permisos de administrador (ni para instalar, ni para actualizar, ni para
desinstalar), no crea servicios de Windows y la base corre con la cuenta del propio usuario (PostgreSQL se niega a
correr con privilegios de administrador). Por equipo exigiría elevar en cada instalación, servicios con una cuenta
propia y ACL sobre `%ProgramData%`.

- **Programa**: `%LOCALAPPDATA%\Programs\AD-EL POS\` (la plantilla NSIS de Tauri 2.12.4 instala por omisión en
  `%LOCALAPPDATA%\AD-EL POS`, que es la carpeta de datos; se usa una plantilla propia con ese único cambio).
- **Datos**: `%LOCALAPPDATA%\AD-EL POS\` (clúster, configuración y secretos del servidor, bitácoras, respaldos). La
  plantilla de Tauri solo borra al desinstalar los archivos que instaló y, si se marca «borrar datos», las carpetas
  `%APPDATA%\local.adel.pos.dev` y `%LOCALAPPDATA%\local.adel.pos.dev` (perfil de WebView2): los datos del restaurante
  nunca se tocan.

**Procesos (contradicción señalada)**: §3.1 de la 9E pide que en Windows **la aplicación** inicialice, arranque y
detenga PostgreSQL y Go, y que al cerrarla no queden procesos; `CLAUDE.md` §3 dice que Tauri no los inicia ni detiene.
La 9E es la especificación vigente y describe exactamente ese comportamiento para Windows, así que se aplica **solo en
Windows y solo en la instalación empaquetada**; macOS no cambia. Diseño:

1. Go gana un comando nuevo, `host` (solo Windows), que en la carpeta de datos crea el clúster con los binarios de
   PostgreSQL incluidos (`initdb`, rol `adel_app`, base `adel_pos`, `server.json`, token de aprovisionamiento), limpia un
   `postmaster.pid` huérfano, arranca PostgreSQL con `pg_ctl`, respalda y aplica las migraciones pendientes, sirve igual
   que `serve` y al terminar detiene HTTP, los trabajadores y PostgreSQL (`pg_ctl stop -m fast`). Toda la lógica de base
   de datos sigue en Go.
2. La aplicación de Windows, si no hay un servidor respondiendo en `127.0.0.1:8181`, lanza `ad-el-pos-server.exe host`
   oculto (`CREATE_NO_WINDOW`) con la entrada estándar conectada a una tubería: al cerrarse la aplicación (también si
   termina mal) la tubería se cierra y Go se apaga en orden. Sin `SIGTERM`.
3. En modo desarrollo (`tauri dev`) y en macOS la aplicación no lanza nada.

**Resto de adaptaciones (fase C)**, cada una detrás de `//go:build windows`, `#[cfg(target_os = "windows")]` o de la
plataforma detectada en la interfaz, con su prueba:

- Impresión: el valor `cups` del transporte pasa a significar «cola de impresión del sistema» (sin migración): en
  Windows es la cola de impresión de Windows, que recibe **ESC/POS en modo RAW** por la API del administrador de
  impresión (`winspool.drv`: `OpenPrinter`, `StartDocPrinter` con `RAW`, `WritePrinter`); las impresoras instaladas se
  listan con `EnumPrinters` y la detección del cajón usa los datos de la cola (puerto USB, serie, paralelo o TCP/IP; se
  rechazan colas remotas y virtuales como «Microsoft Print to PDF»). ESC/POS por red no cambia. Los textos de la
  interfaz dicen «Windows» en lugar de «CUPS» cuando el servidor corre en Windows.
- Secretos: Administrador de credenciales de Windows con el mismo `vault.rs` (backend `windows-native` de `keyring`) y
  la prueba real habilitada también en Windows; textos neutros por plataforma.
- Rutas: carpetas conocidas de Windows (`%LOCALAPPDATA%`, `%TEMP%`), con prueba de rutas con espacios y acentos.
- Menú nativo, atajos con Ctrl y paleta: verificar en WebView2; decidir si se oculta la barra de menú.
- Ventana: recordar tamaño y posición en Windows; pantalla completa con F11 y el botón de la barra.
- Respaldos: comandos de Go `backup` y `restore-check` con `pg_dump`/`pg_restore` incluidos.
- Scripts: `scripts/dev.ps1` y `scripts/env.ps1` (PowerShell 5.1) equivalentes a `dev.sh`, sin tocar los de macOS.

**Instalador (fase D)**: `tauri.windows.conf.json` (Tauri lo combina solo en Windows, así que `tauri.conf.json` de
macOS no cambia) con destino `nsis`, `icons/icon.ico`, el servidor como `externalBin`, solo los binarios de
PostgreSQL que se usan como recursos, el instalador de arranque de WebView2 incluido (`embedBootstrapper`), accesos
directos y la plantilla propia. Script `scripts/build-windows.ps1` de principio a fin.

**Herramientas (fase A)**: Go, Node y Rust se instalan **dentro del proyecto** (`.tools\`, excluida de git), como en
macOS, desde los archivos oficiales con su SHA-256: no requieren administrador y no reemplazan el Node 20 que ya usa
este equipo (actualizarlo con winget afectaría otros programas del usuario). Visual Studio Build Tools solo se instala
por equipo: se usa winget y Windows pedirá permiso de administrador (UAC) una vez. PostgreSQL 18.6 con los binarios
oficiales para Windows (zip de EDB, sin instalador ni servicio).

## Fase A · Herramientas de desarrollo y fin de línea (12/11/2026)

### Herramientas instaladas

| Herramienta                     | Versión                       | Dónde y cómo                                                                                                       |
| ------------------------------- | ----------------------------- | ------------------------------------------------------------------------------------------------------------------ |
| Go                              | 1.27.3 windows/amd64          | `.tools\go`, zip oficial de go.dev (SHA-256 `f26081f4…7885fff`, igual al publicado)                                |
| Node / npm                      | 24.22.2 / 11.19.1             | `.tools\node`, zip oficial de nodejs.org (SHA-256 `ca1bff5d…7799e2`, igual a `SHASUMS256.txt`); el Node 20 del equipo no se tocó |
| Rust                            | rustc 1.99.1, cargo 1.99.1, rustfmt 1.10.0 | `.tools\rustup` y `.tools\cargo` con `rustup-init.exe` oficial (SHA-256 `38027d51…63eec7db`, igual al publicado); destino `x86_64-pc-windows-msvc`, perfil mínimo, sin modificar el `PATH` del usuario |
| Visual Studio Build Tools 2022  | 17.14.37906.3 (paquete winget 17.14.45) | winget `Microsoft.VisualStudio.2022.BuildTools` con la carga `Microsoft.VisualStudio.Workload.VCTools` y los componentes recomendados (MSVC x64 y SDK de Windows 11 10.0.26100); por equipo, con una confirmación de Control de cuentas de usuario; `isRebootRequired=false` |
| PostgreSQL                      | 18.6 (binarios EDB 18.6-1)    | `.tools\pgsql`, `postgresql-18.6-1-windows-x64-binaries.zip` de get.enterprisedb.com (SHA-256 `fbe23da2…0252f8c`; EDB no publica la suma del zip). Sin instalador ni servicio |
| CLI de Tauri                    | 2.12.4                        | `npm ci` en `desktop` (`package-lock.json` sin cambios)                                                            |
| Módulos de Go                   | los de `go.sum`               | `go mod download` a `.cache\go-mod`                                                                                |

- Windows tiene un **reinicio pendiente de Windows Update** (marcas `RebootPending` y `RebootRequired`), ajeno a esta
  etapa: Visual Studio no pidió reiniciar y nada de lo siguiente lo necesita.
- Go, Node y Rust van **dentro del proyecto**, como en macOS: no piden administrador, no cambian variables del
  usuario y no reemplazan el Node 20 instalado en el equipo. `scripts\env.ps1` los antepone al `PATH` solo en la
  sesión que lo carga.
- No se instaló Python (los scripts de Windows usan Node) ni un compilador de C para `cgo`: `go test -race` necesita
  cgo en Windows, así que las suites de Go corren sin `-race` en este equipo (los scripts lo usan solo si encuentran
  `gcc`). En macOS siguen con `-race`.
- Los binarios de PostgreSQL de EDB **no incluyen** el runtime de Visual C++ (`vcruntime140.dll`, `msvcp140.dll`): el
  instalador debe llevarlo (fase D).

### Fin de línea

- `.gitattributes` nuevo: `* text=auto eol=lf`, entradas explícitas para código, SQL, plantillas, scripts y datos
  (`*.go`, `*.sql`, `*.rs`, `*.ts(x)`, `*.json`, `*.md`, `*.sh`, `*.py`, `*.ps1`, `*.nsi`/`*.nsh`, …), CRLF solo para
  `*.cmd`/`*.bat` y binarios marcados (`*.png`, `*.ico`, fuentes, `*.pdf`, `*.xlsx`, `*.dump`, `*.bin`, …).
- `git add --renormalize .` no cambió nada en el índice (ya tenía LF). Se volvió a extraer el área de trabajo:
  `git ls-files --eol` informa **1 008 archivos `i/lf w/lf` y 5 binarios**, y `git status` no muestra cambios
  espurios. Las once especificaciones de `docs/etapas/` quedaron idénticas a lo versionado (md5 de cada archivo igual
  al de `git show HEAD:…`; los md5 de la Fase 0 eran de las copias con CRLF).
- Comprobado: `go build ./...` y `GOOS=darwin GOARCH=arm64 go build ./...` compilan.

## Fase B · Modo desarrollo y pruebas existentes en Windows (12/11/2026)

### Scripts de desarrollo para Windows

`scripts/dev.sh` y los scripts de Python son de macOS y no se tocaron. Se agregaron sus equivalentes:

- `scripts/env.ps1`: herramientas de `.tools` en el `PATH` de la sesión y cachés en `.cache` (equivalente de `env.sh`).
- `scripts/dev.ps1 <comando>`: punto de entrada (`powershell -ExecutionPolicy Bypass -File scripts\dev.ps1 …`); la
  lógica está en `scripts/windows/dev.mjs` (Node del proyecto, sin dependencias) porque maneja rutas con espacios
  (la carpeta del repositorio es «ADEL POS») y procesos mejor que PowerShell 5.1. Comandos: `sync`, `install`,
  `format`, `format-check`, `db-init`, `db-start`, `db-stop`, `backup`, `restore-check`, `build-server`, `migrate`,
  `check-config`, `prepare-setup`, `provision-sa`, `reset-sa`, `tls-init`, `create-enrollment`, `server`,
  `server-start`, `server-stop`, `status`, `desktop`, `open-app`, `test`, `test-integration`, `test-browser` y
  `build-windows`. Conservan las reglas de macOS: clúster exclusivo con marcador, secretos sin mostrar, bases
  desechables `adel_pos_test_*` y parada ordenada.

Dos defectos reales de Windows aparecieron al usarlos y se corrigieron:

1. **Herencia de tuberías**: `pg_ctl start` deja a PostgreSQL con las salidas de `pg_ctl`, y un hijo de Node hereda
   las tuberías de Node; la consola que lanzó el comando esperaba a que PostgreSQL o Go terminaran. `db-start` ya
   no comparte su salida con PostgreSQL y `server-start` lanza Go con `Start-Process` (sin herencia), con la
   bitácora en archivo (opción nueva `--log-file` de Go, con rotación simple al pasar de 20 MiB).
2. **Sin `SIGTERM`**: en Windows cada servidor crea un evento con nombre ligado a su PID y el comando nuevo
   `ad-el-pos-server stop --pid N` lo activa después de comprobar que el proceso es el mismo programa
   (`cmd/ad-el-pos-server/stop_windows.go`; en macOS `stop_other.go` no hace nada y la parada sigue con `SIGTERM`).
   Medido: parada ordenada en 561 ms con «server_shutdown_complete»; un PID de otro programa se rechaza sin enviar
   nada.

Comprobado en este equipo: `db-init` (clúster `.local\postgres`, rol, base y `server.json`), `db-stop`, `db-start`,
`db-init` repetido, `migrate` (`applied: 21`), `prepare-setup`, `server-start` (readiness 200, versión 0.9.3, esquema
21), `status` y `server-stop`.

### Pruebas existentes en Windows

| Suite                                                              | Resultado en Windows                                                                                               |
| ------------------------------------------------------------------ | ------------------------------------------------------------------------------------------------------------------ |
| `go test ./...` (sin `-race`: no hay compilador de C para cgo)     | 22 paquetes aprobados y 1 falla: `TestTLSRequiresTrustedCAAndCorrectHostname` (corregida, ver abajo)               |
| `go test -tags=integration` (`dev.ps1 test-integration`, PostgreSQL 18.6 real, una base desechable por paquete) | **los 9 paquetes aprobados** (`database`, `administration`, `catalog`, `operations`, `payments`, `reports`, `reports/operational`, `designer`, `identity`; 559 s) |
| Vitest (`npm test`)                                                | 750 de 751 aprobadas; 1 intermitente (corregida, ver abajo)                                                        |
| `npm run build` (TypeScript + Vite)                                | aprobado                                                                                                           |
| `cargo test --locked`                                              | al principio no compilaba (faltaba `icons/icon.ico`, ver abajo); después **41 aprobadas**                          |
| Playwright Chromium (`dev.ps1 test-browser`, Go temporal en 8183)  | **19 de 19 aprobadas** (7.6 min); WebKit se ejecuta después, junto con la fase C                                   |

Entre lo que pasó sin cambios: las equivalencias **byte a byte** de las plantillas de documentos de la 9B
(`kitchen/legacy_template_test.go`, `payments/legacy_documents_test.go`), la generación de PDF y Excel con las
fuentes Go incluidas (acentos y «ñ»), las zonas horarias (Go trae `time/tzdata`) y el corte del día a las 05:00
(`internal/businessday`), y la barra superior a 1280 × 720 con la fuente Segoe UI.

Clasificación y corrección de los fallos:

- **`TestTLSRequiresTrustedCAAndCorrectHostname` · prueba dependiente de macOS y defecto real de Windows.** Comprobaba
  los bits 0600 de la clave privada, que en Windows no protegen. Paquete nuevo `internal/fileacl`: en Windows fija una
  ACL propia sin herencia (cuenta actual, SYSTEM y Administradores) y en macOS no hace nada. Se aplica a la carpeta
  TLS, al token de aprovisionamiento y al código de enrolamiento; la prueba comprueba la ACL en Windows
  (`private_windows_test.go`) y los bits 0600 en macOS (`private_unix_test.go`).
- **`closing.test.tsx` («guarda el borrador con su revisión…») · prueba frágil por tiempos, no un defecto de
  Windows.** Fallaba 1 de cada 3 veces: el toque del teclado llegaba antes del efecto que adopta el borrador del
  servidor y quedaba reemplazado. La prueba vacía los efectos pendientes con `act` antes de teclear; 6 de 6 corridas
  aprobadas.
- **Rust sin `icons/icon.ico` · requisito de Windows.** `tauri-build` exige un `.ico` para el recurso del ejecutable.
  Se generó con `tauri icon` a partir del mismo `icons/icon.png` (sin modificarlo) y solo se agregó el `.ico`.
- **`provision-sa` y `reset-sa` · defecto de Windows.** Abrían `/dev/tty`; en Windows usan la consola (`CONIN$` y
  `CONOUT$`, `terminal_windows.go`), y en macOS siguen con `/dev/tty` (`terminal_unix.go`). No se ejecutaron (son
  solo del propietario); se comprobó que compilan para las dos plataformas.

Además se movió `golang.org/x/sys` (ya en `go.sum`, v0.49.0) de indirecta a directa: lo usan los archivos de Windows.
Ningún módulo, crate ni paquete npm nuevo. `go build ./...` y `GOOS=darwin GOARCH=arm64 go build ./...` y `go vet`
para ambas plataformas aprobados.

## Fase C · Adaptaciones a Windows (12/11/2026)

### C1 · Procesos, recuperación, secretos, rutas, ventana y respaldos

**Servidor y PostgreSQL en la instalación (§3.1).** Como se señaló en la Fase 0, la 9E pide que en Windows la
aplicación los inicie y los detenga; en macOS nada cambia (`server_host::start` devuelve `None` y Tauri sigue sin
iniciar procesos). Piezas:

- `ad-el-pos-server host` (solo Windows, `cmd/ad-el-pos-server/host_windows.go` y paquete nuevo
  `internal/standalone`, todo con `//go:build windows`): toma un bloqueo exclusivo de la carpeta de datos
  (`host.lock`, que Windows libera aunque el proceso termine mal), le fija la ACL privada, crea el clúster la primera
  vez con los binarios incluidos (`initdb --locale=C --auth=scram-sha-256`, solo `127.0.0.1:55432`, UTC, sin socket de
  Unix y con marcador de propiedad como en macOS), lo arranca con `pg_ctl` (o adopta el que siga activo), borra un
  `postmaster.pid` huérfano, crea el rol `adel_app` y la base `adel_pos`, escribe `server.json` y el token del
  asistente, **respalda antes de aplicar migraciones sobre una base con datos** (`backups\…-antes-de-migrar.dump`),
  migra, sirve exactamente como `serve` (misma función `serveHTTP`) y al terminar detiene HTTP, los trabajadores y
  PostgreSQL (`pg_ctl stop -m fast`). Los procesos hijos no tienen ventana (`CREATE_NO_WINDOW`) ni heredan tuberías.
- `desktop/src-tauri/src/server_host.rs` (solo Windows y solo si junto al ejecutable están el servidor y
  `postgresql\bin`, es decir, en la instalación): un supervisor revisa cada medio segundo; si nadie responde en
  `127.0.0.1:8181`, lanza `host --watch-stdin` oculto con la entrada estándar en una tubería. Al salir, la aplicación
  cierra la tubería y espera hasta 25 s; si la aplicación termina mal, Windows cierra la tubería y Go se apaga igual.
  Relanza con espera creciente (1, 2, 4… hasta 30 s) si el servidor muere enseguida, y mientras arranca la pantalla
  de conexión dice que el equipo se está preparando (la primera vez tarda hasta un minuto). El token del asistente
  se lee de la carpeta de datos (en macOS sigue llegando por `ADEL_PROVISIONING_FILE`). En `tauri dev` no lanza nada.

Comprobado con el `.exe` real y PostgreSQL 18.6 en una carpeta con espacios y acentos («Datos José Ñandú»):

| Caso                                                              | Resultado                                                                       |
| ----------------------------------------------------------------- | ------------------------------------------------------------------------------- |
| Primer arranque (crear clúster, rol, base, 21 migraciones)        | listo en 26 s                                                                   |
| Cierre de la entrada estándar (como al cerrar la aplicación)       | apagado ordenado en 4.7 s, código 0, **ningún proceso de PostgreSQL restante** |
| Segundo arranque                                                  | 2.7 s                                                                           |
| Servidor terminado a la fuerza (PostgreSQL sigue vivo)            | el siguiente arranque adopta PostgreSQL: 1.8 s                                 |
| Todo terminado a la fuerza (`postmaster.pid` huérfano)             | se borra el pid huérfano, PostgreSQL se recupera: 5.7 s                         |
| Segundo servidor sobre la misma carpeta                           | rechazado («la carpeta de datos ya está en uso»), el primero sigue sirviendo   |
| Carpeta en forma corta (`EXPEDI~1`) y larga mezcladas              | corregido: todas las comparaciones usan la forma larga de Windows               |

**Respaldos (§3.8).** `ad-el-pos-server backup`, `restore-check` y `restore` usan los `pg_dump` y `pg_restore`
incluidos y funcionan con la aplicación abierta o cerrada (cerrada, arrancan y detienen PostgreSQL). `restore` exige
la aplicación cerrada, solo acepta respaldos de la carpeta `backups` y antes crea un respaldo de seguridad
(`…-antes-de-restaurar.dump`). Comprobado: respaldo creado y legible, ensayo de restauración con 134 tablas de
conteos idénticos, restauración real y ensayo del respaldo de seguridad; rechazo sin `--file` y de un archivo
fuera de `backups`. Los mensajes para el restaurante van a la consola y el detalle técnico a
`logs\maintenance.log`.

**Secretos (§3.3).** Mismo `vault.rs` con el backend `windows-native` de `keyring` (Administrador de credenciales).
La prueba real del almacén, que solo existía para macOS, ahora también corre en Windows y pasó: escribió, leyó y
borró una credencial temporal. El aviso de espera del almacén dice «Revisa el aviso de Windows» en Windows y sigue
diciendo «Revisa el diálogo de macOS» en macOS.

**Ventana (§3.7).** `window_state.rs` (solo Windows): sigue abriendo en pantalla completa como en macOS, pero si se
cerró en modo ventana vuelve con el mismo tamaño, posición y maximización (`window-state.json` en la carpeta de
configuración de la aplicación; nunca datos del restaurante) y al salir de pantalla completa regresa a la última
geometría en modo ventana. Una posición fuera de los monitores conectados no se aplica.

Pruebas: `go test ./internal/standalone` con 8 pruebas, incluida la de ciclo completo con PostgreSQL real
(`ADEL_STANDALONE_PG_BIN`, 26 s); `cargo test --locked` con 46 pruebas (lanzador: solo el servidor local de la
instalación, nada en desarrollo, carpeta de datos y espera creciente; estado de la ventana); `go vet` y `go build`
para Windows y `GOOS=darwin GOARCH=arm64`.
