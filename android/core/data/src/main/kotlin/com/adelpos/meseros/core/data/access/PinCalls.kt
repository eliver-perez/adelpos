package com.adelpos.meseros.core.data.access

import com.adelpos.core.network.api.ApiOutcome
import com.adelpos.core.network.pairing.PairedConnection
import com.adelpos.meseros.core.data.api.PinLoginRequest
import com.adelpos.meseros.core.data.api.PinLoginResponse
import com.adelpos.meseros.core.data.api.PinUsersResponse

// La entrada con PIN por la conexión con el servidor emparejado: las mismas llamadas que tenía
// PairedConnection antes de pasar a android-core (etapa K1), con su búsqueda en otra dirección.

internal suspend fun PairedConnection.pinUsers(): ApiOutcome<PinUsersResponse> = call { api ->
    api.get("auth/pin-users", PinUsersResponse.serializer(), credentials.headers())
}

internal suspend fun PairedConnection.pinLogin(
    userId: String,
    pin: String,
): ApiOutcome<PinLoginResponse> = call { api ->
    api.post(
        "auth/pin-login",
        PinLoginRequest(userId, pin),
        PinLoginRequest.serializer(),
        PinLoginResponse.serializer(),
        credentials.headers(),
    )
}
