-- Etapa 7: administración de caja, arqueos, movimientos de efectivo y cortes.
-- Amplía el turno mínimo de la etapa 6 sin reconstruir tablas financieras ni alterar filas
-- históricas: pagos, devoluciones y movimientos de cobro quedan intactos. Los ingresos, retiros,
-- gastos, correcciones y regularizaciones son registros nuevos e inmutables; los arqueos, cortes y
-- cierres guardan su resultado histórico. Todas las restricciones nuevas tienen nombre explícito y,
-- cuando intervienen columnas opcionales, exigen IS NOT NULL en la rama que valida datos (un CHECK
-- que evalúa a NULL se acepta en PostgreSQL). Solo se siembra configuración: ningún dato ficticio.

-- 1. Turnos: estado «Cierre en curso», fondo en dólares, observaciones, fecha comercial y cierre.
-- El CHECK del estado se creó sin nombre en la 0006; su nombre real verificado es cash_shifts_state_check.
ALTER TABLE app.cash_shifts DROP CONSTRAINT cash_shifts_state_check;
ALTER TABLE app.cash_shifts
    ADD CONSTRAINT cash_shifts_state_check CHECK (state IN ('open', 'closing', 'closed'));

ALTER TABLE app.cash_shifts
    -- Fondo inicial en dólares; NULL significa que el turno no recibió dólares al abrir.
    ADD COLUMN opening_float_usd numeric(18,2),
    ADD COLUMN opening_notes text NOT NULL DEFAULT '',
    -- Fecha comercial: fecha de apertura en la zona horaria del establecimiento. Los turnos
    -- anteriores conservan NULL y se calculan con opened_at en la zona vigente.
    ADD COLUMN business_date date,
    -- Inicio del arqueo de cierre: quién, cuándo, desde qué terminal y cuántos movimientos de
    -- cobro y operaciones de efectivo tenía el turno en ese momento (la confirmación vuelve a
    -- contarlos para detectar cualquier registro posterior).
    ADD COLUMN closing_started_by uuid REFERENCES app.users(id),
    ADD COLUMN closing_started_at timestamptz,
    ADD COLUMN closing_device_id uuid REFERENCES app.devices(id),
    ADD COLUMN closing_movement_count integer,
    ADD COLUMN closing_operation_count integer,
    ADD COLUMN closed_device_id uuid REFERENCES app.devices(id),
    ADD CONSTRAINT cash_shifts_opening_float_usd_check CHECK (opening_float_usd IS NULL OR opening_float_usd >= 0),
    ADD CONSTRAINT cash_shifts_opening_notes_check CHECK (length(opening_notes) <= 2000),
    ADD CONSTRAINT cash_shifts_closing_counts_check CHECK (
        (closing_movement_count IS NULL OR closing_movement_count >= 0)
        AND (closing_operation_count IS NULL OR closing_operation_count >= 0)
    ),
    -- Un turno en arqueo registra siempre quién lo inició, cuándo, desde dónde y su referencia de conteo.
    ADD CONSTRAINT cash_shifts_closing_fields_check CHECK (
        state <> 'closing'
        OR (
            closing_started_by IS NOT NULL
            AND closing_started_at IS NOT NULL
            AND closing_device_id IS NOT NULL
            AND closing_movement_count IS NOT NULL
            AND closing_operation_count IS NOT NULL
        )
    ),
    -- Un turno abierto no conserva datos de un arqueo cancelado: la auditoría guarda su historia.
    ADD CONSTRAINT cash_shifts_open_fields_check CHECK (
        state <> 'open'
        OR (
            closing_started_by IS NULL
            AND closing_started_at IS NULL
            AND closing_device_id IS NULL
            AND closing_movement_count IS NULL
            AND closing_operation_count IS NULL
            AND closed_device_id IS NULL
        )
    );

-- Un turno cerrado registra quién lo cerró y desde qué terminal. Antes de la etapa 7 no existía
-- cierre, por lo que ninguna fila debería incumplirlo; NOT VALID evita fallar sobre datos
-- históricos inesperados y aplica la regla a toda fila nueva o modificada (Go también la valida).
ALTER TABLE app.cash_shifts
    ADD CONSTRAINT cash_shifts_closed_fields_check CHECK (
        state <> 'closed' OR (closed_by IS NOT NULL AND closed_device_id IS NOT NULL)
    ) NOT VALID;

