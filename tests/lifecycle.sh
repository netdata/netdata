#!/bin/bash

set -e

if [ ! -f .gitignore ]
then
  echo "Run as ./tests/$(basename "$0") from top level directory of git repository"
  exit 1
fi

WORKSPACE=$(mktemp -d)
cp -r ./ "${WORKSPACE}/"
cd "${WORKSPACE}"

echo "========= INSTALL ========="
./netdata-installer.sh  --dont-wait --dont-start-it --auto-update --install /tmp &>/dev/null
cp netdata-uninstaller.sh /tmp/netdata-uninstaller.sh
ls /tmp

rm -rf "${WORKSPACE}"
echo "========= UPDATE ========="
ENVIRONMENT_FILE=/tmp/netdata/etc/netdata/.environment /etc/cron.daily/netdata-updater

#TODO(paulfantom): Enable with #5031
#echo "========= UNINSTALL ========="
#ENVIRONMENT_FILE=/tmp/netdata/etc/netdata/.environment /tmp/netdata-uninstaller.sh --yes --force
#[ -f /tmp/netdata/usr/sbin/netdata ] && exit 1
