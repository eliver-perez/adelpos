# AD-EL POS ANDROID — ETAPA A2: PARA LLEVAR, IMPRIMIR PRECUENTA, AVISO DE PLATILLOS LISTOS E IDENTIFICADOR DEFINITIVO

Segunda etapa de la aplicación Android de meseros. Se ejecuta **en la Mac**, sobre `main`, después de la A1.

- **Parte de:** servidor 0.12.0 (esquema 25) y aplicación Android 0.1.0. Confirma los valores reales en `config/product.json`, `CLAUDE.md` y `android/CLAUDE.md`.
- **Al terminar:** aplicación **0.2.0** y, si cambia el servidor, servidor **0.13.0**.

**Contenido:**

1. **Identificador definitivo** de la aplicación: `com.adelpos.meseros`.
2. **Modo Para llevar.**
3. **Imprimir la precuenta** desde el dispositivo.
4. **Aviso de platillos listos**, que llega aunque la pantalla esté apagada o la aplicación cerrada.

Las reglas de la A1 siguen vigentes: el servidor es la única autoridad, la aplicación no cobra y nunca calcula reglas de negocio.

---

## 0. FORMA DE TRABAJO

### Rama y especificaciones

- Rama **`etapa-a2-android`** desde `main`. Las especificaciones en `docs/etapas/` son de solo lectura.
- Lee este documento, `CLAUDE.md`, `android/CLAUDE.md`, `docs/API_V1.md`, el contrato OpenAPI y `docs/ANDROID_INSTALAR.md`.

### Cambios en el servidor

- Solo los indispensables, en el contrato v1, de forma compatible y con pruebas de contrato.
- Si algo obliga a cambiar una decisión de negocio, **detente y pregunta**.

### Eficiencia

- La Fase 0 la hace **un solo agente**.
- **Máximo 2 agentes en total trabajando al mismo tiempo. Los subagentes no pueden lanzar otros subagentes.**
- En cada fase, solo las pruebas de lo que cambió; la regresión completa, una sola vez, en la Fase F.
- **Revisor independiente solo para el aviso en segundo plano (§4):** servicio en primer plano, reconexión y entrega sin duplicados ni pérdidas.

### Autonomía

Aplica la regla de autonomía de `CLAUDE.md` §0.

- **Las pruebas nunca usan el servicio real de la Mac.** Aplica la protección que dejó la A1 tras el incidente de la Fase F: tipo de anuncio de pruebas `_adelpostest._tcp`, wifi del emulador apagada si ve el anuncio real, y detenerse si la lista muestra cualquier servidor real.
- **Paradas:**
  - Actualizar el servicio de la Mac con el `.pkg`, si cambió el servidor (contraseña de administrador).
  - Conectar un dispositivo físico, si el propietario tiene uno.

### Fases

| Fase | Contenido |
| ---- | --------- |
| **0** | Verificación (§1). |
| **A** | Identificador definitivo (§2). |
| **B** | Modo Para llevar (§3). |
| **C** | Imprimir la precuenta (§5). |
| **D** | Aviso de platillos listos (§4). |
| **E** | Pruebas de punta a punta en los cuatro emuladores (§6). |
| **F** | Regresión completa, documentación, APK, `.pkg` si aplica, fusión y push. |

**Al terminar cada fase:** ejecuta sus pruebas, registra un resumen en `docs/ETAPA_A2_AVANCE.md`, haz un commit y continúa.

---

## 0-B. DECISIONES DEL PROPIETARIO

**Si una sección parece decir otra cosa, prevalece esta.**

- **Identificador de la aplicación:** **`com.adelpos.meseros`**, definitivo.
- **Para llevar funciona igual que en la caja, sin cobro:**
  - «Nuevo pedido» entra directo al catálogo y el pedido nace con el primer producto.
  - Nombre del cliente y teléfono son opcionales y se capturan en cualquier momento.
  - «Enviar a cocina» explícito, con la misma cola sin conexión.
  - Lista de pendientes con estado de preparación y de pago.
  - **No se entrega un pedido sin pagar**, igual que en la caja.
  - No hay mesa ficticia: el pedido se registra como para llevar en el servidor.
- **Precuenta:**
  - El mesero la imprime desde el dispositivo, en la impresora asignada a **ese** dispositivo en Administración › Sistema › Terminales.
  - El botón y el impreso usan el **nombre visible configurado**: Precuenta, Ticket de venta o Cuenta.
