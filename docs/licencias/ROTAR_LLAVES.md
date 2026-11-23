# Rotar las llaves de firma de las licencias

Procedimiento de la §3 de la [guía de integración](INTEGRACION_ADEL_POS.md) para cuando el propietario cambia la llave
con que el servidor de licencias firma. Manda el [contrato](CONTRATO_LICENCIAS_ADEL_POS.md) (§3 y §10).

## Qué guarda AD-EL POS

- Solo **llaves públicas**, cada una con su `kid`. Las privadas nunca salen del servidor de licencias.
- Cada versión confía **solo en las llaves de su entorno**, que se fija al compilar:
  - **desarrollo** (sin etiqueta): las del entorno `test`, en `config/licencias/llaves-test.json`;
  - **producción** (`-tags adelpos_production`): las del entorno `production`, en `config/licencias/llaves-production.json`.
    Ese archivo todavía no existe, y por eso la compilación de producción no compila.
- `./scripts/dev.sh sync` (`scripts/sync-metadata.mjs`) copia cada `config/licencias/llaves-<entorno>.json` a
  `backend/internal/licensing/`, que lo embebe en el servidor Go. `sync-check` falla si la copia no coincide.
- Las llaves de los archivos de prueba del contrato (`vectors.json`) solo las usan las pruebas automáticas. Ninguna
  versión las embebe.
- Se guardan **todas** las llaves del entorno: las preparadas, la activa y las anteriores. Así las licencias firmadas
  con una llave anterior siguen valiendo, por ejemplo una perpetua.

## Orden obligatorio

La llave nueva se activa en el servidor de licencias **solo después** de que AD-EL POS confíe en ella.

1. **El propietario genera la llave nueva** en el servidor de licencias, sin activarla todavía.
2. **La exporta**, en el VPS, con la orden de la §3 de la guía (`bin/signing-key.php --action=export`). El resultado
   es público: `environment` y la lista `keys`, cada una con `kid`, `environment` y `public_key`.
3. **En AD-EL POS**, en una rama nueva:
   1. Reemplaza `config/licencias/llaves-test.json` por la exportación completa. Para producción es
      `llaves-production.json`.
      - **Nunca quites una llave anterior.**
      - Todas deben ser del entorno del archivo: el servidor rechaza al arrancar un archivo con una llave de otro
        entorno, un `kid` repetido o una llave que no sea Ed25519 de 32 bytes en base64url.
   2. Ejecuta `./scripts/dev.sh sync`.
   3. Agrega la llave nueva a la lista esperada de `TestCompiledTrustIsTheOwnersTestKeyList`
      (`backend/internal/licensing/trust_test.go`). La prueba fija la lista exacta para que ninguna llave entre o
      salga por accidente.
   4. Comprueba: `sync-check`, `fmt-check` y las pruebas de Go (`go-unit`).
4. **Publica e instala** la versión nueva en todas las computadoras de ese entorno: el `.pkg` de macOS y el instalador
   de Windows.
5. **Solo entonces**, el propietario activa la llave nueva en el servidor de licencias (`--trust-confirmed`).

## Si se activa antes de tiempo

Una computadora con una versión que todavía no conoce la llave recibe licencias con un `kid` desconocido.

- Las rechaza (`unknown_kid`) y **sigue con la licencia que ya tenía**: un archivo de una llave no confiable se ignora
  (contrato §6 y §10).
- Al instalar la versión nueva, «Buscar actualización» o el `.lic` la aceptan.

## Retirar una llave

En V1 no se retira ninguna llave mientras haya licencias firmadas con ella: una perpetua puede durar para siempre. Si
una llave privada se compromete, el propietario decide qué hacer. Es una propuesta para la V1.1 del contrato.

## Producción

- El mismo procedimiento, con `config/licencias/llaves-production.json` y la compilación con `-tags adelpos_production`.
- El archivo de producción nunca lleva llaves `test` ni las de los archivos de prueba.
- Una versión de producción nunca apunta al servidor de licencias de pruebas.
- Lo demás que falta para producción (su servidor de licencias, su dirección y la llave de soporte) va en
  `docs/licencias/PASO_A_PRODUCCION.md` (Fase H).
