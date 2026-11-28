# Cocina, comandas e impresión · Etapa 5

Versión 0.9.3, esquema 21 (la migración `0021_account_cancellation.sql` de la etapa 9D no cambia tablas de cocina y se aplicó a la instalación real el 12 de noviembre de 2026; la cancelación de cuentas completas llega a cocina como las cancelaciones de partidas, ver «Cancelar cuenta y «Enviado · sin confirmar en cocina» · etapa 9D»; la migración `0020_terminal_home_and_printer_columns.sql` de la etapa 9C agrega las columnas de texto por impresora y se aplicó a la instalación real el 11 de noviembre de 2026 con la autorización previa del propietario; ver «Columnas por impresora y prueba de ancho · etapa 9C»; la migración 0009 de la etapa 7.1 se aplicó a la instalación real el 9 de noviembre de 2026; ver «Cajón por CUPS en modo raw · etapa 7.1»; las 0010–0014 de la etapa 8 y las 0015–0017 de la etapa 9A se aplicaron el 10 de noviembre de 2026; las 0018 y 0019 de la etapa 9B se aplicaron a la instalación real el 11 de noviembre de 2026 con autorización del propietario). Desde la etapa 9B todos los documentos impresos salen de un renderizador único con plantillas por bloques, versiones e identidad del establecimiento, sin cambiar enrutamiento, cola ni reglas de resultado (ver «Plantillas, formato y logotipo · etapa 9B» y [DOCUMENT_DESIGNER.md](DOCUMENT_DESIGNER.md)). La etapa 9A no cambia la cocina: los reportes de demanda por producto y por hora y de preparación por estación leen las liberaciones canónicas con su fecha de liberación, en una transacción de solo lectura y sin el lock operativo ([REPORTS.md](REPORTS.md)). Go es la autoridad de preparación y ejecuta los trabajadores sin depender de una ventana Tauri. No existe aplicación KDS independiente. Desde la etapa 8 la liberación canónica es el origen del consumo de inventario (ver «Consumo de inventario · etapa 8»). La preparación lista no significa pagado o entregado: pago y entrega pertenecen a `payments` (etapa 6).

**Desde la etapa K1.1 (servidor 0.17.0, esquema 30) un producto sin ruta nunca detiene el pedido**: lo que tiene ruta
llega a su estación y lo demás sigue el ajuste «Productos sin ruta» de Enrutamiento. Lo que este documento dice de las
liberaciones bloqueadas es historia de las versiones anteriores, salvo donde se indica; la regla vigente está en
«Pedidos mixtos y productos sin ruta · etapa K1.1», al final.

**Desde la etapa M1 (servidor 0.18.0, esquema 31)** quitar la hora a un pedido programado lo envía a cocina en ese
momento (M1-D17, reemplaza K11-D9), un producto que no necesita preparación no lleva etiqueta de cocina (M1-D15) y la
comanda, la pantalla de cocina y Preparación dicen el nombre de la unión de mesas («Mesa 5 + 6»): ver «Uniones de mesas,
etiquetas por renglón y textos de Preparación · etapa M1», al final.

## Configuración inicial

Abrir **Cocina y preparación** con permisos administrativos. No se crean estaciones, rutas, impresoras ni pedidos ficticios en la instalación real.

1. En **Impresoras**, descubrir las colas CUPS del servidor o registrar un destino manual. Seleccionar ancho 58/80 mm, 1–3 copias y capacidad de corte real. El destino corresponde al equipo donde corre Go.
2. En **Estaciones**, crear nombre, descripción, orden, estado, tiempos objetivo/advertencia/atraso y recepción por impresora, KDS o ambos. Varias estaciones pueden seleccionar la misma impresora.
3. En **Enrutamiento**, asignar categoría y, si procede, una excepción por producto. Un producto puede llegar a varias estaciones. **Sin preparación** sigue disponible para marcar explícitamente artículos que no deban generar trabajo; desde la etapa K1.1 lo que no tiene ruta ya no detiene nada y sigue el ajuste **«Productos sin ruta»** de la misma pestaña («No necesitan preparación», el valor inicial, o «Enviar a una estación»), y la lista «Lo que no va a ninguna estación» dice qué queda sin ruta.
4. En **Comandas y tiempos**, elegir documentos independientes o unificados, seguimiento por partida o pedido, formato y política de programación. Una estación puede sobrescribir agrupación y seguimiento.
5. En Estaciones › Computadoras en modo Cocina (hasta la etapa K1.1, «Terminales de cocina»), elegir las estaciones que muestra cada computadora (todas por omisión) y su aviso sonoro; las pantallas de cocina Android (etapa K1) se configuran en Sistema › Terminales, con el asistente «Configurar qué recibe» desde la K1.1 ([COCINA_INSTALAR.md](COCINA_INSTALAR.md), sección 4); en Personal › Usuarios › Estaciones de cocina, las que atiende cada usuario (todas por omisión). Desde la etapa 9F en Preparación se ve la intersección (ver «Cocina por estaciones y terminal de cocina · etapa 9F»); además cada usuario necesita cada permiso funcional. El listado administrativo de configuración conserva su propio permiso.

La primera configuración no altera contraseñas, catálogo, cuentas históricas ni credenciales del Llavero. Los veinte permisos nuevos se conceden inicialmente al rol administrador. Los otros roles necesitan asignación explícita. La identidad reservada `sa` conserva su política anterior.

### Datos de prueba · etapa 9F

Las pruebas automáticas crean un destino de aceptación que nunca imprime («Destino controlado E2E», transporte de
agente, destino «Agente de aceptación no disponible»), estaciones «Cocina caliente» y «Freidoras» con ese destino y la
ruta de «Platillos». Desde la 9F nunca llegan a una instalación real: el servidor en modo de pruebas (`test_mode`) se
niega a arrancar contra una base que no sea `adel_pos_test_*`, las pruebas de navegador exigen ese modo y la prueba de
humo de Windows abre la aplicación con una carpeta de datos temporal y `ADEL_TEST_MODE=1`.

Lo que ya quedó en una instalación se quita en **Administración › Sistema › Mantenimiento › «Quitar datos de prueba»**
(`settings.manage`): «Revisar datos de prueba» muestra lo que se elimina, lo que se desactiva porque ya tiene
operaciones registradas (trabajos de impresión, tareas de cocina, ventas) o participa en una promoción del restaurante
(la promoción no cambia), la configuración que se desvincula (impresora de tickets de Cobros, cajón, impresora de
respaldo, estaciones por terminal y, desde la fase C, por usuario, y rutas; la ruta de un producto o una categoría del restaurante que solo enviaba a
estaciones de prueba se quita y aparece aquí, no como dato de prueba) y lo que se conserva, cada uno con su motivo.
Solo se reconocen las firmas exactas de los archivos de prueba (nombre más atributos, lista cerrada en
`backend/internal/maintenance/signatures.go`): una estación «Cocina caliente» con otra impresora, o una categoría
«Platillos» con productos del restaurante, no se tocan. Al confirmar, Go crea primero un respaldo de la base (si no
puede, no quita nada), aplica todo en una transacción y lo audita; el aviso dice lo quitado y el nombre del respaldo.
Ventas, pagos, cortes y movimientos de inventario nunca cambian, así que una instalación de Windows que operó con la
prueba de humo conserva su turno y sus ventas: para dejarla limpia hay que reinstalar con otra carpeta de datos.

## Enrutamiento y liberación

La ruta de producto prevalece sobre categoría. Los componentes elegidos de un combo se resuelven recursivamente con sus identidades de catálogo; no se duplican productos ni unidades comerciales. La expansión conserva variantes, porciones, ingredientes, grupos y observaciones. Hasta la 0.16.0, una ruta ausente, una estación inactiva o un destino inválido producía advertencias visibles y una liberación bloqueada (nunca un envío aparente), y bastaba **un** producto así para que no se liberara nada del envío. **Desde la etapa K1.1 un envío nunca se bloquea** (K11-D1): un producto sin componentes (simple o combo vacío) sin ruta propia, de su categoría ni heredada sigue «Productos sin ruta»; de una ruta solo reciben las estaciones activas y, si no queda ninguna, el producto se trata como sin ruta; una impresora desactivada deja su trabajo en «Requiere intervención» sin detener las tareas ni la pantalla. Un combo con componentes y sin ruta propia sigue sin parte propia: cada componente se resuelve solo (nunca se trata un combo como «Sin preparación»). La decisión de cada producto es una sola función de Go (`decideRoute`, `kitchen/routing.go`), la misma que usa la cobertura de Enrutamiento y Terminales.

