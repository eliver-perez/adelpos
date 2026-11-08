# Verificación · Etapa 4

Fecha: 7 de noviembre de 2026, America/Monterrey. macOS Apple Silicon. Versión 0.4.0, esquema 4. Evidencia anterior: [etapa 1](VERIFICATION_STAGE1.md), [etapa 2](VERIFICATION_STAGE2.md), [etapa 3](VERIFICATION_STAGE3.md).

## Comprobaciones ejecutadas

- `./scripts/dev.sh test`: aprobado. Prettier, gofmt, Rustfmt, sincronización de metadatos, Go con race detector y vet, frontend, TypeScript estricto, Vite y Rust con lockfile.
- Frontend: **29 pruebas aprobadas en siete archivos**. Conserva las 19 anteriores. Añade conservación de campos/aria/foco, revelación de error nativo y error asincrónico al cerrar un acordeón, teclado alfanumérico/numérico/desactivado, edición compatible con React y teclado físico, exclusión de contraseñas, descarte de respuesta tardía, reintento con idéntico UUID/cuerpo y nueva operación después de un conflicto conocido.
- Rust: **8 aprobadas, 1 opt-in omitida**. Añade métodos/rutas cerradas de restaurante y rechazo de acciones de entrega/cobro inexistentes; conserva protección de secretos y de espera del almacén seguro. El roundtrip opt-in aislado del Keychain no se repitió; el perfil real sí retomó actividad autenticada.
- `./scripts/dev.sh test-integration`: aprobado. Cuatro paquetes con PostgreSQL real, race detector y bases temporales independientes: migrador, administración, catálogo y operación. Todas las bases se eliminaron al terminar. No se sembraron fixtures en adel_pos.
- `./scripts/dev.sh test-browser`: aprobado, un recorrido en Chromium y otro en WebKit a **1280×720**. Cada navegador usa otra base temporal, Go real y el frontend compilado de producción servido por Vite preview. Se sustituye únicamente IPC en Node; los tokens permanecen fuera del navegador. El producto continúa requiriendo Tauri.
- `./scripts/dev.sh build-macos`: aprobado. Bundle ARM64 `desktop/src-tauri/target/release/bundle/macos/AD-EL POS.app`, versión 0.4.0, aproximadamente 9.32 MiB.

## Aceptación del restaurante en PostgreSQL

La suite `TestRestaurantAcceptance` ejecutó los siguientes escenarios con autenticación HTTP y transacciones reales:

- Crear áreas/mesas, guardar distribución, releer coordenadas y rechazar revisión antigua; ocupación derivada de cuentas reales.
- Dos cuentas en la misma mesa, captura de borrador sin confirmar y cotización de selección sin guardar que no cambia revisión ni persiste partidas.
- Confirmación explícita con huella vigente; rechazo de huella incorrecta, repetición idempotente y conflicto al reutilizar UUID con otro cuerpo.
- Cambiar catálogo después de confirmar conserva importes anteriores; editar observaciones conserva precio histórico.
- División parcial, transferencia, combinación y traslado entre mesas conservan unidades, notas e importes; cancelación parcial conserva historia; anular cuenta vacía permite desactivar la mesa histórica.
- Captura real de presentación e ingrediente adicional; retirar todos los complementos, reemplazarlos todos y pedir doble papas. Se mantiene piso de $150 y los cambios explícitos producen $152.50, $165 o $180 según escenario.
- Combo confirmado con tramos **$90/$100/$140**; modificación de guarnición a $155 y nuevo producto de $40 en otro lote producen $195 sin alterar el lote anterior.
- Promoción 2×1 con dos/tres unidades y distribución parcial de centavos sin pérdida. Lista alternativa histórica y rechazo si cambia el precio entre revisión y confirmación.
- Para llevar sin mesa, origen Ventanilla, programación local preservada, confirmación, edición, búsqueda por folio/cliente/teléfono, orden por antigüedad y umbrales de atraso. Preparación, pago y entrega permanecen independientes; entregar responde 404.
- Dos sesiones/dispositivos compiten por el mismo borrador o confirmación: una operación gana y la otra recibe 409, sin doble pedido.
- Mesero con permisos efectivos edita y cancela sin supervisor. Operaciones sin permiso se rechazan, una revocación surte efecto en la siguiente petición y una transferencia inválida revierte completamente. Auditoría y eventos quedan persistidos.

Las pruebas unitarias de centavos comprueban suma exacta para 1, 2, 3, 7 y 1000 unidades, importes desde cero/centavos hasta valores grandes y asignación estable del residuo. Las suites anteriores verifican adicionalmente equivalencia, porciones, personalización de hijos de combo, exclusiones/vigencias/promociones y seguridad de catálogo.

## Interfaz y revisión visual

