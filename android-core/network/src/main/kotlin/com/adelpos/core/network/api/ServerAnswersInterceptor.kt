package com.adelpos.core.network.api

import com.adelpos.core.model.connection.ServerAnswers
import com.adelpos.core.model.error.AppError
import java.io.IOException
import java.io.InterruptedIOException
import okhttp3.ConnectionPool
import okhttp3.Interceptor
import okhttp3.Request
import okhttp3.Response

/**
 * El punto único del cliente HTTP hacia el servidor emparejado (etapa A3, A3-D1):
 * 1. Informa a [answers] (el estado único de conexión) cada respuesta de Go y cada petición que se
 *    quedó sin respuesta. Una cancelación propia (cambio de mesero, salir de primer plano, el fin
 *    de un canal) no cuenta.
 * 2. Repite **una vez**, con una conexión nueva, una lectura (`GET`) que falló sin respuesta. Es la
 *    red de seguridad de la causa de la Fase 0: OkHttp no revisa una conexión reutilizada antes de
 *    un `GET` (cuenta con reintentar) y la aplicación desactivó sus reintentos
 *    (`retryOnConnectionFailure(false)`) por las escrituras. Las escrituras **nunca** se repiten
 *    aquí: llevan su `operation_id` y cada una decide cuándo repetir con la misma clave.
 *
 * No se repiten: los canales (`text/event-stream`, con su propia reconexión y cursor: su protocolo
 * no cambia), un plazo vencido (repetirlo duplicaría la espera del mesero) ni otro certificado (la
 * huella no cambia al repetir).
 *
 * @param pool el de este cliente: antes de repetir se sueltan las conexiones libres, para que la
 *   repetición no tome otra conexión que Go ya cerró.
 */
internal class ServerAnswersInterceptor(
    private val answers: ServerAnswers?,
    private val pool: ConnectionPool,
) : Interceptor {
    override fun intercept(chain: Interceptor.Chain): Response {
        val request = chain.request()
        val response =
            try {
                chain.proceed(request)
            } catch (failure: IOException) {
                if (chain.call().isCanceled()) {
                    throw failure
                }
                if (!repeatable(request, failure)) {
                    answers?.noAnswer()
                    throw failure
                }
                pool.evictAll()
                answers?.repeated()
                try {
                    chain.proceed(request)
                } catch (again: IOException) {
                    if (!chain.call().isCanceled()) {
                        answers?.noAnswer()
                    }
                    again.addSuppressed(failure)
                    throw again
                }
            }
        answers?.answered()
        return response
    }

    private fun repeatable(request: Request, failure: IOException): Boolean =
        request.method == "GET" &&
            request.header(ACCEPT) != EVENT_STREAM &&
            failure !is InterruptedIOException &&
            ApiErrors.fromException(failure) != AppError.ServerIdentityChanged

    private companion object {
        const val ACCEPT = "Accept"
        const val EVENT_STREAM = "text/event-stream"
    }
}
