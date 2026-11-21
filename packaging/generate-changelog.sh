#!/bin/sh

set -e

SCRIPT_SOURCE="$(
    self=${0}
    while [ -L "${self}" ]
    do
        cd "${self%/*}" || exit 1
        self=$(readlink "${self}")
    done
    cd "${self%/*}" || exit 1
    echo "$(pwd -P)/${self##*/}"
)"

cd "$(dirname "${SCRIPT_SOURCE}")/.." || exit 1

exec python3 "$(dirname "${SCRIPT_SOURCE}")/generate-changelog.py" "$@"
