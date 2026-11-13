# Navegación, selector, teclado y atajos · Etapa 9C

Versión 0.9.2, API v1, esquema 20 (migración `0020_terminal_home_and_printer_columns.sql`, que agrega la pantalla de
inicio de cada terminal). Este documento describe solo lo implementado en la rama `etapa-9c`. Las decisiones del
propietario están en [etapas/ETAPA_9C_v3.md](etapas/ETAPA_9C_v3.md) §0-B; lo hecho y probado en cada fase, en
[ETAPA_9C_AVANCE.md](ETAPA_9C_AVANCE.md); la evidencia, en [VERIFICATION_STAGE9C.md](VERIFICATION_STAGE9C.md); los
contratos, en [API_CONTRACTS.md](../API_CONTRACTS.md) («Etapa 9C»).

La etapa 9C cambia **cómo se llega a cada pantalla y cómo se presentan los controles**; no cambia reglas de negocio,
cálculos, permisos ni flujos. Todo lo que aquí se ve lo sigue validando el servidor. Solo se probó con pruebas
automatizadas (Vitest y Playwright con Chromium y WebKit): la ventana nativa, el menú real de macOS, el menú en Windows
y la pantalla táctil física están **sin verificar** (ver «Lo que falta comprobar» al final).

**Etapa 9D** (versión 0.9.3, esquema 21; avance en [ETAPA_9D_AVANCE.md](ETAPA_9D_AVANCE.md) y evidencia en
[VERIFICATION_STAGE9D.md](VERIFICATION_STAGE9D.md)): la barra se reacomoda (Caja pasa al lado derecho, en dos renglones),
el estado del servidor dice «Sin conexión» en lugar de «Fuera de línea», los colores pasan al azul de la marca, desaparece
«Actualizar» de la captura y Escape en Mesas vuelve al resumen del área. Los atajos, el menú nativo y el registro de
secciones no cambian. Desde la fase D, «Cuenta sin mesa» y «Nuevo pedido para llevar» (botones, menú nativo y paleta) abren la captura sin
crear nada: la cuenta nace al guardar el primer producto; las pestañas de Mesas miden 44 px.

**Etapa 9F** (fase D; avance en [ETAPA_9F_AVANCE.md](ETAPA_9F_AVANCE.md)): la barra compacta (debajo de 1280 px, por
ejemplo con la ventana sin maximizar a unos 1500 px en Windows al 125 %) muestra el isotipo «AE» de la marca, el estado
corto del turno en Caja, «En línea» con su texto y el nombre del usuario; el logotipo ya no se queda con el contorno de
foco después de tocarlo; y la leyenda del mapa de mesas se acomoda en dos renglones en lugar de desplazarse de lado.

La primera parte es para el personal del restaurante; la segunda, para quien desarrolle.

## Contenido

