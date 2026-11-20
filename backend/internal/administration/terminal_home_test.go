package administration

import "testing"

// La pantalla de inicio de una terminal es una lista cerrada (etapa 9C, §1.5): solo las secciones
// operativas de la barra superior. «Estado del sistema», la administración y los valores con otra
// escritura se rechazan antes de escribir.
func TestTerminalHomeIsAClosedList(t *testing.T) {
	for _, screen := range []string{"tables", "takeaway", "kitchen", "cash"} {
		if !validTerminalHome(screen) {
			t.Fatalf("%q debía aceptarse como pantalla de inicio", screen)
		}
	}
	for _, screen := range []string{"", "status", "restaurant", "administration", "catalog", "preferences", "Tables", " tables"} {
		if validTerminalHome(screen) {
			t.Fatalf("%q no debía aceptarse como pantalla de inicio", screen)
		}
	}
}

// Un teléfono o tableta de meseros solo abre en Mesas o Para llevar (etapa A3, §5.1 y A3-D7): la aplicación Android no
// tiene modo cocina ni cobra. Su lista es parte de la lista cerrada de todas las terminales.
func TestAndroidTerminalHomeIsTablesOrTakeaway(t *testing.T) {
	for _, screen := range []string{"tables", "takeaway"} {
		if !validAndroidHome(screen) || !validTerminalHome(screen) {
			t.Fatalf("%q debía aceptarse como pantalla de inicio de un android", screen)
		}
	}
	for _, screen := range []string{"kitchen", "cash", "", "status", "Tables"} {
		if validAndroidHome(screen) {
			t.Fatalf("%q no debía aceptarse como pantalla de inicio de un android", screen)
		}
	}
}
