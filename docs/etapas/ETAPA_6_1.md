# AD-EL POS — ETAPA 6.1: CORRECCIONES, FUNCIONES ACTIVABLES Y CAPTURA TÁCTIL

Continúa el desarrollo de **AD-EL POS** sobre la versión **0.6.0** (esquema 6), con la etapa 6 ya aplicada a la instalación real.

Esta entrega reúne lo que el propietario encontró al probar la aplicación en su Mac. Incluye correcciones de errores, mejoras de formularios, dos cambios funcionales (IVA por producto y pago en dólares como función activable) y el **rediseño de la pantalla de captura de pedidos para uso táctil**.

Al terminar, la versión será **0.6.1**, con el esquema que resulte de las migraciones nuevas.

---

## 0. ANTES DE EMPEZAR

### Contexto obligatorio

Lee `CLAUDE.md`, `AGENTS.md`, `docs/PAYMENTS_AND_REFUNDS.md`, `docs/CATALOG.md`, `docs/RESTAURANT_OPERATIONS.md` y `docs/ETAPA_6_AVANCE.md`.

Lee también `CLAUDE.md` §21 sobre el sistema anterior en C#. Para esta entrega consulta en él especialmente:

- `CapturaOrden.cs`, `CapturaOrden.Designer.cs`, `Seleccionar_Producto.cs` y `ArticuloLista.cs` (pantalla de captura).
- `PagarEfectivo.cs` y `PagoDivisa.cs` (pago en pesos y dólares).

Úsalo como referencia funcional, nunca como código a copiar.

Este documento vive en el repositorio. Si la conversación se compacta, vuelve a leerlo antes de continuar.

### Trabajo por fases

| Fase | Contenido |
| ---- | --------- |
| **A** | Errores (§1). |
| **B** | Mejoras de formularios y configuración (§2). |
| **C** | Funciones activables, IVA por producto y pago en dólares, con su migración (§3). |
| **D** | Rediseño de la captura de pedidos (§4). |
| **E** | Regresiones completas, documentación y entrega final (§5 y §6). |

**Al terminar cada fase:**

1. Ejecuta las pruebas de la fase y las suites anteriores que puedan verse afectadas.
2. Aplica `./scripts/dev.sh format` y verifica `format-check`.
3. Registra un resumen breve en `docs/ETAPA_6_1_AVANCE.md`.
4. Haz un **commit local** de la fase, sin push.
5. **Si la fase quedó funcional y sus pruebas pasan, continúa con la siguiente sin esperar confirmación.**

**Detente y pregunta únicamente si:**

- Una prueba falla y arreglarla exige cambiar una decisión de este documento.
- Encuentras una contradicción que obligue a tomar una decisión funcional importante.
- Necesitas aplicar una migración a la instalación real.
- Una acción podría afectar datos, servicios o archivos fuera del alcance.

Ante dudas menores, toma la opción más conservadora y coherente con el proyecto, anótala en `docs/ETAPA_6_1_AVANCE.md` y continúa.

---

## 1. FASE A — ERRORES

Para cada error: identifica la causa raíz, corrígela en el componente compartido cuando el problema venga de ahí (no parches por pantalla) y agrega una prueba que falle con el código actual.

### 1.1 Mesas redondas se dibujan ovaladas

En Restaurante → Áreas y distribución, una mesa redonda de 100 × 100 se dibuja como óvalo; el propietario tuvo que poner 100 × 180 para que se viera circular.

- **Ancho y alto iguales deben producir un círculo** (o un cuadrado, en mesas cuadradas).
- Probablemente el mapa escala el eje horizontal y el vertical con factores distintos. El escalado debe ser **uniforme** y conservar la proporción en cualquier tamaño de ventana.
- Revisa si el mismo defecto deforma mesas cuadradas y rectangulares.
- No migres ni alteres automáticamente las medidas ya guardadas. Si después de la corrección una mesa guardada como 100 × 180 se ve ovalada, es correcto: es lo que el usuario capturó.

### 1.2 «Volver al listado» a veces no responde

Ocurre al capturar un registro nuevo en **Categorías, Productos y Combos**. Encuentra en qué condiciones falla (formulario con cambios, validaciones, estado del acordeón, etc.) y corrígelo para que siempre funcione. Si hay cambios sin guardar, debe preguntar si se descartan, no quedarse sin respuesta.

### 1.3 Campos numéricos que aceptan texto

El «Precio base anunciado» acepta letras y solo las rechaza al guardar.

