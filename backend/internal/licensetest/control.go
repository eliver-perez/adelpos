//go:build !adelpos_production

package licensetest

import (
	"encoding/json"
	"io"
	"net/http"
	"time"
)

// Controles del servidor simulado para la orden de pruebas (Playwright), en /_prueba/. Son lo mismo que FailNext,
// DropNext, AlterNext, SetOffset, Revoke, Renew y Snapshot de las pruebas de Go. Todos son POST con JSON, salvo
// GET /_prueba/estado:
//
//   - /_prueba/siguiente-error {"accion","codigo","retry_after","transfer_available"}: el siguiente error de esa
//     acción (challenge, activate, transfer, refresh o deactivate).
//   - /_prueba/perder-siguiente {"accion"}: la siguiente respuesta de esa acción se guarda y se pierde.
//   - /_prueba/alterar-siguiente {}: la siguiente licencia llega con la firma alterada.
//   - /_prueba/hora {"desfase"}: desfase de la hora del servidor, como duración de Go («-2h», «90m»; «0s» lo quita).
//   - /_prueba/revocar {"license_key"} y /_prueba/renovar {"license_key","meses"}: lo que haría el panel.
//   - GET /_prueba/estado: las licencias simuladas, su revisión y su activación.
func (server *Server) serveControl(writer http.ResponseWriter, request *http.Request) {
	if request.URL.Path == "/_prueba/estado" && request.Method == http.MethodGet {
		writeControl(writer, http.StatusOK, server.Snapshot())
		return
	}
	if request.Method != http.MethodPost {
		writeControl(writer, http.StatusMethodNotAllowed, map[string]string{"error": "usa POST"})
		return
	}
	var input struct {
		Action            string `json:"accion"`
		Code              string `json:"codigo"`
		RetryAfter        int    `json:"retry_after"`
		TransferAvailable bool   `json:"transfer_available"`
		Offset            string `json:"desfase"`
		LicenseKey        string `json:"license_key"`
		Months            int    `json:"meses"`
	}
	body, _ := io.ReadAll(io.LimitReader(request.Body, 16<<10))
	if len(body) > 0 && json.Unmarshal(body, &input) != nil {
		writeControl(writer, http.StatusBadRequest, map[string]string{"error": "JSON inválido"})
		return
	}
	switch request.URL.Path {
	case "/_prueba/siguiente-error":
		server.FailNext(Scripted{Action: input.Action, Code: input.Code, RetryAfter: input.RetryAfter, TransferAvailable: input.TransferAvailable})
	case "/_prueba/perder-siguiente":
		server.DropNext(input.Action)
	case "/_prueba/alterar-siguiente":
		server.AlterNext()
	case "/_prueba/hora":
		offset, err := time.ParseDuration(input.Offset)
		if err != nil {
			writeControl(writer, http.StatusBadRequest, map[string]string{"error": "desfase inválido"})
			return
		}
		server.SetOffset(offset)
	case "/_prueba/revocar":
		if err := server.Revoke(input.LicenseKey); err != nil {
			writeControl(writer, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
	case "/_prueba/renovar":
		months := input.Months
		if months == 0 {
			months = 1
		}
		if err := server.Renew(input.LicenseKey, months); err != nil {
			writeControl(writer, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
	default:
		writeControl(writer, http.StatusNotFound, map[string]string{"error": "control desconocido"})
		return
	}
	writeControl(writer, http.StatusOK, map[string]bool{"ok": true})
}

func writeControl(writer http.ResponseWriter, status int, value any) {
	writer.Header().Set("Content-Type", "application/json")
	writer.WriteHeader(status)
	_ = json.NewEncoder(writer).Encode(value)
}
