# AD-EL POS — ETAPA 9C: NUEVA INTERFAZ PRINCIPAL, NAVEGACIÓN, TECLADO Y AJUSTES

Continúa el desarrollo de **AD-EL POS** sobre la versión que dejó la **9B** (0.9.1). Confirma la versión y el esquema reales en `config/product.json` y `CLAUDE.md`.

Esta entrega rediseña la **estructura de navegación** de toda la aplicación: elimina el menú lateral, agrega una barra superior táctil, una pantalla de Administración por mosaicos, un selector moderno para las listas desplegables y acceso completo por teclado, menú y paleta de comandos para quien no usa pantalla táctil. Incluye además algunos pendientes acordados.

**No cambia reglas de negocio, cálculos, permisos ni flujos operativos.** Cambia cómo se llega a cada pantalla y cómo se presentan los controles.

Al terminar, la versión será **0.9.2**. Si alguna parte requiere migración, que sea una sola y pide autorización antes de aplicarla a la instalación real.

---

## 0. FORMA DE TRABAJO

### Rama y especificaciones

Trabaja en una rama nueva **`etapa-9c`** creada desde `main`. No hagas push ni merge.

Las especificaciones en `docs/etapas/` son de solo lectura; lee únicamente este documento. Si existe `docs/etapas/ETAPA_9C_referencia.png`, es una maqueta de referencia visual aprobada con los ajustes de §0-B: úsala como guía de composición, no como diseño literal.

### Eficiencia

- La Fase 0 la hace **un solo agente**.
- **Máximo 2 agentes trabajando al mismo tiempo en total**, incluidos los que lancen otros agentes.
- En cada fase ejecuta solo las pruebas relacionadas con lo que cambió. La regresión completa (todas las suites, Playwright y la compilación) solo en la fase final.
- Cada agente lee solo las secciones de la documentación que necesita.

### Fase 0: verificación contra el código

Registra en `docs/ETAPA_9C_AVANCE.md`:

1. Cómo está hecha hoy la navegación: menú lateral, rutas o estado de página, cómo se filtran las opciones por permisos, pantalla de inicio por terminal y protección de cambios sin guardar al navegar.
2. Dónde están hoy el atajo de Caja (7.1), el indicador de conexión («Sesión verificada · En tiempo real»), el botón de pantalla completa (`WindowControls`), el menú del usuario y Mis preferencias.
3. Todas las listas desplegables nativas (`<select>`) de la aplicación y cuáles tienen muchas opciones.
4. Si existen atajos de teclado y si Tauri ya define un menú nativo.
5. Los reportes del sistema anterior de §5 (`CLAUDE.md` §21).

Si encuentras algo que obligue a cambiar una decisión del propietario, **detente y pregunta**. Si no, continúa.

### Fases

| Fase | Contenido |
| ---- | --------- |
| **0** | Verificación contra el código. |
| **A** | Barra superior, Administración por mosaicos, ruta de navegación, menú del usuario, estado de caja y del servidor, pantalla completa (§1). |
| **B** | Selector moderno para listas desplegables (§2). |
| **C** | Teclado, menú nativo y paleta de comandos (§3). |
| **D** | Ajustes visuales de mesas, selector de hora, estado del sistema, nombres y textos, apariencia (§2.1, §4) y pendientes acordados (§5). |
| **E** | Regresión completa, documentación y entrega. |

**Al terminar cada fase:** ejecuta sus pruebas, aplica el formato (sin tocar `docs/etapas/`), registra un resumen en el avance, haz un commit local y **continúa sin esperar confirmación** si todo pasa.

**Detente y pregunta únicamente si:** una prueba falla y arreglarla exige cambiar una decisión de este documento; encuentras una contradicción que obligue a una decisión funcional importante; necesitas aplicar una migración a la instalación real; o una acción podría afectar datos o servicios fuera del alcance.

---

## 0-B. DECISIONES DEL PROPIETARIO

**Si una sección parece decir otra cosa, prevalece esta.**

