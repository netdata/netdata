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

if [ ! "${DISABLE_TELEMETRY:-0}" -eq 0 ] ||
  [ -n "$DISABLE_TELEMETRY" ] ||
  [ ! "${DO_NOT_TRACK:-0}" -eq 0 ] ||
  [ -n "$DO_NOT_TRACK" ]; then
  touch /etc/netdata/.opt-out-from-anonymous-statistics
fi


BALENA_PGID=$(ls -nd /var/run/balena.sock | awk '{print $4}')
DOCKER_PGID=$(ls -nd /var/run/docker.sock | awk '{print $4}')

re='^[0-9]+$'
if [[ $BALENA_PGID =~ $re ]]; then
  echo "Netdata detected balena-engine.sock"
  DOCKER_HOST='/var/run/balena-engine.sock'
  PGID="$BALENA_PGID"
elif [[ $DOCKER_PGID =~ $re ]]; then
  echo "Netdata detected docker.sock"
  DOCKER_HOST="/var/run/docker.sock"
  PGID=$(ls -nd /var/run/docker.sock | awk '{print $4}')
fi
export PGID
export DOCKER_HOST

if [ -n "${PGID}" ]; then
  echo "Creating docker group ${PGID}"
  addgroup -g "${PGID}" "docker" || echo >&2 "Could not add group docker with ID ${PGID}, its already there probably"
  echo "Assign netdata user to docker group ${PGID}"
  usermod -a -G "${PGID}" "${DOCKER_USR}" || echo >&2 "Could not add netdata user to group docker with ID ${PGID}"
fi

if mountpoint -q /etc/netdata; then
  if [ -n "${NETDATA_COPY_STOCK_CONIG}" ] || [ "$(find /etc/netdata -type f | wc -l)" -eq 0 ]; then
    echo "Copying stock configuration to /etc/netdata"
    cp -a /etc/netdata.stock/. /etc/netdata
  fi
fi

if [ -n "${NETDATA_CLAIM_URL}" ] && [ -n "${NETDATA_CLAIM_TOKEN}" ] && [ ! -f /var/lib/netdata/cloud.d/claimed_id ]; then
  /usr/sbin/netdata-claim.sh -token="${NETDATA_CLAIM_TOKEN}" \
                             -url="${NETDATA_CLAIM_URL}" \
                             ${NETDATA_CLAIM_ROOMS:+-rooms="${NETDATA_CLAIM_ROOMS}"} \
                             ${NETDATA_CLAIM_PROXY:+-proxy="${NETDATA_CLAIM_PROXY}"} \
                             -daemon-not-running
fi

exec /usr/sbin/netdata -u "${DOCKER_USR}" -D -s /host -p "${NETDATA_LISTENER_PORT}" -W set web "web files group" root -W set web "web files owner" root "$@"
