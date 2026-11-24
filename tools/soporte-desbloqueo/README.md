# soporte-desbloqueo

Herramienta del **propietario de AD-EL POS** para el desbloqueo de soporte sin internet (§6.4 de la especificación de la
etapa L1; procedimiento completo en [docs/licencias/DESBLOQUEO_SOPORTE.md](../../docs/licencias/DESBLOQUEO_SOPORTE.md)).
Se usa cuando la computadora principal de un restaurante está en «Sistema detenido por la fecha y hora de la
computadora», su hora es correcta y no tiene internet para «Comprobar la hora con internet».

La orden vive en `backend/cmd/soporte-desbloqueo` y comparte el formato con el servidor (`internal/licensing/unlock.go`).
Esta envoltura la ejecuta con el Go del proyecto. **Los instaladores no la incluyen.**

## Órdenes

```bash
# Una sola vez por entorno: crea la llave de soporte en ~/.adel-pos-dev/soporte/ (carpeta 0700, llave 0600) y agrega
# su pública al archivo que se incluye al compilar. Nunca reemplaza una llave existente.
tools/soporte-desbloqueo/soporte-desbloqueo generar-llave --publica config/licencias/soporte-development.pub.json
node scripts/sync-metadata.mjs

# Para cada solicitud: el código que dicta el restaurante (24 caracteres, en seis grupos de cuatro).
tools/soporte-desbloqueo/soporte-desbloqueo responder XXXX-XXXX-XXXX-XXXX-XXXX-XXXX
tools/soporte-desbloqueo/soporte-desbloqueo responder XXXX-XXXX-XXXX-XXXX-XXXX-XXXX --salida respuesta.desbloqueo
tools/soporte-desbloqueo/soporte-desbloqueo responder XXXX-XXXX-XXXX-XXXX-XXXX-XXXX --hora 2026-11-23T18:40:00Z
```

- `responder` acepta el código en minúsculas, con espacios y con O en lugar de 0 o I y L en lugar de 1. Un carácter mal
  dictado casi siempre se detecta (la verificación es de 12 bits) y entonces no se firma nada.
- `--hora` es la nueva última hora confiable, en UTC; por omisión, la hora actual de esta computadora.
- La salida muestra la solicitud, la nueva hora y la **respuesta** en una sola línea, para pegarla por WhatsApp. Con
  `--salida`, además la guarda como archivo `.desbloqueo` para importarlo.
- Si la llave no corresponde a la pública incluida en esta versión, la orden lo dice y no entrega una respuesta que el
  restaurante no podría usar.
- `--entorno production` y `--carpeta DIR` sirven para la llave de producción, que se crea con el paso a producción.

## Seguridad

- La llave privada **nunca** va en el repositorio, en un respaldo del restaurante, en el instalador ni en la salida de
  una orden. Quien la tenga puede desbloquear la hora de cualquier instalación de ese entorno.
- El código y la respuesta no tienen secretos: la respuesta vale una sola vez, solo en esa instalación y dentro de las 24
  horas de la solicitud.