- **Se elimina el menú lateral.** Toda la navegación pasa a una barra superior de una sola fila.
- **Solo escritorio y pantallas táctiles de punto de venta.** No hay que adaptar la interfaz a celulares: la aplicación para meseros será un proyecto Android aparte. El tamaño mínimo de diseño sigue siendo 1280 × 720, aprovechando monitores más grandes.
- **Las acciones del panel de mesa no cambian.** Se conservan exactamente las de la 6.2 y la 7.1 (Capturar como principal, Cobrar, Precuenta, Dividir, Más…, Liberar mesa, fuera de servicio). Esta entrega solo reacomoda su presentación dentro del nuevo diseño.
- **Los colores de estado de mesa no cambian:** Libre (neutro), Ocupada (verde), Esperando pago (ámbar), Pago parcial (azul) y Fuera de servicio (**gris tenue con candado, nunca rojo**), como se implementaron en la 7.1.
- **El estado del turno de caja se conserva** como indicador en el botón Caja de la barra.
- **Botones de la barra de 52–56 px de alto** para uso táctil todo el día.
- No se rediseñan los contenidos internos de cada módulo (sus pestañas, formularios y tablas siguen como están), salvo los selectores de §2.

---

## 1. FASE A — BARRA SUPERIOR Y ADMINISTRACIÓN

### 1.1 Barra superior (una sola fila)

De izquierda a derecha:

1. **Logotipo de AD-EL POS.** Al tocarlo vuelve a la pantalla de inicio de la terminal.
2. **Secciones operativas** como botones grandes con **icono a la izquierda y texto**, la activa resaltada con fondo y borde: **Mesas, Para llevar, Cocina, Caja**.
   - **Caja** lleva un indicador del turno de la caja de la terminal: verde (turno abierto), ámbar (en arqueo de cierre), gris (sin turno). Al tocarlo conserva el menú rápido de la 7.1 (Ingreso, Retiro, Gasto, Abrir cajón, Corte parcial, Ir a Caja), con sus permisos.
3. **Administración** como botón con icono y texto, a continuación de las secciones operativas.
4. **Estado del servidor**: indicador compacto con tres estados (en línea, reconectando, fuera de línea), coherente con el estado real de la conexión y del SSE. Si el espacio no alcanza en 1280 px, se reduce a un punto de color con su descripción accesible y un aviso visible cuando no está «en línea». Sustituye los textos actuales «Sesión verificada · En tiempo real».
5. **Menú del usuario** (nombre y rol): Mis preferencias, Cambiar contraseña y Cerrar sesión. **Mis preferencias sale de Administración** porque es de cada usuario.
6. **Pantalla completa**: un solo botón con icono que cambia entre **maximizar** (entrar) y **reducir** (salir), con descripción accesible. Sustituye los textos «Salir de pantalla completa». Conserva el comportamiento actual de `WindowControls` y del modo ventana.

Reglas:

- Cada usuario ve solo las secciones que su rol permite. Un usuario con solo Mesas y Para llevar ve únicamente esos dos botones; si no tiene ninguna opción de administración, no ve el botón Administración.
- La barra no se desplaza con el contenido.
- La protección de cambios sin guardar sigue funcionando al navegar desde la barra.
- Temas claro y oscuro con los tokens existentes.

### 1.2 Barra reducida durante la captura

Dentro de la captura de pedidos (mesa o para llevar) la barra se reduce para dar el espacio a la captura: **«‹ Mesas»** (o «‹ Para llevar»), la mesa y cuenta (o el pedido), el indicador de Caja y el usuario. Los botones principales de la captura (Enviar a cocina y Cobrar) no cambian de lugar.

### 1.3 Administración por mosaicos

El botón Administración abre una pantalla de **mosaicos grandes** con icono, nombre y una línea de descripción, filtrados por permisos:

| Mosaico | Contiene |
| --- | --- |
| **Catálogo** | Categorías, productos, combos, complementos, porciones, grupos, listas de precios, promociones |
| **Inventarios** | El módulo de la etapa 8 con su navegación agrupada |
| **Reportes** | Dashboard y reportes de la 9A |
| **Documentos** | Plantillas e identidad del establecimiento (9B) |
| **Cobros** | Métodos de pago, IVA, políticas y motivos de descuento |
| **Personal** | Usuarios, roles y sesiones |
| **Sistema** | Configuración general, terminales, cajas físicas y **Registro de movimientos** (la actual Auditoría, §4.2) |

Los mosaicos que agrupan varias pantallas (Personal, Sistema) abren una segunda pantalla de mosaicos o una navegación por pestañas, lo que resulte más claro con la estructura actual.

### 1.4 Ruta de navegación

Dentro de Administración, una ruta con flecha de regreso: «‹ Administración › Catálogo › Productos». Cada parte de la ruta es tocable.

### 1.5 Pantalla de inicio por terminal

Si ya existe una preferencia de pantalla de inicio por terminal, conservarla y hacer que el logotipo vuelva a ella. Si no existe, agrégala (Mesas por defecto) en la configuración de la terminal.

