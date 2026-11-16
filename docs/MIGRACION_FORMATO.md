# Paquete de migración del sistema anterior · Formato `.adelmig` v1

Etapa 9H, Fase B (§3.2 de [etapas/ETAPA_9H.md](etapas/ETAPA_9H.md)). Estructura del archivo que produce el lector del
respaldo (`ad-el-pos-server legacy-prepare`) y que lee el importador, en cualquier computadora con AD-EL POS y sin
Docker. **Solo estructura**: el paquete contiene datos reales del restaurante y nunca entra al repositorio
(`.gitignore` con `*.bak`, sus versiones comprimidas `.bak.gz`, `.bak.zip` y `.bak.7z`, `*.mdf`, `*.ndf`, `*.ldf`,
`*.trn`, `*.adelmig` y `legacy-data/`, y `scripts/check-legacy-data.sh`, que además reconoce por su contenido un paquete
o un respaldo renombrados, también un respaldo dentro de un gzip). La estructura de la base de origen está en
[MIGRACION_ANALISIS.md](MIGRACION_ANALISIS.md).

Código: `backend/internal/legacypkg` (formato, neutral), `backend/internal/legacyread` (lector, con Docker; solo lo usa
el subcomando) y `backend/internal/legacysynth` (base y paquete sintéticos de las pruebas).

## 1. Archivo

```text
┌──────────────────────────── encabezado en claro ────────────────────────────┐
│ firma (8 bytes) │ versión (uint16) │ longitud L (uint32) │ JSON (L bytes) │ SHA-256 (32 bytes) │
└─────────────────────────────────────────────────────────────────────────────┘
┌──────────────────────── cuerpo: flujo cifrado por bloques ──────────────────┐
│ bloque 0 │ bloque 1 │ … │ último bloque                                     │
└─────────────────────────────────────────────────────────────────────────────┘
        texto claro = gzip( tar( manifest.json, tables/<tabla>.json, … ) )
```

- Enteros en orden de bytes de red (big endian).
- **Firma**: `41 44 45 4C 4D 49 47 00` («ADELMIG» y un byte NUL, para que ningún archivo de texto empiece igual).
- **Versión del formato**: 1. Una versión mayor que la que conoce AD-EL POS es «paquete de una versión más nueva»; 0 es
  «paquete dañado».
- **JSON de parámetros** (1 a 4096 bytes; campos exactos, sin campos de más):

  | Campo            | Valor en v1                                                    |
  | ---------------- | -------------------------------------------------------------- |
  | `kdf.name`       | `argon2id`                                                     |
  | `kdf.salt`       | 32 bytes aleatorios (base64)                                   |
  | `kdf.time`       | pasadas de Argon2id (al escribir: 3)                           |
  | `kdf.memory_kib` | memoria de Argon2id en KiB (al escribir: 65536 = 64 MiB)       |
  | `kdf.threads`    | hilos de Argon2id (al escribir: 4)                             |
  | `cipher`         | `AES-256-GCM`                                                  |
  | `chunk_size`     | bytes de texto claro por bloque (al escribir: 1048576 = 1 MiB) |
  | `nonce_prefix`   | 7 bytes aleatorios (base64)                                    |
  | `key_check`      | valor de comprobación de la llave, 32 bytes (base64)           |
  | `compression`    | `gzip`                                                         |
  | `archive`        | `tar`                                                          |

- **SHA-256 del encabezado**: de la firma hasta el final del JSON. Detecta un encabezado dañado sin conocer la frase.

## 2. Llaves y cifrado

- Frase de paso: la elige el propietario (mínimo 8 caracteres, máximo 1024 bytes), normalizada a Unicode NFC (la misma
  frase escrita en una Mac o en Windows da la misma llave). Llega al subcomando **solo por la entrada estándar**.
- Llave maestra = Argon2id(frase, sal, `time`, `memory_kib`, `threads`, 32 bytes).
- Subllaves con HKDF-SHA-256 (sal = la sal del encabezado): cifrado = HKDF(maestra, `adelmig v1 encryption key`), y
  comprobación = HKDF(maestra, `adelmig v1 key check key`).
- `key_check` = HMAC-SHA-256(subllave de comprobación, `adelmig v1 key check value`). Si no coincide, la frase de paso es
  incorrecta (se distingue de un paquete dañado antes de descifrar nada).
