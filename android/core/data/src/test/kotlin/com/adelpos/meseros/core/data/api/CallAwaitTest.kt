package com.adelpos.meseros.core.data.api

import java.util.concurrent.CountDownLatch
import java.util.concurrent.Executor
import java.util.concurrent.Executors
import java.util.concurrent.TimeUnit
import java.util.concurrent.atomic.AtomicReference
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.asCoroutineDispatcher
import kotlinx.coroutines.launch
import mockwebserver3.MockResponse
import mockwebserver3.MockWebServer
import okhttp3.MediaType
import okhttp3.OkHttpClient
import okhttp3.Request
import okhttp3.ResponseBody
import okio.BufferedSource
import okio.ForwardingSource
import okio.buffer
import org.junit.After
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Before
import org.junit.Test

/**
 * Etapa A3 (Fase E, `adjustments a4` en Android 13): si la corrutina que espera una llamada se
 * cancela cuando la respuesta ya llegó pero todavía no se entregó, la respuesta se cierra en otro
 * hilo, nunca en el de la corrutina (en la aplicación, el principal: cerrar una conexión TLS
 * escribe en la red y Android lanzaba `NetworkOnMainThreadException`).
 */
class CallAwaitTest {
    private lateinit var server: MockWebServer
    private val main = Executors.newSingleThreadExecutor { task -> Thread(task, MAIN_THREAD) }

    @Before
    fun start() {
        server = MockWebServer()
        server.start()
    }

    @After
    fun stop() {
        main.shutdownNow()
        server.close()
    }

    @Test
    fun laRespuestaAbandonadaSeCierraFueraDelHiloDeLaCorrutina() {
        val thread = threadThatClosesAnAbandonedResponse(closer = null)
        assertFalse("se cerró en «$thread»", thread.startsWith(MAIN_THREAD))
    }

    @Test
    fun laPruebaReproduceLaCarrera() {
        // Con el cierre en el mismo hilo (como antes), se cerraría en el de la corrutina: la
        // prueba de arriba de verdad pasa por el manejador de la cancelación.
        // (Con la depuración de corrutinas el nombre del hilo lleva « @coroutine#N».)
        val thread = threadThatClosesAnAbandonedResponse(closer = Executor { it.run() })
        assertTrue("se cerró en «$thread»", thread.startsWith(MAIN_THREAD))
    }

    /** Hilo en que se cierra una respuesta que llegó cuando la corrutina ya se cancelaba. */
    private fun threadThatClosesAnAbandonedResponse(closer: Executor?): String {
        server.enqueue(MockResponse.Builder().body("{\"ok\":true}").build())
        val responded = CountDownLatch(1)
        val closedOn = AtomicReference<String>()
        val closed = CountDownLatch(1)
        // Una respuesta que dice en qué hilo se cerró.
        val client =
            OkHttpClient.Builder()
                .addInterceptor { chain ->
                    val response = chain.proceed(chain.request())
                    val body = response.body
                    val watched =
                        object : ForwardingSource(body.source()) {
                            override fun close() {
                                closedOn.compareAndSet(null, Thread.currentThread().name)
                                closed.countDown()
                                super.close()
                            }
                        }
                    responded.countDown()
                    response.newBuilder().body(WatchedBody(body, watched.buffer())).build()
                }
                .build()
        val call = client.newCall(Request.Builder().url(server.url("/api/v1/health/live")).build())
        val scope = CoroutineScope(main.asCoroutineDispatcher())
        val job = scope.launch {
            if (closer == null) call.awaitResponse() else call.awaitResponse(closer)
        }

        // La respuesta llega mientras el hilo de la corrutina está ocupado: queda sin entregar.
        val busy = CountDownLatch(1)
        main.execute { busy.await(10, TimeUnit.SECONDS) }
        assertTrue("llegó la respuesta", responded.await(10, TimeUnit.SECONDS))
        Thread.sleep(SETTLE_MILLIS)
        // Se cancela antes de entregarla (como al salir de la pantalla) y el hilo sigue.
        job.cancel()
        busy.countDown()

        assertTrue("la respuesta abandonada se cerró", closed.await(10, TimeUnit.SECONDS))
        assertTrue(job.isCancelled)
        return closedOn.get()
    }

    /** El cuerpo original con la fuente vigilada. */
    private class WatchedBody(
        private val original: ResponseBody,
        private val source: BufferedSource,
    ) : ResponseBody() {
        override fun contentType(): MediaType? = original.contentType()

        override fun contentLength(): Long = original.contentLength()

        override fun source(): BufferedSource = source
    }

    private companion object {
        const val MAIN_THREAD = "hilo-principal-de-prueba"
        const val SETTLE_MILLIS = 300L
    }
}