- Para el restaurante: [La barra superior](#la-barra-superior) · [Caja](#caja) ·
  [Estado del servidor](#estado-del-servidor) · [Menú del usuario](#menú-del-usuario) ·
  [Pantalla completa](#pantalla-completa) · [La barra durante la captura](#la-barra-durante-la-captura) ·
  [Administración](#administración) · [Pantalla de inicio de la terminal](#pantalla-de-inicio-de-la-terminal) ·
  [El selector](#el-selector-listas-desplegables) · [Selector de hora](#selector-de-hora) ·
  [Menú de la aplicación](#menú-de-la-aplicación) · [Atajos de teclado](#atajos-de-teclado) ·
  [Paleta de comandos](#paleta-de-comandos)
- Para quien desarrolle: [Piezas](#piezas) · [Registro de secciones](#registro-de-secciones) ·
  [Registro de acciones y despachador](#registro-de-acciones-y-despachador) ·
  [Capa de teclado](#capa-de-teclado) · [Menú nativo](#menú-nativo-rust) ·
  [Cómo agregar una pantalla](#cómo-agregar-una-pantalla) · [Cómo agregar una acción](#cómo-agregar-una-acción) ·
  [Pruebas](#pruebas) · [Lo que falta comprobar](#lo-que-falta-comprobar)

# Para el restaurante

## La barra superior

Ya no hay menú lateral. Toda la navegación está en una barra de una sola fila, fija arriba (no se desplaza con el
contenido), con botones de 52 px de alto. Desde la etapa 9D, a la izquierda van el logotipo y las cuatro secciones,
seguidas y **del mismo ancho**; a la derecha, Caja, el estado del servidor, el usuario y pantalla completa. De izquierda a
derecha:

| Elemento              | Qué hace                                                                                                         |
| --------------------- | ---------------------------------------------------------------------------------------------------------------- |
| Logotipo de AD-EL POS | Vuelve a la pantalla de inicio de la terminal (ver «Pantalla de inicio de la terminal»).                         |
| **Mesas**             | Mapa de mesas. Dentro siguen las pestañas Mesas, Cuentas (cuentas sin mesa) y Áreas y distribución.              |
| **Para llevar**       | Listado de pedidos para llevar y ventanilla (sin pestañas).                                                      |
| **Cocina**            | Preparación, cola de impresión y configuración de cocina (la pantalla se sigue llamando «Cocina y preparación»). |
| **Administración**    | Mosaicos de administración (ver «Administración»).                                                               |
| **Caja**              | A la derecha: «Caja» y debajo el estado del turno de esta terminal, con su menú rápido (ver «Caja»).             |
| Estado del servidor   | En línea, reconectando o sin conexión (ver «Estado del servidor»).                                               |
| Menú del usuario      | Nombre y rol; Mis preferencias, Cambiar contraseña y Cerrar sesión.                                              |
| Pantalla completa     | Un botón de icono: maximizar para entrar y reducir para salir.                                                   |

- La sección activa se ve resaltada con fondo y borde. Cada botón lleva icono a la izquierda y texto.
- **Cada usuario ve solo lo que su rol permite**: quien solo tiene permisos de Mesas y Para llevar ve únicamente esos dos
  botones; quien no tiene ninguna opción de administración no ve el botón Administración. Ocultar un botón no cambia
  ningún permiso: el servidor vuelve a comprobarlos en cada consulta y en cada cambio.
- Si la pantalla que estás usando tiene cambios sin guardar (hoy, el editor de plantillas de Documentos), salir desde
  la barra, el menú, la paleta o un atajo pregunta primero «Salir sin guardar».
- Temas claro y oscuro; desde la etapa 9D con el azul de la marca (la sección activa con fondo azul muy claro y borde azul).
- De 1280 a 1599 px la barra reduce sus rellenos para que quepan las cuatro secciones del mismo ancho, Caja en dos
  renglones y «Sin conexión»; si un nombre de usuario muy largo aún no cabe, solo él se acorta con puntos suspensivos (el
  nombre completo sigue en la descripción del botón). Debajo de 1280 px va la **barra compacta**; desde la etapa 9F
  muestra el isotipo «AE» de la marca (el mismo del ícono del instalador), Caja con el estado corto del turno
  («Abierto», «En arqueo» o «Cerrado»), «En línea» con su texto y el nombre del usuario, que es lo único que se acorta
  con puntos suspensivos. Debajo de 1150 px el turno y «En línea» quedan como un punto de color con su ayuda emergente
  y el usuario como icono. Se verificó a 1200, 1280, 1440, 1500 y 1920 px.
- Tocar el logotipo (o hacer clic en él) vuelve al inicio y no le deja el contorno de foco; el contorno solo aparece al
  llegar a él con el teclado (Tab), como en cualquier botón.

## Caja

El botón **Caja** (desde la etapa 9D a la derecha de la barra, en dos renglones: «Caja» y debajo el estado) muestra el
estado del turno de la caja asignada a esta terminal:

| Color | Texto           | Significado                                     |
| ----- | --------------- | ----------------------------------------------- |
| Azul  | «Turno abierto» | Se puede cobrar y registrar efectivo.           |
| Ámbar | «En arqueo»     | El cierre está en curso: la caja se pausa.      |
| Gris  | «Sin turno»     | No hay turno abierto (se captura, no se cobra). |

(En la 9C «Turno abierto» era verde.) Mientras llega el turno dice «Consultando…». En la barra compacta (debajo de
1280 px, desde la etapa 9F) el estado va abreviado: «Abierto», «En arqueo» o «Cerrado»; al dejar el dedo o el puntero
sobre el botón, la ayuda emergente dice el estado completo. Al tocarlo se abre el **menú rápido de la etapa 7.1**, alineado con el borde derecho del botón, con las mismas hojas de siempre: Ingreso, Retiro, Gasto, Abrir
cajón, Corte parcial, Abrir turno (solo sin turno) e Ir a Caja. Cada opción aparece solo con su permiso; Ingreso,
Retiro, Gasto y Corte parcial requieren un turno abierto. Nada se registra desde el menú: cada hoja pide los datos y la
confirmación de siempre.

- El menú rápido aparece para quien puede consultar el turno y los cobros. Quien solo puede ver la pantalla de Caja
  tiene un botón que lleva directamente a ella.
- El turno se consulta cada 15 segundos en todas las pantallas (antes de la 9C, solo en Mesas y Para llevar).

## Estado del servidor

Sustituye a los textos «Sesión verificada · En tiempo real»:

| Estado           | Qué significa                                                                                                               |
| ---------------- | --------------------------------------------------------------------------------------------------------------------------- |
| **En línea**     | El equipo servidor responde y los cambios llegan al momento.                                                                |
| **Reconectando** | El servidor responde, pero el aviso inmediato de cambios se está recuperando; la terminal se actualiza cada pocos segundos. |
| **Sin conexión** | No hay comunicación con el equipo servidor. Se reintenta solo; espera a que vuelva la conexión para guardar cambios.        |

Colores desde la etapa 9D: «En línea» azul, «Reconectando» ámbar y «Sin conexión» rojo (hasta la 9C se llamaba «Fuera de
línea»). Desde la 9D «En línea» se ve con su texto también a 1280 px (antes era solo un punto de color) y desde la 9F
también en la barra compacta hasta 1150 px; más angosto queda el punto, con una ayuda emergente que empieza por el
estado («En línea: …»). Reconectando o sin conexión, el texto queda siempre a la vista. Sin conexión, Mesas, Cuentas, Para llevar y la captura muestran
**«Reintentar»**, que vuelve a consultar la sesión y los datos. El estado sale de dos fuentes reales: la
consulta de la sesión cada 5 segundos y el canal de eventos (solo para quien lo recibe; sin canal, con la sesión
verificada, el estado es «En línea»).

## Menú del usuario

Muestra el nombre y el rol de quien inició sesión y ofrece:

- **Mis preferencias**: la **Apariencia** propia (clara u oscura, se aplica de inmediato y se conserva al volver a
  entrar) y el **Teclado en pantalla** de esta terminal. «Mis preferencias» ya no está en Administración, porque es de cada
  usuario, y ya no tiene «Pantalla inicial» (ahora la decide la terminal).
- **Cambiar contraseña**.
- **Cerrar sesión**.

La apariencia de **Configuración general** se llama ahora «Apariencia predeterminada para usuarios nuevos»: la reciben
los usuarios que se creen después; los que ya existen conservan la suya.

## Pantalla completa

Un solo botón de icono en la barra: **maximizar** para entrar a pantalla completa y **reducir** para salir, con su
descripción accesible («Pantalla completa» o «Salir de pantalla completa») y el atajo estándar de cada sistema
(⌃⌘F en macOS y F11 en Windows). Conserva el comportamiento de antes: la aplicación inicia en pantalla completa y
funciona también en ventana normal, sin modo quiosco. En el acceso y en el asistente inicial (sin barra) el botón sigue
arriba a la derecha.

## La barra durante la captura

Al capturar un pedido (de una mesa o para llevar) la barra se reduce para dejar el espacio a la captura:

- **«‹ Mesas»** (o «‹ Para llevar», o «‹ Cuentas» en una cuenta sin mesa) para volver;
- la mesa y la cuenta (o el pedido) en grande, con «Datos de la cuenta» y las otras cuentas de la mesa;
- el indicador de **Caja** y el usuario (desde la etapa 9D sin **Actualizar**: los datos llegan solos y **Reintentar**
  aparece solo sin conexión);
- el estado del servidor, solo cuando no está en línea.

**Enviar a cocina** y **Cobrar** no cambiaron de lugar.

La captura se conserva por sección: si vas a otra sección sin tocar «‹ Mesas» o «‹ Para llevar» (por ejemplo, con ⌘3 o
desde Caja), al volver a Mesas o a Para llevar se retoma la captura de la mesa o del pedido que dejaste, con sus
productos; para ver el mapa o el listado, toca «‹ Mesas» o «‹ Para llevar».

## Administración

El botón **Administración** abre una pantalla de mosaicos grandes, cada uno con icono, nombre y una línea de
descripción. Solo aparecen los que los permisos del usuario permiten:

| Mosaico         | Contiene                                                                                                         | Permiso con que se ve                                                                    |
| --------------- | ---------------------------------------------------------------------------------------------------------------- | ---------------------------------------------------------------------------------------- |
| **Catálogo**    | Categorías, productos, combos, complementos, porciones, grupos, listas de precios y promociones                  | `catalog.read`                                                                           |
| **Inventarios** | El módulo de la etapa 8 con su navegación agrupada                                                               | `inventory.read`                                                                         |
| **Reportes**    | Resumen ejecutivo y reportes                                                                                     | `reports.read`                                                                           |
| **Documentos**  | Plantillas de tickets y documentos e identidad del establecimiento                                               | `documents.templates.read`                                                               |
| **Cobros**      | Métodos de pago, IVA, políticas de cobro y motivos de descuento                                                  | `payments.methods.manage`, `payments.settings.manage` o `payments.tax.manage`            |
| **Personal**    | Segunda pantalla de mosaicos: **Usuarios**, **Roles** y **Sesiones**                                             | Alguno de sus mosaicos (`users.read`, `roles.read`, `sessions.read`)                     |
| **Sistema**     | Segunda pantalla de mosaicos: **Configuración**, **Terminales**, **Cajas físicas** y **Registro de movimientos** | Alguno de sus mosaicos (`settings.read`, `devices.read`, `registers.read`, `audit.read`) |

- **Cobros** ya no aparece solo por poder ver el turno: el turno actual se consulta en Caja. Ningún permiso cambió;
  solo qué mosaicos ve cada rol.
- **Ruta de navegación**: dentro de Administración, arriba del contenido, por ejemplo «‹ Administración › Sistema ›
  Terminales». Cada parte anterior se puede tocar para volver a ella; la pantalla actual es texto. Catálogo, Inventarios
  y Reportes agregan su pestaña o grupo como último tramo («‹ Administración › Catálogo › Productos»).
- **Esc** vuelve a la pantalla anterior de la ruta, salvo que haya un formulario a la vista (para no descartar lo
  capturado).
- **«Registro de movimientos»** es el nuevo nombre de «Auditoría» (mismo permiso, mismos datos). Se conservan los
  nombres parecidos de otras pantallas, que se distinguen por su lugar: «Caja → Movimientos», «Inventarios →
  Existencias → Movimientos» y el reporte «Movimientos de efectivo».
- **Estado del sistema** ya no es una sección ni la primera pantalla. Está en **Sistema → Configuración → «Ver estado
  del sistema»**, en lenguaje sencillo: si el servidor responde, si hay conexión con la base de datos, la versión de
  AD-EL POS y, si algo falla, qué hacer («Reinicia el equipo servidor…» o «Comunícate con soporte…»). Los datos técnicos
  quedan en un bloque plegado «Información para soporte», visible solo para quien administra la configuración del
  establecimiento (`settings.manage`: el administrador). Antes de iniciar sesión, «Ver estado del sistema» en la
  pantalla de acceso muestra el mismo resumen sencillo, **sin** el bloque de soporte (no se sabe quién usa la terminal).

## Pantalla de inicio de la terminal

Cada terminal tiene su pantalla de inicio: **Mesas** (por omisión), **Para llevar**, **Cocina** o **Caja**. Se elige en
**Administración → Sistema → Terminales → «Pantalla inicial»**, con el permiso de administrar terminales
(`devices.manage`, el mismo de siempre para configurarlas), y la columna «Pantalla inicial» del listado la muestra.

- La aplicación abre en esa pantalla al iniciar sesión, y el logotipo vuelve a ella.
- Si el usuario no puede ver esa sección, abre la primera de la barra que sí puede ver; si no ve ninguna,
  Administración, y si tampoco, Mis preferencias. **Nunca abre en «Estado del sistema».**
- La «Pantalla inicial» que antes se guardaba por usuario ya no decide nada (el servidor la conserva sin usarla).

## El selector (listas desplegables)

Todas las listas desplegables de la aplicación usan ahora el mismo selector, igual en todos los equipos y pensado para
pantalla táctil (ya no queda ninguna lista nativa):

- **Toca el campo** para abrir la lista. Cada opción mide al menos 44 px. La opción elegida queda marcada.
- **Búsqueda**: con más de 8 opciones (zonas horarias, productos, insumos, proveedores, categorías…) la lista trae un
  campo de búsqueda arriba. No distingue mayúsculas ni acentos y las palabras pueden ir en cualquier orden. Con el dedo,
  el teclado en pantalla no aparece solo: toca la búsqueda para escribir con él; la lista no se cierra al tocar el
  teclado y se coloca por encima de él. Algunas búsquedas (insumos, proveedores, preparaciones, ingredientes) las
  resuelve el servidor: mientras dice «Buscando…», Intro no elige nada.
- **Grupos** (por ejemplo, zonas horarias «México» y «Otras zonas»), opciones deshabilitadas (se ven, pero no se eligen)
  y **acciones al final**, siempre visibles, como «+ Nueva categoría» o «Otra unidad o presentación…».
- **Selección múltiple** (alcance de las promociones: productos, categorías y listas): cada opción se marca o desmarca,
  «Listo» cierra y lo elegido queda como etiquetas que se quitan con un toque.
- En los campos obligatorios, «Selecciona…» es solo el texto del campo vacío; en los opcionales y los filtros, «Sin
  asignar», «Todos» o «Sin filtro» siguen siendo opciones.
- Las tasas de IVA siguen siendo botones cerrados (16 %, 8 % o 0 %), sin escritura libre.

Con teclado:

| Tecla                         | Con la lista cerrada                                        | Con la lista abierta                                                 |
| ----------------------------- | ----------------------------------------------------------- | -------------------------------------------------------------------- |
| ↓, ↑, Intro, Espacio, Alt + ↓ | Abre en la opción elegida                                   | Flechas: recorren (saltan las deshabilitadas)                        |
| Inicio / Fin                  | Abre en la primera / la última                              | Primera / última opción                                              |
| Re Pág / Av Pág               | —                                                           | Una página                                                           |
| Intro (o Espacio en el campo) | —                                                           | Elige (en la múltiple, marca o desmarca)                             |
| Esc o Alt + ↑                 | —                                                           | Cierra sin cambiar y vuelve al campo (no cierra el diálogo de atrás) |
| Tab                           | Pasa al siguiente campo                                     | Cierra **sin cambiar** y pasa al siguiente campo                     |
| Escribir                      | Abre y busca (sin búsqueda: va a la opción que empieza así) | Busca                                                                |

## Selector de hora

Las horas ya no se escriben: se eligen con dos selectores, **Hora** y **Minutos**, en formato de 24 horas, de 5 en 5
minutos (o de 1 en 1 donde hace falta precisión). Con teclado, escribir «1» y «4» seguidos lleva a las 14. Se usa en
la hora de inicio del día (Configuración general), la hora inicial y final de las restricciones de horario (productos,
presentaciones, categorías y promociones; la final admite «24», medianoche, fin del día), el pedido programado y la
reprogramación desde cocina (con fecha dd/mm/aaaa y hora) y los filtros Desde/Hasta del Registro de movimientos (de
minuto en minuto; sin hora proponen 00:00 y 23:59). Una hora guardada que no cae en el paso (por ejemplo, 05:07) se
conserva como opción. El servidor sigue validando cada valor.

## Menú de la aplicación

La aplicación tiene su propio menú en español. Sus opciones hacen **exactamente lo mismo** que los botones de la
pantalla, con los mismos permisos; se deshabilitan según los permisos y el contexto (por ejemplo, «Imprimir precuenta»
solo con una cuenta con productos enviados a cocina).

| Menú                   | Opciones                                                                                                     |
| ---------------------- | ------------------------------------------------------------------------------------------------------------ |
| AD-EL POS (solo macOS) | Acerca de AD-EL POS, Ocultar AD-EL POS, Ocultar otras, Mostrar todas, Salir de AD-EL POS                     |
| Archivo                | Nueva cuenta en mesa, Cuenta sin mesa, Nuevo pedido para llevar, Imprimir precuenta, Cerrar sesión, Salir    |
| Edición                | Deshacer, Rehacer, Cortar, Copiar, Pegar, Seleccionar todo (los del sistema) y Buscar en la pantalla         |
| Ver                    | Mesas, Para llevar, Cocina, Caja, Administración, Paleta de comandos, Pantalla completa, Tema claro u oscuro |
| Caja                   | Ingreso, Retiro, Gasto, Corte parcial, Abrir cajón, Abrir turno, Ir a Caja                                   |
| Ayuda                  | Atajos de teclado, Acerca de AD-EL POS                                                                       |

- Las opciones de Caja **solo abren su hoja**, como el menú rápido; nada se registra sin la confirmación de siempre.
- No hay «Cerrar ventana» (⌘W cerraría la única ventana).
- Con un diálogo abierto, el menú no actúa detrás de él (avisa «Cierra la ventana abierta para continuar.»), salvo
  «Pantalla completa».
- **Salir** cierra esta aplicación **sin detener el servidor del restaurante**: las demás terminales siguen funcionando.
  No pregunta por cambios sin guardar.
- El menú no explica por qué una opción está deshabilitada; la paleta de comandos sí.
- En pantalla completa el menú puede no verse (macOS lo oculta; en Windows no aparece): por eso existen también los
  atajos y la paleta. El menú se diseñó para funcionar en Windows (empieza en Archivo, sin el menú «AD-EL POS»), pero
  ahí **no se ha compilado ni probado**.

## Atajos de teclado

Usan **⌘ (Comando) en macOS y Ctrl en Windows**. La lista completa también está en **Ayuda → Atajos de teclado** y en la
paleta, y cada botón que tiene atajo lo muestra en su descripción.

| macOS    | Windows              | Acción                                                                                                                                         |
| -------- | -------------------- | ---------------------------------------------------------------------------------------------------------------------------------------------- |
| ⌘1       | Ctrl+1               | Mesas (dentro de la captura de una mesa equivale a «‹ Mesas»)                                                                                  |
| ⌘2       | Ctrl+2               | Para llevar (dentro de su captura equivale a «‹ Para llevar»)                                                                                  |
| ⌘3       | Ctrl+3               | Cocina                                                                                                                                         |
| ⌘4       | Ctrl+4               | Pantalla de Caja (el botón Caja de la barra abre su menú rápido)                                                                               |
| ⌘5       | Ctrl+5               | Administración                                                                                                                                 |
| ⌘K       | Ctrl+K               | Abre (o cierra) la paleta de comandos                                                                                                          |
| ⌘F       | Ctrl+F               | Buscar en la pantalla: productos en la captura, el buscador de mesas en el mapa, cuentas, tablas de Administración; sin buscador, la paleta    |
| ⌘N       | Ctrl+N               | Nueva cuenta en mesa: abre la captura en la mesa elegida (la del panel o la de la captura) sin crear la cuenta; sin mesa elegida, abre el mapa |
| ⌘P       | Ctrl+P               | Imprimir precuenta: **abre** la precuenta de la cuenta a la vista; imprimir sigue siendo su botón                                              |
| ⌃⌘F      | F11                  | Pantalla completa (entrar o salir)                                                                                                             |
| Esc      | Esc                  | Cerrar o volver un nivel (ver abajo)                                                                                                           |
| ⌘C ⌘V ⌘X | Ctrl+C Ctrl+V Ctrl+X | Copiar, pegar y cortar en los campos                                                                                                           |
| ⌘Z ⇧⌘Z   | Ctrl+Z Ctrl+Y        | Deshacer y rehacer en los campos                                                                                                               |
| ⌘A       | Ctrl+A               | Seleccionar todo el texto del campo                                                                                                            |

Reglas:

- **Ningún atajo cobra, cancela, cierra un turno, devuelve ni registra efectivo.** Las opciones de Caja no tienen atajo
  y solo abren su hoja; «Cuenta sin mesa» y «Nuevo pedido para llevar» tampoco (en la 9C creaban la cuenta al instante;
  desde la fase D de la 9D solo abren la captura sin crear nada —su descripción dice «Se abre al guardar el primer
  producto»—, y siguen sin atajo). Desde el menú o la paleta piden primero la sección y abren la captura solo cuando la
  pantalla la recibió (así la vista principal de la sección no queda encima de la captura).
- **Los atajos no actúan mientras escribes en un campo** (salvo Esc), ni con un diálogo, hoja, menú o lista abiertos
  (la pantalla completa sí, con un diálogo abierto), ni al mantener la tecla.
- **Esc**: cierra el diálogo, la hoja, el menú o la lista abiertos (cada uno con su propio Esc); cierra el panel abierto
  de la cuenta (como su «Volver»); en la captura vuelve al mapa o al listado (como «‹ Mesas»); cierra el panel lateral de
  la mesa (desde la 9D el panel queda a la vista con el resumen del área); en Administración vuelve a la pantalla anterior de la ruta (no con un formulario a la vista). En un campo, el
  primer Esc solo sale del campo. **En el cobro, Esc sigue limpiando el importe**, como antes. Esc no cierra la
  personalización de un producto en la captura (su «Volver» confirma el descarte). Desde la fase D de la 9D, Esc cierra el
  menú de un producto del detalle de la cuenta (mientras está abierto, los atajos no actúan).
- Con el cobro abierto, «Nueva cuenta en mesa», «Cuenta sin mesa», «Nuevo pedido para llevar» y elegir otra mesa en la
  paleta quedan deshabilitados (cerrar el cobro perdería lo tecleado); ⌘1 y ⌘2 en el cobro solo navegan.

## Paleta de comandos

**⌘K** (Ctrl+K en Windows), o **Ver → Paleta de comandos**, abre una búsqueda rápida para ir a cualquier pantalla o
ejecutar una acción escribiendo, por ejemplo, «productos», «retiro», «corte» o «mesa 5»:

- Contiene las mismas acciones del menú, todas las pantallas de Administración que el usuario puede ver (con su ruta) y
  las mesas del mapa.
- Busca sin acentos ni mayúsculas, por palabras al inicio del nombre o de sus sinónimos: «productos» encuentra Catálogo,
  «auditoría» el Registro de movimientos y «mesa 5» encuentra «Mesa 5» sin «Mesa 15».
- Está filtrada por permisos; lo que el contexto impide se ve deshabilitado con su motivo.
- Teclado: flechas para recorrer, Re Pág/Av Pág, Ctrl + Inicio/Fin, **Intro** ejecuta, **Esc** o ⌘K la cierra.
- Las mismas reglas de los atajos: nada financiero ni irreversible sin su confirmación de siempre (las acciones de Caja
  abren su hoja; «Imprimir precuenta» abre la precuenta). Elegir una mesa es como tocarla en el mapa.
- «Atajos de teclado» y «Acerca de AD-EL POS» se abren desde la paleta y desde Ayuda, también antes de iniciar sesión.

# Para quien desarrolle

## Piezas

| Archivo                                                                                                           | Responsabilidad                                                                                                       |
| ----------------------------------------------------------------------------------------------------------------- | --------------------------------------------------------------------------------------------------------------------- |
| `desktop/src/navigation/sections.ts`                                                                              | Registro único de secciones: pantallas, permisos, mosaicos, ruta e inicio de la terminal.                             |
| `navigation/NavigationContext.tsx`                                                                                | `useNavigation()` (pantalla, inicio y el único `requestScreen`) y `useBreadcrumbTail(label)`.                         |
| `navigation/TopBar.tsx`, `TileScreen.tsx`, `Breadcrumb.tsx`, `UserMenu.tsx`, `ServerStatus.tsx`, `serverState.ts` | Barra superior, mosaicos, ruta, menú del usuario y estado del servidor.                                               |
| `desktop/src/commands/actions.ts`                                                                                 | Registro único de acciones (menú nativo, atajos y paleta) y entradas de la paleta.                                    |
| `commands/dispatcher.ts`                                                                                          | Despachador único: evita la doble ejecución.                                                                          |
| `commands/useCommandCenter.tsx`, `keyboard.ts`, `platform.ts`, `search.ts`, `hints.ts`                            | Capa de teclado, atajos por plataforma, búsqueda de la paleta y atajo en la descripción de los botones.               |
| `commands/nativeMenu.ts`, `operationBridge.ts`                                                                    | Puente con el menú de Rust; contexto y botones de la pantalla operativa.                                              |
| `commands/CommandPalette.tsx`, `CommandDialog.tsx`, `HelpDialogs.tsx`                                             | Paleta, «Atajos de teclado» y «Acerca de».                                                                            |
| `desktop/src-tauri/src/menu.rs`                                                                                   | Menú nativo, lista cerrada `MENU_ACTIONS` y las operaciones `menu_subscribe` y `menu_set_state`.                      |
| `desktop/src/components/Select.tsx`, `SelectPopup.tsx`, `selectHooks.ts`, `selectModel.ts`                        | Selector único (`Select` y `MultiSelect`).                                                                            |
| `desktop/src/components/TimeField.tsx`                                                                            | Selector de hora (`TimeField` y `DateTimeField`).                                                                     |
| `desktop/e2e/navigation.ts`, `e2e/select.ts`, `src/testing/select.ts`                                             | Ayudantes de Playwright (barra, mosaicos, selector) y de Vitest (selector).                                           |
| `desktop/e2e/native-driver.ts`                                                                                    | Simula `menu_subscribe`/`menu_set_state` leyendo `MENU_ACTIONS` de `menu.rs`; ofrece `clickMenu(id)` y `menuState()`. |

Desde la etapa 9D, `requestScreen` de `NavigationContext` devuelve `Promise<boolean>` (`true` si la pantalla cambió, como
ya hacía la función de `App.tsx`): las acciones que cambian de sección y después abren algo (la paleta, el menú nativo y
«Nuevo pedido para llevar» del resumen de Mesas) esperan ese resultado y no abren nada si el usuario eligió seguir
editando.

La aplicación no tiene rutas de URL: `App.tsx` guarda la pantalla visible en su estado y la dibuja en `content()`.
**Toda navegación pasa por un único `requestScreen`** (barra, logotipo, mosaicos, ruta, menú del usuario, «Ir a Caja»,
Inventarios → Configuración, Documentos → Configuración, «Ver estado del sistema», menú nativo, paleta, atajos y Esc),
que respeta `useLeaveGuard` (diálogo «Salir sin guardar»). Cerrar sesión usa la misma confirmación.

## Registro de secciones

Cada `SectionEntry` de `sections.ts`:

| Campo                                             | Uso                                                                                                                             |
| ------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------- |
| `id: ScreenID`                                    | Pantalla de `App.tsx` (`restaurant`, `takeaway`, `kitchen`, `cash`, `administration`, `catalog`, …, `preferences`, `password`). |
| `label`, `icon`, `description`                    | Nombre visible (barra, mosaico, ruta, paleta), icono de `components/Icon.tsx` y línea del mosaico.                              |
| `group`                                           | `operation` (barra superior), `administration` (Administración y lo que contiene) o `personal` (menú del usuario).              |
| `parent`                                          | Pantalla que la contiene: forma los mosaicos y la ruta (`users → staff → administration`).                                      |
| `permission` / `anyPermission` / `allPermissions` | Permiso exacto, alguno de, y todos los adicionales. Son las reglas de la tabla `screens` de antes, movidas aquí.                |
| `grouping`                                        | Pantalla de mosaicos (Administración, Personal, Sistema): se ve si alguna de sus pantallas se ve.                               |
| `hiddenTile`                                      | Se abre desde otra pantalla y no es mosaico («Estado del sistema», desde Configuración).                                        |
| `home: TerminalHome`                              | Valor de `devices.home_screen` que abre la sección (`tables`, `takeaway`, `kitchen`, `cash`).                                   |

Funciones: `screenAuthorized`, `canOpen`, `barSections` (secciones de la barra en orden), `tilesOf`, `breadcrumbOf`,
`barHighlight`, `resolveHome` (inicio de la terminal con sus respaldos; nunca `status`) y `terminalHomes`. Las reglas de
permisos **solo deciden qué se muestra**; Go valida todo.

Inicio por terminal en Go: `GET auth/me` informa `terminal.home_screen`; `GET devices` lo incluye en cada terminal;
`PATCH devices/{id}` lo acepta como campo opcional con `devices.manage` (lista cerrada `terminalHomeScreens` en
`administration/devices.go`, CHECK `devices_home_screen_check` de la 0020; sin el campo se conserva el guardado),
auditado en `device.updated`.

## Registro de acciones y despachador

`commands/actions.ts` declara 23 acciones. Cada `ActionDefinition` tiene:

- `id` (por ejemplo `view.tables`, `cash.withdrawal`), `label`, `group` de menú (`file`, `edit`, `view`, `cash`, `help`),
  `keywords` (sinónimos de la paleta), `description` y, si tiene, `shortcut` (`modShortcut('1')` = ⌘1/Ctrl+1;
  `fullscreenShortcut` = ⌃⌘F/F11).
- `visible(context)`: las **mismas reglas de permisos** que el botón de la interfaz. Si no, la acción no aparece en la
  paleta y queda deshabilitada en el menú.
- `enabled(context)` y `hint`: el contexto (cuenta a la vista, turno abierto, cobro abierto…); deshabilitada, la paleta
  muestra el `hint`.
- `run(runtime, context)`: **la misma función que el botón** (`requestScreen`, abrir la hoja de Caja, «Capturar»,
  «Cuenta sin mesa», «Nuevo pedido», «Precuenta», el tema de Mis preferencias, cerrar sesión, pantalla completa). Nunca
  hay un camino nuevo hacia el servidor.

`CommandContext` reúne autenticación, permisos, pantalla, si hay ventana de escritorio, el estado de la operación
(`OperationCommandState` de `operationBridge.ts`) y las opciones del menú rápido de Caja (`shortcutOptions`).
`commandEntries` arma la paleta: las acciones visibles, las pantallas autorizadas que no cubre una acción (con su ruta y
los sinónimos de `screenKeywords`) y las mesas del mapa.

**Despachador** (`dispatcher.ts`): menú, teclado y paleta ejecutan por `dispatch(id, source, run)`.

- En la ventana nativa, un atajo que también es acelerador del menú puede llegar dos veces (la pulsación al WebView y
  el evento del menú). Una misma acción que llega **por otra vía** dentro de 700 ms (`twinWindowMs`) es su gemela y se
  descarta **una sola vez**.
- Una pulsación que no se ejecutó (mientras se escribe, con un diálogo abierto o deshabilitada) se registra igual
  (`suppress`/`record`) y consume a su gemela del menú.
- Una acción asíncrona en curso (una navegación que espera «Salir sin guardar», guardar el tema) no se vuelve a
  ejecutar.

## Capa de teclado

`useCommandCenter.tsx`:

- Un oyente en `window` (fase de burbuja) resuelve los atajos con `matchesShortcut` (los dígitos aceptan también
  `event.code`, para cualquier distribución). Al coincidir, **siempre** hace `preventDefault` (el navegador no busca,
  imprime ni abre ventana, y en macOS el acelerador nativo no repite la acción).
- No actúa con la tecla mantenida (`event.repeat`), en un campo (`isEditable`: `input` de texto, `textarea`, `select`,
  contenido editable y roles `textbox`, `searchbox`, `combobox` y `spinbutton`, que incluye el disparador del selector)
  ni con algo abierto (`overlayOpen`: `aria-modal`, `role=dialog|alertdialog|listbox|menu`, la hoja y el menú de Caja, los
  menús de la barra, «Más…» y la paleta de colores; desde la fase D de la 9D también el menú de un producto,
  `.capture-line-menu`); la pantalla completa sí actúa con algo abierto.
- **Esc**: una foto en la fase de captura (¿había algo abierto?, ¿el foco estaba en un campo o en un control propio con
  `tabindex`?) y la decisión después de todos los oyentes (`setTimeout 0`): si alguien lo atendió (`defaultPrevented`,
  como el teclado del cobro) o había algo abierto, no hace nada; en un campo solo sale del campo; si no, `back()` de la
  operación o la pantalla anterior de la ruta en Administración (no con un `form` a la vista, `formShown`).
- **⌘F** (`focusSearch`): el buscador de la pantalla visible (`screenSearch`: el marcado con `data-screen-search` —el
  de productos de la captura y el de mesas del mapa— o el primer `input[type=search]` a la vista); si no hay, la paleta.

## Menú nativo (Rust)

`menu.rs` arma el menú en Rust (`Builder::menu` y `on_menu_event` en `main.rs`), sin permisos `core:*` nuevos:

- `layout(platform)` es un dato puro con la estructura por plataforma: el submenú «AD-EL POS» solo en macOS (allí el
  primer submenú es el de la aplicación); Edición con los elementos **predefinidos** (sin ellos se pierden ⌘C/⌘V en los
  campos); sin submenú Ventana ni ⌘W.
- `MENU_ACTIONS` es la **lista cerrada** de opciones propias (id, texto, acelerador para macOS y para Windows en sintaxis
  de Tauri, `CmdOrCtrl+N`). Rust no ejecuta ninguna acción de negocio ni conoce rutas del servidor: al elegir una opción
  envía `{ "action": id }` a React. La única que atiende Rust es «Salir» de Archivo (`file.quit`, fuera de la lista):
  `app.exit(0)`.
- **`menu_subscribe(channel)`**: React se suscribe con un `Channel`; una suscripción nueva sustituye a la anterior.
- **`menu_set_state(enabled)`**: React informa las opciones habilitadas (solo cuando cambian); las demás se
  deshabilitan; un identificador fuera de `MENU_ACTIONS` (incluido `file.quit`) o una lista más larga que ella rechaza
  todo el estado. Antes del primer estado solo están habilitadas «Pantalla completa», «Atajos de teclado» y «Acerca de».
- Registradas en `build.rs`, `permissions/diagnostics.toml` (`allow-diagnostics`) y `generate_handler!`.
- `e2e/native-driver.ts` lee `MENU_ACTIONS` con una expresión regular: cada entrada conserva la forma
  `MenuAction { id: "…", label: "…", mac: …, other: … }` (rustfmt la escribe en varias líneas).

## Cómo agregar una pantalla

1. Agregar el identificador a `ScreenID` y su `SectionEntry` en `sections.ts`: nombre, icono, grupo, `parent` (el
   mosaico que la contiene), descripción y la regla de permisos que ya usa su módulo (sin inventar permisos nuevos; si
   hace falta uno, va en Go y en una migración, concedido solo al administrador).
2. Dibujarla en el `switch` de `content()` en `App.tsx`. Aparece sola en su pantalla de mosaicos, en la ruta y en la
   paleta.
3. Navegar **solo** con `useNavigation().requestScreen(id)`; nunca cambiar la pantalla directamente (se saltaría «Salir
   sin guardar»). Si la pantalla tiene cambios sin guardar, registrarlos con `useLeaveGuard`.
4. Si tiene pestañas, informar la actual con `useBreadcrumbTail(etiqueta)` para el último tramo de la ruta.
5. Sinónimos para la paleta en `screenKeywords` de `actions.ts`.
6. Que una pantalla nueva pueda ser la pantalla de inicio de una terminal exige ampliar la lista cerrada en Go
   (`terminalHomeScreens`), el CHECK `devices_home_screen_check` (migración nueva) y `TerminalHome`: solo con una decisión
   del propietario.
7. Pruebas: `navigation/navigation.test.tsx` (mosaicos y barra por permisos) y Playwright con `e2e/navigation.ts`.

## Cómo agregar una acción

1. Agregar el identificador a `ActionID` y su `ActionDefinition` en `actions.ts`: `visible` con las reglas de permisos
   de su botón, `enabled` y `hint` para el contexto, y `run` llamando a la **misma función** del botón. Si necesita el
   estado o un botón de la pantalla operativa, ampliar `OperationCommandState` / `OperationCommandHandlers` de
   `operationBridge.ts`.
2. Atajo solo si no choca con otro (una prueba exige que sean únicos) y **nunca** para una acción financiera, irreversible
   o que cree registros al instante; esas solo abren su pantalla u hoja.
3. Para que esté en el menú nativo: agregar su `MenuAction` a `MENU_ACTIONS` en `menu.rs` (mismo id, texto y
   acelerador) y colocarla en `layout()`. La prueba de Vitest «tiene las mismas opciones, textos y atajos» compara ambos
   registros y las pruebas Rust de `menu::tests` revisan la lista cerrada, los aceleradores y la estructura.
4. Mostrar el atajo en el botón con `shortcutProps(id, etiqueta)` de `hints.ts` (`title` y `aria-keyshortcuts`) y, si
   corresponde, agregarlo a los grupos de `HelpDialogs.tsx` («Atajos de teclado»).
5. Pruebas: `commands/commands.test.ts` (permisos, contexto, despachador), `commands/commandCenter.test.tsx` (teclado,
   menú simulado) y, si cambia el recorrido, `e2e/workflow-stage9c-keyboard.spec.ts` con `clickMenu(id)`.

Una operación nueva del puente (como `menu_subscribe`) se agrega a `build.rs`, `diagnostics.toml`,
`generate_handler!`, su prueba Rust y su simulación en `native-driver.ts`; nunca rutas arbitrarias ni tokens en React.

## Pruebas

- **Selector**: `components/Select.test.tsx` (15 pruebas: nombre por etiqueta, ratón, teclado completo, búsqueda sin
  acentos con anuncio, grupos, acciones, validación dentro de un acordeón, deshabilitado, múltiple con etiquetas, búsqueda
  en Go, teclado virtual, apertura con el dedo, posición dentro de la ventana y opción con valor vacío). Para elegir
  opciones en pruebas, usar los ayudantes (`e2e/select.ts` en Playwright; `src/testing/select.ts` en Vitest); en Vitest no
  abrir ni cerrar un selector dentro de `waitFor` (usar `waitForOption` o `chooseOptionAsync`).
- **Hora**: `components/TimeField.test.tsx` (10 pruebas; comprueba además que no queda ningún `type="time"`,
  `datetime-local` ni `type="number"` en `desktop/src`).
- **Navegación**: `navigation/navigation.test.tsx` (barra para administrador, cajero y mesero, indicador de Caja en sus
  tres estados, estado del servidor, pantalla completa, ruta, mosaicos por permisos, inicio por terminal).
- **Teclado, menú y paleta**: `commands/commands.test.ts` y `commands/commandCenter.test.tsx`; Rust `menu::tests` (6).
- **Playwright** (Chromium y WebKit): `workflow-stage9c-navigation.spec.ts`, `workflow-stage9c-keyboard.spec.ts` y
  `workflow-stage9c-tables-time.spec.ts`, ordenadas después de las demás porque todas comparten la base de cada motor.
- **Etapa 9D**: `navigation/navigation.test.tsx` (orden de las cuatro secciones, Caja a la derecha antes del servidor, el
  usuario y pantalla completa, Caja en dos renglones en sus tres estados, Caja sin turno visible a la derecha, «Sin
  conexión»), `App.test.tsx` («Sin conexión» y recuperación), `src/stage9dLayout.test.ts` (columnas iguales, 52 px,
  colores de los indicadores por token, menú de Caja a la derecha, solo el nombre del usuario se acorta) y
  `commands/commandCenter.test.tsx` (Escape de «Acerca de» en cuanto aparece). Playwright
  `workflow-stage9d-topbar-tables.spec.ts` (orden, mismo ancho, alturas de 52–56 px y ningún texto cortado ni
  desbordamiento a 1280 × 720 y 1920 × 1080 en ambos temas, menú de Caja dentro de la ventana, «Sin conexión» simulado
  rechazando peticiones en el puente); `e2e/navigation.ts` busca Caja en toda la barra.
- **Etapa 9F** (fase D): `navigation/navigation.test.tsx` («barra compacta»: isotipo con su texto alternativo y sin la
  «A·», estado corto de Caja en sus tres estados con la ayuda emergente, «En línea» con texto y ayuda que empieza por el
  estado, el logotipo suelta el foco con el puntero y lo conserva con el teclado), `src/stage9fLayout.test.ts` (reglas de
  la compacta y de debajo de 1150 px, isotipo de 128 px, leyenda en dos renglones sin `overflow-x`) y Playwright
  `workflow-stage9f-topbar-map.spec.ts` (la barra a 1200, 1280, 1440, 1500 y 1920 px en ambos temas con un nombre de
  usuario largo: sin desplazamiento horizontal, botones visibles y tocables de al menos 44 px, isotipo, estado del turno
  y «En línea» legibles, nombre con puntos suspensivos; «Sin conexión» a 1200 px; el foco del logotipo tras un clic, con
  Escape, una flecha, ⌘2 y la paleta, y con el teclado; la leyenda sin desplazamiento horizontal en los cinco anchos, en
  dos renglones a 1200 px y en uno a 1920 px).

## Lo que falta comprobar

Pruebas físicas y de entorno que **no** se hicieron: la ventana nativa (WKWebView) con el menú real de macOS (si la
pulsación y el acelerador llegan dos veces, los elementos que macOS agrega por su cuenta en Edición y Ver, el menú en
pantalla completa); el menú en Windows (no se compiló); la pantalla táctil física (barra, mosaicos, selector y selector
de hora); el teclado virtual sobre el selector y sobre la paleta; un lector de pantalla real (VoiceOver). Detalle en
[VERIFICATION_STAGE9C.md](VERIFICATION_STAGE9C.md). De la 9D tampoco se comprobó en hardware: la barra nueva en la
ventana nativa y en una pantalla táctil física, el cursor «no disponible» en la ventana nativa y el arrastre del mapa con
el dedo ([VERIFICATION_STAGE9D.md](VERIFICATION_STAGE9D.md)).
