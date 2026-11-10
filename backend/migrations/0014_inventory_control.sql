-- Etapa 8, fase D: control de inventario. Ajustes de discrepancia y mermas operativas con sus
-- renglones, transferencias entre almacenes con sus renglones, conteos físicos por almacén con sus
-- renglones y niveles de existencia por almacén (mínimo y punto de reabastecimiento), con los
-- permisos de la fase D.
-- Transaccional: el migrador la aplica dentro de la misma transacción que la registra en
-- schema_migrations y no contiene sentencias que no admitan transacción (ni CREATE INDEX
-- CONCURRENTLY ni VACUUM), de modo que un fallo la revierte completa. No modifica 0001–0013 ni filas
-- existentes y no siembra datos (ni ajustes, ni conteos, ni niveles: los registra el restaurante). Las
-- columnas opcionales llevan CHECK que exigen IS NOT NULL en la rama que valida, porque un CHECK que
-- evalúa a NULL se acepta.
--
-- Ninguna de estas tablas cambia existencias por sí misma: todo efecto en existencias y costos es un
-- movimiento del libro (inventory_movements) que registra Go con el motor de costos; los documentos
-- guardan la referencia a sus movimientos. Cantidades numeric(20,6) en la unidad base del insumo,
-- capturadas además tal como se escribieron (presentación o unidad de su dimensión).

-- 1. Ajustes de inventario. Un ajuste de discrepancia (kind discrepancy) corrige existencias por
-- rotura, derrame, caducidad, faltante o sobrante físico, error de registro, consumo extraordinario u
-- otro motivo con texto; si quien lo registra no puede autorizarlo queda pendiente hasta que otra
-- persona lo autorice o lo rechace (nunca se autoriza a sí mismo). Una merma operativa (kind waste)
-- se aplica directamente y solo registra salidas. Un ajuste aplicado o rechazado ya no cambia.
CREATE TABLE app.inventory_adjustments (
    id uuid PRIMARY KEY,
    folio bigint GENERATED ALWAYS AS IDENTITY,
    kind text NOT NULL CONSTRAINT inventory_adjustments_kind_check CHECK (kind IN ('discrepancy', 'waste')),
    reason text NOT NULL CONSTRAINT inventory_adjustments_reason_check CHECK (
        reason IN (
            'breakage', 'spill', 'expiry', 'physical_shortage', 'surplus', 'entry_error', 'extraordinary_use', 'other'
        )
    ),
    reason_detail text NOT NULL DEFAULT '' CONSTRAINT inventory_adjustments_reason_detail_check
        CHECK (length(reason_detail) <= 200 AND reason_detail = btrim(reason_detail)),
    warehouse_id uuid NOT NULL CONSTRAINT inventory_adjustments_warehouse_id_fkey REFERENCES app.inventory_warehouses(id),
    state text NOT NULL CONSTRAINT inventory_adjustments_state_check
        CHECK (state IN ('pending_authorization', 'applied', 'rejected')),
    notes text NOT NULL DEFAULT '' CONSTRAINT inventory_adjustments_notes_check CHECK (length(notes) <= 500),
    line_count integer NOT NULL CONSTRAINT inventory_adjustments_line_count_check CHECK (line_count BETWEEN 1 AND 200),
    requested_by uuid NOT NULL CONSTRAINT inventory_adjustments_requested_by_fkey REFERENCES app.users(id),
    requested_at timestamptz NOT NULL DEFAULT now(),
    authorized_by uuid CONSTRAINT inventory_adjustments_authorized_by_fkey REFERENCES app.users(id),
    authorized_at timestamptz,
    applied_at timestamptz,
    rejected_by uuid CONSTRAINT inventory_adjustments_rejected_by_fkey REFERENCES app.users(id),
    rejected_at timestamptz,
    rejection_reason text,
    revision integer NOT NULL DEFAULT 1 CONSTRAINT inventory_adjustments_revision_check CHECK (revision > 0),
    operation_id uuid NOT NULL CONSTRAINT inventory_adjustments_operation_id_fkey
        REFERENCES app.restaurant_operations(id) DEFERRABLE INITIALLY DEFERRED,
    decision_operation_id uuid CONSTRAINT inventory_adjustments_decision_operation_id_fkey
        REFERENCES app.restaurant_operations(id) DEFERRABLE INITIALLY DEFERRED,
    updated_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT inventory_adjustments_folio_key UNIQUE (folio),
    CONSTRAINT inventory_adjustments_operation_id_key UNIQUE (operation_id),
    CONSTRAINT inventory_adjustments_decision_operation_id_key UNIQUE (decision_operation_id),
    -- «Otro» exige el texto del motivo.
    CONSTRAINT inventory_adjustments_other_check CHECK (reason <> 'other' OR length(reason_detail) >= 3),
    -- Una merma operativa es una pérdida: no admite sobrante, faltante físico ni error de registro.
    CONSTRAINT inventory_adjustments_waste_reason_check
        CHECK (kind <> 'waste' OR reason IN ('breakage', 'spill', 'expiry', 'extraordinary_use', 'other')),
    -- Estados: pendiente (solo discrepancias, sin decisión); aplicado directamente (sin autorización
    -- separada) o autorizado por otra persona; rechazado por otra persona con motivo (5–500).
    CONSTRAINT inventory_adjustments_state_shape_check CHECK (
        (
            state = 'pending_authorization' AND kind = 'discrepancy' AND authorized_by IS NULL AND authorized_at IS NULL
            AND applied_at IS NULL AND rejected_by IS NULL AND rejected_at IS NULL AND rejection_reason IS NULL
            AND decision_operation_id IS NULL
        )
        OR (
            state = 'applied' AND applied_at IS NOT NULL AND rejected_by IS NULL AND rejected_at IS NULL
            AND rejection_reason IS NULL
            AND (
                (authorized_by IS NULL AND authorized_at IS NULL AND decision_operation_id IS NULL)
                OR (
                    kind = 'discrepancy' AND authorized_by IS NOT NULL AND authorized_at IS NOT NULL
                    AND decision_operation_id IS NOT NULL AND authorized_by <> requested_by
                )
            )
        )
        OR (
            state = 'rejected' AND kind = 'discrepancy' AND rejected_by IS NOT NULL AND rejected_at IS NOT NULL
            AND rejection_reason IS NOT NULL AND length(rejection_reason) BETWEEN 5 AND 500
            AND decision_operation_id IS NOT NULL AND rejected_by <> requested_by AND applied_at IS NULL
            AND authorized_by IS NULL AND authorized_at IS NULL
        )
    )
);
CREATE INDEX inventory_adjustments_state_idx ON app.inventory_adjustments (state, requested_at DESC);
CREATE INDEX inventory_adjustments_warehouse_idx ON app.inventory_adjustments (warehouse_id, requested_at DESC);

