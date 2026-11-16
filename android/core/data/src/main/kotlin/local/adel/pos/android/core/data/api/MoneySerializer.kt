package local.adel.pos.android.core.data.api

import kotlinx.serialization.KSerializer
import kotlinx.serialization.SerializationException
import kotlinx.serialization.descriptors.PrimitiveKind
import kotlinx.serialization.descriptors.PrimitiveSerialDescriptor
import kotlinx.serialization.descriptors.SerialDescriptor
import kotlinx.serialization.encoding.Decoder
import kotlinx.serialization.encoding.Encoder
import kotlinx.serialization.json.JsonDecoder
import kotlinx.serialization.json.JsonPrimitive
import local.adel.pos.android.core.domain.money.Money
import local.adel.pos.android.core.domain.money.MoneyFormat

/**
 * Importes de la API v1: siempre cadenas decimales (`"130.00"`), nunca números de punto flotante.
 * Un número JSON (`130.0`) se rechaza en lugar de convertirse, porque pasaría por un Double y
 * podría perder centavos. Uso en un DTO: `@Serializable(with = MoneySerializer::class) val total:
 * Money`.
 */
object MoneySerializer : KSerializer<Money> {
    override val descriptor: SerialDescriptor =
        PrimitiveSerialDescriptor("local.adel.pos.android.Money", PrimitiveKind.STRING)

    override fun serialize(encoder: Encoder, value: Money) {
        encoder.encodeString(value.decimal)
    }

    override fun deserialize(decoder: Decoder): Money {
        val text =
            if (decoder is JsonDecoder) {
                val element = decoder.decodeJsonElement()
                if (element !is JsonPrimitive || !element.isString) {
                    throw SerializationException("El importe debe llegar como cadena decimal")
                }
                element.content
            } else {
                decoder.decodeString()
            }
        if (!MoneyFormat.isDecimal(text)) {
            throw SerializationException("Importe no válido")
        }
        return Money(text)
    }
}
