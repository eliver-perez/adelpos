# AD-EL POS ANDROID — ETAPA A1: BASE, EMPAREJAMIENTO, PIN Y MODO MESERO CON CAPTURA SIN CONEXIÓN

**Primera etapa de la aplicación Android de meseros.** Se ejecuta en la Mac **después de terminar la 9H**, sobre `main`.

La aplicación Android es un **cliente más del servidor del restaurante** y usa la **API v1** de la 9G. **El servidor Go es la única autoridad** en precios, impuestos, permisos y estados. La aplicación **nunca** calcula ni decide reglas de negocio.

**Alcance de esta etapa:**

- Proyecto Android.
- Emparejamiento y búsqueda del servidor.
- Entrada con PIN en dispositivos compartidos.
- Catálogo guardado en el dispositivo.
- **Modo Mesero completo:** mesas, cuentas y captura, con y sin conexión, y la cola de pedidos.
- Diseño adaptado a teléfono y a tableta.

**Etapa A2** (después): modo Para llevar, impresión de precuenta desde el dispositivo y aviso de platillos listos.

---

## 0. FORMA DE TRABAJO

### Repositorio y estructura

- Mismo repositorio, carpeta nueva **`android/`**, con su propio `android/CLAUDE.md`: estructura, comandos de compilación y pruebas, y reglas.
- Rama **`etapa-a1-android`** desde `main`.
- Las especificaciones en `docs/etapas/` son de solo lectura.
- Lee este documento, `CLAUDE.md`, **`docs/API_V1.md`** y el contrato OpenAPI (`internal/apicontract/openapi.json`), y `docs/INSTALAR_EN_RED.md`.

### Cambios en el servidor

- **Solo los indispensables** que pide este documento (§3 PIN, §8 ajustes de la API) o que la Fase 0 demuestre necesarios.
- Siempre **en el contrato v1**, de forma compatible y con sus pruebas de contrato.
- Si algo de la API obliga a cambiar una decisión de negocio, **detente y pregunta**.

### Eficiencia

- La Fase 0 la hace **un solo agente**.
- **Máximo 2 agentes en total trabajando al mismo tiempo. Los subagentes no pueden lanzar otros subagentes.**
- En cada fase, solo las pruebas de lo que cambió; la regresión completa (servidor y Android), una sola vez, en la Fase G.
- **Revisor independiente solo** para el PIN y la sesión (§3) y para la cola sin conexión (§6).

### Autonomía

Aplica la regla de autonomía de `CLAUDE.md` §0. **Las pruebas nunca usan el servicio real de la Mac** (puertos 8181 y 8443, carpeta del servicio): usan un servidor Go temporal con su base desechable, como las pruebas de la 9G.

**Paradas:**

- Instalar Android Studio o el SDK, si no están. Indica al propietario qué instalar.
- Conectar un teléfono físico con depuración USB. Indica cómo activarla.
- Actualizar el servicio de la Mac con un `.pkg`, si hubo cambios en el servidor (contraseña de administrador).

### Fases

| Fase | Contenido |
| ---- | --------- |
| **0** | Verificación: entorno Android, API v1 y lo que falte en el servidor. |
| **A** | Proyecto base, arquitectura, tema y diseño adaptable (§2). |
| **B** | PIN en el servidor y entrada con PIN en el dispositivo (§3). |
| **C** | Búsqueda del servidor, emparejamiento y conexión cifrada (§4). |
| **D** | Catálogo en el dispositivo (§5). |
| **E** | Modo Mesero: mesas, cuentas, captura y envío, con la cola sin conexión (§6, §7). |
| **F** | Pruebas de extremo a extremo contra un servidor temporal y en dispositivos (§9). |
| **G** | Regresión completa, documentación, APK de prueba, fusión y push. |

**Al terminar cada fase:** ejecuta sus pruebas, registra un resumen en `docs/ETAPA_A1_AVANCE.md`, haz un commit y continúa.

---

## 0-B. DECISIONES DEL PROPIETARIO

**Si una sección parece decir otra cosa, prevalece esta.**

- **Teléfonos y tabletas.** Los meseros usan tabletas, y teléfonos como apoyo. Un solo diseño que se adapta: en tableta, catálogo y cuenta lado a lado; en teléfono, una pantalla a la vez con la cuenta siempre a un toque.
- **Los dispositivos se comparten entre meseros.** Se entra con **PIN**.
- **Sin límite de tiempo para capturar sin conexión.** El servidor valida todo al recibir el pedido.
- **Nunca se cierra la sesión ni se bloquea el dispositivo mientras hay una captura en curso o pedidos sin enviar.**
- **El dispositivo no cobra,** ni en mesas ni en Para llevar.
- **Precios cambiados o productos agotados** en un pedido capturado sin conexión: el servidor no lo acepta en silencio. El mesero ve los cambios y los acepta antes de que el pedido entre (contrato de la 9G, §5.5).
- **Un borrador nunca se envía solo a cocina.** El mesero confirma explícitamente con «Enviar a cocina».
- **Nunca se muestra un pedido como recibido por cocina si el servidor no lo ha confirmado.**
- **Licencias:** todavía no se limita nada. El servidor ya cuenta las sesiones Android.

