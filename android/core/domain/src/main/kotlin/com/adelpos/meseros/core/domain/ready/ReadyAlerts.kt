package com.adelpos.meseros.core.domain.ready

import com.adelpos.meseros.core.domain.waiter.OrderMode

/*
 * Aviso de platillos listos en el dispositivo (etapa A2, §4; A2-D7 a A2-D10). Go decide a quién
 * llega cada aviso y entrega solo sus datos (mesa o folio, cliente, mesero, platillo y cantidad;
 * nunca precios). El dispositivo los muestra **una sola vez** aunque vuelvan a llegar (cursor
 * atrasado, reintento, proceso reiniciado) y nunca pierde uno por un corte de red (al volver pide
 * `ready-alerts?after=<cursor>`). Todo lo de aquí es lógica pura, probada en la JVM; la red, Room y
 * las notificaciones viven en `core:data/ready` y en `app`.
 */

/** Un platillo listo de un aviso: el nombre visible que da Go («Filete · Grande») y cuántos. */
data class ReadyItem(val productName: String, val quantity: Int)

/**
 * Un aviso de `GET ready-alerts` (`ReadyAlert` del contrato): lo que cocina marcó «Listo» en una
 * cuenta de una sola vez, agrupado por platillo.
 *
 * @param id el número del aviso en Go: **solo** sirve de cursor (`after`, `Last-Event-ID`). Tras
 *   restaurar un respaldo en la computadora principal, Go puede repetir números con otro contenido.
 * @param key la identidad del aviso que da Go, única y que nunca se reutiliza: un aviso con una
 *   [key] ya mostrada en este dispositivo nunca se vuelve a mostrar.
 * @param waiterUserId el mesero del aviso: al tocarlo en «Selecciona tu usuario» se le pide su PIN.
 */
data class ReadyAlert(
    val id: Long,
    val key: String,
    val accountId: String,
    val mode: OrderMode,
    val tableName: String?,
    val folio: Long,
    val customerName: String,
    val waiterUserId: String,
    val waiterShortName: String,
    val items: List<ReadyItem>,
    val readyAt: String,
)

/**
 * Lo que se eligió en Administración › Sistema › Terminales › Editar del dispositivo (§4.5).
 * [enabled] apagado llega con `active: false`: sin avisos y el servicio se detiene.
 */
data class ReadyAlertSettings(val enabled: Boolean, val sound: Boolean, val vibration: Boolean) {
    companion object {
        /** Lo de Go mientras no se cambie (todo activado). */
        val DEFAULT = ReadyAlertSettings(enabled = true, sound = true, vibration = true)
    }
}

/**
 * Una respuesta de `GET ready-alerts` o un mensaje `alerts` de su canal.
 *
 * @param cursor el último aviso que Go revisó para este dispositivo (le corresponda o no): siempre
 *   crece y se guarda tal cual; nunca se calcula con los [ReadyAlert.id].
 * @param active si el dispositivo debe mantener su servicio de avisos (aviso activado y algún
 *   mesero con sesión o que entró en el día comercial).
 * @param dayEndsAtMillis fin del día comercial en curso: ahí dejan de contar los meseros que
 *   entraron hoy.
 */
data class ReadyAlertsBatch(
    val cursor: Long,
    val active: Boolean,
    val dayEndsAtMillis: Long,
    val settings: ReadyAlertSettings,
    val alerts: List<ReadyAlert>,
) {
    /** Lo que se guarda en el dispositivo de este lote (sin los avisos). */
    val status: ReadyAlertsStatus
        get() = ReadyAlertsStatus(cursor, active, dayEndsAtMillis, settings)
}

/**
 * Lo último que dijo Go, guardado en el dispositivo: el cursor para reanudar y si el servicio debe
 * arrancar al abrir la aplicación o al reiniciar el equipo (§0.5.5 de la Fase 0).
 */
data class ReadyAlertsStatus(
    val cursor: Long,
    val active: Boolean,
    val dayEndsAtMillis: Long,
    val settings: ReadyAlertSettings,
)

/**
 * Registro del dispositivo (Room en `core:data`): el cursor, lo último que dijo Go y los avisos ya
 * mostrados del día. Un solo escritor a la vez (el canal de avisos del proceso).
 */
interface ReadyAlertsLedger {
    /** Lo guardado, o null si este dispositivo nunca preguntó (o se borraron sus datos). */
    suspend fun status(): ReadyAlertsStatus?

