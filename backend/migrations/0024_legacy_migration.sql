-- Etapa 9H: la única migración de la etapa (migración del sistema anterior y ajustes pendientes; esquema 24). Cada fase
-- agrega aquí su propia sección con el encabezado «Fase X». Es transaccional (el migrador aplica todas las pendientes
-- en una sola transacción) y no modifica filas operativas: ventas, pagos, cortes, cuentas, pedidos, sesiones, trabajos
-- de impresión ni sus fotografías.

-- =====================================================================================================================
-- Fase A · Pendientes y ajustes (§2 de la especificación)
-- =====================================================================================================================

-- 1. Sesiones en varios dispositivos (§2.4, decisión del propietario): un usuario puede tener una sesión activa en cada
--    terminal a la vez (por ejemplo, una caja y un teléfono). Desde la 0002, sessions_one_active_user_idx permitía una
--    sola sesión vigente por usuario en todo el sistema e iniciar sesión en una terminal cerraba las de las demás. Ahora
--    el índice es por usuario y terminal: iniciar sesión solo cierra la sesión anterior del usuario en esa misma
--    terminal (administration/auth.go, issueSession). El índice nuevo es más permisivo que el anterior, así que las
--    sesiones existentes lo cumplen; ninguna se cierra ni se modifica.
DROP INDEX app.sessions_one_active_user_idx;
CREATE UNIQUE INDEX sessions_one_active_user_device_idx ON app.sessions (user_id, device_id) WHERE revoked_at IS NULL;

