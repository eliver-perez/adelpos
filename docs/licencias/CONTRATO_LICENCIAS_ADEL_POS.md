# Contrato de licencias AD-EL POS — V1.0

Documento compartido entre dos proyectos:

- **Servidor de licencias de AD-EL POS** (PHP/MySQL), en `licensing.sicrat.com` para pruebas.
- **AD-EL POS:** el servidor Go del restaurante y su etapa de integración de licencias.

**Esta es la versión 1.0 definitiva del contrato.** Sustituye al borrador inicial del 16 de noviembre de 2026 e incorpora las 37 aclaraciones que surgieron al implementar las etapas 2 a 4 del servidor de licencias. Al final hay una tabla de trazabilidad.

Ninguna instalación ha usado el borrador, así que no hay versiones anteriores que mantener. Las licencias llevan `schema_version: "1.0"`.

Ningún proyecto puede cambiar por su cuenta campos, significados, firmas, rutas ni reglas de fechas. Si surge una incompatibilidad, se registra una propuesta y se conserva esta versión hasta que el propietario apruebe la siguiente (1.1).

Las decisiones comerciales vienen del propietario (sección 14). Este documento solo las traduce a reglas técnicas.

---

## 1. Responsabilidades

| Componente | Responsabilidad |
| --- | --- |
| Servidor de licencias | Emitir, firmar, renovar, convertir, cambiar de plan, transferir, revocar y auditar licencias. Calcular todas las fechas. Administrar planes, precios, pagos, clientes y alertas. |
| Servidor Go del restaurante | Guardar su identidad y su llave. Activar, transferir, consultar actualizaciones, liberar e importar archivos. Verificar la licencia firmada sin internet. Aplicar estados y cupos. |
| Terminales (cajas, comanderos Android, KDS) | Se autentican con el servidor Go del restaurante. **Nunca** hablan con el servidor de licencias ni aplican reglas comerciales propias. |

La licencia pertenece a la **computadora principal del restaurante**, no a las terminales. Una licencia tiene como máximo **una activación vigente**. No se cobra por usuario creado ni por dispositivo registrado.

---

## 2. Identificadores

| Campo | Formato | Origen |
| --- | --- | --- |
| `product_id` | Siempre `adel_pos`. El servidor no emite ni firma licencias de otro producto. | Fijo |
| `license_id` | UUID | Servidor de licencias, al emitir |
| `activation_id` | UUID | Servidor de licencias, en cada activación o transferencia |
| `server_id` | UUID v4, **solo en minúsculas** | Identidad existente de AD-EL POS (`identity/`). Nunca se regenera sola y no viaja en los respaldos. |
| `installation_public_key` | base64url sin relleno, 32 bytes Ed25519 | Llave de licencias de la instalación |
| `request_id` | UUID | Cliente; una por operación, para que los reintentos no dupliquen nada |
| `challenge_id` | UUID | Servidor de licencias |
| `app_version` | De 1 a 100 caracteres, sin caracteres de control | Cliente |

- Los UUID distintos de `server_id` se aceptan en mayúsculas o minúsculas, y se usan tal como se recibieron en la prueba de posesión.
- **La identidad de una instalación es el par `server_id` + `installation_public_key`.** La misma identidad con otra llave, o la misma llave con otra identidad, es **otra instalación**.

**Llave de licencias de la instalación:**
- El servidor Go genera una sola vez, dentro de `identity/`, un par de llaves **Ed25519 exclusivo para licencias**, separado del certificado ECDSA de la red local.
- Igual que el `server_id`, nunca se regenera sola ni viaja en los respaldos. La llave privada nunca sale del equipo.
- Si la identidad se pierde, o si se restaura un respaldo en otra computadora, esa computadora tiene otra identidad y debe activarse o transferirse.

---

## 3. Formatos y criptografía

**Formatos:**
- JSON en UTF-8. Las llaves y firmas binarias van en base64url sin relleno; el receptor rechaza las codificaciones no canónicas.
- Los instantes van en RFC 3339, en UTC y terminados en `Z`. El servidor **firma siempre seis decimales** (`2026-11-16T06:30:00.000000Z`). El esquema publicado y el cliente aceptan de 0 a 9 decimales. Nunca se aceptan horas locales con desfase.

**Licencia firmada:** JWS Compact con este encabezado exacto:

```json
{"alg":"EdDSA","kid":"<kid>","typ":"adelpos-lic+jws"}
```

**Entornos de firma separados:** `test` y `production`.
- El servidor se configura con `LICENSE_ENVIRONMENT`, que es independiente del modo de la aplicación. `licensing.sicrat.com` funciona en modo de producción, pero firma con el entorno `test`.
- `environment` en la licencia debe coincidir con el entorno de la llave `kid`.
- Las versiones de producción de AD-EL POS **solo confían en llaves de producción**. Las versiones de desarrollo, solo en llaves de prueba. Una licencia de prueba nunca funciona en una instalación de producción.
- Una base de datos del servidor pertenece a un solo entorno. Pasar a producción implica otro servidor, con otra base y otras llaves.

**Llaves:**
- El cliente solo contiene **llaves públicas**, asociadas a su `kid`. Se exportan con `bin/signing-key.php --action=export`.
- Las llaves privadas nunca se incluyen en repositorios ni en instaladores.
- Las llaves de los archivos de prueba compartidos (sección 13) son públicas y deterministas. El servidor las rechaza en producción y el cliente de producción no confía en ellas.

---

## 4. Contenido exacto de la licencia firmada

Todos los campos son obligatorios. Los que admiten `null` lo indican.

