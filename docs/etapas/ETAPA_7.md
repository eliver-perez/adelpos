# AD-EL POS — ETAPA 7: ADMINISTRACIÓN DE CAJA, ARQUEOS, MOVIMIENTOS Y CORTES

## Contexto del desarrollo

Continúa el desarrollo de AD-EL POS, sistema comercial de punto de venta para restaurantes.

Actúa como arquitecto e ingeniero senior full stack especializado en Go, PostgreSQL, React, TypeScript, Tailwind CSS y Tauri 2.

Las etapas 1 a 6 están implementadas, incluyendo las mejoras de la etapa 6.1.

La versión de referencia reportada es **0.6.1, esquema PostgreSQL 7**.

Tu tarea es desarrollar la etapa 7 completa: **administración de turnos de caja, ingresos, retiros, arqueos, cortes parciales, cierre ciego, diferencias, autorizaciones, impresión de cortes y consulta histórica**.

Debes ampliar la infraestructura financiera existente, no crear una paralela.

La etapa debe quedar funcional, integrada con las operaciones comerciales, persistida en PostgreSQL, protegida mediante permisos y cubierta por pruebas automatizadas y recorridos de aceptación.

Al terminar, la versión será **0.7.0**, con el esquema que resulte de las migraciones nuevas.

---

## 0. FORMA DE TRABAJO

### Rama

Trabaja en una rama nueva **`etapa-7`** creada desde `main` (que contiene hasta la 6.1). No hagas push ni merge; el propietario los hará después de probar.

### Fase 0: verificación contra el código (obligatoria, antes de diseñar)

Esta especificación se escribió sin ver el código. Antes de implementar, comprueba en el repositorio real y registra el resultado en `docs/ETAPA_7_AVANCE.md`:

1. Qué existe ya de cada sección de este documento (turno mínimo, fondo inicial, tipo de cambio por turno, movimientos por moneda, políticas de turno compartido, autorización puntual, funciones activables, colas de impresión, transporte de cajón). **Reutiliza todo lo que exista.**
2. Los nombres reales de tablas, columnas, estados, locks y permisos. Si difieren de los que menciona este documento, **mandan los reales**.
3. Si alguna regla de este documento contradice una decisión registrada en `CLAUDE.md` o en el código de las etapas 6 y 6.1, señálala.
4. Revisa en el sistema anterior (`CLAUDE.md` §21) `IniciarCorte.cs`, `CorteParcial.cs`, `CerrarCorte.cs`, `Egresos.cs`, `RegistroEgresos.cs` y las tablas `cortes`, `cortes_parciales`, `salidas` y `topes_caja`, y reporta funciones que tenía y no estén aquí.

Si la verificación no encuentra contradicciones de fondo, continúa sin esperar. Si encuentra una que obligue a cambiar una decisión del propietario, **detente y pregunta**.

### Fases

| Fase | Contenido |
| ---- | --------- |
| **0** | Verificación contra el código (arriba). |
| **A** | Migración, modelo y backend: apertura ampliada, denominaciones, ingresos, retiros, gastos, correcciones, cálculo del efectivo esperado, cortes parciales, arqueo, cierre ciego, diferencias, regularización, permisos, concurrencia e idempotencia, con pruebas de integración. |
| **B** | Interfaz: panel de caja, apertura, movimientos, arqueo y cierre ciego, historial y reportes operativos. |
| **C** | Impresión de comprobantes y cortes, cajón de dinero, tope de caja y eventos SSE. |
| **D** | Regresiones completas, documentación y entrega final. |

**Al terminar cada fase:** ejecuta sus pruebas y las suites afectadas, aplica `./scripts/dev.sh format` y `format-check`, registra un resumen en `docs/ETAPA_7_AVANCE.md`, haz un commit local y **continúa con la siguiente fase sin esperar confirmación** si todo pasa.

**Detente y pregunta únicamente si:** una prueba falla y arreglarla exige cambiar una decisión de este documento; encuentras una contradicción que obligue a una decisión funcional importante; necesitas aplicar la migración a la instalación real; o una acción podría afectar datos o servicios fuera del alcance. Ante dudas menores, toma la opción más conservadora, anótala en el avance y continúa.

---

## 0-B. DECISIONES DEL PROPIETARIO Y ACLARACIONES

Estas decisiones resuelven ambigüedades de las secciones siguientes. **Si una sección parece decir otra cosa, prevalece esta.**

### Terminología

En el sistema anterior, «Corte parcial» era la pantalla para **retirar** dinero de la caja. En AD-EL POS son dos cosas distintas y la interfaz debe dejarlo claro:

- **Retiro de efectivo**: saca dinero de la caja. Movimiento físico.
- **Corte parcial**: informe del turno hasta ese momento. No mueve dinero.

Ofrece un acceso directo «Retirar efectivo» desde la pantalla del corte parcial, para quien esté acostumbrado al flujo anterior.

### Autorizaciones de salidas de efectivo

- **Retiros y gastos requieren siempre autorización de supervisor.** Toda salida de efectivo queda autorizada, igual que en el sistema anterior.
- **Si quien registra la operación tiene el permiso de autorizarla, su propio permiso basta**: no se pide una segunda contraseña y el registro queda como autorizado por él mismo. Es el mismo criterio que los descuentos de la etapa 6.
- Si no tiene ese permiso, se usa la autorización puntual de un supervisor, que debe ser **otra persona**.
- Las correcciones de ingresos y retiros y las regularizaciones siguen la misma regla.

### Cierre ciego y permisos de consulta

