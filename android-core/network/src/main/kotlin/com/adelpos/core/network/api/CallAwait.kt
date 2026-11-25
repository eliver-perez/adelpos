package com.adelpos.core.network.api

import java.io.IOException
import java.util.concurrent.Executor
import kotlin.coroutines.resumeWithException
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.asExecutor
import kotlinx.coroutines.suspendCancellableCoroutine
import okhttp3.Call
import okhttp3.Callback
import okhttp3.Response

/**
 * Ejecuta la llamada sin bloquear el hilo; cancelar la corrutina cancela la petición. Lanza
 * `IOException` si no hubo respuesta. La única forma de esperar una llamada de OkHttp en las
 * aplicaciones Android (la API del servidor, el modo Mesero y las llamadas con la sesión del
 * mesero).
 *
 * Si la corrutina se cancela cuando la respuesta ya llegó pero todavía no se entregó (por ejemplo,
 * al salir de una pantalla mientras responde Go), la respuesta se cierra **en otro hilo**
 * ([closer]): cerrar una conexión TLS escribe en la red (`close_notify`) y, si la corrutina era del
 * hilo principal, Android lanzaba `NetworkOnMainThreadException` dentro del manejador de la
 * cancelación y la aplicación se cerraba (visto en la Fase E de la A3, `adjustments a4`).
 */
suspend fun Call.awaitResponse(closer: Executor = RESPONSE_CLOSER): Response =
    suspendCancellableCoroutine { continuation ->
        continuation.invokeOnCancellation { cancel() }
        enqueue(
            object : Callback {
                override fun onFailure(call: Call, e: IOException) {
                    continuation.resumeWithException(e)
                }

                override fun onResponse(call: Call, response: Response) {
                    continuation.resume(response) { _, value, _ ->
                        closer.execute { value.close() }
                    }
                }
            },
        )
    }

// Los cierres de respuestas abandonadas van a los hilos de entrada y salida, nunca al principal.
private val RESPONSE_CLOSER: Executor = Dispatchers.IO.asExecutor()
