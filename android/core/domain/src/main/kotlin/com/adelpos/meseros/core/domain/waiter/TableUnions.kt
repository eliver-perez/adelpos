package com.adelpos.meseros.core.domain.waiter

import com.adelpos.meseros.core.domain.table.TableState
import kotlinx.coroutines.flow.StateFlow

/*
 * Etapa M1 (§1 y §3): unir mesas desde la tableta. Go decide todo (contrato §10): qué mesas se
 * pueden unir, el nombre, la capacidad y qué pasa con las cuentas. Aquí solo hay lo que la
 * aplicación muestra (qué mesas se ofrecen, dónde se dibuja el contorno) y la interfaz con Go.
 */

/** Lo que Go respondió a la vista previa de una unión (`previewTableUnion`). */
data class UnionPreview(
    val name: String,
    val seats: Int,
    val tableIds: List<String>,
    val problems: List<UnionProblem>,
) {
    /** Sin problemas: se puede unir. */
    val allowed: Boolean
        get() = problems.isEmpty()
}

/** Un problema de la vista previa, con el texto de Go tal cual (`message`). */
data class UnionProblem(val code: String, val message: String, val tableId: String?)

/** Cómo terminó una operación de uniones con Go. */
sealed interface UnionAnswer {
    data class Previewed(val preview: UnionPreview) : UnionAnswer

    /**
     * La unión quedó guardada (o se deshizo al quitar la penúltima mesa, [dissolved]): se vuelve a
     * leer Mesas.
     */
    data class Saved(val union: TableUnion?, val dissolved: Boolean) : UnionAnswer

    /**
     * `TABLE_UNION_DEFERRED`: todas las mesas están libres; la unión vive en la pantalla y nace con
     * el primer producto (M1-D9).
     */
    data object Deferred : UnionAnswer

    /** Go no lo permitió: su mensaje, tal cual. */
    data class Rejected(val message: String) : UnionAnswer

    /** Sin conexión o sin sesión de Go: unir mesas necesita conexión (M1-D13). */
    data object NoConnection : UnionAnswer
}

/**
 * Las uniones de mesas en Go, con la sesión del mesero activo y **nunca** en la cola (M1-D13): sin
 * conexión responden [UnionAnswer.NoConnection].
 */
interface TableUnionService {
    /** El servidor tiene la capacidad `table-unions-v1`; sin ella no se ofrece unir. */
    val available: StateFlow<Boolean>

    /** Vista previa de unir [tableIds] (la primera es la mesa de inicio). */
    suspend fun preview(tableIds: List<String>): UnionAnswer

    /** Vista previa del atajo de un grupo («Mesa 3»). */
    suspend fun previewGroup(groupId: String): UnionAnswer

    /** Vista previa de agregar [tableIds] a [unionId]. */
    suspend fun previewAdd(unionId: String, tableIds: List<String>): UnionAnswer

    /**
     * Une desde [startTableId] (ocupada: sus cuentas pasan a la unión) con [tableIds] o con todo el
     * grupo [groupId]. Con todas libres, Go responde [UnionAnswer.Deferred].
     */
    suspend fun create(startTableId: String, tableIds: List<String>, groupId: String?): UnionAnswer

    suspend fun addTable(union: TableUnion, tableId: String): UnionAnswer

    suspend fun removeTable(union: TableUnion, tableId: String): UnionAnswer

    companion object {
        /** Sin servidor que lo permita (pruebas y pantallas sin uniones). */
        val NONE: TableUnionService =
            object : TableUnionService {
                override val available: StateFlow<Boolean> =
                    kotlinx.coroutines.flow.MutableStateFlow(false)

                override suspend fun preview(tableIds: List<String>) = UnionAnswer.NoConnection

                override suspend fun previewGroup(groupId: String) = UnionAnswer.NoConnection

                override suspend fun previewAdd(unionId: String, tableIds: List<String>) =
                    UnionAnswer.NoConnection

                override suspend fun create(
                    startTableId: String,
                    tableIds: List<String>,
                    groupId: String?,
                ) = UnionAnswer.NoConnection

                override suspend fun addTable(union: TableUnion, tableId: String) =
                    UnionAnswer.NoConnection

                override suspend fun removeTable(union: TableUnion, tableId: String) =
                    UnionAnswer.NoConnection
            }
    }
}

