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

-- =====================================================================================================================
-- Fase C · Cocina por estaciones y terminal de cocina (§5, decisiones D6 y D7). No depende de la sección de la fase B
-- y no modifica filas operativas (liberaciones, tareas, comandas ni cola).
-- =====================================================================================================================

-- 1. «Estaciones que muestra esta terminal». kitchen_device_stations (0005) deja de ser un ámbito de autorización (vacío
--    significaba «ninguna estación» para quien no tenía kitchen.stations.all) y pasa a ser la selección de estaciones que
--    muestra la terminal. La marca kitchen_all_stations (predeterminada) muestra todas, incluidas las que se creen
--    después. Una terminal que ya tenía estaciones asignadas conserva su selección (marca en falso); las demás muestran
--    todas. kitchen_sound_alert es el aviso sonoro opcional de la terminal al llegar una comanda nueva de sus
--    estaciones (desactivado por omisión).
ALTER TABLE app.devices
    ADD COLUMN kitchen_all_stations boolean NOT NULL DEFAULT true,
    ADD COLUMN kitchen_sound_alert boolean NOT NULL DEFAULT false;
UPDATE app.devices d
SET kitchen_all_stations = false
WHERE EXISTS (SELECT FROM app.kitchen_device_stations s WHERE s.device_id = d.id);

-- 2. «Estaciones que puede atender» cada usuario: todas por omisión o una selección. Quien tiene kitchen.stations.all
--    atiende siempre todas (Go lo decide; la selección guardada se conserva sin efecto). Lo que se ve en Cocina es la
--    intersección de las estaciones de la terminal y las del usuario. Son configuración: «Quitar datos de prueba»
--    (fase A) desvincula de aquí una estación de prueba antes de quitarla.
ALTER TABLE app.users
    ADD COLUMN kitchen_all_stations boolean NOT NULL DEFAULT true;
CREATE TABLE app.kitchen_user_stations (
    user_id uuid NOT NULL CONSTRAINT kitchen_user_stations_user_id_fkey REFERENCES app.users(id),
    station_id uuid NOT NULL CONSTRAINT kitchen_user_stations_station_id_fkey REFERENCES app.kitchen_stations(id),
    CONSTRAINT kitchen_user_stations_pkey PRIMARY KEY (user_id, station_id)
);
CREATE INDEX kitchen_user_stations_station_idx ON app.kitchen_user_stations(station_id);

-- 3. Encargo de la fase E: la ruta antigua de anulación se retiró y accounts.manage ya no cancela cuentas vacías; la
--    descripción se alinea con la que muestra Go.
UPDATE app.permissions SET description = 'Editar datos de cuentas' WHERE key = 'accounts.manage';
