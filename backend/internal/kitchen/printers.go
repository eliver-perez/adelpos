package kitchen

import (
	"encoding/json"
	"fmt"
	"net"
	"regexp"
	"slices"
	"strings"

	"local/ad-el-pos/server/internal/documents"
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
	if err := validatePaperCut(*printer); err != nil {
		return err
	}
	// Columnas de texto (etapa 9C, §5.3): lista cerrada por papel. Cambiar el papel sin elegir de
	// nuevo unas columnas válidas se rechaza con el motivo; el predeterminado se guarda como NULL.
	if printer.Columns != nil {
		if !documents.ValidColumns(printer.PaperWidth, *printer.Columns) {
			return invalid(columnsMessage(printer.PaperWidth, *printer.Columns))
		}
		if *printer.Columns == documents.ColumnsFor(printer.PaperWidth) {
			printer.Columns = nil
		}
	}
	switch printer.Transport {
	case "cups":
		// La cola de impresión del equipo: una cola CUPS o, en Windows, una impresora instalada en
		// Windows (system_queue_unix.go y system_queue_windows.go).
		if err := validateSystemQueue(printer); err != nil {
			return err
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

// Tipos de corte de papel (etapa 9F, §4.4): la lista cerrada de Go y del CHECK de la 0022.
const (
	CutFull    = "full"
	CutPartial = "partial"
)

// validatePaperCut valida el tipo de corte y los renglones de avance. Vacíos (una fotografía anterior a
// la 9F o un cliente que no los envía) son válidos: el corte es total con los tres saltos de siempre.
func validatePaperCut(printer Printer) error {
	if printer.CutMode != "" && printer.CutMode != CutFull && printer.CutMode != CutPartial {
		return invalid("Elige corte total o parcial.")
	}
	if printer.CutFeedLines != nil && (*printer.CutFeedLines < 0 || *printer.CutFeedLines > documents.MaxCutFeedLines) {
		return invalid("Los renglones de avance antes del corte van de 0 a 8.")
	}
	return nil
}

// PaperCut es el final de cada documento de la impresora: con «Cortar el papel al terminar», sus renglones
// de avance y el corte total o parcial; sin corte, los tres saltos de siempre. Una fotografía anterior a
// la etapa 9F (sin tipo ni avance) corta como entonces: total y con tres saltos.
func (printer Printer) PaperCut() documents.PaperCut {
	feed := documents.LegacyCutFeedLines
	if printer.CutFeedLines != nil {
		feed = *printer.CutFeedLines
	}
	return documents.PaperCut{
		Enabled:   printer.Cut && printer.SupportsCut,
		Partial:   printer.CutMode == CutPartial,
		FeedLines: feed,
	}
}

// presentFields lee qué campos trae el cuerpo JSON: un cliente anterior a la etapa 9F no envía el tipo de
// corte ni el avance, y una impresora nueva sin «cut» recibe el corte predeterminado de su papel.
func presentFields(body []byte) map[string]bool {
	var raw map[string]json.RawMessage
	present := map[string]bool{}
	if err := json.Unmarshal(body, &raw); err != nil {
		return present
	}
	for key := range raw {
		present[key] = true
	}
	return present
}

// applyCutDefaults completa el corte de papel antes de validar (etapa 9F, decisión D5):
//   - una impresora nueva sin «cut» corta al terminar si su papel es de 80 mm (las de 58 mm muchas veces no
//     tienen cortador); sin tipo, corte total; sin avance, 4 renglones;
//   - al editar, lo que el cliente no envía conserva lo guardado (compatibilidad de la API);
//   - supports_cut sigue a cut: ya no hacen falta dos casillas para cortar.
func applyCutDefaults(printer *Printer, before *Printer, present map[string]bool) {
	if before == nil {
		if !present["cut"] {
			printer.Cut = printer.PaperWidth == 80 || printer.PaperWidth == 0
		}
		if printer.CutMode == "" {
			printer.CutMode = CutFull
		}
		if printer.CutFeedLines == nil {
			feed := documents.DefaultCutFeedLines
			printer.CutFeedLines = &feed
		}
	} else {
		if !present["cut"] {
			printer.Cut = before.Cut
		}
		if printer.CutMode == "" {
			printer.CutMode = before.CutMode
		}
		if printer.CutFeedLines == nil {
			printer.CutFeedLines = before.CutFeedLines
		}
	}
	printer.SupportsCut = printer.Cut
}

// validateCUPSQueue valida el destino de una impresora en una cola CUPS: el nombre de la cola
// (nunca una ruta ni argumentos) y la opción de corte que anuncia su controlador.
func validateCUPSQueue(printer *Printer) error {
	if !queuePattern.MatchString(printer.Destination) {
		return invalid("Selecciona el nombre de una cola CUPS del servidor.")
	}
	if printer.CutterOption != "" && !cutterPattern.MatchString(printer.CutterOption) {
		return invalid("La opción de corte CUPS debe corresponder a CutMedia, CutType o Cutter del controlador.")
	}
	return nil
}

// columnsMessage explica la lista cerrada de columnas de un papel cuando se pide otro valor (por
// ejemplo, 48 columnas después de cambiar el papel a 58 mm).
func columnsMessage(paperWidth, columns int) string {
	choices := documents.ColumnChoices(paperWidth)
	return fmt.Sprintf("Con papel de %d mm las columnas de texto son %d (predeterminado) o %d; %d no es una opción. "+
		"Si cambiaste el papel, elige de nuevo las columnas.", paperWidth, choices[0], choices[1], columns)
}

// TextColumns son las columnas de texto efectivas de la impresora: las elegidas o, sin elegir (o
// con un valor que ya no corresponde a su papel), las predeterminadas de su papel.
func (printer Printer) TextColumns() int {
	if printer.Columns != nil && documents.ValidColumns(printer.PaperWidth, *printer.Columns) {
		return *printer.Columns
	}
	return documents.ColumnsFor(printer.PaperWidth)
}

// RenderOptions es el ancho con que el renderizador único prepara un documento para esta
// impresora. Con las columnas predeterminadas Columns queda en cero: la salida es la de la 9B.
func (printer Printer) RenderOptions() documents.RenderOptions {
	options := documents.RenderOptions{PaperWidth: printer.PaperWidth}
	if columns := printer.TextColumns(); columns != documents.ColumnsFor(printer.PaperWidth) {
		options.Columns = columns
	}
	return options
}

// customColumns indica si la impresora imprime con columnas distintas de las de su papel.
func (printer Printer) customColumns() bool {
	return printer.TextColumns() != documents.ColumnsFor(printer.PaperWidth)
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
	var before *Printer
	if len(parts) == 2 {
		var err error
		before, err = one[Printer](handler.ctx, handler.tx, "SELECT to_jsonb(p) FROM app.kitchen_printers p WHERE id=$1", parts[1])
		if err != nil {
			return nil, err
		}
	}
	applyCutDefaults(&printer, before, presentFields(handler.request.Body))
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
	if len(parts) == 1 {
		if printer.ID != "" || printer.Revision != 0 {
			return nil, invalid("La impresora nueva no reutiliza identidad ni revisión.")
		}
		printer.ID = events.NewID()
		_, err := handler.tx.Exec(handler.ctx, `INSERT INTO app.kitchen_printers(id,name,transport,destination,port,paper_width,
            copies,supports_cut,cut,cutter_option,active,fallback_id,accepts_escpos,"columns",cut_mode,cut_feed_lines)
            VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16)`,
			printer.ID, printer.Name, printer.Transport, printer.Destination, printer.Port, printer.PaperWidth, printer.Copies,
			printer.SupportsCut, printer.Cut, printer.CutterOption, printer.Active, printer.FallbackID, printer.AcceptsESCPos, printer.Columns,
			printer.CutMode, printer.CutFeedLines)
		if err != nil {
			return nil, err
		}
	} else {
		if printer.ID != parts[1] || printer.Revision != before.Revision {
			return nil, conflict()
		}
		_, err := handler.tx.Exec(handler.ctx, `UPDATE app.kitchen_printers SET name=$1,transport=$2,destination=$3,port=$4,
            paper_width=$5,copies=$6,supports_cut=$7,cut=$8,cutter_option=$9,active=$10,fallback_id=$11,accepts_escpos=$12,
            "columns"=$13,cut_mode=$14,cut_feed_lines=$15,revision=revision+1,updated_at=now() WHERE id=$16`, printer.Name,
			printer.Transport, printer.Destination, printer.Port, printer.PaperWidth, printer.Copies, printer.SupportsCut, printer.Cut,
			printer.CutterOption, printer.Active, printer.FallbackID, printer.AcceptsESCPos, printer.Columns, printer.CutMode,
			printer.CutFeedLines, printer.ID)
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
