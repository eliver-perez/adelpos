package local.adel.pos.android.feature.waiter

import androidx.lifecycle.SavedStateHandle
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test

/**
 * Lo capturado en Mesas se guarda en SavedStateHandle y se restaura en un ViewModel nuevo (proceso
 * recreado).
 */
class TablesViewModelTest {
    @Test
    fun guardaLoCapturadoEnElEstadoGuardado() {
        val savedState = SavedStateHandle()
        val viewModel = TablesViewModel(savedState)
        assertEquals("", viewModel.search)
        assertFalse(viewModel.showingAccount)

        viewModel.onSearchChange("Terraza 4")
        viewModel.onShowingAccountChange(true)

        assertEquals("Terraza 4", viewModel.search)
        assertEquals("Terraza 4", savedState.get<String>(TablesViewModel.KEY_SEARCH))
        assertEquals(true, savedState.get<Boolean>(TablesViewModel.KEY_SHOWING_ACCOUNT))
    }

    @Test
    fun restauraLoCapturadoDespuesDeRecrearElProceso() {
        val restored =
            TablesViewModel(
                SavedStateHandle(
                    mapOf(
                        TablesViewModel.KEY_SEARCH to "Barra 2",
                        TablesViewModel.KEY_SHOWING_ACCOUNT to true,
                    ),
                ),
            )
        assertEquals("Barra 2", restored.search)
        assertTrue(restored.showingAccount)
    }
}
