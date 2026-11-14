-- Etapa 9G: la única migración de la etapa (servidor del restaurante, terminales en red y preparación para Android;
-- esquema 23, decisión D12 del coordinador). Cada fase agrega aquí su propia sección con el encabezado «Fase X». Es
-- transaccional (el migrador aplica todas las pendientes en una sola transacción) y no modifica filas operativas:
-- ventas, pagos, cortes, cuentas, pedidos, trabajos de impresión ni sus fotografías.

-- =====================================================================================================================
-- Fase D · API con versión y preparación para Android (§5 de la especificación, decisiones D8, D9 y D12)
-- =====================================================================================================================

-- 1. «Agotado» (§5.4, decisión D8). Es distinto de «Pausar» (temporary_unavailable, que no cambia): lo marca quien
--    atiende desde la captura del punto de venta y vuelve solo a «disponible» con el inicio del día comercial
--    siguiente («Hora de inicio del día», inventory_settings.operating_day_cutoff), salvo que se marque como agotado
--    hasta nuevo aviso. sold_out_at es cuándo se marcó, sold_out_until el instante en que deja de estar agotado (Go lo
--    calcula al marcarlo con internal/businessday; NULL = hasta nuevo aviso) y sold_out_by quién lo marcó. Un producto
--    está agotado si sold_out_at no es nulo y sold_out_until es nulo o posterior al momento de la consulta; un
--    trabajador de Go limpia los vencidos y avisa a las terminales. Marcarlo no cambia la revisión del producto: no
--    interrumpe a quien edita el producto en Catálogo.
ALTER TABLE app.catalog_products
    ADD COLUMN sold_out_at timestamptz,
    ADD COLUMN sold_out_until timestamptz,
    ADD COLUMN sold_out_by uuid
        CONSTRAINT catalog_products_sold_out_by_fkey REFERENCES app.users(id),
    ADD CONSTRAINT catalog_products_sold_out_check CHECK (
        (sold_out_at IS NULL AND sold_out_until IS NULL AND sold_out_by IS NULL)
        OR (sold_out_at IS NOT NULL AND (sold_out_until IS NULL OR sold_out_until > sold_out_at))
    );
CREATE INDEX catalog_products_sold_out_idx ON app.catalog_products (sold_out_until) WHERE sold_out_at IS NOT NULL;

-- 2. Permiso propio de «Agotado» (D8), solo al administrador (en una instalación nueva el asistente concede al
--    administrador todos los permisos después de las migraciones; a los demás roles se les asigna desde Personal). Se
--    registra también en Go (catalog.Permissions) con la misma descripción.
INSERT INTO app.permissions(key, description) VALUES
    ('catalog.sold_out.manage', 'Marcar productos como agotados y disponibles otra vez desde la captura');
INSERT INTO app.role_permissions(role_id, permission_key)
SELECT id, 'catalog.sold_out.manage' FROM app.roles WHERE code = 'administrator';

-- 3. El catálogo publica eventos en tiempo real (D8): ámbito «catalog», que reciben quienes consultan el catálogo
--    (events/store.go). Los eventos solo identifican qué cambió, nunca precios. Si otra fase de esta migración redefine
--    esta restricción después de esta sección, debe conservar 'catalog'.
ALTER TABLE app.realtime_events DROP CONSTRAINT realtime_events_scope_check;
ALTER TABLE app.realtime_events
    ADD CONSTRAINT realtime_events_scope_check CHECK (
        scope IN ('restaurant', 'takeaway', 'kitchen', 'printing', 'configuration', 'cash', 'inventory', 'documents', 'catalog')
    );

-- 4. Versión del catálogo descargable (§5.3). Cada transacción que cambia el catálogo toma un número nuevo de la
--    secuencia (uno por transacción, no por renglón) y deja en catalog_change_log, por cada registro que cambió, el
--    último número y si se borró. La versión vigente es el máximo de la bitácora (0 si nunca cambió nada después de
--    esta migración). Un cliente descarga el catálogo completo y después solo «los cambios desde la versión X»: los
--    registros con un número mayor que X, con las bajas (borrados) incluidas. La bitácora guarda una sola fila por
--    registro, así que no crece con cada edición.
--    Consistencia: toda escritura del catálogo toma el lock 7820148304 exclusivo (catalog.Handle, «Quitar datos de
--    prueba») y la descarga lo toma compartido, así que nunca se lee una versión mayor mientras otra transacción con un
--    número menor sigue sin confirmarse. El disparador lo toma también (es reentrante) para que ninguna escritura
--    futura lo olvide; el cambio de la tasa general de IVA lo toma después de 7820148305, en el orden de siempre.
--    Entidades: category, product (incluye presentaciones, ingredientes del producto, grupos y complementos incluidos,
--    combos y sus precios en las listas), ingredient, complement, portion, group (incluye sus opciones y tramos),
--    price_list, quick_note, image y tax (la tasa general de IVA, con un identificador fijo). Las promociones no se
--    descargan: el servidor las aplica al confirmar.
CREATE SEQUENCE app.catalog_version_seq AS bigint;

