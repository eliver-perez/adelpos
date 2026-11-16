package local.adel.pos.android.core.data.api

import androidx.test.ext.junit.runners.AndroidJUnit4
import androidx.test.platform.app.InstrumentationRegistry
import kotlinx.coroutines.runBlocking
import local.adel.pos.android.core.data.security.Hex
import local.adel.pos.android.core.domain.error.AppError
import local.adel.pos.android.core.domain.error.AppResult
import okhttp3.HttpUrl.Companion.toHttpUrl
import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Assume.assumeTrue
import org.junit.Test
import org.junit.runner.RunWith

/**
 * El cliente con huella fijada contra el Go temporal del arnés (`iso.sh server-up`), desde el
 * dispositivo, por la escucha de red HTTPS: comprueba las suites restringidas de TLS en cada
 * versión de Android (TLS 1.2 en Android 8, 1.3 desde Android 10). Solo corre con los argumentos de
 * instrumentación `pin` (huella SPKI en hexadecimal) y `net` (`https://10.0.2.2:<ISO_NET_PORT>`);
 * sin ellos se omite. Nunca contra el servicio real de la Mac.
 */
@RunWith(AndroidJUnit4::class)
class ServerApiOnDeviceTest {
    private val arguments = InstrumentationRegistry.getArguments()

    private fun argumentOrSkip(name: String): String {
        val value = arguments.getString(name)
        assumeTrue(
            "Sin el argumento de instrumentación $name (Go temporal)",
            !value.isNullOrBlank(),
        )
        return value!!
    }

    @Test
    fun versionDelServidorConLaHuellaFijada() = runBlocking {
        val pin = Hex.decode(argumentOrSkip("pin"))
        val network = argumentOrSkip("net").toHttpUrl()
        val api =
            ServerApi(network, PinnedHttpClient.create(pin, ClientIdentity(version = "0.1.0")))
        val result = api.getVersion()
        assertTrue("Se esperaba la versión: $result", result is AppResult.Success)
        val version = (result as AppResult.Success).value
        assertEquals("v1", version.apiVersion)
        assertTrue(version.testMode)
        assertTrue(version.clientKinds.contains("android"))
    }

    @Test
    fun otraHuellaSeRechazaContraElServidorReal() = runBlocking {
        val pin = Hex.decode(argumentOrSkip("pin"))
        val network = argumentOrSkip("net").toHttpUrl()
        val other = pin.copyOf()
        other[0] = (other[0].toInt() xor 0xff).toByte()
        val api =
            ServerApi(network, PinnedHttpClient.create(other, ClientIdentity(version = "0.1.0")))
        assertEquals(AppResult.Failure(AppError.ServerIdentityChanged), api.getVersion())
    }
}
