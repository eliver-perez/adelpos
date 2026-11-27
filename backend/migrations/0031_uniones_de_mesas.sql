-- Etapa M1: la única migración de la etapa (uniones de mesas, mesas divisibles y ajustes pendientes; esquema 31). Cada
-- fase agrega aquí su propia sección con el encabezado «Fase X». Es transaccional (el migrador aplica todas las
-- pendientes en una sola transacción) y el servicio respalda la base antes de aplicarla. Solo agrega tablas, columnas,
-- índices y permisos: ninguna fila de ventas, pagos, cuentas, partidas, unidades, cortes, inventario ni cocina cambia.

-- =====================================================================================================================
-- Fase A · Servidor: uniones de mesas y mesas divisibles (§1 a §4 de la especificación; decisiones M1-D1 a M1-D10)
-- =====================================================================================================================

-- 1. Mesas divisibles (§3; M1-D4). Un grupo predefinido es un nombre («Mesa 3») sobre varias mesas normales del mismo
--    área (sus secciones, «3A», «3B» y «3C»), que no cambian de nombre y se abren, se cobran y se liberan por separado.
--    El atajo del grupo une todas sus secciones con el mecanismo de las uniones. Quitar un grupo es active = false: deja
--    de leerse en todas partes, su nombre vuelve a estar libre y sus secciones se borran (la fila del grupo se conserva
--    para las uniones que lo recuerdan). source = legacy son los grupos que calcula el importador del sistema anterior
--    (M1-D5): una segunda importación los vuelve a calcular; los manuales se conservan.
CREATE TABLE app.table_groups (
    id uuid PRIMARY KEY,
    area_id uuid NOT NULL REFERENCES app.restaurant_areas(id),
    name text NOT NULL CHECK (char_length(name) BETWEEN 1 AND 60),
    active boolean NOT NULL DEFAULT true,
    source text NOT NULL DEFAULT 'manual' CHECK (source IN ('manual', 'legacy')),
    legacy_batch_id uuid REFERENCES app.legacy_import_batches(id),
    legacy_key text CHECK (char_length(legacy_key) BETWEEN 1 AND 100),
    revision integer NOT NULL DEFAULT 1 CHECK (revision > 0),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    removed_at timestamptz,
    CHECK ((source = 'legacy') = (legacy_key IS NOT NULL)),
    CHECK (active = (removed_at IS NULL))
);
-- El nombre de un grupo activo no se repite en su área (sin distinguir mayúsculas). Go comprueba además que no sea el de
-- una mesa del área (TABLE_GROUP_NAME_TAKEN).
CREATE UNIQUE INDEX table_groups_active_name_idx ON app.table_groups (area_id, lower(name)) WHERE active;

-- Las secciones en su orden. Una mesa es sección de un solo grupo: las de un grupo quitado se borran.
CREATE TABLE app.table_group_sections (
    group_id uuid NOT NULL REFERENCES app.table_groups(id),
    table_id uuid NOT NULL UNIQUE REFERENCES app.restaurant_tables(id),
    position integer NOT NULL CHECK (position >= 0),
    PRIMARY KEY (group_id, table_id),
    UNIQUE (group_id, position)
);

-- 2. Uniones de mesas (§1 y §2; M1-D1, M1-D2 y M1-D10). Una unión es un conjunto de mesas del mismo área que se comporta
--    como una sola mesa. Una cuenta es de la unión porque su table_id es una mesa miembro: restaurant_accounts no
--    cambia. name es el nombre vigente que calcula Go («Mesa 5 + 6», o el del grupo si une todas sus secciones), con su
--    group_id. La unión se deshace sola cuando ya no le quedan cuentas abiertas (no_accounts) o cuando le queda una sola
--    mesa (single_table); nunca se borra.
CREATE TABLE app.table_unions (
    id uuid PRIMARY KEY,
    area_id uuid NOT NULL REFERENCES app.restaurant_areas(id),
    group_id uuid REFERENCES app.table_groups(id),
    name text NOT NULL CHECK (char_length(name) BETWEEN 1 AND 4000),
    state text NOT NULL DEFAULT 'open' CHECK (state IN ('open', 'dissolved')),
    opened_at timestamptz NOT NULL DEFAULT now(),
    opened_by uuid NOT NULL REFERENCES app.users(id),
    opened_device_id uuid NOT NULL REFERENCES app.devices(id),
    dissolved_at timestamptz,
    dissolved_by uuid REFERENCES app.users(id),
    dissolved_device_id uuid REFERENCES app.devices(id),
    dissolve_reason text CHECK (dissolve_reason IN ('no_accounts', 'single_table')),
    revision integer NOT NULL DEFAULT 1 CHECK (revision > 0),
    updated_at timestamptz NOT NULL DEFAULT now(),
    CHECK ((state = 'dissolved') = (dissolved_at IS NOT NULL)),
    CHECK ((state = 'dissolved') = (dissolve_reason IS NOT NULL))
);
CREATE INDEX table_unions_open_idx ON app.table_unions (area_id) WHERE state = 'open';

