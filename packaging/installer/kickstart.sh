#!/bin/sh
#
# SPDX-License-Identifier: GPL-3.0-or-later
#
# Next unused error code: F0530

# Docker actions are terminal: dispatch before host discovery or installation.
dem_docker_dispatch() {
  dem_terminal=no
  for dem_option do
    case "$dem_option" in --docker-container|--dem-docker-policy-status|--dem-docker-policy-remove) dem_terminal=yes ;; esac
  done
  [ "$dem_terminal" = yes ] || return 0
  PATH=/usr/sbin:/usr/bin:/sbin:/bin
  export PATH
  LC_ALL=C
  export LC_ALL
  umask 077
  dem_fail() { printf 'kickstart Docker setup: %s\n' "$*" >&2; exit 1; }
  dem_action=; dem_container=; dem_token=; dem_install=no; dem_dry=no; dem_stopped=no
  while [ "$#" -gt 0 ]; do
    case "$1" in
      --install-dem-runtime) [ "$dem_install" = no ] || dem_fail 'duplicate install action'; dem_install=yes; shift ;;
      --docker-container|--dem-docker-policy-status|--dem-docker-policy-remove)
        [ "$#" -ge 2 ] && [ -n "$2" ] && [ -z "$dem_action" ] || dem_fail 'provide exactly one Docker action with its value'
        case "$1" in
          --docker-container) dem_action=docker-install; dem_container=$2 ;;
          --dem-docker-policy-status) dem_action=policy-status; dem_token=$2 ;;
          --dem-docker-policy-remove) dem_action=policy-remove; dem_token=$2 ;;
        esac
        shift 2 ;;
      --dry-run) dem_dry=yes; shift ;;
      --deployments-stopped) dem_stopped=yes; shift ;;
      --non-interactive) shift ;;
      *) dem_fail 'Docker actions cannot be combined with host installation, claiming, update or other options' ;;
    esac
  done
  case "$dem_action:$dem_install:$dem_stopped" in
    docker-install:yes:no|policy-status:no:no|policy-remove:no:yes) ;;
    *) dem_fail 'container enrollment requires --install-dem-runtime; policy removal requires --deployments-stopped; these are separate terminal actions' ;;
  esac
  for dem_redirect in "${DOCKER_HOST-}" "${DOCKER_CONTEXT-}" "${DOCKER_TLS_VERIFY-}" "${DOCKER_CERT_PATH-}" "${DOCKER_CONFIG-}" "${DOCKER_API_VERSION-}"; do
    [ -z "$dem_redirect" ] || dem_fail 'Docker client overrides are unsupported; use a clean environment on the daemon host'
  done
  [ "$(id -u)" = 0 ] || dem_fail 'run this action as root on the Docker daemon host'
  [ "$(uname -s)" = Linux ] || dem_fail 'Docker host actions require Linux'
  case "$dem_container$dem_token" in ''|*[!a-zA-Z0-9_.-]*) dem_fail 'invalid container name or registration token' ;; esac
  [ "${#dem_container}" -le 128 ] && [ "${#dem_token}" -le 128 ] || dem_fail 'container name or token is too long'
  if [ "$dem_dry:$dem_action" = yes:policy-status ] || [ "$dem_dry:$dem_action" = yes:policy-remove ]; then
    printf 'kickstart Docker setup: dry-run: would run %s for registration %s; no policy was changed\n' "$dem_action" "$dem_token" >&2
    exit 0
  fi
  dem_private=$(mktemp -d /tmp/netdata-dem-kickstart.XXXXXXXX) || exit $?
  trap 'rm -rf -- "$dem_private"' 0
  trap 'exit 1' 1 2 3 15
  dem_embedded_helper > "$dem_private/netdata-dem-runtime.sh" || exit $?
  chmod 600 "$dem_private/netdata-dem-runtime.sh" || exit $?
  if [ "$dem_action" = docker-install ]; then
    set -- docker-install --container "$dem_container"
    [ "$dem_dry" != yes ] || set -- "$@" --dry-run
  else
    set -- "$dem_action" --deployment "$dem_token"
    [ "$dem_stopped" != yes ] || set -- "$@" --deployments-stopped
  fi
  # The child owns cooperative Docker cancellation. Keep the parent alive until
  # it has waited for the browser supervisor rather than deleting its controls.
  env -i PATH="$PATH" /bin/sh "$dem_private/netdata-dem-runtime.sh" "$@" &
  dem_child=$!
  trap 'kill -TERM "$dem_child" 2>/dev/null || : ' 1 2 3 15
  while :; do
    if wait "$dem_child"; then dem_status=0; break; else dem_status=$?; fi
    kill -0 "$dem_child" 2>/dev/null || break
  done
  exit "$dem_status"
}

dem_embedded_helper() {
# BEGIN GENERATED DEM HOST HELPER
cat <<'NETDATA_DEM_HOST_HELPER_EMBEDDED'
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
host_root_owned() {
    host_meta=$(stat -c '%u:%a' "$1") || return $?
    case "$host_meta" in 0:*) ;; *) return 1 ;; esac
    host_mode=${host_meta#*:}
    [ $((0$host_mode & 022)) -eq 0 ]
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
            [ "$host_links" -le 40 ] && [ "$(stat -c '%u' "$host_candidate")" = 0 ] || return 1
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
        case "${host_entry##*/}" in lock|registrations|policies|work.*) ;; *) host_die 'unknown host state entry; inspect before proceeding' ;; esac
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
# Docker targeting stays in the administrator-owned helper; target output is data.
docker_error() { host_error "Docker setup: $*"; }
docker_die() { docker_error "$*"; exit 1; }
docker_call() { env -i PATH="$PATH" HOME="$docker_tmp" "$docker_cli" --host unix:///var/run/docker.sock --config "$docker_tmp" "$@"; }
# Treat target diagnostics as terminal text, never control sequences. ASCII JSON
# remains intact; non-ASCII diagnostics can be inspected through container logs.
docker_safe_text() { LC_ALL=C tr -cd '\011\012\040-\176'; }
docker_capture() {
    # Bound both files at the kernel before target-controlled bytes reach disk.
    # These read-only probes cannot have a runtime supervisor to abandon.
    if (ulimit -f 64 && docker_call "$@" > "$docker_tmp/result" 2> "$docker_tmp/diagnostic"); then docker_capture_rc=0;
    else docker_capture_rc=$?; fi
    head -c 32768 "$docker_tmp/diagnostic" | docker_safe_text >&2
    [ "$docker_capture_rc" -eq 0 ] || return "$docker_capture_rc"
    [ "$(wc -c < "$docker_tmp/result" | tr -d ' ')" -le 32768 ] || docker_die 'oversized daemon/helper response'
    [ "$(tr -d '\000-\010\013-\037\177' < "$docker_tmp/result" | wc -c | tr -d ' ')" = "$(wc -c < "$docker_tmp/result" | tr -d ' ')" ] || docker_die 'control bytes in daemon/helper response'
    cat "$docker_tmp/result"
}
docker_single() {
    docker_single_value=$(docker_capture "$@") || return $?
    [ "$(wc -l < "$docker_tmp/result" | tr -d ' ')" = 1 ] && host_text "$docker_single_value" || docker_die 'expected one bounded response line'
    printf '%s\n' "$docker_single_value"
}
docker_inspect() {
    case "$1" in
        '{{range '*) docker_capture inspect --type container --format "$1" "$docker_id" ;;
        *) docker_single inspect --type container --format "$1" "$docker_id" ;;
    esac
}
docker_identity_check() {
    docker_now=$(docker_inspect '{{.Id}}|{{.Name}}|{{.State.Running}}|{{.State.Pid}}|{{.State.StartedAt}}') || return $?
    [ "$docker_now" = "$docker_start" ] || { docker_error 'container stopped or was replaced; retry the same command for the current deployment'; return 1; }
    docker_named=$(docker_single inspect --type container --format '{{.Id}}' "$docker_name") || return $?
    [ "$docker_named" = "$docker_id" ] || { docker_error 'deployment name now identifies another container; retry the same command'; return 1; }
}
docker_partial() {
    docker_error "state: host-policy=$docker_policy runtime=$docker_runtime target=$docker_id"
    if [ "$docker_policy" != not-required ] && [ "$docker_policy" != not-attempted ]; then
        docker_error "registration-token=$docker_token"
        docker_error "Inspect: sudo sh ./kickstart.sh --dem-docker-policy-status $docker_token"
        docker_error "To retire this registration after its runtime consumers have stopped, run sudo sh ./kickstart.sh --dem-docker-policy-remove $docker_token --deployments-stopped"
        docker_error 'The policy attachment is shared by matching paths across containers; removal preserves other registered owners and never erases a Docker volume.'
    fi
    [ "$docker_runtime" = prepared ] || docker_error "Retry: sudo sh ./kickstart.sh --install-dem-runtime --docker-container $docker_name"
}
docker_mounts() {
    docker_mount_data=$(docker_inspect '{{range .Mounts}}{{printf "%q|%q|%q|%t|%q\n" .Type .Source .Destination .RW .Name}}{{end}}') || return $?
    printf '%s\n' "$docker_mount_data" > "$docker_tmp/mounts"
    docker_cover=no
    while IFS='|' read -r docker_type docker_source docker_dest docker_rw docker_volume docker_extra; do
        [ -n "$docker_type" ] || continue
        [ -z "$docker_extra" ] || docker_die 'unsupported mount metadata'
        for docker_value in "$docker_type" "$docker_source" "$docker_dest" "$docker_volume"; do
            # Go's %q escapes ambiguous names; only plain ASCII path tokens qualify.
            case "$docker_value" in \"*\") ;; *) docker_die 'unsupported mount path' ;; esac
            docker_value=${docker_value#\"}; docker_value=${docker_value%\"}
            case "$docker_value" in *[!a-zA-Z0-9/_.-]*) docker_die 'mount paths must use plain ASCII path components' ;; esac
        done
        docker_type=${docker_type#\"}; docker_type=${docker_type%\"}
        docker_source=${docker_source#\"}; docker_source=${docker_source%\"}
        docker_dest=${docker_dest#\"}; docker_dest=${docker_dest%\"}
        docker_volume=${docker_volume#\"}; docker_volume=${docker_volume%\"}
        host_path "$docker_dest" || docker_die 'unsupported mount destination'
        case "$docker_dest" in
            /usr|/usr/*|/etc|/etc/netdata.stock|/etc/netdata.stock/*|/bin|/bin/*|/sbin|/sbin/*|/lib|/lib/*|/lib64|/lib64/*)
                docker_die 'mount shadows the supported image/configuration owner; use the explicit administrator route' ;;
            /var/lib/netdata/dem/runtime/*) docker_die 'nested runtime mount is unsupported' ;;
            /var|/var/lib) docker_die 'broad runtime ancestor mount is unsupported' ;;
            /var/lib/netdata|/var/lib/netdata/dem|/var/lib/netdata/dem/runtime)
                [ "$docker_cover" = no ] && [ "$docker_rw" = true ] || docker_die 'runtime must have one writable persistent mount without nested shadowing'
                docker_cover=yes
                host_path "$docker_source" && [ -d "$docker_source" ] && [ "$(readlink -f -- "$docker_source")" = "$docker_source" ] || docker_die 'runtime mount source is not a canonical host directory'
                case "$docker_type" in
                    volume)
                        host_token "$docker_volume" || docker_die 'invalid Docker volume name'
                        [ "$docker_source" = "$docker_data_root/volumes/$docker_volume/_data" ] || docker_die 'unexpected Docker volume storage path'
                        docker_volume_info=$(docker_single volume inspect --format '{{.Driver}}|{{.Scope}}|{{json .Options}}' "$docker_volume") || return $?
                        case "$docker_volume_info" in 'local|local|null'|'local|local|{}') ;; *) docker_die 'only ordinary local Docker volumes are supported' ;; esac ;;
                    bind) [ -z "$docker_volume" ] || docker_die 'unexpected bind metadata' ;;
                    *) docker_die 'runtime requires a local persistent volume or bind mount' ;;
                esac
                docker_fs=$(findmnt -n -o FSTYPE -T "$docker_source") || return $?
                case "$docker_fs" in ext2|ext3|ext4|xfs|btrfs|zfs) ;; *) docker_die 'runtime source must reside on a supported local filesystem (ext, xfs, btrfs or zfs)' ;; esac ;;
        esac
    done < "$docker_tmp/mounts"
    [ "$docker_cover" = yes ] || docker_die 'runtime root is ephemeral; mount persistent storage at /var/lib/netdata before retrying'
}
docker_host_policy() {
    docker_policy=not-required
    [ -e "$HOST_APPARMOR_ENABLED" ] || return 0
    docker_enabled=$(cat "$HOST_APPARMOR_ENABLED") || return $?
    case "$docker_enabled" in N) return 0 ;; Y) ;; *) docker_die 'unknown host AppArmor state' ;; esac
    # Kernels without this AppArmor restriction have no corresponding global
    # unconfined-userns gate. Parent profiles were independently verified above.
    [ -e /proc/sys/kernel/apparmor_restrict_unprivileged_userns ] || return 0
    docker_restrict=$(cat /proc/sys/kernel/apparmor_restrict_unprivileged_userns) || docker_die 'unknown host user namespace restriction; use explicit administrator policy setup'
    case "$docker_restrict" in 0) return 0 ;; 1) ;; *) docker_die 'unknown host user namespace restriction' ;; esac
    [ "$(cat /proc/sys/kernel/apparmor_restrict_unprivileged_unconfined)" = 0 ] &&
        [ "$(cat /proc/sys/kernel/unprivileged_userns_clone)" = 1 ] || docker_die 'hardened user namespace settings require explicit administrator setup; global settings were not changed'
    grep -qx 'ID=ubuntu' /etc/os-release && grep -Eq '^VERSION_ID="?24\.04"?$' /etc/os-release || docker_die 'automatic restricted-host policy is qualified for Ubuntu 24.04; use explicit administrator setup'
    docker_policy=needed
}
docker_cancel() {
    docker_cancelled=yes
    # EOF asks the target wrapper to signal its CLI and await verified drainage.
    # Keep the Docker client alive so cancellation never abandons a supervisor.
    exec 8>&-
    docker_error 'cancellation requested; waiting for runtime cleanup and verified browser drainage'
}
docker_install() {
    docker_name=; docker_dry=no
    while [ "$#" -gt 0 ]; do
        case "$1" in
            --container) [ "$#" -ge 2 ] && [ -z "$docker_name" ] || docker_die 'provide one exact container name'; docker_name=$2; shift 2 ;;
            --dry-run) docker_dry=yes; shift ;;
            *) docker_die 'unsupported Docker action option' ;;
        esac
    done
    host_token "$docker_name" || docker_die 'provide a container name of at most 128 ASCII letters, digits, dots, underscores or hyphens'
    case "$docker_name" in [a-zA-Z0-9]*) ;; *) docker_die 'container name must start with a letter or digit' ;; esac
    [ "$(id -u)" = 0 ] || docker_die 'run this terminal action as root on the Docker daemon host'
    [ "$(uname -s)" = Linux ] || docker_die 'only local rootful Docker on Linux is supported'
    for docker_variable in "${DOCKER_HOST-}" "${DOCKER_CONTEXT-}" "${DOCKER_TLS_VERIFY-}" "${DOCKER_CERT_PATH-}" "${DOCKER_CONFIG-}" "${DOCKER_API_VERSION-}"; do
        [ -z "$docker_variable" ] || docker_die 'Docker redirection/client overrides are unsupported; run on the system daemon host in a clean environment'
    done
    [ ! -e /.dockerenv ] && [ ! -e /run/.containerenv ] || docker_die 'nested/containerized host setup is unsupported'
    if [ -e "$HOST_APPARMOR_ENABLED" ] && [ "$(cat "$HOST_APPARMOR_ENABLED")" != N ]; then
        [ "$(cat /proc/self/attr/current 2>/dev/null)" = unconfined ] || docker_die 'the administrator helper requires an unconfined host AppArmor context'
    fi
    host_ancestors /run && [ "$(readlink -f -- /var/run)" = /run ] && [ "$(stat -c %u /var/run)" = 0 ] || docker_die 'unsafe system socket directory'
    [ -S /var/run/docker.sock ] && [ ! -L /var/run/docker.sock ] && [ "$(stat -c %u /var/run/docker.sock)" = 0 ] || docker_die 'a root-owned system Docker Unix socket is required'
    docker_cli=$(host_resolve_executable /usr/bin/docker) || docker_die 'a trusted root-owned /usr/bin/docker CLI is required'
    docker_tmp=$(mktemp -d /run/netdata-dem-docker.XXXXXXXX) || return $?
    trap 'rm -rf -- "$docker_tmp"' 0
    trap 'exit 1' 1 2 3 15
    docker_daemon=$(docker_single info --format '{{.ID}}') || return $?
    host_text "$docker_daemon" && [ "${#docker_daemon}" -le 128 ] || docker_die 'invalid daemon identity'
    case "$docker_daemon" in *[!a-zA-Z0-9:_.-]*) docker_die 'invalid daemon identity' ;; esac
    docker_security=$(docker_single info --format '{{json .SecurityOptions}}|{{.OperatingSystem}}') || return $?
    case "$docker_security" in *rootless*|*userns*|*Desktop*|*desktop*) docker_die 'rootless, remapped or Desktop daemons are unsupported' ;; esac
    docker_data_root=$(docker_single info --format '{{.DockerRootDir}}') || return $?
    host_path "$docker_data_root" && [ -d "$docker_data_root" ] && [ "$(readlink -f -- "$docker_data_root")" = "$docker_data_root" ] || docker_die 'daemon data directory is not local and canonical'
    docker_id=$(docker_single inspect --type container --format '{{.Id}}' "$docker_name") || return $?
    [ "${#docker_id}" = 64 ] || docker_die 'invalid immutable container identity'
    case "$docker_id" in *[!0-9a-f]*) docker_die 'invalid immutable container identity' ;; esac
    docker_start=$(docker_inspect '{{.Id}}|{{.Name}}|{{.State.Running}}|{{.State.Pid}}|{{.State.StartedAt}}') || return $?
    IFS='|' read -r docker_checked_id docker_checked_name docker_running docker_pid docker_started docker_extra <<EOF_ID
$docker_start
EOF_ID
    [ -z "$docker_extra" ] && [ "$docker_checked_id" = "$docker_id" ] && [ "$docker_checked_name" = "/$docker_name" ] && [ "$docker_running" = true ] || docker_die 'the exact named container must be running'
    case "$docker_pid" in ''|*[!0-9]*|0) docker_die 'invalid container task PID' ;; esac
    host_text "$docker_started" || docker_die 'invalid container start identity'
    [ -d "/proc/$docker_pid" ] && grep -Eq "(^|[/:.-])$docker_id([/.:-]|$)" "/proc/$docker_pid/cgroup" || docker_die 'cannot correlate the daemon container with a task on this host; remote/nested daemons are unsupported'
    [ "$(awk '{print $1 ":" $2 ":" $3}' "/proc/$docker_pid/uid_map")" = 0:0:4294967295 ] || docker_die 'user namespace mappings are unsupported'
    docker_config=$(docker_inspect '{{json .Config.Entrypoint}}|{{json .Config.Cmd}}|{{.Path}}|{{json .Args}}|{{.Config.User}}|{{.AppArmorProfile}}|{{.HostConfig.Privileged}}|{{.HostConfig.UsernsMode}}') || return $?
    IFS='|' read -r docker_entry docker_cmd docker_path docker_args docker_user docker_parent docker_privileged docker_userns docker_extra <<EOF_CONFIG
$docker_config
EOF_CONFIG
    [ -z "$docker_extra" ] && [ "$docker_entry" = '["/usr/sbin/run.sh"]' ] &&
        [ "$docker_path" = /usr/sbin/run.sh ] && [ "$docker_args" = '[]' ] &&
        [ "$docker_privileged" = false ] || docker_die 'requires the stock entrypoint with no extra Agent arguments and an ordinary rootful container'
    case "$docker_cmd:$docker_user:$docker_userns" in
        null::|null:root:|null:0:|null:0:0:|\[\]::|\[\]:root:|\[\]:0:|\[\]:0:0:|null::host|\[\]::host) ;;
        *) docker_die 'custom command, startup user or user namespace mode requires the expert route' ;;
    esac
    case "$docker_parent" in
        unconfined) ;;
        '') [ ! -e "$HOST_APPARMOR_ENABLED" ] || [ "$(cat "$HOST_APPARMOR_ENABLED")" = N ] || docker_die 'cannot verify unconfined container AppArmor context' ;;
        *) docker_die 'confined container parent requires explicit administrator setup; automatic setup requires apparmor=unconfined' ;;
    esac
    docker_mounts || return $?
    docker_credentials=$(docker_single exec --user 65534:65534 "$docker_id" /usr/libexec/netdata/netdata-dem-docker identity) || return $?
    case "$docker_credentials" in *:*) ;; *) docker_die 'invalid service identity response' ;; esac
    docker_uid=${docker_credentials%%:*}; docker_gid=${docker_credentials#*:}
    for docker_value in "$docker_uid" "$docker_gid"; do
        case "$docker_value" in ''|*[!0-9]*|0|0*) docker_die 'the resolved service UID and GID must be positive canonical numbers' ;; esac
        [ "${#docker_value}" -le 10 ] && [ "$docker_value" -le 4294967294 ] || docker_die 'service identity is out of range'
    done
    docker_plan=$(docker_capture exec --user "$docker_credentials" "$docker_id" /usr/libexec/netdata/netdata-dem-docker plan) || return $?
    [ "$(wc -l < "$docker_tmp/result" | tr -d ' ')" = 4 ] && [ "$docker_plan" = 'netdata-dem-runtime-setup-v1
standalone
managed
/var/lib/netdata/dem/runtime' ] || docker_die 'container plan is unsupported or root differs from the authorized /var/lib/netdata/dem/runtime'
    docker_token=docker-$(host_hash "netdata-dem-docker-registration-v1:$docker_daemon:$docker_name") || return $?
    docker_runtime=not-attempted; docker_policy=not-attempted
    docker_host_policy || return $?
    docker_identity_check || return $?
    if [ "$docker_dry" = yes ]; then
        docker_error "dry-run: target=$docker_id service=$docker_credentials root=/var/lib/netdata/dem/runtime host-policy=$docker_policy registration-token=$docker_token"
        docker_error 'Would ensure a signed runtime with daily automatic preparation on first enrollment; no host policy or runtime was changed.'
        return 0
    fi
    docker_error 'Ensuring a signed browser runtime; first enrollment permits network acquisition and daily automatic preparation. Existing selection and update consent are preserved.'
    if [ "$docker_policy" = needed ]; then
        docker_policy=attempted
        docker_partial
        if (host_main policy-prepare --deployment "$docker_token" --runtime-root /var/lib/netdata/dem/runtime); then docker_policy=ready;
        else docker_rc=$?; docker_partial; return "$docker_rc"; fi
    elif [ -e "$HOST_STATE/registrations/deployment-$docker_token" ]; then
        # Keep durable ownership visible if the host restriction was later relaxed.
        docker_policy=retained
    fi
    docker_identity_check || { docker_partial; return 1; }
    docker_runtime=attempted
    mkfifo -m 600 "$docker_tmp/control" "$docker_tmp/output" || { docker_partial; return 1; }
    # Drain all output, retaining a bounded report. Closing a pipe on overflow
    # could kill the Docker client and abandon a still-draining supervisor.
    (head -c 65536 > "$docker_tmp/report"; cat > /dev/null) < "$docker_tmp/output" &
    docker_output_pid=$!
    docker_cancelled=no
    trap docker_cancel 1 2 3 15
    docker_call exec -i --user "$docker_credentials" "$docker_id" /usr/libexec/netdata/netdata-dem-docker ensure < "$docker_tmp/control" > "$docker_tmp/output" 2>&1 &
    docker_exec_pid=$!
    exec 8> "$docker_tmp/control"
    [ "$docker_cancelled" != yes ] || exec 8>&-
    # Interrupted wait must resume: only the target supervisor can prove drainage.
    while :; do
        if wait "$docker_exec_pid"; then docker_rc=0; break; else docker_rc=$?; fi
        if kill -0 "$docker_exec_pid" 2>/dev/null; then continue; fi
        break
    done
    exec 8>&-
    wait "$docker_output_pid" || { docker_partial; return 1; }
    docker_safe_text < "$docker_tmp/report"
    if [ "$(wc -c < "$docker_tmp/report" | tr -d ' ')" = 65536 ]; then
        docker_error 'runtime output truncated at 64 KiB; inspect the runtime status inside the container for further diagnostics'
    fi
    trap 'exit 1' 1 2 3 15
    if [ "$docker_rc" -ne 0 ] || [ "$docker_cancelled" = yes ]; then
        docker_runtime=failed-or-cancelled
        docker_partial
        [ "$docker_rc" -ne 0 ] || docker_rc=1
        return "$docker_rc"
    fi
    docker_runtime=prepared
    docker_identity_check || { docker_partial; return 1; }
    docker_partial
    docker_error "Runtime selection prepared. Activation in the running Agent is unverified; restart the deployment explicitly when appropriate: docker restart $docker_name"
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
                    host_register ;;
                policy-status) host_policy_status ;;
                policy-remove) [ "$host_retired" = yes ] || host_die 'policy removal requires --deployments-stopped after every affected deployment is retired'; host_unregister ;;
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
if [ "${1-}" = docker-install ]; then shift; docker_install "$@"; else host_main "$@"; fi
NETDATA_DEM_HOST_HELPER_EMBEDDED
# END GENERATED DEM HOST HELPER
}

# Avoid passing ordinary kickstart arguments through the terminal parser.
case " $* " in
  *' --docker-container '*|*' --dem-docker-policy-status '*|*' --dem-docker-policy-remove '*)
    dem_docker_dispatch "$@"
    exit $?
    ;;
esac

# ======================================================================
# Constants

DEFAULT_RELEASE_CHANNEL="nightly"
KICKSTART_OPTIONS="${*}"
KICKSTART_SOURCE="$(
    self=${0}
    while [ -L "${self}" ]
    do
        cd "${self%/*}" || exit 1
        self=$(readlink "${self}")
    done
    cd "${self%/*}" || exit 1
    echo "$(pwd -P)/${self##*/}"
)"
DEFAULT_PLUGIN_PACKAGES=""
REPOCONFIG_DEB_VERSION="5-5"
REPOCONFIG_RPM_VERSION="5-5"
START_TIME="$(date +%s)"
STATIC_INSTALL_ARCHES="x86_64 armv7l armv6l aarch64"

# ======================================================================
# Properly sort out inconsistencies in `$PATH` across distros
#
# Debian and Ubuntu still don’t include sbin directories for regular
# users (even though the distinction is all but pointless at this point in
# time), and a number of distros don’t include the standard paths under
# `/usr/local` either.

for dir in /usr/sbin /sbin /usr/local/bin /usr/local/sbin ; do
  case ":${PATH}:" in
    *:${dir}:*) ;;
    *) PATH="${PATH}:${dir}" ;;
  esac
done

# ======================================================================
# URLs used throughout the script

AGENT_BUG_REPORT_URL="https://github.com/netdata/netdata/issues/new/choose"
CLOUD_BUG_REPORT_URL="https://github.com/netdata/netdata-cloud/issues/new/choose"
DISCORD_INVITE="https://discord.gg/5ygS846fR6"
DISCUSSIONS_URL="https://github.com/netdata/netdata/discussions"
DOCS_URL="https://learn.netdata.cloud/docs/"
FORUM_URL="https://community.netdata.cloud/"
INSTALL_DOC_URL="https://learn.netdata.cloud/docs/install-the-netdata-agent/one-line-installer-for-all-linux-systems"
PACKAGES_SCRIPT="https://raw.githubusercontent.com/netdata/netdata/master/packaging/installer/install-required-packages.sh"
PUBLIC_CLOUD_URL="https://app.netdata.cloud"
RELEASE_INFO_URL="https://repo.netdata.cloud/releases"
STABLE_REPO_URL_PREFIX="https://repository.netdata.cloud/repos/stable"
NIGHTLY_REPO_URL_PREFIX="https://repository.netdata.cloud/repos/edge"
REPOCONFIG_URL_PREFIX="https://repository.netdata.cloud/repos/repoconfig"
TELEMETRY_URL="https://us-east1-netdata-analytics-bi.cloudfunctions.net/ingest_agent_events"
RELEASE_ARTIFACT_URL_PREFIX="https://artifacts.netdata.cloud"

# ======================================================================
# Defaults for environment variables

DRY_RUN=0
SELECTED_INSTALL_METHOD="none"
INSTALL_TYPE="unknown"
INSTALL_PREFIX=""
NETDATA_AUTO_UPDATES="default"
NETDATA_INSTALL_DEM_RUNTIME=0
NETDATA_CLAIM_URL="https://app.netdata.cloud"
NETDATA_COMMAND="default"
NETDATA_INSTALLER_OPTIONS=""
NETDATA_REQUESTED_INSTALL_TYPE="auto"
NETDATA_OFFLINE_INSTALL_SOURCE=""
NETDATA_WARNINGS=""
RELEASE_CHANNEL="default"

if [ -n "$DISABLE_TELEMETRY" ]; then
  NETDATA_DISABLE_TELEMETRY="${DISABLE_TELEMETRY}"
elif [ -n "$DO_NOT_TRACK" ]; then
  NETDATA_DISABLE_TELEMETRY="${DO_NOT_TRACK}"
else
  NETDATA_DISABLE_TELEMETRY=0
fi

NETDATA_TARBALL_BASEURL="${NETDATA_TARBALL_BASEURL:-https://github.com/netdata/netdata-nightlies/releases}"

if echo "${0}" | grep -q 'kickstart-static64'; then
  NETDATA_REQUESTED_INSTALL_TYPE='static'
fi

if [ ! -t 1 ]; then
  INTERACTIVE=0
else
  INTERACTIVE=1
fi

CURL="$(PATH="${PATH}:/opt/netdata/bin" command -v curl 2>/dev/null && true)"
WGET="$(command -v wget 2>/dev/null && true)"
[ -z "${WGET}" ] && WGET="$(command -v wget2 2>/dev/null && true)"

# ======================================================================
# Shared messages used in multiple places throughout the script.

BADCACHE_MSG="Usually this is a result of an older copy of the file being cached somewhere upstream and can be resolved by retrying in an hour"
BADNET_MSG="This is usually a result of a networking issue"
ERROR_F0003="Could not find a usable HTTP client. Either curl or wget is required to proceed with installation."
BADOPT_MSG="If you are following a third-party guide online, please see ${INSTALL_DOC_URL} for current instructions for using this script. If you are using a local copy of this script instead of fetching it from our servers, consider updating it. If you intended to pass this option to the installer code, please use either --local-build-options or --static-install-options to specify it instead."
GITHUB_BADNET_MSG="This usually means there is some networking issue preventing access to https://github.com/ from this system."
RELEASE_BADNET_MSG="This usually means there is some networking issue preventing access to ${RELEASE_ARTIFACT_URL_PREFIX} from this system."

# ======================================================================
# Core program logic

main() {
  case "${ACTION}" in
    uninstall)
      uninstall
      printf >&2 "Finished uninstalling the Netdata Agent."
      deferred_warnings
      cleanup
      trap - EXIT
      exit 0
      ;;
    reinstall-clean)
      NEW_INSTALL_PREFIX="${INSTALL_PREFIX}"
      uninstall
      cleanup

      ACTION=''
      INSTALL_PREFIX="${NEW_INSTALL_PREFIX}"
      # shellcheck disable=SC2086
      main

      trap - EXIT
      exit 0
      ;;
    prepare-offline)
      prepare_offline_install_source "${OFFLINE_TARGET}"
      deferred_warnings
      trap - EXIT
      exit 0
      ;;
  esac

  handle_existing_install
  set_tmpdir

  if [ -n "${INSTALL_VERSION}" ]; then
    if echo "${INSTALL_VERSION}" | grep -E -o "^[[:digit:]]+\.[[:digit:]]+\.[[:digit:]]+$" > /dev/null 2>&1; then
      NEW_SELECTED_RELEASE_CHANNEL="stable"
    else
      NEW_SELECTED_RELEASE_CHANNEL="nightly"
    fi

    if ! [ "${NEW_SELECTED_RELEASE_CHANNEL}" = "${SELECTED_RELEASE_CHANNEL}" ]; then
        warning "Selected release channel does not match this version and it will be changed automatically."
        SELECTED_RELEASE_CHANNEL="${NEW_SELECTED_RELEASE_CHANNEL}"
    fi
  fi

  case "${SYSTYPE}" in
    Linux) install_on_linux ;;
    Darwin) install_on_macos ;;
    FreeBSD) install_on_freebsd ;;
  esac

  if [ -n "${NETDATA_CLAIM_TOKEN}" ]; then
    claim
  fi

  set_auto_updates
  install_dem_runtime

  printf >&2 "%s\n\n" "Successfully installed the Netdata Agent."
  deferred_warnings
  success_banner
  telemetry_event INSTALL_SUCCESS "" ""
  cleanup
  trap - EXIT
}

