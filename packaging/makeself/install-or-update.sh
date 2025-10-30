#!/usr/bin/env bash

# SPDX-License-Identifier: GPL-3.0-or-later

# shellcheck source=./packaging/makeself/functions.sh
. "$(dirname "${0}")"/functions.sh

export LC_ALL=C
umask 002

# Be nice on production environments
renice 19 $$ >/dev/null 2>/dev/null

NETDATA_PREFIX="/opt/netdata"
NETDATA_USER_CONFIG_DIR="${NETDATA_PREFIX}/etc/netdata"

# -----------------------------------------------------------------------------
if [ -d /opt/netdata/etc/netdata.old ]; then
  progress "Found old etc/netdata directory, reinstating this"
  [ -d /opt/netdata/etc/netdata.new ] && rm -rf /opt/netdata/etc/netdata.new
  mv -f /opt/netdata/etc/netdata /opt/netdata/etc/netdata.new
  mv -f /opt/netdata/etc/netdata.old /opt/netdata/etc/netdata

  progress "Trigger stock config clean up"
  rm -f /opt/netdata/etc/netdata/.installer-cleanup-of-stock-configs-done
fi

STARTIT=1
REINSTALL_OPTIONS=""
NETDATA_CERT_MODE="${NETDATA_CERT_MODE:-check}"
NETDATA_CERT_TEST_URL="${NETDATA_CERT_TEST_URL:-https://app.netdata.cloud}"
RELEASE_CHANNEL="nightly"

while [ "${1}" ]; do
  case "${1}" in
    "--dont-start-it")
      STARTIT=0
      REINSTALL_OPTIONS="${REINSTALL_OPTIONS} ${1}"
      ;;
    "--auto-update" | "-u") ;;
    "--stable-channel")
      RELEASE_CHANNEL="stable"
      REINSTALL_OPTIONS="${REINSTALL_OPTIONS} ${1}"
      ;;
    "--nightly-channel")
      RELEASE_CHANNEL="nightly"
      REINSTALL_OPTIONS="${REINSTALL_OPTIONS} ${1}"
      ;;
    "--disable-telemetry")
      NETDATA_DISABLE_TELEMETRY=1
      REINSTALL_OPTIONS="${REINSTALL_OPTIONS} ${1}"
      ;;
    "--certificates")
      case "${2}" in
        auto | system) NETDATA_CERT_MODE="auto" ;;
        check) NETDATA_CERT_MODE="check" ;;
        bundled) NETDATA_CERT_MODE="bundled" ;;
        *)
          run_failed "Unknown certificate handling mode '${2}'. Supported modes are auto, check, system, and bundled."
          exit 1
          ;;
      esac
      shift 1
      ;;
    "--certificate-test-url")
      NETDATA_CERT_TEST_URL="${2}"
      shift 1
      ;;

    *) echo >&2 "Unknown option '${1}'. Ignoring it." ;;
  esac
  shift 1
done

if [ ! "${DISABLE_TELEMETRY:-0}" -eq 0 ] ||
  [ -n "$DISABLE_TELEMETRY" ] ||
  [ ! "${DO_NOT_TRACK:-0}" -eq 0 ] ||
  [ -n "$DO_NOT_TRACK" ]; then
  NETDATA_DISABLE_TELEMETRY=1
  REINSTALL_OPTIONS="${REINSTALL_OPTIONS} --disable-telemetry"
fi

if [ -n "${NETDATA_CERT_MODE}" ]; then
  REINSTALL_OPTIONS="${REINSTALL_OPTIONS} --certificates ${NETDATA_CERT_MODE}"
fi

if [ -n "${NETDATA_CERT_TEST_URL}" ]; then
  REINSTALL_OPTIONS="${REINSTALL_OPTIONS} --certificate-test-url ${NETDATA_CERT_TEST_URL}"
fi

# -----------------------------------------------------------------------------
progress "Attempt to create user/group netdata/netadata"

NETDATA_WANTED_GROUPS="docker nginx varnish haproxy adm nsd proxy squid ceph nobody I2C"
NETDATA_ADDED_TO_GROUPS=""
# Default user/group
NETDATA_USER="netdata"
NETDATA_GROUP="netdata"

create_netdata_accounts

# -----------------------------------------------------------------------------
progress "Install logrotate configuration for netdata"

install_netdata_logrotate || run_failed "Cannot install logrotate file for netdata."

progress "Install journald configuration for netdata"

install_netdata_journald_conf || run_failed "Cannot install journald file for netdata."

# -----------------------------------------------------------------------------
progress "Telemetry configuration"

# Opt-out from telemetry program
if [ -n "${NETDATA_DISABLE_TELEMETRY}" ]; then
  run touch "${NETDATA_USER_CONFIG_DIR}/.opt-out-from-anonymous-statistics"
else
  printf "You can opt out from anonymous statistics via the --disable-telemetry option, or by creating an empty file %s \n\n" "${NETDATA_USER_CONFIG_DIR}/.opt-out-from-anonymous-statistics"
