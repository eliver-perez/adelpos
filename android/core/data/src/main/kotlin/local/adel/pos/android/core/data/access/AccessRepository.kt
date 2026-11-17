package local.adel.pos.android.core.data.access

import java.time.OffsetDateTime
import java.time.format.DateTimeParseException
import java.util.concurrent.ConcurrentHashMap
import kotlinx.coroutines.CoroutineDispatcher
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.delay
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.launch
import kotlinx.coroutines.sync.Mutex
import kotlinx.coroutines.sync.withLock
import kotlinx.coroutines.withContext
import kotlinx.serialization.json.JsonPrimitive
import kotlinx.serialization.json.booleanOrNull
import local.adel.pos.android.core.data.api.ApiErrors
import local.adel.pos.android.core.data.api.ApiOutcome
import local.adel.pos.android.core.data.api.ApiProblem
import local.adel.pos.android.core.data.api.ClientIdentity
import local.adel.pos.android.core.data.api.DeviceCredentials
import local.adel.pos.android.core.data.api.PinUserDto
import local.adel.pos.android.core.data.api.ServerApi
import local.adel.pos.android.core.data.db.AccessDao
import local.adel.pos.android.core.data.db.DeviceLogEntry
import local.adel.pos.android.core.data.db.KnownWaiterEntity
import local.adel.pos.android.core.data.db.PairedServerEntity
import local.adel.pos.android.core.data.session.ServerTransport
import local.adel.pos.android.core.data.wipe.DeviceDataWipe
import local.adel.pos.android.core.domain.access.ActiveWaiter
import local.adel.pos.android.core.domain.access.DeviceAccess
import local.adel.pos.android.core.domain.access.DeviceStatus
import local.adel.pos.android.core.domain.access.EntryResult
import local.adel.pos.android.core.domain.access.KnownWaiterInvalidation
import local.adel.pos.android.core.domain.access.LocalAttempts
import local.adel.pos.android.core.domain.access.OfflineAttemptPolicy
import local.adel.pos.android.core.domain.access.PinVerifier
import local.adel.pos.android.core.domain.access.RepairReason
import local.adel.pos.android.core.domain.access.UnsentOrder
import local.adel.pos.android.core.domain.access.UnsentOrdersSource
import local.adel.pos.android.core.domain.access.WaiterProfile
import local.adel.pos.android.core.domain.access.WaiterRoster
import local.adel.pos.android.core.domain.error.AppError
import local.adel.pos.android.core.domain.security.SecretSealer
import local.adel.pos.android.core.domain.security.SecretUnavailableException
import local.adel.pos.android.core.domain.server.ServerCompatibility
import local.adel.pos.android.core.domain.server.ServerRelocator
import local.adel.pos.android.core.domain.text.Hex
import okhttp3.Headers

/**
 * Entrada al dispositivo (§3.2 y §4) sobre Room, el Android Keystore y la API v1. Es la única
 * fuente de verdad del estado del dispositivo ([status]) y del mesero activo ([activeWaiter]).
 *
 * Secretos (A1-D13): la credencial del dispositivo, el verificador local del PIN y la credencial de
 * envío de cada mesero se guardan **cifrados** con [sealer] (Android Keystore) y ligados a su uso;
 * las sesiones de Go de cada mesero solo viven en memoria (se pierden al cerrar la aplicación: se
 * vuelve a entrar con el PIN).
 *
 * @param destroyKey borra la llave del Keystore al retirar el dispositivo: todo lo cifrado queda
 *   ilegible aunque algo sobreviviera al borrado.
 * @param isOnline si el dispositivo tiene una red por la que intentar (monitor de conexión).
 * @param unsentOrders pedidos sin enviar que pasan a la bitácora antes de borrar todo (Fase E).
 * @param wipe registro único de «borrar los datos de este dispositivo» (catálogo, imágenes y, en la
 *   Fase E, la cola): se usa al retirar el dispositivo y en «Emparejar de nuevo».
 */
