#!/usr/bin/env bash
set -euo pipefail

if [[ $# -lt 3 || $# -gt 5 ]]; then
  echo "usage: prepare_darwin_runtime.sh <app-bundle> <app-executable> <ffi-dylib> [codesign-identity] [entitlements]" >&2
  exit 2
fi

app_bundle="$1"
app_executable="$2"
ffi_source="$3"
requested_identity="${4:-}"
entitlements_path="${5:-}"

frameworks_dir="$app_bundle/Contents/Frameworks"
bundled_ffi="$frameworks_dir/libopenviking_ffi.dylib"
argus_helper="$app_bundle/Contents/Helpers/Argus.app"
ffi_install_name="@rpath/libopenviking_ffi.dylib"
frameworks_rpath="@executable_path/../Frameworks"

if [[ ! -d "$app_bundle" ]]; then
  echo "missing app bundle: $app_bundle" >&2
  exit 1
fi
if [[ ! -f "$app_executable" ]]; then
  echo "missing app executable: $app_executable" >&2
  exit 1
fi
if [[ ! -f "$ffi_source" ]]; then
  echo "missing openviking ffi dylib: $ffi_source" >&2
  exit 1
fi

mkdir -p "$frameworks_dir"
cp "$ffi_source" "$bundled_ffi"
chmod 755 "$bundled_ffi"

current_ffi_ref="$(
  otool -L "$app_executable" | awk '/libopenviking_ffi\.dylib/ {print $1; exit}'
)"
if [[ -z "$current_ffi_ref" ]]; then
  echo "failed to locate libopenviking_ffi.dylib load command in $app_executable" >&2
  exit 1
fi

install_name_tool -id "$ffi_install_name" "$bundled_ffi"
if [[ "$current_ffi_ref" != "$ffi_install_name" ]]; then
  install_name_tool -change "$current_ffi_ref" "$ffi_install_name" "$app_executable"
fi
if ! otool -l "$app_executable" | grep -Fq "$frameworks_rpath"; then
  install_name_tool -add_rpath "$frameworks_rpath" "$app_executable"
fi

resolve_codesign_identity() {
  local requested="${1:-}"
  if [[ -n "$requested" ]]; then
    if [[ "$requested" == "-" ]]; then
      printf '%s\n' "-"
      return 0
    fi
    if security find-identity -v -p codesigning 2>/dev/null | grep -Fq "$requested"; then
      printf '%s\n' "$requested"
      return 0
    fi
    echo "requested codesign identity not found: $requested" >&2
    exit 1
  fi
  if security find-identity -v -p codesigning 2>/dev/null | grep -Fq "Argus Dev"; then
    printf '%s\n' "Argus Dev"
    return 0
  fi
  if [[ "${ALLOW_ADHOC_SIGNING:-0}" == "1" ]]; then
    printf '%s\n' "-"
    return 0
  fi
  cat >&2 <<'EOF'
no stable codesigning identity found.
set CODESIGN_IDENTITY to a valid identity, or set ALLOW_ADHOC_SIGNING=1 to opt into adhoc signing explicitly.
EOF
  exit 1
}

codesign_identity="$(resolve_codesign_identity "$requested_identity")"
echo "desktop packaging: using codesign identity: $codesign_identity"

codesign_file() {
  local path="$1"
  if [[ "$codesign_identity" == "-" ]]; then
    codesign --force --sign - "$path"
  else
    codesign --force --sign "$codesign_identity" "$path"
  fi
}

codesign_app_bundle() {
  local path="$1"
  if [[ "$codesign_identity" == "-" ]]; then
    codesign --force --sign - "$path"
    return 0
  fi

  local args=(
    --force
    --sign "$codesign_identity"
    --options runtime
  )
  if [[ -n "$entitlements_path" && -f "$entitlements_path" ]]; then
    args+=(--entitlements "$entitlements_path")
  fi
  args+=("$path")
  codesign "${args[@]}"
}

codesign_file "$bundled_ffi"
if [[ -d "$argus_helper" ]]; then
  if [[ "$codesign_identity" == "-" ]]; then
    codesign --force --deep --sign - "$argus_helper"
  else
    codesign --force --deep --sign "$codesign_identity" "$argus_helper"
  fi
fi
codesign_app_bundle "$app_bundle"
codesign --verify --deep --strict "$app_bundle"
