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
    /** Etapa M1 (§1): la unión abierta en que está (`union_id`), o null. */
    val unionId: String? = null,
    /** Etapa M1 (§3): el grupo predefinido del que es sección (`group_id`), o null. */
    val groupId: String? = null,
)

/**
 * Etapa M1 (§1, contrato §2): una unión abierta de mesas (`TableUnion`), que se comporta como una
 * sola mesa. Go calcula todo: el nombre («Mesa 5 + 6», M1-D2), la capacidad sumada, el estado y el
 * saldo de sus cuentas; la aplicación lo muestra tal cual.
 *
 * @param tableIds sus mesas en el orden del nombre.
 * @param state el de sus cuentas (nunca libre: una unión abierta siempre tiene cuentas); null si
 *   esta versión no conoce el que manda Go.
 */
data class TableUnion(
    val id: String,
    val name: String,
    val areaId: String,
    val groupId: String?,
    val tableIds: List<String>,
    val seats: Int,
    val state: TableState?,
    val accountCount: Int,
    val balance: Money,
    val occupiedSeconds: Long,
    val longOpen: Boolean,
    val revision: Int,
)

/**
 * Etapa M1 (§3): una mesa divisible (`TableGroup`), por ejemplo «Mesa 3» con sus secciones 3A, 3B y
 * 3C. Cada sección es una mesa normal; el grupo solo se dibuja junto en el mapa y su atajo une
 * todas.
 */
data class TableGroup(
    val id: String,
    val areaId: String,
    val name: String,
    val tableIds: List<String>,
    val seats: Int,
    val revision: Int,
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
    /** Etapa M1: las uniones abiertas (`unions`); vacía con un servidor anterior. */
    val unions: List<TableUnion> = emptyList(),
    /** Etapa M1: los grupos activos (`groups`); vacía con un servidor anterior. */
    val groups: List<TableGroup> = emptyList(),
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

    /** Etapa M1: la unión abierta en que está la mesa [tableId], o null. */
    fun unionOf(tableId: String): TableUnion? =
        unions.firstOrNull { tableId in it.tableIds }
            ?: table(tableId)?.unionId?.let { id -> unions.firstOrNull { it.id == id } }

    fun group(id: String): TableGroup? = groups.firstOrNull { it.id == id }

    fun groupsIn(areaId: String): List<TableGroup> = groups.filter { it.areaId == areaId }

    fun unionsIn(areaId: String): List<TableUnion> = unions.filter { it.areaId == areaId }

    /**
     * Etapa M1 (§1.2): la lista de un área con **una sola entrada por unión** (en el lugar de su
     * primera mesa) y las mesas que no están unidas, en su orden.
     */
    fun entriesIn(areaId: String): List<TablesEntry> {
        val shown = mutableSetOf<String>()
        val entries = mutableListOf<TablesEntry>()
        for (table in tablesIn(areaId)) {
            val union = unionOf(table.id)
            if (union == null) {
                entries += TablesEntry.Single(table)
            } else if (shown.add(union.id)) {
                entries += TablesEntry.Joined(union, union.tableIds.mapNotNull(::table))
            }
        }
        return entries
    }

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

/** Una entrada de la lista de Mesas (etapa M1): una mesa sola o una unión con sus mesas. */
sealed interface TablesEntry {
    val key: String

    data class Single(val table: DiningTable) : TablesEntry {
        override val key: String
            get() = table.id
    }

    data class Joined(val union: TableUnion, val tables: List<DiningTable>) : TablesEntry {
        override val key: String
            get() = "union-${union.id}"
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
