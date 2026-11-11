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
  cambios. Al cerrar la etapa, por indicación del propietario, `docs/etapas/` se agregó a `.prettierignore` y las
  especificaciones quedaron versionadas.

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

### Cierre de la fase B

Sobre una copia exacta del commit `5fe33a2` (`git archive`, clúster temporal 55499): formato (Prettier sin
`docs/etapas/`, gofmt y `cargo fmt --check`) limpio; `go vet` con y sin `-tags=integration` sin hallazgos;
`go test -race` aprobado en `identity`, `logoraster`, `brand`, `documents`, `designer`, `kitchen`, `reports`,
`administration` y `payments`; integración `identity` (15.9 s), `designer` (17.5 s), `database` (6.0 s), `reports`
(91.8 s), `reports/operational` (40.2 s), `administration` (54.2 s) y `operations` (226.3 s) `ok`; `tsc --noEmit` sin
errores; Vitest de `src/services` (9 archivos, 105 pruebas), del panel de identidad y de `useRealtime` (2 archivos, 15
pruebas) y `cargo test` (33 aprobadas, 1 ignorada) aprobados. La prueba de navegador de la identidad se ejecuta con la
fase C.

## Fase C · Editor visual de documentos

Pantalla **Documentos** (`documents.templates.read`) con las pestañas «Plantillas» e «Identidad del
establecimiento» (monta `IdentityPanel` de la fase B2). Sin cambios de backend, migraciones ni rutas.

- **Plantillas por tipo**: los diez tipos agrupados (Venta, Cocina, Caja); por tipo, la activa primero,
  la predeterminada marcada «Predeterminada · solo lectura» y, con `documents.templates.manage`,
  editar, duplicar, renombrar, activar (confirmación), historial, nueva y «Restaurar predeterminada»
  (confirmación: activa la del sistema y conserva las demás).
- **Editor en tres áreas** (`pages/documents/designer/`): elementos compatibles del catálogo de Go
  agrupados (con los atajos de «Campo de datos»), agregar tocando (después del bloque elegido) o
  arrastrando; vista previa con la IR exacta de Go en una tira de papel con el ancho real (30/42
  columnas), 58/80 mm, tamaños dobles, negrita y logotipo, marca «Vista previa con datos de
  muestra», documento real con `documents.preview.real`, avisos y bloques vacíos con motivo, tocar
  una línea elige su bloque; estructura con asa de arrastre, Subir/Bajar, teclado (Alt + flecha),
  quitar con deshacer y aviso fiscal no eliminable; propiedades táctiles (segmentados, interruptores,
  pasos, textos con teclado virtual, lista de datos del campo) y notas de compatibilidad por
  impresora. Antirrebote de 250 ms y una sola petición en vuelo (la última gana).
- **Guardar y versiones**: indicador de cambios sin guardar, protección al salir (editor y
  navegación principal), nota opcional, `409 REVISION_CONFLICT` con borrador conservado y opciones
  explícitas, aviso por SSE si otra persona guarda mientras se edita, historial con vista previa de
  solo lectura y «Restaurar esta versión», 422 con el bloque señalado.
- **Muestra**: impresora con ancho y compatibilidad, confirmación, estado de la cola con los
  mensajes de la 7.1 y remisión a Cocina → Cola de impresión si queda incierta o en intervención.
- **Imprimir con la plantilla vigente** junto a «Reimprimir» en documentos de una cuenta, documentos
  de caja y la cola de cocina (comandas y avisos), con su permiso y una confirmación que explica la
  diferencia con la copia idéntica.
- **Diseño adaptable**: consultas de contenedor; tres columnas desde 960 px de ancho útil, cuatro
  desde 1400 px, pestañas por debajo; en solo lectura la vista previa ocupa la columna de elementos.
  Sin desbordamiento a 1280 × 720 ni 1920 × 1080 en ambos temas (Playwright).

### Limitaciones de la fase C

- Los segmentos de línea del papel (tocar una línea para elegir su bloque) miden menos de 44 px: son una vía adicional;
  las filas de la estructura sí cumplen los 44 px.
