# AD-EL POS — ETAPA 9F: REGRESIÓN EN MAC, IMPRESIÓN, COCINA POR ESTACIONES Y PENDIENTES

Esta etapa se ejecuta **en la Mac**. Parte de la versión **0.9.4** que dejó la 9E en `main`, compilada y probada en Windows pero todavía no en macOS. La Mac tiene instalada la 0.9.3, con esquema 21. Confirma la versión y el esquema reales en `config/product.json` y `CLAUDE.md`.

**Contenido:**

1. Regresión completa de la 0.9.4 en macOS.
2. Problemas encontrados al probar en Windows: datos de prueba, impresión, cola de impresión, corte de papel y textos.
3. Cocina por estaciones para usar una terminal como **pantalla de cocina básica (KDS)**.
4. Pendientes técnicos acumulados.

Al terminar, la versión será **0.9.5**.

**Todo cambio debe seguir funcionando en Windows.** Lo específico de cada sistema va detrás de condiciones por plataforma, como dejó la 9E.

---

## 0. FORMA DE TRABAJO

### Rama y especificaciones

- Haz `git pull` de `main` y trabaja en una rama nueva **`etapa-9f`**.
- Las especificaciones en `docs/etapas/` son de solo lectura. Lee este documento, `CLAUDE.md`, `NEXT_STEPS_FOR_SOL.md` y `docs/NOTAS_WINDOWS.md`.

### Eficiencia

- La Fase 0 la hace **un solo agente**.
- **Máximo 2 agentes en total trabajando al mismo tiempo. Los subagentes no pueden lanzar otros subagentes.**
- En cada fase ejecuta solo las pruebas relacionadas con lo que cambió. La regresión completa en la Fase 0-R y en la Fase F.

### Autonomía

Aplica la regla de autonomía de `CLAUDE.md` §0. Continúa de fase en fase sin esperar confirmación mientras las pruebas pasen. Aplica las migraciones con el procedimiento de respaldo. Instala en la Mac, fusiona en `main` y haz push al terminar. Detente solo en los casos que indica esa regla.

**Memoria de trabajo:** al iniciar, actualiza tu memoria para que coincida con la regla de autonomía de `CLAUDE.md` §0. Elimina las reglas anteriores («nunca push», «pedir autorización para cada migración»). Si no se te permite, dilo en la entrega.

### Fases

| Fase | Contenido |
| ---- | --------- |
| **0-R** | Regresión completa de la 0.9.4 en macOS e instalación (§1). |
| **0** | Verificación contra el código (§2). |
| **A** | Datos de prueba fuera de las instalaciones reales (§3). |
| **B** | Impresión: aviso sin impresora, reintento, cola y corte de papel (§4). |
| **C** | Cocina por estaciones y terminal de cocina (§5). |
| **D** | Ajustes de interfaz (§6). |
| **E** | Pendientes técnicos (§7). |
| **F** | Regresión completa, documentación, instalación, fusión en `main` y push. |

**Al terminar cada fase:** ejecuta sus pruebas, aplica el formato (sin tocar `docs/etapas/`), registra un resumen en `docs/ETAPA_9F_AVANCE.md`, haz un commit local y continúa.

---

## 1. FASE 0-R — REGRESIÓN DE LA 0.9.4 EN MACOS

- Ejecuta en la Mac todas las suites, como indica `CLAUDE.md`: Go con `-race`, la integración con PostgreSQL, Vitest, Rust, Playwright en Chromium y WebKit, y la compilación.
- Pon atención a lo que tocó la 9E, porque se cambió desde Windows:
  - Arranque y cierre de procesos.
  - Credenciales (Keychain).
  - Rutas.
  - Impresión.
  - Finales de línea (`.gitattributes`).
  - La corrección de atajos y Escape en diálogos.
- Corrige lo que falle sin romper Windows.
- Instala la 0.9.4 en la Mac con el procedimiento habitual. No debería haber migración, porque el esquema sigue en 21.

---

## 2. FASE 0 — VERIFICACIÓN

Registra en el avance:

1. **Datos de prueba en instalaciones reales.** En Windows, la prueba de humo de la 9E dejó destinos, estaciones o rutas de prueba en la instalación real: por ejemplo «Destino controlado E2E», al que se envían las precuentas y que nunca imprime. Revisa:
   - Qué pruebas crean datos y dónde los crean.
   - Si pueden escribir en la base de datos de una instalación real.
   - Si la base de datos de la Mac también tiene datos de prueba mezclados.
