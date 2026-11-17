package local.adel.pos.android.core.data.db

import androidx.room.ColumnInfo
import androidx.room.Dao
import androidx.room.Entity
import androidx.room.Index
import androidx.room.Insert
import androidx.room.PrimaryKey
import androidx.room.Query
import androidx.room.Transaction
import androidx.room.Upsert

/**
 * El servidor del restaurante con el que está emparejado el dispositivo (una sola fila). La huella
 * SPKI y el `server_id` son públicos (los anuncia el propio servidor); la credencial del
 * dispositivo va **cifrada** con el Android Keystore (A1-D13): sin la llave del Keystore, que no
 * sale del dispositivo, no se puede leer.
 */
@Entity(tableName = "paired_server")
class PairedServerEntity(
    /** Siempre [SLOT]: hay como máximo un servidor emparejado. */
    @PrimaryKey val slot: Int = SLOT,
    @ColumnInfo(name = "server_id") val serverId: String,
    @ColumnInfo(name = "server_name") val serverName: String,
    /** Última dirección donde respondió (`https://host:puerto`); cambia con el cambio de IP. */
    @ColumnInfo(name = "base_url") val baseUrl: String,
    /** Huella SPKI fijada en el emparejamiento (SHA-256 en hexadecimal). */
    @ColumnInfo(name = "fingerprint_hex") val fingerprintHex: String,
    @ColumnInfo(name = "device_id") val deviceId: String,
    @ColumnInfo(name = "device_name") val deviceName: String,
    @ColumnInfo(name = "sealed_device_token", typeAffinity = ColumnInfo.BLOB)
    val sealedDeviceToken: ByteArray,
    /** Bloqueo por inactividad de este dispositivo (Administración › Terminales). */
    @ColumnInfo(name = "idle_lock_seconds") val idleLockSeconds: Int,
    @ColumnInfo(name = "paired_at_millis") val pairedAtMillis: Long,
) {
    companion object {
        const val SLOT = 1
    }
}

/**
 * Mesero que ya entró **con conexión** en este dispositivo (§3.2): su verificador local del PIN y
 * su credencial de envío (A1-D1), ambos cifrados con el Android Keystore y ligados a su `user_id`.
 * Lo demás (nombre, inicial, color y los intentos sin conexión) no es secreto.
 */
@Entity(tableName = "known_waiters", indices = [Index("server_id")])
class KnownWaiterEntity(
    @PrimaryKey @ColumnInfo(name = "user_id") val userId: String,
    @ColumnInfo(name = "server_id") val serverId: String,
    val name: String,
    val initial: String,
    val color: String?,
    @ColumnInfo(name = "pin_version") val pinVersion: Int,
    @ColumnInfo(name = "sealed_verifier", typeAffinity = ColumnInfo.BLOB)
    val sealedVerifier: ByteArray?,
    @ColumnInfo(name = "sealed_offline_credential", typeAffinity = ColumnInfo.BLOB)
    val sealedOfflineCredential: ByteArray?,
    @ColumnInfo(name = "last_online_entry_millis") val lastOnlineEntryMillis: Long,
    @ColumnInfo(name = "local_failures") val localFailures: Int,
    @ColumnInfo(name = "local_window_start_millis") val localWindowStartMillis: Long,
    @ColumnInfo(name = "local_locked_until_millis") val localLockedUntilMillis: Long?,
)

/**
 * Bitácora del dispositivo: lo que debe quedar a la vista aunque se borre todo lo demás, como los
 * pedidos que no se enviaron cuando el dispositivo fue retirado del restaurante (§3.2). Se conserva
 * hasta que alguien la borra en el dispositivo. Nunca guarda credenciales.
 */
@Entity(tableName = "device_log")
class DeviceLogEntry(
    @PrimaryKey(autoGenerate = true) val id: Long = 0,
    @ColumnInfo(name = "created_at_millis") val createdAtMillis: Long,
    val kind: String,
    @ColumnInfo(name = "waiter_name") val waiterName: String?,
    val description: String,
    @ColumnInfo(name = "captured_at_millis") val capturedAtMillis: Long?,
) {
    companion object {
        /** Un pedido sin enviar al retirar el dispositivo o emparejarlo de nuevo. */
        const val KIND_UNSENT_ORDER = "unsent_order"
    }
}

/** Acceso a lo del emparejamiento, los meseros conocidos y la bitácora. */
@Dao
abstract class AccessDao {
    @Query("SELECT * FROM paired_server WHERE slot = 1")
    abstract suspend fun pairedServer(): PairedServerEntity?

    @Upsert abstract suspend fun savePairedServer(server: PairedServerEntity)

    @Query("UPDATE paired_server SET base_url = :baseUrl WHERE slot = 1")
    abstract suspend fun updateBaseUrl(baseUrl: String)

    @Query(
        "UPDATE paired_server SET device_name = :deviceName, idle_lock_seconds = :idleLockSeconds " +
            "WHERE slot = 1",
    )
    abstract suspend fun updateDevice(deviceName: String, idleLockSeconds: Int)

    @Query("SELECT * FROM known_waiters ORDER BY name COLLATE NOCASE, user_id")
    abstract suspend fun knownWaiters(): List<KnownWaiterEntity>

    @Query("SELECT * FROM known_waiters WHERE user_id = :userId")
    abstract suspend fun knownWaiter(userId: String): KnownWaiterEntity?

    @Upsert abstract suspend fun saveKnownWaiter(waiter: KnownWaiterEntity)

    @Query("DELETE FROM known_waiters WHERE user_id IN (:userIds)")
    abstract suspend fun deleteKnownWaiters(userIds: List<String>)

    @Query(
        "UPDATE known_waiters SET local_failures = :failures, " +
            "local_window_start_millis = :windowStart, local_locked_until_millis = :lockedUntil " +
            "WHERE user_id = :userId",
    )
    abstract suspend fun updateLocalAttempts(
        userId: String,
        failures: Int,
        windowStart: Long,
        lockedUntil: Long?,
    )

    @Query("SELECT * FROM device_log ORDER BY id")
    abstract suspend fun deviceLog(): List<DeviceLogEntry>

    @Insert abstract suspend fun addToLog(entries: List<DeviceLogEntry>)

    @Query("DELETE FROM device_log") abstract suspend fun clearLog()

    @Query("DELETE FROM paired_server") protected abstract suspend fun deletePairedServer()

    @Query("DELETE FROM known_waiters") protected abstract suspend fun deleteAllKnownWaiters()

    /**
     * Borra todo lo del servidor emparejado (emparejamiento, meseros, verificadores y credenciales
     * cifradas) y anota antes en la bitácora [log]. Todo o nada: una sola transacción.
     */
    @Transaction
    open suspend fun forgetServer(log: List<DeviceLogEntry>) {
        if (log.isNotEmpty()) {
            addToLog(log)
        }
        deletePairedServer()
        deleteAllKnownWaiters()
    }
}
