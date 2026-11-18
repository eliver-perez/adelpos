package com.adelpos.meseros.core.domain.waiter

import com.adelpos.meseros.core.domain.waiter.PrecheckFeedback.message
import kotlinx.coroutines.ExperimentalCoroutinesApi
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.test.TestScope
import kotlinx.coroutines.test.advanceTimeBy
import kotlinx.coroutines.test.advanceUntilIdle
import kotlinx.coroutines.test.runCurrent
import kotlinx.coroutines.test.runTest
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Test

/**
 * Imprimir la precuenta desde el dispositivo (etapa A2, Fase C, §5): el nombre visible, cada clave
 * de Go (estado y problema del trabajo, códigos de rechazo) → lo que se le dice al mesero, cuándo
 * se ofrece «Reintentar» (solo si nunca salió en papel, regla de la 9F), sin conexión, y el
 * seguimiento de unos 15 s que nunca bloquea ni repite la impresión.
 */
@OptIn(ExperimentalCoroutinesApi::class)
class PrecheckRulesTest {
    // --- Nombre visible ---

    @Test
    fun nombreVisibleDeLaListaCerradaDeGo() {
        assertEquals(PrecheckName.PRECHECK, PrecheckName.fromKey("precheck"))
        assertEquals(PrecheckName.SALES_TICKET, PrecheckName.fromKey("sales_ticket"))
        assertEquals(PrecheckName.BILL, PrecheckName.fromKey("bill"))
        // Una clave desconocida (o ninguna) es «Precuenta», como en Go.
        assertEquals(PrecheckName.PRECHECK, PrecheckName.fromKey("otra"))
        assertEquals(PrecheckName.PRECHECK, PrecheckName.fromKey(null))
        assertEquals(PrecheckName.PRECHECK, PrecheckSetup.UNKNOWN.name)
        assertFalse(PrecheckSetup.UNKNOWN.read)
    }

    @Test
    fun aDondeVaLaPrecuenta() {
        assertEquals("Caja 2", PrecheckPrinter("USB Caja 2", "Caja 2").destination)
        assertEquals("Barra", PrecheckPrinter("Barra", null).destination)
        assertEquals("Barra", PrecheckPrinter("Barra", " ").destination)
        assertEquals("Caja 1", PrecheckFeedback.destination("USB", "Caja 1"))
        assertEquals("Red", PrecheckFeedback.destination("Red", null))
        assertNull(PrecheckFeedback.destination(null, null))
        assertTrue(PrecheckFeedback.terminalNamesItself("Caja 2"))
        assertTrue(PrecheckFeedback.terminalNamesItself("terminal sur"))
        assertTrue(PrecheckFeedback.terminalNamesItself("Computadora de la barra"))
        // «La terminal Mostrador está desconectada», para que la frase concuerde.
        assertFalse(PrecheckFeedback.terminalNamesItself("Mostrador"))
    }

    // --- Cada estado y problema de Go → su aviso ---

    @Test
    fun cadaProblemaDelTrabajoTieneSuAviso() {
        // En la cola o aceptada por la impresora: «Enviada a Caja 1» (la terminal manda).
        for (state in listOf("pending", "processing", "submitted", null)) {
            assertEquals(
                PrintMessage.SentTo("Caja 1"),
                message(state, null, "USB Caja 1", "Caja 1"),
            )
        }
        assertEquals(PrintMessage.SentTo("Red"), message("submitted", null, "Red", null))
        // «La Caja 2 está desconectada»: ya al encolar (pending) o con un error que se reintenta.
        assertEquals(
            PrintMessage.TerminalOffline("Caja 2"),
            message("pending", "agent_offline", "USB", "Caja 2"),
        )
        assertEquals(
            PrintMessage.TerminalOffline("Caja 2"),
            message("error", "agent_offline", "USB", "Caja 2"),
        )
        // «La impresora no responde».
        assertEquals(
            PrintMessage.NotResponding("Red"),
            message("error", "unreachable", "Red", null),
        )
        // Revisión del administrador.
        for (problem in listOf("not_found", "setup", "inactive", "unsupported")) {
            assertEquals(
                problem,
                PrintMessage.NeedsReview("Red"),
                message("error", problem, "Red", null),
            )
        }
        assertEquals(PrintMessage.NeedsReview("Red"), message("intervention", "setup", "Red", null))
        // No se sabe si salió: revisar el papel.
        assertEquals(PrintMessage.CheckPaper, message("uncertain", "uncertain", "Red", null))
        assertEquals(PrintMessage.CheckPaper, message("uncertain", null, "Red", null))
        assertEquals(PrintMessage.Cancelled, message("cancelled", null, "Red", null))
        // Sin la clave del problema (no debería pasar), según el estado.
        assertEquals(PrintMessage.NotResponding("Red"), message("error", null, "Red", null))
        assertEquals(PrintMessage.NeedsReview("Red"), message("intervention", null, "Red", null))
    }

