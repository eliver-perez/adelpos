package local.adel.pos.android.core.data.access

import android.content.Context
import androidx.room.Room
import androidx.test.core.app.ApplicationProvider
import androidx.test.ext.junit.runners.AndroidJUnit4
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.SupervisorJob
import kotlinx.coroutines.cancel
import kotlinx.coroutines.delay
import kotlinx.coroutines.flow.first
import kotlinx.coroutines.runBlocking
import kotlinx.coroutines.withContext
import kotlinx.coroutines.withTimeout
import local.adel.pos.android.core.data.api.ClientIdentity
import local.adel.pos.android.core.data.db.AppDatabase
import local.adel.pos.android.core.data.db.PairedServerEntity
import local.adel.pos.android.core.data.wipe.DeviceDataWipe
import local.adel.pos.android.core.domain.access.ActiveWaiter
import local.adel.pos.android.core.domain.access.EntryResult
import local.adel.pos.android.core.domain.access.OfflineAttemptPolicy
import local.adel.pos.android.core.domain.access.UnsentOrder
import local.adel.pos.android.core.domain.error.AppError
import local.adel.pos.android.core.domain.server.ServerRelocator
import local.adel.pos.android.core.domain.text.Hex
import okhttp3.Request
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
 * Lo que encontró la revisión independiente R3 (§3), probado al revés: cada prueba reproduce el
 * escenario del hallazgo y comprueba que ya no ocurre.
 * - A1 y B7: una entrada sin conexión nunca lleva la sesión de Go (se mira la petición real).
 * - M1: el límite sin conexión no depende del reloj.
 * - M2: el bloqueo de Go y los fallos con conexión cierran la entrada sin conexión (A1-D16).
 * - M3: un acceso que deja de valer nunca saca al mesero: le pide volver a escribir su PIN.
 * - M4: retirar el dispositivo borra lo registrado, después de pasar los pendientes a la bitácora.
 * - M5: las cabeceras son las del mesero dueño de la operación.
 */
@RunWith(AndroidJUnit4::class)
class AccessSafetyTest {
    private val context: Context = ApplicationProvider.getApplicationContext()
    private lateinit var go: FakeGoServer
    private lateinit var database: AppDatabase
    private val sealer = FakeSecretSealer()
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

    private fun repository(wipe: DeviceDataWipe = DeviceDataWipe()) =
        AccessRepository(
            dao = database.access(),
            sealer = sealer,
            destroyKey = {},
            identity = identity,
            relocator = ServerRelocator({ emptyList() }, { _, _ -> false }, { 0L }),
            isOnline = { online },
            unsentOrders = { unsent.toList() },
            scope = scope,
            wipe = wipe,
            wallClock = { now },
            verifierIterations = 1_000,
        )

    private suspend fun AccessRepository.pair() {
        onPaired(
            PairedServerEntity(
                serverId = go.serverId,
                serverName = "Fonda",
                baseUrl = go.base,
                fingerprintHex = Hex.encode(go.fingerprint),
                deviceId = go.deviceId,
                deviceName = "Tableta 1",
                sealedDeviceToken =
                    sealer.seal(go.deviceToken.toByteArray(), AccessRepository.DEVICE_PURPOSE),
                idleLockSeconds = 120,
                pairedAtMillis = now,
            ),
        )
    }

    // Una petición de verdad a Go con las cabeceras de [userId], como la haría la Fase D o E;
    // devuelve la cabecera Authorization que recibió el servidor.
    private suspend fun AccessRepository.authorizationSentFor(userId: String): String? {
        val transport = checkNotNull(serverTransport())
        val headers = checkNotNull(requestHeaders(userId))
        val request =
            Request.Builder()
                .url(transport.baseUrl.resolve("/api/v1/restaurant/state")!!)
                .headers(headers)
                .build()
        withContext(Dispatchers.IO) { transport.client.newCall(request).execute().close() }
        val received = go.requests.last { it.url.encodedPath.endsWith("restaurant/state") }
        return received.headers["Authorization"]
    }

    private suspend fun AccessRepository.awaitActive(
        predicate: (ActiveWaiter?) -> Boolean,
    ): ActiveWaiter? = withTimeout(5_000) { activeWaiter.first(predicate) }

