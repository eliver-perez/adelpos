package com.adelpos.meseros.core.domain.ready

import com.adelpos.meseros.core.domain.error.AppError
import com.adelpos.meseros.core.domain.ready.ReadyAlertsLifecycle.ErrorAction
import com.adelpos.meseros.core.domain.ready.ReadyAlertsLifecycle.StartDecision
import com.adelpos.meseros.core.domain.ready.ReadyAlertsLifecycle.Trigger
import com.adelpos.meseros.core.domain.waiter.OrderMode
import kotlinx.coroutines.runBlocking
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNull
import org.junit.Assert.assertSame
import org.junit.Assert.assertTrue
import org.junit.Test

/**
 * Reglas del aviso de platillos listos en el dispositivo (etapa A2, Fase D): texto (A2-D10) con
 * singular y plural, agrupación por cuenta, sin duplicados, reanudación desde el cursor, decisiones
 * de arranque y parada, espera creciente y canal según la configuración (A2-D7).
 */
class ReadyAlertsRulesTest {
    /** Las mismas plantillas de `app/src/main/res/values/strings.xml`. */
    private val words =
        ReadyAlertWords(
            dishOne = "platillo listo",
            dishOther = "platillos listos",
            takeaway = "Para llevar #%1\$d",
            takeawayNamed = "Para llevar #%1\$d (%2\$s)",
            account = "Cuenta #%1\$d",
            title = "%1\$s · %2\$s",
            item = "%1\$d × %2\$s",
            itemSeparator = ", ",
            summaryTitle = "Platillos listos",
            summarySeparator = ", ",
        )

    private fun alert(
        id: Long,
        account: String = "cuenta-4",
        mode: OrderMode = OrderMode.DINE_IN,
        table: String? = "Mesa 4",
        folio: Long = 31,
        customer: String = "",
        waiter: String = "ana",
        items: List<ReadyItem> = listOf(ReadyItem("Filete de res", 2)),
        key: String = "aviso-$id",
    ) =
        ReadyAlert(
            id = id,
            key = key,
            accountId = account,
            mode = mode,
            tableName = table,
            folio = folio,
            customerName = customer,
            waiterUserId = waiter,
            waiterShortName = waiter.replaceFirstChar { it.uppercase() },
            items = items,
            readyAt = "2026-11-18T20:41:0$id.120Z",
        )

    private fun batch(
        cursor: Long,
        vararg alerts: ReadyAlert,
        active: Boolean = true,
        dayEnds: Long = DAY_ENDS,
        settings: ReadyAlertSettings = ReadyAlertSettings.DEFAULT,
    ) = ReadyAlertsBatch(cursor, active, dayEnds, settings, alerts.toList())

    // --- Texto (A2-D10) ---

    @Test
    fun textoDeMesaConPluralYSinPrecios() {
        val notice = ReadyNotice.of(alert(1))
        assertEquals("Mesa 4 · 2 platillos listos", words.title(notice))
        assertEquals("2 × Filete de res", words.text(notice))
    }

    @Test
    fun textoDeParaLlevarConYSinNombreEnSingular() {
        val named =
            ReadyNotice.of(
                alert(
                    1,
                    mode = OrderMode.TAKEAWAY,
                    table = null,
                    folio = 12,
                    customer = " Juan ",
                    items = listOf(ReadyItem("Hamburguesa", 1)),
                ),
            )
        assertEquals("Para llevar #12 (Juan) · 1 platillo listo", words.title(named))
        assertEquals("1 × Hamburguesa", words.text(named))
        val anonymous =
            ReadyNotice.of(alert(2, mode = OrderMode.TAKEAWAY, table = null, folio = 12))
        assertEquals("Para llevar #12 · 2 platillos listos", words.title(anonymous))
        // Una cuenta de salón sin nombre de mesa (no debería pasar): con su folio.
        assertEquals(
            "Cuenta #31 · 2 platillos listos",
            words.title(ReadyNotice.of(alert(3, table = " "))),
        )
    }

    @Test
    fun variasPartidasSeparadasPorComasYElContadorSuma() {
        val notice =
            ReadyNotice.of(
                alert(
                    1,
                    items = listOf(ReadyItem("Filete · Grande", 2), ReadyItem("Tacos", 1)),
                ),
            )
        assertEquals("Mesa 4 · 3 platillos listos", words.title(notice))
        assertEquals("2 × Filete · Grande, 1 × Tacos", words.text(notice))
    }