- Arrastrar desde el panel de elementos a la estructura solo funciona cuando ambas áreas se ven a la vez (no en el modo de
  pestañas); en pantalla táctil el arrastre empieza al mover en horizontal, porque el movimiento vertical desplaza el
  panel. Agregar tocando y «Subir»/«Bajar» funcionan siempre.
- La protección de cambios sin guardar cubre la navegación de la aplicación y `beforeunload`, no el cierre de la ventana
  nativa (Tauri no lo intercepta hoy).
- En Playwright, la reimpresión exacta se compara por la API (contenido del ticket antes y después) y «Imprimir con la
  plantilla vigente» se recorre desde los documentos de la cuenta; en Caja y en la cola de cocina ese botón solo tiene
  pruebas Vitest.
- No verificado: ventana nativa WKWebView, pantalla táctil física y papel.

### Pruebas de la fase C

En la copia aislada del agente (puertos 8186/1424): Vitest 60 archivos / 529 pruebas (39 nuevas de la fase C en
`pages/documents/designer`, `templates` y `rerender.test.tsx`), `tsc --noEmit` y `npm run build`
aprobados; Playwright Chromium y WebKit: `documents-designer.spec.ts` (nueva),
`documents-identity.spec.ts` (B2), `restaurant.spec.ts` y `stock-consumption.spec.ts` aprobadas.

## Fase D · Correcciones, documentación y entrega

### Versión

`config/product.json` pasa a la versión **0.9.1** y el esquema **19** (0018 y 0019), con `./scripts/dev.sh sync`
(`sync --check`: «Metadatos coherentes»). Como en la 9A, en `Cargo.lock` y `package-lock.json` solo cambia la entrada del
propio paquete; ninguna dependencia.

### Orden de las pruebas de navegador

En el cierre de la fase C (copia exacta de `f3509d3`) pasaron el formato, `tsc --noEmit`, Vitest de `src` (60 archivos,
529 pruebas) y `npm run build`, pero la suite completa de Playwright dio 9 fallos por motor: las especificaciones nuevas
(`documents-designer.spec.ts` y `documents-identity.spec.ts`, nombre propuesto por el coordinador) se ordenaban antes que
`restaurant.spec.ts` y le consumían el folio 1 que espera, porque todas comparten la base de cada motor y corren en orden
alfabético. Se renombraron a `workflow-stage9b-designer.spec.ts` y `workflow-stage9b-identity.spec.ts`, con una nota en su
encabezado; con ese orden la suite completa aprobó en una copia del árbol (Chromium 12/12 y WebKit 12/12, 4.7 min cada
uno). Además, el filtro de archivos de Playwright no distingue mayúsculas y la ruta del directorio temporal contiene
«Documents-ADEL-POS»: un filtro `documents-` selecciona todas las especificaciones.

### Revisión independiente

Un revisor de solo lectura revisó las fases 0, A y B (commit `5fe33a2`) en copias exactas, con pruebas de reproducción
propias. **Sin hallazgos críticos ni altos**; ninguna fuga del efectivo esperado ni de las diferencias (impresión, vistas
previas, reimpresión, plantilla vigente, auditoría, eventos, errores, datos guardados y cola); permisos comprobados en Go
antes de escribir; puente cerrado; reimpresión idéntica también de documentos anteriores a la 9B; lease, reintentos y
resultados inciertos sin cambios; equivalencia exacta del texto, los bytes ESC/POS y los argumentos de `lp`; migraciones
correctas sobre un esquema 17 con datos. Encontró 5 hallazgos medios y 8 bajos, todos corregidos.

### Correcciones de la revisión

La revisión independiente de las fases 0, A y B no encontró hallazgos críticos ni altos ni fugas de datos protegidos.
Sus cinco hallazgos medios y ocho bajos se corrigieron, cada uno con pruebas que fallaban antes de corregirlo y pasan
después. Las migraciones 0018 y 0019 se ajustaron antes de aplicarse a cualquier instalación y el esquema sigue en 19.

