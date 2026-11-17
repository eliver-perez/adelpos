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
import kotlinx.coroutines.runBlocking
import kotlinx.coroutines.withTimeout
import local.adel.pos.android.core.data.api.ClientIdentity
import local.adel.pos.android.core.data.db.AppDatabase
import local.adel.pos.android.core.data.db.PairedServerEntity
import local.adel.pos.android.core.data.session.AuthorizedServer
import local.adel.pos.android.core.data.session.DeviceServerAccess
import local.adel.pos.android.core.domain.access.DeviceStatus
import local.adel.pos.android.core.domain.access.EntryResult
import local.adel.pos.android.core.domain.access.NoUnsentOrders
import local.adel.pos.android.core.domain.access.NoWaiterWork
import local.adel.pos.android.core.domain.realtime.AccessInterruption
import local.adel.pos.android.core.domain.server.ServerRelocator
import local.adel.pos.android.core.domain.text.Hex
import okhttp3.HttpUrl.Companion.toHttpUrl
import org.junit.After
import org.junit.Assert.assertEquals
import org.junit.Assert.assertNotEquals
import org.junit.Assert.assertNotNull
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Before
import org.junit.Test
import org.junit.runner.RunWith

/**
 * La interfaz estrecha del catálogo y del canal de tiempo real sobre la entrada al dispositivo:
 * conexión solo con un mesero con sesión de Go, otra al cambiar de mesero, y los avisos de sesión
 * terminada y dispositivo retirado llegan a `AccessRepository.report`.
 */
@RunWith(AndroidJUnit4::class)
class DeviceServerAccessTest {
    private val context: Context = ApplicationProvider.getApplicationContext()
    private val scope = CoroutineScope(SupervisorJob() + Dispatchers.IO)
    private val sealer = FakeSecretSealer()
    private val ana = "aaaaaaaa-0000-4000-8000-000000000001"
    private val beto = "bbbbbbbb-0000-4000-8000-000000000002"
    private lateinit var go: FakeGoServer
    private lateinit var database: AppDatabase
    private lateinit var access: AccessRepository

    @Before
    fun start() {
        go = FakeGoServer()
        go.waiters[ana] = Triple("Ana", "4829", 1)
        go.waiters[beto] = Triple("Beto", "6152", 1)
        database = Room.inMemoryDatabaseBuilder(context, AppDatabase::class.java).build()
        access =
            AccessRepository(
                dao = database.access(),
                sealer = sealer,
                destroyKey = {},
                identity = ClientIdentity("0.1.0"),
                relocator = ServerRelocator({ emptyList() }, { _, _ -> false }, { 0L }),
                isOnline = { true },
                work = NoWaiterWork,
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

    private suspend fun pair() {
        access.onPaired(
            PairedServerEntity(
                serverId = go.serverId,
                serverName = "Fonda Doña Ñeli",
                baseUrl = go.base,
                fingerprintHex = Hex.encode(go.fingerprint),
                deviceId = go.deviceId,
                deviceName = "Tableta 1",
                sealedDeviceToken =
                    sealer.seal(go.deviceToken.toByteArray(), AccessRepository.DEVICE_PURPOSE),
                idleLockSeconds = 120,
                pairedAtMillis = 1,
            ),
        )
    }

    private suspend fun DeviceServerAccess.awaitCurrent(
        predicate: (AuthorizedServer?) -> Boolean,
    ): AuthorizedServer? =
        withTimeout(5_000) {
            while (!predicate(current.value)) {
                delay(10)
            }
            current.value
        }

    @Test
    fun conexionSoloConMeseroConSesionYOtraAlCambiarDeMesero() =
        runBlocking<Unit> {
            val adapter = DeviceServerAccess(access, scope)
            pair()
            assertTrue(access.status.value is DeviceStatus.Ready)
            // Sin mesero dentro no hay conexión autorizada.
            assertNull(adapter.current.value)

            assertTrue(access.enter(ana, "4829") is EntryResult.Entered)
            val anaServer = adapter.awaitCurrent { it?.userId == ana }!!
            assertEquals(go.base.toHttpUrl(), anaServer.baseUrl)
            assertEquals(go.deviceId, anaServer.credentials[AuthorizedServer.DEVICE_ID_HEADER])
            assertEquals(
                "Bearer ${access.sessionToken(ana)}",
                anaServer.credentials[AuthorizedServer.AUTHORIZATION_HEADER],
            )

            access.leave()
            adapter.awaitCurrent { it == null }
            assertTrue(access.enter(beto, "6152") is EntryResult.Entered)
            val betoServer = adapter.awaitCurrent { it?.userId == beto }!!
            assertNotEquals(
                "Otra sesión es otra conexión (el canal se reabre)",
                anaServer,
                betoServer,
            )

            // Sin permiso de eventos el mesero sigue dentro.
            adapter.interruptions.onInterrupted(
                AccessInterruption.PermissionLost("PERMISSION_DENIED"),
                beto,
            )
            delay(200)
            assertNotNull(adapter.current.value)

            // Sesión terminada: solo ese mesero vuelve a la entrada.
            adapter.interruptions.onInterrupted(
                AccessInterruption.SessionEnded("SESSION_REVOKED"),
                beto,
            )
            adapter.awaitCurrent { it == null }
            assertNull(access.activeWaiter.value)
            assertNull(access.sessionToken(beto))
            assertNotNull(access.sessionToken(ana))
        }

    @Test
    fun dispositivoRetiradoDesdeElCanalRetiraElDispositivo() =
        runBlocking<Unit> {
            val adapter = DeviceServerAccess(access, scope)
            pair()
            access.enter(ana, "4829")
            adapter.awaitCurrent { it != null }
            adapter.interruptions.onInterrupted(AccessInterruption.DeviceRevoked, ana)
            withTimeout(5_000) {
                while (access.status.value !is DeviceStatus.Retired) {
                    delay(10)
                }
            }
            adapter.awaitCurrent { it == null }
        }
}