- **Ningún campo de importe, precio, cantidad o medida debe aceptar caracteres no válidos al teclear.** Importes: dígitos y un solo punto decimal, con la precisión del sistema. Cantidades y medidas enteras: solo dígitos.
- Aplica la regla en **todos los formularios** (catálogo, mesas, roles, cobros, configuración), no solo en el precio base.
- Sigue la regla ya aprobada: campos de texto con `inputMode="decimal"` o `inputMode="numeric"`, **nunca `type="number"`** (hallazgo M2). Revisa `DecimalField` y crea su equivalente entero si no existe.
- La validación final sigue en Go.

### 1.4 Combos: «Productos permitidos» obligatorio

Al registrar un componente de combo, el sistema obliga a agregar una fila en «Productos permitidos y sustituciones» aunque no se ofrezcan sustituciones.

- **El producto original del componente siempre está permitido de forma implícita.** «Productos permitidos y sustituciones» es opcional y solo sirve para agregar alternativas.
- Revisa la validación en Go y en la interfaz. Conserva el comportamiento de los combos ya guardados.

### 1.5 Selectores con un producto preseleccionado

En el alta de combos, «Producto original» y «Producto permitido» aparecen con el primer producto de la lista ya elegido; en la prueba quedó elegido el mismo combo que se estaba creando.

- Los selectores de producto deben **iniciar vacíos** y exigir una elección explícita.
- **Un combo no puede contenerse a sí mismo.** Excluye el combo actual de las opciones de sus componentes y valida en Go. Si el modelo no admite combos dentro de combos, exclúyelos también y valídalo.
- Revisa el mismo patrón en todos los selectores del catálogo.

### 1.6 Fila vacía en «Tramos incrementales»

Al agregar un tramo aparece una fila que solo contiene el botón «Quitar fila», sin campos para capturar. Corrige la fila para que muestre sus campos.

### 1.7 Selector de archivo en inglés

El campo de imagen muestra «Choose File / no file selected», texto nativo del navegador. Reemplázalo por un control propio en español («Elegir imagen», «Sin imagen», «Quitar imagen»), con vista previa, que funcione con toque.

### 1.8 Restricciones de horario con fechas prellenadas

El acordeón de horarios muestra «Desde» y «Hasta» con la fecha de hoy aunque el texto dice «Sin restricciones: disponible siempre». Confunde: no se sabe si el registro está restringido.

- Sin restricciones, **los campos deben estar vacíos**.
- Verifica qué se guarda hoy: un registro sin restricciones no debe quedar limitado a la fecha de captura. Si encuentras registros guardados así por este defecto, repórtalo; no los modifiques sin autorización.

---

## 2. FASE B — MEJORAS DE FORMULARIOS Y CONFIGURACIÓN

### 2.1 Alta continua de mesas

Hoy, después de crear una mesa, el formulario se queda editando esa mesa y hay que subir hasta arriba para pulsar «Crear mesa».

- Agrega **«Guardar y crear otra»**: guarda y deja el formulario limpio para la siguiente, conservando el área seleccionada y proponiendo el siguiente nombre (Mesa 5 después de Mesa 4).
- La acción principal debe estar siempre visible, sin desplazarse hasta arriba.
- Aplica el mismo criterio en los catálogos donde se capturan muchos registros seguidos (categorías, productos, ingredientes, complementos), si su estructura lo permite.

### 2.2 Orden visual con posiciones en lugar de números

En lugar de capturar un número de orden, el usuario elige la posición:

- «Al inicio».
- «Después de *Desayunos*», «Después de *Bebidas*», etc., con los registros existentes en su orden actual.
- Al editar, la opción actual aparece seleccionada.

Internamente puede seguir guardándose un número; la interfaz y Go se encargan de reacomodar. Aplícalo en **todos los catálogos que tienen orden visual**. La operación de reordenar debe ser transaccional y concurrente-segura.

### 2.3 Color con paleta

