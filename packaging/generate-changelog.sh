#!/bin/sh

set -e

FIRST_TAG="v1.47.0" # Last release prior to v2.0.0

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

git-cliff --config "$(dirname "${SCRIPT_SOURCE}")/cliff.toml" \
          --output "$(dirname "${SCRIPT_SOURCE}")/../CHANGELOG.md" \
          --verbose \
          "${FIRST_TAG}..HEAD"
