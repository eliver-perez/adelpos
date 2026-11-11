# Etapa 9B · Avance

Identidad del establecimiento y diseñador visual de documentos. Especificación del propietario:
[docs/etapas/ETAPA_9B.md](etapas/ETAPA_9B.md) (§0-B prevalece). Rama local `etapa-9b`, creada desde `main`
(`ee0330d`, etapa 9A, versión 0.9.0, esquema 17). Sin push ni merge.

## Fase 0 · Verificación contra el código

Revisión hecha el 11 de noviembre de 2026 sobre `main` (`ee0330d`): `config/product.json` declara la versión 0.9.0, la
API v1 y el esquema 17; `CLAUDE.md` confirma que 0001–0017 están aplicadas en la instalación real y que la siguiente
migración es la 0018. Solo lectura del código; no se ejecutó nada contra la instalación real.

### 1. Cómo se generan hoy los documentos impresos

Hay **tres plantillas de texto escritas en código**, todas en Go, que producen texto plano monoespaciado. La cola, los
transportes y el control de duplicados son únicos (etapa 5) y los comparten todos los documentos.

| Documentos                                                          | Plantilla                                                                           | Datos que recibe                                                                                                                                                                       |
| ------------------------------------------------------------------- | ----------------------------------------------------------------------------------- | -------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Comanda, modificación y cancelación (cocina)                        | `kitchen/template.go` → `RenderText(document, paperWidth)`                          | `kitchen.Document`: `Context` (establecimiento, zona, folio, cuenta, mesa/área o para llevar, cliente, quien capturó, pedido), `Settings` de Cocina copiados y `Sections` por estación |
| Precuenta y ticket de pago                                          | `payments/documents.go` → `renderDocument(document, width)`                         | `payments.CommercialDocument`: partidas, ajustes con motivo, IVA por tasa, total, total en dólares (7.1), este pago, pagos registrados, saldo                                          |
| Comprobantes de ingreso, retiro y gasto; corte parcial y definitivo | `payments/cash_documents.go` → `renderCashDocument(source, includeExpected, width)` | El corte (`cash_shift_reports.snapshot`, inmutable) o la operación de efectivo (`cash_operations`) y el turno                                                                          |

- **Ancho de papel**: 42 columnas en 80 mm y 30 en 58 mm (`payments.paperWidth`, `kitchen.RenderText`). La comanda
  admite el tamaño de texto «grande» de la configuración de Cocina, que se aplica a **todo el documento** y reduce las
  columnas a la mitad (21 y 15).
- **Cortes de línea**: tres comportamientos que hay que conservar para la equivalencia exacta: la comanda corta por
  carácter y conserva saltos de línea (`wrapText` + `cleanText`); precuenta y ticket cortan por carácter y convierten
  controles en espacios (`fit` + `printable`); los documentos de caja cortan por palabras (`wrapWords`). Los renglones
  «concepto … importe» (`row`) alinean el importe a la derecha.
- **Importes**: formato único `internal/moneytext` (`$1,234.56`, `US$`, signo antes del símbolo), con su tabla de casos
  compartida con React. Plurales con `internal/plural`.
- **Opciones de Cocina que cambian la comanda** (`kitchen_settings`, copiadas en cada documento): mostrar precios,
  observaciones y nombre del establecimiento, tamaño de texto, espaciado entre partidas y estilo de cantidad
  («2 x» o «× 2»).
- **Transportes** (`kitchen/transport.go`, detrás de la interfaz `Transport`):
  - CUPS: `lp` con argumentos separados, `document-format=text/plain`, `cpi=12` (6 en texto grande), papel
    `Custom.<ancho>x<alto>mm` calculado por líneas, márgenes en cero y la opción de corte que anuncia el controlador. Solo
    entrega texto: **no hay negrita, tamaños por línea ni imágenes**.
  - ESC/POS por TCP: `ESC @`, `ESC t 2` (CP850), `GS ! 0x11` solo en texto grande, el texto, tres saltos y `GS V 0` si la
    impresora corta. Es el único lugar que genera bytes de control (el texto llega saneado).
  - Desde la 7.1, `kitchen/cups_raw.go` detecta con lecturas (`lpstat`, `lpoptions`) si una cola CUPS acepta datos raw;
    hoy solo lo usa el pulso del cajón (`lp -o raw`).
