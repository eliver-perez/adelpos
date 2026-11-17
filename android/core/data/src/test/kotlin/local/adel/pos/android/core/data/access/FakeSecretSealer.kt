package local.adel.pos.android.core.data.access

import java.nio.ByteBuffer
import java.security.SecureRandom
import javax.crypto.AEADBadTagException
import javax.crypto.Cipher
import javax.crypto.KeyGenerator
import javax.crypto.SecretKey
import javax.crypto.spec.GCMParameterSpec
import local.adel.pos.android.core.domain.security.SecretSealer
import local.adel.pos.android.core.domain.security.SecretUnavailableException

/**
 * Keystore falso para las pruebas de la JVM: AES-256-GCM con una llave en memoria y el propósito
 * como dato autenticado, igual que `KeystoreSecretSealer` (que solo existe en Android). [destroy]
 * simula la llave perdida.
 */
class FakeSecretSealer : SecretSealer {
    private var key: SecretKey? = KeyGenerator.getInstance("AES").apply { init(256) }.generateKey()
    private val random = SecureRandom()

    fun destroy() {
        key = null
    }

    override fun seal(secret: ByteArray, purpose: String): ByteArray {
        val current =
            key ?: throw SecretUnavailableException(SecretUnavailableException.Reason.KEY_LOST)
        val iv = ByteArray(12).also(random::nextBytes)
        val cipher = Cipher.getInstance("AES/GCM/NoPadding")
        cipher.init(Cipher.ENCRYPT_MODE, current, GCMParameterSpec(128, iv))
        cipher.updateAAD(purpose.toByteArray())
        val body = cipher.doFinal(secret)
        return ByteBuffer.allocate(iv.size + body.size).put(iv).put(body).array()
    }

    override fun open(sealed: ByteArray, purpose: String): ByteArray {
        val current =
            key ?: throw SecretUnavailableException(SecretUnavailableException.Reason.KEY_LOST)
        if (sealed.size < 28) {
            throw SecretUnavailableException(SecretUnavailableException.Reason.DAMAGED)
        }
        val cipher = Cipher.getInstance("AES/GCM/NoPadding")
        cipher.init(Cipher.DECRYPT_MODE, current, GCMParameterSpec(128, sealed, 0, 12))
        cipher.updateAAD(purpose.toByteArray())
        try {
            return cipher.doFinal(sealed, 12, sealed.size - 12)
        } catch (exception: AEADBadTagException) {
            throw SecretUnavailableException(SecretUnavailableException.Reason.DAMAGED, exception)
        }
    }
}
