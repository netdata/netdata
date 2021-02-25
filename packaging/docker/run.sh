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

if [ ! "${DO_NOT_TRACK:-0}" -eq 0 ] || [ -n "$DO_NOT_TRACK" ]; then
  touch /etc/netdata/.opt-out-from-anonymous-statistics
fi

if [ -n "${PGID}" ]; then
  echo "Creating docker group ${PGID}"
  addgroup -g "${PGID}" "docker" || echo >&2 "Could not add group docker with ID ${PGID}, its already there probably"
  echo "Assign netdata user to docker group ${PGID}"
  usermod -a -G "${PGID}" "${DOCKER_USR}" || echo >&2 "Could not add netdata user to group docker with ID ${PGID}"
fi

if [ -f /var/lib/netdata/cloud.d/claimed_id ] || [ -z "${NETDATA_CLAIM_URL}" ] || [ -z "${NETDATA_CLAIM_TOKEN}" ]; then
  exec /usr/sbin/netdata \
    -u "${DOCKER_USR}" \
    -D -s /host \
    -p "${NETDATA_LISTENER_PORT}" \
    -W set web "web files group" root -W set web "web files owner" root \
    "$@"
else
  exec /usr/sbin/netdata \
    -u "${DOCKER_USR}" \
    -D -s /host \
    -p "${NETDATA_LISTENER_PORT}" \
    -W set web "web files group" root -W set web "web files owner" root \
    -W set2 cloud global enabled true \
    -W set2 cloud global "cloud base url" "${NETDATA_CLAIM_URL}" \
    -W "claim -url=${NETDATA_CLAIM_URL} -token=${NETDATA_CLAIM_TOKEN} -rooms=${NETDATA_CLAIM_ROOMS} -proxy=${NETDATA_CLAIM_PROXY}" \
    "$@"
fi
