// Modo Mesero: Mesas y el panel de la mesa con sus cuentas (Fase E). La captura, «Pedidos
// pendientes» y «Requiere tu revisión» viven en core:capture desde la etapa A2 (compartidas con
// Para
// llevar).
plugins {
    alias(libs.plugins.android.library)
    alias(libs.plugins.kotlin.compose)
}

android {
    namespace = "com.adelpos.meseros.feature.waiter"
    compileSdk { version = release(37) }

    defaultConfig { minSdk = 26 }

    compileOptions {
        sourceCompatibility = JavaVersion.VERSION_17
        targetCompatibility = JavaVersion.VERSION_17
    }

    buildFeatures { compose = true }

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

dependencies {
    implementation(project(":core:ui"))
    implementation(project(":core:capture"))
    implementation(libs.androidx.lifecycle.viewmodel.compose)
    implementation(libs.androidx.lifecycle.viewmodel.savedstate)
    implementation(libs.compose.ui.tooling.preview)
    debugImplementation(libs.compose.ui.tooling)

    testImplementation(libs.junit)
    testImplementation(libs.kotlinx.coroutines.test)
}
