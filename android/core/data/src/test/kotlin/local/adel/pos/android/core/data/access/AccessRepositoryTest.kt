package local.adel.pos.android.core.data.access

import android.content.Context
import androidx.room.Room
import androidx.test.core.app.ApplicationProvider
import androidx.test.ext.junit.runners.AndroidJUnit4
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.SupervisorJob
import kotlinx.coroutines.cancel
import kotlinx.coroutines.flow.first
import kotlinx.coroutines.runBlocking
import kotlinx.coroutines.withTimeout
import local.adel.pos.android.core.data.api.ClientIdentity
import local.adel.pos.android.core.data.db.AppDatabase
import local.adel.pos.android.core.data.db.PairedServerEntity
import local.adel.pos.android.core.data.discovery.HttpsServerProbe
import local.adel.pos.android.core.domain.access.DeviceStatus
import local.adel.pos.android.core.domain.access.EntryResult
import local.adel.pos.android.core.domain.access.NoWaiterWork
import local.adel.pos.android.core.domain.access.OfflineAttemptPolicy
import local.adel.pos.android.core.domain.access.RepairReason
import local.adel.pos.android.core.domain.access.UnsentOrder
import local.adel.pos.android.core.domain.error.AppError
import local.adel.pos.android.core.domain.server.FoundServer
import local.adel.pos.android.core.domain.server.ServerAnnouncements
import local.adel.pos.android.core.domain.server.ServerRelocator
import local.adel.pos.android.core.domain.text.Hex
import okhttp3.tls.HeldCertificate
import org.junit.After
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNotNull
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Before
import org.junit.Test
import org.junit.runner.RunWith

/**
 * Entrada al dispositivo en la JVM (Robolectric SDK 36): Room en memoria, un Keystore falso y un
 * servidor Go simulado con TLS. Entrada con y sin conexión, límite local, invalidación por
 * `pin_version`, revocación, versiones, huella distinta y cambio de IP.
 */
@RunWith(AndroidJUnit4::class)
class AccessRepositoryTest {
    private val context: Context = ApplicationProvider.getApplicationContext()
    private lateinit var go: FakeGoServer
    private lateinit var database: AppDatabase
    private var sealer = FakeSecretSealer()
    private var keyDestroyed = false
    private var online = true
    private var now = 1_700_000_000_000L
    private val unsent = mutableListOf<UnsentOrder>()
    private val scope = CoroutineScope(SupervisorJob() + Dispatchers.IO)
    private val identity = ClientIdentity("0.1.0")
    private val ana = "aaaaaaaa-0000-4000-8000-000000000001"
    private val beto = "bbbbbbbb-0000-4000-8000-000000000002"

    @Before
    fun start() {
        go = FakeGoServer()
        go.waiters[ana] = Triple("Ana", "4829", 1)
        go.waiters[beto] = Triple("Beto", "6152", 1)
        database = Room.inMemoryDatabaseBuilder(context, AppDatabase::class.java).build()
    }

    @After
    fun stop() {
        go.close()
        database.close()
        scope.cancel()
    }

    private fun repository(
        relocator: ServerRelocator = ServerRelocator({ emptyList() }, { _, _ -> false }, { 0L }),
    ) =
        AccessRepository(
            dao = database.access(),
            sealer = sealer,
            destroyKey = { keyDestroyed = true },
            identity = identity,
            relocator = relocator,
            isOnline = { online },
            work = NoWaiterWork,
            unsentOrders = { unsent.toList() },
            scope = scope,
            wallClock = { now },
            verifierIterations = 1_000,
        )

    private suspend fun AccessRepository.pair(
        base: String = go.base,
        fingerprint: ByteArray = go.fingerprint,
    ) {
        onPaired(
            PairedServerEntity(
                serverId = go.serverId,
                serverName = "Fonda Doña Ñeli",
                baseUrl = base,
                fingerprintHex = Hex.encode(fingerprint),
                deviceId = go.deviceId,
                deviceName = "Tableta 1",
                sealedDeviceToken =
                    sealer.seal(go.deviceToken.toByteArray(), AccessRepository.DEVICE_PURPOSE),
                idleLockSeconds = 120,
                pairedAtMillis = now,
            ),
        )
    }

    @Test
    fun sinServidorEmparejadoPideBuscarlo() = runBlocking {
        val access = repository()
        access.start()
        assertEquals(DeviceStatus.Unpaired(), access.status.value)
    }

