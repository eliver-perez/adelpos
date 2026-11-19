// Aplicación Android de meseros de AD-EL POS (etapas A1 y A2). Estructura, comandos y reglas:
// android/CLAUDE.md.
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
}

rootProject.name = "adel-pos-android"

include(":app")

include(":core:domain")

include(":core:data")

include(":core:ui")

include(":core:capture")

include(":feature:access")

include(":feature:waiter")

include(":feature:takeaway")
