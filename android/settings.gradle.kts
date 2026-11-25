// Aplicación Android de meseros de AD-EL POS (etapas A1 y A2). Estructura, comandos y reglas:
// android/CLAUDE.md. Desde la etapa K1 usa el código compartido de android-core/ (build incluido).
pluginManagement {
    repositories {
        google {
            // El repositorio de Google solo sirve los grupos de Google y de AndroidX.
            content {
                includeGroupByRegex("androidx\\..*")
                includeGroupByRegex("com\\.android(\\..*)?")
                includeGroupByRegex("com\\.google\\..*")
            }
        }
        mavenCentral()
        gradlePluginPortal()
    }
}

dependencyResolutionManagement {
    // Los repositorios se declaran solo aquí: ningún módulo agrega los suyos.
    repositoriesMode.set(RepositoriesMode.FAIL_ON_PROJECT_REPOS)
    repositories {
        google {
            content {
                includeGroupByRegex("androidx\\..*")
                includeGroupByRegex("com\\.android(\\..*)?")
                includeGroupByRegex("com\\.google\\..*")
            }
        }
        mavenCentral()
    }
    // Etapa K1 (K1-D2): un solo catálogo de versiones para los tres proyectos Android, el de
    // android-core.
    versionCatalogs { create("libs") { from(files("../android-core/gradle/libs.versions.toml")) } }
}

rootProject.name = "adel-pos-android"

// Etapa K1 (K1-D2): lo compartido con la aplicación de cocina. Los módulos piden
// «com.adelpos.core:<módulo>» y Gradle los sustituye por los proyectos de android-core.
includeBuild("../android-core")

include(":app")

include(":core:domain")

include(":core:data")

include(":core:ui")

include(":core:capture")

include(":feature:access")

include(":feature:waiter")

include(":feature:takeaway")
