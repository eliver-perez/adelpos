-- Operación persistente del restaurante. No registra pagos, envíos a cocina ni inventario.
CREATE TABLE app.restaurant_areas (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    name text NOT NULL UNIQUE CHECK (length(name) BETWEEN 1 AND 100),
    description text NOT NULL DEFAULT '',
    sort_order integer NOT NULL DEFAULT 0,
    active boolean NOT NULL DEFAULT true,
    revision integer NOT NULL DEFAULT 1 CHECK (revision > 0),
    updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE app.restaurant_tables (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    area_id uuid NOT NULL REFERENCES app.restaurant_areas(id),
    name text NOT NULL CHECK (length(name) BETWEEN 1 AND 60),
    shape text NOT NULL DEFAULT 'rectangle' CHECK (shape IN ('rectangle', 'round')),
    x integer NOT NULL DEFAULT 20 CHECK (x BETWEEN 0 AND 936),
    y integer NOT NULL DEFAULT 20 CHECK (y BETWEEN 0 AND 504),
    width integer NOT NULL DEFAULT 100 CHECK (width BETWEEN 64 AND 240),
    height integer NOT NULL DEFAULT 72 CHECK (height BETWEEN 56 AND 180),
    seats integer NOT NULL DEFAULT 4 CHECK (seats BETWEEN 1 AND 40),
    active boolean NOT NULL DEFAULT true,
    out_of_service boolean NOT NULL DEFAULT false,
    revision integer NOT NULL DEFAULT 1 CHECK (revision > 0),
    updated_at timestamptz NOT NULL DEFAULT now(),
    CHECK (x + width <= 1000 AND y + height <= 560),
    UNIQUE (area_id, name)
);
CREATE INDEX restaurant_tables_area_idx ON app.restaurant_tables(area_id);

CREATE TABLE app.restaurant_settings (
    singleton boolean PRIMARY KEY DEFAULT true CHECK (singleton),
    warning_minutes integer NOT NULL DEFAULT 15 CHECK (warning_minutes BETWEEN 1 AND 240),
    overdue_minutes integer NOT NULL DEFAULT 30 CHECK (overdue_minutes BETWEEN warning_minutes AND 1440),
    revision integer NOT NULL DEFAULT 1 CHECK (revision > 0)
);
INSERT INTO app.restaurant_settings(singleton) VALUES (true);

CREATE TABLE app.restaurant_accounts (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    folio bigint GENERATED ALWAYS AS IDENTITY UNIQUE,
    mode text NOT NULL CHECK (mode IN ('dine_in', 'takeaway')),
    name text NOT NULL CHECK (length(name) BETWEEN 1 AND 100),
    table_id uuid REFERENCES app.restaurant_tables(id),
    price_list_id uuid NOT NULL REFERENCES app.catalog_price_lists(id),
    customer_name text NOT NULL DEFAULT '',
    phone text NOT NULL DEFAULT '',
    origin text NOT NULL DEFAULT 'Mostrador',
    notes text NOT NULL DEFAULT '',
    scheduled_at timestamptz,
    schedule_kind text CHECK (schedule_kind IN ('preparation', 'delivery')),
    state text NOT NULL DEFAULT 'open' CHECK (state IN ('open', 'merged', 'voided')),
    merged_into_id uuid REFERENCES app.restaurant_accounts(id),
    opened_by uuid NOT NULL REFERENCES app.users(id),
    assigned_user_id uuid NOT NULL REFERENCES app.users(id),
    device_id uuid NOT NULL REFERENCES app.devices(id),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    revision integer NOT NULL DEFAULT 1 CHECK (revision > 0),
    CHECK (mode <> 'takeaway' OR table_id IS NULL),
    CHECK ((scheduled_at IS NULL) = (schedule_kind IS NULL)),
    CHECK (state <> 'merged' OR merged_into_id IS NOT NULL),
    CHECK (merged_into_id IS NULL OR merged_into_id <> id)
);
CREATE INDEX restaurant_accounts_table_idx ON app.restaurant_accounts(table_id) WHERE state='open';
CREATE INDEX restaurant_accounts_board_idx ON app.restaurant_accounts(mode, created_at, id) WHERE state='open';

CREATE TABLE app.restaurant_draft_items (
    id uuid PRIMARY KEY,
    account_id uuid NOT NULL REFERENCES app.restaurant_accounts(id),
    product_id uuid NOT NULL REFERENCES app.catalog_products(id),
    variant_id uuid,
    quantity integer NOT NULL CHECK (quantity BETWEEN 1 AND 1000),
    customization jsonb NOT NULL,
    position integer NOT NULL,
    author_id uuid NOT NULL REFERENCES app.users(id),
    device_id uuid NOT NULL REFERENCES app.devices(id),
    updated_at timestamptz NOT NULL DEFAULT now(),
    FOREIGN KEY (variant_id, product_id) REFERENCES app.catalog_variants(id, product_id)
);
CREATE INDEX restaurant_drafts_account_idx ON app.restaurant_draft_items(account_id);

CREATE TABLE app.restaurant_operations (
    id uuid PRIMARY KEY,
    actor_id uuid NOT NULL REFERENCES app.users(id),
    device_id uuid NOT NULL REFERENCES app.devices(id),
    route text NOT NULL,
    fingerprint text NOT NULL,
    response jsonb NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE app.restaurant_orders (
    id uuid PRIMARY KEY,
    original_account_id uuid NOT NULL REFERENCES app.restaurant_accounts(id),
    operation_id uuid NOT NULL UNIQUE REFERENCES app.restaurant_operations(id) DEFERRABLE INITIALLY DEFERRED,
    kind text NOT NULL CHECK (kind IN ('confirmation', 'amendment')),
    author_id uuid NOT NULL REFERENCES app.users(id),
    device_id uuid NOT NULL REFERENCES app.devices(id),
    created_at timestamptz NOT NULL DEFAULT now(),
    price_list_id uuid NOT NULL REFERENCES app.catalog_price_lists(id),
    pricing_snapshot jsonb NOT NULL,
    quote jsonb NOT NULL
);

CREATE TABLE app.restaurant_items (
    id uuid PRIMARY KEY,
    original_order_id uuid NOT NULL REFERENCES app.restaurant_orders(id),
    product_id uuid NOT NULL REFERENCES app.catalog_products(id),
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE app.restaurant_item_versions (
    id uuid PRIMARY KEY,
    item_id uuid NOT NULL REFERENCES app.restaurant_items(id),
    order_id uuid NOT NULL REFERENCES app.restaurant_orders(id),
    version integer NOT NULL CHECK (version > 0),
    request jsonb NOT NULL,
    quote_line jsonb NOT NULL,
    author_id uuid NOT NULL REFERENCES app.users(id),
    device_id uuid NOT NULL REFERENCES app.devices(id),
    created_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (item_id, version),
    UNIQUE (id, item_id)
);

-- Cada unidad conserva identidad y centavos propios al dividir, fusionar o transferir cuentas.
CREATE TABLE app.restaurant_item_units (
    id uuid PRIMARY KEY,
    item_id uuid NOT NULL REFERENCES app.restaurant_items(id),
    version_id uuid NOT NULL,
    ordinal integer NOT NULL CHECK (ordinal > 0),
    account_id uuid NOT NULL REFERENCES app.restaurant_accounts(id),
    state text NOT NULL DEFAULT 'active' CHECK (state IN ('active', 'cancelled')),
    subtotal numeric(18,2) NOT NULL CHECK (subtotal >= 0),
    discount numeric(18,2) NOT NULL CHECK (discount BETWEEN 0 AND subtotal),
    total numeric(18,2) NOT NULL CHECK (total = subtotal - discount),
    created_at timestamptz NOT NULL DEFAULT now(),
    cancelled_at timestamptz,
    FOREIGN KEY (version_id, item_id) REFERENCES app.restaurant_item_versions(id, item_id),
    UNIQUE (item_id, ordinal),
    CHECK ((state = 'cancelled') = (cancelled_at IS NOT NULL))
);
CREATE INDEX restaurant_units_account_idx ON app.restaurant_item_units(account_id, state, version_id);

-- Diario durable para la etapa 5: aún no existe consumidor ni entrega confirmada a cocina.
CREATE TABLE app.restaurant_events (
    id uuid PRIMARY KEY,
    sequence bigint GENERATED ALWAYS AS IDENTITY UNIQUE,
    account_id uuid NOT NULL REFERENCES app.restaurant_accounts(id),
    related_account_id uuid REFERENCES app.restaurant_accounts(id),
    operation_id uuid NOT NULL REFERENCES app.restaurant_operations(id) DEFERRABLE INITIALLY DEFERRED,
    kind text NOT NULL,
    payload jsonb NOT NULL,
    author_id uuid NOT NULL REFERENCES app.users(id),
    device_id uuid NOT NULL REFERENCES app.devices(id),
    created_at timestamptz NOT NULL DEFAULT now(),
    integration_state text NOT NULL DEFAULT 'pending_integration' CHECK (integration_state='pending_integration'),
    UNIQUE (operation_id, account_id, kind)
);
CREATE INDEX restaurant_events_account_idx ON app.restaurant_events(account_id, sequence);
CREATE INDEX restaurant_events_related_idx ON app.restaurant_events(related_account_id, sequence);

INSERT INTO app.permissions(key,description) VALUES
    ('restaurant.areas.read', 'Consultar áreas del restaurante'),
    ('restaurant.areas.manage', 'Administrar áreas del restaurante'),
    ('restaurant.tables.read', 'Consultar mesas y ocupación real'),
    ('restaurant.tables.manage', 'Administrar mesas y distribución'),
    ('restaurant.settings.manage', 'Configurar umbrales del tablero para llevar'),
    ('accounts.read', 'Consultar cuentas e importes'),
    ('accounts.open', 'Abrir cuentas'),
    ('accounts.manage', 'Editar referencias y anular cuentas sin partidas activas'),
    ('accounts.split', 'Dividir cuentas'),
    ('accounts.merge', 'Combinar cuentas compatibles'),
    ('accounts.transfer', 'Transferir la responsabilidad de cuentas'),
    ('accounts.move', 'Mover cuentas entre mesas'),
    ('accounts.transfer_items', 'Transferir partidas y cantidades entre cuentas'),
    ('orders.capture', 'Capturar y editar borradores de pedidos'),
    ('orders.confirm', 'Confirmar pedidos'),
    ('orders.edit', 'Modificar partidas confirmadas sin aprobación adicional'),
    ('orders.cancel', 'Cancelar partidas o cantidades confirmadas'),
    ('orders.price_list.manage', 'Seleccionar listas de precios para nuevas capturas'),
    ('orders.history.read', 'Consultar historial de pedidos y cuentas'),
    ('takeaway.read', 'Consultar tablero de pedidos para llevar y ventanilla'),
    ('takeaway.manage', 'Administrar datos de pedidos para llevar');

-- Solo permisos nuevos al administrador: no se restablecen roles ni credenciales existentes.
INSERT INTO app.role_permissions(role_id,permission_key)
SELECT r.id,p.key FROM app.roles r CROSS JOIN app.permissions p
WHERE r.code='administrator' AND (
    p.key LIKE 'restaurant.%' OR p.key LIKE 'accounts.%' OR p.key LIKE 'orders.%' OR p.key LIKE 'takeaway.%'
);
