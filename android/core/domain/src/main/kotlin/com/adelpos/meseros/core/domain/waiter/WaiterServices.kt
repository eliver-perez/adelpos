package com.adelpos.meseros.core.domain.waiter

import com.adelpos.core.model.error.AppError
import com.adelpos.core.model.error.AppResult
import com.adelpos.meseros.core.domain.catalog.LineSelection
import com.adelpos.meseros.core.domain.money.Money
import kotlinx.coroutines.flow.Flow
import kotlinx.coroutines.flow.StateFlow

/*
 * Interfaces del modo Mesero (Fase E) para las pantallas de `feature:waiter`, la captura
 * compartida de `core:capture` y, desde la A2, Para llevar (`feature:takeaway`). Las implementa
 * `core:data` sobre la API v1, Room y WorkManager; las pantallas solo conocen estas.
 */

/** Cómo va la lectura de mesas y cuentas desde Go. */
data class RestaurantStatus(
    val loading: Boolean,
    /** La última lectura vino de Go (con la sesión del mesero) y no de lo guardado. */
    val live: Boolean,
    val lastError: AppError?,
)

/**
 * Mesas, cuentas abiertas y permisos para el mesero activo (§6.1 y §6.2). Con conexión se leen de
 * Go (`getRestaurantOverview` y `getRecoveryState`) y se actualizan al momento con el canal de
 * tiempo real; sin conexión se ve lo último guardado en el dispositivo.
 */
interface RestaurantSource {
    val view: StateFlow<RestaurantView>

    /** Permisos vigentes del mesero activo (desconocidos si nunca se leyeron). */
    val permissions: StateFlow<WaiterPermissions>

    val status: StateFlow<RestaurantStatus>

    /** Cuentas abiertas según la última lectura, con su revisión. */
    val openAccounts: StateFlow<List<AccountSummary>>

    /** Vuelve a leer mesas, cuentas y permisos (al abrir la pantalla o con «Actualizar»). */
    fun refresh()

    /** Cuentas cuyo contenido cambió en Go (otra terminal, cocina): volver a leerlas. */
    val accountChanges: Flow<String>

    /** La cuenta completa desde Go, con la sesión del mesero activo. */
    suspend fun bill(accountId: String): AppResult<AccountBill>

    /**
     * «Descartar cuenta vacía» (etapa A3, §4.10) de [account], que Go marcó `discardable`: con
     * conexión y la sesión del mesero activo (nunca en la cola), con su revisión. Después se vuelve
     * a leer.
     */
    suspend fun discardEmptyAccount(account: AccountSummary): DiscardAccountResult
}

/** Cómo quedó «Descartar cuenta vacía». */
sealed interface DiscardAccountResult {
    data object Discarded : DiscardAccountResult

    /**
     * La cuenta ya no está vacía o cambió en otra terminal (`ACCOUNT_NOT_EMPTY`,
     * `ACCOUNT_CONFLICT`): se vuelve a leer y se ve como está.
     */
    data object Changed : DiscardAccountResult

    /** Sin conexión o sin sesión de Go: descartar necesita conexión. */
    data object NoConnection : DiscardAccountResult

    /** Go lo rechazó por otro motivo (su mensaje, si lo dio). */
    data class Failed(val message: String?) : DiscardAccountResult
}

/** Resultado de «Enviar a cocina» para la pantalla de captura. */
sealed interface SendRequest {
    /** El pedido ya está en la cola con su estado (Enviando o Por enviar); la captura terminó. */
    data class Queued(val operationId: String, val state: OrderState) : SendRequest

    /** No hay nada de este mesero que enviar. */
    data object NothingToSend : SendRequest

    /** El borrador tiene más renglones de los que acepta Go en un pedido. */
    data object TooManyLines : SendRequest

    /**
     * Con conexión, el guardado anterior del borrador todavía no tiene respuesta: el pedido no se
     * arma hasta saber qué guardó Go (revisión R6, C1). El mesero lo vuelve a intentar en un
     * momento.
     */
    data object StillSaving : SendRequest

