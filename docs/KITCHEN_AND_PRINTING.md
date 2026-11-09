# Cocina, comandas e impresión · Etapa 5

Versión 0.6.2, esquema 7. Go es la autoridad de preparación y ejecuta los trabajadores sin depender de una ventana Tauri. No existen inventario físico ni aplicación KDS independiente. La preparación lista no significa pagado o entregado: pago y entrega pertenecen a `payments` (etapa 6).

## Configuración inicial

Abrir **Cocina y preparación** con permisos administrativos. No se crean estaciones, rutas, impresoras ni pedidos ficticios en la instalación real.

1. En **Impresoras**, descubrir las colas CUPS del servidor o registrar un destino manual. Seleccionar ancho 58/80 mm, 1–3 copias y capacidad de corte real. El destino corresponde al equipo donde corre Go.
2. En **Estaciones**, crear nombre, descripción, orden, estado, tiempos objetivo/advertencia/atraso y recepción por impresora, KDS o ambos. Varias estaciones pueden seleccionar la misma impresora.
3. En **Enrutamiento**, asignar categoría y, si procede, una excepción por producto. Un producto puede llegar a varias estaciones. Seleccionar explícitamente **Sin preparación** para artículos que no deban generar trabajo.
4. En **Comandas y tiempos**, elegir documentos independientes o unificados, seguimiento por partida o pedido, formato y política de programación. Una estación puede sobrescribir agrupación y seguimiento.
5. En Estaciones, limitar las terminales a sus estaciones. Un usuario sin `kitchen.stations.all` solo puede consultar/operar el trabajo asignado a su dispositivo; además necesita cada permiso funcional. El listado administrativo de configuración conserva su propio permiso.

La primera configuración no altera contraseñas, catálogo, cuentas históricas ni credenciales del Llavero. Los veinte permisos nuevos se conceden inicialmente al rol administrador. Los otros roles necesitan asignación explícita. La identidad reservada `sa` conserva su política anterior.

## Enrutamiento y liberación

La ruta de producto prevalece sobre categoría. Los componentes elegidos de un combo se resuelven recursivamente con sus identidades de catálogo; no se duplican productos ni unidades comerciales. La expansión conserva variantes, porciones, ingredientes, grupos y observaciones. Una ruta ausente, una estación inactiva o un destino inválido produce advertencias visibles y una liberación bloqueada; nunca un envío aparente. Un combo vacío sin ruta tampoco se interpreta como exención implícita.

Guardar o previsualizar un borrador no crea comandas. Confirmar registra el pedido y su intención durable de preparación en la misma transacción. Un pedido inmediato intenta liberarse; la ausencia de configuración conserva el pedido y muestra qué corregir. Después de corregir, usar **Liberar** con la revisión actual.

`kitchen_unit_releases` es el punto canónico de liberación futura para inventario: una fila por UUID de unidad comercial, con versión y liberación. Las tareas por estación y las copias de impresión no representan ventas o consumos adicionales. Esta etapa no descuenta existencias.

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

El formato admite nombre del establecimiento, notas, precios, texto normal/grande, separación 1–3 y cantidad delante o multiplicador. Los controles de usuario, ESC/GS y caracteres invisibles se eliminan del texto; solo el controlador genera comandos. No existe diseñador visual arbitrario ni ejecución de plantillas.

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

Quedan fuera: agente remoto, KDS independiente, Android, offline durable, inventario físico, facturación electrónica, diseñador arbitrario, servicio comercial de licencias e instaladores Windows. No se ha certificado cada modelo de impresora, corte físico, detección de papel, muchas terminales o resistencia a pérdida eléctrica. Deben medirse volumen, retención de eventos/auditoría y tamaño de snapshots antes de uso comercial; el replay acotado no elimina físicamente filas del outbox.

## Documentos comerciales · etapa 6

Precuentas y tickets de pago se generan en `payments/documents.go` y se encolan con `kitchen.EnqueueCommercial` en la impresora de tickets elegida en las políticas de cobro. Usan `kitchen_tickets.kind` `precheck` o `receipt` y la misma cola, lease, serialización por impresora y resolución de incertidumbre que las comandas, pero no crean liberaciones ni tareas y no aparecen en el listado de comandas (`GET /kitchen/tickets`); sí pueden consultarse por identificador. La reimpresión (`payments/accounts/{id}/documents/reprint`) conserva el contenido exacto; exige que el trabajo original esté enviado o cancelado (un trabajo con error o incierto se resuelve primero en la cola) y responde `PRINT_COPY_PENDING` mientras otra copia siga sin resolver. Un fallo de impresión nunca revierte el pago y el estado del trabajo aparece en los documentos de la cuenta.

## Documentos de caja y cajón de dinero · etapa 7 (fase C)

**Tipos de ticket de caja** (migración 0008): `cash_report` (corte parcial o definitivo), `cash_voucher` (comprobante de ingreso, retiro o gasto) y `cash_drawer` (comando de apertura del cajón). Los genera `payments` (`cash_documents.go` y `cash_drawer.go`) y usan la misma cola, lease, serialización por impresora y resolución de incertidumbre que las comandas, pero no crean liberaciones ni tareas y no aparecen en el listado de comandas. Los documentos llegan ya renderizados (texto de 42 o 30 columnas) y se imprimen en la impresora de tickets de Cobros con `kitchen.EnqueueCommercial`; su registro (`cash_documents`) indica si el contenido incluye el efectivo esperado. Desde la cola: un documento de caja con importes solo se consulta con `cash.shifts.read` y, si incluye el esperado, con `cash.expected.read` y fuera del arqueo de cierre; las copias se piden desde Caja con `cash.reports.reprint` (la reimpresión de la cola las rechaza).

**Pulso del cajón**: cada comando es un trabajo de una sola copia en la impresora configurada para el cajón de la caja. El controlador ESC/POS envía exactamente `ESC p m 25 250` (`0x1b 0x70 m 0x19 0xfa`, con `m` = 0 para el conector 2 o 1 para el 5): pulso de 50 ms y pausa de 500 ms, sin texto, sin corte y sin `ESC @` (reiniciar podría descartar lo que la impresora aún tenga en su búfer). Solo el transporte **ESC/POS TCP** entrega esos bytes sin alterar; con CUPS (filtra el contenido como texto) o con el agente futuro el trabajo queda en intervención sin enviar nada, y Go rechaza esa configuración al guardarla. La aceptación TCP no confirma que el cajón se abrió.

**Sin reintentos**: un comando del cajón con resultado incierto nunca se reenvía solo (como todo trabajo) y, a diferencia de los documentos, **tampoco un error confirmado**: un cajón que se abre minutos después sin nadie presente es un riesgo físico. Nunca pasa a una impresora de respaldo (el cajón está conectado a una impresora concreta). Desde la cola solo se cancela o se reconoce una aceptación conocida; «Reintentar» y «Autorizar otro envío» se rechazan, y para volver a abrir el cajón se usa la apertura manual de Caja, con su permiso y motivo auditado. Al terminar un trabajo de caja, el trabajador publica además `cash.print.updated` en el ámbito de caja.

**Verificación**: solo con el adaptador controlado de las pruebas de aceptación y una prueba unitaria de los bytes; no se probó con una impresora ESC/POS real ni con un cajón físico.
