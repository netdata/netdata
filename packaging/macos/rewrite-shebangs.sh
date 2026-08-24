#!/bin/sh
# SPDX-License-Identifier: GPL-3.0-or-later
#
# Rewrite bash shebangs in a staged install tree to a bundled interpreter.
#
# Usage: rewrite-shebangs.sh <tree> <interpreter>
#
#   <tree>         root of the staged install (DESTDIR + prefix)
#   <interpreter>  absolute runtime path of the bundled bash,
#                  e.g. /opt/netdata/bin/bash
#
# Only bash shebangs are rewritten: macOS ships Bash 3.2 while several
# shipped scripts need Bash 4+, so every script asking for bash gets the
# bundled one. /bin/sh shebangs are left alone - the OS sh is fine.

set -eu

TREE="${1:?usage: rewrite-shebangs.sh <tree> <interpreter>}"
INTERP="${2:?usage: rewrite-shebangs.sh <tree> <interpreter>}"

[ -d "${TREE}" ] || { echo "rewrite-shebangs: no such tree: ${TREE}" >&2; exit 1; }

# The whole pipeline tail runs in one subshell so the counter survives the
# read loop; iterating with read keeps paths with spaces intact.
find "${TREE}" -type f | LC_ALL=C sort | {
    rewritten=0
    while IFS= read -r f; do
        line="$(head -n 1 "${f}" 2>/dev/null || true)"
        case "${line}" in
            '#!/usr/bin/env bash'|'#!/usr/bin/env bash '*|'#!/bin/bash'|'#!/bin/bash '*)
                # Keep any arguments after the interpreter.
                args="$(printf '%s' "${line}" | sed -e 's|^#!/usr/bin/env bash||' -e 's|^#!/bin/bash||')"
                tmp="${f}.shebang-rewrite"
                # cp -p carries the permission bits over portably; the
                # redirection then replaces only the content.
                cp -p "${f}" "${tmp}"
                {
                    printf '#!%s%s\n' "${INTERP}" "${args}"
                    tail -n +2 "${f}"
                } > "${tmp}"
                mv "${tmp}" "${f}"
                rewritten=$((rewritten + 1))
                ;;
        esac
    done
    echo "rewrite-shebangs: rewrote ${rewritten} script(s) under ${TREE} to ${INTERP}"
}