-- 2. Renglones de un ajuste: insumo, cantidad con signo capturada en una presentación o en una unidad
-- de su dimensión (exactamente una) y convertida a la unidad base, y el movimiento que la aplicó (vacío
-- mientras el ajuste está pendiente). Un insumo una sola vez por ajuste.
CREATE TABLE app.inventory_adjustment_lines (
    adjustment_id uuid NOT NULL CONSTRAINT inventory_adjustment_lines_adjustment_id_fkey
        REFERENCES app.inventory_adjustments(id),
    line_no integer NOT NULL CONSTRAINT inventory_adjustment_lines_line_no_check CHECK (line_no BETWEEN 1 AND 200),
    item_id uuid NOT NULL CONSTRAINT inventory_adjustment_lines_item_id_fkey REFERENCES app.inventory_items(id),
    presentation_id uuid CONSTRAINT inventory_adjustment_lines_presentation_id_fkey REFERENCES app.inventory_presentations(id),
    unit_id uuid CONSTRAINT inventory_adjustment_lines_unit_id_fkey REFERENCES app.inventory_units(id),
    captured_quantity numeric(20,6) NOT NULL CONSTRAINT inventory_adjustment_lines_captured_quantity_check
        CHECK (captured_quantity <> 0 AND abs(captured_quantity) <= 1000000000),
    quantity numeric(20,6) NOT NULL CONSTRAINT inventory_adjustment_lines_quantity_check
        CHECK (quantity <> 0 AND abs(quantity) <= 1000000000),
    movement_id uuid CONSTRAINT inventory_adjustment_lines_movement_id_fkey REFERENCES app.inventory_movements(id),
    CONSTRAINT inventory_adjustment_lines_pkey PRIMARY KEY (adjustment_id, line_no),
    CONSTRAINT inventory_adjustment_lines_item_key UNIQUE (adjustment_id, item_id),
    CONSTRAINT inventory_adjustment_lines_movement_id_key UNIQUE (movement_id),
    CONSTRAINT inventory_adjustment_lines_capture_check CHECK (
        (presentation_id IS NOT NULL AND unit_id IS NULL) OR (presentation_id IS NULL AND unit_id IS NOT NULL)
    ),
    CONSTRAINT inventory_adjustment_lines_sign_check CHECK (sign(captured_quantity) = sign(quantity))
);
CREATE INDEX inventory_adjustment_lines_item_idx ON app.inventory_adjustment_lines (item_id);

