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
git config user.email "test@example.com"
git config user.name "test"

echo "========= INSTALL ========="
./netdata-installer.sh  --dont-wait --dont-start-it --auto-update --install /tmp &>/dev/null
ls /tmp

echo "========= UPDATE ========="
ENVIRONMENT_FILE=/tmp/netdata/etc/.environment /etc/cron.daily/netdata-updater

#echo "========= UNINSTALL ========="
#mv /tmp/netdata-uninstaller.sh ./netdata-uninstaller.sh
#./netdata-uninstaller.sh --yes --force
