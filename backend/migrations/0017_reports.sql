-- Etapa 9A, fase B (B1): reportes. Libro inmutable de reconocimiento de ventas, índices de fecha para
-- las consultas de reportes y permisos del módulo Reportes.
-- Transaccional: el migrador la aplica dentro de la misma transacción que la registra en
-- schema_migrations y no contiene sentencias que no admitan transacción (ni CREATE INDEX
-- CONCURRENTLY ni VACUUM), de modo que un fallo la revierte completa. No modifica 0001–0016 ni filas
-- existentes; solo inserta el relleno inicial del libro con las cuentas hoy cerradas (datos reales,
-- nunca ficticios). Las restricciones llevan nombre explícito y los CHECK con columnas opcionales
-- exigen IS NOT NULL en la rama que valida, porque un CHECK que evalúa a NULL se acepta.

-- 1. Reconocimiento de ventas (§0-B «Cuándo cuenta una venta»). La venta cuenta cuando la cuenta se
-- cierra (queda liquidada), en el día de cierre. restaurant_accounts.closed_at no sirve para
-- reportar: una devolución reabre la cuenta y lo borra, y una reliquidación lo mueve. Por eso
-- payments.closeIfSettled escribe aquí, en la misma transacción del cierre, la diferencia de cada
-- fuente respecto de lo ya reconocido:
--   - unit: una unidad comercial activa (cantidad 1, importe bruto, promoción y neto);
--   - allocation: la asignación de un descuento o cortesía por partida a una unidad;
--   - adjustment: un descuento o cortesía general de cuenta (repartido entre tasas de IVA en
--     proporción, como payments/finance.go) o un cargo general.
-- La clave de una fuente es (source_kind, unit_id, adjustment_id, tax_rate). El primer cierre
-- reconoce todo; una reliquidación tras una reapertura reconoce solo diferencias (una unidad
-- cancelada después de reabrir resta en el día del nuevo cierre) y no escribe nada si nada cambió.
-- Venta del periodo = Σ de los renglones con recognized_at en el periodo. Las devoluciones nunca
-- restan aquí: se reportan en su propio día con los cobros. Las propinas tampoco son venta.
-- El IVA de cada renglón se calcula con la regla de finance.go (precios con IVA incluido, desglose
-- por cuenta y tasa) y se reparte entre los renglones de la misma cuenta y tasa, de modo que la suma
-- por cuenta y tasa es exactamente el desglose de la cuenta (no se recalcula sobre el periodo).
-- Los nombres de producto, variante, categoría, lista de precios, mesa, área y motivo se copian al
-- reconocer, para reportar con los datos de la operación sin leer el snapshot completo del pedido.
CREATE TABLE app.sales_recognitions (
    id uuid NOT NULL DEFAULT gen_random_uuid() CONSTRAINT sales_recognitions_pkey PRIMARY KEY,
    sequence bigint GENERATED ALWAYS AS IDENTITY CONSTRAINT sales_recognitions_sequence_key UNIQUE,
    closing_id uuid NOT NULL,
    recognition_kind text NOT NULL CONSTRAINT sales_recognitions_kind_check
        CHECK (recognition_kind IN ('initial', 'resettlement', 'backfill')),
    recognized_at timestamptz NOT NULL,
    account_id uuid NOT NULL CONSTRAINT sales_recognitions_account_id_fkey REFERENCES app.restaurant_accounts(id),
    account_folio bigint NOT NULL,
    mode text NOT NULL CONSTRAINT sales_recognitions_mode_check CHECK (mode IN ('dine_in', 'takeaway')),
    origin text NOT NULL,
    table_id uuid,
    table_name text,
    area_id uuid,
    area_name text,
    responsible_user_id uuid CONSTRAINT sales_recognitions_responsible_user_id_fkey REFERENCES app.users(id),
    shift_id uuid CONSTRAINT sales_recognitions_shift_id_fkey REFERENCES app.cash_shifts(id),
    register_id uuid CONSTRAINT sales_recognitions_register_id_fkey REFERENCES app.registers(id),
    actor_id uuid CONSTRAINT sales_recognitions_actor_id_fkey REFERENCES app.users(id),
    device_id uuid CONSTRAINT sales_recognitions_device_id_fkey REFERENCES app.devices(id),
    operation_id uuid CONSTRAINT sales_recognitions_operation_id_fkey REFERENCES app.restaurant_operations(id)
        DEFERRABLE INITIALLY DEFERRED,
    source_kind text NOT NULL CONSTRAINT sales_recognitions_source_kind_check
        CHECK (source_kind IN ('unit', 'allocation', 'adjustment')),
    unit_id uuid CONSTRAINT sales_recognitions_unit_id_fkey REFERENCES app.restaurant_item_units(id),
    adjustment_id uuid CONSTRAINT sales_recognitions_adjustment_id_fkey REFERENCES app.account_adjustments(id),
    version_id uuid CONSTRAINT sales_recognitions_version_id_fkey REFERENCES app.restaurant_item_versions(id),
    order_id uuid CONSTRAINT sales_recognitions_order_id_fkey REFERENCES app.restaurant_orders(id),
    product_id uuid,
    product_name text,
    product_code text,
    product_kind text,
    variant_id uuid,
    variant_name text,
    category_id uuid,
    category_name text,
    price_list_id uuid,
    price_list_name text,
    adjustment_kind text CONSTRAINT sales_recognitions_adjustment_kind_check
        CHECK (adjustment_kind IS NULL OR adjustment_kind IN ('discount', 'charge')),
    adjustment_scope text CONSTRAINT sales_recognitions_adjustment_scope_check
        CHECK (adjustment_scope IS NULL OR adjustment_scope IN ('account', 'items')),
    is_courtesy boolean NOT NULL DEFAULT false,
    reason_id uuid,
    reason_name text,
    concept text,
    adjustment_actor_id uuid CONSTRAINT sales_recognitions_adjustment_actor_id_fkey REFERENCES app.users(id),
    adjustment_authorized_by uuid CONSTRAINT sales_recognitions_adjustment_authorized_by_fkey REFERENCES app.users(id),
    adjustment_created_at timestamptz,
    tax_rate integer NOT NULL CONSTRAINT sales_recognitions_tax_rate_check CHECK (tax_rate IN (0, 8, 16)),
    quantity integer NOT NULL,
    gross numeric(18,2) NOT NULL,
    promotion numeric(18,2) NOT NULL,
    discount numeric(18,2) NOT NULL,
    courtesy numeric(18,2) NOT NULL,
    charge numeric(18,2) NOT NULL,
    net numeric(18,2) NOT NULL,
    base numeric(18,2) NOT NULL,
    tax numeric(18,2) NOT NULL,
    recorded_at timestamptz NOT NULL DEFAULT clock_timestamp(),
    CONSTRAINT sales_recognitions_net_check CHECK (net = gross - promotion - discount - courtesy + charge),
    CONSTRAINT sales_recognitions_tax_check CHECK (net = base + tax),
    -- Cada clase de fuente lleva exactamente sus identificadores; los datos del ajuste solo existen en
    -- las fuentes que vienen de un ajuste.
    CONSTRAINT sales_recognitions_source_check CHECK (
        (source_kind = 'unit' AND unit_id IS NOT NULL AND adjustment_id IS NULL AND adjustment_kind IS NULL
            AND NOT is_courtesy)
        OR (source_kind = 'allocation' AND unit_id IS NOT NULL AND adjustment_id IS NOT NULL
            AND adjustment_kind IS NOT NULL AND adjustment_kind = 'discount' AND adjustment_scope IS NOT NULL
            AND adjustment_scope = 'items' AND quantity = 0 AND gross = 0 AND promotion = 0 AND charge = 0)
        OR (source_kind = 'adjustment' AND unit_id IS NULL AND adjustment_id IS NOT NULL
            AND adjustment_kind IS NOT NULL AND adjustment_scope IS NOT NULL AND adjustment_scope = 'account'
            AND quantity = 0 AND gross = 0 AND promotion = 0)
    ),
    CONSTRAINT sales_recognitions_courtesy_check CHECK (is_courtesy OR courtesy = 0)
);
-- Filtro por fecha de reconocimiento (todas las consultas de ventas, productos y descuentos).
CREATE INDEX sales_recognitions_recognized_idx ON app.sales_recognitions (recognized_at, sequence);
-- Diferencias al reliquidar y cuentas reabiertas en «En curso».
CREATE INDEX sales_recognitions_account_idx ON app.sales_recognitions (account_id, sequence);
CREATE INDEX sales_recognitions_unit_idx ON app.sales_recognitions (unit_id) WHERE unit_id IS NOT NULL;
CREATE INDEX sales_recognitions_adjustment_idx ON app.sales_recognitions (adjustment_id) WHERE adjustment_id IS NOT NULL;
-- Ventas por turno (la venta pertenece al turno de la operación que cerró la cuenta).
CREATE INDEX sales_recognitions_shift_idx ON app.sales_recognitions (shift_id, recognized_at) WHERE shift_id IS NOT NULL;

