package com.adelpos.core.network.security

import android.annotation.SuppressLint
import java.security.MessageDigest
import java.security.cert.CertificateException
import java.security.cert.X509Certificate
import javax.net.ssl.X509TrustManager

/**
 * Confianza de **una sesión de emparejamiento** (como `PinnedVerifier::first_seen` del escritorio):
 * fija la huella SPKI del primer certificado que presenta el servidor y desde entonces solo acepta
 * ese mismo en las conexiones siguientes de la sesión. Esa huella entra en el código de 6 dígitos
 * (PairingProtocol): si un intermediario presentó otro certificado, el código que ve el
 * administrador en la computadora principal no coincide y no se aprueba. Aprobado el código, la
 * huella queda fijada para siempre ([PinnedTrustManager]).
 *
 * Nunca se usa fuera del emparejamiento: un dispositivo emparejado solo confía en la huella
 * guardada.
 *
 * Lint advierte de todo X509TrustManager propio (CustomX509TrustManager): este acepta el primer
 * certificado de la sesión a propósito, porque un servidor de la red local no puede tener un
 * certificado de una autoridad y la identidad la confirma el código que aprueba el administrador.
 */
@SuppressLint("CustomX509TrustManager")
class FirstSeenTrustManager : X509TrustManager {
    private var pinned: ByteArray? = null

    /** Huella fijada en la sesión (la del primer certificado), o null si todavía no conectó. */
    val fingerprint: ByteArray?
        @Synchronized get() = pinned?.copyOf()

    @Synchronized
    override fun checkServerTrusted(chain: Array<out X509Certificate>?, authType: String?) {
        val leaf =
            chain?.firstOrNull() ?: throw PinMismatchException("Sin certificado del servidor")
        val seen = PinnedTrustManager.spkiFingerprint(leaf)
        val current = pinned
        if (current == null) {
            pinned = seen
            return
        }
        // Un certificado distinto a mitad de la sesión (otro servidor o un intermediario) se
        // rechaza.
        if (!MessageDigest.isEqual(seen, current)) {
            throw PinMismatchException(
                "El servidor cambió de certificado durante el emparejamiento",
            )
        }
    }

    override fun checkClientTrusted(chain: Array<out X509Certificate>?, authType: String?) {
        throw CertificateException("No se aceptan certificados de cliente")
    }

    override fun getAcceptedIssuers(): Array<X509Certificate> = emptyArray()
}
