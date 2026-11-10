-- Etapa 8, fase B: recetas versionadas, subrecetas, porciones habituales de los ingredientes del
-- catálogo, solicitudes de consumo por liberación a cocina, estado de consumo por unidad comercial,
-- clasificación de cancelaciones y elaboraciones de subrecetas almacenables, con los permisos de la
-- fase B.
-- Transaccional: el migrador la aplica dentro de la misma transacción que la registra en
-- schema_migrations y no contiene sentencias que no admitan transacción (ni CREATE INDEX
-- CONCURRENTLY ni VACUUM), de modo que un fallo la revierte completa. No modifica 0001–0010 ni
-- filas existentes y no siembra datos (ni recetas ni porciones: las configura el restaurante). Las
-- columnas opcionales llevan CHECK que exigen IS NOT NULL en la rama que valida, porque un CHECK
-- que evalúa a NULL se acepta. El cambio de catalog_category_quick_notes (indicaciones con efecto)
-- va en otra migración.
--
-- Precisión (la misma de la 0010): cantidades numeric(20,6) en la unidad base, valores
-- numeric(20,6). Las capturas guardan la cantidad y la unidad tal como se escribieron y, además,
-- la cantidad convertida a la unidad base al guardar (base_quantity): cambiar después la
-- equivalencia de una unidad propia no altera recetas ni consumos históricos.

-- 1. Porción habitual de cada ingrediente del catálogo: la cantidad física de un insumo que
-- representa una porción del ingrediente («20 g de cebolla por porción»). Historial de solo
-- inserción; la vigente a una fecha es la de effective_from más reciente (desempate por seq). Una
-- fila sin insumo retira la porción: desde entonces el ingrediente no consume nada.
CREATE TABLE app.inventory_ingredient_portions (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    seq bigserial NOT NULL,
    ingredient_id uuid NOT NULL CONSTRAINT inventory_ingredient_portions_ingredient_id_fkey
        REFERENCES app.catalog_ingredients(id),
    item_id uuid CONSTRAINT inventory_ingredient_portions_item_id_fkey REFERENCES app.inventory_items(id),
    quantity numeric(20,6),
    unit_id uuid CONSTRAINT inventory_ingredient_portions_unit_id_fkey REFERENCES app.inventory_units(id),
    base_quantity numeric(20,6),
    effective_from timestamptz NOT NULL DEFAULT clock_timestamp(),
    actor_id uuid NOT NULL CONSTRAINT inventory_ingredient_portions_actor_id_fkey REFERENCES app.users(id),
    operation_id uuid CONSTRAINT inventory_ingredient_portions_operation_id_fkey
        REFERENCES app.restaurant_operations(id) DEFERRABLE INITIALLY DEFERRED,
    CONSTRAINT inventory_ingredient_portions_seq_key UNIQUE (seq),
    CONSTRAINT inventory_ingredient_portions_shape_check CHECK (
        (item_id IS NULL AND quantity IS NULL AND unit_id IS NULL AND base_quantity IS NULL)
        OR (
            item_id IS NOT NULL AND quantity IS NOT NULL AND unit_id IS NOT NULL AND base_quantity IS NOT NULL
            AND quantity > 0 AND quantity <= 1000000000 AND base_quantity > 0 AND base_quantity <= 1000000000
        )
    )
);
CREATE INDEX inventory_ingredient_portions_ingredient_idx
    ON app.inventory_ingredient_portions (ingredient_id, effective_from DESC, seq DESC);
CREATE INDEX inventory_ingredient_portions_item_idx ON app.inventory_ingredient_portions (item_id) WHERE item_id IS NOT NULL;

