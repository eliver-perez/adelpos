package com.adelpos.meseros.core.data.api

import com.adelpos.meseros.core.domain.error.AppError
import com.adelpos.meseros.core.domain.error.LicenseMessages
import com.adelpos.meseros.core.domain.error.LicenseRefusal
import com.adelpos.meseros.core.domain.error.LicenseSeat
import com.adelpos.meseros.core.domain.error.RefusedLicense
import org.junit.Assert.assertEquals
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Test

/**
 * Etapa L1: los cinco códigos de la puerta de Go (L1-D17, siempre 409) con las formas del contrato
 * v1 (`LicenseRejection`, `LicenseSeatUnavailable` y `SystemClockInvalid`) se vuelven
 * [AppError.LicenseRefused], y el texto sale del código, nunca del `message`.
 */
class LicenseErrorCodesTest {
    private fun problem(status: Int, code: String, details: String? = null): ApiProblem {
        val extra = details?.let { ""","details":$it""" }.orEmpty()
        return ApiErrors.problemOf(
            status,
            """{"error":{"code":"$code","message":"Mensaje de Go.","request_id":"r"$extra}}""",
        )
    }

    @Test
    fun sinCupoConSusDetails() {
        val seat =
            problem(
                409,
                "LICENSE_SEAT_UNAVAILABLE",
                """{"seat":"order_takers","in_use":1,"limit":1}""",
            )
        val error = ApiErrors.toAppError(seat)
        assertEquals(
            AppError.LicenseRefused(
                refusal = LicenseRefusal.NoSeat(LicenseSeat.ORDER_TAKERS, 1, 1),
                code = "LICENSE_SEAT_UNAVAILABLE",
                serverMessage = "Mensaje de Go.",
                requestId = "r",
            ),
            error,
        )
        assertEquals(
            "No hay cupo de comandero libre (1 de 1 en uso). Pide que se cierre la sesión de otra " +
                "terminal.",
            ApiErrors.shownMessage(seat),
        )
        // El plan básico: 0 comanderos.
        val basic =
            problem(
                409,
                "LICENSE_SEAT_UNAVAILABLE",
                """{"seat":"order_takers","in_use":0,"limit":0}""",
            )
        assertTrue(ApiErrors.shownMessage(basic)!!.startsWith("La licencia no incluye comanderos"))
    }

    @Test
    fun licenciaYHoraConLaFormaDelContrato() {
        val expired =
            problem(
                409,
                "LICENSE_EXPIRED",
                """{"operation":"capture","expires_at":"2026-11-01T00:00:00Z",""" +
                    """"grace_ended_at":"2026-11-06T00:00:00Z"}""",
            )
        assertEquals(
            LicenseRefusal.LicenseState(RefusedLicense.EXPIRED, "capture"),
            ApiErrors.licenseRefusal(expired),
        )
        assertEquals(LicenseMessages.CAPTURE_NOT_ALLOWED, ApiErrors.shownMessage(expired))
        val notActivated =
            problem(
                409,
                "LICENSE_NOT_ACTIVATED",
                """{"operation":"administration","state":"not_activated_with_data",""" +
                    """"copied":true,"machine_unverified":false}""",
            )
        assertEquals(
            LicenseRefusal.LicenseState(RefusedLicense.NOT_ACTIVATED, "administration"),
            ApiErrors.licenseRefusal(notActivated),
        )
        val revoked =
            problem(409, "LICENSE_REVOKED", """{"operation":"capture","reason":"revoked"}""")
        assertEquals(LicenseMessages.CAPTURE_NOT_ALLOWED, ApiErrors.shownMessage(revoked))
        val clock =
            problem(
                409,
                "SYSTEM_CLOCK_INVALID",
                """{"blocked":true,"local_time":"2026-11-23T16:15:00Z",""" +
                    """"last_activity_time":"2026-11-23T18:40:00Z","margin_minutes":10,""" +
                    """"timezone":"America/Monterrey","local_time_text":"lunes 23 nov 2026, 10:15",""" +
                    """"last_activity_time_text":"lunes 23 nov 2026, 12:40",""" +
                    """"license_state":"active","license_ok":true}""",
            )
        assertEquals(LicenseRefusal.ClockInvalid, ApiErrors.licenseRefusal(clock))
        assertEquals(LicenseMessages.CLOCK_INVALID, ApiErrors.shownMessage(clock))
        // La protección de respaldo de los cupos (sin details) también es de la licencia.
        assertEquals(
            LicenseMessages.CAPTURE_NOT_ALLOWED,
            ApiErrors.shownMessage(problem(409, "LICENSE_NOT_ACTIVATED")),
        )
    }

    @Test
    fun soloCon409YLosCodigosDeLaPuerta() {
        // Los de la gestión de la licencia (escritorio) y los demás rechazos siguen igual.
        val other = problem(409, "LICENSE_ACTIVATION_LIMIT")
        assertNull(ApiErrors.licenseRefusal(other))
        assertEquals(
            AppError.Rejected(409, "LICENSE_ACTIVATION_LIMIT", "Mensaje de Go.", "r"),
            ApiErrors.toAppError(other),
        )
        assertEquals("Mensaje de Go.", ApiErrors.shownMessage(problem(409, "ACCOUNT_CONFLICT")))
        // Un código de la puerta con otro estado no tiene la forma del contrato: no se interpreta.
        assertNull(ApiErrors.licenseRefusal(problem(400, "SYSTEM_CLOCK_INVALID")))
        // Details con otra forma: el texto se arma igual, sin números.
        val odd = problem(409, "LICENSE_SEAT_UNAVAILABLE", """{"seat":3,"in_use":"x"}""")
        assertEquals(
            LicenseRefusal.NoSeat(LicenseSeat.UNKNOWN, null, null),
            ApiErrors.licenseRefusal(odd),
        )
        assertEquals(LicenseMessages.NO_SEAT, ApiErrors.shownMessage(odd))
    }
}
