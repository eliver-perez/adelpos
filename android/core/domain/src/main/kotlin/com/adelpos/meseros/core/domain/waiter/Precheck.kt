package com.adelpos.meseros.core.domain.waiter

import kotlinx.coroutines.flow.StateFlow

/*
 * Imprimir la precuenta desde el dispositivo (etapa A2, Fase C, §5 de la especificación, A2-D3 y la
 * «Forma final» de la parte C del servidor). Go decide todo: la impresora (solo la asignada a este
 * dispositivo en Terminales), el contenido (la misma plantilla y el mismo renderizador de la caja,
 * con «Atendió: <mesero>»), el permiso (`payments.tickets.print`) y qué le pasó al trabajo. El
 * dispositivo solo pide, sigue el estado unos segundos y lo dice en palabras del restaurante.
 * Imprimir **nunca** entra en la cola sin conexión.
 */

/**
 * Nombre visible del documento previo al cobro (etapa 9F): la lista cerrada de Go
 * (`payments.PrecheckNames`), elegida en Administración › Documentos y leída de
 * `establishment.precheck_name` de `getIdentity`. El botón dice «Imprimir precuenta», «Imprimir
 * ticket de venta» o «Imprimir cuenta»; el encabezado impreso lo pone Go igual.
 */
enum class PrecheckName(val key: String) {
    PRECHECK("precheck"),
    SALES_TICKET("sales_ticket"),
    BILL("bill");

    companion object {
        /** La clave de Go; una desconocida (o ninguna) es «Precuenta», como en Go. */
        fun fromKey(key: String?): PrecheckName = entries.firstOrNull { it.key == key } ?: PRECHECK
    }
}

/**
 * La impresora de precuentas de este dispositivo (Terminales › Editar,
 * `terminal.precheck_printer`). [terminalName] es la terminal que la tiene conectada si es la USB
 * de una terminal («Caja 2»).
 */
data class PrecheckPrinter(val name: String, val terminalName: String?) {
    /** Cómo se dice a dónde va: la terminal si es la USB de una terminal; si no, la impresora. */
    val destination: String
        get() = terminalName?.takeIf { it.isNotBlank() } ?: name
}

/**
 * Lo último que se leyó de Go para imprimir, guardado para verse también sin conexión: el nombre
 * visible y la impresora de este dispositivo (null si no tiene). Con [read] en false nunca se leyó:
 * no se avisa nada antes de tocar (la respuesta de Go manda siempre).
 */
data class PrecheckSetup(
    val name: PrecheckName,
    val printer: PrecheckPrinter?,
    val read: Boolean,
) {
    companion object {
        val UNKNOWN = PrecheckSetup(PrecheckName.PRECHECK, printer = null, read = false)
    }
}

/** Lo que respondió Go a una petición de impresión (la traduce `core:data`). */
sealed interface PrintAnswer {
    /**
     * 200 de `printPrecheck` o `retryDocument`: el trabajo quedó en la cola de impresión de Go.
     * [problem] es lo que Go ya sabe al encolar (`agent_offline`: la terminal de la impresora está
     * desconectada; el trabajo espera y sale cuando vuelva).
     */
    data class Queued(
        val ticketId: String,
        val jobId: String,
        val status: String?,
        val problem: String?,
        val printerName: String?,
        val terminalName: String?,
    ) : PrintAnswer

    /** 200 de `getDocumentStatus`: el estado del último trabajo del documento. */
    data class Status(
        val jobState: String?,
        val jobProblem: String?,
        val printerName: String?,
        val terminalName: String?,
    ) : PrintAnswer

    /**
     * Go lo rechazó con su código (`PRINTER_NOT_CONFIGURED`, `PRINTER_UNAVAILABLE`,
     * `PERMISSION_DENIED`, `PRINT_NOT_FAILED`…) y su mensaje en español; [status] es el HTTP.
     */
    data class Rejected(
        val status: Int,
        val code: String?,
        val message: String?,
    ) : PrintAnswer

    /** Sin conexión con la computadora principal (sin sesión de Go, sin red o sin respuesta). */
    data object NoConnection : PrintAnswer
}

/**
 * Lo que ve el mesero junto al botón, en palabras del restaurante (los textos, en `core:capture`).
 */
sealed interface PrintMessage {
    /** La petición va en camino. */
    data object Sending : PrintMessage

    /**
     * «Enviada a Caja 1»: Go la encoló (o la impresora o su cola ya la aceptó) en [destination], la
     * terminal de la impresora USB o la impresora de red; null si Go no dijo cuál.
     */
    data class SentTo(val destination: String?) : PrintMessage

    /**
     * «La Caja 2 está desconectada» (`agent_offline`): la impresora es la USB de [terminal] y esa
     * terminal no está trabajando; la precuenta espera y sale sola cuando vuelva.
     */
    data class TerminalOffline(val terminal: String?) : PrintMessage

    /** «La impresora no responde» (`unreachable`): nada salió en papel; se puede reintentar. */
    data class NotResponding(val printer: String?) : PrintMessage

