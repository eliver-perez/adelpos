-- Cocina amplía las unidades comerciales existentes; nunca duplica ventas ni inventario.
CREATE TABLE app.kitchen_settings (
    singleton boolean PRIMARY KEY DEFAULT true CHECK (singleton),
    grouping text NOT NULL DEFAULT 'independent' CHECK (grouping IN ('independent','unified')),
    tracking text NOT NULL DEFAULT 'item' CHECK (tracking IN ('item','order')),
    automatic_release boolean NOT NULL DEFAULT false,
    lead_minutes integer NOT NULL DEFAULT 25 CHECK (lead_minutes BETWEEN 0 AND 1440),
    show_prices boolean NOT NULL DEFAULT false,
    show_notes boolean NOT NULL DEFAULT true,
    show_establishment boolean NOT NULL DEFAULT true,
    text_size text NOT NULL DEFAULT 'normal' CHECK (text_size IN ('normal','large')),
    line_spacing integer NOT NULL DEFAULT 1 CHECK (line_spacing BETWEEN 1 AND 3),
    quantity_style text NOT NULL DEFAULT 'prefix' CHECK (quantity_style IN ('prefix','multiplier')),
    revision integer NOT NULL DEFAULT 1 CHECK (revision > 0)
);
INSERT INTO app.kitchen_settings(singleton) VALUES (true);

CREATE TABLE app.kitchen_printers (
    id uuid PRIMARY KEY,
    name text NOT NULL UNIQUE CHECK (length(name) BETWEEN 1 AND 100),
    transport text NOT NULL CHECK (transport IN ('cups','escpos_tcp','agent')),
    destination text NOT NULL CHECK (length(destination) BETWEEN 1 AND 200),
    port integer NOT NULL DEFAULT 9100 CHECK (port BETWEEN 9100 AND 9109),
    paper_width integer NOT NULL DEFAULT 80 CHECK (paper_width IN (58,80)),
    copies integer NOT NULL DEFAULT 1 CHECK (copies BETWEEN 1 AND 3),
    supports_cut boolean NOT NULL DEFAULT false,
    cut boolean NOT NULL DEFAULT false,
    cutter_option text NOT NULL DEFAULT '',
    active boolean NOT NULL DEFAULT true,
    fallback_id uuid REFERENCES app.kitchen_printers(id),
    revision integer NOT NULL DEFAULT 1 CHECK (revision > 0),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    CHECK (NOT cut OR supports_cut),
    CHECK (fallback_id IS NULL OR fallback_id <> id)
);

CREATE TABLE app.kitchen_stations (
    id uuid PRIMARY KEY,
    name text NOT NULL UNIQUE CHECK (length(name) BETWEEN 1 AND 100),
    description text NOT NULL DEFAULT '' CHECK (length(description) <= 2000),
    active boolean NOT NULL DEFAULT true,
    sort_order integer NOT NULL DEFAULT 0,
    reception text NOT NULL DEFAULT 'printer' CHECK (reception IN ('printer','kds','both')),
    printer_id uuid REFERENCES app.kitchen_printers(id),
    target_minutes integer NOT NULL DEFAULT 15 CHECK (target_minutes BETWEEN 1 AND 1440),
    warning_minutes integer NOT NULL DEFAULT 15 CHECK (warning_minutes BETWEEN 1 AND 1440),
    overdue_minutes integer NOT NULL DEFAULT 25 CHECK (overdue_minutes BETWEEN warning_minutes AND 2880),
    grouping text NOT NULL DEFAULT 'inherit' CHECK (grouping IN ('inherit','independent','unified')),
    tracking text NOT NULL DEFAULT 'inherit' CHECK (tracking IN ('inherit','item','order')),
    revision integer NOT NULL DEFAULT 1 CHECK (revision > 0),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    CHECK (reception='kds' OR printer_id IS NOT NULL)
);

