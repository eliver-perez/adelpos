-- Etapa 8, fase A: núcleo de inventario. Función activable «Control de inventario», configuración,
-- unidades, categorías, insumos, presentaciones, almacenes, asignación de estaciones a almacenes,
-- existencias, valuación por insumo y libro de movimientos, con los permisos de la fase A.
-- Transaccional: el migrador la aplica dentro de la misma transacción que la registra en
-- schema_migrations y no contiene sentencias que no admitan transacción (ni CREATE INDEX
-- CONCURRENTLY ni VACUUM), de modo que un fallo la revierte completa. No modifica 0001–0009 ni
-- filas existentes; solo siembra datos de referencia (función desactivada, configuración, las seis
-- unidades del sistema y «Almacén general»), nunca datos ficticios. Las columnas opcionales llevan
-- CHECK que exigen IS NOT NULL en la rama que valida, porque un CHECK que evalúa a NULL se acepta.
--
-- Precisión (decisión de diseño de la etapa 8): cantidades numeric(20,6) en la unidad base del
-- insumo, valores numeric(20,6) y costos unitarios numeric(24,10). Go calcula con shopspring/decimal
-- y viaja como cadena en JSON; el redondeo a centavos es solo para mostrar e imprimir.

-- 1. Función activable. enabled_at guarda la fecha y hora en que la función pasó de desactivada a
-- activada (PUT settings/features); desactivarla la conserva. Las filas existentes quedan en NULL:
-- nunca se activaron después de esta migración. El inventario solo consume liberaciones a cocina
-- posteriores a enabled_at (fase B); la administración se puede preparar con la función desactivada.
ALTER TABLE app.establishment_features ADD COLUMN enabled_at timestamptz;
INSERT INTO app.establishment_features(code, enabled) VALUES ('inventory_control', false) ON CONFLICT DO NOTHING;

-- 2. Eventos en tiempo real con ámbito propio de inventario. Se recrea el CHECK con el mismo nombre
-- y se conservan los seis ámbitos anteriores (0008). Los eventos solo llevan referencias: ningún
-- costo viaja por SSE.
ALTER TABLE app.realtime_events DROP CONSTRAINT realtime_events_scope_check;
ALTER TABLE app.realtime_events
    ADD CONSTRAINT realtime_events_scope_check CHECK (
        scope IN ('restaurant', 'takeaway', 'kitchen', 'printing', 'configuration', 'cash', 'inventory')
    );

-- 3. Parámetros de inventario (fila única con revisión). La fase A crea todas las columnas que usan
-- las fases B–D para no alterar la tabla después:
--   - purchase_tax_in_cost: «El IVA de las compras forma parte del costo» (activado por defecto);
--     cada recepción lo copia al registrarse, por lo que cambiarlo solo afecta recepciones futuras;
--   - operating_day_cutoff: hora de corte del día operativo en la zona horaria del establecimiento
--     (cancelaciones sin clasificar, fase B);
--   - warn_products_without_recipe: alerta de productos vendibles sin receta (fase D);
--   - count_frequency y count_interval_days: frecuencia sugerida de conteos (solo genera la alerta
--     «conteo pendiente», fase D); el intervalo solo existe, y es obligatorio, con «custom».
CREATE TABLE app.inventory_settings (
    singleton boolean PRIMARY KEY DEFAULT true CONSTRAINT inventory_settings_singleton_check CHECK (singleton),
    purchase_tax_in_cost boolean NOT NULL DEFAULT true,
    operating_day_cutoff time NOT NULL DEFAULT '05:00',
    warn_products_without_recipe boolean NOT NULL DEFAULT true,
    count_frequency text NOT NULL DEFAULT 'none' CONSTRAINT inventory_settings_count_frequency_check
        CHECK (count_frequency IN ('none', 'daily', 'weekly', 'monthly', 'custom')),
    count_interval_days integer,
    revision integer NOT NULL DEFAULT 1 CONSTRAINT inventory_settings_revision_check CHECK (revision > 0),
    updated_by uuid CONSTRAINT inventory_settings_updated_by_fkey REFERENCES app.users(id),
    updated_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT inventory_settings_count_interval_check CHECK (
        (count_frequency = 'custom' AND count_interval_days IS NOT NULL AND count_interval_days BETWEEN 1 AND 365)
        OR (count_frequency <> 'custom' AND count_interval_days IS NULL)
    )
);
INSERT INTO app.inventory_settings(singleton) VALUES (true);

