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
#   #include <anything>                                              system, C++ standard library, googletest -
#                                                                    except anything that resolves inside this
#                                                                    repository's src tree, see below
#   #include "support.h"                                             this suite's own shared header
#   #include "database/storage-engines/dbengine/include/dbengine/X"  the engine's public headers
#
# Angle brackets are not a safe-by-construction category here. libnetdata exports the whole src tree as a PUBLIC
# include directory, which is how the quoted public-header include in support.h resolves at all - and the compiler
# searches the same directories for <...>. So `#include <database/storage-engines/dbengine/rrdengine.h>` compiles
# exactly as the quoted form would. An angle include whose first path component names a directory under src/ is
# therefore treated as a repository include and refused; everything else is system.
#
# Files named internal_* are exempt: they test the engine's internals and say so in their name. They cannot be used
# to smuggle a private header into an API test, because an API test including one would itself be rejected.

set -euo pipefail

here=$(cd "$(dirname "$0")" && pwd)

readonly PUBLIC_PREFIX="database/storage-engines/dbengine/include/dbengine/"

# tests -> dbengine -> storage-engines -> database -> src
src_dir=$(cd "$here/../../../.." && pwd)
if [ ! -d "$src_dir/libnetdata" ]; then
    echo "check-public-includes: '$src_dir' does not look like the source tree; refusing to guess" >&2
    exit 1
fi

status=0
checked=0

# Recursive on purpose: a glob of this directory alone would silently skip anything added in a subdirectory, and
# a check that quietly covers less than it appears to is the failure this script exists to avoid.
while IFS= read -r file; do
    name=$(basename "$file")
    case "$name" in
        internal_*) continue ;;
    esac

    checked=$((checked + 1))

    while IFS= read -r line; do
        # An angle include is fine unless it names a directory of this repository's source tree, in which case it
        # reaches exactly what the quoted forms are checked for.
        if printf '%s\n' "$line" | grep -qE '^[[:space:]]*#[[:space:]]*include[[:space:]]*<[^>]+>'; then
            angled=$(printf '%s\n' "$line" | sed -E 's/^[[:space:]]*#[[:space:]]*include[[:space:]]*<([^>]+)>.*/\1/')
            first=${angled%%/*}

            if [ "$first" != "$angled" ] && [ -d "$src_dir/$first" ]; then
                echo "$name: includes <$angled>, which resolves inside the source tree rather than the system" >&2
                status=1
            fi

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
done < <(find "$here" -type f \( -name '*.cc' -o -name '*.cpp' -o -name '*.cxx' \
                                -o -name '*.h' -o -name '*.hh' -o -name '*.hpp' \) | sort)

if [ "$checked" -eq 0 ]; then
    echo "check-public-includes: no files checked, which cannot be right" >&2
    exit 1
fi

if [ "$status" -eq 0 ]; then
    echo "check-public-includes: $checked file(s) reach the engine through its public headers only"
fi

exit "$status"
