#!/bin/sh

set -e

user="${1}"
group="${2}"
homedir="${3}"

[ "$(id -u)" -eq 0 ] || exit 0

need_group=1
need_user=1

if command -v getent > /dev/null 2>&1; then
    if getent group "${group}" > /dev/null 2>&1; then
        need_group=0
    fi
elif command -v dscl > /dev/null 2>&1; then
    if dscl . read /Groups/"${group}" >/dev/null 2>&1; then
        need_group=0
    fi
else
    if cut -d ':' -f 1 < /etc/group | grep "^${group}$" 1> /dev/null 2>&1; then
        need_group=0
    fi
fi

if [ "${need_group}" -ne 0 ]; then
    if command -v groupadd 1> /dev/null 2>&1; then
        groupadd -r "${group}"
    elif command -v pw 1> /dev/null 2>&1; then
        pw groupadd "${group}"
    elif command -v addgroup 1> /dev/null 2>&1; then
        addgroup "${group}"
    elif command -v dseditgroup 1> /dev/null 2>&1; then
        dseditgroup -o create "${group}"
    fi
fi

if command -v getent > /dev/null 2>&1; then
    if getent passwd "${user}" > /dev/null 2>&1; then
        need_user=0
    fi
elif command -v dscl > /dev/null 2>&1; then
    if dscl . read /Users/"${user}" >/dev/null 2>&1; then
        need_user=0
    fi
else
    if cut -d ':' -f 1 < /etc/passwd | grep "^${user}$" 1> /dev/null 2>&1; then
        need_user=0
    fi
fi

if [ "${need_user}" -ne 0 ]; then
    nologin="$(command -v nologin || echo '/bin/false')"

    if command -v useradd 1> /dev/null 2>&1; then
        useradd -r -g "${user}" -c "${user}" -s "${nologin}" --no-create-home -d "${homedir}" "${user}"
    elif command -v pw 1> /dev/null 2>&1; then
        pw useradd "${user}" -d "${homedir}" -g "${user}" -s "${nologin}"
    elif command -v adduser 1> /dev/null 2>&1; then
        adduser -h "${homedir}" -s "${nologin}" -D -G "${user}" "${user}"
    elif command -v sysadminctl 1> /dev/null 2>&1; then
        gid=$(dscl . read /Groups/"${group}" 2>/dev/null | grep PrimaryGroupID | grep -Eo "[0-9]+")
        if sysadminctl -addUser "${user}" -shell /usr/bin/false -home /var/empty -GID "$gid"; then
            # FIXME: I think the proper solution is to create a role account:
            # -roleAccount + name starting with _ and UID in 200-400 range.
            dscl . create /Users/"${user}" IsHidden 1
        fi
    fi
fi