- **La captura del arqueo de cierre es siempre ciega**, para todos los usuarios, incluido el administrador. Durante la captura, la API no devuelve efectivo esperado ni diferencias a nadie.
- **Después del cierre**, ver el efectivo esperado y las diferencias depende del permiso «Consultar importes esperados y diferencias». Sin ese permiso, el cajero ve únicamente lo que declaró y que el turno quedó cerrado.
- El mismo permiso controla, **durante el turno**, cualquier dato que permita deducir el efectivo esperado:
  - **Corte parcial**: sin el permiso, muestra ventas por método, propinas, ingresos, retiros y gastos, pero **no** el efectivo esperado en caja por moneda.
  - **Retiros y gastos que excedan el efectivo disponible**: el rechazo no debe revelar cuánto efectivo hay. El mensaje dice que el importe no puede autorizarse y que lo revise un supervisor, sin cifras.
  - **Tope de caja**: el aviso solo lo ven usuarios con el permiso.
- Revisa todos los endpoints del módulo para que ninguno filtre el esperado a quien no tenga el permiso, directa o indirectamente.

### La caja se pausa durante el arqueo de cierre

Para que el conteo no quede desfasado por cobros de otras terminales, sin mantener bloqueos de base de datos mientras se cuentan billetes:

- Al **iniciar el arqueo de cierre**, el turno pasa a un estado persistente «Cierre en curso» (con quién lo inició, cuándo y desde qué terminal).
- Mientras está en ese estado, **el turno no acepta cobros, devoluciones, ingresos, retiros, gastos ni correcciones**. Las terminales que intenten registrarlos reciben un mensaje claro: «La caja X está en arqueo de cierre».
- El arqueo se puede **cancelar** (vuelve a «Abierto») o **confirmar** (cierra el turno). Ambas operaciones son idempotentes y auditadas. Cancelar un arqueo ajeno requiere permiso.
- Las cuentas abiertas siguen abiertas. Si el restaurante necesita cobrar mientras una caja cuenta, lo hace en otra caja con turno abierto.
- La confirmación vuelve a verificar, dentro de su transacción, que el turno sigue en «Cierre en curso» y que no se registraron movimientos después de iniciar el arqueo.
- Los **cortes parciales** no pausan la caja.

### Operaciones inciertas

El servidor es la fuente de verdad: una operación está en el turno si su transacción se confirmó. Al cerrar o pausar un turno, cualquier reintento posterior de una operación incierta sobre ese turno será rechazado con un error claro y, si sigue haciendo falta, deberá registrarse como operación nueva en un turno abierto. La interfaz debe explicarlo al cajero, usando el almacén global de operaciones pendientes de la etapa 6.

### Fecha comercial

La **fecha comercial** de un turno es la fecha de su apertura en la zona horaria del establecimiento. Los reportes filtran por fecha comercial; los movimientos conservan además su fecha y hora reales.

### Regularizaciones posteriores al cierre

- Se registran en el **turno abierto vigente** de la misma caja, enlazadas al movimiento original del turno cerrado, con motivo y autorización.
- **No cambian la diferencia registrada del turno cerrado.** Los reportes muestran la diferencia original y, aparte, las regularizaciones enlazadas.
- No existe un movimiento de tipo «ajuste de diferencia». Una regularización solo corrige un movimiento identificado (por ejemplo, un retiro capturado con importe equivocado).

### Tope de caja (función activable)

Basado en `topes_caja` del sistema anterior. Usa el patrón de funciones activables de la etapa 6.1, **desactivada por defecto**:

- Un importe máximo de efectivo por caja, en MXN (y en USD si esa función está activa), configurable con permiso y auditado.
- Cuando el efectivo esperado supera el tope, los usuarios con permiso de consultar importes esperados ven un aviso sugiriendo un retiro. **No bloquea cobros.**

### Denominaciones

- Las denominaciones son un catálogo configurable por moneda, sembrado por la migración con las de México (billetes de 1000, 500, 200, 100, 50 y 20; monedas de 20, 10, 5, 2, 1 y 0.50) y, para USD, billetes de 100, 50, 20, 10, 5 y 1. Las monedas de USD son opcionales.
- **El arqueo de cierre se captura por denominaciones.** Se admite un importe adicional identificado (por ejemplo, monedas sueltas sin desglosar).
- **La apertura** puede registrarse por total o por denominaciones. Con fondo de $0 no se pide desglose.

---

## 1. Revisión obligatoria del repositorio

Antes de modificar código, revisa los archivos reales y su contenido:

- `CLAUDE.md`
- `AGENTS.md`
- `README.md`
- `ARCHITECTURE.md`
- `API_CONTRACTS.md`
- `NEXT_STEPS_FOR_SOL.md`
- `DEVELOPMENT_MACOS.md`
- `docs/PAYMENTS_AND_REFUNDS.md`
- `docs/RESTAURANT_OPERATIONS.md`
- `docs/KITCHEN_AND_PRINTING.md`
- `docs/VERIFICATION.md`
- Los documentos y registros de las etapas 6 y 6.1.
- Las migraciones 0001–0007.
- Los módulos reales de backend, frontend, Tauri, permisos, impresión y eventos.

Inspecciona específicamente:

- `backend/internal/payments`
- `cash_shifts`
- `cash_movements`
- `payment_methods`
- `payments`
- `payment_refunds`
- Los mecanismos de autorización de supervisor.
- Las operaciones idempotentes.
- El sistema de auditoría.
- La infraestructura de impresión persistente.
- La configuración de funciones activables.
- Los movimientos de efectivo en MXN y USD.

No asumas que el modelo es exactamente como aparece en esta especificación: comprueba cómo está implementado.

Si algún requisito ya existe, reutilízalo.

No modificar migraciones aplicadas ni reconstruir tablas financieras.

No alterar registros históricos.

No introducir otra arquitectura, lenguaje, base de datos o servicio externo.

No modificar el mecanismo reservado de soporte `sa`.

El entorno de desarrollo y pruebas continúa siendo macOS. Windows deberá contemplarse en la arquitectura, pero no se solicitan todavía instaladores ni pruebas comerciales de Windows.

---