    @Test
    fun cadaRechazoDeGoTieneSuAviso() {
        assertEquals(
            PrintMessage.NoPrinter,
            PrecheckFeedback.rejection("PRINTER_NOT_CONFIGURED", 409, "Este dispositivo…"),
        )
        assertEquals(
            PrintMessage.PrinterUnavailable("La impresora Red está desactivada."),
            PrecheckFeedback.rejection(
                "PRINTER_UNAVAILABLE",
                409,
                "La impresora Red está desactivada.",
            ),
        )
        assertEquals(
            PrintMessage.NoPermission,
            PrecheckFeedback.rejection("PERMISSION_DENIED", 403, "Sin permiso"),
        )
        assertEquals(PrintMessage.NoPermission, PrecheckFeedback.rejection(null, 403, null))
        assertEquals(
            PrintMessage.Failed("La cuenta ya se cerró."),
            PrecheckFeedback.rejection("ACCOUNT_NOT_OPEN", 409, "La cuenta ya se cerró."),
        )
        assertEquals(PrintMessage.Failed(null), PrecheckFeedback.rejection(null, 500, null))
        // Los de «Reintentar» que dicen que el trabajo ya no está fallido: volver a leer.
        for (code in listOf("PRINT_NOT_FAILED", "PRINT_UNCERTAIN", "PRINT_JOB_SUPERSEDED")) {
            assertTrue(code, PrecheckFeedback.isStaleRetry(code))
        }
        assertFalse(PrecheckFeedback.isStaleRetry("PRINTER_NOT_CONFIGURED"))
    }

    @Test
    fun reintentarSoloSiNuncaSalioEnPapel() {
        assertTrue(PrecheckFeedback.canRetry("error", "unreachable"))
        assertTrue(PrecheckFeedback.canRetry("error", "not_found"))
        assertTrue(PrecheckFeedback.canRetry("intervention", "setup"))
        // Salió, va en camino o no se sabe: nunca «Reintentar» (volver a imprimir es otra
        // precuenta, con el botón, como en la caja).
        for (state in
            listOf("pending", "processing", "submitted", "uncertain", "cancelled", null)) {
            assertFalse(state, PrecheckFeedback.canRetry(state, null))
        }
        assertFalse(PrecheckFeedback.canRetry("error", "uncertain"))
        // La terminal desconectada la imprime sola al volver.
        assertFalse(PrecheckFeedback.canRetry("error", "agent_offline"))
        // Qué estados ya no se siguen.
        for (state in listOf("submitted", "error", "intervention", "uncertain", "cancelled")) {
            assertTrue(state, PrecheckFeedback.isFinal(state))
        }
        for (state in listOf("pending", "processing", null)) {
            assertFalse(state, PrecheckFeedback.isFinal(state))
        }
    }

    @Test
    fun imprimirEsUnPermisoDeGo() {
        val with = WaiterPermissions(setOf("payments.tickets.print", "accounts.read"))
        assertTrue(with.grants(WaiterAction.PRINT_PRECHECK))
        assertFalse(WaiterPermissions(setOf("accounts.read")).grants(WaiterAction.PRINT_PRECHECK))
        // Con permisos desconocidos no se muestra: imprimir exige conexión y, con ella, se leen.
        assertFalse(WaiterPermissions.UNKNOWN.grants(WaiterAction.PRINT_PRECHECK))
    }

    // --- El seguimiento (PrecheckPrinting) ---

    /** Go simulado: respuestas en orden y lo que se le pidió. */
    private class FakePrecheck : PrecheckService {
        val prints = ArrayDeque<PrintAnswer>()
        val statuses = ArrayDeque<PrintAnswer>()
        val retries = ArrayDeque<PrintAnswer>()
        val printed = mutableListOf<String>()
        val retried = mutableListOf<Pair<String, String>>()
        var statusCalls = 0

        override val setup: StateFlow<PrecheckSetup> = MutableStateFlow(PrecheckSetup.UNKNOWN)
        override val connected: StateFlow<Boolean> = MutableStateFlow(true)

        override suspend fun refreshSetup(userId: String) = Unit

        override suspend fun print(
            userId: String,
            accountId: String,
            operationId: String,
        ): PrintAnswer {
            printed.add(operationId)
            return prints.removeFirstOrNull() ?: PrintAnswer.NoConnection
        }