- Cuerpo: el texto claro se parte en bloques de `chunk_size` bytes; todos son completos salvo el último (que puede ir
  incompleto o vacío). Cada bloque es AES-256-GCM (texto cifrado y etiqueta de 16 bytes) con:
  - nonce de 12 bytes = `nonce_prefix` (7) ‖ número de bloque (uint32, desde 0) ‖ bandera de último bloque (1 byte: 1
    en el último, 0 en los demás);
  - AAD = SHA-256 del encabezado completo (con su SHA-256).
- Un bloque completo es el último solo si el archivo termina justo después; uno incompleto siempre es el último. Así un
  byte alterado, un bloque reordenado, duplicado o quitado, el último bloque perdido, un archivo truncado o datos de más
  al final hacen fallar el descifrado: «paquete dañado».

## 3. Contenido (texto claro)

`gzip` de un solo miembro con un `tar` de entradas regulares, en este orden exacto:

1. `manifest.json` (primero, para leerlo sin recorrer el paquete).
2. `tables/<tabla>.json`, una por tabla extraída, en el orden del manifiesto. Si el nombre de la tabla no es solo
   letras, números y `_`, la entrada es `tables/_tNNNN.json` (por posición); el nombre real está en el manifiesto.

No hay otras entradas, carpetas ni enlaces, y después del tar no hay nada.

Cada `tables/<tabla>.json` es un arreglo de objetos, uno por fila, con **todas** las columnas del manifiesto **en su
orden** (los nulos como `null`). Los valores:

| Tipo de SQL Server                                                   | En el JSON                                                                 |
| -------------------------------------------------------------------- | -------------------------------------------------------------------------- |
| `int`, `smallint`, `tinyint`, `bigint`                               | número entero                                                              |
| `money`, `smallmoney`, `decimal`, `numeric`                          | número con su texto decimal exacto (`money` con 4 decimales: `150.0000`)   |
| `float`, `real`                                                      | número (puede venir en notación científica)                                |
| `bit` (y las columnas `<columna>_present`)                           | `true` / `false`                                                           |
| `datetime`, `smalldatetime`                                          | texto `AAAA-MM-DDTHH:MM:SS.mmm`, hora local del equipo de origen, sin zona |
| `date`                                                               | texto `AAAA-MM-DD`                                                         |
| `char`, `varchar`, `nchar`, `nvarchar`, `text`, `ntext`, `xml`, etc. | texto (UTF-8; `char(n)` con sus espacios de relleno)                       |

- Los importes **nunca** pasan por `float`: el lector toma el texto que entrega SQL Server (`FOR JSON`) y la lectura los
  entrega como `json.Number`.
- Orden de las filas, determinista: por la llave primaria; sin llave (o si una columna de la llave no viaja), por todas
  las columnas en su orden, las de texto con la intercalación binaria `Latin1_General_BIN2` y después por su
  `DATALENGTH` (SQL Server ignora los espacios del final al comparar). Las filas que empatan son idénticas.

## 4. `manifest.json`

| Campo                 | Contenido                                                                                                                                                                                                                                                                                                                                                                                                                                                                                |
| --------------------- | ---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `format`              | `adelmig`                                                                                                                                                                                                                                                                                                                                                                                                                                                                                |
| `format_version`      | 1 (igual a la del encabezado)                                                                                                                                                                                                                                                                                                                                                                                                                                                            |
| `created_at`          | fecha de creación (RFC 3339, UTC)                                                                                                                                                                                                                                                                                                                                                                                                                                                        |
| `created_by`          | `product`, `version` (de AD-EL POS) y `tool` (`legacy-prepare` o `legacy-synthetic`)                                                                                                                                                                                                                                                                                                                                                                                                     |
| `source`              | origen: `kind` (`sqlserver_backup` o `synthetic`) y del respaldo: `backup_file` (solo el nombre), `backup_bytes`, `backup_sha256`, `database_name`, `backup_started_at` y `backup_finished_at` (hora local de origen), `server_version` (SQL Server que lo hizo), `database_version`, `compatibility_level`, `collation`, `backup_set_position`, `backup_sets` (juegos de respaldo del archivo); del lector: `reader_image` (imagen con su digest), `reader_image_id` y `reader_version` |
| `summary`             | opcional: `sales_table`, `sales_first` y `sales_last` (fecha de la primera y la última venta, de `ventas.fecha`)                                                                                                                                                                                                                                                                                                                                                                         |
| `tables`              | por tabla extraída: `name`, `file`, `rows`, `bytes` (tamaño de su JSON), `sha256` (de su JSON), `primary_key` (nombres en orden), `columns` (`name`, `type` con longitud o precisión, `nullable`, `identity` y, en las de contraseña, `replaces`) y `omitted_columns` (`name`, `type`, `reason`, `rows_with_value`)                                                                                                                                                                      |
| `skipped_tables`      | tablas no extraídas: `name`, `rows` y `reason`                                                                                                                                                                                                                                                                                                                                                                                                                                           |
| `content_fingerprint` | huella de contenido (abajo)                                                                                                                                                                                                                                                                                                                                                                                                                                                              |

