package com.adelpos.meseros

import android.content.Context
import android.net.ConnectivityManager
import android.net.NetworkCapabilities
import android.os.Build
import android.os.ParcelFileDescriptor
import android.util.Log
import androidx.test.ext.junit.runners.AndroidJUnit4
import androidx.test.platform.app.InstrumentationRegistry
import androidx.work.Constraints
import androidx.work.ExistingWorkPolicy
import androidx.work.NetworkType
import androidx.work.OneTimeWorkRequestBuilder
import androidx.work.WorkInfo
import androidx.work.WorkManager
import androidx.work.Worker
import androidx.work.WorkerParameters
import com.adelpos.meseros.core.data.waiter.WorkManagerOrderScheduler
import java.util.UUID
import org.junit.After
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assume.assumeTrue
import org.junit.Test
import org.junit.runner.RunWith

/**
 * Riesgo de la Fase 0 (avance §1.6), comprobado en el dispositivo: la wifi del restaurante puede no
 * tener salida a internet y Android no la «valida». Se fuerza esa situación haciendo que la
 * comprobación de internet del sistema falle (sus direcciones apuntan a un puerto cerrado de la
 * propia tableta) y se mira qué trabajos de WorkManager corren:
 * - uno con `NetworkType.CONNECTED` (lo habitual);
 * - uno sin restricción de red, como el de la cola (`WorkManagerOrderScheduler.CONSTRAINTS`).
 *
 * Solo Android 11+ (la red se corta y se devuelve con el modo avión desde la prueba). Al terminar
 * se devuelven los ajustes del sistema y la red.
 */
@RunWith(AndroidJUnit4::class)
class WorkNetworkOnDeviceTest {
    private val context: Context = InstrumentationRegistry.getInstrumentation().targetContext

    class FlagWorker(context: Context, parameters: WorkerParameters) : Worker(context, parameters) {
        override fun doWork(): Result = Result.success()
    }

    private fun shell(command: String): String {
        val output =
            InstrumentationRegistry.getInstrumentation().uiAutomation.executeShellCommand(command)
        ParcelFileDescriptor.AutoCloseInputStream(output).use {
            return it.readBytes().toString(Charsets.UTF_8)
        }
    }

    private fun validated(): Boolean? {
        val manager = context.getSystemService(ConnectivityManager::class.java)
        val network = manager.activeNetwork ?: return null
        val capabilities = manager.getNetworkCapabilities(network) ?: return null
        return capabilities.hasCapability(NetworkCapabilities.NET_CAPABILITY_VALIDATED)
    }

    private fun reconnect() {
        shell("cmd connectivity airplane-mode enable")
        Thread.sleep(SETTLE_MILLIS)
        shell("cmd connectivity airplane-mode disable")
        // Espera a que vuelva una red (validada o no).
        val deadline = System.currentTimeMillis() + RECONNECT_TIMEOUT_MILLIS
        while (validated() == null && System.currentTimeMillis() < deadline) {
            Thread.sleep(POLL_MILLIS)
        }
        Thread.sleep(SETTLE_MILLIS)
    }

    @After
    fun restore() {
        if (Build.VERSION.SDK_INT < Build.VERSION_CODES.R) {
            return
        }
        for (key in PROBE_KEYS) {
            shell("settings delete global $key")
        }
        reconnect()
    }

    @Test
    fun conRedSinInternetSoloCorreElTrabajoSinRestriccion() {
        assumeTrue(
            "Modo avión por shell: Android 11+",
            Build.VERSION.SDK_INT >= Build.VERSION_CODES.R,
        )
        for (key in PROBE_KEYS) {
            shell("settings put global $key $CLOSED_URL")
        }
        reconnect()
        val validatedNow = validated()
        Log.i(TAG, "red activa validada: $validatedNow")
        assumeTrue("La red no quedó sin validar (no se pudo simular)", validatedNow == false)

        val workManager = WorkManager.getInstance(context)
        val connected =
            OneTimeWorkRequestBuilder<FlagWorker>()
                .setConstraints(
                    Constraints.Builder().setRequiredNetworkType(NetworkType.CONNECTED).build(),
                )
                .build()
        val unrestricted =
            OneTimeWorkRequestBuilder<FlagWorker>()
                .setConstraints(WorkManagerOrderScheduler.CONSTRAINTS)
                .build()
        val name = UUID.randomUUID().toString()
        workManager.enqueueUniqueWork("$name-conectado", ExistingWorkPolicy.REPLACE, connected)
        workManager.enqueueUniqueWork("$name-libre", ExistingWorkPolicy.REPLACE, unrestricted)
        val deadline = System.currentTimeMillis() + WAIT_MILLIS
        while (
            System.currentTimeMillis() < deadline &&
                workManager.getWorkInfoById(unrestricted.id).get()?.state !=
                    WorkInfo.State.SUCCEEDED
        ) {
            Thread.sleep(POLL_MILLIS)
        }
        // Un poco más para darle al de CONNECTED la misma oportunidad.
        Thread.sleep(EXTRA_WAIT_MILLIS)
        val connectedState = workManager.getWorkInfoById(connected.id).get()?.state
        val unrestrictedState = workManager.getWorkInfoById(unrestricted.id).get()?.state
        Log.i(TAG, "CONNECTED: $connectedState; sin restricción: $unrestrictedState")
        assertEquals(WorkInfo.State.SUCCEEDED, unrestrictedState)
        // El riesgo es real: con CONNECTED el trabajo espera una red validada.
        assertFalse(connectedState == WorkInfo.State.SUCCEEDED)
        workManager.cancelWorkById(connected.id)
    }

    private companion object {
        const val TAG = "AdelWorkNetwork"
        const val CLOSED_URL = "http://127.0.0.1:9/generate_204"
        val PROBE_KEYS =
            listOf(
                "captive_portal_http_url",
                "captive_portal_https_url",
                "captive_portal_fallback_url",
                "captive_portal_other_fallback_urls",
            )
        const val SETTLE_MILLIS = 4_000L
        const val RECONNECT_TIMEOUT_MILLIS = 30_000L
        const val POLL_MILLIS = 250L
        const val WAIT_MILLIS = 30_000L
        const val EXTRA_WAIT_MILLIS = 10_000L
    }
}