Guardar o previsualizar un borrador no crea comandas. Confirmar registra el pedido y su intención durable de preparación en la misma transacción. Un pedido inmediato se libera al confirmarse: desde la K1.1 **toda unidad activa del envío se libera una vez** (fila canónica e inventario), con tarea o sin ella, como siempre lo hizo «Sin preparación». Hasta la 0.16.0 la ausencia de configuración conservaba el pedido bloqueado y había que usar **Liberar** después de corregir; los bloqueados que quedaron de antes se envían con «Enviar a cocina ahora» (ver la sección de la K1.1).

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

Las modificaciones antes de la primera liberación se integran en el documento inicial. Después de liberar, se utilizan las rutas históricas (de ellas solo reciben las estaciones activas, K11-D15); un componente nuevo explícito se resuelve con la configuración actual y, desde la K1.1, sigue la misma regla del envío, sin dejar aviso pendiente (K11-D2). Si una estación histórica que ya tenía la comanda está inactiva, se conserva el cambio en `kitchen_pending_changes` y se muestra como aviso pendiente (hasta la K1.1, **Modificación pendiente**). No se permite avanzar tareas con una versión anterior. Reactivar la estación y usar **Volver a avisar** (hasta la K1.1, «Revisar cambio pendiente» o «Revisar destinos y generar aviso»). El lote afectado se conserva completo hasta poder resolver sus destinos; no se envían avisos parciales silenciosos. Desde la K1.1, ese aviso se da por terminado cuando la cuenta ya no está abierta (se cobra y se cierra, o se cancela), con su registro en el historial.

Documentos independientes: una comanda por estación aunque compartan impresora. Unificación: estaciones compatibles que seleccionan la misma impresora y agrupación unificada comparten documento, manteniendo secciones. Las estaciones KDS conservan su propia tarea y requisito de recepción, aunque compartan papel con otras.

El formato admite nombre del establecimiento, notas, precios, texto normal/grande, separación 1–3 y cantidad delante o multiplicador. Los controles de usuario, ESC/GS y caracteres invisibles se eliminan del texto; solo el controlador genera comandos. No existe diseñador visual arbitrario ni ejecución de plantillas: desde la etapa 9B la presentación se configura con plantillas de bloques de una lista cerrada (sección «Plantillas, formato y logotipo · etapa 9B»), y estas opciones de Cocina siguen copiándose en cada comanda y valiendo como «Según Cocina».

Indicaciones rápidas por categoría (etapa 7.1): la captura las guarda como texto en las observaciones de la partida (`customization.notes`), unidas por comas con las escritas, por lo que la comanda las imprime en la misma línea «NOTA:» (ajustada al ancho del papel) sin ningún cambio en el enrutamiento, la plantilla ni la cola. Administración en [CATALOG.md](CATALOG.md).

Indicaciones con efecto (etapa 8, migración 0012): la partida guarda además las indicaciones resueltas en su desglose y cada parte de la comanda (`instructions.parts[].quick_notes`, omitido sin indicaciones) las recibe del enrutamiento. Una indicación con efecto («Sin cebolla», «Queso extra») ya fija la cantidad del ingrediente, así que la plantilla **imprime la indicación una sola vez**: si su texto está entre las indicaciones de la nota impresa (la de la partida para la parte principal, la del componente para cada componente de combo), se omite la línea estructurada de ese ingrediente («SIN Cebolla», «Queso: 1 (incluido 0)»). Si la comanda no muestra notas o la nota ya no contiene el texto, la línea estructurada se conserva para que cocina no pierda el cambio. Sin indicaciones con efecto la comanda es idéntica a la de la etapa 7.1 (prueba de plantilla con el texto exacto). El resumen de la partida en la captura aplica la misma regla. La pantalla de preparación no lista ingredientes, así que no cambia.

## Transportes e impresoras

**CUPS**: usa `lpstat`, `lpoptions` y `lp` locales mediante argumentos separados, sin shell. Descubre nombres de colas y opciones; una consulta fallida informa indisponibilidad, no una lista vacía ficticia. Envía archivo temporal privado UTF-8, ancho personalizado, tamaño de letra, copias y título técnico. Lo elimina al terminar. El corte exige una opción admitida por el controlador (`CutMedia`, `CutType` o `Cutter`), capacidad declarada y activación explícita.

**ESC/POS TCP**: dirección IP privada/loopback explícita, puertos 9100–9109 y tiempo límite. Codifica acentos en CP850; caracteres no representables se sustituyen. Inicialización, tamaño y corte pertenecen al controlador aislado. No acepta comandos introducidos en notas. Una conexión fallida antes de escribir es error confirmado; una escritura parcial o sin resultado concluyente es incierta. La aceptación TCP tampoco confirma papel.

**Impresora USB de una terminal** (agente de impresión, desde la etapa 9G; sección «Impresión con varias terminales · etapa 9G» al final): una impresora conectada a otra computadora se registra con su terminal y la cola de esa computadora, y la imprime la aplicación de esa terminal mientras está abierta. Una impresora de agente sin terminal (anterior a la 9G, o los destinos de aceptación de las pruebas) sigue sin imprimir: sus trabajos quedan en intervención con explicación. No se finge un envío.

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
| `intervention` | Destino desactivado, impresora de tipo agente sin terminal u otra condición que exige revisión.        |
| `cancelled`    | El operador canceló este trabajo de impresión; no cancela el pedido.                                   |

Dos trabajadores de impresión corren en Go. La selección utiliza `FOR UPDATE SKIP LOCKED`; un advisory lock de sesión por impresora permanece durante el transporte. Los trabajos de una misma impresora no se intercalan, incluso entre trabajadores de distintos procesos conectados a esa base. El intento y la lease de 60 segundos se confirman antes de cualquier envío externo. El transporte tiene límite de 12 segundos. No se conserva una transacción abierta durante la impresión.

Al caer el proceso, un trabajo `processing` cuya lease venció pasa a `uncertain`; no se reenvía. Los errores confirmados se reintentan como máximo tres veces automáticamente con espera creciente. Se registra cada intento, resultado, referencia de transporte y última falla. Si Go pierde PostgreSQL después del envío, la reconciliación posterior es conservadora.

La vista **Cola de impresión** permite consultar documento e intentos, reintentar errores conocidos, cancelar, reconocer aceptación conocida o autorizar conscientemente otro envío. Cada intervención requiere permiso, revisión y motivo de 5–500 caracteres. Cambiar de impresora exige además `kitchen.printers.manage`.

Una copia adicional usa un trabajo nuevo enlazado al original. Solo se solicita después de finalizar o resolver el anterior; se rechaza otra copia mientras exista alguna pendiente/incierta de esa misma familia. Toda escritura lleva UUID idempotente, ligado a cuerpo, ruta, actor y terminal. Repetir el mismo UUID recupera la respuesta original; no se debe regenerar tras una respuesta de red incierta.

**Límite físico:** una base transaccional y una impresora no comparten commit. Si el transporte acepta papel pero la confirmación se pierde, no puede garantizarse exactamente una copia física. Se conserva incertidumbre y se exige decisión humana; no se oculta con reintentos ciegos.

## Preparación y pantallas del POS

**Preparación** muestra estación, cuenta, producto, cantidad, notas, antigüedad, tiempos y avisos. Filtra por estación/estado y pagina hasta 250 tareas. Permite avanzar de liberado (desde la etapa M1 la tarjeta dice «Por preparar», M1-D18) a en preparación o listo, por partida o por pedido conforme a la configuración. No permite retroceder una tarea lista ni preparar unidades canceladas. Las revisiones evitan que otra terminal sobrescriba cambios recientes.

Una cuenta está lista cuando todas sus tareas activas de todas las estaciones están listas. La recepción KDS, aceptación de impresión, pago y entrega son estados diferentes. Una exención explícita sin preparación no crea tareas ficticias. Las cuentas y partidas del restaurante y para llevar muestran los estados calculados por Go; **Preparación y comandas** abre el detalle de la cuenta.

