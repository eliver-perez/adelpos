package com.adelpos.meseros.core.data.ready

import android.content.Context
import androidx.room.Room
import androidx.test.core.app.ApplicationProvider
import androidx.test.ext.junit.runners.AndroidJUnit4
import com.adelpos.meseros.core.data.db.AppDatabase
import com.adelpos.meseros.core.domain.ready.ReadyAlert
import com.adelpos.meseros.core.domain.ready.ReadyAlertSettings
import com.adelpos.meseros.core.domain.ready.ReadyAlertsBatch
import com.adelpos.meseros.core.domain.ready.ReadyItem
import com.adelpos.meseros.core.domain.waiter.OrderMode
import kotlinx.coroutines.runBlocking
import org.junit.After
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Before
import org.junit.Test
import org.junit.runner.RunWith

/**
 * El registro del aviso en Room (`AppDatabase` v6) con Robolectric: cursor y estado en una sola
 * fila, avisos ya mostrados (por la `key` de Go), día comercial que cambia, consulta previa sin
 * mover el cursor y borrado del dispositivo.
 */
@RunWith(AndroidJUnit4::class)
class RoomReadyAlertsLedgerTest {
    private val context: Context = ApplicationProvider.getApplicationContext()
    private lateinit var database: AppDatabase
    private lateinit var ledger: RoomReadyAlertsLedger

    @Before
    fun open() {
        database =
            Room.inMemoryDatabaseBuilder(context, AppDatabase::class.java)
                .allowMainThreadQueries()
                .build()
        ledger = RoomReadyAlertsLedger(database.readyAlerts(), clock = { 1_000L })
    }

    @After
    fun close() {
        database.close()
    }

    private fun alert(id: Long, key: String = "k-$id") =
        ReadyAlert(
            id = id,
            key = key,
            accountId = "cuenta-$id",
            mode = OrderMode.DINE_IN,
            tableName = "Mesa $id",
            folio = id,
            customerName = "",
            waiterUserId = "ana",
            waiterShortName = "Ana",
            items = listOf(ReadyItem("Filete", 1)),
            readyAt = "2026-11-18T20:00:0${id}Z",
        )

    private fun batch(
        cursor: Long,
        dayEnds: Long = TODAY_ENDS,
        active: Boolean = true,
        settings: ReadyAlertSettings = ReadyAlertSettings.DEFAULT,
    ) = ReadyAlertsBatch(cursor, active, dayEnds, settings, emptyList())

    @Test
    fun guardaElCursorYLosAvisosMostradosEnUnaTransaccion() = runBlocking {
        assertNull("nunca se preguntó", ledger.status())
        assertEquals(2, ledger.unseen(listOf(alert(1), alert(2))).size)

        ledger.commit(batch(2), listOf(alert(1), alert(2)))

        val status = ledger.status()!!
        assertEquals(2L, status.cursor)
        assertTrue(status.active)
        assertEquals(TODAY_ENDS, status.dayEndsAtMillis)
        assertTrue(ledger.unseen(listOf(alert(1), alert(2))).isEmpty())
        assertEquals(listOf(alert(3)), ledger.unseen(listOf(alert(2), alert(3))))
        // El mismo número con otra key es otro aviso (respaldo restaurado en la principal); la
        // misma key con otro número es el mismo.
        assertEquals(1, ledger.unseen(listOf(alert(2, key = "k-otro"))).size)
        assertTrue(ledger.unseen(listOf(alert(7, key = "k-1"))).isEmpty())
    }

    @Test
    fun alCambiarDeDiaOlvidaLosAvisosDeDiasAnteriores() = runBlocking {
        ledger.commit(batch(5), listOf(alert(4), alert(5)))
        // Mismo día: se conservan.
        ledger.commit(batch(6), listOf(alert(6)))
        assertEquals(listOf(4L, 5L, 6L), ledger.seenIds())
        // Día siguiente: se borran los de ayer y el cursor se conserva.
        ledger.commit(batch(8, dayEnds = TOMORROW_ENDS), listOf(alert(8)))
        assertEquals(listOf(8L), ledger.seenIds())
        assertEquals(8L, ledger.status()?.cursor)
    }

    @Test
    fun laConsultaPreviaGuardaElEstadoSinMoverElCursor() = runBlocking {
        ledger.rememberStatus(batch(30))
        assertEquals("sin cursor guardado, desde el principio del día", 0L, ledger.status()?.cursor)
        ledger.commit(batch(12), emptyList())
        ledger.rememberStatus(
            batch(30, active = false, settings = ReadyAlertSettings(false, true, false)),
        )
        val status = ledger.status()!!
        assertEquals(12L, status.cursor)
        assertFalse(status.active)
        assertEquals(ReadyAlertSettings(false, true, false), status.settings)
    }

    @Test
    fun borrarLosDatosDelDispositivoQuitaTodo() = runBlocking {
        ledger.commit(batch(3), listOf(alert(3)))
        ledger.erase()
        assertNull(ledger.status())
        assertTrue(ledger.seenIds().isEmpty())
        assertEquals(1, ledger.unseen(listOf(alert(3))).size)
    }

    private companion object {
        const val TODAY_ENDS = 1_763_550_000_000L
        const val TOMORROW_ENDS = TODAY_ENDS + 86_400_000L
    }
}
