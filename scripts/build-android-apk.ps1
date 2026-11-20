$ErrorActionPreference = 'Stop'
$root = Split-Path -Parent $PSScriptRoot
Push-Location $root

try {
    $base = "$env:USERPROFILE\.android-build-env"
    if (-not $env:JAVA_HOME -and (Test-Path "$base\jdk21")) { $env:JAVA_HOME = "$base\jdk21" }
    if (-not $env:ANDROID_HOME -and (Test-Path "$base\sdk")) { $env:ANDROID_HOME = "$base\sdk" }
    $env:ANDROID_SDK_ROOT = $env:ANDROID_HOME

    $gradle = if (Get-Command gradle -ErrorAction SilentlyContinue) { "gradle" } else { "$base\gradle-8.11.1\bin\gradle.bat" }
    if (-not (Test-Path $gradle) -and -not (Get-Command gradle -ErrorAction SilentlyContinue)) {
        throw "Gradle not found. Install Gradle 8.11+ or set up $base"
    }

    Write-Host "1/4 Building Svelte 5 Web UI..."
    Push-Location web
    npm.cmd run build
    Pop-Location

    Write-Host "2/4 Cross-compiling pure-Go engine (arm64-v8a, armeabi-v7a, x86_64)..."
    $jni = "mobile\android\app\src\main\jniLibs"
    New-Item -ItemType Directory -Force -Path "$jni\arm64-v8a", "$jni\armeabi-v7a", "$jni\x86_64" | Out-Null

    $env:CGO_ENABLED="0"; $env:GOOS="linux"; $env:GOARCH="arm64"; Remove-Item Env:GOARM -ErrorAction SilentlyContinue
    go build -trimpath -ldflags="-s -w" -o "$jni\arm64-v8a\libinkweave.so" ./cmd/inkweave

    $env:CGO_ENABLED="0"; $env:GOOS="linux"; $env:GOARCH="arm"; $env:GOARM="7"
    go build -trimpath -ldflags="-s -w" -o "$jni\armeabi-v7a\libinkweave.so" ./cmd/inkweave

    $env:CGO_ENABLED="0"; $env:GOOS="linux"; $env:GOARCH="amd64"; Remove-Item Env:GOARM -ErrorAction SilentlyContinue
    go build -trimpath -ldflags="-s -w" -o "$jni\x86_64\libinkweave.so" ./cmd/inkweave
    Remove-Item Env:GOOS, Env:GOARCH, Env:GOARM, Env:CGO_ENABLED -ErrorAction SilentlyContinue

    Write-Host "3/4 Building signed Release APK..."
    $ksFile = "mobile\android\keystore\inkweave-release.jks"
    if (-not (Test-Path $ksFile)) {
        New-Item -ItemType Directory -Force -Path "mobile\android\keystore" | Out-Null
        & "$env:JAVA_HOME\bin\keytool.exe" -genkeypair -v `
            -keystore $ksFile -storetype PKCS12 -alias inkweave `
            -keyalg RSA -keysize 2048 -validity 9125 `
            -storepass "inkweave-android-2026" -keypass "inkweave-android-2026" `
            -dname "CN=Inkweave Studio, OU=Mobile, O=Inkweave Open Source, L=Bangkok, ST=Bangkok, C=TH"
    }
    Push-Location "mobile\android"
    & $gradle --no-daemon assembleRelease
    if ($LASTEXITCODE -ne 0) { throw "Gradle build failed" }
    Pop-Location

    Write-Host "4/4 Exporting & verifying signed APK..."
    New-Item -ItemType Directory -Force -Path "dist" | Out-Null
    $outApk = "dist\Inkweave-v0.2.0-signed.apk"
    Copy-Item "mobile\android\app\build\outputs\apk\release\app-release.apk" $outApk -Force
    & "$env:ANDROID_HOME\build-tools\35.0.0\apksigner.bat" verify --verbose $outApk
    Write-Host "Signed APK ready: $(Resolve-Path $outApk)"
} finally {
    Pop-Location
}
