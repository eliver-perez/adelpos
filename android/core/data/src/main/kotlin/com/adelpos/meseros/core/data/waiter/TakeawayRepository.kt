package com.adelpos.meseros.core.data.waiter

import com.adelpos.core.model.error.AppError
import com.adelpos.core.network.api.ApiErrors
import com.adelpos.core.network.api.ApiOutcome
import com.adelpos.core.network.api.ApiProblem
import com.adelpos.meseros.core.data.waiter.db.CustomerEditEntity
import com.adelpos.meseros.core.data.waiter.db.WaiterDao
import com.adelpos.meseros.core.data.waiter.db.WaiterStorage
import com.adelpos.meseros.core.domain.waiter.AccountSummary
import com.adelpos.meseros.core.domain.waiter.CustomerEdit
import com.adelpos.meseros.core.domain.waiter.CustomerTarget
import com.adelpos.meseros.core.domain.waiter.DeliveryResult
import com.adelpos.meseros.core.domain.waiter.OrderState
import com.adelpos.meseros.core.domain.waiter.TakeawayCustomer
import com.adelpos.meseros.core.domain.waiter.TakeawayRead
import com.adelpos.meseros.core.domain.waiter.TakeawayService
import java.util.UUID
import kotlinx.coroutines.delay
import kotlinx.coroutines.flow.Flow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.map
import kotlinx.coroutines.sync.Mutex
import kotlinx.coroutines.sync.withLock

/**
 * Para llevar del mesero activo (§3.3, A2-D1 y A2-D2): los pedidos para llevar que leyó
 * [restaurant] (con conexión y al momento; sin ella, lo último guardado con su hora), las ediciones
 * del nombre y el teléfono por enviar (Room, `takeaway_customer_edits`) y «Entregado».
 *
 * «Entregado» **nunca** va a la cola: exige conexión y la sesión del mesero. La regla «no se
 * entrega un pedido sin pagar» la impone Go (`BALANCE_PENDING`).
 */
class TakeawayRepository(
    private val dao: WaiterDao,
    private val server: WaiterServer,
    private val api: WaiterApi,
    private val restaurant: RestaurantRepository,
    private val scheduler: OrderSendScheduler,
    private val clock: () -> Long = System::currentTimeMillis,
    private val newId: () -> String = { UUID.randomUUID().toString() },
    private val retryDelayMillis: Long = RETRY_DELAY_MILLIS,
) : TakeawayService {
    private val editing = Mutex()

    override val accounts: StateFlow<List<AccountSummary>> = restaurant.allOpenAccounts

    override val readAt: StateFlow<TakeawayRead?> = restaurant.accountsReadAt

    override fun refresh() {
        restaurant.refresh()
    }

    override fun customerEdits(): Flow<List<CustomerEdit>> =
        dao.observeCustomerEdits().map { rows -> rows.map(WaiterStorage::toEdit) }

    override suspend fun editCustomer(
        userId: String,
        target: CustomerTarget,
        customer: TakeawayCustomer,
    ) {
        val wanted = customer.trimmed()
        editing.withLock {
            val (key, accountId) =
                when (target) {
                    is CustomerTarget.Account -> target.accountId to target.accountId
                    is CustomerTarget.QueuedOrder -> {
                        val order =
                            dao.order(target.operationId)?.let(WaiterStorage::toOrder) ?: return
                        // Si ya entró, la cuenta se conoce; si va a un pedido que ya existía, esa.
                        target.operationId to
                            (order.receivedAccountId.takeIf { order.state == OrderState.RECEIVED }
                                ?: order.destination.accountId)
                    }
                }
            val previous = dao.customerEditFor(key, accountId)
            if (
                previous != null &&
                    previous.customerName == wanted.name &&
                    previous.phone == wanted.phone
            ) {
                return
            }
            // La última edición gana: reemplaza a la anterior con una clave de operación nueva (si
            // la anterior salió sin respuesta, su respuesta ya no borra esta).
            dao.replaceCustomerEdit(
                CustomerEditEntity(
                    key = previous?.key ?: key,
                    accountId = accountId ?: previous?.accountId,
                    waiterUserId = userId,
                    customerName = wanted.name,
                    phone = wanted.phone,
                    operationId = newId(),
                    updatedAt = clock(),
                ),
            )
        }
        scheduler.requestSend()
    }

    override suspend fun deliver(userId: String, accountId: String): DeliveryResult {
        val transport = server.transport() ?: return DeliveryResult.NoConnection
        val headers = server.sessionHeaders(userId) ?: return DeliveryResult.NoConnection
        val body = DeliverBody(operationId = newId())
        var outcome = api.deliverTakeaway(transport, headers, accountId, body)
        if (outcome is ApiOutcome.NoAnswer && outcome.requestSent) {
            // Pudo haber entrado: se repite **igual** (misma clave) y Go devuelve lo mismo.
            delay(retryDelayMillis)
            outcome = api.deliverTakeaway(transport, headers, accountId, body)
        }
        val result =
            when (outcome) {
                is ApiOutcome.Success -> DeliveryResult.Delivered(outcome.value.closed)
                is ApiOutcome.Rejected -> rejection(userId, outcome.problem)
                is ApiOutcome.NoAnswer -> DeliveryResult.NoConnection
                is ApiOutcome.Malformed -> DeliveryResult.Failed(null)
            }
        // La lista se vuelve a leer en cualquier caso (el canal también avisa).
        restaurant.refresh()
        return result
    }

    private fun rejection(userId: String, problem: ApiProblem): DeliveryResult =
        when (problem.code) {
            BALANCE_PENDING -> DeliveryResult.CollectFirst
            ALREADY_DELIVERED -> DeliveryResult.AlreadyDelivered
            else -> {
                val error = ApiErrors.toAppError(problem)
                if (isAccessProblem(error)) {
                    server.report(error, userId)
                }
                DeliveryResult.Failed(ApiErrors.shownMessage(problem))
            }
        }

    companion object {
        const val BALANCE_PENDING = "BALANCE_PENDING"
        const val ALREADY_DELIVERED = "ALREADY_DELIVERED"
        private const val RETRY_DELAY_MILLIS = 1_500L
    }
}

