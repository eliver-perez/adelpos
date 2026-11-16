// Proyecto raíz de la aplicación Android: declara los complementos (cada módulo aplica los suyos),
// activa el bloqueo de dependencias en todos los proyectos y el formato de Kotlin (ktfmt, estilo
// oficial de Kotlin) en todos los módulos.
import com.ncorti.ktfmt.gradle.KtfmtExtension

buildscript {
    // Bloqueo de los complementos de Gradle resueltos por el bloque plugins
    // (buildscript-gradle.lockfile).
    configurations.classpath { resolutionStrategy.activateDependencyLocking() }
}

plugins {
    alias(libs.plugins.android.application) apply false
    alias(libs.plugins.android.library) apply false
    alias(libs.plugins.kotlin.jvm) apply false
    alias(libs.plugins.kotlin.compose) apply false
    alias(libs.plugins.kotlin.serialization) apply false
    alias(libs.plugins.ksp) apply false
    alias(libs.plugins.room) apply false
    alias(libs.plugins.ktfmt) apply false
}

allprojects {
    // Formato de Kotlin con ktfmt en el estilo oficial de Kotlin (4 espacios, 100 columnas), para
    // las fuentes y los scripts *.kts de cada proyecto: ./gradlew ktfmtFormat y ktfmtCheck
    // (scripts/dev.sh android-format y android-format-check). La versión de ktfmt la fija el
    // complemento y queda en el bloqueo (configuración ktfmt).
    apply(plugin = "com.ncorti.ktfmt.gradle")
    extensions.configure<KtfmtExtension> { kotlinLangStyle() }

    // Cada proyecto guarda en su gradle.lockfile las versiones exactas de todas sus
    // configuraciones.
    dependencyLocking { lockAllConfigurations() }

    // Resuelve todas las configuraciones resolubles (versiones y archivos) para escribir los
    // bloqueos y las sumas SHA-256 completos (ver android/CLAUDE.md, «Dependencias»): ./gradlew
    // resolveAllDependencies --write-locks --write-verification-metadata sha256
    tasks.register("resolveAllDependencies") {
        notCompatibleWithConfigurationCache("Recorre las configuraciones al ejecutarse")
        doLast {
            // Algunas configuraciones internas de AGP solo se resuelven con los atributos que AGP
            // les pone al usarlas: se informan y se omiten aquí; las cubre la compilación real que
            // acompaña a esta tarea.
            configurations
                .filter { it.isCanBeResolved }
                .forEach { configuration ->
                    try {
                        configuration.incoming.resolutionResult.allComponents
                        configuration.incoming.artifactView { isLenient = true }.files.files
                    } catch (exception: Exception) {
                        logger.lifecycle("Sin resolver aquí: $path ${configuration.name}")
                    }
                }
        }
    }
}
