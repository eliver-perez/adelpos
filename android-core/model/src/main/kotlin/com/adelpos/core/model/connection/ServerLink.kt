package com.adelpos.core.model.connection

import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow

/**
 * **El único estado de conexión con la computadora principal** de toda la aplicación (etapa A3, §2,
 * A3-D1). La barra («En línea» / «Sin conexión»), «Selecciona tu usuario» y la entrada con PIN,
 * Mesas, Para llevar, la precuenta, «Entregado» y la notificación del canal de avisos leen este
 * estado y nada más: nunca se contradicen.
 *
 * Lo alimentan la red de Android (sin red: «Sin conexión» al instante) y cada respuesta de Go en un
 * punto único del cliente HTTP ([ServerAnswers]). Las reglas de tolerancia y de revisión viven en
 * [ServerLinkTracker]; la implementación con su revisión periódica, en `core:data`
 * (`ServerLinkMonitor`, una instancia por proceso en `AppContainer`).
 */
interface ServerLink {
    /** [LinkState.ONLINE] («En línea») o [LinkState.OFFLINE] («Sin conexión»). */
    val state: StateFlow<LinkState>

    /**
     * Reintento en el momento, antes de decir «Sin conexión» por algo que tocó el mesero: si el
     * estado es «Sin conexión» pero Android tiene red, pregunta a Go enseguida y espera la
     * respuesta. Devuelve true si el estado queda «En línea».
     */
    suspend fun confirm(): Boolean
}

/**
 * Lo que el cliente HTTP informa de cada petición al servidor emparejado (el punto único de
 * `PinnedHttpClient`). Una respuesta HTTP cualquiera (también 4xx o 5xx: Go respondió) es
 * [answered]; una petición que se quedó sin respuesta es [noAnswer]. Una cancelación propia (cambio
 * de mesero, salir de primer plano) no cuenta.
 */
interface ServerAnswers {
    fun answered()

    fun noAnswer()

    /**
     * Una lectura falló sin respuesta y se repitió con otra conexión (solo para el registro: su
     * resultado llega después como [answered] o [noAnswer]).
     */
    fun repeated() = Unit
}

/** Estado fijo para pruebas y vistas previas: [set] lo cambia y [confirm] solo lo lee. */
class FixedServerLink(initial: LinkState = LinkState.ONLINE) : ServerLink {
    private val mutableState = MutableStateFlow(initial)

    override val state: StateFlow<LinkState> = mutableState.asStateFlow()

    fun set(link: LinkState) {
        mutableState.value = link
    }

    override suspend fun confirm(): Boolean = mutableState.value == LinkState.ONLINE
}
