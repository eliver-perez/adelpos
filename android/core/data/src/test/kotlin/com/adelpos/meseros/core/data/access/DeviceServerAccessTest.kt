package com.adelpos.meseros.core.data.access

import android.content.Context
import androidx.room.Room
import androidx.test.core.app.ApplicationProvider
import androidx.test.ext.junit.runners.AndroidJUnit4
import com.adelpos.meseros.core.data.api.ClientIdentity
import com.adelpos.meseros.core.data.db.AppDatabase
import com.adelpos.meseros.core.data.db.PairedServerEntity
import com.adelpos.meseros.core.data.session.AuthorizedServer
import com.adelpos.meseros.core.data.session.DeviceServerAccess
import com.adelpos.meseros.core.domain.access.DeviceStatus
import com.adelpos.meseros.core.domain.access.EntryResult
import com.adelpos.meseros.core.domain.access.NoUnsentOrders
import com.adelpos.meseros.core.domain.realtime.AccessInterruption
import com.adelpos.meseros.core.domain.server.ServerRelocator
import com.adelpos.meseros.core.domain.text.Hex
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.SupervisorJob
import kotlinx.coroutines.cancel
import kotlinx.coroutines.delay
import kotlinx.coroutines.runBlocking
import kotlinx.coroutines.withTimeout
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

            // Sesión terminada: sin conexión autorizada hasta que ese mesero vuelva a escribir su
            // PIN; sigue activo (no pierde su trabajo) y la sesión de Ana no se toca.
            adapter.interruptions.onInterrupted(
                AccessInterruption.SessionEnded("SESSION_REVOKED"),
                beto,
            )
            adapter.awaitCurrent { it == null }
            assertEquals(beto, access.activeWaiter.value?.userId)
            assertTrue(access.activeWaiter.value!!.mustReenterPin)
            assertNull(access.sessionToken(beto))
            assertNotNull(access.sessionToken(ana))
            assertTrue(access.enter(beto, "6152") is EntryResult.Entered)
            val again = adapter.awaitCurrent { it?.userId == beto }!!
            assertEquals(
                "Bearer ${access.sessionToken(beto)}",
                again.credentials[AuthorizedServer.AUTHORIZATION_HEADER],
            )
            assertNotEquals(betoServer, again)
        }

    @Test
    fun unMeseroQueEntroSinConexionNoTieneConexionAutorizada() =
        runBlocking<Unit> {
            var reachable = true
            access =
                AccessRepository(
                    dao = database.access(),
                    sealer = sealer,
                    destroyKey = {},
                    identity = ClientIdentity("0.1.0"),
                    relocator = ServerRelocator({ emptyList() }, { _, _ -> false }, { 0L }),
                    isOnline = { reachable },
                    unsentOrders = NoUnsentOrders,
                    scope = scope,
                    verifierIterations = 1_000,
                )
            val adapter = DeviceServerAccess(access, scope)
            pair()
            access.enter(ana, "4829")
            adapter.awaitCurrent { it?.userId == ana }
            access.leave()
            adapter.awaitCurrent { it == null }
            reachable = false
            assertTrue(access.enter(ana, "4829") is EntryResult.Entered)
            reachable = true
            delay(200)
            assertNull("la sesión anterior de Ana no se reutiliza", adapter.current.value)
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
