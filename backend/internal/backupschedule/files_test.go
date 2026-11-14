package backupschedule

import (
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"testing"
	"time"
)

func touch(t *testing.T, directory, name string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(directory, name), []byte("x"), 0600); err != nil {
		t.Fatal(err)
	}
}

func names(t *testing.T, directory string) []string {
	t.Helper()
	entries, err := os.ReadDir(directory)
	if err != nil {
		t.Fatal(err)
	}
	result := []string{}
	for _, entry := range entries {
		result = append(result, entry.Name())
	}
	return result
}

// realPath es la ruta sin enlaces (en macOS la carpeta temporal puede estar detrás de uno).
func realPath(t *testing.T, path string) string {
	t.Helper()
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil {
		t.Fatal(err)
	}
	return resolved
}

// El nombre lleva la base, la instalación (8 caracteres hexadecimales de su identificador), la fecha UTC, el sufijo y
// la etiqueta.
func TestFileNames(t *testing.T) {
	instant := time.Date(2026, 11, 14, 11, 0, 5, 0, time.UTC)
	if name := FileName("adel_pos", "3f2a9c1b", KindAutomatic, instant, "a1b2c3"); name != "adel_pos-3f2a9c1b-20261114T110005Z-a1b2c3-automatico.dump" {
		t.Fatalf("automático: %s", name)
	}
	if name := FileName("adel_pos", "3f2a9c1b", KindManual, instant, "a1b2c3"); name != "adel_pos-3f2a9c1b-20261114T110005Z-a1b2c3-manual.dump" {
		t.Fatalf("manual: %s", name)
	}
	if tag := InstallationTag("3F2A9C1B-0d4e-4f5a-8b6c-7d8e9f0a1b2c"); tag != "3f2a9c1b" {
		t.Fatalf("identificador de la instalación: %q", tag)
	}
	for _, invalid := range []string{"", "3f2a", "3f2a9c1g-0000", "../../etc"} {
		if tag := InstallationTag(invalid); tag != "" {
			t.Fatalf("identificador aceptado: %q → %q", invalid, tag)
		}
	}
}

// La retención elige, de los anteriores de la misma carpeta (del más reciente al más viejo según el registro), los que
// sobran contando el recién hecho como uno de los conservados.
func TestToRemove(t *testing.T) {
	older := []retentionRow{{ID: "4"}, {ID: "3"}, {ID: "2"}, {ID: "1"}}
	ids := func(rows []retentionRow) []string {
		result := []string{}
		for _, row := range rows {
			result = append(result, row.ID)
		}
		return result
	}
	if got := ids(toRemove(older, 3)); !slices.Equal(got, []string{"2", "1"}) {
		t.Fatalf("conservar 3: %v", got)
	}
	if got := ids(toRemove(older, 1)); !slices.Equal(got, []string{"4", "3", "2", "1"}) {
		t.Fatalf("conservar 1: %v", got)
	}
	if got := toRemove(older, 30); len(got) != 0 {
		t.Fatalf("conservar 30: %v", got)
	}
	if got := toRemove(nil, 2); len(got) != 0 {
		t.Fatal("sin anteriores")
	}
}

// removeRegular solo borra archivos regulares: nunca sigue un enlace ni borra una carpeta; un archivo que ya no está
// cuenta como borrado.
func TestRemoveRegular(t *testing.T) {
	directory := t.TempDir()
	touch(t, directory, "propio.dump")
	if gone, err := removeRegular(directory, "propio.dump"); !gone || err != nil {
		t.Fatalf("archivo propio: %v %v", gone, err)
	}
	if gone, err := removeRegular(directory, "propio.dump"); !gone || err != nil {
		t.Fatalf("archivo que ya no está: %v %v", gone, err)
	}
	victim := filepath.Join(t.TempDir(), "ajeno.dump")
	if err := os.WriteFile(victim, []byte("ajeno"), 0600); err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS != "windows" {
		if err := os.Symlink(victim, filepath.Join(directory, "enlace.dump")); err != nil {
			t.Fatal(err)
		}
		if gone, err := removeRegular(directory, "enlace.dump"); gone || err == nil {
			t.Fatal("se borró un enlace")
		}
		if _, err := os.Stat(victim); err != nil {
			t.Fatal("se borró el destino del enlace")
		}
	}
	if err := os.Mkdir(filepath.Join(directory, "carpeta.dump"), 0700); err != nil {
		t.Fatal(err)
	}
	if gone, err := removeRegular(directory, "carpeta.dump"); gone || err == nil {
		t.Fatal("se borró una carpeta")
	}
	for _, invalid := range []string{"", ".", "..", "../ajeno.dump", "a/b.dump"} {
		if gone, err := removeRegular(directory, invalid); gone || err == nil {
			t.Fatalf("nombre aceptado: %q", invalid)
		}
	}
}

