# Cocina, comandas e impresión · Etapa 5

Versión 0.9.3, esquema 21 (la migración `0021_account_cancellation.sql` de la etapa 9D no cambia tablas de cocina y se aplicó a la instalación real el 12 de noviembre de 2026; la cancelación de cuentas completas llega a cocina como las cancelaciones de partidas, ver «Cancelar cuenta y «Enviado · sin confirmar en cocina» · etapa 9D»; la migración `0020_terminal_home_and_printer_columns.sql` de la etapa 9C agrega las columnas de texto por impresora y se aplicó a la instalación real el 11 de noviembre de 2026 con la autorización previa del propietario; ver «Columnas por impresora y prueba de ancho · etapa 9C»; la migración 0009 de la etapa 7.1 se aplicó a la instalación real el 9 de noviembre de 2026; ver «Cajón por CUPS en modo raw · etapa 7.1»; las 0010–0014 de la etapa 8 y las 0015–0017 de la etapa 9A se aplicaron el 10 de noviembre de 2026; las 0018 y 0019 de la etapa 9B se aplicaron a la instalación real el 11 de noviembre de 2026 con autorización del propietario). Desde la etapa 9B todos los documentos impresos salen de un renderizador único con plantillas por bloques, versiones e identidad del establecimiento, sin cambiar enrutamiento, cola ni reglas de resultado (ver «Plantillas, formato y logotipo · etapa 9B» y [DOCUMENT_DESIGNER.md](DOCUMENT_DESIGNER.md)). La etapa 9A no cambia la cocina: los reportes de demanda por producto y por hora y de preparación por estación leen las liberaciones canónicas con su fecha de liberación, en una transacción de solo lectura y sin el lock operativo ([REPORTS.md](REPORTS.md)). Go es la autoridad de preparación y ejecuta los trabajadores sin depender de una ventana Tauri. No existe aplicación KDS independiente. Desde la etapa 8 la liberación canónica es el origen del consumo de inventario (ver «Consumo de inventario · etapa 8»). La preparación lista no significa pagado o entregado: pago y entrega pertenecen a `payments` (etapa 6).

## Configuración inicial

Abrir **Cocina y preparación** con permisos administrativos. No se crean estaciones, rutas, impresoras ni pedidos ficticios en la instalación real.

1. En **Impresoras**, descubrir las colas CUPS del servidor o registrar un destino manual. Seleccionar ancho 58/80 mm, 1–3 copias y capacidad de corte real. El destino corresponde al equipo donde corre Go.
2. En **Estaciones**, crear nombre, descripción, orden, estado, tiempos objetivo/advertencia/atraso y recepción por impresora, KDS o ambos. Varias estaciones pueden seleccionar la misma impresora.
3. En **Enrutamiento**, asignar categoría y, si procede, una excepción por producto. Un producto puede llegar a varias estaciones. Seleccionar explícitamente **Sin preparación** para artículos que no deban generar trabajo.
4. En **Comandas y tiempos**, elegir documentos independientes o unificados, seguimiento por partida o pedido, formato y política de programación. Una estación puede sobrescribir agrupación y seguimiento.
5. En Estaciones, limitar las terminales a sus estaciones. Un usuario sin `kitchen.stations.all` solo puede consultar/operar el trabajo asignado a su dispositivo; además necesita cada permiso funcional. El listado administrativo de configuración conserva su propio permiso.

La primera configuración no altera contraseñas, catálogo, cuentas históricas ni credenciales del Llavero. Los veinte permisos nuevos se conceden inicialmente al rol administrador. Los otros roles necesitan asignación explícita. La identidad reservada `sa` conserva su política anterior.

### Datos de prueba · etapa 9F

Las pruebas automáticas crean un destino de aceptación que nunca imprime («Destino controlado E2E», transporte de
agente, destino «Agente de aceptación no disponible»), estaciones «Cocina caliente» y «Freidoras» con ese destino y la
ruta de «Platillos». Desde la 9F nunca llegan a una instalación real: el servidor en modo de pruebas (`test_mode`) se
niega a arrancar contra una base que no sea `adel_pos_test_*`, las pruebas de navegador exigen ese modo y la prueba de
humo de Windows abre la aplicación con una carpeta de datos temporal y `ADEL_TEST_MODE=1`.

Lo que ya quedó en una instalación se quita en **Administración › Sistema › Mantenimiento › «Quitar datos de prueba»**
(`settings.manage`): «Revisar datos de prueba» muestra lo que se elimina, lo que se desactiva porque ya tiene
operaciones registradas (trabajos de impresión, tareas de cocina, ventas), la configuración que se desvincula (impresora
de tickets de Cobros, cajón, impresora de respaldo, estaciones por terminal y rutas) y lo que se conserva, cada uno con
su motivo. Solo se reconocen las firmas exactas de los archivos de prueba (nombre más atributos, lista cerrada en
`backend/internal/maintenance/signatures.go`): una estación «Cocina caliente» con otra impresora, o una categoría
«Platillos» con productos del restaurante, no se tocan. Al confirmar, Go crea primero un respaldo de la base (si no
puede, no quita nada), aplica todo en una transacción y lo audita; el aviso dice lo quitado y el nombre del respaldo.
Ventas, pagos, cortes y movimientos de inventario nunca cambian, así que una instalación de Windows que operó con la
prueba de humo conserva su turno y sus ventas: para dejarla limpia hay que reinstalar con otra carpeta de datos.

## Enrutamiento y liberación

La ruta de producto prevalece sobre categoría. Los componentes elegidos de un combo se resuelven recursivamente con sus identidades de catálogo; no se duplican productos ni unidades comerciales. La expansión conserva variantes, porciones, ingredientes, grupos y observaciones. Una ruta ausente, una estación inactiva o un destino inválido produce advertencias visibles y una liberación bloqueada; nunca un envío aparente. Un combo vacío sin ruta tampoco se interpreta como exención implícita.

Guardar o previsualizar un borrador no crea comandas. Confirmar registra el pedido y su intención durable de preparación en la misma transacción. Un pedido inmediato intenta liberarse; la ausencia de configuración conserva el pedido y muestra qué corregir. Después de corregir, usar **Liberar** con la revisión actual.

`kitchen_unit_releases` es el punto canónico de liberación para inventario: una fila por UUID de unidad comercial, con versión y liberación. Las tareas por estación y las copias de impresión no representan ventas o consumos adicionales. La etapa 5 no descontaba existencias; desde la etapa 8 cada fila canónica origina, con el control de inventario activado, una sola solicitud de consumo (ver «Consumo de inventario · etapa 8»).

