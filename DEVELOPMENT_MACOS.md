# Desarrollo en macOS

## Entorno de esta entrega

Mac Apple Silicon (`arm64`), macOS 26.5.2, Command Line Tools en `/Library/Developer/CommandLineTools`. Se conservaron Go 1.25.5 y Node 22.15.0 globales. Las herramientas del proyecto son Go 1.27.3, Node 24.22.2 y Rust 1.99.1 en `.tools`; `dev.sh` configura PATH, cachés y homes de Cargo/Rustup sin modificar el shell del usuario.

PostgreSQL 18.6 se instaló con Homebrew. No existía un servidor PostgreSQL instalado detectado. Homebrew creó su clúster predeterminado en `/opt/homebrew/var/postgresql@18`, que **no se inició ni se utiliza**. Este proyecto usa otro clúster dentro de `.local/postgres` en el puerto **55432**. No se ejecutó `brew services start`, no se instaló launchd y no se reemplazaron bases existentes. Homebrew también instaló una actualización de certificados CA como dependencia.

## Requisitos en otro Mac

Revisar primero `uname -m`, `sw_vers`, `xcode-select -p`, `go version`, `node --version`, `cargo --version` y PostgreSQL existente. No ejecutar una reinstalación si ya hay una versión compatible. Con Homebrew disponible, instalar solo lo que falte:

```bash
xcode-select --install              # Solo si faltan Command Line Tools; abre diálogo de Apple
brew install postgresql@18         # Solo si falta un servidor PostgreSQL apropiado
brew install go node@24 rustup      # Solo las herramientas ausentes
rustup-init --profile minimal       # Si se eligió rustup y aún no existe un toolchain
```

Estas instalaciones pueden requerir intervención y cambian el equipo; no forman parte del instalador comercial. Alternativamente, usar distribuciones oficiales de Go/Node y Rustup. No reemplazar PostgreSQL ni actualizar su clúster sin revisar datos y respaldos. Node debe satisfacer los requisitos actuales de Vite (se verificó con 24.22.2); el Node 22.15.0 preexistente no era adecuado para Vite 8.

Para reutilizar herramientas fuera de `.tools`, basta que estén en PATH: el script solo antepone directorios locales si existen. `ADEL_PG_BIN` permite seleccionar otro PostgreSQL; por defecto se usa `/opt/homebrew/opt/postgresql@18/bin` en ARM o `/usr/local/opt/postgresql@18/bin` en Intel. Las pruebas de esta entrega solo cubren ARM64.

## Preparación desde el repositorio

```bash
cd "/ruta/al/ad-el-pos"
./scripts/dev.sh install
./scripts/dev.sh db-init
./scripts/dev.sh migrate
```

`install` instala paquetes npm desde lockfile y módulos Go. Cargo obtiene sus dependencias al compilar Tauri. Para reconstrucción reproducible conservar los tres lockfiles; la primera descarga requiere Internet.

`db-init` genera secretos aleatorios de 48 caracteres, un clúster exclusivo con SCRAM, un administrador de desarrollo local, el rol `adel_app` sin SUPERUSER/CREATEDB/CREATEROLE y la base `adel_pos` propiedad del rol de aplicación. El administrador solo lo usa el script; Go usa `adel_app`. Las credenciales quedan en `.local` con modo 0600, el directorio es 0700. No subirlo, copiarlo a una imagen comercial ni enviar sus logs completos sin revisión.

Repetir `db-init` no reinicializa datos ni rota contraseñas. Comprueba un marcador de propiedad del clúster. Si falla una inicialización parcial, revisar el estado antes de retirar archivos; el script no borra directorios ajenos. En un PostgreSQL existente, crear manualmente un rol/base dedicados y un JSON privado con la plantilla, sin ejecutar `db-init` contra sus datos.

## Ejecución diaria

```bash
./scripts/dev.sh db-start
./scripts/dev.sh server-start
./scripts/dev.sh desktop
```

El primer comando inicia solo el PostgreSQL del proyecto. El segundo construye Go y lo inicia como proceso separado con log `.local/server.log`. El tercero ejecuta Vite/Tauri; no inicia Go ni DB. El servidor también puede ejecutarse en primer plano en otra terminal:

```bash
./scripts/dev.sh server
# Equivalente, una vez construido:
./bin/ad-el-pos-server serve --config .local/server.json
```

Cerrar la terminal que aloja Go en primer plano puede detenerlo; para independencia de esa terminal usar `server-start` o, en el futuro, launchd. Cerrar la ventana Tauri no envía señales a Go.

```bash
./scripts/dev.sh status
curl --fail http://127.0.0.1:8181/api/v1/health/live
curl --fail http://127.0.0.1:8181/api/v1/health/ready
./scripts/dev.sh server-stop
./scripts/dev.sh db-stop
```

Tras reiniciar el Mac, ejecutar `db-start` y `server-start` antes de abrir el `.app`. No hay autoinicio instalado.

## Configuración y modos

Go lee `.local/server.json` por defecto. Para otro archivo: `ADEL_SERVER_CONFIG=/ruta/privada/server.json ./scripts/dev.sh server-start` o flag `--config`. `ADEL_DB_PASSWORD` sustituye únicamente la contraseña DB; no ponerla en scripts versionados ni argumentos de proceso. JSON desconocido/adicional es error. `check-config` valida sintaxis/reglas, pero no prueba conexión ni autoriza distribución comercial.

Para solo servidor, copiar la configuración privada y cambiar `installation_mode` a `server-only`. Ejecutar Go sin escritorio. Para solo terminal, la misma aplicación lee la configuración de conexión en este orden:

1. Archivo absoluto indicado por `ADEL_TERMINAL_CONFIG`.
2. `connection.json` dentro de `app_config_dir`: en este Mac `~/Library/Application Support/local.adel.pos.dev/connection.json`.
3. Valor de desarrollo compilado desde `config/terminal.development.json`: servidor `http://127.0.0.1:8181`, modo `server-terminal`.

La configuración se lee al iniciar; reiniciar Tauri tras editarla. Archivo inválido produce un mensaje, sin sustituirlo silenciosamente por una conexión distinta.

```bash
ADEL_TERMINAL_CONFIG="$PWD/config/terminal.only.example.json" ./scripts/dev.sh desktop
```

Editar la IP del ejemplo antes de usarlo. Para una prueba local sin otra máquina, copiar el ejemplo a `.local/terminal-only.json`, conservar `terminal-only` y cambiar URL a `http://127.0.0.1:8181`. No se requiere recompilar el `.app`; para abrirlo con variable, ejecutar directamente su binario desde esa terminal. Finder usa el archivo persistente o el valor compilado.

La URL no puede contener credenciales, query, fragmentos ni rutas. HTTP sin TLS solo admite localhost o una IP privada explícita. Para pruebas LAN, Go debe escuchar en la IP privada específica y tener `allow_lan=true`; no se habilitó esta exposición durante la entrega. Configurar firewall y TLS/autenticación antes de uso operativo. No mapear el puerto del router.

## Migraciones y respaldo

```bash
./scripts/dev.sh backup
./scripts/dev.sh migrate
```

Antes de migrar una base con información importante: revisar SQL, crear backup, **restaurarlo en una base temporal distinta** y validar. `backup` usa formato custom, permisos privados y `pg_restore --list`; no afirma haber probado restauración. El script usa `ADEL_SERVER_CONFIG` cuando está definido. No hay restauración destructiva automatizada ni rollback automático de versiones ya confirmadas.

Para probar manualmente un respaldo, crear una base temporal vacía con un usuario autorizado y ejecutar `pg_restore --exit-on-error --no-owner --no-acl --dbname BASE_TEMPORAL archivo.dump`; comprobar registros/versiones antes de eliminar exclusivamente esa base temporal. No restaurar encima de `adel_pos` como prueba.

## Pruebas y compilación

```bash
./scripts/dev.sh test               # Go -race/vet, frontend tests/types/build, Rust tests
./scripts/dev.sh test-integration   # Base aleatoria adel_pos_test_*; borrado en finally
./scripts/dev.sh build-server
./scripts/dev.sh build-macos
open "desktop/src-tauri/target/release/bundle/macos/AD-EL POS.app"
```

Se construye solo `.app` macOS para la arquitectura actual, con mínimo declarado macOS 14 por la base WebKit/CSS moderna; solo se probó macOS 26.5.2. No se genera instalador Windows ni DMG. La aplicación contiene frontend y ejecutable Tauri, no Go, Node, Rust ni PostgreSQL. El `.app` puede abrirse sin Vite; necesita que el servidor seleccionado esté activo. Firma de distribución/notarización Apple y aplicación universal Intel+ARM quedan pendientes; el artefacto local no es un instalador comercial.

Pruebas operativas opcionales, **solo en el entorno de desarrollo propio**: `python3 tests/runtime_smoke.py` detiene/recupera temporalmente DB y Go, cierra/reabre la aplicación y comprueba la independencia; deja los componentes activos. `python3 tests/runtime_smoke.py --terminal-only` verifica el mismo `.app` con configuración externa de solo terminal y vuelve al modo normal. No ejecutarlas durante trabajo con datos reales ni con otro proceso de Vite/Tauri consultando el mismo backend.

## Errores habituales

| Síntoma | Comprobación |
| --- | --- |
| Servidor desconectado | `server-start`, `status`, dirección en configuración, `.local/server.log`. |
| PostgreSQL no disponible | `db-start`, puerto 55432, credenciales privadas; `/live` puede seguir devolviendo 200. |
| Esquema incompatible | Ejecutar `migrate` con binario correcto. No editar checksums ni migraciones aplicadas para ocultar el problema. |
| Puerto ocupado | Revisar el proceso propietario. No matarlo indiscriminadamente; elegir un puerto libre y actualizar ambas configuraciones. |
| `Operation not permitted` al iniciar PostgreSQL/Go | Un entorno de agente aislado puede bloquear memoria compartida, puertos o inspección de procesos. Ejecutar en Terminal o autorizar esa ejecución concreta. |
| Error npm/Vite por versión Node | Usar `dev.sh`, que selecciona el Node local; revisar requisitos oficiales antes de actualizar. |
| Fallo Rust por SDK | `xcode-select -p`, Command Line Tools y licencia de Xcode si Apple la solicita. |
| `.app` sin respuesta del servidor después de reiniciar | El backend y DB no tienen servicio automático; arrancarlos por separado. |

## Servicio futuro

`ops/launchd/local.adel.pos.server.plist.example` es una plantilla, no una instalación. Antes de usarla sustituir todas las rutas absolutas, validar con `plutil -lint` y decidir el usuario/directorios de datos. No ejecutar simultáneamente `server-start` y launchd en el mismo puerto. Para uso durante sesión de usuario se puede preparar un LaunchAgent; para servicio persistente comercial sin sesión, diseñar un LaunchDaemon y su usuario de servicio. La instalación de servicios privilegiados requiere una decisión explícita posterior.
