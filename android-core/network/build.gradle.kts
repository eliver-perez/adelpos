// Red compartida (etapa K1): cliente HTTPS con la huella fijada y el punto único de respuestas de
// la A3, la API v1 y sus errores, la búsqueda del servidor y el emparejamiento, el canal de tiempo
// real, el estado único de conexión con su revisión, el cifrado con el Android Keystore y el
// registro del borrado de los datos del dispositivo. Biblioteca Android sin pantallas ni Room: la
// base local es de cada aplicación.
plugins {
    alias(libs.plugins.android.library)
    alias(libs.plugins.kotlin.serialization)
}

android {
    namespace = "com.adelpos.core.network"
    compileSdk { version = release(37) }

    defaultConfig {
        minSdk = 26
        testInstrumentationRunner = "androidx.test.runner.AndroidJUnitRunner"
    }

    compileOptions {
        sourceCompatibility = JavaVersion.VERSION_17
        targetCompatibility = JavaVersion.VERSION_17
    }

    testOptions {
        animationsDisabled = true
        // El APK de pruebas de la biblioteca toma el nivel de compileSdk (37) si no se indica: en
        // Android 17 eso exige el permiso de red local para NSD y la red local (A1-D7). Se prueba
        // con el mismo nivel objetivo que las aplicaciones (36).
        targetSdk = 36
    }

    lint {
        abortOnError = true
        warningsAsErrors = true
        // Las versiones las fija el avance de la etapa A1 (§1.2) y targetSdk 36 es la decisión
        // A1-D7.
        disable +=
            setOf(
                "GradleDependency",
                "AndroidGradlePluginVersion",
                "NewerVersionAvailable",
                "OldTargetApi",
            )
        // Las verificaciones de lint que trae lifecycle 2.11.0 usan una API que el lint de AGP
        // 9.5.1 ya no tiene: lint las omite y lo avisa (UnsafeLifecycleWhenUsage,
        // RepeatOnLifecycleWrongUsage). Queda como aviso informativo.
        informational += "ObsoleteLintCustomCheck"
    }
}

dependencies {
    api(project(":model"))
    api(libs.okhttp)
    // Canal de tiempo real (SSE) de Go: un solo flujo por proceso (RealtimeClient).
    implementation(libs.okhttp.sse)
    implementation(libs.androidx.core.ktx)
    // El primer plano del proceso (AppForeground) para la revisión del estado de conexión.
    implementation(libs.androidx.lifecycle.process)
    implementation(libs.kotlinx.serialization.json)
    implementation(libs.kotlinx.coroutines.android)

    testImplementation(libs.junit)
    testImplementation(libs.kotlinx.coroutines.test)
    testImplementation(libs.mockwebserver)
    testImplementation(libs.okhttp.tls)

    androidTestImplementation(libs.androidx.test.runner)
    androidTestImplementation(libs.androidx.test.core)
    androidTestImplementation(libs.androidx.test.ext.junit)
    androidTestImplementation(libs.kotlinx.coroutines.test)
}
