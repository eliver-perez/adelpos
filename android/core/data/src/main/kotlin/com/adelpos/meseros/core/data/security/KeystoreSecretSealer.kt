package com.adelpos.meseros.core.data.security

import android.content.Context
import android.content.pm.PackageManager
import android.os.Build
import android.security.keystore.KeyGenParameterSpec
import android.security.keystore.KeyPermanentlyInvalidatedException
import android.security.keystore.KeyProperties
import android.security.keystore.StrongBoxUnavailableException
import androidx.annotation.RequiresApi
import com.adelpos.meseros.core.domain.security.SecretSealer
import com.adelpos.meseros.core.domain.security.SecretUnavailableException
import com.adelpos.meseros.core.domain.security.SecretUnavailableException.Reason
import java.nio.ByteBuffer
import java.security.GeneralSecurityException
import java.security.KeyStore
import java.security.ProviderException
import javax.crypto.AEADBadTagException
import javax.crypto.Cipher
import javax.crypto.KeyGenerator
import javax.crypto.SecretKey
import javax.crypto.spec.GCMParameterSpec

/**
 * Cifra secretos con AES-256-GCM y una llave del Android Keystore que nunca sale del dispositivo
 * (no se puede exportar ni viaja en copias de seguridad). En la Fase B guardará el verificador
 * local del PIN y la credencial de envío por mesero; en la Fase C, la credencial del dispositivo.
 * - La llave se crea en StrongBox (chip seguro, Android 9+) si el dispositivo lo tiene; si no, en
 *   el Keystore normal (respaldado por hardware en casi todos los dispositivos reales).
 * - No exige desbloquear el dispositivo ni autenticar al usuario: los dispositivos se comparten
 *   entre meseros y la cola de pedidos (Fase E) envía con la aplicación cerrada y la pantalla
 *   bloqueada.
 * - El vector de inicialización lo genera el Keystore en cada cifrado (nunca se repite con la misma
 *   llave) y [purpose] entra como dato autenticado: un secreto cifrado para un uso no se abre para
 *   otro.
 *
 * Formato del resultado: versión (1 byte) · largo del vector (1 byte) · vector · texto cifrado con
 * su etiqueta.
 */
