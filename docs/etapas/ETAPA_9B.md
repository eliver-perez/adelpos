# AD-EL POS — ETAPA 9B: IDENTIDAD DEL ESTABLECIMIENTO Y DISEÑADOR VISUAL DE DOCUMENTOS

## CONTEXTO

Continúa el desarrollo de AD-EL POS, sistema comercial de punto de venta para restaurantes.

Stack aprobado:

- Backend: Go.
- Base de datos: PostgreSQL.
- Frontend: React, TypeScript y Tailwind CSS.
- Escritorio: Tauri 2.
- Comunicación: API HTTP versionada y SSE autenticado.

Las etapas 1 a 8 están desarrolladas, incluidas las ampliaciones intermedias.

La versión de partida es la que dejó la **9A** (0.9.0). Confirma la versión y el esquema reales en `config/product.json` y en `CLAUDE.md`.

La etapa 9 se dividió en dos entregas. **La 9A ya implementó** los ajustes pendientes, los reportes, el dashboard y las exportaciones PDF/Excel. **Esta, la 9B**, implementa la identidad visual del establecimiento (logotipo y datos para documentos) y el diseñador visual de tickets, precuentas, comandas, avisos de cocina y documentos de caja.


**Esta especificación debe contrastarse con el código real antes de implementarse.**

No reconstruyas módulos terminados, no dupliques motores financieros ni de inventario y no modifiques migraciones aplicadas.

El resultado debe ser funcional, mantenible, comprobable y apto para operación comercial.

Al terminar, la versión será **0.9.1** (o la siguiente a la que dejó la 9A), con el esquema que resulte de las migraciones nuevas.

---

# 0. FORMA DE TRABAJO

### Rama

Trabaja en una rama nueva **`etapa-9b`** creada desde `main` (que contiene hasta la 9A). No hagas push ni merge.

Las especificaciones en `docs/etapas/` son de solo lectura. Lee únicamente este documento.

### Fase 0: verificación contra el código (obligatoria, antes de diseñar)

Comprueba en el repositorio y registra en `docs/ETAPA_9B_AVANCE.md`:

1. **Cómo se generan hoy los documentos impresos** de las etapas 5, 6, 7 y 7.1: plantillas en código, datos que reciben, ancho de papel, formato de importes, transportes (CUPS y ESC/POS por red) y cola persistente.
2. **Qué se guarda de cada documento emitido** (ticket, precuenta, comanda, comprobante, corte) y cómo funciona hoy la reimpresión.
3. Los datos del establecimiento que ya existen en Configuración, para no duplicarlos.
4. Cómo imprimía el sistema anterior sus tickets (`CLAUDE.md` §21).

Si encuentras algo que obligue a cambiar una decisión del propietario, **detente y pregunta**. Si no, continúa.

### Fases

| Fase | Contenido |
| ---- | --------- |
| **0** | Verificación contra el código. |
| **A** | Modelo de plantillas por bloques, versiones, validación en Go, renderizador único (vista previa e impresión) y migración de los formatos actuales a plantillas predeterminadas idénticas. |
| **B** | Identidad del establecimiento y logotipo, incluida su conversión para impresoras térmicas. |
| **C** | Editor visual: panel de bloques, vista previa, propiedades, versiones, activar, duplicar, restaurar e impresión de muestra. |
| **D** | Regresiones completas, documentación y entrega final, con el procedimiento para aplicar las migraciones a la instalación real **en espera de autorización**. |

**Al terminar cada fase:** ejecuta sus pruebas y las suites afectadas, aplica `./scripts/dev.sh format` y `format-check`, registra un resumen en el avance, haz un commit local y **continúa sin esperar confirmación** si todo pasa.

**Detente y pregunta únicamente si:** una prueba falla y arreglarla exige cambiar una decisión de este documento; encuentras una contradicción que obligue a una decisión funcional importante; necesitas aplicar migraciones a la instalación real; o una acción podría afectar datos o servicios fuera del alcance.

---

# 0-B. DECISIONES DEL PROPIETARIO Y DISEÑO TÉCNICO

