-- Catálogo comercial: referencias estables, precios exactos y edición optimista.
CREATE TABLE app.catalog_images (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    media_type text NOT NULL CHECK (media_type IN ('image/png', 'image/jpeg')),
    content bytea NOT NULL CHECK (octet_length(content) BETWEEN 1 AND 1048576),
    checksum text NOT NULL UNIQUE,
    width integer NOT NULL CHECK (width BETWEEN 1 AND 2048),
    height integer NOT NULL CHECK (height BETWEEN 1 AND 2048),
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE app.catalog_categories (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    name text NOT NULL UNIQUE CHECK (length(name) BETWEEN 1 AND 100),
    description text NOT NULL DEFAULT '',
    active boolean NOT NULL DEFAULT true,
    sort_order integer NOT NULL DEFAULT 0,
    color text NOT NULL DEFAULT '',
    image_id uuid REFERENCES app.catalog_images(id),
    availability jsonb NOT NULL DEFAULT '{}',
    revision integer NOT NULL DEFAULT 1 CHECK (revision > 0),
    updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE app.catalog_ingredients (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    name text NOT NULL UNIQUE CHECK (length(name) BETWEEN 1 AND 100),
    description text NOT NULL DEFAULT '',
    active boolean NOT NULL DEFAULT true,
    unit text NOT NULL DEFAULT 'porción',
    updated_at timestamptz NOT NULL DEFAULT now(),
    revision integer NOT NULL DEFAULT 1 CHECK (revision > 0)
);

CREATE TABLE app.catalog_portions (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    name text NOT NULL UNIQUE CHECK (length(name) BETWEEN 1 AND 100),
    description text NOT NULL DEFAULT '',
    active boolean NOT NULL DEFAULT true,
    factor numeric(12,3) NOT NULL CHECK (factor > 0),
    updated_at timestamptz NOT NULL DEFAULT now(),
    revision integer NOT NULL DEFAULT 1 CHECK (revision > 0)
);

CREATE TABLE app.catalog_products (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    category_id uuid NOT NULL REFERENCES app.catalog_categories(id),
    name text NOT NULL CHECK (length(name) BETWEEN 1 AND 100),
    description text NOT NULL DEFAULT '',
    code text NOT NULL UNIQUE CHECK (length(code) BETWEEN 1 AND 60),
    barcode text UNIQUE,
    kind text NOT NULL CHECK (kind IN ('product', 'combo')),
    base_price numeric(14,2) NOT NULL CHECK (base_price >= 0),
    image_id uuid REFERENCES app.catalog_images(id),
    active boolean NOT NULL DEFAULT true,
    archived boolean NOT NULL DEFAULT false,
    temporary_unavailable boolean NOT NULL DEFAULT false,
    sort_order integer NOT NULL DEFAULT 0,
    availability jsonb NOT NULL DEFAULT '{}',
    revision integer NOT NULL DEFAULT 1 CHECK (revision > 0),
    updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX catalog_products_category_idx ON app.catalog_products(category_id, sort_order, name);
CREATE INDEX catalog_products_active_idx ON app.catalog_products(active, archived);

CREATE TABLE app.catalog_variants (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    product_id uuid NOT NULL REFERENCES app.catalog_products(id),
    name text NOT NULL CHECK (length(name) BETWEEN 1 AND 100),
    code text NOT NULL UNIQUE,
    barcode text UNIQUE,
    price numeric(14,2) NOT NULL CHECK (price >= 0),
    active boolean NOT NULL DEFAULT true,
    availability jsonb NOT NULL DEFAULT '{}',
    UNIQUE (id, product_id)
);
CREATE INDEX catalog_variants_product_idx ON app.catalog_variants(product_id);

CREATE TABLE app.catalog_product_ingredients (
    product_id uuid NOT NULL REFERENCES app.catalog_products(id),
    ingredient_id uuid NOT NULL REFERENCES app.catalog_ingredients(id),
    included_quantity numeric(12,3) NOT NULL CHECK (included_quantity >= 0),
    removable boolean NOT NULL,
    addable boolean NOT NULL,
    max_quantity numeric(12,3) NOT NULL CHECK (max_quantity >= included_quantity),
    extra_price numeric(14,2) NOT NULL CHECK (extra_price >= 0),
    recipe_quantity numeric(12,3) NOT NULL DEFAULT 0 CHECK (recipe_quantity >= 0),
    PRIMARY KEY (product_id, ingredient_id)
);

CREATE TABLE app.catalog_complements (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    name text NOT NULL UNIQUE CHECK (length(name) BETWEEN 1 AND 100),
    description text NOT NULL DEFAULT '',
    active boolean NOT NULL DEFAULT true,
    product_id uuid REFERENCES app.catalog_products(id),
    ingredient_id uuid REFERENCES app.catalog_ingredients(id),
    updated_at timestamptz NOT NULL DEFAULT now(),
    revision integer NOT NULL DEFAULT 1 CHECK (revision > 0),
    CHECK (product_id IS NULL OR ingredient_id IS NULL)
);

CREATE TABLE app.catalog_option_groups (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    name text NOT NULL UNIQUE CHECK (length(name) BETWEEN 1 AND 100),
    description text NOT NULL DEFAULT '',
    active boolean NOT NULL DEFAULT true,
    pricing_mode text NOT NULL CHECK (pricing_mode IN ('value', 'quantity')),
    min_quantity numeric(12,3) NOT NULL DEFAULT 0 CHECK (min_quantity >= 0),
    max_quantity numeric(12,3) CHECK (max_quantity >= min_quantity),
    updated_at timestamptz NOT NULL DEFAULT now(),
    revision integer NOT NULL DEFAULT 1 CHECK (revision > 0)
);

CREATE TABLE app.catalog_group_options (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    group_id uuid NOT NULL REFERENCES app.catalog_option_groups(id),
    complement_id uuid NOT NULL REFERENCES app.catalog_complements(id),
    portion_id uuid REFERENCES app.catalog_portions(id),
    reference_value numeric(14,2) NOT NULL CHECK (reference_value >= 0),
    extra_price numeric(14,2) NOT NULL DEFAULT 0 CHECK (extra_price >= 0),
    max_quantity numeric(12,3) CHECK (max_quantity >= 0),
    active boolean NOT NULL DEFAULT true,
    UNIQUE (id, group_id),
    UNIQUE NULLS NOT DISTINCT (group_id, complement_id, portion_id)
);
CREATE INDEX catalog_group_options_group_idx ON app.catalog_group_options(group_id);

CREATE TABLE app.catalog_option_tiers (
    option_id uuid NOT NULL REFERENCES app.catalog_group_options(id),
    from_quantity integer NOT NULL CHECK (from_quantity >= 1),
    unit_price numeric(14,2) NOT NULL CHECK (unit_price >= 0),
    PRIMARY KEY (option_id, from_quantity)
);

CREATE TABLE app.catalog_product_groups (
    product_id uuid NOT NULL REFERENCES app.catalog_products(id),
    group_id uuid NOT NULL REFERENCES app.catalog_option_groups(id),
    min_quantity numeric(12,3) CHECK (min_quantity >= 0),
    max_quantity numeric(12,3) CHECK (max_quantity >= coalesce(min_quantity, 0)),
    PRIMARY KEY (product_id, group_id)
);

CREATE TABLE app.catalog_group_defaults (
    product_id uuid NOT NULL,
    group_id uuid NOT NULL,
    option_id uuid NOT NULL,
    quantity numeric(12,3) NOT NULL CHECK (quantity >= 0),
    PRIMARY KEY (product_id, group_id, option_id),
    FOREIGN KEY (product_id, group_id) REFERENCES app.catalog_product_groups(product_id, group_id),
    FOREIGN KEY (option_id, group_id) REFERENCES app.catalog_group_options(id, group_id)
);

CREATE TABLE app.catalog_combo_slots (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    combo_id uuid NOT NULL REFERENCES app.catalog_products(id),
    name text NOT NULL,
    min_quantity integer NOT NULL DEFAULT 0 CHECK (min_quantity >= 0),
    max_quantity integer NOT NULL CHECK (max_quantity BETWEEN 1 AND 100 AND max_quantity >= min_quantity),
    included_quantity integer NOT NULL DEFAULT 0 CHECK (included_quantity >= 0 AND included_quantity <= max_quantity),
    default_quantity integer NOT NULL CHECK (default_quantity >= min_quantity AND default_quantity <= max_quantity),
    default_product_id uuid NOT NULL REFERENCES app.catalog_products(id)
);
CREATE INDEX catalog_combo_slots_combo_idx ON app.catalog_combo_slots(combo_id);

CREATE TABLE app.catalog_combo_options (
    slot_id uuid NOT NULL REFERENCES app.catalog_combo_slots(id),
    product_id uuid NOT NULL REFERENCES app.catalog_products(id),
    substitution_price numeric(14,2) NOT NULL DEFAULT 0 CHECK (substitution_price >= 0),
    PRIMARY KEY (slot_id, product_id)
);
CREATE INDEX catalog_combo_options_product_idx ON app.catalog_combo_options(product_id);

CREATE TABLE app.catalog_combo_tiers (
    slot_id uuid NOT NULL,
    product_id uuid NOT NULL,
    from_quantity integer NOT NULL CHECK (from_quantity >= 1),
    unit_price numeric(14,2) NOT NULL CHECK (unit_price >= 0),
    PRIMARY KEY (slot_id, product_id, from_quantity),
    FOREIGN KEY (slot_id, product_id) REFERENCES app.catalog_combo_options(slot_id, product_id)
);

CREATE TABLE app.catalog_price_lists (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    name text NOT NULL UNIQUE,
    description text NOT NULL DEFAULT '',
    active boolean NOT NULL DEFAULT true,
    is_default boolean NOT NULL DEFAULT false,
    fallback text NOT NULL CHECK (fallback IN ('base', 'reject')),
    allow_promotions boolean NOT NULL DEFAULT false,
    updated_at timestamptz NOT NULL DEFAULT now(),
    revision integer NOT NULL DEFAULT 1 CHECK (revision > 0),
    CHECK (NOT is_default OR active)
);
CREATE UNIQUE INDEX catalog_default_price_list_idx ON app.catalog_price_lists(is_default) WHERE is_default;

CREATE TABLE app.catalog_price_entries (
    list_id uuid NOT NULL REFERENCES app.catalog_price_lists(id),
    product_id uuid NOT NULL REFERENCES app.catalog_products(id),
    variant_id uuid,
    price numeric(14,2) NOT NULL CHECK (price >= 0),
    UNIQUE NULLS NOT DISTINCT (list_id, product_id, variant_id),
    FOREIGN KEY (variant_id, product_id) REFERENCES app.catalog_variants(id, product_id)
);

CREATE TABLE app.catalog_promotions (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    name text NOT NULL UNIQUE,
    description text NOT NULL DEFAULT '',
    active boolean NOT NULL DEFAULT true,
    kind text NOT NULL CHECK (kind IN ('two_for_one', 'percent', 'quantity', 'special_price')),
    percent numeric(5,2) NOT NULL DEFAULT 0 CHECK (percent BETWEEN 0 AND 100),
    special_price numeric(14,2) NOT NULL DEFAULT 0 CHECK (special_price >= 0),
    buy_quantity integer NOT NULL DEFAULT 2 CHECK (buy_quantity BETWEEN 1 AND 1000),
    pay_quantity integer NOT NULL DEFAULT 1 CHECK (pay_quantity >= 0 AND pay_quantity < buy_quantity),
    minimum_quantity integer NOT NULL DEFAULT 1 CHECK (minimum_quantity BETWEEN 1 AND 1000),
    priority integer NOT NULL DEFAULT 0,
    stackable boolean NOT NULL DEFAULT false,
    include_extras boolean NOT NULL DEFAULT false,
    availability jsonb NOT NULL DEFAULT '{}',
    updated_at timestamptz NOT NULL DEFAULT now(),
    revision integer NOT NULL DEFAULT 1 CHECK (revision > 0)
);

CREATE TABLE app.catalog_promotion_products (
    promotion_id uuid NOT NULL REFERENCES app.catalog_promotions(id),
    product_id uuid NOT NULL REFERENCES app.catalog_products(id),
    PRIMARY KEY (promotion_id, product_id)
);
CREATE TABLE app.catalog_promotion_categories (
    promotion_id uuid NOT NULL REFERENCES app.catalog_promotions(id),
    category_id uuid NOT NULL REFERENCES app.catalog_categories(id),
    PRIMARY KEY (promotion_id, category_id)
);
CREATE TABLE app.catalog_promotion_lists (
    promotion_id uuid NOT NULL REFERENCES app.catalog_promotions(id),
    list_id uuid NOT NULL REFERENCES app.catalog_price_lists(id),
    PRIMARY KEY (promotion_id, list_id)
);

INSERT INTO app.catalog_price_lists(name, is_default, fallback, allow_promotions)
VALUES ('Restaurante', true, 'base', true);

INSERT INTO app.permissions(key, description) VALUES
    ('catalog.read', 'Consultar catálogo y configuraciones'),
    ('catalog.prices.read', 'Consultar precios y previsualizar cálculos'),
    ('catalog.prices.manage', 'Modificar precios y valores comerciales'),
    ('catalog.products.manage', 'Crear y editar productos'),
    ('catalog.products.state', 'Activar, desactivar y archivar productos'),
    ('catalog.categories.manage', 'Administrar categorías'),
    ('catalog.ingredients.manage', 'Administrar ingredientes'),
    ('catalog.complements.manage', 'Administrar complementos y porciones'),
    ('catalog.groups.manage', 'Administrar grupos de personalización'),
    ('catalog.combos.manage', 'Administrar combos y componentes'),
    ('catalog.price_lists.manage', 'Administrar listas de precios'),
    ('catalog.promotions.manage', 'Administrar promociones'),
    ('catalog.availability.manage', 'Modificar disponibilidad y horarios'),
    ('catalog.availability.override', 'Autorizar excepciones de disponibilidad en previsualización'),
    ('catalog.images.manage', 'Administrar imágenes del catálogo');

-- Se añaden únicamente los permisos nuevos al administrador; no se restablece ningún rol existente.
INSERT INTO app.role_permissions(role_id, permission_key)
SELECT r.id, p.key FROM app.roles r CROSS JOIN app.permissions p
WHERE r.code = 'administrator' AND p.key LIKE 'catalog.%';
