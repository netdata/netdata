#!/usr/bin/env bash
# SPDX-License-Identifier: GPL-3.0-or-later

# shellcheck source=./packaging/makeself/functions.sh
. "${NETDATA_MAKESELF_PATH}"/functions.sh "${@}" || exit 1

# shellcheck disable=SC2015
[ "${GITHUB_ACTIONS}" = "true" ] && echo "::group::Confirming that Netdata is statically linked" || true

# Job 81 owns required-file checks. Inspect all installed ELF executables here;
# scripts and non-executable data do not have program headers to validate.
find "${NETDATA_INSTALL_PATH}" -type f \( -perm -100 -o -perm -010 -o -perm -001 \) -print0 |
while IFS= read -r -d '' binary; do
  magic="$(head -c 4 "${binary}")"
  [ "${magic}" = $'\177ELF' ] || continue
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
