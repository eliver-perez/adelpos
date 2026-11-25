package com.adelpos.core.network.discovery

import android.content.Context
import android.net.nsd.NsdManager
import android.net.nsd.NsdServiceInfo
import android.util.Log
import androidx.test.ext.junit.runners.AndroidJUnit4
import androidx.test.platform.app.InstrumentationRegistry
import com.adelpos.core.model.server.ServerAddress
import java.util.concurrent.CountDownLatch
import java.util.concurrent.TimeUnit
import kotlinx.coroutines.runBlocking
import org.junit.After
import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Test
import org.junit.runner.RunWith

/**
 * La búsqueda NSD en el dispositivo, con las dos API según la versión
 * (`registerServiceInfoCallback` desde la 34, `resolveService` antes), contra un anuncio que
 * registra la propia prueba dentro del emulador con el TXT del servidor Go. Usa un tipo propio,
 * `_adelpostest._tcp`, para no anunciar nunca un servidor falso del restaurante ni ver el real
 * (avance §1.5: el emulador no encuentra un servidor de la Mac; la búsqueda real solo se prueba en
 * un dispositivo físico).
 */
@RunWith(AndroidJUnit4::class)
class NsdServerBrowserTest {
    private val context: Context = InstrumentationRegistry.getInstrumentation().targetContext
    private val manager = context.getSystemService(NsdManager::class.java)
    private var registration: NsdManager.RegistrationListener? = null
    private val serverId = "11111111-2222-4333-8444-555555555555"

    @After
    fun unregister() {
        registration?.let { listener ->
            try {
                manager.unregisterService(listener)
            } catch (_: IllegalArgumentException) {
                // Ya no estaba registrado.
            }
        }
    }

    private fun announce(name: String, port: Int) {
        val info =
            NsdServiceInfo().apply {
                serviceName = name
                serviceType = TEST_TYPE
                setPort(port)
                setAttribute("txtvers", "1")
                setAttribute("server_id", serverId)
                setAttribute("api", "v1")
                setAttribute("port", port.toString())
                setAttribute("fp", "0011223344556677")
            }
        val registered = CountDownLatch(1)
        val listener =
            object : NsdManager.RegistrationListener {
                override fun onServiceRegistered(serviceInfo: NsdServiceInfo) =
                    registered.countDown()

                override fun onRegistrationFailed(serviceInfo: NsdServiceInfo, errorCode: Int) =
                    Unit

                override fun onServiceUnregistered(serviceInfo: NsdServiceInfo) = Unit

                override fun onUnregistrationFailed(serviceInfo: NsdServiceInfo, errorCode: Int) =
                    Unit
            }
        manager.registerService(info, NsdManager.PROTOCOL_DNS_SD, listener)
        registration = listener
        assertTrue("el anuncio de prueba no se registró", registered.await(10, TimeUnit.SECONDS))
    }

    @Test
    fun encuentraElAnuncioYLeeSuTxt() = runBlocking {
        announce("Fonda Doña Ñeli — Prueba", 9443)
        val browser = NsdServerBrowser(context, serviceType = { TEST_TYPE }, windowMillis = 8_000)
        val result = browser.browse()
        Log.i("NsdServerBrowserTest", "Resultado: $result")
        val found = (result as NsdServerBrowser.Browse.Found).servers
        assertEquals(1, found.size)
        val server = found.single()
        assertEquals(serverId, server.serverId)
        assertEquals("0011223344556677", server.fingerprintPrefix)
        assertEquals(9443, server.port)
        assertEquals("Fonda Doña Ñeli — Prueba", server.name)
        assertTrue(server.hosts.isNotEmpty())
        assertTrue(server.hosts.all(ServerAddress::isLocalHost))
    }

    @Test
    fun sinAnunciosNoEncuentraNada() = runBlocking {
        val browser =
            NsdServerBrowser(context, serviceType = { "_adelposvacio._tcp" }, windowMillis = 2_000)
        assertEquals(NsdServerBrowser.Browse.Found(emptyList()), browser.browse())
    }

    private companion object {
        const val TEST_TYPE = "_adelpostest._tcp"
    }
}
