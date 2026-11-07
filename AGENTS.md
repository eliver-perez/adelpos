# Instrucciones para continuar

Leer README.md, ARCHITECTURE.md, API_CONTRACTS.md y NEXT_STEPS_FOR_SOL.md antes de ampliar funcionalidad.

- Mantener Go independiente de Tauri y toda operación SQL en backend.
- No introducir módulos comerciales fuera del alcance solicitado ni datos ficticios.
- No leer/imprimir secretos de .local ni modificar clústeres ajenos. No versionar .local, .tools, cachés o builds.
- Mantener localhost por defecto, CSP y capabilities restringidas.
- Metadatos desde config/product.json; ejecutar scripts/dev.sh sync al cambiarlos.
- Migraciones aplicadas inmutables. Añadir pruebas significativas de transacciones/contratos cuando cambien.
- Verificar con scripts/dev.sh test y, para cambios de persistencia, test-integration contra la base temporal del proyecto.
- Actualizar la documentación con lo realmente ejecutado, sin presentar tareas pendientes como completas.
