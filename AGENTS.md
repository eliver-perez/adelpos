# Instrucciones para continuar

Leer README.md, ARCHITECTURE.md, API_CONTRACTS.md y NEXT_STEPS_FOR_SOL.md antes de ampliar funcionalidad.

## Convenciones permanentes de legibilidad

- Está prohibido ofuscar, minificar o compactar el código fuente para reducir líneas.
- Go: desarrollar funciones y bloques if/else/for/switch en múltiples líneas, con llaves e indentación de gofmt. No concentrar condiciones y operaciones completas en una línea.
- React/JSX/TSX: cada etiqueta anidada debe tener su propia línea e indentación clara, con aperturas y cierres identificables.
- TypeScript/JavaScript: separar operaciones y expandir condiciones, objetos y expresiones complejas. Revisar manualmente lo que Prettier no expanda.
- Respetar indentación consistente: tabulaciones de gofmt en Go; dos espacios con Prettier en archivos compatibles; convenciones propias de Rust, Python y shell.
- Usar nombres descriptivos, funciones con responsabilidad clara y componentes mantenibles; evitar archivos o funciones gigantescos.
- Escribir comentarios explicativos en español para reglas, validaciones y lógica compleja; evitar comentarios redundantes.
- Ejecutar gofmt y Prettier antes de entregar; preservar el comportamiento durante el reformateo.
- La claridad y mantenibilidad tienen prioridad sobre reducir el número de líneas. Estas reglas rigen todas las etapas futuras.

- Mantener Go independiente de Tauri y toda operación SQL en backend.
- No introducir módulos comerciales fuera del alcance solicitado ni datos ficticios.
- No leer/imprimir secretos de .local ni modificar clústeres ajenos. No versionar .local, .tools, cachés o builds.
- Mantener localhost por defecto, CSP y capabilities restringidas.
- Metadatos desde config/product.json; ejecutar scripts/dev.sh sync al cambiarlos.
- Migraciones aplicadas inmutables. Añadir pruebas significativas de transacciones/contratos cuando cambien.
- Verificar con scripts/dev.sh test y, para cambios de persistencia, test-integration contra la base temporal del proyecto.
- Actualizar la documentación con lo realmente ejecutado, sin presentar tareas pendientes como completas.

## Reglas confirmadas en etapa 2

- El nombre inicial del establecimiento es ADEL POS; proponer la zona horaria del sistema operativo y permitir editarla. No cambiar la marca técnica por inferencia.
- admin/admin es una política funcional aprobada: crear una sola vez, advertir sin obligar al cambio y no restablecer en reinicios/migraciones.
- sa es reservado y no editable desde administración ordinaria. Su contraseña solo la define privadamente el propietario en Terminal local; no inventarla ni pedirla por chat.
- Los secretos de sesión/dispositivo solo se conservan en el almacén seguro nativo. Auditar cambios transaccionalmente y no registrar contraseñas, hashes ni tokens.
- Consultar dispositivos, sesiones y permisos vigentes en Go en cada operación protegida.
- Delegación segura (corrección previa a etapa 6): nadie concede ni asigna permisos efectivos que no posee, administra a usuarios con más permisos ni modifica su propio rol o excepciones. Debe permanecer un administrador completo por permisos efectivos. Toda autorización ocurre antes de la primera escritura; además, un 403 revierte la operación mediante savepoint y solo confirma la auditoría de la denegación.

## Reglas confirmadas en etapa 3

- Catálogo y dinero se calculan exclusivamente en `internal/catalog` con decimal exacto. Dinero JSON como cadenas; no introducir cálculos financieros con number en React.
- Conservar equivalencia por valor con piso en base, extras explícitos separados y tramos incrementales. Nunca imponer una opción original obligatoria si el mínimo es cero.
- Combos referencian productos estables; no duplicar identidades ni consumo. Validar ciclos, disponibilidad y personalización de hijos.
- Respetar revisión optimista y lock de catálogo; cada escritura y su auditoría pertenecen a la misma transacción. No eludir permisos de precios/estado/disponibilidad/imágenes mediante el editor completo.
- 0003 aplicada es inmutable. La previsualización no registra ventas ni movimientos de inventario. Leer docs/CATALOG.md antes de ampliar reglas.
- Preservar los PNG originales de assets/brand; BrandLogo elige por tema y conserva proporciones. No sustituir la marca ni confundirla con el logotipo futuro del restaurante.

## Regla visual permanente desde etapa 4

