# Instrucciones para continuar

Leer README.md, ARCHITECTURE.md, API_CONTRACTS.md y NEXT_STEPS_FOR_SOL.md antes de ampliar funcionalidad.

## Convenciones permanentes de legibilidad

- Está prohibido ofuscar, minificar o compactar el código fuente para reducir líneas.
- Go: desarrollar funciones y bloques if/else/for/switch en múltiples líneas, con llaves e indentación de gofmt. No concentrar condiciones y operaciones completas en una línea.
- React/JSX/TSX: cada etiqueta anidada debe tener su propia línea e indentación clara, con aperturas y cierres identificables.
- TypeScript/JavaScript: separar operaciones y expandir condiciones, objetos y expresiones complejas. Revisar manualmente lo que Prettier no expanda.
- Respetar indentación consistente: tabulaciones de gofmt en Go; dos espacios con Prettier en archivos compatibles; convenciones propias de Rust, Python y shell.
- Usar nombres descriptivos, funciones con responsabilidad clara y componentes mantenibles; evitar archivos o funciones gigantescos.
- Escribir comentarios explicativos en español para reglas, validaciones y lógica compleja; evitar comentarios redundantes.
- Ejecutar gofmt y Prettier antes de entregar; preservar el comportamiento durante el reformateo.
- La claridad y mantenibilidad tienen prioridad sobre reducir el número de líneas. Estas reglas rigen todas las etapas futuras.

- Mantener Go independiente de Tauri y toda operación SQL en backend.
- No introducir módulos comerciales fuera del alcance solicitado ni datos ficticios.
- No leer/imprimir secretos de .local ni modificar clústeres ajenos. No versionar .local, .tools, cachés o builds.
- Mantener localhost por defecto, CSP y capabilities restringidas.
- Metadatos desde config/product.json; ejecutar scripts/dev.sh sync al cambiarlos.
- Migraciones aplicadas inmutables. Añadir pruebas significativas de transacciones/contratos cuando cambien.
- Verificar con scripts/dev.sh test y, para cambios de persistencia, test-integration contra la base temporal del proyecto.
- Actualizar la documentación con lo realmente ejecutado, sin presentar tareas pendientes como completas.

## Reglas confirmadas en etapa 2

- El nombre inicial del establecimiento es ADEL POS; proponer la zona horaria del sistema operativo y permitir editarla. No cambiar la marca técnica por inferencia.
- admin/admin es una política funcional aprobada: crear una sola vez, advertir sin obligar al cambio y no restablecer en reinicios/migraciones.
- sa es reservado y no editable desde administración ordinaria. Su contraseña solo la define privadamente el propietario en Terminal local; no inventarla ni pedirla por chat.
- Los secretos de sesión/dispositivo solo se conservan en el almacén seguro nativo. Auditar cambios transaccionalmente y no registrar contraseñas, hashes ni tokens.
- Consultar dispositivos, sesiones y permisos vigentes en Go en cada operación protegida.

## Reglas confirmadas en etapa 3

- Catálogo y dinero se calculan exclusivamente en `internal/catalog` con decimal exacto. Dinero JSON como cadenas; no introducir cálculos financieros con number en React.
- Conservar equivalencia por valor con piso en base, extras explícitos separados y tramos incrementales. Nunca imponer una opción original obligatoria si el mínimo es cero.
- Combos referencian productos estables; no duplicar identidades ni consumo. Validar ciclos, disponibilidad y personalización de hijos.
- Respetar revisión optimista y lock de catálogo; cada escritura y su auditoría pertenecen a la misma transacción. No eludir permisos de precios/estado/disponibilidad/imágenes mediante el editor completo.
- 0003 aplicada es inmutable. La previsualización no registra ventas ni movimientos de inventario. Leer docs/CATALOG.md antes de ampliar reglas.
- Preservar los PNG originales de assets/brand; BrandLogo elige por tema y conserva proporciones. No sustituir la marca ni confundirla con el logotipo futuro del restaurante.

## Regla visual permanente desde etapa 4

- Utilizar components/Accordion para secciones colapsables. Encabezado completo con título, resumen opcional y chevron; tarjeta discreta, temas claro/oscuro y transición con movimiento reducido.
- Mantener controles montados al cerrar, conservar valores/errores y revelar automáticamente errores de formulario. Preservar relaciones aria y navegación por teclado. No reintroducir details/summary sin este patrón ni convertir indiscriminadamente todas las pantallas en acordeones.
- Pedidos confirmados conservan snapshots y unidades estables. Transferir o dividir no recalcula precios ni genera preparación. No afirmar envío a cocina, pago o entrega hasta contar con sus módulos reales.

## Continuidad de etapa 4

- 0001–0005 ya están aplicadas y son inmutables. La siguiente ampliación del esquema requiere 0006. Leer docs/RESTAURANT_OPERATIONS.md, docs/KITCHEN_AND_PRINTING.md y docs/VERIFICATION.md.
- Toda escritura operativa conserva UUID, revisión, resultado idempotente, auditoría y eventos en la misma transacción. No regenerar UUID al reintentar una respuesta incierta.
- Observaciones y reducciones de receta idéntica conservan importes de unidades históricas; otros cambios requieren cotización y aceptación explícitas. No recalcular automáticamente pedidos anteriores por cambios de catálogo o promociones.
- Ventanilla pertenece a takeaway. Mantener preparación, pago y entrega independientes. No agregar una acción de entregar sin el futuro cobro real.
- El borrador persistido y el editor local son estados distintos. No descartar selecciones ante sondeo o conflicto. No prometer recuperación offline después de cerrar el proceso.
- Las pruebas visuales usan solo bases adel_pos_test_* y transporte IPC de prueba exclusivo de Node. No introducir un acceso web alternativo en producción ni pasar tokens al WebView.

## Regla permanente desde etapa 5

- Iniciar en pantalla completa nativa mediante Tauri, con salida/regreso visibles y modo ventana normal. No imponer quiosco ni bloquear controles o Spaces de macOS.
- 1280×720 es un escenario de prueba; no un ancho/alto fijo del producto. Usar todo el monitor con distribuciones fluidas y paneles prácticos; conservar formularios y selecciones al redimensionar.
- La liberación canónica es una por UUID de unidad comercial en kitchen_unit_releases; estaciones/copias no crean consumo adicional. No registrar inventario físico todavía.
- Conservar rutas/documentos históricos. Modificaciones bloqueadas permanecen en kitchen_pending_changes; no aparentar preparación lista ni avanzar una versión obsoleta.
- La cola confirma intento/lease antes del transporte y serializa por impresora. Un resultado desconocido exige resolución explícita; nunca reenviarlo automáticamente ni activar respaldo por incertidumbre.
- submitted significa aceptación CUPS/TCP, no salida física de papel. Reimprimir conserva presentación sin leyendas adicionales y bloquea otras copias sin resolver.
- El planificador y los trabajadores pertenecen a Go independiente. SSE es el equivalente elegido a WebSocket: outbox, cursor/replay/reset, permisos por estación y Channel nativo sin tokens en React. Conservar HTTP idempotente para comandos/acuse KDS.
- TestControlledCUPSPrinter nunca imprime en suites normales. El envío de hardware necesita destino explícito y ADEL_HARDWARE_SEND=1; no activar en CI ni dirigir a la impresora predeterminada por inferencia.