CREATE TABLE app.catalog_change_log (
    entity text NOT NULL CONSTRAINT catalog_change_log_entity_check CHECK (
        entity IN ('category', 'product', 'ingredient', 'complement', 'portion', 'group', 'price_list', 'quick_note',
                   'image', 'tax')
    ),
    entity_id uuid NOT NULL,
    version bigint NOT NULL CONSTRAINT catalog_change_log_version_check CHECK (version > 0),
    deleted boolean NOT NULL DEFAULT false,
    changed_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (entity, entity_id)
);
CREATE INDEX catalog_change_log_version_idx ON app.catalog_change_log (version);

-- catalog_record_change deja en la bitácora el registro afectado con el número de versión de la transacción. El
-- número se guarda en una variable local de la transacción (adel.catalog_version) para que todos los renglones de la
-- misma transacción compartan uno solo.
CREATE FUNCTION app.catalog_note_change(target_entity text, target_id uuid, removed boolean) RETURNS void
    LANGUAGE plpgsql AS $$
DECLARE
    current_version bigint;
BEGIN
    IF target_id IS NULL THEN
        RETURN;
    END IF;
    PERFORM pg_advisory_xact_lock(7820148304);
    current_version := nullif(current_setting('adel.catalog_version', true), '')::bigint;
    IF current_version IS NULL THEN
        current_version := nextval('app.catalog_version_seq');
        PERFORM set_config('adel.catalog_version', current_version::text, true);
    END IF;
    INSERT INTO app.catalog_change_log (entity, entity_id, version, deleted, changed_at)
    VALUES (target_entity, target_id, current_version, removed, now())
    ON CONFLICT (entity, entity_id) DO UPDATE
        SET version = excluded.version, deleted = excluded.deleted, changed_at = excluded.changed_at;
END;
$$;

-- El disparador traduce cada tabla del catálogo a la entidad que descarga el cliente. Un renglón hijo (presentación,
-- opción de grupo, ranura de combo…) marca a su padre como cambiado, nunca como borrado; solo el borrado del propio
-- registro es una baja.
CREATE FUNCTION app.catalog_record_change() RETURNS trigger
    LANGUAGE plpgsql AS $$
DECLARE
    old_row jsonb := CASE WHEN TG_OP IN ('UPDATE', 'DELETE') THEN to_jsonb(OLD) END;
    new_row jsonb := CASE WHEN TG_OP IN ('INSERT', 'UPDATE') THEN to_jsonb(NEW) END;
    target_entity text := TG_ARGV[0];
    id_column text := TG_ARGV[1];
    own_record boolean := TG_ARGV[2] = 'own';
BEGIN
    IF TG_TABLE_NAME IN ('catalog_combo_options', 'catalog_combo_tiers') THEN
        -- El combo de una opción o un tramo se encuentra por su ranura; si la ranura ya se borró, su borrado ya
        -- marcó al combo.
        IF old_row IS NOT NULL THEN
            PERFORM app.catalog_note_change('product',
                (SELECT s.combo_id FROM app.catalog_combo_slots s WHERE s.id = (old_row ->> 'slot_id')::uuid), false);
        END IF;
        IF new_row IS NOT NULL THEN
            PERFORM app.catalog_note_change('product',
                (SELECT s.combo_id FROM app.catalog_combo_slots s WHERE s.id = (new_row ->> 'slot_id')::uuid), false);
        END IF;
        RETURN NULL;
    END IF;
    IF TG_TABLE_NAME = 'catalog_option_tiers' THEN
        IF old_row IS NOT NULL THEN
            PERFORM app.catalog_note_change('group',
                (SELECT o.group_id FROM app.catalog_group_options o WHERE o.id = (old_row ->> 'option_id')::uuid), false);
        END IF;
        IF new_row IS NOT NULL THEN
            PERFORM app.catalog_note_change('group',
                (SELECT o.group_id FROM app.catalog_group_options o WHERE o.id = (new_row ->> 'option_id')::uuid), false);
        END IF;
        RETURN NULL;
    END IF;
    IF TG_TABLE_NAME = 'payment_settings' THEN
        PERFORM app.catalog_note_change('tax', '00000000-0000-0000-0000-000000000000'::uuid, false);
        RETURN NULL;
    END IF;
    IF old_row IS NOT NULL AND (new_row IS NULL OR (old_row ->> id_column) IS DISTINCT FROM (new_row ->> id_column)) THEN
        PERFORM app.catalog_note_change(target_entity, (old_row ->> id_column)::uuid, own_record AND TG_OP = 'DELETE');
    END IF;
    IF new_row IS NOT NULL THEN
        PERFORM app.catalog_note_change(target_entity, (new_row ->> id_column)::uuid, false);
    END IF;
    RETURN NULL;
