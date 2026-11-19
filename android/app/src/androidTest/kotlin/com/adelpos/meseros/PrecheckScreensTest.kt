package com.adelpos.meseros

import androidx.activity.compose.setContent
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.setValue
import androidx.compose.ui.test.assertIsEnabled
import androidx.compose.ui.test.assertIsNotEnabled
import androidx.compose.ui.test.assertTextEquals
import androidx.compose.ui.test.hasTestTag
import androidx.compose.ui.test.junit4.v2.createAndroidComposeRule
import androidx.compose.ui.test.onNodeWithTag
import androidx.compose.ui.test.performClick
import androidx.test.ext.junit.runners.AndroidJUnit4
import com.adelpos.meseros.core.capture.PrecheckPrintBar
import com.adelpos.meseros.core.capture.PrecheckSection
import com.adelpos.meseros.core.capture.TAG_PRECHECK_DESTINATION
import com.adelpos.meseros.core.capture.TAG_PRECHECK_HINT
import com.adelpos.meseros.core.capture.TAG_PRECHECK_MESSAGE
import com.adelpos.meseros.core.capture.TAG_PRECHECK_MESSAGE_PREFIX
import com.adelpos.meseros.core.capture.TAG_PRECHECK_OFFLINE
import com.adelpos.meseros.core.capture.TAG_PRECHECK_PRINT
import com.adelpos.meseros.core.capture.TAG_PRECHECK_RETRY
import com.adelpos.meseros.core.domain.waiter.PrecheckName
import com.adelpos.meseros.core.domain.waiter.PrecheckPrintState
import com.adelpos.meseros.core.domain.waiter.PrecheckPrinter
import com.adelpos.meseros.core.domain.waiter.PrecheckSetup
import com.adelpos.meseros.core.domain.waiter.PrintMessage
import com.adelpos.meseros.core.ui.theme.AdelTheme
import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Rule
import org.junit.Test
import org.junit.runner.RunWith

/**
 * «Imprimir precuenta» desde la cuenta (etapa A2, Fase C, §5) con estados fijos, en tema claro y
 * oscuro: el botón con el nombre visible (Precuenta, Ticket de venta, Cuenta), a dónde va, cada
 * aviso con su texto en español («Enviada a Caja 1», «La Caja 2 está desconectada», «La impresora
 * no responde», «Este dispositivo no tiene impresora asignada»…), «Reintentar» solo si nunca salió
 * en papel y, sin conexión, el botón que no se puede tocar y lo dice.
 */
@RunWith(AndroidJUnit4::class)
class PrecheckScreensTest {
    @get:Rule val compose = createAndroidComposeRule<MainActivity>()

    private var section by mutableStateOf(section(PrecheckSetup.UNKNOWN, PrecheckPrintState.IDLE))
    private var prints = 0
    private var retries = 0

    private fun section(
        setup: PrecheckSetup,
        state: PrecheckPrintState,
        available: Boolean = true,
    ) =
        PrecheckSection(
            setup = setup,
            state = state,
            available = available,
            onPrint = { prints++ },
            onRetry = { retries++ },
        )

    private fun show(dark: Boolean) {
        compose.activity.runOnUiThread {
            compose.activity.setContent {
                AdelTheme(darkTheme = dark) { PrecheckPrintBar(section) }
            }
        }
        compose.waitForIdle()
    }

    private fun set(
        message: PrintMessage?,
        name: PrecheckName = PrecheckName.PRECHECK,
        canRetry: Boolean = false,
        busy: Boolean = false,
        available: Boolean = true,
        printer: PrecheckPrinter? = PrecheckPrinter("USB Caja 2", "Caja 2"),
        read: Boolean = true,
    ) {
        compose.runOnIdle {
            section =
                section(
                    setup = PrecheckSetup(name, printer, read),
                    state =
                        PrecheckPrintState(
                            message = message,
                            busy = busy,
                            canRetry = canRetry,
                            following = false,
                        ),
                    available = available,
                )
        }
        compose.waitForIdle()
    }

    private fun present(tag: String): Boolean =
        compose
            .onAllNodes(hasTestTag(tag), useUnmergedTree = true)
            .fetchSemanticsNodes()
            .isNotEmpty()

    private fun assertMessage(kind: String, text: String, hint: String? = null) {
        assertTrue("aviso $kind", present("$TAG_PRECHECK_MESSAGE_PREFIX$kind"))
        compose.onNodeWithTag(TAG_PRECHECK_MESSAGE, useUnmergedTree = true).assertTextEquals(text)
        if (hint == null) {
            assertTrue("$kind sin segunda línea", !present(TAG_PRECHECK_HINT))
        } else {
            compose.onNodeWithTag(TAG_PRECHECK_HINT, useUnmergedTree = true).assertTextEquals(hint)
        }
    }

