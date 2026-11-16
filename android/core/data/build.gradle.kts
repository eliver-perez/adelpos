// Datos: cliente HTTP con huella fijada, JSON de la API v1, Room, monitor de conexión y cifrado con
// el Android Keystore. Biblioteca Android sin pantallas.
plugins {
    alias(libs.plugins.android.library)
    alias(libs.plugins.kotlin.serialization)
    alias(libs.plugins.ksp)
    alias(libs.plugins.room)
}

android {
    namespace = "local.adel.pos.android.core.data"
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
        unitTests.isIncludeAndroidResources = true
        animationsDisabled = true
        // El APK de pruebas de la biblioteca toma el nivel de compileSdk (37) si no se indica: en
        // Android 17 eso exige el permiso de red local para NSD y la red local (A1-D7). Se prueba
        // con el mismo nivel objetivo que la aplicación (36).
        targetSdk = 36
    }

    lint {
        abortOnError = true
        warningsAsErrors = true
        // Las versiones las fija el avance de la etapa (§1.2) y targetSdk 36 es la decisión A1-D7.
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

room {
    // Esquemas exportados y versionados: cada fase que cambie la base sube la versión y agrega su
    // migración.
    schemaDirectory("$projectDir/schemas")
}

dependencies {
    api(project(":core:domain"))
    api(libs.okhttp)
    implementation(libs.androidx.core.ktx)
    // api: AppDatabase (pública para el AppContainer) extiende RoomDatabase.
    api(libs.room.runtime)
    ksp(libs.room.compiler)
    implementation(libs.kotlinx.serialization.json)
    implementation(libs.kotlinx.coroutines.android)

    testImplementation(libs.junit)
    testImplementation(libs.kotlinx.coroutines.test)
    testImplementation(libs.robolectric)
    testImplementation(libs.androidx.test.core)
    testImplementation(libs.androidx.test.ext.junit)
    testImplementation(libs.mockwebserver)
    testImplementation(libs.okhttp.tls)

    androidTestImplementation(libs.androidx.test.runner)
    androidTestImplementation(libs.androidx.test.core)
    androidTestImplementation(libs.androidx.test.ext.junit)
    androidTestImplementation(libs.kotlinx.coroutines.test)
    androidTestImplementation(libs.room.testing)
}
