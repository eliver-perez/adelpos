// Package operations conserva la operación del restaurante. Los cobros pertenecen a payments, que
// aplica sus guardas financieras dentro de la misma transacción operativa.
package operations

import (
	"encoding/json"
	"time"

	"local/ad-el-pos/server/internal/catalog"
	"local/ad-el-pos/server/internal/payments"
)

type Area struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	SortOrder   int    `json:"sort_order"`
	Active      bool   `json:"active"`
	Revision    int    `json:"revision"`
	TableCount  int    `json:"table_count"`
}

type Table struct {
	ID           string `json:"id"`
	AreaID       string `json:"area_id"`
	Name         string `json:"name"`
	Shape        string `json:"shape"`
	X            int    `json:"x"`
	Y            int    `json:"y"`
	Width        int    `json:"width"`
	Height       int    `json:"height"`
	Seats        int    `json:"seats"`
	Active       bool   `json:"active"`
	OutOfService bool   `json:"out_of_service"`
	Revision     int    `json:"revision"`
	AccountCount int    `json:"account_count"`
	// State lo calcula Go (etapa 7.1): free, occupied, awaiting_payment, partial_payment u
	// out_of_service. Con varias cuentas abiertas gana el más avanzado.
	State string `json:"state"`
	// OccupiedSince es la apertura de la cuenta abierta más antigua y OccupiedSeconds el tiempo
	// transcurrido según el servidor; ambos vacíos o en cero si la mesa no tiene cuentas.
	OccupiedSince   *time.Time `json:"occupied_since"`
	OccupiedSeconds int64      `json:"occupied_seconds"`
	// Balance es la suma de los saldos de las cuentas abiertas, calculada por payments.
	Balance string `json:"balance"`
}

type Settings struct {
	WarningMinutes int `json:"warning_minutes"`
	OverdueMinutes int `json:"overdue_minutes"`
	Revision       int `json:"revision"`
}

type Mutation struct {
	OperationID string `json:"operation_id"`
	Revision    int    `json:"revision"`
}

type AccountInput struct {
	Mutation
	Name           string  `json:"name"`
	Mode           string  `json:"mode"`
	TableID        *string `json:"table_id"`
	PriceListID    *string `json:"price_list_id"`
	CustomerName   string  `json:"customer_name"`
	Phone          string  `json:"phone"`
	Origin         string  `json:"origin"`
	Notes          string  `json:"notes"`
	ScheduledLocal string  `json:"scheduled_local"`
	ScheduleKind   string  `json:"schedule_kind"`
	AssignedUserID *string `json:"assigned_user_id"`
}

// OpenAccountInput es el cuerpo de POST restaurant/accounts. Desde la etapa 7.1 la cuenta de una mesa
// se crea con su primer producto: Items es el borrador inicial (mismo formato y límites que PUT
// …/draft) y SeenAccountIDs, las cuentas abiertas que la terminal ya veía en la mesa. Sin
// seen_account_ids no se comprueba la mesa (comportamiento anterior).
type OpenAccountInput struct {
	AccountInput
	Items          []DraftItem `json:"items"`
	SeenAccountIDs *[]string   `json:"seen_account_ids"`
}

// TableAccountRef identifica una cuenta abierta que otra terminal creó en la mesa
// (detalle del rechazo TABLE_HAS_ACCOUNT).
type TableAccountRef struct {
	ID    string `json:"id"`
	Folio int64  `json:"folio"`
	Name  string `json:"name"`
}