-- El libro es de solo inserción también en la base: UPDATE, DELETE y TRUNCATE se rechazan. Una
-- corrección comercial se expresa como una diferencia en el siguiente cierre de la cuenta.
CREATE FUNCTION app.sales_recognitions_reject_mutation() RETURNS trigger
    LANGUAGE plpgsql AS $$
BEGIN
    RAISE EXCEPTION 'Los registros de % son de solo inserción', TG_TABLE_NAME USING ERRCODE = '55000';
END;
$$;
CREATE TRIGGER sales_recognitions_insert_only
    BEFORE UPDATE OR DELETE ON app.sales_recognitions
    FOR EACH ROW EXECUTE FUNCTION app.sales_recognitions_reject_mutation();
CREATE TRIGGER sales_recognitions_no_truncate
    BEFORE TRUNCATE ON app.sales_recognitions
    FOR EACH STATEMENT EXECUTE FUNCTION app.sales_recognitions_reject_mutation();

-- 2. Índices de fecha de los reportes (transaccionales, sin CONCURRENTLY). Los reportes filtran por
-- instantes [desde, hasta) calculados en Go con la hora de inicio del día.
CREATE INDEX payments_created_idx ON app.payments (created_at);
CREATE INDEX payment_refunds_created_idx ON app.payment_refunds (created_at);
CREATE INDEX account_adjustments_created_idx ON app.account_adjustments (created_at);
CREATE INDEX cash_movements_created_idx ON app.cash_movements (created_at);
CREATE INDEX cash_operations_created_idx ON app.cash_operations (created_at);
CREATE INDEX kitchen_unit_releases_released_idx ON app.kitchen_unit_releases (released_at);
CREATE INDEX kitchen_tasks_released_idx ON app.kitchen_tasks (released_at);
CREATE INDEX restaurant_item_units_cancelled_idx ON app.restaurant_item_units (cancelled_at) WHERE state = 'cancelled';
CREATE INDEX inventory_supplier_returns_occurred_idx ON app.inventory_supplier_returns (occurred_at);
CREATE INDEX payment_authorizations_created_idx ON app.payment_authorizations (created_at);
-- Primer cierre de cada cuenta en la auditoría (relleno inicial y cuentas reabiertas).
CREATE INDEX audit_events_account_closed_idx ON app.audit_events (entity_id, occurred_at)
    WHERE action = 'payments.account_closed';

