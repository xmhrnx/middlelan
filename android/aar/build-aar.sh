#!/bin/sh
# Packs MiddleMan's extra Android manifest entries and resources into android/libs/middlelan.aar,
# which giode build merges into the APK (giode.toml: android.libraries).
# Run from anywhere after changing files in android/aar.
set -e
here=$(cd "$(dirname "$0")" && pwd)
out="$here/../libs/middlelan.aar"
mkdir -p "$(dirname "$out")"
rm -f "$out"
cd "$here"
zip -qr -X "$out" AndroidManifest.xml res
echo "wrote $out"
