package local.adel.pos.android.core.domain.server

import local.adel.pos.android.core.domain.text.Hex

/**
 * Un servidor del restaurante encontrado en la red local por NSD (mDNS/DNS-SD, `_adelpos._tcp`). El
 * anuncio solo sirve para encontrar la dirección, **nunca para confiar**: la identidad la da la
 * huella SPKI completa al conectarse (al emparejar, el código la ata; después, la huella fijada).
 *
 * @param serverId identificador del servidor (UUID en minúsculas, TXT `server_id`).
 * @param name nombre legible de la instancia («Fonda Doña Ñeli — Mac de caja»).
 * @param fingerprintPrefix primeros 8 bytes de la huella SPKI en hexadecimal (TXT `fp`).
 * @param port puerto HTTPS de la escucha de red.
 * @param hosts direcciones de la red local donde se anunció (solo las que acepta [ServerAddress]).
 */
data class FoundServer(
    val serverId: String,
    val name: String,
    val fingerprintPrefix: String,
    val port: Int,
    val hosts: List<String>,
) {
    /** Bases HTTPS que se pueden probar, en el orden en que llegaron. */
    fun bases(): List<String> = hosts.map { host -> "https://$host:$port" }
}

/**
 * Lectura de los anuncios del servidor, con las reglas de `collect` del escritorio
 * (`desktop/src-tauri/src/discovery.rs`): el TXT lleva `txtvers=1`, `server_id`, `api=v1`, `port` y
 * `fp` (como los escribe `backend/internal/discovery`). Solo se acepta un anuncio completo y
 * válido: identificador UUID en minúsculas, huella corta de 16 hexadecimales, puerto y al menos una
 * dirección de la red local. Un mismo `server_id` anunciado varias veces junta sus direcciones.
 */
object ServerAnnouncements {
    const val SERVICE_TYPE = "_adelpos._tcp"
    private const val FINGERPRINT_PREFIX_LENGTH = 16
    private const val MAX_NAME = 80
    private val serverIdPattern =
        Regex("^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$")
    private val prefixPattern = Regex("^[0-9a-f]{16}$")

    /**
     * Un anuncio resuelto. [txt] trae los valores tal como los da el sistema (bytes o null); las
     * claves se comparan sin mayúsculas. [port] es el del registro SRV; si el TXT trae otro, cuenta
     * el del SRV (es el que de verdad escucha). [hosts] son las direcciones IP resueltas en texto.
     */
    fun parse(
        instanceName: String,
        txt: Map<String, ByteArray?>,
        port: Int,
        hosts: List<String>,
    ): FoundServer? {
        val values =
            txt.entries.associate { (key, value) -> key.lowercase() to value?.decodeText() }
        val serverId = values["server_id"].orEmpty()
        val prefix = values["fp"].orEmpty()
        if (!serverIdPattern.matches(serverId) || !prefixPattern.matches(prefix)) {
            return null
        }
        if (port !in 1..65535) {
            return null
        }
        val localHosts = hosts.map(::bracketed).filter(ServerAddress::isLocalHost).distinct()
        if (localHosts.isEmpty()) {
            return null
        }
        return FoundServer(
            serverId = serverId,
            name = displayName(instanceName),
            fingerprintPrefix = prefix,
            port = port,
            hosts = localHosts,
        )
    }

    /** Junta los anuncios repetidos del mismo servidor (por `server_id`, nunca por el nombre). */
    fun merge(found: List<FoundServer>): List<FoundServer> {
        val merged = LinkedHashMap<String, FoundServer>()
        for (server in found) {
            val existing = merged[server.serverId]
            merged[server.serverId] =
                if (existing == null) {
                    server
                } else {
                    existing.copy(hosts = (existing.hosts + server.hosts).distinct())
                }
        }
        return merged.values.toList()
    }

    /**
     * Direcciones donde probar el servidor emparejado (cambio de IP): solo las del mismo
     * `server_id` **y** la misma huella corta (las demás ni se intentan) y distintas de la que
     * acaba de fallar. La comprobación de verdad es la huella completa al conectar.
     */
    fun candidates(
        found: List<FoundServer>,
        serverId: String,
        fingerprint: ByteArray,
        failedBase: String,
    ): List<String> {
        val prefix = shortFingerprint(fingerprint)
        return found
            .filter { it.serverId == serverId && it.fingerprintPrefix == prefix }
            .flatMap(FoundServer::bases)
            .filter { it != failedBase }
            .distinct()
    }

    /** Huella corta del anuncio: los primeros 8 bytes de la huella SPKI en hexadecimal. */
    fun shortFingerprint(fingerprint: ByteArray): String =
        Hex.encode(fingerprint.copyOfRange(0, 8)).also {
            check(it.length == FINGERPRINT_PREFIX_LENGTH)
        }

    private fun ByteArray.decodeText(): String = String(this, Charsets.UTF_8)

    // Una IPv6 en texto se escribe comprimida y entre corchetes para formar la URL; una con zona
    // («fe80::1%wlan0») queda tal cual y la descarta ServerAddress.isLocalHost.
    private fun bracketed(host: String): String {
        if (!host.contains(':')) {
            return host
        }
        val bare = host.removePrefix("[").removeSuffix("]")
        val groups = IpLiteral.parseIpv6(bare) ?: return host
        return "[${IpLiteral.formatIpv6(groups)}]"
    }

    private fun displayName(instance: String): String =
        unescapeInstance(instance).filterNot(Char::isISOControl).trim().take(MAX_NAME)

    /**
     * Nombre de instancia sin los escapes de DNS-SD que algunas versiones de Android dejan en
     * `NsdServiceInfo.serviceName` («Fonda\032Doña» → «Fonda Doña»): `\ddd` es un byte en decimal y
     * `\x`, el carácter x. Los bytes juntos se leen como UTF-8.
     */
    fun unescapeInstance(name: String): String {
        if (!name.contains('\\')) {
            return name
        }
        val bytes = java.io.ByteArrayOutputStream()
        var index = 0
        while (index < name.length) {
            val character = name[index]
            val decimal = name.substring(index + 1, minOf(index + 4, name.length))
            when {
                character != '\\' -> {
                    // Por punto de código: un emoji ocupa dos caracteres de Kotlin.
                    val codePoint = name.codePointAt(index)
                    bytes.write(String(Character.toChars(codePoint)).toByteArray(Charsets.UTF_8))
                    index += Character.charCount(codePoint)
                }
                decimal.length == 3 && decimal.all { it in '0'..'9' } && decimal.toInt() <= 255 -> {
                    bytes.write(decimal.toInt())
                    index += 4
                }
                index + 1 < name.length -> {
                    bytes.write(name[index + 1].toString().toByteArray(Charsets.UTF_8))
                    index += 2
                }
                else -> index += 1
            }
        }
        return String(bytes.toByteArray(), Charsets.UTF_8)
    }
}