class KeystoreSecretSealer(
    private val alias: String = DEFAULT_ALIAS,
    private val strongBoxAvailable: Boolean = false,
) : SecretSealer {

    override fun seal(secret: ByteArray, purpose: String): ByteArray {
        val key = existingKey() ?: createKey()
        try {
            val cipher = Cipher.getInstance(TRANSFORMATION)
            cipher.init(Cipher.ENCRYPT_MODE, key)
            cipher.updateAAD(purpose.toByteArray(Charsets.UTF_8))
            val body = cipher.doFinal(secret)
            val iv = cipher.iv
            return ByteBuffer.allocate(2 + iv.size + body.size)
                .put(FORMAT_VERSION)
                .put(iv.size.toByte())
                .put(iv)
                .put(body)
                .array()
        } catch (exception: KeyPermanentlyInvalidatedException) {
            throw SecretUnavailableException(Reason.KEY_LOST, exception)
        } catch (exception: GeneralSecurityException) {
            throw SecretUnavailableException(Reason.KEYSTORE_FAILED, exception)
        } catch (exception: ProviderException) {
            throw SecretUnavailableException(Reason.KEYSTORE_FAILED, exception)
        }
    }

    override fun open(sealed: ByteArray, purpose: String): ByteArray {
        val buffer = ByteBuffer.wrap(sealed)
        if (sealed.size < 2 || buffer.get() != FORMAT_VERSION) {
            throw SecretUnavailableException(Reason.DAMAGED)
        }
        val ivSize = buffer.get().toInt()
        if (ivSize != GCM_IV_BYTES || buffer.remaining() < ivSize + GCM_TAG_BYTES) {
            throw SecretUnavailableException(Reason.DAMAGED)
        }
        val iv = ByteArray(ivSize)
        buffer.get(iv)
        val body = ByteArray(buffer.remaining())
        buffer.get(body)
        // Al abrir nunca se crea una llave: si ya no existe, el secreto se perdió.
        val key = existingKey() ?: throw SecretUnavailableException(Reason.KEY_LOST)
        try {
            val cipher = Cipher.getInstance(TRANSFORMATION)
            cipher.init(Cipher.DECRYPT_MODE, key, GCMParameterSpec(GCM_TAG_BYTES * 8, iv))
            cipher.updateAAD(purpose.toByteArray(Charsets.UTF_8))
            return cipher.doFinal(body)
        } catch (exception: KeyPermanentlyInvalidatedException) {
            throw SecretUnavailableException(Reason.KEY_LOST, exception)
        } catch (exception: AEADBadTagException) {
            // Otra llave (se creó una nueva después de perder la anterior), otro propósito o datos
            // alterados.
            throw SecretUnavailableException(Reason.DAMAGED, exception)
        } catch (exception: GeneralSecurityException) {
            throw SecretUnavailableException(Reason.KEYSTORE_FAILED, exception)
        } catch (exception: ProviderException) {
            throw SecretUnavailableException(Reason.KEYSTORE_FAILED, exception)
        }
    }

    /**
     * Borra la llave: todo lo cifrado con ella queda ilegible (revocación del dispositivo, Fase B).
     */
    fun destroyKey() {
        try {
            keyStore().deleteEntry(alias)
        } catch (exception: GeneralSecurityException) {
            throw SecretUnavailableException(Reason.KEYSTORE_FAILED, exception)
        }
    }

    private fun existingKey(): SecretKey? =
        try {
            keyStore().getKey(alias, null) as? SecretKey
        } catch (exception: GeneralSecurityException) {
            throw SecretUnavailableException(Reason.KEYSTORE_FAILED, exception)
        }

    @Synchronized
    private fun createKey(): SecretKey {
        // Otro hilo pudo crearla mientras este esperaba.
        existingKey()?.let {
            return it
        }
        try {
            if (strongBoxAvailable && Build.VERSION.SDK_INT >= Build.VERSION_CODES.P) {
                generateStrongBoxKeyOrNull()?.let {
                    return it
                }
            }
            return generateKey(strongBox = false)
        } catch (exception: GeneralSecurityException) {
            throw SecretUnavailableException(Reason.KEYSTORE_FAILED, exception)
        } catch (exception: ProviderException) {
            throw SecretUnavailableException(Reason.KEYSTORE_FAILED, exception)
        }
    }

    // El dispositivo anuncia StrongBox pero no lo puede usar ahora: null, y la llave va al Keystore
    // normal.
    @RequiresApi(Build.VERSION_CODES.P)
    private fun generateStrongBoxKeyOrNull(): SecretKey? =
        try {
            generateKey(strongBox = true)
        } catch (_: StrongBoxUnavailableException) {
            null
        }

    private fun generateKey(strongBox: Boolean): SecretKey {
        val spec =
            KeyGenParameterSpec.Builder(
                    alias,
                    KeyProperties.PURPOSE_ENCRYPT or KeyProperties.PURPOSE_DECRYPT,
                )
                .setBlockModes(KeyProperties.BLOCK_MODE_GCM)
                .setEncryptionPaddings(KeyProperties.ENCRYPTION_PADDING_NONE)
                .setKeySize(KEY_BITS)
                .setRandomizedEncryptionRequired(true)
        if (strongBox && Build.VERSION.SDK_INT >= Build.VERSION_CODES.P) {
            spec.setIsStrongBoxBacked(true)
        }
        val generator = KeyGenerator.getInstance(KeyProperties.KEY_ALGORITHM_AES, KEYSTORE)
        generator.init(spec.build())
        return generator.generateKey()
    }

    private fun keyStore(): KeyStore = KeyStore.getInstance(KEYSTORE).apply { load(null) }

    companion object {
        const val DEFAULT_ALIAS = "adel-pos-secrets-v1"
        private const val KEYSTORE = "AndroidKeyStore"
        private const val TRANSFORMATION = "AES/GCM/NoPadding"
        private const val KEY_BITS = 256
        private const val GCM_IV_BYTES = 12
        private const val GCM_TAG_BYTES = 16
        private const val FORMAT_VERSION: Byte = 1

        /** Sellador con la llave de la aplicación, en StrongBox si el dispositivo lo tiene. */
        fun create(context: Context, alias: String = DEFAULT_ALIAS): KeystoreSecretSealer {
            val strongBox =
                Build.VERSION.SDK_INT >= Build.VERSION_CODES.P &&
                    context.packageManager.hasSystemFeature(
                        PackageManager.FEATURE_STRONGBOX_KEYSTORE,
                    )
            return KeystoreSecretSealer(alias, strongBox)
        }
    }
}