CREATE TABLE app.kitchen_routes (
    id uuid PRIMARY KEY,
    product_id uuid UNIQUE REFERENCES app.catalog_products(id),
    category_id uuid UNIQUE REFERENCES app.catalog_categories(id),
    mode text NOT NULL CHECK (mode IN ('prepare','none')),
    revision integer NOT NULL DEFAULT 1 CHECK (revision > 0),
    updated_at timestamptz NOT NULL DEFAULT now(),
    CHECK ((product_id IS NULL) <> (category_id IS NULL))
);
CREATE TABLE app.kitchen_route_stations (
    route_id uuid NOT NULL REFERENCES app.kitchen_routes(id),
    station_id uuid NOT NULL REFERENCES app.kitchen_stations(id),
    PRIMARY KEY (route_id,station_id)
);

-- Un dispositivo KDS futuro solo puede operar las estaciones asignadas expresamente.
CREATE TABLE app.kitchen_device_stations (
    device_id uuid NOT NULL REFERENCES app.devices(id),
    station_id uuid NOT NULL REFERENCES app.kitchen_stations(id),
    PRIMARY KEY (device_id,station_id)
);

CREATE TABLE app.kitchen_event_receipts (
    event_id uuid PRIMARY KEY REFERENCES app.restaurant_events(id),
    processed_at timestamptz NOT NULL DEFAULT now(),
    result text NOT NULL
);

CREATE TABLE app.kitchen_releases (
    id uuid PRIMARY KEY,
    source_event_id uuid NOT NULL UNIQUE REFERENCES app.restaurant_events(id),
    order_id uuid NOT NULL REFERENCES app.restaurant_orders(id),
    account_id uuid NOT NULL REFERENCES app.restaurant_accounts(id),
    unit_ids uuid[] NOT NULL,
    state text NOT NULL DEFAULT 'pending' CHECK (state IN ('pending','scheduled','blocked','released','cancelled')),
    automatic boolean NOT NULL DEFAULT false,
    lead_minutes integer NOT NULL DEFAULT 25 CHECK (lead_minutes BETWEEN 0 AND 1440),
    scheduled_at timestamptz,
    due_at timestamptz,
    warnings jsonb NOT NULL DEFAULT '[]',
    routing_snapshot jsonb NOT NULL DEFAULT '{}',
    station_snapshot jsonb NOT NULL DEFAULT '{}',
    context_snapshot jsonb NOT NULL DEFAULT '{}',
    released_at timestamptz,
    revision integer NOT NULL DEFAULT 1 CHECK (revision > 0),
    created_at timestamptz NOT NULL DEFAULT now(),
    CHECK ((state='released') = (released_at IS NOT NULL))
);
CREATE INDEX kitchen_releases_due_idx ON app.kitchen_releases(due_at) WHERE state='scheduled' AND automatic;
CREATE INDEX kitchen_releases_account_idx ON app.kitchen_releases(account_id,created_at);

-- Una configuración inválida no puede hacer perder una modificación comercial confirmada.
CREATE TABLE app.kitchen_pending_changes (
    event_id uuid PRIMARY KEY REFERENCES app.restaurant_events(id),
    account_id uuid NOT NULL REFERENCES app.restaurant_accounts(id),
    kind text NOT NULL,
    payload jsonb NOT NULL,
    warnings jsonb NOT NULL,
    revision integer NOT NULL DEFAULT 1,
    created_at timestamptz NOT NULL DEFAULT now()
);