- **Cola persistente** (`kitchen_print_jobs`): el trabajo guarda `content` (texto ya renderizado) y la fotografía de la
  impresora; intento y lease se confirman antes de enviar, se serializa por impresora (lock de sesión con semilla
  7820148307), `uncertain` exige resolución humana y `submitted` solo significa aceptación de CUPS o TCP. Un fallo de
  impresión nunca revierte el cobro o el cierre (punto de guardado).

### 2. Qué se guarda de cada documento emitido y cómo se reimprime

| Documento          | Se guarda                                                                                                                                                                                                            | Reimpresión actual                                                                                                                                                                                                          |
| ------------------ | -------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | --------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Comandas y avisos  | `kitchen_tickets` (`kind` `normal`/`modification`/`cancellation`, estaciones y `document` con contexto, configuración copiada y secciones) y sus trabajos                                                            | Desde la cola (`kitchen.stations.all`, motivo de 5–500 caracteres): **se vuelve a renderizar** el `Document` guardado con `RenderText` al ancho de la impresora destino (puede elegirse otra con `kitchen.printers.manage`) |
| Precuenta y ticket | `kitchen_tickets.document` con el `CommercialDocument` completo (incluido el texto), `payment_documents` (cuenta, pago, revisión y ajustes de la precuenta) y trabajos                                               | `POST payments/accounts/{id}/documents/reprint` (`payments.tickets.reprint`): **copia el texto** del trabajo original                                                                                                       |
| Documentos de caja | `kitchen_tickets.document` con el texto, el título y las referencias (turno, corte u operación, `includes_expected`), `cash_documents` (uno por corte u operación) y trabajos. **No guarda los datos estructurados** | `POST payments/cash-documents/{id}/reprint` (`cash.reports.reprint`; si incluye el esperado, además `cash.expected.read` y fuera del arqueo): **copia el texto** del trabajo original                                       |

- Ninguna copia agrega «REIMPRESIÓN» (decisión vigente desde la etapa 5) y toda copia se rechaza mientras exista otra
  pendiente o sin resolver de la misma familia (`PRINT_COPY_PENDING`). Las reimpresiones quedan en la historia de la
  cola (`print.reprinted`) y en la auditoría del módulo (`payments.document_reprinted`, `cash.document_reprinted`).
- La prueba de impresora (`POST kitchen/printers/test`) usa la plantilla de comanda con el título «PRUEBA DE
  IMPRESIÓN» (`kind = 'test'`); no es uno de los diez tipos de §20.
- Consecuencia para la 9B: la reimpresión exacta de precuentas, tickets y documentos de caja ya es una copia del texto;
  la de comandas depende de que el renderizado sea el mismo, por lo que cada comanda debe recordar su versión de
  plantilla (y los datos de identidad que use). Los documentos de caja anteriores a la 9B **no pueden volver a
  generarse con otra plantilla con sus datos originales**: solo guardan el texto (el estado de una operación puede
  cambiar después, p. ej. «corregido»), así que para ellos solo existirá la reimpresión exacta.

### 3. Datos del establecimiento que ya existen

`app.establishment` (0002) tiene nombre comercial (1–100), zona horaria, moneda, **teléfono, correo y dirección**
(opcionales), tema y fechas, y se editan en Configuración general (`PUT settings`, `settings.manage`, auditado). Los
documentos impresos solo usan hoy el nombre y la zona; los reportes PDF y Excel de la 9A, el nombre. No existen
logotipo, información adicional, textos de encabezado o pie ni ajustes de presentación. **Decisión**: la identidad
documental reutiliza nombre, dirección, teléfono y correo de `app.establishment` (sin duplicarlos) y solo agrega lo
nuevo. Las imágenes del catálogo (`catalog/images.go`: PNG o JPEG de hasta 1 MiB y 2048 × 2048, decodificación completa
y base64 por el puente) son el patrón de carga que seguirá el logotipo. La marca de AD-EL POS vive en
`assets/brand/adel-pos-tema-claro.png` y `adel-pos-tema-oscuro.png` (2048 × 682, RGBA) y no se modifica.

