plugins {
    id("com.android.application")
    id("org.jetbrains.kotlin.android")
    id("org.jetbrains.kotlin.plugin.compose")
    id("com.google.gms.google-services")
}

android {
    namespace = "com.kinjo.app"
    compileSdk = 35

    defaultConfig {
        applicationId = "com.kinjo.app"
        minSdk = 29
        targetSdk = 35
        versionCode = 1
        versionName = "0.1"
        // Build with -PkinjoApi=https://your.server for a shareable APK; the default reaches a dev server over adb reverse.
        val api = (project.findProperty("kinjoApi") as String?) ?: "http://localhost:8001"
        buildConfigField("String", "API_BASE", "\"$api\"")
    }

    // No debug suffix: Firebase push is registered for com.kinjo.app only (app/google-services.json, not in git).
    buildFeatures {
        compose = true
        buildConfig = true
    }

    compileOptions {
        sourceCompatibility = JavaVersion.VERSION_17
        targetCompatibility = JavaVersion.VERSION_17
    }
    kotlinOptions { jvmTarget = "17" }
}

dependencies {
    implementation("androidx.core:core-ktx:1.13.1")
    implementation("com.google.android.gms:play-services-location:21.3.0")
    implementation("androidx.activity:activity-compose:1.9.3")
    implementation(platform("androidx.compose:compose-bom:2024.12.01"))
    implementation("androidx.compose.foundation:foundation")
    implementation("androidx.compose.ui:ui")
    implementation("com.squareup.okhttp3:okhttp:4.12.0")
    implementation("io.coil-kt:coil-compose:2.7.0")
    implementation(platform("com.google.firebase:firebase-bom:33.7.0"))
    implementation("com.google.firebase:firebase-messaging") // push transport only: wakes, no Firebase auth or data
    testImplementation("junit:junit:4.13.2")
}
