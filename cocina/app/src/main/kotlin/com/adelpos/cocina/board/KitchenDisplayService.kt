package com.adelpos.cocina.board

import com.adelpos.cocina.access.KitchenAccess
import com.adelpos.core.model.error.AppError
import com.adelpos.core.network.api.ApiOutcome
import com.adelpos.core.network.api.IgnoredBody
import com.adelpos.core.network.api.ServerApi
import kotlinx.serialization.DeserializationStrategy
import kotlinx.serialization.SerializationStrategy
import okhttp3.Headers

/**
 * Las rutas de la pantalla de cocina (`kitchen-display/…`), cada una con su resultado completo
 * (valor, rechazo con su código o falta de respuesta). La pantalla de pedidos ([KitchenBoard])
 * habla con Go solo por aquí; las pruebas lo sustituyen.
 */
interface KitchenDisplayService {
    suspend fun connect(takeSeat: Boolean): ApiOutcome<ConnectResponseDto>

    suspend fun orders(): ApiOutcome<OrdersResponseDto>

    suspend fun markItems(tasks: List<TaskRef>): ApiOutcome<MarkResultDto>

    suspend fun markOrder(tasks: List<TaskRef>): ApiOutcome<MarkResultDto>

    suspend fun recent(): ApiOutcome<RecentResponseDto>

    suspend fun recover(tasks: List<TaskRef>): ApiOutcome<RecoverResultDto>

    suspend fun receipts(receipts: List<ReceiptDto>): ApiOutcome<IgnoredBody>
}

/**
 * Las rutas de la pantalla de cocina por la conexión del servidor emparejado ([access]), con la
 * credencial del dispositivo y sin sesión (K1-D4).
 */
class HttpKitchenDisplayService(private val access: KitchenAccess) : KitchenDisplayService {
    override suspend fun connect(takeSeat: Boolean): ApiOutcome<ConnectResponseDto> =
        post(
            "kitchen-display/connect",
            // `{}` al arrancar o reconectar; `take_seat` solo con «Usar esta pantalla».
            ConnectRequest(takeSeat.takeIf { it }),
            ConnectRequest.serializer(),
            ConnectResponseDto.serializer(),
        )

    override suspend fun orders(): ApiOutcome<OrdersResponseDto> =
        get("kitchen-display/orders", OrdersResponseDto.serializer())

    override suspend fun markItems(tasks: List<TaskRef>): ApiOutcome<MarkResultDto> =
        post(
            "kitchen-display/items/ready",
            TasksRequest(tasks.map(::wire)),
            TasksRequest.serializer(),
            MarkResultDto.serializer(),
        )

    override suspend fun markOrder(tasks: List<TaskRef>): ApiOutcome<MarkResultDto> =
        post(
            "kitchen-display/orders/ready",
            TasksRequest(tasks.map(::wire)),
            TasksRequest.serializer(),
            MarkResultDto.serializer(),
        )

    override suspend fun recent(): ApiOutcome<RecentResponseDto> =
        get("kitchen-display/recent", RecentResponseDto.serializer())

    override suspend fun recover(tasks: List<TaskRef>): ApiOutcome<RecoverResultDto> =
        post(
            "kitchen-display/recover",
            TasksRequest(tasks.map(::wire)),
            TasksRequest.serializer(),
            RecoverResultDto.serializer(),
        )

    override suspend fun receipts(receipts: List<ReceiptDto>): ApiOutcome<IgnoredBody> =
        post(
            "kitchen-display/receipts",
            ReceiptsRequest(receipts),
            ReceiptsRequest.serializer(),
            IgnoredBody.serializer(),
        )

    private fun wire(task: TaskRef): TaskRefDto = TaskRefDto(task.id, task.versionId)

    private suspend fun <T> get(
        path: String,
        deserializer: DeserializationStrategy<T>,
    ): ApiOutcome<T> =
        access.call { api: ServerApi, headers: Headers -> api.get(path, deserializer, headers) }
            ?: NOT_PAIRED

    private suspend fun <B, T> post(
        path: String,
        body: B,
        serializer: SerializationStrategy<B>,
        deserializer: DeserializationStrategy<T>,
    ): ApiOutcome<T> =
        access.call { api: ServerApi, headers: Headers ->
            api.post(path, body, serializer, deserializer, headers)
        } ?: NOT_PAIRED

    private companion object {
        // Sin servidor emparejado no sale nada: como una petición que no llegó.
        val NOT_PAIRED = ApiOutcome.NoAnswer(AppError.Unreachable, requestSent = false)
    }
}