-- Un ajuste solo cambia mientras está pendiente (autorizarlo o rechazarlo) y sin tocar lo que se
-- registró; nunca se borra.
CREATE FUNCTION app.inventory_adjustment_guard() RETURNS trigger
    LANGUAGE plpgsql AS $$
BEGIN
    IF TG_OP = 'UPDATE' AND OLD.state = 'pending_authorization'
        AND NEW.id = OLD.id
        AND NEW.folio = OLD.folio
        AND NEW.kind = OLD.kind
        AND NEW.reason = OLD.reason
        AND NEW.reason_detail = OLD.reason_detail
        AND NEW.warehouse_id = OLD.warehouse_id
        AND NEW.notes = OLD.notes
        AND NEW.line_count = OLD.line_count
        AND NEW.requested_by = OLD.requested_by
        AND NEW.requested_at = OLD.requested_at
        AND NEW.operation_id = OLD.operation_id THEN
        RETURN NEW;
    END IF;
    RAISE EXCEPTION 'Los ajustes de inventario no se modifican ni se borran: solo se autoriza o rechaza uno pendiente'
        USING ERRCODE = '55000';
END;
$$;
CREATE TRIGGER inventory_adjustments_guard
    BEFORE UPDATE OR DELETE ON app.inventory_adjustments
    FOR EACH ROW EXECUTE FUNCTION app.inventory_adjustment_guard();
CREATE TRIGGER inventory_adjustments_no_truncate
    BEFORE TRUNCATE ON app.inventory_adjustments
    FOR EACH STATEMENT EXECUTE FUNCTION app.inventory_reject_mutation();

-- Los renglones de una merma son salidas; la única actualización admitida es registrar una vez el
-- movimiento que aplicó el renglón (al autorizar un ajuste pendiente). No se borran.
CREATE FUNCTION app.inventory_adjustment_line_guard() RETURNS trigger
    LANGUAGE plpgsql AS $$
DECLARE
    parent_kind text;
BEGIN
    IF TG_OP = 'INSERT' THEN
        SELECT kind INTO parent_kind FROM app.inventory_adjustments WHERE id = NEW.adjustment_id;
        IF parent_kind = 'waste' AND NEW.quantity >= 0 THEN
            RAISE EXCEPTION 'Una merma operativa solo registra salidas' USING ERRCODE = '23514';
        END IF;
        RETURN NEW;
    END IF;
    IF TG_OP = 'UPDATE' AND OLD.movement_id IS NULL AND NEW.movement_id IS NOT NULL
        AND NEW.adjustment_id = OLD.adjustment_id
        AND NEW.line_no = OLD.line_no
        AND NEW.item_id = OLD.item_id
        AND NEW.presentation_id IS NOT DISTINCT FROM OLD.presentation_id
        AND NEW.unit_id IS NOT DISTINCT FROM OLD.unit_id
        AND NEW.captured_quantity = OLD.captured_quantity
        AND NEW.quantity = OLD.quantity THEN
        RETURN NEW;
    END IF;
    RAISE EXCEPTION 'Los renglones de un ajuste no se modifican ni se borran' USING ERRCODE = '55000';
END;
$$;
CREATE TRIGGER inventory_adjustment_lines_guard
    BEFORE INSERT OR UPDATE OR DELETE ON app.inventory_adjustment_lines
    FOR EACH ROW EXECUTE FUNCTION app.inventory_adjustment_line_guard();
