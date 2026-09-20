#!/bin/bash
# SPDX-License-Identifier: GPL-3.0-or-later
#
# The suite's API tests must reach the engine the way an embedder that is not the daemon would: through the public
# headers under include/dbengine/ only. Nothing in the build enforces that - libnetdata exports the whole src/ tree
# as a PUBLIC include directory, so a private header would compile from here without complaint - so it is enforced
# here instead. Without this the property is a convention, and a convention rots.
#
# Files named internal_*.cc are exempt: they test the engine's internals and say so in their name.

set -euo pipefail

here=$(cd "$(dirname "$0")" && pwd)

private=$(cd "$here/.." && ls ./*.h | sed 's#^\./##' | tr '\n' '|' | sed 's/|$//')
if [ -z "$private" ]; then
    echo "check-public-includes: found no private dbengine headers to check against" >&2
    exit 1
fi

status=0
checked=0

for file in "$here"/*.cc "$here"/*.h; do
    [ -e "$file" ] || continue

    name=$(basename "$file")
    case "$name" in
        internal_*) continue ;;
    esac

    checked=$((checked + 1))

    # Every #include of a dbengine header must land in include/dbengine/. A bare "cache.h" or a path that walks into
    # the engine's directory is a private header by either spelling.
    while IFS= read -r line; do
        included=$(printf '%s\n' "$line" | sed -E 's/^[[:space:]]*#[[:space:]]*include[[:space:]]*[<"]([^">]+)[">].*/\1/')

        case "$included" in
            */include/dbengine/*) continue ;;
        esac

        base=$(basename "$included")
        if printf '%s\n' "$base" | grep -qE "^($private)$"; then
            echo "$name: includes the private dbengine header '$included'" >&2
            status=1
        fi

        case "$included" in
            *storage-engines/dbengine/*)
                echo "$name: includes '$included' from inside the engine rather than its public headers" >&2
                status=1
                ;;
        esac
    done < <(grep -E '^[[:space:]]*#[[:space:]]*include' "$file" || true)
done

if [ "$checked" -eq 0 ]; then
    echo "check-public-includes: no files checked, which cannot be right" >&2
    exit 1
fi

if [ "$status" -eq 0 ]; then
    echo "check-public-includes: $checked file(s) reach the engine through its public headers only"
fi

exit "$status"
