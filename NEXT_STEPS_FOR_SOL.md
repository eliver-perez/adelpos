# Continuidad para Sol · Estado de la etapa 9A

## Estado actual

Versión 0.9.0 y esquema 17 en la rama `etapa-9a` (etapa 9A: ajustes pendientes de catálogo e inventarios, reportes, indicadores y exportaciones; [docs/REPORTS.md](docs/REPORTS.md) y [docs/ETAPA_9A_AVANCE.md](docs/ETAPA_9A_AVANCE.md)), con las migraciones `0015_catalog_price_and_precheck.sql`, `0016_inventory_suppliers_counter.sql` y `0017_reports.sql`. **La instalación real está en 0.9.0 y esquema 17**: las migraciones 0015–0017 se aplicaron el 10 de noviembre de 2026 con autorización del propietario, tras respaldo y ensayo de restauración, y el servidor y el `.app` 0.9.0 están instalados (registro en [docs/ETAPA_9A_AVANCE.md](docs/ETAPA_9A_AVANCE.md), «Instalación real»); las 0010–0014 se aplicaron antes, ese mismo día (0009 se aplicó el 9 de noviembre de 2026; etapa 7.1 en [docs/ETAPA_7_1_AVANCE.md](docs/ETAPA_7_1_AVANCE.md)). Etapas 1–3 conservadas: Go independiente, PostgreSQL exclusivo, Tauri, almacén seguro, configuración, usuarios/permisos/sesiones/terminales y catálogo decimal. La etapa 4 conserva áreas, mesas, cuentas, borradores, versiones y transferencias; la etapa 5 añade estaciones, rutas, liberaciones, preparación, documentos, cola/transportes de impresión y SSE autenticado. La etapa 6 añade el módulo `payments`: cobros totales, parciales, mixtos y por selección, efectivo y cambio, descuentos y cortesías con límite por rol, cargos generales, propinas, IVA incluido (16/8/0), devoluciones, correcciones, autorización puntual de supervisor, turno mínimo, precuentas, tickets y entrega de pedidos para llevar. La etapa 7 añade la administración de caja (arqueo y cierre ciego, cortes, documentos, cajón y tope) y la 7.1 ajustes de operación, mesas, caja y mensajes. La etapa 8 añade el paquete `inventory`: insumos, unidades y presentaciones, almacenes, existencias y libro de movimientos con costo promedio, recetas versionadas y subrecetas, porciones habituales, indicaciones rápidas con efecto, consumo asíncrono al liberar a cocina (función activable `inventory_control`), cancelaciones con día operativo, compras con recepciones, devoluciones y vínculo opcional con gastos de caja, ajustes, mermas, transferencias, conteos, alertas y sugerencias. La etapa 9A añade el precio de complementos configurable por producto, la vigencia de la precuenta por revisión, datos adicionales de proveedores, «Compras de mostrador», el código interno sugerido, las categorías de insumos y la navegación agrupada de Inventarios, y el módulo `reports`: 92 reportes de un registro cerrado en Go con una transacción de solo lectura propia, «Hora de inicio del día» (`internal/businessday`), reconocimiento inmutable de ventas al cierre, resumen ejecutivo, exportación PDF y Excel con el diálogo nativo y la sección Reportes. Tauri inicia fullscreen nativo y utiliza un diseño fluido.

Leer primero AGENTS.md, README.md, ARCHITECTURE.md, API_CONTRACTS.md, docs/CATALOG.md, docs/RESTAURANT_OPERATIONS.md, docs/KITCHEN_AND_PRINTING.md, docs/PAYMENTS_AND_REFUNDS.md, docs/CASH_SHIFTS_AND_RECONCILIATION.md, docs/INVENTORY_AND_PURCHASING.md, docs/REPORTS.md y docs/VERIFICATION.md. La verificación anterior está archivada por etapa. No reiniciar ni reconstruir el proyecto.

## Reglas permanentes

