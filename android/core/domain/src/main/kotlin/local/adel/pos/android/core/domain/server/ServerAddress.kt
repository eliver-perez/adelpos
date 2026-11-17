package local.adel.pos.android.core.domain.server

/**
 * Dirección del servidor del restaurante escrita a mano, validada como la valida el escritorio
 * (`normalize_address` y `valid_server_host` de `desktop/src-tauri/src/pairing.rs`, decisión D2 de
 * la 9G): HTTPS a una IPv4 privada (10/8, 172.16/12, 192.168/16) o de enlace local (169.254/16), a
 * una IPv6 única local (fc00::/7) o de enlace local (fe80::/10), o a un nombre `.local` (mDNS).
 * Nunca loopback, una IP pública ni otro nombre: el dispositivo solo habla con la red del
 * restaurante. El puerto por omisión es el de la escucha HTTPS del servidor, 8443.
 *
 * Diferencias deliberadas con el escritorio (más estrictas o más claras, nunca más permisivas): una
 * IPv4 con ceros a la izquierda («192.168.01.20») o abreviada («192.168.1») se rechaza en lugar de
 * interpretarse como octal o completarse; el puerto 443 escrito se respeta (la biblioteca de URL
 * del escritorio lo omite por ser el de HTTPS y termina usando 8443); el puerto 0 se rechaza.
 */
object ServerAddress {
    const val DEFAULT_PORT = 8443
    private const val MAX_INPUT = 270
    private const val MAX_NAME = 253
    private const val MAX_LABEL = 63
    private const val SCHEME = "https://"

    /** Por qué no se aceptó una dirección. */
    enum class Problem {
        /** Vacía o mal escrita (espacios, otro esquema, ruta, usuario, consulta…). */
        MALFORMED,

        /** Bien escrita, pero fuera de la red local del restaurante. */
        NOT_LOCAL,
    }

    /** Resultado de [normalize]: la base `https://host:puerto` o el problema. */
    sealed interface Result {
        data class Valid(val base: String) : Result

        data class Invalid(val problem: Problem) : Result
    }

    /**
     * Valida lo que escribió el usuario y devuelve la base `https://host:puerto`. Acepta
     * «192.168.1.20», «192.168.1.20:8443», «caja.local», «[fd00::5]» y lo mismo con «https://» (y
     * una «/» final).
     */
    fun normalize(input: String): Result {
        val text = input.trim()
        if (text.isEmpty() || text.length > MAX_INPUT || text.any(Char::isWhitespace)) {
            return Result.Invalid(Problem.MALFORMED)
        }
        val rest =
            when {
                text.startsWith(SCHEME, ignoreCase = true) -> text.substring(SCHEME.length)
                text.contains("://") -> return Result.Invalid(Problem.MALFORMED)
                else -> text
            }
        // Solo «host[:puerto]» y, como mucho, una «/» final: nada de ruta, consulta, fragmento ni
        // usuario.
        val authority = if (rest.endsWith("/")) rest.dropLast(1) else rest
        if (authority.isEmpty() || authority.any { it in "/?#@\\" }) {
            return Result.Invalid(Problem.MALFORMED)
        }
        val (rawHost, port) = splitHostPort(authority) ?: return Result.Invalid(Problem.MALFORMED)
        val host = canonicalHost(rawHost) ?: return Result.Invalid(Problem.MALFORMED)
        if (!isLocalHost(host)) {
            return Result.Invalid(Problem.NOT_LOCAL)
        }
        return Result.Valid("https://$host:$port")
    }

    /**
     * Host aceptable para un servidor emparejado o encontrado en la red: IPv4 privada o de enlace
     * local, IPv6 única local o de enlace local (entre corchetes o sin ellos), o un nombre `.local`
     * válido. Es la misma regla que `valid_server_host` del escritorio.
     */
    fun isLocalHost(host: String): Boolean {
        val bare = host.removePrefix("[").removeSuffix("]")
        IpLiteral.parseIpv4(bare)?.let {
            return isPrivateOrLinkLocalIpv4(it)
        }
        IpLiteral.parseIpv6(bare)?.let {
            return isUniqueOrLinkLocalIpv6(it)
        }
        return isLocalName(host.lowercase())
    }

