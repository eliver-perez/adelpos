package com.adelpos.meseros.core.domain.server

import com.adelpos.core.model.server.SemanticVersion
import com.adelpos.core.model.server.ServerCompatibility
import com.adelpos.core.model.server.ServerCompatibility.Verdict
import org.junit.Assert.assertEquals
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Test

/** «Actualiza esta aplicación» y «Actualiza el servidor del restaurante» (§4, «Versiones»). */
class ServerCompatibilityTest {
    // La comprobación compartida (android-core, etapa K1) con los requisitos de meseros.
    private fun check(
        appVersion: String,
        serverVersion: String,
        capabilities: List<String>,
        minClientVersion: String?,
    ) =
        ServerCompatibility.check(
            appVersion,
            serverVersion,
            capabilities,
            minClientVersion,
            WaiterServerRequirements.REQUIREMENTS,
        )

    private val withPin =
        listOf(
            "offline-orders-v1",
            "waiter-pin-v1",
            "takeaway-v1",
            "device-precheck-printer-v1",
            "ready-alerts-v1",
        )

    @Test
    fun ordenDeLasVersiones() {
        val ordered =
            listOf("0.0.9", "0.1.0-beta", "0.1.0", "0.1.1", "0.9.0", "0.10.0", "0.12.0", "1.0.0")
                .map { SemanticVersion.parse(it)!! }
        assertEquals(ordered, ordered.shuffled().sorted())
        assertEquals("0.12.0", SemanticVersion.parse("0.12.0").toString())
        for (invalid in listOf("", "1", "1.2", "01.2.3", "1.2.3.4", "v1.2.3", "1.2.x")) {
            assertNull(invalid, SemanticVersion.parse(invalid))
        }
    }

    @Test
    fun servidorCompatible() {
        assertEquals(
            Verdict.Compatible,
            check("0.2.0", "0.13.0", withPin, "0.1.0"),
        )
        assertEquals(Verdict.Compatible, check("0.2.0", "1.0.0", withPin, null))
    }

    @Test
    fun servidorViejoOSinPin() {
        assertEquals(
            Verdict.UpdateServer("0.11.0"),
            check("0.2.0", "0.11.0", withPin, "0.1.0"),
        )
        assertEquals(
            Verdict.UpdateServer("0.13.0"),
            check("0.2.0", "0.13.0", listOf("offline-orders-v1"), "0.1.0"),
        )
        assertEquals(
            Verdict.UpdateServer("0.13.0-beta"),
            check("0.2.0", "0.13.0-beta", withPin, "0.1.0"),
        )
    }

    /** La A2 exige el servidor 0.13.0 con Para llevar (`takeaway-v1`; A2-D6). */
    @Test
    fun servidorSinParaLlevar() {
        // El servidor de la A1 (0.12.0 con el PIN) ya no basta.
        assertEquals(
            Verdict.UpdateServer("0.12.0"),
            check(
                "0.2.0",
                "0.12.0",
                listOf("offline-orders-v1", "waiter-pin-v1"),
                "0.1.0",
            ),
        )
        // Un 0.13.0 sin la capacidad (no debería existir) tampoco.
        assertEquals(
            Verdict.UpdateServer("0.13.0"),
            check(
                "0.2.0",
                "0.13.0",
                listOf("offline-orders-v1", "waiter-pin-v1"),
                "0.1.0",
            ),
        )
        assertEquals(
            Verdict.UpdateServer("0.12.0"),
            check("0.2.0", "0.12.0", withPin, "0.1.0"),
        )
        assertTrue(
            check("0.1.0", "raro", withPin, null) is Verdict.UpdateServer,
        )
    }

    /**
     * La Fase C de la A2 exige además la impresora de precuentas por dispositivo
     * (`device-precheck-printer-v1`, A2-D6): sin ella, la precuenta podría salir en otra impresora.
     */
    @Test
    fun servidorSinImpresoraDePrecuentasPorDispositivo() {
        assertEquals(
            Verdict.UpdateServer("0.13.0"),
            check(
                "0.2.0",
                "0.13.0",
                listOf("offline-orders-v1", "waiter-pin-v1", "takeaway-v1", "ready-alerts-v1"),
                "0.1.0",
            ),
        )
        assertTrue(
            WaiterServerRequirements.CAPABILITY_PRECHECK_PRINTER in
                WaiterServerRequirements.REQUIRED_CAPABILITIES,
        )
    }

    /**
     * La Fase D de la A2 exige además el aviso de platillos listos (`ready-alerts-v1`, A2-D6): un
     * 0.13.0 sin el registro de «listos» no sirve.
     */
    @Test
    fun servidorSinAvisoDePlatillosListos() {
        assertEquals(
            Verdict.UpdateServer("0.13.0"),
            check(
                "0.2.0",
                "0.13.0",
                listOf(
                    "offline-orders-v1",
                    "waiter-pin-v1",
                    "takeaway-v1",
                    "device-precheck-printer-v1",
                ),
                "0.1.0",
            ),
        )
        assertTrue(
            WaiterServerRequirements.CAPABILITY_READY_ALERTS in
                WaiterServerRequirements.REQUIRED_CAPABILITIES,
        )
    }

    @Test
    fun aplicacionVieja() {
        assertEquals(
            Verdict.UpdateApp("0.1.0", "0.13.0"),
            check("0.0.9", "0.13.0", withPin, "0.1.0"),
        )
        assertEquals(
            Verdict.UpdateApp("0.2.0", "0.13.0"),
            check("0.1.0", "0.13.0", withPin, "0.2.0"),
        )
    }
}