-- 2. Recetas. Identidad estable de una receta de:
--   - producto del catálogo (variant_id opcional: la receta de la variante prevalece sobre la del
--     producto);
--   - complemento del catálogo (opciones de grupo);
--   - preparación (subreceta): de elaboración inmediata (se descompone en sus insumos al consumir)
--     o almacenable (produce el insumo preparation_item_id, de tipo prepared, mediante
--     elaboraciones; al venderse se consume ese insumo y no sus ingredientes).
-- El contenido vive en versiones inmutables (sección 3); name, active y revision son el reflejo de
-- la versión más reciente para los listados. La modalidad de la preparación y su insumo no cambian
-- después de crearla (Go lo valida): cambiarlos alteraría la lectura de consumos históricos.
CREATE TABLE app.inventory_recipes (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    owner_kind text NOT NULL CONSTRAINT inventory_recipes_owner_kind_check
        CHECK (owner_kind IN ('product', 'complement', 'preparation')),
    product_id uuid CONSTRAINT inventory_recipes_product_id_fkey REFERENCES app.catalog_products(id),
    variant_id uuid,
    complement_id uuid CONSTRAINT inventory_recipes_complement_id_fkey REFERENCES app.catalog_complements(id),
    preparation_mode text CONSTRAINT inventory_recipes_preparation_mode_check
        CHECK (preparation_mode IS NULL OR preparation_mode IN ('immediate', 'stored')),
    preparation_item_id uuid CONSTRAINT inventory_recipes_preparation_item_id_fkey REFERENCES app.inventory_items(id),
    name text NOT NULL CONSTRAINT inventory_recipes_name_check CHECK (length(name) BETWEEN 1 AND 100 AND name = btrim(name)),
    active boolean NOT NULL DEFAULT true,
    revision integer NOT NULL DEFAULT 1 CONSTRAINT inventory_recipes_revision_check CHECK (revision > 0),
    created_by uuid NOT NULL CONSTRAINT inventory_recipes_created_by_fkey REFERENCES app.users(id),
    updated_by uuid NOT NULL CONSTRAINT inventory_recipes_updated_by_fkey REFERENCES app.users(id),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    -- La variante debe pertenecer al producto de la receta.
    CONSTRAINT inventory_recipes_variant_id_fkey FOREIGN KEY (variant_id, product_id)
        REFERENCES app.catalog_variants(id, product_id),
    -- Coherencia de cada tipo de receta: exactamente el destino que le corresponde.
    CONSTRAINT inventory_recipes_owner_check CHECK (
        (
            owner_kind = 'product' AND product_id IS NOT NULL AND complement_id IS NULL
            AND preparation_mode IS NULL AND preparation_item_id IS NULL
        )
        OR (
            owner_kind = 'complement' AND complement_id IS NOT NULL AND product_id IS NULL AND variant_id IS NULL
            AND preparation_mode IS NULL AND preparation_item_id IS NULL
        )
        OR (
            owner_kind = 'preparation' AND product_id IS NULL AND variant_id IS NULL AND complement_id IS NULL
            AND preparation_mode IS NOT NULL
            AND (
                (preparation_mode = 'immediate' AND preparation_item_id IS NULL)
                OR (preparation_mode = 'stored' AND preparation_item_id IS NOT NULL)
            )
        )
    )
);
CREATE UNIQUE INDEX inventory_recipes_product_key ON app.inventory_recipes (product_id)
    WHERE owner_kind = 'product' AND variant_id IS NULL;
CREATE UNIQUE INDEX inventory_recipes_variant_key ON app.inventory_recipes (product_id, variant_id) WHERE variant_id IS NOT NULL;
CREATE UNIQUE INDEX inventory_recipes_complement_key ON app.inventory_recipes (complement_id) WHERE complement_id IS NOT NULL;
CREATE UNIQUE INDEX inventory_recipes_preparation_item_key ON app.inventory_recipes (preparation_item_id)
    WHERE preparation_item_id IS NOT NULL;
CREATE UNIQUE INDEX inventory_recipes_preparation_name_key ON app.inventory_recipes (lower(name)) WHERE owner_kind = 'preparation';

