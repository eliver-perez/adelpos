-- Etapa 9B, fase A: plantillas de documentos por bloques ordenados con versiones inmutables, versión
-- de plantilla de cada documento emitido, IR de los trabajos con atributos, nuevos tipos de ticket de
-- la cola (muestras e impresión con la plantilla vigente), opción ESC/POS de las impresoras CUPS y
-- permisos del diseñador.
-- Transaccional: el migrador la aplica dentro de la misma transacción que la registra en
-- schema_migrations y no contiene sentencias que no admitan transacción (ni CREATE INDEX
-- CONCURRENTLY ni VACUUM), de modo que un fallo la revierte completa. No modifica 0001–0017 ni
-- filas existentes: los documentos anteriores conservan template_version_id NULL (se reimprimen con
-- la versión 1 del sistema, idéntica al formato anterior) y sus trabajos layout NULL. La semilla
-- son las diez plantillas del sistema (datos de configuración, nunca ficticios). Las restricciones
-- llevan nombre explícito y los CHECK con columnas opcionales exigen IS NOT NULL en la rama que
-- valida, porque un CHECK que evalúa a NULL se acepta.

-- 1. Plantillas. Una plantilla pertenece a un tipo de documento de la lista cerrada; las del sistema
-- (una por tipo, con identificadores fijos) son de solo lectura y no tienen autor. Exactamente una
-- plantilla activa por tipo (índice único parcial y siembra de las del sistema activas). revision
-- es la revisión optimista que comprueba el guardado entre administradores.
CREATE TABLE app.document_templates (
    id uuid CONSTRAINT document_templates_pkey PRIMARY KEY,
    kind text NOT NULL CONSTRAINT document_templates_kind_check CHECK (
        kind IN (
            'receipt', 'precheck', 'kitchen_order', 'kitchen_modification', 'kitchen_cancellation',
            'cash_income', 'cash_withdrawal', 'cash_expense', 'cash_partial_cut', 'cash_final_cut'
        )
    ),
    name text NOT NULL CONSTRAINT document_templates_name_check CHECK (
        length(name) BETWEEN 1 AND 80 AND name = btrim(name)
    ),
    system boolean NOT NULL DEFAULT false,
    active boolean NOT NULL DEFAULT false,
    current_version_id uuid NOT NULL,
    revision integer NOT NULL DEFAULT 1 CONSTRAINT document_templates_revision_check CHECK (revision > 0),
    created_by uuid CONSTRAINT document_templates_created_by_fkey REFERENCES app.users(id),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_by uuid CONSTRAINT document_templates_updated_by_fkey REFERENCES app.users(id),
    updated_at timestamptz NOT NULL DEFAULT now(),
    -- Una plantilla del sistema no tiene autor; una creada por el restaurante, siempre.
    CONSTRAINT document_templates_author_check CHECK (system OR created_by IS NOT NULL)
);
-- El nombre es único por tipo sin distinguir mayúsculas.
CREATE UNIQUE INDEX document_templates_name_key ON app.document_templates (kind, lower(name));
CREATE UNIQUE INDEX document_templates_active_key ON app.document_templates (kind) WHERE active;
CREATE UNIQUE INDEX document_templates_system_key ON app.document_templates (kind) WHERE system;

-- 2. Versiones. Cada guardado crea una versión nueva con el layout canónico que validó Go (bloques
-- de la lista cerrada; nunca HTML, CSS ni comandos ESC/POS). Son inmutables: un disparador impide
-- UPDATE, DELETE y TRUNCATE. Las del sistema no tienen autor.
CREATE TABLE app.document_template_versions (
    id uuid CONSTRAINT document_template_versions_pkey PRIMARY KEY,
    template_id uuid NOT NULL CONSTRAINT document_template_versions_template_fkey REFERENCES app.document_templates(id),
    number integer NOT NULL CONSTRAINT document_template_versions_number_check CHECK (number > 0),
    layout jsonb NOT NULL CONSTRAINT document_template_versions_layout_check CHECK (
        jsonb_typeof(layout) = 'object' AND octet_length(layout::text) <= 65536
    ),
    note text NOT NULL DEFAULT '' CONSTRAINT document_template_versions_note_check CHECK (length(note) <= 200),
    created_by uuid CONSTRAINT document_template_versions_created_by_fkey REFERENCES app.users(id),
    created_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT document_template_versions_number_key UNIQUE (template_id, number),
    CONSTRAINT document_template_versions_template_version_key UNIQUE (template_id, id)
);

-- La versión vigente pertenece a la misma plantilla (llave compuesta). Es diferida para crear la
-- plantilla y su primera versión en la misma transacción.
ALTER TABLE app.document_templates
    ADD CONSTRAINT document_templates_current_version_fkey FOREIGN KEY (id, current_version_id)
        REFERENCES app.document_template_versions (template_id, id) DEFERRABLE INITIALLY DEFERRED;

CREATE FUNCTION app.document_template_versions_reject_mutation() RETURNS trigger
    LANGUAGE plpgsql AS $$
BEGIN
    RAISE EXCEPTION 'Las versiones de plantilla son inmutables (%)', TG_TABLE_NAME USING ERRCODE = '55000';
END;
$$;
CREATE TRIGGER document_template_versions_insert_only
    BEFORE UPDATE OR DELETE ON app.document_template_versions
    FOR EACH ROW EXECUTE FUNCTION app.document_template_versions_reject_mutation();
CREATE TRIGGER document_template_versions_no_truncate
    BEFORE TRUNCATE ON app.document_template_versions
    FOR EACH STATEMENT EXECUTE FUNCTION app.document_template_versions_reject_mutation();

