# AD-EL POS — ETAPA 9G: SERVIDOR DEL RESTAURANTE, TERMINALES EN RED Y PREPARACIÓN PARA ANDROID

Parte de la versión que dejó la **9F** en `main` (0.9.5). Confirma la versión y el esquema reales en `config/product.json` y `CLAUDE.md`.

Hasta ahora AD-EL POS funciona en una sola computadora, y el servidor vive mientras la ventana está abierta. Esta etapa lo convierte en lo que necesita un restaurante real:

- **Una computadora principal por restaurante.** El servidor Go y PostgreSQL corren en ella como servicio, desde que arranca, aunque nadie abra la ventana.
- **Las demás computadoras son terminales** que se conectan a ella por la red local.
- **El servidor queda preparado para la aplicación Android de meseros:** API con versión, pedidos sin duplicados, catálogo descargable y validación de pedidos capturados sin conexión.

**La aplicación Android no se hace en esta etapa.** Aquí se deja lista y probada la parte del servidor que va a usar.

Al terminar, la versión será **0.10.0**.

---

## 0. FORMA DE TRABAJO

### Dónde se ejecuta

La etapa tiene dos partes:

1. **Fases 0 a G, en la Mac.** Al terminar, fusiona en `main` y haz push.
2. **Fase W, en la computadora con Windows 11**, en otra sesión, con el mismo documento: `git pull` y «ejecuta la Fase W de ETAPA_9G». Incluye las pruebas cruzadas Mac ↔ Windows.

### Rama y especificaciones

- Rama nueva **`etapa-9g`** desde `main`.
- Las especificaciones en `docs/etapas/` son de solo lectura. Lee este documento, `CLAUDE.md`, `docs/NOTAS_WINDOWS.md` y `NEXT_STEPS_FOR_SOL.md`.

### Eficiencia

- La Fase 0 la hace **un solo agente**.
- **Máximo 2 agentes en total trabajando al mismo tiempo. Los subagentes no pueden lanzar otros subagentes.**
- En cada fase, solo las pruebas de lo que cambió. La regresión completa, una sola vez, en la Fase G (y la de Windows en la Fase W).
- **Revisor independiente solo** para lo delicado: emparejamiento y cifrado (Fase B), idempotencia y validación de pedidos (Fase D) y respaldos (Fase F). No en cada fase.

### Autonomía

Aplica la regla de autonomía de `CLAUDE.md` §0: continúa sin esperar confirmación mientras las pruebas pasen, aplica las migraciones con el procedimiento de respaldo, instala, fusiona y haz push.

**Detente y pregunta además si:**

- Hay que instalar o registrar algo que pida la contraseña de administrador del sistema. Avísale al propietario qué ventana va a aparecer.
- Hay que cambiar el firewall, el router o la configuración de red.
- Hay que reiniciar la computadora.

### Fases

| Fase | Dónde | Contenido |
| ---- | ----- | --------- |
| **0** | Mac | Verificación contra el código. |
| **A** | Mac | Modalidades de instalación y servidor como servicio en macOS (§1, §2). |
| **B** | Mac | Emparejamiento de terminales y conexión cifrada (§3). |
| **C** | Mac | Descubrimiento del servidor y comportamiento en red (§4). |
| **D** | Mac | API con versión y preparación para Android (§5). |
| **E** | Mac | Impresión con varias terminales (§6). |
| **F** | Mac | Respaldos programados (§7). |
| **G** | Mac | Regresión completa, documentación, instalación, fusión y push. |
| **W** | Windows | Servicio de Windows, instalador para todo el equipo, migración de instalaciones por usuario y pruebas cruzadas (§8). |

**Al terminar cada fase:** ejecuta sus pruebas, aplica el formato (sin tocar `docs/etapas/`), registra un resumen en `docs/ETAPA_9G_AVANCE.md`, haz un commit y continúa.

---

## 0-B. DECISIONES DEL PROPIETARIO

**Si una sección parece decir otra cosa, prevalece esta.**

