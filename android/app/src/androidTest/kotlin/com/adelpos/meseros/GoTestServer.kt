package com.adelpos.meseros

import androidx.test.platform.app.InstrumentationRegistry
import java.nio.ByteBuffer
import java.security.MessageDigest
import java.security.SecureRandom
import java.util.UUID
import kotlinx.serialization.json.Json
import kotlinx.serialization.json.JsonObject
import kotlinx.serialization.json.jsonObject
import kotlinx.serialization.json.jsonPrimitive
import okhttp3.MediaType.Companion.toMediaType
import okhttp3.OkHttpClient
import okhttp3.Request
import okhttp3.RequestBody.Companion.toRequestBody
import org.junit.Assert.assertEquals
import org.junit.Assume.assumeTrue

/**
 * Ayudante de las pruebas contra el Go temporal del arnés (`iso.sh server-up`), con el patrón de la
 * Fase 0 (prueba t05): la escucha local HTTP (`http://10.0.2.2:<ISO_GO_PORT>`, que Go trata como la
 * «computadora principal») configura, crea el catálogo, aprueba el emparejamiento y hace los
 * cambios; el dispositivo usa la escucha de red HTTPS con la huella fijada. Argumentos de la
 * instrumentación: `pin` (huella SPKI en hexadecimal), `net`, `local` y la terminal principal del
 * guion (`mainDevice` y `mainToken`) o `provisioning`. **Nunca** contra el servicio real de la Mac
 * (8181, 8443).
 */
class GoTestServer {
    private val arguments = InstrumentationRegistry.getArguments()
    private val plain = OkHttpClient()

    val local: String = argumentOrSkip("local")
    val network: String = argumentOrSkip("net")
    val pin: ByteArray = hexDecode(argumentOrSkip("pin"))

    /** Respuesta de Go: estado y cuerpo JSON (vacío si no tenía). */
    data class Answer(val status: Int, val body: JsonObject) {
        fun text(name: String): String = body.getValue(name).jsonPrimitive.content
    }

    fun argument(name: String): String? = arguments.getString(name)

    private fun argumentOrSkip(name: String): String {
        val value = arguments.getString(name)
        assumeTrue(
            "Sin el argumento de instrumentación $name (Go temporal)",
            !value.isNullOrBlank(),
        )
        return value!!
    }

    /** Petición bloqueante (las pruebas instrumentadas no corren en el hilo principal). */
    fun call(
        client: OkHttpClient,
        method: String,
        url: String,
        body: String? = null,
        headers: Map<String, String> = emptyMap(),
    ): Answer {
        val builder = Request.Builder().url(url)
        headers.forEach { (name, value) -> builder.header(name, value) }
        // Un POST sin cuerpo manda un cuerpo vacío (OkHttp lo exige); un GET, ninguno.
        val payload =
            when {
                body != null -> body.toRequestBody(JSON_TYPE)
                method == "POST" -> "".toRequestBody(JSON_TYPE)
                else -> null
            }
        builder.method(method, payload)
        client.newCall(builder.build()).execute().use { response ->
            val text = response.body.string()
            val parsed =
                if (text.isBlank()) {
                    JsonObject(emptyMap())
                } else {
                    Json.parseToJsonElement(text).jsonObject
                }
            return Answer(response.code, parsed)
        }
    }

    /** Petición a la escucha local como la computadora principal con su sesión de administrador. */
    fun main(method: String, path: String, body: String? = null, main: MainComputer): Answer =
        call(plain, method, "$local/api/v1/$path", body, main.headers())

    /** Credenciales de la computadora principal (escucha local). */
    data class MainComputer(val deviceId: String, val deviceToken: String, val session: String) {
        fun headers(): Map<String, String> =
            mapOf(
                "X-Device-ID" to deviceId,
                "X-Device-Token" to deviceToken,
                "Authorization" to "Bearer $session",
            )
    }

    /** Credenciales del dispositivo Android emparejado y su sesión. */
    data class Device(val deviceId: String, val deviceToken: String, val session: String)

    /**
     * La computadora principal con sesión de administrador: la terminal que creó el guion
     * (`scripts/android-e2e.sh`, argumentos `mainDevice` y `mainToken`) o, sin ella, el asistente
     * inicial con el token de aprovisionamiento (argumento `provisioning`).
     */
    fun mainComputer(): MainComputer {
        val deviceId = arguments.getString("mainDevice")
        val deviceToken = arguments.getString("mainToken")
        if (deviceId.isNullOrBlank() || deviceToken.isNullOrBlank()) {
            return setUpMainComputer()
        }
        return MainComputer(deviceId, deviceToken, adminSession(deviceId, deviceToken))
    }