-- 4. Unidades de medida. factor es la equivalencia con la unidad base de su dimensión (g, ml,
-- pieza y porción valen 1). Solo se convierte dentro de la misma dimensión. Paquete y caja no son
-- unidades: son presentaciones de cada insumo (sección 7). Las seis unidades del sistema son de
-- referencia, inmutables; se pueden agregar unidades propias de una dimensión (por ejemplo, onza
-- de masa) con inventory.items.manage. El factor de una unidad usada no cambia (Go lo valida).
CREATE TABLE app.inventory_units (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    code text NOT NULL CONSTRAINT inventory_units_code_check CHECK (code ~ '^[a-z][a-z0-9_]{0,15}$'),
    name text NOT NULL CONSTRAINT inventory_units_name_check CHECK (length(name) BETWEEN 1 AND 40 AND name = btrim(name)),
    dimension text NOT NULL CONSTRAINT inventory_units_dimension_check
        CHECK (dimension IN ('mass', 'volume', 'count', 'portion')),
    factor numeric(24,10) NOT NULL CONSTRAINT inventory_units_factor_check CHECK (factor > 0 AND factor <= 1000000),
    system boolean NOT NULL DEFAULT false,
    active boolean NOT NULL DEFAULT true,
    revision integer NOT NULL DEFAULT 1 CONSTRAINT inventory_units_revision_check CHECK (revision > 0),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT inventory_units_code_key UNIQUE (code),
    -- Una unidad del sistema siempre está activa.
    CONSTRAINT inventory_units_system_active_check CHECK (NOT system OR active)
);
CREATE UNIQUE INDEX inventory_units_name_key ON app.inventory_units (lower(name));
INSERT INTO app.inventory_units(code, name, dimension, factor, system) VALUES
    ('g', 'Gramo', 'mass', 1, true),
    ('kg', 'Kilogramo', 'mass', 1000, true),
    ('ml', 'Mililitro', 'volume', 1, true),
    ('l', 'Litro', 'volume', 1000, true),
    ('pza', 'Pieza', 'count', 1, true),
    ('porcion', 'Porción', 'portion', 1, true);

