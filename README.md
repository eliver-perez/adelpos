# AD-EL POS · Etapa 9G

Aplicación de escritorio para restaurantes: monolito modular Go, PostgreSQL y React/TypeScript/Tailwind dentro de Tauri 2. Conserva configuración del establecimiento, usuarios, roles, permisos, sesiones, terminales, cajas físicas y auditoría. Añade catálogo, ingredientes, personalizaciones, combos, listas y promociones con un motor decimal exacto en Go. Incluye áreas, mesas, cuentas, pedidos persistidos y operación para llevar. Integra cocina, estaciones, rutas, comandas, cola durable de impresión y eventos autenticados. Registra cobros totales, parciales, mixtos y por productos con efectivo y cambio, descuentos, cortesías, cargos generales, propinas, IVA incluido, devoluciones, correcciones, turno mínimo de caja, precuentas, tickets y entrega de pedidos para llevar. Administra la caja: apertura con fondo, ingresos, retiros y gastos autorizados, cortes parciales, arqueo y cierre ciego con diferencias por moneda, comprobantes y cortes impresos, cajón de dinero y tope de caja. Registra inventario (etapa 8): insumos con unidades y presentaciones, almacenes, existencias y un libro de movimientos inmutable con costo promedio; recetas versionadas, subrecetas y porciones habituales; consumo teórico al liberar a cocina como función activable; mermas, ajustes, transferencias y conteos físicos; proveedores, órdenes de compra, recepciones y devoluciones a proveedor con vínculo opcional a gastos de caja; alertas y sugerencias de compra. Consulta reportes (etapa 9A): un resumen ejecutivo y 92 reportes de ventas, productos, categorías, cocina, cobros, descuentos, caja, inventarios, compras, proveedores, costos y márgenes y empleados, calculados en Go con la «Hora de inicio del día» y la semana ISO, y exportables a PDF y Excel con los mismos permisos. Diseña sus documentos impresos (etapa 9B): identidad del establecimiento con el logotipo del restaurante para documentos y reportes, y un editor visual por bloques de tickets, precuentas, comandas, avisos de cocina, comprobantes de caja y cortes, con vista previa exacta a 58 y 80 mm, versiones, muestras y reimpresión idéntica al original. Desde la etapa 9C se navega con una barra superior táctil (sin menú lateral), Administración por mosaicos con ruta, una pantalla de inicio por terminal, un selector único para todas las listas desplegables y un selector de hora, y también solo con teclado: menú nativo en español, atajos y paleta de comandos, sin cambiar reglas de negocio, cálculos ni permisos. Desde la etapa 9D el color principal es el azul de la marca, con una paleta de estados que se reconoce por color, texto e icono; Caja pasa al lado derecho de la barra; Mesas muestra siempre su panel lateral con el resumen del área y un mapa que se ajusta solo; y cualquier cuenta sin pagos se puede **cancelar** con un motivo del catálogo y, si hace falta, la autorización de un supervisor, sin contarla como venta. Desde la etapa 9F una terminal puede usarse como **pantalla de cocina** (cada terminal muestra sus estaciones y cada usuario atiende las suyas, con tarjetas grandes que cambian de color con el tiempo y un aviso sonoro opcional), las impresoras pueden cortar el papel al terminar (corte total o parcial, con renglones de avance), una impresión sin impresora válida se avisa en el momento con «Configurar impresora» y «Reintentar», el documento previo al cobro puede llamarse «Precuenta», «Ticket de venta» o «Cuenta», cancelar o reducir lo enviado a cocina sin permiso pide la autorización de un supervisor, y las pruebas automáticas ya no pueden escribir en una instalación real. Desde la etapa 9G (versión 0.10.0) el restaurante tiene **una computadora principal** con el servidor como servicio del sistema, que arranca con la computadora aunque nadie inicie sesión, y **terminales** que la encuentran en la red, se emparejan con un código que aprueba un administrador y se comunican cifradas; sin conexión, una terminal lo dice, no deja vender y se reconecta sola; las impresoras USB de una caja imprimen desde esa terminal; la computadora principal respalda sola la base todos los días; y el servidor queda listo para la futura aplicación Android (API con contrato, catálogo descargable, «Agotado» desde la captura y pedidos capturados sin conexión). Inicia en pantalla completa nativa con salida y regreso visibles; las pantallas aprovechan la resolución disponible (solo escritorio y pantallas táctiles de punto de venta).

## Inicio en este Mac

```bash
cd "/Users/eliver/Documents/ADEL POS/ad-el-pos"
./scripts/dev.sh db-start
./scripts/dev.sh migrate
./scripts/dev.sh prepare-setup
./scripts/dev.sh server-start
./scripts/dev.sh open-app
```

