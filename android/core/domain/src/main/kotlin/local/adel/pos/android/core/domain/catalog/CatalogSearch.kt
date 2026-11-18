package local.adel.pos.android.core.domain.catalog

import java.text.Normalizer
import java.util.Locale

/**
 * Búsqueda de productos sobre el catálogo guardado, sin distinguir acentos ni mayúsculas: «cafe»
 * encuentra «Café», «JAMON» encuentra «Jamón» y «pina» encuentra «Piña». No necesita el servidor.
 * La base local guarda cada producto con su [searchKey] ya normalizada y la consulta usa
 * [normalize] para el texto escrito.
 */
object CatalogSearch {
    private val combiningMarks = Regex("\\p{Mn}+")
    private val spaces = Regex("\\s+")

    /** Minúsculas, sin acentos ni diéresis y con un solo espacio entre palabras. */
    fun normalize(text: String): String {
        val decomposed = Normalizer.normalize(text, Normalizer.Form.NFD)
        return decomposed
            .replace(combiningMarks, "")
            .lowercase(Locale.ROOT)
            .replace(spaces, " ")
            .trim()
    }

    /** Palabras de la búsqueda, ya normalizadas; vacía si no se escribió nada. */
    fun tokens(query: String): List<String> {
        val normalized = normalize(query)
        if (normalized.isEmpty()) {
            return emptyList()
        }
        return normalized.split(' ')
    }

    /** Texto donde se busca: nombre, clave y código de barras del producto. */
    fun searchKey(name: String, code: String, barcode: String?): String =
        normalize(listOfNotNull(name, code, barcode).joinToString(" "))

    /**
     * Si un producto con esta clave coincide: cada palabra escrita debe aparecer (en cualquier
     * orden), como parte de una palabra o completa.
     */
    fun matches(searchKey: String, tokens: List<String>): Boolean = tokens.all { token ->
        searchKey.contains(token)
    }
}

/**
 * Qué productos mostrar: los de una categoría ([categoryId]) o, con texto, los que coinciden en
 * todo el catálogo (la búsqueda ignora la categoría elegida, como en el escritorio, para que el
 * mesero encuentre el producto sin saber dónde está).
 */
data class CatalogQuery(val text: String = "", val categoryId: String? = null) {
    val searching: Boolean
        get() = CatalogSearch.tokens(text).isNotEmpty()
}
