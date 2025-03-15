// SPDX-License-Identifier: GPL-3.0-or-later

#include "sentry-native.h"
#include "daemon/common.h"

#include "sentry.h"

static char sentry_path[FILENAME_MAX] = "";

bool nd_sentry_crash_report_enabled = true;

const char *nd_sentry_path(void) {
    return sentry_path;
}

void nd_sentry_crash_report(bool enable) {
    nd_sentry_crash_report_enabled = enable;
}

static sentry_value_t nd_sentry_on_crash(
    const sentry_ucontext_t *uctx __maybe_unused,   // provides the user-space context of the crash
    sentry_value_t event,                           // used the same way as in `before_send`
    void *closure __maybe_unused                    // user-data that you can provide at configuration time
) {
    // IMPORTANT: this function is called from a signal handler

    if (!nd_sentry_crash_report_enabled) {
        sentry_value_decref(event);
        return sentry_value_new_null();
    }

    return event;
}

static sentry_value_t nd_sentry_before_send(
    sentry_value_t event,
    void *hint __maybe_unused,
    void *closure __maybe_unused) {

    if (!nd_sentry_crash_report_enabled) {
        sentry_value_decref(event);
        return sentry_value_new_null();
    }

    return event;
}

void nd_sentry_init(void) {
    if (!analytics_check_enabled())
        return;

    // path where sentry should save stuff
    snprintfz(sentry_path, FILENAME_MAX - 1, "%s/.sentry-native", netdata_configured_cache_dir);

    // ----------------------------------------------------------------------------------------------------------------
    // sentry options

    sentry_options_t *options = sentry_options_new();
    sentry_options_set_dsn(options, NETDATA_SENTRY_DSN);
    sentry_options_set_database_path(options, sentry_path);
    sentry_options_set_environment(options, NETDATA_SENTRY_ENVIRONMENT);

    if(NETDATA_VERSION[0] == 'v')
        sentry_options_set_release(options, &NETDATA_VERSION[1]);
    else
        sentry_options_set_release(options, NETDATA_VERSION);

    sentry_options_set_dist(options, NETDATA_SENTRY_DIST);
#ifdef NETDATA_INTERNAL_CHECKS
    sentry_options_set_debug(options, 1);
#endif

    sentry_options_set_on_crash(options, nd_sentry_on_crash, NULL);
    sentry_options_set_before_send(options, nd_sentry_before_send, NULL);

    // ----------------------------------------------------------------------------------------------------------------
    // initialization

    nd_cleanup_fatal_signals();
    sentry_init(options);
    nd_initialize_signals(true);

    // ----------------------------------------------------------------------------------------------------------------
    // tags

    sentry_set_tag("install_type", daemon_status_file_get_install_type());
    sentry_set_tag("architecture", daemon_status_file_get_architecture());
    sentry_set_tag("virtualization", daemon_status_file_get_virtualization());
    sentry_set_tag("container", daemon_status_file_get_container());
    sentry_set_tag("os_name", daemon_status_file_get_os_name());
    sentry_set_tag("os_version", daemon_status_file_get_os_version());
    sentry_set_tag("os_id", daemon_status_file_get_os_id());
    sentry_set_tag("os_id_like", daemon_status_file_get_os_id_like());

    // profile
    CLEAN_BUFFER *profile = buffer_create(0, NULL);
    ND_PROFILE_2buffer(profile, nd_profile_detect_and_configure(false), " ");
    sentry_set_tag("profile", buffer_tostring(profile));

    // db_mode
    sentry_set_tag("db_mode", rrd_memory_mode_name(default_rrd_memory_mode));

    // db_tiers
    char tiers[UINT64_MAX_LENGTH];
    print_uint64(tiers, nd_profile.storage_tiers);
    sentry_set_tag("db_tiers", tiers);

    // invocation_id
    ND_UUID invocation_id = nd_log_get_invocation_id();
    char invocation_str[UUID_STR_LEN];
    uuid_unparse_lower(invocation_id.uuid, invocation_str);
    sentry_set_tag("invocation_id", invocation_str);

    // agent_events_version
    sentry_set_tag("agent_events_version", TOSTRING(STATUS_FILE_VERSION));
}

void nd_sentry_fini(void) {
    if (!analytics_check_enabled())
        return;

    sentry_close();
}

void nd_sentry_set_user(const char *guid) {
    sentry_value_t user = sentry_value_new_object();
    sentry_value_set_by_key(user, "id", sentry_value_new_string(guid));
    sentry_set_user(user);
}

void nd_sentry_add_fatal_message_as_breadcrumb(void) {
    if (!analytics_check_enabled())
        return;

    const char *function = daemon_status_file_get_fatal_function();
    if(!function || !*function)
        function = "unknown";

    sentry_set_transaction(function);
    // sentry_set_fingerprint("{{ default }}", function, NULL);

    sentry_value_t crumb = sentry_value_new_breadcrumb("fatal", "fatal() event details");

    sentry_value_t data = sentry_value_new_object();
    sentry_value_set_by_key(data, "message", sentry_value_new_string(daemon_status_file_get_fatal_message()));
    sentry_value_set_by_key(data, "function", sentry_value_new_string(daemon_status_file_get_fatal_function()));
    sentry_value_set_by_key(data, "filename", sentry_value_new_string(daemon_status_file_get_fatal_filename()));

    char line[UINT64_MAX_LENGTH];
    print_uint64(line, daemon_status_file_get_fatal_line());
    sentry_value_set_by_key(data, "line", sentry_value_new_string(line));
    sentry_value_set_by_key(data, "errno", sentry_value_new_string(daemon_status_file_get_fatal_errno()));
    sentry_value_set_by_key(data, "stack_trace", sentry_value_new_string(daemon_status_file_get_fatal_stack_trace()));

    sentry_value_set_by_key(crumb, "data", data);
    sentry_add_breadcrumb(crumb);
}
