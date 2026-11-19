package com.adelpos.meseros.core.data.access

import android.content.Context
import androidx.room.Room
import androidx.test.core.app.ApplicationProvider
import androidx.test.ext.junit.runners.AndroidJUnit4
import com.adelpos.meseros.core.data.api.ClientIdentity
import com.adelpos.meseros.core.data.db.AppDatabase
import com.adelpos.meseros.core.data.discovery.NsdServerBrowser
import com.adelpos.meseros.core.domain.access.DeviceStatus
import com.adelpos.meseros.core.domain.access.NoUnsentOrders
import com.adelpos.meseros.core.domain.connection.FixedServerLink
import com.adelpos.meseros.core.domain.error.AppError
import com.adelpos.meseros.core.domain.pairing.FoundServerChoice
import com.adelpos.meseros.core.domain.pairing.PairingProblem
import com.adelpos.meseros.core.domain.pairing.PairingState
import com.adelpos.meseros.core.domain.pairing.PairingStep
import com.adelpos.meseros.core.domain.pairing.PairingTarget
import com.adelpos.meseros.core.domain.pairing.ServerSearch
import com.adelpos.meseros.core.domain.server.FoundServer
import com.adelpos.meseros.core.domain.server.ServerAddress
import com.adelpos.meseros.core.domain.server.ServerAnnouncements
import com.adelpos.meseros.core.domain.server.ServerRelocator
import com.adelpos.meseros.core.domain.text.Hex
import java.security.MessageDigest
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.SupervisorJob
import kotlinx.coroutines.cancel
import kotlinx.coroutines.runBlocking
import org.junit.After
import org.junit.Assert.assertArrayEquals
import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Before
import org.junit.Test
import org.junit.runner.RunWith

/**
 * Emparejamiento con el protocolo de la 9G contra un servidor Go simulado (TLS): compromiso,
 * revelación, código con la huella del primer certificado, aprobación, reclamo único y credencial
 * guardada cifrada; servidor de la búsqueda con otra huella u otro `server_id`; estados finales.
 */
@RunWith(AndroidJUnit4::class)
class PairingRepositoryTest {
    private val context: Context = ApplicationProvider.getApplicationContext()
    private lateinit var go: FakeGoServer
    private lateinit var database: AppDatabase
    private val sealer = FakeSecretSealer()
    private val scope = CoroutineScope(SupervisorJob() + Dispatchers.IO)
    private val identity = ClientIdentity("0.1.0")
    private lateinit var access: AccessRepository
    private var found: List<FoundServer> = emptyList()

    @Before
    fun start() {
        go = FakeGoServer()
        database = Room.inMemoryDatabaseBuilder(context, AppDatabase::class.java).build()
        access =
            AccessRepository(
                dao = database.access(),
                sealer = sealer,
                destroyKey = {},
                identity = identity,
                relocator = ServerRelocator({ emptyList() }, { _, _ -> false }, { 0L }),
                link = FixedServerLink(),
                unsentOrders = NoUnsentOrders,
                scope = scope,
                verifierIterations = 1_000,
            )
    }

    @After
    fun stop() {
        go.close()
        database.close()
        scope.cancel()
    }

    // El servidor simulado escucha en loopback, que la validación real rechaza: aquí cualquier
    // dirección escrita lleva a él.
    private fun pairing(
        normalize: (String) -> ServerAddress.Result = { ServerAddress.Result.Valid(go.base) },
    ) =
        PairingRepository(
            browse = { NsdServerBrowser.Browse.Found(found) },
            identity = identity,
            osLabel = "Android 13",
            sealer = sealer,
            access = access,
            normalizeAddress = normalize,
        )

    private fun PairingStep.view() = (this as PairingStep.Showing).view

