// Package config carga configuración estricta sin registrar secretos.
package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// TestDatabasePrefix es el prefijo obligatorio del nombre de la base en modo de pruebas (etapa 9F): el
// mismo que exigen las pruebas de integración y que usan las bases desechables de los scripts.
const TestDatabasePrefix = "adel_pos_test_"

type Database struct {
	Host           string `json:"host"`
	Port           int    `json:"port"`
	Name           string `json:"name"`
	User           string `json:"user"`
	Password       string `json:"password"`
	SSLMode        string `json:"ssl_mode"`
	SSLRootCert    string `json:"ssl_root_cert"`
	MaxConnections int32  `json:"max_connections"`
}

type Config struct {
	Environment      string   `json:"environment"`
	InstallationMode string   `json:"installation_mode"`
	ListenAddress    string   `json:"listen_address"`
	AllowLAN         bool     `json:"allow_lan"`
	AllowedOrigins   []string `json:"allowed_origins"`
	Database         Database `json:"database"`
	LicenseMode      string   `json:"license_mode"`
	TLSCertificate   string   `json:"tls_certificate,omitempty"`
	TLSPrivateKey    string   `json:"tls_private_key,omitempty"`
	// TestMode («"test_mode": true» en el archivo) marca el servidor de las pruebas automáticas (etapa
	// 9F). Con él, la configuración solo es válida contra una base desechable (TestDatabasePrefix) y
	// /api/v1/version lo informa para que los clientes de prueba se nieguen a correr contra un servidor
	// real. Sin el campo (omitempty) los archivos que escribe la instalación de Windows no cambian.
	TestMode bool `json:"test_mode,omitempty"`
	// PostgresBin es la carpeta opcional de pg_dump y pg_restore para el respaldo que se crea antes de
	// «Quitar datos de prueba» (etapa 9F). Sin valor: ADEL_PG_BIN o la ruta de Homebrew.
	PostgresBin string `json:"postgres_bin,omitempty"`
	// BackupDirectory es la carpeta opcional de esos respaldos. Sin valor: «backups» junto a la carpeta
	// de la configuración (.local/server.json → backups/, como scripts/database.py backup).
	BackupDirectory string `json:"backup_directory,omitempty"`
}

func Load(filename string) (Config, error) {
	f, err := os.Open(filename)
	if err != nil {
		return Config{}, errors.New("no se pudo abrir la configuración")
	}
	defer f.Close()
	var c Config
	d := json.NewDecoder(io.LimitReader(f, 65537))
	d.DisallowUnknownFields()
	if err := d.Decode(&c); err != nil {
		return c, errors.New("configuración JSON inválida o campos desconocidos")
	}
	if err := d.Decode(new(any)); err != io.EOF {
		return c, errors.New("contenido adicional en configuración")
	}
	if password, ok := os.LookupEnv("ADEL_DB_PASSWORD"); ok {
		c.Database.Password = password
	}
	return c, c.Validate()
}

