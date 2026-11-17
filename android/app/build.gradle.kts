// Aplicación: entrada (Application con AppContainer, MainActivity), navegación entre las pantallas
// de los módulos feature y tipos de compilación (debug para pruebas, release reducido y sin ofuscar
// para el APK de prueba).
import java.io.File
import java.util.Properties

plugins {
    alias(libs.plugins.android.application)
    alias(libs.plugins.kotlin.compose)
    alias(libs.plugins.kotlin.serialization)
}

// Versión de la aplicación: único lugar, android/gradle.properties (también la usa
// X-Client-Version).
val appVersionName: String = providers.gradleProperty("adelpos.versionName").get()
val appVersionCode: Int = providers.gradleProperty("adelpos.versionCode").get().toInt()

/**
 * Firma del tipo release (A1-D8): una llave de desarrollo que vive FUERA del repositorio. La ruta
 * de su archivo de propiedades (storeFile, storePassword, keyAlias y keyPassword) llega por la
 * propiedad de Gradle adelpos.signingProperties o por la variable de entorno
 * ADELPOS_SIGNING_PROPERTIES. Sin ninguna de las dos, release se compila sin firmar. Nunca se
 * acepta un archivo o una llave dentro del repositorio.
 */
fun loadReleaseSigning(): Properties? {
    val path =
        providers
            .gradleProperty("adelpos.signingProperties")
            .orElse(providers.environmentVariable("ADELPOS_SIGNING_PROPERTIES"))
            .orNull ?: return null
    val repository = rootDir.parentFile.canonicalFile
    val propertiesFile = File(path).canonicalFile
    check(!propertiesFile.startsWith(repository)) {
        "Las propiedades de firma deben vivir fuera del repositorio: $propertiesFile"
    }
    check(propertiesFile.isFile) { "No existe el archivo de propiedades de firma: $propertiesFile" }
    val properties = Properties()
    val text =
        providers.fileContents(layout.projectDirectory.file(propertiesFile.path)).asText.get()
    properties.load(text.reader())
    for (key in listOf("storeFile", "storePassword", "keyAlias", "keyPassword")) {
        check(!properties.getProperty(key).isNullOrBlank()) { "Falta $key en $propertiesFile" }
    }
    val storeFile = File(properties.getProperty("storeFile")).canonicalFile
    check(!storeFile.startsWith(repository)) {
        "La llave de firma debe vivir fuera del repositorio: $storeFile"
    }
    return properties
}

val releaseSigning: Properties? = loadReleaseSigning()

android {
    namespace = "local.adel.pos.android"
    compileSdk { version = release(37) }

    defaultConfig {
        // Identificador provisional A1-D9; cambiarlo obliga a reinstalar y emparejar de nuevo.
        applicationId = "local.adel.pos.android"
        // A1-D7: Android 8.0 como mínimo (tabletas económicas), el nivel que exige hoy Google Play
        // como destino.
        minSdk = 26
        targetSdk = 36
        versionCode = appVersionCode
        versionName = appVersionName
        // Ejecutor propio: búsqueda NSD con un tipo de prueba y versión de cliente por argumento.
        testInstrumentationRunner = "local.adel.pos.android.AdelTestRunner"
    }

    signingConfigs {
        if (releaseSigning != null) {
            create("release") {
                storeFile = File(releaseSigning.getProperty("storeFile"))
                storePassword = releaseSigning.getProperty("storePassword")
                keyAlias = releaseSigning.getProperty("keyAlias")
                keyPassword = releaseSigning.getProperty("keyPassword")
            }
        }
    }

    buildTypes {
        release {
            // A1-D8: R8 reduce el APK, pero no ofusca (-dontobfuscate en proguard-rules.pro):
            // errores legibles.
            isMinifyEnabled = true
            isShrinkResources = true
            proguardFiles(
                getDefaultProguardFile("proguard-android-optimize.txt"),
                "proguard-rules.pro",
            )
            signingConfig = signingConfigs.findByName("release")
        }
    }

    buildFeatures {
        compose = true
        buildConfig = true
    }

    compileOptions {
        sourceCompatibility = JavaVersion.VERSION_17
        targetCompatibility = JavaVersion.VERSION_17
    }

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
    implementation(project(":core:domain"))
    implementation(project(":core:data"))
    implementation(project(":core:ui"))
    implementation(project(":feature:access"))
    implementation(project(":feature:waiter"))
    implementation(libs.androidx.core.ktx)
    implementation(libs.androidx.activity.compose)
    implementation(libs.androidx.lifecycle.runtime.compose)
    implementation(libs.androidx.lifecycle.viewmodel.compose)
    // Primer y segundo plano de toda la aplicación: abre y cierra el canal de tiempo real y el
    // temporizador del catálogo (CatalogRealtimeContainer).
    implementation(libs.androidx.lifecycle.process)
    implementation(libs.androidx.navigation.compose)
    // Configuración de WorkManager con la fábrica del trabajador de la cola (Configuration.Provider).
    implementation(libs.work.runtime)
    implementation(libs.kotlinx.serialization.json)
    implementation(libs.compose.ui.tooling.preview)
    debugImplementation(libs.compose.ui.tooling)

    androidTestImplementation(libs.androidx.test.runner)
    androidTestImplementation(libs.androidx.test.ext.junit)
    androidTestImplementation(platform(libs.compose.bom))
    androidTestImplementation(libs.compose.ui.test.junit4)
    androidTestImplementation(libs.androidx.test.espresso.core)
    debugImplementation(libs.compose.ui.test.manifest)
}
