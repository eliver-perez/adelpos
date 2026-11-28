# AD-EL POS — ETAPA M1: UNIR MESAS, MESAS DIVISIBLES Y AJUSTES PENDIENTES

Se ejecuta **en la Mac**, sobre `main`.

- **Parte de:** servidor 0.17.0 (esquema 30), meseros 0.4.1 y cocina 0.1.0. Confirma los valores reales en `config/product.json`, `CLAUDE.md`, `android/CLAUDE.md` y `cocina/CLAUDE.md`.
- **Al terminar:**
  - servidor **0.18.0**;
  - meseros **0.5.0** (versionCode 6);
  - cocina **0.1.1**, solo si cambia su código.

**Contenido:**

- **Unir mesas** y **mesas divisibles** (§1 a §4): función nueva acordada con el propietario.
- **Ajustes pendientes** (§5): defectos y observaciones del propietario de la A3 a la K1.1, para dejar limpia la lista de pendientes.

No se agregan funciones fuera de lo que dice este documento.

---

## 0. FORMA DE TRABAJO

### Rama y especificaciones

- Rama **`etapa-m1`** desde `main`. Las especificaciones en `docs/etapas/` son de solo lectura.
- Lee este documento, `CLAUDE.md`, `android/CLAUDE.md`, `cocina/CLAUDE.md`, `docs/API_V1.md`, `docs/KITCHEN_AND_PRINTING.md` y `docs/MIGRAR_SISTEMA_ANTERIOR.md`.

### Eficiencia

- La Fase 0 la hace **un solo agente**.
- **Máximo 2 agentes en total trabajando al mismo tiempo. Los subagentes no pueden lanzar otros subagentes.**
- En cada fase, solo las pruebas de lo que cambió. La regresión completa, **una sola vez**, en la Fase F. Si una corrección de la regresión toca `backend/`, `config/` o código compartido, repite la regresión completa; si no, solo las suites afectadas.
- **Revisor independiente** solo para la ocupación de mesas al unir (§2.3): dos terminales a la vez sobre las mismas mesas no deben dejar una mesa en dos lugares ni perder una cuenta.
- **Android en dos emuladores durante las fases** (`Pixel_Tablet` y `ADEL_A1_Tablet_API26`); en la regresión final también `Pixel_8` (teléfono), porque esta etapa cambia pantallas de meseros en teléfono.
- **Capturas:** solo de las pantallas nuevas o cambiadas, en tema claro.

### Autonomía

Aplica la regla de autonomía de `CLAUDE.md` §0 y la protección contra el servicio real. Ninguna prueba llama a `licensing.sicrat.com`.

**Paradas:** instalar el `.pkg` (contraseña de administrador).

### Fases

| Fase | Contenido |
| ---- | --------- |
| **0** | Verificación y reproducción de los defectos (§6). |
| **A** | Servidor: uniones, grupos predefinidos y lo del servidor de los ajustes (§1 a §4, §5.1, §5.3). |
| **B** | Escritorio: mapa, «Unir con…», mesas divisibles en Áreas y distribución, y ajustes del escritorio (§1 a §3, §5.1, §5.2, §5.4). |
| **C** | Aplicación de meseros: unir mesas, tarjeta única de la cuenta y ajustes de Android (§1, §5.2, §5.5 a §5.7). |
| **D** | Aplicación de cocina, solo si hace falta (§1.5). |
| **E** | Pruebas frágiles (§5.8). |
| **F** | Regresión completa, documentación, APK, `.pkg`, fusión y push (§8). |

**Al terminar cada fase:** ejecuta sus pruebas, registra un resumen en `docs/ETAPA_M1_AVANCE.md`, haz un commit y continúa.

---

## 1. UNIR MESAS

### 1.1 Qué se puede unir

- Un grupo de mesas **de la misma área** se une en el momento con **«Unir con…»**, desde el mapa o la lista de mesas.
- **Las mesas que se agregan deben estar libres.** La mesa desde la que se empieza puede estar **libre u ocupada**: si está ocupada, sus cuentas pasan a la unión y se conservan. Así crece una mesa a la que llegaron más personas.
- **No se unen dos mesas ocupadas.** Para juntar cuentas de mesas distintas ya existe «Combinar cuentas» (9D); el mensaje lo dice.
- **Se pueden agregar o quitar mesas libres** mientras la unión está abierta:
  - quitar una mesa no mueve ninguna cuenta;
  - si al quitar queda una sola mesa, la unión se deshace y las cuentas quedan en esa mesa.

