package com.adelpos.meseros.core.domain.connection

/**
 * Las reglas del estado único de conexión ([ServerLink], etapa A3, §2 y A3-D1), sin Android: se
 * prueba en la JVM con un reloj virtual. [clock] es un reloj monótono en milisegundos
 * (`SystemClock.elapsedRealtime` en la aplicación).
 *
 * **Tolerancia**:
 * - Sin red de Android: «Sin conexión» al instante.
 * - Una petición sin respuesta deja el estado en «En línea» y empieza una racha («dudando»); se
 *   declara «Sin conexión» solo con [MIN_FAILURES] fallas seguidas **y** [MIN_OFFLINE_MILLIS] desde
 *   la primera de la racha sin ningún éxito.
 * - El primer éxito (cualquier respuesta de Go) vuelve a «En línea» y termina la racha.
 *
 * **Revisión según el uso** ([nextProbeIn]; la sonda es `GET health/live`):
 * - Dudando: una sonda cada [DOUBT_PROBE_MILLIS] para confirmarlo pronto.
 * - «Sin conexión» con red de Android: una cada [OFFLINE_PROBE_MILLIS] para volver pronto.
 * - En línea con el canal de eventos abierto: ninguna (el canal es la revisión).
 * - En línea sin canal (en «¿Quién eres?», sin sesión o sin uso): una cada [IDLE_CHECK_MILLIS], que
 *   se salta si hubo otro éxito en ese lapso (la lista de «¿Quién eres?» cuenta).
 * - En segundo plano o sin red de Android: ninguna (la batería; al volver a primer plano o la red
 *   se revisa enseguida).
 *
 * **Al arrancar** (etapa A3, decisión del coordinador en la Fase C: el indicador nunca dice algo
 * falso), con la red de Android todavía desconocida ([initialNetwork] null: Android aún no la
 * informó), el estado **duda** y se ve «En línea» hasta la primera respuesta de Go («En línea»), el
 * primer aviso de Android de que no hay red («Sin conexión» al instante) o el plazo de la
 * tolerancia sin ninguna de las dos ([MIN_OFFLINE_MILLIS]: «Sin conexión»). Un estado que todavía
 * no se conoce nunca se muestra como «Sin conexión».
 */
class ServerLinkTracker(initialNetwork: LinkState?, private val clock: () -> Long) {
    // La red de Android: null mientras Android no la informa (al arrancar).
    private var network: LinkState? = initialNetwork
    private val startedAt = clock()

    // «Sin conexión» declarado por la tolerancia o por la red de Android; solo un éxito con red lo
    // quita.
    private var declaredOffline = initialNetwork == LinkState.OFFLINE
    private var failures = 0
    private var streakStartedAt: Long? = null
    private var lastFailureAt: Long? = null
    private var lastSuccessAt: Long? = null
    private var lastProbeAt: Long? = null

    /** Lo que se muestra: «En línea» o «Sin conexión». */
    val state: LinkState
        @Synchronized
        get() =
            if (network == LinkState.OFFLINE || declaredOffline || startupExpired()) {
                LinkState.OFFLINE
            } else {
                LinkState.ONLINE
            }

    /** Puede haber red de Android (la hay, o al arrancar todavía no se sabe). */
    val hasNetwork: Boolean
        @Synchronized get() = network != LinkState.OFFLINE

    /**
     * Se sigue mostrando «En línea» sin haberlo confirmado: hubo fallas seguidas o, al arrancar,
     * todavía no responde Go ni informa Android.
     */
    val doubting: Boolean
        @Synchronized get() = state == LinkState.ONLINE && (failures > 0 || startingUp())

    // Al arrancar: sin aviso de Android ni respuesta de Go todavía.
    private fun startingUp(): Boolean = network == null && lastSuccessAt == null

    // Pasó el plazo de la tolerancia al arrancar sin aviso de Android ni respuesta de Go.
    private fun startupExpired(): Boolean =
        startingUp() && clock() - startedAt >= MIN_OFFLINE_MILLIS

    /** La red de Android cambió. Al volver, el estado sigue igual hasta el primer éxito. */
    @Synchronized
    fun onNetwork(link: LinkState): LinkState {
        // Ya se mostraba «Sin conexión» por el plazo al arrancar: se queda hasta el primer éxito.
        if (startupExpired()) {
            declaredOffline = true
        }
        network = link
        if (link == LinkState.OFFLINE) {
            declaredOffline = true
            failures = 0
            streakStartedAt = null
            lastFailureAt = null
        } else {
            // Red de vuelta: se sondea enseguida (sin esperar el intervalo de la última sonda).
            lastProbeAt = null
        }
        return state
    }

