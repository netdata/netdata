#!/bin/sh

# This script installs supporting packages needed for CI, which provide following:
# cron, pidof

set -e

if [ -f /etc/centos-release ] || [ -f /etc/redhat-release ] || [ -f /etc/fedora-release ] || [ -f /etc/almalinux-release ]; then
    # Alma, Fedora, CentOS, Redhat
    dnf install -y procps-ng cronie cronie-anacron || yum install -y procps-ng cronie cronie-anacron
elif [ -f /etc/arch-release ]; then
    # Arch
    pacman -S --noconfirm cronie
fi
