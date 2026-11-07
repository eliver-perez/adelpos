package config

import (
	"encoding/json"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func validConfig() Config {
	return Config{Environment: "development", InstallationMode: "server-terminal", ListenAddress: "127.0.0.1:8181", AllowedOrigins: []string{"tauri://localhost"}, LicenseMode: "development", Database: Database{Host: "127.0.0.1", Port: 55432, Name: "adel_pos", User: "adel_app", Password: "test-only", SSLMode: "disable", MaxConnections: 8}}
}

func TestValidation(t *testing.T) {
	if err := validConfig().Validate(); err != nil {
		t.Fatal(err)
	}
	tests := map[string]func(*Config){
		"missing password":              func(c *Config) { c.Database.Password = "" },
		"remote database without TLS":   func(c *Config) { c.Database.Host = "192.168.1.50" },
		"TLS downgrade":                 func(c *Config) { c.Database.SSLMode = "prefer" },
		"wildcard CORS":                 func(c *Config) { c.AllowedOrigins = []string{"*"} },
		"invalid CORS":                  func(c *Config) { c.AllowedOrigins = []string{"http://localhost/path"} },
		"terminal cannot run server":    func(c *Config) { c.InstallationMode = "terminal-only" },
		"implicit LAN":                  func(c *Config) { c.ListenAddress = "192.168.1.8:8181" },
		"public binding":                func(c *Config) { c.ListenAddress = "0.0.0.0:8181"; c.AllowLAN = true },
		"invalid port":                  func(c *Config) { c.ListenAddress = "127.0.0.1:0" },
		"unbounded pool":                func(c *Config) { c.Database.MaxConnections = 0 },
		"production development bypass": func(c *Config) { c.Environment = "production" },
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			c := validConfig()
			mutate(&c)
			if c.Validate() == nil {
				t.Fatal("expected rejection")
			}
		})
	}
	c := validConfig()
	c.ListenAddress = "192.168.1.8:8181"
	c.AllowLAN = true
	if err := c.Validate(); err != nil {
		t.Fatal(err)
	}
	c.Database.Host = "db.example.test"
	c.Database.SSLMode = "verify-full"
	if err := c.Validate(); err != nil {
		t.Fatal(err)
	}
}

func TestStrictLoadAndSecretOverride(t *testing.T) {
	filename := filepath.Join(t.TempDir(), "server.json")
	c := validConfig()
	c.Database.Password = ""
	body, _ := json.Marshal(c)
	if err := os.WriteFile(filename, body, 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("ADEL_DB_PASSWORD", "only-in-env")
	loaded, err := Load(filename)
	if err != nil || loaded.Database.Password != "only-in-env" {
		t.Fatalf("override failed: %v", err)
	}
	for _, bad := range []string{string(body) + " {}", strings.Replace(string(body), `"environment"`, `"unknown"`, 1)} {
		_ = os.WriteFile(filename, []byte(bad), 0600)
		if _, err := Load(filename); err == nil {
			t.Fatal("accepted invalid config")
		}
	}
	if strings.Contains(loaded.String(), "only-in-env") {
		t.Fatal("secret in config summary")
	}
}

func TestConnectionStringEscapesPassword(t *testing.T) {
	c := validConfig()
	c.Database.Password = "p@ss:/?#%& space"
	u, err := url.Parse(c.Database.ConnectionString())
	if err != nil {
		t.Fatal(err)
	}
	password, _ := u.User.Password()
	if password != c.Database.Password {
		t.Fatal("password altered")
	}
}
