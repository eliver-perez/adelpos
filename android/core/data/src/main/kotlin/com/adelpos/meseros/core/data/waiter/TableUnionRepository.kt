package com.adelpos.meseros.core.data.waiter

import com.adelpos.core.model.error.AppError
import com.adelpos.core.network.api.ApiErrors
import com.adelpos.core.network.api.ApiOutcome
import com.adelpos.meseros.core.domain.waiter.TableUnion
import com.adelpos.meseros.core.domain.waiter.TableUnionService
import com.adelpos.meseros.core.domain.waiter.UnionAnswer
import com.adelpos.meseros.core.domain.waiter.UnionPreview
import com.adelpos.meseros.core.domain.waiter.UnionProblem
import java.util.UUID
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.delay
import kotlinx.coroutines.flow.SharingStarted
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.map
import kotlinx.coroutines.flow.stateIn

/**
 * Etapa M1 (§1, contrato §3): unir mesas, agregar y quitar mesas de una unión, con **la sesión del
 * mesero activo y solo con conexión** (M1-D13: una unión nunca entra en la cola). Go decide todo y
 * sus rechazos se muestran tal cual (`message`). Cada escritura lleva su `operation_id`; sin
 * respuesta se repite **una vez con la misma clave** (Go no une dos veces). Después de una
 * escritura se vuelve a leer Mesas (el canal también avisa).
 */
class TableUnionRepository(
    private val server: WaiterServer,
    private val api: WaiterApi,
    private val restaurant: RestaurantRepository,
    scope: CoroutineScope,
    private val newId: () -> String = { UUID.randomUUID().toString() },
    private val retryDelayMillis: Long = RETRY_DELAY_MILLIS,
) : TableUnionService {
    override val available: StateFlow<Boolean> =
        server.capabilities
            .map { CAPABILITY in it }
            .stateIn(scope, SharingStarted.Eagerly, CAPABILITY in server.capabilities.value)

    override suspend fun preview(tableIds: List<String>): UnionAnswer =
        previewWith(UnionPreviewBody(tableIds = tableIds))

    override suspend fun previewGroup(groupId: String): UnionAnswer =
        previewWith(UnionPreviewBody(groupId = groupId))

    override suspend fun previewAdd(unionId: String, tableIds: List<String>): UnionAnswer =
        previewWith(UnionPreviewBody(unionId = unionId, tableIds = tableIds))

    override suspend fun create(
        startTableId: String,
        tableIds: List<String>,
        groupId: String?,
    ): UnionAnswer {
        val body =
            CreateUnionBody(
                operationId = newId(),
                startTableId = startTableId,
                tableIds = tableIds.takeIf { groupId == null },
                groupId = groupId,
            )
        return write { transport, headers -> api.createUnion(transport, headers, body) }
    }

    override suspend fun addTable(union: TableUnion, tableId: String): UnionAnswer {
        val body =
            AddUnionTableBody(operationId = newId(), revision = union.revision, tableId = tableId)
        return write { transport, headers -> api.addUnionTable(transport, headers, union.id, body) }
    }

    override suspend fun removeTable(union: TableUnion, tableId: String): UnionAnswer {
        val body = MutationBody(operationId = newId(), revision = union.revision)
        return write { transport, headers ->
            api.removeUnionTable(transport, headers, union.id, tableId, body)
        }
    }

    private suspend fun previewWith(body: UnionPreviewBody): UnionAnswer {
        val userId = server.activeWaiter.value?.userId ?: return UnionAnswer.NoConnection
        val transport = server.transport() ?: return UnionAnswer.NoConnection
        val headers = server.sessionHeaders(userId) ?: return UnionAnswer.NoConnection
        var outcome = api.previewUnion(transport, headers, body)
        if (outcome is ApiOutcome.NoAnswer) {
            // La vista previa no escribe: se repite una vez como cualquier lectura.
            delay(retryDelayMillis)
            outcome = api.previewUnion(transport, headers, body)
        }
        return when (outcome) {
            is ApiOutcome.Success ->
                UnionAnswer.Previewed(
                    UnionPreview(
                        name = outcome.value.name,
                        seats = outcome.value.seats,
                        tableIds = outcome.value.tableIds,
                        problems =
                            outcome.value.problems.map {
                                UnionProblem(it.code, it.message, it.tableId)
                            },
                    ),
                )
            is ApiOutcome.Rejected -> rejected(outcome, userId)
            is ApiOutcome.NoAnswer -> UnionAnswer.NoConnection
            is ApiOutcome.Malformed -> UnionAnswer.Rejected("")
        }
    }

    /**
     * Una escritura con la sesión del mesero activo; sin respuesta (habiendo salido) se repite
     * **igual**, con la misma clave.
     */
    private suspend fun write(
        call:
            suspend (
                com.adelpos.core.network.session.ServerTransport,
                okhttp3.Headers,
            ) -> ApiOutcome<UnionAnswerDto>,
    ): UnionAnswer {
        val userId = server.activeWaiter.value?.userId ?: return UnionAnswer.NoConnection
        val transport = server.transport() ?: return UnionAnswer.NoConnection
        val headers = server.sessionHeaders(userId) ?: return UnionAnswer.NoConnection
        var outcome = call(transport, headers)
        if (outcome is ApiOutcome.NoAnswer && outcome.requestSent) {
            delay(retryDelayMillis)
            outcome = call(transport, headers)
        }
        val answer =
            when (outcome) {
                is ApiOutcome.Success ->
                    UnionAnswer.Saved(
                        union = WaiterMapper.union(outcome.value.union),
                        dissolved = outcome.value.dissolved,
                    )
                is ApiOutcome.Rejected ->
                    if (outcome.problem.code == DEFERRED) {
                        UnionAnswer.Deferred
                    } else {
                        rejected(outcome, userId)
                    }
                is ApiOutcome.NoAnswer -> UnionAnswer.NoConnection
                is ApiOutcome.Malformed -> UnionAnswer.Rejected("")
            }
        // Mesas se vuelve a leer en cualquier caso (otra terminal pudo cambiar las mesas).
        restaurant.refresh()
        return answer
    }

    private fun rejected(outcome: ApiOutcome.Rejected, userId: String): UnionAnswer {
        val error = ApiErrors.toAppError(outcome.problem)
        when (error) {
            is AppError.SessionEnded,
            is AppError.DeviceRevoked,
            is AppError.DeviceNotRecognized,
            is AppError.UpdateRequired,
            is AppError.ServerIdentityChanged -> server.report(error, userId)
            else -> Unit
        }
        return UnionAnswer.Rejected(ApiErrors.shownMessage(outcome.problem) ?: "")
    }

    companion object {
        /** La capacidad de Go que dice que hay uniones de mesas (contrato §1). */
        const val CAPABILITY = "table-unions-v1"

        const val DEFERRED = "TABLE_UNION_DEFERRED"
        private const val RETRY_DELAY_MILLIS = 1_500L
    }
}
