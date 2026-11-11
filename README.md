# AD-EL POS · Etapa 9B

Aplicación de escritorio para restaurantes: monolito modular Go, PostgreSQL y React/TypeScript/Tailwind dentro de Tauri 2. Conserva configuración del establecimiento, usuarios, roles, permisos, sesiones, terminales, cajas físicas y auditoría. Añade catálogo, ingredientes, personalizaciones, combos, listas y promociones con un motor decimal exacto en Go. Incluye áreas, mesas, cuentas, pedidos persistidos y operación para llevar. Integra cocina, estaciones, rutas, comandas, cola durable de impresión y eventos autenticados. Registra cobros totales, parciales, mixtos y por productos con efectivo y cambio, descuentos, cortesías, cargos generales, propinas, IVA incluido, devoluciones, correcciones, turno mínimo de caja, precuentas, tickets y entrega de pedidos para llevar. Administra la caja: apertura con fondo, ingresos, retiros y gastos autorizados, cortes parciales, arqueo y cierre ciego con diferencias por moneda, comprobantes y cortes impresos, cajón de dinero y tope de caja. Registra inventario (etapa 8): insumos con unidades y presentaciones, almacenes, existencias y un libro de movimientos inmutable con costo promedio; recetas versionadas, subrecetas y porciones habituales; consumo teórico al liberar a cocina como función activable; mermas, ajustes, transferencias y conteos físicos; proveedores, órdenes de compra, recepciones y devoluciones a proveedor con vínculo opcional a gastos de caja; alertas y sugerencias de compra. Consulta reportes (etapa 9A): un resumen ejecutivo y 92 reportes de ventas, productos, categorías, cocina, cobros, descuentos, caja, inventarios, compras, proveedores, costos y márgenes y empleados, calculados en Go con la «Hora de inicio del día» y la semana ISO, y exportables a PDF y Excel con los mismos permisos. Diseña sus documentos impresos (etapa 9B): identidad del establecimiento con el logotipo del restaurante para documentos y reportes, y un editor visual por bloques de tickets, precuentas, comandas, avisos de cocina, comprobantes de caja y cortes, con vista previa exacta a 58 y 80 mm, versiones, muestras y reimpresión idéntica al original. Inicia en pantalla completa nativa con salida y regreso visibles; las pantallas aprovechan la resolución disponible.

## Inicio en este Mac

```bash
cd "/Users/eliver/Documents/ADEL POS/ad-el-pos"
./scripts/dev.sh db-start
./scripts/dev.sh migrate
./scripts/dev.sh prepare-setup
./scripts/dev.sh server-start
./scripts/dev.sh open-app
```

**Instalación existente de este Mac:** las migraciones `0018_document_templates.sql` y `0019_document_identity.sql` de la etapa 9B (versión 0.9.1, esquema 19) se aplicaron a la instalación real el 11 de noviembre de 2026 con autorización del propietario, tras respaldo y ensayo de restauración (registro en [docs/ETAPA_9B_AVANCE.md](docs/ETAPA_9B_AVANCE.md), «Instalación real»); el servidor y el `.app` 0.9.1 (esquema 19) están instalados. Las migraciones 0015–0017 de la etapa 9A se aplicaron a la instalación real el 10 de noviembre de 2026 con autorización del propietario, tras respaldo y ensayo de restauración, con el procedimiento de [DEVELOPMENT_MACOS.md](DEVELOPMENT_MACOS.md); entonces quedaron instalados el servidor y el `.app` 0.9.0 (esquema 17). Las 0010–0014 de la etapa 8 se aplicaron antes, ese mismo día. Toda migración nueva (desde la 0020) requiere la misma autorización y procedimiento.

Para desarrollo con recarga: `./scripts/dev.sh desktop`. Para construir el `.app`: `./scripts/dev.sh build-macos`. La compilación está en `desktop/src-tauri/target/release/bundle/macos/AD-EL POS.app`.

En una instalación nueva aparece el asistente local. El nombre inicial del establecimiento es **ADEL POS** y la zona horaria se obtiene del sistema operativo; ambos son editables. Al guardar se crea **Caja 1**, sin abrir turnos ni registrar fondos, y se autoriza la terminal principal. Después accede con **admin / admin**. La advertencia de contraseña inicial recomienda cambiarla, pero permite continuar.

