package legacypkg

import "testing"

func TestPasswordColumns(t *testing.T) {
	passwords := []string{"password", "PASSWORD", "user_Password", "contrasena", "Contraseña", "passwd", "pwd", "PassHash", "clave_pass"}
	for _, name := range passwords {
		if !IsPasswordColumn(name) {
			t.Errorf("%s debería tratarse como contraseña", name)
		}
	}
	others := []string{"usuario", "pasaje", "personal", "activo", "id"}
	for _, name := range others {
		if IsPasswordColumn(name) {
			t.Errorf("%s no es una contraseña", name)
		}
	}
	if PresentColumnName("password") != "password_present" {
		t.Fatal("nombre de la columna booleana")
	}
}

func TestExcludedTablesAndBinaryTypes(t *testing.T) {
	if ExcludedTableReason("sysdiagrams") != ReasonTool || ExcludedTableReason("Personal_Detalles") != ReasonPersonalData {
		t.Fatal("tablas excluidas")
	}
	if ExcludedTableReason("personal") != "" || ExcludedTableReason("ventas") != "" {
		t.Fatal("tabla extraída marcada como excluida")
	}
	for _, binary := range []string{"varbinary", "IMAGE", "binary", "timestamp"} {
		if !IsBinaryType(binary) {
			t.Errorf("%s es binario", binary)
		}
	}
	if IsBinaryType("varchar") || IsBinaryType("money") {
		t.Fatal("tipo no binario")
	}
}
