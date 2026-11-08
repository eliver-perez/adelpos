-- Etapa 6: cobros, pagos, descuentos manuales, cargos, propinas, IVA, devoluciones, turno mínimo y entregas.
-- Los registros financieros son inmutables: toda corrección se expresa con registros compensatorios enlazados.
-- No modifica importes históricos de unidades ni pedidos; solo amplía restricciones y agrega entidades.

-- Configuración de cobro e IVA. La tasa solo admite la lista cerrada 16, 8 o 0 por ciento.
CREATE TABLE app.payment_settings (
    singleton boolean PRIMARY KEY DEFAULT true CHECK (singleton),
    tax_rate integer NOT NULL DEFAULT 16 CHECK (tax_rate IN (0, 8, 16)),
    shared_shifts boolean NOT NULL DEFAULT false,
    print_receipt_default boolean NOT NULL DEFAULT true,
    receipt_printer_id uuid REFERENCES app.kitchen_printers(id),
    refund_authorization_threshold numeric(18,2) CHECK (refund_authorization_threshold IS NULL OR refund_authorization_threshold >= 0),
    revision integer NOT NULL DEFAULT 1 CHECK (revision > 0),
    updated_at timestamptz NOT NULL DEFAULT now()
);
INSERT INTO app.payment_settings(singleton) VALUES (true);

-- Cada versión confirmada conserva la tasa vigente al confirmarse. Las existentes reciben 16 %;
-- las nuevas deben indicarla expresamente (sin valor predeterminado).
ALTER TABLE app.restaurant_item_versions
    ADD COLUMN tax_rate integer NOT NULL DEFAULT 16 CHECK (tax_rate IN (0, 8, 16));
ALTER TABLE app.restaurant_item_versions ALTER COLUMN tax_rate DROP DEFAULT;

-- Una cuenta liquidada se cierra y libera su mesa; una devolución que deja saldo la reabre.
ALTER TABLE app.restaurant_accounts DROP CONSTRAINT restaurant_accounts_state_check;
ALTER TABLE app.restaurant_accounts
    ADD CONSTRAINT restaurant_accounts_state_check CHECK (state IN ('open', 'merged', 'voided', 'closed'));
ALTER TABLE app.restaurant_accounts ADD COLUMN closed_at timestamptz;
ALTER TABLE app.restaurant_accounts
    ADD CONSTRAINT restaurant_accounts_closed_check CHECK ((state = 'closed') = (closed_at IS NOT NULL));

-- Precuentas y tickets reutilizan la cola durable de impresión de la etapa 5.
ALTER TABLE app.kitchen_tickets DROP CONSTRAINT kitchen_tickets_kind_check;
ALTER TABLE app.kitchen_tickets
    ADD CONSTRAINT kitchen_tickets_kind_check CHECK (kind IN ('normal', 'modification', 'cancellation', 'test', 'receipt', 'precheck'));

-- Métodos configurables. El nombre visible puede cambiar: cada pago conserva su propia copia.
CREATE TABLE app.payment_methods (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    code text UNIQUE,
    name text NOT NULL UNIQUE CHECK (length(name) BETWEEN 1 AND 60),
    icon text NOT NULL CHECK (icon IN ('cash', 'card', 'transfer', 'platform', 'voucher', 'other')),
    active boolean NOT NULL DEFAULT true,
    sort_order integer NOT NULL DEFAULT 0,
    is_cash boolean NOT NULL DEFAULT false,
    reference_mode text NOT NULL DEFAULT 'none' CHECK (reference_mode IN ('none', 'optional', 'required')),
    allows_tip boolean NOT NULL DEFAULT true,
    allows_refund boolean NOT NULL DEFAULT true,
    revision integer NOT NULL DEFAULT 1 CHECK (revision > 0),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    CHECK (NOT is_cash OR reference_mode = 'none')
);
INSERT INTO app.payment_methods(code, name, icon, sort_order, is_cash, reference_mode, allows_tip) VALUES
    ('cash', 'Efectivo', 'cash', 10, true, 'none', true),
    ('debit', 'Tarjeta de débito', 'card', 20, false, 'optional', true),
    ('credit', 'Tarjeta de crédito', 'card', 30, false, 'optional', true),
    ('transfer', 'Transferencia', 'transfer', 40, false, 'optional', true),
    ('platforms', 'Plataformas', 'platform', 50, false, 'optional', false);