## 2. Objetivo funcional

El módulo debe permitir administrar la operación completa de efectivo y turnos de caja.

Incluye:

1. Consultar cajas físicas.
2. Abrir turnos y registrar fondos iniciales.
3. Consultar turnos activos.
4. Registrar ingresos de efectivo.
5. Registrar retiros de efectivo.
6. Registrar gastos pagados desde caja.
7. Autorizar operaciones que lo requieran.
8. Consultar movimientos por turno.
9. Realizar cortes parciales informativos.
10. Registrar conteos físicos.
11. Realizar cierre ciego.
12. Calcular diferencias.
13. Permitir cierres con diferencias.
14. Consultar turnos cerrados.
15. Imprimir cortes y comprobantes.
16. Administrar efectivo en pesos y dólares cuando la función USD esté activada.
17. Registrar correcciones sin modificar operaciones históricas.
18. Conservar información para reportes y auditoría.

Todas estas funciones deben utilizar los movimientos financieros reales existentes.

No crear ventas, pagos ni importes ficticios para calcular cortes.

---

## 3. Reglas permanentes de desarrollo

Mantener las convenciones existentes.

### Código legible

- No ofuscar ni compactar código.
- No escribir bloques Go completos en una sola línea.
- JSX/TSX correctamente indentado y organizado.
- Comentarios en español.
- Nombres descriptivos.
- Separación de responsabilidades.
- Código mantenible.
- Validaciones de permisos en Go.
- Operaciones financieras transaccionales.
- Cálculos con decimales exactos.
- Formato mediante gofmt, Prettier y Rustfmt.

No introducir cálculos monetarios autoritativos en React utilizando números de punto flotante.

### Interfaz

Conservar:

- Temas claro y oscuro.
- Identidad visual de AD-EL POS.
- Acordeones elegantes existentes.
- Pantalla completa nativa predeterminada.
- Diseño adaptable.
- Controles táctiles moderados.
- Interfaz compacta y profesional.
- Componentes reutilizables.
- Mensajes y permisos legibles.

No limitar artificialmente las pantallas a 1280 × 720.

---

## 4. Cajas físicas y turnos

La etapa 2 creó el concepto de caja física y la etapa 6 incorporó una apertura mínima de turno.

**Extiende estas implementaciones. No las dupliques.**

Cada caja debe tener identidad persistente y una configuración compatible con las terminales autorizadas.

### Turnos

Permitir:

- Consultar turno abierto.
- Consultar quién lo abrió.
- Consultar fecha y hora de apertura.
- Consultar caja y terminal asociadas.
- Consultar otros operadores autorizados cuando el turno sea compartido.
- Registrar fondo inicial.
- Consultar movimientos.
- Registrar cortes parciales.
- Iniciar arqueo de cierre.
- Cerrar turno.
- Consultar historial.

### Reglas

- Una caja física puede tener como máximo un turno abierto.
- No permitir dos aperturas simultáneas.
- El fondo inicial es obligatorio, pero puede ser $0.
- No inventar automáticamente un fondo inicial.
- Registrar responsable y terminal.
- Conservar la configuración existente de turnos compartidos.
- Si los turnos compartidos están desactivados, respetar al operador autorizado.
- Si están activados, identificar a cada usuario que realiza movimientos.
- Una cuenta comercial puede permanecer abierta después del cierre de un turno.
- Una mesa ocupada no impide cerrar caja.

Si durante el turno se cambia de cajero, conservar el historial de operaciones y el responsable de cada movimiento.

No modificar sesiones de usuario para cambiar el responsable del turno.

---

## 5. Apertura de turno

Ampliar la apertura existente para ofrecer un flujo cómodo y completo.

### Campos

- Caja.
- Usuario responsable.
- Fondo inicial en MXN.
- Fondo inicial en USD cuando la función esté activada.
- Fecha y hora proporcionadas por Go.
- Observaciones opcionales.
- Tipo de cambio cuando corresponda.

Si no se utilizan dólares, no mostrar campos innecesarios.

### Conteo por denominaciones

Permitir desglosar el fondo inicial por billetes y monedas.

El usuario también podrá registrar directamente el total cuando tenga permiso o la configuración del establecimiento lo permita.

No obligar a contar por denominaciones para abrir una caja con fondo de $0.

Cuando se capturen denominaciones, calcular su total automáticamente y comprobar que coincide con el fondo registrado.

La apertura debe ser una operación idempotente y transaccional.

Si se pierde la respuesta de red, consultar o recuperar la operación existente en lugar de abrir otro turno.

---

## 6. Movimientos de efectivo

Crear una interfaz real de movimientos dentro del turno.

### Ingresos

Permitir registrar dinero que entra a la caja por conceptos distintos a una venta.

Ejemplos:

- Aportación adicional al fondo.
- Cambio proporcionado por el encargado.
- Reintegro de efectivo.
- Otro ingreso identificado.

Campos:

- Tipo de movimiento.
- Concepto.
- Importe.
- Moneda.
- Observaciones opcionales.
- Usuario.
- Caja.
- Turno.
- Fecha y hora.

**Los ingresos de efectivo no requieren una segunda contraseña de supervisor cuando el usuario ya tiene el permiso necesario.**

Sí deben quedar auditados.

No registrar estos movimientos como ventas.

### Retiros

Permitir retirar efectivo por motivos como:

- Resguardo.
- Retiro parcial de efectivo acumulado.
- Entrega a administración.
- Traslado a otro depósito de efectivo.
- Otro concepto identificado.

**Los retiros requieren autorización de supervisor**, además de los permisos aplicables. Si quien registra el retiro tiene el permiso de autorizar retiros, su propio permiso basta (ver §0-B).

La autorización debe reutilizar el mecanismo puntual ya desarrollado en la etapa 6.

No iniciar una sesión nueva del supervisor ni expulsarlo de otra terminal.