El asistente no vuelve a inicializar una instalación configurada ni restablece contraseñas. Los usuarios y cambios se guardan en PostgreSQL. Cerrar Tauri conserva Go y PostgreSQL; ambos se detienen de forma explícita con `server-stop` y `db-stop`.

## Mesas, cuentas y pedidos

En **Restaurante → Áreas y distribución** crea áreas y mesas y guarda su distribución. En **Mesas**, selecciona mesa y cuenta sin salir de la pantalla; admite varias cuentas por mesa y cuentas sin mesa. Captura productos del catálogo, personaliza con el mismo editor y guarda el borrador. **Confirmar pedido** registra un lote separado con precios y promociones históricos. Un mesero autorizado puede modificar o cancelar con motivo, sin aprobación universal de supervisor.

Desde **Acciones de cuenta** divide cantidades enteras, transfiere productos, combina cuentas, cambia mesa/responsable o consulta historial. Las unidades conservan identidad, personalización y centavos; las transferencias no generan preparación adicional. Una revisión obsoleta conserva la captura para revisión. Una respuesta incierta se recupera con el mismo UUID para evitar duplicados.

**Para llevar** utiliza listado a la izquierda y cuenta a la derecha, orden por antigüedad, búsqueda, avisos configurables y programación en la zona horaria del establecimiento. Ventanilla es un origen de esta misma modalidad. Confirmar crea una intención durable y resuelve la liberación real a cocina; la configuración incompleta deja advertencias y el pedido conservado. Preparación, pago y entrega permanecen separados: un pedido para llevar se marca entregado solo cuando está liquidado.

Los acordeones comparten un componente accesible en pantallas anteriores y nuevas. El teclado virtual sigue la preferencia de cada terminal. Consulta [docs/RESTAURANT_OPERATIONS.md](docs/RESTAURANT_OPERATIONS.md) para ciclo de vida, precios, concurrencia, límites y recorrido de uso. La integración actual está en [Cocina e impresión](docs/KITCHEN_AND_PRINTING.md).

## Cobros, pagos y devoluciones

Antes de cobrar, la terminal debe estar asociada a una caja con turno abierto; la apertura se hace en la pantalla de cobro o en **Caja** con un fondo explícito (puede ser $0). Mientras la caja está en arqueo de cierre no se cobra ni se devuelve.

Desde una cuenta, **Cobrar** abre la pantalla de cobro sin salir de la operación: productos con su estado de pago, métodos como botones con icono, teclado numérico fijo y captura de efectivo recibido, cambio, propina y referencia. Go calcula cada importe; la interfaz solo muestra el resultado. Se admiten pagos totales, abonos parciales y pagos mixtos sucesivos, y el cobro de productos o cantidades seleccionados. Una cuenta de salón liquidada se cierra y libera la mesa; un pedido para llevar se cierra al entregarse.

Descuentos por partida o generales, por porcentaje o importe, cortesías del 100 % y cargos extras generales se administran desde las acciones de la cuenta, con un acceso contextual desde el cobro. Cada rol tiene un límite de descuento configurable; superarlo, devolver sin permiso suficiente o corregir un pago requiere la autorización puntual de un supervisor, que no inicia sesión ni afecta la suya. Las devoluciones (totales, de un pago o por productos) y las correcciones de método son registros compensatorios; nunca se edita el pago original.

El IVA está incluido en los precios. En **Cobros** se elige la tasa del establecimiento con botones (16 %, 8 % o 0 %), se administran métodos de pago y políticas, y se consultan los movimientos del turno actual. Cada producto confirmado conserva la tasa vigente al confirmarse. Las precuentas y tickets se imprimen en la impresora de tickets configurada; un fallo de impresión no revierte el pago y los documentos no son comprobantes fiscales (CFDI). Consulta [docs/PAYMENTS_AND_REFUNDS.md](docs/PAYMENTS_AND_REFUNDS.md).

## Caja, arqueos y cortes

En **Caja** se administra el turno de la caja física de la terminal, según los permisos de cada usuario:

- **Operación**: apertura por total o por denominaciones (el fondo puede ser $0, en pesos y, con la función de dólares, también en dólares), ingresos, retiros y gastos con categoría, cortes parciales, «Abrir cajón» y el arqueo de cierre. Los retiros y gastos siempre quedan autorizados: con el permiso propio de autorizar no se pide otra contraseña; sin él, autoriza un supervisor. Un retiro mayor que el efectivo disponible se rechaza sin mostrar cifras.
- **Arqueo y cierre ciego**: «Iniciar arqueo» pausa la caja (no se cobra, devuelve ni registra efectivo) y el cajero cuenta por denominaciones, más un importe adicional identificado, sin ver el esperado; nadie lo ve durante el arqueo, ni el administrador. El borrador se guarda en el servidor. Se cancela para reanudar la caja o se confirma; las diferencias (sobrante o faltante, por moneda) no impiden cerrar y las cuentas abiertas tampoco. Después del cierre, quien tiene el permiso consulta el esperado y las diferencias.
- **Movimientos, Historial y Reportes**: movimientos del turno con su detalle y correcciones compensatorias, turnos anteriores con sus cortes y regularizaciones posteriores, y resumen del turno. **Configuración**: denominaciones, categorías de gasto, cajón por caja y tope de caja.
- **Documentos**: comprobantes de ingreso, retiro y gasto, y cortes parcial y definitivo con vista previa, impresión opcional y reimpresión; el esperado solo se imprime para quien puede consultarlo. Un fallo de impresión nunca revierte la operación ni el cierre, y «enviado» no confirma que el papel salió.
- **Cajón y tope**: el cajón se abre con un pulso de una impresora ESC/POS por red o de una cola CUPS en modo raw (etapa 7.1) tras operaciones con efectivo, nunca por pagos electrónicos; la interfaz dice «Comando de apertura enviado». El tope de caja es una función activable (desactivada por defecto) que solo avisa, a quien ve el esperado, que conviene un retiro; nunca bloquea cobros.

Consulta [docs/CASH_SHIFTS_AND_RECONCILIATION.md](docs/CASH_SHIFTS_AND_RECONCILIATION.md). La migración `0008_cash_shifts_and_reconciliation.sql` se aplica a una instalación existente con el procedimiento de respaldo y ensayo de restauración de [DEVELOPMENT_MACOS.md](DEVELOPMENT_MACOS.md).

## Inventarios, recetas y compras

En **Inventarios** (visible con el permiso de consultar inventario) se administra el inventario según los permisos de cada usuario. La administración funciona aunque el control esté desactivado, para preparar todo antes de activarlo:

- **Insumos y almacenes**: insumos con unidad base, presentaciones de compra («Caja con 24», «Costal 25 kg») y costo de referencia; unidades propias; categorías; almacenes con «Almacén general» como predeterminado y el almacén del que consume cada estación de cocina.
- **Existencias y movimientos**: existencia por insumo y almacén (las negativas se resaltan y nunca bloquean ventas), existencia inicial con costo, y el libro de movimientos con su origen y responsable. Los costos y el valor del inventario solo los ve quien tiene permiso para consultarlos.
- **Recetas**: recetas versionadas de productos, variantes y complementos, subrecetas de elaboración inmediata o almacenables con sus elaboraciones, porciones habituales de los ingredientes del catálogo y componentes «solo para llevar».
- **Consumo**: con la función **Control de inventario** activada (Configuración → Funciones del establecimiento; desactivada por omisión), cada producto liberado a cocina descuenta una sola vez los insumos de su receta y de las personalizaciones confirmadas: «Sin cebolla» omite la porción, «Con cebolla» usa la porción habitual y «Cebolla extra» el doble, sin pedir gramos al mesero. Las observaciones libres no cambian el inventario. Go calcula el consumo en segundo plano, así que la captura no espera.
- **Cancelaciones y mermas**: una partida cancelada después de liberarse se clasifica como «No preparado» (se revierte el consumo) o «Preparado (merma)» hasta el corte del día operativo; sin clasificar, Go la considera «No preparado» al cierre.
- **Compras**: proveedores, órdenes de compra, recepciones (de una orden o directas) con IVA 16/8/0, devoluciones a proveedor y vínculo opcional de una recepción con gastos de caja ya registrados. Una compra nunca crea un gasto de caja ni un gasto crea una compra.
- **Control**: ajustes de discrepancia (los que registra alguien sin permiso de autorizar esperan a que otra persona los autorice), mermas operativas, transferencias entre almacenes, conteos físicos por almacén con hoja de captura para tableta (a ciegas para quien solo cuenta) y niveles mínimos por almacén. El Resumen reúne las alertas (existencias negativas, bajo mínimo, consumos sin calcular, conteos pendientes…) y Compras ofrece sugerencias de compra que solo crean borradores.

