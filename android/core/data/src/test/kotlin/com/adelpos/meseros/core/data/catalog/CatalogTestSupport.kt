package com.adelpos.meseros.core.data.catalog

import com.adelpos.meseros.core.data.session.AuthorizedServer
import mockwebserver3.MockResponse
import mockwebserver3.MockWebServer
import okhttp3.OkHttpClient

/** Utilidades compartidas por las pruebas del catálogo contra MockWebServer. */
object CatalogTestSupport {
    const val DEVICE_ID = "11111111-1111-4111-8111-111111111111"
    const val DEVICE_TOKEN = "token-del-dispositivo"
    const val SESSION = "sesion-del-mesero"

    /** Bytes de una «imagen» de prueba (el contenido no importa: se comprueba su SHA-256). */
    val IMAGE_BYTES: ByteArray = ByteArray(75) { index -> (index * 7).toByte() }

    const val IMAGE_ID = "d1000000-0000-4000-8000-000000000001"

    fun server(mock: MockWebServer, session: String = SESSION): AuthorizedServer =
        AuthorizedServer(
            baseUrl = mock.url("/"),
            client = OkHttpClient(),
            credentials = AuthorizedServer.credentials(DEVICE_ID, DEVICE_TOKEN, session),
        )

    /** Lee un JSON de ejemplo de `src/test/resources/catalog` con el checksum real de la imagen. */
    fun fixture(name: String): String {
        val stream =
            requireNotNull(javaClass.classLoader?.getResourceAsStream("catalog/$name")) {
                "Falta el recurso catalog/$name"
            }
        val text = stream.use { it.readBytes().decodeToString() }
        return text.replace("CHECKSUM_PLACEHOLDER", CatalogImageCache.sha256(IMAGE_BYTES))
    }

    fun json(status: Int, body: String): MockResponse =
        MockResponse.Builder()
            .code(status)
            .addHeader("Content-Type", "application/json; charset=utf-8")
            .body(body)
            .build()

    fun error(status: Int, code: String): MockResponse =
        json(status, """{"error":{"code":"$code","message":"Mensaje de Go","request_id":"r1"}}""")
}