- Fuentes legibles, nombres descriptivos y comentarios de negocio en español. gofmt y Prettier obligatorios; revisar también lo que no expandan. Los artefactos generados se mantienen separados de los fuentes.
- Migraciones 0001–0017 inmutables (todas en la instalación real; las 0015–0017 de la etapa 9A se aplicaron el 10 de noviembre de 2026 con autorización, respaldo y ensayo de restauración). La próxima ampliación requiere 0018 y, sobre la instalación real, autorización expresa previa. Respaldo y ensayo real de restauración antes de modificar datos existentes; nunca borrar .local para resolver un problema.
- `config/product.json` es canónico. Ejecutar sync y mantener lockfiles. No actualizar dependencias por inferencia de una nueva etapa.
- admin/admin es la política inicial autorizada; no restablecer ni imponer cambio. sa permanece reservado al propietario mediante CLI local y no aparece en la administración normal.
- Tokens duraderos solo en el almacén seguro nativo; no imprimirlos, versionarlos ni transportarlos al WebView. El OnceLock de Keychain es asíncrono: un consentimiento pendiente no significa credenciales vacías.
- Toda operación protegida valida sesión, dispositivo y permisos vigentes en Go. Mantener loopback, CSP/capabilities limitadas y HTTPS/CA verificadas para LAN.
- Todo dinero usa numeric/decimal en Go y cadenas JSON. `catalog.Calculate` y `QuoteForOrder` son la única autoridad de precios y `payments` la única autoridad financiera; React no calcula importes. Mantener equivalencia por valor con piso base, extras explícitos y tramos incrementales.
- Confirmar guarda snapshot comercial. Editar observaciones o reducir receta idéntica conserva centavos; otros cambios requieren previsualización explícita. Transferir/dividir mueve los mismos UUID de unidad y no genera preparación ni consumo.
- Cada escritura operativa lleva UUID/revisión y guarda resultado, eventos y auditoría en la misma transacción. Conservar el UUID/cuerpo de una respuesta incierta y consultar estado actual después del reintento.
- Usar `Accordion` para secciones colapsables; conservar valores montados, aria, foco, revelación de errores y movimiento reducido. No reintroducir details/summary ni convertir cada bloque en acordeón.
- Mantener selección local ante actualización y conflicto. El borrador persiste. Las operaciones inciertas viven en el almacén global de la aplicación (`hooks/pendingOperations.tsx`) y sobreviven a la navegación, pero no al cierre del proceso.

## Continuidad de cocina

No reconstruir las etapas anteriores ni cambiar 0005 después de aplicarla. `operations` integra cocina dentro de la misma transacción y UUID. `kitchen_unit_releases` contiene una marca canónica por unidad comercial, base del consumo de inventario desde la etapa 8; tareas por estación/copias no son otros consumos. Borradores, cambios de mesa y divisiones no generan comandas. Modificaciones y cancelaciones mantienen identidades y documentos históricos. Una estación inactiva conserva cambios pendientes y no debe permitir aparentar preparación lista.

La cola guarda intentos y lease antes de enviar. El advisory lock de sesión por impresora cubre el transporte. Un intento interrumpido es incierto y exige resolución; no reenviar automáticamente. El respaldo debe estar configurado expresamente y solo se usa tras tres fallos confirmados consecutivos sin envío. Copias adicionales conservan presentación sin etiquetas y se bloquean si hay otra pendiente. `submitted` significa aceptación del transporte, nunca confirmación física de papel.

El planificador pertenece a Go; no trasladarlo a temporizadores de React. La política de cada programado es histórica. SSE es el equivalente elegido a WebSocket: comandos HTTP idempotentes y notificación unidireccional autenticada, outbox/replay/cursor/reset y Channel nativo. No pasar secretos al WebView ni sustituir SSE por una simulación local. Un futuro KDS puede usar consulta/acuse/estado con permisos y ámbitos de terminal; la aplicación independiente todavía no existe.

La instalación real no recibe estaciones, productos ni impresoras ficticias. La prueba física fue una sola comanda técnica a Printer_POS_80, aceptada por CUPS como 219; no se certificó salida de papel o corte. `TestControlledCUPSPrinter` es opt-in y las suites ordinarias no imprimen. No reutilizar esa cola real en pruebas automáticas sin intención explícita de hardware.

## Continuidad de cobros

No cambiar 0006 después de aplicarla. Los registros financieros son inmutables: pagos, ajustes y devoluciones se corrigen con registros compensatorios enlazados. Una unidad con pago o descuento manual vigente no se transfiere, divide, combina, cancela, edita ni anula; ninguna operación deja la cuenta con total menor que lo pagado. Toda escritura de `payments/` usa el lock 7820148305, `restaurant_operations` y auditoría en la misma transacción; la huella excluye la contraseña de autorización.

`cash_movements` ya registra cobros, devoluciones y reversos por turno, con efectivo recibido y cambio, y `GET payments/shifts/{id}/movements` informa totales por método y `expected_cash`. Una caja con turno abierto o en arqueo no se desactiva y su terminal no cambia de caja hasta el cierre. Una corrección directa solo procede mientras el turno original está abierto. Desde la etapa 7, `expected_cash` y `by_currency` solo se entregan con `cash.expected.read` y nunca durante el arqueo.

## Continuidad de caja

La etapa 7 amplía `cash_shifts` y `cash_movements` en `payments` (`cash_*.go`): apertura por total o denominaciones, ingresos, retiros y gastos autorizados, correcciones compensatorias, cortes parciales con snapshot, arqueo ciego persistente (`closing`) que pausa la caja, cierre con diferencias por moneda que no bloquean ni exigen liquidar cuentas, regularizaciones posteriores enlazadas, documentos de caja, cajón y tope. El esperado se calcula solo en `cash_expected.go` y su visibilidad la decide `cash_access.go`. No duplicar ese cálculo en otro módulo ni en React. Ver docs/CASH_SHIFTS_AND_RECONCILIATION.md.

## Continuidad de inventario