END;
$$;

CREATE TRIGGER catalog_categories_version AFTER INSERT OR UPDATE OR DELETE ON app.catalog_categories
    FOR EACH ROW EXECUTE FUNCTION app.catalog_record_change('category', 'id', 'own');
CREATE TRIGGER catalog_products_version AFTER INSERT OR UPDATE OR DELETE ON app.catalog_products
    FOR EACH ROW EXECUTE FUNCTION app.catalog_record_change('product', 'id', 'own');
CREATE TRIGGER catalog_variants_version AFTER INSERT OR UPDATE OR DELETE ON app.catalog_variants
    FOR EACH ROW EXECUTE FUNCTION app.catalog_record_change('product', 'product_id', 'child');
CREATE TRIGGER catalog_product_ingredients_version AFTER INSERT OR UPDATE OR DELETE ON app.catalog_product_ingredients
    FOR EACH ROW EXECUTE FUNCTION app.catalog_record_change('product', 'product_id', 'child');
CREATE TRIGGER catalog_product_groups_version AFTER INSERT OR UPDATE OR DELETE ON app.catalog_product_groups
    FOR EACH ROW EXECUTE FUNCTION app.catalog_record_change('product', 'product_id', 'child');
CREATE TRIGGER catalog_group_defaults_version AFTER INSERT OR UPDATE OR DELETE ON app.catalog_group_defaults
    FOR EACH ROW EXECUTE FUNCTION app.catalog_record_change('product', 'product_id', 'child');
CREATE TRIGGER catalog_combo_slots_version AFTER INSERT OR UPDATE OR DELETE ON app.catalog_combo_slots
    FOR EACH ROW EXECUTE FUNCTION app.catalog_record_change('product', 'combo_id', 'child');
CREATE TRIGGER catalog_combo_options_version AFTER INSERT OR UPDATE OR DELETE ON app.catalog_combo_options
    FOR EACH ROW EXECUTE FUNCTION app.catalog_record_change('product', 'slot_id', 'child');
CREATE TRIGGER catalog_combo_tiers_version AFTER INSERT OR UPDATE OR DELETE ON app.catalog_combo_tiers
    FOR EACH ROW EXECUTE FUNCTION app.catalog_record_change('product', 'slot_id', 'child');
CREATE TRIGGER catalog_price_entries_version AFTER INSERT OR UPDATE OR DELETE ON app.catalog_price_entries
    FOR EACH ROW EXECUTE FUNCTION app.catalog_record_change('product', 'product_id', 'child');
CREATE TRIGGER catalog_price_lists_version AFTER INSERT OR UPDATE OR DELETE ON app.catalog_price_lists
    FOR EACH ROW EXECUTE FUNCTION app.catalog_record_change('price_list', 'id', 'own');
CREATE TRIGGER catalog_ingredients_version AFTER INSERT OR UPDATE OR DELETE ON app.catalog_ingredients
    FOR EACH ROW EXECUTE FUNCTION app.catalog_record_change('ingredient', 'id', 'own');
CREATE TRIGGER catalog_complements_version AFTER INSERT OR UPDATE OR DELETE ON app.catalog_complements
    FOR EACH ROW EXECUTE FUNCTION app.catalog_record_change('complement', 'id', 'own');
CREATE TRIGGER catalog_portions_version AFTER INSERT OR UPDATE OR DELETE ON app.catalog_portions
    FOR EACH ROW EXECUTE FUNCTION app.catalog_record_change('portion', 'id', 'own');
