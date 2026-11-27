package com.adelpos.meseros.core.data.waiter.db

import androidx.room.ColumnInfo
import androidx.room.Dao
import androidx.room.Entity
import androidx.room.Index
import androidx.room.Insert
import androidx.room.OnConflictStrategy
import androidx.room.PrimaryKey
import androidx.room.Query
import androidx.room.Transaction
import androidx.room.Update
import androidx.room.Upsert
import kotlinx.coroutines.flow.Flow

/**
 * Captura en curso de un mesero (el «Borrador» de §7). Las listas (borrador de Go, toques locales,
 * renglones propios, precios vistos) se guardan como documentos JSON con la forma del contrato
 * (`WaiterStorage`); las columnas sueltas son las que se consultan.
 */
@Entity(tableName = "captures", indices = [Index(value = ["waiter_user_id"])])
data class CaptureEntity(
    @PrimaryKey val id: String,
    @ColumnInfo(name = "waiter_user_id") val waiterUserId: String,
    /** v6: null en un pedido para llevar (antes `NOT NULL`). */
    @ColumnInfo(name = "table_id") val tableId: String?,
    @ColumnInfo(name = "table_name") val tableName: String,
    @ColumnInfo(name = "account_id") val accountId: String?,
    @ColumnInfo(name = "account_name") val accountName: String?,
    @ColumnInfo(name = "seen_account_ids") val seenAccountIds: String,
    val base: String,
    @ColumnInfo(name = "base_revision") val baseRevision: Int,
    val operations: String,
    @ColumnInfo(name = "own_item_ids") val ownItemIds: String,
    @ColumnInfo(name = "seen_prices") val seenPrices: String,
    val sync: String,
    @ColumnInfo(name = "sync_message") val syncMessage: String?,
    @ColumnInfo(name = "table_accounts") val tableAccounts: String,
    @ColumnInfo(name = "pending_operation_id") val pendingOperationId: String?,
    @ColumnInfo(name = "in_flight_count") val inFlightCount: Int,
    @ColumnInfo(name = "created_at") val createdAt: Long,
    @ColumnInfo(name = "updated_at") val updatedAt: Long,
    /** v5: la cuenta nació en Go con esta captura (revisión R6, A2). */
    @ColumnInfo(name = "created_account", defaultValue = "0") val createdAccount: Boolean = false,
    /** v6 (etapa A2): `dine_in` (una mesa) o `takeaway` (para llevar). */
    @ColumnInfo(name = "mode", defaultValue = "'dine_in'") val mode: String = "dine_in",
    /** v6: el nombre del cliente de un pedido para llevar mientras no sale a Go (A2-D1). */
    @ColumnInfo(name = "customer_name", defaultValue = "''") val customerName: String = "",
    /** v6: el teléfono, igual que [customerName]. */
    @ColumnInfo(name = "customer_phone", defaultValue = "''") val customerPhone: String = "",
    /**
     * v7 (etapa M1): la unión que veía el mesero y la unión en pantalla que nace con el primer
     * producto (JSON); null en lo guardado antes.
     */
    @ColumnInfo(name = "union_view") val unionView: String? = null,
)

/**
 * Edición del nombre y el teléfono de un pedido para llevar que todavía no está en Go (v6, A2-D1):
 * una fila por pedido, la última edición. Se envía con la sesión del mesero que la hizo y se borra
 * solo cuando Go la confirma (con la misma [operationId]) o la rechaza para siempre.
 *
 * @param key el pedido: la cuenta de Go si se conoce; si no, la captura o el pedido de la cola.
 * @param accountId la cuenta de Go en cuanto se sabe.
 */
@Entity(tableName = "takeaway_customer_edits", indices = [Index(value = ["account_id"])])
data class CustomerEditEntity(
    @PrimaryKey val key: String,
    @ColumnInfo(name = "account_id") val accountId: String?,
    @ColumnInfo(name = "waiter_user_id") val waiterUserId: String,
    @ColumnInfo(name = "customer_name") val customerName: String,
    val phone: String,
    @ColumnInfo(name = "operation_id") val operationId: String,
    @ColumnInfo(name = "updated_at") val updatedAt: Long,
)

/**
 * Pedido de la cola (§7): **nunca** se borra por cerrar la aplicación, reiniciar el dispositivo o
 * perder la batería; cada cambio de estado se escribe en una transacción y **solo si el pedido
 * sigue en el estado que se leyó** ([WaiterDao.transition]; revisión R6, M1 y O1). [operationId] es
 * la clave de operación, la misma en todos los reenvíos.
 */
