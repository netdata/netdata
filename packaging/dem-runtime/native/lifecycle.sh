# SPDX-License-Identifier: GPL-3.0-or-later
# Embedded in each maintscript: pre-unpack hooks cannot load new package files.
NATIVE_HELPER=/usr/libexec/netdata/plugins.d/dem.plugin
NATIVE_INTENT_DIR=/run/netdata-dem-runtime
NATIVE_SYSTEMD_DIR=/run/systemd/system
NATIVE_UNIT=netdata.service
NATIVE_APPARMOR_ENABLED=/sys/module/apparmor/parameters/enabled
NATIVE_APPARMOR_PROFILES=/sys/kernel/security/apparmor/profiles
NATIVE_APPARMOR_PROFILE=/etc/apparmor.d/netdata-dem-browser
NATIVE_APPARMOR_DISABLED=/etc/apparmor.d/disable/netdata-dem-browser

native_error() { printf 'netdata-dem-runtime: %s\n' "$*" >&2; }

native_run() {
    printf 'netdata-dem-runtime: run' >&2
    for native_arg do
        # Only fixed lifecycle arguments are safe to log. An unexpected future
        # argument may carry credentials; redact its display, never its value.
        case "$native_arg" in
            "$NATIVE_HELPER"|systemctl|deb-systemd-invoke|rm|runtime|native-prepare|native-configure|apparmor_parser|-r|-R|"$NATIVE_APPARMOR_PROFILE"|--system|start|stop|--|"$NATIVE_UNIT"|"$NATIVE_INTENT_DIR/restart-pending")
                native_quoted=$(printf '%s' "$native_arg" | sed "s/'/'\\\\''/g") ;;
            *) native_quoted='<redacted>' ;;
        esac
        printf " '%s'" "$native_quoted" >&2
    done
    printf '\n' >&2
    if "$@"; then
        return 0
    else
        native_status=$?
        native_error "operation failed in $(pwd): status ${native_status}"
        return "$native_status"
    fi
}

native_environment() {
    if [ -n "${DPKG_ROOT:-}" ] || [ ! -d "$NATIVE_SYSTEMD_DIR" ]; then
        native_error 'a running systemd host is required; chroot/image configuration is not qualified'
        return 1
    fi
    if [ "$(id -u)" != 0 ]; then
        native_error 'package lifecycle requires root'
        return 1
    fi
    if [ ! -x "$NATIVE_HELPER" ]; then
        native_error 'installed native helper API 1 is unavailable'
        return 1
    fi
}

native_state() {
    native_loaded=$(systemctl --system show --property=LoadState --value "$NATIVE_UNIT") || return $?
    case "$native_loaded" in
        loaded|masked) ;;
        *) native_error "cannot use service LoadState '${native_loaded}'"; return 1 ;;
    esac
    native_active=$(systemctl --system show --property=ActiveState --value "$NATIVE_UNIT") || return $?
    case "$native_active" in
        active|inactive|failed) ;;
        *) native_error "cannot infer service safety from ActiveState '${native_active}'"; return 1 ;;
    esac
}

native_intent_directory() {
    if [ ! -e "$NATIVE_INTENT_DIR" ] && [ ! -L "$NATIVE_INTENT_DIR" ]; then
        (umask 077; mkdir "$NATIVE_INTENT_DIR") || return $?
    fi
    if [ -L "$NATIVE_INTENT_DIR" ] || [ ! -d "$NATIVE_INTENT_DIR" ] ||
       [ "$(stat -c '%u:%a' "$NATIVE_INTENT_DIR")" != '0:700' ]; then
        native_error 'restart intent directory must be a plain root-owned 0700 directory'
        return 1
    fi
}

native_read_intent() {
    native_intent=none
    native_marker=$NATIVE_INTENT_DIR/restart-pending
    if [ -e "$native_marker" ] || [ -L "$native_marker" ]; then
        if [ -L "$native_marker" ] || [ ! -f "$native_marker" ] ||
           [ "$(stat -c '%u:%a' "$native_marker")" != '0:600' ]; then
            native_error 'restart intent must be a plain root-owned 0600 file'
            return 1
        fi
        native_intent=$(cat "$native_marker") || return $?
        case "$native_intent" in
            active|inactive) ;;
            *) native_error 'restart intent is incomplete; inspect service state before package recovery'; return 1 ;;
        esac
    fi
}

native_record_intent() {
    native_intent_directory || return $?
    native_read_intent || return $?
    if [ "$native_intent" = none ]; then
        native_original=inactive
        if [ "$native_active" = active ]; then native_original=active; fi
        # Package-manager serialization owns this boot-scoped record. A partial
        # write fails closed instead of treating unknown state as inactive.
        (umask 077; set -C; printf '%s\n' "$native_original" > "$native_marker") || return $?
        native_intent=$native_original
    fi
}

native_service() {
    if [ "$NATIVE_FAMILY" = deb ]; then
        native_run deb-systemd-invoke "$1" "$NATIVE_UNIT"
    else
        native_run systemctl --system "$1" "$NATIVE_UNIT"
    fi
}

native_restore() {
    native_intent_directory || return $?
    native_read_intent || return $?
    if [ "$native_intent" = none ]; then return 0; fi
    if [ "$native_intent" = active ]; then
        native_state || return $?
        if [ "$native_active" != active ]; then
            if native_service start; then :; else
                native_restore_status=$?
                native_error 'service restoration failed; restart is pending'
                return "$native_restore_status"
            fi
            native_state || return $?
            if [ "$native_active" != active ]; then
                native_error 'service start was denied or skipped; activation requires an administrator restart'
                # Do not turn policy denial into an unexpected later start.
                native_run rm -- "$native_marker" || return $?
                return 1
            fi
        fi
    fi
    native_run rm -- "$native_marker"
}

