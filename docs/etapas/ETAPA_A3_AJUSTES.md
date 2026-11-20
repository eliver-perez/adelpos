# AD-EL POS — ETAPA A3: AJUSTES DE ANDROID, PERMISOS AGRUPADOS Y ROLES DE FÁBRICA

Se ejecuta **en la Mac**, sobre `main`.

- **Parte de:** servidor 0.13.0 (esquema 26) y aplicación Android 0.2.0. Confirma los valores reales en `config/product.json`, `CLAUDE.md` y `android/CLAUDE.md`.
- **Al terminar:** aplicación **0.3.0** y servidor **0.14.0**.

**Contenido:** observaciones del propietario al probar la A2 en los emuladores, la reorganización de la pantalla de permisos y los roles con permisos de fábrica. Son ajustes acotados: **no se agregan funciones nuevas fuera de lo que dice este documento**.

---

## 0. FORMA DE TRABAJO

### Rama y especificaciones

- Rama **`etapa-a3`** desde `main`. Las especificaciones en `docs/etapas/` son de solo lectura.
- Lee este documento, `CLAUDE.md`, `android/CLAUDE.md`, `docs/API_V1.md` y `docs/ANDROID_INSTALAR.md`.

### Eficiencia

- La Fase 0 la hace **un solo agente**.
- **Máximo 2 agentes en total trabajando al mismo tiempo. Los subagentes no pueden lanzar otros subagentes.**
- En cada fase, solo las pruebas de lo que cambió; la regresión completa, una sola vez, en la Fase E.
- **Sin revisor independiente,** salvo para los roles de fábrica y el restablecimiento de permisos (§3), porque tocan la seguridad.
- **Android en dos emuladores durante las fases** (`Pixel_Tablet` y `ADEL_A1_Phone_API33`); los cuatro solo en la regresión final.

### Autonomía

Aplica la regla de autonomía de `CLAUDE.md` §0 y la protección contra el servicio real de la A1 y la A2.

**Paradas:** instalar el `.pkg` (contraseña de administrador).

### Fases

| Fase | Contenido |
| ---- | --------- |
| **0** | Verificación. |
| **A** | Regla de textos y conexión tolerante (§1, §2). |
| **B** | Permisos agrupados y roles de fábrica (§3). |
| **C** | Ajustes de la aplicación Android (§4). |
| **D** | Ajustes de Terminales en el escritorio (§5). |
| **E** | Regresión completa, documentación, APK, `.pkg`, fusión y push. |

**Al terminar cada fase:** ejecuta sus pruebas, registra un resumen en `docs/ETAPA_A3_AVANCE.md`, haz un commit y continúa.

---

## 1. REGLA GENERAL DE TEXTOS

- **Ningún texto provisional o de desarrollo en la interfaz,** ni en el escritorio ni en Android. Por ejemplo:
  - «Solo se cuentan; todavía no se limita ninguna» en Sesiones activas.
  - Notas de etapas futuras.
  - «Todavía no…».
  - Identificadores internos.
- **Revisa ambas aplicaciones completas** y quita o reescribe esos textos.
- **Agrega la regla** a `CLAUDE.md` y `android/CLAUDE.md`, con una prueba o comprobación sencilla que busque frases típicas en los textos de la interfaz: «todavía no», «etapa», «próximamente», «provisional».

---

## 2. CONEXIÓN TOLERANTE EN ANDROID

**Problema:** con la tableta abierta y sin usar, aparece varias veces «Sin conexión con la computadora principal», y al mismo tiempo la barra superior dice «En línea» (captura del propietario: los dos estados a la vez).

**Comportamiento esperado:**

1. **Un solo estado de conexión** en toda la aplicación. El indicador de la barra y los avisos de cada pantalla leen el mismo estado y nunca se contradicen.
2. **Tolerancia:**
   - Un fallo aislado no cambia a «Sin conexión».
   - Se declara «Sin conexión» solo tras **varios fallos seguidos durante unos 20 segundos** (ajusta el valor en la Fase 0 y documéntalo), o si Android informa que no hay red.
   - Al volver, pasa a «En línea» con el primer éxito.
3. **Revisión según el uso:**
   - **Con el dispositivo inactivo** (pantalla de entrada o sin interacción), la revisión se espacia: por ejemplo, cada 30–60 segundos, o solo con el canal de eventos abierto.
   - **Al tocar algo que necesita la computadora principal,** se reintenta en ese momento antes de mostrar un error.
   - El canal de avisos de platillos listos (A2) no cambia su protocolo; solo se comparte el estado.
