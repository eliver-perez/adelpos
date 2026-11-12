# AD-EL POS — ETAPA 9D: AJUSTES DE INTERFAZ, COLOR DE MARCA Y CANCELAR CUENTA

Continúa el desarrollo de **AD-EL POS** sobre la versión que dejó la **9C** (0.9.2, esquema 20). Confirma la versión y el esquema reales en `config/product.json` y `CLAUDE.md`.

Esta entrega reúne las observaciones del propietario al probar la 9C:

- Ajustes de Mesas, Cuentas y Para llevar.
- Cambio del color principal de verde al azul de la marca.
- Reacomodo de la barra superior.
- La función **Cancelar cuenta** con motivo y autorización.

El instalador de Windows **no** es parte de esta entrega: se hará después, desde una computadora con Windows 11, con un prompt aparte.

Al terminar, la versión será **0.9.3**.

---

## 0. FORMA DE TRABAJO

### Rama y especificaciones

Trabaja en una rama nueva **`etapa-9d`** creada desde `main` (antes confirma que `main` ya contiene la 9C; si no, avísalo en el avance y parte de `etapa-9c`).

Las especificaciones en `docs/etapas/` son de solo lectura; lee únicamente este documento. No apliques formato a `docs/etapas/`.

### Eficiencia

- La Fase 0 la hace **un solo agente**.
- **Máximo 2 agentes en total trabajando al mismo tiempo. Los subagentes no pueden lanzar otros subagentes.**
- En cada fase ejecuta solo las pruebas relacionadas con lo que cambió. La regresión completa (todas las suites, Playwright y la compilación) solo en la fase final.
- Cada agente lee solo las secciones de la documentación que necesita.

### Autonomía (nueva regla permanente)

El propietario autoriza que, **si según tu criterio todos los cambios son correctos y las pruebas pasan, los apliques sin esperar su autorización**. Esto incluye:

1. **Continuar de una fase a la siguiente** sin pedir confirmación.
2. **Migraciones:** aplícalas a la instalación real siguiendo siempre el procedimiento de respaldo: respaldo previo, verificación de que el respaldo se puede restaurar, migración, verificación posterior. Registra la ruta del respaldo en el avance. Nunca borres ni reescribas datos de operación (ventas, pagos, cortes, movimientos de inventario) sin pedir autorización.
3. **Instalar la nueva versión** en la Mac de pruebas.
4. **Al terminar:** fusionar `etapa-9d` en `main` y hacer push al repositorio remoto.

**Agrega esta regla a `CLAUDE.md`** como forma de trabajo permanente para las siguientes peticiones, con el mismo texto y las mismas excepciones.

**Detente y pregunta únicamente si:**

- Una prueba falla y arreglarla exige cambiar una decisión de este documento.
- Encuentras una contradicción que obliga a una decisión funcional importante.
- Una acción no se puede deshacer y podría perder datos.
- Algo afecta servicios o cuentas fuera del proyecto (credenciales, pagos, publicación).

### Fase 0: verificación contra el código

Registra en `docs/ETAPA_9D_AVANCE.md`:

1. Dónde se definen hoy los colores. Busca cuántos componentes tienen el verde u otros colores escritos directamente, en lugar de tomarlos de las variables del tema.
2. Cómo se crea hoy una cuenta de mesa (creación diferida de la 9C) y una de Para llevar. Lista las cuentas vacías existentes.
3. Qué existe hoy de «Anular cuenta»: permisos, motivos, reportes de cuentas anuladas y la columna «Autorizó».
4. **Por qué hay productos enviados a cocina hace días que siguen «Pendiente de liberación · revisar cocina».** Indica si es solo porque en las pruebas no hay pantalla de cocina ni impresora configurada, o si hay un defecto.
5. Cómo se empaqueta hoy la aplicación en macOS: cómo se incluyen y se inician el servidor Go y PostgreSQL, dónde se guardan los datos y los secretos (Keychain) y cómo se genera el instalador.
6. Qué partes dependen de macOS: rutas, impresión con CUPS, cajón por USB, Keychain, menú nativo, atajos con Cmd y servicios en segundo plano.