class AccessRepository(
    private val dao: AccessDao,
    private val sealer: SecretSealer,
    private val destroyKey: () -> Unit,
    private val identity: ClientIdentity,
    private val relocator: ServerRelocator,
    private val isOnline: () -> Boolean,
    private val unsentOrders: UnsentOrdersSource,
    private val scope: CoroutineScope,
    private val wipe: DeviceDataWipe = DeviceDataWipe(),
    private val wallClock: () -> Long = System::currentTimeMillis,
    private val verifierIterations: Int = PinVerifier.DEFAULT_ITERATIONS,
    private val cpuDispatcher: CoroutineDispatcher = Dispatchers.Default,
    private val busyRetryMillis: Long = BUSY_RETRY_MILLIS,
) : DeviceAccess {
    private val mutableStatus = MutableStateFlow<DeviceStatus>(DeviceStatus.Starting)
    private val mutableActive = MutableStateFlow<ActiveWaiter?>(null)
    override val status: StateFlow<DeviceStatus> = mutableStatus.asStateFlow()
    override val activeWaiter: StateFlow<ActiveWaiter?> = mutableActive.asStateFlow()

    // Un cambio de estado a la vez (comprobar, entrar, retirar, emparejar).
    private val mutex = Mutex()
    private var server: PairedServerEntity? = null
    private var connection: PairedConnection? = null

    // La última lista de Go (con conexión).
    private var listed: List<WaiterProfile> = emptyList()

    // Sesión de Go de cada mesero en este dispositivo (solo en memoria; cada mesero tiene la suya).
    // Solo la pone una entrada CON conexión; una entrada sin conexión, una sesión terminada o un
    // acceso que dejó de valer la quitan (revisión R3, A1): nunca se reutiliza sin que Go haya
    // verificado el PIN.
    private val sessions = ConcurrentHashMap<String, String>()

    override suspend fun start() {
        mutex.withLock {
            // Se relee lo guardado cada vez que se abre la pantalla principal; el aviso de
            // dispositivo retirado se conserva hasta que el mesero lo lee.
            if (mutableStatus.value !is DeviceStatus.Retired) {
                loadLocked()
            }
            checkLocked()
        }
    }

    override suspend fun recheck() {
        mutex.withLock { checkLocked() }
    }

    override suspend fun roster(): WaiterRoster = mutex.withLock {
        val current = connection
        if (current != null && mutableStatus.value is DeviceStatus.Ready) {
            if (isOnline()) {
                refreshRosterLocked(current)
            } else {
                // Sin red no se pregunta a Go: solo los meseros que ya entraron aquí.
                markOffline()
            }
        }
        val status = mutableStatus.value
        if (status is DeviceStatus.Ready && status.online) {
            WaiterRoster(listed, online = true)
        } else {
            WaiterRoster(knownProfiles(), online = false)
        }
    }

    override suspend fun enter(userId: String, pin: String): EntryResult = mutex.withLock {
        if (!PIN_PATTERN.matches(pin)) {
            return@withLock EntryResult.WrongPin
        }
        val current = connection ?: return@withLock EntryResult.Failed(AppError.Unreachable)
        val status = mutableStatus.value
        if (status !is DeviceStatus.Ready) {
            return@withLock EntryResult.Failed(AppError.Unreachable)
        }
        // Con la última comprobación sin respuesta se entra directo sin conexión (y se vuelve
        // a comprobar aparte); así el mesero no espera a que venza la conexión.
        if (status.online && isOnline()) {
            val online = enterOnlineLocked(current, userId, pin)
            if (online != null) {
                return@withLock online
            }
        } else {
            scope.launch { recheck() }
        }
        enterOfflineLocked(userId, pin)
    }

    override fun leave() {
        mutableActive.value = null
    }

    override suspend fun pendingOrders(): List<UnsentOrder> = unsentOrders.unsentOrders()

    override suspend fun forgetServer() {
        mutex.withLock {
            forgetLocked()
            mutableStatus.value = DeviceStatus.Unpaired(deviceLog())
        }
    }

    override suspend fun clearDeviceLog() {
        mutex.withLock {
            dao.clearLog()
            val status = mutableStatus.value
            if (status is DeviceStatus.Unpaired || status is DeviceStatus.Retired) {
                mutableStatus.value = DeviceStatus.Unpaired()
            }
        }
    }

    override fun acknowledgeRetirement() {
        val status = mutableStatus.value
        if (status is DeviceStatus.Retired) {
            mutableStatus.value = DeviceStatus.Unpaired(status.unsentOrders)
        }
    }

    override fun report(error: AppError, userId: String?) {
        scope.launch {
            mutex.withLock {
                when {
                    error is AppError.DeviceRevoked -> retireLocked()
                    error is AppError.SessionEnded ->
                        accessEndedLocked(userId, forgetOfflineEntry = false)
                    error is AppError.Rejected && error.code == OFFLINE_CREDENTIAL_INVALID ->
                        // La credencial de envío ya no vale (PIN cambiado, usuario desactivado,
                        // reemplazada): el verificador tampoco; hace falta entrar con conexión.
                        accessEndedLocked(userId, forgetOfflineEntry = true)
                    else -> applyErrorLocked(error)
                }
            }
        }
    }

    /**
     * El acceso de [userId] dejó de valer (sesión terminada o revocada, PIN cambiado, credencial de
     * envío rechazada; revisión R3, M3): se olvida su sesión (y, con [forgetOfflineEntry], su
     * verificador y su credencial). Si es el mesero activo, **sigue activo** con su captura y sus
     * pendientes, pero la pantalla le pide «Vuelve a escribir tu PIN»; nunca se le saca ni se pasa
     * a otro mesero en silencio.
     */
    private suspend fun accessEndedLocked(userId: String?, forgetOfflineEntry: Boolean) {
        if (userId == null) {
            return
        }
        sessions.remove(userId)
        if (forgetOfflineEntry) {
            dao.deleteKnownWaiters(listOf(userId))
        }
        val active = mutableActive.value
        if (active != null && active.userId == userId) {
            mutableActive.value = active.copy(hasServerSession = false, mustReenterPin = true)
        }
    }

    /** Sesión de Go del mesero [userId] en este dispositivo (la cola y el modo Mesero, Fase E). */
    fun sessionToken(userId: String): String? = sessions[userId]

    /** Credencial del dispositivo emparejado, para las demás rutas de la API (Fase E). */
    fun deviceCredentials(): DeviceCredentials? = connection?.credentials

    /**
     * Punto único para que otro módulo (catálogo de la Fase D, modo Mesero de la Fase E) hable con
     * el servidor emparejado: la API en la dirección vigente con la huella fijada, o null si no hay
     * servidor emparejado. Se pide en cada uso (la dirección cambia con el cambio de IP).
     */
    fun serverApi(): ServerApi? = connection?.api()

    /**
     * Dirección vigente y cliente con la huella fijada del servidor emparejado, para lo que no cabe
     * en [ServerApi] (imágenes binarias con `ETag` y el canal de tiempo real de la Fase D). null
     * sin servidor emparejado; se pide en cada uso, como [serverApi].
     */
    fun serverTransport(): ServerTransport? = connection?.transport()

    /**
     * Cabeceras de autenticación para una operación **del mesero [userId]** (el dueño de la
     * operación, que la guarda al crearla; revisión R3, M5): las del dispositivo (`X-Device-ID`,
     * `X-Device-Token`) y, si ese mesero tiene **sesión de Go** (entró con conexión y no terminó),
     * su `Authorization: Bearer`. Nunca las de otro mesero: una operación de Ana que termina
     * después de cambiar a Beto sale con la sesión de Ana (o sin sesión). null sin servidor
     * emparejado. Si la respuesta es 401 `DEVICE_REVOKED`, `SESSION_*` u
     * `OFFLINE_CREDENTIAL_INVALID`, o la huella cambió, llamar a [report] con el mismo [userId].
     */
    fun requestHeaders(userId: String): Headers? {
        val credentials = connection?.credentials ?: return null
        return credentials.headers(sessions[userId])
    }

    /**
     * Credencial de envío del mesero [userId] (A1-D1) para `submitOfflineOrder` sin sesión, o null
     * si no hay (nunca entró con conexión aquí o cambió su PIN).
     */
    suspend fun offlineCredential(userId: String): String? {
        val sealed = dao.knownWaiter(userId)?.sealedOfflineCredential ?: return null
        return try {
            String(sealer.open(sealed, offlinePurpose(userId)), Charsets.UTF_8)
        } catch (_: SecretUnavailableException) {
            null
        }
    }

    /** Guarda el servidor recién emparejado (la credencial ya va cifrada) y lo comprueba. */
    internal suspend fun onPaired(entity: PairedServerEntity) {
        mutex.withLock {
            dao.savePairedServer(entity)
            loadLocked()
            checkLocked()
        }
    }

    private suspend fun loadLocked() {
        val entity = dao.pairedServer()
        if (entity == null) {
            server = null
            connection = null
            mutableStatus.value = DeviceStatus.Unpaired(deviceLog())
            return
        }
        val token =
            try {
                String(sealer.open(entity.sealedDeviceToken, DEVICE_PURPOSE), Charsets.UTF_8)
            } catch (_: SecretUnavailableException) {
                server = entity
                connection = null
                mutableStatus.value = DeviceStatus.NeedsRepair(RepairReason.CREDENTIAL_LOST)
                return
            }
        server = entity
        connection =
            PairedConnection(
                serverId = entity.serverId,
                fingerprint = Hex.decode(entity.fingerprintHex),
                initialBase = entity.baseUrl,
                credentials = DeviceCredentials(entity.deviceId, token),
                identity = identity,
                relocator = relocator,
                onMoved = { base -> dao.updateBaseUrl(base) },
            )
        mutableStatus.value = ready(entity, online = false)
    }

    // Comprueba versiones y la lista de meseros. Solo desde un estado que tenga servidor.
    private suspend fun checkLocked() {
        val current = connection ?: return
        val status = mutableStatus.value
        if (status is DeviceStatus.Retired || status is DeviceStatus.Unpaired) {
            return
        }
        if (status is DeviceStatus.NeedsRepair && status.reason != RepairReason.IDENTITY_CHANGED) {
            return
        }
        if (!isOnline()) {
            markOffline()
            return
        }
        when (val version = current.version()) {
            is ApiOutcome.Success -> {
                val info = version.value
                val verdict =
                    ServerCompatibility.check(
                        appVersion = identity.version,
                        serverVersion = info.serverVersion,
                        capabilities = info.capabilities,
                        minAndroidVersion = info.minClientVersions[ClientIdentity.KIND],
                    )
                when (verdict) {
                    is ServerCompatibility.Verdict.UpdateServer -> {
                        mutableStatus.value = DeviceStatus.UpdateServer(verdict.serverVersion)
                        return
                    }
                    is ServerCompatibility.Verdict.UpdateApp -> {
                        mutableStatus.value =
                            DeviceStatus.UpdateApp(identity.version, verdict.minClientVersion)
                        return
                    }
                    ServerCompatibility.Verdict.Compatible -> Unit
                }
            }
            is ApiOutcome.Rejected -> {
                applyErrorLocked(ApiErrors.toAppError(version.problem))
                return
            }
            is ApiOutcome.NoAnswer -> {
                applyErrorLocked(version.error)
                return
            }
            is ApiOutcome.Malformed -> {
                markOffline()
                return
            }
        }
        refreshRosterLocked(current)
    }

    private suspend fun refreshRosterLocked(current: PairedConnection) {
        when (val outcome = current.pinUsers()) {
            is ApiOutcome.Success -> {
                val profiles = outcome.value.users.map(::profileOf)
                invalidateLocked(profiles)
                val device = outcome.value.device
                dao.updateDevice(device.name, device.idleLockSeconds)
                val entity = dao.pairedServer() ?: return
                server = entity
                listed = profiles
                mutableStatus.value = ready(entity, online = true)
            }
            is ApiOutcome.Rejected -> applyErrorLocked(ApiErrors.toAppError(outcome.problem))
            is ApiOutcome.NoAnswer -> applyErrorLocked(outcome.error)
            is ApiOutcome.Malformed -> markOffline()
        }
    }

    // Con la lista de Go: borra verificador y credencial de envío de quien ya no aparece o cambió
    // de PIN, cierra la entrada sin conexión de quien tiene el PIN bloqueado en Go (M2) y actualiza
    // nombre, inicial y color de los demás. Si el mesero activo dejó de valer, sigue activo y se le
    // pide volver a escribir su PIN (M3).
    private suspend fun invalidateLocked(profiles: List<WaiterProfile>) {
        val known = dao.knownWaiters()
        val stale =
            KnownWaiterInvalidation.stale(
                known.map { KnownWaiterInvalidation.Known(it.userId, it.pinVersion) },
                profiles,
            )
        if (stale.isNotEmpty()) {
            dao.deleteKnownWaiters(stale.toList())
        }
        val byId = profiles.associateBy(WaiterProfile::userId)
        for (waiter in known) {
            val profile = byId[waiter.userId] ?: continue
            if (waiter.userId in stale) {
                continue
            }
            val lockedInGo = profile.lockedUntilEpochMillis != null || profile.requiresUnlock
            if (lockedInGo && !waiter.offlineClosed) {
                dao.updateOfflineAttempts(waiter.userId, waiter.offlineFailures, closed = true)
            }
            if (
                profile.name != waiter.name ||
                    profile.initial != waiter.initial ||
                    profile.color != waiter.color
            ) {
                dao.knownWaiter(waiter.userId)?.let { current ->
                    dao.saveKnownWaiter(current.withProfile(profile))
                }
            }
        }
        for (userId in stale) {
            accessEndedLocked(userId, forgetOfflineEntry = false)
        }
    }

    // null: no hubo respuesta del servidor; se sigue con el verificador local.
    private suspend fun enterOnlineLocked(
        current: PairedConnection,
        userId: String,
        pin: String,
    ): EntryResult? {
        var attempt = 1
        var outcome = current.pinLogin(userId, pin)
        // 503 PIN_LOGIN_BUSY: Go está verificando muchos PIN a la vez. No es un PIN incorrecto ni
        // falta de red (no cuenta como fallo ni se entra con el verificador local): se vuelve a
        // intentar a los pocos segundos y, si sigue ocupado, se avisa.
        while (outcome is ApiOutcome.Rejected && outcome.problem.code == PIN_LOGIN_BUSY) {
            if (attempt >= BUSY_ATTEMPTS) {
                return EntryResult.Busy
            }
            attempt += 1
            delay(busyRetryMillis)
            outcome = current.pinLogin(userId, pin)
        }
        when (outcome) {
            is ApiOutcome.Success -> {
                val response = outcome.value
                val profile = enteredProfile(userId, response.pinVersion)
                rememberWaiterLocked(profile, pin, response.offlineCredential)
                sessions[userId] = response.sessionToken
                val waiter = ActiveWaiter(profile, hasServerSession = true)
                mutableActive.value = waiter
                return EntryResult.Entered(waiter)
            }
            is ApiOutcome.Rejected -> return entryRejectedLocked(userId, outcome.problem)
            is ApiOutcome.NoAnswer -> {
                if (outcome.error == AppError.ServerIdentityChanged) {
                    applyErrorLocked(outcome.error)
                    return EntryResult.Failed(outcome.error)
                }
                markOffline()
                return null
            }
            is ApiOutcome.Malformed ->
                return EntryResult.Failed(AppError.InvalidResponse(outcome.status))
        }
    }

    // Nombre, inicial y color del mesero que acaba de entrar: de la lista de Go o, si todavía no
    // se tenía, de lo guardado; con la pin_version que confirmó Go y sin bloqueo.
    private suspend fun enteredProfile(userId: String, pinVersion: Int): WaiterProfile {
        val known =
            listed.firstOrNull { it.userId == userId }
                ?: dao.knownWaiter(userId)?.profile()
                ?: WaiterProfile(
                    userId = userId,
                    name = "",
                    initial = "",
                    color = null,
                    pinVersion = pinVersion,
                    lockedUntilEpochMillis = null,
                )
        return known.copy(pinVersion = pinVersion, lockedUntilEpochMillis = null)
    }

    // Rechazos de pin-login. Un PIN incorrecto con conexión cuenta en el mismo límite que los de
    // sin conexión, y un bloqueo de Go cierra la entrada sin conexión (revisión R3, M2): el
    // presupuesto de intentos es uno solo, con o sin red.
    private suspend fun entryRejectedLocked(userId: String, problem: ApiProblem): EntryResult =
        when (problem.code) {
            "PIN_INVALID" -> {
                countOfflineFailureLocked(userId)
                EntryResult.WrongPin
            }
            "VALIDATION_ERROR" -> EntryResult.WrongPin
            "PIN_LOCKED" -> {
                closeOfflineEntryLocked(userId)
                EntryResult.Locked(problem.lockedUntil(), requiresUnlock = problem.requiresUnlock())
            }
            "PERMISSION_DENIED" -> EntryResult.NotAllowed
            else -> {
                val error = ApiErrors.toAppError(problem)
                applyErrorLocked(error)
                EntryResult.Failed(error)
            }
        }

    // Un fallo más para el límite sin conexión de un mesero conocido aquí.
    private suspend fun countOfflineFailureLocked(userId: String) {
        val known = dao.knownWaiter(userId) ?: return
        val next =
            OfflineAttemptPolicy.afterFailure(
                LocalAttempts(known.offlineFailures, known.offlineClosed),
            )
        dao.updateOfflineAttempts(userId, next.failures, next.closed)
    }

    // Go bloqueó el PIN: la entrada sin conexión de ese mesero se cierra hasta que entre con
    // conexión.
    private suspend fun closeOfflineEntryLocked(userId: String) {
        val known = dao.knownWaiter(userId) ?: return
        val next =
            OfflineAttemptPolicy.afterServerLock(
                LocalAttempts(known.offlineFailures, known.offlineClosed),
            )
        dao.updateOfflineAttempts(userId, next.failures, next.closed)
    }

    // Guarda (o renueva) el verificador local y la credencial de envío, cifrados con el Keystore.
    // Si el Keystore falla, el mesero entra igual con conexión, solo que sin entrada sin conexión.
    private suspend fun rememberWaiterLocked(
        profile: WaiterProfile,
        pin: String,
        offlineCredential: String,
    ) {
        val serverId = server?.serverId ?: return
        val now = wallClock()
        val verifier =
            withContext(cpuDispatcher) {
                PinVerifier.create(pin, profile.pinVersion, verifierIterations)
            }
        val sealedVerifier: ByteArray?
        val sealedCredential: ByteArray?
        try {
            sealedVerifier = PinVerifier.seal(verifier, profile.userId, sealer)
            sealedCredential =
                sealer.seal(
                    offlineCredential.toByteArray(Charsets.UTF_8),
                    offlinePurpose(profile.userId),
                )
        } catch (_: SecretUnavailableException) {
            dao.deleteKnownWaiters(listOf(profile.userId))
            return
        }
        dao.saveKnownWaiter(
            KnownWaiterEntity(
                userId = profile.userId,
                serverId = serverId,
                name = profile.name,
                initial = profile.initial,
                color = profile.color,
                pinVersion = profile.pinVersion,
                sealedVerifier = sealedVerifier,
                sealedOfflineCredential = sealedCredential,
                lastOnlineEntryMillis = now,
                // Go acaba de verificar el PIN: los fallos vuelven a cero y la entrada sin conexión
                // se abre (OfflineAttemptPolicy.afterOnlineSuccess).
                offlineFailures = OfflineAttemptPolicy.afterOnlineSuccess().failures,
                offlineClosed = OfflineAttemptPolicy.afterOnlineSuccess().closed,
            ),
        )
    }

    // Sin conexión: solo quien ya entró con conexión aquí, con su verificador y el límite local,
    // que no depende de ningún reloj (revisión R3, M1). La entrada sin conexión nunca da una sesión
    // de Go: la que el mesero tuviera en memoria de una entrada anterior se olvida (R3, A1).
    private suspend fun enterOfflineLocked(userId: String, pin: String): EntryResult {
        val known = dao.knownWaiter(userId)
        val sealed = known?.sealedVerifier ?: return EntryResult.UnknownOffline
        val attempts = LocalAttempts(known.offlineFailures, known.offlineClosed)
        if (attempts.closed) {
            return EntryResult.OfflineClosed
        }
        val verifier =
            try {
                PinVerifier.open(sealed, userId, sealer)
            } catch (_: SecretUnavailableException) {
                // Ya no se puede abrir (llave perdida o dato dañado): se olvida y se pide entrar
                // con conexión.
                dao.deleteKnownWaiters(listOf(userId))
                return EntryResult.UnknownOffline
            }
        val matches = withContext(cpuDispatcher) { verifier.matches(pin) }
        if (!matches) {
            val next = OfflineAttemptPolicy.afterFailure(attempts)
            dao.updateOfflineAttempts(userId, next.failures, next.closed)
            if (next.closed) {
                return EntryResult.OfflineClosed
            }
            return EntryResult.WrongPin
        }
        val next = OfflineAttemptPolicy.afterOfflineSuccess(attempts)
        dao.updateOfflineAttempts(userId, next.failures, next.closed)
        sessions.remove(userId)
        val waiter = ActiveWaiter(known.profile(), hasServerSession = false)
        mutableActive.value = waiter
        return EntryResult.Entered(waiter)
    }

    // Lo que hace cada error de Go con el estado del dispositivo.
    private suspend fun applyErrorLocked(error: AppError) {
        when (error) {
            is AppError.DeviceRevoked -> retireLocked()
            is AppError.DeviceNotRecognized -> needsRepair(RepairReason.NOT_RECOGNIZED)
            AppError.ServerIdentityChanged -> needsRepair(RepairReason.IDENTITY_CHANGED)
            is AppError.UpdateRequired ->
                mutableStatus.value =
                    DeviceStatus.UpdateApp(identity.version, error.minClientVersion)
            is AppError.Rejected ->
                when (error.code) {
                    "PIN_DEVICE_NOT_ALLOWED" -> needsRepair(RepairReason.NOT_RECOGNIZED)
                    // El tipo de cliente no coincide con el del dispositivo: un problema para
                    // soporte, no una sesión que dejó de valer ni falta de red; nada cambia aquí.
                    CLIENT_KIND_MISMATCH -> Unit
                    else -> markOffline()
                }
            is AppError.ServerProblem ->
                if (error.code != PIN_LOGIN_BUSY) {
                    markOffline()
                }
            else -> markOffline()
        }
    }

    private fun needsRepair(reason: RepairReason) {
        mutableActive.value = null
        mutableStatus.value = DeviceStatus.NeedsRepair(reason)
    }

    private fun markOffline() {
        val entity = server ?: return
        if (connection == null) {
            return
        }
        mutableStatus.value = ready(entity, online = false)
    }

    // DEVICE_REVOKED (§3.2): los pedidos sin enviar pasan a la bitácora, se borra todo lo demás
    // (servidor, meseros, verificadores y credenciales) y se destruye la llave del Keystore.
    private suspend fun retireLocked() {
        if (mutableStatus.value is DeviceStatus.Retired) {
            return
        }
        forgetLocked()
        mutableStatus.value = DeviceStatus.Retired(deviceLog())
    }

    // Retirar el dispositivo o emparejarlo de nuevo (revisión R3, M4), en este orden:
    // 1. los pedidos sin enviar pasan a la bitácora (se conserva hasta que alguien la borra);
    // 2. se borran los datos de este dispositivo de todo el registro (catálogo, imágenes y, en la
    //    Fase E, la cola con su trabajo de WorkManager);
    // 3. se olvidan el emparejamiento, los meseros, sus verificadores y credenciales;
    // 4. se destruye la llave del Keystore: lo cifrado que quedara ya no se puede leer.
    private suspend fun forgetLocked() {
        val now = wallClock()
        val unsent = unsentOrders.unsentOrders()
        if (unsent.isNotEmpty()) {
            dao.addToLog(
                unsent.map { order ->
                    DeviceLogEntry(
                        createdAtMillis = now,
                        kind = DeviceLogEntry.KIND_UNSENT_ORDER,
                        waiterName = order.waiterName,
                        description = order.description,
                        capturedAtMillis = order.capturedAtEpochMillis,
                    )
                },
            )
        }
        wipe.eraseAll()
        dao.forgetServer(emptyList())
        sessions.clear()
        listed = emptyList()
        mutableActive.value = null
        connection = null
        server = null
        try {
            destroyKey()
        } catch (_: SecretUnavailableException) {
            // Lo cifrado ya no tiene servidor que lo use; la llave se reemplaza al emparejar.
        }
    }

    private suspend fun deviceLog(): List<UnsentOrder> =
        dao.deviceLog()
            .filter { it.kind == DeviceLogEntry.KIND_UNSENT_ORDER }
            .map { entry ->
                UnsentOrder(
                    waiterName = entry.waiterName.orEmpty(),
                    description = entry.description,
                    capturedAtEpochMillis = entry.capturedAtMillis ?: entry.createdAtMillis,
                )
            }

    // Los meseros que pueden intentar entrar sin conexión aquí; los de entrada cerrada se muestran
    // con su aviso («Entra con conexión»).
    private suspend fun knownProfiles(): List<WaiterProfile> =
        dao.knownWaiters()
            .filter { it.sealedVerifier != null }
            .map { waiter -> waiter.profile().copy(offlineEntryClosed = waiter.offlineClosed) }

    private fun ready(entity: PairedServerEntity, online: Boolean) =
        DeviceStatus.Ready(
            serverName = entity.serverName,
            deviceName = entity.deviceName,
            idleLockSeconds = entity.idleLockSeconds,
            online = online,
        )

    private fun profileOf(user: PinUserDto) =
        WaiterProfile(
            userId = user.id,
            name = user.name,
            initial = user.initial,
            color = user.color,
            pinVersion = user.pinVersion,
            lockedUntilEpochMillis = user.lockedUntil?.let(::epochMillis),
            requiresUnlock = user.requiresUnlock,
        )

    private fun KnownWaiterEntity.profile() =
        WaiterProfile(userId, name, initial, color, pinVersion, lockedUntilEpochMillis = null)

    private fun KnownWaiterEntity.withProfile(profile: WaiterProfile) =
        KnownWaiterEntity(
            userId = userId,
            serverId = serverId,
            name = profile.name,
            initial = profile.initial,
            color = profile.color,
            pinVersion = pinVersion,
            sealedVerifier = sealedVerifier,
            sealedOfflineCredential = sealedOfflineCredential,
            lastOnlineEntryMillis = lastOnlineEntryMillis,
            offlineFailures = offlineFailures,
            offlineClosed = offlineClosed,
        )

    // `details.locked_until` de 429 PIN_LOCKED (RFC 3339), en milisegundos.
    private fun ApiProblem.lockedUntil(): Long? {
        val value = details?.get("locked_until") as? JsonPrimitive ?: return null
        if (!value.isString) {
            return null
        }
        return epochMillis(value.content)
    }

    // `details.requires_unlock` de 429 PIN_LOCKED (A1-D16): bloqueado hasta que lo desbloquee un
    // administrador.
    private fun ApiProblem.requiresUnlock(): Boolean {
        val value = details?.get("requires_unlock") as? JsonPrimitive ?: return false
        return value.booleanOrNull == true
    }

    companion object {
        /** Credencial de envío que ya no vale (`submitOfflineOrder`). */
        const val OFFLINE_CREDENTIAL_INVALID = "OFFLINE_CREDENTIAL_INVALID"

        /** 503: demasiadas entradas con PIN verificándose a la vez en Go; reintentar. */
        const val PIN_LOGIN_BUSY = "PIN_LOGIN_BUSY"

        /** 400: `X-Client-Kind` distinto del tipo del dispositivo emparejado (soporte). */
        const val CLIENT_KIND_MISMATCH = "CLIENT_KIND_MISMATCH"

        /**
         * Intentos de `pin-login` mientras Go responde que está ocupado, y la espera entre ellos.
         */
        const val BUSY_ATTEMPTS = 3
        const val BUSY_RETRY_MILLIS = 2_000L

        /** Propósito del cifrado de la credencial del dispositivo. */
        const val DEVICE_PURPOSE = "device-credential"
        private val PIN_PATTERN = Regex("^[0-9]{4,6}$")

        /** Propósito del cifrado de la credencial de envío de [userId] (A1-D1). */
        fun offlinePurpose(userId: String): String = "offline-credential:$userId"

        /** Fecha RFC 3339 de Go («2026-11-16T14:35:00.123-06:00») en milisegundos, o null. */
        fun epochMillis(text: String): Long? =
            try {
                OffsetDateTime.parse(text).toInstant().toEpochMilli()
            } catch (_: DateTimeParseException) {
                null
            }
    }
}
