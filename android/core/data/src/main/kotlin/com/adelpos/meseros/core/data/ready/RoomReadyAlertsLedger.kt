package com.adelpos.meseros.core.data.ready

import com.adelpos.meseros.core.data.ready.db.ReadyAlertSeenEntity
import com.adelpos.meseros.core.data.ready.db.ReadyAlertsDao
import com.adelpos.meseros.core.data.ready.db.ReadyAlertsStateEntity
import com.adelpos.meseros.core.domain.ready.ReadyAlert
import com.adelpos.meseros.core.domain.ready.ReadyAlertSettings
import com.adelpos.meseros.core.domain.ready.ReadyAlertsBatch
import com.adelpos.meseros.core.domain.ready.ReadyAlertsLedger
import com.adelpos.meseros.core.domain.ready.ReadyAlertsStatus
import kotlinx.coroutines.sync.Mutex
import kotlinx.coroutines.sync.withLock

/**
 * El registro del aviso en Room (`ready_alerts_state` y `ready_alerts_seen`): el cursor, lo último
 * que dijo Go y los avisos ya mostrados del día. Las escrituras van de una en una ([mutex]): el
 * canal de avisos del proceso, la consulta previa al arranque y el borrado del dispositivo nunca se
 * pisan.
 */
class RoomReadyAlertsLedger(
    private val dao: ReadyAlertsDao,
    private val clock: () -> Long = System::currentTimeMillis,
) : ReadyAlertsLedger {
    private val mutex = Mutex()

    override suspend fun status(): ReadyAlertsStatus? = dao.state()?.toStatus()

    /**
     * Un aviso cuenta como ya mostrado si hay uno guardado con el mismo número **y** la misma hora;
     * el mismo número con otra hora es otro aviso (la computadora principal restauró un respaldo y
     * Go volvió a usar el número): se muestra.
     */
    override suspend fun unseen(alerts: List<ReadyAlert>): List<ReadyAlert> {
        if (alerts.isEmpty()) {
            return emptyList()
        }
        val seen = dao.seen(alerts.map { it.id }).associateBy { it.alertId }
        return alerts.filter { alert -> seen[alert.id]?.readyAt != alert.readyAt }
    }

    override suspend fun commit(batch: ReadyAlertsBatch, shown: List<ReadyAlert>) {
        mutex.withLock {
            val now = clock()
            val previous = dao.state()
            dao.commit(
                state = batch.toEntity(cursor = batch.cursor, now = now),
                shown =
                    shown.map { alert ->
                        ReadyAlertSeenEntity(
                            alertId = alert.id,
                            readyAt = alert.readyAt,
                            accountId = alert.accountId,
                            dayEndsAt = batch.dayEndsAtMillis,
                            seenAt = now,
                        )
                    },
                previousDayEndsAt = previous?.dayEndsAt,
            )
        }
    }

    override suspend fun rememberStatus(batch: ReadyAlertsBatch) {
        mutex.withLock {
            // El cursor se queda como estaba (o 0 si nunca se guardó): los avisos de este lote los
            // entregará el servicio.
            val cursor = dao.state()?.cursor ?: 0L
            dao.saveState(batch.toEntity(cursor = cursor, now = clock()))
        }
    }

    override suspend fun erase() {
        mutex.withLock { dao.eraseAll() }
    }

    /** Los avisos ya mostrados (soporte y pruebas). */
    suspend fun seenIds(): List<Long> = dao.allSeen().map { it.alertId }

    private fun ReadyAlertsBatch.toEntity(cursor: Long, now: Long) =
        ReadyAlertsStateEntity(
            cursor = cursor,
            active = active,
            dayEndsAt = dayEndsAtMillis,
            enabled = settings.enabled,
            sound = settings.sound,
            vibration = settings.vibration,
            updatedAt = now,
        )

    private fun ReadyAlertsStateEntity.toStatus() =
        ReadyAlertsStatus(
            cursor = cursor,
            active = active,
            dayEndsAtMillis = dayEndsAt,
            settings = ReadyAlertSettings(enabled, sound, vibration),
        )
}
