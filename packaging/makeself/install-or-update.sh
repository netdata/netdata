#!/usr/bin/env bash

# SPDX-License-Identifier: GPL-3.0-or-later

# shellcheck source=./packaging/makeself/functions.sh
. "$(dirname "${0}")"/functions.sh

export LC_ALL=C
umask 002

# Be nice on production environments
renice 19 $$ > /dev/null 2> /dev/null

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

# -----------------------------------------------------------------------------
progress "Attempt to create user/group netdata/netadata"

NETDATA_WANTED_GROUPS="docker nginx varnish haproxy adm nsd proxy squid ceph nobody I2C"
NETDATA_ADDED_TO_GROUPS=""
# Default user/group
NETDATA_USER="root"
NETDATA_GROUP="root"

if portable_add_group netdata; then
  if portable_add_user netdata "/opt/netdata"; then
    progress "Add user netdata to required user groups"
    for g in ${NETDATA_WANTED_GROUPS}; do
      # shellcheck disable=SC2086
      if portable_add_user_to_group ${g} netdata; then
        NETDATA_ADDED_TO_GROUPS="${NETDATA_ADDED_TO_GROUPS} ${g}"
      else
        run_failed "Failed to add netdata user to secondary groups"
      fi
    done
    # Netdata must be able to read /etc/pve/qemu-server/* and /etc/pve/lxc/*
    # for reading VMs/containers names, CPU and memory limits on Proxmox.
    if [ -d "/etc/pve" ]; then
      portable_add_user_to_group "www-data" netdata && NETDATA_ADDED_TO_GROUPS="${NETDATA_ADDED_TO_GROUPS} www-data"
    fi
    NETDATA_USER="netdata"
    NETDATA_GROUP="netdata"
  else
    run_failed "I could not add user netdata, will be using root"
  fi
else
  run_failed "I could not add group netdata, so no user netdata will be created as well. Netdata run as root:root"
fi

# -----------------------------------------------------------------------------
progress "Install logrotate configuration for netdata"

install_netdata_logrotate || run_failed "Cannot install logrotate file for netdata."

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
run chown -R ${NETDATA_USER}:${NETDATA_GROUP} /opt/netdata/var

if [ -d /opt/netdata/usr/libexec/netdata/plugins.d/ebpf.d ]; then
  run chown -R root:${NETDATA_GROUP} /opt/netdata/usr/libexec/netdata/plugins.d/ebpf.d
fi

# -----------------------------------------------------------------------------

progress "changing plugins ownership and permissions"

for x in ndsudo apps.plugin perf.plugin slabinfo.plugin debugfs.plugin freeipmi.plugin ioping cgroup-network local-listeners ebpf.plugin nfacct.plugin xenstat.plugin python.d.plugin charts.d.plugin go.d.plugin ioping.plugin cgroup-network-helper.sh; do
  f="usr/libexec/netdata/plugins.d/${x}"
  if [ -f "${f}" ]; then
    run chown root:${NETDATA_GROUP} "${f}"
  fi
done

if command -v setcap >/dev/null 2>&1; then
    run setcap "cap_dac_read_search,cap_sys_ptrace=ep" "usr/libexec/netdata/plugins.d/apps.plugin"
    run setcap "cap_dac_read_search=ep" "usr/libexec/netdata/plugins.d/slabinfo.plugin"
    run setcap "cap_dac_read_search=ep" "usr/libexec/netdata/plugins.d/debugfs.plugin"

    if command -v capsh >/dev/null 2>&1 && capsh --supports=cap_perfmon 2>/dev/null ; then
        run setcap "cap_perfmon=ep" "usr/libexec/netdata/plugins.d/perf.plugin"
    else
        run setcap "cap_sys_admin=ep" "usr/libexec/netdata/plugins.d/perf.plugin"
    fi

    run setcap "cap_dac_read_search+epi cap_net_admin+epi cap_net_raw=eip" "usr/libexec/netdata/plugins.d/go.d.plugin"
else
  for x in ndsudo apps.plugin perf.plugin slabinfo.plugin debugfs.plugin; do
    f="usr/libexec/netdata/plugins.d/${x}"
    run chmod 4750 "${f}"
  done
fi

for x in freeipmi.plugin ioping cgroup-network local-listeners ebpf.plugin nfacct.plugin xenstat.plugin; do
  f="usr/libexec/netdata/plugins.d/${x}"

  if [ -f "${f}" ]; then
    run chmod 4750 "${f}"
  fi
done

# -----------------------------------------------------------------------------

echo "Configure TLS certificate paths"
if [ ! -L /opt/netdata/etc/ssl ] && [ -d /opt/netdata/etc/ssl ] ; then
  echo "Preserving existing user configuration for TLS"
else
  if [ -d /etc/pki/tls ] ; then
    echo "Using /etc/pki/tls for TLS configuration and certificates"
    ln -sf /etc/pki/tls /opt/netdata/etc/ssl
  elif [ -d /etc/ssl ] ; then
    echo "Using /etc/ssl for TLS configuration and certificates"
    ln -sf /etc/ssl /opt/netdata/etc/ssl
  else
    echo "Using bundled TLS configuration and certificates"
    ln -sf /opt/netdata/share/ssl /opt/netdata/etc/ssl
  fi
fi

# -----------------------------------------------------------------------------

echo "Save install options"
grep -qv 'IS_NETDATA_STATIC_BINARY="yes"' "${NETDATA_PREFIX}/etc/netdata/.environment" || echo IS_NETDATA_STATIC_BINARY=\"yes\" >> "${NETDATA_PREFIX}/etc/netdata/.environment"
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