---

## 1. FASE 0 — VERIFICACIÓN

Registra en el avance:

1. **Entorno:** si están Android Studio, el SDK, un emulador y Java/Gradle en la Mac. Si falta algo, **detente** e indica qué instalar.
2. **La API v1 desde el punto de vista del mesero.** Recorre el contrato e indica qué ya existe y qué falta para:
   - Emparejamiento.
   - Entrada con PIN.
   - Catálogo completo y por cambios con época.
   - Áreas y mesas con su estado.
   - Cuentas de una mesa, abrir cuenta (creación diferida) y borrador.
   - Enviar a cocina con clave de operación.
   - Pedido sin conexión con validación.
   - Modificar o cancelar unidades según permisos.
   - Estados de preparación.
   - Recuperación del estado (`restaurant/state`).
   - Eventos en tiempo real.
   - Versión mínima de cliente.
3. **Eventos en tiempo real (SSE)** con una aplicación Android: si el formato y la reanudación sirven, y qué hace falta para que la aplicación se recupere al volver de segundo plano sin depender de haber recibido todos los eventos.
4. **Permisos actuales del rol Mesero** y si alcanzan para lo que pide esta etapa.

---

## 2. FASE A — PROYECTO BASE

- **Tecnología:**
  - **Kotlin** y **Jetpack Compose** (Material 3), con la versión estable actual.
  - **Room** (SQLite) para los datos locales.
  - **WorkManager** para la cola de envío.
  - Un cliente HTTP (OkHttp/Retrofit o Ktor) con **TLS de certificado fijado**.
  - Corrutinas y Flow.
- **Android mínimo:** 8.0 (API 26), para cubrir tabletas económicas. Destino: el SDK actual que exige Google Play.
- **Arquitectura:**
  - Capas separadas: datos (API y Room), dominio y pantallas.
  - Componentes compartidos para que el modo Para llevar de la A2 los reutilice sin duplicar.
  - Inyección de dependencias sencilla (Hilt o manual).
- **Diseño:**
  - Colores de AD-EL POS: el azul de la marca (`#1F5FA8`, con `#173D6E` presionado) y la paleta de estados de la 9D (Libre, Ocupada, Esperando pago, Pago parcial y Fuera de servicio, con sus íconos).
  - Tema claro y oscuro.
  - Botones grandes para uso táctil (48 dp como mínimo, 56 dp en las acciones principales).
  - Textos en español, sin tecnicismos, con los mismos criterios de la aplicación de escritorio.
- **Diseño adaptable:**
  - Tableta o pantalla ancha: dos paneles.
  - Teléfono: un panel con navegación.
  - Rotación sin perder lo capturado.
- **Indicador de conexión siempre visible,** en todas las pantallas:
  - En línea.
  - Sin conexión.
  - **Pedidos pendientes de envío**, con contador.

---

## 3. FASE B — PIN

### 3.1 Servidor

- **Cada usuario puede tener un PIN** de 4 a 6 dígitos. Lo define el propio usuario o se lo asigna un administrador con permiso de administrar usuarios.
- **Seguridad del PIN:**
  - Se guarda con un hash lento y con sal, igual que las contraseñas.
  - Se rechazan PIN triviales (1234, 0000, 1111…).
- **La entrada con PIN solo se acepta desde un dispositivo emparejado,** con su credencial. Nunca desde una conexión sin credencial de dispositivo.
- **Intentos fallidos:** tras 5, el PIN de ese usuario se bloquea unos minutos y queda en el Registro de movimientos. Un administrador puede desbloquearlo.
- **Lista de usuarios para el dispositivo:** nombre corto y, si existe, una inicial o color, **solo** de los usuarios con PIN y permiso para usar dispositivos de meseros.
- Agrégalo al contrato v1 con sus pruebas.

### 3.2 Dispositivo

