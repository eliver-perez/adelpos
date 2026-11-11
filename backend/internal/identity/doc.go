// Package identity administra la identidad documental del establecimiento (etapa 9B, fase B y §19
// de la especificación): la información adicional, el texto de encabezado, el texto de pie y el
// logotipo del restaurante con su presentación en los documentos térmicos.
//
// # Qué no se duplica
//
// El nombre comercial, la dirección, el teléfono y el correo siguen en app.establishment y se
// editan en Configuración general; esta identidad solo los combina al leerse.
//
// # Persistencia (migración 0019)
//
//   - app.document_identity: fila única con los textos, el logotipo vigente (logo_id, NULL sin
//     logotipo), su presentación (ancho relativo, alineación, método y umbral) y la revisión
//     optimista.
//   - app.document_logos: originales inmutables (PNG o JPEG validados con internal/logoraster), uno
//     por contenido (SHA-256 único).
//   - app.document_logo_rasters: conversiones monocromáticas inmutables por logotipo, ancho de papel
//     (58 y 80 mm) y presentación. Se generan al cargar el logotipo o al cambiar su presentación,
//     nunca al imprimir, y se reutilizan si la misma combinación ya existe.
//
// Reemplazar o retirar el logotipo solo cambia la referencia de la identidad: los originales y sus
// conversiones nunca se borran, así que un documento ya emitido (que guarda la copia de la identidad
// con los identificadores de sus conversiones) se reimprime idéntico.
//
// # Rutas (delegadas desde la administración con la misma transacción)
//
//   - GET  documents/identity: identidad con vistas previas (original y papel de 58 y 80 mm).
//   - PUT  documents/identity: textos y presentación, con operation_id y revisión.
//   - POST documents/identity/logo: cargar o reemplazar el logotipo (content_base64).
//   - POST documents/identity/logo/remove: retirar el logotipo.
//   - POST documents/identity/logo/preview: vista previa de otra presentación, sin guardar.
//
// Leer exige documents.templates.read o settings.read; escribir y la vista previa de otra
// presentación exigen documents.identity.manage. Los permisos se comprueban antes de leer el cuerpo
// o escribir. Toda escritura guarda su resultado idempotente en app.restaurant_operations, audita
// (documents.identity_updated, documents.logo_uploaded, documents.logo_removed; nunca bytes ni
// base64) y publica documents.identity.changed en el ámbito SSE documents, en la misma transacción.
// Las escrituras se serializan con el lock de fila de la identidad (no hay lock consultivo nuevo).
//
// # Conexiones con otros módulos
//
//   - ReportLogo es la fuente del logotipo de los reportes PDF y Excel (reports.LogoSource): solo un
//     SELECT dentro de la transacción de solo lectura del reporte.
//   - Rasters implementa documents.RasterSource para el trabajador de impresión y la vista previa.
package identity