@Entity(
    tableName = "pending_orders",
    indices = [Index(value = ["waiter_user_id"]), Index(value = ["state"])],
)
data class PendingOrderEntity(
    @PrimaryKey @ColumnInfo(name = "operation_id") val operationId: String,
    @ColumnInfo(name = "waiter_user_id") val waiterUserId: String,
    @ColumnInfo(name = "waiter_name") val waiterName: String,
    val state: String,
    val destination: String,
    val lines: String,
    val replaces: String,
    @ColumnInfo(name = "catalog_version") val catalogVersion: Long,
    @ColumnInfo(name = "catalog_epoch") val catalogEpoch: String?,
    @ColumnInfo(name = "captured_at") val capturedAt: Long,
    val confirmation: String?,
    val review: String?,
    val attempts: Int,
    @ColumnInfo(name = "last_error") val lastError: String?,
    @ColumnInfo(name = "credential_rejected") val credentialRejected: Boolean,
    @ColumnInfo(name = "online_confirm") val onlineConfirm: String?,
    @ColumnInfo(name = "received_account_id") val receivedAccountId: String?,
    @ColumnInfo(name = "received_at") val receivedAt: Long?,
    @ColumnInfo(name = "updated_at") val updatedAt: Long,
    /** v5: el guardado del borrador que quedó sin respuesta al enviarlo (JSON; revisión R6, C1). */
    @ColumnInfo(name = "unresolved_save") val unresolvedSave: String? = null,
    /** v5: la hora de captura ya está en la hora de Go (revisión R6, B4). */
    @ColumnInfo(name = "clock_corrected", defaultValue = "0") val clockCorrected: Boolean = false,
)

/**
 * Lo último que se leyó de Go para mostrar sin conexión: mesas (`overview`) y cuentas abiertas
 * (`accounts`), como documentos JSON del contrato.
 */
@Entity(tableName = "restaurant_snapshot")
data class RestaurantSnapshotEntity(
    @PrimaryKey val kind: String,
    val document: String,
    @ColumnInfo(name = "read_at") val readAt: Long,
)

/** Permisos vigentes de un mesero en su última lectura de `restaurant/state` (JSON). */
@Entity(tableName = "waiter_permissions")
data class WaiterPermissionsEntity(
    @PrimaryKey @ColumnInfo(name = "user_id") val userId: String,
    val permissions: String,
    @ColumnInfo(name = "read_at") val readAt: Long,
)

/**
 * SQL de las tablas del modo Mesero en la versión 4 de `AppDatabase` (`MIGRATION_3_4`): el que
 * genera Room para el esquema exportado (`schemas/…AppDatabase/4.json`); la prueba de la migración
 * compara el resultado con ese esquema.
 */
internal val WAITER_TABLES_V4: List<String> =
    listOf(
        "CREATE TABLE IF NOT EXISTS `captures` (`id` TEXT NOT NULL, " +
            "`waiter_user_id` TEXT NOT NULL, `table_id` TEXT NOT NULL, " +
            "`table_name` TEXT NOT NULL, `account_id` TEXT, `account_name` TEXT, " +
            "`seen_account_ids` TEXT NOT NULL, `base` TEXT NOT NULL, " +
            "`base_revision` INTEGER NOT NULL, `operations` TEXT NOT NULL, " +
            "`own_item_ids` TEXT NOT NULL, `seen_prices` TEXT NOT NULL, " +
            "`sync` TEXT NOT NULL, `sync_message` TEXT, " +
            "`table_accounts` TEXT NOT NULL, `pending_operation_id` TEXT, " +
            "`in_flight_count` INTEGER NOT NULL, `created_at` INTEGER NOT NULL, " +
            "`updated_at` INTEGER NOT NULL, PRIMARY KEY(`id`))",
        "CREATE INDEX IF NOT EXISTS `index_captures_waiter_user_id` ON `captures` (`waiter_user_id`)",
        "CREATE TABLE IF NOT EXISTS `pending_orders` (`operation_id` TEXT NOT NULL, " +
            "`waiter_user_id` TEXT NOT NULL, `waiter_name` TEXT NOT NULL, " +
            "`state` TEXT NOT NULL, `destination` TEXT NOT NULL, " +
            "`lines` TEXT NOT NULL, `replaces` TEXT NOT NULL, " +
            "`catalog_version` INTEGER NOT NULL, `catalog_epoch` TEXT, " +
            "`captured_at` INTEGER NOT NULL, `confirmation` TEXT, `review` TEXT, " +
            "`attempts` INTEGER NOT NULL, `last_error` TEXT, " +
            "`credential_rejected` INTEGER NOT NULL, `online_confirm` TEXT, " +
            "`received_account_id` TEXT, `received_at` INTEGER, " +
            "`updated_at` INTEGER NOT NULL, PRIMARY KEY(`operation_id`))",
        "CREATE INDEX IF NOT EXISTS `index_pending_orders_waiter_user_id` ON `pending_orders` (`waiter_user_id`)",
        "CREATE INDEX IF NOT EXISTS `index_pending_orders_state` ON `pending_orders` (`state`)",
        "CREATE TABLE IF NOT EXISTS `restaurant_snapshot` (`kind` TEXT NOT NULL, " +
            "`document` TEXT NOT NULL, `read_at` INTEGER NOT NULL, " +
            "PRIMARY KEY(`kind`))",
        "CREATE TABLE IF NOT EXISTS `waiter_permissions` (`user_id` TEXT NOT NULL, " +
            "`permissions` TEXT NOT NULL, `read_at` INTEGER NOT NULL, " +
            "PRIMARY KEY(`user_id`))",
    )

