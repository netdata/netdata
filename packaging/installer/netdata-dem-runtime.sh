#!/bin/sh
# SPDX-License-Identifier: GPL-3.0-or-later
# Short-lived administrator entry point; runtime acquisition always stays nonroot.
# Chained checks intentionally fail if any predicate or operation fails.
# They are not if/then/else expressions: the final return/exit is the shared error path.
# shellcheck disable=SC2015
PATH=/usr/sbin:/usr/bin:/sbin:/bin
export PATH
LC_ALL=C
export LC_ALL
umask 077
HOST_STATE=/var/lib/netdata-dem-runtime-host-policy
host_docker_policy_helper=
HOST_RUN=/run
HOST_APPARMOR_DIR=/etc/apparmor.d
HOST_APPARMOR_ENABLED=/sys/module/apparmor/parameters/enabled
HOST_APPARMOR_PROFILES=/sys/kernel/security/apparmor/profiles

host_error() { printf 'netdata-dem-runtime: %s\n' "$*" >&2; }
host_die() { host_error "$*"; exit 1; }
host_run() {
    # Variable arguments can include credential-bearing URLs. Log only the action.
    host_quoted=$(printf '%s' "$1" | sed "s/'/'\\\\''/g")
    printf "netdata-dem-runtime: run '%s' (arguments redacted)\n" "$host_quoted" >&2
    if "$@"; then return 0; else
        host_rc=$?
        host_error "operation failed in $(pwd): status $host_rc"
        return "$host_rc"
    fi
}
host_text() {
    [ -n "$1" ] && [ "${#1}" -le 4096 ] &&
        [ "$(printf '%s' "$1" | tr -d '\000-\037\177' | wc -c | tr -d ' ')" = "${#1}" ]
}
host_path() {
    host_text "$1" || return 1
    case "$1" in /) return 1 ;; /*) ;; *) return 1 ;; esac
    case "$1/" in *//*|*/./*|*/../*) return 1 ;; esac
}
host_token() {
    [ -n "$1" ] && [ "${#1}" -le 128 ] || return 1
    case "$1" in *[!a-zA-Z0-9_.-]*|.|..) return 1 ;; esac
}
host_hash() {
    host_hash_output=$(printf '%s' "$1" | sha256sum) || return $?
    host_hash_value=${host_hash_output%% *}
    [ "${#host_hash_value}" = 64 ] || { host_error 'sha256sum returned an invalid digest'; return 1; }
    case "$host_hash_value" in *[!0-9a-f]*) host_error 'sha256sum returned an invalid digest'; return 1 ;; esac
    printf '%s\n' "$host_hash_value"
}
host_untrusted_path() {
    host_error "untrusted path: $1 (owner UID=${2%%:*}, mode=0${2#*:})"
    host_error "$3"
    host_error 'Inspect this path and its parents with namei -l before changing ownership or permissions, then retry.'
    return 1
}
host_root_owned() {
    host_meta=$(stat -c '%u:%a' "$1") || return $?
    host_mode=${host_meta#*:}
    case "$host_meta" in 0:*) [ $((0$host_mode & 022)) -eq 0 ] && return 0 ;; esac
    host_untrusted_path "$1" "$host_meta" \
        'Requires owner UID=0 and no group/other write permission. For a path intended to be root-managed, correct ownership and use chmod go-w on that path only.'
}
host_ancestors() {
    host_parent=$1
    while :; do
        [ ! -L "$host_parent" ] && [ -d "$host_parent" ] || return 1
        host_root_owned "$host_parent" || return $?
        [ "$host_parent" != / ] || break
        host_parent=${host_parent%/*}
        [ -n "$host_parent" ] || host_parent=/
    done
}
host_file() {
    [ ! -L "$1" ] && [ -f "$1" ] && [ "$(stat -c '%u:%a' "$1")" = "0:$2" ]
}
host_executable() {
    host_path "$1" || return 1
    host_ancestors "${1%/*}" || return 1
    [ ! -L "$1" ] && [ -f "$1" ] && [ -x "$1" ] || return 1
    host_root_owned "$1"
}
host_resolve_executable() {
    # Walk the original chain and every link target; realpath alone loses who
    # controls the aliases. Static installs legitimately use root-owned aliases.
    host_path "$1" && host_ancestors / || return 1
    host_remaining=${1#/}
    host_resolved=/
    host_links=0
    while [ -n "$host_remaining" ]; do
        [ "${#host_remaining}" -le 4096 ] || return 1
        host_component=${host_remaining%%/*}
        case "$host_remaining" in */*) host_remaining=${host_remaining#*/} ;; *) host_remaining= ;; esac
        case "$host_component" in
            ''|.) continue ;;
            ..) host_resolved=${host_resolved%/*}; [ -n "$host_resolved" ] || host_resolved=/; continue ;;
        esac
        host_candidate=${host_resolved%/}/$host_component
        if [ -L "$host_candidate" ]; then
            host_links=$((host_links + 1))
            [ "$host_links" -le 40 ] || return 1
            host_link_meta=$(stat -c '%u:%a' "$host_candidate") || return $?
            case "$host_link_meta" in 0:*) ;; *)
                host_untrusted_path "$host_candidate" "$host_link_meta" \
                    'Requires a root-owned symbolic link (UID=0). Have an administrator correct the link ownership or use a root-owned installation.'
                return 1 ;;
            esac
            # Preserve trailing controls instead of letting command substitution trim them.
            host_target=$(readlink -n -- "$host_candidate" && printf '.') || return $?
            host_target=${host_target%.}
            host_text "$host_target" || return 1
            case "$host_target" in */) host_target=${host_target}. ;; esac
            case "$host_target" in /*) host_resolved=/; host_target=${host_target#/} ;; esac
            if [ -n "$host_remaining" ]; then host_remaining=$host_target/$host_remaining;
            else host_remaining=$host_target; fi
        else
            if [ -n "$host_remaining" ]; then
                [ -d "$host_candidate" ] && host_root_owned "$host_candidate" || return 1
            fi
            host_resolved=$host_candidate
        fi
    done
    host_executable "$host_resolved" || return 1
    printf '%s\n' "$host_resolved"
}
host_directory() {
    if [ ! -e "$1" ] && [ ! -L "$1" ]; then
        mkdir -m 700 -- "$1" || [ -d "$1" ] || return $?
    fi
    [ ! -L "$1" ] && [ -d "$1" ] && [ "$(stat -c '%u:%a' "$1")" = 0:700 ]
}
host_lock() {
    host_ancestors "${HOST_STATE%/*}" || host_die 'host state ancestors must be root-owned and not writable by other users'
    host_directory "$HOST_STATE" || host_die 'host state must be a plain root-owned 0700 directory'
    if [ ! -e "$HOST_STATE/lock" ] && [ ! -L "$HOST_STATE/lock" ]; then
        (set -C; : > "$HOST_STATE/lock") || host_file "$HOST_STATE/lock" 600 || exit $?
    fi
    host_file "$HOST_STATE/lock" 600 || host_die 'unsafe host lock file'
    command -v flock >/dev/null 2>&1 || host_die 'host setup requires flock (util-linux)'
    exec 9> "$HOST_STATE/lock" || exit $?
    flock -x 9 || exit $?
    host_directory "$HOST_STATE/registrations" && host_directory "$HOST_STATE/policies" || host_die 'unsafe host metadata directory'
    host_tmp=$(mktemp -d "$HOST_STATE/work.XXXXXXXX") || exit $?
    host_drain_dir=
    host_publish=
    trap 'rm -rf -- "$host_tmp"; if [ -n "$host_publish" ]; then rm -f -- "$host_publish"; fi; if [ -n "$host_drain_dir" ]; then rm -rf -- "$host_drain_dir"; fi' 0
    trap 'exit 1' 1 2 3 15
    # Stale private work directories from an interrupted operation are inert.
    for host_entry in "$HOST_STATE"/*; do
        case "${host_entry##*/}" in lock|registrations|policies|docker-helpers|work.*) ;; *) host_die 'unknown host state entry; inspect before proceeding' ;; esac
    done
}
host_atomic() {
    # Publication must stay on the destination filesystem, including /etc policy.
    host_publish=$(mktemp "${2%/*}/.${2##*/}.XXXXXXXX") || return $?
    if cp -- "$1" "$host_publish" && chmod "$3" "$host_publish" &&
        sync -f "$host_publish" && mv -- "$host_publish" "$2"; then
        host_publish=
        sync -f "${2%/*}"
    else
        host_atomic_rc=$?
        rm -f -- "$host_publish"
        host_publish=
        return "$host_atomic_rc"
    fi
}