**Huella de contenido**: SHA-256 de, por cada tabla de `tables` en orden, `<longitud del nombre>:<nombre>\n<filas>\n<sha256>\n`.
Depende solo de los datos: dos paquetes del mismo respaldo tienen la misma huella aunque se cifren con otra frase. Los
conteos (`rows`) son los que compara la validación del importador (§4.4 de la especificación).

## 5. Lo que no viaja

- **Contraseñas**: toda columna cuyo nombre sea o contenga `password`, `contrasena`, `contraseña`, `passwd`, `pwd` o
  `pass` (sin distinguir mayúsculas) se sustituye en la consulta por `<columna>_present` (`bit`:
  `DATALENGTH(columna) > 0`). El valor nunca sale de SQL Server. En el respaldo real, solo `usuarios.password`.
- **Binarios** (`binary`, `varbinary`, `image`, `timestamp`) y tipos que el formato no lleva (geografía, `sql_variant`…):
  se omiten; `omitted_columns` registra la columna, su motivo (`binary` o `unsupported_type`) y cuántas filas tenían valor.
  En el respaldo real: `productos.portada`, `productos_imagenes.imagen`, `articulos_imagenes.imagen` (y
  `sysdiagrams.definition`, cuya tabla tampoco viaja).
- **Tablas excluidas** (`skipped_tables`, con su conteo): `sysdiagrams` (motivo `tool`: diagramas de SQL Server
  Management Studio) y `personal_detalles` (motivo `personal_data`: domicilio, teléfonos y correo de los empleados; no se
  importan y, por minimización, no viajan).

## 6. Lectura y límites

`legacypkg.Open` lee en flujo (sin cargar el paquete en memoria): el encabezado, la frase de paso y el manifiesto al
abrir, y después cada tabla y sus filas en orden. La integridad completa se confirma al recorrerlo: la lectura termina
(`io.EOF`) solo si cuadran los bloques, el SHA-256 y el conteo de cada tabla, la huella y el final del paquete; el
importador no debe confirmar su transacción antes.

Errores tipificados (`legacypkg.ErrorKind`), con su mensaje en español: `not_package`, `unsupported_version`,
`wrong_passphrase`, `damaged`, `limit` y `unreadable` (un error al leer el archivo, por ejemplo una USB desconectada o
una subida interrumpida: no dice que el paquete esté dañado y puede bastar con volver a intentarlo).

Límites contra paquetes manipulados (`legacypkg.DefaultLimits`; un campo en cero de `OpenOptions.Limits` toma el valor
predeterminado), comprobados antes de gastar memoria o tiempo:

| Límite                                | Valor           |
| ------------------------------------- | --------------- |
| Memoria de Argon2id                   | 8 MiB a 256 MiB |
| Pasadas de Argon2id                   | 1 a 10          |
| Hilos de Argon2id                     | 1 a 16          |
| Tamaño de bloque                      | 4 KiB a 8 MiB   |
| Contenido descomprimido (todo el tar) | 16 GiB          |
| `manifest.json`                       | 16 MiB          |
| Cada `tables/<tabla>.json`            | 8 GiB           |
| Tablas y columnas por tabla           | 1000 y 1024     |
| Una fila                              | 4 MiB           |

Los nombres de las entradas del tar se validan (solo `manifest.json` y `tables/<nombre>.json`, regulares y en el orden
del manifiesto): la lectura nunca extrae archivos al disco.

## 7. Escritura

