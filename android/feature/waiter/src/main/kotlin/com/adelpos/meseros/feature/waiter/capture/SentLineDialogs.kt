package com.adelpos.meseros.feature.waiter.capture

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableIntStateOf
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.saveable.rememberSaveable
import androidx.compose.runtime.setValue
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.testTag
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.unit.dp
import com.adelpos.meseros.core.domain.catalog.KitchenNotes
import com.adelpos.meseros.core.domain.catalog.LineSelection
import com.adelpos.meseros.core.domain.waiter.DraftQuote
import com.adelpos.meseros.core.domain.waiter.SentLine
import com.adelpos.meseros.core.ui.catalog.Stepper
import com.adelpos.meseros.core.ui.components.LocalUserActivity
import com.adelpos.meseros.core.ui.components.reportsUserActivity
import com.adelpos.meseros.feature.waiter.R

/**
 * Modificar o cancelar unidades enviadas (§6.3), siempre con motivo (de 5 a 500 caracteres, como
 * exige Go) y sin supervisor (9H). Modificar cambia la cantidad o las observaciones: primero se
 * calcula con Go (`previewEdit`, su huella y su importe) y después se aplica (`editUnits`). Reducir
 * la cantidad exige además el permiso de cancelar; los motivos y los importes los valida Go.
 */
@Composable
internal fun SentLineDialogs(
    dialog: SentDialog?,
    quote: DraftQuote?,
    error: String?,
    working: Boolean,
    canReduce: Boolean,
    onDismiss: () -> Unit,
    onCancelUnits: (SentLine, Int, String) -> Unit,
    onPreviewEdit: (SentLine, LineSelection) -> Unit,
    onEditUnits: (SentLine, LineSelection, String) -> Unit,
) {
    when (dialog) {
        is SentDialog.Cancel -> CancelDialog(dialog.line, error, working, onDismiss, onCancelUnits)
        is SentDialog.Modify ->
            ModifyDialog(
                line = dialog.line,
                quote = quote,
                error = error,
                working = working,
                canReduce = canReduce,
                onDismiss = onDismiss,
                onPreview = onPreviewEdit,
                onEdit = onEditUnits,
            )
        null -> Unit
    }
}

@Composable
private fun CancelDialog(
    line: SentLine,
    error: String?,
    working: Boolean,
    onDismiss: () -> Unit,
    onCancel: (SentLine, Int, String) -> Unit,
) {
    var quantity by rememberSaveable(line.versionId) { mutableIntStateOf(1) }
    var reason by rememberSaveable(line.versionId) { mutableStateOf("") }
    AlertDialog(
        onDismissRequest = onDismiss,
        title = { Text(stringResource(R.string.sent_cancel_title, line.name)) },
        text = {
            Column(verticalArrangement = Arrangement.spacedBy(8.dp)) {
                Text(stringResource(R.string.sent_cancel_quantity))
                Stepper(
                    value = quantity.toString(),
                    onDecrease = { quantity -= 1 },
                    onIncrease = { quantity += 1 },
                    canDecrease = quantity > 1,
                    canIncrease = quantity < line.quantity,
                    decreaseLabel = "−",
                    increaseLabel = "+",
                    testTag = TAG_CANCEL_QUANTITY,
                )
                ReasonField(reason) { reason = it }
                ErrorText(error)
            }
        },
        confirmButton = {
            TextButton(
                onClick = { onCancel(line, quantity, reason) },
                enabled = !working && validReason(reason),
                modifier = Modifier.testTag(TAG_CANCEL_CONFIRM),
            ) {
                Text(stringResource(R.string.sent_cancel_confirm))
            }
        },
        dismissButton = {
            TextButton(onClick = onDismiss) { Text(stringResource(R.string.sent_close)) }
        },
        modifier = Modifier.reportsUserActivity(LocalUserActivity.current),
    )
}

