package com.adelpos.cocina.db

import android.content.Context
import androidx.room.ColumnInfo
import androidx.room.Dao
import androidx.room.Database
import androidx.room.Entity
import androidx.room.PrimaryKey
import androidx.room.Query
import androidx.room.Room
import androidx.room.RoomDatabase
import androidx.room.Upsert

/**
 * El servidor emparejado de la pantalla de cocina: una sola fila. La credencial del dispositivo va
 * cifrada con el Android Keystore (`PairedDevice.CREDENTIAL_PURPOSE`); nunca en claro.
 */
@Entity(tableName = "paired_server")
class PairedServerEntity(
    @PrimaryKey val slot: Int = SINGLE_SLOT,
    @ColumnInfo(name = "server_id") val serverId: String,
    @ColumnInfo(name = "server_name") val serverName: String,
    @ColumnInfo(name = "base_url") val baseUrl: String,
    @ColumnInfo(name = "fingerprint_hex") val fingerprintHex: String,
    @ColumnInfo(name = "device_id") val deviceId: String,
    @ColumnInfo(name = "device_name") val deviceName: String,
    @ColumnInfo(name = "sealed_device_token") val sealedDeviceToken: ByteArray,
    @ColumnInfo(name = "paired_at_millis") val pairedAtMillis: Long,
) {
    // Nunca muestra la credencial (ni cifrada) en un registro.
    override fun toString(): String = "PairedServerEntity($serverId, $deviceId)"

    companion object {
        const val SINGLE_SLOT = 1
    }
}

@Dao
interface PairedServerDao {
    @Query("SELECT * FROM paired_server WHERE slot = ${PairedServerEntity.SINGLE_SLOT}")
    suspend fun pairedServer(): PairedServerEntity?

    @Upsert suspend fun save(server: PairedServerEntity)

    @Query("UPDATE paired_server SET base_url = :baseUrl") suspend fun moveTo(baseUrl: String)

    @Query("DELETE FROM paired_server") suspend fun forget()
}

/**
 * Base local de la aplicación de cocina (Room), aparte de la de meseros: otra aplicación, otro
 * archivo ([FILE_NAME]). El esquema de cada versión se exporta a `cocina/app/schemas/` y se
 * versiona.
 * - Versión 1 (etapa K1, Fase B): el servidor emparejado.
 *
 * Cada cambio sube la versión, agrega su `Migration` y una prueba con `MigrationTestHelper`. Nunca
 * `fallbackToDestructiveMigration`: perder la base obligaría a emparejar de nuevo.
 */
@Database(entities = [PairedServerEntity::class], version = 1, exportSchema = true)
abstract class KitchenDatabase : RoomDatabase() {
    abstract fun pairedServer(): PairedServerDao

    companion object {
        /** Archivo de la base en el dispositivo: un valor fijo, nunca derivado del paquete. */
        const val FILE_NAME = "cocina.db"

        fun open(context: Context): KitchenDatabase =
            Room.databaseBuilder(context, KitchenDatabase::class.java, FILE_NAME).build()
    }
}
