#!/bin/sh

while true; do
  /usr/sbin/netdata -D
  printf >&2 " > Netdata process terminated with %d , restarting ..." "$?"
  sleep 3
done