-- Un turno activo (abierto o en arqueo) por caja. Sustituye al índice de la 0006, que solo
-- cubría 'open' y permitiría abrir otro turno mientras uno está en arqueo.
DROP INDEX app.cash_shifts_one_open_idx;
CREATE UNIQUE INDEX cash_shifts_one_active_idx ON app.cash_shifts(register_id) WHERE state IN ('open', 'closing');
CREATE INDEX cash_shifts_history_idx ON app.cash_shifts(register_id, business_date, opened_at);

-- 2. Catálogo de denominaciones por moneda. El valor, la moneda y el tipo son inmutables una vez
-- creados (los conteos los referencian); solo se activan o desactivan.
CREATE TABLE app.cash_denominations (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    currency text NOT NULL CONSTRAINT cash_denominations_currency_check CHECK (currency IN ('MXN', 'USD')),
    value numeric(18,2) NOT NULL CONSTRAINT cash_denominations_value_check CHECK (value > 0),
    kind text NOT NULL CONSTRAINT cash_denominations_kind_check CHECK (kind IN ('bill', 'coin')),
    active boolean NOT NULL DEFAULT true,
    revision integer NOT NULL DEFAULT 1 CONSTRAINT cash_denominations_revision_check CHECK (revision > 0),
    updated_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT cash_denominations_unique UNIQUE (currency, value, kind)
);
-- México: billetes de 1000 a 20 y monedas de 20 a 0.50. Dólares: billetes de 100 a 1; las
-- monedas de dólar son opcionales y no se siembran (pueden agregarse con permiso).
INSERT INTO app.cash_denominations(currency, value, kind) VALUES
    ('MXN', 1000, 'bill'),
    ('MXN', 500, 'bill'),
    ('MXN', 200, 'bill'),
    ('MXN', 100, 'bill'),
    ('MXN', 50, 'bill'),
    ('MXN', 20, 'bill'),
    ('MXN', 20, 'coin'),
    ('MXN', 10, 'coin'),
    ('MXN', 5, 'coin'),
    ('MXN', 2, 'coin'),
    ('MXN', 1, 'coin'),
    ('MXN', 0.50, 'coin'),
    ('USD', 100, 'bill'),
    ('USD', 50, 'bill'),
    ('USD', 20, 'bill'),
    ('USD', 10, 'bill'),
    ('USD', 5, 'bill'),
    ('USD', 1, 'bill');

-- 3. Conteos físicos por denominaciones: el fondo inicial desglosado (final al abrir) y el arqueo
-- de cierre (borrador en el servidor hasta confirmar). Go calcula importes y totales. El importe
-- adicional identificado nunca es negativo: no sirve para compensar errores de conteo.
CREATE TABLE app.cash_counts (
    id uuid PRIMARY KEY,
    shift_id uuid NOT NULL REFERENCES app.cash_shifts(id),
    purpose text NOT NULL CONSTRAINT cash_counts_purpose_check CHECK (purpose IN ('opening', 'closing')),
    state text NOT NULL CONSTRAINT cash_counts_state_check CHECK (state IN ('draft', 'final')),
    counted_mxn numeric(18,2) NOT NULL DEFAULT 0 CONSTRAINT cash_counts_counted_mxn_check CHECK (counted_mxn >= 0),
    counted_usd numeric(18,2) NOT NULL DEFAULT 0 CONSTRAINT cash_counts_counted_usd_check CHECK (counted_usd >= 0),
    extra_mxn numeric(18,2) NOT NULL DEFAULT 0 CONSTRAINT cash_counts_extra_mxn_check CHECK (extra_mxn >= 0),
    extra_usd numeric(18,2) NOT NULL DEFAULT 0 CONSTRAINT cash_counts_extra_usd_check CHECK (extra_usd >= 0),
    extra_note text NOT NULL DEFAULT '' CONSTRAINT cash_counts_extra_note_length_check CHECK (length(extra_note) <= 200),
    notes text NOT NULL DEFAULT '' CONSTRAINT cash_counts_notes_check CHECK (length(notes) <= 2000),
    updated_by uuid NOT NULL REFERENCES app.users(id),
    updated_device_id uuid NOT NULL REFERENCES app.devices(id),
    revision integer NOT NULL DEFAULT 1 CONSTRAINT cash_counts_revision_check CHECK (revision > 0),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    -- El importe adicional debe estar identificado (por ejemplo, «monedas sueltas sin desglosar»).
    CONSTRAINT cash_counts_extra_note_check CHECK ((extra_mxn = 0 AND extra_usd = 0) OR length(extra_note) >= 3),
    CONSTRAINT cash_counts_counted_extra_check CHECK (counted_mxn >= extra_mxn AND counted_usd >= extra_usd),
    -- El desglose de apertura se registra en la misma operación que abre el turno.
    CONSTRAINT cash_counts_opening_final_check CHECK (purpose <> 'opening' OR state = 'final')
);
CREATE UNIQUE INDEX cash_counts_one_per_purpose_idx ON app.cash_counts(shift_id, purpose);