    /**
     * La impresora necesita revisión (`not_found`, `setup`, `inactive`, `unsupported`): avisar al
     * administrador.
     */
    data class NeedsReview(val printer: String?) : PrintMessage

    /**
     * `uncertain`: no se sabe si salió en papel; revisarlo antes de pedir otra (sin «Reintentar»).
     */
    data object CheckPaper : PrintMessage

    /** El trabajo se canceló en la cola de impresión (desde la computadora principal). */
    data object Cancelled : PrintMessage

    /**
     * «Este dispositivo no tiene impresora asignada» (A2-D3, 409 `PRINTER_NOT_CONFIGURED` con
     * `details.scope: "device"`): pedírsela al administrador. Nunca se usa otra impresora.
     */
    data object NoPrinter : PrintMessage

    /**
     * 409 `PRINTER_UNAVAILABLE`: la impresora asignada no sirve ahora (desactivada o que no puede
     * imprimir). [message] es el de Go («La impresora Caja está desactivada.»).
     */
    data class PrinterUnavailable(val message: String?) : PrintMessage

    /** «Para imprimir necesitas conexión con la computadora principal». */
    data object NoConnection : PrintMessage

    /** 403: el mesero ya no tiene el permiso de imprimir. */
    data object NoPermission : PrintMessage

    /** Otro rechazo de Go, con su mensaje (null: el texto general). */
    data class Failed(val message: String?) : PrintMessage
}

/**
 * Cómo va la impresión de la precuenta en la cuenta abierta.
 *
 * @param busy hay una petición en curso: el botón no se repite.
 * @param canRetry se ofrece «Reintentar» sin motivo: el trabajo **nunca salió en papel** (`error` o
 *   `intervention`, regla de la 9F). Si salió, volver a imprimir es tocar otra vez el botón (otra
 *   precuenta, como en la caja; no hay copia con motivo en el dispositivo).
 * @param following se consulta el estado del trabajo (unos 15 s).
 */
data class PrecheckPrintState(
    val message: PrintMessage?,
    val busy: Boolean,
    val canRetry: Boolean,
    val following: Boolean,
) {
    companion object {
        val IDLE =
            PrecheckPrintState(message = null, busy = false, canRetry = false, following = false)
    }
}

/**
 * Imprimir la precuenta con la sesión del mesero [userId] (nunca con la credencial de envío: la
 * impresión no va a la cola). Lo implementa `core:data` sobre `getIdentity`, `printPrecheck`,
 * `getDocumentStatus` y `retryDocument`.
 */
interface PrecheckService {
    /**
     * Nombre visible e impresora de este dispositivo: lo último que se leyó (también sin conexión).
     */
    val setup: StateFlow<PrecheckSetup>

    /** El dispositivo tiene red (el indicador dice «En línea»). */
    val connected: StateFlow<Boolean>

    /** Vuelve a leer el nombre visible y la impresora (`getIdentity`) con la sesión de [userId]. */
    suspend fun refreshSetup(userId: String)

    /**
     * Pide una precuenta nueva de la cuenta con el estado vigente; [operationId] es la clave de la
     * petición (la misma si se repite por falta de respuesta: Go no imprime dos).
     */
    suspend fun print(userId: String, accountId: String, operationId: String): PrintAnswer

    /** El estado del último trabajo de la precuenta [ticketId] de esa cuenta. */
    suspend fun status(userId: String, accountId: String, ticketId: String): PrintAnswer

    /** «Reintentar» sin motivo la precuenta [ticketId] que nunca salió en papel. */
    suspend fun retry(
        userId: String,
        accountId: String,
        ticketId: String,
        operationId: String,
    ): PrintAnswer
}

/**
 * Reglas de lo que se le dice al mesero, puras (las pruebas de la JVM las recorren). No deciden
 * nada del negocio: traducen las claves de Go (`kitchen/print_problems.go`) y sus códigos.
 */
object PrecheckFeedback {
    // Estados del trabajo de impresión de Go (`kitchen_print_jobs.state`).
    const val STATE_PENDING = "pending"
    const val STATE_PROCESSING = "processing"
    const val STATE_SUBMITTED = "submitted"
    const val STATE_ERROR = "error"
    const val STATE_INTERVENTION = "intervention"
    const val STATE_UNCERTAIN = "uncertain"
    const val STATE_CANCELLED = "cancelled"

    // Problemas de un trabajo (lista cerrada de Go).
    const val PROBLEM_UNREACHABLE = "unreachable"
    const val PROBLEM_AGENT_OFFLINE = "agent_offline"
    const val PROBLEM_NOT_FOUND = "not_found"
    const val PROBLEM_INACTIVE = "inactive"
    const val PROBLEM_UNSUPPORTED = "unsupported"
    const val PROBLEM_SETUP = "setup"
    const val PROBLEM_UNCERTAIN = "uncertain"

