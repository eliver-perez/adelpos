# Integración de licencias en AD-EL POS — guía de orientación

Para la etapa de integración de licencias en el **servidor Go del restaurante** de AD-EL POS. Dice qué leer, dónde está cada archivo, cómo obtener las llaves y qué probar contra `licensing.sicrat.com`.

**No sustituye al contrato.** Las reglas están en [`CONTRATO_LICENCIAS_ADEL_POS.md`](CONTRATO_LICENCIAS_ADEL_POS.md), versión 1.0 definitiva (SHA-256 `e3435f16e27e11c85ac263ef2accfa72aad07cf30dbcdee0a0b8b22326314058`). Si esta guía y el contrato difieren, manda el contrato. Si la integración encuentra una incompatibilidad, se registra una propuesta para la versión 1.1 y se sigue con la 1.0 hasta que el propietario la apruebe: ningún proyecto cambia el contrato por su cuenta.

---

## 1. Qué leer del contrato

| Sección | Para qué sirve en la integración |
| --- | --- |
| §1 Responsabilidades | Quién hace qué. La licencia es de la computadora principal; las terminales nunca hablan con el servidor de licencias. |
| §2 Identificadores | `server_id` (el de `identity/`) y la **llave Ed25519 de licencias** que el servidor Go genera una sola vez, aparte del certificado de la red local. |
| §3 Formatos y criptografía | JSON, base64url sin relleno, instantes en UTC, JWS Compact con su encabezado exacto, y los entornos `test` y `production`. |
| §4 Contenido de la licencia | Todos los campos de la licencia firmada. |
| §5.6 Estado temporal | **Lo único de fechas que calcula AD-EL POS:** vigente, tolerancia o vencida, con la última hora confiable. Las §5.1 a 5.5 son del servidor: no se recalculan. |
| §6 Estados | Los seis estados y qué operaciones permite cada uno, con sus reglas firmes. |
| §7 Cupos | Cajas, comanderos y pantallas de cocina, y cuándo se ocupa y se libera cada cupo. |
| §8 API en línea | Las cinco rutas, la prueba de posesión, los reintentos y los errores. |
| §9 Sin internet | Las solicitudes `.licreq` y las respuestas `.lic`. |
| §10 Verificación | Lo que el servidor Go comprueba antes de aceptar una licencia. |
| §12 Privacidad y seguridad | Qué no se guarda ni se registra. |
| §13 Archivos de prueba | Qué casos hay y para qué sirven. |

La §11 (planes y ciclo comercial) es del servidor de licencias. Para AD-EL POS basta saber que una renovación, una conversión, un cambio de plan o una reducción llegan como **una revisión nueva** de la misma licencia.

---

## 2. Dónde está cada archivo

Todo está en `contracts/v1/` de este repositorio, en la etiqueta **`v1.0.0`**:

| Archivo | Qué es |
| --- | --- |
| `contracts/v1/openapi.yaml` | La API en línea (§8) en OpenAPI 3.1: rutas, peticiones, respuestas, errores y ejemplos |
| `contracts/v1/license.schema.json` | El contenido de la licencia (§4) en JSON Schema 2020-12 |
| `contracts/v1/fixtures/vectors.json` | Todos los casos de prueba con su resultado esperado: licencias válidas y negativas, revisiones, fechas, bonificaciones, estados temporales, pruebas de posesión y `.licreq` |
| `contracts/v1/fixtures/licenses/*.lic` | Las licencias de prueba, tal como viajan |
| `contracts/v1/fixtures/requests/*.licreq` | Las solicitudes sin internet de las tres acciones |
| `contracts/v1/verify/` | El **verificador de referencia** en Go, sin dependencias externas: aplica la §10 y la §5.6 |
| `contracts/v1/README.md` | Qué contiene cada sección de `vectors.json` |

El verificador ejecuta todos los casos:

```sh
env GOTOOLCHAIN=local GOPROXY=off GOSUMDB=off go run -C contracts/v1/verify . ../fixtures/vectors.json
```

**Recomendación:** copia `contracts/v1/` al proyecto de AD-EL POS desde la etiqueta `v1.0.0`, sin cambios, y anota de dónde salió. Las pruebas de AD-EL POS deben pasar los mismos casos que el verificador de referencia. Si algo no coincide, el error está en la integración o hay que proponer la versión 1.1; los archivos no se editan.

---

## 3. Llaves públicas

Hay tres juegos de llaves y cada versión de AD-EL POS confía **solo en las de su entorno** (§3):

| Llaves | De dónde salen | Quién confía en ellas |
| --- | --- | --- |
| De los archivos de prueba | `signing_keys` de `vectors.json`, con semillas públicas | Solo las pruebas automáticas de AD-EL POS |
| Del entorno `test` de `licensing.sicrat.com` | La exportación del paso 8 de `DEPLOY_UBUNTU_24_04.md` | Las versiones de **desarrollo** de AD-EL POS |
| De producción | Otro servidor, con otra base y otras llaves (todavía no existe) | Solo las versiones de producción |

**Cómo obtener las del entorno `test`.** En el VPS, el propietario las exporta con:

```sh
cd /srv/adelpos-license/app
sudo -u adelpos-license env ADELPOS_LICENSE_CONFIG=/srv/adelpos-license/shared/config.php \
    php8.3 bin/signing-key.php --action=export | sudo tee /srv/adelpos-license/shared/llaves-publicas-test.json
```

El resultado es público y no lleva nada privado:

```json
{
    "environment": "test",
    "keys": [
        {
            "kid": "adelpos-test-202611-1",
            "environment": "test",
            "public_key": "base64url de 32 bytes"
        }
    ]
}
```

- Incluye las llaves preparadas, la activa y las anteriores del entorno: guarda **todas** en AD-EL POS, asociadas a su `kid`, para que las licencias firmadas con una llave anterior sigan valiendo.
- **Al rotar la llave:** el propietario genera la nueva, la exporta, AD-EL POS publica una versión de desarrollo que confía en ella y solo entonces el propietario la activa (`--trust-confirmed`).
- Una versión de producción nunca incluye estas llaves, ni las de los archivos de prueba.

---

## 4. Flujos en línea

La dirección base es configurable; para pruebas, `https://licensing.sicrat.com`. Todas las peticiones son `POST` por HTTPS con TLS verificado, `Content-Type: application/json` y un cuerpo de hasta 16 KiB (§8).

Toda operación empieza igual:

1. Pide un **desafío** (`POST /v1/activations/challenge`) para la acción. Dura 5 minutos y es de un solo uso.
2. Firma la **prueba de posesión** con la llave privada de licencias de la instalación (§8.2).
3. Envía la operación con un `request_id` nuevo.
4. **Verifica** el `license_jws` de la respuesta como dice la §10 antes de aceptarlo, y guarda el `server_time` como hora confiable.

| Operación | Ruta | Cuándo |
| --- | --- | --- |
| Activar | `POST /v1/activations` | La primera vez, con el código comercial que escribe el usuario. Si la misma instalación ya está activa, recibe su revisión vigente. |
| Transferir | `POST /v1/activations/transfer` | Cuando activar responde `409 ACTIVATION_LIMIT` con `details.transfer_available: true` y el usuario confirma que quiere mover la licencia a esta computadora. Mismos campos que activar. |
| Consultar actualización | `POST /v1/activations/refresh` | En segundo plano, con internet: como valor orientativo, al arrancar y una vez al día. Trae la revisión vigente, que puede ser más nueva. No es obligatoria para operar. |
| Liberar | `POST /v1/activations/deactivate` | Cuando el usuario quiere usar la licencia en otra computadora. Devuelve la revisión de cierre. |

