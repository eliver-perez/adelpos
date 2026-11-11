// Package documents es el modelo neutral de plantillas de documentos impresos (etapa 9B): tipos de
// documento, bloques ordenados y su validación, datos tipados de cada familia, el renderizador único
// que produce la representación intermedia (IR) para la vista previa y la impresión, las plantillas
// predeterminadas que reproducen byte a byte los formatos anteriores, los datos de muestra, la
// codificación ESC/POS de un layout con atributos y el acceso a las plantillas guardadas.
//
// Reglas de arquitectura:
//   - no importa kitchen, payments, operations ni administration: esos módulos convierten sus
//     estructuras a los datos tipados de este paquete (KitchenData, CommercialData, CashData);
//   - Go es la única autoridad de las líneas impresas: React solo muestra la IR que devuelve Go;
//   - una plantilla es una lista ordenada de bloques de una lista cerrada; nunca HTML, CSS, JSON
//     libre del usuario ni comandos ESC/POS;
//   - los bloques protegidos (esperado y diferencias de caja) solo existen en los cortes y se
//     renderizan solo cuando quien imprime puede verlos (RenderOptions.IncludeExpected).
package documents

// Kind es uno de los diez tipos de documento de la lista cerrada (§20 de la especificación).
type Kind string

const (
	KindReceipt             Kind = "receipt"
	KindPrecheck            Kind = "precheck"
	KindKitchenOrder        Kind = "kitchen_order"
	KindKitchenModification Kind = "kitchen_modification"
	KindKitchenCancellation Kind = "kitchen_cancellation"
	KindCashIncome          Kind = "cash_income"
	KindCashWithdrawal      Kind = "cash_withdrawal"
	KindCashExpense         Kind = "cash_expense"
	KindCashPartialCut      Kind = "cash_partial_cut"
	KindCashFinalCut        Kind = "cash_final_cut"
)

// Family agrupa los tipos que comparten datos y reglas de corte de línea.
type Family string

const (
	FamilyCommercial Family = "commercial"
	FamilyKitchen    Family = "kitchen"
	FamilyCash       Family = "cash"
)

// Tipos de ticket de la cola de impresión que agrega la etapa 9B (kitchen_tickets.kind). Ambos son
// documentos ya renderizados: se imprimen y copian tal cual y no aparecen entre las comandas.
const (
	// TicketKindSample es una muestra impresa desde el diseñador, con datos ficticios.
	TicketKindSample = "document_sample"
	// TicketKindRerender es un documento histórico impreso con la plantilla vigente.
	TicketKindRerender = "document_rerender"
)

// KindInfo describe un tipo de documento para el catálogo del editor.
type KindInfo struct {
	Kind   Kind   `json:"kind"`
	Name   string `json:"name"`
	Family Family `json:"family"`
}

// Kinds es la lista cerrada de tipos, en el orden en que los muestra el editor.
var Kinds = []KindInfo{
	{KindReceipt, "Ticket de venta", FamilyCommercial},
	{KindPrecheck, "Precuenta", FamilyCommercial},
	{KindKitchenOrder, "Comanda", FamilyKitchen},
	{KindKitchenModification, "Aviso de modificación", FamilyKitchen},
	{KindKitchenCancellation, "Aviso de cancelación", FamilyKitchen},
	{KindCashIncome, "Comprobante de ingreso", FamilyCash},
	{KindCashWithdrawal, "Comprobante de retiro", FamilyCash},
	{KindCashExpense, "Comprobante de gasto", FamilyCash},
	{KindCashPartialCut, "Corte parcial", FamilyCash},
	{KindCashFinalCut, "Corte definitivo", FamilyCash},
}

// ParseKind valida una clave de tipo recibida del cliente.
func ParseKind(value string) (Kind, bool) {
	for _, info := range Kinds {
		if string(info.Kind) == value {
			return info.Kind, true
		}
	}
	return "", false
}

// Info devuelve la descripción del tipo; un tipo desconocido devuelve su clave sin familia.
func (kind Kind) Info() KindInfo {
	for _, info := range Kinds {
		if info.Kind == kind {
			return info
		}
	}
	return KindInfo{Kind: kind, Name: string(kind)}
}

// Family es la familia del tipo.
func (kind Kind) Family() Family {
	return kind.Info().Family
}

// Name es el nombre en pantalla del tipo.
func (kind Kind) Name() string {
	return kind.Info().Name
}

// IsCut indica los cortes parcial y definitivo, los únicos tipos con bloques protegidos.
func (kind Kind) IsCut() bool {
	return kind == KindCashPartialCut || kind == KindCashFinalCut
}

// IsVoucher indica los comprobantes de ingreso, retiro y gasto.
func (kind Kind) IsVoucher() bool {
	return kind == KindCashIncome || kind == KindCashWithdrawal || kind == KindCashExpense
}

// KitchenKind traduce el tipo de ticket de cocina (kitchen_tickets.kind) a su tipo de documento.
// La prueba técnica de impresora («test») usa la plantilla de la comanda, como antes de la 9B.
func KitchenKind(ticketKind string) (Kind, bool) {
	switch ticketKind {
	case "normal", "test":
		return KindKitchenOrder, true
	case "modification":
		return KindKitchenModification, true
	case "cancellation":
		return KindKitchenCancellation, true
	}
	return "", false
}

// CashKind traduce el tipo de documento de caja (cash_documents.kind) a su tipo de documento.
func CashKind(documentKind string) (Kind, bool) {
	switch documentKind {
	case "income":
		return KindCashIncome, true
	case "withdrawal":
		return KindCashWithdrawal, true
	case "expense":
		return KindCashExpense, true
	case "partial_cut":
		return KindCashPartialCut, true
	case "final_cut":
		return KindCashFinalCut, true
	}
	return "", false
}

// Permission es un permiso del diseñador de documentos.
type Permission struct {
	Key         string
	Description string
}

// Permisos de la etapa 9B. Se registran en Go (administration los agrega a su registro) y en la
// migración 0018 con la misma descripción, concedidos solo al administrador.
const (
	PermissionTemplatesRead   = "documents.templates.read"
	PermissionTemplatesManage = "documents.templates.manage"
	PermissionPreviewReal     = "documents.preview.real"
	PermissionSamplesPrint    = "documents.samples.print"
	PermissionIdentityManage  = "documents.identity.manage"
	PermissionRenderCurrent   = "documents.history.render_current"
)

// Permissions enumera los permisos del diseñador de documentos.
var Permissions = []Permission{
	{PermissionTemplatesRead, "Consultar plantillas de documentos, versiones y vista previa con datos de muestra"},
	{PermissionTemplatesManage, "Crear, editar, duplicar, renombrar, activar y restaurar plantillas de documentos"},
	{PermissionPreviewReal, "Previsualizar plantillas con un documento real (además del permiso de su origen)"},
	{PermissionSamplesPrint, "Imprimir muestras de plantillas de documentos"},
	{PermissionIdentityManage, "Modificar la identidad documental y el logotipo del establecimiento"},
	{PermissionRenderCurrent, "Imprimir un documento histórico con la plantilla vigente (además del permiso de reimprimir)"},
}

// Scope es el ámbito SSE del diseñador: lo reciben quienes consultan plantillas.
const Scope = "documents"

// EventTemplatesChanged avisa que cambió una plantilla o la activa de un tipo.
const EventTemplatesChanged = "documents.templates.changed"
