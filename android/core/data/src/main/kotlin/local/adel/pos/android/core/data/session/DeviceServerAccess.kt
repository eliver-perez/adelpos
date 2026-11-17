package local.adel.pos.android.core.data.session

import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.flow.SharingStarted
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.combine
import kotlinx.coroutines.flow.stateIn
import local.adel.pos.android.core.data.access.AccessRepository
import local.adel.pos.android.core.domain.access.ActiveWaiter
import local.adel.pos.android.core.domain.access.DeviceStatus
import local.adel.pos.android.core.domain.error.AppError
import local.adel.pos.android.core.domain.realtime.AccessInterruption
import local.adel.pos.android.core.domain.realtime.AccessInterruptions

/**
 * La interfaz estrecha de la Fase D ([ServerAccess]) sobre la entrada al dispositivo de las fases B
 * y C ([AccessRepository]): hay conexión autorizada solo con el dispositivo listo y un mesero
 * activo **con sesión de Go** (uno que entró sin conexión no la tiene; su captura la valida Go al
 * enviar). Cambia al entrar, al cambiar de mesero y al salir, y el canal de tiempo real se vuelve a
 * abrir con la sesión nueva. La dirección se pide en cada uso (cambio de IP).
 */
class DeviceServerAccess(private val access: AccessRepository, scope: CoroutineScope) :
    ServerAccess {
    override val current: StateFlow<AuthorizedServer?> =
        combine(access.status, access.activeWaiter) { status, waiter -> authorized(status, waiter) }
            .stateIn(scope, SharingStarted.Eagerly, null)

    private fun authorized(status: DeviceStatus, waiter: ActiveWaiter?): AuthorizedServer? {
        if (status !is DeviceStatus.Ready || waiter == null || !waiter.hasServerSession) {
            return null
        }
        val headers = access.requestHeaders() ?: return null
        if (headers[AuthorizedServer.AUTHORIZATION_HEADER] == null) {
            return null
        }
        val initial = access.serverTransport() ?: return null
        return AuthorizedServer(
            transport = { access.serverTransport() ?: initial },
            credentials = headers,
            userId = waiter.userId,
        )
    }

    /**
     * Avisos del catálogo y del canal de tiempo real al punto único del acceso
     * ([AccessRepository.report]): dispositivo retirado, credencial no reconocida, sesión terminada
     * (solo de ese mesero) y versión vieja. Sin `events.read` el canal se detiene hasta que cambie
     * la sesión, pero no se avisa: el mesero sigue dentro y el catálogo se actualiza igual.
     */
    val interruptions: AccessInterruptions = AccessInterruptions { interruption, userId ->
        val error =
            when (interruption) {
                AccessInterruption.DeviceRevoked -> AppError.DeviceRevoked(null, null)
                is AccessInterruption.DeviceNotRecognized ->
                    AppError.DeviceNotRecognized(interruption.code, null, null)
                is AccessInterruption.SessionEnded ->
                    AppError.SessionEnded(interruption.code, null, null)
                is AccessInterruption.UpdateRequired ->
                    AppError.UpdateRequired(interruption.code, null, null, null, null)
                is AccessInterruption.PermissionLost -> null
            }
        if (error != null) {
            access.report(error, userId)
        }
    }
}
