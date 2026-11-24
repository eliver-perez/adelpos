package com.adelpos.meseros.core.data.license

import com.adelpos.meseros.core.domain.license.LicenseClock
import com.adelpos.meseros.core.domain.license.LicenseStateName
import com.adelpos.meseros.core.domain.license.LicenseStatusSource
import com.adelpos.meseros.core.domain.license.LicenseSummary
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.serialization.SerialName
import kotlinx.serialization.Serializable

/*
 * Etapa L1 (Fase C del servidor): `LicenseSummary` y `LicenseClock` del contrato v1
 * (`backend/internal/apicontract/openapi.json`, `docs/API_V1.md`, «Licencia: estado, hora del
 * equipo y rechazos»), en `license` de `getIdentity` y `getRecoveryState`. Solo lo que usan los
 * avisos del dispositivo; lo demás lo ignora ApiJson.
 */

@Serializable
data class LicenseSummaryDto(
    val state: String = "",
    @SerialName("grace_days_remaining") val graceDaysRemaining: Int? = null,
    val clock: LicenseClockDto? = null,
)

@Serializable
data class LicenseClockDto(
    val blocked: Boolean = false,
    @SerialName("local_time_text") val localTimeText: String? = null,
    @SerialName("last_activity_time_text") val lastActivityTimeText: String? = null,
)

/**
 * Lo último que dijo Go de la licencia y de la hora de la computadora principal, para los avisos de
 * la barra (etapa L1, §8). Solo en memoria: al abrir la aplicación no se sabe hasta la primera
 * lectura con conexión (un aviso viejo de otro momento confundiría más que ayudar). Lo alimentan
 * las lecturas de `restaurant/state` (`RestaurantRepository`, que vuelve a leer con el evento
 * `license.changed`) y de `auth/me` (`PrecheckRepository`). Un servidor anterior a la etapa no
 * envía `license`: sin aviso.
 */
class LicenseStatusRepository : LicenseStatusSource {
    private val mutableSummary = MutableStateFlow<LicenseSummary?>(null)

    override val summary: StateFlow<LicenseSummary?> = mutableSummary.asStateFlow()

    /** Lo que trajo una lectura de Go (null: el servidor no lo envía). */
    fun apply(dto: LicenseSummaryDto?) {
        mutableSummary.value = dto?.let(::toSummary)
    }

    /** Al borrar los datos de este dispositivo (retiro o «Emparejar de nuevo»). */
    fun forget() {
        mutableSummary.value = null
    }

    private fun toSummary(dto: LicenseSummaryDto): LicenseSummary =
        LicenseSummary(
            state = LicenseStateName.fromWireName(dto.state),
            graceDaysRemaining = dto.graceDaysRemaining?.takeIf { it >= 0 },
            clock =
                dto.clock?.let { clock ->
                    LicenseClock(
                        blocked = clock.blocked,
                        localTimeText = clock.localTimeText,
                        lastActivityText = clock.lastActivityTimeText,
                    )
                } ?: LicenseClock.NOT_BLOCKED,
        )
}