-- 3. Versiones de receta (inmutables). Cada guardado crea una versión con effective_from = la hora
-- del guardado; la vigente a una fecha es la de effective_from máxima ≤ esa fecha. Desactivar una
-- receta también crea una versión (active = false), de modo que la vigencia es histórica.
--   - yield_quantity + yield_unit_id: rendimiento. En recetas de producto y de complemento se
--     expresa en porciones (1 porción por omisión): cada unidad vendida consume 1/rendimiento de los
--     renglones. En preparaciones es la cantidad que produce un lote (gramos, mililitros, piezas o
--     porciones; en las almacenables, de la misma dimensión que la unidad base de su insumo).
--   - yield_base_quantity: el rendimiento en la unidad base de su dimensión, fijado al guardar.
--   - production_waste_percent: merma de elaboración de una preparación (0 en productos y
--     complementos). El rendimiento efectivo de un lote es yield_base_quantity × (1 − merma/100).
CREATE TABLE app.inventory_recipe_versions (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    recipe_id uuid NOT NULL CONSTRAINT inventory_recipe_versions_recipe_id_fkey REFERENCES app.inventory_recipes(id),
    version integer NOT NULL CONSTRAINT inventory_recipe_versions_version_check CHECK (version > 0),
    name text NOT NULL CONSTRAINT inventory_recipe_versions_name_check
        CHECK (length(name) BETWEEN 1 AND 100 AND name = btrim(name)),
    active boolean NOT NULL,
    yield_quantity numeric(20,6) NOT NULL CONSTRAINT inventory_recipe_versions_yield_quantity_check
        CHECK (yield_quantity > 0 AND yield_quantity <= 1000000000),
    yield_unit_id uuid NOT NULL CONSTRAINT inventory_recipe_versions_yield_unit_id_fkey REFERENCES app.inventory_units(id),
    yield_base_quantity numeric(20,6) NOT NULL CONSTRAINT inventory_recipe_versions_yield_base_quantity_check
        CHECK (yield_base_quantity > 0 AND yield_base_quantity <= 1000000000),
    production_waste_percent numeric(7,4) NOT NULL DEFAULT 0 CONSTRAINT inventory_recipe_versions_waste_check
        CHECK (production_waste_percent >= 0 AND production_waste_percent < 100),
    effective_from timestamptz NOT NULL DEFAULT clock_timestamp(),
    notes text NOT NULL DEFAULT '' CONSTRAINT inventory_recipe_versions_notes_check CHECK (length(notes) <= 1000),
    created_by uuid NOT NULL CONSTRAINT inventory_recipe_versions_created_by_fkey REFERENCES app.users(id),
    operation_id uuid CONSTRAINT inventory_recipe_versions_operation_id_fkey
        REFERENCES app.restaurant_operations(id) DEFERRABLE INITIALLY DEFERRED,
    created_at timestamptz NOT NULL DEFAULT clock_timestamp(),
    CONSTRAINT inventory_recipe_versions_recipe_version_key UNIQUE (recipe_id, version)
);
CREATE INDEX inventory_recipe_versions_effective_idx ON app.inventory_recipe_versions (recipe_id, effective_from DESC, version DESC);

-- 4. Renglones de una versión (inmutables con su versión):
--   - item: insumo fijo (cantidad por rendimiento de la receta);
--   - preparation: subreceta; si es inmediata se descompone en sus insumos (cantidad ÷ rendimiento
--     efectivo de la preparación) y si es almacenable se consume su insumo preparado;
--   - ingredient: porción de un ingrediente del catálogo para este producto; sobrescribe su porción
--     habitual (se multiplica por las porciones pedidas) y apunta a un insumo o a una preparación.
-- quantity/unit_id son la captura (misma dimensión que la unidad base del insumo o que el
-- rendimiento de la preparación) y base_quantity su conversión al guardar. takeaway_only marca
-- componentes que solo se consumen en cuentas para llevar (incluida ventanilla), como empaques; no
-- aplica a renglones de ingrediente, que dependen de lo pedido.
CREATE TABLE app.inventory_recipe_lines (
    version_id uuid NOT NULL CONSTRAINT inventory_recipe_lines_version_id_fkey REFERENCES app.inventory_recipe_versions(id),
    line_no integer NOT NULL CONSTRAINT inventory_recipe_lines_line_no_check CHECK (line_no BETWEEN 1 AND 200),
    component_kind text NOT NULL CONSTRAINT inventory_recipe_lines_component_kind_check
        CHECK (component_kind IN ('item', 'preparation', 'ingredient')),
    item_id uuid CONSTRAINT inventory_recipe_lines_item_id_fkey REFERENCES app.inventory_items(id),
    preparation_recipe_id uuid CONSTRAINT inventory_recipe_lines_preparation_recipe_id_fkey REFERENCES app.inventory_recipes(id),
    ingredient_id uuid CONSTRAINT inventory_recipe_lines_ingredient_id_fkey REFERENCES app.catalog_ingredients(id),
    quantity numeric(20,6) NOT NULL CONSTRAINT inventory_recipe_lines_quantity_check
        CHECK (quantity > 0 AND quantity <= 1000000000),
    unit_id uuid NOT NULL CONSTRAINT inventory_recipe_lines_unit_id_fkey REFERENCES app.inventory_units(id),
    base_quantity numeric(20,6) NOT NULL CONSTRAINT inventory_recipe_lines_base_quantity_check
        CHECK (base_quantity > 0 AND base_quantity <= 1000000000),
    takeaway_only boolean NOT NULL DEFAULT false,
    CONSTRAINT inventory_recipe_lines_pkey PRIMARY KEY (version_id, line_no),
    CONSTRAINT inventory_recipe_lines_component_check CHECK (
        (component_kind = 'item' AND item_id IS NOT NULL AND preparation_recipe_id IS NULL AND ingredient_id IS NULL)
        OR (component_kind = 'preparation' AND preparation_recipe_id IS NOT NULL AND item_id IS NULL AND ingredient_id IS NULL)
        OR (
            component_kind = 'ingredient' AND ingredient_id IS NOT NULL AND NOT takeaway_only
            AND (
                (item_id IS NOT NULL AND preparation_recipe_id IS NULL)
                OR (item_id IS NULL AND preparation_recipe_id IS NOT NULL)
            )
        )
    )
);
CREATE UNIQUE INDEX inventory_recipe_lines_ingredient_key ON app.inventory_recipe_lines (version_id, ingredient_id)
    WHERE ingredient_id IS NOT NULL;