-- =====================================================================================================================
-- Fase C · Historial importado del sistema anterior (§4 de la especificación)
-- =====================================================================================================================
--
-- El historial del sistema anterior (C#/WinForms con SQL Server 2012) vive en tablas propias con el prefijo legacy_,
-- nunca en las operativas (decisión del coordinador; motivos en docs/ETAPA_9H_AVANCE.md, Fase 0, nota técnica 2):
--
--   - Cada importación es un lote (legacy_import_batches). Todas las tablas del historial llevan batch_id, el
--     identificador y el folio originales, los importes numeric(19,4) exactamente como los guardó el anterior (MONEY,
--     4 decimales; nunca se recalculan) y, donde aplica, marcas de incoherencia (flags, de la lista cerrada de
--     internal/legacyimport/flags.go y docs/MIGRACION_ANALISIS.md §6) con una nota.
--   - Los nombres (producto, categoría, mesa, usuario, caja) se copian: el historial no depende del catálogo vivo.
--     legacy_id_map guarda la correspondencia de lo importado al catálogo, a las mesas y a los usuarios.
--   - legacy_sales_recognitions es el libro del historial para los reportes, con las mismas medidas que
--     app.sales_recognitions (0017): la suma por venta es exactamente lo que se debía según el anterior
--     (total + montoExtra − descuentos; decisión 2 del propietario).
--   - Solo lectura por construcción: ninguna ruta ni módulo existente las escribe, ninguna tabla operativa tiene llaves
--     hacia ellas y un disparador por sentencia rechaza INSERT, UPDATE, DELETE y TRUNCATE salvo dentro de la
--     transacción de la importación (que fija adel.legacy_import con el identificador de su lote en curso).
--   - Fechas: el anterior guardaba DATETIME sin zona con la hora local del restaurante; la importación las interpreta
--     con establishment.timezone (la que guarda el lote) y aquí quedan como instantes.
--
-- legacy_import_runs (las corridas de análisis e importación con su avance) no es historial: la escribe el trabajador
-- de la migración en transacciones cortas, fuera de la protección.

-- 1. Protección del historial. La importación fija set_config('adel.legacy_import', '<lote>', true) (local a su
--    transacción). Para crear su lote (INSERT en legacy_import_batches) basta un identificador válido: el lote nace en
--    esa misma transacción; cualquier otra escritura exige además que ese lote exista y esté «importing». Es por
--    sentencia: el COPY de cientos de miles de filas no paga un disparador por fila.
CREATE FUNCTION app.legacy_history_guard() RETURNS trigger
    LANGUAGE plpgsql AS $$
DECLARE
    current_batch text := coalesce(current_setting('adel.legacy_import', true), '');
BEGIN
    IF current_batch !~ '^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$' THEN
        RAISE EXCEPTION 'El historial del sistema anterior es de solo lectura (%)', TG_TABLE_NAME USING ERRCODE = '55000';
    END IF;
    IF NOT (TG_TABLE_NAME = 'legacy_import_batches' AND TG_OP = 'INSERT') AND NOT EXISTS (
        SELECT 1 FROM app.legacy_import_batches WHERE id = current_batch::uuid AND status = 'importing'
    ) THEN
        RAISE EXCEPTION 'El historial del sistema anterior solo se escribe al importar un lote (%)', TG_TABLE_NAME
            USING ERRCODE = '55000';
    END IF;
    RETURN NULL;
END;
$$;

-- 2. Lotes. status: importing solo existe dentro de la transacción de la importación (se confirma como completed);
--    replaced es un lote que otro reemplazó completo (sus filas del historial se borraron en esa misma transacción y
--    aquí queda su registro). Un solo lote vigente y uno en curso a la vez.
CREATE TABLE app.legacy_import_batches (
    id uuid CONSTRAINT legacy_import_batches_pkey PRIMARY KEY,
    seq bigint GENERATED ALWAYS AS IDENTITY CONSTRAINT legacy_import_batches_seq_key UNIQUE,
    status text NOT NULL CONSTRAINT legacy_import_batches_status_check
        CHECK (status IN ('importing', 'completed', 'replaced')),
    run_id uuid,
    -- Paquete: huella de contenido del manifiesto, SHA-256 del archivo, versión del formato y quién lo creó.
    package_fingerprint text NOT NULL CONSTRAINT legacy_import_batches_fingerprint_check
        CHECK (package_fingerprint ~ '^[0-9a-f]{64}$'),
    package_sha256 text CONSTRAINT legacy_import_batches_sha256_check CHECK (package_sha256 ~ '^[0-9a-f]{64}$'),
    package_bytes bigint,
    format_version integer NOT NULL,
    package_created_at timestamptz,
    package_tool text NOT NULL DEFAULT '',
    package_product_version text NOT NULL DEFAULT '',
    -- Respaldo de origen: fecha (hora local del equipo de origen interpretada con timezone) y datos técnicos del
    -- manifiesto (nombre de la base, versión de SQL Server, imagen del lector; nunca datos del restaurante).
    backup_taken_at timestamptz,
    source jsonb NOT NULL DEFAULT '{}',
    timezone text NOT NULL,
    imported_by uuid,
    imported_by_username text NOT NULL DEFAULT '',
    imported_device uuid,
    started_at timestamptz NOT NULL,
    completed_at timestamptz,
    replaced_at timestamptz,
    replaced_by uuid,
    sales_first_at timestamptz,
    sales_last_at timestamptz,
    -- counts: filas escritas por tabla; validation: la comparación de §4.4; report: el informe final (conteos,
    -- advertencias, marcas y tiempos; la lista detallada de datos incoherentes sale de las tablas, en el PDF).
    counts jsonb NOT NULL DEFAULT '{}',
    validation jsonb,
    report jsonb,
    has_differences boolean NOT NULL DEFAULT false,
    CONSTRAINT legacy_import_batches_status_fields_check CHECK (
        (status = 'importing' AND completed_at IS NULL AND replaced_at IS NULL)
        OR (status = 'completed' AND completed_at IS NOT NULL AND validation IS NOT NULL AND replaced_at IS NULL)
        OR (status = 'replaced' AND completed_at IS NOT NULL AND replaced_at IS NOT NULL)
    )
);
CREATE UNIQUE INDEX legacy_import_batches_current_idx ON app.legacy_import_batches ((true)) WHERE status = 'completed';
CREATE UNIQUE INDEX legacy_import_batches_importing_idx ON app.legacy_import_batches ((true)) WHERE status = 'importing';

-- 3. Correspondencia de lo importado a lo vivo (catálogo, mesas y usuarios). legacy_key es el identificador anterior
--    (o, para los grupos de complementos que no existían en el anterior, el conjunto de complementos que los forma).
--    state retired: algo de un lote anterior que ya no viene en el paquete y no se pudo borrar (quedó desactivado).
--    Sin llaves hacia las tablas vivas: el historial no depende de ellas.
CREATE TABLE app.legacy_id_map (
    batch_id uuid NOT NULL CONSTRAINT legacy_id_map_batch_id_fkey REFERENCES app.legacy_import_batches(id),
    entity text NOT NULL CONSTRAINT legacy_id_map_entity_check CHECK (entity IN (
        'category', 'product', 'complement', 'option_group', 'group_option', 'area', 'table', 'user')),
    legacy_key text NOT NULL CONSTRAINT legacy_id_map_legacy_key_check CHECK (length(legacy_key) BETWEEN 1 AND 400),
    new_id uuid NOT NULL,
    state text NOT NULL DEFAULT 'active' CONSTRAINT legacy_id_map_state_check CHECK (state IN ('active', 'retired')),
    note text NOT NULL DEFAULT '',
    CONSTRAINT legacy_id_map_pkey PRIMARY KEY (batch_id, entity, legacy_key),
    CONSTRAINT legacy_id_map_new_id_key UNIQUE (batch_id, entity, new_id)
);
CREATE INDEX legacy_id_map_new_id_idx ON app.legacy_id_map (entity, new_id);

-- 4. Ventas (ventas). status: paid (pagado = 1 sin cancelación), cancelled (fila en cancelaciones; no son venta,
--    regla de la 9D) u open (sin pagar al hacer el respaldo; historial, no venta). recognized_at solo en las pagadas
--    (decisión del coordinador, precisada en la revisión): la fecha del pago con el que lo acumulado de los pagos
--    mayores que cero (por fecha y luego por id) alcanza lo que se debía; si nunca lo alcanza, la del último pago mayor
--    que cero; sin pagos mayores que cero, la de apertura. last_payment_at es el último pago de cualquier importe.
--    amount_due = total + montoExtra − Σ descuentos («por cobrar» del anterior, PagarEfectivo.cs:205);
--    lines_amount = Σ renglones + Σ complementos. Los valores originales de pagado, enmesa, facturado y cancelado se
--    conservan tal cual (cancelado nunca se escribió en el anterior).
CREATE TABLE app.legacy_sales (
    batch_id uuid NOT NULL CONSTRAINT legacy_sales_batch_id_fkey REFERENCES app.legacy_import_batches(id),
    legacy_id integer NOT NULL,
    folio text NOT NULL,
    status text NOT NULL CONSTRAINT legacy_sales_status_check CHECK (status IN ('paid', 'cancelled', 'open')),
    opened_at timestamptz NOT NULL,
    recognized_at timestamptz,
    last_payment_at timestamptz,
    cancelled_at timestamptz,
    mode text NOT NULL CONSTRAINT legacy_sales_mode_check CHECK (mode IN ('dine_in', 'takeaway')),
    legacy_table_id integer,
    table_id uuid,
    table_name text,
    area_id uuid,
    area_name text,
    legacy_register_id integer,
    register_name text,
    legacy_cashier_id integer,
    cashier_user_id uuid,
    cashier_name text,
    legacy_shift_id integer,
    account_name text NOT NULL DEFAULT '',
    subtotal numeric(19,4) NOT NULL,
    tax numeric(19,4) NOT NULL,
    total numeric(19,4) NOT NULL,
    amount_paid numeric(19,4),
    paid_flag smallint,
    in_table_flag smallint,
    invoiced_flag smallint,
    cancelled_flag smallint,
    charge_concept text NOT NULL DEFAULT '',
    charge_amount numeric(19,4),
    discount_amount numeric(19,4) NOT NULL DEFAULT 0,
    amount_due numeric(19,4) NOT NULL,
    lines_amount numeric(19,4) NOT NULL,
    lines_count integer NOT NULL,
    payments_amount numeric(19,4) NOT NULL,
    payments_count integer NOT NULL,
    tax_estimated boolean NOT NULL DEFAULT false,
    flags text[] NOT NULL DEFAULT '{}',
    note text NOT NULL DEFAULT '',
    CONSTRAINT legacy_sales_pkey PRIMARY KEY (batch_id, legacy_id),
    CONSTRAINT legacy_sales_recognized_check CHECK ((status = 'paid') = (recognized_at IS NOT NULL)),
    CONSTRAINT legacy_sales_cancelled_check CHECK ((status = 'cancelled') = (cancelled_at IS NOT NULL)),
    CONSTRAINT legacy_sales_amount_due_check CHECK (amount_due = total + coalesce(charge_amount, 0) - discount_amount)
);
CREATE INDEX legacy_sales_recognized_idx ON app.legacy_sales (recognized_at) WHERE status = 'paid';
CREATE INDEX legacy_sales_opened_idx ON app.legacy_sales (opened_at);
CREATE INDEX legacy_sales_flags_idx ON app.legacy_sales (batch_id) WHERE flags <> '{}';

-- 5. Renglones (ventas_productos): un renglón por renglón del anterior, con la cantidad actual y la original, sus
--    importes tal como se guardaron, la tasa deducida (iva/subtotal, la más cercana de 0, 8 o 16) y la suma de sus
--    complementos. El producto, su código y su categoría se copian; product_id es el del catálogo importado.
CREATE TABLE app.legacy_sale_lines (
    batch_id uuid NOT NULL CONSTRAINT legacy_sale_lines_batch_id_fkey REFERENCES app.legacy_import_batches(id),
    legacy_id integer NOT NULL,
    legacy_sale_id integer NOT NULL,
    legacy_product_id integer NOT NULL,
    product_id uuid,
    product_code text,
    product_name text NOT NULL,
    legacy_category_id integer,
    category_id uuid,
    category_name text,
    quantity numeric(19,3) NOT NULL,
    original_quantity numeric(19,3),
    subtotal numeric(19,4) NOT NULL,
    tax numeric(19,4) NOT NULL,
    total numeric(19,4) NOT NULL,
    tax_rate integer CONSTRAINT legacy_sale_lines_tax_rate_check CHECK (tax_rate IN (0, 8, 16)),
    tax_estimated boolean NOT NULL DEFAULT false,
    complements_count integer NOT NULL DEFAULT 0,
    complements_subtotal numeric(19,4) NOT NULL DEFAULT 0,
    complements_tax numeric(19,4) NOT NULL DEFAULT 0,
    complements_total numeric(19,4) NOT NULL DEFAULT 0,
    notes text NOT NULL DEFAULT '',
    flags text[] NOT NULL DEFAULT '{}',
    CONSTRAINT legacy_sale_lines_pkey PRIMARY KEY (batch_id, legacy_id)
);
CREATE INDEX legacy_sale_lines_sale_idx ON app.legacy_sale_lines (batch_id, legacy_sale_id);

-- 6. Complementos de cada renglón (ventas_complementos, sin llave en el anterior: position es su orden en el
--    paquete). included y requested por unidad del renglón; el importe ya es (pedido − incluido) × precio × cantidad
--    del renglón, con signo (un complemento incluido que se quitó se bonificaba: importe negativo).
CREATE TABLE app.legacy_line_complements (
    batch_id uuid NOT NULL CONSTRAINT legacy_line_complements_batch_id_fkey REFERENCES app.legacy_import_batches(id),
    position integer NOT NULL,
    legacy_line_id integer NOT NULL,
    legacy_sale_id integer,
    legacy_complement_id integer NOT NULL,
    complement_id uuid,
    complement_name text NOT NULL,
    included numeric(19,3),
    requested numeric(19,3),
    subtotal numeric(19,4) NOT NULL,
    tax numeric(19,4) NOT NULL,
    total numeric(19,4) NOT NULL,
    tax_estimated boolean NOT NULL DEFAULT false,
    flags text[] NOT NULL DEFAULT '{}',
    CONSTRAINT legacy_line_complements_pkey PRIMARY KEY (batch_id, position)
);
CREATE INDEX legacy_line_complements_line_idx ON app.legacy_line_complements (batch_id, legacy_line_id);

-- 7. Cambios de cantidad (ventas_modificaciones): original_quantity es cantAnt (la cantidad original del renglón,
--    no la anterior al cambio) y new_quantity cantNueva; quién autorizó (el anterior no guardaba quién modificó).
CREATE TABLE app.legacy_quantity_changes (
    batch_id uuid NOT NULL CONSTRAINT legacy_quantity_changes_batch_id_fkey REFERENCES app.legacy_import_batches(id),
    position integer NOT NULL,
    legacy_line_id integer NOT NULL,
    legacy_sale_id integer,
    original_quantity numeric(19,3),
    new_quantity numeric(19,3),
    legacy_authorized_by integer,
    authorized_user_id uuid,
    authorized_name text,
    changed_at timestamptz,
    CONSTRAINT legacy_quantity_changes_pkey PRIMARY KEY (batch_id, position)
);
CREATE INDEX legacy_quantity_changes_sale_idx ON app.legacy_quantity_changes (batch_id, legacy_sale_id);

-- 8. Descuentos de cuenta (descuentos con su tipo de tipos_descuentos). Los cargos extra son ventas.extra y
--    ventas.montoExtra (legacy_sales.charge_concept y charge_amount).
CREATE TABLE app.legacy_discounts (
    batch_id uuid NOT NULL CONSTRAINT legacy_discounts_batch_id_fkey REFERENCES app.legacy_import_batches(id),
    position integer NOT NULL,
    legacy_sale_id integer NOT NULL,
    legacy_type_id integer,
    type_name text,
    description text NOT NULL DEFAULT '',
    amount numeric(19,4) NOT NULL,
    flags text[] NOT NULL DEFAULT '{}',
    CONSTRAINT legacy_discounts_pkey PRIMARY KEY (batch_id, position)
);
CREATE INDEX legacy_discounts_sale_idx ON app.legacy_discounts (batch_id, legacy_sale_id);

-- 9. Cancelaciones de cuenta (cancelaciones): quién autorizó y cuándo; el anterior no guardaba motivo ni quién canceló.
CREATE TABLE app.legacy_cancellations (
    batch_id uuid NOT NULL CONSTRAINT legacy_cancellations_batch_id_fkey REFERENCES app.legacy_import_batches(id),
    legacy_id integer NOT NULL,
    legacy_sale_id integer NOT NULL,
    legacy_authorized_by integer,
    authorized_user_id uuid,
    authorized_name text,
    cancelled_at timestamptz NOT NULL,
    CONSTRAINT legacy_cancellations_pkey PRIMARY KEY (batch_id, legacy_id)
);
CREATE INDEX legacy_cancellations_sale_idx ON app.legacy_cancellations (batch_id, legacy_sale_id);

-- 10. Pagos (pagos). method: cash («Efectivo»), card («Tarjeta (sistema anterior)», forma de pago propia del
--     historial, sin método nuevo en Cobros) u other (un tipo que el anterior no tenía; conserva su nombre). Importes
--     tal como estaban, también en cero o negativos (con su marca). reference es pagos.folio (el comprobante de la
--     tarjeta). Sin moneda ni tipo de cambio: el anterior no los guardaba (los dólares quedaban como efectivo).
CREATE TABLE app.legacy_payments (
    batch_id uuid NOT NULL CONSTRAINT legacy_payments_batch_id_fkey REFERENCES app.legacy_import_batches(id),
    legacy_id integer NOT NULL,
    legacy_sale_id integer NOT NULL,
    legacy_method_id integer,
    method text NOT NULL CONSTRAINT legacy_payments_method_check CHECK (method IN ('cash', 'card', 'other')),
    method_name text NOT NULL,
    reference text NOT NULL DEFAULT '',
    amount numeric(19,4) NOT NULL,
    legacy_received_by integer,
    received_user_id uuid,
    received_name text,
    paid_at timestamptz NOT NULL,
    legacy_shift_id integer,
    sale_status text NOT NULL CONSTRAINT legacy_payments_sale_status_check CHECK (sale_status IN ('paid', 'cancelled', 'open')),
    flags text[] NOT NULL DEFAULT '{}',
    CONSTRAINT legacy_payments_pkey PRIMARY KEY (batch_id, legacy_id)
);
CREATE INDEX legacy_payments_paid_idx ON app.legacy_payments (paid_at);
CREATE INDEX legacy_payments_sale_idx ON app.legacy_payments (batch_id, legacy_sale_id);
CREATE INDEX legacy_payments_shift_idx ON app.legacy_payments (legacy_shift_id, paid_at) WHERE legacy_shift_id IS NOT NULL;

-- 11. Cortes (turnos de un cajero en una caja) tal como se guardaron: fondo, venta, retiros, conteo final y diferencia
--     del anterior (su «venta» incluía tarjeta y canceladas). No se convierten en cash_shifts ni se recalculan.
--     opened_at y closed_at son los del anterior; sin fecha_inicio (columna con nulos), la de la primera venta o pago
--     del corte (marca start_estimated) y, cerrado sin fecha_fin, la de la última (end_estimated); sin ventas ni pagos,
--     queda nula (no_start).
CREATE TABLE app.legacy_cash_shifts (
    batch_id uuid NOT NULL CONSTRAINT legacy_cash_shifts_batch_id_fkey REFERENCES app.legacy_import_batches(id),
    legacy_id integer NOT NULL,
    legacy_register_id integer,
    register_name text,
    legacy_cashier_id integer,
    cashier_user_id uuid,
    cashier_name text,
    status text NOT NULL CONSTRAINT legacy_cash_shifts_status_check CHECK (status IN ('open', 'closed', 'unknown')),
    legacy_status text,
    opening_cash numeric(19,4),
    closing_cash numeric(19,4),
    sales_amount numeric(19,4),
    withdrawals_amount numeric(19,4),
    difference numeric(19,4),
    legacy_authorized_by integer,
    authorized_user_id uuid,
    authorized_name text,
    opened_at timestamptz,
    closed_at timestamptz,
    flags text[] NOT NULL DEFAULT '{}',
    CONSTRAINT legacy_cash_shifts_pkey PRIMARY KEY (batch_id, legacy_id)
);
CREATE INDEX legacy_cash_shifts_opened_idx ON app.legacy_cash_shifts (opened_at);

-- 12. Retiros de efectivo del turno (cortes_parciales): importe, razón (texto libre), fecha y quién autorizó. Son
--     retiros, no gastos (decisión del coordinador); el anterior no tenía egresos (salidas y entradas vacías).
CREATE TABLE app.legacy_cash_withdrawals (
    batch_id uuid NOT NULL CONSTRAINT legacy_cash_withdrawals_batch_id_fkey REFERENCES app.legacy_import_batches(id),
    legacy_id integer NOT NULL,
    legacy_shift_id integer,
    amount numeric(19,4) NOT NULL,
    reason text NOT NULL DEFAULT '',
    withdrawn_at timestamptz,
    legacy_authorized_by integer,
    authorized_user_id uuid,
    authorized_name text,
    flags text[] NOT NULL DEFAULT '{}',
    CONSTRAINT legacy_cash_withdrawals_pkey PRIMARY KEY (batch_id, legacy_id)
);
CREATE INDEX legacy_cash_withdrawals_shift_idx ON app.legacy_cash_withdrawals (legacy_shift_id);

-- 12 b. Salidas de efectivo del turno (salidas, sin llave en el anterior: position es su orden en el paquete). En el
--     respaldo real está vacía (el anterior no tenía egresos); si un paquete las trae, se conservan y forman el «Total de
--     egresos» de la validación.
CREATE TABLE app.legacy_cash_expenses (
    batch_id uuid NOT NULL CONSTRAINT legacy_cash_expenses_batch_id_fkey REFERENCES app.legacy_import_batches(id),
    position integer NOT NULL,
    legacy_shift_id integer,
    amount numeric(19,4) NOT NULL,
    spent_at timestamptz,
    legacy_authorized_by integer,
    authorized_user_id uuid,
    authorized_name text,
    flags text[] NOT NULL DEFAULT '{}',
    CONSTRAINT legacy_cash_expenses_pkey PRIMARY KEY (batch_id, position)
);

-- 13. Historial de precios (productos_precios y complementos_precios, sin llave en el anterior) con sus vigencias. El
--     catálogo importado solo tiene el precio más reciente; aquí queda todo para consulta.
CREATE TABLE app.legacy_price_history (
    batch_id uuid NOT NULL CONSTRAINT legacy_price_history_batch_id_fkey REFERENCES app.legacy_import_batches(id),
    position integer NOT NULL,
    item_kind text NOT NULL CONSTRAINT legacy_price_history_item_kind_check CHECK (item_kind IN ('product', 'complement')),
    legacy_item_id integer NOT NULL,
    item_id uuid,
    item_name text NOT NULL,
    starts_at timestamptz NOT NULL,
    valid_until timestamptz,
    subtotal numeric(19,4) NOT NULL,
    tax numeric(19,4) NOT NULL,
    total numeric(19,4) NOT NULL,
    tax_rate integer CONSTRAINT legacy_price_history_tax_rate_check CHECK (tax_rate IN (0, 8, 16)),
    flags text[] NOT NULL DEFAULT '{}',
    CONSTRAINT legacy_price_history_pkey PRIMARY KEY (batch_id, position)
);
CREATE INDEX legacy_price_history_item_idx ON app.legacy_price_history (batch_id, item_kind, legacy_item_id, starts_at);

-- 14. Libro del historial para los reportes: las mismas medidas que app.sales_recognitions (quantity, gross,
--     discount, charge, net, base y tax; sin promociones ni cortesías, que el anterior no tenía) y sus dimensiones
--     (instante de reconocimiento, modalidad, mesa, área, categoría, producto, turno y cuenta). Solo las ventas pagadas
--     no canceladas, con recognized_at = legacy_sales.recognized_at:
--       - line: un renglón por renglón del anterior, con sus complementos sumados (gross = total del renglón + Σ total
--         de sus complementos; tax = Σ IVA guardado; base = net − tax);
--       - discount: un renglón por descuento (discount = importe; net = −importe; sin IVA, como en el anterior);
--       - charge: el cargo extra de la venta (charge = montoExtra; sin IVA);
--       - difference: «Diferencia del sistema anterior» cuando el total o el IVA guardados de la venta no cuadran con
--         sus renglones (decisión 2 del propietario): net = total − Σ renglones, tax = iva − Σ IVA de renglones.
--     Σ net por venta = legacy_sales.amount_due y Σ tax por venta = legacy_sales.tax, exactos.
CREATE TABLE app.legacy_sales_recognitions (
    sequence bigint GENERATED ALWAYS AS IDENTITY CONSTRAINT legacy_sales_recognitions_pkey PRIMARY KEY,
    batch_id uuid NOT NULL CONSTRAINT legacy_sales_recognitions_batch_id_fkey REFERENCES app.legacy_import_batches(id),
    entry_kind text NOT NULL CONSTRAINT legacy_sales_recognitions_entry_kind_check
        CHECK (entry_kind IN ('line', 'discount', 'charge', 'difference')),
    legacy_sale_id integer NOT NULL,
    sale_folio text NOT NULL,
    legacy_line_id integer,
    recognized_at timestamptz NOT NULL,
    opened_at timestamptz NOT NULL,
    mode text NOT NULL CONSTRAINT legacy_sales_recognitions_mode_check CHECK (mode IN ('dine_in', 'takeaway')),
    legacy_table_id integer,
    table_id uuid,
    table_name text,
    area_id uuid,
    area_name text,
    legacy_register_id integer,
    register_name text,
    legacy_shift_id integer,
    legacy_cashier_id integer,
    responsible_user_id uuid,
    responsible_name text,
    legacy_product_id integer,
    product_id uuid,
    product_code text,
    product_name text,
    legacy_category_id integer,
    category_id uuid,
    category_name text,
    concept text,
    tax_rate integer CONSTRAINT legacy_sales_recognitions_tax_rate_check CHECK (tax_rate IN (0, 8, 16)),
    tax_estimated boolean NOT NULL DEFAULT false,
    quantity numeric(19,3) NOT NULL,
    gross numeric(19,4) NOT NULL,
    discount numeric(19,4) NOT NULL,
    charge numeric(19,4) NOT NULL,
    net numeric(19,4) NOT NULL,
    base numeric(19,4) NOT NULL,
    tax numeric(19,4) NOT NULL,
    CONSTRAINT legacy_sales_recognitions_net_check CHECK (net = gross - discount + charge),
    CONSTRAINT legacy_sales_recognitions_tax_check CHECK (net = base + tax),
    CONSTRAINT legacy_sales_recognitions_kind_check CHECK (
        (entry_kind = 'line' AND legacy_line_id IS NOT NULL AND discount = 0 AND charge = 0)
        OR (entry_kind = 'discount' AND legacy_line_id IS NULL AND quantity = 0 AND gross = 0 AND charge = 0)
        OR (entry_kind = 'charge' AND legacy_line_id IS NULL AND quantity = 0 AND gross = 0 AND discount = 0)
        OR (entry_kind = 'difference' AND legacy_line_id IS NULL AND quantity = 0 AND discount = 0 AND charge = 0)
    )
);
-- Filtro por periodo (como sales_recognitions_recognized_idx), por venta, por turno y por producto.
CREATE INDEX legacy_sales_recognitions_recognized_idx ON app.legacy_sales_recognitions (recognized_at, sequence);
CREATE INDEX legacy_sales_recognitions_sale_idx ON app.legacy_sales_recognitions (batch_id, legacy_sale_id);
CREATE INDEX legacy_sales_recognitions_shift_idx ON app.legacy_sales_recognitions (legacy_shift_id, recognized_at)
    WHERE legacy_shift_id IS NOT NULL;
CREATE INDEX legacy_sales_recognitions_product_idx ON app.legacy_sales_recognitions (product_id, recognized_at)
    WHERE product_id IS NOT NULL;

-- 15. Disparadores de protección en todas las tablas del historial (una sola función, por sentencia).
CREATE TRIGGER legacy_import_batches_guard BEFORE INSERT OR UPDATE OR DELETE OR TRUNCATE ON app.legacy_import_batches
    FOR EACH STATEMENT EXECUTE FUNCTION app.legacy_history_guard();
CREATE TRIGGER legacy_id_map_guard BEFORE INSERT OR UPDATE OR DELETE OR TRUNCATE ON app.legacy_id_map
    FOR EACH STATEMENT EXECUTE FUNCTION app.legacy_history_guard();
CREATE TRIGGER legacy_sales_guard BEFORE INSERT OR UPDATE OR DELETE OR TRUNCATE ON app.legacy_sales
    FOR EACH STATEMENT EXECUTE FUNCTION app.legacy_history_guard();
CREATE TRIGGER legacy_sale_lines_guard BEFORE INSERT OR UPDATE OR DELETE OR TRUNCATE ON app.legacy_sale_lines
    FOR EACH STATEMENT EXECUTE FUNCTION app.legacy_history_guard();
CREATE TRIGGER legacy_line_complements_guard BEFORE INSERT OR UPDATE OR DELETE OR TRUNCATE ON app.legacy_line_complements
    FOR EACH STATEMENT EXECUTE FUNCTION app.legacy_history_guard();
CREATE TRIGGER legacy_quantity_changes_guard BEFORE INSERT OR UPDATE OR DELETE OR TRUNCATE ON app.legacy_quantity_changes
    FOR EACH STATEMENT EXECUTE FUNCTION app.legacy_history_guard();
CREATE TRIGGER legacy_discounts_guard BEFORE INSERT OR UPDATE OR DELETE OR TRUNCATE ON app.legacy_discounts
    FOR EACH STATEMENT EXECUTE FUNCTION app.legacy_history_guard();
CREATE TRIGGER legacy_cancellations_guard BEFORE INSERT OR UPDATE OR DELETE OR TRUNCATE ON app.legacy_cancellations
    FOR EACH STATEMENT EXECUTE FUNCTION app.legacy_history_guard();
CREATE TRIGGER legacy_payments_guard BEFORE INSERT OR UPDATE OR DELETE OR TRUNCATE ON app.legacy_payments
    FOR EACH STATEMENT EXECUTE FUNCTION app.legacy_history_guard();
CREATE TRIGGER legacy_cash_shifts_guard BEFORE INSERT OR UPDATE OR DELETE OR TRUNCATE ON app.legacy_cash_shifts
    FOR EACH STATEMENT EXECUTE FUNCTION app.legacy_history_guard();
CREATE TRIGGER legacy_cash_withdrawals_guard BEFORE INSERT OR UPDATE OR DELETE OR TRUNCATE ON app.legacy_cash_withdrawals
    FOR EACH STATEMENT EXECUTE FUNCTION app.legacy_history_guard();
CREATE TRIGGER legacy_cash_expenses_guard BEFORE INSERT OR UPDATE OR DELETE OR TRUNCATE ON app.legacy_cash_expenses
    FOR EACH STATEMENT EXECUTE FUNCTION app.legacy_history_guard();
CREATE TRIGGER legacy_price_history_guard BEFORE INSERT OR UPDATE OR DELETE OR TRUNCATE ON app.legacy_price_history
    FOR EACH STATEMENT EXECUTE FUNCTION app.legacy_history_guard();
CREATE TRIGGER legacy_sales_recognitions_guard BEFORE INSERT OR UPDATE OR DELETE OR TRUNCATE ON app.legacy_sales_recognitions
    FOR EACH STATEMENT EXECUTE FUNCTION app.legacy_history_guard();

-- 16. Corridas de la migración (análisis e importación) con su avance, para el asistente (Fase D). Las escribe el
--     trabajador de la migración en transacciones cortas (no es historial). Una sola en curso a la vez (índice
--     parcial; además, el lock consultivo 7820148310 en el trabajador). Al arrancar el servidor, una corrida que quedó
--     en curso pasa a interrupted (su transacción ya se deshizo: sin datos a medias). La frase de paso nunca se guarda.
CREATE TABLE app.legacy_import_runs (
    id uuid CONSTRAINT legacy_import_runs_pkey PRIMARY KEY,
    seq bigint GENERATED ALWAYS AS IDENTITY CONSTRAINT legacy_import_runs_seq_key UNIQUE,
    kind text NOT NULL CONSTRAINT legacy_import_runs_kind_check CHECK (kind IN ('analyze', 'import')),
    status text NOT NULL CONSTRAINT legacy_import_runs_status_check
        CHECK (status IN ('queued', 'running', 'succeeded', 'failed', 'interrupted')),
    phase text NOT NULL DEFAULT 'queued',
    progress jsonb NOT NULL DEFAULT '{}',
    upload_id uuid NOT NULL,
    package_sha256 text NOT NULL CONSTRAINT legacy_import_runs_sha256_check CHECK (package_sha256 ~ '^[0-9a-f]{64}$'),
    package_bytes bigint NOT NULL CONSTRAINT legacy_import_runs_bytes_check CHECK (package_bytes > 0),
    analysis_run_id uuid CONSTRAINT legacy_import_runs_analysis_fkey REFERENCES app.legacy_import_runs(id),
    replace_batch_id uuid,
    batch_id uuid,
    backup_run_id uuid,
    backup_file text,
    requested_by uuid NOT NULL CONSTRAINT legacy_import_runs_requested_by_fkey REFERENCES app.users(id),
    requested_device uuid NOT NULL CONSTRAINT legacy_import_runs_requested_device_fkey REFERENCES app.devices(id),
    summary jsonb,
    error_code text,
    error_message text,
    with_differences boolean NOT NULL DEFAULT false,
    timings jsonb NOT NULL DEFAULT '{}',
    created_at timestamptz NOT NULL DEFAULT now(),
    started_at timestamptz,
    finished_at timestamptz,
    updated_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT legacy_import_runs_status_fields_check CHECK (
        (status = 'queued' AND started_at IS NULL AND finished_at IS NULL AND error_code IS NULL)
        OR (status = 'running' AND started_at IS NOT NULL AND finished_at IS NULL AND error_code IS NULL)
        OR (status = 'succeeded' AND finished_at IS NOT NULL AND error_code IS NULL AND summary IS NOT NULL)
        OR (status IN ('failed', 'interrupted') AND finished_at IS NOT NULL AND error_code IS NOT NULL)
    ),
    CONSTRAINT legacy_import_runs_import_fields_check CHECK (
        kind = 'import' OR (analysis_run_id IS NULL AND replace_batch_id IS NULL AND batch_id IS NULL
            AND backup_run_id IS NULL)
    )
);
CREATE UNIQUE INDEX legacy_import_runs_active_idx ON app.legacy_import_runs ((true)) WHERE status IN ('queued', 'running');
COMMENT ON TABLE app.legacy_import_batches IS
    'Lotes importados del sistema anterior: paquete, respaldo de origen, quién y cuándo, conteos, validación e informe (etapa 9H, fase C)';
COMMENT ON TABLE app.legacy_sales_recognitions IS
    'Libro del historial del sistema anterior para los reportes, con las medidas de sales_recognitions (etapa 9H, fase C)';
COMMENT ON TABLE app.legacy_import_runs IS
    'Corridas de análisis e importación del paquete del sistema anterior con su avance (etapa 9H, fase C)';
