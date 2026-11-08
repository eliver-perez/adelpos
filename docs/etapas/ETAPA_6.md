# AD-EL POS — ETAPA 6: COBROS, PAGOS, DESCUENTOS, DEVOLUCIONES Y ENTREGA

Continúa el desarrollo de **AD-EL POS**, sistema comercial de punto de venta para restaurantes.

Actúa como arquitecto e ingeniero senior full stack, especializado en Go, PostgreSQL, React, TypeScript, Tailwind CSS y Tauri 2.

Las etapas 1 a 5 fueron implementadas previamente por otros modelos. La versión actual reportada es **0.5.0**, con esquema PostgreSQL 5.

**Tu tarea es implementar la etapa 6 completa, respetando el proyecto existente, sin reconstruirlo y sin comenzar desde cero.**

Esta etapa incorpora cobros reales, pagos parciales y mixtos, cobro por productos, descuentos, cargos extras, propinas, devoluciones, impresión de tickets y entrega de pedidos para llevar.

También debe preparar correctamente los movimientos financieros para la administración de caja de la etapa 7.

No quiero únicamente documentación, maquetas o demostraciones. Debes implementar la funcionalidad real, persistirla en PostgreSQL, integrarla con la interfaz existente y ejecutar pruebas.

Al terminar la etapa, la versión será **0.6.0** con esquema PostgreSQL **6** (o el número que resulte si se necesitan varias migraciones), actualizada en `config/product.json` y sincronizada con `./scripts/dev.sh sync`.

---

## 0. ANTES DE EMPEZAR: CONTEXTO, PRERREQUISITOS Y FASES

### Contexto obligatorio

Lee primero **`CLAUDE.md`** y **`docs/DIAGNOSTICO_ETAPAS_1_5.md`**. Contienen las decisiones vigentes, los locks consultivos, los criterios para agregar funcionalidad (§20 de `CLAUDE.md`) y los riesgos abiertos (IDs H/M/B) que esta etapa debe atender.

Este documento vive en el repositorio. Si la conversación se compacta o se reinicia, **vuelve a leerlo** antes de continuar.

### Prerrequisitos de seguridad

Revisa en `CLAUDE.md` §19 y en el código si ya están corregidos estos hallazgos del diagnóstico:

- **H1**: escalada de privilegios por delegación de usuarios, roles y permisos.
- **M1**: protección del último administrador.
- **H2**: un 403 confirma la transacción.

**Si alguno sigue abierto, corrígelo como primer paso de la fase 6A**, con pruebas de integración negativas, antes de implementar cualquier cobro. Los límites de descuento por rol y las autorizaciones de supervisor de esta etapa no tienen sentido si un usuario puede elevar sus propios permisos.

### Trabajo por fases

La etapa es grande y delicada. Impleméntala en cuatro fases, en este orden:

| Fase   | Contenido                                                                                                                                                                                                                                                                |
| ------ | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------ |
| **6A** | Prerrequisitos de seguridad (si faltan), migración(es), módulo de pagos en Go, métodos de pago, IVA, turno mínimo, pagos totales/parciales/mixtos, cobro por selección, reglas sobre partidas con pago, idempotencia, concurrencia y pruebas de integración del backend. |
| **6B** | Pantalla de cobro: métodos como botones, teclado numérico fijo, efectivo y cambio, selección de partidas, apertura de turno, estados de interfaz y almacén global de operaciones inciertas.                                                                              |
| **6C** | Descuentos manuales, cortesías, autorizaciones de supervisor, cargos extras generales, propinas, devoluciones y correcciones de pago.                                                                                                                                    |
| **6D** | Precuentas, tickets, toggle de impresión, entrega de pedidos para llevar, regresiones completas, documentación y verificación final.                                                                                                                                     |

Las fases ordenan el trabajo para que cada una se construya sobre una base ya probada. No son puntos de espera obligatorios (ver reglas abajo).

**Al terminar cada fase:**

1. Ejecuta las pruebas correspondientes y las suites de etapas anteriores que puedan verse afectadas.
2. Aplica `./scripts/dev.sh format` y verifica `format-check`.
3. Escribe un resumen breve de la fase: qué se implementó, qué pruebas se ejecutaron y su resultado, decisiones tomadas y pendientes. Acumula estos resúmenes en `docs/ETAPA_6_AVANCE.md` para que yo pueda revisarlos al final.
4. Haz un **commit local** de la fase (sin push), con un mensaje claro, por ejemplo `Etapa 6A: modelo financiero y pagos en backend`. Así cada fase queda como punto de regreso.
5. **Si la fase quedó funcional y todas sus pruebas pasan, continúa con la siguiente fase sin esperar mi confirmación.** Yo probaré la etapa completa al final.

**Detente y pregúntame únicamente si:**

- Una prueba falla y no puedes corregirla sin cambiar una decisión funcional o una regla de este documento.
- Encuentras una contradicción entre este documento, `AGENTS.md`, `CLAUDE.md` o el código que obligue a tomar una decisión funcional importante.
- Necesitas aplicar la migración a la instalación real (§23).
- Una acción podría afectar datos, servicios o archivos fuera del alcance de esta etapa.

Ante dudas menores, toma la opción más conservadora y coherente con el proyecto, anótala en `docs/ETAPA_6_AVANCE.md` y continúa.

---

## 1. REVISIÓN INICIAL Y CONTINUIDAD

Antes de modificar el proyecto, revisa cuidadosamente:

- `CLAUDE.md`.
- `docs/DIAGNOSTICO_ETAPAS_1_5.md`.
- `AGENTS.md`.
- `README.md`.
- `ARCHITECTURE.md`.
- `API_CONTRACTS.md`.
- `DEVELOPMENT_MACOS.md`.
- `NEXT_STEPS_FOR_SOL.md`.
- `docs/VERIFICATION.md`.
- `docs/CATALOG.md`.
- `docs/RESTAURANT_OPERATIONS.md`.
- `docs/KITCHEN_AND_PRINTING.md`.
- Las migraciones existentes.
- El motor de precios.
- Los módulos de cuentas, pedidos y división de partidas.
- La seguridad, permisos, sesiones y terminales.
- La infraestructura de impresión de la etapa 5.
- Los eventos autenticados implementados mediante SSE.
- Las estructuras existentes de cajas físicas y configuración.

Consulta también cualquier otro documento que exista en el repositorio y resulte relevante.

**La implementación real del repositorio es la fuente de verdad técnica.**

Si encuentras alguna diferencia entre la documentación y el código, identifícala. Resuélvela únicamente cuando sea necesaria para la etapa 6 y sin destruir funcionalidades previamente aprobadas; en los demás casos, repórtala en el resumen de la fase.

Las contradicciones ya conocidas están en `docs/DIAGNOSTICO_ETAPAS_1_5.md` §8. En particular:

- `NEXT_STEPS_FOR_SOL.md` prohíbe añadir turnos, propinas e impuestos de forma incidental. Esta etapa los incluye **de forma expresa**; actualiza ese documento.
- `AGENTS.md` dice que `catalog` es el único motor financiero. A partir de esta etapa, `catalog` sigue siendo la **única autoridad de precios**, y el nuevo módulo de pagos (§23) es la autoridad de cobros, cambio, descuentos manuales, cargos, propinas, IVA y devoluciones. Actualiza `AGENTS.md` y `CLAUDE.md` para reflejarlo.

No cambies el nombre del proyecto.

No sustituyas Go, PostgreSQL, React, Tailwind ni Tauri.