CREATE INDEX inventory_recipe_lines_item_idx ON app.inventory_recipe_lines (item_id) WHERE item_id IS NOT NULL;
CREATE INDEX inventory_recipe_lines_preparation_idx ON app.inventory_recipe_lines (preparation_recipe_id)
    WHERE preparation_recipe_id IS NOT NULL;

-- 5. Solicitudes de consumo (§0-B «Consumo asíncrono»). La liberación a cocina solo registra una
-- solicitud release por unidad comercial liberada (en su misma transacción, sin el lock de
-- inventario ni leer recetas); las modificaciones y cancelaciones de unidades ya liberadas
-- registran amend y cancel. El trabajador de Go las procesa en transacciones propias, en orden de
-- seq por unidad, de forma idempotente. version_id es la versión liberada (release) o la vigente
-- tras el cambio (amend, cancel); released_at es la hora de la liberación canónica: recetas,
-- porciones y almacenes se resuelven a esa fecha. service_mode se congela al liberar. last_error es
-- un mensaje genérico, sin datos internos ni costos; diagnostics enumera lo que no tenía
-- configuración (producto sin receta, ingrediente sin porción) sin inventar consumos.
CREATE TABLE app.inventory_consumption_requests (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    seq bigserial NOT NULL,
    kind text NOT NULL CONSTRAINT inventory_consumption_requests_kind_check CHECK (kind IN ('release', 'amend', 'cancel')),
    restaurant_unit_id uuid NOT NULL CONSTRAINT inventory_consumption_requests_restaurant_unit_id_fkey
        REFERENCES app.restaurant_item_units(id),
    source_id uuid NOT NULL,
    version_id uuid NOT NULL CONSTRAINT inventory_consumption_requests_version_id_fkey
        REFERENCES app.restaurant_item_versions(id),
    released_at timestamptz NOT NULL,
    occurred_at timestamptz NOT NULL,
    service_mode text NOT NULL CONSTRAINT inventory_consumption_requests_service_mode_check
        CHECK (service_mode IN ('dine_in', 'takeaway')),
    state text NOT NULL DEFAULT 'pending' CONSTRAINT inventory_consumption_requests_state_check
        CHECK (state IN ('pending', 'processed', 'failed')),
    attempts integer NOT NULL DEFAULT 0 CONSTRAINT inventory_consumption_requests_attempts_check CHECK (attempts >= 0),
    last_error text CONSTRAINT inventory_consumption_requests_last_error_check
        CHECK (last_error IS NULL OR length(last_error) BETWEEN 1 AND 200),
    next_attempt_at timestamptz NOT NULL DEFAULT clock_timestamp(),
    processed_at timestamptz,
    diagnostics jsonb NOT NULL DEFAULT '[]' CONSTRAINT inventory_consumption_requests_diagnostics_check
        CHECK (jsonb_typeof(diagnostics) = 'array'),
    created_at timestamptz NOT NULL DEFAULT clock_timestamp(),
    CONSTRAINT inventory_consumption_requests_seq_key UNIQUE (seq),
    -- Una sola solicitud por liberación (o cambio) y unidad: la liberación repetida no duplica.
    CONSTRAINT inventory_consumption_requests_source_key UNIQUE (kind, restaurant_unit_id, source_id),
    CONSTRAINT inventory_consumption_requests_processed_check CHECK ((state = 'processed') = (processed_at IS NOT NULL)),
    CONSTRAINT inventory_consumption_requests_failed_check
        CHECK (state <> 'failed' OR (last_error IS NOT NULL AND attempts > 0))
);
CREATE INDEX inventory_consumption_requests_pending_idx ON app.inventory_consumption_requests (next_attempt_at, seq)
    WHERE state <> 'processed';
