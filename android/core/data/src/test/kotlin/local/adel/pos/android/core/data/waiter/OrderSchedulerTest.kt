package local.adel.pos.android.core.data.waiter

import android.content.Context
import androidx.test.core.app.ApplicationProvider
import androidx.test.ext.junit.runners.AndroidJUnit4
import androidx.work.Configuration
import androidx.work.WorkInfo
import androidx.work.WorkManager
import androidx.work.testing.SynchronousExecutor
import androidx.work.testing.WorkManagerTestInitHelper
import kotlinx.coroutines.runBlocking
import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Before
import org.junit.Test
import org.junit.runner.RunWith

/**
 * «Borrar los datos de este dispositivo» y WorkManager (revisión R6, M1 y B5): el envío y el
 * periódico se cancelan **esperando** a WorkManager y, al volver a programarlo, el periódico existe
 * otra vez sin reiniciar el proceso.
 */
@RunWith(AndroidJUnit4::class)
class OrderSchedulerTest {
    private val context: Context = ApplicationProvider.getApplicationContext()

    @Before
    fun setUp() {
        WorkManagerTestInitHelper.initializeTestWorkManager(
            context,
            Configuration.Builder().setExecutor(SynchronousExecutor()).build(),
        )
    }

    private fun states(name: String): List<WorkInfo.State> =
        WorkManager.getInstance(context).getWorkInfosForUniqueWork(name).get().map { it.state }

    @Test
    fun alBorrarSeCancelaElEnvioYElPeriodicoVuelveAProgramarse() = runBlocking {
        val scheduler = WorkManagerOrderScheduler(context, OrderSendSignals())
        scheduler.schedulePeriodic()
        scheduler.requestSend()
        assertTrue(states(WorkManagerOrderScheduler.PERIODIC_WORK).contains(WorkInfo.State.ENQUEUED))
        scheduler.cancelAndWait()
        assertEquals(
            listOf(WorkInfo.State.CANCELLED),
            states(WorkManagerOrderScheduler.PERIODIC_WORK).distinct(),
        )
        // Ningún envío queda programado ni corriendo (en la prueba no hay fábrica del
        // trabajador, así que el envío pudo terminar antes por sí solo).
        assertTrue(states(WorkManagerOrderScheduler.SEND_WORK).all { it.isFinished })
        scheduler.schedulePeriodic()
        assertTrue(states(WorkManagerOrderScheduler.PERIODIC_WORK).contains(WorkInfo.State.ENQUEUED))
    }
}