Al liberar se congelan ruta, estaciones, contexto y documento; una edición de configuración posterior no altera la comanda histórica. Dividir, transferir, combinar o mover cuentas conserva las unidades y no vuelve a liberar ni imprimir. Las consultas de preparación siguen la pertenencia actual de esas unidades; los documentos originales conservan su contexto histórico.

## Programación

Las cuentas para llevar admiten hora local en la zona IANA configurable del establecimiento. El valor inicial de zona proviene del sistema operativo; no se fija America/Monterrey en código. Go valida la hora y conserva instantes UTC. Una hora local inexistente se rechaza; en una hora ambigua se utiliza la resolución estándar de Go, sin selector de ocurrencia.

La política se captura por liberación: automática o manual y minutos de anticipación. `due_at` resta la anticipación al instante programado, tanto si la referencia de la cuenta es preparación como entrega; usar anticipación cero si se desea liberar exactamente a esa hora. El trabajador Go revisa vencimientos cada segundo, incluso sin Tauri. Reiniciar no duplica liberaciones gracias a marcadores canónicos y locks transaccionales.

La política automática está desactivada inicialmente; anticipación inicial 25 minutos. Cambiar la configuración general no reescribe pedidos programados existentes. Se puede actualizar una programación pendiente con revisión o liberar manualmente. Una reprogramación hecha en cocina se conserva aunque después se editen otros datos de la cuenta (teléfono, notas, responsable); solo una nueva programación capturada en la propia cuenta la sustituye. Adelantar una liberación requiere también `kitchen.release_early`.

## Documentos

Cada comanda tiene UUID, fecha/hora, cuenta/folio, pedido, empleado y contexto de mesa/área o para llevar. Las secciones identifican estación y las partidas incluyen cantidades, personalización, notas y UUID estable. Cuando una partida agrupa varias unidades, el detalle indica que corresponde a cada unidad. Los precios se omiten inicialmente y solo se incluyen al habilitarlos expresamente; los cálculos y cadenas monetarias proceden de Go.

- Los productos confirmados después generan una **COMANDA** normal, sin etiqueta ADICIÓN.
- Una edición de preparación genera **MODIFICACIÓN**, con ANTES/AHORA. Las tareas afectadas vuelven a preparación pendiente; no se repite una liberación comercial.
- Una cancelación genera **CANCELACIÓN**, identifica cantidad/motivo y cancela las tareas correspondientes.
- Una reimpresión conserva la presentación histórica, sin leyenda REIMPRESIÓN y sin modificar preparación.

Las modificaciones antes de la primera liberación se integran en el documento inicial. Después de liberar, se utilizan las rutas históricas; un componente nuevo explícito puede necesitar una ruta actual adicional. Si una estación histórica está inactiva, se conserva el cambio en `kitchen_pending_changes` y se muestra **Modificación pendiente**. No se permite avanzar tareas con una versión anterior. Reactivar/corregir y usar **Revisar cambio pendiente**. El lote afectado se conserva completo hasta poder resolver sus destinos; no se envían avisos parciales silenciosos.

Documentos independientes: una comanda por estación aunque compartan impresora. Unificación: estaciones compatibles que seleccionan la misma impresora y agrupación unificada comparten documento, manteniendo secciones. Las estaciones KDS conservan su propia tarea y requisito de recepción, aunque compartan papel con otras.

El formato admite nombre del establecimiento, notas, precios, texto normal/grande, separación 1–3 y cantidad delante o multiplicador. Los controles de usuario, ESC/GS y caracteres invisibles se eliminan del texto; solo el controlador genera comandos. No existe diseñador visual arbitrario ni ejecución de plantillas: desde la etapa 9B la presentación se configura con plantillas de bloques de una lista cerrada (sección «Plantillas, formato y logotipo · etapa 9B»), y estas opciones de Cocina siguen copiándose en cada comanda y valiendo como «Según Cocina».

Indicaciones rápidas por categoría (etapa 7.1): la captura las guarda como texto en las observaciones de la partida (`customization.notes`), unidas por comas con las escritas, por lo que la comanda las imprime en la misma línea «NOTA:» (ajustada al ancho del papel) sin ningún cambio en el enrutamiento, la plantilla ni la cola. Administración en [CATALOG.md](CATALOG.md).

Indicaciones con efecto (etapa 8, migración 0012): la partida guarda además las indicaciones resueltas en su desglose y cada parte de la comanda (`instructions.parts[].quick_notes`, omitido sin indicaciones) las recibe del enrutamiento. Una indicación con efecto («Sin cebolla», «Queso extra») ya fija la cantidad del ingrediente, así que la plantilla **imprime la indicación una sola vez**: si su texto está entre las indicaciones de la nota impresa (la de la partida para la parte principal, la del componente para cada componente de combo), se omite la línea estructurada de ese ingrediente («SIN Cebolla», «Queso: 1 (incluido 0)»). Si la comanda no muestra notas o la nota ya no contiene el texto, la línea estructurada se conserva para que cocina no pierda el cambio. Sin indicaciones con efecto la comanda es idéntica a la de la etapa 7.1 (prueba de plantilla con el texto exacto). El resumen de la partida en la captura aplica la misma regla. La pantalla de preparación no lista ingredientes, así que no cambia.

## Transportes e impresoras

**CUPS**: usa `lpstat`, `lpoptions` y `lp` locales mediante argumentos separados, sin shell. Descubre nombres de colas y opciones; una consulta fallida informa indisponibilidad, no una lista vacía ficticia. Envía archivo temporal privado UTF-8, ancho personalizado, tamaño de letra, copias y título técnico. Lo elimina al terminar. El corte exige una opción admitida por el controlador (`CutMedia`, `CutType` o `Cutter`), capacidad declarada y activación explícita.

**ESC/POS TCP**: dirección IP privada/loopback explícita, puertos 9100–9109 y tiempo límite. Codifica acentos en CP850; caracteres no representables se sustituyen. Inicialización, tamaño y corte pertenecen al controlador aislado. No acepta comandos introducidos en notas. Una conexión fallida antes de escribir es error confirmado; una escritura parcial o sin resultado concluyente es incierta. La aceptación TCP tampoco confirma papel.

**Agente de impresión**: contrato de destino futuro. Puede registrarse, pero los trabajos quedan en intervención con explicación. No se finge un envío ni se ejecuta un agente remoto.

