// Aplicación de cocina de AD-EL POS para Android (etapa K1): una pantalla de cocina (KDS) para
// tabletas en horizontal, conectada a la computadora principal. Proyecto Gradle propio, sin nada
// de la aplicación de meseros; lo compartido viene de android-core/ (build incluido). Estructura,
// comandos y reglas: cocina/CLAUDE.md.
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
    // K1-D2: un solo catálogo de versiones para los tres proyectos Android, el de android-core.
    versionCatalogs { create("libs") { from(files("../android-core/gradle/libs.versions.toml")) } }
}

rootProject.name = "adel-pos-cocina"

// K1-D2: lo compartido con la aplicación de meseros. La aplicación pide «com.adelpos.core:<módulo>»
// y Gradle lo sustituye por los proyectos de android-core.
includeBuild("../android-core")

include(":app")
