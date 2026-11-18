package com.adelpos.meseros.core.data.catalog

import android.content.Context
import android.database.sqlite.SQLiteConstraintException
import androidx.room.Room
import androidx.room.testing.MigrationTestHelper
import androidx.test.ext.junit.runners.AndroidJUnit4
import androidx.test.platform.app.InstrumentationRegistry
import com.adelpos.meseros.core.data.catalog.db.CatalogDatabase
import kotlinx.coroutines.runBlocking
import org.junit.After
import org.junit.Assert.assertEquals
import org.junit.Assert.fail
import org.junit.Rule
import org.junit.Test
import org.junit.runner.RunWith

/**
 * catalog.db en el dispositivo: el esquema 1 que crea Room es el exportado
 * (`core/data/schemas/…CatalogDatabase/1.json`) y una descarga completa reemplaza el catálogo en
 * una sola transacción (si falla a la mitad, el anterior queda intacto y sobrevive a reabrir la
 * base). Cada versión nueva del esquema agrega aquí su caso de migración.
 */
@RunWith(AndroidJUnit4::class)
class CatalogDatabaseMigrationTest {
    private val testDatabase = "catalog-migration-test.db"
    private val context: Context = InstrumentationRegistry.getInstrumentation().targetContext

    @get:Rule
    val helper =
        MigrationTestHelper(
            InstrumentationRegistry.getInstrumentation(),
            CatalogDatabase::class.java,
        )

    @After
    fun deleteDatabase() {
        context.deleteDatabase(testDatabase)
    }

    @Test
    fun laVersion1CoincideConElEsquemaExportado() {
        helper.createDatabase(testDatabase, 1).use { database ->
            database.execSQL(
                "INSERT INTO catalog_state (id, version, epoch, generated_at, timezone, tax_rate, " +
                    "price_list_id, price_list_name, synced_at_millis) " +
                    "VALUES (1, 41, 'e', '', 'America/Monterrey', 16, 'l', 'General', 1)",
            )
        }
        helper.runMigrationsAndValidate(testDatabase, 1, true, *CatalogDatabase.MIGRATIONS).close()
    }

    @Test
    fun laDescargaCompletaReemplazaTodoEnUnaSolaTransaccion() = runBlocking {
        helper.createDatabase(testDatabase, 1).close()
        val first = open()
        try {
            val store = CatalogStore(first)
            store.save(CatalogPayload.parse(CatalogJsonSamples.full(version = 41, products = 3), 1))
            // Un identificador repetido falla a mitad del reemplazo: nada de lo nuevo queda.
            try {
                store.save(
                    CatalogPayload.parse(
                        CatalogJsonSamples.full(version = 50, products = 5, duplicateLast = true),
                        2,
                    ),
                )
                fail("La descarga con un identificador repetido debía fallar")
            } catch (expected: SQLiteConstraintException) {
                // La transacción se deshizo.
            }
            assertEquals(41L, store.state()?.version)
            assertEquals(3, store.productCount())
            // Un reemplazo correcto deja solo lo nuevo.
            store.save(CatalogPayload.parse(CatalogJsonSamples.full(version = 52, products = 2), 3))
            assertEquals(2, store.productCount())
        } finally {
            first.close()
        }
        // Al reabrir (otro proceso) sigue el último catálogo guardado.
        val reopened = open()
        try {
            val catalog = CatalogStore(reopened).load()
            assertEquals(52L, catalog.version?.version)
            assertEquals(listOf("Producto 1", "Producto 2"), catalog.products.map { it.name })
            assertEquals(1, reopened.openHelper.readableDatabase.version)
        } finally {
            reopened.close()
        }
    }

    private fun open(): CatalogDatabase =
        Room.databaseBuilder(context, CatalogDatabase::class.java, testDatabase)
            .addMigrations(*CatalogDatabase.MIGRATIONS)
            .build()
}
