package local.adel.pos.android.core.data.api

import okhttp3.Interceptor
import okhttp3.Response

/** Agrega `X-Client-Version` y `X-Client-Kind: android` a cada petición al servidor. */
class ClientHeadersInterceptor(private val identity: ClientIdentity) : Interceptor {
    override fun intercept(chain: Interceptor.Chain): Response {
        val request =
            chain
                .request()
                .newBuilder()
                .header(ClientIdentity.VERSION_HEADER, identity.version)
                .header(ClientIdentity.KIND_HEADER, identity.kind)
                .build()
        return chain.proceed(request)
    }
}