    // --- Agrupación por cuenta (§4.4) ---

    @Test
    fun losAvisosDeUnaCuentaSeAcumulanEnUnaSolaNotificacion() {
        val notices =
            ReadyNotice.accumulate(
                listOf(
                    alert(1),
                    alert(2, account = "cuenta-7", table = "Mesa 7"),
                    alert(3, items = listOf(ReadyItem("Filete de res", 1), ReadyItem("Sopa", 1))),
                ),
            ) {
                null
            }
        assertEquals(listOf("cuenta-4", "cuenta-7"), notices.map { it.accountId })
        val table4 = notices.first()
        assertEquals("Mesa 4 · 4 platillos listos", words.title(table4))
        assertEquals("3 × Filete de res, 1 × Sopa", words.text(table4))
        assertEquals(listOf("aviso-1", "aviso-3"), table4.shown)
        assertEquals("Mesa 4, Mesa 7", words.summaryText(notices))
    }

    @Test
    fun loNuevoSeSumaALoQueYaEstaALaVistaYUnAvisoContenidoNoCambiaNada() {
        val visible = ReadyNotice.of(alert(1))
        val again = visible.plus(alert(1))
        assertSame("un aviso que ya contiene no cambia nada", visible, again)
        val notices = ReadyNotice.accumulate(listOf(alert(5))) { visible }
        assertEquals("Mesa 4 · 4 platillos listos", words.title(notices.single()))
        assertEquals(listOf("aviso-1", "aviso-5"), notices.single().shown)
    }

    @Test
    fun alTransferirLaCuentaElAvisoQuedaConElNuevoMesero() {
        val notice = ReadyNotice.of(alert(1, waiter = "ana")).plus(alert(2, waiter = "beto"))
        assertEquals("beto", notice.waiterUserId)
    }

    // --- Sin duplicados ni pérdidas ---

    @Test
    fun unAvisoSeMuestraUnaSolaVezAunqueVuelvaALlegar() = runBlocking {
        val ledger = MemoryLedger()
        val presenter = RecordingPresenter()
        val processor = ReadyAlertsProcessor(ledger, presenter)
        assertEquals(ReadyBatchOutcome.CONTINUE, processor.onBatch(batch(2, alert(1), alert(2))))
        // El mismo lote otra vez (reintento) y un cursor atrasado que repite el 2: nada nuevo.
        processor.onBatch(batch(2, alert(1), alert(2)))
        processor.onBatch(batch(3, alert(2), alert(3)))
        // Repetido dentro del mismo lote: una vez.
        processor.onBatch(batch(4, alert(4), alert(4)))
        assertEquals(listOf(1L, 2L, 3L, 4L), presenter.shown.map { it.id })
        assertEquals(4L, ledger.status()?.cursor)
        // Un proceso nuevo (se reinició) con el mismo registro tampoco los repite.
        val restarted = RecordingPresenter()
        ReadyAlertsProcessor(ledger, restarted).onBatch(batch(4, alert(3), alert(4)))
        assertTrue(restarted.shown.isEmpty())
    }

    @Test
    fun noSeEscribeEnElDispositivoConCadaMensajeSinCambios() = runBlocking {
        val ledger = MemoryLedger()
        val processor = ReadyAlertsProcessor(ledger, RecordingPresenter())
        processor.onBatch(batch(10))
        processor.onBatch(batch(10))
        processor.onBatch(batch(10))
        assertEquals(1, ledger.commits)
        // Avanza el cursor (avisos de otros meseros) o cambia la configuración: se guarda.
        processor.onBatch(batch(11))
        processor.onBatch(batch(11, settings = ReadyAlertSettings(true, false, true)))
        assertEquals(3, ledger.commits)
    }

