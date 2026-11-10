-- Etapa 7.1: ajustes de operación, mesas, caja y motivos. Es la única migración de la etapa: la
-- fase C (caja) aporta las dos primeras secciones y la fase D agregará las suyas al final.
-- Transaccional: el migrador la aplica dentro de la misma transacción que la registra en
-- schema_migrations y no contiene sentencias que no admitan transacción (ni CREATE INDEX
-- CONCURRENTLY ni VACUUM), de modo que un fallo la revierte completa. No modifica 0001–0008 ni
-- filas existentes y no siembra datos ficticios.

-- 1. Cajón de dinero (fase C): «Probar apertura» desde la configuración registra su comando con el
-- motivo test. El CHECK de motivos tiene nombre explícito desde la 0008; se reemplaza con el mismo
-- nombre y conserva los motivos anteriores, de modo que las filas existentes siguen siendo válidas.
ALTER TABLE app.cash_drawer_events DROP CONSTRAINT cash_drawer_events_reason_check;
ALTER TABLE app.cash_drawer_events
    ADD CONSTRAINT cash_drawer_events_reason_check
        CHECK (reason IN ('payment', 'refund', 'operation', 'manual', 'test'));

-- 2. Rechazos por efectivo insuficiente (fase C): cada retiro o gasto rechazado queda en
-- audit_events con la acción cash.outflow_rejected, la entidad cash_shift y success=false. La
-- consulta por turno (cash.expected.read) usa este índice parcial, que solo contiene esas filas.
CREATE INDEX audit_events_cash_outflow_rejected_idx
    ON app.audit_events (entity_id, occurred_at)
    WHERE action = 'cash.outflow_rejected';

-- 3. Motivos de descuento (fase D). Única siembra de la migración: el catálogo inicial que decidió
-- el propietario (Atención, Promoción, Demora, Cortesía de la casa y Otro), no datos ficticios.
-- Se administra en Cobros con payments.settings.manage (permiso existente), con orden visual por
-- posiciones (internal/ordering) y revisión optimista. Solo «Otro» exige descripción. Desactivar
-- un motivo no altera los descuentos registrados: no se borra ninguno y account_adjustments
-- conserva su referencia.
CREATE TABLE app.discount_reasons (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    name text NOT NULL CONSTRAINT discount_reasons_name_check
        CHECK (length(name) BETWEEN 1 AND 60 AND name = btrim(name)),
    requires_description boolean NOT NULL DEFAULT false,
    active boolean NOT NULL DEFAULT true,
    sort_order integer NOT NULL DEFAULT 0 CONSTRAINT discount_reasons_sort_order_check CHECK (sort_order >= 0),
    revision integer NOT NULL DEFAULT 1 CONSTRAINT discount_reasons_revision_check CHECK (revision > 0),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT discount_reasons_name_key UNIQUE (name)
);
INSERT INTO app.discount_reasons(name, requires_description, sort_order) VALUES
    ('Atención', false, 10),
    ('Promoción', false, 20),
    ('Demora', false, 30),
    ('Cortesía de la casa', false, 40),
    ('Otro', true, 50);

-- 4. Motivo de cada descuento o cortesía (fase D), para el historial de la cuenta, el ticket, la
-- precuenta y los reportes de la etapa 9. Las filas existentes quedan en NULL (descuentos
-- anteriores a esta etapa o registrados por la API sin motivo del catálogo). Las columnas son
-- opcionales, por lo que cada CHECK exige IS NULL en la rama que las omite y nunca evalúa a NULL:
-- un cargo nunca lleva motivo de descuento. reason_name conserva el nombre del motivo tal como era
-- al registrar el descuento: renombrar o desactivar un motivo del catálogo no altera lo registrado.
ALTER TABLE app.account_adjustments
    ADD COLUMN reason_id uuid CONSTRAINT account_adjustments_reason_id_fkey REFERENCES app.discount_reasons(id),
    ADD COLUMN reason_name text;
ALTER TABLE app.account_adjustments
    ADD CONSTRAINT account_adjustments_reason_kind_check CHECK (reason_id IS NULL OR kind = 'discount');
ALTER TABLE app.account_adjustments
    ADD CONSTRAINT account_adjustments_reason_name_check CHECK (
        (reason_id IS NULL AND reason_name IS NULL)
        OR (reason_id IS NOT NULL AND reason_name IS NOT NULL AND length(reason_name) BETWEEN 1 AND 60)
    );
CREATE INDEX account_adjustments_reason_idx ON app.account_adjustments (reason_id) WHERE reason_id IS NOT NULL;

-- 5. Indicaciones rápidas por categoría (fase D). Se administran en Catálogo con
-- catalog.categories.manage, con orden visual por posiciones dentro de su categoría y revisión
-- optimista. La captura las ofrece como botones y las guarda como texto en customization.notes,
-- que une las indicaciones con comas: por eso el texto no admite comas ni espacios al inicio o al
-- final (se compararía distinto al separarlo). El índice único evita dos indicaciones que solo
-- difieren en mayúsculas dentro de la misma categoría.
CREATE TABLE app.catalog_category_quick_notes (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    category_id uuid NOT NULL
        CONSTRAINT catalog_category_quick_notes_category_id_fkey REFERENCES app.catalog_categories(id),
    text text NOT NULL CONSTRAINT catalog_category_quick_notes_text_check
        CHECK (length(text) BETWEEN 1 AND 80 AND text = btrim(text) AND strpos(text, ',') = 0),
    active boolean NOT NULL DEFAULT true,
    sort_order integer NOT NULL DEFAULT 0
        CONSTRAINT catalog_category_quick_notes_sort_order_check CHECK (sort_order >= 0),
    revision integer NOT NULL DEFAULT 1 CONSTRAINT catalog_category_quick_notes_revision_check CHECK (revision > 0),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX catalog_category_quick_notes_text_key
    ON app.catalog_category_quick_notes (category_id, lower(text));
