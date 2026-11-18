-- Etapa A2: la única migración de la etapa (aplicación Android de meseros: Para llevar, impresión de la precuenta y
-- aviso de platillos listos; esquema 26). Cada parte del servidor agrega aquí su propia sección con el encabezado
-- «Parte X», en el orden de las partes (la parte B, Para llevar, no necesitó cambios en la base). Es transaccional (el
-- migrador aplica todas las pendientes en una sola transacción) y no modifica filas existentes: ventas, pagos, cortes,
-- cuentas, pedidos, sesiones, usuarios, terminales, impresoras, trabajos de impresión ni sus fotografías. Solo agrega
-- columnas con su valor por omisión (o vacías), índices y restricciones que las filas existentes ya cumplen.

-- =====================================================================================================================
-- Parte C · Impresora de precuentas por dispositivo (§5 de la especificación; decisión A2-D3 del coordinador en
-- docs/ETAPA_A2_AVANCE.md)
-- =====================================================================================================================

-- 1. Impresora de precuentas de un teléfono o tableta de meseros (§0-B: el mesero imprime «en la impresora asignada a
--    ese dispositivo»). Se elige en Administración › Sistema › Terminales › Editar entre las mismas impresoras que
--    sirven para precuentas y tickets en Cobros: las de red (escpos_tcp o la cola del equipo servidor) y las USB de una
--    terminal con agente de impresión (agent con agent_device_id). Go la valida al guardarla (activa y que puede
--    imprimir, kitchen.PrinterProblem) y solo la acepta en un dispositivo android; la restricción lo repite. Vacía por
--    omisión: un dispositivo android sin ella no imprime en ninguna otra (A2-D3: 409 PRINTER_NOT_CONFIGURED con
--    details.scope «device»), y las terminales de escritorio y las cajas siguen con la impresora de su caja o la
--    general de Cobros, como hasta ahora. Reintentar y reimprimir un documento pedido desde el dispositivo usan la
--    misma impresora. «Quitar datos de prueba» la desvincula si es una impresora de prueba (maintenance/apply.go).
ALTER TABLE app.devices
    ADD COLUMN precheck_printer_id uuid
        CONSTRAINT devices_precheck_printer_id_fkey REFERENCES app.kitchen_printers(id),
    ADD CONSTRAINT devices_precheck_printer_android_check CHECK (precheck_printer_id IS NULL OR kind = 'android');
CREATE INDEX devices_precheck_printer_idx ON app.devices (precheck_printer_id) WHERE precheck_printer_id IS NOT NULL;
COMMENT ON COLUMN app.devices.precheck_printer_id IS
    'Impresora de precuentas de un dispositivo android: imprime solo en ella (etapa A2, parte C, A2-D3)';
