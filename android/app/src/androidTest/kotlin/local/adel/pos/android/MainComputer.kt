package local.adel.pos.android

import java.util.concurrent.TimeUnit
import kotlinx.serialization.json.Json
import kotlinx.serialization.json.JsonArray
import kotlinx.serialization.json.JsonElement
import kotlinx.serialization.json.JsonNull
import kotlinx.serialization.json.JsonObject
import kotlinx.serialization.json.boolean
import kotlinx.serialization.json.jsonArray
import kotlinx.serialization.json.jsonObject
import kotlinx.serialization.json.jsonPrimitive
import okhttp3.MediaType.Companion.toMediaType
import okhttp3.OkHttpClient
import okhttp3.Request
import okhttp3.RequestBody.Companion.toRequestBody

/**
 * La «computadora principal» de las pruebas contra el Go temporal (patrón de la Fase 0): habla por
 * la escucha **local** del Go temporal (`http://10.0.2.2:<ISO_GO_PORT>`, la Mac vista desde el
 * emulador; Go la trata como loopback) con la terminal principal que creó el asistente en el arnés
 * y el usuario `admin`. Hace lo que haría un administrador en Administración: aprobar el código,
 * crear meseros con PIN, cambiar un PIN y retirar el dispositivo. Nunca el servicio real.
 *
 * @param local `http://10.0.2.2:<puerto>` del Go temporal.
 * @param deviceId y [deviceToken]: la terminal principal que devolvió `POST setup`.
 */
class MainComputer(
    private val local: String,
    private val deviceId: String,
    private val deviceToken: String,
) {
    private val client =
        OkHttpClient.Builder()
            .connectTimeout(5, TimeUnit.SECONDS)
            .readTimeout(30, TimeUnit.SECONDS)
            .build()
    private val json = Json { ignoreUnknownKeys = true }
    private var session: String? = null

    class Answer(val status: Int, val body: JsonElement) {
        val obj: JsonObject
            get() = body.jsonObject

        val errorCode: String?
            get() =
                (body as? JsonObject)?.get("error")?.jsonObject?.get("code")?.jsonPrimitive?.content
    }

    fun call(method: String, path: String, body: String? = null): Answer {
        if (session == null && path != "auth/login") {
            login()
        }
        val builder =
            Request.Builder()
                .url("$local/api/v1/$path")
                .header("X-Device-ID", deviceId)
                .header("X-Device-Token", deviceToken)
        session?.let { builder.header("Authorization", "Bearer $it") }
        // POST y PUT siempre llevan cuerpo JSON (vacío si no hay datos); GET no lleva.
        val payload =
            when {
                body != null -> body.toRequestBody(JSON)
                method == "POST" || method == "PUT" -> "{}".toRequestBody(JSON)
                else -> null
            }
        builder.method(method, payload)
        client.newCall(builder.build()).execute().use { response ->
            val text = response.body.string()
            val parsed = if (text.isBlank()) JsonNull else json.parseToJsonElement(text)
            return Answer(response.code, parsed)
        }
    }

    private fun ok(method: String, path: String, body: String? = null): Answer {
        val answer = call(method, path, body)
        check(answer.status == 200) { "$method $path: ${answer.status} ${answer.body}" }
        return answer
    }

    private fun login() {
        val answer = call("POST", "auth/login", """{"username":"admin","password":"admin"}""")
        check(answer.status == 200) { "login: ${answer.status} ${answer.body}" }
        session = answer.obj.getValue("session_token").jsonPrimitive.content
    }

    /** Aprueba la solicitud de emparejamiento lista (ya reveló su número) con [code]. */
    fun approvePairing(code: String): Answer {
        val listed = ok("GET", "devices/pairing-requests").obj.getValue("requests").jsonArray
        val ready =
            listed.map { it.jsonObject }.lastOrNull { it.getValue("ready").jsonPrimitive.boolean }
                ?: error("No hay una solicitud lista: $listed")
        val id = ready.getValue("id").jsonPrimitive.content
        return call("POST", "devices/pairing-requests/$id/approve", """{"code":"$code"}""")
    }

    private fun users(): JsonArray = ok("GET", "users").body.jsonArray

    private fun userId(username: String): String? =
        users()
            .map { it.jsonObject }
            .firstOrNull { it.getValue("username").jsonPrimitive.content == username }
            ?.getValue("id")
            ?.jsonPrimitive
            ?.content

    /**
     * Rol «Meseros A1» con el permiso del PIN y los meseros [waiters] (usuario → nombre corto y
     * PIN) con su PIN y un color de la paleta de Go. Si ya existen, no los vuelve a crear.
     */
    fun ensureWaiters(waiters: Map<String, Pair<String, String>>): Map<String, String> {
        val roleId = ensureRole()
        val colors = ok("GET", "users/badge-colors").body.jsonArray
        val ids = mutableMapOf<String, String>()
        for ((index, entry) in waiters.entries.withIndex()) {
            val (username, profile) = entry
            val (shortName, pin) = profile
            val id = userId(username) ?: createUser(username, shortName, roleId)
            val color =
                colors[(index * 5 + 8) % colors.size]
                    .jsonObject
                    .getValue("hex")
                    .jsonPrimitive
                    .content
            ok(
                "PUT",
                "users/$id/pin",
                """{"pin":"$pin","short_name":"$shortName","badge_color":"$color"}""",
            )
            ids[username] = id
        }
        return ids
    }

    private fun createUser(username: String, shortName: String, roleId: String): String {
        val body =
            """{"username":"$username","display_name":"$shortName A1","role_id":"$roleId",
            |"active":true,"password":"clave-$username"}"""
                .trimMargin()
        return ok("POST", "users", body).obj.getValue("id").jsonPrimitive.content
    }

    // El rol de los meseros de prueba: con el permiso del PIN y lo mínimo para ver mesas.
    private fun ensureRole(): String {
        val roles = ok("GET", "roles").body.jsonArray.map { it.jsonObject }
        val existing = roles.firstOrNull { it.getValue("name").jsonPrimitive.content == ROLE }
        if (existing != null) {
            return existing.getValue("id").jsonPrimitive.content
        }
        val permissions = """["waiter_devices.use","accounts.read","restaurant.tables.read"]"""
        val created = ok("POST", "roles", """{"name":"$ROLE","permissions":$permissions}""")
        return created.obj.getValue("id").jsonPrimitive.content
    }

    /** Cambia el PIN de [username] (sube su `pin_version` y cierra sus sesiones android). */
    fun changePin(username: String, pin: String) {
        val id = userId(username) ?: error("No existe $username")
        ok("PUT", "users/$id/pin", """{"pin":"$pin"}""")
    }

    /** Retira (revoca) los teléfonos y tabletas emparejados. */
    fun revokeAndroidDevices(): Int {
        val devices =
            ok("GET", "devices")
                .body
                .jsonArray
                .map { it.jsonObject }
                .filter {
                    it.getValue("kind").jsonPrimitive.content == "android" &&
                        it["revoked_at"] is JsonNull
                }
        for (device in devices) {
            ok("POST", "devices/${device.getValue("id").jsonPrimitive.content}/revoke")
        }
        return devices.size
    }

    private companion object {
        val JSON = "application/json; charset=utf-8".toMediaType()
        const val ROLE = "Meseros A1"
    }
}
