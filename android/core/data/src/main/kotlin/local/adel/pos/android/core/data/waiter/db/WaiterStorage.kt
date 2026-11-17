package local.adel.pos.android.core.data.waiter.db

import kotlinx.serialization.SerialName
import kotlinx.serialization.Serializable
import kotlinx.serialization.builtins.ListSerializer
import kotlinx.serialization.builtins.MapSerializer
import kotlinx.serialization.builtins.serializer
import local.adel.pos.android.core.data.catalog.CustomizationJson
import local.adel.pos.android.core.data.waiter.DraftItemDto
import local.adel.pos.android.core.data.waiter.OfflineChangeDto
import local.adel.pos.android.core.data.waiter.WaiterMapper
import local.adel.pos.android.core.domain.money.Money
import local.adel.pos.android.core.domain.waiter.Capture
import local.adel.pos.android.core.domain.waiter.DraftLine
import local.adel.pos.android.core.domain.waiter.DraftOperation
import local.adel.pos.android.core.domain.waiter.DraftSync
import local.adel.pos.android.core.domain.waiter.OnlineConfirm
import local.adel.pos.android.core.domain.waiter.OrderConfirmation
import local.adel.pos.android.core.domain.waiter.OrderDestination
import local.adel.pos.android.core.domain.waiter.OrderLine
import local.adel.pos.android.core.domain.waiter.OrderReview
import local.adel.pos.android.core.domain.waiter.OrderState
import local.adel.pos.android.core.domain.waiter.PendingOrder
import local.adel.pos.android.core.domain.waiter.ReviewReason
import local.adel.pos.android.core.domain.waiter.TableAccountRef

/*
 * Documentos JSON de las tablas del modo Mesero (`AppDatabase` v4). Los renglones van con la forma `DraftItem` del contrato (la misma
 * que se envía a Go), así que lo guardado es exactamente lo que se mandará; los importes, como
 * cadenas decimales.
 */

@Serializable
private data class StoredOperation(
    val kind: String,
    val id: String? = null,
    val delta: Int? = null,
    val item: DraftItemDto? = null,
    val fallback: DraftItemDto? = null,
)

@Serializable
private data class StoredTableAccount(val id: String, val folio: Int, val name: String)

@Serializable
private data class StoredDestination(
    @SerialName("table_id") val tableId: String,
    @SerialName("table_name") val tableName: String,
    @SerialName("account_id") val accountId: String?,
    @SerialName("account_name") val accountName: String?,
    @SerialName("seen_account_ids") val seenAccountIds: List<String>,
)

@Serializable
private data class StoredLine(
    val item: DraftItemDto,
    @SerialName("product_name") val productName: String,
    @SerialName("seen_unit_price") val seenUnitPrice: String,
)

@Serializable
private data class StoredConfirmation(
    @SerialName("catalog_version") val catalogVersion: Long,
    val changes: List<OfflineChangeDto>,
)

@Serializable
private data class StoredReview(
    val reason: String,
    val message: String,
    val changes: List<OfflineChangeDto>,
    @SerialName("catalog_version") val catalogVersion: Long?,
)

@Serializable
private data class StoredOnlineConfirm(
    @SerialName("operation_id") val operationId: String,
    @SerialName("account_id") val accountId: String,
    val revision: Int,
    val fingerprint: String,
)

/** Conversión entre el dominio y las filas de las tablas del modo Mesero. */
object WaiterStorage {
    private val json = CustomizationJson
    private val lines = ListSerializer(DraftItemDto.serializer())
    private val operations = ListSerializer(StoredOperation.serializer())
    private val strings = ListSerializer(String.serializer())
    private val prices = MapSerializer(String.serializer(), String.serializer())
    private val tableAccounts = ListSerializer(StoredTableAccount.serializer())
    private val orderLines = ListSerializer(StoredLine.serializer())

