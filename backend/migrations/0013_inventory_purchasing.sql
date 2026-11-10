-- Etapa 8, fase C: compras. Proveedores, insumos que surte cada proveedor, órdenes de compra y sus
-- renglones, recepciones de mercancía y sus renglones, devoluciones a proveedor y sus renglones y
-- vínculos opcionales de una recepción con gastos de caja, con los permisos de la fase C.
-- Transaccional: el migrador la aplica dentro de la misma transacción que la registra en
-- schema_migrations y no contiene sentencias que no admitan transacción (ni CREATE INDEX
-- CONCURRENTLY ni VACUUM), de modo que un fallo la revierte completa. No modifica 0001–0012 ni
-- filas existentes y no siembra datos (ni proveedores ni compras: los registra el restaurante). Las
-- columnas opcionales llevan CHECK que exigen IS NOT NULL en la rama que valida, porque un CHECK que
-- evalúa a NULL se acepta.
--
-- Importes de compra (decisión de diseño de la etapa 8): precio unitario sin IVA numeric(18,6);
-- descuento comercial, subtotal (después del descuento), IVA y total por renglón y por documento
-- numeric(18,2), calculados en Go con decimales y redondeo a centavos mitad alejándose de cero; la
-- tasa de IVA es la lista cerrada 16/8/0 de ventas. El valor de inventario de una recepción
-- (numeric(20,6)) es el total con IVA o el subtotal sin IVA según tax_in_cost, que cada recepción
-- copia de inventory_settings al registrarse. Las cantidades se guardan tal como se capturaron
-- (presentación o unidad) y convertidas a la unidad base del insumo al registrarse.
--
-- Una compra nunca crea un gasto de caja y un gasto nunca crea una compra (§2, reglas 5 y 6): el
-- vínculo es una tabla propia que referencia cash_operations sin modificarla, ni al turno, ni al
-- cierre.

