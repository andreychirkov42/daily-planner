# Сборка Flask.exe: сначала все проверки, потом exe.
# Если что-то не прошло, exe не собирается и старая версия остаётся как была.
#   .\build.ps1             проверки + сборка; запущенная программа перезапускается
#   .\build.ps1 -SkipTests  только сборка (не рекомендуется)
param([switch]$SkipTests)

$ErrorActionPreference = "Stop"
Set-Location $PSScriptRoot
$exe = Join-Path $PSScriptRoot "Flask.exe"

function Step([string]$name, [scriptblock]$action) {
    Write-Host "==> $name" -ForegroundColor Cyan
    & $action
    if ($LASTEXITCODE -ne 0) {
        Write-Host "✗ $name — сборка остановлена, exe не изменён" -ForegroundColor Red
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
    Step "Тесты: логика + страницы в браузере без окна" { go test -count=1 ./... }
}

# запущенный exe заблокирован Windows — останавливаем его и запускаем снова после сборки
$running = Get-Process | Where-Object { $_.Path -eq $exe }
if ($running) {
    Write-Host "==> Останавливаю запущенный Flask" -ForegroundColor Cyan
    $running | Stop-Process -Force
    Start-Sleep -Milliseconds 800
}

Step "Сборка exe" { go build -ldflags "-H windowsgui -s -w" -o $exe . }

if ($running) {
    Start-Process -FilePath $exe -ArgumentList "-autostart" -WorkingDirectory $PSScriptRoot
    Write-Host "==> Flask снова запущен в трее" -ForegroundColor Cyan
}
Write-Host "✓ Готово: $exe" -ForegroundColor Green