func TestCheckDirectory(t *testing.T) {
	directory := t.TempDir()
	if reason := CheckDirectory(directory); reason != "" {
		t.Fatalf("carpeta disponible: %s", reason)
	}
	if left := names(t, directory); len(left) != 0 {
		t.Fatalf("la prueba de escritura dejó archivos: %v", left)
	}
	if reason := CheckDirectory(filepath.Join(directory, "usb-desconectada")); reason != ReasonMissing {
		t.Fatalf("carpeta inexistente: %s", reason)
	}
	// Comprobar una carpeta que no existe nunca la crea.
	if _, err := os.Stat(filepath.Join(directory, "usb-desconectada")); err == nil {
		t.Fatal("se creó la carpeta inexistente")
	}
	touch(t, directory, "archivo")
	if reason := CheckDirectory(filepath.Join(directory, "archivo")); reason != ReasonNotDirectory {
		t.Fatalf("archivo: %s", reason)
	}
	if runtime.GOOS != "windows" && os.Geteuid() != 0 {
		readOnly := filepath.Join(directory, "solo-lectura")
		if err := os.Mkdir(readOnly, 0500); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() {
			_ = os.Chmod(readOnly, 0700)
		})
		if reason := CheckDirectory(readOnly); reason != ReasonNotWritable {
			t.Fatalf("sin permiso de escritura: %s", reason)
		}
	}
}

// La carpeta elegida se guarda resuelta (sin enlaces) y no puede estar dentro de una carpeta de datos de PostgreSQL ni
// de las carpetas protegidas del servidor, ni llegar a ellas por un enlace.
func TestValidateDirectory(t *testing.T) {
	root := realPath(t, t.TempDir())
	usb := filepath.Join(root, "usb", "Respaldos")
	if err := os.MkdirAll(usb, 0700); err != nil {
		t.Fatal(err)
	}
	server := filepath.Join(root, "AD-EL POS")
	postgres := filepath.Join(server, "postgres")
	for _, folder := range []string{filepath.Join(server, "identity"), filepath.Join(server, "logs"), filepath.Join(postgres, "pg_wal")} {
		if err := os.MkdirAll(folder, 0700); err != nil {
			t.Fatal(err)
		}
	}
	touch(t, postgres, "PG_VERSION")
	external := filepath.Join(root, "otro-cluster", "base")
	if err := os.MkdirAll(external, 0700); err != nil {
		t.Fatal(err)
	}
	touch(t, filepath.Dir(external), "PG_VERSION")
	protected := []string{server}
	clean, err := ValidateDirectory(usb+string(filepath.Separator)+".", protected)
	if err != nil || clean != usb {
		t.Fatalf("carpeta válida: %q %v", clean, err)
	}
	cases := map[string]string{
		"":                                ReasonInvalid,
		"respaldos":                       ReasonRelative,
		"./respaldos":                     ReasonRelative,
		filepath.Join(root, "no-existe"):  ReasonMissing,
		root + "/linea\nnueva":            ReasonInvalid,
		postgres:                          ReasonPostgresData,
		filepath.Join(postgres, "pg_wal"): ReasonPostgresData,
		external:                          ReasonPostgresData,
		server:                            ReasonServerData,
		filepath.Join(server, "identity"): ReasonServerData,
		filepath.Join(server, "logs"):     ReasonServerData,
		string(make([]byte, maxDirectoryLength+1)): ReasonInvalid,
	}
	if runtime.GOOS != "windows" {
		// Un enlace hacia los datos del servidor no los vuelve aceptables, y uno hacia la USB se guarda resuelto.
		toServer := filepath.Join(root, "atajo-servidor")
		if err := os.Symlink(filepath.Join(server, "logs"), toServer); err != nil {
			t.Fatal(err)
		}
		cases[toServer] = ReasonServerData
		toPostgres := filepath.Join(root, "atajo-base")
		if err := os.Symlink(filepath.Join(postgres, "pg_wal"), toPostgres); err != nil {
			t.Fatal(err)
		}
		cases[toPostgres] = ReasonPostgresData
		toUSB := filepath.Join(root, "atajo-usb")
		if err := os.Symlink(usb, toUSB); err != nil {
			t.Fatal(err)
		}
		if resolved, err := ValidateDirectory(toUSB, protected); err != nil || resolved != usb {
			t.Fatalf("enlace a la USB: %q %v", resolved, err)
		}
	}
	for input, reason := range cases {
		_, err := ValidateDirectory(input, protected)
		problem, ok := err.(*DirectoryError)
		if !ok || problem.Reason != reason {
			t.Fatalf("%q: %v, esperado %s", input, err, reason)
		}
	}
}

