package com.adelpos.core.network.pairing

/**
 * El servidor emparejado y la credencial del dispositivo, como los deja el emparejamiento
 * ([PairingRepository]) al terminar. La credencial va cifrada con el Android Keystore con el
 * propósito [CREDENTIAL_PURPOSE]; nunca en claro.
 */
data class PairedDevice(
    val serverId: String,
    val serverName: String,
    val baseUrl: String,
    val fingerprintHex: String,
    val deviceId: String,
    val deviceName: String,
    val sealedDeviceToken: ByteArray,
    val pairedAtMillis: Long,
) {
    // Nunca muestra la credencial (ni cifrada) en un registro.
    override fun toString(): String = "PairedDevice($serverId, $deviceId)"

    override fun equals(other: Any?): Boolean =
        other is PairedDevice &&
            other.serverId == serverId &&
            other.deviceId == deviceId &&
            other.sealedDeviceToken.contentEquals(sealedDeviceToken)

    override fun hashCode(): Int = 31 * serverId.hashCode() + deviceId.hashCode()

    companion object {
        /**
         * Propósito del cifrado de la credencial del dispositivo (dato autenticado del sobre). Se
         * guarda en el dispositivo: es un valor fijo (K1-D2), el mismo de la 0.4.0 de meseros, y
         * nunca se deriva del paquete.
         */
        const val CREDENTIAL_PURPOSE = "device-credential"
    }
}

/**
 * Dónde guarda cada aplicación el servidor emparejado (etapa K1): la de meseros en su `AppDatabase`
 * (`paired_server`), la de cocina en su propia base. Se llama una vez por emparejamiento, con la
 * credencial ya cifrada; si falla, el emparejamiento lo vuelve a intentar en la siguiente consulta
 * sin reclamar otra credencial.
 */
fun interface PairingStore {
    suspend fun onPaired(device: PairedDevice)
}