- **Pantalla de entrada:** lista de meseros con su nombre en botones grandes. Se toca el propio y se teclea el PIN en un teclado numérico grande.
- **Cambio rápido de usuario** desde un menú visible.
- **Bloqueo por inactividad:** tras un tiempo sin uso (2 minutos por defecto, configurable por dispositivo en Administración › Terminales), el dispositivo vuelve a la pantalla de entrada, **excepto** si hay una captura en curso o pedidos sin enviar de ese mesero. Lo capturado se queda guardado con el mesero que lo capturó, y el siguiente no lo ve.
- **Entrada sin conexión:** si el dispositivo no tiene señal, solo pueden entrar los meseros que ya entraron antes en **ese** dispositivo.
  - Se usa un verificador local del PIN, guardado cifrado con la llave del Android Keystore.
  - El verificador se renueva en cada entrada con conexión, y al reconectarse el servidor puede invalidarlo si el PIN cambió o el usuario se desactivó.
  - Todo lo que se capture así lo valida el servidor al recibirlo, incluidos los permisos.
- **Dispositivo revocado** desde la computadora principal: al enterarse, el dispositivo borra sus datos locales y su credencial.
  - Si tenía pedidos sin enviar, los muestra al mesero con un aviso claro antes de borrarlos: «Este dispositivo fue retirado del restaurante; estos pedidos no se enviaron».
  - Esos pedidos quedan en la bitácora del dispositivo hasta que se borra.

---

## 4. FASE C — SERVIDOR Y EMPAREJAMIENTO

- **Búsqueda automática** del servidor en la red local con NSD (mDNS, `_adelpos._tcp`, TXT con `server_id` y huella corta, como en la 9G). También se puede **escribir la dirección** a mano.
- **Emparejamiento con el protocolo exacto de la 9G:**
  - Compromiso, código de 6 dígitos que el administrador aprueba en la computadora principal, y reclamo único de la credencial.
  - Tipo de dispositivo `android`.
  - Nombre editable («Tableta 1»).
- **Certificado fijado:** la huella SPKI se fija durante el emparejamiento. Si después cambia, el dispositivo no se conecta y lo avisa. **Nunca acepta cualquier certificado.**
- **La credencial del dispositivo** se guarda cifrada con el **Android Keystore**.
- **Cambio de IP:** si el servidor cambia de IP, el dispositivo lo vuelve a encontrar por `server_id` y huella, como el escritorio.
- **Versiones:** envía `X-Client-Version` y `X-Client-Kind: android`, y maneja «Actualiza esta aplicación» (426).
- **Permisos de Android:** solicita los de red y de dispositivos cercanos que exija cada versión, con una explicación clara en español.

---

## 5. FASE D — CATÁLOGO EN EL DISPOSITIVO

- **Descarga del catálogo:** completo la primera vez; después, **solo los cambios** desde la versión y la época guardadas. Con `reset: true`, reemplaza todo.
- **Se actualiza:**
  - Al abrir la aplicación.
  - Al recuperar la conexión.
  - Al recibir el evento de cambio de catálogo.
  - Periódicamente mientras está en uso.
- **Imágenes** con caché propia, descargadas aparte.
- **El catálogo local permite capturar sin conexión:**
  - Categorías y productos.
  - Presentaciones, complementos, combos, notas rápidas y porciones (Sin/Normal/Extra).
  - **Disponibilidad («Agotado»).**
- **Los precios del catálogo local se muestran como referencia.** El importe final siempre lo calcula el servidor.

---

## 6. FASE E — MODO MESERO

### 6.1 Mesas

- **Áreas y mesas** con su estado y tiempo.
- **Vista de lista** (predeterminada) y **vista de mapa**, que se puede cambiar.
- **Búsqueda de mesa.**
- **Las mesas que el mesero no puede atender** según sus permisos no se muestran o aparecen deshabilitadas, según lo que ya haga el servidor.

### 6.2 Cuentas de una mesa

- Ver las cuentas abiertas y elegir una, o empezar una nueva.
- **Creación diferida:** la cuenta nace al guardar el primer producto, como en el escritorio. Sin conexión, nace al enviarse.

### 6.3 Captura

- **Catálogo:**
  - Categorías siempre a la mano.
  - **Búsqueda de productos.**
  - Personalizador adaptado a pantalla táctil: complementos, porciones con −/+, notas rápidas y observaciones.
- **La cuenta o el pedido siempre visible:** al lado en tableta, a un toque en teléfono.
- **Modificar** lo no enviado.
- **Modificar o cancelar unidades enviadas** según los permisos que ya existen, registrando quién lo hizo. Eso genera los avisos de cocina que ya produce el servidor.
- **«Enviar a cocina»** explícito, con la clave de operación.
- **Estados de preparación** de lo enviado, con el mismo modelo de estados del escritorio y del KDS.
- **Cambios hechos desde otras terminales** visibles al momento cuando hay conexión.

### 6.4 Si cambian cosas mientras tanto

- Si al enviar el servidor responde que cambió un precio, un producto se agotó o la mesa se cerró, el pedido queda en **«Requiere tu revisión»**.
- El mesero ve una lista clara: «Hamburguesa: $120 → $130», «Malteada de fresa: agotada» (quitar o cambiar), «La Mesa 4 ya se cobró».
- El pedido se vuelve a enviar solo **después de que el mesero acepta los cambios**.

