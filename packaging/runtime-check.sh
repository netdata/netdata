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
    for unit in \
        ${NETDATA_SYSTEMD_UNIT:-} \
        /etc/systemd/system/netdata.service \
        /usr/lib/systemd/system/netdata.service \
        /lib/systemd/system/netdata.service \
        /usr/local/lib/systemd/system/netdata.service; do
        if [ -f "${unit}" ]; then
            printf "%s\n" "${unit}"
            return 0
        fi
    done

    return 1
}

systemd_unit_capability_bounding_set() {
    awk '
        /^[[:space:]]*CapabilityBoundingSet[[:space:]]*=/ {
            line = $0
            sub(/^[^=]*=/, "", line)
            count = split(line, caps, /[[:space:]]+/)
            for (i = 1; i <= count; i++) {
                if (caps[i] != "") {
                    print caps[i]
                }
            }
            found = 1
        }
        END {
            if (!found) {
                exit 1
            }
        }
    ' "${1}"
}

check_systemd_capability_bounding_set() {
    command -v getcap >/dev/null 2>&1 || return 0

    systemd_unit="$(find_netdata_systemd_unit)" || return 0
    bounding_caps="$(systemd_unit_capability_bounding_set "${systemd_unit}")" || return 0

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

                if ! printf "%s\n" "${bounding_caps}" | grep -qx "${cap_name}"; then
                    cap_success=0
                    printf "!!! %s has file capability %s, but %s CapabilityBoundingSet does not allow it\n" \
                        "${plugin}" "${cap_name}" "${systemd_unit}"
                fi

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