Registrar:

- Usuario que ejecuta.
- Supervisor que autoriza.
- Importe.
- Moneda.
- Motivo.
- Fecha.
- Caja y turno.

No permitir retiros con importes negativos o cero.

No permitir retirar una cantidad física superior al efectivo disponible calculado por moneda sin una operación excepcional explícita y autorizada. **El mensaje de rechazo no debe revelar el efectivo disponible** a quien no tenga permiso de consultar importes esperados (§0-B).

No confundir el efectivo esperado con el último conteo físico realizado.

---

## 7. Gastos pagados desde caja

Permitir registrar gastos menores efectuados con dinero de la caja.

Ejemplos:

- Compra urgente de suministros.
- Pago de un servicio menor.
- Gasto operativo autorizado.

Campos:

- Concepto.
- Importe.
- Moneda.
- Categoría o tipo de gasto.
- Observaciones.
- Referencia o comprobante opcional.
- Usuario y supervisor cuando corresponda.

Los gastos deben disminuir el efectivo esperado de la caja.

Deben quedar diferenciados de los retiros de resguardo.

El registro de un gasto desde caja no debe crear automáticamente una compra de inventario.

La relación formal con proveedores, compras e inventarios se desarrollará en la etapa 8.

**Los gastos requieren siempre autorización de supervisor, igual que los retiros**, con la misma regla de §0-B: si quien registra tiene el permiso de autorizar, su propio permiso basta. Aplican también la validación de efectivo disponible por moneda y la regla de no revelar cifras.

Ofrece categorías de gasto configurables (por ejemplo: Insumos, Servicios, Mantenimiento, Otros), sembradas por la migración y editables con permiso.

---

## 8. Movimientos en pesos y dólares

La etapa 6.1 incorporó recepción de efectivo en MXN y USD, tipo de cambio por turno y cambio entregado en cualquiera de las dos monedas.

**La etapa 7 debe conservar exactamente esa trazabilidad.**

### Reglas

Cuando la función USD esté desactivada:

- Trabajar únicamente con pesos.
- No mostrar innecesariamente controles de dólares.
- Conservar los datos históricos existentes si anteriormente se utilizó USD.

Cuando esté activada:

- Registrar fondos iniciales por moneda.
- Registrar ingresos por moneda.
- Registrar retiros por moneda.
- Registrar gastos por moneda.
- Registrar conteos físicos por moneda.
- Separar el efectivo recibido y el cambio entregado.
- Consultar los movimientos físicos en su moneda original.

### Tipo de cambio

Reutilizar el tipo de cambio asociado al turno.

No crear un segundo tipo de cambio independiente para el módulo de caja.

No modificar retroactivamente conversiones de pagos ya registrados.

No utilizar la cotización actual de una fuente externa para recalcular un turno histórico.

### Efectivo físico y equivalencias

Conservar dos niveles de información:

1. Cantidad física por moneda.
2. Equivalencia financiera en MXN utilizando las reglas y el tipo de cambio histórico del turno.

Ejemplo:

Efectivo físico disponible:

- $2,500 MXN.
- US$120.

Tipo de cambio del turno: $18.50 MXN/USD.

Equivalente:

- $2,500 MXN.
- $2,220 MXN por los dólares.
- Total equivalente: $4,720 MXN.

**El equivalente no debe sustituir la identificación del efectivo físico.**

Una diferencia de US$10 debe permanecer identificada como una diferencia de US$10, aunque también se muestre su equivalente en pesos.

No mezclar importes físicos sin distinguir sus monedas.

Respetar las reglas decimales y de redondeo de la etapa 6.1.

---

## 9. Cálculo del efectivo esperado

El efectivo esperado debe provenir exclusivamente de movimientos reales del turno.

Considerar:

- Fondo inicial.
- Cobros en efectivo.
- Propinas recibidas en efectivo.
- Cambio entregado.
- Devoluciones pagadas en efectivo.
- Correcciones financieras.
- Ingresos adicionales.
- Retiros.
- Gastos pagados desde caja.
- Otros movimientos autorizados que alteren físicamente el dinero.

### No duplicar importes

Si un cliente entrega $500 para pagar $300 y recibe $200 de cambio, la caja aumenta netamente $300, no $500.

Si el cliente entrega dólares y recibe cambio en pesos, registrar los dos movimientos físicos en sus monedas correspondientes.

Los pagos con tarjeta, transferencia o plataformas no incrementan el efectivo físico de caja.

Las propinas deben mantenerse separadas comercialmente de las ventas, aunque sí afecten el efectivo físico cuando se recibieron en efectivo.

### Información financiera

Distinguir:

- Ventas cobradas.
- Efectivo físico.
- Otros métodos de pago.
- Propinas.
- Devoluciones.
- Cortesías y descuentos.
- Ingresos.
- Retiros.
- Gastos.
- Diferencias de arqueo.

No utilizar el total de ventas como equivalente automático al efectivo esperado.

---

## 10. Cortes parciales

Implementar cortes parciales informativos.

### Características

- Requieren permiso específico.
- No cierran el turno.
- No reinician los acumulados.
- No modifican saldos.
- No eliminan movimientos.
- No equivalen a retiros de dinero.
- No generan movimientos de efectivo por sí mismos.

Registrar un snapshot con los totales y el momento del corte.

### Contenido

- Caja.
- Turno.
- Usuario que lo realizó.
- Fecha y hora.
- Intervalo cubierto.
- Operaciones registradas.
- Ventas por método de pago.
- Devoluciones.
- Descuentos y cortesías.
- Propinas.
- Ingresos.
- Retiros.
- Gastos.
- Efectivo esperado por moneda, **solo para usuarios con permiso de consultar importes esperados** (§0-B). El snapshot lo guarda siempre; lo que se oculta es su consulta e impresión.
- Datos generales de control.

Permitir varios cortes parciales por turno.

