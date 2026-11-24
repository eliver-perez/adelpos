package main

import (
	"context"
	"crypto/x509"
	"errors"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	"local/ad-el-pos/server/internal/administration"
	"local/ad-el-pos/server/internal/licensing"
	"local/ad-el-pos/server/internal/localsetup"
	"local/ad-el-pos/server/internal/product"
)

// Etapa L1, fase A: carga del gestor de la licencia al arrancar, en serve y en el host de macOS y de Windows
// (los tres pasan por serveHTTP).
//
// Va después de la identidad: con ella se carga (o se crea, una sola vez) la llave de licencias
// identity/license-key.pem, y el estado de la licencia vive en identity/licencia. Sin identidad o sin llave, la
// computadora principal sigue atendiendo: el gestor queda «Sin activar» con IdentityUnavailable y no escribe
// nada, y el problema queda en la bitácora.

// licenseLoadTimeout limita el arranque del gestor (la huella del equipo y dos consultas de solo lectura).
const licenseLoadTimeout = 15 * time.Second

// licenseOptions son las piezas del arranque del gestor; las pruebas las arman sin base.
type licenseOptions struct {
	identityDir string
	identity    *localsetup.ServerIdentity
	database    licensing.DatabaseProbe
	testMode    bool
}

// loadLicense prepara el gestor de la licencia y lee su estado. Nunca impide arrancar.
func loadLicense(ctx context.Context, logger *slog.Logger, options licenseOptions) *licensing.Manager {
	managerOptions := licensing.Options{
		Directory: filepath.Join(options.identityDir, localsetup.LicenseStateFolder),
		Database:  options.database,
		Logger:    logger,
	}
	if options.identity == nil {
		managerOptions.IdentityProblem = "la identidad del servidor no está disponible"
	} else {
		key, err := localsetup.EnsureLicenseKey(options.identityDir, licensing.ReadMachineFingerprint)
		if err != nil {
			managerOptions.IdentityProblem = err.Error()
			logger.Error("license_key_unavailable", "reason", err.Error())
		} else {
			managerOptions.Installation = &licensing.Installation{ServerID: options.identity.ServerID, PublicKey: key.PublicKey}
			managerOptions.PrivateKey = key.PrivateKey
		}
	}
	if options.testMode {
		applyLicenseTestHooks(logger, &managerOptions)
	}
	manager := licensing.NewManager(managerOptions)
	loadContext, cancel := context.WithTimeout(ctx, licenseLoadTimeout)
	defer cancel()
	if err := manager.Load(loadContext); err != nil {
		logger.Error("license_state_unavailable", "reason", err.Error())
	}
	return manager
}

// useLicense pone el gestor de la licencia en el servicio y conecta los cupos con los límites reales de esa licencia y el
// reloj del gestor (etapa L1, fase C; L1-D13 y L1-D16): Vigente, Tolerancia, Vencida o Revocada dan sus cupos; sin
// licencia válida, ninguno. Se hace antes de atender peticiones, en serve y en el host de macOS y de Windows.
func useLicense(application *administration.Service, manager *licensing.Manager) {
	application.License = manager
	application.Seats = administration.LicenseSeats(manager)
}

// applyLicenseTestHooks aplica, solo con test_mode y solo en una compilación de desarrollo, las ayudas del Go
// temporal de las pruebas (L1-D16 y L1-D18):
//
//   - ADEL_TEST_LICENSE_TRUST: ruta de un vectors.json del contrato; se confía además en sus llaves de prueba
//     del entorno «test», para firmar licencias en las pruebas de interfaz;
//   - ADEL_TEST_CLOCK_FILE: archivo con un desfase del reloj («-36h»), para simular un reloj atrasado sin
//     cambiar la hora de la computadora.
func applyLicenseTestHooks(logger *slog.Logger, options *licensing.Options) {
	if filename := os.Getenv("ADEL_TEST_LICENSE_TRUST"); filename != "" {
		trust, err := licensing.CompiledTrust()
		if err == nil {
			trust, err = trust.WithTestKeys(filename)
		}
		if err != nil {
			logger.Error("license_test_trust_ignored", "reason", err.Error())
		} else {
			options.Trust = &trust
			logger.Warn("license_test_trust_enabled", "keys", len(trust.Keys))
		}
	}
	if filename := os.Getenv("ADEL_TEST_CLOCK_FILE"); filename != "" {
		clock, err := licensing.OffsetClock(filename)
		if err != nil {
			logger.Error("license_test_clock_ignored", "reason", err.Error())
		} else {
			options.Now = clock
			logger.Warn("license_test_clock_enabled")
		}
	}
}

// newLicenseClient prepara el cliente del servidor de licencias (Fase B): la dirección de license_server_url o la
// del entorno compilado. Con test_mode solo habla con loopback, salvo ADEL_LICENSE_ALLOW_REMOTE=1 (L1-D18, solo la
// Fase G), y en una compilación de desarrollo confía además en la autoridad del servidor simulado de las pruebas
// (ADEL_TEST_LICENSE_CA, un PEM que escribe license-test server).
func newLicenseClient(logger *slog.Logger, manager *licensing.Manager, serverURL string, testMode bool) *licensing.Client {
	options := licensing.ClientOptions{
		BaseURL:    serverURL,
		AppVersion: product.Read().Version,
		TestMode:   testMode,
		Logger:     logger,
	}
	if testMode {
		options.AllowRemote = os.Getenv("ADEL_LICENSE_ALLOW_REMOTE") == "1"
		if options.AllowRemote {
			logger.Warn("license_remote_allowed_in_test_mode")
		}
		if filename := os.Getenv("ADEL_TEST_LICENSE_CA"); filename != "" {
			roots, err := testLicenseRoots(filename)
			if err != nil {
				logger.Error("license_test_ca_ignored", "reason", err.Error())
			} else {
				options.RootCAs = roots
				logger.Warn("license_test_ca_enabled")
			}
		}
	}
	return licensing.NewClient(manager, options)
}

// testLicenseRoots lee la autoridad del servidor de licencias simulado (solo desarrollo y test_mode).
func testLicenseRoots(filename string) (*x509.CertPool, error) {
	if !licensing.TestHooksAllowed() {
		return nil, errors.New("una versión de producción no confía en la autoridad de las pruebas")
	}
	raw, err := os.ReadFile(filename)
	if err != nil {
		return nil, errors.New("no se pudo leer la autoridad del servidor de licencias simulado")
	}
	roots := x509.NewCertPool()
	if !roots.AppendCertsFromPEM(raw) {
		return nil, errors.New("la autoridad del servidor de licencias simulado no es un PEM válido")
	}
	return roots, nil
}
