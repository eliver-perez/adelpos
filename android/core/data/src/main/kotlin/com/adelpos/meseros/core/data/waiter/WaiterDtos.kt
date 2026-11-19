package com.adelpos.meseros.core.data.waiter

import com.adelpos.meseros.core.data.api.MoneySerializer
import com.adelpos.meseros.core.data.catalog.CustomizationDto
import com.adelpos.meseros.core.data.catalog.LineRequestDto
import com.adelpos.meseros.core.domain.money.Money
import kotlinx.serialization.SerialName
import kotlinx.serialization.Serializable

/*
 * Formas del contrato v1 que usa el modo Mesero (`backend/internal/apicontract/openapi.json`,
 * `docs/API_V1.md` «Cuentas y pedidos», «Modo Mesero» y «Pedido capturado sin conexión»). Al leer,
 * las propiedades opcionales del contrato llevan un valor por omisión (ApiJson ignora lo que esta
 * versión no conoce); al escribir se usa CustomizationJson, que escribe los null del contrato
 * (`"variant_id": null`). Los importes son cadenas decimales (MoneySerializer), nunca números.
 */

// --- Mesas (getRestaurantOverview) ---

@Serializable
data class RestaurantOverviewDto(
    val areas: List<AreaDto>,
    val tables: List<TableDto>,
    val settings: RestaurantSettingsDto? = null,
    val timezone: String? = null,
)

@Serializable
data class AreaDto(
    val id: String,
    val name: String,
    @SerialName("sort_order") val sortOrder: Int = 0,
    val active: Boolean = true,
)

@Serializable
data class TableDto(
    val id: String,
    @SerialName("area_id") val areaId: String,
    val name: String,
    val shape: String? = null,
    val x: Int = 0,
    val y: Int = 0,
    val width: Int = 100,
    val height: Int = 80,
    val seats: Int = 1,
    val active: Boolean = true,
    @SerialName("out_of_service") val outOfService: Boolean = false,
    @SerialName("account_count") val accountCount: Int = 0,
    val state: String,
    @SerialName("occupied_seconds") val occupiedSeconds: Long = 0,
    @Serializable(with = MoneySerializer::class) val balance: Money = Money("0.00"),
    @SerialName("long_open") val longOpen: Boolean = false,
)

@Serializable
data class RestaurantSettingsDto(
    @SerialName("warning_minutes") val warningMinutes: Int,
    @SerialName("overdue_minutes") val overdueMinutes: Int,
    @SerialName("long_open_hours") val longOpenHours: Int,
)

// --- Recuperación del estado (getRecoveryState) ---

@Serializable
data class RecoveryStateDto(
    @SerialName("catalog_version") val catalogVersion: Long = 0,
    @SerialName("catalog_epoch") val catalogEpoch: String? = null,
    val user: PrincipalDto,
    val tables: List<TableDto>? = null,
    val accounts: List<AccountDto>? = null,
    @SerialName("event_cursor") val eventCursor: Long? = null,
)

@Serializable
data class PrincipalDto(
    val id: String,
    val permissions: List<String> = emptyList(),
)

// --- Cuentas ---

@Serializable
data class TotalsDto(
    @Serializable(with = MoneySerializer::class) val subtotal: Money,
    @Serializable(with = MoneySerializer::class) val discount: Money,
    @Serializable(with = MoneySerializer::class) val total: Money,
)

@Serializable
data class AccountDto(
    val id: String,
    val folio: Int = 0,
    val name: String = "",
    val mode: String = "dine_in",
    val state: String,
    @SerialName("table_id") val tableId: String? = null,
    @SerialName("table_name") val tableName: String? = null,
    val revision: Int,
    @SerialName("preparation_state") val preparationState: String? = null,
    @SerialName("confirmed_quantity") val confirmedQuantity: Int = 0,
    @SerialName("draft_quantity") val draftQuantity: Int = 0,
    val totals: TotalsDto? = null,
    @SerialName("assigned_name") val assignedName: String? = null,
    @SerialName("elapsed_seconds") val elapsedSeconds: Long? = null,
    @SerialName("long_open") val longOpen: Boolean = false,
    @SerialName("merged_into_id") val mergedIntoId: String? = null,
    // Etapa A2 (Para llevar): Go siempre los entrega (vacíos en salón).
    @SerialName("customer_name") val customerName: String = "",
    val phone: String = "",
    val origin: String = "",
    @SerialName("payment_state") val paymentState: String? = null,
    @SerialName("delivery_state") val deliveryState: String? = null,
    @SerialName("created_at") val createdAt: String? = null,
    // Etapa A3 (§4.10): Go dice si es una cuenta vacía que se puede descartar.
    val discardable: Boolean = false,
)