- **Una sola computadora principal por restaurante.** Las demás son terminales: cajas, cocina y, después, teléfonos Android.
- **Tres modalidades de instalación,** elegidas en el instalador:
  - **Servidor y terminal:** la computadora principal, que además se usa como caja. Es el caso más común.
  - **Solo servidor:** una computadora apartada que nadie usa.
  - **Solo terminal:** no instala PostgreSQL ni el servidor.
- **El servidor arranca con la computadora** como servicio del sistema, sin que nadie inicie sesión ni abra la ventana. Cerrar la ventana no lo detiene.
- **Las terminales encuentran el servidor solas,** con la opción de escribir la dirección a mano.
- **Ninguna terminal se conecta sin emparejarse:** la terminal muestra un código y un administrador lo aprueba en la computadora principal. Así nadie sin acceso a esa computadora puede registrar una terminal.
- **Toda la comunicación por la red local va cifrada.**
- **Escritorio sin conexión no vende:** si una caja o la cocina pierde el servidor, muestra «Sin conexión», bloquea las operaciones y se reconecta sola. **Solo la aplicación Android** (en su etapa) podrá capturar sin conexión, con el catálogo guardado en el teléfono y una cola de pedidos que se envían al volver la señal.
- **Pedidos capturados sin conexión con cambios:** si al llegar al servidor cambió un precio o un producto se agotó, **el servidor no acepta el pedido en silencio.** Responde qué cambió («Hamburguesa: $120 → $130», «Malteada de fresa: agotada») y el pedido se acepta solo cuando el mesero confirma los cambios. Nunca entra con un precio viejo ni con un producto agotado sin que el mesero lo vea.
- **El servidor es la única autoridad** en precios, impuestos, permisos y estados. Ningún cliente calcula ni decide reglas de negocio.
- **Licencias de comanderos por sesiones simultáneas** (paquetes de 3, 5 y 10). En esta etapa solo se **cuentan** las sesiones activas por tipo de dispositivo; no se limita nada todavía.
- **Respaldos automáticos diarios,** conservando los últimos 30, configurable.

---

## 1. FASE 0 — VERIFICACIÓN

Registra en el avance:

1. Cómo se conecta hoy la aplicación de escritorio al servidor:
   - Dirección fija `127.0.0.1` o configurable.
   - Puerto.
   - Autenticación (sesiones, tokens, CSRF).
   - Uso de eventos en tiempo real (SSE).
2. Qué existe ya de **terminales**: registro, nombre, configuración por terminal (pantalla de inicio, estaciones de la 9F, impresoras) y cómo se identifica hoy una terminal.
3. Cómo arranca hoy el servidor en macOS. ¿Ya sigue activo al cerrar la ventana? ¿Arranca al encender la Mac sin iniciar sesión? ¿Cómo se instala: arrastrando a Aplicaciones, `.pkg` o registrado como servicio?
4. Cómo maneja hoy el servidor la **concurrencia**: dos terminales modificando la misma cuenta, versiones o bloqueo optimista, y qué ve cada una.
5. Qué existe de **disponibilidad de productos** («agotado»). Si no existe, se agrega en la Fase D.
6. Si las operaciones que crean o modifican pedidos aceptan ya una **clave de idempotencia**.
7. Cómo se imprime hoy en impresoras USB de la propia computadora, y qué significa «agente de impresión aún no implementado».
8. Qué existe de respaldos: manuales, ubicación y verificación.

Con eso define el plan detallado de las fases A a F y continúa.

---

## 2. FASE A — MODALIDADES DE INSTALACIÓN Y SERVIDOR COMO SERVICIO (MACOS)

### 2.1 Separar el servidor de la ventana

- El servidor Go (con PostgreSQL) y la aplicación de escritorio son **procesos independientes**. La ventana es un cliente más, igual que una terminal remota.
- **Cliente configurable:**
  - En la computadora principal, la aplicación se conecta al servidor local.
  - En una terminal, se conecta al servidor emparejado (§3).

### 2.2 Servicio en macOS

