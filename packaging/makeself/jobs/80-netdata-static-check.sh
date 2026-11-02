#!/usr/bin/env bash
# SPDX-License-Identifier: GPL-3.0-or-later

# shellcheck source=./packaging/makeself/functions.sh
. "${NETDATA_MAKESELF_PATH}"/functions.sh "${@}" || exit 1

# shellcheck disable=SC2015
[ "${GITHUB_ACTIONS}" = "true" ] && echo "::group::Confirming that Netdata is statically linked" || true

# Read the configured feature state, including installer overrides and toolchain fallback.
# Job 70 configures in this directory before installing the binaries checked here.
cmake_cache="$(build_path netdata)/CMakeCache.txt"
if ipmi_enabled="$(sed -n 's/^ENABLE_PLUGIN_IPMI:BOOL=//p' "${cmake_cache}")"; then
  :
else
  printf >&2 "Failed to read IPMI feature state: %s\n" "${cmake_cache}"
  exit 1
fi
binaries=("${NETDATA_INSTALL_PATH}/bin/netdata")
ipmi_binary="${NETDATA_INSTALL_PATH}/usr/libexec/netdata/plugins.d/ipmi.plugin"
case "${ipmi_enabled^^}" in
  1|ON|TRUE|YES|Y) binaries+=("${ipmi_binary}") ;;
  0|OFF|FALSE|NO|N)
    # Inspect an installed plugin even if this build did not request it.
    if [ -e "${ipmi_binary}" ] || [ -L "${ipmi_binary}" ]; then
      binaries+=("${ipmi_binary}")
    fi
    ;;
  *)
    printf >&2 "Invalid or missing ENABLE_PLUGIN_IPMI in %s: %s\n" "${cmake_cache}" "${ipmi_enabled}"
    exit 1
    ;;
esac

for binary in "${binaries[@]}"; do
  if [ ! -x "${binary}" ]; then
    printf >&2 "Missing required binary: %s\n" "${binary}"
    exit 1
  fi
  if elf_headers="$(run readelf -l "${binary}")"; then
    :
  else
    status=$?
    printf >&2 "Failed to inspect ELF program headers: %s (status %s)\n" "${binary}" "${status}"
    exit "${status}"
  fi
  if printf '%s\n' "${elf_headers}" | grep 'INTERP'; then
    printf >&2 "Ooops. %s is not a statically linked binary!\n" "${binary}"
    ldd "${binary}"
    exit 1
  fi
done

# shellcheck disable=SC2015
[ "${GITHUB_ACTIONS}" = "true" ] && echo "::endgroup::" || true
