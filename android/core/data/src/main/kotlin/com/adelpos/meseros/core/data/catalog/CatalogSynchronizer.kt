package com.adelpos.meseros.core.data.catalog

import android.database.SQLException
import com.adelpos.core.model.error.AppError
import com.adelpos.core.model.error.AppResult
import com.adelpos.core.model.realtime.AccessInterruptions
import com.adelpos.core.network.realtime.AccessInterruptionMapping
import com.adelpos.core.network.session.ServerAccess
import kotlinx.coroutines.CoroutineDispatcher
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.sync.Mutex
import kotlinx.coroutines.sync.withLock
import kotlinx.coroutines.withContext
import kotlinx.serialization.SerializationException

/** Cómo terminó una descarga del catálogo. */
sealed interface CatalogSyncOutcome {
    /** Sin servidor emparejado o sin mesero con sesión: no se descargó nada. */
    data object NoSession : CatalogSyncOutcome

    /**
     * Se guardó la descarga.
     *
     * @property full se reemplazó el catálogo completo (primera vez, `since` 0 o [reset]).
     * @property reset Go entregó todo porque la época o la versión guardadas ya no existían.
     * @property records registros que traía (0 = sin cambios).
     */
    data class Saved(
        val full: Boolean,
        val reset: Boolean,
        val version: Long,
        val records: Int,
    ) : CatalogSyncOutcome

    data class Failed(val error: AppError) : CatalogSyncOutcome
}

/**
 * Descarga del catálogo (§5 de la especificación): completo la primera vez; después **solo los
 * cambios** desde la versión y la época guardadas (`since` + `epoch`); con `reset: true` (época
 * distinta o versión que el servidor no conoce) reemplaza todo en una transacción. Una sola
 * descarga a la vez ([Mutex]); quien agenda las repeticiones es [CatalogRepository].
 *
 * Los precios que se guardan son los que calculó Go (referencia); la aplicación no calcula nada.
 */
class CatalogSynchronizer(
    private val access: ServerAccess,
    private val api: CatalogApi,
    private val store: CatalogStore,
    private val interruptions: AccessInterruptions,
    private val clock: () -> Long = System::currentTimeMillis,
    private val parseDispatcher: CoroutineDispatcher = Dispatchers.Default,
) {
    private val mutex = Mutex()

    suspend fun synchronize(): CatalogSyncOutcome = mutex.withLock {
        val server = access.current.value ?: return@withLock CatalogSyncOutcome.NoSession
        val saved = store.state()
        // Sin catálogo guardado (o con la versión 0 de un catálogo vacío) se pide todo.
        val since = saved?.version?.takeIf { it > 0 }
        val epoch = saved?.epoch?.takeIf { since != null }
        when (val result = api.sync(server, since, epoch)) {
            is AppResult.Failure -> failed(result.error, server.userId)
            is AppResult.Success -> save(result.value)
        }
    }

    private suspend fun save(body: String): CatalogSyncOutcome {
        val payload =
            try {
                withContext(parseDispatcher) { CatalogPayload.parse(body, clock()) }
            } catch (_: SerializationException) {
                return CatalogSyncOutcome.Failed(AppError.InvalidResponse(HTTP_OK))
            } catch (_: IllegalArgumentException) {
                return CatalogSyncOutcome.Failed(AppError.InvalidResponse(HTTP_OK))
            }
        try {
            store.save(payload)
        } catch (_: SQLException) {
            // Por ejemplo, un identificador repetido en la descarga completa: la transacción se
            // deshizo y el catálogo anterior sigue intacto.
            return CatalogSyncOutcome.Failed(AppError.InvalidResponse(HTTP_OK))
        }
        return CatalogSyncOutcome.Saved(
            full = payload.full,
            reset = payload.reset,
            version = payload.state.version,
            records = payload.recordCount,
        )
    }

    private fun failed(error: AppError, userId: String?): CatalogSyncOutcome {
        // El dispositivo retirado, la sesión terminada o la versión vieja se avisan al punto único
        // del acceso; sin permiso de catálogo solo se informa en el estado de la descarga.
        AccessInterruptionMapping.fromAppError(error)?.let { interruption ->
            interruptions.onInterrupted(interruption, userId)
        }
        return CatalogSyncOutcome.Failed(error)
    }

    private companion object {
        const val HTTP_OK = 200
    }
}
