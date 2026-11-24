# Licencias en AD-EL POS · cómo está hecha la integración

Qué hace AD-EL POS 0.15.0 con la licencia: cómo está construido, qué decide por su cuenta, los estados y los cupos, los
códigos de error y las herramientas de prueba. Es la integración de la etapa L1.

**Manda el contrato.** Las reglas están en el [contrato de licencias V1.0](CONTRATO_LICENCIAS_ADEL_POS.md), copiado sin
cambios de la etiqueta `v1.0.0` del servidor de licencias, junto con su [guía](INTEGRACION_ADEL_POS.md) y
`contracts/licencias/v1/` (origen y SHA-256 en `contracts/licencias/ORIGEN.md`). Este documento solo explica cómo lo
aplica AD-EL POS y lo que el contrato le deja decidir. Las decisiones `L1-Dn` están en el
[avance de la etapa](../ETAPA_L1_AVANCE.md); la especificación, en
[etapas/ETAPA_L1_LICENCIAS_v2.md](../etapas/ETAPA_L1_LICENCIAS_v2.md).

Otros documentos de la carpeta:

- [ROTAR_LLAVES.md](ROTAR_LLAVES.md): cambiar las llaves con que el servidor de licencias firma.
- [DESBLOQUEO_SOPORTE.md](DESBLOQUEO_SOPORTE.md): quitar el bloqueo por la hora sin internet.
- [PASO_A_PRODUCCION.md](PASO_A_PRODUCCION.md): qué falta para la versión del restaurante.

---

## 1. Cómo está hecho

### Quién hace qué

- **Solo el servidor Go de la computadora principal** habla con el servidor de licencias (contrato §1). Las terminales
  de escritorio y Android **solo muestran** el estado y los rechazos que Go les da, por su código de error.
- **Go es la única autoridad** de la licencia, los cupos, la hora y lo que se permite en cada estado. La regla se aplica
  en el backend, en cada operación, para el escritorio, la API v1 y Android.
- Los textos que ve el restaurante salen de los códigos de error de AD-EL POS, **nunca** del `message` del servidor de
  licencias.

### Las piezas

| Dónde                                                                                   | Qué hace                                                                                                                                                                                                                                                                                                                                                                                                                                                                         |
| --------------------------------------------------------------------------------------- | -------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `backend/internal/licensing/`                                                           | El núcleo. `verify.go` y `protocol.go` (la §10 del contrato, adaptada del verificador de referencia), `state.go` y `status.go` (los seis estados y la §5.6), `store.go` (los archivos de `identity/licencia/`), `machine*.go` (la huella del equipo), `manager.go` (el gestor), `client.go` y `worker.go` (en línea y la consulta diaria), `offline.go` (`.licreq` y `.lic`), `operations.go` (la tabla de la §6), `clock.go` (la hora) y `unlock.go` (el desbloqueo de soporte) |
| `backend/internal/licenseseats/`                                                        | Los cupos: qué ocupa cada operación (`operations.go`), qué cuenta como «en uso» y la asignación atómica (`store.go`), con el lock 7820148311                                                                                                                                                                                                                                                                                                                                     |
| `backend/internal/administration/license*.go`                                           | Las rutas `license/*` (`license.go`), la puerta de cada operación (`license_gate.go`), las rutas de la hora (`license_clock.go`), el vigilante (`license_monitor.go`) y los cupos en el despacho (`license_seats.go`)                                                                                                                                                                                                                                                            |
| `backend/cmd/ad-el-pos-server/license.go`                                               | El arranque: carga el gestor después de la identidad, conecta los cupos y el cliente y arranca los trabajadores, en `serve` y en el `host` de macOS y Windows                                                                                                                                                                                                                                                                                                                    |
| `backend/cmd/license-test/`                                                             | Herramientas de prueba (§7 de este documento). Nunca llegan a un instalador                                                                                                                                                                                                                                                                                                                                                                                                      |
| `backend/cmd/soporte-desbloqueo/` y `tools/soporte-desbloqueo/`                         | La herramienta del propietario para el desbloqueo de soporte                                                                                                                                                                                                                                                                                                                                                                                                                     |
| `config/licencias/`                                                                     | Las llaves públicas por entorno: `llaves-test.json` (firma de licencias) y `soporte-development.pub.json` (desbloqueo). `scripts/sync-metadata.mjs` las copia a `backend/internal/licensing/` y `sync-check` lo vigila                                                                                                                                                                                                                                                           |
| `backend/migrations/0028_licencias.sql`                                                 | La única migración de la etapa (esquema 28): segundas copias de las revisiones y de la hora confiable, el permiso `license.manage` y `app.license_seats`                                                                                                                                                                                                                                                                                                                         |
| `desktop/src/pages/license/` y `desktop/src/services/license*.ts`                       | Primer arranque, Configuración › Licencia, avisos, la pantalla de la fecha y hora y los textos de cada código. Se esconde por la capacidad `license-v1`                                                                                                                                                                                                                                                                                                                          |
| `desktop/src-tauri/src/license_files.rs`                                                | Guardar el `.licreq` y abrir el `.lic` o la respuesta de soporte con los diálogos nativos                                                                                                                                                                                                                                                                                                                                                                                        |
| Android 0.4.0 (`core:domain/error/License*.kt`, `core:ui/components/LicenseNotices.kt`) | Los mensajes de los rechazos, el aviso fijo de la hora y el aviso de tolerancia. Ver [ANDROID_INSTALAR.md](../ANDROID_INSTALAR.md), sección 11                                                                                                                                                                                                                                                                                                                                   |

