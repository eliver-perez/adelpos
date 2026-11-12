//go:build windows

package standalone

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/url"
	"os"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5"
	"local/ad-el-pos/server/internal/administration"
	"local/ad-el-pos/server/internal/config"
	"local/ad-el-pos/server/internal/fileacl"
)

// Prepare deja la instalación lista para servir: carpeta y clúster, PostgreSQL en marcha, rol y base de la
// aplicación, configuración del servidor y token del asistente inicial. Devuelve la configuración cargada.
func Prepare(ctx context.Context, cluster *Cluster, databaseName string) (config.Config, error) {
	if err := cluster.Ensure(ctx); err != nil {
		return config.Config{}, err
	}
	if err := cluster.Start(ctx); err != nil {
		return config.Config{}, err
	}
	if err := ensureConfig(cluster.layout, databaseName); err != nil {
		return config.Config{}, err
	}
	cfg, err := config.Load(cluster.layout.ConfigFile())
	if err != nil {
		return config.Config{}, err
	}
	if err := ensureDatabase(ctx, cluster.layout, cfg.Database); err != nil {
		return config.Config{}, err
	}
	if err := ensureProvisioning(cluster.layout); err != nil {
		return config.Config{}, err
	}
	return cfg, nil
}

// serverConfig es la configuración de desarrollo de la instalación de prueba: la 9E no requiere licencias
// (licensing solo admite development) y todo escucha en loopback. Los orígenes son los del WebView de
// Tauri en Windows y macOS.
func serverConfig(databaseName, password string, port int) config.Config {
	return config.Config{
		Environment:      "development",
		InstallationMode: "server-terminal",
		ListenAddress:    ServerAddress,
		AllowLAN:         false,
		AllowedOrigins:   []string{"tauri://localhost", "http://tauri.localhost"},
		Database: config.Database{
			Host:           "127.0.0.1",
			Port:           port,
			Name:           databaseName,
			User:           appRole,
			Password:       password,
			SSLMode:        "disable",
			MaxConnections: 8,
		},
		LicenseMode: "development",
	}
}

// ensureConfig escribe server.json la primera vez, con una contraseña aleatoria para el rol de la
// aplicación. Si ya existe se conserva tal cual: la contraseña del rol se alinea con la del archivo.
func ensureConfig(layout Layout, databaseName string) error {
	if _, err := os.Stat(layout.ConfigFile()); err == nil {
		return nil
	}
	password, err := randomHex(24)
	if err != nil {
		return err
	}
	body, err := json.MarshalIndent(serverConfig(databaseName, password, layout.Port), "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(layout.ConfigFile(), append(body, '\n'), 0600); err != nil {
		return errors.New("no se pudo escribir la configuración del servidor")
	}
	return fileacl.Protect(layout.ConfigFile())
}

// superuserConnection abre una conexión de administración con el superusuario del clúster.
func superuserConnection(ctx context.Context, layout Layout, database string) (*pgx.Conn, error) {
	password, err := secretFile(layout.AdminPassword())
	if err != nil {
		return nil, err
	}
	address := url.URL{
		Scheme:   "postgresql",
		User:     url.UserPassword(superuser, password),
		Host:     net.JoinHostPort("127.0.0.1", strconv.Itoa(layout.Port)),
		Path:     "/" + database,
		RawQuery: "sslmode=disable&connect_timeout=5",
	}
	connection, err := pgx.Connect(ctx, address.String())
	if err != nil {
		return nil, errors.New("no se pudo conectar con PostgreSQL de la instalación")
	}
	return connection, nil
}

// ensureDatabase crea el rol sin privilegios y la base de la aplicación si faltan (como db-init en macOS)
// y alinea la contraseña del rol con la de server.json. Los nombres y la contraseña viajan citados por
// format() de PostgreSQL, nunca concatenados.
func ensureDatabase(ctx context.Context, layout Layout, database config.Database) error {
	connection, err := superuserConnection(ctx, layout, "postgres")
	if err != nil {
		return err
	}
	defer connection.Close(context.Background())
	var exists bool
	if err := connection.QueryRow(ctx, "SELECT EXISTS (SELECT FROM pg_roles WHERE rolname = $1)", database.User).Scan(&exists); err != nil {
		return errors.New("no se pudo consultar el rol de la aplicación")
	}
	statement := "SELECT format('ALTER ROLE %I LOGIN PASSWORD %L', $1::text, $2::text)"
	if !exists {
		statement = "SELECT format('CREATE ROLE %I LOGIN PASSWORD %L NOSUPERUSER NOCREATEDB NOCREATEROLE', $1::text, $2::text)"
	}
	if err := execFormatted(ctx, connection, statement, database.User, database.Password); err != nil {
		return errors.New("no se pudo preparar el rol de la aplicación")
	}
	if err := connection.QueryRow(ctx, "SELECT EXISTS (SELECT FROM pg_database WHERE datname = $1)", database.Name).Scan(&exists); err != nil {
		return errors.New("no se pudo consultar la base de la aplicación")
	}
	if !exists {
		if err := execFormatted(ctx, connection, "SELECT format('CREATE DATABASE %I OWNER %I', $1::text, $2::text)", database.Name, database.User); err != nil {
			return errors.New("no se pudo crear la base de la aplicación")
		}
	}
	return nil
}

// execFormatted arma una sentencia con format() en el servidor y la ejecuta.
func execFormatted(ctx context.Context, connection *pgx.Conn, query string, arguments ...any) error {
	var statement string
	if err := connection.QueryRow(ctx, query, arguments...).Scan(&statement); err != nil {
		return err
	}
	_, err := connection.Exec(ctx, statement)
	return err
}

// ensureProvisioning crea el token del asistente inicial (como prepare-setup en macOS) si falta.
func ensureProvisioning(layout Layout) error {
	if text, err := os.ReadFile(layout.Provisioning()); err == nil {
		if len(strings.TrimSpace(string(text))) != 43 {
			return errors.New("el archivo de aprovisionamiento no es válido; requiere revisión local")
		}
		return nil
	}
	if err := os.WriteFile(layout.Provisioning(), []byte(administration.NewToken()+"\n"), 0600); err != nil {
		return errors.New("no se pudo crear el aprovisionamiento local")
	}
	return fileacl.Protect(layout.Provisioning())
}
