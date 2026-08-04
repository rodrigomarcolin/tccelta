allprojects {
    repositories {
        google()
        mavenCentral()
    }
}

val newBuildDir: Directory =
    rootProject.layout.buildDirectory
        .dir("../../build")
        .get()
rootProject.layout.buildDirectory.value(newBuildDir)

subprojects {
    val newSubprojectBuildDir: Directory = newBuildDir.dir(project.name)
    project.layout.buildDirectory.value(newSubprojectBuildDir)

    // Alguns plugins (ex.: app_settings) fixam um compileSdk antigo (33) que conflita
    // com dependências androidx que exigem 34+. Forçamos os subprojetos Android a
    // compilarem contra o mesmo compileSdk do app.
    afterEvaluate {
        extensions.findByName("android")?.let { android ->
            try {
                val current = android.javaClass.getMethod("getCompileSdkVersion").invoke(android) as? String
                val currentApi = current?.substringAfter("android-")?.toIntOrNull() ?: 0
                if (currentApi < 36) {
                    android.javaClass
                        .getMethod("compileSdkVersion", Int::class.javaPrimitiveType)
                        .invoke(android, 36)
                }
            } catch (_: Exception) {
                // Subprojeto sem extensão Android compatível: ignora.
            }
        }
    }
}
subprojects {
    project.evaluationDependsOn(":app")
}

tasks.register<Delete>("clean") {
    delete(rootProject.layout.buildDirectory)
}