No introduzcas microservicios ni otra base de datos.

No modifiques las migraciones 0001 a 0005 ya aplicadas.

No elimines datos del establecimiento.

No modifiques el mecanismo reservado del superadministrador `sa`. Su configuración comercial definitiva se revisará en una etapa posterior.

El desarrollo y las pruebas continúan prioritariamente en **macOS**.

No crear instaladores Windows todavía.

---

## 2. REGLAS PERMANENTES DE CÓDIGO E INTERFAZ

Respeta y conserva las convenciones documentadas en `AGENTS.md`.

### Código fuente legible

**Está prohibido ofuscar, minificar o compactar el código fuente para ahorrar líneas.**

- En Go, no escribir bloques `if`, `for`, `switch` o funciones completas en una línea.
- En React/JSX/TSX, colocar las etiquetas anidadas correctamente separadas e indentadas.
- Utilizar saltos de línea suficientes.
- Mantener indentación consistente.
- Usar nombres descriptivos.
- Incluir comentarios explicativos en español.
- Evitar funciones y archivos monolíticos.
- Separar la lógica financiera de la presentación.
- Aplicar gofmt, Prettier y Rustfmt.
- Revisar los casos que los formateadores no expandan automáticamente.

### Diseño

Mantener la identidad visual existente:

- Logotipos de AD-EL POS.
- Temas claro y oscuro.
- Acordeones modernos tipo tarjeta.
- Controles táctiles de tamaño moderado.
- Diseño profesional, compacto y elegante.
- Navegación consistente.
- Pantallas de administración con tablas, filtros y permisos.

**AD-EL POS debe continuar iniciando en pantalla completa nativa.**

La interfaz debe aprovechar la resolución disponible, sin limitarse artificialmente a 1280 × 720.

Debe permitir salir y regresar a pantalla completa y trabajar correctamente en ventanas redimensionadas.

No convertir el sistema en un quiosco bloqueado.

---

## 3. OBJETIVO GENERAL

Implementar el ciclo financiero de una cuenta comercial.

El sistema debe permitir:

1. Consultar el saldo de una cuenta.
2. Abrir la pantalla de cobro.
3. Elegir un método de pago.
4. Registrar un pago total.
5. Registrar pagos parciales.
6. Combinar métodos mediante operaciones sucesivas.
7. Cobrar productos y cantidades seleccionadas.
8. Dividir cuentas conservando los pagos.
9. Calcular el cambio de efectivo.
10. Registrar referencias electrónicas.
11. Aplicar descuentos por partida y generales.
12. Registrar cargos extras generales.
13. Registrar propinas opcionales separadas de las ventas.
14. Imprimir o no imprimir ticket.
15. Consultar e imprimir precuentas.
16. Registrar devoluciones totales y parciales.
17. Registrar entrega de pedidos para llevar.
18. Consultar el historial financiero.
19. Generar movimientos compatibles con futuros cortes de caja.
20. Mantener integridad, idempotencia, permisos y auditoría.

No implementar una interfaz de cobro independiente de las cuentas existentes. Debe integrarse con las estructuras reales de las etapas anteriores.

---

## 4. INTERFAZ PRINCIPAL DE COBRO

La pantalla de cobro debe ser rápida, táctil y estar organizada para que el cajero pueda realizar una operación sencilla con muy pocos toques.

### Distribución

Organiza la pantalla en áreas funcionales que mantengan visibles:

**Cuenta y productos:**

- Mesa o identificación del pedido.
- Cuenta seleccionada.
- Productos y cantidades.
- Personalizaciones relevantes.
- Precio de cada partida.
- Descuentos aplicados.
- Total.
- Abonos registrados.
- Saldo pendiente.
- Selección de productos y cantidades por cobrar.

**Métodos de pago:**

- Métodos disponibles.
- Método seleccionado.
- Historial compacto de pagos registrados.
- Campos de captura correspondientes.

**Captura:**

- Importe a cobrar.
- Efectivo entregado, cuando corresponda.
- Teclado numérico fijo.
- Cambio.
- Acción principal para registrar el pago.

La distribución debe adaptarse al tamaño de la ventana y aprovechar el monitor completo.

No desperdiciar espacio con botones gigantescos.

### Métodos como botones seleccionables

Los métodos de pago deberán mostrarse mediante botones o tarjetas rectangulares.

Cada botón tendrá:

- Icono a la izquierda.
- Nombre del método.
- Tamaño adecuado para pantalla táctil.
- Estado seleccionado visualmente resaltado mediante borde y fondo.

No utilizar exclusivamente una lista desplegable.

No mostrar iconos de checkbox a la derecha.

Utiliza una distribución flexible o grid que aproveche el ancho disponible.

### Cobro rápido

Para una cuenta sencilla:

1. Abrir cobro.
2. Seleccionar método.
3. Registrar el pago.

El saldo completo debe proponerse como importe inicial.

No exigir abrir asistentes adicionales ni llenar campos irrelevantes.

La interfaz debe responder claramente al éxito, rechazo o error de cada operación.

---

## 5. MÉTODOS DE PAGO

Crear un catálogo configurable de métodos.

Métodos iniciales:

1. Efectivo.
2. Tarjeta de débito.
3. Tarjeta de crédito.
4. Transferencia.
5. Plataformas.

El establecimiento podrá agregar otros métodos.

Cada método debe permitir configurar, según corresponda:

- Nombre visible.
- Icono.
- Estado activo.
- Orden.
- Naturaleza de efectivo o no efectivo.
- Si admite referencia.
- Si la referencia es opcional u obligatoria.
- Si permite propina.
- Permisos o restricciones operativas aplicables.

No permitir que cambiar el nombre de un método destruya las referencias de pagos históricos.

No integrar todavía terminales bancarias.

Los pagos electrónicos son operaciones registradas por el cajero a partir de una confirmación externa, no cobros automáticos mediante bancos.

No registrar como confirmado un pago electrónico cuya ejecución externa todavía sea desconocida.

---

## 6. TECLADO NUMÉRICO FIJO

**El teclado numérico de cobro debe estar integrado de forma fija en la pantalla.**

No debe ser una ventana flotante.

Debe contar con:

- Dígitos 0 a 9.
- Punto decimal.
- Borrar último carácter.
- Limpiar importe.
- Captura táctil rápida.
- Compatibilidad con teclado físico.

Puedes incorporar teclas rápidas útiles sin saturar la interfaz.

### Validación

- No permitir dos puntos decimales.
- No permitir importes negativos.
- Respetar precisión monetaria.
- Evitar importes inválidos.
- No aceptar cadenas mal formadas.
- Manejar correctamente valores vacíos y cero.

La captura debe ser clara, pero todos los importes finales serán validados y calculados en Go con aritmética decimal exacta.

### Lecciones del diagnóstico (M2 y M11)

- **Los campos de importe deben ser campos de texto con `inputMode="decimal"`, nunca `type="number"`.** El diagnóstico reprodujo que las teclas `+` y `.` del teclado virtual vacían los campos `type="number"`.
- El teclado fijo de cobro escribe sobre el estado del importe, no simulando eventos de teclado sobre un `<input>`.
- Teclas y botones de la pantalla de cobro con objetivos táctiles de **al menos 44 px**.
- En la pantalla de cobro, el teclado flotante no debe aparecer sobre los campos de importe. Agrega a `VirtualKeyboard` una forma explícita de excluir campos, y úsala aquí.

### Teclado flotante independiente