```json
{
  "schema_version": "1.0",
  "product_id": "adel_pos",
  "environment": "production",
  "license_id": "UUID",
  "activation_id": "UUID",
  "server_id": "UUID v4",
  "installation_public_key": "base64url de 32 bytes",
  "licensee_name": "Nombre o razón social del cliente",
  "license_type": "subscription",
  "billing_period": "monthly",
  "license_status": "active",
  "status_reason": null,
  "license_revision": 7,
  "issued_at": "2026-11-16T06:30:00.000000Z",
  "period_starts_at": "2026-11-16T06:30:00.000000Z",
  "expires_at": "2026-12-16T06:30:00.000000Z",
  "grace_days": 5,
  "updates_until": "2026-12-16T06:30:00.000000Z",
  "plan": { "plan_id": "elemental", "name": "Plan elemental", "custom": false },
  "limits": { "cash_registers": 3, "order_takers": 3, "kitchen_displays": 3 },
  "features": {}
}
```

| Campo | Regla |
| --- | --- |
| `schema_version` | El servidor firma `"1.0"`, y el esquema publicado lo fija. El cliente acepta cualquier valor que empiece con `1.`. |
| `environment` | `production` o `test` |
| `licensee_name` | La razón social del cliente si está capturada; si no, su nombre comercial. Máximo 190 caracteres. Se toma al firmar. |
| `license_type` | `perpetual`, `subscription` o `trial` |
| `billing_period` | En suscripciones: `monthly`, `quarterly`, `semiannual` o `annual`. `null` en perpetua y prueba. |
| `license_status` | Solo `active` o `revoked`. Es `active` únicamente si la licencia comercial y la activación están vigentes. |
| `status_reason` | `null` si está `active`. Si está `revoked`: `transferred` (se transfirió a otra computadora, o el propietario la retiró de esa computadora), `deactivated` (la computadora liberó la licencia) o `revoked` (el propietario revocó la licencia). |
| `license_revision` | Entero ≥ 1. Es un contador **global por licencia** que aumenta en cada firma nueva, de cualquier activación. Puede saltar números y nunca se reutiliza. |
| `issued_at` | Instante de la firma |
| `period_starts_at` | Inicio del periodo vigente. **Puede estar en el futuro** después de una renovación anticipada (§5.3). En una perpetua es el inicio del primer periodo, o la fecha de pago si viene de una prueba convertida (§5.4). Es informativo: el estado se calcula solo con `expires_at` (§5.6). |
| `expires_at` | Instante exclusivo de vencimiento. `null` solo en perpetua. |
| `grace_days` | `5` en suscripción; `0` en prueba y en perpetua |
| `updates_until` | Última fecha de publicación de versiones a la que tiene derecho la instalación |
| `plan` | Informativo. `plan_id` y `name` vienen del catálogo; el nombre se toma cada vez que se firma. Los planes iniciales se llaman «Plan básico», «Plan elemental» y «Plan premium». `custom: true` si la licencia tiene cupos personalizados. |
| `limits` | Enteros de 0 a 999. En V1 no existe «sin límite». **Son los derechos reales.** Mientras espera una reducción programada (§11), una licencia puede firmar cupos **mayores** que los de su plan con `custom: false`. |
| `features` | Objeto `clave → true`, solo con los módulos habilitados. Claves con el formato `^[a-z][a-z0-9_]{1,63}$`. En V1 va vacío (`{}`). Siempre es un objeto, nunca una lista. |

**El cliente:**
- rechaza la licencia si falta un campo obligatorio o si su tipo o formato es incorrecto;
- ignora los campos adicionales desconocidos;
- ignora las claves de `features` que no conoce, sin habilitarlas para nada;
- trata como incompatible una `schema_version` que no empiece con `1.`.

**El servidor valida cada contenido antes de firmarlo.** Si no cumple, no firma y la operación se revierte por completo.

---

## 5. Reglas de fechas (las calcula únicamente el servidor de licencias)

### 5.1 Meses de calendario

- Las fechas se calculan en la **zona horaria del cliente** (IANA, guardada en su ficha; por omisión `America/Monterrey`) y se firman en UTC.
- **Sumar N meses** conserva el **día ancla** y la hora local. Si el día no existe en el mes destino, se usa el último día de ese mes. El ancla se guarda para que el día no se recorra: 31 ene → 28 feb → 31 mar → 30 abr.
- **Hora inexistente** por un cambio de horario: se usa el **primer instante después del salto**. Por ejemplo, 02:30 inexistente → 03:00.
- **Hora repetida:** se usa su **primera vez**. En ninguno de los dos casos cambian el día ni la hora ancla.
- **Duraciones de suscripción:** mensual 1, trimestral 3, semestral 6 y anual 12 meses.
- **Duraciones de prueba:** 15 días, 1 mes o 3 meses. «15 días» son 15 días de calendario a la misma hora local.

### 5.2 Primer periodo

- **Activación en línea:** el periodo empieza en el instante en que el servidor confirma la activación.
- **Activación sin internet:** empieza en el instante en que se aprueba la solicitud y se genera el `.lic`.
- El día ancla y la hora ancla se toman de ese instante.
- **Las fechas pertenecen a la licencia, no a la activación.** Si la licencia se libera o se transfiere y se activa en otra computadora, conserva sus fechas. Transferir nunca reinicia, extiende ni modifica las fechas.
- **Una licencia emitida pero no activada:**
  - no tiene fechas;
  - el panel puede editar su periodo inicial (tipo, periodo o duración, meses de actualizaciones, plan y cupos), sin registrar un pago;
  - no puede renovarse.
- Una suscripción o una prueba vencida se puede activar: el servidor firma sus fechas y AD-EL POS calcula el estado.

### 5.3 Renovación de suscripciones