    fun toEntity(capture: Capture, nowMillis: Long): CaptureEntity =
        CaptureEntity(
            id = capture.id,
            waiterUserId = capture.waiterUserId,
            tableId = capture.tableId,
            tableName = capture.tableName,
            accountId = capture.accountId,
            accountName = capture.accountName,
            seenAccountIds = json.encodeToString(strings, capture.seenAccountIds),
            base = encodeLines(capture.base),
            baseRevision = capture.baseRevision,
            operations = json.encodeToString(operations, capture.operations.map(::storedOperation)),
            ownItemIds = json.encodeToString(strings, capture.ownItemIds.sorted()),
            seenPrices =
                json.encodeToString(prices, capture.seenPrices.mapValues { it.value.decimal }),
            sync = capture.sync.name,
            syncMessage = capture.syncMessage,
            tableAccounts =
                json.encodeToString(
                    tableAccounts,
                    capture.tableAccounts.map { StoredTableAccount(it.id, it.folio, it.name) },
                ),
            pendingOperationId = capture.pendingOperationId,
            createdAt = capture.createdAtMillis,
            updatedAt = nowMillis,
        )

    fun toCapture(entity: CaptureEntity): Capture =
        Capture(
            id = entity.id,
            waiterUserId = entity.waiterUserId,
            tableId = entity.tableId,
            tableName = entity.tableName,
            accountId = entity.accountId,
            accountName = entity.accountName,
            seenAccountIds = json.decodeFromString(strings, entity.seenAccountIds),
            base = decodeLines(entity.base),
            baseRevision = entity.baseRevision,
            operations = json.decodeFromString(operations, entity.operations).map(::operation),
            ownItemIds = json.decodeFromString(strings, entity.ownItemIds).toSet(),
            seenPrices =
                json.decodeFromString(prices, entity.seenPrices).mapValues { Money(it.value) },
            sync = DraftSync.valueOf(entity.sync),
            syncMessage = entity.syncMessage,
            tableAccounts =
                json.decodeFromString(tableAccounts, entity.tableAccounts).map {
                    TableAccountRef(it.id, it.folio, it.name)
                },
            pendingOperationId = entity.pendingOperationId,
            createdAtMillis = entity.createdAt,
        )

    fun toEntity(order: PendingOrder): PendingOrderEntity =
        PendingOrderEntity(
            operationId = order.operationId,
            waiterUserId = order.waiterUserId,
            waiterName = order.waiterName,
            state = order.state.storedName,
            destination =
                json.encodeToString(
                    StoredDestination.serializer(),
                    StoredDestination(
                        tableId = order.destination.tableId,
                        tableName = order.destination.tableName,
                        accountId = order.destination.accountId,
                        accountName = order.destination.accountName,
                        seenAccountIds = order.destination.seenAccountIds,
                    ),
                ),
            lines =
                json.encodeToString(
                    orderLines,
                    order.lines.map { line ->
                        StoredLine(
                            item = WaiterMapper.draftItem(line.item),
                            productName = line.productName,
                            seenUnitPrice = line.seenUnitPrice.decimal,
                        )
                    },
                ),
            replaces = encodeLines(order.replaces),
            catalogVersion = order.catalogVersion,
            catalogEpoch = order.catalogEpoch,
            capturedAt = order.capturedAtMillis,
            confirmation =
                order.confirmation?.let { confirmation ->
                    json.encodeToString(
                        StoredConfirmation.serializer(),
                        StoredConfirmation(
                            catalogVersion = confirmation.catalogVersion,
                            changes = confirmation.changes.map(WaiterMapper::changeDto),
                        ),
                    )
                },
            review =
                order.review?.let { review ->
                    json.encodeToString(
                        StoredReview.serializer(),
                        StoredReview(
                            reason = review.reason.name,
                            message = review.message,
                            changes = review.changes.map(WaiterMapper::changeDto),
                            catalogVersion = review.catalogVersion,
                        ),
                    )
                },
            attempts = order.attempts,
            lastError = order.lastError,
            credentialRejected = order.credentialRejected,
            onlineConfirm =
                order.onlineConfirm?.let { online ->
                    json.encodeToString(
                        StoredOnlineConfirm.serializer(),
                        StoredOnlineConfirm(
                            operationId = online.operationId,
                            accountId = online.accountId,
                            revision = online.revision,
                            fingerprint = online.fingerprint,
                        ),
                    )
                },
            receivedAccountId = order.receivedAccountId,
            receivedAt = order.receivedAtMillis,
            updatedAt = order.updatedAtMillis,
        )

