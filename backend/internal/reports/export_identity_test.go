package reports

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/binary"
	"hash/crc32"
	"image"
	"image/color"
	"image/gif"
	"image/jpeg"
	"image/png"
	"io"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"testing"

	"local/ad-el-pos/server/internal/logoraster"
)

// Pruebas del encabezado de identidad de las exportaciones (etapa 9B): preparación de la imagen
// (reducida, opaca, sin metadatos, PNG o JPEG), logotipo del restaurante con nombre, dirección,
// teléfono y correo en el PDF y en la hoja «Filtros», respaldo con la marca de AD-EL POS, y el
// resto del contenido sin cambios (mismas cadenas del PDF y misma hoja «Datos»).

const (
	testAddress = "Av. Constitución 123, Centro, Monterrey, N.L."
	testPhone   = "81 1234 5678"
	testEmail   = "hola@elpenon.mx"
)

// testBrandLogo es la marca de AD-EL POS preparada para el encabezado.
func testBrandLogo(t *testing.T) *exportLogo {
	t.Helper()
	logo, err := brandHeaderLogo()
	if err != nil {
		t.Fatal(err)
	}
	return logo
}

// restaurantPicture es un logotipo de 120 × 40 con fondo transparente, un recuadro rojo y una franja
// azul semitransparente.
func restaurantPicture() *image.NRGBA {
	picture := image.NewNRGBA(image.Rect(0, 0, 120, 40))
	for y := 0; y < 40; y++ {
		for x := 0; x < 120; x++ {
			switch {
			case x >= 10 && x < 50 && y >= 5 && y < 35:
				picture.Set(x, y, color.NRGBA{R: 200, G: 30, B: 40, A: 255})
			case x >= 60 && x < 110 && y >= 15 && y < 25:
				picture.Set(x, y, color.NRGBA{R: 20, G: 60, B: 220, A: 160})
			}
		}
	}
	return picture
}

// withTextChunk inserta un fragmento tEXt (metadatos) justo después de IHDR.
func withTextChunk(content []byte, keyword, text string) []byte {
	data := append([]byte(keyword+"\x00"), []byte(text)...)
	var chunk bytes.Buffer
	_ = binary.Write(&chunk, binary.BigEndian, uint32(len(data)))
	chunk.WriteString("tEXt")
	chunk.Write(data)
	_ = binary.Write(&chunk, binary.BigEndian, crc32.ChecksumIEEE(append([]byte("tEXt"), data...)))
	// Firma (8) + IHDR (4 de longitud, 4 de tipo, 13 de datos y 4 de CRC).
	insertAt := 8 + 25
	result := append([]byte{}, content[:insertAt]...)
	result = append(result, chunk.Bytes()...)
	return append(result, content[insertAt:]...)
}

func restaurantPNG(t *testing.T) []byte {
	t.Helper()
	var buffer bytes.Buffer
	if err := png.Encode(&buffer, restaurantPicture()); err != nil {
		t.Fatal(err)
	}
	return withTextChunk(buffer.Bytes(), "Comment", "Metadatos que no deben llegar al archivo")
}

func restaurantJPEG(t *testing.T) []byte {
	t.Helper()
	var buffer bytes.Buffer
	if err := jpeg.Encode(&buffer, restaurantPicture(), &jpeg.Options{Quality: 95}); err != nil {
		t.Fatal(err)
	}
	return buffer.Bytes()
}

// pngChunkTypes lista los tipos de fragmento de un PNG en orden.
func pngChunkTypes(t *testing.T, content []byte) []string {
	t.Helper()
	types := []string{}
	for position := 8; position+8 <= len(content); {
		length := int(binary.BigEndian.Uint32(content[position:]))
		types = append(types, string(content[position+4:position+8]))
		position += 12 + length
	}
	return types
}

// identityDocument es el documento de prueba con la identidad completa y el logotipo indicado.
func identityDocument(t *testing.T, rows int, logo *exportLogo) *exportDocument {
	t.Helper()
	document := exportTestDocument(t, exportTestDefinition(), exportTestRows(rows), PermissionCostsRead)
	document.Address = testAddress
	document.Phone = testPhone
	document.Email = testEmail
	document.Logo = logo
	return document
}