### 4. Cómo imprimía el sistema anterior (`CLAUDE.md` §21)

En `PuntoDeVenta.cs` y `CapturaOrden.cs` (`imprimirVenta`, `imprimirCocina`, `imprimirCocinaModificar`):

- Enviaba ESC/POS directo por TCP al puerto 9100 (librería `PrinterUtility`, comandos Epson), con codificación ASCII.
- **Logotipo**: no lo enviaba; imprimía una imagen guardada de antemano en la memoria de la impresora (`FS p 1 48`,
  imagen NV número 1).
- Encabezado del ticket **escrito en el código** (RFC, domicilio en dos líneas y teléfono) centrado; después datos a la
  izquierda: atendió, fecha (con segundos), folio de 6 dígitos con mesa y nombre.
- **48 columnas** en 80 mm (fuente A); tabla «# PRODUCTO PU TOTAL» con precio unitario, complementos en renglones
  propios y observaciones; subtotal, IVA, total y **total en dólares**, descuento y extra alineados a la derecha; pie
  centrado «GRACIAS POR SU VISITA»; saltos de línea y corte.
- Comanda: folio, mesa y nombre, tabla «CANT. PRODUCTO», complementos y observaciones; sin tamaños especiales.
- Usaba `double` para dinero y consultas concatenadas; no hay nada que copiar del código.

**Funciones del sistema anterior que no aparecen en la especificación** (se reportan y **no se implementan** sin
confirmación del propietario, como manda §21):

1. **Logotipo guardado en la memoria de la impresora (NV)**. La 9B envía la imagen raster en cada documento, como pide
   §0-B; usar la imagen NV sería otra opción por impresora.
2. **48 columnas en 80 mm**. AD-EL POS usa 42 (y 30 en 58 mm) desde la etapa 5; un número de columnas configurable por
   impresora cambiaría todos los documentos y no se agrega. Las plantillas predeterminadas conservan 42 y 30.
3. **Precio unitario (PU) por renglón**. AD-EL POS reparte centavos entre unidades (`payments.allocate`), así que las
   unidades de una partida pueden tener importes distintos: un precio unitario impreso requeriría una regla nueva.
4. **RFC en el encabezado**. No existe un campo de RFC del establecimiento; la «Información adicional autorizada» de
   §19 (texto libre) permite imprimirlo sin crear un dato fiscal.

### Conclusiones para el diseño (sin contradicciones con §0-B)

- Un renderizador único en Go (paquete nuevo neutral) reproduce exactamente las tres plantillas con los bloques
  predeterminados; las pruebas comparan contra copias literales de los renderizadores actuales, por tipo y ancho.
- La representación intermedia (líneas con alineación, tamaño, negrita e imágenes) se traduce así: un documento sin
  atributos ni imágenes viaja **exactamente como hoy** (mismo texto, mismos bytes ESC/POS y mismos argumentos de `lp`);
  con atributos o logotipo, ESC/POS por red recibe los comandos y una cola CUPS solo los recibe si admite datos raw
  (detección de solo lectura de la 7.1); en otra cola se imprime el texto y el logotipo se omite con aviso, sin fallar.
- Cada documento emitido desde la 9B guarda la versión de plantilla (y, en los de caja, sus datos estructurados); los
  anteriores equivalen a la versión 1 predeterminada de su tipo.
- No se encontró nada que obligue a cambiar una decisión del propietario: **se continúa con la fase A**.

### Forma de trabajo de la etapa

- Pruebas en copias aisladas del repositorio en el directorio temporal de la sesión, con un clúster PostgreSQL temporal
  propio (puerto 55499, solo loopback) y Go temporal en puertos propios; nunca contra `adel_pos`, `.local`, el Keychain ni
  los servicios reales (Go 8181 y PostgreSQL 55432 siguen en marcha y no se tocan).
