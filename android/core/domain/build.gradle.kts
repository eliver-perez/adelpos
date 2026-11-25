// Dominio: Kotlin/JVM puro, sin Android, para probarlo en la JVM en segundos. Modelos y reglas de
// la aplicación (nunca de negocio: precios, impuestos, permisos y estados los decide el servidor
// Go). Lo compartido con la aplicación de cocina vive en android-core:model (etapa K1).
import org.jetbrains.kotlin.gradle.dsl.JvmTarget

plugins { alias(libs.plugins.kotlin.jvm) }

java {
    sourceCompatibility = JavaVersion.VERSION_17
    targetCompatibility = JavaVersion.VERSION_17
}

kotlin { compilerOptions { jvmTarget = JvmTarget.JVM_17 } }

dependencies {
    // Etapa K1: el modelo compartido con la aplicación de cocina (android-core:model).
    api("com.adelpos.core:model")
    api(libs.kotlinx.coroutines.core)

    testImplementation(libs.junit)
    testImplementation(libs.kotlinx.coroutines.test)
    // Solo para leer la tabla de casos del formato de importes que comparten Go y el escritorio.
    testImplementation(libs.kotlinx.serialization.json)
}

tasks.test {
    // Tabla única de casos del formato de importes
    // (backend/internal/moneytext/testdata/cases.json): la misma que usan las pruebas de Go y de
    // Vitest.
    val moneyCases = rootProject.file("../backend/internal/moneytext/testdata/cases.json")
    inputs.file(moneyCases).withPropertyName("moneyCases")
    systemProperty("adelpos.moneyCases", moneyCases.absolutePath)
}