    @Test
    fun siElProcesoMuereDespuesDeMostrarYAntesDeGuardarNoSePierdeNiSeRepite() = runBlocking {
        val ledger = MemoryLedger()
        // Las notificaciones a la vista sobreviven al proceso (las guarda Android).
        val screen = RecordingPresenter()
        ledger.failNextCommit = true
        val dying = ReadyAlertsProcessor(ledger, screen)
        runCatching { dying.onBatch(batch(1, alert(1))) }
        assertEquals("se mostró antes de guardar", listOf(1L), screen.shown.map { it.id })
        assertEquals("no se guardó", null, ledger.status())
        // El proceso nuevo vuelve a leer desde el cursor guardado (0): el aviso vuelve a llegar,
        // el presentador lo reconoce a la vista y no lo repite; ahora sí queda guardado.
        ReadyAlertsProcessor(ledger, screen).onBatch(batch(1, alert(1)))
        assertEquals(listOf(1L), screen.shown.map { it.id })
        assertEquals(1L, ledger.status()?.cursor)
        assertTrue(ledger.unseen(listOf(alert(1))).isEmpty())
    }

    @Test
    fun trasUnCorteDeRedSeReanudaDesdeElCursorGuardado() = runBlocking {
        val ledger = MemoryLedger()
        val presenter = RecordingPresenter()
        val processor = ReadyAlertsProcessor(ledger, presenter)
        processor.onBatch(batch(7, alert(7)))
        // Durante el corte, cocina marcó «Listo» el 8 y el 9; al volver, Go entrega desde el 7.
        val resumeFrom = ledger.status()!!.cursor
        assertEquals(7L, resumeFrom)
        processor.onBatch(batch(9, alert(8), alert(9)))
        assertEquals(listOf(7L, 8L, 9L), presenter.shown.map { it.id })
    }

    @Test
    fun seDescartaPorLaKeyDeGoNoPorElNumeroQuePuedeRepetirseTrasUnRespaldo() = runBlocking {
        val ledger = MemoryLedger()
        val presenter = RecordingPresenter()
        val processor = ReadyAlertsProcessor(ledger, presenter)
        processor.onBatch(batch(5, alert(5, key = "k-original")))
        // Tras restaurar un respaldo, Go repite el número 5 con otro aviso (otra key): se muestra.
        processor.onBatch(batch(5, alert(5, key = "k-nuevo")))
        // Y la misma key con otro número (cursor distinto) es el mismo aviso: no se repite.
        processor.onBatch(batch(9, alert(9, key = "k-nuevo")))
        assertEquals(listOf("k-original", "k-nuevo"), presenter.shown.map { it.key })
    }

    @Test
    fun inactivoDetieneYAvanzaElCursor() = runBlocking {
        val ledger = MemoryLedger()
        val processor = ReadyAlertsProcessor(ledger, RecordingPresenter())
        assertEquals(ReadyBatchOutcome.INACTIVE, processor.onBatch(batch(40, active = false)))
        assertEquals(40L, ledger.status()?.cursor)
        assertFalse(ledger.status()!!.active)
    }

    @Test
    fun laConsultaPreviaNoMueveElCursorSiEstaActivo() = runBlocking {
        val ledger = MemoryLedger()
        val processor = ReadyAlertsProcessor(ledger, RecordingPresenter())
        processor.onBatch(batch(3, alert(3)))
        processor.onProbe(batch(9, alert(8), alert(9)))
        assertEquals("los avisos los entrega el servicio", 3L, ledger.status()?.cursor)
        assertTrue(ledger.status()!!.active)
        // Apagado en Terminales: el cursor avanza (al activarse no llegan avisos viejos).
        processor.onProbe(
            batch(12, active = false, settings = ReadyAlertSettings(false, true, true)),
        )
        assertEquals(12L, ledger.status()?.cursor)
        assertFalse(ledger.status()!!.settings.enabled)
    }

    // --- Arranque y parada (§4.3, §0.5.5) ---