El sistema ya dispone de una preferencia para teclado virtual flotante por terminal.

Conservarla.

Si el método de pago requiere capturar una referencia y el teclado flotante está habilitado, mostrar el teclado adecuado al enfocar ese campo.

No confundirlo con el teclado numérico fijo de cobro.

---

## 7. COBROS TOTALES Y PARCIALES

Permitir seleccionar entre:

- Pago total.
- Pago parcial.

Puede utilizarse un control segmentado compacto y claro.

### Pago total

Proponer automáticamente el saldo pendiente.

El cajero selecciona el método y confirma.

### Pago parcial

Permitir capturar el importe que se desea abonar.

Después del registro:

- Guardar el pago.
- Actualizar el saldo.
- Mantener la cuenta abierta cuando tenga importe pendiente.
- Mostrar el pago en el historial.
- Permitir agregar otro pago.

No considerar liquidada una cuenta con saldo pendiente.

### Pagos mixtos

**No crear un proceso complejo o una ventana especial de «Pago mixto».**

Los pagos mixtos se forman registrando pagos sucesivos con diferentes métodos.

Ejemplo:

Cuenta: $500.

- Primer pago: $200 en efectivo.
- Saldo: $300.
- Segundo pago: $150 en débito.
- Saldo: $150.
- Tercer pago: $150 en transferencia.
- Saldo: $0.

Conservar cada movimiento por separado, con su método, usuario, terminal, caja, fecha y referencia correspondiente.

No combinar los tres registros en una sola operación que pierda trazabilidad.

---

## 8. EFECTIVO Y CAMBIO

Cuando se seleccione Efectivo, mostrar claramente:

- Importe de la operación.
- Efectivo recibido.
- Cambio a entregar.
- Importe efectivamente aplicado.
- Saldo restante.

**El cambio es una ayuda visual importante para el cajero.**

Ejemplo:

Cuenta pendiente: $270.

El cliente entrega: $500.

Resultado:

- Aplicado a la venta: $270.
- Cambio: $230.
- Saldo: $0.

### Pagos parciales en efectivo

No confundir el efectivo recibido con el importe del abono.

Ejemplo:

Se desea abonar $100 a una cuenta de $400.

El cliente entrega $200.

Resultado:

- Abono registrado: $100.
- Cambio: $100.
- Saldo: $300.

Registrar correctamente ambas cantidades, sin inflar el ingreso neto de caja.

El cambio no debe registrarse como una venta negativa independiente ni duplicarse al contabilizar efectivo.

### Otros métodos

Los métodos electrónicos no deben generar cambio en efectivo.

No permitir que un importe excedente se interprete automáticamente como propina sin consentimiento y registro explícito.

---

## 9. COBRO POR PRODUCTOS Y CANTIDADES

La cuenta deberá permitir seleccionar directamente las partidas que el cliente quiere pagar.

### Ejemplo

Cuenta:

- 2 hamburguesas.
- 3 refrescos.
- 1 orden de papas.

El cliente selecciona:

- 1 hamburguesa.
- 1 refresco.

El sistema debe calcular únicamente el importe correspondiente.

Cuando una partida tenga varias unidades, permitir indicar cuántas se desean cobrar.

### Requisitos

- Selección táctil de partidas.
- Selección de cantidades.
- Total de selección en tiempo real.
- Disponibilidad para pago total o parcial de lo seleccionado.
- Conservación de las partidas restantes.
- Prevención de doble cobro.
- Identificación de cantidades ya pagadas.
- Relación entre pago y partidas liquidadas.
- Trazabilidad completa.

### Combos

Mantener la integridad de los combos.

No convertir arbitrariamente sus componentes en productos independientes al cobrar.

Si se cobran unidades parciales de un mismo combo, distribuir los importes correctamente según las cantidades vendidas y las reglas confirmadas.

### Descuentos y promociones

Distribuir descuentos y promociones entre cantidades o partidas de manera determinista.

Las asignaciones deben conservar exactamente el total monetario de la cuenta.

No perder ni duplicar centavos.

No modificar los precios históricos confirmados simplemente porque cambió el catálogo.

**El servidor Go será la autoridad sobre el cálculo.**

### Reglas sobre partidas con pago (decisión del propietario)

Cada producto de una cuenta existe como unidades individuales en `app.restaurant_item_units`, con importe exacto propio. Hoy esas unidades no saben si fueron pagadas (riesgo **H3** del diagnóstico). Estas son las reglas aprobadas:

**Dos tipos de pago:**

1. **Pago general a la cuenta** (pago total o abono sin seleccionar productos): reduce el saldo de la cuenta, pero no se asigna a unidades específicas.
2. **Pago por selección** (el cliente elige productos y cantidades): se asigna a las unidades seleccionadas mediante registros de asignación inmutables. Una unidad está **liquidada** cuando sus asignaciones netas cubren su total, y **con pago parcial** cuando tiene alguna asignación sin llegar a cubrirlo.

**Unidades con cualquier pago asignado (liquidadas o con pago parcial):**

- No pueden transferirse, combinarse, cancelarse, editarse ni anularse.
- Para cambiarlas, primero debe revertirse su pago mediante una devolución o corrección autorizada (§17 y §18). Después ya son unidades sin pago y siguen las reglas normales.
- Bloquear con un mensaje claro en español que explique por qué y qué operación corresponde.

**Unidades sin pago asignado:**

- Siguen las reglas de las etapas 4 y 5 (transferir, dividir, combinar, cancelar, editar), **con una condición**: ninguna operación puede dejar una cuenta con un total menor que sus pagos netos. Si un movimiento lo provocaría, bloquearlo y ofrecer la devolución o ajuste correspondiente.

**Combinación de cuentas:**

- La cuenta de origen de una combinación **no puede tener pagos registrados**. La cuenta de destino sí puede tenerlos.
- Así cada pago permanece siempre en la cuenta donde se registró y no hay que trasladar registros financieros.

**Agregar productos después de un pago:** permitido. Aumenta el saldo pendiente sin tocar los pagos anteriores.

Implementa estas guardas en el backend Go (transferencias, divisiones, combinaciones, cancelaciones, ediciones y anulaciones; ver evidencias de H3 en el diagnóstico), dentro de la misma transacción y con el lock correspondiente, y cúbrelas con pruebas de integración.

---

## 10. DIVISIÓN Y COMBINACIÓN DE CUENTAS

La etapa 4 ya incorporó división, combinación y transferencia de cuentas.

**Reutiliza esas operaciones; no crees otro sistema independiente dentro de cobros.**

Desde la pantalla de cobro debe existir acceso para:

- Separar cuentas.
- Seleccionar productos.
- Elegir cantidades.
- Crear cuentas resultantes.
- Consultar cada cuenta.
- Cobrar cada cuenta por separado.

### Integridad financiera

Después de registrar pagos, las divisiones y transferencias deben considerar las cantidades e importes ya liquidados.

No permitir que una partida pagada vuelva a considerarse pendiente por haber cambiado de cuenta.

No perder pagos ni referencias.

No duplicar pedidos, comandas ni inventario.

Cuando una transferencia no pueda realizarse sin alterar una asignación financiera, bloquearla con un mensaje claro y ofrecer la operación de ajuste o devolución autorizada que corresponda.

Aplica las **reglas sobre partidas con pago** de §9: las unidades con pago permanecen en su cuenta; solo se mueven unidades sin pago; ninguna cuenta queda con total menor que sus pagos netos; la cuenta origen de una combinación no puede tener pagos.