    /**
     * Etapa M1 (M1-D13): la captura lleva una unión en pantalla que todavía no nace en Go (sin
     * conexión, o Go la rechazó): nunca entra en la cola. El mesero elige «Enviar solo a la mesa» o
     * «Esperar».
     */
    data object UnionPending : SendRequest
}

/** Cómo quedó un descarte (de una captura o de un pedido en revisión). */
enum class DiscardResult {
    /** Ya no queda nada: lo del mesero se quitó también del borrador de Go (o nunca llegó). */
    DONE,

    /**
     * Sin conexión (o sin sesión): lo del mesero que ya estaba guardado en el borrador de Go se
     * quita en cuanto haya conexión; mientras, se ve como «Descarte pendiente» (revisión R6, A2).
     */
    PENDING,
}

/**
 * Capturas del mesero (el «Borrador» de §7), guardadas en Room **por mesero**. En línea, el
 * borrador también se guarda en Go con una espera corta y la revisión vigente, como el escritorio
 * (la cuenta nace con el primer producto); sin conexión, todo queda en el dispositivo.
 */
interface CaptureService {
    fun captures(userId: String): Flow<List<Capture>>

    fun capture(captureId: String): Flow<Capture?>

    /**
     * Retoma la captura del mesero en esa mesa y cuenta (o en una cuenta nueva de la mesa si
     * [account] es null) o empieza una. [seenAccountIds] son las cuentas abiertas que veía en la
     * mesa (`seen_account_ids`; en una unión, las de toda la unión). Etapa M1: [seenUnion] es la
     * unión que veía y [unionJoin], la unión en pantalla que nace con el primer producto.
     */
    suspend fun open(
        userId: String,
        tableId: String,
        tableName: String,
        account: AccountSummary?,
        seenAccountIds: List<String>,
        seenUnion: SeenUnion? = null,
        unionJoin: UnionJoin? = null,
    ): Capture

    /**
     * Etapa M1 (M1-D13): «Enviar solo a la mesa»: la captura deja la unión en pantalla y sigue como
     * una cuenta nueva de la mesa de inicio.
     */
    suspend fun dropUnion(captureId: String) {}

    /**
     * Para llevar (A2, §3.2): retoma la captura del mesero en ese pedido para llevar o, con
     * [account] null («Nuevo pedido»), empieza una nueva que entra directo al catálogo. El pedido
     * nace en Go con el primer producto (o, sin conexión, al enviarse); salir sin productos no deja
     * nada ni en Go ni en el dispositivo.
     */
    suspend fun openTakeaway(userId: String, account: AccountSummary?): Capture

    /**
     * Cambia el nombre y el teléfono del cliente de un pedido para llevar desde la captura (A2-D1).
     * Mientras el pedido no ha salido a Go se guardan en la captura (viajan al crearlo); después,
     * como una edición aparte que se envía en cuanto se pueda (la última gana).
     */
    suspend fun setCustomer(captureId: String, customer: TakeawayCustomer)

    /** Un toque sobre el borrador; [seenPrice] es el precio de referencia que vio el mesero. */
    suspend fun stage(captureId: String, operation: DraftOperation, seenPrice: Money?)

    /** Otra terminal cambió la cuenta: aplicar mis toques sobre la cuenta actual. */
    suspend fun keepMine(captureId: String)

    /** Otra terminal cambió la cuenta: descartar mis toques y quedarme con lo de Go. */
    suspend fun discardMine(captureId: String)

    /** La mesa ya tenía otra cuenta: agregar mis productos a esa. */
    suspend fun joinAccount(captureId: String, account: TableAccountRef)

    /** La mesa ya tenía otra cuenta: abrir otra de todos modos. */
    suspend fun openAnotherAccount(captureId: String)

    /** Reintenta guardar en Go (después de un rechazo o sin respuesta). */
    suspend fun retrySave(captureId: String)

    /** La cuenta completa de Go más reciente que vio la captura (para conciliar). */
    suspend fun observe(captureId: String, bill: AccountBill)

