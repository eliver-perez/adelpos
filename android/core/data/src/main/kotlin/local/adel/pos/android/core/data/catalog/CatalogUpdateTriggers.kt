package local.adel.pos.android.core.data.catalog

import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Job
import kotlinx.coroutines.delay
import kotlinx.coroutines.flow.Flow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.collectLatest
import kotlinx.coroutines.flow.distinctUntilChanged
import kotlinx.coroutines.flow.filter
import kotlinx.coroutines.flow.map
import kotlinx.coroutines.launch
import local.adel.pos.android.core.data.session.ServerAccess
import local.adel.pos.android.core.domain.connection.LinkState
import local.adel.pos.android.core.domain.realtime.RealtimeFeed
import local.adel.pos.android.core.domain.realtime.RealtimeScopes
import local.adel.pos.android.core.domain.realtime.RealtimeSignal

/**
 * Cuándo se actualiza el catálogo (§5 de la especificación):
 * - al abrir la aplicación o volver a ella ([foreground] pasa a verdadero);
 * - cuando hay sesión (un mesero entra o cambia);
 * - al recuperar la conexión ([link] pasa de «Sin conexión» a «En línea»; también se pide al canal
 *   de tiempo real que reconecte sin esperar);
 * - al recibir un evento del ámbito `catalog` (`catalog.changed`, agotado o disponible otra vez) o
 *   un aviso de «reconstruir» del canal;
 * - cada [periodMillis] mientras la aplicación está en uso (en primer plano), por si se perdió un
 *   evento.
 *
 * Todas terminan en [request] (normalmente `CatalogRepository.requestRefresh`), que atiende una
 * descarga a la vez.
 */
class CatalogUpdateTriggers(
    private val request: () -> Unit,
    private val access: ServerAccess,
    private val foreground: StateFlow<Boolean>,
    private val link: Flow<LinkState>,
    private val realtime: RealtimeFeed?,
    private val periodMillis: Long = DEFAULT_PERIOD_MILLIS,
) {
    /** Empieza a escuchar en [scope]; devuelve el trabajo para detenerlo (pruebas). */
    fun start(scope: CoroutineScope): Job = scope.launch {
        launch {
            access.current
                .map { it != null }
                .distinctUntilChanged()
                .filter { it }
                .collect { request() }
        }
        launch { foreground.filter { it }.collect { request() } }
        launch {
            var previous: LinkState? = null
            link.distinctUntilChanged().collect { state ->
                if (previous == LinkState.OFFLINE && state == LinkState.ONLINE) {
                    realtime?.reconnectNow()
                    request()
                }
                previous = state
            }
        }
        realtime?.let { feed -> launch { feed.signals.collect { onSignal(it) } } }
        launch {
            // collectLatest cancela el temporizador en cuanto la aplicación pasa a segundo
            // plano.
            foreground.collectLatest { inUse ->
                if (inUse) {
                    while (true) {
                        delay(periodMillis)
                        request()
                    }
                }
            }
        }
    }

    private fun onSignal(signal: RealtimeSignal) {
        when (signal) {
            is RealtimeSignal.Changes ->
                if (signal.events.any { it.scope == RealtimeScopes.CATALOG }) {
                    request()
                }
            is RealtimeSignal.Rebuild -> request()
            is RealtimeSignal.Interrupted -> Unit
        }
    }

    companion object {
        /** Cada 5 minutos en uso: el tiempo real avisa al momento; esto es la red de seguridad. */
        const val DEFAULT_PERIOD_MILLIS: Long = 5 * 60 * 1000L
    }
}