**Si una sección parece decir otra cosa, prevalece esta.**

### Diseñador de bloques ordenados, no lienzo libre

Las impresoras térmicas imprimen línea por línea. Un lienzo para acomodar elementos en cualquier posición prometería resultados que el papel no reproduce.

- Una plantilla es una **lista ordenada de bloques**: logotipo, encabezado del establecimiento, texto fijo, campos de datos, lista de productos, totales, pagos, separador, espacio, pie, etc.
- El usuario **agrega, quita, reordena (arrastrando o con flechas) y configura** bloques. Las propiedades de §21 se aplican por bloque, limitadas a lo que la impresora puede hacer: alineación, tamaño (normal, doble alto, doble ancho, doble), negrita, mayúsculas, visibilidad y espaciado.
- **Cada tipo de documento ofrece solo sus bloques compatibles** (una comanda no ofrece métodos de pago; un corte no ofrece la lista de productos).
- El panel de propiedades nunca pide HTML, CSS, JSON ni códigos ESC/POS.

### Un solo renderizador

- Go convierte una plantilla y los datos del documento en una **representación intermedia** de líneas con atributos (texto, alineación, tamaño, negrita) e imágenes.
- **La vista previa y la impresión usan esa misma representación.** La vista previa muestra exactamente las líneas que se enviarán, con el ancho en caracteres de cada papel, no una aproximación con HTML libre.
- Los transportes existentes (CUPS y ESC/POS por red) traducen esa representación. La cola, el control de duplicados y los resultados inciertos no cambian.
- Las observaciones y textos capturados por usuarios se limpian de caracteres de control antes de imprimirse.

### Plantillas predeterminadas idénticas a los formatos actuales

- La primera versión de cada plantilla predeterminada debe producir **el mismo resultado** que los formatos actuales de las etapas 5 a 7.1. Compruébalo con pruebas que comparen la salida anterior y la nueva.
- Así, instalar la 9B no cambia ningún ticket hasta que el restaurante decida editarlo.

### Documentos históricos y reimpresión

- Cada documento emitido guarda la **versión de plantilla** que usó, además de sus datos comerciales históricos.
- **La reimpresión es idéntica al original**: mismos datos y misma versión de plantilla, aunque la plantilla haya cambiado después. **No se agrega ninguna marca de «REIMPRESIÓN»** (decisión del propietario). La reimpresión queda registrada en la auditoría y en la cola como hoy.
- Imprimir un documento histórico con la plantilla actual es una acción distinta y explícita, con su propio nombre, permiso y auditoría.
- Las versiones de plantilla son inmutables: editar crea una versión nueva.

### Logotipo en impresoras térmicas

- El logotipo se valida (formato, tamaño, dimensiones) y se convierte en Go a **imagen monocromática** al ancho imprimible de 58 y 80 mm, con un método de tramado configurable (umbral o difuminado) y vista previa de cómo saldrá en papel.
- La versión convertida se guarda para no procesarla en cada impresión.
- Para ESC/POS se envía con el comando de imagen raster estándar. Si una cola o impresora no lo admite, el bloque de logotipo se omite con un aviso en la configuración, sin fallar la impresión.
- **El resultado en papel queda como prueba física pendiente**: el propietario la hará con sus impresoras. No lo des por verificado con el adaptador simulado.
- El logotipo del restaurante se usa en documentos; la aplicación conserva la marca de AD-EL POS.

### Logotipo en reportes PDF y Excel (decisión del propietario)

- Los reportes PDF de la 9A y la hoja de metadatos de los Excel deben mostrar en su encabezado el **logotipo del restaurante** junto con su nombre y datos.
- **Si el restaurante no ha subido logotipo, se usa el logotipo de AD-EL POS** (`assets/brand/`, sin modificarlo), con el nombre del establecimiento.
- Revisa cómo generó la 9A esos encabezados y ajústalos sin cambiar su contenido ni sus consultas.
- En los **tickets y documentos térmicos**, las plantillas predeterminadas siguen sin logotipo (como hoy). El restaurante lo agrega con el bloque de logotipo; si no subió el suyo, ese bloque no imprime nada.

