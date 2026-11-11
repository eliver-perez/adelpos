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