El corte parcial **no pausa la caja** y no retira dinero. Ofrece un acceso directo a «Retirar efectivo» (§0-B, terminología).

Un corte parcial posterior debe distinguir entre totales acumulados y movimientos ocurridos desde el corte anterior cuando se presenten ambas vistas.

No contabilizar dos veces el mismo movimiento.

La impresión debe ser opcional y no alterar el resultado del corte.

---

## 11. Arqueo de caja

Implementar un proceso de conteo físico previo al cierre.

### Captura por denominaciones

Permitir capturar la cantidad de cada billete y moneda.

Para MXN, utilizar un catálogo configurable de denominaciones o una lista inicial adecuada para México.

Para USD, utilizar denominaciones aplicables cuando la función esté habilitada.

No codificar la lógica de cálculos dentro de componentes JSX.

El usuario introduce cantidades físicas y Go valida y calcula los importes.

Ejemplo de MXN:

- 5 billetes de $500.
- 8 billetes de $200.
- 10 monedas de $10.

Total contado: $4,200.

El arqueo debe permitir agregar un importe adicional identificado si existe efectivo que no se haya desglosado, pero no utilizar una diferencia negativa para compensar silenciosamente errores de conteo.

### Requisitos

- Conteo por moneda.
- Captura táctil.
- Teclado numérico apropiado.
- Totales visibles del conteo realizado.
- Observaciones opcionales.
- Validación en Go.
- Conservación de borrador durante navegación recuperable.
- Protección ante cierres y cambios de turno concurrentes.

Evitar que una pantalla de arqueo muestre accidentalmente datos financieros que corresponden exclusivamente a la revisión posterior del cierre ciego.

Iniciar el arqueo de cierre pone el turno en «Cierre en curso» y pausa la caja (§0-B). El borrador del conteo se guarda en el servidor para que sobreviva a cambios de pantalla o al cierre accidental de la aplicación.

---

## 12. Cierre ciego

**Este es uno de los requisitos más importantes del proyecto.**

Durante el cierre ciego, el cajero debe registrar cuánto dinero encontró realmente sin conocer previamente el efectivo esperado.

### Comportamiento

Antes de confirmar el cierre, el cajero puede ver:

- Caja.
- Turno.
- Fecha.
- Su conteo por denominaciones.
- Totales físicos que está declarando.
- Observaciones.

Pero **no debe mostrarse**:

- Efectivo esperado.
- Diferencia calculada.
- Importe que debería registrar para cuadrar.
- Un indicador visual que revele si se está acercando al esperado.

No basta con ocultar visualmente el dato en React.

La API utilizada durante el arqueo ciego tampoco debe devolver al operador importes esperados que no esté autorizado a consultar.

El cierre ciego debe ser una regla de seguridad y operación implementada en el servidor.

### Confirmación

Al confirmar:

1. Validar usuario y permisos.
2. Verificar que el turno siga abierto.
3. Comprobar que no existe otro cierre en curso incompatible.
4. Validar el conteo.
5. Obtener un snapshot consistente de los movimientos.
6. Calcular el efectivo esperado.
7. Calcular diferencias por moneda.
8. Registrar el arqueo definitivo.
9. Cerrar el turno de forma transaccional.
10. Conservar el resultado histórico.

Después del cierre, mostrar el resultado según los permisos autorizados.

No exponer cifras sensibles durante la captura únicamente porque el usuario podrá consultarlas después.

---

## 13. Diferencias de caja

Calcular las diferencias entre efectivo físico declarado y efectivo esperado.

### Resultados

- Sin diferencia.
- Sobrante.
- Faltante.

Distinguir las diferencias de MXN y USD.

Mostrar equivalencias históricas cuando corresponda, sin sustituir los importes físicos.

### Regla comercial obligatoria

**Las diferencias NO bloquean el cierre.**

Ejemplo:

Efectivo esperado: $5,000.

Efectivo contado: $4,850.

Diferencia: −$150.

El sistema debe permitir cerrar y registrar el faltante.

Lo mismo aplica a sobrantes.

Permitir observaciones del cajero y comentarios posteriores del supervisor, con auditoría.

Una diferencia no debe desaparecer ni convertirse automáticamente en un ajuste financiero para hacer que el corte cuadre artificialmente.

No modificar pagos, ventas o retiros para ocultar diferencias.

El resultado debe conservarse históricamente.

---

## 14. Concurrencia durante el cierre

Contemplar que otras terminales podrían estar cobrando mientras se realiza un arqueo.

### Requisitos

- Utilizar los mecanismos transaccionales y de bloqueo existentes.
- No introducir bloqueos con un orden incompatible con otros módulos.
- Evitar pagos registrados después de cerrar el turno.
- Evitar cierres duplicados.
- Evitar que un arqueo basado en datos antiguos cierre silenciosamente un turno modificado.
- **Aplicar la pausa de caja de §0-B**: desde que inicia el arqueo de cierre, el turno rechaza cobros, devoluciones y movimientos; la confirmación verifica dentro de su transacción que no hubo movimientos posteriores al inicio del arqueo.
- Rechazar de forma clara los reintentos de operaciones inciertas sobre un turno pausado o cerrado (§0-B).

El cierre final debe utilizar un conjunto consistente de movimientos.

No mantener bloqueos largos de base de datos durante los minutos que el cajero tarda contando billetes.

### Cuentas pendientes

**No exigir cerrar todas las mesas o cuentas antes de cerrar un turno.**

La cuenta pertenece a la operación comercial del restaurante.

El turno pertenece a la caja.

Una cuenta puede continuar abierta y recibir pagos posteriormente en otro turno válido.

No mover automáticamente pagos históricos de un turno a otro.

---

## 15. Cierre definitivo e historial

Después de cerrar un turno:

- No permitir agregarle movimientos ordinarios.
- No permitir cobrar nuevos pagos asociados a ese turno.
- No permitir retiros o ingresos retroactivos.
- No alterar movimientos históricos.
- Conservar el arqueo y los totales.
- Mantener el responsable del cierre.
- Mantener fecha, terminal y caja.
- Registrar la auditoría.
- Permitir consultar e imprimir el corte definitivo.
- Permitir iniciar un nuevo turno cuando corresponda.

No implementar reapertura libre de turnos cerrados.

Si posteriormente se detecta un error, utilizar un procedimiento autorizado de regularización con registros compensatorios y trazabilidad, sin editar el turno cerrado.

---

## 16. Correcciones de ingresos y retiros

El sistema debe permitir corregir errores administrativos, pero sin alterar el historial.

### Reglas

- Registrar siempre el movimiento original.
- Crear un movimiento compensatorio o una anulación enlazada.
- Registrar motivo.
- Identificar usuario y supervisor.
- Registrar fecha.
- Conservar los importes originales.
- Actualizar correctamente el efectivo esperado.

**Las correcciones de ingresos y retiros requieren autorización de supervisor.**

Incluso si el usuario tiene permiso para registrar ingresos ordinarios, eso no implica autorización para corregirlos sin supervisión.

Si el turno original ya está cerrado, no modificarlo.

Definir un procedimiento de regularización posterior en el turno vigente, enlazado al registro original y sujeto a autorización.

No permitir que este procedimiento sirva para ocultar diferencias de caja.

---

## 17. Autorizaciones de supervisor

Reutilizar el sistema puntual desarrollado para la etapa 6.

No crear otra autenticación paralela.

No iniciar una sesión del supervisor para aprobar una operación.

No expulsar su sesión existente.

### Operaciones sujetas a autorización

- Retiro de efectivo.
- Corrección de ingreso.
- Corrección de retiro.
- Regularización posterior al cierre.
- Operaciones excepcionales que lo requieran según la política configurada.

Registrar quién solicitó y quién autorizó.

La autorización debe ser:

- Limitada a una operación.
- De un solo uso.
- No reutilizable con otro importe.
- Protegida contra intentos repetidos.
- Auditada.
- Sin almacenamiento de contraseñas en registros.

La autorización debe validarse dentro de la operación transaccional correspondiente.

---

## 18. Cajón de dinero

Preparar o implementar la apertura de cajón de efectivo cuando exista hardware compatible.

### Requisitos

- Configuración por terminal o caja física.
- Soporte mediante impresora o controlador compatible.
- Apertura asociada a operaciones autorizadas.
- Permiso específico para apertura manual.
- Auditoría de aperturas manuales.
- No abrir automáticamente el cajón por pagos exclusivamente electrónicos.
- No asumir que todos los dispositivos soportan el mismo protocolo.

Reutilizar las abstracciones de dispositivos y colas de impresión cuando corresponda.

Evitar reintentos automáticos de comandos físicos si su resultado es incierto.

Si no hay hardware disponible, probar mediante un adaptador controlado y documentar la limitación.

No afirmar que el cajón se abrió físicamente cuando solo se emitió un comando.

---

## 19. Panel de administración de caja

Implementar una pantalla principal de Caja con una navegación clara.

### Vista operativa

Mostrar:

- Caja seleccionada.
- Estado del turno.
- Responsable.
- Hora de apertura.
- Acciones disponibles según permisos.
- Ingresos.
- Retiros.
- Gastos.
- Cortes parciales.
- Arqueo y cierre.

No mostrar importes esperados a usuarios sin el permiso «Consultar importes esperados y diferencias» (§0-B). Muestra claramente cuando una caja está en «Cierre en curso» y quién lo inició.

### Vista de movimientos

Tabla con:

- Fecha.
- Hora.
- Tipo.
- Concepto.
- Método o moneda.
- Importe.
- Usuario.
- Referencia.
- Estado.
- Acceso a detalles.

Filtros por fecha, turno, tipo de movimiento y usuario según permisos.

No mostrar identificadores internos crudos al usuario.

### Vista histórica

Permitir consultar turnos cerrados con:

- Fecha.
- Caja.
- Apertura.
- Cierre.
- Responsables.
- Totales.
- Diferencias.
- Movimientos.
- Cortes parciales.
- Corte definitivo.

La información sensible debe respetar los permisos de cada perfil.

La pantalla debe ser táctil, profesional y adaptable.

---

## 20. Impresión de documentos de caja

Reutilizar la infraestructura de impresión persistente de la etapa 5 y los documentos comerciales de la etapa 6.

Implementar formatos básicos para:

- Comprobante de ingreso.
- Comprobante de retiro.
- Comprobante de gasto.
- Corte parcial.
- Corte definitivo.

### Contenido de corte

- Nombre del establecimiento.
- Caja.
- Turno.
- Fecha y hora de apertura.
- Fecha y hora del corte.
- Responsable.
- Ventas y pagos por método.
- Devoluciones.
- Propinas.
- Descuentos y cortesías.
- Ingresos.
- Retiros.
- Gastos.
- Efectivo esperado, según el tipo de documento y los permisos.
- Efectivo declarado.
- Diferencias por moneda.
- Observaciones.

### Impresión

- Permitir vista previa.
- Permitir imprimir opcionalmente.
- Registrar trabajos.
- Reimprimir con permiso.
- No crear movimientos financieros nuevos al imprimir.
- No volver a cerrar el turno por una reimpresión.
- No revertir un cierre exitoso por error de impresora.

Si CUPS acepta el trabajo, no afirmar que el papel salió físicamente.

El diseñador visual avanzado de documentos sigue reservado a la etapa 9.

---

## 21. Reportes operativos de caja

Esta etapa debe incluir consultas suficientes para administrar turnos y cortes, sin intentar desarrollar el dashboard completo de la etapa 9.

Implementar:

- Resumen del turno actual.
- Historial de turnos.
- Ventas cobradas por método.
- Efectivo recibido y entregado.
- Propinas.
- Devoluciones.
- Ingresos.
- Retiros.
- Gastos.
- Cortes parciales.
- Cierres.
- Diferencias.
- Totales físicos por moneda.

### Consideraciones

- Consultar información desde movimientos reales.
- No sumar dos veces operaciones compensatorias.
- Respetar permisos.
- Usar la zona horaria del restaurante.
- Permitir filtrar por caja y fechas.
- Diferenciar fecha comercial de la fecha natural cuando un turno cruce medianoche.

Los reportes deben ser consistentes con los importes utilizados al cerrar la caja.

---

## 22. Permisos y auditoría

Integrar permisos granulares en el sistema actual.

Como mínimo, contemplar:

- Consultar cajas.
- Consultar turno propio.
- Consultar turnos de otras cajas.
- Abrir turno.
- Registrar ingreso.
- Registrar retiro.
- Autorizar retiro.
- Registrar gasto.
- Corregir movimientos.
- Autorizar correcciones.
- Consultar movimientos.
- Realizar corte parcial.
- Realizar arqueo.
- Cerrar turno.
- Consultar cortes históricos.
- Consultar importes esperados.
- Consultar diferencias.
- Imprimir y reimprimir cortes.
- Abrir cajón manualmente.
- Administrar configuración de caja.

Aprovechar la estructura de roles y permisos individuales existente.

Sigue `CLAUDE.md` §20: cada permiso nuevo se registra en Go **y** en la migración, concedido **solo al rol administrador** (si no, el administrador deja de contar como completo para la protección del último administrador). No asignes permisos nuevos a otros roles por inferencia.

Los nombres visibles deben ser comprensibles.

**La seguridad debe aplicarse en Go, no únicamente ocultando botones.**

Auditar aperturas, movimientos, autorizaciones, arqueos, cortes, cierres, correcciones y reimpresiones.

Evitar registrar secretos.

---

## 23. Persistencia, migraciones e idempotencia

La instalación actual se reporta en esquema 7.

Crear las nuevas migraciones que resulten necesarias.

No modificar migraciones anteriores.

### Antes de migrar

- Revisar la instalación real.
- Generar respaldo.
- Verificar restauración en base temporal.
- Validar estructura y conteos.
- Ejecutar pruebas aisladas.
- Solicitar autorización antes de modificar la base real si todavía no se ha concedido.
- Aplicar migraciones solamente después de cumplir las salvaguardas existentes.

### Estructuras

Extender las entidades financieras existentes cuando sea posible.

Agregar únicamente lo necesario para:

- Arqueos.
- Denominaciones.
- Cortes parciales.
- Cortes definitivos.
- Ingresos y retiros.
- Gastos.
- Autorizaciones.
- Regularizaciones.
- Historial de cierre.
- Documentos.
- Configuraciones de caja.

No duplicar tablas existentes que ya representen estos conceptos de manera segura.

### Integridad

- Movimientos inmutables.
- Correcciones compensatorias.
- Identificadores idempotentes.
- Restricciones de integridad.
- Índices apropiados.
- Control de concurrencia.
- Snapshot consistente del cierre.
- Cálculos monetarios exactos.
- Ninguna edición silenciosa de registros históricos.

Reutilizar los mecanismos de idempotencia y bloqueo del repositorio, respetando su orden establecido.

---

## 24. Actualizaciones en tiempo real

Integrar los nuevos eventos con la infraestructura SSE existente.

Actualizar:

- Estado del turno.
- Apertura y cierre.
- Movimientos.
- Cortes parciales.
- Estado de la caja.
- Disponibilidad para cobrar.

Si una caja se cierra desde una terminal, las demás terminales autorizadas deben reflejarlo.

No permitir que otra terminal siga registrando movimientos sobre el turno cerrado debido a información local obsoleta.

Los reintentos deben recuperar el resultado sin duplicar operaciones.

No sustituir SSE por otra tecnología sin una razón técnica real.

---

## 25. Pruebas obligatorias

Implementar pruebas automatizadas y recorridos de aceptación para los siguientes grupos.

### Apertura y turnos

- Abrir turno con fondo de $0.
- Abrir turno con fondo positivo.
- Abrir con denominaciones.
- Impedir dos turnos abiertos en una caja.
- Respetar turnos compartidos.
- Mantener responsables individuales.
- Reiniciar Go sin perder el turno.

### Ingresos y retiros

- Registrar ingreso con permiso.
- Registrar ingreso sin segunda autorización.
- Rechazar ingreso sin permiso.
- Registrar retiro autorizado.
- Rechazar retiro sin autorización.
- Rechazar autoautorización indebida.
- Corregir ingreso con autorización.
- Corregir retiro con autorización.
- Mantener movimientos originales.
- Registrar gastos diferenciados.

### Cortes parciales

- Crear varios cortes.
- Mantener el turno abierto.
- No modificar el saldo.
- Registrar snapshots correctos.
- Evitar duplicación por reintento.
- Reimprimir sin generar otro corte.

### Arqueos y cierre ciego

- Capturar denominaciones.
- Calcular totales correctos.
- No exponer efectivo esperado antes del cierre.
- Rechazar acceso API sin permiso.
- Cerrar sin diferencia.
- Cerrar con faltante.
- Cerrar con sobrante.
- Conservar diferencias históricas.
- Permitir cierre con cuentas abiertas.
- Impedir dos cierres simultáneos.
- Detectar movimientos concurrentes.
- No aceptar pagos nuevos en un turno cerrado.

### MXN y USD

- Función USD desactivada.
- Turno solamente en MXN.
- Turno con fondo en MXN y USD.
- Cobro con dólares y cambio en pesos.
- Cobro con pesos y cambio en dólares.
- Retiro en USD.
- Conteo por moneda.
- Diferencias por moneda.
- Equivalencias con tipo de cambio histórico.
- Conservación exacta de redondeos.
- Correcciones de operaciones en dólares.