**Comandas** consulta documentos históricos y recepción KDS pendiente; **Cola de impresión** consulta trabajos e intentos; **Historial** muestra cambios y responsables. Las vistas globales de cola/historial exigen `kitchen.stations.all`, además del permiso específico. Los documentos de un cliente restringido solo incluyen sus secciones y ocultan dinero salvo configuración y permiso de precios. Desde la etapa 9F Preparación es el modo cocina de la terminal: solo las estaciones visibles (terminal ∩ usuario), tarjetas grandes con color por tiempo, sin «Actualizar» y con aviso sonoro opcional (ver «Cocina por estaciones y terminal de cocina · etapa 9F»).

## Eventos autenticados y futuro KDS

Se implementó **SSE**, equivalente funcional para este flujo, en lugar de WebSocket: las notificaciones son servidor→cliente y las acciones ya disponen de HTTP autenticado e idempotente. Evita un segundo protocolo de comandos y conserva la misma comprobación de permisos. No se presenta como una implementación WebSocket.

`GET /api/v1/events` usa cabeceras de sesión y dispositivo; Rust abre la conexión y entrega mensajes mediante `tauri::ipc::Channel`. Nunca pasa tokens al WebView. Se comprueban sesión, dispositivo y permisos durante el flujo, aproximadamente cada 750 ms. Límite dos conexiones por dispositivo, duración 30 segundos con reconexión, escrituras acotadas y frames nativos hasta 256 KiB.

El outbox PostgreSQL se escribe dentro de la transacción de negocio. Una secuencia durable con lock de publicación impide saltar commits pendientes. Cada evento tiene UUID, versión 1, secuencia, tipo, ámbito, agregado opcional, estación e instante. Solo contiene referencias de invalidación, no nombres de clientes, precios o credenciales. Ámbitos `restaurant`, `takeaway`, `kitchen`, `printing`, `configuration`; se filtran por permisos y estación (desde la etapa 9F, las estaciones visibles en la terminal para el usuario; con `kitchen.stations.all`, todas).

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

Quedan fuera: KDS independiente (desde la 9F hay un KDS básico dentro del POS), Android, offline durable, facturación electrónica, diseñador arbitrario, servicio comercial de licencias e instaladores Windows. No se ha certificado cada modelo de impresora, corte físico, detección de papel, muchas terminales o resistencia a pérdida eléctrica. Deben medirse volumen, retención de eventos/auditoría y tamaño de snapshots antes de uso comercial; el replay acotado no elimina físicamente filas del outbox.

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
- con una estación inactiva o un destino inválido, el cambio quedaba **pendiente y durable**: la respuesta de la
  cancelación decía `change_pending` y Cocina → «Pendientes de liberación y avisos» lo mostraba como «Aviso de
  cancelación pendiente» (antes de la 9D solo `item.cancelled` se llamaba así); su reintento no duplica avisos ni
  liberaciones. **Desde la etapa K1.1** una cuenta cancelada no deja nada pendiente (K11-D4 y K11-D21): lo no enviado
  se cancela sin papel **antes** de mirar el aviso (hasta la 0.16.0 el aviso pendiente salía antes y dejaba también el
  envío bloqueado), y el aviso para la estación desactivada se da por terminado en la misma transacción
  (`change.dismissed` con `account_cancelled`); la respuesta, su reintento y la lectura dicen `cancelled`, y «Volver a
  avisar» responde 409 `KITCHEN_CHANGE_NOT_PENDING` sin imprimir;
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
**la cocina todavía no lo recibió**. **Desde la etapa K1.1 casi nunca aparece**: un envío ya no se bloquea, así que solo
la dicen un pedido al que se le quitó la hora de su programación **con la 0.17.0** (espera en «Por enviar a cocina» a
que alguien lo envíe, K11-D9; desde la 0.18.0 quitar la hora lo envía en ese momento, M1-D17) y un bloqueado de antes
de la 0.17.0 de una cuenta todavía abierta. Desde la etapa M1 tampoco la lleva un producto que no necesita preparación
(`preparation_required: false`, M1-D15). Lo que sigue es cómo era hasta la 0.16.0, cuando **su liberación quedaba
bloqueada**:

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
- las liberaciones bloqueadas de cuentas ya cerradas seguían en esa lista (limitación observada en la Fase 0 de la 9D;
  **corregida en la etapa K1.1**: la `0030` las da por terminadas y, desde la 0.17.0, cobrar y cerrar o cancelar la
  cuenta también);
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

## Impresión · etapa 9F (fase B)

### Sin impresora válida, aviso en el momento

Precuentas, tickets de pago y documentos de caja usan la «Impresora de tickets» de Cobros › Políticas de cobro. Desde la
9F (decisión D4) solo sirve si está configurada, activa y no es de otra terminal (desde la revisión independiente, B3,
Cobros solo ofrece esas impresoras —las demás aparecen deshabilitadas con «desactivada» o «Esta impresora no se puede
usar para imprimir»— y Go rechaza elegir otra con `422 RECEIPT_PRINTER_UNAVAILABLE`); si no, Go avisa **antes de
encolar**
(no queda un trabajo fallido en la cola) y la interfaz lo dice al momento: «No hay impresora configurada para
precuentas» (con el nombre visible de la precuenta), con **«Configurar impresora»** (lleva con el único `requestScreen`
a Administración › Cobros con «Políticas de cobro» desplegada; solo si el usuario puede elegir la impresora, si no el
aviso dice a quién pedirlo) y **«Reintentar»** en el mismo aviso. El ticket de pago nunca hace fallar el pago: el
resultado del cobro dice por qué no se imprimió y ofrece los mismos dos botones («Reintentar» imprime el ticket de ese
pago; desde la revisión B1, solo el de un pago cuyo ticket quedó pendiente por esa razón y sin devoluciones). Las
comandas no cambian (hasta la etapa K1.1, una impresora inactiva bloqueaba la liberación; desde la 0.17.0 su trabajo
queda en «Requiere intervención» con «La impresora X está desactivada.», K11-D1) y una de otra terminal deja la comanda en
«Requiere intervención», ahora con «Esta impresora no se puede usar para imprimir.» (hasta la etapa A3, «Esta
impresora todavía no puede usarse para imprimir.»).

### Reintentar sin motivo, reimprimir con motivo

- Lo que **nunca salió en papel** (error confirmado o en intervención) se reintenta con un toque, sin motivo: desde la
  cola («Reintentar», `kitchen.queue.retry`) y desde el propio documento (documentos de la cuenta y de Caja, con el
  permiso de imprimirlo: `payments.tickets.print` o `cash.reports.print`, sin necesitar la cola de cocina). El aviso
  dice qué pasó en palabras del restaurante, derivado de la clave del intento (`unreachable`: «La impresora de caja no
  responde» o «La impresora … está apagada o sin conexión»; `not_found`, desde la revisión independiente (B4): «No se
  encontró la impresora de caja (o la impresora …) en el equipo», cuando la cola CUPS no existe o la impresora de
  Windows no está instalada o se renombró; `inactive`: «La impresora … está desactivada»; `unsupported`: «Esta
  impresora no se puede usar para imprimir»; `setup`: «… necesita que se revise su configuración»). Cuando un
  documento que la pantalla veía en camino a la impresora falla, aparece además un aviso que se cierra solo con
  «Reintentar» y, si se resuelve configurando la impresora (`not_found`, `inactive`, `unsupported`, `setup`), también
  «Configurar impresora» (en Caja, junto a «Reintentar» del documento).
- **El reintento sale con la impresora vigente** (revisión independiente, M2): desde la cola, la configuración actual
  de la impresora del trabajo (o la elegida); desde el documento, la impresora de tickets de Cobros con su
  configuración actual. Así «Configurar impresora» + «Reintentar» funciona tanto si se corrigió la cola o la dirección
  de la misma impresora como si se eligió otra. Si el papel o las columnas cambiaron, el documento se prepara de nuevo
  con los mismos datos y la misma versión de plantilla (como la reimpresión de una comanda); una muestra del diseñador
  o un documento de caja anterior a las plantillas no se pueden preparar de nuevo (409 `PRINT_RENDER_UNAVAILABLE`: se
  cancelan y se vuelven a pedir). La reimpresión con motivo de precuentas, tickets y documentos de caja también sale
  en la impresora de tickets vigente con su destino actual, pero idéntica al original: conserva el papel y las columnas
  con que se emitió (la reimpresión exacta de la 9C).
