#!/usr/bin/env bash
# SPDX-License-Identifier: GPL-3.0-or-later

# shellcheck source=./packaging/makeself/functions.sh
. "${NETDATA_MAKESELF_PATH}"/functions.sh "${@}" || exit 1

# shellcheck disable=SC2015
[ "${GITHUB_ACTIONS}" = "true" ] && echo "::group::Confirming that Netdata is statically linked" || true

# Verify the daemon and the default dedicated Go IPMI collector.
for binary in "${NETDATA_INSTALL_PATH}/bin/netdata" \
  "${NETDATA_INSTALL_PATH}/usr/libexec/netdata/plugins.d/ipmi.plugin"; do
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