### Dónde se guarda

En `identity/`, junto a `server.json`, que **nunca viaja en un respaldo** (L1-D9):

- `license-key.pem`: la llave Ed25519 de licencias de la instalación (contrato §2), PKCS#8, 0600. Se crea una sola vez,
  aparte del certificado ECDSA de la red local. Sin ella no se regenera sola.
- `license-machine.json`: la huella del equipo, firmada con esa llave y atada al `server_id` (L1-D29).
- `licencia/estado.json`: la licencia vigente (su JWS), la revisión más alta de cada licencia, la última hora confiable
  con su fuente y la solicitud de desbloqueo pendiente. Es el punto de confirmación: archivo temporal, `fsync` y
  renombrado.
- `licencia/vigente.lic`: la copia de la licencia tal como viaja.

En la base, solo segundas copias que protegen contra borrar o editar `estado.json` (L1-D31 y L1-D44): la revisión más
alta (`app.license_revisions`) y la última hora confiable (`app.license_trusted_time`). Vale el máximo de las dos copias,
así que restaurar un respaldo viejo de la base no devuelve una revisión superada ni baja la hora. También están el
permiso «Administrar la licencia» y los cupos ocupados (`app.license_seats`).

Al arrancar, la licencia guardada vuelve a pasar la §10 completa (L1-D20). Un archivo alterado, firmado con una llave no
confiable o de otra instalación se ignora y queda la última licencia válida.

### El entorno, fijado al compilar (L1-D18)

- **Desarrollo** (sin etiqueta): confía en las llaves del entorno `test` (`config/licencias/llaves-test.json`) y apunta a
  `https://licensing.sicrat.com`. Es la versión de esta etapa y del `.pkg` 0.15.0.
- **Producción** (`-tags adelpos_production`): confía en `llaves-production.json` y en `soporte-production.pub.json`,
  que todavía no existen, así que **no compila** (`TestProductionBuildFailsWithoutProductionKeys`). Ver
  [PASO_A_PRODUCCION.md](PASO_A_PRODUCCION.md).
- `CheckStartup` exige que `server.json` diga el mismo entorno: una versión de desarrollo solo arranca con `environment`
  y `license_mode` `development`.
- `license_server_url` en `server.json` cambia la dirección (https). En `test_mode` el cliente solo habla con loopback,
  salvo `ADEL_LICENSE_ALLOW_REMOTE=1`, que solo usó la Fase G.

### En línea y sin internet

- **En línea** (contrato §8): activar, transferir, buscar actualización y liberar, cada una con su desafío y su prueba de
  posesión. Hasta tres intentos con el mismo cuerpo y el mismo `request_id`; se decide por `error.code`, nunca por el
  HTTP. Un `Retry-After` de más de 5 s se devuelve como `LICENSE_SERVER_BUSY`. Una falla de red, de TLS o del servidor
  nunca toca la licencia instalada.