/**
 * Columnas nuevas de la versión 5 (`MIGRATION_4_5`, revisión R6): solo se agregan, con su valor por
 * omisión; las filas que ya había no cambian.
 */
internal val WAITER_COLUMNS_V5: List<String> =
    listOf(
        "ALTER TABLE `captures` ADD COLUMN `created_account` INTEGER NOT NULL DEFAULT 0",
        "ALTER TABLE `pending_orders` ADD COLUMN `unresolved_save` TEXT",
        "ALTER TABLE `pending_orders` ADD COLUMN `clock_corrected` INTEGER NOT NULL DEFAULT 0",
    )

/**
 * Cambios de la versión 6 (`MIGRATION_5_6`, etapa A2, Para llevar): `captures` pasa a admitir
 * capturas sin mesa y guarda la modalidad y los datos del cliente. SQLite de Android 8 no cambia
 * una columna a opcional: se crea la tabla nueva, se copian las filas (todas de salón) y se cambia
 * el nombre, como indica Room. Después, la tabla de ediciones del cliente por enviar. El SQL es el
 * que genera Room para `schemas/…AppDatabase/6.json`.
 */
internal val WAITER_CHANGES_V6: List<String> =
    listOf(
        "CREATE TABLE IF NOT EXISTS `captures_v6` (`id` TEXT NOT NULL, " +
            "`waiter_user_id` TEXT NOT NULL, `table_id` TEXT, `table_name` TEXT NOT NULL, " +
            "`account_id` TEXT, `account_name` TEXT, `seen_account_ids` TEXT NOT NULL, " +
            "`base` TEXT NOT NULL, `base_revision` INTEGER NOT NULL, " +
            "`operations` TEXT NOT NULL, `own_item_ids` TEXT NOT NULL, " +
            "`seen_prices` TEXT NOT NULL, `sync` TEXT NOT NULL, `sync_message` TEXT, " +
            "`table_accounts` TEXT NOT NULL, `pending_operation_id` TEXT, " +
            "`in_flight_count` INTEGER NOT NULL, `created_at` INTEGER NOT NULL, " +
            "`updated_at` INTEGER NOT NULL, " +
            "`created_account` INTEGER NOT NULL DEFAULT 0, " +
            "`mode` TEXT NOT NULL DEFAULT 'dine_in', " +
            "`customer_name` TEXT NOT NULL DEFAULT '', " +
            "`customer_phone` TEXT NOT NULL DEFAULT '', PRIMARY KEY(`id`))",
        "INSERT INTO `captures_v6` (`id`, `waiter_user_id`, `table_id`, `table_name`, " +
            "`account_id`, `account_name`, `seen_account_ids`, `base`, `base_revision`, " +
            "`operations`, `own_item_ids`, `seen_prices`, `sync`, `sync_message`, " +
            "`table_accounts`, `pending_operation_id`, `in_flight_count`, `created_at`, " +
            "`updated_at`, `created_account`, `mode`, `customer_name`, `customer_phone`) " +
            "SELECT `id`, `waiter_user_id`, `table_id`, `table_name`, `account_id`, " +
            "`account_name`, `seen_account_ids`, `base`, `base_revision`, `operations`, " +
            "`own_item_ids`, `seen_prices`, `sync`, `sync_message`, `table_accounts`, " +
            "`pending_operation_id`, `in_flight_count`, `created_at`, `updated_at`, " +
            "`created_account`, 'dine_in', '', '' FROM `captures`",
        "DROP TABLE `captures`",
        "ALTER TABLE `captures_v6` RENAME TO `captures`",
        "CREATE INDEX IF NOT EXISTS `index_captures_waiter_user_id` ON `captures` (`waiter_user_id`)",
        "CREATE TABLE IF NOT EXISTS `takeaway_customer_edits` (`key` TEXT NOT NULL, " +
            "`account_id` TEXT, `waiter_user_id` TEXT NOT NULL, " +
            "`customer_name` TEXT NOT NULL, `phone` TEXT NOT NULL, " +
            "`operation_id` TEXT NOT NULL, `updated_at` INTEGER NOT NULL, " +
            "PRIMARY KEY(`key`))",
        "CREATE INDEX IF NOT EXISTS `index_takeaway_customer_edits_account_id` " +
            "ON `takeaway_customer_edits` (`account_id`)",
    )

