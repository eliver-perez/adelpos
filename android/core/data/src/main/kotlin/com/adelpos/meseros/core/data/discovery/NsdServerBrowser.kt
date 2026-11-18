package com.adelpos.meseros.core.data.discovery

import android.content.Context
import android.net.nsd.NsdManager
import android.net.nsd.NsdServiceInfo
import android.os.Build
import androidx.annotation.RequiresApi
import com.adelpos.meseros.core.domain.server.FoundServer
import com.adelpos.meseros.core.domain.server.ServerAnnouncements
import com.adelpos.meseros.core.domain.server.ServerBrowser
import java.util.concurrent.Executor
import java.util.concurrent.Executors
import kotlin.coroutines.resume
import kotlinx.coroutines.CompletableDeferred
import kotlinx.coroutines.channels.Channel
import kotlinx.coroutines.suspendCancellableCoroutine
import kotlinx.coroutines.withTimeoutOrNull

/**
 * Búsqueda del servidor del restaurante en la red local con NSD (mDNS/DNS-SD) por su tipo
 * `_adelpos._tcp` (§4). Cada búsqueda dura [windowMillis]: descubre las instancias, resuelve cada
 * una (dirección, puerto y TXT) y entrega solo los anuncios válidos ([ServerAnnouncements.parse]).
 * El anuncio no da confianza: solo la dirección.
 *
 * Diferencias por versión (avance §1.6): desde Android 14 (API 34) se resuelve con
 * `registerServiceInfoCallback` y `hostAddresses`; antes, con `resolveService` y `host`, **de una
 * en una** (antes de la 34 una resolución simultánea falla con `FAILURE_ALREADY_ACTIVE`).
 *
 * @param serviceType tipo que se busca; las pruebas instrumentadas usan uno propio para no ver
 *   nunca el servidor real del restaurante (ver `AdelTestRunner`).
 */
