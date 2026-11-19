-- Etapa A2: la única migración de la etapa (aplicación Android de meseros: Para llevar, impresión de la precuenta y
-- aviso de platillos listos; esquema 26). Cada parte del servidor agrega aquí su propia sección con el encabezado
-- «Parte X», en el orden de las partes (la parte B, Para llevar, no necesitó cambios en la base). Es transaccional (el
-- migrador aplica todas las pendientes en una sola transacción) y no modifica filas existentes: ventas, pagos, cortes,
-- cuentas, pedidos, sesiones, usuarios, terminales, impresoras, trabajos de impresión ni sus fotografías. Solo agrega
-- columnas con su valor por omisión (o vacías), una tabla nueva vacía, índices y restricciones que las filas existentes
-- ya cumplen.

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

-- =====================================================================================================================
-- Parte D · Registro y canal de platillos listos (§4 de la especificación; decisiones A2-D4 y A2-D9 del coordinador
-- en docs/ETAPA_A2_AVANCE.md)
-- =====================================================================================================================

-- 2. Registro de «listos». El evento restaurant.account.preparation_changed no dice qué pasó a «Listo» ni para quién, y
--    con un reset del canal de eventos no se sabe qué se perdió; el aviso al mesero necesita un registro propio que
--    se pueda leer desde un cursor. Cocina (kitchen/preparation.go) escribe una fila por unidad en la misma
--    transacción que publica preparation_changed, cuando la unidad queda con todas sus tareas vivas (no canceladas)
--    en «Listo»: porque la última pasó a «Listo» o porque se canceló la que faltaba (revisión R1). «Listo» no
--    retrocede y una modificación que cambia la receta es otra versión, otra preparación y otro aviso, así que
--    UNIQUE (unit_id, version_id) impide que la misma preparación se avise dos veces.
--      · sequence es la clave y el cursor de la lectura (GET ready-alerts?after=<sequence> y su canal SSE, con
--        Last-Event-ID): crece con cada aviso; los escritores tienen el lock de la operación (7820148305) en
--        exclusiva hasta su commit y la lectura lo toma compartido, así que ningún aviso queda detrás del cursor.
--      · recipient_user_id es el responsable de la cuenta en el momento del «Listo» (transferir la cuenta cambia el
--        destinatario de lo que aún no estaba listo; lo ya avisado no se repite).
--      · recipient_device_id es, en ese mismo momento, el dispositivo android donde el responsable tiene la sesión con
--        PIN vigente de actividad más reciente (A2-D4; si es de antes del inicio del día comercial, solo usada en los
--        últimos 30 minutos, A2-D12) entre los que pueden recibir el aviso (no revocados, con «Avisar platillos
--        listos» activado y con la aplicación 0.2.0 o posterior si Go conoce su versión; revisión R7), o vacío si no
--        hay ninguno (entonces el aviso va a todos los dispositivos donde el mesero entró). Se fija al registrar el
--        aviso para que cada dispositivo lo reciba o no sin importar cuándo lea, aunque el mesero cambie de dispositivo
--        o un dispositivo se reconecte tarde: ni se pierde ni llega a dos dispositivos por un cambio de sesión a
--        destiempo.
--      · alert_key es la clave del aviso que ve el dispositivo: la comparten las unidades que cocina marcó «Listo» en
--        la misma operación y en la misma cuenta, y nunca se reutiliza (un uuid aleatorio), ni siquiera si se restaura
--        un respaldo anterior y sequence vuelve a dar números ya vistos (revisión R3). El dispositivo descarta los
--        avisos repetidos por esta clave; sequence es solo el cursor.
--      · El nombre, la cantidad, la mesa o el folio se arman al leer; nunca se guardan ni se envían precios.
--      · Es un dato derivado, no de operación: lo de más de 2 días se poda al escribir (el aviso es del día comercial
--        en curso).
--    Las instalaciones existentes empiezan sin filas: lo que ya estaba listo antes de la actualización no se avisa.
CREATE TABLE app.ready_notices (
    sequence bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    unit_id uuid NOT NULL
        CONSTRAINT ready_notices_unit_id_fkey REFERENCES app.restaurant_item_units(id),
    version_id uuid NOT NULL
        CONSTRAINT ready_notices_version_id_fkey REFERENCES app.restaurant_item_versions(id),
    account_id uuid NOT NULL
        CONSTRAINT ready_notices_account_id_fkey REFERENCES app.restaurant_accounts(id),
    recipient_user_id uuid NOT NULL
        CONSTRAINT ready_notices_recipient_user_id_fkey REFERENCES app.users(id),
    recipient_device_id uuid
        CONSTRAINT ready_notices_recipient_device_id_fkey REFERENCES app.devices(id),
    alert_key uuid NOT NULL,
    ready_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT ready_notices_unit_version_key UNIQUE (unit_id, version_id)
);
-- La lectura de un dispositivo: los avisos de sus meseros desde el cursor; la poda, por antigüedad.
CREATE INDEX ready_notices_recipient_idx ON app.ready_notices (recipient_user_id, sequence);
CREATE INDEX ready_notices_ready_at_idx ON app.ready_notices (ready_at);
COMMENT ON TABLE app.ready_notices IS
    'Unidades que pasaron a Listo, con el responsable de su cuenta en ese momento: avisos de platillos listos (etapa A2, parte D, A2-D9)';

-- La lectura de los avisos revisa, en cada consulta de cada dispositivo, sus sesiones con PIN: las creadas desde el
-- inicio del día comercial y las vigentes. sessions no se poda y cada entrada con PIN es una fila, así que sin estos
-- índices parciales cada lectura recorrería todas las sesiones del dispositivo (revisión R5).
CREATE INDEX sessions_pin_device_created_idx ON app.sessions (device_id, created_at) WHERE opened_with_pin;
CREATE INDEX sessions_pin_device_open_idx ON app.sessions (device_id) WHERE opened_with_pin AND revoked_at IS NULL;

-- 3. «Avisar platillos listos», «Sonido» y «Vibración» de cada teléfono o tableta de meseros (§4.4 y §4.5), en
--    Sistema › Terminales › Editar. Activados por omisión, también en las terminales existentes (solo los usan los
--    dispositivos android). Con el aviso apagado, ready-alerts responde active false y sin avisos; sonido y vibración
--    los aplica el dispositivo a su canal «Platillos listos» (A2-D7).
ALTER TABLE app.devices
    ADD COLUMN ready_alerts_enabled boolean NOT NULL DEFAULT true,
    ADD COLUMN ready_alerts_sound boolean NOT NULL DEFAULT true,
    ADD COLUMN ready_alerts_vibration boolean NOT NULL DEFAULT true;
