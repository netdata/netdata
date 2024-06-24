#!/bin/sh

set -e

path="${1}"
group="${2}"
shift 2
caps="${*}"

[ "$(id -u)" -eq 0 ] || exit 0

chown root:"${group}" "${path}"
chmod 0750 "${path}"

if command -v setcap >/dev/null 2>&1; then
    capset="$(echo "${caps}" | tr ' ' ',')+eip"
    failed=0

    if (echo "${capset}" | grep -q "perfmon"); then
        if ! setcap "${capset}" "${path}" 2>/dev/null; then
            capset="$(echo "${capset}" | sed -e 's/perfmon/sys_admin/g')"

            if ! setcap "${capset}" "${path}" 2>/dev/null; then
                failed=1
            fi
        fi
    elif ! setcap "${capset}" "${path}" 2>/dev/null; then
        failed=1
    fi

    [ "${failed}" -eq 0 ] && exit 0
fi

# Fall back to SUID if we can’t set capabilities.
chmod 4750 "${path}"