class NsdServerBrowser(
    context: Context,
    private val serviceType: () -> String,
    private val windowMillis: Long = DEFAULT_WINDOW_MILLIS,
) : ServerBrowser {
    private val manager: NsdManager? =
        context.applicationContext.getSystemService(NsdManager::class.java)

    /** Resultado de una búsqueda, distinguiendo «no se pudo buscar» de «no hay nada». */
    sealed interface Browse {
        data class Found(val servers: List<FoundServer>) : Browse

        /** El sistema no permitió buscar (sin NSD, sin permiso de red local o error al empezar). */
        data object Unavailable : Browse
    }

    override suspend fun search(): List<FoundServer> =
        when (val result = browse()) {
            is Browse.Found -> result.servers
            Browse.Unavailable -> emptyList()
        }

    /** Busca durante [windowMillis] y devuelve los servidores encontrados. */
    suspend fun browse(): Browse {
        val nsd = manager ?: return Browse.Unavailable
        val type = serviceType()
        val discovered = Channel<NsdServiceInfo>(Channel.UNLIMITED)
        val started = CompletableDeferred<Boolean>()
        val listener =
            object : NsdManager.DiscoveryListener {
                override fun onDiscoveryStarted(serviceType: String) {
                    started.complete(true)
                }

                override fun onStartDiscoveryFailed(serviceType: String, errorCode: Int) {
                    started.complete(false)
                }

                override fun onServiceFound(serviceInfo: NsdServiceInfo) {
                    discovered.trySend(serviceInfo)
                }

                override fun onServiceLost(serviceInfo: NsdServiceInfo) = Unit

                override fun onDiscoveryStopped(serviceType: String) = Unit

                override fun onStopDiscoveryFailed(serviceType: String, errorCode: Int) = Unit
            }
        try {
            nsd.discoverServices(type, NsdManager.PROTOCOL_DNS_SD, listener)
        } catch (_: RuntimeException) {
            // Sin permiso de red local (targetSdk 37 en Android 17) o NSD no disponible.
            return Browse.Unavailable
        }
        val found = mutableListOf<FoundServer>()
        try {
            if (withTimeoutOrNull(START_TIMEOUT_MILLIS) { started.await() } != true) {
                return Browse.Unavailable
            }
            // Durante la ventana se resuelve cada instancia que aparece, de una en una (así
            // funciona
            // igual en todas las versiones); al vencer, lo que se haya encontrado.
            withTimeoutOrNull(windowMillis) {
                for (service in discovered) {
                    val server = resolve(nsd, service)
                    if (server != null) {
                        found.add(server)
                    }
                }
            }
        } finally {
            discovered.close()
            try {
                nsd.stopServiceDiscovery(listener)
            } catch (_: IllegalArgumentException) {
                // La búsqueda ya había terminado.
            }
        }
        return Browse.Found(ServerAnnouncements.merge(found))
    }

    private suspend fun resolve(nsd: NsdManager, service: NsdServiceInfo): FoundServer? =
        withTimeoutOrNull(RESOLVE_TIMEOUT_MILLIS) {
            val resolved =
                if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.UPSIDE_DOWN_CAKE) {
                    resolveWithCallback(nsd, service)
                } else {
                    resolveLegacy(nsd, service)
                }
            resolved?.let(::toFoundServer)
        }

    private fun toFoundServer(resolved: Resolved): FoundServer? =
        ServerAnnouncements.parse(resolved.name, resolved.txt, resolved.port, resolved.hosts)

    private class Resolved(
        val name: String,
        val txt: Map<String, ByteArray?>,
        val port: Int,
        val hosts: List<String>,
    )

    @RequiresApi(Build.VERSION_CODES.UPSIDE_DOWN_CAKE)
    private suspend fun resolveWithCallback(nsd: NsdManager, service: NsdServiceInfo): Resolved? =
        suspendCancellableCoroutine { continuation ->
            val callback =
                object : NsdManager.ServiceInfoCallback {
                    override fun onServiceInfoCallbackRegistrationFailed(errorCode: Int) {
                        if (continuation.isActive) {
                            continuation.resume(null)
                        }
                    }

                    override fun onServiceUpdated(serviceInfo: NsdServiceInfo) {
                        val hosts = serviceInfo.hostAddresses.mapNotNull { it.hostAddress }
                        if (hosts.isEmpty() || !continuation.isActive) {
                            return
                        }
                        continuation.resume(
                            Resolved(
                                serviceInfo.serviceName,
                                serviceInfo.attributes,
                                serviceInfo.port,
                                hosts,
                            ),
                        )
                        unregister(nsd, this)
                    }

                    override fun onServiceLost() = Unit

                    override fun onServiceInfoCallbackUnregistered() = Unit
                }
            try {
                nsd.registerServiceInfoCallback(service, callbackExecutor, callback)
            } catch (_: RuntimeException) {
                continuation.resume(null)
                return@suspendCancellableCoroutine
            }
            continuation.invokeOnCancellation { unregister(nsd, callback) }
        }

    @RequiresApi(Build.VERSION_CODES.UPSIDE_DOWN_CAKE)
    private fun unregister(nsd: NsdManager, callback: NsdManager.ServiceInfoCallback) {
        try {
            nsd.unregisterServiceInfoCallback(callback)
        } catch (_: IllegalArgumentException) {
            // Ya no estaba registrado.
        }
    }

    // Android 8 a 13: resolveService y NsdServiceInfo.host están en desuso desde la API 34, pero
    // son la única forma antes de ella.
    @Suppress("DEPRECATION")
    private suspend fun resolveLegacy(nsd: NsdManager, service: NsdServiceInfo): Resolved? =
        suspendCancellableCoroutine { continuation ->
            val listener =
                object : NsdManager.ResolveListener {
                    override fun onResolveFailed(serviceInfo: NsdServiceInfo, errorCode: Int) {
                        if (continuation.isActive) {
                            continuation.resume(null)
                        }
                    }

                    override fun onServiceResolved(serviceInfo: NsdServiceInfo) {
                        val host = serviceInfo.host?.hostAddress
                        if (!continuation.isActive) {
                            return
                        }
                        continuation.resume(
                            host?.let {
                                Resolved(
                                    serviceInfo.serviceName,
                                    serviceInfo.attributes,
                                    serviceInfo.port,
                                    listOf(it),
                                )
                            },
                        )
                    }
                }
            try {
                nsd.resolveService(service, listener)
            } catch (_: RuntimeException) {
                continuation.resume(null)
            }
        }

    companion object {
        const val DEFAULT_WINDOW_MILLIS = 4_000L
        private const val START_TIMEOUT_MILLIS = 3_000L
        private const val RESOLVE_TIMEOUT_MILLIS = 3_000L
        private val callbackExecutor: Executor = Executors.newSingleThreadExecutor()
    }
}
