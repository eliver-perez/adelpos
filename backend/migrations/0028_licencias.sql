-- Etapa L1: la única migración de la etapa (integración con el servidor de licencias; esquema 28). Cada fase agrega aquí
-- su propia sección con el encabezado «Fase X» (decisión L1-D23 del coordinador), en el orden «Fase A (revisión R1)»,
-- «Fase B» y «Fase D». Es transaccional (el migrador aplica todas las pendientes en una sola transacción). No toca
-- ventas, pagos, cortes, cuentas, pedidos, sesiones, usuarios, roles ni terminales: solo agrega lo que dice cada
-- sección. La licencia, su llave y la última hora confiable no van en la base sino en identity/ (decisión L1-D9): la
-- base solo guarda una segunda copia de las revisiones aceptadas (revisión R1) y de la última hora confiable (revisión
-- R2), el permiso y los cupos.

-- =====================================================================================================================
-- Fase A (revisión R1) · Segundas copias del historial de revisiones (R1-3) y de la última hora confiable (R2-3)
-- =====================================================================================================================

-- 1. La revisión más alta aceptada de cada licencia (contrato §10.4) vive en identity/licencia/estado.json (L1-D9).
--    Esta tabla es una segunda copia: la revisión mínima aceptable es el máximo de las dos. Si se restaura un respaldo
--    viejo de la base, manda la de identity/; si se borra o se edita estado.json, manda la de la base. Las dos se
--    escriben al aceptar una licencia, primero esta (si falla, la licencia no se acepta). Nunca baja: el servidor solo
--    la sube (internal/licensing, RecordAcceptedRevision). No guarda la licencia ni el código comercial.
CREATE TABLE app.license_revisions (
    license_id uuid CONSTRAINT license_revisions_pkey PRIMARY KEY,
    license_revision bigint NOT NULL
        CONSTRAINT license_revisions_revision_check CHECK (license_revision >= 1),
    sha256 text NOT NULL
        CONSTRAINT license_revisions_sha256_check CHECK (sha256 ~ '^[0-9a-f]{64}$'),
    accepted_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);
COMMENT ON TABLE app.license_revisions IS
    'Segunda copia de la revisión más alta aceptada de cada licencia (etapa L1, revisión R1; la primera está en identity/licencia)';

-- 2. La última hora confiable (§5.6 del contrato y §6 de la especificación) vive en identity/licencia/estado.json
--    (L1-D9). Esta tabla es su segunda copia (hallazgo R2-3 de la revisión R2), con una sola fila: la que vale es el
--    máximo de las dos. Si se edita o se borra estado.json, manda la de la base; si se restaura un respaldo viejo de la
--    base, manda la de identity/. El servidor la sube cada minuto junto con estado.json y nunca la baja
--    (internal/licensing, RaiseTrustedTimeCopy), salvo «Comprobar la hora con internet» (§6.3) y el desbloqueo de
--    soporte (§6.4), que bajan las dos copias (ReplaceTrustedTimeCopy). Nace vacía: el primer arranque de la 0.15.0 la
--    llena con la mayor entre estado.json y la última actividad de la auditoría (L1-D15). source es la fuente de esa
--    hora, con los nombres de internal/licensing.
CREATE TABLE app.license_trusted_time (
    id smallint NOT NULL DEFAULT 1 CONSTRAINT license_trusted_time_pkey PRIMARY KEY
        CONSTRAINT license_trusted_time_single_row_check CHECK (id = 1),
    trusted_time timestamptz NOT NULL,
    source text NOT NULL
        CONSTRAINT license_trusted_time_source_check CHECK (source IN ('server_time', 'issued_at', 'local_clock',
            'operation', 'audit', 'internet_check', 'support_unlock')),
    updated_at timestamptz NOT NULL DEFAULT now()
);
COMMENT ON TABLE app.license_trusted_time IS
    'Segunda copia de la última hora confiable (etapa L1, revisión R2; la primera está en identity/licencia)';

-- =====================================================================================================================
-- Fase B · Cliente del servidor de licencias (§3 y §4 de la especificación; decisiones L1-D21 y L1-D23)
-- =====================================================================================================================

-- 1. «Administrar la licencia» (L1-D21): activar, transferir, buscar actualización, liberar, la solicitud sin internet,
--    importar un archivo de licencia y crear una identidad nueva para una instalación copiada. Ver el estado de la
--    licencia no pide permiso. Solo lo recibe el rol Administrador de fábrica (administrator): ningún otro rol de
--    fábrica lo lleva, tampoco el alcance de mesero. En una instalación nueva los roles nacen en el asistente, después
--    de las migraciones, y el asistente le da al administrador todos los permisos (setup.go). Se registra también en Go
--    (administration.Permissions) con la misma descripción.
INSERT INTO app.permissions(key, description) VALUES
    ('license.manage', 'Administrar la licencia');
INSERT INTO app.role_permissions(role_id, permission_key)
SELECT id, 'license.manage' FROM app.roles WHERE code = 'administrator';

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
--        minutos, tenga o no una sesión vigente (revisión R3, L1-D36); la fila se borra al cerrarse la última sesión de
--        esa terminal. Una actividad con fecha futura se baja a la hora de Go con la hora confiable (L1-D43, L1-D49).
--        Varios meseros con su PIN en la misma tableta son una sola fila.
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
