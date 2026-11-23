package main

import (
	"bytes"
	"crypto/ed25519"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"local/ad-el-pos/server/internal/licensing"
)

// testTool es la orden con una carpeta personal temporal (nunca ~/.adel-pos-dev) y las llaves de soporte de la carpeta
// de la prueba como las «incluidas en la versión».
func testTool(t *testing.T, home string, now time.Time) (*tool, *bytes.Buffer, *bytes.Buffer) {
	t.Helper()
	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	command := &tool{
		stdout: stdout,
		stderr: stderr,
		now:    func() time.Time { return now },
		home:   func() (string, error) { return home, nil },
		trust: func() (licensing.SupportTrust, error) {
			_, public := keyPaths(filepath.Join(home, ".adel-pos-dev", "soporte"), licensing.BuildEnvironment)
			raw, err := os.ReadFile(public)
			if err != nil {
				return licensing.SupportTrust{}, err
			}
			return licensing.ParseSupportKeys(raw, licensing.BuildEnvironment)
		},
	}
	return command, stdout, stderr
}

// privateMaterial son las formas en que la llave privada podría aparecer en una salida: el PEM, su contenido en
// base64 y la semilla.
func privateMaterial(t *testing.T, folder string) []string {
	t.Helper()
	privatePath, _ := keyPaths(folder, licensing.BuildEnvironment)
	raw, err := os.ReadFile(privatePath)
	if err != nil {
		t.Fatal(err)
	}
	block, _ := pem.Decode(raw)
	parsed, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		t.Fatal(err)
	}
	seed := parsed.(ed25519.PrivateKey).Seed()
	lines := strings.Split(strings.TrimSpace(string(raw)), "\n")
	return []string{
		"PRIVATE KEY",
		lines[1][:24],
		base64.StdEncoding.EncodeToString(seed)[:24],
		base64.RawURLEncoding.EncodeToString(seed)[:24],
	}
}

func requireNoPrivate(t *testing.T, material []string, outputs ...string) {
	t.Helper()
	for _, output := range outputs {
		for _, piece := range material {
			if strings.Contains(output, piece) {
				t.Fatalf("la salida muestra la llave privada:\n%s", output)
			}
		}
	}
}

// generar-llave crea la llave en la carpeta de soporte (0700 y 0600), agrega la pública al archivo de la versión,
// nunca muestra la privada y nunca reemplaza una llave existente.
func TestGenerateKey(t *testing.T) {
	home := t.TempDir()
	now := time.Date(2026, 11, 23, 18, 0, 0, 0, time.UTC)
	command, stdout, stderr := testTool(t, home, now)
	publicFile := filepath.Join(t.TempDir(), "soporte-"+licensing.BuildEnvironment+".pub.json")
	if err := os.WriteFile(publicFile, []byte(`{"environment":"`+licensing.BuildEnvironment+`","keys":[]}`), 0644); err != nil {
		t.Fatal(err)
	}
	if code := command.run([]string{"generar-llave", "--publica", publicFile}); code != 0 {
		t.Fatalf("generar-llave: %d %s", code, stderr)
	}
	folder := filepath.Join(home, ".adel-pos-dev", "soporte")
	privatePath, publicPath := keyPaths(folder, licensing.BuildEnvironment)
	if runtime.GOOS != "windows" {
		folderInfo, _ := os.Stat(folder)
		keyInfo, _ := os.Stat(privatePath)
		if folderInfo.Mode().Perm() != 0700 || keyInfo.Mode().Perm() != 0600 {
			t.Fatalf("permisos: carpeta %v, llave %v", folderInfo.Mode().Perm(), keyInfo.Mode().Perm())
		}
	}
	requireNoPrivate(t, privateMaterial(t, folder), stdout.String(), stderr.String())
	local, err := os.ReadFile(publicPath)
	if err != nil {
		t.Fatal(err)
	}
	version, err := os.ReadFile(publicFile)
	if err != nil {
		t.Fatal(err)
	}
	localTrust, errLocal := licensing.ParseSupportKeys(local, licensing.BuildEnvironment)
	versionTrust, errVersion := licensing.ParseSupportKeys(version, licensing.BuildEnvironment)
	if errLocal != nil || errVersion != nil || len(versionTrust.Keys) != 1 {
		t.Fatalf("archivos de la pública: %v %v", errLocal, errVersion)
	}
	for kid, public := range localTrust.Keys {
		if !versionTrust.Keys[kid].Equal(public) || kid != "adelpos-soporte-"+licensing.BuildEnvironment+"-20261123" {
			t.Fatalf("la pública agregada no es la de la llave: %s", kid)
		}
	}
	// Nunca se reemplaza.
	before, _ := os.ReadFile(privatePath)
	if code := command.run([]string{"generar-llave"}); code == 0 {
		t.Fatal("una segunda llave del mismo entorno no se crea")
	}
	after, _ := os.ReadFile(privatePath)
	if !bytes.Equal(before, after) {
		t.Fatal("la llave existente cambió")
	}
}

