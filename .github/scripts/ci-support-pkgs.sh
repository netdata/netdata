#!/bin/sh

# This script installs supporting packages needed for CI, which provide following:
# curl, cron, pidof

set -e

. /etc/os-release

case "${ID}" in
    amzn|almalinux|centos|fedora)
        dnf install -y procps-ng cronie cronie-anacron curl || \
        yum install -y procps-ng cronie cronie-anacron curl
        ;;
    arch)
        pacman -S --noconfirm cronie curl
        ;;
    debian|ubuntu)
        apt-get update && apt-get install -y cronie anacron curl
        ;;
esac
