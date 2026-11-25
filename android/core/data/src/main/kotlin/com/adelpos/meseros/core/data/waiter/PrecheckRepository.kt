package com.adelpos.meseros.core.data.waiter

import com.adelpos.core.network.api.ApiErrors
import com.adelpos.core.network.api.ApiJson
import com.adelpos.core.network.api.ApiOutcome
import com.adelpos.core.network.license.LicenseStatusRepository
import com.adelpos.core.network.license.LicenseSummaryDto
import com.adelpos.meseros.core.data.waiter.db.RestaurantSnapshotEntity
import com.adelpos.meseros.core.data.waiter.db.WaiterDao
import com.adelpos.meseros.core.domain.waiter.DeviceHome
import com.adelpos.meseros.core.domain.waiter.PrecheckName
import com.adelpos.meseros.core.domain.waiter.PrecheckPrinter
import com.adelpos.meseros.core.domain.waiter.PrecheckService
import com.adelpos.meseros.core.domain.waiter.PrecheckSetup
import com.adelpos.meseros.core.domain.waiter.PrintAnswer
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.delay
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.distinctUntilChanged
import kotlinx.coroutines.flow.map
import kotlinx.coroutines.launch
import kotlinx.serialization.SerialName
import kotlinx.serialization.Serializable
import kotlinx.serialization.SerializationException

/*
 * Formas del contrato v1 para imprimir la precuenta desde el dispositivo (etapa A2, parte C del
 * servidor: `getIdentity`, `printPrecheck`, `getDocumentStatus` y `retryDocument` en
 * `backend/internal/apicontract/openapi.json` y `docs/API_V1.md`, «Imprimir la precuenta»). Solo
 * lo que usa el dispositivo; lo demás lo ignora ApiJson.
 */

/** `Identity` de `getIdentity`: lo que el dispositivo necesita para imprimir. */
@Serializable
data class IdentityDto(
    val terminal: IdentityTerminalDto? = null,
    val establishment: IdentityEstablishmentDto? = null,
    // Etapa L1: la licencia y la hora de la computadora principal (los avisos de la barra).
    val license: LicenseSummaryDto? = null,
)

@Serializable
data class IdentityTerminalDto(
    @SerialName("precheck_printer") val precheckPrinter: PrecheckPrinterDto? = null,
    // Etapa A3 (§4.12): la pantalla inicial del dispositivo que se guarda en Terminales.
    @SerialName("home_screen") val homeScreen: String? = null,
)

/** `Terminal.precheck_printer`: la impresora de precuentas del dispositivo, o null. */
@Serializable
data class PrecheckPrinterDto(
    val id: String,
    val name: String,
    @SerialName("terminal_name") val terminalName: String? = null,
)

@Serializable
data class IdentityEstablishmentDto(
    @SerialName("precheck_name") val precheckName: String? = null,
)

/** Cuerpo de `printPrecheck`. */
@Serializable data class PrintPrecheckBody(@SerialName("operation_id") val operationId: String)

/** Cuerpo de `retryDocument` (sin `job_id`: Go reintenta el trabajo más reciente). */
@Serializable
data class RetryDocumentBody(
    @SerialName("operation_id") val operationId: String,
    @SerialName("ticket_id") val ticketId: String,
)

/** `PrintedDocument`: respuesta de `printPrecheck` y `retryDocument`. */
@Serializable
data class PrintedDocumentDto(
    val status: String? = null,
    @SerialName("job_id") val jobId: String,
    @SerialName("ticket_id") val ticketId: String,
    val problem: String? = null,
    @SerialName("printer_name") val printerName: String? = null,
    @SerialName("terminal_name") val terminalName: String? = null,
)

/** `DocumentStatus` de `getDocumentStatus` (sin importes ni pagos). */
@Serializable
data class DocumentStatusDto(
    @SerialName("ticket_id") val ticketId: String,
    @SerialName("job_state") val jobState: String? = null,
    @SerialName("job_problem") val jobProblem: String? = null,
    @SerialName("printer_name") val printerName: String? = null,
    @SerialName("terminal_name") val terminalName: String? = null,
)

/** Lo que se guarda de `getIdentity` en `restaurant_snapshot` para verlo sin conexión. */
@Serializable
data class SavedPrecheckSetup(
    @SerialName("precheck_name") val precheckName: String,
    val printer: PrecheckPrinterDto? = null,
    // Etapa A3: lo guardado antes no lo tiene (Mesas).
    @SerialName("home_screen") val homeScreen: String? = null,
)