    @Test
    fun cuandoArrancaElServicio() {
        val now = DAY_ENDS - 3_600_000
        val active = ReadyAlertsStatus(1, true, DAY_ENDS, ReadyAlertSettings.DEFAULT)
        val inactive = active.copy(active = false)
        val disabled = inactive.copy(settings = ReadyAlertSettings(false, true, true))
        assertEquals(
            StartDecision.START,
            ReadyAlertsLifecycle.decide(Trigger.ONLINE_ENTRY, null, now),
        )
        assertEquals(
            StartDecision.START,
            ReadyAlertsLifecycle.decide(Trigger.ONLINE_ENTRY, inactive, now),
        )
        assertEquals(
            "apagado en Terminales: preguntar (pudo volver a activarse)",
            StartDecision.ASK_FIRST,
            ReadyAlertsLifecycle.decide(Trigger.ONLINE_ENTRY, disabled, now),
        )
        assertEquals(StartDecision.START, ReadyAlertsLifecycle.decide(Trigger.OPENED, active, now))
        assertEquals(
            StartDecision.ASK_FIRST,
            ReadyAlertsLifecycle.decide(Trigger.OPENED, null, now),
        )
        assertEquals(
            StartDecision.ASK_FIRST,
            ReadyAlertsLifecycle.decide(Trigger.OPENED, inactive, now),
        )
        assertEquals(
            "terminó el día: preguntar",
            StartDecision.ASK_FIRST,
            ReadyAlertsLifecycle.decide(Trigger.OPENED, active, DAY_ENDS),
        )
        assertEquals(StartDecision.START, ReadyAlertsLifecycle.decide(Trigger.BOOT, active, now))
        assertEquals(StartDecision.NONE, ReadyAlertsLifecycle.decide(Trigger.BOOT, null, now))
        assertEquals(StartDecision.NONE, ReadyAlertsLifecycle.decide(Trigger.BOOT, inactive, now))
        assertEquals(
            StartDecision.NONE,
            ReadyAlertsLifecycle.decide(Trigger.BOOT, active, DAY_ENDS),
        )
    }

    @Test
    fun alTerminarElDiaVuelveAPreguntarSinDetenerse() {
        val after = ReadyAlertsLifecycle.DAY_CHECK_AFTER_MILLIS
        assertEquals(60_000L + after, ReadyAlertsLifecycle.dayCheckIn(DAY_ENDS, DAY_ENDS - 60_000))
        assertEquals(1L, ReadyAlertsLifecycle.dayCheckIn(DAY_ENDS, DAY_ENDS + after - 1))
        // Ya pasó (Go sigue con el mismo fin de día): no se pregunta en bucle; lo dirá el canal.
        assertNull(ReadyAlertsLifecycle.dayCheckIn(DAY_ENDS, DAY_ENDS + after))
        assertNull(ReadyAlertsLifecycle.dayCheckIn(null, DAY_ENDS))
    }

    @Test
    fun pasadoElFinDelDiaSinGoYSinNadieUsandoloSeDetiene() {
        val margin = ReadyAlertsLifecycle.DAY_END_STOP_MILLIS
        fun over(lastReached: Long?, now: Long, inUse: Boolean = false) =
            ReadyAlertsLifecycle.dayOverWithoutGo(DAY_ENDS, lastReached, now, inUse)
        // A2-D12: diez minutos después del fin del día, sin haber alcanzado a Go desde entonces.
        assertTrue(over(lastReached = null, now = DAY_ENDS + margin))
        assertTrue(over(lastReached = DAY_ENDS - 1, now = DAY_ENDS + margin + 60_000))
        // Antes del margen, sigue reintentando.
        assertFalse(over(lastReached = null, now = DAY_ENDS + margin - 1))
        // Go respondió después del fin del día: él decide (active).
        assertFalse(over(lastReached = DAY_ENDS + 1, now = DAY_ENDS + margin + 60_000))
        // Un mesero lo está usando: no se detiene aunque Go no responda.
        assertFalse(over(lastReached = null, now = DAY_ENDS + 3_600_000, inUse = true))
        // Sin fin de día conocido, no hay de dónde contar.
        assertFalse(ReadyAlertsLifecycle.dayOverWithoutGo(null, null, DAY_ENDS + margin, false))
    }