-- El tipo de una plantilla y su condición de plantilla del sistema no cambian; una plantilla del
-- sistema conserva su nombre y su versión (solo se activa o desactiva), y ninguna plantilla se borra.
CREATE FUNCTION app.document_templates_guard() RETURNS trigger
    LANGUAGE plpgsql AS $$
BEGIN
    IF TG_OP = 'DELETE' THEN
        RAISE EXCEPTION 'Las plantillas de documentos no se eliminan' USING ERRCODE = '55000';
    END IF;
    IF NEW.kind <> OLD.kind OR NEW.system <> OLD.system THEN
        RAISE EXCEPTION 'El tipo de una plantilla no cambia' USING ERRCODE = '55000';
    END IF;
    IF OLD.system AND (NEW.name <> OLD.name OR NEW.current_version_id <> OLD.current_version_id) THEN
        RAISE EXCEPTION 'Las plantillas del sistema son de solo lectura' USING ERRCODE = '55000';
    END IF;
    RETURN NEW;
END;
$$;
CREATE TRIGGER document_templates_guard
    BEFORE UPDATE OR DELETE ON app.document_templates
    FOR EACH ROW EXECUTE FUNCTION app.document_templates_guard();

-- 3. Semilla: las diez plantillas del sistema activas, con identificadores fijos (plantilla y
-- versión 1). El layout es exactamente documents.Default(kind) de Go, que reproduce byte a byte los
-- documentos anteriores a la 9B; una prueba de integración compara lo sembrado con Go.
INSERT INTO app.document_templates (id, kind, name, system, active, current_version_id) VALUES
    ('9b0d0c00-0000-4000-8000-000000000001', 'receipt', 'Predeterminada', true, true, '9b0d0c01-0000-4000-8000-000000000001'),
    ('9b0d0c00-0000-4000-8000-000000000002', 'precheck', 'Predeterminada', true, true, '9b0d0c01-0000-4000-8000-000000000002'),
    ('9b0d0c00-0000-4000-8000-000000000003', 'kitchen_order', 'Predeterminada', true, true, '9b0d0c01-0000-4000-8000-000000000003'),
    ('9b0d0c00-0000-4000-8000-000000000004', 'kitchen_modification', 'Predeterminada', true, true, '9b0d0c01-0000-4000-8000-000000000004'),
    ('9b0d0c00-0000-4000-8000-000000000005', 'kitchen_cancellation', 'Predeterminada', true, true, '9b0d0c01-0000-4000-8000-000000000005'),
    ('9b0d0c00-0000-4000-8000-000000000006', 'cash_income', 'Predeterminada', true, true, '9b0d0c01-0000-4000-8000-000000000006'),
    ('9b0d0c00-0000-4000-8000-000000000007', 'cash_withdrawal', 'Predeterminada', true, true, '9b0d0c01-0000-4000-8000-000000000007'),
    ('9b0d0c00-0000-4000-8000-000000000008', 'cash_expense', 'Predeterminada', true, true, '9b0d0c01-0000-4000-8000-000000000008'),
    ('9b0d0c00-0000-4000-8000-000000000009', 'cash_partial_cut', 'Predeterminada', true, true, '9b0d0c01-0000-4000-8000-000000000009'),
    ('9b0d0c00-0000-4000-8000-000000000010', 'cash_final_cut', 'Predeterminada', true, true, '9b0d0c01-0000-4000-8000-000000000010');