### Campos protegidos

- Los bloques de efectivo esperado, diferencias y costos solo existen en los documentos de caja donde ya se mostraban, y se renderizan **solo si quien imprime tiene el permiso**, igual que en la etapa 7. Durante un arqueo ciego no se renderizan para nadie.
- Una plantilla no puede agregar esos bloques a otros tipos de documento. Go lo valida al guardar.

---

# 1. REVISIÓN OBLIGATORIA DEL REPOSITORIO

Antes de escribir código, examina:

- `CLAUDE.md`.
- `AGENTS.md`.
- `ARCHITECTURE.md`.
- `API_CONTRACTS.md`.
- `README.md`.
- `NEXT_STEPS_FOR_SOL.md`.
- `docs/VERIFICATION.md`.
- `docs/CATALOG.md`.
- `docs/RESTAURANT_OPERATIONS.md`.
- `docs/PAYMENTS_AND_REFUNDS.md`.
- `docs/KITCHEN_AND_PRINTING.md`.
- `docs/CASH_SHIFTS_AND_RECONCILIATION.md`.
- `docs/INVENTORY_AND_PURCHASING.md`.
- La documentación técnica de las etapas 7.1 y 8.
- Las migraciones existentes.
- Los módulos de catálogo, restaurante, cocina, pagos, caja e inventarios.
- El sistema actual de impresión.
- La seguridad, permisos, auditoría y eventos SSE.
- Los componentes compartidos de tablas, formularios, filtros, notificaciones y gráficos, si existen.

Verifica las estructuras y los contratos reales antes de determinar el diseño de los reportes.

### Referencias del sistema anterior

Consulta la sección 21 de `CLAUDE.md` y, cuando estén disponibles, los archivos del POS original desarrollado en C#.

Presta especial atención a:

- Reportes de ventas.
- Reportes por producto.
- Reportes de caja.
- Reportes por empleado.
- Reportes de proveedores.
- Reporte de compras por periodo.
- Consultas históricas de inventario.
- Tickets y formatos impresos.

El sistema anterior sirve como referencia funcional.

No copiar su arquitectura Windows Forms ni trasladar consultas SQL Server sin adaptarlas correctamente.

---

# 2. PRINCIPIOS DE LA ETAPA

Las siguientes reglas son obligatorias:

1. Todos los importes y cantidades deben provenir de información real del backend Go y PostgreSQL.
2. No recalcular precios históricos utilizando el catálogo actual.
3. No sumar ventas y cobros como si fueran el mismo ingreso.
4. No contabilizar una compra y su gasto de caja vinculado como dos compras o dos salidas físicas adicionales.
5. No tratar una devolución financiera como una devolución de ingredientes.
6. No confundir inventario teórico con inventario físico.
7. No exponer costos a usuarios sin permisos de costos.
8. No exponer efectivo esperado o diferencias de caja sin los permisos correspondientes.
9. Los archivos PDF y Excel deben aplicar las mismas autorizaciones que las consultas de pantalla.
10. Los reportes deben respetar fechas comerciales, zonas horarias, monedas y registros históricos.
11. La impresión no debe crear nuevas ventas ni nuevos movimientos.
12. La reimpresión de documentos históricos debe conservar la información comercial originalmente registrada.
13. Los diseños personalizados no deben permitir ejecutar código arbitrario.
14. Los módulos anteriores deben continuar funcionando normalmente.
15. La aplicación debe mantener la interfaz moderna, adaptable y compatible con pantalla completa.

---

# 19. IDENTIDAD VISUAL DEL ESTABLECIMIENTO

Permitir que cada restaurante configure su identidad para documentos.

### Datos

- Nombre comercial.
- Logotipo.
- Dirección.
- Teléfono.
- Información adicional autorizada.
- Texto de encabezado.
- Texto de pie de página.

Reutilizar los datos del establecimiento existentes.

No duplicar campos que ya estén implementados.

### Logotipo

Permitir cargar un logotipo del restaurante.