---

## 2. FASE B — SELECTOR MODERNO PARA LISTAS DESPLEGABLES

Las listas desplegables nativas (`<select>`) se ven distintas en cada sistema, son difíciles de usar en pantalla táctil y no permiten buscar.

- Crea un **componente selector único**, al estilo de Select2 o un *select picker*, y úsalo en toda la aplicación en lugar de `<select>`.
- Requisitos:
  - Objetivos táctiles de al menos 44 px y lista con opciones amplias.
  - **Búsqueda** automática cuando la lista tiene muchas opciones (por ejemplo, más de 8): categorías, productos, insumos, proveedores, zonas horarias.
  - Selección múltiple donde hoy se usa.
  - Uso completo con teclado (flechas, Enter, Esc, escribir para buscar) y accesible para lectores de pantalla.
  - Temas claro y oscuro; funciona sin internet.
  - Compatible con el teclado virtual en la búsqueda y con la acción «+ Nueva categoría» de la 9A.
- Puedes usar una librería madura y ligera para React que respete el diseño del proyecto, o construirlo con los componentes existentes. Si agregas una dependencia, fíjala en el lockfile y regístrala en `docs/DEPENDENCIES.md`.
- Los selectores de tasas de IVA siguen siendo listas cerradas sin escritura libre.

### 2.1 Selector de hora

Capturar una hora escribiendo en el campo nativo es incómodo: hay que seleccionar el texto para reemplazarlo.

- Crea un **selector de hora** táctil: **hora** y **minutos** como dos selectores (el componente de §2), con pasos de minutos razonables según el uso (por ejemplo, de 5 en 5, o de 1 en 1 donde haga falta precisión), formato de 24 horas y uso completo con teclado.
- Úsalo en **todos** los lugares donde se captura una hora: hora de inicio del día, intervalos de las restricciones de horario, pedidos programados, reprogramación desde cocina y cualquier otro que encuentres en la Fase 0.
- Sin escritura libre. Go sigue validando el valor.

---

## 3. FASE C — TECLADO, MENÚ NATIVO Y PALETA DE COMANDOS

Quien usa la aplicación con teclado y ratón debe poder trabajar igual de rápido que en pantalla táctil.

### 3.1 Menú nativo de la aplicación (Tauri)

Agrega el menú nativo de la aplicación, en español:

- **Archivo:** Nueva cuenta en mesa, Cuenta sin mesa, Nuevo pedido para llevar, Imprimir precuenta, Cerrar sesión, Salir.
- **Ver:** Mesas, Para llevar, Cocina, Caja, Administración, Pantalla completa, Tema claro u oscuro.
- **Caja:** Ingreso, Retiro, Gasto, Corte parcial, Abrir cajón, Ir a Caja.
- **Ayuda:** Atajos de teclado, Acerca de AD-EL POS.

Reglas:

- Cada opción se habilita según los permisos del usuario y el contexto (por ejemplo, «Imprimir precuenta» solo con una cuenta abierta).
- Las opciones ejecutan **las mismas acciones** que los botones de la interfaz; no crean caminos nuevos hacia el servidor. Go sigue validando todo.
- En pantalla completa el menú nativo puede no verse (en macOS se oculta; en Windows no aparece). Por eso existen también los atajos y la paleta.
- Diseña el menú para que funcione en Windows cuando se prepare esa plataforma, sin depender de comportamientos exclusivos de macOS.

### 3.2 Atajos de teclado

Define un conjunto de atajos coherente, usando Cmd en macOS y Ctrl en Windows. Propuesta mínima (ajústala si choca con algo existente y documéntala):

- Cmd/Ctrl + 1, 2, 3, 4: Mesas, Para llevar, Cocina, Caja.
- Cmd/Ctrl + K: paleta de comandos.
- Cmd/Ctrl + F: buscar en la pantalla actual (productos en la captura, mesas, tablas).
- Esc: cerrar el diálogo o panel abierto, o volver un nivel.
- Pantalla completa con el atajo estándar de cada sistema.

Reglas:

- **Ningún atajo ejecuta una acción irreversible o financiera sin confirmación** (cobrar, cancelar, cerrar turno, devolver). Pueden abrir la pantalla correspondiente.
- Los atajos no actúan mientras el usuario escribe en un campo, salvo Esc.
- Los atajos aparecen junto a cada opción en el menú nativo, en la paleta y en las descripciones de los botones.
- Pantalla **«Atajos de teclado»** accesible desde Ayuda y desde la paleta.