    @Test
    fun elBotonDiceElNombreVisibleYADondeVa() {
        for (dark in listOf(false, true)) {
            show(dark)
            for ((name, label) in
                listOf(
                    PrecheckName.PRECHECK to "Imprimir precuenta",
                    PrecheckName.SALES_TICKET to "Imprimir ticket de venta",
                    PrecheckName.BILL to "Imprimir cuenta",
                )) {
                set(message = null, name = name)
                compose.onNodeWithTag(TAG_PRECHECK_PRINT).assertIsEnabled().assertTextEquals(label)
            }
            // A dónde va: la terminal de la impresora USB o la impresora de red.
            compose
                .onNodeWithTag(TAG_PRECHECK_DESTINATION, useUnmergedTree = true)
                .assertTextEquals("Se imprime en Caja 2.")
            set(message = null, printer = PrecheckPrinter("Barra", null))
            compose
                .onNodeWithTag(TAG_PRECHECK_DESTINATION, useUnmergedTree = true)
                .assertTextEquals("Se imprime en Barra.")
            // Sin impresora asignada se avisa antes de tocar (Go manda igual).
            set(message = null, printer = null)
            compose
                .onNodeWithTag(TAG_PRECHECK_DESTINATION, useUnmergedTree = true)
                .assertTextEquals("Este dispositivo no tiene impresora asignada.")
            compose.onNodeWithTag(TAG_PRECHECK_PRINT).assertIsEnabled()
            // Nunca leído: no se dice nada antes de tocar.
            set(message = null, printer = null, read = false)
            assertTrue(!present(TAG_PRECHECK_DESTINATION))
            // Mientras se pide: «Enviando…», sin poder tocar otra vez.
            set(message = PrintMessage.Sending, busy = true)
            compose
                .onNodeWithTag(TAG_PRECHECK_PRINT)
                .assertIsNotEnabled()
                .assertTextEquals("Enviando…")
        }
        set(message = null)
        compose.onNodeWithTag(TAG_PRECHECK_PRINT).performClick()
        compose.waitForIdle()
        assertEquals(1, prints)
    }

    @Test
    fun cadaAvisoConSuTextoYReintentarSoloSiNuncaSalio() {
        for (dark in listOf(false, true)) {
            show(dark)
            set(PrintMessage.SentTo("Caja 1"))
            assertMessage("sent", "Enviada a Caja 1")
            set(PrintMessage.SentTo("Caja 1"), name = PrecheckName.SALES_TICKET)
            assertMessage("sent", "Enviado a Caja 1")
            set(PrintMessage.SentTo(null), name = PrecheckName.BILL)
            assertMessage("sent", "Enviada a la impresora")
            set(PrintMessage.TerminalOffline("Caja 2"))
            assertMessage(
                "terminal-offline",
                "La Caja 2 está desconectada.",
                "Se imprimirá en cuanto esa terminal esté encendida y con AD-EL POS abierto; " +
                    "no hace falta reintentar.",
            )
            set(PrintMessage.TerminalOffline("Mostrador"))
            assertMessage(
                "terminal-offline",
                "La terminal Mostrador está desconectada.",
                "Se imprimirá en cuanto esa terminal esté encendida y con AD-EL POS abierto; " +
                    "no hace falta reintentar.",
            )
            assertTrue(!present(TAG_PRECHECK_RETRY))
            set(PrintMessage.NotResponding("Red"), canRetry = true)
            assertMessage(
                "not-responding",
                "La impresora no responde.",
                "Revisa que esté encendida, con papel y conectada, y toca «Reintentar».",
            )
            assertTrue(present(TAG_PRECHECK_RETRY))
            set(PrintMessage.NeedsReview("Red"), canRetry = true)
            assertMessage("needs-review", "La impresora necesita revisión: avisa al administrador.")
            set(PrintMessage.CheckPaper)
            assertMessage(
                "check-paper",
                "No se sabe si salió en papel. Revisa la impresora antes de imprimir otra vez.",
            )
            // No se sabe si salió: nunca «Reintentar».
            assertTrue(!present(TAG_PRECHECK_RETRY))
            set(PrintMessage.NoPrinter, printer = null)
            assertMessage(
                "no-printer",
                "Este dispositivo no tiene impresora asignada.",
                "Pídele al administrador que la elija en Administración › Sistema › Terminales.",
            )
            set(PrintMessage.PrinterUnavailable("La impresora Red está desactivada."))
            assertMessage(
                "printer-unavailable",
                "La impresora Red está desactivada.",
                "Pídele al administrador que la revise en Administración › Sistema › Terminales.",
            )
            set(PrintMessage.NoPermission)
            assertMessage("no-permission", "No tienes permiso para imprimir.")
            set(PrintMessage.NoConnection)
            assertMessage(
                "no-connection",
                "Para imprimir necesitas conexión con la computadora principal.",
            )
            set(PrintMessage.Failed(null))
            assertMessage("failed", "No se pudo imprimir. Intenta de nuevo.")
            set(PrintMessage.Failed("La cuenta ya se cerró."))
            assertMessage("failed", "La cuenta ya se cerró.")
            set(PrintMessage.Cancelled)
            assertMessage("cancelled", "Se canceló la impresión en la computadora principal.")
        }
        set(PrintMessage.NotResponding("Red"), canRetry = true)
        compose.onNodeWithTag(TAG_PRECHECK_RETRY).performClick()
        compose.waitForIdle()
        assertEquals(1, retries)
    }

    @Test
    fun sinConexionElBotonLoDiceYNoSePuedeTocar() {
        for (dark in listOf(false, true)) {
            show(dark)
            // Aunque lo último fuera «no responde», sin conexión solo se dice cómo imprimir.
            set(PrintMessage.NotResponding("Red"), canRetry = true, available = false)
            compose
                .onNodeWithTag(TAG_PRECHECK_PRINT)
                .assertIsNotEnabled()
                .assertTextEquals("Imprimir precuenta")
            compose
                .onNodeWithTag(TAG_PRECHECK_OFFLINE, useUnmergedTree = true)
                .assertTextEquals("Para imprimir necesitas conexión con la computadora principal.")
            assertTrue(!present(TAG_PRECHECK_RETRY))
            assertTrue(!present(TAG_PRECHECK_MESSAGE))
        }
        compose.onNodeWithTag(TAG_PRECHECK_PRINT).performClick()
        compose.waitForIdle()
        assertEquals(0, prints)
    }
}
