# SPDX-License-Identifier: GPL-3.0-or-later
# Source from Bash 3.2 or newer. Protocol output goes only to stdout.
# Collection helpers encode data; nd_next reads one canonical host request.

_nd_quote() {
    local _nd_value=$1 _nd_code _nd_char _nd_escape
    _nd_value=${_nd_value//\\/\\\\}
    _nd_value=${_nd_value//\"/\\\"}
    for ((_nd_code=1; _nd_code<32; _nd_code++)); do
        printf -v _nd_escape '\\%03o' "$_nd_code"
        printf -v _nd_char '%b' "$_nd_escape"
        printf -v _nd_escape '\\u%04x' "$_nd_code"
        _nd_value=${_nd_value//"$_nd_char"/$_nd_escape}
    done
    _nd_json='"'$_nd_value'"'
}

_nd_labels() {
    if (( $# % 2 )); then
        printf '%s\n' 'native: labels require key/value pairs' >&2
        return 1
    fi
    local _nd_sep= _nd_key
    _nd_labels_json='{'
    while (( $# )); do
        _nd_quote "$1"; _nd_key=$_nd_json
        _nd_quote "$2"
        _nd_labels_json+=$_nd_sep$_nd_key:$_nd_json
        _nd_sep=,
        shift 2
    done
    _nd_labels_json+='}'
}

nd_begin() {
    _nd_metrics=()
    _nd_checks=()
}

nd_metric() {
    if (( $# < 2 )) || [[ ! $2 =~ ^-?(0|[1-9][0-9]*)(\.[0-9]+)?([eE][+-]?[0-9]+)?$ ]]; then
        printf '%s\n' 'native: metric requires a name and a JSON number' >&2
        return 1
    fi
    local _nd_name _nd_number=$2 _nd_json _nd_labels_json
    _nd_quote "$1"; _nd_name=$_nd_json
    shift 2
    _nd_labels "$@" || return
    _nd_metrics+=("{\"name\":$_nd_name,\"value\":$_nd_number,\"labels\":$_nd_labels_json}")
}

nd_check() {
    if (( $# < 2 )); then
        printf '%s\n' 'native: check requires an id and state' >&2
        return 1
    fi
    case $2 in ok|warning|critical|unknown) ;; *)
        printf '%s\n' 'native: invalid check state' >&2; return 1;;
    esac
    local _nd_id _nd_state=$2 _nd_json _nd_labels_json
    _nd_quote "$1"; _nd_id=$_nd_json
    shift 2
    _nd_labels "$@" || return
    _nd_checks+=("{\"id\":$_nd_id,\"state\":\"$_nd_state\",\"labels\":$_nd_labels_json}")
}

nd_end() {
    local _nd_item _nd_sep=
    if [[ -n ${_nd_request_id:-} ]]; then
        printf '{"id":"%s","result":' "$_nd_request_id"
    fi
    printf '%s' '{"version":"v1","metrics":['
    # Bash 3.2 with nounset treats an empty array as unset.
    for _nd_item in ${_nd_metrics[@]+"${_nd_metrics[@]}"}; do
        printf '%s%s' "$_nd_sep" "$_nd_item"; _nd_sep=,
    done
    printf '%s' '],"checks":['
    _nd_sep=
    for _nd_item in ${_nd_checks[@]+"${_nd_checks[@]}"}; do
        printf '%s%s' "$_nd_sep" "$_nd_item"; _nd_sep=,
    done
    printf '%s' ']}'
    if [[ -n ${_nd_request_id:-} ]]; then
        printf '%s' '}'
        _nd_request_id=
    fi
    printf '\n'
}

# Persistent mode: emit readiness once, then answer each nd_next with nd_end or
# nd_fail. The host deliberately uses this canonical spelling for Bash peers.
nd_ready() {
    _nd_request_id=
    printf '%s\n' '{"version":"v1","ready":true}'
}

nd_next() {
    if [[ -n ${_nd_request_id:-} ]]; then
        printf '%s\n' 'native: previous request has no reply' >&2
        return 1
    fi
    local _nd_line _nd_pattern='^\{"id":"([1-9][0-9]*)","method":"collect"\}$'
    IFS= read -r _nd_line || return 1
    if [[ ! $_nd_line =~ $_nd_pattern ]]; then
        printf '%s\n' 'native: invalid collection request' >&2
        return 1
    fi
    _nd_request_id=${BASH_REMATCH[1]}
}

nd_fail() {
    if [[ -z ${_nd_request_id:-} ]]; then
        printf '%s\n' 'native: no pending collection request' >&2
        return 1
    fi
    printf '{"id":"%s","error":"collection_failed"}\n' "$_nd_request_id"
    _nd_request_id=
}

# Call once before nd_ready (persistent) or collection (one-shot) when the
# manifest declares config_schema. ND_CONFIG stays a shell variable, not an
# environment variable. Scripts may parse it with jq or another JSON decoder.
nd_read_config() {
    IFS= read -r ND_CONFIG || {
        printf '%s\n' 'native: missing configuration envelope' >&2
        return 1
    }
}