- **Transferir** solo después de `ACTIVATION_LIMIT` con `transfer_available: true` y la confirmación de la persona.
- **La consulta en segundo plano** (`worker.go`): la primera entre 1 y 5 minutos después de arrancar y después cada
  24 h ± 1 h; si falla, en 1 hora o lo que pida `Retry-After`. Solo con una activación vigente y nunca en una instalación
  copiada. No bloquea nada.
- **Sin internet** (contrato §9): el `.licreq` de `activate` (código opcional), `renew` y `deactivate`, idéntico byte por
  byte a los de `vectors.json`, y el `.lic` que se verifica igual que una respuesta en línea.
- Las operaciones en línea corren **después** de confirmar la autenticación (`completeDeferred`): la red nunca retiene
  una conexión de la base ni un lock. El plazo de las rutas `license/*` es de 75 s.

### Los trabajadores

- **El gestor** (`Manager.Run`): cada minuto sube la última hora confiable con el reloj local y la guarda en las dos
  copias; también al apagar.
- **El vigilante** (`license_monitor.go`): cada 15 s revisa la hora y el estado. Audita cada bloqueo y desbloqueo por la
  hora (`license.clock_blocked` y `license.clock_unblocked`, con las dos fechas) y cada cambio de estado
  (`license.state_changed`), y publica `license.changed` para todas las terminales. Escribe en su propia transacción
  corta.
- **La consulta diaria** del cliente (arriba).

---

## 2. Lo que decide AD-EL POS

El contrato deja a AD-EL POS estas reglas. Son las de la especificación (§5 y §6), con las decisiones del avance.

### Los 15 minutos del comandero

- Una terminal que captura sin turno de caja abierto ocupa un **cupo de comandero** (L1-D3 y L1-D13). Es **por
  terminal**: varios meseros con su PIN en una tableta ocupan uno (L1-D2).
- **Cuenta por actividad** (L1-D36): ocupa el cupo durante **15 minutos desde su última captura**, tenga o no una sesión
  vigente. Lo renuevan abrir una cuenta, el borrador, confirmar, modificar lo enviado y el pedido sin conexión; las
  previas solo renuevan si la terminal ya tiene el cupo (L1-D39).
- **Se libera antes**: al cerrarse la última sesión de esa terminal, al abrir turno en ella, con «Liberar cupo» en
  Terminales o al revocar la terminal.
- El valor es una constante del backend (`licenseseats.OrderTakerIdle`), no un ajuste de la interfaz.
- **Una actividad con fecha futura** (la hora saltó adelante y se capturó) cuenta como de ahora: se baja a la hora
  actual y vence 15 minutos reales después (L1-D43), **solo con la hora confiable** (L1-D49). Con la hora en «Hora del
  equipo incorrecta», el «ahora» de Go es el equivocado.

### Los 7 días del aviso

- **7 días antes del vencimiento** de una suscripción o una prueba, el escritorio muestra un aviso al entrar, una vez al
  día, a quien tiene «Administrar la licencia»; los demás ven un indicador discreto. Se cierra con «Entendido» y no vuelve
  ese día, por usuario y en esa computadora (L1-D51).
- **En Tolerancia**, un aviso diario para todos con los días que quedan antes de que se bloquee la captura. En Android,
  un aviso discreto en la barra.
- **Vencida o Revocada**: un aviso fijo con lo que sí y lo que no se puede hacer.
- Go da `days_until_expiry` y `grace_days_remaining` ya calculados con la hora confiable; el umbral de 7 días está en el
  escritorio.

### La hora

- **La regla** (§6.1): la hora de la computadora principal no puede estar más de **10 minutos** (`licensing.ClockMargin`)
  antes de la **última hora confiable**. Se revisa al arrancar, cada 15 s y en cada operación protegida, solo en el
  servidor principal; las terminales usan la hora de Go.
- **La última hora confiable** (L1-D15, L1-D44 y L1-D45) solo sube: el `server_time` de una respuesta correcta del
  servidor de licencias, el `issued_at` de una licencia aceptada, el reloj local cada minuto y cada operación con fecha
  aceptada. Si falta en las dos copias, la inicia la última actividad de la auditoría.