- **Logotipo al final del documento (medio 1).** `documents/escpos.go` emite el salto de línea antes de `GS v 0` cuando
  la última línea de texto quedó sin salto (tickets, precuentas y caja). El texto y el camino plano no cambian.
- **JPEG progresivo lento (medio 2).** `logoraster.Validate` recorre los marcadores antes de decodificar y rechaza más de
  100 barridos (SOS) o 1024 segmentos. El JPEG de la revisión (1 MiB, ≈ 61 000 barridos, 70 s) se rechaza al momento;
  validar uno de 2048 × 2048 con 100 barridos tarda ≈ 0,15 s. La carga valida y decodifica el archivo antes del lock de
  la identidad (`identity/handler.go`, paso `prepare`).
- **«Importes» de la lista de cocina (medio 3).** La comanda solo guarda precios si Cocina los muestra, así que la opción
  queda en «Según la configuración de Cocina» u «Ocultar». Go rechaza «Sí» con `422 TEMPLATE_INVALID` y el catálogo
  explica que se active «Incluir importes comerciales en comandas» en Cocina. La vista previa y la muestra impresa con
  datos de muestra aplican la configuración vigente de Cocina con la misma regla que el papel.
- **Colas CUPS raw (medio 4).** Opción explícita por impresora, «Esta impresora entiende ESC/POS (formato y logotipo)»
  (`kitchen_printers.accepts_escpos`, desactivada por omisión, solo en CUPS, en la 0018). Se edita en Cocina →
  Impresoras, con texto explicativo. Con atributos o logotipo, una impresora CUPS solo recibe ESC/POS si está marcada
  **y** su cola admite raw; si no, se imprime el texto con aviso, sin fallar. `GET documents/printers` distingue
  «Desactivado en la impresora» de «Cola sin raw» (campo `code`). Las predeterminadas siguen planas e idénticas.
- **Identidad (medio 5, bajos 1 y 2).** Mientras haya un guardado en curso o sin confirmar, los textos, la presentación y
  el logotipo quedan en solo lectura; el reintento envía exactamente el cuerpo original. La vista previa sin guardar se
  vuelve a pedir al reemplazar el logotipo. Tras una escritura correcta se aplica primero su respuesta: si la relectura
  falla, la pantalla conserva lo guardado y su revisión, avisa y permite volver a cargar, sin falso conflicto.
- **Reimpresión y plantilla vigente (bajo 4).** Ambas responden `409 PRINT_COPY_PENDING` mientras el documento tenga
  cualquier trabajo sin resolver de cualquiera de las dos familias (copias exactas, impresiones con la plantilla vigente
  y sus copias), con una sola consulta (`kitchen/rerender.go`).
- **Otro ancho de papel (bajo 5).** Un trabajo con respaldo, redirigido o copiado a una impresora de otro ancho no envía
  el raster de otro ancho: la imagen se omite con aviso y el resto se imprime.
- **`GET documents/printers` (bajo 6).** Las consultas a CUPS tienen un límite total de 4 s, como el cajón. La ruta no
  toma locks consultivos y no consulta la cola de una impresora CUPS sin la marca ESC/POS.
- **Vista previa de caja real (bajo 7).** Un documento emitido desde la 9B usa los datos guardados en su ticket (lo
  mismo que imprime «Imprimir con la plantilla vigente»), con la misma regla del esperado. Solo los anteriores a la 9B
  leen el corte o la operación, y la vista previa lo avisa.
- **`GET documents/recent` (bajo 8).** Los comprobantes exigen `cash.movements.read` y los cortes `cash.shifts.read`. El
  alcance de los turnos se resuelve en una sola consulta con `LIMIT 20`, sin consultas por documento bajo el lock.

**Notas.**

- **Bajo 3 (sin cambio de comportamiento).** Al volver a subir un archivo ya cargado se reutiliza el original por su
  SHA-256, así que «Cargado: …» muestra el autor y la fecha de la primera carga. La auditoría registra bien la nueva
  carga, y la identidad muestra en «Última modificación» quién y cuándo la cambió.
