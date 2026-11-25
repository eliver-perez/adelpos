# AD-EL POS — ETAPA K1: APLICACIÓN DE COCINA (KDS) PARA ANDROID

Se ejecuta **en la Mac**, sobre `main`, **después de la L1** (licencias).

- **Parte de:** servidor 0.15.0 (esquema 28) y aplicación de meseros 0.4.0, con la L1 de licencias fusionada. Confirma los valores reales en `config/product.json`, `CLAUDE.md` y `android/CLAUDE.md`.
- **Al terminar:**
  - servidor **0.16.0**;
  - aplicación de cocina **0.1.0** (versionCode 1), `com.adelpos.cocina`;
  - aplicación de meseros **0.4.1** (usa el módulo compartido de §1).

**Contenido:**

- una aplicación Android que funciona **solo como pantalla de cocina**, para tabletas en horizontal, y que se conecta a la computadora principal;
- la corrección del aviso falso «Reconectando» en el escritorio (§7).

**El modo Cocina del escritorio no se toca en esta etapa.** No se agregan funciones fuera de lo que dice este documento.

---

## 0. FORMA DE TRABAJO

### Rama y especificaciones

- Rama **`etapa-k1`** desde `main`. Las especificaciones en `docs/etapas/` son de solo lectura.
- Lee este documento, `CLAUDE.md`, `android/CLAUDE.md`, `docs/API_V1.md`, `docs/licencias/INTEGRACION.md` y lo que dejó la 9F sobre estaciones y el modo Cocina.
- De la L1, revisa en `docs/ETAPA_L1_AVANCE.md` las decisiones sobre el cupo de cocina (L1-D7, L1-D13, L1-D28 y L1-D36) y la versión de Android en Terminales (L1-D64).

### Eficiencia

- La Fase 0 la hace **un solo agente**.
- **Máximo 2 agentes en total trabajando al mismo tiempo. Los subagentes no pueden lanzar otros subagentes.**
- En cada fase, solo las pruebas de lo que cambió. La regresión completa, **una sola vez**, en la Fase F. Si una corrección de la regresión toca `backend/`, `config/` o código compartido, repite la regresión completa; si no, solo las suites afectadas.
- **Revisor independiente** solo para el emparejamiento como pantalla de cocina y la autorización de sus rutas de la API: tocan la seguridad.
- **Emuladores:** `Pixel_Tablet` y `ADEL_A1_Tablet_API26`, en horizontal. Los cuatro solo si el propietario lo pide antes de una entrega.
- **Capturas de pantalla:** solo de las pantallas nuevas o cambiadas.

### Autonomía

Aplica la regla de autonomía de `CLAUDE.md` §0 y la protección contra el servicio real de la A1 a la L1. Ninguna prueba llama a `licensing.sicrat.com`.

**Paradas:** instalar el `.pkg` (contraseña de administrador).

### Fases

| Fase | Contenido |
| ---- | --------- |
| **0** | Verificación y estructura del proyecto. |
| **A** | Servidor: dispositivo de cocina, estaciones, API y eventos (§2, §3). |
| **B** | Módulo compartido y proyecto de la aplicación de cocina (§1). |
| **C** | Pantalla de pedidos (§4). |
| **D** | Conexión tolerante, alertas y pantalla siempre encendida (§5). |
| **E** | Configuración desde Terminales y licencia (§6), y el aviso «Reconectando» del escritorio (§7). |
| **F** | Regresión completa, documentación, APK, `.pkg`, fusión y push (§9). |

**Al terminar cada fase:** ejecuta sus pruebas, registra un resumen en `docs/ETAPA_K1_AVANCE.md`, haz un commit y continúa.

### Fase 0: verificación

1. Revisa en el código actual:
   - el KDS de la 9F: qué rutas, eventos y tablas usa para pedidos por estación y «listo»;
   - el aviso de platillo listo de la A2 (cómo llega al mesero);
   - el emparejamiento de Android, el tipo de dispositivo y el estado de conexión tolerante de la A3;
   - si las estaciones ya permiten elegir impresora, pantalla o ambas.
2. Decide la estructura del proyecto (§1) y anótala en el avance antes de la Fase B.

---

## 1. ESTRUCTURA DEL PROYECTO

El propietario eligió **carpetas separadas con código compartido**, para no mezclar los proyectos:

```
android/        ← aplicación de meseros (como está)
cocina/         ← aplicación de cocina, proyecto Gradle propio
android-core/   ← lo compartido por las dos
```