**Cosas que conviene tener presentes:**

- **Transferencia.** La computadora nueva queda activa al instante. La anterior se entera en su siguiente consulta (o al liberar, o con un `.licreq`): recibe su revisión de cierre `revoked` / `transferred`, y eso **confirma** la transferencia. Mientras no se confirme, otra transferencia por autoservicio responde `409 TRANSFER_LIMIT`: AD-EL POS muestra «Comunícate con soporte» (§8.4).
- **Cierres.** Una revisión con `license_status: revoked` deja la licencia en el estado «Revocada». El motivo está en `status_reason` (`deactivated`, `transferred` o `revoked`), y el mensaje al usuario depende de él. En la liberación, el `status` de la respuesta siempre es `"deactivated"`: el motivo real va en la licencia (§8.6).
- **Reintentos.** Si una operación no recibió respuesta, se reintenta con **el mismo cuerpo**, incluido su `request_id`: recibe exactamente la misma respuesta. Un `request_id` reutilizado con otro contenido responde `409 REQUEST_CONFLICT` (§8.7).
- **Errores.** AD-EL POS decide por `error.code`, no por el código HTTP, y muestra sus propios textos. `TEMPORARY_UNAVAILABLE` y los errores de red nunca afectan a una licencia válida ya instalada.
- **Límites de intentos.** El servidor limita las peticiones por dirección y por código comercial (`429 RATE_LIMITED`, con `Retry-After`). Las consultas en segundo plano deben espaciarse y respetar esa espera.
- **Privacidad.** El servidor de licencias solo recibe el `server_id`, la llave pública, la versión de la aplicación y el código comercial, y los códigos comerciales nunca se escriben en registros (§8.7 y §12). AD-EL POS tampoco debe escribirlos en los suyos.

---

## 5. Flujo sin internet

Para una computadora que no tiene internet (§9):

1. AD-EL POS genera una solicitud **`.licreq`** firmada con su llave de licencias: `activate` (con el código comercial, opcional), `renew` o `deactivate`.
2. El usuario se la hace llegar al propietario (correo, USB…).
3. El propietario la importa en el panel, en **Sin internet**, revisa a qué licencia corresponde y la aprueba. **Recibir una solicitud no concede nada.**
4. El propietario descarga la respuesta **`.lic`** (el mismo JWS de la API en línea) y se la entrega al usuario.
5. AD-EL POS importa el `.lic` y lo verifica igual que una respuesta en línea (§10).

- `renew` entrega la revisión vigente. Si la computadora sigue activa, el propietario puede registrar ahí mismo una renovación con su pago.
- `deactivate` entrega el cierre `deactivated` y libera la licencia.
- Si la activación ya terminó (por ejemplo, la licencia se transfirió), `renew` y `deactivate` entregan la revisión de cierre: así la computadora se entera sin internet, y si sale de la computadora anterior de una transferencia pendiente, la confirma.
- Una copia que sigue desconectada no se puede borrar a distancia: sigue funcionando hasta su vencimiento, o indefinidamente si es perpetua. Es una limitación aceptada del contrato.

Los ejemplos de las tres acciones están en `contracts/v1/fixtures/requests/`.

---

## 6. Estados y cupos

**Estados (§6).** Después de verificar, el estado sale así:

1. Sin ninguna licencia válida aceptada: **Sin activar**, o **Sin activar con datos** si la base tiene operaciones (por ejemplo, un respaldo restaurado).
2. Con `license_status: revoked`: **Revocada**.
3. Si no, el estado temporal de la §5.6: **Vigente**, **Tolerancia** o **Vencida**.

Un archivo alterado, de una llave no confiable o de otra instalación se ignora y se sigue usando la última licencia válida. La tabla de operaciones de la §6 se aplica **en el backend**, en cada operación protegida; las terminales solo muestran el estado. Las reglas firmes no tienen excepciones: ningún estado borra datos, impide cobrar una cuenta ya abierta ni bloquea reportes o historial.

