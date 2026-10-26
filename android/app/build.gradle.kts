import java.util.Properties

plugins {
    id("com.android.application")
}

// ключ подписи лежит вне git (android/keystore.properties → D:\Android\clepsy-release.jks);
// без файла APK подписывается отладочным ключом — так можно собрать на любом ПК, но не для релиза
val keystore = Properties().apply {
    val f = rootProject.file("keystore.properties")
    if (f.exists()) f.inputStream().use { load(it) }
}

android {
    namespace = "io.github.andreychirkov42.clepsy"
    compileSdk = 37

    defaultConfig {
        applicationId = "io.github.andreychirkov42.clepsy"
        minSdk = 26
        targetSdk = 37
        versionCode = 2
        versionName = "1.2.0-beta.1"
    }

    signingConfigs {
        if (keystore.isNotEmpty()) create("release") {
            storeFile = file(keystore.getProperty("storeFile"))
            storePassword = keystore.getProperty("storePassword")
            keyAlias = keystore.getProperty("keyAlias")
            keyPassword = keystore.getProperty("keyPassword")
        }
    }

    buildTypes {
        release {
            isMinifyEnabled = false
            // телефон принимает обновление только с той же подписью — ключ нельзя терять
            signingConfig = signingConfigs.findByName("release") ?: signingConfigs.getByName("debug")
        }
    }

    // Go-сервер лежит в APK как libclepsy.so и запускается отдельным процессом,
    // поэтому библиотеки должны распаковываться на диск при установке
    packaging {
        jniLibs {
            useLegacyPackaging = true
        }
    }

    compileOptions {
        sourceCompatibility = JavaVersion.VERSION_17
        targetCompatibility = JavaVersion.VERSION_17
    }
}
