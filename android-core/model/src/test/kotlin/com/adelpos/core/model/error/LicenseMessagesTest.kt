package com.adelpos.core.model.error

import com.adelpos.core.model.license.LicenseClock
import com.adelpos.core.model.license.LicenseNotices
import com.adelpos.core.model.license.LicenseStateName
import com.adelpos.core.model.license.LicenseSummary
import org.junit.Assert.assertEquals
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Test

/**
 * Etapa L1 (§8): los textos de la licencia salen del código de Go y de sus `details`, nunca del
 * `message`, y en un solo lugar ([LicenseMessages]).
 */
class LicenseMessagesTest {
    @Test
    fun losCincoCodigosDeLaPuertaYNingunOtro() {
        assertEquals(
            setOf(
                "LICENSE_NOT_ACTIVATED",
                "LICENSE_EXPIRED",
                "LICENSE_REVOKED",
                "LICENSE_SEAT_UNAVAILABLE",
                "SYSTEM_CLOCK_INVALID",
            ),
            LicenseRefusal.CODES,
        )
        assertNull(LicenseRefusal.of("ACCOUNT_CONFLICT"))
        assertNull(LicenseRefusal.of(null))
        // Los de la gestión de la licencia (escritorio) no son de la puerta.
        assertNull(LicenseRefusal.of("LICENSE_KEY_NOT_FOUND"))
    }

    @Test
    fun licenciaSinActivarVencidaORevocada() {
        for (code in listOf("LICENSE_NOT_ACTIVATED", "LICENSE_EXPIRED", "LICENSE_REVOKED")) {
            val capture = LicenseRefusal.of(code, operation = "capture")!!
            assertEquals(
                "La licencia del sistema no permite capturar pedidos. Avisa al encargado.",
                LicenseMessages.of(capture),
            )
            // Sin la fila, se supone la captura (es lo que hace el dispositivo).
            assertEquals(
                LicenseMessages.CAPTURE_NOT_ALLOWED,
                LicenseMessages.of(LicenseRefusal.of(code)!!),
            )
            val other = LicenseRefusal.of(code, operation = "captured")!!
            assertEquals(
                "La licencia del sistema no permite esta operación. Avisa al encargado.",
                LicenseMessages.of(other),
            )
        }
        assertEquals(
            LicenseRefusal.LicenseState(RefusedLicense.REVOKED, "capture"),
            LicenseRefusal.of("LICENSE_REVOKED", operation = "capture"),
        )
    }

    @Test
    fun alEmparejarSinActivarPideActivarAntes() {
        val notActivated =
            LicenseRefusal.of("LICENSE_NOT_ACTIVATED", operation = "administration")!!
        assertEquals(
            "La computadora principal no tiene la licencia activada. Pide al encargado que la " +
                "active y vuelve a intentarlo.",
            LicenseMessages.forPairing(notActivated),
        )
        assertEquals(
            LicenseMessages.CLOCK_INVALID,
            LicenseMessages.forPairing(LicenseRefusal.ClockInvalid),
        )
    }

    @Test
    fun sinCupoConCuantosEnUsoYElLimite() {
        assertEquals(
            "No hay cupo de comandero libre (3 de 3 en uso). Pide que se cierre la sesión de otra " +
                "terminal.",
            LicenseMessages.of(
                LicenseRefusal.of("LICENSE_SEAT_UNAVAILABLE", null, "order_takers", 3, 3)!!,
            ),
        )
        assertEquals(
            "La licencia no incluye comanderos (0 de 0): solo se capturan pedidos en una terminal " +
                "con el turno de caja abierto.",
            LicenseMessages.of(
                LicenseRefusal.of("LICENSE_SEAT_UNAVAILABLE", null, "order_takers", 0, 0)!!,
            ),
        )
        // El cupo lo dice Go: nunca se supone el de comandero.
        assertEquals(
            "No hay cupo de caja libre (1 de 1 en uso). Se libera al cerrar un turno de caja.",
            LicenseMessages.of(
                LicenseRefusal.of("LICENSE_SEAT_UNAVAILABLE", null, "cash_registers", 1, 1)!!,
            ),
        )
        assertEquals(
            "La licencia no incluye pantallas de cocina (0 de 0): Preparación no está disponible.",
            LicenseMessages.of(
                LicenseRefusal.of("LICENSE_SEAT_UNAVAILABLE", null, "kitchen_displays", 0, 0)!!,
            ),
        )
    }

