package com.adelpos.meseros

import androidx.activity.compose.setContent
import androidx.compose.foundation.layout.Row
import androidx.compose.runtime.Composable
import androidx.compose.ui.test.assertHasClickAction
import androidx.compose.ui.test.assertIsEnabled
import androidx.compose.ui.test.getBoundsInRoot
import androidx.compose.ui.test.hasTestTag
import androidx.compose.ui.test.junit4.v2.createAndroidComposeRule
import androidx.compose.ui.test.onNodeWithContentDescription
import androidx.compose.ui.test.onNodeWithTag
import androidx.compose.ui.test.onNodeWithText
import androidx.compose.ui.test.performClick
import androidx.compose.ui.unit.dp
import androidx.test.ext.junit.runners.AndroidJUnit4
import com.adelpos.core.model.error.LicenseMessages
import com.adelpos.core.model.license.ClockNotice
import com.adelpos.core.model.license.LicenseNotices
import com.adelpos.core.network.license.LicenseClockDto
import com.adelpos.core.network.license.LicenseSummaryDto
import com.adelpos.core.ui.theme.AdelTheme
import com.adelpos.meseros.core.capture.pending.ReviewScreen
import com.adelpos.meseros.core.capture.pending.TAG_REVIEW_ACCEPT
import com.adelpos.meseros.core.capture.pending.TAG_REVIEW_MESSAGE
import com.adelpos.meseros.core.capture.pending.TAG_REVIEW_NEW_ACCOUNT
import com.adelpos.meseros.core.capture.pending.TAG_REVIEW_RESEND
import com.adelpos.meseros.core.capture.pending.TAG_REVIEW_SEND
import com.adelpos.meseros.core.domain.catalog.Customization
import com.adelpos.meseros.core.domain.catalog.LineSelection
import com.adelpos.meseros.core.domain.money.Money
import com.adelpos.meseros.core.domain.waiter.DraftLine
import com.adelpos.meseros.core.domain.waiter.OrderDestination
import com.adelpos.meseros.core.domain.waiter.OrderLine
import com.adelpos.meseros.core.domain.waiter.OrderReview
import com.adelpos.meseros.core.domain.waiter.OrderState
import com.adelpos.meseros.core.domain.waiter.PendingOrder
import com.adelpos.meseros.core.domain.waiter.ReviewReason
import com.adelpos.meseros.core.ui.components.LicenseClockBanner
import com.adelpos.meseros.core.ui.components.LicenseGraceBadge
import com.adelpos.meseros.core.ui.components.TAG_LICENSE_CLOCK_BANNER
import com.adelpos.meseros.core.ui.components.TAG_LICENSE_CLOCK_DETAIL
import com.adelpos.meseros.core.ui.components.TAG_LICENSE_CLOCK_TEXT
import com.adelpos.meseros.core.ui.components.TAG_LICENSE_GRACE_BADGE
import com.adelpos.meseros.core.ui.components.TAG_LICENSE_GRACE_DETAIL
import com.adelpos.meseros.core.ui.components.TAG_LICENSE_GRACE_TEXT
import org.junit.After
import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Rule
import org.junit.Test
import org.junit.runner.RunWith

/**
 * Etapa L1 (§6.2 y §8) con datos fijos, en tema claro y oscuro: el aviso fijo de la hora con sus
 * dos fechas, el aviso discreto de la tolerancia (con texto o solo el ícono, siempre con 48 dp para
 * tocarlo y el texto completo al tocarlo), «Requiere tu revisión» de un pedido que la licencia, un
 * cupo o la hora no permitieron (con «Enviar de nuevo» y sin «Aceptar los cambios»), y los dos
 * avisos en la barra de la aplicación con lo que dijo Go.
 */
@RunWith(AndroidJUnit4::class)
class StageL1ScreensTest {
    @get:Rule val compose = createAndroidComposeRule<MainActivity>()

    private val license
        get() = TestDevice.app.container.licenseStatus