### 1.2 Cómo se ve

- En el mapa (escritorio y Android), la unión se ve como **un solo contorno** que abarca sus mesas, con el nombre **«5 + 6»** (en orden del nombre de las mesas) y la **capacidad sumada**.
- En la lista de mesas aparece como una sola entrada «5 + 6».
- Las mesas unidas no se pueden abrir por separado mientras dure la unión: tocar cualquiera de ellas abre la unión.

### 1.3 Cómo se comporta

- **La unión se comporta como una sola mesa:** sus cuentas son de la unión, con su mesero, sus pedidos, su precuenta y su cobro, igual que una mesa normal.
- **Creación diferida**, igual que en mesas: si se une y se sale sin capturar nada, no queda nada guardado. La unión nace al guardar el primer producto, o al unir una mesa ya ocupada.
- **Se deshace sola** al liberarse, cuando ya no le quedan cuentas abiertas. Las mesas vuelven a estar libres y separadas.
- **Permiso nuevo «Unir mesas»**, en los roles de fábrica Mesero, Cajero, Supervisor y Administrador. Se agrega también en «Restablecer permisos predeterminados».

### 1.4 Registro y reportes

- **Las ventas quedan registradas en la unión**, con la lista de mesas que la formaron.
- Los documentos (comanda, precuenta, ticket, comprobante) dicen **«Mesa 5 + 6»**.
- Los reportes por mesa muestran la unión como «5 + 6», **sin repartir la venta** entre las mesas.
- El historial de la cuenta registra cuándo se unió, se agregó o se quitó cada mesa y quién lo hizo.

### 1.5 Cocina

- La tarjeta del KDS, Preparación y la comanda impresa dicen **«Mesa 5 + 6»**.
- Un cambio de unión mientras hay tarjetas en cocina actualiza el encabezado de la tarjeta (el evento de encabezado de la K1, K1-D7).
- Si Go ya manda el nombre y el KDS lo muestra sin cambios, la aplicación de cocina no cambia ni se compila otra vez.

---

## 2. SERVIDOR: UNIONES

### 2.1 Modelo

- Diseña el modelo en la Fase 0: por ejemplo, una unión con su lista de mesas, y las cuentas ligadas a la unión.
- **Sin tocar las ventas ni las cuentas ya cerradas.**
- La migración es la `0031` y solo agrega tablas y columnas. Las cuentas existentes siguen igual.

### 2.2 Reglas en el servidor

- Todas las reglas de §1 se validan **en el servidor**: misma área, mesas libres, el permiso, «no unir dos ocupadas». Escritorio y Android solo muestran.
- Cada operación con su `operation_id` e idempotencia, como las demás escrituras del restaurante.
- Eventos para que el mapa de todas las terminales y tabletas se actualice al momento.

### 2.3 Concurrencia (revisor independiente)

- **Dos terminales que intentan usar la misma mesa a la vez** (una la abre sola y otra la une, o las dos la unen a grupos distintos): solo una gana. La otra recibe un mensaje claro, por ejemplo «La mesa 6 ya está ocupada».
- **Una unión diferida** (en pantalla, sin guardar) que al guardar encuentra una de sus mesas ocupada: no guarda nada y lo dice, sin perder lo capturado en pantalla.
- **Una prueba de concurrencia** con varias terminales a la vez sobre las mismas mesas.
- Usa el orden de candados de `CLAUDE.md` §3. Si hace falta uno nuevo, es el siguiente número libre y se documenta.

### 2.4 API y cola sin conexión de Android

- API v1 para unir, agregar, quitar y leer uniones, documentada en `docs/API_V1.md` y `openapi.json`, con una capacidad nueva (por ejemplo `table-unions-v1`).
- **Un pedido de la cola sin conexión de Android** que se capturó en una mesa que, al llegar, ya está en una unión, va a la unión. Si la unión ya no existe y la mesa está libre, va a la mesa. Define la regla en la Fase 0 y pruébala.

---

## 3. MESAS DIVISIBLES (CASO DE LA MESA EN U)