fi

# -----------------------------------------------------------------------------
progress "Install netdata at system init"

install_netdata_service || run_failed "Cannot install netdata init service."

set_netdata_updater_channel || run_failed "Cannot set netdata updater tool release channel to '${RELEASE_CHANNEL}'"

# -----------------------------------------------------------------------------
progress "Install (but not enable) netdata updater tool"
install_netdata_updater || run_failed "Cannot install netdata updater tool."

# -----------------------------------------------------------------------------
progress "creating quick links"

dir_should_be_link() {
  local p="${1}" t="${2}" d="${3}" old

  old="${PWD}"
  cd "${p}" || return 0

  if [ -e "${d}" ]; then
    if [ -h "${d}" ]; then
      run rm "${d}"
    else
      run mv -f "${d}" "${d}.old.$$"
    fi
  fi

  run ln -s "${t}" "${d}"
  cd "${old}" || true
}

dir_should_be_link . bin sbin
dir_should_be_link usr ../bin bin
dir_should_be_link usr ../bin sbin
dir_should_be_link usr . local

dir_should_be_link . etc/netdata netdata-configs
dir_should_be_link . usr/share/netdata/web netdata-web-files
dir_should_be_link . usr/libexec/netdata netdata-plugins
dir_should_be_link . var/lib/netdata netdata-dbs
dir_should_be_link . var/cache/netdata netdata-metrics
dir_should_be_link . var/log/netdata netdata-logs

dir_should_be_link etc/netdata ../../usr/lib/netdata/conf.d orig

# -----------------------------------------------------------------------------
progress "fix permissions"

run chmod g+rx,o+rx /opt
run find /opt/netdata -type d -exec chmod go+rx '{}' \+

install_netdata_tmpfiles

if command -v systemd-tmpfiles >/dev/null 2>&1; then
  run systemd-tmpfiles --create /usr/lib/tmpfiles.d/netdata.conf
else
  run chown -R ${NETDATA_USER}:${NETDATA_GROUP} /opt/netdata/var
fi

if [ -d /opt/netdata/usr/libexec/netdata/plugins.d/ebpf.d ]; then
  run chown -R root:${NETDATA_GROUP} /opt/netdata/usr/libexec/netdata/plugins.d/ebpf.d
fi

# -----------------------------------------------------------------------------

progress "changing plugins ownership and permissions"

for x in ndsudo apps.plugin perf.plugin slabinfo.plugin debugfs.plugin freeipmi.plugin ioping cgroup-network local-listeners network-viewer.plugin ebpf.plugin nfacct.plugin xenstat.plugin python.d.plugin charts.d.plugin go.d.plugin ioping.plugin cgroup-network-helper.sh; do
  f="usr/libexec/netdata/plugins.d/${x}"
  if [ -f "${f}" ]; then
    run chown root:${NETDATA_GROUP} "${f}"
  fi
done

if command -v setcap >/dev/null 2>&1; then
  if ! run setcap "cap_dac_read_search,cap_sys_ptrace=ep" "usr/libexec/netdata/plugins.d/apps.plugin"; then
    run chmod 4750 "usr/libexec/netdata/plugins.d/apps.plugin"
  fi
  if ! run setcap "cap_dac_read_search=ep" "usr/libexec/netdata/plugins.d/slabinfo.plugin"; then
    run chmod 4750 "usr/libexec/netdata/plugins.d/slabinfo.plugin"
  fi
  if ! run setcap "cap_dac_read_search=ep" "usr/libexec/netdata/plugins.d/debugfs.plugin"; then
    run chmod 4750 "usr/libexec/netdata/plugins.d/debugfs.plugin"
  fi
  if ! run setcap "cap_dac_read_search+epi cap_net_admin+epi cap_net_raw=eip" "usr/libexec/netdata/plugins.d/go.d.plugin"; then
    run chmod 4750 "usr/libexec/netdata/plugins.d/go.d.plugin"
  fi

  perf_caps="cap_sys_admin=ep"
  if command -v capsh >/dev/null 2>&1 && capsh --supports=cap_perfmon 2>/dev/null; then
    perf_caps="cap_perfmon=ep"
  fi

  if ! run setcap "${perf_caps}" "usr/libexec/netdata/plugins.d/perf.plugin"; then
    run chmod 4750 "usr/libexec/netdata/plugins.d/perf.plugin"
  fi
else
  for x in apps.plugin perf.plugin slabinfo.plugin debugfs.plugin; do
    f="usr/libexec/netdata/plugins.d/${x}"
    run chmod 4750 "${f}"
  done
fi

for x in ndsudo freeipmi.plugin ioping cgroup-network local-listeners network-viewer.plugin ebpf.plugin nfacct.plugin xenstat.plugin; do
  f="usr/libexec/netdata/plugins.d/${x}"

  if [ -f "${f}" ]; then
    run chmod 4750 "${f}"
  fi
done

# -----------------------------------------------------------------------------

