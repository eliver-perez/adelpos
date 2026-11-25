package com.adelpos.cocina.alerts

import com.adelpos.cocina.board.AlertSound
import kotlin.math.PI
import kotlin.math.min
import kotlin.math.roundToInt
import kotlin.math.sin

/**
 * Los sonidos de la pantalla de cocina, **generados aquí** (§5.2): ningún archivo ni biblioteca de
 * otros. Son notas puras con un poco de su armónico, cada una con subida y bajada suaves (sin
 * chasquidos), en PCM de 16 bits mono a [SAMPLE_RATE].
 * - **Pedido nuevo**: tres notas que suben (sol, si, mi), claras y cortas.
 * - **Cancelación o cambio**: dos pares de notas que bajan, más graves y largas: se distingue sin
 *   mirar la pantalla.
 */
object Tones {
    const val SAMPLE_RATE = 44_100

    /** Una nota: frecuencia (Hz) y duración (ms); frecuencia 0 es un silencio. */
    data class Note(val hertz: Double, val millis: Int)

    private val NEW_ORDER =
        listOf(
            Note(784.0, 140),
            Note(0.0, 40),
            Note(988.0, 140),
            Note(0.0, 40),
            Note(1319.0, 260),
        )

    private val CHANGE =
        listOf(
            Note(660.0, 200),
            Note(0.0, 50),
            Note(440.0, 260),
            Note(0.0, 160),
            Note(660.0, 200),
            Note(0.0, 50),
            Note(440.0, 260),
        )

    fun notes(sound: AlertSound): List<Note> =
        when (sound) {
            AlertSound.NEW_ORDER -> NEW_ORDER
            AlertSound.CHANGE -> CHANGE
        }

    /** Las muestras de [sound] a volumen completo (el volumen elegido se aplica al reproducir). */
    fun pcm(sound: AlertSound): ShortArray {
        val notes = notes(sound)
        val total = notes.sumOf { samplesFor(it.millis) }
        val samples = ShortArray(total)
        var offset = 0
        for (note in notes) {
            val count = samplesFor(note.millis)
            if (note.hertz > 0) {
                writeNote(samples, offset, count, note.hertz)
            }
            offset += count
        }
        return samples
    }

    private fun samplesFor(millis: Int): Int = SAMPLE_RATE * millis / MILLIS_PER_SECOND

    private fun writeNote(samples: ShortArray, offset: Int, count: Int, hertz: Double) {
        val fade = min(samplesFor(FADE_MILLIS), count / 2)
        for (index in 0 until count) {
            val time = index.toDouble() / SAMPLE_RATE
            val wave =
                FUNDAMENTAL * sin(2 * PI * hertz * time) + HARMONIC * sin(4 * PI * hertz * time)
            // Subida y bajada lineales: la onda empieza y termina en cero.
            val envelope =
                when {
                    index < fade -> index.toDouble() / fade
                    index >= count - fade -> (count - 1 - index).toDouble() / fade
                    else -> 1.0
                }
            samples[offset + index] = (wave * envelope * PEAK).roundToInt().toShort()
        }
    }

    private const val MILLIS_PER_SECOND = 1000
    private const val FADE_MILLIS = 12

    // La nota con un poco de su octava: suena más clara en la bocina chica de una tableta.
    private const val FUNDAMENTAL = 0.8
    private const val HARMONIC = 0.2

    // Un poco por debajo del máximo de 16 bits: nunca se satura.
    private const val PEAK = 30_000.0
}