CREATE TABLE app.cash_count_lines (
    count_id uuid NOT NULL REFERENCES app.cash_counts(id),
    denomination_id uuid NOT NULL REFERENCES app.cash_denominations(id),
    quantity integer NOT NULL CONSTRAINT cash_count_lines_quantity_check CHECK (quantity BETWEEN 0 AND 100000),
    amount numeric(18,2) NOT NULL CONSTRAINT cash_count_lines_amount_check CHECK (amount >= 0),
    PRIMARY KEY (count_id, denomination_id)
);

-- 4. Categorías de gasto configurables. Un gasto desde caja no crea compras de inventario (etapa 8).
CREATE TABLE app.cash_expense_categories (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    name text NOT NULL CONSTRAINT cash_expense_categories_name_check CHECK (length(name) BETWEEN 1 AND 80),
    active boolean NOT NULL DEFAULT true,
    sort_order integer NOT NULL DEFAULT 0,
    revision integer NOT NULL DEFAULT 1 CONSTRAINT cash_expense_categories_revision_check CHECK (revision > 0),
    updated_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT cash_expense_categories_name_key UNIQUE (name)
);
INSERT INTO app.cash_expense_categories(name, sort_order) VALUES
    ('Insumos', 10),
    ('Servicios', 20),
    ('Mantenimiento', 30),
    ('Otros', 40);