Los puntos 5 y 6 no se cambian en esta entrega: guárdalos además en **`docs/NOTAS_WINDOWS.md`**, que servirá de punto de partida al preparar el instalador de Windows.

Si algo obliga a cambiar una decisión del propietario, **detente y pregunta**. Si no, continúa.

### Fases

| Fase | Contenido |
| ---- | --------- |
| **0** | Verificación contra el código. |
| **A** | Color de marca, paleta de estados, avisos, botones destructivos y cursores (§1). |
| **B** | Barra superior (§2). |
| **C** | Mesas: panel, mapa y tiempos (§3). |
| **D** | Cuentas y Para llevar: creación diferida, tarjetas y detalle de la cuenta (§4). |
| **E** | Cancelar cuenta (§5). Probablemente requiere migración. |
| **F** | Regresión completa, documentación, notas para Windows, instalación en la Mac, fusión en `main` y push. |

**Al terminar cada fase:** ejecuta sus pruebas, aplica el formato (sin tocar `docs/etapas/`), registra un resumen en el avance, haz un commit local y continúa.

---

## 0-B. DECISIONES DEL PROPIETARIO

**Si una sección parece decir otra cosa, prevalece esta.**

- **El color principal pasa de verde al azul de la marca.** El verde desaparece de la interfaz.
- **Paleta definitiva** (reemplaza los colores de estado de la 7.1):
  - **Azul:** botones principales, selección, mesa Ocupada, Turno abierto, En línea y avisos de éxito.
  - **Ámbar:** Esperando pago, caja en arqueo y advertencias.
  - **Uva:** Pago parcial.
  - **Gris:** Libre (relleno blanco o neutro), Fuera de servicio (con candado) y Turno cerrado.
  - **Turquesa:** avisos informativos.
  - **Rojo:** acciones destructivas, errores y Sin conexión.
- **Caja sale del grupo de secciones** y pasa al lado derecho de la barra, junto a «En línea».
- **Cualquier cuenta sin pagos se puede cancelar**, sea de mesa o para llevar, siempre con motivo. La cancelación requiere autorización de supervisor si el usuario no tiene el permiso; si lo tiene, su propio permiso basta.
- **Ninguna cuenta se guarda sin productos**, ni en mesas ni en Para llevar.
- **No cambian:** las reglas de negocio, los cálculos, la clasificación de cancelaciones de la etapa 8, los flujos de cobro ni las acciones del panel de mesa. Solo cambian su presentación y lo indicado aquí.

---

## 1. FASE A — COLOR DE MARCA Y ESTILOS GENERALES

### 1.1 Color principal

- **Azul principal: `#1F5FA8`. Presionado o activo: `#173D6E`.** Úsalos en los botones principales, en la pestaña o sección activa, en el contorno de los campos activos, en la mesa o tarjeta seleccionada y en los indicadores positivos.
- Fondo suave para la sección activa y la tarjeta seleccionada: un azul muy claro de la misma familia (por ejemplo `#E7EEF7`, con borde `#A9C2E2`).
- **Tema oscuro:** aclara el azul lo necesario para mantener el contraste. Puedes basarte en el azul claro del logo, `#4090F0`, y verificar el contraste de los textos sobre él.
- **El color se define en un solo lugar** (variables del tema o de Tailwind). Elimina todos los colores escritos directamente en los componentes que encontraste en la Fase 0, para que un cambio futuro de color se haga en un solo sitio.

### 1.2 Estados de mesa

Aplica la paleta de §0-B. La selección se comporta como hoy, solo cambia el color:

- **Libre:** relleno blanco. Seleccionada: el mismo relleno con borde azul.
- **Ocupada:** relleno azul claro con borde azul medio. Seleccionada: el mismo relleno con el borde azul más grueso.
- **Esperando pago:** ámbar. **Pago parcial:** uva, un violeta definido que no se confunda con el azul. **Fuera de servicio:** gris tenue con candado.
- Cada estado conserva su ícono en la tarjeta y en la leyenda, para que se reconozca por color y por ícono.

