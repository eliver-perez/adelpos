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
}

func Read() Metadata {
	var m Metadata
	if err := json.Unmarshal(metadata, &m); err != nil {
		panic("metadatos generados inválidos")
	}
	return m
}
