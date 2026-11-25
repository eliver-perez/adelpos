package com.adelpos.meseros

import android.os.Build
import android.os.ParcelFileDescriptor
import androidx.compose.ui.test.assertTextEquals
import androidx.compose.ui.test.hasText
import androidx.compose.ui.test.junit4.v2.createAndroidComposeRule
import androidx.compose.ui.test.onNodeWithTag
import androidx.test.ext.junit.runners.AndroidJUnit4
import androidx.test.platform.app.InstrumentationRegistry
import com.adelpos.core.ui.components.TAG_CONNECTION_STATE
import org.junit.After
import org.junit.Assume.assumeTrue
import org.junit.Rule
import org.junit.Test
import org.junit.runner.RunWith

/**
 * El indicador sigue a la red real del dispositivo: «Sin conexión» al activar el modo avión y «En
 * línea» al quitarlo. `cmd connectivity airplane-mode` existe desde Android 11 (API 30); en Android
 * 8 la orden no hace nada como `shell` (avance §1.4), así que ahí la prueba no aplica y el
 * indicador se prueba con estados forzados (core/ui).
 */
@RunWith(AndroidJUnit4::class)
class ConnectionOnDeviceTest {
    @get:Rule val compose = createAndroidComposeRule<MainActivity>()

    private fun shell(command: String) {
        val output =
            InstrumentationRegistry.getInstrumentation().uiAutomation.executeShellCommand(command)
        ParcelFileDescriptor.AutoCloseInputStream(output).use { it.readBytes() }
    }

    @After
    fun restoreNetwork() {
        if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.R) {
            shell("cmd connectivity airplane-mode disable")
        }
    }

    @Test
    fun modoAvionCambiaElIndicador() {
        assumeTrue(
            "cmd connectivity airplane-mode requiere Android 11",
            Build.VERSION.SDK_INT >= Build.VERSION_CODES.R,
        )
        // Sin servidor emparejado el estado único de conexión sigue a la red de Android (etapa A3);
        // con uno que no responde diría «Sin conexión» con razón.
        TestDevice.reset()
        val state = compose.onNodeWithTag(TAG_CONNECTION_STATE, useUnmergedTree = true)
        compose.waitUntil(timeoutMillis = 15_000) {
            compose
                .onAllNodes(hasText("En línea"), useUnmergedTree = true)
                .fetchSemanticsNodes()
                .isNotEmpty()
        }
        state.assertTextEquals("En línea")

        shell("cmd connectivity airplane-mode enable")
        compose.waitUntil(timeoutMillis = 15_000) {
            compose
                .onAllNodes(hasText("Sin conexión"), useUnmergedTree = true)
                .fetchSemanticsNodes()
                .isNotEmpty()
        }
        state.assertTextEquals("Sin conexión")

        shell("cmd connectivity airplane-mode disable")
        compose.waitUntil(timeoutMillis = 30_000) {
            compose
                .onAllNodes(hasText("En línea"), useUnmergedTree = true)
                .fetchSemanticsNodes()
                .isNotEmpty()
        }
        state.assertTextEquals("En línea")
    }
}
