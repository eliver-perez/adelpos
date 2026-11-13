-- Etapa 9F: la única migración de la etapa (§7.3, esquema 22). Cada fase agrega aquí su propia sección. Es
-- transaccional (el migrador aplica todas las pendientes en una sola transacción) y no modifica filas operativas:
-- ventas, pagos, cortes, trabajos de impresión ni sus fotografías.

-- =====================================================================================================================
-- Fase B · Impresión (§4)
-- =====================================================================================================================

-- 1. Corte de papel por impresora (§4.4, decisión D5). «Cortar el papel al terminar» sigue siendo la columna cut; se
--    agregan el tipo de corte (total o parcial, GS V 0 / GS V 1) y los renglones de avance antes del corte (lista
--    cerrada de 0 a 8). Las impresoras existentes conservan su papel: corte total y 3 renglones de avance, los mismos
--    bytes que hasta la etapa 9E; las impresoras nuevas reciben 4 renglones. Sin corte, el final de cada documento no
--    cambia. supports_cut y su CHECK (NOT cut OR supports_cut) se conservan: Go guarda supports_cut = cut.
ALTER TABLE app.kitchen_printers
    ADD COLUMN cut_mode text NOT NULL DEFAULT 'full'
        CONSTRAINT kitchen_printers_cut_mode_check CHECK (cut_mode IN ('full', 'partial')),
    ADD COLUMN cut_feed_lines smallint NOT NULL DEFAULT 3
        CONSTRAINT kitchen_printers_cut_feed_lines_check CHECK (cut_feed_lines BETWEEN 0 AND 8);
ALTER TABLE app.kitchen_printers ALTER COLUMN cut_feed_lines SET DEFAULT 4;

-- 2. Nombre visible del documento previo al cobro (§4.5): «Precuenta» (precheck, predeterminado), «Ticket de venta»
--    (sales_ticket, como lo llamaba el sistema anterior) o «Cuenta» (bill). Es el mismo documento con las mismas
--    reglas; solo cambian los botones, los avisos y el encabezado impreso. Se cambia en Cobros › Políticas de cobro.
ALTER TABLE app.payment_settings
    ADD COLUMN precheck_name text NOT NULL DEFAULT 'precheck'
        CONSTRAINT payment_settings_precheck_name_check CHECK (precheck_name IN ('precheck', 'sales_ticket', 'bill'));