    fun toOrder(entity: PendingOrderEntity): PendingOrder {
        val destination = json.decodeFromString(StoredDestination.serializer(), entity.destination)
        return PendingOrder(
            operationId = entity.operationId,
            waiterUserId = entity.waiterUserId,
            waiterName = entity.waiterName,
            state = OrderState.fromStoredName(entity.state),
            destination =
                OrderDestination(
                    tableId = destination.tableId,
                    tableName = destination.tableName,
                    accountId = destination.accountId,
                    accountName = destination.accountName,
                    seenAccountIds = destination.seenAccountIds,
                ),
            lines =
                json.decodeFromString(orderLines, entity.lines).map { stored ->
                    OrderLine(
                        item = WaiterMapper.draftLine(stored.item),
                        productName = stored.productName,
                        seenUnitPrice = Money(stored.seenUnitPrice),
                    )
                },
            replaces = decodeLines(entity.replaces),
            catalogVersion = entity.catalogVersion,
            catalogEpoch = entity.catalogEpoch,
            capturedAtMillis = entity.capturedAt,
            confirmation =
                entity.confirmation?.let { text ->
                    val stored = json.decodeFromString(StoredConfirmation.serializer(), text)
                    OrderConfirmation(
                        catalogVersion = stored.catalogVersion,
                        changes = stored.changes.map(WaiterMapper::change),
                    )
                },
            review =
                entity.review?.let { text ->
                    val stored = json.decodeFromString(StoredReview.serializer(), text)
                    OrderReview(
                        reason = ReviewReason.valueOf(stored.reason),
                        message = stored.message,
                        changes = stored.changes.map(WaiterMapper::change),
                        catalogVersion = stored.catalogVersion,
                    )
                },
            attempts = entity.attempts,
            lastError = entity.lastError,
            credentialRejected = entity.credentialRejected,
            onlineConfirm =
                entity.onlineConfirm?.let { text ->
                    val stored = json.decodeFromString(StoredOnlineConfirm.serializer(), text)
                    OnlineConfirm(
                        operationId = stored.operationId,
                        accountId = stored.accountId,
                        revision = stored.revision,
                        fingerprint = stored.fingerprint,
                    )
                },
            receivedAccountId = entity.receivedAccountId,
            receivedAtMillis = entity.receivedAt,
            updatedAtMillis = entity.updatedAt,
        )
    }

    fun encodeStrings(values: List<String>): String = json.encodeToString(strings, values)

    fun decodeStrings(text: String): List<String> = json.decodeFromString(strings, text)

    private fun encodeLines(values: List<DraftLine>): String =
        json.encodeToString(lines, values.map(WaiterMapper::draftItem))

    private fun decodeLines(text: String): List<DraftLine> =
        json.decodeFromString(lines, text).map(WaiterMapper::draftLine)

    private fun storedOperation(operation: DraftOperation): StoredOperation =
        when (operation) {
            is DraftOperation.Add ->
                StoredOperation(kind = ADD, item = WaiterMapper.draftItem(operation.item))
            is DraftOperation.Quantity ->
                StoredOperation(
                    kind = QUANTITY,
                    id = operation.id,
                    delta = operation.delta,
                    fallback = WaiterMapper.draftItem(operation.fallback),
                )
            is DraftOperation.Replace ->
                StoredOperation(
                    kind = REPLACE,
                    id = operation.id,
                    item = WaiterMapper.draftItem(operation.item),
                )
            is DraftOperation.Remove -> StoredOperation(kind = REMOVE, id = operation.id)
        }

    private fun operation(stored: StoredOperation): DraftOperation =
        when (stored.kind) {
            ADD -> DraftOperation.Add(WaiterMapper.draftLine(requireNotNull(stored.item)))
            QUANTITY ->
                DraftOperation.Quantity(
                    id = requireNotNull(stored.id),
                    delta = requireNotNull(stored.delta),
                    fallback = WaiterMapper.draftLine(requireNotNull(stored.fallback)),
                )
            REPLACE ->
                DraftOperation.Replace(
                    id = requireNotNull(stored.id),
                    item = WaiterMapper.draftLine(requireNotNull(stored.item)),
                )
            REMOVE -> DraftOperation.Remove(requireNotNull(stored.id))
            else -> error("Operación guardada desconocida: ${stored.kind}")
        }

    private const val ADD = "add"
    private const val QUANTITY = "quantity"
    private const val REPLACE = "replace"
    private const val REMOVE = "remove"
}
