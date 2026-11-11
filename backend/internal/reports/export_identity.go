package reports

import (
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"image/jpeg"
	"image/png"
	"strings"
	"sync"

	"github.com/jackc/pgx/v5"
	"local/ad-el-pos/server/internal/brand"
	"local/ad-el-pos/server/internal/logoraster"
)

// Identidad del establecimiento en el encabezado de las exportaciones (etapa 9B, §0-B «Logotipo en
// reportes PDF y Excel»). La primera página del PDF y la hoja «Filtros» del Excel muestran el
// logotipo del restaurante junto con su nombre, su dirección, su teléfono y su correo (solo los que
// existan, de app.establishment); si el restaurante no tiene logotipo propio se usa el de AD-EL POS
// (internal/brand) con el nombre del establecimiento. El contenido, las columnas, los totales, el
// orden, las consultas, los límites y la auditoría de los reportes no cambian.
//
// La identidad se lee en la misma transacción REPEATABLE READ READ ONLY del reporte (la misma
// fotografía, sin locks). El logotipo del restaurante llega por una LogoSource que la fase B2 de la
// 9B conecta con su tabla; mientras tanto la fuente por omisión dice «sin logotipo del restaurante».
// La imagen se prepara fuera de la transacción: se valida otra vez con logoraster.Validate, se
// compone sobre blanco, se reduce a lo más 600 × 168 píxeles (≈ 50 × 14 mm a 300 ppp) y se vuelve a
// codificar sin metadatos, para que el archivo pese poco y no lleve fragmentos ajenos al dibujo.

// LogoSource lee el logotipo vigente del restaurante para el encabezado de los archivos exportados.
//
// Contrato para quien la implemente (fase B2):
//   - recibe la transacción REPEATABLE READ READ ONLY del reporte: solo SELECT, sin FOR UPDATE ni
//     FOR SHARE, sin locks consultivos, sin escrituras y sin llamar handlers de otros módulos;
//   - devuelve los bytes del original tal como se validaron al cargarlo (PNG o JPEG de hasta 1 MiB),
//     o nil si no hay logotipo vigente (nunca cargado o retirado);
//   - un error de la consulta se devuelve tal cual y hace fallar la exportación como cualquier otro
//     error de base de datos; un contenido que ya no pase la validación no la hace fallar: el
//     encabezado usa entonces el logotipo de AD-EL POS.
type LogoSource func(ctx context.Context, tx pgx.Tx) ([]byte, error)

// NoRestaurantLogo es la fuente por omisión: el restaurante no tiene logotipo propio y los archivos
// llevan el de AD-EL POS.
func NoRestaurantLogo(context.Context, pgx.Tx) ([]byte, error) {
	return nil, nil
}

const (
	// exportLogoMaxWidth y exportLogoMaxHeight acotan la imagen del encabezado en píxeles; un
	// logotipo más pequeño conserva su tamaño (nunca se amplía).
	exportLogoMaxWidth    = 600
	exportLogoMaxHeight   = 168
	exportLogoJPEGQuality = 90
)

// exportLogo es la imagen del encabezado ya preparada para fpdf y excelize.
type exportLogo struct {
	content []byte
	// format es logoraster.FormatPNG o logoraster.FormatJPEG.
	format string
	width  int
	height int
	// restaurant indica si es el logotipo del restaurante (falso: el de AD-EL POS).
	restaurant bool
}

// fit devuelve las medidas que ocupa la imagen dentro de maxWidth × maxHeight conservando su
// proporción (en las unidades de los límites: milímetros en el PDF, píxeles en el Excel).
func (logo *exportLogo) fit(maxWidth, maxHeight float64) (float64, float64) {
	width := maxHeight * float64(logo.width) / float64(logo.height)
	if width <= maxWidth {
		return width, maxHeight
	}
	return maxWidth, maxWidth * float64(logo.height) / float64(logo.width)
}

// exportIdentity es lo que se lee en la fotografía del reporte para el encabezado.
type exportIdentity struct {
	name           string
	address        string
	phone          string
	email          string
	restaurantLogo []byte
}

// readExportIdentity lee nombre, dirección, teléfono y correo de app.establishment y el logotipo
// del restaurante de source, en la transacción de solo lectura del reporte.
func readExportIdentity(ctx context.Context, tx pgx.Tx, source LogoSource) (exportIdentity, error) {
	identity := exportIdentity{}
	err := tx.QueryRow(ctx, "SELECT name, address, phone, email FROM app.establishment").
		Scan(&identity.name, &identity.address, &identity.phone, &identity.email)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return identity, err
	}
	if source == nil {
		source = NoRestaurantLogo
	}
	identity.restaurantLogo, err = source(ctx, tx)
	return identity, err
}