El color se captura como hexadecimal (#RRGGBB) y la mayoría de los usuarios no sabe qué es.

- El campo **no acepta escritura**. Al tocarlo se abre una **paleta** de unos 16 colores bien diferenciados, legibles en tema claro y oscuro.
- **Sin color** es el valor predeterminado, con un botón para **quitar el color**.
- Go valida que el valor pertenezca a la paleta o sea vacío. Los colores ya guardados que no estén en la paleta se conservan y se muestran como «Color personalizado».

El color de categoría se usará en la nueva pantalla de captura (§4).

### 2.4 «Restricciones de horario»

- Renombra el acordeón «Fechas, días y horarios» a **«Restricciones de horario»** en todos los catálogos donde aparece.
- Su resumen cerrado debe decir el estado: «Disponible siempre» o un resumen breve («Lun a vie, 7:00–12:00»).
- Reescribe el texto de ayuda en lenguaje sencillo. Explica «fin exclusivo» y el cruce de medianoche con un ejemplo, o simplifica la captura para que no haga falta explicarlo.

### 2.5 Código interno sugerido

Al escribir el nombre de un producto, el código interno se llena con una sugerencia derivada del nombre (por ejemplo «Machacado Natural» → `machacado-natural`), siguiendo las reglas de formato que ya valida Go.

- La sugerencia se actualiza mientras el usuario no haya editado el código manualmente; después, no se toca.
- Si el código sugerido ya existe, propón uno libre (`machacado-natural-2`).
- Al editar un producto existente no se cambia su código.

### 2.6 «Probar personalización»

Es una herramienta de prueba visible en el menú principal. Quítala del menú principal y déjala dentro de Catálogo como «Vista previa», solo para usuarios con permiso de administrar catálogo.

### 2.7 Configuración general

- **Zona horaria:** selector con las zonas IANA, con las de México primero (America/Monterrey, America/Mexico_City, America/Tijuana, America/Hermosillo, America/Chihuahua, America/Matamoros, America/Cancun, etc.) y búsqueda. Sin escritura libre. Go valida que la zona exista.
- **Dirección:** el campo crece hasta un alto máximo razonable y después muestra desplazamiento interno. No puede crecer sin límite.
- **Moneda:** el sistema solo admite MXN. Muéstrala como dato de solo lectura, no como campo editable.

---

## 3. FASE C — FUNCIONES ACTIVABLES, IVA POR PRODUCTO Y DÓLARES

Una sola migración nueva (`0007_…sql`) o las que resulten necesarias. No modifiques 0001 a 0006. Antes de aplicarla a la instalación real, pide autorización.

### 3.1 Patrón: funciones activables del establecimiento

AD-EL POS es un producto comercial para restaurantes distintos. Algunas funciones solo le interesan a una parte de ellos. Crea un patrón reutilizable:

- Una sección **«Funciones del establecimiento»** en Configuración, con interruptores (botones seleccionables, no casillas a la derecha) y una explicación breve de cada función.
- Las funciones se guardan en PostgreSQL. Cambiarlas exige un permiso nuevo, concedido solo al administrador en la migración, y queda auditado.
- **Go hace cumplir el estado de cada función:** rechaza las operaciones de una función desactivada aunque alguien llame a la API directamente. La interfaz oculta lo que no aplica.
- Desactivar una función **nunca borra ni altera** los registros históricos que generó.
- Documenta cómo agregar una función nueva al patrón. En la etapa 10 se podrán ligar a la licencia; no implementes nada de licencias ahora.

La primera función del patrón es el pago en dólares (§3.3).

### 3.2 IVA por producto

Decisión del propietario: además de la tasa general del establecimiento, cada producto puede tener su propia tasa.

- En el producto (y en el combo, que es un producto), un selector con: **«Tasa general del establecimiento»** (predeterminado), **16 %**, **8 %** y **0 %**. Sin captura libre, igual que la tasa general.
- Go y PostgreSQL validan la lista cerrada. `NULL` (o el valor que elijas) significa «hereda la tasa general».
- Al confirmar un pedido, cada unidad guarda la **tasa efectiva** (la del producto o, si hereda, la general vigente). Cambiar la tasa del producto o la general solo afecta lo que se confirme después.
- Un combo usa la tasa del combo para todo su importe.
- El desglose por tasa de la etapa 6 (ticket, precuenta, devoluciones) debe agrupar correctamente cuando una cuenta mezcla tasas: una línea de base e IVA por cada tasa presente, y base + IVA = total exacto en cada grupo.
- Cambiar la tasa de un producto requiere permiso de catálogo y queda auditado.

### 3.3 Pago en dólares (función activable)

Basado en el sistema anterior (`PagarEfectivo.cs`), usado a diario en un restaurante de frontera. **Desactivado por defecto.**

**Con la función desactivada,** no aparece nada relacionado con dólares: ni equivalencias, ni campos, ni tipo de cambio al abrir turno. El cobro queda exactamente como en la etapa 6.

**Con la función activada:**

**Tipo de cambio**

- Lo captura un usuario con permiso **al abrir el turno**. Queda fijo durante el turno.
- Validación en Go: solo números con la precisión definida, dentro de un rango razonable configurable (por ejemplo, 10 a 40 MXN por USD), y una confirmación adicional si difiere mucho del tipo del turno anterior. Esto evita errores como 1.95 en lugar de 19.50.
- Si la función se activa con un turno ya abierto, el turno no acepta dólares hasta que alguien con permiso capture el tipo de cambio para ese turno. Auditado.

**Cobro**

- Solo en **efectivo**. Tarjeta y transferencia siguen en pesos.
- La pantalla muestra el saldo en pesos y su **equivalente en dólares**.
- En una misma operación se pueden capturar **pesos recibidos y dólares recibidos** (por ejemplo, 20 USD más 100 MXN).
- El cambio se muestra **en pesos y en dólares** para que el cajero informe al cliente.
- Un selector **«Cambio entregado en: Pesos / Dólares»**, con **Pesos** por defecto, registra la moneda en que salió el cambio realmente.
- Todos los cálculos y conversiones en Go con `decimal`. Define y documenta el redondeo de las conversiones.

**Registro**

- Cada pago guarda: pesos recibidos, dólares recibidos, tipo de cambio aplicado, importe aplicado a la cuenta en pesos, cambio y moneda del cambio.
- Los movimientos del turno distinguen **pesos físicos y dólares físicos** que entraron y salieron, para que el corte de la etapa 7 cuente cada moneda por separado.
- El ticket muestra, cuando aplica, los dólares recibidos y el tipo de cambio.
- Las **devoluciones** se registran en pesos. Si una devolución sale en dólares físicos, debe registrarse como tal en los movimientos del turno; si eso complica el modelo, limita las devoluciones a pesos y documéntalo.
- Las correcciones de pago de la etapa 6 deben seguir funcionando con pagos en dólares.

---

## 4. FASE D — REDISEÑO DE LA CAPTURA DE PEDIDOS

### 4.1 Problema

La pantalla actual de captura no es práctica en pantalla táctil:

- La cuenta y las acciones se eligen en listas desplegables («Acciones de cuenta…»).
- No hay una cuadrícula de productos para tocar.
- La personalización ocupa la pantalla con campos de formulario y selectores.

### 4.2 Referencia

El sistema anterior (`CapturaOrden`) resolvía bien el flujo: categorías a la izquierda, productos al centro como botones grandes, la cuenta a la derecha, un toque para agregar y la personalización en el mismo panel central con botones. El propietario aprobó una propuesta visual basada en ese flujo; está descrita abajo.

### 4.3 Distribución aprobada

Tres columnas que aprovechan toda la pantalla (fluido, sin limitarse a 1280 × 720, funcional desde 1280 × 720):

**Barra superior**

- Botón «Mesas» para volver al mapa.
- Mesa y cuenta actual, en grande («Mesa 1 · Cuenta #1»), con mesero, personas y hora de apertura.
- Las demás cuentas de la mesa como **botones**, más «+ Nueva cuenta». No usar lista desplegable.

**Columna izquierda: categorías**

- Botones grandes (alto mínimo 64 px), uno por categoría, con el **color de la categoría** (§2.3) como marca, nombre y número de productos.
- La categoría activa resaltada con borde en su color.
- Respeta el orden visual de las categorías y solo muestra las disponibles por horario.

**Columna central: productos**

- Buscador por nombre o código, arriba.
- **Cuadrícula de productos** como botones grandes (alto mínimo de unos 110 px), con franja del color de su categoría, nombre, precio y, si tiene imagen, la imagen (configurable si ocupa demasiado).
- **Un toque agrega el producto a la cuenta** cuando no requiere elecciones.
- Los productos con elecciones obligatorias muestran una etiqueta «Opciones» y, al tocarlos, la **personalización se abre en el mismo panel central**, sin ventanas emergentes.

**Personalización en el panel central**

- Botón «Volver», nombre del producto y explicación breve.
- Cada grupo de complementos o componente de combo como un bloque con **botones grandes** (alto mínimo 56 px) para cada opción; lo incluido se marca seleccionado; los cargos extra se muestran en el botón («Aros de cebolla +$10.00»).
- **Indicaciones para cocina** como botones rápidos («Sin cebolla», «Bien cocida»…), más «Escribir otra» con teclado. Los botones rápidos salen de las indicaciones ya configuradas en el catálogo; si no existe ese concepto, propón cómo configurarlas y repórtalo en el avance.
- Quitar ingredientes (el «pedido especial» del sistema anterior), si el catálogo lo permite.
- Cantidad con − y + grandes, y el botón principal «Agregar a la cuenta · $total».
- **Reutiliza la lógica, las reglas y la cotización de `ProductCustomizer` y del motor de Go.** Cambia la presentación, no las reglas. Go sigue siendo la autoridad de precios.

**Columna derecha: cuenta**

- Partidas con nombre, personalizaciones en texto pequeño, total y una etiqueta de estado: **«En cocina»** para lo confirmado y **«Nuevo»** para lo que está en borrador.
- Las partidas nuevas tienen − y + (mínimo 44 px). Las enviadas no se editan con esos botones: sus cambios usan los flujos existentes de modificación y cancelación, con sus permisos y las reglas de partidas con pago de la etapa 6.
- Totales: enviado a cocina, nuevo sin enviar y **total grande**, con la leyenda «Precios con IVA incluido».
- Acciones secundarias como **botones**: Dividir, Descuento, Precuenta, Más… (el menú «Más…» reúne las acciones menos frecuentes que hoy están en «Acciones de cuenta…»).
- Dos botones principales grandes:
  - **«Enviar a cocina»** con el número de productos nuevos; desactivado si no hay nada nuevo. Equivale a confirmar el pedido con las reglas actuales (borrador persistido, huella esperada, idempotencia).
  - **«Cobrar · $total»**, que abre la pantalla de cobro de la etapa 6.

### 4.4 Reglas

- **Reutiliza los contratos y operaciones existentes**: borradores persistidos, confirmación explícita, `expected_fingerprint`, `operation_id`, liberación a cocina, SSE. No crees un segundo flujo de pedidos.
- Agregar productos con toques rápidos no debe generar una petición por toque que sature el servidor ni perder toques. Define y documenta cómo se sincroniza el borrador.
- La misma pantalla se usa para **Para llevar**, con los datos del pedido (nombre, teléfono, hora programada) en la barra superior en lugar de la mesa.
- Objetivos táctiles: mínimo 44 px en general y los tamaños indicados arriba en los elementos principales. Sin depender de hover.
- Temas claro y oscuro con los tokens existentes. Respeta `Accordion`, `VirtualKeyboard` y los patrones aprobados.
- Conserva lo que hoy funciona: selección sin guardar recuperable, conflictos de revisión, actualización en tiempo real desde otras terminales.

---

## 5. PRUEBAS

- Una prueba que reproduzca cada error de §1 y falle con el código actual.
- Go: posiciones de orden, paleta de colores, zona horaria, funciones activables (incluida la API rechazando dólares con la función desactivada), IVA por producto (herencia, tasa efectiva por unidad, cuentas con tasas mixtas, desglose exacto por tasa) y pagos en dólares (conversión, pesos y dólares en la misma operación, cambio en ambas monedas, movimientos por moneda, tipo de cambio fuera de rango, idempotencia y concurrencia).
- Vitest: campos numéricos que rechazan caracteres inválidos, selector de color, selector de posición, código sugerido, alta continua de mesas y la nueva captura.
- Playwright (Chromium y WebKit): actualiza el recorrido operativo a la nueva captura: categoría → producto directo → producto con opciones → enviar a cocina → cobrar. Incluye un cobro en dólares con la función activada.
- Repite todas las suites de etapas anteriores.

**No declares superada ninguna prueba que no hayas ejecutado.**

---

## 6. DOCUMENTACIÓN Y ENTREGA FINAL

Actualiza `CLAUDE.md` (estado, versión, funciones activables, IVA por producto, dólares, nueva captura), `API_CONTRACTS.md`, `ARCHITECTURE.md`, `docs/CATALOG.md`, `docs/RESTAURANT_OPERATIONS.md`, `docs/PAYMENTS_AND_REFUNDS.md` y `docs/VERIFICATION.md` (archivando la anterior), solo con lo realmente ejecutado.

Entrega final, breve:

1. Errores corregidos y su causa raíz.
2. Mejoras implementadas.
3. Funciones activables, IVA por producto y pago en dólares.
4. Nueva pantalla de captura y cómo usarla.
5. Migraciones y estado del esquema.
6. Pruebas ejecutadas y resultados.
7. Pendientes, limitaciones y decisiones menores tomadas.
8. Procedimiento para aplicar la migración a la instalación real, en espera de autorización.

No cambies la arquitectura, no adelantes funciones de etapas posteriores y conserva todo lo aprobado de las etapas 1 a 6.