-- 3. Relleno inicial del libro con las cuentas hoy cerradas (decisión de la fase B). La fecha es el
-- primer payments.account_closed de la auditoría de la cuenta (o closed_at si no existe); el turno y
-- la caja, los del pago registrado en esa misma transacción (sin pago, sin turno); el actor y la
-- terminal, los de esa auditoría; los importes y los nombres, los del estado actual de la cuenta.
-- Limitaciones documentadas: el historial de reaperturas anterior a la 9A no se reconstruye (una
-- cuenta reabierta y vuelta a cerrar queda con sus importes actuales en la fecha de su primer
-- cierre), las cuentas reabiertas que hoy siguen abiertas no se rellenan (aparecen en «En curso» y se
-- reconocerán al cerrarse) y la mesa y el área son las actuales de la cuenta.
-- El cálculo es el mismo de payments/recognition.go (buildRecognitionTargets); la prueba de
-- integración de reportes ejecuta este bloque sobre cuentas cerradas por la API y compara el
-- resultado con lo que escribió Go.
-- reconocimiento:relleno:inicio
INSERT INTO app.sales_recognitions(closing_id, recognition_kind, recognized_at, account_id, account_folio, mode, origin,
    table_id, table_name, area_id, area_name, responsible_user_id, shift_id, register_id, actor_id, device_id,
    source_kind, unit_id, adjustment_id, version_id, order_id, product_id, product_name, product_code, product_kind,
    variant_id, variant_name, category_id, category_name, price_list_id, price_list_name, adjustment_kind, adjustment_scope,
    is_courtesy, reason_id, reason_name, concept, adjustment_actor_id, adjustment_authorized_by, adjustment_created_at,
    tax_rate, quantity, gross, promotion, discount, courtesy, charge, net, base, tax)