- Utilizar components/Accordion para secciones colapsables. Encabezado completo con título, resumen opcional y chevron; tarjeta discreta, temas claro/oscuro y transición con movimiento reducido.
- Mantener controles montados al cerrar, conservar valores/errores y revelar automáticamente errores de formulario. Preservar relaciones aria y navegación por teclado. No reintroducir details/summary sin este patrón ni convertir indiscriminadamente todas las pantallas en acordeones.
- Pedidos confirmados conservan snapshots y unidades estables. Transferir o dividir no recalcula precios ni genera preparación. No afirmar envío a cocina, pago o entrega hasta contar con sus módulos reales.

## Continuidad de etapa 4

- 0001–0005 ya están aplicadas y son inmutables; la 0006 (etapa 6) también lo es una vez aplicada. La siguiente ampliación del esquema requiere 0007. Leer docs/RESTAURANT_OPERATIONS.md, docs/KITCHEN_AND_PRINTING.md y docs/VERIFICATION.md.
- Toda escritura operativa conserva UUID, revisión, resultado idempotente, auditoría y eventos en la misma transacción. No regenerar UUID al reintentar una respuesta incierta.
- Observaciones y reducciones de receta idéntica conservan importes de unidades históricas; otros cambios requieren cotización y aceptación explícitas. No recalcular automáticamente pedidos anteriores por cambios de catálogo o promociones.
- Ventanilla pertenece a takeaway. Mantener preparación, pago y entrega independientes. No agregar una acción de entregar sin el futuro cobro real.
- El borrador persistido y el editor local son estados distintos. No descartar selecciones ante sondeo o conflicto. No prometer recuperación offline después de cerrar el proceso.
- Las pruebas visuales usan solo bases adel_pos_test_* y transporte IPC de prueba exclusivo de Node. No introducir un acceso web alternativo en producción ni pasar tokens al WebView.

## Regla permanente desde etapa 5

- Iniciar en pantalla completa nativa mediante Tauri, con salida/regreso visibles y modo ventana normal. No imponer quiosco ni bloquear controles o Spaces de macOS.
- 1280×720 es un escenario de prueba; no un ancho/alto fijo del producto. Usar todo el monitor con distribuciones fluidas y paneles prácticos; conservar formularios y selecciones al redimensionar.
- La liberación canónica es una por UUID de unidad comercial en kitchen_unit_releases; estaciones/copias no crean consumo adicional. No registrar inventario físico todavía.
- Conservar rutas/documentos históricos. Modificaciones bloqueadas permanecen en kitchen_pending_changes; no aparentar preparación lista ni avanzar una versión obsoleta.
- La cola confirma intento/lease antes del transporte y serializa por impresora. Un resultado desconocido exige resolución explícita; nunca reenviarlo automáticamente ni activar respaldo por incertidumbre.
- submitted significa aceptación CUPS/TCP, no salida física de papel. Reimprimir conserva presentación sin leyendas adicionales y bloquea otras copias sin resolver.
- El planificador y los trabajadores pertenecen a Go independiente. SSE es el equivalente elegido a WebSocket: outbox, cursor/replay/reset, permisos por estación y Channel nativo sin tokens en React. Conservar HTTP idempotente para comandos/acuse KDS.
- TestControlledCUPSPrinter nunca imprime en suites normales. El envío de hardware necesita destino explícito y ADEL_HARDWARE_SEND=1; no activar en CI ni dirigir a la impresora predeterminada por inferencia.

## Reglas permanentes desde etapa 6

- `catalog` es la única autoridad de precios; `payments` es la única autoridad financiera (saldos, pagos, asignaciones, cambio, descuentos manuales, cortesías, cargos, propinas, IVA, devoluciones, correcciones, turno mínimo, documentos comerciales y entregas). `payments` no importa `operations` ni `administration`; `operations` solo consulta sus guardas.
- 0001–0006 están aplicadas en la instalación real y son inmutables. Toda migración futura sobre la instalación real requiere autorización expresa del propietario, previo respaldo y ensayo de restauración.
- Los registros financieros son inmutables: una corrección, cancelación o devolución es un registro compensatorio enlazado; nunca se edita ni borra un pago, ajuste o devolución.
- Dinero con `numeric`/`shopspring/decimal` en Go y cadenas en JSON. React no suma, resta ni redondea dinero: muestra lo que devuelve Go y envía la cadena capturada. Los importes se capturan con `inputMode="decimal"` y validación estricta; nunca con `type="number"`.
- El repartidor de centavos (`payments.allocate`) es determinista y conserva el total exacto. El IVA está incluido en los precios: la tasa del establecimiento es una lista cerrada (16, 8 o 0) elegida con botones, se copia a cada versión confirmada y no se recalcula al cambiar la configuración.
- Una unidad con asignación neta de pago o con descuento manual vigente no se transfiere, divide, combina, cancela, edita ni anula. Una cuenta nunca queda con total menor que lo pagado.
- Toda escritura de `payments/` toma el lock 7820148305 exclusivo, guarda su resultado en `restaurant_operations` y audita en la misma transacción. La huella de idempotencia excluye `authorization.password`; un reintento conserva el mismo `operation_id`.
- La autorización de supervisor es puntual: valida usuario y contraseña con el límite de intentos del inicio de sesión, no crea sesión, rechaza la autoautorización y nunca se conserva en React (el almacén de operaciones pendientes descarta la contraseña).
- Cobrar exige turno abierto en la caja de la terminal. Desde la etapa 7 el arqueo y el cierre existen: no presentar `expected_cash` como arqueo ni entregarlo sin `cash.expected.read`.
- Precuentas y tickets de pago son documentos comerciales: viven en la cola de impresión con `kind` `precheck`/`receipt`, no aparecen entre las comandas, no constituyen CFDI y un fallo de impresión nunca revierte un pago.

