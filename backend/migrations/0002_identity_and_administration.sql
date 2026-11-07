-- Seguridad y configuración; no contiene turnos ni movimientos económicos.
CREATE TABLE app.roles (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    code text UNIQUE,
    name text NOT NULL UNIQUE CHECK (length(name) BETWEEN 1 AND 80),
    reserved boolean NOT NULL DEFAULT false,
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE app.permissions (
    key text PRIMARY KEY,
    description text NOT NULL
);

CREATE TABLE app.role_permissions (
    role_id uuid NOT NULL REFERENCES app.roles(id),
    permission_key text NOT NULL REFERENCES app.permissions(key),
    PRIMARY KEY (role_id, permission_key)
);

CREATE TABLE app.users (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    username text NOT NULL UNIQUE CHECK (username = lower(username)),
    display_name text NOT NULL CHECK (length(display_name) BETWEEN 1 AND 100),
    password_hash text,
    role_id uuid NOT NULL REFERENCES app.roles(id),
    support boolean NOT NULL DEFAULT false,
    active boolean NOT NULL DEFAULT true,
    initial_password boolean NOT NULL DEFAULT false,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    CHECK (support = (username = 'sa'))
);
CREATE INDEX users_role_idx ON app.users(role_id);

CREATE TABLE app.user_permissions (
    user_id uuid NOT NULL REFERENCES app.users(id),
    permission_key text NOT NULL REFERENCES app.permissions(key),
    effect text NOT NULL CHECK (effect IN ('allow', 'deny')),
    PRIMARY KEY (user_id, permission_key)
);

CREATE TABLE app.establishment (
    singleton boolean PRIMARY KEY DEFAULT true CHECK (singleton),
    name text NOT NULL CHECK (length(name) BETWEEN 1 AND 100),
    timezone text NOT NULL,
    currency text NOT NULL DEFAULT 'MXN' CHECK (currency = 'MXN'),
    phone text NOT NULL DEFAULT '',
    email text NOT NULL DEFAULT '',
    address text NOT NULL DEFAULT '',
    theme text NOT NULL DEFAULT 'light' CHECK (theme IN ('light', 'dark')),
    configured_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE app.registers (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    name text NOT NULL UNIQUE CHECK (length(name) BETWEEN 1 AND 80),
    description text NOT NULL DEFAULT '',
    active boolean NOT NULL DEFAULT true,
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE app.devices (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    name text NOT NULL CHECK (length(name) BETWEEN 1 AND 80),
    secret_hash text NOT NULL UNIQUE,
    register_id uuid REFERENCES app.registers(id),
    keyboard_mode text NOT NULL DEFAULT 'auto' CHECK (keyboard_mode IN ('auto', 'always', 'disabled')),
    created_at timestamptz NOT NULL DEFAULT now(),
    last_seen_at timestamptz NOT NULL DEFAULT now(),
    revoked_at timestamptz
);
CREATE INDEX devices_register_idx ON app.devices(register_id);

CREATE TABLE app.enrollments (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    code_hash text NOT NULL UNIQUE,
    created_by uuid REFERENCES app.users(id),
    created_at timestamptz NOT NULL DEFAULT now(),
    expires_at timestamptz NOT NULL,
    used_at timestamptz
);

CREATE TABLE app.sessions (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id uuid NOT NULL REFERENCES app.users(id),
    device_id uuid NOT NULL REFERENCES app.devices(id),
    token_hash text NOT NULL UNIQUE,
    created_at timestamptz NOT NULL DEFAULT now(),
    expires_at timestamptz NOT NULL,
    last_seen_at timestamptz NOT NULL DEFAULT now(),
    revoked_at timestamptz,
    revoke_reason text
);
CREATE UNIQUE INDEX sessions_one_active_user_idx ON app.sessions(user_id) WHERE revoked_at IS NULL;
CREATE INDEX sessions_device_idx ON app.sessions(device_id);

CREATE TABLE app.login_attempts (
    device_id uuid NOT NULL REFERENCES app.devices(id),
    username text NOT NULL,
    failures integer NOT NULL DEFAULT 0,
    window_start timestamptz NOT NULL DEFAULT now(),
    locked_until timestamptz,
    PRIMARY KEY (device_id, username)
);

CREATE TABLE app.user_preferences (
    user_id uuid PRIMARY KEY REFERENCES app.users(id),
    theme text NOT NULL DEFAULT 'light' CHECK (theme IN ('light', 'dark')),
    home_screen text NOT NULL DEFAULT 'status'
);

CREATE TABLE app.audit_events (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    occurred_at timestamptz NOT NULL DEFAULT now(),
    actor_id uuid REFERENCES app.users(id),
    actor_username text NOT NULL,
    actor_kind text NOT NULL,
    device_id uuid REFERENCES app.devices(id),
    action text NOT NULL,
    entity_type text NOT NULL,
    entity_id uuid,
    success boolean NOT NULL,
    before_state jsonb,
    after_state jsonb
);
CREATE INDEX audit_occurred_idx ON app.audit_events(occurred_at DESC, id);
CREATE INDEX audit_actor_idx ON app.audit_events(actor_id, occurred_at DESC);
CREATE INDEX audit_device_idx ON app.audit_events(device_id, occurred_at DESC);

COMMENT ON TABLE app.audit_events IS 'Histórico persistente; nunca almacenar contraseñas, hashes ni tokens';

-- Identidad reservada sin contraseña utilizable: solo el propietario puede aprovisionarla localmente.
INSERT INTO app.roles(code, name, reserved) VALUES ('support', 'Soporte reservado', true);
INSERT INTO app.users(username, display_name, role_id, support)
SELECT 'sa', 'Soporte', id, true FROM app.roles WHERE code = 'support';
INSERT INTO app.audit_events(actor_username, actor_kind, action, entity_type, entity_id, success)
SELECT 'local_owner', 'migration', 'support.identity_reserved', 'user', id, true
FROM app.users WHERE username = 'sa';
