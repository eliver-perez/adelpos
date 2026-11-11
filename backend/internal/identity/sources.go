package identity

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"

	"local/ad-el-pos/server/internal/documents"
)

// ReportLogo es la fuente del logotipo del restaurante para el encabezado de los reportes PDF y
// Excel (reports.LogoSource). Corre en la transacción REPEATABLE READ READ ONLY del reporte: un solo
// SELECT, sin FOR UPDATE ni FOR SHARE, sin locks consultivos ni escrituras. Devuelve el original
// vigente tal como se validó al cargarlo, o nil si no hay logotipo (nunca cargado o retirado).
func ReportLogo(ctx context.Context, tx pgx.Tx) ([]byte, error) {
	var content []byte
	err := tx.QueryRow(ctx, `SELECT l.content
        FROM app.document_identity i
        JOIN app.document_logos l ON l.id = i.logo_id
        WHERE i.singleton`).Scan(&content)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	return content, err
}

// Rasters carga las conversiones guardadas del logotipo por su identificador. Implementa
// documents.RasterSource para el trabajador de impresión (con el pool) y para la vista previa del
// diseñador (con la transacción de la petición). Las conversiones son inmutables y nunca se borran:
// un documento emitido con un logotipo que después se reemplazó o retiró se imprime con el suyo.
type Rasters struct {
	db querier
}

// NewRasters crea la fuente sobre un pool o una transacción.
func NewRasters(db querier) *Rasters {
	return &Rasters{db: db}
}

// Raster devuelve los bits de una conversión o nil sin error si el identificador no es válido o ya
// no existe: el transporte omite la imagen con aviso y el documento se imprime igual.
func (source *Rasters) Raster(ctx context.Context, id string) (*documents.Raster, error) {
	if !identifierPattern.MatchString(id) {
		return nil, nil
	}
	row, err := readRaster(ctx, source.db, id)
	if err != nil || row == nil {
		return nil, err
	}
	return &documents.Raster{WidthDots: row.WidthDots, HeightDots: row.HeightDots, Bits: row.Bits}, nil
}

// DataURI devuelve la vista previa PNG de una conversión ("" si no existe), para mostrar las
// líneas de imagen de la IR en la vista previa del diseñador.
func (source *Rasters) DataURI(ctx context.Context, id string) (string, error) {
	if !identifierPattern.MatchString(id) {
		return "", nil
	}
	row, err := readRaster(ctx, source.db, id)
	if err != nil || row == nil {
		return "", err
	}
	content, err := rasterPNG(row.WidthDots, row.HeightDots, row.Bits)
	if err != nil {
		return "", err
	}
	return pngDataURI(content), nil
}
