package documents

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
)

// Acceso a las plantillas guardadas (migración 0018). Las versiones son inmutables (un disparador
// impide UPDATE y DELETE); cada tipo tiene exactamente una plantilla activa (índice único parcial)
// y diez plantillas del sistema con identificadores fijos, cuya versión 1 es Default(kind).

// systemSuffix numera los tipos en los identificadores fijos de la semilla.
var systemSuffix = map[Kind]string{
	KindReceipt:             "01",
	KindPrecheck:            "02",
	KindKitchenOrder:        "03",
	KindKitchenModification: "04",
	KindKitchenCancellation: "05",
	KindCashIncome:          "06",
	KindCashWithdrawal:      "07",
	KindCashExpense:         "08",
	KindCashPartialCut:      "09",
	KindCashFinalCut:        "10",
}

// SystemTemplateID es el identificador fijo de la plantilla del sistema de un tipo.
func SystemTemplateID(kind Kind) string {
	return fmt.Sprintf("9b0d0c00-0000-4000-8000-0000000000%s", systemSuffix[kind])
}

// SystemVersionID es el identificador fijo de la versión 1 de la plantilla del sistema de un tipo.
// Un documento anterior a la 9B (sin versión guardada) se renderiza con esta versión.
func SystemVersionID(kind Kind) string {
	return fmt.Sprintf("9b0d0c01-0000-4000-8000-0000000000%s", systemSuffix[kind])
}

// Version es una versión de plantilla lista para renderizar.
type Version struct {
	ID         string
	TemplateID string
	Kind       Kind
	Number     int
	Template   *Template
}

// systemVersion es la versión 1 del sistema, armada en Go (idéntica a la sembrada).
func systemVersion(kind Kind) *Version {
	return &Version{ID: SystemVersionID(kind), TemplateID: SystemTemplateID(kind), Kind: kind, Number: 1, Template: Default(kind)}
}

// ActiveVersion devuelve la versión vigente de la plantilla activa del tipo. Si no hay plantilla
// activa o su contenido no es válido para el catálogo actual, imprime con la versión del sistema:
// la impresión nunca falla por una plantilla.
func ActiveVersion(ctx context.Context, tx pgx.Tx, kind Kind) (*Version, error) {
	var id, templateID string
	var number int
	var layout []byte
	err := tx.QueryRow(ctx, `SELECT v.id,v.template_id,v.number,v.layout FROM app.document_templates t
        JOIN app.document_template_versions v ON v.id=t.current_version_id
        WHERE t.kind=$1 AND t.active`, string(kind)).Scan(&id, &templateID, &number, &layout)
	if errors.Is(err, pgx.ErrNoRows) {
		return systemVersion(kind), nil
	}
	if err != nil {
		return nil, err
	}
	template, err := ParseTemplate(kind, layout)
	if err != nil {
		return systemVersion(kind), nil
	}
	return &Version{ID: id, TemplateID: templateID, Kind: kind, Number: number, Template: template}, nil
}

// VersionByID devuelve una versión guardada. Un identificador vacío (documento anterior a la 9B)
// o el de la versión 1 del sistema usan la plantilla predeterminada de Go.
func VersionByID(ctx context.Context, tx pgx.Tx, kind Kind, id *string) (*Version, error) {
	if id == nil || *id == "" || *id == SystemVersionID(kind) {
		return systemVersion(kind), nil
	}
	var templateID, storedKind string
	var number int
	var layout []byte
	err := tx.QueryRow(ctx, `SELECT v.template_id,t.kind,v.number,v.layout FROM app.document_template_versions v
        JOIN app.document_templates t ON t.id=v.template_id WHERE v.id=$1`, *id).Scan(&templateID, &storedKind, &number, &layout)
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && Kind(storedKind) != kind) {
		return systemVersion(kind), nil
	}
	if err != nil {
		return nil, err
	}
	template, err := ParseTemplate(kind, layout)
	if err != nil {
		return systemVersion(kind), nil
	}
	return &Version{ID: *id, TemplateID: templateID, Kind: kind, Number: number, Template: template}, nil
}

// LoadIdentity lee la identidad vigente del establecimiento para copiarla en un documento: el nombre,
// la dirección, el teléfono y el correo de app.establishment (Configuración general) y, de la
// identidad documental (0019, fase B), la información adicional, el encabezado, el pie y las
// conversiones vigentes del logotipo para 58 y 80 mm (sin logotipo, Logo queda vacío y el bloque no
// imprime nada). El documento guarda esta copia: su reimpresión es idéntica aunque la identidad, el
// logotipo o su presentación cambien después (las conversiones son inmutables).
func LoadIdentity(ctx context.Context, tx pgx.Tx) (Identity, error) {
	var identity Identity
	err := tx.QueryRow(ctx, `SELECT name,coalesce(address,''),coalesce(phone,''),coalesce(email,'')
        FROM app.establishment LIMIT 1`).Scan(&identity.Name, &identity.Address, &identity.Phone, &identity.Email)
	if errors.Is(err, pgx.ErrNoRows) {
		return Identity{}, nil
	}
	if err != nil {
		return Identity{}, err
	}
	return identity, loadDocumentIdentity(ctx, tx, &identity)
}

// loadDocumentIdentity completa la identidad con los textos y las conversiones vigentes del logotipo
// (internal/identity las genera al cargar el logotipo o cambiar su presentación, nunca al imprimir).
func loadDocumentIdentity(ctx context.Context, tx pgx.Tx, identity *Identity) error {
	var raster58, raster80 *string
	var width58, height58, width80, height80 *int
	err := tx.QueryRow(ctx, `SELECT i.additional_info, i.header_text, i.footer_text,
            r58.id, r58.width_dots, r58.height_dots, r80.id, r80.width_dots, r80.height_dots
        FROM app.document_identity i
        LEFT JOIN app.document_logo_rasters r58 ON r58.id = i.logo_raster_58_id
        LEFT JOIN app.document_logo_rasters r80 ON r80.id = i.logo_raster_80_id
        WHERE i.singleton`).Scan(
		&identity.Extra,
		&identity.Header,
		&identity.Footer,
		&raster58,
		&width58,
		&height58,
		&raster80,
		&width80,
		&height80,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	if raster58 != nil && width58 != nil && height58 != nil {
		identity.Logo = append(identity.Logo, LogoRaster{PaperWidth: 58, RasterID: *raster58, WidthDots: *width58, HeightDots: *height58})
	}
	if raster80 != nil && width80 != nil && height80 != nil {
		identity.Logo = append(identity.Logo, LogoRaster{PaperWidth: 80, RasterID: *raster80, WidthDots: *width80, HeightDots: *height80})
	}
	return nil
}
