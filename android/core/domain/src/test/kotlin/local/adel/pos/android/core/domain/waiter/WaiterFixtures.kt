package local.adel.pos.android.core.domain.waiter

import local.adel.pos.android.core.domain.catalog.Customization
import local.adel.pos.android.core.domain.catalog.LineSelection
import local.adel.pos.android.core.domain.money.Money
import local.adel.pos.android.core.domain.table.TableState

/** Datos de ejemplo de las pruebas del modo Mesero. */
object WaiterFixtures {
    const val ANA = "11111111-1111-4111-8111-111111111111"
    const val TABLE = "22222222-2222-4222-8222-222222222222"
    const val ACCOUNT = "33333333-3333-4333-8333-333333333333"
    const val BURGER = "44444444-4444-4444-8444-444444444444"
    const val SHAKE = "55555555-5555-4555-8555-555555555555"
    const val TACOS = "66666666-6666-4666-8666-666666666666"

    fun line(productId: String, quantity: Int = 1, notes: String = ""): LineSelection =
        LineSelection(
            productId = productId,
            variantId = null,
            quantity = quantity,
            customization = Customization(notes = notes),
        )

    fun draft(id: String, productId: String, quantity: Int = 1, notes: String = "") =
        DraftLine(id, line(productId, quantity, notes))

    fun capture(
        base: List<DraftLine> = emptyList(),
        operations: List<DraftOperation> = emptyList(),
        own: Set<String> = emptySet(),
        accountId: String? = ACCOUNT,
        prices: Map<String, Money> = emptyMap(),
    ) =
        Capture(
            id = "capture-1",
            waiterUserId = ANA,
            tableId = TABLE,
            tableName = "Mesa 4",
            accountId = accountId,
            accountName = accountId?.let { "Mesa 4" },
            seenAccountIds = emptyList(),
            base = base,
            baseRevision = 3,
            operations = operations,
            ownItemIds = own,
            seenPrices = prices,
            sync = DraftSync.SAVED,
            syncMessage = null,
            tableAccounts = emptyList(),
            pendingOperationId = null,
            inFlightCount = 0,
            createdAtMillis = 1_000L,
        )

    val lookup = OrderFactory.CatalogLookup { line ->
        when (line.productId) {
            BURGER -> OrderFactory.Described("Hamburguesa", Money("120.00"))
            SHAKE -> OrderFactory.Described("Malteada de fresa", Money("65.00"))
            TACOS -> OrderFactory.Described("Tacos al pastor", Money("90.00"))
            else -> null
        }
    }

    fun order(
        capture: Capture,
        state: OrderState = OrderState.QUEUED,
        operationId: String = "op-1",
    ): PendingOrder {
        val created =
            requireNotNull(
                OrderFactory.fromCapture(
                    capture = capture,
                    waiterName = "Ana",
                    operationId = operationId,
                    catalog = OrderFactory.CatalogStamp(41, "epoca"),
                    lookup = lookup,
                    onlineConfirm = null,
                    nowMillis = 2_000L,
                ),
            )
        return created.copy(state = state)
    }

    fun table(
        id: String,
        name: String,
        areaId: String = "area-1",
        active: Boolean = true,
        accounts: Int = 0,
    ) =
        DiningTable(
            id = id,
            areaId = areaId,
            name = name,
            shape = TableShape.RECTANGLE,
            x = 0,
            y = 0,
            width = 100,
            height = 80,
            seats = 4,
            active = active,
            outOfService = false,
            accountCount = accounts,
            state = TableState.FREE,
            occupiedSeconds = 0,
            balance = Money("0.00"),
            longOpen = false,
        )

    fun change(
        kind: ChangeKind,
        itemId: String?,
        acceptable: Boolean,
        message: String = "cambio",
        now: String? = null,
    ) =
        OrderChange(
            kind = kind,
            wireKind = kind.wireName,
            itemId = itemId,
            productId = null,
            productName = "",
            before = null,
            now = now,
            message = message,
            acceptable = acceptable,
        )
}