// pdfImageSizes devuelve ancho y alto de cada imagen del PDF, y si alguna usa DCTDecode (JPEG).
func pdfImageSizes(content []byte) ([][2]int, bool) {
	pattern := regexp.MustCompile(`/Subtype /Image\s*/Width (\d+)\s*/Height (\d+)`)
	sizes := [][2]int{}
	for _, match := range pattern.FindAllSubmatch(content, -1) {
		width, _ := strconv.Atoi(string(match[1]))
		height, _ := strconv.Atoi(string(match[2]))
		sizes = append(sizes, [2]int{width, height})
	}
	return sizes, bytes.Contains(content, []byte("/Filter /DCTDecode"))
}

// pdfAllStrings son todas las cadenas de texto del PDF, en orden.
func pdfAllStrings(t *testing.T, content []byte) []string {
	t.Helper()
	return strings.Split(strings.Join(pdfPages(t, content), "\n"), "\n")
}

func renderPDFForTest(t *testing.T, document *exportDocument) []byte {
	t.Helper()
	content, _, err := renderPDF(context.Background(), document)
	if err != nil {
		t.Fatal(err)
	}
	return content
}

func renderXLSXForTest(t *testing.T, document *exportDocument) []byte {
	t.Helper()
	content, err := renderXLSX(context.Background(), document)
	if err != nil {
		t.Fatal(err)
	}
	return content
}

// zipPart devuelve el contenido descomprimido de una parte del libro.
func zipPart(t *testing.T, content []byte, name string) []byte {
	t.Helper()
	archive, err := zip.NewReader(bytes.NewReader(content), int64(len(content)))
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range archive.File {
		if entry.Name != name {
			continue
		}
		reader, err := entry.Open()
		if err != nil {
			t.Fatal(err)
		}
		data, err := io.ReadAll(reader)
		_ = reader.Close()
		if err != nil {
			t.Fatal(err)
		}
		return data
	}
	t.Fatalf("El libro no tiene la parte %s", name)
	return nil
}

func TestPrepareExportLogo(t *testing.T) {
	// La marca (2048 × 682, transparente) queda en 504 × 168 PNG RGB opaco, sin metadatos, con el
	// fondo en blanco.
	brandLogo := testBrandLogo(t)
	if brandLogo.restaurant || brandLogo.format != logoraster.FormatPNG || brandLogo.width != 504 || brandLogo.height != 168 {
		t.Fatalf("Marca preparada: %s %d × %d", brandLogo.format, brandLogo.width, brandLogo.height)
	}
	content := brandLogo.content
	if content[24] != 8 || content[25] != 2 || content[28] != 0 {
		t.Fatalf("La marca debe ser PNG RGB de 8 bits sin entrelazar: profundidad %d, tipo %d, entrelazado %d",
			content[24], content[25], content[28])
	}
	decoded, err := png.Decode(bytes.NewReader(content))
	if err != nil {
		t.Fatal(err)
	}
	if red, green, blue, alpha := decoded.At(0, 0).RGBA(); red != 0xFFFF || green != 0xFFFF || blue != 0xFFFF || alpha != 0xFFFF {
		t.Fatal("El fondo transparente de la marca debe quedar en blanco")
	}
	t.Logf("Marca en el encabezado: %d × %d píxeles, %d bytes", brandLogo.width, brandLogo.height, len(content))

	// Un PNG del restaurante que ya cabe conserva su tamaño y pierde sus metadatos.
	original := restaurantPNG(t)
	if !slices.Contains(pngChunkTypes(t, original), "tEXt") {
		t.Fatal("El PNG de prueba debe llevar metadatos")
	}
	logo, err := prepareExportLogo(original, true)
	if err != nil {
		t.Fatal(err)
	}
	if !logo.restaurant || logo.width != 120 || logo.height != 40 || logo.format != logoraster.FormatPNG {
		t.Fatalf("Logotipo del restaurante: %s %d × %d", logo.format, logo.width, logo.height)
	}
	if chunks := pngChunkTypes(t, logo.content); !slices.Equal(chunks, []string{"IHDR", "IDAT", "IEND"}) {
		t.Fatalf("El PNG preparado solo debe tener IHDR, IDAT e IEND: %v", chunks)
	}
	if bytes.Contains(logo.content, []byte("Metadatos")) {
		t.Fatal("Los metadatos del original llegaron al archivo")
	}
	// Uno más grande se reduce a lo más 600 × 168 sin deformarse.
	var wide bytes.Buffer
	if err := png.Encode(&wide, image.NewGray(image.Rect(0, 0, 2000, 100))); err != nil {
		t.Fatal(err)
	}
	reduced, err := prepareExportLogo(wide.Bytes(), true)
	if err != nil || reduced.width != 600 || reduced.height != 30 {
		t.Fatalf("2000 × 100 reducido: %+v %v", reduced, err)
	}
	// Un JPEG se conserva como JPEG.
	photo, err := prepareExportLogo(restaurantJPEG(t), true)
	if err != nil {
		t.Fatal(err)
	}
	_, format, err := image.DecodeConfig(bytes.NewReader(photo.content))
	if err != nil || format != "jpeg" || photo.format != logoraster.FormatJPEG {
		t.Fatalf("JPEG preparado: %s %v", format, err)
	}
	cases := [][6]int{
		{2048, 682, 600, 168, 504, 168},
		{2000, 100, 600, 168, 600, 30},
		{100, 2000, 600, 168, 8, 168},
		{120, 40, 600, 168, 120, 40},
		{600, 168, 600, 168, 600, 168},
		{5000, 1, 600, 168, 600, 1},
	}
	for _, testCase := range cases {
		width, height := fitPixels(testCase[0], testCase[1], testCase[2], testCase[3])
		if width != testCase[4] || height != testCase[5] {
			t.Fatalf("fitPixels(%d, %d): %d × %d", testCase[0], testCase[1], width, height)
		}
	}
}

