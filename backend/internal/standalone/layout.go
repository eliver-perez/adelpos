// Package standalone opera el servidor de una instalación (sin scripts de desarrollo): el comando «host»
// crea en la carpeta de datos el clúster de PostgreSQL, lo arranca, prepara la base y la configuración,
// respalda antes de migrar, sirve y al terminar detiene PostgreSQL.
//
//   - Windows (etapa 9E): la aplicación instalada lanza el servidor con los binarios de PostgreSQL que
//     trae el instalador y la carpeta de datos es %LOCALAPPDATA%\<producto>.
//   - macOS (etapa 9G, fase A): el servidor es un servicio del sistema (LaunchDaemon) que corre con la
//     cuenta del usuario que instaló, con los datos en /Library/Application Support/<producto> y el
//     PostgreSQL instalado en el equipo (postgres_bin, ADEL_PG_BIN o Homebrew).
//
// Reglas que conserva de la instalación de desarrollo: un clúster exclusivo con marcador de propiedad
// (nunca se opera sobre uno ajeno), solo TCP en 127.0.0.1, autenticación SCRAM, zona UTC y locale C en la
// base, secretos en archivos privados que no se muestran, y respaldos pg_dump en formato custom con su
// comprobación.
package standalone

import (
	"path/filepath"
)

const (
	// ServerAddress y ClusterPort son los de siempre: Go en 8181 y PostgreSQL en 55432, ambos solo en
	// loopback (sin reglas de firewall).
	ServerAddress = "127.0.0.1:8181"
	ClusterPort   = 55432
	// superuser administra el clúster (crear el rol y la base, respaldos de prueba); appRole es el rol sin
	// privilegios con el que corre el servidor, como en macOS.
	superuser = "adel_admin"
	appRole   = "adel_app"
)

// Layout son las rutas de una instalación: la carpeta de datos (nunca la de instalación) y la carpeta de
// los binarios de PostgreSQL.
type Layout struct {
	DataDir string
	PGBin   string
	// Port es el del clúster: ClusterPort en la instalación; las pruebas usan otro para no chocar con el
	// clúster de desarrollo.
	Port int
	// Listen es la dirección de Go que se escribe en server.json la primera vez (ServerAddress si está
	// vacía). Solo las pruebas la cambian; una configuración existente nunca se reescribe.
	Listen string
}

// NewLayout completa las rutas que no se indicaron con las de la instalación de cada sistema
// (layout_windows.go y layout_unix.go).
func NewLayout(dataDir, pgBin, productName string) (Layout, error) {
	return newPlatformLayout(dataDir, pgBin, productName)
}

func (layout Layout) serverAddress() string {
	if layout.Listen != "" {
		return layout.Listen
	}
	return ServerAddress
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

// ModeFile guarda la modalidad de la computadora del lado del servidor (etapa 9G, mode.go).
func (layout Layout) ModeFile() string {
	return filepath.Join(layout.DataDir, ModeFileName)
}

// EnableFile es el archivo de habilitación del servicio de macOS: mientras existe, launchd mantiene vivo
// el servidor (KeepAlive → PathState del plist) y el servidor sigue sirviendo (mode.go).
func (layout Layout) EnableFile() string {
	return filepath.Join(layout.DataDir, EnableFileName)
}

// AdoptionMarker registra el paso de los datos de una instalación anterior (adopt.go).
func (layout Layout) AdoptionMarker() string {
	return filepath.Join(layout.DataDir, "legacy-adopted.json")
}

func (layout Layout) lockFile() string {
	return filepath.Join(layout.DataDir, "host.lock")
}
