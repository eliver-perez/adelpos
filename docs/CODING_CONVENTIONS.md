# Convenciones permanentes de código

La claridad y mantenibilidad tienen prioridad sobre reducir líneas. Estas reglas se aplican al código existente y a todas las etapas siguientes.

- No ofuscar, minificar ni compactar fuentes. Los artefactos de compilación permanecen separados de los fuentes.
- Go: gofmt y bloques de control/funciones expandidos en varias líneas. Revisar manualmente las funciones anónimas y expresiones extensas.
- TypeScript, JavaScript y React: Prettier, dos espacios, nombres descriptivos y etiquetas JSX anidadas en líneas independientes.
- Evitar múltiples operaciones en una línea, ternarios anidados extensos y funciones gigantescas. Extraer responsabilidades con nombres claros.
- Comentarios explicativos en español para validaciones, seguridad, reglas y transacciones; no comentar instrucciones obvias.
- Rust, Python y shell: indentación consistente y operaciones separadas; respetar sus convenciones.
- No cambiar lógica incidentalmente al reformatear. Las migraciones SQL ya aplicadas son inmutables y se excluyen del reformateo.

Comandos desde la raíz:

```bash
./scripts/dev.sh format
./scripts/dev.sh format-check
```

La configuración compartida vive en `.editorconfig`, `.prettierrc.json` y `.prettierignore`. Prettier queda instalado localmente y fijado en el lockfile, según su [documentación oficial](https://prettier.io/docs/install).
