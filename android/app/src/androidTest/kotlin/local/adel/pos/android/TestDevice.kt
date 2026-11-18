package local.adel.pos.android

import androidx.compose.ui.test.hasTestTag
import androidx.compose.ui.test.junit4.ComposeTestRule
import androidx.compose.ui.test.onNodeWithTag
import androidx.compose.ui.test.performClick
import androidx.test.core.app.ApplicationProvider
import kotlinx.coroutines.runBlocking
import local.adel.pos.android.core.data.access.AccessRepository
import local.adel.pos.android.core.data.db.KnownWaiterEntity
import local.adel.pos.android.core.data.db.PairedServerEntity
import local.adel.pos.android.core.domain.access.PinVerifier
import local.adel.pos.android.core.ui.components.TAG_PIN_KEY_PREFIX
import local.adel.pos.android.core.ui.components.TAG_PIN_SUBMIT
import local.adel.pos.android.feature.access.pin.TAG_WAITER_PREFIX
import local.adel.pos.android.feature.waiter.tables.TAG_TABLES_SEARCH

/**
 * Estado del dispositivo para las pruebas de la aplicación completa sin servidor: un servidor
 * emparejado en una dirección donde no responde nadie (`10.0.2.2:9`, la Mac vista desde el
 * emulador, puerto cerrado; nunca el servicio real) y un mesero que ya entró antes, con su
 * verificador del PIN cifrado con el Keystore real. Así se entra **sin conexión**, por el camino
 * real de la aplicación.
 */
object TestDevice {
    const val WAITER_ID = "aaaaaaaa-0000-4000-8000-0000000000a1"
    const val WAITER_NAME = "Mesero de prueba"
    const val PIN = "4829"

    val app: AdelPosApplication
        get() = ApplicationProvider.getApplicationContext()

    /** Sin servidor emparejado ni bitácora: «Buscar el servidor». */
    fun reset() = runBlocking {
        val access = app.container.deviceAccess
        access.forgetServer()
        access.clearDeviceLog()
    }

    /** Emparejado con un servidor que no responde y un mesero conocido con [PIN]. */
    fun seedOfflineWaiter() = runBlocking {
        reset()
        val container = app.container
        val dao = container.database.access()
        val sealer = container.secretSealer
        val serverId = "11111111-2222-4333-8444-555555555555"
        val now = System.currentTimeMillis()
        dao.savePairedServer(
            PairedServerEntity(
                serverId = serverId,
                serverName = "Restaurante de prueba",
                baseUrl = "https://10.0.2.2:9",
                fingerprintHex = "00".repeat(32),
                deviceId = "0b5d2f6c-6b1a-4c4b-9b70-3f0f8e1c2a10",
                deviceName = "Tableta de prueba",
                sealedDeviceToken =
                    sealer.seal("t".repeat(43).toByteArray(), AccessRepository.DEVICE_PURPOSE),
                // Una hora: el bloqueo por inactividad no interrumpe las pruebas.
                idleLockSeconds = 3_600,
                pairedAtMillis = now,
            ),
        )
        dao.saveKnownWaiter(
            KnownWaiterEntity(
                userId = WAITER_ID,
                serverId = serverId,
                name = WAITER_NAME,
                initial = "M",
                color = "#3591d4",
                pinVersion = 1,
                sealedVerifier = PinVerifier.seal(PinVerifier.create(PIN, 1), WAITER_ID, sealer),
                sealedOfflineCredential = null,
                lastOnlineEntryMillis = now,
                offlineFailures = 0,
                offlineClosed = false,
            ),
        )
        container.deviceAccess.start()
    }

    /** Entra como el mesero de prueba con el teclado del PIN y espera la pantalla Mesas. */
    fun enter(compose: ComposeTestRule, pin: String = PIN) {
        val waiter = "$TAG_WAITER_PREFIX$WAITER_ID"
        compose.waitUntil(timeoutMillis = 30_000) {
            compose.onAllNodes(hasTestTag(waiter)).fetchSemanticsNodes().isNotEmpty()
        }
        compose.onNodeWithTag(waiter).performClick()
        for (digit in pin) {
            compose.onNodeWithTag("$TAG_PIN_KEY_PREFIX$digit").performClick()
        }
        compose.onNodeWithTag(TAG_PIN_SUBMIT).performClick()
        compose.waitUntil(timeoutMillis = 30_000) {
            compose.onAllNodes(hasTestTag(TAG_TABLES_SEARCH)).fetchSemanticsNodes().isNotEmpty()
        }
    }
}