-- Turno mínimo: una caja física admite como máximo un turno abierto. El cierre pertenece a la etapa 7.
CREATE TABLE app.cash_shifts (
    id uuid PRIMARY KEY,
    register_id uuid NOT NULL REFERENCES app.registers(id),
    state text NOT NULL DEFAULT 'open' CHECK (state IN ('open', 'closed')),
    opening_float numeric(18,2) NOT NULL CHECK (opening_float >= 0),
    opened_by uuid NOT NULL REFERENCES app.users(id),
    opened_device_id uuid NOT NULL REFERENCES app.devices(id),
    opened_at timestamptz NOT NULL DEFAULT now(),
    operation_id uuid NOT NULL UNIQUE REFERENCES app.restaurant_operations(id) DEFERRABLE INITIALLY DEFERRED,
    closed_at timestamptz,
    closed_by uuid REFERENCES app.users(id),
    revision integer NOT NULL DEFAULT 1 CHECK (revision > 0),
    CHECK ((state = 'closed') = (closed_at IS NOT NULL))
);
CREATE UNIQUE INDEX cash_shifts_one_open_idx ON app.cash_shifts(register_id) WHERE state = 'open';

-- Pago aplicado a la venta. La propina y el cambio se guardan aparte para no inflar las ventas.
CREATE TABLE app.payments (
    id uuid PRIMARY KEY,
    account_id uuid NOT NULL REFERENCES app.restaurant_accounts(id),
    shift_id uuid NOT NULL REFERENCES app.cash_shifts(id),
    register_id uuid NOT NULL REFERENCES app.registers(id),
    method_id uuid NOT NULL REFERENCES app.payment_methods(id),
    method_name text NOT NULL,
    is_cash boolean NOT NULL,
    kind text NOT NULL CHECK (kind IN ('general', 'selection')),
    amount numeric(18,2) NOT NULL CHECK (amount > 0),
    tip numeric(18,2) NOT NULL DEFAULT 0 CHECK (tip >= 0),
    received numeric(18,2),
    change_given numeric(18,2) NOT NULL DEFAULT 0 CHECK (change_given >= 0),
    reference text NOT NULL DEFAULT '' CHECK (length(reference) <= 120),
    corrects_payment_id uuid REFERENCES app.payments(id),
    actor_id uuid NOT NULL REFERENCES app.users(id),
    device_id uuid NOT NULL REFERENCES app.devices(id),
    operation_id uuid NOT NULL UNIQUE REFERENCES app.restaurant_operations(id) DEFERRABLE INITIALLY DEFERRED,
    created_at timestamptz NOT NULL DEFAULT now(),
    CHECK (is_cash OR (received IS NULL AND change_given = 0)),
    CHECK (received IS NULL OR change_given = received - amount - tip),
    CHECK (corrects_payment_id IS NULL OR corrects_payment_id <> id)
);
CREATE INDEX payments_account_idx ON app.payments(account_id, created_at);
CREATE INDEX payments_shift_idx ON app.payments(shift_id, created_at);

-- Pago por selección: importe asignado a cada unidad comercial elegida.
CREATE TABLE app.payment_allocations (
    payment_id uuid NOT NULL REFERENCES app.payments(id),
    unit_id uuid NOT NULL REFERENCES app.restaurant_item_units(id),
    amount numeric(18,2) NOT NULL CHECK (amount > 0),
    PRIMARY KEY (payment_id, unit_id)
);
CREATE INDEX payment_allocations_unit_idx ON app.payment_allocations(unit_id);

