-- Etapa 9B, fase B (B2): identidad documental del establecimiento y logotipo del restaurante.
-- Transaccional: el migrador la aplica dentro de la misma transacción que la registra en
-- schema_migrations y no contiene sentencias que no admitan transacción (ni CREATE INDEX
-- CONCURRENTLY ni VACUUM), de modo que un fallo la revierte completa. No modifica 0001–0018 ni
-- filas existentes. Solo inserta la fila única de la identidad con textos vacíos, sin logotipo y con
-- la presentación por omisión (configuración, nunca datos ficticios). Las restricciones llevan nombre
-- explícito y los CHECK con columnas opcionales exigen IS NOT NULL en la rama que valida, porque un
-- CHECK que evalúa a NULL se acepta. No hay permisos nuevos: documents.identity.manage es de la 0018.
--
-- El nombre comercial, la dirección, el teléfono y el correo NO se duplican: siguen en
-- app.establishment (Configuración general) y la identidad documental los combina al leerse.

-- 1. Logotipos originales. Cada archivo aceptado (PNG o JPEG validado en Go con
-- internal/logoraster: firma, dimensiones antes de decodificar y decodificación completa) se guarda
-- tal cual, una sola vez por contenido (SHA-256 único y comprobado contra los bytes). Son inmutables:
-- reemplazar o retirar el logotipo solo cambia la referencia de la identidad, nunca borra el
-- original ni sus conversiones, de modo que los documentos ya emitidos conservan su imagen.
CREATE TABLE app.document_logos (
    id uuid NOT NULL CONSTRAINT document_logos_pkey PRIMARY KEY,
    content bytea NOT NULL,
    media_type text NOT NULL CONSTRAINT document_logos_media_type_check
        CHECK (media_type IN ('image/png', 'image/jpeg')),
    width integer NOT NULL CONSTRAINT document_logos_width_check CHECK (width BETWEEN 1 AND 2048),
    height integer NOT NULL CONSTRAINT document_logos_height_check CHECK (height BETWEEN 1 AND 2048),
    size_bytes integer NOT NULL CONSTRAINT document_logos_size_check
        CHECK (size_bytes BETWEEN 1 AND 1048576 AND size_bytes = octet_length(content)),
    sha256 text NOT NULL CONSTRAINT document_logos_sha256_check
        CHECK (sha256 = encode(sha256(content), 'hex')),
    created_by uuid NOT NULL CONSTRAINT document_logos_created_by_fkey REFERENCES app.users(id),
    created_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT document_logos_sha256_key UNIQUE (sha256)
);

-- 2. Conversiones monocromáticas guardadas para no procesar el logotipo en cada impresión: una por
-- logotipo, ancho de papel (384 puntos en 58 mm y 576 en 80 mm, ancho imprimible completo con el
-- relleno de la alineación) y presentación (ancho relativo, alineación, método y umbral). Los bits
-- van por filas de arriba abajo, el bit más significativo es el punto izquierdo y 1 = negro (el
-- formato de logoraster y de GS v 0). El difuminado usa un punto de decisión fijo de 128, por eso su
-- umbral guardado es siempre 128. converter_version identifica el algoritmo de conversión: si un día
-- cambia, las conversiones nuevas no se confunden con las guardadas. Son inmutables (los documentos
-- emitidos las referencian por id, sin llave foránea, y deben reimprimirse idénticos).
CREATE TABLE app.document_logo_rasters (
    id uuid NOT NULL CONSTRAINT document_logo_rasters_pkey PRIMARY KEY,
    logo_id uuid NOT NULL CONSTRAINT document_logo_rasters_logo_id_fkey REFERENCES app.document_logos(id),
    paper_width integer NOT NULL CONSTRAINT document_logo_rasters_paper_width_check CHECK (paper_width IN (58, 80)),
    width_percent integer NOT NULL CONSTRAINT document_logo_rasters_width_percent_check
        CHECK (width_percent BETWEEN 25 AND 100),
    align text NOT NULL CONSTRAINT document_logo_rasters_align_check CHECK (align IN ('left', 'center', 'right')),
    method text NOT NULL CONSTRAINT document_logo_rasters_method_check CHECK (method IN ('threshold', 'dither')),
    threshold integer NOT NULL CONSTRAINT document_logo_rasters_threshold_check
        CHECK (threshold BETWEEN 1 AND 255 AND (method = 'threshold' OR threshold = 128)),
    converter_version integer NOT NULL CONSTRAINT document_logo_rasters_converter_version_check
        CHECK (converter_version >= 1),
    width_dots integer NOT NULL CONSTRAINT document_logo_rasters_width_dots_check
        CHECK ((paper_width = 58 AND width_dots = 384) OR (paper_width = 80 AND width_dots = 576)),
    height_dots integer NOT NULL CONSTRAINT document_logo_rasters_height_dots_check CHECK (height_dots BETWEEN 1 AND 320),
    bits bytea NOT NULL CONSTRAINT document_logo_rasters_bits_check
        CHECK (octet_length(bits) = (width_dots / 8) * height_dots),
    created_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT document_logo_rasters_presentation_key
        UNIQUE (logo_id, paper_width, width_percent, align, method, threshold, converter_version),
    -- Destino de las llaves compuestas de la identidad: la conversión vigente es del logotipo vigente.
    CONSTRAINT document_logo_rasters_logo_key UNIQUE (id, logo_id)
);

CREATE FUNCTION app.document_logos_reject_mutation() RETURNS trigger
    LANGUAGE plpgsql AS $$
BEGIN
    RAISE EXCEPTION 'Los registros de % son inmutables', TG_TABLE_NAME USING ERRCODE = '55000';
