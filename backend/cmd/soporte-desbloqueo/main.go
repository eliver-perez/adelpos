// Orden soporte-desbloqueo: la herramienta del propietario de AD-EL POS para el desbloqueo de soporte sin internet
// (§6.4 de la especificación de la etapa L1, decisión L1-D19; procedimiento en docs/licencias/DESBLOQUEO_SOPORTE.md).
//
//	soporte-desbloqueo generar-llave [--entorno development] [--carpeta DIR] [--publica ARCHIVO]
//	soporte-desbloqueo responder CODIGO [--hora AAAA-MM-DDTHH:MM:SSZ] [--entorno development] [--carpeta DIR] [--salida ARCHIVO]
//
// La llave privada de soporte vive solo en la carpeta de soporte del propietario (por omisión ~/.adel-pos-dev/soporte,
// 0700, con la llave en 0600). Esta orden nunca la muestra ni la copia a otra parte: solo escribe la pública (que se
// incluye al compilar, config/licencias/soporte-<entorno>.pub.json) y las respuestas firmadas. Los instaladores no
// incluyen esta orden: solo compilan ./cmd/ad-el-pos-server.
package main

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"local/ad-el-pos/server/internal/licensing"
)

func main() {
	command := tool{
		stdout: os.Stdout,
		stderr: os.Stderr,
		now:    time.Now,
		trust:  licensing.CompiledSupportTrust,
		home:   os.UserHomeDir,
	}
	os.Exit(command.run(os.Args[1:]))
}

// tool es la orden con sus dependencias (las pruebas inyectan el reloj, la carpeta personal y las llaves de soporte
// en que confía la versión).
type tool struct {
	stdout io.Writer
	stderr io.Writer
	now    func() time.Time
	trust  func() (licensing.SupportTrust, error)
	home   func() (string, error)
}

const usage = `Uso:
  soporte-desbloqueo generar-llave [--entorno development] [--carpeta DIR] [--publica ARCHIVO]
      Crea la llave de soporte del entorno en la carpeta de soporte (por omisión ~/.adel-pos-dev/soporte).
      Con --publica agrega su llave pública a ese archivo (config/licencias/soporte-<entorno>.pub.json).
  soporte-desbloqueo responder CODIGO [--hora AAAA-MM-DDTHH:MM:SSZ] [--entorno development] [--carpeta DIR] [--salida ARCHIVO]
      Firma la respuesta a un código de solicitud. --hora es la nueva última hora confiable (por omisión, la hora
      actual de esta computadora). Con --salida, además la guarda como archivo .desbloqueo.
`

func (command tool) run(arguments []string) int {
	if len(arguments) == 0 {
		fmt.Fprint(command.stderr, usage)
		return 2
	}
	var err error
	switch arguments[0] {
	case "generar-llave":
		err = command.generateKey(arguments[1:])
	case "responder":
		err = command.respond(arguments[1:])
	case "ayuda", "-h", "--help":
		fmt.Fprint(command.stdout, usage)
		return 0
	default:
		fmt.Fprint(command.stderr, usage)
		return 2
	}
	if err != nil {
		fmt.Fprintln(command.stderr, "Error:", err)
		return 1
	}
	return 0
}

// keyPaths son los archivos de la llave de un entorno en la carpeta de soporte.
func keyPaths(folder, environment string) (private, public string) {
	return filepath.Join(folder, "soporte-"+environment+".key"), filepath.Join(folder, "soporte-"+environment+".pub.json")
}

func (command tool) defaultFolder() (string, error) {
	home, err := command.home()
	if err != nil || home == "" {
		return "", errors.New("no se pudo encontrar la carpeta personal; indica --carpeta")
	}
	return filepath.Join(home, ".adel-pos-dev", "soporte"), nil
}

func validEnvironment(environment string) error {
	if environment != "development" && environment != "production" {
		return errors.New("el entorno debe ser development o production")
	}
	return nil
}

