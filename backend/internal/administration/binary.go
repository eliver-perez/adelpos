package administration

import (
	"net/http"
	"strconv"

	"local/ad-el-pos/server/internal/catalog"
)

// writeBinary envía una respuesta binaria del catálogo (etapa 9G, §5.3: el contenido de una imagen) con su tipo, su
// etiqueta y su caché, o 304 sin contenido si el cliente ya la tiene. Devuelve false si el resultado no es binario.
func writeBinary(writer http.ResponseWriter, result any) bool {
	content, ok := result.(*catalog.Binary)
	if !ok {
		return false
	}
	header := writer.Header()
	header.Set("ETag", content.ETag)
	header.Set("Cache-Control", content.CacheControl)
	if content.NotModified {
		writer.WriteHeader(http.StatusNotModified)
		return true
	}
	header.Set("Content-Type", content.ContentType)
	header.Set("Content-Length", strconv.Itoa(len(content.Content)))
	writer.WriteHeader(http.StatusOK)
	_, _ = writer.Write(content.Content)
	return true
}
