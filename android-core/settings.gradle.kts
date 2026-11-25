// Código compartido de las aplicaciones Android de AD-EL POS (etapa K1, K1-D2): la de meseros
// (android/) y la de cocina (cocina/) lo incluyen con includeBuild("../android-core"). Es un
// build propio: se compila y se prueba también solo. Reglas: android-core/CLAUDE.md.
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
    // Los repositorios se declaran solo aquí: ningún módulo agrega los suyos. El catálogo de
    // versiones (gradle/libs.versions.toml) es el único de los tres proyectos: este build lo toma
    // solo y android/ y cocina/ lo importan.
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

rootProject.name = "adel-pos-android-core"

include(":model")

include(":network")

include(":ui")