-- Las mesas de cada unión. Una mesa que sale de la unión (o la unión que se deshace) recibe left_at; una mesa puede
-- volver a la misma unión con otra fila.
CREATE TABLE app.table_union_members (
    id uuid PRIMARY KEY,
    union_id uuid NOT NULL REFERENCES app.table_unions(id),
    table_id uuid NOT NULL REFERENCES app.restaurant_tables(id),
    joined_at timestamptz NOT NULL DEFAULT now(),
    joined_by uuid NOT NULL REFERENCES app.users(id),
    left_at timestamptz,
    left_by uuid REFERENCES app.users(id),
    CHECK ((left_at IS NULL) = (left_by IS NULL))
);
-- Defensa en la base (M1-D1): una mesa nunca está en dos uniones abiertas. La primera defensa es el lock 7820148305
-- exclusivo de toda escritura del restaurante.
CREATE UNIQUE INDEX table_union_members_open_table_idx ON app.table_union_members (table_id) WHERE left_at IS NULL;
CREATE INDEX table_union_members_union_idx ON app.table_union_members (union_id, left_at);
CREATE INDEX table_union_members_table_idx ON app.table_union_members (table_id, joined_at);

-- El historial de la unión (§1.4): unir (una fila por mesa inicial), agregar, quitar y deshacer, con quién, la
-- terminal, la operación y el nombre de la unión después del cambio (union_name: los reportes de cancelaciones leen el
-- nombre que tenía la unión en ese momento). table_name es el nombre de la mesa en ese momento.
CREATE TABLE app.table_union_events (
    id uuid PRIMARY KEY,
    sequence bigint GENERATED ALWAYS AS IDENTITY UNIQUE,
    union_id uuid NOT NULL REFERENCES app.table_unions(id),
    kind text NOT NULL CHECK (kind IN ('joined', 'table_added', 'table_removed', 'dissolved')),
    table_id uuid REFERENCES app.restaurant_tables(id),
    table_name text,
    union_name text NOT NULL,
    operation_id uuid,
    author_id uuid NOT NULL REFERENCES app.users(id),
    device_id uuid NOT NULL REFERENCES app.devices(id),
    created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX table_union_events_union_idx ON app.table_union_events (union_id, sequence);

-- 3. La venta queda registrada en la unión (§1.4; M1-D3): la instantánea de la unión vigente de la mesa de la cuenta al
--    reconocer la venta, como table_name. Las filas existentes quedan en NULL y no se reescribe ninguna (la tabla solo
--    admite INSERT). table_union_tables es [{id, name}] de cada mesa de la unión, en el orden de su nombre.
ALTER TABLE app.sales_recognitions
    ADD COLUMN table_union_id uuid,
    ADD COLUMN table_union_name text,
    ADD COLUMN table_union_tables jsonb;

-- 4. Permiso «Unir mesas» (§1.3; M1-D8). Lo piden unir, agregar y quitar mesas y la unión que nace con el primer
--    producto. Se concede a los roles existentes Administrador, Mesero, Cajero y Supervisor que ya tengan lo que el
--    permiso necesita (abrir cuentas y ver las mesas), como la 0025 con waiter_devices.use: solo agrega, nunca quita
--    nada. En una instalación nueva los roles nacen en el asistente con sus conjuntos de fábrica (permission_catalog.go),
--    que ya lo incluyen. Se registra también en Go (operations.Permissions) con la misma descripción.
INSERT INTO app.permissions(key, description) VALUES
    ('restaurant.tables.join', 'Unir mesas');
INSERT INTO app.role_permissions(role_id, permission_key)
SELECT r.id, 'restaurant.tables.join'
FROM app.roles r
WHERE r.code IN ('administrator', 'waiter', 'cashier', 'supervisor')
    AND EXISTS (SELECT FROM app.role_permissions p WHERE p.role_id = r.id AND p.permission_key = 'accounts.open')
    AND EXISTS (SELECT FROM app.role_permissions p WHERE p.role_id = r.id AND p.permission_key = 'restaurant.tables.read')
ON CONFLICT DO NOTHING;

COMMENT ON TABLE app.table_groups IS 'Mesas divisibles: un nombre sobre varias mesas del mismo área (etapa M1, Fase A)';
COMMENT ON TABLE app.table_unions IS 'Uniones de mesas: varias mesas que se comportan como una sola (etapa M1, Fase A)';
COMMENT ON TABLE app.table_union_members IS 'Mesas de cada unión; left_at al salir o al deshacerse (etapa M1, Fase A)';
COMMENT ON TABLE app.table_union_events IS 'Historial de cada unión: unir, agregar, quitar y deshacer (etapa M1, Fase A)';
