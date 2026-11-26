# AD-EL POS — ETAPA K1.1: PEDIDOS MIXTOS, PRODUCTOS SIN RUTA Y CONFIGURACIÓN GUIADA DE COCINA

Se ejecuta **en la Mac**, sobre `main`.

- **Parte de:** servidor 0.16.0 (esquema 29), meseros 0.4.1 y cocina 0.1.0. Confirma los valores reales en `config/product.json`, `CLAUDE.md`, `android/CLAUDE.md` y `cocina/CLAUDE.md`.
- **Al terminar:** servidor **0.17.0**. Las aplicaciones Android solo cambian de versión si cambia su código (cocina 0.1.1, meseros 0.4.2).

**Contenido:** lo que encontró el propietario al probar la K1 con la 0.16.0 instalada. Son correcciones y ajustes de cocina: **no se agregan funciones fuera de lo que dice este documento.**

---

## 0. FORMA DE TRABAJO

### Rama y especificaciones

- Rama **`etapa-k1-1`** desde `main`. Las especificaciones en `docs/etapas/` son de solo lectura.
- Lee este documento, `CLAUDE.md`, `cocina/CLAUDE.md`, `docs/KITCHEN_AND_PRINTING.md`, `docs/COCINA_INSTALAR.md` y, del avance de la K1, las decisiones K1-D5, K1-D11 y K1-D23.

### Eficiencia

- La Fase 0 la hace **un solo agente**.
- **Máximo 2 agentes en total trabajando al mismo tiempo. Los subagentes no pueden lanzar otros subagentes.**
- En cada fase, solo las pruebas de lo que cambió. La regresión completa, **una sola vez**, en la Fase D. Si una corrección de la regresión toca `backend/`, `config/` o código compartido, repite la regresión completa; si no, solo las suites afectadas.
- **Sin revisor independiente**: no se toca seguridad, cupos ni licencias.
- **Android en dos emuladores** (`Pixel_Tablet` y `ADEL_A1_Tablet_API26`, en horizontal), solo si cambia algo que llegue a las aplicaciones.
- **Capturas:** solo de las pantallas nuevas o cambiadas, en tema claro.

### Autonomía

Aplica la regla de autonomía de `CLAUDE.md` §0 y la protección contra el servicio real. Ninguna prueba llama a `licensing.sicrat.com`.

**Paradas:** instalar el `.pkg` (contraseña de administrador).

### Fases

| Fase | Contenido |
| ---- | --------- |
| **0** | Diagnóstico con los casos del propietario (§1). |
| **A** | Servidor: productos sin ruta y pendientes de liberación (§2, §3). |
| **B** | Escritorio: configuración guiada y diagnóstico (§4). |
| **C** | Lo que haga falta en la aplicación de cocina, si algo (§5). |
| **D** | Regresión completa, documentación, `.pkg` (y APK si cambiaron), fusión y push (§7). |

**Al terminar cada fase:** ejecuta sus pruebas, registra un resumen en `docs/ETAPA_K1_1_AVANCE.md`, haz un commit y continúa.

---

## 1. LO QUE VIO EL PROPIETARIO (FASE 0: REPRODUCIR Y ENCONTRAR LA CAUSA)

**Configuración de su prueba:**

- una estación «Cocina» con recepción «Solo pantalla de cocina»;
- enrutamiento: las categorías **Desayunos** y **Hamburguesas** a Cocina;
- **Refrescos** (Coca Cola 600 ml) y **Extras** (Papas a la francesa) **sin ruta**;
- un KDS emparejado y «En línea», con la estación Cocina.

**Resultados:**

| Pedido enviado desde la terminal | ¿Llega al KDS? |
|---|---|
| Solo machacado | Sí |
| Machacado + hamburguesa | Sí |
| Machacado + Coca Cola | **No** |

**En Cocina › Preparación del escritorio:**

- El panel **«Pendientes de liberación y avisos»** se llena de pedidos marcados **«Bloqueado por configuración»**, con renglones «Sin ruta de preparación: Coca Cola 600ml», «… Papas a la francesa», etc. (13 pedidos en su captura).
- **Esos pedidos no se quitan aunque la cuenta ya se cobró y se cerró.** Solo desaparecen los que tienen todo en estaciones con ruta.
- Hay pedidos de antes de configurar el enrutamiento («Sin ruta de preparación: Machacado Natural») con «Liberar a preparación» y «Ajustar programación»; para el propietario no queda claro qué hacer con ellos.