- **Solo dos cosas la bajan**: «Comprobar la hora con internet» (§6.3) y el desbloqueo de soporte (§6.4). Las dos se
  auditan **antes** de aplicarse y bajan las dos copias (L1-D42); si no se puede guardar, no cambia nada.
- **El bloqueo es una protección local, no un estado de la licencia** (L1-D1). Con «Hora del equipo incorrecta» se
  rechaza todo lo que registra con fecha, **también cobrar**, con `SYSTEM_CLOCK_INVALID`. Siguen las lecturas, los
  reportes, el historial, los respaldos, la licencia, la sesión, el canal de eventos y el agente de impresión (L1-D14).
  Se quita solo en cuanto el reloj se corrige.
- **La pantalla** «Sistema detenido por la fecha y hora de la computadora» se abre sola en la computadora principal, una
  vez por bloqueo, con las dos fechas de Go, por qué se detiene, qué sí se puede hacer y cómo resolverlo. Las terminales
  muestran un aviso fijo y, al registrar, «No se puede registrar: la computadora principal tiene la fecha y hora
  incorrectas. Avisa al encargado.».
- **La aclaración de la licencia**: «Tu licencia está bien…» solo en Vigente o Tolerancia. Como el estado se calcula con
  la hora confiable (§5.6 del contrato), un salto al futuro también puede poner una suscripción en Tolerancia o Vencida:
  en esos dos estados la pantalla dice que corregir la fecha y hora puede resolver también el aviso de la licencia
  (L1-D58). En Revocada y Sin activar dice que es otro problema.
- **«Comprobar la hora con internet»** (§6.3, L1-D5 y L1-D53): con una activación vigente, usa la consulta (`refresh`) de
  esa activación, cuyo `server_time` es contractual. Si coincide con el reloj (±10 min), la hora confiable pasa a ese
  `server_time` aunque solo difieran segundos (L1-D57) y se audita `license.clock_corrected` con la lista de las
  operaciones con fecha futura, que no se modifican. Si no coincide, `LICENSE_TIME_MISMATCH` con la hora de internet.
  Tiene un piso: nunca baja de la emisión de la licencia vigente.
- Las tres rutas de la hora (`license/time-check`, `license/support-request` y `license/support-unlock`) solo van por la
  escucha local y con cualquier sesión: quien esté en la caja debe poder resolverlo. Desde otra terminal,
  `409 LICENSE_LOCAL_ONLY` (L1-D54).

### La huella del equipo

- Se lee el `IOPlatformUUID` en macOS (`/usr/sbin/ioreg`) y el `MachineGuid` en Windows; se guarda solo su SHA-256 con
  prefijo, en `license-machine.json`, firmado (L1-D10 y L1-D29). Si no se puede leer tras tres intentos, falla cerrada
  con su propio mensaje.
- **Una carpeta de datos que aparece en otra computadora** (huella distinta, registro faltante o alterado) queda como
  copiada: la licencia guardada se ignora sin borrarla y el estado es «Sin activar con datos», o «Sin activar» si la base
  no tiene operaciones (L1-D26), con el mensaje «Esta instalación se copió de otra computadora. Actívala o transfiere la
  licencia.».
- **Qué cuenta como operación** (L1-D25): una fila en `app.restaurant_accounts`, `app.cash_shifts`, `app.payments`,
  `app.inventory_movements` o `app.legacy_sales`.
- **La identidad nueva** (L1-D11): solo en una instalación copiada, en la computadora principal, con «Administrar la
  licencia» y con confirmación (`POST license/identity/regenerate`). Revoca las terminales emparejadas por red, aparta
  `identity/` a `identity-reemplazada-AAAAMMDD-HHMMSS/`, crea `server_id`, certificado, llave de licencias y huella
  nuevos y conserva la última hora confiable. No toca ningún dato. Pide reiniciar el servidor. Desde otra terminal,
  `409 LICENSE_IDENTITY_LOCAL_ONLY` (L1-D56).
- **Un respaldo restaurado en otra computadora** también queda «Sin activar con datos»: la llave y la licencia no viajan
  en el respaldo, así que se activa o se transfiere ahí (§1.1 de la especificación).

### El desbloqueo de soporte

