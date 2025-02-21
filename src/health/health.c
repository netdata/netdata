// SPDX-License-Identifier: GPL-3.0-or-later

#include "health.h"
#include "health_internals.h"

struct health_plugin_globals health_globals = {
    .initialization = {
        .spinlock = SPINLOCK_INITIALIZER,
        .done = false,
    },
    .config = {
        .enabled = true,
        .stock_enabled = true,
        .use_summary_for_notifications = true,

        .health_log_entries_max = HEALTH_LOG_ENTRIES_DEFAULT,
        .health_log_retention_s = HEALTH_LOG_RETENTION_DEFAULT,

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


void health_load_config_defaults(void) {
    static bool done = false;
    if(done) return;
    done = true;

    char filename[FILENAME_MAX + 1];

    health_globals.config.enabled =
        inicfg_get_boolean(&netdata_config, CONFIG_SECTION_HEALTH,
                           "enabled",
                           health_globals.config.enabled);

    health_globals.config.stock_enabled =
        inicfg_get_boolean(&netdata_config, CONFIG_SECTION_HEALTH,
                           "enable stock health configuration",
                           health_globals.config.stock_enabled);

    health_globals.config.use_summary_for_notifications =
        inicfg_get_boolean(&netdata_config, CONFIG_SECTION_HEALTH,
                           "use summary for notifications",
                           health_globals.config.use_summary_for_notifications);

    health_globals.config.default_warn_repeat_every =
        inicfg_get_duration_seconds(&netdata_config, CONFIG_SECTION_HEALTH, "default repeat warning", 0);

    health_globals.config.default_crit_repeat_every =
        inicfg_get_duration_seconds(&netdata_config, CONFIG_SECTION_HEALTH, "default repeat critical", 0);

    health_globals.config.health_log_entries_max =
        inicfg_get_number(&netdata_config, CONFIG_SECTION_HEALTH, "in memory max health log entries",
                          health_globals.config.health_log_entries_max);

    health_globals.config.health_log_retention_s =
        inicfg_get_duration_seconds(&netdata_config, CONFIG_SECTION_HEALTH, "health log retention", HEALTH_LOG_RETENTION_DEFAULT);

    snprintfz(filename, FILENAME_MAX, "%s/alarm-notify.sh", netdata_configured_primary_plugins_dir);
    health_globals.config.default_exec =
        string_strdupz(inicfg_get(&netdata_config, CONFIG_SECTION_HEALTH, "script to execute on alarm", filename));

    health_globals.config.enabled_alerts =
        simple_pattern_create(inicfg_get(&netdata_config, CONFIG_SECTION_HEALTH, "enabled alarms", "*"),
                              NULL, SIMPLE_PATTERN_EXACT, true);

    health_globals.config.run_at_least_every_seconds =
        (int)inicfg_get_duration_seconds(&netdata_config, CONFIG_SECTION_HEALTH, "run at least every",
                                         health_globals.config.run_at_least_every_seconds);

    health_globals.config.postpone_alarms_during_hibernation_for_seconds =
        inicfg_get_duration_seconds(&netdata_config, CONFIG_SECTION_HEALTH,
                                    "postpone alarms during hibernation for",
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
        inicfg_set_number(&netdata_config, CONFIG_SECTION_HEALTH, "in memory max health log entries",
                          (long)health_globals.config.health_log_entries_max);
    }
    else if(health_globals.config.health_log_entries_max > HEALTH_LOG_ENTRIES_MAX) {
        nd_log(NDLS_DAEMON, NDLP_WARNING,
               "Health configuration has invalid max log entries %u, using maximum of %u",
               health_globals.config.health_log_entries_max,
               HEALTH_LOG_ENTRIES_MAX);

        health_globals.config.health_log_entries_max = HEALTH_LOG_ENTRIES_MAX;
        inicfg_set_number(&netdata_config, CONFIG_SECTION_HEALTH, "in memory max health log entries",
                          (long)health_globals.config.health_log_entries_max);
    }

    if (health_globals.config.health_log_retention_s < HEALTH_LOG_MINIMUM_HISTORY) {
        nd_log(NDLS_DAEMON, NDLP_WARNING,
               "Health configuration has invalid health log retention %u. Using minimum %d",
               health_globals.config.health_log_retention_s, HEALTH_LOG_MINIMUM_HISTORY);

        health_globals.config.health_log_retention_s = HEALTH_LOG_MINIMUM_HISTORY;
        inicfg_set_duration_seconds(&netdata_config, CONFIG_SECTION_HEALTH, "health log retention", health_globals.config.health_log_retention_s);
    }

    nd_log(NDLS_DAEMON, NDLP_DEBUG,
           "Health log history is set to %u seconds (%u days)",
           health_globals.config.health_log_retention_s, health_globals.config.health_log_retention_s / 86400);
}

inline const char *health_user_config_dir(void) {
    char buffer[FILENAME_MAX + 1];
    snprintfz(buffer, FILENAME_MAX, "%s/health.d", netdata_configured_user_config_dir);
    return inicfg_get(&netdata_config, CONFIG_SECTION_DIRECTORIES, "health config", buffer);
}

inline const char *health_stock_config_dir(void) {
    char buffer[FILENAME_MAX + 1];
    snprintfz(buffer, FILENAME_MAX, "%s/health.d", netdata_configured_stock_config_dir);
    return inicfg_get(&netdata_config, CONFIG_SECTION_DIRECTORIES, "stock health config", buffer);
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
