package local.adel.pos.android

import androidx.compose.ui.test.assertIsDisplayed
import androidx.compose.ui.test.hasAnyAncestor
import androidx.compose.ui.test.hasTestTag
import androidx.compose.ui.test.hasText
import androidx.compose.ui.test.junit4.v2.createAndroidComposeRule
import androidx.compose.ui.test.onNodeWithTag
import androidx.compose.ui.test.performClick
import androidx.test.ext.junit.runners.AndroidJUnit4
import local.adel.pos.android.core.ui.components.TAG_CONNECTION_INDICATOR
import local.adel.pos.android.core.ui.components.TAG_TOP_BAR
import local.adel.pos.android.feature.access.TAG_ACCESS_ENTER
import local.adel.pos.android.feature.access.TAG_ACCESS_SCREEN
import local.adel.pos.android.feature.waiter.TAG_TABLES_SEARCH
import org.junit.Rule
import org.junit.Test
import org.junit.runner.RunWith

/**
 * Navegación de muestra: Entrar → Mesas y Atrás, con la barra superior y el indicador en todas las
 * pantallas.
 */
@RunWith(AndroidJUnit4::class)
class AppNavigationTest {
    @get:Rule val compose = createAndroidComposeRule<MainActivity>()

    private fun assertTitle(title: String) {
        compose
            .onNode(hasText(title).and(hasAnyAncestor(hasTestTag(TAG_TOP_BAR))))
            .assertIsDisplayed()
        compose.onNodeWithTag(TAG_CONNECTION_INDICATOR).assertIsDisplayed()
    }

    @Test
    fun entrarLlevaAMesasYAtrasRegresa() {
        compose.onNodeWithTag(TAG_ACCESS_SCREEN).assertIsDisplayed()
        assertTitle("Entrar")

        compose.onNodeWithTag(TAG_ACCESS_ENTER).performClick()
        compose.onNodeWithTag(TAG_TABLES_SEARCH).assertIsDisplayed()
        assertTitle("Mesas")

        compose.runOnUiThread { compose.activity.onBackPressedDispatcher.onBackPressed() }
        compose.onNodeWithTag(TAG_ACCESS_SCREEN).assertIsDisplayed()
        assertTitle("Entrar")
    }
}