// responder firma una respuesta que el servidor acepta con la pública incluida en la versión, sin mostrar la privada;
// con un código mal dictado o una llave que no es la de la versión, no entrega nada.
func TestRespond(t *testing.T) {
	home := t.TempDir()
	now := time.Date(2026, 11, 23, 18, 0, 0, 0, time.UTC)
	command, stdout, stderr := testTool(t, home, now)
	if code := command.run([]string{"generar-llave"}); code != 0 {
		t.Fatalf("generar-llave: %s", stderr)
	}
	stdout.Reset()
	tag := licensing.SupportInstallationTag("22222222-2222-4222-8222-222222222222")
	code, err := licensing.EncodeSupportCode(tag, "0a0b0c0d0e", now.Add(3*time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	output := filepath.Join(t.TempDir(), "respuesta.desbloqueo")
	if exit := command.run([]string{"responder", strings.ToLower(code), "--hora", "2026-11-23T17:55:00Z", "--salida", output}); exit != 0 {
		t.Fatalf("responder: %d %s", exit, stderr)
	}
	text := stdout.String()
	requireNoPrivate(t, privateMaterial(t, filepath.Join(home, ".adel-pos-dev", "soporte")), text, stderr.String())
	lines := strings.Split(strings.TrimSpace(text), "\n")
	response := lines[len(lines)-1]
	trust, _ := command.trust()
	content, newTrusted, err := trust.VerifySupportResponse(response)
	if err != nil {
		t.Fatalf("la respuesta no verifica: %v", err)
	}
	if content.RequestCode != code || content.InstallationTag != tag || content.RequestNonce != "0a0b0c0d0e" ||
		!newTrusted.Equal(time.Date(2026, 11, 23, 17, 55, 0, 0, time.UTC)) {
		t.Fatalf("contenido: %+v", content)
	}
	saved, err := os.ReadFile(output)
	if err != nil || strings.TrimSpace(string(saved)) != response {
		t.Fatal("el archivo .desbloqueo no tiene la respuesta")
	}
	if !strings.Contains(text, code) || !strings.Contains(text, "una sola vez") {
		t.Fatalf("la salida explica la respuesta:\n%s", text)
	}

	// Un carácter mal dictado (el último es parte de la verificación): no se firma nada.
	stdout.Reset()
	last := code[len(code)-1]
	replacement := byte('0')
	if last == '0' {
		replacement = '1'
	}
	broken := code[:len(code)-1] + string(replacement)
	if exit := command.run([]string{"responder", broken}); exit == 0 || strings.Contains(stdout.String(), "Respuesta") {
		t.Fatalf("un código mal dictado no se responde: %s", stdout)
	}

	// Una llave que no es la pública de la versión: se avisa en lugar de entregar una respuesta inútil.
	stdout.Reset()
	command.trust = func() (licensing.SupportTrust, error) {
		public, _, _ := ed25519.GenerateKey(nil)
		return licensing.SupportTrust{Environment: licensing.BuildEnvironment,
			Keys: map[string]ed25519.PublicKey{"otra": public}}, nil
	}
	if exit := command.run([]string{"responder", code}); exit == 0 || strings.Contains(stdout.String(), "Respuesta") {
		t.Fatalf("con otra llave en la versión no se entrega respuesta: %s", stdout)
	}
	if !strings.Contains(stderr.String(), "no es la pública incluida") {
		t.Fatalf("el error lo explica: %s", stderr)
	}
}

// Sin llave, sin código o con una orden desconocida, la orden lo dice y termina con error.
func TestUsageErrors(t *testing.T) {
	command, _, stderr := testTool(t, t.TempDir(), time.Now())
	for _, arguments := range [][]string{{}, {"desconocida"}, {"responder"}, {"responder", "0000-0000-0000-0000-0000-0000"}} {
		stderr.Reset()
		if command.run(arguments) == 0 {
			t.Errorf("%v debía terminar con error", arguments)
		}
		if stderr.Len() == 0 {
			t.Errorf("%v no explica el error", arguments)
		}
	}
}