El administrador captura la **fecha de pago** y la **duración**. La fecha de pago por omisión es la actual; no puede ser futura ni anterior al último pago registrado de la licencia. La duración puede ser distinta de la anterior.

```
limite_tolerancia = expires_at + grace_days × 24 h          (exclusivo, en UTC)

si fecha_pago < limite_tolerancia:
    base = expires_at        (pagó antes de vencer o dentro de la tolerancia; conserva el día ancla)
si no:
    base = fecha_pago        (el día ancla y la hora local pasan a ser los del pago)

period_starts_at = base
nuevo expires_at = base + duración, aplicando la regla del día ancla
```

Ejemplos, todos con hora local 10:00:

| Vence | Fecha de pago | Nuevo vencimiento | Motivo |
| --- | --- | --- | --- |
| 2 de marzo | 1 de marzo | 2 de abril | Pagó antes de vencer |
| 2 de marzo | 5 de marzo | 2 de abril | Pagó dentro de la tolerancia |
| 2 de marzo | 7 de marzo, 10:00 o después | 7 de abril | La tolerancia terminó y se regalan esos 5 días |
| 2 de marzo | 20 de marzo | 20 de abril | La tolerancia terminó |

- Al renovar, la licencia **conserva sus cupos** y su marca de personalizada. Si tiene una reducción programada cuyo instante ya llegó, se aplica en la misma revisión (§11).
- **No se renueva:** una perpetua (se extiende con mantenimiento), una prueba (se convierte), una suscripción sin activar (se edita su periodo inicial) ni una licencia revocada.
- **Sin internet:** al aprobar el `renew` de una computadora que sigue activa, el propietario puede registrar ahí mismo la renovación con su pago y entregar la revisión nueva. Si esa computadora ya liberó la licencia, recibe su cierre, y la renovación se registra desde el detalle de la licencia.

### 5.4 Pruebas y su conversión

- **Al emitir una prueba** se eligen el plan y la duración. No tiene tolerancia (`grace_days: 0`) y `updates_until = expires_at`.
- **Conversión.** Una prueba se convierte en **cualquier tipo**:
  - suscripción, con cualquier periodo;
  - perpetua, con 0, 6 o 12 meses de actualizaciones.

  Puede convertirse a cualquier plan o con cupos personalizados, durante la prueba o después de vencida. No hay bonificación.
- **Prueba activada:**
  - el periodo nuevo empieza en la **fecha de pago**, y los días de prueba que queden no se acumulan;
  - la fecha de pago no puede ser anterior al inicio de la prueba;
  - se firma una revisión nueva para la misma activación, sin reinstalar;
  - si se convierte en perpetua, `period_starts_at` es la fecha de pago y las actualizaciones cuentan desde ella.
- **Prueba sin activar:** cambian sus condiciones y las fechas se fijan al activar.
- **Transferencia:** una prueba también se puede transferir por autoservicio (§8.4) y conserva su vigencia.

### 5.5 Actualizaciones

- **Suscripción y prueba:** `updates_until = expires_at`.
- **Perpetua:** al emitir se eligen 0, 6 o 12 meses (6 por omisión), contados desde el inicio del primer periodo (§5.2). Con 0 meses, `updates_until` es igual al inicio.
- **Mantenimiento de una perpetua:**
  - solo se compra por 12 meses;
  - `updates_until` pasa a ser el mayor entre la fecha de pago y el `updates_until` vigente, más 12 meses;
  - solo para perpetuas activadas y no revocadas.
- **Fecha de publicación de las versiones:** la comparación le corresponde al actualizador de AD-EL POS, fuera de este contrato. Cuando termina el periodo de actualizaciones, la versión instalada sigue funcionando.

### 5.6 Estado temporal, calculado por el cliente

```
ahora_efectivo = máximo(reloj_local, última_hora_confiable)

perpetua:                                 vigente
ahora_efectivo < expires_at:              vigente
expires_at <= ahora_efectivo < limite:    tolerancia   (limite = expires_at + grace_days × 86 400 s)
ahora_efectivo >= limite:                 vencida
```

- **El estado se calcula solo con `expires_at` y `grace_days`.** Un `period_starts_at` en el futuro es válido y no afecta el estado.
- **`última_hora_confiable`** es el valor más reciente, guardado de forma persistente, entre estos tres:
  - el `server_time` de las respuestas;
  - el `issued_at` de las licencias aceptadas;
  - la última hora local registrada mientras el sistema funcionaba.
- Atrasar el reloj nunca prolonga una licencia. Las demás protecciones contra cambios de reloj y contra copias de la base le corresponden a AD-EL POS.

---

## 6. Estados de la licencia en AD-EL POS

| Estado | Condición |
| --- | --- |
| **Vigente** | Licencia válida, `active`, antes de su vencimiento |
| **Tolerancia** | Licencia válida, `active`, dentro de sus `grace_days` |
| **Vencida** | Licencia válida, `active`, con la tolerancia agotada |
| **Revocada** | Licencia válida con `license_status: revoked`. El mensaje depende de `status_reason`. |
| **Sin activar** | Ninguna licencia válida y la base sin operaciones |
| **Sin activar con datos** | Ninguna licencia válida, pero la base tiene operaciones (por ejemplo, un respaldo restaurado) |

Un archivo alterado, firmado con una llave no confiable o destinado a otra instalación **se ignora**. En ese caso se usa la última licencia válida aceptada. Si no hay ninguna, el estado es «sin activar».