- **Observación de la 0019 (aplicada).** Cada conversión vigente debe ser de su ancho de papel. Las llaves compuestas
  incluyen ahora `paper_width`, mediante las columnas constantes `logo_raster_58_paper` y `logo_raster_80_paper`.
- **Fuera del alcance.** `catalog/images.go` (etapa 3) decodifica las imágenes del catálogo con `image.Decode` sin
  limitar los barridos de un JPEG progresivo. Es el mismo riesgo del hallazgo medio 2 y no se cambió.

### Documentación

Guía nueva [docs/DOCUMENT_DESIGNER.md](DOCUMENT_DESIGNER.md) (uso para el restaurante y referencia técnica) y
[docs/VERIFICATION_STAGE9B.md](VERIFICATION_STAGE9B.md); actualizados `CLAUDE.md`, `AGENTS.md`, `README.md`,
`ARCHITECTURE.md`, `API_CONTRACTS.md` (rutas, contratos, permisos y los cambios de la revisión), `NEXT_STEPS_FOR_SOL.md`,
`docs/VERIFICATION.md`, `docs/KITCHEN_AND_PRINTING.md`, `docs/PAYMENTS_AND_REFUNDS.md`,
`docs/CASH_SHIFTS_AND_RECONCILIATION.md`, `docs/REPORTS.md` y `docs/DEPENDENCIES.md`.

### Procedimiento para la instalación real (en espera de autorización)

**No ejecutado.** Aplicar `0018_document_templates.sql` y `0019_document_identity.sql` a la instalación real
(`adel_pos`, clúster 55432, servidor 8181) requiere la **autorización expresa del propietario**; la de la etapa 9A no se
extiende a esta (§30 de la especificación). Hasta entonces la instalación sigue en 0.9.0 y esquema 17.

**Advertencia.** Desde la rama `etapa-9b`, `./scripts/dev.sh migrate`, `server-start` y `build-macos` **tocan la
instalación real**: `migrate` aplica 0018 y 0019 a `adel_pos`; `server-start` compila el binario 0.9.1, que declara
incompatible el esquema 17 (readiness distinta de 200) si se inicia sin migrar; `build-macos` sustituye el `.app` 0.9.0.
No ejecutar ninguno sin la autorización. (`./scripts/dev.sh format` ya es seguro: `docs/etapas/` está en `.prettierignore`.)

Requisitos previos: regresión final de la fase D aprobada; árbol confirmado en `etapa-9b` (anotar el commit); ningún
trabajo real en curso en el restaurante (sin cobros, cortes ni arqueo abiertos durante la ventana, y la cola de impresión
sin trabajos `processing`); servidor y clúster reales en marcha como hoy.

1. **Estado previo** (solo lectura): `./scripts/dev.sh status`; `GET /api/v1/version` responde 0.9.0 y esquema 17.
2. **Respaldo**: `./scripts/dev.sh backup` → anotar `backups/adel_pos-<fecha>.dump` y que `pg_restore --list` terminó
   sin error.
3. **Ensayo de restauración**: `python3 scripts/restore-check.py backups/adel_pos-<fecha>.dump` → todas las tablas con
   conteos idénticos (esperado: 127 tablas, las de `app` tras la 9A; en el ensayo de la 9A, antes de la 0017, fueron 126) y `.local/stageN-restore-verification.json` escrito. Si alguna tabla difiere, **detenerse**.
4. **Comprobación previa de solo lectura** (script en el directorio temporal que carga `scripts/database.py` con
   `runpy`, usa `admin_env()` con `PGOPTIONS=-c default_transaction_read_only=on` y `PGDATABASE=adel_pos`, y solo imprime
   conteos, nunca credenciales; el de la 9A, `stage9a_readonly_check.py`, sirve de base):
   - `SHOW transaction_read_only` = `on`;
   - migraciones registradas `1–17`;
   - tablas en `app` (esperado 127) y permisos registrados (esperado 147, según el registro de la instalación real de la
     9A);
   - conteos que la 0018 y la 0019 no deben cambiar: cuentas por estado, unidades comerciales, liberaciones canónicas,
     pagos, ajustes de cuenta, `payment_documents`, turnos por estado, operaciones de caja, `cash_documents`,
     `kitchen_tickets` por `kind`, `kitchen_print_jobs` por estado, impresoras de cocina, categorías, productos, auditoría
     (`audit_events`), `sales_recognitions` y la fila de `app.establishment` (solo si tiene dirección, teléfono y correo:
     sí/no, sin imprimir los valores);
   - que no existen `app.document_templates`, `app.document_template_versions`, `app.document_logos`,
     `app.document_logo_rasters` ni `app.document_identity`.