No alterar registros financieros históricos silenciosamente.

---

## 11. DESCUENTOS MANUALES

Implementar descuentos:

- Por partida.
- Generales de cuenta.
- Por porcentaje.
- Por importe fijo.

Permitir descuentos de hasta el **100 %**, incluyendo cortesías.

### Permisos

Los roles pueden tener límites máximos de descuento.

Ejemplo:

- Cajero: hasta 10 %.
- Supervisor: hasta 30 %.
- Administrador: hasta 100 %.

Estos son ejemplos, no valores obligatorios predeterminados.

Los límites deberán ser configurables.

Si el usuario intenta superar su autorización:

- Solicitar autorización de un supervisor habilitado.
- Validar la autorización en Go.
- Registrar quién ejecutó y quién autorizó.
- Aplicar el descuento solo después de una autorización válida.

Si el usuario ya tiene permiso suficiente, no exigir una segunda contraseña innecesariamente.

### Autorización de supervisor

Una autorización puntual con contraseña no debe convertirse en un inicio de sesión que expulse al supervisor de su propia terminal.

Recordar que AD-EL POS ya utiliza una sesión activa por usuario.

Implementar un mecanismo apropiado de autorización puntual, limitado a la operación solicitada y sin almacenar contraseñas en auditoría.

Además:

- La autorización se valida en Go contra los **permisos efectivos actuales** del supervisor (rol + excepciones), en la misma transacción que la operación autorizada.
- Es de un solo uso y queda ligada a esa operación concreta (`operation_id`, cuenta e importe). No puede reutilizarse para otra.
- Aplica **límite de intentos fallidos** por supervisor y terminal, reutilizando el mecanismo de bloqueo del inicio de sesión. El diagnóstico (B14) señala que `/auth/password` no tiene límite; no repetir ese hueco aquí.
- Un usuario no puede autorizarse a sí mismo.

### Cortesías

Un descuento del 100 % puede dejar el importe a pagar en cero.

En ese caso, permitir liquidar la obligación comercial con registro de cortesía autorizado, **sin inventar un pago ficticio de $0**.

Conservar los productos vendidos y su preparación para las futuras estadísticas e inventario.

Registrar el importe descontado para reportes y futuros cortes de caja.

### Accesos de interfaz

La administración principal de descuentos debe existir fuera de la pantalla de cobro, desde las operaciones de cuenta.

En cobros puede existir un acceso contextual al mismo componente.

No duplicar las reglas en dos formularios diferentes.

---

## 12. CARGOS EXTRAS GENERALES

Implementar cargos manuales generales a una cuenta.

Cada cargo deberá registrar:

- Concepto.
- Importe.
- Usuario responsable.
- Fecha.
- Cuenta.
- Motivo cuando corresponda.

Ejemplos:

- Servicio adicional.
- Cargo especial autorizado.
- Ajuste comercial identificado.

**Estos cargos son distintos de los extras de productos, ingredientes y complementos.**

No obligar a asociarlos con un platillo.

El cargo debe reflejarse en el total de la cuenta y conservar su trazabilidad.

Su modificación o cancelación debe estar autorizada y auditada.

No permitir importes negativos como sustituto de descuentos.

La función principal debe estar disponible en la administración de la cuenta, con acceso contextual opcional desde la pantalla de cobro.

---

## 13. PROPINAS

Las propinas son opcionales.

Permitir registrar una propina cuando corresponda a un pago.

### Reglas

- Separar propinas de ingresos por venta.
- Registrar el método por el cual se recibió.
- Conservar importe, fecha, cajero y caja.
- Permitir consultar el total de propinas recibidas.
- Considerar correctamente el efectivo físicamente recibido.
- Mantener registros adecuados para reportes y cortes.

No sumar la propina al precio comercial de los productos.

No registrar automáticamente como propina el cambio que un cliente no recibió.

No implementar reparto, asignación a meseros ni liquidación interna de propinas.

**La distribución de propinas se administra fuera del POS.**

---

## 13 BIS. IVA (DECISIÓN DEL PROPIETARIO)

Hasta la etapa 5 no existen impuestos. Esta etapa incorpora el IVA con las siguientes reglas.

### Los precios incluyen IVA

Los precios del catálogo y los totales que ve el cliente **ya incluyen IVA**. El IVA no se suma al cobrar; se **desglosa** a partir del total.

Esto no cambia ningún precio ni importe existente.

### Tasa seleccionable, nunca capturada a mano

La tasa se configura a nivel del establecimiento eligiendo **una opción de una lista cerrada**:

- **16 %** (tasa general).
- **8 %** (región fronteriza).
- **0 %**.

Reglas:

- **No existe ningún campo de texto ni numérico para capturar la tasa.** La interfaz muestra únicamente estas tres opciones como botones o control segmentado.
- Go valida que la tasa sea exactamente una de las tres y rechaza cualquier otro valor.
- La base de datos refuerza la lista con una restricción `CHECK` (o una tabla de referencia de solo lectura sembrada por la migración). Ningún otro valor puede persistirse.
- Agrega un permiso específico para cambiar la tasa, concedido solo al administrador.
- Cada cambio de tasa se audita con el valor anterior, el nuevo, el usuario y la fecha.
- Para instalaciones existentes, la migración define **16 %** como valor inicial y la pantalla de configuración lo muestra claramente para que el establecimiento lo confirme o cambie.

### Conservación histórica

- La tasa aplicada se guarda con cada lote confirmado o cada venta (según lo que resulte más coherente con el modelo de `restaurant_item_units` y los snapshots por lote).
- **Cambiar la tasa solo afecta a lo que se confirme después del cambio.** Nunca recalcula cuentas, pagos, tickets ni devoluciones anteriores.

### Cálculo

- Todo el desglose se calcula en Go con `shopspring/decimal`.
- Base = total / (1 + tasa); IVA = total − base. Redondeo comercial a centavos, igual que el resto del sistema.
- **Base + IVA debe ser exactamente igual al total**, sin perder ni duplicar centavos. Documenta en qué nivel se calcula el desglose (por ticket, por cuenta o por unidad) y por qué ese nivel garantiza la coherencia.
- Los descuentos manuales y cortesías reducen el total y, por lo tanto, la base y el IVA.
- Los **cargos extras generales** forman parte de la venta y llevan IVA a la misma tasa.
- Las **propinas no llevan IVA** y no forman parte de la base gravable.
- Las devoluciones desglosan su IVA con la tasa de la venta original, no con la tasa vigente.

### Presentación

- El ticket y la precuenta muestran: subtotal sin IVA, IVA con su tasa, y total.
- Con tasa 0 %, mostrar el IVA en $0.00 con la leyenda de tasa 0 %.
- El ticket **no es un CFDI** (§15). La facturación electrónica queda fuera de alcance.

### Fuera de alcance

- Tasas por producto o por categoría.
- IEPS u otros impuestos.
- Productos exentos.

El modelo debe permitir agregar tasas por producto en el futuro sin rehacer los registros históricos.

---

## 14. CONFIGURACIÓN MÍNIMA DE TURNOS PARA COBRAR

La administración completa de caja se desarrollará en la etapa 7.

Sin embargo, la etapa 6 necesita una estructura financiera real para que cada pago pertenezca a un turno y caja específicos.

### Implementar ahora lo indispensable

- Identificación de caja física.
- Asociación con la terminal.
- Apertura mínima de turno.
- Identificación de quien abre.
- Fondo inicial obligatorio.
- Fondo inicial permitido de $0.
- Registro de fecha y hora.
- Estado de turno abierto.
- Asociación de cada cobro con el turno correspondiente.
- Registro persistente de movimientos financieros originados por cobros y devoluciones.