@Dao
interface WaiterDao {
    // --- Capturas ---

    @Query("SELECT * FROM captures WHERE waiter_user_id = :userId ORDER BY created_at")
    fun observeCaptures(userId: String): Flow<List<CaptureEntity>>

    @Query("SELECT * FROM captures WHERE id = :id")
    fun observeCapture(id: String): Flow<CaptureEntity?>

    @Query("SELECT * FROM captures WHERE id = :id") suspend fun capture(id: String): CaptureEntity?

    @Query("SELECT * FROM captures WHERE waiter_user_id = :userId")
    suspend fun capturesOf(userId: String): List<CaptureEntity>

    @Query("SELECT * FROM captures") suspend fun allCaptures(): List<CaptureEntity>

    @Upsert suspend fun saveCapture(capture: CaptureEntity)

    @Query("DELETE FROM captures WHERE id = :id") suspend fun deleteCapture(id: String)

    // --- Cola ---

    @Query("SELECT * FROM pending_orders WHERE waiter_user_id = :userId ORDER BY captured_at")
    fun observeOrders(userId: String): Flow<List<PendingOrderEntity>>

    @Query("SELECT * FROM pending_orders WHERE operation_id = :operationId")
    fun observeOrder(operationId: String): Flow<PendingOrderEntity?>

    @Query("SELECT * FROM pending_orders WHERE operation_id = :operationId")
    suspend fun order(operationId: String): PendingOrderEntity?

    @Query("SELECT * FROM pending_orders WHERE state = :state ORDER BY captured_at")
    suspend fun ordersIn(state: String): List<PendingOrderEntity>

    @Query("SELECT * FROM pending_orders WHERE state IN (:states) ORDER BY captured_at")
    suspend fun ordersInAny(states: List<String>): List<PendingOrderEntity>

    @Query("SELECT * FROM pending_orders WHERE waiter_user_id = :userId")
    suspend fun ordersOf(userId: String): List<PendingOrderEntity>

    @Query("SELECT COUNT(*) FROM pending_orders WHERE state IN (:states)")
    fun observeCount(states: List<String>): Flow<Int>

    /** Los pedidos sin enviar de los demás meseros (para contarlos, nunca para mostrarlos). */
    @Query(
        "SELECT * FROM pending_orders WHERE waiter_user_id != :userId AND state IN (:states) " +
            "ORDER BY captured_at",
    )
    fun observeOthers(userId: String, states: List<String>): Flow<List<PendingOrderEntity>>

    @Query(
        "SELECT COUNT(*) FROM pending_orders WHERE waiter_user_id = :userId AND state IN (:states)",
    )
    fun observeCountOf(userId: String, states: List<String>): Flow<Int>

    @Insert(onConflict = OnConflictStrategy.ABORT)
    suspend fun insertOrder(order: PendingOrderEntity)

    /** Reescribe un pedido que **ya existe** (nunca lo vuelve a crear): filas actualizadas. */
    @Update suspend fun updateOrder(order: PendingOrderEntity): Int

    /**
     * Escribe el pedido **solo si sigue** en [expectedState] (revisión R6, M1 y O1): un pedido
     * borrado («Emparejar de nuevo», retiro) nunca vuelve, y un cambio que llegó primero (otro
     * envío, la recuperación al arrancar, el mesero) no se pisa. Devuelve si se escribió.
     */
    @Transaction
    suspend fun transition(order: PendingOrderEntity, expectedState: String): Boolean {
        val current = order(order.operationId) ?: return false
        if (current.state != expectedState) {
            return false
        }
        return updateOrder(order) == 1
    }