CREATE TRIGGER inventory_adjustment_lines_no_truncate
    BEFORE TRUNCATE ON app.inventory_adjustment_lines
    FOR EACH STATEMENT EXECUTE FUNCTION app.inventory_reject_mutation();

-- 3. Transferencias entre almacenes: origen distinto del destino, responsable, hora y observaciones.
-- Cada renglón es una salida y una entrada vinculadas del libro con el mismo valor (el costo del
-- insumo no cambia). Solo inserción: una transferencia equivocada se corrige con otra en sentido
-- contrario.
CREATE TABLE app.inventory_transfers (
    id uuid PRIMARY KEY,
    folio bigint GENERATED ALWAYS AS IDENTITY,
    from_warehouse_id uuid NOT NULL CONSTRAINT inventory_transfers_from_warehouse_id_fkey
        REFERENCES app.inventory_warehouses(id),
    to_warehouse_id uuid NOT NULL CONSTRAINT inventory_transfers_to_warehouse_id_fkey REFERENCES app.inventory_warehouses(id),
    occurred_at timestamptz NOT NULL,
    actor_id uuid NOT NULL CONSTRAINT inventory_transfers_actor_id_fkey REFERENCES app.users(id),
    notes text NOT NULL DEFAULT '' CONSTRAINT inventory_transfers_notes_check CHECK (length(notes) <= 500),
    line_count integer NOT NULL CONSTRAINT inventory_transfers_line_count_check CHECK (line_count BETWEEN 1 AND 200),
    operation_id uuid NOT NULL CONSTRAINT inventory_transfers_operation_id_fkey
        REFERENCES app.restaurant_operations(id) DEFERRABLE INITIALLY DEFERRED,
    created_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT inventory_transfers_folio_key UNIQUE (folio),
    CONSTRAINT inventory_transfers_operation_id_key UNIQUE (operation_id),
    CONSTRAINT inventory_transfers_warehouses_check CHECK (from_warehouse_id <> to_warehouse_id)
);
CREATE INDEX inventory_transfers_occurred_idx ON app.inventory_transfers (occurred_at DESC, folio DESC);
CREATE INDEX inventory_transfers_from_idx ON app.inventory_transfers (from_warehouse_id, occurred_at DESC);
CREATE INDEX inventory_transfers_to_idx ON app.inventory_transfers (to_warehouse_id, occurred_at DESC);

CREATE TABLE app.inventory_transfer_lines (
    transfer_id uuid NOT NULL CONSTRAINT inventory_transfer_lines_transfer_id_fkey REFERENCES app.inventory_transfers(id),
    line_no integer NOT NULL CONSTRAINT inventory_transfer_lines_line_no_check CHECK (line_no BETWEEN 1 AND 200),
    item_id uuid NOT NULL CONSTRAINT inventory_transfer_lines_item_id_fkey REFERENCES app.inventory_items(id),
    presentation_id uuid CONSTRAINT inventory_transfer_lines_presentation_id_fkey REFERENCES app.inventory_presentations(id),
    unit_id uuid CONSTRAINT inventory_transfer_lines_unit_id_fkey REFERENCES app.inventory_units(id),
    captured_quantity numeric(20,6) NOT NULL CONSTRAINT inventory_transfer_lines_captured_quantity_check
        CHECK (captured_quantity > 0 AND captured_quantity <= 1000000000),
    quantity numeric(20,6) NOT NULL CONSTRAINT inventory_transfer_lines_quantity_check
        CHECK (quantity > 0 AND quantity <= 1000000000),
    out_movement_id uuid NOT NULL CONSTRAINT inventory_transfer_lines_out_movement_id_fkey
        REFERENCES app.inventory_movements(id),
    in_movement_id uuid NOT NULL CONSTRAINT inventory_transfer_lines_in_movement_id_fkey
        REFERENCES app.inventory_movements(id),
    CONSTRAINT inventory_transfer_lines_pkey PRIMARY KEY (transfer_id, line_no),
    CONSTRAINT inventory_transfer_lines_item_key UNIQUE (transfer_id, item_id),
    CONSTRAINT inventory_transfer_lines_out_movement_id_key UNIQUE (out_movement_id),
    CONSTRAINT inventory_transfer_lines_in_movement_id_key UNIQUE (in_movement_id),
    CONSTRAINT inventory_transfer_lines_capture_check CHECK (
        (presentation_id IS NOT NULL AND unit_id IS NULL) OR (presentation_id IS NULL AND unit_id IS NOT NULL)
    ),
    CONSTRAINT inventory_transfer_lines_movements_check CHECK (out_movement_id <> in_movement_id)
);
CREATE INDEX inventory_transfer_lines_item_idx ON app.inventory_transfer_lines (item_id);

