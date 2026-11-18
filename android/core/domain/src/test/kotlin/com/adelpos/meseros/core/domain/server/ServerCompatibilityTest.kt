package com.adelpos.meseros.core.domain.server

import com.adelpos.meseros.core.domain.server.ServerCompatibility.Verdict
import org.junit.Assert.assertEquals
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Test

/** «Actualiza esta aplicación» y «Actualiza el servidor del restaurante» (§4, «Versiones»). */
class ServerCompatibilityTest {
    private val withPin = listOf("offline-orders-v1", "waiter-pin-v1", "takeaway-v1")

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
            ServerCompatibility.check("0.2.0", "0.13.0", withPin, "0.1.0"),
        )
        assertEquals(Verdict.Compatible, ServerCompatibility.check("0.2.0", "1.0.0", withPin, null))
    }

    @Test
    fun servidorViejoOSinPin() {
        assertEquals(
            Verdict.UpdateServer("0.11.0"),
            ServerCompatibility.check("0.2.0", "0.11.0", withPin, "0.1.0"),
        )
        assertEquals(
            Verdict.UpdateServer("0.13.0"),
            ServerCompatibility.check("0.2.0", "0.13.0", listOf("offline-orders-v1"), "0.1.0"),
        )
        assertEquals(
            Verdict.UpdateServer("0.13.0-beta"),
            ServerCompatibility.check("0.2.0", "0.13.0-beta", withPin, "0.1.0"),
        )
    }

    /** La A2 exige el servidor 0.13.0 con Para llevar (`takeaway-v1`; A2-D6). */
    @Test
    fun servidorSinParaLlevar() {
        // El servidor de la A1 (0.12.0 con el PIN) ya no basta.
        assertEquals(
            Verdict.UpdateServer("0.12.0"),
            ServerCompatibility.check(
                "0.2.0",
                "0.12.0",
                listOf("offline-orders-v1", "waiter-pin-v1"),
                "0.1.0",
            ),
        )
        // Un 0.13.0 sin la capacidad (no debería existir) tampoco.
        assertEquals(
            Verdict.UpdateServer("0.13.0"),
            ServerCompatibility.check(
                "0.2.0",
                "0.13.0",
                listOf("offline-orders-v1", "waiter-pin-v1"),
                "0.1.0",
            ),
        )
        assertEquals(
            Verdict.UpdateServer("0.12.0"),
            ServerCompatibility.check("0.2.0", "0.12.0", withPin, "0.1.0"),
        )
        assertTrue(
            ServerCompatibility.check("0.1.0", "raro", withPin, null) is Verdict.UpdateServer,
        )
    }

    @Test
    fun aplicacionVieja() {
        assertEquals(
            Verdict.UpdateApp("0.1.0", "0.13.0"),
            ServerCompatibility.check("0.0.9", "0.13.0", withPin, "0.1.0"),
        )
        assertEquals(
            Verdict.UpdateApp("0.2.0", "0.13.0"),
            ServerCompatibility.check("0.1.0", "0.13.0", withPin, "0.2.0"),
        )
    }
}
