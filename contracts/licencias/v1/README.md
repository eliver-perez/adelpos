# Archivos del contrato de licencias V1.0

Esta carpeta es la forma exacta, legible por una máquina, del [contrato de licencias](../../docs/CONTRATO_LICENCIAS_ADEL_POS.md) entre el servidor de licencias y AD-EL POS, en su **versión 1.0 definitiva**. Si un archivo de aquí y el contrato difieren, **manda el contrato**.

| Archivo | Qué es |
| --- | --- |
| [`license.schema.json`](license.schema.json) | Contenido de la licencia firmada (sección 4), en JSON Schema 2020-12 |
| [`openapi.yaml`](openapi.yaml) | API en línea (sección 8), en OpenAPI 3.1: rutas, peticiones, respuestas, errores y ejemplos |
| [`fixtures/`](fixtures/) | Archivos de prueba compartidos (sección 13) |
| [`generate-fixtures.php`](generate-fixtures.php) | Generador de los archivos de prueba |
| [`verify/`](verify/) | Verificador de referencia en Go, base de la integración de AD-EL POS |

## Llaves de prueba

Todas las llaves de `fixtures/` salen de **semillas públicas**. Sirven solo para pruebas.

- El servidor de licencias las rechaza en producción (`SigningKeys::FIXTURE_SEEDS`).
- Ninguna versión de producción de AD-EL POS debe confiar en ellas.
- `adelpos-prod-fixture-a` hace de llave de producción únicamente en los casos negativos. Sigue siendo pública.

## Archivos de prueba

`fixtures/vectors.json` lo contiene todo, con el resultado esperado de cada caso:

| Sección | Contenido |
| --- | --- |
| `signing_keys`, `installations` | Llaves de firma por entorno y dos instalaciones, con su semilla pública: `sha256("ADELPOS LICENSE PUBLIC TEST ONLY V1 " + name)` |
| `licenses` | 20 licencias válidas, cada una con la instalación que la verifica (`installation`): perpetua con 0, 6 y 12 meses de actualizaciones, suscripción mensual y anual, prueba de 15 días, personalizada, las tres revisiones de cierre (`deactivated`, `transferred`, `revoked`), una firmada con otra llave de confianza y, del ciclo comercial, una suscripción renovada (revisión mayor, misma activación), una prueba convertida a suscripción y otra a perpetua, una perpetua con mantenimiento, una subida de plan y una reducción aplicada por la tarea programada; y, de la transferencia por autoservicio, la revisión que tenía la computadora anterior, su cierre `transferred` y la revisión `active` de la computadora nueva, con las mismas fechas |
| `negative_licenses` | 17 licencias que deben rechazarse, con su motivo: firma o contenido alterados, `kid` desconocido, encabezado distinto, licencia de prueba en una instalación de producción, entorno que no coincide con la llave, otra instalación, otro producto, campo faltante, `schema_version` 2.0, tipos incorrectos y más |
| `revision_cases` | 10 casos de la regla «la revisión nunca retrocede» (sección 10), cada uno con la instalación que lo verifica: entre ellos la renovación que llega como revisión mayor de la misma activación y, en una transferencia, la computadora nueva que acepta su primera revisión de una licencia con revisiones previas, la anterior que acepta su cierre y la revisión de la nueva rechazada en la anterior |
| `dates` | 40 casos de fechas con su resultado: meses de calendario con día ancla, años bisiestos, cambios de horario, 15 días de calendario y el primer periodo de cada tipo de licencia (24); renovación (`renewal`, 9: los cuatro ejemplos de §5.3, un microsegundo antes del límite de la tolerancia, día ancla 31 encadenado, cambio de duración y cambio de horario); conversión de una prueba (`conversion`, 4); y mantenimiento antes y después de que terminen las actualizaciones (`maintenance`, 3) |
| `credits` | 11 casos de la bonificación de un cambio de plan. Cada caso trae el instante del pago y los periodos pagados que todavía no terminan, cada uno con lo cobrado, la bonificación que se aplicó en ese pago, su valor (la suma de los dos), su inicio y su fin; y el resultado de cada periodo (segundos no consumidos, segundos del periodo y su parte al centavo) y el total. Incluye la renovación anticipada con subida de plan, la segunda subida sobre un periodo pagado con bonificación y la combinación de los dos. En centavos y en texto MXN. Útiles si algún día AD-EL POS muestra montos |
| `temporal_states` | 13 casos del estado temporal (sección 5.6): límites exactos de vigente, tolerancia y vencida, y reloj atrasado |
| `proofs` | Pruebas de posesión de las cuatro acciones, `transfer` incluida |
| `offline_requests` | Solicitudes `.licreq` de las tres acciones |

Además, cada licencia está en `fixtures/licenses/<nombre>.lic` y cada solicitud en `fixtures/requests/<acción>.licreq`, tal como viajan.

Los casos del ciclo comercial se agregaron en la etapa 3 y los de la transferencia por autoservicio en la etapa 4, con el servidor ya implementando esas reglas. Los valores esperados de fechas y bonificaciones están calculados a mano.

```sh
php contracts/v1/generate-fixtures.php           # vuelve a escribir fixtures/
php contracts/v1/generate-fixtures.php --check   # comprueba que nada cambió
```

El generador no usa el código del servidor: arma cada contenido a mano, con sus valores esperados. Las pruebas (`tests/Unit/ContractFilesTest.php`) comparan después el servidor contra estos archivos: firma los mismos bytes, calcula las mismas fechas y valida igual que el esquema.

