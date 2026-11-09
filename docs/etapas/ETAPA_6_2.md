# AD-EL POS — ETAPA 6.2: FLUJO RÁPIDO DE MESAS Y PARA LLEVAR

Continúa el desarrollo de **AD-EL POS** sobre la versión **0.6.1** (esquema 7). Trabaja en una rama nueva **`etapa-6.2`** creada desde `main`. No hagas push ni merge.

Al terminar, la versión será **0.6.2**. Esta entrega **no debería necesitar migraciones**; si alguna resulta imprescindible, explica por qué y pide autorización antes de aplicarla a la instalación real.

---

## 0. CONTEXTO

Lee `CLAUDE.md`, `AGENTS.md`, `docs/RESTAURANT_OPERATIONS.md` y `docs/ETAPA_6_1_AVANCE.md`.

La nueva pantalla de captura de la etapa 6.1 funciona bien. El problema está **antes** de llegar a ella: abrir una cuenta exige demasiados pasos.

Hoy, en Restaurante, el usuario toca la mesa en el diagrama, elige «Abrir cuenta» y llena nombre, lista de precios y observaciones **antes** de poder capturar. En la operación real:

- Las cuentas de mesa casi nunca llevan nombre.
- El nombre y las observaciones se capturan **durante** la orden, si hacen falta.
- En Para llevar, el nombre del cliente sí es habitual, pero también se captura durante la orden.

Consulta en el sistema anterior (`CLAUDE.md` §21) `PuntoDeVenta.cs` y `PuntoDeVenta.Designer.cs`: el mapa de mesas tenía un **panel a la derecha** que mostraba la orden activa de la mesa seleccionada, con sus productos y totales, y botones para Capturar orden, Modificar, Pagar, Cancelar, Imprimir y Dividir cuenta. Desde la mesa se operaba todo sin abrir formularios.

Úsalo como referencia funcional, no como código.

---

## 1. MESAS

### 1.1 Mapa con panel lateral

- Al tocar una mesa en el mapa, se abre un **panel a la derecha** (sin ventana emergente) con:
  - Nombre de la mesa, estado y tiempo abierta.
  - Las cuentas abiertas de esa mesa como botones, si hay más de una.
  - Para la cuenta seleccionada: sus productos (con estado «En cocina» / «Nuevo»), totales, pagado y saldo.
  - Botones grandes: **Capturar** (abre la captura de esa cuenta), **Cobrar**, **Precuenta**, **Dividir**, **Más…** (cancelar, transferir, combinar y demás acciones existentes, con sus permisos).
- El mapa sigue visible y usable mientras el panel está abierto. Tocar otra mesa cambia el panel.
- El panel se actualiza en tiempo real con los mecanismos existentes.

### 1.2 Abrir cuenta sin formulario

- **Mesa libre:** tocarla muestra en el panel un botón **«Abrir cuenta y capturar»**. Un toque abre la cuenta y entra directo a la captura. Sin pedir nombre, lista de precios ni observaciones.
- **Mesa ocupada:** el panel muestra su cuenta; «Capturar» entra a ella. «+ Nueva cuenta» abre otra cuenta en la misma mesa y entra a capturar.
- Si el establecimiento tiene varias listas de precios, la cuenta usa la **predeterminada** que ya define el sistema. Cambiarla sigue siendo posible desde la captura, con su permiso.
- La apertura sigue siendo idempotente: un doble toque no abre dos cuentas.

### 1.3 Datos de la cuenta durante la captura

En la barra superior de la pantalla de captura, un botón **«Datos de la cuenta»** (o tocar el nombre de la cuenta) abre un panel lateral para capturar o cambiar, en cualquier momento:

- Nombre de la cuenta (opcional).
- Número de comensales: botones rápidos 1 a 8 y «Más». Opcional. Si la cuenta ya guarda comensales, úsalo; si no, propón cómo guardarlo sin migración o repórtalo como pendiente. Sustituye la capacidad de la mesa que hoy se muestra en «personas».
- Observaciones.
- Lista de precios, con permiso.

Usa las operaciones existentes de edición de cuenta, con su idempotencia, revisión y auditoría. Recuerda que la etapa 6.1 corrigió que editar datos de la cuenta borrara la hora reprogramada en cocina (M3): no lo reintroduzcas.

---

## 2. PARA LLEVAR

- **«Nuevo pedido» entra directo a la pantalla de captura**, sin formulario previo.
- En la barra superior de la captura, **«Datos del pedido»** abre el mismo panel lateral con: nombre del cliente, teléfono, hora programada (si aplica), origen (para llevar / ventanilla) y observaciones.
- **El nombre del cliente se sugiere pero no bloquea**: si el pedido se envía a cocina sin nombre, la captura muestra un aviso discreto («Pedido sin nombre») y usa el número de pedido como identificación. No obligar.
- Si alguno de esos datos es hoy obligatorio en Go para enviar a cocina o para programar, mantén la validación solo donde sea indispensable (por ejemplo, la hora en un pedido programado) y repórtalo.
- El tablero de Para llevar conserva su funcionamiento actual.

---

## 3. REGLAS

- Reutiliza las operaciones y contratos existentes. No crees un segundo flujo de apertura de cuentas.
- Objetivos táctiles de al menos 44 px y los tamaños de la etapa 6.1 en los botones principales. Temas claro y oscuro. Layout fluido.
- Conserva la pantalla «Áreas y distribución» para administrar mesas; el mapa operativo y el de administración pueden compartir el dibujo, pero el operativo no debe mostrar controles de edición del mapa.

---

## 4. PRUEBAS Y ENTREGA

- Vitest y Playwright (Chromium y WebKit): mesa libre → «Abrir cuenta y capturar» → producto → enviar a cocina; mesa ocupada → panel lateral → capturar y cobrar; nueva cuenta en mesa ocupada; doble toque sin cuentas duplicadas; para llevar → captura directa → datos del pedido → enviar sin nombre.
- Go: si cambia alguna validación de apertura o envío, pruebas de integración.
- Repite las suites de etapas anteriores.
- Actualiza `CLAUDE.md`, `docs/RESTAURANT_OPERATIONS.md` y `docs/VERIFICATION.md` solo con lo realmente ejecutado.
- Commits locales en `etapa-6.2`, sin push. Entrega final breve: cambios, pruebas, pendientes.

**No declares superada ninguna prueba que no hayas ejecutado.**
