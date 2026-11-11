-- Etapa 9A, fase A (A2): datos adicionales de proveedores y «Compras de mostrador».
-- Transaccional: el migrador la aplica dentro de la misma transacción que la registra en
-- schema_migrations y no contiene sentencias que no admitan transacción (ni CREATE INDEX
-- CONCURRENTLY ni VACUUM), de modo que un fallo la revierte completa. No modifica 0001–0015 ni
-- filas existentes: las columnas nuevas tienen valores por omisión que dejan a los proveedores y
-- recepciones históricos tal como estaban (formales y sin comercio). Las restricciones llevan
-- nombre explícito y los CHECK de columnas opcionales exigen IS NOT NULL en la rama que valida.
--
-- Única siembra: el proveedor especial «Compras de mostrador» (un registro de catálogo, como las
-- unidades del sistema). No crea órdenes, recepciones, movimientos, insumos surtidos ni precios.

-- 1. Datos adicionales de proveedores (§3.3): segundo teléfono, teléfono celular y página web,
-- opcionales (cadena vacía = sin dato). Go valida su formato solo en estos campos nuevos para no
-- invalidar proveedores existentes.
ALTER TABLE app.inventory_suppliers
    ADD COLUMN phone_secondary text NOT NULL DEFAULT ''
        CONSTRAINT inventory_suppliers_phone_secondary_check
            CHECK (length(phone_secondary) <= 40 AND phone_secondary = btrim(phone_secondary)),
    ADD COLUMN mobile_phone text NOT NULL DEFAULT ''
        CONSTRAINT inventory_suppliers_mobile_phone_check
            CHECK (length(mobile_phone) <= 40 AND mobile_phone = btrim(mobile_phone)),
    ADD COLUMN website text NOT NULL DEFAULT ''
        CONSTRAINT inventory_suppliers_website_check
            CHECK (length(website) <= 200 AND website = btrim(website) AND website !~ '\s');

-- 2. Modalidad del proveedor (§3.2): 'formal' (todos los existentes y los que se registren) o
-- 'counter', el proveedor especial «Compras de mostrador» para mercados, centrales de abastos,
-- supermercados y comercios ocasionales. Solo puede existir uno (índice único parcial) y nunca
-- lleva RFC ni razón social. Go no permite crear otro ni cambiar la modalidad de un proveedor, lo
-- rechaza en órdenes de compra y sugerencias, no lo marca como preferido y no registra su último
-- precio: solo se usa en recepciones directas.
ALTER TABLE app.inventory_suppliers
    ADD COLUMN kind text NOT NULL DEFAULT 'formal'
        CONSTRAINT inventory_suppliers_kind_check CHECK (kind IN ('formal', 'counter')),
    ADD CONSTRAINT inventory_suppliers_counter_check CHECK (kind <> 'counter' OR (rfc IS NULL AND legal_name = ''));
CREATE UNIQUE INDEX inventory_suppliers_counter_key ON app.inventory_suppliers (kind) WHERE kind = 'counter';

-- 3. Comercio concreto de una compra de mostrador («Central de abastos, bodega 12»): referencia
-- opcional de la recepción (cadena vacía = sin comercio). Go solo la admite cuando el proveedor de la
-- recepción es el de mostrador. Las recepciones históricas quedan sin comercio y siguen siendo
-- inmutables (el valor por omisión no reescribe filas ni dispara sus guardas).
ALTER TABLE app.inventory_receipts
    ADD COLUMN counter_store text NOT NULL DEFAULT ''
        CONSTRAINT inventory_receipts_counter_store_check
            CHECK (length(counter_store) <= 100 AND counter_store = btrim(counter_store));

-- 4. Siembra del proveedor especial. Si la instalación ya tiene un proveedor formal con ese nombre
-- (por ejemplo, uno creado a mano para compras sueltas, quizá con órdenes y recepciones), no se
-- convierte ni se toca: el especial toma el primer nombre libre de «Compras de mostrador»,
-- «Compras de mostrador (general)», «Compras de mostrador (general 2)»…, y el restaurante puede
-- renombrarlo después. Queda activo, sin usuario que lo creara y sin insumos ni compras.
INSERT INTO app.inventory_suppliers (id, trade_name, kind, notes)
SELECT gen_random_uuid(), candidate.name, 'counter',
    'Compras ocasionales en mercados, centrales de abastos, supermercados y comercios sin proveedor registrado.'
FROM (
    SELECT attempt,
        CASE
            WHEN attempt = 0 THEN 'Compras de mostrador'
            WHEN attempt = 1 THEN 'Compras de mostrador (general)'
            ELSE 'Compras de mostrador (general ' || attempt || ')'
        END AS name
    FROM generate_series(0, 50) AS attempt
) AS candidate
WHERE NOT EXISTS (SELECT FROM app.inventory_suppliers s WHERE lower(s.trade_name) = lower(candidate.name))
    AND NOT EXISTS (SELECT FROM app.inventory_suppliers s WHERE s.kind = 'counter')
ORDER BY candidate.attempt
LIMIT 1;