| Operación | Vigente | Tolerancia | Vencida / Revocada | Sin activar con datos | Sin activar |
| --- | --- | --- | --- | --- | --- |
| Crear pedidos y agregar productos a cuentas abiertas | Sí, dentro de los cupos | Sí, dentro de los cupos | **No** | No | No |
| Cobrar, cancelar y aplicar descuentos a lo ya capturado | Sí | Sí | **Sí** | No | — |
| Abrir turno de caja | Sí, dentro de los cupos | Sí, dentro de los cupos | Solo si hay cuentas pendientes de cobro, y dentro de los cupos | No | No |
| Cerrar turno y hacer el corte | Sí | Sí | Sí | No | — |
| KDS: mostrar pedidos ya enviados | Sí | Sí | Sí | No | — |
| Reportes, historial y respaldos | Sí | Sí | Sí | Sí | — |
| Administración (usuarios, catálogo, configuración) | Sí | Sí | Sí | No | No |
| Activar, transferir, renovar o importar un `.lic` | Sí | Sí | Sí | Sí | Sí |

**Reglas firmes:**
- Ningún estado borra ventas, inventario, historial ni configuración.
- Ningún estado impide cobrar una cuenta ya abierta en una instalación activada.
- Ningún estado bloquea la consulta de reportes ni del historial.
- Los permisos de usuario y los derechos de la licencia son independientes: tener permiso para cobrar no garantiza que haya un cupo libre.

---

## 7. Cupos de sesiones simultáneas (los aplica el servidor Go)

| Límite | Qué ocupa un cupo | Cuándo se libera |
| --- | --- | --- |
| `cash_registers` (cajas) | Un **turno de caja abierto** en una terminal. Una caja es una terminal operando como caja; no es una computadora, un usuario, un turno ni un cajón. | Al cerrar ese turno. Si la terminal se cae, el turno sigue abierto y el cupo sigue ocupado. Un administrador puede cerrar el turno desde otra computadora. Cambiar de cajero en la misma terminal no consume otro cupo. |
| `order_takers` (comanderos) | Una sesión, Android o de escritorio, que **captura pedidos sin turno de caja abierto** | Al cerrar la sesión, al salir de ese modo o tras un periodo sin actividad. Ese periodo se define en la etapa de integración. |
| `kitchen_displays` (KDS) | Una terminal en **modo Cocina** | Al salir de ese modo o al cerrar la sesión |

- Reportes, catálogo, inventario y administración no ocupan cupo.
- Se valida la operación: crear o modificar un pedido exige tener un turno de caja abierto o un cupo de comandero.
- La asignación de cupos debe ser atómica, para que dos terminales no obtengan el mismo cupo al mismo tiempo.
- Si una licencia nueva trae menos cupos de los que están en uso, no se interrumpe nada. Solo se impide ocupar cupos nuevos hasta quedar dentro del límite.
- **Nota para la integración:** el conteo actual (`sessions-summary`, por `desktop`, `android` y `kitchen`) cuenta tipos de dispositivo. Este contrato cuenta **lo que está haciendo cada terminal**. Una terminal de escritorio puede ser caja, comandero, cocina o solo administración.

---

## 8. API en línea

- Todas las peticiones van por HTTPS con TLS verificado, `Content-Type: application/json` y un máximo de 16 KiB.
- La dirección base es configurable en AD-EL POS. Para pruebas: `https://licensing.sicrat.com`.
- La especificación formal está en `contracts/v1/openapi.yaml` y `contracts/v1/license.schema.json`.

### 8.1 Desafío

`POST /v1/activations/challenge`

```json
{ "action": "activate", "product_id": "adel_pos", "server_id": "UUID", "activation_id": null }
```

- `action` puede ser `activate`, `transfer`, `refresh` o `deactivate`.
- `activation_id` es `null` en `activate` y en `transfer`, y obligatorio en `refresh` y en `deactivate`.

Respuesta `200`:

```json
{ "challenge_id": "UUID", "nonce": "base64url de 32 bytes", "expires_at": "RFC 3339" }
```

Cada desafío es de un solo uso, dura 5 minutos y solo sirve para la acción, el producto y la instalación que lo pidieron.

### 8.2 Prueba de posesión

```
mensaje = UTF8("ADELPOS-LIC-V1\n" + action + "\n" + challenge_id + "\n" + nonce + "\n" + product_id + "\n" + server_id + "\n" + (activation_id o "-"))
proof   = base64url(Ed25519.sign(llave_privada_de_licencias, mensaje))
```

- En `activate` y `transfer`, el servidor verifica la prueba con la `installation_public_key` enviada.
- En `refresh` y `deactivate`, la verifica con la llave vinculada a la activación.

### 8.3 Activar

`POST /v1/activations`

```json
{
  "request_id": "UUID", "product_id": "adel_pos", "license_key": "K7M-9R3X-6TVP-8CHD",
  "server_id": "UUID", "installation_public_key": "base64url", "challenge_id": "UUID",
  "proof": "base64url", "app_version": "0.12.0"
}
```

Respuesta de éxito `200`:

```json
{ "license_jws": "...", "server_time": "RFC 3339", "request_id": "UUID" }
```

Casos:

- **Primera activación:** calcula el primer periodo (§5.2).
- **La misma instalación vuelve a activar** (mismo `server_id` y misma llave, con activación vigente): recibe su revisión vigente, sin otra activación ni otras fechas.
- **La licencia está activa en otra instalación:** `409 ACTIVATION_LIMIT` con `details.transfer_available`. Vale `true` si no hay una transferencia por autoservicio pendiente de confirmar; si la hay, vale `false`.
- **Licencia revocada:** `403 REVOKED`, también con el código retirado al revocar.

### 8.4 Transferir por autoservicio

`POST /v1/activations/transfer` recibe **los mismos campos que activar** y, si tiene éxito, responde igual. Las reglas se aplican en este orden:

1. **Código desconocido, mal formado, retirado (de una licencia vigente) o de otro producto:** `404 LICENSE_NOT_FOUND`. La respuesta es idéntica en todos los casos.
2. **Licencia revocada:** `403 REVOKED`, también con el código retirado al revocar.
3. **La misma instalación ya está activa:** recibe su revisión vigente. No cuenta como transferencia.
4. **Ninguna activación vigente** (por ejemplo, la anterior ya liberó la licencia): es una **activación normal** que no cuenta como transferencia. Si es la primera activación, empieza el primer periodo.
5. **Hay una transferencia por autoservicio pendiente de confirmar:** `409 TRANSFER_LIMIT`.
   - En AD-EL POS se muestra «Comunícate con soporte».
   - La negativa consume el desafío, queda guardada como respuesta idempotente y genera una alerta en el panel.
6. **En cualquier otro caso**, en una sola transacción:
   - la activación anterior queda `transferred` y se firma su revisión de cierre `revoked` / `transferred`;
   - se crea la activación nueva y se firma su revisión `active`, con número mayor y **las mismas fechas, cupos, plan y módulos** de la licencia;
   - la transferencia queda **pendiente de confirmar**.

**Aplica a cualquier tipo de licencia,** incluida una prueba, vigente, en tolerancia o vencida. Una licencia vencida sigue vencida después de transferirse.

**Confirmación.** La transferencia pendiente se confirma, y el contador vuelve a 0, cuando la **computadora anterior** demuestra con su llave que es ella y recibe su revisión de cierre. Puede hacerlo de cualquiera de estas formas:
- `refresh` en línea;
- `deactivate` en línea;
- la aprobación de su `.licreq` de `renew` o de `deactivate` (§9).

La consulta de la computadora nueva y una llave ajena nunca confirman nada. La confirmación es idempotente.

**Liberación manual.** Cuando la computadora anterior no va a volver a conectarse (por ejemplo, porque se dañó), el superadministrador libera el contador desde el panel.
- Requiere un motivo obligatorio y volver a autenticarse.
- Queda en la auditoría quién lo hizo, cuándo y por qué.
- La computadora anterior no se entera. Si algún día se conecta, recibe su cierre.

**Transferencias del propietario.** Las que hace el propietario desde el panel, con computadora de destino o retirando la licencia de una computadora («recuperar plaza»):
- nacen confirmadas;
- **no consumen** el autoservicio;
- firman su cierre con `transferred`.

**Alertas en el panel** (no afectan a AD-EL POS):
- un intento bloqueado con `TRANSFER_LIMIT`;
- dos o más transferencias de la licencia, de autoservicio o del propietario y confirmadas o no, dentro de los 12 meses de calendario anteriores en la zona del cliente. **La ventana incluye su inicio:** una transferencia hecha exactamente 12 meses antes, al mismo día y hora local, cuenta.

### 8.5 Consultar actualización

`POST /v1/activations/refresh`

```json
{
  "request_id": "UUID", "product_id": "adel_pos", "server_id": "UUID", "activation_id": "UUID",
  "challenge_id": "UUID", "proof": "base64url", "app_version": "0.12.0"
}
```

- Responde `200` igual que activar, con la **revisión vigente** de esa activación. Puede ser más nueva que la que tiene AD-EL POS por una renovación, una conversión, un cambio de plan, una propagación del catálogo, una reducción programada o una revocación.
- Si la activación ya terminó, devuelve su revisión de cierre y **confirma la transferencia pendiente** que salga de ella.
- **No es obligatoria para operar.** AD-EL POS la hace en segundo plano cuando tiene internet; como valor orientativo, al arrancar y una vez al día.

### 8.6 Desactivar (liberar la licencia)

`POST /v1/activations/deactivate` recibe los mismos campos que consultar actualización. Respuesta `200`:

```json
{
  "activation_id": "UUID", "deactivated_at": "RFC 3339", "status": "deactivated",
  "license_jws": "revisión de cierre", "server_time": "RFC 3339", "request_id": "UUID"
}
```

- Libera la licencia y firma su cierre `revoked` / `deactivated`. Después se puede activar en otra computadora con el mismo código, sin límite y sin soporte.
- **Si la activación ya había terminado**, responde `200` con su revisión de cierre existente, sin firmar otra. En ese caso confirma la transferencia pendiente que salga de ella.
- `status` es siempre `"deactivated"`. **El motivo real va en `status_reason` de `license_jws`**, que puede ser `deactivated`, `transferred` o `revoked`.

### 8.7 Reintentos y errores

- **Reintentos:** se identifican por `product_id` + `request_id`. Un reintento idéntico recibe exactamente la misma respuesta. Reutilizar un `request_id` con otro contenido, en línea o sin internet, recibe `409 REQUEST_CONFLICT`.
- **Formato de error:**

  ```json
  { "error": { "code": "...", "message": "...", "request_id": "UUID o null", "details": {} } }
  ```

  - `request_id` es el que envió el cliente si es un UUID válido. Es `null` en el desafío y cuando falta o no es válido.
  - `details` siempre es un objeto.
  - `message` es informativo: AD-EL POS muestra sus propios textos a partir de `code`.