### 1.3 Avisos (Sonner)

| Tipo | Color | Ícono |
| ---- | ----- | ----- |
| Éxito | Azul | Palomita |
| Información | Turquesa | «i» |
| Advertencia | Ámbar | Triángulo |
| Error | Rojo | Equis |

### 1.4 Importes

El total y el saldo de las cuentas, que hoy salen en verde, pasan al color normal del texto, en negritas.

### 1.5 Acciones destructivas en rojo

Todas las acciones que no se pueden deshacer usan el estilo destructivo rojo de forma consistente, en botones y en el botón de confirmar del diálogo: Cancelar cuenta, Cancelar unidades, Eliminar, Descartar y equivalentes. Revisa la aplicación completa, no solo las pantallas mencionadas.

### 1.6 Cursor en controles deshabilitados

En macOS los controles deshabilitados muestran el cursor de «cargando», que comunica «espera» y no «no disponible». Define en el estilo global que **todo control deshabilitado use `cursor: not-allowed`** (o la flecha normal). El cursor de progreso se reserva para cuando realmente se está esperando una respuesta.

---

## 2. FASE B — BARRA SUPERIOR

Hoy la leyenda «Turno abierto» junto a Caja empuja a Administración y la hace perderse. Nuevo orden:

- **Izquierda:** logotipo, **Mesas, Para llevar, Cocina, Administración**, todas seguidas y del mismo tamaño.
- **Derecha:** **Caja** con el estado del turno debajo, en dos renglones («Caja» / «Turno abierto»); luego **En línea**, el menú del usuario y pantalla completa.
- Caja sigue siendo un botón con el mismo menú rápido y los mismos permisos de la 7.1/9C. Solo cambia de lugar y de formato.
- **Colores de los indicadores:**
  - Turno abierto y En línea: azul.
  - Turno cerrado: gris.
  - Caja en arqueo: ámbar.
  - Sin conexión: rojo.
- Mantén los botones táctiles de 52–56 px de alto y verifica que la barra no se corte a 1280 px de ancho.

---

## 3. FASE C — MESAS

### 3.1 Panel lateral siempre visible

Hoy el panel aparece solo al seleccionar una mesa, y el mapa cambia de tamaño. El panel debe estar **siempre visible**, con un ancho fijo:

- **Sin mesa seleccionada:** muestra el resumen del área: cuántas mesas hay libres, ocupadas, esperando pago y con pago parcial. Cada número se puede tocar y resalta esas mesas en el mapa. Muestra también las acciones rápidas «Cuenta sin mesa» y «Nuevo pedido para llevar», y el texto de ayuda que hoy está arriba del mapa (se quita de ahí).
- **Con mesa seleccionada:** el contenido actual del panel.

### 3.2 Mapa

- El mapa ocupa **la misma altura que el panel**.
- **Al abrir un área, ajusta la escala automáticamente** para que se vean todas sus mesas, con la misma escala para todas.
- **Tamaño mínimo de mesa de unos 64 px.** Si con ese tamaño no caben todas, el mapa se desplaza: arrastrando con el dedo y con barras de desplazamiento.
- La leyenda va dentro del mapa, en una sola línea.

### 3.3 Botón principal del panel

**«Capturar pedidos en esta mesa»** (y su equivalente «Capturar productos en esta cuenta») pasa a ser el **botón principal relleno en azul**. Hoy, sin color, pasa desapercibido. Las demás acciones (Ver cuenta, Precuenta, Dividir, Más…) quedan como botones secundarios: **un solo botón principal por panel**. Aplica este criterio a todos los paneles de detalle de cuentas de la aplicación.

### 3.4 Tiempos

