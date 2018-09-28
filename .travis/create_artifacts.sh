#!/bin/bash
# shellcheck disable=SC2230

if [ ! -f .gitignore ]
then
  echo "Run as ./travis/$(basename "$0") from top level directory of git repository"
  exit 1
fi

eval "$(ssh-agent -s)"
./.travis/decrypt-if-have-key decb6f6387c4
export KEYSERVER=ipv4.pool.sks-keyservers.net
./packaging/gpg-recv-key phil@firehol.org "0762 9FF7 89EA 6156 012F  9F50 C406 9602 1359 9237"
./packaging/gpg-recv-key costa@tsaousis.gr "4DFF 624A E564 3B51 2872  1F40 29CA 3358 89B9 A863"
# Run the commit hooks in case the developer didn't
git diff 4b825dc642cb6eb9a060e54bf8d69288fbee4904 | ./packaging/check-files -
fakeroot ./packaging/git-build
# Make sure stdout is in blocking mode. If we don't, then conda create will barf during downloads.
# See https://github.com/travis-ci/travis-ci/issues/4704#issuecomment-348435959 for details.
python -c 'import os,sys,fcntl; flags = fcntl.fcntl(sys.stdout, fcntl.F_GETFL); fcntl.fcntl(sys.stdout, fcntl.F_SETFL, flags&~os.O_NONBLOCK);'
# make self-extractor
./makeself/build-x86_64-static.sh
for i in *.tar.gz; do sha512sum -b "$i" > "$i.sha"; done
for i in *.gz.run; do sha512sum -b "$i" > "$i.sha"; done
./.travis/deploy-if-have-key