    // Separa «host:puerto», «[ipv6]:puerto», «host» o «[ipv6]». Devuelve null si el puerto no es
    // válido.
    private fun splitHostPort(authority: String): Pair<String, Int>? {
        if (authority.startsWith("[")) {
            val close = authority.indexOf(']')
            if (close < 0) {
                return null
            }
            val host = authority.substring(0, close + 1)
            val after = authority.substring(close + 1)
            return when {
                after.isEmpty() -> host to DEFAULT_PORT
                after.startsWith(":") -> parsePort(after.substring(1))?.let { host to it }
                else -> null
            }
        }
        val colon = authority.indexOf(':')
        if (colon < 0) {
            return authority to DEFAULT_PORT
        }
        // Una sola «:» fuera de corchetes: una IPv6 sin corchetes no es una dirección válida aquí.
        if (authority.indexOf(':', colon + 1) >= 0) {
            return null
        }
        val port = parsePort(authority.substring(colon + 1)) ?: return null
        return authority.substring(0, colon) to port
    }

    private fun parsePort(text: String): Int? {
        if (text.isEmpty() || text.length > 5 || !text.all { it.isAsciiDigit() }) {
            return null
        }
        val value = text.toInt()
        return if (value in 1..65535) value else null
    }

    // Forma canónica del host: la IPv4 tal cual, la IPv6 comprimida en minúsculas entre corchetes
    // y el nombre en minúsculas. null si no es ninguna de las tres.
    private fun canonicalHost(host: String): String? {
        if (host.startsWith("[")) {
            if (!host.endsWith("]")) {
                return null
            }
            val groups = IpLiteral.parseIpv6(host.substring(1, host.length - 1)) ?: return null
            return "[${IpLiteral.formatIpv6(groups)}]"
        }
        if (host.isEmpty()) {
            return null
        }
        IpLiteral.parseIpv4(host)?.let {
            return it.joinToString(".")
        }
        // Algo que parece una IPv4 pero no lo es (ceros a la izquierda, tres números, 300…) se
        // rechaza en lugar de tratarse como nombre.
        if (host.all { it.isAsciiDigit() || it == '.' }) {
            return null
        }
        val name = host.lowercase()
        return if (name.all { it.isAsciiLetterOrDigit() || it == '-' || it == '.' }) name else null
    }

    private fun isLocalName(name: String): Boolean {
        if (!name.endsWith(".local") || name.length > MAX_NAME) {
            return false
        }
        val stem = name.removeSuffix(".local")
        if (stem.isEmpty()) {
            return false
        }
        return stem.split('.').all { label ->
            label.isNotEmpty() &&
                label.length <= MAX_LABEL &&
                !label.startsWith('-') &&
                !label.endsWith('-') &&
                label.all { it.isAsciiLetterOrDigit() || it == '-' }
        }
    }

    private fun isPrivateOrLinkLocalIpv4(octets: IntArray): Boolean {
        val first = octets[0]
        val second = octets[1]
        return first == 10 ||
            (first == 172 && second in 16..31) ||
            (first == 192 && second == 168) ||
            (first == 169 && second == 254)
    }

    private fun isUniqueOrLinkLocalIpv6(groups: IntArray): Boolean {
        val first = groups[0]
        // fc00::/7 (única local) o fe80::/10 (enlace local).
        return (first and 0xfe00) == 0xfc00 || (first and 0xffc0) == 0xfe80
    }

    private fun Char.isAsciiDigit(): Boolean = this in '0'..'9'

    private fun Char.isAsciiLetterOrDigit(): Boolean =
        this in 'a'..'z' || this in 'A'..'Z' || isAsciiDigit()
}

