plugins {
    id("com.android.application")
}

android {
    namespace = "io.github.andreychirkov42.clepsy"
    compileSdk = 37

    defaultConfig {
        applicationId = "io.github.andreychirkov42.clepsy"
        minSdk = 26
        targetSdk = 37
        versionCode = 1
        versionName = "1.1.2"
    }

    buildTypes {
        release {
            isMinifyEnabled = false
            // пока подписываем отладочным ключом этого ПК (~/.android/debug.keystore):
            // без того же ключа телефон не примет обновление поверх установленной версии
            signingConfig = signingConfigs.getByName("debug")
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
