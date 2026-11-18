package com.adelpos.meseros.core.domain.waiter

import com.adelpos.meseros.core.domain.money.Money
import com.adelpos.meseros.core.domain.waiter.WaiterFixtures.ACCOUNT
import com.adelpos.meseros.core.domain.waiter.WaiterFixtures.BURGER
import com.adelpos.meseros.core.domain.waiter.WaiterFixtures.SHAKE
import com.adelpos.meseros.core.domain.waiter.WaiterFixtures.TACOS
import com.adelpos.meseros.core.domain.waiter.WaiterFixtures.capture
import com.adelpos.meseros.core.domain.waiter.WaiterFixtures.draft
import com.adelpos.meseros.core.domain.waiter.WaiterFixtures.line
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Test

class WaiterRulesTest {
    @Test
    fun tiempoConElFormatoDelEscritorio() {
        assertEquals("0 min", ElapsedTime.format(0))
        assertEquals("0 min", ElapsedTime.format(-30))
        assertEquals("45 min", ElapsedTime.format(45 * 60 + 59))
        assertEquals("1 h 0 min", ElapsedTime.format(60 * 60))
        assertEquals("5 h 12 min", ElapsedTime.format(5 * 3600 + 12 * 60))
        assertEquals("23 h 59 min", ElapsedTime.format(24 * 3600 - 1))
        assertEquals("1 d 0 h", ElapsedTime.format(24 * 3600))
        assertEquals("3 d 14 h", ElapsedTime.format(3 * 86_400 + 14 * 3600 + 59 * 60))
        // Lo que pasó desde la lectura de Go se suma con el reloj monótono (nunca hacia atrás).
        assertEquals(130, ElapsedTime.secondsNow(100, 5_000, 35_000))
        assertEquals(100, ElapsedTime.secondsNow(100, 5_000, 1_000))
    }

    @Test
    fun lienzoDelMapaEscalaSinDeformar() {
        assertEquals(0.5f, TableCanvas.scaleToFit(500f, 1000f), 0.0001f)
        assertEquals(0.5f, TableCanvas.scaleToFit(2000f, 280f), 0.0001f)
        assertEquals(0f, TableCanvas.scaleToFit(0f, 200f), 0.0001f)
    }

    @Test
    fun busquedaDeMesaSinAcentosNiMayusculas() {
        val terraza = WaiterFixtures.table("t1", "Terraza Árbol 3")
        assertTrue(TableSearch.matches(terraza, "arbol"))
        assertTrue(TableSearch.matches(terraza, "  3   TERRAZA "))
        assertTrue(TableSearch.matches(terraza, ""))
        assertFalse(TableSearch.matches(terraza, "barra"))
    }

    @Test
    fun mesasVisiblesComoEnElEscritorio() {
        val view =
            RestaurantView(
                areas =
                    listOf(
                        DiningArea("b", "Terraza", 2, true),
                        DiningArea("a", "Salón", 1, true),
                        DiningArea("c", "Cerrada", 0, false),
                    ),
                tables =
                    listOf(
                        WaiterFixtures.table("1", "Mesa 1", "a"),
                        WaiterFixtures.table("2", "Mesa 2", "a", active = false),
                        WaiterFixtures.table("3", "Mesa 3", "a", active = false, accounts = 1),
                    ),
                settings = null,
                readAtElapsedMillis = 0,
            )
        assertEquals(listOf("a", "b"), view.visibleAreas.map { it.id })
        // Una mesa desactivada sigue a la vista mientras tenga cuentas abiertas.
        assertEquals(listOf("1", "3"), view.tablesIn("a").map { it.id })
    }

    @Test
    fun permisosSoloDeshabilitanLoQueGoRechazaria() {
        val waiter =
            WaiterPermissions(
                setOf(
                    "restaurant.tables.read",
                    "restaurant.areas.read",
                    "accounts.read",
                    "orders.capture",
                    "orders.confirm",
                    "catalog.read",
                    "catalog.prices.read",
                    "orders.edit",
                ),
            )
        assertTrue(waiter.allows(WaiterAction.VIEW_TABLES))
        assertTrue(waiter.allows(WaiterAction.SEND_TO_KITCHEN))
        assertFalse(waiter.allows(WaiterAction.OPEN_ACCOUNT))
        assertFalse(waiter.allows(WaiterAction.CANCEL_SENT))
        assertTrue(waiter.allows(WaiterAction.EDIT_SENT))
        // Reducir lo enviado pide modificar y cancelar.
        assertFalse(waiter.canReduceSent)
        // Sin conocer los permisos (entró sin conexión) no se deshabilita nada: Go valida al
        // recibir.
        assertTrue(WaiterPermissions.UNKNOWN.allows(WaiterAction.CANCEL_SENT))
        assertFalse(WaiterPermissions.UNKNOWN.known)
    }