- Lo que **pudo haber salido** (`uncertain`) sigue exigiendo decisión con motivo en la cola.
- **Reimprimir** lo que ya salió (o se canceló) pide motivo de 5 a 500 caracteres en cocina (como antes), en las
  precuentas y tickets y en los documentos de caja; el motivo queda en el Registro de movimientos y en el historial del
  trabajo. Desde la revisión independiente (B2), «Imprimir con la plantilla vigente» de cobros y caja también lo pide
  (como la de cocina): saca otra copia en papel de un documento ya emitido.

### Pantalla «Cola de impresión»

La lista tiene altura limitada y desplazamiento propio; «Consultar trabajo e intentos» abre el detalle a la vista junto
a la lista (fijo al desplazarse; en pantallas angostas debajo, con desplazamiento hasta él). Cada trabajo dice qué
documento es («Precuenta · #12», «Comanda · #7»), su impresora, su estado y qué pasó, sin la referencia técnica del
sistema de impresión ni estados en inglés; esos datos quedan en «Información para soporte» del detalle. Los trabajos
repetidos de lo mismo (las precuentas de una cuenta, las copias y reintentos de un documento, seguidos en la página, con
la misma impresora y el mismo estado) se agrupan con «×N» y «Ver los N trabajos»; la paginación de Go no cambia.

### Corte de papel (decisión D5)

Por impresora, en «Corte de papel y respaldo»: **«Cortar el papel al terminar»** (una sola casilla; Go guarda
`supports_cut = cut`), **tipo de corte** total o parcial y **«Renglones de avance antes del corte»** (lista cerrada 0–8,
4 en impresoras nuevas). Activado por omisión en impresoras **nuevas** de 80 mm y desactivado en las de 58 mm (muchas
no tienen cortador); las impresoras existentes conservan su papel: las que ya cortaban quedan sin renglones de avance
elegidos (`null`, «Como hasta ahora» en la lista) y cortan como hasta la 9E; las que no cortaban reciben 4.
Cambiarlo exige `kitchen.printers.manage` y queda en el Registro de movimientos («Impresora guardada»).

Bytes al final de **cada documento** (tickets, precuentas, comandas, avisos, cortes y comprobantes de caja, prueba de
ancho y prueba técnica; nunca el pulso del cajón): con corte, el salto que cierra el último renglón si quedó abierto,
**N renglones en blanco** y `GS V 0` (total) o `GS V 1` (parcial); **sin corte, los tres saltos de siempre, byte a byte
iguales a la 9E**. Revisión independiente (M1): precuentas, tickets y documentos de caja terminan sin salto y una
impresora solo imprime un renglón al recibir su salto y solo corta al inicio de uno; antes, con 0 renglones, `GS V`
llegaba con la leyenda «no es un comprobante fiscal» todavía en el búfer y el avance contaba un renglón menos que en
las comandas. Ahora el avance es el mismo en todos los documentos. Una impresora que ya cortaba antes de la 9F (sin
renglones elegidos) y las fotografías de esa época conservan su final: tres saltos fijos y `GS V 0`, porque ningún
número reproduce a la vez el papel de las precuentas (que terminan sin salto) y el de las comandas (que terminan con
él); al elegir sus renglones pasa al avance uniforme. Es el mismo final en el texto plano
(`kitchen.ESCPosBytes`) y en el ESC/POS con formato (`documents.EncodeESCPos`, `PaperCut.Ending`), por red, por la cola
de Windows (RAW) y por CUPS raw con «Esta impresora entiende ESC/POS». **Por CUPS de texto** no se pueden enviar
comandos ESC/POS: el corte lo hace el controlador con «Opción de corte anunciada por CUPS» si está configurada; sin
ella, el documento **se imprime sin corte** (antes quedaba en «Requiere intervención», y una preferencia de papel
detenía la impresión).

**Impresoras sin cortador**: la mayoría ignora `GS V`, así que activar el corte no causa problemas; si una imprime
caracteres extraños al final, desactívalo. Para probarlo, «Imprimir prueba de ancho» también corta con la
configuración elegida (guárdala antes de probar).

### Datos de prueba de la aceptación

Desde la 9F las pruebas de navegador usan como impresora de tickets «Tickets controlados E2E» (`payments-flow.ts`) y
«Tickets controlados 9B» (`workflow-stage9b-designer.spec.ts`): una cola del equipo que no existe en ningún equipo
(`ADEL_E2E_SIN_IMPRESORA`), válida para precuentas y tickets pero que nunca imprime (cada trabajo termina en error
confirmado sin enviar nada). Sus firmas están en `backend/internal/maintenance/signatures.go`, de modo que «Quitar datos
de prueba» también las reconoce.

**Pendiente en hardware**: el corte total y parcial y el avance en papel, en una impresora con y sin cortador, por red,
USB en Windows y CUPS, son prueba física del propietario.

## Cocina por estaciones y terminal de cocina · etapa 9F

Fase C de la etapa 9F (§5, decisiones D6 y D7 del coordinador). Guía para el propietario:
[TERMINAL_DE_COCINA.md](TERMINAL_DE_COCINA.md). Migración: sección «Fase C» de la 0022 (esquema 22).

- **Estaciones que muestra esta terminal** (`kitchen_device_stations`, antes ámbito de autorización): marca
  `devices.kitchen_all_stations` («todas», la predeterminada, incluidas las estaciones que se creen después) o una
  selección de 1 a 100 estaciones existentes. La 0022 dejó con su selección (marca en falso) a las terminales que ya
  tenían filas y en «todas» a las demás. Se configura en **Cocina › Estaciones › Computadoras en modo Cocina** (hasta la etapa K1.1, «Terminales de cocina»)
  (`PUT kitchen/access`, `kitchen.stations.manage`, historial auditado `station.access.saved`), junto a las
  estaciones; la pantalla de inicio sigue en Sistema › Terminales, que remite allí. Se eligió ese lugar porque la
  ruta, el permiso y la auditoría ya existían y porque elegir estaciones exige consultarlas.
- **Estaciones que puede atender** cada usuario: marca `users.kitchen_all_stations` (todas por omisión) o una selección
  en `kitchen_user_stations`. Pestaña **Estaciones de cocina** de Personal › Usuarios (`GET` con `users.read`, `PUT`
  con `users.manage`; nadie cambia las propias ni las de un usuario con más permisos; auditada
  `user.kitchen_stations_changed`). Quien tiene `kitchen.stations.all` atiende siempre todas (la pestaña lo dice).
- **Se ve la intersección** (paquete neutral `internal/stationscope`, una sola regla para cocina, eventos y
  administración): estaciones de la terminal ∩ estaciones del usuario. Rige Preparación (`GET kitchen/board`, que
  además informa `station_scope` y `sound_alert`), los cambios de estado (`POST kitchen/states`: una tarea de una
  estación no visible responde `403 STATION_SCOPE_DENIED` sin cambiar nada), los acuses de recepción y el filtro por
  estación. Las **comandas guardadas** (lista, detalle, vista previa de la cola y del diseñador) y el **tiempo real**
  usan la intersección para quien no tiene `kitchen.stations.all` y todas para quien lo tiene, porque la cola, la
  reimpresión y el historial son administrativos. Liberar a mano, programar, la cola y el historial siguen exigiendo
  `kitchen.stations.all`. Sin estaciones en común, Go lo informa (`station_scope.common = false`, con los nombres) y
  la pantalla muestra un aviso en lugar de una pantalla vacía.
- **Un producto en varias estaciones** (§5.3): un producto simple con una ruta a dos estaciones llega **completo** a
  ambas (los «4 filetes» de la prueba de Windows eran esa configuración); un **combo** reparte sus componentes por sus
  rutas: cada estación recibe solo su parte, «Marcar producto listo» marca solo esa tarea y la partida (también en la
  cuenta) queda lista cuando todas sus tareas lo están. Era el comportamiento previo; lo confirma
  `TestStage9FKitchenStations` (`operations`). Los **complementos** de un grupo (por ejemplo, papas como guarnición
  de una hamburguesa) **no se reparten**: viajan como opción de su producto a la estación de ese producto aunque estén
  ligados a un producto con otra ruta (la misma prueba lo comprueba). Repartirlos cambiaría las reglas de enrutamiento
  y no se implementó.
- **Modo cocina de Preparación** (`pages/kitchen/PreparationBoard.tsx`, `PreparationTaskCard.tsx` y las reglas puras
  de `preparationModel.ts`): tarjetas con letra grande, la parte de la estación a la vista (no plegada), «#folio ·
  cuenta · mesa» sin repetir la mesa cuando la cuenta se llama igual (sin distinguir mayúsculas ni acentos), botones de
  64 px, más antiguos primero (orden de Go), color por tiempo contra el objetivo de la estación (D7): normal, **ámbar**
  desde la advertencia (o el 75 % del objetivo si la advertencia no es menor) con «Por vencer» y **rojo** al pasar el
  objetivo con «Atrasada», solo con tokens (`--warning*`, `--danger*`). El atraso (`overdue_minutes`) ya no colorea.
  Sin «Actualizar» (sondeo de 5 s y tiempo real); «Reintentar» solo con «Sin conexión».
- **Aviso sonoro** (`devices.kitchen_sound_alert`, desactivado por omisión): dos tonos cortos generados con Web Audio
  (`services/kitchenSound.ts`, sin archivos ni red) cuando aparece en Preparación una liberación nueva y reciente de
  las estaciones visibles; no suena al abrir la pantalla, al cambiar de filtro o página ni por cambios de estado.
  WebKit (la ventana de macOS) y WebView2 solo dejan sonar el audio que se preparó durante un toque o una tecla:
  desde la revisión independiente (parte 3, B2), con el aviso activado, el primer toque o tecla en Preparación (y
  activar la casilla en Computadoras en modo Cocina) crea o reanuda el contexto de audio, y si al llegar una comanda el equipo
  todavía no permite reproducir, se avisa una sola vez «Toca la pantalla para activar el aviso sonoro.» en lugar de
  callar el fallo.
- **Terminal y lista vacía** (revisión independiente, parte 3, B1): hasta la 9E una terminal sin estaciones no mostraba
  ninguna a quien no tenía `kitchen.stations.all`; desde D6 muestra todas. Para que un cliente anterior no abra una
  terminal a todas por accidente, `PUT kitchen/access` sin `all_stations` y con la lista vacía responde `422
KITCHEN_ACCESS_ALL_REQUIRED`. La descripción del permiso `kitchen.stations.manage` pasa a «Administrar estaciones y
  las estaciones que muestra cada terminal».
- **Estaciones del usuario** (revisión independiente, parte 3, B4): `PUT users/{id}/kitchen-stations` no lleva
  `operation_id`; si lo pedido ya es lo guardado (el reenvío de un guardado cuya respuesta se perdió), responde con lo
  vigente sin escribir en lugar de «cambiaron en otra terminal».
- **Quitar datos de prueba** (fase A) desvincula también las estaciones de prueba de los usuarios
  («Estaciones que atiende «…»»); el usuario conserva su marca de selección (no se le amplía a «todas»). Una
  **terminal** cuya selección eran solo estaciones de prueba no se queda sin ninguna: pasa a «Todas las estaciones» y
  la vista previa lo dice («La terminal «…» mostraba solo estaciones de prueba; mostrará todas las estaciones.»;
  revisión independiente, parte 3, B3).

**Verificación**: integración PostgreSQL (`operations`, `administration`, `database`), Vitest y
`workflow-stage9f-kitchen-stations.spec.ts` (Chromium y WebKit). **Pendiente**: pantalla táctil física, sonido en el
equipo real y dos terminales a la vez.

## Impresión con varias terminales · etapa 9G (fase E)

Fase E de la etapa 9G (§6, decisiones D10 y D13 del coordinador; avance en
[ETAPA_9G_AVANCE.md](ETAPA_9G_AVANCE.md)). Migración: sección «Fase E» de la 0023 (esquema 23). Rutas en
[API_CONTRACTS.md](../API_CONTRACTS.md), «Etapa 9G · impresión con varias terminales».

- **Impresoras de red y del equipo servidor**: sin cambios; las usa el trabajador de Go.
- **Impresora USB de una terminal** (transporte `agent` con `kitchen_printers.agent_device_id`; CHECK: solo una
  impresora `agent` tiene terminal). Se registra en **Cocina › Impresoras** con «Impresora USB de una terminal»:
  primero la terminal («Conectada» o «Desconectada») y después una de las impresoras que informó su computadora; las
  que no sirven aparecen deshabilitadas con el motivo (las decide Go con la misma regla del cajón por CUPS: USB,
  serie, paralelo o AppSocket con controlador; nunca una clase, una impresora compartida de otra computadora ni una
  de documentos AirPrint) y Go lo vuelve a comprobar al guardar. Recibe siempre ESC/POS sin convertir (formato,
  logotipo y corte en los bytes, como una impresora de red); no tiene opción de corte de CUPS ni la marca «Esta
  impresora entiende ESC/POS».
- **Agente de impresión** (`desktop/src-tauri/src/print_agent.rs`, D13): mientras la aplicación está abierta, aunque
  nadie haya iniciado sesión, la terminal informa sus impresoras (`lpstat -a` y `lpoptions -p`, solo lecturas; al abrir
  y cada 5 minutos), consulta cada 3 s y con cada evento del canal en tiempo real, reclama de la **misma cola
  durable** el siguiente trabajo de sus impresoras (Go confirma el intento y la **lease de 60 s** antes de entregarlo,
  con los bytes ESC/POS ya preparados: texto CP850, documento con formato o el pulso del cajón; las copias repetidas
  en los mismos bytes), lo entrega con `lpstat -p <cola>` y `lp -d <cola> -o raw -n 1 -t ADEL-<trabajo> <archivo>`
  (argumentos separados, archivo privado que se borra, sin intérprete de órdenes) y confirma el resultado con las
  reglas de siempre: aceptado por la cola de su computadora (`submitted`, nunca papel), error confirmado sin enviar
  nada (`not_found` si la cola no existe, `unreachable` si no respondió) o incierto si `lp` empezó y no devolvió un
  resultado inequívoco (nunca se reenvía solo). Un reclamo o un resultado sin respuesta se repite igual (Go no
  duplica: la misma clave de reclamo devuelve el mismo intento y el mismo resultado responde lo guardado) y un intento
  ya entregado nunca se vuelve a imprimir. Un resultado que llega con la lease vencida no se acepta: el trabajo queda
  «Resultado incierto · requiere revisión» y el informe tardío en su historial. El trabajador del servidor nunca envía
  un trabajo de una terminal; solo deja incierto el que su agente dejó en proceso con la lease vencida.
- **Terminal apagada o sin conexión**: el trabajo espera en la cola. Si la terminal no consultó en 30 s
  (`devices.print_agent_seen_at`) o fue revocada, Go calcula el problema `agent_offline` (`kitchen/print_problems.go`)
  y la cola, los documentos de la cuenta y los de Caja dicen «La Caja 2 está desconectada.» («La terminal Mostrador
  está desconectada.» si el nombre no empieza por Caja, Terminal o Computadora) con «Se imprimirá sola cuando esa
  terminal esté encendida…»; no se ofrece «Reintentar». Al volver, el agente imprime lo pendiente en orden.
- **Cajón**: la impresora USB de una terminal abre el cajón de su caja (Caja › Configuración la ofrece como
  compatible, «USB de Caja 2»); el pulso nunca se reintenta solo, como siempre.
- **Impresora de tickets por caja** (D10): en **Cobros › Políticas de cobro**, «Impresora de tickets por caja» elige
  la impresora propia de cada caja (por ejemplo, la USB de la Caja 2); sin una propia, la caja usa la impresora de
  tickets general. Precuentas, tickets de pago, documentos de caja y sus reintentos y reimpresiones salen en la
  impresora de la caja de la terminal que los pide (`payments/document_printing.go`, `receiptPrinter`). El cambio se
  audita en `payments.settings_changed`. «Quitar datos de prueba» devuelve a la general una caja cuya impresora propia
  era de prueba.
- **Dónde está cada impresora**: al elegir impresora para estaciones, respaldo, reenvío desde la cola, tickets y
  cajón se ve «USB de caja · en Caja 2». **Sistema › Terminales › Ver configuración** muestra las impresoras de
  AD-EL POS de la terminal y las de su computadora (cuál se puede usar y por qué no), si está conectada y cuándo
  informó su lista. El informe se audita (`device.local_printers_reported`, autor: la terminal) solo cuando la lista
  cambia.
- **Windows**: la cola de impresión de Windows del agente (EnumPrinters y RAW con winspool, como
  `kitchen/windows_queue.go` de Go) es de la Fase W; hasta entonces el agente no arranca en Windows y una terminal de
  Windows no informa impresoras.

**Verificación**: pruebas unitarias de Go (bytes del agente, resultados, regla de colas), integración PostgreSQL
(`TestStage9GPrintAgent` en `operations` y la sección E en `TestStage9GAndroidUpgrade`), pruebas de Rust con un
ejecutor simulado (argumentos separados, `lpstat`, reintentos sin duplicar), Vitest y
`workflow-stage9g-print-agent.spec.ts` (Chromium y WebKit, agente simulado en el puente; su impresora de prueba,
«Impresora USB 9G-E», usa la cola `ADEL_E2E_SIN_IMPRESORA`, que no existe en ninguna computadora, y tiene su firma en
`maintenance/signatures.go`). **Pendiente en hardware**:
papel, logotipo, corte y cajón reales por la impresora USB de otra terminal y la terminal Windows (Fase W). Para el
propietario: [INSTALAR_EN_RED.md](INSTALAR_EN_RED.md), «Impresora USB conectada a una caja».

## Pedidos mixtos y productos sin ruta · etapa K1.1

Etapa K1.1 (servidor 0.17.0, esquema 30; especificación
[etapas/ETAPA_K1_1_COCINA_AJUSTES.md](etapas/ETAPA_K1_1_COCINA_AJUSTES.md) y decisiones K11-D1 a K11-D23 en
[ETAPA_K1_1_AVANCE.md](ETAPA_K1_1_AVANCE.md)). Migración `0030_productos_sin_ruta.sql`, sección «Fase A». Rutas en
[API_V1.md](API_V1.md), «Configuración guiada de cocina (etapa K1.1)». Guía para el propietario:
[COCINA_INSTALAR.md](COCINA_INSTALAR.md), sección 4.

**La causa** (Fase 0). `prepareRelease` juntaba las advertencias de todas las unidades del envío y, con una sola, dejaba
la liberación `blocked` sin liberar ninguna unidad. Un machacado con una Coca Cola sin ruta no llegaba a la pantalla de
cocina, la cuenta decía «Enviado · sin confirmar en cocina» y el pedido seguía en el panel de Preparación aunque la cuenta
se cobrara y se cerrara (cerrar no llamaba a cocina y el panel no miraba el estado de la cuenta).

### Un envío nunca se bloquea (K11-D1 y K11-D2)

- **Un producto sin componentes** (simple o combo vacío) sin ruta propia, sin la de su categoría y sin estaciones
  heredadas de su combo sigue el ajuste **«Productos sin ruta»**: «No necesitan preparación» (sin parte ni tarea) o
  «Enviar a una estación» (su parte va a esa estación si está activa; si no, como «No necesitan preparación»).
- **De una ruta solo reciben las estaciones activas**; las desactivadas se omiten y, si no queda ninguna, el producto se
  trata como sin ruta. Un combo con componentes cuya ruta solo iba a estaciones desactivadas queda sin parte propia y sus
  componentes se resuelven solos (K11-D16).
- **Una impresora desactivada** de una estación «Solo impresora» o «Impresora y pantalla» no detiene nada: las tareas y la
  comanda se crean, el trabajo queda en «Requiere intervención» con «La impresora X está desactivada.» y, con «Impresora
  y pantalla», la pantalla de cocina lo recibe igual.
- **Toda unidad activa del envío se libera una vez** (fila canónica de `kitchen_unit_releases` e inventario), con tarea o
  sin ella, como siempre lo hizo «Sin preparación». Un combo con componentes y sin ruta propia nunca se trata como «Sin
  preparación» (dejaría de visitar sus componentes).
- **La decisión se congela** en `routing_snapshot`, con dos campos opcionales de `Route`: `unrouted: true` (vino del
  ajuste) y `omitted_station_ids` (estaciones desactivadas que no recibieron). Una modificación posterior sigue la ruta
  congelada, de la que solo reciben las estaciones activas, y nunca aplica el ajuste (K11-D15). `POST/PATCH
kitchen/routes` rechazan esos dos campos.
- **Lo que no necesita preparación no cuenta** para la tarjeta de la pantalla de cocina (K1-D5) ni para el aviso de
  platillos listos (A2): una unidad sin tarea no está en la tarjeta y nunca avisa. La tarjeta de «machacado + Coca Cola +
  papas» sigue abierta hasta marcar el machacado; entonces se cierra, el mesero recibe **un** aviso y la cuenta queda
  lista.
- **Cambios de algo ya enviado**: un componente nuevo sigue la misma regla y ya no deja aviso pendiente. El único aviso
  pendiente que queda es el de una estación desactivada que **ya tenía** la comanda (regla de la etapa 5 y de la 9D).
- `prepareRelease` ya no escribe `blocked`: ese valor queda solo en las liberaciones anteriores a la 0.17.0 (el `CHECK`
  lo conserva). La función que decide cada producto es una sola (`decideRoute`, `kitchen/routing.go`), la misma del
  envío y de la cobertura.

### «Productos sin ruta» (K11-D3)

- Ajuste general en **Cocina › Enrutamiento**: «No necesitan preparación» (el valor al actualizar) o «Enviar a una
  estación», con la estación (solo las activas; la guardada, si se desactivó, se ve sin poder elegirla). Debajo, «Lo que
  no va a ninguna estación» («Sin ruta: Refrescos, Extras (no necesitan preparación).» o «(van a Cocina).») y los avisos
  de las rutas a estaciones desactivadas y del ajuste, con «Cambiar la ruta».
- Se guarda en `kitchen_settings.unrouted_station_id` (`NULL` = «No necesitan preparación»), **fuera** de la
  configuración que viaja en cada comanda, con `PUT kitchen/unrouted` (`kitchen.routing.manage`, `operation_id`,
  `expected_station_id`; 409 `KITCHEN_UNROUTED_CHANGED` si otro lo cambió, 400 «Elige una estación activa.»), historial
  `unrouted.saved` y el evento `kitchen.unrouted.changed`. `GET kitchen/settings` trae `unrouted_station_id`; `PUT
kitchen/settings` y `PUT kitchen/timing` lo aceptan en el cuerpo y no lo cambian.
- Si la estación elegida se desactiva, lo que no tiene ruta no necesita preparación mientras tanto (con su aviso).
  «Quitar datos de prueba» lo vuelve a «No necesitan preparación» antes de quitar o desactivar una estación de prueba
  (K11-D19), con su renglón en la vista previa.

### La cobertura del enrutamiento (K11-D5)

`GET kitchen/coverage` (`kitchen.stations.read`, capacidad `kitchen-coverage-v1`, solo lectura) resuelve **en Go** cada
producto activo con la misma función del envío (`kitchen/coverage.go`): las estaciones con sus categorías (y cuántos de
sus productos llegan de verdad) y sus excepciones por producto, lo que tiene «Sin preparación», lo que no va a ninguna
estación por una ruta (`no_route` o `inactive_station`), cada categoría con su ruta, las pantallas de cocina con las
estaciones que muestran y los avisos, con el texto listo para la pantalla:

| Código                       | Texto                                                                                                     |
| ---------------------------- | --------------------------------------------------------------------------------------------------------- |
| `display_without_stations`   | «Esta pantalla no tiene estaciones.»                                                                      |
| `display_no_screen_stations` | «Ninguna de sus estaciones recibe comandas en pantalla.»                                                  |
| `station_without_routes`     | «La estación Cocina no tiene categorías ni productos asignados.»                                          |
| `route_station_inactive`     | «Extras va a Barra, que está desactivada: sus productos se tratan como productos sin ruta.»               |
| `unrouted_station_inactive`  | «Los productos sin ruta iban a Barra, que está desactivada. Mientras siga así, no necesitan preparación.» |
| `station_printer_inactive`   | «La impresora Cocina 80 de Cocina está desactivada: sus comandas esperan en la cola.»                     |

Un combo vacío en la cobertura es el que tiene todos sus componentes en cero por omisión (K11-D17); en el envío manda el
pedido real. React no calcula rutas: solo agrupa lo que Go resolvió.

### Lo pendiente de una cuenta que ya no está abierta (K11-D4, K11-D18 y K11-D20)

- **La regla**: una liberación sigue pendiente si alguna de sus unidades activas está en una cuenta abierta (por la
  pertenencia actual de las unidades: lo combinado, dividido o transferido a una cuenta abierta sigue pendiente); un
  aviso, si alguna de sus unidades activas está en una cuenta abierta o su cuenta (o aquella en que terminó combinada)
  sigue abierta. Un pedido para llevar cobrado y todavía no entregado sigue abierto: su pedido programado llega a la
  cocina a su hora.
- **Al cerrar** (`payments.closeIfSettled`, en su transacción y con el 7820148305 que ya tomó el cobro, sin lock nuevo) y
  **al cancelar** (`account.cancelled`), `dismissAccountPending` (`kitchen/pending.go`) toma la cuenta y las combinadas
  en ella: las liberaciones `pending`, `scheduled` o `blocked` pasan a `cancelled` (`release.dismissed`) y los avisos
  reciben `dismissed_at` y `dismissed_reason` (`change.dismissed`), con el motivo (`account_closed` o
  `account_cancelled`) y el actor y el dispositivo de la operación (sin `operation_id`, K11-D20), y publica
  `kitchen.releases.changed`. Reabrir una cuenta cerrada (con una devolución) no revive lo terminado.
- **Nunca cambian** ventas, pagos, cuentas, partidas, unidades, cortes, inventario ni `kitchen_unit_releases`: solo el
  estado de esos pendientes y su historial. Las tareas de unidades ya canceladas quedan como estaban (K11-D21).
- **Lecturas**: `GET kitchen/board` solo trae liberaciones `pending`, `scheduled` o `blocked` de cuentas abiertas y los
  avisos no terminados, cada uno con `inactive_stations` (las estaciones desactivadas a las que no se pudo avisar);
  `VersionPreparation` y el reintento ignoran los avisos terminados.
- **«Enviar a cocina ahora»** (`POST kitchen/releases/{id}/release`) responde 409 `KITCHEN_RELEASE_NOT_PENDING` («Este
  pedido ya no está pendiente.») con una liberación ya enviada, cancelada o de una cuenta que ya no está abierta (hasta la
  0.16.0 respondía 200 sin hacer nada, o mandaba a la cocina el pedido de una cuenta ya cobrada); **«Volver a avisar»**
  (`POST kitchen/changes/{event_id}/retry`), 409 `KITCHEN_CHANGE_NOT_PENDING` con un aviso terminado.

### El panel «Por enviar a cocina» (K11-D8, K11-D9 y M1-D17)

En **Cocina › Preparación**, para quien tiene `kitchen.stations.all` (`pages/kitchen/PendingKitchen.tsx`; hasta la
0.16.0, «Pendientes de liberación y avisos»), con el resumen «N pedidos · N avisos». Lo que queda con la regla nueva:

| Caso                                                     | Título                                                  | Texto                                                                                                   | Botones                                     |
| -------------------------------------------------------- | ------------------------------------------------------- | ------------------------------------------------------------------------------------------------------- | ------------------------------------------- |
| Programado que se envía solo                             | «Programado: se envía solo a su hora»                   | «Se envía a cocina solo el {día} a las {hora}.»                                                         | «Enviar a cocina ahora» y «Cambiar la hora» |
| Programado manual                                        | «Programado: espera a que lo envíes»                    | «Programado para el {día} a las {hora}. Espera a que lo envíes a cocina.»                               | «Enviar a cocina ahora» y «Cambiar la hora» |
| Programado al que se le quitó la hora con la 0.17.0      | «Espera a que lo envíes a cocina»                       | «La cocina no lo recibe hasta que toques «Enviar a cocina ahora».»                                      | «Enviar a cocina ahora»                     |
| Bloqueado de antes de la 0.17.0, de una cuenta abierta   | «La cocina no recibió este pedido»                      | «Se envió con la versión anterior, que lo detenía por un producto sin ruta o una estación desactivada.» | «Enviar a cocina ahora»                     |
| Aviso para una estación desactivada que tenía la comanda | «La cocina no recibió una modificación» (o cancelación) | «No se pudo avisar a Barra: está desactivada.» y «Actívala en Cocina › Estaciones y vuelve a avisar.»   | «Volver a avisar»                           |

- Debajo de cada botón, su texto: «La cocina lo recibe al momento.» («…, antes de su hora.» antes de tiempo, o por qué
  no se puede sin `kitchen.release_early`); «Cambia cuándo llega a cocina. No cambia el pedido ni su precio.»; «Manda el
  cambio a la estación si ya está activa.». «Enviar a cocina ahora» solo dice que lo envió con `state: 'released'` en la
  respuesta; los dos 409 se avisan con el mensaje de Go y el panel se vuelve a leer.
- **Ya no pueden ocurrir**: «Bloqueado por configuración» de un envío nuevo (sus renglones «Sin ruta de preparación» no
  se muestran ni en los viejos) y los avisos pendientes por componentes nuevos sin ruta.
- **Fuera del panel**: el detalle de la cuenta dice «No se envió a cocina» para una liberación cancelada («Sus productos
  se cancelaron o la cuenta se cerró antes de enviarlo.»), y Cocina › Historial nombra «Pedido sin enviar dado por
  terminado», «Aviso sin entregar dado por terminado» y «Productos sin ruta actualizado», con el motivo en palabras.
- Sin la capacidad `kitchen-coverage-v1` (un servidor 0.16.0), el panel es el de siempre.
- **Desde la 0.18.0 (M1-D17) ya no entra ningún pedido nuevo en la fila «Programado al que se le quitó la hora»**:
  quitar la hora lo envía en la misma transacción. La fila se conserva para los que quedaron así con la 0.17.0 (la
  instalación del propietario puede tener alguno), hasta enviarlos o cerrar su cuenta.

### Configuración guiada en el escritorio (K11-D6, K11-D7 y K11-D22)

- **Asistente «Configurar qué recibe»** (`pages/kitchenDisplay/KitchenDisplayAssistant.tsx`): al aprobar una pantalla de
  cocina en Sistema › Terminales y desde su fila o «Ver configuración». Tres pasos con las rutas de siempre, cada una con
  su `operation_id` y su permiso: las estaciones de la pantalla (crear «Cocina» con `POST kitchen/stations`, usar una
  «Cocina» que ya existe o pasar una estación que solo imprime a «Impresora y pantalla de cocina» con `PATCH
kitchen/stations/{id}`, y, en una pantalla ya emparejada, cuáles muestra con `PUT kitchen/access`), las categorías
  (`POST kitchen/routes` o `PATCH kitchen/routes/{id}`; marcar una que va a otra estación la **cambia**, con el aviso
  «Desayunos dejará de ir a Barra.») y el resumen, leído otra vez de la cobertura.
- **Diagnóstico** en «Ver configuración» (`KitchenDisplayDiagnostics.tsx`): por estación, su recepción, sus categorías
  con cuántos productos, sus productos y si recibe lo que no tiene ruta; los avisos de Go con un botón por aviso (el
  asistente en el paso 1 o 2, Cocina › Enrutamiento o Cocina › Impresoras).
- **Estaciones**: «Terminales de cocina» pasa a **«Computadoras en modo Cocina»** («Las pantallas de cocina (tabletas)
  se configuran en Sistema › Terminales.»), y la recepción en pantalla dice «Las pantallas de cocina muestran estos
  pedidos al momento. En Comandas se ve si la pantalla ya los recibió.».
- Todo se esconde con un servidor sin `kitchen-coverage-v1`.

### La `0030` en una instalación con datos

En la misma transacción del migrador (el `host` respalda antes de migrar):

- agrega `kitchen_settings.unrouted_station_id` (`NULL`: «No necesitan preparación») y `dismissed_at` y
  `dismissed_reason` en `kitchen_pending_changes` (con su `CHECK`);
- pasa a `cancelled` (con su revisión siguiente) las liberaciones `pending`, `scheduled` o `blocked` cuyas unidades
  activas ya no están en ninguna cuenta abierta, cada una con `release.dismissed` (`account_not_open`, `migration:
'0030'`, sin actor), y da por terminados con `change.dismissed` los avisos pendientes de cuentas que ya no están
  abiertas. Sus advertencias y sus fotografías quedan como estaban;
- **no cambia ninguna otra tabla** (lo comprueba `TestStageK11Upgrade` con sumas de comprobación de todas las tablas del
  esquema 29). Los bloqueados de cuentas todavía abiertas siguen en el panel hasta enviarlos o cerrar la cuenta.

### Limitaciones

- **Hasta la 0.17.0, quitar la hora a un pedido programado no lo enviaba** (K11-D9): quedaba «Espera a que lo envíes a
  cocina» hasta que alguien tocaba «Enviar a cocina ahora». **Corregido en la etapa M1** (M1-D17): desde la 0.18.0 se
  envía en ese momento, como un pedido sin hora.
- **Inventario de los pendientes viejos** (K11-D11): lo que quedó bloqueado nunca consumió existencias, y darlo por
  terminado no registra ese consumo.

**Verificación**: integración PostgreSQL (`TestStageK11UnroutedProducts` en `operations`, `TestStageK11Upgrade` en
`database`, `contractKitchenCoverage` en `TestContractV1` y la subprueba de «Quitar datos de prueba» en
`administration`), Vitest y `workflow-stagez-k11-assistant.spec.ts` y `workflow-stagez-k11-diagnostics.spec.ts`
(Chromium y WebKit); regresión en [VERIFICATION_STAGEK1_1.md](VERIFICATION_STAGEK1_1.md). **Pendiente**: la comanda en
papel, una tableta real y la ventana nativa.

## Uniones de mesas, etiquetas por renglón y textos de Preparación · etapa M1

Etapa M1 (servidor 0.18.0, esquema 31; especificación [etapas/ETAPA_M1_MESAS.md](etapas/ETAPA_M1_MESAS.md) y decisiones
M1-D1 a M1-D43 en [ETAPA_M1_AVANCE.md](ETAPA_M1_AVANCE.md)). La aplicación de cocina no cambió (sigue en 0.1.0, M1-D14):
pinta el título que manda Go. Guía para el propietario: [UNIR_MESAS.md](UNIR_MESAS.md).

### El nombre de la unión en cocina (§1.5, M1-D14)

- **Comanda impresa** (`kitchen/releases.go`): «Salón · Mesa 5 + 6». **Pantalla de cocina** (`title` de
  `kitchen/display.go`): «Mesa 5 + 6». **Preparación** (`table_name` de la tarea, `kitchen/reads.go`) y los **avisos de
  platillos listos** (`administration/ready_alerts.go`): el nombre de la unión. Todos leen el lugar de la cuenta con el
  mismo fragmento SQL de Go (`internal/tableunions`: la unión abierta de su mesa o la mesa sola) y lo pasan por
  `documents.TableContext`, así que dicen «Mesa 5 + 6» también con mesas llamadas «5» y «6». Una mesa sola sigue como
  antes.
- **Un cambio de la unión con tarjetas en cocina** (unir, agregar, quitar o deshacerse) publica
  `restaurant.account.table_union` por cada cuenta abierta afectada, en la misma transacción. Está en
  `displayAccountKinds`: la pantalla de cocina vuelve a leer el encabezado de la tarjeta (el evento de encabezado de la
  K1, K1-D7).
- **El subtítulo de la tarjeta no repite** una cuenta sin nombre propio que se llama como la unión (con o sin «Mesa»), ni
  la referencia recortada en « + …» de una unión de nombre largo (M1-D29).

### La etiqueta de cada renglón sigue a Go (§5.1, M1-D15)

- La etiqueta de cocina de cada renglón del escritorio (captura y panel de la mesa) sale de una sola regla,
  `lineKitchenTag` (`desktop/src/services/kitchen.ts`), con el estado que manda Go y los mismos textos que la aplicación
  de meseros (`WaiterBadges.kt`): «Listo» (en turquesa), «En preparación», «En cocina» (`released`), «Programado para
  preparación», «Enviado · sin confirmar en cocina», «Cambio de cocina pendiente de revisión» y «Pendiente de
  liberación».
- **La causa** del defecto de la 0.17.0 era del escritorio: la captura ponía siempre «En cocina» con el estado real en
  gris al lado y el panel de la mesa tenía «En cocina» fijo. Fallaba igual con la marca de la pantalla de cocina que con
  la de Preparación del escritorio (el mismo campo y el mismo dibujo).

### Sin etiqueta para lo que no necesita preparación (§5.2, M1-D15 y M1-D34)

- Go manda **`preparation_required`** en cada renglón confirmado y en cada cuenta (`kitchen.VersionPreparationRequired`
  y `kitchen.AccountPreparationRequired`). Un renglón es `false` cuando ya se liberó y **ninguna** de sus unidades generó
  tarea de preparación (un producto sin ruta con «No necesitan preparación», una ruta «Sin preparación»); antes de
  liberarse (programado o pendiente) es `true`, y un combo con algún componente con tarea también (los combos no
  cambian). Una cuenta es `true` si al menos un renglón vigente lo es, o si no tiene renglones confirmados.
- Con `false`, el escritorio y Android no ponen **ninguna etiqueta de cocina**: ni en el renglón, ni en la tarjeta de la
  cuenta, ni «Preparación: …» en su encabezado; «Cocina» de la cuenta dice «Los productos de esta cuenta no necesitan
  preparación.». Un renglón cancelado conserva «Cancelado» (es su estado, no una etiqueta de cocina). Un cliente que no
  recibe el campo (un servidor anterior) lo trata como `true`.

### Quitar la hora envía a cocina (§5.3, M1-D17)

- Al integrar `account.updated` de un pedido que se queda sin hora, cocina **libera en la misma transacción** las
  liberaciones que acaban de quedar sin hora (`prepareRelease`, como un pedido sin hora). La respuesta del `PATCH` de la
  cuenta se lee después de integrar cocina, así que ya trae `released`. Reemplaza K11-D9.
- Los que se quedaron sin hora con la 0.17.0 siguen en «Por enviar a cocina» con «Espera a que lo envíes a cocina» hasta
  enviarlos o cerrar su cuenta (`PendingKitchen.tsx`, M1-D28).

### Textos de Preparación (§5.4, M1-D18 y M1-D43)

| Antes                                                                                  | Desde la 0.18.0                                                                                |
| -------------------------------------------------------------------------------------- | ---------------------------------------------------------------------------------------------- |
| «Liberado a preparación» (la tarjeta de una tarea que llegó y nadie ha empezado)       | «Por preparar» (para no confundirse con «En preparación»), también en el historial             |
| «No hay unidades de preparación en este filtro.»                                       | «No hay pedidos con este filtro.»                                                              |
| «Liberar a cocina automáticamente a su hora»                                           | «Enviar a cocina automáticamente a su hora»                                                    |
| «Esta operación ajusta preparación; conserva cantidades, precios y la hora comercial…» | «Solo cambia cuándo llega el pedido a cocina: sus productos, cantidades y precios no cambian.» |
| «…el permiso de liberar anticipadamente.»                                              | «…el permiso de enviar a cocina antes de tiempo.»                                              |

En las vistas de la cuenta, `released` dice «En cocina» (antes «Liberado a preparación»), en el escritorio y en Android.
**Sin cambiar en esta etapa** (M1-D43: la §5.4 solo pide Preparación; quedan como sugerencia): la liberación automática
y su anticipación de Cocina › Comandas y tiempos, los textos de Comandas y de Enrutamiento, «Liberación manual
revisada» del historial de cocina y el panel que solo se ve con servidores anteriores a la 0.17.0 (`PendingReleases`).

**Verificación**: integración PostgreSQL (`TestStageM1UnionReaders`, `TestStageM1NoPreparationLines` y
`TestStageM1RemovingScheduleSends` en `operations`), Vitest (`lineKitchenTag` y Preparación), Playwright
`workflow-stagez-m1-labels.spec.ts`, `workflow-stage9d-accounts.spec.ts` y `workflow-stagez-k11-diagnostics.spec.ts`
(Chromium y WebKit) y la suite `unions` de Android (u10, el refresco sin etiqueta). **Pendiente**: la comanda en papel
con una unión y una tableta de cocina real.