/**
 * Una unión que vive solo en la pantalla (M1-D9): todas sus mesas estaban libres y nace en Go con
 * el primer producto (`union_table_ids` o `union_group_id` en `openAccount`). Salir sin capturar no
 * guarda nada.
 *
 * @param tableIds las otras mesas (sin la de inicio); vacía con [groupId].
 * @param name y [seats]: los de la vista previa de Go.
 */
data class PendingUnion(
    val startTableId: String,
    val tableIds: List<String>,
    val groupId: String?,
    val name: String,
    val seats: Int,
) {
    /** Todas sus mesas, la de inicio primero (las del grupo las sabe la vista). */
    fun allTableIds(view: RestaurantView): List<String> {
        val group = groupId?.let(view::group)
        val others = group?.tableIds?.filter { it != startTableId } ?: tableIds
        return listOf(startTableId) + others
    }
}

/**
 * Qué mesas se ofrecen en «Unir con…» (contrato §10): las del área de la mesa de inicio; las
 * ocupadas, unidas o fuera de servicio se ven deshabilitadas con su motivo. Solo presentación: lo
 * que vale es la vista previa de Go.
 */
object UnionChoices {
    enum class Unavailable {
        OCCUPIED,
        IN_UNION,
        OUT_OF_SERVICE,
    }

    data class Choice(val table: DiningTable, val unavailable: Unavailable?)

    /**
     * [exclude]: las mesas que ya están en la unión (o la de inicio). Las mesas van en el orden de
     * la lista del área.
     */
    fun of(view: RestaurantView, areaId: String, exclude: Set<String>): List<Choice> =
        view
            .tablesIn(areaId)
            .filter { it.id !in exclude }
            .map { table ->
                val unavailable =
                    when {
                        !table.active ||
                            table.outOfService ||
                            table.state == TableState.OUT_OF_SERVICE -> Unavailable.OUT_OF_SERVICE
                        view.unionOf(table.id) != null -> Unavailable.IN_UNION
                        table.state != TableState.FREE -> Unavailable.OCCUPIED
                        else -> null
                    }
                Choice(table, unavailable)
            }
}

/**
 * El contorno de una unión o el marco de un grupo en el mapa (contrato §10): el rectángulo que
 * envuelve a sus mesas con la misma escala de [TableMapFit], más un margen.
 */
object UnionOutline {
    data class Box(val left: Float, val top: Float, val width: Float, val height: Float)

    /** null si ninguna de sus mesas está en el mapa. */
    fun of(fit: TableMapFit.Fit, tables: List<DiningTable>, margin: Float): Box? {
        if (tables.isEmpty()) {
            return null
        }
        val left = tables.minOf { fit.left(it) } - margin
        val top = tables.minOf { fit.top(it) } - margin
        val right = tables.maxOf { fit.left(it) + it.width * fit.scale } + margin
        val bottom = tables.maxOf { fit.top(it) + it.height * fit.scale } + margin
        return Box(left, top, right - left, bottom - top)
    }
}

/**
 * El nombre de una mesa en una frase («la Mesa 5»), la regla de `documents.TableContext` de Go: sin
 * repetir «Mesa» si el nombre ya empieza con esa palabra (sin distinguir mayúsculas ni acentos); si
 * no, «Mesa » y el nombre. Solo para textos de la aplicación (M1-D13).
 */
object TableContext {
    fun of(name: String): String = if (includesWord(name)) name.trim() else "Mesa ${name.trim()}"

    private fun includesWord(name: String): Boolean {
        val folded =
            java.text.Normalizer.normalize(name.trim().lowercase(), java.text.Normalizer.Form.NFD)
                .replace(MARKS, "")
        if (!folded.startsWith(WORD)) {
            return false
        }
        val next = folded.drop(WORD.length).firstOrNull() ?: return true
        return !next.isLetter()
    }

    private const val WORD = "mesa"
    private val MARKS = Regex("\\p{Mn}+")
}