Para un restaurante sin internet cuyo reloj quedó «antes» de lo registrado: un código de solicitud de 24 caracteres
ligado a la instalación, un número de un solo uso y la última hora confiable; una respuesta firmada con la llave de
soporte que se acepta una sola vez, solo en esa instalación y dentro de 24 horas (L1-D19). Una instalación copiada no lo
pide ni lo acepta (`LICENSE_INSTALLATION_COPIED`, L1-D46). El procedimiento completo está en
[DESBLOQUEO_SOPORTE.md](DESBLOQUEO_SOPORTE.md).

---

## 3. Estados y cupos

### Los estados

`licensing.Status` calcula el estado en memoria, sin base ni internet:

1. Sin licencia válida aceptada: **Sin activar** (`not_activated`), o **Sin activar con datos**
   (`not_activated_with_data`) si la base tiene operaciones.
2. Con `license_status: revoked`: **Revocada** (`revoked`), con el motivo (`deactivated`, `transferred` o `revoked`).
3. Si no, la §5.6: **Vigente** (`active`), **Tolerancia** (`grace`) o **Vencida** (`expired`), con
   `ahora_efectivo = máximo(reloj local, última hora confiable)`.

### Qué se permite en cada estado

La tabla de la §6 del contrato, en `licensing/operations.go`, con una fila por `(método, ruta)`. Una prueba de
completitud la compara con `api.rs`, `openapi.json` y las rutas locales; una escritura que no está en la tabla toma la
fila más restrictiva (solo Vigente y Tolerancia, y con fecha).

| Operación                                                               | Vigente / Tolerancia      | Vencida / Revocada                      | Sin activar (con o sin datos) |
| ----------------------------------------------------------------------- | ------------------------- | --------------------------------------- | ----------------------------- |
| Capturar: abrir cuenta, borrador, enviar, modificar, sin conexión       | Sí, con cupo              | No                                      | No                            |
| Cobrar, cancelar, descuentos y reorganizar lo capturado                 | Sí                        | Sí                                      | No                            |
| Abrir turno                                                             | Sí, con cupo              | Solo con cuentas pendientes, y con cupo | No                            |
| Cerrar turno y corte                                                    | Sí                        | Sí                                      | No                            |
| Cocina y Preparación                                                    | Sí (Preparación con cupo) | Sí                                      | No                            |
| Administración (usuarios, catálogo, configuración, emparejar)           | Sí                        | Sí                                      | No                            |
| Reportes, historial y respaldos                                         | Sí                        | Sí                                      | Sí                            |
| La licencia, la sesión, las preferencias, «Reiniciar» y liberar un cupo | Sí                        | Sí                                      | Sí                            |

- **Las reglas firmes** salen de la tabla: la puerta solo rechaza antes de escribir, ningún estado borra datos, cobrar lo
  abierto sigue en Vencida y Revocada, y reportes, historial y respaldos siempre.
- **«Cuentas pendientes de cobro»** (L1-D6): al menos una cuenta abierta con unidades activas.
- **En el arranque** se permiten el asistente (`setup`) y el alta local (`devices/enroll`), solo sin establecimiento y por
  la escucha local.
- **La puerta** (`administration/license_gate.go`) se llama en `Service.dispatch`, antes de las rutas públicas y en
  cuanto se autentica la terminal. **Primero la hora y después el estado.** El cupo se pide después del manejador.

### Los cupos (contrato §7)

| Cupo               | Lo ocupa                                                                                        | Se libera                                                                           |
| ------------------ | ----------------------------------------------------------------------------------------------- | ----------------------------------------------------------------------------------- |
| `cash_registers`   | La terminal que abre un turno y la que captura con el turno activo de su caja (L1-D3)           | Al cerrar el turno, también desde otra computadora: se liberan todas sus terminales |
| `order_takers`     | La terminal que captura sin turno activo en su caja, durante 15 minutos desde su última captura | Arriba, «Los 15 minutos del comandero»                                              |
| `kitchen_displays` | La terminal que entra a Preparación (`POST kitchen/display-mode`, L1-D7 y L1-D28)               | Al salir de Preparación, al cerrar esa sesión o con «Liberar cupo»                  |