5. **Detener el servidor**: `./scripts/dev.sh server-stop` (ordenado) y `status` sin proceso.
6. **Migrar**: `./scripts/dev.sh migrate` → `applied:2`; repetirlo → `applied:0`.
7. **Iniciar el servidor**: `./scripts/dev.sh server-start` → readiness 200 (PostgreSQL disponible, esquema compatible)
   y `GET /api/v1/version` con 0.9.1 y esquema 19.
8. **Comprobación posterior de solo lectura** (el mismo script en modo posterior):
   - migraciones `1–19`; tablas en `app` 127 → 132 (las cinco nuevas);
   - todos los conteos del paso 4 iguales (la 0018 y la 0019 no modifican filas existentes; los tickets y trabajos
     anteriores quedan con `template_version_id` y `layout` en NULL: contar que **todos** los `kitchen_tickets` existentes
     tienen `template_version_id` NULL y todos los `kitchen_print_jobs`, `layout` NULL);
   - **plantillas del sistema**: 10 filas en `app.document_templates`, todas `system` y `active`, nombre «Predeterminada»,
     identificadores `9b0d0c00-0000-4000-8000-000000000001` … `…010` (uno por tipo) y `revision` 1; 10 filas en
     `app.document_template_versions`, número 1, identificadores `9b0d0c01-…-000000000001` … `…010`, `created_by` NULL y
     `current_version_id` de cada plantilla igual a su versión 1; ninguna plantilla propia;
   - **permisos**: 147 → 153 (+6; medido en bases nuevas del clúster temporal: +5 tablas y +6 permisos); los seis `documents.*` (`documents.templates.read`, `documents.templates.manage`,
     `documents.preview.real`, `documents.samples.print`, `documents.identity.manage`,
     `documents.history.render_current`) concedidos **solo** al rol `administrator` (contar concesiones por rol); los
     demás roles sin cambios;
   - **identidad vacía**: 1 fila en `app.document_identity` con `additional_info`, `header_text` y `footer_text` vacíos,
     `logo_id`, `logo_raster_58_id` y `logo_raster_80_id` NULL, presentación por omisión (60, `center`, `threshold`, 128)
     y `revision` 1; 0 filas en `app.document_logos` y en `app.document_logo_rasters`;
   - restricciones: `kitchen_tickets_kind_check` admite `document_sample` y `document_rerender`;
     `realtime_events_scope_check` admite `documents`;
   - **impresoras**: todas las existentes con `kitchen_printers.accepts_escpos` en `false` («Esta impresora entiende
     ESC/POS (formato y logotipo)» desactivada; la 0018 la agrega con ese valor) y la restricción
     `kitchen_printers_accepts_escpos_check`.
9. **Aplicación de escritorio**: `./scripts/dev.sh build-macos` → `AD-EL POS.app` 0.9.1 (anotar el tamaño) y
   `./scripts/dev.sh open-app`.
10. **Registro**: anotar en el avance la hora de inicio y fin, el commit, el respaldo, el resultado del ensayo de
    restauración, `applied:2`/`applied:0`, la readiness, la versión y las comprobaciones antes y después. El recorrido en la
    ventana nativa (Documentos, identidad, editor, muestra en una impresora real) queda como verificación del propietario:
    no darlo por hecho.

**Vuelta atrás** (si algo falla después de migrar): no hay migración de bajada. Detener el servidor, restaurar el
respaldo del paso 2 con el procedimiento de `DEVELOPMENT_MACOS.md` (sin borrar `.local` ni el Keychain), volver a
compilar e iniciar desde `main` (0.9.0) y repetir la comprobación del paso 4. Si el fallo ocurre **antes** de que
`migrate` responda, la migración es transaccional y no deja cambios parciales: basta iniciar el servidor 0.9.0 desde
`main`.