-- 5. Operaciones de efectivo del turno distintas de un cobro: ingresos, retiros, gastos,
-- correcciones y regularizaciones. Son inmutables: una corrección es un registro nuevo que
-- invierte exactamente el efecto de la original (misma moneda e importe, dirección opuesta) y
-- cada original se corrige una sola vez. Una regularización es la corrección de una operación de
-- un turno cerrado, registrada en el turno vigente de la misma caja; no cambia la diferencia del
-- turno cerrado ni el efectivo esperado del turno vigente.
CREATE TABLE app.cash_operations (
    id uuid PRIMARY KEY,
    shift_id uuid NOT NULL REFERENCES app.cash_shifts(id),
    register_id uuid NOT NULL REFERENCES app.registers(id),
    kind text NOT NULL CONSTRAINT cash_operations_kind_check CHECK (kind IN ('income', 'withdrawal', 'expense', 'correction')),
    currency text NOT NULL CONSTRAINT cash_operations_currency_check CHECK (currency IN ('MXN', 'USD')),
    amount numeric(18,2) NOT NULL CONSTRAINT cash_operations_amount_check CHECK (amount > 0),
    -- Efecto físico en el cajón: entra o sale efectivo de la moneda indicada.
    direction text NOT NULL CONSTRAINT cash_operations_direction_value_check CHECK (direction IN ('in', 'out')),
    concept text NOT NULL CONSTRAINT cash_operations_concept_check CHECK (length(concept) BETWEEN 1 AND 120),
    -- Tipo de ingreso o de retiro de una lista cerrada que valida Go.
    subtype text CONSTRAINT cash_operations_subtype_format_check CHECK (subtype IS NULL OR subtype ~ '^[a-z][a-z_]{1,39}$'),
    category_id uuid REFERENCES app.cash_expense_categories(id),
    reference text NOT NULL DEFAULT '' CONSTRAINT cash_operations_reference_check CHECK (length(reference) <= 120),
    notes text NOT NULL DEFAULT '' CONSTRAINT cash_operations_notes_check CHECK (length(notes) <= 2000),
    -- Motivo de una corrección o de una salida excepcional autorizada.
    reason text NOT NULL DEFAULT '' CONSTRAINT cash_operations_reason_length_check CHECK (length(reason) <= 500),
    corrects_operation_id uuid REFERENCES app.cash_operations(id),
    regularization boolean NOT NULL DEFAULT false,
    exceptional boolean NOT NULL DEFAULT false,
    -- Tipo de cambio del turno para mostrar la equivalencia de una operación en dólares.
    usd_rate numeric(8,2),
    actor_id uuid NOT NULL REFERENCES app.users(id),
    device_id uuid NOT NULL REFERENCES app.devices(id),
    authorized_by uuid REFERENCES app.users(id),
    authorization_mode text CONSTRAINT cash_operations_authorization_mode_check
        CHECK (authorization_mode IS NULL OR authorization_mode IN ('self', 'supervisor')),
    operation_id uuid NOT NULL REFERENCES app.restaurant_operations(id) DEFERRABLE INITIALLY DEFERRED,
    created_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT cash_operations_operation_id_key UNIQUE (operation_id),
    CONSTRAINT cash_operations_direction_check CHECK (
        (kind = 'income' AND direction = 'in')
        OR (kind IN ('withdrawal', 'expense') AND direction = 'out')
        OR kind = 'correction'
    ),
    CONSTRAINT cash_operations_category_check CHECK ((kind = 'expense') = (category_id IS NOT NULL)),
    CONSTRAINT cash_operations_subtype_check CHECK ((kind IN ('income', 'withdrawal')) = (subtype IS NOT NULL)),
    -- Los ingresos no requieren segunda autorización. Toda salida y toda corrección queda
    -- autorizada: por el propio permiso del actor ('self') o por otra persona ('supervisor').
    CONSTRAINT cash_operations_authorization_check CHECK (
        (kind = 'income' AND authorized_by IS NULL AND authorization_mode IS NULL)
        OR (
            kind <> 'income'
            AND authorized_by IS NOT NULL
            AND authorization_mode IS NOT NULL
            AND (
                (authorization_mode = 'self' AND authorized_by = actor_id)
                OR (authorization_mode = 'supervisor' AND authorized_by <> actor_id)
            )
        )
    ),
    CONSTRAINT cash_operations_correction_check CHECK (
        (kind = 'correction') = (corrects_operation_id IS NOT NULL)
        AND (kind = 'correction' OR NOT regularization)
        AND (kind <> 'correction' OR length(reason) >= 5)
        AND corrects_operation_id IS DISTINCT FROM id
    ),
    CONSTRAINT cash_operations_exceptional_check CHECK (
        NOT exceptional OR (kind IN ('withdrawal', 'expense') AND length(reason) >= 5)
    ),
    CONSTRAINT cash_operations_usd_rate_check CHECK (
        (currency = 'MXN' AND usd_rate IS NULL)
        OR (currency = 'USD' AND usd_rate IS NOT NULL AND usd_rate > 0)
    )
);
CREATE UNIQUE INDEX cash_operations_one_correction_idx ON app.cash_operations(corrects_operation_id)
    WHERE corrects_operation_id IS NOT NULL;
CREATE INDEX cash_operations_shift_idx ON app.cash_operations(shift_id, created_at);

