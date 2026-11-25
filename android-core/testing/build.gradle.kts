// Ayudantes de las pruebas instrumentadas contra el Go temporal del arnés (etapa K1): la
// «computadora principal» por la escucha local (MainComputer), el servidor de pruebas con sus
// argumentos (GoTestServer) y los diálogos del sistema que estorban (SystemDialogs). Los usan las
// pruebas de las dos aplicaciones (androidTestImplementation); nunca entran en un APK de entrega.
plugins {
    alias(libs.plugins.android.library)
    alias(libs.plugins.kotlin.serialization)
}

android {
    namespace = "com.adelpos.core.testing"
    compileSdk { version = release(37) }

    defaultConfig { minSdk = 26 }

    compileOptions {
        sourceCompatibility = JavaVersion.VERSION_17
        targetCompatibility = JavaVersion.VERSION_17
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
    api(libs.okhttp)
    api(libs.kotlinx.serialization.json)
    api(libs.junit)
    api(libs.androidx.test.runner)
}
