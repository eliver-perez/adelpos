// Modelo compartido (etapa K1): Kotlin/JVM puro, sin Android, para probarlo en la JVM en segundos.
// El estado único de conexión de la A3, el emparejamiento, la dirección y el anuncio del servidor,
// la compatibilidad de versiones, el tiempo real, los errores y los mensajes de la licencia. Nunca
// reglas de negocio: precios, impuestos, permisos y estados los decide el servidor Go.
import org.jetbrains.kotlin.gradle.dsl.JvmTarget

plugins { alias(libs.plugins.kotlin.jvm) }

java {
    sourceCompatibility = JavaVersion.VERSION_17
    targetCompatibility = JavaVersion.VERSION_17
}

kotlin { compilerOptions { jvmTarget = JvmTarget.JVM_17 } }

dependencies {
    api(libs.kotlinx.coroutines.core)

    testImplementation(libs.junit)
    testImplementation(libs.kotlinx.coroutines.test)
}
