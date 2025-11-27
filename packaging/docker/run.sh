#!/usr/bin/env bash
# SPDX-License-Identifier: GPL-3.0-or-later
#
# Entry point script for netdata

set -e

if [ ! -w / ] && [ "${EUID}" -eq 0 ]; then
  echo >&2 "WARNING: This Docker host appears to not properly support newer stat system calls. This is known to cause issues with Netdata (most notably, nodes running on such hosts **cannot be claimed**)."
  echo >&2 "WARNING: For more information, see https://learn.netdata.cloud/docs/agent/claim#known-issues-on-older-hosts-with-seccomp-enabled"
fi

# Needed to read Proxmox VMs and (LXC) containers configuration files (name resolution + CPU and memory limits)
function add_netdata_to_proxmox_conf_files_group() {
  [ "${DOCKER_USR}" = "root" ] && return

  local group_guid
  group_guid="$(stat -c %g /host/etc/pve 2>/dev/null || true)"
  [ -z "${group_guid}" ] && return

  if ! getent group "${group_guid}" >/dev/null; then
    echo "Creating proxmox-etc-pve group with GID ${group_guid}"
    if ! addgroup --gid "${group_guid}" "proxmox-etc-pve"; then
      echo >&2 "Failed to add group proxmox-etc-pve with GID ${group_guid}."
      return
    fi
  fi

  if ! getent group "${group_guid}" | grep -q "${DOCKER_USR}"; then
    echo "Assigning ${DOCKER_USR} user to group ${group_guid}"
    if ! usermod --apend --groups "${group_guid}" "${DOCKER_USR}"; then
      echo >&2 "Failed to add ${DOCKER_USR} user to group with GID ${group_guid}."
      return
    fi
  fi
}

# Needed to access NVIDIA GPU monitoring
function add_netdata_to_nvidia_group() {
  [ "${DOCKER_USR}" = "root" ] && return

  local group_gid
  group_gid="$(stat -c %g /dev/nvidiactl 2>/dev/null || true)"
  [ -z "${group_gid}" ] && return

  # Skip if the device is owned by root group
  [ "${group_gid}" -eq 0 ] && return

  if ! getent group "${group_gid}" >/dev/null; then
    echo "Creating nvidia-dev group with GID ${group_gid}"
    if ! addgroup --gid "${group_gid}" "nvidia-dev"; then
      echo >&2 "Failed to add group nvidia-dev with GID ${group_gid}."
      return
    fi
  fi

  if ! getent group "${group_gid}" | grep -q "${DOCKER_USR}"; then
    echo "Assigning ${DOCKER_USR} user to group ${group_gid}"
    if ! usermod --append --groups "${group_gid}" "${DOCKER_USR}"; then
      echo >&2 "Failed to add ${DOCKER_USR} user to group with GID ${group_gid}."
      return
    fi
  fi
}

if [ ! "${DISABLE_TELEMETRY:-0}" -eq 0 ] ||
  [ -n "$DISABLE_TELEMETRY" ] ||
  [ ! "${DO_NOT_TRACK:-0}" -eq 0 ] ||
  [ -n "$DO_NOT_TRACK" ]; then
  touch /etc/netdata/.opt-out-from-anonymous-statistics
fi

chmod o+rX / 2>/dev/null || echo "Unable to change permissions without errors."

if [ "${EUID}" -eq 0 ]; then
  if [ -n "${NETDATA_EXTRA_APK_PACKAGES}" ]; then
    echo >&2 "WARNING: Netdata’s Docker images have switched from Alpine to Debian as a base platform. Supplementary package support is now handled through the NETDATA_EXTRA_DEB_PACKAGES variable instead of NETDATA_EXTRA_APK_PACKAGES."
    echo >&2 "WARNING: The container will still run, but supplementary packages listed in NETDATA_EXTRA_APK_PACKAGES will not be installed."
    echo >&2 "WARNING: To remove these messages, either undefine NETDATA_EXTRA_APK_PACKAGES, or define it to an empty string."
  fi

  if [ -n "${NETDATA_EXTRA_DEB_PACKAGES}" ]; then
    echo "Fetching APT repository metadata."
    if ! apt-get update; then
      echo "Failed to fetch APT repository metadata."
    else
      echo "Installing supplementary packages."
      export DEBIAN_FRONTEND="noninteractive"
      # shellcheck disable=SC2086
      if ! apt-get install -y --no-install-recommends ${NETDATA_EXTRA_DEB_PACKAGES}; then
        echo "Failed to install supplementary packages."
      fi
    fi
  fi

  BALENA_PGID=$(stat -c %g /var/run/balena.sock 2>/dev/null || true)
  DOCKER_PGID=$(stat -c %g /var/run/docker.sock 2>/dev/null || true)

  re='^[0-9]+$'
  if [[ $BALENA_PGID =~ $re ]]; then
    echo "Netdata detected balena-engine.sock"
    DOCKER_HOST='unix:///var/run/balena-engine.sock'
    PGID="$BALENA_PGID"
  elif [[ $DOCKER_PGID =~ $re ]]; then
    echo "Netdata detected docker.sock"
    DOCKER_HOST="unix:///var/run/docker.sock"
    PGID="$DOCKER_PGID"
  fi
  export PGID
  export DOCKER_HOST

  if [ -n "${PGID}" ]; then
    echo "Creating docker group with GID ${PGID}"
    addgroup --gid "${PGID}" "docker" || echo >&2 "Failed to add group docker with GID ${PGID}, probably one already exists."
    echo "Assigning ${DOCKER_USR} user to group with GID ${PGID}"
    usermod --append --groups "${PGID}" "${DOCKER_USR}" || echo >&2 "Failed to add ${DOCKER_USR} user to group with GID ${PGID}."
  fi

  if [ -d "/host/etc/pve" ]; then
    add_netdata_to_proxmox_conf_files_group || true
  fi
  if [ -e "/dev/nvidiactl" ]; then
    add_netdata_to_nvidia_group || true
  fi
else
  echo >&2 "WARNING: Entrypoint started as non-root user. This is not officially supported and some features may not be available."
fi

if mountpoint -q /etc/netdata; then
  echo "Copying stock configuration to /etc/netdata"
  cp -an /etc/netdata.stock/* /etc/netdata
  cp -an /etc/netdata.stock/.[^.]* /etc/netdata
fi

if [ -w "/etc/netdata" ]; then
  if mountpoint -q /etc/netdata; then
    hostname >/etc/netdata/.container-hostname
  else
    rm -f /etc/netdata/.container-hostname
  fi
fi

exec /usr/sbin/netdata -u "${DOCKER_USR}" -D -s /host -p "${NETDATA_LISTENER_PORT}" "$@"