**Instalación existente de este Mac:** la migración `0022_printing_and_kitchen_stations.sql` de la etapa 9F (versión 0.9.5, esquema 22) se aplicó a la instalación real el 13 de noviembre de 2026 con el procedimiento de respaldo, ensayo de restauración y verificación posterior de [DEVELOPMENT_MACOS.md](DEVELOPMENT_MACOS.md) (respaldo `backups/adel_pos-20261113T211630Z-a3fceb.dump` con restauración verificada: 134 tablas idénticas; `migrate` `applied:1` y repetición `applied:0`; readiness 200 con esquema compatible; servidor y `.app` 0.9.5); registro en [docs/ETAPA_9F_AVANCE.md](docs/ETAPA_9F_AVANCE.md), «Fase F». Antes, al empezar la 9F, la 0.9.4 aprobó la regresión completa en la Mac y se instaló el 13 de noviembre de 2026 sin migraciones (respaldo `backups/adel_pos-20261113T144419Z-6e62f4.dump` con restauración verificada: 134 tablas idénticas; `migrate` `applied:0`; readiness 200; servidor y `.app` 0.9.4); registro en [docs/ETAPA_9F_AVANCE.md](docs/ETAPA_9F_AVANCE.md), «Fase 0-R». La migración `0021_account_cancellation.sql` de la etapa 9D (versión 0.9.3, esquema 21) se aplicó a la instalación real el 12 de noviembre de 2026 con el procedimiento de respaldo, ensayo de restauración y verificación posterior de [DEVELOPMENT_MACOS.md](DEVELOPMENT_MACOS.md) (respaldo `backups/adel_pos-20261112T203123Z-ec18a4.dump` con restauración verificada: 132 tablas idénticas; `migrate` `applied:1` y repetición `applied:0`; readiness 200 con esquema compatible; servidor y `.app` 0.9.3); registro en [docs/ETAPA_9D_AVANCE.md](docs/ETAPA_9D_AVANCE.md), «Instalación real». La migración `0020_terminal_home_and_printer_columns.sql` de la etapa 9C (versión 0.9.2, esquema 20) se aplicó a la instalación real el 11 de noviembre de 2026 con la autorización previa del propietario (respaldo `backups/adel_pos-20261112T045957Z-a3a2b3.dump` con restauración verificada: 132 tablas idénticas; `migrate` `applied:1` y repetición `applied:0`; readiness 200 con esquema compatible; servidor y `.app` 0.9.2); registro en [docs/ETAPA_9C_AVANCE.md](docs/ETAPA_9C_AVANCE.md), «Instalación real». Las migraciones `0018_document_templates.sql` y `0019_document_identity.sql` de la etapa 9B (versión 0.9.1, esquema 19) se aplicaron a la instalación real el 11 de noviembre de 2026 con autorización del propietario, tras respaldo y ensayo de restauración (registro en [docs/ETAPA_9B_AVANCE.md](docs/ETAPA_9B_AVANCE.md), «Instalación real»); entonces quedaron instalados el servidor y el `.app` 0.9.1 (esquema 19). Las migraciones 0015–0017 de la etapa 9A se aplicaron a la instalación real el 10 de noviembre de 2026 con autorización del propietario, tras respaldo y ensayo de restauración, con el procedimiento de [DEVELOPMENT_MACOS.md](DEVELOPMENT_MACOS.md); entonces quedaron instalados el servidor y el `.app` 0.9.0 (esquema 17). Las 0010–0014 de la etapa 8 se aplicaron antes, ese mismo día. Toda migración nueva requiere el mismo procedimiento; quién la autoriza está en `CLAUDE.md` §0 («Forma de trabajo»).

**Servidor como servicio (etapa 9G)**: **desde el 14/11/2026 la instalación real de este Mac es el servicio**: el servidor es un servicio del sistema con sus datos en `/Library/Application Support/AD-EL POS`, la aplicación se abre desde `/Applications` y ya no se usan `db-start`, `server-start`, `migrate` ni `backup` de `dev.sh` con la instalación real (ver [DEVELOPMENT_MACOS.md](DEVELOPMENT_MACOS.md), «Servidor del restaurante como servicio»). `.local` queda detenido como copia histórica (no borrar).

Para desarrollo con recarga: `./scripts/dev.sh desktop`. Para construir el `.app`: `./scripts/dev.sh build-macos`. La compilación está en `desktop/src-tauri/target/release/bundle/macos/AD-EL POS.app`.

## Windows (etapa 9E)