**No abrir turnos automáticamente con un fondo inventado.**

Si no existe turno abierto, solicitar su apertura antes de registrar cobros.

### Reglas

- Una caja física tiene como máximo un turno activo.
- Varios usuarios podrán utilizar el mismo turno únicamente si la configuración del establecimiento lo permite.
- Cada movimiento debe conservar el usuario que lo realizó.
- No suponer que cada cajero tiene necesariamente una caja física exclusiva.
- No generar pagos sin una caja y turno identificados.
- Evitar aperturas duplicadas por concurrencia.

### Preparación para etapa 7

El modelo debe permitir posteriormente:

- Cortes parciales.
- Cierre ciego.
- Conteo por denominaciones.
- Ingresos de efectivo.
- Retiros autorizados.
- Diferencias.
- Correcciones.
- Reportes de turno.
- Cierre de caja.

**No desarrollar toda esa interfaz todavía.**

No imponer la liquidación de cuentas pendientes como requisito para cerrar un turno en el futuro.

Conservar los pagos y movimientos de manera que la etapa 7 no tenga que reconstruirlos.

---

## 15. TICKETS Y PRECUENTAS

Reutilizar la infraestructura de impresión creada en la etapa 5.

### Precuenta

Permitir consultar e imprimir una precuenta antes de realizar el pago.

Debe mostrar:

- Productos.
- Cantidades.
- Personalizaciones relevantes.
- Precios.
- Descuentos.
- Cargos.
- Subtotal sin IVA, IVA con su tasa y total.
- Pagos registrados, cuando existan.
- Saldo pendiente.

No registrar una venta adicional por imprimirla.

### Ticket de pago

Al registrar un cobro, permitir imprimir el comprobante correspondiente.

El ticket debe mostrar los datos aplicables:

- Establecimiento.
- Folio.
- Fecha y hora.
- Cuenta o mesa.
- Productos.
- Total.
- Descuentos y cargos.
- Subtotal sin IVA, IVA con su tasa y total (§13 BIS).
- Abonos.
- Métodos utilizados.
- Importe efectivamente pagado.
- Saldo, si continúa pendiente.
- Efectivo recibido y cambio, cuando corresponda.
- Propina separada, si existe.

No confundir un ticket comercial con un CFDI.

La facturación electrónica se desarrollará posteriormente.

### Botón seleccionable de impresión

Incluir un botón táctil:

**Imprimir ticket**

Funcionará como un toggle.

Cuando esté activado debe resaltar visualmente mediante fondo y borde.

**No colocar un icono de checkbox ni una marca de verificación a la derecha.**

El cajero puede activarlo o desactivarlo antes de confirmar el cobro.

Permitir configurar su estado predeterminado por establecimiento o terminal cuando corresponda.

### Fallos de impresión

Un pago confirmado no debe revertirse porque falle la impresora.

Si la impresión falla:

- Conservar el pago.
- Registrar el trabajo pendiente o fallido.
- Permitir reintento según las reglas de la etapa 5.
- Evitar duplicaciones no controladas.
- Mostrar información clara al cajero.

No duplicar el pago al reimprimir.

El editor visual avanzado de tickets se implementará en la etapa 9. En esta etapa crear un formato base profesional y una separación adecuada de datos, plantilla y transporte.

---

## 16. ENTREGA DE PEDIDOS PARA LLEVAR

Mantener la modalidad única:

**Para llevar / Ventanilla / Drive-thru.**

No crear tres módulos diferentes.

### Botón seleccionable

En la pantalla de cobro de un pedido para llevar, incorporar:

**Marcar como entregado**

Debe funcionar como un botón seleccionable, resaltado al activarse.

No utilizar checkbox visual a la derecha.

Este botón no debe aparecer innecesariamente en el cobro normal de mesas de salón.

### Reglas

- El botón estará inicialmente desactivado.
- Si el cajero lo selecciona, al completar correctamente el cobro podrá registrarse también la entrega.
- Si el pago es parcial y todavía queda saldo, no marcar el pedido como entregado.
- Si existe saldo pendiente, la acción de entrega desde el tablero debe abrir el flujo real de cobro.
- Si ya está completamente pagado, permitir confirmar directamente la entrega.
- No considerar que un pedido pagado está automáticamente entregado.
- Conservar la fecha, usuario y terminal de entrega.
- Actualizar el tablero en tiempo real.

La entrega debe ser una operación idempotente y auditada.

No permitir dobles entregas por doble clic o reintentos.

### Estados independientes

Mantener totalmente independientes:

- Preparación.
- Pago.
- Entrega.

Un pedido puede estar listo y no pagado.

También puede estar pagado y pendiente de preparación.

Y puede estar listo y pagado, pero todavía no entregado.

No mezclar esas condiciones en un solo estado.

---

## 17. DEVOLUCIONES TOTALES Y PARCIALES

Implementar devoluciones sobre cuentas previamente pagadas.

### Modalidades

- Devolución total.
- Devolución parcial.
- Devolución de productos seleccionados.
- Devolución de cantidades parciales, cuando corresponda.

### Requisitos

- Identificar el pago y los productos relacionados.
- Comprobar cuánto puede devolverse.
- Impedir devolver dos veces la misma cantidad o importe.
- Registrar motivo.
- Registrar usuario.
- Registrar fecha, terminal y caja.
- Conservar los registros originales.
- Generar movimientos compensatorios.
- Actualizar el saldo o resultado financiero correspondiente.
- Mantener historial auditable.

No borrar ni sobrescribir el cobro original.

### Autorización

La autorización de supervisor debe ser **configurable**.

Si el cajero tiene permiso suficiente para ejecutar devoluciones, podrá hacerlo sin introducir una segunda contraseña.

Si no tiene autorización suficiente, solicitar aprobación válida de un supervisor.

No imponer aprobación de supervisor a todas las devoluciones indiscriminadamente.

### Método de devolución

El método utilizado para devolver dinero puede ser distinto del método original de pago.

Ejemplo:

El cliente pagó con tarjeta y el restaurante decide devolver efectivo.

El sistema debe registrar el método de salida real y su impacto correspondiente en caja.

Para devoluciones electrónicas, no afirmar que un banco procesó automáticamente el reembolso. Registrar únicamente la operación realmente confirmada por el procedimiento externo correspondiente.

### Inventario

No reintegrar automáticamente ingredientes al inventario por una devolución.

Una devolución financiera no significa que la comida pueda recuperar sus ingredientes físicos.

Conservar información para futuras decisiones sobre merma, cancelación y reversión de consumo.

---

## 18. CORRECCIONES DE PAGOS Y TURNOS

Contemplar errores de captura de pagos.

Ejemplo:

El cajero registró un cobro como efectivo, pero realmente fue tarjeta.

### Reglas

- No editar silenciosamente el registro original.
- Registrar una anulación o corrección compensatoria.
- Registrar el movimiento correcto.
- Mantener auditoría.
- Validar permisos o aprobación.
- No duplicar ingresos.
- No perder relación con cuenta y turno.

**Una corrección directa de un pago solamente estará permitida mientras el turno original permanezca abierto**, mediante operaciones compensatorias debidamente autorizadas.

Una vez cerrado el turno, no modificar retroactivamente sus movimientos.

Las regularizaciones posteriores deberán utilizar un procedimiento separado y auditado, cuya administración completa se finalizará en la etapa 7.

