# Cocina, comandas e impresión · Etapa 5

Versión 0.9.0, esquema 17 (la migración 0009 de la etapa 7.1 se aplicó a la instalación real el 9 de noviembre de 2026; ver «Cajón por CUPS en modo raw · etapa 7.1»; las 0010–0014 de la etapa 8 y las 0015–0017 de la etapa 9A se aplicaron el 10 de noviembre de 2026). La etapa 9A no cambia la cocina: los reportes de demanda por producto y por hora y de preparación por estación leen las liberaciones canónicas con su fecha de liberación, en una transacción de solo lectura y sin el lock operativo ([REPORTS.md](REPORTS.md)). Go es la autoridad de preparación y ejecuta los trabajadores sin depender de una ventana Tauri. No existe aplicación KDS independiente. Desde la etapa 8 la liberación canónica es el origen del consumo de inventario (ver «Consumo de inventario · etapa 8»). La preparación lista no significa pagado o entregado: pago y entrega pertenecen a `payments` (etapa 6).

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

El formato admite nombre del establecimiento, notas, precios, texto normal/grande, separación 1–3 y cantidad delante o multiplicador. Los controles de usuario, ESC/GS y caracteres invisibles se eliminan del texto; solo el controlador genera comandos. No existe diseñador visual arbitrario ni ejecución de plantillas.

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
