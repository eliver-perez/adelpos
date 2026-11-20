package com.adelpos.meseros.core.domain.ready

/**
 * Las notificaciones de cuenta que se ven, para decidir el resumen del grupo (etapa A3, Fase E,
 * `alerts r5`). Android publica cada notificación después de pedirla (`NotificationManager.notify`
 * es asíncrono): justo después de publicarla, su lista muchas veces todavía no la tiene (en Android
 * 13, 18 de 30 en una medición en el emulador), y con una sola cuenta en la lista no se publicaba
 * el resumen. Lo mismo al quitar una: la lista todavía la tiene un momento.
 *
 * Por eso se decide con lo que Android lista **más** lo que la aplicación acaba de publicar y
 * **menos** lo que acaba de quitar. Lo recordado dura hasta que la lista de Android lo refleja o
 * pasa [lagMillis] (por ejemplo, el mesero descartó la notificación enseguida o las notificaciones
 * están apagadas): después manda solo la lista de Android.
 */
class ReadyShownNotices(
    private val clock: () -> Long,
    private val lagMillis: Long = LAG_MILLIS,
) {
    private class Posted(val notice: ReadyNotice, val at: Long)

    private val posted = LinkedHashMap<String, Posted>()
    private val removed = LinkedHashMap<String, Long>()

    /** La aplicación acaba de publicar (o reemplazar) la notificación de esta cuenta. */
    @Synchronized
    fun posted(notice: ReadyNotice) {
        posted[notice.accountId] = Posted(notice, clock())
        removed.remove(notice.accountId)
    }

    /** La aplicación acaba de quitar la notificación de [accountId]. */
    @Synchronized
    fun removed(accountId: String) {
        posted.remove(accountId)
        removed[accountId] = clock()
    }

    /** Se quitaron todas: no queda nada recordado. */
    @Synchronized
    fun cleared() {
        posted.clear()
        removed.clear()
    }

    /**
     * Las cuentas a la vista: [listed] (lo que Android lista ahora), más lo publicado que todavía
     * no lista, menos lo quitado que todavía lista.
     */
    @Synchronized
    fun visible(listed: Map<String, ReadyNotice>): Map<String, ReadyNotice> {
        val now = clock()
        // Ya lo refleja Android (o pasó el plazo): manda su lista.
        posted.entries.removeIf { (account, entry) ->
            listed[account]?.shown == entry.notice.shown || now - entry.at > lagMillis
        }
        removed.entries.removeIf { (account, at) -> account !in listed || now - at > lagMillis }
        return (listed + posted.mapValues { it.value.notice }) - removed.keys
    }

    companion object {
        /** Lo más que se espera a que la lista de Android refleje lo publicado o quitado. */
        const val LAG_MILLIS = 10_000L

        /**
         * Con dos o más cuentas a la vista se publica (o se pone al día) el resumen del grupo. Con
         * una sola no se publica; si ya estaba, se deja (quitarlo quitaría también la notificación
         * de esa cuenta).
         */
        fun needsSummary(visible: Map<String, ReadyNotice>): Boolean = visible.size >= 2
    }
}
