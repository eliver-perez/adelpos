# Etapa 9E · Avance (Windows 11: compilación, pruebas e instalador)

Especificación del propietario: [docs/etapas/ETAPA_9E_WINDOWS.md](etapas/ETAPA_9E_WINDOWS.md) (solo lectura). Rama
`etapa-9e-windows`, creada desde `main` (`6763379`, etapa 9D con la versión 0.9.3 y el esquema 21, comprobados en
`config/product.json` y `CLAUDE.md`). Se trabaja en una computadora con Windows 11 sobre un clon recién hecho del
repositorio. Este archivo registra solo lo que realmente se ejecutó.

## Fase 0 · Verificación del entorno y del código (12/11/2026)

### 1. Equipo

| Dato         | Valor                                                                                               |
| ------------ | --------------------------------------------------------------------------------------------------- |
| Windows      | Windows 11 Pro 25H2, versión 10.0.26200, compilación 26200.9168                                     |
| Arquitectura | x64 (64 bits); procesador Intel Core i7-1355U (10 núcleos, 12 hilos)                                |
| Memoria      | 31.7 GB (16.7 GB libres al iniciar)                                                                 |
| Disco C:     | 1 331 GB libres (530 GB usados)                                                                     |
| Usuario      | `Expediente`, perfil `C:\Users\Expediente` (sin espacios ni acentos); la sesión **no** está elevada |
| Directivas   | `ExecutionPolicy` del equipo `RemoteSigned`; la del proceso de esta sesión, `Bypass`                |

### 2. Herramientas encontradas

| Herramienta                                                         | Estado                                                                                                 |
| ------------------------------------------------------------------- | ------------------------------------------------------------------------------------------------------ |
| Git                                                                 | 2.56.0.windows.4 (`C:\Program Files\Git`); `core.autocrlf=true` en el `gitconfig` del sistema          |
| Go                                                                  | **no instalado** (el proyecto usa 1.27.3; `go.mod` declara 1.27.0)                                     |
| Node / npm                                                          | 20.19.6 / 10.8.2 instalados en todo el equipo (`C:\Program Files\nodejs`); el proyecto usa **24.22.2** |
| Gestor de paquetes del proyecto                                     | npm con `package-lock.json` (`npm ci`)                                                                 |
| Rust (`rustup`, `x86_64-pc-windows-msvc`)                           | **no instalado** (el proyecto usa 1.99.1, `Cargo.toml` `rust-version = "1.99"`)                        |
| Visual Studio Build Tools («Desarrollo para el escritorio con C++») | **no instalado** (no existe `vswhere.exe`)                                                             |
| WebView2                                                            | 157.0.4341.46 (Evergreen, en todo el equipo)                                                           |
| CLI de Tauri                                                        | se instala con `npm ci` (`@tauri-apps/cli` 2.12.4 fijado en `package.json`)                            |
| PostgreSQL                                                          | **no instalado** (el proyecto usa 18.6)                                                                |
| Python                                                              | solo el alias de Microsoft Store (no hay intérprete)                                                   |
| Compilador de C para `cgo` (`go test -race`)                        | no instalado                                                                                           |
| winget                                                              | 1.29.380                                                                                               |

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

