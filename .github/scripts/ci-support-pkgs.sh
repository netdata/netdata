#!/bin/sh

# This script installs supporting packages needed for CI, which provide following:
# curl, cron, pidof, gpg

set -e

. /etc/os-release

case "${ID}" in
    amzn|almalinux|centos|fedora)
        dnf install -y procps-ng cronie cronie-anacron curl gnupg2 || \
        yum install -y procps-ng cronie cronie-anacron curl gnupg2
        ;;
    arch)
        pacman -S --noconfirm cronie curl gnupg
        ;;
    debian|ubuntu)
        apt-get update && apt-get install -y cronie anacron curl gnupg
        ;;
esac