    @Query("DELETE FROM pending_orders WHERE operation_id = :operationId")
    suspend fun deleteOrder(operationId: String)

    /** Recibidos hace tiempo: ya no hace falta mostrarlos (Go los tiene). */
    @Query("DELETE FROM pending_orders WHERE state = :received AND received_at < :before")
    suspend fun pruneReceived(received: String, before: Long)

    /**
     * Pasa la captura a la cola en **una** transacción: el pedido existe o la captura sigue, nunca
     * ninguno de los dos ni los dos.
     */
    @Transaction
    suspend fun queueFromCapture(order: PendingOrderEntity, captureId: String) {
        insertOrder(order)
        deleteCapture(captureId)
    }

    /** «Corregir en la captura»: el pedido vuelve a ser borrador en una transacción. */
    @Transaction
    suspend fun captureFromOrder(capture: CaptureEntity, operationId: String) {
        saveCapture(capture)
        deleteOrder(operationId)
    }

    // --- Lo guardado de Go ---

    @Query("SELECT * FROM restaurant_snapshot WHERE kind = :kind")
    suspend fun snapshot(kind: String): RestaurantSnapshotEntity?

    @Upsert suspend fun saveSnapshot(snapshot: RestaurantSnapshotEntity)

    @Query("SELECT * FROM waiter_permissions WHERE user_id = :userId")
    suspend fun permissions(userId: String): WaiterPermissionsEntity?

    @Upsert suspend fun savePermissions(permissions: WaiterPermissionsEntity)

    // --- Ediciones del cliente de Para llevar (A2-D1) ---

    @Query("SELECT * FROM takeaway_customer_edits ORDER BY updated_at")
    fun observeCustomerEdits(): Flow<List<CustomerEditEntity>>

    @Query("SELECT * FROM takeaway_customer_edits ORDER BY updated_at")
    suspend fun customerEdits(): List<CustomerEditEntity>

    @Query(
        "SELECT * FROM takeaway_customer_edits WHERE `key` = :key " +
            "OR (:accountId IS NOT NULL AND account_id = :accountId) LIMIT 1",
    )
    suspend fun customerEditFor(key: String, accountId: String?): CustomerEditEntity?

    @Insert(onConflict = OnConflictStrategy.REPLACE)
    suspend fun insertCustomerEdit(edit: CustomerEditEntity)

    @Query("DELETE FROM takeaway_customer_edits WHERE `key` = :key")
    suspend fun deleteCustomerEdit(key: String)

    /**
     * Guarda la última edición de un pedido (la última gana): reemplaza la que hubiera con la misma
     * clave o la misma cuenta, en una transacción.
     */
    @Transaction
    suspend fun replaceCustomerEdit(edit: CustomerEditEntity) {
        val previous = customerEditFor(edit.key, edit.accountId)
        if (previous != null && previous.key != edit.key) {
            deleteCustomerEdit(previous.key)
        }
        insertCustomerEdit(edit)
    }

    /** Ya se sabe en qué cuenta de Go nació el pedido [key]. */
    @Query(
        "UPDATE takeaway_customer_edits SET account_id = :accountId " +
            "WHERE `key` = :key AND account_id IS NULL",
    )
    suspend fun bindCustomerEdit(key: String, accountId: String)

    /**
     * Go confirmó (o rechazó para siempre) **esta** edición: se borra solo si sigue siendo la misma
     * (una más reciente, con otra clave de operación, se queda para enviarse). Filas borradas.
     */
    @Query(
        "DELETE FROM takeaway_customer_edits WHERE `key` = :key AND operation_id = :operationId",
    )
    suspend fun finishCustomerEdit(key: String, operationId: String): Int

    // --- Borrar los datos de este dispositivo (retiro o «Emparejar de nuevo») ---

    @Query("DELETE FROM captures") suspend fun deleteAllCaptures()

    @Query("DELETE FROM takeaway_customer_edits") suspend fun deleteAllCustomerEdits()

    @Query("DELETE FROM pending_orders") suspend fun deleteAllOrders()

    @Query("DELETE FROM restaurant_snapshot") suspend fun deleteAllSnapshots()

    @Query("DELETE FROM waiter_permissions") suspend fun deleteAllPermissions()

    @Transaction
    suspend fun eraseAll() {
        deleteAllCaptures()
        deleteAllOrders()
        deleteAllSnapshots()
        deleteAllPermissions()
        deleteAllCustomerEdits()
    }
}
