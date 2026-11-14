package backupschedule

import (
	"context"
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

func TestFileNames(t *testing.T) {
	instant := time.Date(2026, 11, 14, 11, 0, 5, 0, time.UTC)
	if name := FileName("adel_pos", KindAutomatic, instant, "a1b2c3"); name != "adel_pos-20261114T110005Z-a1b2c3-automatico.dump" {
		t.Fatalf("automático: %s", name)
	}
	if name := FileName("adel_pos", KindManual, instant, "a1b2c3"); name != "adel_pos-20261114T110005Z-a1b2c3-manual.dump" {
		t.Fatalf("manual: %s", name)
	}
	if !automaticPattern("adel_pos").MatchString(FileName("adel_pos", KindAutomatic, instant, "a1b2c3")) {
		t.Fatal("el patrón no reconoce el nombre automático")
	}
}

// La retención conserva los N automáticos más recientes y solo borra archivos regulares con el nombre exacto de un
// respaldo automático de esta base: nunca manuales, de la línea de comandos, de antes de migrar, de otra base, a
// medias, enlaces, carpetas ni otros archivos.
func TestRetainOnlyRemovesOwnAutomaticFiles(t *testing.T) {
	directory := t.TempDir()
	automatic := []string{
		"adel_pos-20261110T110000Z-aaaaaa-automatico.dump",
		"adel_pos-20261111T110000Z-bbbbbb-automatico.dump",
		"adel_pos-20261112T110000Z-cccccc-automatico.dump",
		"adel_pos-20261113T110000Z-dddddd-automatico.dump",
		"adel_pos-20261114T110000Z-eeeeee-automatico.dump",
	}
	others := []string{
		"adel_pos-20261101T110000Z-ffffff-manual.dump",
		"adel_pos-20261101T110000Z-ffffff.dump",
		"adel_pos-20261101T110000Z-ffffff-antes-de-migrar.dump",
		"adel_pos-20261101T110000Z-ffffff-antes-de-quitar-datos-de-prueba.dump",
		"otra_base-20261101T110000Z-ffffff-automatico.dump",
		"adel_pos_test-20261101T110000Z-ffffff-automatico.dump",
		"adel_pos-20261101T110000Z-ffffff-automatico.dump.partial",
		"adel_pos-20261101T110000Z-FFFFFF-automatico.dump",
		"adel_pos-20261101T110000Z-ffffff-automatico.dump.bak",
		"copia adel_pos-20261101T110000Z-ffffff-automatico.dump",
		"notas.txt",
	}
	for _, name := range append(slices.Clone(automatic), others...) {
		touch(t, directory, name)
	}
	// Una carpeta y un enlace con el nombre de un respaldo automático viejo tampoco se tocan.
	if err := os.Mkdir(filepath.Join(directory, "adel_pos-20261001T110000Z-111111-automatico.dump"), 0700); err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS != "windows" {
		target := filepath.Join(t.TempDir(), "fuera.dump")
		touch(t, filepath.Dir(target), filepath.Base(target))
		if err := os.Symlink(target, filepath.Join(directory, "adel_pos-20261002T110000Z-222222-automatico.dump")); err != nil {
			t.Fatal(err)
		}
	}
	removed, err := Retain(directory, "adel_pos", 3)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(removed, automatic[:2]) {
		t.Fatalf("borrados: %v", removed)
	}
	left := names(t, directory)
	for _, name := range append(slices.Clone(automatic[2:]), others...) {
		if !slices.Contains(left, name) {
			t.Fatalf("se borró %s", name)
		}
	}
	for _, name := range automatic[:2] {
		if slices.Contains(left, name) {
			t.Fatalf("no se borró %s", name)
		}
	}
	// Con menos archivos que el límite no se borra nada.
	again, err := Retain(directory, "adel_pos", 30)
	if err != nil || len(again) != 0 {
		t.Fatalf("retención con límite mayor: %v %v", again, err)
	}
	if _, err := Retain(directory, "adel_pos", 0); err == nil {
		t.Fatal("se aceptó conservar cero respaldos")
	}
}

func TestCleanPartialsOnlyRemovesOwnLeftovers(t *testing.T) {
	directory := t.TempDir()
	touch(t, directory, "adel_pos-20261114T110000Z-aaaaaa-automatico.dump.partial")
	touch(t, directory, "adel_pos-20261114T110000Z-bbbbbb-manual.dump.partial")
	touch(t, directory, "adel_pos-20261114T110000Z-cccccc-automatico.dump")
	touch(t, directory, "otro.dump.partial")
	CleanPartials(directory, "adel_pos")
	left := names(t, directory)
	if !slices.Equal(left, []string{"adel_pos-20261114T110000Z-cccccc-automatico.dump", "otro.dump.partial"}) {
		t.Fatalf("quedaron: %v", left)
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

func TestValidateDirectory(t *testing.T) {
	directory := t.TempDir()
	clean, err := ValidateDirectory(directory + string(filepath.Separator) + ".")
	if err != nil || clean != directory {
		t.Fatalf("carpeta válida: %q %v", clean, err)
	}
	postgres := filepath.Join(directory, "postgres")
	if err := os.Mkdir(postgres, 0700); err != nil {
		t.Fatal(err)
	}
	touch(t, postgres, "PG_VERSION")
	cases := map[string]string{
		"":                                    ReasonInvalid,
		"respaldos":                           ReasonRelative,
		"./respaldos":                         ReasonRelative,
		filepath.Join(directory, "no-existe"): ReasonMissing,
		directory + "/linea\nnueva":           ReasonInvalid,
		postgres:                              ReasonPostgresData,
		string(make([]byte, maxDirectoryLength+1)): ReasonInvalid,
	}
	for input, reason := range cases {
		_, err := ValidateDirectory(input)
		problem, ok := err.(*DirectoryError)
		if !ok || problem.Reason != reason {
			t.Fatalf("%q: %v, esperado %s", input, err, reason)
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

// Fallo de verificación: el respaldo que no se puede leer no se conserva (ni con su nombre definitivo ni a medias) y
// el resultado es «verify_failed»; un respaldo correcto queda verificado con su nombre definitivo.
func TestProduceVerifiesBeforeKeepingTheFile(t *testing.T) {
	root := t.TempDir()
	target := newFakeTarget(filepath.Join(root, "respaldos"))
	runner := &Runner{Target: target, Now: func() time.Time {
		return time.Date(2026, 11, 14, 11, 0, 0, 0, time.UTC)
	}}
	result := runner.produce(context.Background(), Run{Kind: KindAutomatic}, Settings{Keep: 30})
	if result.failure != "" || !result.verified || result.size == 0 {
		t.Fatalf("respaldo correcto: %+v", result)
	}
	if !automaticPattern(target.database).MatchString(result.fileName) {
		t.Fatalf("nombre: %s", result.fileName)
	}
	if left := names(t, target.directory); !slices.Equal(left, []string{result.fileName}) {
		t.Fatalf("archivos: %v", left)
	}
	if !slices.Contains(target.verified, result.fileName+partialSuffix) {
		t.Fatalf("se verificó antes de darle su nombre definitivo: %v", target.verified)
	}
	target.set(func(target *fakeTarget) {
		target.failVerify = true
	})
	failed := runner.produce(context.Background(), Run{Kind: KindManual}, Settings{Keep: 30})
	if failed.failure != FailureVerify || failed.verified || failed.fileName != "" {
		t.Fatalf("verificación fallida: %+v", failed)
	}
	if left := names(t, target.directory); !slices.Equal(left, []string{result.fileName}) {
		t.Fatalf("quedó el respaldo que no se pudo verificar: %v", left)
	}
	target.set(func(target *fakeTarget) {
		target.failVerify = false
		target.failDump = true
	})
	if dump := runner.produce(context.Background(), Run{Kind: KindManual}, Settings{Keep: 30}); dump.failure != FailureDump {
		t.Fatalf("pg_dump fallido: %+v", dump)
	}
	// Interrumpido: el contexto del trabajador se canceló (el servidor se apaga).
	target.set(func(target *fakeTarget) {
		target.failDump = false
		target.block = make(chan struct{})
	})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if interrupted := runner.produce(ctx, Run{Kind: KindAutomatic}, Settings{Keep: 30}); interrupted.failure != FailureInterrupted {
		t.Fatalf("interrumpido: %+v", interrupted)
	}
	if left := names(t, target.directory); len(left) != 1 {
		t.Fatalf("quedaron restos: %v", left)
	}
}

// El aviso: un fallo avisa hasta que haya un respaldo correcto posterior; un respaldo en la predeterminada porque la
// elegida no estaba avisa mientras esa siga siendo la carpeta configurada.
func TestAlertFor(t *testing.T) {
	started := time.Date(2026, 11, 14, 11, 0, 0, 0, time.UTC)
	first := started.Add(-24 * time.Hour)
	failed := Run{Status: StatusFailed, FailureReason: FailureVerify, StartedAt: started}
	alert := alertFor(failed, 2, first, "")
	if alert == nil || alert.Kind != "failed" || alert.Reason != FailureVerify || alert.Failures != 2 || !alert.Since.Equal(first) {
		t.Fatalf("fallo: %+v", alert)
	}
	fallback := Run{Status: StatusSucceeded, StartedAt: started, UsedDefault: true, FallbackReason: ReasonMissing,
		RequestedDirectory: "/Volumes/USB/Respaldos"}
	if alert := alertFor(fallback, 0, time.Time{}, "/Volumes/USB/Respaldos"); alert == nil || alert.Kind != "fallback" {
		t.Fatalf("predeterminada por falta de la elegida: %+v", alert)
	}
	if alert := alertFor(fallback, 0, time.Time{}, ""); alert != nil {
		t.Fatalf("se volvió a la predeterminada: %+v", alert)
	}
	if alert := alertFor(fallback, 0, time.Time{}, "/Volumes/OTRA"); alert != nil {
		t.Fatalf("se eligió otra carpeta: %+v", alert)
	}
	if alert := alertFor(Run{Status: StatusSucceeded, StartedAt: started}, 0, time.Time{}, ""); alert != nil {
		t.Fatalf("respaldo correcto: %+v", alert)
	}
}