/**
 * Envía las ediciones del nombre y el teléfono por enviar (A2-D1) con `updateTakeawayCustomer` y la
 * **sesión del mesero que hizo la edición** (la credencial de envío solo sirve para
 * `submitOfflineOrder`, revisión R3): si ese mesero no tiene sesión, la edición espera a que entre
 * con conexión (nunca se pierde). Lo usa el mismo trabajador de WorkManager que la cola (sin
 * restricción de red), después de los pedidos.
 *
 * Una edición de un pedido que todavía no nace en Go espera a que la cola o la captura sepan su
 * cuenta. Cada edición lleva su clave; se borra solo si Go confirmó **esa** clave (la última gana)
 * o la rechazó para siempre (el pedido ya no está abierto, datos o permiso).
 */
class CustomerEditSender(
    private val dao: WaiterDao,
    private val server: WaiterServer,
    private val api: WaiterApi,
    private val newId: () -> String = { UUID.randomUUID().toString() },
) {
    private val running = Mutex()

    /**
     * Envía lo que se pueda; true si conviene reintentar más tarde (sin respuesta o servidor
     * ocupado).
     */
    suspend fun sendPending(): Boolean = running.withLock {
        server.prepare()
        for (row in dao.customerEdits()) {
            val edit = bound(row) ?: continue
            val accountId = edit.accountId ?: continue
            val transport = server.transport() ?: return@withLock true
            // Sin sesión de ese mesero (entró sin conexión o se cerró la aplicación): espera.
            val headers = server.sessionHeaders(edit.waiterUserId) ?: continue
            val outcome =
                api.updateTakeawayCustomer(
                    transport,
                    headers,
                    accountId,
                    TakeawayCustomerBody(
                        operationId = edit.operationId,
                        customerName = edit.customerName,
                        phone = edit.phone,
                    ),
                )
            when (outcome) {
                is ApiOutcome.Success -> dao.finishCustomerEdit(edit.key, edit.operationId)
                is ApiOutcome.Rejected -> {
                    val problem = outcome.problem
                    val error = ApiErrors.toAppError(problem)
                    when {
                        isAccessProblem(error) -> server.report(error, edit.waiterUserId)
                        error is AppError.TryLater || error is AppError.ServerProblem ->
                            return@withLock true
                        // Etapa L1: la hora de la computadora principal (o la licencia) no lo
                        // permitió y Go no guardó nada. No es definitivo: la edición se conserva y
                        // sale en una vuelta siguiente de la cola (al volver la red o el mesero, al
                        // abrir la aplicación o la de cada 15 minutos), sin pedir reintentos
                        // seguidos.
                        error is AppError.LicenseRefused -> Unit
                        problem.code == IDEMPOTENCY_CONFLICT ->
                            // La clave ya se usó con otro contenido (no debería pasar: cada edición
                            // lleva una nueva): la misma edición con otra clave.
                            dao.replaceCustomerEdit(edit.copy(operationId = newId()))
                        // Rechazo definitivo: el pedido ya no está abierto, no es para llevar, el
                        // dato no cabe o el mesero ya no tiene el permiso. No hay a qué aplicarla.
                        else -> dao.finishCustomerEdit(edit.key, edit.operationId)
                    }
                }
                is ApiOutcome.NoAnswer,
                is ApiOutcome.Malformed -> return@withLock true
            }
        }
        false
    }

    /**
     * La edición con su cuenta de Go, si ya se sabe: la de la captura (nació con el primer
     * producto) o la del pedido de la cola recibido. Si el pedido ya no está en el dispositivo (se
     * descartó), no hay a qué aplicarla y se borra.
     */
    private suspend fun bound(row: CustomerEditEntity): CustomerEditEntity? {
        if (row.accountId != null) {
            return row
        }
        val order = dao.order(row.key)?.let(WaiterStorage::toOrder)
        val capture = dao.capture(row.key)?.let(WaiterStorage::toCapture)
        val accountId =
            when {
                order != null ->
                    order.receivedAccountId.takeIf { order.state == OrderState.RECEIVED }
                        ?: return null
                capture != null -> capture.accountId ?: return null
                else -> {
                    dao.finishCustomerEdit(row.key, row.operationId)
                    return null
                }
            }
        dao.bindCustomerEdit(row.key, accountId)
        return row.copy(accountId = accountId)
    }

    companion object {
        private const val IDEMPOTENCY_CONFLICT = "IDEMPOTENCY_CONFLICT"
    }
}

/** Errores de acceso que decide la entrada al dispositivo (punto único `report`). */
internal fun isAccessProblem(error: AppError): Boolean =
    error is AppError.SessionEnded ||
        error is AppError.DeviceRevoked ||
        error is AppError.DeviceNotRecognized ||
        error is AppError.UpdateRequired
