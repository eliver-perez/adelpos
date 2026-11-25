package com.adelpos.core.ui.access.status

import android.content.Context
import android.content.Intent
import android.content.pm.PackageManager
import android.net.Uri
import android.os.Build
import android.provider.Settings
import androidx.activity.compose.rememberLauncherForActivityResult
import androidx.activity.result.contract.ActivityResultContracts
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.saveable.rememberSaveable
import androidx.compose.runtime.setValue
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.platform.testTag
import androidx.compose.ui.res.stringResource
import androidx.core.content.ContextCompat
import com.adelpos.core.model.access.NetworkPermissions
import com.adelpos.core.ui.R
import com.adelpos.core.ui.access.deviceWords
import com.adelpos.core.ui.access.findserver.CenteredColumn
import com.adelpos.core.ui.access.findserver.ProblemNotice
import com.adelpos.core.ui.components.PrimaryButton
import com.adelpos.core.ui.components.SecondaryButton

/**
 * Permisos de red de Android antes de buscar o hablar con el servidor (§4, «Permisos de Android»),
 * con una explicación en español **antes** de pedirlos. Los permisos que hacen falta los decide
 * [NetworkPermissions] según la versión de Android y el `targetSdk` de la aplicación: hoy
 * (`targetSdk` 36) ninguno, así que [content] se muestra directo. Al subir a `targetSdk` 37, en
 * Android 17 se pedirá `ACCESS_LOCAL_NETWORK` aquí; si se niega, se explica cómo darlo en los
 * ajustes (sin él no hay ninguna conexión con la red local, ni siquiera escribiendo la dirección).
 */
@Composable
fun LocalNetworkPermissionGate(content: @Composable () -> Unit) {
    val context = LocalContext.current
    val required =
        remember(context) {
            NetworkPermissions.required(
                sdkInt = Build.VERSION.SDK_INT,
                targetSdk = context.applicationInfo.targetSdkVersion,
            )
        }
    var granted by remember(required) { mutableStateOf(allGranted(context, required)) }
    var denied by rememberSaveable { mutableStateOf(false) }
    val launcher =
        rememberLauncherForActivityResult(ActivityResultContracts.RequestMultiplePermissions()) {
            results ->
            granted = results.values.all { it }
            denied = !granted
        }
    if (granted) {
        content()
        return
    }
    val words = deviceWords()
    CenteredColumn(modifier = Modifier.testTag(TAG_PERMISSION)) {
        Text(
            text = stringResource(R.string.permission_local_network_title),
            style = MaterialTheme.typography.headlineSmall,
        )
        if (denied) {
            ProblemNotice(
                stringResource(R.string.permission_local_network_denied, words.thisDevice),
            )
            PrimaryButton(
                text = stringResource(R.string.permission_open_settings),
                onClick = { context.startActivity(appSettings(context)) },
                modifier = Modifier.fillMaxWidth(),
            )
            SecondaryButton(
                text = stringResource(R.string.permission_try_again),
                onClick = {
                    granted = allGranted(context, required)
                    if (!granted) {
                        launcher.launch(required.toTypedArray())
                    }
                },
                modifier = Modifier.fillMaxWidth(),
            )
        } else {
            Text(
                text = stringResource(R.string.permission_local_network_text),
                style = MaterialTheme.typography.bodyLarge,
            )
            PrimaryButton(
                text = stringResource(R.string.permission_continue),
                onClick = { launcher.launch(required.toTypedArray()) },
                modifier = Modifier.fillMaxWidth(),
            )
        }
    }
}

private fun allGranted(context: Context, permissions: List<String>): Boolean =
    permissions.all { permission ->
        ContextCompat.checkSelfPermission(context, permission) == PackageManager.PERMISSION_GRANTED
    }

private fun appSettings(context: Context): Intent =
    Intent(Settings.ACTION_APPLICATION_DETAILS_SETTINGS)
        .setData(Uri.fromParts("package", context.packageName, null))
        .addFlags(Intent.FLAG_ACTIVITY_NEW_TASK)

const val TAG_PERMISSION = "access-permission"