- **Formato**: `./scripts/dev.sh format` pasa Prettier por todo el árbol y reescribiría las especificaciones del
  propietario (`docs/etapas/ETAPA_9B.md` no tiene formato Prettier). Se usa un equivalente que entrega a Prettier, gofmt
  y `cargo fmt` la lista explícita de archivos sin `docs/etapas/`, y `format-check` del mismo modo. En el primer intento
  de esta fase la exclusión no funcionó y Prettier reformateó las especificaciones: las versionadas se restauraron de
  `HEAD` y en `ETAPA_9B.md` (sin versionar) se revirtieron sus dos cambios (la tabla de fases y una línea en blanco);
  aplicar Prettier al archivo restaurado reproduce exactamente la versión reformateada, de modo que no hubo otros
  cambios.

## Fase A · Plantillas por bloques, renderizador único e impresión real

Hecha sobre `etapa-9b` (fase 0 en `fd1de81`), en una copia aislada del repositorio con el clúster
temporal 55499. Nada se ejecutó contra la instalación real.

### Qué quedó

- **Paquete neutral `internal/documents`** (no importa kitchen, payments, operations ni
  administration): diez tipos de documento, modelo de plantilla por bloques ordenados con catálogo
  cerrado por tipo (33 bloques; comunes, comerciales, de cocina y de caja; aviso fiscal obligatorio;
  esperado y diferencias protegido solo en cortes), validación estricta en Go con límites y textos
  saneados (`422` con el bloque afectado), datos tipados por familia, renderizador único a una IR
  (`Layout`/`Line`/`Image`, `Text()`, `Plain()`, identificador de bloque por línea y bloques vacíos
  con su motivo), plantillas predeterminadas, datos de muestra ficticios con las marcas de MUESTRA,
  codificación ESC/POS de un layout con atributos (`GS !`, `ESC E`, `GS v 0` en franjas) y acceso a
  las plantillas guardadas (`ActiveVersion`, `VersionByID`, identificadores fijos del sistema,
  `LoadIdentity`), con la interfaz `RasterSource` para la fase B.
- **Equivalencia exacta**: copias literales de `RenderText`, `renderParts`, `quickNoteCoverage`,
  `renderDocument`, `cashWithDollars`, `renderCashDocument` y sus ayudantes en archivos `_test.go`;
  la salida nueva es idéntica byte a byte en 6 144 comandas y avisos (incluida la prueba técnica), 168
  precuentas y tickets y más de 1 000 comprobantes y cortes, a 58 y 80 mm. Las tres plantillas en
  código se reemplazaron por el renderizador; los nombres antiguos quedan como envolturas finas solo
  en los archivos de prueba y las pruebas anteriores pasan sin cambiar sus expectativas. El camino
  plano conserva los bytes ESC/POS y los argumentos de `lp`.
- **Migración `0018_document_templates.sql`**: `document_templates` (una activa por tipo, nombre
  único por tipo sin distinguir mayúsculas, una del sistema por tipo, versión vigente de la misma
  plantilla por llave compuesta diferida), `document_template_versions` inmutables (disparador contra
  UPDATE, DELETE y TRUNCATE; plantillas del sistema de solo lectura y sin borrado), semilla de las diez
  plantillas del sistema activas con UUID fijos y el layout exacto de Go,
  `kitchen_tickets.template_version_id`, tipos de ticket `document_sample` y `document_rerender`,
  `kitchen_print_jobs.layout`, ámbito SSE `documents` y seis permisos `documents.*` solo al
  administrador.
- **Impresión real** sin cambiar reglas de negocio ni enrutamiento: comandas (versión activa al crear
  el ticket, copia de la identidad y tamaño efectivo; la reimpresión desde la cola usa la versión
  guardada; sin versión = versión 1 del sistema), precuenta y ticket (incluida la vista previa
  `GET …/precheck`), comprobantes y cortes (vista previa e impresión; el ticket guarda ahora los datos
  estructurados, sin el esperado si no se incluyó), vista previa de la cola y prueba técnica (sin
  cambios visibles). El trabajador carga los rasters y los transportes traducen la IR: plano = camino
  anterior; con atributos = ESC/POS por red, CUPS raw o texto con aviso, sin fallar.
