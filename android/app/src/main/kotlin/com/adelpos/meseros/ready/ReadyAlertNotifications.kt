package com.adelpos.meseros.ready

import android.app.Notification
import android.app.NotificationChannel
import android.app.NotificationManager
import android.app.PendingIntent
import android.content.Context
import android.content.Intent
import android.media.AudioAttributes
import android.os.Bundle
import android.provider.Settings
import android.service.notification.StatusBarNotification
import androidx.core.app.NotificationCompat
import androidx.core.app.NotificationManagerCompat
import com.adelpos.meseros.MainActivity
import com.adelpos.meseros.R
import com.adelpos.meseros.core.data.ready.ReadyOngoing
import com.adelpos.meseros.core.domain.ready.ReadyAlert
import com.adelpos.meseros.core.domain.ready.ReadyAlertPresenter
import com.adelpos.meseros.core.domain.ready.ReadyAlertSettings
import com.adelpos.meseros.core.domain.ready.ReadyAlertWords
import com.adelpos.meseros.core.domain.ready.ReadyChannels
import com.adelpos.meseros.core.domain.ready.ReadyItem
import com.adelpos.meseros.core.domain.ready.ReadyNotice
import com.adelpos.meseros.core.domain.waiter.OrderMode

/**
 * Lo que necesita el presentador del aviso dentro de la aplicación (§4.4: «si el mesero ya está
 * viendo esa cuenta, se muestra un aviso dentro de la aplicación en lugar de la notificación del
 * sistema»).
 */
interface InAppAlerts {
    /** La aplicación está a la vista con la captura de [accountId] abierta. */
    fun isViewing(accountId: String): Boolean

    fun show(notice: ReadyNotice)
}

/**
 * Las notificaciones del aviso de platillos listos (A2-D7 y §4.4). Los canales existen desde la API
 * 26 (el `minSdk`), así que no hay otro camino.
 *
 * - «AD-EL POS conectado» (`IMPORTANCE_LOW`, sin sonido ni globo): la notificación permanente del
 *   servicio; al tocarla abre la ayuda.
 * - **Un solo canal visible «Platillos listos»** (`IMPORTANCE_HIGH`, globo emergente) cuyo
 *   identificador depende de la combinación de sonido y vibración de Terminales ([ReadyChannels]):
 *   al cambiar la combinación se crea el nuevo y se borran los otros. La aplicación nunca reproduce
 *   el sonido ella misma (Android 17 endurece el audio en segundo plano): lo hace el canal.
 * - **Una notificación por cuenta** (etiqueta = la cuenta) que se reemplaza con el acumulado
 *   mientras siga a la vista, y un resumen del grupo con varias cuentas. Sin precios. Se ve también
 *   en la pantalla de bloqueo (solo mesa o pedido y platillos, §0-B).
 * - Cada notificación guarda en sus extras qué avisos contiene: si el proceso murió después de
 *   mostrar un aviso y antes de guardarlo como avisado, al volver a llegar se reconoce aquí y no se
 *   repite.
 * - Sin el permiso de notificaciones (Android 13+) Android no las muestra y no falla: la ayuda lo
 *   dice.
 */