    @Test
    fun conElServidorCompatibleListaLosMeserosYElBloqueoPorInactividad() = runBlocking {
        go.idleLockSeconds = 300
        val access = repository()
        access.pair()
        assertEquals(
            DeviceStatus.Ready("Fonda Doña Ñeli", "Tableta de la terraza", 300, online = true),
            access.status.value,
        )
        val roster = access.roster()
        assertTrue(roster.online)
        assertEquals(listOf("Ana", "Beto"), roster.waiters.map { it.name })
        assertEquals("#3591d4", roster.waiters[0].color)
        // Las rutas del PIN llevan la credencial del dispositivo y las cabeceras del cliente.
        val request = go.requests.last { it.url.encodedPath.endsWith("pin-users") }
        assertEquals(go.deviceId, request.headers["X-Device-ID"])
        assertEquals("android", request.headers["X-Client-Kind"])
        assertNull(request.headers["Authorization"])
    }

    @Test
    fun entrarConConexionGuardaElVerificadorYLaCredencialCifrados() = runBlocking {
        val access = repository()
        access.pair()
        val entered = access.enter(ana, "4829")
        assertTrue(entered is EntryResult.Entered)
        assertTrue((entered as EntryResult.Entered).waiter.hasServerSession)
        assertEquals(ana, access.activeWaiter.value?.userId)
        assertEquals("a".repeat(42) + "1", access.sessionToken(ana))
        assertEquals("c".repeat(42) + "1", access.offlineCredential(ana))
        val stored = database.access().knownWaiter(ana)!!
        assertNotNull(stored.sealedVerifier)
        // Lo guardado no contiene el PIN ni la credencial en claro.
        val raw =
            String(stored.sealedVerifier!! + stored.sealedOfflineCredential!!, Charsets.ISO_8859_1)
        assertFalse(raw.contains("4829") || raw.contains("c".repeat(42)))
        // Una entrada nueva con conexión reemplaza la credencial de envío.
        access.leave()
        access.enter(ana, "4829")
        assertEquals("c".repeat(42) + "2", access.offlineCredential(ana))
    }

    @Test
    fun sinConexionSoloEntraQuienYaEntroAqui() = runBlocking {
        val access = repository()
        access.pair()
        assertTrue(access.enter(ana, "4829") is EntryResult.Entered)
        access.leave()
        online = false
        val before = go.requests.size
        val roster = access.roster()
        assertFalse(roster.online)
        assertEquals(listOf(ana), roster.waiters.map { it.userId })
        // Beto tiene PIN en Go, pero nunca entró en este dispositivo.
        assertEquals(EntryResult.UnknownOffline, access.enter(beto, "6152"))
        assertEquals(EntryResult.WrongPin, access.enter(ana, "4828"))
        val entered = access.enter(ana, "4829")
        assertTrue(entered is EntryResult.Entered)
        assertFalse(
            "sin conexión no hay sesión de Go",
            (entered as EntryResult.Entered).waiter.hasServerSession,
        )
        assertEquals("sin red no se habla con el servidor", before, go.requests.size)
    }

    @Test
    fun cincoFallosSinConexionBloqueanLaEntradaLocal() = runBlocking {
        val access = repository()
        access.pair()
        access.enter(ana, "4829")
        access.leave()
        online = false
        repeat(4) { assertEquals(EntryResult.WrongPin, access.enter(ana, "1111")) }
        val locked = access.enter(ana, "1111")
        assertEquals(EntryResult.Locked(now + OfflineAttemptPolicy.LOCK_MILLIS), locked)
        assertEquals("bloqueado aun con el PIN correcto", locked, access.enter(ana, "4829"))
        assertEquals(
            now + OfflineAttemptPolicy.LOCK_MILLIS,
            access.roster().waiters[0].lockedUntilEpochMillis,
        )
        now += OfflineAttemptPolicy.LOCK_MILLIS
        assertTrue(access.enter(ana, "4829") is EntryResult.Entered)
    }

    @Test
    fun pinIncorrectoBloqueadoYSinPermisoConConexion() = runBlocking {
        val access = repository()
        access.pair()
        assertEquals(EntryResult.WrongPin, access.enter(ana, "1234"))
        assertEquals(EntryResult.WrongPin, access.enter(ana, "12"))
        go.forbidden = setOf(beto)
        assertEquals(EntryResult.NotAllowed, access.enter(beto, "6152"))
        go.lockedUntil = "2026-11-16T14:35:00-06:00"
        assertEquals(
            EntryResult.Locked(AccessRepository.epochMillis("2026-11-16T20:35:00Z")),
            access.enter(ana, "4829"),
        )
        assertNull(access.activeWaiter.value)
        assertNull("un rechazo no guarda verificador", database.access().knownWaiter(ana))
    }

    @Test
    fun elPinCambiadoInvalidaElVerificadorAlReconectar() = runBlocking {
        val access = repository()
        access.pair()
        access.enter(ana, "4829")
        access.enter(beto, "6152")
        access.leave()
        // En la principal: Ana cambia de PIN (sube pin_version) y Beto pierde el permiso.
        go.waiters[ana] = Triple("Ana", "7305", 2)
        go.waiters.remove(beto)
        access.recheck()
        assertNull(database.access().knownWaiter(ana))
        assertNull(database.access().knownWaiter(beto))
        assertNull(access.offlineCredential(ana))
        online = false
        assertEquals(EntryResult.UnknownOffline, access.enter(ana, "4829"))
        assertEquals(emptyList<String>(), access.roster().waiters.map { it.userId })
    }

