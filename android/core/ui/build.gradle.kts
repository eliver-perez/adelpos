// Interfaz compartida con la etapa A2: tema AD-EL POS (claro y oscuro), íconos, botones táctiles,
// indicador de conexión, estado de mesa y andamio de uno o dos paneles.
plugins {
    alias(libs.plugins.android.library)
    alias(libs.plugins.kotlin.compose)
}

android {
    namespace = "local.adel.pos.android.core.ui"
    compileSdk { version = release(37) }

    defaultConfig {
        minSdk = 26
        testInstrumentationRunner = "androidx.test.runner.AndroidJUnitRunner"
    }

    compileOptions {
        sourceCompatibility = JavaVersion.VERSION_17
        targetCompatibility = JavaVersion.VERSION_17
    }

    buildFeatures { compose = true }

    testOptions { animationsDisabled = true }

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
    api(project(":core:domain"))
    api(platform(libs.compose.bom))
    api(libs.compose.material3)
    api(libs.compose.ui)
    api(libs.compose.foundation)
    api(libs.material3.adaptive)
    api(libs.androidx.window.core)
    implementation(libs.androidx.activity.compose)
    implementation(libs.compose.ui.tooling.preview)
    debugImplementation(libs.compose.ui.tooling)

    testImplementation(libs.junit)

    androidTestImplementation(libs.androidx.test.runner)
    androidTestImplementation(libs.androidx.test.ext.junit)
    androidTestImplementation(platform(libs.compose.bom))
    androidTestImplementation(libs.compose.ui.test.junit4)
    androidTestImplementation(libs.androidx.test.espresso.core)
    debugImplementation(libs.compose.ui.test.manifest)
}

tasks.withType<Test>().configureEach {
    // La prueba de colores recorre las fuentes de todos los módulos (los colores solo viven en el
    // tema).
    val androidRoot = rootProject.projectDir
    inputs
        .files(
            fileTree(androidRoot) {
                include("**/src/main/**/*.kt", "**/src/main/res/**/*.xml")
                exclude("**/build/**")
            },
        )
        .withPropertyName("androidSources")
    systemProperty("adelpos.androidRoot", androidRoot.absolutePath)
}
