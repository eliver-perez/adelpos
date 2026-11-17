package local.adel.pos.android.core.data.waiter

import java.util.UUID
import local.adel.pos.android.core.data.api.ApiErrors
import local.adel.pos.android.core.data.api.ApiOutcome
import local.adel.pos.android.core.data.catalog.toLineRequest
import local.adel.pos.android.core.domain.catalog.LineSelection
import local.adel.pos.android.core.domain.error.AppError
import local.adel.pos.android.core.domain.error.AppResult
import local.adel.pos.android.core.domain.waiter.DraftQuote
import local.adel.pos.android.core.domain.waiter.SentChangeResult
import local.adel.pos.android.core.domain.waiter.SentLineService

/**
 * Modificar o cancelar unidades ya enviadas (§6.3) con la sesión del mesero que lo hace: siempre en
 * línea, con motivo y su propia clave de operación (queda registrado quién lo hizo y cocina recibe
 * su aviso, como en el escritorio). Sin supervisor (9H). Go decide los permisos, los pagos y los
 * importes; aquí solo se manda lo que eligió el mesero.
 */
class SentLineRepository(
    private val server: WaiterServer,
    private val api: WaiterApi,
    private val newId: () -> String = { UUID.randomUUID().toString() },
) : SentLineService {
    override suspend fun previewEdit(
        userId: String,
        accountId: String,
        revision: Int,
        versionId: String,
        line: LineSelection,
    ): AppResult<DraftQuote> {
        val transport = server.transport() ?: return AppResult.Failure(AppError.Unreachable)
        val headers =
            server.sessionHeaders(userId) ?: return AppResult.Failure(AppError.Unreachable)
        val outcome =
            api.previewEdit(
                transport,
                headers,
                accountId,
                EditPreviewBody(
                    revision = revision,
                    versionId = versionId,
                    line = line.toLineRequest(),
                ),
            )
        return when (outcome) {
            is ApiOutcome.Success -> AppResult.Success(WaiterMapper.quote(outcome.value))
            else -> AppResult.Failure(failure(outcome, userId))
        }
    }

    override suspend fun edit(
        userId: String,
        accountId: String,
        revision: Int,
        versionId: String,
        line: LineSelection,
        fingerprint: String,
        reason: String,
    ): SentChangeResult {
        val transport = server.transport() ?: return SentChangeResult.Failed(AppError.Unreachable)
        val headers =
            server.sessionHeaders(userId) ?: return SentChangeResult.Failed(AppError.Unreachable)
        val outcome =
            api.editUnits(
                transport,
                headers,
                accountId,
                EditUnitsBody(
                    operationId = newId(),
                    revision = revision,
                    versionId = versionId,
                    line = line.toLineRequest(),
                    expectedFingerprint = fingerprint,
                    reason = reason.trim(),
                ),
            )
        return when (outcome) {
            is ApiOutcome.Success -> SentChangeResult.Done(WaiterMapper.bill(outcome.value))
            else -> SentChangeResult.Failed(failure(outcome, userId))
        }
    }

    override suspend fun cancel(
        userId: String,
        accountId: String,
        revision: Int,
        versionId: String,
        quantity: Int,
        reason: String,
    ): SentChangeResult {
        val transport = server.transport() ?: return SentChangeResult.Failed(AppError.Unreachable)
        val headers =
            server.sessionHeaders(userId) ?: return SentChangeResult.Failed(AppError.Unreachable)
        val outcome =
            api.cancelUnits(
                transport,
                headers,
                accountId,
                CancelUnitsBody(
                    operationId = newId(),
                    revision = revision,
                    versionId = versionId,
                    quantity = quantity,
                    reason = reason.trim(),
                ),
            )
        return when (outcome) {
            is ApiOutcome.Success -> SentChangeResult.Done(WaiterMapper.bill(outcome.value))
            else -> SentChangeResult.Failed(failure(outcome, userId))
        }
    }

    private fun failure(outcome: ApiOutcome<*>, userId: String): AppError {
        val error =
            when (outcome) {
                is ApiOutcome.Rejected -> ApiErrors.toAppError(outcome.problem)
                is ApiOutcome.NoAnswer -> outcome.error
                is ApiOutcome.Malformed -> AppError.InvalidResponse(outcome.status)
                is ApiOutcome.Success -> AppError.InvalidResponse(OK)
            }
        when (error) {
            is AppError.SessionEnded,
            is AppError.DeviceRevoked,
            is AppError.DeviceNotRecognized,
            is AppError.UpdateRequired -> server.report(error, userId)
            else -> Unit
        }
        return error
    }

    private companion object {
        const val OK = 200
    }
}
