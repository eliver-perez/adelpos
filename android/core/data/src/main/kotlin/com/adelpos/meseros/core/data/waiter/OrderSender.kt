package com.adelpos.meseros.core.data.waiter

import com.adelpos.meseros.core.data.access.AccessRepository
import com.adelpos.meseros.core.data.api.ApiErrors
import com.adelpos.meseros.core.data.api.ApiJson
import com.adelpos.meseros.core.data.api.ApiOutcome
import com.adelpos.meseros.core.data.api.ApiProblem
import com.adelpos.meseros.core.data.catalog.toDto
import com.adelpos.meseros.core.data.session.ServerTransport
import com.adelpos.meseros.core.data.waiter.db.WaiterDao
import com.adelpos.meseros.core.data.waiter.db.WaiterStorage
import com.adelpos.meseros.core.domain.error.AppError
import com.adelpos.meseros.core.domain.error.LicenseMessages
import com.adelpos.meseros.core.domain.waiter.OrderReview
import com.adelpos.meseros.core.domain.waiter.OrderSendRules
import com.adelpos.meseros.core.domain.waiter.OrderState
import com.adelpos.meseros.core.domain.waiter.PendingOrder
import com.adelpos.meseros.core.domain.waiter.ReviewReason
import com.adelpos.meseros.core.domain.waiter.SaveResolution
import com.adelpos.meseros.core.domain.waiter.SendOutcome
import java.time.Instant
import java.time.format.DateTimeFormatter
import java.util.concurrent.ConcurrentHashMap
import kotlin.coroutines.cancellation.CancellationException
import kotlinx.coroutines.NonCancellable
import kotlinx.coroutines.sync.Mutex
import kotlinx.coroutines.sync.withLock
import kotlinx.coroutines.withContext
import kotlinx.serialization.DeserializationStrategy
import kotlinx.serialization.SerializationException
import kotlinx.serialization.json.JsonObject
import okhttp3.Headers

/** Cómo terminó una vuelta del trabajador sobre la cola. */
data class SendRun(val sent: Int, val reviews: Int, val retryLater: Boolean)

/**
 * Pedidos que este proceso está confirmando en línea en este momento («Enviar a cocina» con
 * `confirmDraft`): siguen «Enviando» con su petición en curso y nadie más los toca. Cualquier otro
 * «Enviando» quedó así por un trabajador detenido o un proceso cerrado (revisión R6, B2).
 */
class OrdersInFlight {
    private val ids: MutableSet<String> = ConcurrentHashMap.newKeySet()

    fun add(operationId: String) {
        ids.add(operationId)
    }

    fun remove(operationId: String) {
        ids.remove(operationId)
    }

    operator fun contains(operationId: String): Boolean = operationId in ids
}

/**
 * Envía la cola sin conexión (§7) con `submitOfflineOrder` y la **credencial de envío del mesero
 * que capturó cada pedido** (A1-D1; revisión R3, M5): nunca con la de otro. Cada pedido sale con su
 * clave de operación de siempre, **la misma que usó el «Enviar a cocina» en línea** (A1-D19): si la
 * respuesta se pierde, el reintento devuelve lo mismo y Go nunca lo duplica
 * (`OFFLINE_ORDER_ALREADY_ACCEPTED`, también si la confirmación en línea ya había entrado).
 *
 * Un pedido para llevar nuevo sale con `destination.takeaway` (etapa A2) y las mismas reglas de
 * clave; si Go dice que sus partidas ya están en un pedido para llevar abierto
 * (`items_in_account`), se manda a ese pedido con la misma clave, como en una mesa.
 *
 * Antes de cada pedido (revisión R6): recupera los que quedaron «Enviando» sin petición en curso
 * (B2), repite con su clave el guardado del borrador que quedó sin respuesta si el mesero tiene
 * sesión (C1) y, si Go dice que las partidas ya están en una cuenta de la mesa
 * (`items_in_account`), lo manda a esa cuenta con la misma clave. Cada escritura en Room es
 * condicional al estado que se leyó (M1 y O1).
 *
 * Lo usa el trabajador de WorkManager (con la aplicación abierta o cerrada); una sola vuelta a la
 * vez en el proceso.
 */