- **Cobrar y las operaciones de efectivo nunca piden cupo.** Una `operation_id` ya registrada tampoco.
- **Asignación atómica** (`licenseseats/store.go`, `claim`): con `pg_advisory_xact_lock(7820148311)`, una sola sentencia
  borra lo que ya no está en uso y cuenta lo que sí, con la hora de Go. 7820148311 es siempre el último lock de la
  transacción y, con él tomado, no se publica ningún evento ni se pide otro lock (`CLAUDE.md` §3).
- **Una licencia nueva con menos cupos** no interrumpe a nadie: quien ya tiene su cupo lo conserva y solo se impide
  ocupar cupos nuevos hasta quedar dentro del límite.
- **Sin licencia válida**, los tres límites valen 0 y nada que pida cupo procede (falla cerrada). En Vencida o Revocada
  los cupos siguen siendo los de la licencia.
- **El escritorio** muestra los cupos en Configuración › Licencia y en Terminales (la columna «Cupo» y «Liberar cupo» de
  comandero o de cocina, que cierra las sesiones de esa terminal). Las cajas se liberan cerrando el turno.
- `GET devices/sessions-summary` conserva `by_kind` para el escritorio 0.14.0 y agrega `seats` con `in_use` y `limit`
  (L1-D22).
- **Plan básico** (1 caja, 0 comanderos, 0 pantallas de cocina): solo se captura en la terminal con el turno de caja
  abierto. Ni tabletas ni Preparación.

**Limitación conocida (L1-D28).** Un escritorio 0.14.0 conectado a un servidor 0.15.0 entra a Preparación sin ocupar
cupo de cocina, porque el cupo se pide con `POST kitchen/display-mode` y no con la lectura del tablero. Las versiones
mínimas de cliente no cambiaron (L1-D22): cada terminal debe actualizarse a la 0.15.0 para que se respete.

---

## 4. Los códigos de error

### La puerta (todos 409)

Ninguno es un 403: así no quedan como `access.denied` en la auditoría.

| Código                     | Caso                                                  | `details`                                                                                                                   |
| -------------------------- | ----------------------------------------------------- | --------------------------------------------------------------------------------------------------------------------------- |
| `LICENSE_NOT_ACTIVATED`    | Sin activar o Sin activar con datos                   | `operation`, `state`, `copied` y `machine_unverified`                                                                       |
| `LICENSE_EXPIRED`          | Vencida (también abrir turno sin cuentas pendientes)  | `operation`, `expires_at` y `grace_ended_at`; al abrir turno, `pending_accounts`                                            |
| `LICENSE_REVOKED`          | Revocada                                              | `operation` y `reason`; al abrir turno, `pending_accounts`                                                                  |
| `LICENSE_SEAT_UNAVAILABLE` | Sin cupo                                              | `seat` (`cash_registers`, `order_takers` o `kitchen_displays`), `in_use` y `limit`                                          |
| `SYSTEM_CLOCK_INVALID`     | Hora del equipo incorrecta (no empieza con `LICENSE`) | `blocked`, `local_time`, `last_activity_time`, `margin_minutes`, `timezone`, los dos textos, `license_state` y `license_ok` |

Estos cinco son parte del contrato v1 para clientes (`docs/API_V1.md`, «Licencia: estado, hora del equipo y rechazos» y
«Cupos de la licencia»). Android muestra «La licencia del sistema no permite capturar pedidos. Avisa al encargado.» con
los tres primeros y el mensaje de §5 con el de cupo, con los números de `details`.

### La gestión de la licencia y la hora

