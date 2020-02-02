#!/bin/sh

apk --no-cache -U add alpine-sdk bash curl libuv-dev zlib-dev util-linux-dev libmnl-dev gcc make git autoconf automake pkgconfig python logrotate

./netdata-installer.sh -u --dont-wait --disable-go --dont-start-it

(
  /etc/periodic/daily/netdata-updater
) || (
  echo "ERROR: Test failed"
  exec /bin/sh
)