CREATE TRIGGER inventory_transfers_insert_only
    BEFORE UPDATE OR DELETE ON app.inventory_transfers
    FOR EACH ROW EXECUTE FUNCTION app.inventory_reject_mutation();
CREATE TRIGGER inventory_transfers_no_truncate
    BEFORE TRUNCATE ON app.inventory_transfers
    FOR EACH STATEMENT EXECUTE FUNCTION app.inventory_reject_mutation();
CREATE TRIGGER inventory_transfer_lines_insert_only
    BEFORE UPDATE OR DELETE ON app.inventory_transfer_lines
    FOR EACH ROW EXECUTE FUNCTION app.inventory_reject_mutation();
CREATE TRIGGER inventory_transfer_lines_no_truncate
    BEFORE TRUNCATE ON app.inventory_transfer_lines
    FOR EACH STATEMENT EXECUTE FUNCTION app.inventory_reject_mutation();

-- 4. Conteos físicos por almacén. Estados: abierto (se capturan cantidades en transacciones cortas;
-- la venta sigue) → enviado a revisión → aprobado → aplicado; un conteo abierto, enviado o aprobado
-- se puede cancelar con motivo. Al aplicarse guarda su momento de referencia (la hora máxima de sus
-- renglones contados). Un conteo aplicado o cancelado ya no cambia.
CREATE TABLE app.inventory_counts (
    id uuid PRIMARY KEY,
    folio bigint GENERATED ALWAYS AS IDENTITY,
    warehouse_id uuid NOT NULL CONSTRAINT inventory_counts_warehouse_id_fkey REFERENCES app.inventory_warehouses(id),
    scope text NOT NULL CONSTRAINT inventory_counts_scope_check CHECK (scope IN ('all', 'category', 'items')),
    category_id uuid CONSTRAINT inventory_counts_category_id_fkey REFERENCES app.inventory_categories(id),
    state text NOT NULL DEFAULT 'open' CONSTRAINT inventory_counts_state_check
        CHECK (state IN ('open', 'submitted', 'approved', 'applied', 'cancelled')),
    notes text NOT NULL DEFAULT '' CONSTRAINT inventory_counts_notes_check CHECK (length(notes) <= 1000),
    line_count integer NOT NULL CONSTRAINT inventory_counts_line_count_check CHECK (line_count BETWEEN 0 AND 2000),
    created_by uuid NOT NULL CONSTRAINT inventory_counts_created_by_fkey REFERENCES app.users(id),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_by uuid NOT NULL CONSTRAINT inventory_counts_updated_by_fkey REFERENCES app.users(id),
    updated_at timestamptz NOT NULL DEFAULT now(),
    submitted_by uuid CONSTRAINT inventory_counts_submitted_by_fkey REFERENCES app.users(id),
    submitted_at timestamptz,
    approved_by uuid CONSTRAINT inventory_counts_approved_by_fkey REFERENCES app.users(id),
    approved_at timestamptz,
    applied_by uuid CONSTRAINT inventory_counts_applied_by_fkey REFERENCES app.users(id),
    applied_at timestamptz,
    reference_at timestamptz,
    cancelled_by uuid CONSTRAINT inventory_counts_cancelled_by_fkey REFERENCES app.users(id),
    cancelled_at timestamptz,
    cancel_reason text,
    revision integer NOT NULL DEFAULT 1 CONSTRAINT inventory_counts_revision_check CHECK (revision > 0),
    operation_id uuid NOT NULL CONSTRAINT inventory_counts_operation_id_fkey
        REFERENCES app.restaurant_operations(id) DEFERRABLE INITIALLY DEFERRED,
    CONSTRAINT inventory_counts_folio_key UNIQUE (folio),
    CONSTRAINT inventory_counts_operation_id_key UNIQUE (operation_id),
    -- La categoría solo (y siempre) en un conteo por categoría.
    CONSTRAINT inventory_counts_scope_category_check CHECK ((scope = 'category') = (category_id IS NOT NULL)),
    -- Envío, aprobación, aplicación y cancelación siempre completos (quién y cuándo) en los estados
    -- que los tienen; un cancelado conserva lo que hubiera alcanzado antes.
    CONSTRAINT inventory_counts_submit_check CHECK (
        (state IN ('submitted', 'approved', 'applied') AND submitted_by IS NOT NULL AND submitted_at IS NOT NULL)
        OR (state = 'open' AND submitted_by IS NULL AND submitted_at IS NULL)
        OR (
            state = 'cancelled'
            AND ((submitted_by IS NULL AND submitted_at IS NULL) OR (submitted_by IS NOT NULL AND submitted_at IS NOT NULL))
        )
    ),
    CONSTRAINT inventory_counts_approve_check CHECK (
        (state IN ('approved', 'applied') AND approved_by IS NOT NULL AND approved_at IS NOT NULL)
        OR (state IN ('open', 'submitted') AND approved_by IS NULL AND approved_at IS NULL)
        OR (
            state = 'cancelled'
            AND ((approved_by IS NULL AND approved_at IS NULL) OR (approved_by IS NOT NULL AND approved_at IS NOT NULL))
        )
    ),
    CONSTRAINT inventory_counts_apply_check CHECK (
        (state = 'applied' AND applied_by IS NOT NULL AND applied_at IS NOT NULL AND reference_at IS NOT NULL)
        OR (state <> 'applied' AND applied_by IS NULL AND applied_at IS NULL AND reference_at IS NULL)
    ),
    CONSTRAINT inventory_counts_cancel_check CHECK (
        (
            state = 'cancelled' AND cancelled_by IS NOT NULL AND cancelled_at IS NOT NULL AND cancel_reason IS NOT NULL
            AND length(cancel_reason) BETWEEN 5 AND 500
        )
        OR (state <> 'cancelled' AND cancelled_by IS NULL AND cancelled_at IS NULL AND cancel_reason IS NULL)
    )
);
CREATE INDEX inventory_counts_warehouse_idx ON app.inventory_counts (warehouse_id, state, created_at DESC);
CREATE INDEX inventory_counts_applied_idx ON app.inventory_counts (warehouse_id, reference_at DESC) WHERE state = 'applied';