-- 1. Proveedores. Nombre comercial único sin distinguir mayúsculas; razón social, RFC (opcional,
-- con formato validado y no único: varias sucursales pueden compartirlo), teléfono, correo,
-- domicilio, contacto y observaciones. Se desactivan, nunca se borran (no existe ruta de borrado y
-- las compras los referencian).
CREATE TABLE app.inventory_suppliers (
    id uuid PRIMARY KEY,
    trade_name text NOT NULL CONSTRAINT inventory_suppliers_trade_name_check
        CHECK (length(trade_name) BETWEEN 1 AND 100 AND trade_name = btrim(trade_name)),
    legal_name text NOT NULL DEFAULT '' CONSTRAINT inventory_suppliers_legal_name_check
        CHECK (length(legal_name) <= 200 AND legal_name = btrim(legal_name)),
    rfc text CONSTRAINT inventory_suppliers_rfc_check
        CHECK (rfc IS NULL OR rfc ~ '^[A-ZÑ&]{3,4}[0-9]{6}[A-Z0-9]{3}$'),
    phone text NOT NULL DEFAULT '' CONSTRAINT inventory_suppliers_phone_check CHECK (length(phone) <= 40),
    email text NOT NULL DEFAULT '' CONSTRAINT inventory_suppliers_email_check CHECK (length(email) <= 120),
    address text NOT NULL DEFAULT '' CONSTRAINT inventory_suppliers_address_check CHECK (length(address) <= 500),
    contact text NOT NULL DEFAULT '' CONSTRAINT inventory_suppliers_contact_check CHECK (length(contact) <= 100),
    notes text NOT NULL DEFAULT '' CONSTRAINT inventory_suppliers_notes_check CHECK (length(notes) <= 2000),
    active boolean NOT NULL DEFAULT true,
    revision integer NOT NULL DEFAULT 1 CONSTRAINT inventory_suppliers_revision_check CHECK (revision > 0),
    created_by uuid CONSTRAINT inventory_suppliers_created_by_fkey REFERENCES app.users(id),
    updated_by uuid CONSTRAINT inventory_suppliers_updated_by_fkey REFERENCES app.users(id),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX inventory_suppliers_trade_name_key ON app.inventory_suppliers (lower(trade_name));
CREATE INDEX inventory_suppliers_rfc_idx ON app.inventory_suppliers (rfc) WHERE rfc IS NOT NULL;

-- 2. Insumos que surte cada proveedor: código del proveedor, presentación habitual, preferido (uno
-- por insumo) y el último precio recibido tal como se capturó (sin IVA y antes del descuento, por la
-- presentación o la unidad de esa recepción). Las recepciones crean la relación si no existía y
-- actualizan el último precio solo con una recepción igual o más reciente que la anterior.
CREATE TABLE app.inventory_supplier_items (
    supplier_id uuid NOT NULL CONSTRAINT inventory_supplier_items_supplier_id_fkey REFERENCES app.inventory_suppliers(id),
    item_id uuid NOT NULL CONSTRAINT inventory_supplier_items_item_id_fkey REFERENCES app.inventory_items(id),
    presentation_id uuid CONSTRAINT inventory_supplier_items_presentation_id_fkey REFERENCES app.inventory_presentations(id),
    supplier_code text NOT NULL DEFAULT '' CONSTRAINT inventory_supplier_items_supplier_code_check
        CHECK (length(supplier_code) <= 60 AND supplier_code = btrim(supplier_code)),
    preferred boolean NOT NULL DEFAULT false,
    last_unit_price numeric(18,6) CONSTRAINT inventory_supplier_items_last_unit_price_check
        CHECK (last_unit_price IS NULL OR last_unit_price >= 0),
    last_price_presentation_id uuid CONSTRAINT inventory_supplier_items_last_price_presentation_id_fkey
        REFERENCES app.inventory_presentations(id),
    last_price_unit_id uuid CONSTRAINT inventory_supplier_items_last_price_unit_id_fkey REFERENCES app.inventory_units(id),
    last_price_at timestamptz,
    last_receipt_id uuid,
    revision integer NOT NULL DEFAULT 1 CONSTRAINT inventory_supplier_items_revision_check CHECK (revision > 0),
    updated_by uuid CONSTRAINT inventory_supplier_items_updated_by_fkey REFERENCES app.users(id),
    updated_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT inventory_supplier_items_pkey PRIMARY KEY (supplier_id, item_id),
    -- El último precio existe completo (precio, fecha, recepción y exactamente una unidad de captura) o
    -- no existe.
    CONSTRAINT inventory_supplier_items_last_price_check CHECK (
        (
            last_unit_price IS NULL AND last_price_at IS NULL AND last_receipt_id IS NULL
            AND last_price_presentation_id IS NULL AND last_price_unit_id IS NULL
        )
        OR (
            last_unit_price IS NOT NULL AND last_price_at IS NOT NULL AND last_receipt_id IS NOT NULL
            AND (
                (last_price_presentation_id IS NOT NULL AND last_price_unit_id IS NULL)
                OR (last_price_presentation_id IS NULL AND last_price_unit_id IS NOT NULL)
            )
        )
    )
);
CREATE UNIQUE INDEX inventory_supplier_items_preferred_key ON app.inventory_supplier_items (item_id) WHERE preferred;
CREATE INDEX inventory_supplier_items_item_idx ON app.inventory_supplier_items (item_id);

-- 3. Órdenes de compra. Folio propio (identidad), proveedor, estado, fecha, almacén de destino
-- previsto, observaciones, responsable y los datos de emisión, cancelación y cierre administrativo.
-- Los importes del documento son la suma de sus renglones. Crear o emitir una orden no toca
-- existencias ni caja; las recepciones cambian su estado (parcial o recibida).
CREATE TABLE app.inventory_purchase_orders (
    id uuid PRIMARY KEY,
    folio bigint GENERATED ALWAYS AS IDENTITY,
    supplier_id uuid NOT NULL CONSTRAINT inventory_purchase_orders_supplier_id_fkey REFERENCES app.inventory_suppliers(id),
    state text NOT NULL DEFAULT 'draft' CONSTRAINT inventory_purchase_orders_state_check
        CHECK (state IN ('draft', 'issued', 'partially_received', 'received', 'cancelled', 'closed')),
    order_date date NOT NULL,
    expected_warehouse_id uuid NOT NULL CONSTRAINT inventory_purchase_orders_expected_warehouse_id_fkey
        REFERENCES app.inventory_warehouses(id),
    notes text NOT NULL DEFAULT '' CONSTRAINT inventory_purchase_orders_notes_check CHECK (length(notes) <= 2000),
    subtotal numeric(18,2) NOT NULL DEFAULT 0 CONSTRAINT inventory_purchase_orders_subtotal_check CHECK (subtotal >= 0),
    discount numeric(18,2) NOT NULL DEFAULT 0 CONSTRAINT inventory_purchase_orders_discount_check CHECK (discount >= 0),
    tax numeric(18,2) NOT NULL DEFAULT 0 CONSTRAINT inventory_purchase_orders_tax_check CHECK (tax >= 0),
    total numeric(18,2) NOT NULL DEFAULT 0,
    line_count integer NOT NULL DEFAULT 0 CONSTRAINT inventory_purchase_orders_line_count_check CHECK (line_count BETWEEN 0 AND 200),
    created_by uuid NOT NULL CONSTRAINT inventory_purchase_orders_created_by_fkey REFERENCES app.users(id),
    updated_by uuid NOT NULL CONSTRAINT inventory_purchase_orders_updated_by_fkey REFERENCES app.users(id),
    issued_by uuid CONSTRAINT inventory_purchase_orders_issued_by_fkey REFERENCES app.users(id),
    issued_at timestamptz,
    cancelled_by uuid CONSTRAINT inventory_purchase_orders_cancelled_by_fkey REFERENCES app.users(id),
    cancelled_at timestamptz,
    cancel_reason text,
    closed_by uuid CONSTRAINT inventory_purchase_orders_closed_by_fkey REFERENCES app.users(id),
    closed_at timestamptz,
    close_reason text,
    revision integer NOT NULL DEFAULT 1 CONSTRAINT inventory_purchase_orders_revision_check CHECK (revision > 0),
    operation_id uuid NOT NULL CONSTRAINT inventory_purchase_orders_operation_id_fkey
        REFERENCES app.restaurant_operations(id) DEFERRABLE INITIALLY DEFERRED,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT inventory_purchase_orders_folio_key UNIQUE (folio),
    CONSTRAINT inventory_purchase_orders_operation_id_key UNIQUE (operation_id),
    CONSTRAINT inventory_purchase_orders_total_check CHECK (total = subtotal + tax),
    -- Emisión: un borrador no está emitido; toda orden emitida, recibida o cerrada lo está; una
    -- cancelada puede venir de borrador (sin emisión) o de emitida (con emisión completa).
    CONSTRAINT inventory_purchase_orders_issue_check CHECK (
        (state = 'draft' AND issued_by IS NULL AND issued_at IS NULL)
        OR (
            state IN ('issued', 'partially_received', 'received', 'closed')
            AND issued_by IS NOT NULL AND issued_at IS NOT NULL
        )
        OR (
            state = 'cancelled'
            AND ((issued_by IS NULL AND issued_at IS NULL) OR (issued_by IS NOT NULL AND issued_at IS NOT NULL))
        )
    ),
    -- Cancelación y cierre administrativo siempre con quién, cuándo y motivo (5–500); ningún otro
    -- estado los conserva.
    CONSTRAINT inventory_purchase_orders_cancel_check CHECK (
        (
            state = 'cancelled' AND cancelled_by IS NOT NULL AND cancelled_at IS NOT NULL AND cancel_reason IS NOT NULL
            AND length(cancel_reason) BETWEEN 5 AND 500
        )
        OR (state <> 'cancelled' AND cancelled_by IS NULL AND cancelled_at IS NULL AND cancel_reason IS NULL)
    ),
    CONSTRAINT inventory_purchase_orders_close_check CHECK (
        (
            state = 'closed' AND closed_by IS NOT NULL AND closed_at IS NOT NULL AND close_reason IS NOT NULL
            AND length(close_reason) BETWEEN 5 AND 500
        )
        OR (state <> 'closed' AND closed_by IS NULL AND closed_at IS NULL AND close_reason IS NULL)
    )
);
CREATE INDEX inventory_purchase_orders_supplier_idx ON app.inventory_purchase_orders (supplier_id, order_date DESC);
CREATE INDEX inventory_purchase_orders_state_idx ON app.inventory_purchase_orders (state, order_date DESC);

-- 4. Renglones de una orden: insumo, cantidad capturada en una presentación o en una unidad de su
-- dimensión (exactamente una), cantidad base convertida, precio unitario sin IVA, descuento
-- comercial en pesos, tasa de IVA 16/8/0 y subtotal (importe menos descuento), IVA y total. Solo se
-- reemplazan mientras la orden es borrador (disparador de abajo).
CREATE TABLE app.inventory_purchase_order_lines (
    order_id uuid NOT NULL CONSTRAINT inventory_purchase_order_lines_order_id_fkey REFERENCES app.inventory_purchase_orders(id),
    line_no integer NOT NULL CONSTRAINT inventory_purchase_order_lines_line_no_check CHECK (line_no BETWEEN 1 AND 200),
    item_id uuid NOT NULL CONSTRAINT inventory_purchase_order_lines_item_id_fkey REFERENCES app.inventory_items(id),
    presentation_id uuid CONSTRAINT inventory_purchase_order_lines_presentation_id_fkey REFERENCES app.inventory_presentations(id),
    unit_id uuid CONSTRAINT inventory_purchase_order_lines_unit_id_fkey REFERENCES app.inventory_units(id),
    quantity numeric(20,6) NOT NULL CONSTRAINT inventory_purchase_order_lines_quantity_check
        CHECK (quantity > 0 AND quantity <= 1000000000),
    base_quantity numeric(20,6) NOT NULL CONSTRAINT inventory_purchase_order_lines_base_quantity_check
        CHECK (base_quantity > 0 AND base_quantity <= 1000000000),
    unit_price numeric(18,6) NOT NULL CONSTRAINT inventory_purchase_order_lines_unit_price_check
        CHECK (unit_price >= 0 AND unit_price <= 1000000),
    discount numeric(18,2) NOT NULL DEFAULT 0 CONSTRAINT inventory_purchase_order_lines_discount_check CHECK (discount >= 0),
    tax_rate integer NOT NULL CONSTRAINT inventory_purchase_order_lines_tax_rate_check CHECK (tax_rate IN (16, 8, 0)),
    subtotal numeric(18,2) NOT NULL CONSTRAINT inventory_purchase_order_lines_subtotal_check CHECK (subtotal >= 0),
    tax numeric(18,2) NOT NULL CONSTRAINT inventory_purchase_order_lines_tax_check CHECK (tax >= 0),
    total numeric(18,2) NOT NULL,
    CONSTRAINT inventory_purchase_order_lines_pkey PRIMARY KEY (order_id, line_no),
    CONSTRAINT inventory_purchase_order_lines_capture_check CHECK (
        (presentation_id IS NOT NULL AND unit_id IS NULL) OR (presentation_id IS NULL AND unit_id IS NOT NULL)
    ),
    CONSTRAINT inventory_purchase_order_lines_total_check CHECK (total = subtotal + tax)
);
CREATE UNIQUE INDEX inventory_purchase_order_lines_item_key ON app.inventory_purchase_order_lines (order_id, item_id);

-- 5. Recepciones de mercancía (documento inmutable). Folio propio, proveedor obligatorio (también en
-- la recepción directa, sin orden), orden opcional, almacén, hora de la recepción, quién recibió,
-- referencia del proveedor (factura o remisión), tax_in_cost copiado de la configuración, importes
-- del documento, valor de inventario y si se aceptó explícitamente recibir más de lo pendiente.
-- Idempotente por operation_id. Una recepción no representa un pago.
CREATE TABLE app.inventory_receipts (
    id uuid PRIMARY KEY,
    folio bigint GENERATED ALWAYS AS IDENTITY,
    supplier_id uuid NOT NULL CONSTRAINT inventory_receipts_supplier_id_fkey REFERENCES app.inventory_suppliers(id),
    purchase_order_id uuid CONSTRAINT inventory_receipts_purchase_order_id_fkey REFERENCES app.inventory_purchase_orders(id),
    warehouse_id uuid NOT NULL CONSTRAINT inventory_receipts_warehouse_id_fkey REFERENCES app.inventory_warehouses(id),
    occurred_at timestamptz NOT NULL,
    received_by uuid NOT NULL CONSTRAINT inventory_receipts_received_by_fkey REFERENCES app.users(id),
    supplier_reference text NOT NULL DEFAULT '' CONSTRAINT inventory_receipts_supplier_reference_check
        CHECK (length(supplier_reference) <= 60 AND supplier_reference = btrim(supplier_reference)),
    tax_in_cost boolean NOT NULL,
    subtotal numeric(18,2) NOT NULL CONSTRAINT inventory_receipts_subtotal_check CHECK (subtotal >= 0),
    discount numeric(18,2) NOT NULL CONSTRAINT inventory_receipts_discount_check CHECK (discount >= 0),
    tax numeric(18,2) NOT NULL CONSTRAINT inventory_receipts_tax_check CHECK (tax >= 0),
    total numeric(18,2) NOT NULL,
    inventory_value numeric(20,6) NOT NULL CONSTRAINT inventory_receipts_inventory_value_check CHECK (inventory_value >= 0),
    line_count integer NOT NULL CONSTRAINT inventory_receipts_line_count_check CHECK (line_count BETWEEN 1 AND 200),
    excess boolean NOT NULL DEFAULT false,
    notes text NOT NULL DEFAULT '' CONSTRAINT inventory_receipts_notes_check CHECK (length(notes) <= 2000),
    operation_id uuid NOT NULL CONSTRAINT inventory_receipts_operation_id_fkey
        REFERENCES app.restaurant_operations(id) DEFERRABLE INITIALLY DEFERRED,
    created_at timestamptz NOT NULL DEFAULT clock_timestamp(),
    CONSTRAINT inventory_receipts_folio_key UNIQUE (folio),
    CONSTRAINT inventory_receipts_operation_id_key UNIQUE (operation_id),
    -- Permite que los renglones referencien la orden de su recepción (llave compuesta de abajo).
    CONSTRAINT inventory_receipts_order_key UNIQUE (id, purchase_order_id),
    CONSTRAINT inventory_receipts_total_check CHECK (total = subtotal + tax),
    -- El valor de inventario es el total con IVA o el subtotal sin IVA, según la configuración copiada.
    CONSTRAINT inventory_receipts_value_check CHECK (
        (tax_in_cost AND inventory_value = total) OR (NOT tax_in_cost AND inventory_value = subtotal)
    ),
    -- Solo una recepción de una orden puede exceder lo pendiente.
    CONSTRAINT inventory_receipts_excess_check CHECK (NOT excess OR purchase_order_id IS NOT NULL)
);
CREATE INDEX inventory_receipts_supplier_idx ON app.inventory_receipts (supplier_id, occurred_at DESC);
CREATE INDEX inventory_receipts_order_idx ON app.inventory_receipts (purchase_order_id) WHERE purchase_order_id IS NOT NULL;
CREATE INDEX inventory_receipts_occurred_idx ON app.inventory_receipts (occurred_at DESC, id);
-- El último precio de un insumo por proveedor apunta a la recepción que lo fijó.
ALTER TABLE app.inventory_supplier_items
    ADD CONSTRAINT inventory_supplier_items_last_receipt_id_fkey FOREIGN KEY (last_receipt_id)
        REFERENCES app.inventory_receipts(id);

-- 6. Renglones de una recepción (inmutables): el renglón de la orden que recibe (si lo hay), insumo,
-- cantidad capturada y base, importes, valor de inventario, costo por unidad base y el movimiento
-- receipt que registró en el libro.
CREATE TABLE app.inventory_receipt_lines (
    receipt_id uuid NOT NULL CONSTRAINT inventory_receipt_lines_receipt_id_fkey REFERENCES app.inventory_receipts(id),
    line_no integer NOT NULL CONSTRAINT inventory_receipt_lines_line_no_check CHECK (line_no BETWEEN 1 AND 200),
    order_id uuid,
    order_line_no integer,
    item_id uuid NOT NULL CONSTRAINT inventory_receipt_lines_item_id_fkey REFERENCES app.inventory_items(id),
    presentation_id uuid CONSTRAINT inventory_receipt_lines_presentation_id_fkey REFERENCES app.inventory_presentations(id),
    unit_id uuid CONSTRAINT inventory_receipt_lines_unit_id_fkey REFERENCES app.inventory_units(id),
    quantity numeric(20,6) NOT NULL CONSTRAINT inventory_receipt_lines_quantity_check
        CHECK (quantity > 0 AND quantity <= 1000000000),
    base_quantity numeric(20,6) NOT NULL CONSTRAINT inventory_receipt_lines_base_quantity_check
        CHECK (base_quantity > 0 AND base_quantity <= 1000000000),
    unit_price numeric(18,6) NOT NULL CONSTRAINT inventory_receipt_lines_unit_price_check
        CHECK (unit_price >= 0 AND unit_price <= 1000000),
    discount numeric(18,2) NOT NULL CONSTRAINT inventory_receipt_lines_discount_check CHECK (discount >= 0),
    tax_rate integer NOT NULL CONSTRAINT inventory_receipt_lines_tax_rate_check CHECK (tax_rate IN (16, 8, 0)),
    subtotal numeric(18,2) NOT NULL CONSTRAINT inventory_receipt_lines_subtotal_check CHECK (subtotal >= 0),
    tax numeric(18,2) NOT NULL CONSTRAINT inventory_receipt_lines_tax_check CHECK (tax >= 0),
    total numeric(18,2) NOT NULL,
    inventory_value numeric(20,6) NOT NULL CONSTRAINT inventory_receipt_lines_inventory_value_check CHECK (inventory_value >= 0),
    unit_cost numeric(24,10) NOT NULL CONSTRAINT inventory_receipt_lines_unit_cost_check CHECK (unit_cost >= 0),
    movement_id uuid NOT NULL CONSTRAINT inventory_receipt_lines_movement_id_fkey REFERENCES app.inventory_movements(id),
    CONSTRAINT inventory_receipt_lines_pkey PRIMARY KEY (receipt_id, line_no),
    CONSTRAINT inventory_receipt_lines_movement_id_key UNIQUE (movement_id),
    -- El renglón de orden pertenece a la orden de su recepción.
    CONSTRAINT inventory_receipt_lines_receipt_order_fkey FOREIGN KEY (receipt_id, order_id)
        REFERENCES app.inventory_receipts(id, purchase_order_id),
    CONSTRAINT inventory_receipt_lines_order_line_fkey FOREIGN KEY (order_id, order_line_no)
        REFERENCES app.inventory_purchase_order_lines(order_id, line_no),
    CONSTRAINT inventory_receipt_lines_order_check CHECK (
        (order_id IS NULL AND order_line_no IS NULL) OR (order_id IS NOT NULL AND order_line_no IS NOT NULL)
    ),
    CONSTRAINT inventory_receipt_lines_capture_check CHECK (
        (presentation_id IS NOT NULL AND unit_id IS NULL) OR (presentation_id IS NULL AND unit_id IS NOT NULL)
    ),
    CONSTRAINT inventory_receipt_lines_total_check CHECK (total = subtotal + tax)
);
CREATE INDEX inventory_receipt_lines_order_line_idx ON app.inventory_receipt_lines (order_id, order_line_no)
    WHERE order_id IS NOT NULL;
CREATE INDEX inventory_receipt_lines_item_idx ON app.inventory_receipt_lines (item_id);

-- 7. Devoluciones a proveedor (documento inmutable), siempre ligadas a una recepción: proveedor de
-- la recepción, almacén del que sale la mercancía, hora, motivo, responsable, valor asociado (a costo
-- original de la recepción) y la diferencia que la existencia restante no pudo absorber (regla 5 del
-- motor de costos). No crea ingresos de efectivo ni devoluciones bancarias. Idempotente por
-- operation_id.
CREATE TABLE app.inventory_supplier_returns (
    id uuid PRIMARY KEY,
    folio bigint GENERATED ALWAYS AS IDENTITY,
    receipt_id uuid NOT NULL CONSTRAINT inventory_supplier_returns_receipt_id_fkey REFERENCES app.inventory_receipts(id),
    supplier_id uuid NOT NULL CONSTRAINT inventory_supplier_returns_supplier_id_fkey REFERENCES app.inventory_suppliers(id),
    warehouse_id uuid NOT NULL CONSTRAINT inventory_supplier_returns_warehouse_id_fkey REFERENCES app.inventory_warehouses(id),
    occurred_at timestamptz NOT NULL,
    reason text NOT NULL CONSTRAINT inventory_supplier_returns_reason_check CHECK (length(reason) BETWEEN 5 AND 500),
    actor_id uuid NOT NULL CONSTRAINT inventory_supplier_returns_actor_id_fkey REFERENCES app.users(id),
    value numeric(20,6) NOT NULL CONSTRAINT inventory_supplier_returns_value_check CHECK (value >= 0),
    unabsorbed numeric(20,6) NOT NULL DEFAULT 0,
    line_count integer NOT NULL CONSTRAINT inventory_supplier_returns_line_count_check CHECK (line_count BETWEEN 1 AND 200),
    operation_id uuid NOT NULL CONSTRAINT inventory_supplier_returns_operation_id_fkey
        REFERENCES app.restaurant_operations(id) DEFERRABLE INITIALLY DEFERRED,
    created_at timestamptz NOT NULL DEFAULT clock_timestamp(),
    CONSTRAINT inventory_supplier_returns_folio_key UNIQUE (folio),
    CONSTRAINT inventory_supplier_returns_operation_id_key UNIQUE (operation_id),
    CONSTRAINT inventory_supplier_returns_receipt_key UNIQUE (id, receipt_id)
);
CREATE INDEX inventory_supplier_returns_receipt_idx ON app.inventory_supplier_returns (receipt_id);
CREATE INDEX inventory_supplier_returns_supplier_idx ON app.inventory_supplier_returns (supplier_id, occurred_at DESC);

-- 8. Renglones de una devolución (inmutables): el renglón de la recepción que devuelve, cantidad
-- capturada (en la presentación o unidad de esa recepción) y base, costo unitario original, valor a
-- costo original, valor de salida al promedio vigente, ajuste de costo separado (si la existencia
-- restante lo absorbió) y diferencia no absorbida, con sus movimientos.
CREATE TABLE app.inventory_supplier_return_lines (
    return_id uuid NOT NULL CONSTRAINT inventory_supplier_return_lines_return_id_fkey REFERENCES app.inventory_supplier_returns(id),
    line_no integer NOT NULL CONSTRAINT inventory_supplier_return_lines_line_no_check CHECK (line_no BETWEEN 1 AND 200),
    receipt_id uuid NOT NULL,
    receipt_line_no integer NOT NULL,
    item_id uuid NOT NULL CONSTRAINT inventory_supplier_return_lines_item_id_fkey REFERENCES app.inventory_items(id),
    quantity numeric(20,6) NOT NULL CONSTRAINT inventory_supplier_return_lines_quantity_check
        CHECK (quantity > 0 AND quantity <= 1000000000),
    base_quantity numeric(20,6) NOT NULL CONSTRAINT inventory_supplier_return_lines_base_quantity_check
        CHECK (base_quantity > 0 AND base_quantity <= 1000000000),
    original_unit_cost numeric(24,10) NOT NULL CONSTRAINT inventory_supplier_return_lines_original_unit_cost_check
        CHECK (original_unit_cost >= 0),
    value numeric(20,6) NOT NULL CONSTRAINT inventory_supplier_return_lines_value_check CHECK (value >= 0),
    outflow_value numeric(20,6) NOT NULL CONSTRAINT inventory_supplier_return_lines_outflow_value_check CHECK (outflow_value >= 0),
    cost_adjustment numeric(20,6) NOT NULL DEFAULT 0,
    unabsorbed numeric(20,6) NOT NULL DEFAULT 0,
    movement_id uuid NOT NULL CONSTRAINT inventory_supplier_return_lines_movement_id_fkey REFERENCES app.inventory_movements(id),
    cost_adjustment_movement_id uuid CONSTRAINT inventory_supplier_return_lines_adjustment_movement_id_fkey
        REFERENCES app.inventory_movements(id),
    CONSTRAINT inventory_supplier_return_lines_pkey PRIMARY KEY (return_id, line_no),
    CONSTRAINT inventory_supplier_return_lines_movement_id_key UNIQUE (movement_id),
    -- Un renglón por renglón de recepción en cada devolución, de la misma recepción del documento.
    CONSTRAINT inventory_supplier_return_lines_receipt_line_key UNIQUE (return_id, receipt_line_no),
    CONSTRAINT inventory_supplier_return_lines_return_receipt_fkey FOREIGN KEY (return_id, receipt_id)
        REFERENCES app.inventory_supplier_returns(id, receipt_id),
    CONSTRAINT inventory_supplier_return_lines_receipt_line_fkey FOREIGN KEY (receipt_id, receipt_line_no)
        REFERENCES app.inventory_receipt_lines(receipt_id, line_no),
    -- El ajuste de costo tiene su movimiento y viceversa; la diferencia se reparte entre ajuste y no
    -- absorbida: value − outflow_value = −cost_adjustment + unabsorbed.
    CONSTRAINT inventory_supplier_return_lines_adjustment_check CHECK (
        (cost_adjustment = 0 AND cost_adjustment_movement_id IS NULL)
        OR (cost_adjustment <> 0 AND cost_adjustment_movement_id IS NOT NULL)
    ),
    CONSTRAINT inventory_supplier_return_lines_difference_check CHECK (
        value - outflow_value = unabsorbed - cost_adjustment
    )
);
CREATE INDEX inventory_supplier_return_lines_receipt_line_idx ON app.inventory_supplier_return_lines (receipt_id, receipt_line_no);

-- 9. Vínculos opcionales de una recepción con gastos de caja (§20). Solo referencian el gasto: nunca
-- modifican cash_operations, el turno ni su cierre. Se admiten vínculos parciales: Go valida bajo los
-- locks 7820148305 (compartido) y 7820148308 que Σ vínculos vigentes por gasto ≤ importe del gasto y
-- Σ por recepción ≤ total de la recepción. Desvincular es un registro (quién, cuándo, motivo y su
-- operación), no un borrado; un gasto corregido o regularizado después se detecta al leer.
CREATE TABLE app.inventory_receipt_cash_links (
    id uuid PRIMARY KEY,
    receipt_id uuid NOT NULL CONSTRAINT inventory_receipt_cash_links_receipt_id_fkey REFERENCES app.inventory_receipts(id),
    cash_operation_id uuid NOT NULL CONSTRAINT inventory_receipt_cash_links_cash_operation_id_fkey
        REFERENCES app.cash_operations(id),
    amount numeric(18,2) NOT NULL CONSTRAINT inventory_receipt_cash_links_amount_check CHECK (amount > 0),
    notes text NOT NULL DEFAULT '' CONSTRAINT inventory_receipt_cash_links_notes_check CHECK (length(notes) <= 500),
    linked_by uuid NOT NULL CONSTRAINT inventory_receipt_cash_links_linked_by_fkey REFERENCES app.users(id),
    linked_at timestamptz NOT NULL DEFAULT clock_timestamp(),
    operation_id uuid NOT NULL CONSTRAINT inventory_receipt_cash_links_operation_id_fkey
        REFERENCES app.restaurant_operations(id) DEFERRABLE INITIALLY DEFERRED,
    unlinked_by uuid CONSTRAINT inventory_receipt_cash_links_unlinked_by_fkey REFERENCES app.users(id),
    unlinked_at timestamptz,
    unlink_reason text,
    unlink_operation_id uuid CONSTRAINT inventory_receipt_cash_links_unlink_operation_id_fkey
        REFERENCES app.restaurant_operations(id) DEFERRABLE INITIALLY DEFERRED,
    CONSTRAINT inventory_receipt_cash_links_operation_id_key UNIQUE (operation_id),
    CONSTRAINT inventory_receipt_cash_links_unlink_operation_id_key UNIQUE (unlink_operation_id),
    CONSTRAINT inventory_receipt_cash_links_unlink_check CHECK (
        (unlinked_by IS NULL AND unlinked_at IS NULL AND unlink_reason IS NULL AND unlink_operation_id IS NULL)
        OR (
            unlinked_by IS NOT NULL AND unlinked_at IS NOT NULL AND unlink_operation_id IS NOT NULL
            AND unlink_reason IS NOT NULL AND length(unlink_reason) BETWEEN 5 AND 500
        )
    )
);
-- Un solo vínculo vigente entre la misma recepción y el mismo gasto (para cambiar el importe se
-- desvincula y se vincula de nuevo).
CREATE UNIQUE INDEX inventory_receipt_cash_links_active_key ON app.inventory_receipt_cash_links (receipt_id, cash_operation_id)
    WHERE unlinked_at IS NULL;
CREATE INDEX inventory_receipt_cash_links_cash_operation_idx ON app.inventory_receipt_cash_links (cash_operation_id);
CREATE INDEX inventory_receipt_cash_links_receipt_idx ON app.inventory_receipt_cash_links (receipt_id, linked_at);

-- Los renglones de una orden solo se agregan, cambian o quitan mientras la orden es borrador: después
-- de emitirla son el compromiso que reciben las recepciones.
CREATE FUNCTION app.inventory_purchase_order_lines_guard() RETURNS trigger
    LANGUAGE plpgsql AS $$
DECLARE
    current_state text;
BEGIN
    IF TG_OP = 'INSERT' THEN
        SELECT state INTO current_state FROM app.inventory_purchase_orders WHERE id = NEW.order_id;
    ELSE
        SELECT state INTO current_state FROM app.inventory_purchase_orders WHERE id = OLD.order_id;
    END IF;
    IF current_state IS DISTINCT FROM 'draft' THEN
        RAISE EXCEPTION 'Los renglones de una orden de compra emitida no se modifican' USING ERRCODE = '55000';
    END IF;
    IF TG_OP = 'DELETE' THEN
        RETURN OLD;
    END IF;
    RETURN NEW;
END;
$$;
CREATE TRIGGER inventory_purchase_order_lines_draft_only
    BEFORE INSERT OR UPDATE OR DELETE ON app.inventory_purchase_order_lines
    FOR EACH ROW EXECUTE FUNCTION app.inventory_purchase_order_lines_guard();
CREATE TRIGGER inventory_purchase_order_lines_no_truncate
    BEFORE TRUNCATE ON app.inventory_purchase_order_lines
    FOR EACH STATEMENT EXECUTE FUNCTION app.inventory_reject_mutation();

-- Recepciones, devoluciones y sus renglones son de solo inserción también en la base (la función de
-- rechazo es la de la 0010): toda corrección es un documento nuevo.
CREATE TRIGGER inventory_receipts_insert_only
    BEFORE UPDATE OR DELETE ON app.inventory_receipts
    FOR EACH ROW EXECUTE FUNCTION app.inventory_reject_mutation();
CREATE TRIGGER inventory_receipts_no_truncate
    BEFORE TRUNCATE ON app.inventory_receipts
    FOR EACH STATEMENT EXECUTE FUNCTION app.inventory_reject_mutation();
CREATE TRIGGER inventory_receipt_lines_insert_only
    BEFORE UPDATE OR DELETE ON app.inventory_receipt_lines
    FOR EACH ROW EXECUTE FUNCTION app.inventory_reject_mutation();
CREATE TRIGGER inventory_receipt_lines_no_truncate
    BEFORE TRUNCATE ON app.inventory_receipt_lines
    FOR EACH STATEMENT EXECUTE FUNCTION app.inventory_reject_mutation();
CREATE TRIGGER inventory_supplier_returns_insert_only
    BEFORE UPDATE OR DELETE ON app.inventory_supplier_returns
    FOR EACH ROW EXECUTE FUNCTION app.inventory_reject_mutation();
CREATE TRIGGER inventory_supplier_returns_no_truncate
    BEFORE TRUNCATE ON app.inventory_supplier_returns
    FOR EACH STATEMENT EXECUTE FUNCTION app.inventory_reject_mutation();
CREATE TRIGGER inventory_supplier_return_lines_insert_only
    BEFORE UPDATE OR DELETE ON app.inventory_supplier_return_lines
    FOR EACH ROW EXECUTE FUNCTION app.inventory_reject_mutation();
CREATE TRIGGER inventory_supplier_return_lines_no_truncate
    BEFORE TRUNCATE ON app.inventory_supplier_return_lines
    FOR EACH STATEMENT EXECUTE FUNCTION app.inventory_reject_mutation();

-- Un vínculo con un gasto no se borra ni se modifica: la única actualización admitida es registrar
-- su desvinculación una vez, sin tocar ningún otro dato.
CREATE FUNCTION app.inventory_cash_link_guard() RETURNS trigger
    LANGUAGE plpgsql AS $$
BEGIN
    IF TG_OP = 'UPDATE' AND OLD.unlinked_at IS NULL AND NEW.unlinked_at IS NOT NULL
        AND NEW.id = OLD.id
        AND NEW.receipt_id = OLD.receipt_id
        AND NEW.cash_operation_id = OLD.cash_operation_id
        AND NEW.amount = OLD.amount
        AND NEW.notes = OLD.notes
        AND NEW.linked_by = OLD.linked_by
        AND NEW.linked_at = OLD.linked_at
        AND NEW.operation_id = OLD.operation_id THEN
        RETURN NEW;
    END IF;
    RAISE EXCEPTION 'Los vínculos con gastos de caja no se modifican ni se borran: se desvinculan con un registro'
        USING ERRCODE = '55000';
END;
$$;
CREATE TRIGGER inventory_receipt_cash_links_guard
    BEFORE UPDATE OR DELETE ON app.inventory_receipt_cash_links
    FOR EACH ROW EXECUTE FUNCTION app.inventory_cash_link_guard();
CREATE TRIGGER inventory_receipt_cash_links_no_truncate
    BEFORE TRUNCATE ON app.inventory_receipt_cash_links
    FOR EACH STATEMENT EXECUTE FUNCTION app.inventory_reject_mutation();

-- 10. Permisos de la fase C. Se registran también en Go (inventory.Permissions) con la misma
-- descripción y se conceden solo al administrador; el rol «Inventarios» no recibe permisos por
-- inferencia.
INSERT INTO app.permissions(key, description) VALUES
    ('inventory.suppliers.manage', 'Administrar proveedores'),
    ('inventory.purchases.create', 'Crear órdenes de compra'),
    ('inventory.purchases.authorize', 'Emitir (autorizar), cancelar y cerrar órdenes de compra'),
    ('inventory.purchases.receive', 'Recibir mercancía'),
    ('inventory.supplier_returns.record', 'Registrar devoluciones a proveedores'),
    ('inventory.purchases.link_cash', 'Vincular compras con gastos de caja');

INSERT INTO app.role_permissions(role_id, permission_key)
SELECT r.id, p.key FROM app.roles r CROSS JOIN app.permissions p
WHERE r.code = 'administrator' AND p.key IN (
    'inventory.suppliers.manage', 'inventory.purchases.create', 'inventory.purchases.authorize',
    'inventory.purchases.receive', 'inventory.supplier_returns.record', 'inventory.purchases.link_cash'
)
ON CONFLICT DO NOTHING;
