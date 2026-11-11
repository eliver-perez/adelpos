// Package logoraster valida el logotipo del restaurante y lo convierte en una imagen monocromática
// para impresoras térmicas (etapa 9B, §0-B «Logotipo en impresoras térmicas» y §19). Es un paquete
// puro: no usa base de datos, HTTP ni el sistema de archivos, y el mismo contenido con las mismas
// opciones produce siempre los mismos bytes.
//
// # Validación (Validate)
//
// Solo se aceptan PNG y JPEG, con el mismo criterio que las imágenes del catálogo
// (catalog/images.go): a lo más 1 MiB, de 1 × 1 a 2048 × 2048 píxeles, firma del formato al inicio
// del archivo, dimensiones leídas con image.DecodeConfig **antes** de decodificar (un encabezado que
// declara dimensiones enormes se rechaza sin reservar memoria) y decodificación completa después
// (un archivo truncado o dañado se rechaza aunque su encabezado parezca válido). La extensión del
// nombre nunca se consulta: SVG, GIF, WebP o cualquier otro contenido se rechaza por su firma, se
// llame como se llame. Los mensajes están en español y se pueden mostrar tal cual.
//
// # Conversión (Convert)
//
//   - Ancho imprimible: 384 puntos en papel de 58 mm y 576 en 80 mm (8 puntos por mm).
//   - Ancho del logotipo: 25–100 % del ancho imprimible, con la altura proporcional; si la altura
//     excede MaxHeightDots (320 puntos, 40 mm de papel) se reduce el tamaño completo en proporción.
//   - Escalado de calidad con Catmull-Rom (golang.org/x/image/draw) componiendo la imagen sobre
//     blanco: la transparencia se imprime en blanco.
//   - Luminancia entera de la ITU-R BT.601 (la misma de color.GrayModel).
//   - Método threshold: negro si la luminancia es menor que el umbral (1–255). Método dither:
//     difuminado Floyd–Steinberg con aritmética entera y punto de decisión fijo en 128 (el umbral no
//     interviene).
//   - Alineación (izquierda, centro o derecha) mediante relleno blanco: el raster siempre mide el
//     ancho imprimible completo, de modo que la impresora no necesita comandos de justificación.
//
// # Formato del raster
//
// Raster guarda el ancho (siempre el imprimible: 384 o 576 puntos) y el alto en puntos, y los bits
// empaquetados por fila de arriba abajo: cada fila ocupa BytesPerRow() = ancho/8 bytes (48 o 72),
// el bit más significativo de cada byte es el punto de la izquierda y 1 = punto negro. Es el mismo
// orden de bits del comando GS v 0, así que ESCPos() copia las filas sin transformarlas. Un raster de
// 58 mm ocupa a lo más 48 × 320 = 15 360 bytes y uno de 80 mm, 72 × 320 = 23 040 bytes.
//
// # Comando ESC/POS
//
// ESCPos() usa el comando estándar de imagen raster GS v 0 (1D 76 30 m xL xH yL yH d1…dk) con
// m = 0 (tamaño normal), x = bytes por fila e y = filas. Epson admite 1 ≤ x ≤ 128 y 1 ≤ y ≤ 4095,
// pero hay impresoras con memoria de imagen pequeña y clones que solo leen yL; por eso la imagen se
// envía en franjas consecutivas de a lo más MaxBandRows (128) filas: cada bloque lleva yH = 0 y a lo
// más 6 144 bytes de datos en 58 mm o 9 216 en 80 mm. Las franjas se imprimen una tras otra sin
// separación. Como el raster cubre todo el ancho imprimible, la justificación (ESC a) no lo mueve.
// El resultado en papel es una prueba física pendiente del propietario.
package logoraster
