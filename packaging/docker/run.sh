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

create_group=
remove_group=
create_user=
remove_user=
user_in_group=

if [ -n "${DOCKER_USR}" ]; then
  NETDATA_USER="${DOCKER_USR}"
fi

if [ -w /etc/passwd ] && [ -w /etc/group ] && [ -w /etc/shadow ] && [ -w /etc/gshadow ] ; then
  if getent group netdata > /dev/null; then
  existing_gid="$(getent group netdata | cut -d ':' -f 3)"

  if [ "${existing_gid}" != "${NETDATA_GID}" ]; then
    echo "Netdata group ID mismatch (expected ${NETDATA_GID} but found ${existing_gid}), the existing group will be replaced."
    remove_group=1
    create_group=1
  fi
  else
  echo "Netdata group not found, preparing to create one with GID=${NETDATA_GID}."
  create_group=1
  fi

  if [ -n "${remove_group}" ]; then
  delgroup netdata netdata
  delgroup netdata || exit 1
  fi

  if [ -n "${create_group}" ]; then
  addgroup -g "${NETDATA_GID}" -S netdata || exit 1
  fi

  if [ "${NETDATA_USER}" = "netdata" ]; then
    if getent passwd netdata > /dev/null; then
      existing_user="$(getent passwd netdata)"
      existing_uid="$(echo "${existing_user}" | cut -d ':' -f 3)"
      existing_primary_gid="$(echo "${existing_user}" | cut -d ':' -f 4)"

      if [ "${existing_gid}" != "${NETDATA_UID}" ]; then
        echo "Netdata user ID mismatch (expected ${NETDATA_UID} but found ${existing_uid}), the existing user will be replaced."
        remove_user=1
        create_user=1
      fi

      if [ "${existing_primary_gid}" = "${NETDATA_GID}" ]; then
        user_in_group=1
      else
        echo "Netdata user is not in the correct primary group (expected ${NETDATA_GID} but found ${existing_primary_gid}), the user will be updated."
      fi
    else
      echo "Netdata user not found, preparing to create one with UID=${NETDATA_UID}."
      create_user=1
    fi

    if [ -n "${remove_user}" ]; then
      userdel netdata || exit 1
    fi

    if [ -n "${create_user}" ]; then
      adduser -S -H -s /usr/sbin/nologin -u "${NETDATA_UID}" -h /etc/netdata -G netdata netdata
    elif [ -z "${user_in_group}" ]; then
      usermod -a -G netdata netdata
    fi
  fi
else
  echo "Account databases are not writable, assuming you know what you’re doing and continuing."
fi

chown -R "${NETDATA_USER}:root" /usr/lib/netdata /var/cache/netdata /var/lib/netdata /var/log/netdata
chown -R "${NETDATA_USER}:netdata" /var/lib/netdata/cloud.d

if [ -n "${PGID}" ]; then
  echo "Creating docker group ${PGID}"
  addgroup -g "${PGID}" "docker" || echo >&2 "Could not add group docker with ID ${PGID}, its already there probably"
  echo "Assign netdata user to docker group ${PGID}"
  usermod -a -G "${PGID}" "${NETDATA_USER}" || echo >&2 "Could not add netdata user to group docker with ID ${PGID}"
fi

if [ -n "${NETDATA_CLAIM_URL}" ] && [ -n "${NETDATA_CLAIM_TOKEN}" ] && [ ! -f /var/lib/netdata/cloud.d/claimed_id ]; then
  /usr/sbin/netdata-claim.sh -token="${NETDATA_CLAIM_TOKEN}" \
                             -url="${NETDATA_CLAIM_URL}" \
                             ${NETDATA_CLAIM_ROOMS:+-rooms="${NETDATA_CLAIM_ROOMS}"} \
                             ${NETDATA_CLAIM_PROXY:+-proxy="${NETDATA_CLAIM_PROXY}"} \
                             -daemon-not-running
fi

exec /usr/sbin/netdata -u "${NETDATA_USER}" -D -s /host -p "${NETDATA_LISTENER_PORT}" -W set web "web files group" root -W set web "web files owner" root "$@"