CREATE TRIGGER catalog_option_groups_version AFTER INSERT OR UPDATE OR DELETE ON app.catalog_option_groups
    FOR EACH ROW EXECUTE FUNCTION app.catalog_record_change('group', 'id', 'own');
CREATE TRIGGER catalog_group_options_version AFTER INSERT OR UPDATE OR DELETE ON app.catalog_group_options
    FOR EACH ROW EXECUTE FUNCTION app.catalog_record_change('group', 'group_id', 'child');
CREATE TRIGGER catalog_option_tiers_version AFTER INSERT OR UPDATE OR DELETE ON app.catalog_option_tiers
    FOR EACH ROW EXECUTE FUNCTION app.catalog_record_change('group', 'option_id', 'child');
CREATE TRIGGER catalog_category_quick_notes_version AFTER INSERT OR UPDATE OR DELETE ON app.catalog_category_quick_notes
    FOR EACH ROW EXECUTE FUNCTION app.catalog_record_change('quick_note', 'id', 'own');
CREATE TRIGGER catalog_images_version AFTER INSERT OR UPDATE OR DELETE ON app.catalog_images
    FOR EACH ROW EXECUTE FUNCTION app.catalog_record_change('image', 'id', 'own');
CREATE TRIGGER payment_settings_tax_version AFTER UPDATE OF tax_rate ON app.payment_settings
    FOR EACH ROW WHEN (OLD.tax_rate IS DISTINCT FROM NEW.tax_rate)
    EXECUTE FUNCTION app.catalog_record_change('tax', 'singleton', 'child');

-- 5. Claves de operación (§5.2). app.restaurant_operations guarda la respuesta de cada escritura con clave de
--    operación (pedidos, cuentas, cocina, cobros y, desde esta etapa, «Agotado» y el pedido capturado sin conexión) y
--    nunca se borra: la especificación pide conservarlas al menos 7 días.
COMMENT ON TABLE app.restaurant_operations IS
    'Resultado de cada escritura con clave de operación (idempotencia); se conserva para siempre (al menos 7 días, §5.2 de la etapa 9G)';

-- =====================================================================================================================
-- Fase F · Respaldos programados (§7 de la especificación, decisiones D7 y D12)
-- =====================================================================================================================

-- 1. Configuración de los respaldos automáticos (una sola fila), en Sistema › Mantenimiento con settings.manage. El
--    servidor respalda una vez al día: scheduled_time es la hora (NULL = la «Hora de inicio del día»,
--    inventory_settings.operating_day_cutoff, 05:00 por omisión, de modo que cambiarla también mueve el respaldo),
--    keep_count cuántos respaldos automáticos se conservan en la carpeta (los más viejos se borran solos; nunca los
--    manuales ni otros archivos) y directory la carpeta elegida en la computadora principal con el diálogo nativo
--    (NULL = la carpeta predeterminada: «backups» en la carpeta de datos del servicio o backup_directory de la
--    configuración). Go valida la carpeta al elegirla (ruta absoluta, existente y con permiso de escritura) y otra vez
--    en cada respaldo; si entonces no está disponible, usa la predeterminada y lo avisa.
CREATE TABLE app.backup_settings (
    singleton boolean PRIMARY KEY DEFAULT true CONSTRAINT backup_settings_singleton_check CHECK (singleton),
    enabled boolean NOT NULL DEFAULT true,
    scheduled_time time CONSTRAINT backup_settings_scheduled_time_check
        CHECK (scheduled_time IS NULL OR extract(second FROM scheduled_time) = 0),
    keep_count integer NOT NULL DEFAULT 30 CONSTRAINT backup_settings_keep_count_check CHECK (keep_count BETWEEN 1 AND 365),
    directory text CONSTRAINT backup_settings_directory_check
        CHECK (directory IS NULL OR (length(directory) BETWEEN 2 AND 1024 AND directory !~ '[[:cntrl:]]')),
    updated_by uuid CONSTRAINT backup_settings_updated_by_fkey REFERENCES app.users(id),
    updated_at timestamptz NOT NULL DEFAULT now()
);
INSERT INTO app.backup_settings(singleton) VALUES (true);