    @Test
    fun elDispositivoRevocadoBorraTodoYConservaLosPedidosEnLaBitacora() = runBlocking {
        val access = repository()
        access.pair()
        access.enter(ana, "4829")
        val order = UnsentOrder("Ana", "Mesa 4: 2 hamburguesas", now - 60_000)
        unsent.add(order)
        go.revoked = true
        access.recheck()
        assertEquals(DeviceStatus.Retired(listOf(order)), access.status.value)
        assertNull(access.activeWaiter.value)
        assertNull(access.sessionToken(ana))
        assertNull(database.access().pairedServer())
        assertEquals(emptyList<Any>(), database.access().knownWaiters())
        assertTrue("la llave del Keystore se destruye", keyDestroyed)
        assertEquals(
            listOf("Mesa 4: 2 hamburguesas"),
            database.access().deviceLog().map { it.description },
        )
        access.acknowledgeRetirement()
        assertEquals(DeviceStatus.Unpaired(listOf(order)), access.status.value)
        // Al volver a abrir la aplicación la bitácora sigue ahí hasta que se borra.
        val reopened = repository()
        reopened.start()
        assertEquals(DeviceStatus.Unpaired(listOf(order)), reopened.status.value)
        reopened.clearDeviceLog()
        assertEquals(DeviceStatus.Unpaired(), reopened.status.value)
    }

    @Test
    fun laRevocacionAvisadaPorOtraParteTambienRetira() =
        runBlocking<Unit> {
            val access = repository()
            access.pair()
            access.enter(ana, "4829")
            access.report(AppError.SessionEnded("SESSION_REVOKED", null, null), ana)
            withTimeout(5_000) { access.activeWaiter.first { it == null } }
            assertNull(access.sessionToken(ana))
            access.report(AppError.DeviceRevoked(null, null), null)
            withTimeout(5_000) { access.status.first { it is DeviceStatus.Retired } }
        }

    @Test
    fun versionesDeLaAplicacionYDelServidor() = runBlocking {
        go.minAndroid = "0.2.0"
        val access = repository()
        access.pair()
        assertEquals(DeviceStatus.UpdateApp("0.1.0", "0.2.0"), access.status.value)
        go.minAndroid = "0.1.0"
        go.serverVersion = "0.11.0"
        access.recheck()
        assertEquals(DeviceStatus.UpdateServer("0.11.0"), access.status.value)
        go.serverVersion = "0.12.0"
        access.recheck()
        assertTrue(access.status.value is DeviceStatus.Ready)
    }

    @Test
    fun otroCertificadoNuncaSeAceptaYPideEmparejarDeNuevo() = runBlocking {
        val other = HeldCertificate.Builder().commonName("AD-EL POS prueba").build()
        val access = repository()
        access.pair(
            fingerprint =
                FakeGoServer(other).let { impostor ->
                    impostor.close()
                    impostor.fingerprint
                },
        )
        assertEquals(DeviceStatus.NeedsRepair(RepairReason.IDENTITY_CHANGED), access.status.value)
        assertEquals("ninguna petición llegó al servidor", 0, go.requests.size)
        access.forgetServer()
        assertEquals(DeviceStatus.Unpaired(), access.status.value)
        assertNull(database.access().pairedServer())
    }

    @Test
    fun laCredencialIlegiblePideEmparejarDeNuevo() = runBlocking {
        repository().pair()
        sealer.destroy()
        val reopened = repository()
        reopened.start()
        assertEquals(DeviceStatus.NeedsRepair(RepairReason.CREDENTIAL_LOST), reopened.status.value)
    }

    @Test
    fun siCambiaDeIpLoEncuentraConLaMismaHuellaYGuardaLaDireccion() = runBlocking {
        // El servidor se mudó: en la dirección vieja ya no hay nada y en la nueva responde la misma
        // llave.
        val oldBase = go.base
        go.close()
        val moved = FakeGoServer(go.certificate)
        moved.waiters.putAll(go.waiters)
        go = moved
        val announcement =
            FoundServer(
                go.serverId,
                "Fonda",
                ServerAnnouncements.shortFingerprint(go.fingerprint),
                go.server.port,
                listOf(go.server.hostName),
            )
        val relocator =
            ServerRelocator({ listOf(announcement) }, HttpsServerProbe(identity), { 0L })
        val access = repository(relocator)
        access.pair(base = oldBase)
        assertTrue(access.status.value.let { it is DeviceStatus.Ready && it.online })
        assertEquals(go.base, database.access().pairedServer()?.baseUrl)
        assertTrue(access.enter(ana, "4829") is EntryResult.Entered)
    }
}
