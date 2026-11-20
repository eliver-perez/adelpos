package com.adelpos.meseros.core.domain.ready

import com.adelpos.meseros.core.domain.waiter.OrderMode
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test

/**
 * Etapa A3 (Fase E, `alerts r5` en Android 13): el resumen del grupo se decide con lo que se
 * publicó y se quitó, aunque la lista de Android todavía no lo refleje (`notify` es asíncrono).
 */
class ReadyShownNoticesTest {
    private var now = 0L
    private val shown = ReadyShownNotices(clock = { now })

    private fun notice(account: String, vararg shownKeys: String) =
        ReadyNotice(
            accountId = account,
            mode = OrderMode.DINE_IN,
            tableName = "Mesa $account",
            folio = 1,
            customerName = "",
            waiterUserId = "ana",
            items = listOf(ReadyItem("Filete", 1)),
            shown = shownKeys.toList(),
        )

    @Test
    fun conLaListaAtrasadaLaSegundaCuentaYaCuentaParaElResumen() {
        val mesa2 = notice("mesa-2", "k1")
        val mesa3 = notice("mesa-3", "k2")
        // Android ya lista la Mesa 2; la Mesa 3 se acaba de publicar y todavía no aparece.
        shown.posted(mesa3)
        val visible = shown.visible(mapOf("mesa-2" to mesa2))
        assertEquals(setOf("mesa-2", "mesa-3"), visible.keys)
        assertTrue(ReadyShownNotices.needsSummary(visible))
        // Solo con la lista de Android (lo de antes) no había resumen.
        assertFalse(ReadyShownNotices.needsSummary(mapOf("mesa-2" to mesa2)))
    }

    @Test
    fun loPublicadoSeOlvidaCuandoAndroidLoListaOPasaElPlazo() {
        val mesa3 = notice("mesa-3", "k2")
        shown.posted(mesa3)
        // Android ya lo lista: manda su lista (si después lo quita el mesero, ya no se cuenta).
        assertEquals(setOf("mesa-3"), shown.visible(mapOf("mesa-3" to mesa3)).keys)
        assertTrue(shown.visible(emptyMap()).isEmpty())
        // Publicado y nunca listado (notificaciones apagadas, descartada al instante): pasado el
        // plazo ya no cuenta.
        shown.posted(notice("mesa-5", "k5"))
        assertEquals(setOf("mesa-5"), shown.visible(emptyMap()).keys)
        now += ReadyShownNotices.LAG_MILLIS + 1
        assertTrue(shown.visible(emptyMap()).isEmpty())
    }

    @Test
    fun elAcumuladoRecienPublicadoReemplazaAlDeLaLista() {
        // La Mesa 2 se reemplazó con el acumulado; la lista todavía tiene el aviso anterior.
        shown.posted(notice("mesa-2", "k1", "k3"))
        val visible = shown.visible(mapOf("mesa-2" to notice("mesa-2", "k1")))
        assertEquals(listOf("k1", "k3"), visible.getValue("mesa-2").shown)
    }

    @Test
    fun loQuitadoNoCuentaAunqueLaListaTodaviaLoTenga() {
        val mesa2 = notice("mesa-2", "k1")
        val mesa3 = notice("mesa-3", "k2")
        val listed = mapOf("mesa-2" to mesa2, "mesa-3" to mesa3)
        shown.removed("mesa-3")
        val visible = shown.visible(listed)
        assertEquals(setOf("mesa-2"), visible.keys)
        assertFalse(
            "con una sola cuenta se quita el resumen",
            ReadyShownNotices.needsSummary(visible),
        )
        // Cuando Android ya no la lista, lo recordado se olvida.
        assertEquals(setOf("mesa-2"), shown.visible(mapOf("mesa-2" to mesa2)).keys)
        assertEquals(setOf("mesa-2", "mesa-3"), shown.visible(listed).keys)
    }

    @Test
    fun alQuitarTodoNoQuedaNadaRecordado() {
        shown.posted(notice("mesa-2", "k1"))
        shown.removed("mesa-3")
        shown.cleared()
        assertTrue(shown.visible(emptyMap()).isEmpty())
    }
}
