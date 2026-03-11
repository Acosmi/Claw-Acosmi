#!/usr/bin/env bash
set -euo pipefail

if [[ $# -ne 3 ]]; then
  echo "usage: stage_openviking_framework.sh <app-executable> <frameworks-dir> <ffi-dylib-source>" >&2
  exit 2
fi

app_executable="$1"
frameworks_dir="$2"
ffi_source="$3"
ffi_name="libopenviking_ffi.dylib"
ffi_ref="@rpath/${ffi_name}"
frameworks_rpath="@executable_path/../Frameworks"
bundled_ffi="${frameworks_dir}/${ffi_name}"

if [[ ! -f "$app_executable" ]]; then
  echo "missing app executable: $app_executable" >&2
  exit 1
fi
if [[ ! -f "$ffi_source" ]]; then
  echo "missing ffi dylib: $ffi_source" >&2
  exit 1
fi

mkdir -p "$frameworks_dir"
cp "$ffi_source" "$bundled_ffi"
chmod 755 "$bundled_ffi"

current_ref="$(
  otool -L "$app_executable" | awk '/libopenviking_ffi\.dylib/ { print $1; exit }'
)"
if [[ -z "$current_ref" ]]; then
  echo "failed to locate ${ffi_name} dependency in $app_executable" >&2
  exit 1
fi

install_name_tool -id "$ffi_ref" "$bundled_ffi"
install_name_tool -change "$current_ref" "$ffi_ref" "$app_executable"

if ! otool -l "$app_executable" | grep -Fq "$frameworks_rpath"; then
  install_name_tool -add_rpath "$frameworks_rpath" "$app_executable"
fi

if ! otool -L "$app_executable" | grep -Fq "$ffi_ref"; then
  echo "failed to rewrite ${ffi_name} dependency to ${ffi_ref}" >&2
  exit 1
fi