- El servidor arranca **al encender la Mac, sin iniciar sesión**, como servicio del sistema (`LaunchDaemon`). Usa la vía recomendada actual de Apple, por ejemplo `SMAppService`. Si hace falta un instalador `.pkg` para registrarlo, genéralo.
- Si el servicio se cae, se reinicia solo.
- Los datos quedan en una carpeta del sistema, no en la del usuario. Si hoy están en la carpeta del usuario, **muévelos automáticamente en la actualización**, con respaldo previo y verificación.
- Desde la aplicación, el administrador puede ver el estado del servicio y reiniciarlo (en Mantenimiento, sin tecnicismos).

### 2.3 Modalidades en el instalador y en el primer arranque

- Al instalar o en el primer arranque se elige: **Servidor y terminal**, **Solo servidor** o **Solo terminal**.
- **Solo terminal** no instala ni inicia PostgreSQL ni el servidor, y pasa directo a buscar el servidor (§4).
- **Solo servidor** no abre la interfaz de venta. Solo muestra una pequeña pantalla de estado: servidor activo, terminales conectadas, último respaldo y acceso a Administración para un administrador.
- **Cambiar la modalidad después** es posible desde Mantenimiento, con confirmación. Nunca borra la base de datos: si una computadora deja de ser servidor, sus datos se conservan y se avisa.

---

## 3. FASE B — EMPAREJAMIENTO Y CONEXIÓN CIFRADA

### 3.1 Cifrado

- El servidor genera, al instalarse, **su propio certificado** para la red local. Todas las conexiones desde otras computadoras o teléfonos van por **HTTPS**.
- Las conexiones locales dentro de la misma computadora pueden seguir sin cifrar solo si se limitan a `127.0.0.1`.
- **Cada terminal fija el certificado del servidor durante el emparejamiento** (*pinning*). Si después el certificado cambia, la terminal no se conecta y avisa, en lugar de aceptar cualquier certificado.
- Los eventos en tiempo real (SSE) van por la misma conexión cifrada.

### 3.2 Emparejamiento con código

1. Una terminal nueva encuentra el servidor (§4) y envía una solicitud. En su pantalla aparece un **código de 6 dígitos** y el nombre propuesto («Caja 2», editable).
2. En la computadora principal, en **Administración › Terminales**, aparece la solicitud pendiente con su código, el sistema operativo y la hora. **Un usuario con permiso de administrar terminales** compara el código y la aprueba. También puede rechazarla.
3. El código se vincula criptográficamente a la conexión y al certificado, para que nadie en medio pueda suplantar al servidor ni a la terminal.
4. La terminal recibe una **credencial de dispositivo** propia, guardada en el almacén seguro del sistema (Keychain o Administrador de credenciales).
5. **Los códigos vencen** a los pocos minutos, y hay un límite de intentos.

### 3.3 Administración de terminales

En **Administración › Terminales**:

- Lista de terminales con nombre, tipo (escritorio, y en el futuro Android o cocina), sistema, última conexión, usuario actual y estado (en línea o desconectada).
- **Renombrar**, **revocar** (se desconecta en ese momento y debe emparejarse de nuevo) y **ver su configuración**: pantalla de inicio, estaciones, impresoras.
- Todo queda en el Registro de movimientos.

### 3.4 Sesiones

- **Inicio de sesión:** el usuario inicia sesión en una terminal emparejada. El servidor valida al usuario **y** al dispositivo, y el Registro de movimientos guarda los dos.
- **Conteo de sesiones activas por tipo de dispositivo** (escritorio, comandero Android, cocina), visible para el administrador. **No se limita nada todavía:** queda listo para conectarlo a las licencias de la etapa 10.

---

## 4. FASE C — DESCUBRIMIENTO Y COMPORTAMIENTO EN RED

### 4.1 Encontrar el servidor

