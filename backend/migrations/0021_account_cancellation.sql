-- Etapa 9D: cancelar cuentas con motivo y autorización (§5), catálogo de motivos de cancelación, umbral de cuenta
-- abierta por mucho tiempo (§3.4) y permiso «Cancelar cuentas». Una sola migración, transaccional; no modifica filas
-- operativas existentes: las cuentas anuladas antes de esta etapa conservan su evento account.void y su motivo libre.

-- 1. Catálogo de motivos de cancelación de cuentas, con la misma forma que los motivos de descuento (0009). Solo
--    «Otro» exige descripción. prepared_is_waste marca los motivos cuyo contenido ya enviado a cocina se preclasifica
--    como «Preparado / merma» en el inventario (decisión 4 del coordinador); el encargado puede corregirlo hasta el
--    corte del día, como en la etapa 8. Renombrar o desactivar un motivo no altera lo registrado (reason_name).
CREATE TABLE app.account_cancel_reasons (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    name text NOT NULL CONSTRAINT account_cancel_reasons_name_check
        CHECK (length(name) BETWEEN 1 AND 60 AND name = btrim(name)),
    requires_description boolean NOT NULL DEFAULT false,
    prepared_is_waste boolean NOT NULL DEFAULT false,
    active boolean NOT NULL DEFAULT true,
    sort_order integer NOT NULL DEFAULT 0 CONSTRAINT account_cancel_reasons_sort_order_check CHECK (sort_order >= 0),
    revision integer NOT NULL DEFAULT 1 CONSTRAINT account_cancel_reasons_revision_check CHECK (revision > 0),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT account_cancel_reasons_name_key UNIQUE (name)
);
INSERT INTO app.account_cancel_reasons(name, requires_description, prepared_is_waste, sort_order) VALUES
    ('Cliente se retiró sin pagar', false, true, 10),
    ('Cliente canceló el pedido', false, false, 20),
    ('Error de captura', false, false, 30),
    ('Otro', true, false, 40);

-- 2. Registro de cuentas canceladas: una fila por cuenta, escrita en la misma transacción que la cancelación. Guarda el
--    motivo con su nombre y su marca de merma congelados, la descripción, quién canceló, quién autorizó (NULL si bastó
--    su propio permiso; nunca la misma persona), la terminal, la operación idempotente y las unidades e importe
--    activos al cancelar. Es de solo inserción. Las columnas opcionales exigen IS NOT NULL explícito en la rama que
--    valida datos, para que ningún CHECK evalúe a NULL.
CREATE TABLE app.account_cancellations (
    account_id uuid PRIMARY KEY CONSTRAINT account_cancellations_account_id_fkey REFERENCES app.restaurant_accounts(id),
    operation_id uuid NOT NULL CONSTRAINT account_cancellations_operation_id_fkey
        REFERENCES app.restaurant_operations(id) DEFERRABLE INITIALLY DEFERRED,
    reason_id uuid NOT NULL CONSTRAINT account_cancellations_reason_id_fkey REFERENCES app.account_cancel_reasons(id),
    reason_name text NOT NULL CONSTRAINT account_cancellations_reason_name_check
        CHECK (length(reason_name) BETWEEN 1 AND 60),
    description text CONSTRAINT account_cancellations_description_check CHECK (
        description IS NULL
        OR (description IS NOT NULL AND length(description) BETWEEN 1 AND 500 AND description = btrim(description))
    ),
    prepared_is_waste boolean NOT NULL,
    cancelled_by uuid NOT NULL CONSTRAINT account_cancellations_cancelled_by_fkey REFERENCES app.users(id),
    authorized_by uuid CONSTRAINT account_cancellations_authorized_by_fkey REFERENCES app.users(id),
    device_id uuid NOT NULL CONSTRAINT account_cancellations_device_id_fkey REFERENCES app.devices(id),
    cancelled_units integer NOT NULL CONSTRAINT account_cancellations_units_check CHECK (cancelled_units >= 0),
    cancelled_amount numeric(18,2) NOT NULL CONSTRAINT account_cancellations_amount_check CHECK (cancelled_amount >= 0),
    created_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT account_cancellations_operation_key UNIQUE (operation_id),
    CONSTRAINT account_cancellations_authorizer_check CHECK (
        authorized_by IS NULL OR (authorized_by IS NOT NULL AND authorized_by <> cancelled_by)
    )
);
-- «Cuentas canceladas» y su total por motivo leen por fecha de la cancelación.
CREATE INDEX account_cancellations_created_idx ON app.account_cancellations (created_at);
CREATE INDEX account_cancellations_reason_idx ON app.account_cancellations (reason_id);

CREATE FUNCTION app.account_cancellations_reject_mutation() RETURNS trigger
    LANGUAGE plpgsql AS $$
BEGIN
    RAISE EXCEPTION 'Los registros de % son de solo inserción', TG_TABLE_NAME USING ERRCODE = '55000';
END;
$$;
CREATE TRIGGER account_cancellations_insert_only
    BEFORE UPDATE OR DELETE ON app.account_cancellations
    FOR EACH ROW EXECUTE FUNCTION app.account_cancellations_reject_mutation();
CREATE TRIGGER account_cancellations_no_truncate
    BEFORE TRUNCATE ON app.account_cancellations
    FOR EACH STATEMENT EXECUTE FUNCTION app.account_cancellations_reject_mutation();

-- 3. Umbral de cuenta abierta por mucho tiempo (§3.4): horas desde la apertura a partir de las cuales la tarjeta y la
--    mesa muestran la advertencia. 12 por omisión; Go valida de 1 a 72 y lo edita en «Funciones del establecimiento».
ALTER TABLE app.restaurant_settings
    ADD COLUMN long_open_hours integer NOT NULL DEFAULT 12
        CONSTRAINT restaurant_settings_long_open_hours_check CHECK (long_open_hours BETWEEN 1 AND 72);

-- 4. Permiso propio para cancelar cuentas, solo al administrador (en una instalación nueva el asistente concede al
--    administrador todos los permisos después de las migraciones). Sin él, la cancelación exige la autorización
--    puntual de un supervisor que lo tenga. La descripción de accounts.manage se alinea con la que muestra Go.
INSERT INTO app.permissions(key, description) VALUES ('accounts.cancel', 'Cancelar cuentas');
INSERT INTO app.role_permissions(role_id, permission_key)
SELECT id, 'accounts.cancel' FROM app.roles WHERE code = 'administrator';
UPDATE app.permissions SET description = 'Editar datos de cuentas y cancelar cuentas vacías con motivo libre'
WHERE key = 'accounts.manage';