Validar:

- Formato permitido.
- Tamaño.
- Dimensiones.
- Tipo de archivo.
- Integridad.
- Seguridad del procesamiento.

Permitir:

- Subir.
- Reemplazar.
- Previsualizar.
- Retirar.
- Ajustar su presentación en documentos.

**El logotipo del restaurante es distinto del logotipo comercial de AD-EL POS.**

No reemplazar la identidad visual del software por el logotipo del cliente en todas las pantallas.

La personalización del restaurante debe utilizarse principalmente en tickets, reportes y documentos.

---

# 20. DISEÑADOR VISUAL DE DOCUMENTOS

**Esta es una de las funcionalidades principales de la etapa 9.**

Desarrollar un editor visual que permita configurar los formatos utilizados por el restaurante.

Debe ser una interfaz gráfica real, no un formulario donde el usuario tenga que escribir HTML, CSS, JSON o código de impresión.

### Tipos de documentos

Como mínimo:

- Ticket de venta.
- Precuenta.
- Comanda.
- Aviso de modificación a cocina.
- Aviso de cancelación a cocina.
- Comprobante de ingreso.
- Comprobante de retiro.
- Comprobante de gasto.
- Corte parcial.
- Corte definitivo.

Reutilizar el motor de impresión desarrollado en las etapas 5, 6 y 7.

No crear otro sistema paralelo de colas de impresión.

---

# 21. EXPERIENCIA DEL EDITOR VISUAL

Proponer una interfaz con tres áreas funcionales:

**Panel de elementos**

Contendrá los componentes que el usuario puede incorporar al documento.

**Lienzo o vista previa**

Mostrará el documento con sus dimensiones y contenido.

**Panel de propiedades**

Permitirá modificar las propiedades del elemento seleccionado.

### Elementos configurables

- Logotipo.
- Nombre del establecimiento.
- Texto fijo.
- Campo de datos.
- Fecha.
- Hora.
- Folio.
- Mesa.
- Cajero.
- Nombre del cliente cuando corresponda.
- Lista de productos.
- Cantidades.
- Complementos.
- Observaciones.
- Precios.
- Descuentos.
- Impuestos.
- Subtotales.
- Total.
- Pagos.
- Cambio.
- Separadores.
- Espaciadores.
- Totales de caja.
- Diferencias autorizadas.
- Texto de pie.

No ofrecer campos incompatibles con el tipo de documento.

Por ejemplo, una comanda de cocina no necesita mostrar automáticamente los métodos de pago.

### Propiedades

Permitir ajustar, según compatibilidad:

- Orden.
- Alineación.
- Tamaño relativo.
- Énfasis.
- Visibilidad.
- Espaciado.
- Encabezado.
- Pie.
- Ancho.
- Separadores.
- Impresión de observaciones.
- Cantidades y personalizaciones.

Para documentos térmicos, las opciones deben respetar las capacidades reales del transporte y del equipo.

No prometer tipografías, gráficos o posiciones que una impresora ESC/POS determinada no pueda reproducir.

---

# 22. VISTA PREVIA EN TIEMPO REAL

El editor debe mostrar cambios de manera inmediata.

### Modos

- Vista previa de datos de ejemplo, claramente identificados como muestra.
- Vista previa de un documento real autorizado cuando corresponda.

Los datos de muestra no deben insertarse como ventas, pagos ni movimientos reales.

### Tamaños

Contemplar anchos térmicos habituales, especialmente:

- 58 mm.
- 80 mm.

No asumir que la cantidad exacta de caracteres es idéntica entre todas las impresoras.

El formato debe considerar:

- Densidad.
- Ancho imprimible.
- Codificación.
- Capacidades de corte.
- Representación del logotipo.

### Prueba de impresión

Permitir imprimir una muestra desde la configuración, con autorización.

Debe quedar claramente identificada como prueba.

No generar una venta ni un pago por imprimir una muestra.

La cola debe registrar correctamente el resultado conocido o incierto.

---

# 23. PLANTILLAS Y VERSIONES

Implementar plantillas administrables.

