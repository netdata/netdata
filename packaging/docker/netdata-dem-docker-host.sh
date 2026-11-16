#!/bin/sh
# SPDX-License-Identifier: GPL-3.0-or-later
# Sourced by the image-shipped host helper for the Docker install action.
# shellcheck disable=SC2015
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
docker_close() {
    if [ -n "$docker_watcher" ]; then
        # Only signal our own watcher. It may already have reported EOF and exited.
        if [ "$(awk '/^PPid:/ {print $2}' "/proc/$docker_watcher/status" 2>/dev/null)" = "$$" ]; then
            kill -TERM "$docker_watcher" 2>/dev/null || :
            wait "$docker_watcher" 2>/dev/null || :
        fi
    fi
    rm -rf -- "$docker_tmp"
}
docker_install() {
    docker_name=; docker_expected_id=; docker_dry=no; docker_watch_stdin=no
    while [ "$#" -gt 0 ]; do
        case "$1" in
            --container) [ "$#" -ge 2 ] && [ -z "$docker_name" ] || docker_die 'provide one exact container name'; docker_name=$2; shift 2 ;;
            --container-id) [ "$#" -ge 2 ] || docker_die 'missing container identity'; docker_expected_id=$2; shift 2 ;;
            --cancel-on-stdin-close) docker_watch_stdin=yes; shift ;;
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
    docker_watcher=
    trap docker_close 0
    trap 'exit 1' 1 2 3 15
    if [ "$docker_watch_stdin" = yes ]; then
        # Kickstart owns the sole writer. Notify this shell using our own credentials.
        exec 7<&0
        (IFS= read -r _ || :; [ ! -d "$docker_tmp" ] || kill -TERM "$$") <&7 7<&- >/dev/null 2>&1 &
        docker_watcher=$!
        exec 7<&-
    fi
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
    [ "$docker_id" = "$docker_expected_id" ] || docker_die 'deployment was replaced after image selection; retry for the current deployment'
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
        if (host_docker_policy_helper=$0; host_main policy-prepare --deployment "$docker_token" --runtime-root /var/lib/netdata/dem/runtime); then docker_policy=ready;
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
