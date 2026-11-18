package local.adel.pos.android.core.domain.text

/** Hexadecimal en minúsculas, como guarda Go las huellas. */
object Hex {
    private val pattern = Regex("^([0-9a-f]{2})*$")

    fun encode(bytes: ByteArray): String =
        bytes.joinToString(separator = "") { byte -> "%02x".format(byte) }

    fun decode(text: String): ByteArray {
        require(pattern.matches(text)) { "Hexadecimal no válido" }
        return ByteArray(text.length / 2) { index ->
            text.substring(index * 2, index * 2 + 2).toInt(16).toByte()
        }
    }
}
