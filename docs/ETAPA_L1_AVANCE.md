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