END;
$$;
CREATE TRIGGER document_logos_insert_only
    BEFORE UPDATE OR DELETE ON app.document_logos
    FOR EACH ROW EXECUTE FUNCTION app.document_logos_reject_mutation();
CREATE TRIGGER document_logos_no_truncate
    BEFORE TRUNCATE ON app.document_logos
    FOR EACH STATEMENT EXECUTE FUNCTION app.document_logos_reject_mutation();
CREATE TRIGGER document_logo_rasters_insert_only
    BEFORE UPDATE OR DELETE ON app.document_logo_rasters
    FOR EACH ROW EXECUTE FUNCTION app.document_logos_reject_mutation();
CREATE TRIGGER document_logo_rasters_no_truncate
    BEFORE TRUNCATE ON app.document_logo_rasters
    FOR EACH STATEMENT EXECUTE FUNCTION app.document_logos_reject_mutation();

-- 3. Identidad documental (fila única con revisión optimista). Textos de hasta 300 caracteres y 8
-- líneas, sin caracteres de control salvo el salto de línea (Go además unifica saltos, convierte
-- tabuladores y quita invisibles antes de guardar, y el renderizador vuelve a sanear al imprimir).
-- logo_id es el logotipo vigente (NULL = sin logotipo: el bloque «Logotipo» no imprime nada);
-- las columnas logo_width_percent, logo_align, logo_method y logo_threshold son su presentación en
-- los documentos y se conservan al retirarlo. logo_raster_58_id y logo_raster_80_id son las
-- conversiones vigentes del logotipo con esa presentación, que Go genera (o reutiliza) en la misma
-- transacción que cambia el logotipo o la presentación: o existen las tres referencias o ninguna, y
-- cada conversión pertenece al logotipo vigente (llave compuesta). Los documentos copian estos
-- identificadores al emitirse.
CREATE TABLE app.document_identity (
    singleton boolean NOT NULL DEFAULT true CONSTRAINT document_identity_pkey PRIMARY KEY
        CONSTRAINT document_identity_singleton_check CHECK (singleton),
    additional_info text NOT NULL DEFAULT '' CONSTRAINT document_identity_additional_info_check CHECK (
        char_length(additional_info) <= 300
        AND length(additional_info) - length(replace(additional_info, E'\n', '')) <= 7
        AND additional_info !~ '[\x01-\x09\x0B-\x1F\x7F-\x9F]'
    ),
    header_text text NOT NULL DEFAULT '' CONSTRAINT document_identity_header_text_check CHECK (
        char_length(header_text) <= 300
        AND length(header_text) - length(replace(header_text, E'\n', '')) <= 7
        AND header_text !~ '[\x01-\x09\x0B-\x1F\x7F-\x9F]'
    ),
    footer_text text NOT NULL DEFAULT '' CONSTRAINT document_identity_footer_text_check CHECK (
        char_length(footer_text) <= 300
        AND length(footer_text) - length(replace(footer_text, E'\n', '')) <= 7
        AND footer_text !~ '[\x01-\x09\x0B-\x1F\x7F-\x9F]'
    ),
    logo_id uuid CONSTRAINT document_identity_logo_id_fkey REFERENCES app.document_logos(id),
    logo_width_percent integer NOT NULL DEFAULT 60 CONSTRAINT document_identity_logo_width_percent_check
        CHECK (logo_width_percent BETWEEN 25 AND 100),
    logo_align text NOT NULL DEFAULT 'center' CONSTRAINT document_identity_logo_align_check
        CHECK (logo_align IN ('left', 'center', 'right')),
    logo_method text NOT NULL DEFAULT 'threshold' CONSTRAINT document_identity_logo_method_check
        CHECK (logo_method IN ('threshold', 'dither')),
    logo_threshold integer NOT NULL DEFAULT 128 CONSTRAINT document_identity_logo_threshold_check
        CHECK (logo_threshold BETWEEN 1 AND 255),
    logo_raster_58_id uuid,
    logo_raster_80_id uuid,
    revision integer NOT NULL DEFAULT 1 CONSTRAINT document_identity_revision_check CHECK (revision > 0),
    updated_by uuid CONSTRAINT document_identity_updated_by_fkey REFERENCES app.users(id),
    updated_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT document_identity_logo_rasters_check CHECK (
        (logo_id IS NULL AND logo_raster_58_id IS NULL AND logo_raster_80_id IS NULL)
        OR (logo_id IS NOT NULL AND logo_raster_58_id IS NOT NULL AND logo_raster_80_id IS NOT NULL)
    ),
    CONSTRAINT document_identity_logo_raster_58_fkey FOREIGN KEY (logo_raster_58_id, logo_id)
        REFERENCES app.document_logo_rasters (id, logo_id),
    CONSTRAINT document_identity_logo_raster_80_fkey FOREIGN KEY (logo_raster_80_id, logo_id)
        REFERENCES app.document_logo_rasters (id, logo_id)
);

-- La fila única no se borra: sin ella los documentos perderían su información adicional, su
-- encabezado y su pie. Se cambia solo con UPDATE desde Go (documents/identity).
CREATE FUNCTION app.document_identity_reject_delete() RETURNS trigger
    LANGUAGE plpgsql AS $$
BEGIN
    RAISE EXCEPTION 'La identidad documental no se puede eliminar' USING ERRCODE = '55000';
END;
$$;
CREATE TRIGGER document_identity_no_delete
    BEFORE DELETE ON app.document_identity
    FOR EACH ROW EXECUTE FUNCTION app.document_identity_reject_delete();
CREATE TRIGGER document_identity_no_truncate
    BEFORE TRUNCATE ON app.document_identity
    FOR EACH STATEMENT EXECUTE FUNCTION app.document_identity_reject_delete();

INSERT INTO app.document_identity (singleton) VALUES (true);
