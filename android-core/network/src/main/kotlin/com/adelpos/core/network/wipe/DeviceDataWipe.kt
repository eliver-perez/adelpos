package com.adelpos.core.network.wipe

import java.util.concurrent.CopyOnWriteArrayList
import kotlinx.coroutines.CancellationException
import kotlinx.coroutines.CoroutineDispatcher
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.withContext

/**
 * Una parte de los datos que este dispositivo guarda del restaurante y que se borra al retirarlo o
 * al emparejarlo de nuevo (por ejemplo, el catálogo y sus imágenes).
 */
fun interface DeviceDataPart {
    suspend fun erase()
}

/**
 * Registro único de «borrar los datos de este dispositivo» (§3.2: el dispositivo retirado borra sus
 * datos locales; revisión R3, M4). Cada módulo que guarda datos del restaurante **se registra
 * aquí** al crearse, y la entrada al dispositivo lo usa al retirarlo (`DEVICE_REVOKED`) y en
 * «Emparejar de nuevo», después de pasar los pedidos sin enviar a la bitácora y antes de olvidar el
 * emparejamiento y destruir la llave del Keystore.
 *
 * Registrados hoy: el catálogo (`catalog.db`) con su caché de imágenes (Fase D). **La Fase E debe
 * registrar** la cola de pedidos y sus borradores (y cancelar su trabajo de WorkManager con
 * `WorkManager.cancelAllWork()`), después de que `UnsentOrdersSource` haya entregado lo que no se
 * envió.
 */
class DeviceDataWipe(private val dispatcher: CoroutineDispatcher = Dispatchers.IO) {
    private class Registered(val name: String, val part: DeviceDataPart)

    private val parts = CopyOnWriteArrayList<Registered>()

    /** Agrega una parte con un nombre para el registro (catálogo, cola…). */
    fun register(name: String, part: DeviceDataPart) {
        parts.add(Registered(name, part))
    }

    /** Nombres de las partes registradas, en el orden en que se borran. */
    val registered: List<String>
        get() = parts.map { it.name }

    /**
     * Borra todas las partes, en orden y fuera del hilo principal ([dispatcher]). Una que falla no
     * detiene a las demás (se borra todo lo que se pueda); devuelve los nombres de las que
     * fallaron.
     */
    suspend fun eraseAll(): List<String> =
        withContext(dispatcher) {
            val failed = mutableListOf<String>()
            for (registered in parts) {
                try {
                    registered.part.erase()
                } catch (cancelled: CancellationException) {
                    throw cancelled
                } catch (_: Exception) {
                    failed.add(registered.name)
                }
            }
            failed
        }
}