    private fun show(dark: Boolean, content: @Composable () -> Unit) {
        compose.activity.runOnUiThread {
            compose.activity.setContent { AdelTheme(darkTheme = dark) { content() } }
        }
        compose.waitForIdle()
    }

    @After
    fun forget() {
        license.forget()
    }

    @Test
    fun avisoFijoDeLaHoraConSusFechasEnAmbosTemas() {
        val notice =
            ClockNotice(
                text = LicenseMessages.CLOCK_INVALID,
                detail =
                    LicenseMessages.clockDetail(
                        "lunes 23 nov 2026, 10:15",
                        "lunes 23 nov 2026, 12:40",
                    ),
            )
        for (dark in listOf(false, true)) {
            show(dark) { LicenseClockBanner(notice) }
            compose.onNodeWithTag(TAG_LICENSE_CLOCK_BANNER).assertExists()
            compose.onNodeWithTag(TAG_LICENSE_CLOCK_TEXT, useUnmergedTree = true).assertExists()
            compose
                .onNodeWithText(LicenseMessages.CLOCK_INVALID, useUnmergedTree = true)
                .assertExists()
            compose
                .onNodeWithText(
                    "La computadora principal tiene la hora lunes 23 nov 2026, 10:15 y la " +
                        "última actividad registrada fue el lunes 23 nov 2026, 12:40.",
                    useUnmergedTree = true,
                )
                .assertExists()
        }
        // Sin las fechas, solo la causa.
        show(dark = false) { LicenseClockBanner(notice.copy(detail = null)) }
        compose.onNodeWithTag(TAG_LICENSE_CLOCK_DETAIL, useUnmergedTree = true).assertDoesNotExist()
    }

    @Test
    fun avisoDiscretoDeLaToleranciaConTextoOSoloElIcono() {
        val notice =
            LicenseNotices.grace(
                summaryOf(LicenseSummaryDto(state = "grace", graceDaysRemaining = 3)),
            )!!
        show(dark = false) { Row { LicenseGraceBadge(notice, showText = true) } }
        compose.onNodeWithTag(TAG_LICENSE_GRACE_TEXT, useUnmergedTree = true).assertExists()
        compose.onNodeWithText("Licencia: quedan 3 días", useUnmergedTree = true).assertExists()
        val badge = compose.onNodeWithTag(TAG_LICENSE_GRACE_BADGE)
        badge.assertHasClickAction()
        val bounds = badge.getBoundsInRoot()
        assertTrue("48 dp de alto", bounds.bottom - bounds.top >= 48.dp)
        badge.performClick()
        compose.onNodeWithTag(TAG_LICENSE_GRACE_DETAIL).assertExists()
        compose.onNodeWithText(LicenseMessages.graceLong(3)).assertExists()
        compose.onNodeWithText("Entendido").performClick()
        compose.onNodeWithTag(TAG_LICENSE_GRACE_DETAIL).assertDoesNotExist()

        // Teléfono en vertical: solo el ícono, con el texto para los lectores de pantalla.
        show(dark = true) { Row { LicenseGraceBadge(notice, showText = false) } }
        compose.onNodeWithTag(TAG_LICENSE_GRACE_TEXT, useUnmergedTree = true).assertDoesNotExist()
        compose.onNodeWithContentDescription("Licencia: quedan 3 días").assertExists()
        val compact = compose.onNodeWithTag(TAG_LICENSE_GRACE_BADGE).getBoundsInRoot()
        assertTrue("48 dp de ancho", compact.right - compact.left >= 48.dp)
    }