-- 5. Categorías de insumos (opcionales), con orden visual por posiciones (internal/ordering) y
-- nombre único sin distinguir mayúsculas. Se desactivan, nunca se borran.
CREATE TABLE app.inventory_categories (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    name text NOT NULL CONSTRAINT inventory_categories_name_check
        CHECK (length(name) BETWEEN 1 AND 60 AND name = btrim(name)),
    active boolean NOT NULL DEFAULT true,
    sort_order integer NOT NULL DEFAULT 0 CONSTRAINT inventory_categories_sort_order_check CHECK (sort_order >= 0),
    revision integer NOT NULL DEFAULT 1 CONSTRAINT inventory_categories_revision_check CHECK (revision > 0),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX inventory_categories_name_key ON app.inventory_categories (lower(name));

-- 6. Insumos inventariables. Son independientes de los productos de venta y de los ingredientes
-- del catálogo (las recetas de la fase B los relacionan). Código y nombre únicos sin distinguir
-- mayúsculas; código de barras opcional y único (Go además impide repetirlo en una presentación).
-- base_unit_id no cambia si el insumo tiene movimientos (Go lo valida). kind 'prepared' es una
-- subreceta almacenable (fase B). reference_cost es por unidad base y es confidencial
-- (inventory.costs.read / inventory.costs.manage); NULL significa sin costo de referencia.
-- min_stock y reorder_point se expresan en la unidad base; NULL significa sin alerta.
CREATE TABLE app.inventory_items (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    code text NOT NULL CONSTRAINT inventory_items_code_check CHECK (length(code) BETWEEN 1 AND 40 AND code = btrim(code)),
    barcode text CONSTRAINT inventory_items_barcode_check CHECK (barcode IS NULL OR barcode ~ '^[!-~]{1,64}$'),
    name text NOT NULL CONSTRAINT inventory_items_name_check CHECK (length(name) BETWEEN 1 AND 100 AND name = btrim(name)),
    description text NOT NULL DEFAULT '' CONSTRAINT inventory_items_description_check CHECK (length(description) <= 2000),
    category_id uuid CONSTRAINT inventory_items_category_id_fkey REFERENCES app.inventory_categories(id),
    base_unit_id uuid NOT NULL CONSTRAINT inventory_items_base_unit_id_fkey REFERENCES app.inventory_units(id),
    kind text NOT NULL DEFAULT 'purchased' CONSTRAINT inventory_items_kind_check CHECK (kind IN ('purchased', 'prepared')),
    active boolean NOT NULL DEFAULT true,
    reference_cost numeric(24,10) CONSTRAINT inventory_items_reference_cost_check
        CHECK (reference_cost IS NULL OR (reference_cost > 0 AND reference_cost <= 1000000)),
    min_stock numeric(20,6) CONSTRAINT inventory_items_min_stock_check CHECK (min_stock IS NULL OR min_stock >= 0),
    reorder_point numeric(20,6) CONSTRAINT inventory_items_reorder_point_check
        CHECK (reorder_point IS NULL OR reorder_point >= 0),
    revision integer NOT NULL DEFAULT 1 CONSTRAINT inventory_items_revision_check CHECK (revision > 0),
    created_by uuid CONSTRAINT inventory_items_created_by_fkey REFERENCES app.users(id),
    updated_by uuid CONSTRAINT inventory_items_updated_by_fkey REFERENCES app.users(id),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX inventory_items_code_key ON app.inventory_items (lower(code));
CREATE UNIQUE INDEX inventory_items_name_key ON app.inventory_items (lower(name));
CREATE UNIQUE INDEX inventory_items_barcode_key ON app.inventory_items (barcode) WHERE barcode IS NOT NULL;
CREATE INDEX inventory_items_category_idx ON app.inventory_items (category_id) WHERE category_id IS NOT NULL;

-- 7. Presentaciones de compra de cada insumo («Caja con 24», «Costal 25 kg»): base_quantity es su
-- equivalencia en la unidad base del insumo. La conversión caja → piezas vive aquí, nunca como
-- equivalencia universal. Una sola presentación de compra predeterminada (activa) por insumo.
-- Los documentos guardan la cantidad base calculada al registrarse, por lo que cambiar una
-- presentación después no altera la historia.
CREATE TABLE app.inventory_presentations (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    item_id uuid NOT NULL CONSTRAINT inventory_presentations_item_id_fkey REFERENCES app.inventory_items(id),
    name text NOT NULL CONSTRAINT inventory_presentations_name_check
        CHECK (length(name) BETWEEN 1 AND 60 AND name = btrim(name)),
    base_quantity numeric(20,6) NOT NULL CONSTRAINT inventory_presentations_base_quantity_check
        CHECK (base_quantity > 0 AND base_quantity <= 1000000000),
    barcode text CONSTRAINT inventory_presentations_barcode_check CHECK (barcode IS NULL OR barcode ~ '^[!-~]{1,64}$'),
    active boolean NOT NULL DEFAULT true,
    purchase_default boolean NOT NULL DEFAULT false,
    revision integer NOT NULL DEFAULT 1 CONSTRAINT inventory_presentations_revision_check CHECK (revision > 0),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT inventory_presentations_default_active_check CHECK (NOT purchase_default OR active)
);
CREATE UNIQUE INDEX inventory_presentations_name_key ON app.inventory_presentations (item_id, lower(name));
CREATE UNIQUE INDEX inventory_presentations_barcode_key ON app.inventory_presentations (barcode) WHERE barcode IS NOT NULL;
CREATE UNIQUE INDEX inventory_presentations_default_key ON app.inventory_presentations (item_id) WHERE purchase_default;

-- 8. Almacenes. «Almacén general» es el destino por omisión que exige la especificación (estaciones
-- sin asignación y documentos sin almacén); no es un almacén ficticio y no se crea ningún otro
-- automáticamente. El predeterminado es único y siempre está activo.
CREATE TABLE app.inventory_warehouses (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    name text NOT NULL CONSTRAINT inventory_warehouses_name_check
        CHECK (length(name) BETWEEN 1 AND 60 AND name = btrim(name)),
    description text NOT NULL DEFAULT '' CONSTRAINT inventory_warehouses_description_check CHECK (length(description) <= 500),
    active boolean NOT NULL DEFAULT true,
    is_default boolean NOT NULL DEFAULT false,
    revision integer NOT NULL DEFAULT 1 CONSTRAINT inventory_warehouses_revision_check CHECK (revision > 0),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT inventory_warehouses_default_active_check CHECK (NOT is_default OR active)
);
CREATE UNIQUE INDEX inventory_warehouses_name_key ON app.inventory_warehouses (lower(name));
CREATE UNIQUE INDEX inventory_warehouses_default_key ON app.inventory_warehouses (is_default) WHERE is_default;
INSERT INTO app.inventory_warehouses(name, description, is_default)
VALUES ('Almacén general', 'Almacén predeterminado del inventario', true);

-- 9. Asignación de cada estación de preparación a su almacén de consumo, como historial de solo
-- inserción: la vigente es la de effective_from más reciente (desempate por seq) y el trabajador de
-- la fase B la resuelve a la fecha de liberación. warehouse_id NULL significa usar el almacén
-- predeterminado («Almacén general»). Varias estaciones pueden compartir un almacén.
CREATE TABLE app.inventory_station_warehouses (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    seq bigserial NOT NULL,
    station_id uuid NOT NULL CONSTRAINT inventory_station_warehouses_station_id_fkey REFERENCES app.kitchen_stations(id),
    warehouse_id uuid CONSTRAINT inventory_station_warehouses_warehouse_id_fkey REFERENCES app.inventory_warehouses(id),
    effective_from timestamptz NOT NULL DEFAULT clock_timestamp(),
    actor_id uuid NOT NULL CONSTRAINT inventory_station_warehouses_actor_id_fkey REFERENCES app.users(id),
    operation_id uuid CONSTRAINT inventory_station_warehouses_operation_id_fkey
        REFERENCES app.restaurant_operations(id) DEFERRABLE INITIALLY DEFERRED,
    CONSTRAINT inventory_station_warehouses_seq_key UNIQUE (seq)
);
CREATE INDEX inventory_station_warehouses_station_idx
    ON app.inventory_station_warehouses (station_id, effective_from DESC, seq DESC);

-- 10. Existencias por insumo y almacén. Solo las actualiza el libro de movimientos (Go); no existe
-- edición manual. Pueden ser negativas: las existencias negativas no bloquean las ventas.
CREATE TABLE app.inventory_stock (
    item_id uuid NOT NULL CONSTRAINT inventory_stock_item_id_fkey REFERENCES app.inventory_items(id),
    warehouse_id uuid NOT NULL CONSTRAINT inventory_stock_warehouse_id_fkey REFERENCES app.inventory_warehouses(id),
    quantity numeric(20,6) NOT NULL DEFAULT 0,
    updated_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT inventory_stock_pkey PRIMARY KEY (item_id, warehouse_id)
);
CREATE INDEX inventory_stock_warehouse_idx ON app.inventory_stock (warehouse_id);

-- 11. Valuación por insumo (el costo promedio es uno solo por insumo, no por almacén): cantidad total
-- (Σ existencias del insumo) y valor total, de los que sale el promedio valor ÷ cantidad cuando la
-- cantidad es positiva. last_valid_cost es el último promedio con existencia positiva y
-- pending_cost_quantity la parte del déficit valorada en cero por falta de costo. Solo la
-- actualiza el libro. El CHECK hace cumplir en la base el invariante «cantidad 0 ⇒ valor 0».
CREATE TABLE app.inventory_valuations (
    item_id uuid PRIMARY KEY CONSTRAINT inventory_valuations_item_id_fkey REFERENCES app.inventory_items(id),
    quantity numeric(20,6) NOT NULL DEFAULT 0,
    value numeric(20,6) NOT NULL DEFAULT 0,
    last_valid_cost numeric(24,10) CONSTRAINT inventory_valuations_last_valid_cost_check
        CHECK (last_valid_cost IS NULL OR last_valid_cost > 0),
    pending_cost_quantity numeric(20,6) NOT NULL DEFAULT 0
        CONSTRAINT inventory_valuations_pending_cost_quantity_check CHECK (pending_cost_quantity >= 0),
    updated_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT inventory_valuations_zero_check CHECK (quantity <> 0 OR value = 0)
);

-- 12. Libro de movimientos (solo inserción). Cada movimiento guarda insumo, almacén, cantidad con
-- signo en la unidad base del insumo, valor con signo, costo unitario y su base de cálculo, la hora
-- de negocio (occurred_at: liberación, recepción, hora del renglón contado) y la de registro, el
-- origen (source_kind + source_id + source_line), la unidad comercial si aplica, el motivo, el
-- responsable (usuario o proceso de Go, exactamente uno), la operación idempotente que lo generó y
-- el estado después de aplicarlo (existencia del almacén y valuación del insumo) para auditar.
-- entry_key es la clave determinista de idempotencia del libro: aplicar dos veces la misma entrada
-- devuelve el movimiento registrado sin repetir efectos. cost_adjustment es solo valor (cantidad 0,
-- sin almacén) y se liga a la operación que lo produjo (related_movement_id).
CREATE TABLE app.inventory_movements (
    id uuid PRIMARY KEY,
    seq bigserial NOT NULL,
    item_id uuid NOT NULL CONSTRAINT inventory_movements_item_id_fkey REFERENCES app.inventory_items(id),
    warehouse_id uuid CONSTRAINT inventory_movements_warehouse_id_fkey REFERENCES app.inventory_warehouses(id),
    kind text NOT NULL CONSTRAINT inventory_movements_kind_check CHECK (
        kind IN (
            'initial', 'receipt', 'consumption', 'consumption_reversal', 'waste', 'adjustment_in', 'adjustment_out',
            'transfer_out', 'transfer_in', 'supplier_return', 'production_in', 'production_out', 'count_in', 'count_out',
            'cost_adjustment'
        )
    ),
    quantity numeric(20,6) NOT NULL,
    value numeric(20,6) NOT NULL,
    unit_cost numeric(24,10) CONSTRAINT inventory_movements_unit_cost_check CHECK (unit_cost IS NULL OR unit_cost >= 0),
    cost_basis text NOT NULL CONSTRAINT inventory_movements_cost_basis_check CHECK (
        cost_basis IN ('average', 'reference', 'configured', 'pending', 'document', 'original', 'provisional', 'difference')
    ),
    cost_pending boolean NOT NULL DEFAULT false,
    occurred_at timestamptz NOT NULL,
    recorded_at timestamptz NOT NULL DEFAULT clock_timestamp(),
    source_kind text NOT NULL CONSTRAINT inventory_movements_source_kind_check CHECK (source_kind ~ '^[a-z][a-z_]{1,39}$'),
    source_id uuid,
    source_line integer CONSTRAINT inventory_movements_source_line_check CHECK (source_line IS NULL OR source_line > 0),
    restaurant_unit_id uuid CONSTRAINT inventory_movements_restaurant_unit_id_fkey REFERENCES app.restaurant_item_units(id),
    related_movement_id uuid CONSTRAINT inventory_movements_related_movement_id_fkey REFERENCES app.inventory_movements(id),
    reason text CONSTRAINT inventory_movements_reason_check CHECK (reason IS NULL OR length(reason) BETWEEN 1 AND 500),
    actor_id uuid CONSTRAINT inventory_movements_actor_id_fkey REFERENCES app.users(id),
    process text CONSTRAINT inventory_movements_process_check CHECK (process IS NULL OR process ~ '^[a-z][a-z_]{1,39}$'),
    operation_id uuid CONSTRAINT inventory_movements_operation_id_fkey
        REFERENCES app.restaurant_operations(id) DEFERRABLE INITIALLY DEFERRED,
    entry_key text NOT NULL CONSTRAINT inventory_movements_entry_key_check CHECK (length(entry_key) BETWEEN 1 AND 200),
    stock_after numeric(20,6),
    valuation_quantity_after numeric(20,6) NOT NULL,
    valuation_value_after numeric(20,6) NOT NULL,
    CONSTRAINT inventory_movements_seq_key UNIQUE (seq),
    CONSTRAINT inventory_movements_entry_key_key UNIQUE (entry_key),
    -- Exactamente un responsable: el usuario que registró la operación o el proceso de Go.
    CONSTRAINT inventory_movements_responsible_check CHECK (
        (actor_id IS NOT NULL AND process IS NULL) OR (actor_id IS NULL AND process IS NOT NULL)
    ),
    -- El ajuste de costo es solo valor; todo otro movimiento mueve existencias de un almacén.
    CONSTRAINT inventory_movements_shape_check CHECK (
        (kind = 'cost_adjustment' AND warehouse_id IS NULL AND quantity = 0 AND stock_after IS NULL AND unit_cost IS NULL)
        OR (kind <> 'cost_adjustment' AND warehouse_id IS NOT NULL AND quantity <> 0 AND stock_after IS NOT NULL)
    ),
    -- Signo de la cantidad según el tipo: entradas positivas y salidas negativas.
    CONSTRAINT inventory_movements_sign_check CHECK (
        kind = 'cost_adjustment'
        OR (
            kind IN ('initial', 'receipt', 'consumption_reversal', 'adjustment_in', 'transfer_in', 'production_in', 'count_in')
            AND quantity > 0
        )
        OR (
            kind IN ('consumption', 'waste', 'adjustment_out', 'transfer_out', 'supplier_return', 'production_out', 'count_out')
            AND quantity < 0
        )
    ),
    -- Un movimiento sin costo disponible siempre queda marcado como «costo pendiente».
    CONSTRAINT inventory_movements_pending_check CHECK (cost_basis <> 'pending' OR cost_pending)
);
CREATE INDEX inventory_movements_item_warehouse_idx ON app.inventory_movements (item_id, warehouse_id, occurred_at);
CREATE INDEX inventory_movements_source_idx ON app.inventory_movements (source_kind, source_id);
CREATE INDEX inventory_movements_unit_idx ON app.inventory_movements (restaurant_unit_id) WHERE restaurant_unit_id IS NOT NULL;
CREATE INDEX inventory_movements_recorded_idx ON app.inventory_movements (recorded_at);
CREATE INDEX inventory_movements_occurred_idx ON app.inventory_movements (occurred_at, seq);

-- El libro y el historial de asignaciones son de solo inserción también en la base: cualquier UPDATE,
-- DELETE o TRUNCATE se rechaza. Toda corrección se expresa con movimientos nuevos (§0-B, regla 9).
CREATE FUNCTION app.inventory_reject_mutation() RETURNS trigger
    LANGUAGE plpgsql AS $$
BEGIN
    RAISE EXCEPTION 'Los registros de % son de solo inserción', TG_TABLE_NAME USING ERRCODE = '55000';
END;
$$;
CREATE TRIGGER inventory_movements_insert_only
    BEFORE UPDATE OR DELETE ON app.inventory_movements
    FOR EACH ROW EXECUTE FUNCTION app.inventory_reject_mutation();
CREATE TRIGGER inventory_movements_no_truncate
    BEFORE TRUNCATE ON app.inventory_movements
    FOR EACH STATEMENT EXECUTE FUNCTION app.inventory_reject_mutation();
CREATE TRIGGER inventory_station_warehouses_insert_only
    BEFORE UPDATE OR DELETE ON app.inventory_station_warehouses
    FOR EACH ROW EXECUTE FUNCTION app.inventory_reject_mutation();
CREATE TRIGGER inventory_station_warehouses_no_truncate
    BEFORE TRUNCATE ON app.inventory_station_warehouses
    FOR EACH STATEMENT EXECUTE FUNCTION app.inventory_reject_mutation();

-- 13. Permisos de la fase A. Se registran también en Go (inventory.Permissions) con la misma
-- descripción y se conceden solo al administrador, para que siga contando como administrador
-- completo. El rol existente «Inventarios» no recibe permisos por inferencia; cada fase agrega solo
-- los permisos que usa. inventory.adjustments.authorize lo exige la existencia inicial.
INSERT INTO app.permissions(key, description) VALUES
    ('inventory.read', 'Consultar inventario (sección y resumen)'),
    ('inventory.items.manage', 'Administrar insumos, unidades y presentaciones'),
    ('inventory.costs.read', 'Consultar costos (unitarios, valores, costo teórico)'),
    ('inventory.costs.manage', 'Administrar costos de referencia y costos de existencias iniciales'),
    ('inventory.stock.read', 'Consultar existencias (cantidades, sin costos)'),
    ('inventory.warehouses.manage', 'Administrar almacenes y su asignación a estaciones'),
    ('inventory.movements.read', 'Consultar movimientos (cantidades; valores solo con consulta de costos)'),
    ('inventory.adjustments.authorize', 'Autorizar ajustes y corregir clasificaciones vencidas'),
    ('inventory.valuation.read', 'Consultar valuaciones (valor total del inventario por insumo/almacén)'),
    ('inventory.settings.manage', 'Administrar parámetros de inventario');

INSERT INTO app.role_permissions(role_id, permission_key)
SELECT r.id, p.key FROM app.roles r CROSS JOIN app.permissions p
WHERE r.code = 'administrator' AND p.key IN (
    'inventory.read', 'inventory.items.manage', 'inventory.costs.read', 'inventory.costs.manage',
    'inventory.stock.read', 'inventory.warehouses.manage', 'inventory.movements.read',
    'inventory.adjustments.authorize', 'inventory.valuation.read', 'inventory.settings.manage'
)
ON CONFLICT DO NOTHING;
