package com.adelpos.meseros.core.domain.access

import com.adelpos.core.model.security.SecretSealer
import com.adelpos.core.model.security.SecretUnavailableException
import java.nio.ByteBuffer
import java.security.MessageDigest
import java.security.SecureRandom
import javax.crypto.SecretKeyFactory
import javax.crypto.spec.PBEKeySpec

/**
 * Verificador local del PIN de un mesero (§3.2, «Entrada sin conexión»): permite comprobar el PIN
 * sin conexión, solo para quien ya entró con conexión en **este** dispositivo. Nunca se guarda el
 * PIN: se guarda una derivación lenta con sal (PBKDF2-HMAC-SHA256), cifrada además con la llave del
 * Android Keystore ([SecretSealer], A1-D13) y ligada al mesero (el propósito del cifrado lleva su
 * `user_id`: el verificador de un mesero no se puede usar para otro).
 *
 * Un PIN tiene pocas combinaciones (10⁴ a 10⁶): la defensa principal es que el verificador cifrado
 * no se puede abrir fuera del dispositivo (la llave no se exporta) y el límite local de intentos
 * ([OfflineAttemptPolicy]); la derivación lenta encarece además cada prueba.
 *
 * @param pinVersion la `pin_version` con que se creó: si en el servidor cambia (PIN cambiado o
 *   quitado), el verificador deja de valer.
 */
class PinVerifier(
    val pinVersion: Int,
    val iterations: Int,
    salt: ByteArray,
    hash: ByteArray,
) {
    private val salt: ByteArray = salt.copyOf()
    private val hash: ByteArray = hash.copyOf()

    init {
        require(pinVersion >= 0) { "pin_version no válida" }
        require(iterations > 0) { "Iteraciones no válidas" }
        require(this.salt.size == SALT_BYTES && this.hash.size == HASH_BYTES) {
            "Verificador dañado"
        }
    }

    /** Si [pin] es el PIN con que se creó; la comparación es en tiempo constante. */
    fun matches(pin: String): Boolean {
        if (!PIN_PATTERN.matches(pin)) {
            return false
        }
        return MessageDigest.isEqual(derive(pin, salt, iterations), hash)
    }

    /**
     * Bytes para guardar (antes de cifrarlos): versión · pin_version · iteraciones · sal · hash.
     */
    fun encode(): ByteArray =
        ByteBuffer.allocate(1 + 4 + 4 + SALT_BYTES + HASH_BYTES)
            .put(FORMAT_VERSION)
            .putInt(pinVersion)
            .putInt(iterations)
            .put(salt)
            .put(hash)
            .array()

    companion object {
        /**
         * Iteraciones de PBKDF2-HMAC-SHA256, medidas con PinVerifierOnDeviceTest en la tableta de
         * Android 8 (`ADEL_A1_Tablet_API26`): 0.27 a 0.32 s por derivación (en `Pixel_8`, Android
         * 17: 0.37 a 0.54 s). Son emuladores sobre la Mac: una tableta económica real puede tardar
         * varias veces más; se debe volver a medir en el dispositivo físico. Cada verificador
         * guarda sus iteraciones, así que cambiar este valor no invalida los anteriores (el nuevo
         * vale desde la siguiente entrada con conexión).
         */
        const val DEFAULT_ITERATIONS = 120_000
        const val SALT_BYTES = 16
        const val HASH_BYTES = 32
        private const val FORMAT_VERSION: Byte = 1
        private const val ALGORITHM = "PBKDF2WithHmacSHA256"
        private val PIN_PATTERN = Regex("^[0-9]{4,6}$")
        private val random = SecureRandom()

        /** Crea el verificador de [pin] con una sal nueva (en cada entrada con conexión). */
        fun create(
            pin: String,
            pinVersion: Int,
            iterations: Int = DEFAULT_ITERATIONS,
        ): PinVerifier {
            require(PIN_PATTERN.matches(pin)) { "El PIN debe tener de 4 a 6 dígitos" }
            val salt = ByteArray(SALT_BYTES).also(random::nextBytes)
            return PinVerifier(pinVersion, iterations, salt, derive(pin, salt, iterations))
        }

        /** Lee lo que escribió [encode]; lanza [SecretUnavailableException] si está dañado. */
        fun decode(bytes: ByteArray): PinVerifier {
            val buffer = ByteBuffer.wrap(bytes)
            if (
                bytes.size != 1 + 4 + 4 + SALT_BYTES + HASH_BYTES || buffer.get() != FORMAT_VERSION
            ) {
                throw SecretUnavailableException(SecretUnavailableException.Reason.DAMAGED)
            }
            val pinVersion = buffer.getInt()
            val iterations = buffer.getInt()
            val salt = ByteArray(SALT_BYTES).also { buffer.get(it) }
            val hash = ByteArray(HASH_BYTES).also { buffer.get(it) }
            if (pinVersion < 0 || iterations <= 0) {
                throw SecretUnavailableException(SecretUnavailableException.Reason.DAMAGED)
            }
            return PinVerifier(pinVersion, iterations, salt, hash)
        }

        /** Propósito del cifrado del verificador de [userId] (dato autenticado de AES-GCM). */
        fun purpose(userId: String): String = "pin-verifier:$userId"

        /** Cifra el verificador de [userId] con [sealer] (Android Keystore). */
        fun seal(verifier: PinVerifier, userId: String, sealer: SecretSealer): ByteArray =
            sealer.seal(verifier.encode(), purpose(userId))

        /** Abre el verificador cifrado de [userId]; lanza [SecretUnavailableException]. */
        fun open(sealed: ByteArray, userId: String, sealer: SecretSealer): PinVerifier =
            decode(sealer.open(sealed, purpose(userId)))

        private fun derive(pin: String, salt: ByteArray, iterations: Int): ByteArray {
            val spec = PBEKeySpec(pin.toCharArray(), salt, iterations, HASH_BYTES * 8)
            try {
                return SecretKeyFactory.getInstance(ALGORITHM).generateSecret(spec).encoded
            } finally {
                spec.clearPassword()
            }
        }
    }
}
