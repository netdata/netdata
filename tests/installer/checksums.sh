#!/bin/bash

set -e

if [ ! -f .gitignore ]; then
	echo "Run as ./tests/installer/$(basename "$0") from top level directory of git repository"
	exit 1
fi

for file in kickstart.sh kickstart-static64.sh; do
	OLD_CHECKSUM=$(grep "$file" packaging/installer/README.md | grep md5sum | cut -d '"' -f2)
	NEW_CHECKSUM="$(md5sum "packaging/installer/$file" | cut -d' ' -f1)"
	if [ "$OLD_CHECKSUM" != "$NEW_CHECKSUM" ]; then
		echo "Invalid checksum for $file in docs."
		echo "checksum in docs: $OLD_CHECKSUM"
		echo "current checksum: $NEW_CHECKSUM"
		exit 1
	fi
done