Desde la etapa 9A, Inventarios se organiza en cinco grupos (Resumen, Catálogo, Existencias, Compras y Configuración), los proveedores admiten segundo teléfono, celular y página web, las recepciones directas pueden registrarse como **Compra de mostrador** sin un proveedor formal, el alta de insumos sugiere el código interno y las categorías de insumos se administran y se crean desde el editor del insumo.

Consulta [docs/INVENTORY_AND_PURCHASING.md](docs/INVENTORY_AND_PURCHASING.md).

## Reportes e indicadores

En **Reportes** (visible con el permiso de consultar reportes; etapa 9A) se consultan, según los permisos de cada usuario:

- **Resumen ejecutivo**: ventas, cobros, descuentos, caja, compras, inventario y margen estimado del periodo, comparados con el periodo anterior equivalente; cada tarjeta abre su reporte detallado. Se actualiza con «Actualizar» o cada 5 minutos con la ventana visible, no con cada cambio.
- **Reportes** agrupados en Ventas, Productos, Categorías, Pagos, Descuentos y cortesías, Caja, Cocina y operación, Inventarios, Compras, Proveedores, Costos y márgenes y Empleados, con filtro de periodo (hoy, ayer, semana, mes o año actuales, un día, una semana del año ISO, un mes, un año o un rango), filtros propios de cada reporte, tablas con los totales de todo el periodo calculados en Go, orden, paginación y gráficos con su tabla alternativa.
- **Cuándo cuenta una venta**: cuando la cuenta se cierra liquidada, en el día del cierre; las cuentas abiertas aparecen en «En curso» y una devolución se reporta en su propio día. El día comercial lo define la **Hora de inicio del día** (Configuración general; con 05:00, una venta de la 01:30 pertenece al día anterior).
- **Permisos**: los reportes respetan los de Caja e Inventarios: el esperado y las diferencias de caja solo con el permiso de consultarlos (y nunca durante un arqueo), los costos y márgenes solo con el de consultar costos.
- **Exportar PDF** y **Exportar Excel** guardan el reporte con el diálogo nativo de macOS, con el mismo contenido que la pantalla.

Consulta [docs/REPORTS.md](docs/REPORTS.md).

## Documentos e identidad del establecimiento

En **Documentos** (visible con el permiso de consultar plantillas; etapa 9B):

- **Identidad del establecimiento**: información adicional (RFC, razón social…), texto de encabezado y de pie, y el **logotipo del restaurante** (PNG o JPEG de hasta 1 MiB), con su vista previa tal como saldrá en papel de 58 y 80 mm, ancho, alineación y método de conversión (umbral o difuminado). Nombre, dirección, teléfono y correo siguen en Configuración general. El logotipo se usa en los documentos y en el encabezado de los reportes PDF y Excel; la aplicación conserva la marca de AD-EL POS.
- **Plantillas**: para cada uno de los diez documentos, la «Predeterminada» (idéntica a los formatos anteriores y de solo lectura) y las propias, que se crean, duplican, renombran, activan y restauran. Instalar la 9B no cambia ningún papel hasta activar una plantilla propia.
- **Editor visual** en tres áreas: elementos compatibles con el documento, vista previa con exactamente las líneas que se imprimirán (58 u 80 mm, datos de muestra o un documento real) y estructura y propiedades del bloque elegido (orden, alineación, tamaño, negrita, mayúsculas, visibilidad y espaciado). Nunca se escribe HTML, código ni comandos de impresora. Cada guardado crea una versión; dos personas que editan la misma plantilla no se sobrescriben.
- **Muestras** marcadas «MUESTRA · NO ES UN DOCUMENTO VÁLIDO», sin ventas, pagos ni movimientos, por la cola de impresión de siempre.
- **Reimprimir** saca una copia idéntica al original aunque la plantilla haya cambiado; **Imprimir con la plantilla vigente** es otra acción, con su permiso, que imprime los mismos datos con la plantilla activa hoy.
- Negrita, tamaños y logotipo solo salen por ESC/POS en red o por una impresora CUPS marcada «Esta impresora entiende ESC/POS (formato y logotipo)» cuya cola admita datos raw; en las demás se imprime el texto, con aviso. El resultado en papel es una prueba física pendiente.

Consulta [docs/DOCUMENT_DESIGNER.md](docs/DOCUMENT_DESIGNER.md).

## Cocina e impresión

En **Cocina y preparación** configura impresoras, estaciones y rutas por categoría/producto. Admite varios destinos por producto, varias estaciones en una impresora y documentos independientes o unificados con secciones. Los productos nuevos generan comandas normales; las ediciones y cancelaciones producen avisos históricos. La reimpresión conserva la presentación original.

