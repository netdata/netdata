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
RELEASE_CHANNEL="nightly" # check .travis/create_artifacts.sh before modifying

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

deleted_stock_configs=0
if [ ! -f "etc/netdata/.installer-cleanup-of-stock-configs-done" ]; then

  # -----------------------------------------------------------------------------
  progress "Deleting stock configuration files from user configuration directory"

  declare -A configs_signatures=()
  source "system/configs.signatures"

  if [ ! -d etc/netdata ]; then
    run mkdir -p etc/netdata
  fi

  md5sum="$(command -v md5sum 2> /dev/null || command -v md5 2> /dev/null)"
  while IFS= read -r -d '' x; do
    # find it relative filename
    f="${x/etc\/netdata\//}"

    # find the stock filename
    t="${f/.conf.old/.conf}"
    t="${t/.conf.orig/.conf}"

    if [ -n "${md5sum}" ]; then
      # find the checksum of the existing file
      md5="$(${md5sum} < "${x}" | cut -d ' ' -f 1)"
      #echo >&2 "md5: ${md5}"

      # check if it matches
      if [ "${configs_signatures[${md5}]}" = "${t}" ]; then
        # it matches the default
        run rm -f "${x}"
        deleted_stock_configs=$((deleted_stock_configs + 1))
      fi
    fi
  done < <(find etc -type f)

  touch "etc/netdata/.installer-cleanup-of-stock-configs-done"
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
  cd "${old}"
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

if [ ${deleted_stock_configs} -gt 0 ]; then
  dir_should_be_link etc/netdata ../../usr/lib/netdata/conf.d "000.-.USE.THE.orig.LINK.TO.COPY.AND.EDIT.STOCK.CONFIG.FILES"
fi

# -----------------------------------------------------------------------------
progress "fix permissions"

run chmod g+rx,o+rx /opt
run chown -R ${NETDATA_USER}:${NETDATA_GROUP} /opt/netdata

# -----------------------------------------------------------------------------

progress "changing plugins ownership and setting setuid"

for x in apps.plugin freeipmi.plugin ioping cgroup-network ebpf.plugin perf.plugin slabinfo.plugin nfacct.plugin xenstat.plugin; do
  f="usr/libexec/netdata/plugins.d/${x}"

  if [ -f "${f}" ]; then
    run chown root:${NETDATA_GROUP} "${f}"
    run chmod 4750 "${f}"
  fi
done

if [ -f "usr/libexec/netdata/plugins.d/go.d.plugin" ] && command -v setcap 1>/dev/null 2>&1; then
  run setcap "cap_net_admin+epi cap_net_raw=eip" "usr/libexec/netdata/plugins.d/go.d.plugin"
fi

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
