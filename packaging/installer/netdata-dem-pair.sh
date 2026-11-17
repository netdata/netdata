#!/bin/sh
# SPDX-License-Identifier: GPL-3.0-or-later
# Sourced by the source and static installation owners. No browser is executed.
DP_ACTIVE=0
DP_WORK=
DP_STAGE_CREATED=
DP_STAGE_PARENT=

dem_pair_error() { printf 'netdata-dem-pair: %s\n' "$*" >&2; }
dem_pair_run() {
    printf 'netdata-dem-pair: run' >&2
    dem_pair_redact_next=0
    for dem_pair_argument in "$@"; do
        dem_pair_display=$dem_pair_argument
        if [ "$dem_pair_redact_next" = 1 ]; then
            dem_pair_display='[redacted]'
            dem_pair_redact_next=0
        else
            case "$dem_pair_argument" in
                *://*|*token=*|*password=*|*secret=*|*authorization=*) dem_pair_display='[redacted]' ;;
                --token|--password|--secret|--authorization) dem_pair_redact_next=1 ;;
            esac
        fi
        case "$dem_pair_display" in
            *"'"*) dem_pair_quoted=$(printf '%s' "$dem_pair_display" | sed "s/'/'\\\\''/g") ;;
            *) dem_pair_quoted=$dem_pair_display ;;
        esac
        printf " '%s'" "$dem_pair_quoted" >&2
    done
    printf '\n' >&2
    if "$@"; then return 0; else
        dem_pair_rc=$?
        dem_pair_error "operation failed in $(pwd): status $dem_pair_rc"
        return "$dem_pair_rc"
    fi
}
dem_pair_source_stage_new() {
    # The target helper runs as the service UID, which may not be able to traverse
    # the installer's private TMPDIR. Probe execution without changing its parents.
    dem_pair_stage_rc=1
    for dem_pair_parent in "${TMPDIR:-/tmp}" /tmp /var/tmp; do
        if DP_STAGE_PARENT=$(CDPATH= cd -- "$dem_pair_parent" && pwd -P); then :;
        else dem_pair_stage_rc=$?; continue; fi
        if DP_DEST=$(mktemp -d "$DP_STAGE_PARENT/netdata-pair-stage.XXXXXX"); then :;
        else dem_pair_stage_rc=$?; continue; fi
        DP_STAGE_CREATED=$DP_DEST
        trap 'dem_pair_exit "$?"' EXIT
        trap 'exit 130' INT
        trap 'exit 143' TERM HUP
        if chmod 755 "$DP_DEST" && printf '#!/bin/sh\nexit 0\n' > "$DP_DEST/exec-check" &&
           chmod 755 "$DP_DEST/exec-check" && dem_pair_source_stage_probe; then
            dem_pair_run rm -- "$DP_DEST/exec-check" || return $?
            return 0
        else
            dem_pair_stage_rc=$?
        fi
        dem_pair_cleanup_stage || return $?
    done
    dem_pair_error 'no temporary directory permits target-helper execution as the service account'
    return "$dem_pair_stage_rc"
}
dem_pair_source_stage_probe() {
    if [ -n "${DP_USER:-}" ]; then
        dem_pair_as_user "$DP_DEST/exec-check"
    else
        dem_pair_run "$DP_DEST/exec-check"
    fi
}
dem_pair_stage_target() {
    [ "$DP_TARGET_STAGED" = 0 ] || return 0
    dem_pair_source_stage_new || return $?
    dem_pair_run install -m 755 "$DP_TARGET_TOOL" "$DP_DEST/dem.plugin" || return $?
    DP_TARGET_TOOL=$DP_DEST/dem.plugin
    DP_TOOL=$DP_TARGET_TOOL
    DP_TARGET_STAGED=1
}
dem_pair_cleanup_stage() {
    [ -n "$DP_STAGE_CREATED" ] || return 0
    [ "$DP_STAGE_CREATED" = "${DP_DEST:-}" ] || return 1
    [ -n "$DP_STAGE_PARENT" ] && [ "${DP_STAGE_CREATED%/*}" = "$DP_STAGE_PARENT" ] || return 1
    case "${DP_STAGE_CREATED##*/}" in netdata-pair-stage.*) ;; *) return 1 ;; esac
    dem_pair_stage_suffix=${DP_STAGE_CREATED##*/netdata-pair-stage.}
    [ "${#dem_pair_stage_suffix}" = 6 ] || return 1
    case "$dem_pair_stage_suffix" in *[!a-zA-Z0-9]*) return 1 ;; esac
    [ ! -L "$DP_STAGE_CREATED" ] && [ -d "$DP_STAGE_CREATED" ] || return 1
    # Source installers also run on BSD hosts; use the host's stat spelling.
    dem_pair_stage_uid=$(stat -c %u "$DP_STAGE_CREATED" 2>/dev/null) ||
        dem_pair_stage_uid=$(stat -f %u "$DP_STAGE_CREATED") || return $?
    [ "$dem_pair_stage_uid" = "$(id -u)" ] || return 1
    dem_pair_run rm -rf -- "$DP_STAGE_CREATED" || return $?
    DP_STAGE_CREATED=
}
dem_pair_as_user() (
    exec 9>&-
    if [ "$(id -u)" = 0 ] && [ "$DP_USER" != root ]; then
        dem_pair_run runuser -u "$DP_USER" -- env NETDATA_LIB_DIR="$DP_LIB" \
            NETDATA_USER_CONFIG_DIR="$DP_CONFIG" NETDATA_STOCK_CONFIG_DIR="${DP_STOCK:-$DP_PREFIX/usr/lib/netdata/conf.d}" "$@"
    else
        dem_pair_run env NETDATA_LIB_DIR="$DP_LIB" NETDATA_USER_CONFIG_DIR="$DP_CONFIG" \
            NETDATA_STOCK_CONFIG_DIR="${DP_STOCK:-$DP_PREFIX/usr/lib/netdata/conf.d}" "$@"
    fi
)
dem_pair_runtime() {
    dem_pair_command=$1
    shift
    dem_pair_as_user "$DP_TOOL" runtime "$dem_pair_command" --config-dir "$DP_CONFIG" "$@"
}
dem_pair_legacy_absent() {
    # Narrow transition glue for the known baseline, not a general state parser.
    DP_LEGACY_PLAN=$(dem_pair_runtime setup-plan) || return $?
    dem_pair_old_ifs=$IFS
    IFS='
'
    set -f
    # Split the line protocol with globbing disabled and newline-only IFS.
    # shellcheck disable=SC2086
    set -- $DP_LEGACY_PLAN
    set +f
    IFS=$dem_pair_old_ifs
    [ "$#" = 4 ] && [ "$1" = netdata-dem-runtime-setup-v1 ] && [ "$2" = standalone ] && [ "$3" = managed ] || return 1
    DP_MODE=managed DP_CONSENT=manual DP_CACHE=$4
    case "$DP_CACHE" in /*) ;; *) return 1 ;; esac
    dem_pair_as_user sh -s -- "$DP_CACHE" "$DP_TOOL" "$DP_CONFIG" <<'LEGACY_EMPTY'
root=$1 tool=$2 config=$3
[ -e "$root" ] || { [ ! -L "$root" ]; exit $?; }
[ ! -L "$root" ] && [ -d "$root" ] || exit 1
[ "$(stat -c '%u:%a' "$root")" = "$(id -u):700" ] || exit 1
# Open in the old CLI must not finish initialization or reconcile publication.
for file in layout.json selection.json coordination.lock installation.lock; do
    [ ! -L "$root/$file" ] && [ -f "$root/$file" ] || exit 1
done
printf '%s\n' '{"format":1,"phase":"ready"}' | cmp -s - "$root/layout.json" || exit 1
if printf '%s\n' '{"release":null}' | cmp -s - "$root/selection.json"; then :;
elif printf '%s\n' '{"manifest":null}' | cmp -s - "$root/selection.json"; then :;
else exit 1; fi
# Serialize with first installation, which also holds this existing lock.
exec 8<"$root/installation.lock" || exit $?
flock -n 8 || exit $?
for entry in "$root"/* "$root"/.[!.]* "$root"/..?*; do
    [ -e "$entry" ] || [ -L "$entry" ] || continue
    case "${entry##*/}" in
        layout.json|selection.json|coordination.lock|installation.lock|generations|claims|uses|staging|trash|initializing) ;;
        *) exit 1 ;;
    esac
