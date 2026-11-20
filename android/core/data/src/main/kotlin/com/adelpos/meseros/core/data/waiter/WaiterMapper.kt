package com.adelpos.meseros.core.data.waiter

import com.adelpos.meseros.core.data.catalog.LineRequestDto
import com.adelpos.meseros.core.data.catalog.toDomain
import com.adelpos.meseros.core.data.catalog.toDto
import com.adelpos.meseros.core.domain.catalog.LineSelection
import com.adelpos.meseros.core.domain.table.TableState
import com.adelpos.meseros.core.domain.waiter.AccountBill
import com.adelpos.meseros.core.domain.waiter.AccountSummary
import com.adelpos.meseros.core.domain.waiter.ChangeKind
import com.adelpos.meseros.core.domain.waiter.DiningArea
import com.adelpos.meseros.core.domain.waiter.DiningTable
import com.adelpos.meseros.core.domain.waiter.DraftLine
import com.adelpos.meseros.core.domain.waiter.DraftQuote
import com.adelpos.meseros.core.domain.waiter.OrderChange
import com.adelpos.meseros.core.domain.waiter.OrderMode
import com.adelpos.meseros.core.domain.waiter.PreparationState
import com.adelpos.meseros.core.domain.waiter.QuotedLine
import com.adelpos.meseros.core.domain.waiter.RestaurantSettings
import com.adelpos.meseros.core.domain.waiter.SentLine
import com.adelpos.meseros.core.domain.waiter.TableShape
import com.adelpos.meseros.core.domain.waiter.Takeaway
import com.adelpos.meseros.core.domain.waiter.TakeawayCustomer
import com.adelpos.meseros.core.domain.waiter.Totals

/** Correspondencia entre las formas del contrato del modo Mesero y el dominio. */
object WaiterMapper {
    fun area(dto: AreaDto): DiningArea =
        DiningArea(id = dto.id, name = dto.name, sortOrder = dto.sortOrder, active = dto.active)

    /** Una mesa con un estado que esta versión no conoce no se muestra (nunca se inventa uno). */
    fun table(dto: TableDto): DiningTable? {
        val state = TableState.fromWireName(dto.state) ?: return null
        return DiningTable(
            id = dto.id,
            areaId = dto.areaId,
            name = dto.name,
            shape = TableShape.fromWireName(dto.shape),
            x = dto.x,
            y = dto.y,
            width = dto.width,
            height = dto.height,
            seats = dto.seats,
            active = dto.active,
            outOfService = dto.outOfService,
            accountCount = dto.accountCount,
            state = state,
            occupiedSeconds = dto.occupiedSeconds,
            balance = dto.balance,
            longOpen = dto.longOpen,
        )
    }

    fun settings(dto: RestaurantSettingsDto): RestaurantSettings =
        RestaurantSettings(
            warningMinutes = dto.warningMinutes,
            overdueMinutes = dto.overdueMinutes,
            longOpenHours = dto.longOpenHours,
        )

    fun totals(dto: TotalsDto?): Totals =
        if (dto == null) {
            Totals.ZERO
        } else {
            Totals(subtotal = dto.subtotal, discount = dto.discount, total = dto.total)
        }

    fun account(dto: AccountDto): AccountSummary =
        AccountSummary(
            id = dto.id,
            folio = dto.folio,
            name = dto.name,
            tableId = dto.tableId,
            tableName = dto.tableName,
            state = dto.state,
            revision = dto.revision,
            preparationState = PreparationState.fromWireName(dto.preparationState),
            confirmedQuantity = dto.confirmedQuantity,
            draftQuantity = dto.draftQuantity,
            totals = totals(dto.totals),
            assignedName = dto.assignedName,
            elapsedSeconds = dto.elapsedSeconds,
            longOpen = dto.longOpen,
            mergedIntoId = dto.mergedIntoId,
            mode = OrderMode.fromWireName(dto.mode),
            customer = TakeawayCustomer(dto.customerName, dto.phone),
            paymentState = dto.paymentState,
            deliveryState = dto.deliveryState,
            discardable = dto.discardable,
        )

