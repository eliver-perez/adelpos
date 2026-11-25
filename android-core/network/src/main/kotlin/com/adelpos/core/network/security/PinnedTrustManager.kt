package com.adelpos.core.network.security

import android.annotation.SuppressLint
import java.security.MessageDigest
import java.security.cert.CertificateException
import java.security.cert.X509Certificate
import javax.net.ssl.X509TrustManager

/** El servidor presentó un certificado cuya huella no es la fijada en el emparejamiento. */
class PinMismatchException(message: String) : CertificateException(message)

/**
 * Confía solo en el certificado cuya huella SPKI (SHA-256 de la SubjectPublicKeyInfo en DER) es la
 * fijada durante el emparejamiento, como `PinnedVerifier` del escritorio
 * (`desktop/src-tauri/src/pairing.rs`) y `RawSubjectPublicKeyInfo` de Go. No cuentan el nombre, la
 * IP, la cadena ni la vigencia: el servidor del restaurante usa un certificado propio autofirmado y
 * la identidad es su llave pública. Nunca acepta cualquier certificado.
 *
 * Lint advierte de todo X509TrustManager propio (CustomX509TrustManager) porque muchos aceptan
 * cualquier certificado; este hace lo contrario: rechaza todo lo que no sea exactamente la llave
 * fijada (ver PinnedTrustManagerTest y ServerApiTest), que es más estricto que la validación del
 * sistema y no depende de autoridades de certificación que un servidor de red local no puede tener.
 */
@SuppressLint("CustomX509TrustManager")
class PinnedTrustManager(spkiSha256: ByteArray) : X509TrustManager {
    private val pinned: ByteArray = spkiSha256.copyOf()

    init {
        require(pinned.size == SHA256_BYTES) { "La huella debe tener $SHA256_BYTES bytes" }
    }

    override fun checkServerTrusted(chain: Array<out X509Certificate>?, authType: String?) {
        val leaf =
            chain?.firstOrNull() ?: throw PinMismatchException("Sin certificado del servidor")
        val seen = spkiFingerprint(leaf)
        // Comparación en tiempo constante, como Go (subtle.ConstantTimeCompare).
        if (!MessageDigest.isEqual(seen, pinned)) {
            throw PinMismatchException("La huella del servidor no es la fijada")
        }
    }

    override fun checkClientTrusted(chain: Array<out X509Certificate>?, authType: String?) {
        throw CertificateException("No se aceptan certificados de cliente")
    }

    override fun getAcceptedIssuers(): Array<X509Certificate> = emptyArray()

    companion object {
        const val SHA256_BYTES = 32

        /**
         * Huella SPKI de un certificado: `getEncoded()` de su llave pública es la
         * SubjectPublicKeyInfo en DER.
         */
        fun spkiFingerprint(certificate: X509Certificate): ByteArray =
            MessageDigest.getInstance("SHA-256").digest(certificate.publicKey.encoded)
    }
}