host_apparmor() {
    host_aa=no
    if [ ! -e "$HOST_APPARMOR_ENABLED" ]; then return 0; fi
    [ -r "$HOST_APPARMOR_ENABLED" ] || host_die 'cannot inspect AppArmor enabled state'
    host_enabled=$(cat "$HOST_APPARMOR_ENABLED") || exit $?
    case "$host_enabled" in N) return 0 ;; Y) host_aa=yes ;; *) host_die 'unknown AppArmor enabled state' ;; esac
    [ -r "$HOST_APPARMOR_PROFILES" ] || host_die 'enabled AppArmor requires readable loaded-profile inventory'
    command -v apparmor_parser >/dev/null 2>&1 || host_die 'enabled AppArmor requires apparmor_parser; install host AppArmor tools before retrying (offline setup does not download prerequisites)'
    host_ancestors "$HOST_APPARMOR_DIR" || host_die 'unsafe AppArmor policy directory'
}
host_loaded() {
    host_live=no
    [ "$host_aa" = yes ] || return 0
    while IFS= read -r host_line || [ -n "$host_line" ]; do
        case "$host_line" in
            "$host_name (unconfined)") host_live=yes ;;
            "$host_name ("*) host_die 'loaded profile has an unexpected administrator-controlled mode; refusing automatic changes' ;;
        esac
    done < "$HOST_APPARMOR_PROFILES" || host_die 'cannot read loaded AppArmor inventory'
}
host_profile() {
    host_apparmor
    host_digest=$(host_hash "$host_root") || exit $?
    host_name=netdata-dem-launcher-$host_digest
    host_policy=$HOST_APPARMOR_DIR/$host_name
    host_owner=$HOST_STATE/policies/$host_digest
    host_escaped=$(printf '%s' "$host_root" | awk '{for(i=1;i<=length($0);i++){c=substr($0,i,1); if(index("\\\"*?[]{}^@",c))printf "\\"; printf "%s",c}}') || exit $?
    printf '%s\n' '# Generated by netdata-dem-runtime.sh; administrator changes require manual reconciliation.' \
        'abi <abi/3.0>,' "profile $host_name \"$host_escaped/generations/*/chrome\" flags=(unconfined) {" '}' > "$host_tmp/profile" || exit $?
    printf '%s\n' netdata-dem-host-policy-v1 "$host_root" > "$host_tmp/owner" || exit $?
    if [ -e "$host_owner" ] || [ -L "$host_owner" ]; then
        host_file "$host_owner" 600 && cmp -s "$host_owner" "$host_tmp/owner" || host_die 'unknown or modified policy ownership metadata'
        host_owned=yes
    else host_owned=no; fi
    if [ -e "$host_policy" ] || [ -L "$host_policy" ]; then
        [ "$host_owned" = yes ] && host_file "$host_policy" 644 && cmp -s "$host_policy" "$host_tmp/profile" || host_die 'unknown or administrator-modified AppArmor policy; refusing automatic changes'
        host_persistent=yes
    else host_persistent=no; fi
    if [ -e "$HOST_APPARMOR_DIR/disable/$host_name" ] || [ -L "$HOST_APPARMOR_DIR/disable/$host_name" ]; then
        host_die 'AppArmor policy is administrator-disabled; remove the override explicitly before retrying'
    fi
    host_loaded
    if [ "$host_persistent" = no ] && [ "$host_live" = yes ]; then
        host_die 'loaded policy has no matching persistent file; administrator reconciliation required'
    fi
    if [ "$host_owned" = no ] && [ "$host_live" = yes ]; then host_die 'unowned loaded policy'; fi
}
host_record_write() {
    printf '%s\n' netdata-dem-host-registration-v1 "$host_kind" "$host_root" "$host_agent" "$host_user" \
        "$host_plugin" "$host_config" "$host_stock" "$host_lib" > "$host_tmp/record" || exit $?
}
host_record_read() {
    host_file "$1" 600 || host_die 'unsafe registration metadata'
    [ "$(wc -l < "$1" | tr -d ' ')" = 9 ] && [ "$(wc -c < "$1" | tr -d ' ')" -le 30000 ] || host_die 'unsupported or incomplete registration'
    # Reject NUL and all other controls before shell command substitution.
    [ "$(tr -d '\000-\011\013-\037\177' < "$1" | wc -c | tr -d ' ')" = "$(wc -c < "$1" | tr -d ' ')" ] || host_die 'control bytes in registration'
    { IFS= read -r host_magic; IFS= read -r host_kind; IFS= read -r host_root; IFS= read -r host_agent;
      IFS= read -r host_user; IFS= read -r host_plugin; IFS= read -r host_config;
      IFS= read -r host_stock; IFS= read -r host_lib; } < "$1"
    [ "$host_magic" = netdata-dem-host-registration-v1 ] && host_path "$host_root" || host_die 'unknown registration version or root'
    case "$host_kind" in
        external) [ "$host_agent:$host_user:$host_plugin:$host_config:$host_stock:$host_lib" = '-:-:-:-:-:-' ] || host_die 'invalid external registration' ;;
        managed|preloaded)
            host_path "$host_agent" && host_text "$host_user" && host_path "$host_plugin" && host_path "$host_config" && host_path "$host_stock" && host_path "$host_lib" || host_die 'invalid Agent registration'
            ;;
        *) host_die 'unknown registration kind' ;;
    esac
}
host_inventory() {
    # A malformed reference must never be silently ignored during shared cleanup.
    for host_record in "$HOST_STATE/registrations"/*; do
        [ -e "$host_record" ] || [ -L "$host_record" ] || continue
        case "${host_record##*/}" in
            deployment-*) host_basename=${host_record##*/}; host_token "${host_basename#deployment-}" || host_die 'invalid deployment registration filename' ;;
            agent-*) [ "${#host_record}" -lt 4096 ] || host_die 'invalid Agent registration filename' ;;
            *) host_die 'invalid registration filename' ;;
        esac
        host_record_read "$host_record"
        case "$host_kind" in
            external) case "${host_record##*/}" in deployment-*) ;; *) host_die 'external registration has invalid identity' ;; esac ;;
            *) [ "${host_record##*/}" = "agent-$(host_hash "$host_agent")-$(host_hash "$host_kind:$host_root")" ] || host_die 'Agent registration identity differs from metadata' ;;
        esac
    done
    for host_record in "$HOST_STATE/policies"/*; do
        [ -e "$host_record" ] || [ -L "$host_record" ] || continue
        host_file "$host_record" 600 || host_die 'unsafe policy metadata'
        [ "$(wc -l < "$host_record" | tr -d ' ')" = 2 ] || host_die 'unknown policy metadata'
        host_root=$(sed -n '2p' "$host_record")
        host_path "$host_root" && [ "${host_record##*/}" = "$(host_hash "$host_root")" ] || host_die 'invalid policy metadata identity'
        printf '%s\n' netdata-dem-host-policy-v1 "$host_root" > "$host_tmp/expected"
        cmp -s "$host_record" "$host_tmp/expected" || host_die 'unknown policy metadata version'
    done
}
host_register() {
    host_profile
    host_record_write
    host_registration=$HOST_STATE/registrations/$host_id
    if [ -e "$host_registration" ] || [ -L "$host_registration" ]; then
        host_file "$host_registration" 600 && cmp -s "$host_registration" "$host_tmp/record" || host_die 'deployment already registered with different metadata; remove its old registration explicitly'
    fi
    if [ "$host_owned" = no ]; then host_atomic "$host_tmp/owner" "$host_owner" 600 || return $?; fi
    host_atomic "$host_tmp/record" "$host_registration" 600 || return $?
    if [ "$host_aa" = no ]; then host_error 'AppArmor absent/disabled: policy skipped; registration retained'; return 0; fi
    if [ "$host_persistent" = no ]; then
        # Validate before publishing, respecting all administrator parser settings.
        host_run apparmor_parser -Q -T "$host_tmp/profile" || return $?
        host_atomic "$host_tmp/profile" "$host_policy" 644 || return $?
    fi
    host_run apparmor_parser -r "$host_policy" || return $?
    host_loaded
    [ "$host_live" = yes ] || { host_error 'parser returned success but policy is not loaded'; return 1; }
    host_error 'host policy ready; browser qualification remains a separate runtime operation'
}
host_unregister() {
    host_registration=$HOST_STATE/registrations/$host_id
    if [ ! -e "$host_registration" ] && [ ! -L "$host_registration" ]; then host_error 'deployment is not registered'; return 0; fi
    host_record_read "$host_registration"
    host_profile
    host_shared=no
    for host_other in "$HOST_STATE/registrations"/*; do
        [ "$host_other" != "$host_registration" ] || continue
        [ -e "$host_other" ] || continue
        host_other_root=$(sed -n '3p' "$host_other")
        if [ "$host_other_root" = "$host_root" ]; then host_shared=yes; fi
    done
    if [ "$host_shared" = yes ]; then
        rm -- "$host_registration" && sync -f "$HOST_STATE/registrations" || return $?
        host_error 'registration removed; shared policy retained'
        return 0
    fi
    if [ "$host_aa" = no ] && [ "$host_persistent" = yes ]; then
        host_error 'AppArmor is disabled but persistent policy remains; enable AppArmor or reconcile policy manually before cleanup'; return 1
    fi
    if [ "$host_live" = yes ]; then host_run apparmor_parser -R "$host_policy" || return $?; fi
    host_loaded
    [ "$host_live" = no ] || { host_error 'policy remains loaded; ownership retained'; return 1; }
    if [ "$host_persistent" = yes ]; then rm -- "$host_policy" && sync -f "$HOST_APPARMOR_DIR" || return $?; fi
    # Last reference remains until both disk and kernel policy are gone.
    if [ -e "$host_owner" ]; then rm -- "$host_owner" && sync -f "$HOST_STATE/policies" || return $?; fi
    rm -- "$host_registration" && sync -f "$HOST_STATE/registrations" || return $?
    host_error 'registration and host policy removed'
}
host_policy_status() {
    host_registration=$HOST_STATE/registrations/$host_id
    if [ ! -e "$host_registration" ] && [ ! -L "$host_registration" ]; then printf '%s\n' 'registration=absent'; return 0; fi
    host_record_read "$host_registration"
    host_profile
    printf 'registration=present\npolicy=%s\nloaded=%s\n' "$host_persistent" "$host_live"
    if [ "$host_aa" = no ]; then printf '%s\n' 'apparmor=skipped'; else printf '%s\n' 'apparmor=enabled'; fi
}
host_query() { (cd / && env -i PATH="$PATH" "$host_agent" -W get "$1" "$2" ''); }
host_discover() {
    host_agent=$(host_resolve_executable "$host_agent") || host_die 'Agent executable, aliases and every directory in their resolution must be root-owned and not writable by other users'
    # Preserve trailing controls, removing only the Agent's output terminator.
    host_user=$(host_query global 'run as user' && printf '.') || exit $?
    host_user=${host_user%.}
    host_user=${host_user%"
"}
    host_text "$host_user" || host_die 'a bounded service account without control bytes is required'
    host_uid=$(id -u -- "$host_user") || exit $?
    case "$host_uid" in ''|*[!0-9]*|0) host_die 'a nonroot service UID is required' ;; esac
    host_config=$(host_query directories config) || exit $?
    host_stock=$(host_query directories 'stock config') || exit $?
    host_lib=$(host_query directories lib) || exit $?
    [ -z "$host_config_override" ] || host_config=$host_config_override
    host_path "$host_config" && host_path "$host_stock" && host_path "$host_lib" || host_die 'invalid Agent directory configuration'
    host_list=$(host_query directories plugins) || exit $?
    host_plugin=
    while [ -n "$host_list" ]; do
        host_list=${host_list#"${host_list%%[![:space:]]*}"}
        [ -n "$host_list" ] || break
        case "$host_list" in
            \"*) host_list=${host_list#\"}; case "$host_list" in *\"*) ;; *) host_die 'malformed plugins list' ;; esac; host_dir=${host_list%%\"*}; host_list=${host_list#*\"} ;;
            \') host_die 'malformed plugins list' ;;
            \'*) host_list=${host_list#\'}; case "$host_list" in *\'*) ;; *) host_die 'malformed plugins list' ;; esac; host_dir=${host_list%%\'*}; host_list=${host_list#*\'} ;;
            *) host_dir=${host_list%%[[:space:]]*}; host_list=${host_list#"$host_dir"} ;;
        esac
        case "$host_list" in ''|[[:space:]]*) ;; *) host_die 'malformed plugins list' ;; esac
        host_path "$host_dir" || host_die 'invalid plugin directory'
        if [ -x "$host_dir/dem.plugin" ]; then host_plugin=$host_dir/dem.plugin; break; fi
    done
    [ -n "$host_plugin" ] || host_die 'an installed dem.plugin is required'
    host_plugin=$(host_resolve_executable "$host_plugin") || host_die 'a trusted installed dem.plugin and alias chain are required'
    command -v runuser >/dev/null 2>&1 || host_die 'runuser (util-linux) is required to run runtime setup as the service account'
}
host_cli() {
    host_executable "$host_plugin" || { host_error 'registered plugin is no longer trusted'; return 1; }
    host_uid=$(id -u -- "$host_user") || return $?
    case "$host_uid" in ''|*[!0-9]*|0) host_error 'registered service identity is no longer nonroot'; return 1 ;; esac
    (exec 9>&-; cd / && host_run runuser -u "$host_user" -- env -i PATH="$PATH" \
        NETDATA_LIB_DIR="$host_lib" NETDATA_USER_CONFIG_DIR="$host_config" NETDATA_STOCK_CONFIG_DIR="$host_stock" \
        "$host_plugin" runtime "$@" --config-dir "$host_config")
}
host_plan() {
    if [ -n "$host_cache" ]; then
        host_cli setup-plan --cache "$host_cache" > "$host_tmp/plan" || return $?
    elif [ -n "$host_preload" ]; then
        host_cli setup-plan --root "$host_preload" > "$host_tmp/plan" || return $?
    else host_cli setup-plan > "$host_tmp/plan" || return $?; fi
    [ "$(wc -c < "$host_tmp/plan" | tr -d ' ')" -le 4200 ] && [ "$(wc -l < "$host_tmp/plan" | tr -d ' ')" = 4 ] || host_die 'invalid runtime setup-plan length'
    [ "$(tr -d '\000-\011\013-\037\177' < "$host_tmp/plan" | wc -c | tr -d ' ')" = "$(wc -c < "$host_tmp/plan" | tr -d ' ')" ] || host_die 'invalid runtime setup-plan control bytes'
    { IFS= read -r host_magic; IFS= read -r host_delivery; IFS= read -r host_kind; IFS= read -r host_root; } < "$host_tmp/plan"
    [ "$host_magic" = netdata-dem-runtime-setup-v1 ] || host_die 'unsupported runtime setup-plan version'
    case "$host_delivery" in standalone|native-package) ;; *) host_die 'unknown Agent delivery owner' ;; esac
    case "$host_kind" in managed|preloaded|native) ;; *) host_die 'unknown runtime setup mode' ;; esac
    host_path "$host_root" || host_die 'invalid runtime setup root'
    host_agent_digest=$(host_hash "$host_agent") || return $?
    host_root_digest=$(host_hash "$host_kind:$host_root") || return $?
    host_id=agent-$host_agent_digest-$host_root_digest
}
host_runtime_remove() {
    # An absent cache must not be created merely to uninstall the Agent.
    if [ ! -e "$host_root" ] && [ ! -L "$host_root" ]; then return 0; fi
    host_ancestors "$HOST_RUN" || { host_error 'unsafe runtime drain scratch directory'; return 1; }
    host_drain_dir=$(mktemp -d "$HOST_RUN/netdata-dem-runtime-drain.XXXXXXXX") || return $?
    # This contains no secrets. A root-controlled empty managed configuration
    # lets retained roots drain even after the installation's mode/config changes.
    printf '%s\n' 'runtime:' '  mode: managed' > "$host_drain_dir/dem.conf" &&
        chmod 644 "$host_drain_dir/dem.conf" && chmod 755 "$host_drain_dir" || return $?
    if (host_config=$host_drain_dir; host_stock=$host_drain_dir; host_cli uninstall --cache "$host_root"); then host_remove_rc=0;
    else host_remove_rc=$?; fi
    rm -rf -- "$host_drain_dir" || return $?
    host_drain_dir=
    return "$host_remove_rc"
}
host_agent_remove() {
    host_identity=$host_agent
    # Inspect all retained records before deleting any runtime files.
    for host_item in "$HOST_STATE/registrations"/*; do
        [ -e "$host_item" ] || continue
        host_record_read "$host_item"
        if [ "$host_agent" = "$host_identity" ] && [ "$host_kind" = preloaded ] && [ "$host_retired" != yes ]; then
            host_die 'registered protected preload requires explicit retirement: use agent-remove --deployments-stopped after stopping every deployment'
        fi
    done
    for host_item in "$HOST_STATE/registrations"/*; do
        [ -e "$host_item" ] || continue
        host_record_read "$host_item"
        [ "$host_agent" = "$host_identity" ] || continue
        host_id=${host_item##*/}
        if [ "$host_kind" = managed ]; then
            host_runtime_remove || return $?
            printf '%s\n' "$host_root" >> "$host_tmp/drained" || return $?
        fi
        if host_unregister; then :; else
            host_cleanup_rc=$?
            host_error 'runtime retirement completed; host policy cleanup is pending; retry agent-remove'
            return "$host_cleanup_rc"
        fi
    done
    host_agent=$host_identity
    host_discover
    host_plan || return $?
    if [ "$host_kind" = native ]; then host_error 'native runtime remains package-manager owned'; fi
    if [ "$host_kind" = preloaded ] && [ "$host_retired" != yes ]; then
        host_die 'configured protected preload requires explicit --deployments-stopped before Agent removal'
    fi
    if [ "$host_kind" = managed ]; then
        if [ -f "$host_tmp/drained" ] && grep -Fx -- "$host_root" "$host_tmp/drained" >/dev/null; then return 0; fi
        host_runtime_remove || return $?
    fi
}
host_usage() {
    cat <<'HELP'
Usage: netdata-dem-runtime.sh install|repair|preload|uninstall|status --agent /installed/sbin/netdata [runtime flags]
       netdata-dem-runtime.sh agent-remove --agent /installed/sbin/netdata [--deployments-stopped]
       netdata-dem-runtime.sh policy-prepare --deployment TOKEN --runtime-root /execution/visible/root
       netdata-dem-runtime.sh policy-status --deployment TOKEN
       netdata-dem-runtime.sh policy-remove --deployment TOKEN --deployments-stopped
Host tools: flock/runuser, sha256sum and sync -f (util-linux/coreutils); AppArmor tools when enabled.
Runtime flags: --config-dir DIR --cache DIR --root DIR --release FILE --archive FILE --allow-download --manual-updates
Setup prepares an administrator-owned namespace allowance and then runs signed runtime setup as the service user.
Agent-linked setup is for static/source builds; native Agent builds use the optional runtime package.
It does not grant confinement or restrict eligibility to signed bytes. Automatic updates remain unprivileged.
Docker policy operations run on the daemon host; they require no Agent and do not prove container retirement.
Stop standalone consumers and prevent concurrent administrator installs before Agent teardown.
Protected preloads require explicit retirement before policy removal; runtime preload files are never erased here.
To retire one prepared preload: uninstall --agent PATH --root DIR --deployments-stopped.
HELP
}