-- 6. Cortes: el parcial es un informe que no mueve dinero ni pausa la caja; el definitivo se
-- registra al confirmar el cierre. El snapshot conserva siempre todos los totales, incluido el
-- efectivo esperado; la consulta y la impresión lo ocultan a quien no tenga permiso.
CREATE TABLE app.cash_shift_reports (
    id uuid PRIMARY KEY,
    shift_id uuid NOT NULL REFERENCES app.cash_shifts(id),
    kind text NOT NULL CONSTRAINT cash_shift_reports_kind_check CHECK (kind IN ('partial', 'final')),
    sequence integer NOT NULL CONSTRAINT cash_shift_reports_sequence_check CHECK (sequence > 0),
    previous_report_id uuid REFERENCES app.cash_shift_reports(id),
    covered_from timestamptz NOT NULL,
    covered_to timestamptz NOT NULL,
    snapshot jsonb NOT NULL,
    created_by uuid NOT NULL REFERENCES app.users(id),
    device_id uuid NOT NULL REFERENCES app.devices(id),
    operation_id uuid NOT NULL REFERENCES app.restaurant_operations(id) DEFERRABLE INITIALLY DEFERRED,
    created_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT cash_shift_reports_operation_id_key UNIQUE (operation_id),
    CONSTRAINT cash_shift_reports_sequence_key UNIQUE (shift_id, kind, sequence),
    CONSTRAINT cash_shift_reports_interval_check CHECK (covered_to >= covered_from),
    -- Un solo corte definitivo por turno.
    CONSTRAINT cash_shift_reports_final_check CHECK (kind = 'partial' OR sequence = 1)
);

-- 7. Cierre definitivo: arqueo, esperado y diferencias por moneda conservados históricamente.
-- La diferencia es contado − esperado; un sobrante es positivo y un faltante negativo. La
-- equivalencia en pesos usa el tipo de cambio del turno y nunca sustituye las cantidades físicas.
CREATE TABLE app.cash_shift_closures (
    shift_id uuid PRIMARY KEY REFERENCES app.cash_shifts(id),
    count_id uuid NOT NULL REFERENCES app.cash_counts(id),
    final_report_id uuid NOT NULL REFERENCES app.cash_shift_reports(id),
    expected_mxn numeric(18,2) NOT NULL,
    expected_usd numeric(18,2) NOT NULL,
    counted_mxn numeric(18,2) NOT NULL CONSTRAINT cash_shift_closures_counted_mxn_check CHECK (counted_mxn >= 0),
    counted_usd numeric(18,2) NOT NULL CONSTRAINT cash_shift_closures_counted_usd_check CHECK (counted_usd >= 0),
    difference_mxn numeric(18,2) NOT NULL,
    difference_usd numeric(18,2) NOT NULL,
    usd_rate numeric(8,2) CONSTRAINT cash_shift_closures_usd_rate_check CHECK (usd_rate IS NULL OR usd_rate > 0),
    difference_equivalent_mxn numeric(18,2),
    result_mxn text NOT NULL CONSTRAINT cash_shift_closures_result_mxn_check CHECK (result_mxn IN ('even', 'over', 'short')),
    result_usd text NOT NULL CONSTRAINT cash_shift_closures_result_usd_check CHECK (result_usd IN ('even', 'over', 'short')),
    cashier_notes text NOT NULL DEFAULT '' CONSTRAINT cash_shift_closures_notes_check CHECK (length(cashier_notes) <= 2000),
    closed_by uuid NOT NULL REFERENCES app.users(id),
    device_id uuid NOT NULL REFERENCES app.devices(id),
    operation_id uuid NOT NULL REFERENCES app.restaurant_operations(id) DEFERRABLE INITIALLY DEFERRED,
    closed_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT cash_shift_closures_count_key UNIQUE (count_id),
    CONSTRAINT cash_shift_closures_report_key UNIQUE (final_report_id),
    CONSTRAINT cash_shift_closures_operation_id_key UNIQUE (operation_id),
    CONSTRAINT cash_shift_closures_difference_check CHECK (
        difference_mxn = counted_mxn - expected_mxn
        AND difference_usd = counted_usd - expected_usd
    ),
    CONSTRAINT cash_shift_closures_result_check CHECK (
        result_mxn = CASE WHEN difference_mxn > 0 THEN 'over' WHEN difference_mxn < 0 THEN 'short' ELSE 'even' END
        AND result_usd = CASE WHEN difference_usd > 0 THEN 'over' WHEN difference_usd < 0 THEN 'short' ELSE 'even' END
    ),
    CONSTRAINT cash_shift_closures_equivalent_check CHECK (
        (usd_rate IS NULL AND difference_equivalent_mxn IS NULL)
        OR (
            usd_rate IS NOT NULL
            AND difference_equivalent_mxn IS NOT NULL
            AND difference_equivalent_mxn = difference_mxn + round(difference_usd * usd_rate, 2)
        )
    )
);

