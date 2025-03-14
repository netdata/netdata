// SPDX-License-Identifier: GPL-3.0-or-later

#include "sentry-native.h"
#include "daemon/common.h"

#include "sentry.h"

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
