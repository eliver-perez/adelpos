package com.adelpos.meseros.core.capture

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.text.KeyboardOptions
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.saveable.rememberSaveable
import androidx.compose.runtime.setValue
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.testTag
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.text.input.ImeAction
import androidx.compose.ui.text.input.KeyboardCapitalization
import androidx.compose.ui.text.input.KeyboardType
import androidx.compose.ui.unit.dp
import com.adelpos.core.ui.components.LocalUserActivity
import com.adelpos.core.ui.components.reportsUserActivity
import com.adelpos.meseros.core.domain.waiter.OrderDestination
import com.adelpos.meseros.core.domain.waiter.TakeawayCustomer

/*
 * Lo que comparten la captura y la lista de Para llevar (etapa A2): el diálogo «Nombre y
 * teléfono» (A2-D1) y cómo se nombra un pedido para llevar en las pantallas de la cola.
 */

/**
 * «Nombre y teléfono» de un pedido para llevar (A2-D1): los dos opcionales y sin formato; solo se
 * impiden los textos que no caben en los límites de Go (100 y 40, en bytes del texto recortado).
 * Vacío quita el dato.
 */
@Composable
fun CustomerDialog(
    initial: TakeawayCustomer,
    onSave: (TakeawayCustomer) -> Unit,
    onDismiss: () -> Unit,
) {
    var name by rememberSaveable { mutableStateOf(initial.name) }
    var phone by rememberSaveable { mutableStateOf(initial.phone) }
    // Etapa M1 (§5.7): en Android 8 el teclado se quedaba abierto al cerrar el diálogo y tapaba la
    // lista de Para llevar. Se suelta el foco y se esconde el teclado al cerrar y, ya sin el
    // diálogo, también desde la ventana de la actividad (HideKeyboardWhenClosed).
    HideKeyboardWhenClosed()
    val close = rememberKeyboardCloser()
    AlertDialog(
        onDismissRequest = { close(onDismiss) },
        title = { Text(stringResource(R.string.customer_title)) },
        text = {
            Column(verticalArrangement = Arrangement.spacedBy(12.dp)) {
                OutlinedTextField(
                    value = name,
                    onValueChange = { text ->
                        if (TakeawayCustomer.fitsName(text)) {
                            name = text
                        }
                    },
                    label = { Text(stringResource(R.string.customer_name)) },
                    supportingText = { Text(stringResource(R.string.customer_optional)) },
                    singleLine = true,
                    keyboardOptions =
                        KeyboardOptions(
                            capitalization = KeyboardCapitalization.Words,
                            imeAction = ImeAction.Next,
                        ),
                    modifier = Modifier.fillMaxWidth().testTag(TAG_CUSTOMER_NAME),
                )
                OutlinedTextField(
                    value = phone,
                    onValueChange = { text ->
                        if (TakeawayCustomer.fitsPhone(text)) {
                            phone = text
                        }
                    },
                    label = { Text(stringResource(R.string.customer_phone)) },
                    supportingText = { Text(stringResource(R.string.customer_optional)) },
                    singleLine = true,
                    keyboardOptions =
                        KeyboardOptions(
                            keyboardType = KeyboardType.Phone,
                            imeAction = ImeAction.Done,
                        ),
                    modifier = Modifier.fillMaxWidth().testTag(TAG_CUSTOMER_PHONE),
                )
            }
        },
        confirmButton = {
            TextButton(
                onClick = { close { onSave(TakeawayCustomer(name, phone).trimmed()) } },
                modifier = Modifier.testTag(TAG_CUSTOMER_SAVE),
            ) {
                Text(stringResource(R.string.customer_save))
            }
        },
        dismissButton = {
            TextButton(onClick = { close(onDismiss) }) {
                Text(stringResource(R.string.capture_cancel))
            }
        },
        modifier = Modifier.reportsUserActivity(LocalUserActivity.current),
    )
}

/** El nombre y el teléfono en una línea («Juan · 555 1234»), o el texto de «sin datos». */
@Composable
fun customerSummary(customer: TakeawayCustomer): String {
    val parts = listOf(customer.name.trim(), customer.phone.trim()).filter { it.isNotEmpty() }
    if (parts.isEmpty()) {
        return stringResource(R.string.customer_none)
    }
    return parts.joinToString(" · ")
}

/**
 * Cómo se nombra el destino de un pedido de la cola: la mesa o, en un pedido para llevar, «Para
 * llevar» con el nombre del cliente si lo hay.
 */
@Composable
fun destinationLabel(destination: OrderDestination): String =
    if (destination.takeaway) {
        takeawayLabel(destination.customer)
    } else {
        destination.tableName
    }

/** «Para llevar» o «Para llevar · Juan». */
@Composable
fun takeawayLabel(customer: TakeawayCustomer): String {
    val name = customer.name.trim()
    return if (name.isEmpty()) {
        stringResource(R.string.takeaway_label)
    } else {
        stringResource(R.string.takeaway_label_named, name)
    }
}

const val TAG_CUSTOMER_NAME = "customer-name"
const val TAG_CUSTOMER_PHONE = "customer-phone"
const val TAG_CUSTOMER_SAVE = "customer-save"
