// SPDX-License-Identifier: GPL-3.0-or-later

#include "sentry-native.h"
#include "daemon/common.h"

#include "sentry.h"

bool nd_sentry_crash_report_enabled = false;

void nd_sentry_crash_report(bool enable) {
    nd_sentry_crash_report_enabled = enable;
}

static sentry_value_t nd_sentry_on_crash(
    const sentry_ucontext_t *uctx __maybe_unused, // provides the user-space context of the crash
    sentry_value_t event __maybe_unused,          // used the same way as in `before_send`
    void *closure __maybe_unused                  // user-data that you can provide at configuration time
) {
    // IMPORTANT: this function is called from a signal handler

    if (!nd_sentry_crash_report_enabled)
        return sentry_value_new_null();

    return event;
}

static sentry_value_t nd_sentry_before_send(
    sentry_value_t event __maybe_unused,
    void *hint __maybe_unused,
    void *closure __maybe_unused) {

    if (!nd_sentry_crash_report_enabled)
        return sentry_value_new_null();

    return event;
}

void nd_sentry_init(void)
{
    if (!analytics_check_enabled())
        return;

    // path where sentry should save stuff
    char path[FILENAME_MAX];
    snprintfz(path, FILENAME_MAX - 1, "%s/.sentry-native", netdata_configured_cache_dir);

    sentry_options_t *options = sentry_options_new();
    sentry_options_set_dsn(options, NETDATA_SENTRY_DSN);
    sentry_options_set_database_path(options, path);
    sentry_options_set_environment(options, NETDATA_SENTRY_ENVIRONMENT);

    if(NETDATA_VERSION[0] == 'v')
        sentry_options_set_release(options, &NETDATA_VERSION[1]);
    else
        sentry_options_set_release(options, NETDATA_VERSION);

    sentry_options_set_dist(options, NETDATA_SENTRY_DIST);
#ifdef NETDATA_INTERNAL_CHECKS
    sentry_options_set_debug(options, 1);
#endif

    CLEAN_BUFFER *profile = buffer_create(0, NULL);
    ND_PROFILE_2buffer(profile, nd_profile_detect_and_configure(false), " ");
    sentry_set_tag("profile", buffer_tostring(profile));

    sentry_set_tag("db_mode", rrd_memory_mode_name(default_rrd_memory_mode));

    char tiers[UINT64_MAX_LENGTH];
    print_uint64(tiers, nd_profile.storage_tiers);
    sentry_set_tag("db_tiers", tiers);

    sentry_options_set_on_crash(options, nd_sentry_on_crash, NULL);
    sentry_options_set_before_send(options, nd_sentry_before_send, NULL);

    sentry_init(options);
}

void nd_sentry_fini(void)
{
    if (!analytics_check_enabled())
        return;

    sentry_close();
}

void nd_sentry_set_user(const char *guid)
{
    sentry_value_t user = sentry_value_new_object();
    sentry_value_set_by_key(user, "id", sentry_value_new_string(guid));
    sentry_set_user(user);
}

void nd_sentry_add_breadcrumb(const char *message)
{
    if (!analytics_check_enabled())
        return;

    sentry_value_t crumb = sentry_value_new_breadcrumb("fatal", message);
    sentry_add_breadcrumb(crumb);
}