/**
 * Lectura estricta de direcciones IP escritas como texto, sin consultar ningún DNS (a diferencia de
 * `InetAddress.getByName`, que con un nombre sí lo haría).
 */
internal object IpLiteral {
    /** Cuatro números de 0 a 255 sin ceros a la izquierda; null si no es una IPv4 así. */
    fun parseIpv4(text: String): IntArray? {
        val parts = text.split('.')
        if (parts.size != 4) {
            return null
        }
        val octets = IntArray(4)
        for ((index, part) in parts.withIndex()) {
            if (part.isEmpty() || part.length > 3 || !part.all { it in '0'..'9' }) {
                return null
            }
            if (part.length > 1 && part[0] == '0') {
                return null
            }
            val value = part.toInt()
            if (value > 255) {
                return null
            }
            octets[index] = value
        }
        return octets
    }

    /**
     * Ocho grupos de 16 bits de una IPv6 (con «::» y, al final, una IPv4 incrustada); null si no es
     * válida. Las zonas («%wlan0») no se aceptan: una URL no las admite.
     */
    fun parseIpv6(text: String): IntArray? {
        if (text.isEmpty() || text.contains('%') || !text.contains(':')) {
            return null
        }
        val doubleColon = text.indexOf("::")
        if (doubleColon >= 0 && text.indexOf("::", doubleColon + 1) >= 0) {
            return null
        }
        val head = if (doubleColon >= 0) text.substring(0, doubleColon) else text
        val tail = if (doubleColon >= 0) text.substring(doubleColon + 2) else ""
        val headGroups = parseGroups(head, allowIpv4 = doubleColon < 0) ?: return null
        val tailGroups = parseGroups(tail, allowIpv4 = true) ?: return null
        val total = headGroups.size + tailGroups.size
        if (doubleColon < 0 && total != 8) {
            return null
        }
        if (doubleColon >= 0 && total > 7) {
            return null
        }
        val groups = IntArray(8)
        headGroups.forEachIndexed { index, value -> groups[index] = value }
        tailGroups.forEachIndexed { index, value -> groups[8 - tailGroups.size + index] = value }
        return groups
    }

    // Grupos hexadecimales separados por «:»; el último puede ser una IPv4 (dos grupos).
    private fun parseGroups(text: String, allowIpv4: Boolean): List<Int>? {
        if (text.isEmpty()) {
            return emptyList()
        }
        val parts = text.split(':')
        val groups = mutableListOf<Int>()
        for ((index, part) in parts.withIndex()) {
            val last = index == parts.lastIndex
            if (last && allowIpv4 && part.contains('.')) {
                val octets = parseIpv4(part) ?: return null
                groups.add((octets[0] shl 8) or octets[1])
                groups.add((octets[2] shl 8) or octets[3])
                continue
            }
            if (part.isEmpty() || part.length > 4 || !part.all { it.isHexDigit() }) {
                return null
            }
            groups.add(part.toInt(16))
        }
        return groups
    }

    /** Forma comprimida en minúsculas (RFC 5952): la racha más larga de ceros se escribe «::». */
    fun formatIpv6(groups: IntArray): String {
        var bestStart = -1
        var bestLength = 0
        var index = 0
        while (index < 8) {
            if (groups[index] == 0) {
                val start = index
                while (index < 8 && groups[index] == 0) {
                    index += 1
                }
                if (index - start > bestLength) {
                    bestStart = start
                    bestLength = index - start
                }
            } else {
                index += 1
            }
        }
        if (bestLength < 2) {
            return groups.joinToString(":") { Integer.toHexString(it) }
        }
        val before = groups.copyOfRange(0, bestStart).joinToString(":") { Integer.toHexString(it) }
        val after =
            groups.copyOfRange(bestStart + bestLength, 8).joinToString(":") {
                Integer.toHexString(it)
            }
        return "$before::$after"
    }

    private fun Char.isHexDigit(): Boolean =
        this in '0'..'9' || this in 'a'..'f' || this in 'A'..'F'
}