    /** De [alerts], los que todavía no se mostraron en este dispositivo. */
    suspend fun unseen(alerts: List<ReadyAlert>): List<ReadyAlert>

    /**
     * En una transacción: marca [shown] como ya avisados y guarda el cursor y el estado del lote.
     * Al cambiar de día comercial borra los avisados de días anteriores (Go ya no los entrega).
     */
    suspend fun commit(batch: ReadyAlertsBatch, shown: List<ReadyAlert>)

    /**
     * Guarda solo si está activo, el fin del día y la configuración, **sin mover el cursor** (la
     * consulta previa al arranque, que no muestra avisos).
     */
    suspend fun rememberStatus(batch: ReadyAlertsBatch)

    /** Borra todo (retirar el dispositivo o emparejarlo de nuevo). */
    suspend fun erase()
}

/**
 * Quien muestra los avisos (las notificaciones de Android o el aviso dentro de la aplicación).
 * [present] debe ser **idempotente**: si recibe un aviso que ya está a la vista (el proceso murió
 * después de mostrarlo y antes de guardarlo), no lo vuelve a mostrar.
 */
interface ReadyAlertPresenter {
    /** La configuración de sonido y vibración vigente (elige el canal, A2-D7). */
    suspend fun apply(settings: ReadyAlertSettings)

    /** Muestra los avisos nuevos de un lote, agrupados por cuenta. */
    suspend fun present(alerts: List<ReadyAlert>)
}

/** Qué sigue después de un lote. */
enum class ReadyBatchOutcome {
    /** Seguir recibiendo. */
    CONTINUE,

    /** `active: false`: no hay a quién avisar (o se apagó en Terminales): detener el servicio. */
    INACTIVE,
}

/**
 * Lo que hace el dispositivo con cada lote de Go: elegir el canal, quitar lo ya avisado, mostrar lo
 * nuevo y guardar el cursor.
 *
 * **Orden**: primero se muestran los avisos y después se guardan (ids y cursor, en una
 * transacción). Si el proceso muere en medio, el aviso vuelve a llegar desde el cursor guardado y
 * el presentador lo reconoce en la notificación que ya está a la vista: no se pierde y no se
 * repite. Al revés (guardar y después mostrar) un proceso que muere en medio lo perdería.
 *
 * No escribe en Room con cada mensaje del canal (Go manda uno cada 750 ms): solo si llegó algo
 * nuevo o cambió el cursor, si está activo, el fin del día o la configuración.
 */
class ReadyAlertsProcessor(
    private val ledger: ReadyAlertsLedger,
    private val presenter: ReadyAlertPresenter,
) {
    @Volatile private var lastSaved: ReadyAlertsStatus? = null

    /**
     * Cada vez que el canal vuelve a empezar (o tras borrar los datos del dispositivo) el primer
     * lote se guarda siempre: lo recordado en memoria pudo dejar de ser lo que hay en Room.
     */
    fun restart() {
        lastSaved = null
    }

    suspend fun onBatch(batch: ReadyAlertsBatch): ReadyBatchOutcome {
        presenter.apply(batch.settings)
        // Un mismo aviso dos veces en el lote (no debería pasar) cuenta una sola vez.
        val fresh = ledger.unseen(batch.alerts.distinctBy { it.key })
        if (fresh.isNotEmpty()) {
            presenter.present(fresh)
        }
        val status = batch.status
        if (fresh.isNotEmpty() || status != lastSaved) {
            ledger.commit(batch, fresh)
            lastSaved = status
        }
        return if (batch.active) ReadyBatchOutcome.CONTINUE else ReadyBatchOutcome.INACTIVE
    }

    /**
     * La consulta previa al arranque (al abrir la aplicación o al entrar un mesero): no muestra
     * avisos. Activo, guarda solo el estado (el servicio los recibirá desde el cursor guardado);
     * inactivo, también el cursor, como lo haría el servicio: así, al activarse más tarde, no
     * llegan avisos viejos del tiempo en que estuvo apagado (decisión 5 de la parte D del
     * servidor).
     */
    suspend fun onProbe(batch: ReadyAlertsBatch) {
        if (batch.active) {
            ledger.rememberStatus(batch)
        } else {
            ledger.commit(batch.copy(alerts = emptyList()), emptyList())
        }
        lastSaved = null
    }
}