- A partir de 24 horas, los tiempos se muestran en días y horas: «3 d 14 h» en lugar de «87 h 33 min». Aplica en mesas, cuentas, Para llevar y en «Abierta hace…».
- **Cuentas abiertas por mucho tiempo:** cuando una cuenta lleva abierta más de **12 horas**, su tarjeta y su mesa muestran un estilo de advertencia discreto (ámbar). El umbral es configurable en las Funciones del establecimiento, con 12 horas como valor predeterminado.

### 3.5 Botón «Actualizar»

Los datos se actualizan solos en tiempo real, así que el botón «Actualizar» sobra. **Quítalo** de Mesas, Cuentas y Para llevar. Muestra **«Reintentar»** solo cuando se pierde la conexión.

---

## 4. FASE D — CUENTAS Y PARA LLEVAR

### 4.1 Creación diferida en Para llevar

Hoy, si se toca «Nuevo pedido» y se sale sin capturar, queda un pedido vacío. Igual que se corrigió en mesas:

- «Nuevo pedido» entra a la captura **sin crear nada**. El pedido nace al guardar el primer producto.
- Si se sale sin productos, no queda nada guardado.

### 4.2 Cuentas vacías existentes

Las cuentas vacías que ya existen, de mesa (por ejemplo las #1 y #3 en «Borrador por confirmar» con $0) y de Para llevar (#5, #8, #9 y #10), se pueden **descartar con un botón, sin motivo**, igual que «Liberar mesa». Descartar una cuenta vacía no cuenta como cancelación ni aparece en reportes.

### 4.3 Tarjetas compactas

En **Para llevar** y en **Mesas › Cuentas**, cada tarjeta hoy ocupa mucho alto para poca información (caben unas 4 por pantalla). Compáctalas a **2 o 3 renglones**:

- **Renglón 1:** folio, mesa o cliente, tiempo transcurrido e importe.
- **Renglón 2:** los estados como etiquetas en la misma línea (pago, entrega, preparación) y las unidades.

Elimina datos repetidos. El objetivo es que quepa al menos el doble de tarjetas. La selección usa el estilo azul de §1.

### 4.4 Detalle de la cuenta: textos

- **«Pendiente de liberación · revisar cocina»** aparece en el encabezado y en cada producto, y suena a error. Cámbialo por una etiqueta corta y clara, como **«Enviado · sin confirmar en cocina»**, una sola vez por producto. Si en la Fase 0 encontraste un defecto, corrígelo.
- **«Desglose · Versión 1 · Descuento $0.00»** es información interna. El desglose muestra solo los complementos elegidos y el descuento si lo hay. No muestres números de versión, y si no hay descuento, no lo muestres.
- **Nombres repetidos en combos:** «Papas a la francesa: Papas a la francesa · Hamburguesa: Hamburguesa Sencilla». Cuando lo elegido tiene el mismo nombre que el grupo, muestra solo lo elegido: «Papas a la francesa · Hamburguesa sencilla · Coca Cola 600ml». Si es distinto, conserva «Grupo: elección».

### 4.5 Detalle de la cuenta: acciones por producto

«Modificar» y «Cancelar unidades» en cada renglón ocupan mucho alto: con tres productos ya no cabe la cuenta. Pásalos a un **menú por producto** que se abre al tocar el renglón o un ícono «⋯». «Cancelar unidades» va en rojo dentro de ese menú. Los controles de cantidad de los productos nuevos sin enviar (− 1 + y Personalizar) se quedan como están.

### 4.6 Lo que se conserva

La separación «Enviado a cocina» / «Nuevo sin enviar» y «Cobrar · $importe enviado» están bien y no cambian. Solo cambia el color del total (§1.4).

---

## 5. FASE E — CANCELAR CUENTA

### 5.1 Nombre y disponibilidad

- «Anular cuenta» pasa a llamarse **«Cancelar cuenta»**, con estilo destructivo rojo, en todas las pantallas, menús, permisos visibles, reportes y el Registro de movimientos.
- Está disponible en **cualquier cuenta sin pagos**, de mesa o para llevar, tenga o no productos enviados. Hoy, en mesas con productos no existe, y el caso real de un cliente que se va sin pagar no tiene una salida correcta: solo quedaría registrar un pago falso o un descuento del 100 %, y los dos ensucian la caja y los reportes.
- Si la cuenta tiene pagos parciales, primero se devuelven, como ya exige la etapa 6. El botón lo indica en lugar de solo deshabilitarse.

### 5.2 Motivo obligatorio

- Se elige de una **lista configurable** (en Administración, junto a los demás catálogos de motivos si existen), con estos valores iniciales: **Cliente se retiró sin pagar, Cliente canceló el pedido, Error de captura y Otro**.
- Al elegir «Otro», la descripción es obligatoria.

### 5.3 Autorización

- Si el usuario tiene el permiso de cancelar cuentas, su propio permiso basta.
- Si no lo tiene, se pide la autorización de un supervisor con el mismo mecanismo que ya usan los descuentos.
- Se registran quién canceló y quién autorizó. Esto habilita la columna **«Autorizó»** en el reporte de cuentas canceladas.

### 5.4 Efectos

- **Productos ya enviados a cocina:** siguen la clasificación de la etapa 8 (preparado, que cuenta como merma, o no preparado). En «Cliente se retiró sin pagar», lo ya preparado queda como merma.
- **Reportes:** la cuenta **no cuenta como venta**. Aparece en **«Cuentas canceladas»** (antes «Cuentas anuladas») con su motivo, importe, quién la canceló y quién la autorizó. Agrega un total por motivo, para que el propietario vea cuánto se pierde al mes por clientes que se van sin pagar.
- La mesa queda libre.
- El evento queda en el Registro de movimientos.

### 5.5 Migración

Si se necesitan columnas o tablas nuevas (motivo, catálogo de motivos, autorizador, umbral de §3.4), hazlo en **una sola migración** y aplícala con el procedimiento de respaldo de §0. **Esquema esperado: 21.**

---

## 7. PRUEBAS

- **Por fase**, las pruebas de lo que cambió.
- **Interfaz (Playwright):**
  - Panel de mesas siempre visible.
  - Mapa con ajuste de escala y desplazamiento.
  - Nuevo pedido para llevar sin productos que no deja nada guardado.
  - Descartar cuentas vacías.
  - Tarjetas compactas.
  - Menú por producto.
  - Barra superior a 1280 px.
  - Cursor `not-allowed` en controles deshabilitados.
- **Cancelar cuenta:**
  - Con y sin permiso (autorización de supervisor).
  - Con productos preparados y no preparados.
  - Con pagos parciales (debe exigir la devolución).
  - Motivo «Otro» sin descripción (debe rechazarse).
  - Exclusión de ventas y aparición en «Cuentas canceladas» con «Autorizó» y total por motivo.
- **Tiempos:** formato «d h» y advertencia a partir del umbral.
- **Colores:** una prueba o revisión que confirme que no quedan colores escritos directamente en los componentes, y capturas en tema claro y oscuro de Mesas, Cuentas, Para llevar, la barra superior y los avisos.
- **Regresión completa** en la Fase F.

---

## 8. DOCUMENTACIÓN Y ENTREGA

- Actualiza `CLAUDE.md`: versión 0.9.3, esquema 21 (si hubo migración), la paleta de colores y **la nueva regla de autonomía de §0**.
- Actualiza `docs/ETAPA_9D_AVANCE.md` con el resumen por fase, la ruta del respaldo y el resultado de la migración.
- Completa `docs/NOTAS_WINDOWS.md` con lo encontrado en la Fase 0 (puntos 5 y 6) y con cualquier detalle de esta entrega que afecte a Windows.
- Instala la versión en la Mac, fusiona en `main` y haz push (Fase F).
- **Entrega final breve:**
  - Cambios por fase.
  - Pruebas ejecutadas.
  - Resultado de la migración.
  - Limitaciones: pantalla táctil física y lo que no se haya podido verificar.