    // Códigos de rechazo de Go.
    const val PRINTER_NOT_CONFIGURED = "PRINTER_NOT_CONFIGURED"
    const val PRINTER_UNAVAILABLE = "PRINTER_UNAVAILABLE"
    const val PERMISSION_DENIED = "PERMISSION_DENIED"
    const val PRINT_NOT_FAILED = "PRINT_NOT_FAILED"
    const val PRINT_UNCERTAIN = "PRINT_UNCERTAIN"
    const val PRINT_JOB_SUPERSEDED = "PRINT_JOB_SUPERSEDED"

    /** El trabajo nunca salió en papel: «Reintentar» sin motivo (regla de la 9F). */
    private val RETRYABLE_STATES = setOf(STATE_ERROR, STATE_INTERVENTION)

    /** Estados en que ya no hay nada que seguir: la impresora lo aceptó o exige una decisión. */
    private val FINAL_STATES =
        setOf(STATE_SUBMITTED, STATE_ERROR, STATE_INTERVENTION, STATE_UNCERTAIN, STATE_CANCELLED)

    private val REVIEW_PROBLEMS =
        setOf(PROBLEM_NOT_FOUND, PROBLEM_INACTIVE, PROBLEM_UNSUPPORTED, PROBLEM_SETUP)

    /**
     * Rechazos de «Reintentar» que solo dicen que el trabajo ya no está fallido (salió, ya no se
     * sabe o hay una impresión más reciente): se vuelve a leer el estado.
     */
    private val STALE_RETRY_CODES = setOf(PRINT_NOT_FAILED, PRINT_UNCERTAIN, PRINT_JOB_SUPERSEDED)

    /** A dónde se envió: la terminal de la impresora USB o la impresora (null si Go no lo dijo). */
    fun destination(printerName: String?, terminalName: String?): String? =
        terminalName?.takeIf { it.isNotBlank() } ?: printerName?.takeIf { it.isNotBlank() }

    /** Lo que se dice de un trabajo con su estado y su problema (respuesta o consulta). */
    fun message(
        jobState: String?,
        jobProblem: String?,
        printerName: String?,
        terminalName: String?,
    ): PrintMessage =
        when {
            jobProblem == PROBLEM_AGENT_OFFLINE -> PrintMessage.TerminalOffline(terminalName)
            jobProblem == PROBLEM_UNREACHABLE -> PrintMessage.NotResponding(printerName)
            jobProblem == PROBLEM_UNCERTAIN || jobState == STATE_UNCERTAIN ->
                PrintMessage.CheckPaper
            jobProblem in REVIEW_PROBLEMS -> PrintMessage.NeedsReview(printerName)
            jobState == STATE_CANCELLED -> PrintMessage.Cancelled
            // Sin la clave del problema (no debería pasar: Go la calcula siempre para estos).
            jobState == STATE_ERROR -> PrintMessage.NotResponding(printerName)
            jobState == STATE_INTERVENTION -> PrintMessage.NeedsReview(printerName)
            // pending, processing o submitted: va (o ya está) en esa impresora.
            else -> PrintMessage.SentTo(destination(printerName, terminalName))
        }

    /**
     * «Reintentar» sin motivo solo si el trabajo nunca salió en papel (`error`, `intervention`).
     * Con la terminal desconectada no hace falta: la precuenta sale sola cuando vuelva.
     */
    fun canRetry(jobState: String?, jobProblem: String?): Boolean =
        jobState in RETRYABLE_STATES &&
            jobProblem != PROBLEM_UNCERTAIN &&
            jobProblem != PROBLEM_AGENT_OFFLINE

    /** Ya no hay nada que seguir consultando. */
    fun isFinal(jobState: String?): Boolean = jobState in FINAL_STATES

    /** Lo que se dice de un rechazo de Go al pedir, reintentar o consultar. */
    fun rejection(code: String?, status: Int, message: String?): PrintMessage =
        when {
            code == PRINTER_NOT_CONFIGURED -> PrintMessage.NoPrinter
            code == PRINTER_UNAVAILABLE -> PrintMessage.PrinterUnavailable(message)
            code == PERMISSION_DENIED || status == HTTP_FORBIDDEN -> PrintMessage.NoPermission
            else -> PrintMessage.Failed(message)
        }

    /** Un rechazo de «Reintentar» que pide volver a leer el estado en lugar de mostrarse. */
    fun isStaleRetry(code: String?): Boolean = code in STALE_RETRY_CODES

    /**
     * Cómo empieza la frase de una terminal desconectada: «La Caja 2» si su nombre ya dice que es
     * una caja, una terminal o una computadora; si no, «La terminal Mostrador», para que la frase
     * concuerde («está desconectada»). La misma regla del escritorio (`printProblems.ts`).
     */
    fun terminalNamesItself(terminal: String): Boolean {
        val lower = terminal.trim().lowercase()
        return lower.startsWith("caja") ||
            lower.startsWith("terminal") ||
            lower.startsWith("computadora")
    }

    private const val HTTP_FORBIDDEN = 403
}