-- Devolución o reverso de corrección de un pago concreto. Nunca modifica el pago original.
CREATE TABLE app.payment_refunds (
    id uuid PRIMARY KEY,
    payment_id uuid NOT NULL REFERENCES app.payments(id),
    account_id uuid NOT NULL REFERENCES app.restaurant_accounts(id),
    kind text NOT NULL CHECK (kind IN ('refund', 'correction')),
    shift_id uuid NOT NULL REFERENCES app.cash_shifts(id),
    register_id uuid NOT NULL REFERENCES app.registers(id),
    method_id uuid NOT NULL REFERENCES app.payment_methods(id),
    method_name text NOT NULL,
    is_cash boolean NOT NULL,
    amount numeric(18,2) NOT NULL CHECK (amount >= 0),
    tip numeric(18,2) NOT NULL DEFAULT 0 CHECK (tip >= 0),
    reason text NOT NULL CHECK (length(reason) BETWEEN 5 AND 500),
    reference text NOT NULL DEFAULT '' CHECK (length(reference) <= 120),
    actor_id uuid NOT NULL REFERENCES app.users(id),
    device_id uuid NOT NULL REFERENCES app.devices(id),
    authorized_by uuid REFERENCES app.users(id),
    operation_id uuid NOT NULL REFERENCES app.restaurant_operations(id) DEFERRABLE INITIALLY DEFERRED,
    created_at timestamptz NOT NULL DEFAULT now(),
    CHECK (amount + tip > 0),
    UNIQUE (operation_id, payment_id)
);
CREATE INDEX payment_refunds_payment_idx ON app.payment_refunds(payment_id);
CREATE INDEX payment_refunds_account_idx ON app.payment_refunds(account_id, created_at);

-- Reverso de asignaciones de unidades: tras devolverlas, las unidades vuelven a ser movibles.
CREATE TABLE app.payment_refund_allocations (
    refund_id uuid NOT NULL REFERENCES app.payment_refunds(id),
    unit_id uuid NOT NULL REFERENCES app.restaurant_item_units(id),
    amount numeric(18,2) NOT NULL CHECK (amount > 0),
    PRIMARY KEY (refund_id, unit_id)
);
CREATE INDEX payment_refund_allocations_unit_idx ON app.payment_refund_allocations(unit_id);

-- Descuentos manuales, cortesías y cargos extras generales. Son distintos del descuento de promoción
-- guardado en restaurant_item_units.discount, que no se reutiliza.
CREATE TABLE app.account_adjustments (
    id uuid PRIMARY KEY,
    account_id uuid NOT NULL REFERENCES app.restaurant_accounts(id),
    kind text NOT NULL CHECK (kind IN ('discount', 'charge')),
    scope text NOT NULL CHECK (scope IN ('account', 'items')),
    method text NOT NULL CHECK (method IN ('percent', 'amount')),
    value numeric(18,2) NOT NULL CHECK (value > 0),
    amount numeric(18,2) NOT NULL CHECK (amount > 0),
    courtesy boolean NOT NULL DEFAULT false,
    tax_rate integer CHECK (tax_rate IN (0, 8, 16)),
    concept text NOT NULL CHECK (length(concept) BETWEEN 1 AND 120),
    reason text NOT NULL DEFAULT '' CHECK (length(reason) <= 500),
    actor_id uuid NOT NULL REFERENCES app.users(id),
    device_id uuid NOT NULL REFERENCES app.devices(id),
    authorized_by uuid REFERENCES app.users(id),
    operation_id uuid NOT NULL UNIQUE REFERENCES app.restaurant_operations(id) DEFERRABLE INITIALLY DEFERRED,
    created_at timestamptz NOT NULL DEFAULT now(),
    CHECK (method <> 'percent' OR value <= 100),
    CHECK (NOT courtesy OR (kind = 'discount' AND method = 'percent' AND value = 100)),
    CHECK (kind = 'discount' OR (scope = 'account' AND method = 'amount' AND tax_rate IS NOT NULL)),
    CHECK (kind = 'charge' OR tax_rate IS NULL)
);
CREATE INDEX account_adjustments_account_idx ON app.account_adjustments(account_id, created_at);

