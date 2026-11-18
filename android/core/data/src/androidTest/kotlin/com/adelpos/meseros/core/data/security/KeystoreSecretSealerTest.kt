package com.adelpos.meseros.core.data.security

import android.security.keystore.KeyInfo
import androidx.test.ext.junit.runners.AndroidJUnit4
import androidx.test.platform.app.InstrumentationRegistry
import com.adelpos.meseros.core.domain.security.SecretUnavailableException
import java.security.KeyStore
import javax.crypto.SecretKey
import javax.crypto.SecretKeyFactory
import org.junit.After
import org.junit.Assert.assertArrayEquals
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNull
import org.junit.Assert.fail
import org.junit.Test
import org.junit.runner.RunWith

/**
 * Cifrado con el Android Keystore en el dispositivo: ida y vuelta, llave no exportable y errores
 * controlados.
 */
@RunWith(AndroidJUnit4::class)
class KeystoreSecretSealerTest {
    private val alias = "adel-pos-prueba-${System.nanoTime()}"
    private val context = InstrumentationRegistry.getInstrumentation().targetContext
    private val sealer = KeystoreSecretSealer.create(context, alias)
    private val secret = "credencial-de-prueba-ñ".toByteArray(Charsets.UTF_8)

    @After
    fun deleteKey() {
        sealer.destroyKey()
    }

    private fun expectFailure(reason: SecretUnavailableException.Reason, block: () -> Unit) {
        try {
            block()
            fail("Se esperaba SecretUnavailableException($reason)")
        } catch (exception: SecretUnavailableException) {
            assertEquals(reason, exception.reason)
        }
    }

    @Test
    fun cifraYDescifra() {
        val sealed = sealer.seal(secret, "device-credential")
        assertFalse(
            "El secreto no debe quedar a la vista",
            String(sealed, Charsets.ISO_8859_1).contains("credencial"),
        )
        assertArrayEquals(secret, sealer.open(sealed, "device-credential"))
        // Cada cifrado usa un vector nuevo: el mismo secreto no produce el mismo resultado.
        val again = sealer.seal(secret, "device-credential")
        assertFalse(sealed.contentEquals(again))
        assertArrayEquals(secret, sealer.open(again, "device-credential"))
    }

    @Test
    fun laLlaveEsAes256DelKeystoreYNoSeExporta() {
        sealer.seal(secret, "device-credential")
        val keyStore = KeyStore.getInstance("AndroidKeyStore").apply { load(null) }
        val key = keyStore.getKey(alias, null) as SecretKey
        // Una llave del Keystore nunca entrega su material.
        assertNull(key.encoded)
        val info =
            SecretKeyFactory.getInstance(key.algorithm, "AndroidKeyStore")
                .getKeySpec(key, KeyInfo::class.java) as KeyInfo
        assertEquals(256, info.keySize)
        assertFalse(info.isUserAuthenticationRequired)
    }

    @Test
    fun otroPropositoNoAbreElSecreto() {
        val sealed = sealer.seal(secret, "device-credential")
        expectFailure(SecretUnavailableException.Reason.DAMAGED) {
            sealer.open(sealed, "pin-verifier")
        }
    }

    @Test
    fun datosAlteradosOIncompletosSonUnErrorControlado() {
        val sealed = sealer.seal(secret, "device-credential")
        val altered = sealed.copyOf()
        altered[altered.size - 1] = (altered[altered.size - 1].toInt() xor 1).toByte()
        expectFailure(SecretUnavailableException.Reason.DAMAGED) {
            sealer.open(altered, "device-credential")
        }
        expectFailure(SecretUnavailableException.Reason.DAMAGED) {
            sealer.open(sealed.copyOf(10), "device-credential")
        }
        expectFailure(SecretUnavailableException.Reason.DAMAGED) {
            sealer.open(ByteArray(0), "device-credential")
        }
    }

    @Test
    fun llavePerdidaEsUnErrorControlado() {
        val sealed = sealer.seal(secret, "device-credential")
        sealer.destroyKey()
        expectFailure(SecretUnavailableException.Reason.KEY_LOST) {
            sealer.open(sealed, "device-credential")
        }
        // Con una llave nueva, lo cifrado con la anterior sigue sin abrirse (no se adivina ni se
        // reutiliza).
        val fresh = sealer.seal(secret, "device-credential")
        assertArrayEquals(secret, sealer.open(fresh, "device-credential"))
        expectFailure(SecretUnavailableException.Reason.DAMAGED) {
            sealer.open(sealed, "device-credential")
        }
    }
}