No hay redirección automática a la impresora predeterminada del sistema. Solo se utiliza un respaldo configurado explícitamente, sin ciclos, después de tres fallos confirmados consecutivos previos al envío y una sola vez por trabajo. Un resultado incierto nunca activa ese respaldo: si un intento incierto o en intervención aparece entre los tres últimos (por ejemplo, antes de un envío autorizado manualmente), la cuenta vuelve a empezar y se requieren tres errores confirmados seguidos.

Referencias oficiales: [lp](https://www.cups.org/doc/man-lp.html), [lpstat](https://www.cups.org/doc/man-lpstat.html), [opciones CUPS](https://www.cups.org/doc/cupspm.html). La prueba en hardware y sus límites constan en [VERIFICATION.md](VERIFICATION.md).

## Cola durable y protección contra duplicados

| Estado         | Significado y acción                                                                                   |
| -------------- | ------------------------------------------------------------------------------------------------------ |
| `pending`      | Documento guardado; espera trabajador.                                                                 |
| `processing`   | Intento y lease persistidos antes del transporte. Esperar resultado.                                   |
| `submitted`    | CUPS/TCP aceptó el envío, o un operador reconcilió aceptación conocida. No significa papel confirmado. |
| `error`        | Fallo confirmado sin envío; reintento limitado o intervención.                                         |
| `uncertain`    | Pudo haberse recibido. Revisar físicamente y resolver con motivo. Nunca reintentar automáticamente.    |
| `intervention` | Destino desactivado, agente no implementado u otra condición que exige revisión.                       |
| `cancelled`    | El operador canceló este trabajo de impresión; no cancela el pedido.                                   |

Dos trabajadores de impresión corren en Go. La selección utiliza `FOR UPDATE SKIP LOCKED`; un advisory lock de sesión por impresora permanece durante el transporte. Los trabajos de una misma impresora no se intercalan, incluso entre trabajadores de distintos procesos conectados a esa base. El intento y la lease de 60 segundos se confirman antes de cualquier envío externo. El transporte tiene límite de 12 segundos. No se conserva una transacción abierta durante la impresión.

Al caer el proceso, un trabajo `processing` cuya lease venció pasa a `uncertain`; no se reenvía. Los errores confirmados se reintentan como máximo tres veces automáticamente con espera creciente. Se registra cada intento, resultado, referencia de transporte y última falla. Si Go pierde PostgreSQL después del envío, la reconciliación posterior es conservadora.

La vista **Cola de impresión** permite consultar documento e intentos, reintentar errores conocidos, cancelar, reconocer aceptación conocida o autorizar conscientemente otro envío. Cada intervención requiere permiso, revisión y motivo de 5–500 caracteres. Cambiar de impresora exige además `kitchen.printers.manage`.

Una copia adicional usa un trabajo nuevo enlazado al original. Solo se solicita después de finalizar o resolver el anterior; se rechaza otra copia mientras exista alguna pendiente/incierta de esa misma familia. Toda escritura lleva UUID idempotente, ligado a cuerpo, ruta, actor y terminal. Repetir el mismo UUID recupera la respuesta original; no se debe regenerar tras una respuesta de red incierta.

**Límite físico:** una base transaccional y una impresora no comparten commit. Si el transporte acepta papel pero la confirmación se pierde, no puede garantizarse exactamente una copia física. Se conserva incertidumbre y se exige decisión humana; no se oculta con reintentos ciegos.

## Preparación y pantallas del POS

**Preparación** muestra estación, cuenta, producto, cantidad, notas, antigüedad, tiempos y avisos. Filtra por estación/estado y pagina hasta 250 tareas. Permite avanzar de liberado a en preparación o listo, por partida o por pedido conforme a la configuración. No permite retroceder una tarea lista ni preparar unidades canceladas. Las revisiones evitan que otra terminal sobrescriba cambios recientes.

Una cuenta está lista cuando todas sus tareas activas de todas las estaciones están listas. La recepción KDS, aceptación de impresión, pago y entrega son estados diferentes. Una exención explícita sin preparación no crea tareas ficticias. Las cuentas y partidas del restaurante y para llevar muestran los estados calculados por Go; **Preparación y comandas** abre el detalle de la cuenta.

**Comandas** consulta documentos históricos y recepción KDS pendiente; **Cola de impresión** consulta trabajos e intentos; **Historial** muestra cambios y responsables. Las vistas globales de cola/historial exigen `kitchen.stations.all`, además del permiso específico. Los documentos de un cliente restringido solo incluyen sus secciones y ocultan dinero salvo configuración y permiso de precios.

## Eventos autenticados y futuro KDS

Se implementó **SSE**, equivalente funcional para este flujo, en lugar de WebSocket: las notificaciones son servidor→cliente y las acciones ya disponen de HTTP autenticado e idempotente. Evita un segundo protocolo de comandos y conserva la misma comprobación de permisos. No se presenta como una implementación WebSocket.

`GET /api/v1/events` usa cabeceras de sesión y dispositivo; Rust abre la conexión y entrega mensajes mediante `tauri::ipc::Channel`. Nunca pasa tokens al WebView. Se comprueban sesión, dispositivo y permisos durante el flujo, aproximadamente cada 750 ms. Límite dos conexiones por dispositivo, duración 30 segundos con reconexión, escrituras acotadas y frames nativos hasta 256 KiB.

El outbox PostgreSQL se escribe dentro de la transacción de negocio. Una secuencia durable con lock de publicación impide saltar commits pendientes. Cada evento tiene UUID, versión 1, secuencia, tipo, ámbito, agregado opcional, estación e instante. Solo contiene referencias de invalidación, no nombres de clientes, precios o credenciales. Ámbitos `restaurant`, `takeaway`, `kitchen`, `printing`, `configuration`; se filtran por permisos y estación del dispositivo.

El cliente envía `after` o `Last-Event-ID`. Se devuelven lotes hasta 200 eventos y cursor, avanzando también sobre entradas filtradas. La ventana lógica de replay es de 5000 secuencias; un cursor inválido/antiguo devuelve `reset:true`. React deduplica UUID y vuelve a consultar estado autoritativo en reconexión/reset/eventos, sin reemplazar formularios locales. El sondeo existente queda activo como recuperación. Una sesión revocada deja de recibir datos autorizados. No hay persistencia offline de esta suscripción.

Contratos para el futuro KDS, sin una aplicación KDS en esta entrega:

- Consultar `/kitchen/board?station_id=...` y `/kitchen/tickets?station_id=...`.
- Obtener `/kitchen/tickets/{id}` con documento y secciones autorizadas.
- `POST /kitchen/tickets/{id}/ack` con `operation_id` y `station_id`: recepción durable por ticket/estación/dispositivo. No marca preparado ni impreso.
- `POST /kitchen/states` con UUID, referencias y revisiones para preparación autorizada.
- Suscribirse a `/events?after=...&station_id=...` o recuperar `/events/snapshot`, y reconstruir tablero cuando sea necesario.

Rutas, permisos y cuerpos detallados en [API_CONTRACTS.md](../API_CONTRACTS.md).

## Pantalla completa y diseño adaptable

Tauri inicia en pantalla completa nativa y ofrece **Salir de pantalla completa / Pantalla completa** en todas las fases, con consultas nativas al recibir foco/redimensionar. Conserva ventana normal, controles y Spaces de macOS; también admite el atajo del sistema. No es modo quiosco.

Se eliminaron anchos máximos rígidos de los espacios administrativos y operativos. Las columnas de captura/cuenta, catálogo, personalización y cocina aprovechan el espacio, se reorganizan en resoluciones menores y conservan formularios/selecciones al redimensionar. 1280×720 es solo un escenario de prueba. Se conservan temas, acordeones accesibles y teclado virtual. No se alteraron los PNG originales de marca.

La configuración usa las APIs oficiales de [Tauri](https://v2.tauri.app/reference/config/) y de [ventana](https://v2.tauri.app/reference/javascript/api/namespacewindow/). Las pruebas de navegador no sustituyen la comprobación nativa de fullscreen; ambas evidencias se distinguen en VERIFICATION.

## Persistencia, migraciones y límites

`0005_kitchen_and_printing.sql` añade 17 tablas: configuración, impresoras, estaciones, rutas, relaciones de rutas/dispositivos, recibos de eventos, liberaciones, cambios pendientes, marcadores canónicos, tareas, documentos, recepciones KDS, trabajos, intentos, historial y outbox. No modifica los archivos 0001–0004. Los pedidos anteriores se importan como pendientes de revisión manual, sin imprimir ni presumir preparación previa.

Antes de aplicar a una instalación existente: respaldo custom, restauración completa en base temporal con comparación de todas las tablas, detener Go, migrar, verificar reaplicación sin cambios y reiniciar. No ejecutar setup de nuevo ni borrar `.local` o Keychain. La evidencia de instalación se registra por etapa.

Quedan fuera: agente remoto, KDS independiente, Android, offline durable, facturación electrónica, diseñador arbitrario, servicio comercial de licencias e instaladores Windows. No se ha certificado cada modelo de impresora, corte físico, detección de papel, muchas terminales o resistencia a pérdida eléctrica. Deben medirse volumen, retención de eventos/auditoría y tamaño de snapshots antes de uso comercial; el replay acotado no elimina físicamente filas del outbox.

## Documentos comerciales · etapa 6

Precuentas y tickets de pago se generan en `payments/documents.go` y se encolan con `kitchen.EnqueueCommercial` en la impresora de tickets elegida en las políticas de cobro. Usan `kitchen_tickets.kind` `precheck` o `receipt` y la misma cola, lease, serialización por impresora y resolución de incertidumbre que las comandas, pero no crean liberaciones ni tareas y no aparecen en el listado de comandas (`GET /kitchen/tickets`); sí pueden consultarse por identificador. La reimpresión (`payments/accounts/{id}/documents/reprint`) conserva el contenido exacto; exige que el trabajo original esté enviado o cancelado (un trabajo con error o incierto se resuelve primero en la cola) y responde `PRINT_COPY_PENDING` mientras otra copia siga sin resolver. Un fallo de impresión nunca revierte el pago y el estado del trabajo aparece en los documentos de la cuenta.

Etapa 7.1 (fase D): la precuenta y el ticket imprimen el motivo del catálogo de cada descuento o cortesía junto a su concepto («Descuento manual · Atención») y, con `usd_cash` activa y tipo de cambio del turno, la línea informativa «Total en dólares (T.C. $Y)» con `US$X` después del TOTAL (detalle en [PAYMENTS_AND_REFUNDS.md](PAYMENTS_AND_REFUNDS.md)). Las pruebas unitarias de `payments/documents_test.go` cubren ambas líneas, incluido el ajuste al papel de 58 mm; no se imprimió nada en papel.

## Documentos de caja y cajón de dinero · etapa 7 (fase C)

**Tipos de ticket de caja** (migración 0008): `cash_report` (corte parcial o definitivo), `cash_voucher` (comprobante de ingreso, retiro o gasto) y `cash_drawer` (comando de apertura del cajón). Los genera `payments` (`cash_documents.go` y `cash_drawer.go`) y usan la misma cola, lease, serialización por impresora y resolución de incertidumbre que las comandas, pero no crean liberaciones ni tareas y no aparecen en el listado de comandas. Los documentos llegan ya renderizados (texto de 42 o 30 columnas) y se imprimen en la impresora de tickets de Cobros con `kitchen.EnqueueCommercial`; su registro (`cash_documents`) indica si el contenido incluye el efectivo esperado. Desde la cola: un documento de caja con importes solo se consulta con `cash.shifts.read` y, si incluye el esperado, con `cash.expected.read` y fuera del arqueo de cierre; las copias se piden desde Caja con `cash.reports.reprint` (la reimpresión de la cola las rechaza).

**Pulso del cajón**: cada comando es un trabajo de una sola copia en la impresora configurada para el cajón de la caja. El controlador ESC/POS envía exactamente `ESC p m 25 250` (`0x1b 0x70 m 0x19 0xfa`, con `m` = 0 para el conector 2 o 1 para el 5): pulso de 50 ms y pausa de 500 ms, sin texto, sin corte y sin `ESC @` (reiniciar podría descartar lo que la impresora aún tenga en su búfer). En la etapa 7 solo el transporte **ESC/POS TCP** entregaba esos bytes sin alterar; desde la etapa 7.1 también una **cola CUPS en modo raw** (sección siguiente). Con el agente futuro el trabajo queda en intervención sin enviar nada y Go rechaza esa configuración al guardarla. Ninguna aceptación (TCP o CUPS) confirma que el cajón se abrió.

**Sin reintentos**: un comando del cajón con resultado incierto nunca se reenvía solo (como todo trabajo) y, a diferencia de los documentos, **tampoco un error confirmado**: un cajón que se abre minutos después sin nadie presente es un riesgo físico. Nunca pasa a una impresora de respaldo (el cajón está conectado a una impresora concreta). Desde la cola solo se cancela o se reconoce una aceptación conocida; «Reintentar» y «Autorizar otro envío» se rechazan, y para volver a abrir el cajón se usa la apertura manual de Caja, con su permiso y motivo auditado. Al terminar un trabajo de caja, el trabajador publica además `cash.print.updated` en el ámbito de caja.

**Verificación**: solo con el adaptador controlado de las pruebas de aceptación y una prueba unitaria de los bytes; no se probó con una impresora ESC/POS real ni con un cajón físico.

## Cajón por CUPS en modo raw · etapa 7.1

El cajón está conectado a la impresora, no a la computadora: el establecimiento usa impresoras de red (ESC/POS TCP) y otros clientes las conectarán por USB a través de CUPS. El cajón admite ambas (modo mixto); todo el envío sigue detrás de la interfaz `Transport` (`kitchen/transport.go`), de modo que en Windows se implementará con su sistema de impresión sin cambiar pagos ni la cola.

**Detección de datos raw** (`kitchen/cups_raw.go`, `QueueInspector`): solo con consultas de lectura y argumentos separados, nunca imprimiendo para averiguarlo. Primero `lpstat -p <cola>` (la cola existe; `lpoptions` con una cola desconocida termina con éxito y escribe bytes sin sentido, comprobado en este Mac) y después `lpoptions -p <cola>` sin `-o` (atributos `device-uri`, `printer-make-and-model` y `printer-type`; la salida no se traduce, a diferencia de la de `lpstat`). Cada consulta tiene un límite de 3 s y la configuración un presupuesto total de 4 s. Criterio conservador:

| Situación                                                                | Resultado                                                              |
| ------------------------------------------------------------------------ | ---------------------------------------------------------------------- |
| `lpstat -p` falla (cola inexistente o CUPS sin respuesta)                | No se usa: «no existe o CUPS no responde» (`DRAWER_QUEUE_UNAVAILABLE`) |
| Sin `device-uri`                                                         | No se usa: no se sabe a qué dispositivo envía                          |
| `printer-type` con el bit de clase (0x1) o de cola remota (0x2)          | No se usa: el cajón está en una impresora concreta de este equipo      |
| `printer-make-and-model` con «AirPrint», «IPP Everywhere» o «driverless» | No se usa: CUPS convierte cada trabajo al formato de la impresora      |
| Conexión `usb`, `serial`, `parallel` o `socket` (AppSocket, puerto 9100) | Admite datos raw                                                       |
| Otra conexión (`ipp`, `ipps`, `dnssd`, `lpd`, `http`…)                   | No se usa: recibe documentos; para red, usar ESC/POS por red           |

Criterio comprobado en este Mac solo con lecturas (`lpstat -a`, `lpstat -v`, `lpstat -l -p`, `lpoptions -p`), sin enviar trabajos: `Printer_POS_80` (`usb://Printer/POS-80`, controlador POS-80) y `_192_168_1_80` (`socket://192.168.1.80/`) admiten datos raw; `HP_LaserJet_400_M401n` (`ipp://`, «Generic PostScript Printer») y `HP_Smart_Tank_530_series` (`dnssd://…_ipps._tcp`, «…-AirPrint») no. Sus salidas de `lpoptions` (ubicación y UUID anonimizados) y de `lpstat` están grabadas en `backend/internal/kitchen/testdata/cups/` para las pruebas unitarias.

**Envío**: el trabajo `cash_drawer` en una impresora CUPS vuelve a consultar la cola (la configuración de CUPS pudo cambiar) y, si ya no admite datos raw, queda en `intervention` sin enviar nada. Si la admite, escribe los cinco bytes del pulso en un archivo temporal privado y ejecuta `/usr/bin/lp -d <cola> -o raw -n 1 -t ADEL-cajon-<trabajo> <archivo>` (argumentos separados, cola validada por `queuePattern`, sin shell); el archivo se elimina al terminar. Con el identificador del trabajo de CUPS en la respuesta, `submitted` («Comando de apertura aceptado por CUPS; no confirma que el cajón se abrió»); si `lp` no pudo ejecutarse, error confirmado; cualquier otro resultado, `uncertain`. Como en la etapa 7, un comando del cajón nunca se reintenta solo (ni por error ni por incertidumbre) y nunca pasa a una impresora de respaldo.

**Probar apertura**: Configuración → Cajón ofrece «Probar apertura» (`POST payments/cash/drawers/{caja}/test`, `cash.settings.manage`, auditada, motivo `test` de la migración 0009). Usa la misma cola y las mismas reglas.

**Verificación**: pruebas unitarias con un ejecutor simulado que registra los argumentos de `lpstat`, `lpoptions` y `lp` y el contenido del archivo del pulso sin ejecutar nada, y aceptación PostgreSQL con una consulta de colas simulada y el adaptador controlado de la cola. **Pendiente en hardware**: no se envió ningún trabajo ni pulso a una impresora real; la prueba física queda pendiente hasta que el propietario tenga una impresora USB con cajón. No se agregó una prueba de hardware del cajón: cuando exista, deberá ser opt-in y con destino explícito, como `TestControlledCUPSPrinter`.

## Consumo de inventario · etapa 8

El inventario lee la liberación canónica; la cocina no espera al inventario ni cambia su comportamiento. Detalle en [INVENTORY_AND_PURCHASING.md](INVENTORY_AND_PURCHASING.md).

- **Solicitud en la liberación**: `prepareRelease` (`kitchen/releases.go`), justo después de insertar las filas de `kitchen_unit_releases` y antes de los documentos, llama a `inventory.RecordReleases`. Con la función «Control de inventario» activada, un solo `INSERT … SELECT … ON CONFLICT DO NOTHING` registra una solicitud por unidad con la versión y la hora de la liberación canónica y la modalidad congelada; no toma el lock de inventario 7820148308 ni lee recetas, de modo que la transacción de la liberación (lock 7820148305) apenas cambia (≈ 0.2 ms de media en la confirmación, medido). Cubre los cuatro caminos que liberan: confirmación inmediata, planificador de programados, liberación manual y reintento de un cambio pendiente. Una liberación bloqueada por advertencias no crea filas canónicas ni solicitudes hasta resolverse.
- **Cambios**: `kitchen.Integrate` llama a `inventory.RecordChange` para `item.modified` e `item.cancelled` antes de procesar cocina, también cuando el cambio de cocina queda pendiente: `amend` para las unidades modificadas y `cancel` para las canceladas o sobrantes de una reducción, solo si la unidad tiene fila canónica y solicitud de liberación.
- **Una sola vez por unidad**: varias estaciones, copias, reimpresiones, cambios de impresora, divisiones, transferencias, combinaciones y cambios de mesa no crean solicitudes; un reintento HTTP devuelve la respuesta guardada y un evento reprocesado lo detiene `kitchen_event_receipts`.
- **Estaciones y almacenes**: el trabajador de inventario lee `kitchen_tasks.instructions.parts[].path` (también de tareas canceladas) para elegir el almacén de cada parte según la estación que la recibió y la asignación estación → almacén vigente a la hora de la liberación; los productos «Sin preparación» consumen del almacén predeterminado.
- **Comandas**: las indicaciones con efecto se imprimen una sola vez (ver «Documentos»); sin indicaciones con efecto la comanda es idéntica a la de la etapa 7.1. La pantalla de preparación no lista ingredientes y no cambió.

## Plantillas, formato y logotipo · etapa 9B

La etapa 9B reemplaza las tres plantillas de texto escritas en código (`kitchen/template.go`, `payments/documents.go` y
`payments/cash_documents.go`) por un **renderizador único** en `internal/documents` con plantillas de bloques versionadas.
**La plantilla define cómo aparece cada documento, nunca a dónde va**: estaciones, rutas, impresoras, unificación,
respaldo, cola, lease, serialización por impresora, reintentos y resultados inciertos no cambian. Guía completa en
[DOCUMENT_DESIGNER.md](DOCUMENT_DESIGNER.md).

- **Representación intermedia (IR)**: Go convierte la plantilla y los datos del documento en líneas ya cortadas y
  alineadas con atributos (negrita, tamaño) e imágenes (`documents.Layout`). La vista previa del diseñador y la impresión
  usan la misma IR. `kitchen_print_jobs.content` guarda `Text()` y la columna nueva `layout` (0018) guarda la IR solo si
  lleva atributos o logotipo; con las plantillas predeterminadas es NULL y el trabajo es idéntico al anterior a la 9B
  (equivalencia byte a byte comprobada en 6 144 comandas y avisos, 168 precuentas y tickets y más de 1 000 documentos de
  caja, a 58 y 80 mm, incluidos los bytes ESC/POS y los argumentos de `lp`).
- **Transportes** (`kitchen/transport_formatted.go`): una IR plana viaja por el camino anterior. Con atributos o logotipo,
  ESC/POS por red recibe `GS !`, `ESC E` y la imagen con `GS v 0` en franjas de hasta 128 filas
  (`documents.EncodeESCPos`). Una impresora CUPS solo los recibe, con `/usr/bin/lp -o raw` y argumentos separados, si
  tiene activada la opción **«Esta impresora entiende ESC/POS (formato y logotipo)» (`accepts_escpos`)** (desactivada por omisión; corrección
  de la revisión independiente: una cola que admite raw no implica una impresora ESC/POS) y la cola admite datos raw según
  la detección de solo lectura de la 7.1 (`cups_raw.go`), consultada otra vez antes de enviar. En cualquier otro caso se
  imprime el texto por el camino anterior, sin negrita, tamaños ni logotipo, y el mensaje del resultado lo dice; **nunca
  falla por eso**. Si CUPS no responde a la consulta, el trabajo queda en error confirmado sin enviar nada.
- **Logotipo**: el trabajador carga los bits ya convertidos por identificador (`Worker.Rasters`, `identity.Rasters`) sin
  transacción abierta; si no los encuentra, la imagen se omite con aviso. Un trabajo que se envía a una impresora de otro
  ancho (respaldo, redirección o copia a otra impresora) no envía el logotipo del ancho original.
- **Comandas**: al crear el ticket se guardan la versión activa (`kitchen_tickets.template_version_id`), la copia de la
  identidad y el tamaño efectivo; la configuración de Cocina sigue copiada en el `Document`. **La reimpresión desde la cola
  vuelve a renderizar con la versión guardada** (sin versión = versión 1 del sistema, idéntica al formato anterior), sin
  leyenda REIMPRESIÓN. Precuentas, tickets y documentos de caja copian el contenido y el layout del trabajo original.
- **Imprimir con la plantilla vigente** (`POST kitchen/print-jobs/rerender`, `kitchen.reprint` + `kitchen.stations.all` +
  `documents.history.render_current`, motivo de 5–500 caracteres): acción distinta de la reimpresión, con un ticket
  `document_rerender`, auditoría `print.rerendered` y la impresora del trabajo original (otra con
  `kitchen.printers.manage`). La reimpresión exacta y esta acción se bloquean entre sí mientras haya una copia sin
  resolver (`PRINT_COPY_PENDING`).
- **Muestras**: `document_sample`, con «*** MUESTRA · NO ES UN DOCUMENTO VÁLIDO ***» al inicio y al final, por la misma
  cola; su resultado se resuelve en la Cola de impresión como cualquier trabajo. Ni `document_sample` ni
  `document_rerender` aparecen entre las comandas (`GET kitchen/tickets`).
- **Lista de cocina en la plantilla**: «Importes» solo admite «Según Cocina» u «Ocultar» (las comandas se guardan sin
  precios si Cocina los oculta); observaciones, estilo de cantidad y espaciado admiten «Según Cocina» o un valor propio.
  El tamaño de texto del documento admite «Según Cocina», normal o grande.
- **Prueba técnica** (`kitchen/printers/test`): sigue igual, con la versión 1 de la comanda.
- **Verificación**: solo con el transporte controlado de la cola, un puerto TCP de loopback y un ejecutor simulado de
  CUPS. **Ningún documento con formato o logotipo se imprimió en papel**: el resultado en cada impresora es una prueba
  física pendiente del propietario.

## Columnas por impresora y prueba de ancho · etapa 9C

AD-EL POS imprime desde la etapa 5 con 42 columnas en 80 mm y 30 en 58 mm; el sistema anterior imprimía 48 en 80 mm. No
todas las impresoras de 80 mm imprimen el ancho completo (muchas tienen 72 mm imprimibles), así que **el restaurante
elige el valor por impresora después de una prueba en papel** (§5.3 de [etapas/ETAPA_9C_v3.md](etapas/ETAPA_9C_v3.md)).

**En la pantalla** (Cocina → «Cocina y preparación» → Impresoras, con `kitchen.printers.manage`):

- **Columnas de texto**: en 80 mm, «42 (predeterminado)» o «48 columnas»; en 58 mm, «30 (predeterminado)» o «32
  columnas». Lista cerrada, sin escritura libre. Al cambiar el papel las columnas vuelven al predeterminado del papel
  nuevo. El listado muestra papel y columnas de cada impresora.
- **Imprimir prueba de ancho (N columnas)**, junto a la opción: imprime, por la cola de siempre, una regla numerada del
  ancho elegido (con las decenas), una línea completa de guiones y el renglón «Producto de ejemplo … $1,234.50», entre
  las marcas «*** PRUEBA DE ANCHO ***». Usa las columnas elegidas en el formulario aunque aún no se hayan guardado; si se
  cambió el papel, hay que guardarlo antes de imprimir su prueba. **No crea ventas, pagos ni movimientos de caja o
  inventario.** Usa 48 o 32 solo si en el papel se ven completos el último número, la línea de guiones y el importe.
- El cambio aplica a los documentos nuevos; las copias de documentos ya impresos conservan su ancho.

**Reglas** (Go y PostgreSQL):

- `kitchen_printers.columns` (0020): NULL = predeterminado del papel, de modo que las impresoras existentes y las
  fotografías guardadas en trabajos anteriores imprimen exactamente igual. Lista cerrada en Go
  (`documents.ColumnChoices`, `documents.ValidColumns`) y en el CHECK `kitchen_printers_columns_check`; elegir el
  predeterminado se guarda como NULL; un valor fuera de la lista o que no corresponde al papel se rechaza (`400
KITCHEN_VALIDATION`). Queda en el historial `printer.saved` como cualquier cambio de impresora.
- **El renderizador único usa las columnas de la impresora destino** (`kitchen.Printer.RenderOptions`): comandas y avisos
  al encolar, prueba técnica, muestras del diseñador, precuentas, tickets, comprobantes y cortes (impresora de tickets de
  Cobros). Con 48 o 32 columnas los renglones «concepto … importe» y los cortes de línea se recalculan al nuevo ancho; el
  texto grande de Cocina sigue usando la mitad de las columnas; el logotipo no cambia (depende de los puntos, 384 o 576).
  **Con el predeterminado la salida es idéntica a la de la 9B** (las pruebas de equivalencia de la 9B siguen aprobadas).
- **Reimpresión exacta**: la copia de una comanda desde la cola se vuelve a renderizar con la **fotografía de la
  impresora del trabajo original** (`printer_snapshot`, que ahora incluye las columnas), así que sale con las columnas con
  que se emitió aunque después cambien; una fotografía anterior a la 9C no tiene el campo y usa el predeterminado.
  Precuentas, tickets y documentos de caja copian su texto, como antes. «Imprimir con la plantilla vigente» usa la
  configuración actual de la impresora del trabajo original.
- **CUPS en texto**: con el predeterminado los argumentos de `lp` no cambian (`cpi` 12, o 6 con el texto grande); con
  otras columnas, `cpi` en proporción (12 × columnas ÷ predeterminado: 13.71 para 48 y 12.8 para 32; 6.86 y 6.4 con el
  texto grande). ESC/POS por red y CUPS raw no cambian (fuente A de 12 puntos: 48 × 12 = 576 y 32 × 12 = 384 puntos).
- **Prueba de ancho** (`POST kitchen/printers/{id}/width-test`, operación del puente `kitchen_printer_width_test`): ticket
  `test` con `purpose: "width_test"`, trabajo con la fotografía de la impresora con las columnas probadas (para que CUPS
  de texto use la misma densidad que usará al guardarlas), idempotente por `operation_id`, historial
  `print.width_test_requested`. Una copia desde la cola repite el mismo texto. Contrato en
  [API_CONTRACTS.md](../API_CONTRACTS.md) («Etapa 9C»).
- El respaldo automático de impresora no vuelve a renderizar (como ya ocurría con el papel): si el respaldo tiene otras
  columnas, el texto conserva las del original y CUPS usa la densidad del respaldo. La vista previa de una comanda en la
  cola y la precuenta en pantalla siguen a 80 mm y 42 columnas.

**Verificación**: equivalencia sin cambios con el predeterminado; salida a 48 y 32 columnas en los diez tipos; valor
fuera de la lista rechazado en Go y en PostgreSQL; reimpresión exacta tras cambiar las columnas; prueba de ancho sin
ventas ni movimientos (conteos antes y después), todo con el transporte controlado de la cola y ejecutores simulados
([VERIFICATION_STAGE9C.md](VERIFICATION_STAGE9C.md)). **El papel a 48 y 32 columnas y la densidad fraccionaria de CUPS
son una prueba física pendiente**: para eso existe la prueba de ancho.

## Cancelar cuenta y «Enviado · sin confirmar en cocina» · etapa 9D

Sin cambios de enrutamiento, cola, transportes, plantillas ni reglas de resultado; sin tablas nuevas de cocina. Detalle
de la función en [RESTAURANT_OPERATIONS.md](RESTAURANT_OPERATIONS.md) («Mesas, cuentas, Para llevar y cancelar cuenta ·
etapa 9D») y contratos en [API_CONTRACTS.md](../API_CONTRACTS.md) («Etapa 9D»).

**Aviso de cancelación de una cuenta completa.** «Cancelar cuenta» registra **un solo evento** `account.cancelled` con
todas las unidades activas de la cuenta en `after.unit_ids` (la misma forma que `item.cancelled`), y `kitchen.Integrate`
lo procesa en la misma transacción exactamente como una cancelación de partidas:

- lo que todavía no se liberó (liberación pendiente, programada o bloqueada) se cancela **sin papel**;
- lo ya liberado cancela sus tareas y genera el **aviso de cancelación** para cada estación que lo recibió, por la cola
  durable de siempre (un aviso por liberación y estación, con el mismo documento de cancelación de las partidas);
- con una estación inactiva o un destino inválido, el cambio queda **pendiente y durable**: la respuesta de la
  cancelación ya dice `change_pending` y Cocina → «Pendientes de liberación y avisos» lo muestra como «Aviso de
  cancelación pendiente» (antes de la 9D solo `item.cancelled` se llamaba así); su reintento no duplica avisos ni
  liberaciones;
- como siempre, `submitted` significa que CUPS o el puerto TCP aceptaron el trabajo, no que salió el papel. **La impresión
  real del aviso de cancelación de una cuenta no se verificó** (solo el transporte controlado de la cola).

Antes de cocina, `inventory.RecordChange` registra las solicitudes `cancel` de lo ya liberado y consumido (solo con
«Control de inventario» activado). Si el motivo de la cancelación marca «lo enviado a cocina cuenta como merma»
(«Cliente se retiró sin pagar» en la semilla), el trabajador de inventario las **preclasifica** como «Preparado / merma»
(«Preclasificada al cancelar la cuenta», corregible por el encargado hasta el corte del día, como en la etapa 8); con los
demás motivos quedan por clasificar. La cocina no sabe si algo se preparó: sin pantalla de cocina, las tareas nunca pasan
a «Listo», por eso la marca vive en el motivo y no en el estado de la tarea.

**«Enviado · sin confirmar en cocina».** Es la etiqueta corta que sustituye, una sola vez por producto y ya no en el
encabezado de la cuenta, a «Pendiente de liberación · revisar cocina» (`preparation_state: "pending_release"`, que
`kitchen.VersionPreparation` devuelve cuando una versión tiene unidades activas sin liberación canónica en
`kitchen_unit_releases` y ninguna liberación programada); en la tarjeta de la cuenta, «Sin confirmar en cocina». Significa que el pedido se confirmó, pero
**su liberación quedó bloqueada**:

- al confirmar, `prepareRelease` no encuentra ruta («Sin ruta de preparación: producto»), estación activa o destino válido
  y deja la liberación `blocked` sin unidades liberadas, con una advertencia por unidad: no se imprime nada, no se crean
  tareas y no se consume inventario. Es la regla de la etapa 5 (una configuración incompleta produce advertencias visibles
  y una liberación bloqueada, **nunca un envío aparente**);
- por eso aparece en una instalación **sin estaciones, rutas ni impresoras**, como la instalación real al revisar la
  Fase 0 de la 9D (0 estaciones, 0 rutas y 0 impresoras; 4 liberaciones bloqueadas, 23 advertencias «Sin ruta de
  preparación» y 15 unidades activas sin liberar). No es un defecto del cálculo;
- configurar después la estación y la ruta **no reintenta** una liberación bloqueada (el planificador solo reintenta las
  programadas). Se libera a mano en **Cocina → «Pendientes de liberación y avisos» → «Liberar a preparación»**
  (`kitchen.release` + `kitchen.stations.all`). Un pedido nuevo con la configuración completa se libera solo. Para
  productos que no van a cocina, la ruta **Sin preparación** (`mode='none'`) evita la liberación bloqueada (quedan como
  «Listo»);
- las liberaciones bloqueadas de cuentas ya cerradas siguen en esa lista (limitación observada en la Fase 0, sin
  cambio);
- «Preparación y comandas» (detalle de la cuenta) muestra cada advertencia distinta una sola vez, con «(×N)» si se
  registró por varias unidades (Go sigue registrando una por unidad).

## Cola de impresión de Windows · etapa 9E

Con el servidor Go en Windows, el transporte guardado como `cups` significa **«cola de impresión del equipo»**: la
impresora instalada en Windows (USB, puerto serie o paralelo, o «Puerto TCP/IP estándar») recibe **ESC/POS en modo
RAW** por el administrador de impresión de Windows. Sin migración ni valor nuevo en la base: solo cambia qué hace el
equipo servidor con ese valor. En macOS todo sigue igual (CUPS).

- **Dónde vive**: `kitchen/system_queue.go` elige la cola del sistema (`system_queue_unix.go` → CUPS,
  `system_queue_windows.go` → Windows); `kitchen/windows_queue.go` tiene la lógica pura (qué impresoras se aceptan,
  resultados, resumen en español) y `kitchen/winspool_windows.go` las llamadas a `winspool.drv`, cargada solo desde
  System32: `EnumPrinters`, `GetPrinter`, `EnumPorts`, `OpenPrinter`, `StartDocPrinter` (tipo `RAW`), `StartPagePrinter`,
  `WritePrinter`, `EndPagePrinter`, `EndDocPrinter` y `AbortPrinter`. Las pruebas con los programas de CUPS simulados
  siguen ejercitando CUPS en los dos sistemas; las que sustituyen el administrador de Windows, siempre Windows.
- **Consulta previa, solo lectura**, antes de configurar el cajón, en la vista previa del diseñador y antes de cada envío.
  Se aceptan puertos USB, COM, LPT y TCP/IP estándar. Se rechazan, sin enviar nada y con el motivo: impresoras de otro
  equipo o compartidas, de Escritorio remoto o de la nube, virtuales (Microsoft Print to PDF, XPS, OneNote, fax),
  `FILE:`, `PORTPROMPT:`, `NUL:`, puertos WSD y monitores de puerto desconocidos.
- **Mismos bytes que ESC/POS por red**: texto con el codificador CP850 de siempre, documentos con formato o logotipo con
  `documents.EncodeESCPos` y el pulso del cajón con `DrawerPulseBytes`. La marca «Esta impresora entiende ESC/POS
  (formato y logotipo)» no aplica en Windows (se oculta), y tampoco la opción de corte de CUPS: el corte va en los
  bytes ESC/POS como en una impresora de red.
- **Resultados** (reglas de la cola sin cambio): `error` confirmado si nada se envió (impresora inexistente, rechazada o
  sin permiso); `uncertain` si falló después de abrir el documento (nunca se reintenta solo ni pasa al respaldo);
  `submitted` con «Aceptado por la cola de impresión de Windows; no confirma salida física de papel.». El cajón conserva
  las reglas de la etapa 7: solo efectivo físico, sin reintentos y «Comando de apertura enviado».
- **Configuración**: «Consultar impresoras del equipo servidor» lista las impresoras instaladas en Windows con su
  puerto, controlador y si AD-EL POS puede usarlas («Usar esta impresora»); si Windows no responde,
  `503 SPOOLER_UNAVAILABLE`. Los textos de la interfaz dicen «Impresora instalada en Windows» solo en Windows
  (`isWindows()` de `commands/platform.ts`); en macOS quedan idénticos.
- **ESC/POS por red** no depende del sistema y funciona igual en Windows.

**Verificación**: pruebas unitarias de la lógica pura y del despacho por sistema, prueba en Windows de la consulta real
(`EnumPrinters`) que confirma que «Microsoft Print to PDF», la única impresora del equipo de pruebas, se rechaza como
virtual, y aceptación PostgreSQL de `designer`, `payments` y `operations`. **Pendiente en hardware**: ningún trabajo ni
pulso se envió a una impresora USB real; papel, logotipo, corte y cajón por USB en Windows son prueba física del
propietario (lista en [INSTALAR_EN_WINDOWS.md](INSTALAR_EN_WINDOWS.md)).
