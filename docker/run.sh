#!/bin/sh

#set -e

if [ ${PGID+x} ]; then
  echo "Adding user netdata to group with id ${PGID}"
  addgroup -g "${PGID}" -S hostgroup 2>/dev/null
  sed -i "s/${PGID}:$/${PGID}:netdata/g" /etc/group
fi

exec /usr/sbin/netdata -u netdata -D -s /host -p "${NETDATA_PORT}" "$@"