- **API** `documents/*` (paquete `internal/designer`, delegado desde `administration/routes.go`):
  catálogo, plantillas (crear, duplicar, guardar como versión nueva, renombrar, activar, restaurar
  predeterminada), versiones, vista previa con muestra o documento real, recientes, muestras impresas
  e impresoras compatibles. Permisos antes de escribir, idempotencia por `operation_id`, revisión
  optimista (`409 REVISION_CONFLICT`), auditoría y evento `documents.templates.changed` en la misma
  transacción. «Imprimir con la plantilla vigente» en sus módulos: `kitchen/print-jobs/rerender`,
  `payments/accounts/{id}/documents/rerender` y `payments/cash-documents/{id}/rerender`.
- **Puente y servicios**: 17 operaciones nuevas en la lista cerrada de `api.rs` (con su prueba y el
  filtro `kind` en la lista blanca) y `desktop/src/services/documents.ts` (tipos y llamadas).

### Decisiones

- Las plantillas predeterminadas imprimen los renglones fijos aunque sus datos estén vacíos
  (`omit_empty: false`, como «Pedido: » en la prueba técnica); los campos que agrega el editor los
  omiten por omisión. La etiqueta de un campo se imprime seguida de un espacio y el valor.
- El nombre del establecimiento de un documento es el copiado en él (contexto de la comanda o
  `establishment` del ticket); la identidad copiada aporta los demás datos. La vista previa con un
  documento real y la impresión con la plantilla vigente usan la identidad vigente.
- Una plantilla guardada que dejara de ser válida para el catálogo (o la falta de plantilla activa)
  imprime con la versión 1 del sistema: la impresión nunca falla por una plantilla.
- Activar solo sube la revisión de la plantilla activada; la que se desactiva conserva la suya.
- La vista previa con un documento de caja lee el corte (snapshot inmutable) o la operación, de
  modo que también funciona con documentos anteriores a la 9B; la impresión con la plantilla vigente
  solo con los datos guardados desde la 9B (`409 DOCUMENT_DATA_UNAVAILABLE`).
- Las impresiones con la plantilla vigente usan la impresora del trabajo original (la cola admite
  elegir otra con `kitchen.printers.manage`) y se rechazan mientras el original o una impresión
  anterior con la plantilla vigente sigan sin resolver.

### Pruebas de la fase A

Ejecutadas por el agente de la fase en su copia aislada (clúster temporal 55499), con el árbol final:

- `go vet ./...` y `go vet -tags=integration ./...` sin hallazgos; `go test -race -count=1` aprobado en `documents`,
  `kitchen`, `payments`, `designer`, `administration`, `database` y `operations`.
- Integración PostgreSQL: `designer` (17.8 s), `database` con la prueba de la 0018 (6.3 s), `administration`
  (55.6 s), `payments` (2.2 s) y `operations` con la cadena de aceptación completa y las aceptaciones de la 9B
  (230.7 s): todas `ok`.
- Equivalencia byte a byte contra las copias literales: 6 144 comandas y avisos, 168 precuentas y tickets y más de
  1 000 comprobantes y cortes; bytes ESC/POS y argumentos de `lp` sin cambios en el camino plano.
- `cargo test --locked --offline`: 32 aprobadas y 1 ignorada (la de hardware), incluida la de la lista cerrada de
  `documents_*`; Vitest de `services/documents.test.ts` y `api.test.ts` (7 pruebas).

Cierre de la fase sobre una copia exacta del commit `9a4884d` (`git archive`, clúster temporal 55499): formato
(Prettier sin `docs/etapas/`, gofmt y `cargo fmt --check`) limpio; `go vet` con y sin `-tags=integration` sin
hallazgos; `go test -race` aprobado en `documents`, `designer`, `kitchen`, `payments`, `administration` y `database`;
integración `designer` (17.2 s), `database` (5.8 s), `administration` (54.6 s), `payments` (2.1 s) y `operations`
(230.9 s) `ok`; `tsc --noEmit` sin errores; Vitest de `src/services` (7 archivos, 90 pruebas) y `cargo test` (32
aprobadas, 1 ignorada) aprobados.