Aunque todavía no exista el cierre de turno completo, deja este invariante protegido desde el modelo financiero.

---

## 19. INTEGRIDAD FINANCIERA Y CONCURRENCIA

Esta parte es crítica.

Todos los cálculos monetarios deben realizarse en Go utilizando tipos decimales exactos.

No utilizar `float32`, `float64` ni `number` de JavaScript como autoridad para operaciones financieras.

### Invariantes

- No registrar dos veces un pago.
- No cobrar dos veces las mismas unidades.
- No perder centavos.
- No generar saldos negativos por errores.
- No permitir sobrepago electrónico sin una operación explícita compatible.
- No convertir importes excedentes automáticamente en propinas.
- No permitir devoluciones superiores al importe elegible.
- No modificar pagos históricos al cambiar productos.
- No dejar una cuenta en estado incoherente al fallar una transacción.
- No cambiar importes confirmados por modificaciones posteriores de precios de catálogo.

### Idempotencia

Utilizar identificadores únicos por operación, asociados a terminal y usuario según corresponda.

Si el cliente reintenta una operación porque perdió la respuesta, el servidor debe poder devolver el resultado persistido sin crear otro cobro.

Un reintento con el mismo identificador pero contenido diferente debe rechazarse como conflicto.

Persistir de manera transaccional la operación y su resultado.

### Concurrencia

Contemplar que dos terminales puedan intentar cobrar la misma cuenta.

Utilizar bloqueos, revisiones o mecanismos transaccionales apropiados.

Solo una operación compatible debe aplicarse sobre un saldo determinado.

Si una terminal utiliza información obsoleta, debe actualizarse y mostrar el saldo real, no sobrescribirlo.

### Auditoría

Registrar operaciones financieras significativas con actor, terminal, caja, turno, fecha, resultado y cambios relevantes.

No registrar contraseñas, tokens ni secretos.

---

## 20. CAMBIOS EN PEDIDOS DESPUÉS DE RECIBIR PAGOS

El POS permite modificar pedidos confirmados y agregar o cancelar productos.

Eso debe continuar funcionando, pero ahora habrá pagos involucrados.

### Agregar productos

Si una cuenta ya recibió un pago y posteriormente se agregan productos, actualizar el nuevo saldo pendiente sin modificar los pagos anteriores.

### Modificar productos

Si un cambio incrementa el total, registrar correctamente el incremento.

Si disminuye el total, comprobar los pagos existentes.

No generar saldos negativos o devoluciones implícitas.

### Cancelar productos

Si el producto todavía no ha sido pagado, aplicar la modificación comercial conforme a las reglas existentes.

Si ya fue pagado total o parcialmente, exigir la devolución o ajuste autorizado que corresponda.

Mantener separadas:

- Modificación de pedido.
- Cancelación de preparación.
- Devolución financiera.
- Ajuste de saldo.
- Movimiento de inventario futuro.

No reutilizar una sola operación para representar todas esas acciones.

---

## 21. PERMISOS Y AUDITORÍA

Extender el sistema actual de permisos.

Incluir permisos granulares para:

- Consultar información financiera de una cuenta.
- Registrar cobros.
- Registrar pagos parciales.
- Cobrar productos seleccionados.
- Administrar métodos de pago.
- Aplicar descuentos.
- Autorizar descuentos superiores al límite.
- Otorgar cortesías.
- Registrar cargos extras generales.
- Modificar cargos extras.
- Registrar propinas.
- Consultar pagos.
- Anular o corregir pagos.
- Ejecutar devoluciones.
- Autorizar devoluciones.
- Imprimir tickets.
- Reimprimir tickets.
- Abrir turno de caja.
- Consultar movimientos del turno.
- Registrar entrega de pedidos para llevar.
- Configurar la tasa de IVA.

Reutilizar roles y excepciones individuales de **Heredar / Permitir / Denegar**.

Todas las validaciones reales deben hacerse en Go.

No confiar en que un botón está oculto para proteger una operación.

Sigue el patrón de `CLAUDE.md` §20:

- Cada permiso nuevo se registra en Go **y** en la migración (`INSERT INTO app.permissions` + concesión al rol `administrator`), porque las instalaciones existentes no vuelven a ejecutar el asistente.
- **No asignar permisos nuevos a otros roles por inferencia.** El establecimiento los asignará desde la administración de roles.
- **Todas las comprobaciones de permisos y autorizaciones ocurren antes de la primera escritura** de la operación (riesgo H2).
- Los límites de descuento por rol se guardan como configuración del rol, se validan en Go y quedan sujetos a las mismas reglas de delegación segura de H1: nadie puede asignarse ni asignar a otros un límite mayor que el suyo.

La interfaz debe mostrar nombres legibles de permisos, no identificadores técnicos sin interpretar.

---

## 22. API, EVENTOS Y FRONTEND

Crear endpoints versionados, consistentes con los contratos existentes.

No establecer una segunda API financiera paralela.

Reutilizar:

- Sesiones.
- Identidad de terminales.
- Permisos.
- Auditoría.
- Manejo de errores.
- SSE autenticado.
- Infraestructura de impresión.
- Contratos de cuentas y pedidos.

### Actualizaciones en tiempo real

Cuando se registre un pago o cambie el saldo:

- Actualizar la cuenta.
- Actualizar las terminales conectadas.
- Reflejar el movimiento en su historial.
- Actualizar el tablero para llevar cuando corresponda.
- Actualizar el estado de liquidación.
- Registrar entrega cuando se confirme.

Conservar recuperación ante pérdida de conexión y evitar eventos duplicados.

No sustituir SSE por WebSockets sin una razón técnica concreta.

### Interfaz

Implementar estados claros:

- Capturando pago.
- Validando.
- Registrando.
- Pago registrado.
- Saldo pendiente.
- Cuenta liquidada.
- Error recuperable.
- Conflicto de cobro.
- Impresión pendiente.
- Entrega realizada.

Después de registrar un pago, no permitir que el usuario lo envíe otra vez por un doble clic accidental.

### Operaciones inciertas a nivel de aplicación (M8)

El diagnóstico encontró que el UUID de una operación incierta de cocina vive en el estado de la página y se pierde al navegar; al volver se genera uno nuevo.

**Para cobros eso no es aceptable.** Si una respuesta de pago se pierde y el cajero cambia de pantalla, al volver podría registrar el pago dos veces.

- Crea un **almacén de operaciones pendientes a nivel de aplicación** (fuera de cualquier página que se desmonte), que conserve el `operation_id` y el cuerpo exacto de cada operación financiera con resultado incierto.
- Mientras exista una operación incierta sobre una cuenta, la interfaz debe ofrecer **reintentar con el mismo identificador** o consultar el resultado, y bloquear nuevos cobros sobre esa cuenta hasta resolverla.
- Reutiliza `useRestaurantMutation` como base. Si conviene, migra también los pendientes de cocina a este almacén, sin cambiar su comportamiento aprobado.

No crear ventanas modales innecesarias para operaciones simples.

---

## 23. BASE DE DATOS Y MIGRACIONES

### Ubicación del módulo de pagos (decisión del propietario)

Crea un módulo nuevo en Go: **`backend/internal/payments`** (o un nombre equivalente y descriptivo).