func (command tool) generateKey(arguments []string) error {
	flags := flag.NewFlagSet("generar-llave", flag.ContinueOnError)
	flags.SetOutput(command.stderr)
	environment := flags.String("entorno", licensing.BuildEnvironment, "development o production")
	folder := flags.String("carpeta", "", "carpeta de soporte (por omisión ~/.adel-pos-dev/soporte)")
	publicFile := flags.String("publica", "", "archivo de llaves públicas donde agregar la nueva")
	if err := flags.Parse(arguments); err != nil {
		return err
	}
	if flags.NArg() > 0 {
		return errors.New("generar-llave no recibe argumentos sueltos")
	}
	if err := validEnvironment(*environment); err != nil {
		return err
	}
	if *folder == "" {
		value, err := command.defaultFolder()
		if err != nil {
			return err
		}
		*folder = value
	}
	privatePath, publicPath := keyPaths(*folder, *environment)
	if _, err := os.Stat(privatePath); err == nil {
		return fmt.Errorf("ya existe una llave de soporte de %s en %s; no se reemplaza", *environment, privatePath)
	}
	// La carpeta es solo del dueño (0700), aunque ya existiera.
	if err := os.MkdirAll(*folder, 0700); err != nil {
		return errors.New("no se pudo crear la carpeta de soporte")
	}
	if err := os.Chmod(*folder, 0700); err != nil {
		return errors.New("no se pudo proteger la carpeta de soporte")
	}
	public, private, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return errors.New("no se pudo generar la llave")
	}
	encoded, err := x509.MarshalPKCS8PrivateKey(private)
	if err != nil {
		return errors.New("no se pudo preparar la llave")
	}
	// O_EXCL: nunca reemplaza una llave que apareció mientras tanto. 0600: solo el dueño la lee.
	file, err := os.OpenFile(privatePath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return errors.New("no se pudo crear el archivo de la llave")
	}
	writeErr := pem.Encode(file, &pem.Block{Type: "PRIVATE KEY", Bytes: encoded})
	syncErr := file.Sync()
	closeErr := file.Close()
	if writeErr != nil || syncErr != nil || closeErr != nil {
		_ = os.Remove(privatePath)
		return errors.New("no se pudo guardar la llave")
	}
	kid := "adelpos-soporte-" + *environment + "-" + command.now().UTC().Format("20060102")
	record := licensing.SupportKeyRecord{Kid: kid, PublicKey: licensing.EncodeSupportPublicKey(public)}
	if err := writeKeyFile(publicPath, licensing.SupportKeyFile{Environment: *environment, Keys: []licensing.SupportKeyRecord{record}}); err != nil {
		return err
	}
	fmt.Fprintf(command.stdout, "Llave de soporte de %s creada.\n  kid: %s\n  pública: %s\n  privada: %s (no se muestra ni se comparte)\n",
		*environment, kid, record.PublicKey, privatePath)
	if *publicFile != "" {
		if err := addPublicKey(*publicFile, *environment, record); err != nil {
			return err
		}
		fmt.Fprintf(command.stdout, "Llave pública agregada a %s. Ejecuta node scripts/sync-metadata.mjs para incluirla al compilar.\n", *publicFile)
	}
	return nil
}

// addPublicKey agrega la pública al archivo de llaves de la versión (lo crea si falta). Conserva las que ya tenga,
// para poder rotar la llave sin que dejen de valer las respuestas de la anterior.
func addPublicKey(filename, environment string, record licensing.SupportKeyRecord) error {
	keys := licensing.SupportKeyFile{Environment: environment}
	raw, err := os.ReadFile(filename)
	switch {
	case err == nil:
		if err := json.Unmarshal(raw, &keys); err != nil || keys.Environment != environment {
			return fmt.Errorf("%s no es un archivo de llaves de soporte de %s", filename, environment)
		}
	case !errors.Is(err, os.ErrNotExist):
		return fmt.Errorf("no se pudo leer %s", filename)
	}
	for _, existing := range keys.Keys {
		if existing.Kid == record.Kid {
			return fmt.Errorf("%s ya tiene el kid %s", filename, record.Kid)
		}
	}
	keys.Keys = append(keys.Keys, record)
	return writeKeyFile(filename, keys)
}