### 3.3 Paleta de comandos

Con Cmd/Ctrl + K se abre una búsqueda rápida para ir a cualquier pantalla o ejecutar una acción escribiendo («productos», «retiro», «corte», «mesa 5»), filtrada por permisos y contexto. Usa el mismo registro de acciones que el menú nativo y los atajos, para que las tres vías siempre estén sincronizadas.

---

## 4. FASE D — AJUSTES VISUALES Y DETALLES DE LA INTERFAZ

Sin cambiar acciones, estados ni colores (§0-B):

- Mosaicos de mesa más legibles: icono de mesa, nombre o número grande, estado en texto, y cuando aplique comensales y tiempo ocupada, cada uno con su icono.
- El panel lateral de la mesa conserva sus acciones actuales, presentadas con iconos a la izquierda del texto y la acción principal destacada.
- Buscador de mesas y selector de área en el encabezado del mapa.

---

### 4.1 Pantalla de inicio y «Estado del sistema»

- **«Estado del sistema» deja de ser una sección del menú y nunca es la primera pantalla.** La aplicación abre en la pantalla de inicio de la terminal (§1.5; Mesas por defecto).
- En **Administración → Sistema → Configuración** queda un botón **«Ver estado del sistema»** que muestra solo información útil para el restaurante, en lenguaje sencillo: si hay conexión con la base de datos, si el servidor responde, la versión de AD-EL POS y, si algo falla, qué hacer («Reinicia el equipo servidor» o «Comunícate con soporte»).
- **No mostrar detalles técnicos:** nombres de tecnologías (Go, PostgreSQL, Tauri, React), direcciones como `127.0.0.1` o puertos, rutas de archivos, identificadores internos ni mensajes como «cerrar la aplicación no detiene Go ni PostgreSQL». Esos datos pueden quedar en un bloque «Información para soporte» plegado, visible solo para el administrador.

### 4.2 «Auditoría» pasa a llamarse «Registro de movimientos»

Renombra la sección y sus textos visibles. El permiso, las rutas y los datos no cambian. Revisa que no quede otra pantalla con el mismo nombre para algo distinto; si existe, dale un nombre diferenciado y repórtalo.

### 4.3 Textos sin términos técnicos

AD-EL POS se entrega como aplicación de escritorio: el usuario no necesita saber cómo está construida. Revisa los textos visibles de **todas** las pantallas y reemplaza las referencias técnicas por lenguaje del restaurante. Ejemplos actuales:

- «Datos del establecimiento guardados en PostgreSQL» → «Datos del establecimiento».
- «Productos, preparación y reglas de precio persistidos en el servidor» → «Productos, preparación y precios».
- «Los importes y las autorizaciones los calcula y valida el servidor» → quitar o reducir a una frase útil.
- «Identificador IANA del establecimiento» → «Zona horaria del restaurante».

No cambies textos de documentación técnica, registros internos ni mensajes de soporte del bloque plegado de §4.1.

### 4.4 Apariencia

La apariencia existe en dos lugares y debe quedar claro qué hace cada uno:

- En **Configuración general**: renombra el campo a **«Apariencia predeterminada para usuarios nuevos»**, con una línea que explique que cada usuario la cambia en Mis preferencias.
- En **Mis preferencias** (menú del usuario): la apariencia propia de ese usuario, que es la que se aplica.
- Verifica en la Fase 0 que cambiar la apariencia en Mis preferencias aplica el tema claro u oscuro de inmediato y se conserva al volver a entrar. Si no funciona, corrígelo.

## 5. PENDIENTES ACORDADOS

### 5.1 Reportes del sistema anterior

Agrégalos al módulo de Reportes de la 9A, con la misma arquitectura, permisos y exportación PDF/Excel:

- **Cuentas anuladas o canceladas** con quién las canceló, quién autorizó, motivo, importe y fecha.
- **Modificaciones de cantidades autorizadas**: partidas cuya cantidad se redujo o canceló después de confirmarse, con quién la hizo, quién autorizó y la diferencia.
- **Columna «Recibió»** (quién registró la recepción de mercancía) en el reporte de compras por periodo.
- **Folio o referencia del comprobante de tarjeta** en los reportes de cobros, donde se capturó.

Revisa en la Fase 0 qué datos existen para cada uno; si alguno no se guarda hoy, no lo inventes: repórtalo.

### 5.2 Imágenes del catálogo