    @Test
    fun operacionesLocalesSobreElBorrador() {
        val base = listOf(draft("a", BURGER), draft("b", SHAKE, notes = "sin crema"))
        val ids = generateSequence(1) { it + 1 }.map { "n$it" }.iterator()
        // Un producto sencillo igual a uno sencillo que ya está suma unidades.
        val more = DraftOperations.addLine(base, line(BURGER, 2)) { ids.next() }
        assertTrue(more is DraftOperation.Quantity)
        assertEquals(3, DraftOperations.apply(base, listOf(more)).first().line.quantity)
        // Uno personalizado nunca se combina.
        val custom = DraftOperations.addLine(base, line(SHAKE, notes = "sin crema")) { ids.next() }
        assertTrue(custom is DraftOperation.Add)
        assertEquals(3, DraftOperations.apply(base, listOf(custom)).size)
        // «−» hasta cero quita el renglón; «+» sobre uno que ya no está lo vuelve a crear con su
        // respaldo y solo las unidades sumadas.
        val minus = DraftOperations.changeQuantity(base[0], -1) { ids.next() }
        assertEquals(listOf("b"), DraftOperations.apply(base, listOf(minus)).map { it.id })
        val plus = DraftOperations.changeQuantity(base[0], 2) { "respaldo" }
        val afterSend = DraftOperations.apply(emptyList(), listOf(plus))
        assertEquals(listOf("respaldo"), afterSend.map { it.id })
        assertEquals(2, afterSend.single().line.quantity)
        val minusGone = DraftOperations.changeQuantity(base[0], -1) { "otro" }
        assertTrue(DraftOperations.apply(emptyList(), listOf(minusGone)).isEmpty())
        // Sustituir conserva el identificador y el lugar; quitar lo quita.
        val replace = DraftOperation.Replace("b", draft("x", SHAKE, 2))
        val replaced = DraftOperations.apply(base, listOf(replace))
        assertEquals(listOf("a", "b"), replaced.map { it.id })
        assertEquals(2, replaced[1].line.quantity)
        assertEquals(
            listOf("a"),
            DraftOperations.apply(base, listOf(DraftOperation.Remove("b"))).map { it.id },
        )
    }

    @Test
    fun capturaEnCursoSoloConAlgoDelMesero() {
        val foreign = capture(base = listOf(draft("ajeno", BURGER)))
        assertFalse(foreign.inProgress)
        val touched = foreign.copy(operations = listOf(DraftOperation.Remove("ajeno")))
        assertTrue(touched.inProgress)
        val ownSaved = foreign.copy(ownItemIds = setOf("ajeno"))
        assertTrue(ownSaved.inProgress)
    }

    @Test
    fun pedidoSinConexionSustituyeSoloLoQueVioYTocoEsteDispositivo() {
        val base =
            listOf(
                draft("propio", BURGER),
                draft("ajeno-igual", SHAKE),
                draft("ajeno-cambiado", TACOS),
                draft("propio-quitado", BURGER, notes = "bien cocida"),
            )
        val operations =
            listOf(
                DraftOperation.Replace("ajeno-cambiado", draft("x", TACOS, 3)),
                DraftOperation.Remove("propio-quitado"),
                DraftOperation.Add(draft("nuevo", SHAKE, 2)),
            )
        val plan =
            OfflineOrderPlan.of(
                capture(
                    base = base,
                    operations = operations,
                    own = setOf("propio", "propio-quitado", "nuevo"),
                ),
            )
        // Partidas: la propia guardada, la ajena que cambió aquí y la nueva (en el orden visible).
        assertEquals(listOf("propio", "ajeno-cambiado", "nuevo"), plan.items.map { it.id })
        assertEquals(3, plan.items[1].line.quantity)
        // Sustituye lo que vio de esas y lo que quitó; lo ajeno sin tocar no se manda.
        assertEquals(
            listOf("propio", "ajeno-cambiado", "propio-quitado"),
            plan.replaces.map { it.id },
        )
        assertEquals(1, plan.replaces[1].line.quantity)
    }

    @Test
    fun cuentaPendienteNoSustituyeBorrador() {
        val plan =
            OfflineOrderPlan.of(
                capture(
                    accountId = null,
                    operations = listOf(DraftOperation.Add(draft("n1", BURGER))),
                    own = setOf("n1"),
                ),
            )
        assertEquals(listOf("n1"), plan.items.map { it.id })
        assertTrue(plan.replaces.isEmpty())
    }

    @Test
    fun pedidoConLosPreciosQueVioElMesero() {
        val order =
            WaiterFixtures.order(
                capture(
                    operations =
                        listOf(
                            DraftOperation.Add(draft("n1", BURGER)),
                            DraftOperation.Add(draft("n2", SHAKE)),
                        ),
                    own = setOf("n1", "n2"),
                    // La cotización de Go en línea manda sobre la referencia del catálogo.
                    prices = mapOf("n2" to Money("70.00")),
                ),
            )
        assertEquals(listOf("120.00", "70.00"), order.lines.map { it.seenUnitPrice.decimal })
        assertEquals(listOf("Hamburguesa", "Malteada de fresa"), order.lines.map { it.productName })
        assertEquals(ACCOUNT, order.destination.accountId)
        assertEquals(41L, order.catalogVersion)
        assertEquals("epoca", order.catalogEpoch)
        // Sin nada del mesero no hay pedido.
        assertNull(
            OrderFactory.fromCapture(
                capture = capture(base = listOf(draft("ajeno", BURGER))),
                waiterName = "Ana",
                operationId = "op",
                catalog = OrderFactory.CatalogStamp(1, null),
                lookup = WaiterFixtures.lookup,
                onlineConfirm = null,
                nowMillis = 0,
            ),
        )
    }
}