Funciones:

- Crear plantilla.
- Editarla.
- Duplicarla.
- Renombrarla.
- Activarla.
- Seleccionarla por tipo de documento.
- Restaurar configuración predeterminada.
- Previsualizar.
- Imprimir muestra.
- Consultar historial de cambios.

### Versionado

Los cambios de un formato deben quedar versionados.

No modificar automáticamente el contenido de documentos históricos.

Una venta registrada ayer debe conservar sus productos, precios, descuentos y pagos originales.

### Reimpresión

El sistema actual ya permite reimpresiones.

**Una reimpresión debe conservar el contenido histórico del documento registrado**, sin sustituir silenciosamente sus importes o condiciones comerciales por la información actual.

Cuando se permita generar una nueva representación de un documento histórico con una plantilla actualizada, esa operación deberá ser explícita y distinguible de la reimpresión exacta.

No crear nuevas ventas, pagos o cierres al reimprimir.

### Concurrencia

Evitar que dos administradores sobrescriban cambios de una plantilla sin advertencia.

Utilizar revisiones o versiones de configuración.

---

# 24. SEGURIDAD DEL DISEÑADOR

No permitir que una plantilla ejecute:

- JavaScript arbitrario.
- Código SQL.
- Comandos del sistema.
- Solicitudes de red no autorizadas.
- HTML activo no sanitizado.
- Instrucciones ESC/POS suministradas directamente por usuarios.

Utilizar elementos y campos permitidos mediante un modelo estructurado.

Validar plantillas desde Go.

Limitar dimensiones, tamaños de recursos y complejidad.

Evitar que una observación comercial pueda alterar el flujo de control de una impresora.

Los documentos generados deben respetar permisos y restricciones de datos.

### Campos sensibles

Si una plantilla incluye efectivo esperado, diferencias, costos u otros valores protegidos:

- Validar permisos.
- Validar el estado del turno.
- Impedir su exposición durante un arqueo ciego.
- No permitir su uso en tipos de documento incompatibles.

La configuración visual no debe convertirse en una vía para eludir permisos.

---

# 25. INTEGRACIÓN CON IMPRESORAS

Reutilizar:

- Estaciones.
- Rutas.
- Impresoras.
- Cola persistente.
- Control de duplicados.
- Reintentos seguros.
- Resultados inciertos.
- Reimpresiones.
- Compatibilidad CUPS.
- Transporte ESC/POS cuando corresponda.

El diseñador modifica la presentación, no las reglas de negocio.

### Ejemplo

Si un pedido debe enviarse a Cocina y Barra, la plantilla no debe modificar el enrutamiento.

Solo define cómo aparece cada comanda.

Un trabajo aceptado por CUPS no equivale a una confirmación física del papel.

No afirmar impresión exitosa cuando solamente se conoce la aceptación por la cola del sistema operativo.

---

# 27. PERMISOS Y AUDITORÍA

Reutilizar los permisos existentes.

Extenderlos cuando sea necesario para:

- Consultar reportes.
- Consultar ventas.
- Consultar pagos.
- Consultar descuentos.
- Consultar devoluciones.
- Consultar reportes de caja.
- Consultar costos.
- Consultar valuaciones.
- Consultar inventarios.
- Consultar compras.
- Consultar proveedores.
- Exportar PDF.
- Exportar Excel.
- Administrar plantillas.
- Previsualizar documentos.
- Imprimir muestras.
- Modificar identidad documental.
- Consultar reportes de empleados.

Respetar siempre los permisos más específicos.

Por ejemplo:

Tener permiso para consultar ventas no implica permiso para consultar costos ni diferencias de caja.

Tener permiso para exportar Excel no implica permiso para exportar todos los datos existentes.

Los endpoints deben verificar permisos en Go.

No confiar solamente en la interfaz.

Registrar auditoría de:

- Exportaciones sensibles.
- Cambios de plantillas.
- Cambios de logotipo.
- Reimpresiones.
- Impresiones de muestra.
- Modificaciones de configuración.
- Consultas sensibles cuando el sistema ya tenga esa política.

