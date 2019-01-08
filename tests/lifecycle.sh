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
# Copy uninstaller as upgrader will overwrite it with a version from master branch
cp netdata-uninstaller.sh /tmp/netdata-uninstaller.sh

echo "========= ADD GARBAGE ========="
touch garbagefile
git add garbagefile
git commit -m 'test commit'
touch new_file
git status

echo "========= UPDATE ========="
/etc/periodic/daily/netdata-updater

echo "========= UNINSTALL ========="
mv /tmp/netdata-uninstaller.sh ./netdata-uninstaller.sh
./netdata-uninstaller.sh --yes --force