replace_symlink() {
  target="${1}"
  name="${2}"
  rm -f "${name}"
  ln -s "${target}" "${name}"
}

ensure_ca_certificates_link() {
  local ssl_prefix="/opt/netdata/etc/ssl/"
  local link_path="${ssl_prefix}/certs/ca-certificates.crt"

  # If ca-certificates.crt already exists, we're done
  [ -e "${link_path}" ] && return 0

  local cert_names=(
    "certs/ca-bundle.crt" # RHEL, Fedora, RHEL clones
    "ca-bundle.pem"       # SLE, OpenSUSE
    "cert.pem"            # Alpine
  )

  mkdir -p "$(dirname "${link_path}")"

  for cert_name in "${cert_names[@]}"; do
    local target="${ssl_prefix}/${cert_name}"

    if [ -f "${target}" ] && [ -r "${target}" ]; then
      # Create relative symlink to avoid breaking if Netdata is uninstalled
      if command -v realpath >/dev/null 2>&1; then
        ln -s "$(realpath --relative-to="$(dirname "${link_path}")" "${target}")" "${link_path}"
      else
        ln -s "../${cert_name}" "${link_path}"
      fi
      return 0
    fi
  done

  echo "Warning: No valid certificate bundle found"
  return 1
}

select_system_certs() {
  if [ -d /etc/pki/tls ]; then
    echo "${1} /etc/pki/tls for TLS configuration and certificates"
    replace_symlink /etc/pki/tls /opt/netdata/etc/ssl
  elif [ -d /etc/ssl ]; then
    echo "${1} /etc/ssl for TLS configuration and certificates"
    replace_symlink /etc/ssl /opt/netdata/etc/ssl
  fi

  # Ensure static curl can find the certificates
  ensure_ca_certificates_link
}

select_internal_certs() {
  echo "Using bundled TLS configuration and certificates"
  replace_symlink /opt/netdata/share/ssl /opt/netdata/etc/ssl
}

certs_selected() {
  [ -L /opt/netdata/etc/ssl ] || return 1
}

test_certs() {
  /opt/netdata/bin/curl --fail --max-time 300 --silent --output /dev/null "${NETDATA_CERT_TEST_URL}"

  case "$?" in
    35 | 77)
      echo "Failed to load certificate files for test."
      return 1
      ;;
    60 | 82 | 83)
      echo "Certificates cannot be used to connect to ${NETDATA_CERT_TEST_URL}"
      return 1
      ;;
    53 | 54 | 66)
      echo "Unable to use OpenSSL configuration associated with certificates"
      return 1
      ;;
    0) echo "Successfully connected to ${NETDATA_CERT_TEST_URL} using certificates" ;;
    *) echo "Unable to test certificates due to networking problems, blindly assuming they work" ;;
  esac
}

# If the user has manually set up certificates, don’t mess with it.
if [ ! -L /opt/netdata/etc/ssl ] && [ -d /opt/netdata/etc/ssl ]; then
  echo "Preserving existing user configuration for TLS"
else
  echo "Configure TLS certificate paths (mode: ${NETDATA_CERT_MODE})"
  case "${NETDATA_CERT_MODE}" in
    check)
      select_system_certs "Testing"
      if certs_selected && test_certs; then
        select_system_certs "Using"
      else
        select_internal_certs
      fi
      ;;
    bundled) select_internal_certs ;;
    *)
      select_system_certs "Using"
      if ! certs_selected; then
        select_internal_certs
      fi
      ;;
  esac
fi

# -----------------------------------------------------------------------------

echo "Save install options"
grep -qv 'IS_NETDATA_STATIC_BINARY="yes"' "${NETDATA_PREFIX}/etc/netdata/.environment" || echo IS_NETDATA_STATIC_BINARY=\"yes\" >>"${NETDATA_PREFIX}/etc/netdata/.environment"
REINSTALL_OPTIONS="$(echo "${REINSTALL_OPTIONS}" | awk '{gsub("/", "\\/"); print}')"
sed -i "s/REINSTALL_OPTIONS=\".*\"/REINSTALL_OPTIONS=\"${REINSTALL_OPTIONS}\"/" "${NETDATA_PREFIX}/etc/netdata/.environment"

# -----------------------------------------------------------------------------
if [ ${STARTIT} -eq 0 ]; then
  create_netdata_conf "${NETDATA_PREFIX}/etc/netdata/netdata.conf"
  netdata_banner "is installed now!"
else
  progress "starting netdata"

  if ! restart_netdata "${NETDATA_PREFIX}/bin/netdata"; then
    create_netdata_conf "${NETDATA_PREFIX}/etc/netdata/netdata.conf"
    netdata_banner "is installed and running now!"
  else
    create_netdata_conf "${NETDATA_PREFIX}/etc/netdata/netdata.conf" "http://localhost:19999/netdata.conf"
    netdata_banner "is installed now!"
  fi
fi
run chmod 0644 "${NETDATA_PREFIX}/etc/netdata/netdata.conf"