CREATE INDEX inventory_consumption_requests_unit_idx ON app.inventory_consumption_requests (restaurant_unit_id, seq);

-- 6. Estado de consumo por unidad comercial: existe desde que el trabajador procesa su liberación
-- (con la función activada) y sirve para calcular solo la diferencia en las modificaciones. Una
-- unidad liberada antes de activar no tiene fila y nunca consume ni se ajusta.
CREATE TABLE app.inventory_unit_consumptions (
    restaurant_unit_id uuid PRIMARY KEY CONSTRAINT inventory_unit_consumptions_restaurant_unit_id_fkey
        REFERENCES app.restaurant_item_units(id),
    release_request_id uuid NOT NULL CONSTRAINT inventory_unit_consumptions_release_request_id_fkey
        REFERENCES app.inventory_consumption_requests(id),
    last_request_id uuid NOT NULL CONSTRAINT inventory_unit_consumptions_last_request_id_fkey
        REFERENCES app.inventory_consumption_requests(id),
    last_version_id uuid NOT NULL CONSTRAINT inventory_unit_consumptions_last_version_id_fkey
        REFERENCES app.restaurant_item_versions(id),
    released_at timestamptz NOT NULL,
    service_mode text NOT NULL CONSTRAINT inventory_unit_consumptions_service_mode_check
        CHECK (service_mode IN ('dine_in', 'takeaway')),
    state text NOT NULL DEFAULT 'consumed' CONSTRAINT inventory_unit_consumptions_state_check
        CHECK (state IN ('consumed', 'reversed')),
    created_at timestamptz NOT NULL DEFAULT clock_timestamp(),
    updated_at timestamptz NOT NULL DEFAULT clock_timestamp(),
    CONSTRAINT inventory_unit_consumptions_release_request_key UNIQUE (release_request_id)
);

