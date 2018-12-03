#!/bin/bash

set -e

if [ ! -f .gitignore ]
then
  echo "Run as ./travis/$(basename "$0") from top level directory of git repository"
  exit 1
fi

git config user.email "test@example.com"
git config user.name "test"

docker run -it -v "${PWD}:/code:rw" -w /code "netdata/os-test:$1" ./tests/lifecycle.sh