INSERT INTO app.document_template_versions (id, template_id, number, layout, note)
SELECT seed.id::uuid, seed.template_id::uuid, 1, seed.layout::jsonb, 'Formato predeterminado, igual al anterior a la etapa 9B'
FROM (VALUES
    ('9b0d0c01-0000-4000-8000-000000000001', '9b0d0c00-0000-4000-8000-000000000001', '{"options":{"text_size":"normal"},"blocks":[
        {"id":"name","type":"establishment_name","visible":true,"align":"left","size":"normal","bold":false,"uppercase":false,"spacing_before":0,"spacing_after":0,"options":{}},
        {"id":"title","type":"document_title","visible":true,"align":"left","size":"normal","bold":false,"uppercase":false,"spacing_before":0,"spacing_after":0,"options":{"text":""}},
        {"id":"account","type":"field","visible":true,"align":"left","size":"normal","bold":false,"uppercase":false,"spacing_before":0,"spacing_after":0,"options":{"items":[{"key":"folio","label":"Folio"},{"key":"account_name","label":""}],"layout":"inline","omit_empty":false}},
        {"id":"context","type":"field","visible":true,"align":"left","size":"normal","bold":false,"uppercase":false,"spacing_before":0,"spacing_after":0,"options":{"items":[{"key":"context","label":""},{"key":"datetime","label":""}],"layout":"inline","omit_empty":false}},
        {"id":"cashier","type":"field","visible":true,"align":"left","size":"normal","bold":false,"uppercase":false,"spacing_before":0,"spacing_after":0,"options":{"items":[{"key":"cashier","label":"Atendió:"}],"layout":"inline","omit_empty":false}},
        {"id":"divider-1","type":"divider","visible":true,"align":"left","size":"normal","bold":false,"uppercase":false,"spacing_before":0,"spacing_after":0,"options":{"character":"-"}},
        {"id":"products","type":"products","visible":true,"align":"left","size":"normal","bold":false,"uppercase":false,"spacing_before":0,"spacing_after":0,"options":{"show_notes":true}},
        {"id":"adjustments","type":"adjustments","visible":true,"align":"left","size":"normal","bold":false,"uppercase":false,"spacing_before":0,"spacing_after":0,"options":{"show_reason":true}},
        {"id":"divider-2","type":"divider","visible":true,"align":"left","size":"normal","bold":false,"uppercase":false,"spacing_before":0,"spacing_after":0,"options":{"character":"-"}},
        {"id":"taxes","type":"taxes","visible":true,"align":"left","size":"normal","bold":false,"uppercase":false,"spacing_before":0,"spacing_after":0,"options":{"show_base":true}},
        {"id":"total","type":"total","visible":true,"align":"left","size":"normal","bold":false,"uppercase":false,"spacing_before":0,"spacing_after":0,"options":{}},
        {"id":"usd-total","type":"usd_total","visible":true,"align":"left","size":"normal","bold":false,"uppercase":false,"spacing_before":0,"spacing_after":0,"options":{}},
        {"id":"this-payment","type":"this_payment","visible":true,"align":"left","size":"normal","bold":false,"uppercase":false,"spacing_before":0,"spacing_after":0,"options":{"divider":true,"heading":true}},
        {"id":"payments","type":"payments","visible":true,"align":"left","size":"normal","bold":false,"uppercase":false,"spacing_before":0,"spacing_after":0,"options":{"divider":true,"heading":true}},
        {"id":"balance","type":"balance","visible":true,"align":"left","size":"normal","bold":false,"uppercase":false,"spacing_before":0,"spacing_after":0,"options":{}},
        {"id":"divider-3","type":"divider","visible":true,"align":"left","size":"normal","bold":false,"uppercase":false,"spacing_before":0,"spacing_after":0,"options":{"character":"-"}},
        {"id":"fiscal","type":"fiscal_notice","visible":true,"align":"left","size":"normal","bold":false,"uppercase":false,"spacing_before":0,"spacing_after":0,"options":{}}
    ]}'),
    ('9b0d0c01-0000-4000-8000-000000000002', '9b0d0c00-0000-4000-8000-000000000002', '{"options":{"text_size":"normal"},"blocks":[
        {"id":"name","type":"establishment_name","visible":true,"align":"left","size":"normal","bold":false,"uppercase":false,"spacing_before":0,"spacing_after":0,"options":{}},
        {"id":"title","type":"document_title","visible":true,"align":"left","size":"normal","bold":false,"uppercase":false,"spacing_before":0,"spacing_after":0,"options":{"text":""}},
        {"id":"account","type":"field","visible":true,"align":"left","size":"normal","bold":false,"uppercase":false,"spacing_before":0,"spacing_after":0,"options":{"items":[{"key":"folio","label":"Folio"},{"key":"account_name","label":""}],"layout":"inline","omit_empty":false}},
        {"id":"context","type":"field","visible":true,"align":"left","size":"normal","bold":false,"uppercase":false,"spacing_before":0,"spacing_after":0,"options":{"items":[{"key":"context","label":""},{"key":"datetime","label":""}],"layout":"inline","omit_empty":false}},
        {"id":"cashier","type":"field","visible":true,"align":"left","size":"normal","bold":false,"uppercase":false,"spacing_before":0,"spacing_after":0,"options":{"items":[{"key":"cashier","label":"Atendió:"}],"layout":"inline","omit_empty":false}},
        {"id":"divider-1","type":"divider","visible":true,"align":"left","size":"normal","bold":false,"uppercase":false,"spacing_before":0,"spacing_after":0,"options":{"character":"-"}},
        {"id":"products","type":"products","visible":true,"align":"left","size":"normal","bold":false,"uppercase":false,"spacing_before":0,"spacing_after":0,"options":{"show_notes":true}},
        {"id":"adjustments","type":"adjustments","visible":true,"align":"left","size":"normal","bold":false,"uppercase":false,"spacing_before":0,"spacing_after":0,"options":{"show_reason":true}},
        {"id":"divider-2","type":"divider","visible":true,"align":"left","size":"normal","bold":false,"uppercase":false,"spacing_before":0,"spacing_after":0,"options":{"character":"-"}},
        {"id":"taxes","type":"taxes","visible":true,"align":"left","size":"normal","bold":false,"uppercase":false,"spacing_before":0,"spacing_after":0,"options":{"show_base":true}},
        {"id":"total","type":"total","visible":true,"align":"left","size":"normal","bold":false,"uppercase":false,"spacing_before":0,"spacing_after":0,"options":{}},
        {"id":"usd-total","type":"usd_total","visible":true,"align":"left","size":"normal","bold":false,"uppercase":false,"spacing_before":0,"spacing_after":0,"options":{}},
        {"id":"payments","type":"payments","visible":true,"align":"left","size":"normal","bold":false,"uppercase":false,"spacing_before":0,"spacing_after":0,"options":{"divider":true,"heading":true}},
        {"id":"balance","type":"balance","visible":true,"align":"left","size":"normal","bold":false,"uppercase":false,"spacing_before":0,"spacing_after":0,"options":{}},
        {"id":"divider-3","type":"divider","visible":true,"align":"left","size":"normal","bold":false,"uppercase":false,"spacing_before":0,"spacing_after":0,"options":{"character":"-"}},
        {"id":"fiscal","type":"fiscal_notice","visible":true,"align":"left","size":"normal","bold":false,"uppercase":false,"spacing_before":0,"spacing_after":0,"options":{}}
    ]}'),
    ('9b0d0c01-0000-4000-8000-000000000003', '9b0d0c00-0000-4000-8000-000000000003', '{"options":{"text_size":"inherit"},"blocks":[
        {"id":"name","type":"establishment_name","visible":true,"align":"left","size":"normal","bold":false,"uppercase":false,"spacing_before":0,"spacing_after":0,"options":{"when":"kitchen_setting"}},
        {"id":"title","type":"document_title","visible":true,"align":"left","size":"normal","bold":false,"uppercase":false,"spacing_before":0,"spacing_after":0,"options":{"text":""}},
        {"id":"date-time","type":"field","visible":true,"align":"left","size":"normal","bold":false,"uppercase":false,"spacing_before":0,"spacing_after":0,"options":{"items":[{"key":"datetime","label":""}],"layout":"inline","omit_empty":false}},
        {"id":"account","type":"field","visible":true,"align":"left","size":"normal","bold":false,"uppercase":false,"spacing_before":0,"spacing_after":0,"options":{"items":[{"key":"folio","label":"Cuenta"},{"key":"account_name","label":""}],"layout":"inline","omit_empty":false}},
        {"id":"location","type":"kitchen_location","visible":true,"align":"left","size":"normal","bold":false,"uppercase":false,"spacing_before":0,"spacing_after":0,"options":{"show_customer":true}},
        {"id":"employee","type":"field","visible":true,"align":"left","size":"normal","bold":false,"uppercase":false,"spacing_before":0,"spacing_after":0,"options":{"items":[{"key":"employee","label":"Capturó:"}],"layout":"inline","omit_empty":false}},
        {"id":"order","type":"field","visible":true,"align":"left","size":"normal","bold":false,"uppercase":false,"spacing_before":0,"spacing_after":0,"options":{"items":[{"key":"order","label":"Pedido:"}],"layout":"inline","omit_empty":false}},
        {"id":"items","type":"kitchen_items","visible":true,"align":"left","size":"normal","bold":false,"uppercase":false,"spacing_before":0,"spacing_after":0,"options":{"show_notes":"inherit","show_prices":"inherit","quantity_style":"inherit","line_spacing":"inherit","station_divider":true,"show_station_headings":true,"show_item_reference":true}},
        {"id":"divider-end","type":"divider","visible":true,"align":"left","size":"normal","bold":false,"uppercase":false,"spacing_before":0,"spacing_after":0,"options":{"character":"-"}},
        {"id":"document","type":"field","visible":true,"align":"left","size":"normal","bold":false,"uppercase":false,"spacing_before":0,"spacing_after":0,"options":{"items":[{"key":"document","label":"Documento:"}],"layout":"inline","omit_empty":false}}
    ]}'),
    ('9b0d0c01-0000-4000-8000-000000000004', '9b0d0c00-0000-4000-8000-000000000004', '{"options":{"text_size":"inherit"},"blocks":[
        {"id":"name","type":"establishment_name","visible":true,"align":"left","size":"normal","bold":false,"uppercase":false,"spacing_before":0,"spacing_after":0,"options":{"when":"kitchen_setting"}},
        {"id":"title","type":"document_title","visible":true,"align":"left","size":"normal","bold":false,"uppercase":false,"spacing_before":0,"spacing_after":0,"options":{"text":""}},
        {"id":"date-time","type":"field","visible":true,"align":"left","size":"normal","bold":false,"uppercase":false,"spacing_before":0,"spacing_after":0,"options":{"items":[{"key":"datetime","label":""}],"layout":"inline","omit_empty":false}},
        {"id":"account","type":"field","visible":true,"align":"left","size":"normal","bold":false,"uppercase":false,"spacing_before":0,"spacing_after":0,"options":{"items":[{"key":"folio","label":"Cuenta"},{"key":"account_name","label":""}],"layout":"inline","omit_empty":false}},
        {"id":"location","type":"kitchen_location","visible":true,"align":"left","size":"normal","bold":false,"uppercase":false,"spacing_before":0,"spacing_after":0,"options":{"show_customer":true}},
        {"id":"employee","type":"field","visible":true,"align":"left","size":"normal","bold":false,"uppercase":false,"spacing_before":0,"spacing_after":0,"options":{"items":[{"key":"employee","label":"Capturó:"}],"layout":"inline","omit_empty":false}},
        {"id":"order","type":"field","visible":true,"align":"left","size":"normal","bold":false,"uppercase":false,"spacing_before":0,"spacing_after":0,"options":{"items":[{"key":"order","label":"Pedido:"}],"layout":"inline","omit_empty":false}},
        {"id":"items","type":"kitchen_items","visible":true,"align":"left","size":"normal","bold":false,"uppercase":false,"spacing_before":0,"spacing_after":0,"options":{"show_notes":"inherit","show_prices":"inherit","quantity_style":"inherit","line_spacing":"inherit","station_divider":true,"show_station_headings":true,"show_item_reference":true}},
        {"id":"divider-end","type":"divider","visible":true,"align":"left","size":"normal","bold":false,"uppercase":false,"spacing_before":0,"spacing_after":0,"options":{"character":"-"}},
        {"id":"document","type":"field","visible":true,"align":"left","size":"normal","bold":false,"uppercase":false,"spacing_before":0,"spacing_after":0,"options":{"items":[{"key":"document","label":"Documento:"}],"layout":"inline","omit_empty":false}}
    ]}'),
    ('9b0d0c01-0000-4000-8000-000000000005', '9b0d0c00-0000-4000-8000-000000000005', '{"options":{"text_size":"inherit"},"blocks":[
        {"id":"name","type":"establishment_name","visible":true,"align":"left","size":"normal","bold":false,"uppercase":false,"spacing_before":0,"spacing_after":0,"options":{"when":"kitchen_setting"}},
        {"id":"title","type":"document_title","visible":true,"align":"left","size":"normal","bold":false,"uppercase":false,"spacing_before":0,"spacing_after":0,"options":{"text":""}},
        {"id":"date-time","type":"field","visible":true,"align":"left","size":"normal","bold":false,"uppercase":false,"spacing_before":0,"spacing_after":0,"options":{"items":[{"key":"datetime","label":""}],"layout":"inline","omit_empty":false}},
        {"id":"account","type":"field","visible":true,"align":"left","size":"normal","bold":false,"uppercase":false,"spacing_before":0,"spacing_after":0,"options":{"items":[{"key":"folio","label":"Cuenta"},{"key":"account_name","label":""}],"layout":"inline","omit_empty":false}},
        {"id":"location","type":"kitchen_location","visible":true,"align":"left","size":"normal","bold":false,"uppercase":false,"spacing_before":0,"spacing_after":0,"options":{"show_customer":true}},
        {"id":"employee","type":"field","visible":true,"align":"left","size":"normal","bold":false,"uppercase":false,"spacing_before":0,"spacing_after":0,"options":{"items":[{"key":"employee","label":"Capturó:"}],"layout":"inline","omit_empty":false}},
        {"id":"order","type":"field","visible":true,"align":"left","size":"normal","bold":false,"uppercase":false,"spacing_before":0,"spacing_after":0,"options":{"items":[{"key":"order","label":"Pedido:"}],"layout":"inline","omit_empty":false}},
        {"id":"items","type":"kitchen_items","visible":true,"align":"left","size":"normal","bold":false,"uppercase":false,"spacing_before":0,"spacing_after":0,"options":{"show_notes":"inherit","show_prices":"inherit","quantity_style":"inherit","line_spacing":"inherit","station_divider":true,"show_station_headings":true,"show_item_reference":true}},
        {"id":"divider-end","type":"divider","visible":true,"align":"left","size":"normal","bold":false,"uppercase":false,"spacing_before":0,"spacing_after":0,"options":{"character":"-"}},
        {"id":"document","type":"field","visible":true,"align":"left","size":"normal","bold":false,"uppercase":false,"spacing_before":0,"spacing_after":0,"options":{"items":[{"key":"document","label":"Documento:"}],"layout":"inline","omit_empty":false}}
    ]}'),
    ('9b0d0c01-0000-4000-8000-000000000006', '9b0d0c00-0000-4000-8000-000000000006', '{"options":{"text_size":"normal"},"blocks":[
        {"id":"name","type":"establishment_name","visible":true,"align":"left","size":"normal","bold":false,"uppercase":false,"spacing_before":0,"spacing_after":0,"options":{}},
        {"id":"title","type":"document_title","visible":true,"align":"left","size":"normal","bold":false,"uppercase":false,"spacing_before":0,"spacing_after":0,"options":{"text":""}},
        {"id":"register","type":"field","visible":true,"align":"left","size":"normal","bold":false,"uppercase":false,"spacing_before":0,"spacing_after":0,"options":{"items":[{"key":"register","label":"Caja:"}],"layout":"inline","omit_empty":false}},
        {"id":"shift","type":"field","visible":true,"align":"left","size":"normal","bold":false,"uppercase":false,"spacing_before":0,"spacing_after":0,"options":{"items":[{"key":"business_date","label":"Turno del"}],"layout":"inline","omit_empty":false}},
        {"id":"date-time","type":"field","visible":true,"align":"left","size":"normal","bold":false,"uppercase":false,"spacing_before":0,"spacing_after":0,"options":{"items":[{"key":"datetime","label":"Fecha"}],"layout":"pair","omit_empty":false}},
        {"id":"divider-1","type":"divider","visible":true,"align":"left","size":"normal","bold":false,"uppercase":false,"spacing_before":0,"spacing_after":0,"options":{"character":"-"}},
        {"id":"detail","type":"voucher_detail","visible":true,"align":"left","size":"normal","bold":false,"uppercase":false,"spacing_before":0,"spacing_after":0,"options":{"show_reference":true,"show_notes":true}},
        {"id":"divider-2","type":"divider","visible":true,"align":"left","size":"normal","bold":false,"uppercase":false,"spacing_before":0,"spacing_after":0,"options":{"character":"-"}},
        {"id":"signatures","type":"signatures","visible":true,"align":"left","size":"normal","bold":false,"uppercase":false,"spacing_before":0,"spacing_after":0,"options":{"show_status":true,"signature_line":true}},
        {"id":"divider-3","type":"divider","visible":true,"align":"left","size":"normal","bold":false,"uppercase":false,"spacing_before":0,"spacing_after":0,"options":{"character":"-"}},
        {"id":"fiscal","type":"fiscal_notice","visible":true,"align":"left","size":"normal","bold":false,"uppercase":false,"spacing_before":0,"spacing_after":0,"options":{}}
    ]}'),
    ('9b0d0c01-0000-4000-8000-000000000007', '9b0d0c00-0000-4000-8000-000000000007', '{"options":{"text_size":"normal"},"blocks":[
        {"id":"name","type":"establishment_name","visible":true,"align":"left","size":"normal","bold":false,"uppercase":false,"spacing_before":0,"spacing_after":0,"options":{}},
        {"id":"title","type":"document_title","visible":true,"align":"left","size":"normal","bold":false,"uppercase":false,"spacing_before":0,"spacing_after":0,"options":{"text":""}},
        {"id":"register","type":"field","visible":true,"align":"left","size":"normal","bold":false,"uppercase":false,"spacing_before":0,"spacing_after":0,"options":{"items":[{"key":"register","label":"Caja:"}],"layout":"inline","omit_empty":false}},
        {"id":"shift","type":"field","visible":true,"align":"left","size":"normal","bold":false,"uppercase":false,"spacing_before":0,"spacing_after":0,"options":{"items":[{"key":"business_date","label":"Turno del"}],"layout":"inline","omit_empty":false}},
        {"id":"date-time","type":"field","visible":true,"align":"left","size":"normal","bold":false,"uppercase":false,"spacing_before":0,"spacing_after":0,"options":{"items":[{"key":"datetime","label":"Fecha"}],"layout":"pair","omit_empty":false}},
        {"id":"divider-1","type":"divider","visible":true,"align":"left","size":"normal","bold":false,"uppercase":false,"spacing_before":0,"spacing_after":0,"options":{"character":"-"}},
        {"id":"detail","type":"voucher_detail","visible":true,"align":"left","size":"normal","bold":false,"uppercase":false,"spacing_before":0,"spacing_after":0,"options":{"show_reference":true,"show_notes":true}},
        {"id":"divider-2","type":"divider","visible":true,"align":"left","size":"normal","bold":false,"uppercase":false,"spacing_before":0,"spacing_after":0,"options":{"character":"-"}},
        {"id":"signatures","type":"signatures","visible":true,"align":"left","size":"normal","bold":false,"uppercase":false,"spacing_before":0,"spacing_after":0,"options":{"show_status":true,"signature_line":true}},
        {"id":"divider-3","type":"divider","visible":true,"align":"left","size":"normal","bold":false,"uppercase":false,"spacing_before":0,"spacing_after":0,"options":{"character":"-"}},
        {"id":"fiscal","type":"fiscal_notice","visible":true,"align":"left","size":"normal","bold":false,"uppercase":false,"spacing_before":0,"spacing_after":0,"options":{}}
    ]}'),
    ('9b0d0c01-0000-4000-8000-000000000008', '9b0d0c00-0000-4000-8000-000000000008', '{"options":{"text_size":"normal"},"blocks":[
        {"id":"name","type":"establishment_name","visible":true,"align":"left","size":"normal","bold":false,"uppercase":false,"spacing_before":0,"spacing_after":0,"options":{}},
        {"id":"title","type":"document_title","visible":true,"align":"left","size":"normal","bold":false,"uppercase":false,"spacing_before":0,"spacing_after":0,"options":{"text":""}},
        {"id":"register","type":"field","visible":true,"align":"left","size":"normal","bold":false,"uppercase":false,"spacing_before":0,"spacing_after":0,"options":{"items":[{"key":"register","label":"Caja:"}],"layout":"inline","omit_empty":false}},
        {"id":"shift","type":"field","visible":true,"align":"left","size":"normal","bold":false,"uppercase":false,"spacing_before":0,"spacing_after":0,"options":{"items":[{"key":"business_date","label":"Turno del"}],"layout":"inline","omit_empty":false}},
        {"id":"date-time","type":"field","visible":true,"align":"left","size":"normal","bold":false,"uppercase":false,"spacing_before":0,"spacing_after":0,"options":{"items":[{"key":"datetime","label":"Fecha"}],"layout":"pair","omit_empty":false}},
        {"id":"divider-1","type":"divider","visible":true,"align":"left","size":"normal","bold":false,"uppercase":false,"spacing_before":0,"spacing_after":0,"options":{"character":"-"}},
        {"id":"detail","type":"voucher_detail","visible":true,"align":"left","size":"normal","bold":false,"uppercase":false,"spacing_before":0,"spacing_after":0,"options":{"show_reference":true,"show_notes":true}},
        {"id":"divider-2","type":"divider","visible":true,"align":"left","size":"normal","bold":false,"uppercase":false,"spacing_before":0,"spacing_after":0,"options":{"character":"-"}},
        {"id":"signatures","type":"signatures","visible":true,"align":"left","size":"normal","bold":false,"uppercase":false,"spacing_before":0,"spacing_after":0,"options":{"show_status":true,"signature_line":true}},
        {"id":"divider-3","type":"divider","visible":true,"align":"left","size":"normal","bold":false,"uppercase":false,"spacing_before":0,"spacing_after":0,"options":{"character":"-"}},
        {"id":"fiscal","type":"fiscal_notice","visible":true,"align":"left","size":"normal","bold":false,"uppercase":false,"spacing_before":0,"spacing_after":0,"options":{}}
    ]}'),
    ('9b0d0c01-0000-4000-8000-000000000009', '9b0d0c00-0000-4000-8000-000000000009', '{"options":{"text_size":"normal"},"blocks":[
        {"id":"name","type":"establishment_name","visible":true,"align":"left","size":"normal","bold":false,"uppercase":false,"spacing_before":0,"spacing_after":0,"options":{}},
        {"id":"title","type":"document_title","visible":true,"align":"left","size":"normal","bold":false,"uppercase":false,"spacing_before":0,"spacing_after":0,"options":{"text":""}},
        {"id":"register","type":"field","visible":true,"align":"left","size":"normal","bold":false,"uppercase":false,"spacing_before":0,"spacing_after":0,"options":{"items":[{"key":"register","label":"Caja:"}],"layout":"inline","omit_empty":false}},
        {"id":"shift","type":"field","visible":true,"align":"left","size":"normal","bold":false,"uppercase":false,"spacing_before":0,"spacing_after":0,"options":{"items":[{"key":"business_date","label":"Turno del"},{"key":"opened_by","label":"responsable"}],"layout":"inline","omit_empty":false}},
        {"id":"opened-at","type":"field","visible":true,"align":"left","size":"normal","bold":false,"uppercase":false,"spacing_before":0,"spacing_after":0,"options":{"items":[{"key":"opened_at","label":"Apertura"}],"layout":"pair","omit_empty":false}},
        {"id":"cut-at","type":"field","visible":true,"align":"left","size":"normal","bold":false,"uppercase":false,"spacing_before":0,"spacing_after":0,"options":{"items":[{"key":"cut_at","label":"Corte"}],"layout":"pair","omit_empty":false}},
        {"id":"previous-cut","type":"field","visible":true,"align":"left","size":"normal","bold":false,"uppercase":false,"spacing_before":0,"spacing_after":0,"options":{"items":[{"key":"previous_cut_at","label":"Corte anterior"}],"layout":"pair","omit_empty":true}},
        {"id":"made-by","type":"field","visible":true,"align":"left","size":"normal","bold":false,"uppercase":false,"spacing_before":0,"spacing_after":0,"options":{"items":[{"key":"created_by","label":"Realizó:"},{"key":"device","label":""}],"layout":"inline","omit_empty":false}},
        {"id":"divider-1","type":"divider","visible":true,"align":"left","size":"normal","bold":false,"uppercase":false,"spacing_before":0,"spacing_after":0,"options":{"character":"-"}},
        {"id":"float","type":"opening_float","visible":true,"align":"left","size":"normal","bold":false,"uppercase":false,"spacing_before":0,"spacing_after":0,"options":{}},
        {"id":"divider-2","type":"divider","visible":true,"align":"left","size":"normal","bold":false,"uppercase":false,"spacing_before":0,"spacing_after":0,"options":{"character":"-"}},
        {"id":"sales","type":"sales_by_method","visible":true,"align":"left","size":"normal","bold":false,"uppercase":false,"spacing_before":0,"spacing_after":0,"options":{"heading":true}},
        {"id":"totals","type":"shift_totals","visible":true,"align":"left","size":"normal","bold":false,"uppercase":false,"spacing_before":0,"spacing_after":0,"options":{}},
        {"id":"divider-3","type":"divider","visible":true,"align":"left","size":"normal","bold":false,"uppercase":false,"spacing_before":0,"spacing_after":0,"options":{"character":"-"}},
        {"id":"cash","type":"cash_movements","visible":true,"align":"left","size":"normal","bold":false,"uppercase":false,"spacing_before":0,"spacing_after":0,"options":{"heading":true}},
        {"id":"divider-4","type":"divider","visible":true,"align":"left","size":"normal","bold":false,"uppercase":false,"spacing_before":0,"spacing_after":0,"options":{"character":"-"}},
        {"id":"expected","type":"expected_differences","visible":true,"align":"left","size":"normal","bold":false,"uppercase":false,"spacing_before":0,"spacing_after":0,"options":{}},
        {"id":"divider-5","type":"divider","visible":true,"align":"left","size":"normal","bold":false,"uppercase":false,"spacing_before":0,"spacing_after":0,"options":{"character":"-"}},
        {"id":"fiscal","type":"fiscal_notice","visible":true,"align":"left","size":"normal","bold":false,"uppercase":false,"spacing_before":0,"spacing_after":0,"options":{}}
    ]}'),
    ('9b0d0c01-0000-4000-8000-000000000010', '9b0d0c00-0000-4000-8000-000000000010', '{"options":{"text_size":"normal"},"blocks":[
        {"id":"name","type":"establishment_name","visible":true,"align":"left","size":"normal","bold":false,"uppercase":false,"spacing_before":0,"spacing_after":0,"options":{}},
        {"id":"title","type":"document_title","visible":true,"align":"left","size":"normal","bold":false,"uppercase":false,"spacing_before":0,"spacing_after":0,"options":{"text":""}},
        {"id":"register","type":"field","visible":true,"align":"left","size":"normal","bold":false,"uppercase":false,"spacing_before":0,"spacing_after":0,"options":{"items":[{"key":"register","label":"Caja:"}],"layout":"inline","omit_empty":false}},
        {"id":"shift","type":"field","visible":true,"align":"left","size":"normal","bold":false,"uppercase":false,"spacing_before":0,"spacing_after":0,"options":{"items":[{"key":"business_date","label":"Turno del"},{"key":"opened_by","label":"responsable"}],"layout":"inline","omit_empty":false}},
        {"id":"opened-at","type":"field","visible":true,"align":"left","size":"normal","bold":false,"uppercase":false,"spacing_before":0,"spacing_after":0,"options":{"items":[{"key":"opened_at","label":"Apertura"}],"layout":"pair","omit_empty":false}},
        {"id":"cut-at","type":"field","visible":true,"align":"left","size":"normal","bold":false,"uppercase":false,"spacing_before":0,"spacing_after":0,"options":{"items":[{"key":"cut_at","label":"Cierre"}],"layout":"pair","omit_empty":false}},
        {"id":"closed-by","type":"field","visible":true,"align":"left","size":"normal","bold":false,"uppercase":false,"spacing_before":0,"spacing_after":0,"options":{"items":[{"key":"closed_by","label":"Cerró:"}],"layout":"inline","omit_empty":true}},
        {"id":"made-by","type":"field","visible":true,"align":"left","size":"normal","bold":false,"uppercase":false,"spacing_before":0,"spacing_after":0,"options":{"items":[{"key":"created_by","label":"Realizó:"},{"key":"device","label":""}],"layout":"inline","omit_empty":false}},
        {"id":"divider-1","type":"divider","visible":true,"align":"left","size":"normal","bold":false,"uppercase":false,"spacing_before":0,"spacing_after":0,"options":{"character":"-"}},
        {"id":"float","type":"opening_float","visible":true,"align":"left","size":"normal","bold":false,"uppercase":false,"spacing_before":0,"spacing_after":0,"options":{}},
        {"id":"divider-2","type":"divider","visible":true,"align":"left","size":"normal","bold":false,"uppercase":false,"spacing_before":0,"spacing_after":0,"options":{"character":"-"}},
        {"id":"sales","type":"sales_by_method","visible":true,"align":"left","size":"normal","bold":false,"uppercase":false,"spacing_before":0,"spacing_after":0,"options":{"heading":true}},
        {"id":"totals","type":"shift_totals","visible":true,"align":"left","size":"normal","bold":false,"uppercase":false,"spacing_before":0,"spacing_after":0,"options":{}},
        {"id":"divider-3","type":"divider","visible":true,"align":"left","size":"normal","bold":false,"uppercase":false,"spacing_before":0,"spacing_after":0,"options":{"character":"-"}},
        {"id":"cash","type":"cash_movements","visible":true,"align":"left","size":"normal","bold":false,"uppercase":false,"spacing_before":0,"spacing_after":0,"options":{"heading":true}},
        {"id":"declared","type":"declared_cash","visible":true,"align":"left","size":"normal","bold":false,"uppercase":false,"spacing_before":0,"spacing_after":0,"options":{"divider":true,"heading":true}},
        {"id":"divider-4","type":"divider","visible":true,"align":"left","size":"normal","bold":false,"uppercase":false,"spacing_before":0,"spacing_after":0,"options":{"character":"-"}},
        {"id":"expected","type":"expected_differences","visible":true,"align":"left","size":"normal","bold":false,"uppercase":false,"spacing_before":0,"spacing_after":0,"options":{}},
        {"id":"observations","type":"observations","visible":true,"align":"left","size":"normal","bold":false,"uppercase":false,"spacing_before":0,"spacing_after":0,"options":{"divider":true,"heading":true}},
        {"id":"divider-5","type":"divider","visible":true,"align":"left","size":"normal","bold":false,"uppercase":false,"spacing_before":0,"spacing_after":0,"options":{"character":"-"}},
        {"id":"fiscal","type":"fiscal_notice","visible":true,"align":"left","size":"normal","bold":false,"uppercase":false,"spacing_before":0,"spacing_after":0,"options":{}}
    ]}')
) AS seed (id, template_id, layout);

