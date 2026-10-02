# SPDX-License-Identifier: GPL-3.0-or-later
# Source from Bash 3.2 or newer. Protocol output goes only to stdout.
# Collection helpers encode data; nd_next reads canonical collection requests.

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
    _nd_family_count=0 _nd_sample_count=0
    _nd_families=() _nd_kinds=() _nd_charts=()
    _nd_heads=() _nd_tails=() _nd_samples=() _nd_next_samples=()
    ND_FAMILY=
}

# A family handle belongs to the current snapshot. Save ND_FAMILY immediately;
# do not invoke declarations through command substitution (which loses state).
_nd_family() {
    ND_FAMILY=$_nd_family_count
    _nd_family_count=$((_nd_family_count+1))
    _nd_families[ND_FAMILY]=$2
    _nd_kinds[ND_FAMILY]=$1
    _nd_charts[ND_FAMILY]=
    _nd_heads[ND_FAMILY]=-1 _nd_tails[ND_FAMILY]=-1
}

_nd_handle() {
    if [[ ! $1 =~ ^(0|[1-9][0-9]*)$ ]] ||
        (( ${#1} > ${#_nd_family_count} || $1 >= _nd_family_count )) ||
        [[ -n ${2:-} && ${_nd_kinds[$1]:-} != "$2" ]]; then
        printf '%s\n' 'native: invalid family handle or sample kind' >&2
        return 1
    fi
}

nd_metric() {
    if (( $# < 1 || $# > 3 )); then
        printf '%s\n' 'native: metric requires name, optional type and unit' >&2; return 1
    fi
    local _nd_name _nd_type=${2:-gauge} _nd_unit _nd_json
    case $_nd_type in gauge|counter) ;; *)
        printf '%s\n' 'native: invalid scalar metric type' >&2; return 1;;
    esac
    _nd_quote "$1"; _nd_name=$_nd_json
    _nd_quote "${3:-value}"; _nd_unit=$_nd_json
    _nd_family scalar "{\"name\":$_nd_name,\"type\":\"$_nd_type\",\"unit\":$_nd_unit"
}

_nd_strings() {
    local _nd_sep= _nd_json
    _nd_strings_json='['
    while (( $# )); do
        _nd_quote "$1"
        _nd_strings_json+=$_nd_sep$_nd_json
        _nd_sep=,
        shift
    done
    _nd_strings_json+=']'
}

nd_stateset() {
    if (( $# < 3 )); then
        printf '%s\n' 'native: stateset requires name, mode and states' >&2; return 1
    fi
    local _nd_name _nd_mode=$2 _nd_json _nd_strings_json
    case $_nd_mode in enum|bitset) ;; *)
        printf '%s\n' 'native: invalid stateset mode' >&2; return 1;;
    esac
    _nd_quote "$1"; _nd_name=$_nd_json
    shift 2
    _nd_strings "$@"
    _nd_family stateset "{\"name\":$_nd_name,\"type\":\"stateset\",\"mode\":\"$_nd_mode\",\"states\":$_nd_strings_json"
}

nd_check() {
    if (( $# < 2 )); then
        printf '%s\n' 'native: check requires id and title' >&2; return 1
    fi
    local _nd_id _nd_title _nd_json _nd_strings_json
    _nd_quote "$1"; _nd_id=$_nd_json
    _nd_quote "$2"; _nd_title=$_nd_json
    shift 2
    _nd_strings "$@"
    _nd_family check "{\"id\":$_nd_id,\"title\":$_nd_title,\"by_labels\":$_nd_strings_json"
}

# Blank metadata arguments are omitted, including priority. The host validates
# family contracts, finite values and state membership before publishing.
nd_chart() {
    if (( $# != 4 )); then
        printf '%s\n' 'native: chart requires handle, title, family and priority' >&2; return 1
    fi
    _nd_handle "$1" || return
    case ${_nd_kinds[$1]} in scalar|stateset) ;; *)
        printf '%s\n' 'native: chart requires a metric family' >&2; return 1;;
    esac
    local _nd_meta='{' _nd_sep= _nd_json
    if [[ -n $2 ]]; then _nd_quote "$2"; _nd_meta+='"title":'$_nd_json; _nd_sep=,; fi
    if [[ -n $3 ]]; then _nd_quote "$3"; _nd_meta+=$_nd_sep'"family":'$_nd_json; _nd_sep=,; fi
    if [[ -n $4 ]]; then
        if [[ ! $4 =~ ^[1-9][0-9]*$ ]]; then
            printf '%s\n' 'native: priority must be a positive integer' >&2; return 1
        fi
        _nd_meta+=$_nd_sep'"priority":'$4
    fi
    _nd_charts[$1]=',"chart_meta":'$_nd_meta'}'
}

# Store each encoded sample once and link indices per family. Interleaving does
# not rescan observations or repeatedly copy a growing family JSON string.
# The helper visits each sample once on output. Bash 3.2 indexed-array lookup
# itself can cost more as family count grows; fixed-family sample scaling is linear.
_nd_sample() {
    local _nd_handle=$1 _nd_tail=${_nd_tails[$1]}
    _nd_samples[_nd_sample_count]=$2
    _nd_next_samples[_nd_sample_count]=-1
    if (( _nd_tail < 0 )); then
        _nd_heads[_nd_handle]=$_nd_sample_count
    else
        _nd_next_samples[_nd_tail]=$_nd_sample_count
    fi
    _nd_tails[_nd_handle]=$_nd_sample_count
    _nd_sample_count=$((_nd_sample_count+1))
}

nd_sample() {
    if (( $# < 2 )) || [[ ! $2 =~ ^-?(0|[1-9][0-9]*)(\.[0-9]+)?([eE][+-]?[0-9]+)?$ ]]; then
        printf '%s\n' 'native: sample requires handle and JSON number' >&2; return 1
    fi
    _nd_handle "$1" scalar || return
    local _nd_handle=$1 _nd_number=$2 _nd_labels_json
    shift 2
    _nd_labels "$@" || return
    _nd_sample "$_nd_handle" "{\"value\":$_nd_number,\"labels\":$_nd_labels_json}"
}

nd_state_sample() {
    local _nd_available=$(($# - 2))
    # Bound decimal length before arithmetic, as with family handles.
    if (( $# < 2 )) || [[ ! $2 =~ ^(0|[1-9][0-9]*)$ ]] ||
        (( ${#2} > ${#_nd_available} || $2 > _nd_available )); then
        printf '%s\n' 'native: state sample requires handle, active count and active states' >&2; return 1
    fi
    _nd_handle "$1" stateset || return
    local _nd_handle=$1 _nd_count=$2 _nd_labels_json _nd_strings_json
    shift 2
    _nd_strings "${@:1:$_nd_count}"
    shift "$_nd_count" || return
    _nd_labels "$@" || return
    _nd_sample "$_nd_handle" "{\"active\":$_nd_strings_json,\"labels\":$_nd_labels_json}"
}

nd_check_sample() {
    if (( $# < 2 )); then
        printf '%s\n' 'native: check sample requires handle and state' >&2; return 1
    fi
    _nd_handle "$1" check || return
    case $2 in ok|warning|critical|unknown) ;; *)
        printf '%s\n' 'native: invalid check state' >&2; return 1;;
    esac
    local _nd_handle=$1 _nd_state=$2 _nd_labels_json
    shift 2
    _nd_labels "$@" || return
    _nd_sample "$_nd_handle" "{\"state\":\"$_nd_state\",\"labels\":$_nd_labels_json}"
}

_nd_emit_families() {
    local _nd_kind=$1 _nd_i _nd_j _nd_sep= _nd_sample_sep
    for ((_nd_i=0; _nd_i<_nd_family_count; _nd_i++)); do
        if [[ $_nd_kind == checks ]]; then
            [[ ${_nd_kinds[_nd_i]} == check ]] || continue
        else
            [[ ${_nd_kinds[_nd_i]} != check ]] || continue
        fi
        printf '%s%s%s,"samples":[' "$_nd_sep" "${_nd_families[_nd_i]}" "${_nd_charts[_nd_i]}"
        _nd_sep=, _nd_sample_sep=
        for ((_nd_j=${_nd_heads[_nd_i]}; _nd_j>=0; _nd_j=${_nd_next_samples[_nd_j]})); do
            printf '%s%s' "$_nd_sample_sep" "${_nd_samples[_nd_j]}"
            _nd_sample_sep=,
        done
        printf '%s' ']}'
    done
}

nd_end() {
    if [[ -n ${_nd_request_id:-} ]]; then
        printf '{"id":"%s","result":' "$_nd_request_id"
    fi
    printf '%s' '{"version":"v1","metrics":['
    _nd_emit_families metrics
    printf '%s' '],"checks":['
    _nd_emit_families checks
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

# For packages with Functions, parse ND_REQUEST using jq or another JSON decoder.
# This also reads the one-shot Function request after optional nd_read_config.
nd_read_request() {
    IFS= read -r ND_REQUEST
}

# Emit a correlated reply. The caller supplies one compact JSON result object
# produced by a JSON encoder; this helper only validates and quotes the host ID.
nd_reply() {
    if (( $# != 2 )) || [[ ! $1 =~ ^[1-9][0-9]*$ || $2 == *$'\n'* ]]; then
        printf '%s\n' 'native: reply requires a request ID and compact JSON result' >&2
        return 1
    fi
    printf '{"id":"%s","result":%s}\n' "$1" "$2"
}