-- Comentarios posteriores del supervisor sobre un cierre: auditados, nunca cambian la diferencia.
CREATE TABLE app.cash_closure_comments (
    id uuid PRIMARY KEY,
    shift_id uuid NOT NULL REFERENCES app.cash_shift_closures(shift_id),
    author_id uuid NOT NULL REFERENCES app.users(id),
    device_id uuid NOT NULL REFERENCES app.devices(id),
    comment text NOT NULL CONSTRAINT cash_closure_comments_comment_check CHECK (length(comment) BETWEEN 1 AND 2000),
    operation_id uuid NOT NULL REFERENCES app.restaurant_operations(id) DEFERRABLE INITIALLY DEFERRED,
    created_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT cash_closure_comments_operation_id_key UNIQUE (operation_id)
);
CREATE INDEX cash_closure_comments_shift_idx ON app.cash_closure_comments(shift_id, created_at);

-- 8. Tope de caja (función activable cash_limit, desactivada) y cajón de dinero. El tope nunca
-- bloquea cobros; NULL significa sin tope. Los usa la fase C de la etapa 7.
ALTER TABLE app.payment_settings
    ADD COLUMN cash_limit_mxn numeric(18,2),
    ADD COLUMN cash_limit_usd numeric(18,2),
    ADD CONSTRAINT payment_settings_cash_limit_check CHECK (
        (cash_limit_mxn IS NULL OR cash_limit_mxn > 0)
        AND (cash_limit_usd IS NULL OR cash_limit_usd > 0)
    );
INSERT INTO app.establishment_features(code, enabled) VALUES ('cash_limit', false) ON CONFLICT DO NOTHING;

-- Configuración del cajón por caja física: se abre con un pulso de la impresora asociada.
CREATE TABLE app.cash_drawer_settings (
    register_id uuid PRIMARY KEY REFERENCES app.registers(id),
    printer_id uuid REFERENCES app.kitchen_printers(id),
    pulse_pin smallint NOT NULL DEFAULT 0 CONSTRAINT cash_drawer_settings_pulse_pin_check CHECK (pulse_pin IN (0, 1)),
    enabled boolean NOT NULL DEFAULT false,
    revision integer NOT NULL DEFAULT 1 CONSTRAINT cash_drawer_settings_revision_check CHECK (revision > 0),
    updated_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT cash_drawer_settings_printer_check CHECK (NOT enabled OR printer_id IS NOT NULL)
);