    @Test
    fun queHacerAnteCadaError() {
        assertEquals(
            ErrorAction.STOP_AND_REPORT,
            ReadyAlertsLifecycle.onError(AppError.DeviceRevoked(null, null)),
        )
        // Al abrir o leer, un 401 DEVICE_REQUIRED es como en las demás rutas (se avisa a la
        // entrada).
        assertEquals(
            ErrorAction.STOP_AND_REPORT,
            ReadyAlertsLifecycle.onError(
                AppError.DeviceNotRecognized("DEVICE_REQUIRED", null, null),
            ),
        )
        assertEquals(
            ErrorAction.STOP_AND_REPORT,
            ReadyAlertsLifecycle.onError(AppError.ServerIdentityChanged),
        )
        assertEquals(
            ErrorAction.STOP_AND_REPORT,
            ReadyAlertsLifecycle.onError(
                AppError.UpdateRequired("CLIENT_UPDATE_REQUIRED", "0.3.0", null, null, null),
            ),
        )
        assertEquals(
            ErrorAction.WAIT_AND_READ,
            ReadyAlertsLifecycle.onError(
                AppError.TryLater("READY_ALERTS_CONNECTION_LIMIT", null, null),
            ),
        )
        assertEquals(
            ErrorAction.STOP,
            ReadyAlertsLifecycle.onError(
                AppError.Rejected(403, "READY_ALERTS_DEVICE_NOT_ALLOWED", null, null),
            ),
        )
        assertEquals(ErrorAction.RETRY, ReadyAlertsLifecycle.onError(AppError.Unreachable))
        assertEquals(
            ErrorAction.RETRY,
            ReadyAlertsLifecycle.onError(AppError.ServerProblem(503, null, null, null)),
        )
    }

    @Test
    fun esperaCrecienteDeDosATreintaSegundos() {
        val backoff = ReadyAlertsLifecycle.backoff()
        assertEquals(
            listOf(0L, 2_000L, 4_000L, 8_000L, 16_000L, 30_000L, 30_000L),
            (0..6).map(backoff::delayFor),
        )
    }

    // --- Canales (A2-D7) ---

    @Test
    fun unCanalPorCombinacionDeSonidoYVibracion() {
        assertEquals(
            "listos_sonido_vibracion",
            ReadyChannels.readyChannel(ReadyAlertSettings(true, true, true)),
        )
        assertEquals(
            "listos_sonido",
            ReadyChannels.readyChannel(ReadyAlertSettings(true, true, false)),
        )
        assertEquals(
            "listos_vibracion",
            ReadyChannels.readyChannel(ReadyAlertSettings(true, false, true)),
        )
        assertEquals(
            "listos_silencio",
            ReadyChannels.readyChannel(ReadyAlertSettings(true, false, false)),
        )
        assertEquals(
            listOf("listos_sonido_vibracion", "listos_sonido", "listos_vibracion"),
            ReadyChannels.stale(ReadyAlertSettings(true, false, false)),
        )
        assertNull(ReadyChannels.READY.firstOrNull { it == ReadyChannels.CONNECTED })
    }

    /** El registro en memoria (Room en `core:data`), con una falla a pedido al guardar. */
    private class MemoryLedger : ReadyAlertsLedger {
        private var stored: ReadyAlertsStatus? = null
        private val seen = mutableSetOf<String>()
        var commits = 0
        var failNextCommit = false

        override suspend fun status(): ReadyAlertsStatus? = stored

        override suspend fun unseen(alerts: List<ReadyAlert>): List<ReadyAlert> = alerts.filter {
            it.key !in seen
        }

        override suspend fun commit(batch: ReadyAlertsBatch, shown: List<ReadyAlert>) {
            if (failNextCommit) {
                failNextCommit = false
                error("el proceso murió")
            }
            commits += 1
            shown.forEach { seen.add(it.key) }
            stored = batch.status
        }

        override suspend fun rememberStatus(batch: ReadyAlertsBatch) {
            stored = batch.status.copy(cursor = stored?.cursor ?: 0L)
        }

        override suspend fun erase() {
            stored = null
            seen.clear()
        }
    }

    /**
     * Lo que haría la notificación de cada cuenta: recuerda lo que tiene a la vista y no vuelve a
     * mostrar un aviso que ya contiene.
     */
    private class RecordingPresenter : ReadyAlertPresenter {
        val shown = mutableListOf<ReadyAlert>()
        private val visible = mutableSetOf<String>()

        override suspend fun apply(settings: ReadyAlertSettings) = Unit

        override suspend fun present(alerts: List<ReadyAlert>) {
            for (alert in alerts) {
                if (visible.add(alert.key)) {
                    shown.add(alert)
                }
            }
        }
    }

    private companion object {
        const val DAY_ENDS = 1_763_550_000_000L
    }
}
