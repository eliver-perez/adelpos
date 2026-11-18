package com.adelpos.meseros.feature.waiter

import androidx.lifecycle.SavedStateHandle
import com.adelpos.meseros.core.capture.CaptureNotice
import com.adelpos.meseros.core.capture.CaptureViewModel
import com.adelpos.meseros.core.capture.CustomerView
import com.adelpos.meseros.core.domain.catalog.Catalog
import com.adelpos.meseros.core.domain.catalog.CatalogCategory
import com.adelpos.meseros.core.domain.catalog.CatalogProduct
import com.adelpos.meseros.core.domain.catalog.CatalogVersion
import com.adelpos.meseros.core.domain.catalog.Customization
import com.adelpos.meseros.core.domain.catalog.LineSelection
import com.adelpos.meseros.core.domain.catalog.ProductAvailability
import com.adelpos.meseros.core.domain.catalog.ProductKind
import com.adelpos.meseros.core.domain.money.Money
import com.adelpos.meseros.core.domain.table.TableState
import com.adelpos.meseros.core.domain.waiter.AccountSummary
import com.adelpos.meseros.core.domain.waiter.CustomerEdit
import com.adelpos.meseros.core.domain.waiter.DiningTable
import com.adelpos.meseros.core.domain.waiter.DraftLine
import com.adelpos.meseros.core.domain.waiter.DraftOperation
import com.adelpos.meseros.core.domain.waiter.OrderMode
import com.adelpos.meseros.core.domain.waiter.PrecheckPrintState
import com.adelpos.meseros.core.domain.waiter.PrecheckPrinting
import com.adelpos.meseros.core.domain.waiter.PrintAnswer
import com.adelpos.meseros.core.domain.waiter.PrintMessage
import com.adelpos.meseros.core.domain.waiter.TableShape
import com.adelpos.meseros.core.domain.waiter.TakeawayCustomer
import com.adelpos.meseros.core.domain.waiter.Totals
import com.adelpos.meseros.core.domain.waiter.WaiterPermissions
import com.adelpos.meseros.feature.waiter.tables.TablesViewModel
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.ExperimentalCoroutinesApi
import kotlinx.coroutines.test.StandardTestDispatcher
import kotlinx.coroutines.test.advanceTimeBy
import kotlinx.coroutines.test.advanceUntilIdle
import kotlinx.coroutines.test.resetMain
import kotlinx.coroutines.test.runTest
import kotlinx.coroutines.test.setMain
import org.junit.After
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Before
import org.junit.Test

@OptIn(ExperimentalCoroutinesApi::class)
class WaiterViewModelsTest {
    private val dispatcher = StandardTestDispatcher()

    @Before
    fun main() {
        Dispatchers.setMain(dispatcher)
    }

    @After
    fun reset() {
        Dispatchers.resetMain()
    }

    private val table =
        DiningTable(
            id = "t1",
            areaId = "a1",
            name = "Mesa 4",
            shape = TableShape.RECTANGLE,
            x = 0,
            y = 0,
            width = 100,
            height = 80,
            seats = 4,
            active = true,
            outOfService = false,
            accountCount = 1,
            state = TableState.OCCUPIED,
            occupiedSeconds = 60,
            balance = Money("0.00"),
            longOpen = false,
        )

    private fun account(id: String, tableId: String) =
        AccountSummary(
            id = id,
            folio = 1,
            name = "Mesa 4",
            tableId = tableId,
            tableName = "Mesa 4",
            state = "open",
            revision = 2,
            preparationState = null,
            confirmedQuantity = 1,
            draftQuantity = 0,
            totals = Totals.ZERO,
            assignedName = null,
            elapsedSeconds = 60,
            longOpen = false,
        )

    @Test
    fun mesasConservaBusquedaYMesaElegidaYAbreConLasCuentasVistas() =
        runTest(dispatcher) {
            val fake = FakeWaiter()
            val saved = SavedStateHandle()
            val model = TablesViewModel(fake.dependencies(), saved)
            model.onSearchChange("terraza")
            model.onTableSelect(table)
            // Otro ViewModel con el mismo estado guardado (rotación o cierre del proceso).
            val restored = TablesViewModel(fake.dependencies(), saved)
            assertEquals("terraza", restored.search)
            assertEquals("t1", restored.selectedTableId)
            assertTrue(restored.showingAccount)

            // Sin permiso para abrir cuentas, «Nueva cuenta» no abre nada.
            fake.permissions.value = WaiterPermissions(setOf("accounts.read"))
            var opened: String? = null
            restored.openCapture(table, account = null) { opened = it }
            advanceUntilIdle()
            assertNull(opened)
            // Con permiso: la cuenta nueva sabe qué cuentas veía el mesero en la mesa.
            fake.permissions.value = WaiterPermissions.UNKNOWN
            fake.accounts.value = listOf(account("c1", "t1"), account("c2", "otra"))
            advanceUntilIdle()
            restored.openCapture(table, account = null) { opened = it }
            advanceUntilIdle()
            assertEquals("captura-1", opened)
            assertEquals(listOf("c1"), fake.opened.single().third)
        }

