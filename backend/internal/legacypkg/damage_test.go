package legacypkg

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/binary"
	"encoding/json"
	"strings"
	"testing"
)

// samplePackage es un paquete de varios bloques para las pruebas de daño.
func samplePackage(t *testing.T) []byte {
	t.Helper()
	encoded, _ := buildPackage(t, []testTable{{spec: salesSpec(), rows: salesRows(3000)}}, nil)
	return encoded
}

// headerLength es el tamaño del encabezado de un paquete (firma, versión, longitud, JSON y su SHA-256).
func headerLength(encoded []byte) int {
	length := binary.BigEndian.Uint32(encoded[len(Signature)+2:])
	return fixedHeaderSize + int(length) + sha256.Size
}

func blocks(t *testing.T, encoded []byte) (int, [][]byte) {
	t.Helper()
	start := headerLength(encoded)
	sealed := testChunkSize + tagSize
	list := [][]byte{}
	for offset := start; offset < len(encoded); offset += sealed {
		end := offset + sealed
		if end > len(encoded) {
			end = len(encoded)
		}
		list = append(list, encoded[offset:end])
	}
	if len(list) < 4 {
		t.Fatalf("el paquete de prueba tiene solo %d bloques", len(list))
	}
	return start, list
}

func join(header []byte, list ...[]byte) []byte {
	result := append([]byte{}, header...)
	for _, block := range list {
		result = append(result, block...)
	}
	return result
}

func verifyBytes(encoded []byte) error {
	_, err := Verify(bytes.NewReader(encoded), testPassphrase, OpenOptions{})
	return err
}

func TestDamagedPackagesAreRejected(t *testing.T) {
	original := samplePackage(t)
	if err := verifyBytes(original); err != nil {
		t.Fatal(err)
	}
	start, list := blocks(t, original)
	header := original[:start]
	last := len(list) - 1
	flip := func(position int) []byte {
		changed := append([]byte{}, original...)
		changed[position] ^= 0x01
		return changed
	}
	cases := map[string][]byte{
		"byte del JSON del encabezado":    flip(fixedHeaderSize + 10),
		"byte del SHA-256 del encabezado": flip(start - 1),
		"byte del primer bloque":          flip(start + 5),
		"byte de un bloque intermedio":    flip(start + 2*(testChunkSize+tagSize) + 100),
		"byte de la etiqueta del último":  flip(len(original) - 1),
		"truncado a media":                original[:start+(testChunkSize+tagSize)*2+17],
		"truncado al final":               original[:len(original)-3],
		"sin el último bloque":            join(header, list[:last]...),
		"último bloque duplicado":         join(header, append(append([][]byte{}, list...), list[last])...),
		"bloques intercambiados":          join(header, append([][]byte{list[1], list[0]}, list[2:]...)...),
		"bloque intermedio quitado":       join(header, append([][]byte{list[0]}, list[2:]...)...),
		"datos de más al final":           append(append([]byte{}, original...), 0x00),
		"solo el encabezado":              append([]byte{}, header...),
	}
	for name, encoded := range cases {
		t.Run(name, func(t *testing.T) {
			requireKind(t, verifyBytes(encoded), KindDamaged)
		})
	}
}

func TestNotAPackage(t *testing.T) {
	cases := map[string][]byte{
		"vacío":            {},
		"texto":            []byte("esto no es un paquete"),
		"firma incompleta": []byte("ADELMI"),
		"firma sin NUL":    []byte("ADELMIG!" + strings.Repeat("x", 100)),
		"respaldo MTF":     append([]byte("TAPE"), make([]byte, 100)...),
	}
	for name, encoded := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := Open(bytes.NewReader(encoded), testPassphrase, OpenOptions{})
			requireKind(t, err, KindNotPackage)
			_, err = Inspect(bytes.NewReader(encoded), OpenOptions{})
			requireKind(t, err, KindNotPackage)
		})
	}
}

// Una versión del formato más nueva se reconoce antes de comprobar nada más (para decir «actualiza AD-EL POS»).
func TestUnknownFormatVersion(t *testing.T) {
	encoded := samplePackage(t)
	newer := append([]byte{}, encoded...)
	binary.BigEndian.PutUint16(newer[len(Signature):], FormatVersion+1)
	_, err := Open(bytes.NewReader(newer), testPassphrase, OpenOptions{})
	requireKind(t, err, KindUnsupportedVersion)
	if !strings.Contains(err.Error(), "más nueva") {
		t.Fatalf("mensaje: %v", err)
	}
	zero := append([]byte{}, encoded...)
	binary.BigEndian.PutUint16(zero[len(Signature):], 0)
	_, err = Open(bytes.NewReader(zero), testPassphrase, OpenOptions{})
	requireKind(t, err, KindDamaged)
}