func (c Config) Validate() error {
	if c.Environment != "development" && c.Environment != "production" {
		return errors.New("environment debe ser development o production")
	}
	if c.InstallationMode != "server-terminal" && c.InstallationMode != "server-only" {
		return errors.New("el backend solo admite server-terminal o server-only; terminal-only nunca inicia Go")
	}
	host, port, err := net.SplitHostPort(c.ListenAddress)
	if err != nil {
		return errors.New("listen_address debe contener IP:puerto")
	}
	ip := net.ParseIP(host)
	n, err := strconv.Atoi(port)
	if ip == nil || err != nil || n < 1 || n > 65535 {
		return errors.New("listen_address debe contener IP explícita y puerto válido")
	}
	if !ip.IsLoopback() && (!c.AllowLAN || c.Environment != "development") {
		return errors.New("escucha LAN requiere allow_lan y development; producción remota aún no habilitada")
	}
	if !ip.IsLoopback() && !ip.IsPrivate() {
		return errors.New("la escucha solo admite loopback o una IP privada específica")
	}
	if err := c.checkTestMode(); err != nil {
		return err
	}
	relativeBin := c.PostgresBin != "" && !filepath.IsAbs(c.PostgresBin)
	relativeBackups := c.BackupDirectory != "" && !filepath.IsAbs(c.BackupDirectory)
	if relativeBin || relativeBackups {
		return errors.New("postgres_bin y backup_directory deben ser rutas absolutas")
	}
	if (c.TLSCertificate == "") != (c.TLSPrivateKey == "") {
		return errors.New("TLS requiere certificado y clave privada juntos")
	}
	if !ip.IsLoopback() && c.TLSCertificate == "" {
		return errors.New("la escucha LAN requiere un certificado TLS; HTTP solo se admite en loopback")
	}
	if c.Environment == "development" && c.LicenseMode != "development" {
		return errors.New("desarrollo requiere license_mode development")
	}
	if c.Environment == "production" && c.LicenseMode != "commercial" {
		return errors.New("producción requiere license_mode commercial")
	}
	for _, origin := range c.AllowedOrigins {
		u, err := url.Parse(origin)
		if err != nil {
			return errors.New("allowed_origins contiene una dirección inválida")
		}
		allowedScheme := u.Scheme == "http" || u.Scheme == "https" || u.Scheme == "tauri"
		hasExtraComponents := u.User != nil || u.Path != "" || u.RawQuery != "" || u.Fragment != ""
		if u.Host == "" || hasExtraComponents || strings.Contains(origin, "*") || !allowedScheme {
			return errors.New("allowed_origins debe contener orígenes exactos sin comodines")
		}
	}
	db := c.Database
	if strings.TrimSpace(db.Host) == "" || db.Name == "" || db.User == "" || db.Password == "" {
		return errors.New("database requiere host, name, user y password; use ADEL_DB_PASSWORD para el secreto")
	}
	if db.Port < 1 || db.Port > 65535 || db.MaxConnections < 1 || db.MaxConnections > 100 {
		return errors.New("puerto o límite de conexiones PostgreSQL inválido")
	}
	dbIP := net.ParseIP(db.Host)
	localDB := dbIP != nil && dbIP.IsLoopback()
	if db.SSLMode != "verify-full" && !(db.SSLMode == "disable" && localDB && c.Environment == "development") {
		return errors.New("PostgreSQL requiere verify-full; disable solo se permite en loopback de desarrollo")
	}
	return nil
}

// checkTestMode impide que el modo de pruebas arranque contra la base de una instalación real (etapa 9F,
// §3): serve, migrate, prepare-setup y los demás comandos cargan la configuración con Validate, de modo
// que ninguno llega a abrir la base. Sin test_mode no cambia nada.
func (c Config) checkTestMode() error {
	if !c.TestMode {
		return nil
	}
	if !strings.HasPrefix(c.Database.Name, TestDatabasePrefix) || len(c.Database.Name) == len(TestDatabasePrefix) {
		return errors.New("el modo de pruebas solo se usa con una base de datos de pruebas cuyo nombre empiece por " +
			TestDatabasePrefix + "; nunca con la base de una instalación real")
	}
	return nil
}

func (d Database) ConnectionString() string {
	u := url.URL{Scheme: "postgresql", User: url.UserPassword(d.User, d.Password), Host: net.JoinHostPort(d.Host, strconv.Itoa(d.Port)), Path: "/" + d.Name}
	q := u.Query()
	q.Set("sslmode", d.SSLMode)
	q.Set("connect_timeout", "3")
	if d.SSLRootCert != "" {
		q.Set("sslrootcert", d.SSLRootCert)
	}
	u.RawQuery = q.Encode()
	return u.String()
}

func (c Config) String() string {
	return fmt.Sprintf("environment=%s mode=%s listen=%s", c.Environment, c.InstallationMode, c.ListenAddress)
}