// Un logotipo guardado que no pasa la validación no impide exportar: el encabezado usa la marca. Un
// logotipo válido se prepara una vez mientras no cambie.
func TestHeaderLogoChoice(t *testing.T) {
	brandLogo := testBrandLogo(t)
	if logo := headerLogo(nil); logo != brandLogo {
		t.Fatal("Sin logotipo del restaurante se usa la marca de AD-EL POS")
	}
	var animated bytes.Buffer
	if err := gif.Encode(&animated, image.NewPaletted(image.Rect(0, 0, 4, 4), color.Palette{color.White, color.Black}), nil); err != nil {
		t.Fatal(err)
	}
	original := restaurantPNG(t)
	for name, content := range map[string][]byte{"GIF": animated.Bytes(), "PNG truncado": original[:len(original)/2],
		"SVG": []byte(`<svg xmlns="http://www.w3.org/2000/svg"/>`)} {
		if logo := headerLogo(content); logo != brandLogo {
			t.Fatalf("%s: debe usarse la marca de AD-EL POS", name)
		}
	}
	first := headerLogo(original)
	second := headerLogo(original)
	if first == nil || !first.restaurant || first != second {
		t.Fatal("El logotipo del restaurante debe prepararse una vez y reutilizarse")
	}
	photo := headerLogo(restaurantJPEG(t))
	if photo == first || photo.format != logoraster.FormatJPEG {
		t.Fatal("Un logotipo distinto debe prepararse de nuevo")
	}
}

func TestContactLine(t *testing.T) {
	cases := []struct {
		phone, email, line string
	}{
		{testPhone, testEmail, "Tel. 81 1234 5678 · hola@elpenon.mx"},
		{testPhone, "", "Tel. 81 1234 5678"},
		{"", testEmail, "hola@elpenon.mx"},
		{"", "", ""},
	}
	for _, testCase := range cases {
		document := &exportDocument{Phone: testCase.phone, Email: testCase.email}
		if line := document.contactLine(); line != testCase.line {
			t.Fatalf("Contacto %q %q: %q", testCase.phone, testCase.email, line)
		}
	}
	document := &exportDocument{}
	document.applyIdentity(exportIdentity{name: "El Peñón", address: "  ", phone: " 81 1234 5678 ", email: ""})
	if document.Establishment != "El Peñón" || document.Address != "" || document.Phone != testPhone || document.Logo == nil ||
		document.Logo.restaurant {
		t.Fatalf("Identidad aplicada: %+v", document)
	}
}

