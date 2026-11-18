package local.adel.pos.android.core.data.realtime

import java.io.IOException
import kotlinx.coroutines.CoroutineDispatcher
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.withContext
import kotlinx.serialization.SerializationException
import kotlinx.serialization.json.Json
import local.adel.pos.android.core.data.api.ApiJson
import local.adel.pos.android.core.data.session.AuthorizedServer
import local.adel.pos.android.core.data.session.apiUrl
import local.adel.pos.android.core.data.session.execute
import local.adel.pos.android.core.data.session.request

/**
 * Semilla del canal: el `event_cursor` de `GET restaurant/state` (API v1, «Sesión y recuperación
 * del estado», punto 5). Go lo lee antes que mesas y cuentas con el lock de la operación tomado,
 * así que abrir el canal desde ahí no pierde nada. Con un servidor sin `event_cursor` o sin
 * respuesta devuelve null y el canal se abre sin cursor (el primer mensaje igual avisa de
 * reconstruir).
 *
 * Es la semilla por omisión; la Fase E puede sustituirla por la de su propia lectura del estado
 * para no leerlo dos veces.
 */
class RecoveryStateCursorSeed(
    private val json: Json = ApiJson,
    private val ioDispatcher: CoroutineDispatcher = Dispatchers.IO,
) : RealtimeCursorSeed {
    override suspend fun eventCursor(server: AuthorizedServer): Long? {
        val request = server.request(server.apiUrl("restaurant/state")).get().build()
        val response =
            try {
                server.execute(request)
            } catch (_: IOException) {
                return null
            }
        return withContext(ioDispatcher) {
            response.use { answer ->
                if (!answer.isSuccessful) {
                    return@withContext null
                }
                try {
                    json
                        .decodeFromString(RecoveryCursorDto.serializer(), answer.body.string())
                        .eventCursor
                } catch (_: IOException) {
                    null
                } catch (_: SerializationException) {
                    null
                } catch (_: IllegalArgumentException) {
                    null
                }
            }
        }
    }
}