    @Test
    fun emparejamientoCompletoConElCodigoDeLaHuellaFijada() = runBlocking {
        val flow = pairing()
        val step = flow.start(PairingTarget.Address("caja.local"), "  Tableta 1  ")
        val view = step.view()
        assertEquals(PairingState.PENDING, view.state)
        // El código es el que calcula Go con su propia huella: la del certificado que vio el
        // dispositivo.
        assertEquals(go.expectedCode(), view.code)
        assertEquals("Fonda Doña Ñeli", view.serverName)
        assertEquals("Tableta 1", view.deviceName)
        assertEquals(
            AccessRepository.epochMillis("2099-01-01T06:05:00.5Z"),
            view.expiresAtEpochMillis,
        )
        val created = go.requests.first { it.url.encodedPath == "/api/v1/pairing/requests" }
        val body = created.body!!.utf8()
        assertTrue(body.contains("\"device_kind\":\"android\""))
        assertTrue(body.contains("\"os\":\"Android 13\""))
        assertTrue(body.contains("\"name\":\"Tableta 1\""))
        // El compromiso enviado es SHA-256 del número que se reveló después.
        val commitment = Regex("\"commitment\":\"([0-9a-f]{64})\"").find(body)!!.groupValues[1]
        assertEquals(
            Hex.encode(MessageDigest.getInstance("SHA-256").digest(go.revealedNonce)),
            commitment,
        )

        assertEquals(PairingState.PENDING, flow.poll().view().state)
        go.pairingState = "approved"
        val paired = flow.poll().view()
        assertEquals(PairingState.PAIRED, paired.state)
        assertEquals("Tableta de la terraza", paired.deviceName)
        assertEquals(1, go.claims)
        // Guardado: la huella fijada y la credencial cifrada; después, la entrada con PIN.
        val stored = database.access().pairedServer()!!
        assertEquals(Hex.encode(go.fingerprint), stored.fingerprintHex)
        assertEquals(go.serverId, stored.serverId)
        assertArrayEquals(
            go.deviceToken.toByteArray(),
            sealer.open(stored.sealedDeviceToken, AccessRepository.DEVICE_PURPOSE),
        )
        assertTrue(access.status.value is DeviceStatus.Ready)
        // La sesión terminó: no se reclama otra vez.
        assertEquals(PairingStep.Failed(PairingProblem.NoSession), flow.poll())
        assertEquals(1, go.claims)
    }

    @Test
    fun elServidorDeLaBusquedaDebeTenerLaHuellaYElIdAnunciados() = runBlocking {
        val prefix = ServerAnnouncements.shortFingerprint(go.fingerprint)
        val flow = pairing()
        found =
            listOf(
                FoundServer(
                    go.serverId,
                    "Caja",
                    "0011223344556677",
                    go.server.port,
                    listOf(go.server.hostName),
                ),
            )
        assertEquals(
            ServerSearch.Found(listOf(FoundServerChoice(go.serverId, "Caja"))),
            flow.search(),
        )
        assertEquals(
            PairingStep.Failed(PairingProblem.AnnouncementMismatch),
            flow.start(PairingTarget.Found(go.serverId), "Tableta 1"),
        )
        assertEquals(
            "no se pidió código a quien no es",
            0,
            go.requests.count { it.url.encodedPath.endsWith("/requests") },
        )

        val otherId = "22222222-2222-4333-8444-555555555555"
        found =
            listOf(FoundServer(otherId, "Caja", prefix, go.server.port, listOf(go.server.hostName)))
        flow.search()
        assertEquals(
            PairingStep.Failed(PairingProblem.AnnouncementMismatch),
            flow.start(PairingTarget.Found(otherId), "Tableta 1"),
        )

        found =
            listOf(
                FoundServer(
                    go.serverId,
                    "Caja",
                    prefix,
                    go.server.port,
                    listOf(go.server.hostName),
                ),
            )
        flow.search()
        val started = flow.start(PairingTarget.Found(go.serverId), "Tableta 1").view()
        assertEquals(go.expectedCode(), started.code)
        assertEquals(
            PairingStep.Failed(PairingProblem.NoLongerFound),
            flow.start(PairingTarget.Found(otherId), "Tableta 1"),
        )
    }