4. **Fase 0:** investiga por qué hay desconexiones en el emulador con la Mac sin carga: tiempo de espera, reapertura de TLS, canal de eventos o revisión periódica. Corrige la causa, no solo el aviso.

---

## 3. PERMISOS AGRUPADOS Y ROLES DE FÁBRICA

### 3.1 Roles con permisos de fábrica

- **Una instalación nueva trae los roles con sus permisos habituales ya marcados:** Mesero, Cajero, Cocina, Supervisor, Inventario y Finanzas.
  - **Mesero:** todo lo de la tabla de `docs/ANDROID_INSTALAR.md` §4, incluidos Para llevar e imprimir precuenta, salvo «Modificar» y «Cancelar unidades», que quedan opcionales.
  - **Los demás roles:** propón los conjuntos en la Fase 0 a partir de lo que cada puesto hace en el sistema, y documéntalos.
  - **Administrador** sigue con todo.
- **En instalaciones existentes, la migración no toca los roles.** Se agrega el botón **«Restablecer permisos predeterminados»** por rol, que muestra qué se agregará y qué se quitará, pide confirmación y queda en el Registro de movimientos. Un rol creado por el usuario no tiene ese botón.
- **El servidor sigue validando cada permiso.** Nada de esto cambia la seguridad.

### 3.2 Pantalla de permisos de un rol

- **Agrupada por área,** con secciones plegables:
  - Mesas y cuentas.
  - Captura y pedidos.
  - Para llevar.
  - Cocina e impresión.
  - Caja y cobros.
  - Catálogo.
  - Inventario.
  - Reportes.
  - Personal.
  - Terminales y sistema.

  Cada sección muestra **«5 de 12»** y tiene **«Marcar todo el grupo»** y **«Quitar todo el grupo»**.
- **Buscador** arriba, que filtra por nombre y descripción.
- **Cada permiso con una descripción corta** en lenguaje claro.
- **Dependencias automáticas:**
  - Al marcar un permiso, se marcan solos los que necesita. Por ejemplo, «Abrir cuentas» marca «Consultar cuentas».
  - Al quitar uno del que dependen otros, se avisa cuáles dejarán de funcionar.

  Las dependencias se definen en un solo lugar del servidor y la interfaz las lee de ahí.
- **Vista sencilla por capacidades** como pestaña inicial, con la vista detallada al lado:
  - Opciones como «Atender mesas», «Para llevar», «Cobrar», «Manejar caja», «Cocina», «Ver reportes», «Administrar catálogo», «Administrar personal» y «Configurar el sistema».
  - Cada una marca o quita su conjunto de permisos.
  - Si un rol tiene solo una parte del conjunto, la capacidad se ve como parcial.

---

## 4. AJUSTES DE LA APLICACIÓN ANDROID

1. **Flecha de regreso visible** arriba a la izquierda en toda pantalla que no sea la principal: Captura, Pendientes, cuenta y Para llevar. Esto es además del Atrás de Android. Al salir de una captura con productos sin enviar, se aplica la regla de la A1 (lo capturado queda guardado con el mesero) y se avisa con una frase clara.
2. **El título de la captura dice dónde se está:** «Mesa 4» o «Para llevar #12», no «Captura».
3. **Buscador de productos de una sola línea,** con el texto «Buscar producto».
4. **«Ver la cuenta (0)»:** se quita el número. Si hay productos nuevos sin enviar, el botón dice «Ver la cuenta · 2 por enviar»; si no, «Ver la cuenta».
5. **Pantalla de entrada:**
   - Logotipo de AD-EL POS en la barra superior, junto al título.
   - «Entrar» → **«Ingresar»**.
   - «Toca tu nombre y escribe tu PIN» → **«Selecciona tu usuario y escribe tu PIN»**.
   - «Elegir otro nombre» → **«Elegir otro usuario»**.
   - «¿Quién eres?» → **«Selecciona tu usuario»**.
6. **Pendientes:**
   - Título corto **«Pendientes»**.
   - Flecha de regreso.
   - Sin «Reintentar ya» cuando no hay pendientes.
