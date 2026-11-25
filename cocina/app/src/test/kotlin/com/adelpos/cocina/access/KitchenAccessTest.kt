package com.adelpos.cocina.access

import com.adelpos.cocina.db.PairedServerDao
import com.adelpos.cocina.db.PairedServerEntity
import com.adelpos.core.model.access.RepairReason
import com.adelpos.core.model.connection.FixedServerLink
import com.adelpos.core.model.connection.LinkState
import com.adelpos.core.model.error.AppError
import com.adelpos.core.model.security.SecretSealer
import com.adelpos.core.model.security.SecretUnavailableException
import com.adelpos.core.model.server.ServerCompatibility
import com.adelpos.core.model.server.ServerRelocator
import com.adelpos.core.network.api.ClientIdentity
import com.adelpos.core.network.api.ClientKinds
import com.adelpos.core.network.pairing.PairedDevice
import com.adelpos.core.network.wipe.DeviceDataWipe
import kotlinx.coroutines.ExperimentalCoroutinesApi
import kotlinx.coroutines.test.TestScope
import kotlinx.coroutines.test.UnconfinedTestDispatcher
import kotlinx.coroutines.test.advanceUntilIdle
import kotlinx.coroutines.test.runTest
import org.junit.Assert.assertEquals
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Test

/**
 * El estado de la pantalla de cocina sin red (etapa K1, Fase B): lo guardado, el emparejamiento que
 * llega del flujo compartido, la credencial que ya no se abre y lo que hace cada error de Go. Con
 * «Sin conexión» (`FixedServerLink`) nunca se pregunta a Go, así que no hace falta un servidor.
 */
@OptIn(ExperimentalCoroutinesApi::class)
class KitchenAccessTest {
    private class MemoryDao : PairedServerDao {
        var stored: PairedServerEntity? = null

        override suspend fun pairedServer(): PairedServerEntity? = stored

        override suspend fun save(server: PairedServerEntity) {
            stored = server
        }

        override suspend fun moveTo(baseUrl: String) = Unit

        override suspend fun forget() {
            stored = null
        }
    }

    // Sobre falso: el secreto con su propósito por delante; sin llave, ya no se abre.
    private class PlainSealer : SecretSealer {
        var keyLost = false

        override fun seal(secret: ByteArray, purpose: String): ByteArray =
            "$purpose|".toByteArray() + secret

        override fun open(sealed: ByteArray, purpose: String): ByteArray {
            val prefix = "$purpose|".toByteArray()
            if (keyLost || !sealed.copyOfRange(0, prefix.size).contentEquals(prefix)) {
                throw SecretUnavailableException(SecretUnavailableException.Reason.KEY_LOST)
            }
            return sealed.copyOfRange(prefix.size, sealed.size)
        }
    }

    private val dao = MemoryDao()
    private val sealer = PlainSealer()
    private var keysDestroyed = 0
    private val erased = mutableListOf<String>()

    private fun TestScope.access(): KitchenAccess =
        KitchenAccess(
            dao = dao,
            sealer = sealer,
            destroyKey = { keysDestroyed++ },
            identity = ClientIdentity("0.1.0", ClientKinds.KITCHEN),
            relocator =
                ServerRelocator(
                    browser = { emptyList() },
                    probe = { _, _ -> false },
                    clock = { 0L },
                ),
            link = FixedServerLink(LinkState.OFFLINE),
            scope = TestScope(UnconfinedTestDispatcher(testScheduler)),
            // El borrado corre en el reloj de la prueba (en la aplicación, fuera del hilo
            // principal).
            wipe =
                DeviceDataWipe(UnconfinedTestDispatcher(testScheduler)).apply {
                    register("prueba") { erased += "prueba" }
                },
        )

    private fun paired() =
        PairedDevice(
            serverId = "0b6f3a52-6b52-4c3e-9a55-2f1d1f4b7d10",
            serverName = "Restaurante de prueba",
            baseUrl = "https://10.0.2.2:8471/",
            fingerprintHex = "00".repeat(32),
            deviceId = "6a1c8e7d-3f0b-4e2a-8c55-1d9e0f2b3a47",
            deviceName = "Cocina 1",
            sealedDeviceToken = sealer.seal("t".repeat(43).toByteArray(), "device-credential"),
            pairedAtMillis = 1L,
        )

    @Test
    fun sinServerEmparejadoBuscaElServidor() = runTest {
        val access = access()
        access.start()
        assertEquals(KitchenStatus.Unpaired, access.status.value)
        assertNull(access.serverApi())
    }

    @Test
    fun alEmparejarseGuardaElServidorYMuestraLosPedidos() = runTest {
        val access = access()
        access.start()
        access.onPaired(paired())
        assertEquals(KitchenStatus.Ready("Restaurante de prueba", "Cocina 1"), access.status.value)
        // La conexión para el canal de eventos: la credencial del dispositivo, sin sesión.
        val channel = access.channelServer()
        assertEquals(null, channel?.userId)
        assertEquals(
            "6a1c8e7d-3f0b-4e2a-8c55-1d9e0f2b3a47",
            channel?.credentials?.get("X-Device-ID"),
        )
        assertEquals("Cocina 1", dao.stored?.deviceName)
        // El mismo propósito de la credencial que la aplicación de meseros (K1-D2).
        assertEquals("device-credential", PairedDevice.CREDENTIAL_PURPOSE)
        assertTrue(access.serverApi() != null)
    }

