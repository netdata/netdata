#!/usr/bin/env bash
# SPDX-License-Identifier: GPL-3.0-or-later

# shellcheck source=./packaging/makeself/functions.sh
. "${NETDATA_MAKESELF_PATH}"/functions.sh "${@}" || exit 1

# shellcheck disable=SC2015
[ "${GITHUB_ACTIONS}" = "true" ] && echo "::group::Performing basic runtime checks" || true

dump_log() {
  cat ./netdata.log
}

static_installer_declared_file_caps() {
  local installer="${1}"

  sed -n 's/.*run setcap "\([^"$]*\)" "usr\/libexec\/netdata\/plugins.d\/.*".*/\1/p' "${installer}" |
    awk '
      {
        for (i = 1; i <= NF; i++) {
          split($i, caps, /,/)
          for (cap in caps) {
            sub(/[=+].*/, "", caps[cap])
            if (caps[cap] ~ /^cap_/)
              print toupper(caps[cap])
          }
        }
      }
    ' |
    sort -u
}

static_systemd_unit_capability_bounding_set() {
  local unit="${1}"

  awk '
    /^[[:space:]]*CapabilityBoundingSet=/ {
      line = $0
      sub(/^[[:space:]]*CapabilityBoundingSet=/, "", line)
      split(line, caps, /[[:space:]]+/)
      for (i in caps) {
        if (caps[i] != "" && caps[i] !~ /^~/)
          print toupper(caps[i])
      }
    }
  ' "${unit}" | sort -u
}

check_static_installer_systemd_capability_bounding_set() {
  local installer="${NETDATA_MAKESELF_PATH}/install-or-update.sh"
  local unit="${NETDATA_INSTALL_PATH}/usr/lib/netdata/system/systemd/netdata.service"
  local bounding_caps=
  local cap=
  local success=0

  [ -f "${installer}" ] || return 0
  [ -f "${unit}" ] || return 0

  bounding_caps="$(static_systemd_unit_capability_bounding_set "${unit}")"
  [ -n "${bounding_caps}" ] || return 0

  while IFS= read -r cap; do
    [ -n "${cap}" ] || continue

    if ! printf '%s\n' "${bounding_caps}" | grep -qx "${cap}"; then
      echo >&2 "!!! static installer grants ${cap}, but ${unit} CapabilityBoundingSet does not allow it"
      success=1
    fi
  done < <(static_installer_declared_file_caps "${installer}")

  return "${success}"
}

trap dump_log EXIT

export NETDATA_LIBEXEC_PREFIX="${NETDATA_INSTALL_PATH}/usr/libexec/netdata"
export NETDATA_SKIP_LIBEXEC_PARTS="freeipmi|xenstat|cups"

case "${BUILDARCH}" in
    x86_64) ;;
    armv6l) NETDATA_SKIP_LIBEXEC_PARTS="${NETDATA_SKIP_LIBEXEC_PARTS}|ebpf|otel|systemd-journal" ;;
    *) NETDATA_SKIP_LIBEXEC_PARTS="${NETDATA_SKIP_LIBEXEC_PARTS}|ebpf" ;;
esac

check_static_installer_systemd_capability_bounding_set || exit 1

"${NETDATA_INSTALL_PATH}/bin/netdata" -D > ./netdata.log 2>&1 &

"${NETDATA_SOURCE_PATH}/packaging/runtime-check.sh" || exit 1

trap - EXIT

run rm -rv "${NETDATA_INSTALL_PATH}/var/lib/netdata" "${NETDATA_INSTALL_PATH}/var/cache/netdata"

# shellcheck disable=SC2015
[ "${GITHUB_ACTIONS}" = "true" ] && echo "::endgroup::" || true