CREATE TABLE app.account_adjustment_allocations (
    adjustment_id uuid NOT NULL REFERENCES app.account_adjustments(id),
    unit_id uuid NOT NULL REFERENCES app.restaurant_item_units(id),
    amount numeric(18,2) NOT NULL CHECK (amount > 0),
    PRIMARY KEY (adjustment_id, unit_id)
);
CREATE INDEX account_adjustment_allocations_unit_idx ON app.account_adjustment_allocations(unit_id);

-- Cancelación auditada de un descuento o cargo: el registro original permanece.
CREATE TABLE app.account_adjustment_voids (
    adjustment_id uuid PRIMARY KEY REFERENCES app.account_adjustments(id),
    reason text NOT NULL CHECK (length(reason) BETWEEN 5 AND 500),
    actor_id uuid NOT NULL REFERENCES app.users(id),
    device_id uuid NOT NULL REFERENCES app.devices(id),
    authorized_by uuid REFERENCES app.users(id),
    operation_id uuid NOT NULL UNIQUE REFERENCES app.restaurant_operations(id) DEFERRABLE INITIALLY DEFERRED,
    created_at timestamptz NOT NULL DEFAULT now()
);

-- Autorización puntual de supervisor: no crea sesión, no guarda contraseña y no se reutiliza.
CREATE TABLE app.payment_authorizations (
    id uuid PRIMARY KEY,
    operation_id uuid NOT NULL REFERENCES app.restaurant_operations(id) DEFERRABLE INITIALLY DEFERRED,
    scope text NOT NULL CHECK (scope IN ('discount', 'courtesy', 'refund', 'correction', 'adjustment_void')),
    account_id uuid NOT NULL REFERENCES app.restaurant_accounts(id),
    amount numeric(18,2) NOT NULL CHECK (amount >= 0),
    requested_by uuid NOT NULL REFERENCES app.users(id),
    authorized_by uuid NOT NULL REFERENCES app.users(id),
    device_id uuid NOT NULL REFERENCES app.devices(id),
    created_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (operation_id),
    CHECK (requested_by <> authorized_by)
);

-- Límite máximo de descuento por rol. Sin fila, el límite es 0 %.
CREATE TABLE app.role_discount_limits (
    role_id uuid PRIMARY KEY REFERENCES app.roles(id),
    max_percent numeric(5,2) NOT NULL CHECK (max_percent BETWEEN 0 AND 100)
);
INSERT INTO app.role_discount_limits(role_id, max_percent)
SELECT id, 100 FROM app.roles WHERE code = 'administrator';

-- Movimientos de turno para la etapa 7. Importes firmados: positivos al cobrar, negativos al devolver.
-- El efectivo recibido y el cambio se conservan sin duplicar el ingreso neto.
CREATE TABLE app.cash_movements (
    id uuid PRIMARY KEY,
    shift_id uuid NOT NULL REFERENCES app.cash_shifts(id),
    register_id uuid NOT NULL REFERENCES app.registers(id),
    kind text NOT NULL CHECK (kind IN ('payment', 'refund', 'correction_reversal')),
    method_id uuid NOT NULL REFERENCES app.payment_methods(id),
    method_name text NOT NULL,
    is_cash boolean NOT NULL,
    sale_amount numeric(18,2) NOT NULL,
    tip_amount numeric(18,2) NOT NULL DEFAULT 0,
    net_amount numeric(18,2) NOT NULL,
    cash_received numeric(18,2) NOT NULL DEFAULT 0 CHECK (cash_received >= 0),
    change_given numeric(18,2) NOT NULL DEFAULT 0 CHECK (change_given >= 0),
    account_id uuid NOT NULL REFERENCES app.restaurant_accounts(id),
    payment_id uuid REFERENCES app.payments(id),
    refund_id uuid REFERENCES app.payment_refunds(id),
    actor_id uuid NOT NULL REFERENCES app.users(id),
    device_id uuid NOT NULL REFERENCES app.devices(id),
    operation_id uuid NOT NULL REFERENCES app.restaurant_operations(id) DEFERRABLE INITIALLY DEFERRED,
    created_at timestamptz NOT NULL DEFAULT now(),
    CHECK (net_amount = sale_amount + tip_amount),
    CHECK ((payment_id IS NULL) <> (refund_id IS NULL)),
    CHECK (kind = 'payment' OR refund_id IS NOT NULL)
);
CREATE INDEX cash_movements_shift_idx ON app.cash_movements(shift_id, created_at);

