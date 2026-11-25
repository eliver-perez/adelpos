package com.adelpos.core.model.security

/**
 * Cifra secretos que no deben salir del dispositivo (la credencial del dispositivo y la credencial
 * de envío por mesero de las fases B y C, el verificador local del PIN) con una llave que nunca se
 * exporta. La implementación real usa el Android Keystore; las pruebas de la JVM usan una falsa.
 *
 * [purpose] liga el cifrado a su uso (por ejemplo, "device-credential"): un secreto cifrado para un
 * propósito no se puede abrir con otro.
 */
interface SecretSealer {
    /** Cifra [secret]; el resultado incluye todo lo necesario para abrirlo (salvo la llave). */
    fun seal(secret: ByteArray, purpose: String): ByteArray

    /** Abre lo que cifró [seal]. Lanza [SecretUnavailableException] si ya no se puede. */
    fun open(sealed: ByteArray, purpose: String): ByteArray
}

/**
 * Un secreto guardado ya no se puede abrir. En todos los casos la salida es la misma: olvidarlo y
 * pedirlo de nuevo (emparejar otra vez o entrar con conexión); nunca se intenta adivinar ni se
 * reutiliza otro.
 */
class SecretUnavailableException(val reason: Reason, cause: Throwable? = null) :
    Exception("Secreto no disponible: $reason", cause) {
    enum class Reason {
        /**
         * La llave ya no existe o el sistema la invalidó (datos de la aplicación borrados,
         * restauración).
         */
        KEY_LOST,

        /** El dato no corresponde a la llave, al propósito o está dañado. */
        DAMAGED,

        /** El almacén de llaves del sistema falló. */
        KEYSTORE_FAILED,
    }
}