@Composable
private fun ModifyDialog(
    line: SentLine,
    quote: DraftQuote?,
    error: String?,
    working: Boolean,
    canReduce: Boolean,
    onDismiss: () -> Unit,
    onPreview: (SentLine, LineSelection) -> Unit,
    onEdit: (SentLine, LineSelection, String) -> Unit,
) {
    var quantity by rememberSaveable(line.versionId) { mutableIntStateOf(line.quantity) }
    var notes by
        rememberSaveable(line.versionId) { mutableStateOf(line.request.customization.notes) }
    var reason by rememberSaveable(line.versionId) { mutableStateOf("") }
    val request =
        line.request.copy(
            quantity = quantity,
            customization = line.request.customization.copy(notes = notes.trim()),
        )
    val minimum =
        if (canReduce) {
            1
        } else {
            line.quantity
        }
    AlertDialog(
        onDismissRequest = onDismiss,
        title = { Text(stringResource(R.string.sent_modify_title, line.name)) },
        text = {
            Column(verticalArrangement = Arrangement.spacedBy(8.dp)) {
                Stepper(
                    value = quantity.toString(),
                    onDecrease = { quantity -= 1 },
                    onIncrease = { quantity += 1 },
                    canDecrease = quantity > minimum,
                    canIncrease = quantity < LineSelection.MAXIMUM_LINE_QUANTITY,
                    decreaseLabel = "−",
                    increaseLabel = "+",
                    testTag = TAG_MODIFY_QUANTITY,
                )
                OutlinedTextField(
                    value = notes,
                    onValueChange = { text ->
                        // El mismo límite de Go para las observaciones (bytes UTF-8).
                        if (KitchenNotes.fits(text)) {
                            notes = text
                        }
                    },
                    label = { Text(stringResource(R.string.sent_notes)) },
                    modifier = Modifier.fillMaxWidth().testTag(TAG_MODIFY_NOTES),
                )
                if (quote != null) {
                    val total = quote.lines.firstOrNull()?.total ?: quote.combinedTotal
                    Text(
                        stringResource(R.string.sent_modify_quote, total.display()),
                        style = MaterialTheme.typography.bodyMedium,
                        modifier = Modifier.testTag(TAG_MODIFY_QUOTE),
                    )
                    ReasonField(reason) { reason = it }
                }
                ErrorText(error)
            }
        },
        confirmButton = {
            if (quote == null) {
                TextButton(
                    onClick = { onPreview(line, request) },
                    enabled = !working,
                    modifier = Modifier.testTag(TAG_MODIFY_PREVIEW),
                ) {
                    Text(stringResource(R.string.sent_modify_preview))
                }
            } else {
                TextButton(
                    onClick = { onEdit(line, request, reason) },
                    enabled = !working && validReason(reason),
                    modifier = Modifier.testTag(TAG_MODIFY_CONFIRM),
                ) {
                    Text(stringResource(R.string.sent_modify_confirm))
                }
            }
        },
        dismissButton = {
            TextButton(onClick = onDismiss) { Text(stringResource(R.string.sent_close)) }
        },
        modifier = Modifier.reportsUserActivity(LocalUserActivity.current),
    )
}

@Composable
private fun ReasonField(reason: String, onChange: (String) -> Unit) {
    OutlinedTextField(
        value = reason,
        onValueChange = { text ->
            if (text.length <= MAXIMUM_REASON) {
                onChange(text)
            }
        },
        label = { Text(stringResource(R.string.sent_reason)) },
        supportingText = { Text(stringResource(R.string.sent_reason_hint)) },
        modifier = Modifier.fillMaxWidth().testTag(TAG_SENT_REASON),
    )
}

@Composable
private fun ErrorText(error: String?) {
    if (error != null) {
        Text(
            error,
            color = MaterialTheme.colorScheme.error,
            style = MaterialTheme.typography.bodyMedium,
            modifier = Modifier.testTag(TAG_SENT_ERROR),
        )
    }
}

/** El motivo que acepta Go: de 5 a 500 caracteres (se vuelve a validar allá). */
private fun validReason(reason: String): Boolean =
    reason.trim().length in MINIMUM_REASON..MAXIMUM_REASON

private const val MINIMUM_REASON = 5
private const val MAXIMUM_REASON = 500

const val TAG_CANCEL_QUANTITY = "cancel-quantity"
const val TAG_CANCEL_CONFIRM = "cancel-confirm"
const val TAG_MODIFY_QUANTITY = "modify-quantity"
const val TAG_MODIFY_NOTES = "modify-notes"
const val TAG_MODIFY_PREVIEW = "modify-preview"
const val TAG_MODIFY_CONFIRM = "modify-confirm"
const val TAG_MODIFY_QUOTE = "modify-quote"
const val TAG_SENT_REASON = "sent-reason"
const val TAG_SENT_ERROR = "sent-error"