-- 2. Registro de cada respaldo que hace el servidor: los automáticos (kind 'automatic', con el instante programado al
--    que corresponden) y los de «Respaldar ahora» (kind 'manual', con su clave de operación, quién y desde qué
--    terminal). Los respaldos de la línea de comandos (dev.sh backup y «ad-el-pos-server backup») no pasan por aquí.
--    Cada ejecución guarda inicio y fin, la carpeta configurada al empezar (requested_directory, NULL = la
--    predeterminada), la carpeta usada (directory), si se usó la predeterminada porque la configurada no estaba
--    disponible (used_default_directory con fallback_reason), el archivo y su tamaño, si se verificó (pg_restore
--    --list), el resultado y el motivo de un fallo. removed_at es cuándo la retención borró el archivo. Un respaldo
--    solo cuenta como correcto si quedó verificado. Una fila en 'running' cuyo servidor se detuvo pasa a 'failed'
--    con motivo 'interrupted' la siguiente vez que el servidor toma el lock de los respaldos (7820148309).
CREATE TABLE app.backup_runs (
    id uuid PRIMARY KEY,
    kind text NOT NULL CONSTRAINT backup_runs_kind_check CHECK (kind IN ('automatic', 'manual')),
    status text NOT NULL CONSTRAINT backup_runs_status_check CHECK (status IN ('running', 'succeeded', 'failed')),
    scheduled_for timestamptz,
    started_at timestamptz NOT NULL,
    finished_at timestamptz,
    requested_directory text,
    directory text,
    used_default_directory boolean NOT NULL DEFAULT false,
    fallback_reason text CONSTRAINT backup_runs_fallback_reason_check
        CHECK (fallback_reason IN ('missing', 'not_directory', 'not_writable')),
    file_name text CONSTRAINT backup_runs_file_name_check CHECK (file_name ~ '^[A-Za-z0-9_.-]{1,200}$'),
    size_bytes bigint CONSTRAINT backup_runs_size_bytes_check CHECK (size_bytes > 0),
    verified boolean NOT NULL DEFAULT false,
    failure_reason text CONSTRAINT backup_runs_failure_reason_check
        CHECK (failure_reason IN ('directory_unavailable', 'dump_failed', 'verify_failed', 'interrupted')),
    removed_at timestamptz,
    operation_id uuid CONSTRAINT backup_runs_operation_id_key UNIQUE,
    requested_by uuid CONSTRAINT backup_runs_requested_by_fkey REFERENCES app.users(id),
    requested_device uuid CONSTRAINT backup_runs_requested_device_fkey REFERENCES app.devices(id),
    CONSTRAINT backup_runs_kind_fields_check CHECK (
        (kind = 'automatic' AND scheduled_for IS NOT NULL AND operation_id IS NULL AND requested_by IS NULL
            AND requested_device IS NULL)
        OR (kind = 'manual' AND scheduled_for IS NULL AND operation_id IS NOT NULL AND requested_by IS NOT NULL
            AND requested_device IS NOT NULL)
    ),
    CONSTRAINT backup_runs_status_fields_check CHECK (
        (status = 'running' AND finished_at IS NULL AND failure_reason IS NULL AND NOT verified AND file_name IS NULL
            AND removed_at IS NULL)
        OR (status = 'succeeded' AND finished_at IS NOT NULL AND failure_reason IS NULL AND verified
            AND file_name IS NOT NULL AND size_bytes IS NOT NULL AND directory IS NOT NULL)
        OR (status = 'failed' AND finished_at IS NOT NULL AND failure_reason IS NOT NULL AND NOT verified
            AND file_name IS NULL AND size_bytes IS NULL AND removed_at IS NULL)
    ),
    CONSTRAINT backup_runs_fallback_check CHECK (
        fallback_reason IS NULL OR (used_default_directory AND requested_directory IS NOT NULL)
    ),
    CONSTRAINT backup_runs_finished_check CHECK (finished_at >= started_at)
);
CREATE INDEX backup_runs_started_idx ON app.backup_runs (started_at DESC);
CREATE INDEX backup_runs_running_idx ON app.backup_runs (started_at) WHERE status = 'running';
COMMENT ON TABLE app.backup_settings IS
    'Respaldos automáticos diarios: activo, hora (NULL = hora de inicio del día), cuántos conservar y carpeta (etapa 9G, fase F)';
COMMENT ON TABLE app.backup_runs IS
    'Cada respaldo automático o de «Respaldar ahora» con carpeta, archivo, tamaño, verificación y resultado (etapa 9G, fase F)';
