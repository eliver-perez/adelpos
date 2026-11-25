package com.adelpos.core.model.pairing

import com.adelpos.core.model.text.Hex
import java.security.MessageDigest
import java.security.SecureRandom

/**
 * Cálculos del emparejamiento de la etapa 9G, idénticos a Go (`PairingCode` y `PairingCommitment`
 * de `backend/internal/administration/pairing.go`) y a Rust (`desktop/src-tauri/src/pairing.rs`).
 *
 * Protocolo (la explicación completa de qué protege está en `pairing.go`):
 * 1. El dispositivo genera un número aleatorio nT de 32 bytes y envía su compromiso SHA-256(nT).
 * 2. El servidor responde con su número nS; solo después el dispositivo revela nT.
 * 3. Ambos derivan el código de 6 dígitos con la huella SPKI del certificado que cada uno conoce:
 *    el dispositivo, la del primer certificado que vio en la sesión; el servidor, la suya. Con un
 *    intermediario (otro certificado) los códigos no coinciden y el administrador no aprueba.
 */
object PairingProtocol {
    const val NONCE_BYTES = 32
    private const val FINGERPRINT_BYTES = 32
    private const val CODE_MODULUS = 1_000_000UL
    private val requestIdPattern =
        Regex("^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$")

    private val random = SecureRandom()

    /** Número aleatorio nuevo del dispositivo (32 bytes de un generador criptográfico). */
    fun newNonce(): ByteArray = ByteArray(NONCE_BYTES).also(random::nextBytes)

    /** Compromiso del número del dispositivo: SHA-256(nT) en hexadecimal en minúsculas. */
    fun commitment(terminalNonce: ByteArray): String {
        require(terminalNonce.size == NONCE_BYTES) { "El número debe tener 32 bytes" }
        return Hex.encode(sha256(terminalNonce))
    }

    /**
     * Código de 6 dígitos: SHA-256(huella ‖ nT ‖ nS ‖ id de la solicitud en 16 bytes); los 8
     * primeros bytes del resumen como entero sin signo big-endian, módulo 10⁶, con ceros a la
     * izquierda.
     *
     * @throws IllegalArgumentException si un campo no tiene su tamaño o el id no es un UUID.
     */
    fun code(
        fingerprint: ByteArray,
        terminalNonce: ByteArray,
        serverNonce: ByteArray,
        requestId: String,
    ): String {
        require(fingerprint.size == FINGERPRINT_BYTES) { "La huella debe tener 32 bytes" }
        require(terminalNonce.size == NONCE_BYTES && serverNonce.size == NONCE_BYTES) {
            "Los números aleatorios deben tener 32 bytes"
        }
        require(requestIdPattern.matches(requestId)) {
            "La solicitud de emparejamiento no es válida"
        }
        val identifier = Hex.decode(requestId.replace("-", "").lowercase())
        val digest = MessageDigest.getInstance("SHA-256")
        digest.update(fingerprint)
        digest.update(terminalNonce)
        digest.update(serverNonce)
        digest.update(identifier)
        val sum = digest.digest()
        var value = 0UL
        for (index in 0 until 8) {
            value = (value shl 8) or (sum[index].toUByte().toULong())
        }
        return (value % CODE_MODULUS).toString().padStart(6, '0')
    }

    /** El código en dos grupos de tres para leerlo en voz alta sin errores: «123 456». */
    fun grouped(code: String): String =
        if (code.length == 6) "${code.substring(0, 3)} ${code.substring(3)}" else code

    private fun sha256(bytes: ByteArray): ByteArray =
        MessageDigest.getInstance("SHA-256").digest(bytes)
}
