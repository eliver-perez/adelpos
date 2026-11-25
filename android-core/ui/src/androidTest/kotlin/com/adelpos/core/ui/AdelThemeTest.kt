package com.adelpos.core.ui

import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Surface
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.setValue
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.toPixelMap
import androidx.compose.ui.platform.testTag
import androidx.compose.ui.test.captureToImage
import androidx.compose.ui.test.junit4.v2.createComposeRule
import androidx.compose.ui.test.onNodeWithTag
import androidx.test.ext.junit.runners.AndroidJUnit4
import com.adelpos.core.ui.theme.AdelColors
import com.adelpos.core.ui.theme.AdelTheme
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Rule
import org.junit.Test
import org.junit.runner.RunWith

/**
 * Tema claro y oscuro: los roles de Material salen de los tokens de AD-EL POS y se dibujan con
 * ellos.
 */
@RunWith(AndroidJUnit4::class)
class AdelThemeTest {
    @get:Rule val compose = createComposeRule()

    @Test
    fun temaOscuroYClaroConLosTokensDelEscritorio() {
        var dark by mutableStateOf(true)
        var colors: AdelColors? = null
        var primary = Color.Unspecified
        var background = Color.Unspecified
        compose.setContent {
            AdelTheme(darkTheme = dark) {
                colors = AdelTheme.colors
                primary = MaterialTheme.colorScheme.primary
                background = MaterialTheme.colorScheme.background
                Surface(color = MaterialTheme.colorScheme.background) {
                    Box(Modifier.fillMaxSize().testTag("fondo"))
                }
            }
        }

        compose.waitForIdle()
        val darkColors = checkNotNull(colors)
        assertTrue(darkColors.isDark)
        assertEquals(darkColors.blue, primary)
        assertEquals(darkColors.background, background)
        assertEquals(background, centerPixel())

        dark = false
        compose.waitForIdle()
        val lightColors = checkNotNull(colors)
        assertFalse(lightColors.isDark)
        assertEquals(lightColors.background, background)
        assertEquals(background, centerPixel())
    }

    private fun centerPixel(): Color {
        val image = compose.onNodeWithTag("fondo").captureToImage().toPixelMap()
        return image[image.width / 2, image.height / 2]
    }
}