| Código                             | HTTP | Caso                                                                                                                    |
| ---------------------------------- | ---- | ----------------------------------------------------------------------------------------------------------------------- |
| `LICENSE_KEY_INVALID_FORMAT`       | 400  | El código no tiene el formato: 15 símbolos de `23456789ABCDEFGHJKMNPQRSTUVWXYZ` en grupos 3-4-4-4 (L1-D4)               |
| `LICENSE_KEY_NOT_FOUND`            | 404  | `LICENSE_NOT_FOUND` del servidor de licencias                                                                           |
| `LICENSE_SERVER_REVOKED`           | 409  | `REVOKED`: la licencia fue revocada                                                                                     |
| `LICENSE_ACTIVATION_LIMIT`         | 409  | Activa en otra computadora; `details.transfer_available`                                                                |
| `LICENSE_TRANSFER_LIMIT`           | 409  | Transferencia pendiente: «Comunícate con soporte»                                                                       |
| `LICENSE_INSTALLATION_COPIED`      | 409  | La instalación se copió de otra computadora: primero la identidad nueva (también en el desbloqueo)                      |
| `LICENSE_NOT_ACTIVATED`            | 409  | Buscar actualización, liberar o pedir `renew`/`deactivate` sin una activación                                           |
| `LICENSE_TIME_MISMATCH`            | 409  | «Comprobar la hora con internet» no coincide; `details.server_time` y su texto                                          |
| `LICENSE_LOCAL_ONLY`               | 409  | Una ruta de la hora desde otra terminal (L1-D54)                                                                        |
| `LICENSE_IDENTITY_LOCAL_ONLY`      | 409  | La identidad nueva desde otra terminal (L1-D56)                                                                         |
| `IDENTITY_REGENERATION_NOT_NEEDED` | 409  | La identidad nueva en una instalación que no se copió                                                                   |
| `LICENSE_SEAT_NOT_HELD`            | 409  | «Liberar cupo» en una terminal sin cupo de comandero ni de cocina; `details.seats`                                      |
| `LICENSE_SERVER_BUSY`              | 429  | `RATE_LIMITED`; `details.retry_after_seconds`                                                                           |
| `LICENSE_SERVER_ERROR`             | 502  | `INVALID_REQUEST`, `INVALID_PROOF`, `REQUEST_CONFLICT`, `INCOMPATIBLE_SCHEMA` u otra respuesta inesperada               |
| `LICENSE_RESPONSE_INVALID`         | 502  | La licencia recibida no pasó la §10 y no se instaló; `details.reason`                                                   |
| `LICENSE_SERVER_UNAVAILABLE`       | 503  | Sin conexión, falla temporal o TLS; `details.reason`. La licencia instalada no cambia                                   |
| `LICENSE_STATE_NOT_SAVED`          | 503  | No se pudo guardar la licencia o la hora; no cambió nada                                                                |
| `IDENTITY_UNAVAILABLE`             | 503  | Falta la identidad o la llave, o no se pudo comprobar la computadora (`details.reason`)                                 |
| `LICENSE_FILE_INVALID`             | 400  | El `.lic` no vale para esta computadora; `details.reason` es el motivo del verificador                                  |
| `SUPPORT_UNLOCK_INVALID`           | 400  | La respuesta de soporte no vale; `details.reason`: `used`, `other_installation`, `expired`, `altered` o `not_requested` |
| `SUPPORT_UNLOCK_UNAVAILABLE`       | 503  | Esta versión no tiene la llave pública de soporte                                                                       |

Las rutas de la gestión son del escritorio y no son parte del contrato v1 para clientes: están en `API_CONTRACTS.md`,
«Etapa L1». Los textos de cada código están en `desktop/src/services/license.ts` y, en Android, en
`core:domain/error/LicenseMessages.kt`.

---

## 5. Las herramientas de prueba

**Ninguna prueba automática llama a `licensing.sicrat.com`** (§9.1 de la especificación).
`TestNoAutomatedTestCallsTheRealLicenseServer` busca el nombre en todos los archivos de pruebas del repositorio y en las
configuraciones de ejemplo, y en `test_mode` el cliente rechaza cualquier destino que no sea loopback, empezando por la
dirección compilada (en la bitácora queda `license_server_disabled` con `remote_not_allowed`). Las pruebas usan **solo**
las llaves de `contracts/licencias/v1/fixtures/vectors.json`.

- **Las pruebas de Go**: `vectors_test.go` recorre todos los casos de `vectors.json`, como el verificador de referencia.
  `licensing/licensingtest` y `licenseseats/seatstest` dan gestores, cupos y relojes simulados (solo los importan
  pruebas). Los paquetes de integración `./internal/licensing` y `./internal/licenseseats` están en la tupla de
  `scripts/database.py`.
