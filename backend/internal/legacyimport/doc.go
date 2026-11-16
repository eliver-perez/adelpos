// Package legacyimport importa el paquete de migración del sistema anterior (`.adelmig`, etapa 9H, §4): la
// correspondencia del catálogo, las mesas y los usuarios a lo vivo, el historial completo en las tablas propias
// app.legacy_* (migración 0024, sección C) y la validación posterior (§4.4).
//
// Flujo:
//
//   - La aplicación sube el paquete por partes (uploads.go): el servicio de macOS no puede leer las carpetas del
//     usuario, así que el archivo llega a una carpeta de trabajo privada dentro de la carpeta de datos.
//   - Analizar (en segundo plano, worker.go): abre el paquete con la frase de paso, lo lee completo (load.go), hace la
//     correspondencia en seco (plan.go) y entrega el resumen previo (preview.go), con lo que impide importar
//     (clean.go) y el lote que se reemplazaría.
//   - Importar (en segundo plano): vuelve a leer el paquete, comprueba la instalación, hace el respaldo verificado
//     previo y escribe todo en una sola transacción (write.go): o queda el lote nuevo completo o el anterior intacto.
//     Dentro de esa transacción compara lo escrito con lo calculado del paquete por una pasada independiente
//     (expected.go y validate.go).
//
// Los importes del anterior (MONEY, 4 decimales) se manejan como enteros de diezmilésimos (Money) y las cantidades
// (NUMERIC(18,3)) como milésimos (Quantity): nada se redondea ni pasa por float. Los datos son reales del restaurante:
// nunca se escriben en bitácoras ni en mensajes; solo en la base, en el resumen para el propietario y en el PDF.
package legacyimport
