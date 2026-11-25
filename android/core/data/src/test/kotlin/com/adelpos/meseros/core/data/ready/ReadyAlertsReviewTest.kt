package com.adelpos.meseros.core.data.ready

import com.adelpos.core.model.error.AppError
import com.adelpos.core.model.realtime.ReconnectBackoff
import com.adelpos.core.network.session.AuthorizedServer
import com.adelpos.meseros.core.domain.ready.ReadyAlert
import com.adelpos.meseros.core.domain.ready.ReadyAlertPresenter
import com.adelpos.meseros.core.domain.ready.ReadyAlertSettings
import com.adelpos.meseros.core.domain.ready.ReadyAlertsBatch
import com.adelpos.meseros.core.domain.ready.ReadyAlertsLedger
import com.adelpos.meseros.core.domain.ready.ReadyAlertsLifecycle
import com.adelpos.meseros.core.domain.ready.ReadyAlertsProcessor
import com.adelpos.meseros.core.domain.ready.ReadyAlertsStatus
import java.util.concurrent.atomic.AtomicInteger
import kotlinx.coroutines.runBlocking
import kotlinx.coroutines.withTimeoutOrNull
import mockwebserver3.MockWebServer
import okhttp3.Headers
import okhttp3.HttpUrl
import okhttp3.OkHttpClient
import org.junit.Assert.assertNotNull
import org.junit.Test

/**
 * Revisión independiente del aviso de platillos listos (§4 de la etapa A2, parte del dispositivo):
 * reproducción de un hallazgo de `$SP/doc-rev.md`. Comprueba lo que pide la especificación, así que
 * **falla** con el código actual (el mensaje dice lo que pasó). No corrige nada.
 *
 * RD1: pasado `day_ends_at` (y su margen), si la computadora principal no responde, el canal sigue
 * reintentando para siempre con la notificación permanente: §4.3 pide que el servicio se detenga al
 * cerrar el día comercial, y sin Go nadie puede confirmar que siga activo.
 */
class ReadyAlertsReviewTest {
    @Test
    fun pasadoElFinDelDiaSinComputadoraPrincipalElCanalNoSeDetiene() = runBlocking {
        // Una dirección donde ya no escucha nadie: conexión rechazada (la Mac apagada al cerrar).
        val gone = MockWebServer()
        gone.start()
        val url = gone.url("/")
        gone.close()
        val server = UnreachableServer(url)
        val dayEnds = 1_763_550_000_000L
        // Lo último que dijo Go: activo, con el día que terminó hace una hora.
        val ledger = MemoryLedger(ReadyAlertsStatus(77, true, dayEnds, ReadyAlertSettings.DEFAULT))
        val now = dayEnds + ReadyAlertsLifecycle.DAY_CHECK_AFTER_MILLIS + 3_600_000L
        val channel =
            ReadyAlertsChannel(
                server = server,
                ledger = ledger,
                processor = ReadyAlertsProcessor(ledger, NoPresenter),
                client = ReadyAlertsClient(readTimeoutMillis = 300),
                clock = { now },
                // La espera creciente de la aplicación (2 a 30 s) comprimida: 20 a 160 ms.
                backoff = ReconnectBackoff(initialMillis = 20, maximumMillis = 160),
                healthyConnectionMillis = 100,
            )

        val reason = withTimeoutOrNull(3_000) { channel.run() }

        assertNotNull(
            "una hora después del fin del día comercial y sin respuesta de la computadora " +
                "principal, el canal (y con él el servicio en primer plano y su notificación) " +
                "seguía reintentando a los 3 s: ${server.attempts.get()} intentos, " +
                "${server.relocations.get()} búsquedas de otra dirección, sin detenerse",
            reason,
        )
    }

    private class UnreachableServer(private val url: HttpUrl) : ReadyAlertsServer {
        val attempts = AtomicInteger(0)
        val relocations = AtomicInteger(0)

        override val ready: Boolean = true

        override suspend fun prepare() = Unit

        override fun connection(): AuthorizedServer {
            attempts.incrementAndGet()
            return AuthorizedServer(
                baseUrl = url,
                client = OkHttpClient(),
                credentials = Headers.headersOf("X-Device-ID", "d-1", "X-Device-Token", "token"),
            )
        }

        override fun report(error: AppError) = Unit

        override suspend fun relocate() {
            relocations.incrementAndGet()
        }
    }

    private class MemoryLedger(@Volatile var stored: ReadyAlertsStatus?) : ReadyAlertsLedger {
        override suspend fun status(): ReadyAlertsStatus? = stored

        override suspend fun unseen(alerts: List<ReadyAlert>): List<ReadyAlert> = alerts

        override suspend fun commit(batch: ReadyAlertsBatch, shown: List<ReadyAlert>) {
            stored = batch.status
        }

        override suspend fun rememberStatus(batch: ReadyAlertsBatch) {
            stored = batch.status.copy(cursor = stored?.cursor ?: 0L)
        }

        override suspend fun erase() {
            stored = null
        }
    }

    private object NoPresenter : ReadyAlertPresenter {
        override suspend fun apply(settings: ReadyAlertSettings) = Unit

        override suspend fun present(alerts: List<ReadyAlert>) = Unit
    }
}
