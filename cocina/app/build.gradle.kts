// Aplicación de cocina (etapa K1): una pantalla de cocina para tabletas en horizontal, conectada a
// la computadora principal. Lo compartido con la de meseros (emparejamiento, red, estado de
// conexión, tema y pantallas de la entrada) viene de android-core; aquí solo lo de cocina.
import java.io.File
import java.util.Properties

plugins {
    alias(libs.plugins.android.application)
    alias(libs.plugins.kotlin.compose)
    alias(libs.plugins.kotlin.serialization)
    alias(libs.plugins.ksp)
    alias(libs.plugins.room)
}

// Versión de la aplicación: único lugar, cocina/gradle.properties (también la usa
// X-Client-Version).
val appVersionName: String = providers.gradleProperty("adelpos.versionName").get()
val appVersionCode: Int = providers.gradleProperty("adelpos.versionCode").get().toInt()

/**
 * Firma del tipo release: la misma llave de desarrollo de la aplicación de meseros (§1 de la etapa
 * K1), que vive FUERA del repositorio. La ruta de su archivo de propiedades (storeFile,
 * storePassword, keyAlias y keyPassword) llega por la propiedad de Gradle adelpos.signingProperties
 * o por la variable de entorno ADELPOS_SIGNING_PROPERTIES. Sin ninguna de las dos, release se
 * compila sin firmar. Nunca se acepta un archivo o una llave dentro del repositorio. Es la misma
 * regla de android/app/build.gradle.kts (A1-D8).
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
    namespace = "com.adelpos.cocina"
    compileSdk { version = release(37) }

    defaultConfig {
        // La aplicación de cocina es otra aplicación que la de meseros (com.adelpos.meseros): se
        // instalan juntas en la misma tableta si hace falta.
        applicationId = "com.adelpos.cocina"
        // Las mismas que meseros (A1-D7): Android 8.0 como mínimo (tabletas económicas).
        minSdk = 26
        targetSdk = 36
        versionCode = appVersionCode
        versionName = appVersionName
        // Ejecutor propio: búsqueda NSD con el tipo de pruebas y versión de cliente por argumento.
        testInstrumentationRunner = "com.adelpos.cocina.CocinaTestRunner"
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

    testOptions {
        animationsDisabled = true
        unitTests.isIncludeAndroidResources = true
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

room {
    // Esquemas exportados y versionados: cada cambio de la base sube la versión y agrega su
    // migración.
    schemaDirectory("$projectDir/schemas")
}

dependencies {
    // Lo compartido con la aplicación de meseros (K1-D2).
    implementation("com.adelpos.core:model")
    implementation("com.adelpos.core:network")
    implementation("com.adelpos.core:ui")
    implementation(libs.androidx.core.ktx)
    implementation(libs.androidx.activity.compose)
    implementation(libs.androidx.lifecycle.runtime.compose)
    implementation(libs.androidx.lifecycle.viewmodel.compose)
    // Primer plano del proceso: la revisión del estado de conexión (AppForeground).
    implementation(libs.androidx.lifecycle.process)
    // La base propia de la pantalla de cocina (el servidor emparejado).
    implementation(libs.room.runtime)
    ksp(libs.room.compiler)
    implementation(libs.kotlinx.coroutines.android)
    // Los cuerpos de las rutas de la pantalla de cocina (kitchen-display/*).
    implementation(libs.kotlinx.serialization.json)
    implementation(libs.compose.ui.tooling.preview)
    debugImplementation(libs.compose.ui.tooling)

    testImplementation(libs.junit)
    testImplementation(libs.kotlinx.coroutines.test)
    testImplementation(libs.robolectric)
    testImplementation(libs.androidx.test.core)

    // Los ayudantes de las pruebas contra el Go temporal, compartidos con la de meseros.
    androidTestImplementation("com.adelpos.core:testing")
    androidTestImplementation(libs.androidx.test.runner)
    androidTestImplementation(libs.androidx.test.ext.junit)
    androidTestImplementation(libs.kotlinx.coroutines.test)
    androidTestImplementation(platform(libs.compose.bom))
    androidTestImplementation(libs.compose.ui.test.junit4)
    androidTestImplementation(libs.androidx.test.espresso.core)
    debugImplementation(libs.compose.ui.test.manifest)
}