`legacypkg.Writer` escribe en flujo. Como el manifiesto va primero pero sus huellas solo se conocen al final, cada tabla
se prepara antes en un archivo temporal de una carpeta privada (0700; en Windows con ACL propia), **cifrado** con una
llave aleatoria que solo vive en memoria (los datos nunca quedan en claro en el disco), y se borra al terminar.
`legacypkg.CreateOutput` escribe el paquete en un temporal privado junto al destino (`.<nombre>.<aleatorio>.partial`,
0600, con `fileacl.CreatePrivate`; en Windows, en una USB con FAT32 o exFAT, que no guardan listas de control de acceso,
sin ACL propia: el paquete va cifrado) que solo recibe el nombre definitivo al terminar, **sin reemplazar nunca** un
archivo existente (`renamex_np`/`renameat2`/`MoveFileEx` sin reemplazo; en una USB sin esas operaciones, enlace duro o
comprobación previa). `legacypkg.RemoveStalePartial` borra el temporal de un paquete que quedó a medias, solo si su
nombre es exactamente el de uno de estos temporales.

## 8. Subcomando `legacy-prepare`

```text
ad-el-pos-server legacy-prepare --backup <respaldo.bak> --output <paquete.adelmig> [--watch-stdin]
ad-el-pos-server legacy-prepare --cleanup
```

- La frase de paso llega **solo por la entrada estándar** (la primera línea; en una terminal se pide dos veces sin
  mostrarla). Nunca por argumentos ni variables de entorno.
- `--watch-stdin`: cerrar la entrada estándar cancela la preparación (para la aplicación de escritorio, también en
  Windows). Ctrl-C y SIGTERM también cancelan.
- Sale con 0 si el paquete quedó listo y con 1 si no. Al terminar (bien, con error, con Ctrl-C o SIGTERM, al cerrarse
  la entrada con `--watch-stdin`, si se cierra su salida o con un fallo interno) elimina el contenedor, su volumen, la
  copia del respaldo, las tablas en preparación y el paquete a medias. **Si el proceso termina a la fuerza** (SIGKILL,
  el cierre forzado de Windows o un apagón) no puede limpiar: quedan el contenedor y su volumen, la carpeta de la
  preparación (tablas en preparación cifradas y, si hizo falta, la copia del respaldo **en claro**) y el paquete a
  medias. Los borra la siguiente preparación al empezar, o `legacy-prepare --cleanup` (la aplicación lo llama al abrir el
  asistente).
- **Una sola preparación a la vez** por cuenta de usuario: el bloqueo del sistema operativo (`flock` en macOS,
  `LockFileEx` en Windows) sobre `<carpeta temporal del usuario>/adel-legacy-prepare/lock`, que el sistema suelta aunque
  el proceso termine a la fuerza. Con otra en curso: `ALREADY_RUNNING`, sin tocar nada.
- **Carpeta de trabajo** `<carpeta temporal del usuario>/adel-legacy-prepare` (0700; en Windows con la ACL privada): el
  bloqueo y una carpeta `run-<aleatorio>` por preparación con las tablas en preparación, la copia del respaldo y el
  registro de la ruta del paquete a medias. Con el bloqueo tomado, todo lo demás que haya en ella es un resto.
- `--cleanup` borra esos restos (y, si Docker está en marcha, los contenedores y volúmenes del lector cuyo proceso ya no
  existe) y responde con un evento `cleanup`. No lee la entrada estándar. Con una preparación en curso:
  `ALREADY_RUNNING`.
- Antes de arrancar el contenedor se calcula el SHA-256 del respaldo (va al manifiesto) y al final se comprueba que el
  archivo no cambió (tamaño y fecha): si cambió, `BACKUP_CHANGED`. Antes de darle su nombre, el paquete escrito se abre
  con la misma frase de paso y se recorre completo (`legacypkg.Verify`): un paquete con nombre siempre se puede importar.

