package local.adel.pos.android.core.ui.catalog

import android.graphics.BitmapFactory
import android.util.LruCache
import androidx.compose.foundation.Image
import androidx.compose.foundation.background
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.produceState
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.ImageBitmap
import androidx.compose.ui.graphics.asImageBitmap
import androidx.compose.ui.layout.ContentScale
import androidx.compose.ui.platform.testTag
import java.io.File
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.withContext
import local.adel.pos.android.core.domain.catalog.CatalogImages

/**
 * Imagen de un producto o categoría desde la caché propia del dispositivo. Mientras no hay imagen
 * (no tiene, todavía no se descarga o no hay conexión) muestra un marcador con la inicial del
 * nombre. Decorativa: el nombre del producto siempre está escrito al lado.
 */
@Composable
fun ProductImage(
    imageId: String?,
    name: String,
    images: CatalogImages?,
    modifier: Modifier = Modifier,
) {
    val bitmap by
        produceState(initialValue = imageId?.let { DecodedImages.get(it) }, imageId, images) {
            if (imageId == null || images == null || value != null) {
                return@produceState
            }
            val file =
                withContext(Dispatchers.IO) { images.cached(imageId) } ?: images.obtain(imageId)
            if (file != null) {
                value = withContext(Dispatchers.IO) { DecodedImages.decode(imageId, file) }
            }
        }
    Box(
        modifier = modifier.background(MaterialTheme.colorScheme.surfaceVariant),
        contentAlignment = Alignment.Center,
    ) {
        val image = bitmap
        if (image != null) {
            Image(
                bitmap = image,
                contentDescription = null,
                contentScale = ContentScale.Crop,
                modifier = Modifier.fillMaxSize().testTag(TAG_PRODUCT_IMAGE),
            )
        } else {
            Text(
                text = name.trim().take(1).uppercase(),
                style = MaterialTheme.typography.headlineMedium,
                color = MaterialTheme.colorScheme.onSurfaceVariant,
                modifier = Modifier.testTag(TAG_PRODUCT_IMAGE_PLACEHOLDER),
            )
        }
    }
}

/**
 * Imágenes ya decodificadas en memoria (las últimas [CAPACITY]), para no volver a leer el archivo
 * al desplazarse por la cuadrícula. Se decodifican reducidas a unos [TARGET_PIXELS] de lado: en una
 * tableta económica una foto completa ocuparía varios MB de memoria.
 */
internal object DecodedImages {
    private const val CAPACITY = 96
    private const val TARGET_PIXELS = 320

    private val cache = LruCache<String, ImageBitmap>(CAPACITY)

    fun get(imageId: String): ImageBitmap? = cache.get(imageId)

    fun decode(imageId: String, file: File): ImageBitmap? {
        val bounds = BitmapFactory.Options().apply { inJustDecodeBounds = true }
        BitmapFactory.decodeFile(file.path, bounds)
        if (bounds.outWidth <= 0 || bounds.outHeight <= 0) {
            return null
        }
        var sample = 1
        while (
            bounds.outWidth / (sample * 2) >= TARGET_PIXELS &&
                bounds.outHeight / (sample * 2) >= TARGET_PIXELS
        ) {
            sample *= 2
        }
        val options = BitmapFactory.Options().apply { inSampleSize = sample }
        val decoded = BitmapFactory.decodeFile(file.path, options) ?: return null
        val image = decoded.asImageBitmap()
        cache.put(imageId, image)
        return image
    }
}

const val TAG_PRODUCT_IMAGE = "product-image"
const val TAG_PRODUCT_IMAGE_PLACEHOLDER = "product-image-placeholder"