-- 4. Cada documento emitido desde la 9B guarda la versión de plantilla con la que se renderizó
-- (NULL = documento anterior: se reimprime con la versión 1 del sistema de su tipo). La cola agrega
-- dos tipos de ticket ya renderizados: las muestras del diseñador (document_sample) y los
-- históricos impresos con la plantilla vigente (document_rerender); ninguno aparece entre las
-- comandas ni crea ventas, pagos o documentos comerciales.
ALTER TABLE app.kitchen_tickets
    ADD COLUMN template_version_id uuid
        CONSTRAINT kitchen_tickets_template_version_fkey REFERENCES app.document_template_versions(id);
ALTER TABLE app.kitchen_tickets DROP CONSTRAINT kitchen_tickets_kind_check;
ALTER TABLE app.kitchen_tickets
    ADD CONSTRAINT kitchen_tickets_kind_check CHECK (
        kind IN (
            'normal', 'modification', 'cancellation', 'test', 'receipt', 'precheck', 'cash_report', 'cash_voucher', 'cash_drawer',
            'document_sample', 'document_rerender'
        )
    );

-- 5. IR de los trabajos con negrita, tamaños o logotipo. NULL en los de texto plano (todos los de
-- las plantillas predeterminadas): esos trabajos son idénticos a los anteriores a la 9B.
ALTER TABLE app.kitchen_print_jobs
    ADD COLUMN layout jsonb
        CONSTRAINT kitchen_print_jobs_layout_check CHECK (layout IS NULL OR (layout IS NOT NULL AND jsonb_typeof(layout) = 'object'));