type Account struct {
	ID                string          `json:"id"`
	Folio             int64           `json:"folio"`
	Name              string          `json:"name"`
	Mode              string          `json:"mode"`
	State             string          `json:"state"`
	TableID           *string         `json:"table_id"`
	TableName         *string         `json:"table_name"`
	PriceListID       string          `json:"price_list_id"`
	CustomerName      string          `json:"customer_name"`
	Phone             string          `json:"phone"`
	Origin            string          `json:"origin"`
	Notes             string          `json:"notes"`
	ScheduledAt       *time.Time      `json:"scheduled_at"`
	ScheduledLocal    string          `json:"scheduled_local"`
	ScheduleKind      *string         `json:"schedule_kind"`
	AssignedUserID    string          `json:"assigned_user_id"`
	AssignedName      string          `json:"assigned_name"`
	CreatedAt         time.Time       `json:"created_at"`
	UpdatedAt         time.Time       `json:"updated_at"`
	Revision          int             `json:"revision"`
	MergedIntoID      *string         `json:"merged_into_id"`
	ElapsedSeconds    int64           `json:"elapsed_seconds"`
	Urgency           string          `json:"urgency"`
	PreparationState  string          `json:"preparation_state"`
	PaymentState      string          `json:"payment_state"`
	DeliveryState     string          `json:"delivery_state"`
	Finance           *payments.Brief `json:"finance"`
	ConfirmedQuantity int             `json:"confirmed_quantity"`
	DraftQuantity     int             `json:"draft_quantity"`
	Totals            Totals          `json:"totals"`
	// Releasable: la cuenta está abierta y vacía en sentido estricto, por lo que «Liberar mesa»
	// procede sin motivo (etapa 7.1). Lo calcula Go en cada lectura.
	Releasable bool `json:"releasable"`
}

type Totals struct {
	Subtotal string `json:"subtotal"`
	Discount string `json:"discount"`
	Total    string `json:"total"`
}

type DraftItem struct {
	ID string `json:"id"`
	catalog.LineRequest
}

type ConfirmedLine struct {
	PreparationState string              `json:"preparation_state"`
	VersionID        string              `json:"version_id"`
	ItemID           string              `json:"item_id"`
	OriginalOrderID  string              `json:"original_order_id"`
	Version          int                 `json:"version"`
	State            string              `json:"state"`
	Quantity         int                 `json:"quantity"`
	Request          catalog.LineRequest `json:"request"`
	QuoteLine        catalog.QuoteLine   `json:"quote_line"`
	Totals           Totals              `json:"totals"`
	CreatedAt        time.Time           `json:"created_at"`
}

type Bill struct {
	Account    Account         `json:"account"`
	Draft      []DraftItem     `json:"draft"`
	Lines      []ConfirmedLine `json:"lines"`
	ServerTime time.Time       `json:"server_time"`
	Timezone   string          `json:"timezone"`
}

type QuoteResult struct {
	AccountID     string         `json:"account_id"`
	Revision      int            `json:"revision"`
	Fingerprint   string         `json:"fingerprint"`
	Quote         *catalog.Quote `json:"quote"`
	Combined      Totals         `json:"combined"`
	PricingPolicy string         `json:"pricing_policy"`
}

type Selection struct {
	VersionID string `json:"version_id"`
	Quantity  int    `json:"quantity"`
}

type TransferInput struct {
	Mutation
	TargetAccountID *string       `json:"target_account_id"`
	TargetRevision  int           `json:"target_revision"`
	NewAccount      *AccountInput `json:"new_account"`
	Selections      []Selection   `json:"selections"`
}

type EditInput struct {
	Mutation
	VersionID           string              `json:"version_id"`
	Line                catalog.LineRequest `json:"line"`
	ExpectedFingerprint string              `json:"expected_fingerprint"`
	Reason              string              `json:"reason"`
}

type Unit struct {
	ID        string `json:"id"`
	ItemID    string `json:"item_id"`
	VersionID string `json:"version_id"`
	Ordinal   int    `json:"ordinal"`
	Subtotal  string `json:"subtotal"`
	Discount  string `json:"discount"`
	Total     string `json:"total"`
}

type HistoryEvent struct {
	ID               string          `json:"id"`
	Sequence         int64           `json:"sequence"`
	AccountID        string          `json:"account_id"`
	RelatedAccountID *string         `json:"related_account_id"`
	Kind             string          `json:"kind"`
	Payload          json.RawMessage `json:"payload"`
	AuthorName       string          `json:"author_name"`
	DeviceName       string          `json:"device_name"`
	CreatedAt        time.Time       `json:"created_at"`
	IntegrationState string          `json:"integration_state"`
}
