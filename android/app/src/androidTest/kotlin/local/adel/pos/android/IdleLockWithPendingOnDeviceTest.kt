package local.adel.pos.android

import androidx.compose.ui.test.hasTestTag
import androidx.compose.ui.test.junit4.v2.createEmptyComposeRule
import androidx.test.core.app.ActivityScenario
import androidx.test.ext.junit.runners.AndroidJUnit4
import kotlinx.coroutines.runBlocking
import local.adel.pos.android.core.data.db.PairedServerEntity
import local.adel.pos.android.core.data.waiter.db.WaiterStorage
import local.adel.pos.android.core.domain.catalog.Customization
import local.adel.pos.android.core.domain.catalog.LineSelection
import local.adel.pos.android.core.domain.money.Money
import local.adel.pos.android.core.domain.waiter.DraftLine
import local.adel.pos.android.core.domain.waiter.OrderDestination
import local.adel.pos.android.core.domain.waiter.OrderLine
import local.adel.pos.android.core.domain.waiter.OrderState
import local.adel.pos.android.core.domain.waiter.PendingOrder
import local.adel.pos.android.feature.access.pin.TAG_ENTRY
import local.adel.pos.android.feature.waiter.tables.TAG_TABLES_SEARCH
import org.junit.After
import org.junit.Assert.assertTrue
import org.junit.Rule
import org.junit.Test
import org.junit.runner.RunWith

/**
 * El bloqueo por inactividad **no ocurre** mientras el mesero tiene pedidos sin enviar (§0-B, §7),
 * en la aplicación real con la cola en Room: con un pedido «Por enviar», más tiempo que el plazo
 * sin tocar la pantalla deja al mesero en Mesas; sin pedidos, se bloquea en la siguiente revisión.
 */
@RunWith(AndroidJUnit4::class)
class IdleLockWithPendingOnDeviceTest {
    @get:Rule val compose = createEmptyComposeRule()

    private var scenario: ActivityScenario<MainActivity>? = null

    @After
    fun close() {
        scenario?.close()
        runBlocking { TestDevice.app.container.orderQueue.eraseAll() }
        TestDevice.reset()
    }

    private fun present(tag: String): Boolean =
        compose.onAllNodes(hasTestTag(tag)).fetchSemanticsNodes().isNotEmpty()

    @Test
    fun conUnPedidoPorEnviarNoSeBloqueaYSinElSi() {
        TestDevice.seedOfflineWaiter()
        val container = TestDevice.app.container
        runBlocking {
            // El plazo más corto que admite Go (30 s).
            val dao = container.database.access()
            val paired = dao.pairedServer()!!
            dao.savePairedServer(
                PairedServerEntity(
                    serverId = paired.serverId,
                    serverName = paired.serverName,
                    baseUrl = paired.baseUrl,
                    fingerprintHex = paired.fingerprintHex,
                    deviceId = paired.deviceId,
                    deviceName = paired.deviceName,
                    sealedDeviceToken = paired.sealedDeviceToken,
                    idleLockSeconds = IDLE_SECONDS,
                    pairedAtMillis = paired.pairedAtMillis,
                ),
            )
            container.deviceAccess.start()
            container.database.waiter().insertOrder(WaiterStorage.toEntity(order()))
        }
        scenario = ActivityScenario.launch(MainActivity::class.java)
        TestDevice.enter(compose)

        // Más que el plazo sin tocar la pantalla: sigue en Mesas.
        Thread.sleep((IDLE_SECONDS + 15) * 1_000L)
        assertTrue("Sigue en Mesas con un pedido por enviar", present(TAG_TABLES_SEARCH))
        assertTrue(!present(TAG_ENTRY))

        // Sin pedidos pendientes, el bloqueo llega en la siguiente revisión.
        runBlocking { container.database.waiter().deleteOrder(OPERATION_ID) }
        compose.waitUntil(timeoutMillis = 40_000) { present(TAG_ENTRY) }
    }

    private fun order() =
        PendingOrder(
            operationId = OPERATION_ID,
            waiterUserId = TestDevice.WAITER_ID,
            waiterName = TestDevice.WAITER_NAME,
            state = OrderState.QUEUED,
            destination = OrderDestination("mesa", "Mesa 4", null, null, emptyList()),
            lines =
                listOf(
                    OrderLine(
                        item =
                            DraftLine(
                                "renglon",
                                LineSelection("producto", null, 1, Customization.ORIGINAL),
                            ),
                        productName = "Hamburguesa",
                        seenUnitPrice = Money("120.00"),
                    ),
                ),
            replaces = emptyList(),
            catalogVersion = 1,
            catalogEpoch = null,
            capturedAtMillis = System.currentTimeMillis(),
            confirmation = null,
            review = null,
            attempts = 0,
            lastError = null,
            credentialRejected = false,
            onlineConfirm = null,
            receivedAccountId = null,
            receivedAtMillis = null,
            updatedAtMillis = System.currentTimeMillis(),
        )

    private companion object {
        const val IDLE_SECONDS = 30
        const val OPERATION_ID = "00000000-0000-4000-8000-00000000e001"
    }
}