**Cupos (§7).** Cuentan lo que hace cada terminal, no qué tipo de dispositivo es:

| Cupo | Lo ocupa | Se libera |
| --- | --- | --- |
| `cash_registers` | Un turno de caja abierto en una terminal | Al cerrar el turno (también desde otra computadora) |
| `order_takers` | Una sesión que captura pedidos sin turno de caja | Al cerrar la sesión, salir de ese modo o tras un periodo sin actividad |
| `kitchen_displays` | Una terminal en modo Cocina | Al salir del modo o cerrar la sesión |

- La asignación debe ser atómica.
- Una licencia nueva con menos cupos no interrumpe nada: solo impide ocupar cupos nuevos hasta quedar dentro del límite.
- El conteo actual de AD-EL POS (`sessions-summary`, por tipo de dispositivo) tiene que cambiar a este modelo.

**Lo que el contrato deja a la etapa de integración:** el periodo sin actividad que libera un cupo de comandero (§7) y las protecciones contra cambios de reloj y copias de la base, además de la última hora confiable (§5.6).

---

## 7. Qué probar contra licensing.sicrat.com

`licensing.sicrat.com` firma con el entorno `test`: solo lo usan versiones de **desarrollo** que confían en la llave exportada (sección 3). Para empezar, el propietario registra en el panel un cliente de prueba y emite una licencia de cada tipo (perpetua, suscripción mensual y prueba).

| # | Prueba | Resultado esperado |
| --- | --- | --- |
| 1 | Activar en línea por primera vez | Revisión 1 con las fechas que calculó el servidor; la computadora aparece en el detalle de la licencia, sección **Computadoras y transferencias** |
| 2 | Activar otra vez desde la misma instalación | La misma revisión, sin otra activación |
| 3 | Activar desde otra instalación (otra identidad) | `409 ACTIVATION_LIMIT` con `transfer_available: true` |
| 4 | Transferir a la otra instalación | La nueva queda activa con las mismas fechas, cupos, plan y módulos; la transferencia queda pendiente |
| 5 | Transferir otra vez antes de que la anterior consulte | `409 TRANSFER_LIMIT` y una alerta en el panel |
| 6 | Consultar desde la computadora anterior | Su cierre `revoked` / `transferred`; la transferencia queda confirmada |
| 7 | Renovar, convertir o cambiar de plan en el panel y consultar | Una revisión mayor, que AD-EL POS acepta |
| 8 | Revocar en el panel y consultar | El cierre `revoked`; estado «Revocada» |
| 9 | Liberar y activar en otra computadora con el mismo código | El cierre `deactivated`; la otra activa sin soporte |
| 10 | Sin internet: `.licreq` de `activate`, `renew` y `deactivate` | El propietario los aprueba en **Sin internet**; AD-EL POS importa cada `.lic` |
| 11 | Un código inexistente o mal escrito | `404 LICENSE_NOT_FOUND`, igual en todos los casos |
| 12 | Reintentar una operación con el mismo cuerpo, y luego con el mismo `request_id` y otro contenido | La misma respuesta; después, `409 REQUEST_CONFLICT` |
| 13 | Importar un `.lic` alterado, de otra instalación o con una revisión menor | Se ignora; sigue la última licencia válida |
| 14 | Atrasar el reloj de la computadora | La licencia no se prolonga |

- La tolerancia y el vencimiento no se esperan en tiempo real: se prueban con los casos `temporal_states` de `vectors.json`, que tienen los límites exactos.
- Una versión de producción **nunca** apunta a `licensing.sicrat.com` ni confía en sus llaves.
- Ante un resultado distinto del esperado, primero se comprueba contra el contrato y el verificador de referencia. Si el servidor de licencias no cumple el contrato, es un defecto del servidor; si el contrato no alcanza, es una propuesta para la versión 1.1.
