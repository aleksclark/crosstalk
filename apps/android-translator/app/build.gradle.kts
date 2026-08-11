import org.openapitools.generator.gradle.plugin.tasks.GenerateTask

plugins {
    alias(libs.plugins.android.application)
    alias(libs.plugins.kotlin.android)
    alias(libs.plugins.kotlin.compose)
    alias(libs.plugins.kotlin.serialization)
    alias(libs.plugins.openapi.generator)
}

val openApiOutputDir = layout.buildDirectory.dir("generated/openapi")
val openApiSpec = rootProject.file("../../api/openapi.json")

android {
    namespace = "com.crosstalk.translator"
    compileSdk = 35

    defaultConfig {
        applicationId = "com.crosstalk.translator"
        minSdk = 26
        targetSdk = 35
        versionCode = 1
        versionName = "0.1.0"

        testInstrumentationRunner = "androidx.test.runner.AndroidJUnitRunner"

        buildConfigField("String", "API_BASE_URL", "\"https://crosstalk.local\"")
        buildConfigField("Boolean", "ALLOW_CLEARTEXT", "false")
    }

    buildFeatures {
        compose = true
        buildConfig = true
    }

    compileOptions {
        sourceCompatibility = JavaVersion.VERSION_17
        targetCompatibility = JavaVersion.VERSION_17
    }

    kotlinOptions {
        jvmTarget = "17"
    }

    buildTypes {
        debug {
            applicationIdSuffix = ".debug"
            versionNameSuffix = "-debug"
            buildConfigField("String", "API_BASE_URL", "\"http://10.0.2.2:8080\"")
            buildConfigField("Boolean", "ALLOW_CLEARTEXT", "true")
        }
        release {
            isMinifyEnabled = true
            isShrinkResources = true
            proguardFiles(
                getDefaultProguardFile("proguard-android-optimize.txt"),
                "proguard-rules.pro",
            )
            // Debug-signed release smoke builds are acceptable for Phase 1 gates.
            signingConfig = signingConfigs.getByName("debug")
        }
    }

    packaging {
        resources {
            excludes += "/META-INF/{AL2.0,LGPL2.1}"
        }
    }

    testOptions {
        unitTests {
            isIncludeAndroidResources = true
        }
    }

    sourceSets {
        getByName("main") {
            java.srcDir(openApiOutputDir.map { it.dir("src/main/kotlin") })
        }
    }

    lint {
        abortOnError = true
        warningsAsErrors = false
        checkReleaseBuilds = false
    }
}

kotlin {
    jvmToolchain(17)
}

tasks.register<Delete>("cleanOpenApiGenerate") {
    delete(openApiOutputDir)
}

// Plugin already registers openApiGenerate; configure it in place.
tasks.named<GenerateTask>("openApiGenerate") {
    group = "openapi"
    description = "Generate Kotlin OkHttp client from api/openapi.json"
    dependsOn("cleanOpenApiGenerate")
    generatorName.set("kotlin")
    library.set("jvm-okhttp4")
    inputSpec.set(openApiSpec.absolutePath)
    outputDir.set(openApiOutputDir.map { it.asFile.absolutePath })
    apiPackage.set("com.crosstalk.translator.generated.api")
    modelPackage.set("com.crosstalk.translator.generated.model")
    invokerPackage.set("com.crosstalk.translator.generated.infrastructure")
    packageName.set("com.crosstalk.translator.generated")
    configOptions.set(
        mapOf(
            "dateLibrary" to "java8",
            "serializationLibrary" to "kotlinx_serialization",
            "useCoroutines" to "true",
            "enumPropertyNaming" to "UPPERCASE",
            "collectionType" to "list",
            "sortParamsByRequiredFlag" to "true",
            "sortModelPropertiesByRequiredFlag" to "true",
            "omitGradleWrapper" to "true",
        ),
    )
    // Empty values mean "all" for models/apis/supportingFiles.
    globalProperties.set(
        mapOf(
            "models" to "",
            "apis" to "",
            "supportingFiles" to "",
            "apiTests" to "false",
            "modelTests" to "false",
            "apiDocs" to "false",
            "modelDocs" to "false",
        ),
    )
    skipValidateSpec.set(false)
    generateApiTests.set(false)
    generateModelTests.set(false)
    generateApiDocumentation.set(false)
    generateModelDocumentation.set(false)
    inputs.file(openApiSpec)
    outputs.dir(openApiOutputDir)
    doLast {
        val root = openApiOutputDir.get().asFile
        listOf(
            "build.gradle",
            "build.gradle.kts",
            "settings.gradle",
            "settings.gradle.kts",
            "pom.xml",
            "gradlew",
            "gradlew.bat",
            "gradle",
            "README.md",
            ".openapi-generator-ignore",
            ".openapi-generator",
            "docs",
            "src/test",
        ).forEach { rel ->
            val f = root.resolve(rel)
            if (f.isDirectory) f.deleteRecursively() else if (f.exists()) f.delete()
        }
    }
}

listOf(
    "preBuild",
    "compileDebugKotlin",
    "compileReleaseKotlin",
    "compileDebugUnitTestKotlin",
    "compileReleaseUnitTestKotlin",
    "compileDebugAndroidTestKotlin",
).forEach { taskName ->
    tasks.matching { it.name == taskName }.configureEach {
        dependsOn("openApiGenerate")
    }
}

dependencies {
    val composeBom = platform(libs.compose.bom)
    implementation(composeBom)
    androidTestImplementation(composeBom)

    implementation(libs.androidx.core.ktx)
    implementation(libs.androidx.activity.compose)
    implementation(libs.androidx.lifecycle.runtime.ktx)
    implementation(libs.androidx.lifecycle.runtime.compose)
    implementation(libs.androidx.lifecycle.viewmodel.compose)
    implementation(libs.androidx.lifecycle.service)
    implementation(libs.androidx.navigation.compose)
    implementation(libs.androidx.datastore.preferences)
    implementation(libs.kotlinx.coroutines.android)
    implementation(libs.kotlinx.serialization.json)
    implementation(libs.okhttp)
    implementation(libs.webrtc)
    implementation(libs.compose.ui)
    implementation(libs.compose.ui.tooling.preview)
    implementation(libs.compose.foundation)
    implementation(libs.compose.material3)
    debugImplementation(libs.compose.ui.tooling)
    debugImplementation(libs.compose.ui.test.manifest)

    testImplementation(libs.junit)
    testImplementation(libs.turbine)
    testImplementation(libs.robolectric)
    testImplementation(libs.kotlinx.coroutines.test)
    testImplementation(libs.okhttp.mockwebserver)
    testImplementation(libs.androidx.test.core)

    androidTestImplementation(libs.androidx.test.runner)
    androidTestImplementation(libs.androidx.test.core)
    androidTestImplementation(libs.androidx.test.rules)
    androidTestImplementation(libs.androidx.espresso.core)
    androidTestImplementation(libs.androidx.uiautomator)
    androidTestImplementation(libs.compose.ui.test.junit4)
}
