package com.adelpos.meseros.core.data.ready.db

import androidx.room.ColumnInfo
import androidx.room.Dao
import androidx.room.Entity
import androidx.room.Index
import androidx.room.PrimaryKey
import androidx.room.Query
import androidx.room.Transaction
import androidx.room.Upsert

/*
 * Aviso de platillos listos en Room (etapa A2, Fase D; `AppDatabase` v6, la versión única de la
 * etapa). Nada de esto es secreto: son el cursor de Go y los números de los avisos ya mostrados.
 * Todo se borra con «borrar los datos de este dispositivo» (`DeviceDataWipe`).
 */

/**
 * Lo último que dijo Go en `ready-alerts` (una sola fila, [slot] 0): el cursor para reanudar sin
 * perder avisos y lo que decide si el servicio arranca al abrir la aplicación o tras reiniciar el
 * equipo (si estaba activo y cuándo termina el día comercial), con la configuración de Terminales.
 */
@Entity(tableName = "ready_alerts_state")
data class ReadyAlertsStateEntity(
    @PrimaryKey val slot: Int = SLOT,
    val cursor: Long,
    val active: Boolean,
    @ColumnInfo(name = "day_ends_at") val dayEndsAt: Long,
    val enabled: Boolean,
    val sound: Boolean,
    val vibration: Boolean,
    @ColumnInfo(name = "updated_at") val updatedAt: Long,
) {
    companion object {
        const val SLOT = 0
    }
}

/**
 * Un aviso ya mostrado en este dispositivo (§4.1: «cada partida lista se avisa una sola vez por
 * dispositivo»): se descarta si vuelve a llegar, venga de donde venga. Se identifica por la [key]
 * de Go (única y nunca reutilizada); [alertId] es el número, que solo sirve de cursor y puede
 * repetirse tras restaurar un respaldo. [dayEndsAt] es el fin del día comercial en que llegó: al
 * cambiar de día se borran los de días anteriores.
 */
@Entity(tableName = "ready_alerts_seen", indices = [Index("day_ends_at")])
data class ReadyAlertSeenEntity(
    @PrimaryKey val key: String,
    @ColumnInfo(name = "alert_id") val alertId: Long,
    @ColumnInfo(name = "account_id") val accountId: String,
    @ColumnInfo(name = "day_ends_at") val dayEndsAt: Long,
    @ColumnInfo(name = "seen_at") val seenAt: Long,
)

/**
 * El SQL de las tablas del aviso, el que genera Room para la versión 6 (`schemas/…/6.json`): va al
 * final de la migración 5 → 6 (la versión de la etapa, todavía sin entregar).
 */
internal val READY_ALERTS_TABLES_V6: List<String> =
    listOf(
        "CREATE TABLE IF NOT EXISTS `ready_alerts_state` (`slot` INTEGER NOT NULL, " +
            "`cursor` INTEGER NOT NULL, `active` INTEGER NOT NULL, " +
            "`day_ends_at` INTEGER NOT NULL, `enabled` INTEGER NOT NULL, " +
            "`sound` INTEGER NOT NULL, `vibration` INTEGER NOT NULL, " +
            "`updated_at` INTEGER NOT NULL, PRIMARY KEY(`slot`))",
        "CREATE TABLE IF NOT EXISTS `ready_alerts_seen` (`key` TEXT NOT NULL, " +
            "`alert_id` INTEGER NOT NULL, `account_id` TEXT NOT NULL, " +
            "`day_ends_at` INTEGER NOT NULL, `seen_at` INTEGER NOT NULL, " +
            "PRIMARY KEY(`key`))",
        "CREATE INDEX IF NOT EXISTS `index_ready_alerts_seen_day_ends_at` " +
            "ON `ready_alerts_seen` (`day_ends_at`)",
    )

@Dao
interface ReadyAlertsDao {
    @Query("SELECT * FROM ready_alerts_state WHERE slot = 0")
    suspend fun state(): ReadyAlertsStateEntity?

    @Upsert suspend fun saveState(state: ReadyAlertsStateEntity)

    @Query("SELECT `key` FROM ready_alerts_seen WHERE `key` IN (:keys)")
    suspend fun seenKeys(keys: List<String>): List<String>

    @Upsert suspend fun markSeen(rows: List<ReadyAlertSeenEntity>)

    @Query("DELETE FROM ready_alerts_seen WHERE day_ends_at < :dayEndsAt")
    suspend fun forgetSeenBefore(dayEndsAt: Long)

    @Query("SELECT * FROM ready_alerts_seen ORDER BY alert_id, `key`")
    suspend fun allSeen(): List<ReadyAlertSeenEntity>

    @Query("DELETE FROM ready_alerts_seen") suspend fun deleteAllSeen()

    @Query("DELETE FROM ready_alerts_state") suspend fun deleteState()

    /**
     * Guarda en una transacción los avisos ya mostrados y el estado (con el cursor): o quedan los
     * dos o ninguno. Si cambió el día comercial ([previousDayEndsAt] distinto del de [state]),
     * olvida los avisados de días anteriores.
     */
    @Transaction
    suspend fun commit(
        state: ReadyAlertsStateEntity,
        shown: List<ReadyAlertSeenEntity>,
        previousDayEndsAt: Long?,
    ) {
        if (previousDayEndsAt != null && previousDayEndsAt != state.dayEndsAt) {
            forgetSeenBefore(state.dayEndsAt)
        }
        if (shown.isNotEmpty()) {
            markSeen(shown)
        }
        saveState(state)
    }

    /** «Borrar los datos de este dispositivo». */
    @Transaction
    suspend fun eraseAll() {
        deleteAllSeen()
        deleteState()
    }
}
