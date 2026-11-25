package com.adelpos.cocina.settings

import com.adelpos.cocina.board.DisplaySettingsService
import com.adelpos.core.model.connection.LinkState
import com.adelpos.core.model.error.AppError
import com.adelpos.core.network.api.ApiErrors
import com.adelpos.core.network.api.ApiOutcome
import com.adelpos.core.network.api.ApiProblem
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Job
import kotlinx.coroutines.delay
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.launch

/** Lo que se ve de los ajustes de la tableta (§5.4). */
sealed interface SettingsScreen {
    /** Cerrados: no hay ninguna contraseña en memoria. */
    data object Closed : SettingsScreen

    /** Pidiendo el usuario y la contraseña de un administrador. */
    data class Password(val busy: Boolean = false, val error: String? = null) : SettingsScreen

    /**
     * Abiertos: la dirección, la conexión, la versión y «Volver a emparejar» ([confirmUnpair]: la
     * pregunta de confirmación a la vista).
     */
    data class Open(
        val confirmUnpair: Boolean = false,
        val busy: Boolean = false,
        val error: String? = null,
    ) : SettingsScreen

    /**
     * Ya se volvió a emparejar (la pantalla está en «Buscar el servidor»). [message]: si Go no
     * respondió o ya no reconocía la pantalla, que hay que revocarla en Terminales.
     */
    data class Unpaired(val message: String?) : SettingsScreen
}

/** Lo que se puede hacer en los ajustes (las pruebas de pantallas lo sustituyen). */
interface SettingsActions {
    fun open()

    fun submit(username: String, password: String)

    /** Cualquier toque o tecla: los ajustes siguen abiertos otros dos minutos. */
    fun touch()

    fun askUnpair()

    fun cancelUnpair()

    fun confirmUnpair()

    fun close()
}

/** Sin ajustes (vistas previas y pruebas de una pantalla). */
object NoSettings : SettingsActions {
    override fun open() = Unit

    override fun submit(username: String, password: String) = Unit

    override fun touch() = Unit

    override fun askUnpair() = Unit

    override fun cancelUnpair() = Unit

    override fun confirmUnpair() = Unit

    override fun close() = Unit
}

/**
 * Los ajustes de la tableta (§5.4, K1-D13 y K1-D32), escondidos detrás de una pulsación larga en el
 * logotipo:
 * - piden la contraseña de un administrador y la valida Go (`settings-access`, `devices.manage`);
 *   sin conexión ni se intenta;
 * - **la contraseña vive solo en memoria** mientras están abiertos (la usa «Volver a emparejar») y
 *   se borra al cerrarlos, al volver a emparejar y si nadie los toca en [idleMillis];
 * - «Volver a emparejar» llama a `kitchen-display/unpair` (suelta el cupo y revoca esta pantalla) y
 *   después borra lo de la tableta ([forget]). Si Go no responde o ya no reconoce la pantalla, la
 *   tableta borra lo suyo de todos modos y dice que hay que revocarla en Terminales.
 *
 * @param clock reloj monotónico (milisegundos) para el cierre automático.
 * @param report un rechazo del dispositivo (revocada, otra versión…) va a la entrada, como siempre.
 */