/**
 * Imprimir la precuenta (etapa A2, §5) con la **sesión del mesero** que la pide: nunca va a la cola
 * sin conexión ni usa la credencial de envío (que solo sirve para `submitOfflineOrder`, revisión R3
 * de la A1). Sin sesión o con «Sin conexión» responde [PrintAnswer.NoConnection] sin pedir nada.
 *
 * El nombre visible (`establishment.precheck_name`) y la impresora de este dispositivo
 * (`terminal.precheck_printer`) se leen de `getIdentity` al entrar un mesero con conexión y al
 * abrir una cuenta, y se guardan en `restaurant_snapshot` (tipo `precheck`; sin cambiar el esquema
 * de Room) para mostrarse sin conexión. Ese registro lo borra `WaiterDao.eraseAll`, ya en
 * `deviceDataWipe`.
 *
 * @param connected el estado único de conexión está en «En línea» (etapa A3, A3-D1: el mismo de la
 *   barra; antes, solo la red de Android).
 * @param confirmConnection el reintento en el momento: con «Sin conexión» y red de Android,
 *   pregunta a Go antes de decir que falta conexión (`ServerLink.confirm`).
 */
class PrecheckRepository(
    private val dao: WaiterDao,
    private val server: WaiterServer,
    private val api: WaiterApi,
    override val connected: StateFlow<Boolean>,
    private val scope: CoroutineScope,
    private val confirmConnection: suspend () -> Boolean = { connected.value },
    private val wallClock: () -> Long = System::currentTimeMillis,
    private val retryDelayMillis: Long = RETRY_DELAY_MILLIS,
    private val license: LicenseStatusRepository? = null,
) : PrecheckService {
    private val mutableSetup = MutableStateFlow(PrecheckSetup.UNKNOWN)
    private var started = false

    override val setup: StateFlow<PrecheckSetup> = mutableSetup.asStateFlow()

    /** Lee lo guardado y vuelve a leer de Go cada vez que un mesero entra con conexión. */
    fun start() {
        if (started) {
            return
        }
        started = true
        scope.launch {
            loadSaved()
            server.activeWaiter
                .map { waiter -> waiter?.takeIf { it.hasServerSession }?.userId }
                .distinctUntilChanged()
                .collect { userId -> if (userId != null) refreshSetup(userId) }
        }
    }

    override suspend fun refreshSetup(userId: String) {
        val transport = server.transport() ?: return
        val headers = server.sessionHeaders(userId) ?: return
        val outcome = api.identity(transport, headers)
        when (outcome) {
            is ApiOutcome.Success -> {
                val saved =
                    SavedPrecheckSetup(
                        precheckName =
                            outcome.value.establishment?.precheckName ?: PrecheckName.PRECHECK.key,
                        printer = outcome.value.terminal?.precheckPrinter,
                        homeScreen = outcome.value.terminal?.homeScreen,
                    )
                // Primero se guarda (para verlo sin conexión) y después se muestra.
                dao.saveSnapshot(
                    RestaurantSnapshotEntity(
                        kind = SNAPSHOT_KIND,
                        document = ApiJson.encodeToString(SavedPrecheckSetup.serializer(), saved),
                        readAt = wallClock(),
                    ),
                )
                mutableSetup.value = saved.toSetup()
                // Etapa L1: la misma lectura dice cómo está la licencia y la hora.
                license?.apply(outcome.value.license)
            }
            is ApiOutcome.Rejected -> reportAccess(outcome, userId)
            // Sin respuesta: se queda lo último que se leyó.
            is ApiOutcome.NoAnswer,
            is ApiOutcome.Malformed -> Unit
        }
    }

    override suspend fun print(
        userId: String,
        accountId: String,
        operationId: String,
    ): PrintAnswer {
        val transport = server.transport() ?: return PrintAnswer.NoConnection
        val headers = server.sessionHeaders(userId) ?: return PrintAnswer.NoConnection
        if (!confirmConnection()) {
            return PrintAnswer.NoConnection
        }
        val body = PrintPrecheckBody(operationId = operationId)
        var outcome = api.printPrecheck(transport, headers, accountId, body)
        if (outcome is ApiOutcome.NoAnswer && outcome.requestSent) {
            // Pudo haber entrado: se repite **igual** (misma clave) y Go no imprime dos.
            delay(retryDelayMillis)
            outcome = api.printPrecheck(transport, headers, accountId, body)
        }
        val answer = queued(outcome, userId)
        afterPrint(answer, userId)
        return answer
    }

    override suspend fun status(userId: String, accountId: String, ticketId: String): PrintAnswer {
        val transport = server.transport() ?: return PrintAnswer.NoConnection
        val headers = server.sessionHeaders(userId) ?: return PrintAnswer.NoConnection
        return when (val outcome = api.documentStatus(transport, headers, accountId, ticketId)) {
            is ApiOutcome.Success ->
                PrintAnswer.Status(
                    jobState = outcome.value.jobState,
                    jobProblem = outcome.value.jobProblem,
                    printerName = outcome.value.printerName,
                    terminalName = outcome.value.terminalName,
                )
            is ApiOutcome.Rejected -> rejected(outcome, userId)
            is ApiOutcome.NoAnswer,
            is ApiOutcome.Malformed -> PrintAnswer.NoConnection
        }
    }

    override suspend fun retry(
        userId: String,
        accountId: String,
        ticketId: String,
        operationId: String,
    ): PrintAnswer {
        val transport = server.transport() ?: return PrintAnswer.NoConnection
        val headers = server.sessionHeaders(userId) ?: return PrintAnswer.NoConnection
        if (!confirmConnection()) {
            return PrintAnswer.NoConnection
        }
        val body = RetryDocumentBody(operationId = operationId, ticketId = ticketId)
        var outcome = api.retryDocument(transport, headers, accountId, body)
        if (outcome is ApiOutcome.NoAnswer && outcome.requestSent) {
            delay(retryDelayMillis)
            outcome = api.retryDocument(transport, headers, accountId, body)
        }
        val answer = queued(outcome, userId)
        afterPrint(answer, userId)
        return answer
    }

    private fun queued(outcome: ApiOutcome<PrintedDocumentDto>, userId: String): PrintAnswer =
        when (outcome) {
            is ApiOutcome.Success ->
                PrintAnswer.Queued(
                    ticketId = outcome.value.ticketId,
                    jobId = outcome.value.jobId,
                    status = outcome.value.status,
                    problem = outcome.value.problem,
                    printerName = outcome.value.printerName,
                    terminalName = outcome.value.terminalName,
                )
            is ApiOutcome.Rejected -> rejected(outcome, userId)
            // Sin respuesta: «necesitas conexión» solo si el estado único ya dice «Sin conexión».
            is ApiOutcome.NoAnswer ->
                if (connected.value) PrintAnswer.NoAnswer else PrintAnswer.NoConnection
            is ApiOutcome.Malformed -> PrintAnswer.Rejected(outcome.status, null, null)
        }

    /**
     * Después de imprimir (o de un rechazo por la impresora), la impresora asignada pudo haber
     * cambiado en Terminales: se vuelve a leer (aparte, sin hacer esperar la respuesta) para que la
     * cuenta diga a dónde va.
     */
    private fun afterPrint(answer: PrintAnswer, userId: String) {
        if (answer is PrintAnswer.Queued || answer is PrintAnswer.Rejected) {
            scope.launch { refreshSetup(userId) }
        }
    }

    private fun rejected(outcome: ApiOutcome.Rejected, userId: String): PrintAnswer.Rejected {
        reportAccess(outcome, userId)
        return PrintAnswer.Rejected(
            status = outcome.problem.status,
            code = outcome.problem.code,
            message = ApiErrors.shownMessage(outcome.problem),
        )
    }

    /** Los errores de acceso (sesión, dispositivo, versión) van al punto único de la entrada. */
    private fun reportAccess(outcome: ApiOutcome.Rejected, userId: String) {
        val error = ApiErrors.toAppError(outcome.problem)
        if (isAccessProblem(error)) {
            server.report(error, userId)
        }
    }

    private suspend fun loadSaved() {
        val saved = dao.snapshot(SNAPSHOT_KIND) ?: return
        if (mutableSetup.value.read) {
            return
        }
        val decoded =
            try {
                ApiJson.decodeFromString(SavedPrecheckSetup.serializer(), saved.document)
            } catch (_: SerializationException) {
                return
            } catch (_: IllegalArgumentException) {
                return
            }
        mutableSetup.value = decoded.toSetup()
    }

    private fun SavedPrecheckSetup.toSetup(): PrecheckSetup =
        PrecheckSetup(
            name = PrecheckName.fromKey(precheckName),
            printer = printer?.let { PrecheckPrinter(it.name, it.terminalName) },
            read = true,
            home = DeviceHome.fromWire(homeScreen),
        )

    companion object {
        /** `restaurant_snapshot.kind` de lo que se guarda para imprimir. */
        const val SNAPSHOT_KIND = "precheck"

        private const val RETRY_DELAY_MILLIS = 1_500L
    }
}