WITH closing AS MATERIALIZED (
    SELECT a.id AS account_id, a.folio, a.mode, a.origin, a.table_id, t.name AS table_name, t.area_id, area.name AS area_name,
        a.assigned_user_id, a.price_list_id AS account_price_list_id, list.name AS account_price_list_name,
        coalesce(first_close.occurred_at, a.closed_at) AS recognized_at, first_close.actor_id, first_close.device_id,
        gen_random_uuid() AS closing_id
    FROM app.restaurant_accounts a
    LEFT JOIN app.restaurant_tables t ON t.id = a.table_id
    LEFT JOIN app.restaurant_areas area ON area.id = t.area_id
    LEFT JOIN app.catalog_price_lists list ON list.id = a.price_list_id
    LEFT JOIN LATERAL (
        SELECT e.occurred_at, e.actor_id, e.device_id FROM app.audit_events e
        WHERE e.action = 'payments.account_closed' AND e.entity_id = a.id
        ORDER BY e.occurred_at, e.id LIMIT 1
    ) first_close ON true
    WHERE a.state = 'closed'
),
closing_shift AS (
    SELECT c.account_id, p.shift_id, p.register_id
    FROM closing c
    CROSS JOIN LATERAL (
        SELECT p.shift_id, p.register_id FROM app.payments p
        WHERE p.account_id = c.account_id AND p.created_at = c.recognized_at
        ORDER BY p.id LIMIT 1
    ) p
),
units AS (
    SELECT c.account_id, u.id AS unit_id, u.version_id, v.order_id, i.product_id,
        coalesce(v.quote_line->>'name', '') AS product_name,
        o.pricing_snapshot->'Products'->(i.product_id::text)->>'code' AS product_code,
        o.pricing_snapshot->'Products'->(i.product_id::text)->>'kind' AS product_kind,
        nullif(v.quote_line->>'variant_id', '')::uuid AS variant_id,
        (SELECT variant->>'name' FROM jsonb_array_elements(
                coalesce(o.pricing_snapshot->'Products'->(i.product_id::text)->'variants', '[]'::jsonb)) variant
            WHERE variant->>'id' = v.quote_line->>'variant_id' LIMIT 1) AS variant_name,
        nullif(v.quote_line->>'category_id', '')::uuid AS category_id,
        o.pricing_snapshot->'Categories'->(v.quote_line->>'category_id')->>'name' AS category_name,
        o.price_list_id, o.quote->>'price_list_name' AS price_list_name,
        v.tax_rate, u.subtotal, u.discount AS promotion, u.total
    FROM closing c
    JOIN app.restaurant_item_units u ON u.account_id = c.account_id AND u.state = 'active'
    JOIN app.restaurant_item_versions v ON v.id = u.version_id
    JOIN app.restaurant_items i ON i.id = u.item_id
    JOIN app.restaurant_orders o ON o.id = v.order_id
),
allocations AS (
    SELECT u.*, j.id AS adjustment_id, j.courtesy AS is_courtesy, j.reason_id, j.reason_name, j.concept,
        j.actor_id AS adjustment_actor_id, j.authorized_by AS adjustment_authorized_by, j.created_at AS adjustment_created_at,
        al.amount
    FROM units u
    JOIN app.account_adjustment_allocations al ON al.unit_id = u.unit_id
    JOIN app.account_adjustments j ON j.id = al.adjustment_id AND j.kind = 'discount'
    WHERE NOT EXISTS (SELECT FROM app.account_adjustment_voids x WHERE x.adjustment_id = j.id)
),
general_adjustments AS (
    SELECT c.account_id, j.id AS adjustment_id, j.kind, j.courtesy AS is_courtesy, j.reason_id, j.reason_name, j.concept,
        j.actor_id AS adjustment_actor_id, j.authorized_by AS adjustment_authorized_by, j.created_at AS adjustment_created_at,
        j.tax_rate, j.amount,
        row_number() OVER (PARTITION BY c.account_id, j.kind ORDER BY j.created_at DESC, j.id DESC) = 1 AS is_last
    FROM closing c
    JOIN app.account_adjustments j ON j.account_id = c.account_id AND j.scope = 'account'
    WHERE NOT EXISTS (SELECT FROM app.account_adjustment_voids x WHERE x.adjustment_id = j.id)
),
-- Bruto por cuenta y tasa (finance.go): unidades después de sus descuentos por partida, más cargos.
gross AS (
    SELECT account_id, tax_rate, sum(amount) AS amount
    FROM (
        SELECT u.account_id, u.tax_rate, u.total - coalesce((SELECT sum(a.amount) FROM allocations a WHERE a.unit_id = u.unit_id), 0) AS amount
        FROM units u
        UNION ALL
        SELECT account_id, tax_rate, amount FROM general_adjustments WHERE kind = 'charge'
    ) parts
    GROUP BY account_id, tax_rate
),
-- Pesos del reparto en centavos, en el orden de tasas 16, 8, 0 (payments.allocate: residuo mayor y
-- desempate por posición; con pesos nulos, reparto igualitario).
weights AS (
    SELECT account_id, tax_rate, row_number() OVER (PARTITION BY account_id ORDER BY tax_rate DESC) AS position,
        CASE WHEN sum(amount) OVER (PARTITION BY account_id) = 0 THEN 1 ELSE amount * 100 END AS weight,
        CASE WHEN sum(amount) OVER (PARTITION BY account_id) = 0 THEN count(*) OVER (PARTITION BY account_id)
            ELSE sum(amount * 100) OVER (PARTITION BY account_id) END AS weight_sum
    FROM gross
),
-- Descuento general total de la cuenta repartido entre tasas (igual que finance.go).
account_discount_total AS (
    SELECT account_id, sum(amount) * 100 AS cents FROM general_adjustments WHERE kind = 'discount' GROUP BY account_id
),
account_share_floor AS (
    SELECT d.account_id, w.tax_rate, w.position, d.cents, div(d.cents * w.weight, w.weight_sum) AS floor_cents,
        mod(d.cents * w.weight, w.weight_sum) AS rest
    FROM account_discount_total d JOIN weights w ON w.account_id = d.account_id
),
account_share AS (
    SELECT account_id, tax_rate, floor_cents + CASE
            WHEN row_number() OVER (PARTITION BY account_id ORDER BY rest DESC, position)
                <= cents - sum(floor_cents) OVER (PARTITION BY account_id) THEN 1 ELSE 0 END AS share_cents
    FROM account_share_floor
),
-- Cada descuento general se reparte entre tasas con los mismos pesos; el último registrado recibe lo
-- que falta de cada tasa, de modo que las partes suman el reparto de la cuenta y cada descuento.
discount_floor AS (
    SELECT j.account_id, j.adjustment_id, j.is_last, w.tax_rate, w.position, j.amount * 100 AS cents,
        div(j.amount * 100 * w.weight, w.weight_sum) AS floor_cents, mod(j.amount * 100 * w.weight, w.weight_sum) AS rest
    FROM general_adjustments j JOIN weights w ON w.account_id = j.account_id
    WHERE j.kind = 'discount'
),
discount_split AS (
    SELECT account_id, adjustment_id, is_last, tax_rate, floor_cents + CASE
            WHEN row_number() OVER (PARTITION BY adjustment_id ORDER BY rest DESC, position)
                <= cents - sum(floor_cents) OVER (PARTITION BY adjustment_id) THEN 1 ELSE 0 END AS share_cents
    FROM discount_floor
),
discount_parts AS (
    SELECT d.account_id, d.adjustment_id, d.tax_rate,
        CASE WHEN d.is_last THEN s.share_cents - coalesce((SELECT sum(o.share_cents) FROM discount_split o
                WHERE o.account_id = d.account_id AND o.tax_rate = d.tax_rate AND NOT o.is_last), 0)
            ELSE d.share_cents END / 100.0 AS amount
    FROM discount_split d JOIN account_share s ON s.account_id = d.account_id AND s.tax_rate = d.tax_rate
),
recognition_rows AS (
    SELECT 'unit'::text AS source_kind, u.account_id, u.unit_id, NULL::uuid AS adjustment_id, u.version_id, u.order_id,
        u.product_id, u.product_name, u.product_code, u.product_kind, u.variant_id, u.variant_name, u.category_id,
        u.category_name, u.price_list_id, u.price_list_name, NULL::text AS adjustment_kind, NULL::text AS adjustment_scope,
        false AS is_courtesy, NULL::uuid AS reason_id, NULL::text AS reason_name, NULL::text AS concept,
        NULL::uuid AS adjustment_actor_id, NULL::uuid AS adjustment_authorized_by, NULL::timestamptz AS adjustment_created_at,
        u.tax_rate, 1 AS quantity, u.subtotal AS gross, u.promotion, 0::numeric AS discount, 0::numeric AS courtesy,
        0::numeric AS charge, u.total AS net
    FROM units u
    UNION ALL
    SELECT 'allocation', a.account_id, a.unit_id, a.adjustment_id, a.version_id, a.order_id, a.product_id, a.product_name,
        a.product_code, a.product_kind, a.variant_id, a.variant_name, a.category_id, a.category_name, a.price_list_id,
        a.price_list_name, 'discount', 'items', a.is_courtesy, a.reason_id, a.reason_name, a.concept, a.adjustment_actor_id,
        a.adjustment_authorized_by, a.adjustment_created_at, a.tax_rate, 0, 0, 0,
        CASE WHEN a.is_courtesy THEN 0 ELSE a.amount END, CASE WHEN a.is_courtesy THEN a.amount ELSE 0 END, 0, -a.amount
    FROM allocations a
    UNION ALL
    SELECT 'adjustment', j.account_id, NULL, j.adjustment_id, NULL, NULL, NULL, NULL, NULL, NULL, NULL, NULL, NULL, NULL,
        c.account_price_list_id, c.account_price_list_name, 'charge', 'account', false, NULL, NULL, j.concept,
        j.adjustment_actor_id, j.adjustment_authorized_by, j.adjustment_created_at, j.tax_rate, 0, 0, 0, 0, 0, j.amount, j.amount
    FROM general_adjustments j JOIN closing c ON c.account_id = j.account_id
    WHERE j.kind = 'charge'
    UNION ALL
    SELECT 'adjustment', j.account_id, NULL, j.adjustment_id, NULL, NULL, NULL, NULL, NULL, NULL, NULL, NULL, NULL, NULL,
        c.account_price_list_id, c.account_price_list_name, 'discount', 'account', j.is_courtesy, j.reason_id, j.reason_name,
        j.concept, j.adjustment_actor_id, j.adjustment_authorized_by, j.adjustment_created_at, p.tax_rate, 0, 0, 0,
        CASE WHEN j.is_courtesy THEN 0 ELSE p.amount END, CASE WHEN j.is_courtesy THEN p.amount ELSE 0 END, 0, -p.amount
    FROM discount_parts p
    JOIN general_adjustments j ON j.adjustment_id = p.adjustment_id
    JOIN closing c ON c.account_id = p.account_id
    WHERE p.amount <> 0
),
-- IVA por renglón: provisional con la regla de finance.go y el residuo de centavos de cada cuenta y
-- tasa repartido uno a uno entre sus renglones de mayor importe neto, para que la suma sea el
-- desglose de la cuenta (base = total / (1 + tasa) redondeada; IVA = total − base).
taxed AS (
    SELECT r.*,
        CASE WHEN r.tax_rate = 0 THEN 0
            ELSE r.net - round(round(r.net::numeric(30,12) / ((100 + r.tax_rate)::numeric(30,12) / 100), 8), 2) END AS provisional,
        sum(r.net) OVER (PARTITION BY r.account_id, r.tax_rate) AS rate_net,
        row_number() OVER (PARTITION BY r.account_id, r.tax_rate
            ORDER BY r.net DESC, r.source_kind COLLATE "C", r.unit_id NULLS FIRST, r.adjustment_id NULLS FIRST) AS rank,
        count(*) OVER (PARTITION BY r.account_id, r.tax_rate) AS rate_rows
    FROM recognition_rows r
),
residual AS (
    SELECT t.*, ((CASE WHEN t.tax_rate = 0 THEN 0
            ELSE t.rate_net - round(round(t.rate_net::numeric(30,12) / ((100 + t.tax_rate)::numeric(30,12) / 100), 8), 2) END)
        - sum(t.provisional) OVER (PARTITION BY t.account_id, t.tax_rate)) * 100 AS residual_cents
    FROM taxed t
),
final AS (
    SELECT r.*, r.provisional + sign(r.residual_cents) * (div(abs(r.residual_cents), r.rate_rows)
        + CASE WHEN r.rank <= mod(abs(r.residual_cents), r.rate_rows) THEN 1 ELSE 0 END) / 100.0 AS tax_amount
    FROM residual r
)
SELECT c.closing_id, 'backfill', c.recognized_at, c.account_id, c.folio, c.mode, c.origin, c.table_id, c.table_name, c.area_id,
    c.area_name, c.assigned_user_id, s.shift_id, s.register_id, c.actor_id, c.device_id, f.source_kind, f.unit_id, f.adjustment_id,
    f.version_id, f.order_id, f.product_id, f.product_name, f.product_code, f.product_kind, f.variant_id, f.variant_name,
    f.category_id, f.category_name, f.price_list_id, f.price_list_name, f.adjustment_kind, f.adjustment_scope, f.is_courtesy,
    f.reason_id, f.reason_name, f.concept, f.adjustment_actor_id, f.adjustment_authorized_by, f.adjustment_created_at,
    f.tax_rate, f.quantity, f.gross, f.promotion, f.discount, f.courtesy, f.charge, f.net, f.net - f.tax_amount, f.tax_amount