- `catalog` sigue siendo la **única autoridad de precios**. No metas lógica de cobros en `catalog`.
- `payments` es la autoridad de saldos, pagos, asignaciones a unidades, cambio, descuentos manuales, cortesías, cargos, propinas, IVA, devoluciones, correcciones, turnos mínimos y movimientos.
- Reutiliza los importes históricos de `restaurant_item_units`; no los recalcules.
- Las rutas se delegan desde `administration/routes.go` con la misma transacción, `Actor` y callback de auditoría que los demás módulos.
- Las operaciones de pago toman **primero el lock 7820148305** (restaurante y cocina), porque comparten el agregado de la cuenta con transferencias y combinaciones. Cualquier lock nuevo se toma siempre después (ver tabla de locks en `CLAUDE.md` §3). No reutilizar números de lock existentes.
- Traduce los errores del módulo a `Fault` en `routes.go` (el diagnóstico B9 señala que hoy un error de cocina dentro de una operación comercial sale como 503 genérico).
- Si necesitas un repartidor proporcional de importes (descuentos de cuenta, división financiera), créalo en `payments` con pruebas unitarias. `unitMoney` hoy solo reparte en partes iguales.
- Agrega el paquete nuevo a la tupla `packages` de `scripts/database.py`; si no, `test-integration` no ejecuta sus pruebas.

### Esquema

Partir del esquema 5 existente.

Notas del diagnóstico para la migración 0006:

- Las restricciones `CHECK` sin nombre de 0004/0005 (por ejemplo, los estados de `restaurant_accounts`) se redefinen con `ALTER TABLE … DROP/ADD CONSTRAINT` en la migración nueva.
- `payment_state` y `delivery_state` hoy son literales fijos en `accounts_read.go` y `services/restaurant.ts`. Deben persistirse o derivarse de los registros financieros reales.
- **No reutilizar `restaurant_item_units.discount`**, que significa descuento de promoción. Los descuentos manuales van en sus propios registros.
- Pagos, asignaciones, devoluciones y movimientos son **inmutables**: las correcciones se hacen con registros compensatorios enlazados, nunca con UPDATE ni DELETE.
- Una apertura activa por caja: índice único parcial.
- Al cambiar la caja de una terminal o desactivar una caja, verificar que no tenga un turno abierto (`devices.go`, `registers.go`).

**Antes de aplicar la migración a la instalación real**, pídeme autorización. Las pruebas se ejecutan en bases desechables.

Crear la nueva migración o migraciones que correspondan.

**No modificar las migraciones aplicadas anteriormente.**

Antes de migrar:

1. Revisar el esquema real.
2. Generar respaldo.
3. Restaurarlo en una base temporal.
4. Comprobar integridad.
5. Aplicar migraciones.
6. Ejecutar pruebas.
7. Conservar los datos originales.

Diseñar las entidades necesarias para:

- Métodos de pago.
- Pagos.
- Asignaciones de pago a partidas/cantidades.
- Saldos.
- Movimientos financieros.
- Apertura mínima de turnos.
- Fondos iniciales.
- Descuentos.
- Autorizaciones.
- Cargos generales.
- Propinas.
- Devoluciones.
- Anulaciones y correcciones.
- Impresiones de tickets.
- Precuentas.
- Entregas.
- Referencias externas.
- Eventos y auditoría relacionados.

No crear tablas redundantes si las entidades existentes pueden extenderse de forma segura.

Documentar las relaciones, restricciones, índices, claves idempotentes y reglas transaccionales.

Preparar el modelo para que la etapa 7 pueda implementar cortes sin modificar pagos históricos.

---

## 24. PRUEBAS DE ACEPTACIÓN

Implementa y ejecuta pruebas reales de todas las áreas importantes.

### Cobros

1. Cobro total en efectivo.
2. Cobro total con tarjeta.
3. Cobro parcial.
4. Pagos mixtos sucesivos.
5. Cambio en efectivo.
6. Cambio al recibir más efectivo que el importe de un abono.
7. Referencia electrónica opcional.
8. Referencia obligatoria cuando se configure.
9. Rechazo de importes inválidos.
10. Rechazo de sobrepagos indebidos.

### Productos y cuentas

11. Cobro de una partida específica.
12. Cobro de una unidad de una partida con varias unidades.
13. Cobro parcial de una selección.
14. División de cuentas sin duplicar productos.
15. Conservación de pagos al dividir.
16. Transferencia de cantidades pendientes.
17. Rechazo de transferencias que alteren pagos existentes.
18. Integridad de combos.
19. Asignación exacta de descuentos y centavos.
20. Incorporación de nuevos productos tras un pago parcial.

### Descuentos y extras

21. Descuento por producto.
22. Descuento general.
23. Descuento de 100 %.
24. Cortesía sin pago ficticio.
25. Límite de descuento por perfil.
26. Autorización puntual de supervisor.
27. Autorización sin expulsar la sesión previa del supervisor.
28. Cargo extra general.
29. Cargo extra independiente de complementos.
30. Cambios auditados.

### Propinas y devoluciones

31. Propina en efectivo.
32. Propina con método electrónico.
33. Separación entre venta y propina.
34. Devolución total.
35. Devolución parcial.
36. Devolución por productos.
37. Método de devolución distinto al original.
38. Prevención de devoluciones duplicadas.
39. Devolución con permiso directo.
40. Devolución con autorización de supervisor.
41. Corrección de método de pago durante turno abierto.
42. Conservación de los movimientos originales.

### Caja

43. Apertura mínima de turno.
44. Fondo inicial de $0.
45. Rechazo de cobro sin turno abierto.
46. Una sola apertura activa por caja.
47. Asociación de movimiento con caja, turno y usuario.
48. Concurrencia entre cajeros.
49. No duplicación de ingresos.
50. Conservación del saldo al reiniciar Go.

### Tickets y entrega

51. Precuenta antes del pago.
52. Ticket después del pago.
53. Toggle de imprimir ticket.
54. Pago válido aunque falle la impresora.
55. Reimpresión sin duplicar cobros.
56. Toggle de marcar como entregado.
57. Pedido parcialmente pagado no entregable.
58. Pedido pagado que permanece pendiente de entrega.
59. Entrega directa de pedido ya liquidado.
60. Actualización del tablero para llevar.

### Integridad y seguridad

61. Reintento de pago con el mismo identificador.
62. Conflicto al reutilizar identificador con otro contenido.
63. Doble clic de pago.
64. Cobro simultáneo desde dos terminales.
65. Sesión sin permisos.
66. Sesión revocada.
67. Auditoría financiera.
68. Cálculos decimales exactos.
69. Conservación de precios confirmados.
70. Fallo transaccional sin cobro parcial inconsistente.

### IVA

A1. Selección de tasa 16 %, 8 % y 0 % desde la configuración.
A2. Rechazo en Go y en PostgreSQL de cualquier tasa fuera de la lista (por ejemplo 15 %, 0.16 escrito como 16, valores negativos o texto).
A3. Desglose exacto: base + IVA = total en todos los casos, incluidos importes con centavos difíciles.
A4. Cambio de tasa sin alterar cuentas, pagos, tickets ni devoluciones anteriores.
A5. Descuento y cortesía reducen base e IVA; propina sin IVA; cargo extra con IVA.
A6. Devolución desglosada con la tasa de la venta original después de cambiar la tasa.
A7. Cambio de tasa auditado y protegido por permiso.

### Partidas con pago (§9)