-- Cada comando de apertura emitido; no afirma que el cajón se abrió físicamente.
CREATE TABLE app.cash_drawer_events (
    id uuid PRIMARY KEY,
    register_id uuid NOT NULL REFERENCES app.registers(id),
    shift_id uuid REFERENCES app.cash_shifts(id),
    reason text NOT NULL CONSTRAINT cash_drawer_events_reason_check CHECK (reason IN ('payment', 'manual', 'operation')),
    actor_id uuid NOT NULL REFERENCES app.users(id),
    device_id uuid NOT NULL REFERENCES app.devices(id),
    job_id uuid REFERENCES app.kitchen_print_jobs(id),
    operation_id uuid NOT NULL REFERENCES app.restaurant_operations(id) DEFERRABLE INITIALLY DEFERRED,
    created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX cash_drawer_events_register_idx ON app.cash_drawer_events(register_id, created_at);

-- 9. Documentos de caja (comprobantes y cortes) en la cola durable de impresión de la etapa 5.
-- El CHECK del tipo de ticket se recreó con nombre explícito en la 0006.
ALTER TABLE app.kitchen_tickets DROP CONSTRAINT kitchen_tickets_kind_check;
ALTER TABLE app.kitchen_tickets
    ADD CONSTRAINT kitchen_tickets_kind_check CHECK (
        kind IN ('normal', 'modification', 'cancellation', 'test', 'receipt', 'precheck', 'cash_report', 'cash_voucher')
    );

CREATE TABLE app.cash_documents (
    id uuid PRIMARY KEY,
    ticket_id uuid NOT NULL REFERENCES app.kitchen_tickets(id),
    shift_id uuid NOT NULL REFERENCES app.cash_shifts(id),
    report_id uuid REFERENCES app.cash_shift_reports(id),
    cash_operation_id uuid REFERENCES app.cash_operations(id),
    kind text NOT NULL CONSTRAINT cash_documents_kind_check
        CHECK (kind IN ('partial_cut', 'final_cut', 'income', 'withdrawal', 'expense')),
    created_by uuid NOT NULL REFERENCES app.users(id),
    device_id uuid NOT NULL REFERENCES app.devices(id),
    created_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT cash_documents_ticket_key UNIQUE (ticket_id),
    -- Exactamente una referencia según el tipo: un corte o una operación de efectivo.
    CONSTRAINT cash_documents_reference_check CHECK (
        (kind IN ('partial_cut', 'final_cut') AND report_id IS NOT NULL AND cash_operation_id IS NULL)
        OR (kind IN ('income', 'withdrawal', 'expense') AND cash_operation_id IS NOT NULL AND report_id IS NULL)
    )
);
CREATE INDEX cash_documents_shift_idx ON app.cash_documents(shift_id, created_at);

-- 10. Eventos en tiempo real con ámbito propio de caja. El CHECK sin nombre de la 0005 se llama
-- realtime_events_scope_check (verificado en una base temporal migrada).
ALTER TABLE app.realtime_events DROP CONSTRAINT realtime_events_scope_check;
ALTER TABLE app.realtime_events
    ADD CONSTRAINT realtime_events_scope_check CHECK (
        scope IN ('restaurant', 'takeaway', 'kitchen', 'printing', 'configuration', 'cash')
    );

-- 11. Permisos nuevos. Se registran también en Go (payments.Permissions) y se conceden solo al
-- administrador, para que siga contando como administrador completo. No se asignan por inferencia
-- a otros roles ni se restablecen roles o credenciales existentes.
INSERT INTO app.permissions(key, description) VALUES
    ('cash.shifts.read_all', 'Consultar turnos de otras cajas'),
    ('cash.income.record', 'Registrar ingresos de efectivo'),
    ('cash.withdrawals.record', 'Registrar retiros de efectivo'),
    ('cash.expenses.record', 'Registrar gastos pagados desde caja'),
    ('cash.outflows.authorize', 'Autorizar retiros y gastos de efectivo'),
    ('cash.corrections.record', 'Corregir y regularizar movimientos de efectivo'),
    ('cash.corrections.authorize', 'Autorizar correcciones y regularizaciones de efectivo'),
    ('cash.movements.read', 'Consultar movimientos de efectivo'),
    ('cash.partial_cuts.create', 'Realizar cortes parciales'),
    ('cash.counts.perform', 'Realizar arqueos'),
    ('cash.shifts.close', 'Cerrar turnos de caja'),
    ('cash.closing.cancel_any', 'Cancelar el arqueo de cierre iniciado por otro usuario'),
    ('cash.history.read', 'Consultar turnos y cortes históricos'),
    ('cash.expected.read', 'Consultar importes esperados y diferencias'),
    ('cash.closures.review', 'Comentar cierres de caja'),
    ('cash.reports.print', 'Imprimir cortes y comprobantes de caja'),
    ('cash.reports.reprint', 'Reimprimir cortes y comprobantes de caja'),
    ('cash.drawer.open', 'Abrir el cajón de dinero manualmente'),
    ('cash.settings.manage', 'Administrar denominaciones, categorías de gasto, tope de caja y cajón');

INSERT INTO app.role_permissions(role_id, permission_key)
SELECT r.id, p.key FROM app.roles r CROSS JOIN app.permissions p
WHERE r.code = 'administrator' AND p.key IN (
    'cash.shifts.read_all', 'cash.income.record', 'cash.withdrawals.record', 'cash.expenses.record',
    'cash.outflows.authorize', 'cash.corrections.record', 'cash.corrections.authorize', 'cash.movements.read',
    'cash.partial_cuts.create', 'cash.counts.perform', 'cash.shifts.close', 'cash.closing.cancel_any',
    'cash.history.read', 'cash.expected.read', 'cash.closures.review', 'cash.reports.print',
    'cash.reports.reprint', 'cash.drawer.open', 'cash.settings.manage'
)
ON CONFLICT DO NOTHING;