func TestInside(t *testing.T) {
	separator := string(filepath.Separator)
	base := separator + filepath.Join("datos", "AD-EL POS")
	for _, inside := range []string{base, filepath.Join(base, "logs"), filepath.Join(base, "postgres", "pg_wal")} {
		if !Inside(inside, base) {
			t.Fatalf("%s está dentro de %s", inside, base)
		}
	}
	for _, outside := range []string{separator + "datos", base + " 2", filepath.Join(separator+"datos", "otra"), separator + filepath.Join("..datos")} {
		if Inside(outside, base) {
			t.Fatalf("%s no está dentro de %s", outside, base)
		}
	}
}

func TestDescribe(t *testing.T) {
	folder, volume := Describe("/Users/restaurante/Respaldos")
	if folder != "Respaldos" || volume != "" {
		t.Fatalf("carpeta local: %q %q", folder, volume)
	}
	if runtime.GOOS == "darwin" {
		folder, volume = Describe("/Volumes/KINGSTON/AD-EL POS/Respaldos")
		if folder != "Respaldos" || volume != "KINGSTON" {
			t.Fatalf("USB: %q %q", folder, volume)
		}
		folder, volume = Describe("/Volumes/KINGSTON")
		if folder != "" || volume != "KINGSTON" {
			t.Fatalf("raíz de la USB: %q %q", folder, volume)
		}
	}
	if folder, volume := Describe(""); folder != "" || volume != "" {
		t.Fatal("carpeta vacía")
	}
}

// Carpeta no disponible → predeterminada: la carpeta elegida que no existe (USB desconectada) o no acepta archivos se
// sustituye por la predeterminada (que se crea si falta) con el motivo; sin ninguna disponible, el respaldo falla.
func TestChooseDirectoryFallsBackToDefault(t *testing.T) {
	root := t.TempDir()
	target := newFakeTarget(filepath.Join(root, "predeterminada"))
	runner := &Runner{Target: target}
	chosen := filepath.Join(root, "usb")
	if err := os.Mkdir(chosen, 0700); err != nil {
		t.Fatal(err)
	}
	directory, usedDefault, fallback, ok := runner.chooseDirectory(chosen)
	if !ok || directory != chosen || usedDefault || fallback != "" {
		t.Fatalf("carpeta elegida disponible: %s %v %q %v", directory, usedDefault, fallback, ok)
	}
	directory, usedDefault, fallback, ok = runner.chooseDirectory(filepath.Join(root, "usb-desconectada"))
	if !ok || directory != target.directory || !usedDefault || fallback != ReasonMissing {
		t.Fatalf("carpeta elegida desconectada: %s %v %q %v", directory, usedDefault, fallback, ok)
	}
	if info, err := os.Stat(target.directory); err != nil || !info.IsDir() {
		t.Fatal("no se creó la carpeta predeterminada")
	}
	if _, err := os.Stat(filepath.Join(root, "usb-desconectada")); err == nil {
		t.Fatal("se creó la carpeta de la USB desconectada")
	}
	directory, usedDefault, fallback, ok = runner.chooseDirectory("")
	if !ok || directory != target.directory || !usedDefault || fallback != "" {
		t.Fatalf("sin carpeta elegida: %s %v %q %v", directory, usedDefault, fallback, ok)
	}
	// La predeterminada tampoco sirve (un archivo ocupa su nombre).
	blocked := filepath.Join(root, "ocupada")
	touch(t, root, "ocupada")
	runner.Target = newFakeTarget(blocked)
	if _, _, fallback, ok := runner.chooseDirectory(filepath.Join(root, "usb-desconectada")); ok || fallback != ReasonMissing {
		t.Fatalf("ninguna carpeta disponible: %v %q", ok, fallback)
	}
}