host_main() {
    [ "$#" -gt 0 ] || { host_usage; return 1; }
    host_action=$1; shift
    case "$host_action" in -h|--help|help) host_usage; return 0 ;; install|repair|preload|uninstall|status|agent-remove|policy-prepare|policy-status|policy-remove) ;; *) host_die 'unknown operation; use --help' ;; esac
    [ "$(id -u)" = 0 ] || host_die 'host setup requires administrator root privileges; run ordinary runtime commands as the service user'
    host_agent=; host_config_override=; host_cache=; host_preload=; host_release=; host_archive=; host_download=no; host_manual=no; host_retired=no; host_deployment=; host_root=
    while [ "$#" -gt 0 ]; do
        case "$1" in
            --agent=*|--config-dir=*|--cache=*|--root=*|--release=*|--archive=*|--deployment=*|--runtime-root=*)
                host_option=${1%%=*}; host_value=${1#*=}; shift; set -- "$host_option" "$host_value" "$@" ;;
            --allow-download=true) host_download=yes; shift ;; --allow-download=false) host_download=no; shift ;;
            --manual-updates=true) host_manual=yes; shift ;; --manual-updates=false) host_manual=no; shift ;;
            --allow-download) host_download=yes; shift ;; --manual-updates) host_manual=yes; shift ;; --deployments-stopped) host_retired=yes; shift ;;
            --agent|--config-dir|--cache|--root|--release|--archive|--deployment|--runtime-root)
                [ "$#" -ge 2 ] || host_die 'missing option value'
                host_text "$2" || host_die 'invalid option value'
                case "$1" in --agent) host_agent=$2 ;; --config-dir) host_config_override=$2 ;; --cache) host_cache=$2 ;; --root) host_preload=$2 ;; --release) host_release=$2 ;; --archive) host_archive=$2 ;; --deployment) host_deployment=$2 ;; --runtime-root) host_explicit_root=$2; host_root=$2 ;; esac
                shift 2 ;;
            *) host_die 'unknown option; use --help' ;;
        esac
    done
    case "$host_action" in
        policy-*)
            [ -z "$host_agent$host_config_override$host_cache$host_preload$host_release$host_archive" ] &&
                [ "$host_download:$host_manual" = no:no ] || host_die 'Agent/runtime flags are invalid for policy operations'
            host_token "$host_deployment" || host_die 'invalid deployment token'
            case "$host_action" in
                policy-prepare) host_path "$host_explicit_root" || host_die 'invalid runtime root'; [ "$host_retired" = no ] || host_die 'retirement flag is only valid for removal' ;;
                policy-status) [ -z "$host_explicit_root" ] && [ "$host_retired" = no ] || host_die 'invalid policy status flags' ;;
                policy-remove) [ -z "$host_explicit_root" ] && [ "$host_retired" = yes ] || host_die 'policy removal requires --deployments-stopped after every affected deployment is retired' ;;
            esac ;;
        *)
            [ -n "$host_agent" ] && host_path "$host_agent" || host_die 'provide --agent with the absolute installed Agent executable'
            [ -z "$host_deployment$host_explicit_root" ] || host_die 'deployment flags are only valid for policy operations'
            [ -z "$host_cache" ] || [ -z "$host_preload" ] || host_die '--cache and --root are mutually exclusive'
            for host_value in "$host_config_override" "$host_cache" "$host_preload" "$host_release" "$host_archive"; do
                [ -z "$host_value" ] || host_path "$host_value" || host_die 'runtime path flags must be absolute and normalized'
            done
            case "$host_action" in
                install|repair|preload)
                    [ "$host_retired" = no ] || host_die 'retirement flag is only valid for removal'
                    if [ "$host_download" = yes ]; then [ -z "$host_archive" ] || host_die 'choose --archive or --allow-download';
                    else [ -n "$host_archive" ] && [ -n "$host_release" ] || host_die 'setup requires explicit --allow-download or offline --release and --archive'; fi
                    if [ "$host_action" = preload ]; then
                        [ -n "$host_preload" ] && [ -z "$host_cache" ] && [ "$host_manual" = no ] || host_die 'preload requires --root and does not accept --cache or --manual-updates'
                    else [ -z "$host_preload" ] || host_die '--root is only valid for preload'; fi ;;
                *)
                    [ -z "$host_release$host_archive" ] && [ "$host_download:$host_manual" = no:no ] || host_die 'acquisition flags are only valid for setup'
                    if [ -n "$host_preload" ]; then
                        [ "$host_action:$host_retired" = uninstall:yes ] || host_die 'targeted preload cleanup requires uninstall --root PATH --deployments-stopped'
                    fi
                    if [ "$host_action" = status ]; then [ "$host_retired" = no ] || host_die 'invalid status retirement flag'; fi
                    if [ "$host_action" = agent-remove ]; then [ -z "$host_cache" ] || host_die 'agent-remove covers every registered root; --cache is invalid'; fi ;;
            esac ;;
    esac
    host_lock
    (host_inventory) || exit $?
    case "$host_action" in
        policy-*)
            host_token "$host_deployment" || host_die 'deployment must be a token of at most 128 ASCII letters, digits, dots, underscores or hyphens'
            host_id=deployment-$host_deployment
            case "$host_action" in
                policy-prepare)
                    host_root=$host_explicit_root
                    host_path "$host_root" || host_die 'runtime root must be an absolute normalized execution-visible path with no control bytes'
                    host_kind=external; host_agent=-; host_user=-; host_plugin=-; host_config=-; host_stock=-; host_lib=-
                    if [ -n "$host_docker_policy_helper" ]; then
                        # Publish recovery before the first durable policy change. Docker can disappear at any time.
                        host_directory "$HOST_STATE/docker-helpers" || host_die 'unsafe Docker recovery directory'
                        host_recovery=$HOST_STATE/docker-helpers/$host_id
                        if [ -e "$host_recovery" ] || [ -L "$host_recovery" ]; then
                            host_file "$host_recovery" 600 || host_die 'unsafe Docker recovery helper'
                        fi
                        host_atomic "$host_docker_policy_helper" "$host_recovery" 600 || return $?
                    fi
                    host_register ;;
                policy-status) host_policy_status ;;
                policy-remove)
                    host_unregister || return $?
                    if [ -d "$HOST_STATE/docker-helpers" ]; then
                        host_directory "$HOST_STATE/docker-helpers" || host_die 'unsafe Docker recovery directory'
                        rm -f -- "$HOST_STATE/docker-helpers/$host_id" && sync -f "$HOST_STATE/docker-helpers" || return $?
                    fi ;;
            esac
            return $? ;;
    esac
    host_discover
    if [ "$host_action" = agent-remove ]; then host_agent_remove; return $?; fi
    host_plan || return $?
    if [ "$host_kind" = native ]; then host_die 'native runtime lifecycle belongs to the package manager; use the netdata-dem-runtime package'; fi
    case "$host_action:$host_kind" in
        install:managed|repair:managed|preload:preloaded|uninstall:managed|uninstall:preloaded|status:managed|status:preloaded) ;;
        *) host_die 'runtime setup mode is incompatible with this operation; managed install/repair requires runtime.mode: managed' ;;
    esac
    case "$host_action" in
        install|repair|preload)
            [ "$host_delivery" = standalone ] || host_die 'native Agent builds use the optional netdata-dem-runtime package; managed/preload host setup is only supported for static/source installations'
            host_register || return $?
            set -- "$host_action"
            [ -z "$host_cache" ] || set -- "$@" --cache "$host_cache"
            [ -z "$host_preload" ] || set -- "$@" --root "$host_preload"
            [ -z "$host_release" ] || set -- "$@" --release "$host_release"
            [ -z "$host_archive" ] || set -- "$@" --archive "$host_archive"
            [ "$host_download" != yes ] || set -- "$@" --allow-download
            [ "$host_manual" != yes ] || set -- "$@" --manual-updates
            host_cli "$@" ;;
        uninstall)
            if [ "$host_kind" = preloaded ]; then
                [ "$host_retired" = yes ] || host_die 'preload policy removal requires explicit --deployments-stopped; preload files are retained'
            else host_runtime_remove || return $?; fi
            if host_unregister; then :; else
                host_cleanup_rc=$?
                host_error 'runtime removal completed; host policy cleanup is pending; retry uninstall'
                return "$host_cleanup_rc"
            fi ;;
        status)
            (host_policy_status) || return $?
            if [ -n "$host_cache" ]; then host_cli status --cache "$host_cache"; else host_cli status; fi ;;
    esac
}
host_explicit_root=
if [ "${1-}" = docker-install ]; then
    shift
    # Docker ships this sibling. Token retirement uses the policy helper alone.
    # shellcheck disable=SC1091
    . "${0%/*}/netdata-dem-docker-host.sh"
    docker_install "$@"
else
    host_main "$@"
fi