**SQL Server temporal** (§3.1): imagen oficial `mcr.microsoft.com/mssql/server:2022-latest` (`linux/amd64`; en Apple
Silicon con la emulación de Docker Desktop; se descarga la primera vez), edición Express, `--network none` (SQL Server
solo escucha en el loopback del contenedor; ningún puerto publicado), etiqueta `local.adel.pos.legacy-reader=1`,
contraseña de `sa` generada al momento y pasada solo por el entorno del proceso `docker` (`-e MSSQL_SA_PASSWORD` sin
valor), etiquetas con el PID del proceso que lo creó y su hora (`local.adel.pos.legacy-reader.pid` y
`local.adel.pos.legacy-reader.started`), respaldo montado de solo lectura (`--mount type=bind,…,readonly`; si Docker no puede montar su carpeta, una copia
privada temporal) y un volumen propio para `/var/opt/mssql`. Las consultas van por `docker exec -i … sqlcmd` con la
contraseña leída dentro del contenedor. Docker debe ser el de esta computadora: si `DOCKER_HOST` o el contexto actual apuntan a otra
(`docker context inspect` con un extremo que no es `unix://` ni `npipe://`), `DOCKER_REMOTE` sin contactarlo.
`RESTORE HEADERONLY` (elige el respaldo completo más reciente del archivo; con respaldos completos de varias bases,
`BACKUP_INVALID`),
`RESTORE FILELISTONLY` y `RESTORE DATABASE … WITH MOVE`, y la base queda de solo lectura. Al empezar se eliminan los
contenedores y volúmenes con esa etiqueta que haya dejado una ejecución interrumpida: solo los de un proceso que ya no
existe (o de hace más de 12 horas, por si el PID se reutilizó), nunca los de otra preparación en curso. Cada tabla tiene
un plazo de lectura muy generoso (2 horas) para que un SQL Server atascado no deje la preparación esperando.

**Avance**: una línea JSON por evento en la salida estándar.

| Evento     | Campos                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                       |
| ---------- | -------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `step`     | `step` (`image`, `start`, `restore`, `read`, `write`, `cleanup`), `status` (`started` o `finished`), `message` y, al terminar, `seconds`                                                                                                                                                                                                                                                                                                                                                                                                     |
| `progress` | `step`, `message`, `current` y `total` (capas de la imagen descargadas o porcentaje de la restauración)                                                                                                                                                                                                                                                                                                                                                                                                                                      |
| `table`    | `table`, `status` (`reading`, `done` o `skipped`), `done` y `total` (tablas), `rows` (filas leídas) y `table_rows` (filas de la tabla)                                                                                                                                                                                                                                                                                                                                                                                                       |
| `result`   | `summary`: `output`, `package_bytes`, `content_fingerprint`, `verified` (el paquete se comprobó antes de darle su nombre), `backup` (`file`, `bytes`, `database`, `started_at`, `finished_at`, `server_version`, `sets`), `sales` (`first`, `last`), `tables` (`name`, `rows`), `skipped_tables`, `omitted_columns` (`table`, `column`, `reason`, `rows_with_value`), `timings` (segundos por paso, `verify` y `total`) y `previous_leftovers` (lo que se borró de una preparación anterior: `folders`, `partials`, `containers`, `volumes`) |
| `cleanup`  | solo con `--cleanup`: `removed` (`folders`, `partials`, `containers`, `volumes`) y `docker` (`checked` o `unavailable`: sin Docker solo se borran las carpetas)                                                                                                                                                                                                                                                                                                                                                                              |
| `error`    | `code`, `message` (para el propietario, sin tecnicismos) y `detail` (técnico, sin datos del respaldo)                                                                                                                                                                                                                                                                                                                                                                                                                                        |

Códigos de error: `USAGE`, `ALREADY_RUNNING`, `PASSPHRASE_INVALID`, `BACKUP_UNREADABLE`, `BACKUP_INVALID`,
`BACKUP_CHANGED`, `OUTPUT_EXISTS`, `OUTPUT_INVALID`, `DOCKER_MISSING` (con las instrucciones para instalar Docker
Desktop), `DOCKER_NOT_RUNNING`, `DOCKER_UNSUPPORTED` (contenedores de Windows), `DOCKER_REMOTE`, `IMAGE_UNAVAILABLE`, `READER_START_FAILED`, `READER_MEMORY`,
`RESTORE_FAILED`, `READ_FAILED`, `WRITE_FAILED`, `CANCELLED` e `INTERNAL`.

**Solo pruebas**: `ADEL_TEST_MODE=1 ad-el-pos-server legacy-synthetic --output <paquete.adelmig>` (frase por la entrada
estándar) escribe el paquete sintético de `internal/legacysynth`, sin Docker; responde con una línea JSON (`result` o
`error`).
