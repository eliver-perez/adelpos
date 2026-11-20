package com.adelpos.meseros.core.domain.waiter

import com.adelpos.meseros.core.domain.money.Money
import com.adelpos.meseros.core.domain.table.TableState
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Test

/**
 * Etapa A3, §4: el mapa con los mismos casos que la prueba del escritorio
 * (`stage9dTables.test.tsx`, «escala automática del mapa»), el título de la captura y la pantalla
 * inicial del dispositivo.
 */
class StageA3PanelRulesTest {
    private fun table(id: String, x: Int, y: Int, width: Int, height: Int) =
        DiningTable(
            id = id,
            areaId = "salon",
            name = id,
            shape = TableShape.RECTANGLE,
            x = x,
            y = y,
            width = width,
            height = height,
            seats = 4,
            active = true,
            outOfService = false,
            accountCount = 0,
            state = TableState.FREE,
            occupiedSeconds = 0,
            balance = Money("0.00"),
            longOpen = false,
        )

    @Test
    fun ajustaUnaSolaEscalaParaVerTodasLasMesasDelArea() {
        val tables = listOf(table("a", 300, 100, 120, 100), table("b", 600, 300, 120, 100))
        val fit = TableMapFit.of(tables, 800f, 500f)!!
        // El conjunto mide 420 × 300 unidades; el espacio útil es 768 × 468.
        assertEquals(468f / 300f, fit.scale, 0.00001f)
        assertFalse(fit.overflow)
        assertEquals(800f, fit.width)
        assertEquals(500f, fit.height)
        for (entry in tables) {
            assertTrue(fit.left(entry) + entry.width * fit.scale <= 800f)
            assertTrue(fit.top(entry) + entry.height * fit.scale <= 500f)
        }
    }

    @Test
    fun ningunaMesaQuedaDebajoDe64SiNoCabenElMapaSeDesplaza() {
        val tables = buildList {
            for (row in 0 until 5) {
                for (column in 0 until 7) {
                    add(table("m$row$column", 10 + column * 140, 10 + row * 110, 64, 56))
                }
            }
        }
        val fit = TableMapFit.of(tables, 600f, 400f)!!
        assertEquals(TableMapFit.MINIMUM_TABLE_SIDE / 56f, fit.scale, 0.00001f)
        assertEquals(64f, tables.minOf { it.height * fit.scale }, 0.0001f)
        assertTrue(fit.overflow)
        assertTrue(fit.width > 600f)
        assertTrue(fit.height > 400f)
    }

    @Test
    fun pocasMesasNoPasanDelDobleYQuedanCentradas() {
        val fit = TableMapFit.of(listOf(table("a", 0, 0, 120, 100)), 1000f, 800f)!!
        assertEquals(TableMapFit.MAXIMUM_SCALE, fit.scale)
        assertEquals((1000f - 240f) / 2f, fit.offsetX, 0.0001f)
        assertEquals((800f - 200f) / 2f, fit.offsetY, 0.0001f)
    }

    @Test
    fun laTabletaVerticalYElTelefonoHorizontalLlenanElEspacio() {
        // Antes se escalaba el lienzo completo de 1000 × 560: en 400 × 900 dp (la mitad de una
        // tableta vertical) quedaba a 0.4 y 224 dp de alto, con el resto vacío.
        val tables = listOf(table("a", 100, 80, 100, 72), table("b", 400, 300, 100, 72))
        val tall = TableMapFit.of(tables, 400f, 900f)!!
        assertTrue("más grande que el lienzo completo", tall.scale > 400f / TableCanvas.WIDTH)
        // El teléfono horizontal (≈ 914 × 411 dp, menos la barra y la fila): sin espacio negativo.
        val wide = TableMapFit.of(tables, 840f, 260f)!!
        assertTrue(wide.scale > 0f)
        assertTrue(tables.all { wide.top(it) + it.height * wide.scale <= wide.height + 0.5f })
    }

    @Test
    fun sinMesasOSinEspacioNoHayAjuste() {
        assertNull(TableMapFit.of(emptyList(), 800f, 500f))
        assertNull(TableMapFit.of(listOf(table("a", 0, 0, 10, 10)), 0f, 500f))
    }

    private fun capture(takeaway: Boolean, accountId: String? = null, name: String = "") =
        Capture(
            id = "c1",
            waiterUserId = "ana",
            tableId = if (takeaway) null else "t4",
            tableName = if (takeaway) "" else "Mesa 4",
            accountId = accountId,
            accountName = null,
            seenAccountIds = emptyList(),
            base = emptyList(),
            baseRevision = 0,
            operations = emptyList(),
            ownItemIds = emptySet(),
            seenPrices = emptyMap(),
            sync = DraftSync.SAVED,
            syncMessage = null,
            tableAccounts = emptyList(),
            pendingOperationId = null,
            inFlightCount = 0,
            createdAtMillis = 0,
            mode = if (takeaway) OrderMode.TAKEAWAY else OrderMode.DINE_IN,
            customer = TakeawayCustomer(name, ""),
        )

    private fun bill(id: String, folio: Int) =
        AccountBill(
            account =
                AccountSummary(
                    id = id,
                    folio = folio,
                    name = "Para llevar",
                    tableId = null,
                    tableName = null,
                    state = AccountSummary.STATE_OPEN,
                    revision = 1,
                    preparationState = null,
                    confirmedQuantity = 0,
                    draftQuantity = 0,
                    totals = Totals.ZERO,
                    assignedName = null,
                    elapsedSeconds = null,
                    longOpen = false,
                ),
            draft = emptyList(),
            lines = emptyList(),
        )

    @Test
    fun elTituloDeLaCapturaDiceDondeSeEsta() {
        assertEquals(CaptureTitle.Table("Mesa 4"), CaptureTitle.of(capture(false), null))
        assertEquals(
            CaptureTitle.Takeaway(12),
            CaptureTitle.of(capture(true, accountId = "p12"), bill("p12", 12)),
        )
        // La cuenta leída es de otra captura: su folio no cuenta.
        assertEquals(
            CaptureTitle.NewTakeaway,
            CaptureTitle.of(capture(true, accountId = "p13"), bill("p12", 12)),
        )
        assertEquals(
            CaptureTitle.NamedTakeaway("Juan"),
            CaptureTitle.of(capture(true, name = " Juan "), null),
        )
        assertEquals(CaptureTitle.NewTakeaway, CaptureTitle.of(capture(true), null))
    }

    @Test
    fun laPantallaInicialDelDispositivoSoloEsMesasOParaLlevar() {
        assertEquals(DeviceHome.TAKEAWAY, DeviceHome.fromWire("takeaway"))
        // A3-D7: cualquier otro valor (también los de un servidor 0.13.0) es Mesas.
        for (other in listOf("tables", "kitchen", "cash", "otra", "", null)) {
            assertEquals("$other", DeviceHome.TABLES, DeviceHome.fromWire(other))
        }
        val canSee = WaiterPermissions(setOf("takeaway.read"))
        assertTrue(DeviceHome.startsInTakeaway(DeviceHome.TAKEAWAY, canSee))
        assertFalse(DeviceHome.startsInTakeaway(DeviceHome.TABLES, canSee))
        // Sin permiso de ver Para llevar (o con permisos desconocidos), empieza en Mesas.
        assertFalse(DeviceHome.startsInTakeaway(DeviceHome.TAKEAWAY, WaiterPermissions(emptySet())))
        assertFalse(DeviceHome.startsInTakeaway(DeviceHome.TAKEAWAY, WaiterPermissions.UNKNOWN))
    }
}