AD-EL POS 0.9.4 compila, pasa sus pruebas y tiene un **instalador de prueba** (NSIS, sin firmar) para Windows 10 y 11 de 64 bits; se desarrolló y se probó en Windows 11. El instalador se genera con `powershell -ExecutionPolicy Bypass -File scripts\build-windows.ps1` y queda en `desktop\src-tauri\target\release\bundle\nsis\`. En la instalación de Windows la aplicación arranca y detiene su propio servidor y PostgreSQL, y los datos viven en `%LOCALAPPDATA%\AD-EL POS` (desinstalar no los borra). Desarrollo en Windows: [DEVELOPMENT_WINDOWS.md](DEVELOPMENT_WINDOWS.md) (`scripts\dev.ps1`). Guía para instalarlo: [docs/INSTALAR_EN_WINDOWS.md](docs/INSTALAR_EN_WINDOWS.md). La 0.9.4 se probó e instaló después en la Mac (etapa 9F). La 0.9.5 de la etapa 9F se desarrolló en la Mac y **no se ha compilado ni probado en Windows**: lo que hay que comprobar allí está en [docs/NOTAS_WINDOWS.md](docs/NOTAS_WINDOWS.md), «Etapa 9F: qué verificar en Windows». Desde la 9F la prueba de humo de la aplicación instalada usa una carpeta de datos temporal y ya no crea datos en la instalación; lo que dejó la de la 9E se quita en Administración → Sistema → Mantenimiento («Quitar datos de prueba»).

En una instalación nueva aparece el asistente local. El nombre inicial del establecimiento es **ADEL POS** y la zona horaria se obtiene del sistema operativo; ambos son editables. Al guardar se crea **Caja 1**, sin abrir turnos ni registrar fondos, y se autoriza la terminal principal. Después accede con **admin / admin**. La advertencia de contraseña inicial recomienda cambiarla, pero permite continuar.

El asistente no vuelve a inicializar una instalación configurada ni restablece contraseñas. Los usuarios y cambios se guardan en PostgreSQL. Cerrar Tauri conserva Go y PostgreSQL; ambos se detienen de forma explícita con `server-stop` y `db-stop`.

## Computadora principal y terminales en red (etapa 9G)

Guía para el propietario: [docs/INSTALAR_EN_RED.md](docs/INSTALAR_EN_RED.md).

- **Instalador `.pkg` para macOS** (`./scripts/dev.sh build-macos-pkg`, sin firma): la aplicación va a Aplicaciones y,
  con la opción «Servidor del restaurante», el servidor queda como servicio del sistema (necesita PostgreSQL 18 de
  Homebrew). Al abrir la aplicación por primera vez se elige **Servidor y terminal**, **Solo servidor** (sin venta,
  solo una pantalla de estado) o **Solo terminal**; se puede cambiar en Sistema › Mantenimiento sin borrar la base.
- **Emparejar una terminal**: en la terminal, «Buscar el servidor» lista las computadoras principales de la red (o se
  escribe la dirección) y muestra un código de 6 dígitos; un administrador lo escribe en **Sistema › Terminales** de la
  computadora principal. Ahí se ven las terminales con su tipo, sistema, usuario actual y si están en línea, se
  revocan y se cuentan las sesiones por tipo (sin limitar nada todavía).
- **Sin conexión**: la terminal muestra «Sin conexión con el servidor», bloquea las operaciones sin perder lo capturado
  y se reconecta sola; si la computadora principal cambia de dirección, la terminal la vuelve a encontrar.
- **«Agotado»** desde la captura con un toque («Solo por hoy» o «Hasta nuevo aviso»), con su permiso; llega al momento
  a todas las terminales y vuelve a disponible con el corte del día.
- **Impresoras USB de una terminal** (la de la Caja 2): las imprime esa terminal mientras AD-EL POS está abierta; si
  está apagada, la cola dice «La Caja 2 está desconectada». Cada caja puede tener su impresora de tickets.
- **Respaldos automáticos diarios** en la computadora principal (a la hora de inicio del día, últimos 30, carpeta
  configurable, revisados al crearse y con aviso en la pantalla de inicio si fallan), en Sistema › Mantenimiento ›
  Respaldos, con «Respaldar ahora». Restaurar sigue siendo una acción de soporte por línea de comandos.
- **API v1 para Android** (la aplicación no se hace aquí): contrato OpenAPI y guía en [docs/API_V1.md](docs/API_V1.md).

Windows (servicio, instalador para todo el equipo y pruebas cruzadas) es la Fase W de la etapa
([docs/NOTAS_WINDOWS.md](docs/NOTAS_WINDOWS.md)).

## Navegación, teclado y atajos

Desde la etapa 9C no hay menú lateral. Una **barra superior** fija de una fila, con botones grandes de icono y texto, lleva a la izquierda el logotipo (vuelve a la pantalla de inicio de la terminal) y **Mesas**, **Para llevar**, **Cocina** y **Administración**, seguidas y del mismo ancho; a la derecha, **Caja** en dos renglones («Caja» y el estado del turno: azul abierto, ámbar en arqueo o gris sin turno, con su menú rápido), el estado del servidor (en línea en azul, reconectando en ámbar o **sin conexión** en rojo), el menú del usuario (Mis preferencias, Cambiar contraseña y Cerrar sesión) y el botón de pantalla completa (orden de la etapa 9D; en la 9C Caja iba entre Cocina y Administración). Cada usuario ve solo lo que su rol permite. Durante la captura de un pedido la barra se reduce («‹ Mesas», la mesa y la cuenta, Caja y el usuario). Desde la 9D no hay botón «Actualizar» en Mesas, Cuentas, Para llevar ni la captura: los datos llegan solos y «Reintentar» aparece solo sin conexión. Desde la 9F, con la ventana más angosta (por ejemplo, una ventana de unos 1500 px en Windows al 125 %), la barra se compacta sin perder información: el isotipo «AE» de la marca, Caja con el estado corto del turno («Abierto», «En arqueo» o «Cerrado»), «En línea» con su texto y el nombre del usuario, que es lo único que se acorta; el logotipo ya no se queda marcado después de tocarlo.

**Administración** abre mosaicos grandes (Catálogo, Inventarios, Reportes, Documentos, Cobros, Personal y Sistema) con una ruta tocable («‹ Administración › Sistema › Terminales»). «Auditoría» se llama ahora **Registro de movimientos** y «Estado del sistema» se abre con «Ver estado del sistema» en Sistema → Configuración, en lenguaje sencillo. Cada terminal tiene su **pantalla inicial** (Mesas por omisión, o Para llevar, Cocina o Caja) en Sistema → Terminales. Desde la etapa 9F, **Sistema → Mantenimiento** ofrece «Quitar datos de prueba»: muestra antes la lista de lo que dejaron las pruebas automáticas (reconocido por su firma exacta, nunca por suposición), crea un respaldo antes de borrar, desactiva en lugar de borrar lo que ya tiene operaciones y nunca toca ventas, pagos, cortes ni movimientos de inventario.

Todas las listas desplegables usan un **selector** táctil con búsqueda (más de 8 opciones), grupos y selección múltiple, y las horas se eligen con un **selector de hora**. Para quien usa teclado y ratón: **menú de la aplicación** en español (Archivo, Edición, Ver, Caja y Ayuda), **atajos** con ⌘ en macOS y Ctrl en Windows (⌘1–⌘4 Mesas, Para llevar, Cocina y Caja, ⌘5 Administración, ⌘K paleta, ⌘F buscar, ⌘N nueva cuenta en mesa, ⌘P precuenta, ⌃⌘F pantalla completa, Esc cerrar o volver) y una **paleta de comandos** (⌘K) para ir a cualquier pantalla o acción escribiendo. Ningún atajo cobra, cancela, cierra turno ni devuelve: solo abren la pantalla correspondiente, con su confirmación de siempre. Consulta [docs/NAVIGATION_AND_SHORTCUTS.md](docs/NAVIGATION_AND_SHORTCUTS.md). El menú en Windows, la ventana nativa con el menú real y la pantalla táctil física no se han verificado.

**Colores (etapa 9D)**: azul para los botones principales, la selección, la mesa ocupada, el turno abierto, «En línea» y los avisos de éxito; ámbar para esperando pago, la caja en arqueo y las advertencias; uva para el pago parcial; gris para la mesa libre, fuera de servicio (con candado) y el turno cerrado; turquesa para los avisos informativos y rojo para las acciones que no se pueden deshacer, los errores y «Sin conexión». Los totales y saldos van en el color del texto, en negritas, y un botón deshabilitado muestra el cursor de «no disponible». Temas claro y oscuro.

## Mesas, cuentas y pedidos

En **Mesas → Áreas y distribución** (en la etapa 9B, «Restaurante → Áreas y distribución») crea áreas y mesas y guarda su distribución. En **Mesas**, selecciona mesa y cuenta sin salir de la pantalla; admite varias cuentas por mesa y cuentas sin mesa. Captura productos del catálogo, personaliza con el mismo editor y guarda el borrador. **Confirmar pedido** registra un lote separado con precios y promociones históricos. Un mesero autorizado puede modificar o cancelar con motivo; desde la etapa 9F, quien no tiene el permiso de cancelar productos necesita la autorización de un supervisor para «Cancelar unidades» o reducir la cantidad de lo ya enviado (como en «Cancelar cuenta»), y el historial y los reportes dicen quién autorizó. Desde la etapa 9C el encabezado del mapa tiene el selector de área y un **buscador de mesas** («5» encuentra «Mesa 5» y no «Mesa 15»), y cada mesa muestra su icono, el nombre grande, el estado en texto y, cuando aplica, el tiempo ocupada y la capacidad; acciones, estados y colores no cambian.

Desde la etapa 9D el **panel lateral de Mesas está siempre visible**: sin mesa elegida muestra el resumen del área (libres, ocupadas, esperando pago y pago parcial; tocar un número resalta esas mesas), «Cuenta sin mesa» y «Nuevo pedido para llevar». El mapa ajusta la escala al abrir un área para que se vean todas sus mesas y, si con un tamaño mínimo de unos 64 px no caben, se desplaza arrastrando o con las barras. «Capturar pedidos en esta mesa» es el único botón relleno del panel. Los tiempos de más de 24 horas se muestran en días y horas («3 d 14 h») y una cuenta abierta más de 12 horas (configurable en Funciones del establecimiento) se marca en ámbar.

**Cancelar cuenta** (etapa 9D), en el menú «Más…» de la cuenta y en rojo, sirve para cualquier cuenta con productos enviados y sin pagos, de mesa o para llevar (por ejemplo, un cliente que se va sin pagar): se elige un motivo de la lista (Cliente se retiró sin pagar, Cliente canceló el pedido, Error de captura u Otro, que pide descripción; se administra en Administración → Cobros), y quien no tiene el permiso «Cancelar cuentas» necesita la autorización de un supervisor, como en los descuentos. Si la cuenta tiene pagos, primero se devuelven; si tiene un descuento o cargo, primero se quita. La cuenta no cuenta como venta, la mesa queda libre y la cancelación aparece en el Registro de movimientos y en «Cuentas canceladas». Una cuenta sin productos enviados no se cancela: se **descarta**, sin motivo, y no aparece en reportes.

Desde la 9D, «Nuevo pedido» de Para llevar y «Cuenta sin mesa» entran a la captura sin crear nada: el pedido o la cuenta nacen al guardar el primer producto y, si se sale sin productos, no queda nada guardado (los datos del cliente se capturan después del primer producto). Las cuentas vacías que ya existían se quitan con **Descartar cuenta vacía**, sin motivo y sin contarlas como cancelación. Las tarjetas de Para llevar y de Mesas › Cuentas ocupan dos renglones (caben más del doble), el detalle dice «Enviado · sin confirmar en cocina» una sola vez por producto, el desglose solo muestra lo elegido y el descuento si lo hay, y «Modificar» y «Cancelar unidades» pasan a un menú que se abre al tocar el producto o «⋯».

Desde **Acciones de cuenta** divide cantidades enteras, transfiere productos, combina cuentas, cambia mesa/responsable o consulta historial. Las unidades conservan identidad, personalización y centavos; las transferencias no generan preparación adicional. Una revisión obsoleta conserva la captura para revisión. Una respuesta incierta se recupera con el mismo UUID para evitar duplicados.

**Para llevar** utiliza listado a la izquierda y cuenta a la derecha, orden por antigüedad, búsqueda, avisos configurables y programación en la zona horaria del establecimiento. Ventanilla es un origen de esta misma modalidad. Confirmar crea una intención durable y resuelve la liberación real a cocina; la configuración incompleta deja advertencias y el pedido conservado. Preparación, pago y entrega permanecen separados: un pedido para llevar se marca entregado solo cuando está liquidado.

Los acordeones comparten un componente accesible en pantallas anteriores y nuevas. El teclado virtual sigue la preferencia de cada terminal. Consulta [docs/RESTAURANT_OPERATIONS.md](docs/RESTAURANT_OPERATIONS.md) para ciclo de vida, precios, concurrencia, límites y recorrido de uso. La integración actual está en [Cocina e impresión](docs/KITCHEN_AND_PRINTING.md).

## Cobros, pagos y devoluciones

Antes de cobrar, la terminal debe estar asociada a una caja con turno abierto; la apertura se hace en la pantalla de cobro o en **Caja** con un fondo explícito (puede ser $0). Mientras la caja está en arqueo de cierre no se cobra ni se devuelve.

Desde una cuenta, **Cobrar** abre la pantalla de cobro sin salir de la operación: productos con su estado de pago, métodos como botones con icono, teclado numérico fijo y captura de efectivo recibido, cambio, propina y referencia. Go calcula cada importe; la interfaz solo muestra el resultado. Se admiten pagos totales, abonos parciales y pagos mixtos sucesivos, y el cobro de productos o cantidades seleccionados. Una cuenta de salón liquidada se cierra y libera la mesa; un pedido para llevar se cierra al entregarse.

Descuentos por partida o generales, por porcentaje o importe, cortesías del 100 % y cargos extras generales se administran desde las acciones de la cuenta, con un acceso contextual desde el cobro. Cada rol tiene un límite de descuento configurable; superarlo, devolver sin permiso suficiente o corregir un pago requiere la autorización puntual de un supervisor, que no inicia sesión ni afecta la suya. Las devoluciones (totales, de un pago o por productos) y las correcciones de método son registros compensatorios; nunca se edita el pago original.

El IVA está incluido en los precios. En **Administración → Cobros** se elige la tasa del establecimiento con botones (16 %, 8 % o 0 %), se administran métodos de pago y políticas, y se consultan los movimientos del turno actual. Cada producto confirmado conserva la tasa vigente al confirmarse. Las precuentas y tickets se imprimen en la impresora de tickets configurada; un fallo de impresión no revierte el pago y los documentos no son comprobantes fiscales (CFDI). Desde la etapa 9F, si no hay una impresora válida se avisa en el momento con **Configurar impresora** y **Reintentar** (sin mandar a la cola de impresión); lo que no llegó a imprimirse se reintenta con un toque y sin motivo, y reimprimir lo que ya salió pide un motivo. El documento previo al cobro puede llamarse **Precuenta** (predeterminado), **Ticket de venta** (como en el sistema anterior) o **Cuenta**, en Administración → Documentos: cambia en botones, avisos y el encabezado impreso, y sigue siendo el mismo documento. Consulta [docs/PAYMENTS_AND_REFUNDS.md](docs/PAYMENTS_AND_REFUNDS.md).

## Caja, arqueos y cortes

En **Caja** se administra el turno de la caja física de la terminal, según los permisos de cada usuario:

- **Operación**: apertura por total o por denominaciones (el fondo puede ser $0, en pesos y, con la función de dólares, también en dólares), ingresos, retiros y gastos con categoría, cortes parciales, «Abrir cajón» y el arqueo de cierre. Los retiros y gastos siempre quedan autorizados: con el permiso propio de autorizar no se pide otra contraseña; sin él, autoriza un supervisor. Un retiro mayor que el efectivo disponible se rechaza sin mostrar cifras.
- **Arqueo y cierre ciego**: «Iniciar arqueo» pausa la caja (no se cobra, devuelve ni registra efectivo) y el cajero cuenta por denominaciones, más un importe adicional identificado, sin ver el esperado; nadie lo ve durante el arqueo, ni el administrador. El borrador se guarda en el servidor. Se cancela para reanudar la caja o se confirma; las diferencias (sobrante o faltante, por moneda) no impiden cerrar y las cuentas abiertas tampoco. Después del cierre, quien tiene el permiso consulta el esperado y las diferencias.
- **Movimientos, Historial y Reportes**: movimientos del turno con su detalle y correcciones compensatorias, turnos anteriores con sus cortes y regularizaciones posteriores, y resumen del turno. **Configuración**: denominaciones, categorías de gasto, cajón por caja y tope de caja.
- **Documentos**: comprobantes de ingreso, retiro y gasto, y cortes parcial y definitivo con vista previa, impresión opcional y reimpresión; el esperado solo se imprime para quien puede consultarlo. Un fallo de impresión nunca revierte la operación ni el cierre, y «enviado» no confirma que el papel salió.
- **Cajón y tope**: el cajón se abre con un pulso de una impresora ESC/POS por red o de una cola CUPS en modo raw (etapa 7.1) tras operaciones con efectivo, nunca por pagos electrónicos; la interfaz dice «Comando de apertura enviado». El tope de caja es una función activable (desactivada por defecto) que solo avisa, a quien ve el esperado, que conviene un retiro; nunca bloquea cobros.

Consulta [docs/CASH_SHIFTS_AND_RECONCILIATION.md](docs/CASH_SHIFTS_AND_RECONCILIATION.md). La migración `0008_cash_shifts_and_reconciliation.sql` se aplica a una instalación existente con el procedimiento de respaldo y ensayo de restauración de [DEVELOPMENT_MACOS.md](DEVELOPMENT_MACOS.md).

## Inventarios, recetas y compras

En **Administración → Inventarios** (visible con el permiso de consultar inventario) se administra el inventario según los permisos de cada usuario. La administración funciona aunque el control esté desactivado, para preparar todo antes de activarlo:

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

En **Administración → Reportes** (visible con el permiso de consultar reportes; etapa 9A) se consultan, según los permisos de cada usuario:

- **Resumen ejecutivo**: ventas, cobros, descuentos, caja, compras, inventario y margen estimado del periodo, comparados con el periodo anterior equivalente; cada tarjeta abre su reporte detallado. Se actualiza con «Actualizar» o cada 5 minutos con la ventana visible, no con cada cambio.
- **Reportes** agrupados en Ventas, Productos, Categorías, Pagos, Descuentos y cortesías, Caja, Cocina y operación, Inventarios, Compras, Proveedores, Costos y márgenes y Empleados, con filtro de periodo (hoy, ayer, semana, mes o año actuales, un día, una semana del año ISO, un mes, un año o un rango), filtros propios de cada reporte, tablas con los totales de todo el periodo calculados en Go, orden, paginación y gráficos con su tabla alternativa.
- **Cuándo cuenta una venta**: cuando la cuenta se cierra liquidada, en el día del cierre; las cuentas abiertas aparecen en «En curso» y una devolución se reporta en su propio día. El día comercial lo define la **Hora de inicio del día** (Configuración general; con 05:00, una venta de la 01:30 pertenece al día anterior).
- **Permisos**: los reportes respetan los de Caja e Inventarios: el esperado y las diferencias de caja solo con el permiso de consultarlos (y nunca durante un arqueo), los costos y márgenes solo con el de consultar costos.
- **Exportar PDF** y **Exportar Excel** guardan el reporte con el diálogo nativo de macOS, con el mismo contenido que la pantalla.
- **Desde la etapa 9C** (94 reportes): «Cuentas anuladas» y «Modificaciones de cantidades» (quién, terminal, motivo, fecha e importes; sin «Autorizó», porque AD-EL POS no pedía la autorización de otra persona para esas acciones) y la referencia del comprobante en «Devoluciones» y «Correcciones de pagos».
- **Desde la etapa 9D** (95 reportes): «Cuentas anuladas» se llama **«Cuentas canceladas»** y muestra motivo, descripción, importe, quién canceló y **quién autorizó**, con filtro por motivo; las cuentas vacías liberadas o descartadas ya no aparecen. El reporte nuevo **«Cancelaciones por motivo»** suma cuántas cuentas, unidades e importe se cancelaron por cada motivo, para ver, por ejemplo, cuánto se pierde al mes por clientes que se van sin pagar.
- **Desde la etapa 9F**: «Modificaciones de cantidades» y «Productos cancelados» tienen la columna **«Autorizó»** (vacía si bastó el permiso propio y en los registros anteriores), también en PDF y Excel, y «Actividad por empleado» cuenta esas autorizaciones.

Consulta [docs/REPORTS.md](docs/REPORTS.md).

## Documentos e identidad del establecimiento

En **Administración → Documentos** (visible con el permiso de consultar plantillas; etapa 9B):

- **Identidad del establecimiento**: información adicional (RFC, razón social…), texto de encabezado y de pie, y el **logotipo del restaurante** (PNG o JPEG de hasta 1 MiB), con su vista previa tal como saldrá en papel de 58 y 80 mm, ancho, alineación y método de conversión (umbral o difuminado). Nombre, dirección, teléfono y correo siguen en Configuración general. El logotipo se usa en los documentos y en el encabezado de los reportes PDF y Excel; la aplicación conserva la marca de AD-EL POS.
- **Plantillas**: para cada uno de los diez documentos, la «Predeterminada» (idéntica a los formatos anteriores y de solo lectura) y las propias, que se crean, duplican, renombran, activan y restauran. Instalar la 9B no cambia ningún papel hasta activar una plantilla propia.
- **Editor visual** en tres áreas: elementos compatibles con el documento, vista previa con exactamente las líneas que se imprimirán (58 u 80 mm, datos de muestra o un documento real) y estructura y propiedades del bloque elegido (orden, alineación, tamaño, negrita, mayúsculas, visibilidad y espaciado). Nunca se escribe HTML, código ni comandos de impresora. Cada guardado crea una versión; dos personas que editan la misma plantilla no se sobrescriben.
- **Muestras** marcadas «MUESTRA · NO ES UN DOCUMENTO VÁLIDO», sin ventas, pagos ni movimientos, por la cola de impresión de siempre.
- **Reimprimir** saca una copia idéntica al original aunque la plantilla haya cambiado; **Imprimir con la plantilla vigente** es otra acción, con su permiso, que imprime los mismos datos con la plantilla activa hoy.
- **Nombre de la precuenta** (etapa 9F): «Precuenta» (predeterminado), «Ticket de venta» o «Cuenta»; se usa en botones, avisos y el encabezado impreso. En el editor, el tipo de documento sigue siendo «Precuenta» y el del ticket de pago se llama «Ticket de pago».
- Negrita, tamaños y logotipo solo salen por ESC/POS en red o por una impresora CUPS marcada «Esta impresora entiende ESC/POS (formato y logotipo)» cuya cola admita datos raw; en las demás se imprime el texto, con aviso. El resultado en papel es una prueba física pendiente.

Consulta [docs/DOCUMENT_DESIGNER.md](docs/DOCUMENT_DESIGNER.md).

## Cocina e impresión

En **Cocina** (pantalla «Cocina y preparación») configura impresoras, estaciones y rutas por categoría/producto. Admite varios destinos por producto, varias estaciones en una impresora y documentos independientes o unificados con secciones. Los productos nuevos generan comandas normales; las ediciones y cancelaciones producen avisos históricos (desde la etapa 9D también al cancelar una cuenta completa). La reimpresión conserva la presentación original. Sin estaciones, rutas ni impresoras configuradas, lo confirmado queda como liberación bloqueada («Sin ruta de preparación») y se libera a mano en Cocina; desde la 9D la cuenta lo dice como «Enviado · sin confirmar en cocina».

Desde la etapa 9C cada impresora elige sus **columnas de texto** (80 mm: 42, el formato de siempre, o 48; 58 mm: 30 o 32) con el botón **Imprimir prueba de ancho**, que imprime una regla numerada sin crear ventas ni movimientos para comprobar en papel si el renglón se corta. Con el valor predeterminado nada cambia; las copias de documentos ya impresos conservan su ancho. El resultado en papel a 48 o 32 columnas es una prueba física pendiente.

La cola persiste intentos y resultados. CUPS o ESC/POS TCP aceptado significa recepción del transporte; no confirma salida de papel. Un resultado incierto exige revisión con motivo y nunca se reenvía automáticamente. Go procesa impresión y pedidos programados aunque Tauri esté cerrado. El estado listo requiere completar todas las estaciones y permanece separado de pago/entrega.

Desde la etapa 9F:

- **Corte de papel por impresora**: «Cortar el papel al terminar» con corte total o parcial y 0 a 8 «Renglones de avance antes del corte» (4 por omisión), activado en las impresoras nuevas de 80 mm y desactivado en las de 58 mm; se aplica a tickets, precuentas, comandas, avisos y cortes de caja, y la prueba de ancho también corta. Una impresora sin cortador normalmente ignora el comando. El resultado en papel es una prueba física pendiente.
- **Cola de impresión** con altura limitada, el detalle del trabajo a la vista, sin datos técnicos y con los trabajos repetidos agrupados; «Mesa» ya no se repite («Mesa Mesa 1») en la precuenta ni en Cocina.
- **Terminal de cocina (KDS básico)**: cada terminal muestra todas las estaciones o solo las suyas (Cocina → Estaciones → Terminales de cocina, con aviso sonoro opcional) y cada usuario atiende todas o las suyas (Personal → Usuarios → Estaciones de cocina); se ve lo que tienen en común. **Preparación** pasa a modo cocina: tarjetas grandes, botones de 64 px, los pedidos más antiguos primero, ámbar «Por vencer» y rojo «Atrasada» según el tiempo objetivo de la estación, y sin «Actualizar». Un combo reparte sus componentes entre estaciones; los complementos viajan con su producto. Guía paso a paso: [docs/TERMINAL_DE_COCINA.md](docs/TERMINAL_DE_COCINA.md).

Los eventos utilizan SSE autenticado desde Rust, con outbox, cursor, replay, deduplicación y recuperación de estado completo. Se prepararon contratos de recepción/estado para el futuro KDS; la etapa 9F agregó el KDS básico dentro del POS (arriba), pero no existe una aplicación KDS independiente; desde la 9G sí existe el agente de impresión de las terminales (impresoras USB de otra computadora). Consulta [la guía técnica y operativa](docs/KITCHEN_AND_PRINTING.md) y [la evidencia de verificación](docs/VERIFICATION.md).

## Catálogo y precios

En **Administración → Catálogo** administra categorías, productos independientes, presentaciones opcionales, ingredientes, complementos, porciones, grupos reutilizables, combos, listas de precios y promociones. Incluye imágenes opcionales, horarios, archivo lógico, búsqueda, filtros y paginación. La edición rechaza revisiones obsoletas de otra terminal.

En **Probar personalización** retira o sustituye todos los complementos, cambia cantidades, ingredientes, componentes y observaciones. Go valida y devuelve el desglose exacto; no se crea una venta. Las listas conservan la identidad del producto y las promociones explican su aplicación o exclusión. Los PNG originales locales cambian con el tema.

Desde la etapa 9A cada producto puede activar **«El precio puede bajar al cambiar complementos»** (desactivada por omisión): sin ella, quitar o cambiar un complemento nunca baja el precio. Consulta [docs/CATALOG.md](docs/CATALOG.md) para reglas y ejemplos reproducibles: filete $150 → $155 al cambiar arroz por papas, $150 → $140 con la opción al cambiar arroz por frijoles, y combo con papas $90 / $100 / $140. La migración de catálogo es `0003_catalog_and_pricing.sql`; la etapa 4 añade `0004_restaurant_operations.sql`, la etapa 5 `0005_kitchen_and_printing.sql`, la etapa 6 `0006_payments_and_refunds.sql`, la etapa 6.1 `0007_features_product_tax_and_usd_cash.sql`, la etapa 7 `0008_cash_shifts_and_reconciliation.sql`, la etapa 7.1 `0009_operation_tables_cash_and_reasons.sql` la etapa 8 `0010_inventory_core.sql` a `0014_inventory_control.sql` (aplicadas en la instalación real) la etapa 9A `0015_catalog_price_and_precheck.sql`, `0016_inventory_suppliers_counter.sql` y `0017_reports.sql` (también aplicadas en la instalación real) la etapa 9B `0018_document_templates.sql` y `0019_document_identity.sql` (aplicadas en la instalación real el 11 de noviembre de 2026) la etapa 9C `0020_terminal_home_and_printer_columns.sql` (aplicada en la instalación real el 11 de noviembre de 2026) y la etapa 9D `0021_account_cancellation.sql` (aplicada en la instalación real el 12 de noviembre de 2026). Las migraciones anteriores se conservan. Antes de migrar una instalación existente, generar respaldo con `./scripts/dev.sh backup` y ensayar su restauración en una base temporal.

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
│   ├── internal/maintenance/   # «Quitar datos de prueba»: firmas exactas, plan y respaldo previo (etapa 9F)
│   ├── internal/stationscope/  # Estaciones que se ven en Cocina: terminal ∩ usuario (etapa 9F)
│   ├── internal/standalone/    # Carpeta de datos, clúster propio, respaldos, modalidad y paso de datos del servicio (Windows 9E; macOS 9G)
│   ├── internal/lanlistener/   # Escucha HTTPS de la red local con filtro de orígenes (etapa 9G)
│   ├── internal/discovery/     # Anuncio del servidor en la red local, _adelpos._tcp (etapa 9G)
│   ├── internal/backupschedule/ # Respaldos programados (etapa 9G)
│   ├── internal/apicontract/   # Contrato OpenAPI de la API v1 para clientes y sus pruebas (etapa 9G)
│   ├── internal/idempotency/   # Idempotencia para módulos sin una propia (etapa 9G)
│   ├── internal/httpapi/       # HTTP, diagnósticos, CORS y errores
│   ├── internal/localsetup/    # Certificados locales e identidad propia del servidor (etapa 9G)
│   └── migrations/             # 0001 a 0023 (0001–0022 aplicadas en la instalación real e inmutables; la 0023 de la 9G, inmutable en cuanto se aplique)
├── desktop/src/
│   ├── components/             # Formularios, permisos, selector único, selector de hora y presentación reutilizables
│   ├── navigation/             # Registro de secciones, barra superior, mosaicos y ruta (etapa 9C)
│   ├── commands/               # Registro de acciones, despachador, atajos, menú nativo y paleta (etapa 9C)
│   ├── hooks/                  # Sesión, recursos y conectividad
│   ├── pages/                  # Administración y operación del restaurante
│   └── services/               # Contratos tipados y puente API
├── desktop/src-tauri/src/       # Rutas enumeradas, TLS con huella fijada y emparejamiento, búsqueda en la red, modalidades, agente de impresión, credenciales (Keychain o Administrador de credenciales), diálogos nativos, menú nativo y servidor local de Windows
├── config/                     # Identidad canónica y ejemplos sin secretos
├── scripts/                    # Desarrollo, formato, procesos y PostgreSQL (dev.sh y build-macos-pkg.sh con macos-pkg/ en macOS; dev.ps1 y build-windows.ps1 en Windows)
├── tests/                      # Pruebas operativas macOS
├── ops/launchd/                # Plantilla del servicio LaunchDaemon que instala el .pkg (etapa 9G)
└── docs/                       # Convenciones, dependencias y evidencia
```

