#!/bin/sh

# This script installs supporting packages needed for CI, which provide following:
# cron, pidof

set -e

. /etc/os-release

case "${ID}" in
    amzn|almalinux|centos|fedora)
        dnf install -y procps-ng cronie cronie-anacron || yum install -y procps-ng cronie cronie-anacron
        ;;
    arch)
        pacman -S --noconfirm cronie
        ;;
esac
