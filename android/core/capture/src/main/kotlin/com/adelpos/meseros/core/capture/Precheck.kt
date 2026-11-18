package com.adelpos.meseros.core.capture

import androidx.compose.foundation.BorderStroke
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Surface
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.testTag
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.unit.dp
import com.adelpos.meseros.core.domain.waiter.PrecheckFeedback
import com.adelpos.meseros.core.domain.waiter.PrecheckName
import com.adelpos.meseros.core.domain.waiter.PrecheckPrintState
import com.adelpos.meseros.core.domain.waiter.PrecheckSetup
import com.adelpos.meseros.core.domain.waiter.PrintMessage
import com.adelpos.meseros.core.ui.components.SecondaryButton
import com.adelpos.meseros.core.ui.theme.AdelTheme

/*
 * «Imprimir precuenta» desde la cuenta de una mesa o un pedido para llevar (etapa A2, Fase C, §5):
 * el botón con el nombre visible, a dónde va y lo que pasó, en palabras del restaurante. Go decide
 * todo (impresora, contenido, permiso y estado); aquí solo se dice.
 */

/**
 * Lo que necesita la sección de la precuenta en la cuenta (null en `OrderPanel`: no se muestra,
 * porque la cuenta todavía no existe en Go o el mesero no tiene `payments.tickets.print`).
 *
 * @param available hay conexión con la computadora principal y el mesero tiene sesión de Go:
 *   imprimir **nunca** entra en la cola sin conexión.
 */
class PrecheckSection(
    val setup: PrecheckSetup,
    val state: PrecheckPrintState,
    val available: Boolean,
    val onPrint: () -> Unit,
    val onRetry: () -> Unit,
)

/**
 * El botón «Imprimir precuenta» («Imprimir ticket de venta», «Imprimir cuenta») con lo que dice Go:
 * «Enviada a Caja 1», «La Caja 2 está desconectada», «La impresora no responde» (con «Reintentar»
 * sin motivo, porque nunca salió en papel), «Este dispositivo no tiene impresora asignada»… Sin
 * conexión el botón no se puede tocar y lo dice. Botón secundario: el principal de la cuenta es
 * «Enviar a cocina».
 */
@Composable
fun PrecheckPrintBar(section: PrecheckSection, modifier: Modifier = Modifier) {
    val state = section.state
    val name = section.setup.name
    Surface(
        shape = MaterialTheme.shapes.medium,
        border = BorderStroke(1.dp, MaterialTheme.colorScheme.outlineVariant),
        modifier = modifier.fillMaxWidth().testTag(TAG_PRECHECK),
    ) {
        Column(
            modifier = Modifier.padding(12.dp),
            verticalArrangement = Arrangement.spacedBy(8.dp),
        ) {
            SecondaryButton(
                text =
                    if (state.busy) {
                        stringResource(R.string.precheck_sending)
                    } else {
                        stringResource(printLabel(name))
                    },
                onClick = section.onPrint,
                enabled = section.available && !state.busy,
                modifier = Modifier.fillMaxWidth().testTag(TAG_PRECHECK_PRINT),
            )
            if (!section.available) {
                // Sin conexión: lo último que se dijo ya no importa; solo cómo se puede imprimir.
                Text(
                    stringResource(R.string.precheck_no_connection),
                    style = MaterialTheme.typography.bodyMedium,
                    color = MaterialTheme.colorScheme.onSurfaceVariant,
                    modifier = Modifier.testTag(TAG_PRECHECK_OFFLINE),
                )
                return@Column
            }
            val message = state.message
            if (message == null || message == PrintMessage.Sending) {
                Destination(section.setup)
            } else {
                MessageBox(message, name)
            }
            if (state.canRetry && !state.busy) {
                SecondaryButton(
                    text = stringResource(R.string.precheck_retry),
                    onClick = section.onRetry,
                    modifier = Modifier.testTag(TAG_PRECHECK_RETRY),
                )
            }
        }
    }
}

/**
 * A dónde va la precuenta según lo último que se leyó de Go; si el dispositivo no tiene impresora,
 * se avisa antes de tocar (la respuesta de Go manda igual).
 */
@Composable
private fun Destination(setup: PrecheckSetup) {
    if (!setup.read) {
        return
    }
    val printer = setup.printer
    Text(
        text =
            if (printer == null) {
                stringResource(R.string.precheck_no_printer_known)
            } else {
                stringResource(R.string.precheck_destination, printer.destination)
            },
        style = MaterialTheme.typography.bodySmall,
        color = MaterialTheme.colorScheme.onSurfaceVariant,
        modifier = Modifier.testTag(TAG_PRECHECK_DESTINATION),
    )
}