**Hipótesis:** un solo producto sin ruta bloquea **todo el envío** («Bloqueado por configuración»), así que lo que sí tiene ruta tampoco llega al KDS. El escritorio lo muestra en el panel de pendientes, el KDS no lo ve.

**Fase 0:**

1. Reproduce los tres casos con un Go temporal y una prueba automática.
2. Confirma la causa con el código (la regla de la 9F que bloquea un envío por un producto sin ruta) y anótala en el avance. Si la causa es otra, anótala y sigue con lo que pide este documento.
3. Revisa todo lo que muestra el panel «Pendientes de liberación y avisos»: qué casos lo llenan, cuándo salen, qué hace cada botón. Anótalo en lenguaje sencillo.

---

## 2. REGLA NUEVA: UN PRODUCTO SIN RUTA NUNCA DETIENE EL PEDIDO

- **Un producto sin ruta no bloquea a los demás.** Lo que tiene ruta llega a su estación (impresora, KDS o ambas) aunque el mismo envío lleve productos sin ruta.
- **Qué pasa con un producto sin ruta:** se decide con un ajuste general nuevo en Cocina › Enrutamiento, **«Productos sin ruta»**:
  - **«No necesitan preparación»** (por omisión): no van a ninguna estación ni bloquean nada. Es el caso normal de refrescos y extras listos para servir.
  - **«Enviar a una estación»**, eligiendo la estación: para quien prefiere que todo lo no configurado llegue a la cocina.
- **«Bloqueado por configuración» desaparece** como resultado normal de un producto sin ruta. Si queda algún caso que de verdad no se pueda resolver solo (por ejemplo, una ruta a una estación desactivada o borrada), el envío **no se detiene**: lo demás sigue y ese producto se trata como «Productos sin ruta», con un aviso en Enrutamiento.
- **El aviso al mesero de platillos listos** (A2) y la tarjeta del KDS (K1-D5) no cuentan lo que no necesita preparación: una tarjeta no se cierra sola porque la Coca Cola «esté lista». Pruébalo con el caso del propietario.
- **Envíos anteriores** (ya guardados bloqueados): no se reenvían solos a la cocina. Ver §3.

---

## 3. PENDIENTES DE LIBERACIÓN

- **Una cuenta cobrada, cerrada o cancelada no deja nada en «Pendientes de liberación y avisos».** Lo que quedó pendiente de una cuenta así se da por terminado (con su registro en el historial), sin borrar datos de operación.
- **Lo que ya está en la Mac del propietario** (los 13 pendientes de su captura): al actualizar, los de cuentas cobradas, cerradas o canceladas dejan de mostrarse. Hazlo con una regla de lectura o una migración que **solo cambia el estado de esos pendientes**, nunca ventas, pagos ni cuentas. Si es migración, el `host` ya respalda antes de aplicarla.
- **El panel debe explicarse solo.** Para cada caso que quede (por ejemplo, un pedido programado para más tarde):
  - un título que diga qué pasa, en lenguaje sencillo;
  - qué hace cada botón («Liberar a preparación», «Ajustar programación») con un texto corto debajo o al lado;
  - si un caso ya no puede ocurrir con la regla de §2, quítalo.
- **Nombre del panel:** si después de §2 solo quedan pedidos programados, llámalo así («Pedidos programados»); si quedan otros casos, un nombre que los describa.

---

## 4. CONFIGURACIÓN GUIADA (ESCRITORIO)

**Problema:** para que un pedido llegue a un KDS hay que acertar en cuatro lugares (Estaciones, la recepción de la estación, Enrutamiento y Terminales) y ninguno avisa si falta algo. El propietario no supo cómo hacerlo funcionar.

### 4.1 Asistente al emparejar un KDS

Al aprobar el código de una pantalla de cocina en Terminales, un asistente corto:

1. **Estaciones de esta pantalla:** las que reciben comandas en pantalla (recepción «Solo pantalla» o «Impresora y pantalla»).
   - Si no hay ninguna, ofrece **crear «Cocina»** ahí mismo (recepción «Impresora y pantalla» si hay impresora de cocina; si no, «Solo pantalla»).
   - Si hay estaciones que solo imprimen, ofrece cambiar su recepción a «Impresora y pantalla», con su explicación.
2. **Qué categorías llegan a esas estaciones:** casillas con las categorías del catálogo; las que ya tienen ruta salen marcadas con su estación. Marcar una crea o cambia la ruta de esa categoría.
3. **Resumen:** «Esta pantalla recibirá: Desayunos, Hamburguesas. No recibirá: Refrescos, Extras (no necesitan preparación).».

El asistente se puede saltar y volver a abrir desde Terminales («Configurar qué recibe»).

### 4.2 Diagnóstico en Terminales

En «Ver configuración» de un KDS:

- **qué categorías y productos le llegan**, por estación;
- **avisos claros** cuando algo falta, con un botón que lleva a corregirlo:
  - «Ninguna de sus estaciones recibe comandas en pantalla.»;
  - «La estación Cocina no tiene categorías ni productos asignados.»;
  - «Esta pantalla no tiene estaciones.».

### 4.3 Enrutamiento

- **Lista de lo que no va a ninguna estación:** «Sin ruta: Refrescos, Extras», con lo que dice el ajuste de §2 («no necesitan preparación» o «van a Cocina»).
- El ajuste **«Productos sin ruta»** de §2.

### 4.4 Estaciones

- El apartado **«Terminales de cocina»** de Estaciones ya no configura las tabletas (K1). Renómbralo a **«Computadoras en modo Cocina»** con una línea que diga que las pantallas de cocina se configuran en Terminales.
- La descripción «La recepción en la pantalla de cocina queda pendiente hasta que una pantalla de cocina autorizada reciba y confirme la comanda.» se reescribe en lenguaje sencillo.

---

## 5. APLICACIÓN DE COCINA

- Con §2, los pedidos mixtos deben llegar sin cambiar la aplicación. Si la Fase 0 encuentra algo que corregir en `cocina/` o `android-core/`, se corrige aquí y la aplicación sube de versión.
- Si no cambia nada, no se compila un APK nuevo.

---

## 6. PRUEBAS

- **Los casos del propietario, automáticos**, con la configuración de §1:
  - solo machacado; machacado + hamburguesa; machacado + Coca Cola; machacado + Coca Cola + papas: **los cuatro llegan al KDS** con lo que tiene ruta, y la Coca Cola y las papas no aparecen;
  - con «Enviar a una estación»: la Coca Cola y las papas llegan a esa estación;
  - la tarjeta no se cierra sola por lo que no necesita preparación;
  - el aviso de platillos listos al mesero sale cuando lo que tiene ruta está listo;
  - lo mismo con una estación de impresora: la comanda impresa lleva lo que tiene ruta.
- **Pendientes:** una cuenta cobrada, cerrada o cancelada no deja nada en el panel; la regla o migración que limpia los pendientes viejos sin tocar ventas, pagos ni cuentas.
- **Asistente y diagnóstico** (Playwright, Chromium y WebKit): emparejar un KDS sin estaciones y terminar con «Cocina» creada y dos categorías; cada aviso del diagnóstico; la lista «Sin ruta» de Enrutamiento.
- **Aplicación de cocina**, solo si cambió: la suite `kitchen` en los dos emuladores.
- **Regresión completa** una sola vez en la Fase D.

---

## 7. DOCUMENTACIÓN Y ENTREGA

- **`docs/COCINA_INSTALAR.md`:** los pasos en orden para que un pedido llegue a la pantalla (o el asistente), y qué significa «Productos sin ruta».
- Actualiza `docs/KITCHEN_AND_PRINTING.md` con la regla de §2 y lo que quedó del panel de §3, y `CLAUDE.md`.
- **Entregables:** `.pkg` **0.17.0** con su parada; APK solo de la aplicación que cambió.
- Fusiona en `main` y haz push.
- **Entrega final breve:**
  - La causa encontrada en la Fase 0.
  - Cambios por fase.
  - Qué pasa con los pendientes que ya tiene la Mac.
  - Pruebas.
  - Limitaciones.