func writeKeyFile(filename string, keys licensing.SupportKeyFile) error {
	body, err := json.MarshalIndent(keys, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(filename, append(body, '\n'), 0644); err != nil {
		return fmt.Errorf("no se pudo escribir %s", filename)
	}
	return nil
}

// loadKey lee la llave privada y su pública de la carpeta de soporte, y comprueba que correspondan.
func loadKey(folder, environment string) (ed25519.PrivateKey, licensing.SupportKeyRecord, error) {
	privatePath, publicPath := keyPaths(folder, environment)
	info, err := os.Stat(privatePath)
	if err != nil {
		return nil, licensing.SupportKeyRecord{}, fmt.Errorf("no hay llave de soporte de %s en %s; créala con generar-llave", environment, folder)
	}
	if info.Mode().Perm()&0077 != 0 {
		return nil, licensing.SupportKeyRecord{}, fmt.Errorf("la llave %s la pueden leer otros usuarios; deja sus permisos en 0600", privatePath)
	}
	raw, err := os.ReadFile(privatePath)
	if err != nil {
		return nil, licensing.SupportKeyRecord{}, errors.New("no se pudo leer la llave de soporte")
	}
	block, _ := pem.Decode(raw)
	if block == nil || block.Type != "PRIVATE KEY" {
		return nil, licensing.SupportKeyRecord{}, errors.New("el archivo de la llave de soporte no es válido")
	}
	parsed, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	private, isEd25519 := parsed.(ed25519.PrivateKey)
	if err != nil || !isEd25519 {
		return nil, licensing.SupportKeyRecord{}, errors.New("el archivo de la llave de soporte no es una llave Ed25519")
	}
	publicRaw, err := os.ReadFile(publicPath)
	if err != nil {
		return nil, licensing.SupportKeyRecord{}, fmt.Errorf("falta %s", publicPath)
	}
	trust, err := licensing.ParseSupportKeys(publicRaw, environment)
	if err != nil || len(trust.Keys) != 1 {
		return nil, licensing.SupportKeyRecord{}, fmt.Errorf("%s no es válido", publicPath)
	}
	for kid, public := range trust.Keys {
		if !public.Equal(private.Public()) {
			return nil, licensing.SupportKeyRecord{}, fmt.Errorf("%s no corresponde a la llave privada", publicPath)
		}
		return private, licensing.SupportKeyRecord{Kid: kid, PublicKey: licensing.EncodeSupportPublicKey(public)}, nil
	}
	return nil, licensing.SupportKeyRecord{}, errors.New("sin llave pública")
}

func (command tool) respond(arguments []string) error {
	flags := flag.NewFlagSet("responder", flag.ContinueOnError)
	flags.SetOutput(command.stderr)
	environment := flags.String("entorno", licensing.BuildEnvironment, "development o production")
	folder := flags.String("carpeta", "", "carpeta de soporte (por omisión ~/.adel-pos-dev/soporte)")
	hour := flags.String("hora", "", "nueva última hora confiable (por omisión, la hora actual)")
	output := flags.String("salida", "", "archivo .desbloqueo donde guardar la respuesta")
	// El código va primero y las opciones después (o al revés): se separan antes de leer las opciones.
	codeParts, options := splitArguments(arguments)
	if err := flags.Parse(options); err != nil {
		return err
	}
	codeParts = append(codeParts, flags.Args()...)
	if len(codeParts) == 0 {
		return errors.New("falta el código de solicitud")
	}
	if err := validEnvironment(*environment); err != nil {
		return err
	}
	code, err := licensing.ParseSupportCode(strings.Join(codeParts, ""))
	if err != nil {
		return err
	}
	now := command.now().UTC()
	newTrustedTime := now.Truncate(time.Second)
	if *hour != "" {
		parsed, err := time.Parse(time.RFC3339, *hour)
		if err != nil {
			return errors.New("--hora debe tener la forma AAAA-MM-DDTHH:MM:SSZ")
		}
		newTrustedTime = parsed.UTC()
	}
	if *folder == "" {
		value, err := command.defaultFolder()
		if err != nil {
			return err
		}
		*folder = value
	}
	private, record, err := loadKey(*folder, *environment)
	if err != nil {
		return err
	}
	response, err := licensing.SignSupportResponse(private, record.Kid, *environment, code, newTrustedTime, now)
	if err != nil {
		return err
	}
	// La respuesta debe poder verificarla la versión del restaurante: si la llave no es la pública que se incluyó al
	// compilar (config/licencias/soporte-<entorno>.pub.json), se avisa en lugar de entregar una respuesta inútil.
	if *environment == licensing.BuildEnvironment {
		trust, err := command.trust()
		if err != nil {
			return fmt.Errorf("esta versión no tiene llaves de soporte (%v)", err)
		}
		if _, _, err := trust.VerifySupportResponse(response); err != nil {
			return errors.New("la llave de soporte no es la pública incluida en esta versión (config/licencias/soporte-" +
				*environment + ".pub.json)")
		}
	}
	fmt.Fprintf(command.stdout, "Solicitud: %s\n  instalación: %s\n", code.Text, code.InstallationTag)
	if !code.TrustedTime.IsZero() {
		fmt.Fprintf(command.stdout, "  última actividad registrada en la solicitud: %s (UTC)\n", code.TrustedTime.Format("2006-01-02 15:04"))
	}
	fmt.Fprintf(command.stdout, "  nueva última hora confiable: %s (UTC)\n", newTrustedTime.Format("2006-01-02 15:04:05"))
	if newTrustedTime.After(now.Add(licensing.ClockMargin)) {
		fmt.Fprintln(command.stdout, "  Aviso: la nueva hora es posterior a la hora actual de esta computadora.")
	}
	fmt.Fprintln(command.stdout, "  La respuesta vale una sola vez, en esa instalación y dentro de las 24 horas de la solicitud.")
	if *output != "" {
		if err := os.WriteFile(*output, []byte(response+"\n"), 0600); err != nil {
			return fmt.Errorf("no se pudo escribir %s", *output)
		}
		fmt.Fprintf(command.stdout, "  Archivo para importar: %s\n", *output)
	}
	fmt.Fprintf(command.stdout, "Respuesta (pégala en «Desbloqueo de soporte»):\n%s\n", response)
	return nil
}

// splitArguments separa las partes del código (lo que no empieza con «-») de las opciones con su valor.
func splitArguments(arguments []string) (code, options []string) {
	for index := 0; index < len(arguments); index++ {
		argument := arguments[index]
		if strings.HasPrefix(argument, "-") {
			options = append(options, argument)
			if !strings.Contains(argument, "=") && index+1 < len(arguments) {
				options = append(options, arguments[index+1])
				index++
			}
			continue
		}
		code = append(code, argument)
	}
	return code, options
}
