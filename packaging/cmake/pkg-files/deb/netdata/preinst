#!/bin/sh

set -e

case "$1" in
  install)
    if ! getent group netdata > /dev/null; then
      addgroup --quiet --system netdata
    fi

    if ! getent passwd netdata > /dev/null; then
      adduser --quiet --system --ingroup netdata --home /var/lib/netdata --no-create-home netdata
    fi

    for item in docker nginx varnish haproxy adm nsd proxy squid ceph nobody I2C; do
      if getent group $item > /dev/null 2>&1; then
        usermod -a -G $item netdata
      fi
    done
    # Netdata must be able to read /etc/pve/qemu-server/* and /etc/pve/lxc/*
    # for reading VMs/containers names, CPU and memory limits on Proxmox.
    if [ -d "/etc/pve" ] && getent group "www-data" > /dev/null 2>&1; then
      usermod -a -G www-data netdata
    fi
    ;;
esac