---

## 7. COLA SIN CONEXIÓN

**Estados del pedido**, siempre visibles y distintos entre sí:

1. **Borrador:** en el dispositivo, no enviado.
2. **Por enviar:** el mesero ya confirmó, pero no hay conexión.
3. **Enviando.**
4. **Recibido por el restaurante:** confirmado por el servidor.
5. **Requiere tu revisión:** cambios de precio, agotados o conflictos.

**Reglas:**

- **Al confirmar sin conexión,** el pedido se guarda en Room y entra a la cola con su **clave de operación**, la **versión y época del catálogo** y los **precios que vio** el mesero.
- **WorkManager envía los pendientes** al volver la conexión, aunque la aplicación esté cerrada o en segundo plano, con espera creciente. Si el envío se pierde antes de la respuesta, el reintento usa la misma clave y **nunca duplica** el pedido. Usa `OFFLINE_ORDER_ALREADY_ACCEPTED` de la 9G.
- **Los pedidos pendientes nunca se borran** por cerrar la aplicación, reiniciar el dispositivo o perder la batería.
- **No se puede cerrar sesión, cambiar de mesero ni bloquear por inactividad** con pedidos de ese mesero en «Por enviar», «Enviando» o «Requiere tu revisión».
- **Pantalla «Pedidos pendientes»:** lista de lo que falta enviar, con su estado y la opción de reintentar ya.

---

## 8. AJUSTES DE LA API (SI HACEN FALTA)

- Si la Fase 0 encuentra huecos en la API v1 para el modo Mesero, agrégalos **de forma compatible**, en el contrato y con pruebas, sin romper el escritorio.
- Documenta cada cambio en `docs/API_V1.md`.
- **Asignación de la impresora de precuentas por dispositivo** (para la A2): si el modelo de terminales de la 9G ya la admite para dispositivos `android`, no hagas nada; si no, déjalo anotado para la A2.

---

## 9. PRUEBAS

- **Unitarias (JVM):**
  - Dominio.
  - Estados de la cola.
  - Correspondencia con la API.
  - Verificador del PIN.
  - Selección del servidor por `server_id` y huella.
- **Room:** migraciones del esquema local, persistencia de la cola tras reiniciar el proceso y datos por mesero.
- **Contra un servidor Go temporal** (pruebas de integración en el emulador), con base desechable y nunca el servicio real:
  - Emparejamiento completo con aprobación del código.
  - Certificado cambiado (rechazado).
  - Entrada con PIN, PIN incorrecto y bloqueo.
  - Catálogo completo y por cambios, `reset` y producto agotado.
  - Captura en línea y envío a cocina.
  - **Captura sin conexión** (la red del emulador apagada), envío al volver, **respuesta perdida y reintento sin duplicar**, y precio cambiado o agotado con confirmación del mesero.
  - Mesa cobrada mientras tanto.
  - Revocación del dispositivo.
  - Versión vieja (426).
- **Interfaz (Compose):**
  - Teléfono y tableta, con rotación sin perder lo capturado.
  - Tema oscuro.
  - Indicador de conexión y de pendientes.
  - Bloqueo por inactividad que no ocurre con pendientes.
- **Dispositivo físico:** si el propietario conecta una tableta o un teléfono, repite el flujo principal contra el servidor temporal.
- **Regresión completa del servidor** en la Fase G si hubo cambios en Go.

---

## 10. DOCUMENTACIÓN Y ENTREGA

- **`android/CLAUDE.md`:** estructura, comandos, pruebas y reglas permanentes, incluida la de que la aplicación nunca calcula reglas de negocio.
- **`docs/API_V1.md`** actualizado si hubo cambios.
- **`docs/ANDROID_INSTALAR.md`** para el propietario, corto y sin tecnicismos:
  1. Instalar el APK de prueba en una tableta o un teléfono.
  2. Emparejarlo con la computadora principal.
  3. Asignar PIN a los meseros.
  4. Qué hacer si no encuentra el servidor.
- **APK de prueba** firmado con una llave de desarrollo, **nunca la llave de publicación**, y sin subir llaves al repositorio. Versión Android **0.1.0**.
- Si hubo cambios en el servidor: `.pkg` (parada para instalarlo con contraseña de administrador), versión del servidor **0.12.0** y `min_client_version` coherente.
- Fusiona en `main` y haz push.
- **Entrega final:**
  - Cambios por fase.
  - Cambios en el servidor.
  - Pruebas.
  - Dispositivos probados.
  - Tamaño del APK.
  - Limitaciones, y lo que queda para la A2: Para llevar, imprimir precuenta y aviso de platillos listos.