## Fase B · Identidad del establecimiento y logotipo

### Parte B1: conversión del logotipo y encabezado de los reportes

Piezas puras, sin base de datos ni HTTP, hechas en paralelo con la fase A:

- **`internal/logoraster`**: `Validate` (solo PNG o JPEG por firma y por `image.DecodeConfig`, ≤ 1 MiB, de 1 × 1 a
  2048 × 2048, dimensiones comprobadas **antes** de decodificar y decodificación completa después; rechaza SVG, GIF,
  WebP, vacíos, truncados y extensiones engañosas, con mensajes en español), `Convert` (58 mm = 384 puntos y 80 mm = 576,
  ancho relativo 25–100 %, alineación por relleno blanco hasta el ancho imprimible, transparencia a blanco, luminancia
  BT.601, escalado Catmull-Rom de `golang.org/x/image/draw` —ya estaba en el módulo; sin dependencias nuevas—, umbral
  1–255 o difuminado Floyd–Steinberg entero, altura máxima de 320 puntos ≈ 40 mm, determinista) y `Raster` inmutable
  (bits por fila, MSB primero, 1 = negro; `PNG()` de vista previa de 1 bit y `ESCPos()` con `GS v 0` en franjas de 128
  filas para impresoras con poca memoria de imagen).
- **`internal/brand`**: copia embebida byte a byte de `assets/brand/adel-pos-tema-claro.png` (la original no se tocó);
  una prueba compara bytes y SHA-256 con el archivo del repositorio.
- **Reportes PDF y Excel de la 9A**: la primera página del PDF lleva el logotipo (≤ 50 × 14 mm) con el nombre, la
  dirección y «Tel. … · correo» del establecimiento (solo los datos que existan); la hoja «Filtros» del Excel inserta el
  logotipo en A1 (anclado, sin hipervínculo), el nombre en B1 y «Dirección», «Teléfono» y «Correo electrónico» tras
  «Establecimiento». Sin logotipo del restaurante se usa el de AD-EL POS con el nombre del establecimiento. La imagen se
  vuelve a validar y a codificar sin metadatos (≤ 600 × 168 px). Comparado con el commit de la 9A: las cadenas del PDF y
  la hoja «Datos» del Excel son idénticas; contenido, columnas, totales, orden, consultas, límites y auditoría no
  cambian; la primera página crece ≈ 14.5 mm (un reporte que llenaba sus páginas puede ocupar una más) y cada archivo
  pesa ≈ 28–29 KB más.
- Pruebas (copia aislada, clúster 55499): `go vet` con y sin `-tags=integration` sin hallazgos; `go test -race` `ok` en
  `logoraster`, `brand` y `reports`; integración `internal/reports` `ok` (93 s, con el subtest nuevo del encabezado) e
  `internal/reports/operational` `ok` (40 s).

### Parte B2: identidad del establecimiento y logotipo

Segunda parte de la fase B, sobre los paquetes puros de B1 (`internal/logoraster`, `internal/brand`)
y conectada con la fase A (commit `9a4884d`). Nada se ejecutó contra la instalación real: copias aisladas del repositorio y el clúster temporal 55499.

#### Qué quedó

- **Migración `0019_document_identity.sql`** (transaccional, restricciones con nombre, sin datos
  ficticios): `app.document_identity` (fila única: información adicional, texto de encabezado y de
  pie de ≤ 300 caracteres y ≤ 8 renglones sin controles; logotipo vigente; presentación —ancho
  25–100 %, alineación, método `threshold|dither`, umbral 1–255—; conversiones vigentes de 58 y 80 mm
  con llaves compuestas al logotipo vigente y «las tres o ninguna»; `revision`, `updated_by`,
  `updated_at`; disparador contra DELETE y TRUNCATE), `app.document_logos` (original inmutable en
  `bytea`, tipo, dimensiones, tamaño y SHA-256 comprobado contra los bytes, único; quién y cuándo) y
  `app.document_logo_rasters` (conversiones inmutables por logotipo, ancho de papel, presentación y
  versión del convertidor, con ancho y alto en puntos y bits de tamaño exacto). Disparadores de
  inmutabilidad en originales y conversiones. Sin permisos nuevos (`documents.identity.manage` es de
  la 0018).