-- 5. Renglones de un conteo: un insumo por renglón; la cantidad física contada (en la unidad base y
-- tal como se capturó), la hora y el responsable de la captura, la existencia teórica del almacén a esa
-- hora (Σ del libro con occurred_at ≤ captured_at, calculada al capturar y otra vez al aplicar), la
-- diferencia (contado − teórico) y el movimiento count_in/count_out que la aplicó. Un renglón sin
-- captura no se ajusta.
CREATE TABLE app.inventory_count_lines (
    count_id uuid NOT NULL CONSTRAINT inventory_count_lines_count_id_fkey REFERENCES app.inventory_counts(id),
    line_no integer NOT NULL CONSTRAINT inventory_count_lines_line_no_check CHECK (line_no > 0),
    item_id uuid NOT NULL CONSTRAINT inventory_count_lines_item_id_fkey REFERENCES app.inventory_items(id),
    counted_quantity numeric(20,6) CONSTRAINT inventory_count_lines_counted_quantity_check
        CHECK (counted_quantity IS NULL OR (counted_quantity >= 0 AND counted_quantity <= 1000000000)),
    captured_quantity numeric(20,6) CONSTRAINT inventory_count_lines_captured_quantity_check
        CHECK (captured_quantity IS NULL OR (captured_quantity >= 0 AND captured_quantity <= 1000000000)),
    presentation_id uuid CONSTRAINT inventory_count_lines_presentation_id_fkey REFERENCES app.inventory_presentations(id),
    unit_id uuid CONSTRAINT inventory_count_lines_unit_id_fkey REFERENCES app.inventory_units(id),
    captured_at timestamptz,
    captured_by uuid CONSTRAINT inventory_count_lines_captured_by_fkey REFERENCES app.users(id),
    theoretical_quantity numeric(20,6),
    difference numeric(20,6),
    movement_id uuid CONSTRAINT inventory_count_lines_movement_id_fkey REFERENCES app.inventory_movements(id),
    CONSTRAINT inventory_count_lines_pkey PRIMARY KEY (count_id, line_no),
    CONSTRAINT inventory_count_lines_item_key UNIQUE (count_id, item_id),
    CONSTRAINT inventory_count_lines_movement_id_key UNIQUE (movement_id),
    -- Una captura está completa (cantidad base y capturada, exactamente una unidad de captura, hora y
    -- responsable) o no existe.
    CONSTRAINT inventory_count_lines_capture_check CHECK (
        (
            counted_quantity IS NULL AND captured_quantity IS NULL AND presentation_id IS NULL AND unit_id IS NULL
            AND captured_at IS NULL AND captured_by IS NULL
        )
        OR (
            counted_quantity IS NOT NULL AND captured_quantity IS NOT NULL AND captured_at IS NOT NULL
            AND captured_by IS NOT NULL
            AND ((presentation_id IS NOT NULL AND unit_id IS NULL) OR (presentation_id IS NULL AND unit_id IS NOT NULL))
        )
    ),
    -- La diferencia es exactamente contado − teórico y solo existe con captura.
    CONSTRAINT inventory_count_lines_difference_check CHECK (
        (theoretical_quantity IS NULL AND difference IS NULL)
        OR (
            theoretical_quantity IS NOT NULL AND difference IS NOT NULL AND counted_quantity IS NOT NULL
            AND difference = counted_quantity - theoretical_quantity
        )
    ),
    -- Solo una diferencia distinta de cero genera movimiento.
    CONSTRAINT inventory_count_lines_movement_check CHECK (movement_id IS NULL OR (difference IS NOT NULL AND difference <> 0))
);
CREATE INDEX inventory_count_lines_item_idx ON app.inventory_count_lines (item_id);