-- 6. Impresoras CUPS que entienden ESC/POS. Una cola CUPS que admite datos raw entrega los bytes tal
-- cual, pero eso no garantiza que la impresora entienda ESC/POS (una impresora de oficina o una que
-- solo admite raster con su controlador imprimiría basura). Por eso los documentos con negrita,
-- tamaños o logotipo solo viajan como ESC/POS a una cola CUPS si quien administra la impresora lo
-- indica de forma explícita («Esta impresora entiende ESC/POS (formato y logotipo)»), desactivado por
-- omisión, y además la cola admite raw; si no, se imprime el texto con aviso. ESC/POS por red ya los
-- recibe: la opción solo existe en CUPS. Las impresoras existentes quedan desactivadas.
ALTER TABLE app.kitchen_printers
    ADD COLUMN accepts_escpos boolean NOT NULL DEFAULT false,
    ADD CONSTRAINT kitchen_printers_accepts_escpos_check CHECK (NOT accepts_escpos OR transport = 'cups');

-- 7. Ámbito SSE de los documentos: avisa que cambió una plantilla (y, desde la fase B, la identidad
-- documental), sin contenido. Lo reciben quienes consultan plantillas (events/store.go).
ALTER TABLE app.realtime_events DROP CONSTRAINT realtime_events_scope_check;
ALTER TABLE app.realtime_events
    ADD CONSTRAINT realtime_events_scope_check CHECK (
        scope IN ('restaurant', 'takeaway', 'kitchen', 'printing', 'configuration', 'cash', 'inventory', 'documents')
    );

