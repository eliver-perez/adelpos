# Desbloqueo de soporte sin internet

Procedimiento del §6.4 de la especificación de la etapa L1 ([etapas/ETAPA_L1_LICENCIAS_v2.md](../etapas/ETAPA_L1_LICENCIAS_v2.md))
y de la decisión L1-D19 del avance. Lo usa el **propietario de AD-EL POS** para quitar el bloqueo «Sistema detenido por la
fecha y hora de la computadora» de un restaurante que no tiene internet.

## Cuándo se usa

La computadora principal se detiene cuando su reloj queda más de **10 minutos antes de la última actividad registrada**
(la «última hora confiable», §6.1). Se bloquea todo lo que registra algo con fecha, también cobrar; reportes, historial,
respaldos y la pantalla de la licencia siguen. El bloqueo se quita solo en cuanto el reloj se corrige.

El caso difícil es el de la §6.3: la fecha saltó al futuro, se registró actividad con esa fecha y después la hora se
corrigió. Ahora la hora real queda «antes» de la última actividad y corregir el reloj no basta. Hay dos salidas:

1. **«Comprobar la hora con internet»** (`POST license/time-check`): si la hora del servidor de licencias coincide con el
   reloj, la última hora confiable baja a esa hora. Necesita internet.
   - Con una licencia activada en línea, la hora sale de la consulta de esa activación. Si el servidor de licencias la
     rechaza (por ejemplo, una activación que ya no conoce), la persona ve el texto de su código y usa el desbloqueo de
     soporte (L1-D53).
2. **«Desbloqueo de soporte»**, sin internet: este procedimiento.

Como el estado de la licencia se calcula con la última hora confiable, un salto al futuro también puede mostrar una
suscripción en Tolerancia o Vencida. Al bajar la hora, por cualquiera de las dos salidas, Go vuelve a calcularlo, y la
pantalla de la hora lo dice (L1-D58).

## Paso a paso

1. **En la computadora principal**, en la pantalla de la hora, «Desbloqueo de soporte» muestra un **código de
   solicitud** (`POST license/support-request`): 24 caracteres en seis grupos de cuatro, por ejemplo
   `3RF0-HBRQ-2082-0C20-A1SE-E1SR`. Mientras no se use y no pasen 24 horas, la ruta entrega el mismo; con
   `{"renew": true}` crea uno nuevo, que reemplaza al anterior.
2. **El restaurante lo dicta** por teléfono o lo envía por mensaje. Al leerlo se aceptan minúsculas, espacios, O en lugar
   de 0 e I o L en lugar de 1.
3. **El propietario de AD-EL POS**, en su computadora, con la llave privada de soporte:

   ```bash
   tools/soporte-desbloqueo/soporte-desbloqueo responder 3RF0-HBRQ-2082-0C20-A1SE-E1SR
   ```

   - La nueva última hora confiable es, por omisión, **la hora actual de su computadora**. Con
     `--hora AAAA-MM-DDTHH:MM:SSZ` se elige otra (en UTC).
   - La salida muestra la última actividad que traía la solicitud y la **respuesta**, en una sola línea. Con
     `--salida respuesta.desbloqueo` también la guarda como archivo.

4. **El restaurante pega la respuesta** en «Desbloqueo de soporte» o **importa el archivo** `.desbloqueo`
   (`POST license/support-unlock` con `{"response": "…", "source": "text"}` o `"file"`; el escritorio lee el archivo y
   envía su texto). Los espacios y saltos de línea que agregue un mensaje se ignoran.
5. **Al aceptarla**, la última hora confiable pasa a la de la respuesta (también hacia atrás). Si el reloj de la
   computadora está bien, el bloqueo se quita en ese momento; si sigue mal, la pantalla sigue pidiendo corregirlo.

Las operaciones registradas con fecha futura **no se modifican**. «Comprobar la hora con internet» las lista en la
auditoría (`license.clock_corrected`); con el desbloqueo de soporte, el propietario las revisa en el Registro de
movimientos.

## Qué se acepta

Una respuesta se acepta **una sola vez**, **solo en esa instalación** y **dentro de las 24 horas** de la solicitud. Si
no, `400 SUPPORT_UNLOCK_INVALID` con `details.reason`:

| `reason`             | Qué pasó                                                              | Qué hacer                        |
| -------------------- | --------------------------------------------------------------------- | -------------------------------- |
| `used`               | La respuesta ya se usó                                                | Pedir un código nuevo            |
| `other_installation` | La respuesta es para otra computadora                                 | Revisar a quién se envió         |
| `expired`            | Pasaron 24 horas desde la solicitud, o el reloj se corrigió en medio  | Pedir un código nuevo            |
| `altered`            | La firma o el contenido no son válidos (copiada incompleta o editada) | Volver a copiarla completa       |
| `not_requested`      | No corresponde al código pendiente (se pidió otro después)            | Responder el código más reciente |

- **Las 24 horas se miden con el reloj local** respecto al de la solicitud, porque es un reloj que se sabe incorrecto.
  Si el reloj se corrige entre la solicitud y la respuesta (más de 10 minutos hacia atrás, o hasta pasar las 24 horas),
  se pide una solicitud nueva.