    /**
     * «Enviar a cocina» explícito (§0-B: un borrador nunca se envía solo). Con conexión y el
     * borrador guardado, confirma en Go con la huella de la cotización; si no, el pedido entra a la
     * cola sin conexión con su clave de operación.
     */
    suspend fun sendToKitchen(captureId: String, waiterName: String): SendRequest

    /** Cotización de Go del borrador guardado (precios para mostrar y la huella), si la hay. */
    fun quote(captureId: String): Flow<DraftQuote?>

    /**
     * Descarta la captura (con la confirmación del mesero) y quita del borrador de Go lo suyo que
     * ya estaba guardado; si la cuenta nació con esta captura y queda vacía, también la descarta
     * (revisión R6, A2). Sin conexión, el descarte queda pendiente ([DiscardResult.PENDING]).
     */
    suspend fun discard(captureId: String): DiscardResult

    /**
     * Al salir de la captura: si no hay nada del mesero, se olvida (no deja registro). No espera:
     * sigue aunque la pantalla ya se haya cerrado.
     */
    fun closeIfIdle(captureId: String)
}

/**
 * La cola de pedidos (§7) y «Requiere tu revisión» (§6.4), por mesero: el siguiente mesero no ve
 * los pedidos de otro.
 */
interface OrderQueueService {
    fun orders(userId: String): Flow<List<PendingOrder>>

    fun order(operationId: String): Flow<PendingOrder?>

    /** «Reintentar ya»: pide al trabajador que envíe enseguida lo que falta. */
    fun retryNow()

    /** «Aceptar los cambios y enviar». */
    suspend fun accept(operationId: String)

    /** «Quitar» una partida que Go señaló. */
    suspend fun removeLine(operationId: String, itemId: String)

    /** «Enviar» después de corregir. */
    suspend fun resend(operationId: String)

    /** «Abrir una cuenta nueva en la mesa». */
    suspend fun moveToNewAccount(operationId: String)

    /**
     * «Enviar a la cuenta en que se combinó» o «Enviar a esa cuenta» (las partidas ya están en una
     * cuenta de la mesa): el mismo pedido a [accountId], con la misma clave.
     */
    suspend fun sendToAccount(operationId: String, accountId: String)

    /**
     * «Corregir en la captura»: el pedido vuelve a ser un borrador del mesero en la misma mesa;
     * devuelve el identificador de la captura.
     */
    suspend fun backToCapture(operationId: String): String?

    /**
     * «Descartar el pedido» (con la confirmación del mesero): también quita del borrador de Go lo
     * suyo que ya estaba guardado ([DiscardResult.PENDING] sin conexión).
     */
    suspend fun discard(operationId: String): DiscardResult

    /** Lo pendiente de los **demás** meseros en este dispositivo, sin el detalle (B3). */
    fun othersPending(userId: String): Flow<List<OthersPending>>
}

/** Resultado de modificar o cancelar unidades enviadas. */
sealed interface SentChangeResult {
    data class Done(val bill: AccountBill) : SentChangeResult

    data class Failed(val error: AppError) : SentChangeResult
}

/**
 * Modificar o cancelar unidades ya enviadas (§6.3; `previewEdit`/`editUnits` con `orders.edit`,
 * reducir con `orders.cancel`, `cancelUnits` con `orders.cancel`), siempre con motivo y con
 * conexión, a nombre del mesero [userId] (queda registrado quién lo hizo y cocina recibe su aviso).
 */
interface SentLineService {
    suspend fun previewEdit(
        userId: String,
        accountId: String,
        revision: Int,
        versionId: String,
        line: LineSelection,
    ): AppResult<DraftQuote>

    suspend fun edit(
        userId: String,
        accountId: String,
        revision: Int,
        versionId: String,
        line: LineSelection,
        fingerprint: String,
        reason: String,
    ): SentChangeResult

    suspend fun cancel(
        userId: String,
        accountId: String,
        revision: Int,
        versionId: String,
        quantity: Int,
        reason: String,
    ): SentChangeResult
}