No registrar innecesariamente datos personales o secretos.

---

# 29. API Y EVENTOS

Mantener la API `/api/v1`.

Agregar rutas versionadas para:

- Catálogo de reportes disponibles.
- Filtros.
- Consultas agregadas.
- Desgloses.
- Dashboard.
- Exportaciones.
- Configuración documental.
- Plantillas.
- Vista previa.
- Impresión de prueba.
- Identidad visual del establecimiento.

Utilizar contratos claros y tipados.

No crear endpoints genéricos donde el cliente envíe SQL arbitrario.

### SSE

Reutilizar la infraestructura SSE cuando resulte útil para:

- Actualización de indicadores.
- Cambios de configuración.
- Plantillas.
- Estados de exportación prolongada, si corresponde.

No utilizar SSE como sustituto de consultas históricas.

La base de datos sigue siendo la autoridad.

No introducir servicios externos innecesarios.

---

# 30. MIGRACIONES

Confirma el esquema actual y usa el siguiente número de migración disponible después de los de la 9A.

Verifica la estructura real antes de crear nuevas migraciones.

No modificar las ya aplicadas.

Crear únicamente las tablas o columnas necesarias para:

- Ajustes de precio por producto.
- Campos adicionales de proveedores.
- Proveedor o modalidad de compras de mostrador.
- Categorías de insumos, si no existen.
- Configuración de identidad documental.
- Plantillas.
- Versiones de plantillas.
- Referencias a documentos generados.
- Configuración de reportes.
- Otros elementos estrictamente necesarios.

No duplicar datos transaccionales históricos en tablas de reportes sin una justificación de rendimiento y una política de actualización consistente.

### Seguridad

Antes de modificar una instalación real:

- Realizar respaldo.
- Ensayar restauración.
- Verificar conteos e integridad.
- Aplicar migraciones en el procedimiento controlado existente.
- Comprobar compatibilidad.
- Ejecutar regresiones.
- Conservar una vía de recuperación.

No asumir que una autorización para migraciones anteriores se extiende automáticamente a las de esta etapa.

---

# 31. PRUEBAS OBLIGATORIAS

Implementar pruebas reales para todas las áreas principales.

### Diseñador visual

- Crear plantilla.
- Editar plantilla.
- Duplicar.
- Versionar.
- Restaurar predeterminada.
- Cargar logotipo.
- Vista previa.
- Cambio de tamaño 58/80 mm.
- Campos dinámicos.
- Control de permisos.
- Rechazo de elementos peligrosos.
- Conservación de documentos históricos.
- Reimpresión exacta.
- Impresión de muestra sin generar venta.
- Compatibilidad con cola existente.
- Control de resultados inciertos.

### Decisiones de §0-B

- Plantillas predeterminadas con salida idéntica a los formatos actuales (comparación de la salida anterior y la nueva, por tipo de documento y ancho).
- Vista previa e impresión con la misma representación intermedia.
- Solo bloques compatibles por tipo de documento; rechazo en Go de bloques protegidos en tipos incompatibles.
- Bloques protegidos sin permiso y durante un arqueo ciego: no se renderizan.
- Reimpresión de un documento emitido antes de editar la plantilla: idéntica al original y sin marca de reimpresión; registrada en la auditoría.
- Impresión de un histórico con la plantilla actual como acción distinta, con permiso y auditoría.
- Logotipo convertido a monocromo para 58 y 80 mm; cola o impresora sin soporte raster: bloque omitido con aviso, sin fallar la impresión.
- Observación con caracteres de control: limpiada antes de imprimir.
- Dos administradores editando la misma plantilla: conflicto de revisión, sin sobrescribir.
- Reportes y exportaciones de la 9A sin cambios.

### Regresiones generales

- Catálogo.
- Personalizaciones.
- Combos.
- Mesas.
- Captura de pedidos.
- Cocina.
- Comandas.
- Cobros.
- Caja.
- Inventarios.
- Compras.
- Usuarios.
- Permisos.
- SSE.
- Go race detector y vet.
- PostgreSQL.
- Vitest.
- TypeScript estricto.
- Rust/Tauri.
- Chromium.
- WebKit.
- Compilación macOS.
- Independencia del servidor.