## Reglas permanentes desde etapa 6.1

- 0001–0007 están aplicadas en la instalación real y son inmutables; toda migración sobre la instalación real requiere autorización expresa previa con respaldo y ensayo de restauración. Un CHECK que evalúa a NULL se acepta en PostgreSQL: las restricciones con columnas opcionales deben exigir `IS NOT NULL` explícito en la rama que valida datos.
- Nunca usar `window.confirm`, `window.alert` ni `window.prompt`: el WebView nativo de macOS no los muestra. Usar `components/ConfirmDialog.tsx` o formularios dentro de la pantalla.
- Campos numéricos con `services/numericInput.ts` (importes 2 decimales, cantidades 3, enteros solo dígitos) o el teclado de cobro; nunca `type="number"`. Fechas opcionales con `OptionalDateField` (el control nativo de WebKit muestra la fecha de hoy con el valor vacío).
- Orden visual por posiciones (`position: {after}`) con `internal/ordering`; no volver a capturar números de orden. Colores de categoría solo de la paleta de Go.
- Funciones activables: registrar la función en `internal/features`, hacerla cumplir en Go aunque la interfaz la oculte y no borrar ni alterar registros al desactivarla. El pago en dólares (`usd_cash`) está desactivado por defecto; con él desactivado el cobro es idéntico al de la etapa 6.
- IVA por producto: NULL hereda la tasa general; cada versión confirmada guarda la tasa efectiva. Dólares: solo efectivo, tipo de cambio fijo por turno, conversiones redondeadas a centavos en Go y devoluciones solo en pesos.
- Captura táctil: un solo flujo de pedidos (borrador persistido + confirmación con huella). El borrador se sincroniza con antirrebote y una sola petición en vuelo; no crear una petición por toque ni un segundo flujo.

## Reglas permanentes desde etapa 7

- La 0008 (`0008_cash_shifts_and_reconciliation.sql`) no está aplicada en la instalación real hasta que el propietario lo autorice; una vez aplicada es inmutable y la siguiente ampliación requiere 0009. 0001–0007 siguen inmutables.
- La caja vive en `payments` (`cash_*.go`): un turno activo por caja (`open` o `closing`), escrituras con el lock 7820148305, idempotencia, auditoría y evento `cash` en la misma transacción. El efectivo esperado se calcula en un solo lugar (`cash_expected.go`) a partir de registros reales y por moneda.
- Cierre ciego para todos: durante `closing` ninguna respuesta, documento, aviso ni auditoría contiene el esperado o las diferencias, ni siquiera para el administrador; fuera del arqueo solo con `cash.expected.read`. Los rechazos por efectivo insuficiente nunca llevan cifras.
- Retiros y gastos siempre autorizados (permiso propio `cash.outflows.authorize` o autorización puntual de otra persona). Las correcciones son compensatorias y las regularizaciones de un turno cerrado van al turno abierto vigente de la misma caja, sin cambiar la diferencia del cerrado.
- Imprimir o abrir el cajón nunca revierte ni bloquea una operación o un cierre: se ejecutan en un punto de guardado y la respuesta informa su estado. El cajón solo se abre por ESC/POS TCP y nunca por pagos exclusivamente electrónicos; sus comandos no se reintentan solos. Mensajes: «Comando de apertura enviado», nunca «cajón abierto»; «enviado», nunca «impreso».
- El tope de caja (`cash_limit`) es solo un aviso para quien ve el esperado y nunca bloquea cobros.
