-- Etapa 6.1 (fase C): funciones activables del establecimiento, IVA por producto y pago en dólares.
-- Solo agrega entidades, columnas y restricciones: no modifica importes, pagos ni movimientos
-- históricos. Las filas anteriores conservan NULL en las columnas nuevas, que significa
-- «sin dato propio» (el producto hereda la tasa general; el pago y el movimiento fueron solo en pesos).

-- Funciones activables. La lista de funciones conocidas vive en Go (internal/features); una fila
-- ausente equivale a una función desactivada. Desactivar nunca borra ni altera registros.
CREATE TABLE app.establishment_features (
    code text PRIMARY KEY CONSTRAINT establishment_features_code_check CHECK (code ~ '^[a-z][a-z0-9_]{1,39}$'),
    enabled boolean NOT NULL DEFAULT false,
    revision integer NOT NULL DEFAULT 1 CONSTRAINT establishment_features_revision_check CHECK (revision > 0),
    updated_by uuid REFERENCES app.users(id),
    updated_at timestamptz NOT NULL DEFAULT now()
);
-- El pago en dólares en efectivo inicia desactivado.
INSERT INTO app.establishment_features(code, enabled) VALUES ('usd_cash', false);

-- IVA por producto (incluidos los combos): NULL hereda la tasa general vigente del establecimiento.
ALTER TABLE app.catalog_products
    ADD COLUMN tax_rate integer
        CONSTRAINT catalog_products_tax_rate_check CHECK (tax_rate IS NULL OR tax_rate IN (0, 8, 16));

-- Rango aceptable del tipo de cambio (MXN por USD) y porcentaje de diferencia contra el turno
-- anterior que exige una confirmación adicional. Evita capturas como 1.95 en lugar de 19.50.
ALTER TABLE app.payment_settings
    ADD COLUMN usd_rate_min numeric(8,2) NOT NULL DEFAULT 10,
    ADD COLUMN usd_rate_max numeric(8,2) NOT NULL DEFAULT 40,
    ADD COLUMN usd_rate_alert_percent numeric(5,2) NOT NULL DEFAULT 10,
    ADD CONSTRAINT payment_settings_usd_rate_range_check CHECK (usd_rate_min > 0 AND usd_rate_max > usd_rate_min),
    ADD CONSTRAINT payment_settings_usd_rate_alert_check CHECK (usd_rate_alert_percent BETWEEN 0 AND 100);

-- Tipo de cambio fijo por turno: se captura al abrir o una sola vez después; los tres datos van juntos.
ALTER TABLE app.cash_shifts
    ADD COLUMN usd_rate numeric(8,2),
    ADD COLUMN usd_rate_set_by uuid REFERENCES app.users(id),
    ADD COLUMN usd_rate_set_at timestamptz,
    ADD CONSTRAINT cash_shifts_usd_rate_positive_check CHECK (usd_rate IS NULL OR usd_rate > 0),
    ADD CONSTRAINT cash_shifts_usd_rate_fields_check CHECK (
        (usd_rate IS NULL) = (usd_rate_set_by IS NULL) AND (usd_rate IS NULL) = (usd_rate_set_at IS NULL)
    );

-- Pago en efectivo con dólares. received sigue siendo el total recibido expresado en pesos y
-- change_given el cambio en pesos; las columnas nuevas guardan lo físico: pesos y dólares
-- recibidos, tipo de cambio aplicado y moneda del cambio. Cada conversión se redondea a centavos,
-- mitad alejándose de cero (round de numeric en PostgreSQL y Decimal.Round/DivRound en Go).
ALTER TABLE app.payments
    ADD COLUMN received_mxn numeric(18,2),
    ADD COLUMN received_usd numeric(18,2),
    ADD COLUMN usd_rate numeric(8,2),
    ADD COLUMN change_currency text,
    ADD COLUMN change_usd numeric(18,2),
    ADD CONSTRAINT payments_change_currency_check CHECK (change_currency IS NULL OR change_currency IN ('MXN', 'USD')),
    ADD CONSTRAINT payments_usd_fields_check CHECK (
        (received_mxn IS NULL AND received_usd IS NULL AND usd_rate IS NULL AND change_currency IS NULL AND change_usd IS NULL)
        OR (
            -- Los IS NOT NULL explícitos evitan que una comparación con NULL deje el CHECK sin
            -- decidir (PostgreSQL acepta un CHECK que evalúa a NULL) y admita datos a medias.
            is_cash
            AND received IS NOT NULL
            AND received_mxn IS NOT NULL
            AND received_usd IS NOT NULL
            AND usd_rate IS NOT NULL
            AND received_mxn >= 0
            AND received_usd >= 0
            AND usd_rate > 0
            AND change_currency IS NOT NULL
            AND (received_usd > 0 OR change_currency = 'USD')
            AND received = received_mxn + round(received_usd * usd_rate, 2)
            AND (
                (change_currency = 'MXN' AND change_usd IS NULL)
                OR (change_currency = 'USD' AND change_usd IS NOT NULL AND change_usd = round(change_given / usd_rate, 2))
            )
        )
    );

-- Movimientos físicos por moneda para el corte de la etapa 7. Las filas nuevas de efectivo llenan
-- entradas y salidas de pesos y de dólares; NULL (filas anteriores o métodos no efectivo) significa
-- que el movimiento fue solo en pesos y su efecto físico es net_amount.
ALTER TABLE app.cash_movements
    ADD COLUMN mxn_in numeric(18,2),
    ADD COLUMN mxn_out numeric(18,2),
    ADD COLUMN usd_in numeric(18,2),
    ADD COLUMN usd_out numeric(18,2),
    ADD COLUMN usd_rate numeric(8,2),
    ADD CONSTRAINT cash_movements_currency_check CHECK (
        (mxn_in IS NULL AND mxn_out IS NULL AND usd_in IS NULL AND usd_out IS NULL AND usd_rate IS NULL)
        OR (
            -- Igual que en payments: sin NULL parciales en las cuatro cantidades físicas.
            is_cash
            AND mxn_in IS NOT NULL
            AND mxn_out IS NOT NULL
            AND usd_in IS NOT NULL
            AND usd_out IS NOT NULL
            AND mxn_in >= 0
            AND mxn_out >= 0
            AND usd_in >= 0
            AND usd_out >= 0
            AND ((usd_rate IS NOT NULL AND usd_rate > 0) OR (usd_rate IS NULL AND usd_in = 0 AND usd_out = 0))
        )
    );

INSERT INTO app.permissions(key, description) VALUES
    ('settings.features.manage', 'Activar o desactivar funciones del establecimiento'),
    ('payments.usd_rate.manage', 'Capturar el tipo de cambio del dólar del turno');

-- Solo los permisos nuevos al administrador: no se restablecen roles ni credenciales existentes.
INSERT INTO app.role_permissions(role_id, permission_key)
SELECT r.id, p.key FROM app.roles r CROSS JOIN app.permissions p
WHERE r.code = 'administrator' AND p.key IN ('settings.features.manage', 'payments.usd_rate.manage')
ON CONFLICT DO NOTHING;