7. **Pantalla de Mesas compacta,** en teléfono, tableta, vertical y horizontal:
   - Sin el campo «Buscar mesa».
   - Una sola fila con «Lista | Mapa», las áreas como pestañas y «Pendientes».
   - Sin el botón «Ver la mesa»: tocar la mesa la abre.
   - **El mapa llena todo el espacio disponible,** con la escala ajustada a las mesas del área y desplazamiento si no caben, como el escritorio. Hoy, en la tableta vertical, se ve diminuto, y en el teléfono horizontal no se ve.
8. **Defecto: tarjetas de cuenta en el panel de la mesa** (tableta). Hoy se dibujan letra por letra en vertical porque la etiqueta de estado ocupa todo el ancho.
   - Tarjeta en dos renglones: «Mesa 6 · folio 12 · $755.00» arriba y la etiqueta corta abajo.
   - Prueba en todos los anchos.
9. **Imprimir precuenta desde el panel de la mesa,** en cada cuenta, además de dentro de la cuenta. Si falta permiso o impresora asignada, **se muestra el botón con la explicación**, en lugar de ocultarlo sin decir nada.
10. **Cuentas en borrador con $0.00** en el panel: botón **«Descartar cuenta vacía»**, como en el escritorio.
11. **Tarjetas de producto sin imagen:** en teléfono, tarjeta compacta con nombre y precio, sin la letra grande. En tableta, la tarjeta sin imagen también se compacta.
12. **Pantalla inicial por dispositivo:** la aplicación respeta la pantalla inicial configurada en Terminales (Mesas o Para llevar) al entrar el mesero. Hoy siempre abre Mesas.

---

## 5. TERMINALES EN EL ESCRITORIO

1. **Las opciones dependen del tipo de dispositivo:**
   - En un dispositivo `android`, «Pantalla inicial» ofrece solo **Mesas** y **Para llevar**. «Cocina» no se ofrece, porque la aplicación no tiene modo cocina.
   - Un dispositivo que ya tenga «Cocina» guardada pasa a «Mesas», mediante la migración de la etapa.
   - Aplica el mismo criterio al resto de los campos: cada tipo solo ve lo que le aplica.
2. **Columna «Caja»:** en dispositivos `android`, un guion en lugar de «Sin asociación».
3. **«Código: 2b6489b9-…»:** se quita de la tabla. Si hace falta, se muestra solo en «Ver configuración», con el nombre «Identificador».

---

## 6. PRUEBAS

- **Regla de textos:** la comprobación automática, y una revisión de las pantallas principales con capturas en ambas aplicaciones.
- **Conexión:**
  - Un fallo aislado no muestra «Sin conexión».
  - Varios fallos seguidos sí lo muestran.
  - Recuperación con el primer éxito.
  - Indicador y avisos siempre coherentes.
  - Espaciado de la revisión con el dispositivo inactivo.
  - Una hora en el emulador sin uso sin avisos falsos, con la Mac sin carga.
- **Roles de fábrica:**
  - Instalación nueva con los roles listos: un mesero nuevo puede atender una mesa sin tocar permisos.
  - «Restablecer permisos predeterminados» con la vista previa de cambios.
  - Las dependencias automáticas.
  - El servidor rechaza lo que no se tiene aunque la interfaz lo muestre.
  - Las capacidades parciales.
- **Pantalla de permisos** (Playwright): grupos, contadores, marcar y quitar el grupo, buscador, dependencias y vista sencilla.
- **Android,** cada punto de §4, en teléfono y tableta, vertical y horizontal; el defecto de las tarjetas en todos los anchos; la pantalla inicial Para llevar.
- **Terminales:** opciones según el tipo, columna Caja e identificador.
- **Regresión completa** en la Fase E: servidor, escritorio y Android en los cuatro emuladores.

---

## 7. DOCUMENTACIÓN Y ENTREGA

- Actualiza `docs/ANDROID_INSTALAR.md` (roles de fábrica: ya no hay que marcar permisos a mano), `CLAUDE.md` y `android/CLAUDE.md`.
- **Entregables:**
  - APK **0.3.0** con la llave de desarrollo de siempre.
  - `.pkg` **0.14.0** con su parada.
- Fusiona en `main` y haz push.
- **Entrega final breve:**
  - Cambios por fase.
  - Causa de las desconexiones encontrada en la Fase 0.
  - Conjuntos de permisos de cada rol de fábrica.
  - Pruebas.
  - Limitaciones.