/**
 * `DraftItem`: renglón del borrador (también el `replaces_draft_items` del pedido sin conexión).
 */
@Serializable
data class DraftItemDto(
    val id: String,
    @SerialName("product_id") val productId: String,
    @SerialName("variant_id") val variantId: String?,
    val quantity: Int,
    val customization: CustomizationDto,
)

@Serializable
data class QuoteLineDto(
    @SerialName("product_id") val productId: String,
    val name: String,
    @Serializable(with = MoneySerializer::class) @SerialName("unit_price") val unitPrice: Money,
    val quantity: Int,
    @Serializable(with = MoneySerializer::class) val total: Money,
)

@Serializable
data class ConfirmedLineDto(
    @SerialName("preparation_state") val preparationState: String? = null,
    @SerialName("version_id") val versionId: String,
    @SerialName("item_id") val itemId: String,
    val state: String,
    val quantity: Int,
    val request: LineRequestDto,
    @SerialName("quote_line") val quoteLine: QuoteLineDto,
    val totals: TotalsDto,
)

@Serializable
data class BillDto(
    val account: AccountDto,
    val draft: List<DraftItemDto> = emptyList(),
    val lines: List<ConfirmedLineDto> = emptyList(),
)

@Serializable
data class QuoteDto(
    val lines: List<QuoteLineDto> = emptyList(),
    @Serializable(with = MoneySerializer::class) val total: Money? = null,
)

/** `QuoteResult` de `previewDraft` y `previewEdit`. */
@Serializable
data class QuoteResultDto(
    @SerialName("account_id") val accountId: String,
    val revision: Int,
    val fingerprint: String = "",
    val quote: QuoteDto? = null,
    val combined: TotalsDto,
)

// --- Escrituras de cuentas (con operation_id) ---

/**
 * `openAccount` con su primer producto (creación diferida). En una mesa lleva `table_id` y
 * `seen_account_ids`; en un pedido para llevar (etapa A2), `table_id: null`, sin cuentas vistas y
 * con `origin` («Para llevar»), `customer_name` y `phone` (lo que vale lo predeterminado no se
 * escribe: el cuerpo de una mesa queda igual que en la A1).
 */
@Serializable
data class OpenAccountBody(
    @SerialName("operation_id") val operationId: String,
    val mode: String,
    @SerialName("table_id") val tableId: String?,
    @SerialName("seen_account_ids") val seenAccountIds: List<String>? = null,
    val items: List<DraftItemDto>,
    val origin: String? = null,
    @SerialName("customer_name") val customerName: String? = null,
    val phone: String? = null,
)

@Serializable
data class SaveDraftBody(
    @SerialName("operation_id") val operationId: String,
    val revision: Int,
    val items: List<DraftItemDto>,
)

/** `operation_id` y la revisión vigente (descartar una cuenta vacía, `discardAccount`). */
@Serializable
data class MutationBody(
    @SerialName("operation_id") val operationId: String,
    val revision: Int,
)

@Serializable
data class ConfirmDraftBody(
    @SerialName("operation_id") val operationId: String,
    val revision: Int,
    @SerialName("expected_fingerprint") val expectedFingerprint: String,
)

@Serializable
data class CancelUnitsBody(
    @SerialName("operation_id") val operationId: String,
    val revision: Int,
    @SerialName("version_id") val versionId: String,
    val quantity: Int,
    val reason: String,
)

@Serializable
data class EditPreviewBody(
    val revision: Int,
    @SerialName("version_id") val versionId: String,
    val line: LineRequestDto,
)

@Serializable
data class EditUnitsBody(
    @SerialName("operation_id") val operationId: String,
    val revision: Int,
    @SerialName("version_id") val versionId: String,
    val line: LineRequestDto,
    @SerialName("expected_fingerprint") val expectedFingerprint: String,
    val reason: String,
)

// --- Pedido capturado sin conexión (submitOfflineOrder) ---

@Serializable
data class OfflineDestinationDto(
    @SerialName("account_id") val accountId: String? = null,
    @SerialName("table_id") val tableId: String? = null,
    @SerialName("seen_account_ids") val seenAccountIds: List<String>? = null,
    /** Un pedido para llevar nuevo (etapa A2): exclusivo con [accountId] y [tableId]. */
    val takeaway: OfflineTakeawayDto? = null,
)

