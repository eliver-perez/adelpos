package local.adel.pos.android.core.domain.server

import local.adel.pos.android.core.domain.server.ServerAddress.Problem
import local.adel.pos.android.core.domain.server.ServerAddress.Result
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test

/**
 * La dirección escrita a mano se valida como en el escritorio
 * (`only_local_network_addresses_are_accepted` de `desktop/src-tauri/src/pairing.rs`), con las
 * mismas entradas aceptadas y rechazadas, más los casos donde Android es más estricto.
 */
class ServerAddressTest {
    @Test
    fun aceptaLasDireccionesDeLaRedLocalComoElEscritorio() {
        val cases =
            listOf(
                "192.168.1.20" to "https://192.168.1.20:8443",
                "192.168.1.20:9443" to "https://192.168.1.20:9443",
                "https://10.0.0.5" to "https://10.0.0.5:8443",
                "172.20.1.1" to "https://172.20.1.1:8443",
                "169.254.3.4" to "https://169.254.3.4:8443",
                "caja-principal.local" to "https://caja-principal.local:8443",
                "Caja.Local:8443" to "https://caja.local:8443",
                "[fd00::5]" to "https://[fd00::5]:8443",
                " 192.168.1.20 " to "https://192.168.1.20:8443",
                // Del emulador: la Mac es 10.0.2.2 (red privada).
                "10.0.2.2:8454" to "https://10.0.2.2:8454",
                "HTTPS://192.168.1.20/" to "https://192.168.1.20:8443",
                "[FD00:0:0:0::5]:9000" to "https://[fd00::5]:9000",
                "[fe80::1]" to "https://[fe80::1]:8443",
                "172.31.255.255" to "https://172.31.255.255:8443",
                "192.168.1.20:443" to "https://192.168.1.20:443",
            )
        for ((input, expected) in cases) {
            assertEquals(input, Result.Valid(expected), ServerAddress.normalize(input))
        }
    }

    @Test
    fun rechazaLoMismoQueElEscritorio() {
        val malformed =
            listOf(
                "",
                "http://192.168.1.20",
                "https://192.168.1.20/admin",
                "https://user:clave@192.168.1.20",
                "192.168.1.20?x=1",
                "caja .local",
                "file:///etc/passwd",
            )
        for (input in malformed) {
            assertEquals(input, Result.Invalid(Problem.MALFORMED), ServerAddress.normalize(input))
        }
        val notLocal =
            listOf(
                "8.8.8.8",
                "127.0.0.1",
                "localhost",
                "[::1]",
                "example.com",
                "servidor",
                "100.64.0.1",
                "[2001:db8::1]",
                ".local",
                "-caja.local",
                "172.32.0.1",
            )
        for (input in notLocal) {
            assertEquals(input, Result.Invalid(Problem.NOT_LOCAL), ServerAddress.normalize(input))
        }
    }

    @Test
    fun esMasEstrictaConFormasAmbiguas() {
        for (input in
            listOf(
                "192.168.01.20",
                "192.168.1",
                "192.168.1.256",
                "192.168.1.20:0",
                "192.168.1.20:65536",
                "192.168.1.20:",
                "fd00::5",
                "[fd00::5",
                "[fe80::1%wlan0]",
                "[fd00::1::2]",
                "https://",
                "caja.local#x",
            )) {
            assertEquals(input, Result.Invalid(Problem.MALFORMED), ServerAddress.normalize(input))
        }
    }

    @Test
    fun hostsDeLaRedLocal() {
        assertTrue(ServerAddress.isLocalHost("10.0.2.15"))
        assertTrue(ServerAddress.isLocalHost("[fd12:3456::1]"))
        assertTrue(ServerAddress.isLocalHost("fd12:3456::1"))
        assertTrue(ServerAddress.isLocalHost("mac-de-caja.local"))
        assertFalse(ServerAddress.isLocalHost("fe80::1%wlan0"))
        assertFalse(ServerAddress.isLocalHost("203.0.113.7"))
        assertFalse(ServerAddress.isLocalHost("caja.lan"))
        assertFalse(ServerAddress.isLocalHost("a".repeat(64) + ".local"))
    }

    @Test
    fun ipv6ConIpv4IncrustadaYCompresion() {
        assertEquals(
            Result.Valid("https://[fd00::c0a8:114]:8443"),
            ServerAddress.normalize("[fd00::192.168.1.20]"),
        )
        assertEquals(
            Result.Valid("https://[fd00:1:0:1::1]:8443"),
            ServerAddress.normalize("[fd00:1:0:1:0:0:0:1]"),
        )
        assertEquals(
            Result.Valid("https://[fd00:1:2:3:4:5:6:7]:8443"),
            ServerAddress.normalize("[fd00:1:2:3:4:5:6:7]"),
        )
    }
}
