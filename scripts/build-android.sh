#!/usr/bin/env bash
set -euo pipefail

echo "1. Building Svelte Literary Studio UI..."
(cd web && npm ci && npm run build)

echo "2. Building Go Android AAR via gomobile..."
if ! command -v gomobile >/dev/null 2>&1; then
  go install golang.org/x/mobile/cmd/gomobile@latest
  go install golang.org/x/mobile/cmd/gobind@latest
  gomobile init
fi
mkdir -p mobile/android/app/libs
gomobile bind -v -androidapi 26 -target=android/arm64,android/arm -o mobile/android/app/libs/inkweavecore.aar ./mobile/gomobile

echo "3. Building Android APK..."
(cd mobile/android && gradle assembleRelease)
echo "Done! Check mobile/android/app/build/outputs/apk/release/"
