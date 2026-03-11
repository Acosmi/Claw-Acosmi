#!/usr/bin/env bash
set -euo pipefail

if [[ $# -lt 1 || $# -gt 3 ]]; then
  echo "usage: sign_darwin_bundle.sh <app-bundle> [codesign-identity] [entitlements]" >&2
  exit 2
fi

app_bundle="$1"
requested_identity="${2:-}"
entitlements_path="${3:-}"
framework_ffi="${app_bundle}/Contents/Frameworks/libopenviking_ffi.dylib"
argus_helper="${app_bundle}/Contents/Helpers/Argus.app"
runtime_enabled=0

if [[ ! -d "$app_bundle" ]]; then
  echo "missing app bundle: $app_bundle" >&2
  exit 1
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

  echo "desktop packaging: no stable codesigning identity found; falling back to adhoc" >&2
  printf '%s\n' "-"
}

codesign_identity="$(resolve_codesign_identity "$requested_identity")"
echo "desktop packaging: using codesign identity: $codesign_identity"

codesign_path() {
  local path="$1"
  if [[ "$codesign_identity" == "-" ]]; then
    codesign --force --sign - "$path"
  else
    codesign --force --sign "$codesign_identity" "$path"
  fi
}

signed_team_identifier() {
  local path="$1"
  local team_id

  team_id="$(
    codesign -dvv "$path" 2>&1 | sed -n 's/^TeamIdentifier=//p' | head -n 1
  )"
  if [[ "$team_id" == "not set" ]]; then
    team_id=""
  fi
  printf '%s\n' "$team_id"
}

codesign_bundle() {
  local path="$1"
  if [[ "$codesign_identity" == "-" ]]; then
    codesign --force --sign - "$path"
    return 0
  fi

  local args=(
    --force
    --sign "$codesign_identity"
  )
  if [[ "$runtime_enabled" == "1" ]]; then
    args+=(--options runtime)
  fi
  if [[ -n "$entitlements_path" && -f "$entitlements_path" ]]; then
    args+=(--entitlements "$entitlements_path")
  fi
  args+=("$path")
  codesign "${args[@]}"
}

if [[ -f "$framework_ffi" ]]; then
  codesign_path "$framework_ffi"
fi

if [[ "$codesign_identity" != "-" ]]; then
  framework_team_id=""
  if [[ -f "$framework_ffi" ]]; then
    framework_team_id="$(signed_team_identifier "$framework_ffi")"
  fi
  if [[ -n "$framework_team_id" ]]; then
    runtime_enabled=1
  else
    echo "desktop packaging: codesign identity has no TeamIdentifier; signing app without hardened runtime so bundled dylibs remain loadable" >&2
  fi
fi

if [[ -d "$argus_helper" ]]; then
  if [[ "$codesign_identity" == "-" ]]; then
    codesign --force --deep --sign - "$argus_helper"
  else
    codesign --force --deep --sign "$codesign_identity" "$argus_helper"
  fi
fi

codesign_bundle "$app_bundle"
codesign --verify --deep --strict "$app_bundle"
