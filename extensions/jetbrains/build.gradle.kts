plugins {
    id("java")
    id("org.jetbrains.kotlin.jvm") version "1.9.22"
    id("org.jetbrains.intellij") version "1.17.2"
}

group = "com.metastackr"
version = "1.0.0"

repositories {
    mavenCentral()
}

// Configure IntelliJ Platform Plugin SDK
intellij {
    version.set("2023.3.4")
    type.set("IC") // IntelliJ Community Edition

    plugins.set(listOf("git4idea"))
}

tasks {
    patchPluginXml {
        sinceBuild.set("233")
        untilBuild.set("242.*")
    }

    signPlugin {
        certificateChain.set(System.getenv("CERTIFICATE_CHAIN"))
        privateKey.set(System.getenv("PRIVATE_KEY"))
        password.set(System.getenv("PRIVATE_KEY_PASSWORD"))
    }

    publishPlugin {
        token.set(System.getenv("PUBLISH_TOKEN"))
    }
}
