//go:build windows

// Package standalone opera la instalación autónoma de Windows (etapa 9E): la aplicación instalada lanza
// el servidor en modo «host», que en la carpeta de datos del usuario crea el clúster de PostgreSQL con los
// binarios incluidos en el instalador, lo arranca, prepara la base y la configuración, respalda antes de
// migrar y lo detiene al terminar. En macOS nada de esto existe: PostgreSQL y Go se administran con
// scripts/dev.sh y la aplicación nunca los inicia.
//
// Reglas que conserva de la instalación de macOS: un clúster exclusivo con marcador de propiedad (nunca se
// opera sobre uno ajeno), solo TCP en 127.0.0.1, autenticación SCRAM, zona UTC y locale C en la base,
// secretos en archivos privados que no se muestran, y respaldos pg_dump en formato custom con su
// comprobación.
package standalone

import (
	"errors"
	"os"
	"path/filepath"
)

const (
	// ServerAddress y ClusterPort son los mismos de la instalación de macOS: Go en 8181 y PostgreSQL en
	// 55432, ambos solo en loopback (sin reglas de firewall).
	ServerAddress = "127.0.0.1:8181"
	ClusterPort   = 55432
	// superuser administra el clúster (crear el rol y la base, respaldos de prueba); appRole es el rol sin
	// privilegios con el que corre el servidor, como en macOS.
	superuser = "adel_admin"
	appRole   = "adel_app"
)

// Layout son las rutas de una instalación: la carpeta de datos (nunca la de instalación) y la carpeta de
// los binarios de PostgreSQL que trae el instalador.
type Layout struct {
	DataDir string
	PGBin   string
	// Port es el del clúster: ClusterPort en la instalación; las pruebas usan otro para no chocar con el
	// clúster de desarrollo.
	Port int
}

// NewLayout completa las rutas que no se indicaron: datos en %LOCALAPPDATA%\<producto> y PostgreSQL en
// la carpeta «postgresql\bin» junto al ejecutable del servidor.
func NewLayout(dataDir, pgBin, productName string) (Layout, error) {
	if dataDir == "" {
		base := os.Getenv("LOCALAPPDATA")
		if base == "" {
			cache, err := os.UserCacheDir()
			if err != nil {
				return Layout{}, errors.New("no se encontró la carpeta de datos local del usuario")
			}
			base = cache
		}
		dataDir = filepath.Join(base, productName)
	}
	if pgBin == "" {
		executable, err := os.Executable()
		if err != nil {
			return Layout{}, errors.New("no se encontró la carpeta del programa")
		}
		pgBin = filepath.Join(filepath.Dir(executable), "postgresql", "bin")
	}
	// Forma larga y absoluta: el marcador del clúster y las comparaciones de procesos no dependen de cómo
	// llegó la ruta (corta, relativa o con otras mayúsculas).
	return Layout{DataDir: canonicalPath(dataDir), PGBin: canonicalPath(pgBin), Port: ClusterPort}, nil
}

func (layout Layout) Cluster() string {
	return filepath.Join(layout.DataDir, "postgres")
}

func (layout Layout) ConfigFile() string {
	return filepath.Join(layout.DataDir, "server.json")
}

func (layout Layout) AdminPassword() string {
	return filepath.Join(layout.DataDir, "admin-password")
}

func (layout Layout) Marker() string {
	return filepath.Join(layout.DataDir, "cluster-owner.json")
}

// Provisioning es el token del asistente inicial; la aplicación lo lee de aquí (etapa 9E).
func (layout Layout) Provisioning() string {
	return filepath.Join(layout.DataDir, "provisioning-token")
}

func (layout Layout) Logs() string {
	return filepath.Join(layout.DataDir, "logs")
}

func (layout Layout) ServerLog() string {
	return filepath.Join(layout.Logs(), "server.log")
}

func (layout Layout) PostgresLog() string {
	return filepath.Join(layout.Logs(), "postgres.log")
}

// MaintenanceLog es la bitácora de los respaldos y restauraciones que se piden desde la consola.
func (layout Layout) MaintenanceLog() string {
	return filepath.Join(layout.Logs(), "maintenance.log")
}

func (layout Layout) ControlLog() string {
	return filepath.Join(layout.Logs(), "pg_ctl.log")
}

func (layout Layout) Backups() string {
	return filepath.Join(layout.DataDir, "backups")
}

func (layout Layout) lockFile() string {
	return filepath.Join(layout.DataDir, "host.lock")
}

func (layout Layout) tool(name string) string {
	return filepath.Join(layout.PGBin, name+".exe")
}