        override suspend fun status(
            userId: String,
            accountId: String,
            ticketId: String,
        ): PrintAnswer {
            statusCalls++
            return statuses.removeFirstOrNull() ?: PrintAnswer.NoConnection
        }

        override suspend fun retry(
            userId: String,
            accountId: String,
            ticketId: String,
            operationId: String,
        ): PrintAnswer {
            retried.add(ticketId to operationId)
            return retries.removeFirstOrNull() ?: PrintAnswer.NoConnection
        }
    }

    private fun queued(
        problem: String? = null,
        printer: String = "USB Caja 2",
        terminal: String? = "Caja 2",
        ticket: String = "t1",
    ) = PrintAnswer.Queued(ticket, "j1", "pending", problem, printer, terminal)

    private fun status(state: String?, problem: String? = null, terminal: String? = "Caja 2") =
        PrintAnswer.Status(state, problem, "USB Caja 2", terminal)

    private fun TestScope.printing(service: FakePrecheck): PrecheckPrinting {
        var next = 0
        return PrecheckPrinting(service, backgroundScope, newId = { "op-${++next}" })
    }

    @Test
    fun enviadaYSeSigueHastaQueLaImpresoraLaAcepta() = runTest {
        val go = FakePrecheck()
        go.prints.add(queued())
        go.statuses.addAll(listOf(status("pending"), status("submitted")))
        val printing = printing(go)
        printing.print("ana", "c1")
        assertTrue(printing.state.value.busy)
        assertEquals(PrintMessage.Sending, printing.state.value.message)
        runCurrent()
        assertEquals(PrintMessage.SentTo("Caja 2"), printing.state.value.message)
        assertTrue(printing.state.value.following)
        assertFalse(printing.state.value.busy)
        advanceTimeBy(PrecheckPrinting.POLL_MILLIS * 2 + 1)
        // La impresora (o la cola de su terminal) la aceptó: ya no se sigue.
        assertEquals(PrintMessage.SentTo("Caja 2"), printing.state.value.message)
        assertFalse(printing.state.value.following)
        assertFalse(printing.state.value.canRetry)
        advanceUntilIdle()
        assertEquals(2, go.statusCalls)
        assertEquals(listOf("op-1"), go.printed)
    }

    @Test
    fun terminalDesconectadaYSaleCuandoVuelve() = runTest {
        val go = FakePrecheck()
        go.prints.add(queued(problem = "agent_offline"))
        go.statuses.addAll(listOf(status("pending", "agent_offline"), status("submitted")))
        val printing = printing(go)
        printing.print("ana", "c1")
        runCurrent()
        assertEquals(PrintMessage.TerminalOffline("Caja 2"), printing.state.value.message)
        // No hace falta reintentar: el trabajo espera y sale cuando vuelve la terminal.
        assertFalse(printing.state.value.canRetry)
        advanceTimeBy(PrecheckPrinting.POLL_MILLIS + 1)
        assertEquals(PrintMessage.TerminalOffline("Caja 2"), printing.state.value.message)
        advanceTimeBy(PrecheckPrinting.POLL_MILLIS)
        assertEquals(PrintMessage.SentTo("Caja 2"), printing.state.value.message)
        assertFalse(printing.state.value.following)
    }

    @Test
    fun elSeguimientoDuraUnos15SegundosYNoBloquea() = runTest {
        val go = FakePrecheck()
        go.prints.add(queued(problem = "agent_offline"))
        // La terminal nunca vuelve; una consulta sin respuesta no cambia lo que se dice.
        repeat(3) { go.statuses.add(status("pending", "agent_offline")) }
        go.statuses.add(PrintAnswer.NoConnection)
        repeat(10) { go.statuses.add(status("pending", "agent_offline")) }
        val printing = printing(go)
        printing.print("ana", "c1")
        runCurrent()
        // Mientras se sigue, el botón ya se puede volver a tocar (no está ocupado).
        assertFalse(printing.state.value.busy)
        advanceTimeBy(PrecheckPrinting.FOLLOW_MILLIS + PrecheckPrinting.POLL_MILLIS)
        advanceUntilIdle()
        assertEquals(
            (PrecheckPrinting.FOLLOW_MILLIS / PrecheckPrinting.POLL_MILLIS).toInt(),
            go.statusCalls,
        )
        assertFalse(printing.state.value.following)
        assertEquals(PrintMessage.TerminalOffline("Caja 2"), printing.state.value.message)
    }

