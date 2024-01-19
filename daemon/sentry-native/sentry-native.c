#include "sentry-native.h"
#include "daemon/common.h"

#include "sentry.h"

void sentry_native_init(void)
{
    sentry_options_t *options = sentry_options_new();

    // we should get this from CI (SENTRY_DSN)
    sentry_options_set_dsn(options, NETDATA_SENTRY_DSN);

    // where to save sentry files
    char path[FILENAME_MAX];
    snprintfz(path, FILENAME_MAX - 1, "%s/%s", netdata_configured_cache_dir, ".sentry-native");
    sentry_options_set_database_path(options, path);

    sentry_options_set_auto_session_tracking(options, false);

    // TODO: we should get this from CI (SENTRY_ENVIRONMENT)
    sentry_options_set_environment(options, NETDATA_SENTRY_ENVIRONMENT);

    // TODO: we should get this from CI (SENTRY_RELEASE)
    sentry_options_set_release(options, NETDATA_SENTRY_RELEASE);

    sentry_options_set_dist(options, NETDATA_SENTRY_DIST);
    // TODO: use config_get() to (un)set this
    sentry_options_set_debug(options, 1);

    // TODO: ask @ktsaou/@stelfrag if we want to attach something, eg.
    // sentry_options_add_attachment(options, "/path/to/file");

    sentry_init(options);

    time_t now;
    time(&now);

    char message[1024];
    snprintfz(message, 1024 - 1, "GVD: generated at %s", ctime(&now));

    sentry_capture_event(sentry_value_new_message_event(
        SENTRY_LEVEL_INFO,
        "custom",
        message
    ));
}

void sentry_native_fini(void)
{
    sentry_close();
}