- **Paquete `backend/internal/identity`**: lectura combinada con `app.establishment` (sin duplicar
  nombre, dirección, teléfono ni correo), `PUT` con revisión, carga/reemplazo/retiro del logotipo con
  `logoraster` (validación de B1, conversión a 384 y 576 puntos guardada al cargar o al cambiar la
  presentación, reutilizada si la combinación existe; el mismo archivo se guarda una sola vez), vista
  previa como `data_uri` (original y papel), vista previa de otra presentación sin guardar,
  idempotencia por `operation_id` con el lock de fila de la identidad tomado antes de buscar el
  resultado, auditoría sin bytes ni base64 y evento SSE `documents.identity.changed` en el ámbito
  `documents`, todo en la misma transacción. Permisos en Go antes de leer el cuerpo: leer con
  `documents.templates.read` o `settings.read`; escribir y previsualizar otra presentación con
  `documents.identity.manage`.
- **Conexiones**: `documents.LoadIdentity` completa la copia de la identidad de cada documento
  (información adicional, encabezado, pie y las dos conversiones vigentes); `identity.Rasters`
  implementa `documents.RasterSource` y `DataURI` (vista previa del diseñador) y queda asignada en
  `kitchen.Worker` y `administration.Service` (`main.go`; en las pruebas, `service.documentRasters()`
  usa el pool); `documents/printers` dice en el motivo de compatibilidad del logotipo cuándo no hay
  logotipo cargado; los reportes PDF y Excel usan `identity.ReportLogo` (`UseLogoSource`) con el
  respaldo de AD-EL POS sin logotipo; `documents/identity*` se delega antes del prefijo del
  diseñador; la ruta de carga admite 2 MiB.
- **Puente e interfaz**: cinco operaciones cerradas en `api.rs` (con prueba Rust) y en `api.ts`;
  `services/documentIdentity.ts`; `pages/documents/IdentityPanel.tsx` (firma acordada con C) con
  `pages/documents/identity/` (datos del establecimiento de solo lectura con «Editar en
  Configuración general», textos con contador, logotipo con `<input type="file">` PNG/JPEG en
  base64, reemplazo, retiro con `ConfirmDialog`, original y papel de 58/80 mm, presentación con
  pasos y segmentados —nunca `type="number"`—, vista previa sin guardar con antirrebote y una sola
  petición en vuelo, conflicto de revisión sin perder lo escrito, reintento de un resultado incierto
  con el mismo `operation_id`, solo lectura sin `documents.identity.manage`, avisos con `notify.ts`,
  tokens de tema y papel blanco en ambos temas); `useRealtime` publica
  `adel-documents-identity-change` solo con `documents.identity.changed` (la lectura lleva imágenes).

#### Decisiones

- Las conversiones vigentes se guardan como referencias en la identidad (`logo_raster_58_id`,
  `logo_raster_80_id`): la copia de cada documento es un SELECT simple y nunca se convierte al
  imprimir. Las conversiones y originales nunca se borran: un documento emitido antes de reemplazar o
  retirar el logotipo se reimprime con los mismos bits.
- Con difuminado el umbral no interviene: la conversión se identifica con 128 (punto de decisión fijo)
  y se reutiliza aunque cambie el umbral guardado en la presentación.
- `converter_version` (1) forma parte de la llave de una conversión, por si el algoritmo de
  `logoraster` cambia.
- Las escrituras responden la identidad sin imágenes (así se guarda en `restaurant_operations`); la
  pantalla vuelve a consultar `GET`. Guardar sin cambios no crea revisión, auditoría ni evento.