    @Test
    fun revisionDeUnPedidoQueNoPermitioLaLicencia() {
        var resent = 0
        for (dark in listOf(false, true)) {
            show(dark) {
                ReviewScreen(
                    order = order(),
                    working = false,
                    onAccept = {},
                    onResend = { resent++ },
                    onRemoveLine = {},
                    onNewAccount = {},
                    onBackToCapture = {},
                    onDiscard = {},
                )
            }
            compose
                .onNodeWithText(
                    "La computadora principal no aceptó el pedido. Sigue guardado en este " +
                        "dispositivo: cuando se resuelva, envíalo de nuevo.",
                )
                .assertExists()
            compose.onNodeWithTag(TAG_REVIEW_MESSAGE).assertExists()
            compose.onNodeWithText(SEAT_MESSAGE).assertExists()
            compose.onNodeWithTag(TAG_REVIEW_ACCEPT).assertDoesNotExist()
            compose.onNodeWithTag(TAG_REVIEW_SEND).assertDoesNotExist()
            compose.onNodeWithTag(TAG_REVIEW_NEW_ACCOUNT).assertDoesNotExist()
            compose.onNodeWithTag(TAG_REVIEW_RESEND).assertIsEnabled().performClick()
        }
        assertEquals(2, resent)
    }

    /** Los dos avisos en la barra de la aplicación, con lo último que dijo Go. */
    @Test
    fun laBarraDeLaAplicacionMuestraLosAvisosDeGo() {
        compose.waitForIdle()
        compose.onNodeWithTag(TAG_LICENSE_CLOCK_BANNER).assertDoesNotExist()
        compose.onNodeWithTag(TAG_LICENSE_GRACE_BADGE).assertDoesNotExist()
        compose.runOnIdle {
            license.apply(
                LicenseSummaryDto(
                    state = "grace",
                    graceDaysRemaining = 2,
                    clock =
                        LicenseClockDto(
                            blocked = true,
                            localTimeText = "lunes 23 nov 2026, 10:15",
                            lastActivityTimeText = "lunes 23 nov 2026, 12:40",
                        ),
                ),
            )
        }
        compose.waitUntil(timeoutMillis = 10_000) {
            compose
                .onAllNodes(
                    hasTestTag(TAG_LICENSE_CLOCK_BANNER),
                )
                .fetchSemanticsNodes()
                .isNotEmpty()
        }
        compose.onNodeWithTag(TAG_LICENSE_GRACE_BADGE).assertExists()
        // Go corrigió la hora y la licencia volvió a estar vigente: los dos se quitan.
        compose.runOnIdle {
            license.apply(LicenseSummaryDto(state = "active", clock = LicenseClockDto(false)))
        }
        compose.waitUntil(timeoutMillis = 10_000) {
            compose
                .onAllNodes(
                    hasTestTag(TAG_LICENSE_CLOCK_BANNER),
                )
                .fetchSemanticsNodes()
                .isEmpty()
        }
        compose.onNodeWithTag(TAG_LICENSE_GRACE_BADGE).assertDoesNotExist()
    }

    private fun summaryOf(dto: LicenseSummaryDto) = license.run {
        apply(dto)
        summary.value
    }

    private fun order() =
        PendingOrder(
            operationId = "op-1",
            waiterUserId = "ana",
            waiterName = "Ana",
            state = OrderState.NEEDS_REVIEW,
            destination = OrderDestination("t1", "Mesa 4", null, null, emptyList()),
            lines =
                listOf(
                    OrderLine(
                        DraftLine("r1", LineSelection("cafe", null, 1, Customization.ORIGINAL)),
                        "Café de olla",
                        Money("30.00"),
                    ),
                ),
            replaces = emptyList(),
            catalogVersion = 41,
            catalogEpoch = null,
            capturedAtMillis = 1_000,
            confirmation = null,
            review =
                OrderReview(
                    reason = ReviewReason.NOT_ALLOWED,
                    message = SEAT_MESSAGE,
                    changes = emptyList(),
                    catalogVersion = null,
                ),
            attempts = 1,
            lastError = null,
            credentialRejected = false,
            onlineConfirm = null,
            receivedAccountId = null,
            receivedAtMillis = null,
            updatedAtMillis = 1_000,
        )

    private companion object {
        const val SEAT_MESSAGE =
            "No hay cupo de comandero libre (3 de 3 en uso). Pide que se cierre la sesión de " +
                "otra terminal."
    }
}
