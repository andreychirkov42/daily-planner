# Сборка Clepsy.apk: проверки как в build.ps1, Go-сервер под Android, затем APK через Gradle.
#   .\build-android.ps1             проверки + сборка APK
#   .\build-android.ps1 -Install    и установить на подключённый телефон или эмулятор (adb)
#   .\build-android.ps1 -SkipTests  без проверок (не рекомендуется)
# Установленный Clepsy.exe этот скрипт не трогает.
param(
    [switch]$SkipTests,
    [switch]$Install,
    [string]$Sdk = "D:\Android\Sdk",
    [string]$Jdk = "D:\Android\Android Studio\jbr"
)

$ErrorActionPreference = "Stop"
Set-Location $PSScriptRoot
$apk = Join-Path $PSScriptRoot "Clepsy.apk"

function Step([string]$name, [scriptblock]$action) {
    Write-Host "==> $name" -ForegroundColor Cyan
    # Gradle пишет предупреждения в stderr — успех шага решает только код выхода
    $ErrorActionPreference = "Continue"
    & $action 2>&1 | ForEach-Object { "$_" } | Out-Host
    if ($LASTEXITCODE -ne 0) {
        Write-Host "✗ $name — сборка остановлена, APK не изменён" -ForegroundColor Red
        exit 1
    }
}

if (-not $SkipTests) {
    Step "Форматирование (gofmt)" {
        $bad = gofmt -l .
        if ($bad) { Write-Host "Не отформатированы: $bad"; $global:LASTEXITCODE = 1 } else { $global:LASTEXITCODE = 0 }
    }
    Step "Проверка JS (node --check)" {
        $global:LASTEXITCODE = 0
        if (Get-Command node -ErrorAction SilentlyContinue) {
            foreach ($f in @("web\shared.js") + (Get-ChildItem web\js\*.js | ForEach-Object { $_.FullName })) {
                node --check $f
                if ($LASTEXITCODE -ne 0) { return }
            }
        } else { Write-Host "node не найден — пропускаю, страницы всё равно проверит браузерный тест" }
    }
    Step "go vet" { go vet ./... }
    Step "go vet (android)" { $env:GOOS = "android"; $env:GOARCH = "arm64"; $env:CGO_ENABLED = "0"; try { go vet ./... } finally { Remove-Item env:GOOS, env:GOARCH, env:CGO_ENABLED } }
    Step "Тесты: логика + страницы в браузере без окна" { go test -count=1 ./... }
}

# Сервер кладётся в APK как библиотека: Android распакует его в nativeLibraryDir, откуда разрешён запуск.
# arm64-v8a — телефоны (GOOS=android); x86_64 — эмулятор на ПК: android/amd64 без NDK не собирается,
# поэтому там обычный статический Linux-файл — ядро то же.
# timetzdata встраивает базу часовых поясов: на Android нет /etc/localtime, пояс приходит от оболочки в TZ.
# Антивирус Windows APK не проверяет, поэтому -s -w здесь можно.
$targets = [ordered]@{ "arm64-v8a" = @("android", "arm64"); "x86_64" = @("linux", "amd64") }
foreach ($abi in $targets.Keys) {
    $out = "android\app\src\main\jniLibs\$abi\libclepsy.so"
    Step "Go-сервер для $abi" {
        $env:GOOS = $targets[$abi][0]; $env:GOARCH = $targets[$abi][1]; $env:CGO_ENABLED = "0"
        try { go build -trimpath -tags timetzdata -ldflags "-s -w" -o $out . } finally { Remove-Item env:GOOS, env:GOARCH, env:CGO_ENABLED }
    }
}

if (-not (Test-Path "android\local.properties")) {
    # путь к SDK для Gradle; файл личный, в git не попадает
    "sdk.dir=" + ($Sdk -replace '\\', '\\' -replace ':', '\:') | Set-Content "android\local.properties" -Encoding ascii
}

# Java не может создать служебный сокет во временной папке с кириллицей в пути (C:\Users\Андрей\...)
$tmp = Join-Path (Split-Path $Sdk) "tmp"
New-Item -ItemType Directory $tmp -Force | Out-Null
$env:TMP = $tmp; $env:TEMP = $tmp
$env:JAVA_HOME = $Jdk

Step "Сборка APK (Gradle)" { Push-Location android; try { .\gradlew.bat assembleRelease --console=plain -q } finally { Pop-Location } }
Copy-Item "android\app\build\outputs\apk\release\app-release.apk" $apk -Force

if ($Install) {
    $adb = Join-Path $Sdk "platform-tools\adb.exe"
    Step "Установка (adb install)" { & $adb install -r $apk }
    Step "Запуск" { & $adb shell am start -n io.github.andreychirkov42.clepsy/.MainActivity }
}
Write-Host "✓ Готово: $apk" -ForegroundColor Green