- Se agregó `POST documents/identity/logo/preview` (no estaba en el diseño): ver otra presentación sin
  guardarla es necesario para ajustar el umbral con el dedo; no escribe nada.
- Textos: además de quitar controles e invisibles, se recortan espacios finales y renglones vacíos al
  inicio y al final antes de aplicar los límites.

#### Limitaciones

- **Papel**: el logotipo impreso es una prueba física pendiente del propietario (solo se comprobó con
  el transporte de prueba de loopback y el ejecutor simulado de CUPS).
- Un `GET documents/identity` con un original de 1 MiB pesa ≈ 1,4 MB (cabe en los 2 MiB del puente);
  el panel solo lo vuelve a pedir con su propio evento SSE o tras una escritura propia.
- Quien tiene `documents.identity.manage` sin permiso de lectura recibe la identidad en la respuesta de
  sus escrituras (no es un dato protegido).
- `config/product.json` sigue con `schema_version` 17 hasta la fase D (versión y esquema finales).
- La especificación Playwright `documents-identity.spec.ts` se escribió en esta parte, pero necesita la pantalla
  «Documentos» de la fase C: se ejecuta y se confirma con la fase C.

#### Pruebas ejecutadas (11/11/2026, copia `$SCR/repo9bB2` sincronizada tras `9a4884d`; clúster 55499)

- `gofmt -l backend`: sin archivos. `go vet ./...` y `go vet -tags=integration ./...`: sin hallazgos.
- `go test -race -count=1`: `ok` identity, logoraster, brand, documents, designer, kitchen (incluida
  `TestRealLogoThroughCUPSQueues`: logotipo real por CUPS raw con `GS v 0` y por CUPS sin raw con el
  texto, aviso y `submitted`; sin logotipo el bloque no imprime nada), administration, reports y
  payments.
- Integración (`iter_snap.sh`): `./internal/identity` ok 16 s (`TestDocumentIdentity`, 14 subpruebas:
  lectura combinada, textos con revisión/idempotencia/conflicto, PNG y JPEG con conversiones 58/80 y
  vistas previas, rechazos de SVG, GIF, PNG truncado, > 1 MiB, 2049 px, vacío y base64 inválido,
  reemplazo conservando el anterior, reutilización de conversiones, fuente de reportes en
  transacción de solo lectura sin locks, retiro, inmutabilidad, dos administradores a la vez y mismo
  `operation_id` concurrente, permisos y `access.denied` por HTTP, carga > 64 KiB por HTTP con
  auditoría sin bytes, evento SSE solo para `documents.templates.read`, PDF/Excel con logotipo del
  restaurante y respaldo de AD-EL POS, y conexión con el renderizador: copia de la identidad,
  vista previa del diseñador con `data_uri` a 58 y 80 mm, aviso de impresoras sin logotipo, muestra
  por ESC/POS a un puerto de loopback con `GS v 0` y los bits guardados, y copias idénticas tras
  reemplazar y retirar el logotipo); `./internal/designer` ok 18 s; `./internal/database` ok 6 s;
  `./internal/reports` ok 95 s; `./internal/administration` ok 56 s; `./internal/operations` ok
  226 s (incluye `testStage9BLogo`: comanda real con logotipo, IR con la conversión de 80 mm, bits
  cargados por el trabajador con `GS v 0`, reimpresión idéntica tras reemplazar y retirar, comanda
  sin logotipo sin línea de imagen).
- `cargo test --locked --offline`: 33 ok, 1 ignorada (Keychain real), incluida
  `document_identity_routes_are_closed_and_keep_write_methods`.
- `npx tsc --noEmit`: sin errores. `npm run build`: ok. Vitest: `identityPanel.test.tsx` 11 ok,
  `useRealtime.test.tsx` 4 ok (nueva: evento de identidad), `src/services` ok (105 pruebas en 9
  archivos).
- Playwright: `e2e/documents-identity.spec.ts` escrita (Chromium y WebKit, puertos 8185/1423), **no
  ejecutada**: `pages/documents/DocumentsPage.tsx` de la fase C aún no existe en el árbol.
