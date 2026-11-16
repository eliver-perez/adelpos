-- Etapa A1: la única migración de la etapa (aplicación Android de meseros; esquema 25). Cada fase agrega aquí su propia
-- sección con el encabezado «Fase X», en el orden de las fases. Es transaccional (el migrador aplica todas las
-- pendientes en una sola transacción) y no modifica filas existentes: ventas, pagos, cortes, cuentas, pedidos,
-- sesiones, usuarios, terminales, trabajos de impresión ni sus fotografías. Solo agrega columnas con su valor por
-- omisión, tablas nuevas y un permiso.

-- =====================================================================================================================
-- Fase B · PIN de los meseros, credencial de envío y bloqueo por inactividad (§3 de la especificación; decisiones
-- A1-D1, A1-D2, A1-D3 y A1-D5 del coordinador en docs/ETAPA_A1_AVANCE.md)
-- =====================================================================================================================

-- 1. PIN por usuario (§3.1). Cada usuario puede tener un PIN de 4 a 6 dígitos que solo sirve para entrar en los
--    teléfonos y tabletas de meseros emparejados (A1-D3; las terminales de escritorio siguen con contraseña). Se guarda
--    con el mismo hash lento y con sal de las contraseñas (Argon2id, administration/security.go); nunca el PIN.
--    pin_version sube cada vez que el PIN se define, cambia o se quita: el dispositivo guarda su verificador local con
--    esa versión y lo borra cuando la lista de usuarios del dispositivo trae otra (o ya no trae al usuario).
--    short_name es el nombre corto del botón de la pantalla de entrada (si falta, el nombre visible) y badge_color el
--    color de su distintivo, elegido de la paleta que entrega Go (administration/pin.go, la misma de las categorías del
--    catálogo); Go valida el color al guardarlo. Los usuarios existentes quedan sin PIN, sin nombre corto y sin color.
ALTER TABLE app.users
    ADD COLUMN pin_hash text,
    ADD COLUMN pin_set_at timestamptz,
    ADD COLUMN pin_version integer NOT NULL DEFAULT 0
        CONSTRAINT users_pin_version_check CHECK (pin_version >= 0),
    ADD COLUMN short_name text
        CONSTRAINT users_short_name_check CHECK (
            short_name IS NULL OR (length(short_name) BETWEEN 1 AND 20 AND short_name = btrim(short_name))
        ),
    ADD COLUMN badge_color text
        CONSTRAINT users_badge_color_check CHECK (badge_color IS NULL OR badge_color ~ '^#[0-9a-f]{6}$'),
    ADD CONSTRAINT users_pin_fields_check CHECK ((pin_hash IS NULL) = (pin_set_at IS NULL));

-- 2. Intentos fallidos del PIN y su bloqueo (§3.1, A1-D5). Son por usuario, no por dispositivo: 5 fallos dentro de 15
--    minutos, desde cualquier dispositivo, bloquean el PIN de ese usuario 5 minutos (la contraseña no se bloquea; su
--    límite sigue siendo por usuario y terminal en login_attempts). Al bloquearse, el contador vuelve a cero: al vencer
--    el bloqueo hay otros 5 intentos. last_device_id es el dispositivo del último fallo (el que provocó el bloqueo).
--    Un administrador con users.manage lo desbloquea; un acierto borra la fila.
CREATE TABLE app.pin_attempts (
    user_id uuid CONSTRAINT pin_attempts_pkey PRIMARY KEY
        CONSTRAINT pin_attempts_user_id_fkey REFERENCES app.users(id),
    failures integer NOT NULL DEFAULT 0
        CONSTRAINT pin_attempts_failures_check CHECK (failures BETWEEN 0 AND 5),
    window_start timestamptz NOT NULL DEFAULT now(),
    locked_until timestamptz,
    last_device_id uuid
        CONSTRAINT pin_attempts_last_device_id_fkey REFERENCES app.devices(id)
);