-- Un conteo aplicado o cancelado no cambia; ninguno se borra ni cambia de almacén, alcance o autor.
CREATE FUNCTION app.inventory_count_guard() RETURNS trigger
    LANGUAGE plpgsql AS $$
BEGIN
    IF TG_OP = 'UPDATE' AND OLD.state IN ('open', 'submitted', 'approved')
        AND NEW.id = OLD.id
        AND NEW.folio = OLD.folio
        AND NEW.warehouse_id = OLD.warehouse_id
        AND NEW.scope = OLD.scope
        AND NEW.category_id IS NOT DISTINCT FROM OLD.category_id
        AND NEW.created_by = OLD.created_by
        AND NEW.created_at = OLD.created_at
        AND NEW.operation_id = OLD.operation_id THEN
        RETURN NEW;
    END IF;
    RAISE EXCEPTION 'Un conteo aplicado o cancelado no se modifica y ningún conteo se borra' USING ERRCODE = '55000';
END;
$$;
CREATE TRIGGER inventory_counts_guard
    BEFORE UPDATE OR DELETE ON app.inventory_counts
    FOR EACH ROW EXECUTE FUNCTION app.inventory_count_guard();
CREATE TRIGGER inventory_counts_no_truncate
    BEFORE TRUNCATE ON app.inventory_counts
    FOR EACH STATEMENT EXECUTE FUNCTION app.inventory_reject_mutation();

-- Los renglones se agregan, quitan y capturan solo mientras el conteo está abierto (sin movimiento);
-- al aplicar un conteo aprobado se registra una vez su existencia teórica final, su diferencia y su
-- movimiento, sin tocar la captura.
CREATE FUNCTION app.inventory_count_line_guard() RETURNS trigger
    LANGUAGE plpgsql AS $$
DECLARE
    parent_state text;
