// Package kitchen administra preparación y transportes de impresión sin registrar cobros. Del
// inventario solo registra las solicitudes de consumo de cada liberación y cambio (etapa 8); el
// cálculo lo hace el trabajador de inventario.
package kitchen

import (
	"time"

	"local/ad-el-pos/server/internal/catalog"
	"local/ad-el-pos/server/internal/documents"
)

type Settings struct {
	Grouping          string `json:"grouping"`
	Tracking          string `json:"tracking"`
	AutomaticRelease  bool   `json:"automatic_release"`
	LeadMinutes       int    `json:"lead_minutes"`
	ShowPrices        bool   `json:"show_prices"`
	ShowNotes         bool   `json:"show_notes"`
	ShowEstablishment bool   `json:"show_establishment"`
	TextSize          string `json:"text_size"`
	LineSpacing       int    `json:"line_spacing"`
	QuantityStyle     string `json:"quantity_style"`
	Revision          int    `json:"revision"`
}

type Printer struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Transport   string `json:"transport"`
	Destination string `json:"destination"`
	Port        int    `json:"port"`
	PaperWidth  int    `json:"paper_width"`
	Copies      int    `json:"copies"`
	// SupportsCut se conserva por compatibilidad (CHECK NOT cut OR supports_cut de la 0005): desde la etapa
	// 9F Go la guarda igual a Cut, de modo que «Cortar el papel al terminar» es una sola casilla.
	SupportsCut  bool   `json:"supports_cut"`
	Cut          bool   `json:"cut"`
	CutterOption string `json:"cutter_option"`
	// CutMode es el tipo de corte (etapa 9F, §4.4): «full» (GS V 0) o «partial» (GS V 1). Vacío en las
	// fotografías de impresora anteriores a la 9F, que cortan como entonces (total).
	CutMode string `json:"cut_mode"`
	// CutFeedLines son los renglones en blanco entre el último renglón del documento y el corte (lista
	// cerrada de 0 a 8), iguales en todos los documentos. nil en las impresoras que ya existían antes de la
	// 9F (la 0022 no les asigna avance) y en las fotografías de esa época: conservan el final de siempre,
	// tres saltos y el corte (revisión 9F, M1), hasta que el propietario elija sus renglones.
	CutFeedLines *int `json:"cut_feed_lines"`
	// AcceptsESCPos marca una impresora CUPS que entiende ESC/POS (etapa 9B): solo entonces, y si su
	// cola admite datos raw, recibe documentos con negrita, tamaños o logotipo. Desactivada por
	// omisión; en ESC/POS por red no aplica (siempre recibe ESC/POS).
	AcceptsESCPos bool `json:"accepts_escpos"`
	// Columns son las columnas de texto elegidas para la impresora (etapa 9C, §5.3): nil es el
	// valor predeterminado de su papel (42 en 80 mm y 30 en 58 mm); la otra opción de la lista
	// cerrada es 48 (80 mm) o 32 (58 mm). Viaja en la fotografía de cada trabajo: la reimpresión de
	// una comanda sale con las columnas del original y las fotografías anteriores a la 9C, sin el
	// campo, con el predeterminado.
	Columns    *int    `json:"columns"`
	Active     bool    `json:"active"`
	FallbackID *string `json:"fallback_id"`
	Revision   int     `json:"revision"`
}

type Station struct {
	ID             string  `json:"id"`
	Name           string  `json:"name"`
	Description    string  `json:"description"`
	Active         bool    `json:"active"`
	SortOrder      int     `json:"sort_order"`
	Reception      string  `json:"reception"`
	PrinterID      *string `json:"printer_id"`
	TargetMinutes  int     `json:"target_minutes"`
	WarningMinutes int     `json:"warning_minutes"`
	OverdueMinutes int     `json:"overdue_minutes"`
	Grouping       string  `json:"grouping"`
	Tracking       string  `json:"tracking"`
	Revision       int     `json:"revision"`
}

type Route struct {
	ID         string   `json:"id"`
	ProductID  *string  `json:"product_id"`
	CategoryID *string  `json:"category_id"`
	Mode       string   `json:"mode"`
	StationIDs []string `json:"station_ids"`
	Revision   int      `json:"revision"`
}

type Release struct {
	ID              string             `json:"id"`
	SourceEventID   string             `json:"source_event_id"`
	OrderID         string             `json:"order_id"`
	AccountID       string             `json:"account_id"`
	UnitIDs         []string           `json:"unit_ids"`
	State           string             `json:"state"`
	Automatic       bool               `json:"automatic"`
	LeadMinutes     int                `json:"lead_minutes"`
	ScheduledAt     *time.Time         `json:"scheduled_at"`
	DueAt           *time.Time         `json:"due_at"`
	Warnings        []string           `json:"warnings"`
	RoutingSnapshot map[string]Route   `json:"routing_snapshot"`
	StationSnapshot map[string]Station `json:"station_snapshot"`
	ContextSnapshot TicketContext      `json:"context_snapshot"`
	ReleasedAt      *time.Time         `json:"released_at"`
	Revision        int                `json:"revision"`
	CreatedAt       time.Time          `json:"created_at"`
}

