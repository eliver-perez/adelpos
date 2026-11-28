package com.adelpos.meseros.core.domain.waiter

import com.adelpos.meseros.core.domain.money.Money
import com.adelpos.meseros.core.domain.table.TableState
import org.junit.Assert.assertEquals
import org.junit.Assert.assertNull
import org.junit.Test

/**
 * Etapa M1 (§1 y §3, contrato §10): lo que la aplicación muestra de las uniones, sin decidir nada
 * (Go da el nombre, la capacidad y el estado): una sola entrada por unión en la lista, el contorno
 * con la misma escala del mapa, qué mesas se ofrecen en «Unir con…» y el texto de la mesa en una
 * frase (la regla de `documents.TableContext`).
 */
class StageM1UnionsTest {
    private fun table(
        id: String,
        name: String,
        x: Int,
        state: TableState = TableState.FREE,
        unionId: String? = null,
        outOfService: Boolean = false,
    ) =
        WaiterFixtures.table(id, name)
            .copy(x = x, y = 40, state = state, unionId = unionId, outOfService = outOfService)

    private val union =
        TableUnion(
            id = "u1",
            name = "Mesa 5 + 6",
            areaId = "area-1",
            groupId = null,
            tableIds = listOf("t5", "t6"),
            seats = 8,
            state = TableState.OCCUPIED,
            accountCount = 1,
            balance = Money("0.00"),
            occupiedSeconds = 60,
            longOpen = false,
            revision = 2,
        )

    private val view =
        RestaurantView(
            areas = listOf(DiningArea("area-1", "Salón", 1, true)),
            tables =
                listOf(
                    table("t5", "Mesa 5", 40, TableState.OCCUPIED, unionId = "u1"),
                    table("t6", "Mesa 6", 160, TableState.OCCUPIED, unionId = "u1"),
                    table("t7", "Mesa 7", 280, TableState.OCCUPIED),
                    table("t8", "Mesa 8", 400),
                    table("t9", "Mesa 9", 520, outOfService = true),
                ),
            settings = null,
            readAtElapsedMillis = 0,
            unions = listOf(union),
        )

    @Test
    fun laListaMuestraUnaSolaEntradaPorUnion() {
        val entries = view.entriesIn("area-1")
        assertEquals(
            listOf("union-u1", "t7", "t8", "t9"),
            entries.map { it.key },
        )
        val joined = entries.first() as TablesEntry.Joined
        assertEquals(listOf("t5", "t6"), joined.tables.map { it.id })
        assertEquals(union, view.unionOf("t6"))
        assertNull(view.unionOf("t7"))
    }

    @Test
    fun unirConOfreceLasMesasDelAreaConSuMotivo() {
        val choices = UnionChoices.of(view, "area-1", exclude = setOf("t8"))
        assertEquals(
            mapOf(
                "t5" to UnionChoices.Unavailable.IN_UNION,
                "t6" to UnionChoices.Unavailable.IN_UNION,
                "t7" to UnionChoices.Unavailable.OCCUPIED,
                "t9" to UnionChoices.Unavailable.OUT_OF_SERVICE,
            ),
            choices.associate { it.table.id to it.unavailable },
        )
        val fromUnion = UnionChoices.of(view, "area-1", exclude = union.tableIds.toSet())
        assertEquals(null, fromUnion.single { it.table.id == "t8" }.unavailable)
    }

    @Test
    fun elContornoEnvuelveSusMesasConLaEscalaDelMapa() {
        val tables = view.tablesIn("area-1")
        val fit = requireNotNull(TableMapFit.of(tables, 900f, 400f))
        val members = listOf(view.table("t5")!!, view.table("t6")!!)
        val box = requireNotNull(UnionOutline.of(fit, members, margin = 6f))
        assertEquals(fit.left(members[0]) - 6f, box.left, 0.01f)
        assertEquals(fit.top(members[0]) - 6f, box.top, 0.01f)
        val right = fit.left(members[1]) + members[1].width * fit.scale + 6f
        assertEquals(right - box.left, box.width, 0.01f)
        assertNull(UnionOutline.of(fit, emptyList(), margin = 6f))
    }

    @Test
    fun laUnionEnPantallaSabeSusMesas() {
        val pending = PendingUnion("t8", listOf("t9"), null, "Mesa 8 + 9", 8)
        assertEquals(listOf("t8", "t9"), pending.allTableIds(view))
        val grouped =
            view.copy(
                groups = listOf(TableGroup("g3", "area-1", "Mesa 3", listOf("t8", "t9"), 8, 1)),
            )
        assertEquals(
            listOf("t9", "t8"),
            PendingUnion("t9", emptyList(), "g3", "Mesa 3", 8).allTableIds(grouped),
        )
    }

    @Test
    fun elNombreDeLaMesaEnUnaFraseComoGo() {
        assertEquals("Mesa 5", TableContext.of("Mesa 5"))
        assertEquals("Mesa 5", TableContext.of("5"))
        assertEquals("mesa 3a", TableContext.of("mesa 3a"))
        assertEquals("Mésa 2", TableContext.of("Mésa 2"))
        assertEquals("Mesa Mesanine", TableContext.of("Mesanine"))
        assertEquals("Mesa Terraza 1", TableContext.of("Terraza 1"))
    }

    @Test
    fun unaCapturaConUnionEnPantallaNoSeEnviaSinCuenta() {
        val capture =
            WaiterFixtures.capture(accountId = null)
                .copy(unionJoin = UnionJoin(listOf("t9"), null, "Mesa 8 + 9", "Mesa 8"))
        assertEquals(true, capture.waitsForUnion)
        assertEquals(false, capture.copy(accountId = "cuenta").waitsForUnion)
        assertEquals(
            "el guardado sin respuesta lleva la unión para repetirse igual",
            capture.unionJoin,
            capture.copy(pendingOperationId = "op").unresolvedSave()?.unionJoin,
        )
    }
}