    @Test
    fun laImpresoraNoRespondeYReintentarSinMotivoLaMismaPrecuenta() = runTest {
        val go = FakePrecheck()
        go.prints.add(queued(printer = "Red", terminal = null, ticket = "t7"))
        go.statuses.add(PrintAnswer.Status("error", "unreachable", "Red", null))
        go.retries.add(queued(printer = "Red", terminal = null, ticket = "t7"))
        go.statuses.add(PrintAnswer.Status("submitted", null, "Red", null))
        val printing = printing(go)
        printing.print("ana", "c1")
        runCurrent()
        assertEquals(PrintMessage.SentTo("Red"), printing.state.value.message)
        advanceTimeBy(PrecheckPrinting.POLL_MILLIS + 1)
        assertEquals(PrintMessage.NotResponding("Red"), printing.state.value.message)
        assertTrue(printing.state.value.canRetry)
        assertFalse(printing.state.value.following)
        printing.retry("ana", "c1")
        runCurrent()
        // La misma precuenta, con una clave nueva y sin motivo.
        assertEquals(listOf("t7" to "op-2"), go.retried)
        assertEquals(PrintMessage.SentTo("Red"), printing.state.value.message)
        assertFalse(printing.state.value.canRetry)
        advanceTimeBy(PrecheckPrinting.POLL_MILLIS + 1)
        assertEquals(PrintMessage.SentTo("Red"), printing.state.value.message)
        assertFalse(printing.state.value.following)
        // Sin nada fallido, «Reintentar» no hace nada.
        printing.retry("ana", "c1")
        runCurrent()
        assertEquals(1, go.retried.size)
    }

    @Test
    fun reintentarQueYaNoEstaFallidoVuelveALeerElEstado() = runTest {
        val go = FakePrecheck()
        go.prints.add(queued(printer = "Red", terminal = null))
        go.statuses.add(PrintAnswer.Status("error", "unreachable", "Red", null))
        go.retries.add(PrintAnswer.Rejected(409, "PRINT_NOT_FAILED", "Ya no está fallido."))
        // Mientras tanto, el trabajador de Go lo reintentó solo y salió.
        go.statuses.add(PrintAnswer.Status("submitted", null, "Red", null))
        val printing = printing(go)
        printing.print("ana", "c1")
        runCurrent()
        advanceTimeBy(PrecheckPrinting.POLL_MILLIS + 1)
        assertTrue(printing.state.value.canRetry)
        printing.retry("ana", "c1")
        runCurrent()
        assertEquals(PrintMessage.SentTo("Red"), printing.state.value.message)
        assertFalse(printing.state.value.canRetry)
    }

    @Test
    fun sinImpresoraSinPermisoYSinConexionNoSeSigue() = runTest {
        val go = FakePrecheck()
        go.prints.add(
            PrintAnswer.Rejected(409, "PRINTER_NOT_CONFIGURED", "Este dispositivo no tiene…"),
        )
        go.prints.add(PrintAnswer.Rejected(403, "PERMISSION_DENIED", "Sin permiso"))
        go.prints.add(PrintAnswer.NoConnection)
        val printing = printing(go)
        printing.print("ana", "c1")
        runCurrent()
        assertEquals(PrintMessage.NoPrinter, printing.state.value.message)
        printing.print("ana", "c1")
        runCurrent()
        assertEquals(PrintMessage.NoPermission, printing.state.value.message)
        printing.print("ana", "c1")
        runCurrent()
        assertEquals(PrintMessage.NoConnection, printing.state.value.message)
        advanceUntilIdle()
        // Nada se siguió ni quedó para después: imprimir nunca entra en la cola sin conexión.
        assertEquals(0, go.statusCalls)
        assertFalse(printing.state.value.following)
        assertFalse(printing.state.value.canRetry)
        assertEquals(3, go.printed.size)
        assertEquals(3, go.printed.toSet().size)
    }

    @Test
    fun unToqueMientrasSePideNoImprimeDosYOtraCuentaOlvidaLoAnterior() = runTest {
        val go = FakePrecheck()
        go.prints.add(queued())
        go.statuses.add(status("pending"))
        val printing = printing(go)
        printing.print("ana", "c1")
        // Segundo toque antes de la respuesta: se ignora.
        printing.print("ana", "c1")
        runCurrent()
        assertEquals(1, go.printed.size)
        assertTrue(printing.state.value.following)
        // La captura pasó a otra cuenta: se olvida y ya no se sigue la anterior.
        printing.reset()
        assertEquals(PrecheckPrintState.IDLE, printing.state.value)
        advanceUntilIdle()
        assertEquals(0, go.statusCalls)
        assertNull(printing.state.value.message)
    }
}
