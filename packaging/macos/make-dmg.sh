#!/bin/sh
# Rebuilds the disk image of every build/darwin-*/ directory with the MyGO-Clash
# window (background, arrow, icon positions, volume icon), replacing the plain
# image that mygo made, under the same file name.
#
#   packaging/macos/make-dmg.sh [build-dir ...]      (default: build/darwin-*)
#
# Needs dmgbuild (pip install dmgbuild). Environment:
#   CODESIGN_IDENTITY  sign the image with this identity (default: leave it
#                      unsigned, like mygo does for ad-hoc builds)
set -eu

here=$(cd "$(dirname "$0")" && pwd)
root=$(cd "$here/../.." && pwd)

if ! command -v dmgbuild > /dev/null 2>&1; then
  echo "make-dmg: dmgbuild not found; install it with: pip install dmgbuild" >&2
  exit 1
fi

if [ "$#" -eq 0 ]; then
  set -- "$root"/build/darwin-*
fi

# mygo notarizes the image itself when macos.notarize is configured; this
# script does not, so it leaves mygo's images alone then.
if python3 -I -c 'import json,sys; sys.exit(0 if json.load(open(sys.argv[1])).get("macos",{}).get("notarize") else 1)' "$root/mygo.json"; then
  echo "make-dmg: macos.notarize is configured, keeping mygo's disk images" >&2
  exit 0
fi

# The volume name mygo uses: macos.dmgTitle, else the app name.
volname=$(python3 -I -c 'import json,sys; c=json.load(open(sys.argv[1])); print(c.get("macos",{}).get("dmgTitle") or c["name"])' "$root/mygo.json")

# Built in a temporary directory on the local disk, which hdiutil needs.
work=$(mktemp -d "${TMPDIR:-/tmp}/mygo-dmg.XXXXXX")
trap 'rm -rf "$work"' EXIT INT TERM

for dir in "$@"; do
  [ -d "$dir" ] || continue
  app=$(ls -d "$dir"/*.app 2> /dev/null | head -n 1)
  dmg=$(ls "$dir"/*.dmg 2> /dev/null | head -n 1)
  if [ -z "$app" ] || [ -z "$dmg" ]; then
    echo "make-dmg: no .app and .dmg in $dir, skipping" >&2
    continue
  fi
  icon=$(ls "$app"/Contents/Resources/*.icns 2> /dev/null | head -n 1)

  echo "make-dmg: $(basename "$dmg")"
  out="$work/$(basename "$dmg")"
  if [ -n "$icon" ]; then
    dmgbuild -s "$here/dmgbuild-settings.py" -D app="$app" -D here="$here" -D icon="$icon" "$volname" "$out"
  else
    dmgbuild -s "$here/dmgbuild-settings.py" -D app="$app" -D here="$here" "$volname" "$out"
  fi

  if [ -n "${CODESIGN_IDENTITY:-}" ] && [ "$CODESIGN_IDENTITY" != "-" ]; then
    codesign --force --timestamp --sign "$CODESIGN_IDENTITY" "$out"
  fi
  mv -f "$out" "$dmg"
done