| Código | HTTP | Uso |
| --- | --- | --- |
| `INVALID_REQUEST` | 400 | Campos faltantes, sobrantes o con formato incorrecto |
| `INVALID_REQUEST` | 404, 405, 413, 415 | Errores de transporte: ruta inexistente, otro método (con `Allow: POST`), cuerpo de más de 16 KiB u otro tipo de contenido |
| `INVALID_REQUEST` | 403 | Petición sin TLS, fuera del desarrollo local |
| `INCOMPATIBLE_SCHEMA` | 400 | Un `.licreq` con una `schema_version` no admitida. Las peticiones en línea no llevan versión. |
| `INVALID_PROOF` | 401 | Desafío inválido, vencido, ya usado o de otra acción; prueba incorrecta; identidad desconocida |
| `REVOKED` | 403 | Licencia revocada, en `activate` y `transfer` |
| `LICENSE_NOT_FOUND` | 404 | Código inexistente, mal formado, retirado de una licencia vigente o de otro producto. La respuesta es la misma en todos los casos. |
| `ACTIVATION_LIMIT` | 409 | La licencia está activa en otra instalación. Incluye `details.transfer_available`. |
| `TRANSFER_LIMIT` | 409 | Hay una transferencia por autoservicio pendiente de confirmar |
| `REQUEST_CONFLICT` | 409 | `request_id` reutilizado con otro contenido |
| `RATE_LIMITED` | 429 | Límite de intentos. Incluye un límite por código comercial compartido entre `activate` y `transfer`. |
| `TEMPORARY_UNAVAILABLE` | 503 | Falla temporal. Nunca afecta a una licencia válida ya instalada. |

**AD-EL POS decide por `code`, no por el HTTP.** Los códigos comerciales nunca se escriben en registros ni en respuestas.

---

## 9. Activación y renovación sin internet

### 9.1 Solicitud `.licreq`

```json
{ "schema_version": "1.0", "payload_b64u": "...", "signature_b64u": "..." }
```

**Contenido de `payload_b64u`:**
- `action`: `activate`, `renew` o `deactivate`;
- siempre: `request_id`, `created_at`, `product_id`, `server_id`, `installation_public_key` y `app_version`;
- en `renew` y `deactivate`, además: `license_id` y `activation_id`;
- en `activate`, de forma opcional: `license_key`. **No se admite en las otras acciones.**

**Reglas:**
- `created_at` es un instante UTC terminado en `Z`, con 0 a 9 decimales. Es informativo: **ninguna fecha de la licencia sale de él**.
- **Firma:** `Ed25519(llave_privada_de_licencias, UTF8("ADELPOS-LICREQ-V1\n" + payload_b64u))`.
- El servidor guarda el archivo original cifrado y sin cambios. Si trae código comercial, nunca lo muestra.
- **Recibir una solicitud no concede nada:** el propietario la aprueba en el panel.

**Qué hace cada acción al aprobarse:**

| Acción | Resultado |
| --- | --- |
| `activate` | El propietario asigna la licencia y el primer periodo empieza al aprobar. Si la misma instalación ya está activa, recibe su revisión vigente. El propietario puede aprobarla como **transferencia del propietario** cuando la licencia está activa en otra computadora (§8.4). |
| `renew` | Entrega la revisión vigente, sin firmar otra. Si la computadora sigue activa, el propietario puede registrar ahí mismo una renovación con su pago (§5.3). |
| `deactivate` | Libera la licencia y entrega el cierre `deactivated`. |

**Activación terminada o licencia revocada:**
- `renew` y `deactivate` se atienden y entregan la **revisión de cierre**. Así la computadora se entera aunque no tenga internet.
- Si salen de la computadora anterior de una transferencia por autoservicio pendiente, **la confirman**.
- `activate` sí exige una licencia vigente.

### 9.2 Respuesta `.lic`

- Es el **mismo JWS Compact** de la API en línea, como texto UTF-8.
- No es posible borrar a distancia una copia que siga desconectada. Es una limitación conocida y aceptada:
  - una computadora anterior sin internet sigue funcionando hasta su vencimiento;
  - si es perpetua, sigue funcionando indefinidamente.

---

## 10. Verificación en el servidor Go del restaurante

Antes de aceptar una licencia, en línea o desde un `.lic`, el servidor Go comprueba:

1. **Encabezado y firma:** el encabezado exacto, que el `kid` sea de confianza **para el entorno de esa versión de AD-EL POS** y que la firma Ed25519 sea válida.
2. **Contenido:** que cumpla la §4; que `product_id` sea `adel_pos`; que `environment` coincida con el entorno del `kid`.
3. **Destinatario:** que `server_id` e `installation_public_key` sean exactamente los suyos.
4. **Revisión:** que `license_revision` sea **mayor** que la más alta ya aceptada para ese `license_id`. Una revisión igual solo se acepta si es idéntica byte por byte.
   - Esta regla rechaza los retrocesos incluso entre activaciones distintas.
   - Después de una transferencia, la computadora nueva acepta su primera revisión aunque tenga un número alto, porque nunca había aceptado otra de esa licencia.
5. **Guardado:** si todo es correcto, guarda de forma persistente la licencia, la revisión aceptada y la última hora confiable.

Además:
- La verificación se hace **en cada operación protegida** y en el backend. Las terminales solo muestran el estado.
- El servidor Go nunca necesita internet para verificar.
- La base de esta verificación es el verificador de referencia `contracts/v1/verify/`.

---

## 11. Planes y ciclo comercial (reglas del servidor de licencias)

Esta sección explica de dónde salen los cupos y las fechas que firma el servidor. **AD-EL POS no aplica estas reglas:** solo verifica y respeta lo firmado.

### 11.1 Catálogo de planes

- Se administra en el panel, solo por el superadministrador.
- Cada plan tiene nombre, cupos (cajas, comanderos y KDS) y **precios opcionales en MXN**: mensual, trimestral, semestral, anual, perpetua y mantenimiento anual.
- Un plan nunca se borra: se archiva, y entonces deja de ofrecerse.
- Valores iniciales, provisionales:

| Plan | Cajas | Comanderos | KDS |
| --- | --- | --- | --- |
| Básico (`basico`) | 1 | 0 | 0 |
| Elemental (`elemental`) | 3 | 3 | 3 |
| Premium (`premium`) | 5 | 10 | 5 |

- La licencia firma **números**, no paquetes. Si se ajustan los números de una licencia concreta, queda personalizada (`custom: true`) y deja de seguir al plan.

**Cambios del catálogo:**
- Al guardar, el propietario elige entre «solo licencias nuevas», que es la opción preseleccionada, o «también las existentes que siguen este plan».
- En las existentes se aplican **solo los aumentos**, cupo por cupo, y una licencia nunca baja.
- Se firma una revisión nueva para cada activación vigente.
- **Un cambio del catálogo nunca reduce las licencias existentes, ni siquiera al renovar.** Al renovar, la licencia conserva sus números.

### 11.2 Precios y pagos

- Se registra el monto cobrado de cada operación en MXN, sin IVA ni facturas.
- El registro de pagos es de solo inserción.
- El panel sugiere el precio del catálogo. Si el operador cobra un monto distinto, debe escribir un motivo.
- Una licencia personalizada no tiene precio de catálogo, así que su monto se escribe a mano.
- Una prueba registra un alta de 0.

### 11.3 Cambio de plan

La clasificación se hace **cupo por cupo**, contra los números vigentes de la licencia.

**Suscripción, pagada antes de vencer:**

| Cambio | Qué pasa |
| --- | --- |
| Aumento, o cambio mixto | El periodo nuevo empieza en la **fecha de pago**, con la bonificación de §11.4. Los aumentos se aplican de inmediato. |
| Reducción dentro de un cambio mixto | Se **programa** para el vencimiento del periodo pagado anterior. Mientras tanto la licencia firma el plan nuevo con `custom: false` y el número mayor de los cupos que bajan. |
| Solo reducción, o un cambio de plan que no sube ningún cupo | Se **programa** para el vencimiento, sin cobro. Lo cobra la siguiente renovación. Solo puede haber un cambio programado, y se puede cancelar. |

**Otros casos:**

| Caso | Qué pasa |
| --- | --- |
| Suscripción pagada después de vencer (en tolerancia o después) | No es un cambio de plan: es una **renovación con plan nuevo**, sin bonificación y con los números nuevos. Decide la **fecha de pago**, no la de registro. |
| Perpetua que sube | Se paga la diferencia entre los dos precios perpetuos y el cambio aplica de inmediato. `updates_until` no cambia. |
| Perpetua que baja, o cambio mixto en perpetua | Se rechaza. |
| Prueba | Cualquier cambio de plan es libre, sin cobro, y no cambia sus fechas. |

**Reducciones programadas:**
- Una tarea diaria aplica las reducciones cuyo instante ya llegó y firma su revisión.
- Si la licencia se renovó antes de ese instante, conserva los números mayores hasta el instante, y la tarea firma la reducción después.
- Una computadora sin internet recibe la revisión reducida en su siguiente consulta, `renew` o `.lic`. Mientras tanto conserva los números mayores.

### 11.4 Bonificación

```
valor_del_periodo(pago) = monto_cobrado + bonificación_aplicada_en_ese_pago

bonificación = Σ, sobre cada periodo pagado que todavía no termina en el instante del nuevo pago
               (el periodo en curso y los renovados por adelantado):
               valor_del_periodo × segundos_no_consumidos ÷ segundos_del_periodo

sugerido = máximo(0, precio_nuevo − bonificación)
```

- Se cuentan segundos completos.
- Un periodo que todavía no empieza cuenta completo.
- Cada parte se redondea al centavo, con la mitad hacia arriba, y después se suman.
- Si el periodo vigente no tiene un pago registrado, vale el precio del catálogo vigente de su plan y su periodo.

---

## 12. Privacidad y seguridad

- El servidor de licencias **solo recibe** el `server_id`, la llave pública, la versión de la aplicación y el código comercial. **Nunca** recibe ventas, productos, usuarios, inventario ni la base del restaurante.
- Ningún secreto de firma se comparte con AD-EL POS.
- La firma impide modificar localmente los derechos sin invalidarlos.
- Ni la identidad ni la firma impiden que el administrador de un equipo clone una máquina virtual completa o restaure instantáneas antiguas. Las protecciones locales adicionales le corresponden a AD-EL POS.

---

## 13. Archivos de prueba compartidos

En `contracts/v1/` del servidor de licencias:

- **`license.schema.json`** (JSON Schema 2020-12): el contenido de §4.
- **`openapi.yaml`** (OpenAPI 3.1): las cinco rutas de §8, con sus errores y ejemplos.
- **`fixtures/`**, generados con llaves públicas de prueba y comprobados con `generate-fixtures.php --check`:
  - **licencias válidas:**
    - perpetua con 0, 6 y 12 meses;
    - suscripción mensual y anual;
    - prueba;
    - personalizada;
    - los cierres `deactivated`, `transferred` y `revoked`;
    - renovada;
    - convertida a suscripción y a perpetua;
    - perpetua con mantenimiento;
    - subida de plan;
    - reducción aplicada;
    - las tres revisiones de una transferencia por autoservicio;
  - **casos negativos:** firma alterada; `kid` desconocido; llave de prueba presentada como de producción; entorno distinto; otra instalación; otro producto; revisión menor; campo faltante; `schema_version` 2.0; tipos incorrectos;
  - **casos de revisión**, incluidos los de la transferencia;
  - **casos de fechas:** primer periodo, renovación, conversión y mantenimiento;
  - **casos de bonificación;**
  - **estados temporales** con límites exactos y reloj atrasado;
  - **pruebas de posesión** de las cuatro acciones y **`.licreq`** de las tres acciones.
