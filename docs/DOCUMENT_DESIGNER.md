# Diseñador de documentos e identidad del establecimiento · Etapa 9B

Versión 0.9.1, API v1, esquema 19 (migraciones `0018_document_templates.sql` y `0019_document_identity.sql`). **Las
migraciones 0018 y 0019 no se han aplicado a la instalación real**: esperan la autorización del propietario y el
procedimiento controlado de respaldo, ensayo de restauración y migración (ver «Instalación» al final). Este documento
describe solo lo implementado y probado en la rama `etapa-9b`. Las decisiones del propietario están en
[etapas/ETAPA_9B.md](etapas/ETAPA_9B.md) §0-B; lo que se comprobó en cada fase, en
[ETAPA_9B_AVANCE.md](ETAPA_9B_AVANCE.md); los contratos HTTP completos, en [API_CONTRACTS.md](../API_CONTRACTS.md)
(secciones «Etapa 9B»); la evidencia, en [VERIFICATION_STAGE9B.md](VERIFICATION_STAGE9B.md).

La primera parte es para el personal del restaurante; la segunda, para quien desarrolle.

## Contenido

- Para el restaurante: [Qué se puede hacer](#qué-se-puede-hacer) · [Entrar](#entrar-a-documentos) ·
  [Identidad del establecimiento](#identidad-del-establecimiento) · [Plantillas](#plantillas) ·
  [El editor, paso a paso](#el-editor-paso-a-paso) · [Guardar y versiones](#guardar-y-versiones) ·
  [Activar y volver al original](#activar-y-volver-al-original) · [Imprimir una muestra](#imprimir-una-muestra) ·
  [Reimprimir o imprimir con la plantilla vigente](#reimprimir-o-imprimir-con-la-plantilla-vigente) ·
  [Lo que depende de la impresora](#lo-que-depende-de-la-impresora)
- Para quien desarrolle: [Arquitectura](#arquitectura) · [Modelo de plantilla](#modelo-de-plantilla) ·
  [Catálogo de bloques](#catálogo-de-bloques) · [Campos de datos](#campos-de-datos) · [Límites](#límites) ·
  [Representación intermedia](#representación-intermedia-ir) · [Impresión](#impresión) ·
  [Documentos históricos](#documentos-históricos-y-reimpresión) · [Muestras](#muestras) ·
  [Identidad y logotipo](#identidad-y-logotipo-técnico) · [Seguridad](#seguridad) · [Permisos](#permisos) ·
  [API, eventos y auditoría](#api-eventos-y-auditoría) · [Persistencia](#persistencia) ·
  [Reportes](#reportes-y-exportaciones) · [Decisiones](#decisiones) ·
  [Sistema anterior](#funciones-del-sistema-anterior-no-incluidas) · [Pruebas](#pruebas) ·
  [Limitaciones](#limitaciones-conocidas) · [Cómo ampliar](#cómo-agregar-un-bloque-o-un-dato) ·
  [Instalación](#instalación)

# Para el restaurante

## Qué se puede hacer

- Configurar los datos que el restaurante quiere en sus documentos y **subir su logotipo**. La aplicación sigue con la
  marca de AD-EL POS: el logotipo del restaurante solo aparece en los documentos impresos y en los reportes PDF y Excel.
- Diseñar con **bloques** (no con código) los diez documentos: ticket de venta, precuenta, comanda, aviso de modificación,
  aviso de cancelación, comprobantes de ingreso, retiro y gasto, corte parcial y corte definitivo.
- Ver cada cambio al instante en la vista previa a **58 y 80 mm**, con exactamente las líneas que saldrán.
- Imprimir **muestras** marcadas como tales, sin crear ventas, pagos ni movimientos.
- Activar una plantilla por documento, volver a la predeterminada y consultar el historial de versiones.
- Reimprimir un documento antiguo **idéntico al original**, aunque la plantilla haya cambiado después.

Instalar la 9B **no cambia ningún papel**: cada documento sigue con su plantilla «Predeterminada», que imprime exactamente
lo mismo que antes (sin logotipo), hasta que el restaurante active una plantilla propia.

## Entrar a Documentos

En el menú principal, **Documentos** (visible con el permiso «Consultar plantillas de documentos»; el administrador lo
tiene). Arriba hay dos pestañas: **Plantillas** e **Identidad del establecimiento**. Quien solo puede consultar ve
plantillas, historial, vista previa e identidad, pero no los botones para cambiarlos; el servidor vuelve a comprobar
cada permiso. «Documentos» todavía no se puede elegir como pantalla de inicio.

## Identidad del establecimiento

Pestaña **Identidad del establecimiento**. Para cambiarla se necesita «Modificar la identidad documental y el logotipo
del establecimiento»; sin él se ve en solo lectura.

- **Datos del establecimiento** (nombre comercial, dirección, teléfono y correo): se muestran, pero se editan en
  **Configuración general** (botón «Editar en Configuración general»). No se guardan dos veces.
- **Información adicional** (RFC, razón social, permiso…), **texto de encabezado** y **texto de pie**: hasta 300
  caracteres y 8 renglones cada uno, con contador. Solo salen en los documentos cuya plantilla incluye su bloque
  («Información adicional», «Texto de encabezado», «Texto de pie»). Los caracteres de control se quitan al guardar y las
  líneas se cortan al ancho del papel al imprimir.

### Logotipo, paso a paso

1. **Subir logotipo** y elegir un PNG o JPEG de hasta 1 MiB y hasta 2048 × 2048 píxeles. Funciona mejor un logotipo
   oscuro sobre fondo blanco o transparente, de unos 600 a 1200 píxeles de ancho; la transparencia se imprime en blanco.
   Se reconoce por su contenido, no por el nombre: un GIF, WebP o SVG renombrado se rechaza, igual que un archivo dañado.
2. Revisar el original y **cómo saldrá en papel de 58 y 80 mm**: esas imágenes son exactamente los puntos que se envían a
   la impresora, incluido el margen blanco de la alineación.
3. Ajustar la **presentación** con botones (nunca escribiendo números): ancho de 25 % a 100 % del ancho imprimible (pasos
   de 5 % o 40/60/80/100 %; 60 % por omisión), alineación (izquierda, centro o derecha) y método: **Umbral** para
   logotipos de colores planos (con «Más claro / Normal / Más oscuro» o pasos de 8; subir el umbral oscurece) o
   **Difuminado** para fotografías y degradados (simula grises; no usa el umbral). La altura es proporcional y nunca pasa
   de unos 4 cm de papel. La vista previa dice «Vista previa sin guardar» hasta tocar «Guardar cambios».
4. **Reemplazar logotipo** carga otro archivo; **Retirar logotipo** pide confirmación. Sin logotipo, el bloque
   «Logotipo» no imprime nada y los reportes usan el logotipo de AD-EL POS.
5. Los documentos ya emitidos **conservan su logotipo** al reimprimirse, aunque después se reemplace o se retire.

Si otra persona guardó la identidad mientras editabas, aparece «Otra persona guardó la identidad del establecimiento
mientras editabas»: tus cambios siguen en pantalla y eliges «Guardar mis cambios sobre la versión nueva» o «Descartar mis
cambios y ver la versión guardada». Si se pierde la conexión al guardar, «Reintentar» repite la misma operación sin
duplicarla.

## Plantillas

A la izquierda están los diez documentos agrupados: **Venta** (Ticket de venta y Precuenta), **Cocina** (Comanda, Aviso
de modificación y Aviso de cancelación) y **Caja** (Comprobante de ingreso, de retiro y de gasto, Corte parcial y Corte
definitivo). Al tocar uno aparecen sus plantillas:

- **Activa**: la que se usa al imprimir documentos nuevos de ese tipo (solo una por tipo).
- **Predeterminada · solo lectura**: la del sistema; imprime exactamente como antes de la 9B y no se puede modificar ni
  renombrar.
- Con «Administrar plantillas de documentos»: **Nueva plantilla** (parte de la predeterminada del tipo), **Duplicar**
  (copia la versión vigente de cualquier plantilla, también la predeterminada), **Renombrar**, **Editar**, **Activar**
  (con confirmación), **Historial** y **Restaurar predeterminada** (con confirmación).
- Una plantilla nueva o duplicada queda **inactiva**: no cambia ningún papel hasta activarla. Al crearla se abre el
  editor. El nombre es único por tipo de documento (sin distinguir mayúsculas).

## El editor, paso a paso

Tiene tres áreas. En pantallas angostas se cambia entre ellas con las pestañas «Elementos», «Vista previa» y «Estructura
y propiedades», sin perder lo que estabas haciendo.

1. **Elementos** (izquierda): las piezas que se pueden agregar **a ese documento**, agrupadas en Encabezado, Datos del
   documento, Contenido, Totales y pagos, y Texto y formato. Solo aparecen las compatibles: una comanda no ofrece pagos y
   un corte no ofrece la lista de productos.
   - **Toca** un elemento para agregarlo **debajo del bloque elegido**, o **arrástralo** a la estructura y suéltalo donde
     quieras.
   - «Fecha», «Hora», «Folio», «Mesa», «Cajero», «Cliente»… son campos de datos ya preparados.
   - «Ya está en la plantilla» indica un elemento que solo puede ir una vez (por ejemplo, la lista de productos o el
     aviso fiscal).
2. **Vista previa** (centro): una tira de papel con **exactamente** las líneas que se imprimirán y el ancho real (30
   caracteres en 58 mm y 42 en 80 mm). Arriba se elige **58 mm** u **80 mm**. La franja «Vista previa con datos de
   muestra» recuerda que los datos son de ejemplo: no son ventas ni movimientos.
   - **Tocar una línea** elige el bloque que la produce y resalta sus líneas.
   - «No imprimen nada con estos datos» lista los bloques vacíos con su motivo (por ejemplo, «Total en dólares» sin
     dólares activos).
   - Con «Previsualizar plantillas con un documento real», **Documento real** muestra un documento reciente con tu
     plantilla. No se imprime ni se guarda nada.
3. **Estructura y propiedades** (derecha):
   - **Estructura**: los bloques en el orden del papel. Para moverlos, arrastra el asa (las cuatro flechas) o toca
     **Subir** y **Bajar** en el bloque elegido; con teclado, flechas para recorrer y Alt + flecha para mover.
   - **Quitar** elimina el bloque elegido y ofrece «Deshacer». El **Aviso fiscal** es obligatorio en tickets, precuentas y
     documentos de caja: se puede mover y cambiar su alineación, tamaño y énfasis, pero no quitar ni ocultar.
   - **Propiedades** del bloque elegido: Visible, Alineación (izquierda, centro, derecha), Tamaño (normal, doble alto,
     doble ancho, doble), Negrita, Mayúsculas, espacio antes y después (0 a 3 líneas) y sus opciones propias (el texto de
     un «Texto fijo», el carácter del separador, las líneas en blanco, «Según Cocina / Sí / No» en la lista de cocina…).
     En la lista de cocina, **Importes** solo ofrece «Según Cocina» u «Ocultar»: para imprimirlos se activa «Incluir importes comerciales en comandas» en Cocina →
     Comandas y tiempos (las comandas guardadas sin precios no pueden imprimirlos después).
   - En comandas y avisos hay además **Tamaño de texto de todo el documento** («Según Cocina», Normal o Grande). Con
     texto grande, los tamaños de cada bloque no se aplican (la negrita sí).

Nunca hay que escribir HTML, CSS, JSON ni comandos de impresora.

## Guardar y versiones

1. Mientras haya cambios, arriba se ve «Cambios sin guardar». Si sales del editor o cambias de pantalla en el menú, se
   pregunta antes de descartarlos.
2. **Guardar versión**, con una nota opcional («agregamos el horario en el pie»). Cada guardado crea una **versión
   nueva**; las anteriores nunca se borran ni se modifican.
3. Si **otra persona guardó la misma plantilla** mientras editabas, no se sobrescribe nada: un aviso ofrece ver la versión
   que guardó, guardar tu borrador como la versión más reciente o descartarlo (y «Seguir editando sin guardar»). Tu
   borrador no se pierde hasta que elijas. Si otra persona guarda mientras editas, el editor también lo avisa sin tocar
   tu borrador.
4. Si algo no se puede guardar (por ejemplo, un texto fijo vacío), el bloque queda marcado con «Revisa este bloque» y se
   explica el motivo.
5. **Historial** muestra cada versión con su número, fecha, quién la guardó y su nota; tocar una muestra su vista previa a
   58 u 80 mm. **Restaurar esta versión** la guarda como una versión nueva (se confirma; nada se borra).

## Activar y volver al original

- **Activar** hace que los documentos **nuevos** de ese tipo se impriman con esa plantilla (se pide confirmación).
- **Restaurar predeterminada** vuelve a activar la plantilla del sistema; tus plantillas se conservan y se pueden volver a
  activar.
- Los documentos **ya emitidos** se reimprimen siempre idénticos al original, aunque después cambies o actives otra
  plantilla.

## Imprimir una muestra

1. En el editor, **Imprimir muestra** (permiso «Imprimir muestras de documentos»).
2. Elegir la impresora: se muestra su papel y si reproduce negrita, tamaños y logotipo.
3. Confirmar. La muestra sale con datos de ejemplo y con «\*\*\* MUESTRA · NO ES UN DOCUMENTO VÁLIDO \*\*\*» al inicio y
   al final. **No crea ventas, pagos, movimientos ni documentos de caja.**
4. El aviso dice «enviada a la cola de impresión» o «aceptada por la impresora o la cola»: eso **no garantiza que salió el
   papel**. Si el resultado es incierto o requiere intervención, se revisa en **Cocina y preparación → Cola de
   impresión**, como cualquier otro trabajo.

## Reimprimir o imprimir con la plantilla vigente

Junto a **Reimprimir** aparece **Imprimir con la plantilla vigente** en los documentos de una cuenta (Restaurante o Para
llevar → Cobrar → Documentos), en los documentos de Caja y en la cola de impresión de Cocina (solo comandas y avisos;
pide un motivo).

| Acción                                | Qué imprime                                                                                                      | Permisos                                                                                      |
| ------------------------------------- | ---------------------------------------------------------------------------------------------------------------- | --------------------------------------------------------------------------------------------- |
| **Reimprimir**                        | Una **copia idéntica** al original: mismos datos, misma versión de plantilla y mismo logotipo. Sin leyenda.      | El de reimprimir de su módulo (como antes de la 9B).                                          |
| **Imprimir con la plantilla vigente** | Los **mismos datos** (productos, importes y pagos no cambian) con la plantilla activa hoy y la identidad actual. | El de reimprimir del módulo **y** «Imprimir un documento histórico con la plantilla vigente». |

Ninguna de las dos crea ventas, pagos, cierres ni movimientos; ambas quedan en la auditoría y en la cola. «Imprimir con
la plantilla vigente» se confirma con un diálogo que explica la diferencia. Mientras una copia de cualquiera de las dos
siga sin resolver en la cola (pendiente o incierta), no se puede pedir otra del mismo documento. Los documentos de caja emitidos antes de la
9B solo guardaban su texto: admiten solo la reimpresión exacta («Este documento se emitió antes de las plantillas de
documentos…»).

## Lo que depende de la impresora

Las impresoras térmicas imprimen línea por línea: por eso el diseño es una lista de bloques y no un lienzo libre.

- **Negrita, tamaños y logotipo** solo salen en una impresora **ESC/POS por red** o en una impresora **CUPS** que cumpla
  dos condiciones: que en **Cocina y preparación → Impresoras** tenga activada la opción **«Esta impresora entiende
  ESC/POS (formato y logotipo)»** (desactivada por omisión, porque no toda impresora USB o de red entiende esos
  comandos) y que su cola acepte datos raw. En cualquier otro caso el documento se imprime igual, en texto y sin ese
  formato, y el resultado de la cola lo dice; nunca falla por eso. El editor lo recuerda junto a esos controles, lista las
  impresoras que no reproducirán el formato y muestra la compatibilidad al elegir impresora para una muestra.
- Si un trabajo se envía a una impresora de **otro ancho de papel** (respaldo, redirección o copia a otra impresora), el
  logotipo convertido para el ancho original no se envía.
- Un documento sin negrita, tamaños ni logotipo viaja exactamente como antes de la 9B.
- **Cómo sale en papel cada impresora es una prueba física pendiente** que hará el propietario: nada de lo anterior se
  comprobó con papel. Las impresoras de 80 mm que solo imprimen 512 puntos (en lugar de 576) recortarían el borde derecho
  del logotipo.

# Para quien desarrolle

## Arquitectura

```text
React (Documentos)  ──operaciones cerradas──▶  Rust (api.rs)  ──HTTP──▶  administration/routes.go
                                                                          ├─ documents/identity* → internal/identity
                                                                          └─ documents/*         → internal/designer
kitchen · payments ──convierten sus datos──▶ internal/documents (neutral) ──Render──▶ Layout (IR)
                                                                          └─▶ cola kitchen_print_jobs ─▶ transporte
```

| Paquete                       | Responsabilidad                                                                                                                                                                                                                                                                                                                                                                                                                                                                         |
| ----------------------------- | --------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `internal/documents`          | Neutral (no importa `kitchen`, `payments`, `operations` ni `administration`): diez tipos (`kinds.go`), modelo de plantilla (`model.go`), catálogo cerrado (`catalog.go`), campos de datos (`fields.go`), validación (`validate.go`), datos tipados por familia (`data.go`), renderizador (`render*.go`), IR (`ir.go`), predeterminadas (`defaults.go`), muestras (`samples.go`), ESC/POS (`escpos.go`), acceso a versiones e identidad (`store.go`) y saneamiento heredado (`text.go`). |
| `internal/designer`           | Rutas `documents/*` salvo la identidad: catálogo, plantillas, versiones, vista previa, recientes, muestras e impresoras.                                                                                                                                                                                                                                                                                                                                                                |
| `internal/identity`           | Rutas `documents/identity*`, lectura combinada con `app.establishment`, logotipo (carga, conversión, retiro), `Rasters` (fuente de bits y vista previa) y `ReportLogo` (fuente del logotipo de los reportes).                                                                                                                                                                                                                                                                           |
| `internal/logoraster`         | Puro: validación del archivo, conversión monocromática a 384/576 puntos, umbral o Floyd–Steinberg, PNG de vista previa y bytes `GS v 0`.                                                                                                                                                                                                                                                                                                                                                |
| `internal/brand`              | Copia embebida byte a byte de `assets/brand/adel-pos-tema-claro.png` (respaldo de los reportes).                                                                                                                                                                                                                                                                                                                                                                                        |
| `kitchen`                     | `template.go` (datos de la comanda, `RenderDocument`, renderizado de lo guardado), `transport_formatted.go` (IR con atributos), `rerender.go` (comandas con la plantilla vigente), `document_sources.go` (vista previa real y recientes). El trabajador carga los bits (`Worker.Rasters`).                                                                                                                                                                                              |
| `payments`                    | `document_data.go` (precuenta y ticket: datos y plantilla vigente), `cash_document_data.go` (caja), `document_sources.go` (vista previa real y recientes).                                                                                                                                                                                                                                                                                                                              |
| `desktop/src/pages/documents` | `DocumentsPage.tsx`, `templates/` (lista por tipo y acciones), `designer/` (editor), `identity/` + `IdentityPanel.tsx` (identidad) y `rerender.tsx` (confirmación común de «Imprimir con la plantilla vigente»). Servicios en `services/documents.ts` y `services/documentIdentity.ts`.                                                                                                                                                                                                 |

**Cómo se imprime un documento desde la 9B**:

1. El módulo de origen arma sus datos como siempre (sin cambios de reglas de negocio ni de enrutamiento: la plantilla
   nunca decide a qué estación o impresora va una comanda).
2. Toma la versión activa del tipo (`documents.ActiveVersion`; si no hay o su contenido ya no es válido para el catálogo,
   la versión 1 del sistema: **la impresión nunca falla por una plantilla**) y la identidad vigente
   (`documents.LoadIdentity`).
3. Guarda en el documento la versión usada (`kitchen_tickets.template_version_id` y la clave `template_version_id` del
   JSON), una copia de la identidad y, en caja, los datos estructurados (sin el esperado si no se incluyó).
4. `documents.Render(kind, template, data, RenderOptions{PaperWidth, IncludeExpected})` produce la IR. El trabajo guarda
   `content = Text()` y, solo si la IR no es plana, `layout`.
5. El trabajador carga los bits del logotipo y el transporte traduce la IR (ver «Impresión»). La cola, el lease, la
   serialización por impresora, los reintentos y los resultados inciertos **no cambian**.

La vista previa del editor llama al mismo `Render` con la plantilla en edición: **vista previa e impresión usan la misma
IR**. React solo dibuja las líneas que devuelve Go; nunca calcula anchos, cortes ni importes.

## Modelo de plantilla

Una versión guarda un JSON con las opciones del documento y una **lista ordenada de bloques** de la lista cerrada:

```json
{
  "options": { "text_size": "inherit" },
  "blocks": [
    {
      "id": "b1",
      "type": "field",
      "visible": true,
      "align": "left",
      "size": "normal",
      "bold": false,
      "uppercase": false,
      "spacing_before": 0,
      "spacing_after": 0,
      "options": {
        "items": [{ "key": "date", "label": "Fecha:" }],
        "layout": "inline",
        "omit_empty": true
      }
    }
  ]
}
```

- `id`: `^[a-z0-9-]{1,40}$`, único en la plantilla (lo genera el editor).
- Propiedades comunes: `visible`, `align` (`left|center|right`), `size` (`normal|double_height|double_width|double`),
  `bold`, `uppercase`, `spacing_before` y `spacing_after` (0–3 líneas en blanco). `logo` solo admite visible y espaciado;
  `spacer`, solo visible. Una propiedad ausente toma su valor por omisión (visible, izquierda, normal, sin negrita, sin
  mayúsculas, sin espaciado).
- `options` del bloque: esquema cerrado por tipo con **decodificación estricta** (una clave desconocida se rechaza);
  ausentes = las del tipo. Lo que se guarda es la forma canónica, con todas las propiedades y opciones explícitas.
- `options.text_size`: `inherit` (la opción «Tamaño de texto» de Cocina), `normal` o `large` en las familias de cocina;
  `normal` fijo en las demás.
- **Alineación** = relleno con espacios dentro del ancho efectivo (texto puro: funciona igual en CUPS y ESC/POS y la vista
  previa es exacta). Los renglones «concepto … importe» y los separadores ocupan siempre todo el ancho. **Doble ancho** y
  **doble** usan la mitad de columnas en ese bloque. **Mayúsculas** se aplica antes de cortar líneas. Con texto grande de
  cocina las columnas son la mitad y los tamaños por bloque se ignoran (la negrita se conserva).
- Cada familia conserva su regla de corte anterior, necesaria para la equivalencia exacta: comandas por carácter
  conservando saltos (`CleanText`), precuentas y tickets por carácter con los controles convertidos en espacios
  (`Printable`), documentos de caja entre palabras.

Las **plantillas del sistema** tienen identificadores fijos `9b0d0c00-0000-4000-8000-0000000000NN` y su versión 1
`9b0d0c01-0000-4000-8000-0000000000NN`, con NN = 01 `receipt`, 02 `precheck`, 03 `kitchen_order`, 04
`kitchen_modification`, 05 `kitchen_cancellation`, 06 `cash_income`, 07 `cash_withdrawal`, 08 `cash_expense`, 09
`cash_partial_cut` y 10 `cash_final_cut`. Su layout es `documents.Default(kind)` (la 0018 siembra exactamente ese JSON y
una prueba de integración lo compara). Ninguna lleva logotipo, información adicional, encabezado ni pie.

## Catálogo de bloques

33 tipos de bloque. «Máx.» es el número de apariciones permitidas en una plantilla (— = sin límite propio). `GET
documents/catalog` entrega esta misma lista, con nombres, opciones y valores permitidos, para el editor.

| Bloque (`type`)                                                       | Nombre en pantalla                                        | Tipos de documento                    | Máx. | Opciones (valor por omisión)                                                                                                                                                                                                                                                                                                       | Sin datos                                                                   |
| --------------------------------------------------------------------- | --------------------------------------------------------- | ------------------------------------- | ---- | ---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | --------------------------------------------------------------------------- |
| `logo`                                                                | Logotipo                                                  | Todos                                 | 3    | — (ancho, alineación y método están en la identidad)                                                                                                                                                                                                                                                                               | Sin logotipo: nada, con aviso                                               |
| `establishment_name`                                                  | Nombre del establecimiento                                | Todos                                 | —    | Solo en cocina: `when` `kitchen_setting` \| `always` (`kitchen_setting`)                                                                                                                                                                                                                                                           | En cocina, con «Mostrar nombre del establecimiento» de Cocina apagado       |
| `establishment_address`, `establishment_phone`, `establishment_email` | Dirección, Teléfono, Correo                               | Todos                                 | —    | —                                                                                                                                                                                                                                                                                                                                  | Dato vacío en Configuración general                                         |
| `establishment_extra`, `header_text`, `footer_text`                   | Información adicional, Texto de encabezado, Texto de pie  | Todos                                 | —    | —                                                                                                                                                                                                                                                                                                                                  | Texto vacío en la identidad                                                 |
| `fixed_text`                                                          | Texto fijo                                                | Todos                                 | —    | `text` (obligatorio, ≤ 300 caracteres y ≤ 8 líneas)                                                                                                                                                                                                                                                                                | —                                                                           |
| `document_title`                                                      | Título del documento                                      | Todos                                 | —    | `text` (vacío = el del tipo; ≤ 40). El del corte parcial lleva su número                                                                                                                                                                                                                                                           | —                                                                           |
| `divider`                                                             | Separador                                                 | Todos                                 | —    | `character` `-` \| `=` \| `*` \| `.` (`-`)                                                                                                                                                                                                                                                                                         | —                                                                           |
| `spacer`                                                              | Espacio                                                   | Todos                                 | —    | `lines` 1–5 (1)                                                                                                                                                                                                                                                                                                                    | —                                                                           |
| `field`                                                               | Campo de datos                                            | Todos                                 | —    | `items` 1–4 `{key, label}` (etiqueta ≤ 40); `layout` `inline` (unidos con « · ») \| `pair` (etiqueta a la izquierda, valor a la derecha; un solo dato) (`inline`); `omit_empty` (sí; las predeterminadas usan no)                                                                                                                  | Con `omit_empty`, todos los valores vacíos                                  |
| `products`                                                            | Lista de productos                                        | Ticket, precuenta                     | 1    | `show_notes` (sí)                                                                                                                                                                                                                                                                                                                  | Sin partidas                                                                |
| `adjustments`                                                         | Descuentos y cargos                                       | Ticket, precuenta                     | 1    | `show_reason` (sí): «Descuento · Atención»                                                                                                                                                                                                                                                                                         | Sin ajustes                                                                 |
| `taxes`                                                               | Impuestos                                                 | Ticket, precuenta                     | 1    | `show_base` (sí): subtotal sin IVA además del IVA por tasa                                                                                                                                                                                                                                                                         | Sin desglose                                                                |
| `total`                                                               | Total                                                     | Ticket, precuenta                     | 1    | —                                                                                                                                                                                                                                                                                                                                  | —                                                                           |
| `usd_total`                                                           | Total en dólares                                          | Ticket, precuenta                     | 1    | —                                                                                                                                                                                                                                                                                                                                  | Sin `usd_cash` o sin tipo de cambio                                         |
| `this_payment`                                                        | Este pago                                                 | Ticket                                | 1    | `divider`, `heading` (sí): método, efectivo, dólares, cambio, referencia y propina                                                                                                                                                                                                                                                 | Sin pago propio                                                             |
| `payments`                                                            | Pagos registrados                                         | Ticket, precuenta                     | 1    | `divider`, `heading` (sí)                                                                                                                                                                                                                                                                                                          | Sin pagos                                                                   |
| `balance`                                                             | Saldo pendiente                                           | Ticket, precuenta                     | 1    | —                                                                                                                                                                                                                                                                                                                                  | Saldo cero                                                                  |
| `kitchen_location`                                                    | Ubicación                                                 | Comanda y avisos                      | —    | `show_customer` (sí): para llevar con origen y cliente, o área y mesa                                                                                                                                                                                                                                                              | —                                                                           |
| `kitchen_items`                                                       | Lista de cocina                                           | Comanda y avisos                      | 1    | `show_notes` `inherit` \| `yes` \| `no`; `show_prices` `inherit` \| `no` (para imprimirlos, «Incluir importes comerciales en comandas» de Cocina); `quantity_style` `inherit` \| `prefix` («2 x») \| `multiplier` («× 2»); `line_spacing` `inherit` \| 0–3; `station_divider`, `show_station_headings`, `show_item_reference` (sí) | Sin partidas                                                                |
| `voucher_detail`                                                      | Detalle del comprobante                                   | Ingreso, retiro, gasto                | 1    | `show_reference`, `show_notes` (sí)                                                                                                                                                                                                                                                                                                | —                                                                           |
| `signatures`                                                          | Firmas                                                    | Ingreso, retiro, gasto                | 1    | `show_status` («corregido»), `signature_line` (sí)                                                                                                                                                                                                                                                                                 | —                                                                           |
| `opening_float`                                                       | Fondo inicial                                             | Cortes                                | 1    | —                                                                                                                                                                                                                                                                                                                                  | —                                                                           |
| `sales_by_method`                                                     | Ventas y pagos por método                                 | Cortes                                | 1    | `heading` (sí)                                                                                                                                                                                                                                                                                                                     | —                                                                           |
| `shift_totals`                                                        | Totales del turno                                         | Cortes                                | 1    | —                                                                                                                                                                                                                                                                                                                                  | —                                                                           |
| `cash_movements`                                                      | Ingresos, retiros y gastos                                | Cortes                                | 1    | `heading` (sí)                                                                                                                                                                                                                                                                                                                     | —                                                                           |
| `declared_cash`                                                       | Efectivo declarado                                        | Corte definitivo                      | 1    | `divider`, `heading` (sí)                                                                                                                                                                                                                                                                                                          | Sin declarado                                                               |
| `expected_differences`                                                | Esperado y diferencias (**protegido**)                    | Cortes                                | 1    | —                                                                                                                                                                                                                                                                                                                                  | Sin permiso, en arqueo o sin datos: «Esperado y diferencias: no incluidos.» |
| `observations`                                                        | Observaciones                                             | Corte definitivo                      | 1    | `divider`, `heading` (sí)                                                                                                                                                                                                                                                                                                          | Sin observaciones                                                           |
| `fiscal_notice`                                                       | Aviso fiscal (**obligatorio**: exactamente uno y visible) | Ticket, precuenta y los cinco de caja | 1    | — («Documento comercial. No es un comprobante fiscal (CFDI).» o «Documento interno de caja…»)                                                                                                                                                                                                                                      | —                                                                           |

Las opciones `inherit` de la lista de cocina toman la configuración de Cocina **copiada en cada comanda**
(`Document.Settings`): la reimpresión de una comanda no cambia si después cambia la configuración. Las composiciones
internas del formato anterior (por ejemplo, el separador antes de «ESTE PAGO» solo cuando hay pago) viven dentro del
bloque.

## Campos de datos

Claves que acepta `field` por tipo (las demás se rechazan al guardar). La etiqueta se imprime seguida de un espacio y el
valor; varios datos se unen con « · ». El editor ofrece cada clave como atajo con su etiqueta propuesta.

| Tipos              | Claves (etiqueta propuesta)                                                                                                                                                                                                                                                |
| ------------------ | -------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Todos              | `date` («Fecha:»), `time` («Hora:»), `datetime` (fecha y hora; en cocina con segundos)                                                                                                                                                                                     |
| Comanda y avisos   | `folio` («Cuenta», imprime «#21»), `account_name`, `table` («Mesa:»), `area` («Área:»), `customer` («Cliente:»), `origin` («Origen:»), `employee` («Capturó:»), `order` («Pedido:»), `document` («Documento:»)                                                             |
| Ticket y precuenta | `folio` («Folio:»), `account_name` («Cuenta:»), `context` (mesa o modalidad: «Mesa 4», «Para llevar»), `cashier` («Atendió:»)                                                                                                                                              |
| Comprobantes       | `register` («Caja:»), `business_date` («Turno del»), `recorded_by` («Registró:»), `device` («Terminal:»)                                                                                                                                                                   |
| Cortes             | `register`, `business_date`, `opened_by` («Responsable:»), `opened_at` («Apertura»), `cut_at` («Corte»), `created_by` («Realizó:»), `device`; además `previous_cut_at` («Corte anterior», solo el parcial y solo si hubo uno) y `closed_by` («Cerró:», solo el definitivo) |

## Límites

| Límite                                    | Valor                                                              |
| ----------------------------------------- | ------------------------------------------------------------------ |
| Bloques por plantilla                     | 1–60                                                               |
| Tamaño de la plantilla                    | 32 KiB (la columna admite hasta 64 KiB)                            |
| Texto fijo                                | 300 caracteres y 8 líneas                                          |
| Etiquetas de campo y título personalizado | 40 caracteres                                                      |
| Datos por campo                           | 1–4 (`pair`: exactamente 1)                                        |
| Espaciado antes y después                 | 0–3 líneas                                                         |
| Espacio (`spacer`)                        | 1–5 líneas                                                         |
| Logotipos por plantilla                   | 3                                                                  |
| Nombre de plantilla                       | 1–80 caracteres, único por tipo sin distinguir mayúsculas          |
| Nota de versión                           | 200 caracteres                                                     |
| Historial                                 | las 200 versiones más recientes                                    |
| Documentos recientes (vista previa real)  | 20                                                                 |
| Cuerpo de `documents/*`                   | 64 KiB (la carga del logotipo, 2 MiB)                              |
| Logotipo                                  | PNG o JPEG, ≤ 1 MiB, 1 × 1 a 2048 × 2048 px                        |
| Conversión térmica                        | 384 puntos (58 mm) o 576 (80 mm) de ancho, ≤ 320 de alto (≈ 40 mm) |
| Textos de la identidad                    | 300 caracteres y 8 renglones cada uno                              |
| Columnas                                  | 30 (58 mm) y 42 (80 mm); 15 y 21 con texto grande de cocina        |

## Representación intermedia (IR)

`documents.Layout` (`ir.go`) es lo único que sale del renderizador:

- `Layout{PaperWidth, Columns, TextSize, TrailingNewline, Lines, Warnings, EmptyBlocks}`;
- `Line{Text, Bold, Size, Image, BlockID}`: texto ya cortado, alineado y saneado; `Image{RasterID, WidthDots,
HeightDots}` en las líneas de logotipo (sin texto);
- `Text()`: la proyección de texto (las imágenes no aportan texto). Con las plantillas predeterminadas es **idéntica** al
  texto anterior a la 9B, incluido el salto final de las comandas y su ausencia en los demás;
- `Plain()`: verdadero si ninguna línea lleva negrita, tamaño distinto de normal ni imagen;
- `ForQueue()`: la IR sin los datos que solo usa el editor (`BlockID`, bloques vacíos).

`kitchen_print_jobs.content` sigue guardando `Text()`; la columna nueva `layout` guarda la IR solo si no es plana (NULL
en todos los trabajos de las plantillas predeterminadas: esos trabajos son iguales a los anteriores).

## Impresión

| Transporte                                 | IR plana (o `layout` NULL)                     | IR con negrita, tamaños o logotipo                                                                                                                                                                                                                    |
| ------------------------------------------ | ---------------------------------------------- | ----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| ESC/POS por red                            | El camino anterior (mismos bytes)              | `documents.EncodeESCPos`: `ESC @`, `ESC t 2` (CP850), por línea `GS ! n` y `ESC E n` con reinicio al final, texto y salto; imágenes con `GS v 0` en franjas de hasta 128 filas; al final los tres saltos y `GS V 0` si la impresora corta, como antes |
| CUPS con la opción ESC/POS y cola con raw  | El camino anterior (mismos argumentos de `lp`) | Los mismos bytes ESC/POS con `/usr/bin/lp -d <cola> -o raw -n <copias> -t ADEL-<trabajo> <archivo>` (argumentos separados, sin shell)                                                                                                                 |
| CUPS sin la opción ESC/POS, o cola sin raw | El camino anterior                             | `Text()` por el camino anterior (sin negrita, tamaños ni logotipo), con un aviso en el mensaje del resultado (p. ej. «La cola no admite datos raw: el documento se envió sin formato ni logotipo.»). No falla                                         |
| Agente de impresión                        | Intervención, como antes                       | Intervención («El destino requiere un agente de impresión futuro.»)                                                                                                                                                                                   |

- **Opción por impresora** (revisión independiente, fase D): «Esta impresora entiende ESC/POS (formato y logotipo)» (`accepts_escpos`),
  desactivada por omisión. Una cola CUPS que admite raw no implica una impresora ESC/POS (una impresora de recibos que
  solo acepta raster con su controlador, o una de oficina en el puerto 9100, imprimiría basura o papel en blanco): por eso
  CUPS solo recibe ESC/POS con la opción activa **y** la cola con raw. ESC/POS por red la tiene implícita.
- **Detección de raw**: la misma de solo lectura de la 7.1 (`kitchen/cups_raw.go`: `lpstat -p` y `lpoptions -p`),
  consultada otra vez antes de cada envío con formato. Si CUPS no responde a la consulta, el trabajo queda en `error`
  confirmado («No se pudo consultar la cola CUPS del servidor; no se envió el documento.») y sigue las reglas de
  reintento de los errores confirmados; no se envía nada a ciegas.
- **Logotipo**: los bits ya convertidos (filas de `(ancho+7)/8` bytes, MSB a la izquierda, 1 = negro, rellenados al ancho
  imprimible según la alineación) se cargan por `RasterID` al enviar (`documents.RasterSource`, implementada por
  `identity.Rasters`). Si no se encuentran, la imagen se omite con la nota «No se encontró la imagen del logotipo: se
  omitió.», sin fallar. Un trabajo guardado que se envía a una impresora de otro ancho (respaldo automático,
  redirección o copia a otra impresora) no envía el logotipo del ancho original. Tamaño típico: 3–14 KB por documento (máximo 23 KB en 80 mm).
- **Compatibilidad** (`GET documents/printers`): ESC/POS por red, `supported`; CUPS, `supported` solo con la opción
  ESC/POS y la cola con raw, `unsupported` sin la opción o sin raw (imprime el texto) y `unknown` si CUPS no respondió;
  agente, `unsupported`. Las consultas a CUPS de la petición tienen un límite total de tiempo, como las del cajón.
- **Resultados**: `submitted` sigue significando aceptación de CUPS o TCP, nunca papel; `uncertain` exige resolución
  humana y nunca se reenvía solo; la serialización por impresora, el lease y el respaldo tras tres errores confirmados no
  cambian.
- **Prueba técnica de impresora** (`kitchen/printers/test`, «PRUEBA DE IMPRESIÓN»): no es uno de los diez tipos y sigue
  igual, con la versión 1 de la comanda.

## Documentos históricos y reimpresión

| Documento          | Qué guarda desde la 9B                                                                                                 | Reimprimir (exacta)                                                                              | Imprimir con la plantilla vigente                                                                              |
| ------------------ | ---------------------------------------------------------------------------------------------------------------------- | ------------------------------------------------------------------------------------------------ | -------------------------------------------------------------------------------------------------------------- |
| Comandas y avisos  | `Document` (contexto, configuración de Cocina y secciones, como antes) + `template_version_id` + copia de la identidad | Se vuelve a renderizar el `Document` con **su** versión y su identidad (sin versión = versión 1) | `POST kitchen/print-jobs/rerender`                                                                             |
| Precuenta y ticket | `CommercialDocument` (como antes) + `template_version_id` + identidad                                                  | Copia el contenido y el layout del trabajo original (como antes)                                 | `POST payments/accounts/{id}/documents/rerender`                                                               |
| Documentos de caja | Texto, título y referencias (como antes) + **datos estructurados** (`document.data`, sin el esperado si no se incluyó) | Copia el contenido y el layout del trabajo original (como antes)                                 | `POST payments/cash-documents/{id}/rerender`; los anteriores a la 9B responden `409 DOCUMENT_DATA_UNAVAILABLE` |

- Los documentos anteriores a la 9B tienen `template_version_id` NULL y equivalen a la versión 1 del sistema, que es
  idéntica al formato anterior: su reimpresión no cambia.
- **Sin leyenda «REIMPRESIÓN»** (decisión del propietario): la reimpresión queda en la cola (`print.reprinted`) y en la
  auditoría de su módulo, como antes.
- «Imprimir con la plantilla vigente» crea un ticket `document_rerender` con el original, la versión usada y el texto, en
  la impresora del trabajo original (la cola de cocina permite otra con `kitchen.printers.manage`). **Nunca** crea un
  `payment_documents` (no cambia «Esperando pago») ni un `cash_documents`. Se rechaza mientras el original siga pendiente
  o incierto (`400`). Desde las correcciones de la revisión independiente, la reimpresión exacta y la impresión con la
  plantilla vigente se bloquean **entre sí**: mientras haya una copia sin resolver de cualquiera de las dos (o una copia
  de una impresión con la plantilla vigente) del mismo documento, otra se rechaza con `409 PRINT_COPY_PENDING`. Un documento de caja con el esperado exige además `cash.expected.read` y que el turno no esté en
  arqueo (`409 SHIFT_CLOSING`).
- Las versiones son inmutables (disparador contra UPDATE, DELETE y TRUNCATE) y los logotipos y sus conversiones también:
  un documento emitido antes de reemplazar o retirar el logotipo se reimprime con los mismos bits.

## Muestras

`POST documents/samples` renderiza la plantilla en edición con **datos de muestra** claramente ficticios («de muestra» en
cada nombre) y la identidad real, al ancho de la impresora elegida, con `*** MUESTRA · NO ES UN DOCUMENTO VÁLIDO ***`
centrado al inicio y al final (cortado por palabras en 58 mm). Crea un ticket `document_sample` y un trabajo en la cola de
siempre (historial `print.sample_requested`), y audita `documents.sample_printed`. **Nunca** inserta ventas, pagos,
movimientos ni documentos comerciales o de caja (lo comprueban conteos en la prueba de integración). La respuesta es el
estado del trabajo; su resultado (incierto incluido) se sigue en la cola. En una muestra de corte, el bloque protegido se
renderiza con los datos ficticios solo si quien imprime tiene `cash.expected.read`.

Los tickets `document_sample` y `document_rerender` no aparecen en la lista de comandas (`GET kitchen/tickets`); `GET
kitchen/tickets/{id}` de esos tipos aplica las reglas de su origen.

## Identidad y logotipo (técnico)

- **Sin duplicar datos**: nombre, dirección, teléfono y correo siguen en `app.establishment` (Configuración general,
  `PUT settings`). `app.document_identity` (0019, fila única) agrega información adicional, encabezado, pie, logotipo
  vigente, su presentación y las conversiones vigentes, con `revision`.
- **Validación del archivo** (`logoraster.Validate`): base64 estándar estricto (≤ 1 398 104 caracteres antes de
  decodificar); firma PNG o JPEG **y** `image.DecodeConfig` del mismo formato; dimensiones comprobadas **antes** de
  decodificar (sin bombas de descompresión: un PNG de 60 000 × 60 000 se rechaza reservando menos de 1 MiB); después,
  decodificación completa (integridad). Un JPEG con demasiados barridos (progresivo patológico, que tardaría decenas de
  segundos en decodificarse) se rechaza antes de decodificar, y la validación ocurre **antes** de tomar el lock de la
  fila de la identidad (correcciones de la revisión independiente). Nunca SVG ni formatos activos. Mensajes en español («El logotipo debe ser una
  imagen PNG o JPEG; no se aceptan SVG, GIF, WebP ni otros formatos.», «El logotipo está dañado o incompleto.», …).
- **Conversión** (`logoraster.Convert`): composición sobre blanco, escalado Catmull-Rom de `golang.org/x/image/draw`,
  luminancia BT.601 entera, umbral 1–255 (negro si la luminancia es menor) o Floyd–Steinberg entero con punto de decisión
  fijo en 128, relleno blanco hasta el ancho imprimible según la alineación; determinista en la misma arquitectura.
- **Guardado**: el original se guarda una sola vez por contenido (`document_logos`, SHA-256 único y comprobado contra los
  bytes) y sus conversiones a 58 y 80 mm (`document_logo_rasters`, una por logotipo, ancho, presentación y
  `converter_version`) al cargarlo o al cambiar la presentación; una combinación que ya existe se reutiliza. Nunca se
  convierte al imprimir. Retirar solo desvincula el logotipo; originales y conversiones nunca se borran.
- **Vista previa**: `GET documents/identity` devuelve el original y las conversiones como `data_uri` (un original de 1 MiB
  pesa ≈ 1,4 MB, dentro de los 2 MiB del puente); `POST documents/identity/logo/preview` muestra otra presentación sin
  guardarla (usa la conversión guardada si existe; si no, convierte en memoria).
- **Copia en cada documento**: `documents.LoadIdentity` arma la identidad (con las dos conversiones vigentes) y cada
  documento emitido la copia. El nombre de un documento es el copiado en él (contexto de la comanda o `establishment` del
  ticket), para conservar la equivalencia.

## Seguridad

- **Modelo cerrado**: una plantilla solo contiene bloques de la lista cerrada con opciones de esquema cerrado. No hay HTML,
  CSS, JavaScript, SQL, rutas de archivos, URL, comandos del sistema ni bytes ESC/POS proporcionados por el usuario: solo
  `documents/escpos.go` y los transportes generan comandos de control. Ninguna ruta acepta SQL ni un renderizador
  alternativo.
- **Validación en Go** (`documents.ParseTemplate`) al guardar, al previsualizar y al imprimir una muestra: tipo de
  documento, estructura, tamaño, número de bloques, identificadores únicos, tipos conocidos y **compatibles con el tipo de
  documento**, propiedades y opciones válidas, cardinalidades (aviso fiscal exactamente uno y visible; máximos por
  bloque) y límites de texto. Un error responde `422 TEMPLATE_INVALID` con el mensaje en español y el `block_id`.
- **Saneamiento**: los textos de la plantilla se limpian al guardar (`\r\n` → `\n`, tabuladores a espacio, controles e
  invisibles eliminados; en textos de una línea, también el salto). Los textos capturados por usuarios (observaciones,
  nombres, conceptos) se vuelven a limpiar al renderizar con las reglas de cada familia, de modo que una observación nunca
  altera el flujo de control de la impresora. Los textos de la identidad se limpian al guardar y la 0019 los restringe con
  CHECK.
- **Bloques protegidos**: «Esperado y diferencias» solo es compatible con el corte parcial y el definitivo; Go rechaza
  agregarlo a otro tipo («… solo puede incluirse en el corte parcial y en el corte definitivo.»). Se renderiza solo si
  quien imprime tiene `cash.expected.read` y el turno no está en arqueo (en la vista previa con un documento real, además,
  solo si el original lo conservaba); si no, imprime «Esperado y diferencias: no incluidos.» como antes. Durante un
  arqueo ciego no se renderiza para nadie, administrador incluido. Los costos no forman parte de ningún bloque.
- **Permisos de origen**: la vista previa con un documento real y la lista de recientes (`GET documents/recent`) exigen
  además el permiso de lectura de su origen (`kitchen.tickets.read` con su ámbito de estaciones; `payments.read` en
  precuentas y tickets; `cash.shifts.read` con las reglas de alcance de Caja y, en comprobantes de ingreso, retiro y
  gasto, también `cash.movements.read`); «Imprimir con la plantilla vigente» exige el permiso de reimprimir de su módulo.
  Ninguna etiqueta de «documentos recientes» lleva importes.
- **Vista previa con un documento de caja real**: un documento emitido desde la 9B se previsualiza con sus datos
  guardados (los mismos que usa «Imprimir con la plantilla vigente», con el esperado solo si el original lo incluía); uno
  anterior a la 9B se previsualiza con el corte o la operación actuales, con un aviso, porque no guardó sus datos.
- **Recursos**: logotipo validado antes de decodificar, cuerpos acotados (64 KiB y 2 MiB), plantillas acotadas y altura
  del raster acotada; la vista previa no escribe nada.
- **Marca**: el logotipo del restaurante nunca sustituye a `BrandLogo` ni a `assets/brand/` en la aplicación.

## Permisos

Seis permisos nuevos, registrados en Go (`documents.Permissions`) y en la 0018, **concedidos solo al administrador** (que
sigue contando como administrador completo); ningún otro rol los recibe por inferencia.

| Permiso                            | Permite                                                                                                                                                    |
| ---------------------------------- | ---------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `documents.templates.read`         | Ver Documentos, plantillas, versiones, catálogo, impresoras y vista previa con datos de muestra; recibir el ámbito SSE `documents`                         |
| `documents.templates.manage`       | Crear, editar, duplicar, renombrar, activar y restaurar plantillas (además de `documents.templates.read`)                                                  |
| `documents.preview.real`           | Vista previa con un documento real y lista de recientes (además del permiso de lectura de su origen)                                                       |
| `documents.samples.print`          | Imprimir muestras                                                                                                                                          |
| `documents.identity.manage`        | Cambiar la identidad documental y el logotipo, y previsualizar otra presentación                                                                           |
| `documents.history.render_current` | Imprimir un histórico con la plantilla vigente (además de `kitchen.reprint` + `kitchen.stations.all`, `payments.tickets.reprint` o `cash.reports.reprint`) |

Leer la identidad (`GET documents/identity`) admite `documents.templates.read` **o** `settings.read`. Todos se comprueban
en Go antes de la primera escritura; un 403 revierte con el savepoint y audita `access.denied`.

## API, eventos y auditoría

Contratos completos en [API_CONTRACTS.md](../API_CONTRACTS.md), «Etapa 9B». Resumen (todas bajo `/api/v1`, operación del
puente entre corchetes):

- Diseñador: `GET documents/catalog` [`documents_catalog`], `GET documents/templates?kind=` [`documents_templates`],
  `GET|PUT documents/templates/{id}` [`documents_template`, `documents_save_template`], `POST documents/templates`
  [`documents_create_template`], `POST documents/templates/{id}/rename` y `/activate`, `GET
documents/templates/{id}/versions`, `GET documents/versions/{id}`, `POST documents/restore-default`, `POST
documents/preview`, `GET documents/recent?kind=`, `POST documents/samples` y `GET documents/printers`.
- Identidad: `GET|PUT documents/identity`, `POST documents/identity/logo`, `POST documents/identity/logo/remove` y `POST
documents/identity/logo/preview`.
- Plantilla vigente: `POST kitchen/print-jobs/rerender`, `POST payments/accounts/{id}/documents/rerender` y `POST
payments/cash-documents/{id}/rerender`.
- Escrituras con `operation_id` (idempotencia en `restaurant_operations`; otro cuerpo con el mismo identificador → `409
IDEMPOTENCY_CONFLICT`) y `revision` (`409 REVISION_CONFLICT` sin escribir). Las plantillas del sistema responden `409
TEMPLATE_READ_ONLY`.
- 22 operaciones nuevas en la lista cerrada de `desktop/src-tauri/src/api.rs` (17 del diseñador y de la plantilla vigente
  y 5 de la identidad), con un identificador por ruta y su prueba Rust; el filtro `kind` está en la lista blanca.
- **SSE**: ámbito `documents` (lo reciben quienes tienen `documents.templates.read`) con `documents.templates.changed`
  (`aggregate_id` = plantilla) y `documents.identity.changed` (sin contenido), en la misma transacción de cada cambio. La
  interfaz solo invalida y vuelve a leer: el guardado sigue comprobando la revisión en Go.
- **Auditoría** (sin layouts, bytes ni base64): `documents.template_created`, `documents.template_saved` (versión
  anterior y nueva, bloques, bytes, nota), `documents.template_renamed`, `documents.template_activated`,
  `documents.default_restored`, `documents.sample_printed`, `documents.identity_updated`, `documents.logo_uploaded`
  (tipo, medidas, tamaño, SHA-256 y conversiones), `documents.logo_removed`, `print.rerendered`,
  `payments.document_rerendered` y `cash.document_rerendered`.
- **Locks**: sin locks consultivos nuevos. La vista previa y los recientes toman 7820148305 compartido (leen documentos de
  cocina y cobros); la muestra lo toma exclusivo (encola); las plantillas y la identidad bloquean sus filas
  (`FOR UPDATE`).

## Persistencia

| Migración | Cambios                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                     |
| --------- | --------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| 0018      | `app.document_templates` (tipo de la lista cerrada, nombre 1–80 único por tipo sin distinguir mayúsculas, una activa y una del sistema por tipo con índices únicos parciales, versión vigente de la misma plantilla por llave compuesta diferida, `revision`; disparador: no se borran, no cambian de tipo y las del sistema no cambian de nombre ni de versión); `app.document_template_versions` (número único por plantilla, layout JSON ≤ 64 KiB, nota ≤ 200, inmutables); semilla de las diez plantillas del sistema activas; `kitchen_tickets.template_version_id`; tipos de ticket `document_sample` y `document_rerender`; `kitchen_print_jobs.layout`; ámbito SSE `documents`; seis permisos solo al administrador |
| 0019      | `app.document_logos` (original inmutable con tipo, medidas, tamaño y SHA-256), `app.document_logo_rasters` (conversiones inmutables), `app.document_identity` (fila única con textos ≤ 300 y ≤ 8 renglones sin controles, logotipo vigente, presentación, conversiones vigentes «las tres o ninguna» con llaves compuestas al logotipo vigente, `revision`; no se borra); inserta la fila de la identidad vacía y sin logotipo. Sin permisos nuevos                                                                                                                                                                                                                                                                         |

Ambas son transaccionales, con restricciones de nombre explícito y CHECK seguros ante NULL, no modifican 0001–0017 ni
filas existentes y no siembran datos ficticios. Los paquetes con integración (`./internal/designer` e
`./internal/identity`) están en la tupla de `scripts/database.py`.

## Reportes y exportaciones

La 9B no cambia la arquitectura de reportes, las métricas, las fechas comerciales, los filtros ni los reportes de ventas,
pagos, descuentos, propinas, caja, inventarios, compras, costos y empleados, ni sus permisos: todo eso está en
[REPORTS.md](REPORTS.md) (etapa 9A), que sigue vigente. **Lo único que cambió es el encabezado de las exportaciones**:

- **PDF**: la primera página empieza con el logotipo del restaurante (a lo más 50 × 14 mm) y, a su derecha, el nombre, la
  dirección y «Tel. … · correo» del establecimiento (solo los datos que existan), con una línea divisoria; las páginas
  siguientes conservan su encabezado breve.
- **Excel**, hoja «Filtros»: el logotipo en A1 (anclado, sin hipervínculo, a lo más 200 × 56 px) y el nombre en B1; la
  tabla «Concepto | Valor» empieza en el renglón 3 con «Establecimiento» y, si existen, «Dirección», «Teléfono» y
  «Correo electrónico». La hoja «Datos» es idéntica byte a byte a la de la 9A.
- **Sin logotipo del restaurante** (o con uno guardado que ya no pasa la validación) se usa el de AD-EL POS
  (`internal/brand`, copia idéntica de `assets/brand/adel-pos-tema-claro.png`) con el nombre del establecimiento.
- Se usa el original en color reducido a ≤ 600 × 168 px y codificado otra vez sin metadatos, leído con
  `identity.ReportLogo` dentro de la misma transacción de solo lectura del reporte (sin locks). Contenido, columnas,
  totales, orden, consultas, límites y auditoría no cambian; la primera página del PDF crece ≈ 14.5 mm (un reporte que
  llenaba sus páginas puede ocupar una más) y cada archivo pesa ≈ 28–29 KB más con la marca de AD-EL POS.

## Decisiones

- Las plantillas predeterminadas imprimen los renglones fijos aunque sus datos estén vacíos (`omit_empty: false`, como
  «Pedido: » en la prueba técnica); los campos que agrega el editor los omiten por omisión.
- Una plantilla guardada que dejara de ser válida para el catálogo, o la falta de una activa, imprime con la versión 1 del
  sistema.
- Activar sube solo la revisión de la plantilla activada; la que se desactiva conserva la suya.
- La vista previa con un documento de caja usa los datos guardados desde la 9B; con un documento anterior lee el corte
  (snapshot inmutable) o la operación, con aviso. La impresión con la plantilla vigente solo existe con los datos
  guardados desde la 9B.
- **Correcciones de la revisión independiente** (fase D; sin hallazgos críticos ni altos ni fugas de datos protegidos):
  opción por impresora «Esta impresora entiende ESC/POS (formato y logotipo)»; «Importes» de la lista de cocina limitado a
  «Según Cocina» u «Ocultar», porque las comandas se guardan sin precios cuando Cocina los oculta y la vista previa debe
  coincidir con el papel; bloqueo mutuo de la reimpresión exacta y la impresión con la plantilla vigente; logotipo omitido
  al cambiar de ancho; límite total de tiempo para CUPS en `GET documents/printers`; vista previa de caja con los datos
  guardados; permiso de origen en `GET documents/recent`; JPEG con demasiados barridos rechazado y validación antes del
  lock de la identidad.
- «Imprimir con la plantilla vigente» y la vista previa con un documento real usan la identidad vigente.
- Con difuminado, la conversión se identifica con el umbral 128 y se reutiliza aunque cambie el umbral guardado.
- `converter_version` forma parte de la llave de una conversión, por si el algoritmo cambia.
- Las escrituras de la identidad responden sin imágenes (así se guardan en `restaurant_operations`) y la pantalla vuelve a
  consultar `GET`; guardar sin cambios no crea revisión, auditoría ni evento.
- Se agregó `POST documents/identity/logo/preview` (no estaba en el diseño): ajustar el umbral con el dedo exige ver otra
  presentación sin guardarla.

## Funciones del sistema anterior no incluidas

El sistema anterior (`PuntoDeVenta.cs`, `CapturaOrden.cs`) enviaba ESC/POS directo por TCP con un encabezado escrito en el
código. Se reportaron en la fase 0 y **no se implementaron; requieren confirmación del propietario**:

1. **Logotipo guardado en la memoria de la impresora** (imagen NV, `FS p`): la 9B envía la imagen raster en cada
   documento, como pide §0-B.
2. **48 columnas en 80 mm**: AD-EL POS usa 42 (y 30 en 58 mm) desde la etapa 5; un número de columnas por impresora
   cambiaría todos los documentos.
3. **Precio unitario por renglón**: las unidades de una partida pueden tener importes distintos por el reparto de
   centavos; imprimirlo requeriría una regla nueva.
4. **RFC como dato propio del establecimiento**: no existe; la «Información adicional» (texto libre) permite imprimirlo
   sin crear un dato fiscal.

## Pruebas

Lo ejecutado, con su salida, está en [VERIFICATION_STAGE9B.md](VERIFICATION_STAGE9B.md). Todas las pruebas usan
adaptadores simulados (transporte controlado de la cola, puerto TCP de loopback, ejecutor simulado de CUPS): **ninguna
imprimió en papel**.

- **Equivalencia byte a byte** (`kitchen/legacy_template_test.go` y `template_equivalence_test.go`,
  `payments/legacy_documents_test.go` y `documents_equivalence_test.go`): copias literales de las plantillas anteriores contra el renderizador nuevo en
  6 144 comandas y avisos (incluida la prueba técnica), 168 precuentas y tickets y más de 1 000 comprobantes y cortes, a
  58 y 80 mm; bytes ESC/POS y argumentos de `lp` del camino plano sin cambios.
- **Unitarias de `documents`** (`validate_test.go`, `render_test.go`, `migration_test.go`): validación (tipos
  desconocidos e incompatibles, protegidos fuera de cortes, límites, aviso fiscal, controles), IR, campos, muestras,
  codificación ESC/POS y semilla de la 0018 igual a Go con sus permisos. **Transportes** (`kitchen/transport_formatted_test.go`,
  `transport_logo_test.go`): plano idéntico, atributos, CUPS raw y sin raw, logotipo real por ambas colas.
- **`logoraster` y `brand`**: aceptación y rechazo de archivos (vacío, > 1 MiB, SVG, GIF con extensión engañosa, WebP,
  HTML, BMP, dimensiones enormes sin decodificar, truncados), umbral y Floyd–Steinberg exactos, alineación, anchos,
  altura máxima, transparencia, bytes `GS v 0` y franjas; la marca embebida igual al original.
- **Integración PostgreSQL**: `internal/designer` (`TestDocumentDesignerAcceptance`: semilla igual a Go, crear, editar, duplicar, renombrar, activar,
  restaurar, inmutabilidad, dos administradores, muestras sin ventas ni movimientos, permisos), `internal/identity`
  (14 subpruebas: textos, PNG y JPEG, rechazos, reemplazo, retiro, inmutabilidad, concurrencia, permisos, evento SSE,
  reportes y conexión con el renderizador), `internal/database` (`TestStage9BTemplatesUpgrade`), `internal/reports` (encabezado con logotipo) y la
  aceptación encadenada de `internal/operations` (`documents_stage9b_test.go` y `documents_logo_9b_test.go`: impresión con la versión activa, reimpresión idéntica tras editar en
  comanda, precuenta, ticket, comprobante y corte, plantilla vigente con permiso y auditoría, resultado incierto,
  protegidos sin permiso y en arqueo, comanda con logotipo y reimpresión tras reemplazarlo y retirarlo).
- **Vitest**: `pages/documents/designer` (editor, borrador, propiedades, reordenar, conflictos, permisos),
  `templates/templates.test.tsx`, `rerender.test.tsx`, `identity/identityPanel.test.tsx` y `hooks/useRealtime.test.tsx`.
- **Rust**: lista cerrada de `documents_*`, de la identidad y de la plantilla vigente en `api.rs`.
- **Playwright** (Chromium y WebKit): `e2e/workflow-stage9b-designer.spec.ts` (duplicar, editar, vista previa 58/80,
  guardar, activar, muestra, historial y reimpresión exacta) y `e2e/workflow-stage9b-identity.spec.ts` (subir logotipo,
  vista previa 58/80, presentación, retiro, sin desbordamiento a 1280 × 720 y 1920 × 1080 en ambos temas). Todas las
  especificaciones comparten la base de cada motor y corren en orden alfabético: las nuevas deben ordenarse **después**
  de las existentes (con el nombre anterior, `documents-*.spec.ts`, corrían antes que `restaurant.spec.ts` y le
  consumían el folio 1).

## Limitaciones conocidas

- **Instalación real**: 0018 y 0019 sin aplicar hasta la autorización del propietario.
- **Papel**: ningún documento con plantilla propia, formato o logotipo se imprimió en una impresora real; el resultado del
  logotipo en papel es una prueba física pendiente. Las impresoras de 80 mm con 512 puntos imprimibles recortarían 64
  puntos a la derecha (un ancho imprimible por impresora no se pidió).
- **Sin verificar en hardware**: ventana nativa WKWebView, pantalla táctil física, dos terminales reales y cajón.
- **Escalado**: usa punto flotante de `x/image/draw`; entre arquitecturas podría diferir algún píxel de borde (por eso la
  conversión se guarda). Un JPEG CMYK usa la conversión simple de Go; no se conservan perfiles de color.
- **Reportes**: un logotipo pequeño se amplía al mostrarse en el PDF o el Excel y puede verse borroso.
- **Editor**: los segmentos de línea del papel (tocar una línea para elegir su bloque) miden menos de 44 px (vía
  adicional; las filas de la estructura sí cumplen); arrastrar desde el panel de elementos solo funciona cuando ambas
  áreas se ven a la vez y, en pantalla táctil, empieza al mover en horizontal; la protección de cambios sin guardar no
  cubre el cierre de la ventana nativa.
- **Identidad**: un `GET documents/identity` con un original de 1 MiB pesa ≈ 1,4 MB; quien tiene
  `documents.identity.manage` sin permiso de lectura recibe la identidad en la respuesta de sus escrituras (no es un dato
  protegido).
- **Cobertura de navegador**: la reimpresión exacta se compara por la API y «Imprimir con la plantilla vigente» se recorre
  desde los documentos de la cuenta; en Caja y en la cola de cocina ese botón solo tiene pruebas Vitest.
- Los documentos de caja anteriores a la 9B solo admiten la reimpresión exacta.

## Cómo agregar un bloque o un dato

1. Declarar el bloque en `documents/model.go` (constante y estructura de opciones) y en `definitions` de
   `documents/catalog.go` (tipos compatibles, propiedades, opciones con sus valores, máximo, motivo de «sin datos»); un
   bloque con datos protegidos se marca `Protected` y solo se acepta en los tipos que ya los mostraban.
2. Validar sus opciones en `documents/validate.go` y renderizarlo en el `render*.go` de su familia, con el saneamiento y
   el corte de esa familia.
3. Un dato nuevo de `field` se agrega a la lista de su tipo en `documents/fields.go` y a su `…FieldValue`; si sale de un
   módulo, a los datos tipados de `documents/data.go` y a la conversión del módulo (`kitchen/template.go`,
   `payments/document_data.go` o `cash_document_data.go`).
4. **No cambiar las plantillas predeterminadas**: la 0018 las sembró y las pruebas de equivalencia exigen el mismo papel.
   Un bloque nuevo solo aparece en las plantillas que el restaurante edite.
5. Agregar pruebas unitarias de validación y renderizado a 58 y 80 mm y, si cambia el texto del editor, ajustar
   `desktop/src/pages/documents/designer/blockLabels.ts` (solo presentación: la lista sale del catálogo de Go).

## Instalación

Las migraciones 0018 y 0019 se aplican a una instalación existente con el procedimiento de
[DEVELOPMENT_MACOS.md](../DEVELOPMENT_MACOS.md) (respaldo → `restore-check.py` → `server-stop` → `migrate` → repetición
`applied:0` → `server-start` → `build-macos` → `open-app`), **solo con autorización expresa del propietario**. Hasta
entonces, `migrate`, `server-start` y `build-macos` desde la rama `etapa-9b` tocarían la instalación real (el servidor
0.9.1 declararía incompatible el esquema 17). Al aplicarlas, las diez plantillas del sistema quedan activas y la
identidad vacía y sin logotipo: ningún documento cambia hasta que el restaurante lo decida.
