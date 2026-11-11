package identity

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"

	"local/ad-el-pos/server/internal/logoraster"
)

// Acceso a las tablas de la 0019. Solo esta parte escribe en ellas; los documentos leen la copia de
// la identidad (documents.LoadIdentity) y los rasters por identificador (Rasters).

// querier es lo que comparten pgx.Tx y *pgxpool.Pool para las lecturas de una fila.
type querier interface {
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

// identityRow es la fila única de app.document_identity.
type identityRow struct {
	AdditionalInfo string
	HeaderText     string
	FooterText     string
	LogoID         *string
	Raster58ID     *string
	Raster80ID     *string
	Presentation   Presentation
	Revision       int
	UpdatedBy      *Person
	UpdatedAt      time.Time
}

// readIdentity lee la identidad. Con forUpdate toma el lock de fila que serializa las escrituras.
func readIdentity(ctx context.Context, db querier, forUpdate bool) (*identityRow, error) {
	query := `SELECT i.additional_info, i.header_text, i.footer_text, i.logo_id, i.logo_raster_58_id,
            i.logo_raster_80_id, i.logo_width_percent, i.logo_align, i.logo_method, i.logo_threshold,
            i.revision, i.updated_by, coalesce(u.display_name, ''), i.updated_at
        FROM app.document_identity i
        LEFT JOIN app.users u ON u.id = i.updated_by
        WHERE i.singleton`
	if forUpdate {
		query += " FOR UPDATE OF i"
	}
	row := &identityRow{}
	var updatedByID *string
	var updatedByName string
	err := db.QueryRow(ctx, query).Scan(
		&row.AdditionalInfo,
		&row.HeaderText,
		&row.FooterText,
		&row.LogoID,
		&row.Raster58ID,
		&row.Raster80ID,
		&row.Presentation.WidthPercent,
		&row.Presentation.Align,
		&row.Presentation.Method,
		&row.Presentation.Threshold,
		&row.Revision,
		&updatedByID,
		&updatedByName,
		&row.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	if updatedByID != nil {
		row.UpdatedBy = &Person{ID: *updatedByID, Name: updatedByName}
	}
	return row, nil
}

// lockIdentity toma la fila de la identidad para escribir. La 0019 la siembra y un disparador
// impide borrarla; si aun así faltara, se vuelve a crear con los valores por omisión.
func lockIdentity(ctx context.Context, tx pgx.Tx) (*identityRow, error) {
	_, err := tx.Exec(ctx, "INSERT INTO app.document_identity (singleton) VALUES (true) ON CONFLICT DO NOTHING")
	if err != nil {
		return nil, err
	}
	return readIdentity(ctx, tx, true)
}

// saveIdentity guarda la fila completa con la revisión siguiente y quién la cambió.
func saveIdentity(ctx context.Context, tx pgx.Tx, row *identityRow, actorID string) error {
	_, err := tx.Exec(ctx, `UPDATE app.document_identity
        SET additional_info = $1, header_text = $2, footer_text = $3, logo_id = $4,
            logo_raster_58_id = $5, logo_raster_80_id = $6, logo_width_percent = $7, logo_align = $8,
            logo_method = $9, logo_threshold = $10, revision = revision + 1, updated_by = $11,
            updated_at = now()
        WHERE singleton`,
		row.AdditionalInfo,
		row.HeaderText,
		row.FooterText,
		row.LogoID,
		row.Raster58ID,
		row.Raster80ID,
		row.Presentation.WidthPercent,
		row.Presentation.Align,
		row.Presentation.Method,
		row.Presentation.Threshold,
		actorID,
	)
	return err
}

// readEstablishment lee los datos de Configuración general que la identidad reutiliza. Antes del
// asistente inicial no hay establecimiento: los datos quedan vacíos.
func readEstablishment(ctx context.Context, db querier) (Establishment, error) {
	establishment := Establishment{}
	err := db.QueryRow(ctx, "SELECT name, address, phone, email FROM app.establishment").Scan(
		&establishment.Name,
		&establishment.Address,
		&establishment.Phone,
		&establishment.Email,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return Establishment{}, nil
	}
	return establishment, err
}

// logoRow es un logotipo guardado (sin su contenido).
type logoRow struct {
	ID         string
	MediaType  string
	Width      int
	Height     int
	SizeBytes  int
	SHA256     string
	UploadedAt time.Time
	UploadedBy *Person
}

// readLogo lee los datos de un logotipo y, si withContent, también su original.
func readLogo(ctx context.Context, db querier, id string, withContent bool) (*logoRow, []byte, error) {
	contentColumn := "NULL::bytea"
	if withContent {
		contentColumn = "l.content"
	}
	row := &logoRow{}
	var uploadedByID, uploadedByName string
	var content []byte
	err := db.QueryRow(ctx, `SELECT l.id, l.media_type, l.width, l.height, l.size_bytes, l.sha256, l.created_at,
            l.created_by, coalesce(u.display_name, ''), `+contentColumn+`
        FROM app.document_logos l
        LEFT JOIN app.users u ON u.id = l.created_by
        WHERE l.id = $1`, id).Scan(
		&row.ID,
		&row.MediaType,
		&row.Width,
		&row.Height,
		&row.SizeBytes,
		&row.SHA256,
		&row.UploadedAt,
		&uploadedByID,
		&uploadedByName,
		&content,
	)
	if err != nil {
		return nil, nil, err
	}
	row.UploadedBy = &Person{ID: uploadedByID, Name: uploadedByName}
	return row, content, nil
}

// storeLogo guarda un original validado y devuelve su identificador. El mismo archivo (mismo
// SHA-256) se guarda una sola vez: cargarlo otra vez reutiliza el registro y sus conversiones.
func storeLogo(ctx context.Context, tx pgx.Tx, logo *logoraster.Logo, content []byte, actorID string) (string, bool, error) {
	var id string
	err := tx.QueryRow(ctx, `INSERT INTO app.document_logos
            (id, content, media_type, width, height, size_bytes, sha256, created_by)
        VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
        ON CONFLICT (sha256) DO NOTHING
        RETURNING id`,
		newID(),
		content,
		logo.MediaType,
		logo.Width,
		logo.Height,
		logo.Size,
		logo.SHA256,
		actorID,
	).Scan(&id)
	if err == nil {
		return id, true, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return "", false, err
	}
	err = tx.QueryRow(ctx, "SELECT id FROM app.document_logos WHERE sha256 = $1", logo.SHA256).Scan(&id)
	return id, false, err
}

// rasterRow es una conversión guardada del logotipo.
type rasterRow struct {
	ID         string
	PaperWidth int
	WidthDots  int
	HeightDots int
	Bits       []byte
}

// findRaster busca la conversión de un logotipo para un ancho de papel y una presentación.
func findRaster(ctx context.Context, db querier, logoID string, paperWidth int, presentation Presentation) (*rasterRow, error) {
	row := &rasterRow{}
	err := db.QueryRow(ctx, `SELECT id, paper_width, width_dots, height_dots, bits
        FROM app.document_logo_rasters
        WHERE logo_id = $1 AND paper_width = $2 AND width_percent = $3 AND align = $4 AND method = $5
            AND threshold = $6 AND converter_version = $7`,
		logoID,
		paperWidth,
		presentation.WidthPercent,
		presentation.Align,
		presentation.Method,
		presentation.rasterThreshold(),
		ConverterVersion,
	).Scan(&row.ID, &row.PaperWidth, &row.WidthDots, &row.HeightDots, &row.Bits)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return row, nil
}

// readRaster lee una conversión por su identificador (nil si no existe).
func readRaster(ctx context.Context, db querier, id string) (*rasterRow, error) {
	row := &rasterRow{}
	err := db.QueryRow(ctx, `SELECT id, paper_width, width_dots, height_dots, bits
        FROM app.document_logo_rasters WHERE id = $1`, id).Scan(
		&row.ID,
		&row.PaperWidth,
		&row.WidthDots,
		&row.HeightDots,
		&row.Bits,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return row, nil
}

// storeRaster guarda una conversión nueva. La combinación es única: si otra transacción la guardó
// antes (o ya existía), se usa la guardada.
func storeRaster(ctx context.Context, tx pgx.Tx, logoID string, paperWidth int, presentation Presentation, raster *logoraster.Raster) (*rasterRow, error) {
	_, err := tx.Exec(ctx, `INSERT INTO app.document_logo_rasters
            (id, logo_id, paper_width, width_percent, align, method, threshold, converter_version,
             width_dots, height_dots, bits)
        VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
        ON CONFLICT (logo_id, paper_width, width_percent, align, method, threshold, converter_version) DO NOTHING`,
		newID(),
		logoID,
		paperWidth,
		presentation.WidthPercent,
		presentation.Align,
		presentation.Method,
		presentation.rasterThreshold(),
		ConverterVersion,
		raster.Width(),
		raster.Height(),
		raster.Bits(),
	)
	if err != nil {
		return nil, err
	}
	stored, err := findRaster(ctx, tx, logoID, paperWidth, presentation)
	if err == nil && stored == nil {
		err = errors.New("la conversión del logotipo no quedó guardada")
	}
	return stored, err
}
