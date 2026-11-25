package com.adelpos.cocina.settings

import com.adelpos.cocina.board.DisplaySettingsService
import com.adelpos.cocina.board.SettingsAccessDto
import com.adelpos.core.model.connection.LinkState
import com.adelpos.core.model.error.AppError
import com.adelpos.core.network.api.ApiOutcome
import com.adelpos.core.network.api.ApiProblem
import com.adelpos.core.network.api.IgnoredBody
import kotlinx.coroutines.ExperimentalCoroutinesApi
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.test.TestScope
import kotlinx.coroutines.test.UnconfinedTestDispatcher
import kotlinx.coroutines.test.advanceTimeBy
import kotlinx.coroutines.test.runCurrent
import kotlinx.coroutines.test.runTest
import kotlinx.serialization.json.JsonObject
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test

/**
 * Los ajustes de la tableta (§5.4, K1-D13 y K1-D32): la contraseña de un administrador la valida
 * Go, **vive solo en memoria** con los ajustes abiertos (se borra al cerrarlos, a los dos minutos
 * sin uso y al volver a emparejar), los errores de Go en palabras claras, sin conexión no se
 * intenta y «Volver a emparejar» con Go que responde, que no responde o que ya no la reconoce.
 */
@OptIn(ExperimentalCoroutinesApi::class)
class KitchenSettingsTest {
    private class FakeService : DisplaySettingsService {
        val calls = mutableListOf<String>()
        var accessAnswer: ApiOutcome<SettingsAccessDto> =
            ApiOutcome.Success(SettingsAccessDto(authorized = true))
        var unpairAnswer: ApiOutcome<IgnoredBody> = ApiOutcome.Success(JsonObject(emptyMap()))

        override suspend fun settingsAccess(
            username: String,
            password: String,
        ): ApiOutcome<SettingsAccessDto> {
            calls += "settings-access $username/$password"
            return accessAnswer
        }

        override suspend fun unpair(username: String, password: String): ApiOutcome<IgnoredBody> {
            calls += "unpair $username/$password"
            return unpairAnswer
        }
    }

    private val service = FakeService()
    private val link = MutableStateFlow(LinkState.ONLINE)
    private var forgotten = 0
    private val reported = mutableListOf<AppError>()

    private fun TestScope.settings(): KitchenSettings =
        KitchenSettings(
            service = service,
            link = link,
            forget = { forgotten++ },
            report = { reported += it },
            scope = TestScope(UnconfinedTestDispatcher(testScheduler)),
            clock = { testScheduler.currentTime },
        )

    @Test
    fun laContrasenaSoloViveEnMemoriaConLosAjustesAbiertos() = runTest {
        val settings = settings()
        settings.open()
        assertEquals(SettingsScreen.Password(), settings.screen.value)
        settings.submit("admin", "secreta")
        runCurrent()
        assertEquals(SettingsScreen.Open(), settings.screen.value)
        assertTrue(settings.holdsPassword)
        settings.close()
        assertEquals(SettingsScreen.Closed, settings.screen.value)
        assertFalse("al cerrar se borra", settings.holdsPassword)
    }

    @Test
    fun aLosDosMinutosSinUsoSeCierranYSeBorraLaContrasena() = runTest {
        val settings = settings()
        settings.open()
        settings.submit("admin", "secreta")
        runCurrent()
        advanceTimeBy(90_000)
        settings.touch()
        // Se tocó a los 90 s: cuenta otra vez desde ahí.
        advanceTimeBy(100_000)
        runCurrent()
        assertTrue(settings.screen.value is SettingsScreen.Open)
        assertTrue(settings.holdsPassword)
        advanceTimeBy(20_001)
        runCurrent()
        assertEquals(SettingsScreen.Closed, settings.screen.value)
        assertFalse(settings.holdsPassword)
    }