| Herramienta                    | Versión                                    | Dónde y cómo                                                                                                                                                                                                                                                                 |
| ------------------------------ | ------------------------------------------ | ---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Go                             | 1.27.3 windows/amd64                       | `.tools\go`, zip oficial de go.dev (SHA-256 `f26081f4…7885fff`, igual al publicado)                                                                                                                                                                                          |
| Node / npm                     | 24.22.2 / 11.19.1                          | `.tools\node`, zip oficial de nodejs.org (SHA-256 `ca1bff5d…7799e2`, igual a `SHASUMS256.txt`); el Node 20 del equipo no se tocó                                                                                                                                             |
| Rust                           | rustc 1.99.1, cargo 1.99.1, rustfmt 1.10.0 | `.tools\rustup` y `.tools\cargo` con `rustup-init.exe` oficial (SHA-256 `38027d51…63eec7db`, igual al publicado); destino `x86_64-pc-windows-msvc`, perfil mínimo, sin modificar el `PATH` del usuario                                                                       |
| Visual Studio Build Tools 2022 | 17.14.37906.3 (paquete winget 17.14.45)    | winget `Microsoft.VisualStudio.2022.BuildTools` con la carga `Microsoft.VisualStudio.Workload.VCTools` y los componentes recomendados (MSVC x64 y SDK de Windows 11 10.0.26100); por equipo, con una confirmación de Control de cuentas de usuario; `isRebootRequired=false` |
| PostgreSQL                     | 18.6 (binarios EDB 18.6-1)                 | `.tools\pgsql`, `postgresql-18.6-1-windows-x64-binaries.zip` de get.enterprisedb.com (SHA-256 `fbe23da2…0252f8c`; EDB no publica la suma del zip). Sin instalador ni servicio                                                                                                |
| CLI de Tauri                   | 2.12.4                                     | `npm ci` en `desktop` (`package-lock.json` sin cambios)                                                                                                                                                                                                                      |
| Módulos de Go                  | los de `go.sum`                            | `go mod download` a `.cache\go-mod`                                                                                                                                                                                                                                          |

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

| Suite                                                                                                           | Resultado en Windows                                                                                                                                              |
| --------------------------------------------------------------------------------------------------------------- | ----------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `go test ./...` (sin `-race`: no hay compilador de C para cgo)                                                  | 22 paquetes aprobados y 1 falla: `TestTLSRequiresTrustedCAAndCorrectHostname` (corregida, ver abajo)                                                              |
| `go test -tags=integration` (`dev.ps1 test-integration`, PostgreSQL 18.6 real, una base desechable por paquete) | **los 9 paquetes aprobados** (`database`, `administration`, `catalog`, `operations`, `payments`, `reports`, `reports/operational`, `designer`, `identity`; 559 s) |
| Vitest (`npm test`)                                                                                             | 750 de 751 aprobadas; 1 intermitente (corregida, ver abajo)                                                                                                       |
| `npm run build` (TypeScript + Vite)                                                                             | aprobado                                                                                                                                                          |
| `cargo test --locked`                                                                                           | al principio no compilaba (faltaba `icons/icon.ico`, ver abajo); después **41 aprobadas**                                                                         |
| Playwright Chromium (`dev.ps1 test-browser`, Go temporal en 8183)                                               | **19 de 19 aprobadas** (7.6 min); WebKit se ejecuta después, junto con la fase C                                                                                  |

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

| Caso                                                         | Resultado                                                                      |
| ------------------------------------------------------------ | ------------------------------------------------------------------------------ |
| Primer arranque (crear clúster, rol, base, 21 migraciones)   | listo en 26 s                                                                  |
| Cierre de la entrada estándar (como al cerrar la aplicación) | apagado ordenado en 4.7 s, código 0, **ningún proceso de PostgreSQL restante** |
| Segundo arranque                                             | 2.7 s                                                                          |
| Servidor terminado a la fuerza (PostgreSQL sigue vivo)       | el siguiente arranque adopta PostgreSQL: 1.8 s                                 |
| Todo terminado a la fuerza (`postmaster.pid` huérfano)       | se borra el pid huérfano, PostgreSQL se recupera: 5.7 s                        |
| Segundo servidor sobre la misma carpeta                      | rechazado («la carpeta de datos ya está en uso»), el primero sigue sirviendo   |
| Carpeta en forma corta (`EXPEDI~1`) y larga mezcladas        | corregido: todas las comparaciones usan la forma larga de Windows              |

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

### C2 · Impresión, cajón, menú nativo y comprobación para macOS

**Impresión y cajón (§3.5)**, hecha por un agente con el diseño indicado y revisada antes de integrarla (sin
migración: el transporte guardado como `cups` pasa a significar «cola de impresión del equipo»):