// PDF con el logotipo del restaurante: una sola imagen (la del restaurante, 120 × 40), nombre,
// dirección y contacto en la primera página; las cadenas del reporte son las mismas que sin
// identidad, en el mismo orden; las páginas siguientes conservan su encabezado breve.
func TestPDFHeaderWithRestaurantLogo(t *testing.T) {
	logo, err := prepareExportLogo(restaurantPNG(t), true)
	if err != nil {
		t.Fatal(err)
	}
	document := identityDocument(t, 5, logo)
	content := renderPDFForTest(t, document)
	sizes, jpegFound := pdfImageSizes(content)
	if !slices.Equal(sizes, [][2]int{{120, 40}}) || jpegFound {
		t.Fatalf("Imágenes del PDF: %v (JPEG %v)", sizes, jpegFound)
	}
	pages := pdfPages(t, content)
	for _, expected := range []string{"Restaurante El Peñón", testAddress, "Tel. 81 1234 5678 · hola@elpenon.mx"} {
		if !strings.Contains(pages[0], expected) {
			t.Fatalf("La primera página no contiene «%s»", expected)
		}
	}
	// Mismo documento sin logotipo ni datos de contacto: las cadenas son las mismas salvo la
	// dirección y el contacto del encabezado.
	baseline := exportTestDocument(t, exportTestDefinition(), exportTestRows(5), PermissionCostsRead)
	baseline.Logo = nil
	withoutIdentity := pdfAllStrings(t, renderPDFForTest(t, baseline))
	withIdentity := slices.DeleteFunc(pdfAllStrings(t, content), func(text string) bool {
		return text == testAddress || text == "Tel. 81 1234 5678 · hola@elpenon.mx"
	})
	if !slices.Equal(withIdentity, withoutIdentity) {
		t.Fatalf("El encabezado cambió el contenido del reporte:\n%v\n%v", withIdentity, withoutIdentity)
	}
	if sizes, _ := pdfImageSizes(renderPDFForTest(t, baseline)); len(sizes) != 0 {
		t.Fatalf("Sin logotipo no hay imágenes: %v", sizes)
	}
	// Varias páginas: solo la primera lleva el encabezado de identidad.
	long := renderPDFForTest(t, identityDocument(t, 60, logo))
	longPages := pdfPages(t, long)
	if len(longPages) < 2 {
		t.Fatal("El documento largo debía ocupar varias páginas")
	}
	for number, page := range longPages[1:] {
		if strings.Contains(page, testAddress) || strings.Contains(page, testEmail) {
			t.Fatalf("La página %d repite el encabezado de identidad", number+2)
		}
		if !strings.Contains(page, "Restaurante El Peñón · Del 1 al 30 de noviembre de 2026") {
			t.Fatalf("La página %d perdió su encabezado breve", number+2)
		}
	}
	if sizes, _ := pdfImageSizes(long); len(sizes) != 1 {
		t.Fatalf("El logotipo se incrusta una sola vez: %v", sizes)
	}
	// Un JPEG viaja como DCTDecode.
	photo, err := prepareExportLogo(restaurantJPEG(t), true)
	if err != nil {
		t.Fatal(err)
	}
	if sizes, jpegFound := pdfImageSizes(renderPDFForTest(t, identityDocument(t, 5, photo))); len(sizes) != 1 || !jpegFound {
		t.Fatalf("Logotipo JPEG en el PDF: %v (JPEG %v)", sizes, jpegFound)
	}
}

// Sin logotipo propio el PDF lleva la marca de AD-EL POS (504 × 168) con el nombre del
// establecimiento; sin teléfono ni correo no aparece la línea de contacto.
func TestPDFHeaderFallsBackToBrandLogo(t *testing.T) {
	document := exportTestDocument(t, exportTestDefinition(), exportTestRows(5), PermissionCostsRead)
	content := renderPDFForTest(t, document)
	if sizes, _ := pdfImageSizes(content); !slices.Equal(sizes, [][2]int{{504, 168}}) {
		t.Fatalf("Imágenes del PDF con la marca: %v", sizes)
	}
	first := pdfPages(t, content)[0]
	if !strings.Contains(first, "Restaurante El Peñón") || strings.Contains(first, "Tel.") {
		t.Fatalf("Encabezado con la marca: %s", first)
	}
	withoutLogo := exportTestDocument(t, exportTestDefinition(), exportTestRows(5), PermissionCostsRead)
	withoutLogo.Logo = nil
	t.Logf("PDF de 5 renglones: %d bytes con la marca, %d sin imagen", len(content), len(renderPDFForTest(t, withoutLogo)))
}

