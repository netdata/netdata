#!/bin/sh

wait_for() {
  host="${1}"
  port="${2}"
  name="${3}"
  timeout="30"

  if command -v nc > /dev/null ; then
    netcat="nc"
  elif command -v netcat > /dev/null ; then
    netcat="netcat"
  else
    printf "Unable to find a usable netcat command.\n"
    return 1
  fi

  printf "Waiting for %s on %s:%s ... " "${name}" "${host}" "${port}"

  sleep 30

  i=0
  while ! ${netcat} -z "${host}" "${port}"; do
    sleep 1
    if [ "$i" -gt "$timeout" ]; then
      printf "Timed out!\n"
      return 2
    fi
    i="$((i + 1))"
  done
  printf "OK\n"
}

skip_libexec_part() {
    [ -n "${NETDATA_SKIP_LIBEXEC_PARTS}" ] && printf "%s\n" "${1}" | grep -qE "${NETDATA_SKIP_LIBEXEC_PARTS}"
}

find_netdata_systemd_unit() {
    if [ -n "${NETDATA_SYSTEMD_UNIT}" ]; then
        if [ -f "${NETDATA_SYSTEMD_UNIT}" ] && [ -r "${NETDATA_SYSTEMD_UNIT}" ]; then
            printf "%s\n" "${NETDATA_SYSTEMD_UNIT}"
            return 0
        fi

        printf "!!! %s is not a readable regular file\n" "${NETDATA_SYSTEMD_UNIT}" >&2
        return 2
    fi

    for unit in \
        /etc/systemd/system/netdata.service \
        /usr/lib/systemd/system/netdata.service \
        /lib/systemd/system/netdata.service \
        /usr/local/lib/systemd/system/netdata.service; do
        if [ ! -e "${unit}" ] && [ ! -L "${unit}" ]; then
            continue
        fi

        if [ -f "${unit}" ] && [ -r "${unit}" ]; then
            printf "%s\n" "${unit}"
            return 0
        fi

        printf "!!! %s is not a readable regular file\n" "${unit}" >&2
        return 2
    done

    return 1
}

systemd_unit_allows_capability() {
    awk -v wanted="${2}" '
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
    ' "${1}"
}

check_systemd_capability_bounding_set() {
    command -v getcap >/dev/null 2>&1 || return 0

    systemd_unit="$(find_netdata_systemd_unit)"
    case "${?}" in
        0)
            ;;
        1)
            return 0
            ;;
        *)
            return 1
            ;;
    esac

    cap_success=1
    for part in ${NETDATA_LIBEXEC_PARTS}; do
        if skip_libexec_part "${part}"; then
            continue
        fi

        plugin="${NETDATA_LIBEXEC_PREFIX}/${part}"
        if [ ! -f "${plugin}" ]; then
            continue
        fi

        file_caps="$(getcap "${plugin}" 2>/dev/null || true)"
        if [ -z "${file_caps}" ]; then
            continue
        fi

        file_caps="${file_caps#* }"
        for cap_spec in ${file_caps}; do
            cap_names="${cap_spec%%[=+]*}"

            old_ifs="${IFS}"
            IFS=","
            for cap_name in ${cap_names}; do
                IFS="${old_ifs}"
                cap_name="$(printf "%s" "${cap_name}" | tr "[:lower:]" "[:upper:]")"

                systemd_unit_allows_capability "${systemd_unit}" "${cap_name}"
                case "${?}" in
                    0)
                        ;;
                    1)
                        cap_success=0
                        printf "!!! %s has file capability %s, but %s CapabilityBoundingSet does not allow it\n" \
                            "${plugin}" "${cap_name}" "${systemd_unit}"
                        ;;
                    2)
                        return 0
                        ;;
                    *)
                        cap_success=0
                        printf "!!! could not parse %s CapabilityBoundingSet\n" "${systemd_unit}"
                        ;;
                esac

                IFS=","
            done
            IFS="${old_ifs}"
        done
    done

    [ "${cap_success}" -eq 1 ]
}

wait_for localhost 19999 netdata

case $? in
    1) exit 2 ;;
    2) exit 3 ;;
esac

curl -sfS http://127.0.0.1:19999/api/v1/info > /tmp/response || exit 1

cat /tmp/response

jq '.version' /tmp/response || exit 1

curl -sfS http://127.0.0.1:19999/index.html || exit 1
curl -sfS http://127.0.0.1:19999/v0/index.html || exit 1
curl -sfS http://127.0.0.1:19999/v1/index.html || exit 1
curl -sfS http://127.0.0.1:19999/v2/index.html || exit 1

NETDATA_LIBEXEC_PARTS="
plugins.d/apps.plugin
plugins.d/cgroup-network
plugins.d/cgroup-name
plugins.d/charts.d.plugin
plugins.d/cups.plugin
plugins.d/debugfs.plugin
plugins.d/ebpf.plugin
plugins.d/freeipmi.plugin
plugins.d/go.d.plugin
plugins.d/snmp-trap-profile-gen
plugins.d/ioping.plugin
plugins.d/local-listeners
plugins.d/ndsudo
plugins.d/network-viewer.plugin
plugins.d/nfacct.plugin
plugins.d/otel-plugin
plugins.d/perf.plugin
plugins.d/python.d.plugin
plugins.d/slabinfo.plugin
plugins.d/xenstat.plugin
"

if [ -d "${NETDATA_LIBEXEC_PREFIX}" ]; then
    success=1
    for part in ${NETDATA_LIBEXEC_PARTS}; do
        # shellcheck disable=SC2254
        if echo "${part}" | grep -qE "${NETDATA_SKIP_LIBEXEC_PARTS}"; then
            continue
        fi

        if [ ! -x "${NETDATA_LIBEXEC_PREFIX}/${part}" ]; then
            success=0
            echo "!!! ${NETDATA_LIBEXEC_PREFIX}/${part} is missing"
        fi
    done

    ebpf_plugin="${NETDATA_LIBEXEC_PREFIX}/plugins.d/ebpf.plugin"
    ebpf_go_plugin="${NETDATA_LIBEXEC_PREFIX}/plugins.d/ebpf-go.plugin"
    if [ -f "${ebpf_plugin}" ] && [ -f "${ebpf_go_plugin}" ]; then
        ebpf_mode="$(stat -c '%a' "${ebpf_plugin}")"
        ebpf_go_mode="$(stat -c '%a' "${ebpf_go_plugin}")"
        if [ "${ebpf_mode}" != "${ebpf_go_mode}" ]; then
            success=0
            echo "!!! ${ebpf_go_plugin} has mode ${ebpf_go_mode}, expected ${ebpf_mode}"
        fi
    elif [ -f "${ebpf_plugin}" ] && [ ! -e "${ebpf_go_plugin}" ]; then
        :
    fi

    if ! check_systemd_capability_bounding_set; then
        success=0
    fi

    if [ "${success}" -eq 0 ]; then
        exit 1
    fi
fi