class OrderSender(
    private val dao: WaiterDao,
    private val server: WaiterServer,
    private val api: WaiterApi,
    private val clock: () -> Long = System::currentTimeMillis,
    private val inFlight: OrdersInFlight = OrdersInFlight(),
    private val serverClock: ServerClock? = null,
) {
    private val running = Mutex()

    /**
     * Ejecuta [block] sin ninguna vuelta del trabajador al mismo tiempo: «Borrar los datos de este
     * dispositivo» espera aquí a que termine (o se cancele) el envío en curso (revisión R6, M1).
     */
    suspend fun <T> exclusive(block: suspend () -> T): T = running.withLock { block() }

    /**
     * Envía, del más viejo al más nuevo, los pedidos «Por enviar» que se pueden enviar. Sin
     * respuesta del servidor se detiene (los demás tampoco llegarían) y pide reintentar más tarde.
     */
    suspend fun sendPending(): SendRun = running.withLock {
        var sent = 0
        var reviews = 0
        var retryLater = false
        // Con la aplicación cerrada, WorkManager despierta el proceso sin pantallas: primero se lee
        // el servidor emparejado guardado.
        server.prepare()
        recoverStale()
        val queued = dao.ordersIn(OrderState.QUEUED.storedName).map(WaiterStorage::toOrder)
        for (order in queued) {
            if (order.credentialRejected) {
                // Espera a que el mesero vuelva a entrar con su PIN con conexión.
                continue
            }
            val transport = server.transport()
            if (transport == null) {
                retryLater = true
                break
            }
            val next = sendOne(order, transport) ?: continue
            when {
                next.state == OrderState.RECEIVED -> sent++
                next.state == OrderState.NEEDS_REVIEW -> reviews++
                next.state == OrderState.QUEUED && !next.credentialRejected -> {
                    // Sin respuesta, servidor ocupado o sin poder resolver lo anterior.
                    retryLater = true
                    break
                }
            }
        }
        SendRun(sent = sent, reviews = reviews, retryLater = retryLater)
    }

    /**
     * Un pedido, con las vueltas que haga falta para mandarlo a la cuenta donde ya están sus
     * partidas. Devuelve cómo quedó o null si otro lo cambió o lo borró mientras tanto.
     */
    private suspend fun sendOne(order: PendingOrder, transport: ServerTransport): PendingOrder? {
        var current =
            when (val step = resolveUnresolved(order, transport)) {
                is Resolution.Ready -> step.order
                Resolution.Gone -> return null
                // Sigue «Por enviar» sin tocar: la vuelta se detiene y se reintenta más tarde.
                Resolution.NoAnswer -> return order
            }
        repeat(MAXIMUM_REDIRECTS + 1) {
            val headers = server.offlineHeaders(current.waiterUserId)
            if (headers == null) {
                // Sin credencial de envío (Go la invalidó o el mesero nunca entró con
                // conexión): el pedido se conserva hasta que entre con su PIN con conexión.
                val sending = start(current) ?: return null
                return finish(sending, SendOutcome.CredentialRejected)
            }
            val sending = start(current) ?: return null
            val outcome =
                try {
                    submit(sending, transport, headers)
                } catch (cancelled: CancellationException) {
                    // Si el trabajador se cancela a media petición, el pedido no se queda
                    // «Enviando»: vuelve a la cola con la misma clave (salvo que ya no exista).
                    withContext(NonCancellable) {
                        write(OrderSendRules.recoverInterrupted(sending, clock()), sending.state)
                    }
                    throw cancelled
                }
            val next = finish(sending, outcome) ?: return null
            if (outcome !is SendOutcome.Redirect) {
                return next
            }
            current = next
        }
        return current
    }

    /**
     * Manda el pedido; si Go lo rechazó solo por la hora adelantada del dispositivo, lo corrige.
     */
    private suspend fun submit(
        order: PendingOrder,
        transport: ServerTransport,
        headers: Headers,
    ): SendOutcome {
        val usedOffset = offsetFor(order)
        val first = api.submitOfflineOrder(transport, headers, body(order, usedOffset))
        if (first is ApiOutcome.Rejected && rejectedForClock(order, usedOffset, first.problem)) {
            // La respuesta del rechazo trajo la hora de Go (B4): misma clave, hora corregida (Go
            // no guarda la clave de un rechazo).
            val second = api.submitOfflineOrder(transport, headers, body(order, offsetFor(order)))
            return interpret(order, second)
        }
        return interpret(order, first)
    }

    private fun offsetFor(order: PendingOrder): Long =
        if (order.clockCorrected) 0L else serverClock?.offsetMillis() ?: 0L

    /**
     * Go rechazó con 400 una hora de captura posterior a la suya: la del pedido, con la diferencia
     * que había al enviarlo, pasa la hora de Go por más de lo que Go tolera.
     */
    private fun rejectedForClock(
        order: PendingOrder,
        usedOffset: Long,
        problem: ApiProblem,
    ): Boolean {
        if (problem.status != BAD_REQUEST || order.clockCorrected) {
            return false
        }
        val clock = serverClock ?: return false
        val offset = clock.offsetMillis() ?: return false
        val sentAt = order.capturedAtMillis + usedOffset
        return sentAt > clock.deviceNow() + offset + CLOCK_SKEW_MILLIS
    }

    /**
     * Revisión R6, B2: un pedido «Enviando» que nadie está enviando (WorkManager detuvo al
     * trabajador entre dos escrituras, o el proceso se cerró) vuelve a «Por enviar» con su clave.
     * Corre con el candado de la vuelta, así que los envíos propios no están en curso; los que
     * confirma en línea la captura están en [inFlight].
     */
    private suspend fun recoverStale() {
        for (row in dao.ordersIn(OrderState.SENDING.storedName)) {
            if (row.operationId in inFlight) {
                continue
            }
            val order = WaiterStorage.toOrder(row)
            write(OrderSendRules.recoverInterrupted(order, clock()), OrderState.SENDING)
        }
    }

    /**
     * Revisión R6, C1: el guardado del borrador que quedó sin respuesta al tocar «Enviar a cocina»
     * se repite **igual** (misma clave y mismo contenido) con la sesión del mesero que capturó,
     * para saber si entró y, si abrió la cuenta, mandar el pedido a esa cuenta. Sin sesión se envía
     * así (Go reconoce los renglones por su identificador, A1-D18).
     */
    private suspend fun resolveUnresolved(
        order: PendingOrder,
        transport: ServerTransport,
    ): Resolution {
        val save = order.unresolvedSave ?: return Resolution.Ready(order)
        val headers = server.sessionHeaders(order.waiterUserId) ?: return Resolution.Ready(order)
        val items = save.items.map(WaiterMapper::draftItem)
        val savedAccount = save.accountId
        val outcome =
            if (savedAccount == null) {
                api.openAccount(
                    transport,
                    headers,
                    WaiterMapper.openBody(
                        operationId = save.operationId,
                        mode = save.mode,
                        tableId = save.tableId,
                        seenAccountIds = save.seenAccountIds,
                        customer = save.customer,
                        items = save.items,
                    ),
                )
            } else {
                api.saveDraft(
                    transport,
                    headers,
                    savedAccount,
                    SaveDraftBody(
                        operationId = save.operationId,
                        revision = save.revision,
                        items = items,
                    ),
                )
            }
        val resolution =
            when (outcome) {
                is ApiOutcome.Success -> {
                    val bill = WaiterMapper.bill(outcome.value)
                    SaveResolution.Applied(bill.account.id, bill.account.name, bill.draft)
                }
                is ApiOutcome.Rejected -> {
                    val error = ApiErrors.toAppError(outcome.problem)
                    if (isAccessProblem(error)) {
                        // No se sabe qué pasó: se envía así y Go decide por los identificadores.
                        server.report(error, order.waiterUserId)
                        return Resolution.Ready(order)
                    }
                    if (error is AppError.TryLater || error is AppError.ServerProblem) {
                        return Resolution.NoAnswer
                    }
                    // Un rechazo nunca guardó nada (Go no guarda la clave de un rechazo).
                    SaveResolution.NotApplied
                }
                is ApiOutcome.NoAnswer,
                is ApiOutcome.Malformed -> return Resolution.NoAnswer
            }
        val next = OrderSendRules.resolved(order, resolution, clock())
        return if (write(next, OrderState.QUEUED)) Resolution.Ready(next) else Resolution.Gone
    }

    /** Qué pasó al resolver el guardado sin respuesta de un pedido. */
    private sealed interface Resolution {
        /** Listo para enviarse (quizá con otro destino). */
        data class Ready(val order: PendingOrder) : Resolution

        /** Otro lo cambió o lo borró mientras tanto. */
        data object Gone : Resolution

        /** Go no respondió: se reintenta más tarde. */
        data object NoAnswer : Resolution
    }

    private suspend fun start(order: PendingOrder): PendingOrder? {
        val sending = OrderSendRules.startSending(order, clock())
        return if (write(sending, order.state)) sending else null
    }

    private suspend fun finish(sending: PendingOrder, outcome: SendOutcome): PendingOrder? {
        val next = OrderSendRules.after(sending, outcome, clock())
        if (!write(next, OrderState.SENDING)) {
            return null
        }
        val accountId = next.receivedAccountId
        if (next.state == OrderState.RECEIVED && next.destination.takeaway && accountId != null) {
            // El pedido para llevar ya está en Go: la edición del cliente que se hizo mientras
            // esperaba en la cola ya sabe a qué cuenta va (A2-D1; la envía la misma vuelta).
            dao.bindCustomerEdit(next.operationId, accountId)
        }
        return next
    }

    /** Escribe [order] solo si en Room sigue en [expected] (nunca revive un pedido borrado). */
    private suspend fun write(order: PendingOrder, expected: OrderState): Boolean =
        dao.transition(WaiterStorage.toEntity(order), expected.storedName)

    /** Lo que respondió Go, en términos de la cola. Los errores de acceso van al punto único. */
    private fun interpret(
        order: PendingOrder,
        outcome: ApiOutcome<OfflineOrderResultDto>,
    ): SendOutcome =
        when (outcome) {
            is ApiOutcome.Success -> SendOutcome.Accepted(outcome.value.accountId)
            is ApiOutcome.Rejected -> rejected(order, outcome.problem)
            is ApiOutcome.NoAnswer -> {
                if (outcome.error is AppError.ServerIdentityChanged) {
                    server.report(outcome.error, order.waiterUserId)
                }
                SendOutcome.TryAgain(NO_ANSWER)
            }
            is ApiOutcome.Malformed -> SendOutcome.TryAgain(MALFORMED)
        }

    private fun rejected(order: PendingOrder, problem: ApiProblem): SendOutcome {
        val message = problem.message ?: GENERIC_REJECTION
        // Etapa L1: la licencia, un cupo o la hora de la computadora principal (los cinco códigos
        // de L1-D17). Go no guardó nada: queda marcado con su motivo y su texto, y no se reintenta
        // solo (el mesero lo envía de nuevo con la misma clave cuando se resuelva).
        ApiErrors.licenseRefusal(problem)?.let { refusal ->
            return SendOutcome.NotAllowed(LicenseMessages.of(refusal))
        }
        return when (problem.code) {
            ORDER_REQUIRES_CONFIRMATION -> {
                val details =
                    decode(problem.details, OfflineRejectionDetailsDto.serializer())
                        ?: return SendOutcome.TryAgain(MALFORMED)
                val review =
                    OrderReview(
                        reason = ReviewReason.CHANGES,
                        message = message,
                        changes = details.changes.map(WaiterMapper::change),
                        catalogVersion = details.catalogVersion,
                    )
                // Las partidas ya están en una cuenta abierta de la mesa (A1-D18): se manda ahí
                // con la misma clave, sin preguntar. Con más de una cuenta, decide el mesero.
                val accounts = review.accountsWithItems
                if (accounts.size == 1) {
                    SendOutcome.Redirect(accounts.single())
                } else {
                    SendOutcome.RequiresConfirmation(review)
                }
            }
            ALREADY_ACCEPTED -> {
                // La misma clave ya entró (como pedido sin conexión o como el «Enviar a cocina» en
                // línea de esta captura, origin confirm_draft): «Recibido por el restaurante».
                val details = decode(problem.details, AlreadyAcceptedDetailsDto.serializer())
                SendOutcome.AlreadyAccepted(
                    details?.result?.accountId ?: order.destination.accountId,
                )
            }
            AccessRepository.OFFLINE_CREDENTIAL_INVALID -> {
                server.report(ApiErrors.toAppError(problem), order.waiterUserId)
                SendOutcome.CredentialRejected
            }
            else -> otherRejection(order, problem, message)
        }
    }

    private fun otherRejection(order: PendingOrder, problem: ApiProblem, message: String) =
        when (val error = ApiErrors.toAppError(problem)) {
            is AppError.DeviceRevoked,
            is AppError.DeviceNotRecognized,
            is AppError.SessionEnded,
            is AppError.UpdateRequired -> {
                // Dispositivo retirado, credencial del dispositivo o versión: lo decide la
                // entrada al dispositivo; el pedido se conserva (la bitácora lo muestra si se
                // retira).
                server.report(error, order.waiterUserId)
                SendOutcome.TryAgain(message)
            }
            is AppError.TryLater,
            is AppError.ServerProblem -> SendOutcome.TryAgain(message)
            is AppError.PermissionDenied -> SendOutcome.Rejected(message)
            // No llega aquí ([rejected] la atiende antes); si llegara, nunca se reintenta solo.
            is AppError.LicenseRefused -> SendOutcome.NotAllowed(LicenseMessages.of(error.refusal))
            is AppError.Rejected ->
                if (error.code == CLIENT_KIND_MISMATCH) {
                    SendOutcome.TryAgain(message)
                } else {
                    SendOutcome.Rejected(message)
                }
            else -> SendOutcome.TryAgain(message)
        }

    private fun isAccessProblem(error: AppError): Boolean =
        error is AppError.SessionEnded ||
            error is AppError.DeviceRevoked ||
            error is AppError.DeviceNotRecognized ||
            error is AppError.UpdateRequired

    private fun <T> decode(
        details: JsonObject?,
        deserializer: DeserializationStrategy<T>,
    ): T? {
        val value = details ?: return null
        return try {
            ApiJson.decodeFromJsonElement(deserializer, value)
        } catch (_: SerializationException) {
            null
        } catch (_: IllegalArgumentException) {
            null
        }
    }

    companion object {
        const val ORDER_REQUIRES_CONFIRMATION = "ORDER_REQUIRES_CONFIRMATION"
        const val ALREADY_ACCEPTED = "OFFLINE_ORDER_ALREADY_ACCEPTED"
        const val CLIENT_KIND_MISMATCH = "CLIENT_KIND_MISMATCH"

        /** Vueltas para mandar el pedido a la cuenta donde ya están sus partidas. */
        private const val MAXIMUM_REDIRECTS = 2

        /** Lo que Go tolera que el reloj del dispositivo vaya adelantado (`offlineClockSkew`). */
        private const val CLOCK_SKEW_MILLIS = 10L * 60 * 1000

        private const val BAD_REQUEST = 400
        private const val NO_ANSWER = "Sin respuesta de la computadora principal."
        private const val MALFORMED = "La respuesta de la computadora principal no se entendió."
        private const val GENERIC_REJECTION = "La computadora principal no aceptó el pedido."

        /**
         * El cuerpo de `submitOfflineOrder` de un pedido de la cola; [clockOffsetMillis] corrige la
         * hora de captura con la diferencia con la hora de Go (B4).
         */
        fun body(order: PendingOrder, clockOffsetMillis: Long = 0L): OfflineOrderBody {
            val destination = order.destination
            val accountId = destination.accountId
            return OfflineOrderBody(
                operationId = order.operationId,
                catalogVersion = order.catalogVersion,
                capturedAt =
                    DateTimeFormatter.ISO_INSTANT.format(
                        Instant.ofEpochMilli(order.capturedAtMillis + clockOffsetMillis),
                    ),
                destination =
                    when {
                        accountId != null -> OfflineDestinationDto(accountId = accountId)
                        // Un pedido para llevar nuevo (etapa A2): nace en Go al recibirse, con el
                        // nombre y el teléfono con que salió.
                        destination.takeaway ->
                            OfflineDestinationDto(
                                takeaway =
                                    OfflineTakeawayDto(
                                        customerName = destination.customer.name,
                                        phone = destination.customer.phone,
                                    ),
                            )
                        else ->
                            OfflineDestinationDto(
                                tableId = destination.tableId,
                                seenAccountIds = destination.seenAccountIds,
                            )
                    },
                items =
                    order.lines.map { line ->
                        OfflineItemDto(
                            id = line.item.id,
                            productId = line.item.line.productId,
                            variantId = line.item.line.variantId,
                            quantity = line.item.line.quantity,
                            customization = line.item.line.customization.toDto(),
                            seenUnitPrice = line.seenUnitPrice.decimal,
                        )
                    },
                confirmation =
                    order.confirmation?.let { confirmation ->
                        OfflineConfirmationDto(
                            catalogVersion = confirmation.catalogVersion,
                            changes = confirmation.changes.map(WaiterMapper::changeDto),
                        )
                    },
                // Solo con una cuenta (Go lo rechaza con una mesa) y solo si hay renglones.
                replacesDraftItems =
                    if (accountId != null && order.replaces.isNotEmpty()) {
                        order.replaces.map(WaiterMapper::draftItem)
                    } else {
                        null
                    },
            )
        }
    }
}
