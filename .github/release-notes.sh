#!/usr/bin/env bash
# Writes the notes of the release TAG to stdout: the version's section of
# CHANGELOG.md, links to the files uploaded to the release, then the FAQ of
# release-template.md. Without the release, or before its uploads, the
# downloads are left out.
#
#   .github/release-notes.sh v0.2.0 [owner/repo]
set -euo pipefail

tag="$1"
repo="${2:-${GITHUB_REPOSITORY:-}}"
version="${tag#v}"
here="$(dirname "$0")"

awk -v v="$version" '
  /^## / { if (sec) exit; sec = ($2 == v || $2 == "[" v "]"); next }
  sec { print }
' "$here/../CHANGELOG.md" > "${TMPDIR:-/tmp}/notes.$$"
if ! grep -q '[^[:space:]]' "${TMPDIR:-/tmp}/notes.$$"; then
  echo "CHANGELOG.md has no section for $version" >&2
  exit 1
fi
cat "${TMPDIR:-/tmp}/notes.$$"
rm -f "${TMPDIR:-/tmp}/notes.$$"

# The names GitHub gave the uploads: it turns spaces and "~" into dots.
assets=""
if [ -n "$repo" ]; then
  assets="$(gh release view "$tag" --repo "$repo" --json assets -q '.assets[].name' 2> /dev/null || true)"
fi

# link LABEL GLOB prints "[LABEL](url)" for the first asset GLOB matches.
link() {
  local name
  while IFS= read -r name; do
    # shellcheck disable=SC2053 # the glob is meant to match
    if [[ -n "$name" && "$name" == $2 ]]; then
      printf '[%s](https://github.com/%s/releases/download/%s/%s)' "$1" "$repo" "$tag" "$name"
      return
    fi
  done <<< "$assets"
}

# row TITLE LINK... prints a list item of the links that exist, or nothing.
row() {
  local title="$1" out="" l
  shift
  for l in "$@"; do
    if [ -n "$l" ]; then out="${out:+$out | }$l"; fi
  done
  if [ -n "$out" ]; then printf -- '- %s：%s\n' "$title" "$out"; fi
}

if [ -n "$assets" ]; then
  printf '\n---\n\n## 📥 下载地址 / Downloads\n\n'

  printf '### Windows\n\n'
  row '安装版 Installer' "$(link '64 位 x64' '*Setup*amd64.exe')" "$(link 'ARM64' '*Setup*arm64.exe')"

  printf '\n### macOS\n\n'
  # mygo names the disk image "name version arm64.dmg" since 0.4.0, and
  # "name version-x64.dmg" before that; GitHub turns spaces into dots.
  arm="$(link 'DMG' '*arm64.dmg')"
  intel="$(link 'DMG' '*-x64.dmg')"
  if [ -z "$intel" ]; then intel="$(link 'DMG' '*.amd64.dmg')"; fi
  if [ -z "$intel" ]; then intel="$(link 'DMG' '*.x64.dmg')"; fi
  if [ -n "$arm" ]; then
    row 'Apple 芯片 Apple silicon' "$arm"
    row 'Intel 芯片 Intel' "$intel"
  else
    row 'Apple 芯片与 Intel 芯片通用 Universal' "$(link 'DMG' '*.dmg')"
  fi

  printf '\n### Linux\n\n'
  row 'Debian / Ubuntu (DEB)' "$(link 'x64' '*_amd64.deb')" "$(link 'ARM64' '*_arm64.deb')"
  row 'Fedora / openSUSE / RHEL (RPM)' "$(link 'x64' '*.x86_64.rpm')" "$(link 'ARM64' '*.aarch64.rpm')"
  row 'Arch Linux' "$(link 'x64' '*-x86_64.pkg.tar.zst')" "$(link 'ARM64' '*-aarch64.pkg.tar.zst')"
  row 'AppImage（任意发行版 Any distribution）' "$(link 'x64' '*-x86_64.AppImage')" "$(link 'ARM64' '*-aarch64.AppImage')"
  install_sh="$(link 'install.sh' 'install.sh')"
  if [ -n "$install_sh" ]; then
    printf -- '- 免 root 安装到 `~/.local`，可应用内自动更新 / Install for your user, without root, with in-app updates:\n\n'
    printf '  ```sh\n  curl -fsSL https://github.com/%s/releases/download/%s/install.sh | sh\n  ```\n' "$repo" "$tag"
  fi
fi

sed -e "s/{version}/$version/g" -e "s/{tag}/$tag/g" "$here/release-template.md"