    // Cuántas solicitudes de código llegaron al servidor.
    private fun codeRequests() =
        go.requests.count { it.url.encodedPath == "/api/v1/pairing/requests" }

    @Test
    fun unServidorAnteriorA012PideActualizarloAntesDePedirElCodigo() = runBlocking {
        // Una computadora principal 0.11.0: sin la entrada con PIN ni la mínima por tipo.
        go.serverVersion = "0.11.0"
        go.capabilities = listOf("offline-orders-v1")
        go.informsClientVersions = false
        go.createRejection = 426 to "CLIENT_UPDATE_REQUIRED"
        val flow = pairing()
        assertEquals(
            "con la dirección escrita a mano",
            PairingStep.Failed(PairingProblem.UpdateServer("0.11.0")),
            flow.start(PairingTarget.Address("caja.local"), "Tableta 1"),
        )
        found =
            listOf(
                FoundServer(
                    go.serverId,
                    "Caja",
                    ServerAnnouncements.shortFingerprint(go.fingerprint),
                    go.server.port,
                    listOf(go.server.hostName),
                ),
            )
        flow.search()
        assertEquals(
            "con el servidor encontrado en la red",
            PairingStep.Failed(PairingProblem.UpdateServer("0.11.0")),
            flow.start(PairingTarget.Found(go.serverId), "Tableta 1"),
        )
        assertEquals("no se pidió ningún código", 0, codeRequests())

        // 0.12.0 (la A1, sin Para llevar) tampoco sirve para la 0.2.0, ni un 0.13.0 sin la
        // capacidad del PIN.
        go.serverVersion = "0.12.0"
        go.capabilities = listOf("offline-orders-v1", "client-versions-by-kind", "waiter-pin-v1")
        assertEquals(
            PairingStep.Failed(PairingProblem.UpdateServer("0.12.0")),
            flow.start(PairingTarget.Address("caja.local"), "Tableta 1"),
        )
        go.serverVersion = "0.13.0"
        go.capabilities =
            listOf("offline-orders-v1", "client-versions-by-kind", "takeaway-v1", "ready-alerts-v1")
        assertEquals(
            PairingStep.Failed(PairingProblem.UpdateServer("0.13.0")),
            flow.start(PairingTarget.Address("caja.local"), "Tableta 1"),
        )
        assertEquals(0, codeRequests())
    }

    @Test
    fun conUnServidor012SigueElEmparejamientoDespuesDeComprobarLaVersion() = runBlocking {
        val flow = pairing()
        val view = flow.start(PairingTarget.Address("caja.local"), "Tableta 1").view()
        assertEquals(PairingState.PENDING, view.state)
        assertEquals(go.expectedCode(), view.code)
        val paths = go.requests.map { it.url.encodedPath }
        assertEquals(
            "la versión se pide antes que el código",
            listOf("/api/v1/version", "/api/v1/pairing/requests"),
            paths.take(2),
        )
        // Si el servidor pide una aplicación más reciente: «Actualiza esta aplicación», sin pedir
        // código.
        go.minAndroid = "0.2.0"
        val before = codeRequests()
        val step = flow.start(PairingTarget.Address("caja.local"), "Tableta 1")
        val problem = (step as PairingStep.Failed).problem as PairingProblem.Server
        assertEquals("0.2.0", (problem.error as AppError.UpdateRequired).minClientVersion)
        assertEquals(before, codeRequests())
    }