- **`cocina/`** es su propio proyecto Gradle, en la raíz del repositorio. No contiene nada de la aplicación de meseros.
- **`android-core/`** tiene lo que usan las dos aplicaciones: emparejamiento, TLS con certificado fijado, cliente de la API v1, canal de eventos y el **estado de conexión tolerante** de la A3. Las dos aplicaciones lo incluyen (por ejemplo, como *composite build* con `includeBuild`). Decide el mecanismo en la Fase 0 y anótalo.
  - Lo compartido se **mueve** desde `android/` a `android-core/` **sin cambiar su comportamiento**. Es el único cambio de la aplicación de meseros en esta etapa: sus pruebas existentes deben pasar igual antes y después.
  - Lo que es solo de meseros (PIN, mesas, captura, cola de pedidos) se queda en `android/`.
  - No quedan dos copias del mismo código.
- Los guiones de compilación y de pruebas (`scripts/android-e2e.sh`, `dev.sh`, `sync-check`, formato, Lint) cubren las tres carpetas.
- `com.adelpos.cocina`, minSdk 26, targetSdk 36, firmada con **la misma llave de desarrollo** de la aplicación de meseros.
- Nombre visible: **«AD-EL POS Cocina»**, con su propio ícono, distinguible del de meseros.
- Agrega `cocina/CLAUDE.md` y `android-core/CLAUDE.md` con las reglas de cada carpeta, y actualiza `android/CLAUDE.md`.

---

## 2. SERVIDOR: DISPOSITIVO DE COCINA

- **Emparejamiento:** el mismo flujo de código de 6 dígitos, TLS y certificado fijado, pero el dispositivo queda registrado como **pantalla de cocina**.
- **Sin PIN:** la pantalla de cocina queda siempre abierta en modo cocina, con una sesión de dispositivo que solo tiene permisos de cocina:
  - ver los pedidos de sus estaciones;
  - marcar platillos y pedidos como listos y recuperarlos.
  - **Nada más:** ni capturar, ni cobrar, ni administración. El servidor lo rechaza aunque se intente.
- **Versión instalada:** hoy Go guarda la versión de Android solo al emparejar (L1-D64), y Terminales muestra la versión con que se emparejó. Corrígelo para las dos aplicaciones: cada dispositivo informa su versión al conectarse y Terminales muestra **la instalada**.
- **Revocar el dispositivo** desde Terminales lo desconecta de inmediato y lo regresa a la pantalla de emparejamiento.
- **Cupo `kitchen_displays`** de la licencia (L1): se ocupa al conectarse la pantalla de cocina y se libera al revocarla o con «Liberar cupo» en Terminales. Una pantalla desconectada por la red **no** libera el cupo. Usa el mismo mecanismo de cupos de la L1 (asignación atómica y su candado), no uno nuevo.
- **Estados de licencia** (§6 del contrato): en «Vencida» o «Revocada» la pantalla sigue mostrando lo ya enviado y se puede marcar como listo. En «Sin activar» no muestra pedidos.

---

## 3. SERVIDOR: ESTACIONES, API Y EVENTOS

- **Estaciones por pantalla:** cada pantalla de cocina muestra una o varias estaciones (cocina, barra, parrilla…), elegidas en Terminales.
- **Envío por estación:** cada estación decide si sus comandas van a **impresora, pantalla o ambas**. Si la 9F ya lo permite, solo se verifica; si no, se agrega en la configuración de estaciones. La impresión de comandas sigue funcionando igual que hoy.
- **API v1** para la pantalla de cocina, documentada en `docs/API_V1.md`:
  - pedidos activos de sus estaciones;
  - marcar un platillo como listo;
  - marcar el pedido completo como listo;
  - recuperar un platillo o pedido marcado por error;
  - lista de los recientemente marcados, para recuperar.
- **Eventos en tiempo real** por el canal que ya existe: pedido nuevo, platillos agregados, cancelaciones, cambios y marcas de listo hechas desde otra pantalla.
- **Marcar listo dispara el aviso al mesero** de la A2, igual que hoy. Recuperar un platillo **no** manda otro aviso de «listo»; si se vuelve a marcar, sí.
- **Reutiliza la lógica del KDS de la 9F** en el backend. Las reglas de negocio no se duplican.
- **Reconexión:** al reconectarse, la pantalla pide el estado completo. Ningún pedido se pierde ni aparece duplicado.

---

## 4. PANTALLA DE PEDIDOS