-- 7. Clasificación de cancelaciones de unidades ya consumidas (§0-B «Cancelaciones sin
-- clasificar»). Nace pendiente con su día operativo (zona horaria del establecimiento y hora de
-- corte vigentes al cancelar) y deadline_at = el corte que cierra ese día. Hasta entonces la
-- clasifica inventory.waste.record: «No preparado» revierte exactamente el consumo neto registrado
-- y «Preparado / merma» lo conserva y lo marca como merma. Después del corte, el reconciliador de
-- Go aplica «No preparado» automático (resolución automatic, sin usuario) y solo cabe una
-- corrección con inventory.adjustments.authorize. Toda corrección genera movimientos
-- compensatorios nuevos; nada se reescribe.
CREATE TABLE app.inventory_cancellation_classifications (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    restaurant_unit_id uuid NOT NULL CONSTRAINT inventory_cancellation_classifications_restaurant_unit_id_fkey
        REFERENCES app.restaurant_item_units(id),
    request_id uuid NOT NULL CONSTRAINT inventory_cancellation_classifications_request_id_fkey
        REFERENCES app.inventory_consumption_requests(id),
    cancelled_at timestamptz NOT NULL,
    operating_date date NOT NULL,
    deadline_at timestamptz NOT NULL,
    state text NOT NULL DEFAULT 'pending' CONSTRAINT inventory_cancellation_classifications_state_check
        CHECK (state IN ('pending', 'not_prepared', 'prepared_waste')),
    resolution text CONSTRAINT inventory_cancellation_classifications_resolution_check
        CHECK (resolution IS NULL OR resolution IN ('manual', 'automatic', 'correction')),
    classified_by uuid CONSTRAINT inventory_cancellation_classifications_classified_by_fkey REFERENCES app.users(id),
    classified_at timestamptz,
    notes text CONSTRAINT inventory_cancellation_classifications_notes_check
        CHECK (notes IS NULL OR length(notes) BETWEEN 1 AND 500),
    revision integer NOT NULL DEFAULT 1 CONSTRAINT inventory_cancellation_classifications_revision_check CHECK (revision > 0),
    created_at timestamptz NOT NULL DEFAULT clock_timestamp(),
    updated_at timestamptz NOT NULL DEFAULT clock_timestamp(),
    CONSTRAINT inventory_cancellation_classifications_unit_key UNIQUE (restaurant_unit_id),
    CONSTRAINT inventory_cancellation_classifications_request_key UNIQUE (request_id),
    CONSTRAINT inventory_cancellation_classifications_deadline_check CHECK (deadline_at > cancelled_at),
    -- Pendiente sin resolución; la automática la aplica el proceso (sin usuario) y solo como «No
    -- preparado»; la manual y la corrección registran quién clasificó.
    CONSTRAINT inventory_cancellation_classifications_resolution_shape_check CHECK (
        (state = 'pending' AND resolution IS NULL AND classified_by IS NULL AND classified_at IS NULL)
        OR (
            state = 'not_prepared' AND resolution IS NOT NULL AND resolution = 'automatic'
            AND classified_by IS NULL AND classified_at IS NOT NULL
        )
        OR (
            state <> 'pending' AND resolution IS NOT NULL AND resolution IN ('manual', 'correction')
            AND classified_by IS NOT NULL AND classified_at IS NOT NULL
        )
    )
);
CREATE INDEX inventory_cancellation_classifications_pending_idx ON app.inventory_cancellation_classifications (deadline_at, id)
    WHERE state = 'pending';
CREATE INDEX inventory_cancellation_classifications_date_idx ON app.inventory_cancellation_classifications (operating_date, state);

-- 8. Elaboraciones de subrecetas almacenables (documento inmutable). Consumen los insumos de la
-- versión vigente × lotes (movimientos production_out) y producen el insumo preparado con el valor
-- total consumido (movimiento production_in con documento). expected_quantity es lotes ×
-- rendimiento efectivo, produced_quantity la cantidad real y waste_quantity la merma de
-- elaboración registrada (lotes × rendimiento − producido, nunca negativa). consumed_value es
-- confidencial (inventory.costs.read).
CREATE TABLE app.inventory_productions (
    id uuid PRIMARY KEY,
    recipe_id uuid NOT NULL CONSTRAINT inventory_productions_recipe_id_fkey REFERENCES app.inventory_recipes(id),
    recipe_version_id uuid NOT NULL CONSTRAINT inventory_productions_recipe_version_id_fkey
        REFERENCES app.inventory_recipe_versions(id),
    item_id uuid NOT NULL CONSTRAINT inventory_productions_item_id_fkey REFERENCES app.inventory_items(id),
    warehouse_id uuid NOT NULL CONSTRAINT inventory_productions_warehouse_id_fkey REFERENCES app.inventory_warehouses(id),
    batches numeric(20,6) NOT NULL CONSTRAINT inventory_productions_batches_check CHECK (batches > 0 AND batches <= 100000),
    expected_quantity numeric(20,6) NOT NULL CONSTRAINT inventory_productions_expected_quantity_check
        CHECK (expected_quantity > 0),
    produced_quantity numeric(20,6) NOT NULL CONSTRAINT inventory_productions_produced_quantity_check
        CHECK (produced_quantity > 0 AND produced_quantity <= 1000000000),
    waste_quantity numeric(20,6) NOT NULL CONSTRAINT inventory_productions_waste_quantity_check CHECK (waste_quantity >= 0),
    consumed_value numeric(20,6) NOT NULL CONSTRAINT inventory_productions_consumed_value_check CHECK (consumed_value >= 0),
    occurred_at timestamptz NOT NULL,
    notes text NOT NULL DEFAULT '' CONSTRAINT inventory_productions_notes_check CHECK (length(notes) <= 500),
    actor_id uuid NOT NULL CONSTRAINT inventory_productions_actor_id_fkey REFERENCES app.users(id),
    operation_id uuid NOT NULL CONSTRAINT inventory_productions_operation_id_fkey
        REFERENCES app.restaurant_operations(id) DEFERRABLE INITIALLY DEFERRED,
    movement_id uuid NOT NULL CONSTRAINT inventory_productions_movement_id_fkey REFERENCES app.inventory_movements(id),
    created_at timestamptz NOT NULL DEFAULT clock_timestamp(),
    CONSTRAINT inventory_productions_operation_id_key UNIQUE (operation_id)
);
CREATE INDEX inventory_productions_occurred_idx ON app.inventory_productions (occurred_at DESC, id);
CREATE INDEX inventory_productions_recipe_idx ON app.inventory_productions (recipe_id, occurred_at DESC);