    @Test
    fun un426DeUnServidorQueNoInformaLaMinimaEsUnServidorViejo() = runBlocking {
        val flow = pairing()
        // GET version rechazado con 426: no hay mínima informada.
        go.versionRejection = 426 to "CLIENT_UPDATE_REQUIRED"
        assertEquals(
            PairingStep.Failed(PairingProblem.UpdateServer(null)),
            flow.start(PairingTarget.Address("caja.local"), "Tableta 1"),
        )
        // La versión responde (0.13.0 con las capacidades) pero sin la mínima por tipo, y la
        // solicitud responde 426 (o tipo de cliente desconocido): también es un servidor viejo.
        go.versionRejection = null
        go.informsClientVersions = false
        go.createRejection = 426 to "CLIENT_UPDATE_REQUIRED"
        assertEquals(
            PairingStep.Failed(PairingProblem.UpdateServer("0.13.0")),
            flow.start(PairingTarget.Address("caja.local"), "Tableta 1"),
        )
        go.createRejection = 400 to "CLIENT_KIND_INVALID"
        assertEquals(
            PairingStep.Failed(PairingProblem.UpdateServer("0.13.0")),
            flow.start(PairingTarget.Address("caja.local"), "Tableta 1"),
        )
        // Con la mínima informada, un 426 sí es «Actualiza esta aplicación».
        go.informsClientVersions = true
        go.createRejection = 426 to "CLIENT_UPDATE_REQUIRED"
        val step = flow.start(PairingTarget.Address("caja.local"), "Tableta 1")
        val problem = (step as PairingStep.Failed).problem
        assertTrue("$problem", problem is PairingProblem.Server)
        assertTrue((problem as PairingProblem.Server).error is AppError.UpdateRequired)
    }

    @Test
    fun estadosFinalesYRechazos() = runBlocking {
        val flow = pairing()
        flow.start(PairingTarget.Address("x"), "Tableta 1")
        go.pairingState = "expired"
        assertEquals(PairingState.EXPIRED, flow.poll().view().state)
        assertEquals(PairingStep.Failed(PairingProblem.NoSession), flow.poll())

        flow.start(PairingTarget.Address("x"), "Tableta 1")
        go.pairingState = "rejected"
        assertEquals(PairingState.REJECTED, flow.poll().view().state)

        go.createRejection = 429 to "PAIRING_RATE_LIMITED"
        val limited = flow.start(PairingTarget.Address("x"), "Tableta 1")
        val error = ((limited as PairingStep.Failed).problem as PairingProblem.Server).error
        assertEquals(AppError.TryLater("PAIRING_RATE_LIMITED", "Mensaje de Go.", "r-1"), error)
    }

    @Test
    fun nombreYDireccionSeValidanAntesDePedirNada() = runBlocking {
        val flow = pairing(ServerAddress::normalize)
        assertEquals(
            PairingStep.Failed(PairingProblem.InvalidName),
            flow.start(PairingTarget.Address("caja.local"), "  "),
        )
        assertEquals(
            PairingStep.Failed(PairingProblem.InvalidName),
            flow.start(PairingTarget.Address("caja.local"), "Tableta\n1"),
        )
        assertEquals(
            PairingStep.Failed(PairingProblem.InvalidAddress(ServerAddress.Problem.NOT_LOCAL)),
            flow.start(PairingTarget.Address(go.base), "Tableta 1"),
        )
        assertEquals(
            PairingStep.Failed(PairingProblem.InvalidAddress(ServerAddress.Problem.MALFORMED)),
            flow.start(PairingTarget.Address("http://192.168.1.20"), "Tableta 1"),
        )
        assertEquals(0, go.requests.size)
    }

    @Test
    fun cambiarElNombreYCancelar() = runBlocking {
        val flow = pairing()
        flow.start(PairingTarget.Address("x"), "Tableta 1")
        assertEquals("Tableta 2", flow.rename("Tableta 2").view().deviceName)
        flow.cancel()
        assertTrue(go.requests.any { it.url.encodedPath.endsWith("/cancel") })
        assertEquals(PairingStep.Failed(PairingProblem.NoSession), flow.poll())
    }
}
