package local.adel.pos.android.core.domain.waiter

import local.adel.pos.android.core.domain.money.Money
import local.adel.pos.android.core.domain.table.TableState

/**
 * Área del restaurante (`Area` de `getRestaurantOverview`): agrupa las mesas en la lista y tiene su
 * propio lienzo en el mapa.
 */
data class DiningArea(
    val id: String,
    val name: String,
    val sortOrder: Int,
    val active: Boolean,
)

/** Forma de la mesa en el mapa (`shape`). */
enum class TableShape(val wireName: String) {
    RECTANGLE("rectangle"),
    ROUND("round");

    companion object {
        /** Forma por su nombre en la API; un nombre desconocido se dibuja como rectángulo. */
        fun fromWireName(name: String?): TableShape =
            entries.firstOrNull { it.wireName == name } ?: RECTANGLE
    }
}

/**
 * Mesa con su estado y su tiempo, tal como los calcula Go (`Table` del contrato). La aplicación
 * solo los muestra.
 *
 * @param x posición y tamaño en el lienzo lógico de [TableCanvas] (1000 × 560 por área).
 * @param occupiedSeconds segundos desde que se abrió su cuenta más antigua, según la hora de Go en
 *   el momento de la lectura.
 * @param longOpen la cuenta lleva abierta más horas que el umbral del restaurante (lo decide Go).
 */
data class DiningTable(
    val id: String,
    val areaId: String,
    val name: String,
    val shape: TableShape,
    val x: Int,
    val y: Int,
    val width: Int,
    val height: Int,
    val seats: Int,
    val active: Boolean,
    val outOfService: Boolean,
    val accountCount: Int,
    val state: TableState,
    val occupiedSeconds: Long,
    val balance: Money,
    val longOpen: Boolean,
)

/** Umbrales de tiempo del restaurante (`settings` de `getRestaurantOverview`). */
data class RestaurantSettings(
    val warningMinutes: Int,
    val overdueMinutes: Int,
    val longOpenHours: Int,
)

/**
 * Lienzo lógico del mapa de mesas: Go guarda la posición y el tamaño de cada mesa en un rectángulo
 * de 1000 × 560 por área (restricciones de la migración 0004). La pantalla lo escala al espacio que
 * tiene, conservando la proporción.
 */
object TableCanvas {
    const val WIDTH = 1000
    const val HEIGHT = 560

    /**
     * Escala que hace caber el lienzo completo en [availableWidth] × [availableHeight] sin
     * deformarlo.
     */
    fun scaleToFit(availableWidth: Float, availableHeight: Float): Float {
        if (availableWidth <= 0f || availableHeight <= 0f) {
            return 0f
        }
        val horizontal = availableWidth / WIDTH
        val vertical = availableHeight / HEIGHT
        return minOf(horizontal, vertical)
    }
}

/**
 * Lo que el mesero ve en «Mesas»: áreas activas en su orden y sus mesas, más los umbrales.
 *
 * @param readAtElapsedMillis el reloj monótono del dispositivo al recibir la lectura de Go: el
 *   tiempo de cada mesa se muestra sumando lo que pasó desde entonces (sin conexión, la última
 *   lectura sigue contando).
 */
data class RestaurantView(
    val areas: List<DiningArea>,
    val tables: List<DiningTable>,
    val settings: RestaurantSettings?,
    val readAtElapsedMillis: Long,
) {
    /**
     * Mesas que se muestran en un área: las activas y las que aún tienen cuentas abiertas (una mesa
     * desactivada con cuenta abierta sigue a la vista hasta cerrarla), como `operationalTables` del
     * escritorio.
     */
    fun tablesIn(areaId: String): List<DiningTable> = tables.filter { table ->
        table.areaId == areaId && (table.active || table.accountCount > 0)
    }

    /** Áreas activas en su orden visual (`sort_order`, después el nombre). */
    val visibleAreas: List<DiningArea>
        get() = areas.filter { it.active }.sortedWith(compareBy({ it.sortOrder }, { it.name }))

    fun table(id: String): DiningTable? = tables.firstOrNull { it.id == id }

    companion object {
        val EMPTY =
            RestaurantView(
                areas = emptyList(),
                tables = emptyList(),
                settings = null,
                readAtElapsedMillis = 0L,
            )
    }
}

/** Búsqueda de mesa por nombre (§6.1), sin distinguir acentos ni mayúsculas. */
object TableSearch {
    fun matches(table: DiningTable, query: String): Boolean {
        val wanted = normalize(query)
        if (wanted.isEmpty()) {
            return true
        }
        val name = normalize(table.name)
        return wanted.split(' ').all { word -> name.contains(word) }
    }

    private fun normalize(text: String): String {
        val decomposed = java.text.Normalizer.normalize(text, java.text.Normalizer.Form.NFD)
        val withoutMarks = decomposed.replace(COMBINING_MARKS, "")
        return withoutMarks.lowercase().trim().replace(SPACES, " ")
    }

    private val COMBINING_MARKS = Regex("\\p{Mn}+")
    private val SPACES = Regex("\\s+")
}
