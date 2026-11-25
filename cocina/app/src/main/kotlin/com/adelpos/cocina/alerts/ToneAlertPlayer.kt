package com.adelpos.cocina.alerts

import android.media.AudioAttributes
import android.media.AudioFormat
import android.media.AudioTrack
import android.util.Log
import com.adelpos.cocina.board.AlertSound
import com.adelpos.cocina.board.BoardAlertSink
import java.util.concurrent.ExecutorService
import java.util.concurrent.Executors

/**
 * Hace sonar las alertas de la pantalla de cocina (§5.2) con los tonos de [Tones], uno detrás de
 * otro en un hilo propio (nunca en el de la pantalla).
 *
 * **Canal de audio**: uso «notificación» (`USAGE_NOTIFICATION_EVENT`), no multimedia. Suena con el
 * volumen de notificaciones de la tableta aunque el de multimedia esté bajo, y respeta «No
 * molestar» como cualquier aviso (no se salta lo que eligió el dueño). La alarma sí se lo saltaría
 * y sonaría también en la noche: no hace falta. Lo explica `docs/COCINA_INSTALAR.md`.
 */
class ToneAlertPlayer : BoardAlertSink {
    private val executor: ExecutorService = Executors.newSingleThreadExecutor()
    private val samples = HashMap<AlertSound, ShortArray>()
    private val history = ArrayDeque<String>()

    /** Los últimos sonidos (diagnóstico y pruebas contra Go): «NEW_ORDER medium»… */
    val played: List<String>
        get() = synchronized(history) { history.toList() }

    override fun play(sounds: Set<AlertSound>, volume: String) {
        val ordered = AlertSound.entries.filter { it in sounds }
        if (ordered.isEmpty()) {
            return
        }
        val gain = SoundVolumes.gain(volume)
        executor.execute {
            for (sound in ordered) {
                remember("${sound.name} $volume")
                playBlocking(sound, gain)
            }
        }
    }

    private fun remember(entry: String) {
        Log.i(TAG, "sonido: $entry")
        synchronized(history) {
            history.addLast(entry)
            while (history.size > HISTORY) {
                history.removeFirst()
            }
        }
    }

    private fun playBlocking(sound: AlertSound, gain: Float) {
        val pcm = samples.getOrPut(sound) { Tones.pcm(sound) }
        val track =
            try {
                AudioTrack.Builder()
                    .setAudioAttributes(
                        AudioAttributes.Builder()
                            .setUsage(AudioAttributes.USAGE_NOTIFICATION_EVENT)
                            .setContentType(AudioAttributes.CONTENT_TYPE_SONIFICATION)
                            .build(),
                    )
                    .setAudioFormat(
                        AudioFormat.Builder()
                            .setSampleRate(Tones.SAMPLE_RATE)
                            .setEncoding(AudioFormat.ENCODING_PCM_16BIT)
                            .setChannelMask(AudioFormat.CHANNEL_OUT_MONO)
                            .build(),
                    )
                    .setBufferSizeInBytes(pcm.size * BYTES_PER_SAMPLE)
                    .setTransferMode(AudioTrack.MODE_STATIC)
                    .build()
            } catch (unavailable: UnsupportedOperationException) {
                // Sin salida de audio (raro en una tableta): la animación sigue avisando.
                Log.w(TAG, "sin audio: ${unavailable.message}")
                return
            }
        try {
            track.write(pcm, 0, pcm.size)
            track.setVolume(gain)
            track.play()
            Thread.sleep(pcm.size * MILLIS_PER_SECOND / Tones.SAMPLE_RATE + TAIL_MILLIS)
        } catch (failed: IllegalStateException) {
            Log.w(TAG, "no se pudo reproducir: ${failed.message}")
        } catch (_: InterruptedException) {
            Thread.currentThread().interrupt()
        } finally {
            track.release()
        }
    }

    private companion object {
        const val TAG = "CocinaAlertas"
        const val HISTORY = 20
        const val BYTES_PER_SAMPLE = 2
        const val MILLIS_PER_SECOND = 1000L
        const val TAIL_MILLIS = 60L
    }
}

/** «Volumen» de Terminales (bajo, medio o alto, K1-D15) sobre el volumen de la tableta. */
object SoundVolumes {
    fun gain(volume: String): Float =
        when (volume) {
            "low" -> LOW
            "high" -> HIGH
            else -> MEDIUM
        }

    const val LOW = 0.35f
    const val MEDIUM = 0.65f
    const val HIGH = 1.0f
}
