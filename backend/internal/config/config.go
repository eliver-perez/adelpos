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
	"strconv"
	"strings"
)

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
	if c.Environment == "development" && c.LicenseMode != "development" {
		return errors.New("desarrollo requiere license_mode development")
	}
	if c.Environment == "production" && c.LicenseMode != "commercial" {
		return errors.New("producción requiere license_mode commercial")
	}
	for _, origin := range c.AllowedOrigins {
		u, err := url.Parse(origin)
		if err != nil || u.Host == "" || u.User != nil || u.Path != "" || u.RawQuery != "" || u.Fragment != "" || strings.Contains(origin, "*") || (u.Scheme != "http" && u.Scheme != "https" && u.Scheme != "tauri") {
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
