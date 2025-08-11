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

    if [ "${success}" -eq 0 ]; then
        exit 1
    fi
fi
