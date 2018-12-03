#!/bin/bash

set -e

if [ ! -f .gitignore ]
then
  echo "Run as ./tests/$(basename "$0") from top level directory of git repository"
  exit 1
fi

if ! git config user.name; then
  git config user.email "test@example.com"
  git config user.name "test"
fi

echo "========= INSTALL ========="
./netdata-installer.sh  --dont-wait --dont-start-it --install /tmp &>/dev/null

echo "========= ADD GARBAGE ========="
touch test
git add test
git commit -m 'test commit'
touch new_file
git status

echo "========= UPDATE ========="
./netdata-updater.sh

#echo "========= UNINSTALL ========="
#./netdata-uninstaller.sh