    /** Go respondió (cualquier respuesta HTTP o un mensaje de un canal). */
    @Synchronized
    fun onAnswer(): LinkState {
        lastSuccessAt = clock()
        if (network == null) {
            // Si Go respondió, hay red, aunque Android no lo haya informado todavía.
            network = LinkState.ONLINE
        }
        failures = 0
        streakStartedAt = null
        lastFailureAt = null
        if (network == LinkState.ONLINE) {
            declaredOffline = false
        }
        return state
    }

    /** Una petición se quedó sin respuesta (no una cancelación propia). */
    @Synchronized
    fun onNoAnswer(): LinkState {
        if (network == LinkState.OFFLINE) {
            return state
        }
        val now = clock()
        failures += 1
        lastFailureAt = now
        val start = streakStartedAt ?: now
        streakStartedAt = start
        if (failures >= MIN_FAILURES && now - start >= MIN_OFFLINE_MILLIS) {
            declaredOffline = true
        }
        return state
    }

    /**
     * No hay servidor emparejado al que preguntar: el estado sigue a la red de Android (como antes
     * de emparejar, con «Buscar el servidor»).
     */
    @Synchronized
    fun onNothingToProbe(): LinkState {
        lastProbeAt = clock()
        failures = 0
        streakStartedAt = null
        lastFailureAt = null
        if (network == LinkState.ONLINE) {
            declaredOffline = false
        }
        return state
    }

    /** Empieza una sonda: la siguiente se cuenta desde aquí. */
    @Synchronized
    fun onProbe() {
        lastProbeAt = clock()
    }

    /**
     * Cuánto falta para que el estado cambie solo con el tiempo (el plazo de la tolerancia al
     * arrancar), o null si nada cambia solo. Quien publica el estado lo vuelve a leer entonces.
     */
    @Synchronized
    fun nextDecisionIn(): Long? {
        if (!startingUp() || declaredOffline) {
            return null
        }
        val remaining = startedAt + MIN_OFFLINE_MILLIS - clock()
        return if (remaining > 0) remaining else null
    }

    /**
     * Cuánto falta para la siguiente sonda (0: ya), o null si no hace falta ninguna hasta que algo
     * cambie (la red, el primer plano, el canal de eventos o una falla).
     */
    @Synchronized
    fun nextProbeIn(foreground: Boolean, channelOpen: Boolean): Long? {
        if (network == LinkState.OFFLINE || !foreground) {
            return null
        }
        val now = clock()
        val (interval, since) =
            when {
                declaredOffline || startupExpired() ->
                    OFFLINE_PROBE_MILLIS to latest(lastProbeAt, lastFailureAt)
                failures > 0 || startingUp() ->
                    DOUBT_PROBE_MILLIS to latest(lastProbeAt, lastFailureAt)
                channelOpen -> return null
                else -> IDLE_CHECK_MILLIS to latest(lastProbeAt, lastSuccessAt)
            }
        if (since == null) {
            return 0
        }
        return (since + interval - now).coerceAtLeast(0)
    }

    private fun latest(first: Long?, second: Long?): Long? =
        when {
            first == null -> second
            second == null -> first
            else -> maxOf(first, second)
        }

    companion object {
        /**
         * Fallas seguidas para declarar «Sin conexión». Una sola falla aislada (una conexión que Go
         * cerró, un paquete perdido) nunca cambia el estado: el cliente ya repite una lectura sin
         * respuesta y la sonda lo confirma.
         */
        const val MIN_FAILURES = 3

        /**
         * Tiempo mínimo desde la primera falla de la racha sin ningún éxito (§2: «unos 20
         * segundos», Fase 0 §0.3). Una falla aislada se resuelve en milisegundos; un cambio de
         * punto de acceso o una renovación de DHCP tarda unos segundos; con tres sondas cada 5 s y
         * el plazo de conexión de 5 s del cliente, 20 s distingue una caída real sin dejar al
         * mesero mucho tiempo creyendo que hay conexión.
         */
        const val MIN_OFFLINE_MILLIS = 20_000L

        /** Sonda mientras se duda: tres sondas caben en los 20 s de [MIN_OFFLINE_MILLIS]. */
        const val DOUBT_PROBE_MILLIS = 5_000L

        /**
         * Sonda con «Sin conexión» y red de Android: vuelve a «En línea» a lo más unos 10 s después
         * de que Go responde (el mismo intervalo con el que se busca el servidor en otra dirección,
         * `ServerRelocator.DEFAULT_INTERVAL_MILLIS`).
         */
        const val OFFLINE_PROBE_MILLIS = 10_000L

        /**
         * Revisión con el dispositivo en primer plano sin canal de eventos (§2: «cada 30–60
         * segundos» con el dispositivo inactivo); se salta si hubo otro éxito en ese lapso.
         */
        const val IDLE_CHECK_MILLIS = 60_000L
    }
}
