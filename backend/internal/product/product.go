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
	// MinClientVersions es la versión mínima por tipo de cliente (etapa A1, decisión A1-D4): la aplicación Android
	// numera sus versiones aparte (empieza en 0.1.0) y no puede compararse con la mínima del escritorio.
	// MinClientVersion se conserva con el valor del escritorio para los clientes que solo leen esa propiedad.
	MinClientVersions map[string]string `json:"min_client_versions"`
	MinServerVersion  string            `json:"min_server_version"`
}

// MinimumFor devuelve la versión mínima que el servidor acepta de un tipo de cliente. Un tipo sin mínima propia
// (o sin tipo, como las herramientas locales) usa la general, que es la del escritorio.
func (m Metadata) MinimumFor(kind string) string {
	if minimum, found := m.MinClientVersions[kind]; found && minimum != "" {
		return minimum
	}
	return m.MinClientVersion
}

func Read() Metadata {
	var m Metadata
	if err := json.Unmarshal(metadata, &m); err != nil {
		panic("metadatos generados inválidos")
	}
	return m
}
