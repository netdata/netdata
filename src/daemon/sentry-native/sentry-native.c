// SPDX-License-Identifier: GPL-3.0-or-later

#include "sentry-native.h"
#include "daemon/common.h"

#include "sentry.h"

static bool sentry_telemetry_disabled(void)
{
    char path[FILENAME_MAX + 1];
    sprintf(path, "%s/%s", netdata_configured_user_config_dir, ".opt-out-from-anonymous-statistics");

    struct stat buffer;
    bool opt_out_file_exists = (stat(path, &buffer) == 0);

    if (opt_out_file_exists)
        return true;

    return getenv("DISABLE_TELEMETRY") != NULL;
}

void nd_sentry_init(void)
{
    if (sentry_telemetry_disabled())
        return;

    // path where sentry should save stuff
    char path[FILENAME_MAX];
    snprintfz(path, FILENAME_MAX - 1, "%s/%s", netdata_configured_cache_dir, ".sentry-native");

    sentry_options_t *options = sentry_options_new();
    sentry_options_set_dsn(options, NETDATA_SENTRY_DSN);
    sentry_options_set_database_path(options, path);
    sentry_options_set_environment(options, NETDATA_SENTRY_ENVIRONMENT);

    char release[64];
    snprintfz(release, 64 - 1, "%s.%s.%s",
              NETDATA_VERSION_MINOR, NETDATA_VERSION_PATCH, NETDATA_VERSION_TWEAK);
    sentry_options_set_release(options, release);

    sentry_options_set_dist(options, NETDATA_SENTRY_DIST);
#ifdef NETDATA_INTERNAL_CHECKS
    sentry_options_set_debug(options, 1);
#endif

    sentry_init(options);
}

void nd_sentry_fini(void)
{
    if (sentry_telemetry_disabled())
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
    if (sentry_telemetry_disabled())
        return;

    sentry_value_t crumb = sentry_value_new_breadcrumb("fatal", message);
    sentry_add_breadcrumb(crumb);
}
