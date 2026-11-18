package local.adel.pos.android.core.data.waiter

import android.content.Context
import androidx.work.BackoffPolicy
import androidx.work.Constraints
import androidx.work.CoroutineWorker
import androidx.work.ExistingPeriodicWorkPolicy
import androidx.work.ExistingWorkPolicy
import androidx.work.ListenableWorker
import androidx.work.NetworkType
import androidx.work.OneTimeWorkRequest
import androidx.work.OneTimeWorkRequestBuilder
import androidx.work.PeriodicWorkRequest
import androidx.work.PeriodicWorkRequestBuilder
import androidx.work.WorkManager
import androidx.work.WorkRequest
import androidx.work.WorkerFactory
import androidx.work.WorkerParameters
import androidx.work.await
import java.util.concurrent.TimeUnit
import java.util.concurrent.atomic.AtomicBoolean
import java.util.concurrent.atomic.AtomicInteger

/**
 * Trabajador de WorkManager que envía la cola (§7) **aunque la aplicación esté cerrada o en segundo
 * plano**. Es trabajo normal, no urgente: en Android 8–11 un trabajo urgente corre como servicio en
 * primer plano y exige `getForegroundInfo()` (avance §1.6).
 *
 * Sin respuesta del servidor devuelve `retry()` y WorkManager lo repite con **espera creciente**
 * (exponencial desde 10 s). Cada pedido sale con su misma clave de operación: Go nunca lo duplica.
 */
class OrderSendWorker(
    context: Context,
    parameters: WorkerParameters,
    private val sender: () -> OrderSender,
    private val signals: OrderSendSignals,
) : CoroutineWorker(context, parameters) {
    override suspend fun doWork(): Result {
        signals.running.incrementAndGet()
        try {
            var run = sender().sendPending()
            // Llegó otro pedido (o «Reintentar ya») mientras se enviaba: otra vuelta enseguida en
            // lugar de cancelar la que estaba en curso.
            while (!run.retryLater && signals.again.getAndSet(false)) {
                run = sender().sendPending()
            }
            return if (run.retryLater) {
                Result.retry()
            } else {
                Result.success()
            }
        } finally {
            signals.running.decrementAndGet()
        }
    }
}

/** Avisos entre quien pide enviar y el trabajador que ya está enviando (en este proceso). */
class OrderSendSignals {
    val running = AtomicInteger(0)
    val again = AtomicBoolean(false)
}

/**
 * Crea el trabajador con sus dependencias (inyección manual): la aplicación lo pasa a la
 * configuración de WorkManager (`Configuration.Provider`).
 */
class OrderWorkerFactory(
    private val sender: () -> OrderSender,
    private val signals: OrderSendSignals,
) : WorkerFactory() {
    override fun createWorker(
        appContext: Context,
        workerClassName: String,
        workerParameters: WorkerParameters,
    ): ListenableWorker? =
        if (workerClassName == OrderSendWorker::class.java.name) {
            OrderSendWorker(appContext, workerParameters, sender, signals)
        } else {
            null
        }
}

/**
 * Programa el envío de la cola con WorkManager.
 *
 * **Sin restricción de red** (riesgo de la Fase 0, avance §1.6): `NetworkType.CONNECTED` exige una
 * red «validada» (con salida a internet) y la wifi del restaurante puede no tenerla, así que el
 * trabajo nunca saldría. En su lugar, el trabajador prueba el servidor él mismo y, sin respuesta,
 * reintenta con espera creciente; además la aplicación pide un envío inmediato al recuperar la red,
 * al volver a primer plano y cuando un mesero entra con conexión, y un trabajo periódico (cada 15
 * minutos, el mínimo de Android) cubre el caso de la aplicación cerrada mucho tiempo.
 */
class WorkManagerOrderScheduler(
    private val context: Context,
    private val signals: OrderSendSignals,
) : OrderSendScheduler {
    override fun requestSend() {
        if (signals.running.get() > 0) {
            signals.again.set(true)
            return
        }
        WorkManager.getInstance(context)
            .enqueueUniqueWork(SEND_WORK, ExistingWorkPolicy.REPLACE, sendRequest())
    }

    /** El envío periódico de respaldo (se conserva si ya existe). */
    fun schedulePeriodic() {
        WorkManager.getInstance(context)
            .enqueueUniquePeriodicWork(
                PERIODIC_WORK,
                ExistingPeriodicWorkPolicy.KEEP,
                periodicRequest(),
            )
    }

    /**
     * «Borrar los datos de este dispositivo»: cancela el envío y el periódico y **espera** a que
     * WorkManager lo registre (revisión R6, M1). Quien borra espera además a que termine la vuelta
     * en curso (`OrderSender.exclusive`) y, al final, vuelve a programar el periódico (B5).
     */
    suspend fun cancelAndWait() {
        val workManager = WorkManager.getInstance(context)
        workManager.cancelUniqueWork(SEND_WORK).await()
        workManager.cancelUniqueWork(PERIODIC_WORK).await()
    }

    companion object {
        const val SEND_WORK = "adel-orders-send"
        const val PERIODIC_WORK = "adel-orders-periodic"

        /** Sin restricción de red a propósito (ver la clase). */
        val CONSTRAINTS: Constraints =
            Constraints.Builder().setRequiredNetworkType(NetworkType.NOT_REQUIRED).build()

        fun sendRequest(): OneTimeWorkRequest =
            OneTimeWorkRequestBuilder<OrderSendWorker>()
                .setConstraints(CONSTRAINTS)
                .setBackoffCriteria(
                    BackoffPolicy.EXPONENTIAL,
                    WorkRequest.MIN_BACKOFF_MILLIS,
                    TimeUnit.MILLISECONDS,
                )
                .build()

        fun periodicRequest(): PeriodicWorkRequest =
            PeriodicWorkRequestBuilder<OrderSendWorker>(
                    PeriodicWorkRequest.MIN_PERIODIC_INTERVAL_MILLIS,
                    TimeUnit.MILLISECONDS,
                )
                .setConstraints(CONSTRAINTS)
                .build()
    }
}