// applyIdentity copia la identidad leída al documento y elige la imagen del encabezado.
func (document *exportDocument) applyIdentity(identity exportIdentity) {
	document.Establishment = identity.name
	document.Address = strings.TrimSpace(identity.address)
	document.Phone = strings.TrimSpace(identity.phone)
	document.Email = strings.TrimSpace(identity.email)
	document.Logo = headerLogo(identity.restaurantLogo)
}

// contactLine une teléfono y correo en una línea («Tel. 81 1234 5678 · hola@ejemplo.mx»), solo con
// los que existan.
func (document *exportDocument) contactLine() string {
	parts := []string{}
	if document.Phone != "" {
		parts = append(parts, "Tel. "+document.Phone)
	}
	if document.Email != "" {
		parts = append(parts, document.Email)
	}
	return strings.Join(parts, " · ")
}

// headerLogo es el logotipo del restaurante si existe y es válido; si no, el de AD-EL POS. Solo
// devuelve nil si la marca embebida no se pudiera preparar (la prueba de brand lo impide).
func headerLogo(restaurantContent []byte) *exportLogo {
	if len(restaurantContent) > 0 {
		logo, err := restaurantHeaderLogo(restaurantContent)
		if err == nil {
			return logo
		}
		// El logotipo se validó al cargarlo; si el guardado ya no pasa la validación, la
		// exportación no falla: el encabezado usa la marca de AD-EL POS.
	}
	logo, err := brandHeaderLogo()
	if err != nil {
		return nil
	}
	return logo
}

// La marca se prepara una sola vez por proceso.
var (
	brandLogoOnce  sync.Once
	brandLogo      *exportLogo
	brandLogoError error
)

func brandHeaderLogo() (*exportLogo, error) {
	brandLogoOnce.Do(func() {
		brandLogo, brandLogoError = prepareExportLogo(brand.LightLogoPNG(), false)
	})
	return brandLogo, brandLogoError
}

// restaurantLogoCache conserva la última preparación del logotipo del restaurante por su SHA-256,
// para no decodificarlo y escalarlo en cada exportación mientras no cambie.
var restaurantLogoCache struct {
	sync.Mutex
	digest [sha256.Size]byte
	logo   *exportLogo
}

func restaurantHeaderLogo(content []byte) (*exportLogo, error) {
	digest := sha256.Sum256(content)
	restaurantLogoCache.Lock()
	defer restaurantLogoCache.Unlock()
	if restaurantLogoCache.logo != nil && restaurantLogoCache.digest == digest {
		return restaurantLogoCache.logo, nil
	}
	logo, err := prepareExportLogo(content, true)
	if err != nil {
		return nil, err
	}
	restaurantLogoCache.digest = digest
	restaurantLogoCache.logo = logo
	return logo, nil
}

// prepareExportLogo valida el original con el mismo criterio que la carga, lo reduce (sin ampliarlo)
// a lo más exportLogoMaxWidth × exportLogoMaxHeight píxeles compuesto sobre blanco y lo vuelve a
// codificar: PNG RGB de 8 bits sin entrelazar si el original es PNG y JPEG de calidad 90 si es
// JPEG. El resultado solo contiene los fragmentos de la imagen (sin texto, perfiles ni metadatos
// del original) y es opaco, así que fpdf no necesita máscara de transparencia.
func prepareExportLogo(content []byte, restaurant bool) (*exportLogo, error) {
	original, err := logoraster.Validate(content)
	if err != nil {
		return nil, err
	}
	width, height := fitPixels(original.Width, original.Height, exportLogoMaxWidth, exportLogoMaxHeight)
	flattened := logoraster.Flatten(original.Image, width, height)
	var buffer bytes.Buffer
	if original.Format == logoraster.FormatJPEG {
		err = jpeg.Encode(&buffer, flattened, &jpeg.Options{Quality: exportLogoJPEGQuality})
	} else {
		encoder := png.Encoder{CompressionLevel: png.BestCompression}
		err = encoder.Encode(&buffer, flattened)
	}
	if err != nil {
		return nil, err
	}
	logo := &exportLogo{
		content:    buffer.Bytes(),
		format:     original.Format,
		width:      width,
		height:     height,
		restaurant: restaurant,
	}
	return logo, nil
}

// fitPixels reduce width × height para que quepa en maxWidth × maxHeight conservando la proporción
// (redondeo al entero más cercano, mínimo un píxel); si ya cabe, no cambia.
func fitPixels(width, height, maxWidth, maxHeight int) (int, int) {
	if width <= maxWidth && height <= maxHeight {
		return width, height
	}
	// La medida que más excede su límite manda: width/maxWidth contra height/maxHeight.
	if width*maxHeight >= height*maxWidth {
		return maxWidth, max(1, (2*height*maxWidth+width)/(2*width))
	}
	return max(1, (2*width*maxHeight+height)/(2*height)), maxHeight
}