- En **Windows** esa cola es la impresora instalada en Windows y recibe **ESC/POS en modo RAW** por la API del
  administrador de impresión (`winspool.drv` cargada solo desde System32: `OpenPrinter`, `StartDocPrinter` con
  `RAW`, `WritePrinter`, `EndDocPrinter`, `AbortPrinter`), con los mismos bytes que ESC/POS por red: texto con
  `ESCPosBytes`, documentos con formato y logotipo con `documents.EncodeESCPos` (la marca «Esta impresora entiende
  ESC/POS» no aplica en Windows) y el pulso del cajón con `DrawerPulseBytes`. Antes de cada envío se consulta la
  impresora (solo lectura) y se rechazan, sin enviar nada y con el motivo, las remotas, de red, de Escritorio
  remoto o de la nube, las virtuales (PDF, XPS, OneNote, fax), `FILE:`, `PORTPROMPT:`, `NUL:`, puertos WSD y
  monitores desconocidos; se aceptan puertos USB, COM, LPT y TCP/IP estándar. Resultados: error si nada se envió,
  incierto (nunca se reintenta) si falló después de empezar el documento, y aceptado con «Aceptado por la cola de
  impresión de Windows; no confirma salida física de papel.». «Consultar impresoras del equipo servidor» lista
  las impresoras instaladas en Windows (`EnumPrinters`).
- En **macOS** nada cambia: el despacho (`kitchen/system_queue*.go`) devuelve CUPS fuera de Windows y las
  pruebas con los programas de CUPS simulados siguen corriendo en ambos sistemas.
- Interfaz: `isWindows()` en `commands/platform.ts` y textos de Windows («Impresora instalada en Windows»,
  «Usar esta impresora») en la configuración de impresoras, la cola, el cajón, la identidad, el diseñador y las
  muestras; en Windows se ocultan la opción de corte de CUPS y la casilla «entiende ESC/POS». En macOS todos
  los textos quedan idénticos (prueba `printersWindows.test.tsx`).
- No se usó ninguna impresora física: la única del equipo es «Microsoft Print to PDF» y con ella solo se hicieron
  consultas de lectura (la prueba confirma que se rechaza como virtual).

**Menú nativo (§3.6)**: en Windows aparecía como barra clásica «Archivo Edición Ver Caja Ayuda» incluso en pantalla
completa (captura en modo desarrollo). No es táctil y repite la barra superior, la paleta (Ctrl+K) y los atajos con
Ctrl, que atiende la capa de React; **se omite en Windows** (`with_native_menu` en `main.rs`), sin errores:
`menu_subscribe` y `menu_set_state` siguen respondiendo y el registro de acciones no cambia. En macOS el menú sigue
igual. Para cerrar la aplicación en Windows: salir de pantalla completa y cerrar la ventana, o Alt+F4.

**Modo desarrollo de la interfaz (§2.3)**: `dev.ps1 desktop` (`tauri dev`) abrió la aplicación en pantalla completa
con el asistente inicial, la versión y la zona horaria del sistema (America/Mexico_City) contra el servidor de
desarrollo.

**Comprobación para macOS** (solo verificación; no se genera nada para Mac): `cargo check --locked` y
`cargo check --tests` con el destino `aarch64-apple-darwin` aprobados **sin errores ni avisos**, con el Tauri real
de macOS. La dependencia `objc2-exception-helper` compila un archivo de Objective-C con el compilador de Apple; para
`cargo check`, que no enlaza, se usó un compilador sustituto que solo produce archivos vacíos, fuera del
repositorio. `GOOS=darwin GOARCH=arm64 go build ./...` y `go vet ./...` aprobados.

Pruebas de C2: `go test ./...` (25 paquetes aprobados), `go vet ./...` para Windows y macOS, integración con
PostgreSQL de `designer`, `payments` y `operations` aprobada (74 s), Vitest de las áreas tocadas (236 pruebas) y
completo por el agente (756), `tsc --noEmit`, `cargo test --locked` (46) y Prettier del repositorio completo.

## Fase D · Instalador NSIS (12/11/2026)

Versión **0.9.4** en `config/product.json` (sincronizada con `dev.ps1 sync`; en los lockfiles solo cambia la versión
del propio paquete). Sin migraciones: el esquema sigue en 21.