2. **Cómo se elige hoy a qué impresora va cada documento** (precuenta, ticket, comanda, avisos, cortes), y qué pasa cuando no hay ninguna configurada.
3. **Reintentos:** en qué casos se pide motivo para reintentar una impresión. La 9B acordó pedir motivo solo al reimprimir algo que ya salió en papel.
4. **Corte de papel:** si los documentos ya envían el comando de corte ESC/POS (`GS V`), y si hay alguna opción por impresora.
5. **Estaciones de cocina:**
   - Cómo se asigna un producto a una o varias estaciones.
   - Si existe un filtro de estaciones **por terminal** y **por usuario**. En Preparación aparece «Todas las estaciones autorizadas», lo que sugiere que hay autorización por usuario.
   - Si la pantalla de inicio por terminal de la 9C puede ser Cocina › Preparación.
   - Por qué en la prueba de Windows la misma orden de 4 filetes aparece en Freidoras y en Cocina caliente. Lo más probable es que el producto de prueba esté asignado a las dos estaciones. Confírmalo.
6. **«Precuenta» frente a «Ticket de venta».** En el código del sistema anterior (`legacy-csharp`, `CLAUDE.md` §21), revisa qué documento llamaba «ticket de venta». Indica si se entregaba antes de cobrar (equivale a nuestra precuenta) o después (equivale a nuestro ticket o comprobante).
7. **Textos repetidos:** dónde se forma «Mesa Mesa 1» en la precuenta y «Mesa 2 · Mesa 2» en Cocina.

Si algo obliga a cambiar una decisión de este documento, **detente y pregunta**. Si no, continúa.

---

## 3. FASE A — DATOS DE PRUEBA FUERA DE LAS INSTALACIONES REALES

- **Las pruebas automáticas nunca escriben en la base de datos de una instalación real.** Las pruebas sobre la aplicación instalada, como la prueba de humo de Windows, usan una base de datos propia, temporal o con un nombre claramente de pruebas, y la borran al terminar. Agrega una protección que lo impida: por ejemplo, que el modo de pruebas se niegue a arrancar contra la base de datos de producción.
- **Limpieza de lo existente:** agrega al Mantenimiento del administrador una acción **«Quitar datos de prueba»** que elimine los destinos, estaciones, rutas, impresoras y productos creados por las pruebas automáticas, identificados por una marca o un prefijo inequívoco y nunca por suposición.
  - Antes de borrar, muestra la lista de lo que va a eliminar.
  - Crea un respaldo antes de borrar.
  - No toca ventas, pagos, cortes ni movimientos de inventario. Si un dato de prueba ya tiene operaciones registradas, lo desactiva en lugar de borrarlo.
- Ejecútala en la Mac si encontraste datos de prueba. En Windows la ejecutará el propietario.

---

## 4. FASE B — IMPRESIÓN

### 4.1 Sin impresora configurada

Hoy, al imprimir una precuenta sin una impresora válida, sale «Requiere intervención. Se resuelve en Cocina → Cola de impresión». Eso obliga a ir a otra pantalla.

**Nuevo comportamiento:**

- Si el documento no tiene una impresora válida asignada, **se avisa en el momento**: «No hay impresora configurada para precuentas», con un botón **«Configurar impresora»** que lleva directo al lugar donde se asigna.
- El trabajo no se encola como fallido.
- La acción se puede reintentar desde el mismo aviso una vez configurada.

### 4.2 Reintentos

- **Una impresión que falló y nunca salió en papel se reintenta con un toque, sin motivo**, desde el aviso de error o desde la cola.
- **El motivo se pide solo al reimprimir un documento que ya se imprimió**, como acordó la 9B.
- El aviso de error de impresión dice qué pasó en palabras simples («La impresora de caja no responde») y ofrece **Reintentar**.

### 4.3 Pantalla «Cola de impresión»

- La lista de trabajos tiene **altura limitada con desplazamiento propio**. No crece indefinidamente.
- Al elegir un trabajo («Consultar trabajo e intentos»), el panel de detalle **aparece a la vista junto a la lista**. Hoy queda arriba de la página y parece que no pasó nada.
- **Textos:**
  - Quita los datos internos visibles, como el «.15150» suelto.
  - Cambia «El destino está inactivo o requiere un agente de impresión aún no implementado» por un texto claro para el usuario.
  - Agrupa o pagina los trabajos repetidos.

### 4.4 Corte de papel

- Agrega una opción **por impresora**: **«Cortar el papel al terminar»**, con **corte total o parcial**.
- Agrega **«Renglones de avance antes del corte»**, una lista cerrada de 0 a 8 con 4 como valor predeterminado, para que el corte no caiga sobre el texto.
- **Valores predeterminados:**
  - Activado en impresoras térmicas de 80 mm.
  - Desactivado en las de 58 mm, que muchas no tienen cortador; el propietario lo activa si la suya lo tiene.
