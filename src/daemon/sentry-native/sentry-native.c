// SPDX-License-Identifier: GPL-3.0-or-later

#include "sentry-native.h"
#include "daemon/common.h"

#include "sentry.h"

static void nd_sentry_add_deadly_signal_as_breadcrumb(void);

static char sentry_path[FILENAME_MAX] = "";

static bool sentry_initialized = false;
static bool breadcrumb_added = false;
static bool nd_sentry_crash_report_enabled = true;

const char *nd_sentry_path(void) {
    return sentry_path;
}

void nd_sentry_crash_report(bool enable) {
    nd_sentry_crash_report_enabled = enable;
}

// --------------------------------------------------------------------------------------------------------------------
// helpers

static void nd_sentry_set_tag(const char *key, const char *value) {
    if (!value || !*value)
        return;

    sentry_set_tag(key, value);
}

static void nd_sentry_set_tag_int64(const char *key, int64_t value) {
    if(!value)
        return;

    char buf[UINT64_MAX_LENGTH];
    print_int64(buf, value);
    nd_sentry_set_tag(key, buf);
}

static void nd_sentry_set_tag_uint64(const char *key, uint64_t value) {
    if(!value)
        return;

    char buf[UINT64_MAX_LENGTH];
    print_uint64(buf, value);
    nd_sentry_set_tag(key, buf);
}

static inline void nd_sentry_set_tag_uuid(const char *key, const ND_UUID uuid) {
    if(UUIDiszero(uuid))
        return;

    char uuid_str[UUID_STR_LEN];
    uuid_unparse_lower(uuid.uuid, uuid_str);
    nd_sentry_set_tag(key, uuid_str);
}

static void nd_sentry_set_tag_uuid_compact(const char *key, const ND_UUID uuid) {
    if(UUIDiszero(uuid))
        return;

    char uuid_str[UUID_COMPACT_STR_LEN];
    uuid_unparse_lower_compact(uuid.uuid, uuid_str);
    nd_sentry_set_tag(key, uuid_str);
}

static void nd_sentry_set_tag_uptime(void) {
    nd_sentry_set_tag_int64("uptime", now_realtime_sec() - netdata_start_time);
}

static void nd_sentry_set_tag_status(void) {
    nd_sentry_set_tag("status", DAEMON_STATUS_2str(daemon_status_file_get_status()));
}

// --------------------------------------------------------------------------------------------------------------------
// sentry hooks

static sentry_value_t nd_sentry_on_hook(sentry_value_t event) {
    // IMPORTANT: this function is called from a signal handler

    // sentry enables a custom allocator for their use,
    // that is async signal safe, so the sentry API is available here.

    if (!nd_sentry_crash_report_enabled) {
        sentry_value_decref(event);
        return sentry_value_new_null();
    }

    nd_sentry_add_deadly_signal_as_breadcrumb();

    return event;
}

static sentry_value_t nd_sentry_on_crash(
    const sentry_ucontext_t *uctx __maybe_unused,   // provides the user-space context of the crash
    sentry_value_t event,                           // used the same way as in `before_send`
    void *closure __maybe_unused) {                 // user-data that you can provide at configuration time
    return nd_sentry_on_hook(event);
}

static sentry_value_t nd_sentry_before_send(
    sentry_value_t event,
    void *hint __maybe_unused,
    void *closure __maybe_unused) {
    return nd_sentry_on_hook(event);
}

// --------------------------------------------------------------------------------------------------------------------
// sentry initialization

void nd_sentry_init(void) {
    if (!analytics_check_enabled() || sentry_initialized)
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

    nd_cleanup_deadly_signals(); // remove our signal handlers, so that sentry will not hook back to us
    sentry_init(options);
    nd_initialize_signals(true); // add our signal handlers, we will hook back to sentry

    // ----------------------------------------------------------------------------------------------------------------
    // tags

    nd_sentry_set_tag("install_type", daemon_status_file_get_install_type());
    nd_sentry_set_tag("architecture", daemon_status_file_get_architecture());
    nd_sentry_set_tag("virtualization", daemon_status_file_get_virtualization());
    nd_sentry_set_tag("container", daemon_status_file_get_container());
    nd_sentry_set_tag("os_name", daemon_status_file_get_os_name());
    nd_sentry_set_tag("os_version", daemon_status_file_get_os_version());
    nd_sentry_set_tag("os_id", daemon_status_file_get_os_id());
    nd_sentry_set_tag("os_id_like", daemon_status_file_get_os_id_like());
    nd_sentry_set_tag("cloud_provider", daemon_status_file_get_cloud_provider_type());
    nd_sentry_set_tag("cloud_type", daemon_status_file_get_cloud_instance_type());
    nd_sentry_set_tag("cloud_region", daemon_status_file_get_cloud_instance_region());
    nd_sentry_set_tag("timezone", daemon_status_file_get_timezone());

    // profile
    CLEAN_BUFFER *profile = buffer_create(0, NULL);
    ND_PROFILE_2buffer(profile, nd_profile_detect_and_configure(false), " ");
    nd_sentry_set_tag("profile", buffer_tostring(profile));

    // db_mode
    nd_sentry_set_tag("db_mode", rrd_memory_mode_name(default_rrd_memory_mode));

    // db_tiers
    nd_sentry_set_tag_int64("db_tiers", (int64_t)nd_profile.storage_tiers);

    // ephemeral_id
    nd_sentry_set_tag_uuid_compact("ephemeral_id", nd_log_get_invocation_id());

    // agent_events_version
    nd_sentry_set_tag("agent_events_version", TOSTRING(STATUS_FILE_VERSION));

    nd_sentry_set_tag_uint64("restarts", daemon_status_file_get_restarts());
    nd_sentry_set_tag_int64("reliability", daemon_status_file_get_reliability());
    nd_sentry_set_tag("stack_traces", daemon_status_file_get_stack_trace_backend());

    sentry_initialized = true;
}

