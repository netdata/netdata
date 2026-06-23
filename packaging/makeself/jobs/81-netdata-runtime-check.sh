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

static_systemd_unit_allows_capability() {
  local unit="${1}"
  local cap="${2}"

  awk -v wanted="${cap}" '
    BEGIN {
      wanted = toupper(wanted)
    }

    function trim(value) {
      sub(/^[[:space:]]+/, "", value)
      sub(/[[:space:]]+$/, "", value)
      return value
    }

    function contains_wanted(value,    count, caps, i) {
      count = split(value, caps, /[[:space:]]+/)
      for (i = 1; i <= count; i++) {
        if (toupper(caps[i]) == wanted) {
          return 1
        }
      }

      return 0
    }

    function apply_capability_bounding_set(line,    inverted, value) {
      if (line !~ /^[[:space:]]*CapabilityBoundingSet[[:space:]]*=/) {
        return
      }

      sub(/^[^=]*=/, "", line)
      value = trim(line)

      if (value == "") {
        allowed = 0
        found = 1
        return
      }

      if (value == "~") {
        allowed = 1
        found = 1
        return
      }

      inverted = 0
      if (substr(value, 1, 1) == "~") {
        inverted = 1
        value = trim(substr(value, 2))
      }

      if (inverted) {
        if (!found) {
          allowed = 1
        }

        found = 1
        if (contains_wanted(value)) {
          allowed = 0
        }
      } else {
        if (!found) {
          allowed = 0
        }

        found = 1
        if (contains_wanted(value)) {
          allowed = 1
        }
      }
    }

    {
      line = $0
      sub(/\r$/, "", line)

      if (continued == "" && line ~ /^[[:space:]]*[#;]/) {
        next
      }

      if (line ~ /\\[[:space:]]*$/) {
        sub(/\\[[:space:]]*$/, " ", line)
        continued = continued line
        next
      }

      line = continued line
      continued = ""
      apply_capability_bounding_set(line)
    }

    END {
      if (continued != "") {
        apply_capability_bounding_set(continued)
      }

      if (!found) {
        exit 2
      }

      exit allowed ? 0 : 1
    }
  ' "${unit}"
}

check_static_installer_systemd_capability_bounding_set() {
  local installer="${NETDATA_MAKESELF_PATH}/install-or-update.sh"
  local unit="${NETDATA_INSTALL_PATH}/usr/lib/netdata/system/systemd/netdata.service"
  local cap=
  local rc=
  local success=0

  [ -f "${installer}" ] || return 0
  [ -f "${unit}" ] || return 0

  if [ ! -r "${unit}" ]; then
    echo >&2 "!!! ${unit} is not a readable regular file"
    return 1
  fi

  while IFS= read -r cap; do
    [ -n "${cap}" ] || continue

    static_systemd_unit_allows_capability "${unit}" "${cap}"
    rc="${?}"
    case "${rc}" in
      0)
        ;;
      1)
        echo >&2 "!!! static installer grants ${cap}, but ${unit} CapabilityBoundingSet does not allow it"
        success=1
        ;;
      2)
        return 0
        ;;
      *)
        echo >&2 "!!! could not parse ${unit} CapabilityBoundingSet"
        success=1
        ;;
    esac
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