**Decisión (0-B)**: instalador **por usuario** (`installMode: currentUser`), sin permisos de administrador.

| Qué                                             | Dónde                                                                                                                                     |
| ----------------------------------------------- | ----------------------------------------------------------------------------------------------------------------------------------------- |
| Programa                                        | `%LOCALAPPDATA%\Programs\AD-EL POS\` (`ad-el-pos-desktop.exe`, `ad-el-pos-server.exe`, `postgresql\`, `uninstall.exe`)                    |
| Datos del restaurante                           | `%LOCALAPPDATA%\AD-EL POS\` (clúster `postgres\`, `server.json`, secretos, `logs\`, `backups\`), con ACL privada                          |
| Preferencias de la ventana y perfil de WebView2 | `%APPDATA%\local.adel.pos.dev\` y `%LOCALAPPDATA%\local.adel.pos.dev\` (las únicas que borra la casilla «borrar datos» del desinstalador) |
| Credenciales de la terminal                     | Administrador de credenciales de Windows (destino con `local.adel.pos.dev`)                                                               |

Piezas (solo para Windows; `tauri.conf.json` de macOS no cambió más que la versión):

- `desktop/src-tauri/tauri.windows-installer.conf.json`, que se combina solo al empaquetar (`--config`; `tauri dev`
  no lo usa): destino `nsis`, icono `icons/icon.ico`, el servidor como binario externo
  (`binaries/ad-el-pos-server-x86_64-pc-windows-msvc.exe` → `ad-el-pos-server.exe`), PostgreSQL como recurso,
  instalador de arranque de WebView2 incluido (`embedBootstrapper`), idioma español, carpeta «AD-EL POS» en el menú
  Inicio y casilla de acceso directo en el escritorio (opcional, en la última página).
- `desktop/src-tauri/windows/installer.nsi`: copia de la plantilla NSIS de Tauri 2.12.4 con dos cambios marcados
  «AD-EL POS»: la carpeta de instalación por usuario es `%LOCALAPPDATA%\Programs\AD-EL POS` (la original usaba
  `%LOCALAPPDATA%\AD-EL POS`, la carpeta de datos) y, después de cerrar la aplicación abierta, se espera a que el
  servidor local y PostgreSQL terminen antes de reemplazar o borrar sus archivos.
- `desktop/src-tauri/windows/hooks.nsh` (UTF-8 con BOM): la espera anterior (si PostgreSQL quedó activo tras un
  cierre anómalo, `pg_ctl stop -m fast`, que conserva los datos) y dos accesos directos de mantenimiento en el menú
  Inicio: «Crear respaldo de AD-EL POS» (`ad-el-pos-server.exe backup --pause`) y «Respaldos de AD-EL POS» (abre la
  carpeta de respaldos). Ningún paso borra la carpeta de datos.
- PostgreSQL incluido, solo lo que se usa (64.4 MB sin comprimir): `initdb`, `pg_ctl`, `postgres`, `pg_dump`,
  `pg_restore` y `psql` con sus 13 DLL (obtenidas con `dumpbin /dependents`), PL/pgSQL, diccionarios Snowball y
  conversiones de codificación, `share` sin traducciones (22 MB menos) y las licencias de PostgreSQL. El runtime de
  Visual C++ (`vcruntime140.dll`, `vcruntime140_1.dll`, `msvcp140.dll`) va junto a PostgreSQL porque los binarios
  de EDB no lo traen; la aplicación de escritorio no lo necesita (solo depende de DLL del sistema y del CRT universal).
- `scripts/build-windows.ps1`: todo de principio a fin con un solo comando (herramientas, metadatos, servidor,
  PostgreSQL reducido con comprobación por `initdb`, instalador y su SHA-256).

Resultado de `powershell -ExecutionPolicy Bypass -File scripts\build-windows.ps1` (4 min con las cachés de Rust y
Go; Tauri descargó NSIS 3.13, `nsis_tauri_utils` 0.5.3 con su hash y el instalador de arranque de WebView2):

- **`desktop\src-tauri\target\release\bundle\nsis\AD-EL POS_0.9.4_x64-setup.exe`**
- **36.6 MB** (38 412 295 bytes), SHA-256 `a975ef448a7148edb5855e50d53af6a2709a77c88d37e7179bff158bf617e8fe`
- Sin firma digital (es normal el aviso de SmartScreen).

## Fase E · Instalación real y pruebas (12/11/2026)

Herramientas: `scripts/windows/verify-installed.ps1` (instalación en silencio, arranque, ventanas de consola,
memoria, cierre ordenado, desinstalación con datos conservados; informe JSON) y `scripts/windows/installed-smoke.ps1`
con `desktop/e2e-installed/` y `desktop/playwright.installed.config.ts` (Playwright conectado por CDP al WebView2 de
la aplicación **instalada**; `save-dialog.ps1` contesta el diálogo nativo «Guardar reporte» con mensajes de Win32).

Hecho y aprobado con el instalador 0.9.4:

1. **Instalación limpia** (en silencio, 5.8 s): ventana en 2.4 s, servidor listo en 14 s (crea el clúster y aplica
   21 migraciones), **0 ventanas de consola**, «Aplicaciones instaladas» con AD-EL POS 0.9.4, menú Inicio con los
   3 accesos y acceso en el escritorio, datos en `%LOCALAPPDATA%\AD-EL POS` con ACL privada.
2. **Prueba de humo, 8 pasos** sobre la aplicación instalada: asistente inicial («Fonda Doña Ñeli») y admin/admin;
   catálogo, cocina, impresora de tickets y pago en dólares; área y 2 mesas; captura en mesa, envío a cocina, turno
   con dólar a $18.50, abono en efectivo con cambio, precuenta, descuento y tarjeta; para llevar con pesos y dólares;
   cancelación con motivo; corte parcial y cierre de turno; «Ventas por día» exportado a PDF y Excel con el diálogo
   nativo. PDF abierto en Edge y Excel revisado: acentos y «ñ» correctos, logotipo de AD-EL POS en el encabezado,
   venta neta $285.00 (la cuenta cancelada no cuenta).
3. **Persistencia**: cerrar y abrir; la sesión vuelve del Administrador de credenciales y los datos siguen.
   Arranque con datos: ventana 1.8 s, servidor 1.6 s. Memoria con la aplicación abierta ≈ 750 MB de conjunto de
   trabajo (≈ 390 MB privados: WebView2 178, servidor 122, PostgreSQL 85, aplicación 6.5).
4. **Respaldo y restauración** en la instalación (comando del acceso «Crear respaldo»): respaldo con la aplicación
   abierta, ensayo con 134 tablas idénticas, negativa a restaurar con la aplicación abierta y restauración real
   (una categoría creada después del respaldo desapareció; se creó el respaldo de seguridad previo).
5. **Actualización**: instalador de prueba 0.9.5 encima de la 0.9.4 con la aplicación abierta (se cierra, espera a
   Go y PostgreSQL, conserva datos, una sola entrada en Aplicaciones) y regreso a 0.9.4.
6. **Desinstalación**: quita programa, menú Inicio y entrada; conserva base, configuración, respaldos y credencial.

Defectos reales de Windows encontrados en esta fase y corregidos (con pruebas):

- Mapa de mesas que oscilaba sin fin con la pantalla al 125 % (barras que ocupan espacio y medidas redondeadas):
  `canvasStyle` en `TableMap.tsx`.
- El cierre de Go esperaba 5 s con un flujo SSE abierto y salía con error: los flujos terminan al empezar el
  apagado (`Service.Stopping`); cierre ahora en ≈ 1 s.
- `dev.ps1 db-start` se quedaba esperando si su salida se canalizaba: `pg_ctl` se lanza sin heredar la tubería.
- **Usuario con acentos**: con la carpeta del programa con acentos (por ejemplo `C:\Users\José\…`) `initdb` fallaba
  («invalid byte sequence for encoding UTF8»), porque escribe la ruta de `share` en una sentencia SQL con la
  codificación de Windows. Los programas de PostgreSQL se ejecutan ahora por la ruta corta 8.3 (ASCII); sin nombres
  cortos se explica el problema. Probado con PostgreSQL copiado a «Programa José Ñandú» y datos en «Datos José
  Ñandú» (`TestClusterLifecycleWithBundledPostgreSQL` y `TestToolsRunFromAnASCIIPathWhenTheProgramFolderHasAccents`).
- Icono: el de la etapa 1 era una «A» verde provisional; ahora es el símbolo de la marca (Windows y macOS).
- Respaldo antes de migrar sobre datos existentes: `TestMigrateWithBackupProtectsExistingData` (PostgreSQL real).

### Usuario con acentos e instalador final (reanudación del 12/11/2026)

Se quitó la instalación de prueba anterior (`Prueba José Ñandú`) y sus datos temporales, y se recompiló con
`scripts\build-windows.ps1` (incluye la corrección de acentos):

- **`desktop\src-tauri\target\release\bundle\nsis\AD-EL POS_0.9.4_x64-setup.exe`**, **36.7 MB** (38 465 086 bytes),
  SHA-256 `e04c87b8c61e5c3ed736dc9a068ed3739189fc3b6b7bca2bc21f144439c43674`, sin firma.

**Usuario con acentos (§5.6), simulado sin crear cuentas de Windows**: instalación en silencio con
`/D=%TEMP%\adel9e-acentos\Programa José Ñandú\AD-EL POS` (8.8 s) y la aplicación abierta con `LOCALAPPDATA` en
`…\Usuario José Ñandú\AppData\Local` y un perfil de credenciales aparte (`ADEL_TERMINAL_CONFIG` con
`credential_namespace` propio, para no tocar la credencial de la terminal real). Resultado: servidor listo en 15.7 s
(clúster creado y 21 migraciones) con la carpeta de datos en «Usuario José Ñandú», PostgreSQL ejecutado por la ruta
corta 8.3, asistente inicial («Prueba José Ñandú») e inicio de sesión con admin/admin (Playwright por CDP), cierre
ordenado en 4.7 s sin procesos ni `postmaster.pid`, desinstalación con los datos conservados. Después se borraron
la credencial del perfil de prueba y las carpetas temporales; la credencial real (`…8181|main`) y los datos reales
(2 033 archivos) no cambiaron. **Límite**: el perfil de WebView2 y la carpeta de configuración de la ventana los
resuelve Windows por su API de carpetas conocidas (no por la variable), así que siguieron en la carpeta real del
usuario; solo una cuenta de Windows con acentos los pondría en una ruta con acentos.

**Observación del instalador** (comportamiento de la plantilla de Tauri, sin cambio): el instalador recuerda la última
carpeta de instalación en `HKCU\Software\AD-EL POS\AD-EL POS` y la reutiliza aunque se haya desinstalado; solo la
olvida la casilla «borrar datos de la aplicación» del desinstalador. Por eso el primer intento de instalar en la
carpeta predeterminada volvió a la carpeta de prueba; se desinstaló, se borró ese valor y se repitió. El propietario
no lo verá mientras instale siempre en la carpeta predeterminada.

**Instalación final** en `%LOCALAPPDATA%\Programs\AD-EL POS` sobre los datos de «Fonda Doña Ñeli»
(`verify-installed.ps1`): instalación en silencio en 7.1 s, 108.8 MB en disco; ventana en 2.39 s y servidor listo en
3.06 s (con datos); **0 ventanas de consola**; memoria con la aplicación abierta ≈ 790 MB de conjunto de trabajo
(≈ 403 MB privados: WebView2 186, servidor 122, PostgreSQL 89, aplicación 6.5); cierre ordenado en 0.61 s sin
procesos restantes; menú Inicio con los 3 accesos y acceso en el escritorio. Al volver a abrirla, la sesión volvió
del Administrador de credenciales y mostró «Salón principal» con Mesa 1 y Mesa 2. **Queda instalada y lista** (cerrada)
para el propietario.