type TicketContext struct {
	Establishment string `json:"establishment"`
	Timezone      string `json:"timezone"`
	Folio         int64  `json:"folio"`
	AccountName   string `json:"account_name"`
	Table         string `json:"table"`
	Area          string `json:"area"`
	Mode          string `json:"mode"`
	Customer      string `json:"customer"`
	Origin        string `json:"origin"`
	Employee      string `json:"employee"`
	OrderID       string `json:"order_id"`
}

type Part struct {
	Path        string                        `json:"path"`
	ProductID   string                        `json:"product_id"`
	Name        string                        `json:"name"`
	Variant     string                        `json:"variant"`
	Quantity    int                           `json:"quantity"`
	Ingredients []catalog.IngredientBreakdown `json:"ingredients"`
	Groups      []catalog.GroupBreakdown      `json:"groups"`
	Notes       string                        `json:"notes"`
	// QuickNotes son las indicaciones rápidas resueltas de esta parte (etapa 8). Se omiten sin
	// indicaciones: las tareas y documentos anteriores no cambian de forma.
	QuickNotes []catalog.QuickNoteBreakdown `json:"quick_notes,omitempty"`
}

type Instructions struct {
	ItemID    string        `json:"item_id"`
	VersionID string        `json:"version_id"`
	UnitIDs   []string      `json:"unit_ids"`
	Name      string        `json:"name"`
	Quantity  int           `json:"quantity"`
	Parts     []Part        `json:"parts"`
	Price     string        `json:"price,omitempty"`
	Before    *Instructions `json:"before,omitempty"`
	Reason    string        `json:"reason,omitempty"`
	Notes     string        `json:"notes,omitempty"`
}

type Section struct {
	Station Station        `json:"station"`
	Items   []Instructions `json:"items"`
}

type Document struct {
	Preview   string        `json:"preview,omitempty"`
	ID        string        `json:"id"`
	Kind      string        `json:"kind"`
	CreatedAt time.Time     `json:"created_at"`
	Context   TicketContext `json:"context"`
	Settings  Settings      `json:"settings"`
	Sections  []Section     `json:"sections"`
	// Etapa 9B: versión de plantilla con la que se emitió la comanda (vacía en las anteriores, que
	// se reimprimen con la versión 1 del sistema), copia de la identidad del establecimiento y
	// tamaño de texto efectivo del documento (la cola lo usa en el camino de texto). Se omiten en
	// los documentos anteriores para no cambiar su forma.
	TemplateVersionID *string             `json:"template_version_id,omitempty"`
	Identity          *documents.Identity `json:"identity,omitempty"`
	TextSize          string              `json:"text_size,omitempty"`
}

type Task struct {
	ID           string       `json:"id"`
	UnitID       string       `json:"unit_id"`
	ReleaseID    string       `json:"release_id"`
	StationID    string       `json:"station_id"`
	VersionID    string       `json:"version_id"`
	State        string       `json:"state"`
	Instructions Instructions `json:"instructions"`
	Revision     int          `json:"revision"`
	ReleasedAt   time.Time    `json:"released_at"`
	UpdatedAt    time.Time    `json:"updated_at"`
}

type PrintJob struct {
	OriginalJobID *string `json:"original_job_id"`
	TextSize      string  `json:"text_size,omitempty"`
	// TicketKind y DrawerPulse los completa el trabajador al reclamar el trabajo: el tipo de ticket
	// decide reintentos, respaldo y eventos; el pulso solo existe en los comandos del cajón.
	TicketKind  string       `json:"ticket_kind,omitempty"`
	DrawerPulse *DrawerPulse `json:"drawer_pulse,omitempty"`
	// Layout es la IR de un documento con atributos o imágenes (etapa 9B, kitchen_print_jobs.layout);
	// nil si el documento es texto plano, que viaja por el camino anterior. Rasters son los bits de
	// sus imágenes, que el trabajador carga al enviar; nunca se serializan.
	Layout             *documents.Layout            `json:"layout,omitempty"`
	Rasters            map[string]*documents.Raster `json:"-"`
	ID                 string                       `json:"id"`
	TicketID           string                       `json:"ticket_id"`
	PrinterID          string                       `json:"printer_id"`
	PrinterSnapshot    Printer                      `json:"printer_snapshot"`
	Content            string                       `json:"content,omitempty"`
	State              string                       `json:"state"`
	Attempts           int                          `json:"attempts"`
	Revision           int                          `json:"revision"`
	LastError          string                       `json:"last_error"`
	TransportReference string                       `json:"transport_reference"`
	CreatedAt          time.Time                    `json:"created_at"`
}