### Decisiones de §0-B

- Retiro y gasto registrados por un usuario con permiso de autorizar: sin segunda contraseña, registrados como autoautorizados.
- Retiro y gasto de un usuario sin ese permiso: rechazados sin autorización; aceptados con la de otra persona; rechazados si intenta autorizarse a sí mismo.
- Iniciar arqueo pausa la caja: un cobro, una devolución y un movimiento desde otra terminal son rechazados con mensaje claro.
- Cancelar el arqueo reanuda la caja.
- Confirmación rechazada si se detecta un movimiento posterior al inicio del arqueo.
- Reintento de una operación incierta sobre un turno pausado o cerrado: rechazado sin duplicar.
- Ningún endpoint devuelve el efectivo esperado, directa o indirectamente, a un usuario sin permiso: arqueo, corte parcial, rechazo de retiro por exceder el disponible, tope de caja.
- Corte parcial con y sin permiso de consultar importes esperados.
- Tope de caja: desactivado sin efecto; activado con aviso solo para usuarios con permiso; nunca bloquea cobros.
- Regularización posterior al cierre: enlazada al original, en el turno vigente, sin cambiar la diferencia del turno cerrado.
- Fecha comercial de un turno que cruza medianoche.

### Impresión y hardware

- Corte parcial.
- Corte definitivo.
- Comprobante de ingreso.
- Comprobante de retiro.
- Impresión fallida sin cancelar el cierre.
- Reimpresión idempotente.
- Apertura de cajón autorizada.
- Resultado incierto de comando físico.

### Regresión e integración

- Cobros y devoluciones de etapa 6.
- Función USD de etapa 6.1.
- IVA por producto.
- Cuentas y mesas.
- Pedidos para llevar.
- Cocina y comandas.
- SSE.
- Sesiones.
- Permisos.
- Auditoría.
- Migraciones.
- Go race detector y vet.
- PostgreSQL.
- Vitest.
- TypeScript estricto.
- Rust/Tauri.
- Chromium y WebKit.
- Compilación macOS.
- Independencia entre backend y Tauri.

No inventar resultados de pruebas.

Cuando no pueda comprobarse hardware físico, utilizar un adaptador de pruebas y documentar qué quedó pendiente.

---

## 26. Exclusiones

No implementar todavía:

- Inventarios y compras de etapa 8.
- Contabilidad general empresarial.
- Conciliación bancaria externa.
- Procesamiento bancario automático.
- Distribución de propinas.
- Dashboard avanzado de etapa 9.
- Editor visual avanzado de documentos.
- Facturación electrónica.
- Aplicación Android.
- Aplicación KDS independiente.
- Servidor comercial de licencias.
- Instaladores Windows.
- Migrador del POS anterior.

No crear módulos ficticios para anticipar estas funciones.

---

## 27. Documentación y entrega

Actualizar la documentación existente.

Crear un documento específico para caja y cortes, por ejemplo:

`docs/CASH_SHIFTS_AND_RECONCILIATION.md`

Documentar:

- Apertura.
- Fondos iniciales.
- Ingresos.
- Retiros.
- Gastos.
- Turnos compartidos.
- Arqueos.
- Cierre ciego.
- Diferencias.
- Movimientos por moneda.
- Cálculos del efectivo esperado.
- Cortes parciales.
- Cierre definitivo.
- Correcciones y regularizaciones.
- Permisos.
- Auditoría.
- Impresión.
- Reglas de concurrencia.
- Pruebas ejecutadas.
- Limitaciones reales.

Actualizar `CLAUDE.md`, `AGENTS.md` y demás documentos afectados cuando corresponda.

### Resultado esperado

Al terminar, informar:

1. Funcionalidades desarrolladas.
2. Migraciones nuevas.
3. Contratos API.
4. Pantallas.
5. Cálculos financieros.
6. Manejo de MXN y USD.
7. Integración con cobros y devoluciones.
8. Autorizaciones.
9. Cortes parciales.
10. Cierre ciego.
11. Diferencias.
12. Impresión.
13. Pruebas ejecutadas.
14. Limitaciones.
15. Comandos de inicio.
16. Confirmación de continuidad de las etapas anteriores.
17. Resultado de la Fase 0: lo que ya existía y se reutilizó, contradicciones encontradas y funciones del sistema anterior no incluidas.
18. Procedimiento para aplicar la migración a la instalación real, en espera de autorización.

## CRITERIO FINAL DE ACEPTACIÓN

Debo poder abrir AD-EL POS y administrar una caja desde el inicio del turno hasta su cierre.

Debo poder registrar un fondo inicial, cobrar ventas, recibir ingresos, efectuar retiros autorizados, registrar gastos, consultar movimientos, generar cortes parciales, contar físicamente el dinero y cerrar la caja.

Durante el cierre ciego, el cajero no debe conocer el importe esperado.

Si existe sobrante o faltante, **debe permitirse completar el cierre y registrar la diferencia**.

Si existen mesas o cuentas pendientes, **el turno debe poder cerrarse sin obligar a liquidarlas**.

Cuando se manejen dólares, el sistema debe conservar cantidades físicas por moneda y calcular equivalencias utilizando el tipo de cambio histórico del turno.

Todo debe utilizar la infraestructura financiera real de las etapas 6 y 6.1, sin duplicar pagos ni modificar movimientos históricos.

No quiero maquetas ni registros financieros simulados.

**Implementa, integra, prueba y documenta completamente la etapa 7 por fases (0 → D) en la rama `etapa-7`, avanzando de una a otra cuando cada fase quede funcional y probada, respetando el repositorio, las decisiones de §0-B y todas las decisiones funcionales anteriores.**