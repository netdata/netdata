// SPDX-License-Identifier: GPL-3.0-or-later

#include "netdata-conf-backwards-compatibility.h"
#include "database/engine/rrdengineapi.h"

void netdata_conf_backwards_compatibility(void) {
    // move [global] options to the [web] section

    config_move(CONFIG_SECTION_GLOBAL, "http port listen backlog",
                CONFIG_SECTION_WEB,    "listen backlog");

    config_move(CONFIG_SECTION_GLOBAL, "bind socket to IP",
                CONFIG_SECTION_WEB,    "bind to");

    config_move(CONFIG_SECTION_GLOBAL, "bind to",
                CONFIG_SECTION_WEB,    "bind to");

    config_move(CONFIG_SECTION_GLOBAL, "port",
                CONFIG_SECTION_WEB,    "default port");

    config_move(CONFIG_SECTION_GLOBAL, "default port",
                CONFIG_SECTION_WEB,    "default port");

    config_move(CONFIG_SECTION_GLOBAL, "disconnect idle web clients after seconds",
                CONFIG_SECTION_WEB,    "disconnect idle clients after seconds");

    config_move(CONFIG_SECTION_GLOBAL, "respect web browser do not track policy",
                CONFIG_SECTION_WEB,    "respect do not track policy");

    config_move(CONFIG_SECTION_GLOBAL, "web x-frame-options header",
                CONFIG_SECTION_WEB,    "x-frame-options response header");

    config_move(CONFIG_SECTION_GLOBAL, "enable web responses gzip compression",
                CONFIG_SECTION_WEB,    "enable gzip compression");

    config_move(CONFIG_SECTION_GLOBAL, "web compression strategy",
                CONFIG_SECTION_WEB,    "gzip compression strategy");

    config_move(CONFIG_SECTION_GLOBAL, "web compression level",
                CONFIG_SECTION_WEB,    "gzip compression level");

    config_move(CONFIG_SECTION_GLOBAL,      "config directory",
                CONFIG_SECTION_DIRECTORIES, "config");

    config_move(CONFIG_SECTION_GLOBAL,      "stock config directory",
                CONFIG_SECTION_DIRECTORIES, "stock config");

    config_move(CONFIG_SECTION_GLOBAL,      "log directory",
                CONFIG_SECTION_DIRECTORIES, "log");

    config_move(CONFIG_SECTION_GLOBAL,      "web files directory",
                CONFIG_SECTION_DIRECTORIES, "web");

    config_move(CONFIG_SECTION_GLOBAL,      "cache directory",
                CONFIG_SECTION_DIRECTORIES, "cache");

    config_move(CONFIG_SECTION_GLOBAL,      "lib directory",
                CONFIG_SECTION_DIRECTORIES, "lib");

    config_move(CONFIG_SECTION_GLOBAL,      "home directory",
                CONFIG_SECTION_DIRECTORIES, "home");

    config_move(CONFIG_SECTION_GLOBAL,      "lock directory",
                CONFIG_SECTION_DIRECTORIES, "lock");

    config_move(CONFIG_SECTION_GLOBAL,      "plugins directory",
                CONFIG_SECTION_DIRECTORIES, "plugins");

    config_move(CONFIG_SECTION_HEALTH,      "health configuration directory",
                CONFIG_SECTION_DIRECTORIES, "health config");

    config_move(CONFIG_SECTION_HEALTH,      "stock health configuration directory",
                CONFIG_SECTION_DIRECTORIES, "stock health config");

    config_move(CONFIG_SECTION_REGISTRY,    "registry db directory",
                CONFIG_SECTION_DIRECTORIES, "registry");

    config_move(CONFIG_SECTION_GLOBAL, "debug log",
                CONFIG_SECTION_LOGS,   "debug");

    config_move(CONFIG_SECTION_GLOBAL, "error log",
                CONFIG_SECTION_LOGS,   "error");

    config_move(CONFIG_SECTION_GLOBAL, "access log",
                CONFIG_SECTION_LOGS,   "access");

    config_move(CONFIG_SECTION_GLOBAL, "facility log",
                CONFIG_SECTION_LOGS,   "facility");

    config_move(CONFIG_SECTION_GLOBAL, "errors flood protection period",
                CONFIG_SECTION_LOGS,   "errors flood protection period");

    config_move(CONFIG_SECTION_GLOBAL, "errors to trigger flood protection",
                CONFIG_SECTION_LOGS,   "errors to trigger flood protection");

    config_move(CONFIG_SECTION_GLOBAL, "debug flags",
                CONFIG_SECTION_LOGS,   "debug flags");

    config_move(CONFIG_SECTION_GLOBAL,   "TZ environment variable",
                CONFIG_SECTION_ENV_VARS, "TZ");

    config_move(CONFIG_SECTION_PLUGINS,  "PATH environment variable",
                CONFIG_SECTION_ENV_VARS, "PATH");

    config_move(CONFIG_SECTION_PLUGINS,  "PYTHONPATH environment variable",
                CONFIG_SECTION_ENV_VARS, "PYTHONPATH");

    config_move(CONFIG_SECTION_STATSD,  "enabled",
                CONFIG_SECTION_PLUGINS, "statsd");

    config_move(CONFIG_SECTION_GLOBAL,  "memory mode",
                CONFIG_SECTION_DB,      "db");

    config_move(CONFIG_SECTION_DB,      "mode",
                CONFIG_SECTION_DB,      "db");

    config_move(CONFIG_SECTION_GLOBAL,  "history",
                CONFIG_SECTION_DB,      "retention");

    config_move(CONFIG_SECTION_GLOBAL,  "update every",
                CONFIG_SECTION_DB,      "update every");

    config_move(CONFIG_SECTION_GLOBAL,  "page cache size",
                CONFIG_SECTION_DB,      "dbengine page cache size");

    config_move(CONFIG_SECTION_DB,      "dbengine page cache size MB",
                CONFIG_SECTION_DB,      "dbengine page cache size");

    config_move(CONFIG_SECTION_DB,      "dbengine extent cache size MB",
                CONFIG_SECTION_DB,      "dbengine extent cache size");

    config_move(CONFIG_SECTION_DB,      "page cache size",
                CONFIG_SECTION_DB,      "dbengine page cache size MB");

    config_move(CONFIG_SECTION_GLOBAL,  "page cache uses malloc",
                CONFIG_SECTION_DB,      "dbengine page cache with malloc");

    config_move(CONFIG_SECTION_DB,      "page cache with malloc",
                CONFIG_SECTION_DB,      "dbengine page cache with malloc");

    config_move(CONFIG_SECTION_GLOBAL,  "memory deduplication (ksm)",
                CONFIG_SECTION_DB,      "memory deduplication (ksm)");

    config_move(CONFIG_SECTION_GLOBAL,  "dbengine page fetch timeout",
                CONFIG_SECTION_DB,      "dbengine page fetch timeout secs");

    config_move(CONFIG_SECTION_GLOBAL,  "dbengine page fetch retries",
                CONFIG_SECTION_DB,      "dbengine page fetch retries");

    config_move(CONFIG_SECTION_GLOBAL,  "dbengine extent pages",
                CONFIG_SECTION_DB,      "dbengine pages per extent");

    config_move(CONFIG_SECTION_GLOBAL,  "cleanup obsolete charts after seconds",
                CONFIG_SECTION_DB,      "cleanup obsolete charts after");

    config_move(CONFIG_SECTION_DB,      "cleanup obsolete charts after secs",
                CONFIG_SECTION_DB,      "cleanup obsolete charts after");

    config_move(CONFIG_SECTION_GLOBAL,  "gap when lost iterations above",
                CONFIG_SECTION_DB,      "gap when lost iterations above");

    config_move(CONFIG_SECTION_GLOBAL,  "cleanup orphan hosts after seconds",
                CONFIG_SECTION_DB,      "cleanup orphan hosts after");

    config_move(CONFIG_SECTION_DB,      "cleanup orphan hosts after secs",
                CONFIG_SECTION_DB,      "cleanup orphan hosts after");

    config_move(CONFIG_SECTION_DB,      "cleanup ephemeral hosts after secs",
                CONFIG_SECTION_DB,      "cleanup ephemeral hosts after");

    config_move(CONFIG_SECTION_DB,      "seconds to replicate",
                CONFIG_SECTION_DB,      "replication period");

    config_move(CONFIG_SECTION_DB,      "seconds per replication step",
                CONFIG_SECTION_DB,      "replication step");

    config_move(CONFIG_SECTION_GLOBAL,  "enable zero metrics",
                CONFIG_SECTION_DB,      "enable zero metrics");

    // ----------------------------------------------------------------------------------------------------------------
    // global statistics -> telemetry -> pulse

    config_move(CONFIG_SECTION_PLUGINS, "netdata monitoring",
                CONFIG_SECTION_PLUGINS, "netdata pulse");

    config_move(CONFIG_SECTION_PLUGINS, "netdata telemetry",
                CONFIG_SECTION_PLUGINS, "netdata pulse");

    config_move(CONFIG_SECTION_PLUGINS, "netdata monitoring extended",
                CONFIG_SECTION_PULSE,   "extended");

    config_move("telemetry",            "extended telemetry",
                CONFIG_SECTION_PULSE,   "extended");

    config_move("global statistics",    "update every",
                CONFIG_SECTION_PULSE,   "update every");

    config_move("telemetry",            "update every",
                CONFIG_SECTION_PULSE,   "update every");


    // ----------------------------------------------------------------------------------------------------------------

    bool found_old_config = false;

    if(config_move(CONFIG_SECTION_GLOBAL,  "dbengine disk space",
                    CONFIG_SECTION_DB,      "dbengine tier 0 retention size") != -1)
        found_old_config = true;

    if(config_move(CONFIG_SECTION_GLOBAL,  "dbengine multihost disk space",
                    CONFIG_SECTION_DB,      "dbengine tier 0 retention size") != -1)
        found_old_config = true;

    if(config_move(CONFIG_SECTION_DB,      "dbengine disk space MB",
                    CONFIG_SECTION_DB,      "dbengine tier 0 retention size") != -1)
        found_old_config = true;

    for(size_t tier = 0; tier < RRD_STORAGE_TIERS ;tier++) {
        char old_config[128], new_config[128];

        snprintfz(old_config, sizeof(old_config), "dbengine tier %zu retention days", tier);
        snprintfz(new_config, sizeof(new_config), "dbengine tier %zu retention time", tier);
        config_move(CONFIG_SECTION_DB, old_config,
                    CONFIG_SECTION_DB, new_config);

        if(tier == 0)
            snprintfz(old_config, sizeof(old_config), "dbengine multihost disk space MB");
        else
            snprintfz(old_config, sizeof(old_config), "dbengine tier %zu multihost disk space MB", tier);
        snprintfz(new_config, sizeof(new_config), "dbengine tier %zu retention size", tier);
        if(config_move(CONFIG_SECTION_DB, old_config,
                        CONFIG_SECTION_DB, new_config) != -1 && tier == 0)
            found_old_config = true;

        snprintfz(old_config, sizeof(old_config), "dbengine tier %zu disk space MB", tier);
        snprintfz(new_config, sizeof(new_config), "dbengine tier %zu retention size", tier);
        if(config_move(CONFIG_SECTION_DB, old_config,
                        CONFIG_SECTION_DB, new_config) != -1 && tier == 0)
            found_old_config = true;
    }

    legacy_multihost_db_space = found_old_config;

    // ----------------------------------------------------------------------------------------------------------------

    config_move(CONFIG_SECTION_LOGS,   "error",
                CONFIG_SECTION_LOGS,   "daemon");

    config_move(CONFIG_SECTION_LOGS,   "severity level",
                CONFIG_SECTION_LOGS,   "level");

    config_move(CONFIG_SECTION_LOGS,   "errors to trigger flood protection",
                CONFIG_SECTION_LOGS,   "logs to trigger flood protection");

    config_move(CONFIG_SECTION_LOGS,   "errors flood protection period",
                CONFIG_SECTION_LOGS,   "logs flood protection period");

    config_move(CONFIG_SECTION_HEALTH, "is ephemeral",
                CONFIG_SECTION_GLOBAL, "is ephemeral node");

    config_move(CONFIG_SECTION_HEALTH, "has unstable connection",
                CONFIG_SECTION_GLOBAL, "has unstable connection");

    config_move(CONFIG_SECTION_HEALTH, "run at least every seconds",
                CONFIG_SECTION_HEALTH, "run at least every");

    config_move(CONFIG_SECTION_HEALTH, "postpone alarms during hibernation for seconds",
                CONFIG_SECTION_HEALTH, "postpone alarms during hibernation for");

    config_move(CONFIG_SECTION_HEALTH, "health log history",
                CONFIG_SECTION_HEALTH, "health log retention");

    config_move(CONFIG_SECTION_REGISTRY, "registry expire idle persons days",
                CONFIG_SECTION_REGISTRY, "registry expire idle persons");

    config_move(CONFIG_SECTION_WEB, "disconnect idle clients after seconds",
                CONFIG_SECTION_WEB, "disconnect idle clients after");

    config_move(CONFIG_SECTION_WEB, "accept a streaming request every seconds",
                CONFIG_SECTION_WEB, "accept a streaming request every");

    config_move(CONFIG_SECTION_STATSD, "set charts as obsolete after secs",
                CONFIG_SECTION_STATSD, "set charts as obsolete after");

    config_move(CONFIG_SECTION_STATSD, "disconnect idle tcp clients after seconds",
                CONFIG_SECTION_STATSD, "disconnect idle tcp clients after");

    config_move("plugin:idlejitter", "loop time in ms",
                "plugin:idlejitter", "loop time");

    config_move("plugin:proc:/sys/class/infiniband", "refresh ports state every seconds",
                "plugin:proc:/sys/class/infiniband", "refresh ports state every");
}