    /** Sesión nueva del administrador en la terminal principal [deviceId]. */
    fun adminSession(deviceId: String, deviceToken: String): String {
        val login =
            call(
                plain,
                "POST",
                "$local/api/v1/auth/login",
                """{"username":"admin","password":"admin"}""",
                mapOf("X-Device-ID" to deviceId, "X-Device-Token" to deviceToken),
            )
        assertEquals(login.body.toString(), 200, login.status)
        return login.text("session_token")
    }

    /** Asistente inicial con el token de aprovisionamiento y sesión de administrador. */
    private fun setUpMainComputer(): MainComputer {
        val setup =
            call(
                plain,
                "POST",
                "$local/api/v1/setup",
                """{"name":"Prueba catálogo D","timezone":"America/Monterrey","currency":"MXN",""" +
                    """"theme":"light","terminal_name":"Principal"}""",
                mapOf("X-Provisioning-Token" to argumentOrSkip("provisioning")),
            )
        assertEquals(setup.body.toString(), 200, setup.status)
        val deviceId = setup.text("device_id")
        val deviceToken = setup.text("device_token")
        return MainComputer(deviceId, deviceToken, adminSession(deviceId, deviceToken))
    }

    /**
     * Emparejamiento completo como `android` por la red (compromiso, revelación, código aprobado
     * por la principal y reclamo) y entrada del administrador en el dispositivo.
     */
    fun pairAndroid(client: OkHttpClient, main: MainComputer): Device {
        val nonce = ByteArray(32).also { SecureRandom().nextBytes(it) }
        val commitment = hexEncode(MessageDigest.getInstance("SHA-256").digest(nonce))
        val created =
            call(
                client,
                "POST",
                "$network/api/v1/pairing/requests",
                """{"name":"Tableta catálogo D","device_kind":"android","os":"Android",""" +
                    """"app_version":"0.1.0","commitment":"$commitment"}""",
            )
        assertEquals(created.body.toString(), 200, created.status)
        val requestId = created.text("request_id")
        val secret = mapOf("X-Pairing-Secret" to created.text("pairing_secret"))
        val revealed =
            call(
                client,
                "POST",
                "$network/api/v1/pairing/requests/$requestId/reveal",
                """{"terminal_nonce":"${hexEncode(nonce)}"}""",
                secret,
            )
        assertEquals(revealed.body.toString(), 200, revealed.status)
        val code = pairingCode(pin, nonce, hexDecode(created.text("server_nonce")), requestId)
        val approved =
            main(
                "POST",
                "devices/pairing-requests/$requestId/approve",
                """{"code":"$code"}""",
                main,
            )
        assertEquals(approved.body.toString(), 200, approved.status)
        val claimed =
            call(client, "POST", "$network/api/v1/pairing/requests/$requestId/claim", null, secret)
        assertEquals(claimed.body.toString(), 200, claimed.status)
        val deviceId = claimed.text("device_id")
        val deviceToken = claimed.text("device_token")
        val login =
            call(
                client,
                "POST",
                "$network/api/v1/auth/login",
                """{"username":"admin","password":"admin"}""",
                mapOf("X-Device-ID" to deviceId, "X-Device-Token" to deviceToken),
            )
        assertEquals(login.body.toString(), 200, login.status)
        return Device(deviceId, deviceToken, login.text("session_token"))
    }

    companion object {
        private val JSON_TYPE = "application/json; charset=utf-8".toMediaType()

        fun operationId(): String = UUID.randomUUID().toString()

        fun hexEncode(bytes: ByteArray): String =
            bytes.joinToString(separator = "") { byte -> "%02x".format(byte) }

        fun hexDecode(text: String): ByteArray =
            ByteArray(text.length / 2) { index ->
                text.substring(index * 2, index * 2 + 2).toInt(16).toByte()
            }

        /**
         * Código de 6 dígitos del emparejamiento de la 9G (`administration/pairing.go`):
         * SHA-256(huella ‖ nT ‖ nS ‖ id en 16 bytes), primeros 8 bytes sin signo módulo 10⁶.
         */
        fun pairingCode(
            fingerprint: ByteArray,
            terminalNonce: ByteArray,
            serverNonce: ByteArray,
            requestId: String,
        ): String {
            val digest = MessageDigest.getInstance("SHA-256")
            digest.update(fingerprint)
            digest.update(terminalNonce)
            digest.update(serverNonce)
            digest.update(hexDecode(requestId.replace("-", "").lowercase()))
            val first = ByteBuffer.wrap(digest.digest(), 0, 8).long
            return "%06d".format(java.lang.Long.remainderUnsigned(first, 1_000_000L))
        }
    }
}