@Composable
private fun MessageBox(message: PrintMessage, name: PrecheckName) {
    val colors = AdelTheme.colors
    val tone =
        when (message) {
            is PrintMessage.SentTo -> colors.blueSoft
            is PrintMessage.TerminalOffline,
            PrintMessage.CheckPaper,
            PrintMessage.NoConnection -> colors.amberSoft
            else -> colors.redSoft
        }
    val hint = hintOf(message)
    Surface(
        color = tone,
        shape = MaterialTheme.shapes.medium,
        modifier =
            Modifier.fillMaxWidth().testTag("$TAG_PRECHECK_MESSAGE_PREFIX${kindOf(message)}"),
    ) {
        Column(
            modifier = Modifier.padding(12.dp),
            verticalArrangement = Arrangement.spacedBy(4.dp),
        ) {
            Text(
                textOf(message, name),
                style = MaterialTheme.typography.bodyMedium,
                modifier = Modifier.testTag(TAG_PRECHECK_MESSAGE),
            )
            if (hint != null) {
                Text(
                    hint,
                    style = MaterialTheme.typography.bodySmall,
                    modifier = Modifier.testTag(TAG_PRECHECK_HINT),
                )
            }
        }
    }
}

/** «Imprimir precuenta», «Imprimir ticket de venta» o «Imprimir cuenta». */
fun printLabel(name: PrecheckName): Int =
    when (name) {
        PrecheckName.PRECHECK -> R.string.precheck_print_precheck
        PrecheckName.SALES_TICKET -> R.string.precheck_print_sales_ticket
        PrecheckName.BILL -> R.string.precheck_print_bill
    }

/** La frase principal de cada aviso (con el nombre visible para concordar «enviada/enviado»). */
@Composable
private fun textOf(message: PrintMessage, name: PrecheckName): String =
    when (message) {
        PrintMessage.Sending -> stringResource(R.string.precheck_sending)
        is PrintMessage.SentTo -> sentText(message.destination, name)
        is PrintMessage.TerminalOffline -> offlineText(message.terminal)
        is PrintMessage.NotResponding -> stringResource(R.string.precheck_not_responding)
        is PrintMessage.NeedsReview -> stringResource(R.string.precheck_needs_review)
        PrintMessage.CheckPaper -> stringResource(R.string.precheck_check_paper)
        PrintMessage.Cancelled -> stringResource(R.string.precheck_cancelled)
        PrintMessage.NoPrinter -> stringResource(R.string.precheck_no_printer)
        is PrintMessage.PrinterUnavailable ->
            message.message?.takeIf { it.isNotBlank() }
                ?: stringResource(R.string.precheck_printer_unavailable)
        PrintMessage.NoConnection -> stringResource(R.string.precheck_no_connection)
        PrintMessage.NoPermission -> stringResource(R.string.precheck_no_permission)
        is PrintMessage.Failed ->
            message.message?.takeIf { it.isNotBlank() } ?: stringResource(R.string.precheck_failed)
    }

/** Qué hacer, como segunda línea (null si no hace falta). */
@Composable
private fun hintOf(message: PrintMessage): String? =
    when (message) {
        is PrintMessage.TerminalOffline -> stringResource(R.string.precheck_terminal_offline_hint)
        is PrintMessage.NotResponding -> stringResource(R.string.precheck_not_responding_hint)
        PrintMessage.NoPrinter -> stringResource(R.string.precheck_no_printer_hint)
        is PrintMessage.PrinterUnavailable -> stringResource(R.string.precheck_ask_admin_hint)
        else -> null
    }

@Composable
private fun sentText(destination: String?, name: PrecheckName): String {
    // «Ticket de venta» es masculino: «Enviado a Caja 1».
    val masculine = name == PrecheckName.SALES_TICKET
    return when {
        destination == null && masculine -> stringResource(R.string.precheck_sent_masculine_any)
        destination == null -> stringResource(R.string.precheck_sent_any)
        masculine -> stringResource(R.string.precheck_sent_masculine, destination)
        else -> stringResource(R.string.precheck_sent, destination)
    }
}

@Composable
private fun offlineText(terminal: String?): String {
    val name = terminal?.trim()
    return when {
        name.isNullOrEmpty() -> stringResource(R.string.precheck_terminal_offline_unknown)
        PrecheckFeedback.terminalNamesItself(name) ->
            stringResource(R.string.precheck_terminal_offline, name)
        else -> stringResource(R.string.precheck_terminal_offline_named, name)
    }
}

/** La clase de aviso en la etiqueta de pruebas (`precheck-message-sent`…). */
fun kindOf(message: PrintMessage): String =
    when (message) {
        PrintMessage.Sending -> "sending"
        is PrintMessage.SentTo -> "sent"
        is PrintMessage.TerminalOffline -> "terminal-offline"
        is PrintMessage.NotResponding -> "not-responding"
        is PrintMessage.NeedsReview -> "needs-review"
        PrintMessage.CheckPaper -> "check-paper"
        PrintMessage.Cancelled -> "cancelled"
        PrintMessage.NoPrinter -> "no-printer"
        is PrintMessage.PrinterUnavailable -> "printer-unavailable"
        PrintMessage.NoConnection -> "no-connection"
        PrintMessage.NoPermission -> "no-permission"
        is PrintMessage.Failed -> "failed"
    }

const val TAG_PRECHECK = "precheck"
const val TAG_PRECHECK_PRINT = "precheck-print"
const val TAG_PRECHECK_RETRY = "precheck-retry"
const val TAG_PRECHECK_OFFLINE = "precheck-offline"
const val TAG_PRECHECK_DESTINATION = "precheck-destination"
const val TAG_PRECHECK_MESSAGE = "precheck-message"
const val TAG_PRECHECK_HINT = "precheck-hint"
const val TAG_PRECHECK_MESSAGE_PREFIX = "precheck-message-"