- **Horizontal, pantalla completa**, tema oscuro por omisión (cocina), con opción de tema claro.
- **Tarjetas de pedido**, del más antiguo al más nuevo:
  - **Encabezado:** mesa (o «Para llevar» con el nombre del cliente), mesero, hora de envío y **tiempo de espera** que avanza en vivo.
  - **Platillos:** cantidad y nombre en letra grande, con sus modificadores y notas debajo, bien visibles.
  - **Cancelaciones:** el platillo cancelado se ve tachado y marcado como **«CANCELADO»**, sin desaparecer hasta que se marque el pedido.
  - **Agregados después del envío:** resaltados como **«NUEVO»**.
- **Color por tiempo de espera:** normal, de atención y de retraso. Los límites se configuran en la computadora principal (por omisión: 10 y 20 minutos).
- **Marcar listo:**
  - Tocar un platillo lo marca como listo (con una confirmación visual clara) y lo atenúa.
  - Un botón grande en la tarjeta marca **el pedido completo**, que desaparece de la pantalla.
  - Cuando todos los platillos de un pedido están listos, el pedido se marca completo solo.
- **Recuperar:** un botón «Recientes» muestra los últimos pedidos y platillos marcados (por ejemplo, los de los últimos 30 minutos) para devolverlos a la pantalla.
- **Muchos pedidos:** las tarjetas se acomodan en columnas y la pantalla se desplaza. Un indicador dice cuántos pedidos hay fuera de la vista.
- **Tamaño de letra** por pantalla (normal o grande), elegido en Terminales.
- **Regla de textos:** ningún texto provisional, de desarrollo ni identificadores internos.

---

## 5. CONEXIÓN, ALERTAS Y PANTALLA

### 5.1 Conexión tolerante

**El propietario pidió expresamente que una desconexión de un segundo no muestre ningún mensaje.**

- Usa el **mismo estado de conexión de la A3** (módulo compartido):
  - un fallo aislado no cambia nada visible;
  - se declara «Sin conexión» solo tras varios fallos seguidos durante unos 20 segundos, o si Android informa que no hay red;
  - vuelve a «En línea» con el primer éxito y pide el estado completo.
- **Mientras se reconecta dentro de la tolerancia,** la pantalla sigue mostrando los pedidos sin avisos. Una marca de listo hecha en ese momento se reintenta sola en silencio.
- **Ya declarado «Sin conexión»:**
  - un aviso **grande y claro** en la parte superior: «Sin conexión con la computadora principal. Reconectando…»;
  - no se puede marcar nada, y la pantalla lo deja claro;
  - se reconecta sola, sin que nadie toque la tableta.

### 5.2 Alertas

- **Sonido** al llegar un pedido nuevo, y otro distinto para cancelaciones y cambios. El volumen y el sonido se pueden desactivar desde Terminales.
- Una animación breve en la tarjeta nueva, para quien no escuchó.

### 5.3 Pantalla

- **La pantalla nunca se apaga** mientras la aplicación está abierta.
- **Arranca sola** en la pantalla de pedidos al abrir la aplicación; si se cierra o la tableta se reinicia, al abrirla vuelve directo, sin pasos.
- Documenta en `docs/COCINA_INSTALAR.md` cómo fijar la aplicación en la tableta (fijar pantalla de Android) para que el personal no salga de ella.

### 5.4 Ajustes en la tableta

- Escondidos detrás de una pulsación larga en el logotipo. Piden la **contraseña de un administrador** y validan sus permisos con el servidor.
- Solo muestran: la dirección de la computadora principal, el estado de la conexión, la versión y «Volver a emparejar».

---

## 6. CONFIGURACIÓN DESDE TERMINALES (ESCRITORIO)

- En **Terminales**, una pantalla de cocina emparejada muestra y permite cambiar:
  - las estaciones que muestra;
  - el tamaño de letra y el tema;
  - el sonido;
  - el estado de conexión, la versión instalada y el cupo de cocina que ocupa;
  - «Revocar».
- **Límites de color por tiempo** (10 y 20 minutos por omisión): en la configuración general de cocina, comunes a todas las pantallas.
- **Envío por estación** (impresora, pantalla o ambas): en la configuración de estaciones.
- Los cambios llegan a la tableta al momento, sin reiniciarla.
- **Sin cupo de cocina** al emparejar o conectar: la tableta muestra «No hay cupo de pantalla de cocina libre (N de N en uso). Revoca otra pantalla en Terminales o amplía tu licencia.».

---

## 7. ESCRITORIO: AVISO FALSO «RECONECTANDO»

**Problema (visto por el propietario el 24/11, con la 0.15.0 en la Mac):** con la computadora en línea y sin ningún problema de red, más o menos **cada 30 segundos** aparece arriba el recuadro **«Reconectando»** durante 1 o 2 segundos y se quita solo. Pasa en cualquier pantalla (se vio en Configuración › Licencia). Hace ruido y alarma al usuario, que cree que el sistema se está desconectando.

