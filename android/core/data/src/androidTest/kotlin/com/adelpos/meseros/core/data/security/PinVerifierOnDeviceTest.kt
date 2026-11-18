package com.adelpos.meseros.core.data.security

import android.os.Build
import android.os.SystemClock
import android.util.Log
import androidx.test.ext.junit.runners.AndroidJUnit4
import com.adelpos.meseros.core.domain.access.PinVerifier
import com.adelpos.meseros.core.domain.security.SecretUnavailableException
import org.junit.After
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertThrows
import org.junit.Assert.assertTrue
import org.junit.Test
import org.junit.runner.RunWith

/**
 * El verificador local del PIN en el dispositivo real: PBKDF2-HMAC-SHA256 del sistema (Conscrypt,
 * desde Android 8) y cifrado con el Android Keystore. También mide el costo de
 * [PinVerifier.DEFAULT_ITERATIONS] en cada versión (el valor se eligió con la tableta de Android
 * 8); el tiempo queda en logcat con la etiqueta `PinVerifierTiming`.
 */
@RunWith(AndroidJUnit4::class)
class PinVerifierOnDeviceTest {
    private val sealer = KeystoreSecretSealer(alias = "adel-pos-test-verifier")

    @After
    fun deleteKey() {
        sealer.destroyKey()
    }

    @Test
    fun seCifraConElKeystoreYSoloAbreParaSuMesero() {
        val verifier = PinVerifier.create("4829", pinVersion = 1)
        val sealed = PinVerifier.seal(verifier, "ana", sealer)
        val reopened = PinVerifier.open(sealed, "ana", sealer)
        assertTrue(reopened.matches("4829"))
        assertFalse(reopened.matches("4828"))
        assertThrows(SecretUnavailableException::class.java) {
            PinVerifier.open(sealed, "beto", sealer)
        }
        // Sin la llave (dispositivo retirado) ya no se puede abrir.
        sealer.destroyKey()
        assertThrows(SecretUnavailableException::class.java) {
            PinVerifier.open(sealed, "ana", sealer)
        }
    }

    @Test
    fun costoDeLaDerivacionEnEsteDispositivo() {
        // Una corrida de calentamiento y tres medidas.
        PinVerifier.create("4829", 1, iterations = 10_000)
        val times =
            (1..3).map {
                val start = SystemClock.elapsedRealtime()
                val verifier = PinVerifier.create("4829", 1)
                val created = SystemClock.elapsedRealtime() - start
                val check = SystemClock.elapsedRealtime()
                assertTrue(verifier.matches("4829"))
                created to SystemClock.elapsedRealtime() - check
            }
        Log.i(
            "PinVerifierTiming",
            "API ${Build.VERSION.SDK_INT} ${Build.MODEL}: ${PinVerifier.DEFAULT_ITERATIONS} " +
                "iteraciones, crear/comprobar en ms: $times",
        )
        assertEquals(3, times.size)
        // Tope de cordura: más de 3 s haría esperar demasiado a un mesero.
        assertTrue("derivación demasiado lenta: $times", times.all { it.second < 3_000 })
    }
}