/** `destination.takeaway`: el nombre y el teléfono del cliente (opcionales; Go los recorta). */
@Serializable
data class OfflineTakeawayDto(
    @SerialName("customer_name") val customerName: String,
    val phone: String,
)

@Serializable
data class OfflineItemDto(
    val id: String,
    @SerialName("product_id") val productId: String,
    @SerialName("variant_id") val variantId: String?,
    val quantity: Int,
    val customization: CustomizationDto,
    @SerialName("seen_unit_price") val seenUnitPrice: String,
)

@Serializable
data class OfflineChangeDto(
    val kind: String,
    @SerialName("item_id") val itemId: String? = null,
    @SerialName("product_id") val productId: String? = null,
    @SerialName("product_name") val productName: String = "",
    val before: String? = null,
    val now: String? = null,
    val message: String = "",
    val acceptable: Boolean = false,
    /**
     * Solo en `items_in_account` (A1-D18). Sin valor no se escribe: el eco de la confirmación lleva
     * exactamente lo que dio Go.
     */
    @SerialName("account_id") val accountId: String? = null,
)

@Serializable
data class OfflineConfirmationDto(
    @SerialName("catalog_version") val catalogVersion: Long,
    val changes: List<OfflineChangeDto>,
)

@Serializable
data class OfflineOrderBody(
    @SerialName("operation_id") val operationId: String,
    @SerialName("catalog_version") val catalogVersion: Long,
    @SerialName("captured_at") val capturedAt: String,
    val destination: OfflineDestinationDto,
    val items: List<OfflineItemDto>,
    val confirmation: OfflineConfirmationDto? = null,
    @SerialName("replaces_draft_items") val replacesDraftItems: List<DraftItemDto>? = null,
)

@Serializable
data class OfflineOrderResultDto(
    val status: String = "",
    @SerialName("operation_id") val operationId: String = "",
    @SerialName("order_id") val orderId: String = "",
    @SerialName("account_id") val accountId: String,
    @SerialName("removed_draft_item_ids") val removedDraftItemIds: List<String> = emptyList(),
    /** Partidas que ya estaban en cocina y no se volvieron a enviar (A1-D18). */
    @SerialName("already_sent_item_ids") val alreadySentItemIds: List<String> = emptyList(),
)

/** `details` de `ORDER_REQUIRES_CONFIRMATION`. */
@Serializable
data class OfflineRejectionDetailsDto(
    @SerialName("catalog_version") val catalogVersion: Long,
    @SerialName("requires_changes") val requiresChanges: Boolean = false,
    val changes: List<OfflineChangeDto> = emptyList(),
)

/**
 * `details` de `OFFLINE_ORDER_ALREADY_ACCEPTED`: [origin] `offline_order` (la clave ya entró como
 * pedido sin conexión) o `confirm_draft` (la clave es la de un «Enviar a cocina» en línea que ya se
 * aplicó en esa cuenta; A1-D18).
 */
@Serializable
data class AlreadyAcceptedDetailsDto(
    val origin: String? = null,
    val result: OfflineOrderResultDto? = null,
)

/** `details` de `TABLE_HAS_ACCOUNT` (`openAccount`). */
@Serializable
data class TableHasAccountDetailsDto(val accounts: List<TableAccountDto> = emptyList())

@Serializable data class TableAccountDto(val id: String, val folio: Int = 0, val name: String = "")

/** `details` de `OPERATION_VALIDATION` con la cuenta que ya no está abierta (§6.4). */
@Serializable
data class AccountNotOpenDetailsDto(
    val reason: String = "",
    @SerialName("account_state") val accountState: String? = null,
    @SerialName("merged_into_id") val mergedIntoId: String? = null,
)

// --- Para llevar (etapa A2): nombre y teléfono, y «Entregado» ---

/**
 * `updateTakeawayCustomer` (A2-D1): los dos campos siempre (vacío quita el dato); no lleva ni sube
 * la revisión.
 */
@Serializable
data class TakeawayCustomerBody(
    @SerialName("operation_id") val operationId: String,
    @SerialName("customer_name") val customerName: String,
    val phone: String,
)

/** `deliverTakeaway` (A2-D2). */
@Serializable data class DeliverBody(@SerialName("operation_id") val operationId: String)

/**
 * Respuesta de `deliverTakeaway`: al mesero `finance` siempre llega null (A2-D11) y esta versión no
 * lo lee.
 */
@Serializable
data class DeliveryResultDto(val delivered: Boolean = false, val closed: Boolean = false)