La cola persiste intentos y resultados. CUPS o ESC/POS TCP aceptado significa recepción del transporte; no confirma salida de papel. Un resultado incierto exige revisión con motivo y nunca se reenvía automáticamente. Go procesa impresión y pedidos programados aunque Tauri esté cerrado. El estado listo requiere completar todas las estaciones y permanece separado de pago/entrega.

Los eventos utilizan SSE autenticado desde Rust, con outbox, cursor, replay, deduplicación y recuperación de estado completo. Se prepararon contratos de recepción/estado para el futuro KDS; no se creó una aplicación KDS ni agente remoto. Consulta [la guía técnica y operativa](docs/KITCHEN_AND_PRINTING.md) y [la evidencia de verificación](docs/VERIFICATION.md).

## Catálogo y precios

En **Catálogo** administra categorías, productos independientes, presentaciones opcionales, ingredientes, complementos, porciones, grupos reutilizables, combos, listas de precios y promociones. Incluye imágenes opcionales, horarios, archivo lógico, búsqueda, filtros y paginación. La edición rechaza revisiones obsoletas de otra terminal.

En **Probar personalización** retira o sustituye todos los complementos, cambia cantidades, ingredientes, componentes y observaciones. Go valida y devuelve el desglose exacto; no se crea una venta. Las listas conservan la identidad del producto y las promociones explican su aplicación o exclusión. Los PNG originales locales cambian con el tema.

Desde la etapa 9A cada producto puede activar **«El precio puede bajar al cambiar complementos»** (desactivada por omisión): sin ella, quitar o cambiar un complemento nunca baja el precio. Consulta [docs/CATALOG.md](docs/CATALOG.md) para reglas y ejemplos reproducibles: filete $150 → $155 al cambiar arroz por papas, $150 → $140 con la opción al cambiar arroz por frijoles, y combo con papas $90 / $100 / $140. La migración de catálogo es `0003_catalog_and_pricing.sql`; la etapa 4 añade `0004_restaurant_operations.sql`, la etapa 5 `0005_kitchen_and_printing.sql`, la etapa 6 `0006_payments_and_refunds.sql`, la etapa 6.1 `0007_features_product_tax_and_usd_cash.sql`, la etapa 7 `0008_cash_shifts_and_reconciliation.sql`, la etapa 7.1 `0009_operation_tables_cash_and_reasons.sql` la etapa 8 `0010_inventory_core.sql` a `0014_inventory_control.sql` (aplicadas en la instalación real) la etapa 9A `0015_catalog_price_and_precheck.sql`, `0016_inventory_suppliers_counter.sql` y `0017_reports.sql` (también aplicadas en la instalación real) y la etapa 9B `0018_document_templates.sql` y `0019_document_identity.sql` (pendientes de aplicar en la instalación real). Las migraciones anteriores se conservan. Antes de migrar una instalación existente, generar respaldo con `./scripts/dev.sh backup` y ensayar su restauración en una base temporal.

## Credencial reservada de soporte

La identidad `sa` está reservada y excluida de la administración ordinaria. Su contraseña la debe elegir el propietario en una terminal local:

```bash
./scripts/dev.sh provision-sa
```

El comando solicita y confirma el secreto sin eco, guarda solo Argon2id y audita la operación. No proporcionar contraseñas en argumentos, variables de entorno, chat o archivos versionados. Mientras el propietario no complete este paso, la identidad existe sin una contraseña utilizable. Un restablecimiento excepcional usa `./scripts/dev.sh reset-sa`, con confirmación local explícita y auditoría.

## Estructura