`.local`, `.tools`, cachés, respaldos y artefactos están excluidos del repositorio. Node/Rust son herramientas de construcción; el `.app` funciona sin Vite. PostgreSQL y Go son procesos independientes de la ventana (desde la 9G, en la computadora principal, un servicio del sistema).

## Verificación

```bash
./scripts/dev.sh format-check
./scripts/dev.sh test
./scripts/dev.sh test-integration
./scripts/dev.sh test-browser
```

Las pruebas de integración crean y eliminan únicamente bases aleatorias `adel_pos_test_*`. No borran `adel_pos`. Desde la etapa 9F las pruebas de navegador corren contra un servidor en **modo de pruebas** (`test_mode`), que se niega a arrancar contra una base que no sea de pruebas, y el puente de las pruebas se niega a correr sin él. Consulta los resultados y límites reales en [docs/VERIFICATION.md](docs/VERIFICATION.md).

## Documentación

- [DEVELOPMENT_MACOS.md](DEVELOPMENT_MACOS.md): preparación, ejecución, terminales, TLS y recuperación local.
- [DEVELOPMENT_WINDOWS.md](DEVELOPMENT_WINDOWS.md): herramientas, comandos, pruebas e instalador en Windows (etapa 9E).
- [docs/INSTALAR_EN_WINDOWS.md](docs/INSTALAR_EN_WINDOWS.md): guía corta para instalar, respaldar y desinstalar en Windows.
- [ARCHITECTURE.md](ARCHITECTURE.md): responsabilidades y límites.
- [API_CONTRACTS.md](API_CONTRACTS.md): rutas y permisos.
- [docs/CATALOG.md](docs/CATALOG.md): reglas comerciales, uso y límites del catálogo.
- [docs/RESTAURANT_OPERATIONS.md](docs/RESTAURANT_OPERATIONS.md): flujo de pedidos, unidades y concurrencia.
- [docs/PAYMENTS_AND_REFUNDS.md](docs/PAYMENTS_AND_REFUNDS.md): cobros, IVA, descuentos, devoluciones y tickets.
- [docs/CASH_SHIFTS_AND_RECONCILIATION.md](docs/CASH_SHIFTS_AND_RECONCILIATION.md): caja, arqueos, cierre ciego, cortes, documentos, cajón y tope.
- [docs/INVENTORY_AND_PURCHASING.md](docs/INVENTORY_AND_PURCHASING.md): inventarios, recetas, consumo, costos, compras, ajustes y conteos.
- [docs/REPORTS.md](docs/REPORTS.md): reportes, métricas, fechas comerciales, permisos y exportaciones.
- [docs/NAVIGATION_AND_SHORTCUTS.md](docs/NAVIGATION_AND_SHORTCUTS.md): barra superior, Administración por mosaicos, inicio por terminal, selector, menú nativo, atajos y paleta de comandos.
- [docs/DOCUMENT_DESIGNER.md](docs/DOCUMENT_DESIGNER.md): identidad del establecimiento, logotipo, plantillas, editor visual, versiones e impresión de documentos.
- [docs/NOTAS_WINDOWS.md](docs/NOTAS_WINDOWS.md): qué depende de cada sistema, qué se hizo en Windows en la etapa 9E, qué falta verificar allí de la 9F y qué hacer en la Fase W de la 9G.
- [docs/TERMINAL_DE_COCINA.md](docs/TERMINAL_DE_COCINA.md): cómo configurar una terminal de cocina por estaciones (etapa 9F).
- [docs/INSTALAR_EN_RED.md](docs/INSTALAR_EN_RED.md): guía corta para instalar la computadora principal y las terminales, emparejarlas, revocar una terminal y cuidar los respaldos (etapa 9G).
- [docs/API_V1.md](docs/API_V1.md): contrato de la API v1 para clientes y la futura aplicación Android (etapa 9G).
- [docs/CODING_CONVENTIONS.md](docs/CODING_CONVENTIONS.md): reglas permanentes de legibilidad.
- [NEXT_STEPS_FOR_SOL.md](NEXT_STEPS_FOR_SOL.md): continuidad y decisiones pendientes.

La distribución comercial sigue bloqueada hasta integrar licenciamiento. Firma/notarización Apple, firma digital de Windows, PostgreSQL dentro del instalador de macOS y actualizaciones automáticas siguen pendientes; el instalador de Windows de la etapa 9E y el `.pkg` de macOS de la 9G son de prueba (sin firma). El bundle ID `local.adel.pos.dev` es provisional y no representa un dominio registrado.