**Comportamiento esperado:**

1. **Corrige la causa, no solo el aviso.** Investígala igual que en la A3 con Android, donde la causa fue una conexión reutilizada que Go ya había cerrado. Hay que revisar:
   - el canal de eventos (SSE) del escritorio: si Go, el puente de Tauri o el cliente lo cierran por un tiempo de espera (`WriteTimeout`, `IdleTimeout`, el pulso de mantenimiento);
   - la revisión periódica del estado;
   - si el ciclo de unos 30 segundos coincide con algún valor configurado.
   Anota la causa en el avance.
2. **La misma tolerancia de la A3:**
   - una reconexión breve que se recupera sola **no muestra nada**;
   - «Reconectando» o «Sin conexión» aparecen solo tras varios fallos seguidos durante unos 20 segundos, o si el sistema informa que no hay red;
   - vuelve a «En línea» con el primer éxito;
   - el indicador y los avisos de cada pantalla leen un solo estado y nunca se contradicen.
3. **Mientras se reconecta dentro de la tolerancia,** los eventos perdidos se recuperan al reabrir el canal (estado completo o desde el último evento recibido), sin pedidos ni avisos perdidos.
4. **Aplica también a las terminales de escritorio** conectadas por red a la computadora principal, no solo a la principal.
5. Si la corrección toca Go (por ejemplo, los tiempos de espera o el pulso del canal de eventos), comprueba que Android y la aplicación de cocina no cambian de comportamiento.

---

## 8. PRUEBAS

- **Servidor:**
  - emparejar como pantalla de cocina;
  - la sesión de cocina no puede capturar, cobrar ni administrar;
  - filtrado por estación;
  - eventos de pedido nuevo, agregado, cancelación y marca hecha desde otra pantalla;
  - el aviso al mesero al marcar listo, y que recuperar no avise;
  - reconexión con estado completo, sin pérdidas ni duplicados;
  - cupo de cocina y estados de licencia;
  - envío por estación: impresora, pantalla o ambas.
- **Aplicación de cocina** en los dos emuladores, en horizontal:
  - la pantalla de pedidos con varios casos (mesa, para llevar, modificadores, notas, cancelados, nuevos);
  - los colores por tiempo con reloj simulado;
  - marcar platillo, marcar pedido, cierre automático del pedido y recuperar;
  - muchos pedidos;
  - los dos tamaños de letra y los dos temas.
- **Conexión:** cortes de 1 a 5 segundos **sin ningún aviso**; un corte largo muestra el aviso, impide marcar y se recupera solo; una marca hecha durante un corte corto se reintenta.
- **Aplicación de meseros:** sus pruebas pasan igual después de mover lo compartido a `android-core/`.
- **Versión instalada:** al actualizar una aplicación encima, Terminales muestra la versión nueva sin volver a emparejar.
- **Escritorio, «Reconectando»:**
  - con la computadora en línea, **10 minutos sin uso sin ningún aviso** de reconexión (con la causa corregida, no solo ocultada);
  - un corte de 1 a 5 segundos del canal de eventos no muestra nada y no pierde eventos;
  - un corte largo sí muestra el aviso y se recupera solo;
  - lo mismo en una terminal de escritorio conectada por red (Playwright o la prueba de terminales existente).
- **Una prueba de punta a punta:** un mesero envía un pedido desde la aplicación de meseros; aparece en la pantalla de cocina; se marca listo; el mesero recibe el aviso.

---

## 9. DOCUMENTACIÓN Y ENTREGA

- **Documentación:**
  - `docs/COCINA_INSTALAR.md`: instalar, emparejar, elegir estaciones, fijar la aplicación y qué hacer si no conecta.
  - Actualiza `CLAUDE.md`, `android/CLAUDE.md`, `docs/API_V1.md` y `docs/ANDROID_INSTALAR.md`.
- **Entregables:**
  - APK de cocina **0.1.0**.
  - APK de meseros **0.4.1**, con la misma llave, instalable encima de la 0.4.0 sin desemparejar.
  - `.pkg` **0.16.0** con su parada.
- Fusiona en `main` y haz push.
- **Entrega final breve:**
  - Cambios por fase.
  - Cómo quedó la estructura del proyecto (§1) y el mecanismo para compartir `android-core/`.
  - La causa del «Reconectando» falso y cómo se corrigió.
  - Pruebas.
  - Limitaciones.