- El servidor se **anuncia en la red local** (mDNS / DNS-SD, por ejemplo `_adelpos._tcp`) con el nombre del establecimiento.
- La terminal muestra los servidores encontrados («Fonda Doña Ñeli — Mac de caja») y se elige uno.
- **Opción manual:** escribir la dirección IP o el nombre del equipo, para redes que bloquean el descubrimiento.
- Si la dirección del servidor cambia (por DHCP), la terminal **lo vuelve a encontrar sola** por su identidad y su certificado, sin emparejarse de nuevo. Recomienda en la guía asignar una IP fija a la computadora principal en el módem.

### 4.2 Conexión perdida

- **Terminal de escritorio sin servidor:** aviso claro «Sin conexión con el servidor», operaciones bloqueadas y reconexión automática con espera creciente.
- **Lo capturado y no guardado** en la pantalla no se pierde mientras la ventana siga abierta.
- **Al reconectar,** la terminal **vuelve a consultar el estado actual** (mesas, cuentas, cocina) y no depende de haber recibido todos los eventos en tiempo real. Si el SSE permite reanudar (`Last-Event-ID`), úsalo, pero la consulta completa es la garantía.

### 4.3 Varias terminales a la vez

- Verifica y prueba que los cambios se propagan a todas las terminales: mesas ocupadas o liberadas, cuentas, envíos a cocina, cobros, turnos de caja y estados de preparación.
- **Dos terminales modificando la misma cuenta:** ninguna sobrescribe en silencio la operación de la otra. La segunda recibe un aviso claro y la vista actualizada.
- **Cada caja tiene su propio turno.** El arqueo, la pausa de caja y el tope funcionan por terminal, como se diseñó.

---

## 5. FASE D — API CON VERSIÓN Y PREPARACIÓN PARA ANDROID

### 5.1 Contrato de la API

- La API para clientes se publica bajo una **versión** (por ejemplo `/api/v1`), con un **contrato documentado** (OpenAPI) en el repositorio. Ese documento es el que usará la etapa de Android.
- El servidor informa su versión y **la versión mínima de cliente que acepta.**
  - Una terminal o un teléfono demasiado viejo recibe «Actualiza esta terminal» en lugar de errores raros.
  - Un servidor más viejo que el cliente también se detecta y se avisa.
- **Pruebas de contrato:** un cambio que rompa la v1 hace fallar las pruebas.

### 5.2 Operaciones sin duplicados (idempotencia)

- **Las operaciones que crean o cambian pedidos, cuentas o pagos** aceptan una **clave de operación** generada por el cliente.
- **Si llega dos veces la misma clave** (el teléfono reintentó porque perdió la respuesta), el servidor **devuelve el mismo resultado sin repetir la operación**.
- **Las claves se conservan** al menos 7 días.
- **Pruebas:**
  - Reintentos simultáneos.
  - Reintento después de una respuesta perdida.
  - La misma clave con contenido distinto, que debe rechazarse.

### 5.3 Catálogo descargable por versión

- El catálogo (categorías, productos, precios con IVA, complementos, combos, notas rápidas, porciones, disponibilidad e imágenes) tiene un **número de versión** que aumenta con cada cambio.
- Hay dos formas de descargarlo:
  - **Completo**, para la primera descarga.
  - **Solo los cambios desde la versión X**, para las actualizaciones.
- **Las imágenes se descargan aparte,** con caché, para no inflar cada sincronización.

### 5.4 Disponibilidad («agotado»)

Si no existe (Fase 0, punto 5), agrega la **disponibilidad de productos**:

- **Desde la captura en el punto de venta,** con un toque: «Agotado» y «Disponible otra vez».
- **Con permiso propio** y registrado en el Registro de movimientos.
- **Un producto agotado** no se puede capturar, aparece marcado en el catálogo y se propaga en tiempo real a todas las terminales.
- **Se reinicia sola al cambiar de día** (con el corte de las 05:00), a menos que se marque como agotado indefinido.

### 5.5 Pedidos capturados sin conexión: validación

Para la futura aplicación Android:

- El cliente envía el pedido con la **clave de operación**, la **versión de catálogo** con la que se capturó y los **precios que vio** el mesero.
- **Si nada cambió,** el servidor lo acepta normalmente.
- **Si cambió algo** (precio, producto agotado o eliminado, complemento no disponible, mesa ya cobrada o cerrada), el servidor **no lo acepta** y responde con la **lista de cambios**: producto, antes, ahora y motivo. El pedido queda «requiere confirmación».
- El cliente lo vuelve a enviar **con la confirmación del mesero** y la versión de catálogo actual. Solo entonces se acepta, siempre con los precios del servidor.
- **Registro de movimientos:** guarda que el pedido se capturó sin conexión, cuándo se capturó y cuándo llegó, y qué cambios aceptó el mesero.
- **Pruebas de servidor** que simulen un cliente sin conexión:
  - Cambio de precio.
  - Producto agotado.
  - Mesa cerrada mientras tanto.
  - Doble envío.
  - Confirmación con un catálogo todavía viejo, que debe rechazarse de nuevo.

### 5.6 Recuperación del estado

Expón las consultas que necesita un cliente al reconectarse:

- Las mesas y cuentas activas, con su versión.
- Los estados de preparación de sus pedidos.
- Los permisos actuales del usuario.

Si se le **revocó un permiso o la terminal**, el cliente se entera al reconectar.

---

## 6. FASE E — IMPRESIÓN CON VARIAS TERMINALES

- **Impresoras de red:** las usa el servidor directamente, como hoy.
- **Impresoras USB conectadas a una terminal** (por ejemplo, la impresora de la Caja 2): la aplicación de esa terminal funciona como **agente de impresión**:
  - Recibe del servidor los trabajos destinados a sus impresoras locales y los imprime: CUPS en Mac, cola de impresión en Windows, como en la 9E.
  - Abre el cajón de dinero de su caja.
  - Confirma al servidor si se imprimió o no.

  Esto reemplaza el estado «requiere un agente de impresión aún no implementado».
- **Si la terminal con la impresora está apagada,** el trabajo queda pendiente y se avisa en la cola de impresión con un texto claro («La Caja 2 está desconectada»).
- **En Administración › Terminales,** cada terminal muestra sus impresoras locales, y al asignar impresoras a documentos se ve en qué terminal está cada una.

---

## 7. FASE F — RESPALDOS PROGRAMADOS

- **Respaldo automático diario** hecho por el servidor.
  - **Hora:** la del corte del día (05:00) por defecto, configurable.
  - **Cuántos conservar:** 30 por defecto, configurable. Los más viejos se borran solos.
- **Carpeta de destino configurable,** que puede estar en otro disco o en una USB. Si no está disponible, se avisa y se usa la carpeta predeterminada.
- **Cada respaldo se verifica** después de crearse (por ejemplo, con `pg_restore --list`). Si falla, se avisa al administrador en la pantalla de inicio, no solo en la bitácora.
- **El respaldo manual sigue existiendo.**
- **En el estado del sistema:** último respaldo, si fue correcto y dónde está.
- **Restaurar** sigue siendo una acción del administrador, con confirmación, como hoy.

---

## 8. FASE W — WINDOWS (SE EJECUTA EN LA COMPUTADORA CON WINDOWS 11)

Al iniciar en Windows: `git pull`, lee `docs/ETAPA_9G_AVANCE.md` y continúa desde aquí.

### 8.1 Servicio e instalador