    @Test
    fun capturaAgregaConElPrecioDeReferenciaYEnviaACocina() =
        runTest(dispatcher) {
            val product =
                CatalogProduct(
                    id = "p1",
                    name = "Hamburguesa",
                    description = "",
                    active = true,
                    categoryId = "c1",
                    code = "H",
                    barcode = null,
                    kind = ProductKind.PRODUCT,
                    referencePrice = Money("120.00"),
                    pricingError = null,
                    imageId = null,
                    sortOrder = 1,
                    variants = emptyList(),
                    ingredients = emptyList(),
                    groups = emptyList(),
                    slots = emptyList(),
                    availability = ProductAvailability.AVAILABLE,
                )
            val catalog =
                Catalog(
                    version = CatalogVersion(4, "e", "", "America/Monterrey", 16, "General", 1),
                    categories = listOf(CatalogCategory("c1", "Comida", "", true, 1, null, true)),
                    products = listOf(product),
                    groups = emptyList(),
                    quickNotes = emptyList(),
                )
            val fake = FakeWaiter(catalog)
            fake.captureFlow.value = FakeWaiter.emptyCapture("captura-1", "t1")
            val saved = SavedStateHandle(mapOf(CaptureViewModel.KEY_CAPTURE_ID to "captura-1"))
            val model = CaptureViewModel(fake.captureDependencies(), saved)
            advanceUntilIdle()
            model.onProductSelect(product)
            assertEquals("p1", model.editing?.productId)
            model.onAdd(LineSelection("p1", null, 2, Customization.ORIGINAL))
            advanceUntilIdle()
            val (operation, price) = fake.staged.single()
            assertTrue(operation is DraftOperation.Add)
            assertEquals(Money("120.00"), price)
            assertNull(model.editing)
            model.onSendToKitchen()
            advanceUntilIdle()
            assertEquals(listOf("captura-1"), fake.sent)
            assertEquals(CaptureNotice.Sent, model.notice)
        }

    @Test
    fun cambiarUnaPartidaTrasCerrarseLaAplicacionVuelveConLaPartidaGuardada() =
        runTest(dispatcher) {
            // Fase F (FieldOnGoTest.f8): Android cerró la aplicación con «Cambiar» abierto en una
            // partida de 2 con observaciones; el estado guardado solo tiene el producto y la
            // partida. Debe volver la partida tal como está en la captura, no el producto original.
            val line = LineSelection("p1", null, 2, Customization(notes = "bien dorados"))
            val fake = FakeWaiter(Catalog.EMPTY)
            fake.captureFlow.value =
                FakeWaiter.emptyCapture("captura-1", "t1")
                    .copy(base = listOf(DraftLine("renglon-1", line)))
            val saved =
                SavedStateHandle(
                    mapOf(
                        CaptureViewModel.KEY_CAPTURE_ID to "captura-1",
                        CaptureViewModel.KEY_EDITING_PRODUCT to "p1",
                        CaptureViewModel.KEY_EDITING_ITEM to "renglon-1",
                    ),
                )
            val model = CaptureViewModel(fake.captureDependencies(), saved)
            advanceUntilIdle()
            assertEquals(line, model.editing)
            assertEquals("renglon-1", model.editingItemId)

            // Si la partida ya no existe (se envió o se quitó), el personalizador se cierra.
            val gone =
                SavedStateHandle(
                    mapOf(
                        CaptureViewModel.KEY_CAPTURE_ID to "captura-1",
                        CaptureViewModel.KEY_EDITING_PRODUCT to "p1",
                        CaptureViewModel.KEY_EDITING_ITEM to "renglon-quitado",
                    ),
                )
            val closed = CaptureViewModel(fake.captureDependencies(), gone)
            advanceUntilIdle()
            assertNull(closed.editing)
            assertNull(closed.editingItemId)
        }