    @Test
    fun sinCupoConDetailsIncompletos() {
        assertEquals(
            "No hay cupo de comandero libre. Pide que se cierre la sesión de otra terminal.",
            LicenseMessages.of(
                LicenseRefusal.of("LICENSE_SEAT_UNAVAILABLE", seat = "order_takers")!!,
            ),
        )
        // Un número negativo no tiene sentido: como no dicho.
        val negative = LicenseRefusal.of("LICENSE_SEAT_UNAVAILABLE", null, "order_takers", -1, 3)
        assertEquals(LicenseRefusal.NoSeat(LicenseSeat.ORDER_TAKERS, null, 3), negative)
        assertEquals(
            LicenseMessages.NO_SEAT,
            LicenseMessages.of(LicenseRefusal.of("LICENSE_SEAT_UNAVAILABLE", null, "otro", 1, 1)!!),
        )
        assertEquals(
            LicenseMessages.NO_SEAT,
            LicenseMessages.of(LicenseRefusal.of("LICENSE_SEAT_UNAVAILABLE")!!),
        )
    }

    @Test
    fun horaIncorrecta() {
        assertEquals(
            "No se puede registrar: la computadora principal tiene la fecha y hora incorrectas. " +
                "Avisa al encargado.",
            LicenseMessages.of(LicenseRefusal.of("SYSTEM_CLOCK_INVALID")!!),
        )
        // Nunca se confunde con la licencia.
        assertTrue("licencia" !in LicenseMessages.CLOCK_INVALID)
    }

    @Test
    fun elTextoDeUnErrorDeLaLicenciaNoEsElDeGo() {
        val error =
            AppError.LicenseRefused(
                refusal = LicenseRefusal.ClockInvalid,
                code = "SYSTEM_CLOCK_INVALID",
                serverMessage = "Otro texto",
                requestId = "r",
            )
        assertEquals(LicenseMessages.CLOCK_INVALID, error.shownMessage())
        val other = AppError.Rejected(409, "ACCOUNT_CONFLICT", "Mensaje de Go", "r")
        assertEquals("Mensaje de Go", other.shownMessage())
    }

    @Test
    fun avisoFijoDeLaHoraConSusFechas() {
        val blocked =
            summary(
                LicenseStateName.ACTIVE,
                clock = LicenseClock(true, "lunes 23 nov 2026, 10:15", "lunes 23 nov 2026, 12:40"),
            )
        val notice = LicenseNotices.clock(blocked)!!
        assertEquals(LicenseMessages.CLOCK_INVALID, notice.text)
        assertEquals(
            "La computadora principal tiene la hora lunes 23 nov 2026, 10:15 y la última actividad " +
                "registrada fue el lunes 23 nov 2026, 12:40.",
            notice.detail,
        )
        // Sin las fechas, solo la causa.
        val withoutDates = summary(LicenseStateName.ACTIVE, clock = LicenseClock(true, null, null))
        assertNull(LicenseNotices.clock(withoutDates)!!.detail)
        assertNull(LicenseNotices.clock(summary(LicenseStateName.ACTIVE)))
        assertNull(LicenseNotices.clock(null))
    }

    @Test
    fun avisoDiscretoSoloEnTolerancia() {
        val grace = LicenseNotices.grace(summary(LicenseStateName.GRACE, graceDays = 3))!!
        assertEquals("Licencia: quedan 3 días", grace.short)
        assertEquals(
            "La licencia del sistema venció. Quedan 3 días para renovarla; después ya no se podrán " +
                "capturar pedidos. Avisa al encargado.",
            grace.long,
        )
        assertEquals("Licencia: queda 1 día", LicenseMessages.graceShort(1))
        assertTrue(LicenseMessages.graceLong(1).contains("Queda 1 día para renovarla"))
        assertEquals("Licencia: último día", LicenseMessages.graceShort(0))
        for (state in LicenseStateName.entries - LicenseStateName.GRACE) {
            assertNull(state.name, LicenseNotices.grace(summary(state, graceDays = 3)))
        }
        // Sin los días que calcula Go, no se inventan.
        assertNull(LicenseNotices.grace(summary(LicenseStateName.GRACE, graceDays = null)))
        assertEquals(LicenseStateName.GRACE, LicenseStateName.fromWireName("grace"))
        assertEquals(LicenseStateName.UNKNOWN, LicenseStateName.fromWireName("otro"))
    }

    private fun summary(
        state: LicenseStateName,
        graceDays: Int? = null,
        clock: LicenseClock = LicenseClock.NOT_BLOCKED,
    ) = LicenseSummary(state = state, graceDaysRemaining = graceDays, clock = clock)

    @Test
    fun losTextosDeLaPantallaDeCocina() {
        // §6 de la etapa K1, palabra por palabra.
        assertEquals(
            "No hay cupo de pantalla de cocina libre (2 de 2 en uso). Revoca otra pantalla en " +
                "Terminales o amplía tu licencia.",
            LicenseMessages.kitchenDisplayNoSeat(inUse = 2, limit = 2),
        )
        assertEquals(
            "No hay cupo de pantalla de cocina libre. Revoca otra pantalla en Terminales o amplía " +
                "tu licencia.",
            LicenseMessages.kitchenDisplayNoSeat(inUse = null, limit = 2),
        )
        assertEquals(
            "Esta pantalla dejó su cupo de cocina.",
            LicenseMessages.KITCHEN_DISPLAY_SEAT_RELEASED,
        )
    }
}