class KitchenSettings(
    private val service: DisplaySettingsService,
    private val link: StateFlow<LinkState>,
    private val forget: suspend () -> Unit,
    private val report: (AppError) -> Unit,
    private val scope: CoroutineScope,
    private val clock: () -> Long,
    private val idleMillis: Long = IDLE_MILLIS,
) : SettingsActions {
    private class Administrator(val username: String, val password: String)

    private val mutableScreen = MutableStateFlow<SettingsScreen>(SettingsScreen.Closed)

    val screen: StateFlow<SettingsScreen> = mutableScreen.asStateFlow()

    // Solo en memoria y solo con los ajustes abiertos.
    @Volatile private var administrator: Administrator? = null
    @Volatile private var lastTouch = 0L
    private var idleJob: Job? = null

    /** Si hay una contraseña en memoria (solo para las pruebas: nunca se muestra). */
    val holdsPassword: Boolean
        get() = administrator != null

    override fun open() {
        if (mutableScreen.value != SettingsScreen.Closed) {
            return
        }
        mutableScreen.value = SettingsScreen.Password()
        watchIdle()
    }

    override fun submit(username: String, password: String) {
        touch()
        val current = mutableScreen.value as? SettingsScreen.Password ?: return
        if (current.busy) {
            return
        }
        val user = username.trim()
        if (user.isEmpty() || password.isEmpty()) {
            mutableScreen.value = SettingsScreen.Password(error = SettingsTexts.MISSING)
            return
        }
        if (link.value == LinkState.OFFLINE) {
            mutableScreen.value = SettingsScreen.Password(error = SettingsTexts.OFFLINE)
            return
        }
        mutableScreen.value = SettingsScreen.Password(busy = true)
        scope.launch {
            val outcome = service.settingsAccess(user, password)
            // Se cerraron mientras tanto (cierre automático o «Cancelar»): nada que guardar.
            if (mutableScreen.value !is SettingsScreen.Password) {
                return@launch
            }
            mutableScreen.value =
                when (outcome) {
                    is ApiOutcome.Success ->
                        if (outcome.value.authorized) {
                            administrator = Administrator(user, password)
                            SettingsScreen.Open()
                        } else {
                            SettingsScreen.Password(error = SettingsTexts.NOT_ALLOWED)
                        }
                    is ApiOutcome.Rejected -> rejectedPassword(outcome.problem)
                    is ApiOutcome.NoAnswer -> SettingsScreen.Password(error = SettingsTexts.OFFLINE)
                    is ApiOutcome.Malformed -> SettingsScreen.Password(error = SettingsTexts.FAILED)
                }
        }
    }

    private fun rejectedPassword(problem: ApiProblem): SettingsScreen {
        val error = ApiErrors.toAppError(problem)
        if (deviceProblem(error)) {
            report(error)
            close()
            return SettingsScreen.Closed
        }
        return SettingsScreen.Password(error = SettingsTexts.forProblem(problem))
    }

    override fun touch() {
        lastTouch = clock()
    }

    override fun askUnpair() {
        touch()
        val current = mutableScreen.value as? SettingsScreen.Open ?: return
        if (!current.busy) {
            mutableScreen.value = SettingsScreen.Open(confirmUnpair = true)
        }
    }

    override fun cancelUnpair() {
        touch()
        val current = mutableScreen.value as? SettingsScreen.Open ?: return
        if (!current.busy) {
            mutableScreen.value = SettingsScreen.Open()
        }
    }

    override fun confirmUnpair() {
        touch()
        val current = mutableScreen.value as? SettingsScreen.Open ?: return
        val admin = administrator
        if (current.busy || !current.confirmUnpair || admin == null) {
            return
        }
        mutableScreen.value = SettingsScreen.Open(confirmUnpair = true, busy = true)
        scope.launch {
            when (val outcome = service.unpair(admin.username, admin.password)) {
                is ApiOutcome.Success -> finishUnpair(message = null)
                is ApiOutcome.NoAnswer -> finishUnpair(SettingsTexts.UNPAIRED_NO_ANSWER)
                is ApiOutcome.Malformed -> finishUnpair(message = null)
                is ApiOutcome.Rejected -> {
                    val error = ApiErrors.toAppError(outcome.problem)
                    if (deviceProblem(error)) {
                        // Go ya no reconoce esta pantalla: se borra lo de aquí de todos modos.
                        finishUnpair(SettingsTexts.UNPAIRED_NOT_RECOGNIZED)
                    } else {
                        mutableScreen.value =
                            SettingsScreen.Open(error = SettingsTexts.forProblem(outcome.problem))
                    }
                }
            }
        }
    }

    // La pantalla deja de estar emparejada: la contraseña se olvida y se borra todo lo de aquí.
    private suspend fun finishUnpair(message: String?) {
        administrator = null
        idleJob?.cancel()
        forget()
        mutableScreen.value =
            if (message == null) SettingsScreen.Closed else SettingsScreen.Unpaired(message)
    }

    /**
     * La pantalla dejó de estar lista (revocada, emparejar de nuevo…): unos ajustes abiertos se
     * cierran y se olvida la contraseña. El aviso de «Volver a emparejar» se queda.
     */
    fun onNotReady() {
        val showing = mutableScreen.value
        if (showing is SettingsScreen.Password || showing is SettingsScreen.Open) {
            close()
        }
    }

    override fun close() {
        administrator = null
        idleJob?.cancel()
        idleJob = null
        mutableScreen.value = SettingsScreen.Closed
    }

    // Cierre automático: si nadie toca los ajustes en [idleMillis], se cierran y se borra la
    // contraseña.
    private fun watchIdle() {
        touch()
        idleJob?.cancel()
        idleJob = scope.launch {
            while (true) {
                val remaining = lastTouch + idleMillis - clock()
                if (remaining <= 0) {
                    val showing = mutableScreen.value
                    if (showing is SettingsScreen.Password || showing is SettingsScreen.Open) {
                        administrator = null
                        mutableScreen.value = SettingsScreen.Closed
                    }
                    return@launch
                }
                delay(remaining)
            }
        }
    }

    // Un rechazo del dispositivo (revocada, sin emparejar, otro tipo de cliente): no es de la
    // contraseña.
    private fun deviceProblem(error: AppError): Boolean =
        error is AppError.DeviceRevoked || error is AppError.DeviceNotRecognized

    private companion object {
        // §5.4: «un par de minutos» sin tocar nada.
        const val IDLE_MILLIS = 120_000L
    }
}

/** Los textos de los ajustes (regla de textos: claros y sin códigos). */
object SettingsTexts {
    const val MISSING = "Escribe el usuario y la contraseña."
    const val OFFLINE = "Para abrir los ajustes hace falta conexión con la computadora principal."
    const val WRONG = "El usuario o la contraseña no son correctos."
    const val NOT_ALLOWED =
        "Ese usuario no puede abrir los ajustes de esta pantalla. Pide a un administrador que " +
            "escriba su usuario y su contraseña."
    const val THROTTLED =
        "Hubo demasiados intentos con una contraseña equivocada. Espera unos minutos y vuelve a " +
            "intentarlo."
    const val FAILED = "No se pudo comprobar la contraseña. Vuelve a intentarlo."
    const val UNPAIRED_NO_ANSWER =
        "La computadora principal no respondió. Esta pantalla ya no está emparejada aquí, pero " +
            "puede seguir en Terminales ocupando su cupo de cocina: revócala ahí."
    const val UNPAIRED_NOT_RECOGNIZED =
        "La computadora principal ya no reconocía esta pantalla. Ya no está emparejada aquí; si " +
            "todavía aparece en Terminales, revócala ahí."

    /** El texto de un rechazo de Go a la contraseña (401, 403, 429 o 400). */
    fun forProblem(problem: ApiProblem): String =
        when (problem.code) {
            "AUTHORIZATION_INVALID" -> WRONG
            "PERMISSION_DENIED" -> NOT_ALLOWED
            "AUTHORIZATION_THROTTLED" -> THROTTLED
            "VALIDATION_ERROR" -> MISSING
            else -> problem.message?.takeIf { it.isNotBlank() } ?: FAILED
        }
}
