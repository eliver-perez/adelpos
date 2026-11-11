package kitchen

import "local/ad-el-pos/server/internal/documents"

// RenderText conserva para las pruebas anteriores a la etapa 9B la comanda renderizada con la
// plantilla predeterminada (la versión 1 del sistema). Desde la 9B la impresión usa la versión de
// plantilla guardada en cada comanda (renderStored).
func RenderText(document Document, paperWidth int) string {
	kind := documentKind(document.Kind)
	layout, err := RenderDocument(&documents.Version{Kind: kind, Template: documents.Default(kind)}, document, paperWidth)
	if err != nil {
		panic(err)
	}
	return layout.Text()
}
