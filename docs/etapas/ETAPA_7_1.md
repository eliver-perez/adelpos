# AD-EL POS — ETAPA 7.1: AJUSTES DE OPERACIÓN, MESAS, CAJA Y MENSAJES

Continúa el desarrollo de **AD-EL POS** sobre la versión **0.7.0** (esquema 8). Trabaja en una rama nueva **`etapa-7.1`** creada desde `main`. No hagas push ni merge.

Esta entrega reúne lo que el propietario encontró al probar las etapas 6.2 y 7 en su Mac, más algunos pendientes acordados de entregas anteriores. Al terminar, la versión será **0.7.1**. Si necesitas una migración, que sea una sola (`0009_…sql`); antes de aplicarla a la instalación real, pide autorización.

---

## 0. FORMA DE TRABAJO

Lee `CLAUDE.md`, `AGENTS.md`, `docs/RESTAURANT_OPERATIONS.md`, `docs/PAYMENTS_AND_REFUNDS.md`, `docs/CASH_SHIFTS_AND_RECONCILIATION.md`, `docs/KITCHEN_AND_PRINTING.md` y los avances de las etapas 6.1, 6.2 y 7.

Este documento vive en el repositorio. Si la conversación se compacta, vuelve a leerlo.

| Fase | Contenido |
| ---- | --------- |
| **A** | Componentes compartidos: sistema de mensajes, formato de importes, plurales e iconos (§1). Van primero porque las demás fases los usan. |
| **B** | Mesas: cuentas vacías, estados, panel y fuera de servicio (§2). |
| **C** | Caja: atajos, tarjetas de acción, flujo tras registrar, cajón por CUPS y auditoría de rechazos (§3). |
| **D** | Pendientes acordados: motivos de descuento, total en dólares en el ticket e indicaciones rápidas para cocina (§4). |
| **E** | Regresiones completas, documentación y entrega (§5). |

**Al terminar cada fase:** ejecuta sus pruebas y las suites afectadas, aplica `./scripts/dev.sh format` y `format-check`, registra un resumen en `docs/ETAPA_7_1_AVANCE.md`, haz un commit local y **continúa sin esperar confirmación** si todo pasa.

**Detente y pregunta únicamente si:** una prueba falla y arreglarla exige cambiar una decisión de este documento; encuentras una contradicción que obligue a una decisión funcional importante; necesitas aplicar la migración a la instalación real; o una acción podría afectar datos o servicios fuera del alcance. Ante dudas menores, toma la opción más conservadora, anótala en el avance y continúa.

---

## 1. FASE A — COMPONENTES COMPARTIDOS

### 1.1 Sistema único de mensajes

Hoy algunas acciones muestran una alerta en la parte superior que el usuario no siempre ve, y otras no muestran nada. El usuario no sabe si la operación se hizo.

Crea un sistema único y úsalo en **toda la aplicación**:

- **Toasts** para confirmaciones de éxito («Ingreso registrado: $500.00», «Pedido enviado a cocina»):
  - Aparecen en una posición fija y visible, se cierran solos tras unos segundos y se pueden cerrar con un toque.
  - Texto legible en pantalla táctil.
  - Pueden incluir una acción («Imprimir comprobante», «Ver cuenta»).
  - No tapan los botones principales de cobro ni de captura.
- **Diálogo de confirmación** para acciones importantes o irreversibles (cerrar turno, cancelar productos, devoluciones, liberar mesa, descartar cambios):
  - Convierte en componente estándar el diálogo propio creado en la etapa 6.1 para «Volver al listado».
  - Botones grandes, la acción destructiva claramente diferenciada.
- **Errores**: claros, en español, sin códigos internos. Un error que exige acción del usuario no desaparece solo.

**Implementación:**

- Usa **Sonner** para los toasts, adaptado a los tokens visuales de AD-EL POS en tema claro y oscuro. Fíjalo en `package-lock.json` y regístralo en `docs/DEPENDENCIES.md`.
- Si Sonner tuviera un conflicto real con React 19, Tauri o el proyecto, constrúyelo con los componentes existentes y explica por qué.
- **No uses SweetAlert** ni librerías que impongan un estilo visual ajeno.
- Revisa primero si ya existe algún sistema de avisos: reemplázalo por completo. No deben convivir dos sistemas.
- Recorre las pantallas existentes (catálogo, mesas, captura, cobro, caja, cocina, usuarios, roles, configuración) y conecta sus éxitos y errores al nuevo sistema.
- Los mensajes de éxito no deben revelar información protegida; por ejemplo, ninguno debe mostrar el efectivo esperado a quien no tenga permiso.