void nd_sentry_fini(void) {
    if(!sentry_initialized)
        return;

    sentry_close();
}

void nd_sentry_set_user(const char *guid) {
    sentry_value_t user = sentry_value_new_object();
    sentry_value_set_by_key(user, "id", sentry_value_new_string(guid));
    sentry_set_user(user);
}

// --------------------------------------------------------------------------------------------------------------------
// sentry breadcrumbs

static void nd_sentry_add_key_value_charp(sentry_value_t data, const char *key, const char *value) {
    if (!value || !*value)
        return;

    sentry_value_set_by_key(data, key, sentry_value_new_string(value));
}

static void nd_sentry_add_key_value_int64(sentry_value_t data, const char *key, int64_t value) {
    if (!value)
        return;

    char buf[UINT64_MAX_LENGTH];
    print_int64(buf, value);

    sentry_value_set_by_key(data, key, sentry_value_new_string(buf));
}

static void nd_sentry_add_key_value_uint64(sentry_value_t data, const char *key, uint64_t value) {
    if (!value)
        return;

    char buf[UINT64_MAX_LENGTH];
    print_uint64(buf, value);

    sentry_value_set_by_key(data, key, sentry_value_new_string(buf));
}

void nd_sentry_add_breadcrumb(const char *category, const char *message) {
    if(!sentry_initialized || breadcrumb_added)
        return;

    nd_sentry_set_tag_status();
    nd_sentry_set_tag_uptime();

    nd_sentry_set_tag("thread", daemon_status_file_get_fatal_thread());
    nd_sentry_set_tag_uint64("thread_id", daemon_status_file_get_fatal_thread_id());
    nd_sentry_set_tag_uint64("worker_job_id", daemon_status_file_get_fatal_worker_job_id());

    const char *function = daemon_status_file_get_fatal_function();
    if(!function || !*function)
        function = category;

    // Set the transaction name to the function where the error occurred
    // this should be low cardinality
    sentry_set_transaction(function);

    // Set the fingerprint to the function where the error occurred
    sentry_set_fingerprint("{{ default }}", function, NULL);

    sentry_value_t crumb = sentry_value_new_breadcrumb("fatal", message);

    sentry_value_t data = sentry_value_new_object();
    nd_sentry_add_key_value_charp(data, "message", daemon_status_file_get_fatal_message());
    nd_sentry_add_key_value_charp(data, "function", daemon_status_file_get_fatal_function());
    nd_sentry_add_key_value_charp(data, "filename", daemon_status_file_get_fatal_filename());
    nd_sentry_add_key_value_charp(data, "thread", daemon_status_file_get_fatal_thread());
    nd_sentry_add_key_value_uint64(data, "thread_id", daemon_status_file_get_fatal_thread_id());
    nd_sentry_add_key_value_int64(data, "line", daemon_status_file_get_fatal_line());
    nd_sentry_add_key_value_charp(data, "errno", daemon_status_file_get_fatal_errno());
    nd_sentry_add_key_value_charp(data, "stack_trace", daemon_status_file_get_fatal_stack_trace());
    nd_sentry_add_key_value_charp(data, "status", DAEMON_STATUS_2str(daemon_status_file_get_status()));
    nd_sentry_add_key_value_uint64(data, "worker_job_id", daemon_status_file_get_fatal_worker_job_id());

    sentry_value_set_by_key(crumb, "data", data);
    sentry_add_breadcrumb(crumb);
}

void nd_sentry_add_fatal_message_as_breadcrumb(void) {
    nd_sentry_add_breadcrumb("fatal", "fatal message event details");
}

static void nd_sentry_add_deadly_signal_as_breadcrumb(void) {
    nd_sentry_add_breadcrumb("deadly_signal", "deadly signal event details");
}

void nd_sentry_add_shutdown_timeout_as_breadcrumb(void) {
    nd_sentry_add_breadcrumb("shutdown_timeout", "shutdown timeout event details");
}