# ======================================================================
# Usage info

usage() {
  cat << HEREDOC
USAGE: kickstart.sh [options]
       where options include:

  --non-interactive                Do not prompt for user input. (default: prompt if there is a controlling terminal)
  --interactive                    Prompt for user input even if there is no controlling terminal.
  --dont-start-it                  Do not start the agent by default (only for static installs or local builds)
  --docker-container NAME         Target a running local Docker deployment (with --install-dem-runtime).
  --dem-docker-policy-status TOKEN Inspect a Docker host policy registration without a container.
  --dem-docker-policy-remove TOKEN Retire that registration; requires --deployments-stopped.
  --install-dem-runtime            Explicitly install the optional browser runtime for DEM synthetic monitoring.
  --dry-run                        Report what we would do with the given options on this system, but don’t actually do anything.
  --release-channel                Specify the release channel to use for the install (default: ${DEFAULT_RELEASE_CHANNEL})
  --stable-channel                 Equivalent to "--release-channel stable"
  --nightly-channel                Equivalent to "--release-channel nightly"
  --no-updates                     Do not enable automatic updates (default: enable automatic updates using the best supported scheduling method)
  --auto-update                    Enable automatic updates.
  --auto-update-type               Specify a particular scheduling type for auto-updates (valid types: systemd, interval, crontab)
  --disable-telemetry              Opt-out of anonymous statistics.
  --native-only                    Only install if native binary packages are available.
  --static-only                    Only install if a static build is available.
  --build-only                     Only install using a local build.
  --install-type <type>            Specify what installation type to use (native, static, build, auto, or any).
  --install-prefix <path>          Specify an installation prefix for local builds (default: autodetect based on system type).
  --old-install-prefix <path>      Specify an old local builds installation prefix for uninstall/reinstall (if it's not default).
  --install-version <version>      Specify the version of Netdata to install.
  --claim-token                    Use a specified token for claiming to Netdata Cloud.
  --claim-rooms                    When claiming, add the node to the specified rooms.
  --claim-*                        Specify other options for the claiming script.
  --no-cleanup                     Don't do any cleanup steps. This is intended to help with debugging the installer.
  --local-build-options            Specify additional options to pass to the installer code when building locally. Only valid if --build-only is also specified.
  --static-install-options         Specify additional options to pass to the static installer code. Only valid if --static-only is also specified.
  --offline-architecture           Limit an offline install source being prepared with --prepare-offline-install-source to only include the specified static build architecture.
  --offline-install-source         Specify a folder to use as installation source when working offline. Use --prepare-offline-install-source first to set up this folder.

--install-dem-runtime uses the installed Agent's normal configuration. Custom service
-c/-u arguments or environment overrides require direct runtime CLI setup after
administrator preparation of any required host policy. Native
runtime packages require the named netdata service account. Managed setup downloads
the browser archive and enables daily preparation; restart the Agent to activate it.

The following options are mutually exclusive and specifiy special operations other than trying to install Netdata normally or update an existing install:

  --reinstall                      If there is an existing install, reinstall it instead of trying to update it. If there is no existing install, install netdata normally.
  --reinstall-even-if-unsafe       If there is an existing install, reinstall it instead of trying to update it, even if doing so is known to potentially break things. If there is no existing install, install Netdata normally.
  --reinstall-clean                If there is an existing install, uninstall it before trying to install Netdata. Fails if there is no existing install.
  --uninstall                      Uninstall an existing installation of Netdata. Fails if there is no existing install.
  --claim-only                     If there is an existing install, only try to claim it without attempting to update it. If there is no existing install, install and claim Netdata normally.
  --repositories-only              Only install repository configuration packages instead of doing a full install of Netdata. Automatically sets --native-only.
  --prepare-offline-install-source Instead of installing the agent, prepare a directory that can be used to install on another system without needing to download anything.

Additionally, this script may use the following environment variables:

  TMPDIR:                    Used to specify where to put temporary files. On most systems, the default we select
                             automatically should be fine. The user running the script needs to both be able to
                             write files to the temporary directory, and run files from that location.
  ROOTCMD:                   Used to specify a command to use to run another command with root privileges if needed. By
                             default we try to use sudo, doas, or pkexec (in that order of preference), but if
                             you need special options for one of those to work, or have a different tool to do
                             the same thing on your system, you can specify it here.
  DISABLE_TELEMETRY          If set to a value other than 0, behave as if \`--disable-telemetry\` was specified.

HEREDOC
}

# ======================================================================
# Telemetry functions

telemetry_event() {
  if [ "${NETDATA_DISABLE_TELEMETRY}" -eq 1 ] || [ "${DRY_RUN}" -eq 1 ]; then
    return 0
  fi

  now="$(date +%s)"
  total_duration="$((now - START_TIME))"

  if [ -e "/etc/os-release" ]; then
    eval "$(grep -E "^(NAME|ID|ID_LIKE|VERSION|VERSION_ID)=" < /etc/os-release | sed 's/^/HOST_/')"
  fi

  if [ -z "${HOST_NAME}" ] || [ -z "${HOST_VERSION}" ] || [ -z "${HOST_ID}" ]; then
    if [ -f "/etc/lsb-release" ]; then
      DISTRIB_ID="unknown"
      DISTRIB_RELEASE="unknown"
      DISTRIB_CODENAME="unknown"
      eval "$(grep -E "^(DISTRIB_ID|DISTRIB_RELEASE|DISTRIB_CODENAME)=" < /etc/lsb-release)"
      if [ -z "${HOST_NAME}" ]; then HOST_NAME="${DISTRIB_ID}"; fi
      if [ -z "${HOST_VERSION}" ]; then HOST_VERSION="${DISTRIB_RELEASE}"; fi
      if [ -z "${HOST_ID}" ]; then HOST_ID="${DISTRIB_CODENAME}"; fi
    fi
  fi

  KERNEL_NAME="$(uname -s)"

  if [ "${KERNEL_NAME}" = FreeBSD ]; then
    TOTAL_RAM="$(sysctl -n hw.physmem)"
  elif [ "${KERNEL_NAME}" = Darwin ]; then
    TOTAL_RAM="$(sysctl -n hw.memsize)"
  elif [ -r /proc/meminfo ]; then
    TOTAL_RAM="$(grep -F MemTotal /proc/meminfo | cut -f 2 -d ':' | sed -e 's/^[[:space:]]*//' -e 's/[[:space:]]*$//' | cut -f 1 -d ' ')"
    TOTAL_RAM="$((TOTAL_RAM * 1024))"
  fi

  MD5_PATH="$(exec <&- 2>&-; which md5sum || command -v md5sum || type md5sum)"

  if [ "${KERNEL_NAME}" = Darwin ] && command -v ioreg >/dev/null 2>&1; then
    DISTINCT_ID="macos-$(ioreg -rd1 -c IOPlatformExpertDevice | awk '/IOPlatformUUID/ { split($0, line, "\""); printf("%s\n", line[4]); }')"
  elif [ -f /etc/machine-id ] && [ -n "$MD5_PATH" ]; then
    DISTINCT_ID="machine-$($MD5_PATH < /etc/machine-id | cut -f1 -d" ")"
  elif [ -f /var/db/dbus/machine-id ] && [ -n "$MD5_PATH" ]; then
    DISTINCT_ID="dbus-$($MD5_PATH < /var/db/dbus/machine-id | cut -f1 -d" ")"
  elif [ -f /var/lib/dbus/machine-id ] && [ -n "$MD5_PATH" ]; then
    DISTINCT_ID="dbus-$($MD5_PATH < /var/lib/dbus/machine-id | cut -f1 -d" ")"
  elif command -v uuidgen > /dev/null 2>&1; then
    DISTINCT_ID="uuid-$(uuidgen | tr '[:upper:]' '[:lower:]')"
  else
    DISTINCT_ID="null"
  fi

  REQ_BODY="$(cat << EOF
{
  "event": "${1}",
  "properties": {
    "distinct_id": "${DISTINCT_ID}",
    "event_source": "agent installer",
    "\$current_url": "agent installer",
    "\$pathname": "netdata-installer",
    "\$host": "installer.netdata.io",
    "\$ip": "127.0.0.1",
    "script_variant": "kickstart-ng",
    "error_code": "${3}",
    "error_message": "${2}",
    "install_options": "${KICKSTART_OPTIONS}",
    "install_interactivity": "${INTERACTIVE}",
    "install_auto_updates": "${NETDATA_AUTO_UPDATES}",
    "install_command": "${NETDATA_COMMAND}",
    "total_runtime": "${total_duration}",
    "selected_install_method": "${SELECTED_INSTALL_METHOD}",
    "netdata_release_channel": "${RELEASE_CHANNEL:-null}",
    "netdata_install_type": "${INSTALL_TYPE}",
    "host_os_name": "${HOST_NAME:-unknown}",
    "host_os_id": "${HOST_ID:-unknown}",
    "host_os_id_like": "${HOST_ID_LIKE:-unknown}",
    "host_os_version": "${HOST_VERSION:-unknown}",
    "host_os_version_id": "${HOST_VERSION_ID:-unknown}",
    "system_kernel_name": "${KERNEL_NAME}",
    "system_kernel_version": "$(uname -r)",
    "system_architecture": "$(uname -m)",
    "system_total_ram": "${TOTAL_RAM:-unknown}",
    "system_distinct_id": "${DISTINCT_ID}"
  }
}
EOF
)"

  succeeded=0

  if [ -n "${CURL}" ]; then
    if "${CURL}" --silent -o /dev/null -X POST --max-time 2 --header "Content-Type: application/json" -d "${REQ_BODY}" "${TELEMETRY_URL}" > /dev/null; then
      succeeded=1
    fi
  fi

  if [ "${succeeded}" -eq 0 ]; then
    if [ -n "${WGET}" ]; then
      if "${WGET}" --help 2>&1 | grep BusyBox > /dev/null 2>&1; then
        # BusyBox-compatible version of wget, there is no --no-check-certificate option
        "${WGET}" -q -O - \
        -T 1 \
        --header 'Content-Type: application/json' \
        --post-data "${REQ_BODY}" \
        "${TELEMETRY_URL}" > /dev/null
      else
        "${WGET}" -q -O - --no-check-certificate \
        --method POST \
        --timeout=1 \
        --header 'Content-Type: application/json' \
        --body-data "${REQ_BODY}" \
        "${TELEMETRY_URL}" > /dev/null
      fi
    fi
  fi
}

trap_handler() {
  code="${1}"
  lineno="${2}"

  deferred_warnings

  printf >&2 "%s\n\n" "${TPUT_BGRED}${TPUT_WHITE}${TPUT_BOLD} ERROR ${TPUT_RESET} Installer exited unexpectedly (${code}-${lineno})"

  case "${code}" in
    0) printf >&2 "%s\n" "This is almost certainly the result of a bug. If you have time, please report it at ${AGENT_BUG_REPORT_URL}." ;;
    *)
      printf >&2 "%s\n" "This is probably a result of a transient issue on your system. Things should work correctly if you try again."
      printf >&2 "%s\n" "If you continue to experience this issue, you can reach out to us for support on:"
      support_list
      ;;
  esac

  telemetry_event INSTALL_CRASH "Installer exited unexpectedly (${code}-${lineno})" "E${code}-${lineno}"

  trap - EXIT

  cleanup

  exit 1
}

trap 'trap_handler 0 ${LINENO}' EXIT
trap 'trap_handler 1 0' HUP
trap 'trap_handler 2 0' INT
trap 'trap_handler 3 0' QUIT
trap 'trap_handler 13 0' PIPE
trap 'trap_handler 15 0' TERM

# ======================================================================
# Utility functions

# Check if a URL is valid, and ensure it uses one of the specified protocols.
is_valid_url() {
  printf "%s\n" "${1}" | grep -qE "^(${2})://([^@\/:[:space:]]+(:[^@\/:[:space:]]*)?@)?(\[[0-9A-Fa-f:]+\]|[A-Za-z0-9.-]+)(:[0-9]{1,5})?(/[-A-Za-z0-9._~:/?#\[\]@!$&'()*+,;=]*)?$" || return 1
}

sanitize_string() {
  v="${1}"
  r="$(printf '%s\n' "${1}" | tr -cd "[:alnum:] ._=-")"
  [ "${v}" = "${r}" ] || warning "Unsafe characters found in string, sanitized to ${r}"
  echo "${r}"
}

sanitize_path() {
  v="${1}"
  _path_unsafe=";&'\"|\\<>()\n\$*?\`{}[]"
  if [ -z "${_path_replace}" ]; then
    _path_replace="$(printf "%$(printf "%s" "${_path_unsafe}" | wc -m)s" | tr " " "_")"
  fi
  r="$(printf '%s' "$1" | tr "${_path_unsafe}" "${_path_replace}")"
  [ "${v}" = "${r}" ] || warning "Unsafe characters found in path, sanitized to ${r}"
  echo "${r}"
}

canonical_path() {
  OLDPWD="$(pwd)"
  cd "$(dirname "${1}")" || exit 1
  case "$(basename "${1}")" in
    ..) dirname "$(pwd -P)" ;;
    .) pwd -P ;;
    *) echo "$(pwd -P)/$(basename "${1}")" ;;
  esac
  cd "${OLDPWD}" || exit 1
}

setup_terminal() {
  TPUT_RESET=""
  TPUT_WHITE=""
  TPUT_BGRED=""
  TPUT_BGGREEN=""
  TPUT_BOLD=""
  TPUT_DIM=""

  case "${COLUMNS}" in
    "" | *[!0-9]*) TERM_WIDTH=80 ;;
    *)
      if [ "${COLUMNS}" -ge 0 ]; then
        TERM_WIDTH="${COLUMNS}"
      else
        TERM_WIDTH="80"
      fi
      ;;
  esac

  # Is stderr on the terminal? If not, then fail
  test -t 2 || return 1

  if command -v tput > /dev/null 2>&1; then
    if num_colors=$(tput colors 2> /dev/null) && [ "${num_colors:-0}" -ge 8 ]; then
      # Enable colors
      TPUT_RESET="$(tput sgr 0)"
      TPUT_WHITE="$(tput setaf 7)"
      TPUT_BGRED="$(tput setab 1)"
      TPUT_BGGREEN="$(tput setab 2)"
      TPUT_BOLD="$(tput bold)"
      TPUT_DIM="$(tput dim)"
    fi
  fi

  echo "${TPUT_RESET}"

  return 0
}

support_list() {
  printf >&2 "%s\n" "  - GitHub: ${DISCUSSIONS_URL}"
  printf >&2 "%s\n" "  - Discord: ${DISCORD_INVITE}"
  printf >&2 "%s\n" "  - Our community forums: ${FORUM_URL}"
}

success_banner() {
  printf >&2 "%s\n" "To view the dashboard, open your web browser and enter http://NODE:19999."
  printf >&2 "%s\n\n" "Replace NODE with the IP address or hostname of your Netdata server."

  printf >&2 "%s\n\n" "Official documentation can be found online at ${DOCS_URL}."

  if [ -z "${CLAIM_TOKEN}" ]; then
    printf >&2 "%s\n\n" "Looking to monitor all of your infrastructure with Netdata? Check out Netdata Cloud at ${PUBLIC_CLOUD_URL}."
  fi

  printf >&2 "%s\n" "Join our community and connect with us on:"
  support_list
}

cleanup() {
  if [ -z "${NO_CLEANUP}" ] && [ -n "${tmpdir}" ]; then
    cd || true
    DRY_RUN=0
    run_as_root_silent rm -rf "${tmpdir}"
  fi
}

deferred_warnings() {
  if [ -n "${NETDATA_WARNINGS}" ]; then
    printf >&2 "%s\n" "The following non-fatal warnings or errors were encountered:"
    # shellcheck disable=SC2059
    printf >&2 "${NETDATA_WARNINGS}"
    printf >&2 "\n\n"
  fi
}

fatal() {
  bracket="${TPUT_BGRED}${TPUT_WHITE}${TPUT_BOLD}$(printf "%${TERM_WIDTH}s" " " | tr " " "X")${TPUT_RESET}"
  printf >&2 "%s\n\n" "${bracket}"
  deferred_warnings
  printf >&2 "%s\n" "${bracket}"
  printf >&2 "%s\n" "${TPUT_BOLD} A FATAL ERROR WAS ENCOUNTERED! ${TPUT_RESET}"
  printf >&2 "%s\n" "${TPUT_BOLD} \\/ \\/ \\/ \\/ \\/ \\/ \\/ \\/ \\/ \\/ ${TPUT_RESET}"
  printf >&2 "\n%b\n\n" "${TPUT_BGRED}${TPUT_WHITE}${TPUT_BOLD} ABORTED ${TPUT_RESET} ${1}"
  printf >&2 "%s\n" "${TPUT_BOLD} /\\ /\\ /\\ /\\ /\\ /\\ /\\ /\\ /\\ /\\ ${TPUT_RESET}"
  printf >&2 "%s\n" "${TPUT_BOLD} A FATAL ERROR WAS ENCOUNTERED! ${TPUT_RESET}"
  printf >&2 "%s\n" "${bracket}"
  printf >&2 "%s\n" "For community support, you can connect with us on:"
  support_list
  telemetry_event "INSTALL_FAILED" "${1}" "${2}"
  cleanup
  trap - EXIT
  exit 1
}

ESCAPED_PRINT_METHOD=
# shellcheck disable=SC3050
if printf "%s " test > /dev/null 2>&1; then
  ESCAPED_PRINT_METHOD="printfq"
fi

escaped_print() {
  if [ "${ESCAPED_PRINT_METHOD}" = "printfq" ]; then
    # shellcheck disable=SC3050
    printf "%s " "${@}"
  else
    printf "%s" "${*}"
  fi
  return 0
}

progress() {
  echo >&2 " --- ${TPUT_BOLD}${*}${TPUT_RESET} --- "
}

run_logfile="/dev/null"
_get_run_info() {
  user="${USER--}"
  dir="${PWD}"

  if [ "$(id -u)" = "0" ]; then
    info="[root ${dir}]# "
    info_console="[${TPUT_DIM}${dir}${TPUT_RESET}]# "
  else
    info="[${user} ${dir}]$ "
    info_console="[${TPUT_DIM}${dir}${TPUT_RESET}]$ "
  fi
}

_log_run_cmd() {
  {
    printf "%s" "${info}"
    escaped_print "${@}"
    printf " ... "
  } >> "${run_logfile}"

  printf >&2 "%s" "${info_console}${TPUT_BOLD}"
  escaped_print >&2 "${@}"
  printf >&2 "%s\n" "${TPUT_RESET}"
}

_log_run_result() {
  ret="${1}"
  shift 1
  if [ "${ret}" -ne 0 ]; then
    printf >&2 "%s\n\n" "${TPUT_BGRED}${TPUT_WHITE}${TPUT_BOLD} FAILED ${TPUT_RESET}"
    printf "%s\n" "FAILED with exit code ${ret}" >> "${run_logfile}"
    # shellcheck disable=SC2089
    NETDATA_WARNINGS="${NETDATA_WARNINGS}\n  - Command \"${*}\" failed with exit code ${ret}."
  else
    printf >&2 "%s\n\n" "${TPUT_BGGREEN}${TPUT_WHITE}${TPUT_BOLD} OK ${TPUT_RESET}"
    printf "OK\n" >> "${run_logfile}"
  fi
}

run() {
  _get_run_info

  if [ "${DRY_RUN}" -eq 1 ]; then
    printf >&2 "%s\n" "Would run command:"
  fi

  _log_run_cmd "${@}"

  if [ "${DRY_RUN}" -ne 1 ]; then
    "${@}"
    ret=$?
  else
    ret=0
  fi

  _log_run_result "${ret}" "${@}"

  return "${ret}"
}

always_run() {
  _get_run_info
  _log_run_cmd "${@}"
  "${@}"
  ret="$?"
  _log_run_result "${ret}" "${@}"
  return "${ret}"
}

run_as_root() {
  confirm_root_support

  if [ "$(id -u)" -ne "0" ]; then
    printf >&2 "Root privileges required to run %s\n" "${*}"
  fi

  run ${ROOTCMD} "${@}"
}

# Only to be used in cleanup code.
run_as_root_silent() {
  confirm_root_support

  if [ "$(id -u)" -ne "0" ]; then
    printf >&2 "Root privileges required to run %s\n" "${*}"
  fi

  ${ROOTCMD} "${@}"
}

run_script() {
  old_pwd="${PWD}"
  set_tmpdir

  export NETDATA_SCRIPT_STATUS_PATH="${tmpdir}/.script-status"

  export NETDATA_SAVE_WARNINGS=1
  export NETDATA_PROPAGATE_WARNINGS=1
  # shellcheck disable=SC2090
  export NETDATA_WARNINGS="${NETDATA_WARNINGS}"

  # shellcheck disable=SC2086
  run ${ROOTCMD} "${@}"

  ret="$?"

  if [ -r "${NETDATA_SCRIPT_STATUS_PATH}" ]; then
    # shellcheck disable=SC1090
    . "${NETDATA_SCRIPT_STATUS_PATH}"
    rm -f "${NETDATA_SCRIPT_STATUS_PATH}"
  fi

  cd "${old_pwd}" || fatal "Failed to change current working directory to ${old_pwd}." F000A
  return "${ret}"
}

warning() {
  bracket="${TPUT_BGRED}${TPUT_WHITE}${TPUT_BOLD}$(printf "%${TERM_WIDTH}s" " " | tr " " "=")${TPUT_RESET}"
  msg="${TPUT_BGRED}${TPUT_WHITE}${TPUT_BOLD} WARNING ${TPUT_RESET} ${*}"
  printf >&2 "%s\n%s\n%s\n" "${bracket}" "${msg}" "${bracket}"
  NETDATA_WARNINGS="${NETDATA_WARNINGS}\n  - ${*}"
}

_cannot_use_tmpdir() {
  testfile="$(TMPDIR="${1}" mktemp -q -t netdata-test.XXXXXXXXXX)"
  ret=0

  if [ -z "${testfile}" ]; then
    return "${ret}"
  fi

  if printf '#!/bin/sh\necho SUCCESS\n' > "${testfile}"; then
    if chmod +x "${testfile}"; then
      if [ "$("${testfile}")" = "SUCCESS" ]; then
        ret=1
      fi
    fi
  fi

  rm -f "${testfile}"
  return "${ret}"
}

create_tmp_directory() {
  if [ -z "${TMPDIR}" ] || _cannot_use_tmpdir "${TMPDIR}"; then
    if _cannot_use_tmpdir /tmp; then
      if _cannot_use_tmpdir "${PWD}"; then
        fatal "Unable to find a usable temporary directory. Please set \$TMPDIR to a path that is both writable and allows execution of files and try again." F0400
      else
        TMPDIR="${PWD}"
      fi
    else
      TMPDIR="/tmp"
    fi
  fi

  mktemp -d -t netdata-kickstart-XXXXXXXXXX
}

set_tmpdir() {
  if [ -z "${tmpdir}" ] || [ ! -d "${tmpdir}" ]; then
    tmpdir="$(create_tmp_directory)"
    progress "Using ${tmpdir} as a temporary directory."
    cd "${tmpdir}" || fatal "Failed to change current working directory to ${tmpdir}." F000A
  fi
}

handle_http_status() {
  status="${1}"
  url="${2}"
  action="${3}"

  warn_msg="Server returned ${status} when ${action} ${url}"
  case "${status}" in
    404)
      warning "File not found when ${action} ${url}"
      return 1
      ;;
    4*)
      warning "${warn_msg}"
      return 5
      ;;
    5*)
      warning "${warn_msg}"
      return 6
      ;;
    *)
      warning "${warn_msg}"
      return 4
      ;;
  esac
}

handle_curl_result() {
  ret="${1}"
  url="${2}"
  dl_log="${3}"
  action="${4}"
  dest="${5}"

  case "${ret}" in
    0) return 0 ;;
    22|78)
      status="$(tail -n 1 "${dl_log}")"
      [ -n "${dest}" ] && rm -f "${dest}"
      handle_http_status "${status}" "${url}" "${action}"
      return "$?"
      ;;
    56)
      status="$(tail -n 1 "${dl_log}")"
      [ -n "${dest}" ] && rm -f "${dest}"
      case "${status}" in
        2*)
          warning "Protocol error when ${action} ${url}"
          return 4
          ;;
        *)
          handle_http_status "${status}" "${url}" "${action}"
          return "$?"
          ;;
      esac
      ;;
    5|6|7)
      [ -n "${dest}" ] && rm -f "${dest}"
      warning "Failed to connect to remote host when ${action} ${url}"
      return 2
      ;;
    35|60|83)
      [ -n "${dest}" ] && rm -f "${dest}"
      warning "TLS error while connecting to remote host when ${action} ${url}"
      return 3
      ;;
    *)
      [ -n "${dest}" ] && rm -f "${dest}"
      fatal "Unknown error when ${action} ${url}" F0520
      ;;
  esac
}

handle_wget_result() {
  ret="${1}"
  url="${2}"
  dl_log="${3}"
  action="${4}"
  dest="${5}"

  case "${ret}" in
    0) return 0 ;;
    8)
      status="$(grep "HTTP/" "${dl_log}" | tail -n 1 | awk '{ print $2 }')"
      [ -n "${dest}" ] && rm -f "${dest}"
      handle_http_status "${status}" "${url}" "${action}"
      return "$?"
      ;;
    4)
      [ -n "${dest}" ] && rm -f "${dest}"
      warning "Failed to connect to remote host when ${action} ${url}"
      return 2
      ;;
    5)
      [ -n "${dest}" ] && rm -f "${dest}"
      warning "TLS error while connecting to remote host when ${action} ${url}"
      return 3
      ;;
    *)
      [ -n "${dest}" ] && rm -f "${dest}"
      fatal "Unknown error when ${action} ${url}" F0520
      ;;
  esac
}

# It’s unlikely that any caller of this function will care why exactly it
# wasn’t possible to retrieve the requested URL, but it’s still useful
# to log the differentiated warnings based on the actual result, so we do
# still have the full logic for inspecting the return codes of curl/wget.
check_for_remote_file() {
  url="${1}"

  old_pwd="${PWD}"
  set_tmpdir
  cd "${old_pwd}" || fatal "Failed to change current working directory to ${old_pwd}." F000A
  dl_log="${tmpdir}/download.log"
  rm -f "${dl_log}"

  if echo "${url}" | grep -Eq "^file:///"; then
    [ -e "${url#file://}" ] || return 1
    return 0
  elif [ -n "${NETDATA_ASSUME_REMOTE_FILES_ARE_PRESENT}" ]; then
    return 0
  fi

  if [ -n "${CURL}" ]; then
    always_run "${CURL}" --write-out "%{http_code}" --output /dev/null --silent --head --fail "${url}" > "${dl_log}"

    handle_curl_result "$?" "${url}" "${dl_log}" "checking for remote file at"
    return "$?"
  fi

  if [ -n "${WGET}" ]; then
    always_run "${WGET}" -S -o "${dl_log}" --spider "${url}"

    handle_wget_result "$?" "${url}" "${dl_log}" "checking for remote file at"
    return "$?"
  fi

  fatal "${ERROR_F0003}" F0003
}

download() {
  url="${1}"
  dest="${2}"
  force="${3}"

  old_pwd="${PWD}"
  set_tmpdir
  cd "${old_pwd}" || fatal "Failed to change current working directory to ${old_pwd}." F000A
  dl_log="${tmpdir}/download.log"
  rm -f "${dl_log}"

  if [ "${force}" = "force" ]; then
    runcmd="always_run"
  else
    runcmd="run"
  fi

  if echo "${url}" | grep -Eq "^file:///"; then
    "${runcmd}" cp "${url#file://}" "${dest}" || return 1
    return 0
  fi

  if [ -n "${CURL}" ]; then
    "${runcmd}" sh -c '"${1}" --fail -q -sSL --connect-timeout 10 --retry 3 --write-out "%{http_code}" --output "${2}" "${3}" > "${4}"' _ "${CURL}" "${dest}" "${url}" "${dl_log}"

    handle_curl_result "$?" "${url}" "${dl_log}" "downloading" "${dest}"
    return "$?"
  fi

  if [ -n "${WGET}" ]; then
    "${runcmd}" "${WGET}" -S -T 15 -o "${dl_log}" -O "${dest}" "${url}"

    handle_wget_result "$?" "${url}" "${dl_log}" "downloading" "${dest}"
    return "$?"
  fi

  fatal "${ERROR_F0003}" F0003
}

get_actual_version() {
    major="${1}"
    channel="${2}"
    url="${RELEASE_INFO_URL}/${channel}/${major}"
    old_pwd="${PWD}"
    set_tmpdir
    cd "${old_pwd}" || true
    tmp_file="${tmpdir}/version-info"

    if check_for_remote_file "${RELEASE_INFO_URL}"; then
        if check_for_remote_file "${url}"; then
            download "${url}" "${tmp_file}"
            cat "${tmp_file}"
        else
            echo "NONE"
        fi
    else
        echo ""
    fi
}

get_redirect() {
  url="${1}"
  old_pwd="${PWD}"
  set_tmpdir
  cd "${old_pwd}" || fatal "Failed to change current working directory to ${old_pwd}." F000A
  output="${tmpdir}/download.log"
  rm -f "${output}"

  if [ -n "${CURL}" ]; then
    always_run sh -c '"${1}" "${2}" -s -L -I -o /dev/null -w "%{url_effective}" > "${3}"' _ "${CURL}" "${url}" "${output}"

    ret="$?"

    case "${ret}" in
      0)
        grep -Eo '[^/]+/?$' "${output}" | grep -Eo '^[^/]+'
        rm -f "${output}"
        return 0
        ;;
      *)
        handle_curl_result "${ret}" "${url}" "${output}" "checking redirects for"
        return "$?"
        ;;
    esac
  fi

  if [ -n "${WGET}" ]; then
    always_run "${WGET}" -S -o "${output}" -O /dev/null "${url}"

    ret="$?"

    case "${ret}" in
      0)
        grep -m 1 Location "${output}" | grep -Eo '[^/]+/?$' | grep -Eo '[^/]+$'
        rm -f "${output}"
        return 0
        ;;
      *)
        handle_wget_result "${ret}" "${url}" "${output}" "checking redirects for"
        return "$?"
        ;;
    esac
  fi

  fatal "${ERROR_F0003}" F0003
}

verify_manifest() {
  dir="${1}"
  required_file="${2}"
  old_pwd="${PWD}"
  failed=0
  cd "${dir}" || fatal "Failed to change current working directory to ${dir}." F000A

  if [ "${DRY_RUN}" -eq 1 ]; then
    progress "Would verify integrity of downloaded files."
  else
    progress "Verifying integrity of downloaded files."

    if [ -r "./Manifest" ]; then
      files="$(awk '{ print $1 }' ./Manifest)"
      if [ -n "${required_file}" ]; then
        progress "Verifying that ${required_file} exists in Manifest"

        if ! echo "${files}" | grep -qxF "${required_file}" ; then
          fatal "${required_file} is not present in Manifest" F052C
        fi

        if [ ! -r "${required_file}" ]; then
          fatal "${required_file} cannot be read" F052D
        fi
      fi
      for f in ${files} ; do
        if [ -r "${f}" ]; then
          file_info="$(grep -E "^${f} " ./Manifest)"
          file_size="$(echo "${file_info}" | awk '{ print $2 }')"
          file_hash="$(echo "${file_info}" | awk '{ print $3 }')"

          actual_size="$(wc -c "${f}" | awk '{ print $1 }')"
          if [ "${actual_size}" -ne "${file_size}" ]; then
            failed="$((failed + 1))"
            warning "Invalid file size for ${dir}/${f}, expected ${file_size}, actual ${actual_size}"
            continue
          fi

          if ! echo "${file_hash} *${f}" | safe_sha256sum -c -; then
            failed="$((failed + 1))"
            warning "Invalid checksum for ${dir}/${f}, expected: ${file_hash}, actual $(safe_sha256sum "${f}" | awk '{print $1}')"
          fi
        fi
      done
    elif [ -r "./sha256sums.txt" ]; then
      files="$(awk '{ print $2 }' ./sha256sums.txt | tr -d '*' | sed -e 's|^\./||' -)"
      if [ -n "${required_file}" ]; then
        progress "Verifying that ${required_file} exists in sha256sums.txt"

        if ! echo "${files}" | grep -qxF "${required_file}" ; then
          fatal "${required_file} is not present in sha256sums.txt" F052C
        fi

        if [ ! -r "${required_file}" ]; then
          fatal "${required_file} cannot be read" F052D
        fi
      fi
      for f in ${files} ; do
        if [ -r "${f}" ]; then
          csum="$(grep -E "${f}\$" ./sha256sums.txt | awk '{print $1}')"
          if ! echo "${csum} *${f}" | safe_sha256sum -c -; then
            failed="$((failed + 1))"
            warning "Invalid checksum for ${dir}/${f}, expected: ${csum}, actual $(safe_sha256sum "${f}" | awk '{print $1}')"
          fi
        fi
      done
    else
      fatal "No Manifest or sha256sums.txt file found, unable to verify files in ${dir}" F0524
    fi
  fi

  if [ "${failed}" -gt 0 ]; then
    fatal "${failed} files failed verification, see warnings for details on the failed files.\n${BADCACHE_MSG}" F0525
  fi

  cd "${old_pwd}" || fatal "Failed to change current working directory to ${old_pwd}." F000A
}

safe_sha256sum() {
  if command -v shasum > /dev/null 2>&1; then
    shasum -a 256 "$@"
  elif command -v sha256sum > /dev/null 2>&1; then
    sha256sum "$@"
  else
    fatal "Could not find a usable checksum tool. Either sha256sum, or a version of shasum supporting SHA256 checksums is required to proceed with installation." F0004
  fi
}

report_bad_sha256sum() {
    file="${1}"
    sums="${2}"

    actual="$(safe_sha256sum "${file}" | awk '{ print $1 }')"
    expected="$(grep "${file}" "${sums}" | awk '{ print $1 }')"

    printf "Expected: %s\n" "${expected}"
    printf "Actual: %s\n" "${actual}"
}

get_system_info() {
  SYSARCH="$(uname -m)"

  case "$(uname -s)" in
    Linux)
      SYSTYPE="Linux"

      if [ -z "${SKIP_DISTRO_DETECTION}" ]; then
        os_release_file=
        if [ -s "/etc/os-release" ] && [ -r "/etc/os-release" ]; then
          os_release_file="/etc/os-release"
        elif [ -s "/usr/lib/os-release" ] && [ -r "/usr/lib/os-release" ]; then
          os_release_file="/usr/lib/os-release"
        else
          warning "Cannot find usable OS release information. Native packages will not be available for this install."
        fi

        if [ -n "${os_release_file}" ]; then
          # shellcheck disable=SC1090
          . "${os_release_file}"

          DISTRO="${ID}"
          SYSVERSION="${VERSION_ID}"
          SYSCODENAME="${VERSION_CODENAME}"
        else
          DISTRO="unknown"
          DISTRO_COMPAT_NAME="unknown"
          SYSVERSION="unknown"
          SYSCODENAME="unknown"
        fi
      else
        warning "Distribution auto-detection overridden by user. This is not guaranteed to work, and is not officially supported."
      fi

      supported_compat_names="debian ubuntu centos fedora opensuse ol amzn arch"

      if str_in_list "${DISTRO}" "${supported_compat_names}"; then
          DISTRO_COMPAT_NAME="${DISTRO}"
      else
          case "${DISTRO}" in
          raspbian)
              if [ "$SYSARCH" = "armv7l" ] || [ "$SYSARCH" = "aarch64" ]; then
                DISTRO_COMPAT_NAME="debian"
              fi
              ;;
          opensuse-leap)
              DISTRO_COMPAT_NAME="opensuse"
              ;;
          opensuse-tumbleweed)
              DISTRO_COMPAT_NAME="opensuse"
              SYSVERSION="tumbleweed"
              ;;
          cloudlinux|almalinux|centos-stream|rocky|rhel)
              DISTRO_COMPAT_NAME="centos"
              ;;
          artix|manjaro|obarun)
              DISTRO_COMPAT_NAME="arch"
              ;;
          *)
              DISTRO_COMPAT_NAME="unknown"
              ;;
          esac
      fi

      case "${DISTRO_COMPAT_NAME}" in
        centos|ol) SYSVERSION=$(echo "$SYSVERSION" | cut -d'.' -f1) ;;
      esac
      ;;
    Darwin)
      SYSTYPE="Darwin"
      SYSVERSION="$(sw_vers -buildVersion)"
      ;;
    FreeBSD)
      SYSTYPE="FreeBSD"
      SYSVERSION="$(uname -K)"
      ;;
    *) fatal "Unsupported system type detected. Netdata cannot be installed on this system using this script." F0200 ;;
  esac
}

str_in_list() {
  printf "%s\n" "${2}" | tr ' ' "\n" | grep -qE "^${1}\$"
  return $?
}

confirm_root_support() {
  if [ "$(id -u)" -ne "0" ]; then
    if [ -z "${ROOTCMD}" ] && command -v sudo > /dev/null; then
      if [ "${INTERACTIVE}" -eq 0 ]; then
        ROOTCMD="sudo -n"
      else
        ROOTCMD="sudo"
      fi
    fi

    if [ -z "${ROOTCMD}" ] && command -v doas > /dev/null; then
      if [ "${INTERACTIVE}" -eq 0 ]; then
        ROOTCMD="doas -n"
      else
        ROOTCMD="doas"
      fi
    fi

    if [ -z "${ROOTCMD}" ] && command -v pkexec > /dev/null; then
      ROOTCMD="pkexec"
    fi

    if [ -z "${ROOTCMD}" ]; then
      fatal "This script needs root privileges to install Netdata, but cannot find a way to gain them (we support sudo, doas, and pkexec). Either re-run this script as root, or set \$ROOTCMD to a command that can be used to gain root privileges." F0201
    fi
  fi
}

confirm() {
  prompt="${1} [y/n]"

  while true; do
    echo "${prompt}"
    read -r yn

    case "$yn" in
      [Yy]*) return 0;;
      [Nn]*) return 1;;
      *) echo "Please answer yes or no.";;
    esac
  done
}

# ======================================================================
# Existing install handling code

update() {
  updater="${ndprefix}/usr/libexec/netdata/netdata-updater.sh"

  if run_as_root test -x "${updater}"; then
    if [ "${DRY_RUN}" -eq 1 ]; then
      progress "Would attempt to update existing installation by running the updater script located at: ${updater}"
      return 0
    fi

    if [ "${INTERACTIVE}" -eq 0 ]; then
        opts="--non-interactive"
    else
        opts="--interactive"
    fi

    if [ -n "${NETDATA_OFFLINE_INSTALL_SOURCE}" ]; then
      export NETDATA_OFFLINE_INSTALL_SOURCE="${NETDATA_OFFLINE_INSTALL_SOURCE}"
    fi

    if run_script "${updater}" ${opts} --not-running-from-cron; then
      progress "Updated existing install at ${ndprefix}"
      return 0
    else
      if [ -n "${EXIT_REASON}" ]; then
        fatal "Failed to update existing Netdata install at ${ndprefix}: ${EXIT_REASON}" "${EXIT_CODE}"
      else
        fatal "Failed to update existing Netdata install at ${ndprefix}: Encountered an unhandled error in the updater. Further information about this error may be displayed above." U0000
      fi
    fi
  else
    warning "Could not find a usable copy of the updater script. We are unable to update this system in place."
    return 1
  fi
}

uninstall() {
  set_tmpdir
  get_system_info
  detect_existing_install

  if [ -n "${OLD_INSTALL_PREFIX}" ]; then
    INSTALL_PREFIX="$(echo "${OLD_INSTALL_PREFIX}/" | sed 's/$/netdata/g')"
  else
    INSTALL_PREFIX="${ndprefix}"
  fi

  uninstaller="${INSTALL_PREFIX}/usr/libexec/netdata/netdata-uninstaller.sh"
  uninstaller_url="https://raw.githubusercontent.com/netdata/netdata/master/packaging/installer/netdata-uninstaller.sh"

  if [ $INTERACTIVE = 0 ]; then
    FLAGS="--yes --force"
  else
    FLAGS="--yes"
  fi

  if run_as_root test -x "${uninstaller}"; then
    if [ "${DRY_RUN}" -eq 1 ]; then
      progress "Would attempt to uninstall existing install with uninstaller script found at: ${uninstaller}"
      return 0
    else
      progress "Found existing netdata-uninstaller. Running it.."
      # shellcheck disable=SC2086
      if ! run_script "${uninstaller}" ${FLAGS}; then
        warning "Uninstaller failed. Some parts of Netdata may still be present on the system."
      fi
    fi
  else
    if [ "${DRY_RUN}" -eq 1 ]; then
      progress "Would download uninstaller script from: ${uninstaller_url}"
      progress "Would attempt to uninstall existing install with downloaded uninstaller script."
      return 0
    else
      progress "Downloading netdata-uninstaller ..."
      if download "${uninstaller_url}" "${tmpdir}/netdata-uninstaller.sh"; then
        chmod +x "${tmpdir}/netdata-uninstaller.sh"
        # shellcheck disable=SC2086
        if ! run_script "${tmpdir}/netdata-uninstaller.sh" ${FLAGS}; then
            warning "Uninstaller failed. Some parts of Netdata may still be present on the system."
        fi
      else
        fatal "Failed to fetch uninstaller script" F0529
      fi
    fi
  fi
}

detect_existing_install() {
  set_tmpdir

  progress "Checking for existing installations of Netdata..."
  EXISTING_INSTALL_IS_NATIVE="0"

  if [ -n "${INSTALL_PREFIX}" ]; then
    searchpath="/opt/netdata/bin:${INSTALL_PREFIX}/bin:${INSTALL_PREFIX}/sbin:${INSTALL_PREFIX}/usr/bin:${INSTALL_PREFIX}/usr/sbin:${PATH}"
    searchpath="${INSTALL_PREFIX}/netdata/bin:${INSTALL_PREFIX}/netdata/sbin:${INSTALL_PREFIX}/netdata/usr/bin:${INSTALL_PREFIX}/netdata/usr/sbin:${searchpath}"
  else
    searchpath="/opt/netdata/bin:${PATH}"
  fi

  while [ -n "${searchpath}" ]; do
    _ndpath="$(PATH="${searchpath}" command -v netdata 2>/dev/null)"

    if [ -n "${_ndpath}" ]; then
      _ndpath="$(canonical_path "${_ndpath}")"
    fi

    if [ -z "${ndpath}" ] && [ -n "${_ndpath}" ]; then
      ndpath="${_ndpath}"
    elif [ -n "${_ndpath}" ] && [ "${ndpath}" != "${_ndpath}" ]; then
      fatal "Multiple installs of Netdata agent detected (located at '${ndpath}' and '${_ndpath}'). Such a setup is not generally supported. If you are certain you want to operate on one of them despite this, use the '--install-prefix' option to specifiy the install you want to operate on." F0517
    fi

    if [ -n "${INSTALL_PREFIX}" ] && [ -n "${ndpath}" ]; then
      break
    elif [ -z "${_ndpath}" ]; then
      break
    elif echo "${searchpath}" | grep -v ':'; then
      searchpath=""
    else
      searchpath="$(echo "${searchpath}" | cut -f 2- -d ':')"
    fi
  done

  if pkg_installed netdata; then
    ndprefix="/"
    EXISTING_INSTALL_IS_NATIVE="1"
  elif [ -n "${ndpath}" ]; then
    case "${ndpath}" in
      */usr/bin/netdata|*/usr/sbin/netdata) ndprefix="$(dirname "$(dirname "$(dirname "${ndpath}")")")" ;;
      *) ndprefix="$(dirname "$(dirname "${ndpath}")")" ;;
    esac

    if echo "${ndprefix}" | grep -Eq '^/usr$'; then
      ndprefix="$(dirname "${ndprefix}")"
    fi
  fi

  if [ -n "${ndprefix}" ]; then
    typefile="${ndprefix}/etc/netdata/.install-type"
    if [ -r "${typefile}" ]; then
      # shellcheck disable=SC1090,SC1091
      . "${typefile}"
    else
      INSTALL_TYPE="unknown"
    fi

    envfile="${ndprefix}/etc/netdata/.environment"
    if [ "${INSTALL_TYPE}" = "unknown" ] || [ "${INSTALL_TYPE}" = "custom" ]; then
      if [ -r "${envfile}" ]; then
        # shellcheck disable=SC1090,SC1091
        . "${envfile}"
        if [ -n "${NETDATA_IS_STATIC_INSTALL}" ]; then
          if [ "${NETDATA_IS_STATIC_INSTALL}" = "yes" ]; then
            INSTALL_TYPE="legacy-static"
          else
            INSTALL_TYPE="legacy-build"
          fi
        fi
      fi
    fi
  fi
}

handle_existing_install() {
  detect_existing_install

  if [ -z "${ndprefix}" ] || [ -z "${INSTALL_TYPE}" ]; then
    progress "No existing installations of netdata found, assuming this is a fresh install."
    return 0
  fi

  case "${INSTALL_TYPE}" in
    kickstart-*|legacy-*|binpkg-*|manual-static|unknown)
      if [ "${INSTALL_TYPE}" = "unknown" ]; then
        if [ "${EXISTING_INSTALL_IS_NATIVE}" -eq 1 ]; then
          warning "Found an existing netdata install managed by the system package manager, but could not determine the install type. Usually this means you installed an unsupported third-party netdata package. This script supports claiming most such installs, but attempting to update or reinstall them using this script may be dangerous."
        else
          warning "Found an existing netdata install at ${ndprefix}, but could not determine the install type. Usually this means you installed Netdata through your distribution’s regular package repositories or some other unsupported method."
        fi
      else
        progress "Found an existing netdata install at ${ndprefix}, with installation type '${INSTALL_TYPE}'."
      fi

      if [ "${ACTION}" = "reinstall" ] || [ "${ACTION}" = "unsafe-reinstall" ]; then
        progress "Found an existing netdata install at ${ndprefix}, but user requested reinstall, continuing."

        case "${INSTALL_TYPE}" in
          binpkg-*) NETDATA_REQUESTED_INSTALL_TYPE='native' ;;
          *-build) NETDATA_REQUESTED_INSTALL_TYPE='build' ;;
          *-static) NETDATA_REQUESTED_INSTALL_TYPE='static' ;;
          *)
            if [ "${ACTION}" = "unsafe-reinstall" ]; then
              warning "Reinstalling over top of a ${INSTALL_TYPE} installation may be unsafe, but the user has requested we proceed."
            elif [ "${INTERACTIVE}" -eq 0 ]; then
              fatal "User requested reinstall, but we cannot safely reinstall over top of a ${INSTALL_TYPE} installation, exiting." F0104
            else
              if [ "${EXISTING_INSTALL_IS_NATIVE}" ]; then
                reinstall_prompt="Reinstalling over top of an existing install managed by the system package manager is known to cause things to break, are you sure you want to continue?"
              else
                reinstall_prompt="Reinstalling over top of a ${INSTALL_TYPE} installation may be unsafe, do you want to continue?"
              fi

              if confirm "${reinstall_prompt}"; then
                progress "OK, continuing."
              else
                fatal "Cancelling reinstallation at user request." F0105
              fi
            fi
            ;;
        esac

        return 0
      elif [ "${INSTALL_TYPE}" = "unknown" ]; then
        claimonly_notice="If you just want to claim this install, you should re-run this command with the --claim-only option instead."
        if [ "${ACTION}" != "claim" ]; then
          if [ "${EXISTING_INSTALL_IS_NATIVE}" -eq 1 ]; then
            fatal "Attempting to update an installation managed by the system package manager is known to not work in most cases. If you are trying to install the latest version of Netdata, you will need to manually uninstall it through your system package manager. ${claimonly_notice}" F0106
          else
            fatal "We do not support trying to update installations when we cannot determine the install type. You will need to uninstall the existing install using the same method you used to install it to proceed. ${claimonly_notice}" F0106
          fi
        fi
      fi

      ret=0

      if [ "${ACTION}" != "claim" ]; then
        if ! update; then
          warning "Failed to update existing Netdata install at ${ndprefix}."
        else
          progress "Successfully updated existing netdata install at ${ndprefix}."
        fi
      else
        warning "Not updating existing install at ${ndprefix}."
      fi

      if [ -n "${NETDATA_CLAIM_TOKEN}" ]; then
        progress "Attempting to claim existing install at ${ndprefix}."
        INSTALL_PREFIX="${ndprefix}"
        claim
        ret=$?
      elif [ "${ACTION}" = "claim" ]; then
        fatal "User asked to claim, but did not provide a claiming token." F0202
      else
        progress "Not attempting to claim existing install at ${ndprefix} (no claiming token provided)."
      fi

      dem_claim_status=$ret
      install_dem_runtime
      ret=$dem_claim_status
      deferred_warnings
      success_banner
      cleanup
      trap - EXIT
      exit $ret
      ;;
    oci)
      fatal "This is an OCI container, use the regular container lifecycle management commands for your container tools instead of this script for managing it." F0203
      ;;
    *)
      if [ "${ACTION}" = "reinstall" ] || [ "${ACTION}" = "unsafe-reinstall" ]; then
        if [ "${ACTION}" = "unsafe-reinstall" ]; then
          warning "Reinstalling over top of a ${INSTALL_TYPE} installation may be unsafe, but the user has requested we proceed."
        elif [ "${INTERACTIVE}" -eq 0 ]; then
          fatal "User requested reinstall, but we cannot safely reinstall over top of a ${INSTALL_TYPE} installation, exiting." F0104
        else
          if confirm "Reinstalling over top of a ${INSTALL_TYPE} installation may be unsafe, do you want to continue?"; then
            progress "OK, continuing."
          else
            fatal "Cancelling reinstallation at user request." F0105
          fi
        fi
      else
        if [ -n "${NETDATA_CLAIM_TOKEN}" ]; then
          progress "Attempting to claim existing install at ${ndprefix}."
          INSTALL_PREFIX="${ndprefix}"
          claim
          ret=$?
          dem_claim_status=$ret
          install_dem_runtime
          ret=$dem_claim_status

          cleanup
          trap - EXIT
          exit $ret
        elif [ "${ACTION}" = "claim" ]; then
          fatal "User asked to claim, but did not provide a claiming token." F0202
        else
          fatal "Found an existing netdata install at ${ndprefix}, but the install type is '${INSTALL_TYPE}', which is not supported by this script, refusing to proceed." F0103
        fi
      fi
      ;;
  esac
}

confirm_install_prefix() {
  if [ -n "${INSTALL_PREFIX}" ] && [ "${NETDATA_REQUESTED_INSTALL_TYPE}" != 'build' ]; then
    fatal "The --install-prefix option is only supported together with the --build-only option." F0204
  fi

  if [ -n "${INSTALL_PREFIX}" ]; then
    NETDATA_INSTALLER_OPTIONS="${NETDATA_INSTALLER_OPTIONS} --install-prefix ${INSTALL_PREFIX}"
  else
    case "${SYSTYPE}" in
      Darwin)
        INSTALL_PREFIX="/usr/local/netdata"
        NETDATA_INSTALLER_OPTIONS="${NETDATA_INSTALLER_OPTIONS} --install-no-prefix ${INSTALL_PREFIX}"
        ;;
      FreeBSD)
        INSTALL_PREFIX="/usr/local"
        NETDATA_INSTALLER_OPTIONS="${NETDATA_INSTALLER_OPTIONS} --install-no-prefix ${INSTALL_PREFIX}"
        ;;
    esac
  fi
}

# ======================================================================
# Claiming support code

check_claim_opts() {
# shellcheck disable=SC2235,SC2030
  if [ -z "${NETDATA_CLAIM_TOKEN}" ] && [ -n "${NETDATA_CLAIM_ROOMS}" ]; then
    fatal "Invalid claiming options, claim rooms may only be specified when a token is specified." F0204
  elif [ -z "${NETDATA_CLAIM_TOKEN}" ] && [ -n "${NETDATA_CLAIM_EXTRA}${NETDATA_CLAIM_PROXY}${NETDATA_CLAIM_NORELOAD}${NETDATA_CLAIM_INSECURE}" ]; then
    # The above condition checks if _any_ claiming options other than the rooms have been set when the token is unset.
    fatal "Invalid claiming options, a claiming token must be specified." F0204
  fi
}

is_netdata_running() {
  if command -v pgrep > /dev/null 2>&1; then
    if pgrep netdata; then
      return 0
    else
      return 1
    fi
  else
    if [ -z "${INSTALL_PREFIX}" ]; then
      NETDATACLI_PATH=/usr/sbin/netdatacli
    elif [ "${INSTALL_PREFIX}" = "/opt/netdata" ]; then
      NETDATACLI_PATH="/opt/netdata/bin/netdatacli"
    else
      NETDATACLI_PATH="${INSTALL_PREFIX}/netdata/usr/sbin/netdatacli"
    fi

    if "${NETDATACLI_PATH}" ping > /dev/null 2>&1; then
      return 0
    else
      return 1
    fi
  fi
}

write_claim_config() {
  old_pwd="${PWD}"
  set_tmpdir
  cd "${old_pwd}" || fatal "Failed to change current working directory to ${old_pwd}." F000A

  if [ -z "${INSTALL_PREFIX}" ] || [ "${INSTALL_PREFIX}" = "/" ]; then
    config_path="/etc/netdata"
    netdatacli="$(command -v netdatacli)"
  elif [ "${INSTALL_PREFIX}" = "/opt/netdata" ]; then
    config_path="/opt/netdata/etc/netdata"
    netdatacli="/opt/netdata/bin/netdatacli"
  elif [ ! -d "${INSTALL_PREFIX}/netdata" ]; then
    config_path="${INSTALL_PREFIX}/etc/netdata"
    netdatacli="${INSTALL_PREFIX}/usr/sbin/netdatacli"
  else
    config_path="${INSTALL_PREFIX}/netdata/etc/netdata"
    netdatacli="${INSTALL_PREFIX}/netdata/usr/sbin/netdatacli"
  fi

  claim_config="${config_path}/claim.conf"
  claim_config_tmp="$(mktemp "${tmpdir}/claim.conf.XXXXXX")"

  if [ "${DRY_RUN}" -eq 1 ]; then
    progress "Would attempt to write claiming configuration to ${claim_config}"
    return 0
  fi

  progress "Writing claiming configuration to ${claim_config}"

  printf "[global]\n" > "${claim_config_tmp}" || return 1
  printf "    url = %s\n" "${NETDATA_CLAIM_URL}" >> "${claim_config_tmp}" || return 1
  printf "    token = %s\n" "${NETDATA_CLAIM_TOKEN}" >> "${claim_config_tmp}" || return 1
  if [ -n "${NETDATA_CLAIM_ROOMS}" ]; then
    printf "    rooms = %s\n" "${NETDATA_CLAIM_ROOMS}" >> "${claim_config_tmp}" || return 1
  fi
  if [ -n "${NETDATA_CLAIM_PROXY}" ]; then
    printf "    proxy = %s\n" "${NETDATA_CLAIM_PROXY}" >> "${claim_config_tmp}" || return 1
  fi
  if [ -n "${NETDATA_CLAIM_INSECURE}" ]; then
    printf "    insecure = %s\n" "${NETDATA_CLAIM_INSECURE}" >> "${claim_config_tmp}" || return 1
  fi

  run_as_root chown "root:${NETDATA_CLAIM_GROUP:-netdata}" "${claim_config_tmp}" || return 1
  run_as_root chmod 0640 "${claim_config_tmp}" || return 1
  run_as_root mv -f "${claim_config_tmp}" "${claim_config}" || return 1

  if [ -z "${NETDATA_CLAIM_NORELOAD}" ]; then
    if [ -n "${netdatacli}" ]; then
      run_as_root "${netdatacli}" reload-claiming-state || return 1
    else
      warning "Unable to find the netdatacli binary, the agent must be restarted to finish claiming"
    fi
  fi
}

run_claim_script() {
  if [ -n "${NETDATA_CLAIM_NORELOAD}" ]; then
    NETDATA_CLAIM_EXTRA="${NETDATA_CLAIM_EXTRA} -daemon-not-running"
  fi

  if [ -n "${NETDATA_CLAIM_INSECURE}" ]; then
    NETDATA_CLAIM_EXTRA="${NETDATA_CLAIM_EXTRA} -insecure"
  fi

  if [ -n "${NETDATA_CLAIM_PROXY}" ]; then
    if [ "${NETDATA_CLAIM_PROXY}" = "none" ]; then
      NETDATA_CLAIM_EXTRA="${NETDATA_CLAIM_EXTRA} -noproxy"
    else
      NETDATA_CLAIM_EXTRA="${NETDATA_CLAIM_EXTRA} -proxy=${NETDATA_CLAIM_PROXY}"
    fi
  fi

  # shellcheck disable=SC2086
  run_as_root "${NETDATA_CLAIM_PATH}" -token="${NETDATA_CLAIM_TOKEN}" -rooms="${NETDATA_CLAIM_ROOMS}" -url="${NETDATA_CLAIM_URL}" ${NETDATA_CLAIM_EXTRA}
  case $? in
    0) progress "Successfully claimed node" ;;
    1) warning "Unable to claim node due to invalid claiming options. If you are seeing this message, you’ve probably found a bug and should open a bug report at ${AGENT_BUG_REPORT_URL}" ;;
    2) warning "Unable to claim node due to issues creating the claiming directory or preparing the local claiming key. Make sure you have a working openssl command and that ${INSTALL_PREFIX}/var/lib/netdata/cloud.d exists, then try again." ;;
    3) warning "Unable to claim node due to missing dependencies. Usually this means that the Netdata Agent was built without support for Netdata Cloud. If you built the agent from source, please install all needed dependencies for Cloud support. If you used the regular installation script and see this error, please file a bug report at ${AGENT_BUG_REPORT_URL}." ;;
    4) warning "Failed to claim node due to inability to connect to ${NETDATA_CLAIM_URL}. Usually this either means that the specified claiming URL is wrong, or that you are having networking problems." ;;
    5) progress "Successfully claimed node, but was not able to notify the Netdata Agent. You will need to restart the Netdata service on this node before it will show up in the Cloud." ;;
    8) warning "Failed to claim node due to an invalid agent ID. You can usually resolve this by removing ${INSTALL_PREFIX}/var/lib/netdata/registry/netdata.public.unique.id and restarting the agent. Then try to claim it again using the same options." ;;
    9) warning "Failed to claim node due to an invalid node name. This probably means you tried to specify a custom name for this node (for example, using the --claim-hostname option), but the hostname itself was either empty or consisted solely of whitespace. You can resolve this by specifying a valid host name and trying again." ;;
    10) warning "Failed to claim node due to an invalid room ID. This issue is most likely caused by a typo.  Please check if the room(s) you are trying to add appear on the list of rooms provided to the --claim-rooms option ('${NETDATA_CLAIM_ROOMS}'). Then verify if the rooms are visible in Netdata Cloud and try again." ;;
    11) warning "Failed to claim node due to an issue with the generated RSA key pair. You can usually resolve this by removing all files in ${INSTALL_PREFIX}/var/lib/netdata/cloud.d and then trying again." ;;
    12) warning "Failed to claim node due to an invalid or expired claiming token. Please check that the token specified with the --claim-token option ('${NETDATA_CLAIM_TOKEN}') matches what you see in the Cloud and try again." ;;
    13) warning "Failed to claim node because the Cloud thinks it is already claimed. If this node was created by cloning a VM or as a container from a template, please remove the file ${INSTALL_PREFIX}/var/lib/netdata/registry/netdata.public.unique.id and restart the agent. Then try to claim it again with the same options. Otherwise, if you are certain this node has never been claimed before, you can use the --claim-id option to specify a new node ID to use for claiming, for example by using the uuidgen command like so: --claim-id \"\$(uuidgen)\"" ;;
    14) warning "Failed to claim node because the node is already in the process of being claimed. You should not need to do anything to resolve this, the node should show up properly in the Cloud soon. If it does not, please report a bug at ${AGENT_BUG_REPORT_URL}." ;;
    15|16|17) warning "Failed to claim node due to an internal server error in the Cloud. Please retry claiming this node later, and if you still see this message file a bug report at ${CLOUD_BUG_REPORT_URL}." ;;
    18) warning "Unable to claim node because this Netdata installation does not have a unique ID yet. Make sure the agent is running and started up correctly, and then try again." ;;
    *) warning "Failed to claim node for an unknown reason. This usually means either networking problems or a bug. Please retry claiming later, and if you still see this message file a bug report at ${AGENT_BUG_REPORT_URL}" ;;
  esac
}

claim() {
  if [ "${DRY_RUN}" -eq 1 ]; then
    progress "Would attempt to claim agent to ${NETDATA_CLAIM_URL}"
  else
    progress "Attempting to claim agent to ${NETDATA_CLAIM_URL}"
  fi

  if command -v netdata-claim.sh > /dev/null 2>&1; then
    NETDATA_CLAIM_PATH="$(command -v netdata-claim.sh)"
  elif [ -z "${INSTALL_PREFIX}" ] || [ "${INSTALL_PREFIX}" = "/" ]; then
    NETDATA_CLAIM_PATH=/usr/sbin/netdata-claim.sh
  elif [ "${INSTALL_PREFIX}" = "/opt/netdata" ]; then
    NETDATA_CLAIM_PATH="/opt/netdata/bin/netdata-claim.sh"
  elif [ ! -d "${INSTALL_PREFIX}/netdata" ]; then
    if [ -d "${INSTALL_PREFIX}/usr" ]; then
        NETDATA_CLAIM_PATH="${INSTALL_PREFIX}/usr/sbin/netdata-claim.sh"
    else
        NETDATA_CLAIM_PATH="${INSTALL_PREFIX}/sbin/netdata-claim.sh"
    fi
  else
    NETDATA_CLAIM_PATH="${INSTALL_PREFIX}/netdata/usr/sbin/netdata-claim.sh"
  fi

  method="script"
  err_msg=
  err_code=
  if [ -z "${NETDATA_CLAIM_PATH}" ]; then
    method="config"
  elif [ ! -e "${NETDATA_CLAIM_PATH}" ]; then
    method="config"
  elif [ ! -f "${NETDATA_CLAIM_PATH}" ]; then
    err_msg="Unable to claim node: ${NETDATA_CLAIM_PATH} is not a file."
    err_code=F0513
  elif grep -q '%%NEW_CLAIMING_METHOD%%' "${NETDATA_CLAIM_PATH}"; then
    method="config"
  elif [ ! -x "${NETDATA_CLAIM_PATH}" ]; then
    err_msg="Unable to claim node: claiming script at ${NETDATA_CLAIM_PATH} is not executable. Reinstalling Netdata may resolve this."
    err_code=F0514
  fi

  if [ -n "$err_msg" ]; then
    if [ "${ACTION}" = "claim" ]; then
      fatal "$err_msg" "$err_code"
    else
      warning "$err_msg"
      return 1
    fi
  fi

  if ! is_netdata_running; then
    NETDATA_CLAIM_NORELOAD=1
  fi

  case ${method} in
    script) run_claim_script ;;
    config)
      if ! write_claim_config; then
        warning "Failed to write claiming configuration. This usually means you do not have permissions to access the configuration directory."
      fi
      ;;
  esac

  if [ "${ACTION}" = "claim" ]; then
    deferred_warnings
    printf >&2 "%s\n" "For community support, you can connect with us on:"
    support_list
    cleanup
    trap - EXIT
    exit 1
  fi
}

# ======================================================================
# Explicit DEM browser runtime setup.
install_dem_runtime() {
  [ "${NETDATA_INSTALL_DEM_RUNTIME}" -eq 1 ] || return 0

  if setup_dem_runtime; then
    return 0
  else
    dem_setup_status=$?
    warning "Explicit DEM runtime setup failed (status ${dem_setup_status}). The Agent may already be installed; browser runtime setup did not complete."
    cleanup
    trap - EXIT
    exit "${dem_setup_status}"
  fi
}

dem_runtime_config() {
  # Use the Agent's config parser, including legacy-key migration and compiled
  # directory defaults. No shell evaluation of configuration values is needed.
  run_as_root "${ndpath}" -W get "$1" "$2" ""
}

dem_runtime_plugin() {
  # The Agent exposes plugins as a quoted path list. Consume paths as data,
  # never shell source, and follow the Agent's first executable match.
  dem_plugin_list=$1
  while [ -n "${dem_plugin_list}" ]; do
    dem_plugin_list=${dem_plugin_list#"${dem_plugin_list%%[![:space:]]*}"}
    [ -n "${dem_plugin_list}" ] || break
    case "${dem_plugin_list}" in
      \"*)
        dem_plugin_list=${dem_plugin_list#\"}
        case "${dem_plugin_list}" in *\"*) ;; *) return 1 ;; esac
        dem_plugin_dir=${dem_plugin_list%%\"*}
        dem_plugin_list=${dem_plugin_list#*\"}
        ;;
      \'*)
        dem_plugin_list=${dem_plugin_list#\'}
        case "${dem_plugin_list}" in *\'*) ;; *) return 1 ;; esac
        dem_plugin_dir=${dem_plugin_list%%\'*}
        dem_plugin_list=${dem_plugin_list#*\'}
        ;;
      *)
        dem_plugin_dir=${dem_plugin_list%%[[:space:]]*}
        dem_plugin_list=${dem_plugin_list#"${dem_plugin_dir}"}
        ;;
    esac
    case "${dem_plugin_list}" in ''|[[:space:]]*) ;; *) return 1 ;; esac
    case "${dem_plugin_dir}" in *\\*) return 1 ;; /*) ;; *) return 1 ;; esac
    # Native plugins can be root:netdata 0750, inaccessible to the invoking user.
    # Only discovery is privileged; dem_runtime_cli still uses the service UID.
    if run_as_root test -x "${dem_plugin_dir}/dem.plugin"; then
      dem_plugin="${dem_plugin_dir}/dem.plugin"
      return 0
    fi
  done
  return 1
}

dem_runtime_cli() {
  # Run from a directory accessible to the service user, even if kickstart's
  # temporary checkout is private to root. Never create a managed store as root.
  (
    cd / || exit 1
    set -- env NETDATA_LIB_DIR="${dem_lib}" NETDATA_USER_CONFIG_DIR="${dem_config}" \
      NETDATA_STOCK_CONFIG_DIR="${dem_stock_config}" \
      "${dem_plugin}" runtime "$@" --config-dir "${dem_config}"
    if [ "$(id -u)" = "${dem_uid}" ]; then
      run "$@"
    elif command -v runuser >/dev/null 2>&1; then
      run_as_root runuser -u "${dem_user}" -- "$@"
    elif command -v sudo >/dev/null 2>&1; then
      run_as_root sudo -H -u "${dem_user}" -- "$@"
    else
      warning "Runtime setup requires runuser or sudo to execute dem.plugin as the configured service user. Run the runtime CLI as that user; do not run managed installation as root."
      return 1
    fi
  )
}

setup_dem_runtime() {
  case "${SYSTYPE}:${SYSARCH}" in
    Linux:x86_64|Linux:aarch64) ;;
    *) warning "DEM browser runtimes require Linux x86_64 or aarch64; no runtime was acquired."; return 1 ;;
  esac

  if [ "${DRY_RUN}" -ne 1 ]; then
    # A fresh source install may add /netdata to the requested prefix. Re-detect
    # the installed executable instead of guessing its configured paths.
    ndpath=
    ndprefix=
    detect_existing_install
    if [ -z "${ndpath}" ] || [ ! -x "${ndpath}" ]; then
      warning "Cannot find the installed Agent to read its runtime paths. For static/source setup, use netdata-dem-runtime.sh with the correct --agent path; custom deployments require explicit host policy and runtime setup."
      return 1
    fi
  fi
  dem_install_type=${INSTALL_TYPE}
  if [ "${DRY_RUN}" -ne 1 ] && [ "${EXISTING_INSTALL_IS_NATIVE}" -eq 1 ]; then
    # Package ownership takes precedence over the generic packaged type marker.
    dem_install_type=native
  fi
  case "${dem_install_type}" in
    native|binpkg-deb|binpkg-rpm)
      if [ "${DRY_RUN}" -ne 1 ]; then
        dem_user=$(dem_runtime_config global 'run as user') || return $?
        dem_uid=$(id -u "${dem_user}") || return $?
        if [ "${dem_user}" != netdata ] || [ "${dem_uid}" = 0 ]; then
          warning "Native runtime packages require the named netdata service account with a nonroot UID. A separately configured managed installation under another account requires administrator-prepared host policy and runtime CLI setup."
          return 1
        fi
      fi
      progress "Installing optional netdata-dem-runtime through the system package manager; package hooks may restart the Agent."
      configure_native_package_manager || return $?
      # These are the same fixed package-manager option lists used for the Agent.
      # shellcheck disable=SC2086
      run_as_root env ${env} ${pm_cmd} ${install_subcmd} ${pkg_install_opts} netdata-dem-runtime || return $?
      dem_action=status
      ;;
    kickstart-static|manual-static|legacy-static|kickstart-build|legacy-build)
      progress "Explicit DEM setup prepares host browser policy, downloads signed release metadata and the browser archive, then enables daily automatic preparation. Activation requires an operator Agent restart."
      dem_action=install
      ;;
    *)
      warning "Cannot determine a supported DEM runtime installation method. Use the optional native package for package-managed Agents, or the installed netdata-dem-runtime.sh helper for static/source setup."
      return 1
      ;;
  esac

  if [ "${DRY_RUN}" -eq 1 ]; then
    progress "Would run explicit DEM ${dem_action}; managed setup uses the administrator host helper and executes the runtime CLI as the configured nonroot service user."
    return 0
  fi
  if [ "${dem_action}" = install ]; then
    # detect_existing_install has canonicalized the Agent and its actual prefix.
    # The installed helper owns policy and service-identity discovery together.
    dem_helper="${ndprefix%/}/usr/libexec/netdata/netdata-dem-runtime.sh"
    if ! run_as_root test -x "${dem_helper}"; then
      warning "The installed DEM host setup helper is unavailable. Install an Agent with DEM support; runtime setup was not attempted."
      return 1
    fi
    if run_as_root "${dem_helper}" install --agent "${ndpath}" --allow-download; then
      progress "DEM runtime prepared with daily automatic preparation enabled. Restart the Agent explicitly to activate it; kickstart does not restart it for managed runtime activation."
      return 0
    else
      dem_cli_status=$?
      warning "Managed runtime installation failed. Check the host setup error and the dem.conf runtime.release_url setting; kickstart does not bypass host policy or qualification. Use the installed host helper for offline inputs or --manual-updates."
      return "${dem_cli_status}"
    fi
  fi
  dem_config=$(dem_runtime_config directories config) || return $?
  dem_stock_config=$(dem_runtime_config directories 'stock config') || return $?
  dem_lib=$(dem_runtime_config directories lib) || return $?
  for dem_path in "${dem_config}" "${dem_stock_config}" "${dem_lib}"; do
    case "${dem_path}" in /*) ;; *) warning "The installed Agent did not report absolute runtime directories; use its runtime CLI with explicit configuration."; return 1 ;; esac
  done
  dem_plugins=$(dem_runtime_config directories plugins) || return $?
  if ! dem_runtime_plugin "${dem_plugins}"; then
    warning "Cannot locate dem.plugin in the configured plugin directories. Install an Agent with DEM support, or run the correct installed runtime CLI explicitly for custom plugin layouts."
    return 1
  fi
  if [ "${dem_action}" = status ]; then
    # RPM can report success despite a failed posttrans hook. Ready admission,
    # observed by the actual service identity, is required before success.
    if dem_native_status=$(dem_runtime_cli status); then
      printf '%s\n' "${dem_native_status}"
      # Match the CLI's top-level mode field, not a nested runtime setting.
      if ! printf '%s\n' "${dem_native_status}" | grep -Eq '^  "mode": "native",?$'; then
        warning "The installed DEM configuration does not report native mode. Check dem.conf and use the matching runtime installation method."
        return 1
      fi
      progress "Native DEM runtime is installed and available."
    else
      dem_cli_status=$?
      warning "Native runtime is not available after package installation. Inspect package hook errors and repair the package; no managed fallback was attempted."
      return "${dem_cli_status}"
    fi
  fi
}

# ======================================================================
# Auto-update handling code.
set_auto_updates() {
  if run_as_root test -x "${INSTALL_PREFIX}/usr/libexec/netdata/netdata-updater.sh"; then
    updater="${INSTALL_PREFIX}/usr/libexec/netdata/netdata-updater.sh"
  elif run_as_root test -x "${INSTALL_PREFIX}/netdata/usr/libexec/netdata/netdata-updater.sh"; then
    updater="${INSTALL_PREFIX}/netdata/usr/libexec/netdata/netdata-updater.sh"
  else
    warning "Could not find netdata-updater.sh. This means that auto-updates cannot (currently) be enabled on this system. See https://learn.netdata.cloud/docs/agent/packaging/installer/update for more information about updating Netdata."
    return 0
  fi

  if [ "${AUTO_UPDATE}" -eq 1 ]; then
    if [ "${DRY_RUN}" -eq 1 ]; then
      progress "Would have attempted to enable automatic updates."
    # This first case is for catching using a new kickstart script with an old build. It can be safely removed after v1.34.0 is released.
    elif ! run_as_root grep -q '\--enable-auto-updates' "${updater}"; then
      echo
    elif ! run_as_root "${updater}" --enable-auto-updates "${NETDATA_AUTO_UPDATE_TYPE}"; then
      warning "Failed to enable auto updates. Netdata will still work, but you will need to update manually."
    fi
  else
    if [ "${DRY_RUN}" -eq 1 ]; then
      progress "Would have attempted to disable automatic updates."
    else
      run_as_root "${updater}" --disable-auto-updates
    fi
  fi
}

# ======================================================================
# Native package install code.

# Check for an already installed package with a given name.
pkg_installed() {
  case "${SYSTYPE}" in
    Linux)
      case "${DISTRO_COMPAT_NAME}" in
        debian|ubuntu)
          # shellcheck disable=SC2016
          dpkg-query --show --showformat '${Status}' "${1}" 2>&1 | cut -f 1 -d ' ' | grep -q '^install$'
          return $?
          ;;
        centos|fedora|opensuse|ol|amzn)
          rpm -q "${1}" > /dev/null 2>&1
          return $?
          ;;
        alpine)
          apk -e info "${1}" > /dev/null 2>&1
          return $?
          ;;
        arch)
          pacman -Qi "${1}" > /dev/null 2>&1
          return $?
          ;;
        *) return 1 ;;
      esac
      ;;
    Darwin)
      if command -v brew > /dev/null 2>&1; then
        brew list "${1}" > /dev/null 2>&1
        return $?
      else
        return 1
      fi
      ;;
    FreeBSD)
      if pkg -N > /dev/null 2>&1; then
        pkg info "${1}" > /dev/null 2>&1
        return $?
      else
        return 1
      fi
      ;;
    *) return 1 ;;
  esac
}

# Check for the existence of a usable netdata package in the repo.
netdata_avail_check() {
  case "${DISTRO_COMPAT_NAME}" in
    debian|ubuntu)
      env DEBIAN_FRONTEND=noninteractive apt-cache policy netdata | grep -q "repo.*.netdata.cloud/repos/";
      return $?
      ;;
    centos|fedora|ol|amzn)
      # shellcheck disable=SC2086
      LC_ALL=C ${pm_cmd} info --nogpgcheck netdata | grep -qE '^(Repo(sitory)?|From repo) *: *netdata(-edge)?(/.*)?$'
      return $?
      ;;
    opensuse)
      zypper packages -r "$(zypper repos | grep -E 'netdata |netdata-edge ' | cut -f 1 -d '|' | tr -d ' ')" | grep -E 'netdata '
      return $?
      ;;
    *) return 1 ;;
  esac
}

# Check for any distro-specific dependencies we know we need.
check_special_native_deps() {
  if [ "${DISTRO_COMPAT_NAME}" = "centos" ] && [ "${SYSVERSION}" -gt 6 ]; then
    progress "EPEL is required on this system, checking if it’s available."

    if [ "${SYSVERSION}" -ge 9 ]; then
      progress "EPEL support requires CodeReady Builder repo on this system, attempting to enable it."

      case "${DISTRO}" in
        rhel) run_as_root subscription-manager repos --enable "codeready-builder-for-rhel-${SYSVERSION}-$(arch)-rpms" ;;
        *) run_as_root dnf config-manager --set-enabled crb ;;
      esac
    fi

    if [ "${DISTRO}" = "rhel" ]; then
      if [ "${SYSVERSION}" -eq 7 ]; then
        epel_url="https://archives.fedoraproject.org/pub/archive/epel/7/x86_64/Packages/e/epel-release-7-14.noarch.rpm"
      else
        epel_url="https://dl.fedoraproject.org/pub/epel/epel-release-latest-${SYSVERSION}.noarch.rpm"
      fi
      # shellcheck disable=SC2086
      if ! run_as_root env ${env} ${pm_cmd} ${install_subcmd} ${pkg_install_opts} "${epel_url}"; then
        warning "Failed to install EPEL, even though it is required to install native packages on this system."
        return 1
      fi
    elif LC_ALL=C ${pm_cmd} search --nogpgcheck -v epel-release | grep -q "No matches found"; then
      warning "Unable to find a suitable source for libuv, cannot install using native packages on this system."
      return 1
    else
      progress "EPEL is available, attempting to install so that required dependencies are available."

      # shellcheck disable=SC2086
      if ! run_as_root env ${env} ${pm_cmd} ${install_subcmd} ${pkg_install_opts} epel-release; then
        warning "Failed to install EPEL, even though it is required to install native packages on this system."
        return 1
      fi
    fi
  fi
}

cleanup_apt_state() {
  cache_dir="/var/cache/apt/archives"

  if [ -d "${cache_dir}" ]; then
      run_as_root find "${cache_dir}" -type f -name 'netdata*.deb' -delete
  fi

  for p in netdata-repo netdata-repo-edge; do
    if pkg_installed "${p}" ; then
      warning "Detected already installed repository configuration ${p}, attempting to remove it."
      # shellcheck disable=SC2086
      run_as_root env ${env} ${pm_cmd} ${uninstall_subcmd} ${pkg_install_opts} "${p}"
    fi
  done
}

cleanup_dnf_state() {
  for p in "" "-edge"; do
    if pkg_installed "netdata-repo${p}"; then
      warning "Detected already installed repository configuration netdata-repo${p}, attempting to remove it."
      run_as_root "${pm_cmd}" clean all --disablerepo "*" --enablerepo "netdata${p}"
      run_as_root "${pm_cmd}" "${uninstall_subcmd}" -y "netdata-repo${p}"
    fi
  done
}

cleanup_zypper_state() {
  for p in netdata-repo netdata-repo-edge; do
    if pkg_installed "${p}" ; then
      warning "Detected already installed repository configuration ${p}, attempting to remove it."
      # shellcheck disable=SC2086
      run_as_root env ${env} ${pm_cmd} ${uninstall_subcmd} ${pkg_install_opts} "${p}"
    fi
  done
}

common_rpm_opts() {
  pkg_type="rpm"
  pkg_suffix=".noarch"
  pkg_vsep="-"
  INSTALL_TYPE="binpkg-rpm"
  NATIVE_VERSION="${INSTALL_VERSION:+"-${INSTALL_VERSION}.${SYSARCH}"}"
}

common_dnf_opts() {
  if [ "${INTERACTIVE}" = "0" ]; then
    interactive_opts="-y"
  fi
  if command -v dnf > /dev/null; then
    pm_cmd="dnf"
  else
    pm_cmd="yum"
  fi
  repo_subcmd="makecache"
  install_subcmd="install"
  pkg_install_opts="${interactive_opts}"
  repo_update_opts="${interactive_opts}"
  uninstall_subcmd="remove"
}

configure_native_package_manager() {
  interactive_opts=""
  env=""
  case "${DISTRO_COMPAT_NAME}" in
    debian|ubuntu)
      if [ "${INTERACTIVE}" = "0" ]; then
        install_subcmd="-o Dpkg::Options::=--force-confdef -o Dpkg::Options::=--force-confold install"
        interactive_opts="-y"
        env="DEBIAN_FRONTEND=noninteractive"
      else
        install_subcmd="install"
      fi
      needs_early_refresh=1
      pm_cmd="apt-get"
      repo_subcmd="update"
      pkg_type="deb"
      pkg_vsep="_"
      pkg_install_opts="${interactive_opts}"
      repo_update_opts="${interactive_opts}"
      uninstall_subcmd="purge"
      repo_prefix="${DISTRO_COMPAT_NAME}/${SYSCODENAME}"
      pkg_suffix="+${DISTRO_COMPAT_NAME}${SYSVERSION}_all"
      INSTALL_TYPE="binpkg-deb"
      NATIVE_VERSION="${INSTALL_VERSION:+"=${INSTALL_VERSION}"}"
      ;;
    centos)
      common_rpm_opts
      common_dnf_opts
      repo_prefix="el/${SYSVERSION}"
      # if [ "${SYSVERSION}" -lt 8 ]; then
      #   explicitly_install_native_plugins=1
      # fi
      ;;
    fedora|ol)
      common_rpm_opts
      common_dnf_opts
      repo_prefix="${DISTRO_COMPAT_NAME}/${SYSVERSION}"
      ;;
    opensuse)
      if [ "${INTERACTIVE}" = "0" ]; then
        install_subcmd="--non-interactive --no-gpg-checks install"
      else
        install_subcmd="--no-gpg-checks install"
      fi
      common_rpm_opts
      pm_cmd="zypper"
      repo_subcmd="--gpg-auto-import-keys refresh"
      repo_prefix="opensuse/${SYSVERSION}"
      pkg_install_opts="${interactive_opts} --allow-unsigned-rpm"
      repo_update_opts=""
      uninstall_subcmd="remove"
      ;;
    amzn)
      common_rpm_opts
      common_dnf_opts
      repo_prefix="amazonlinux/${SYSVERSION}"
      ;;
    *)
      warning "We do not provide native packages for ${DISTRO}."
      return 3
      ;;
  esac
}

try_package_install() {
  failed_refresh_msg="Failed to refresh repository metadata. ${BADNET_MSG} or incompatibilities with one or more third-party package repositories in the system package manager configuration."

  if [ -z "${DISTRO_COMPAT_NAME}" ] || [ "${DISTRO_COMPAT_NAME}" = "unknown" ]; then
    warning "Unable to determine Linux distribution for native packages."
    return 3
  elif [ -z "${SYSCODENAME}" ]; then
    case "${DISTRO_COMPAT_NAME}" in
      debian|ubuntu)
        warning "Release codename not set. Unable to check availability of native packages for this system."
        return 3
        ;;
    esac
  fi

  set_tmpdir

  if [ "${DRY_RUN}" -eq 1 ]; then
    progress "Would attempt to install using native packages..."
  else
    progress "Attempting to install using native packages..."
  fi

  if [ "${SELECTED_RELEASE_CHANNEL}" = "nightly" ]; then
    release="-edge"
  else
    release=""
  fi

  if [ -n "${SKIP_DISTRO_DETECTION}" ]; then
    warning "Attempting to use native packages with a distro override. This is not officially supported, but may work in some cases. If your system requires a distro override to use native packages, please open a feature request at ${AGENT_BUG_REPORT_URL} about it so that we can update the installer to auto-detect this."
  fi

  configure_native_package_manager || return $?

  if [ -n "${INSTALL_VERSION}" ]; then
    if echo "${INSTALL_VERSION}" | grep -q "nightly"; then
      new_release="-edge"
    else
      new_release=
    fi

    if { [ -n "${new_release}" ] && [ -z "${release}" ]; } || { [ -z "${new_release}" ] && [ -n "${release}" ]; }; then
      warning "Selected release channel does not match this version and it will be changed automatically."
    fi

    release="${new_release}"
  fi

  repoconfig_name="netdata-repo${release}"

  case "${SELECTED_RELEASE_CHANNEL}" in
    stable) REPO_URL_PREFIX="${STABLE_REPO_URL_PREFIX}" ;;
    nightly) REPO_URL_PREFIX="${NIGHTLY_REPO_URL_PREFIX}" ;;
  esac

  case "${pkg_type}" in
    deb)
      repoconfig_file="${repoconfig_name}${pkg_vsep}${REPOCONFIG_DEB_VERSION}${pkg_suffix}.${pkg_type}"
      repoconfig_url="${REPOCONFIG_URL_PREFIX}/${repo_prefix}/${repoconfig_file}"
      ref_check_url="${REPOCONFIG_URL_PREFIX}"
      pub_check_url="${REPO_URL_PREFIX}/${repo_prefix}/.currently.published"
      pkg_meta_url="${REPO_URL_PREFIX}/${repo_prefix}/InRelease"
      ;;
    rpm)
      repoconfig_file="${repoconfig_name}${pkg_vsep}${REPOCONFIG_RPM_VERSION}${pkg_suffix}.${pkg_type}"
      repoconfig_url="${REPOCONFIG_URL_PREFIX}/${repo_prefix}/${SYSARCH}/${repoconfig_file}"
      ref_check_url="${REPOCONFIG_URL_PREFIX}"
      pub_check_url="${REPO_URL_PREFIX}/${repo_prefix}/${SYSARCH}/.currently.published"
      pkg_meta_url="${REPO_URL_PREFIX}/${repo_prefix}/${SYSARCH}/repodata/repomd.xml"
      ;;
  esac

  check_for_remote_file "${ref_check_url}"

  case "$?" in
    0) ;;
    1) NETDATA_ASSUME_REMOTE_FILES_ARE_PRESENT=1 ;;
    *)
      warning "Unable to communicate with Netdata package repositories"
      return 1
      ;;
  esac

  progress "Checking if native packages are being published for this platform."
  check_for_remote_file "${pub_check_url}"

  case "$?" in
    0) ;;
    1)
      warning "Native packages are not being published for this system."
      return 3
      ;;
    *)
      warning "Unable to determine if native packages are being published for this system."
      return 1
      ;;
  esac

  # Due to the check above for pub_check_url, it’s safe to assume here
  # that the only error we’re going to get is a missing file.
  if ! check_for_remote_file "${pkg_meta_url}"; then
    warning "Native packages have not yet been published for this system."
    return 4
  fi

  progress "Checking for availability of repository configuration package."
  if ! check_for_remote_file "${repoconfig_url}"; then
    warning "No repository configuration package available for ${DISTRO} ${SYSVERSION}. Cannot install native packages on this system."
    return 3
  fi

  if ! download "${repoconfig_url}" "${tmpdir}/${repoconfig_file}"; then
    fatal "Failed to download repository configuration package. ${BADNET_MSG}." F0209
  fi

  case "${pm_cmd}" in
    apt-get) cleanup_apt_state ;;
    yum|dnf) cleanup_dnf_state ;;
    zypper) cleanup_zypper_state ;;
    *) warning "I don’t know how to clean up package manager state for ${pm_cmd}. Please report this as a bug at ${AGENT_BUG_REPORT_URL}."
  esac

  if [ -n "${needs_early_refresh}" ]; then
    # shellcheck disable=SC2086
    if ! run_as_root env ${env} ${pm_cmd} ${repo_subcmd} ${repo_update_opts}; then
      warning "${failed_refresh_msg}"
      return 2
    fi
  fi

  if ! check_special_native_deps; then
    warning "Could not find secondary dependencies for ${DISTRO} on ${SYSARCH}."
    return 2
  fi

  # shellcheck disable=SC2086
  if ! run_as_root env ${env} ${pm_cmd} ${install_subcmd} ${pkg_install_opts} "${tmpdir}/${repoconfig_file}"; then
    warning "Failed to install repository configuration package."
    return 2
  fi

  if [ -n "${repo_subcmd}" ]; then
    # shellcheck disable=SC2086
    if ! run_as_root env ${env} ${pm_cmd} ${repo_subcmd} ${repo_update_opts}; then
      fatal "${failed_refresh_msg} In most cases, disabling any third-party repositories on the system and re-running the installer with the same options should work. If that does not work, consider using a static build with the --static-only option instead of native packages." F0205
    fi
  fi

  if [ "${ACTION}" = "repositories-only" ]; then
    progress "Successfully installed repository configuraion package."
    deferred_warnings
    cleanup
    trap - EXIT
    exit 0
  fi

  if ! netdata_avail_check "${DISTRO_COMPAT_NAME}"; then
    warning "Could not find a usable native package for ${DISTRO} on ${SYSARCH}."
    if [ -z "${NO_CLEANUP}" ]; then
      progress "Attempting to uninstall repository configuration package."
      # shellcheck disable=SC2086
      run_as_root env ${env} ${pm_cmd} ${uninstall_subcmd} ${pkg_install_opts} "${repoconfig_name}"
    fi
    return 2
  fi

  if [ "${NETDATA_DISABLE_TELEMETRY}" -eq 1 ]; then
    run_as_root mkdir -p "/etc/netdata"
    run_as_root touch "/etc/netdata/.opt-out-from-anonymous-statistics"
  fi

  # shellcheck disable=SC2086
  if ! run_as_root env ${env} ${pm_cmd} ${install_subcmd} ${pkg_install_opts} "netdata${NATIVE_VERSION}"; then
    warning "Failed to install Netdata package."
    if [ -z "${NO_CLEANUP}" ]; then
      progress "Attempting to uninstall repository configuration package."
      # shellcheck disable=SC2086
      run_as_root env ${env} ${pm_cmd} ${uninstall_subcmd} ${pkg_install_opts} "${repoconfig_name}"
    fi
    return 2
  fi

  if [ -n "${explicitly_install_native_plugins}" ]; then
    progress "Installing external plugins."
    # shellcheck disable=SC2086
    if ! run_as_root env ${env} ${pm_cmd} ${install_subcmd} ${DEFAULT_PLUGIN_PACKAGES}; then
      warning "Failed to install external plugin packages. Some collectors may not be available."
    fi
  fi
}

# ======================================================================
# Static build install code
# shellcheck disable=SC2034,SC2086,SC2126
set_static_archive_urls() {
  if [ -z "${2}" ]; then
    arch="${SYSARCH}"
  else
    arch="${2}"
  fi

  prefix_set="0"

  if [ -n "${NETDATA_ARTIFACT_URL_PREFIX}" ]; then
    prefix_set="1"
    NETDATA_STATIC_ARCHIVE_URL_PREFIX="${NETDATA_ARTIFACT_URL_PREFIX}"
  elif [ -n "${NETDATA_OFFLINE_INSTALL_SOURCE}" ]; then
    prefix_set="1"
    NETDATA_STATIC_ARCHIVE_URL_PREFIX="file://${NETDATA_OFFLINE_INSTALL_SOURCE}"
  elif [ -n "${INSTALL_VERSION}" ]; then
    NETDATA_STATIC_ARCHIVE_URL_PREFIX="${RELEASE_ARTIFACT_URL_PREFIX}/${1}/v${INSTALL_VERSION}"
  else
    NETDATA_STATIC_ARCHIVE_URL_PREFIX="${RELEASE_ARTIFACT_URL_PREFIX}/${1}/latest"
  fi

  check_for_remote_file "${NETDATA_STATIC_ARCHIVE_URL_PREFIX}/Manifest"

  case $? in
    0) NETDATA_MANIFEST_INSTALL=1 ;;
    *)
      if [ "${prefix_set}" -eq 0 ]; then
        if [ "${1}" = "stable" ]; then
          if [ -n "${INSTALL_VERSION}" ]; then
            version="v${INSTALL_VERSION}"
          else
            version="$(get_redirect "https://github.com/netdata/netdata/releases/latest")"
          fi
          NETDATA_STATIC_ARCHIVE_URL_PREFIX="https://github.com/netdata/netdata/releases/download/${version}"
        else
          if [ -n "${INSTALL_VERSION}" ]; then
            version="v${INSTALL_VERSION}"
          else
            version="$(get_redirect "${NETDATA_TARBALL_BASEURL}/latest")"
          fi
          NETDATA_STATIC_ARCHIVE_URL_PREFIX="${NETDATA_TARBALL_BASEURL}/download/${version}"
        fi

        check_for_remote_file "${NETDATA_STATIC_ARCHIVE_URL_PREFIX}/Manifest"

        case $? in
          0) NETDATA_MANIFEST_INSTALL=1 ;;
          1)
            NETDATA_MANIFEST_INSTALL=0
            NETDATA_STATIC_ARCHIVE_NAME="netdata-${arch}-${version}.gz.run"
            NETDATA_STATIC_ARCHIVE_URL="${NETDATA_STATIC_ARCHIVE_URL_PREFIX}/${NETDATA_STATIC_ARCHIVE_NAME}"
            NETDATA_STATIC_ARCHIVE_CHECKSUM_URL="${NETDATA_STATIC_ARCHIVE_URL_PREFIX}/sha256sums.txt"
            ;;
          *) fatal "Unable to communicate with GitHub. ${GITHUB_BADNET_MSG}" F052A ;;
        esac
      else
        if [ -n "${INSTALL_VERSION}" ]; then
          version="v${INSTALL_VERSION}"
        else
          version="$(get_redirect "${NETDATA_STATIC_ARCHIVE_URL_PREFIX}/latest")"
        fi
        NETDATA_MANIFEST_INSTALL=0
        NETDATA_STATIC_ARCHIVE_NAME="netdata-${arch}-${version}.gz.run"
        NETDATA_STATIC_ARCHIVE_URL="${NETDATA_STATIC_ARCHIVE_URL_PREFIX}/${NETDATA_STATIC_ARCHIVE_NAME}"
        NETDATA_STATIC_ARCHIVE_CHECKSUM_URL="${NETDATA_STATIC_ARCHIVE_URL_PREFIX}/sha256sums.txt"
      fi

      check_for_remote_file "${NETDATA_STATIC_ARCHIVE_URL_PREFIX}"

      case "$?" in
        0) ;;
        1) NETDATA_ASSUME_REMOTE_FILES_ARE_PRESENT=1 ;;
        *) fatal "Unable to communicate with GitHub. ${GITHUB_BADNET_MSG}" F052A ;;
      esac
      ;;
  esac
}

try_static_install() {
  set_static_archive_urls "${SELECTED_RELEASE_CHANNEL}" || return 1

  if [ "${DRY_RUN}" -eq 1 ]; then
    progress "Would attempt to install using static build..."
  else
    progress "Attempting to install using static build..."
  fi

  if [ "${NETDATA_MANIFEST_INSTALL}" -eq 1 ]; then
    if ! download "${NETDATA_STATIC_ARCHIVE_URL_PREFIX}/Manifest" "./Manifest" "force"; then
      fatal "Unable to download release manifest file. ${BADNET_MSG}" F0526
    fi

    archive_info="$(grep -E "^netdata-${SYSARCH}-.*\\.run " ./Manifest)"

    if [ -z "${archive_info}" ]; then
      warning "Could not find a ${SELECTED_RELEASE_CHANNEL} static build for ${SYSARCH} CPUs."
      return 2
    fi

    if [ "$(echo "${archive_info}" | wc -l)" -gt 1 ]; then
      archive_info="$(echo "${archive_info}" | grep -v ".*-latest.*" | tail -n 1)"
    fi

    archive_name="$(echo "${archive_info}" | awk '{ print $1 }')"

    if ! download "${NETDATA_STATIC_ARCHIVE_URL_PREFIX}/${archive_name}" "./${archive_name}"; then
      fatal "Unable to download static build archive for ${SYSARCH}. ${BADNET_MSG}." F0208
    fi
  else
    # Check status code first, so that we can provide nicer fallback for dry runs.
    if check_for_remote_file "${NETDATA_STATIC_ARCHIVE_URL}"; then
      archive_name="${NETDATA_STATIC_ARCHIVE_NAME}"
    else
      warning "Could not find a ${SELECTED_RELEASE_CHANNEL} static build for ${SYSARCH} CPUs. ${GITHUB_BADNET_MSG}"
      return 2
    fi

    if ! download "${NETDATA_STATIC_ARCHIVE_URL}" "./${archive_name}"; then
      fatal "Unable to download static build archive for ${SYSARCH}. ${BADNET_MSG}." F0208
    fi

    if ! download "${NETDATA_STATIC_ARCHIVE_CHECKSUM_URL}" "./sha256sums.txt"; then
      fatal "Unable to fetch checksums to verify static build archive. ${BADNET_MSG}." F0206
    fi
  fi

  verify_manifest ./ "${archive_name}"

  if [ "${INTERACTIVE}" -eq 0 ]; then
    opts="${opts} --accept"
  fi

  env_cmd="env NETDATA_CERT_TEST_URL=${NETDATA_CLAIM_URL} NETDATA_CERT_MODE=check"

  if [ -n "${NETDATA_OFFLINE_INSTALL_SOURCE}" ]; then
    env_cmd="env NETDATA_CERT_TEST_URL=${NETDATA_CLAIM_URL} NETDATA_CERT_MODE=auto"
  fi

  progress "Installing netdata"
  # shellcheck disable=SC2086
  if ! run_as_root ${env_cmd} /bin/sh "./${archive_name}" ${opts} -- ${NETDATA_INSTALLER_OPTIONS}; then
    warning "Failed to install static build of Netdata on ${SYSARCH}."
    run rm -rf /opt/netdata
    return 2
  fi

  if [ "${DRY_RUN}" -ne 1 ]; then
  install_type_file="/opt/netdata/etc/netdata/.install-type"
    if [ -f "${install_type_file}" ]; then
      run_as_root sh -c "cat \"${install_type_file}\" > \"${tmpdir}/install-type\""
      run_as_root chown "$(id -u)":"$(id -g)" "${tmpdir}/install-type"
      # shellcheck disable=SC1090,SC1091
      . "${tmpdir}/install-type"
      cat > "${tmpdir}/install-type" <<- EOF
	INSTALL_TYPE='kickstart-static'
	PREBUILT_ARCH='${PREBUILT_ARCH}'
	EOF
      run_as_root chown netdata:netdata "${tmpdir}/install-type"
      run_as_root cp "${tmpdir}/install-type" "${install_type_file}"
    fi
  fi
}

# ======================================================================
# Local build install code

set_source_archive_urls() {
  prefix_set="0"

  if [ -n "${NETDATA_ARTIFACT_URL_PREFIX}" ]; then
    prefix_set="1"
    NETDATA_SOURCE_ARCHIVE_URL_PREFIX="${NETDATA_ARTIFACT_URL_PREFIX}"
  elif [ -n "${INSTALL_VERSION}" ]; then
    NETDATA_SOURCE_ARCHIVE_URL_PREFIX="${RELEASE_ARTIFACT_URL_PREFIX}/${1}/v${INSTALL_VERSION}"
  else
    NETDATA_SOURCE_ARCHIVE_URL_PREFIX="${RELEASE_ARTIFACT_URL_PREFIX}/${1}/latest"
  fi

  check_for_remote_file "${NETDATA_SOURCE_ARCHIVE_URL_PREFIX}/Manifest"

  case $? in
    0) NETDATA_MANIFEST_INSTALL=1 ;;
    *)
      if [ "${prefix_set}" -eq 0 ]; then
        if [ "${1}" = "stable" ]; then
          if [ -n "${INSTALL_VERSION}" ]; then
            version="v${INSTALL_VERSION}"
          else
            version="$(get_redirect "https://github.com/netdata/netdata/releases/latest")"
          fi
          NETDATA_SOURCE_ARCHIVE_URL_PREFIX="https://github.com/netdata/netdata/releases/download/${version}"
        else
          if [ -n "${INSTALL_VERSION}" ]; then
            version="v${INSTALL_VERSION}"
          else
            version="$(get_redirect "${NETDATA_TARBALL_BASEURL}/latest")"
          fi
          NETDATA_SOURCE_ARCHIVE_URL_PREFIX="${NETDATA_TARBALL_BASEURL}/download/${version}"
        fi

        check_for_remote_file "${NETDATA_SOURCE_ARCHIVE_URL_PREFIX}/Manifest"

        case $? in
          0) NETDATA_MANIFEST_INSTALL=1 ;;
          1)
            NETDATA_MANIFEST_INSTALL=0
            NETDATA_SOURCE_ARCHIVE_BASE_NAME="netdata-${version}.tar"
            NETDATA_SOURCE_ARCHIVE_BASEURL="${NETDATA_SOURCE_ARCHIVE_URL_PREFIX}/${NETDATA_SOURCE_ARCHIVE_BASE_NAME}"
            NETDATA_SOURCE_ARCHIVE_CHECKSUM_URL="${NETDATA_SOURCE_ARCHIVE_URL_PREFIX}/sha256sums.txt"
            ;;
          *) fatal "Unable to communicate with GitHub. ${GITHUB_BADNET_MSG}" F0528 ;;
        esac
      else
        if [ -n "${INSTALL_VERSION}" ]; then
          version="v${INSTALL_VERSION}"
        else
          version="$(get_redirect "${NETDATA_SOURCE_ARCHIVE_URL_PREFIX}/latest")"
        fi
        NETDATA_MANIFEST_INSTALL=0
        NETDATA_SOURCE_ARCHIVE_BASE_NAME="netdata-${version}.tar"
        NETDATA_SOURCE_ARCHIVE_BASEURL="${NETDATA_SOURCE_ARCHIVE_URL_PREFIX}/${NETDATA_SOURCE_ARCHIVE_BASE_NAME}"
        NETDATA_SOURCE_ARCHIVE_CHECKSUM_URL="${NETDATA_SOURCE_ARCHIVE_URL_PREFIX}/sha256sums.txt"
      fi

      check_for_remote_file "${NETDATA_SOURCE_ARCHIVE_URL_PREFIX}"

      case "$?" in
        0) ;;
        1) NETDATA_ASSUME_REMOTE_FILES_ARE_PRESENT=1 ;;
        *) fatal "Unable to communicate with GitHub. ${GITHUB_BADNET_MSG}" F0528 ;;
      esac
      ;;
  esac
}

install_local_build_dependencies() {
  set_tmpdir
  bash="$(command -v bash 2> /dev/null)"

  if [ -z "${bash}" ] || [ ! -x "${bash}" ]; then
    warning "Unable to find a usable version of \`bash\` (required for local build)."
    return 1
  fi

  if ! download "${PACKAGES_SCRIPT}" "${tmpdir}/install-required-packages.sh"; then
    fatal "Failed to download dependency handling script for local build. ${BADNET_MSG}." F000D
  fi

  if [ "${DRY_RUN}" -eq 1 ]; then
    progress "Would run downloaded script to install required build dependencies..."
  else
    progress "Running downloaded script to install required build dependencies..."
  fi

  if [ "${INTERACTIVE}" -eq 0 ]; then
    opts="--dont-wait --non-interactive"
  fi

  # shellcheck disable=SC2086
  if [ "$(uname -s)" = "Darwin" ]; then
    if ! run "${bash}" "${tmpdir}/install-required-packages.sh" ${opts} netdata; then
      warning "Failed to install all required packages, but installation might still be possible."
    fi
  else
    if ! run_as_root "${bash}" "${tmpdir}/install-required-packages.sh" ${opts} netdata; then
      warning "Failed to install all required packages, but installation might still be possible."
    fi
  fi
}

build_and_install() {
  if [ "${DRY_RUN}" -eq 1 ]; then
    progress "Would attempt to build netdata..."
  else
    progress "Building netdata..."
  fi

  echo "INSTALL_TYPE='kickstart-build'" > system/.install-type

  opts="${NETDATA_INSTALLER_OPTIONS}"

  if [ "${INTERACTIVE}" -eq 0 ]; then
    opts="${opts} --dont-wait"
  fi

  if [ "${SELECTED_RELEASE_CHANNEL}" = "stable" ]; then
    opts="${opts} --stable-channel"
  fi

  # shellcheck disable=SC2086
  run_script ./netdata-installer.sh ${opts}

  case $? in
    0) ;;
    1)
      if [ -n "${EXIT_REASON}" ]; then
        fatal "netdata-installer.sh failed to run: ${EXIT_REASON}" "${EXIT_CODE}"
      else
        fatal "netdata-installer.sh failed to run: Encountered an unhandled error in the installer code." I0000
      fi
      ;;
    2) fatal "Insufficient RAM to install netdata." F0008 ;;
    *) fatal "netdata-installer.sh failed to run: Encountered an unhandled error in the installer code." F051A ;;
  esac
}

try_build_install() {
  set_tmpdir

  if [ "${DRY_RUN}" -eq 1 ]; then
    progress "Would attempt to install by building locally..."
  else
    progress "Attempting to install by building locally..."
  fi

  if ! install_local_build_dependencies; then
    return 1
  fi

  set_source_archive_urls "${SELECTED_RELEASE_CHANNEL}"
  zstd="$(command -v zstd 2>/dev/null || true)"
  archive_name=""

  if [ "${NETDATA_MANIFEST_INSTALL}" -eq 1 ]; then
    if ! download "${NETDATA_SOURCE_ARCHIVE_URL_PREFIX}/Manifest" "./Manifest" "force"; then
      fatal "Unable to download release manifest file. ${BADNET_MSG}" F0526
    fi

    zstd_info="$(grep -E "^netdata-.*\\.tar\\.zst " ./Manifest)"
    gzip_info="$(grep -E "^netdata-.*\\.tar\\.gz " ./Manifest)"

    if [ -n "${zstd}" ] && [ -n "${zstd_info}" ]; then
      if [ "$(echo "${zstd_info}" | wc -l)" -gt 1 ]; then
        zstd_info="$(echo "${zstd_info}" | grep -v ".*-latest.*" | tail -n 1)"
      fi

      zstd_name="$(echo "${zstd_info}" | awk '{ print $1 }')"

      if download "${NETDATA_SOURCE_ARCHIVE_URL_PREFIX}/${zstd_name}" "${zstd_name}"; then
        archive_name="${zstd_name}"
        decompress="${zstd} -dcf"
      else
        warning "Unable to fetch zstd compressed source tarball, trying gzip compressed tarball instead."
      fi
    fi

    if [ -z "${archive_name}" ] && [ -n "${gzip_info}" ]; then
      if [ "$(echo "${gzip_info}" | wc -l)" -gt 1 ]; then
        gzip_info="$(echo "${gzip_info}" | grep -v ".*-latest.*" | tail -n 1)"
      fi

      gzip_name="$(echo "${gzip_info}" | awk '{ print $1 }')"

      if download "${NETDATA_SOURCE_ARCHIVE_URL_PREFIX}/${gzip_name}" "${gzip_name}"; then
        archive_name="${gzip_name}"
        decompress="$(command -v gzip 2>/dev/null) -dc"
      else
        fatal "Unable to find a usable ${SELECTED_RELEASE_CHANNEL} source archive in Manifest file.\n${BADNET_MSG}" F0527
      fi
    fi

    if [ -z "${archive_name}" ]; then
      fatal "Failed to download source tarball for local build. ${BADNET_MSG}." F000B
    fi
  else
    if [ -n "${zstd}" ]; then
      if download "${NETDATA_SOURCE_ARCHIVE_BASEURL}.zst" "${NETDATA_SOURCE_ARCHIVE_BASE_NAME}.zst"; then
        archive_name="${NETDATA_SOURCE_ARCHIVE_BASE_NAME}.zst"
        decompress="${zstd} -dcf"
      else
        rm -f "${NETDATA_SOURCE_ARCHIVE_BASE_NAME}.zst"
        warning "Unable to fetch zstd compressed source tarball, trying gzip compressed tarball instead."
      fi
    fi

    if [ -z "${archive_name}" ]; then
      if download "${NETDATA_SOURCE_ARCHIVE_BASEURL}.gz" "${NETDATA_SOURCE_ARCHIVE_BASE_NAME}.gz"; then
        archive_name="${NETDATA_SOURCE_ARCHIVE_BASE_NAME}.gz"
        decompress="$(command -v gzip 2>/dev/null) -dc"
      fi
    fi

    if [ -z "${archive_name}" ]; then
      fatal "Failed to download source tarball for local build. ${BADNET_MSG}." F000B
    fi

    if ! download "${NETDATA_SOURCE_ARCHIVE_CHECKSUM_URL}" "./sha256sums.txt"; then
      fatal "Failed to download checksums for source tarball verification. ${BADNET_MSG}." F000C
    fi
  fi

  verify_manifest ./ "${archive_name}"

  run sh -c '${0} "${1}" | tar -xf - -C "${2}"' "${decompress}" "${archive_name}" "${tmpdir}"
  rm -rf "${archive_name}" > /dev/null 2>&1

  if [ "${DRY_RUN}" -ne 1 ]; then
    netdata_src_dir="$(find "${tmpdir}" -mindepth 1 -maxdepth 1 -type d -name 'netdata-*' | head -n 1)"
    [ -z "${netdata_src_dir}" ] && fatal "Cannot find netdata source directory in ${tmpdir}" F0006
    cd "${netdata_src_dir}" || fatal "Cannot change directory to netdata source tree" F0006
  fi

  if [ -x netdata-installer.sh ] || [ "${DRY_RUN}" -eq 1 ]; then
    build_and_install || return 1
  else
    # This case is needed because some platforms produce an extra directory on the source tarball extraction.
    if [ "$(find . -mindepth 1 -maxdepth 1 -type d | wc -l)" -eq 1 ] && [ -x "$(find . -mindepth 1 -maxdepth 1 -type d)/netdata-installer.sh" ]; then
      cd "$(find . -mindepth 1 -maxdepth 1 -type d)" &&  build_and_install || return 1
    else
      fatal "Cannot install netdata from source (the source directory does not include netdata-installer.sh)." F0009
    fi
  fi
}

# ======================================================================
# Offline install support code

prepare_offline_install_source() {
  if [ -e "${1}" ]; then
    if [ ! -d "${1}" ]; then
      fatal "${1} is not a directory, unable to prepare offline install source." F0503
    elif [ -n "$(find "${1}/" -mindepth 1 -maxdepth 1 -print)" ]; then
      fatal "${1} already contains files. Either specify a different directory, or remove the files." F052E
    fi
  else
    run mkdir -p "${1}" || fatal "Unable to create target directory for offline install preparation." F0504
  fi

  run cd "${1}" || fatal "Failed to switch to target directory for offline install preparation." F0505

  case "${NETDATA_REQUESTED_INSTALL_TYPE}" in
    static|auto|any)
      set_static_archive_urls "${SELECTED_RELEASE_CHANNEL}" "x86_64"

      if [ "${NETDATA_MANIFEST_INSTALL}" -eq 1 ]; then
        manifest_url="${NETDATA_STATIC_ARCHIVE_URL_PREFIX}/Manifest"
        progress "Fetching ${manifest_url}"
        if ! download "${manifest_url}" "./Manifest" "force"; then
          fatal "Unable to download release Manifest file. ${BADNET_MSG}" F0526
        fi

        if ! download "${NETDATA_STATIC_ARCHIVE_URL_PREFIX}/Version" "./Version"; then
          fatal "Unable to download release Version file. ${BADNET_MSG}" F0526
        fi

        archive_info="$(grep -E "^netdata-.*-.*\\.run " ./Manifest)"

        if [ -z "${archive_info}" ]; then
          warning "Could not find a ${SELECTED_RELEASE_CHANNEL} static build for ${SYSARCH} CPUs."
          return 2
        fi

        archive_names="$(echo "${archive_info}" | awk '{ print $1 }')"

        failed=0

        for f in ${archive_names}; do
          archive_url="${NETDATA_STATIC_ARCHIVE_URL_PREFIX}/${f}"
          progress "Fetching ${archive_url}"
          if ! download "${archive_url}" "${f}"; then
            failed="$((failed + 1))"
            warning "Failed to download ${archive_url}. ${BADNET_MSG}."
          elif echo "${f}" | grep -qvE ".*-latest.*" ; then
            latest_name="$(echo "${f}" | sed -e 's/v.*\.\([a-z]\+\)\.run$/latest.\1.run/' -)"
            ln -sf "${f}" "${latest_name}"
          fi
        done

        if [ "${failed}" -ne 0 ]; then
          fatal "Failed to download one or more static installer archives, cannot continue. ${BADNET_MSG}." F052B
        fi
      else
        check_for_remote_file "${NETDATA_TARBALL_BASEURL}"

        case "$?" in
          0) ;;
          1) NETDATA_ASSUME_REMOTE_FILES_ARE_PRESENT=1 ;;
          *)
            warning "Unable to communicate with GitHub. ${GITHUB_BADNET_MSG}"
            return 1
            ;;
        esac

        for arch in $(echo "${NETDATA_OFFLINE_ARCHES:-${STATIC_INSTALL_ARCHES}}" | awk '{for (i=1;i<=NF;i++) if (!a[$i]++) printf("%s%s",$i,FS)}{printf("\n")}'); do
          set_static_archive_urls "${SELECTED_RELEASE_CHANNEL}" "${arch}"

          if check_for_remote_file "${NETDATA_STATIC_ARCHIVE_URL}"; then
              progress "Fetching ${NETDATA_STATIC_ARCHIVE_URL}"
              if ! download "${NETDATA_STATIC_ARCHIVE_URL}" "${NETDATA_STATIC_ARCHIVE_NAME}"; then
                  warning "Failed to download static installer archive for ${arch}. ${BADNET_MSG}."
              fi
          else
              progress "Skipping ${NETDATA_STATIC_ARCHIVE_URL} as it does not exist on the server."
          fi
        done

        if [ -z "$(find . -name '*.gz.run')" ]; then
          fatal "Did not actually download any static installer archives, cannot continue. ${BADNET_MSG}." F0516
        fi

        progress "Fetching ${NETDATA_STATIC_ARCHIVE_CHECKSUM_URL}"
        if ! download "${NETDATA_STATIC_ARCHIVE_CHECKSUM_URL}" "sha256sums.txt"; then
          fatal "Failed to download checksum file. ${BADNET_MSG}." F0506
        fi
      fi
      ;;
    *) fatal "Install type ${NETDATA_REQUESTED_INSTALL_TYPE} is not supported for offline installs." F051B
  esac

  verify_manifest ./

  if [ "${DRY_RUN}" -ne 1 ]; then
    progress "Preparing install script."
    cat > "install.sh" <<-EOF
	#!/bin/sh
	dir=\$(CDPATH= cd -- "\$(dirname -- "\$0")" && pwd)
	"\${dir}/kickstart.sh" --offline-install-source "\${dir}" \${@}
	EOF
    chmod +x "install.sh"
  else
    progress "Would create install script"
  fi

  if [ "${DRY_RUN}" -ne 1 ]; then
    progress "Copying kickstart script."
    cp "${KICKSTART_SOURCE}" "kickstart.sh"
    chmod +x "kickstart.sh"
  else
    progress "Would copy kickstart.sh to offline install source directory"
  fi

  if [ "${DRY_RUN}" -ne 1 ]; then
    progress "Saving release channel information."
    echo "${SELECTED_RELEASE_CHANNEL}" > "channel"
  else
    progress "Would save release channel information to offline install source directory"
  fi

  if [ "${DRY_RUN}" -ne 1 ]; then
    progress "Generating updated Manifest for offline install source directory."
    rm -f sha256sums.txt
    rm -f Manifest

    for f in *; do
      [ -f "${f}" ] || continue
      h="$(safe_sha256sum "${f}" | awk '{ print $1 }')"
      s="$(wc -c "${f}" | awk '{ print $1 }')"
      echo "${f} ${s} ${h}" >> ./Manifest
      echo "${h} *${f}" >> ./sha256sums.txt
    done
  else
    progress "Would generate updated Manifest for offline install source directory."
  fi

  progress "Finished preparing offline install source directory at ${1}. You can now copy this directory to a target system and then run the script ‘install.sh’ from it to install on that system."
}

# ======================================================================
# Per system-type install logic

install_on_linux() {
  if [ "${NETDATA_REQUESTED_INSTALL_TYPE}" != 'static' ] && [ "${NETDATA_REQUESTED_INSTALL_TYPE}" != 'build' ] && [ -z "${NETDATA_OFFLINE_INSTALL_SOURCE}" ]; then
    SELECTED_INSTALL_METHOD="native"
    try_package_install

    case "$?" in
      0)
        NETDATA_INSTALL_SUCCESSFUL=1
        INSTALL_PREFIX="/"
        ;;
      1) fatal "Unable to install on this system." F0300 ;;
      2)
        case "${NETDATA_REQUESTED_INSTALL_TYPE}" in
          native|auto) fatal "Could not install using native binary packages even though they appear to be available. This usually means something is wrong with your system package manager that requires manual intervention to fix. If you want to install anyway without using native packages, re-run this script with the option '--install-type any'" F0301 ;;
          *) warning "Could not install using native binary packages, falling back to alternative installation method." ;;
        esac
        ;;
      3)
        case "${NETDATA_REQUESTED_INSTALL_TYPE}" in
          native) fatal "Native packages are not available for this system, unable to install." F051C ;;
          *) warning "Native packages are not available for this system, falling back to alternative installation method." ;;
        esac
        ;;
      4)
        case "${NETDATA_REQUESTED_INSTALL_TYPE}" in
          native|auto)
            case "${SELECTED_RELEASE_CHANNEL}" in
              stable) fatal "A stable build with native packages has not yet been published for this system, but is expected to be published within the next few weeks. If you need to install now, you should be able to do so either by using a nightly build (add --release-channel=nightly to the options passed to this script) or by using a static build (add --install-type=static to the options passed to this script). Note that if you use a static build, you may have to reinstall once a stable build is published with native packages for this system to get full support." F051D ;;
              nightly) fatal "A nightly build with native packages has not yet been published for this system, but is expected to be published at 01:00 UTC tomorrow. Please try again tomorrow." F051E ;;
            esac
            ;;
          *) warning "Native packages are not available for this system, falling back to alternative installation method." ;;
        esac
    esac
  fi

  if [ "${NETDATA_REQUESTED_INSTALL_TYPE}" != 'native' ] && [ "${NETDATA_REQUESTED_INSTALL_TYPE}" != 'build' ] && [ -z "${NETDATA_INSTALL_SUCCESSFUL}" ]; then
    SELECTED_INSTALL_METHOD="static"
    INSTALL_TYPE="kickstart-static"
    try_static_install

    case "$?" in
      0)
        NETDATA_INSTALL_SUCCESSFUL=1
        INSTALL_PREFIX="/opt/netdata"
        ;;
      1) fatal "Unable to install on this system." F0302 ;;
      2)
        case "${NETDATA_REQUESTED_INSTALL_TYPE}" in
          static) fatal "Could not install static build." F0303 ;;
          *) warning "Could not install static build, falling back to alternative installation method." ;;
        esac
        ;;
    esac
  fi

  if [ "${NETDATA_REQUESTED_INSTALL_TYPE}" != 'native' ] && [ "${NETDATA_REQUESTED_INSTALL_TYPE}" != 'static' ] && [ -z "${NETDATA_INSTALL_SUCCESSFUL}" ]; then
    SELECTED_INSTALL_METHOD="build"
    INSTALL_TYPE="kickstart-build"
    try_build_install

    case "$?" in
      0) NETDATA_INSTALL_SUCCESSFUL=1 ;;
      *) fatal "Unable to install on this system." F0304 ;;
    esac
  fi
}

install_on_macos() {
  case "${NETDATA_REQUESTED_INSTALL_TYPE}" in
    native) fatal "User requested native package, but native packages are not available for macOS. Try installing without \`--native-only\` option." F0305 ;;
    static) fatal "User requested static build, but static builds are not available for macOS. Try installing without \`--static-only\` option." F0306 ;;
    *)
      SELECTED_INSTALL_METHOD="build"
      INSTALL_TYPE="kickstart-build"
      try_build_install

      case "$?" in
        0) NETDATA_INSTALL_SUCCESSFUL=1 ;;
        *) fatal "Unable to install on this system." F0307 ;;
      esac
      ;;
  esac
}

install_on_freebsd() {
  case "${NETDATA_REQUESTED_INSTALL_TYPE}" in
    native) fatal "User requested native package, but native packages are not available for FreeBSD. Try installing without \`--native-only\` option." F0308 ;;
    static) fatal "User requested static build, but static builds are not available for FreeBSD. Try installing without \`--static-only\` option." F0309 ;;
    *)
      SELECTED_INSTALL_METHOD="build"
      INSTALL_TYPE="kickstart-build"
      try_build_install

      case "$?" in
        0) NETDATA_INSTALL_SUCCESSFUL=1 ;;
        *) fatal "Unable to install on this system." F030A ;;
      esac
      ;;
  esac
}

# ======================================================================
# Argument parsing code

handle_major_version() {
  CONTINUE_INSTALL_PROMPT="Attempting to install will use the latest version available overall. Do you wish to continue the install?"

  if [ -z "${INSTALL_MAJOR_VERSION}" ]; then
    return
  fi

  actual_version="$(get_actual_version "v${INSTALL_MAJOR_VERSION}" "${RELEASE_CHANNEL}")"

  if [ -z "${actual_version}" ]; then
    if [ "${INTERACTIVE}" -eq 0 ]; then
      fatal "Could not determine the lastest releaase in channel '${RELEASE_CHANNEL}' with major version '${INSTALL_MAJOR_VERSION}'" F0517
    else
      if confirm "Unable to determine the correct version to install for major version '${INSTALL_MAJOR_VERSION}'. ${CONTINUE_INSTALL_PROMPT}"; then
        progress "User requested continuing the install with the latest version."
      else
        fatal "Cancelling installation at user request." F0518
      fi
    fi
  elif [ "${actual_version}" = 'NONE' ]; then
    if [ "${INTERACTIVE}" -eq 0 ]; then
      warning "No releases with major version '${INSTALL_MAJOR_VERSION}' have been published. Continuing the install with the latest version instead."
    else
      if confirm "No releases with major version '${INSTALL_MAJOR_VERSION}' have been published. ${CONTINUE_INSTALL_PROMPT}"; then
        progress "User requested continuing the install with the latest version."
      else
        fatal "Cancelling installation at user request." F0519
      fi
    fi
  else
    INSTALL_VERSION="${actual_version}"
  fi
}

validate_args() {
  check_claim_opts

  if [ "${NETDATA_INSTALL_DEM_RUNTIME}" -eq 1 ]; then
    case "${ACTION}" in
      uninstall|claim|repositories-only|prepare-offline)
        fatal "--install-dem-runtime cannot be combined with this action. Use an ordinary install/update. Offline runtime setup uses the installed host helper for static/source installations, or the package manager for native installations." F052F ;;
    esac
    if [ -n "${NETDATA_OFFLINE_INSTALL_SOURCE}" ]; then
      fatal "--install-dem-runtime requires online acquisition. For offline static/source setup, run netdata-dem-runtime.sh install --agent /path/to/netdata --release /absolute/release.json --archive /absolute/runtime.tar.zst as root with the configured offline keyring. Native installations use offline package provisioning." F052F
    fi
  fi

  if [ -n "${NETDATA_REQUESTED_INSTALL_TYPE}" ]; then
    SELECTED_INSTALL_METHOD="${NETDATA_REQUESTED_INSTALL_TYPE}"
  fi

  if [ "${ACTION}" = "repositories-only" ] && [ "${NETDATA_REQUESTED_INSTALL_TYPE}" != "native" ]; then
    fatal "Repositories can only be installed for native installs." F050D
  fi

  if [ -n "${NETDATA_OFFLINE_INSTALL_SOURCE}" ]; then
    case "${NETDATA_REQUESTED_INSTALL_TYPE}" in
      native|build) fatal "Offline installs are only supported for static builds currently." F0502 ;;
    esac

    if [ ! -d "${NETDATA_OFFLINE_INSTALL_SOURCE}" ]; then
      fatal "Offline install source must be a directory." F0519
    fi
  fi

  if [ -n "${LOCAL_BUILD_OPTIONS}" ]; then
    case "${NETDATA_REQUESTED_INSTALL_TYPE}" in
      build) NETDATA_INSTALLER_OPTIONS="${NETDATA_INSTALLER_OPTIONS} ${LOCAL_BUILD_OPTIONS}" ;;
      *) fatal "Specifying local build options is only supported when the --build-only option is also specified." F0401 ;;
    esac
  fi

  if [ -n "${STATIC_INSTALL_OPTIONS}" ]; then
    case "${NETDATA_REQUESTED_INSTALL_TYPE}" in
      static) NETDATA_INSTALLER_OPTIONS="${NETDATA_INSTALLER_OPTIONS} ${STATIC_INSTALL_OPTIONS}" ;;
      *) fatal "Specifying installer options options is only supported when the --static-only option is also specified." F0402 ;;
    esac
  fi

  if [ "${RELEASE_CHANNEL}" = "default" ]; then
    if [ -n "${NETDATA_OFFLINE_INSTALL_SOURCE}" ]; then
      SELECTED_RELEASE_CHANNEL="$(cat "${NETDATA_OFFLINE_INSTALL_SOURCE}/channel")"

      if [ -z "${SELECTED_RELEASE_CHANNEL}" ]; then
        fatal "Could not find a release channel indicator in ${NETDATA_OFFLINE_INSTALL_SOURCE}." F0508
      fi
    else
      SELECTED_RELEASE_CHANNEL="${DEFAULT_RELEASE_CHANNEL}"
    fi
  else
    if [ -n "${NETDATA_OFFLINE_INSTALL_SOURCE}" ] && [ "${RELEASE_CHANNEL}" != "$(cat "${NETDATA_OFFLINE_INSTALL_SOURCE}/channel")" ]; then
      fatal "Release channal '${RELEASE_CHANNEL}' requested, but indicated offline installation source release channel is '$(cat "${NETDATA_OFFLINE_INSTALL_SOURCE}/channel")'." F0509
    fi

    SELECTED_RELEASE_CHANNEL="${RELEASE_CHANNEL}"
  fi

  if [ -n "${INSTALL_MAJOR_VERSION}" ] && [ -n "${INSTALL_VERSION}" ]; then
    fatal "Only one of --install-version or --install-major-version may be specified." F0515
  fi

  handle_major_version # Appropriately updates INSTALL_VERSION if INSTALL_MAJOR_VERSION is set.

  if [ -n "${NETDATA_OFFLINE_INSTALL_SOURCE}" ] && [ -n "${INSTALL_VERSION}" ]; then
      fatal "Specifying an install version alongside an offline install source is not supported." F050A
  fi

  if [ "${NETDATA_AUTO_UPDATES}" = "default" ]; then
    if [ -n "${NETDATA_OFFLINE_INSTALL_SOURCE}" ] || [ -n "${INSTALL_VERSION}" ]; then
      AUTO_UPDATE=0
    else
      AUTO_UPDATE=1
    fi
  elif [ "${NETDATA_INSTALL_MAJOR_VERSION}" ]; then
    warning "Forcibly disabling auto updates as a specific major version was requested."
    AUTO_UPDATE=0
  elif [ "${NETDATA_AUTO_UPDATES}" = 1 ]; then
    AUTO_UPDATE=1
  else
    AUTO_UPDATE=0
  fi
}

set_action() {
  new_action="${1}"

  if [ -n "${ACTION}" ]; then
    warning "Ignoring previously specified '${ACTION}' operation in favor of '${new_action}' specified later on the command line."
  fi

  ACTION="${new_action}"
  NETDATA_COMMAND="${new_action}"
}

parse_args() {
  while [ -n "${1}" ]; do
    case "${1}" in
      "--help")
        usage
        cleanup
        trap - EXIT
        exit 0
        ;;
      "--no-cleanup") NO_CLEANUP=1 ;;
      "--dont-wait"|"--non-interactive") INTERACTIVE=0 ;;
      "--interactive") INTERACTIVE=1 ;;
      "--dry-run") DRY_RUN=1 ;;
      "--install-dem-runtime") NETDATA_INSTALL_DEM_RUNTIME=1 ;;
      "--release-channel")
        RELEASE_CHANNEL="$(echo "${2}" | tr '[:upper:]' '[:lower:]')"
        case "${RELEASE_CHANNEL}" in
          nightly|stable|default) shift 1 ;;
          *)
            echo "Unrecognized value for --release-channel. Valid release channels are: stable, nightly, default"
            exit 1
            ;;
        esac
        ;;
      "--stable-channel") RELEASE_CHANNEL="stable" ;;
      "--nightly-channel") RELEASE_CHANNEL="nightly" ;;
      "--reinstall") set_action 'reinstall' ;;
      "--reinstall-even-if-unsafe") set_action 'unsafe-reinstall' ;;
      "--reinstall-clean") set_action 'reinstall-clean' ;;
      "--uninstall") set_action 'uninstall' ;;
      "--claim-only") set_action 'claim' ;;
      "--no-updates") NETDATA_AUTO_UPDATES=0 ;;
      "--auto-update") NETDATA_AUTO_UPDATES="1" ;;
      "--auto-update-type"|"--auto-update-method")
        NETDATA_AUTO_UPDATE_TYPE="$(echo "${2}" | tr '[:upper:]' '[:lower:]')"
        case "${NETDATA_AUTO_UPDATE_TYPE}" in
          systemd|interval|crontab) shift 1 ;;
          *)
            echo "Unrecognized value for --auto-update-type. Valid values are: systemd, interval, crontab"
            exit 1
            ;;
        esac
        ;;
      "--disable-cloud")
        warning "Cloud cannot be disabled"
        ;;
      "--require-cloud")
        warning "Cloud is always required"
        ;;
      "--dont-start-it")
        NETDATA_INSTALLER_OPTIONS="${NETDATA_INSTALLER_OPTIONS} --dont-start-it"
        ;;
      "--disable-telemetry")
        NETDATA_DISABLE_TELEMETRY="1"
        NETDATA_INSTALLER_OPTIONS="${NETDATA_INSTALLER_OPTIONS} --disable-telemetry"
        ;;
      "--install-prefix")
        p="$(sanitize_path "${2}")"
        if [ "${p}" != "${2}" ]; then
          fatal "Unsafe characters detected in install prefix" F0525
        fi
        INSTALL_PREFIX="${p}"
        shift 1
        ;;
      "--old-install-prefix")
        p="$(sanitize_path "${2}")"
        if [ "${p}" != "${2}" ]; then
          fatal "Unsafe characters detected in old install prefix" F0526
        fi
        OLD_INSTALL_PREFIX="${p}"
        shift 1
        ;;
      "--install-major-version")
        INSTALL_MAJOR_VERSION="${2}"
        shift 1
        ;;
      "--install-version")
        INSTALL_VERSION="${2}"
        AUTO_UPDATE=0
        shift 1
        ;;
      "--distro-override")
        if [ -n "${2}" ]; then
          SKIP_DISTRO_DETECTION=1
          DISTRO="$(echo "${2}" | cut -f 1 -d ':' | tr '[:upper:]' '[:lower:]')"
          SYSVERSION="$(echo "${2}" | cut -f 2 -d ':')"
          SYSCODENAME="$(echo "${2}" | cut -f 3 -d ':' | tr '[:upper:]' '[:lower:]')"

          if [ -z "${SYSVERSION}" ]; then
            fatal "You must specify a release as well as a distribution name." F0510
          fi

          shift 1
        else
          fatal "A distribution name and release must be specified for the --distro-override option." F050F
        fi
        ;;
      "--repositories-only")
        set_action 'repositories-only'
        NETDATA_REQUESTED_INSTALL_TYPE="native"
        ;;
      "--native-only") NETDATA_REQUESTED_INSTALL_TYPE="native" ;;
      "--static-only") NETDATA_REQUESTED_INSTALL_TYPE="static" ;;
      "--build-only") NETDATA_REQUESTED_INSTALL_TYPE="build" ;;
      "--install-type")
        case "${2}" in
          native|static|build|auto|any) NETDATA_REQUESTED_INSTALL_TYPE="${2}" ;;
          *) fatal "${2} is not a recognized install type. Please specify one of native, static, build, auto, or any." F051F ;;
        esac
        shift 1
        ;;
      "--claim-"*)
        optname="$(echo "${1}" | cut -d '-' -f 4-)"
        case "${optname}" in
          url)
            is_valid_url "${2}" "http|https" || fatal "${2} is not a valid claim URL" F0522
            NETDATA_CLAIM_URL="${2}"
            shift 1
            ;;
          proxy)
            is_valid_url "${2}" "http|https|socks|socks5" || fatal "${2} is not a valid claim proxy URL" F0523
            NETDATA_CLAIM_PROXY="${2}"
            shift 1
            ;;
          token) NETDATA_CLAIM_TOKEN="$(sanitize_string "${2}")"; shift 1 ;;
          rooms) NETDATA_CLAIM_ROOMS="$(sanitize_string "${2}")"; shift 1 ;;
          noproxy) NETDATA_CLAIM_PROXY="none" ;;
          insecure) NETDATA_CLAIM_INSECURE=yes ;;
          noreload) NETDATA_CLAIM_NORELOAD=1 ;;
          id|user|hostname)
            NETDATA_CLAIM_EXTRA="${NETDATA_CLAIM_EXTRA} -${optname}=$(sanitize_string "${2}")"
            shift 1
            ;;
          verbose|daemon-not-running) NETDATA_CLAIM_EXTRA="${NETDATA_CLAIM_EXTRA} -${optname}" ;;
          *) warning "Ignoring unrecognized claiming option ${optname}" ;;
        esac
        ;;
      "--local-build-options")
        LOCAL_BUILD_OPTIONS="${LOCAL_BUILD_OPTIONS} $(sanitize_string "${2}")"
        shift 1
        ;;
      "--static-install-options")
        STATIC_INSTALL_OPTIONS="${STATIC_INSTALL_OPTIONS} $(sanitize_string "${2}")"
        shift 1
        ;;
      "--prepare-offline-install-source")
        if [ -n "${2}" ]; then
          set_action 'prepare-offline'
          OFFLINE_TARGET="$(sanitize_string "${2}")"
          shift 1
        else
          fatal "A target directory must be specified with the --prepare-offline-install-source option." F0500
        fi
        ;;
      "--offline-architecture")
        if echo "${STATIC_INSTALL_ARCHES}" | grep -qw "${2}"; then
          NETDATA_OFFLINE_ARCHES="${NETDATA_OFFLINE_ARCHES} ${2}"
        else
          fatal "${2} is not a recognized static build architecture (supported architectures are ${STATIC_INSTALL_ARCHES})" F0518
        fi
        shift 1
        ;;
      "--offline-install-source")
        if [ -d "${2}" ]; then
          NETDATA_OFFLINE_INSTALL_SOURCE="$(canonical_path "${2}")"
          NETDATA_TARBALL_BASEURL="file://${NETDATA_OFFLINE_INSTALL_SOURCE}"
          shift 1
        else
          fatal "A source directory must be specified with the --offline-install-source option." F0501
        fi
        ;;
      "--"|"all"|"--yes"|"-y"|"--force"|"--accept") warning "Option '${1}' is not recognized, ignoring it. ${BADOPT_MSG}" ;;
      *) fatal "Unrecognized option '${1}'. ${BADOPT_MSG}" F050E ;;
    esac
    shift 1
  done

  validate_args
}

# ======================================================================
# Main program

setup_terminal || echo > /dev/null

# shellcheck disable=SC2068
parse_args $@

confirm_root_support
get_system_info
confirm_install_prefix

main