done
for directory in generations claims uses staging trash initializing; do
    [ ! -L "$root/$directory" ] && [ -d "$root/$directory" ] || exit 1
    entries=$(find "$root/$directory" -mindepth 1 -print -quit) || exit $?
    [ -z "$entries" ] || exit 1
done
scratch=$(mktemp -d /tmp/netdata-dem-empty.XXXXXX) || exit $?
trap 'rm -rf -- "$scratch"' EXIT
printf '%s\n' 'netdata-dem-pair: validating known-empty legacy runtime inventory' >&2
if "$tool" runtime status --config-dir "$config" >/dev/null 2>"$scratch/status-error"; then exit 1;
else status=$?; fi
[ "$status" = 1 ] && printf '%s\n' 'matching browser runtime is not prepared' | cmp -s - "$scratch/status-error" || {
    cat "$scratch/status-error" >&2
    exit 1
}
LEGACY_EMPTY
}

dem_pair_plan() {
    # Arguments: target helper, installed Agent, prefix, source|static,
    # optional target Agent and CLI for repair when installed files are damaged.
    DP_TOOL=$1 DP_AGENT=$2 DP_FLAVOR=$4 DP_ENABLED=0 DP_TARGET_CLI=${6:-}
    DP_PREFIX=$(dem_pair_canonical_path "${3:-/}") || return $?
    DP_PREFIX=${DP_PREFIX%/}
    DP_CONFIG=${DP_PREFIX}/etc/netdata
    DP_LIB=${DP_PREFIX}/var/lib/netdata
    DP_USER=$(id -un)
    [ -e "$DP_AGENT" ] || [ -d "$DP_CONFIG" ] || return 0
    # A retry may be repairing the installed Agent binary. Read the existing
    # configuration with the built target when the owner supplies it.
    DP_QUERY_AGENT=${5:-$DP_AGENT}
    [ -x "$DP_QUERY_AGENT" ] || DP_QUERY_AGENT=$DP_AGENT
    [ -x "$DP_QUERY_AGENT" ] || return 0
    if [ "$(id -u)" = 0 ]; then
        DP_USER=$("$DP_QUERY_AGENT" -c "$DP_CONFIG/netdata.conf" -W get global 'run as user' netdata) || return $?
    fi
    DP_LIB=$("$DP_QUERY_AGENT" -c "$DP_CONFIG/netdata.conf" -W get directories lib "$DP_LIB") || return $?
    DP_CONFIG=$("$DP_QUERY_AGENT" -c "$DP_CONFIG/netdata.conf" -W get directories config "$DP_CONFIG") || return $?
    DP_STOCK=$("$DP_QUERY_AGENT" -c "$DP_CONFIG/netdata.conf" -W get directories 'stock config' "$DP_PREFIX/usr/lib/netdata/conf.d") || return $?
    DP_WEB=$("$DP_QUERY_AGENT" -c "$DP_CONFIG/netdata.conf" -W get global 'web files directory' "$DP_PREFIX/usr/share/netdata/web") || return $?
    DP_TARGET_TOOL=$DP_TOOL
    DP_INSTALLED_TOOL=$DP_PREFIX/usr/libexec/netdata/plugins.d/dem.plugin
    DP_PLAN_READY=0 DP_TARGET_STAGED=0 DP_ACCOUNT_MISSING=0
    if [ "$(id -u)" = 0 ] && ! id -u "$DP_USER" >/dev/null 2>&1; then
        DP_ACCOUNT_MISSING=1
    fi
    # Prefer the installed read-only protocol, including when DEM is disabled in
    # the target build. An older binary may not implement this command.
    if [ "$DP_ACCOUNT_MISSING" = 0 ] && [ -x "$DP_INSTALLED_TOOL" ]; then
        DP_TOOL=$DP_INSTALLED_TOOL
        if DP_PLAN=$(dem_pair_runtime pair-plan 2>/dev/null); then DP_PLAN_READY=1; fi
    fi
    if [ "$DP_PLAN_READY" != 1 ]; then
        if [ -x "$DP_TARGET_TOOL" ]; then
            DP_TOOL=$DP_TARGET_TOOL
            if [ "$DP_ACCOUNT_MISSING" = 1 ]; then
                # Preseeded first installs precede account creation. This read-only
                # fallback may establish absence, never admit existing storage.
                DP_PLAN=$(dem_pair_run env NETDATA_LIB_DIR="$DP_LIB" NETDATA_USER_CONFIG_DIR="$DP_CONFIG" \
                    NETDATA_STOCK_CONFIG_DIR="$DP_STOCK" "$DP_TOOL" runtime pair-plan --config-dir "$DP_CONFIG") || return $?
            else
                dem_pair_stage_target || return $?
                DP_PLAN=$(dem_pair_runtime pair-plan) || return $?
            fi
        elif [ -x "$DP_INSTALLED_TOOL" ]; then
            DP_TOOL=$DP_INSTALLED_TOOL
            dem_pair_legacy_absent || {
                dem_pair_error 'cannot prove this legacy runtime is unused; retain the target DEM helper or repair through the existing installation owner'; return 1;
            }
            DP_TOOL=$DP_TARGET_TOOL
            DP_ENROLLMENT=absent
            return 0
        else
            return 0
        fi
    fi
    DP_TOOL=$DP_TARGET_TOOL
    # Read exactly the bounded owner protocol, never parse JSON with grep.
    dem_pair_old_ifs=$IFS
    IFS='
'
    set -f
    # Split the line protocol with globbing disabled and newline-only IFS.
    # shellcheck disable=SC2086
    set -- $DP_PLAN
    set +f
    IFS=$dem_pair_old_ifs
    [ "$#" = 5 ] && [ "$1" = netdata-dem-pair-plan-v1 ] || {
        dem_pair_error 'invalid runtime pair-plan protocol'; return 1;
    }
    DP_MODE=$2 DP_ENROLLMENT=$3 DP_CONSENT=$4 DP_CACHE=$5
    if [ "$DP_ACCOUNT_MISSING" = 1 ]; then
        case "$DP_CACHE" in /*) ;; *) return 1 ;; esac
        [ -n "$DP_USER" ] && [ "$DP_MODE" = managed ] && [ "$DP_ENROLLMENT" = absent ] &&
            [ ! -e "$DP_CACHE" ] && [ ! -L "$DP_CACHE" ] || {
                dem_pair_error 'missing service account permits only a fresh installation with no runtime storage'; return 1;
            }
        return 0
    fi
    case "$DP_ENROLLMENT" in absent) return 0 ;; enrolled) ;; *) return 1 ;; esac
    [ -x "$DP_TARGET_TOOL" ] || {
        dem_pair_error 'an enrolled runtime requires the target dem.plugin for a paired update'; return 1;
    }
    case "$DP_MODE" in
        managed)
            case "$DP_CONSENT" in automatic|manual) ;; *) return 1 ;; esac
            case "$DP_CACHE" in /*) ;; *) return 1 ;; esac
            ;;
        preloaded)
            [ "${NETDATA_DEM_PRELOAD_UPDATE:-0}" = 1 ] && [ "${DP_DONT_START:-0}" = 1 ] || {
                dem_pair_error 'preload deployment requires NETDATA_DEM_PRELOAD_UPDATE=1 and --dont-start-it; the deployment owner retains configuration and starts the pair'; return 1;
            }
            ;;
        *) dem_pair_error 'enrolled native runtime requires its package owner and matching target payload'; return 1 ;;
    esac
    dem_pair_stage_target || return $?
    DP_ENABLED=1
}
dem_pair_state_location() {
    # Serialize installation owners without storing Agent recovery payloads.
    DP_KEY=$(printf '%s' "${DP_PREFIX:-/}" | sha256sum) || return $?
    DP_KEY=${DP_KEY%% *}
    if [ "$(id -u)" = 0 ]; then
        DP_STATE=/var/lib/netdata-dem-pair/$DP_KEY
    else
        DP_STATE=$DP_PREFIX/.netdata-dem-pair
    fi
    # Test fixtures use a private directory, never a live service path.
    DP_STATE=${NETDATA_DEM_PAIR_STATE_DIR:-$DP_STATE}
    case "$DP_STATE" in /*) ;; *) return 1 ;; esac
}
dem_pair_canonical_path() (
    dem_pair_path=${1%/}
    [ -n "$dem_pair_path" ] || dem_pair_path=/
    case "$dem_pair_path" in /*) ;; *) return 1 ;; esac
    case "$dem_pair_path" in /) ;; *)
        case "$dem_pair_path/" in *//*|*/./*|*/../*) return 1 ;; esac
    esac
    dem_pair_text=$(printf '%s' "$dem_pair_path" | tr -d '\000-\037\177')
    [ "$dem_pair_text" = "$dem_pair_path" ] || return 1
    dem_pair_suffix=
    while [ ! -d "$dem_pair_path" ]; do
        [ ! -L "$dem_pair_path" ] || return 1
        dem_pair_suffix=/${dem_pair_path##*/}$dem_pair_suffix
        dem_pair_path=${dem_pair_path%/*}
        [ -n "$dem_pair_path" ] || dem_pair_path=/
    done
    dem_pair_physical=$(cd "$dem_pair_path" && pwd -P) || return $?
    dem_pair_canonical=${dem_pair_physical%/}$dem_pair_suffix
    printf '%s\n' "${dem_pair_canonical:-/}"
)
dem_pair_state() {
    dem_pair_state_location || return $?
    dem_pair_saved_umask=$(umask)
    umask 022
    dem_pair_run mkdir -p "$DP_STATE"
    dem_pair_mkdir_status=$?
    umask "$dem_pair_saved_umask"
    [ "$dem_pair_mkdir_status" = 0 ] || return "$dem_pair_mkdir_status"
    [ ! -L "$DP_STATE" ] && [ "$(stat -c %u "$DP_STATE")" = "$(id -u)" ] || return 1
    [ $((0$(stat -c %a "$DP_STATE") & 022)) = 0 ] || return 1
    dem_pair_parent=$DP_STATE
    while [ "$dem_pair_parent" != / ]; do
        [ ! -L "$dem_pair_parent" ] || return 1
        dem_pair_parent_uid=$(stat -c %u "$dem_pair_parent") || return $?
        [ "$dem_pair_parent_uid" = "$(id -u)" ] || [ "$dem_pair_parent_uid" = 0 ] || return 1
        dem_pair_permissions=$(stat -c %a "$dem_pair_parent") || return $?
        # A sticky root temporary directory is a safe ancestor for private test
        # or nonroot prefix state; other writable ancestors are not trusted.
        if [ $((0$dem_pair_permissions & 022)) != 0 ]; then
            [ "$dem_pair_parent_uid" = 0 ] && [ $((0$dem_pair_permissions & 01000)) != 0 ] || {
                dem_pair_error "installation lock ancestor is group/world writable: $dem_pair_parent (uid=$dem_pair_parent_uid mode=$dem_pair_permissions); review its permissions before retrying"
                return 1
            }
        fi
        dem_pair_parent=${dem_pair_parent%/*}
        [ -n "$dem_pair_parent" ] || dem_pair_parent=/
    done
    # flock is already required by the installed runtime host-policy helper.
    exec 9>"$DP_STATE/lock" || return $?
    flock -n 9 || { dem_pair_error 'another paired installation is running'; return 1; }
    chmod 755 "$DP_STATE" || return $?
}
dem_pair_capture_ipc() {
    # Read only exported values, as received by the actual CLI/Agent children.
    DP_IPC_CONTEXT=$(sh -c '
        for value in "${NETDATA_PIPENAME+set:$NETDATA_PIPENAME}" "${NETDATA_RUN_DIR+set:$NETDATA_RUN_DIR}"; do
            clean=$(printf "%s" "$value" | tr -d "\000-\037\177")
            [ "$clean" = "$value" ] || exit 1
            printf "%s\n" "${value:-unset}"
        done
    ') || { dem_pair_error 'IPC environment cannot contain control characters'; return 1; }
    DP_IPC_PIPE=${DP_IPC_CONTEXT%%'
'*}
    DP_IPC_RUN=${DP_IPC_CONTEXT#*'
'}
    dem_pair_validate_ipc
}
dem_pair_validate_ipc() {
    for dem_pair_value in "$DP_IPC_PIPE" "$DP_IPC_RUN"; do
        case "$dem_pair_value" in
            unset|set:|set:/*) ;;
            *) dem_pair_error 'paired installation requires absolute nonempty NETDATA_PIPENAME and NETDATA_RUN_DIR values'; return 1 ;;
        esac
    done
}
dem_pair_restore_ipc() {
    case "$DP_IPC_PIPE" in
        unset) unset NETDATA_PIPENAME ;;
        set:*) NETDATA_PIPENAME=${DP_IPC_PIPE#set:}; export NETDATA_PIPENAME ;;
    esac
    case "$DP_IPC_RUN" in
        unset) unset NETDATA_RUN_DIR ;;
        set:*) NETDATA_RUN_DIR=${DP_IPC_RUN#set:}; export NETDATA_RUN_DIR ;;
    esac
}
dem_pair_begin() {
    [ "${DP_ENABLED:-0}" = 1 ] || return 0
    # Normal postinstall changes ownership/modes recursively in these trees.
    # Runtime proof and administrator state must not be inside them.
    for dem_pair_protected in "$DP_CONFIG" "$DP_LIB" "$DP_CACHE"; do
        dem_pair_protected=$(dem_pair_canonical_path "$dem_pair_protected") || return $?
        for dem_pair_tree in "$DP_PREFIX/usr/libexec/netdata" "$DP_PREFIX/usr/share/netdata/web" "${DP_WEB:-$DP_PREFIX/usr/share/netdata/web}"; do
            dem_pair_tree=$(dem_pair_canonical_path "$dem_pair_tree") || return $?
            case "$dem_pair_protected/" in "$dem_pair_tree/"*)
                dem_pair_error 'runtime/configuration state overlaps an Agent postinstall tree'; return 1 ;;
            esac
            case "$dem_pair_tree/" in "$dem_pair_protected/"*)
                dem_pair_error 'runtime/configuration state contains an Agent postinstall tree'; return 1 ;;
            esac
        done
    done
    dem_pair_state || return $?
    # Both source and static use transient staging. Only runtime generations and
    # their ownership records survive the attempt; Agent files have no snapshot.
    if [ -z "$DP_STAGE_CREATED" ]; then dem_pair_source_stage_new || return $?; fi
    DP_WORK=$DP_DEST
    dem_pair_capture_ipc || return $?
    if [ "$DP_MODE" = managed ]; then
        set -- --config-dir "$DP_CONFIG" --output "$DP_WORK/inputs"
        if [ -n "${NETDATA_DEM_RELEASE:-}" ] && [ -n "${NETDATA_DEM_ARCHIVE:-}" ]; then
            set -- "$@" --release "$NETDATA_DEM_RELEASE" --archive "$NETDATA_DEM_ARCHIVE"
            if [ -n "${NETDATA_DEM_KEYRING:-}" ]; then set -- "$@" --keyring "$NETDATA_DEM_KEYRING"; fi
        elif [ "$DP_CONSENT" = automatic ] || [ "${NETDATA_DEM_ALLOW_DOWNLOAD:-0}" = 1 ]; then
            set -- "$@" --allow-download
        else
            dem_pair_error 'manual runtime updates require NETDATA_DEM_RELEASE and NETDATA_DEM_ARCHIVE, or NETDATA_DEM_ALLOW_DOWNLOAD=1'
            return 1
        fi
        # Verify all inputs before stopping a working Agent. The installer owns
        # these files so the service account cannot replace the trust keyring.
        dem_pair_run env NETDATA_LIB_DIR="$DP_LIB" NETDATA_STOCK_CONFIG_DIR="$DP_STOCK" \
            "$DP_TOOL" runtime fetch "$@" || return $?
        chmod 755 "$DP_WORK/inputs" && chmod 644 "$DP_WORK/inputs/"* || return $?
    else
        dem_pair_runtime preload-validate || return $?
    fi
    DP_WAS_RUNNING=0
    if dem_pair_owner_running; then DP_WAS_RUNNING=1; else
        dem_pair_intent_status=$?
        [ "$dem_pair_intent_status" = 1 ] || return "$dem_pair_intent_status"
    fi
    if [ "$DP_MODE" = preloaded ] && [ "$DP_WAS_RUNNING" != 0 ]; then
        dem_pair_error 'preload deployment requires the Agent already stopped by its deployment owner'
        return 1
    fi
    if [ "$(id -u)" = 0 ] && [ -x "$DP_PREFIX/usr/libexec/netdata/install-service.sh" ]; then
        dem_pair_run "$DP_PREFIX/usr/libexec/netdata/install-service.sh" \
            --save-cmds "$DP_WORK/service" --cmds-only || return $?
        chmod 600 "$DP_WORK/service" || return $?
    fi
    dem_pair_record_executables || return $?
    DP_ACTIVE=1
    if [ "$DP_MODE" = managed ]; then dem_pair_owner_stop || return $?; fi
    dem_pair_wait_stopped 60 || return $?
    if [ "$DP_MODE" = managed ]; then
        dem_pair_runtime prepare --cache "$DP_CACHE" --release "$DP_WORK/inputs/release.json" \
            --archive "$DP_WORK/inputs/runtime.tar.zst" --keyring "$DP_WORK/inputs/release-public.gpg" || return $?
    fi
}
dem_pair_finish() {
    [ "${DP_ACTIVE:-0}" = 1 ] || return 0
    dem_pair_restore_ipc
    if [ "$DP_MODE" = managed ]; then
        # Activation also validates the exact selection when prepare was unchanged.
        dem_pair_runtime activate --cache "$DP_CACHE" || return $?
        if [ "$DP_WAS_RUNNING" = 1 ] && [ "${DP_DONT_START:-0}" != 1 ]; then
            dem_pair_owner_start || return $?
        fi
    else
        dem_pair_runtime preload-validate || return $?
        dem_pair_error 'Matching preload installed; the deployment owner restores service intent.'
    fi
    DP_ACTIVE=0
}
dem_pair_exit() {
    dem_pair_original_status=$1
    trap - EXIT HUP INT TERM
    if [ "${DP_ACTIVE:-0}" = 1 ]; then
        dem_pair_error 'Agent update incomplete; no Agent files were restored. Repair or rerun the same Agent installation before starting it. Runtime selection and ownership records are preserved; inspect runtime status for pending activation or rollback.'
    fi
    if dem_pair_cleanup_stage; then :; else
        dem_pair_cleanup_status=$?
        dem_pair_error 'could not remove the verified installation staging directory'
        [ "$dem_pair_original_status" != 0 ] || dem_pair_original_status=$dem_pair_cleanup_status
    fi
    exit "$dem_pair_original_status"
}

dem_pair_owner_running() {
    case "$DP_FLAVOR" in static) dem_pair_cli=$DP_PREFIX/bin/netdatacli ;; *) dem_pair_cli=$DP_PREFIX/usr/sbin/netdatacli ;; esac
    dem_pair_cli=${DP_TARGET_CLI:-$dem_pair_cli}
    [ -x "$dem_pair_cli" ] || { dem_pair_error 'netdatacli is required to observe service intent'; return 2; }
    if "$dem_pair_cli" ping >/dev/null 2>&1; then return 0; else
        dem_pair_ping=$?
        case "$dem_pair_ping" in 1) return 0 ;; 255) return 1 ;; *) return "$dem_pair_ping" ;; esac
    fi
}
dem_pair_owner_stop() {
    if [ "$(id -u)" = 0 ] && [ -f "$DP_WORK/service" ]; then
        # Trusted service command record created by this installation owner.
        # shellcheck source=/dev/null
        . "$DP_WORK/service"
    fi
    if [ "$(id -u)" = 0 ] && [ -n "${NETDATA_STOP_CMD:-}" ]; then
        # Same trusted command protocol used by functions.sh; never eval data.
        # shellcheck disable=SC2086
        dem_pair_run ${NETDATA_STOP_CMD} || return $?
    elif dem_pair_owner_running; then
        if dem_pair_run "$dem_pair_cli" shutdown-agent; then :; else
            dem_pair_stop_status=$?
            # CMD_EXIT can close IPC before replying. Both callers must still
            # prove executable drainage with dem_pair_wait_stopped before writes.
            [ "$dem_pair_stop_status" = 255 ] || return "$dem_pair_stop_status"
        fi
    else
        dem_pair_stop_status=$?
        [ "$dem_pair_stop_status" = 1 ] || return "$dem_pair_stop_status"
    fi
}
dem_pair_record_executables() {
    : > "$DP_WORK/old-executables"
    if [ -e "$DP_AGENT" ]; then
        stat -Lc '%d:%i' "$DP_AGENT" >> "$DP_WORK/old-executables" || return $?
    fi
    if [ -e "$DP_PREFIX/usr/libexec/netdata/plugins.d/dem.plugin" ]; then
        stat -Lc '%d:%i' "$DP_PREFIX/usr/libexec/netdata/plugins.d/dem.plugin" >> "$DP_WORK/old-executables" || return $?
    fi
}
dem_pair_wait_stopped() {
    [ -d /proc/1 ] || { dem_pair_error 'paired switching requires Linux process identity checks'; return 1; }
    # Observe this installation only; never match or stop processes by name.
    dem_pair_inodes=$(cat "$DP_WORK/old-executables"; stat -Lc '%d:%i' "$DP_AGENT" \
        "$DP_PREFIX/usr/libexec/netdata/plugins.d/dem.plugin" 2>/dev/null)
    dem_pair_attempt=0
    while [ "$dem_pair_attempt" -lt "${1:-60}" ]; do
        dem_pair_found=0
        for dem_pair_proc in /proc/[0-9]*/exe; do
            dem_pair_inode=$(stat -Lc '%d:%i' "$dem_pair_proc" 2>/dev/null) || continue
            if printf '%s\n' "$dem_pair_inodes" | grep -Fxq "$dem_pair_inode"; then
                dem_pair_found=1
                break
            fi
        done
        [ "$dem_pair_found" = 1 ] || return 0
        sleep 1
        dem_pair_attempt=$((dem_pair_attempt + 1))
    done
    dem_pair_error 'this installation still has a live Agent/DEM executable; stop its consumers before retrying'
    return 75
}
dem_pair_owner_start() (
    exec 9>&-
    if [ "$(id -u)" = 0 ] && [ -f "$DP_WORK/service" ]; then
        # shellcheck source=/dev/null
        . "$DP_WORK/service"
    fi
    if [ "$(id -u)" = 0 ] && [ -n "${NETDATA_START_CMD:-}" ]; then
        # Same trusted command protocol used by functions.sh; never eval data.
        # shellcheck disable=SC2086
        dem_pair_run ${NETDATA_START_CMD}
    else
        dem_pair_run "$DP_AGENT"
    fi
)