    @Test
    fun unaEntradaSinConexionNuncaLlevaLaSesionDeGo() = runBlocking {
        val access = repository()
        access.pair()
        assertTrue(access.enter(ana, "4829") is EntryResult.Entered)
        assertEquals("Bearer ${"a".repeat(42)}1", access.authorizationSentFor(ana))
        // Ana se va (cambio de mesero o bloqueo: su sesión de Go no se cierra en Go).
        access.leave()
        online = false
        val entered = access.enter(ana, "4829")
        assertTrue(entered is EntryResult.Entered)
        assertFalse((entered as EntryResult.Entered).waiter.hasServerSession)
        // Vuelve la red: lo que salga a nombre de Ana va sin sesión hasta que Go verifique su PIN.
        online = true
        assertNull(access.sessionToken(ana))
        assertNull(access.authorizationSentFor(ana))
        assertEquals(go.deviceId, access.requestHeaders(ana)?.get("X-Device-ID"))
    }

    @Test
    fun adelantarElRelojNoDaMasIntentosSinConexion() = runBlocking {
        val access = repository()
        access.pair()
        assertTrue(access.enter(ana, "4829") is EntryResult.Entered)
        access.leave()
        online = false
        repeat(OfflineAttemptPolicy.MAX_FAILURES - 1) {
            assertEquals(EntryResult.WrongPin, access.enter(ana, "1111"))
        }
        assertEquals(EntryResult.OfflineClosed, access.enter(ana, "1111"))
        // Alguien adelanta la hora del dispositivo (Ajustes › Fecha y hora) y reinicia la app.
        now += 10L * 365 * 24 * 60 * 60 * 1000
        assertEquals(EntryResult.OfflineClosed, access.enter(ana, "4829"))
        val reopened = repository()
        reopened.start()
        assertEquals(EntryResult.OfflineClosed, reopened.enter(ana, "4829"))
    }

    @Test
    fun losFallosSoloVuelvenACeroConUnaEntradaCorrecta() = runBlocking {
        val access = repository()
        access.pair()
        assertTrue(access.enter(ana, "4829") is EntryResult.Entered)
        access.leave()
        online = false
        repeat(OfflineAttemptPolicy.MAX_FAILURES - 1) {
            assertEquals(EntryResult.WrongPin, access.enter(ana, "1111"))
        }
        assertTrue(access.enter(ana, "4829") is EntryResult.Entered)
        access.leave()
        repeat(OfflineAttemptPolicy.MAX_FAILURES - 1) {
            assertEquals(EntryResult.WrongPin, access.enter(ana, "1111"))
        }
        assertEquals(EntryResult.OfflineClosed, access.enter(ana, "1111"))
    }

    @Test
    fun losFallosConConexionCuentanEnElMismoLimite() = runBlocking {
        val access = repository()
        access.pair()
        assertTrue(access.enter(ana, "4829") is EntryResult.Entered)
        access.leave()
        repeat(3) { assertEquals(EntryResult.WrongPin, access.enter(ana, "1111")) }
        online = false
        assertEquals(EntryResult.WrongPin, access.enter(ana, "1111"))
        assertEquals(EntryResult.OfflineClosed, access.enter(ana, "1111"))
    }

    @Test
    fun elBloqueoDeGoCierraLaEntradaSinConexion() = runBlocking {
        val access = repository()
        access.pair()
        assertTrue(access.enter(ana, "4829") is EntryResult.Entered)
        access.leave()
        go.lockedUntil = "2099-01-01T00:00:00Z"
        assertTrue(access.enter(ana, "4829") is EntryResult.Locked)
        online = false
        assertEquals(
            "con el PIN bloqueado en Go no se prueba sin conexión",
            EntryResult.OfflineClosed,
            access.enter(ana, "4829"),
        )
        // Go lo desbloquea y Ana entra con conexión: la entrada sin conexión se abre otra vez.
        online = true
        go.lockedUntil = null
        access.recheck()
        assertTrue(access.enter(ana, "4829") is EntryResult.Entered)
        access.leave()
        online = false
        assertTrue(access.enter(ana, "4829") is EntryResult.Entered)
    }

    @Test
    fun elBloqueoDeLaListaDeGoTambienCierraLaEntradaSinConexion() = runBlocking {
        val access = repository()
        access.pair()
        assertTrue(access.enter(beto, "6152") is EntryResult.Entered)
        access.leave()
        // Beto quedó bloqueado en Go hasta que lo desbloquee un administrador (A1-D16): se ve en
        // la lista antes de que nadie lo intente aquí.
        go.requiresUnlock = true
        access.recheck()
        assertTrue(access.roster().waiters.first { it.userId == beto }.requiresUnlock)
        assertEquals(
            EntryResult.Locked(untilEpochMillis = null, requiresUnlock = true),
            access.enter(beto, "6152"),
        )
        online = false
        assertEquals(EntryResult.OfflineClosed, access.enter(beto, "6152"))
        assertTrue(access.roster().waiters.single().offlineEntryClosed)
    }