### 1.2 Formato de importes

Los importes aparecen sin separador de miles: «$1000.00» debe ser **«$1,000.00»**.

- Usa una sola función de formato para pantalla e impresión, con separador de miles, dos decimales y el símbolo correspondiente (`$` para MXN, `US$` para USD).
- Aplícala en toda la aplicación, incluidos tickets, precuentas, comprobantes y cortes impresos.
- Los importes siguen llegando de Go como cadenas decimales. El formato es solo de presentación; no conviertas a `number` para calcular.

### 1.3 Plurales

Corrige la concordancia en toda la aplicación: «1 cuentas» → «1 cuenta», «1 operaciones de efectivo» → «1 operación de efectivo», etc. Usa una utilidad común y revisa los textos con contadores.

### 1.4 Iconos en botones

Agrega iconos a la izquierda del texto en los botones de acción de las pantallas operativas: mesas, captura, cobro, caja y para llevar (Capturar, Cobrar, Precuenta, Dividir, Más, Nueva cuenta, Abrir cuenta, Enviar a cocina, Imprimir, etc.).

- Revisa qué iconos usa ya el proyecto y continúa con el mismo conjunto. Si no hay uno, elige una librería de iconos ligera, sin dependencias de red, y regístrala.
- Iconos con trazo uniforme, mismo tamaño, `aria-hidden` cuando el botón ya tiene texto.

---

## 2. FASE B — MESAS

### 2.1 No crear cuentas vacías

Hoy, «Abrir cuenta y capturar» crea la cuenta de inmediato. Si el usuario sale sin capturar nada, la mesa queda ocupada con una cuenta vacía.

- «Abrir cuenta y capturar» entra a la captura de la mesa **sin crear todavía la cuenta**.
- La cuenta se crea **en la misma operación en que se guarda el primer producto** del borrador. Debe ser idempotente.
- Si el usuario sale sin capturar, la mesa sigue libre y no queda ningún registro.
- **Dos terminales en la misma mesa libre:** si al guardar el primer producto la mesa ya tiene una cuenta creada por otra terminal, no crees una segunda cuenta en silencio. Muestra: «Esta mesa ya tiene una cuenta» con dos opciones: agregar los productos a esa cuenta o abrir una cuenta nueva en la mesa.
- Lo mismo aplica a «+ Nueva cuenta» en una mesa ocupada.

### 2.2 Liberar cuentas vacías

Para las cuentas vacías que ya existen (la instalación real tiene varias de las pruebas):

- Botón **«Liberar mesa»** en el panel de la mesa, sin motivo ni autorización adicional.
- Solo aparece y solo lo acepta Go si la cuenta **no tiene productos, borradores, pagos, ajustes, descuentos ni nada enviado a cocina**.
- Queda auditado.
- Una cuenta con cualquier contenido sigue el flujo normal de cancelación con sus permisos.

### 2.3 Panel de mesa libre

El panel de una mesa libre debe tener la **misma estructura** que el de una mesa ocupada: encabezado con la mesa y su estado, área de cuenta vacía, totales en $0.00 y el botón grande **«Capturar»** en la misma posición que en una mesa ocupada. Las acciones que no aplican (Cobrar, Precuenta, Dividir) se muestran desactivadas o se ocultan de forma consistente.

### 2.4 Estados de mesa

El mapa muestra el estado de cada mesa:

| Estado | Condición | Color de referencia | Indicador adicional |
| --- | --- | --- | --- |
| **Libre** | Sin cuentas | Fondo blanco o neutro, borde gris claro | «Libre» |
| **Ocupada** | Al menos una cuenta con productos, sin precuenta impresa | Verde de la marca (tono suave de fondo, borde verde) | Tiempo ocupada |
| **Esperando pago** | Se imprimió la precuenta y queda saldo | Ámbar | Icono de recibo, «Precuenta» |
| **Pago parcial** | Tiene abonos y queda saldo | Azul | «Saldo $X» |
| **Fuera de servicio** | Deshabilitada | Gris tenue con patrón o menor opacidad | Icono de candado |

