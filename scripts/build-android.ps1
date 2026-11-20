$ErrorActionPreference = "Stop"

Write-Host "1. Building Svelte Literary Studio UI..."
Push-Location web
npm.cmd ci
npm.cmd run build
Pop-Location

Write-Host "2. Building Go Android AAR via gomobile..."
if (-not (Get-Command gomobile -ErrorAction SilentlyContinue)) {
    go install golang.org/x/mobile/cmd/gomobile@latest
    go install golang.org/x/mobile/cmd/gobind@latest
    gomobile init
}
New-Item -ItemType Directory -Force -Path "mobile/android/app/libs" | Out-Null
gomobile bind -v -androidapi 26 -target=android/arm64,android/arm -o mobile/android/app/libs/inkweavecore.aar ./mobile/gomobile

Write-Host "3. Building Android APK with Gradle..."
Push-Location mobile/android
if (Test-Path "./gradlew.bat") {
    ./gradlew.bat assembleRelease
} else {
    gradle assembleRelease
}
Pop-Location
Write-Host "Done! Check mobile/android/app/build/outputs/apk/release/"