- **`verify/`:** el verificador de referencia en Go. Aplica §5.6 y §10 y ejecuta todos los casos. Es la base para la integración de AD-EL POS.

Las fechas y las bonificaciones las calcula solo el servidor. El verificador no las recalcula: los casos documentan el resultado esperado.

---

## 14. Decisiones del propietario incorporadas

1. La licencia se ata al servidor Go del restaurante: `server_id` más llave Ed25519. No se cobra por usuario ni por dispositivo.
2. No hay comunicación periódica obligatoria. El servidor fija y firma el vencimiento, también sin internet.
3. Suscripciones mensual, trimestral, semestral y anual, en meses de calendario. Si el día no existe, se usa el último del mes.
4. Al renovar, se cuenta desde el vencimiento si se pagó antes de vencer o dentro de la tolerancia. Si se pagó después, se cuenta desde la fecha de pago y se regalan los días de tolerancia.
5. Tolerancia de 5 días en suscripciones. Las pruebas no tienen tolerancia.
6. Pruebas de 15 días, 1 mes o 3 meses, con el plan elegido al emitir. Se convierten en cualquier tipo y también se transfieren.
7. Perpetua con 0, 6 o 12 meses de actualizaciones (6 por omisión), contados desde la activación en línea o desde la generación del `.lic`. El mantenimiento se compra solo por 12 meses.
8. Transferencia por autoservicio:
   - una sola pendiente; el contador vuelve a 0 al confirmarse, también sin internet;
   - el superadministrador puede liberar el contador a mano, con motivo y auditoría;
   - las alertas se muestran en el panel, sin correos.
9. Planes básico, elemental y premium, editables y con precios opcionales. Los aumentos se propagan solo si se elige. Un cambio del catálogo nunca reduce las licencias existentes.
10. Al subir de plan: bonificación de lo pagado y no consumido (suscripción) o diferencia de precio (perpetua). Al bajar de plan: aplica al vencer lo pagado. La perpetua no baja.
11. Al vencer, se puede cobrar y cerrar lo capturado, pero no capturar pedidos nuevos ni agregar productos.
12. Tabla de estados de la §6.
13. `licensing.sicrat.com` es solo para pruebas y usa llaves de prueba.
14. Transferir nunca modifica las fechas de una licencia: las fechas son de la licencia, no de la activación.

## 15. Fuera de V1.0

- El actualizador, los manifiestos de versiones y la firma de instaladores. Le corresponden a AD-EL POS.
- La huella del equipo y las protecciones locales contra copias o cambios de reloj más allá de la §5.6.
- El tiempo de inactividad que libera un cupo de comandero y los días de aviso antes del vencimiento. Se definen en la integración de AD-EL POS.
- Pasarela de pagos, facturación, impuestos y portal del cliente.
- Avisos a los clientes por cambios del catálogo y avisos por correo de las alertas.

---

## Anexo — Trazabilidad de las aclaraciones incorporadas al borrador

| N.º | Tema | Dónde quedó |
| --- | --- | --- |
| 1 | `product_id` siempre `adel_pos` | §2 |
| 2 | `licensee_name` | §4 |
| 3 | HTTP de los errores de transporte | §8.7 |
| 4 | Hora inexistente y hora repetida | §5.1 |
| 5 | `deactivate` de una activación terminada | §8.6 |
| 6 | Casos de prueba por etapa | §13 |
| 7 | `INCOMPATIBLE_SCHEMA` | §8.7, §9.1 |
| 8 | `request_id` en los errores | §8.7 |
| 9 | Nombres de los planes | §4 |
| 10 | Motivo de «recuperar plaza» | §4, §8.4 |
| 11 | Edición del periodo inicial | §5.2 |
| 12 | `renew` y `activate` sin internet | §9.1 |
| 13 | `app_version` y caja de los UUID | §2 |
| 14 | `status` de la respuesta de desactivar | §8.6 |
| 15 | Cierre sin internet de una licencia revocada | §9.1 |
| 16 | `created_at` de un `.licreq` | §9.1 |
| 17 | `schema_version` | §4 |
| 18 | Decimales de los instantes | §3 |
| 19 | Petición sin TLS | §8.7 |
| 20 | Identidad = `server_id` + llave | §2, §8.3 |
| 21 | El catálogo nunca reduce licencias existentes | §11.1 |
| 22 | Precios, conversión, cambio de plan y mantenimiento | §5.4, §5.5, §11 |
| 23 | `period_starts_at` en el futuro | §4, §5.3, §5.6 |
| 24 | Límites de la fecha de pago | §5.3 |
| 25 | Renovar al aprobar un `renew` | §5.3, §9.1 |
| 26 | `period_starts_at` de una prueba convertida en perpetua | §4, §5.4 |
| 27 | La bonificación suma todos los periodos no consumidos | §11.4 |
| 28 | Valor del periodo = cobrado + bonificación aplicada | §11.4 |
| 29 | Cambio de plan sin aumentos | §11.3 |
| 30 | Cupos mayores con `custom: false` mientras se espera la reducción | §4, §11.3 |
| 31 | Decide la fecha de pago | §11.3 |
| 32 | Editar el periodo inicial no registra un pago | §5.2 |
| 33 | `transfer` sin activación vigente = activación | §8.4 |
| 34 | Confirmación sin internet | §8.4, §9.1 |
| 35 | Liberación manual del contador | §8.4 |
| 36 | Código retirado de una licencia revocada → `REVOKED` | §8.3, §8.4, §8.7 |
| 37 | Ventana de 12 meses con su inicio incluido | §8.4 |