native_prepare() {
    native_environment || return $?
    native_state || return $?
    native_record_intent || return $?
    if [ "$native_active" != inactive ]; then
        if native_service stop; then :; else
            native_prepare_status=$?
            native_restore || :
            return "$native_prepare_status"
        fi
        native_state || return $?
        if [ "$native_active" != inactive ]; then
            native_error 'Agent did not stop (possibly denied by service policy); payload mutation refused'
            native_restore || :
            return 1
        fi
    fi
    if native_run "$NATIVE_HELPER" runtime native-prepare >/dev/null; then
        return 0
    else
        native_prepare_status=$?
        native_restore || :
        return "$native_prepare_status"
    fi
}

native_apparmor() {
    # AppArmor can be absent (for example Rocky's SELinux host). Do not require
    # its tools there; an enabled kernel requires observable policy operations.
    if [ ! -e "$NATIVE_APPARMOR_ENABLED" ]; then return 0; fi
    if [ ! -r "$NATIVE_APPARMOR_ENABLED" ]; then
        native_error 'cannot determine whether AppArmor is enabled'
        return 1
    fi
    read -r native_apparmor_enabled < "$NATIVE_APPARMOR_ENABLED" || return $?
    case "$native_apparmor_enabled" in
        N) return 0 ;;
        Y) ;;
        *) native_error 'unknown AppArmor enabled state'; return 1 ;;
    esac
    if ! command -v apparmor_parser >/dev/null 2>&1; then
        native_error 'active AppArmor requires apparmor_parser; install the host AppArmor tools'
        return 1
    fi
    if [ "$1" = remove ]; then
        # A retry after successful unloading (or reboot with a disabled profile)
        # must not fail merely because the named profile is already absent.
        if [ ! -r "$NATIVE_APPARMOR_PROFILES" ]; then
            native_error 'cannot inspect active AppArmor profiles; removal refused'
            return 1
        fi
        native_apparmor_loaded=no
        while IFS= read -r native_profile; do
            case "$native_profile" in
                'netdata-dem-browser ('*) native_apparmor_loaded=yes ;;
            esac
        done < "$NATIVE_APPARMOR_PROFILES"
        if [ "$native_apparmor_loaded" = no ]; then return 0; fi
        native_run apparmor_parser -R "$NATIVE_APPARMOR_PROFILE"
    else
        if [ -e "$NATIVE_APPARMOR_DISABLED" ] || [ -L "$NATIVE_APPARMOR_DISABLED" ]; then
            native_error 'AppArmor browser profile is administrator-disabled; native admission remains closed'
            return 1
        fi
        # Use the installed conffile and the administrator's parser settings.
        # Never force an ABI, bypass a host override, or replace a local edit.
        native_run apparmor_parser -r "$NATIVE_APPARMOR_PROFILE"
    fi
}

native_configure() {
    # Also stop a RUM-only Agent restored after an earlier failed configuration,
    # so a successful retry activates the newly qualified startup selection.
    native_prepare || return $?
    if native_apparmor load; then :; else
        native_configure_status=$?
        native_error 'AppArmor policy load failed; admission remains closed; restoring eligible non-browser monitoring'
        native_restore || :
        return "$native_configure_status"
    fi
    if native_run "$NATIVE_HELPER" runtime native-configure >/dev/null; then
        native_restore
    else
        native_configure_status=$?
        native_error 'qualification failed; admission remains closed; restoring eligible non-browser monitoring'
        native_restore || :
        return "$native_configure_status"
    fi
}

native_remove() {
    native_prepare || return $?
    if native_apparmor remove; then :; else
        native_remove_status=$?
        native_error 'AppArmor policy removal failed; payload erasure refused'
        native_restore || :
        return "$native_remove_status"
    fi
    # Restore while the helper/Agent dependencies still exist. Admission stays
    # closed throughout later payload removal, including multi-package erasure.
    native_restore
}

native_abort() {
    native_environment || return $?
    # Abort arguments do not prove the payload is intact. Never reopen admission.
    native_restore
}

native_hook() {
    native_kind=$1
    shift
    case "$native_kind:${1:-}" in
        deb-preinst:install|deb-preinst:upgrade|deb-prerm:upgrade|deb-prerm:failed-upgrade) native_prepare ;;
        deb-prerm:remove|deb-prerm:deconfigure) native_remove ;;
        deb-postinst:configure) native_configure ;;
        deb-postinst:abort-upgrade|deb-postinst:abort-remove|deb-postinst:abort-deconfigure|deb-preinst:abort-upgrade) native_abort ;;
        deb-postrm:abort-install|deb-postrm:abort-upgrade|deb-postrm:failed-upgrade)
            # Postrm cannot assume dependencies remain available.
            if [ -x "$NATIVE_HELPER" ] && [ -d "$NATIVE_SYSTEMD_DIR" ]; then native_abort; fi ;;
        deb-postrm:remove|deb-postrm:purge|deb-postrm:upgrade|deb-postrm:disappear) return 0 ;;
        rpm-pre:*) native_prepare ;;
        rpm-preun:0) native_remove ;;
        rpm-preun:*) return 0 ;;
        rpm-posttrans:*) native_configure ;;
        rpm-postun:*) return 0 ;;
        *) native_error "unsupported hook action ${native_kind}:${1:-}"; return 1 ;;
    esac
}
