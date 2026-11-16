package local.adel.pos.android.core.domain.catalog

/**
 * Indicaciones para cocina de una partida. Las rápidas de la categoría y las observaciones que
 * escribe el mesero (con el teclado del sistema) se guardan en el mismo campo `notes`, separadas
 * por comas, como en el escritorio (`captureRules.ts`, `KitchenNotes.tsx`); llegan a la comanda
 * igual y no tienen cargo.
 */
object KitchenNotes {
    /**
     * Límite de Go para las observaciones de una partida: `len(customization.Notes) > 500`
     * (`catalog/calculator.go`, `operations/drafts.go`) cuenta **bytes** en UTF-8, no letras; con
     * acentos caben menos de 500 letras.
     */
    const val MAXIMUM_BYTES = 500

    private const val SEPARATOR = ", "

    /** Las indicaciones del campo, sin espacios a los lados y sin vacías. */
    fun split(notes: String): List<String> =
        notes.split(',').map { it.trim() }.filter { it.isNotEmpty() }

    fun join(entries: List<String>): String = entries.joinToString(SEPARATOR)

    /** Si el texto completo de las observaciones cabe en el límite de Go. */
    fun fits(notes: String): Boolean = notes.toByteArray(Charsets.UTF_8).size <= MAXIMUM_BYTES

    /**
     * Si agregar [entry] a las observaciones de [value] cabe (ya presente cuenta como que cabe).
     */
    fun canAdd(value: Customization, entry: String): Boolean {
        val current = split(value.notes)
        val trimmed = entry.trim()
        if (trimmed in current) {
            return true
        }
        return fits(join(current + trimmed))
    }

    /**
     * Agrega una observación escrita por el mesero (sin repetirla). Devuelve null si está vacía o
     * no cabe: la pantalla avisa en lugar de recortar lo escrito.
     */
    fun withObservation(value: Customization, text: String): Customization? {
        val trimmed = text.trim()
        if (trimmed.isEmpty()) {
            return null
        }
        val current = split(value.notes)
        if (trimmed in current) {
            return value
        }
        val notes = join(current + trimmed)
        if (!fits(notes)) {
            return null
        }
        return value.copy(notes = notes)
    }

    /**
     * Quita un texto de las observaciones y cualquier indicación rápida elegida con ese texto, sin
     * tocar ingredientes.
     */
    fun withoutText(value: Customization, text: String): Customization =
        QuickNoteChoices.withoutEntry(value, noteId = "", text = text)
}
