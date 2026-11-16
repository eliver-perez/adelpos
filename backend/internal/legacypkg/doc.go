// Package legacypkg es el formato del paquete de migración del sistema anterior (`.adelmig`, etapa 9H, §3.2):
// escritura, lectura, cifrado, manifiesto, huellas y errores tipificados.
//
// Es neutral: no sabe nada de Docker ni de SQL Server. Lo usan el lector del respaldo (internal/legacyread, solo desde
// el subcomando legacy-prepare), el importador del servidor y los paquetes sintéticos de las pruebas
// (internal/legacysynth). La estructura del archivo está documentada en docs/MIGRACION_FORMATO.md.
//
// Resumen del archivo:
//
//   - Encabezado en claro: firma de 8 bytes («ADELMIG» y un byte NUL), versión del formato (uint16), un JSON corto con
//     los parámetros (Argon2id, cifrado, tamaño de bloque y el valor de comprobación de la llave) y el SHA-256 de todo
//     lo anterior.
//   - Cuerpo: flujo cifrado por bloques con AES-256-GCM. El nonce de cada bloque es un prefijo aleatorio, el número de
//     bloque y una bandera de último bloque, y el AAD es el SHA-256 del encabezado: un archivo truncado, reordenado o
//     alterado no se descifra.
//   - Texto claro: gzip de un tar con manifest.json primero y después tables/<tabla>.json (arreglo de objetos).
//
// Los datos son reales del restaurante: nunca se escriben en claro en el disco (las tablas en preparación también van
// cifradas, con una llave aleatoria que solo vive en memoria) y nunca se registran en bitácoras ni mensajes.
package legacypkg
