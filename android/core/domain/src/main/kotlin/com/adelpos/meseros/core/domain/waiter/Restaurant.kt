package com.adelpos.meseros.core.domain.waiter

import com.adelpos.meseros.core.domain.money.Money
import com.adelpos.meseros.core.domain.table.TableState

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
 * de 1000 × 560 por área (restricciones de la migración 0004). Desde la etapa A3 el mapa se escala
 * por el rectángulo que ocupan las mesas del área ([TableMapFit]), no por el lienzo completo.
 */
object TableCanvas {
    const val WIDTH = 1000
    const val HEIGHT = 560
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

/**
 * Nombres cortos de las mesas de un área para el mapa del teléfono (observación F4 de la Fase F):
 * con muchas mesas, cada una mide pocos puntos y «Mesa F12» quedaba como «M…». Se quitan las
 * palabras iniciales que comparten **todas** las mesas del área («Mesa F1» → «F1»); si alguna
 * quedaría vacía o no hay palabras comunes, se conserva el nombre completo. El nombre completo
 * sigue en la descripción para la accesibilidad.
 */
object TableNames {
    fun compact(tables: List<DiningTable>): Map<String, String> {
        val words = tables.map { it.name.trim().split(SPACES) }
        val shared =
            words.firstOrNull().orEmpty().indices.takeWhile { index ->
                words.all { name -> name.size > index + 1 && name[index] == words.first()[index] }
            }
        return tables.zip(words).associate { (table, name) ->
            table.id to
                if (shared.isEmpty()) {
                    table.name
                } else {
                    name.drop(shared.size).joinToString(" ")
                }
        }
    }

    private val SPACES = Regex("\\s+")
}