    /**
     * Para llevar en la captura compartida (A2-D1): el nombre y el teléfono que se ven son los de
     * la captura mientras el pedido no sale a Go y, si hay una edición por enviar, esa (marcada
     * como pendiente). En una captura de mesa no hay cliente.
     */
    @Test
    fun capturaParaLlevarMuestraElClienteVigente() =
        runTest(dispatcher) {
            val fake = FakeWaiter(Catalog.EMPTY)
            fake.captureFlow.value = FakeWaiter.emptyCapture("captura-1", "t1")
            val saved = SavedStateHandle(mapOf(CaptureViewModel.KEY_CAPTURE_ID to "captura-1"))
            val model = CaptureViewModel(fake.captureDependencies(), saved)
            advanceUntilIdle()
            assertEquals(CustomerView.NONE, model.customer.value)

            fake.captureFlow.value =
                FakeWaiter.emptyCapture("captura-1", "t1")
                    .copy(
                        tableId = null,
                        tableName = "",
                        accountId = null,
                        mode = OrderMode.TAKEAWAY,
                        customer = TakeawayCustomer("Juan", ""),
                    )
            advanceUntilIdle()
            assertEquals(
                CustomerView(TakeawayCustomer("Juan", ""), pending = false),
                model.customer.value,
            )
            model.onCustomerChange(TakeawayCustomer("Juan Pérez", "555"))
            advanceUntilIdle()
            assertEquals(
                listOf("captura-1" to TakeawayCustomer("Juan Pérez", "555")),
                fake.customers,
            )

            // Ya salió a Go (otra lectura): se ve la edición por enviar, marcada como pendiente.
            fake.edits.value =
                listOf(
                    CustomerEdit(
                        key = "captura-1",
                        accountId = null,
                        waiterUserId = "ana",
                        customer = TakeawayCustomer("Juan Pérez", "555"),
                        operationId = "e1",
                        updatedAtMillis = 1L,
                    ),
                )
            advanceUntilIdle()
            assertEquals(
                CustomerView(TakeawayCustomer("Juan Pérez", "555"), pending = true),
                model.customer.value,
            )
        }

    /**
     * La precuenta desde la cuenta (etapa A2, Fase C): al abrir la cuenta se vuelven a leer el
     * nombre visible y la impresora; sin red o sin sesión no se puede imprimir; «Enviada a…» se
     * sigue unos segundos sin bloquear la captura; «Reintentar» solo si nunca salió en papel; y
     * otra cuenta olvida lo de la anterior.
     */
    @Test
    fun capturaImprimeLaPrecuentaYSigueElTrabajo() =
        runTest(dispatcher) {
            val fake = FakeWaiter(Catalog.EMPTY)
            fake.captureFlow.value =
                FakeWaiter.emptyCapture("captura-1", "t1").copy(accountId = "c1")
            val saved = SavedStateHandle(mapOf(CaptureViewModel.KEY_CAPTURE_ID to "captura-1"))
            val model = CaptureViewModel(fake.captureDependencies(), saved)
            advanceUntilIdle()
            assertEquals(listOf("ana"), fake.setupReads)
            assertTrue(model.canPrintNow.value)

            // Sin red, o sin sesión de Go (entró sin conexión), no se puede imprimir.
            fake.connected.value = false
            advanceUntilIdle()
            assertFalse(model.canPrintNow.value)
            fake.connected.value = true
            fake.active.value = fake.active.value!!.copy(hasServerSession = false)
            advanceUntilIdle()
            assertFalse(model.canPrintNow.value)
            fake.active.value = fake.active.value!!.copy(hasServerSession = true)
            advanceUntilIdle()
            assertTrue(model.canPrintNow.value)

            // La impresora de red no responde: «Reintentar» sin motivo la misma precuenta.
            fake.printAnswers.add(PrintAnswer.Queued("t1", "j1", "pending", null, "Red", null))
            fake.statusAnswers.add(PrintAnswer.Status("error", "unreachable", "Red", null))
            fake.retryAnswers.add(PrintAnswer.Queued("t1", "j1", "pending", null, "Red", null))
            fake.statusAnswers.add(PrintAnswer.Status("submitted", null, "Red", null))
            model.onPrintPrecheck()
            advanceTimeBy(1)
            assertEquals(PrintMessage.SentTo("Red"), model.precheckState.value.message)
            assertTrue(model.precheckState.value.following)
            // Mientras se sigue, la captura sigue igual (nada espera a la impresora).
            model.onSearchTextChange("taco")
            assertEquals("taco", model.searchText)
            advanceTimeBy(PrecheckPrinting.POLL_MILLIS + 1)
            assertEquals(PrintMessage.NotResponding("Red"), model.precheckState.value.message)
            assertTrue(model.precheckState.value.canRetry)
            model.onRetryPrecheck()
            advanceTimeBy(PrecheckPrinting.POLL_MILLIS + 2)
            assertEquals(listOf("t1"), fake.retried)
            assertEquals(PrintMessage.SentTo("Red"), model.precheckState.value.message)
            assertFalse(model.precheckState.value.canRetry)
            assertEquals("c1", fake.printed.single().first)

            // Otra vez «Imprimir»: otra precuenta con otra clave (como en la caja).
            fake.printAnswers.add(
                PrintAnswer.Rejected(409, "PRINTER_NOT_CONFIGURED", "Este dispositivo…"),
            )
            model.onPrintPrecheck()
            advanceUntilIdle()
            assertEquals(PrintMessage.NoPrinter, model.precheckState.value.message)
            assertEquals(2, fake.printed.map { it.second }.toSet().size)

            // La captura pasó a otra cuenta: se olvida lo de la anterior.
            fake.captureFlow.value =
                FakeWaiter.emptyCapture("captura-1", "t1").copy(accountId = "c2")
            advanceUntilIdle()
            assertEquals(PrecheckPrintState.IDLE, model.precheckState.value)
        }
}