FROM final f
JOIN closing c ON c.account_id = f.account_id
LEFT JOIN closing_shift s ON s.account_id = f.account_id;
-- reconocimiento:relleno:fin

-- 4. Permisos del módulo Reportes. Se registran también en Go (reports.Permissions) con la misma
-- descripción y se conceden solo al administrador (para que siga contando como administrador
-- completo); ningún otro rol los recibe por inferencia. Los costos, el esperado de caja y la
-- valuación siguen exigiendo además sus permisos de origen (inventory.costs.read,
-- cash.expected.read, inventory.valuation.read).
INSERT INTO app.permissions(key, description) VALUES
    ('reports.read', 'Consultar el módulo Reportes y el resumen ejecutivo'),
    ('reports.sales.read', 'Consultar reportes de ventas, productos, categorías, cocina y operación'),
    ('reports.payments.read', 'Consultar reportes de cobros, devoluciones y propinas'),
    ('reports.discounts.read', 'Consultar reportes de descuentos, cortesías y autorizaciones'),
    ('reports.cash.read', 'Consultar reportes de caja'),
    ('reports.inventory.read', 'Consultar reportes de inventarios'),
    ('reports.purchases.read', 'Consultar reportes de compras y proveedores'),
    ('reports.employees.read', 'Consultar reportes por empleado'),
    ('reports.export.pdf', 'Exportar reportes a PDF'),
    ('reports.export.excel', 'Exportar reportes a Excel');

INSERT INTO app.role_permissions(role_id, permission_key)
SELECT r.id, p.key FROM app.roles r CROSS JOIN app.permissions p
WHERE r.code = 'administrator' AND p.key IN (
    'reports.read', 'reports.sales.read', 'reports.payments.read', 'reports.discounts.read', 'reports.cash.read',
    'reports.inventory.read', 'reports.purchases.read', 'reports.employees.read', 'reports.export.pdf',
    'reports.export.excel'
)
ON CONFLICT DO NOTHING;
