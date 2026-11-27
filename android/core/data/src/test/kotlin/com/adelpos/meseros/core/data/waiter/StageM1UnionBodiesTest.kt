package com.adelpos.meseros.core.data.waiter

import com.adelpos.core.network.api.ApiJson
import com.adelpos.meseros.core.data.catalog.CustomizationJson
import com.adelpos.meseros.core.domain.waiter.OrderMode
import com.adelpos.meseros.core.domain.waiter.SeenUnion
import com.adelpos.meseros.core.domain.waiter.TakeawayCustomer
import com.adelpos.meseros.core.domain.waiter.UnionJoin
import kotlinx.serialization.json.jsonObject
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Test

/**
 * Etapa M1 (contrato §3 y §7): los cuerpos que salen con la unión y las lecturas de Go. Sin el dato
 * de la unión (lo guardado por la 0.4.1) `seen_union_id` no se escribe; «ninguna unión» se escribe
 * `null`; la unión en pantalla viaja como `union_table_ids` o `union_group_id`.
 */
class StageM1UnionBodiesTest {
    private fun openJson(seen: SeenUnion?, join: UnionJoin?) =
        CustomizationJson.parseToJsonElement(
                CustomizationJson.encodeToString(
                    OpenAccountBody.serializer(),
                    WaiterMapper.openBody(
                        operationId = "op",
                        mode = OrderMode.DINE_IN,
                        tableId = "t8",
                        seenAccountIds = emptyList(),
                        customer = TakeawayCustomer.NONE,
                        items = emptyList(),
                        seenUnion = seen,
                        unionJoin = join,
                    ),
                ),
            )
            .jsonObject

    @Test
    fun seenUnionIdSoloConElDato() {
        assertFalse("sin el dato, como la 0.4.1", "seen_union_id" in openJson(null, null))
        assertEquals("null", openJson(SeenUnion(null), null)["seen_union_id"].toString())
        assertEquals("\"u1\"", openJson(SeenUnion("u1"), null)["seen_union_id"].toString())
    }

    @Test
    fun laUnionEnPantallaViajaConLaCuenta() {
        val tables =
            openJson(SeenUnion(null), UnionJoin(listOf("t9"), null, "Mesa 8 + 9", "Mesa 8"))
        assertEquals("[\"t9\"]", tables["union_table_ids"].toString())
        assertFalse("union_group_id" in tables)
        val group = openJson(SeenUnion(null), UnionJoin(emptyList(), "g3", "Mesa 3", "Mesa 3a"))
        assertEquals("\"g3\"", group["union_group_id"].toString())
        assertFalse("union_table_ids" in group)
    }

    @Test
    fun lasLecturasDeGoConUnionesYGrupos() {
        val overview =
            ApiJson.decodeFromString(
                RestaurantOverviewDto.serializer(),
                """{"areas":[],"tables":[{"id":"t5","area_id":"a","name":"Mesa 5","state":"occupied",""" +
                    """"union_id":"u1","group_id":null}],"unions":[{"id":"u1","name":"Mesa 5 + 6",""" +
                    """"area_id":"a","group_id":null,"table_ids":["t5","t6"],"seats":8,""" +
                    """"state":"occupied","account_count":1,"balance":"0.00",""" +
                    """"occupied_since":"2026-11-27T10:00:00Z","occupied_seconds":60,""" +
                    """"long_open":false,"revision":3}],"groups":[{"id":"g3","area_id":"a",""" +
                    """"name":"Mesa 3","table_ids":["t3a","t3b"],"seats":8,"revision":1}]}""",
            )
        val union = requireNotNull(WaiterMapper.union(overview.unions!!.single()))
        assertEquals("Mesa 5 + 6", union.name)
        assertEquals(listOf("t5", "t6"), union.tableIds)
        assertEquals("u1", WaiterMapper.table(overview.tables.single())?.unionId)
        assertEquals("Mesa 3", WaiterMapper.group(overview.groups!!.single()).name)
        // Una unión deshecha no se dibuja.
        assertNull(WaiterMapper.union(overview.unions!!.single().copy(state = "dissolved")))
        // Un servidor anterior no manda uniones: listas vacías.
        val old =
            ApiJson.decodeFromString(
                RestaurantOverviewDto.serializer(),
                """{"areas":[],"tables":[]}""",
            )
        assertTrue(old.unions == null && old.groups == null)
    }

    @Test
    fun laColaMandaLaUnionQueVioElMesero() {
        val dto =
            OfflineDestinationDto(
                tableId = "t6",
                seenAccountIds = emptyList(),
                seenUnionId = WaiterMapper.seenUnionJson(SeenUnion("u1")),
            )
        val json =
            CustomizationJson.parseToJsonElement(
                    CustomizationJson.encodeToString(OfflineDestinationDto.serializer(), dto),
                )
                .jsonObject
        assertEquals("\"u1\"", json["seen_union_id"].toString())
        val legacy =
            CustomizationJson.encodeToString(
                OfflineDestinationDto.serializer(),
                OfflineDestinationDto(tableId = "t6", seenAccountIds = emptyList()),
            )
        assertFalse(legacy.contains("seen_union_id"))
    }
}