// Excel con el logotipo del restaurante: la hoja «Filtros» tiene la imagen en A1 (la misma imagen
// preparada, anclada a una celda con 168 × 56 píxeles), el nombre en B1 y dirección, teléfono y
// correo en la tabla; la hoja «Datos» es idéntica a la de un libro sin identidad; sin fórmulas ni
// hipervínculos.
func TestXLSXHeaderWithRestaurantLogo(t *testing.T) {
	logo, err := prepareExportLogo(restaurantPNG(t), true)
	if err != nil {
		t.Fatal(err)
	}
	document := identityDocument(t, 12, logo)
	content := renderXLSXForTest(t, document)
	assertSafeWorkbook(t, content)
	file := openWorkbook(t, content)
	pictures, err := file.GetPictures(xlsxInfoSheet, "A1")
	if err != nil || len(pictures) != 1 {
		t.Fatalf("Logotipo en A1: %d imágenes, %v", len(pictures), err)
	}
	if !bytes.Equal(pictures[0].File, logo.content) || pictures[0].Extension != ".png" ||
		pictures[0].Format.AltText != "Logotipo del restaurante" || pictures[0].Format.Hyperlink != "" {
		t.Fatalf("Imagen de A1: %s %q %q", pictures[0].Extension, pictures[0].Format.AltText, pictures[0].Format.Hyperlink)
	}
	drawing := string(zipPart(t, content, "xl/drawings/drawing1.xml"))
	if !strings.Contains(drawing, "oneCellAnchor") || !strings.Contains(drawing, `cx="1600200" cy="533400"`) {
		t.Fatalf("El logotipo debe anclarse a una celda con 168 × 56 píxeles: %s", drawing)
	}
	relationships := string(zipPart(t, content, "xl/drawings/_rels/drawing1.xml.rels"))
	if strings.Contains(drawing, "hlinkClick") || strings.Contains(relationships, "External") {
		t.Fatal("El logotipo no debe llevar hipervínculos ni vínculos externos")
	}
	if others, _ := file.GetPictures(xlsxDataSheet, "A1"); len(others) != 0 {
		t.Fatal("La hoja «Datos» no lleva imágenes")
	}
	if name := shown(t, file, xlsxInfoSheet, "B1"); name != "Restaurante El Peñón" {
		t.Fatalf("Nombre en B1: %q", name)
	}
	if height, _ := file.GetRowHeight(xlsxInfoSheet, 1); height != 48 {
		t.Fatalf("Altura del renglón del logotipo: %v", height)
	}
	if shown(t, file, xlsxInfoSheet, "A3") != "Concepto" || shown(t, file, xlsxInfoSheet, "A4") != "Establecimiento" {
		t.Fatal("La tabla de metadatos empieza en el renglón 3")
	}
	expected := [][2]string{
		{"A5", "Dirección"}, {"B5", testAddress}, {"A6", "Teléfono"}, {"B6", testPhone},
		{"A7", "Correo electrónico"}, {"B7", testEmail}, {"A8", "Reporte"},
	}
	for _, pair := range expected {
		if value := shown(t, file, xlsxInfoSheet, pair[0]); value != pair[1] {
			t.Fatalf("%s: %q (esperado %q)", pair[0], value, pair[1])
		}
	}
	// La hoja «Datos» no cambia con la identidad ni con el logotipo.
	baseline := exportTestDocument(t, exportTestDefinition(), exportTestRows(12), PermissionCostsRead)
	baseline.Logo = nil
	plain := renderXLSXForTest(t, baseline)
	if !bytes.Equal(zipPart(t, content, "xl/worksheets/sheet1.xml"), zipPart(t, plain, "xl/worksheets/sheet1.xml")) {
		t.Fatal("La hoja «Datos» cambió con el encabezado de identidad")
	}
	if pictures, _ := openWorkbook(t, plain).GetPictures(xlsxInfoSheet, "A1"); len(pictures) != 0 {
		t.Fatal("Sin logotipo no hay imagen")
	}
	// Un JPEG se inserta como JPEG.
	photo, err := prepareExportLogo(restaurantJPEG(t), true)
	if err != nil {
		t.Fatal(err)
	}
	jpegBook := renderXLSXForTest(t, identityDocument(t, 3, photo))
	assertSafeWorkbook(t, jpegBook)
	if pictures, _ := openWorkbook(t, jpegBook).GetPictures(xlsxInfoSheet, "A1"); len(pictures) != 1 || pictures[0].Extension != ".jpeg" ||
		!bytes.Equal(pictures[0].File, photo.content) {
		t.Fatal("El logotipo JPEG debe insertarse tal cual en A1")
	}
}