- Se envía el comando ESC/POS de corte al final de cada documento: tickets, precuentas, comandas, avisos y cortes de caja.
- Si la impresora no tiene cortador, el comando no causa problemas: la mayoría lo ignora. Documenta este comportamiento.
- **«Imprimir prueba de ancho»** también corta, para que el propietario lo pruebe.
- Validado en Go y en PostgreSQL; cambiarlo requiere el permiso de administrar impresoras y queda en el Registro de movimientos. Si requiere migración, inclúyela en la única migración de la etapa.

### 4.5 Nombre de la precuenta

Según lo que encuentres en la Fase 0, punto 6:

- Agrega en la configuración de documentos el **nombre visible** del documento previo al cobro, de una lista cerrada: **«Precuenta»** (predeterminado), **«Ticket de venta»** o **«Cuenta»**.
- El nombre se usa en botones, avisos y en el encabezado impreso.
- Internamente sigue siendo el mismo documento, con las mismas reglas. La leyenda de que no es un comprobante fiscal se conserva.

### 4.6 Textos repetidos

- Corrige «Mesa Mesa 1» en la precuenta y «Mesa 2 · Mesa 2» en Cocina.
- Revisa las plantillas y vistas que componen el nombre de la mesa, para que el prefijo «Mesa» no se repita cuando el nombre ya lo incluye.
- **Las plantillas predeterminadas deben seguir siendo idénticas byte a byte**, salvo esta corrección. Actualiza los archivos de referencia de las pruebas y documenta el cambio.

---

## 5. FASE C — COCINA POR ESTACIONES Y TERMINAL DE COCINA (KDS BÁSICO)

**Objetivo:** que una terminal pueda usarse como pantalla de cocina, y que cada área (por ejemplo cocina, barra y postres) **vea solo lo suyo**. Si cada estación ve los pedidos de las demás, el proceso se vuelve lento y confuso.

### 5.1 Estaciones por terminal

- En la configuración de la terminal: **«Estaciones que muestra esta terminal»**, con selección múltiple de las estaciones existentes. Por defecto, todas.
- Combinada con la **pantalla de inicio por terminal** de la 9C, una terminal de cocina arranca directo en **Cocina › Preparación** mostrando solo sus estaciones.

### 5.2 Estaciones por usuario

- Si ya existe la autorización de estaciones por usuario, verifica que funcione. Si no existe, agrégala: **«Estaciones que puede atender»**, en el usuario o en su rol.
- **Lo que se ve es la intersección:** las estaciones de la terminal y, dentro de ellas, solo las autorizadas al usuario. Si no hay ninguna en común, se muestra un aviso claro en lugar de una pantalla vacía.

### 5.3 Un producto en varias estaciones

- Si un producto o combo se prepara en varias estaciones (por ejemplo, la hamburguesa en la plancha y las papas en la freidora), **cada estación ve solo su parte**, no el producto completo repetido.
- «Marcar producto listo» en una estación marca solo esa parte. El producto queda listo cuando todas sus partes lo están, y así se refleja en la cuenta.
- Si el comportamiento actual ya es este y la duplicación de la prueba era configuración, documéntalo y agrega una prueba que lo confirme.

### 5.4 Modo cocina de la pantalla

- **Botones grandes**, para tocar con las manos ocupadas.
- **Tarjetas que cambian de color según el tiempo** contra el objetivo:
  - Normal antes del objetivo.
  - Ámbar al acercarse.
  - Rojo al pasarlo.
- **Orden por antigüedad.**
- **Aviso sonoro opcional** al llegar una comanda nueva, configurable por terminal.
- Quita el botón «Actualizar preparación»: la pantalla se actualiza sola, como Mesas. Muestra «Reintentar» solo sin conexión.
- Para un usuario de cocina, la barra superior muestra solo **Cocina** y el menú del usuario, según sus permisos, sin las demás áreas.

### 5.5 Documentación

Agrega a `CLAUDE.md` y a una guía corta para el propietario (`docs/TERMINAL_DE_COCINA.md`) cómo configurar una terminal de cocina:

1. Crear las estaciones.
2. Asignar los productos a cada estación.
3. Crear un usuario de cocina con sus estaciones.
4. Configurar la terminal: pantalla de inicio y estaciones que muestra.

---

## 6. FASE D — AJUSTES DE INTERFAZ

### 6.1 Barra superior compacta

