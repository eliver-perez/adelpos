package local.adel.pos.android.core.domain.table

import local.adel.pos.android.core.domain.preferences.TablesView
import org.junit.Assert.assertEquals
import org.junit.Assert.assertNull
import org.junit.Test

class TableStateTest {
    @Test
    fun losNombresSonLosDeLaApi() {
        assertEquals(
            listOf("free", "occupied", "awaiting_payment", "partial_payment", "out_of_service"),
            TableState.entries.map { it.wireName },
        )
        assertEquals(TableState.PARTIAL_PAYMENT, TableState.fromWireName("partial_payment"))
        assertNull(TableState.fromWireName("reserved"))
    }

    @Test
    fun laVistaDeMesasPredeterminadaEsLaLista() {
        assertEquals(TablesView.LIST, TablesView.fromStoredName(null))
        assertEquals(TablesView.LIST, TablesView.fromStoredName("otra"))
        assertEquals(TablesView.MAP, TablesView.fromStoredName("map"))
    }
}