// headerWith arma un encabezado válido (con su SHA-256) con parámetros manipulados, seguido de basura.
func headerWith(t *testing.T, change func(params *headerParams)) []byte {
	t.Helper()
	params, _ := testParams(t)
	change(&params)
	header, err := encodeHeader(params)
	if err != nil {
		t.Fatal(err)
	}
	return append(header, make([]byte, 64)...)
}

// Límites contra paquetes manipulados: se rechazan antes de gastar memoria o tiempo (Argon2 con 4 GiB respondería en
// segundos o agotaría la memoria; aquí la prueba termina al instante).
func TestHeaderLimits(t *testing.T) {
	cases := map[string]struct {
		change func(params *headerParams)
		kind   ErrorKind
	}{
		"Argon2 con memoria enorme": {func(params *headerParams) { params.KDF.MemoryKiB = 4 << 20 }, KindLimit},
		"Argon2 con tiempo enorme":  {func(params *headerParams) { params.KDF.Time = 1 << 20 }, KindLimit},
		"Argon2 con muchos hilos":   {func(params *headerParams) { params.KDF.Threads = 255 }, KindLimit},
		"bloque enorme":             {func(params *headerParams) { params.ChunkSize = 1 << 30 }, KindLimit},
		"bloque diminuto":           {func(params *headerParams) { params.ChunkSize = 16 }, KindDamaged},
		"Argon2 sin memoria":        {func(params *headerParams) { params.KDF.MemoryKiB = 1 }, KindDamaged},
		"otro algoritmo":            {func(params *headerParams) { params.Cipher = "AES-128-CBC" }, KindDamaged},
		"sal corta":                 {func(params *headerParams) { params.KDF.Salt = params.KDF.Salt[:4] }, KindDamaged},
	}
	for name, testCase := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := Open(bytes.NewReader(headerWith(t, testCase.change)), testPassphrase, OpenOptions{})
			requireKind(t, err, testCase.kind)
		})
	}
	unknown := headerWith(t, func(params *headerParams) {})
	length := binary.BigEndian.Uint32(unknown[len(Signature)+2:])
	params := unknown[fixedHeaderSize : fixedHeaderSize+int(length)]
	withExtra := bytes.Replace(params, []byte(`"archive":"tar"`), []byte(`"archive":"tar","extra":1`), 1)
	var rebuilt bytes.Buffer
	rebuilt.WriteString(Signature)
	_ = binary.Write(&rebuilt, binary.BigEndian, FormatVersion)
	_ = binary.Write(&rebuilt, binary.BigEndian, uint32(len(withExtra)))
	rebuilt.Write(withExtra)
	checksum := sha256.Sum256(rebuilt.Bytes())
	rebuilt.Write(checksum[:])
	_, err := Open(bytes.NewReader(rebuilt.Bytes()), testPassphrase, OpenOptions{})
	requireKind(t, err, KindDamaged)
}

func validManifestEntry(t *testing.T, archive *tar.Writer, tables map[string][]byte, order []string, rows map[string]int64) {
	t.Helper()
	addEntry(t, archive, &tar.Header{Name: ManifestFile}, encodeJSON(t, manifestFor(tables, order, rows)))
}

