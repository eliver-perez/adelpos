# Etapa L1 · Avance

Integración con el servidor de licencias. El servidor Go del restaurante:

- se activa con una licencia firmada por `licensing.sicrat.com` (entorno `test`);
- aplica los estados y los cupos de la licencia en el backend;
- y protege la hora del equipo.

Especificación: [docs/etapas/ETAPA_L1_LICENCIAS_v2.md](etapas/ETAPA_L1_LICENCIAS_v2.md). **Manda el contrato de
licencias V1.0** ([docs/licencias/CONTRATO_LICENCIAS_ADEL_POS.md](licencias/CONTRATO_LICENCIAS_ADEL_POS.md)), copiado
sin cambios de la etiqueta `v1.0.0` del servidor de licencias junto con su guía y `contracts/licencias/v1/`.

Rama `etapa-l1`, creada desde `main` (`1749b0b`):

- **Al empezar**: servidor 0.14.0 (esquema 27) y Android 0.3.0. El servicio de la Mac ya está en 0.14.0.
- **Al terminar**: servidor **0.15.0** y Android **0.4.0** (`versionCode` 4).

Cómo se trabaja:

- Coordinador y agentes, cada uno en su árbol de trabajo. **El propietario quitó el límite de agentes simultáneos en
  esta etapa** (23/11/2026); los agentes siguen sin lanzar otros.
- Se prueba en copias aisladas: el clúster temporal 55499 y un Go temporal en modo de pruebas.
- Android, en `Pixel_Tablet` y `ADEL_A1_Tablet_API26` durante las fases.
- **Ninguna prueba toca el servicio real de la Mac.**
- **Ninguna prueba automática llama a `licensing.sicrat.com`**; solo la Fase G, con las tres licencias de prueba.
- **Los códigos de esas licencias nunca se escriben** en el repositorio, el avance, los registros ni las capturas. Se
  nombran «la Perpetua», «la Mensual» y «la Prueba».
- Ningún commit lleva líneas de atribución.

---

## Fase 0 · Verificación

Agente único de la Fase 0, en `$SP/wt/l1-p0` (rama `etapa-l1-p0`, desde `etapa-l1` en `e2687fa`). Sin código de AD-EL
POS: solo las copias del contrato, su `ORIGEN.md` y las exclusiones del formato. Sin Go temporal, sin emuladores, sin
llamadas a `licensing.sicrat.com` y sin leer los códigos de licencia de prueba. La única red fue el clon de GitHub.

### 0.1 Clon, etiqueta y huella del contrato

- **Clon**: `https://github.com/eliver-perez/adelpos-license-server` en `~/src/adelpos-license-server` (se creó
  `~/src`), fuera de AD-EL POS. Queda en la etiqueta, sin cambios (`git status` limpio).
- **Etiqueta `v1.0.0`**: anotada (objeto `2a9275511196cd9dc6213da955c1459a13c72f51`), apunta al commit
  **`309dcff220430669c6167579d51145b4c8b8cbd5`** (17/11/2026, «Etapa 5, Fase 8: cierre de la etapa y versión 1.0.0»).
- **SHA-256 del contrato** `docs/CONTRATO_LICENCIAS_ADEL_POS.md` de la etiqueta:
  `e3435f16e27e11c85ac263ef2accfa72aad07cf30dbcdee0a0b8b22326314058`. **Coincide** con el de la especificación. Su
  encabezado dice «V1.0».
- **El archivo «V1.1»**: no está en la etiqueta. Está en `main` del servidor de licencias, en un commit posterior
  (`b70f0f3`, «Contrato de Licencias»), como `docs/CONTRATO_LICENCIAS_ADEL_POS_v2.md`. Cambia el encabezado, el párrafo
  de introducción y dos renglones del final (14 renglones). El mismo commit agrega `_v3.md`, idéntico a la V1.0. Se usa
  solo la V1.0 de la etiqueta.
- **Guía de integración** `docs/INTEGRACION_ADEL_POS.md`: SHA-256
  `15a19069e1c834ef735e9607158c740b7fe5f1287ee388f4250d8be5736672e8`. Cita el SHA-256 del contrato V1.0.

### 0.2 Copias sin cambios y exclusiones del formato

- `contracts/v1/` (33 archivos) se copió a **`contracts/licencias/v1/`**. El contrato y la guía se copiaron a
  **`docs/licencias/`** con el mismo nombre.
- **Comprobación**: SHA-256 de los 35 archivos de la copia contra el clon, todos idénticos. Además, los blobs de git del
  índice son los mismos que los de la etiqueta (`git ls-tree -r v1.0.0` contra `git ls-files -s`, 35 de 35).
- **`contracts/licencias/ORIGEN.md`**, el único archivo nuevo ahí, guarda:
  - el repositorio, la etiqueta (con su objeto), el commit y la fecha;
  - el SHA-256 y el tamaño de cada archivo copiado;
  - una orden `shasum -c` que los comprueba desde la tabla;
  - la orden del verificador de referencia;
  - la frase de que son copias de solo lectura que no se editan, no se formatean ni se regeneran.
- **Qué revisaría cada comprobación**:

  | Comprobación                                                                                                                                                                | ¿Llega a las copias?                                                                                                                                                    | Qué se hizo                                                                                                                                                 |
  | --------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | ----------------------------------------------------------------------------------------------------------------------------------------------------------------------- | ----------------------------------------------------------------------------------------------------------------------------------------------------------- |
  | Prettier (`format` y `format-check`, `scripts/dev.sh:41` y `:48`; también `scripts/windows/dev.mjs:659` y `:830`)                                                           | **Sí**: `prettier --check` marcaba 5 (`vectors.json`, `license.schema.json`, `README.md`, contrato y guía)                                                              | `.prettierignore`: `contracts/licencias/v1/` y los dos documentos copiados. `ORIGEN.md` y los documentos nuevos de `docs/licencias/` sí siguen con Prettier |
  | `gofmt -l backend` / `-w backend` (`scripts/dev.sh:42`, `:49`)                                                                                                              | No: solo `backend/`                                                                                                                                                     | Nada                                                                                                                                                        |
  | `go vet`, `go test`, `GOOS=windows` del arnés (`go-unit`)                                                                                                                   | No: corren en el módulo `backend/`; `verify/` es otro módulo (`go.mod` propio) y no hay `go.work` (`go list ./...` en `backend` no tiene ningún paquete de `contracts`) | Nada                                                                                                                                                        |
  | `cargo fmt`                                                                                                                                                                 | No: solo `desktop/src-tauri`                                                                                                                                            | Nada                                                                                                                                                        |
  | `check-legacy-data` (`scripts/check-legacy-data.sh`, dentro de `format-check`)                                                                                              | Sí, recorre el índice, pero ningún nombre ni firma coincide (`.lic` empieza con `eyJ`)                                                                                  | Nada; aprobado con las copias en el índice                                                                                                                  |
  | `sync-check` (`scripts/sync-metadata.mjs`)                                                                                                                                  | No: solo sus archivos fijos                                                                                                                                             | Nada                                                                                                                                                        |
  | Pruebas que recorren carpetas (`apicontract/plain_language_test.go:76`, `legacyread/imports_test.go:22`, `styles.test.ts`, `unknownValues.test.ts`, `auditActions.test.ts`) | No: `backend/internal`, `backend/` o `desktop/src`                                                                                                                      | Nada                                                                                                                                                        |
  | Editor (`.editorconfig`: `insert_final_newline`, `trim_trailing_whitespace`)                                                                                                | Sí, si alguien abre y guarda un `.lic` o `.licreq` (no tienen salto de línea final)                                                                                     | Sección de `.editorconfig` que desactiva esas reglas en las rutas copiadas                                                                                  |
  | Fin de línea (`.gitattributes`, `* text=auto eol=lf`)                                                                                                                       | Sí, pero sin efecto: todo es LF o no tiene fin de línea (`git ls-files --eol`: `i/lf` o `i/none`)                                                                       | Nada                                                                                                                                                        |

- **Después**: `format` solo cambió `ORIGEN.md`. Los SHA-256 de las copias y de `docs/etapas/` son iguales antes y
  después. `fmt-check` y `sync-check` aprobados.
- **Nota**: `contracts/licencias/v1/README.md` enlaza al contrato con `../../docs/…`, la ruta del repositorio de origen.
  En AD-EL POS ese enlace no lleva a nada. No se edita; `ORIGEN.md` dice dónde está el contrato.

### 0.3 Verificador de referencia

Orden de la §2 de la guía, en el clon:
`env GOTOOLCHAIN=local GOPROXY=off GOSUMDB=off go run -C contracts/v1/verify . ../fixtures/vectors.json`, con Go
1.27.3 de `.tools` y `GOCACHE` en el scratchpad.

- **Resultado**: `OK: 20 licencias, 17 casos negativos, 10 casos de revisión, 13 estados temporales, 4 pruebas de
posesión y 3 solicitudes .licreq.` Son **67 casos**, todos aprobados (código de salida 0). Desde la copia
  (`contracts/licencias/v1/verify`) da lo mismo.
- **Lo que no ejecuta el verificador**: las secciones `dates` (40 casos) y `credits` (11) de `vectors.json`. Las calcula
  solo el servidor de licencias (§5.1 a §5.5 y §11) y AD-EL POS no las recalcula.
- **Defectos del servidor de licencias**: ninguno.

### 0.4 Lo que el contrato pide al servidor Go (lectura de §1 a §10, §12, §13 y la guía)

- **Identidad (§2)**: `server_id` (UUID v4 en minúsculas, el de `identity/`) y una llave **Ed25519 exclusiva para
  licencias**, aparte del ECDSA. Ninguno se regenera solo ni viaja en los respaldos. La identidad de la instalación es
  el **par** `server_id` + llave pública.
- **Formatos (§3)**:
  - base64url sin relleno y canónico;
  - instantes UTC con `Z` y de 0 a 9 decimales;
  - JWS compacto con el encabezado exacto `{"alg":"EdDSA","kid":"<kid>","typ":"adelpos-lic+jws"}`;
  - entornos `test` y `production` separados.
- **Contenido (§4)**:
  - todos los campos son obligatorios, con sus tipos;
  - los campos desconocidos se ignoran, y las claves desconocidas de `features` también;
  - una `schema_version` que no empieza con `1.` es incompatible;
  - `limits` son los derechos reales, de 0 a 999.
- **Estado temporal (§5.6)**:
  - `ahora_efectivo = máximo(reloj_local, última_hora_confiable)`, calculado solo con `expires_at` y `grace_days`;
  - la última hora confiable sale del `server_time`, del `issued_at` y de la última hora local registrada.
- **Estados y tabla de operaciones (§6)**: seis estados, ocho filas y las reglas firmes. Ver 0.5.7.
- **Cupos (§7)**: cajas, comanderos y KDS, con asignación atómica. Una licencia con menos cupos no interrumpe nada.
- **API (§8)**:
  - el desafío dura 5 min y es de un solo uso;
  - prueba de posesión `ADELPOS-LIC-V1\n…`;
  - activar, transferir (con `ACTIVATION_LIMIT` y `transfer_available`), consultar y liberar;
  - reintentos con el mismo cuerpo y el mismo `request_id`;
  - se decide por `error.code`;
  - `TEMPORARY_UNAVAILABLE` nunca afecta la licencia instalada.
- **Sin internet (§9)**:
  - `.licreq` firmado `ADELPOS-LICREQ-V1\n` + `payload_b64u`, para `activate` (código opcional), `renew` y
    `deactivate`;
  - `.lic` = el mismo JWS.
- **Verificación (§10)**: encabezado, `kid` del entorno, firma, contenido, entorno, destinatario y revisión mayor que
  la más alta aceptada (igual solo si es idéntica byte por byte). Después se guarda de forma persistente.
- **Privacidad (§12)**: el código comercial y las llaves privadas nunca se registran. Las protecciones contra copias y
  cambios de reloj las pone AD-EL POS.
- **Archivos de prueba (§13)**: los casos de `vectors.json`, con llaves de semillas públicas que solo usan las pruebas.

### 0.5 Código actual de AD-EL POS

#### 0.5.1 Identidad

- **Archivos**: `backend/internal/localsetup/identity.go:41-47` crea, en la carpeta `identity/` junto a `server.json`:
  - `identity.json`: `server_id` UUID v4 en minúsculas y `created_at`;
  - `server-cert.pem`: certificado autofirmado ECDSA P-256 de 30 años;
  - `server-key.pem`: su llave privada, 0600, en una carpeta 0700 con ACL privada en Windows (`fileacl.Protect`,
    `:188`).
- **`EnsureIdentity`** (`:78-98`):
  - si están los tres archivos, los carga;
  - si no está ninguno, la crea;
  - si está incompleta, falla y **nunca la reemplaza** (`:91-93`), porque las terminales emparejadas dejarían de
    conectarse.
  - La creación es atómica: carpeta temporal renombrada (`:158-247`).
  - `identity.json` se lee con `DisallowUnknownFields` (`:132`). Un campo nuevo ahí rompería la lectura de un
    servidor 0.14.0, así que la llave Ed25519 va en un **archivo aparte**.
- **Cuándo se crea**: en cada arranque de `serve` y del `host`, por `serveHTTP` → `loadIdentity`
  (`backend/cmd/ad-el-pos-server/main.go:181-189`, `:244-249`):
  - en `serve` de desarrollo, la carpeta está junto a la configuración (`.local/identity`, `main.go:163-168`);
  - en el servicio de macOS, en `layout.DataDir/identity` (`host_unix.go:174-176`);
  - en Windows, en `host_windows.go:113`.
  - Sin identidad, el servidor sigue atendiendo la escucha local, sin red ni emparejamiento (`main.go:241-249`).
  - Los respaldos programados llevan el `server_id` en su nombre (`main.go:252-254`).
- **En la base no está el `server_id`**. Solo está la huella SPKI que fijó cada terminal (`devices.server_fingerprint`,
  `migrations/0023…sql:218-228`).
- **Las terminales se atan a `server_id` + huella**:
  - Android busca el servidor por los dos (`android/core/data/…/access/PairedConnection.kt:24`, `:91`);
  - el escritorio guarda su credencial por `server_id` (`desktop/src-tauri/src/connection.rs:43-44`).
- **Dónde quedaría la llave Ed25519**:
  - en `identity/license-key.pem` (PKCS#8, 0600), creada por una función nueva de `localsetup`
    (`EnsureLicenseKey`) que reutiliza `writeExclusive`, `fileacl` y el renombrado atómico;
  - y una subcarpeta `identity/licencia/` (0700) con el estado de la licencia. Ver la Decisión propuesta 1.

#### 0.5.2 Respaldos

- **Todos los respaldos son solo la base**: `pg_dump --format=custom --no-owner --no-acl`. Esto vale para:
  - el manual;
  - «Respaldar ahora» y los programados (`backupschedule`);
  - el de antes de migrar (`host.go:139-162`);
  - el de antes de «Quitar datos de prueba» (`host.go:100-106`).
- **Dónde está el código**: `backend/internal/standalone/backup.go:39-99` (pg_dump en `:81`) en el servicio y
  `backend/internal/maintenance/backup.go:201` en `serve`.
- **Qué no incluyen**: ni `server.json`, ni `identity/`, ni `logs/`. La carpeta elegida para los respaldos no puede
  estar dentro de la carpeta de datos (`host.go:127-131`, `ProtectedDirectories`).
- **Restaurar** (`standalone.Restore`, `backup.go:219`) reemplaza solo la base. `identity/` no cambia.
- **`adopt-legacy`** (`standalone/adopt.go:20-34`, `:61-147`): respalda la base anterior con pg_dump y la restaura en
  el clúster nuevo. **No copia `identity/`**: el servicio crea su propia identidad en el primer `host`.
- **«Quitar datos de prueba»** (`maintenance/plan.go`) borra o desactiva catálogo y configuración de prueba, con un
  respaldo previo. No toca operaciones, y en la propuesta tampoco nada de licencias.
- **Conclusión**: `identity/` **nunca viaja** en un respaldo. Por eso la propuesta guarda ahí la llave y el estado de
  la licencia, y no en la base. Restaurar un respaldo viejo en la misma computadora no puede:
  - devolver una revisión anterior, como la de antes de una revocación;
  - ni bajar la última hora confiable.

#### 0.5.3 Copia a otro equipo y huella

- **Respaldo restaurado en otra computadora**: esa computadora tiene su propia identidad. Las terminales de la base
  copiada tienen fijada la huella de la original y no se conectan.
- **Carpeta de datos completa copiada** (con `identity/`): la copia es **indistinguible** de la original, con el mismo
  `server_id` y el mismo certificado. Las terminales emparejadas se conectarían a cualquiera de las dos.
- **Huella del equipo hoy**: el código no la lee (no hay `ioreg`, `IOPlatformUUID` ni `MachineGuid`).
- **Cómo se leería**:
  - **macOS**: `/usr/sbin/ioreg -rd1 -c IOPlatformExpertDevice`, renglón `"IOPlatformUUID" = "…"`, con ruta absoluta
    y sin cgo. Comprobado en esta Mac sin mostrar el valor: un renglón con el formato esperado.
  - **Windows**: `HKLM\SOFTWARE\Microsoft\Cryptography\MachineGuid` con `golang.org/x/sys/windows/registry` y
    `WOW64_64KEY`. El paquete ya está en `x/sys v0.49.0` del `go.mod` y en la caché de módulos: **sin dependencias
    nuevas**. Se prueba con una lectura simulada; la real, en la Fase W.
  - **Lo que se guarda**: solo `SHA-256("ADELPOS-EQUIPO-V1\n" + valor)`, nunca el valor.
- **Comportamiento exacto propuesto para la §1.1** (Decisión propuesta 4):
  1. **Detección**. Al aceptar una licencia se guarda la huella con ella. Al arrancar se compara con la actual. Si no
     coincide:
     - la licencia guardada **se ignora**, sin borrarla;
     - el estado es «Sin activar con datos», o «Sin activar» si la base no tiene operaciones, con
       `copied: true`;
     - el mensaje es «Esta instalación se copió de otra computadora. Actívala o transfiere la licencia.».
     - Si la huella no se puede leer tras tres intentos, se trata como distinta (falla cerrada), con otro mensaje: «No
       se pudo comprobar esta computadora. Reinicia el servidor; si sigue igual, comunícate con soporte.».
  2. **No se activa con la identidad copiada**. El servidor de licencias la vería como la misma instalación y
     entregaría su revisión vigente: las dos computadoras quedarían activas. Por eso «Activar en esta computadora»
     primero pide confirmar **«Crear una identidad nueva para esta computadora»**. La confirmación solo se acepta:
     - en la computadora principal (`request.Local`);
     - con «Administrar la licencia»;
     - con un diálogo que explica las consecuencias.
  3. **Qué se regenera, al confirmar**: los **tres** (`server_id` nuevo, ECDSA nuevo y Ed25519 nueva), con la misma
     creación atómica de `createIdentity`. Por qué:
     - la §2 dice que una copia «tiene otra identidad» (`server_id` + llave);
     - las terminales se reconocen por `server_id` + huella SPKI, así que un `server_id` nuevo ya obliga a emparejarlas
       otra vez;
     - conservar el certificado dejaría dos computadoras con la misma identidad de red, y las terminales de la original
       se conectarían a la copia.
  4. **La identidad anterior** se mueve a `identity-reemplazada-AAAAMMDD-HHMMSS/` (0700), junto a `identity/`, para
     soporte. Nunca se vuelve a usar con el servidor de licencias.
  5. **Terminales emparejadas**: se revocan las emparejadas por red (`devices.server_fingerprint IS NOT NULL`), con sus
     sesiones y credenciales de envío.
     - Motivo: `identity_regenerated`.
     - Quedan en el historial y en Terminales como revocadas, y hay que emparejarlas otra vez. Escritorio y Android ya
       tienen su aviso de identidad cambiada.
     - La terminal de la propia computadora (inscrita por la escucha local, sin huella) sigue igual.
  6. **No se toca**:
     - ventas, cuentas, turnos, inventario, catálogo, usuarios, roles, configuración, historial ni respaldos;
     - el archivo de la licencia ignorada, que se conserva para soporte y nunca vuelve a aceptarse (es de otra
       instalación).
  7. **Después**:
     - Auditoría `license.identity_regenerated` con los `server_id` y las huellas SPKI, anteriores y nuevos, y el
       número de terminales revocadas.
     - La escucha de red se reinicia con el certificado nuevo; si no se puede en caliente, se pide «Reiniciar».
     - Sigue la activación. Con `ACTIVATION_LIMIT` y `transfer_available: true` se ofrece «Transferir a esta
       computadora».
  8. **La computadora original** no cambia. Sigue hasta su siguiente consulta, donde recibe su cierre
     `revoked`/`transferred` y confirma la transferencia.
  9. **Caso legítimo**: cambiar la tarjeta madre o reinstalar Windows cambia la huella. Es el mismo camino, pero la
     computadora «anterior» ya no existe: la transferencia queda pendiente y el propietario la libera desde el panel.
     Va en `docs/licencias/INTEGRACION.md`.

#### 0.5.4 Sesiones y `sessions-summary`

- **`GET devices/sessions-summary`** (`administration/routes.go:188-190` → `devices.go:289-322`):
  - cuenta las sesiones vigentes (no revocadas ni vencidas) de terminales no revocadas;
  - las agrupa por `devices.kind`: `desktop`, `android` o `kitchen` (`pairing.go:92`, `0023…sql:218-228`);
  - responde `limited: false` y pide `devices.read`.
  - El escritorio la muestra en «Sesiones activas por tipo de terminal» (`desktop/src/pages/DevicesPage.tsx:399-407`).
- **`app.sessions`** (`0002…sql:85-97`):
  - una vigente por usuario y terminal desde la 0024 (`sessions_one_active_user_device_idx`, `0024…sql:16-17`);
  - `opened_with_pin` (`0025…sql:104-105`);
  - vencen a las 12 h (`auth.go:139`);
  - `last_seen_at` se actualiza en cada petición (`service.go:486`).
- **Android nunca llama a `auth/logout`**. Sus sesiones con PIN duran hasta vencer, reemplazarse o revocarse. Volver
  a «Selecciona tu usuario» pasa solo en la aplicación: la sesión sigue vigente.

#### 0.5.5 Turnos, modo Cocina, entrada por tipo de terminal y actividad de captura

- **Turnos de caja**: son por **caja física** (`app.registers`), no por terminal.
  - Hay un turno activo (`open` o `closing`) por caja (`cash_shifts_one_active_idx`, `0008…sql:72`).
  - La terminal usa la caja de `devices.register_id` (`payments/shifts.go:103-115`). Ese campo no es único: **dos
    terminales pueden compartir una caja y su turno**.
  - Android no tiene caja física.
- **Abrir turno**: `POST payments/shifts` (`openShift`, `shifts.go:267-316`), bajo 7820148305 exclusivo
  (`payments/handler.go:98`). El turno guarda `opened_device_id`.
- **Cerrar turno**: por el identificador del turno, `payments/shifts/{id}/closing` (`cash_closing.go:73`, `:242` y
  `:379`). Se hace desde cualquier terminal con el permiso, también desde otra computadora.
- **Modo Cocina**: **no es un estado de Go**.
  - Es Cocina › Preparación (`GET kitchen/board`, `POST kitchen/states`; `api.rs:74`, `:79`) en «modo cocina»
    (`CLAUDE.md`, etapa 9F, D7).
  - Una terminal con pantalla inicial `kitchen` abre ahí (`0020…sql:7-9`; `docs/TERMINAL_DE_COCINA.md` §4).
  - Cualquier usuario con permisos de cocina puede abrir Preparación en cualquier terminal de escritorio.
  - `devices.kind = 'kitchen'` está reservado para la futura aplicación KDS.
- **Cómo abre sesión cada tipo de terminal**:

  | Terminal                    | Entrada                                                                                                                                                    | Sesión                                                                                                                                                                                                                                         |
  | --------------------------- | ---------------------------------------------------------------------------------------------------------------------------------------------------------- | ---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
  | Escritorio (caja o captura) | `auth/login`, con usuario y contraseña (`auth.go:14`)                                                                                                      | Una por usuario y terminal (`issueSession`, `auth.go:110-140`)                                                                                                                                                                                 |
  | Terminal de cocina          | La misma de escritorio                                                                                                                                     | La misma; abre en Preparación por su pantalla inicial                                                                                                                                                                                          |
  | `android`                   | «Selecciona tu usuario» (`GET auth/pin-users`, `pin_login.go:71`) y PIN (`POST auth/pin-login`, `pin_login.go:117-198`), con la credencial del dispositivo | Sesión con permisos de mesero (`opened_with_pin`) más la credencial de envío, que solo sirve para `restaurant/offline-orders` (`pin_login.go:31`, `:328`). **Varios meseros en la misma tableta son varias sesiones con el mismo `device_id`** |

- **Actividad de captura para los 15 minutos** (Decisión propuesta 13):
  - **Ocupan el cupo y renuevan la actividad**:
    - `POST restaurant/accounts` (abrir una cuenta, con o sin productos, también para llevar);
    - `PUT restaurant/accounts/{id}/draft`;
    - `POST …/confirm` y `POST …/edit`;
    - `POST restaurant/offline-orders`.
  - **Solo renuevan la actividad** si la terminal ya tiene el cupo: `POST …/draft-preview` y `POST …/edit-preview`
    (entrar a una mesa para capturar).
  - **No cuentan**: las lecturas, los cobros, las cancelaciones, las precuentas, reorganizar cuentas, cocina ni
    administración.

#### 0.5.6 `internal/licensing` hoy

- **`backend/internal/licensing/licensing.go:1-11`**: `CheckStartup(environment, mode)` solo acepta
  `development`/`development`. La prueba `licensing_test.go:5-16` comprueba que nada más pasa.
  - Lo llaman `main.go:125` (todas las órdenes de `serve` salvo `version`, `stop` y `check-config`) y
    `host_unix.go:159`, igual en el `host` de Windows.
- **Configuración** (`config/config.go:33`, `:39`): `environment` (`development`|`production`) y `license_mode`
  (`development`|`commercial`), validados en `:121-126`.
  - El servicio escribe `development`/`development` (`standalone/bootstrap.go:44-64`).
- **Cómo encaja**: el paquete pasa a ser el núcleo (verificador, estado, almacén, entorno compilado, huella, guarda de
  la hora y desbloqueo), y `CheckStartup` se conserva con otra comprobación:
  - una compilación de desarrollo exige `environment: development`;
  - una de producción exige `production`/`commercial`.
  - `license_mode` se conserva con su significado: lo escriben los `server.json` existentes de macOS y Windows.

#### 0.5.7 Rutas protegidas: la tabla de la §6

**Filas de la tabla** (V = Vigente, T = Tolerancia, X = Vencida o Revocada, D = Sin activar con datos, S = Sin
activar):

| Fila                                                | V            | T            | X                                       | D   | S   |
| --------------------------------------------------- | ------------ | ------------ | --------------------------------------- | --- | --- |
| F1 Crear pedidos y agregar productos                | Sí, con cupo | Sí, con cupo | No                                      | No  | No  |
| F2 Cobrar, cancelar y descuentos sobre lo capturado | Sí           | Sí           | Sí                                      | No  | —   |
| F3 Abrir turno                                      | Sí, con cupo | Sí, con cupo | Solo con cuentas pendientes, y con cupo | No  | No  |
| F4 Cerrar turno y corte                             | Sí           | Sí           | Sí                                      | No  | —   |
| F5 KDS: lo ya enviado                               | Sí           | Sí           | Sí                                      | No  | —   |
| F6 Reportes, historial y respaldos                  | Sí           | Sí           | Sí                                      | Sí  | —   |
| F7 Administración                                   | Sí           | Sí           | Sí                                      | No  | No  |
| F8 Activar, transferir, renovar, importar           | Sí           | Sí           | Sí                                      | Sí  | Sí  |

**Clasificación propuesta por familia** («Fecha»: se bloquea con «Hora del equipo incorrecta»):

| Familia                                                                                    | Rutas                                                                                                                                                                                    | Fila                              | Fecha             | Cupo                                                    |
| ------------------------------------------------------------------------------------------ | ---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | --------------------------------- | ----------------- | ------------------------------------------------------- |
| `restaurant/*` lecturas                                                                    | `GET` de overview, tables, areas, accounts, settings, staff, cancel-reasons, history; `restaurant/state`                                                                                 | F6                                | No                | —                                                       |
| `restaurant/*` captura                                                                     | `POST restaurant/accounts`, `PUT accounts/{id}/draft`, `POST accounts/{id}/confirm` y `edit`                                                                                             | F1                                | Sí                | comandero, salvo turno activo en la caja de la terminal |
| `restaurant/offline-orders`                                                                | `POST` (credencial de envío)                                                                                                                                                             | F1                                | Sí                | comandero                                               |
| `restaurant/*` previas                                                                     | `POST accounts/{id}/draft-preview` y `edit-preview`                                                                                                                                      | F6 (solo lectura)                 | No                | renueva la actividad si ya lo tiene                     |
| `restaurant/*` sobre lo capturado                                                          | `accounts/{id}/cancel`, `cancel-account`, `discard`, `release`, `customer`, `PATCH accounts/{id}`, `split`, `transfer`, `merge`, `move`, `assign`                                        | F2 (Duda 7)                       | Sí                | —                                                       |
| `restaurant/*` administración                                                              | mutaciones de areas, tables, `tables/{id}/service`, layout, settings, cancel-reasons                                                                                                     | F7                                | Sí                | —                                                       |
| `payments/*` lecturas y previas                                                            | `GET`, `preview`, `cash/count-preview`, `previewDiscount`, `previewRefund`                                                                                                               | F6                                | No                | —                                                       |
| `payments/*` cobro                                                                         | `accounts/{id}/payments`, descuentos, cargos, devoluciones, correcciones, cancelación de ajustes, precuenta, ticket (imprimir, reimprimir, reintentar, rehacer), «Entregado» para llevar | F2                                | Sí                | —                                                       |
| `payments/shifts`                                                                          | `POST` (abrir)                                                                                                                                                                           | F3                                | Sí                | caja                                                    |
| `payments/shifts/{id}/*`                                                                   | closing, closing/cancel, conteo, ingresos, retiros, gastos, correcciones, cortes parciales, tipo de cambio; `cash/open-drawer`                                                           | F4                                | Sí                | —                                                       |
| `payments/*` configuración                                                                 | methods, settings, usd, tax, discount-reasons, cash/denominations, expense-categories, limit, drawers y su prueba                                                                        | F7                                | Sí                | —                                                       |
| `payments/cash-*` documentos                                                               | imprimir y reimprimir reportes y documentos de caja                                                                                                                                      | F6                                | No                | —                                                       |
| `kitchen/*` tablero                                                                        | `GET kitchen/board`, `POST kitchen/states`, `POST kitchen/tickets/{id}/ack`                                                                                                              | F5                                | Sí (el `GET`, no) | cocina                                                  |
| `kitchen/*` modo Cocina (nueva)                                                            | `POST kitchen/display-mode` (`{active}`)                                                                                                                                                 | F5                                | No                | cocina (ocupa o libera)                                 |
| `kitchen/*` cola y planificador                                                            | releases (release, schedule), changes/retry, jobs retry/resolve, reprint, test-print, print-jobs/rerender                                                                                | F5 (Duda 8)                       | Sí                | —                                                       |
| `kitchen/*` configuración                                                                  | stations, printers (y width-test), routes, settings, timing, access                                                                                                                      | F7                                | Sí                | —                                                       |
| `kitchen/*` lecturas                                                                       | los demás `GET`                                                                                                                                                                          | F6                                | No                | —                                                       |
| `inventory/*`                                                                              | `GET`                                                                                                                                                                                    | F6                                | No                | —                                                       |
| `inventory/*` mutaciones                                                                   | movimientos, conteos, recepciones, producción, transferencias, devoluciones, ajustes, existencias iniciales, compras y configuración                                                     | F7 (Duda 6)                       | Sí                | —                                                       |
| `catalog/*`                                                                                | `GET` → F6; mutaciones, también «Agotado»                                                                                                                                                | F7                                | Sí                | —                                                       |
| `reports/*`                                                                                | todas, también las exportaciones                                                                                                                                                         | F6                                | No                | —                                                       |
| `maintenance/backups*`                                                                     | estado, «Respaldar ahora» y ajustes                                                                                                                                                      | F6                                | No                | —                                                       |
| `maintenance/service*`                                                                     | estado y «Reiniciar»                                                                                                                                                                     | siempre                           | No                | —                                                       |
| `maintenance/legacy*` y `maintenance/test-data*`                                           | migración del sistema anterior y «Quitar datos de prueba»                                                                                                                                | F7                                | Sí                | —                                                       |
| `devices/*`                                                                                | `GET` y `sessions-summary` → F6; `PATCH`, revoke, invitations y `pairing-requests`                                                                                                       | F7                                | Sí                | —                                                       |
| `devices/{id}/seat/release` (nueva)                                                        | —                                                                                                                                                                                        | siempre                           | No                | libera                                                  |
| `devices/enroll`                                                                           | solo local                                                                                                                                                                               | arranque                          | No                | —                                                       |
| `pairing/*` (públicas)                                                                     | —                                                                                                                                                                                        | F7 («activar antes de emparejar») | Sí                | —                                                       |
| `auth/*`                                                                                   | login, pin-users, pin-login, logout, me, password, pin                                                                                                                                   | siempre                           | No                | —                                                       |
| `users/*`, `roles/*`, `permissions/*`, `settings/*`, `registers/*`, `documents/*`, `audit` | `GET` → F6; mutaciones                                                                                                                                                                   | F7                                | Sí                | —                                                       |
| `sessions/{id}/revoke`                                                                     | —                                                                                                                                                                                        | siempre                           | No                | —                                                       |
| `preferences`, `terminal/preferences`                                                      | —                                                                                                                                                                                        | siempre                           | No                | —                                                       |
| `setup/status`, `setup`                                                                    | solo sin establecimiento                                                                                                                                                                 | arranque (Duda 10)                | No                | —                                                       |
| `events` (SSE), `events/snapshot`, `ready-alerts` y su canal                               | —                                                                                                                                                                                        | siempre (lectura)                 | No                | —                                                       |
| `print-agent/*`                                                                            | entrega de trabajos ya registrados                                                                                                                                                       | siempre                           | No                | —                                                       |
| `license/*` (nuevas)                                                                       | —                                                                                                                                                                                        | F8                                | No                | —                                                       |
| `health`, `version`, `openapi.json`                                                        | no pasan por la administración (`httpapi/router.go`)                                                                                                                                     | —                                 | No                | —                                                       |

- **Dónde se comprueba**: un punto único en **`administration.Service.dispatch`** (`service.go:297-387`). Todas las
  peticiones pasan por ahí (`Execute`, `service.go:216-238`), y así ninguna se escapa:
  - las públicas (`setup`, `devices/enroll`, `pairing/*`);
  - las de sesión;
  - las de la credencial de envío;
  - `print-agent/*` y `ready-alerts`.
- **Cómo se comprueba**: la clase se calcula una vez por `(método, ruta)` con una tabla explícita en
  `internal/licensing/operations.go`.
  - **Estado y hora**: antes de las ramas públicas, y después de autenticar en las demás.
  - **Cupo**: después de autenticar, porque necesita el dispositivo, y en la misma transacción. Toma 7820148305
    exclusivo y después 7820148311. El manejador vuelve a tomar 7820148305 (es reentrante en la transacción). Si la
    operación falla, el cupo no se ocupa.
  - **Rutas desconocidas**: una ruta de escritura que no está en la tabla toma la clase más restrictiva (F7 con
    fecha).
  - **Prueba de completitud**: toda operación del puente (`desktop/src-tauri/src/api.rs:56`, `route()`, unas 360), de
    `backend/internal/apicontract/openapi.json` y de las rutas locales tiene su clase. Hay precedente:
    `reports/export_test.go:742` ya lee `export.rs`.
- **Operaciones que registran algo con fecha** (§6.2 de la especificación):
  - **Se bloquean**: todo lo marcado «Sí» en la columna «Fecha» (capturar, cobrar, cancelar, turnos, inventario,
    cocina y administración).
  - **Siguen disponibles**: lecturas y previas, reportes y exportaciones, respaldos, la licencia, `auth/*`,
    preferencias, «Reiniciar», el SSE y el agente de impresión. Ver la Decisión propuesta 7.

#### 0.5.8 Hora

- **«La hora de la última operación registrada»**:
  - La base registra con `now()` de PostgreSQL, el mismo reloj de la computadora.
  - La fuente más barata y completa es `max(app.audit_events.occurred_at)`. Casi toda operación se audita, y tiene el
    índice descendente `audit_occurred_idx` (`0002…sql:114-128`).
  - Además, cada operación aceptada por la puerta deja la hora local en la memoria del gestor.
- **Dónde se guarda la última hora confiable**: en `identity/licencia/estado.json`.
  - Se actualiza en memoria con cada `server_time`, cada `issued_at`, cada operación aceptada y el reloj cada minuto.
  - Se escribe a disco cada minuto y al apagar. Si se corta la luz, se pierde como mucho un minuto, dentro del margen.
  - La de `audit_events` **solo la inicia** cuando el archivo no la tiene (primer arranque tras la actualización o
    identidad nueva). Si se leyera siempre, la corrección de la §6.3 no podría bajarla (Duda 16).
- **Reloj inyectable**: no hay uno general. Hay campos `Now func() time.Time` en algunos componentes:
  - `backupschedule.Runner` (`runner.go:46`);
  - `inventory.Worker` (`worker.go:49`);
  - `legacyimport.Manager` (`manager.go:58`);
  - y otros sin exportar en `maintenance/backup.go:99` y `lanlistener.go:110`.
  - La autenticación usa `time.Now()` (`service.go:468`) y el SQL usa `now()`.
- **Cómo se inyectaría el reloj simulado**:
  - **Pruebas de Go**: `licensing.Manager.Now` y el reloj del gestor de cupos, con la convención de siempre (`nil` usa
    `time.Now`). El gestor de cupos escribe y compara la actividad con ese reloj, como parámetro y nunca con `now()`.
    Así los 15 minutos se prueban sin esperar.
  - **Procesos de prueba** (Playwright y Android, con el Go temporal): un desfase leído de un archivo
    (`ADEL_TEST_CLOCK_FILE`), **solo con `test_mode: true`**. No hay ninguna ruta de prueba en la API.
- **Margen**: 10 minutos.

#### 0.5.9 Compilación por entorno (§1.2)

- **Go**: el entorno se fija con una etiqueta de compilación.
  - `internal/licensing/trust_development.go` (`//go:build !adelpos_production`) embebe `llaves-test.json` y la
    pública de soporte de desarrollo. Declara `Environment = "test"` y la dirección por omisión
    `https://licensing.sicrat.com`.
  - `trust_production.go` (`//go:build adelpos_production`) embebe `llaves-production.json`, que no existe. Por eso
    **`go build -tags adelpos_production` no compila** (`pattern llaves-production.json: no matching files found`).
    Una prueba lo comprueba con `go build` y espera el fallo.
  - Las llaves no se pueden embeber desde fuera del módulo (`backend/`). Se guardan en `config/licencias/` y
    `scripts/sync-metadata.mjs` las copia a `backend/internal/licensing/`, como `product.json` → `metadata.json`
    (`sync-check` lo vigila).
- **`.pkg` y Windows**: `scripts/build-macos-pkg.sh:66` y `build-windows.ps1:84` compilan sin etiquetas, es decir, en
  desarrollo (confía en `test`). La compilación de producción, en `PASO_A_PRODUCCION.md`, agregará la etiqueta y las
  llaves.
- **Escritorio**: no se compila por entorno. `GET license/status` responde `environment: "test"` y solo entonces
  Configuración › Licencia muestra «Servidor de licencias de pruebas».

#### 0.5.10 Desbloqueo de soporte (§6.4)

- **Dónde vive**: un paquete de Go no puede importar `backend/internal` desde fuera de `backend/`.
  - El comando va en **`backend/cmd/soporte-desbloqueo`**. Comparte el formato y la verificación con
    `internal/licensing/unlock.go`, y lo prueba `go-unit`.
  - **`tools/soporte-desbloqueo/`** tiene la envoltura (`soporte-desbloqueo`, bash: `go run -C backend
./cmd/soporte-desbloqueo "$@"`) y su README.
  - Los instaladores no la incluyen: solo compilan `./cmd/ad-el-pos-server`.
- **Llaves**:
  - la privada de desarrollo, en `~/.adel-pos-dev/soporte/soporte-development.key` (PKCS#8, 0600, carpeta 0700), la
    genera `soporte-desbloqueo generar-llave`;
  - la pública, en `config/licencias/soporte-development.pub.json` (`kid`, `public_key`), sincronizada y embebida;
  - la de producción falta, así que tampoco compila con la etiqueta de producción.
- **Código de solicitud**: 24 caracteres en base32 de Crockford, en 6 grupos de 4 (`XXXX-XXXX-XXXX-XXXX-XXXX-XXXX`).
  - Alfabeto `0-9` y `A-Z`, sin I, L, O ni U. Al leerlo, acepta minúsculas, O = 0 e I/L = 1.
  - Contenido, 120 bits:

    | Bits | Campo                                                                                             |
    | ---- | ------------------------------------------------------------------------------------------------- |
    | 4    | versión                                                                                           |
    | 40   | etiqueta de la instalación: los primeros 5 bytes de `SHA-256("ADELPOS-SOPORTE-V1\n" + server_id)` |
    | 40   | número aleatorio de un solo uso                                                                   |
    | 24   | última hora confiable, en minutos desde 2026-01-01 UTC (unos 31 años)                             |
    | 12   | verificación: SHA-256 de lo anterior, para detectar un error al dictar                            |

  - La solicitud pendiente se guarda en `estado.json`. Solo hay una: una nueva reemplaza a la anterior.
- **Respuesta**: un JWS compacto con el encabezado exacto `{"alg":"EdDSA","kid":"<kid>","typ":"adelpos-unlock+jws"}`.
  - Contenido: `schema_version`, `product_id`, `environment`, `installation_tag`, `request_nonce`, `request_code`,
    `new_trusted_time` (por omisión, la hora del propietario) e `issued_at`.
  - Mide unos 470 caracteres: se pega por WhatsApp o se importa como archivo `.desbloqueo`.
- **Se acepta solo si**:
  - la firma es de la llave de soporte del entorno compilado;
  - la etiqueta es la propia;
  - el número es el de la solicitud pendiente y no se usó;
  - han pasado menos de 24 h desde la solicitud, medidas con el reloj local respecto al de la solicitud (Duda 15).
- **Al aceptarla**: la última hora confiable pasa a `new_trusted_time`, se audita con los dos valores, el número queda
  usado y el bloqueo se quita si el reloj queda dentro del margen.

### 0.6 Plan por fase

**Reparto**: dos agentes como máximo a la vez.

- El **agente servidor (S)** hace A, B, C y D, en ese orden.
- El **agente escritorio/Android (W)** hace E y F.
- Las **tres revisiones independientes** las hace un agente nuevo que no escribió el código, en el segundo lugar libre.
  Reciben el código, no las conclusiones:
  - **R1**: el verificador (§10) y el almacén, al terminar A;
  - **R2**: la protección de la hora (la puerta, la guarda, la §6.3 y el desbloqueo), al terminar C;
  - **R3**: la asignación atómica de cupos, al terminar D.

| Turno | Lugar 1                                  | Lugar 2                                                               |
| ----- | ---------------------------------------- | --------------------------------------------------------------------- |
| 1     | S: Fase A                                | —                                                                     |
| 2     | S: Fase B                                | R1 (Fase A)                                                           |
| 3     | S: Fase C                                | W: Fase E1 (activación y Configuración › Licencia, sobre la API de B) |
| 4     | S: Fase D                                | W: Fase F1 (códigos de estado y hora en Android)                      |
| 5     | R2 (Fase C)                              | W: Fase E2 (bloqueo por hora, sin cupo, Terminales)                   |
| 6     | R3 (Fase D)                              | W: Fase F2 (cupo y cola sin conexión en Android)                      |
| 7     | S: correcciones de R1, R2 y R3           | W: correcciones                                                       |
| 8     | Fase G (un agente)                       | —                                                                     |
| 9     | Fase H (coordinador y hasta dos agentes) |                                                                       |

**Fase A · Núcleo (S)**

- **Archivos**: `backend/internal/licensing/`:
  - `verify.go`: el verificador de referencia, adaptado. Encabezado exacto, `kid` del entorno, firma, contenido §4
    estricto con base64url canónico, entorno, destinatario y revisión, con el mismo código de error que el de
    referencia;
  - `state.go`: §5.6 y los seis estados;
  - `trust_development.go` y `trust_production.go`;
  - `store.go`: `identity/licencia/estado.json` y `vigente.lic`. Escritura atómica (temporal, `fsync` y renombrado),
    revisión más alta por `license_id`, última hora confiable, huella y solicitud de desbloqueo pendiente;
  - `machine_darwin.go`, `machine_windows.go` y `machine_other.go`: la huella;
  - `manager.go`: el estado en memoria con mutex, `Now` inyectable y `Status()`.
- **Fuera del paquete**:
  - `localsetup.EnsureLicenseKey`;
  - `main.go`, `host_unix.go` y `host_windows.go`: carga del gestor después de la identidad, y el `CheckStartup`
    nuevo;
  - `config/licencias/llaves-test.json` (las del propietario, §1.2, todas con su `kid`), con su copia sincronizada en
    `scripts/sync-metadata.mjs`;
  - `docs/licencias/ROTAR_LLAVES.md`.
- **Pruebas**:
  - `licensing/vectors_test.go` lee `../../../contracts/licencias/v1/fixtures/vectors.json` y recorre **los 67
    casos**: 20 licencias con su instalación, 17 negativos con su código, 10 de revisión, 13 temporales, 4 pruebas de
    posesión y 3 `.licreq`;
  - el almacén: persistencia, corte a la mitad, permisos y revisión que nunca retrocede;
  - la identidad:
    - 0.14.0 con tres archivos → crea la llave una sola vez;
    - estado sin llave → error y no la regenera;
    - huella distinta → licencia ignorada;
  - la compilación de producción falla;
  - `go-unit`, `fmt-check` y `sync-check`.
- **Base**: la Fase A no guarda nada.

**Fase B · Cliente en línea y sin internet (S)**

- **Cliente** (`internal/licensing/client.go`):
  - HTTPS con TLS verificado, JSON y cuerpos de hasta 16 KiB, con plazos;
  - desafío y prueba de posesión;
  - activar, transferir, consultar y liberar;
  - reintentos con el mismo cuerpo y `request_id`;
  - decide por `error.code` y respeta `Retry-After`;
  - verifica cada `license_jws` con la §10 antes de aceptarlo;
  - cada `server_time` actualiza la última hora confiable.
- **Consulta en segundo plano** (`worker.go`): al arrancar, tras una espera aleatoria de 1 a 5 min, y después cada
  24 h ± 1 h. Nunca bloquea nada y en la bitácora solo deja códigos.
- **Sin internet** (`offline.go`): `.licreq` de `activate` (código opcional), `renew` y `deactivate`, e importar un
  `.lic`.
- **Dirección base**: `license_server_url` opcional en `server.json` (`omitempty`). Sin ella, la del entorno compilado.
- **Protección**: con `test_mode: true` el cliente rechaza todo host que no sea loopback, salvo con
  `ADEL_LICENSE_ALLOW_REMOTE=1`, que solo usa la Fase G.
- **Rutas** (`administration/license.go`):
  - `GET license/status` con cualquier sesión. Con solo la credencial del dispositivo, el resumen (estado, entorno,
    bloqueo).
  - Con `license.manage`: `POST license/activate`, `transfer`, `refresh`, `deactivate`, `offline-request` e `import`.
  - `POST license/identity/regenerate`, además, solo en la computadora principal (`request.Local`).
- **`0028` · Fase B**: el permiso `license.manage`, «Administrar la licencia», solo para el rol `administrator`.
  - Va también en `permissions.go` y `permission_catalog_entries.go` (grupo Sistema).
  - En una instalación nueva, el asistente ya da todos los permisos al administrador (`setup.go:97`).
  - Ningún rol de fábrica de `role_defaults.go` lo lleva, y tampoco el alcance de mesero.
- **Versiones**: `config/product.json` pasa a 0.15.0 y esquema 28.
- **Contrato de la API**:
  - capacidad `license-v1` (`httpapi/router.go:22-66`);
  - `GET license/status` en `openapi.json` (para Android) y en `docs/API_V1.md`, con `TestContractV1`.
- **Pruebas**:
  - un servidor de licencias simulado (`httptest` con TLS) que cumple `openapi.yaml`:
    - cada código de error;
    - reintento idéntico y `REQUEST_CONFLICT`;
    - `Retry-After`;
    - `ACTIVATION_LIMIT` con `transfer_available` `true` y `false`, y `TRANSFER_LIMIT`;
    - un JWS alterado en la respuesta (se ignora);
    - `TEMPORARY_UNAVAILABLE` sin afectar la licencia;
  - `.licreq` verificado con la verificación de referencia;
  - **una prueba de que ninguna prueba automática llama a `licensing.sicrat.com`**: el cliente en `test_mode` rechaza
    el host, y una búsqueda del nombre en los archivos de prueba;
  - `go-int ./internal/administration` (rutas y permiso) y `go-unit`.

**Fase C · Estados en el backend y protección de la hora (S)**

- **La puerta**: `administration/license_gate.go`, en `dispatch`, con `internal/licensing/operations.go` (la tabla de
  0.5.7) y los cinco códigos de 0.6.2.
- **Guarda de la hora** (`licensing/clock.go`):
  - margen de 10 min;
  - revisión al arrancar, cada minuto y en cada operación protegida;
  - auditoría `license.clock_blocked` y `license.clock_unblocked`, con las dos fechas;
  - `POST license/time-check` (§6.3; Duda 5);
  - las operaciones con fecha futura se listan en la auditoría.
- **Desbloqueo de soporte** (§6.4): `licensing/unlock.go`, `POST license/support-request` y `license/support-unlock`,
  `backend/cmd/soporte-desbloqueo`, `tools/soporte-desbloqueo/`, la llave de desarrollo en `~/.adel-pos-dev/soporte/`
  y su pública sincronizada.
- **Estado para las terminales**: la licencia (estado, bloqueo y días de tolerancia) en `auth/me` y
  `restaurant/state`, y el evento `license.changed`.
- **Documentos**: `docs/licencias/DESBLOQUEO_SOPORTE.md`, con la **propuesta para la V1.1** de pasar este desbloqueo
  al panel del servidor de licencias.
- **Base**: la Fase C no guarda nada (el estado está en `identity/licencia/` y la auditoría en `audit_events`).
- **Pruebas**:
  - la tabla de la §6, estado por estado y fila por fila, por `Execute` y por la API v1 (`httptest`), con las reglas
    firmes: cobrar lo abierto en Vencida y Revocada, reportes y respaldos;
  - la completitud de la clasificación;
  - la hora con reloj simulado: bloqueo, pantalla con su causa y desbloqueo al corregir;
  - la §6.3 con `server_time` simulado;
  - el desbloqueo de soporte: válido, repetido, de otra instalación, vencido y alterado;
  - huella distinta y respaldo restaurado → «Sin activar con datos»;
  - `go-int ./internal/administration ./cmd/ad-el-pos-server` y `go-unit`.

**Fase D · Cupos (S)**

- **Paquete**: `backend/internal/licenseseats/`, con pgx.
- **Conteo derivado**, sin escribir al liberar:
  - **Caja**: los turnos activos (`open` o `closing`).
  - **Comandero**: hay fila en la tabla, la última actividad es de menos de 15 min, la terminal tiene alguna sesión
    vigente y su caja no tiene turno activo.
  - **Cocina**: hay fila, el modo está activo y la terminal tiene alguna sesión vigente.
- **Ocupar**: con el lock nuevo **7820148311** (de transacción, exclusivo, siempre después de 7820148305). Dentro se
  cuenta y se inserta o actualiza.
  - Quien ya tiene su cupo sigue sin contarse otra vez.
  - Con menos cupos en una licencia nueva, nadie se interrumpe y no se ocupan cupos nuevos.
- **Liberar**:
  - **Caja**: al cerrar el turno, también desde otra computadora.
  - **Comandero**: cuando cierra sesión el último usuario de la terminal, cuando se abre turno en ella o tras 15 min
    sin actividad de captura.
  - **Cocina**: al salir del modo o cerrar la sesión.
  - **Desde Terminales**: `POST devices/{id}/seat/release` cierra las sesiones de esa terminal, con la auditoría
    `license.seat_released`.
- **`sessions-summary`**: conserva `by_kind`, para el escritorio 0.14.0, y agrega `seats` con `in_use` y `limit` por
  cupo, y `limited: true`.
- **Terminales**: `GET devices` dice qué cupo ocupa cada una.
- **Ruta nueva**: `POST kitchen/display-mode`.
- **`0028` · Fase D**: `app.license_seats`, con `device_id` (FK), `kind` (`order_taker` o `kitchen_display`),
  `acquired_at` y `last_activity_at`, y la PK `(device_id, kind)`.
- **`CLAUDE.md` §3**: el lock 7820148311 y su orden.
- **Pruebas** (`go-int`):
  - ocupar y liberar por cada causa, también los 15 min con reloj simulado;
  - concurrencia: N transacciones en conexiones distintas por el último cupo de cada tipo, y solo una lo obtiene;
  - la tableta compartida con tres PIN ocupa un cupo, y volver a «Selecciona tu usuario» no lo libera;
  - una licencia nueva con menos cupos;
  - el plan básico (1, 0 y 0).

**Fase E · Escritorio (W)**

- **Pantallas** (`desktop/src/pages/license/`):
  - la activación del primer arranque, con el formato del código validado al escribir (Duda 4) y «Activar sin
    internet»;
  - Configuración › Licencia, con sus botones;
  - la pantalla «Sistema detenido por la fecha y hora de la computadora» (§6.2), con «Comprobar la hora con internet» y
    «Desbloqueo de soporte».
- **Avisos**:
  - 7 días antes, al iniciar sesión, una vez al día, para quien tiene el permiso, y un indicador para los demás;
  - en tolerancia, uno diario para todos;
  - vencida o revocada, uno fijo.
- **Mensajes**: los de sin cupo, en el momento. `services/license.ts` tiene los textos por código, nunca el `message`
  del servidor de licencias.
- **Terminales**: el cupo de cada terminal y «Liberar cupo».
- **Puente y archivos**:
  - operaciones nuevas del puente en `desktop/src-tauri/src/api.rs` (lista cerrada) y en `native-driver.ts`;
  - guardar el `.licreq` e importar el `.lic` con los diálogos de Tauri;
  - todo escondido por la capacidad `license-v1`.
- **Pruebas**:
  - `vitest`, `build` y `cargo`;
  - Playwright Chromium y WebKit `workflow-stagel1-*.spec.ts`, contra el Go temporal en `test_mode` con un servidor
    de licencias simulado en loopback y licencias firmadas con las llaves de `vectors.json`;
  - capturas en tema claro de las pantallas nuevas.

**Fase F · Android 0.4.0 (W)**

- **Código**:
  - los códigos en `core/domain/…/AppError.kt` y `core/data`;
  - los mensajes de §8;
  - el aviso discreto en tolerancia y el fijo por la hora;
  - en la cola (`OrderQueueRepository` y `OrderSender`), el pedido rechazado por licencia, cupo u hora queda marcado
    con el motivo y no se reintenta. Puede necesitar una migración de Room si hace falta un motivo nuevo.
- **Versión**: `versionCode` 4 y 0.4.0.
- **Documentos**: `docs/ANDROID_INSTALAR.md` (activar antes de emparejar).
- **Pruebas**:
  - `ktfmtCheck` y las unitarias;
  - las instrumentadas y `android-e2e` en `Pixel_Tablet` y `ADEL_A1_Tablet_API26`, siempre con `AdelTestRunner`, con
    un pedido en cola rechazado por cupo.

**Fase G · Contra `licensing.sicrat.com`**

- Lo que dice la §9.2: instancias de desarrollo separadas, cada una con su carpeta, su base y su identidad, con
  `ADEL_LICENSE_ALLOW_REMOTE=1`.
- Los códigos se leen dentro de un script, sin mostrarlos.
- Al terminar se liberan las tres licencias y se deja la lista para el propietario.

**Fase H**

- La regresión completa, una sola vez.
- Documentos: `docs/licencias/INTEGRACION.md` y `PASO_A_PRODUCCION.md`, `CLAUDE.md`, `android/CLAUDE.md`,
  `docs/API_V1.md` y `docs/ANDROID_INSTALAR.md`.
- El APK 0.4.0 y el `.pkg` 0.15.0, con su parada para instalarlo.
- Fusión y push.

#### 0.6.1 Migración `0028_licencias.sql` (una sola, una sección por fase)

| Fase | Qué guarda                                                                             |
| ---- | -------------------------------------------------------------------------------------- |
| A    | Nada: la llave y el estado están en `identity/`                                        |
| B    | El permiso `license.manage` y su concesión al rol `administrator`                      |
| C    | Nada: la hora está en `identity/licencia/estado.json` y la auditoría en `audit_events` |
| D    | `app.license_seats` con sus índices                                                    |

#### 0.6.2 Códigos de error de la API (uno por caso)

**La puerta** (todos 409, para que no entren en la auditoría de `access.denied` de los 403, `service.go:255-295`):

| Código                     | Caso                                                | `details`                                                                          |
| -------------------------- | --------------------------------------------------- | ---------------------------------------------------------------------------------- |
| `LICENSE_NOT_ACTIVATED`    | Sin activar, o sin activar con datos                | `state` (`not_activated` o `not_activated_with_data`) y `copied`                   |
| `LICENSE_EXPIRED`          | Vencida; también abrir turno sin cuentas pendientes | `operation`, `expires_at` y `grace_ended_at`                                       |
| `LICENSE_REVOKED`          | Revocada                                            | `reason` (`deactivated`, `transferred` o `revoked`)                                |
| `LICENSE_SEAT_UNAVAILABLE` | Sin cupo                                            | `seat` (`cash_registers`, `order_takers` o `kitchen_displays`), `in_use` y `limit` |
| `SYSTEM_CLOCK_INVALID`     | Hora incorrecta                                     | `local_time`, `last_activity_time` y `margin_minutes`                              |

`SYSTEM_CLOCK_INVALID` no empieza con «LICENSE»: así nadie confunde la hora con la licencia (§6.2).

**La gestión de la licencia** (`license/*`):

| Código                       | HTTP | Caso                                                                                                      |
| ---------------------------- | ---- | --------------------------------------------------------------------------------------------------------- |
| `LICENSE_KEY_INVALID_FORMAT` | 400  | El código comercial no tiene el formato                                                                   |
| `LICENSE_KEY_NOT_FOUND`      | 404  | `LICENSE_NOT_FOUND`                                                                                       |
| `LICENSE_SERVER_REVOKED`     | 409  | `REVOKED`                                                                                                 |
| `LICENSE_ACTIVATION_LIMIT`   | 409  | Con `transfer_available`                                                                                  |
| `LICENSE_TRANSFER_LIMIT`     | 409  | Se muestra «Comunícate con soporte»                                                                       |
| `LICENSE_SERVER_BUSY`        | 429  | `RATE_LIMITED`, con `retry_after_seconds`                                                                 |
| `LICENSE_SERVER_UNAVAILABLE` | 503  | `TEMPORARY_UNAVAILABLE`, red o TLS                                                                        |
| `LICENSE_SERVER_ERROR`       | 502  | `INVALID_REQUEST`, `INVALID_PROOF`, `REQUEST_CONFLICT`, `INCOMPATIBLE_SCHEMA` u otra respuesta inesperada |
| `LICENSE_RESPONSE_INVALID`   | 502  | El JWS de la respuesta no pasa la §10; se ignora                                                          |
| `LICENSE_FILE_INVALID`       | 400  | `.lic` rechazado; `details.reason` dice el motivo del verificador                                         |
| `LICENSE_TIME_MISMATCH`      | 409  | La §6.3 no coincide; con `server_time`                                                                    |
| `SUPPORT_UNLOCK_INVALID`     | 400  | `reason`: `used`, `other_installation`, `expired`, `altered` o `not_requested`                            |
| `IDENTITY_UNAVAILABLE`       | 503  | La identidad o la llave de licencias no están disponibles                                                 |

### 0.7 Dudas del contrato

**Contradicciones entre la especificación y el contrato** (manda el contrato):

1. **El bloqueo por hora y la regla firme de cobro.** El contrato (§6) dice que ningún estado impide cobrar una cuenta
   ya abierta en una instalación activada, y la especificación lo repite (§4, «sin excepciones»). Su §6.2 bloquea
   cobrar con la hora incorrecta, «excepción deliberada».
   - **Lectura propuesta**: la hora no es uno de los seis estados de la §6. Es una protección local que la §5.6 y la
     §15 dejan a AD-EL POS, así que la regla firme no aplica.
   - **Hay que confirmarlo con el propietario.** Es una decisión de negocio: si no se acepta, el cobro seguiría
     abierto con la hora incorrecta.
   - **Propuesta para la V1.1**: escribir la excepción en la §6.
2. **Comandero: «una sesión» o «una terminal».** El contrato (§7) dice «una sesión, Android o de escritorio». La
   especificación dice «por terminal, no por usuario».
   - En AD-EL POS cada mesero tiene su sesión en la tableta, así que contar sesiones contaría tres cupos en una tableta
     con tres PIN.
   - **Lectura propuesta**: «sesión» de la terminal. Es compatible y es lo que el propietario pidió.
   - **V1.1**: decir «una terminal».
3. **Caja: «una terminal operando como caja».** El contrato dice que una caja no es una computadora, un usuario, un
   turno ni un cajón. En AD-EL POS el turno es de la caja física y dos terminales pueden compartirlo.
   - **Propuesta**: contar los turnos activos, como dice la tabla de la especificación. La segunda terminal de una caja
     compartida captura con turno y no ocupa otro cupo.
   - Hay que confirmarlo (la otra lectura contaría terminales). **V1.1**: aclarar la unidad.

**Lo que el contrato no cubre**:

4. **El formato del código comercial.** El contrato solo dice que es una cadena de 1 a 256 caracteres, con un
   ejemplo. La especificación pide validarlo al escribirlo.
   - **Propuesta**: el formato del servidor de licencias (`src/Domain/ActivationCode.php`): 15 símbolos de
     `23456789ABCDEFGHJKMNPQRSTUVWXYZ` en grupos 3-4-4-4, guiones opcionales y sin distinguir mayúsculas. Se normaliza
     antes de enviarlo.
   - Depende de la implementación, no del contrato. **V1.1**: escribirlo en la §8.3.
5. **Sin activación no hay `server_time`.** El desafío no lo trae, y «Comprobar la hora con internet» no tiene fuente
   contractual en una instalación sin activar.
   - **Propuesta**: con activación, `refresh` (su `server_time` es contractual). Sin ella, el `expires_at` del desafío
     menos 300 s (la §8.1 fija los 5 minutos).
   - **V1.1**: `server_time` en la respuesta del desafío.
6. **El inventario no está en la tabla de la §6.** **Propuesta**: fila Administración (se permite en Vencida y
   Revocada). Registra con fecha. **V1.1**: agregar la fila.
7. **Reorganizar cuentas** (dividir, juntar, mover, transferir productos, asignar mesero y datos del cliente) no está
   en la tabla.
   - **Propuesta**: como «lo ya capturado» (F2). Sirve para cobrar lo abierto y no agrega productos.
   - **V1.1**: aclararlo.
8. **«KDS: mostrar pedidos ya enviados».** Avanzar estados («En preparación», «Listo»), liberar lo programado y
   reintentar la impresión no se mencionan.
   - **Propuesta**: en la fila KDS. Son necesarios para servir y cobrar lo capturado.
   - **V1.1**: aclararlo.
9. **«Cuentas pendientes de cobro»** (abrir turno con la licencia vencida) no está definido. **Propuesta**: hay al
   menos una cuenta `open` con unidades activas.
10. **Arranque en «Sin activar».** Administración «No» impediría el asistente inicial (`POST setup`) y la inscripción
    de la propia computadora (`devices/enroll` local), y los dos hacen falta para iniciar sesión y activar.
    **Propuesta**: se permiten como arranque (solo sin establecimiento y solo locales).
11. **«Liberar» (`deactivate`)** no está en la fila «Activar, transferir, renovar o importar». **Propuesta**: se
    permite en todos los estados, como esa fila.
12. **Emparejar terminales es Administración**, así que no se puede en Sin activar ni en Sin activar con datos. Es
    coherente con «activar la computadora principal antes de emparejar». Va en `docs/ANDROID_INSTALAR.md`.

**Lo que decide AD-EL POS** (la especificación lo deja abierto):

13. **La copia del `README.md`** enlaza `../../docs/CONTRATO_LICENCIAS_ADEL_POS.md`, la ruta del repositorio de
    origen. En AD-EL POS no lleva a nada. No se edita.
14. **La respuesta del desbloqueo de soporte** lleva una etiqueta del `server_id` (40 bits), no el `server_id`
    completo como dice la especificación. Así el código cabe en 24 caracteres. **Propuesta**: aceptarlo. Junto con el
    número de un solo uso, ata la respuesta a la instalación.
15. **Las 24 horas del desbloqueo** se miden con un reloj que se sabe incorrecto. **Propuesta**: la diferencia con el
    reloj local al crear la solicitud. Si el reloj se corrige en medio, se pide una solicitud nueva.
16. **«La hora de la última operación registrada»** (especificación §6.1), si se leyera de la base en cada arranque,
    impediría la corrección de la §6.3: la actividad con fecha futura seguiría en la base. **Propuesta**: solo inicia
    la última hora confiable cuando no existe (Decisión propuesta 8).
17. **Un cajero que abre Preparación ocupa un cupo de cocina.** En el plan básico (0 KDS) nadie podría abrir
    Preparación. Es coherente con «Ni tabletas ni modo Cocina», pero hay que confirmarlo.
18. **«La verificación se hace en cada operación protegida»** (§10). **Propuesta**: la firma se comprueba al aceptar y
    al arrancar, y en cada operación se evalúa en memoria el estado de la licencia ya verificada, con el reloj y la
    huella.

**Propuestas para la V1.1** (además de las de 1 a 8): **pasar el desbloqueo de soporte de la §6.4 al panel del
servidor de licencias**, como pide la especificación.

### Decisiones propuestas (para numerar como L1-Dn)

1. **El estado de la licencia en `identity/`, no en la base.**
   - Contenido:
     - `identity/license-key.pem` (Ed25519, PKCS#8, 0600);
     - `identity/licencia/estado.json`, con la revisión más alta por `license_id`, la última hora confiable, la huella
       y la solicitud de desbloqueo pendiente;
     - `identity/licencia/vigente.lic`.
   - Escritura atómica y un proceso por carpeta de datos (`host.lock`).
   - Por qué: no viaja en los respaldos (contrato §2), así que restaurar uno viejo no devuelve una revisión anterior ni
     baja la última hora confiable.
2. **La llave Ed25519 aparte de los tres archivos de la identidad**, porque `identity.json` usa
   `DisallowUnknownFields`.
   - Instalación nueva: se crea con la identidad.
   - Instalación existente sin `licencia/`: se crea una sola vez.
   - `licencia/estado.json` sin llave: pérdida. No se regenera sola y se pide la pantalla de la §1.1.
3. **Huella del equipo.**
   - Lectura: `IOPlatformUUID` con `/usr/sbin/ioreg` y `MachineGuid` con `x/sys/windows/registry`.
   - Se guarda su SHA-256 con prefijo.
   - Si no se puede leer tras tres intentos, falla cerrada, con su propio mensaje.
4. **El comportamiento de la §1.1 de 0.5.3.** Se regenera la identidad completa (`server_id`, ECDSA y Ed25519) solo
   con confirmación en la computadora principal, y se revocan las terminales emparejadas por red. La identidad
   anterior se aparta y no se toca ningún dato.
5. **La puerta única** en `administration.Service.dispatch`, con una tabla explícita por `(método, ruta)` y su prueba
   de completitud (`api.rs`, `openapi.json` y las rutas locales). Una escritura desconocida toma la clase más
   restrictiva.
6. **La clasificación de lo que la tabla no lista.**
   - El inventario es Administración.
   - Reorganizar cuentas es «lo ya capturado».
   - Avanzar estados, liberar e imprimir en cocina van en la fila KDS.
   - El agente de impresión, `auth/*`, las preferencias, «Reiniciar» y liberar un cupo van siempre.
   - `setup` y `devices/enroll` local se permiten en el arranque.
   - Liberar la licencia, en todos los estados.
7. **Con «Hora del equipo incorrecta» solo siguen las lecturas, reportes y exportaciones, respaldos, la licencia,
   `auth/*`, las preferencias, «Reiniciar», el SSE y el agente de impresión.** Todo lo demás que escribe se bloquea,
   también la administración.
8. **La última hora confiable.**
   - Fuentes: `server_time`, `issued_at`, el reloj local cada minuto y cada operación aceptada (en memoria, a disco
     cada minuto).
   - `max(audit_events.occurred_at)` solo la inicia cuando falta.
   - La §6.3 y la §6.4 la reemplazan.
9. **El reloj inyectable**: `Now func() time.Time` en el gestor de licencias y en el de cupos. En los procesos de
   prueba, un desfase leído de un archivo, solo con `test_mode`.
10. **Margen de 10 minutos.** Se revisa al arrancar, cada minuto y en cada operación protegida, solo en el servidor
    principal.
11. **Los códigos de error de 0.6.2.** Los cinco de la puerta en 409; la hora con `SYSTEM_CLOCK_INVALID`.
12. **La unidad de cada cupo.**
    - Caja: un turno activo (`open` o `closing`) de una caja física.
    - Comandero: una terminal que captura sin turno activo en su caja.
    - Cocina: una terminal en Preparación (`POST kitchen/display-mode`).
    - Conteo derivado, `app.license_seats` y el lock 7820148311, siempre después de 7820148305.
13. **La actividad de captura de 0.5.5.** Ocupa el cupo al primer intento de crear o modificar. Las previas solo
    renuevan la actividad.
14. **El entorno se fija al compilar** con `adelpos_production` y `go:embed`: sin llaves de producción no compila.
    - `CheckStartup` compara el entorno compilado con `server.json`.
    - Las llaves viven en `config/licencias/` y se sincronizan con `sync-metadata.mjs`.
15. **Las pruebas automáticas usan solo las llaves de `vectors.json`.**
    - Un Go temporal en `test_mode` confía además en las de prueba de `vectors.json`, solo si se le indica su ruta
      (`ADEL_TEST_LICENSE_TRUST`), y habla con un servidor de licencias simulado en loopback.
    - En `test_mode`, el cliente rechaza todo host que no sea loopback, salvo con `ADEL_LICENSE_ALLOW_REMOTE=1`
      (Fase G).
16. **El desbloqueo de soporte de 0.5.10.** El comando en `backend/cmd/soporte-desbloqueo`, la envoltura en
    `tools/soporte-desbloqueo/`, el código de 24 caracteres y la respuesta JWS con `typ` `adelpos-unlock+jws`.
17. **«Comprobar la hora con internet»**: con `refresh` si hay activación; sin ella, el `expires_at` del desafío menos
    300 s.
18. **Versiones mínimas de cliente sin cambio**, como en A3-D9. Los mensajes de Go están en español sencillo para los
    clientes anteriores.
19. **El permiso `license.manage`, solo para el Admin de fábrica.**
    - Ver el estado no pide permiso.
    - La gestión se hace desde cualquier terminal de escritorio con el permiso.
    - Regenerar la identidad, solo en la computadora principal.
20. **La firma se verifica al aceptar y al arrancar.** En cada operación se evalúa en memoria la licencia ya
    verificada.
21. **El formato del código comercial** es el del servidor de licencias mientras no esté en el contrato (Duda 4).
22. **`sessions-summary` conserva `by_kind`** (escritorio 0.14.0) y agrega `seats`, con `limited: true`.
23. **La versión 0.15.0 y el esquema 28 en la Fase B**, con la primera sección con contenido de la `0028`.

### Pruebas de la Fase 0

| Prueba                                                           | Resultado                                                                                    |
| ---------------------------------------------------------------- | -------------------------------------------------------------------------------------------- |
| `git clone` del servidor de licencias y `checkout v1.0.0`        | Commit `309dcff220430669c6167579d51145b4c8b8cbd5` (etiqueta anotada `2a92755…`); clon limpio |
| SHA-256 del contrato de la etiqueta                              | `e3435f16…314058`: **coincide**                                                              |
| SHA-256 de las 35 copias contra el clon                          | 35 de 35 idénticas                                                                           |
| Blobs del índice contra `git ls-tree -r v1.0.0`                  | 35 de 35 iguales                                                                             |
| Verificador de referencia en el clon (orden de la §2 de la guía) | **OK, 67 casos** (20 + 17 + 10 + 13 + 4 + 3), salida 0                                       |
| El mismo verificador desde la copia                              | El mismo resultado                                                                           |
| `prettier --check` de las copias, antes de excluirlas            | 5 archivos con cambios: por eso se excluyen                                                  |
| `ISO_TAG=P0 iso.sh … format`                                     | Solo formateó `ORIGEN.md`; SHA-256 de las copias y de `docs/etapas/` iguales antes y después |
| `ISO_TAG=P0 iso.sh … fmt-check`, con las copias en el índice     | Aprobado (`check-legacy-data`, Prettier, gofmt y `cargo fmt --check`)                        |
| `ISO_TAG=P0 iso.sh … sync-check`                                 | Aprobado («Metadatos coherentes.»)                                                           |
| `go list ./...` en `backend`                                     | Ningún paquete de `contracts/`                                                               |
| `git ls-files --eol` de las copias                               | `i/lf` o sin fin de línea; sin conversiones                                                  |
| Comprobación con `shasum -c` de `ORIGEN.md`                      | 35 de 35 OK                                                                                  |
| `ioreg -rd1 -c IOPlatformExpertDevice`                           | Un renglón `IOPlatformUUID` con el formato esperado (valor no mostrado)                      |
| `x/sys/windows/registry` en la caché de módulos                  | Disponible, sin dependencias nuevas                                                          |

**No se ejecutó**, porque no hubo cambios de código: `go-unit`, `go-int`, `vitest`, `cargo`, Playwright y Android.
Tampoco hubo Go temporal ni emuladores.

### Commits de la Fase 0 (rama `etapa-l1-p0`)

- `b5e6adc` etapa L1 fase 0: Prettier y el editor no tocan las copias del contrato de licencias (`.prettierignore`,
  `.editorconfig`). Va primero para que cada commit pase `fmt-check` por sí solo.
- `cec3087` etapa L1 fase 0: copia sin cambios del contrato de licencias V1.0 (etiqueta v1.0.0)
  (`contracts/licencias/v1/`, `contracts/licencias/ORIGEN.md` y `docs/licencias/` con el contrato y la guía).

### Decisiones del coordinador al cerrar la Fase 0

Ninguna es una parada. Las dudas 1 a 3, las que podían ser una decisión de negocio, las resuelve el texto del contrato.

**Las dudas del contrato**

- **L1-D1 · La hora es una protección local, no un estado de la licencia (Duda 1).**
  - La §6.2 de la especificación se aplica tal cual: con «Hora del equipo incorrecta» se bloquea también cobrar.
  - Por qué no choca con el contrato:
    - la regla firme dice «ningún **estado** impide cobrar», y la hora no es uno de los seis estados de la §6;
    - la §5.6 y la §15 del contrato dejan a AD-EL POS «las demás protecciones contra cambios de reloj»;
    - la especificación la llama «excepción deliberada».
  - **Propuesta para la V1.1**: escribir la excepción en la §6.
- **L1-D2 · El cupo de comandero es por terminal (Duda 2).**
  - La «sesión» de la §7 se lee como la sesión de captura **de la terminal**: varios meseros con su PIN en una tableta
    ocupan un cupo, como «cambiar de cajero en la misma terminal no consume otro cupo».
  - Es lo que pide la especificación y es compatible con el contrato. **V1.1**: decir «una terminal».
- **L1-D3 · El cupo de caja es por terminal, no por turno (Duda 3).** Manda el contrato: «una caja es una terminal
  operando como caja; no es una computadora, un usuario, un turno ni un cajón». En AD-EL POS el turno es de la caja
  física y dos terminales pueden compartirlo, así que:
  - **Abrir un turno**: la terminal que lo abre ocupa un cupo de caja.
  - **Capturar en una terminal cuya caja tiene un turno activo**, aunque lo abriera otra terminal: esa terminal opera
    como caja. Ocupa su cupo de caja en la primera captura. Sin cupo libre, se rechaza con el cupo de caja.
  - **Capturar sin turno activo en su caja**: ocupa un cupo de comandero (L1-D13).
  - **Cobrar y las operaciones de efectivo nunca piden cupo.** La §7 solo valida crear o modificar pedidos, y la regla
    firme protege el cobro de lo abierto.
  - **Al cerrar el turno**, también desde otra computadora, se liberan los cupos de caja de todas las terminales que lo
    usaban.
  - Así, con 1 caja, una segunda computadora con la misma caja física no captura gratis.
  - **V1.1**: aclarar la unidad con cajas compartidas.
- **L1-D4 · El formato del código comercial** (Duda 4): mientras no esté en el contrato, se valida al escribirlo con el
  formato del servidor de licencias (15 símbolos de `23456789ABCDEFGHJKMNPQRSTUVWXYZ` en grupos 3-4-4-4, guiones
  opcionales y sin distinguir mayúsculas). Se normaliza antes de enviarlo. **V1.1**: escribirlo en la §8.3.
- **L1-D5 · «Comprobar la hora con internet»** (Duda 5):
  - con activación, se usa `refresh`, cuyo `server_time` es contractual;
  - sin activación, el `expires_at` del desafío menos 300 s (la §8.1 fija los 5 minutos).
  - **V1.1**: `server_time` en el desafío.
- **L1-D6 · Lo que la tabla de la §6 no lista** (Dudas 6 a 12):
  - **El inventario** es Administración y registra con fecha.
  - **Reorganizar cuentas** (dividir, juntar, mover, transferir productos, asignar mesero y datos del cliente) es «lo ya
    capturado».
  - **Avanzar estados de cocina**, liberar lo programado y reintentar la impresión van en la fila KDS.
  - **Cuentas pendientes de cobro**: al menos una cuenta `open` con unidades activas.
  - **En el arranque** se permiten `setup` y `devices/enroll`, solo sin establecimiento y solo por la escucha local.
  - **«Liberar la licencia»** se permite en todos los estados.
  - **Emparejar terminales** es Administración: no se puede sin activar. Va en `docs/ANDROID_INSTALAR.md` como
    «activar la computadora principal antes de emparejar».
  - **Siempre se permiten**: el agente de impresión, `auth/*`, las preferencias, «Reiniciar», liberar un cupo, el SSE y
    los avisos de platillos listos.
- **L1-D7 · Un cajero que abre Preparación ocupa un cupo de cocina (Duda 17).** En el plan básico (0 KDS) nadie la abre:
  es lo que dice la especificación («Ni tabletas ni modo Cocina. No hay excepción»). Sin cupo, la pantalla muestra el
  mensaje de §5.
- **L1-D8 · El enlace roto del `README.md` copiado** (Duda 13) no se edita: `ORIGEN.md` dice dónde está el contrato.

**Diseño**

- **L1-D9 · La llave Ed25519 y el estado de la licencia van en `identity/`, no en la base** (Decisiones propuestas 1 y
  2).
  - Archivos:
    - `identity/license-key.pem` (Ed25519, PKCS#8, 0600), aparte de los tres archivos de la identidad, porque
      `identity.json` usa `DisallowUnknownFields`;
    - `identity/licencia/estado.json`;
    - `identity/licencia/vigente.lic`.
  - Escritura atómica y un proceso por carpeta de datos.
  - Por qué: `identity/` no viaja en ningún respaldo (§2 del contrato). Restaurar uno viejo no puede devolver una
    revisión anterior ni bajar la última hora confiable.
  - Una instalación existente crea la llave una sola vez. Sin la llave, no se regenera sola.
- **L1-D10 · La huella del equipo** (Decisión propuesta 3):
  - se lee con `IOPlatformUUID` (`/usr/sbin/ioreg`) y `MachineGuid` (`x/sys/windows/registry`, ya en el módulo);
  - se guarda solo su SHA-256 con prefijo;
  - si no se puede leer tras tres intentos, falla cerrada, con su propio mensaje.
- **L1-D11 · Instalación copiada a otra computadora** (§1.1 y Decisión propuesta 4): el comportamiento exacto de 0.5.3.
  - Se regenera la identidad completa (`server_id`, ECDSA y Ed25519) solo con confirmación en la computadora principal y
    con «Administrar la licencia».
  - La identidad anterior se aparta para soporte.
  - Las terminales emparejadas por red se revocan, con el motivo `identity_regenerated`.
  - No se toca ningún dato.
- **L1-D12 · La puerta única** en `administration.Service.dispatch` (Decisión propuesta 5):
  - una tabla explícita por `(método, ruta)`, con fila de la §6, «registra con fecha» y cupo;
  - una prueba de completitud contra `api.rs`, `openapi.json` y las rutas locales;
  - una escritura desconocida toma la clase más restrictiva.
- **L1-D13 · Cupos** (Decisiones propuestas 12 y 13, con L1-D2, L1-D3 y L1-D7):
  - **Datos**: conteo derivado, más `app.license_seats` para comandero, cocina y caja por terminal.
  - **Lock**: 7820148311, de transacción, siempre después de 7820148305.
  - **Ocupan el cupo y renuevan la actividad**: abrir una cuenta, el borrador, confirmar, editar lo enviado y
    `offline-orders`.
  - **Solo renuevan la actividad**: las previas, si la terminal ya tiene el cupo.
  - **Liberar el comandero**: a los 15 minutos (valor de configuración del backend), al cerrar sesión el último
    usuario de la terminal o al abrir turno en ella.
  - **Cocina**: `POST kitchen/display-mode`.
- **L1-D14 · Con «Hora del equipo incorrecta»** (Decisión propuesta 7) solo siguen:
  - las lecturas y las previas, los reportes y las exportaciones, los respaldos;
  - la licencia, `auth/*`, las preferencias y «Reiniciar»;
  - el SSE y el agente de impresión.
  - Todo lo demás que escribe se bloquea, también la administración.
- **L1-D15 · La última hora confiable** (Decisión propuesta 8 y Duda 16):
  - Fuentes: `server_time`, `issued_at`, el reloj local cada minuto y cada operación aceptada (en memoria; a disco cada
    minuto y al apagar).
  - `max(audit_events.occurred_at)` solo la inicia cuando falta.
  - Solo la §6.3 y la §6.4 la reemplazan.
- **L1-D16 · El reloj y el margen** (Decisiones propuestas 9 y 10):
  - `Now func() time.Time` en el gestor de licencias y en el de cupos;
  - en los procesos de prueba, un desfase leído de un archivo, **solo** con `test_mode`;
  - margen de 10 minutos, revisado al arrancar, cada minuto y en cada operación protegida, solo en el servidor
    principal.
- **L1-D17 · Los códigos de error** de 0.6.2 (Decisión propuesta 11):
  - los cinco de la puerta, con HTTP 409: `LICENSE_NOT_ACTIVATED`, `LICENSE_EXPIRED`, `LICENSE_REVOKED`,
    `LICENSE_SEAT_UNAVAILABLE` (con `seat`, `in_use` y `limit`) y `SYSTEM_CLOCK_INVALID`;
  - los de `license/*`, con su HTTP.
  - Se documentan en `docs/API_V1.md` y en `openapi.json`.
- **L1-D18 · El entorno se fija al compilar** (Decisiones propuestas 14 y 15):
  - con la etiqueta `adelpos_production` y `go:embed`: sin llaves de producción, no compila;
  - `CheckStartup` compara el entorno compilado con `server.json`;
  - las llaves viven en `config/licencias/` y se sincronizan con `sync-metadata.mjs`.
  - **Pruebas**: solo las llaves de `vectors.json`, por `ADEL_TEST_LICENSE_TRUST` y solo con `test_mode`, con un
    servidor de licencias simulado en loopback.
  - **Red**: en `test_mode`, el cliente rechaza todo host que no sea loopback, salvo con `ADEL_LICENSE_ALLOW_REMOTE=1`,
    que solo usa la Fase G.
- **L1-D19 · El desbloqueo de soporte** (Decisión propuesta 16 y Dudas 14 y 15):
  - el comando en `backend/cmd/soporte-desbloqueo` y la envoltura en `tools/soporte-desbloqueo/`;
  - el código de solicitud de 24 caracteres (base32 de Crockford), con la etiqueta de 40 bits del `server_id`;
  - la respuesta JWS con `typ` `adelpos-unlock+jws`;
  - las 24 horas, medidas con la diferencia del reloj local respecto a la solicitud;
  - la llave privada de desarrollo, en `~/.adel-pos-dev/soporte/`.
- **L1-D20 · La firma** se verifica al aceptar una licencia y al arrancar. En cada operación se evalúa en memoria la
  licencia ya verificada (Duda 18).
- **L1-D21 · Permisos**: `license.manage` («Administrar la licencia») solo para el rol `administrator`.
  - Ver el estado no pide permiso.
  - La gestión se hace desde cualquier terminal de escritorio con el permiso.
  - Regenerar la identidad, solo en la computadora principal.
- **L1-D22 · Compatibilidad**:
  - versiones mínimas de cliente sin cambio;
  - `sessions-summary` conserva `by_kind` para el escritorio 0.14.0 y agrega `seats` con `limited: true`;
  - la capacidad nueva es `license-v1`.
- **L1-D23 · Migración y versión**:
  - una sola migración, `0028_licencias.sql`, con las secciones «Fase B» (el permiso) y «Fase D» (`app.license_seats`);
  - la versión 0.15.0 y el esquema 28 los sube la primera fase que cree la `0028`.
- **L1-D24 · Reparto en paralelo** (el propietario quitó el límite de agentes simultáneos en esta etapa, 23/11/2026).
  - **Ahora, a la vez**:
    - un agente hace la Fase A y después la B (núcleo y cliente);
    - otro hace la Fase D (cupos), con los límites detrás de una interfaz.
  - **Después de fusionar la A**:
    - la Fase C (puerta, hora y desbloqueo), con otro agente;
    - la revisión R1 del verificador.
  - **Después**:
    - la Fase E (escritorio), con la API de B, C y D fusionada;
    - la Fase F (Android), con los códigos de C y D;
    - las revisiones R2 (hora) y R3 (cupos);
    - la Fase G contra `licensing.sicrat.com`, en cuanto esté el cliente.
  - Cada agente en su árbol, con su `ISO_TAG` y sus puertos.

---

## Fase A · Núcleo: llave, verificador y estado

Hizo la fase el agente del núcleo, en `$SP/wt/l1-a`, rama `etapa-l1-a`, desde `etapa-l1` (`1ceee93`), con `ISO_TAG=LA`.

- Sin Go temporal y sin emuladores.
- Ninguna llamada a `licensing.sicrat.com`.
- No se leyeron los códigos de licencia de prueba.
- La Fase A no guarda nada en la base: no hay `0028` ni cambio de versión.

### Qué se hizo

- **Verificador (§10)**, en `backend/internal/licensing/verify.go`, `protocol.go` y `state.go`:
  - adaptado del de referencia (`contracts/licencias/v1/verify`), con el mismo orden de comprobaciones y los mismos
    códigos de rechazo (`invalid_signature`, `stale_revision`…);
  - lo único que cambia: los cupos se entregan como `int`, y la regla de la revisión queda aparte (`checkRevision`)
    para usarla con el historial guardado;
  - suma `SignProof`, la firma de la prueba de posesión que usará la Fase B.
- **Estado**: los seis estados de la §6 y la §5.6 (`status.go`, `state.go`), calculados en memoria con
  `ahora_efectivo = máximo(reloj, última hora confiable)`.
- **Entorno compilado** (L1-D18):
  - `trust_development.go` (`!adelpos_production`) embebe `llaves-test.json`. Entorno de firma `test`, `DefaultServerURL`
    = la del servidor de pruebas.
  - `trust_production.go` (`adelpos_production`) embebe `llaves-production.json`, que no existe. **`go build -tags
adelpos_production` no compila** («pattern llaves-production.json: no matching files found»). Su `DefaultServerURL`
    queda vacía.
  - `CheckStartup` (`licensing.go:24`) compara el entorno compilado con `server.json` y también comprueba las llaves
    embebidas.
- **Llaves**:
  - `config/licencias/llaves-test.json` es exactamente la lista de §1.2: `adelpos-test-202611-1` con su pública;
  - `scripts/sync-metadata.mjs` copia cada `config/licencias/llaves-<entorno>.json` a `backend/internal/licensing/`,
    y `sync-check` lo vigila;
  - un archivo con una llave de otro entorno, un `kid` repetido o una llave inválida impide arrancar.
- **Almacén** `identity/licencia/` (`store.go`), L1-D9:
  - **`estado.json`** guarda la licencia vigente (su JWS), la revisión más alta de cada `license_id`, la última hora
    confiable con su fuente, la huella del equipo y la identidad dueña. Es el punto de confirmación: archivo temporal,
    `fsync` y renombrado.
  - **`vigente.lic`** es la copia tal como viaja, escrita después. Si no coincide, al arrancar se vuelve a escribir.
  - **Permisos**: carpeta 0700 y archivos 0600; en Windows, la ACL de `fileacl`.
  - **Al arrancar**, la licencia guardada vuelve a pasar la §10 completa (L1-D20): primero la de `estado.json` y después
    la de `vigente.lic`. Las dos deben ser exactamente la revisión aceptada. Así un archivo alterado, de otra llave o de
    otra instalación se ignora y queda la última válida.
  - **`estado.json` dañado** se aparta (`estado.json.danado-AAAAMMDD-HHMMSS`) y la licencia se recupera de
    `vigente.lic`. Si no se puede leer (permisos), no se toca.
- **Huella del equipo** (`machine*.go`), L1-D10:
  - IOPlatformUUID con `/usr/sbin/ioreg` en macOS; MachineGuid en Windows, con `x/sys/windows/registry` y
    `WOW64_64KEY`, sin dependencias nuevas; `/etc/machine-id` en otros sistemas, solo para compilar;
  - solo se guarda `SHA-256("ADELPOS-EQUIPO-V1\n" + id)`;
  - tres intentos y falla cerrada.
- **Instalación copiada** (L1-D11, solo la detección): con otra huella, la licencia guardada se ignora sin borrarla.
  - El estado es «Sin activar con datos» con `Copied`, o «Sin activar» con `Copied` si la base no tiene operaciones,
    como dice 0.5.3.
  - `Accept` responde `ErrCopiedInstallation`. Confirmar y regenerar son de la Fase B.
- **«Sin activar con datos»** (`datapresence.go`):
  - **Qué cuenta como operación**: al menos una fila en `app.restaurant_accounts`, `app.cash_shifts`, `app.payments`,
    `app.inventory_movements` o `app.legacy_sales`. No cuentan la configuración, el catálogo, los usuarios, las
    terminales, las sesiones ni la auditoría.
  - **Cómo se consulta**: un `SELECT EXISTS … OR EXISTS …` en una transacción `READ ONLY`, con `lock_timeout` de 2 s y
    plazo de 5 s. Solo toma AccessShare y no espera los locks del negocio (lo prueba la integración con 7820148305 y la
    fila tomada).
  - Se consulta al arrancar y cada minuto mientras dé falso. Cuando da verdadero, ya no se repite, porque las
    operaciones no se borran.
- **Última hora confiable** (L1-D15):
  - `server_time` e `issued_at` se guardan de inmediato;
  - el reloj local (cada minuto, en `Run`) y las operaciones, en memoria; se guardan cada minuto y al apagar;
  - `max(audit_events.occurred_at)` solo la inicia cuando falta;
  - `ReplaceTrustedTime` es lo único que la baja, y solo con `internet_check` (§6.3) o `support_unlock` (§6.4).
- **Llave de licencias** (`localsetup/license_key.go`):
  - `identity/license-key.pem`, Ed25519 en PKCS#8, 0600;
  - se crea una sola vez: un temporal 0600 se enlaza con el nombre definitivo, y el enlace nunca reemplaza un archivo;
  - con `licencia/estado.json` o `vigente.lic` y sin llave, `EnsureLicenseKey` da error y no la crea;
  - una llave dañada no se reemplaza.
- **Arranque**:
  - `cmd/ad-el-pos-server/license.go` y `main.go:261`: después de la identidad, en `serveHTTP`, que comparten `serve`, el
    `host` de macOS y el de Windows;
  - el trabajador `License.Run` se apaga en orden (`main.go:381`);
  - sin identidad o sin llave, el servidor sigue atendiendo «Sin activar», con `IdentityUnavailable`, y no escribe
    nada en `identity/licencia`.
  - **Ayudas de prueba**, solo con `test_mode` y en desarrollo (L1-D16 y L1-D18): `ADEL_TEST_LICENSE_TRUST` (la ruta
    de un `vectors.json`) y `ADEL_TEST_CLOCK_FILE` (un desfase con formato de duración de Go, como `-36h`).
- **Documento**: `docs/licencias/ROTAR_LLAVES.md`.
- `scripts/database.py`: `./internal/licensing` en la tupla de integración.

### La interfaz para las Fases B, C y D (no cambia sin avisar)

`administration.Service.License *licensing.Manager` (`service.go:70`), siempre con valor en `serve` y en el `host`.

| Método                                                                                            | Qué hace                                                                                                                                                                                 |
| ------------------------------------------------------------------------------------------------- | ---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `Status() licensing.Status`                                                                       | El estado en memoria, sin base ni internet (`manager.go:392`)                                                                                                                            |
| `TrustedTime() time.Time`                                                                         | La última hora confiable; cero si no hay                                                                                                                                                 |
| `ObserveTime(t time.Time, fuente licensing.TimeSource)`                                           | Solo sube. Fuentes: `TimeFromServer`, `TimeFromLicense`, `TimeFromClock`, `TimeFromOperation` y `TimeFromAudit`                                                                          |
| `ReplaceTrustedTime(t time.Time, motivo licensing.ReplaceReason) (anterior time.Time, err error)` | Solo `ReplaceByInternetCheck` (§6.3) y `ReplaceBySupportUnlock` (§6.4). La baja, la guarda de inmediato y devuelve la anterior para la auditoría. Si no se puede guardar, no cambia nada |
| `Accept(jws string) (licensing.Status, error)`                                                    | §10 y guardado. Un rechazo es un `*licensing.VerifyError` con `Code`; además `ErrIdentityUnavailable`, `ErrCopiedInstallation`, `ErrMachineUnverified` y `ErrStateNotSaved`              |
| `LocalClock() time.Time`                                                                          | El reloj inyectado (`Options.Now`), para que la puerta y los cupos usen el mismo reloj simulado                                                                                          |
| `Load(ctx)`, `Run(ctx)`, `Flush()`                                                                | Arranque, trabajador de cada minuto y guardado pendiente                                                                                                                                 |

`licensing.Status` (`status.go:55`):

- **`State`**: `active` (Vigente), `grace` (Tolerancia), `expired` (Vencida), `revoked` (Revocada), `not_activated`
  (Sin activar) o `not_activated_with_data` (Sin activar con datos). Son los valores de `vectors.json` y de
  `details.state` de 0.6.2.
- **Indicadores**:
  - `Copied`;
  - `MachineUnverified`, la huella que no se pudo leer, con su propio mensaje;
  - `IdentityUnavailable`;
  - `RevocationReason` (`deactivated`, `transferred` o `revoked`).
- **El entorno**: `Environment` (`test` o `production`, el compilado).
- **El cliente y el plan**:
  - `LicenseID`, `ActivationID`, `LicenseeName`, `LicenseType`, `BillingPeriod` y `Revision`;
  - `Plan{PlanID, Name, Custom}`, `Features` (una copia);
  - `IssuedAt`, `PeriodStartsAt` y `UpdatesUntil`.
- **`Limits{CashRegisters, OrderTakers, KitchenDisplays}`**, en `int`.
  - Sin licencia válida, los tres valen 0.
  - En una licencia Vencida o Revocada siguen siendo los de la licencia: la puerta (C) decide si se puede ocupar un
    cupo.
- **Fechas**: `ExpiresAt`, `GraceDays`, `GraceEndsAt` (exclusivo) y `Perpetual` (`ExpiresAt` y `GraceEndsAt` en nil).
- **`EffectiveNow`** y **`TrustedTime`**.
- **`HasLicense()`** es verdadero en Vigente, Tolerancia, Vencida y Revocada. Es «hay licencia válida» para el
  `LimitsSource` de la Fase D.

**Nombres libres para la Fase C**: `operations.go`, `clock.go` y `unlock.go` no existen. Para la solicitud de desbloqueo,
el campo nuevo de `storedState` va en `store.go`. `estado.json` se lee sin `DisallowUnknownFields`, así que agregar un
campo es compatible.

### Decisiones propuestas

1. **Los estados se nombran con los valores del contrato** (`active`, `grace`, `expired`, `revoked`) y con los de
   0.6.2 (`not_activated` y `not_activated_with_data`), no en español: son los de `vectors.json` y los de la API. El
   texto en español es de las pantallas.
2. **`estado.json` es el punto de confirmación y lleva el JWS. `vigente.lic` es su copia.** Por qué: un solo
   renombrado confirma todo, así que nunca hay un par a medias. Además, si uno de los dos archivos se altera, queda la
   otra copia válida. `vigente.lic` solo se acepta como primera revisión si `estado.json` faltaba o estaba dañado (la
   recuperación).
3. **La huella se registra la primera vez que arranca la 0.15.0**, no solo al aceptar una licencia. Así una carpeta
   copiada se detecta también antes de activar. `Accept` se niega con `Copied` (L1-D11, punto 2) y con
   `MachineUnverified`.
4. **Sin identidad o sin llave, el gestor no escribe nada en `identity/licencia`.** Si escribiera, una llave que falló
   al crearse quedaría como «estado sin llave» para siempre. «Estado» es `estado.json` o `vigente.lic`; una carpeta
   vacía no cuenta.
5. **Qué cuenta como operación y cómo se consulta**: lo de arriba. La venta importada del sistema anterior cuenta,
   porque se consulta en los reportes.
6. **La última hora confiable al crear la base**: la `0002` deja un registro de auditoría con la hora de la migración,
   así que una base nueva inicia la hora confiable con esa hora. Es correcto, porque ya pasó.
7. **Un `estado.json` ilegible por permisos no se aparta.** El gestor sigue en memoria y no escribe nada. Solo se aparta
   un contenido inválido.
8. **Sin ninguna licencia válida al arrancar, la guardada no se borra**: queda para soporte. Importar otra vez el `.lic`
   válido la vuelve a instalar.
9. **Otra licencia válida para esta instalación** (otro `license_id`) reemplaza a la vigente. La revisión más alta de
   la anterior se conserva. El contrato no lo prohíbe.
10. **La prueba de la compilación de producción** también compila con un overlay de llaves simuladas, fuera de la
    carpeta. Así se demuestra que lo único que falta son las llaves. Esa prueba se retira en el paso a producción.
11. **El formato de las ayudas de prueba**:
    - `ADEL_TEST_LICENSE_TRUST` es la ruta de un `vectors.json`; solo se agregan las llaves del entorno `test`;
    - `ADEL_TEST_CLOCK_FILE` contiene una duración de Go y se vuelve a leer cada segundo;
    - las dos solo funcionan con `test_mode` y en desarrollo; en producción, `WithTestKeys` y `OffsetClock` fallan.

**Para la Fase B** (no es una decisión de la A):

- Al regenerar la identidad, conviene copiar la última hora confiable al `estado.json` nuevo. Si no, regenerar la
  identidad reiniciaría la protección de la hora.
- Solo debe llegar a `ObserveTime(TimeFromServer)` el `server_time` de una respuesta correcta.

**Diferencia con la especificación**: la §1.1 dice «Sin activar con datos» para una copia, y 0.5.3 (L1-D11) agrega «o Sin
activar si la base no tiene operaciones». Se siguió L1-D11. El contrato no lo trata (§15).

### Archivos

- **`backend/internal/licensing/`**:
  - código: `licensing.go`, `status.go`, `verify.go`, `protocol.go`, `state.go`, `trust.go`, `trust_development.go`,
    `trust_production.go`, `llaves-test.json` (la copia), `store.go`, `machine.go`, `machine_darwin.go`,
    `machine_windows.go`, `machine_other.go`, `datapresence.go`, `manager.go` y `testclock.go`;
  - pruebas: `vectors_test.go`, `manager_test.go`, `store_unix_test.go`, `trust_test.go`, `machine*_test.go`,
    `production_build_test.go`, `testclock_test.go`, `licensing_test.go`, `datapresence_integration_test.go` y
    `private_{unix,windows}_test.go`.
- **`backend/internal/localsetup/`**: `license_key.go` y `license_key_test.go`.
- **`backend/cmd/ad-el-pos-server/`**: `license.go`, `license_test.go` y `main.go` (`:261`, `:376-391`).
- **`backend/internal/administration/service.go:66-70`**: el campo `License`, en un bloque de la Fase A.
- **`config/licencias/llaves-test.json`** y `scripts/sync-metadata.mjs`.
- **`scripts/database.py`**: la tupla de paquetes.
- **`docs/licencias/ROTAR_LLAVES.md`**.

### Pruebas (realmente ejecutadas)

| Prueba                                                                                | Resultado                                                                                                                                                                                                                                             |
| ------------------------------------------------------------------------------------- | ----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `TestContractVectors`: los **67 casos** de `vectors.json` (20 + 17 + 10 + 13 + 4 + 3) | Aprobada: 67 subpruebas de caso. Además, la prueba de posesión que firma AD-EL POS es idéntica a la publicada, y un `.licreq` con la firma de otro contenido no verifica                                                                              |
| El almacén: persistencia, corte a la mitad, permisos y revisión                       | Aprobadas: temporales descartados, corte entre los dos archivos, `estado.json` a medias recuperado de `vigente.lic`, 0700/0600, revisión menor o igual con otros bytes rechazada (también en el almacén y tras reiniciar) y estado ilegible sin tocar |
| Alterado, de otra llave o de otra instalación                                         | Aprobada: al aceptarlo y en el disco (`vigente.lic` y `estado.json`), queda la última válida, y sin ninguna válida, «Sin activar» sin borrar nada                                                                                                     |
| La identidad                                                                          | Aprobadas: una 0.14.0 crea la llave una vez y no toca la identidad; un estado sin llave da error y no la crea; una llave dañada no se reemplaza                                                                                                       |
| La huella                                                                             | Aprobadas: otra huella con datos → «Sin activar con datos» con `Copied` (sin datos, «Sin activar»); tres intentos y falla cerrada; lectura real de `ioreg` en esta Mac, estable y sin mostrar el valor                                                |
| Estados en el gestor, la hora confiable y «Sin activar con datos»                     | Aprobadas: los límites exactos, el reloj atrasado, `ObserveTime`, `ReplaceTrustedTime`, `Run` y la consulta cada minuto                                                                                                                               |
| `TestProductionBuildFailsWithoutProductionKeys`                                       | Aprobada: `go build -tags adelpos_production ./internal/licensing ./cmd/ad-el-pos-server` falla por `llaves-production.json`, y con el overlay compila                                                                                                |
| Arranque (`TestLoadLicenseAtStartup` y `TestLicenseTestHooksRequireTestMode`)         | Aprobadas                                                                                                                                                                                                                                             |
| `ISO_TAG=LA iso.sh … go-unit`                                                         | **Aprobado**, en las dos corridas (vet normal e `integration`, `test -race` y `GOOS=windows` build y vet)                                                                                                                                             |
| `go-int ./internal/licensing`                                                         | **Estado 0**: la consulta de operaciones y de la última actividad contra la base 55499                                                                                                                                                                |
| `go-int ./cmd/ad-el-pos-server`, con el candado                                       | **Estado 0**, en dos corridas (70 s y 64 s); el candado se liberó                                                                                                                                                                                     |
| `format`, `fmt-check` y `sync-check`                                                  | Aprobados; MD5 de `docs/etapas/`, del contrato, de la guía y de `contracts/` iguales antes y después                                                                                                                                                  |

### Lo no verificado

- **La lectura real de MachineGuid y las ACL de Windows**: solo se compilan (`GOOS=windows` vet). Se ejecutan en la
  Fase W.
- **La compilación de producción con llaves reales**: no existen.
- **El `host` de Windows**: no se ejecutó; comparte `serveHTTP` con el de macOS.
- **La revisión independiente R1** del verificador y del almacén: pendiente, según el plan.

### Commits (rama `etapa-l1-a`)

- `fd31224` etapa L1 fase A: interfaz del gestor de licencias (Status, cupos, hora confiable)
- `3939268` etapa L1 fase A: verificador de la §10, estado temporal y llaves del entorno compilado
- `3bf7977` etapa L1 fase A: almacén identity/licencia, huella del equipo y gestor de la licencia
- `d442503` etapa L1 fase A: llave Ed25519 de licencias de la instalación (identity/license-key.pem)
- `6f93547` etapa L1 fase A: carga del gestor de la licencia al arrancar (serve y host de macOS y Windows)
- `90ce38b` etapa L1 fase A: docs/licencias/ROTAR_LLAVES.md (§3 de la guía de integración)
- `6c48592` etapa L1 fase A: el almacén no reemplaza un estado ilegible ni borra la licencia ignorada

### Decisiones del coordinador sobre la Fase A

Fusionada en `27cb24c`.

- **L1-D25 · Se aceptan las diez decisiones propuestas del agente.** Las principales:
  - los estados con los valores del contrato;
  - `estado.json` como punto de confirmación;
  - la huella registrada en el primer arranque de la 0.15.0;
  - qué cuenta como operación: cuentas, turnos, pagos, movimientos de inventario o ventas importadas;
  - las ayudas `ADEL_TEST_LICENSE_TRUST` y `ADEL_TEST_CLOCK_FILE`, solo con `test_mode` y en desarrollo.
- **L1-D26 · Una copia sin operaciones queda «Sin activar», con `Copied`**, y no «Sin activar con datos».
  - Manda el contrato (§6: «Sin activar» es la base sin operaciones).
  - El mensaje de la §1.1 («Esta instalación se copió de otra computadora…») se muestra igual, por `Copied`.
- **Para la Fase B**:
  - al regenerar la identidad se conserva la última hora confiable;
  - solo cuenta como hora confiable el `server_time` de una respuesta correcta.

---

## Fase D · Cupos de la licencia

Árbol `$SP/wt/l1-d`, rama `etapa-l1-d` desde `etapa-l1` (`1ceee93`). `ISO_TAG=LD`. Sin emuladores.

Commits (sin líneas de atribución):

- `f202543` etapa L1 fase D: migración 0028 (sección «Fase D»: app.license_seats y la caja de los turnos activos),
  versión 0.15.0 y esquema 28
- `20019dc` etapa L1 fase D: paquete licenseseats (lock 7820148311, ForOperation y asignación atómica)
- `e816750` etapa L1 fase D: los cupos en dispatch, modo Cocina, «Liberar cupo», Terminales y sessions-summary, con el
  contrato v1
- `bc4db5b` etapa L1 fase D: CLAUDE.md §3 con el lock 7820148311 y API_CONTRACTS.md con las rutas de los cupos

### Qué se hizo

- **Paquete nuevo `backend/internal/licenseseats/`** (pgx), con el lock nuevo **7820148311** (`licenseseats.LockKey`).
  - `seats.go`: los tres cupos (`cash_registers`, `order_takers`, `kitchen_displays`), `Limits`, la interfaz
    `LimitsSource`, `Options`, `Manager`, `UnavailableError` con su mensaje y `ErrNoValidLicense`.
  - `operations.go`: **la única clasificación**, `ForOperation(método, ruta) Effect`.
  - `store.go`: la definición SQL de «en uso» (una sola, `inUseCondition`), la asignación atómica (`claim`) y las
    operaciones del gestor (`Capture`, `OpenShift`, `Renew`, `SetKitchenDisplay`, `Sweep`, `ReleaseDevice`,
    `DeviceSeats`, `Usage`).
  - `seatstest/`: la fuente de cupos y el reloj simulados de las pruebas. Solo lo importan archivos `_test.go`, y su
    fuente no entrega ningún cupo fuera de una prueba (`testing.Testing()`).
- **`0028_licencias.sql`, sección «Fase D»** (el archivo no existía en la rama: lleva solo esta sección y el encabezado
  general). `app.license_seats`:
  - una fila por terminal y tipo (`PRIMARY KEY (device_id, kind)`), `kind` `cash_register`, `order_taker` o
    `kitchen_display`;
  - `shift_id` (la caja, ligada al turno, L1-D3) y `session_id` (la cocina, ligada a la sesión que entró), con un
    `CHECK` que exige a cada tipo su vínculo y solo el suyo;
  - `acquired_at` y `last_activity_at` (hora de Go);
  - llaves foráneas con `ON DELETE CASCADE` a `devices`, `cash_shifts` y `sessions`, e índices parciales por
    `shift_id` y `session_id`;
  - al actualizar con turnos abiertos o en arqueo, la terminal que abrió cada uno ya ocupa su caja (ver Decisión
    propuesta 6).
- **Versión 0.15.0 y esquema 28** en `config/product.json`, con `./scripts/dev.sh sync` en el árbol y los dos
  `lockfile` (`desktop/package-lock.json`, `desktop/src-tauri/Cargo.lock`), como en la subida de la A3.
  `min_client_versions` sin cambio.
- **Un solo punto en `administration.Service.dispatch`** (`administration/service.go:366` para la credencial de envío
  y `:390` para la sesión): `withSeats` (`administration/license_seats.go:45`), después de autenticar y en la misma
  transacción, envuelve a `runProtected`.
- **Rutas**:
  - `POST kitchen/display-mode {active}` (`license_seats.go:127`, enrutada en `routes.go:22`).
  - `POST devices/{id}/seat/release` (`license_seats.go:158`, enrutada en `devices.go:146`), con `devices.manage` y la
    auditoría `license.seat_released`.
  - `GET devices` y `GET devices/{id}/configuration`: `seats` por terminal (`devices.go:49`,
    `licenseseats.DeviceSeatsColumn`).
  - `GET devices/sessions-summary`: conserva `by_kind` y `total`, agrega `seats` (`in_use` y `limit` por cupo) y
    `limited: true` (`devices.go:332`, L1-D22).
- **`main.go`**: `Seats: licenseseats.NewManager(licenseseats.Options{Limits: nil})` en `newApplication`
  (`cmd/ad-el-pos-server/main.go:222-240`), con el comentario de cómo se conecta. **Falla cerrada**: sin fuente no hay
  licencia válida y ninguna operación que pide cupo procede.
- **Contrato v1**: `openapi.json` (esquema `LicenseSeatUnavailable`, 409 en `openAccount`, `saveDraft`,
  `confirmDraft`, `editUnits` y `submitOfflineOrder`, la operación nueva `setKitchenDisplayMode` con la etiqueta
  «Cocina» y la descripción del error genérico), `docs/API_V1.md` (sección «Cupos de la licencia (etapa L1, Fase D)»,
  la fila de la tabla de operaciones y la del código de error) y `TestContractV1` (`contractSeats`).
- **Documentos**: `CLAUDE.md` §3 (fila 7820148311 y el orden de adquisición) y `API_CONTRACTS.md` (sección «Etapa L1 ·
  cupos de la licencia»: `GET devices`, `sessions-summary` y la ruta de liberar).
- **Pruebas**: `licenseseats` (unitarias y de integración, con la concurrencia), `database` (`TestStageL1Upgrade`),
  `operations` (`TestStageL1LicenseSeats`, por la API real) y `apicontract`. El paquete nuevo va en la tupla
  `packages` de `scripts/database.py`.
- **Los 17 `administration.Service{…}` de las pruebas** de `administration`, `operations`, `apicontract`, `catalog`,
  `designer`, `identity`, `reports` y `reports/operational` reciben `Seats: seatstest.Roomy()` (999 de cada cupo, el
  máximo del contrato), o el mismo gestor en los «reinicios» de `operations`.

### La interfaz `LimitsSource` y cómo se conecta

```go
// internal/licenseseats/seats.go
type Limits struct {
	CashRegisters   int
	OrderTakers     int
	KitchenDisplays int
}

// SeatLimits devuelve los cupos y true, o false si no hay ninguna licencia válida («Sin activar» o «Sin activar con
// datos»). Sin licencia válida no se ocupa ningún cupo: nunca hay un valor «sin límite».
type LimitsSource interface {
	SeatLimits() (Limits, bool)
}

type LimitsFunc func() (Limits, bool) // adaptador, como http.HandlerFunc

type Options struct {
	Limits         LimitsSource     // sin valor: falla cerrada
	Now            func() time.Time // sin valor: time.Now; la Fase C le pasa licensing.Manager.LocalClock (L1-D16)
	OrderTakerIdle time.Duration    // solo las pruebas; sin valor, licenseseats.OrderTakerIdle (15 min)
}
```

Para conectarlo (Fase C), en cuanto el gestor de licencias esté cargado y antes de atender peticiones (está escrito en
el comentario de `main.go`):

```go
application.Seats = licenseseats.NewManager(licenseseats.Options{
	Limits: licenseseats.LimitsFunc(func() (licenseseats.Limits, bool) {
		status := manager.Status()
		valid := status.State != licensing.StateNotActivated && status.State != licensing.StateNotActivatedWithData
		return licenseseats.Limits{CashRegisters: status.Limits.CashRegisters,
			OrderTakers: status.Limits.OrderTakers, KitchenDisplays: status.Limits.KitchenDisplays}, valid
	}),
	Now: manager.LocalClock,
})
```

`licensing.Status` y sus `Limits` son los que publicó la Fase A en su rama (`fd31224`). Con una licencia vencida o
revocada los cupos siguen siendo los de esa licencia: la puerta de la Fase C decide qué operación se permite en cada
estado.

### La clasificación: `licenseseats.ForOperation(método, ruta) Effect`

Ruta relativa a `/api/v1`, con o sin el prefijo y con los identificadores reales (`operations.go:80`):

| Efecto            | Operaciones                                                                                                                 | Qué hace                                                                                                                                                                                             |
| ----------------- | --------------------------------------------------------------------------------------------------------------------------- | ---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `capture`         | `POST restaurant/accounts`, `PUT …/{id}/draft`, `POST …/{id}/confirm`, `POST …/{id}/edit`, `POST restaurant/offline-orders` | Con turno activo en la caja de la terminal: ocupa su cupo de caja (L1-D3) y borra su comandero. Si no: ocupa el comandero o renueva su actividad. Sin cupo, 409 y la transacción se revierte entera. |
| `preview`         | `POST …/{id}/draft-preview`, `POST …/{id}/edit-preview`                                                                     | Renueva la actividad del comandero si la terminal lo tiene. Nunca ocupa ni rechaza.                                                                                                                  |
| `open_shift`      | `POST payments/shifts`                                                                                                      | Ocupa la caja del turno nuevo (sin cupo, el turno no se abre) y borra los comanderos de las terminales de esa caja.                                                                                  |
| `close_shift`     | `POST payments/shifts/{id}/closing/confirm`                                                                                 | Libera las cajas de todas las terminales del turno. No escribe: la fila cuenta solo con el turno activo.                                                                                             |
| `kitchen_display` | `POST kitchen/display-mode`                                                                                                 | Ocupa o libera el cupo de cocina (lo hace la ruta).                                                                                                                                                  |
| `session_end`     | `POST auth/logout`, `POST auth/password`, `POST sessions/{id}/revoke`, `POST devices/{id}/revoke`                           | Después de la operación borra las filas que ya no cuentan (`Sweep`).                                                                                                                                 |
| `device_release`  | `POST devices/{id}/seat/release`                                                                                            | Lo hace la ruta.                                                                                                                                                                                     |
| `none`            | Todo lo demás: lecturas, cobros, efectivo, cancelar, reorganizar, cocina, catálogo, inventario, reportes, administración    | Nada.                                                                                                                                                                                                |

Cada efecto también dice qué cupos ocupa (`Occupies`), cuáles libera (`Releases`) y si renueva (`Renews`), para la
prueba de completitud de la Fase C.

### Qué cuenta como «en uso» (`store.go:22`)

Una sola condición SQL, con la hora de Go como parámetro (nunca `now()`, L1-D16):

- **caja**: el turno de la fila está `open` o `closing`;
- **comandero**: `last_activity_at` después del corte (hora − 15 min) **y** la terminal tiene alguna sesión vigente
  (`revoked_at IS NULL AND expires_at > hora`);
- **cocina**: la sesión que entró a Preparación sigue vigente.

### Asignación atómica (`store.go:62`, `claim`)

1. `pg_advisory_xact_lock(7820148311)` y **después** se lee la hora de Go.
2. Sin licencia válida: `ErrNoValidLicense`.
3. **Una sola sentencia** con la misma foto: un `DELETE` en un `WITH` borra las filas que ya no están en uso y el
   `SELECT` cuenta las que sí, de otras terminales, y si la terminal ya tiene la suya (en la caja, del mismo turno).
4. La tiene → renueva (`last_activity_at`; en cocina pasa a la sesión que entra). Así una licencia nueva con menos
   cupos no interrumpe a nadie.
5. Si no: `otras >= límite` → `UnavailableError{Seat, InUse: otras, Limit}`; si hay lugar, `INSERT … ON CONFLICT`.

Toda escritura de `app.license_seats` se hace con 7820148311 tomado. **Orden**: 7820148311 es el último lock de la
transacción. En `withSeats`, la captura y la apertura de turno corren primero el manejador (que toma 7820148305 y, si
hace falta, 7820148304) y después piden 7820148311; las previas, igual (con 7820148305 compartido); el modo Cocina, el
barrido de los cierres de sesión y «Liberar cupo» lo toman solos (la liberación cierra las sesiones antes de pedirlo).
Ninguna transacción que tiene 7820148311 espera otro lock consultivo.

### Qué responde `restaurant/offline-orders` sin cupo (para la Fase F)

```
HTTP/1.1 409
{"error": {
  "code": "LICENSE_SEAT_UNAVAILABLE",
  "message": "No hay cupo de comandero libre (1 de 1 en uso). Pide que se cierre la sesión de otra terminal.",
  "request_id": "…",
  "details": {"seat": "order_takers", "in_use": 1, "limit": 1}
}}
```

- Con límite 0 (plan básico), `message` es «La licencia no incluye comanderos (0 de 0): solo se capturan pedidos en una
  terminal con el turno de caja abierto.» y `details` `{"seat": "order_takers", "in_use": 0, "limit": 0}`.
- `details.seat` es `order_takers` en todo dispositivo android (no tienen caja física). Solo sería `cash_registers` si
  alguien asignara una caja con turno activo a ese dispositivo en Terminales: la Fase F debe leer `details.seat` y no
  suponerlo.
- **Orden**: primero las credenciales, después el propio pedido (`ORDER_REQUIRES_CONFIRMATION`, agotados, mesa…) y al
  final el cupo. Un pedido con cambios recibe primero su revisión; el cupo se pide cuando se aceptaría.
- **No se guarda nada**: ni la cuenta, ni el pedido, ni la `operation_id` en `restaurant_operations`, ni la auditoría
  `restaurant.offline_order.*`, ni eventos (la transacción se revierte entera). Lo comprueban `TestStageL1LicenseSeats`
  y `TestContractV1`.
- **Definitivo para ese envío**: el mismo cuerpo reenviado se rechaza igual mientras no haya cupo (se probó dos veces).
  Como la clave no quedó registrada, si el mesero lo reenvía a propósito con la misma `operation_id` y ya hay cupo,
  entra.
- **Android 0.3.0 ya lo trata como rechazo definitivo**: `ApiErrors.toAppError` lo vuelve `AppError.Rejected` (409 con
  código propio) y `OrderSender.otherRejection` → `SendOutcome.Rejected(message)`: se marca con el mensaje y no se
  reintenta (`android/core/data/…/waiter/OrderSender.kt:382-404`). La Fase F solo tiene que mostrarlo con su texto.
- **Lo que ya entró no se rechaza**: si la `operation_id` ya está registrada (la respuesta se perdió), Go responde su
  resultado guardado (o `OFFLINE_ORDER_ALREADY_ACCEPTED`) **sin pedir cupo**, aunque el del dispositivo haya vencido.
- Sin licencia válida (protección de respaldo): 409 `LICENSE_NOT_ACTIVATED`; la puerta de la Fase C responde antes con
  sus `details`.

### Decisiones propuestas

1. **El cupo se pide después del manejador, en la misma transacción, y 7820148311 es el último lock.** El plan de 0.5.7
   decía «toma 7820148305 y después 7820148311; el manejador vuelve a tomar 7820148305». Pedirlo antes del manejador
   dejaría 7820148311 entre 7820148305 y 7820148304 (el catálogo de la captura) en las capturas y después de 7820148304
   en las previas: dos órdenes distintos que habría que cuidar siempre. Después del manejador:
   - el orden es siempre 7820148305 → 7820148304 → 7820148308 → 7820148311, y quien tiene 7820148311 nunca espera otro
     lock consultivo (no hay interbloqueo posible entre ellos);
   - los errores propios de la operación se informan primero (turno ya abierto, mesa ocupada, cambios del pedido sin
     conexión);
   - el turno nuevo ya existe para ligarle la caja;
   - si no hay cupo, toda la transacción se revierte: nada queda guardado.
   - Sigue siendo atómico: contar y ocupar ocurren con 7820148311 tomado, y lo que decide el conteo (turnos) no cambia
     porque la operación tiene 7820148305.
2. **Una operación con `operation_id` ya registrada no pide cupo.** El manejador responde con lo guardado o con su
   conflicto y nunca escribe nada nuevo. Sin esto, la cola de Android que reintenta un pedido que sí entró recibiría
   un rechazo por cupo falso.
3. **La caja no se libera al revocar la terminal**: el cupo dura lo que dura el turno, como dice el contrato («si la
   terminal se cae, el turno sigue abierto y el cupo sigue ocupado») y la especificación («las cajas se liberan cerrando
   el turno»).
4. **El comandero cuenta solo con sesiones vigentes en la terminal, y nunca vuelve sin pedirlo.** La fila que dejó de
   contar se borra en el barrido de cada asignación (en la misma sentencia que cuenta) y después de cada cierre de
   sesión (`session_end`). Sin el barrido, una sesión nueva en la terminal la haría contar otra vez sin pasar por el
   límite.
5. **La cocina se liga a la sesión que entró a Preparación**: se libera al cerrar esa sesión aunque la terminal tenga
   otras. Si otra sesión de la misma terminal entra, el cupo pasa a ella.
6. **La migración ocupa la caja de cada turno activo al actualizar** (la terminal que lo abrió, con su hora de
   apertura). Si no, una instalación con turnos abiertos empezaría con 0 cajas en uso. Con menos cajas en la licencia,
   nadie se interrumpe.
7. **Mensajes con límite 0** (plan básico): «La licencia no incluye comanderos (0 de 0): solo se capturan pedidos en una
   terminal con el turno de caja abierto.», «La licencia no incluye pantallas de cocina (0 de 0): Preparación no está
   disponible.» y «La licencia no incluye cajas (0 de 0). Avisa al encargado.». «Pide que se cierre la sesión de otra
   terminal» no tiene sentido sin cupos. Con límite: «No hay cupo de comandero libre (3 de 3 en uso). Pide que se
   cierre la sesión de otra terminal.», «No hay cupo de caja libre (1 de 1 en uso). Se libera al cerrar un turno de
   caja.» y «No hay cupo de pantalla de cocina libre (1 de 1 en uso). Pide que otra terminal salga de Preparación.».
8. **Sin licencia válida, `409 LICENSE_NOT_ACTIVATED`** con el mensaje «La licencia del sistema no permite esta
   operación. Avisa al encargado.» y sin `details`: es la protección de respaldo; la puerta de la Fase C responde antes
   con `state` y `copied` (L1-D17). Salir de Preparación y las previas siguen funcionando.
9. **`POST devices/{id}/seat/release`**:
   - sin comandero ni cocina en uso, `409 LICENSE_SEAT_NOT_HELD` con `details.seats`, sin cerrar nada (si solo ocupa
     una caja, el mensaje dice que se libera al cerrar su turno);
   - cierra las sesiones con `revoke_reason` `seat_released` (la sesión responde `SESSION_REVOKED`);
   - **no** revoca las credenciales de envío de sus meseros: los pedidos de la cola de esa tableta piden cupo otra vez
     al llegar (y se rechazan si no hay);
   - responde `{released: [...], sessions_closed}`.
10. **Los 15 minutos son una constante del backend**, `licenseseats.OrderTakerIdle`, no un campo de `server.json`
    (`Options.OrderTakerIdle` solo lo usan las pruebas). Si el propietario quiere que cada instalación lo cambie, es un
    campo opcional nuevo.
11. **`POST kitchen/display-mode`**: pide `kitchen.states.read` (el mismo de `GET kitchen/board`), responde
    `{active}`, no lleva `operation_id` (repetirlo deja el mismo estado) y en un dispositivo android es 400 (sin
    Preparación; la sesión con PIN ya no tiene el permiso y responde 403).
12. **El tablero de cocina no exige el cupo.** El plan de 0.5.7 ponía «cocina» en la columna de cupo de `kitchen/board`,
    `kitchen/states` y `tickets/{id}/ack`; el encargo de esta fase solo lo pide en `display-mode`. Consecuencia: **un
    escritorio 0.14.0** (que no llama a `display-mode`) entra a Preparación sin ocupar el cupo de cocina. Exigirlo en
    `GET kitchen/board` rompería Preparación en los escritorios 0.14.0 y en las pantallas que la Fase E todavía no
    cambie. **El coordinador decide** (las versiones mínimas no cambian, L1-D22).
13. **Sin capacidad nueva**: el escritorio esconde lo de los cupos con `license-v1` (Fase B, L1-D22).
14. **`sessions-summary` sin licencia válida** informa `limit: 0` en los tres cupos.
15. **Un turno en arqueo de cierre (`closing`) sigue ocupando su caja** hasta confirmarlo; cancelar el arqueo no cambia
    nada.
16. **Las pruebas usan `seatstest.Roomy()`** (999, el máximo del contrato) salvo las de cupos. La fuente de
    `seatstest` no da cupos fuera de una prueba (`testing.Testing()`), así que ni por error llega a un instalador.
17. **Un rechazo por cupo no deja auditoría** (como cualquier 409 de la operación). Si el propietario quiere verlos en
    el Registro de movimientos, se agrega `license.seat_denied` con el patrón de `Fault.Rejection`.

### Lo que el coordinador debe saber al fusionar

- **Hasta conectar `LimitsSource`, el servidor no captura**: `main.go` crea el gestor sin fuente (falla cerrada). Una
  corrida de Playwright o de `android-e2e` sobre esta rama sola, o sobre A + B + D sin la conexión de la Fase C, falla
  en toda captura, apertura de turno y Preparación con `409 LICENSE_NOT_ACTIVATED`. Se comprobó con el binario real
  (prueba de humo, abajo).
- **`config/product.json`**: esta rama y la Fase B suben a 0.15.0 y esquema 28 (L1-D23 lo pide a «la primera fase que
  cree la `0028`»; las dos la crean). Al unir, una sola subida; los archivos generados por `sync` coinciden.
- **`0028_licencias.sql`**: esta rama crea el archivo con el encabezado general y la sección «Fase D». Al unir con la
  Fase B, su sección «Fase B» va antes de la «Fase D» y el encabezado queda uno.
- **`stage9h_upgrade_test.go`** excluye `license_seats` de su conteo de filas (como `ready_notices`). Si la Fase B
  agrega permisos, `stageA1Snapshot` necesitará excluir `license.manage` como excluye `waiter_devices.use`.
- La Fase C pone la puerta del estado y de la hora alrededor de `withSeats` (antes, en `dispatch`) y su prueba de
  completitud puede usar `licenseseats.ForOperation` y `Effects`.
- Archivos compartidos tocados en bloques propios con comentario «Etapa L1 (Fase D)»: `administration/service.go`
  (campo `Seats` y las dos llamadas a `withSeats`), `routes.go` (modo Cocina y el gestor para `devices` y
  `sessions-summary`), `devices.go`, `main.go`, `openapi.json`, `docs/API_V1.md`.

### Archivos

- `backend/internal/licenseseats/seats.go`, `operations.go`, `store.go`, `seatstest/seatstest.go`
- `backend/internal/licenseseats/operations_test.go`, `seats_integration_test.go`
- `backend/internal/administration/license_seats.go`, `service.go`, `routes.go`, `devices.go`
- `backend/cmd/ad-el-pos-server/main.go`
- `backend/migrations/0028_licencias.sql`
- `backend/internal/database/stagel1_upgrade_test.go`, `stage9h_upgrade_test.go`
- `backend/internal/operations/stagel1_seats_test.go` y los fixtures con `Seats`
- `backend/internal/administration/pairing_acceptance_test.go`: `stage9g_sesiones_activas_por_tipo` esperaba
  `limited: false`; ahora espera `limited: true` y los tres `seats` (L1-D22), con el mismo `by_kind`
- `backend/internal/apicontract/openapi.json`, `contract_integration_test.go`
- `config/product.json` y los generados por `sync`; `scripts/database.py`
- `docs/API_V1.md`, `API_CONTRACTS.md`, `CLAUDE.md`

### Pruebas

Todo con `ISO_TAG=LD` en el clúster temporal 55499. Lo de abajo es lo que **realmente** se ejecutó, en el árbol con
los cambios (las corridas completas, sobre el estado final del código salvo donde se dice).

| Prueba                                                                                                    | Resultado                                                                                                                                                               |
| --------------------------------------------------------------------------------------------------------- | ----------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `go test ./internal/licenseseats/...` (unitarias: `ForOperation`, efectos, mensajes, valores por omisión) | `ok`                                                                                                                                                                    |
| `go-int ./internal/licenseseats` (`TestSeatAssignmentIsAtomic`, `TestSeatRules`)                          | `ok` (12 s). Concurrencia: 12 vueltas × 3 cupos × 8 transacciones en conexiones distintas por el último cupo; en cada vuelta una sola lo obtiene                        |
| La misma prueba de concurrencia **sin** el `pg_advisory_xact_lock` (cambio temporal, revertido)           | **Falla** en la vuelta 2: «8 terminales obtuvieron el último cupo». La prueba detecta la falta del lock                                                                 |
| `go-int ./internal/database` (todas las subidas, con `TestStageL1Upgrade` y `TestStage9HUpgrade`)         | `ok` (15 s)                                                                                                                                                             |
| `go-int ./internal/operations` completo (con `TestStageL1LicenseSeats`)                                   | `ok` (543 s). `TestStageL1LicenseSeats` sola: `ok` (82 s)                                                                                                               |
| `go-int ./internal/administration` completo                                                               | Primera corrida: falló `stage9g_sesiones_activas_por_tipo` (esperaba `limited: false`). Se cambió la prueba a `limited: true` y `seats` (L1-D22). Segunda: `ok` (260 s) |
| `go-int ./internal/apicontract` (`TestContractV1` con `contractSeats`) y sus unitarias                    | `ok` (43 s)                                                                                                                                                             |
| `go-int ./internal/catalog`, `./internal/identity`, `./internal/designer`                                 | `ok` (24 s, 22 s, 25 s)                                                                                                                                                 |
| `go-int ./internal/reports`, `./internal/reports/operational`                                             | `ok` (145 s, 47 s)                                                                                                                                                      |
| `go-int ./cmd/ad-el-pos-server`, con el candado `$SP/l1/candado-cmd`                                      | `ok` (65 s); candado soltado                                                                                                                                            |
| `go-unit` (vet normal e integración, `test -race`, compilación y vet para Windows)                        | `ok` (36 s), antes del último cambio, que solo tocó una prueba de integración (después, `go vet -tags=integration ./internal/administration` limpio)                    |
| `format`, `fmt-check`, `sync-check`                                                                       | Aprobados; `fmt-check` otra vez antes de cada commit                                                                                                                    |
| Prueba de humo con el binario real (Go temporal 8203/8473, `server-up` y `server-down`)                   | Sin fuente de límites: abrir cuenta, abrir turno y entrar a Preparación → `409 LICENSE_NOT_ACTIVATED`; salir de Preparación 200; `sessions-summary` con límites 0       |
| `md5` de `docs/etapas/*.md`, `docs/licencias/CONTRATO*`, la guía y `contracts/`, antes y después          | Iguales                                                                                                                                                                 |

`TestStageL1LicenseSeats` (`operations`, por la API real) cubre: el plan básico (1, 0 y 0) con la caja que captura, la
otra terminal de la misma caja rechazada por caja, el escritorio y la tableta rechazados por comandero, cobrar y
registrar efectivo sin cupo y Preparación rechazada; el pedido sin conexión rechazado dos veces sin dejar rastro; los
15 minutos con la previa que renueva; el reintento de una captura que ya entró; la tableta con tres PIN (un cupo) y
«Selecciona tu usuario»; «Liberar cupo» (respuesta, auditoría, sesiones cerradas, 409 sin cupo o con caja, 403 sin
permiso); la cocina (entrar, otro rechazado, salir, cerrar sesión, 403 en la tableta, 400 sin `active`); cerrar sesión
el último usuario sin que la sesión siguiente lo recupere; abrir turno libera el comandero; la caja compartida (dos
cupos) y el cierre del turno desde la computadora principal que libera los dos; la licencia nueva con menos cupos
(nadie se interrumpe; ni capturas nuevas ni un turno nuevo); y tres vueltas de cuatro capturas simultáneas por el
último cupo.

### No verificado

- **El escritorio y Android** (Fases E y F): ningún `vitest`, `cargo`, Playwright ni emulador. Las pruebas de Vitest
  del escritorio simulan `sessions-summary` con `limited: false`; no cambian.
- **`go-int ./internal/payments`**: no se tocó el paquete (cobrar no pide cupo y eso se probó por la API en
  `operations`).
- **La conexión real con el gestor de licencias** (Fase C) y la puerta del estado y de la hora alrededor de los cupos.
- **La revisión independiente R3** de la asignación atómica: pendiente; sus correcciones vuelven a esta fase.
- **Carga**: el lock 7820148311 se suma a cada captura y previa (después de 7820148305, que ya serializa las escrituras);
  no se midió con varias terminales reales.

### Decisiones del coordinador sobre la Fase D

Fusionada en `f4b42a3`. Los conflictos con la Fase A fueron solo de renglones vecinos: la tupla de `database.py` y dos
campos de `Service`. `go-unit` pasó sobre el árbol fusionado.

- **L1-D27 · Se aceptan las doce decisiones propuestas del agente.**
  - El cupo se pide después del manejador, y 7820148311 es siempre el último lock (05 → 04 → 08 → 11): ajusta el orden
    de 0.5.7.
  - Una `operation_id` ya registrada no pide cupo.
  - El cupo de caja dura lo que dura el turno.
  - El comandero solo cuenta con una sesión vigente.
  - El cupo de cocina va ligado a la sesión que entró a Preparación.
  - La migración da su cupo de caja a la terminal que abrió cada turno activo.
  - Los mensajes son distintos con límite 0.
  - `LICENSE_SEAT_NOT_HELD`.
  - Los 15 minutos son una constante del backend.
  - `display-mode` pide `kitchen.states.read`.
  - No hay capacidad nueva, se usa `license-v1`.
- **L1-D28 · El cupo de cocina sigue siendo explícito** (`POST kitchen/display-mode`). No se pide en `GET kitchen/board`,
  porque esa lectura también la usa la vista de una cuenta (`AccountKitchen.tsx`) y ocuparía un cupo de cocina con solo
  ver una cuenta.
  - **Limitación**: un escritorio 0.14.0 conectado a un servidor 0.15.0 entra a Preparación sin ocupar cupo de cocina.
    Las versiones mínimas no cambian (L1-D22), así que cada terminal debe actualizarse a la 0.15.0 para que se respete.
    Va en `docs/licencias/INTEGRACION.md`.
- **La conexión con el gestor de licencias** (`LimitsSource` con `licensing.Manager.Status()` y `LocalClock`) la hace la
  Fase C. Hasta entonces, `etapa-l1` rechaza toda captura, toda apertura de turno y la entrada a Preparación (falla
  cerrada, como se pidió): ninguna prueba de interfaz corre sobre `etapa-l1` antes de esa conexión.

---

## Revisión R1 · Verificación de licencias (§10 del contrato) y su almacén

La hizo el revisor independiente R1 en `$SP/wt/l1-r1`, rama `etapa-l1-r1`, sobre `27cb24c` (la Fase A fusionada). Se
revisó `git diff 1ceee93..27cb24c`: `backend/internal/licensing/`, `backend/internal/localsetup/license_key.go`, el
arranque (`cmd/ad-el-pos-server/license.go` y `main.go`) y `scripts/sync-metadata.mjs` con `config/licencias/`.

- `ISO_TAG=R1`, sin Go temporal y sin emuladores.
- Ninguna llamada a `licensing.sicrat.com`. No se leyeron los códigos de licencia de prueba ni se imprimió ninguna
  llave privada.
- No se cambió código de producción: solo pruebas nuevas.

### Hallazgos

Las pruebas «…Defect…» **fallan hasta que se corrija su hallazgo**. Por eso `go-unit` falla hoy en
`./internal/licensing`, solo por esas tres pruebas.

| N.º      | Gravedad | Dónde                                                                                                                                                                                      | Escenario (entrada → resultado incorrecto)                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                               | Prueba (commit)                                                                                                                                                   | Corrección propuesta                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                  |
| -------- | -------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------ | -------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | ----------------------------------------------------------------------------------------------------------------------------------------------------------------- | --------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| **R1-2** | **Alta** | `manager.go:232` (`checkMachine`), `manager.go:311` (recuperación), `store.go:207` y `store.go:226`                                                                                        | Se copia la carpeta de datos a otra computadora y se **borra o daña `estado.json`**, o se le quita `machine_fingerprint` → la huella nueva se registra como si fuera la primera vez y `vigente.lic` se recupera → la copia queda **Vigente** y sin `Copied`. Tiene la misma identidad y la misma llave, así que también funciona **en línea**: el servidor de licencias ve una sola instalación en dos computadoras y nunca lo puede corregir. La §1.1 de la especificación se anula borrando un archivo de una carpeta que es de la cuenta del servicio, sin `sudo`.                                                                                                                                                                                                                                                                                                                                    | `TestReviewR1DefectCopiedFolderWithoutStateIsNotDetected`, `review_r1_defects_test.go` (`419b0d8`, `82f6a4a`): borrado, dañado y sin huella                       | 1. La huella no debe depender solo de `estado.json`: hay que guardarla junto con la llave de licencias, en el mismo paso en que se crea (por ejemplo, un registro aparte o un encabezado PEM). 2. Si la llave existe y falta la huella o no es válida, se falla cerrado (`Copied` o `MachineUnverified`), nunca se registra una nueva. 3. El registro debería ser un MAC con la llave de licencias, no un SHA-256 con un prefijo público, para que no se pueda calcular a mano.                                                                                       |
| **R1-1** | Media    | `manager.go:167-172` (Load deja `persistent=false` y un estado vacío), `manager.go:544-550` (Accept prepara la carpeta y escribe)                                                          | Un `estado.json` que la cuenta del servicio no puede leer → `Load` no lo toca (Decisión propuesta 7 de la A). El primer `Accept` lo **reemplaza** con un estado vacío más la licencia recibida. En la prueba: después del cierre `deactivated` (r2), `Accept` de la r1 activa → **aceptada**. Además, `accepted_revisions` queda en r1 y la última hora confiable baja del 10/01/2027 al `issued_at` (16/11/2026). Al reiniciar, la licencia liberada está Vigente. Puede pasar sin mala intención: en macOS el servicio corre con la cuenta del usuario, así que basta un `estado.json` que dejó un proceso con `sudo`; en Windows, un antivirus con el archivo abierto. La consulta automática de la Fase B llamará a `Accept` al arrancar. Lo mismo pasa si `ensureDirectory` falla en `Load` (el estado nunca se lee) o si un estado dañado no se pudo apartar (esto se comprobó leyendo el código). | `TestReviewR1DefectAcceptOverwritesAnUnreadableState`, `review_r1_defects_unix_test.go` (`5ef1ab8`, `82f6a4a`)                                                    | 1. Recordar si el estado se leyó: leído, faltante o dañado ya apartado. 2. Si no se leyó, `Accept` lo vuelve a leer bajo el mutex. Si sigue ilegible, responde `ErrStateNotSaved` sin escribir nada. 3. `persist` no debe escribir un estado que nunca se leyó.                                                                                                                                                                                                                                                                                                       |
| **R1-3** | Media    | `store.go:203-227` (faltante o dañado → estado nuevo, sin historial), `manager.go:311` (`vigente.lic` como «primera revisión»), `manager.go:525` (el historial sale solo de `estado.json`) | Se aceptan la r1 activa y el cierre r2, y se deja en `vigente.lic` el `.lic` de antes de liberar. Después, una de tres: se **borra**, se **daña** o se **edita** `estado.json` (se baja la revisión y su `sha256`, que se calcula con `shasum`). Resultado: al arrancar vuelve la **r1 Vigente**, y además importar a mano el `.lic` anterior se acepta (§10.4). Funciona mientras la computadora no consulte en línea, porque `refresh` devuelve otra vez el cierre. Es parecido a la limitación aceptada de la §9.2, pero se puede hacer después de recibir el cierre, y se deshace también una revocación del propietario. El mismo efecto, sin mala intención: un `estado.json` de un formato futuro (al bajar de versión) se aparta como dañado y se pierde el historial.                                                                                                                           | `TestReviewR1DefectDeletedStateBringsBackAnOldRevision`, `review_r1_defects_test.go` (`419b0d8`, `82f6a4a`): borrado, dañado y editado, al arrancar y al importar | 1. Guardar una segunda copia de la revisión más alta de cada `license_id` (revisión y `sha256`) fuera de `estado.json`. Propuesta: en la base, con el máximo de las dos copias. La base viaja en los respaldos, pero restaurar uno solo bajaría su copia, no la de `identity/`, y borrar `estado.json` no borraría la de la base. 2. Opcional: un MAC del estado con la llave de licencias, para que una edición a mano se trate como daño. 3. Que «dañado» no reinicie el historial si la otra copia lo tiene. **Decisión del coordinador**: toca L1-D9 y la `0028`. |
| R1-4     | Baja     | `scripts/sync-metadata.mjs:31-37`                                                                                                                                                          | `sync-check` solo recorre `config/licencias/`. Una copia **huérfana**, por ejemplo `backend/internal/licensing/llaves-production.json` sin su fuente, pasa `sync-check` (que dice «Metadatos coherentes.») y hace compilar la versión de producción. Pasa lo mismo si se borra la fuente y queda la copia. Se comprobó a mano en el árbol y se borró el archivo; no hay commit.                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                          | Reproducción manual, sin commit                                                                                                                                   | En `--check`, listar también `backend/internal/licensing/llaves-*.json` y fallar si alguno no tiene su fuente idéntica.                                                                                                                                                                                                                                                                                                                                                                                                                                               |
| R1-5     | Baja     | `verify.go:227-233` (igual que la referencia)                                                                                                                                              | En el contenido firmado, una clave repetida se acepta y gana la última: dos `limits` dan el segundo, con 999. También se aceptan datos después del objeto (`{…} {"basura":true}`). En el encabezado no pasa, porque se compara exacto. Hoy no se puede explotar: solo firma el servidor de licencias, y `json_encode` de PHP no repite claves.                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                           | Reproducción manual en una prueba temporal, ya borrada                                                                                                            | Endurecimiento opcional: rechazar las claves repetidas y el contenido sobrante. Se aparta de la referencia, así que es mejor proponerlo para la V1.1. Lo decide el coordinador.                                                                                                                                                                                                                                                                                                                                                                                       |
| R1-6     | Baja     | `.gitignore`                                                                                                                                                                               | `identity/license-key.pem` y `identity/licencia/` solo quedan fuera de git dentro de `.local/`. No hay una regla de seguridad para otra carpeta de datos dentro del repositorio, como sí la hay para las llaves de firma de Android.                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                     | —                                                                                                                                                                 | Agregar `**/identity/license-key.pem` y `**/identity/licencia/`.                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                      |

### Observaciones (sin número, para decidir o para otras fases)

- **Decisión propuesta 9 de la A**: otra licencia válida para la instalación (otro `license_id`) reemplaza a la
  vigente, aunque sea un **cierre**. Si se importa por error el cierre de una licencia anterior, una instalación con
  licencia Vigente queda Revocada. El contrato lo permite («la última licencia válida aceptada»). Las Fases B y E
  deberían confirmar con el usuario antes de reemplazar la vigente por otra licencia.
- **Un `.lic` con salto de línea final** se rechaza (`malformed_jws`), igual que la referencia. La Fase B decide si
  quita los espacios del archivo importado. Si lo hace, el `sha256` de la regla «idéntica byte por byte» debe salir
  siempre del texto ya recortado.
- **Time Machine y el Asistente de Migración**: no hay ninguna exclusión (`tmutil`). Restaurar `identity/licencia`
  desde Time Machine en la misma Mac devuelve revisiones anteriores. Es la «instantánea antigua» de la §12 del
  contrato, y además restaura la base. El Asistente de Migración a otra Mac sí se detecta por la huella, mientras no
  ocurra R1-2.
- **El disco se escribe con el mutex tomado**: `Accept`, `ObserveTime(TimeFromServer)` y `Flush` cada minuto. En
  macOS, `Sync` es `F_FULLFSYNC`, así que `Status()` espera esa escritura. Que lo tenga en cuenta la puerta de la
  Fase C, que llama a `Status()` en cada operación.
- **Los casos de revisión de `vectors.json`** se recorren con el `Verifier`, igual que la referencia, no con el
  almacén. El almacén lo cubren `TestManagerRevisionNeverGoesBack` y estas pruebas.

### Lo que se comprobó y está bien

- **§10, paso por paso**: `verify.go` es la referencia sin cambios de lógica. El `diff` con
  `contracts/licencias/v1/verify/license.go` solo cambia los cupos a `int`, separa `checkRevision` y quita `describe`.
  - **Encabezado**: se compara exacto, byte por byte.
  - **`kid`**: debe ser del entorno compilado. Aunque se cuele una llave de otro entorno, `key.Environment` se comprueba
    otra vez.
  - **Firma Ed25519**: sobre `encabezado.contenido` tal como llegaron.
  - **base64url**: estricto y canónico; se vuelve a codificar y se compara.
  - **Contenido de la §4**: tipos, campos obligatorios, `schema_version` que empieza con `1.`, cupos de 0 a 999 solo
    como enteros JSON, instantes UTC con «Z» y de 0 a 9 decimales, y las reglas por tipo y estado.
  - **Destinatario y entorno**: se comparan con `server_id` y con la llave pública.
  - **Revisión**: debe ser mayor que la aceptada, o igual solo si su `sha256` es idéntico.
  - **Guardado**: temporal, `fsync` y renombrado, todo antes de cambiar la memoria. Si no se guarda, no se acepta.
- **Intentos de engaño rechazados** (`TestReviewR1HeaderAndEncodingAreExact`, `…SignatureIsNotMalleable` y
  `…ContentIsStrict`, `ca5db48`):
  - relleno, espacios, saltos de línea, alfabeto estándar, bits de relleno no canónicos, cuatro segmentos y todo en
    mayúsculas;
  - `typ` distinto, `alg` en mayúsculas, campo `crit`, `kid` o `alg` repetidos, otro orden, `kid` con el escape JSON de «-» (barra invertida, `u002d`)
    y `KID` en mayúsculas, todos con firma válida;
  - firma maleable `S + L`, también como «misma revisión»;
  - cupos de 1000, −1, `3.0`, `3e0` o `"3"`;
  - instantes con desfase, sin zona, con `z`, con 10 decimales, el 30 de febrero o el segundo 60;
  - `schema_version` `1`, `2.0` o `01.0`, y otros tipos y formatos incorrectos.
- **Los 67 casos de `vectors.json`** pasan, y ninguno se ajustó:
  - el SHA-256 de `vectors.json` es el de `ORIGEN.md` y el de la etiqueta `v1.0.0`;
  - `diff -r` contra el clon no muestra diferencias;
  - el verificador de referencia da «OK: 20 licencias, 17 casos negativos, 10 casos de revisión, 13 estados
    temporales, 4 pruebas de posesión y 3 solicitudes .licreq»;
  - la prueba de AD-EL POS lee los resultados esperados del archivo y además comprueba que `SignProof` dé la prueba
    publicada.
- **El entorno**:
  - `go build -tags adelpos_production ./cmd/ad-el-pos-server` falla por `llaves-production.json`, y
    `TestProductionBuildFailsWithoutProductionKeys` pasa;
  - en producción, `WithTestKeys` y `OffsetClock` fallan por la constante, y `DefaultServerURL` es `""`;
  - `licensing.sicrat.com` solo aparece en `trust_development.go`;
  - la compilación no confía en las llaves de `vectors.json`, y solo `test_mode` las agrega. Se comprobó por el
    resultado de la verificación: `unknown_kid` sin `test_mode` y `other_installation` con él
    (`TestReviewR1FixtureTrustOnlyWithTestMode` y `TestReviewR1FixtureKeysAreNotTrustedByTheBuild`);
  - `test_mode` exige una base `adel_pos_test_*`, y en el `host`, `RefuseRealDataDir`;
  - el formato de `config/licencias/llaves-test.json` es exactamente el de `SigningKeys::publicKeys()` del servidor
    de licencias.
- **El almacén**:
  - los permisos son 0700 y 0600;
  - los temporales se descartan;
  - si hay un corte entre `estado.json` y `vigente.lic`, manda `estado.json`;
  - un archivo alterado, de otra llave o de otra instalación se ignora;
  - `Copied` se detecta con `estado.json` intacto;
  - si la huella no se puede leer, se reintenta tres veces y falla cerrado.
- **La llave de licencias**:
  - se crea una sola vez y nunca reemplaza una existente;
  - un estado sin llave da error;
  - la identidad 0.14.0 no se toca;
  - `EnsureIdentity` no regenera una identidad cuando la carpeta tiene la llave o `licencia/`.
- **Los secretos**:
  - ningún registro ni error lleva la llave privada, el JWS completo ni el identificador del equipo;
  - `Options.PrivateKey` no se expone;
  - `identity/` no está en ningún respaldo, porque solo se respalda la base;
  - la Fase A no maneja el código comercial, así que su registro se revisa en la Fase B.

### Lo que no se pudo verificar

- **Windows**: la ACL de `fileacl` en `identity/licencia`, la lectura real de `MachineGuid`, el renombrado encima del
  archivo y el caso del antivirus de R1-1. Solo se comprobaron `GOOS=windows` build y vet.
- **R1-1 cuando falla `ensureDirectory` en `Load`**: se vio solo en el código. Como usuario sin `root` no se puede
  hacer fallar `chmod` en una carpeta propia.
- **La compilación de producción con llaves reales**: todavía no existen.
- **La Fase B**: la importación del `.lic`, las respuestas de la API y el código comercial.

### Pruebas ejecutadas

| Prueba                                                                                                                                                                                 | Resultado                                                                                                                                                                                                                                                                         |
| -------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | --------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Verificador de referencia (`go run -C contracts/licencias/v1/verify . ../fixtures/vectors.json`)                                                                                       | OK, los 67 casos                                                                                                                                                                                                                                                                  |
| `TestContractVectors -v`                                                                                                                                                               | 67 subpruebas de caso aprobadas                                                                                                                                                                                                                                                   |
| `TestReviewR1HeaderAndEncodingAreExact`, `…SignatureIsNotMalleable`, `…ContentIsStrict`, `…FixtureKeysAreNotTrustedByTheBuild` y `TestReviewR1FixtureTrustOnlyWithTestMode` (en `cmd`) | Aprobadas                                                                                                                                                                                                                                                                         |
| `TestReviewR1DefectAcceptOverwritesAnUnreadableState` (R1-1)                                                                                                                           | **Falla**, como se esperaba                                                                                                                                                                                                                                                       |
| `TestReviewR1DefectCopiedFolderWithoutStateIsNotDetected` (R1-2), las 3 subpruebas                                                                                                     | **Falla**, como se esperaba                                                                                                                                                                                                                                                       |
| `TestReviewR1DefectDeletedStateBringsBackAnOldRevision` (R1-3), las 3 subpruebas                                                                                                       | **Falla**, como se esperaba                                                                                                                                                                                                                                                       |
| `ISO_TAG=R1 iso.sh … go-unit`                                                                                                                                                          | vet normal e `integration`: aprobado. `test -race`: todos los paquetes aprobados salvo `./internal/licensing`, que falla solo por las tres pruebas de arriba. El arnés se detiene ahí, así que `GOOS=windows` build y vet (normal e `integration`) se corrieron aparte: aprobados |
| `ISO_TAG=R1 ISO_RUN=… go-int ./internal/licensing` (sin las pruebas de los hallazgos)                                                                                                  | Estado 0                                                                                                                                                                                                                                                                          |
| `go build -tags adelpos_production ./cmd/ad-el-pos-server`                                                                                                                             | Falla por `llaves-production.json`, como se esperaba                                                                                                                                                                                                                              |
| R1-4 a mano: una copia huérfana con `sync-check` y la compilación de producción                                                                                                        | `sync-check` la aprueba y la compilación compila. El archivo se borró                                                                                                                                                                                                             |
| `format`, `fmt-check`, `sync-check` y `gofmt -l`                                                                                                                                       | Aprobados. Los MD5 de `docs/etapas/`, el contrato, la guía y `contracts/` son iguales antes y después                                                                                                                                                                             |

No hizo falta `go-int ./cmd/ad-el-pos-server`, porque no se agregaron pruebas de integración ahí, y no se tomó el
candado.

### Commits (rama `etapa-l1-r1`)

- `ca5db48` etapa L1 revisión R1: pruebas adversariales del verificador de la §10 (pasan)
- `5ef1ab8` etapa L1 revisión R1: R1-1, Accept reemplaza un estado.json que no se pudo leer (prueba que falla)
- `419b0d8` etapa L1 revisión R1: R1-2 y R1-3, borrar estado.json anula la huella y la revisión aceptada (pruebas
  que fallan)
- `82f6a4a` etapa L1 revisión R1: R1-3 también con estado.json dañado o editado e importando el .lic anterior

Archivos: `backend/internal/licensing/review_r1_test.go`, `review_r1_defects_test.go`,
`review_r1_defects_unix_test.go` y `backend/cmd/ad-el-pos-server/review_r1_license_test.go`.

---

## Fase B · Cliente en línea y sin internet

La hizo el agente del núcleo en `$SP/wt/l1-a`, rama `etapa-l1-a`, desde `406e730`, con `ISO_TAG=LA`. En el camino trajo
`etapa-l1` con la Fase D (`dcb6cd3`) e hizo las correcciones de la revisión R1.

- Sin Go temporal y sin emuladores.
- Ninguna llamada a `licensing.sicrat.com`: todo contra el servidor de licencias simulado, en loopback.
- No se leyeron los códigos de licencia de prueba.

### Qué se hizo

- **Cliente en línea** (`backend/internal/licensing/client.go`, §8 del contrato):
  - **Transporte**: HTTPS con TLS verificado (TLS 1.2 o más), JSON, cuerpos de 16 KiB como máximo, 15 s por intento, sin
    redirecciones y sin proxy.
  - **Cada operación**: pide su desafío y firma la prueba de posesión (`SignProof`, que da la prueba publicada en
    `vectors.json`). Las operaciones son `Activate`, `Transfer`, `Refresh` y `Deactivate`, una a la vez.
  - **Reintentos**: hasta tres intentos con **el mismo cuerpo y el mismo `request_id`**. Se reintentan las fallas de red,
    `TEMPORARY_UNAVAILABLE` y `RATE_LIMITED` con un `Retry-After` de 5 s o menos. Un `Retry-After` más largo se
    devuelve con sus segundos.
  - **Se decide por `error.code`** (`*ServerError`), nunca por el HTTP ni por el `message`.
  - **La respuesta**: cada `license_jws` pasa por `Manager.Accept` (§10). Una licencia que no pasa se ignora
    (`*ResponseRejectedError`) y sigue la instalada.
  - **Hora confiable** (L1-D26): solo el `server_time` de una respuesta correcta la alimenta. Correcta quiere decir 200,
    con su forma, su `request_id` y su licencia aceptada.
  - **Errores sin respuesta** (red, TLS, plazo, respuesta inesperada): `*TransportError` con su motivo. Nunca llegan a
    `Accept`, así que nunca afectan la licencia instalada.
  - **En `test_mode`** (L1-D18): solo loopback, salvo `ADEL_LICENSE_ALLOW_REMOTE=1`. Se comprueba en dos lugares: la
    dirección al crear el cliente y, además, cada conexión ya resuelta (`loopbackOnly`).
- **La consulta en segundo plano** (`worker.go`):
  - la primera, entre 1 y 5 minutos después de arrancar;
  - después, cada 24 h ± 1 h;
  - si falla, en 1 hora o en lo que pida `Retry-After`;
  - solo con una activación instalada que no sea un cierre, y nunca con la instalación copiada.
  - Arranca en `serveHTTP` y se apaga en orden.
- **Sin internet** (`offline.go`):
  - `Manager.OfflineRequest` arma el `.licreq` de `activate` (código opcional), `renew` y `deactivate`. Lleva los campos
    en el orden de los casos y JSON compacto, así que es **idéntico byte por byte** a los de `vectors.json`. Antes de
    entregarlo se comprueba con `VerifyOfflineRequest`.
  - `Manager.ImportLicense` acepta un `.lic` con la §10. Antes quita los espacios, los saltos de línea y el BOM de
    alrededor; el `sha256` sale del JWS ya recortado.
- **El código comercial** (L1-D4, `keyformat.go`): 15 símbolos de `23456789ABCDEFGHJKMNPQRSTUVWXYZ` en grupos 3-4-4-4,
  con guiones opcionales y sin distinguir mayúsculas. Se normaliza antes de enviarlo. Nunca se registra ni se audita.
- **Rutas `license/*`** (`administration/license.go`, con su rama en `routes.go`). Las operaciones en línea corren
  después de confirmar la autenticación (`completeDeferred`, en `reports.go`): la red nunca retiene una conexión de la
  base ni un lock. Se auditan en una transacción corta aparte. El plazo de estas rutas es de 75 s (`http.go`).

  | Ruta                                                  | Permiso                       | Qué hace                                                                                          |
  | ----------------------------------------------------- | ----------------------------- | ------------------------------------------------------------------------------------------------- |
  | `GET license/status`                                  | cualquier sesión              | Estado completo (`LicenseStatus`)                                                                 |
  | `GET license/status` sin `Authorization`              | credencial del dispositivo    | Resumen `{state, environment, copied}`. Es una rama en `dispatch`, antes de `authenticateSession` |
  | `POST license/activate {license_key}`                 | `license.manage`              | §8.3                                                                                              |
  | `POST license/transfer {license_key, confirmed}`      | `license.manage`              | §8.4; `confirmed: true` es obligatorio                                                            |
  | `POST license/refresh`                                | `license.manage`              | «Buscar actualización»                                                                            |
  | `POST license/deactivate {confirmed}`                 | `license.manage`              | «Liberar esta licencia»                                                                           |
  | `POST license/offline-request {action, license_key?}` | `license.manage`              | `{action, request_id, filename, content}`                                                         |
  | `POST license/import {license}`                       | `license.manage`              | El `.lic`; el rechazo se audita aunque no cambie nada                                             |
  | `POST license/identity/regenerate {confirmed}`        | `license.manage` y solo local | L1-D11, ver abajo                                                                                 |

- **La identidad nueva de una instalación copiada** (L1-D11). Solo se hace con `copied`, si no responde 409
  `IDENTITY_REGENERATION_NOT_NEEDED`, y solo por la escucha local, si no responde 403 `LICENSE_IDENTITY_LOCAL_ONLY`.
  1. Revoca las terminales emparejadas por red, con sus sesiones (`identity_regenerated`) y sus credenciales de envío.
  2. `localsetup.RegenerateIdentity` aparta `identity/` completa a `identity-reemplazada-AAAAMMDD-HHMMSS/` y crea
     `server_id`, certificado, llave de licencias y registro de la huella nuevos. Si algo falla, la anterior vuelve a
     su lugar.
  3. `Manager.SwitchInstallation` pasa el gestor a la identidad nueva, **con la misma última hora confiable**
     (L1-D25).
  4. Responde `restart_required: true`: la escucha de red sigue con el certificado anterior hasta reiniciar.
  5. Se audita `license.identity_regenerated`.
- **Antes de ir a la red** se comprueba si la identidad está disponible (`Manager.SigningProblem`): con la instalación
  copiada, 409 `LICENSE_INSTALLATION_COPIED`.
- **El permiso** `license.manage` («Administrar la licencia», L1-D21):
  - en la sección «Fase B» de la `0028`, solo para el rol `administrator`;
  - en `permissions.go` y en `permission_catalog_entries.go`, en el grupo Sistema;
  - no entra en ninguna capacidad de la vista sencilla ni en ningún rol de fábrica.
- **Versión 0.15.0 y esquema 28**: ya venían de la Fase D. Al unir quedaron iguales.
- **Capacidad** `license-v1` (L1-D22), con su prueba en `clientversion_test.go`.
- **Contrato v1**:
  - `getLicenseStatus` en `openapi.json`, con `LicenseStatus` y `LicenseDeviceSummary` y la seguridad «sesión» o «solo
    el dispositivo»;
  - su paso en `TestContractV1`, con y sin sesión;
  - la sección «Licencia del servidor principal» en `docs/API_V1.md`, con la tabla de códigos;
  - las rutas del escritorio en `API_CONTRACTS.md`.
- **`server.json`**: `license_server_url` opcional (https), y `newLicenseClient` en `cmd/ad-el-pos-server/license.go`.
- **Registro de movimientos**: los nombres de las acciones de la licencia en `desktop/src/services/auditActions.ts`.
  Además, `license.seat_released` de la Fase D, que no tenía nombre y hacía fallar `auditActions.test.ts`.

### La interfaz para la Fase C (L1-D5)

```go
// En administration: service.LicenseServer.ServerTime(ctx)
func (client *licensing.Client) ServerTime(ctx context.Context) (licensing.ServerTime, error)

type ServerTime struct {
	Time   time.Time // la hora del servidor de licencias, en UTC
	Source string    // licensing.ServerTimeFromRefresh o licensing.ServerTimeFromChallenge
}
```

- **Con una activación instalada** (y la identidad utilizable) usa `refresh`. Su `server_time` es contractual, y como
  toda respuesta correcta también sube la última hora confiable y puede traer una revisión nueva.
- **Sin activación**, o con la instalación copiada, pide un desafío de `activate`, que no se usa. La hora es su
  `expires_at` menos 300 s, y no alimenta la última hora confiable (L1-D26). Para la §6.3 decide la Fase C, con
  `Manager.ReplaceTrustedTime`.
- **Errores**: los del cliente (`*ServerError`, `*TransportError`, `*ResponseRejectedError`, `ErrIdentityUnavailable`).
  `licenseFault` de `administration/license.go` los traduce a los códigos de L1-D17.

### Herramientas de prueba para las Fases E y F (`backend/cmd/license-test`)

- **Qué es**: un programa aparte del servidor, con dos órdenes: `issue` y `server`.
- **Exige `ADEL_TEST_MODE=1`**. Nunca llama al servidor real: firma con las llaves públicas de `vectors.json`.
- **Nunca llega a un instalador ni a producción**:
  - todas sus fuentes llevan `//go:build !adelpos_production`;
  - `guard_test.go` comprueba que `./cmd/ad-el-pos-server` no depende de `internal/licensetest` ni de esta orden, que
    `build-macos-pkg.sh`, `build-windows.ps1`, `windows/dev.mjs` y `dev.sh` solo compilan el servidor, y que con la
    etiqueta de producción no compila.
- **El Go temporal debe confiar en esas llaves**. Necesita:
  - `test_mode: true`;
  - `ADEL_TEST_LICENSE_TRUST=<ruta absoluta de contracts/licencias/v1/fixtures/vectors.json>`;
  - para activar en línea, además, `"license_server_url": "https://127.0.0.1:<puerto>"` en su `server.json` y
    `ADEL_TEST_LICENSE_CA=<el PEM de --ca-out>`.
  - Hoy `iso.sh server-up` no pasa esas variables: **hay que agregarlas al arnés** o arrancar el Go temporal a mano.

**`issue`: un `.lic` de prueba** para poner el Go temporal en cualquier estado con `POST license/import`.

```sh
cd backend
ADEL_TEST_MODE=1 ADEL_TEST_LICENSE_TRUST="$PWD/../contracts/licencias/v1/fixtures/vectors.json" \
  go run ./cmd/license-test issue --identity "$ISO_SERVER_IDENTITY" \
  --type subscription --expires-at -2d --revision 2 --license-id 7a0c5a4e-0000-4000-8000-000000000001 \
  --output "$SP/run-E/tolerancia.lic"
# {"event":"issued","output":".../tolerancia.lic","license_id":"7a0c…","license_revision":2}
```

| Opción                                                                                     | Valor (por omisión)                                                                                                    |
| ------------------------------------------------------------------------------------------ | ---------------------------------------------------------------------------------------------------------------------- |
| `--identity <carpeta identity>`                                                            | La del Go temporal (`ISO_SERVER_IDENTITY`). Se lee su `server_id` y su llave pública. O `--server-id` y `--public-key` |
| `--type`                                                                                   | `perpetual`, `subscription` o `trial` (`perpetual`)                                                                    |
| `--expires-at`, `--issued-at`, `--period-starts-at`, `--updates-until`                     | RFC 3339 o un desfase desde ahora (`-2d`, `36h`, `-90m`). Suscripción: +1 mes; prueba: +15 días                        |
| `--grace-days`                                                                             | El del tipo (5 en suscripción, 0 en los demás). Otro valor falla, porque el contrato lo rechaza                        |
| `--plan`                                                                                   | `basico`, `elemental` o `premium` (`elemental`, 3/3/3)                                                                 |
| `--cash-registers`, `--order-takers`, `--kitchen-displays`                                 | Los del plan. Con otro valor la licencia queda personalizada                                                           |
| `--status`, `--reason`                                                                     | `active` o `revoked`, con `deactivated`, `transferred` o `revoked` (`revoked`)                                         |
| `--revision`, `--license-id`, `--activation-id`, `--licensee`, `--kid`, `--billing-period` | `1`, nuevos, «Restaurante de prueba», `adelpos-test-fixture-a`, `monthly`                                              |
| `--output`                                                                                 | Sin él, el `.lic` va en `output` de la línea JSON                                                                      |

- **Los estados**: vigente (`--expires-at 10d`), tolerancia (`-2d`), vencida (`-6d` en suscripción, o una prueba con
  `-1d`), revocada (`--status revoked`), 0 comanderos (`--order-takers 0`) y plan básico (`--plan basico`).
- **La revisión**: con el mismo `--license-id`, cada `.lic` nuevo necesita una revisión mayor que la anterior (§10.4).
  Si no, `LICENSE_FILE_INVALID` con `stale_revision`. Usa otro `--license-id` por escenario o sube `--revision`.

**`server`: el servidor de licencias simulado** como proceso, con TLS y solo en loopback.

```sh
ADEL_TEST_MODE=1 ADEL_TEST_LICENSE_TRUST="$PWD/../contracts/licencias/v1/fixtures/vectors.json" \
  go run ./cmd/license-test server --listen 127.0.0.1:8475 --ca-out "$SP/run-E/licencias-ca.pem"
# {"event":"ready","url":"https://127.0.0.1:8475","ca":".../licencias-ca.pem"}   (SIGTERM: {"event":"stopped"})
```

Cumple la API del contrato:

- el desafío es de un solo uso, de 5 minutos;
- la prueba de posesión se verifica;
- la revisión es global por licencia;
- la misma instalación que vuelve a activar recibe su revisión vigente;
- la transferencia queda pendiente hasta que la computadora anterior consulte o libere;
- los cierres llegan con su motivo;
- un reintento idéntico recibe la misma respuesta y uno distinto, `REQUEST_CONFLICT`;
- los errores de transporte son 404, 405, 413 y 415.

**Cada código comercial** con el formato de L1-D4 es una licencia distinta del simulador. Son códigos inventados del
simulador, no licencias reales. Su **primer grupo** decide el comportamiento:

| Prefijo | Comportamiento                                                                                                                |
| ------- | ----------------------------------------------------------------------------------------------------------------------------- |
| `PRP`   | Perpetua, Plan elemental (3, 3 y 3)                                                                                           |
| `BSC`   | Perpetua, Plan básico (1, 0 y 0)                                                                                              |
| `MNS`   | Suscripción mensual desde la primera activación, 5 días de tolerancia                                                         |
| `PRB`   | Prueba de 15 días, Plan básico                                                                                                |
| `GRC`   | Suscripción vencida hace 2 días (tolerancia)                                                                                  |
| `VNC`   | Suscripción vencida hace 40 días (vencida)                                                                                    |
| `RVK`   | 403 `REVOKED`                                                                                                                 |
| `ACT`   | Activa en otra computadora: 409 `ACTIVATION_LIMIT` con `transfer_available: true`; la transferencia funciona                  |
| `ACN`   | Activa en otra computadora con una transferencia pendiente: `transfer_available: false`, y transferir da 409 `TRANSFER_LIMIT` |
| `TMP`   | 503 `TEMPORARY_UNAVAILABLE`                                                                                                   |
| `RAT`   | 429 `RATE_LIMITED` con `Retry-After: 30`                                                                                      |
| `PRF`   | 401 `INVALID_PROOF`                                                                                                           |
| otro    | 404 `LICENSE_NOT_FOUND`                                                                                                       |

Ejemplos: `PRP-2222-2222-2222`, `MNS-2223-2222-2222` y `ACT-2222-2222-2222`. Usa solo `23456789ABCDEFGHJKMNPQRSTUVWXYZ`.

**Controles** (POST con JSON en la misma dirección, salvo `estado`):

| Control                      | Cuerpo                                                   | Efecto                                                                                           |
| ---------------------------- | -------------------------------------------------------- | ------------------------------------------------------------------------------------------------ |
| `/_prueba/siguiente-error`   | `{"accion","codigo","retry_after","transfer_available"}` | El siguiente error de esa acción (`challenge`, `activate`, `transfer`, `refresh` o `deactivate`) |
| `/_prueba/perder-siguiente`  | `{"accion"}`                                             | La siguiente respuesta se guarda y se pierde (el cliente reintenta)                              |
| `/_prueba/alterar-siguiente` | `{}`                                                     | La siguiente licencia llega con la firma alterada                                                |
| `/_prueba/hora`              | `{"desfase":"-2h"}`                                      | La hora del servidor (para la §6.3 de la Fase C)                                                 |
| `/_prueba/revocar`           | `{"license_key"}`                                        | La revocación del panel (prueba 8 de la guía)                                                    |
| `/_prueba/renovar`           | `{"license_key","meses"}`                                | Una revisión nueva con más vigencia (prueba 7)                                                   |
| `GET /_prueba/estado`        | —                                                        | Licencias, revisión, activación y transferencia pendiente                                        |

**Android (Fase F)**: el teléfono lee `GET license/status` con solo su credencial (el resumen) o con su sesión. El
estado del Go temporal se pone con `issue` + `license/import`, desde la computadora principal y con la sesión del
administrador.

### Correcciones de la revisión R1

Tomé las pruebas del revisor con `cherry-pick` de `ca5db48`, `5ef1ab8`, `419b0d8` y `82f6a4a` (aquí `85062de`,
`4a18e00`, `c03adcf` y `dc52145`). Ninguna se ajustó y **las siete pasan**.

| Hallazgo                                   | Corrección                                                                                                                | Pruebas                                                                                                                                                  |
| ------------------------------------------ | ------------------------------------------------------------------------------------------------------------------------- | -------------------------------------------------------------------------------------------------------------------------------------------------------- |
| **R1-2**, la huella anulada                | Ver abajo                                                                                                                 | La del revisor (las tres subpruebas) y `TestMachineRecordFailsClosed`: sin registro, alterado o de otra instalación                                      |
| **R1-1**, estado ilegible                  | Ver abajo                                                                                                                 | La del revisor, `TestUnreadableStateIsReadAgainBeforeWriting` y `TestUnreadableStateIsNeverReplaced`                                                     |
| **R1-3**, volver a una revisión superada   | Ver abajo                                                                                                                 | La del revisor (borrado, dañado y editado), `TestRevisionFloorUsesBothCopies`, `TestDatabaseRevisionCopyNeverGoesBack` (`go-int`) y `TestStageL1Upgrade` |
| **R1-4**, copia de llaves huérfana         | `sync-metadata.mjs`: con `--check`, una `backend/internal/licensing/llaves-*.json` sin su fuente falla; `sync` la borra   | A mano: una `llaves-production.json` huérfana hace fallar `sync-check` y `sync` la borra                                                                 |
| **R1-6**, `.gitignore`                     | `**/identity/license-key.pem`, `**/identity/license-machine.json`, `**/identity/licencia/` y `**/identity-reemplazada-*/` | `git check-ignore` de las tres primeras rutas                                                                                                            |
| **R1-5**, claves repetidas en el contenido | No se cambia, para no ser más estricto que el verificador de referencia                                                   | Propuesta para la V1.1                                                                                                                                   |

- **R1-2, la huella firmada**:
  - **Dónde vive**: en `identity/license-machine.json` (`machine_record.go`), firmada con la llave Ed25519 y atada al
    `server_id` y a la llave pública. Mensaje `ADELPOS-EQUIPO-REGISTRO-V1\n…`.
  - **Cuándo se crea**: `EnsureLicenseKey` crea primero el registro y al final la llave. El enlace de la llave es el
    punto de confirmación: un corte deja a lo más un registro sin llave, que se reemplaza en el siguiente intento.
    Sin poder leer la huella no se crea nada.
  - **Al arrancar**: con la llave y sin registro válido (falta, está alterado o es de otra instalación) o con otra
    huella, se falla cerrado: `Copied`, «Sin activar» o «Sin activar con datos» (L1-D26).
  - **Nunca se vuelve a crear solo**: el único camino es la identidad nueva.
  - **Ya no se registra la huella por faltar en `estado.json`**. El campo sigue ahí, informativo.
  - **Qué no protege**: quien tiene la llave privada (la misma carpeta, los mismos permisos) puede firmar otro registro.
    Es el administrador del equipo (§12 del contrato).
  - **Efecto en instalaciones de prueba**: un Go temporal o una instalación de desarrollo que creó la llave con la
    Fase A, antes de esta corrección, no tiene registro. Queda como copiada y necesita la identidad nueva. La Mac real
    sigue en 0.14.0, sin llave: con la 0.15.0 crea las dos juntas.
- **R1-1, el estado ilegible**:
  - El gestor recuerda si leyó `estado.json` (`stateLoaded`).
  - Toda escritura lo vuelve a intentar bajo el mutex: `Accept`, `ObserveTime` del `server_time`, `Flush`,
    `ReplaceTrustedTime`, la consulta en segundo plano y la carpeta que no se pudo preparar.
  - Si sigue ilegible: `ErrStateNotSaved` sin escribir nada.
  - Al poder leerlo, conserva la hora observada en memoria y recupera la licencia.
- **R1-3, la segunda copia de las revisiones**:
  - **La tabla**: `app.license_revisions` (`license_id`, revisión, `sha256`), en la sección «Fase A (revisión R1)» al
    principio de la `0028`. `PoolProbe` la lee y la escribe; nunca baja (`ON CONFLICT … WHERE`).
  - **La revisión mínima aceptable** es el máximo de las dos copias. Al arrancar y en `Accept`, la licencia guardada y
    `vigente.lic` deben ser exactamente esa revisión.
  - **La recuperación** desde `vigente.lic` como «primera revisión» solo vale si ninguna de las dos copias conoce la
    licencia.
  - **Al aceptar** se escribe primero la copia de la base: si falla, no se acepta. Si la base no se pudo leer al
    arrancar, `Accept` la vuelve a leer y, si sigue sin poder, no acepta.
  - **La prueba de subida** (`TestStageL1Upgrade`) comprueba la tabla vacía y sus restricciones. Las pruebas de subida
    anteriores (`stageA1Snapshot`, `stage9HRowCounts` y la de la etapa 8) excluyen lo nuevo de la `0028`.

### Decisiones propuestas

1. **Las herramientas de prueba son un programa aparte**, `cmd/license-test`, y no órdenes del servidor como
   `legacy-synthetic`. Así no llegan a ningún instalador, aunque el `.pkg` sea de desarrollo. Una prueba lo comprueba.
2. **La autoridad del servidor simulado** (`ADEL_TEST_LICENSE_CA`, solo con `test_mode` y en desarrollo): el cliente
   sigue verificando TLS también en las pruebas, en lugar de admitir `http`.
3. **Los códigos nuevos de la gestión**:
   - `LICENSE_INSTALLATION_COPIED` (409);
   - `LICENSE_STATE_NOT_SAVED` (503);
   - `IDENTITY_REGENERATION_NOT_NEEDED` (409);
   - `LICENSE_IDENTITY_LOCAL_ONLY` (403);
   - `IDENTITY_UNAVAILABLE` con `details.reason` (`identity` o `machine_unverified`);
   - `LICENSE_NOT_ACTIVATED` (el de la puerta) para consultar, liberar y renovar sin activación.
4. **Cuándo se reintenta**: tres intentos con el mismo cuerpo ante una falla de red, `TEMPORARY_UNAVAILABLE` o
   `RATE_LIMITED` con `Retry-After` de 5 s o menos. Un `Retry-After` mayor se devuelve como `LICENSE_SERVER_BUSY`, con
   sus segundos.
5. **La identidad nueva solo con `copied`**, y la escucha de red no se cambia en caliente: `restart_required: true`.
   Entre la identidad nueva y el reinicio, emparejar una terminal fijaría el certificado anterior. El escritorio (Fase
   E) debe pedir «Reiniciar» enseguida.
6. **`transfer` y `deactivate` piden `confirmed: true`** en el cuerpo, además de la confirmación en pantalla.
7. **La consulta en segundo plano no se audita** (solo deja la bitácora). Una revocación que llega así aparece en el
   estado. Lo audita la Fase C con `license.changed` si hace falta.

### Archivos

- **Paquete `backend/internal/licensing/`**:
  - el cliente y sin internet: `client.go`, `worker.go`, `offline.go`, `keyformat.go`;
  - R1: `machine_record.go`, y los cambios en `manager.go` y `datapresence.go` (también la parte de R1).
  - pruebas: `client_test.go` (paquete externo), `offline_test.go`, `review_r1_fixes_test.go`,
    `no_real_server_test.go`, `store_unix_test.go` y las del revisor.
- **Herramientas de prueba**:
  - `backend/internal/licensetest/` (`fixtures.go`, `issue.go`, `server.go`, `control.go`, `tls.go` y
    `server_test.go`);
  - `backend/cmd/license-test/` (`main.go`, `main_test.go` y `guard_test.go`).
- **`backend/internal/administration/`**:
  - `license.go` y `license_acceptance_test.go`;
  - bloques propios en `routes.go`, `reports.go` (`completeDeferred`), `http.go` (el plazo) y `service.go` (el campo
    `LicenseServer` y el resumen sin sesión en `dispatch`);
  - `permissions.go` y `permission_catalog_entries.go`.
- **`backend/internal/localsetup/`**:
  - `identity_regenerate.go`;
  - `license_key.go`: el registro de la huella, con la nueva firma `EnsureLicenseKey(dir, fingerprint)`.
- **`backend/cmd/ad-el-pos-server/`**: `license.go` (`newLicenseClient` y las ayudas de prueba) y `main.go` (el cliente
  y su trabajador).
- **Configuración y contrato**:
  - `backend/internal/config/config.go` (`license_server_url`) y `backend/internal/httpapi/router.go` (`license-v1`);
  - `backend/migrations/0028_licencias.sql` (las secciones «Fase A (revisión R1)» y «Fase B», antes de la «Fase D»);
  - `backend/internal/apicontract/openapi.json` y `contract_integration_test.go`, `docs/API_V1.md` y `API_CONTRACTS.md`.
- **Pruebas de la base**: `backend/internal/database/stagel1_upgrade_test.go`, `stagea1_upgrade_test.go`,
  `stage9h_upgrade_test.go` y `existing_install_test.go`.
- **Otros**: `desktop/src/services/auditActions.ts`, `scripts/sync-metadata.mjs` y `.gitignore`.

### Pruebas (realmente ejecutadas)

| Prueba                                                                                            | Resultado                                                                                                                                                                                                                                                                                                                                                                                                                                    |
| ------------------------------------------------------------------------------------------------- | -------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Cliente contra el servidor simulado (`TestClient*`: 9 pruebas)                                    | Aprobadas. Cubren cada código de error, el reintento con el mismo cuerpo y `request_id` (respuesta perdida y repetida), `Retry-After` (corto se espera, largo se devuelve), `ACTIVATION_LIMIT` true y false, `TRANSFER_LIMIT`, el JWS alterado (su `server_time` no cuenta), `TEMPORARY_UNAVAILABLE` sin cambiar la licencia, red y TLS, `ServerTime` por desafío y por `refresh`, la consulta en segundo plano y el loopback en `test_mode` |
| `.licreq` (`TestOfflineRequestsMatchTheContractFixtures`, `TestManagerOfflineRequests`)           | Aprobadas: las tres acciones idénticas byte por byte a `vectors.json` y verificadas con la referencia                                                                                                                                                                                                                                                                                                                                        |
| `TestImportLicenseFile` y `TestNormalizeLicenseKey`                                               | Aprobadas                                                                                                                                                                                                                                                                                                                                                                                                                                    |
| El simulador (`TestSimulatedServerFollowsTheContract`)                                            | Aprobada: 404, 405 con `Allow`, 415, 413, campo sobrante, reintento idéntico, `REQUEST_CONFLICT`, desafío usado y `Retry-After`                                                                                                                                                                                                                                                                                                              |
| `license-test` (`TestLicenseTestRequiresTestMode`, `…Issue`, `…Server`, `TestTestToolsNeverShip`) | Aprobadas                                                                                                                                                                                                                                                                                                                                                                                                                                    |
| `TestNoAutomatedTestCallsTheRealLicenseServer` (§9.1)                                             | Aprobada. A mano, con una especificación temporal que nombraba el servidor real, falló como debía (se borró)                                                                                                                                                                                                                                                                                                                                 |
| Las pruebas del revisor R1 (7) y las de las correcciones                                          | Aprobadas                                                                                                                                                                                                                                                                                                                                                                                                                                    |
| `go-int ./internal/administration`, con `TestLicenseRoutes`                                       | Estado 0, el paquete completo (274 s)                                                                                                                                                                                                                                                                                                                                                                                                        |
| `go-int ./cmd/ad-el-pos-server`, con el candado                                                   | Estado 0 (67 s); el candado se liberó                                                                                                                                                                                                                                                                                                                                                                                                        |
| `go-int ./internal/database`                                                                      | Estado 0, con `TestStageL1Upgrade` (Fases A-R1, B y D)                                                                                                                                                                                                                                                                                                                                                                                       |
| `go-int ./internal/apicontract`                                                                   | Estado 0 (`TestContractV1` con `getLicenseStatus` y `TestPlainLanguageMessages`)                                                                                                                                                                                                                                                                                                                                                             |
| `go-int ./internal/licensing`                                                                     | Estado 0                                                                                                                                                                                                                                                                                                                                                                                                                                     |
| `go-unit` (vet normal e `integration`, `test -race`, `GOOS=windows` build y vet)                  | Aprobado                                                                                                                                                                                                                                                                                                                                                                                                                                     |
| `vitest src/services/auditActions.test.ts`                                                        | 5 de 5                                                                                                                                                                                                                                                                                                                                                                                                                                       |
| `format`, `fmt-check` y `sync-check`                                                              | Aprobados                                                                                                                                                                                                                                                                                                                                                                                                                                    |
| MD5 de `docs/etapas/`, del contrato, de la guía y de `contracts/`                                 | Iguales antes y después                                                                                                                                                                                                                                                                                                                                                                                                                      |

### Lo no verificado

- **Windows**: la ACL de `license-machine.json` y la lectura real de `MachineGuid`. Solo `GOOS=windows` vet.
- **El servidor simulado como proceso, contra un Go temporal real**: `TestLicenseTestServer` lo prueba con un gestor en
  el mismo proceso, pero no se arrancó el Go temporal con `license_server_url`. Es de la Fase E.
- **Activar contra `licensing.sicrat.com`**: es de la Fase G.
- **Reiniciar la escucha de red después de la identidad nueva**: solo se responde `restart_required`.

### Para el coordinador

- **La unión `755f0b8`** dejó la `0028` con un separador de conflicto. Se corrigió en `0d75e56`: hay que fusionar la
  rama entera, no la unión sola.
- **Los números de decisión**: el mensaje de `df8f1e3` usa L1-D27 a L1-D29, la numeración del primer aviso. Los
  comentarios del código ya no llevan números, solo los hallazgos R1-1 a R1-3.
- **Conectar `Seats` con el gestor**: `licenseseats` ya está en `etapa-l1`, pero `main.go` todavía le pasa
  `Limits: nil`. Conectar `application.License.Status()` (`Limits` y `HasLicense()`) es de la Fase C o del coordinador,
  como dice el comentario de `main.go`.
- **El arnés**: para las Fases E y F hay que agregar a `iso.sh server-up` `ADEL_TEST_LICENSE_TRUST`,
  `ADEL_TEST_LICENSE_CA` y `license_server_url`.

### Commits (rama `etapa-l1-a`, desde `406e730`)

- `fd2c9d0` cliente del servidor de licencias (en línea, sin internet y consulta en segundo plano)
- `4913923` servidor de licencias simulado y orden license-test, solo de pruebas, y pruebas del cliente
- `f703972` license_server_url en server.json e identidad nueva de una instalación copiada
- `6645495` rutas license/*, permiso license.manage (0028, sección «Fase B»), capacidad license-v1 y versión 0.15.0
- `85062de`, `4a18e00`, `c03adcf` y `dc52145`: las pruebas de la revisión R1 (cherry-pick)
- `df8f1e3` correcciones de R1-1, R1-2 y R1-3
- `7417221` sync-check con una copia de llaves sin fuente (R1-4) y `.gitignore` (R1-6)
- `755f0b8` trae etapa-l1 con la Fase D
- `e861692` los comentarios nombran los hallazgos en lugar de números de decisión
- `0d75e56` la 0028 unida bien y la prueba de subida de 27 a 28
- `df9e868` contrato v1 con getLicenseStatus, TestContractV1, docs/API_V1.md y API_CONTRACTS.md
- `7c9ccc8` nombres del Registro de movimientos para la licencia (y license.seat_released)
- `3c109d6` pruebas de las rutas license/* y la copia comprobada antes de ir a la red
- `75cb9d0` prueba de que ninguna prueba automática nombra ni llama al servidor real (§9.1)

### Decisiones del coordinador sobre la revisión R1 y la Fase B

La Fase B y las correcciones de R1 están fusionadas en `5cd23dd`; `go-unit` pasó sobre el árbol fusionado. El mensaje del
commit `df8f1e3` nombra estas decisiones como L1-D27 a L1-D29, la numeración de un aviso anterior. Valen las de aquí.

- **L1-D29 · R1-2 (alta).** La huella del equipo va en `identity/license-machine.json`, firmada con la llave de licencias
  y atada al `server_id`. Si el registro falta, está alterado o es de otra huella, la instalación queda como copiada (falla
  cerrada). No protege contra quien tiene la llave privada: es la §12 del contrato.
- **L1-D30 · R1-1 (media).** Con un estado ilegible no se escribe nada (`ErrStateNotSaved`).
- **L1-D31 · R1-3 (media).** Hay una segunda copia del historial de revisiones en `app.license_revisions`, sección «Fase A
  (revisión R1)» de la `0028`. La revisión mínima es el máximo de las dos copias, así que ni restaurar un respaldo viejo ni
  borrar el archivo devuelven una revisión superada. L1-D9 sigue igual.
- **L1-D32 · R1-4 y R1-6 (bajas).** `sync-check` detecta copias de llaves sin fuente, y `.gitignore` excluye la llave, el
  registro de la huella y el estado de la licencia.
- **L1-D33 · R1-5 (baja).** No se cambia: así no somos más estrictos que el verificador de referencia. **Propuesta para la
  V1.1**: que el contenido firmado rechace claves repetidas y datos después del objeto.
- **L1-D34 · Se aceptan las seis decisiones propuestas de la Fase B**:
  - las herramientas `license-test` son un programa aparte;
  - `ADEL_TEST_LICENSE_CA`, solo en `test_mode` y en desarrollo;
  - los códigos nuevos de la gestión de la licencia;
  - tres intentos, y un `Retry-After` de más de 5 s se devuelve como `LICENSE_SERVER_BUSY`;
  - `confirmed: true` en `transfer` y `deactivate`;
  - «Reiniciar» después de la identidad nueva.
- **Para las Fases C, E, F y G**:
  - Las instalaciones de prueba creadas con la Fase A (llave sin registro de huella) quedan como copiadas: se crean
    nuevas.
  - El arnés pasa las ayudas de prueba de la licencia a `server-up`: lo hace el coordinador.
  - La conexión de los cupos con la licencia la hace la Fase C.

---

## Fase C · Estados en el backend y protección de la hora

Hizo la fase el agente de la puerta y la hora, en `$SP/wt/l1-c`, rama `etapa-l1-c`, desde `etapa-l1` (`406e730`, con la
Fase A fusionada), con `ISO_TAG=LC`. A mitad de la fase se trajo `etapa-l1` con la Fase D fusionada (`f4b42a3` y
`dcb6cd3`): fusión `ed71382`, con un solo conflicto (los campos de `Service` en `service.go`), resuelto conservando los
dos lados.

- Sin Go temporal ni emuladores. Ninguna llamada a `licensing.sicrat.com`. No se leyeron los códigos de licencia de
  prueba. La hora de la Mac no se tocó: todo con el reloj simulado (`licensingtest.Clock`).
- **La Fase C no guarda nada en la base**: no hay sección «Fase C» en la `0028`. El evento `license.changed` usa el
  ámbito `configuration`, que ya existe (ver Decisión propuesta 9).
- La llave de soporte de desarrollo se generó con la herramienta en `~/.adel-pos-dev/soporte/` (carpeta 0700, llave
  0600). La privada nunca se mostró; la salida solo tiene el `kid`, la pública y la ruta.

### Qué se hizo

**La tabla de la §6** (`backend/internal/licensing/operations.go`): la única clasificación por `(método, ruta)`.

- Cada operación tiene su fila (`RowCapture` F1, `RowCaptured` F2, `RowOpenShift` F3, `RowCloseShift` F4,
  `RowKitchen` F5, `RowRecords` F6, `RowAdministration` F7, `RowLicense` F8, y las que la tabla no lista:
  `RowAlways` y `RowStartup`, L1-D6), si registra algo con fecha (`Dated`, L1-D14) y si está en la tabla (`Known`).
- `Row.Decide(state)` es la celda de la tabla: `Allow`, `Deny` o `AllowWithPendingAccounts` (abrir turno en Vencida o
  Revocada, L1-D6). Las celdas «—» de Sin activar son «No», salvo reportes, historial y respaldos.
- **Una escritura que no está en la tabla** toma `RowUnknownWrite`: solo Vigente y Tolerancia, y con fecha (Decisión
  propuesta 3).
- **El cupo no se clasifica aquí**: `administration.ClassifyGateOperation` (`license_gate.go:58`) junta la fila con
  `licenseseats.ForOperation(método, ruta)` de la Fase D, sin repetirla.

**La puerta** (`backend/internal/administration/license_gate.go:102`), llamada desde `Service.dispatch` en dos puntos:

- antes de las rutas públicas (asistente, alta local y emparejamiento; `service.go:330`);
- y, para todo lo demás, en cuanto se autentica la terminal (`service.go:371`), antes de la entrada con contraseña o PIN,
  la credencial de envío, el agente de impresión, los avisos de platillos listos y toda operación con sesión.
- **Orden**: primero la hora (una operación con fecha y el reloj bloqueado → `SYSTEM_CLOCK_INVALID`), después el estado
  de la licencia (`LICENSE_NOT_ACTIVATED`, `LICENSE_EXPIRED` o `LICENSE_REVOKED`). El cupo lo aplica después
  `withSeats` (Fase D), con 7820148311 como último lock (L1-D27): la puerta no duplica nada de los cupos.
- **Abrir turno en Vencida o Revocada**: solo con al menos una cuenta `open` con unidades activas (una lectura sin
  locks consultivos), y con cupo de caja (lo aplica la Fase D con los cupos de esa licencia).
- **Las reglas firmes** salen de la tabla: la puerta solo rechaza antes de escribir, cobrar lo abierto (F2) se permite
  en Vencida y Revocada, y reportes, historial y respaldos (F6) siempre, también con la hora incorrecta.
- **Cada operación con fecha aceptada** alimenta la última hora confiable (`ObserveTime(…, TimeFromOperation)`,
  L1-D15).
- **Sin identidad**, el emparejamiento sigue respondiendo `503 PAIRING_UNAVAILABLE` (la causa real) y no el «Sin activar»
  que también resulta de no tener identidad.

**Los cinco códigos de L1-D17** (todos 409, con `message` en español sencillo para los clientes anteriores):

| Código                     | Lo produce | `details`                                                                                                                                  |
| -------------------------- | ---------- | ------------------------------------------------------------------------------------------------------------------------------------------ |
| `LICENSE_NOT_ACTIVATED`    | La puerta  | `operation`, `state` (`not_activated` o `not_activated_with_data`), `copied` y `machine_unverified`                                        |
| `LICENSE_EXPIRED`          | La puerta  | `operation`, `expires_at` y `grace_ended_at`; al abrir turno, `pending_accounts`                                                           |
| `LICENSE_REVOKED`          | La puerta  | `operation` y `reason`; al abrir turno, `pending_accounts`                                                                                 |
| `LICENSE_SEAT_UNAVAILABLE` | Fase D     | `seat`, `in_use` y `limit`                                                                                                                 |
| `SYSTEM_CLOCK_INVALID`     | La puerta  | `blocked`, `local_time`, `last_activity_time`, `margin_minutes`, `timezone`, los dos textos de la pantalla, `license_state` y `license_ok` |

- Los textos son las dos fechas completas en la zona del restaurante: «lunes 23 nov 2026, 10:15»
  (`licensing.ScreenTime`). `license_ok` (Vigente o Tolerancia) decide la aclaración «Tu licencia está bien…».
- El mensaje de `SYSTEM_CLOCK_INVALID` es el de la §6.2 para las terminales: «No se puede registrar: la computadora
  principal tiene la fecha y hora incorrectas. Avisa al encargado.».
- El `LICENSE_NOT_ACTIVATED` sin `details` de los cupos queda como respaldo: la puerta responde antes (L1-D27).

**La protección de la hora** (`backend/internal/licensing/clock.go`):

- `ClockMargin` = 10 min; `CheckClock(local, última)` bloquea con `local < última − 10 min`; `Manager.Clock()` lee las dos
  con el reloj del gestor. Se quita solo al corregir el reloj: no se guarda nada.
- **Se revisa** en cada operación protegida (la puerta), al arrancar y cada 15 s (el vigilante).
- **El vigilante** (`administration/license_monitor.go`, arrancado en `serveHTTP`, `main.go:395`): audita cada bloqueo
  y desbloqueo con las dos fechas (`license.clock_blocked` y `license.clock_unblocked`), cada cambio de estado
  (`license.state_changed`) y publica `license.changed`. Escribe en su propia transacción corta, nunca dentro de la de
  una petición (así la puerta no toma una segunda conexión). La puerta lo despierta al ver un bloqueo.

**§6.3, `POST license/time-check`** (`administration/license_clock.go:98`), según L1-D5:

- la hora del servidor de licencias llega por la interfaz `LicenseServerClock` (`ServerTime(ctx) (time.Time, error)`),
  que implementa el cliente de la Fase B; sin ella, `503 LICENSE_SERVER_UNAVAILABLE`;
- **si coincide** (±10 min) y la última hora confiable estaba en el futuro, se reemplaza por el `server_time`
  (`ReplaceTrustedTime(…, ReplaceByInternetCheck)`) y se audita `license.clock_corrected` con los dos valores y la
  **lista de las operaciones con fecha futura** (cuentas, pedidos, cobros, turnos y movimientos de inventario, con
  hasta 20 identificadores de cada tipo), que no se modifican;
- **si no coincide**: `409 LICENSE_TIME_MISMATCH` con `server_time` y su texto, y se audita el intento.

**§6.4, desbloqueo de soporte** (`backend/internal/licensing/unlock.go`, L1-D19):

- **código de solicitud** de 24 caracteres en base32 de Crockford (versión, etiqueta de 40 bits del `server_id`, número
  de un solo uso, última hora confiable en minutos y 12 bits de verificación). Se lee con minúsculas, espacios, O = 0 e
  I/L = 1;
- **la solicitud pendiente** va en `identity/licencia/estado.json` (campo `support_unlock` de `storedState`, en
  `store.go`), con los últimos 16 números usados. Una solicitud vigente se vuelve a entregar; `renew` crea otra;
- **la respuesta** es un JWS con `typ` `adelpos-unlock+jws`. Se acepta una sola vez, solo en esa instalación y dentro de
  las 24 h medidas con el reloj local (también se rechaza si el reloj se corrigió más de 10 min hacia atrás desde la
  solicitud). Reemplazar la hora y marcar el número como usado van en **una sola escritura**;
- **rutas**: `POST license/support-request` y `POST license/support-unlock` (`{response, source: text|file}`;
  `400 SUPPORT_UNLOCK_INVALID` con `reason`: `used`, `other_installation`, `expired`, `altered` o `not_requested`).
  Cada solicitud, desbloqueo e intento rechazado se audita;
- **las tres rutas de la hora** solo por la escucha local (`403 LICENSE_LOCAL_ONLY`), con cualquier sesión y sin permiso
  especial (Decisión propuesta 5);
- **herramienta**: `backend/cmd/soporte-desbloqueo` (`generar-llave` y `responder`) y su envoltura
  `tools/soporte-desbloqueo/soporte-desbloqueo` con `README.md`. Comparte el formato con el servidor y comprueba que su
  respuesta verifica con la pública compilada antes de entregarla;
- **llaves**: la privada de desarrollo en `~/.adel-pos-dev/soporte/soporte-development.key`; la pública en
  `config/licencias/soporte-development.pub.json` (`kid` `adelpos-soporte-development-20261123`), sincronizada por
  `scripts/sync-metadata.mjs` (que ahora copia también `soporte-<entorno>.pub.json`) y embebida por
  `support_development.go`. `support_production.go` embebe `soporte-production.pub.json`, que no existe: **la
  compilación de producción tampoco compila sin ella** (`TestProductionBuildFailsWithoutProductionKeys` lo comprueba
  ahora con las dos llaves).

**El estado para las terminales**:

- `license` (`LicenseSummary`) en `auth/me` y `restaurant/state` (`routes.go:42` y `:71`): estado, entorno, `copied`,
  motivo de revocación, tipo, vencimiento, días de tolerancia, `days_until_expiry`, `grace_days_remaining` y `clock`
  (`LicenseClock`, con las dos fechas);
- `license.changed` en el canal de eventos (`events.AppendLicenseChanged`), que **reciben todas las personas** que leen
  el canal, sin importar permisos ni estación (`events/store.go:104`).
- **Documentado** en `openapi.json` (esquemas `LicenseSummary`, `LicenseClock`, `LicenseRejection` y
  `SystemClockInvalid`; `license` en `Identity` y `RecoveryState`; los 409 de las operaciones de captura de la Fase D
  con los dos rechazos nuevos; las descripciones de `Error` y `EventBatch`) y en `docs/API_V1.md` (sección «Licencia:
  estado, hora del equipo y rechazos» y tres renglones en los códigos de error).

**Las conexiones**:

- **Los cupos con los límites reales**: `administration.LicenseSeats(manager)` (`license_gate.go:76`) da los cupos de
  `Status().Limits` solo con `HasLicense()` y el reloj `LocalClock()`. `serveHTTP` lo conecta con `useLicense`
  (`cmd/ad-el-pos-server/license.go`, `main.go:268`), en `serve` y en el host de macOS y de Windows, antes de atender
  peticiones. **Desde ahí `etapa-l1` ya no rechaza toda captura** (L1-D28).
- **El gestor con la última operación registrada**: la puerta llama a `ObserveTime(TimeFromOperation)` en cada operación
  con fecha aceptada (L1-D15).
- **Pendiente de conectar**: `Service.LicenseServerClock` con el cliente de la Fase B (ver «Para el coordinador»).

**Otros**:

- `backend/internal/licensing/licensingtest`: gestores de prueba con licencias firmadas con las llaves de `vectors.json`
  para una instalación de esos archivos, reloj simulado y llave de soporte de prueba. Solo lo importan pruebas.
- `desktop/src/services/auditActions.ts`: los nombres del Registro de movimientos de las acciones nuevas, y también el de
  `license.seat_released` de la Fase D, que faltaba (lo exige `auditActions.test.ts`).
- `docs/licencias/DESBLOQUEO_SOPORTE.md`, con la **propuesta para la V1.1** de pasar el desbloqueo al panel del
  servidor de licencias.

### Decisiones propuestas

El coordinador aceptó las siete del informe como **L1-D35** (unión con la Fase B).

1. **Sin gestor de licencias (`Service.License` nil) la puerta no aplica nada.** Solo las pruebas que no tratan de la
   licencia construyen así el servicio (16 lugares); `serveHTTP` lo crea siempre antes de atender peticiones. Por qué:
   hacerla fallar cerrada obligaba a cambiar las mismas líneas de esas pruebas que cambió la Fase D. Los cupos sí fallan
   cerrados sin fuente (Fase D). Si el revisor prefiere fallar cerrada también aquí, es un cambio pequeño en
   `licenseGate` y en las pruebas.
2. **La hora va antes que el estado de la licencia.** Con los dos problemas, la respuesta es `SYSTEM_CLOCK_INVALID` y
   `details.license_state` permite a la pantalla mostrar los dos por separado (§6.2, punto 5).
3. **«La clase más restrictiva» para una escritura desconocida es «solo Vigente y Tolerancia, con fecha»** (como F1),
   no F7 como decía entre paréntesis 0.5.7: F1 es más restrictiva que F7 (F7 se permite en Vencida y Revocada). La
   prueba de completitud impide que una ruta conocida caiga ahí.
4. **El asistente (`setup`) y el alta local (`devices/enroll`) no se bloquean por la hora.** Son el arranque (L1-D6), no
   registran operaciones del negocio, y sin el alta la computadora principal no podría ni iniciar sesión para ver la
   pantalla de la hora. Los clasifico `RowStartup` sin fecha.
5. **Las tres rutas de la hora** (`time-check`, `support-request`, `support-unlock`): solo por la escucha local y con
   cualquier sesión, sin `license.manage`. La pantalla está en la computadora principal y quien esté en la caja debe
   poder resolverlo; la seguridad del desbloqueo está en la firma y la comprobación con internet solo baja la hora si el
   servidor de licencias coincide con el reloj.
6. **`support-request` entrega la solicitud pendiente** mientras siga vigente (`reused: true`) y solo crea otra con
   `renew`: así refrescar la pantalla no invalida un código que el propietario está procesando.
7. **Las 24 h del desbloqueo también vencen si el reloj retrocede** más de 10 min desde la solicitud (la lectura de
   L1-D19: «si el reloj se corrige en medio, se pide una solicitud nueva»).
8. **El contenido de la respuesta de soporte** (`schema_version` 1.0, `product_id`, `environment` = entorno de
   compilación, `installation_tag`, `request_nonce`, `request_code`, `new_trusted_time`, `issued_at`) se lee estricto:
   un campo desconocido es «alterada». Mide unos 600 caracteres, no 470.
9. **`license.changed` se guarda en el ámbito `configuration`**, que ya existe, y `events.Read` lo entrega a todos por
   su tipo. Por qué: un ámbito nuevo («license») exigía cambiar la restricción de `realtime_events` en la `0028`, y el
   plan de la Fase C es no guardar nada en la base. Los clientes anteriores lo ignoran (Android) o solo vuelven a leer
   (escritorio).
10. **El vigilante revisa cada 15 s** (no cada minuto): el aviso de las terminales aparece y se quita pronto. Solo
    escribe cuando algo cambia.
11. **`POST license/time-check` hace la consulta al servidor de licencias fuera de la transacción** (desde la unión
    con la Fase B, como sus operaciones en línea): la ruta autoriza y `completeDeferred` consulta y audita aparte.
12. **Los mensajes de los tres rechazos de la licencia** dicen qué sigue disponible: «Se puede seguir cobrando lo
    abierto, hacer cortes y consultar reportes. Avisa al encargado.», con la causa según el estado y el motivo de
    revocación.
13. **Piso firmado para la §6.3**: el `server_time` no va firmado (solo lo protege el TLS), así que una hora del
    servidor anterior a la emisión (`issued_at`, firmada) de la licencia vigente no se usa para bajar la hora confiable:
    `502 LICENSE_RESPONSE_INVALID`. No cambia nada en el uso normal: el servidor de licencias emitió esa licencia con su
    propia hora.
14. **Códigos nuevos fuera de la tabla de 0.6.2**: `LICENSE_LOCAL_ONLY` (403), `LICENSE_STATE_NOT_SAVED` (503, no se
    pudo guardar en `identity/licencia`) y `SUPPORT_UNLOCK_UNAVAILABLE` (503, la versión no tiene llave de soporte).

### Para el coordinador (lo que falta conectar o decidir)

Lo que quedaba pendiente con la Fase B (el adaptador de la §6.3, el aviso desde `license/*`, `license.seat_released`)
y la duda de `GET kitchen/board` se resolvieron en la «Unión con la Fase B» (abajo). Sigue en pie:

- **Pruebas de interfaz y Android sobre `etapa-l1`**: con la puerta, un Go temporal sin licencia queda «Sin activar» y
  rechaza la captura, la administración y emparejar. Las suites de Playwright y Android de las fases E, F y H necesitan
  una licencia firmada con las llaves de `vectors.json` para la identidad del Go temporal (`ADEL_TEST_LICENSE_TRUST`, o
  la orden `license-test` de la Fase B).
- **Sin activar no se administra** (tabla de la §6): en una instalación nueva hay que activar antes de crear usuarios,
  catálogo o configuración. Es lo que dice el contrato; la Fase E debe llevar al usuario a la activación primero.

### Unión con la Fase B

Se trajo `etapa-l1` (`a8dd193`: la Fase B con las correcciones de R1) con la fusión `097df8cb`.

- **Conflictos, conservando los dos lados**:
  - `main.go`: el cliente de la Fase B y sus trabajos en segundo plano junto a `useLicense` y el vigilante;
  - `docs/API_V1.md`: la sección «Licencia del servidor principal» (B) y después «Licencia: estado, hora del equipo y
    rechazos» (C);
  - `openapi.json`: se tomó el de la Fase B y se volvieron a aplicar, sobre él, los esquemas y descripciones de la C
    (`LicenseSummary`, `LicenseClock`, `LicenseRejection`, `SystemClockInvalid`, `license` en `Identity` y
    `RecoveryState`, y los 409 de las operaciones de captura).
- **Sin conflicto de texto**: `routes.go` (el bloque de la C va antes de `licenseRoutes` de la B), `service.go` y
  `auditActions.ts` (`license.seat_released` queda una sola vez, la de la B).
- **Ajustes al código de la B** (R1-1 y R1-2):
  - el desbloqueo de soporte escribe `estado.json` con `ensureStateLocked`, como toda escritura después de R1-1;
  - `licensingtest` firma el registro de la huella (`identity/license-machine.json`, R1-2) y entrega la llave de la
    instalación, que firma las pruebas de posesión.

**El adaptador de la §6.3** (`administration.LicenseClientClock`, `license_clock.go`):

- toma `licensing.Client.ServerTime` de la B (refresh con activación; sin ella, el desafío, L1-D5), devuelve su `.Time` y
  traduce sus errores con `licenseFault` (los códigos de la B: `LICENSE_SERVER_UNAVAILABLE`, `LICENSE_SERVER_ERROR`…);
- se conecta en `serveHTTP` (`main.go`), justo después del cliente.
- **La comprobación ya no corre dentro de la transacción**: la ruta solo autoriza y `completeDeferred` hace la consulta
  después de confirmarla, como las operaciones en línea de la B. La auditoría va en una transacción corta aparte. El plazo
  es el de las rutas `license/*` (75 s), con 60 s para la consulta y sus reintentos.

**El aviso desde las rutas `license/*`** (`notifyLicense`, que llama a `CheckLicense` fuera de la transacción):

- después de activar, transferir, buscar actualización y liberar (`completeLicenseOperation`, ya fuera de la transacción);
- después de importar un `.lic` y de crear la identidad nueva: esas rutas devuelven `licenseNotice` y
  `completeDeferred` avisa después de confirmar, sin tomar una segunda conexión con la transacción abierta;
- después de «Comprobar la hora con internet».
- Solo publica si algo cambió (una consulta sin cambios no avisa).

**`GET kitchen/board` pasa a F6** (lectura), como decidió el coordinador: lo lee la vista de una cuenta y entrar en modo
Cocina ya lo controla `kitchen/display-mode` (F5, con cupo). La prueba de completitud lo comprueba.

**Una prueba de la B que cambió**: `TestLicenseRoutes` daba de alta a la cajera sin activar. Con la puerta, la
administración no se permite en «Sin activar» (tabla de la §6): ese bloque se movió a después de activar, sin cambiar lo
que comprueba.

**Pruebas nuevas de la unión**: `TestStageL1LicenseNoticesAndTimeCheckWithLicenseServer` (administration) contra el
servidor simulado de la B (`licensetest`, TLS en loopback):

- `license.changed` después de activar, de una consulta con renovación (una sin cambios no avisa), de liberar, de importar
  y de transferir;
- la §6.3 con el cliente real: coincide (con su `refresh`; la hora confiable baja, el bloqueo se quita, se audita y se
  avisa), un error del servidor de licencias (`502 LICENSE_SERVER_ERROR` de la B) y no coincide (servidor 3 h adelante:
  `409 LICENSE_TIME_MISMATCH` con su hora).

**Pruebas de la unión** (sobre la cabeza `e7ad4adb`, realmente ejecutadas):

| Prueba                                                                                                                                                        | Resultado                                                                                                                                                                                                   |
| ------------------------------------------------------------------------------------------------------------------------------------------------------------- | ----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `ISO_TAG=LC iso.sh … go-unit`                                                                                                                                 | **Aprobado**                                                                                                                                                                                                |
| `go-int ./internal/licensing ./internal/database ./internal/apicontract ./internal/administration ./internal/operations`                                      | **Estado 0** en los cinco (9 s, 16 s, 57 s, 293 s y 531 s)                                                                                                                                                  |
| `go-int ./cmd/ad-el-pos-server`, con el candado (script de bash con `trap` que siempre lo libera)                                                             | **Estado 0** (83 s); candado liberado                                                                                                                                                                       |
| `TestStageL1LicenseNoticesAndTimeCheckWithLicenseServer`, `TestLicenseRoutes` (B, con el bloque de la cajera movido) y `TestStageL1LicenseGateWithoutSession` | Aprobadas (también por separado con `-v`)                                                                                                                                                                   |
| `vitest src/services/auditActions.test.ts`                                                                                                                    | Aprobada con las acciones de la Fase C; antes de unir la Fase B fallaba solo por `license.seat_released` de la Fase D (sin nombre en `etapa-l1`). Después de la unión, aprobada (ver «Unión con la Fase B») |
| `format`, `fmt-check` y `sync-check`                                                                                                                          | Aprobados; MD5 de `docs/etapas/`, el contrato, la guía y `contracts/` iguales                                                                                                                               |
| Go temporal                                                                                                                                                   | No se usó; `run-LC` no existe                                                                                                                                                                               |

### Archivos

- `backend/internal/licensing/`: `operations.go`, `clock.go`, `unlock.go`, `support_trust.go`,
  `support_development.go`, `support_production.go`, `soporte-development.pub.json` (copia), `store.go` (el campo
  `Support`), `licensingtest/licensingtest.go`; pruebas `operations_test.go`, `clock_test.go`, `unlock_test.go` y
  `production_build_test.go`.
- `backend/internal/administration/`: `license_gate.go`, `license_clock.go`, `license_monitor.go`; en `service.go` los
  campos de la Fase C y las dos llamadas de `dispatch`; en `routes.go` `auth/me`, `restaurant/state` y las tres rutas;
  pruebas `license_gate_test.go` (completitud y cupos) y `license_gate_acceptance_test.go`.
- `backend/internal/events/store.go`: `license.changed`.
- `backend/internal/operations/stagel1_license_gate_test.go`: la tabla estado por estado.
- `backend/internal/apicontract/`: `openapi.json` y `contract_license_integration_test.go`.
- `backend/cmd/ad-el-pos-server/`: `main.go` (vigilante y `useLicense`), `license.go` (`useLicense`) y `license_test.go`.
- `backend/cmd/soporte-desbloqueo/main.go` y `main_test.go`; `tools/soporte-desbloqueo/` (envoltura y `README.md`).
- `config/licencias/soporte-development.pub.json` y `scripts/sync-metadata.mjs`.
- `docs/API_V1.md`, `docs/licencias/DESBLOQUEO_SOPORTE.md` y `desktop/src/services/auditActions.ts`.

### Pruebas (realmente ejecutadas)

| Prueba                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                          | Resultado                                                                                                                                                                                                                                                |
| --------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | -------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `TestOperationTableOfTheContract` (`licensing`): las 11 filas × 6 estados de la tabla de la §6                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                  | Aprobada                                                                                                                                                                                                                                                 |
| `TestClassifyOperation` y `TestUnknownOperationsTakeTheMostRestrictiveClass` (75 operaciones representativas; escrituras desconocidas)                                                                                                                                                                                                                                                                                                                                                                                                                                                                          | Aprobadas                                                                                                                                                                                                                                                |
| `TestCheckClockMargin`, `TestManagerClockBlocksAndUnblocks`, `TestScreenTime`, `TestServerTimeMatches` (límite exacto de 10 min, atrasar el reloj nunca baja la hora confiable, textos «lunes 23 nov 2026, 10:15»)                                                                                                                                                                                                                                                                                                                                                                                              | Aprobadas                                                                                                                                                                                                                                                |
| `TestSupportCodeRoundTrip` (dictado con minúsculas, O, I y L; de 744 variantes con un carácter cambiado se aceptan como mucho 2), `TestSupportUnlock` (válido, repetido también tras reiniciar, de otra instalación, vencido a las 24 h y con el reloj hacia atrás, alterado de tres formas, sin solicitud), `TestSupportRequestIsPersisted`, `TestCompiledSupportTrust`                                                                                                                                                                                                                                        | Aprobadas                                                                                                                                                                                                                                                |
| `TestProductionBuildFailsWithoutProductionKeys`: sin llaves no compila; con solo `llaves-production.json` tampoco (falta `soporte-production.pub.json`); con las dos simuladas, compila                                                                                                                                                                                                                                                                                                                                                                                                                         | Aprobada                                                                                                                                                                                                                                                 |
| `cmd/soporte-desbloqueo`: `TestGenerateKey` (0700/0600, nunca reemplaza, la salida no muestra la privada), `TestRespond` (la respuesta verifica, `--salida`, código mal dictado y llave que no es la de la versión), `TestUsageErrors`                                                                                                                                                                                                                                                                                                                                                                          | Aprobadas                                                                                                                                                                                                                                                |
| `responder` con la llave real de desarrollo y un código sintético: firma y verifica con la pública compilada; la salida no tiene «PRIVATE»                                                                                                                                                                                                                                                                                                                                                                                                                                                                      | Correcto (a mano)                                                                                                                                                                                                                                        |
| `TestEveryOperationHasItsLicenseClass` (`administration`): **405 operaciones** (362 del puente `api.rs`, 41 de `openapi.json`, 29 locales) en la tabla, coherentes con `licenseseats.ForOperation`                                                                                                                                                                                                                                                                                                                                                                                                              | Aprobada                                                                                                                                                                                                                                                 |
| `TestLicenseSeatsUseTheLicense` y `TestStartupConnectsSeatsToTheLicense` (`cmd`): los cupos salen del gestor real en cada estado y con su reloj                                                                                                                                                                                                                                                                                                                                                                                                                                                                 | Aprobadas                                                                                                                                                                                                                                                |
| `TestStageL1LicenseGate` (`operations`, API v1 y `Execute`): Sin activar, Vigente, Tolerancia, Vencida (cobrar lo abierto, cerrar y abrir turno con cuentas pendientes, sin ellas 409), Revocada (motivo `transferred`), carpeta copiada y respaldo restaurado («Sin activar con datos»), hora incorrecta (también cobrar; las dos fechas; auditoría y `license.changed`; desbloqueo al corregir), §6.3 (sin cliente 503; no coincide 409; hora anterior a la licencia 502; coincide: baja, auditada con las operaciones futuras, que no cambian), §6.4 por la API y cupos del plan básico con la licencia real | Aprobada (10 subpruebas)                                                                                                                                                                                                                                 |
| `TestStageL1LicenseGateWithoutSession` (`administration`): asistente y entrada sin activar; emparejar 503 sin identidad y 409 con ella; PIN, avisos, `auth/me` y `restaurant/state` con la hora incorrecta; `license.changed` para el mesero; `LICENSE_LOCAL_ONLY` desde la red                                                                                                                                                                                                                                                                                                                                 | Aprobada                                                                                                                                                                                                                                                 |
| `TestContractV1License` (`apicontract`): `license` en `getIdentity` y `getRecoveryState` en cada estado y con la hora incorrecta, `LicenseRejection` y `SystemClockInvalid` validados contra su esquema exacto                                                                                                                                                                                                                                                                                                                                                                                                  | Aprobada                                                                                                                                                                                                                                                 |
| `ISO_TAG=LC iso.sh … go-int ./internal/licensing ./internal/apicontract ./internal/payments ./internal/administration ./internal/operations`                                                                                                                                                                                                                                                                                                                                                                                                                                                                    | **Estado 0** en los cinco (7 s, 51 s, 3 s, 274 s y 556 s)                                                                                                                                                                                                |
| `go-int ./cmd/ad-el-pos-server`, con el candado                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                 | **Estado 0** (67 s); candado liberado                                                                                                                                                                                                                    |
| `ISO_TAG=LC iso.sh … go-unit`                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                   | **Aprobado**, dos veces (la última, sobre la cabeza final)                                                                                                                                                                                               |
| `format`, `fmt-check` y `sync-check`                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                            | Aprobados; MD5 de `docs/etapas/`, el contrato, la guía y `contracts/` iguales antes y después                                                                                                                                                            |
| `vitest src/services/auditActions.test.ts`                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                      | Aprobada con las acciones de la Fase C. **Falla en la cabeza final solo por `license.seat_released` de la Fase D**, que ya estaba sin nombre en `etapa-l1`: lo agrega la Fase B (`7c9ccc85`); no lo dupliqué para que la unión no deje la clave repetida |

### Lo no verificado

- **La §6.3 contra `licensing.sicrat.com`**: solo con el cliente real contra el servidor simulado de la Fase B (y con un
  `server_time` inyectado); la prueba contra el servidor real es de la Fase G.
- **La pantalla de la hora, los avisos y Android**: son de las fases E y F. Aquí solo el backend y el contrato.
- **El host de Windows**: comparte `serveHTTP`; no se ejecutó.
- **Un respaldo restaurado con `pg_restore`**: «Sin activar con datos» se probó con la misma base y una identidad sin
  licencia, que es lo que ve una computadora nueva.
- **La revisión independiente R2** de la protección de la hora: pendiente, según el plan.
- **Un límite conocido** (contrato §12): quien administra la Mac puede atrasar el reloj y, con un servidor falso con un
  certificado en el que confíe el sistema, bajar la hora confiable con la §6.3 hasta la emisión de la licencia vigente
  (el piso de la Decisión propuesta 13). Va para la revisión R2.

### Commits (rama `etapa-l1-c`)

- `63e24665` etapa L1 fase C: tabla de operaciones de la §6 (fila, fecha y lo desconocido) y regla de la hora con su margen de 10 minutos
- `ce7a5cad` etapa L1 fase C: desbloqueo de soporte (código de solicitud, respuesta firmada, solicitud pendiente en estado.json y llave pública de desarrollo embebida)
- `ecc09a68` etapa L1 fase C: orden soporte-desbloqueo (generar-llave y responder) para el propietario
- `e33335a9` etapa L1 fase C: licensingtest, gestores de licencia de prueba con las llaves de vectors.json y reloj simulado
- `fe5fbcef` etapa L1 fase C: puerta de la licencia y de la hora en dispatch, rutas license/time-check y support-*, vigilante con license.changed y estado de la licencia en auth/me y restaurant/state
- `ed713823` Merge branch 'etapa-l1' into etapa-l1-c (la Fase D; conflicto en los campos de `Service`, se conservaron los dos lados)
- `b377a4d2` etapa L1 fase C: cupos con los límites reales de la licencia y el reloj del gestor (LimitsSource), clasificación de los cupos en la puerta y prueba de completitud
- `9ba687b2` etapa L1 fase C: prueba de la puerta sin sesión (asistente, emparejamiento, PIN, avisos y license.changed para todos)
- `59513af8` etapa L1 fase C: contrato v1 con el estado de la licencia (LicenseSummary y LicenseClock), LicenseRejection, SystemClockInvalid y license.changed, con su prueba de contrato
- `f0da7d0f` etapa L1 fase C: envoltura tools/soporte-desbloqueo y docs/licencias/DESBLOQUEO_SOPORTE.md con la propuesta para la V1.1
- `1c4badac` etapa L1 fase C: nombres del Registro de movimientos para la hora del equipo, el desbloqueo de soporte y los cupos
- `8ec938df` etapa L1 fase C: la comprobación con internet no usa una hora del servidor anterior a la emisión de la licencia vigente
- `1b2670e2` etapa L1 fase C: el nombre de license.seat_released queda en la Fase B, que también lo agrega
- `097df8cb` Merge branch 'etapa-l1' into etapa-l1-c (la Fase B con R1; conflictos en `main.go`, `openapi.json` y `docs/API_V1.md`, conservando los dos lados)
- `3201783a` etapa L1 fase C: unión con la Fase B: el desbloqueo de soporte escribe con la regla de R1-1 y licensingtest firma el registro de la huella
- `8cfb6b7b` etapa L1 fase C: «Comprobar la hora con internet» fuera de la transacción con el cliente de la Fase B (LicenseClientClock en serveHTTP) y license.changed después de activar, transferir, consultar, liberar, importar y la identidad nueva
- `e7ad4adb` etapa L1 fase C: GET kitchen/board es una lectura (F6); entrar en modo Cocina sigue en F5 con cupo

### Decisiones del coordinador sobre la Fase C

Fusionada en `91f17ba`, con la Fase B ya unida en su rama. `go-unit` pasó sobre el árbol fusionado.

- **L1-D35 · Se aceptan las siete decisiones propuestas del agente.**
  - Sin `Service.License`, la puerta no aplica nada. Solo pasa en pruebas: `serveHTTP` siempre pone el gestor.
  - La hora se revisa antes que el estado.
  - Una escritura desconocida toma la fila F1.
  - `setup` y `devices/enroll` no se bloquean por la hora.
  - Las rutas de la hora solo van por la escucha local.
  - La §6.3 tiene un piso firmado.
  - `license.changed` usa el ámbito `configuration`.
- **`GET kitchen/board` es una lectura (F6).** Entrar en modo Cocina lo controla `kitchen/display-mode` (F5, con cupo).
- **Desde esta fusión, `etapa-l1` ya no rechaza toda captura.** Los cupos toman los límites de la licencia real. El Go
  temporal de las pruebas de interfaz y de Android necesita una licencia de prueba (`license-test issue`).

---

## Fase G · Pruebas contra `licensing.sicrat.com` (entorno `test`)

Lo hizo el agente de la Fase G en `$SP/wt/l1-g`, rama `etapa-l1-g`.

- **Árbol**: empezó en `a8dd193` (A, B y D fusionadas, con R1). Para los cupos se adelantó a `f32d740` (Fase C fusionada)
  con `git merge --ff-only etapa-l1`.
- **Sin emuladores y sin commits.** Los ayudantes llaman al servidor real, así que viven en `$SP/l1/fase-g/` y no en el
  repositorio. El árbol quedó sin cambios.
- El trabajo se hizo en dos tandas: la tarde del 23/11 (pruebas 1 a 6, 9 y 11 a 14) y la noche del 24/11 UTC (cupos,
  prueba 14 completa y liberar). En medio hubo un corte por el límite de uso (ver «Comprobación al volver del corte»).

**Las reglas de la fase:**

- **Los códigos** se leyeron solo dentro de `g.py` y nunca se imprimieron ni se escribieron. Aquí se nombran «la
  Perpetua», «la Mensual» y «la Prueba».
- **Las instancias de desarrollo** salieron de `iso.sh server-up`, con una base `adel_pos_test_lg*_…` del clúster 55499.
  - Cada una tiene su `identity/` y su llave de licencias propias. «Otra computadora» es otra identidad (`server_id` +
    llave, §2 del contrato).
  - La huella es la misma Mac en todas, así que ninguna queda como copiada.
  - Para volver a levantar una instancia se usó `levantar.sh`, que conserva la identidad, y nunca otra vez
    `server-up`.
- **Nunca se tocó** la instalación real (8181/8443, 55432, `/Library/Application Support/AD-EL POS`), la hora de la
  Mac, el contrato ni `contracts/`. Los MD5 de `docs/etapas/`, el contrato, la guía y `contracts/` son iguales; solo se
  agregó `DESBLOQUEO_SOPORTE.md`, de la Fase C.
- **El reloj**: cada instancia arrancó con `ADEL_TEST_CLOCK_FILE=$SP/l1/fase-g/reloj-<TAG>` vacío. Los tres
  archivos quedaron vacíos.
- **La consulta en segundo plano** corre de 1 a 5 min después de arrancar.
  - Antes de la cadena de la Prueba se esperó a que ese primer turno pasara sin activación en las tres instancias. Así
    ninguna consulta automática de la computadora anterior podía confirmar la transferencia antes de la prueba 5.
  - En M y P sí hubo consultas automáticas, inofensivas: la misma revisión.

### Ayudantes (en `$SP/l1/fase-g/`; copia para el propietario en `~/.adel-pos-dev/instancias-l1/herramientas/`)

| Archivo                      | Qué hace                                                                                                                                                                                                                                         |
| ---------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------ |
| `up.sh`                      | `iso.sh server-up` con `ISO_LICENSE_URL=https://licensing.sicrat.com`, `ISO_ALLOW_REMOTE=1` e `ISO_CLOCK_FILE` (identidad nueva)                                                                                                                 |
| `levantar.sh`                | Vuelve a levantar una instancia **con su misma identidad** y una base nueva (`up`, `down` y `pg-down`). `--en-linea` permite el servidor de licencias; `--reloj` usa el reloj simulado. Rechaza 8181 y 8443                                      |
| `g.py`                       | La API local de una instancia: `setup`, `status`, `activate` y `transfer <perpetua\|mensual\|prueba\|inexistente>`, `refresh`, `deactivate`, `licreq`, `import`, `savelic` y `raw`. Lee los códigos dentro del proceso y **redacta** toda salida |
| `cupos.py`                   | Los cupos reales de la licencia instalada, por la API v1 y con terminales de escritorio enroladas (invitación y `devices/enroll`)                                                                                                                |
| `raw/`                       | Programa Go (solo la biblioteca estándar) que habla directo con el servidor de licencias con la llave de una instancia: `notfound` (prueba 11), `retry` y `conflict` (prueba 12) y `consult` (una consulta sin instalar la respuesta)            |
| `alter.py`                   | Un `.lic` con la firma alterada, o con los cupos subidos sin volver a firmar (prueba 13)                                                                                                                                                         |
| `conservar.sh`               | Copia una instancia apagada (`identity/` y la configuración, sin la base) a `~/.adel-pos-dev/instancias-l1/<nombre>/` (0700)                                                                                                                     |
| `scan.py` y `auditoria.py`   | Buscan los códigos, sin mostrarlos, en archivos y en toda `app.audit_events`. `auditoria.py` además lista la auditoría `license.*`                                                                                                               |
| `resultados*.jsonl` y `lic/` | Lo que se obtuvo, ya redactado, y las copias de cada `.lic` (sin códigos)                                                                                                                                                                        |

### Instancias

| Nombre             | TAG y puertos   | `server_id`  | Uso                                                                                | Estado final                                                                                          |
| ------------------ | --------------- | ------------ | ---------------------------------------------------------------------------------- | ----------------------------------------------------------------------------------------------------- |
| A                  | LG1 · 8211/8481 | `b1ae86fa-…` | La Prueba: la primera computadora (pruebas 1, 2, 6 y 13)                           | «Revocada» (`transferred`), rev. 2. Ya no existe                                                      |
| B                  | LG2 · 8215/8485 | `60453b51-…` | La Prueba: transferencia y liberación (3, 4, 9 y 13)                               | «Revocada» (`deactivated`), rev. 4. Ya no existe                                                      |
| C                  | LG3 · 8217/8487 | `33fe13ad-…` | La Prueba: `TRANSFER_LIMIT`, confirmación, activar tras liberar, cupos y prueba 14 | La Prueba liberada (`deactivated`), rev. 6. Hora confiable adelantada: no se conserva                 |
| `sin-internet` (S) | LG1             | `8f54fcd0-…` | El `.licreq` de `activate` con el código de la Mensual (prueba 10)                 | Sin activar. **Conservada**                                                                           |
| M                  | LG1             | `c5492804-…` | La Mensual: activar, consultar y `renew`/`deactivate`; pruebas 12, 14 y cupos      | La Mensual liberada (`deactivated`), rev. 2. **Conservada** (`~/.adel-pos-dev/instancias-l1/mensual`) |
| `en-linea` (O)     | LG2             | `6230543c-…` | Para el propietario: pruebas 7 y 8 en línea                                        | Sin activar. **Conservada**                                                                           |
| P                  | LG2             | `0ced796e-…` | La Perpetua: activar, cupos y liberar                                              | La Perpetua liberada (`deactivated`), rev. 2                                                          |

- Todo quedó apagado. Las bases temporales se borraron al apagar.
- `~/.adel-pos-dev/instancias-l1/` (0700) tiene:
  - `sin-internet/`, `en-linea/` y `mensual/`: cada una con `identity/`, `server.json` sin contraseña, `backups/` y su
    `server.log`;
  - `herramientas/`: `levantar.sh`, `g.py` y `cupos.py`;
  - `bin/ad-el-pos-server`: la compilación de desarrollo de `f32d740`.

### Plan y cupos de cada licencia

| Licencia    | Tipo                                 | Plan                         | Cajas | Comanderos | KDS |
| ----------- | ------------------------------------ | ---------------------------- | ----- | ---------- | --- |
| La Perpetua | `perpetual` (`billing_period: null`) | Plan básico (`basico`)       | 1     | **0**      | 0   |
| La Mensual  | `subscription`, `monthly`            | Plan premium (`premium`)     | 5     | 10         | 5   |
| La Prueba   | `trial` (`billing_period: null`)     | Plan elemental (`elemental`) | 3     | 3          | 3   |

**Las fechas que firmó el servidor:**

- **La Perpetua**:
  - `expires_at: null` y `grace_days` 0;
  - `period_starts_at` es la activación (23/11/2026 21:28:31Z);
  - `updates_until` es el 23/05/2027: 6 meses, el valor por omisión.
- **La Mensual**:
  - `period_starts_at` es la activación (23/11/2026 21:26:46Z);
  - `expires_at` es el 23/12/2026 21:26:46Z: un mes de calendario;
  - `grace_days` 5, así que la tolerancia termina el 28/12/2026 21:26:46Z;
  - `updates_until` = `expires_at`.
- **La Prueba**:
  - `period_starts_at` es la primera activación (23/11/2026 21:24:07Z);
  - `expires_at` es el 23/02/2027 21:24:07Z: una prueba de 3 meses;
  - `grace_days` 0, y `updates_until` = `expires_at`.

**Lo común a las tres:**

- `custom: false`, `features: {}`, `kid` `adelpos-test-202611-1` y `environment: test`;
- ninguna se había activado antes, así que la primera activación empezó su primer periodo (§5.2);
- **el plan con 0 comanderos es el de la Perpetua.**

### Resultados de la §7 de la guía

#### Pruebas 1 a 6: la cadena de la Prueba (A, B y C)

1. **Activar por primera vez (A). Aprobada.**
   - **Esperado**: revisión 1, con las fechas del servidor.
   - **Obtenido**: 200, rev. **1**, `active` («Vigente»), con las fechas de arriba.
   - Que la computadora aparezca en «Computadoras y transferencias» del panel lo revisa el propietario.
2. **Activar otra vez desde A. Aprobada.**
   - **Esperado**: la misma revisión, sin otra activación.
   - **Obtenido**: 200, rev. 1, el mismo `activation_id`. El `vigente.lic` es **idéntico byte por byte** (el mismo
     SHA-256).
3. **Activar desde B. Aprobada.**
   - **Esperado**: `409 ACTIVATION_LIMIT` con `transfer_available: true`.
   - **Obtenido**: el servidor responde `ACTIVATION_LIMIT`, y AD-EL POS 409 `LICENSE_ACTIVATION_LIMIT` con
     `{transfer_available: true}`. B sigue «Sin activar».
4. **Transferir a B. Aprobada.**
   - **Esperado**: B activa, con las mismas fechas, cupos, plan y módulos; la transferencia queda pendiente.
   - **Obtenido**: 200, rev. **3**, activación nueva. Son iguales a las de A: `expires_at`, `period_starts_at`,
     `updates_until`, plan, cupos y `features`. Solo cambia `issued_at`.
   - A sigue con su rev. 1: todavía no se entera.
5. **Transferir otra vez antes de que A consulte (desde C). Aprobada.**
   - **Esperado**: `409 TRANSFER_LIMIT` y una alerta en el panel.
   - **Obtenido**:
     - antes, activar desde C da `ACTIVATION_LIMIT` con **`transfer_available: false`**;
     - transferir: el servidor responde `TRANSFER_LIMIT`, y AD-EL POS 409 `LICENSE_TRANSFER_LIMIT` («Comunícate con
       soporte»).
   - La alerta del panel la revisa el propietario.
6. **Consultar desde A. Aprobada.**
   - **Esperado**: su cierre `revoked` / `transferred`, y la transferencia confirmada.
   - **Obtenido**: 200, rev. **2**, `revoked` / `transferred`: «Revocada» con motivo `transferred`.
   - Confirmada: después, activar desde C vuelve a dar `ACTIVATION_LIMIT` con **`transfer_available: true`**.

#### Pruebas 7, 8 y 10: las hace el propietario

- **7**, renovar o cambiar de plan en el panel y consultar. Se espera una revisión mayor, que AD-EL POS acepta.
- **8**, revocar en el panel y consultar. Se espera el cierre `revoked` y el estado «Revocada».
- **10**, sin internet. Los `.licreq` ya están generados y guardados (ver «`.licreq`»); el propietario los aprueba y
  AD-EL POS importa cada `.lic`.
- Los pasos están en «Lista para el propietario».

#### Prueba 9: liberar y activar en otra computadora (B y C). Aprobada

- **Esperado**: el cierre `deactivated`, y la otra computadora activa sin soporte.
- **Obtenido**:
  - B: 200, rev. **4**, `revoked` / `deactivated`: «Revocada» con motivo `deactivated`;
  - C: 200, rev. **5**, activa, con las mismas fechas de la licencia.

#### Prueba 11: un código inexistente o mal escrito. Aprobada

- **Esperado**: `404 LICENSE_NOT_FOUND`, igual en todos los casos.
- **Por AD-EL POS**: dos códigos inexistentes, con el formato correcto, dan 404 `LICENSE_KEY_NOT_FOUND`. El servidor
  responde `LICENSE_NOT_FOUND`.
- **Directo al servidor** (`raw`), cuatro variantes:
  - inexistente;
  - inexistente en minúsculas y sin guiones;
  - con símbolos fuera del alfabeto y corto;
  - de 40 símbolos.
- Las cuatro dan **404 `LICENSE_NOT_FOUND` con el mismo cuerpo** y `details: {}`. Solo cambia el `request_id`, que es
  el que se envió.
- Un código con un formato equivocado nunca sale de AD-EL POS: responde 400 `LICENSE_KEY_INVALID_FORMAT` (L1-D4).

#### Prueba 12: reintento y `REQUEST_CONFLICT`. Aprobada, con una observación

- **Esperado**: el reintento recibe la misma respuesta; después, `409 REQUEST_CONFLICT`.
- **Obtenido**, con `raw` y el `refresh` de M:
  - **el mismo cuerpo dos veces**: 200 y 200, con la **respuesta idéntica byte por byte** (el mismo `server_time`);
  - **el mismo `request_id`, el mismo desafío y la misma prueba, con otro `app_version`**: **409
    `REQUEST_CONFLICT`**;
  - **el primer cuerpo otra vez**, después del conflicto: idéntico al primero.
- **La observación**: el mismo `request_id` con **otro desafío** (y su prueba válida) responde **401
  `INVALID_PROOF`**, no `REQUEST_CONFLICT`. Es G-1, en «Defectos y propuestas».

#### Prueba 13: un `.lic` alterado, de otra instalación o con una revisión menor. Aprobada

- **Esperado**: se ignora y sigue la última licencia válida.
- **Obtenido** (los rechazos son 400 `LICENSE_FILE_INVALID`):

  | Archivo                                         | `details.reason`     |
  | ----------------------------------------------- | -------------------- |
  | Con la firma alterada                           | `invalid_signature`  |
  | Con los cupos subidos a 999 y la firma original | `invalid_signature`  |
  | El de B, importado en C y en A                  | `other_installation` |
  | La rev. 1 de A, en A (que ya tenía la rev. 2)   | `stale_revision`     |
  | La rev. 3 de B, en B (con la rev. 4)            | `stale_revision`     |
  | La rev. 1 de M, en M conservada (con la rev. 2) | `stale_revision`     |

- En todos los casos el estado, la revisión y `vigente.lic` (con el mismo SHA-256) no cambiaron.
- Importar en M el mismo cierre que ya tenía (rev. 2) da 200 y no cambia nada.

#### Prueba 14: atrasar el reloj. Aprobada

- **Esperado**: la licencia no se prolonga.
- **1) Atrasar (M, la Mensual)**: reloj simulado 30 días y 1 año atrás.
  - `effective_now` se queda en la última hora confiable, de unos segundos antes.
  - Sigue «Vigente», con el mismo `expires_at` y el mismo fin de la tolerancia: no gana días.
- **2) Volver atrás después de vencer (C, la Prueba, con la Fase C)**:
  - Reloj **93 días adelante**: «Vencida», y capturar da 409 `LICENSE_EXPIRED` con `expires_at` y `grace_ended_at`.
  - Al minuto, `estado.json` guardó la hora confiable (25/02/2027 04:16:31Z).
  - **Reloj de vuelta a la hora real**: **sigue «Vencida»**, con `effective_now` en la hora confiable. Capturar da 409
    **`SYSTEM_CLOCK_INVALID`**, con:
    - `local_time_text` «lunes 23 nov 2026, 22:17» y `last_activity_time_text` «miércoles 24 feb 2027, 22:16», en
      America/Monterrey;
    - `margin_minutes` 10, `license_state: expired` y `license_ok: false`.
  - `auth/me` trae el mismo `license.clock` y `sessions-summary` (una lectura) responde 200.
  - Se auditó `license.clock_blocked`.
  - **Al reiniciar** (otra base, la misma identidad): sigue «Vencida» y bloqueada.
  - Liberar la licencia sí se pudo así (fila F8).

**La Mensual, además** (§9.2 de la especificación):

- «Buscar actualización» (`license/refresh`) da 200 con la misma rev. 1 y su vencimiento informado: `expires_at`
  23/12/2026 21:26:46Z y fin de la tolerancia el 28/12/2026 21:26:46Z.
- El `.licreq` de `renew` se generó y se guardó; también uno de `deactivate`.

**La Perpetua**: se activó (rev. 1, «Vigente», perpetua) y se liberó (rev. 2, `deactivated`). **Nunca se transfirió.**

### Cupos reales de cada plan (con la Fase C, por la API v1)

Las pruebas usaron `cupos.py`, con terminales de escritorio enroladas (invitación y `devices/enroll`) y el usuario
administrador. Cada instancia tenía su licencia real, una base nueva y el binario de `f32d740`.

- **Capturar** es abrir una cuenta para llevar, sin turno.
- **Cocina** es `POST kitchen/display-mode {active: true}`.
- **Caja** es abrir un turno, con una caja física propia asignada en Terminales.
- Además, **otra terminal de la caja física 1** (que ya tiene turno) intenta capturar (L1-D3).
- En la tabla, «409 _cupo_ N de N» es `LICENSE_SEAT_UNAVAILABLE` con ese `details.seat`, `in_use` y `limit`.

| Licencia y plan              | Comanderos                                          | Cocina                                              | Cajas                                                                                                                                                       | Resultado |
| ---------------------------- | --------------------------------------------------- | --------------------------------------------------- | ----------------------------------------------------------------------------------------------------------------------------------------------------------- | --------- |
| La Perpetua, básico (1/0/0)  | La 1.ª ya se rechaza: 409 `order_takers` **0 de 0** | La 1.ª ya se rechaza: 409 `kitchen_displays` 0 de 0 | Turno en la caja 1 sí. La 2.ª: 409 `cash_registers` 1 de 1. La caja 1 con su turno captura. La otra terminal de su caja física: 409 `cash_registers` 1 de 1 | Aprobada  |
| La Prueba, elemental (3/3/3) | 3 capturan; la 4.ª: 409 `order_takers` 3 de 3       | 3 entran; la 4.ª: 409 `kitchen_displays` 3 de 3     | 3 turnos; el 4.º: 409 `cash_registers` 3 de 3. La caja 1 captura; la otra de su caja física: 409 3 de 3                                                     | Aprobada  |
| La Mensual, premium (5/10/5) | 10 capturan; la 11.ª: 409 `order_takers` 10 de 10   | 5 entran; la 6.ª: 409 `kitchen_displays` 5 de 5     | 5 turnos; el 6.º: 409 `cash_registers` 5 de 5. La caja 1 captura; la otra de su caja física: 409 5 de 5                                                     | Aprobada  |

**Los mensajes**, en español y sin el `message` del servidor de licencias:

- **0 comanderos**: «La licencia no incluye comanderos (0 de 0): solo se capturan pedidos en una terminal con el turno
  de caja abierto.»
- **0 KDS**: «La licencia no incluye pantallas de cocina (0 de 0): Preparación no está disponible.»
- **Comanderos llenos**: «No hay cupo de comandero libre (3 de 3 en uso). Pide que se cierre la sesión de otra
  terminal.»
- **Cocina llena**: «No hay cupo de pantalla de cocina libre (5 de 5 en uso). Pide que otra terminal salga de
  Preparación.»
- **Cajas llenas**: «No hay cupo de caja libre (1 de 1 en uso). Se libera al cerrar un turno de caja.»

`sessions-summary` mostró en cada paso `seats` con su `in_use` y su `limit`, y `limited: true`.

### `.licreq` guardados en `~/.adel-pos-dev/licreq/` (carpeta 0700, archivos 0600)

| Archivo                                | Instancia                                            | Acción       | ¿Lleva código?                             | `request_id` | Qué hacer                                                         |
| -------------------------------------- | ---------------------------------------------------- | ------------ | ------------------------------------------ | ------------ | ----------------------------------------------------------------- |
| `sin-internet-activate-mensual.licreq` | `sin-internet` (`8f54fcd0-…`)                        | `activate`   | Sí, el de la Mensual (lo pide el contrato) | `fce3d2fd-…` | Prueba 10: activar sin internet (lista, paso 10.1)                |
| `mensual-renew.licreq`                 | `mensual` (M, `c5492804-…`), activación `8aa5b416-…` | `renew`      | No                                         | `e45065a1-…` | §9.2. Al aprobarlo, el panel entrega el cierre (lista, paso 10.4) |
| `mensual-deactivate.licreq`            | `mensual` (M), activación `8aa5b416-…`               | `deactivate` | No                                         | `df4bf50b-…` | Lo mismo, con la acción de liberar (paso 10.4)                    |

- AD-EL POS comprueba cada uno con `VerifyOfflineRequest` antes de entregarlo. Sus campos son los de la §9.1:
  - `renew` y `deactivate` llevan `license_id` y `activation_id`;
  - `activate` lleva `license_key`.
- Un `renew` o un `deactivate` con código no se genera: AD-EL POS responde 400 («El código de la licencia solo va en la
  solicitud de activación.»).
- **Por qué los de M entregan el cierre**: los dos se generaron con la Mensual activa en M, pero la Mensual se liberó
  en línea al terminar, como pide la fase. Su activación ya terminó.
  - Según la §9.1, el panel los atiende y entrega la **revisión de cierre** (rev. 2, `deactivated`).
  - M la importa sin cambios: ya la tiene, y se comprobó con el mismo archivo.
- **El `renew` y el `deactivate` de `sin-internet`** solo se pueden generar después de importar su activación: llevan su
  `activation_id`. Los genera el propietario con `g.py` (pasos 10.2 y 10.3), sin internet.

### Lo que quedó en el panel

- **Ninguna licencia tiene una activación vigente**:
  - la Prueba quedó liberada desde C (rev. 6);
  - la Mensual, desde M (rev. 2);
  - la Perpetua, desde P (rev. 2).
- **Ninguna transferencia por confirmar.** La de la Prueba (A → B, autoservicio) se confirmó en la prueba 6.
- **Una alerta por revisar**: el intento bloqueado con `TRANSFER_LIMIT`, desde C (`33fe13ad-…`), el 23/11/2026 hacia las
  21:24:38Z.
  - No debería haber alerta de «dos o más transferencias en 12 meses»: hubo una sola.
- **Historial de computadoras**:
  - **la Prueba**: A `b1ae86fa-…` (transferida), B `60453b51-…` (liberada) y C `33fe13ad-…` (liberada);
  - **la Mensual**: M `c5492804-…` (liberada);
  - **la Perpetua**: P `0ced796e-…` (liberada).
- **Ningún `.licreq` importado todavía.**
- **Límites usados**: el de código (10 cada 15 minutos) llegó a 8 con la Prueba en la cadena 1 a 6 y 9. El resto quedó
  muy por debajo. No hubo ningún `RATE_LIMITED`.

### Defectos y propuestas para la V1.1

**G-1 · El mismo `request_id` con otro desafío responde `INVALID_PROOF`, no `REQUEST_CONFLICT`** (servidor de licencias,
§8.7).

- **Qué dice el contrato**: «Reutilizar un `request_id` con otro contenido … recibe `409 REQUEST_CONFLICT`».
- **Qué hace el servidor**: primero verifica la prueba nueva contra el mensaje **guardado** del primer intento
  (`ActivationService::authenticateSaved`).
  - Con otro desafío, la prueba no verifica aunque sea válida para la misma llave: `401 INVALID_PROOF`.
  - Solo con el mismo desafío y la misma prueba, y otro dato, responde `REQUEST_CONFLICT`.
- **Efecto en AD-EL POS: ninguno.** El cliente reintenta siempre con el mismo cuerpo, y los dos códigos se muestran
  como `LICENSE_SERVER_ERROR`.
- **Propuesta para la V1.1** (que decida el propietario), una de dos:
  - que el servidor responda `REQUEST_CONFLICT` también en ese caso;
  - o que el contrato aclare que un `request_id` reutilizado sin la prueba del original responde `INVALID_PROOF`.
- Parece una decisión de seguridad deliberada (no revelar que un `request_id` existe sin la prueba del original), así
  que lo más probable es aclararlo en el contrato.

**Observación (AD-EL POS, no es un defecto).** `POST license/transfer` no exige un `ACTIVATION_LIMIT` previo: eso lo
hace la pantalla (Fase E). Si se llama sin que la licencia esté activa en otra computadora, el servidor la trata como
una activación normal (§8.4, regla 4), así que no hay daño.

Fuera de G-1, el servidor cumplió el contrato en todo lo probado.

### Seguridad de los códigos

- **Archivos**: `scan.py` revisó 141 archivos y **ninguno contiene un código**. Revisó:
  - `$SP/l1/fase-g/`, `doc-G.md` y `estado-G.md`;
  - `$SP/run-LG1..3/`: configuración, identidad y `server.log`;
  - `~/.adel-pos-dev/instancias-l1/`.
- **Auditoría**: `auditoria.py` revisó seis bases: las de M, P y C de la primera tanda, y las de M, P y C (esta, la del
  reinicio de la prueba 14) de la segunda. **Ningún** evento de `app.audit_events` contiene un código. La base de C con
  los cupos se borró al reiniciarla, antes de revisarla; esos cupos no usan códigos.
  - La licencia deja:
    - `license.activated`, `license.refreshed`, `license.deactivated` y `license.offline_request_created`;
    - `license.activation_failed` (`LICENSE_KEY_NOT_FOUND`, `LICENSE_ACTIVATION_LIMIT`) y `license.transfer_failed`
      (`LICENSE_TRANSFER_LIMIT`);
    - `license.import_rejected` (`LICENSE_FILE_INVALID`), `license.state_changed` y `license.clock_blocked`.
  - Cada uno lleva el código de error y la revisión, nunca el código comercial.
  - `app.license_revisions` tiene su fila (R1-3).
- **Bitácora**: `server.log` solo deja `license_server_ok` y `license_server_error`, con la acción, el código de error
  y la revisión.
- **El `.licreq` de `activate`** sí lleva el código de la Mensual, como manda el contrato. Está solo en la carpeta del
  propietario (0700/0600).

### Comprobación al volver del corte (24/11/2026, 04:10Z)

Después del corte por el límite de uso se hizo **una sola consulta** (`raw consult`, sin instalar la respuesta) de
cada activación vigente. Así no se repitió nada ni se gastó el límite por código.

- **Qué tenía el servidor**: la Prueba en C (rev. 5), la Mensual en M (rev. 1) y la Perpetua en P (rev. 1), todas
  `active`. Eran las mismas revisiones instaladas.
- **Ninguna transferencia pendiente.**

Después:

- se trajo `f32d740`;
- se levantaron M, P y C con `levantar.sh` (la misma identidad, una base nueva y el binario nuevo);
- se hicieron los cupos y la prueba 14 completa;
- se liberaron las tres.

### Lista para el propietario

Todo se hace en la Mac con las instancias conservadas, sin tocar la instalación real.

- **Dónde está todo**: `H=~/.adel-pos-dev/instancias-l1`.
- **Cómo se levanta una instancia**: `$H/herramientas/levantar.sh up <carpeta> <puerto> <puerto de red>`.
  - Usa el clúster de pruebas 55499. Si el scratchpad se borró, crea uno propio en `$H/pg`; se apaga con
    `levantar.sh pg-down`.
  - Con una base nueva hay que correr `g.py <carpeta> setup` (crea el establecimiento y la sesión de `admin`).
- **Los resultados** quedan en `$H/herramientas/resultados.jsonl`, redactados.
- **Los códigos** los lee `g.py` del archivo del propietario: nunca hay que escribirlos.

**Pruebas 7 y 8 (en línea, con la Prueba, instancia `en-linea`).**

1. Levantar y activar:

   ```sh
   $H/herramientas/levantar.sh up $H/en-linea 8211 8481 --en-linea
   python3 -I $H/herramientas/g.py $H/en-linea setup
   python3 -I $H/herramientas/g.py $H/en-linea activate prueba
   ```

   Se espera 200, «Vigente», una revisión mayor que 6, Plan elemental y vence el 23/02/2027 21:24:07Z. El panel
   muestra la computadora `6230543c-…`.

2. **Prueba 7.** En el panel, cambiar el plan de la Prueba (en una prueba es libre y no cambia las fechas), por
   ejemplo a Plan premium, o convertirla. Después, «Buscar actualización»:

   ```sh
   python3 -I $H/herramientas/g.py $H/en-linea refresh
   ```

   Se espera 200, una **revisión mayor** con los cupos nuevos, aceptada y «Vigente». Para ver los cupos nuevos en
   uso: `python3 -I $H/herramientas/cupos.py $H/en-linea`.

3. **Prueba 8, al final.** En el panel, **revocar** la Prueba. Después, «Buscar actualización» otra vez con el mismo
   `refresh` del paso 2.
   - Se espera el cierre `revoked` / `revoked`: «Revocada», con `revocation_reason: revoked`.
   - Opcional: `g.py $H/en-linea activate prueba` debe dar 409 `LICENSE_SERVER_REVOKED` (el servidor responde
     `REVOKED`).
4. Apagar: `$H/herramientas/levantar.sh down $H/en-linea`.

**Prueba 10 (sin internet, con la Mensual, instancia `sin-internet`; y M).**

1. **El `activate`.**
   - En el panel, en **Sin internet**, importar `~/.adel-pos-dev/licreq/sin-internet-activate-mensual.licreq`.
   - Revisar que corresponde a la Mensual (computadora `8f54fcd0-…`), aprobarla y descargar el `.lic`. Por ejemplo,
     guardarlo como `~/.adel-pos-dev/licreq/sin-internet-activate.lic`.
   - Levantar `sin-internet` **sin** `--en-linea`, así que no puede hablar con el servidor, e importar el `.lic`:

     ```sh
     $H/herramientas/levantar.sh up $H/sin-internet 8211 8481
     python3 -I $H/herramientas/g.py $H/sin-internet setup
     python3 -I $H/herramientas/g.py $H/sin-internet import ~/.adel-pos-dev/licreq/sin-internet-activate.lic
     ```

   - Se espera 200, «Vigente», la Mensual con una revisión mayor que 2 y las fechas de la licencia: vence el
     23/12/2026 21:26:46Z. Las fechas son de la licencia, no de la activación.
2. **El `renew`.** Generarlo con la instancia encendida:

   ```sh
   python3 -I $H/herramientas/g.py $H/sin-internet licreq renew sin-internet-renew
   ```

   - Importar `~/.adel-pos-dev/licreq/sin-internet-renew.licreq` en Sin internet y aprobarlo. Opcional: registrar ahí
     una renovación con su pago.
   - Importar el `.lic` con `g.py … import`.
   - Se espera una revisión mayor, con el nuevo `expires_at` si se renovó. Sin renovar llega la misma revisión, que se
     acepta sin cambios.

3. **El `deactivate`.** Generarlo y aprobarlo:

   ```sh
   python3 -I $H/herramientas/g.py $H/sin-internet licreq deactivate sin-internet-deactivate
   ```

   - Aprobar el archivo e importar el `.lic`.
   - Se espera «Revocada», con `revocation_reason: deactivated`. **La Mensual queda libre otra vez.**
   - Apagar: `levantar.sh down $H/sin-internet`.

4. **Los dos de M** (`mensual-renew.licreq` y `mensual-deactivate.licreq`).
   - Importarlos y aprobarlos en Sin internet. Como esa activación ya terminó, el panel debe entregar **el cierre
     existente** (rev. 2, `deactivated`) sin firmar otro (§8.6 y §9.1).
   - Importarlo en M:

     ```sh
     $H/herramientas/levantar.sh up $H/mensual 8215 8485
     python3 -I $H/herramientas/g.py $H/mensual setup
     python3 -I $H/herramientas/g.py $H/mensual import <el .lic>
     ```

   - Se espera 200, «Revocada» (`deactivated`), rev. 2, sin cambios.
   - Si el panel firmara otra revisión, AD-EL POS la aceptaría por ser mayor, pero sería una desviación de la §9.1 que
     conviene anotar.
   - Apagar: `levantar.sh down $H/mensual`.

**Además, en el panel:**

- **la Prueba**: «Computadoras y transferencias» muestra A, B y C, y la transferencia confirmada (pruebas 1, 4 y 6);
- **la alerta** de `TRANSFER_LIMIT` (prueba 5) se revisa;
- **las tres** quedaron sin activación vigente.

**Para la activación final de la Mac** (Fase H): la Perpetua está libre. No interviene en ninguna prueba de esta lista.

### Volver a levantar una instancia (misma identidad, base nueva)

```sh
G=$SP/l1/fase-g    # o, sin el scratchpad, H=~/.adel-pos-dev/instancias-l1 y $H/herramientas
$G/levantar.sh up <carpeta> <puerto local> <puerto de red> [--bin <binario>] [--en-linea] [--reloj <archivo>]
python3 -I $G/g.py <carpeta> setup     # establecimiento nuevo y sesión de admin (la base es nueva)
python3 -I $G/g.py <carpeta> status    # la licencia sigue: vive en identity/licencia
$G/levantar.sh down <carpeta>          # apaga y borra la base; la identidad se queda
```

- **Qué hace `levantar.sh up`**:
  - crea la base y pone su nombre y su contraseña en `server.json`;
  - corre `migrate` y `prepare-setup` y arranca `serve`.
- **El binario**: por omisión, `<carpeta>/../bin/ad-el-pos-server`, que en `~/.adel-pos-dev/instancias-l1` ya existe.
- **Sin `--en-linea`** la instancia no puede hablar con el servidor de licencias (en `test_mode` solo habla con
  loopback). Sirve para importar un `.lic` o generar un `.licreq`.
- **Comprobado** con `sin-internet`, `en-linea` y `mensual` ya conservadas, y con M, P y C al pasar a `f32d740`:
  - el mismo `server_id`;
  - nunca quedaron como copiadas;
  - la licencia (o su falta) siguió igual.

### Pruebas realmente ejecutadas

- **La §7 de la guía**: la 1, 2, 3, 4, 5, 6, 9, 11, 12, 13 y 14, contra `licensing.sicrat.com`, con lo que se obtuvo
  en cada una (arriba).
- **Los cupos reales de los tres planes**, por la API v1, con `f32d740`. Los tres recorridos se aprobaron, sin
  fallas.
- **La consulta del estado del servidor** al volver del corte.
- **Volver a levantar instancias conservadas**, sin internet: `sin-internet`, `en-linea` y `mensual`.
- **Las búsquedas de códigos** en archivos y en la auditoría.
- **Los MD5** de lo protegido.

### Lo no verificado

- **Lo del panel**: «Computadoras y transferencias», la alerta de `TRANSFER_LIMIT` y la aprobación de los `.licreq`.
  Son de la lista del propietario.
- **Las pruebas 7, 8 y 10**: las hace el propietario.
- **La liberación del comandero a los 15 minutos**, la tableta compartida y la concurrencia: no se repitieron contra el
  servidor real. Las cubren las pruebas automáticas de la Fase D, y el servidor de licencias no participa.
- **Android y el escritorio**: no se usaron. Los cupos se probaron solo con terminales de escritorio por la API.

### Para el coordinador

- **Ningún commit.** El árbol `$SP/wt/l1-g` quedó en `f32d740`, sin cambios.
- **Nada está encendido**: ni Go temporal ni emuladores. Los tres relojes simulados quedaron vacíos.
- **Decisión propuesta**: ninguna. **G-1 es para el propietario** (V1.1).
- **Las identidades de A y B ya no existen**: sus carpetas se reutilizaron. Sus `.lic` quedan en `$SP/l1/fase-g/lic/`.
- **C queda con la hora confiable en 2027** (por la prueba 14). No se conservó y no debe usarse para nada más.

### Decisiones del coordinador sobre la Fase G

- **L1-D40 · El defecto G-1 del servidor de licencias**: un `request_id` reutilizado con otro desafío responde
  `INVALID_PROOF` en vez de `REQUEST_CONFLICT` (§8.7). No afecta a AD-EL POS, que reintenta siempre con el mismo cuerpo.
  Va como defecto del servidor o propuesta para la V1.1, y lo decide el propietario.
- **L1-D41 · La Perpetua es del plan básico (1 / 0 / 0).** _(Desde el 24/11/2026 es del plan elemental: ver L1-D59.)_ Si el propietario activa con ella la Mac real (§10 de la
  especificación), la Mac funciona como caja, pero las tabletas Android no capturan y Preparación no se abre. Va en la
  entrega final.

---

## Fase F · Android 0.4.0 (§8)

Rama `etapa-l1-f` (árbol `$SP/wt/l1-f`), desde la cabeza de `etapa-l1` con la Fase C fusionada (`c55eef74`). La fase la
empezó un agente que se detuvo el 23/11 por el límite de uso; otro agente la terminó el 24/11 (pruebas que faltaban,
capturas de Android 8 y este documento). **No se cambió nada de `backend/`.**

### Qué se hizo

#### Versión

- `android/gradle.properties`: **0.4.0**, `versionCode` **4** (`36b7fe33`).

#### Los rechazos de la licencia, del cupo y de la hora, en un solo lugar (`8a4296ef`)

- `core:domain/error/LicenseRefusal.kt`: los cinco códigos de L1-D17 (`LicenseRefusal.CODES`) y sus `details`:
  `LicenseState` (estado y `details.operation`), `NoSeat` (`seat`, `in_use`, `limit`; el asiento se lee de
  `details.seat`, nunca se supone) y `ClockInvalid` (los dos textos de la hora de Go).
- `core:domain/error/AppError.kt:93`: `AppError.LicenseRefused`. Lo arma `ApiErrors.toAppError`
  (`core:data/api/ApiProblem.kt`) con un 409 y uno de esos códigos. No pasa por `report` (no es un error de acceso).
- `core:domain/error/LicenseMessages.kt:10`: **todos los textos**, nunca el `message` de la respuesta:
  - licencia vencida, revocada o sin activar: «La licencia del sistema no permite capturar pedidos. Avisa al
    encargado.» (en otra operación, «…no permite esta operación…»);
  - sin cupo, el texto de §5 con los números de `details`: «No hay cupo de comandero libre (3 de 3 en uso). Pide que
    se cierre la sesión de otra terminal.»; con límite 0, «La licencia no incluye comanderos (0 de 0): solo se
    capturan pedidos en una terminal con el turno de caja abierto.» (L1-D27); también los de caja y cocina, por si Go
    los dijera;
  - hora: «No se puede registrar: la computadora principal tiene la fecha y hora incorrectas. Avisa al encargado.»,
    más «La computadora principal tiene la hora … y la última actividad registrada fue el …» con los textos de Go;
  - al emparejar con una principal sin activar (L1-D6): «La computadora principal no tiene la licencia activada. Pide
    al encargado que la active y vuelve a intentarlo.».
- Quien muestra un rechazo usa `ApiErrors.shownMessage(problem)` o `AppError.shownMessage()`: captura, modificar lo
  enviado, precuenta, «Entregado», «Descartar cuenta vacía» y la entrada con PIN.

#### La cola sin conexión (`442a7a9b`)

- `core:domain/waiter/OrderSendRules.kt:60`: `SendOutcome.NotAllowed`. El pedido rechazado por la licencia, un cupo o
  la hora queda en **«Requiere tu revisión»** con `ReviewReason.NOT_ALLOWED` (`OrderQueue.kt:168`) y el texto de su
  código (`OrderSendRules.kt:121`). **Nunca vuelve solo a «Por enviar»**: el trabajador no lo reintenta.
- `OrderReviewRules.kt:52` (`resend`) y `:126` (`offersResendAsIs`): la revisión muestra su introducción propia y
  **«Enviar de nuevo»** (`TAG_REVIEW_RESEND`), con la **misma `operation_id`** y sin confirmación: Go no guardó nada
  (ni la clave) y vuelve a revisarlo todo. «Corregir en la captura» y «Descartar el pedido» siguen como antes.
- `CaptureRepository.kt:1077`: un `confirmDraft` rechazado así no sigue como pedido sin conexión (pasaría por la
  misma puerta).
- `TakeawayRepository.kt`: el nombre y teléfono de un pedido para llevar que la hora o la licencia no dejaron guardar
  **se conserva** y sale en una vuelta siguiente de la cola (al volver la red o el mesero, al abrir la aplicación o
  la de cada 15 minutos), sin reintentos seguidos.
- **Sin cambio de esquema de Room**: `NOT_ALLOWED` se guarda por su nombre en el JSON de `pending_orders.review`.

#### El estado de la licencia y los avisos (`eccb9150`)

- `core:domain/license/LicenseSummary.kt`: el `license` del contrato (estado, días de tolerancia, `clock`).
- `core:data/license/LicenseStatusRepository.kt:42` (`AppContainer.licenseStatus`, solo en memoria, borrado en
  `deviceDataWipe`): lo último que traen `restaurant/state` (`RestaurantRepository`) y `auth/me`
  (`PrecheckRepository`). El evento `license.changed` (`RealtimeKinds.LICENSE_CHANGED`) hace volver a leer
  `restaurant/state`. Un servidor anterior que no manda `license`: sin avisos.
- `core:ui/components/LicenseNotices.kt`:
  - `LicenseClockBanner` (`:52`): con `clock.blocked`, el **aviso fijo** de §6.2 debajo de la barra en todas las
    pantallas, con las dos fechas de Go;
  - `LicenseGraceBadge` (`:102`): en Tolerancia, el **aviso discreto** en la barra, «Licencia: quedan 3 días» con
    ≥ 600 dp de ancho o solo el ícono, 48 dp táctiles y el texto completo al tocarlo.
  - Los pone `AdelPosNavigation` (`app/navigation/AdelPosApp.kt`).

#### Documentos

- `docs/ANDROID_INSTALAR.md` (`c04b09f8`): la 0.4.0, «Activa la computadora principal antes de emparejar» y la
  sección 11, «La licencia de la computadora principal», con cada aviso y qué hacer.
- `android/CLAUDE.md` (`9c1070f7`): regla 23 («La licencia la decide Go»), la sección «Licencia», el contrato de la
  cola con `NOT_ALLOWED`, la versión 0.4.0 y la suite `license`.

#### Pruebas nuevas y el guion de punta a punta (`97d1848c`)

- JVM: `LicenseMessagesTest`, `LicenseQueueRulesTest` (dominio) y `LicenseErrorCodesTest` (las formas del contrato).
- Robolectric: `LicenseQueueTest` (sin cupo, hora y licencia vencida en la cola: revisión, sin reintentos del
  trabajador, «Enviar de nuevo» con la misma clave, el motivo al reabrir la base), `LicenseStatusTest` y
  `TakeawayQueueTest` (la edición que la hora no permitió).
- Instrumentada: `StageL1ScreensTest` (los dos avisos, la revisión y la barra).
- `scripts/android-e2e.sh`:
  - **Una licencia para el Go temporal de todas las suites, en un solo lugar**: `prepare_server` → `ensure_test_license`
    (`scripts/android-e2e.sh:415`). Si el servidor temporal dice «Sin activar», le importa una perpetua premium (5
    cajas, 10 comanderos, 5 pantallas de cocina) firmada por `backend/cmd/license-test` con las llaves de prueba de
    `vectors.json`, para la identidad de ese servidor. Nunca habla con un servidor de licencias.
  - **Suite `license`** (`LicenseOnGoTest`, pasos `l1`–`l8`): licencia de 0 comanderos y pedido en cola rechazado por
    cupo (`l1`–`l3`), «Enviar de nuevo» con la misma clave al volver el cupo (`l4`), reloj del servidor 2 h atrás con
    `ADELPOS_E2E_CLOCK_FILE` (`l5`) y corregido (`l6`), tolerancia (`l7`) y vencida (`l8`). Nunca la hora de la Mac
    ni la del emulador.

### Decisiones propuestas

1. **Sin migración de Room.** El motivo nuevo de la revisión, `ReviewReason.NOT_ALLOWED`, se guarda por su nombre en
   el JSON de `pending_orders.review`, como los motivos que ya existen. El estado de la licencia vive solo en memoria.
   Por qué: el encargo pedía una migración solo «si hace falta»; aquí no cambia ninguna tabla ni columna, y la prueba
   `LicenseQueueTest` comprueba que el motivo sigue ahí al reabrir la base.
2. **«Enviar de nuevo» con la misma `operation_id`, sin pedir confirmación.** Por qué: Go no guarda nada de un envío
   rechazado por licencia, cupo u hora (ni la clave, ver «Qué responde `restaurant/offline-orders` sin cupo» en el
   avance) y vuelve a revisar todo. Si la respuesta de un envío aceptado se pierde, la misma clave evita el duplicado.
   La suite `license` lo comprueba en `l4` y `l6` («entra una vez»).
3. **Un pedido rechazado así nunca vuelve solo a «Por enviar».** Lo reenvía el mesero cuando el encargado resuelve
   el problema. Por qué: §8 pide que no se reintente sin fin, y Android no puede saber cuándo vuelve el cupo o se
   corrige la hora.
4. **La edición del cliente de Para llevar (nombre y teléfono) que la hora o la licencia no dejaron guardar se
   conserva y sale en una vuelta siguiente de la cola** (al volver la red o el mesero, al abrir la aplicación o cada
   15 minutos). No pasa a «Requiere tu revisión». Por qué: no es un pedido ni ocupa cupo, y la cola de esas ediciones
   no tiene pantalla de revisión. No hay reintentos seguidos: espera la siguiente vuelta.
5. **Un `confirmDraft` en línea rechazado por la licencia no sigue como pedido sin conexión.** Por qué: pasaría por la
   misma puerta de Go. El mesero ve el texto del código en la captura.
6. **El aviso de tolerancia muestra el texto completo («Licencia: quedan 3 días») con 600 dp o más de ancho, y solo
   el ícono en pantallas angostas.** Al tocarlo, abre el texto completo. Por qué: §8 pide un aviso discreto, y en un
   teléfono vertical la barra ya lleva al mesero, la conexión y «por enviar».
7. **El asiento del mensaje sin cupo sale de `details.seat`.** Los textos de caja y cocina están por si Go los
   dijera, como pide el avance («la Fase F debe leer `details.seat` y no suponerlo»).

### Pruebas (lo que realmente se ejecutó)

Cabeza probada: `9c1070f7` (rama `etapa-l1-f`). Los APK que se instalaron son los de esa cabeza (`assembleDebug` al
día, `versionName` 0.4.0, `versionCode` 4). Go temporal `ISO_TAG=LF` (8209/8479), nuevo en cada corrida, con la
licencia de prueba de `ensure_test_license`. Registros en `$SP/lf/`.

| Prueba                                                                                                  | Dónde                                      | Resultado                                                                                                                                                                                                   |
| ------------------------------------------------------------------------------------------------------- | ------------------------------------------ | ----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `ktfmtCheck`, `:core:domain:test`, `testDebugUnitTest` (JVM y Robolectric, con `--rerun`) y `lintDebug` | Mac                                        | **62 XML, 413 pruebas, 0 fallas, 0 omitidas**; Lint sin errores (`unit-full3.log`)                                                                                                                          |
| Instrumentadas (`connectedDebugAndroidTest`, apps de prueba desinstaladas antes)                        | `Pixel_Tablet` (Android 17, 5580)          | **3 XML** (app, core/data, core/ui): **168 pruebas, 82 corridas, 86 omitidas, 0 fallas** (`connected-emulator-5580-xml.txt`). Se omiten las `*OnGoTest` (corren en las e2e) y `ServerApiOnDeviceTest`       |
| Instrumentadas (igual)                                                                                  | `ADEL_A1_Tablet_API26` (Android 8.0, 5586) | **3 XML: 168 pruebas, 80 corridas, 88 omitidas, 0 fallas** (`connected-emulator-5586-xml.txt`, 23/11). Además de las de arriba, en Android 8 se omiten `ConnectionOnDeviceTest` y `WorkNetworkOnDeviceTest` |
| e2e `license` (`l1`–`l8`)                                                                               | 5580                                       | **8/8 aprobadas** (`e2e/5580-license-1`, 23/11)                                                                                                                                                             |
| e2e `license` (`l1`–`l8`)                                                                               | 5586                                       | **8/8 aprobadas** (`e2e/5586-license-1`)                                                                                                                                                                    |
| e2e `waiter` (`w1`–`w13c`)                                                                              | 5580                                       | **19/19 aprobadas** (`e2e/5580-waiter-1`)                                                                                                                                                                   |
| e2e `waiter`                                                                                            | 5586                                       | **19/19 aprobadas** (`e2e/5586-waiter-1`)                                                                                                                                                                   |
| e2e `access` (pasos 1–5 y 7)                                                                            | 5580                                       | **6/6 aprobadas** (`e2e/5580-access-1`)                                                                                                                                                                     |
| e2e `access` (pasos 1–5 y 7)                                                                            | 5586                                       | **6/6 aprobadas** (`e2e/5586-access-1`)                                                                                                                                                                     |
| e2e `takeaway` (`t1`–`t9`)                                                                              | 5580                                       | **9/9 aprobadas** (`e2e/5580-takeaway-1`)                                                                                                                                                                   |
| e2e `takeaway` (`t1`–`t9`)                                                                              | 5586                                       | **8/9**: `t6` FALLA, dos corridas (`e2e/5586-takeaway-1` y `-2`). Es el pendiente conocido **A3-D32** (ver abajo)                                                                                           |

- En Android 8 se instaló con `adb install -r -t` (lo mismo que `install_app` del guion: antes desinstala la
  aplicación, sus pruebas y las de la 0.1.0) y la suite corrió con `--no-install` (A3-D33). La instalación por Gradle
  no se probó ahí.
- **`takeaway t6` en Android 8** falla en `TakeawayOnGoTest.kt:306` (`waitForText("Luisa Martínez")`). La condición
  anterior sí se cumple: el nombre llega a Go y la cola de ediciones queda vacía. Solo la lista de la tableta no
  muestra el nombre nuevo en 40 s. Es el mismo síntoma de **A3-D32** (C1 de la A3, que el propietario dejó abierto el
  20/11). En las dos corridas no hubo ningún rechazo de licencia, cupo u hora (cero líneas `LICENSE_`/`SYSTEM_CLOCK`
  en el logcat del paso, y el Go temporal solo registra el arranque y la activación de prueba). En 5580 aprueba. **No
  se tocó**: no lo causa la L1 y la decisión del propietario sigue en pie.
- Ninguna e2e existente falló por un cambio de pantalla de esta fase; no hubo que corregir pruebas.
- Ninguna prueba llamó a un servidor de licencias: el Go temporal arranca con `license_server_disabled`
  (`remote_not_allowed`) y las licencias se importan sin internet, firmadas con las llaves de `vectors.json`.

### Capturas (tema claro)

- `$SP/shots-l1/android/pixel-tablet/`: 12 (Android 17, `5580-license-1`).
- `$SP/shots-l1/android/tablet-api26/`: 12 (Android 8.0, `5586-license-1`), revisadas una por una:
  - el **aviso fijo de la hora** en Mesas, en la captura y en la revisión (`l5-*`), con las dos fechas de Go;
  - el **aviso de tolerancia** en la barra y su texto completo al tocarlo (`l7-*`);
  - **«Requiere tu revisión» sin cupo** con «Enviar de nuevo» (`l1-revision-sin-cupo-con-enviar-de-nuevo.png`) y
    Pendientes con dos pedidos en revisión (`l3-*`);
  - la captura con la licencia vencida (`l8-*`) y sin cupo (`l1-captura-*`).
- No hay textos provisionales, códigos ni `message` del servidor a la vista. Todo es español sencillo.
- Observaciones (sin cambio, para que el coordinador decida si valen la pena):
  - en la captura, la tarjeta del rechazo por la hora o por la licencia vencida ofrece «Reintentar», el botón que ya
    tenían los demás rechazos del borrador; mientras el encargado no lo resuelva, reintentar dará el mismo mensaje;
  - con el aviso fijo de la hora, la captura muestra el mismo texto dos veces: en el aviso y en la tarjeta del
    rechazo;
  - la insignia «N pedidos por enviar» de la barra cuenta también los pedidos en «Requiere tu revisión», como antes
    de esta etapa.

### No verificado

- La instalación por Gradle en Android 8 (se usó `adb install`, A3-D33).
- `access` paso 6 (certificado cambiado): necesita otro servidor temporal con otra identidad y no entra en la suite
  por omisión. Esta fase no lo cambia.
- Teléfonos: solo se corrió en las dos tabletas que asignó el encargo. El aviso de tolerancia con solo el ícono lo
  cubre `StageL1ScreensTest` (el componente con `showText = false`, en las instrumentadas). La elección por ancho
  (menos de 600 dp) en una pantalla angosta real no se vio.
- Las suites `catalog`, `field`, `precheck`, `alerts`, `adjustments` y `connection` no se corrieron: el encargo pidió
  `waiter`, `access`, `takeaway` y `license`. Todas pasan por el mismo `ensure_test_license`.

### Para el coordinador

- `takeaway t6` en Android 8 sigue como A3-D32: decidir si se mantiene como pendiente conocido en la L1.
- Las decisiones propuestas 1–7, sobre todo la 1 (sin migración de Room) y la 4 (la edición del cliente no pasa a la
  revisión).
- No hubo commits nuevos al reanudar: la rama queda en `9c1070f7` y no trae `c0217c0d` (avance de la Fase G, solo
  documentación), que está en `etapa-l1`.

### Decisiones del coordinador sobre la Fase F

Fusionada en `774c6f0`, sin conflictos y sin cambios en `backend/`. El árbol unido solo difiere de la cabeza probada
(`9c1070f`) en este avance.

- **L1-D47 · Se aceptan las siete decisiones propuestas del agente.**
  - Sin migración de Room: `NOT_ALLOWED` se guarda por su nombre en el JSON de la revisión.
  - «Enviar de nuevo» usa la misma `operation_id` y no pide confirmación.
  - Un pedido rechazado por la licencia, un cupo o la hora nunca vuelve solo a «Por enviar».
  - La edición del cliente de Para llevar que no se pudo guardar se conserva y sale en la siguiente vuelta de la cola:
    no es un pedido, no ocupa cupo y no tiene pantalla de revisión.
  - Un borrador en línea rechazado así no sigue como pedido sin conexión.
  - El aviso de tolerancia muestra el texto desde 600 dp y solo el ícono en pantallas más angostas.
  - El mensaje sin cupo toma el tipo de cupo de `details.seat`.
- **L1-D48 · `takeaway t6` en Android 8 sigue como el pendiente conocido A3-D32.** Falla igual que en la A3, sin
  ningún rechazo de licencia, cupo u hora; en la Pixel_Tablet aprueba. La regresión de la Fase H lo vuelve a correr.
- **Las tres observaciones de las capturas se quedan como están**:
  - «Reintentar» en la tarjeta del rechazo sirve en cuanto el encargado corrige la hora o la licencia;
  - el texto de la hora aparece dos veces porque la §6.2 pide las dos cosas: el aviso fijo mientras dure y el mensaje
    cuando Go rechaza;
  - el contador «por enviar» cuenta igual que antes de la etapa.

---

## Revisión independiente R3 · Asignación atómica de cupos (Fase D)

Árbol `$SP/wt/l1-r3`, rama `etapa-l1-r3`, desde `f4b42a3` (la Fase D fusionada sobre la A). `ISO_TAG=R3`, clúster
temporal 55499, sin Go temporal ni emuladores. Se revisó `git diff 1ceee93 etapa-l1-d` (`f202543`, `20019dc`,
`e816750` y `bc4db5b`), sobre todo `backend/internal/licenseseats/`, `withSeats` en `administration.Service.dispatch`,
el modo Cocina, «Liberar cupo» y la sección «Fase D» de la `0028`. Las decisiones que mandan son L1-D2, L1-D3, L1-D7,
L1-D13, L1-D27 y L1-D28. Usé `doc-D.md` solo como mapa y comprobé cada afirmación en el código.

No se corrigió nada del código. El único commit agrega las pruebas que demuestran los hallazgos:

- `c0810d7` etapa L1 revisión R3: pruebas de los cupos (R3-1 pedido sin conexión sin sesión, R3-2 reloj de los cupos
  y vencimiento de sesiones; fallan) y la concurrencia por el último cupo (pasa). Archivo:
  `backend/internal/operations/stagel1_r3_seats_test.go`.

**Aviso para fusionar**: mientras no se corrijan R3-1 y R3-2, `go-int ./internal/operations` completo falla en
`TestStageL1R3OfflineOrderWithoutSessionHoldsSeat` y `TestStageL1R3SeatClockAndSessionExpiry`. Esas pruebas fallan a
propósito.

### Hallazgos

| #    | Gravedad                               | Dónde                                                                                                                                                                                                                                                     | Escenario (entrada → resultado incorrecto)                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                       | Prueba (commit `c0810d7`)                                                                                                                                                                                                                                | Corrección propuesta                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                             |
| ---- | -------------------------------------- | --------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------ | -------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | ---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| R3-1 | **Alta**                               | `licenseseats/store.go:26-28` (`inUseCondition`, comandero: exige una sesión vigente), `store.go:93-101` (`claim` inserta sin comprobar que la fila contará), `administration/service.go:368-373` (la credencial de envío entra a `withSeats` sin sesión) | Con 1 comandero: una tableta se queda sin sesión vigente en Go (el último mesero cerró sesión, «Liberar cupo» cerró sus sesiones o venció la sesión de 12 h) y entrega con la credencial de envío (A1-D1) un pedido de su cola. Entra (200), pero su fila `order_taker` no cuenta porque la terminal no tiene sesión. Resultado: `sessions-summary` dice 0 en uso, Terminales no muestra el cupo y otra terminal también captura (200). Cuatro tabletas así, a la vez, obtienen **las cuatro** el único comandero. Con 2 comanderos, después de «Liberar cupo», capturan tres terminales. La afirmación de `doc-D.md` (Decisión propuesta 9: «los pedidos de la cola de esa tableta piden cupo otra vez al llegar») solo es cierta cuando no hay cupo libre: si lo hay, el pedido entra sin ocuparlo. Otra forma de llegar al mismo caso (descrita, sin prueba): una captura que se autenticó justo antes de que «Liberar cupo» revocara la sesión (la lectura de `authenticateSession` no bloquea y su `UPDATE last_seen_at` continúa después de la revocación) ocupa una fila que ya no cuenta | `TestStageL1R3OfflineOrderWithoutSessionHoldsSeat`, con tres subpruebas: «después de cerrar sesión el último mesero», «después de «Liberar cupo», con la cola de la tableta» y «cuatro tabletas sin sesión a la vez por el último comandero». **Fallan** | Que el pedido entregado con la credencial ocupe el comandero los 15 minutos aunque la terminal no tenga sesión. Opciones: (a) que el comandero cuente solo por actividad (`last_activity_at > corte`) y que el cierre de la última sesión de **esa** terminal borre su fila de forma explícita (logout, revocar, cambiar la contraseña, revocar la terminal y «Liberar cupo», con 7820148311), en vez de exigir una sesión vigente en `inUseCondition`; (b) marcar en la fila que la captura llegó con credencial (por ejemplo `credential_id`) y contarla mientras esa credencial siga vigente y dentro del corte. En cualquier caso, `claim` no debe aceptar una captura si la fila que escribe no va a contar. **Toca L1-D27** («El comandero solo cuenta con una sesión vigente»): **decide el coordinador** |
| R3-2 | Baja (solo con reloj de prueba)        | `store.go:26-30` compara `sessions.expires_at` con `Options.Now`; `administration/auth.go:139` escribe `expires_at = time.Now()+12h` y `service.go:486` lo comprueba con `time.Now()`                                                                     | Con el reloj del gestor de cupos adelantado más de 12 h (el desfase de prueba de L1-D16 que la Fase C conectará con `licensing.Manager.LocalClock`, por ejemplo para probar la tolerancia o el vencimiento), la sesión con que la terminal acaba de capturar sigue valiendo para Go, pero ya no cuenta para los cupos. Las filas de comandero y de cocina no cuentan y cualquier número de terminales captura con 1 comandero. Con un desfase menor, las sesiones «vencen» antes para los cupos. En producción `LocalClock` es `time.Now` y no pasa, pero las pruebas de interfaz y de Android con desfase darían resultados falsos                                                                                                                                                                                                                                                                                                                                                                                                                                                              | `TestStageL1R3SeatClockAndSessionExpiry` (reloj +13 h, dos escritorios capturan con 1 comandero). **Falla**                                                                                                                                              | Comparar el vencimiento de la sesión con el mismo reloj que lo escribió y lo valida (`time.Now`), o que `issueSession` y `authenticateSession` usen el reloj inyectado. Avisar a la Fase C                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                       |
| R3-3 | Baja (documentación y riesgo a futuro) | `CLAUDE.md:158` y `:160`; `events/store.go:44`; `operations/queries.go:89`; `payments/shifts.go:308`                                                                                                                                                      | La captura y la apertura de turno publican su evento (`events.Append` toma 7820148306) **antes** de que `withSeats` pida 7820148311. El orden real es 05 → 04 → 08 → 06 → 11, pero la tabla dice «7820148311 siempre al final (05 → 04 → 08 → 11)» y la fila de 06 dice que todos lo toman «al final de sus cambios». Hoy no hay interbloqueo, porque quien tiene 11 no publica eventos. Pero si una fase posterior publica un evento con 11 tomado (por ejemplo, un aviso de «cupos cambiaron» en el modo Cocina o en «Liberar cupo» para Terminales), una captura con 06 que espera 11 y esa ruta con 11 que espera 06 se interbloquean                                                                                                                                                                                                                                                                                                                                                                                                                                                        | Sin prueba (lectura del código)                                                                                                                                                                                                                          | Escribir en §3 que 7820148311 va después de 7820148306 y que con 11 tomado no se llama `events.Append`, o pedir el cupo antes de publicar el evento                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                              |
| R3-4 | Baja (carga)                           | `administration/license_seats.go:67-75`, `store.go:158-167` (`Renew`)                                                                                                                                                                                     | Cada previa (`draft-preview` y `edit-preview`, que el cliente llama mientras se captura) toma 7820148311 **exclusivo**, también en las cajas y en las terminales sin comandero, que no tienen nada que renovar. Antes, las previas solo tomaban 7820148305 compartido y corrían en paralelo; ahora se forman una detrás de otra. No es un error de cupos, pero agrega espera con varias terminales. No se midió                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                  | Sin prueba                                                                                                                                                                                                                                               | Antes del lock, si la terminal no tiene fila `order_taker`, no hacer nada. Las peticiones de una misma terminal ya van en fila por la fila de `app.devices` (`authenticateDevice`), así que ninguna otra puede crearle esa fila a la vez. El lock sigue siendo necesario cuando la fila existe (ver «Comprobado»)                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                |

### Lo que comprobé y está bien

- **Atomicidad**:
  - `claim` toma `pg_advisory_xact_lock(7820148311)` y después lee la hora de Go y los límites.
  - Contar y ocupar ocurren en la misma transacción. El conteo y el barrido van en una sola sentencia, con la foto
    tomada después del lock (las transacciones son READ COMMITTED; solo los reportes usan REPEATABLE READ).
  - Mi prueba `TestStageL1R3ConcurrentLastSeat` (**pasa**, 114 s) hace tres carreras por el último cupo:
    - seis escritorios entran a Preparación a la vez, en 3 vueltas;
    - cuatro terminales de cuatro cajas abren turno a la vez, en 2 vueltas;
    - dos tabletas con sesión (cola con credencial) y dos escritorios capturan a la vez, en 3 vueltas.

    En cada vuelta obtiene el cupo exactamente una y los rechazos traen `in_use` y `limit` exactos.

  - **Mutación** (cambio temporal, revertido con `git checkout`): sin el `pg_advisory_xact_lock`, la carrera de la cocina
    **falla** («6 de 2 en uso»). La de caja y la de comandero siguen pasando: esas operaciones ya van en fila por
    7820148305 exclusivo. El lock 7820148311 solo protege de verdad el modo Cocina y el barrido frente a las
    asignaciones, y ahí funciona.
- **Orden de los locks e interbloqueos**:
  - 7820148311 siempre se toma después de 7820148305 (y de 04, 08 y 06) y ninguna transacción que lo tiene espera otro
    lock consultivo.
  - «Liberar cupo» y la revocación de la terminal cierran las sesiones antes de pedirlo.
  - Las llaves foráneas de `license_seats` toman `FOR KEY SHARE` en `devices`, `sessions` y `cash_shifts`. Eso no choca
    con los `UPDATE` sin cambio de llave que hacen las demás rutas, y no hay `FOR UPDATE` ni `DELETE` sobre esas tablas.
  - Un lock tomado dentro del savepoint (modo Cocina y «Liberar cupo») se suelta solo si el savepoint se revierte, y
    entonces se revierte toda la transacción.
  - No encontré ningún ciclo.
- **Ningún camino escribe un cupo antes de validar**:
  - `claim` comprueba la licencia con el lock tomado.
  - El rechazo es un `Fault` 409 sin `Commit` fuera del savepoint, así que se revierte la transacción entera: cuenta,
    clave, auditoría, eventos y cola de impresión.
  - `completeDeferred` solo corre con éxito.
  - Las únicas escrituras sin 7820148311 son los borrados en cascada y la subida de la `0028`. Solo quitan filas o corren
    sin concurrencia.
- **Clave de operación ya registrada (L1-D27)**:
  - Las cinco capturas y la apertura de turno pasan por el `replay` de `app.restaurant_operations`, que responde lo
    guardado o un conflicto sin escribir nada nuevo.
  - La comprobación previa de `operationRecorded` podría parecer una carrera, pero no se puede explotar. Dos peticiones
    de la misma terminal van en fila por la fila de `app.devices` (el `UPDATE` de `authenticateDevice`), y el `replay`
    exige la misma terminal.
- **Rutas que crean o modifican pedidos**:
  - Todos los `INSERT` en `restaurant_accounts`, `orders`, `items`, `item_versions`, `item_units` y `draft_items` salen
    de las cinco rutas `Capture`. Las cuentas nuevas de transferir y dividir son reorganización (L1-D6).
  - `payments`, `kitchen` y `catalog` no crean pedidos. Cobrar y efectivo dan `None` (L1-D3).
  - `ForOperation` coincide con el enrutador de `operations`, también con la diagonal final.
  - `ServeMux` redirige las rutas con `//`, `.` o `..` antes de llegar al manejador.
  - `offline-orders` con credencial solo se acepta en la ruta exacta.
- **Las reglas**:
  - **L1-D2**: una fila por terminal. Se comprobó la tableta con tres PIN.
  - **L1-D3**: la caja compartida ocupa dos cupos y cerrar el turno desde otra computadora libera los dos. Cobrar no pide
    cupo. Una terminal no puede cambiar de caja con el turno activo (`devices.go`, `SHIFT_OPEN`), así que no se pueden
    abrir dos turnos con un solo cupo.
  - **L1-D7 y L1-D28**: el modo Cocina funciona. La limitación de L1-D28 (el tablero no exige el cupo) queda aceptada por
    el coordinador.
  - **L1-D13**: 15 minutos exactos (`>` el corte), las previas renuevan, abrir turno libera los comanderos de esa caja y
    cerrar sesión el último usuario barre su fila.
  - La licencia nueva con menos cupos y el plan básico funcionan.
- **«Liberar cupo»**:
  - Pide `devices.manage`; las sesiones con PIN no lo tienen (403).
  - Responde `409 LICENSE_SEAT_NOT_HELD` sin cupo o con solo la caja, y deja la auditoría `license.seat_released`.
  - Por el código (sin prueba propia), un identificador inválido responde 400 y uno inexistente 404 (`publicFault`).
- **Migración «Fase D»**: `opened_device_id` es `NOT NULL`, los `CHECK` de vínculo y tipo están bien y `DISTINCT ON`
  deja una fila por terminal.
- **Falla cerrada**: sin fuente de límites no hay licencia válida. `seatstest` solo lo importan las pruebas y no da
  cupos fuera de ellas (`testing.Testing()`).
- **Observación ya resuelta**: `f4b42a3` no pasaba `fmt-check` (el orden de los `import` de `administration/service.go`
  quedó mal al fusionar). En `etapa-l1` ya está bien. Mi archivo pasa `gofmt`, y no confirmé el cambio de `format` en
  `service.go` porque no es de esta revisión.

### Pruebas ejecutadas (lo que de verdad corrió)

| Prueba                                                                                  | Resultado                                                                                                                                                                                                   |
| --------------------------------------------------------------------------------------- | ----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `go vet -tags=integration ./internal/operations`                                        | Limpio                                                                                                                                                                                                      |
| `go-int ./internal/operations`, `ISO_RUN=TestStageL1R3`, sobre `c0810d7`                | `TestStageL1R3OfflineOrderWithoutSessionHoldsSeat` **falla** en sus 3 subpruebas (R3-1). `TestStageL1R3SeatClockAndSessionExpiry` **falla** (R3-2). `TestStageL1R3ConcurrentLastSeat` pasa (158 s en total) |
| `TestStageL1R3ConcurrentLastSeat` sola                                                  | `ok` (114 s)                                                                                                                                                                                                |
| La misma, **sin** `pg_advisory_xact_lock` en `store.go` (cambio temporal, revertido)    | Falla la cocina («6 de 2 en uso»); caja y comandero pasan (7820148305)                                                                                                                                      |
| `go-int ./internal/licenseseats` (las del autor)                                        | `ok`                                                                                                                                                                                                        |
| `go-int ./internal/operations`, `ISO_RUN=TestStageL1LicenseSeats$` (la del autor)       | `ok` (81 s)                                                                                                                                                                                                 |
| `format` y `fmt-check`                                                                  | `fmt-check` solo marca `administration/service.go`, que ya venía mal de `f4b42a3` y está corregido en `etapa-l1`. Mi archivo pasa `gofmt -l`                                                                |
| `md5` de `docs/etapas/*.md`, `docs/licencias/CONTRATO*` y `contracts/`, antes y después | Iguales                                                                                                                                                                                                     |

### Lo que no pude verificar

- **La conexión real con el gestor de licencias** (Fase C): `LimitsSource` con `Status()` y `LocalClock`, y la puerta
  del estado y de la hora alrededor de `withSeats`. R3-2 se volverá real en las pruebas en cuanto se conecte el desfase
  de L1-D16.
- **El escritorio y Android**: no corrí emuladores, Vitest ni Playwright. No comprobé qué hace la cola de Android
  0.3.0 después de «Liberar cupo», más allá de lo que dice el código (`OrderSender` usa la credencial cuando no tiene
  sesión).
- **La carga real** con varias terminales: solo la observación de R3-4.
- **La variante concurrente de R3-1** («Liberar cupo» al mismo tiempo que una captura): la describí por el código, sin
  prueba. La corrección de R3-1 la cubre.

### Correcciones de la revisión R3

Revisión independiente en `$SP/l1/doc-R3.md` y decisiones del coordinador L1-D36 a L1-D39. En la rama `etapa-l1-d`,
sobre `etapa-l1` (`a8dd193`, con la Fase B):

- `5b54a576`: el commit de pruebas del revisor (`c0810d7`), traído tal cual con `cherry-pick`. No se ajustó ninguna
  prueba: las tres están bien planteadas.
- `981be34a`: las correcciones.
- `e3776dd1`: los documentos de la API.
- `5358f25d`: trae `etapa-l1` con la Fase C (`91f17ba`, `f32d740`), sin conflictos. La puerta de la C y su
  `LicenseSeats` quedan como estaban.

**R3-1 (alta, L1-D36): el comandero cuenta por actividad.**

- `inUseCondition` (`licenseseats/store.go:22`): la fila del comandero cuenta mientras `last_activity_at` sea posterior
  al corte de 15 minutos. Ya no se pide una sesión vigente. El pedido que entrega la cola de una tableta sin sesión, con
  la credencial de envío, ocupa su cupo como cualquier captura.
- **Se borra al cerrarse la última sesión de esa terminal**: `licenseseats.ReleaseClosedSessions` (`store.go:210`).
  - Borra el comandero de las terminales a las que **esta transacción** les cerró una sesión (`revoked_at = now()`, la
    hora de inicio de la transacción, igual en todas sus sentencias) y que ya no tienen ninguna sesión vigente.
  - Las demás terminales, también las tabletas sin sesión que capturan con la credencial, no se tocan.
  - Primero comprueba sin el lock si hay algo que borrar; solo entonces toma 7820148311.
  - Lo llaman:
    - `withSeats` después de toda operación `session_end` (`administration/license_seats.go:82`);
    - `authenticateSession` al encontrar una sesión vencida (`administration/service.go:525`, con `time.Now`, el mismo
      reloj con que la comprueba).
  - `ForOperation` agrega a `session_end` las otras rutas que cierran sesiones: `PUT auth/pin`, `PATCH users/{id}`
    (desactivar), `PUT users/{id}/pin`, `POST users/{id}/pin/remove` y `POST license/identity/regenerate`.
- **Revocar la terminal** (`POST devices/{id}/revoke`, `administration/devices.go:147`) libera además su comandero y
  su cocina con `ReleaseDevice`, después de cerrar sus sesiones. Su caja sigue hasta cerrar el turno.
- «Liberar cupo» y abrir turno ya borraban la fila. Un pedido de la cola que llega después de «Liberar cupo» pide cupo
  otra vez: si lo hay, lo ocupa 15 minutos; si no, `LICENSE_SEAT_UNAVAILABLE`.
- **`claim` nunca acepta una fila que no vaya a contar**:
  - la del comandero cuenta por la actividad que escribe;
  - la de la caja, por su turno activo (la operación tiene 7820148305);
  - la de la cocina, por la sesión que entra, que ahora se comprueba vigente en la misma sentencia del conteo. Si se
    cerró mientras llegaba la petición, es `ErrSessionClosed` y `POST kitchen/display-mode` responde `401
SESSION_REVOKED`. Esto cubre la variante concurrente que describió el revisor.

**R3-2 (L1-D37): el reloj de las sesiones.**

- `Options.SessionNow` (`licenseseats/seats.go:130`, por omisión `time.Now`) es el reloj con que la autenticación
  escribe y valida `expires_at`.
- Con él se decide si sigue vigente la sesión de una pantalla de cocina, también en `claim`. El reloj inyectado de los
  cupos (`Options.Now`, que la Fase C conecta a `LocalClock`) queda solo para la actividad y el corte.
- `Window()` conserva su firma `(hora, corte)`, porque la usan las pruebas de la Fase C. La hora de las sesiones sale de `Manager.SessionNow()`, y `devices.go` pasa el corte y esa hora a `DeviceSeatsColumn`.
- El comandero ya no depende de las sesiones (R3-1).
- **Para la Fase C**: si alguna vez la autenticación usa un reloj inyectado, hay que pasarlo también en `SessionNow`.

**R3-3 (L1-D38): el orden de los locks.**

- `CLAUDE.md` §3, en la fila de 7820148306 y en la de 7820148311: 7820148306 (los eventos, `events.Append`) se toma
  antes de 7820148311, y con 7820148311 tomado nunca se publica un evento ni se pide otro lock. El orden de adquisición
  dice 05 → 04 → 08 → 06 → 11.
- **Revisé los caminos**:
  - la captura, la apertura de turno y las rutas de sesiones publican en su manejador, antes de `withSeats`;
  - el modo Cocina, «Liberar cupo», revocar la terminal y la sesión vencida solo escriben la auditoría después del
    lock;
  - el `license.changed` de la Fase C se publica en su propia transacción, fuera de la de la petición (`notifyLicense`).
  - No encontré ningún camino que publique o pida otro lock con 7820148311 tomado.

**R3-4 (L1-D39): las previas, sin lock.**

`Renew` (`store.go:176`) ya no toma 7820148311. Hace un `UPDATE` condicional de la propia fila:
`last_activity_at > corte`, con `greatest` para que la hora nunca retroceda. Si la terminal no tiene comandero, no
escribe nada.

**Es seguro sin el lock porque nunca revive un cupo vencido:**

- La previa tiene 7820148305 compartido (su manejador) y toda asignación de comandero lo tiene exclusivo (la captura y
  el pedido sin conexión, `operations.Handle`), así que nunca se intercalan:
  - si la asignación va primero, la previa lee su hora después y ve el cupo tan vencido como lo vio la asignación;
  - si la previa va primero, la asignación ve la actividad renovada.
- Los borrados (liberar, cerrar sesiones, abrir turno, el barrido de `claim`) chocan con la previa en la fila: el que
  llega segundo espera. Si el borrado va primero, la previa no encuentra la fila; si va después, la borra.
- La asignación de cocina, que no tiene 7820148305, solo cuenta filas de cocina.

**Lo único que cambió en el comportamiento de antes**: una tableta cuya última sesión vence sin volver a usarse (el
vencimiento se marca al usarla) o se cierra fuera de una petición clasificada (el restablecimiento de soporte por línea
de órdenes, `reset-sa`) conserva su comandero hasta 15 minutos después de su última captura. Es el lado seguro: cuenta
de más, nunca de menos.

**Pruebas de las correcciones** (la tabla final, abajo):

- `licenseseats/seats_integration_test.go`:
  - cerrar la última sesión libera, y con otra sesión vigente en la terminal no;
  - la tableta sin sesión ocupa el cupo por su actividad y no la libera el cierre de sesión de otra terminal;
  - la pantalla de cocina con el reloj de los cupos 13 h adelante sigue contando;
  - Preparación con una sesión cerrada responde `ErrSessionClosed`;
  - las vueltas de la concurrencia liberan con `logout` (cierre y liberación en la misma transacción).
- `operations_test.go`: las rutas nuevas de `session_end` y los dos relojes.
- **Mutación otra vez**: sin `pg_advisory_xact_lock`, `TestSeatAssignmentIsAtomic` falla. Lo revertí.

**Tabla de pruebas de las correcciones R3** (lo que realmente corrió, `ISO_TAG=LD`, clúster temporal 55499). Las
corridas completas son del 24/11, sobre el estado final de la rama (`a9913cf1`, que incluye también la corrección de
R2-2, abajo). Las del 23/11 las corrió el agente anterior antes del corte.

| Prueba                                                                                                                                          | Resultado                                                                                                                     |
| ----------------------------------------------------------------------------------------------------------------------------------------------- | ----------------------------------------------------------------------------------------------------------------------------- |
| 23/11, antes de traer la Fase C: `go-int ./internal/operations` con `ISO_RUN=TestStageL1` (las tres del revisor R3 y `TestStageL1LicenseSeats`) | `ok`                                                                                                                          |
| 23/11: sin `pg_advisory_xact_lock` (cambio temporal, revertido)                                                                                 | `TestSeatAssignmentIsAtomic` **falla**                                                                                        |
| 23/11, antes de `d9f5ca40`: `go-unit`, `go-int` de `licenseseats`, `database` y `apicontract`                                                   | `ok`. `administration` y `operations` quedaron a medias por el corte                                                          |
| 24/11: `go-unit` (vet normal e integración, `test -race`, compilación y vet para Windows)                                                       | `ok` (30 s)                                                                                                                   |
| 24/11: `go-int ./internal/licenseseats` completo                                                                                                | Falla **solo** `TestReviewR2DefectSeatActivityFromAClockJumpOutlivesTheCorrection` (la prueba de R2-2 del revisor, ver abajo) |
| 24/11: `go-int ./internal/licenseseats` con todas las demás (`ISO_RUN`)                                                                         | `ok` (4 s)                                                                                                                    |
| 24/11: `go-int ./internal/database`                                                                                                             | `ok` (17 s)                                                                                                                   |
| 24/11: `go-int ./internal/apicontract`                                                                                                          | `ok` (53 s)                                                                                                                   |
| 24/11: `go-int ./internal/administration` completo                                                                                              | `ok` (307 s)                                                                                                                  |
| 24/11: `go-int ./internal/operations` completo (las tres del revisor R3, `TestStageL1LicenseSeats`, la puerta de la Fase C y la nueva de R2-2)  | `ok` (712 s)                                                                                                                  |
| 24/11: `go-int ./cmd/ad-el-pos-server`, con `run-cmd.sh` y el candado `$SP/l1/candado-cmd`                                                      | `ok` (68 s); candado soltado                                                                                                  |
| 24/11: `format`, `fmt-check` y `sync-check` (antes de los commits y otra vez al final)                                                          | Aprobados; el `format` final no cambió nada                                                                                   |
| `md5` de `docs/etapas/*`, `docs/licencias/CONTRATO*` y `contracts/` (53 archivos), antes y después de formatear                                 | Iguales, las dos veces                                                                                                        |

En una corrida anterior del agente anterior (`$SP/l1d-work/administration-run1.log`), `go-int ./internal/administration`
falló una vez en `stage9g_sesiones_activas_por_tipo`, con `by_kind` de 20 escritorios. La prueba compara ese conteo con una consulta propia hecha en otra transacción, así que
parece una carrera de la prueba con las sesiones de las demás subpruebas. En la corrida completa del 24/11 pasó. No lo
investigué más.

### Corrección del hallazgo R2-2 (L1-D43)

Hallazgo R2-2 de la revisión R2 (`$SP/l1/doc-R2.md`, fila de R2-2) y decisión del coordinador L1-D43. En la rama
`etapa-l1-d`, sobre `cc5c4c5f` (la prueba del revisor):

- `3aea0abf`: la corrección y sus pruebas.
- `82bd7303`: `docs/API_V1.md` y `CLAUDE.md` §3.
- `a9913cf1`: un comentario de `Renew` (por qué sin `greatest` la actividad no retrocede por una carrera).

**El problema.** La fecha salta al futuro y una tableta captura: su comandero guarda `last_activity_at` en el futuro
(la hora de Go en ese momento). Se corrige el reloj. `inUseCondition` compara esa fecha con el corte de 15 minutos, así
que la fila sigue «en uso» hasta que el reloj real alcance la fecha del salto. Otra tableta recibe
`LICENSE_SEAT_UNAVAILABLE` por algo que causó la hora.

**Lo que se hizo: la actividad futura se baja a «ahora» y se guarda.** Una actividad de comandero posterior a la hora
de Go cuenta como actividad de ahora: se guarda `now` y vence 15 minutos reales después, como cualquier captura. Se
hace en tres lugares, y en los tres solo con la hora confiable (abajo):

1. **`claim`** (`licenseseats/store.go:119`, con `settleFutureActivity`, `store.go:65`): con 7820148311 ya tomado y la
   hora leída con él, antes del barrido y del conteo, un `UPDATE` baja a `now` las filas de comandero posteriores a
   `now`. Vale para toda asignación (captura, pedido sin conexión, abrir turno, modo Cocina). La terminal del salto que
   vuelve a capturar renueva con la hora corregida, y el conteo la ve como de ahora.
2. **`Renew`** (las previas, `store.go:238`): ya no usa `greatest`, que conservaba la fecha futura. Con la hora
   confiable la actividad queda en `now`; si no, sigue `greatest` (nunca retrocede), como antes.
3. **Después de un rechazo por falta de cupo** (`administration/service.go:264`, `Execute`; `license_seats.go:106` y
   `:118`; `licenseseats.SettleFutureActivity`, `store.go:78`): el rechazo revierte **toda** la transacción de la
   petición, también lo que `claim` bajó. Sin esto, cada intento de la otra tableta volvería a encontrar la fecha
   futura y el cupo seguiría ocupado. Por eso `Execute`, después de revertir la transacción de la petición, abre una
   corta aparte:
   - primero comprueba sin el lock si hay alguna actividad de comandero en el futuro (lo normal es que no, y entonces
     no hace nada más);
   - si la hay, toma 7820148311 (el único lock de esa transacción), lee la hora y la baja a `now`;
   - si falla, el rechazo se responde igual y el detalle va al registro (`license_seats_settle_failed`); el siguiente
     intento lo vuelve a hacer.

**Por qué es correcta.**

- **Nunca deja un cupo ocupado más de 15 minutos reales** después del primer intento con la hora corregida (de esa
  terminal o de cualquier otra), aunque el intento se rechace.
- **Nunca libera un cupo antes de tiempo.** La fila bajada a `now` sigue en uso (`now` es posterior al corte), así que el
  conteo no cambia en ese momento. No se sabe cuándo ocurrió de verdad esa actividad, porque el único reloj que había
  era el adelantado. «Ahora» es lo más que pudo haber durado, y es el lado seguro: cuenta de más, nunca de menos.
- **Por qué no basta la sugerencia del revisor** (`LEAST(last_activity_at, now)` en la consulta): para una fecha
  futura, `LEAST` da `now`, que siempre es posterior al corte, así que nunca vence. Hay que guardar el valor.
- **Por qué no basta hacerlo solo en `claim`** (la forma que sugirió el coordinador): en el caso del hallazgo (1 de 1),
  el intento de la otra tableta **se rechaza** y su transacción se revierte entera. Lo que bajó `claim` se pierde y el
  cupo queda ocupado igual que antes. Por eso existe la transacción aparte del punto 3. La mutación 1 (abajo) lo
  demuestra.
- **Locks** (CLAUDE.md §3):
  - en `claim`, el `UPDATE` va con 7820148311 ya tomado, al final de la operación, sin publicar eventos ni pedir otro
    lock;
  - la transacción aparte de `Execute` empieza después de revertir la de la petición (que tenía esas filas bloqueadas:
    si no, se esperaría a sí misma) y solo toma 7820148311;
  - `Renew` sigue sin el lock: es un `UPDATE` de su propia fila y no se intercala con las asignaciones de comandero
    (7820148305 compartido frente a exclusivo, R3-4). Un `UPDATE` de varias filas siempre va con 7820148311, así que dos
    no se pueden interbloquear entre sí.
  - Lo agregué en `CLAUDE.md` §3, en el párrafo del orden de adquisición. No cambié la fila de la tabla, para que el
    formato no vuelva a alinear toda la tabla.

**Decisión propuesta (R2-2): solo con la hora confiable (`Options.ClockTrusted`).**

- `licenseseats/seats.go:131` y `:196`. `administration.LicenseSeats` la conecta con `!manager.Clock().Blocked`
  (`license_gate.go:96`): la hora es confiable mientras la protección de la hora de la Fase C no esté en «Hora del equipo
  incorrecta». Sin gestor de licencias (y en las pruebas sin valor), siempre es confiable.
- **El porqué.** Localmente no se distingue un reloj que saltó al futuro y se corrigió (la equivocada es la actividad)
  de un reloj que alguien atrasó (la equivocada es la hora de ahora). En el segundo caso, bajar la actividad liberaría
  antes de tiempo el cupo de una tableta que sí está capturando. La Fase C ya resuelve eso:
  - con el reloj atrasado más de 10 minutos respecto de la última hora confiable, bloquea todo lo que registra con
    fecha;
  - la última hora confiable solo baja con la comprobación con internet (§6.3) o con el desbloqueo de soporte (§6.4),
    que es justo cuando se acepta que esa fecha estaba en el futuro.

  Las capturas y abrir turno ya no pasan la puerta con la hora bloqueada, pero **las previas sí** (no registran nada con
  fecha), y entrar en modo Cocina también. Sin esta condición, `Renew` bajaría la actividad con la hora atrasada.

- **Cuando la hora es confiable**, una actividad futura dentro del margen (hasta 10 minutos, por los ajustes normales
  del reloj) también se baja. Así el cupo vence a los 15 minutos reales y no a los 25.
- Es la única parte de la Fase C que toqué: `LicenseSeats` es el punto de conexión de los cupos. No toqué
  `internal/licensing/` ni `license_clock.go`.

**La prueba del revisor (`cc5c4c5f`) sigue fallando y creo que está mal planteada respecto de L1-D43.** No la cambié.

- Lo que hace: salta +48 h, la primera tableta captura, corrige el reloj a +1 min, avanza a +17 min **sin ninguna
  operación en medio** y exige, en la primera observación, 0 en uso y que la segunda tableta entre.
- **Ningún diseño que cumpla L1-D43 puede pasarla.**
  - A +17 min el sistema ve por primera vez una actividad con fecha +48 h. Con L1-D43 cuenta como de ahora (+17 min) y
    vence a los +32 min.
  - Para responder 0 tendría que saber que el reloj se corrigió 16 minutos antes, y ningún reloj de la prueba lo dice:
    el reloj simulado es el único que avanza, y saltó; ni la base ni `time.Now` se mueven.
  - Las únicas formas de pasarla son dar por vencida una actividad futura en cuanto se ve (cuenta de menos, contra
    L1-D43), o compararla con `now()` de PostgreSQL. Esto último va contra L1-D16, y además rompería
    `TestStageL1R3SeatClockAndSessionExpiry` (reloj de los cupos 13 h adelante).
- Hoy falla con «sigue en uso (1 de 1) y la otra tableta recibe: No hay cupo de comandero libre (1 de 1 en uso)».
- **Propuesta para el coordinador**: que el revisor la ajuste a L1-D43. Una observación con la hora corregida (un
  intento de la otra tableta a +1 min, rechazado, y después `SettleFutureActivity`, como hace `Execute`); a +15 min
  todavía en uso; a +16 min, libre. Es lo que comprueba mi prueba `TestSeatActivityFromAClockJumpCountsAsNow`. Otra
  opción es decidir que una actividad futura se dé por vencida en cuanto se ve, pero eso cambia L1-D43.
- **Aviso para fusionar**: mientras no se ajuste, `go-int ./internal/licenseseats` completo falla en esa prueba.

**No corregido (anotado, como pide el encargo).** Las sesiones que se emiten durante el salto también duran de más:
`issueSession` escribe `expires_at` con `time.Now()` + 12 h, y con el reloj adelantado 48 h esa sesión vence 60 h
después de la hora real. Al corregir el reloj sigue vigente hasta entonces, a menos que se cierre o se revoque. Ya no
retiene el comandero (R3-1: el comandero cuenta por actividad), pero sí retiene el cupo de **cocina** de una pantalla
que entró a Preparación durante el salto, porque ese cupo cuenta mientras su sesión esté vigente. Se libera al salir de
Preparación, al cerrar la sesión o con «Liberar cupo».

**Pruebas nuevas.**

- `licenseseats/clock_jump_integration_test.go`, `TestSeatActivityFromAClockJumpCountsAsNow`, con el gestor
  directamente:
  - **el intento rechazado de otra tableta empieza los 15 minutos**: con la hora corregida cuenta 1 en uso. El rechazo
    revierte lo que bajó `claim` (la fecha sigue a +48 h), y `SettleFutureActivity` la deja en ahora. A los 14 minutos
    sigue rechazada; a los 15 se libera y la otra tableta entra;
  - **la tableta del salto renueva con la hora corregida**: la previa deja la actividad en ahora y vence a los 15
    minutos; su captura también;
  - **con la hora bloqueada no se baja nada**: ni la previa, ni `SettleFutureActivity`, ni el rechazo. Una actividad
    pasada sí se renueva hacia adelante. Con la hora confiable otra vez, la previa la baja.
- `operations/stagel1_seats_clock_jump_test.go`, `TestStageL1SeatActivityFromAClockJump`, por la API real (dos
  escritorios sin caja, 1 comandero):
  - **el 409 de la otra terminal deja la actividad en ahora** (la transacción aparte de `Execute`). A los 14 minutos,
    409 otra vez; a los 15, `sessions-summary` dice 0, Terminales ya no muestra el cupo y la otra terminal captura;
  - **la terminal del salto que entra a la mesa** (`draft-preview`) renueva con la hora corregida y libera a los 15
    minutos.
- `administration/license_seats_clock_test.go`, `TestLicenseSeatsTrustTheClockOnlyWhenItIsNotBlocked` (unitaria): con
  el reloj del gestor 3 h atrás de la última hora confiable, la hora de los cupos no es confiable; dentro del margen, sí;
  sin gestor, sí.
- **Mutaciones** (cambios temporales, revertidos copiando el archivo original):
  1. sin la transacción aparte de `Execute`, `TestStageL1SeatActivityFromAClockJump` **falla**: la actividad sigue a +48
     h después del 409;
  2. con `Renew` como antes (`greatest` siempre), **fallan** las dos pruebas nuevas de integración (la previa conserva
     la fecha del salto);
  3. con `Renew` bajando la fecha también con la hora bloqueada, **falla** «con la hora bloqueada no se baja nada».
- **Dónde corrieron**: las tres pruebas nuevas, solas (`ISO_RUN`) y dentro de las corridas completas de la tabla de
  R3 (`licenseseats` sin la del revisor, `operations` y `go-unit`, que incluye la unitaria de `administration`). Todas
  `ok`.

**No verificado (R2-2).**

- **El recorrido completo con la puerta real de la Fase C.** No probé por la API el caso entero con el gestor de
  licencias y su reloj simulado: salto, captura, corrección, bloqueo, «Comprobar la hora con internet» y después los
  cupos. Lo cubren por partes:
  - la unitaria de `LicenseSeats`, que dice cuándo es confiable la hora;
  - las pruebas de cupos, con la hora confiable y bloqueada simulada.
- **Escritorio y Android**: no corrí Vitest, Playwright ni emuladores. El contrato de la API no cambió (mismo 409 y
  mismos `details`).
- **La carga**: el `UPDATE` extra en cada asignación casi nunca encuentra filas. La transacción aparte solo corre después
  de un 409 por cupo, y sin actividad futura se queda en un `SELECT EXISTS`. No se midió.

**Para el coordinador (R2-2).**

1. Que el revisor ajuste o retire `TestReviewR2DefectSeatActivityFromAClockJumpOutlivesTheCorrection`: es incompatible
   con L1-D43 (ver arriba). Mientras tanto, `go-int ./internal/licenseseats` completo falla en ella.
2. Aceptar o rechazar la Decisión propuesta (R2-2) (`ClockTrusted`, conectado en `LicenseSeats` de `license_gate.go`, el
   único cambio en un archivo de la Fase C). Al fusionar con el trabajo de `$SP/wt/l1-fr2`, revisar que
   `manager.Clock()` siga existiendo con el mismo sentido.
3. Las sesiones emitidas durante el salto (`expires_at` más largo) siguen sin corregir. Ya no retienen el comandero,
   pero sí el cupo de cocina de una pantalla que entró a Preparación durante el salto.

### Decisiones del coordinador sobre la revisión R3 y el hallazgo R2-2

Fusionadas en `df63422`. El servidor del árbol unido es idéntico a la cabeza que probó el agente (`44c05d8`): solo
difieren los archivos de Android de la Fase F. Por eso no se repitieron las suites de Go.

- **L1-D36 · R3-1 (alta): el comandero cuenta por actividad.**
  - Ocupa el cupo durante 15 minutos desde su última captura, tenga o no una sesión vigente.
  - Su fila se borra cuando se cierra la última sesión de esa terminal.
  - Revocar la terminal libera su comandero y su cocina.
  - Cambia L1-D27 («el comandero solo cuenta con una sesión vigente»).
- **L1-D37 · R3-2: el reloj de las sesiones (`SessionNow`) se separa del reloj de los cupos.** El vencimiento de una
  sesión se compara con el mismo reloj que lo escribe y lo valida.
- **L1-D38 · R3-3: el orden de los locks en `CLAUDE.md` §3.** 7820148306 va antes de 7820148311. Con 7820148311
  tomado, nunca se publica un evento ni se pide otro lock.
- **L1-D39 · R3-4: las previas renuevan sin lock**, con un `UPDATE` condicional de su propia fila.
- **L1-D43 · R2-2: una actividad de comandero con fecha futura cuenta como de ahora.** Se baja a «ahora», se guarda y
  vence 15 minutos reales después. Se hace en tres lugares:
  - en `claim`;
  - en `Renew`;
  - después de un rechazo por falta de cupo, en una transacción corta aparte (`SettleFutureActivity`), porque el
    rechazo revierte la petición entera.
- **L1-D49 · La fecha futura se baja solo con la hora confiable (`Options.ClockTrusted`).** Con la hora en «Hora del
  equipo incorrecta», el «ahora» de Go es el equivocado. Bajar la actividad entonces liberaría antes de tiempo el cupo
  de una terminal que sí está capturando.
- **La prueba del revisor de R2-2 (`cc5c4c5f`) se ajusta a L1-D43** (`44c05d8`).
  - Pedía el cupo libre 16 minutos después de corregir el reloj, sin que Go viera nada en medio. Con L1-D43, Go no
    puede saber cuándo se corrigió la hora.
  - Ajustada: después de corregir el reloj, la otra tableta intenta capturar y se rechaza; 16 minutos después entra.
    Pasa.
- **L1-D50 · Las sesiones emitidas durante un salto del reloj conservan su vencimiento largo** (hasta 60 h con un
  salto de 48 h). No se corrige en la L1:
  - ya no retienen el comandero (L1-D36);
  - el cupo de cocina de una pantalla que entró a Preparación durante el salto se libera al salir de Preparación, al
    cerrar la sesión o con «Liberar cupo» en Terminales.

  Va como propuesta para la V1.1.

---

## Fase E · Escritorio de la licencia

Hicieron la fase dos agentes del escritorio, en `$SP/wt/l1-e`, rama `etapa-l1-e`, con `ISO_TAG=LE` (Playwright en
`ISO_GO_PORT=8207` e `ISO_PREVIEW_PORT=1437`, el simulador de licencias en 8487 y el Go sin activar en 8211). El primero
hizo el puente, las pantallas, `native-driver.ts` y la especificación de la activación, y se detuvo por el límite de uso
el 23/11; el segundo confirmó ese trabajo, escribió las demás especificaciones, igualó `scripts/browser-tests.py` con el
arnés aislado y corrió las pruebas.

Todo lo nuevo se esconde con la capacidad `license-v1`: contra un servidor 0.14.0 el escritorio se ve como antes
(Configuración sin «Licencia», Terminales con el resumen de sesiones por tipo, Preparación sin avisar a Go).

### Commits

- `502aadc6` puente nativo de la licencia (rutas `license/*`, `kitchen/display-mode` y `seat/release` en la lista cerrada
  de `api.rs`; `license_files.rs` guarda la solicitud `.licreq` e importa el `.lic` y la respuesta de soporte con los
  diálogos nativos)
- `b9cf948c` escritorio de la licencia (activación, Configuración › Licencia, avisos, pantalla de la fecha y hora con
  el desbloqueo de soporte, cupos en Terminales y Preparación) con los textos por código en `services/license.ts`
- `7208e3ee` Merge branch 'etapa-l1' into etapa-l1-e (la Fase C)
- `91b72832` botón «Licencia» en Configuración general y «Comprobar la hora con internet» con el tiempo de espera del
  servidor de licencias en `api.rs`
- `df4f4505` `native-driver.ts` activa el Go temporal con una licencia de prueba de `license-test` y simula los diálogos
  de la licencia; especificación de la activación y resumen de cupos en 9G y 9H
- `9e242bba` especificaciones de los avisos, del bloqueo por la fecha y hora y de los cupos
- `74dab2aa` capturas desde arriba de la página y sin el aviso anterior en la del bloqueo con la licencia vencida
- `c047cd5f` `browser-tests.py` arranca el servidor de licencias simulado, la confianza de prueba y el reloj simulado de
  cada Go temporal y un Go sin activar, como el arnés aislado
- `08e67257` las capturas de la etapa sin avisos de error de un paso anterior y las de un diálogo solo con la ventana

### Qué se hizo

**El puente de Rust** (`desktop/src-tauri/src/api.rs`, `license_files.rs`, `main.rs`):

- en la lista cerrada de `api.rs`: `license_status`, `license_activate`, `license_transfer`, `license_refresh`,
  `license_deactivate`, `license_identity_regenerate`, `license_time_check`, `license_support_request`,
  `license_support_unlock`, `kitchen_display_mode` y `devices_seat_release` (con el identificador validado). La prueba
  `license_and_seat_routes_are_closed` comprueba las rutas y que la solicitud sin internet y la importación **no** están
  en la lista: solo las envía Rust;
- las operaciones que hablan con el servidor de licencias (activar, transferir, buscar actualización, liberar y
  «Comprobar la hora con internet») esperan hasta 80 s (`LICENSE_TIMEOUT_SECONDS`; Go les da 75 s);
- `license_files.rs`, como `legacy_migration.rs`: `license_save_request` pide a Go la solicitud
  (`POST license/offline-request`) y la guarda con el diálogo nativo con un nombre limpio; `license_import_file` y
  `license_import_unlock_file` abren el diálogo, leen el archivo (`.lic` hasta 64 KiB, `.desbloqueo` hasta 16 KiB, UTF-8
  y no vacío) y lo envían a Go. React nunca recibe una ruta ni el contenido.

**Las pantallas** (`desktop/src/pages/license/`):

- `ActivationStartPage.tsx` y `ActivationPanel.tsx`: el primer arranque «Sin activar» o «Sin activar con datos», con
  el código validado al escribirlo (L1-D4: se ordena solo en grupos de 3, 4, 4 y 4 y dice cuántos caracteres faltan o
  qué símbolo no se usa), «Activar sin internet» (guardar la solicitud e importar el `.lic`) y la instalación copiada
  con la identidad nueva (L1-D11), solo en la computadora principal, con su explicación y confirmación. «Continuar sin
  activar» entra al sistema con el aviso fijo;
- `LicensePage.tsx` (Configuración › Licencia, desde el botón «Licencia» de Configuración general): estado, plan,
  cliente, vencimiento o «Perpetua», días de tolerancia, cupos en uso / límite, y el entorno «Servidor de licencias de
  pruebas» solo cuando Go dice `environment: test`. Con «Administrar la licencia»: «Buscar actualización», «Transferir a
  esta computadora» (solo con `ACTIVATION_LIMIT` y `transfer_available: true`, con confirmación; con `TRANSFER_LIMIT`,
  «Comunícate con soporte»), «Liberar esta licencia» (confirmación en rojo), «Solicitud sin internet» (activación,
  renovación y liberación) e «Importar archivo de licencia». Ver el estado no pide permiso;
- `LicenseNotices.tsx`: los avisos en todas las pantallas (en la captura, en un renglón). Fijos: la hora, sin activar,
  vencida y revocada (según su motivo). Diarios, con «Entendido»: la tolerancia para todos y los 7 días antes del
  vencimiento para quien administra la licencia; para los demás, un indicador discreto;
- `ClockBlockPage.tsx`: «Sistema detenido por la fecha y hora de la computadora» con sus cinco partes (§6.2): las dos
  fechas que da Go, por qué se detiene, qué sí se puede hacer (con «Ir a Reportes» e «Ir a Respaldos»), cómo resolverlo
  (Mac, Windows, «Comprobar la hora con internet» y el desbloqueo de soporte con su código, la respuesta pegada o
  importada) y «Tu licencia está bien…» solo con la licencia vigente o en tolerancia; si no, el problema de licencia
  aparte. En la computadora principal se abre sola una vez por bloqueo; las terminales ven el aviso fijo y, al
  registrar, «No se puede registrar: la computadora principal tiene la fecha y hora incorrectas. Avisa al encargado.».

**Cupos** (§5): Terminales muestra el resumen «Cupos de la licencia» en lugar del de sesiones por tipo, la columna «Cupo»
y «Liberar cupo» de comandero o de cocina (con confirmación; cierra la sesión de esa terminal). Preparación
(`PreparationSeat.tsx`) avisa a Go al entrar y al salir (`kitchen/display-mode`, en orden) y, sin cupo, dice cuál falta
y sus cifras con «Volver a intentar».

**Textos**: todos salen de `services/license.ts` a partir del código de error y sus `details` (`licenseErrorText`),
nunca del `message` del servidor de licencias. `api.ts` (`rejectionOf`) los usa para cualquier rechazo y, con
`SYSTEM_CLOCK_INVALID` o un rechazo por el estado de la licencia, pide a la sesión que vuelva a leer el estado (igual
que el evento `license.changed`).

**Pruebas de interfaz**:

- `desktop/e2e/native-driver.ts`: **el único lugar donde las pruebas activan el Go temporal.** Al configurar una base
  nueva, entra como el administrador del asistente e importa una licencia perpetua con 20 cupos de cada tipo, firmada
  por `license-test issue` con las llaves de `vectors.json` para la carpeta `identity` de ese Go. Así las
  especificaciones anteriores siguen pasando. Simula `license_save_request`, `license_import_file` y
  `license_import_unlock_file` con las mismas comprobaciones que Rust, y expone `driver.license` (firmar, instalar,
  escribir un archivo y las colas de los diálogos), el reloj simulado y los controles del simulador;
- `desktop/e2e/license-flow.ts`: ayudas de la etapa (otras terminales por la API o con su página, cupos, captura,
  capturas en `ADEL_SHOTS_DIR` y `restoreTestLicense`, que deja la licencia de las demás especificaciones en el
  `finally`);
- cuatro especificaciones `workflow-stagel1-*.spec.ts` (abajo, en la tabla);
- `workflow-stage9g-terminals` y `workflow-stage9h-adjustments`: el resumen ahora es «Cupos de la licencia».

**Arneses**:

- `$SP/iso.sh browser` con `ISO_LICENSE_SIM=1` compila `license-test`, arranca el simulador antes del Go, le pasa la
  confianza de prueba, su certificado y un reloj simulado, y con `ISO_UNLICENSED_PORT` arranca un segundo Go sin
  activar. Sin esas variables hace lo mismo que antes;
- `scripts/browser-tests.py` (`dev.sh test-browser`) hace lo mismo: compila `license-test` en una carpeta temporal (nunca
  en `bin/`), arranca el simulador en **8189** antes de los servidores, y los tres servidores (8183, 8185 y el nuevo sin
  activar en **8187**) apuntan al simulador, confían en `vectors.json` y tienen su propio reloj simulado. Pasa a
  Playwright `ADEL_BROWSER_LICENSE_TOOL`, `_LICENSE_SERVER_URL`, `_LICENSE_CA`, `_CLOCK_FILE` y
  `_UNLICENSED_SERVER_URL`/`_PROVISIONING`, y al terminar borra todo. `dev.sh` no menciona `license-test`, como exige
  `TestTestToolsNeverShip`.

### Decisiones propuestas

1. **Las pruebas activan el Go temporal en un solo lugar.** `installDriver` importa, al configurar cada base nueva, una
   licencia perpetua con 20 cupos de cada tipo, firmada por `license-test issue` para la identidad de ese Go. Por qué:
   desde la Fase C, un Go sin licencia rechaza la administración y la captura (L1-D35), y las 45 especificaciones
   anteriores no deben saber de licencias. Con 20 cupos, ninguna se queda sin cupo. Las de la etapa cambian la licencia
   desde otra terminal de administración y, en su `finally`, dejan esa misma licencia (`restoreTestLicense`) y el reloj
   en la hora real.
2. **Un tercer Go temporal, sin activar, solo para la activación** (`ADEL_BROWSER_UNLICENSED_SERVER_URL`; 8187 en
   `browser-tests.py`, `ISO_UNLICENSED_PORT` en el arnés). Por qué: el compartido ya está activado cuando llega esa
   especificación. Sin la variable, sus pruebas se omiten, como las de instalación limpia de la 9H.
3. **`license-test` se compila en una carpeta temporal y nunca en `bin/`**, y `dev.sh` no lo menciona. Por qué: así no
   queda junto al servidor que empaquetan los instaladores y `TestTestToolsNeverShip` sigue igual.
4. **La prueba de «Comprobar la hora con internet» activa en línea con el simulador** (un código `PRP` inventado,
   distinto en cada corrida, con la hora del simulador 5 horas atrás). Por qué: con una activación, Go pide la hora con
   la consulta (`refresh`) de esa activación; con un `.lic` firmado por `license-test`, el simulador no conoce la
   activación y responde `INVALID_PROOF` (en el escritorio, «El servidor de licencias rechazó la operación…»). La hora
   atrasada del simulador deja la licencia emitida antes de la hora del paso 5, por el piso firmado de la §6.3.
5. **El desbloqueo de soporte aceptado no se prueba en Playwright.** Necesita la llave privada de soporte, que nunca está
   en las pruebas (§0 «Secretos»). La interfaz se prueba con el código de solicitud, una respuesta pegada que no vale
   y un archivo equivocado; la aceptación la cubren las pruebas de Go de la Fase C.
6. **La pantalla de la fecha y hora se abre sola una vez por bloqueo** en la computadora principal (no en «Solo
   servidor» ni en las terminales). Desde ahí se puede ir a Reportes o Respaldos, y el aviso fijo «Ver cómo resolverlo»
   regresa a ella. Las terminales solo ven el aviso fijo y el mensaje al registrar.
7. **Los avisos diarios** (tolerancia y 7 días antes) se muestran al entrar hasta que el usuario toca «Entendido» y no
   vuelven ese día, por usuario y en esa computadora (`localStorage`; si no está disponible, el aviso vuelve a
   aparecer). El de 7 días es para quien tiene «Administrar la licencia»; los demás ven un indicador discreto.
8. **«Licencia» se abre desde Configuración general** (botón junto a «Ver estado del sistema») y su ruta de navegación
   es Administración › Sistema › Configuración › Licencia. No se agregó un mosaico nuevo en Sistema.

### Pruebas

Todas con el arnés aislado (`ISO_TAG=LE`, clúster 55499), salvo donde se dice. Playwright con
`ISO_GO_PORT=8207 ISO_PREVIEW_PORT=1437 ISO_LICENSE_SIM=1 ISO_LICENSE_PORT=8487 ISO_UNLICENSED_PORT=8211`. Registros en
`$SP/le/`.

| Prueba                                                                                                                                                     | Resultado                                                                                                                                                                                           |
| ---------------------------------------------------------------------------------------------------------------------------------------------------------- | --------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `vitest` (con `tsc`), sobre `7208e3ee` + lo que quedaba sin confirmar (`vitest2.log`)                                                                      | 105 archivos, **999 pruebas aprobadas**. Las de la fase: `stageL1License.test.tsx` (21) y `license.test.ts` (15)                                                                                    |
| `cargo test` (`cargo2.log`)                                                                                                                                | **115 aprobadas**, 3 ignoradas (las de siempre). Incluye `license_and_seat_routes_are_closed` (con `license_time_check`) y las 6 de `license_files.rs`                                              |
| `build`                                                                                                                                                    | Lo hace la vista previa de Playwright (`npm run build`) en cada corrida: sin errores                                                                                                                |
| `tsc --noEmit` después de cada cambio de `e2e/`                                                                                                            | Sin errores                                                                                                                                                                                         |
| `format`, `fmt-check` y `sync-check`                                                                                                                       | Aprobados (última vez sobre `08e67257`). `md5` de `contracts/`, `docs/etapas/` y `docs/licencias/` igual antes y después (`md5-before2.txt`, `md5-after.txt`)                                       |
| `workflow-stagel1-*` solas, Chromium (`pw-l1-chromium-4.log`)                                                                                              | **5/5**                                                                                                                                                                                             |
| `workflow-stagel1-*` solas, WebKit (`pw-l1-webkit-1.log`)                                                                                                  | **5/5**                                                                                                                                                                                             |
| `workflow-stagel1-*` con las capturas finales, sobre `08e67257` (`pw-l1-chromium-final3.log`, `pw-l1-webkit-final3.log`)                                   | **5/5** en Chromium y **5/5** en WebKit                                                                                                                                                             |
| **Suite completa en Chromium**, sobre `c047cd5f` (`pw-full-chromium-1.log`, 21 min)                                                                        | **46 aprobadas, 4 omitidas, 0 fallas.** Las omitidas son las de instalación limpia de la 9H: el arnés aislado no arranca el Go limpio (pasan abajo, con `browser-tests.py`)                         |
| Relacionadas en WebKit: `9f-kitchen-stations`, `9g-terminals`, `9h-adjustments`, `9h-migration`, `a2-terminals` y `stagel1-*` (`pw-rel-webkit-1.log`)      | **15 aprobadas, 4 omitidas** (las mismas de instalación limpia)                                                                                                                                     |
| Relacionadas en Chromium                                                                                                                                   | Dentro de la suite completa: todas aprobadas                                                                                                                                                        |
| `scripts/browser-tests.py`, copia aislada en `$SP/le/bt-root` (ver «No verificado»), filtro `workflow-stagel1 workflow-stage9h-migration` (`bt-run-2.log`) | Chromium **12/12** y WebKit **12/12**, con las 4 de **instalación limpia** (el segundo Go, activado por `native-driver.ts`). Al terminar no quedaron bases, carpetas temporales ni puertos abiertos |

**Lo que cubre cada especificación nueva:**

| Especificación                                            | Cubre                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                     |
| --------------------------------------------------------- | ----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `workflow-stagel1-activation` (2 pruebas, Go sin activar) | Primer arranque: formato al escribir, código inexistente, «Activar sin internet» (la solicitud lleva el código y la firma de la instalación), un archivo que no es `.lic`, uno de otra computadora y el bueno. Configuración › Licencia: estado, plan, cliente, tipo, entorno y cupos sin identificadores a la vista; revocada (aviso fijo por motivo); activar en línea; «Buscar actualización»; «Liberar esta licencia» con su confirmación (cancelar y confirmar); `ACN` → «Comunícate con soporte»; `ACT` → «Transferir a esta computadora» con confirmación; solicitud de renovación |
| `workflow-stagel1-notices`                                | 7 días antes: aviso diario para el administrador, «Entendido» y no vuelve al entrar otra vez ese día; indicador discreto para un mesero sin el permiso. Tolerancia: aviso diario para los dos, cada uno lo cierra por su lado, con los días que quedan. Vencida: aviso fijo sin «Entendido», con lo que sí y lo que no                                                                                                                                                                                                                                                                    |
| `workflow-stagel1-clock`                                  | Reloj 3 h atrás: la pantalla se abre sola con sus cinco partes y las fechas de Go; «Comprobar la hora con internet» sin coincidir; desbloqueo de soporte (código de 24 caracteres, respuesta pegada inválida, archivo `.txt` y `.desbloqueo` dañado); Reportes con el aviso fijo y «Ver cómo resolverlo»; otra terminal con el aviso fijo y el mensaje al capturar; corregir el reloj lo quita solo en las dos; «Comprobar la hora con internet» coincidiendo lo quita; con la licencia vencida, los dos problemas por separado                                                           |
| `workflow-stagel1-seats`                                  | La tableta ocupa el último cupo de comandero y la barra ve «No hay cupo de comandero libre (n de n en uso)…»; Terminales con «Cupos de la licencia», la columna «Cupo» y «Liberar cupo» con confirmación (cierra la sesión de la tableta); después la barra captura. Preparación sin cupo de cocina con sus cifras, «Liberar cupo» de cocina, «Volver a intentar» abre el tablero y salir de Preparación libera el cupo en Go                                                                                                                                                             |

**Capturas** (tema claro, en `$SP/shots-l1/escritorio/`, 21 por navegador, prefijo `chromium-` o `webkit-`): activación
(primer arranque, código válido, sin internet), Configuración › Licencia (configuración, revocada, liberar, transferir,
administrar), avisos (vencimiento, indicador, tolerancia, vencida), hora (pantalla, desbloqueo de soporte, terminal,
desbloqueada, hora y licencia) y cupos (comandero, cocina, Terminales y la confirmación). Las revisé: ningún texto
provisional ni identificador interno. Lo único con aspecto de código son el código de solicitud de soporte (es para
dictarlo) y los códigos inventados del simulador escritos en el campo (`ZZZ-…`, `ACT-…`).

### No verificado y pendiente

- **`scripts/browser-tests.py` no se corrió tal cual**: usa `.local/server.json` y el clúster 55432, que esta etapa no
  toca. Se probó una copia en `$SP/le/bt-root` que solo cambia las cuatro constantes de puertos (a 8207, 8477, 8211 y
  8487), con un `database.py` falso sobre el clúster aislado 55499, un `dev.sh` falso que solo compila el servidor y un
  `npm` falso que agrega el filtro `workflow-stagel1 workflow-stage9h-migration`. Pasó en los dos navegadores (tabla de pruebas) y
  dejó todo limpio (bases, carpetas temporales y puertos). La corrida completa con `dev.sh test-browser` queda para la
  Fase H o el propietario.
- **`scripts/windows/dev.mjs` no se cambió** (Windows no se puede probar aquí). Sin `ADEL_TEST_LICENSE_TRUST`, sus Go
  temporales no aceptan la licencia de prueba y `installDriver` falla en todas las especificaciones. Propuesta: pasar
  `ADEL_TEST_LICENSE_TRUST=<ROOT>/contracts/licencias/v1/fixtures/vectors.json` a los servidores de `startBrowserServer`
  (sin mencionar `license-test`, que `TestTestToolsNeverShip` prohíbe en ese archivo; `native-driver.ts` compila la orden
  por su cuenta). Sin el simulador ni el Go sin activar, la activación y el bloqueo por la hora se omiten; los avisos y
  los cupos sí corren. **Lo decide el coordinador.**
- **El arnés aislado sin `ISO_LICENSE_SIM=1`** arranca el Go como antes, sin la confianza de prueba: sobre `etapa-l1`,
  cualquier corrida de Playwright (la regresión de la Fase H, por ejemplo) necesita `ISO_LICENSE_SIM=1` (o
  `ISO_LICENSE_TRUST`).
- **Sin Playwright** (los cubren `vitest` y Go): el desbloqueo de soporte aceptado (decisión 5), la instalación copiada
  con la identidad nueva (no se puede provocar sin alterar la carpeta `identity` del Go temporal) y el mensaje sin cupo
  de caja al abrir turno (solo el texto, en `license.test.ts`).
- **CLAUDE.md** no se tocó: los puertos nuevos de `browser-tests.py` (8187 y 8189) y el tercer Go van en §6 al unir la
  fase.
- **Observación para Go (no se cambió)**: con una activación vigente, «Comprobar la hora con internet» pide la hora con
  la consulta de esa activación (`ServerTime` en `licensing/client.go`). Si el servidor de licencias rechaza la consulta
  (por ejemplo, una activación que no conoce), la comprobación falla aunque el desafío (`challenge`), que no depende de
  la activación, daría la hora. Podría usarse el desafío como respaldo ante un rechazo
  que no sea de transporte. Lo decide el coordinador; el escritorio ya muestra el texto del código.
- **Para la unión**: la fase no cambia nada de `backend/`. La rama trae `etapa-l1` hasta la Fase C (`7208e3ee`); no
  tiene `c0217c0d` (avance de la Fase G, solo documentación), que no choca con nada de aquí.
- **La suite completa de Chromium** corrió sobre `c047cd5f`; el último commit (`08e67257`) solo cambia cómo se toman
  las capturas en tres especificaciones de la etapa, que volvieron a pasar en los dos navegadores.

### Decisiones del coordinador sobre la Fase E

Fusionada en `20d2725`, sin conflictos. La rama no tocaba `backend/` ni ningún archivo de las Fases D y F.

**Comprobación del árbol unido.** El árbol con D, E y F unidas (`20d2725`, en `$SP/wt/l1-m`, `ISO_LICENSE_SIM=1`)
corrió:

- las cinco especificaciones de la etapa;
- `workflow-stagea2-terminals`, `workflow-stage9f-kitchen-stations`, `workflow-stage9g-terminals` y
  `workflow-stage9h-adjustments`.

Resultado: **12/12 en Chromium y 12/12 en WebKit**, con los cupos por actividad de la Fase D.

- **L1-D51 · Se aceptan las ocho decisiones propuestas del agente.**
  - Un solo lugar activa el Go temporal: una perpetua de prueba con 20 cupos de cada tipo.
  - Un tercer Go sin activar, solo para la activación.
  - `license-test` nunca se compila en `bin/`.
  - La comprobación de la hora activa en línea con el simulador.
  - El desbloqueo de soporte aceptado se prueba solo en Go.
  - La pantalla de la hora se abre sola una vez por bloqueo.
  - Los avisos diarios se cierran con «Entendido».
  - «Licencia» se abre desde Configuración general.
- **L1-D52 · Windows: `scripts/windows/dev.mjs` pasa `ADEL_TEST_LICENSE_TRUST` a sus Go temporales** (`a5ef6e6`).
  - Así `native-driver.ts` los puede activar con una licencia de prueba.
  - No arranca el simulador ni el Go sin activar: ahí se omiten las especificaciones de la activación y de la hora.
  - No se probó en Windows. Sí pasó `TestTestToolsNeverShip`.
  - `CLAUDE.md` §6 anota los puertos 8187 y 8189.
- **L1-D53 · «Comprobar la hora con internet» no usa el desafío como respaldo.**
  - Con una activación vigente, la hora sale de la consulta de esa activación, como ya se hace.
  - Si el servidor de licencias la rechaza, la persona ve el texto de su código y puede usar el desbloqueo de soporte
    (§6.4).
  - Tomar la hora de otra respuesta cambiaría el diseño de seguridad de la §6.3 al final de la etapa. Va como
    propuesta para la V1.1.
- **Para la Fase H**: toda corrida de Playwright sobre `etapa-l1` con el arnés aislado lleva `ISO_LICENSE_SIM=1` e
  `ISO_UNLICENSED_PORT`; sin ellos, el Go temporal no se activa.

---

## Revisión R2 · La protección de la hora (§6 de la especificación)

La hizo el revisor independiente R2 en `$SP/wt/l1-r2`, rama `etapa-l1-r2`, sobre `1b2670e2` (la cabeza de la Fase C, con
las Fases A y D). Se revisó lo de la hora en `git log 406e730..1b2670e2`:

- `licensing/clock.go`, `unlock.go` y la parte de la hora de `operations.go`;
- `administration/license_gate.go`, `license_clock.go` y `license_monitor.go`;
- `cmd/soporte-desbloqueo/`;
- y del gestor y el almacén de la Fase A, lo que alimenta, guarda y reemplaza la última hora confiable.

Además, por indicación del coordinador, se leyó el adaptador `LicenseClientClock` y la comprobación fuera de la
transacción de `etapa-l1` (`91f17ba`). Las cinco pruebas que fallan se corrieron también sobre `91f17ba`, en un árbol
temporal ya borrado.

- `ISO_TAG=R2`, sin Go temporal y sin emuladores. Ninguna llamada a `licensing.sicrat.com`.
- No se cambió la hora de la Mac: todo con relojes simulados.
- No se leyó ni se usó la llave privada de soporte de desarrollo: las pruebas usan llaves de prueba propias.
- No se cambió código de producción: solo pruebas nuevas. MD5 de `docs/etapas/`, el contrato y `contracts/` iguales
  antes y después de `format`.

### Hallazgos

Las pruebas «…Defect…» **fallan hasta que se corrija su hallazgo**. Por eso hoy `go-unit` falla en `./internal/licensing`,
y `go-int` en `./internal/operations` y `./internal/licenseseats`, solo por esas pruebas.

| N.º      | Gravedad                          | Dónde                                                                                                                                                                                                                                                                                                           | Escenario (entrada → resultado incorrecto)                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                         | Prueba (commit)                                                                                                                                                           | Corrección propuesta                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                      |
| -------- | --------------------------------- | --------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------ | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | ----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| **R2-1** | **Media**                         | `license_clock.go:158` (`ReplaceTrustedTime`) antes de `:163` (`futureOperations`) y `:169` (auditoría); `license_clock.go:321` (`AcceptSupportUnlock`, que escribe en `unlock.go:517`) antes de `:343`. En `etapa-l1`: `completeTimeCheck` (igual orden en su transacción corta) y `supportUnlock` sin cambios | Las dos únicas formas de **bajar** la hora confiable la escriben en `estado.json` de inmediato. Su auditoría va después, en una transacción. Si después de escribir el archivo falla la consulta de operaciones futuras, la auditoría o el commit, la transacción se revierte. Causas posibles: se agota el plazo, porque en la Fase C el servidor de licencias puede consumir 8 de los 10 s; una desconexión de la base; un lock. En la prueba, la inserción en `audit_events` espera un lock hasta que se agota el plazo. Resultado: la hora ya bajó, también en disco, pero **no queda `license.clock_corrected` con los dos valores ni la lista de operaciones con fecha futura**, ni `license.support_unlocked`. La §6.3 exige «se registra en la auditoría con los dos valores». La persona ve un **503 genérico** aunque el cambio sí se aplicó. En la §6.4, además, la respuesta de un solo uso ya quedó gastada: al reintentar, «Esta respuesta de soporte ya se usó». El vigilante solo anota después `clock_unblocked`, sin el valor anterior ni las operaciones. **Se reproduce igual en `etapa-l1`.** | `TestReviewR2DefectClockLoweringIsNotAtomicWithItsAudit`, `internal/operations/review_r2_clock_integration_test.go` (`52e38063`, `1daf12cd`), por la API v1               | Que la baja y su auditoría sean una sola cosa. **Opción A**: preparar todo antes de escribir el archivo (consultar las operaciones futuras y armar la auditoría) y escribir la auditoría en una transacción corta propia, confirmada **antes** de `ReplaceTrustedTime`. Si la escritura del archivo falla después, se anota otro registro de «no se aplicó». **Opción B**: si algo falla después de escribir, devolver la hora anterior (`ReplaceTrustedTime(previous)`) y, en la §6.4, el número a pendiente. En las dos, la respuesta no debe ser un error si el cambio quedó aplicado. |
| R2-2     | Baja                              | `licenseseats/store.go:26` (`last_activity_at > cutoff`); también afecta las sesiones emitidas durante el salto (`expires_at` con `time.Now`)                                                                                                                                                                   | Es el caso difícil de la §6.3, y lo que pide la revisión: «qué pasa si el reloj salta hacia adelante y vuelve». La fecha salta dos días al futuro y una tableta captura (`last_activity_at` = +48 h). Se corrige el reloj y la tableta ya no captura. **16 minutos reales después** su comandero sigue «en uso» (1 de 1) y otra tableta recibe `LICENSE_SEAT_UNAVAILABLE` («No hay cupo de comandero libre (1 de 1 en uso)»). Dura hasta que el reloj real alcance la fecha del salto, o hasta cerrar la sesión o usar «Liberar cupo». El usuario ve un problema de **licencia** causado por la **hora**. Toca también a R3. **Se reproduce igual en `etapa-l1`.**                                                                                                                                                                                                                                                                                                                                                                                                                                                 | `TestReviewR2DefectSeatActivityFromAClockJumpOutlivesTheCorrection`, `internal/licenseseats/review_r2_seats_integration_test.go` (`4e2cf376`)                             | En `inUseCondition`, comparar `LEAST(last_activity_at, now)` con el corte: una actividad «en el futuro» cuenta como de ahora y vence a los 15 minutos. Opcional: al reemplazar la hora confiable (§6.3 y §6.4), llevar a la hora nueva los `last_activity_at` posteriores.                                                                                                                                                                                                                                                                                                                |
| R2-3     | Baja (límite §12; para el avance) | `license_clock.go:122` (piso), `manager.go:337` (`loadLicense` sube a `issued_at`) y `store.go:32` (`estado.json` sin firma)                                                                                                                                                                                    | **El límite que anota el autor es así**, con dos precisiones. 1. El piso es `issued_at − 10 min`, no `issued_at`. 2. **No hace falta un servidor falso.** `estado.json` es de la cuenta que corre el servicio (en macOS, la del usuario que instaló, sin `sudo`). Basta editar `last_trusted_time` y reiniciar: al cargar, la licencia sube la hora a su `issued_at` y ahí queda. Más abajo sí se bloquea. Con una **perpetua**, `issued_at` es el de la activación y puede ser de hace meses, porque `refresh` devuelve la misma revisión. Combinado con R1-3 (un `vigente.lic` viejo y el historial editado), el piso es el `issued_at` de una revisión más vieja. Para explotarlo hay que atrasar el reloj (en macOS pide un administrador) y aislar la red del servidor de licencias, porque la consulta diaria vuelve a subir la hora. No encontré otra forma de bajar la hora: ni las fuentes (`ObserveTime` solo sube), ni un respaldo, ni un reinicio, ni la §6.4 sin firma.                                                                                                                               | `TestReviewR2LimitEditingTheStateLowersTrustedTimeToTheLicenseIssuedAt`, `internal/licensing/review_r2_clock_test.go` (`7b557100`), **pasa**: documenta el comportamiento | Escribir el límite completo en el avance: «quien puede editar `identity/licencia` y atrasar el reloj baja la hora hasta el `issued_at` de la licencia cargada». Si se quiere endurecer, hacer como R1-3 propone para las revisiones: una segunda copia de la hora confiable en la base y usar el máximo de las dos. `ReplaceTrustedTime` reemplazaría las dos. **Decisión del coordinador.**                                                                                                                                                                                              |
| R2-4     | Baja                              | `manager.go:265` (`seedTrustedTime`) y `manager.go:606` (`tick`)                                                                                                                                                                                                                                                | `max(audit_events.occurred_at)` inicia la hora confiable solo en `Load` y solo si falta (L1-D15). Si esa consulta falla en ese arranque (plazo de 5 s, `lock_timeout` de 2 s, base lenta), no se reintenta. Al minuto, `tick` toma el reloj local como primera hora confiable. En la prueba, la base tiene actividad hasta las 15:00 y el reloj dice 12:00 → hora confiable 12:00, **sin bloqueo**, también tras reiniciar. Pasa la primera vez que arranca la 0.15.0 (todavía sin `estado.json`), o tras un `estado.json` dañado.                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                 | `TestReviewR2DefectTrustedTimeSeedIsNotRetried`, `review_r2_clock_test.go` (`7b557100`)                                                                                   | Recordar que la semilla quedó pendiente. Mientras lo esté, `tick` la reintenta **antes** de observar el reloj, y no observa el reloj ni las operaciones hasta tener respuesta de la base (con actividad o sin ella).                                                                                                                                                                                                                                                                                                                                                                      |
| R2-5     | Baja                              | `unlock.go:398` (`SupportRequest`) y `:473` (`AcceptSupportUnlock`): no miran `copied` ni `machineUnverified`; `Accept` sí (`manager.go:516`)                                                                                                                                                                   | Se copia la carpeta de datos a otra computadora con una solicitud pendiente. **La misma respuesta de un solo uso se acepta en las dos**: tienen la misma identidad y el mismo número pendiente. La §6.4 dice «solo en esa instalación». Hoy el efecto es pequeño, porque la copia está «Sin activar» y no registra nada con fecha.                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                 | `TestReviewR2DefectCopiedFolderAcceptsTheSameSupportResponse`, `review_r2_clock_test.go` (`7b557100`)                                                                     | Que `SupportRequest` y `AcceptSupportUnlock` respondan `ErrCopiedInstallation` y `ErrMachineUnverified` como `Accept`, con su mensaje en `license_clock.go`.                                                                                                                                                                                                                                                                                                                                                                                                                              |
| R2-6     | Baja                              | `service.go:282` (`runProtected`) con `LICENSE_LOCAL_ONLY` (403, `license_clock.go:57`)                                                                                                                                                                                                                         | Las tres rutas de la hora pedidas desde una terminal de red responden 403 dentro de `runProtected`. Por eso cada intento queda en la auditoría como **`access.denied`** («acceso denegado»), un problema de permisos, aunque el mensaje para la persona sí es claro. La propia puerta usa 409 justo para no caer ahí (comentario de `license_gate.go:42`). Comprobado leyendo el código.                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                           | Sin prueba (lectura del código)                                                                                                                                           | Usar 409 para `LICENSE_LOCAL_ONLY`, o devolverlo antes de `runProtected`, como `ENROLLMENT_LOCAL_ONLY` en `dispatch`.                                                                                                                                                                                                                                                                                                                                                                                                                                                                     |

### Observaciones (sin número, para decidir o para otras fases)

- **Escrituras que siguen con la hora incorrecta, fuera de la lista literal de L1-D14.** Revisé las 405 operaciones de la
  prueba de completitud: toda escritura sin fecha está en esa lista, salvo estas cinco. Ninguna registra una operación
  del negocio. Conviene escribirlo en el avance.
  - `setup` y `devices/enroll`: Decisión propuesta 4, aceptada.
  - `sessions/{id}/revoke` y «Liberar cupo» (L1-D6).
  - `kitchen/display-mode`: solo el cupo.
- **La §6.3 también baja la hora cuando no hay bloqueo** (`license_clock.go:157`). Con el reloj bien y unos segundos
  adelantado respecto al servidor, «Comprobar la hora» baja la hora unos segundos y deja un `license.clock_corrected`
  sin motivo. Sería más claro reemplazarla solo si está más de un margen por delante del `server_time`.
- **El `issued_at` de la respuesta de soporte se lee y no se usa** (`unlock.go:321`). No es un defecto: el número de un
  solo uso y la ventana de 24 h ya garantizan que sea reciente. Una respuesta firmada «con fecha futura» se acepta.
  - Si su `new_trusted_time` queda por delante del reloj, el bloqueo sigue hasta corregir el reloj.
  - Así lo dice `DESBLOQUEO_SOPORTE.md`, y la herramienta avisa al firmarla.
- **Un salto al futuro también cambia el estado de la licencia**: `ahora_efectivo` usa la hora confiable (§5.6).
  - Una suscripción cerca de su vencimiento aparece en Tolerancia o Vencida mientras dure el bloqueo, y el vigilante
    anota `license.state_changed`.
  - `SYSTEM_CLOCK_INVALID` lleva `license_ok: false`, y la pantalla de la Fase E mostraría «dos problemas».
  - Conviene que la pantalla diga que corregir la hora puede resolver también el de la licencia.
- **En `etapa-l1` (`91f17ba`)**:
  - `LicenseClientClock` traduce bien los errores de la Fase B, y la red ya no retiene la transacción, que es un
    cambio positivo.
  - `ServerTime` con activación usa `refresh`, y su `server_time` solo sube la hora (después de verificar la
    licencia). Un servidor falso que repite la licencia vigente con otra hora pasa esa verificación: por eso el piso
    de R2-3 es lo único que lo limita.
  - El plazo de la consulta subió de 8 a 60 s (`timeCheckTimeout`). En la prueba, la petición de la §6.4 con la
    auditoría bloqueada tardó 75 s en fallar: deduzco que las rutas `license/*` tienen un plazo mayor que 10 s (no lo
    leí en `http.go`).
- **Trabajadores en segundo plano durante un bloqueo** (comprobado leyendo el código): el de inventario procesa
  solicitudes ya registradas, con su `occurred_at`. `ReconcileCancellations`, «Agotado» y las liberaciones programadas
  dependen del reloj atrasado, así que se disparan más tarde, no antes. No encontré registros nuevos con fecha atrasada
  creados por ellos.

### Lo que se comprobó y está bien

- **De dónde sale la hora confiable y que solo suba**:
  - `ObserveTime` solo sube (`store.go:124`).
  - `server_time` e `issued_at` se guardan de inmediato. El reloj (`tick`, cada minuto) y las operaciones con fecha
    aceptadas (la puerta, `license_gate.go:139`) se guardan cada minuto y al apagar: `main.go` espera a `licenseDone`.
  - `ReplaceTrustedTime` solo acepta los motivos de la §6.3 y la §6.4.
  - `max(audit_events)` solo la inicia cuando falta (salvo R2-4).
- **Un reinicio y un respaldo restaurado no la bajan**: está en `identity/licencia`, que no viaja en los respaldos. La
  semilla no se repite si ya existe.
- **El salto hacia adelante y la vuelta**: en un minuto la hora confiable sube al salto, y al corregir el reloj se
  bloquea. Se quita con la §6.3, con la §6.4 o cuando el reloj alcanza la hora confiable menos el margen. Salvo R2-2.
- **La regla y el margen**: `CheckClock` bloquea con `local < última − 10 min`, con el límite exacto probado. No guarda
  nada: el bloqueo se quita en cuanto el reloj se corrige.
- **El bloqueo**: la puerta revisa la hora antes que el estado, en los dos puntos de `dispatch`. Así cubre la entrada
  con PIN, la credencial de envío, el agente de impresión, los avisos y toda ruta con sesión.
  - **Lo que registra con fecha** (`Dated`): F1, F2 (también cobrar, L1-D1), F3, F4, F5 con escritura, F7 y toda
    escritura desconocida (F1 con fecha).
  - **Lo que sigue**: las lecturas, previas, reportes, exportaciones, respaldos, licencia, `auth/*`, preferencias,
    «Reiniciar», el SSE y el agente de impresión.
  - Las rutas que no pasan por `dispatch` (SSE y avisos) son lecturas.
  - `HEAD` cuenta como lectura, y no encontré ningún manejador que lo trate como escritura (búsqueda en el código).
  - Solo hay un servidor Go que decide: las terminales usan su hora.
- **El vigilante**: revisa al arrancar, cada 15 s y cuando la puerta ve un bloqueo, en una transacción propia. Audita
  `license.clock_blocked` y `license.clock_unblocked` con las dos fechas y sus textos en la zona del restaurante.
- **§6.3**:
  - Sin cliente responde 503.
  - Si no coincide, 409 `LICENSE_TIME_MISMATCH` con la hora del servidor, y se audita.
  - Si coincide, reemplaza la hora y lista las operaciones futuras sin cambiarlas (salvo R2-1).
  - El piso del `issued_at` funciona: un `server_time` falso por debajo de `issued_at − 10 min` responde 502 y no cambia
    nada.
  - El `server_time` llega por la interfaz y se compara con el reloj leído después de la respuesta.
- **§6.4**:
  - Se acepta una sola vez, también tras reiniciar, solo con el número pendiente y solo con la etiqueta de esta
    instalación, dentro de 24 h medidas con el reloj local.
  - Una solicitud reemplazada responde `not_requested`.
  - Todo esto se rechaza como `altered` sin gastar la solicitud (`TestReviewR2SupportUnlockRejectsForgeries`, pasa):
    el entorno de otra versión en una respuesta con firma válida, una licencia pegada en su lugar, otro `typ`, un
    encabezado con espacios, el relleno de base64, un campo desconocido, una hora sin zona o anterior al origen, y
    otra llave o un `kid` que no es de soporte.
  - La herramienta nunca muestra la privada, exige 0600 y comprueba la respuesta con la pública compilada.
  - Una compilación de producción no compila sin `soporte-production.pub.json`: lo dice la prueba del autor; no la
    volví a correr.
- **Los mensajes**: `SYSTEM_CLOCK_INVALID` no empieza con `LICENSE_`, dice la causa en español sencillo y lleva las dos
  fechas completas, `license_state` y `license_ok`. Los rechazos de la §6.4 dicen qué hacer en cada caso.

### Lo que no se pudo verificar

- **La §6.3 contra el servidor de licencias real**: solo con un `server_time` simulado y leyendo el cliente de la Fase B.
- **La pantalla de la hora, el aviso fijo y Android** (Fases E y F). En particular, que la cola sin conexión de Android
  conserve un pedido rechazado con `SYSTEM_CLOCK_INVALID`, en lugar de descartarlo.
- **El host de Windows**, y `F_FULLFSYNC` con el mutex tomado bajo carga (anotado por R1).

### Pruebas ejecutadas

| Prueba                                                                  | Árbol                     | Resultado                                                                 |
| ----------------------------------------------------------------------- | ------------------------- | ------------------------------------------------------------------------- |
| `ISO_TAG=R2 ISO_RUN=TestReviewR2 iso.sh … go-int ./internal/operations` | `etapa-l1-r2` y `91f17ba` | Falla solo R2-1 (las dos subpruebas), como se espera                      |
| `… go-int ./internal/licenseseats`                                      | `etapa-l1-r2` y `91f17ba` | Falla solo R2-2, como se espera                                           |
| `… go-int ./internal/licensing` (`TestReviewR2…`)                       | `etapa-l1-r2` y `91f17ba` | Fallan R2-4 y R2-5, como se espera; pasan `…Limit…` y `…RejectsForgeries` |
| `format`, `fmt-check` y `go vet -tags=integration` de los tres paquetes | `etapa-l1-r2`             | Aprobados; MD5 iguales antes y después                                    |

No se corrió `go-int ./cmd/ad-el-pos-server`: ninguna prueba nueva está ahí, así que no hizo falta el candado.

### Commits (rama `etapa-l1-r2`)

- `52e38063` revisión R2 de la etapa L1: la baja de la hora confiable (§6.3 y §6.4) se guarda antes que su auditoría
  (R2-1, prueba que falla)
- `4e2cf376` revisión R2 de la etapa L1: el comandero que capturó durante un salto del reloj al futuro no se libera al
  corregirlo (R2-2, prueba que falla)
- `7b557100` revisión R2 de la etapa L1: semilla de la hora confiable sin reintento (R2-4), carpeta copiada con
  desbloqueo de soporte (R2-5), piso del issued_at y respuestas de soporte falsificadas
- `1daf12cd` revisión R2 de la etapa L1: la prueba de R2-1 va por la API v1 y vale también en etapa-l1, donde la
  comprobación con internet corre fuera de la petición

### Correcciones de la revisión R2

Las hizo el agente de las correcciones de R2 en `$SP/wt/l1-fr2`, rama `etapa-l1-fr2`, desde `etapa-l1` (`c0217c0`), con
`ISO_TAG=FR`. Un primer agente tomó las pruebas del revisor y se detuvo por el límite de uso; este agente hizo todo lo
demás.

- Sin Go temporal ni emuladores. Ninguna llamada a `licensing.sicrat.com`. No se leyeron los códigos de licencia de
  prueba ni la llave de soporte de desarrollo: las pruebas usan llaves propias. La hora de la Mac no se tocó: todo con
  relojes simulados.
- MD5 de `docs/etapas/`, el contrato, la guía copiada y `contracts/` iguales antes y después de `format` (54 archivos).
- Las pruebas del revisor se tomaron con `cherry-pick` (aquí `f9a48a42`, `2bf0fe6c`, `3a81e8eb` y `88ae45b9`). **R2-2 no
  se corrigió aquí**: su prueba queda con `t.Skip("R2-2 la corrige la Fase D")` en un commit aparte (`00eefa68`), sin
  otro cambio. Al unir, vale la versión de `etapa-l1`.

#### Qué se hizo

| Hallazgo                                   | Corrección                                                                                                                                                                             | Pruebas                                                                                                                                |
| ------------------------------------------ | -------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | -------------------------------------------------------------------------------------------------------------------------------------- |
| **R2-1** (media), la baja sin su auditoría | La §6.3 y la §6.4 confirman su auditoría **antes** de bajar la hora. Si la baja falla después, se audita que no se aplicó y se responde con error. En la §6.4 la respuesta no se gasta | La del revisor (ajustada, ver abajo), `TestSupportUnlockIsNotSpentWhenNotSaved` y `TestReplaceTrustedTimeNotSavedKeepsBothCopies`      |
| **R2-3** (baja), `estado.json` editado     | Segunda copia de la última hora confiable en `app.license_trusted_time`. Vale el **máximo** de las dos. La §6.3 y la §6.4 bajan las dos                                                | `TestTrustedTimeUsesBothCopies`, `TestDatabaseTrustedTimeCopy` (`go-int`), `TestStageL1Upgrade` y la del límite del revisor (ajustada) |
| **R2-4** (baja), semilla sin reintento     | La lectura de la base queda pendiente y el trabajador la reintenta cada minuto **antes** de observar el reloj. Mientras tanto, el reloj local no se vuelve la primera hora confiable   | La del revisor y `TestPendingSeedDoesNotTrustTheLocalClock`                                                                            |
| **R2-5** (baja), carpeta copiada           | `SupportRequest`, `CheckSupportUnlock` y `AcceptSupportUnlock` responden `ErrCopiedInstallation` y `ErrMachineUnverified`, como `Accept`                                               | La del revisor y `TestSupportUnlockOnlyInTheOriginalInstallation`                                                                      |
| **R2-6** (baja), `access.denied`           | `LICENSE_LOCAL_ONLY` es **409**                                                                                                                                                        | `TestStageL1LicenseGateWithoutSession`: las tres rutas desde la red, 409, y ningún `access.denied` nuevo                               |

##### R2-1: la baja y su auditoría van juntas

El orden de las dos rutas (`administration/license_clock.go`, comentario del principio):

1. Se comprueba y se prepara todo **sin cambiar nada**:
   - en la §6.3, la hora del servidor de licencias, el piso del `issued_at` y las operaciones con fecha futura;
   - en la §6.4, la respuesta, con `licensing.Manager.CheckSupportUnlock` (`unlock.go:488`), que no escribe nada.
2. La auditoría con los dos valores (`license.clock_corrected` o `license.support_unlocked`) se **confirma** en su
   propia transacción corta. Si no se puede confirmar, no se baja nada; en la §6.4 la respuesta sigue sin usar.
3. Solo entonces se baja la hora, en sus dos copias: `ReplaceTrustedTime` (`manager.go:817`) o `AcceptSupportUnlock`
   (`unlock.go:503`).
4. Si la baja falla, se vuelve a auditar **la misma acción** con `success: false`, `applied: false` y el motivo
   (`auditNotApplied`, `license_clock.go:142`). Se responde con error: `503 LICENSE_STATE_NOT_SAVED`, o
   `400 SUPPORT_UNLOCK_INVALID` si la solicitud cambió en medio. La hora no cambió.

Detalles:

- **La §6.4 ahora corre después de confirmar la petición** (`completeDeferred`, igual que la §6.3): `supportUnlock` solo
  autoriza y devuelve un `*supportUnlockAttempt`, y `completeSupportUnlock` (`license_clock.go:486`) hace el resto.
  `administration/reports.go` lleva el caso nuevo en `completeDeferred`, con un comentario de la revisión R2. Quien
  llame a `Execute` sin `ServeHTTP` recibe el intento sin completar, igual que con la §6.3. Ninguna prueba lo hacía.
- **Los rechazos de la §6.4** se siguen auditando como `license.support_unlock_rejected` con su motivo. Los nuevos
  motivos de la auditoría son `installation_copied` y `machine_unverified` (R2-5).
- **Antes de preparar la baja de la §6.3**, se guarda en las dos copias la hora confiable que está en memoria
  (`_ = manager.Flush()`, `license_clock.go:219`). Así, el valor anterior que registra la auditoría es el que queda
  guardado si la baja no se aplica. Las operaciones con fecha solo se guardan cada minuto.
- **Lo único que puede quedar**: si el servidor se apaga entre la auditoría y la baja, queda una auditoría de una baja
  que no se aplicó. Nunca una baja sin su auditoría.
- **El orden dentro de la baja** (`replaceTrustedTimeLocked`, `manager.go:500`): primero la copia de la base y después
  `estado.json` (con el número de soporte marcado como usado en la misma escritura).
  - Si falla la de la base, no cambia nada.
  - Si falla `estado.json` después, la base ya bajó, pero vale el máximo de las dos: tampoco cambió nada. La copia de
    la base se vuelve a subir en el momento o, si no se puede, en el siguiente minuto.

##### R2-3: la segunda copia de la última hora confiable

- **La tabla**: `app.license_trusted_time`, en la sección «Fase A (revisión R1)» de la `0028`, punto 2
  (`0028_licencias.sql:38`). Tiene una sola fila (`id = 1`), con la hora, su fuente y `updated_at`. La fuente tiene una
  restricción con los nombres de `internal/licensing` (`server_time`, `issued_at`, `local_clock`, `operation`, `audit`,
  `internet_check` y `support_unlock`). El encabezado de la migración y el de la sección lo mencionan.
- **`PoolProbe`** (`datapresence.go:174`):
  - `TrustedTimeCopy` la lee en una transacción de solo lectura.
  - `RaiseTrustedTimeCopy` la sube y nunca la baja (`ON CONFLICT … WHERE trusted_time < EXCLUDED.trusted_time`).
  - `ReplaceTrustedTimeCopy` la reemplaza, también hacia atrás.
  - Las dos escrituras van en una transacción corta, con `lock_timeout` de 2 s y el plazo de 5 s.
- **La interfaz `DatabaseProbe`** suma esos tres métodos. Solo la implementan `PoolProbe` y el falso de las pruebas de
  `licensing`. Ninguna otra fase la implementa.
- **El gestor** (`manager.go`, sección «La última hora confiable en la base»):
  - Al arrancar (`Load`) se lee la copia de la base y `estado.json` sube hasta ella.
  - **Cada minuto** (`Flush`), y al apagar, la copia de la base sube hasta `estado.json`. Esa escritura corre **sin el
    mutex** del gestor, así que `Status` nunca espera a la base. Un segundo mutex (`timeMutex`, `manager.go:151`) ordena
    las lecturas y escrituras de la copia: una subida o una lectura que empezó antes de una baja nunca llega después de
    ella. Se toma siempre antes que el mutex.
  - Las horas se comparan al microsegundo, como las guarda PostgreSQL.
  - Una fuente desconocida en `estado.json` (editada a mano) se copia como `local_clock`, para que la copia se pueda
    seguir subiendo.
- **La semilla de la auditoría** (L1-D15) se usa ahora si falta **alguna** de las dos copias: la primera vez que arranca
  la 0.15.0, un `estado.json` dañado o borrado, o una base de antes de la `0028`. Con las dos copias no se vuelve a
  leer: la corrección de la §6.3 sigue pudiendo bajarla.
- **Respaldos**: la tabla viaja en los respaldos (`app`). Restaurar uno viejo baja solo esa copia, y en el siguiente
  minuto vuelve a subir. Restaurar un respaldo hecho **durante** un salto al futuro trae la copia de ese salto y vuelve
  a bloquear. Esa base también trae la actividad con fecha futura, así que es coherente: se resuelve con la §6.3 o la
  §6.4, como la primera vez.

**Lo que queda sin proteger** (§12 del contrato):

- **Quien puede escribir en la base** (con la cuenta de PostgreSQL de AD-EL POS) y además editar `estado.json` baja la
  hora hasta el `issued_at` de la licencia cargada. Así lo documenta la prueba del límite, ajustada. Con una perpetua,
  ese piso puede ser de hace meses. Con R1-3 y un `vigente.lic` viejo, es el de una revisión más vieja. La auditoría
  (`audit_events`), que es la semilla, también está en la base.
- **Una instantánea completa y vieja** de la computadora (Time Machine con `identity/` y la base) baja las dos copias:
  es la «instantánea antigua» de la §12.
- **Hasta un minuto**: la copia de la base sube cada minuto. Editar `estado.json` justo después de una operación pierde
  a lo más ese minuto, dentro del margen de 10 minutos.
- **Mientras la base no responda** al arrancar, vale solo `estado.json`, hasta que el trabajador la pueda leer (R2-4).
- **Sigue igual**: hay que atrasar el reloj (en macOS pide un administrador) y aislar la red del servidor de
  licencias, porque la consulta diaria vuelve a subir la hora. El piso de la §6.3 contra un servidor falso sigue en
  el `issued_at − 10 min`.

##### R2-4: la semilla se reintenta

- Si la lectura de la base falla al arrancar (la copia o `max(audit_events)`), no es un error del arranque: queda en la
  bitácora (`license_activity_unavailable`) y la lectura queda **pendiente** (`databaseTimeLoaded`).
- `tick` la reintenta cada minuto **antes** de observar el reloj (`retryDatabaseTime`, `manager.go:431`). La consulta
  corre sin el mutex.
- Mientras esté pendiente y no haya ninguna hora confiable, `ObserveTime` no toma el reloj local ni las operaciones
  (`manager.go:794`). Las operaciones que se acepten mientras tanto quedan en la auditoría, que es la semilla.
- El `server_time` y el `issued_at` sí se toman: no son el reloj local.
- Si `estado.json` ya tenía una hora confiable, el reloj se sigue observando: solo sube.
- La §6.3 o la §6.4 dan la lectura por hecha, porque fijan la hora en las dos copias.

##### R2-5: el desbloqueo, solo en la instalación original

- `supportInstallationProblemLocked` (`unlock.go:446`) se niega sin identidad, con la carpeta copiada y sin la huella
  comprobada, como `Accept`. Después vuelve a leer el estado si hace falta (R1-1).
- Lo usan `SupportRequest`, `CheckSupportUnlock` y `AcceptSupportUnlock`.
- **Los mensajes** están en `supportInstallationFault` (`license_clock.go:99`):
  - copiada: **409** `LICENSE_INSTALLATION_COPIED`, «Esta instalación se copió de otra computadora. Crea una identidad
    nueva para esta computadora en la pantalla de la licencia y después pide el código de desbloqueo.»;
  - sin huella: **503** `IDENTITY_UNAVAILABLE` con `details.reason: "machine_unverified"`, como en la Fase B.

##### R2-6: desde otra terminal, 409

- `LICENSE_LOCAL_ONLY` es **409** (`license_clock.go:71`), con el mismo mensaje: «Esto se hace en la computadora
  principal, en la pantalla de la fecha y hora.».
- Un 409 sin `Commit` revierte la petición y no pasa por la auditoría de `runProtected`. Antes, cada intento quedaba
  como `access.denied`.

##### Documentación

- `docs/API_V1.md`: en las rutas del escritorio de la hora, el 409 `LICENSE_LOCAL_ONLY`, el 409
  `LICENSE_INSTALLATION_COPIED` y la auditoría antes de la baja, con el 503 si no se aplicó.
- `docs/licencias/DESBLOQUEO_SOPORTE.md`: el 409 desde otra terminal, «solo en la instalación original» y las dos
  copias.
- Las rutas de la hora no están en `openapi.json`: son del escritorio, no del contrato de Android.

#### Aviso del coordinador (Fase D, L1-D49)

`LicenseSeats` conecta `licenseseats.Options.ClockTrusted` como `!manager.Clock().Blocked`. **Estas correcciones no
cambian `Clock()` ni `Blocked`**: siguen siendo `CheckClock(reloj local, última hora confiable)`. `Blocked` es verdadero
solo en «Hora del equipo incorrecta».

Lo único que cambia es de dónde sale la última hora confiable, que ahora es el máximo de las dos copias:

- editar `estado.json` hacia atrás ya no quita un bloqueo;
- con la semilla pendiente, no se bloquea nada hasta que la base responda, igual que antes sin hora confiable.

#### Decisiones propuestas

1. **«No se aplicó» es la misma acción con `success: false`**, `applied: false` y el motivo (`not_saved` o el de un
   rechazo), no una acción nueva.
   - Así el Registro de movimientos ya la nombra («Última actividad corregida con la hora de internet» o «Desbloqueo de
     soporte aplicado», marcada como fallida).
   - No hay que tocar `desktop/src/services/auditActions.ts`, cuya prueba exige un nombre para cada acción de Go.
2. **La §6.4 corre después de confirmar la petición** (`completeDeferred`), como la §6.3. Es la forma de confirmar la
   auditoría antes de escribir `estado.json` sin una segunda transacción dentro de la de la petición.
3. **La semilla de la auditoría se usa si falta alguna de las dos copias** (amplía L1-D15). Si solo faltara
   `estado.json`, una copia de la base atrasada (porque su escritura venía fallando) quedaría como la única referencia.
4. **R2-4**: mientras la semilla está pendiente y no hay hora confiable, se ignoran el reloj local **y** las
   operaciones. El `server_time` y el `issued_at` sí cuentan.
5. **Se guarda la hora en memoria antes de preparar la baja de la §6.3**, para que el valor anterior auditado sea el
   guardado.
6. **La copia de la base sube cada minuto sin el mutex del gestor**, con un segundo mutex para el orden. Así `Status`
   nunca espera a la base.
7. **R2-5**: la carpeta copiada responde `LICENSE_INSTALLATION_COPIED` (409) con su texto propio. Sin la huella,
   `IDENTITY_UNAVAILABLE` (503), como en la Fase B. Los dos casos se auditan como `license.support_unlock_rejected` con
   `installation_copied` o `machine_unverified`.

#### Pruebas ajustadas

Se ajustaron porque la corrección cambia lo que comprobaban. A las del revisor no se les quitó ninguna aserción del
hallazgo.

- **`TestReviewR2DefectClockLoweringIsNotAtomicWithItsAudit`** (`operations`), primera subprueba.
  - **Por qué**: con la corrección, la petición fallida ya no baja la hora. La segunda subprueba no podía preparar su
    actividad «a +30 h», porque la hora confiable seguía a +48 h y la puerta la rechazaba con `SYSTEM_CLOCK_INVALID`.
    Esa subprueba solo funcionaba con el defecto.
  - **Qué se agregó**, al final de la primera: la persona vuelve a intentar sin el lock. La baja se aplica, con un
    `license.clock_corrected` y en las dos copias (las dos juntas). Así la segunda parte de la hora corregida, como
    antes.
- **`TestReviewR2LimitEditingTheStateLowersTrustedTimeToTheLicenseIssuedAt`** pasa a llamarse
  **`TestReviewR2LimitEditingBothCopiesLowersTrustedTimeToTheLicenseIssuedAt`** (`licensing`).
  - Documentaba que editar `estado.json` bajaba la hora, y R2-3 lo impide.
  - Ahora comprueba que editar solo `estado.json` ya no la baja, y que editar las dos copias sí la baja hasta el
    `issued_at`. Ese es el límite que queda.
  - Más abajo del piso se bloquea, igual que antes.
- **`TestStageL1LicenseGate`** (`operations`, Fase C), ayuda `alwaysAvailable`: exigía `support-request` 200 en todos
  los estados, también en la carpeta copiada. Ahora, en la copiada, exige `409 LICENSE_INSTALLATION_COPIED` de la ruta
  (R2-5). La puerta la sigue dejando pasar.
- **`TestStageL1LicenseGateWithoutSession`** (`administration`, Fase C): `LICENSE_LOCAL_ONLY` pasa de 403 a 409, en las
  tres rutas, y sin `access.denied` nuevo (R2-6).

#### Archivos

- `backend/internal/licensing/manager.go`: interfaz `DatabaseProbe`, `TrustedCopy`, `timeMutex`, la sección «La última
  hora confiable en la base», `Load`, `ObserveTime`, `ReplaceTrustedTime`, `Flush` y `tick`.
- `backend/internal/licensing/unlock.go`: `SupportRequest`, `supportInstallationProblemLocked`, `CheckSupportUnlock`,
  `AcceptSupportUnlock` y `prepareSupportUnlockLocked`.
- `backend/internal/licensing/datapresence.go`: los tres métodos de `PoolProbe`.
- `backend/migrations/0028_licencias.sql`: encabezado y sección «Fase A (revisión R1)», punto 2.
- `backend/internal/administration/license_clock.go` y `reports.go` (un caso en `completeDeferred`).
- Pruebas:
  - `licensing`: `review_r2_fixes_test.go` y `review_r2_fixes_unix_test.go` (nuevos), el falso de `manager_test.go`,
    `review_r2_clock_test.go` y `datapresence_integration_test.go`;
  - `database`: `stagel1_upgrade_test.go` y `stage9h_upgrade_test.go` (excluye la tabla nueva);
  - `administration`: `license_gate_acceptance_test.go`;
  - `operations`: `review_r2_clock_integration_test.go` y `stagel1_license_gate_test.go` (`alwaysAvailable`);
  - `licenseseats`: `review_r2_seats_integration_test.go` (solo el `t.Skip`).
- `docs/API_V1.md` y `docs/licencias/DESBLOQUEO_SOPORTE.md`.

#### Pruebas (realmente ejecutadas)

| Prueba                                                                                                                                            | Resultado                                                                                                                                                                                                                                                                                                                          |
| ------------------------------------------------------------------------------------------------------------------------------------------------- | ---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `go test -race ./internal/licensing` (fuera del arnés, durante el trabajo)                                                                        | Aprobada                                                                                                                                                                                                                                                                                                                           |
| `ISO_TAG=FR iso.sh … go-int ./internal/licensing ./internal/database`, y otra vez con `ISO_RUN='TestDatabaseTrustedTimeCopy\|TestStageL1Upgrade'` | Aprobadas                                                                                                                                                                                                                                                                                                                          |
| `… go-int ./internal/administration` (incluye `TestStageL1LicenseGateWithoutSession` con R2-6)                                                    | Aprobada (280 s)                                                                                                                                                                                                                                                                                                                   |
| `… go-int ./internal/operations`, corrida completa                                                                                                | Falló solo `TestStageL1LicenseGate`. Su ayuda `alwaysAvailable` exigía `support-request` 200 también en la carpeta copiada, lo que R2-5 cambia; las otras cuatro subpruebas fallaron en cascada. Se ajustó la ayuda (`af3f86be`). Todo lo demás del paquete pasó, también `TestReviewR2DefectClockLoweringIsNotAtomicWithItsAudit` |
| `… go-int ./internal/operations` con `ISO_RUN='TestStageL1LicenseGate$'`, después del ajuste                                                      | Aprobada                                                                                                                                                                                                                                                                                                                           |
| `… go-int ./internal/operations` con `ISO_RUN='TestReviewR2\|TestStageL1'`                                                                        | Aprobadas la del revisor y la de la puerta. `TestStageL1LicenseSeats` dio una vez 503 en la entrada con PIN, con la carga de la máquina en 38 por otros agentes. Esa prueba no usa el gestor de licencias ni las rutas cambiadas                                                                                                   |
| `… go-int ./internal/operations` con `ISO_RUN='TestStageL1LicenseSeats'`, sola                                                                    | Aprobada (78 s)                                                                                                                                                                                                                                                                                                                    |
| `… go-int ./cmd/ad-el-pos-server`, con el candado `$SP/l1/candado-cmd` desde un script de bash que lo libera siempre                              | Aprobada (64 s); candado liberado                                                                                                                                                                                                                                                                                                  |
| `… go-int ./internal/licenseseats` con `ISO_RUN='TestReviewR2'`                                                                                   | La de R2-2 se omite (`t.Skip`)                                                                                                                                                                                                                                                                                                     |
| `go-unit` (vet normal e `integration`, `test -race` de todo, `GOOS=windows` build y vet)                                                          | Aprobado                                                                                                                                                                                                                                                                                                                           |
| `go test ./internal/apicontract ./internal/administration -run 'TestDocumented\|TestWaiter'`, después de cambiar `API_V1.md`                      | Aprobadas                                                                                                                                                                                                                                                                                                                          |
| `format`, `fmt-check` (tres veces) y `sync-check`                                                                                                 | Aprobados. MD5 de lo protegido iguales antes y después                                                                                                                                                                                                                                                                             |

Las pruebas del revisor pasan: R2-1 (las dos subpruebas, con el ajuste de arriba), R2-4 y R2-5. También
`…RejectsForgeries` y la del límite, ajustada. La de R2-2 queda omitida.

**No se corrió** la suite del escritorio ni la de Android: no cambió nada de ellos.

#### Lo no verificado

- **El escritorio (Fase E) con los códigos nuevos de estas rutas**: `409 LICENSE_LOCAL_ONLY` (antes 403) y
  `409 LICENSE_INSTALLATION_COPIED` en `support-request` y `support-unlock`.
  - En `desktop/src` de esta rama no aparecen ni los códigos ni las rutas de la hora: la pantalla de la hora es de la
    Fase E, que trabaja aparte.
  - Si esa pantalla decide por el estado 403, hay que ajustarla. Android no usa estas rutas.
- **El corte de luz entre la auditoría y la baja** (queda una auditoría sin baja): comprobado leyendo el código, sin
  prueba.
- **Windows**: las pruebas con permisos de carpeta son solo de Unix; el resto es igual en las dos plataformas.
- **Con el reloj y la base reales**: todo con relojes simulados y la base temporal 55499.

#### Para el coordinador

- **`LICENSE_IDENTITY_LOCAL_ONLY`** (Fase B, `license/identity/regenerate` desde la red) sigue siendo **403**. Tiene el
  mismo efecto que R2-6: queda como `access.denied`. No lo cambié porque no es de este encargo.
- **Las observaciones de R2 sin número** no se tocaron, entre ellas que la §6.3 también baja la hora cuando está solo
  unos segundos por delante.
- **Al unir con `etapa-l1`**:
  - en `review_r2_seats_integration_test.go`, quédate con la versión de `etapa-l1`;
  - la `0028` solo cambia en su encabezado y en la sección «Fase A (revisión R1)»; no toqué la «Fase D»;
  - `reports.go` lleva tres renglones nuevos en `completeDeferred`, después del caso de `*timeCheck`.

#### Commits (rama `etapa-l1-fr2`)

- `00eefa68` etapa L1 corrección R2: la prueba de R2-2 queda omitida en esta rama (la corrige la Fase D en licenseseats)
- `6ab6081b` etapa L1 corrección R2: segunda copia de la última hora confiable en la base (R2-3), semilla reintentada
  (R2-4), desbloqueo de soporte que se comprueba antes de aplicarse y solo en la instalación original (R2-1 y R2-5)
- `ef589715` etapa L1 corrección R2: la §6.3 y la §6.4 confirman su auditoría antes de bajar la hora y registran si no
  se aplicó (R2-1); LICENSE_LOCAL_ONLY es 409 y no queda como acceso denegado (R2-6)
- `e4463658` etapa L1 corrección R2: API_V1.md y DESBLOQUEO_SOPORTE.md con el 409 desde otra terminal, la carpeta
  copiada y la auditoría antes de bajar la hora
- `af3f86be` etapa L1 corrección R2: en la prueba de la puerta, la carpeta copiada ya no pide el código de desbloqueo de
  soporte (R2-5)

Antes, las pruebas del revisor: `f9a48a42`, `2bf0fe6c`, `3a81e8eb` y `88ae45b9`.

### Decisiones del coordinador sobre la revisión R2 y sus correcciones

Las correcciones se fusionaron en `f7fcbff`. En `review_r2_seats_integration_test.go` quedó la versión de `etapa-l1`
(la de R2-2 ajustada, sin `t.Skip`). R2-2 ya estaba en la Fase D (L1-D43).

- **L1-D42 · R2-1: la baja de la hora va con su auditoría.**
  - En la §6.3 y en la §6.4, primero se comprueba todo y se confirma la auditoría en su propia transacción. Después se
    bajan las dos copias.
  - Si la baja falla, se audita la misma acción con `success: false`, se responde 503, la hora no cambia y la
    respuesta de soporte no se gasta.
- **L1-D44 · R2-3: segunda copia de la última hora confiable en la base** (`app.license_trusted_time`, sección «Fase A
  (revisión R1)» de la `0028`).
  - Vale el máximo de las dos copias. La §6.3 y la §6.4 bajan las dos.
  - Sigue sin protección (§12):
    - quien escribe en la base y además edita `estado.json` baja la hora hasta el `issued_at` de la licencia;
    - una instantánea vieja de toda la computadora baja las dos copias.
- **L1-D45 · R2-4: la semilla se reintenta.** Mientras está pendiente, ni el reloj local ni las operaciones se vuelven
  la primera hora confiable.
- **L1-D46 · R2-5: una instalación copiada no hace desbloqueo de soporte.** Responde 409 `LICENSE_INSTALLATION_COPIED`
  y, sin huella, 503 `IDENTITY_UNAVAILABLE`.
- **L1-D54 · R2-6: las rutas de la hora desde otra terminal responden 409 `LICENSE_LOCAL_ONLY`** y ya no quedan como
  `access.denied`. El escritorio arma sus textos por el código, no por el estado HTTP, así que no cambia.
- **L1-D55 · Se aceptan las siete decisiones propuestas del agente**:
  - «no se aplicó» es la misma acción con `success: false`;
  - la §6.4 corre después de confirmar la petición;
  - la semilla se usa si falta cualquiera de las dos copias (amplía L1-D15);
  - con la semilla pendiente solo cuentan el `server_time` y el `issued_at`;
  - se guarda la hora en memoria antes de bajarla;
  - la copia de la base se escribe con un mutex propio;
  - los códigos de R2-5.
- **L1-D56 · `LICENSE_IDENTITY_LOCAL_ONLY` (Fase B) recibe el mismo trato que R2-6**: 409 y sin `access.denied`. Lo
  hace la Fase H.
- **L1-D57 · La §6.3 sigue como la escribe la especificación.** Con el reloj bien, «Comprobar la hora» reemplaza la
  hora confiable por el `server_time` aunque solo difieran unos segundos, y lo deja en la auditoría.
- **L1-D58 · Un salto al futuro también puede cambiar el estado de la licencia.** Con el bloqueo por la hora y la
  licencia en Tolerancia o Vencida, la pantalla de la fecha y hora dice que corregir la hora puede resolver también el
  aviso de la licencia. Lo hace la Fase H.

---

## Fase G2 · El cambio de plan de la Perpetua (24/11/2026)

El 24/11/2026 el propietario cambió en el panel la Perpetua del plan básico al **plan elemental**, con la Perpetua
liberada (cierre `deactivated`, rev. 2, de la Fase G). Esta fase comprobó que AD-EL POS recibe los datos nuevos y los
aplica.

- **Instancia P2**, nueva y con identidad propia: `$SP/l1/fase-g2/P2`, `ISO_TAG=LG4`, puertos 8225 (local) y 8497
  (red), `server_id` `2f2cacf1-…`. La levantó `$SP/l1/fase-g2/up-p2.sh`, que hace lo mismo que `up.sh` (en línea, con el
  reloj simulado vacío `reloj-LG4`) pero con la instancia dentro de `fase-g2`.
- **El binario** (0.15.0): `$SP/l1/fase-g2/bin/ad-el-pos-server`, compilado con `go build ./cmd/ad-el-pos-server` de una
  copia `git archive` de `etapa-l1` en `cdc2ce2` (`$SP/l1/fase-g2/src`), con `GOFLAGS=-mod=readonly`, `GOPROXY=off` y el
  `GOMODCACHE` del arnés.
- **Los ayudantes** son copias de `g.py`, `cupos.py`, `scan.py` y `auditoria.py` de la Fase G, en `$SP/l1/fase-g2/`, para
  que sus resultados queden ahí (`resultados.jsonl`, `resultados-cupos.jsonl` y `resultados-raw.jsonl`). `raw` se usó
  desde `$SP/l1/fase-g/raw/`. Los `.lic` están en `$SP/l1/fase-g2/lic/`.
- **Las llamadas a `licensing.sicrat.com`**, solo con la Perpetua: `activate`, `refresh` y `deactivate` desde P2, y una
  `raw consult` al final. Cada una con su desafío. El código se usó una sola vez, en `activate`.
- **Nunca se tocó** la instalación real, la hora de la Mac, la Mensual, la Prueba, las instancias conservadas de
  `~/.adel-pos-dev/instancias-l1/` ni los árboles `$SP/wt/*`. Ningún commit; `git status` del repositorio, limpio.

### Antes y después

| Dato                        | Fase G (P, 23/11)                     | Fase G2 (P2, 24/11)                           |
| --------------------------- | ------------------------------------- | --------------------------------------------- |
| Tipo                        | `perpetual`, `billing_period: null`   | `perpetual`, `billing_period: null`           |
| Plan                        | Plan básico (`basico`)                | **Plan elemental (`elemental`)**              |
| Cajas                       | 1                                     | **3**                                         |
| Comanderos                  | 0                                     | **3**                                         |
| Pantallas de cocina (KDS)   | 0                                     | **3**                                         |
| Revisión                    | 1 (activa) y 2 (cierre `deactivated`) | **3** (activa) y **4** (cierre `deactivated`) |
| `custom`                    | `false`                               | `false`                                       |
| `updates_until`             | 23/05/2027 21:28:31Z                  | 23/05/2027 21:28:31Z (**no cambió**)          |
| `expires_at` y `grace_days` | `null` y 0                            | `null` y 0                                    |
| `period_starts_at`          | 23/11/2026 21:28:31Z                  | 23/11/2026 21:28:31Z (no cambió)              |
| `features`, `kid`, entorno  | `{}`, `adelpos-test-202611-1`, `test` | `{}`, `adelpos-test-202611-1`, `test`         |

- Es la misma licencia (`license_id` `66de35bd-…`), con una activación nueva (`220e83b6-…`).
- **Cumple el contrato**:
  - §11.3, «Perpetua que sube»: el cambio aplica de inmediato y `updates_until` no cambia;
  - §5.2: el periodo no vuelve a empezar, porque no es la primera activación.
- El cambio en el panel no firmó una revisión propia mientras estaba liberada: la siguiente revisión, la 3, ya trae el
  plan nuevo.

### AD-EL POS aplica los datos nuevos

**Lo que ve el escritorio:**

- `license/status`: «Vigente» (`active`), perpetua, Plan elemental, `limits` 3/3/3, rev. 3 y `custom: false`. La
  pantalla Licencia saca de aquí el plan y los cupos.
- `devices/sessions-summary`: `seats` con `limit` 3 en los tres cupos y `limited: true`. La pantalla Licencia saca de
  aquí los que están en uso.
- `auth/me` y `restaurant/state`: `license` con `state: active`, `perpetual: true`, `expires_at: null` y su `clock` sin
  bloqueo. No traen el plan ni los cupos, porque ese resumen no los lleva: no es un defecto.

**Los cupos reales** (`cupos.py`, por la API v1, con terminales de escritorio enroladas y el usuario administrador):

| Cupo                    | Hasta el límite                            | El siguiente                                                  |
| ----------------------- | ------------------------------------------ | ------------------------------------------------------------- |
| Comanderos (3)          | **3 terminales capturan sin turno**        | La 4.ª: 409 `LICENSE_SEAT_UNAVAILABLE`, `order_takers` 3 de 3 |
| Pantallas de cocina (3) | 3 entran a Preparación (y salen)           | La 4.ª: 409, `kitchen_displays` 3 de 3                        |
| Cajas (3)               | 3 abren turno, cada una con su caja física | El 4.º turno: 409, `cash_registers` 3 de 3                    |

- La caja 1, con su turno, captura (200).
- Otra terminal de la caja física 1: 409 `cash_registers` 3 de 3 (L1-D3).
- `sessions-summary` siguió los cupos en cada paso: 3/3 comanderos, 3/3 cocina y después 3/3 cajas.
- **Capturar como comandero ya funciona**. Con el plan básico la primera terminal recibía 409 `order_takers` 0 de 0.
- Los mensajes salen de AD-EL POS, en español; por ejemplo, «No hay cupo de comandero libre (3 de 3 en uso). Pide que se
  cierre la sesión de otra terminal.».
- **Resultado: aprobado.** 0 fallas.

**«Buscar actualización»** (`license/refresh`, una vez): 200, la **misma rev. 3**, sin cambios. `vigente.lic` quedó
idéntico byte por byte (el mismo SHA-256).

### El estado final: liberada y sin transferencias

- **`deactivate`**: 200, rev. **4**, `revoked` / `deactivated`: «Revocada» con motivo `deactivated`. Es el cierre
  instalado en `identity/licencia/vigente.lic`, con la fila de `app.license_revisions` en la rev. 4.
- **`raw consult`** de la activación `220e83b6-…`, directo al servidor y sin instalar la respuesta: 200, la **misma
  rev. 4**, `revoked` / `deactivated`. El servidor no firmó otra.
- **Ninguna activación vigente**:
  - activar desde P2 funcionó a la primera, sin `ACTIVATION_LIMIT`, así que no había otra;
  - la de P2 quedó cerrada.
- **Ninguna transferencia pendiente**: la Perpetua nunca se transfirió, ni en la Fase G ni aquí.
- **La Perpetua queda libre** para activarse en la Mac real (Fase H), ya con el plan elemental.
- **P2 está apagada.**
  - Su base `adel_pos_test_lg4_6fb05bfe26` se borró.
  - Su `identity/` queda en `$SP/l1/fase-g2/P2` y no se conservó en `~/.adel-pos-dev/`.
  - El reloj `reloj-LG4` siguió vacío.
- La consulta en segundo plano no llegó a correr: P2 estuvo encendida unos 2 minutos.

### Las búsquedas de códigos

- **`scan.py`**: revisó 2006 archivos y ninguno contiene un código. Revisó `$SP/l1/fase-g2/` completo (la copia `src/`, `P2/` con su
  `server.log`, la identidad y la configuración, los `.lic` y los resultados), `doc-G2.md` y `estado-G2.md`.
- **`auditoria.py`** de la base de P2, antes de borrarla: ningún evento de `app.audit_events` contiene un código.
  - Los eventos: `license.activated` (rev. 3), `license.state_changed`, `license.refreshed` (rev. 3),
    `license.deactivated` (rev. 4) y `license.state_changed`.
- **`server.log`**: solo `license_accepted` y `license_server_ok`, con la acción, el estado y la revisión.

### Lo no verificado

- **La prueba 7 de la guía**, «Buscar actualización» con un cambio de plan **mientras la licencia está activa**: no se
  puede probar así, porque la Perpetua estaba liberada cuando el propietario la cambió. Aquí el plan nuevo llegó con
  una activación nueva, no con una revisión mayor de una activación vigente.
- **Cómo puede hacerla el propietario**, con la instancia `en-linea` conservada y la Prueba (pasos 1 y 2 de las pruebas
  7 y 8 de la «Lista para el propietario» de la Fase G):
  1. `levantar.sh up $H/en-linea 8211 8481 --en-linea`, después `g.py … setup` y `g.py … activate prueba`. Se espera
     la Prueba «Vigente», con el Plan elemental 3/3/3 y una revisión mayor que 6. Con
     `--bin $SP/l1/fase-g2/bin/ad-el-pos-server` usa el binario de `cdc2ce2` en vez del de `f32d740`.
  2. `g.py … status` y anotar la revisión.
  3. En el panel, cambiar el plan de la Prueba, por ejemplo a premium. En una prueba es libre y no cambia las fechas
     (§11.3).
  4. Sin reiniciar nada, `g.py … refresh`. Se espera 200, una **revisión mayor**, Plan premium 5/10/5 y las mismas
     fechas.
  5. `cupos.py $H/en-linea`: 5 cajas, 10 comanderos y 5 pantallas de cocina, y el siguiente de cada uno con 409.
  6. Después, la prueba 8 (revocar) o liberar con `g.py … deactivate`, y apagar.
  - Si no se pulsa «Buscar actualización», la consulta en segundo plano también debería traer la revisión nueva por
    su cuenta.
- **Con la Perpetua en la Mac real**, un cambio a un plan mayor también llegaría así, pero en una perpetua cobra la
  diferencia (§11.3). Lo decide el propietario.
- **Lo del panel**: que P2 (`2f2cacf1-…`) aparezca como liberada en «Computadoras y transferencias» de la Perpetua.
- **Android y el escritorio**: no se usaron. Los cupos se probaron por la API, con terminales de escritorio.

### Decisión del coordinador sobre la Fase G2

- **L1-D59 · La Perpetua ahora es del plan elemental (3 / 3 / 3). Reemplaza la segunda parte de L1-D41.**
  - El propietario la cambió en el panel el 24/11/2026.
  - AD-EL POS 0.15.0 la recibe al activarla y aplica sus cupos.
  - Cuando el propietario active la Mac real con ella (§10 de la especificación), la Mac funciona como caja. Además, hasta
    3 tabletas capturan como comanderos y Preparación se abre en hasta 3 pantallas.
- **La prueba 7 de la guía sigue en la lista del propietario**: «Buscar actualización» con un cambio de plan mientras la
  licencia está activa. Los pasos, con la instancia `en-linea` conservada y la Prueba, están arriba, en «Lo no
  verificado».

---

## Fase H · Servidor, escritorio, documentos y `.pkg` 0.15.0

Árbol `$SP/wt/l1-h`, rama `etapa-l1-h`, desde `cdc2ce2` (la cabeza de `etapa-l1` con todas las fases unidas).
`ISO_TAG=HS`. Sin emuladores. Ninguna llamada a `licensing.sicrat.com`; la hora de la Mac y la instalación real, sin
tocar; el `.pkg` no se instaló. La rama no trae `49e84aa` (Fase G2, L1-D59): la unión la hace el coordinador. Los
documentos ya escriben la Perpetua con el plan elemental (3/3/3).

### Commits

Rama `etapa-l1-h`, desde `cdc2ce2` (8 commits, sin líneas de atribución):

- `9384e63` LICENSE_IDENTITY_LOCAL_ONLY responde 409 y ya no queda como acceso denegado (L1-D56)
- `623d089` la pantalla de la fecha y hora dice que corregirla puede resolver también el aviso de la licencia en
  Tolerancia o Vencida (L1-D58)
- `129ab49` docs/licencias con INTEGRACION.md y PASO_A_PRODUCCION.md, y ROTAR_LLAVES.md y DESBLOQUEO_SOPORTE.md completos
- `0ffa6b8` API_V1.md con todos los campos de LicenseStatus, LicenseSummary y LicenseClock
- `8493414` CLAUDE.md con la etapa L1
- `16090b8` INTEGRACION.md: la sección de las herramientas y la semilla de la hora confiable (L1-D55)
- `cab5daa` docs/VERIFICATION_STAGEL1.md
- `7cd38b3` CLAUDE.md con el resultado de la regresión final y el `.pkg` 0.15.0

Desde `623d089` (la copia de la regresión) solo cambió Markdown.

### 0. Los dos cambios pequeños

- **L1-D56** (`9384e63`): `regenerateIdentity` (`backend/internal/administration/license.go:430`) responde **409**
  `LICENSE_IDENTITY_LOCAL_ONLY` desde otra terminal, como `LICENSE_LOCAL_ONLY` en R2-6. Así `runProtected` ya no lo
  audita como `access.denied`.
  - Prueba: `TestLicenseRoutes` (`license_acceptance_test.go:330`) exige el 409, el texto «computadora principal» y que
    no aumente `access.denied`. Aprobada; **con el 403 anterior falla** («esperado 409, recibido 403»).
  - `openapi.json`, `docs/API_V1.md` y `TestContractV1` no nombran esta ruta (es del escritorio). La nombraba
    `API_CONTRACTS.md:4238` con 403: corregido. El escritorio arma su texto por el código (`license.ts:597`): sin cambio.
- **L1-D58** (`623d089`): en la pantalla de la fecha y hora (`ClockBlockPage.tsx`), con la licencia:
  - **Tolerancia** («Tu licencia está bien…» sigue, porque `license_ok` es verdadero): se agrega «La licencia aparece en
    tolerancia. Si la computadora tuvo la fecha adelantada, corregir la fecha y hora también puede quitar ese aviso.»;
  - **Vencida**: «Puede ser por la misma fecha equivocada: si la computadora tuvo la fecha adelantada, corregir la fecha
    y hora con los pasos de arriba también puede resolver el aviso de la licencia.», en lugar de «corregir el reloj no
    lo resuelve»;
  - **Revocada y Sin activar** (no dependen de la fecha): siguen con «Es otro problema, aparte de la fecha y hora:
    corregir el reloj no lo resuelve.».
  - Los textos y la regla (`clockLicenseDependsOnDate`) están en `desktop/src/services/license.ts`. «Con los pasos de
    arriba» porque, tras un salto al futuro, corregir el reloj no basta: la hora confiable baja con «Comprobar la hora
    con internet» o el desbloqueo de soporte, que son los pasos 2 y 3 de la pantalla.
  - Pruebas: cuatro nuevas en `stageL1License.test.tsx` (Vencida, Revocada, Tolerancia y Vigente); Vitest 105 archivos,
    1 002 pruebas. `workflow-stagel1-clock` (paso 6) ajustada al texto nuevo y aprobada en la regresión.

### 1. Regresión completa (una sola vez)

Sobre una **copia exacta de `623d089`** (`git archive`, 1 976 archivos, en `$SP/l1/hs/copia`, con `.tools`, `.cache` y
`node_modules` enlazados). La copia quedó **idéntica por md5** al terminar (manifiesto `$SP/l1/hs/copia-md5-antes.txt`
y `-despues.txt`), y `md5 -q docs/etapas/*.md | md5` = `f0a12ff716ded1e1c7a80e72cc52e761` antes y después (también en
el árbol, antes y después de cada `format`; el contrato, la guía y `contracts/` tampoco cambiaron). Guiones:
`$SP/l1/hs-scripts/regresion-servidor.sh` y `regresion-playwright.sh`; registros en `$SP/l1/hs/reg/` y `$SP/l1/hs/pw/`.
El servidor y Playwright corrieron en paralelo (24/11, de 09:50 a 10:48).

| Prueba                                                            | Resultado                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                    |
| ----------------------------------------------------------------- | -------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `fmt-check`                                                       | Aprobado (en la copia, «sin .git»; en el árbol, «el índice de git no tiene respaldos ni paquetes de migración»)                                                                                                                                                                                                                                                                                                                                                                                                                              |
| `sync-check`                                                      | «Metadatos coherentes.»; `config/product.json` 0.15.0 y esquema 28                                                                                                                                                                                                                                                                                                                                                                                                                                                                           |
| `go-unit`                                                         | Aprobado: 39 paquetes `ok` y 7 sin pruebas de unidad; `GOOS=windows` build y vet                                                                                                                                                                                                                                                                                                                                                                                                                                                             |
| `go vet -tags 'integration legacydocker'`, macOS y `GOOS=windows` | Aprobado                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                     |
| Integración, los **16 paquetes** de `database.py`, con `-v`       | **16 de 16**, sin fallas: `database` 19 s, `administration` 313 s, `catalog` 26 s, `operations` 813 s, `payments` 3 s, `reports` 163 s, `reports/operational` 49 s, `designer` 28 s, `identity` 23 s, `apicontract` 53 s, `backupschedule` 10 s, `discovery` 5 s, `legacyimport` 15 s, `licensing` 13 s, `licenseseats` 5 s y `cmd/ad-el-pos-server` 72 s (candado tomado y liberado; `TestServiceHostOnMacOS` 64 s). Omitidas solo las opcionales (mediciones, impresora física, mDNS en vivo); `TestOperationalReports` corrió a las 10:16 |
| 503 por carga (aviso del coordinador)                             | **No apareció**: `operations` completo aprobó a la primera (carga 9 al empezar)                                                                                                                                                                                                                                                                                                                                                                                                                                                              |
| `vitest`                                                          | 105 archivos, **1 002 pruebas**                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                              |
| `build`                                                           | Aprobado                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                     |
| `cargo test --locked --offline`                                   | 115 aprobadas, 3 ignoradas                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                   |
| `scripts/macos-pkg/test-scripts.sh` y `bash -n`                   | «todo correcto» (31 comprobaciones); `bash -n` de 9 guiones                                                                                                                                                                                                                                                                                                                                                                                                                                                                                  |
| **Playwright Chromium** completo                                  | **50 de 50 a la primera** (20,6 min), con las 4 de instalación limpia de la 9H y las 5 de la etapa                                                                                                                                                                                                                                                                                                                                                                                                                                           |
| **Playwright WebKit** completo                                    | **50 de 50 a la primera** (20,3 min), igual                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                  |
| `workflow-stagel1-clock` sola en WebKit, con `ADEL_SHOTS_DIR`     | 1 de 1; capturas en `$SP/l1/hs/capturas/` (revisé `webkit-hora-y-licencia.png`: la frase de L1-D58, tema claro)                                                                                                                                                                                                                                                                                                                                                                                                                              |
| Ninguna llamada a `licensing.sicrat.com`                          | El Go compartido y el sin activar, con `license_server_url` = `https://127.0.0.1:8491` (el simulador); el Go limpio, con `license_server_disabled` (`remote_not_allowed`) y activado por `license/import`; ninguna bitácora nombra el servidor real                                                                                                                                                                                                                                                                                          |
| Pantalla                                                          | Desbloqueada antes y después de cada corrida completa                                                                                                                                                                                                                                                                                                                                                                                                                                                                                        |

**Las cuatro de «instalación limpia» de la 9H**: con un Go limpio nuevo por corrida (`iso.sh server-up`, `ISO_TAG=HSC`,
8229 y 8501, con `ISO_LICENSE_TRUST` = `vectors.json`) y `ADEL_BROWSER_CLEAN_SERVER_URL`/`_PROVISIONING`. Ninguna
omitida. (8221 y 8495 estaban ocupados por otro agente: no los usé.)

**Ninguna falla**, así que no hubo correcciones ni repeticiones.

### 2. Documentos (§10)

- **`docs/licencias/INTEGRACION.md`** (nuevo): cómo está hecho (piezas, dónde se guarda cada cosa, el entorno al
  compilar, en línea y sin internet, los trabajadores), lo que decide AD-EL POS (los 15 minutos, los 7 días, la hora, la
  huella y el desbloqueo), los estados y la tabla de la §6, los cupos, los códigos de error (los 5 de la puerta y los 21
  de la gestión y la hora), las herramientas de prueba y las propuestas para la V1.1.
- **`docs/licencias/PASO_A_PRODUCCION.md`** (nuevo): dónde estamos, el servidor de licencias de producción, sus llaves,
  la llave de soporte, la compilación de producción (los instaladores no pasan `-tags adelpos_production` y
  `standalone/bootstrap.go` escribe `development`), qué pasa con una instalación de desarrollo y lo que no cambia.
- **`ROTAR_LLAVES.md` y `DESBLOQUEO_SOPORTE.md`**: ya existían completos; agregué los enlaces y, en el desbloqueo, L1-D53
  y L1-D58.
- **`CLAUDE.md`**: la línea de «Última revisión» (0.15.0, esquema 28, la `0028`, Android 0.4.0, la Mac «Sin activar con
  datos» hasta activarla con la Perpetua 3/3/3); §3 (la licencia y la composición del backend); §4 (paquetes, `cmd`,
  `config/licencias`, `contracts/licencias`, `tools/soporte-desbloqueo`, la `0028` y `docs/licencias`); §6 (pruebas de
  la licencia, `ISO_LICENSE_SIM`, 16 paquetes); §7 (secretos y llaves); §11–12 («Licencia en la interfaz» y «Reglas
  permanentes desde la etapa L1»); §13–14 (filas de la A3, que faltaba, y de la L1); §15 (las decisiones L1-D1 a
  L1-D59); §16, §17 (producción), §18, §19 («Etapa L1 sin verificar») y §20 (la siguiente migración es la `0029`). Los
  locks (§3) y los puertos (§6) ya estaban al día desde las fases D y E.
- **`docs/API_V1.md`**: comprobado contra `openapi.json` con un guion: las 42 operaciones están en la tabla con su
  método y ruta, y los 5 códigos de licencia del contrato están documentados. Agregué los campos que faltaban de
  `LicenseStatus` (`server_id`, `effective_now`, `can_manage`, `identity_regeneration_available` y los de `license`),
  `LicenseSummary` (`license_type`…) y `LicenseClock`. `TestContractV1` (en `apicontract`, aprobado) recorre todas.
- **`docs/VERIFICATION_STAGEL1.md`** (nuevo), con la forma de la A3: entorno aislado, Fase 0, resultados por fase, las
  tres revisiones, las Fases G y G2, lo no verificado, la regresión final, «Android» («la completa el coordinador»), los
  entregables y la instalación real con los pasos del propietario (Perpetua 3/3/3, L1-D59).
- `android/CLAUDE.md` y `docs/ANDROID_INSTALAR.md` ya tenían lo de la L1 (Fase F, «Activa la computadora principal antes
  de emparejar» y la sección 11): no los toqué.

### 3. El `.pkg` 0.15.0

- Construido con `./scripts/dev.sh build-macos-pkg` sobre una copia exacta de `16090b8` (1 978 archivos), con
  `CARGO_NET_OFFLINE=true`, `GOFLAGS=-mod=readonly` y `GOPROXY=off`, en 123 s. **No se instaló.**
- **`AD-EL POS_0.15.0.pkg`: 23 850 140 bytes, SHA-256 `fe60d29211fcef9f727d9e8d2bcbb45e6aa62747aa65ef3b58a92048a6369443`.**
- Comprobaciones (`$SP/l1/hs-scripts/pkg-revisar.sh`, registro `$SP/l1/hs/pkg/revision.log`): los tres paquetes 0.15.0,
  «Servidor del restaurante» marcada, `arm64`; la aplicación 0.15.0; el servidor de dentro `0.15.0`, `"schema_version":
28` y la `0028_licencias.sql`; `codesign --verify --deep --strict`: «valid on disk», «satisfies its Designated
  Requirement», `adhoc`; **confía en `test`** (`go version -m` sin `-tags`; lleva `adelpos-test-202611-1` con la pública
  de §1.2, el `kid` de soporte de desarrollo y la dirección de pruebas); **no lleva** `kid` ni semillas de
  `vectors.json`, `vectors.json`, `.lic`, `.licreq`, `.pem`, `.key`, `PRIVATE KEY`, `license-test` ni
  `soporte-desbloqueo`; scripts idénticos a los de la copia.
- Copiado a `desktop/src-tauri/target/release/bundle/macos-pkg/` del repositorio, junto a los de 0.10.0 a 0.14.0 (los
  cinco con el mismo SHA-256 que antes), y a `$SP/AD-EL POS_0.15.0.pkg`.

### Hallazgos y correcciones

- **De la regresión**: ninguno. Todo aprobó a la primera.
- **De los documentos** (corregidos):
  - `API_CONTRACTS.md` decía 403 para `LICENSE_IDENTITY_LOCAL_ONLY` (corregido con L1-D56).
  - `CLAUDE.md` §16 y §17 decían que aquí solo existía el límite `internal/licensing` y que producción estaba bloqueada
    solo por `CheckStartup`; §13–14 no tenía la fila de la A3; §20 decía que la siguiente migración era la `0028`.
- **Sin cambiar (tocan `backend/` y obligarían a repetir la regresión completa; los decide el coordinador)**:
  - el comentario de la sección «Fase D» de `backend/migrations/0028_licencias.sql` (líneas 74-77) todavía dice que el
    comandero cuenta «mientras … la terminal tenga alguna sesión vigente», que L1-D36 cambió (cuenta por actividad). Es
    solo un comentario; cambiarlo cambia el archivo de la migración antes de instalarla;
  - el comentario de `serverConfig` en `backend/internal/standalone/bootstrap.go:44` («hasta la etapa 10 no hay
    licencias…») ya no es cierto. Queda anotado en `PASO_A_PRODUCCION.md`.

### Decisiones propuestas

1. **L1-D58 también en Tolerancia.** Con la licencia en Tolerancia, `license_ok` es verdadero y la pantalla sigue
   diciendo «Tu licencia está bien…» (§6.2 de la especificación); debajo, en letra menor, agrego la aclaración. Por qué:
   L1-D58 nombra Tolerancia y Vencida, y la §6.2 pide la frase «Tu licencia está bien» en Tolerancia; así se cumplen las
   dos. En Revocada y Sin activar queda «es otro problema».
2. **«Con los pasos de arriba»** en el texto de Vencida: después de un salto al futuro, corregir el reloj no basta (la
   hora confiable solo baja con la §6.3 o la §6.4), así que la frase remite a los pasos de la pantalla.

### Lo no verificado

- La variante de Tolerancia de L1-D58 solo con Vitest (Playwright cubre Vencida).
- El desbloqueo de soporte aceptado en la interfaz, `browser-tests.py` tal cual (`.local`), Docker real del lector
  (apagado), Windows, la aplicación real de Tauri y dispositivos físicos: como en las fases.
- Android: lo hace el otro agente.
- La instalación del `.pkg`, la activación con la Perpetua y las pruebas 7, 8 y 10 de la guía: del propietario.

### Encendido

Nada: Go temporales (HS, HSC, el simulador y el sin activar) apagados, sin bases `adel_pos_test_hs*` en 55499, puertos
8217, 8219, 8229, 8491, 8501 y 1441 libres, candado `candado-cmd` liberado. El árbol `$SP/wt/l1-h` está limpio. El
clúster 55499 es compartido y sigue encendido.

### Decisiones del coordinador sobre la Fase H (servidor y escritorio)

Fusionada en `02f4642`.

- **L1-D60 · Se aceptan las dos decisiones propuestas del agente sobre el texto de L1-D58.**
  - En Tolerancia, la aclaración va como nota debajo de «Tu licencia está bien…»: así se cumplen a la vez L1-D58 y la
    §6.2.
  - En Vencida dice «con los pasos de arriba», porque después de un salto al futuro no basta corregir el reloj.
- **L1-D61 · El comentario de la `0028` se corrigió antes de instalarla** (`842e088`). Decía que el comandero cuenta con
  una sesión vigente; desde L1-D36 cuenta por actividad.
  - Las migraciones llevan su SHA-256 en `schema_migrations`: después de instalar la `0028`, ya no se podría cambiar.
  - Se repitieron `go-unit` y la integración de `database` y de `cmd/ad-el-pos-server`. Aprobaron.
  - Se rehízo el `.pkg` 0.15.0 sobre `842e088`, con las mismas comprobaciones: 23 850 119 bytes, SHA-256
    `2ef89832bb703f5bc69ccf01b621c5b28eed4f6b1537b171e2fd01ea3f6b669b`. Lleva la `0028` con el comentario nuevo.
  - Reemplazó al primero (`fe60d292…9443`) en el repositorio y en `$SP`. Los de 0.10.0 a 0.14.0 siguen intactos.
- **El comentario de `standalone/bootstrap.go:44`** («hasta la etapa 10 no hay licencias») se queda para después: un
  comentario de Go se puede corregir en cualquier versión, y ya está anotado en `PASO_A_PRODUCCION.md`.