- Cada solicitud, cada desbloqueo y cada intento rechazado queda en la auditoría: `license.support_requested`,
  `license.support_unlocked` (con la última hora confiable anterior y la nueva) y `license.support_unlock_rejected`
  (con el motivo).
- Solo se acepta en la computadora principal (escucha local) y con una sesión de cualquier usuario, sin permiso
  especial: quien esté en la caja debe poder resolverlo. La seguridad está en la firma. Desde otra terminal responde
  `409 LICENSE_LOCAL_ONLY`.
- Solo en la instalación original: una carpeta de datos copiada a otra computadora tiene la misma identidad y la misma
  solicitud pendiente, así que no pide ni acepta un desbloqueo (`409 LICENSE_INSTALLATION_COPIED`) hasta crear una
  identidad nueva. Tampoco sin la huella del equipo comprobada.
- El desbloqueo se audita antes de aplicarse: si la auditoría no se puede guardar, no cambia nada y la respuesta se puede
  volver a pegar. Baja las dos copias de la última hora confiable: `estado.json` y la de la base
  (`app.license_trusted_time`).

## Formatos

### Código de solicitud

120 bits en base32 de Crockford (`0-9` y `A-Z` sin I, L, O ni U), 24 caracteres:

| Bits | Campo                                                                                                  |
| ---- | ------------------------------------------------------------------------------------------------------ |
| 4    | Versión (1)                                                                                            |
| 40   | Etiqueta de la instalación: los primeros 5 bytes de `SHA-256("ADELPOS-SOPORTE-V1\n" + server_id)`      |
| 40   | Número aleatorio de un solo uso                                                                        |
| 24   | Última hora confiable al pedirlo, en minutos desde 2026-01-01 UTC (unos 31 años)                       |
| 12   | Verificación: los primeros 12 bits de SHA-256 de los 15 bytes con este campo en cero (error al dictar) |

La solicitud pendiente (código, número, reloj local al pedirla) se guarda en `identity/licencia/estado.json`, que nunca
viaja en un respaldo. Se recuerdan los últimos 16 números usados, para responder «ya se usó».

La respuesta lleva la etiqueta de 40 bits y no el `server_id` completo (L1-D19): así el código cabe en 24 caracteres.
Junto con el número de un solo uso, ata la respuesta a esa instalación y a esa solicitud.

### Respuesta

JWS compacto, Ed25519, con el encabezado exacto `{"alg":"EdDSA","kid":"<kid>","typ":"adelpos-unlock+jws"}` y este
contenido (campos desconocidos: se rechaza):

```json
{
  "schema_version": "1.0",
  "product_id": "adel_pos",
  "environment": "development",
  "installation_tag": "e1e08af171",
  "request_nonce": "0102030405",
  "request_code": "3RF0-HBRQ-2082-0C20-A1SE-E1SR",
  "new_trusted_time": "2026-11-23T20:56:13Z",
  "issued_at": "2026-11-23T20:56:13Z"
}
```

`environment` es el entorno de compilación de la versión que la recibe (`development` o `production`). Mide unos 600
caracteres. El código de la verificación está en `backend/internal/licensing/unlock.go` y lo comparten el servidor y la
herramienta.

## Llaves de soporte

Una por entorno de compilación. La pública se incluye al compilar; la privada **nunca** va en el repositorio, en un
respaldo, en el instalador ni en la salida de una orden. Quien la tenga puede desbloquear la hora de cualquier
instalación de ese entorno.

- **Desarrollo** (la de esta etapa): la privada está en `~/.adel-pos-dev/soporte/soporte-development.key` (PKCS#8,
  0600, carpeta 0700) de la computadora del propietario, con su pública junto a ella. La creó
  `soporte-desbloqueo generar-llave --publica config/licencias/soporte-development.pub.json`. La pública vive en
  `config/licencias/soporte-development.pub.json` y `scripts/sync-metadata.mjs` la copia a
  `backend/internal/licensing/`, donde `support_development.go` la embebe (`sync-check` lo vigila).
- **Producción**: todavía falta. `support_production.go` embebe `soporte-production.pub.json`, que no existe, así que
  **una compilación de producción no compila**, igual que sin `llaves-production.json` (lo comprueba
  `TestProductionBuildFailsWithoutProductionKeys`). En el paso a producción:
  1. en la computadora del propietario, `soporte-desbloqueo generar-llave --entorno production --publica
config/licencias/soporte-production.pub.json`;
  2. `node scripts/sync-metadata.mjs` y la compilación con la etiqueta `adelpos_production`;
  3. un respaldo de la llave privada fuera de línea (nunca en el repositorio).
- **Rotar una llave**: `generar-llave` nunca reemplaza una llave existente. Se aparta la anterior a mano (por ejemplo,
  `soporte-development.key.2026`), se genera la nueva con `--publica` (que **agrega** la pública al archivo y conserva
  las anteriores) y se publica una versión. Las respuestas firmadas con la llave anterior siguen valiendo mientras su
  pública esté en la versión; se quita en una versión posterior.

## Propuesta para la V1.1 del contrato

**Pasar este desbloqueo al panel del servidor de licencias**: que el propietario procese el código de solicitud desde el
panel, con la llave de soporte guardada en el servidor de licencias y la auditoría allá, en lugar de una herramienta de
línea de órdenes con una llave en su computadora. El formato del código y de la respuesta puede quedarse igual.
