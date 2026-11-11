package kitchen

import (
	"net"
	"regexp"
	"slices"
	"strings"

	"local/ad-el-pos/server/internal/events"
)

var queuePattern = regexp.MustCompile(`^[A-Za-z0-9_][A-Za-z0-9_-]{0,126}$`)
var cutterPattern = regexp.MustCompile(`^(CutMedia|CutType|Cutter)=[A-Za-z0-9_-]{1,30}$`)

func validatePrinter(printer *Printer) error {
	printer.Name = strings.TrimSpace(printer.Name)
	printer.Destination = strings.TrimSpace(printer.Destination)
	if printer.Name == "" || len(printer.Name) > 100 || printer.Destination == "" || len(printer.Destination) > 200 {
		return invalid("Revisa nombre y destino de la impresora.")
	}
	if printer.Port == 0 {
		printer.Port = 9100
	}
	if printer.PaperWidth == 0 {
		printer.PaperWidth = 80
	}
	if printer.Copies == 0 {
		printer.Copies = 1
	}
	if !slices.Contains([]int{58, 80}, printer.PaperWidth) || printer.Copies < 1 || printer.Copies > 3 ||
		printer.Port < 9100 || printer.Port > 9109 || (printer.Cut && !printer.SupportsCut) {
		return invalid("Revisa papel (58/80 mm), copias (1–3), puerto (9100–9109) y capacidad de corte.")
	}
	switch printer.Transport {
	case "cups":
		if !queuePattern.MatchString(printer.Destination) {
			return invalid("Selecciona el nombre de una cola CUPS del servidor.")
		}
		if printer.CutterOption != "" && !cutterPattern.MatchString(printer.CutterOption) {
			return invalid("La opción de corte CUPS debe corresponder a CutMedia, CutType o Cutter del controlador.")
		}
	case "escpos_tcp":
		if printer.AcceptsESCPos {
			return invalid("La opción «Esta impresora entiende ESC/POS» solo se configura en colas CUPS: ESC/POS por red ya recibe formato y logotipo.")
		}
		address := net.ParseIP(printer.Destination)
		if address == nil || !(address.IsPrivate() || address.IsLoopback() || address.IsLinkLocalUnicast()) {
			return invalid("ESC/POS requiere una dirección IP privada explícita de la impresora.")
		}
		if printer.CutterOption != "" {
			return invalid("El corte ESC/POS lo implementa su controlador, no una opción CUPS.")
		}
	case "agent":
		if printer.AcceptsESCPos {
			return invalid("La opción «Esta impresora entiende ESC/POS» solo se configura en colas CUPS.")
		}
		if strings.ContainsAny(printer.Destination, "\r\n\x00") {
			return invalid("El destino de agente no es válido.")
		}
	default:
		return invalid("Selecciona CUPS, ESC/POS de red o agente futuro.")
	}
	if printer.FallbackID != nil {
		if err := validID(*printer.FallbackID); err != nil {
			return err
		}
		if *printer.FallbackID == printer.ID {
			return invalid("Una impresora no puede respaldarse a sí misma.")
		}
	}
	return nil
}

func (handler *service) savePrinter(parts []string) (any, error) {
	var input struct {
		OperationID string `json:"operation_id"`
		Printer
	}
	if err := decode(handler.request.Body, &input); err != nil {
		return nil, err
	}
	printer := input.Printer
	if err := validatePrinter(&printer); err != nil {
		return nil, err
	}
	// Una identidad nueva todavía no puede formar parte de una cadena existente.
	if printer.FallbackID != nil && len(parts) == 2 {
		var cycle bool
		err := handler.tx.QueryRow(handler.ctx, `WITH RECURSIVE chain AS (
            SELECT id,fallback_id,ARRAY[id] AS visited FROM app.kitchen_printers WHERE id=$1
            UNION ALL SELECT p.id,p.fallback_id,c.visited||p.id FROM app.kitchen_printers p
            JOIN chain c ON p.id=c.fallback_id WHERE NOT p.id=ANY(c.visited))
            SELECT EXISTS(SELECT FROM chain WHERE id=$2)`, printer.FallbackID, printer.ID).Scan(&cycle)
		if err != nil {
			return nil, err
		}
		if cycle {
			return nil, invalid("Los destinos de respaldo no pueden formar ciclos.")
		}
	}
	var before *Printer
	if len(parts) == 1 {
		if printer.ID != "" || printer.Revision != 0 {
			return nil, invalid("La impresora nueva no reutiliza identidad ni revisión.")
		}
		printer.ID = events.NewID()
		_, err := handler.tx.Exec(handler.ctx, `INSERT INTO app.kitchen_printers(id,name,transport,destination,port,paper_width,
            copies,supports_cut,cut,cutter_option,active,fallback_id,accepts_escpos) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13)`,
			printer.ID, printer.Name, printer.Transport, printer.Destination, printer.Port, printer.PaperWidth, printer.Copies,
			printer.SupportsCut, printer.Cut, printer.CutterOption, printer.Active, printer.FallbackID, printer.AcceptsESCPos)
		if err != nil {
			return nil, err
		}
	} else {
		var err error
		before, err = one[Printer](handler.ctx, handler.tx, "SELECT to_jsonb(p) FROM app.kitchen_printers p WHERE id=$1", parts[1])
		if err != nil {
			return nil, err
		}
		if printer.ID != parts[1] || printer.Revision != before.Revision {
			return nil, conflict()
		}
		_, err = handler.tx.Exec(handler.ctx, `UPDATE app.kitchen_printers SET name=$1,transport=$2,destination=$3,port=$4,
            paper_width=$5,copies=$6,supports_cut=$7,cut=$8,cutter_option=$9,active=$10,fallback_id=$11,accepts_escpos=$12,
            revision=revision+1,updated_at=now() WHERE id=$13`, printer.Name, printer.Transport, printer.Destination, printer.Port,
			printer.PaperWidth, printer.Copies, printer.SupportsCut, printer.Cut, printer.CutterOption, printer.Active, printer.FallbackID,
			printer.AcceptsESCPos, printer.ID)
		if err != nil {
			return nil, err
		}
	}
	saved, err := one[Printer](handler.ctx, handler.tx, "SELECT to_jsonb(p) FROM app.kitchen_printers p WHERE id=$1", printer.ID)
	if err != nil {
		return nil, err
	}
	return saved, handler.history("printer.saved", &printer.ID, before, saved)
}