- Los colores son una propuesta inicial; el propietario podrá ajustarlos después. Defínelos como tokens del tema, con buen contraste en claro y oscuro, y que se distingan también por luminosidad.
- **Cada estado se distingue además por texto o icono**, no solo por color.
- Con varias cuentas en una mesa, se muestra el estado más avanzado (Esperando pago > Pago parcial > Ocupada).
- Go calcula el estado con los datos reales (precuenta, pagos, saldo). Si no se persiste hoy que una precuenta fue impresa, agrégalo en la migración. Se actualiza en tiempo real por SSE.
- Muestra en la mesa el **tiempo que lleva ocupada** («45 min»).
- La leyenda del mapa muestra los cinco estados.

### 2.5 Fuera de servicio desde la operación

- En el panel de una mesa **libre**: «Poner fuera de servicio». En una mesa fuera de servicio: «Habilitar».
- Requiere permiso (reutiliza el de administrar mesas si ya existe uno adecuado; si no, crea uno nuevo concedido solo al administrador) y queda auditado.
- Una mesa con cuenta abierta no se puede poner fuera de servicio.

---

## 3. FASE C — CAJA

### 3.1 Atajo de caja en las pantallas operativas

En el sistema anterior, las opciones de turno estaban en una barra inferior del punto de venta. Para mantener la pantalla de Restaurante enfocada en pedidos:

- Agrega un botón compacto **«Caja»** en la barra superior de las pantallas operativas (Restaurante, Para llevar, captura, cobro).
- Muestra el estado del turno de la caja de la terminal: «Turno abierto», «En arqueo» o «Sin turno».
- Al tocarlo, abre un menú rápido con: **Ingreso**, **Retiro**, **Gasto**, **Abrir cajón**, **Corte parcial** e **Ir a Caja**. Cada opción aparece solo con su permiso y abre el flujo existente de la etapa 7, sin duplicarlo.
- **Aviso sin turno:** si no hay turno abierto, la captura de pedidos sigue permitida (las cuentas son del restaurante, no de la caja), pero muestra un aviso discreto: «No hay turno abierto: puedes capturar, pero no cobrar». Con permiso, ofrece «Abrir turno».

El menú principal de la aplicación **no se rediseña en esta entrega**; el propietario lo decidirá después.

### 3.2 Tarjetas de acción en Caja

Los botones de Operación (Ingreso, Retiro de efectivo, Gasto, Corte parcial, Arqueo y cierre, Corrección o regularización) se ven planos.

- Conviértelos en **tarjetas táctiles con icono grande** arriba del texto.
- Acento de color por tipo: entradas (verde), salidas (ámbar), informes (neutro), cierre (color distintivo). Mismo criterio de contraste y temas que §2.4.
- La tarjeta seleccionada se resalta con borde y fondo, sin casillas.

### 3.3 Flujo tras registrar

Después de registrar con éxito un ingreso, retiro o gasto:

- El panel se cierra solo y vuelve a la vista de Operación.
- Un toast confirma la operación con su importe y concepto, con la acción **«Imprimir comprobante»** si no se imprimió.
- Los datos del turno (contadores, esperado para quien tenga permiso) se actualizan.

Aplica el mismo criterio al corte parcial y a la apertura de turno.

### 3.4 Cajón de dinero por CUPS (modo mixto)

Hoy el cajón solo funciona con impresoras ESC/POS por red y se rechaza con CUPS. El establecimiento usa impresoras de red, pero otros clientes las conectarán por USB. El cajón se conecta a la impresora, no a la computadora.

- El cajón se configura indicando **a qué impresora está conectado**: ESC/POS por red (como hoy) o una **cola de CUPS** (USB o cualquier otra conexión).
- Para CUPS, el pulso ESC/POS se envía en **modo raw** a esa cola, con argumentos separados (nunca un comando de shell construido con datos del cliente).
- Si la cola no admite datos raw, la configuración lo detecta y lo explica en lugar de fallar en silencio.
- El envío queda detrás de la misma abstracción de transporte, para que en Windows se implemente con su sistema de impresión sin cambiar la lógica.
- Conserva las reglas de la etapa 7: solo con efectivo físico, sin reintentos automáticos de un resultado incierto, permiso para apertura manual, auditoría y el mensaje «Comando de apertura enviado».
- Agrega **«Probar apertura»** en la configuración del cajón, con permiso y auditado.
- Prueba con el adaptador controlado. La prueba física queda pendiente hasta que el propietario tenga una impresora USB con cajón.

