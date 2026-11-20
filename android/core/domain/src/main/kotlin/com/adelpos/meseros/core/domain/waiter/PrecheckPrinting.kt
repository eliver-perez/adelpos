package com.adelpos.meseros.core.domain.waiter

import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Job
import kotlinx.coroutines.delay
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.launch

/**
 * La impresión de la precuenta de la cuenta abierta (etapa A2, §5): pide a Go, dice lo que pasó y
 * sigue el trabajo con `getDocumentStatus` unos [followMillis] (cada [pollMillis]) **sin bloquear
 * la captura**: todo corre en [scope] (el de la pantalla de la cuenta, así que el seguimiento
 * termina cuando el mesero sale de ella). Nunca entra en la cola sin conexión: sin conexión solo se
 * dice.
 *
 * - Tocar «Imprimir …» otra vez arma **otra** precuenta con una clave nueva (como en la caja; si ya
 *   salió en papel, esa es la regla de reimpresión que dejó la parte C).
 * - «Reintentar» ([retry]) solo se ofrece si el trabajo nunca salió en papel (regla de la 9F) y
 *   reenvía la misma precuenta, sin motivo.
 */
class PrecheckPrinting(
    private val service: PrecheckService,
    private val scope: CoroutineScope,
    private val newId: () -> String,
    private val followMillis: Long = FOLLOW_MILLIS,
    private val pollMillis: Long = POLL_MILLIS,
) {
    private val mutableState = MutableStateFlow(PrecheckPrintState.IDLE)
    private var job: Job? = null

    /** La precuenta de la última impresión de esta cuenta (para seguirla y reintentarla). */
    private var ticketId: String? = null

    val state: StateFlow<PrecheckPrintState> = mutableState.asStateFlow()

    /** «Imprimir precuenta» (o el nombre visible): una precuenta nueva con el estado vigente. */
    fun print(userId: String, accountId: String) {
        if (mutableState.value.busy) {
            return
        }
        start(userId, accountId, retrying = false) { service.print(userId, accountId, newId()) }
    }

    /** «Reintentar» la precuenta que nunca salió en papel, sin motivo. */
    fun retry(userId: String, accountId: String) {
        val ticket = ticketId ?: return
        if (mutableState.value.busy || !mutableState.value.canRetry) {
            return
        }
        start(userId, accountId, retrying = true) {
            service.retry(userId, accountId, ticket, newId())
        }
    }

    /** Otra cuenta (la captura cambió de cuenta): se olvida lo de la anterior. */
    fun reset() {
        job?.cancel()
        job = null
        ticketId = null
        mutableState.value = PrecheckPrintState.IDLE
    }

    // La petición y su seguimiento en una sola corrutina: una impresión nueva cancela el
    // seguimiento
    // de la anterior (solo se dice lo de la última).
    private fun start(
        userId: String,
        accountId: String,
        retrying: Boolean,
        request: suspend () -> PrintAnswer,
    ) {
        job?.cancel()
        mutableState.value =
            PrecheckPrintState(
                message = PrintMessage.Sending,
                busy = true,
                canRetry = false,
                following = false,
            )
        job = scope.launch { handle(request(), userId, accountId, retrying) }
    }

    private suspend fun handle(
        answer: PrintAnswer,
        userId: String,
        accountId: String,
        retrying: Boolean,
    ) {
        when (answer) {
            is PrintAnswer.Queued -> {
                ticketId = answer.ticketId
                val state = answer.status ?: PrecheckFeedback.STATE_PENDING
                show(
                    state,
                    answer.problem,
                    answer.printerName,
                    answer.terminalName,
                    following = true,
                )
                follow(userId, accountId, answer.ticketId)
            }
            is PrintAnswer.Rejected -> {
                if (PrecheckFeedback.isStaleRetry(answer.code)) {
                    // El trabajo ya no está fallido (salió, no se sabe o hay uno más nuevo): se
                    // vuelve a leer lo que dice Go en lugar de mostrar el rechazo.
                    val ticket = ticketId
                    if (ticket != null) {
                        refresh(userId, accountId, ticket)
                        return
                    }
                }
                mutableState.value =
                    PrecheckPrintState(
                        message =
                            PrecheckFeedback.rejection(answer.code, answer.status, answer.message),
                        busy = false,
                        canRetry = false,
                        following = false,
                    )
            }
            PrintAnswer.NoConnection ->
                mutableState.value =
                    PrecheckPrintState(
                        message = PrintMessage.NoConnection,
                        busy = false,
                        // Un «Reintentar» sin conexión se puede volver a tocar cuando vuelva.
                        canRetry = retrying,
                        following = false,
                    )
            PrintAnswer.NoAnswer ->
                mutableState.value =
                    PrecheckPrintState(
                        message = PrintMessage.Failed(null),
                        busy = false,
                        canRetry = retrying,
                        following = false,
                    )
            is PrintAnswer.Status ->
                show(answer.jobState, answer.jobProblem, answer.printerName, answer.terminalName)
        }
    }

    /**
     * Consulta el estado cada [pollMillis] hasta que no haya nada que seguir o pasen
     * [followMillis].
     */
    private suspend fun follow(userId: String, accountId: String, ticket: String) {
        var waited = 0L
        while (waited < followMillis && mutableState.value.following) {
            delay(pollMillis)
            waited += pollMillis
            val answer = service.status(userId, accountId, ticket)
            if (answer is PrintAnswer.Status) {
                show(
                    answer.jobState,
                    answer.jobProblem,
                    answer.printerName,
                    answer.terminalName,
                    following = !PrecheckFeedback.isFinal(answer.jobState),
                )
            }
            // Sin respuesta en una consulta se deja lo último que se supo y se vuelve a intentar.
        }
        if (mutableState.value.following) {
            mutableState.value = mutableState.value.copy(following = false)
        }
    }

    /**
     * Vuelve a leer el estado (un «Reintentar» que ya no hacía falta) y, si el trabajo sigue en
     * camino (el trabajador de Go lo estaba reintentando solo), lo sigue como al imprimir.
     */
    private suspend fun refresh(userId: String, accountId: String, ticket: String) {
        when (val answer = service.status(userId, accountId, ticket)) {
            is PrintAnswer.Status -> {
                show(
                    answer.jobState,
                    answer.jobProblem,
                    answer.printerName,
                    answer.terminalName,
                    following = true,
                )
                follow(userId, accountId, ticket)
            }
            else ->
                mutableState.value =
                    PrecheckPrintState(
                        message = PrintMessage.NoConnection,
                        busy = false,
                        canRetry = false,
                        following = false,
                    )
        }
    }

    private fun show(
        jobState: String?,
        jobProblem: String?,
        printerName: String?,
        terminalName: String?,
        following: Boolean = false,
    ) {
        mutableState.value =
            PrecheckPrintState(
                message = PrecheckFeedback.message(jobState, jobProblem, printerName, terminalName),
                busy = false,
                canRetry = PrecheckFeedback.canRetry(jobState, jobProblem),
                following = following && !PrecheckFeedback.isFinal(jobState),
            )
    }

    companion object {
        /** Cuánto se sigue el trabajo después de pedirlo («unos 15 s» de la parte C). */
        const val FOLLOW_MILLIS = 15_000L

        /** Cada cuánto se consulta su estado («cada 2–3 s»). */
        const val POLL_MILLIS = 2_500L
    }
}
