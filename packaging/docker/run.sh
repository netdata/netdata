#!/usr/bin/env bash
#
# Entry point script for netdata
#
# Copyright: 2018 and later Netdata Inc.
# SPDX-License-Identifier: GPL-3.0-or-later
#
# Author  : Pavlos Emm. Katsoulakis <paul@netdata.cloud>
# Author  : Austin S. Hemmelgarn <austin@netdata.cloud>
set -e

if [ ! -w / ] && [ "${EUID}" -eq 0 ]; then
  echo >&2 "WARNING: This Docker host appears to not properly support newer stat system calls. This is known to cause issues with Netdata (most notably, nodes running on such hosts **cannot be claimed**)."
  echo >&2 "WARNING: For more information, see https://learn.netdata.cloud/docs/agent/claim#known-issues-on-older-hosts-with-seccomp-enabled"
fi

if [ ! "${DISABLE_TELEMETRY:-0}" -eq 0 ] ||
  [ -n "$DISABLE_TELEMETRY" ] ||
  [ ! "${DO_NOT_TRACK:-0}" -eq 0 ] ||
  [ -n "$DO_NOT_TRACK" ]; then
  touch /etc/netdata/.opt-out-from-anonymous-statistics
fi

chmod o+rX / # Needed to fix permissions issues in some cases.

BALENA_PGID=$(stat -c %g /var/run/balena.sock 2>/dev/null || true)
DOCKER_PGID=$(stat -c %g /var/run/docker.sock 2>/dev/null || true)

re='^[0-9]+$'
if [[ $BALENA_PGID =~ $re ]]; then
  echo "Netdata detected balena-engine.sock"
  DOCKER_HOST='/var/run/balena-engine.sock'
  PGID="$BALENA_PGID"
elif [[ $DOCKER_PGID =~ $re ]]; then
  echo "Netdata detected docker.sock"
  DOCKER_HOST="/var/run/docker.sock"
  PGID="$DOCKER_PGID"
fi
export PGID
export DOCKER_HOST

if [ -n "${PGID}" ]; then
  echo "Creating docker group ${PGID}"
  addgroup -g "${PGID}" "docker" || echo >&2 "Could not add group docker with ID ${PGID}, its already there probably"
  echo "Assign netdata user to docker group ${PGID}"
  usermod -a -G "${PGID}" "${DOCKER_USR}" || echo >&2 "Could not add netdata user to group docker with ID ${PGID}"
fi

if mountpoint -q /etc/netdata && [ -z "$(ls -A /etc/netdata)" ]; then
  echo "Copying stock configuration to /etc/netdata"
  cp -a /etc/netdata.stock/. /etc/netdata
fi

if [ -w "/etc/netdata" ]; then
  if mountpoint -q /etc/netdata; then
    hostname >/etc/netdata/.container-hostname
  else
    rm -f /etc/netdata/.container-hostname
  fi
fi

if [ -n "${NETDATA_CLAIM_URL}" ] && [ -n "${NETDATA_CLAIM_TOKEN}" ] && [ ! -f /var/lib/netdata/cloud.d/claimed_id ]; then
  # shellcheck disable=SC2086
  /usr/sbin/netdata-claim.sh -token="${NETDATA_CLAIM_TOKEN}" \
                             -url="${NETDATA_CLAIM_URL}" \
                             ${NETDATA_CLAIM_ROOMS:+-rooms="${NETDATA_CLAIM_ROOMS}"} \
                             ${NETDATA_CLAIM_PROXY:+-proxy="${NETDATA_CLAIM_PROXY}"} \
                             ${NETDATA_EXTRA_CLAIM_OPTS} \
                             -daemon-not-running
fi

if [ -n "${NETDATA_EXTRA_APK_PACKAGES}" ]; then
  echo "Fetching APK repository metadata."
  if ! apk update; then
    echo "Failed to fetch APK repository metadata."
  else
    echo "Installing supplementary packages."
    # shellcheck disable=SC2086
    if ! apk add --no-cache ${NETDATA_EXTRA_APK_PACKAGES}; then
      echo "Failed to install supplementary packages."
    fi
  fi
fi

exec /usr/sbin/netdata -u "${DOCKER_USR}" -D -s /host -p "${NETDATA_LISTENER_PORT}" "$@"
