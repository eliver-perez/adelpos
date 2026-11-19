package com.adelpos.meseros.core.domain.server

import kotlinx.coroutines.sync.Mutex
import kotlinx.coroutines.sync.withLock

/** Busca servidores del restaurante en la red local (NSD en Android, falso en las pruebas). */
fun interface ServerBrowser {
    /** Una búsqueda de una sola vez; vacía si no encontró nada o si no se pudo buscar. */
    suspend fun search(): List<FoundServer>
}

/**
 * Comprueba si en [base] responde el servidor con la huella SPKI completa [fingerprint]: solo
 * responde quien tiene la llave privada del servidor emparejado.
 */
fun interface ServerProbe {
    suspend fun answers(base: String, fingerprint: ByteArray): Boolean
}

/**
 * Cambio de IP del servidor emparejado (§4, como `ServerLocator::relocate` del escritorio): si no
 * responde en su última dirección, lo vuelve a buscar en la red por su `server_id` y su huella
 * corta y solo acepta la dirección nueva si ahí responde con la huella **completa** fijada. El
 * anuncio nunca da confianza, solo la dirección.
 *
 * Una sola búsqueda a la vez y como máximo una cada [minIntervalMillis]: si varias peticiones
 * fallan juntas, la primera busca y las demás usan su resultado.
 *
 * @param clock milisegundos de un reloj monótono (inyectable en las pruebas).
 */
class ServerRelocator(
    private val browser: ServerBrowser,
    private val probe: ServerProbe,
    private val clock: () -> Long,
    private val minIntervalMillis: Long = DEFAULT_INTERVAL_MILLIS,
) {
    private val mutex = Mutex()
    private var lastAttempt: Long? = null

    /**
     * Dirección nueva del servidor [serverId] con la huella [fingerprint], o null si no se
     * encontró. [currentBase] es la dirección vigente al llamar: si otra petición ya la cambió (es
     * distinta de [failedBase]), se devuelve esa sin buscar.
     */
    suspend fun relocate(
        serverId: String,
        fingerprint: ByteArray,
        failedBase: String,
        currentBase: () -> String,
    ): String? = mutex.withLock {
        val current = currentBase()
        if (current != failedBase) {
            return@withLock current
        }
        val now = clock()
        val previous = lastAttempt
        if (previous != null && now - previous < minIntervalMillis) {
            return@withLock null
        }
        lastAttempt = now
        val found = ServerAnnouncements.merge(browser.search())
        for (base in ServerAnnouncements.candidates(found, serverId, fingerprint, failedBase)) {
            if (probe.answers(base, fingerprint)) {
                return@withLock base
            }
        }
        null
    }

    companion object {
        const val DEFAULT_INTERVAL_MILLIS = 10_000L
    }
}