-- Porciones, versiones, renglones y elaboraciones son de solo inserción también en la base (la
-- función de rechazo es la de la 0010): toda corrección es un registro nuevo.
CREATE TRIGGER inventory_ingredient_portions_insert_only
    BEFORE UPDATE OR DELETE ON app.inventory_ingredient_portions
    FOR EACH ROW EXECUTE FUNCTION app.inventory_reject_mutation();
CREATE TRIGGER inventory_ingredient_portions_no_truncate
    BEFORE TRUNCATE ON app.inventory_ingredient_portions
    FOR EACH STATEMENT EXECUTE FUNCTION app.inventory_reject_mutation();
CREATE TRIGGER inventory_recipe_versions_insert_only
    BEFORE UPDATE OR DELETE ON app.inventory_recipe_versions
    FOR EACH ROW EXECUTE FUNCTION app.inventory_reject_mutation();
CREATE TRIGGER inventory_recipe_versions_no_truncate
    BEFORE TRUNCATE ON app.inventory_recipe_versions
    FOR EACH STATEMENT EXECUTE FUNCTION app.inventory_reject_mutation();
CREATE TRIGGER inventory_recipe_lines_insert_only
    BEFORE UPDATE OR DELETE ON app.inventory_recipe_lines
    FOR EACH ROW EXECUTE FUNCTION app.inventory_reject_mutation();
CREATE TRIGGER inventory_recipe_lines_no_truncate
    BEFORE TRUNCATE ON app.inventory_recipe_lines
    FOR EACH STATEMENT EXECUTE FUNCTION app.inventory_reject_mutation();
CREATE TRIGGER inventory_productions_insert_only
    BEFORE UPDATE OR DELETE ON app.inventory_productions
    FOR EACH ROW EXECUTE FUNCTION app.inventory_reject_mutation();
CREATE TRIGGER inventory_productions_no_truncate
    BEFORE TRUNCATE ON app.inventory_productions
    FOR EACH STATEMENT EXECUTE FUNCTION app.inventory_reject_mutation();

-- 9. Permisos de la fase B. Se registran también en Go (inventory.Permissions) con la misma
-- descripción y se conceden solo al administrador; el rol «Inventarios» no recibe permisos por
-- inferencia.
INSERT INTO app.permissions(key, description) VALUES
    ('inventory.recipes.manage', 'Administrar recetas y porciones habituales'),
    ('inventory.subrecipes.manage', 'Administrar subrecetas (preparaciones)'),
    ('inventory.production.record', 'Registrar elaboraciones de subrecetas almacenables'),
    ('inventory.waste.record', 'Registrar mermas y clasificar cancelaciones');

INSERT INTO app.role_permissions(role_id, permission_key)
SELECT r.id, p.key FROM app.roles r CROSS JOIN app.permissions p
WHERE r.code = 'administrator' AND p.key IN (
    'inventory.recipes.manage', 'inventory.subrecipes.manage', 'inventory.production.record', 'inventory.waste.record'
)
ON CONFLICT DO NOTHING;
