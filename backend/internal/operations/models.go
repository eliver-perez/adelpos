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
	// LongOpen (etapa 9D, §3.4): la cuenta abierta más antigua de la mesa lleva más de
	// restaurant_settings.long_open_hours horas abierta. Lo calcula Go en cada lectura.
	LongOpen bool `json:"long_open"`
}

type Settings struct {
	WarningMinutes int `json:"warning_minutes"`
	OverdueMinutes int `json:"overdue_minutes"`
	// LongOpenHours (etapa 9D, §3.4) es el umbral de cuenta abierta por mucho tiempo, de 1 a 72 horas
	// (12 por omisión). Se entrega con el mapa y las cuentas; PUT restaurant/settings no lo cambia: se
	// edita en «Funciones del establecimiento» (PUT settings/long-open-hours, settings.features.manage).
	LongOpenHours int `json:"long_open_hours"`
	Revision      int `json:"revision"`
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
	// Etapa 9D. Discardable: «Descartar» procede sin motivo (abierta, sin productos enviados ni
	// registros financieros o de cocina, aunque tenga borrador); una cuenta descartable no se cancela
	// (cancel-account responde ACCOUNT_DISCARDABLE). CancelBlocker: vacío si «Cancelar cuenta» procede
	// o si la cuenta es descartable, "payments" si conserva pagos o propinas (primero la devolución) o
	// "adjustments" si tiene un descuento, cortesía o cargo vigente (primero quitarlo).
	// CancelRequiresAuthorization: el usuario no tiene accounts.cancel y la cancelación pedirá la
	// autorización de un supervisor (false en las descartables: no se cancelan).
	// LongOpen: lleva abierta más de restaurant_settings.long_open_hours horas (§3.4).
	Discardable                 bool   `json:"discardable"`
	CancelBlocker               string `json:"cancel_blocker"`
	CancelRequiresAuthorization bool   `json:"cancel_requires_authorization"`
	LongOpen                    bool   `json:"long_open"`
	// CancelUnitsRequiresAuthorization (etapa 9F, §7.2): la cuenta está abierta y el usuario que lee
	// no tiene orders.cancel, así que «Cancelar unidades» pedirá la autorización de un supervisor. Para
	// reducir la cantidad desde «Modificar», lo dice la previsualización (edit-preview).
	CancelUnitsRequiresAuthorization bool `json:"cancel_units_requires_authorization"`
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
	// Cancellation (etapa 9D) es el registro de «Cancelar cuenta» de una cuenta anulada con motivo
	// del catálogo; null en cualquier otra cuenta (también las anuladas antes, liberadas o descartadas).
	Cancellation *AccountCancellation `json:"cancellation"`
}

type QuoteResult struct {
	AccountID     string         `json:"account_id"`
	Revision      int            `json:"revision"`
	Fingerprint   string         `json:"fingerprint"`
	Quote         *catalog.Quote `json:"quote"`
	Combined      Totals         `json:"combined"`
	PricingPolicy string         `json:"pricing_policy"`
	// CancelUnitsRequiresAuthorization (etapa 9F, solo en edit-preview): la modificación reduce la
	// cantidad de una partida ya enviada y el usuario no tiene orders.cancel, así que «edit» pedirá la
	// autorización de un supervisor. Falso en las demás cotizaciones.
	CancelUnitsRequiresAuthorization bool `json:"cancel_units_requires_authorization"`
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
	// Authorization (etapa 9F) es la autorización de un supervisor para reducir la cantidad sin
	// orders.cancel; se ignora si basta el permiso propio o si la modificación no reduce.
	Authorization *payments.AuthorizationInput `json:"authorization"`
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