- **Aviso de platillos listos:**
  - **Llega aunque la pantalla esté apagada o la aplicación cerrada.** Para eso, Android exige una notificación permanente («AD-EL POS conectado») mientras corresponda (§4.3).
  - **Destinatario:** el **mesero asignado a la cuenta**, en el dispositivo donde tenga sesión abierta. Si no tiene sesión en ninguno, en todos los dispositivos donde ese mesero entró durante el turno o el día comercial en curso.
  - **Contenido:** solo mesa o pedido y platillo, sin precios ni datos de la cuenta («Mesa 4: 2 filetes listos», «Para llevar #12 (Juan): 1 hamburguesa lista»). Puede verse aunque el dispositivo esté en «¿Quién eres?».
  - **Al tocarlo,** abre la cuenta de esa mesa o el pedido, y pide el PIN si hace falta.
  - **Sonido y vibración** que se pueden apagar por dispositivo en Terminales. Los avisos de una misma mesa se agrupan.
  - **Solo funciona si la cocina usa el KDS** para marcar «Listo». Si no se usa, no hay avisos, y la función queda disponible para quien la quiera usar.

---

## 1. FASE 0 — VERIFICACIÓN

Registra en el avance:

1. **Para llevar en la API v1:** qué existe y qué falta para que un dispositivo:
   - Cree un pedido para llevar con creación diferida.
   - Capture nombre y teléfono.
   - Liste los pendientes con su preparación, pago y entrega.
   - Vea que la entrega exige pago.
2. **Impresión:**
   - Si el modelo de terminales de la 9G admite asignar impresora de precuenta a dispositivos `android`.
   - Cómo se pide hoy imprimir una precuenta y qué permiso exige.
   - Cómo se usa el nombre visible configurado (9F).
   - Qué pasa si la impresora está en una terminal desconectada (agente de impresión).
3. **Evento `restaurant.account.preparation_changed` (A1-D11):**
   - Si trae lo necesario para el aviso: cuenta, mesa o folio para llevar, mesero asignado, platillos y cantidades que pasaron a «Listo».
   - Si se puede reanudar sin perder eventos al reconectar.
4. **Mesero asignado:** cómo se determina hoy en cuentas de mesa y en pedidos para llevar, y qué pasa al transferir una cuenta a otro mesero.
5. **Servicio en primer plano** con `targetSdk` 36:
   - Tipo adecuado (por ejemplo `specialUse` o `dataSync`) y sus límites. Android 15 limita `dataSync` a 6 horas al día, y un turno puede durar más.
   - Permiso de notificaciones (Android 13 y posteriores).
   - Optimización de batería y Doze, y cómo convive con el WorkManager de la cola.
   - Recomienda el tipo y justifícalo.
6. **Cambio de identificador:** qué implica en Room, el Keystore, el emparejamiento, el ejecutor de pruebas, `scripts/android-e2e.sh` y la llave de firma de desarrollo (`~/.android/adel-pos-dev/`).

---

## 2. FASE A — IDENTIFICADOR DEFINITIVO

- Cambia el `applicationId` a **`com.adelpos.meseros`**. El espacio de nombres del código también, si conviene para la claridad. Usa una sola convención y documéntala.
- Es una aplicación «nueva» para Android: una instalación de la 0.1.0 (`local.adel.pos.android`) **no se actualiza**, queda como otra aplicación.
  - **En la guía,** explica que antes de instalar la 0.2.0 hay que **enviar los pedidos pendientes, desinstalar la 0.1.0 y revocarla** en Terminales. Hoy no hay dispositivos reales instalados, así que no hay datos que migrar entre aplicaciones.
- **La llave de firma de desarrollo sigue siendo la misma**, la de `~/.android/adel-pos-dev/`.
- Actualiza el ejecutor de pruebas, el guion de punta a punta y `android/CLAUDE.md`.
- **`versionCode` 2, `versionName` 0.2.0.**

---

## 3. FASE B — MODO PARA LLEVAR

### 3.1 Acceso

- El mesero cambia entre **Mesas** y **Para llevar** desde la navegación principal, siempre visible.
- Solo aparece con los permisos del servidor para pedidos para llevar. Documenta en la guía cuáles darle al rol Mesero.

### 3.2 Captura

- **«Nuevo pedido» entra directo al catálogo,** sin formularios previos. El pedido nace al guardar el primer producto. Sin conexión, nace al enviarse.
- **Nombre del cliente y teléfono** son opcionales, se editan en cualquier momento y se ven en la lista y en el aviso.
- **Mismos componentes que el modo Mesero:** catálogo, búsqueda, personalizador, pedido visible, modificar o cancelar según permisos, «Enviar a cocina» y estados de preparación. **No se duplica código.**
- **Cola sin conexión** con los mismos estados, reglas y claves de la A1, incluido «Requiere tu revisión».

### 3.3 Pendientes

- **Lista de pedidos para llevar abiertos:** folio, nombre si hay, tiempo, estado de preparación (pendiente, en preparación, listo) y **estado de pago** (sin pagar, pagado).
- **Entrega:** si el servidor permite marcar «Entregado» desde el dispositivo con permiso, **solo se ofrece con el pedido pagado**. Si no está pagado, se ve «Cobrar en caja antes de entregar». Si la entrega hoy solo se hace en caja, el dispositivo solo muestra el estado. Decídelo según la Fase 0 sin cambiar la regla de negocio.
- Se actualiza en tiempo real con conexión. Sin conexión, muestra lo último que se vio, con su hora.

---

## 4. FASE D — AVISO DE PLATILLOS LISTOS

### 4.1 Servidor

- Asegura que el evento de preparación, u otro nuevo compatible, permita al dispositivo saber **qué platillos de qué cuenta pasaron a «Listo»** y **quién es el mesero asignado**, sin exponer datos de cuentas a dispositivos o meseros que no deban verlos.
- Si el filtrado por destinatario conviene hacerlo en el servidor, hazlo ahí. Cada dispositivo recibe solo lo de los meseros que entraron en él durante el día comercial en curso.
- **Al reconectarse,** el dispositivo puede pedir los «listos» que no recibió desde su último evento, para que **ningún aviso se pierda** por un corte de red.
- **Ningún aviso se repite:** cada partida lista se avisa una sola vez por dispositivo.

### 4.2 Destinatario

- El **mesero asignado a la cuenta**, según el modelo actual. Si la cuenta se transfiere, avisa al nuevo.
- **Dispositivo:** donde el mesero tenga sesión abierta. Si no tiene sesión en ninguno, todos los dispositivos donde entró durante el día comercial en curso.
- **Para llevar:** el mesero que capturó o tiene asignado el pedido.

### 4.3 Recepción con la pantalla apagada

- **Servicio en primer plano** con su notificación permanente **«AD-EL POS conectado»**, del tipo recomendado en la Fase 0.
  - **Activo** mientras haya un mesero con sesión en el dispositivo o un mesero que entró en él durante el día comercial en curso.
  - **Se detiene** al cerrar el día comercial, al retirar el dispositivo o si nadie ha entrado en él en el día.
- **Mantiene la conexión de eventos** con el servidor y reconecta con espera creciente. Al volver, recupera los «listos» pendientes (§4.1).
- **Permisos:**
  - Pide el permiso de **notificaciones** (Android 13+), con una explicación clara en español.
  - Pide la **exclusión de la optimización de batería**, explicando por qué. Si el usuario la niega, la aplicación sigue funcionando y lo indica en una pantalla de ayuda.
- **Convive con el WorkManager de la cola** de la A1 sin duplicar envíos ni conexiones.

### 4.4 El aviso

- **Texto corto, sin precios:** «Mesa 4: 2 filetes listos», «Para llevar #12 (Juan): 1 hamburguesa lista».
- **Agrupado por mesa o pedido;** si se acumulan, un resumen («Mesa 4: 3 platillos listos»).
- **Sonido y vibración** del canal de notificaciones de la aplicación, con un canal propio «Platillos listos». Se pueden apagar por dispositivo en **Administración › Sistema › Terminales › Editar**. Android también deja ajustarlo en sus Ajustes.
- **Al tocarlo,** abre la cuenta o el pedido. Si el dispositivo está en «¿Quién eres?», pide el PIN y después abre.
- Si el mesero ya está viendo esa cuenta, se muestra un aviso dentro de la aplicación en lugar de la notificación del sistema.

### 4.5 Configuración

En **Terminales › Editar** de un dispositivo `android`:

- **«Avisar platillos listos»**, activado por defecto.
- **Sonido** (sí o no) y **vibración** (sí o no).

---

## 5. FASE C — IMPRIMIR LA PRECUENTA

- **En Administración › Sistema › Terminales › Editar** de un dispositivo `android`: **«Impresora de precuentas»**, para elegir entre las impresoras de red y las de terminales con agente de impresión. Si el modelo actual ya lo admite, solo exponlo.
- **En el dispositivo,** en la cuenta de una mesa o en un pedido para llevar: botón **«Imprimir [nombre visible]»** («Imprimir precuenta», «Imprimir ticket de venta» o «Imprimir cuenta», según la configuración de documentos).
  - Con el permiso que ya exige el servidor; documenta en la guía cuál darle al rol Mesero.
  - **Requiere conexión.** Sin conexión, lo dice: «Para imprimir necesitas conexión con la computadora principal».
- **Respuestas claras:**
  - «Enviada a Caja 1».
  - «Este dispositivo no tiene impresora asignada» (con aviso de pedírselo al administrador).
  - «La impresora no responde».
  - «La Caja 2 está desconectada».
  - Reintentar sin motivo si nunca salió en papel (regla de la 9F).
- **Reimprimir algo que ya salió en papel** sigue las reglas actuales de reimpresión.
- **La precuenta impresa** es la misma que genera la caja (misma plantilla y mismo renderizador), con «Atendió: <mesero>».

---

## 6. PRUEBAS

- **Unitarias (JVM)** de lo nuevo:
  - Destinatario y agrupación de avisos.
  - Avisos sin duplicados.
  - Reanudación tras corte de red.
  - Estados de Para llevar.
  - Mensajes de impresión.
- **Contra un Go temporal, en los cuatro emuladores** (Android 8, 13 y 17, teléfono y tableta):
  - **Para llevar:**
    - Nuevo pedido sin productos, que no deja nada.
    - Captura y envío en línea y sin conexión.
    - Nombre y teléfono después.
    - Lista con preparación y pago.
    - Entrega bloqueada sin pago.
  - **Precuenta:**
    - Asignar impresora en Terminales e imprimir con el nombre visible.
    - Sin impresora asignada.
    - Sin conexión.
    - Impresora de una terminal desconectada.
    - El trabajo llega con el contenido correcto, verificado en los bytes ESC/POS generados.
  - **Aviso:**
    - La cocina marca «Listo» en el KDS y llega al mesero asignado con la **pantalla apagada** y con la **aplicación cerrada**.
    - No llega a otro mesero ni a otro dispositivo.
    - Transferir la cuenta cambia el destinatario.
    - Corte de red durante el «Listo» y reconexión: llega una sola vez.
    - Agrupado por mesa.
    - Al tocarlo abre la cuenta y pide PIN si hace falta.
    - Sonido y vibración apagados desde Terminales.
    - El servicio en primer plano se detiene al retirar el dispositivo.
  - **Batería:** registra el consumo aproximado del servicio en una hora en el emulador, como referencia, aclarando que no es medición real.
- **Identificador:** la aplicación nueva instala, empareja y funciona. La guía explica el paso de la 0.1.0 a la 0.2.0.
- **Regresión completa** (servidor, escritorio y Android) en la Fase F.

---

## 7. DOCUMENTACIÓN Y ENTREGA

- **Actualiza `docs/ANDROID_INSTALAR.md`:**
  - Paso de la 0.1.0 a la 0.2.0 por el cambio de identificador.
  - Permisos del rol Mesero para Para llevar e impresión.
  - Impresora de precuentas por dispositivo.
  - Avisos: notificaciones y batería, qué significa la notificación permanente y cómo apagar sonido o vibración.
- **Actualiza `android/CLAUDE.md`, `CLAUDE.md` y `docs/API_V1.md`.**
- **Entregables:**
  - APK **0.2.0** firmado con la llave de desarrollo de siempre.
  - `.pkg` del servidor si cambió, con su parada.
- Fusiona en `main` y haz push.
- **Entrega final:**
  - Cambios por fase y cambios en el servidor.
  - Pruebas.
  - Tipo de servicio en primer plano elegido y por qué.
  - Consumo aproximado de batería.
  - Limitaciones: dispositivo físico, Doze real y wifi del restaurante.