Ocurre con la ventana sin maximizar, a unos 1500 px de ancho:

- **El logotipo se cambia por una «A» genérica.** En modo compacto debe mostrarse el **isotipo de la marca**, el mismo «AE» del ícono del instalador.
- **El logotipo no muestra el contorno de foco** salvo al navegar con teclado (`:focus-visible`).
- **Caja no pierde el estado del turno:** usa un texto corto («Abierto» o «Cerrado») o, como mínimo, una ayuda emergente.
- **«En línea» no queda como un punto suelto:** conserva un texto corto o una ayuda emergente.
- Verifica la barra a 1280, 1440, 1500 y 1920 px.

### 6.2 Leyenda del mapa de mesas

En ventanas más angostas, la leyenda provoca una barra de desplazamiento horizontal. Debe acomodarse en dos renglones o abreviarse, sin desplazamiento horizontal.

---

## 7. FASE E — PENDIENTES TÉCNICOS

### 7.1 Ruta antigua de anulación

La aplicación ya no usa la ruta antigua de anulación de cuentas, pero sigue disponible en el servidor. **Retírala, o redirígela a la lógica de «Cancelar cuenta»** con motivo y autorización, para que ningún cliente (incluida la futura aplicación Android) pueda cancelar sin motivo ni autorización. Agrega una prueba.

### 7.2 Autorización para cancelar unidades

Aplica a «Cancelar unidades» (reducir cantidades ya enviadas a cocina) el mismo criterio de la 9D:

- Basta el permiso propio.
- Sin permiso, se pide la autorización de un supervisor.
- Se registran quién la hizo y quién la autorizó.
- El reporte «Modificaciones de cantidades» muestra la columna **«Autorizó»**.

### 7.3 Migración

Si se necesitan cambios de esquema (corte de papel, estaciones por terminal o por usuario, nombre de la precuenta, autorizador en cancelación de unidades, marca de datos de prueba), hazlos en **una sola migración**, con el procedimiento de respaldo. **Esquema esperado: 22.**

---

## 8. PRUEBAS

- **Fase 0-R:** regresión completa de la 0.9.4.
- **Datos de prueba:**
  - Las pruebas no escriben en la base de producción.
  - «Quitar datos de prueba» muestra la lista, hace el respaldo y respeta los datos con operaciones.
- **Impresión:**
  - Aviso sin impresora con «Configurar impresora».
  - Reintento sin motivo de una impresión fallida.
  - Motivo obligatorio al reimprimir.
  - Corte total, corte parcial y sin corte, con avance, en los bytes ESC/POS generados.
  - Nombre de la precuenta en botones y en el impreso.
  - «Mesa» sin repetir.
- **Cocina:**
  - Estaciones por terminal y por usuario, con la intersección y el aviso sin estaciones en común.
  - Un producto en dos estaciones, donde cada una ve su parte y el producto queda listo al completar ambas.
  - Colores por tiempo.
  - Sin botón «Actualizar».
- **Cola de impresión:** lista con altura limitada y detalle a la vista (Playwright).
- **Barra compacta** en los cuatro anchos, y leyenda del mapa sin desplazamiento horizontal.
- **Cancelar unidades** con y sin permiso, y la columna «Autorizó».
- **Ruta antigua** de anulación retirada o redirigida.
- **Regresión completa** en la Fase F. Como no hay Windows en esta sesión, compila además el código Go para Windows (`GOOS=windows go build ./...`) y verifica que la parte de Rust no tenga errores evidentes de compilación condicional.

---

## 9. DOCUMENTACIÓN Y ENTREGA

- Actualiza `CLAUDE.md`:
  - Versión 0.9.5 y esquema 22 (si hubo migración).
  - La protección contra datos de prueba.
  - El corte de papel.
  - Las estaciones por terminal y por usuario.
  - El nombre de la precuenta.
- Actualiza `docs/NOTAS_WINDOWS.md` con lo que haya que verificar en Windows de esta etapa.
- Crea `docs/TERMINAL_DE_COCINA.md`.
- Instala en la Mac, fusiona en `main` y haz push.
- **Entrega final breve:**
  - Resultado de la regresión de la 0.9.4.
  - Cambios por fase.
  - Qué encontraste sobre el «ticket de venta» del sistema anterior y qué nombre quedó como predeterminado.
  - Si la estación duplicada era configuración o un defecto.
  - Datos de prueba encontrados y eliminados.
  - Pruebas ejecutadas.
  - Migración.
  - Si se actualizó tu memoria de trabajo.
  - Limitaciones: corte de papel y terminal de cocina sin hardware real, y lo que falta verificar en Windows.
