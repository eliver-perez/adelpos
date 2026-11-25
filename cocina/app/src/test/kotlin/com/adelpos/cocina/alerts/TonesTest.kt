package com.adelpos.cocina.alerts

import com.adelpos.cocina.board.AlertSound
import kotlin.math.abs
import org.junit.Assert.assertEquals
import org.junit.Assert.assertNotEquals
import org.junit.Assert.assertTrue
import org.junit.Test

/**
 * Los sonidos generados (§5.2): dos distintos, sin saturar y sin chasquidos al empezar o terminar
 * cada nota, y el volumen de Terminales (bajo, medio, alto) sobre el de la tableta.
 */
class TonesTest {
    @Test
    fun losDosSonidosSonDistintosYCortos() {
        val newOrder = Tones.pcm(AlertSound.NEW_ORDER)
        val change = Tones.pcm(AlertSound.CHANGE)
        assertNotEquals(newOrder.size, change.size)
        for (samples in listOf(newOrder, change)) {
            val seconds = samples.size.toDouble() / Tones.SAMPLE_RATE
            assertTrue("dura menos de 2 s: $seconds", seconds in 0.3..2.0)
        }
        // El pedido nuevo sube; el cambio baja y es más grave.
        val rising = Tones.notes(AlertSound.NEW_ORDER).filter { it.hertz > 0 }.map { it.hertz }
        assertEquals(rising.sorted(), rising)
        val falling = Tones.notes(AlertSound.CHANGE).filter { it.hertz > 0 }.map { it.hertz }
        assertTrue(falling.first() > falling[1])
        assertTrue(falling.max() < rising.max())
    }

    @Test
    fun nuncaSaturaYCadaNotaEmpiezaYTerminaEnSilencio() {
        for (sound in AlertSound.entries) {
            val samples = Tones.pcm(sound)
            assertTrue(samples.all { abs(it.toInt()) < Short.MAX_VALUE })
            assertTrue("se oye", samples.any { abs(it.toInt()) > 20_000 })
            assertTrue("empieza en silencio", abs(samples.first().toInt()) < 100)
            assertTrue("termina en silencio", abs(samples.last().toInt()) < 100)
            // En el límite entre dos notas (o una nota y un silencio) tampoco hay un salto.
            var offset = 0
            for (note in Tones.notes(sound).dropLast(1)) {
                offset += Tones.SAMPLE_RATE * note.millis / 1000
                assertTrue(abs(samples[offset - 1].toInt()) < 1_000)
            }
        }
    }

    @Test
    fun elVolumenDeTerminales() {
        assertEquals(SoundVolumes.LOW, SoundVolumes.gain("low"))
        assertEquals(SoundVolumes.MEDIUM, SoundVolumes.gain("medium"))
        assertEquals(SoundVolumes.HIGH, SoundVolumes.gain("high"))
        // Uno que esta versión no conoce suena en medio.
        assertEquals(SoundVolumes.MEDIUM, SoundVolumes.gain("muy alto"))
        assertTrue(
            SoundVolumes.LOW < SoundVolumes.MEDIUM && SoundVolumes.MEDIUM < SoundVolumes.HIGH,
        )
    }
}
