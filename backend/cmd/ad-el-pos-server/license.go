package main

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	"local/ad-el-pos/server/internal/licensing"
	"local/ad-el-pos/server/internal/localsetup"
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
		key, err := localsetup.EnsureLicenseKey(options.identityDir)
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