    @Test
    fun unaSesionTerminadaNoSacaAlMeseroNiPierdeSuTrabajo() = runBlocking {
        val access = repository()
        access.pair()
        assertTrue(access.enter(ana, "4829") is EntryResult.Entered)
        access.report(AppError.SessionEnded("SESSION_EXPIRED", null, null), ana)
        val waiter = access.awaitActive { it?.mustReenterPin == true }!!
        assertEquals("sigue siendo Ana, con lo que capturaba", ana, waiter.userId)
        assertFalse(waiter.hasServerSession)
        assertNull(access.sessionToken(ana))
        assertNull(access.authorizationSentFor(ana))
        // Vuelve a escribir su PIN: sesión nueva, sin pasar por la lista de meseros.
        val again = access.enter(ana, "4829")
        assertTrue(again is EntryResult.Entered)
        assertFalse(access.activeWaiter.value!!.mustReenterPin)
        assertEquals("Bearer ${"a".repeat(42)}2", access.authorizationSentFor(ana))
    }

    @Test
    fun elPinCambiadoPideVolverAEscribirloSinSacarAlMesero() = runBlocking {
        val access = repository()
        access.pair()
        assertTrue(access.enter(ana, "4829") is EntryResult.Entered)
        go.waiters[ana] = Triple("Ana", "7305", 2)
        access.recheck()
        val waiter = access.activeWaiter.value!!
        assertEquals(ana, waiter.userId)
        assertTrue(waiter.mustReenterPin)
        assertNull(access.sessionToken(ana))
        assertNull("el verificador viejo se borró", access.offlineCredential(ana))
        assertEquals(EntryResult.WrongPin, access.enter(ana, "4829"))
        assertTrue(access.enter(ana, "7305") is EntryResult.Entered)
        assertFalse(access.activeWaiter.value!!.mustReenterPin)
        assertNotNull(access.offlineCredential(ana))
    }

    @Test
    fun unaCredencialDeEnvioQueYaNoValePideEntrarConConexion() = runBlocking {
        val access = repository()
        access.pair()
        assertTrue(access.enter(ana, "4829") is EntryResult.Entered)
        access.report(
            AppError.Rejected(401, AccessRepository.OFFLINE_CREDENTIAL_INVALID, null, null),
            ana,
        )
        access.awaitActive { it?.mustReenterPin == true }
        assertNull(database.access().knownWaiter(ana))
        online = false
        assertEquals(EntryResult.UnknownOffline, access.enter(ana, "4829"))
        assertEquals(ana, access.activeWaiter.value?.userId)
    }

    @Test
    fun lasCabecerasSonDelMeseroDuenoDeLaOperacion() = runBlocking {
        val access = repository()
        access.pair()
        assertTrue(access.enter(ana, "4829") is EntryResult.Entered)
        val anaSession = "Bearer ${access.sessionToken(ana)}"
        // Cambio rápido a Beto: una operación que Ana lanzó y termina ahora sale como de Ana.
        access.leave()
        assertTrue(access.enter(beto, "6152") is EntryResult.Entered)
        val betoSession = "Bearer ${access.sessionToken(beto)}"
        assertEquals(anaSession, access.authorizationSentFor(ana))
        assertEquals(betoSession, access.authorizationSentFor(beto))
        // La sesión terminada de Ana no toca a Beto.
        access.report(AppError.SessionEnded("SESSION_EXPIRED", null, null), ana)
        withTimeout(5_000) {
            while (access.sessionToken(ana) != null) {
                delay(10)
            }
        }
        assertEquals(beto, access.activeWaiter.value?.userId)
        assertFalse(access.activeWaiter.value!!.mustReenterPin)
        assertEquals(betoSession, access.authorizationSentFor(beto))
        assertNull(access.authorizationSentFor(ana))
    }

    @Test
    fun retirarBorraLoRegistradoDespuesDePasarLosPendientesALaBitacora() = runBlocking {
        val seen = mutableListOf<String>()
        val wipe = DeviceDataWipe()
        wipe.register("catálogo e imágenes") {
            // Al borrar, los pendientes ya están en la bitácora.
            seen.add("catálogo con ${database.access().deviceLog().size} en la bitácora")
        }
        wipe.register("parte que falla") { error("disco lleno") }
        wipe.register("cola") { seen.add("cola") }
        val access = repository(wipe)
        access.pair()
        access.enter(ana, "4829")
        unsent.add(UnsentOrder("Ana", "Mesa 4: 2 hamburguesas", now - 60_000))
        assertEquals(unsent, access.pendingOrders())
        go.revoked = true
        access.recheck()
        assertEquals("una parte que falla no detiene a las demás", 2, seen.size)
        assertEquals(listOf("catálogo con 1 en la bitácora", "cola"), seen)
        assertNull(database.access().pairedServer())
    }
}