`catalog/images.go` (etapa 3) decodifica JPEG sin limitar los barridos de un JPEG progresivo (la revisión de la 9B midió más de un minuto con un archivo de 1 MiB). Aplica la misma validación de `logoraster` (como máximo 100 barridos y 1024 segmentos antes de decodificar), con su prueba.

### 5.3 Columnas por impresora (42 o 48)

AD-EL POS imprime 42 columnas en 80 mm y 30 en 58 mm desde la etapa 5. El sistema anterior imprimía 48 en 80 mm. No todas las impresoras de 80 mm imprimen el ancho completo (muchas tienen 72 mm imprimibles), por eso el valor debe elegirlo el restaurante tras una prueba física.

- Configuración **por impresora**, en Cocina → Impresoras: **80 mm: 42 (predeterminado) o 48 columnas; 58 mm: 30 (predeterminado) o 32 columnas.** Lista cerrada, sin escritura libre. Validada en Go y en PostgreSQL. Cambiarla requiere el permiso de administrar impresoras y queda auditado.
- **Botón «Imprimir prueba de ancho»** junto a la opción: imprime una regla numerada del ancho elegido (por ejemplo `123456789012…` y una línea de `-` completa) más un renglón de ejemplo «producto … importe», marcada como prueba, sin crear ventas ni movimientos. Así el personal ve si el final del renglón se corta y decide.
- El renderizador único de la 9B usa las columnas de la impresora destino. La vista previa del diseñador muestra el ancho de la impresora elegida.
- **Con el valor predeterminado nada cambia:** las plantillas siguen produciendo exactamente la misma salida que hoy (comprobarlo con las pruebas de equivalencia de la 9B).
- Con 48 o 32 columnas, los renglones «concepto … importe» y los cortes de línea se recalculan al nuevo ancho; el texto grande de cocina sigue usando la mitad de las columnas.
- El logotipo no cambia: depende de los puntos de impresión (384 o 576), no de las columnas.
- **Reimpresión exacta:** los documentos que se reimprimen copiando su texto conservan el ancho con el que se emitieron. Los que se vuelven a renderizar al ancho de la impresora (comandas desde la cola) usan las columnas guardadas en el documento original para seguir saliendo idénticos; documenta el comportamiento elegido.
- Pruebas: equivalencia sin cambios con el valor predeterminado; salida correcta a 48 y 32 columnas; prueba de ancho sin ventas ni movimientos; valor fuera de la lista rechazado; reimpresión exacta tras cambiar las columnas de la impresora.

### 5.4 Formato y especificaciones

Si `docs/etapas/` todavía no está en `.prettierignore`, agrégalo para que `./scripts/dev.sh format` nunca reescriba las especificaciones del propietario.

---

## 6. PRUEBAS

- Vitest: barra por permisos (administrador, cajero, mesero), indicador de caja en sus tres estados, estado del servidor, botón de pantalla completa, ruta de navegación, mosaicos por permisos, selector (búsqueda, teclado, múltiple, accesibilidad), atajos que no actúan al escribir y que no ejecutan acciones financieras sin confirmación, paleta filtrada por permisos.
- Rust: menú nativo y sus eventos; ninguna operación nueva en el puente sin su prueba.
- Go: reportes nuevos de §5.1 con permisos y exportaciones; validación de JPEG del catálogo.
- Playwright (Chromium y WebKit): actualiza **todos** los recorridos existentes a la nueva navegación (siguen pasando con la barra superior), y agrega: navegación completa por la barra, Administración por mosaicos y ruta, captura con barra reducida, un recorrido solo con teclado (atajos y paleta), y vistas a 1280 × 720 y 1920 × 1080 en ambos temas sin desbordamientos.
- Repite todas las suites al final.

**No declares superada ninguna prueba que no hayas ejecutado.**

---

## 7. DOCUMENTACIÓN Y ENTREGA

Actualiza `CLAUDE.md` (estructura de navegación, componente selector, registro de acciones y atajos, menú nativo), `ARCHITECTURE.md`, `docs/REPORTS.md`, `docs/DEPENDENCIES.md` y `docs/VERIFICATION.md`, solo con lo realmente ejecutado. Agrega una guía breve `docs/NAVIGATION_AND_SHORTCUTS.md` con la estructura de la barra, los mosaicos y la lista de atajos.

Entrega final breve: cambios por fase, pruebas ejecutadas, limitaciones (ventana nativa, pantalla táctil física, menú en Windows) y procedimiento de instalación en espera de autorización si hubo migración.

No cambies reglas de negocio, cálculos ni permisos; no adelantes la etapa 10 ni la migración del sistema anterior.