### 3.5 Auditoría de rechazos por efectivo insuficiente

La etapa 7 documentó que alguien que autoriza salidas sin ver el esperado podría deducirlo probando importes. Para que un administrador pueda detectarlo:

- Cada rechazo de retiro o gasto por efectivo insuficiente queda **auditado** con usuario, terminal, caja, turno, importe intentado y hora.
- Agrega un filtro en Auditoría o en Caja → Movimientos (para quien tenga permiso de consultar importes esperados) que muestre estos intentos.
- No cambia el comportamiento hacia el cajero: el rechazo sigue sin mostrar cifras.

---

## 4. FASE D — PENDIENTES ACORDADOS

### 4.1 Motivos de descuento

Basado en los tipos de descuento del sistema anterior (Atención, Promoción, Demora).

- Catálogo configurable de **motivos de descuento**, sembrado con: Atención, Promoción, Demora, Cortesía de la casa y Otro.
- Al aplicar un descuento o una cortesía, se elige el motivo con botones. «Otro» pide una descripción.
- El motivo se guarda con el descuento y aparece en el historial de la cuenta, en el ticket cuando corresponda y en los datos que usarán los reportes de la etapa 9.
- Administrar el catálogo requiere permiso y queda auditado. Desactivar un motivo no altera los descuentos ya registrados.

### 4.2 Total en dólares en el ticket

Con la función de dólares activa, el ticket y la precuenta muestran también el **total equivalente en dólares** con el tipo de cambio del turno, aunque el pago sea en pesos. Es informativo y no altera ningún importe. Con la función desactivada, no aparece.

### 4.3 Indicaciones rápidas para cocina

La captura de la etapa 6.1 ofrece «Escribir otra» y las indicaciones ya usadas en la cuenta. Completa la propuesta registrada en `docs/ETAPA_6_1_AVANCE.md`:

- Indicaciones rápidas configurables **por categoría** (por ejemplo, Hamburguesas: «Sin cebolla», «Bien cocida»; Bebidas: «Sin hielo»).
- En la captura aparecen como botones al personalizar un producto de esa categoría, junto a «Escribir otra».
- Llegan a la comanda de cocina igual que las indicaciones escritas.
- Administrarlas requiere permiso de catálogo.

---

## 5. PRUEBAS, DOCUMENTACIÓN Y ENTREGA

**Pruebas:**

- Go: cuenta creada con el primer producto; salida sin capturar sin dejar registro; dos terminales en la misma mesa libre; liberar mesa vacía permitido y con contenido rechazado; estados de mesa calculados; fuera de servicio con y sin cuenta; cajón por CUPS raw con adaptador; auditoría de rechazos por efectivo insuficiente; motivos de descuento; indicaciones por categoría.
- Vitest: toasts y diálogo de confirmación; formato de importes con miles y monedas; plurales; tarjetas de Caja; cierre del panel tras registrar.
- Playwright (Chromium y WebKit): mesa libre → capturar → salir sin productos → mesa sigue libre; mesa libre → producto → enviar; liberar mesa vacía; precuenta → estado «Esperando pago»; ingreso desde el atajo «Caja» con toast de confirmación.
- Repite todas las suites de etapas anteriores.

**No declares superada ninguna prueba que no hayas ejecutado.**

**Documentación:** actualiza `CLAUDE.md` (estado, versión, sistema de mensajes, formato, estados de mesa, cajón por CUPS), `API_CONTRACTS.md`, `docs/RESTAURANT_OPERATIONS.md`, `docs/CASH_SHIFTS_AND_RECONCILIATION.md`, `docs/KITCHEN_AND_PRINTING.md`, `docs/DEPENDENCIES.md` y `docs/VERIFICATION.md`, solo con lo realmente ejecutado.

**Entrega final breve:** cambios por fase, migración (si hubo), pruebas ejecutadas, pendientes y procedimiento para aplicar la migración a la instalación real en espera de autorización.

No cambies la arquitectura, no rediseñes el menú principal, no adelantes funciones de la etapa 8 y conserva todo lo aprobado de las etapas 1 a 7.