- En **Áreas y distribución**, una mesa puede configurarse como **grupo predefinido**: por ejemplo, «Mesa 3» formada por **3A, 3B y 3C**, dibujadas juntas.
- **Cada sección es una mesa normal:** se abre, se cobra y se libera por separado.
- **«Mesa 3»** es un atajo que **une las tres de un toque**. Usa el mismo mecanismo de §1 y la unión se llama «Mesa 3».
- **Combinaciones parciales:** 3A + 3B dejando libre 3C, con «Unir con…», igual que cualquier unión. Se llaman «3A + 3B».
- **Una sección ocupada impide abrir «Mesa 3» completa.** Para crecer, se une la sección ocupada con las libres, conservando sus cuentas (§1.1). El mensaje lo explica.
- En el mapa, el grupo se ve dibujado junto, con las secciones distinguibles. Cuando las tres están unidas, se ve como «Mesa 3».
- **Sistema anterior:** el importador (9H) convierte «Mesa 3» y «3a, 3b, 3c» en un grupo predefinido. Revisa en la Fase 0 cómo vienen esas mesas en el paquete de migración. Si el patrón no es seguro de detectar, el importador lo reporta y el grupo se arma a mano en Áreas y distribución; anótalo.

---

## 4. LÍMITES Y TEXTOS

- Las uniones no cambian la licencia, los cupos ni los permisos de cobro.
- **Regla de textos:** ningún texto provisional, identificadores internos ni nada en inglés. Mensajes claros en cada rechazo, por ejemplo:
  - «Solo se pueden unir mesas libres.»;
  - «Las mesas deben ser de la misma área.»;
  - «La sección 3B está ocupada. Únela con las secciones libres para crecer.».

---

## 5. AJUSTES PENDIENTES

### 5.1 Defecto: los renglones de la cuenta en el escritorio no pasan a «Listo»

- **Visto por el propietario (27/11, 0.17.0):** cuenta #26 de la Mesa 1. Después de marcar «Pedido listo» en el KDS, el servidor ya lo tiene listo (la aplicación de meseros dice «Listo» en esa cuenta). Pero en la caja, todos los renglones siguen diciendo «En cocina», aunque se cierre y se abra la aplicación.
- Corrige la etiqueta por renglón del escritorio para que siga el estado real (en cocina, listo, entregado), igual que Android.
- Revisa si también falla con una marca hecha desde Preparación del escritorio.

### 5.2 Sin etiqueta para lo que no necesita preparación

- **Visto por el propietario:** la Coca Cola y las papas, sin ruta («No necesitan preparación»), dicen «En cocina» igual que la hamburguesa.
- **Decisión del propietario:** un renglón que **no necesita preparación no lleva ninguna etiqueta de cocina**, ni en el escritorio ni en Android. Así se ve más limpio y el mesero no espera un aviso que nunca llegará.
- Los combos siguen como están: el KDS muestra el combo con todos sus componentes. El propietario lo acepta.

### 5.3 Quitar la hora a un pedido programado lo envía

- Hoy queda en «Por enviar a cocina» hasta que alguien toque «Enviar a cocina ahora» (K11-D9). **No lo decidió el propietario.**
- **Decisión del propietario:** al quitar la hora, el pedido **se envía a cocina en ese momento**, igual que un pedido sin hora. Es la decisión propuesta 9 del avance de la K1.1.

### 5.4 Textos de Preparación

- «Liberado a preparación» (la etiqueta de cada tarjeta) y «No hay unidades de preparación en este filtro.» se reescriben en lenguaje sencillo, por ejemplo «En preparación» y «No hay pedidos con este filtro.».
- Revisa el resto de Preparación con la misma regla.

### 5.5 Pantalla de la mesa en Android: una tarjeta por cuenta con «Tu cuenta»

- **Hoy:** al entrar a una mesa, el mesero ve su pedido dos veces: en «Lo tuyo en esta mesa» («Mesa 2 · 4 productos · Recibido por el restaurante») y en «Cuentas abiertas» («Mesa 2 · folio 27 · $280.00 · Listo»).
- **Esperado:**
  - **una sola tarjeta por cuenta**, con su folio, total, estado del envío y de cocina («Recibido por el restaurante», «Listo»…) y «Imprimir precuenta»;
  - **una etiqueta «Tu cuenta»** arriba a la derecha cuando la cuenta es de ese mesero;
  - **lo que todavía no tiene cuenta** (un pedido sin conexión por enviar) sigue visible como su propia tarjeta con «Por enviar», para que no se pierda de vista;
  - se quita la sección «Lo tuyo en esta mesa».
- En teléfono y tableta, vertical y horizontal.

### 5.6 Pendientes de Android de la A3 y la L1

- **Textos cortados:**
  - «Enviado · sin confirmar en coci…» en el panel de la tableta vertical;
  - «Nuevo pe…» en el título de Para llevar en el teléfono vertical;
  - las pestañas de área («Terraz…») en la tableta vertical;
  - la insignia «2 pedidos por en…» de Pendientes en el teléfono.
