package com.adelpos.meseros.core.domain.server

import com.adelpos.meseros.core.domain.text.Hex
import java.util.concurrent.atomic.AtomicInteger
import kotlinx.coroutines.async
import kotlinx.coroutines.awaitAll
import kotlinx.coroutines.runBlocking
import kotlinx.coroutines.test.runTest
import org.junit.Assert.assertEquals
import org.junit.Assert.assertNull
import org.junit.Test

/**
 * Lectura del TXT del anuncio `_adelpos._tcp` (como lo escribe `backend/internal/discovery`) y
 * elección del servidor emparejado por `server_id` y huella, como el escritorio
 * (`only_the_paired_server_with_its_fingerprint_is_tried` de `discovery.rs`).
 */
class ServerAnnouncementsTest {
    private val serverId = "11111111-2222-4333-8444-555555555555"
    private val fingerprint = ByteArray(32) { (it * 7).toByte() }
    private val prefix = Hex.encode(fingerprint.copyOfRange(0, 8))

    private fun txt(vararg entries: Pair<String, String?>): Map<String, ByteArray?> =
        entries.associate { (key, value) ->
            key to value?.toByteArray()
        }

    private fun goTxt(id: String = serverId, fp: String = prefix) =
        txt("txtvers" to "1", "server_id" to id, "api" to "v1", "port" to "8443", "fp" to fp)

    @Test
    fun leeElAnuncioDeGo() {
        val found =
            ServerAnnouncements.parse(
                "Fonda Doña Ñeli — Mac de caja",
                goTxt(),
                8443,
                listOf("192.168.1.20", "fd00::5", "8.8.8.8", "fe80::1%en0"),
            )
        assertEquals(
            FoundServer(
                serverId,
                "Fonda Doña Ñeli — Mac de caja",
                prefix,
                8443,
                listOf("192.168.1.20", "[fd00::5]"),
            ),
            found,
        )
        assertEquals(
            listOf("https://192.168.1.20:8443", "https://[fd00::5]:8443"),
            found!!.bases(),
        )
    }

    @Test
    fun lasClavesNoDistinguenMayusculasYElNombreSeLimpia() {
        val found =
            ServerAnnouncements.parse(
                "  Caja\u0007 2  ",
                txt("SERVER_ID" to serverId, "FP" to prefix),
                9443,
                listOf("10.0.0.9"),
            )
        assertEquals("Caja 2", found?.name)
        assertEquals(9443, found?.port)
    }

    @Test
    fun elNombreSinEscapesDeDnsSdYLaIpv6Comprimida() {
        assertEquals(
            "Fonda Doña Ñeli — Mac de caja",
            ServerAnnouncements.unescapeInstance(
                "Fonda\\032Doña\\032Ñeli\\032—\\032Mac\\032de caja",
            ),
        )
        assertEquals("S.A. 😀", ServerAnnouncements.unescapeInstance("S\\.A\\.\\032😀"))
        assertEquals("Caja \\", ServerAnnouncements.unescapeInstance("Caja \\\\"))
        val found = ServerAnnouncements.parse("x", goTxt(), 8443, listOf("fd00:0:0:0:0:0:0:5"))
        assertEquals(listOf("[fd00::5]"), found?.hosts)
    }

    @Test
    fun descartaAnunciosIncompletosOAjenos() {
        assertNull(
            ServerAnnouncements.parse(
                "x",
                goTxt(id = "ABCDEF01-2222-4333-8444-555555555555"),
                8443,
                listOf("10.0.0.9"),
            ),
        )
        assertNull(ServerAnnouncements.parse("x", goTxt(id = "otro"), 8443, listOf("10.0.0.9")))
        assertNull(ServerAnnouncements.parse("x", goTxt(fp = "abc"), 8443, listOf("10.0.0.9")))
        assertNull(
            ServerAnnouncements.parse(
                "x",
                goTxt(fp = prefix.uppercase()),
                8443,
                listOf("10.0.0.9"),
            ),
        )
        assertNull(
            ServerAnnouncements.parse("x", txt("server_id" to serverId), 8443, listOf("10.0.0.9")),
        )
        assertNull(
            ServerAnnouncements.parse(
                "x",
                txt("server_id" to serverId, "fp" to null),
                8443,
                listOf("10.0.0.9"),
            ),
        )
        assertNull(ServerAnnouncements.parse("x", goTxt(), 0, listOf("10.0.0.9")))
        // Solo direcciones públicas o de loopback: no es un servidor de la red del restaurante.
        assertNull(ServerAnnouncements.parse("x", goTxt(), 8443, listOf("8.8.8.8", "127.0.0.1")))
    }