-- 8. Permisos del diseñador de documentos. Se registran también en Go (documents.Permissions) con la
-- misma descripción y se conceden solo al administrador (para que siga contando como administrador
-- completo); ningún otro rol los recibe por inferencia. La vista previa con un documento real y la
-- impresión con la plantilla vigente exigen además los permisos de su origen.
INSERT INTO app.permissions(key, description) VALUES
    ('documents.templates.read', 'Consultar plantillas de documentos, versiones y vista previa con datos de muestra'),
    ('documents.templates.manage', 'Crear, editar, duplicar, renombrar, activar y restaurar plantillas de documentos'),
    ('documents.preview.real', 'Previsualizar plantillas con un documento real (además del permiso de su origen)'),
    ('documents.samples.print', 'Imprimir muestras de plantillas de documentos'),
    ('documents.identity.manage', 'Modificar la identidad documental y el logotipo del establecimiento'),
    ('documents.history.render_current', 'Imprimir un documento histórico con la plantilla vigente (además del permiso de reimprimir)');

INSERT INTO app.role_permissions(role_id, permission_key)
SELECT r.id, p.key FROM app.roles r CROSS JOIN app.permissions p
WHERE r.code = 'administrator' AND p.key IN (
    'documents.templates.read', 'documents.templates.manage', 'documents.preview.real', 'documents.samples.print',
    'documents.identity.manage', 'documents.history.render_current'
)
ON CONFLICT DO NOTHING;