// Sin logotipo propio la hoja «Filtros» lleva la marca de AD-EL POS y el nombre del establecimiento;
// sin dirección, teléfono ni correo no se agregan esos renglones.
func TestXLSXHeaderFallsBackToBrandLogo(t *testing.T) {
	document := exportTestDocument(t, exportTestDefinition(), exportTestRows(3), PermissionCostsRead)
	content := renderXLSXForTest(t, document)
	assertSafeWorkbook(t, content)
	file := openWorkbook(t, content)
	pictures, err := file.GetPictures(xlsxInfoSheet, "A1")
	if err != nil || len(pictures) != 1 || !bytes.Equal(pictures[0].File, testBrandLogo(t).content) ||
		pictures[0].Format.AltText != "Logotipo de AD-EL POS" {
		t.Fatalf("La marca debe ocupar A1: %d imágenes, %v", len(pictures), err)
	}
	rows, err := file.GetRows(xlsxInfoSheet)
	if err != nil {
		t.Fatal(err)
	}
	for _, row := range rows {
		if len(row) > 0 && (row[0] == "Dirección" || row[0] == "Teléfono" || row[0] == "Correo electrónico") {
			t.Fatalf("Renglón de contacto sin dato: %v", row)
		}
	}
	if shown(t, file, xlsxInfoSheet, "B1") != "Restaurante El Peñón" || shown(t, file, xlsxInfoSheet, "A5") != "Reporte" {
		t.Fatal("Con la marca, el nombre va en B1 y la tabla sigue con el reporte")
	}
	withoutLogo := exportTestDocument(t, exportTestDefinition(), exportTestRows(3), PermissionCostsRead)
	withoutLogo.Logo = nil
	t.Logf("XLSX de 3 renglones: %d bytes con la marca, %d sin imagen", len(content), len(renderXLSXForTest(t, withoutLogo)))
}

// El texto del establecimiento que parece fórmula también lleva apóstrofo en el encabezado.
func TestXLSXHeaderEscapesFormulaText(t *testing.T) {
	document := exportTestDocument(t, exportTestDefinition(), exportTestRows(2))
	document.Establishment = "=HYPERLINK(\"http://example.invalid\",\"x\")"
	document.Address = "+52 Calle"
	content := renderXLSXForTest(t, document)
	assertSafeWorkbook(t, content)
	file := openWorkbook(t, content)
	if raw(t, file, xlsxInfoSheet, "B1") != "'"+document.Establishment || raw(t, file, xlsxInfoSheet, "B5") != "'+52 Calle" {
		t.Fatalf("Textos del encabezado: %q %q", raw(t, file, xlsxInfoSheet, "B1"), raw(t, file, xlsxInfoSheet, "B5"))
	}
	if formula, _ := file.GetCellFormula(xlsxInfoSheet, "B1"); formula != "" {
		t.Fatalf("B1 quedó como fórmula: %s", formula)
	}
}

// HeaderLogoForTest expone a las pruebas de integración (paquete reports_test) la imagen que lleva
// el encabezado para un logotipo del restaurante.
func HeaderLogoForTest(t *testing.T, content []byte) []byte {
	t.Helper()
	logo, err := prepareExportLogo(content, true)
	if err != nil {
		t.Fatal(err)
	}
	return logo.content
}

// BrandHeaderLogoForTest es la imagen del encabezado con la marca de AD-EL POS.
func BrandHeaderLogoForTest(t *testing.T) []byte {
	t.Helper()
	return testBrandLogo(t).content
}

// PDFImageSizesForTest devuelve ancho y alto de cada imagen de un PDF.
func PDFImageSizesForTest(content []byte) [][2]int {
	sizes, _ := pdfImageSizes(content)
	return sizes
}