El recorrido automatizado de ambos motores ejecutó inicio de sesión, creación de área/mesa, movimiento con flecha y persistencia de posición, apertura de cuenta, captura de dos productos, borrador, confirmación, modificación de preparación, motivo e historial anterior/posterior, división de una unidad y creación de pedido programado para Ventanilla.

Comprobó operación de acordeones con Enter/Espacio, conservación de fecha al cerrar/abrir, ausencia de `details/summary` en la vista anterior de previsualización, temas claro/oscuro, documento sin desbordamiento horizontal y botón de confirmar dentro de la altura disponible. Con la preferencia de terminal real se activó teclado alfanumérico, se escribió ñ y se verificó que campo enfocado y acción de guardar quedan por encima del teclado mediante desplazamiento interno. No se registraron errores JavaScript durante el recorrido.

Se capturaron y revisaron imágenes del tablero en claro/oscuro y del teclado. Los artefactos privados están en `.local/browser-artifacts/chromium/` y `.local/browser-artifacts/webkit/`. Esta es una aceptación del frontend con backend real; no es automatización de todos los controles de la ventana nativa ni una prueba de segunda máquina LAN.

## Respaldo y migración real

Se generó `backups/adel_pos-20261108T031917Z-bc778c.dump`, se verificó su catálogo y se restauró completamente con `pg_restore --exit-on-error` en una base temporal exclusiva. **Las 35 tablas existentes coincidieron en conteos**. La base de ensayo fue eliminada. El respaldo corresponde al 7 de noviembre local; el nombre usa UTC.

Se detuvo Go, se aplicó 0004 con `applied:1` y se repitió con `applied:0`. Las migraciones 0001–0003 permanecen sin cambios. El nuevo esquema crea once tablas de operación, configura umbrales y añade veintiún permisos al administrador, sin reinicializar usuarios ni catálogo.

La lectura posterior confirmó esquema 4, **ADEL POS**, **America/Monterrey**, **MXN**, dos usuarios y una terminal, con cero categorías, productos, áreas, mesas y cuentas ficticias. Readiness devolvió 200 con PostgreSQL disponible y esquema compatible; versión devolvió 0.4.0/API v1/esquema 4. Evidencia privada: `.local/stage4-restore-verification.json` y `.local/stage4-installation-verification.json`.

## Aplicación nativa y procesos

Se abrió el `.app` 0.4.0 y el perfil existente retomó actividad autenticada, comprobada mediante `last_seen_at` sin leer ni imprimir tokens. No se modificaron credenciales, mecanismo de sa ni preferencia comercial de zona horaria.

La prueba activa explícitamente la ventana antes de medir consultas: WebKit puede suspender temporizadores cuando está en segundo plano. Un reintento final detectó esta condición; al activar la ventana retomó la sesión, y el ejecutor incorpora esa activación.

`python3 tests/runtime_smoke.py` pasó: detener PostgreSQL conserva Go y live=200/ready=503; reiniciarlo recupera readiness sin reiniciar Go. Se verificó server-only en proceso temporal, cierre normal de Tauri conservando Go, reapertura contra el mismo servidor, caída de Go conservando Tauri y recuperación de consultas en la misma ventana.

`python3 tests/runtime_smoke.py --terminal-only` pasó con el mismo bundle y el servidor existente. Se retiró el proceso temporal y se reabrió el perfil normal. Los servicios propios y la aplicación quedaron activos. Evidencia privada en `.local/runtime-verification.json`, `.local/terminal-mode-verification.json`, `.local/stage4-runtime.log` y `.local/stage4-terminal.log`.

## Límites reales

No se han probado carga comercial elevada, catálogo masivo, otra máquina LAN, corte de energía, Windows, Mac Intel ni Android. Las escrituras se serializan por lock operativo. El diario durable no publica eventos ni tiene acuse de cocina; `/events` continúa en 501. No existen impresión, KDS, inventario, cobros ni entrega habilitada.

La selección no guardada y el UUID de reintento permanecen en memoria; el borrador guardado sí sobrevive al cierre. Cuentas/historiales excepcionalmente grandes pueden superar 2 MiB y requieren paginación de detalle. Snapshots por lote, retención, respaldo periódico, firma/notarización, servicio comercial y licencias requieren sus etapas propias. Ver límites y políticas en [RESTAURANT_OPERATIONS.md](RESTAURANT_OPERATIONS.md).

## Repetir

```bash
./scripts/dev.sh test
./scripts/dev.sh test-integration
./scripts/dev.sh test-browser
./scripts/dev.sh build-macos
./scripts/dev.sh db-start
./scripts/dev.sh server-start
./scripts/dev.sh open-app
```

Las pruebas opt-in de procesos deben ejecutarse secuencialmente y sin trabajo real ni pruebas de bases temporales simultáneas. Las claves y archivos privados de `.local` nunca se incluyen en commits ni en esta documentación.