No afirmar que una prueba se ejecutó si únicamente se diseñó.

---

# 32. PRUEBAS FÍSICAS PENDIENTES

Conservar identificadas las pruebas todavía no realizadas:

- Impresión real de tickets.
- Impresión real de precuentas.
- Impresión de comandas.
- Impresión de cortes.
- Cajón conectado a impresora de red.
- Cajón conectado a impresora USB mediante CUPS.
- Pantalla táctil física.
- Dos terminales reales.
- Comportamiento de la ventana nativa en distintas condiciones.

Estas pruebas deberán realizarse cuando el hardware y el entorno estén disponibles.

No bloquear automáticamente el desarrollo de la etapa 9 por no disponer del hardware.

Tampoco afirmar que esas pruebas se completaron mediante adaptadores simulados.

Registrar claramente las limitaciones.

---

# 34. EXCLUSIONES

No implementar todavía:

- Cambios a los reportes, el dashboard o las exportaciones de la 9A, salvo lo indispensable para usar la identidad del establecimiento en sus encabezados.
- Plantillas para documentos fuera de los tipos de §20.
- Rediseño global horizontal del menú principal.
- Aplicación Android.
- Aplicación KDS independiente.
- Servidor comercial de licencias.
- Instaladores Windows.
- Migrador del antiguo SQL Server.
- Contabilidad general.
- Conciliación bancaria externa.
- Facturación electrónica completa.
- Nómina.
- Distribución de propinas.
- Integraciones bancarias automáticas.
- Sistema completo de cuentas por pagar.

No introducir funcionalidades ficticias para anticipar etapas posteriores.

---

# 36. DOCUMENTACIÓN

Crear una guía específica, por ejemplo:

`docs/DOCUMENT_DESIGNER.md`

Documentar:

- Arquitectura de reportes.
- Definiciones de métricas.
- Fechas comerciales.
- Filtros.
- Ventas.
- Pagos.
- Descuentos.
- Propinas.
- Caja.
- Inventarios.
- Compras.
- Costos.
- Permisos.
- Exportaciones.
- Identidad documental.
- Plantillas.
- Editor visual.
- Versionado.
- Impresión.
- Seguridad.
- Pruebas.
- Limitaciones.

Actualizar los documentos técnicos existentes según corresponda:

- `CLAUDE.md`.
- `AGENTS.md`.
- `README.md`.
- `ARCHITECTURE.md`.
- `API_CONTRACTS.md`.
- `NEXT_STEPS_FOR_SOL.md`.
- `docs/VERIFICATION.md`.

No documentar como implementado algo que todavía no funcione.

---

# CRITERIO FINAL DE ACEPTACIÓN

Debo poder abrir AD-EL POS en mi Mac y:

- Configurar el logotipo y los datos del restaurante para documentos, sin cambiar la marca de AD-EL POS en la aplicación.
- Diseñar visualmente, con bloques, tickets, precuentas, comandas, avisos de cocina, comprobantes de caja y cortes.
- Ver cada cambio en la vista previa a 58 y 80 mm.
- Imprimir muestras identificadas como prueba, sin generar ventas ni movimientos.
- Activar una plantilla por tipo de documento y restaurar la predeterminada.
- Consultar el historial de versiones de una plantilla.
- Reimprimir un documento histórico idéntico al original, aunque la plantilla haya cambiado después.
- Utilizar todas estas funciones respetando los permisos, sin que una plantilla revele datos protegidos.

Las operaciones comerciales, financieras, de inventario y los reportes de la 9A deben permanecer intactos.

**No quiero maquetas ni un editor que solamente permita modificar código de plantillas.**

Implementa, integra, prueba y documenta la etapa 9B completa por fases (0 → D) en la rama `etapa-9b`, respetando las decisiones de §0-B, reutilizando la arquitectura existente y preservando la integridad histórica de AD-EL POS.