    fun draftLine(dto: DraftItemDto): DraftLine =
        DraftLine(
            id = dto.id,
            line =
                LineSelection(
                    productId = dto.productId,
                    variantId = dto.variantId,
                    quantity = dto.quantity,
                    customization = dto.customization.toDomain(),
                ),
        )

    fun draftItem(line: DraftLine): DraftItemDto =
        DraftItemDto(
            id = line.id,
            productId = line.line.productId,
            variantId = line.line.variantId,
            quantity = line.line.quantity,
            customization = line.line.customization.toDto(),
        )

    fun lineRequest(dto: LineRequestDto): LineSelection =
        LineSelection(
            productId = dto.productId,
            variantId = dto.variantId,
            quantity = dto.quantity,
            customization = dto.customization.toDomain(),
        )

    fun sentLine(dto: ConfirmedLineDto): SentLine =
        SentLine(
            versionId = dto.versionId,
            itemId = dto.itemId,
            state = dto.state,
            quantity = dto.quantity,
            preparationState = PreparationState.fromWireName(dto.preparationState),
            request = lineRequest(dto.request),
            name = dto.quoteLine.name,
            unitPrice = dto.quoteLine.unitPrice,
            total = dto.totals.total,
        )

    fun bill(dto: BillDto): AccountBill =
        AccountBill(
            account = account(dto.account),
            draft = dto.draft.map(::draftLine),
            lines = dto.lines.map(::sentLine),
        )

    fun quote(dto: QuoteResultDto): DraftQuote =
        DraftQuote(
            accountId = dto.accountId,
            revision = dto.revision,
            fingerprint = dto.fingerprint,
            lines =
                dto.quote?.lines.orEmpty().map { line ->
                    QuotedLine(
                        productId = line.productId,
                        name = line.name,
                        unitPrice = line.unitPrice,
                        quantity = line.quantity,
                        total = line.total,
                    )
                },
            draftTotal = dto.quote?.total,
            combinedTotal = dto.combined.total,
        )

    fun change(dto: OfflineChangeDto): OrderChange =
        OrderChange(
            kind = ChangeKind.fromWireName(dto.kind),
            wireKind = dto.kind,
            itemId = dto.itemId,
            productId = dto.productId,
            productName = dto.productName,
            before = dto.before,
            now = dto.now,
            message = dto.message,
            acceptable = dto.acceptable,
            accountId = dto.accountId,
        )

    /**
     * El cuerpo de `openAccount` (creación diferida con el primer producto) de una captura o de un
     * guardado sin respuesta que se repite **igual**: en una mesa, con `table_id` y las cuentas que
     * se veían en ella; en un pedido para llevar (etapa A2), sin mesa, con el origen «Para llevar»
     * y el nombre y el teléfono con que sale.
     */
    fun openBody(
        operationId: String,
        mode: OrderMode,
        tableId: String?,
        seenAccountIds: List<String>,
        customer: TakeawayCustomer,
        items: List<DraftLine>,
    ): OpenAccountBody =
        when (mode) {
            OrderMode.DINE_IN ->
                OpenAccountBody(
                    operationId = operationId,
                    mode = mode.wireName,
                    tableId = requireNotNull(tableId) { "Una cuenta de salón nace en una mesa" },
                    seenAccountIds = seenAccountIds,
                    items = items.map(::draftItem),
                )
            OrderMode.TAKEAWAY -> {
                val trimmed = customer.trimmed()
                OpenAccountBody(
                    operationId = operationId,
                    mode = mode.wireName,
                    tableId = null,
                    items = items.map(::draftItem),
                    origin = Takeaway.ORIGIN,
                    customerName = trimmed.name,
                    phone = trimmed.phone,
                )
            }
        }

    /** El cambio tal como lo dio Go, para devolverlo en la confirmación. */
    fun changeDto(change: OrderChange): OfflineChangeDto =
        OfflineChangeDto(
            kind = change.wireKind,
            itemId = change.itemId,
            productId = change.productId,
            productName = change.productName,
            before = change.before,
            now = change.now,
            message = change.message,
            acceptable = change.acceptable,
            accountId = change.accountId,
        )
}
