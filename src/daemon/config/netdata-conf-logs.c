// SPDX-License-Identifier: GPL-3.0-or-later

#include "netdata-conf-logs.h"
#include "daemon/common.h"

static void debug_flags_initialize(void) {
    // --------------------------------------------------------------------
    // get the debugging flags from the configuration file

    const char *flags = inicfg_get(&netdata_config, CONFIG_SECTION_LOGS, "debug flags",  "0x0000000000000000");
    nd_setenv("NETDATA_DEBUG_FLAGS", flags, 1);

    debug_flags = strtoull(flags, NULL, 0);
    netdata_log_debug(D_OPTIONS, "Debug flags set to '0x%" PRIX64 "'.", debug_flags);

    if(debug_flags != 0) {
        struct rlimit rl = { RLIM_INFINITY, RLIM_INFINITY };
        if(setrlimit(RLIMIT_CORE, &rl) != 0)
            netdata_log_error("Cannot request unlimited core dumps for debugging... Proceeding anyway...");

#ifdef HAVE_SYS_PRCTL_H
        prctl(PR_SET_DUMPABLE, 1, 0, 0, 0);
#endif
    }
}

void netdata_conf_section_logs(void) {
    static bool run = false;
    if(run) return;
    run = true;

    nd_log_set_facility(inicfg_get(&netdata_config, CONFIG_SECTION_LOGS, "facility", "daemon"));

    time_t period = ND_LOG_DEFAULT_THROTTLE_PERIOD;
    size_t logs = ND_LOG_DEFAULT_THROTTLE_LOGS;
    period = inicfg_get_duration_seconds(&netdata_config, CONFIG_SECTION_LOGS, "logs flood protection period", period);
    logs = (unsigned long)inicfg_get_number(&netdata_config, CONFIG_SECTION_LOGS, "logs to trigger flood protection", (long long int)logs);
    nd_log_set_flood_protection(logs, period);

    const char *netdata_log_level = getenv("NETDATA_LOG_LEVEL");
    netdata_log_level = netdata_log_level ? nd_log_id2priority(nd_log_priority2id(netdata_log_level)) : NDLP_INFO_STR;

    nd_log_set_priority_level(inicfg_get(&netdata_config, CONFIG_SECTION_LOGS, "level", netdata_log_level));

    char filename[FILENAME_MAX + 1];
    char* os_default_method = NULL;
#if defined(OS_LINUX)
    os_default_method = is_stderr_connected_to_journal() /* || nd_log_journal_socket_available() */ ? "journal" : NULL;
#elif defined(OS_WINDOWS)
#if defined(HAVE_ETW)
    os_default_method = "etw";
#elif defined(HAVE_WEL)
    os_default_method = "wel";
#endif
#endif

#if defined(OS_WINDOWS)
    // on windows, debug log goes to windows events
    snprintfz(filename, FILENAME_MAX, "%s", os_default_method);
#else
    snprintfz(filename, FILENAME_MAX, "%s/debug.log", netdata_configured_log_dir);
#endif

    nd_log_set_user_settings(NDLS_DEBUG, inicfg_get(&netdata_config, CONFIG_SECTION_LOGS, "debug", filename));

    if(os_default_method)
        snprintfz(filename, FILENAME_MAX, "%s", os_default_method);
    else
        snprintfz(filename, FILENAME_MAX, "%s/daemon.log", netdata_configured_log_dir);
    nd_log_set_user_settings(NDLS_DAEMON, inicfg_get(&netdata_config, CONFIG_SECTION_LOGS, "daemon", filename));

    if(os_default_method)
        snprintfz(filename, FILENAME_MAX, "%s", os_default_method);
    else
        snprintfz(filename, FILENAME_MAX, "%s/collector.log", netdata_configured_log_dir);
    nd_log_set_user_settings(NDLS_COLLECTORS, inicfg_get(&netdata_config, CONFIG_SECTION_LOGS, "collector", filename));

#if defined(OS_WINDOWS)
    // on windows, access log goes to windows events
    snprintfz(filename, FILENAME_MAX, "%s", os_default_method);
#else
    snprintfz(filename, FILENAME_MAX, "%s/access.log", netdata_configured_log_dir);
#endif
    nd_log_set_user_settings(NDLS_ACCESS, inicfg_get(&netdata_config, CONFIG_SECTION_LOGS, "access", filename));

    if(os_default_method)
        snprintfz(filename, FILENAME_MAX, "%s", os_default_method);
    else
        snprintfz(filename, FILENAME_MAX, "%s/health.log", netdata_configured_log_dir);
    nd_log_set_user_settings(NDLS_HEALTH, inicfg_get(&netdata_config, CONFIG_SECTION_LOGS, "health", filename));

    aclklog_enabled = inicfg_get_boolean(&netdata_config, CONFIG_SECTION_CLOUD, "conversation log", CONFIG_BOOLEAN_NO);
    if (aclklog_enabled) {
#if defined(OS_WINDOWS)
        // on windows, aclk log goes to windows events
        snprintfz(filename, FILENAME_MAX, "%s", os_default_method);
#else
        snprintfz(filename, FILENAME_MAX, "%s/aclk.log", netdata_configured_log_dir);
#endif
        nd_log_set_user_settings(NDLS_ACLK, inicfg_get(&netdata_config, CONFIG_SECTION_CLOUD, "conversation log file", filename));
    }

    debug_flags_initialize();
    aclk_config_get_query_scope();
}