-- 3. Credencial de envío por mesero y dispositivo (A1-D1). La emite auth/pin-login junto con la sesión y solo sirve para
--    entregar pedidos capturados sin conexión (POST restaurant/offline-orders) de ese mesero desde ese dispositivo,
--    acompañada de la credencial del dispositivo: así la cola sale aunque la sesión de 12 h ya haya vencido o el mesero
--    haya entrado sin conexión. No vence por tiempo y no es una sesión (no cuenta en «Sesiones activas por tipo de
--    terminal»). Al usarla, Go vuelve a validar todo: dispositivo no revocado, usuario activo, PIN vigente con la
--    misma pin_version con que se emitió, permiso waiter_devices.use y los permisos de la operación. Una sola vigente
--    por usuario y dispositivo: la nueva sustituye a la anterior (replaced). Se revoca al cambiar o quitar el PIN
--    (pin_changed, pin_removed), al desactivar al usuario (user_disabled) y al revocar el dispositivo
--    (device_revoked). En la base solo su SHA-256, como las demás credenciales.
CREATE TABLE app.offline_order_credentials (
    id uuid CONSTRAINT offline_order_credentials_pkey PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id uuid NOT NULL
        CONSTRAINT offline_order_credentials_user_id_fkey REFERENCES app.users(id),
    device_id uuid NOT NULL
        CONSTRAINT offline_order_credentials_device_id_fkey REFERENCES app.devices(id),
    secret_hash text NOT NULL
        CONSTRAINT offline_order_credentials_secret_hash_key UNIQUE
        CONSTRAINT offline_order_credentials_secret_hash_check CHECK (secret_hash ~ '^[0-9a-f]{64}$'),
    pin_version integer NOT NULL
        CONSTRAINT offline_order_credentials_pin_version_check CHECK (pin_version > 0),
    issued_at timestamptz NOT NULL DEFAULT now(),
    last_used_at timestamptz,
    revoked_at timestamptz,
    revoke_reason text
        CONSTRAINT offline_order_credentials_revoke_reason_check CHECK (
            revoke_reason IN ('replaced', 'pin_changed', 'pin_removed', 'user_disabled', 'device_revoked')
        ),
    CONSTRAINT offline_order_credentials_revocation_check CHECK ((revoked_at IS NULL) = (revoke_reason IS NULL))
);
CREATE UNIQUE INDEX offline_order_credentials_one_active_idx ON app.offline_order_credentials (user_id, device_id)
    WHERE revoked_at IS NULL;
CREATE INDEX offline_order_credentials_device_idx ON app.offline_order_credentials (device_id);

-- 4. Bloqueo por inactividad de cada dispositivo (§3.2): tras ese tiempo sin uso el teléfono o la tableta vuelve a la
--    pantalla de entrada (salvo con una captura en curso o pedidos sin enviar de ese mesero; lo decide el dispositivo).
--    2 minutos por omisión, de 30 segundos a 1 hora; se configura en Administración › Terminales. Las terminales
--    existentes toman el valor por omisión (solo lo usan los dispositivos android).
ALTER TABLE app.devices
    ADD COLUMN idle_lock_seconds integer NOT NULL DEFAULT 120
        CONSTRAINT devices_idle_lock_seconds_check CHECK (idle_lock_seconds BETWEEN 30 AND 3600);

-- 5. Permiso para entrar con PIN (§3.1, A1-D2). La lista de usuarios del dispositivo y la entrada con PIN solo aceptan a
--    usuarios activos con este permiso efectivo (rol más excepciones individuales). Se concede al administrador y al rol
--    Mesero; no tiene efecto hasta que alguien tenga PIN. Ningún otro permiso de los roles existentes cambia: los demás
--    permisos del mesero se asignan en Personal › Roles (docs/ANDROID_INSTALAR.md). En una instalación nueva los roles
--    nacen en el asistente, después de las migraciones, y el asistente hace la misma concesión (setup.go). Se registra
--    también en Go (administration.Permissions) con la misma descripción.
INSERT INTO app.permissions(key, description) VALUES
    ('waiter_devices.use', 'Entrar con PIN en teléfonos y tabletas de meseros');
INSERT INTO app.role_permissions(role_id, permission_key)
SELECT id, 'waiter_devices.use' FROM app.roles WHERE code IN ('administrator', 'waiter');

COMMENT ON TABLE app.pin_attempts IS
    'Intentos fallidos del PIN por usuario y su bloqueo de 5 minutos (etapa A1, fase B)';
COMMENT ON TABLE app.offline_order_credentials IS
    'Credencial de envío de pedidos sin conexión por mesero y dispositivo; solo su SHA-256 (etapa A1, fase B)';

-- =====================================================================================================================
-- Las fases siguientes de la etapa A1 (por ejemplo la E, del servidor para el modo Mesero) agregan su sección aquí,
-- debajo de esta línea, con el mismo formato de encabezado.
-- =====================================================================================================================
