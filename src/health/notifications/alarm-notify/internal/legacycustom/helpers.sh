# SPDX-License-Identifier: GPL-3.0-or-later
# shellcheck shell=bash
# Embedded legacy helpers. Child output is discarded by the foreground runner.
info() { printf '%s\n' "$*" >&2; }
error() { printf '%s\n' "$*" >&2; }

urlencode() {
    local LC_ALL=C string="$1" encoded='' pos c o
    for ((pos=0; pos<${#string}; pos++)); do
        c=${string:pos:1}
        case "$c" in
            [-_.~a-zA-Z0-9]) o="$c" ;;
            *) printf -v o '%%%02x' "'$c" ;;
        esac
        encoded+="$o"
    done
    REPLY="$encoded"
    printf '%s\n' "$REPLY"
}

duration4human() {
    local s="$1" d h m ds=day hs=hour ms=minute ss=second
    REPLY=''
    # Do not interpret caller text as an arithmetic expression.
    [[ $s =~ ^[0-9]{1,10}$ ]] || return 1
    s=$((10#$s))
    ((s <= 4294967295)) || return 1
    d=$((s / 86400)); s=$((s % 86400))
    h=$((s / 3600)); s=$((s % 3600))
    m=$((s / 60)); s=$((s % 60))
    if ((d > 0)); then
        ((m >= 30)) && h=$((h + 1))
        ((d > 1)) && ds=days
        ((h > 1)) && hs=hours
        REPLY="$d $ds"
        ((h > 0)) && REPLY+=" and $h $hs"
    elif ((h > 0)); then
        ((s >= 30)) && m=$((m + 1))
        ((h > 1)) && hs=hours
        ((m > 1)) && ms=minutes
        REPLY="$h $hs"
        ((m > 0)) && REPLY+=" and $m $ms"
    elif ((m > 0)); then
        ((m > 1)) && ms=minutes
        ((s > 1)) && ss=seconds
        REPLY="$m $ms"
        ((s > 0)) && REPLY+=" and $s $ss"
    else
        ((s > 1)) && ss=seconds
        REPLY="$s $ss"
    fi
    printf '%s\n' "$REPLY"
}

docurl() {
    [[ -n $curl ]] || return 1
    "$curl" "${_netdata_custom_curl_options[@]}" --write-out '%{http_code}' --output /dev/null --silent --show-error "$@"
}