// Entradas del tar manipuladas: rutas con «..», un enlace, el manifiesto fuera de lugar, entradas de más o que no
// corresponden al manifiesto. Todas se rechazan sin escribir nada en el disco (la lectura nunca extrae archivos).
func TestManipulatedArchiveEntries(t *testing.T) {
	params, keys := testParams(t)
	content := []byte(`[{"valor":"a"}]`)
	tables := map[string][]byte{"notas": content}
	rows := map[string]int64{"notas": 1}
	cases := map[string]func(archive *tar.Writer){
		"ruta con ..": func(archive *tar.Writer) {
			validManifestEntry(t, archive, tables, []string{"notas"}, rows)
			addEntry(t, archive, &tar.Header{Name: "tables/../../fuera.json"}, content)
		},
		"ruta absoluta": func(archive *tar.Writer) {
			validManifestEntry(t, archive, tables, []string{"notas"}, rows)
			addEntry(t, archive, &tar.Header{Name: "/etc/notas.json"}, content)
		},
		"enlace simbólico": func(archive *tar.Writer) {
			validManifestEntry(t, archive, tables, []string{"notas"}, rows)
			addEntry(t, archive, &tar.Header{Name: "tables/notas.json", Typeflag: tar.TypeSymlink, Linkname: "/etc/passwd"}, nil)
		},
		"manifiesto después": func(archive *tar.Writer) {
			addEntry(t, archive, &tar.Header{Name: "tables/notas.json"}, content)
			validManifestEntry(t, archive, tables, []string{"notas"}, rows)
		},
		"entrada de más": func(archive *tar.Writer) {
			validManifestEntry(t, archive, tables, []string{"notas"}, rows)
			addEntry(t, archive, &tar.Header{Name: "tables/notas.json"}, content)
			addEntry(t, archive, &tar.Header{Name: "tables/extra.json"}, content)
		},
		"falta la tabla": func(archive *tar.Writer) {
			validManifestEntry(t, archive, tables, []string{"notas"}, rows)
		},
		"conteo distinto": func(archive *tar.Writer) {
			validManifestEntry(t, archive, tables, []string{"notas"}, map[string]int64{"notas": 2})
			addEntry(t, archive, &tar.Header{Name: "tables/notas.json"}, content)
		},
		"SHA-256 distinto": func(archive *tar.Writer) {
			validManifestEntry(t, archive, map[string][]byte{"notas": []byte(`[{"valor":"b"}]`)}, []string{"notas"}, rows)
			addEntry(t, archive, &tar.Header{Name: "tables/notas.json"}, content)
		},
		"columna distinta": func(archive *tar.Writer) {
			other := []byte(`[{"otra":"a"}]`)
			validManifestEntry(t, archive, map[string][]byte{"notas": other}, []string{"notas"}, rows)
			addEntry(t, archive, &tar.Header{Name: "tables/notas.json"}, other)
		},
		"valor anidado": func(archive *tar.Writer) {
			nested := []byte(`[{"valor":{"x":1}}]`)
			validManifestEntry(t, archive, map[string][]byte{"notas": nested}, []string{"notas"}, rows)
			addEntry(t, archive, &tar.Header{Name: "tables/notas.json"}, nested)
		},
		"JSON inválido": func(archive *tar.Writer) {
			broken := []byte(`[{"valor":"a"`)
			validManifestEntry(t, archive, map[string][]byte{"notas": broken}, []string{"notas"}, rows)
			addEntry(t, archive, &tar.Header{Name: "tables/notas.json"}, broken)
		},
		"huella de contenido distinta": func(archive *tar.Writer) {
			manifest := manifestFor(tables, []string{"notas"}, rows)
			manifest.ContentFingerprint = strings.Repeat("0", 64)
			addEntry(t, archive, &tar.Header{Name: ManifestFile}, encodeJSON(t, manifest))
			addEntry(t, archive, &tar.Header{Name: "tables/notas.json"}, content)
		},
		"manifiesto inválido": func(archive *tar.Writer) {
			addEntry(t, archive, &tar.Header{Name: ManifestFile}, []byte(`{"format":"adelmig"`))
		},
	}
	for name, fill := range cases {
		t.Run(name, func(t *testing.T) {
			requireKind(t, verifyBytes(craftedPackage(t, params, keys, fill)), KindDamaged)
		})
	}
	valid := craftedPackage(t, params, keys, func(archive *tar.Writer) {
		validManifestEntry(t, archive, tables, []string{"notas"}, rows)
		addEntry(t, archive, &tar.Header{Name: "tables/notas.json"}, content)
	})
	if err := verifyBytes(valid); err != nil {
		t.Fatalf("el paquete armado a mano válido no abrió: %v", err)
	}
}

