package com.adelpos.meseros.core.domain.access

import com.adelpos.core.model.security.SecretUnavailableException
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNotEquals
import org.junit.Assert.assertThrows
import org.junit.Assert.assertTrue
import org.junit.Test

/** Verificador local del PIN con PBKDF2 y cifrado con un Keystore falso. */
class PinVerifierTest {
    // Pocas iteraciones en la JVM: la prueba revisa la lógica, no el costo.
    private val iterations = 1_000

    @Test
    fun aceptaSoloElPinCorrecto() {
        val verifier = PinVerifier.create("4829", pinVersion = 3, iterations = iterations)
        assertTrue(verifier.matches("4829"))
        for (wrong in listOf("4828", "48290", "482", "", "４８２９", "48a9", " 4829")) {
            assertFalse(wrong, verifier.matches(wrong))
        }
        assertEquals(3, verifier.pinVersion)
    }

    @Test
    fun cadaVerificadorLlevaSuPropiaSal() {
        val first = PinVerifier.create("615273", 1, iterations).encode()
        val second = PinVerifier.create("615273", 1, iterations).encode()
        assertFalse(first.contentEquals(second))
        // Ni el PIN ni sus dígitos en texto aparecen en lo que se guarda.
        assertFalse(String(first, Charsets.ISO_8859_1).contains("615273"))
    }

    @Test
    fun seGuardaYSeLeeCifradoParaSuMesero() {
        val sealer = FakeSecretSealer()
        val verifier = PinVerifier.create("4829", pinVersion = 2, iterations = iterations)
        val sealed = PinVerifier.seal(verifier, "ana", sealer)
        assertNotEquals(verifier.encode().toList(), sealed.toList())
        val reopened = PinVerifier.open(sealed, "ana", sealer)
        assertTrue(reopened.matches("4829"))
        assertEquals(2, reopened.pinVersion)
        assertEquals(iterations, reopened.iterations)
        // El verificador de Ana no sirve como el de Beto (el propósito lleva el user_id).
        val error =
            assertThrows(SecretUnavailableException::class.java) {
                PinVerifier.open(sealed, "beto", sealer)
            }
        assertEquals(SecretUnavailableException.Reason.DAMAGED, error.reason)
    }

    @Test
    fun conLaLlavePerdidaOElDatoDanadoNoSeAbre() {
        val sealer = FakeSecretSealer()
        val sealed = PinVerifier.seal(PinVerifier.create("4829", 1, iterations), "ana", sealer)
        val damaged =
            sealed.copyOf().also { it[it.size - 1] = (it[it.size - 1].toInt() xor 1).toByte() }
        assertThrows(SecretUnavailableException::class.java) {
            PinVerifier.open(damaged, "ana", sealer)
        }
        assertThrows(SecretUnavailableException::class.java) { PinVerifier.decode(ByteArray(10)) }
        sealer.destroy()
        val lost =
            assertThrows(SecretUnavailableException::class.java) {
                PinVerifier.open(sealed, "ana", sealer)
            }
        assertEquals(SecretUnavailableException.Reason.KEY_LOST, lost.reason)
    }

    @Test
    fun soloSeCreaConUnPinValido() {
        assertThrows(IllegalArgumentException::class.java) {
            PinVerifier.create("123", 1, iterations)
        }
        assertThrows(IllegalArgumentException::class.java) {
            PinVerifier.create("1234567", 1, iterations)
        }
    }
}