No reconstruir ni duplicar el inventario. El consumo nace solo de la liberación a cocina y es asíncrono: la liberación inserta una solicitud por unidad (sin el lock 7820148308 ni recetas) y `inventory.Worker` la procesa con ese lock, a la hora de la liberación. `inventory/ledger.go` (`Apply`) es la única vía para cambiar existencias y `inventory/costing.go` la única autoridad de costos; el libro es de solo inserción y toda corrección es un movimiento nuevo. Las existencias negativas no bloquean ventas. `inventory.stock.read` nunca implica costos. Una compra no crea gastos ni un gasto compras; el vínculo con caja es opcional y no toca turnos. Ver docs/INVENTORY_AND_PURCHASING.md («Limitaciones conocidas»).

## Continuidad de reportes

No reconstruir ni duplicar los reportes. Un reporte nuevo se declara en el registro cerrado de `internal/reports` (guía en `doc.go`), se agrega a `EXPORTABLE_REPORTS` de `desktop/src-tauri/src/export.rs` y sus filtros a la lista blanca `reports_*` de `api.rs`; la pantalla lo muestra sin código nuevo. Los reportes consultan en su propia transacción `REPEATABLE READ READ ONLY` con el semáforo de 2 conexiones y nunca toman locks consultivos ni llaman handlers de otros módulos. La venta del periodo es la suma del libro inmutable `sales_recognitions` (escrito al cerrarse la cuenta), nunca `closed_at`. La «Hora de inicio del día» es una sola (`businessday`). Los permisos de origen (`cash.expected.read`, `inventory.costs.read`, `inventory.valuation.read`, `inventory.stock.read`) siguen mandando en pantalla, resumen y archivos, y los turnos en arqueo se excluyen de todo agregado. Las exportaciones salen del mismo resultado de la pantalla; el diálogo nativo y la escritura solo existen en Rust. Ver docs/REPORTS.md («Limitaciones conocidas»).

## Próxima etapa

La 9A ya no tiene pendientes de instalación: la regresión final de la fase D aprobó y las 0015–0017 se aplicaron a la instalación real el 10 de noviembre de 2026 con autorización del propietario, respaldo y ensayo de restauración (procedimiento de DEVELOPMENT_MACOS.md); solo queda sin verificar el recorrido en la ventana nativa, el diálogo nativo real de guardado y la pantalla táctil física. Siguiente, según el plan (`CLAUDE.md` §13–14), la etapa **9B**: identidad visual del establecimiento, logotipo y diseñador de tickets y documentos, que la 9A no implementa. No añadir Android, agente remoto, aplicación KDS, facturación electrónica ni licenciamiento de forma incidental.

## Verificar antes de ampliar

```bash
./scripts/dev.sh test
./scripts/dev.sh test-integration
./scripts/dev.sh test-browser
./scripts/dev.sh build-macos
./scripts/dev.sh db-start
./scripts/dev.sh server-start
./scripts/dev.sh open-app
```

La aceptación PostgreSQL crea/elimina bases aleatorias adel_pos_test_*. La visual crea otra base por navegador y usa Go real, con el IPC sustituido únicamente en Node. La producción no ofrece un cliente web alternativo. No confundir este recorrido con automatización completa de la ventana nativa ni con dos máquinas físicas.

`tests/runtime_smoke.py` y su opción `--terminal-only` verifican los procesos propios de desarrollo y los interrumpen brevemente. Ejecutarlos secuencialmente, sin suites PostgreSQL ni trabajo real simultáneo. Ver resultados actuales y evidencia por etapa en docs/VERIFICATION.md.

## Límites a resolver con alcance explícito

Hay SSE, preparación e impresión CUPS/ESC-POS. No existen WebSocket, offline durable, agente remoto, aplicación KDS, facturación electrónica (CFDI), cuentas por pagar ni licenciamiento comercial. El inventario (etapa 8) y los reportes y exportaciones (etapa 9A) no están verificados en la ventana nativa, con el diálogo nativo real de guardado ni en pantalla táctil física. El cierre de turno y el arqueo existen desde la etapa 7, sin hardware de cajón verificado. El servidor production sigue bloqueado por licencias. Firma/notarización, servicio instalado y Windows/Android permanecen pendientes.

Medir carga antes de uso comercial: locks operativos serializan escrituras, snapshots completos por lote consumen espacio y cuentas/historiales grandes pueden exceder el límite IPC de 2 MiB. El lote de captura se limita a 20 partidas/1000 unidades y el catálogo a 5000 referencias. Implementar paginación de detalle y búsqueda remota antes de superar los límites. No se ha medido LAN con otra máquina, muchas terminales ni resistencia a caída de energía.

Faltan retención de auditoría/eventos/sesiones/idempotencia, respaldos operativos periódicos, rotación de certificados, usuario DB runtime separado del migrador y servicio comercial sin sesión OS. La ambigüedad de hora local por cambio estacional usa la resolución estándar de Go; un selector de ocurrencia necesita diseño propio.

## Reglas visuales permanentes

Iniciar fullscreen nativo, mantener controles visibles de salida/regreso y modo ventana normal sin quiosco. 1280×720 es una prueba, no una restricción de monitor. Mantener columnas fluidas, controles prácticos y formularios/selecciones al redimensionar, con gofmt, Prettier y comentarios de reglas complejas en español.