Después de aplicarlas, 0018 y 0019 son inmutables y la siguiente ampliación requiere 0020; actualizar `CLAUDE.md`,
`README.md`, `NEXT_STEPS_FOR_SOL.md`, `docs/VERIFICATION.md` y las guías que dicen «no aplicadas».

### Regresión final de la fase D

Ejecutada el 11 de noviembre de 2026 sobre copias exactas de la rama (`git archive`, sin `.git`, `.local` ni cachés), con
un clúster PostgreSQL temporal propio (puerto 55499, solo loopback) y Go temporal en 8183; nada contra la instalación
real. Primero sobre `31a0ea1` (código y documentación de la fase D) y después sobre `40c8d17`, que solo corrige la
especificación Playwright del diseñador (resolver la copia exacta antes de imprimir con la plantilla vigente, por el
bloqueo mutuo de copias de la revisión): Vitest y Playwright se repitieron sobre `40c8d17`; lo demás no cambia entre ambos
commits.

| Suite                    | Resultado                                                                                                                                                                                                                  |
| ------------------------ | -------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Formato                  | Prettier (sin `docs/etapas/`), gofmt y `cargo fmt --check` limpios                                                                                                                                                         |
| Metadatos                | `sync-metadata.mjs --check`: «Metadatos coherentes» (0.9.1, esquema 19)                                                                                                                                                    |
| `go vet`                 | `./...` y `-tags=integration ./...` sin hallazgos                                                                                                                                                                          |
| Go unitarias (`-race`)   | 22 paquetes `ok`, entre ellos `documents`, `designer`, `identity`, `logoraster`, `brand`, `kitchen`, `payments` y `reports`                                                                                                |
| Integración PostgreSQL   | `database` 5.7 s, `administration` 53.6 s, `catalog` 7.0 s, `operations` 232.3 s, `payments` 2.4 s, `reports` 90.8 s, `reports/operational` 40.1 s, `designer` 17.4 s, `identity` 18.0 s: todas `ok`                       |
| TypeScript estricto      | `tsc --noEmit` sin errores                                                                                                                                                                                                 |
| Vitest (`npm test`)      | 62 archivos, 536 pruebas aprobadas                                                                                                                                                                                         |
| Compilación del frontend | `npm run build` correcto                                                                                                                                                                                                   |
| Rust/Tauri               | `cargo test --locked --offline`: 33 aprobadas, 1 ignorada (Keychain real)                                                                                                                                                  |
| Playwright               | Chromium 12/12 (4.7 min) y WebKit 12/12 (4.7 min), incluidas `workflow-stage9b-designer.spec.ts` y `workflow-stage9b-identity.spec.ts` y las anteriores (operación, cocina, cobros, caja, inventarios, compras y reportes) |
| Paquete de escritorio    | `.app` 0.9.1 compilado en la copia aislada (`tauri build --bundles app`, 9.94 MiB, Mach-O arm64); no se reemplazó ni se abrió el de la instalación                                                                         |

La primera corrida de la regresión dio dos incidencias, ninguna de producto: Vitest se invocó sin el filtro `src` del
proyecto y recogió las especificaciones de Playwright (se repitió con `npm test`), y la especificación del diseñador no
resolvía la copia exacta antes de la impresión con la plantilla vigente, que la corrección del bloqueo mutuo rechaza a
propósito (la falla de la especificación de identidad fue en cascada: al fallar una prueba, Playwright reinicia el proceso
y pierde la terminal preparada). Corregida en `40c8d17`.

No verificado (pruebas físicas pendientes, §32): papel de tickets, precuentas, comandas, cortes y del logotipo; cajón por
red y por USB/CUPS; pantalla táctil física; dos terminales reales; ventana nativa (WKWebView) y diálogo nativo de
guardado. Ninguna se da por hecha con los adaptadores simulados.