// El aviso: un fallo avisa hasta un respaldo correcto posterior; activos y sin respaldo correcto en más de un día y
// dos horas, «atrasado» (también si nunca hubo uno); un respaldo en la predeterminada porque la elegida no estaba avisa
// mientras esa siga siendo la carpeta configurada.
func TestAlertFor(t *testing.T) {
	now := time.Date(2026, 11, 14, 12, 0, 0, 0, time.UTC)
	started := now.Add(-time.Hour)
	first := now.Add(-24 * time.Hour)
	failed := &Run{Status: StatusFailed, FailureReason: FailureVerify, StartedAt: started}
	base := alertInput{Now: now, Enabled: true, Configured: true, Expected: now.Add(-72 * time.Hour)}
	recent := &Run{Status: StatusSucceeded, StartedAt: now.Add(-20 * time.Hour)}
	old := &Run{Status: StatusSucceeded, StartedAt: now.Add(-27 * time.Hour)}

	input := base
	input.Last, input.LastSuccess, input.Failures, input.FirstFailure = failed, recent, 2, first
	if alert := alertFor(input); alert == nil || alert.Kind != "failed" || alert.Reason != FailureVerify || alert.Failures != 2 ||
		!alert.Since.Equal(first) {
		t.Fatalf("fallo: %+v", alert)
	}
	input = base
	input.Last, input.LastSuccess = recent, recent
	if alert := alertFor(input); alert != nil {
		t.Fatalf("respaldo correcto reciente: %+v", alert)
	}
	input.Last, input.LastSuccess = old, old
	if alert := alertFor(input); alert == nil || alert.Kind != "overdue" || alert.Reason != "stale" || !alert.Since.Equal(old.StartedAt) {
		t.Fatalf("atrasado: %+v", alert)
	}
	// Recién activados (o recién configurado el establecimiento): se mide desde entonces.
	input.Expected = now.Add(-2 * time.Hour)
	if alert := alertFor(input); alert != nil {
		t.Fatalf("recién activados: %+v", alert)
	}
	input = base
	if alert := alertFor(input); alert == nil || alert.Kind != "overdue" || alert.Reason != "never" {
		t.Fatalf("nunca hubo un respaldo: %+v", alert)
	}
	input.Enabled = false
	if alert := alertFor(input); alert != nil {
		t.Fatalf("desactivados no avisan atraso: %+v", alert)
	}
	input = base
	input.Configured = false
	if alert := alertFor(input); alert != nil {
		t.Fatalf("sin establecimiento: %+v", alert)
	}
	fallback := &Run{Status: StatusSucceeded, StartedAt: started, UsedDefault: true, FallbackReason: ReasonMissing,
		RequestedDirectory: "/Volumes/USB/Respaldos"}
	input = base
	input.Last, input.LastSuccess, input.ConfiguredDirectory = fallback, fallback, "/Volumes/USB/Respaldos"
	if alert := alertFor(input); alert == nil || alert.Kind != "fallback" {
		t.Fatalf("predeterminada por falta de la elegida: %+v", alert)
	}
	input.ConfiguredDirectory = ""
	if alert := alertFor(input); alert != nil {
		t.Fatalf("se volvió a la predeterminada: %+v", alert)
	}
	input.ConfiguredDirectory = "/Volumes/OTRA"
	if alert := alertFor(input); alert != nil {
		t.Fatalf("se eligió otra carpeta: %+v", alert)
	}
}

// latestPast descarta los instantes «del futuro» (un reloj que estuvo adelantado).
func TestLatestPast(t *testing.T) {
	now := time.Date(2026, 11, 14, 12, 0, 0, 0, time.UTC)
	if got := latestPast(now, now.Add(-2*time.Hour), now.Add(5*24*time.Hour), time.Time{}); !got.Equal(now.Add(-2 * time.Hour)) {
		t.Fatalf("instante del futuro: %v", got)
	}
	if got := latestPast(now, now.Add(time.Minute)); !got.Equal(now.Add(time.Minute)) {
		t.Fatal("dentro de la tolerancia del reloj")
	}
	if got := latestPast(now); !got.IsZero() {
		t.Fatal("sin instantes")
	}
}