-- Entrega de pedidos para llevar: una sola por cuenta, idempotente y auditada.
CREATE TABLE app.takeaway_deliveries (
    account_id uuid PRIMARY KEY REFERENCES app.restaurant_accounts(id),
    actor_id uuid NOT NULL REFERENCES app.users(id),
    device_id uuid NOT NULL REFERENCES app.devices(id),
    operation_id uuid NOT NULL UNIQUE REFERENCES app.restaurant_operations(id) DEFERRABLE INITIALLY DEFERRED,
    delivered_at timestamptz NOT NULL DEFAULT now()
);

-- Precuentas y tickets: documento comercial ligado a la cuenta y, en su caso, al pago.
CREATE TABLE app.payment_documents (
    ticket_id uuid PRIMARY KEY REFERENCES app.kitchen_tickets(id),
    account_id uuid NOT NULL REFERENCES app.restaurant_accounts(id),
    payment_id uuid REFERENCES app.payments(id),
    kind text NOT NULL CHECK (kind IN ('receipt', 'precheck')),
    actor_id uuid NOT NULL REFERENCES app.users(id),
    device_id uuid NOT NULL REFERENCES app.devices(id),
    created_at timestamptz NOT NULL DEFAULT now(),
    CHECK (kind = 'receipt' OR payment_id IS NULL)
);
CREATE INDEX payment_documents_account_idx ON app.payment_documents(account_id, created_at);

INSERT INTO app.permissions(key, description) VALUES
    ('payments.read', 'Consultar información financiera y pagos de cuentas'),
    ('payments.collect', 'Registrar cobros'),
    ('payments.collect_partial', 'Registrar pagos parciales'),
    ('payments.collect_selection', 'Cobrar productos y cantidades seleccionados'),
    ('payments.methods.manage', 'Administrar métodos de pago'),
    ('payments.settings.manage', 'Configurar políticas de cobro e impresión de tickets'),
    ('payments.tax.manage', 'Configurar la tasa de IVA del establecimiento'),
    ('payments.discounts.apply', 'Aplicar descuentos manuales dentro del límite del rol'),
    ('payments.discounts.authorize', 'Autorizar descuentos superiores al límite'),
    ('payments.courtesy', 'Otorgar cortesías'),
    ('payments.charges.create', 'Registrar cargos extras generales'),
    ('payments.charges.manage', 'Modificar o cancelar cargos extras'),
    ('payments.tips.record', 'Registrar propinas'),
    ('payments.corrections', 'Anular o corregir pagos'),
    ('payments.refunds.execute', 'Ejecutar devoluciones'),
    ('payments.refunds.authorize', 'Autorizar devoluciones'),
    ('payments.tickets.print', 'Imprimir tickets y precuentas'),
    ('payments.tickets.reprint', 'Reimprimir tickets'),
    ('cash.shifts.open', 'Abrir turno de caja'),
    ('cash.shifts.read', 'Consultar turnos y movimientos de caja'),
    ('takeaway.deliver', 'Registrar entrega de pedidos para llevar');

-- Solo permisos nuevos al administrador: no se restablecen roles ni credenciales existentes.
INSERT INTO app.role_permissions(role_id, permission_key)
SELECT r.id, p.key FROM app.roles r CROSS JOIN app.permissions p
WHERE r.code = 'administrator' AND (p.key LIKE 'payments.%' OR p.key LIKE 'cash.%' OR p.key = 'takeaway.deliver')
ON CONFLICT DO NOTHING;
