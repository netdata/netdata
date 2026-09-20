#!/bin/bash
# SPDX-License-Identifier: GPL-3.0-or-later
#
# The suite's API tests must reach the engine the way an embedder that is not the daemon would: through the public
# headers under include/dbengine/ only. Nothing in the build enforces that - libnetdata exports the whole src/ tree
# as a PUBLIC include directory, so a private header, or a daemon header, would compile from here without complaint
# - so it is enforced here instead.
#
# This is an allowlist, not a search for known-bad names. An earlier version listed the private headers and rejected
# those; it was defeated four different ways, because anything it had not thought of passed. Here a quoted include
# must be one of a small set of permitted forms and everything else is a failure, including an include this script
# cannot parse.
#
# Permitted:
#   #include <anything>                                              system, C++ standard library, googletest
#   #include "support.h"                                             this suite's own shared header
#   #include "database/storage-engines/dbengine/include/dbengine/X"  the engine's public headers
#
# Files named internal_* are exempt: they test the engine's internals and say so in their name. They cannot be used
# to smuggle a private header into an API test, because an API test including one would itself be rejected.

set -euo pipefail

here=$(cd "$(dirname "$0")" && pwd)

readonly PUBLIC_PREFIX="database/storage-engines/dbengine/include/dbengine/"

status=0
checked=0

for file in "$here"/*.cc "$here"/*.cpp "$here"/*.cxx "$here"/*.h "$here"/*.hh "$here"/*.hpp; do
    [ -e "$file" ] || continue

    name=$(basename "$file")
    case "$name" in
        internal_*) continue ;;
    esac

    checked=$((checked + 1))

    while IFS= read -r line; do
        # A system include is always fine and needs no further thought.
        if printf '%s\n' "$line" | grep -qE '^[[:space:]]*#[[:space:]]*include[[:space:]]*<[^>]+>'; then
            continue
        fi

        # Anything else must be a quoted include this script can read. One it cannot read - a macro, say - is a
        # failure rather than something to skip, because skipping is how the previous version was defeated.
        if ! printf '%s\n' "$line" | grep -qE '^[[:space:]]*#[[:space:]]*include[[:space:]]*"[^"]+"'; then
            echo "$name: cannot read this include, so it cannot be allowed: $(printf '%s' "$line" | tr -s '[:space:]' ' ')" >&2
            status=1
            continue
        fi

        included=$(printf '%s\n' "$line" | sed -E 's/^[[:space:]]*#[[:space:]]*include[[:space:]]*"([^"]+)".*/\1/')

        # A path that walks upwards can land anywhere, whatever it appears to start with.
        case "$included" in
            *..*)
                echo "$name: includes '$included', which walks out of the public headers" >&2
                status=1
                continue
                ;;
        esac

        if [ "$included" = "support.h" ]; then
            continue
        fi

        case "$included" in
            "${PUBLIC_PREFIX}"*) continue ;;
        esac

        echo "$name: includes '$included'; API tests may include only <system headers>, \"support.h\"," >&2
        echo "    and \"${PUBLIC_PREFIX}...\"" >&2
        status=1
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