-- Punto inequívoco de consumo teórico futuro: una sola liberación inicial por unidad.
CREATE TABLE app.kitchen_unit_releases (
    unit_id uuid PRIMARY KEY REFERENCES app.restaurant_item_units(id),
    release_id uuid NOT NULL REFERENCES app.kitchen_releases(id),
    version_id uuid NOT NULL REFERENCES app.restaurant_item_versions(id),
    released_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE app.kitchen_tasks (
    id uuid PRIMARY KEY,
    unit_id uuid NOT NULL REFERENCES app.restaurant_item_units(id),
    release_id uuid NOT NULL REFERENCES app.kitchen_releases(id),
    station_id uuid NOT NULL REFERENCES app.kitchen_stations(id),
    version_id uuid NOT NULL REFERENCES app.restaurant_item_versions(id),
    state text NOT NULL DEFAULT 'released' CHECK (state IN ('released','preparing','ready','cancelled')),
    instructions jsonb NOT NULL,
    revision integer NOT NULL DEFAULT 1 CHECK (revision > 0),
    released_at timestamptz NOT NULL DEFAULT now(),
    started_at timestamptz,
    ready_at timestamptz,
    updated_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (unit_id,station_id)
);
CREATE INDEX kitchen_tasks_station_idx ON app.kitchen_tasks(station_id,state,released_at);

CREATE TABLE app.kitchen_tickets (
    id uuid PRIMARY KEY,
    source_event_id uuid REFERENCES app.restaurant_events(id),
    release_id uuid REFERENCES app.kitchen_releases(id),
    kind text NOT NULL CHECK (kind IN ('normal','modification','cancellation','test')),
    station_ids uuid[] NOT NULL,
    document jsonb NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX kitchen_tickets_event_idx ON app.kitchen_tickets(source_event_id);

CREATE TABLE app.kitchen_kds_receipts (
    ticket_id uuid NOT NULL REFERENCES app.kitchen_tickets(id),
    station_id uuid NOT NULL REFERENCES app.kitchen_stations(id),
    device_id uuid NOT NULL REFERENCES app.devices(id),
    user_id uuid NOT NULL REFERENCES app.users(id),
    acknowledged_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (ticket_id,station_id,device_id)
);

CREATE TABLE app.kitchen_print_jobs (
    id uuid PRIMARY KEY,
    ticket_id uuid NOT NULL REFERENCES app.kitchen_tickets(id),
    printer_id uuid NOT NULL REFERENCES app.kitchen_printers(id),
    operation_id uuid REFERENCES app.restaurant_operations(id) DEFERRABLE INITIALLY DEFERRED,
    original_job_id uuid REFERENCES app.kitchen_print_jobs(id),
    printer_snapshot jsonb NOT NULL,
    content text NOT NULL,
    state text NOT NULL DEFAULT 'pending' CHECK (state IN ('pending','processing','submitted','error','uncertain','cancelled','intervention')),
    attempts integer NOT NULL DEFAULT 0,
    last_attempt_at timestamptz,
    next_attempt_at timestamptz NOT NULL DEFAULT now(),
    lease_until timestamptz,
    transport_reference text NOT NULL DEFAULT '',
    last_error text NOT NULL DEFAULT '',
    result jsonb NOT NULL DEFAULT '{}',
    revision integer NOT NULL DEFAULT 1 CHECK (revision > 0),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX kitchen_print_original_destination_idx ON app.kitchen_print_jobs(ticket_id,printer_id) WHERE original_job_id IS NULL;
CREATE INDEX kitchen_print_pending_idx ON app.kitchen_print_jobs(printer_id,next_attempt_at) WHERE state IN ('pending','processing','error');

CREATE TABLE app.kitchen_print_attempts (
    id uuid PRIMARY KEY,
    job_id uuid NOT NULL REFERENCES app.kitchen_print_jobs(id),
    attempt integer NOT NULL,
    started_at timestamptz NOT NULL DEFAULT now(),
    finished_at timestamptz,
    outcome text NOT NULL DEFAULT 'processing',
    detail text NOT NULL DEFAULT '',
    result jsonb NOT NULL DEFAULT '{}',
    UNIQUE(job_id,attempt)
);

CREATE TABLE app.kitchen_history (
    id uuid PRIMARY KEY,
    operation_id uuid REFERENCES app.restaurant_operations(id) DEFERRABLE INITIALLY DEFERRED,
    actor_id uuid REFERENCES app.users(id),
    device_id uuid REFERENCES app.devices(id),
    entity_id uuid,
    kind text NOT NULL,
    before_value jsonb,
    after_value jsonb,
    created_at timestamptz NOT NULL DEFAULT now()
);

-- El lock de inserción del publicador preserva el orden de commit de estos cursores.
CREATE TABLE app.realtime_events (
    sequence bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    id uuid NOT NULL UNIQUE,
    version integer NOT NULL DEFAULT 1 CHECK (version=1),
    kind text NOT NULL,
    scope text NOT NULL CHECK (scope IN ('restaurant','takeaway','kitchen','printing','configuration')),
    station_id uuid REFERENCES app.kitchen_stations(id),
    aggregate_id uuid,
    payload jsonb NOT NULL DEFAULT '{}',
    created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX realtime_events_scope_idx ON app.realtime_events(scope,station_id,sequence);

-- Los pedidos anteriores se conservan pendientes de revisión; migrar nunca imprime.
INSERT INTO app.kitchen_releases(id,source_event_id,order_id,account_id,unit_ids,state,scheduled_at,due_at,warnings)
SELECT e.id,e.id,o.id,o.original_account_id,
    ARRAY(SELECT u.id FROM app.restaurant_item_units u JOIN app.restaurant_items i ON i.id=u.item_id
          WHERE i.original_order_id=o.id AND u.state='active'),
    CASE WHEN a.scheduled_at IS NULL THEN 'pending' ELSE 'scheduled' END,
    a.scheduled_at,a.scheduled_at,
    '["Pedido anterior a etapa 5: revisar y liberar expresamente."]'::jsonb
FROM app.restaurant_orders o
JOIN app.restaurant_events e ON e.kind='order.confirmed' AND e.operation_id=o.operation_id
JOIN app.restaurant_accounts a ON a.id=o.original_account_id
WHERE o.kind='confirmation';
INSERT INTO app.kitchen_event_receipts(event_id,result)
SELECT id,'legacy_review_pending' FROM app.restaurant_events;

INSERT INTO app.permissions(key,description) VALUES
    ('kitchen.stations.read','Consultar configuración de estaciones'),
    ('kitchen.stations.manage','Administrar estaciones y ámbitos de terminal'),
    ('kitchen.stations.all','Operar todas las estaciones autorizadas del establecimiento'),
    ('kitchen.routing.manage','Administrar rutas de productos y categorías'),
    ('kitchen.printers.manage','Administrar impresoras y destinos'),
    ('kitchen.printers.test','Solicitar impresión de prueba'),
    ('kitchen.formats.manage','Administrar formato y agrupación de comandas'),
    ('kitchen.timing.manage','Administrar tiempos y programación'),
    ('kitchen.release','Liberar pedidos confirmados a preparación'),
    ('kitchen.release_early','Liberar anticipadamente pedidos programados'),
    ('kitchen.tickets.read','Consultar comandas y pendientes por estación'),
    ('kitchen.states.read','Consultar estados de preparación'),
    ('kitchen.states.manage','Modificar estados de preparación'),
    ('kitchen.queue.read','Consultar cola e intentos de impresión'),
    ('kitchen.queue.resolve','Resolver errores y resultados inciertos'),
    ('kitchen.queue.retry','Reintentar errores confirmados de impresión'),
    ('kitchen.reprint','Solicitar reimpresión autorizada'),
    ('kitchen.history.read','Consultar historial de cocina'),
    ('kitchen.kds.ack','Confirmar recepción KDS por estación'),
    ('events.read','Recibir eventos autenticados del establecimiento');
INSERT INTO app.role_permissions(role_id,permission_key)
SELECT r.id,p.key FROM app.roles r CROSS JOIN app.permissions p
WHERE r.code='administrator' AND (p.key LIKE 'kitchen.%' OR p.key='events.read')
ON CONFLICT DO NOTHING;