    @Test
    fun laCredencialQueYaNoSeAbrePideEmparejarDeNuevo() = runTest {
        val access = access()
        access.onPaired(paired())
        sealer.keyLost = true
        access.start()
        assertEquals(
            KitchenStatus.NeedsRepair(RepairReason.CREDENTIAL_LOST),
            access.status.value,
        )
    }

    @Test
    fun revocadaVuelveABuscarElServidorConTodoBorrado() = runTest {
        val access = access()
        access.onPaired(paired())
        access.report(AppError.DeviceRevoked(serverMessage = null, requestId = null))
        advanceUntilIdle()
        assertEquals(KitchenStatus.Unpaired, access.status.value)
        assertNull(dao.stored)
        assertEquals(listOf("prueba"), erased)
        assertEquals(1, keysDestroyed)
    }

    @Test
    fun cadaErrorDeGoLlevaASuPantalla() = runTest {
        val access = access()
        access.onPaired(paired())
        access.report(AppError.ServerIdentityChanged)
        advanceUntilIdle()
        assertEquals(
            KitchenStatus.NeedsRepair(RepairReason.IDENTITY_CHANGED),
            access.status.value,
        )
        access.report(
            AppError.UpdateRequired(
                code = "CLIENT_UPDATE_REQUIRED",
                minClientVersion = "0.2.0",
                serverVersion = "0.16.0",
                serverMessage = null,
                requestId = null,
            ),
        )
        advanceUntilIdle()
        assertEquals(KitchenStatus.UpdateApp("0.1.0", "0.2.0"), access.status.value)
        access.report(AppError.DeviceNotRecognized("DEVICE_UNKNOWN", null, null))
        advanceUntilIdle()
        assertEquals(KitchenStatus.NeedsRepair(RepairReason.NOT_RECOGNIZED), access.status.value)
        // Un servidor que no conoce el tipo `kitchen`: «Actualiza el servidor del restaurante».
        access.report(
            AppError.UpdateRequired(
                code = "CLIENT_KIND_INVALID",
                minClientVersion = null,
                serverVersion = null,
                serverMessage = null,
                requestId = null,
            ),
        )
        advanceUntilIdle()
        assertEquals(KitchenStatus.UpdateServer(null), access.status.value)
        // El dispositivo no es una pantalla de cocina para Go: emparejarla de nuevo.
        access.report(AppError.Rejected(400, "CLIENT_KIND_MISMATCH", null, null))
        advanceUntilIdle()
        assertEquals(KitchenStatus.NeedsRepair(RepairReason.NOT_RECOGNIZED), access.status.value)
        // Otro rechazo o una falta de respuesta no cambian nada.
        access.report(AppError.Rejected(409, "KITCHEN_NOT_RECOVERABLE", null, null))
        access.report(AppError.Unreachable)
        advanceUntilIdle()
        assertEquals(KitchenStatus.NeedsRepair(RepairReason.NOT_RECOGNIZED), access.status.value)
    }

    @Test
    fun emparejarDeNuevoBorraTodo() = runTest {
        val access = access()
        access.onPaired(paired())
        access.forgetServer()
        assertEquals(KitchenStatus.Unpaired, access.status.value)
        assertNull(dao.stored)
        assertEquals(1, keysDestroyed)
    }

    @Test
    fun loQueExigeAlServidorLaPantallaDeCocina() {
        val requirements = KitchenServerRequirements.REQUIREMENTS
        val kitchen = listOf("waiter-pin-v1", KitchenServerRequirements.CAPABILITY_KITCHEN_DISPLAY)
        // Sin la capacidad de la pantalla de cocina (un servidor anterior a la etapa K1).
        assertEquals(
            ServerCompatibility.Verdict.UpdateServer("0.15.0"),
            ServerCompatibility.check(
                "0.1.0",
                "0.15.0",
                listOf("waiter-pin-v1"),
                "0.1.0",
                requirements,
            ),
        )
        // Con la capacidad pero anterior a la 0.16.0 (K1-D27): también pide actualizar el servidor.
        assertEquals(
            ServerCompatibility.Verdict.UpdateServer("0.15.0"),
            ServerCompatibility.check("0.1.0", "0.15.0", kitchen, "0.1.0", requirements),
        )
        assertEquals(
            ServerCompatibility.Verdict.Compatible,
            ServerCompatibility.check("0.1.0", "0.16.0", kitchen, "0.1.0", requirements),
        )
        // La mínima de cocina que anuncia el servidor.
        assertEquals(
            ServerCompatibility.Verdict.UpdateApp("0.2.0", "0.16.0"),
            ServerCompatibility.check("0.1.0", "0.16.0", kitchen, "0.2.0", requirements),
        )
    }
}
