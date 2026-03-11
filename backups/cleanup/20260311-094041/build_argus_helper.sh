#!/usr/bin/env bash
set -euo pipefail

if [[ $# -lt 2 || $# -gt 3 ]]; then
  echo "usage: build_argus_helper.sh <source-binary> <output-app-bundle> [version]" >&2
  exit 2
fi

source_binary="$1"
app_bundle="$2"
version="${3:-0.0.0-dev}"
executable_name="argus-sensory"
bundle_identifier="com.argus.sensory"

if [[ ! -f "$source_binary" ]]; then
  echo "missing Argus source binary: $source_binary" >&2
  exit 1
fi

rm -rf "$app_bundle"
mkdir -p "$app_bundle/Contents/MacOS"
mkdir -p "$app_bundle/Contents/Resources"

cp "$source_binary" "$app_bundle/Contents/MacOS/$executable_name"
chmod 755 "$app_bundle/Contents/MacOS/$executable_name"

cat >"$app_bundle/Contents/Info.plist" <<EOF
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
  <dict>
    <key>CFBundleDevelopmentRegion</key>
    <string>en</string>
    <key>CFBundleDisplayName</key>
    <string>Argus</string>
    <key>CFBundleExecutable</key>
    <string>${executable_name}</string>
    <key>CFBundleIdentifier</key>
    <string>${bundle_identifier}</string>
    <key>CFBundleInfoDictionaryVersion</key>
    <string>6.0</string>
    <key>CFBundleName</key>
    <string>Argus</string>
    <key>CFBundlePackageType</key>
    <string>APPL</string>
    <key>CFBundleShortVersionString</key>
    <string>${version}</string>
    <key>CFBundleVersion</key>
    <string>${version}</string>
    <key>LSBackgroundOnly</key>
    <true/>
    <key>LSMinimumSystemVersion</key>
    <string>10.15.0</string>
  </dict>
</plist>
EOF