```text
ad-el-pos/
├── backend/
│   ├── cmd/ad-el-pos-server/     # Servidor, migraciones y mantenimiento local
│   ├── internal/administration/ # Identidad, permisos y recursos administrativos
│   ├── internal/catalog/       # Catálogo, disponibilidad y motor decimal único
│   ├── internal/config/        # Parámetros técnicos del servidor
│   ├── internal/database/      # Pool y migrador transaccional
│   ├── internal/kitchen/       # Preparación, documentos, transportes y trabajadores
│   ├── internal/events/        # Outbox, cursor y replay autenticado
│   ├── internal/operations/    # Mesas, cuentas, pedidos, unidades e historial
│   ├── internal/payments/      # Cobros, IVA, descuentos, devoluciones, tickets y caja (turnos, cortes, arqueo y cierre)
│   ├── internal/inventory/     # Insumos, almacenes, existencias, costos, recetas, consumo, compras, ajustes y conteos
│   ├── internal/reports/       # Reportes, resumen ejecutivo y exportación PDF/Excel (solo lectura)
│   ├── internal/businessday/   # Día comercial y periodos con semana ISO
│   ├── internal/documents/     # Plantillas por bloques, validación, renderizador único e IR (etapa 9B)
│   ├── internal/designer/      # Rutas del diseñador de documentos
│   ├── internal/identity/      # Identidad documental y logotipo del restaurante
│   ├── internal/logoraster/    # Conversión monocromática del logotipo para impresoras térmicas
│   ├── internal/brand/         # Copia embebida de la marca de AD-EL POS (reportes)
│   ├── internal/httpapi/       # HTTP, diagnósticos, CORS y errores
│   ├── internal/localsetup/    # Certificados locales
│   └── migrations/             # 0001 a 0019; inmutables después de aplicarse
├── desktop/src/
│   ├── components/             # Formularios, permisos y presentación reutilizables
│   ├── hooks/                  # Sesión, recursos y conectividad
│   ├── pages/                  # Administración y operación del restaurante
│   └── services/               # Contratos tipados y puente API
├── desktop/src-tauri/src/       # Rutas enumeradas, TLS, credenciales en Keychain y exportación con diálogo nativo
├── config/                     # Identidad canónica y ejemplos sin secretos
├── scripts/                    # Desarrollo, formato, procesos y PostgreSQL
├── tests/                      # Pruebas operativas macOS
├── ops/launchd/                # Plantilla; no instalada automáticamente
└── docs/                       # Convenciones, dependencias y evidencia
```

`.local`, `.tools`, cachés, respaldos y artefactos están excluidos del repositorio. Node/Rust son herramientas de construcción; el `.app` funciona sin Vite. PostgreSQL y Go son procesos independientes.

## Verificación

```bash
./scripts/dev.sh format-check
./scripts/dev.sh test
./scripts/dev.sh test-integration
./scripts/dev.sh test-browser
```

Las pruebas de integración crean y eliminan únicamente bases aleatorias `adel_pos_test_*`. No borran `adel_pos`. Consulta los resultados y límites reales en [docs/VERIFICATION.md](docs/VERIFICATION.md).

## Documentación

- [DEVELOPMENT_MACOS.md](DEVELOPMENT_MACOS.md): preparación, ejecución, terminales, TLS y recuperación local.
- [ARCHITECTURE.md](ARCHITECTURE.md): responsabilidades y límites.
- [API_CONTRACTS.md](API_CONTRACTS.md): rutas y permisos.
- [docs/CATALOG.md](docs/CATALOG.md): reglas comerciales, uso y límites del catálogo.
- [docs/RESTAURANT_OPERATIONS.md](docs/RESTAURANT_OPERATIONS.md): flujo de pedidos, unidades y concurrencia.
- [docs/PAYMENTS_AND_REFUNDS.md](docs/PAYMENTS_AND_REFUNDS.md): cobros, IVA, descuentos, devoluciones y tickets.
- [docs/CASH_SHIFTS_AND_RECONCILIATION.md](docs/CASH_SHIFTS_AND_RECONCILIATION.md): caja, arqueos, cierre ciego, cortes, documentos, cajón y tope.
- [docs/INVENTORY_AND_PURCHASING.md](docs/INVENTORY_AND_PURCHASING.md): inventarios, recetas, consumo, costos, compras, ajustes y conteos.
- [docs/REPORTS.md](docs/REPORTS.md): reportes, métricas, fechas comerciales, permisos y exportaciones.
- [docs/DOCUMENT_DESIGNER.md](docs/DOCUMENT_DESIGNER.md): identidad del establecimiento, logotipo, plantillas, editor visual, versiones e impresión de documentos.
- [docs/CODING_CONVENTIONS.md](docs/CODING_CONVENTIONS.md): reglas permanentes de legibilidad.
- [NEXT_STEPS_FOR_SOL.md](NEXT_STEPS_FOR_SOL.md): continuidad y decisiones pendientes.

La distribución comercial sigue bloqueada hasta integrar licenciamiento. Firma/notarización Apple, servicio comercial persistente e instaladores Windows siguen pendientes. El bundle ID `local.adel.pos.dev` es provisional y no representa un dominio registrado.
