-- Etapa L1: la única migración de la etapa (integración con el servidor de licencias; esquema 28). Cada fase agrega aquí
-- su propia sección con el encabezado «Fase X» (decisión L1-D23 del coordinador). Es transaccional (el migrador aplica
-- todas las pendientes en una sola transacción). No toca ventas, pagos, cortes, cuentas, pedidos, sesiones, usuarios,
-- roles ni terminales: solo agrega lo que dice cada sección.

-- =====================================================================================================================
-- Fase D · Cupos de la licencia (§5 de la especificación, §7 del contrato; decisiones L1-D2, L1-D3, L1-D7 y L1-D13)
-- =====================================================================================================================

-- 1. app.license_seats: el cupo que ocupa cada terminal (internal/licenseseats). Una fila por terminal y tipo de cupo;
--    el conteo es derivado: una fila solo cuenta mientras sigue «en uso», y liberar casi nunca escribe.
--      · cash_register (cupo cash_registers, L1-D3): la terminal que abre el turno y la que captura con el turno activo
--        de su caja. La fila apunta al turno (shift_id) y cuenta mientras ese turno esté abierto o en arqueo de
--        cierre: al cerrarlo, también desde otra computadora, se liberan todas las terminales que lo usaban.
--      · order_taker (cupo order_takers, L1-D2 y L1-D13): la terminal que captura sin turno activo en su caja. Cuenta
--        mientras su última actividad de captura (last_activity_at, con la hora de Go, L1-D16) tenga menos de 15
--        minutos y la terminal tenga alguna sesión vigente. Varios meseros con su PIN en la misma tableta son una sola
--        fila.
--      · kitchen_display (cupo kitchen_displays, L1-D7): la terminal en Preparación. La fila apunta a la sesión que
--        entró (session_id) y cuenta mientras esa sesión siga vigente.
--    Las llaves foráneas borran la fila con la terminal, el turno o la sesión (nunca se borran en la operación, pero
--    así nada impide limpiar datos de prueba).
CREATE TABLE app.license_seats (
    device_id uuid NOT NULL REFERENCES app.devices(id) ON DELETE CASCADE,
    kind text NOT NULL
        CONSTRAINT license_seats_kind_check CHECK (kind IN ('cash_register', 'order_taker', 'kitchen_display')),
    shift_id uuid REFERENCES app.cash_shifts(id) ON DELETE CASCADE,
    session_id uuid REFERENCES app.sessions(id) ON DELETE CASCADE,
    acquired_at timestamptz NOT NULL,
    last_activity_at timestamptz NOT NULL,
    PRIMARY KEY (device_id, kind),
    -- Cada tipo con su vínculo y solo con el suyo: el turno en la caja y la sesión en la cocina.
    CONSTRAINT license_seats_link_check CHECK (
        (kind = 'cash_register' AND shift_id IS NOT NULL AND session_id IS NULL)
        OR (kind = 'order_taker' AND shift_id IS NULL AND session_id IS NULL)
        OR (kind = 'kitchen_display' AND shift_id IS NULL AND session_id IS NOT NULL)
    )
);
-- Para comprobar si el turno o la sesión de una fila siguen vigentes y para los borrados en cascada. La tabla tiene a lo
-- más tres filas por terminal: el conteo por tipo no necesita índice propio.
CREATE INDEX license_seats_shift_idx ON app.license_seats(shift_id) WHERE shift_id IS NOT NULL;
CREATE INDEX license_seats_session_idx ON app.license_seats(session_id) WHERE session_id IS NOT NULL;

-- 2. Una instalación que se actualiza con turnos abiertos: la terminal que abrió cada turno activo ya ocupa su cupo de
--    caja («la terminal que abre el turno ocupa un cupo», L1-D3), así que el conteo empieza completo. Si la licencia
--    trae menos cajas, nadie se interrumpe: solo no se ocupan cupos nuevos hasta quedar dentro del límite (§7 del
--    contrato). No hay comanderos ni pantallas de cocina que copiar: se ocupan con la primera captura o al entrar a
--    Preparación.
INSERT INTO app.license_seats(device_id, kind, shift_id, acquired_at, last_activity_at)
SELECT DISTINCT ON (s.opened_device_id) s.opened_device_id, 'cash_register', s.id, s.opened_at, s.opened_at
FROM app.cash_shifts s
WHERE s.state IN ('open', 'closing')
ORDER BY s.opened_device_id, s.opened_at DESC;