- **«$0.00 · Borrador»** se ve un momento en el panel de la mesa justo después de «Enviar a cocina», aunque el servidor ya tenga la partida. Mide y corrige el retraso.
- **Contraste de las teclas** en «Vuelve a escribir tu PIN», en tema claro.

### 5.7 `takeaway t6` en Android 8

- Falla desde la A3 (A3-D32): el nombre del cliente editado con conexión llega al servidor, pero la lista de Para llevar de la tableta no se actualiza en 40 s.
- Busca la causa en `ADEL_A1_Tablet_API26`. Puede ser de la prueba o de la aplicación; anótalo.
- Si es de la aplicación, corrígela. Si no se encuentra en un tiempo razonable, anota lo que se descartó y déjalo como pendiente conocido.

### 5.8 Pruebas frágiles

- **`workflow-stage9d-accounts`:** la página entra y enseguida la prueba vuelve a entrar por la API con la misma terminal. Una petición de la página que ya iba en camino recibe `SESSION_REPLACED`, y las siguientes 33 pruebas caen en cascada (K1-D39, K11-D24). La prueba debe entrar por la API **antes** de entrar por la página.
- **`workflow-stagel1-notices`:** falla si cruza la medianoche. Debe usar un reloj o una fecha que no dependa de la hora real.
- **La cascada:** si una especificación falla, las siguientes no deben caer con «La aceptación debe comenzar con una base sin configurar». Si es razonable, cada especificación que necesita una base limpia la prepara ella misma.

---

## 6. FASE 0: VERIFICACIÓN

1. **Mesas hoy:** modelo de mesas, áreas y distribución, cuentas por mesa, «Combinar cuentas» (9D), el mapa del escritorio y de Android, la creación diferida, los reportes por mesa y los documentos impresos.
2. **El importador (9H):** cómo vienen en el paquete las mesas del sistema anterior, en especial «Mesa 3» y «3a, 3b, 3c».
3. **Reproduce los defectos** de §5.1, §5.2 y §5.5 con un Go temporal y una prueba automática que falle antes de corregir.
4. **Propón** el modelo de §2.1, la API de §2.4, la regla de la cola sin conexión y el reparto de fases. Anótalo en el avance.

---

## 7. PRUEBAS

- **Uniones (servidor):**
  - unir libres, crecer una ocupada, rechazar dos ocupadas y áreas distintas;
  - agregar y quitar mesas, deshacerse al liberarse, creación diferida;
  - el permiso;
  - la concurrencia de §2.3;
  - la cola sin conexión de §2.4;
  - documentos y reportes con «5 + 6»;
  - el nombre en el KDS.
- **Divisibles:** el atajo «Mesa 3», combinaciones parciales, una sección ocupada que impide «Mesa 3» y el crecimiento desde la sección ocupada; el importador con un paquete de prueba que tenga «Mesa 3» y «3a, 3b, 3c».
- **Escritorio** (Playwright, Chromium y WebKit): unir desde el mapa, el contorno y el nombre, configurar un grupo en Áreas y distribución, los rechazos y las etiquetas por renglón (§5.1, §5.2).
- **Android** (dos tabletas durante las fases; en la regresión, también `Pixel_8`): unir desde la tableta, ver la unión hecha en el escritorio, la tarjeta única con «Tu cuenta», un pedido por enviar sin conexión y los textos de §5.6.
- **La actualización 0.4.1 → 0.5.0** encima, sin volver a emparejar, con un pedido en la cola.
- **Regresión completa** una sola vez en la Fase F.

---

## 8. DOCUMENTACIÓN Y ENTREGA

- **Documentación:**
  - una guía corta para el dueño: cómo unir mesas y cómo configurar una mesa divisible;
  - `docs/MIGRAR_SISTEMA_ANTERIOR.md` con lo que hace el importador con los grupos;
  - `CLAUDE.md`, `android/CLAUDE.md`, `docs/API_V1.md` y `docs/ANDROID_INSTALAR.md`.
- **Entregables:**
  - `.pkg` **0.18.0** con su parada;
  - APK de meseros **0.5.0** con la llave de desarrollo de siempre;
  - APK de cocina **0.1.1**, solo si cambió.
- Fusiona en `main` y haz push.
- **Entrega final breve:**
  - Cambios por fase.
  - El modelo de uniones elegido.
  - Qué hace el importador con «Mesa 3» y «3a, 3b, 3c».
  - La causa de cada defecto de §5.
  - Pruebas.
  - Limitaciones.