// Bomba de descompresión, tabla o fila enormes y demasiadas tablas: la lectura se detiene en el límite.
func TestContentLimits(t *testing.T) {
	params, keys := testParams(t)
	// 3 MiB de espacios dentro del arreglo: JSON válido que se comprime a casi nada.
	bomb := append(append([]byte("["), bytes.Repeat([]byte(" "), 3<<20)...), ']')
	bombPackage := craftedPackage(t, params, keys, func(archive *tar.Writer) {
		validManifestEntry(t, archive, map[string][]byte{"notas": bomb}, []string{"notas"}, map[string]int64{"notas": 0})
		addEntry(t, archive, &tar.Header{Name: "tables/notas.json"}, bomb)
	})
	if len(bombPackage) > 64<<10 {
		t.Fatalf("la bomba ocupa %d bytes", len(bombPackage))
	}
	small := DefaultLimits()
	small.MaxUncompressedBytes = 1 << 20
	_, err := Verify(bytes.NewReader(bombPackage), testPassphrase, OpenOptions{Limits: small})
	requireKind(t, err, KindLimit)
	tableLimit := DefaultLimits()
	tableLimit.MaxTableBytes = 1 << 20
	_, err = Open(bytes.NewReader(bombPackage), testPassphrase, OpenOptions{Limits: tableLimit})
	requireKind(t, err, KindLimit)
	// Una fila de 2 MiB con un límite de 64 KiB por fila.
	row := encodeJSON(t, []map[string]string{{"valor": strings.Repeat("x", 2<<20)}})
	rowPackage := craftedPackage(t, params, keys, func(archive *tar.Writer) {
		validManifestEntry(t, archive, map[string][]byte{"notas": row}, []string{"notas"}, map[string]int64{"notas": 1})
		addEntry(t, archive, &tar.Header{Name: "tables/notas.json"}, row)
	})
	rowLimit := DefaultLimits()
	rowLimit.MaxRowBytes = 64 << 10
	_, err = Verify(bytes.NewReader(rowPackage), testPassphrase, OpenOptions{Limits: rowLimit})
	requireKind(t, err, KindLimit)
	if _, err := Verify(bytes.NewReader(rowPackage), testPassphrase, OpenOptions{}); err != nil {
		t.Fatalf("con los límites normales la fila de 2 MiB se acepta: %v", err)
	}
	// Más tablas que el límite.
	many := map[string][]byte{}
	order := []string{}
	counts := map[string]int64{}
	for _, name := range []string{"a", "b", "c"} {
		many[name] = []byte(`[]`)
		order = append(order, name)
		counts[name] = 0
	}
	manyPackage := craftedPackage(t, params, keys, func(archive *tar.Writer) {
		validManifestEntry(t, archive, many, order, counts)
		for _, name := range order {
			addEntry(t, archive, &tar.Header{Name: "tables/" + name + ".json"}, many[name])
		}
	})
	tables := DefaultLimits()
	tables.MaxTables = 2
	_, err = Open(bytes.NewReader(manyPackage), testPassphrase, OpenOptions{Limits: tables})
	requireKind(t, err, KindLimit)
	if err := verifyBytes(manyPackage); err != nil {
		t.Fatal(err)
	}
	// Un manifiesto enorme.
	manifestLimit := DefaultLimits()
	manifestLimit.MaxManifestBytes = 64
	_, err = Open(bytes.NewReader(manyPackage), testPassphrase, OpenOptions{Limits: manifestLimit})
	requireKind(t, err, KindLimit)
}

func TestErrorMessagesAreSpanishAndTyped(t *testing.T) {
	_, err := Open(bytes.NewReader([]byte("x")), testPassphrase, OpenOptions{})
	var typed *Error
	if !strings.Contains(err.Error(), "no es un paquete") || !asError(err, &typed) || typed.Kind != KindNotPackage {
		t.Fatalf("error: %v", err)
	}
	encoded, err := json.Marshal(map[string]string{"mensaje": typed.Message})
	if err != nil || !bytes.Contains(encoded, []byte("paquete")) {
		t.Fatal("mensaje")
	}
}

func asError(err error, target **Error) bool {
	typed, ok := err.(*Error)
	if ok {
		*target = typed
	}
	return ok
}

// Texto claro de más después del gzip (otro miembro gzip o bytes sueltos, cifrados como uno legítimo): dañado.
func TestTrailingPlaintextAfterTheArchive(t *testing.T) {
	params, keys := testParams(t)
	content := []byte(`[{"valor":"a"}]`)
	tables := map[string][]byte{"notas": content}
	rows := map[string]int64{"notas": 1}
	for name, trailing := range map[string][]byte{
		"bytes sueltos":    []byte("extra"),
		"otro miembro":     gzipped(t, []byte("otro contenido")),
		"un solo byte NUL": {0},
	} {
		t.Run(name, func(t *testing.T) {
			header, err := encodeHeader(params)
			if err != nil {
				t.Fatal(err)
			}
			var output bytes.Buffer
			output.Write(header)
			headerHash := sha256.Sum256(header)
			encrypted, err := newChunkWriter(&output, keys.encryption, params.NoncePrefix, headerHash[:], params.ChunkSize)
			if err != nil {
				t.Fatal(err)
			}
			compressed := gzip.NewWriter(encrypted)
			archive := tar.NewWriter(compressed)
			validManifestEntry(t, archive, tables, []string{"notas"}, rows)
			addEntry(t, archive, &tar.Header{Name: "tables/notas.json"}, content)
			if err := archive.Close(); err != nil {
				t.Fatal(err)
			}
			if err := compressed.Close(); err != nil {
				t.Fatal(err)
			}
			if _, err := encrypted.Write(trailing); err != nil {
				t.Fatal(err)
			}
			if err := encrypted.Close(); err != nil {
				t.Fatal(err)
			}
			requireKind(t, verifyBytes(output.Bytes()), KindDamaged)
		})
	}
}

func gzipped(t *testing.T, content []byte) []byte {
	t.Helper()
	var output bytes.Buffer
	writer := gzip.NewWriter(&output)
	if _, err := writer.Write(content); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	return output.Bytes()
}
