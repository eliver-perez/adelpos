package com.adelpos.meseros.core.ui.components

import androidx.compose.ui.unit.dp
import com.adelpos.core.ui.theme.AdelDimens
import org.junit.Assert.assertEquals
import org.junit.Test

/**
 * Etapa A3 (§4, teléfono horizontal): el alto de las teclas del PIN para que las cuatro filas
 * quepan, entre el mínimo táctil y el alto normal.
 */
class PinPadSizingTest {
    @Test
    fun enElTelefonoHorizontalLasCuatroFilasCaben() {
        // ADEL_A1_Phone_API33 en horizontal: ≈ 267 dp de alto para el teclado (411 dp menos la
        // barra de estado, la barra superior, la de gestos y los márgenes).
        val key = AdelPinPadDefaults.keyHeightToFit(267.dp)
        assertEquals(60.75f, key.value, 0.01f)
        val rows = key * 4 + AdelPinPadDefaults.CompactGap * 3
        assertEquals(267f, rows.value, 0.01f)
    }

    @Test
    fun conAltoDeSobraNoPasaDelAltoNormal() {
        assertEquals(AdelPinPadDefaults.KeyHeight, AdelPinPadDefaults.keyHeightToFit(800.dp))
    }

    @Test
    fun nuncaMenosQueElMinimoTactil() {
        // Una ventana muy baja: el teclado se desliza en lugar de tener teclas de menos de 48 dp.
        assertEquals(AdelDimens.MinTouchTarget, AdelPinPadDefaults.keyHeightToFit(150.dp))
    }
}
