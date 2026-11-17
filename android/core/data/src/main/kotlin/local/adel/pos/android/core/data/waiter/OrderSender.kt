package local.adel.pos.android.core.data.waiter

import java.time.Instant
import java.time.format.DateTimeFormatter
import kotlin.coroutines.cancellation.CancellationException
import kotlinx.coroutines.NonCancellable
import kotlinx.coroutines.sync.Mutex
import kotlinx.coroutines.sync.withLock
import kotlinx.coroutines.withContext
import kotlinx.serialization.DeserializationStrategy
import kotlinx.serialization.SerializationException
import kotlinx.serialization.json.JsonObject
import local.adel.pos.android.core.data.access.AccessRepository
import local.adel.pos.android.core.data.api.ApiErrors
import local.adel.pos.android.core.data.api.ApiJson
import local.adel.pos.android.core.data.api.ApiOutcome
import local.adel.pos.android.core.data.api.ApiProblem
import local.adel.pos.android.core.data.catalog.toDto
import local.adel.pos.android.core.data.waiter.db.WaiterDao
import local.adel.pos.android.core.data.waiter.db.WaiterStorage
import local.adel.pos.android.core.domain.error.AppError
import local.adel.pos.android.core.domain.waiter.OrderReview
import local.adel.pos.android.core.domain.waiter.OrderSendRules
import local.adel.pos.android.core.domain.waiter.OrderState
import local.adel.pos.android.core.domain.waiter.PendingOrder
import local.adel.pos.android.core.domain.waiter.ReviewReason
import local.adel.pos.android.core.domain.waiter.SendOutcome

/** Cómo terminó una vuelta del trabajador sobre la cola. */
data class SendRun(val sent: Int, val reviews: Int, val retryLater: Boolean)

/**
 * Envía la cola sin conexión (§7) con `submitOfflineOrder` y la **credencial de envío del mesero
 * que capturó cada pedido** (A1-D1; revisión R3, M5): nunca con la de otro. Cada pedido sale con su
 * clave de operación de siempre: si la respuesta se pierde, el reintento devuelve lo mismo y Go
 * nunca lo duplica (`OFFLINE_ORDER_ALREADY_ACCEPTED` si llegó otro contenido con la misma clave).
 *
 * Lo usa el trabajador de WorkManager (con la aplicación abierta o cerrada); una sola vuelta a la
 * vez en el proceso.
 */
class OrderSender(
    private val dao: WaiterDao,
    private val server: WaiterServer,
    private val api: WaiterApi,
    private val clock: () -> Long = System::currentTimeMillis,
) {
    private val running = Mutex()

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
            val headers = server.offlineHeaders(order.waiterUserId)
            if (headers == null) {
                // Sin credencial de envío (Go la invalidó o el mesero nunca entró con
                // conexión): el pedido se conserva hasta que entre con su PIN con conexión.
                save(OrderSendRules.after(start(order), SendOutcome.CredentialRejected, clock()))
                continue
            }
            val sending = start(order)
            val outcome =
                try {
                    interpret(sending, api.submitOfflineOrder(transport, headers, body(sending)))
                } catch (cancelled: CancellationException) {
                    // Si el trabajador se cancela a media petición, el pedido no se queda
                    // «Enviando»: vuelve a la cola con la misma clave.
                    withContext(NonCancellable) {
                        save(OrderSendRules.recoverInterrupted(sending, clock()))
                    }
                    throw cancelled
                }
            val next = OrderSendRules.after(sending, outcome, clock())
            save(next)
            when {
                next.state == OrderState.RECEIVED -> sent++
                next.state == OrderState.NEEDS_REVIEW -> reviews++
                outcome is SendOutcome.TryAgain -> {
                    retryLater = true
                    break
                }
            }
        }
        SendRun(sent = sent, reviews = reviews, retryLater = retryLater)
    }

    private suspend fun start(order: PendingOrder): PendingOrder {
        val sending = OrderSendRules.startSending(order, clock())
        save(sending)
        return sending
    }

    private suspend fun save(order: PendingOrder) {
        dao.saveOrder(WaiterStorage.toEntity(order))
    }

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
        return when (problem.code) {
            ORDER_REQUIRES_CONFIRMATION -> {
                val details =
                    decode(problem.details, OfflineRejectionDetailsDto.serializer())
                        ?: return SendOutcome.TryAgain(MALFORMED)
                SendOutcome.RequiresConfirmation(
                    OrderReview(
                        reason = ReviewReason.CHANGES,
                        message = message,
                        changes = details.changes.map(WaiterMapper::change),
                        catalogVersion = details.catalogVersion,
                    ),
                )
            }
            ALREADY_ACCEPTED -> {
                val details = decode(problem.details, AlreadyAcceptedDetailsDto.serializer())
                SendOutcome.AlreadyAccepted(
                    details?.result?.accountId ?: order.destination.accountId.orEmpty(),
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
            is AppError.Rejected ->
                if (error.code == CLIENT_KIND_MISMATCH) {
                    SendOutcome.TryAgain(message)
                } else {
                    SendOutcome.Rejected(message)
                }
            else -> SendOutcome.TryAgain(message)
        }

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

        private const val NO_ANSWER = "Sin respuesta de la computadora principal."
        private const val MALFORMED = "La respuesta de la computadora principal no se entendió."
        private const val GENERIC_REJECTION = "La computadora principal no aceptó el pedido."

        /** El cuerpo de `submitOfflineOrder` de un pedido de la cola. */
        fun body(order: PendingOrder): OfflineOrderBody {
            val destination = order.destination
            val accountId = destination.accountId
            return OfflineOrderBody(
                operationId = order.operationId,
                catalogVersion = order.catalogVersion,
                capturedAt =
                    DateTimeFormatter.ISO_INSTANT.format(
                        Instant.ofEpochMilli(order.capturedAtMillis),
                    ),
                destination =
                    if (accountId != null) {
                        OfflineDestinationDto(accountId = accountId)
                    } else {
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
