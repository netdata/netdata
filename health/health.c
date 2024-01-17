// SPDX-License-Identifier: GPL-3.0-or-later

#include "health.h"
#include "health_internals.h"

struct health_plugin_globals health_globals = {
    .initialization = {
        .spinlock = NETDATA_SPINLOCK_INITIALIZER,
        .done = false,
    },
    .config = {
        .enabled = true,
        .stock_enabled = true,
        .use_summary_for_notifications = true,

        .health_log_entries_max = HEALTH_LOG_ENTRIES_DEFAULT,
        .health_log_history = HEALTH_LOG_HISTORY_DEFAULT,

        .default_warn_repeat_every = 0,
        .default_crit_repeat_every = 0,

        .run_at_least_every_seconds = 10,
        .postpone_alarms_during_hibernation_for_seconds = 60,
    },
    .prototypes = {
        .dict = NULL,
    }
};

bool health_plugin_enabled(void) {
    return health_globals.config.enabled;
}

void health_plugin_disable(void) {
    health_globals.config.enabled = false;
}


static void health_load_config_defaults(void) {
    char filename[FILENAME_MAX + 1];

    health_globals.config.enabled =
        config_get_boolean(CONFIG_SECTION_HEALTH,
                           "enabled",
                           health_globals.config.enabled);

    health_globals.config.stock_enabled =
        config_get_boolean(CONFIG_SECTION_HEALTH,
                           "enable stock health configuration",
                           health_globals.config.stock_enabled);

    health_globals.config.use_summary_for_notifications =
        config_get_boolean(CONFIG_SECTION_HEALTH,
                           "use summary for notifications",
                           health_globals.config.use_summary_for_notifications);

    health_globals.config.default_warn_repeat_every =
        config_get_duration(CONFIG_SECTION_HEALTH, "default repeat warning", "never");

    health_globals.config.default_crit_repeat_every =
        config_get_duration(CONFIG_SECTION_HEALTH, "default repeat critical", "never");

    health_globals.config.health_log_entries_max =
        config_get_number(CONFIG_SECTION_HEALTH, "in memory max health log entries",
                          health_globals.config.health_log_entries_max);

    health_globals.config.health_log_history =
        config_get_number(CONFIG_SECTION_HEALTH, "health log history", HEALTH_LOG_DEFAULT_HISTORY);

    snprintfz(filename, FILENAME_MAX, "%s/alarm-notify.sh", netdata_configured_primary_plugins_dir);
    health_globals.config.default_exec =
        string_strdupz(config_get(CONFIG_SECTION_HEALTH, "script to execute on alarm", filename));

    health_globals.config.enabled_alerts =
        simple_pattern_create(config_get(CONFIG_SECTION_HEALTH, "enabled alarms", "*"),
                              NULL, SIMPLE_PATTERN_EXACT, true);

    health_globals.config.run_at_least_every_seconds =
        (int)config_get_number(CONFIG_SECTION_HEALTH,
                               "run at least every seconds",
                               health_globals.config.run_at_least_every_seconds);

    health_globals.config.postpone_alarms_during_hibernation_for_seconds =
        config_get_number(CONFIG_SECTION_HEALTH,
                          "postpone alarms during hibernation for seconds",
                          health_globals.config.postpone_alarms_during_hibernation_for_seconds);

    health_globals.config.default_recipient =
        string_strdupz("root");

    // ------------------------------------------------------------------------
    // verify after loading

    if(health_globals.config.run_at_least_every_seconds < 1)
        health_globals.config.run_at_least_every_seconds = 1;

    if(health_globals.config.health_log_entries_max < HEALTH_LOG_ENTRIES_MIN) {
        nd_log(NDLS_DAEMON, NDLP_WARNING,
               "Health configuration has invalid max log entries %u, using minimum of %u",
               health_globals.config.health_log_entries_max,
               HEALTH_LOG_ENTRIES_MIN);

        health_globals.config.health_log_entries_max = HEALTH_LOG_ENTRIES_MIN;
        config_set_number(CONFIG_SECTION_HEALTH, "in memory max health log entries",
                          (long)health_globals.config.health_log_entries_max);
    }
    else if(health_globals.config.health_log_entries_max > HEALTH_LOG_ENTRIES_MAX) {
        nd_log(NDLS_DAEMON, NDLP_WARNING,
               "Health configuration has invalid max log entries %u, using maximum of %u",
               health_globals.config.health_log_entries_max,
               HEALTH_LOG_ENTRIES_MAX);

        health_globals.config.health_log_entries_max = HEALTH_LOG_ENTRIES_MAX;
        config_set_number(CONFIG_SECTION_HEALTH, "in memory max health log entries",
                          (long)health_globals.config.health_log_entries_max);
    }

    if (health_globals.config.health_log_history < HEALTH_LOG_MINIMUM_HISTORY) {
        nd_log(NDLS_DAEMON, NDLP_WARNING,
               "Health configuration has invalid health log history %u. Using minimum %d",
               health_globals.config.health_log_history, HEALTH_LOG_MINIMUM_HISTORY);

        health_globals.config.health_log_history = HEALTH_LOG_MINIMUM_HISTORY;
        config_set_number(CONFIG_SECTION_HEALTH, "health log history", health_globals.config.health_log_history);
    }

    nd_log(NDLS_DAEMON, NDLP_DEBUG,
           "Health log history is set to %u seconds (%u days)",
           health_globals.config.health_log_history, health_globals.config.health_log_history / 86400);
}

inline char *health_user_config_dir(void) {
    char buffer[FILENAME_MAX + 1];
    snprintfz(buffer, FILENAME_MAX, "%s/health.d", netdata_configured_user_config_dir);
    return config_get(CONFIG_SECTION_DIRECTORIES, "health config", buffer);
}

inline char *health_stock_config_dir(void) {
    char buffer[FILENAME_MAX + 1];
    snprintfz(buffer, FILENAME_MAX, "%s/health.d", netdata_configured_stock_config_dir);
    return config_get(CONFIG_SECTION_DIRECTORIES, "stock health config", buffer);
}

void health_plugin_init(void) {
    spinlock_lock(&health_globals.initialization.spinlock);

    if(health_globals.initialization.done)
        goto cleanup;

    health_globals.initialization.done = true;

    health_init_prototypes();
    health_load_config_defaults();

    if(!health_plugin_enabled())
        goto cleanup;

    health_reload_prototypes();
    health_silencers_init();

cleanup:
    spinlock_unlock(&health_globals.initialization.spinlock);
}

void health_plugin_destroy(void) {
    ;
}

void health_plugin_reload(void) {
    health_reload_prototypes();
    health_apply_prototypes_to_all_hosts();
}
