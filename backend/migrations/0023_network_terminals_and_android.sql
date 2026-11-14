-- Etapa 9G: la única migración de la etapa (servidor del restaurante, terminales en red y preparación para Android;
-- esquema 23, decisión D12 del coordinador). Cada fase agrega aquí su propia sección con el encabezado «Fase X». Es
-- transaccional (el migrador aplica todas las pendientes en una sola transacción) y no modifica filas operativas:
-- ventas, pagos, cortes, cuentas, pedidos, trabajos de impresión ni sus fotografías.

-- =====================================================================================================================
-- Fase B · Emparejamiento y conexión cifrada (§3 de la especificación, decisiones D1, D5, D6 y D9)
-- =====================================================================================================================

-- 1. Datos de cada terminal (§3.3): tipo de dispositivo (lista cerrada de D9: escritorio y, en el futuro, comandero
--    Android y cocina; las terminales existentes son de escritorio), sistema operativo y versión de la aplicación que
--    informó al emparejarse, cuándo se emparejó y la huella SPKI del certificado del servidor que fijó (hexadecimal de
--    64 caracteres; sirve a soporte para saber con qué identidad del servidor se emparejó). Las terminales enroladas
--    antes de la etapa 9G (la propia computadora principal) quedan sin sistema, versión ni emparejamiento.
ALTER TABLE app.devices
    ADD COLUMN kind text NOT NULL DEFAULT 'desktop'
        CONSTRAINT devices_kind_check CHECK (kind IN ('desktop', 'android', 'kitchen')),
    ADD COLUMN os text NOT NULL DEFAULT ''
        CONSTRAINT devices_os_check CHECK (length(os) <= 80),
    ADD COLUMN app_version text NOT NULL DEFAULT ''
        CONSTRAINT devices_app_version_check CHECK (length(app_version) <= 40),
    ADD COLUMN paired_at timestamptz,
    ADD COLUMN server_fingerprint text
        CONSTRAINT devices_server_fingerprint_check CHECK (server_fingerprint IS NULL OR server_fingerprint ~ '^[0-9a-f]{64}$');

-- 2. Solicitudes de emparejamiento con código (§3.2, D5). Una terminal nueva envía el nombre propuesto, su tipo,
--    sistema, versión y el compromiso SHA-256 de su número aleatorio (terminal_commitment); el servidor guarda su propio
--    número (server_nonce), la huella de su certificado (server_fingerprint) y solo el SHA-256 del secreto con el que la
--    terminal consulta la solicitud (secret_hash). Al revelar su número (terminal_nonce, que debe corresponder al
--    compromiso) ambos derivan el código de 6 dígitos SHA-256(huella ‖ número de la terminal ‖ número del servidor ‖ id);
--    el código no se guarda. Estados: pending (esperando aprobación), approved (aprobada; la terminal recoge su
--    credencial una sola vez), claimed (credencial entregada; device_id es la terminal creada), rejected (rechazada por
--    un administrador, por 5 códigos incorrectos o porque el número revelado no correspondía al compromiso) y cancelled
--    (la terminal canceló). Una solicitud pendiente o aprobada cuyo expires_at pasó está vencida (Go lo calcula al leer):
--    vence a los 5 minutos y, aprobada, la terminal tiene otros 5 para recoger su credencial. source_address es la IP de
--    origen (límite de solicitudes por origen). Las solicitudes de más de 7 días se borran al crear otra.
CREATE TABLE app.device_pairing_requests (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    proposed_name text NOT NULL
        CONSTRAINT device_pairing_requests_name_check CHECK (length(proposed_name) BETWEEN 1 AND 80),
    device_kind text NOT NULL
        CONSTRAINT device_pairing_requests_kind_check CHECK (device_kind IN ('desktop', 'android', 'kitchen')),
    os text NOT NULL
        CONSTRAINT device_pairing_requests_os_check CHECK (length(os) BETWEEN 1 AND 80),
    app_version text NOT NULL
        CONSTRAINT device_pairing_requests_app_version_check CHECK (length(app_version) BETWEEN 1 AND 40),
    source_address inet NOT NULL,
    secret_hash text NOT NULL
        CONSTRAINT device_pairing_requests_secret_hash_key UNIQUE
        CONSTRAINT device_pairing_requests_secret_hash_check CHECK (secret_hash ~ '^[0-9a-f]{64}$'),
    terminal_commitment text NOT NULL
        CONSTRAINT device_pairing_requests_commitment_check CHECK (terminal_commitment ~ '^[0-9a-f]{64}$'),
    terminal_nonce text
        CONSTRAINT device_pairing_requests_terminal_nonce_check CHECK (terminal_nonce IS NULL OR terminal_nonce ~ '^[0-9a-f]{64}$'),
    server_nonce text NOT NULL
        CONSTRAINT device_pairing_requests_server_nonce_check CHECK (server_nonce ~ '^[0-9a-f]{64}$'),
    server_fingerprint text NOT NULL
        CONSTRAINT device_pairing_requests_fingerprint_check CHECK (server_fingerprint ~ '^[0-9a-f]{64}$'),
    state text NOT NULL DEFAULT 'pending'
        CONSTRAINT device_pairing_requests_state_check CHECK (state IN ('pending', 'approved', 'claimed', 'rejected', 'cancelled')),
    failed_attempts smallint NOT NULL DEFAULT 0
        CONSTRAINT device_pairing_requests_attempts_check CHECK (failed_attempts BETWEEN 0 AND 5),
    reject_reason text
        CONSTRAINT device_pairing_requests_reject_reason_check CHECK (reject_reason IN ('rejected', 'attempts', 'commitment')),
    approved_name text
        CONSTRAINT device_pairing_requests_approved_name_check CHECK (approved_name IS NULL OR length(approved_name) BETWEEN 1 AND 80),
    created_at timestamptz NOT NULL DEFAULT now(),
    revealed_at timestamptz,
    expires_at timestamptz NOT NULL,
    decided_at timestamptz,
    decided_by uuid
        CONSTRAINT device_pairing_requests_decided_by_fkey REFERENCES app.users(id),
    device_id uuid
        CONSTRAINT device_pairing_requests_device_id_fkey REFERENCES app.devices(id),
    claimed_at timestamptz,
    CONSTRAINT device_pairing_requests_reveal_check CHECK ((terminal_nonce IS NULL) = (revealed_at IS NULL)),
    CONSTRAINT device_pairing_requests_claim_check CHECK (
        (state = 'claimed') = (device_id IS NOT NULL AND claimed_at IS NOT NULL)
    ),
    CONSTRAINT device_pairing_requests_rejection_check CHECK ((state = 'rejected') = (reject_reason IS NOT NULL)),
    CONSTRAINT device_pairing_requests_approval_check CHECK (
        state NOT IN ('approved', 'claimed') OR (approved_name IS NOT NULL AND terminal_nonce IS NOT NULL)
    )
);
CREATE INDEX device_pairing_requests_source_idx ON app.device_pairing_requests (source_address, created_at DESC);
CREATE INDEX device_pairing_requests_open_idx ON app.device_pairing_requests (created_at)
    WHERE state IN ('pending', 'approved');
