// Pantallas compartidas (etapa K1, K1-D2): tema AD-EL POS claro y oscuro con la paleta única, los
// íconos, los botones táctiles, la barra superior con el indicador de conexión y las pantallas de
// la entrada al dispositivo que usan las dos aplicaciones (buscar el servidor, el código de
// emparejamiento y los estados: emparejar de nuevo, actualizar, dispositivo retirado).
plugins {
    alias(libs.plugins.android.library)
    alias(libs.plugins.kotlin.compose)
}

android {
    namespace = "com.adelpos.core.ui"
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
    api(platform(libs.compose.bom))
    api(libs.compose.material3)
    api(libs.compose.ui)
    api(libs.compose.foundation)
    implementation(libs.androidx.core.ktx)
    implementation(libs.androidx.activity.compose)
    implementation(libs.androidx.lifecycle.viewmodel.compose)
    implementation(libs.androidx.lifecycle.viewmodel.savedstate)
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
    // Las pruebas de la regla de textos y de los colores recorren las fuentes de los tres
    // proyectos Android (android/, cocina/ y este): los textos y los colores siguen las mismas
    // reglas en las dos aplicaciones.
    val repositoryRoot = rootProject.projectDir.parentFile
    val sources =
        listOf("android", "android-core", "cocina").map { folder ->
            fileTree(repositoryRoot.resolve(folder)) {
                include("**/src/main/**/*.kt", "**/src/main/res/**/*.xml")
                exclude("**/build/**")
            }
        }
    inputs.files(sources).withPropertyName("androidSources")
    // La prueba del distintivo de los meseros lee la paleta de colores de Go.
    inputs
        .file(repositoryRoot.resolve("backend/internal/catalog/palette.go"))
        .withPropertyName("badgePalette")
    systemProperty("adelpos.repositoryRoot", repositoryRoot.absolutePath)
}