    @Test
    fun juntaElMismoServidorPorSuIdentificador() {
        val first = FoundServer(serverId, "Caja", prefix, 8443, listOf("192.168.1.20"))
        val second =
            FoundServer(serverId, "Caja (2)", prefix, 8443, listOf("10.0.0.9", "192.168.1.20"))
        val other =
            FoundServer(
                "22222222-2222-4333-8444-555555555555",
                "Otra",
                prefix,
                8443,
                listOf("10.0.0.7"),
            )
        val merged = ServerAnnouncements.merge(listOf(first, other, second))
        assertEquals(2, merged.size)
        assertEquals(listOf("192.168.1.20", "10.0.0.9"), merged[0].hosts)
        assertEquals("Caja", merged[0].name)
    }

    @Test
    fun soloSePruebaElServidorEmparejadoConSuHuella() {
        val paired =
            FoundServer(serverId, "Caja", prefix, 8443, listOf("192.168.1.30", "192.168.1.20"))
        val impostor =
            FoundServer(serverId, "Caja", "0000000000000000", 8443, listOf("192.168.1.66"))
        val stranger =
            FoundServer(
                "22222222-2222-4333-8444-555555555555",
                "Otra",
                prefix,
                8443,
                listOf("192.168.1.77"),
            )
        val candidates =
            ServerAnnouncements.candidates(
                listOf(impostor, stranger, paired),
                serverId,
                fingerprint,
                failedBase = "https://192.168.1.20:8443",
            )
        assertEquals(listOf("https://192.168.1.30:8443"), candidates)
    }

    private class FakeNetwork(var found: List<FoundServer>, val answering: Set<String>) :
        ServerBrowser, ServerProbe {
        val searches = AtomicInteger()
        val probes = mutableListOf<String>()

        override suspend fun search(): List<FoundServer> {
            searches.incrementAndGet()
            return found
        }

        override suspend fun answers(base: String, fingerprint: ByteArray): Boolean {
            synchronized(probes) { probes.add(base) }
            return base in answering
        }
    }

    @Test
    fun elCambioDeIpSoloAceptaLaHuellaCompletaYBuscaComoMuchoCadaDiezSegundos() = runTest {
        var now = 0L
        val moved =
            FoundServer(serverId, "Caja", prefix, 8443, listOf("192.168.1.66", "192.168.1.30"))
        // 192.168.1.66 tiene la misma huella corta, pero no responde con la llave completa.
        val network = FakeNetwork(listOf(moved), answering = setOf("https://192.168.1.30:8443"))
        val relocator = ServerRelocator(network, network, clock = { now })
        var current = "https://192.168.1.20:8443"
        val found = relocator.relocate(serverId, fingerprint, current) { current }
        assertEquals("https://192.168.1.30:8443", found)
        assertEquals(
            listOf("https://192.168.1.66:8443", "https://192.168.1.30:8443"),
            network.probes,
        )

        // Otra petición falló con la dirección vieja, pero ya se cambió: no busca otra vez.
        current = found!!
        assertEquals(
            current,
            relocator.relocate(serverId, fingerprint, "https://192.168.1.20:8443") { current },
        )
        assertEquals(1, network.searches.get())

        // La nueva falla también: antes de 10 s no vuelve a buscar.
        now = 9_999
        assertNull(relocator.relocate(serverId, fingerprint, current) { current })
        assertEquals(1, network.searches.get())
        now = 10_000
        network.found = emptyList()
        assertNull(relocator.relocate(serverId, fingerprint, current) { current })
        assertEquals(2, network.searches.get())
    }

    @Test
    fun variasFallasJuntasHacenUnaSolaBusqueda() = runBlocking {
        val moved = FoundServer(serverId, "Caja", prefix, 8443, listOf("192.168.1.30"))
        val network = FakeNetwork(listOf(moved), answering = setOf("https://192.168.1.30:8443"))
        val relocator = ServerRelocator(network, network, clock = { 0L })
        var current = "https://192.168.1.20:8443"
        val failed = current
        val results =
            (1..5)
                .map {
                    async {
                        val result = relocator.relocate(serverId, fingerprint, failed) { current }
                        // Como PairedConnection: la dirección nueva queda como la vigente.
                        if (result != null) {
                            current = result
                        }
                        result
                    }
                }
                .awaitAll()
        assertEquals(List(5) { "https://192.168.1.30:8443" }, results)
        assertEquals(1, network.searches.get())
    }
}
