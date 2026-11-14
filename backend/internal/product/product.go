package product

import (
	_ "embed"
	"encoding/json"
)

//go:embed metadata.json
var metadata []byte

type Metadata struct {
	Name          string `json:"name"`
	Version       string `json:"version"`
	TechnicalID   string `json:"technical_id"`
	ServerBinary  string `json:"server_binary"`
	DatabaseName  string `json:"database_name"`
	BundleID      string `json:"bundle_id"`
	APIVersion    string `json:"api_version"`
	SchemaVersion int    `json:"schema_version"`
	// MinClientVersion es la versión más antigua de la aplicación (escritorio o Android) que acepta este
	// servidor y MinServerVersion la más antigua del servidor que necesita esta aplicación (etapa 9G, §5.1).
	// Suben junto con version cuando un cambio de la API v1 lo exige.
	MinClientVersion string `json:"min_client_version"`
	MinServerVersion string `json:"min_server_version"`
}

func Read() Metadata {
	var m Metadata
	if err := json.Unmarshal(metadata, &m); err != nil {
		panic("metadatos generados inválidos")
	}
	return m
}