- **Las ayudas del servidor**, solo con `test_mode` y en una compilación de desarrollo (L1-D25):
  - `ADEL_TEST_LICENSE_TRUST=<ruta de vectors.json>`: confía en las llaves de prueba;
  - `ADEL_TEST_CLOCK_FILE=<archivo>`: un reloj simulado, con un desfase con formato de duración de Go (`-36h`), que se
    lee cada segundo. Nunca se cambia la hora de la computadora;
  - `ADEL_TEST_LICENSE_CA=<PEM>`: la autoridad del servidor de licencias simulado.
- **`backend/cmd/license-test`**, un programa aparte del servidor, con `ADEL_TEST_MODE=1`:
  - `issue`: firma un `.lic` de prueba para la identidad de un Go temporal (`--identity`), con el tipo, las fechas, el
    plan, los cupos, la revisión y el estado que se pidan. Se importa con `POST license/import`;
  - `server`: un servidor de licencias simulado, con TLS y solo en loopback, que cumple la API del contrato. El primer
    grupo del código comercial decide el comportamiento (`PRP` perpetua 3/3/3, `BSC` plan básico, `MNS` mensual, `GRC`
    en tolerancia, `VNC` vencida, `ACT` activa en otra computadora…), y `/_prueba/*` controla el siguiente error, la
    hora, una revocación o una renovación. Son códigos inventados del simulador, no licencias reales.
  - Nunca llega a un instalador: sus fuentes llevan `//go:build !adelpos_production`, `guard_test.go` comprueba que el
    servidor no depende de ella y que los guiones de los instaladores solo compilan el servidor, y nunca se compila en
    `bin/` (L1-D51). Opciones y ejemplos: el avance, Fase B, «Herramientas de prueba».
- **Playwright** (L1-D51): `installDriver` (`desktop/e2e/native-driver.ts`) activa cada base nueva del Go temporal con
  una perpetua de prueba de 20 cupos de cada tipo, firmada con `license-test issue`. Es **el único lugar** donde las
  pruebas activan. Las especificaciones de la etapa (`workflow-stagel1-*`) cambian la licencia y la hora y, al terminar,
  dejan esa misma licencia y el reloj en la hora real.
  - `scripts/browser-tests.py` arranca el simulador en 8189 y un tercer Go temporal sin activar en 8187 (solo para
    `workflow-stagel1-activation`). `scripts/windows/dev.mjs` solo pasa la confianza en las llaves de prueba (L1-D52).
  - En el arnés aislado de las etapas, `ISO_LICENSE_SIM=1` hace lo mismo, con `ISO_LICENSE_PORT` y
    `ISO_UNLICENSED_PORT`. **Sin `ISO_LICENSE_SIM=1`, el Go temporal no se activa y todas las especificaciones fallan.**
- **Android** pone el estado del Go temporal con `license-test issue` y `license/import` desde la computadora principal
  (`android/CLAUDE.md`).
- **El desbloqueo de soporte aceptado** solo se prueba en Go, con una llave de soporte de prueba: la privada de
  desarrollo nunca está en las pruebas.

Las pruebas contra `licensing.sicrat.com` (Fase G, con las tres licencias de prueba) se hicieron a mano, con instancias
de desarrollo separadas y `ADEL_LICENSE_ALLOW_REMOTE=1`. No son una prueba automática.

---

## 6. Propuestas para la V1.1 del contrato

Anotadas en el avance; ninguna cambia la V1.0:

- escribir en la §6 la excepción de la hora: con «Hora del equipo incorrecta» tampoco se cobra (L1-D1);
- decir «una terminal» en el cupo de comandero (L1-D2) y aclarar la unidad del cupo de caja con cajas compartidas
  (L1-D3);
- escribir el formato del código comercial en la §8.3 (L1-D4) y dar `server_time` en el desafío (L1-D5 y L1-D53);
- que el contenido firmado rechace claves repetidas y datos después del objeto (L1-D33);
- el defecto G-1 del servidor de licencias: un `request_id` reutilizado con otro desafío responde `INVALID_PROOF` en vez
  de `REQUEST_CONFLICT` (L1-D40);
- las sesiones emitidas durante un salto del reloj conservan su vencimiento largo (L1-D50);
- pasar el desbloqueo de soporte al panel del servidor de licencias ([DESBLOQUEO_SOPORTE.md](DESBLOQUEO_SOPORTE.md)).