BEGIN
    IF TG_OP = 'DELETE' THEN
        SELECT state INTO parent_state FROM app.inventory_counts WHERE id = OLD.count_id;
        IF parent_state = 'open' AND OLD.movement_id IS NULL THEN
            RETURN OLD;
        END IF;
    ELSE
        SELECT state INTO parent_state FROM app.inventory_counts WHERE id = NEW.count_id;
        IF parent_state = 'open' AND NEW.movement_id IS NULL
            AND (TG_OP = 'INSERT' OR (NEW.count_id = OLD.count_id AND NEW.item_id = OLD.item_id AND NEW.line_no = OLD.line_no)) THEN
            RETURN NEW;
        END IF;
        IF TG_OP = 'UPDATE' AND parent_state = 'approved' AND OLD.movement_id IS NULL
            AND NEW.count_id = OLD.count_id
            AND NEW.line_no = OLD.line_no
            AND NEW.item_id = OLD.item_id
            AND NEW.counted_quantity IS NOT DISTINCT FROM OLD.counted_quantity
            AND NEW.captured_quantity IS NOT DISTINCT FROM OLD.captured_quantity
            AND NEW.presentation_id IS NOT DISTINCT FROM OLD.presentation_id
            AND NEW.unit_id IS NOT DISTINCT FROM OLD.unit_id
            AND NEW.captured_at IS NOT DISTINCT FROM OLD.captured_at
            AND NEW.captured_by IS NOT DISTINCT FROM OLD.captured_by THEN
            RETURN NEW;
        END IF;
    END IF;
    RAISE EXCEPTION 'Los renglones de un conteo solo se capturan mientras está abierto' USING ERRCODE = '55000';
END;
$$;
CREATE TRIGGER inventory_count_lines_guard
    BEFORE INSERT OR UPDATE OR DELETE ON app.inventory_count_lines
    FOR EACH ROW EXECUTE FUNCTION app.inventory_count_line_guard();
CREATE TRIGGER inventory_count_lines_no_truncate
    BEFORE TRUNCATE ON app.inventory_count_lines
    FOR EACH STATEMENT EXECUTE FUNCTION app.inventory_reject_mutation();

-- 6. Niveles por almacén: existencia mínima y punto de reabastecimiento de un insumo en un almacén, en
-- la unidad base. Sobrescriben los del insumo para ese almacén (los del insumo se comparan con la
-- existencia total). Es configuración (inventory.items.manage, con revisión y auditoría): una fila
-- sin ninguno de los dos valores no existe.
CREATE TABLE app.inventory_stock_levels (
    item_id uuid NOT NULL CONSTRAINT inventory_stock_levels_item_id_fkey REFERENCES app.inventory_items(id),
    warehouse_id uuid NOT NULL CONSTRAINT inventory_stock_levels_warehouse_id_fkey REFERENCES app.inventory_warehouses(id),
    min_stock numeric(20,6) CONSTRAINT inventory_stock_levels_min_stock_check
        CHECK (min_stock IS NULL OR (min_stock >= 0 AND min_stock <= 1000000000)),
    reorder_point numeric(20,6) CONSTRAINT inventory_stock_levels_reorder_point_check
        CHECK (reorder_point IS NULL OR (reorder_point >= 0 AND reorder_point <= 1000000000)),
    revision integer NOT NULL DEFAULT 1 CONSTRAINT inventory_stock_levels_revision_check CHECK (revision > 0),
    updated_by uuid NOT NULL CONSTRAINT inventory_stock_levels_updated_by_fkey REFERENCES app.users(id),
    updated_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT inventory_stock_levels_pkey PRIMARY KEY (item_id, warehouse_id),
    CONSTRAINT inventory_stock_levels_values_check CHECK (min_stock IS NOT NULL OR reorder_point IS NOT NULL)
);
CREATE INDEX inventory_stock_levels_warehouse_idx ON app.inventory_stock_levels (warehouse_id);

-- 7. Permisos de la fase D. Se registran también en Go (inventory.Permissions) con la misma
-- descripción y se conceden solo al administrador; el rol «Inventarios» no recibe permisos por
-- inferencia. Las mermas usan el permiso de la fase B y la autorización de ajustes el de la fase A.
INSERT INTO app.permissions(key, description) VALUES
    ('inventory.adjustments.record', 'Registrar ajustes de inventario'),
    ('inventory.counts.perform', 'Realizar conteos físicos'),
    ('inventory.counts.approve', 'Aprobar y aplicar conteos'),
    ('inventory.transfers.record', 'Transferir mercancía entre almacenes');

INSERT INTO app.role_permissions(role_id, permission_key)
SELECT r.id, p.key FROM app.roles r CROSS JOIN app.permissions p
WHERE r.code = 'administrator' AND p.key IN (
    'inventory.adjustments.record', 'inventory.counts.perform', 'inventory.counts.approve', 'inventory.transfers.record'
)
ON CONFLICT DO NOTHING;