    @Test
    fun losErroresDeGoEnPalabrasClaras() = runTest {
        val settings = settings()
        val cases =
            mapOf<ApiOutcome<SettingsAccessDto>, String>(
                rejected<SettingsAccessDto>(401, "AUTHORIZATION_INVALID") to SettingsTexts.WRONG,
                rejected<SettingsAccessDto>(403, "PERMISSION_DENIED") to SettingsTexts.NOT_ALLOWED,
                rejected<SettingsAccessDto>(429, "AUTHORIZATION_THROTTLED") to
                    SettingsTexts.THROTTLED,
                rejected<SettingsAccessDto>(400, "VALIDATION_ERROR") to SettingsTexts.MISSING,
                ApiOutcome.NoAnswer(AppError.Unreachable, requestSent = true) to
                    SettingsTexts.OFFLINE,
            )
        for ((answer, text) in cases) {
            settings.close()
            settings.open()
            service.accessAnswer = answer
            settings.submit("cocina", "otra")
            runCurrent()
            assertEquals(SettingsScreen.Password(error = text), settings.screen.value)
            assertFalse(settings.holdsPassword)
        }
    }

    @Test
    fun sinConexionOVacioNiSeIntenta() = runTest {
        val settings = settings()
        settings.open()
        settings.submit(" ", "")
        assertEquals(SettingsScreen.Password(error = SettingsTexts.MISSING), settings.screen.value)
        link.value = LinkState.OFFLINE
        settings.submit("admin", "secreta")
        assertEquals(SettingsScreen.Password(error = SettingsTexts.OFFLINE), settings.screen.value)
        assertEquals(emptyList<String>(), service.calls)
    }

    @Test
    fun volverAEmparejarUsaLaContrasenaDeLaEntradaYBorraTodo() = runTest {
        val settings = settings()
        settings.open()
        settings.submit("admin", "secreta")
        runCurrent()
        // Sin confirmar no pasa nada.
        settings.confirmUnpair()
        assertEquals(1, service.calls.size)
        settings.askUnpair()
        assertEquals(SettingsScreen.Open(confirmUnpair = true), settings.screen.value)
        settings.confirmUnpair()
        runCurrent()
        assertEquals("unpair admin/secreta", service.calls.last())
        assertEquals(1, forgotten)
        assertEquals(SettingsScreen.Closed, settings.screen.value)
        assertFalse(settings.holdsPassword)
    }

    @Test
    fun siGoNoRespondeOYaNoLaReconoceSeBorraIgualYSeDiceQueHayQueRevocarla() = runTest {
        val settings = settings()
        val cases =
            mapOf<ApiOutcome<IgnoredBody>, String>(
                ApiOutcome.NoAnswer(AppError.Unreachable, requestSent = true) to
                    SettingsTexts.UNPAIRED_NO_ANSWER,
                rejected<IgnoredBody>(401, "DEVICE_REVOKED") to
                    SettingsTexts.UNPAIRED_NOT_RECOGNIZED,
            )
        for ((answer, text) in cases) {
            settings.close()
            settings.open()
            settings.submit("admin", "secreta")
            runCurrent()
            service.unpairAnswer = answer
            settings.askUnpair()
            settings.confirmUnpair()
            runCurrent()
            assertEquals(SettingsScreen.Unpaired(text), settings.screen.value)
            assertFalse(settings.holdsPassword)
        }
        assertEquals(2, forgotten)
    }

    @Test
    fun unaContrasenaQueYaNoSirveNoBorraNada() = runTest {
        val settings = settings()
        settings.open()
        settings.submit("admin", "secreta")
        runCurrent()
        service.unpairAnswer = rejected(401, "AUTHORIZATION_INVALID")
        settings.askUnpair()
        settings.confirmUnpair()
        runCurrent()
        assertEquals(SettingsScreen.Open(error = SettingsTexts.WRONG), settings.screen.value)
        assertEquals(0, forgotten)
    }

    @Test
    fun siLaPantallaDejaDeEstarListaSeCierran() = runTest {
        val settings = settings()
        settings.open()
        settings.submit("admin", "secreta")
        runCurrent()
        settings.onNotReady()
        assertEquals(SettingsScreen.Closed, settings.screen.value)
        assertFalse(settings.holdsPassword)
    }

    @Test
    fun unaPantallaRevocadaVaALaEntrada() = runTest {
        val settings = settings()
        settings.open()
        service.accessAnswer = rejected(401, "DEVICE_REVOKED")
        settings.submit("admin", "secreta")
        runCurrent()
        assertEquals(SettingsScreen.Closed, settings.screen.value)
        assertTrue(reported.single() is AppError.DeviceRevoked)
    }

    private fun <T> rejected(status: Int, code: String): ApiOutcome<T> =
        ApiOutcome.Rejected(ApiProblem(status, code, "mensaje de Go", "req", null))
}
