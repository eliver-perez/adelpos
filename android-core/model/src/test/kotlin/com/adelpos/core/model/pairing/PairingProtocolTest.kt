package com.adelpos.core.model.pairing

import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertThrows
import org.junit.Test

/**
 * El código del emparejamiento coincide con el vector de Go (`administration/pairing_test.go`) y de
 * Rust (`pairing_code_matches_the_go_reference_vector`).
 */
class PairingProtocolTest {
    private fun bytes(offset: Int) = ByteArray(32) { index -> (offset + index).toByte() }

    private val request = "00000000-0000-4000-8000-000000000001"

    @Test
    fun elCodigoEsElDelVectorDeGo() {
        assertEquals("166101", PairingProtocol.code(bytes(0), bytes(32), bytes(64), request))
        assertEquals(
            "72dbb7336c76780023f83da4c355f2eeea85733b13d3477697917790c1229084",
            PairingProtocol.commitment(bytes(32)),
        )
    }

    @Test
    fun otraHuellaDaOtroCodigo() {
        val fingerprint = bytes(0)
        fingerprint[0] = 0xff.toByte()
        assertEquals("089301", PairingProtocol.code(fingerprint, bytes(32), bytes(64), request))
    }

    @Test
    fun elIdDeLaSolicitudEnMayusculasDaElMismoCodigo() {
        assertEquals(
            "166101",
            PairingProtocol.code(bytes(0), bytes(32), bytes(64), request.uppercase()),
        )
    }

    @Test
    fun rechazaCamposMalFormados() {
        assertThrows(IllegalArgumentException::class.java) {
            PairingProtocol.code(bytes(0), bytes(32), bytes(64), "no-es-un-uuid")
        }
        assertThrows(IllegalArgumentException::class.java) {
            PairingProtocol.code(ByteArray(31), bytes(32), bytes(64), request)
        }
        assertThrows(IllegalArgumentException::class.java) {
            PairingProtocol.code(bytes(0), ByteArray(16), bytes(64), request)
        }
        assertThrows(IllegalArgumentException::class.java) {
            PairingProtocol.commitment(ByteArray(8))
        }
    }

    @Test
    fun cadaNumeroEsNuevoYElCodigoSeAgrupa() {
        assertFalse(PairingProtocol.newNonce().contentEquals(PairingProtocol.newNonce()))
        assertEquals(32, PairingProtocol.newNonce().size)
        assertEquals("166 101", PairingProtocol.grouped("166101"))
        assertEquals("12", PairingProtocol.grouped("12"))
    }
}