## Verificador de referencia

`verify/` es un módulo de Go sin dependencias externas. Aplica lo que debe aplicar el servidor Go del restaurante antes de aceptar una licencia (sección 10) y para calcular su estado (sección 5.6):

1. Encabezado exacto `{"alg":"EdDSA","kid":"<kid>","typ":"adelpos-lic+jws"}`.
2. `kid` de confianza **para el entorno de esa versión**: una versión de producción no incluye llaves de prueba.
3. Firma Ed25519.
4. Contenido de la sección 4: campos obligatorios, tipos y formatos. `product_id` es `adel_pos`. Los campos desconocidos se ignoran. Una `schema_version` que no empieza con `1.` es incompatible.
5. `environment` igual al entorno de la llave que firmó.
6. `server_id` e `installation_public_key` exactamente los de la instalación.
7. `license_revision` mayor que la más alta ya aceptada para ese `license_id`. Una revisión igual solo se acepta si es idéntica byte por byte.
8. Estado temporal con `ahora_efectivo = máximo(reloj local, última hora confiable)`.

| Archivo | Contenido |
| --- | --- |
| `verify/license.go` | `Verifier.Verify`: los pasos 1 a 7 |
| `verify/state.go` | `TemporalState` y `EffectiveNow`: el paso 8 |
| `verify/protocol.go` | Mensaje y firma de la prueba de posesión, y verificación de una solicitud `.licreq` |
| `verify/main.go` | Ejecuta todos los casos de `fixtures/vectors.json` y compara cada resultado con el esperado |

```sh
go run -C contracts/v1/verify . ../fixtures/vectors.json
```

Las fechas (`dates`) y las bonificaciones (`credits`) no las ejecuta el verificador: **las calcula únicamente el servidor de licencias** (sección 5). Las comprueba la suite de PHP contra el motor de fechas y contra `Domain\Credit`. El verificador sí acepta las licencias nuevas del ciclo comercial y de la transferencia, y aplica sus casos de revisión con la instalación de cada uno.

No es un validador de producción. Es código de referencia para la etapa de integración de AD-EL POS: se puede copiar y adaptar. Quedan fuera las partes que el contrato deja a AD-EL POS, como guardar de forma persistente la licencia, la revisión aceptada y la última hora confiable.

## Transferencia por autoservicio, desde AD-EL POS

La licencia vale para una sola computadora principal a la vez (sección 1). Cuando el restaurante cambia de computadora, AD-EL POS puede moverla sin soporte (sección 8.4):

1. **Activar en la computadora nueva.** Si la licencia ya está activa en otra, la respuesta es `409 ACTIVATION_LIMIT` con `details.transfer_available`:
   - `true`: la transferencia por autoservicio está disponible. AD-EL POS puede ofrecer «Transferir la licencia a esta computadora».
   - `false`: ya hay una transferencia pendiente de confirmar. AD-EL POS muestra «Comunícate con soporte».
2. **Transferir.** Un desafío con `action: "transfer"` y `activation_id: null`, y `POST /v1/activations/transfer` con los mismos campos que activar. La respuesta exitosa es igual a la de activar: la revisión `active` de la activación nueva, con **las mismas fechas** de la licencia. Transferir nunca reinicia, extiende ni cambia las fechas: una prueba o suscripción vencida sigue vencida.
3. **`409 TRANSFER_LIMIT`.** Ya hay una transferencia por autoservicio pendiente de confirmar. AD-EL POS muestra «Comunícate con soporte»; cada intento suma a una alerta en el panel del propietario. Mientras la transferencia siga pendiente, volver a intentarlo da la misma respuesta.
4. **La computadora anterior conoce su cierre al consultar.** En su siguiente `refresh` (o `deactivate`) recibe su revisión de cierre `revoked` / `transferred`, que tiene un número de revisión mayor que el que ya aceptó, y deja de operar con la licencia. Esa consulta confirma la transferencia y el contador vuelve a 0. Sin internet, la confirma la aprobación de su `.licreq` de `renew` o `deactivate`.
5. **Si la computadora anterior nunca vuelve** (por ejemplo, se dañó), el propietario libera el contador desde el panel.

Para el verificador, la computadora nueva acepta su primera revisión de una licencia que ya tenía revisiones (no ha aceptado ninguna de ese `license_id`), y la anterior acepta su cierre porque es mayor que la que tenía. La revisión de la nueva nunca sirve en la anterior: es de otra instalación.

Mientras la computadora anterior no consulte, sigue funcionando con su última revisión: hasta su vencimiento, o para siempre si es perpetua. Es una limitación conocida del contrato (secciones 9.2 y 12).

## Relación con el código del servidor

| En el contrato | En el servidor | Prueba que los compara |
| --- | --- | --- |
| `license.schema.json` | `src/Domain/LicenseContent.php`, que valida todo contenido antes de firmarlo | Cada regla del validador está en el esquema, salvo las que un esquema no puede expresar: fechas cruzadas, calendario real, seis decimales exactos y el orden de los campos |
| Rutas de `openapi.yaml` | `ApiKernel::ROUTES` | Deben ser las mismas |
| Peticiones de `openapi.yaml` | `Protocol::REQUEST_FIELDS` | Mismos campos, en el mismo orden |
| Errores de `openapi.yaml` | `ApiProblem::STATUS` | Mismos códigos y HTTP |
| Respuestas de `openapi.yaml` | Respuestas reales de la API | Las pruebas de integración validan cada respuesta |