- **Servidor como servicio de Windows,** con arranque automático al encender, sin iniciar sesión, y reinicio automático si se cae.
- **Instalador para todo el equipo,** en `Archivos de programa`, con permisos de administrador y datos en `%ProgramData%\AD-EL POS\`.
  - El instalador pregunta la **modalidad** (§2.3). «Solo terminal» no instala PostgreSQL ni el servicio.
  - Crea la **regla del firewall** para el puerto del servidor, solo para redes privadas, cuando la modalidad incluye servidor.
- **Migración desde la instalación por usuario** de la 9E: el nuevo instalador detecta la instalación anterior en `AppData`, hace un respaldo, mueve los datos a `ProgramData`, verifica y desinstala la versión por usuario. Nunca borra los datos sin haber verificado la copia.
- **Desinstalar** deja los datos, como antes.
- Actualiza `scripts\build-windows.ps1`.

### 8.2 Pruebas cruzadas

El propietario tendrá las dos computadoras en la misma red. Coordínalo con él y explícale con claridad qué hacer en la Mac en cada paso.

1. **Mac como servidor, Windows como terminal:**
   - Descubrimiento y emparejamiento con código.
   - Mesas, captura, envío a cocina y cobro desde Windows, con los cambios vistos al momento en la Mac y al revés.
   - Dos terminales en la misma cuenta.
   - Apagar la red: aviso y reconexión.
   - Revocar la terminal.
   - Impresión desde Windows a una impresora de red, y a su USB si la hay.
2. **Windows como servidor, Mac como terminal:** lo mismo en sentido contrario. Desde esta sesión controlas el servidor. Las acciones en la Mac las hace el propietario siguiendo una lista que le das.
3. **Reinicio de la computadora principal:** el servicio arranca solo y las terminales se reconectan solas. Si requiere reiniciar Windows, avisa antes, según §0.
4. **Prueba de carga moderada:** simula por la API varias terminales y clientes tipo comandero haciendo operaciones simultáneas durante unos minutos. Registra tiempos de respuesta y verifica que no haya duplicados ni inconsistencias.

Registra los resultados en el avance, fusiona en `main` y haz push.

---

## 9. PRUEBAS (FASES A A G)

- Servicio: el servidor sigue activo al cerrar la ventana y arranca sin iniciar sesión (verificable en la Mac).
- Modalidades: «Solo terminal» sin PostgreSQL; cambio de modalidad sin pérdida de datos.
- Emparejamiento:
  - Código correcto e incorrecto.
  - Código vencido.
  - Límite de intentos.
  - Revocación inmediata.
  - Certificado cambiado (la terminal no se conecta).
- Descubrimiento, conexión manual y cambio de IP.
- Reconexión y recuperación del estado.
- Dos terminales en la misma cuenta.
- API: contrato v1, versión mínima de cliente, idempotencia (§5.2), catálogo completo y por cambios, disponibilidad y validación de pedidos sin conexión (§5.5).
- Agente de impresión: trabajo para una terminal conectada y desconectada.
- Respaldos: programación, retención, verificación y aviso de fallo.
- Conteo de sesiones por tipo de dispositivo.
- Regresión completa en la Fase G. Como en esta parte no hay Windows, compila el código Go para Windows (`GOOS=windows go build ./...`).

---

## 10. DOCUMENTACIÓN Y ENTREGA

- Actualiza `CLAUDE.md`:
  - Versión 0.10.0 y esquema nuevo.
  - Arquitectura servidor/terminales.
  - Modalidades.
  - Emparejamiento y cifrado.
  - Idempotencia y validación de pedidos sin conexión.
  - Agente de impresión.
  - Respaldos.
- Crea:
  - **`docs/API_V1.md`** con el contrato OpenAPI, para la etapa de Android.
  - **`docs/INSTALAR_EN_RED.md`**, guía corta y sin tecnicismos para el propietario:
    1. Instalar la computadora principal y asignarle una IP fija en el módem.
    2. Instalar una terminal y emparejarla con el código.
    3. Qué hacer si una terminal no encuentra el servidor.
    4. Cómo revocar una terminal perdida.
    5. Dónde están los respaldos y cómo cambiar su carpeta.
- Actualiza `docs/NOTAS_WINDOWS.md` y `docs/INSTALAR_EN_WINDOWS.md` en la Fase W.
- **Entrega final de la parte Mac:**
  - Cambios por fase.
  - Pruebas.
  - Migración.
  - Qué falta para la Fase W.
- **Entrega final de la Fase W:**
  - Resultado de las pruebas cruzadas.
  - Prueba de carga.
  - Migración de la instalación por usuario.
  - Limitaciones.
