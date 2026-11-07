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
