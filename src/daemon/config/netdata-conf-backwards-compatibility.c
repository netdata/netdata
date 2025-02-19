// SPDX-License-Identifier: GPL-3.0-or-later

#include "netdata-conf-backwards-compatibility.h"
#include "database/engine/rrdengineapi.h"

void netdata_conf_backwards_compatibility(void) {
    FUNCTION_RUN_ONCE();

    // move [global] options to the [web] section

    inicfg_move(&netdata_config, CONFIG_SECTION_GLOBAL, "http port listen backlog",
                CONFIG_SECTION_WEB,    "listen backlog");

    inicfg_move(&netdata_config, CONFIG_SECTION_GLOBAL, "bind socket to IP",
                CONFIG_SECTION_WEB,    "bind to");

    inicfg_move(&netdata_config, CONFIG_SECTION_GLOBAL, "bind to",
                CONFIG_SECTION_WEB,    "bind to");

    inicfg_move(&netdata_config, CONFIG_SECTION_GLOBAL, "port",
                CONFIG_SECTION_WEB,    "default port");

    inicfg_move(&netdata_config, CONFIG_SECTION_GLOBAL, "default port",
                CONFIG_SECTION_WEB,    "default port");

    inicfg_move(&netdata_config, CONFIG_SECTION_GLOBAL, "disconnect idle web clients after seconds",
                CONFIG_SECTION_WEB,    "disconnect idle clients after seconds");

    inicfg_move(&netdata_config, CONFIG_SECTION_GLOBAL, "respect web browser do not track policy",
                CONFIG_SECTION_WEB,    "respect do not track policy");

    inicfg_move(&netdata_config, CONFIG_SECTION_GLOBAL, "web x-frame-options header",
                CONFIG_SECTION_WEB,    "x-frame-options response header");

    inicfg_move(&netdata_config, CONFIG_SECTION_GLOBAL, "enable web responses gzip compression",
                CONFIG_SECTION_WEB,    "enable gzip compression");

    inicfg_move(&netdata_config, CONFIG_SECTION_GLOBAL, "web compression strategy",
                CONFIG_SECTION_WEB,    "gzip compression strategy");

    inicfg_move(&netdata_config, CONFIG_SECTION_GLOBAL, "web compression level",
                CONFIG_SECTION_WEB,    "gzip compression level");

    inicfg_move(&netdata_config, CONFIG_SECTION_GLOBAL,      "config directory",
                CONFIG_SECTION_DIRECTORIES, "config");

    inicfg_move(&netdata_config, CONFIG_SECTION_GLOBAL,      "stock config directory",
                CONFIG_SECTION_DIRECTORIES, "stock config");

    inicfg_move(&netdata_config, CONFIG_SECTION_GLOBAL,      "log directory",
                CONFIG_SECTION_DIRECTORIES, "log");

    inicfg_move(&netdata_config, CONFIG_SECTION_GLOBAL,      "web files directory",
                CONFIG_SECTION_DIRECTORIES, "web");

    inicfg_move(&netdata_config, CONFIG_SECTION_GLOBAL,      "cache directory",
                CONFIG_SECTION_DIRECTORIES, "cache");

    inicfg_move(&netdata_config, CONFIG_SECTION_GLOBAL,      "lib directory",
                CONFIG_SECTION_DIRECTORIES, "lib");

    inicfg_move(&netdata_config, CONFIG_SECTION_GLOBAL,      "home directory",
                CONFIG_SECTION_DIRECTORIES, "home");

    inicfg_move(&netdata_config, CONFIG_SECTION_GLOBAL,      "lock directory",
                CONFIG_SECTION_DIRECTORIES, "lock");

    inicfg_move(&netdata_config, CONFIG_SECTION_GLOBAL,      "plugins directory",
                CONFIG_SECTION_DIRECTORIES, "plugins");

    inicfg_move(&netdata_config, CONFIG_SECTION_HEALTH,      "health configuration directory",
                CONFIG_SECTION_DIRECTORIES, "health config");

    inicfg_move(&netdata_config, CONFIG_SECTION_HEALTH,      "stock health configuration directory",
                CONFIG_SECTION_DIRECTORIES, "stock health config");

    inicfg_move(&netdata_config, CONFIG_SECTION_REGISTRY,    "registry db directory",
                CONFIG_SECTION_DIRECTORIES, "registry");

    inicfg_move(&netdata_config, CONFIG_SECTION_GLOBAL, "debug log",
                CONFIG_SECTION_LOGS,   "debug");

    inicfg_move(&netdata_config, CONFIG_SECTION_GLOBAL, "error log",
                CONFIG_SECTION_LOGS,   "error");

    inicfg_move(&netdata_config, CONFIG_SECTION_GLOBAL, "access log",
                CONFIG_SECTION_LOGS,   "access");

    inicfg_move(&netdata_config, CONFIG_SECTION_GLOBAL, "facility log",
                CONFIG_SECTION_LOGS,   "facility");

    inicfg_move(&netdata_config, CONFIG_SECTION_GLOBAL, "errors flood protection period",
                CONFIG_SECTION_LOGS,   "errors flood protection period");

    inicfg_move(&netdata_config, CONFIG_SECTION_GLOBAL, "errors to trigger flood protection",
                CONFIG_SECTION_LOGS,   "errors to trigger flood protection");

    inicfg_move(&netdata_config, CONFIG_SECTION_GLOBAL, "debug flags",
                CONFIG_SECTION_LOGS,   "debug flags");

    inicfg_move(&netdata_config, CONFIG_SECTION_GLOBAL,   "TZ environment variable",
                CONFIG_SECTION_ENV_VARS, "TZ");

    inicfg_move(&netdata_config, CONFIG_SECTION_PLUGINS,  "PATH environment variable",
                CONFIG_SECTION_ENV_VARS, "PATH");

    inicfg_move(&netdata_config, CONFIG_SECTION_PLUGINS,  "PYTHONPATH environment variable",
                CONFIG_SECTION_ENV_VARS, "PYTHONPATH");

    inicfg_move(&netdata_config, CONFIG_SECTION_STATSD,  "enabled",
                CONFIG_SECTION_PLUGINS, "statsd");

    inicfg_move(&netdata_config, CONFIG_SECTION_GLOBAL,  "memory mode",
                CONFIG_SECTION_DB,      "db");

    inicfg_move(&netdata_config, CONFIG_SECTION_DB,      "mode",
                CONFIG_SECTION_DB,      "db");

    inicfg_move(&netdata_config, CONFIG_SECTION_GLOBAL,  "history",
                CONFIG_SECTION_DB,      "retention");

    inicfg_move(&netdata_config, CONFIG_SECTION_GLOBAL,  "update every",
                CONFIG_SECTION_DB,      "update every");

    inicfg_move(&netdata_config, CONFIG_SECTION_GLOBAL,  "page cache size",
                CONFIG_SECTION_DB,      "dbengine page cache size");

    inicfg_move(&netdata_config, CONFIG_SECTION_DB,      "dbengine page cache size MB",
                CONFIG_SECTION_DB,      "dbengine page cache size");

    inicfg_move(&netdata_config, CONFIG_SECTION_DB,      "dbengine extent cache size MB",
                CONFIG_SECTION_DB,      "dbengine extent cache size");

    inicfg_move(&netdata_config, CONFIG_SECTION_DB,      "page cache size",
                CONFIG_SECTION_DB,      "dbengine page cache size MB");

//    inicfg_move(&netdata_config, CONFIG_SECTION_GLOBAL,  "page cache uses malloc",
//                CONFIG_SECTION_DB,      "dbengine page cache with malloc");
//
//    inicfg_move(&netdata_config, CONFIG_SECTION_DB,      "page cache with malloc",
//                CONFIG_SECTION_DB,      "dbengine page cache with malloc");

    inicfg_move(&netdata_config, CONFIG_SECTION_GLOBAL,  "memory deduplication (ksm)",
                CONFIG_SECTION_DB,      "memory deduplication (ksm)");

    inicfg_move(&netdata_config, CONFIG_SECTION_GLOBAL,  "dbengine page fetch timeout",
                CONFIG_SECTION_DB,      "dbengine page fetch timeout secs");

    inicfg_move(&netdata_config, CONFIG_SECTION_GLOBAL,  "dbengine page fetch retries",
                CONFIG_SECTION_DB,      "dbengine page fetch retries");

    inicfg_move(&netdata_config, CONFIG_SECTION_GLOBAL,  "dbengine extent pages",
                CONFIG_SECTION_DB,      "dbengine pages per extent");

    inicfg_move(&netdata_config, CONFIG_SECTION_GLOBAL,  "cleanup obsolete charts after seconds",
                CONFIG_SECTION_DB,      "cleanup obsolete charts after");

    inicfg_move(&netdata_config, CONFIG_SECTION_DB,      "cleanup obsolete charts after secs",
                CONFIG_SECTION_DB,      "cleanup obsolete charts after");

    inicfg_move(&netdata_config, CONFIG_SECTION_GLOBAL,  "gap when lost iterations above",
                CONFIG_SECTION_DB,      "gap when lost iterations above");

    inicfg_move(&netdata_config, CONFIG_SECTION_GLOBAL,  "cleanup orphan hosts after seconds",
                CONFIG_SECTION_DB,      "cleanup orphan hosts after");

    inicfg_move(&netdata_config, CONFIG_SECTION_DB,      "cleanup orphan hosts after secs",
                CONFIG_SECTION_DB,      "cleanup orphan hosts after");

    inicfg_move(&netdata_config, CONFIG_SECTION_DB,      "cleanup ephemeral hosts after secs",
                CONFIG_SECTION_DB,      "cleanup ephemeral hosts after");

    inicfg_move(&netdata_config, CONFIG_SECTION_DB,      "seconds to replicate",
                CONFIG_SECTION_DB,      "replication period");

    inicfg_move(&netdata_config, CONFIG_SECTION_DB,      "seconds per replication step",
                CONFIG_SECTION_DB,      "replication step");

    inicfg_move(&netdata_config, CONFIG_SECTION_GLOBAL,  "enable zero metrics",
                CONFIG_SECTION_DB,      "enable zero metrics");

    inicfg_move(&netdata_config, CONFIG_SECTION_CLOUD,  "query thread count",
                CONFIG_SECTION_CLOUD,  "query threads");

    // ----------------------------------------------------------------------------------------------------------------
    // global statistics -> telemetry -> pulse

    inicfg_move(&netdata_config, CONFIG_SECTION_PLUGINS, "netdata monitoring",
                CONFIG_SECTION_PLUGINS, "netdata pulse");

    inicfg_move(&netdata_config, CONFIG_SECTION_PLUGINS, "netdata telemetry",
                CONFIG_SECTION_PLUGINS, "netdata pulse");

    inicfg_move(&netdata_config, CONFIG_SECTION_PLUGINS, "netdata monitoring extended",
                CONFIG_SECTION_PULSE,   "extended");

    inicfg_move(&netdata_config, "telemetry",            "extended telemetry",
                CONFIG_SECTION_PULSE,   "extended");

    inicfg_move(&netdata_config, "global statistics",    "update every",
                CONFIG_SECTION_PULSE,   "update every");

    inicfg_move(&netdata_config, "telemetry",            "update every",
                CONFIG_SECTION_PULSE,   "update every");


    // ----------------------------------------------------------------------------------------------------------------

    bool found_old_config = false;

    if(inicfg_move(&netdata_config, CONFIG_SECTION_GLOBAL,  "dbengine disk space",
                    CONFIG_SECTION_DB,      "dbengine tier 0 retention size") != -1)
        found_old_config = true;

    if(inicfg_move(&netdata_config, CONFIG_SECTION_GLOBAL,  "dbengine multihost disk space",
                    CONFIG_SECTION_DB,      "dbengine tier 0 retention size") != -1)
        found_old_config = true;

    if(inicfg_move(&netdata_config, CONFIG_SECTION_DB,      "dbengine disk space MB",
                    CONFIG_SECTION_DB,      "dbengine tier 0 retention size") != -1)
        found_old_config = true;

    for(size_t tier = 0; tier < RRD_STORAGE_TIERS ;tier++) {
        char old_config[128], new_config[128];

        snprintfz(old_config, sizeof(old_config), "dbengine tier %zu retention days", tier);
        snprintfz(new_config, sizeof(new_config), "dbengine tier %zu retention time", tier);
        inicfg_move(&netdata_config, CONFIG_SECTION_DB, old_config,
                    CONFIG_SECTION_DB, new_config);

        if(tier == 0)
            snprintfz(old_config, sizeof(old_config), "dbengine multihost disk space MB");
        else
            snprintfz(old_config, sizeof(old_config), "dbengine tier %zu multihost disk space MB", tier);
        snprintfz(new_config, sizeof(new_config), "dbengine tier %zu retention size", tier);
        if(inicfg_move(&netdata_config, CONFIG_SECTION_DB, old_config,
                        CONFIG_SECTION_DB, new_config) != -1 && tier == 0)
            found_old_config = true;

        snprintfz(old_config, sizeof(old_config), "dbengine tier %zu disk space MB", tier);
        snprintfz(new_config, sizeof(new_config), "dbengine tier %zu retention size", tier);
        if(inicfg_move(&netdata_config, CONFIG_SECTION_DB, old_config,
                        CONFIG_SECTION_DB, new_config) != -1 && tier == 0)
            found_old_config = true;
    }

    legacy_multihost_db_space = found_old_config;

    // ----------------------------------------------------------------------------------------------------------------

    inicfg_move(&netdata_config, CONFIG_SECTION_LOGS,   "error",
                CONFIG_SECTION_LOGS,   "daemon");

    inicfg_move(&netdata_config, CONFIG_SECTION_LOGS,   "severity level",
                CONFIG_SECTION_LOGS,   "level");

    inicfg_move(&netdata_config, CONFIG_SECTION_LOGS,   "errors to trigger flood protection",
                CONFIG_SECTION_LOGS,   "logs to trigger flood protection");

    inicfg_move(&netdata_config, CONFIG_SECTION_LOGS,   "errors flood protection period",
                CONFIG_SECTION_LOGS,   "logs flood protection period");

    inicfg_move(&netdata_config, CONFIG_SECTION_HEALTH, "is ephemeral",
                CONFIG_SECTION_GLOBAL, "is ephemeral node");

    inicfg_move(&netdata_config, CONFIG_SECTION_HEALTH, "has unstable connection",
                CONFIG_SECTION_GLOBAL, "has unstable connection");

    inicfg_move(&netdata_config, CONFIG_SECTION_HEALTH, "run at least every seconds",
                CONFIG_SECTION_HEALTH, "run at least every");

    inicfg_move(&netdata_config, CONFIG_SECTION_HEALTH, "postpone alarms during hibernation for seconds",
                CONFIG_SECTION_HEALTH, "postpone alarms during hibernation for");

    inicfg_move(&netdata_config, CONFIG_SECTION_HEALTH, "health log history",
                CONFIG_SECTION_HEALTH, "health log retention");

    inicfg_move(&netdata_config, CONFIG_SECTION_REGISTRY, "registry expire idle persons days",
                CONFIG_SECTION_REGISTRY, "registry expire idle persons");

    inicfg_move(&netdata_config, CONFIG_SECTION_WEB, "disconnect idle clients after seconds",
                CONFIG_SECTION_WEB, "disconnect idle clients after");

    inicfg_move(&netdata_config, CONFIG_SECTION_WEB, "accept a streaming request every seconds",
                CONFIG_SECTION_WEB, "accept a streaming request every");

    inicfg_move(&netdata_config, CONFIG_SECTION_STATSD, "set charts as obsolete after secs",
                CONFIG_SECTION_STATSD, "set charts as obsolete after");

    inicfg_move(&netdata_config, CONFIG_SECTION_STATSD, "disconnect idle tcp clients after seconds",
                CONFIG_SECTION_STATSD, "disconnect idle tcp clients after");

    inicfg_move(&netdata_config, "plugin:idlejitter", "loop time in ms",
                "plugin:idlejitter", "loop time");

    inicfg_move(&netdata_config, "plugin:proc:/sys/class/infiniband", "refresh ports state every seconds",
                "plugin:proc:/sys/class/infiniband", "refresh ports state every");
}