P1. Bloqueo de transferir, cancelar, editar y anular unidades liquidadas.
P2. Bloqueo de las mismas operaciones sobre unidades con pago parcial.
P3. Transferencia permitida de unidades sin pago en una cuenta con abonos generales.
P4. Bloqueo de un movimiento que deje la cuenta con total menor que sus pagos netos.
P5. Bloqueo de combinar cuando la cuenta de origen tiene pagos; combinación permitida cuando solo el destino los tiene.
P6. Unidad que vuelve a ser movible después de revertir su pago con devolución autorizada.

### Seguridad heredada

S1. Pruebas negativas de H1 y M1 (si se corrigieron en esta etapa).
S2. Un 403 en una operación financiera no deja ninguna escritura confirmada (H2).
S3. Un usuario no puede asignarse ni asignar a otros un límite de descuento mayor que el suyo.
S4. Límite de intentos fallidos en la autorización de supervisor.
S5. Un usuario no puede autorizarse a sí mismo.
S6. Operación de pago incierta que conserva su identificador al cambiar de pantalla y volver (M8).
S7. Teclado de cobro: `+`, `.` y doble punto no vacían ni corrompen el importe (M2).

### Regresiones

71. Conservación del mapa de mesas.
72. Conservación de captura y personalización.
73. Conservación de complementos y combos.
74. Conservación de promociones.
75. Conservación de rutas y comandas.
76. Conservación de eventos SSE.
77. Conservación de usuarios y permisos.
78. Conservación de migraciones previas.
79. Go race detector y vet.
80. Pruebas PostgreSQL.
81. Pruebas frontend y TypeScript estricto.
82. Pruebas Rust/Tauri.
83. Compilación macOS.
84. Inicio en pantalla completa.
85. Independencia del backend respecto de Tauri.

Puedes agrupar escenarios relacionados en pruebas parametrizadas o recorridos integrales. No necesitas crear exactamente 85 funciones de prueba.

**No declares como superada una prueba que no hayas ejecutado.**

---

## 25. FUERA DEL ALCANCE

No implementar todavía:

- Administración completa de cortes de caja.
- Arqueos.
- Cierre ciego.
- Ingresos y retiros manuales de efectivo.
- Conteo por denominaciones.
- Reportes financieros completos.
- Inventarios y compras.
- Distribución interna de propinas.
- Integración directa con terminales bancarias.
- Facturación electrónica.
- Diseñador visual avanzado de tickets.
- Aplicación Android.
- Aplicación independiente KDS.
- Servidor comercial de licencias.
- Migrador SQL Server.
- Instaladores Windows.

La apertura mínima de turno y los movimientos de pagos sí forman parte de esta etapa porque son indispensables para cobrar correctamente.

No introducir botones decorativos que simulen las funciones excluidas.

---

## 26. DOCUMENTACIÓN

Actualizar:

- `CLAUDE.md`: estado de la etapa 6, versión y esquema, módulo `payments`, locks nuevos, decisiones de IVA y de partidas con pago, y los riesgos del diagnóstico que queden resueltos o sigan abiertos.
- `docs/DIAGNOSTICO_ETAPAS_1_5.md`: no se reescribe; agrega al final una sección breve que indique qué hallazgos (H1, H2, H3, M1, M2, M8, M11, B9, B14…) quedaron resueltos en la etapa 6 y dónde.
- `AGENTS.md`, si corresponde.
- `README.md`.
- `ARCHITECTURE.md`.
- `API_CONTRACTS.md`.
- `NEXT_STEPS_FOR_SOL.md`.
- `DEVELOPMENT_MACOS.md`.
- `docs/VERIFICATION.md`: archiva la versión actual como `docs/VERIFICATION_STAGE5.md` y crea la nueva solo con lo realmente ejecutado.
- Documentos operativos afectados.

Crear una guía específica, por ejemplo:

`docs/PAYMENTS_AND_REFUNDS.md`

Documentar:

- Estructura de pagos.
- Métodos.
- Pagos parciales y mixtos.
- Cobro por partidas.
- Distribución de importes.
- Efectivo y cambio.
- Descuentos.
- Cargos extras generales.
- Propinas.
- IVA: tasas permitidas, cálculo, redondeo y conservación histórica.
- Reglas sobre partidas con pago.
- Devoluciones.
- Autorizaciones.
- Apertura mínima de turno.
- Movimientos financieros.
- Correcciones.
- Impresión de tickets.
- Entrega de pedidos.
- Concurrencia e idempotencia.
- Contratos para etapa 7.

Documentar únicamente lo que realmente funcione.

No presentar funcionalidades pendientes como implementadas.

---

## 27. ENTREGA FINAL

Al terminar cada fase (6A, 6B, 6C), registra su resumen en `docs/ETAPA_6_AVANCE.md`, haz su commit local y continúa con la siguiente según las reglas de §0.

Al terminar la fase 6D, proporciona la entrega final completa:

1. Resumen de funcionalidades implementadas.
2. Migraciones nuevas y estado del esquema.
3. Nuevas rutas API.
4. Pantallas incorporadas y modificadas.
5. Flujo de cobro rápido.
6. Funcionamiento de pagos mixtos y parciales.
7. Funcionamiento del cobro por productos y cantidades.
8. Cálculo de cambio.
9. Descuentos y autorizaciones.
10. Cargos extras generales.
11. Propinas.
12. Devoluciones y correcciones.
13. Apertura mínima de turno.
14. Integración con caja física.
15. Impresión de tickets y precuentas.
16. Entrega de pedidos para llevar.
    16-A. IVA: configuración, cálculo y presentación.
    16-B. Reglas sobre partidas con pago y hallazgos del diagnóstico resueltos.
17. Resultado de pruebas realmente ejecutadas.
18. Limitaciones pendientes.
19. Comandos exactos para iniciar y probar.
20. Confirmación de que las etapas anteriores continúan funcionando.

### CRITERIO FINAL DE ACEPTACIÓN

Debo poder abrir AD-EL POS en mi Mac, iniciar sesión y seleccionar una cuenta real.

Desde ahí debo poder:

- Consultar los productos.
- Abrir la pantalla de cobro.
- Seleccionar el método mediante botones táctiles rectangulares con icono y texto.
- Utilizar el teclado numérico fijo.
- Capturar efectivo recibido.
- Ver claramente el cambio.
- Registrar pagos totales.
- Registrar abonos parciales.
- Completar pagos mixtos mediante movimientos sucesivos.
- Cobrar solamente los productos y cantidades seleccionados.
- Dividir cuentas sin perder pagos ni partidas.
- Aplicar descuentos autorizados.
- Registrar cargos generales.
- Registrar propinas separadas.
- Seleccionar la tasa de IVA (16 %, 8 % o 0 %) sin poder capturarla a mano, y verla desglosada en precuenta y ticket.
- Comprobar que una partida ya pagada no puede transferirse ni cancelarse sin una devolución autorizada.
- Consultar y ejecutar devoluciones con permisos.
- Imprimir o no imprimir ticket mediante un botón seleccionable, sin checkbox a la derecha.
- Marcar entregado un pedido para llevar únicamente cuando esté liquidado.
- Consultar los movimientos persistidos en PostgreSQL.
- Ver que las demás terminales reciban los cambios mediante los mecanismos existentes.
- Confirmar que un doble clic o una reconexión no duplican pagos.

Todo debe respetar los permisos, la auditoría, las transacciones y los cálculos monetarios exactos.

No quiero maquetas ni cobros simulados.

**Implementa, integra, prueba y documenta completamente la etapa 6 por fases (6A → 6D), avanzando de una a otra cuando cada fase quede funcional y probada, conservando las cinco etapas anteriores y preparando correctamente la administración de caja de la etapa 7.**
