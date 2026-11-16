package local.adel.pos.android.core.data.db

import androidx.room.ColumnInfo
import androidx.room.Dao
import androidx.room.Entity
import androidx.room.PrimaryKey
import androidx.room.Query
import androidx.room.Upsert
import kotlinx.coroutines.flow.Flow

/**
 * Preferencia de este dispositivo (no de un mesero), por ejemplo la vista de mesas lista o mapa.
 * Clave y valor de texto: una preferencia nueva no necesita migración. Nada sensible va aquí (las
 * credenciales van cifradas con el Keystore, fases B y C).
 */
@Entity(tableName = "device_preferences")
data class DevicePreference(
    @PrimaryKey val key: String,
    val value: String,
    @ColumnInfo(name = "updated_at_millis") val updatedAtMillis: Long,
)

@Dao
interface DevicePreferenceDao {
    @Query("SELECT value FROM device_preferences WHERE `key` = :key")
    fun observe(key: String): Flow<String?>

    @Query("SELECT value FROM device_preferences WHERE `key` = :key")
    suspend fun get(key: String): String?

    @Upsert suspend fun put(preference: DevicePreference)

    @Query("DELETE FROM device_preferences WHERE `key` = :key") suspend fun remove(key: String)
}