class ReadyAlertNotifications(
    context: Context,
    private val inApp: InAppAlerts,
) : ReadyAlertPresenter {
    private val appContext: Context = context.applicationContext
    private val manager: NotificationManager =
        appContext.getSystemService(NotificationManager::class.java)
    private val words: ReadyAlertWords = wordsFrom(appContext)

    @Volatile private var applied: ReadyAlertSettings? = null

    /** El canal «Platillos listos» vigente (el de la última configuración aplicada). */
    val readyChannel: String
        get() = ReadyChannels.readyChannel(applied ?: ReadyAlertSettings.DEFAULT)

    /** Crea el canal de la notificación permanente (no hace nada si ya existe). */
    fun ensureConnectedChannel() {
        val channel =
            NotificationChannel(
                ReadyChannels.CONNECTED,
                appContext.getString(R.string.ready_channel_connected_name),
                NotificationManager.IMPORTANCE_LOW,
            )
        channel.description = appContext.getString(R.string.ready_channel_connected_description)
        channel.setShowBadge(false)
        channel.enableVibration(false)
        channel.setSound(null, null)
        manager.createNotificationChannel(channel)
    }

    override suspend fun apply(settings: ReadyAlertSettings) {
        if (settings == applied) {
            return
        }
        ensureReadyChannel(settings)
        applied = settings
    }

    /**
     * Crea el canal «Platillos listos» de [settings] y borra los de las otras combinaciones: en los
     * Ajustes de Android queda uno solo con ese nombre (A2-D7).
     */
    fun ensureReadyChannel(settings: ReadyAlertSettings) {
        val channel =
            NotificationChannel(
                ReadyChannels.readyChannel(settings),
                appContext.getString(R.string.ready_channel_ready_name),
                NotificationManager.IMPORTANCE_HIGH,
            )
        channel.description = appContext.getString(R.string.ready_channel_ready_description)
        channel.setShowBadge(true)
        channel.lockscreenVisibility = Notification.VISIBILITY_PUBLIC
        if (settings.sound) {
            channel.setSound(
                Settings.System.DEFAULT_NOTIFICATION_URI,
                AudioAttributes.Builder()
                    .setUsage(AudioAttributes.USAGE_NOTIFICATION)
                    .setContentType(AudioAttributes.CONTENT_TYPE_SONIFICATION)
                    .build(),
            )
        } else {
            channel.setSound(null, null)
        }
        channel.enableVibration(settings.vibration)
        if (settings.vibration) {
            channel.vibrationPattern = VIBRATION_PATTERN
        }
        manager.createNotificationChannel(channel)
        for (stale in ReadyChannels.stale(settings)) {
            manager.deleteNotificationChannel(stale)
        }
    }

    override suspend fun present(alerts: List<ReadyAlert>) {
        // Dentro de la aplicación: solo lo nuevo de la cuenta que el mesero está viendo.
        val inAppNotices =
            ReadyNotice.accumulate(alerts.filter { inApp.isViewing(it.accountId) }) { null }
        for (notice in inAppNotices) {
            inApp.show(notice)
        }
        val forSystem = alerts.filterNot { inApp.isViewing(it.accountId) }
        if (forSystem.isEmpty()) {
            return
        }
        val active = activeNotices()
        val notices = ReadyNotice.accumulate(forSystem) { accountId -> active[accountId] }
        val channel = readyChannel
        for (notice in notices) {
            // Ya estaba a la vista con estos mismos avisos (el proceso murió antes de guardarlos):
            // no se vuelve a sonar ni a vibrar.
            if (active[notice.accountId]?.shown == notice.shown) {
                continue
            }
            manager.notify(notice.accountId, ALERT_ID, alertNotification(notice, channel))
        }
        updateSummary(channel)
    }

    /** Quita la notificación de una cuenta (el mesero ya la está viendo en la aplicación). */
    fun cancelAccount(accountId: String) {
        if (active().any { it.tag == accountId && it.id == ALERT_ID }) {
            manager.cancel(accountId, ALERT_ID)
            updateSummary(readyChannel)
        }
    }

    /** Quita todos los avisos (borrar los datos del dispositivo). */
    fun cancelAlerts() {
        for (notification in active()) {
            if (notification.id == ALERT_ID || notification.id == SUMMARY_ID) {
                manager.cancel(notification.tag, notification.id)
            }
        }
    }

    /** La notificación permanente del servicio, según cómo va el canal. */
    fun ongoing(text: ReadyOngoing): Notification =
        NotificationCompat.Builder(appContext, ReadyChannels.CONNECTED)
            .setSmallIcon(R.drawable.ic_stat_connected)
            .setContentTitle(appContext.getString(R.string.ready_ongoing_title))
            .setContentText(
                appContext.getString(
                    when (text) {
                        ReadyOngoing.CONNECTING -> R.string.ready_ongoing_connecting
                        ReadyOngoing.CONNECTED -> R.string.ready_ongoing_connected
                        ReadyOngoing.WAITING -> R.string.ready_ongoing_waiting
                    },
                ),
            )
            .setOngoing(true)
            .setOnlyAlertOnce(true)
            .setSilent(true)
            .setShowWhen(false)
            .setCategory(NotificationCompat.CATEGORY_SERVICE)
            .setPriority(NotificationCompat.PRIORITY_LOW)
            .setForegroundServiceBehavior(NotificationCompat.FOREGROUND_SERVICE_IMMEDIATE)
            .setContentIntent(helpIntent())
            .build()

    /**
     * Si Android muestra las notificaciones de la aplicación y el canal vigente no está apagado.
     */
    fun alertsVisible(): Boolean {
        if (!NotificationManagerCompat.from(appContext).areNotificationsEnabled()) {
            return false
        }
        val channel = manager.getNotificationChannel(readyChannel) ?: return true
        return channel.importance != NotificationManager.IMPORTANCE_NONE
    }

    private fun alertNotification(notice: ReadyNotice, channel: String): Notification {
        val text = words.text(notice)
        return NotificationCompat.Builder(appContext, channel)
            .setSmallIcon(R.drawable.ic_stat_ready)
            .setContentTitle(words.title(notice))
            .setContentText(text)
            .setStyle(NotificationCompat.BigTextStyle().bigText(text))
            .setCategory(NotificationCompat.CATEGORY_EVENT)
            .setPriority(NotificationCompat.PRIORITY_HIGH)
            .setVisibility(NotificationCompat.VISIBILITY_PUBLIC)
            .setGroup(GROUP)
            .setGroupAlertBehavior(NotificationCompat.GROUP_ALERT_CHILDREN)
            .setAutoCancel(true)
            .setShowWhen(true)
            .setWhen(System.currentTimeMillis())
            .setContentIntent(openIntent(notice))
            .addExtras(notice.toExtras())
            .build()
    }

    /** Con dos o más cuentas a la vista, el resumen del grupo (sin sonido); con menos, ninguno. */
    private fun updateSummary(channel: String) {
        val notices = activeNotices().values.toList()
        if (notices.size < 2) {
            manager.cancel(null, SUMMARY_ID)
            return
        }
        val style = NotificationCompat.InboxStyle()
        for (notice in notices) {
            style.addLine(words.title(notice))
        }
        val summary =
            NotificationCompat.Builder(appContext, channel)
                .setSmallIcon(R.drawable.ic_stat_ready)
                .setContentTitle(words.summaryTitle)
                .setContentText(words.summaryText(notices))
                .setStyle(style)
                .setGroup(GROUP)
                .setGroupSummary(true)
                .setGroupAlertBehavior(NotificationCompat.GROUP_ALERT_CHILDREN)
                .setOnlyAlertOnce(true)
                .setAutoCancel(true)
                .setVisibility(NotificationCompat.VISIBILITY_PUBLIC)
                .setContentIntent(appIntent())
                .build()
        manager.notify(null, SUMMARY_ID, summary)
    }

    private fun active(): List<StatusBarNotification> =
        try {
            manager.activeNotifications.toList()
        } catch (_: RuntimeException) {
            // Algunos sistemas fallan al leerlas durante un cambio de usuario: se trata como nada.
            emptyList()
        }

    /** Las notificaciones de cuenta a la vista, con lo que contienen (de sus extras). */
    private fun activeNotices(): Map<String, ReadyNotice> =
        active()
            .filter { it.id == ALERT_ID && it.tag != null }
            .mapNotNull { notification -> notification.notification.extras.toNotice() }
            .associateBy { it.accountId }

    private fun openIntent(notice: ReadyNotice): PendingIntent {
        val intent =
            Intent(appContext, MainActivity::class.java)
                .setAction(ReadyAlertsIntents.ACTION_OPEN_ALERT)
                .putExtra(ReadyAlertsIntents.EXTRA_ACCOUNT_ID, notice.accountId)
                .putExtra(ReadyAlertsIntents.EXTRA_WAITER_ID, notice.waiterUserId)
                .putExtra(ReadyAlertsIntents.EXTRA_MODE, notice.mode.wireName)
                .addFlags(ACTIVITY_FLAGS)
        return PendingIntent.getActivity(
            appContext,
            notice.accountId.hashCode(),
            intent,
            PendingIntent.FLAG_IMMUTABLE or PendingIntent.FLAG_UPDATE_CURRENT,
        )
    }

    private fun helpIntent(): PendingIntent {
        val intent =
            Intent(appContext, MainActivity::class.java)
                .setAction(ReadyAlertsIntents.ACTION_OPEN_HELP)
                .addFlags(ACTIVITY_FLAGS)
        return PendingIntent.getActivity(
            appContext,
            HELP_REQUEST,
            intent,
            PendingIntent.FLAG_IMMUTABLE or PendingIntent.FLAG_UPDATE_CURRENT,
        )
    }

    private fun appIntent(): PendingIntent {
        val intent = Intent(appContext, MainActivity::class.java).addFlags(ACTIVITY_FLAGS)
        return PendingIntent.getActivity(
            appContext,
            APP_REQUEST,
            intent,
            PendingIntent.FLAG_IMMUTABLE or PendingIntent.FLAG_UPDATE_CURRENT,
        )
    }

    companion object {
        /** Identificador de la notificación permanente del servicio. */
        const val ONGOING_ID = 4_100

        /** Identificador de cada notificación de cuenta (la etiqueta es la cuenta). */
        const val ALERT_ID = 4_101

        /** Identificador del resumen del grupo. */
        const val SUMMARY_ID = 4_102

        const val GROUP = "com.adelpos.meseros.platillos-listos"

        private const val HELP_REQUEST = 1
        private const val APP_REQUEST = 2
        private val VIBRATION_PATTERN = longArrayOf(0, 350, 200, 350)

        private const val ACTIVITY_FLAGS =
            Intent.FLAG_ACTIVITY_NEW_TASK or
                Intent.FLAG_ACTIVITY_CLEAR_TOP or
                Intent.FLAG_ACTIVITY_SINGLE_TOP

        private const val KEY_ACCOUNT = "adelpos.ready.account"
        private const val KEY_MODE = "adelpos.ready.mode"
        private const val KEY_TABLE = "adelpos.ready.table"
        private const val KEY_FOLIO = "adelpos.ready.folio"
        private const val KEY_CUSTOMER = "adelpos.ready.customer"
        private const val KEY_WAITER = "adelpos.ready.waiter"
        private const val KEY_ITEM_NAMES = "adelpos.ready.items.names"
        private const val KEY_ITEM_QUANTITIES = "adelpos.ready.items.quantities"
        private const val KEY_SHOWN = "adelpos.ready.shown.keys"

        /** Los textos del aviso con las plantillas de `strings.xml` (A2-D10). */
        fun wordsFrom(context: Context): ReadyAlertWords =
            ReadyAlertWords(
                dishOne = context.getString(R.string.ready_alert_dish_one),
                dishOther = context.getString(R.string.ready_alert_dish_other),
                takeaway = context.getString(R.string.ready_alert_takeaway),
                takeawayNamed = context.getString(R.string.ready_alert_takeaway_named),
                account = context.getString(R.string.ready_alert_account),
                title = context.getString(R.string.ready_alert_title),
                item = context.getString(R.string.ready_alert_item),
                itemSeparator = context.getString(R.string.ready_alert_item_separator),
                summaryTitle = context.getString(R.string.ready_alert_summary_title),
                summarySeparator = context.getString(R.string.ready_alert_summary_separator),
            )

        private fun ReadyNotice.toExtras(): Bundle {
            val extras = Bundle()
            extras.putString(KEY_ACCOUNT, accountId)
            extras.putString(KEY_MODE, mode.wireName)
            extras.putString(KEY_TABLE, tableName)
            extras.putLong(KEY_FOLIO, folio)
            extras.putString(KEY_CUSTOMER, customerName)
            extras.putString(KEY_WAITER, waiterUserId)
            extras.putStringArray(KEY_ITEM_NAMES, items.map { it.productName }.toTypedArray())
            extras.putIntArray(KEY_ITEM_QUANTITIES, items.map { it.quantity }.toIntArray())
            extras.putStringArray(KEY_SHOWN, shown.toTypedArray())
            return extras
        }

        private fun Bundle.toNotice(): ReadyNotice? {
            val account = getString(KEY_ACCOUNT) ?: return null
            val names = getStringArray(KEY_ITEM_NAMES) ?: return null
            val quantities = getIntArray(KEY_ITEM_QUANTITIES) ?: return null
            val shown = getStringArray(KEY_SHOWN) ?: return null
            if (names.size != quantities.size) {
                return null
            }
            return ReadyNotice(
                accountId = account,
                mode = OrderMode.fromWireName(getString(KEY_MODE)),
                tableName = getString(KEY_TABLE),
                folio = getLong(KEY_FOLIO),
                customerName = getString(KEY_CUSTOMER).orEmpty(),
                waiterUserId = getString(KEY_WAITER).orEmpty(),
                items = names.indices.map { ReadyItem(names[it], quantities[it]) },
                shown = shown.toList(),
            )
        }
    }
}

/** Las acciones con que las notificaciones abren la aplicación. */
object ReadyAlertsIntents {
    const val ACTION_OPEN_ALERT = "com.adelpos.meseros.action.ABRIR_AVISO"
    const val ACTION_OPEN_HELP = "com.adelpos.meseros.action.AYUDA_AVISOS"
    const val EXTRA_ACCOUNT_ID = "account_id"
    const val EXTRA_WAITER_ID = "waiter_user_id"
    const val EXTRA_MODE = "mode"
}
