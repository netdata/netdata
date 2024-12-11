// SPDX-License-Identifier: GPL-3.0-or-later

#include "netdata-conf-logs.h"

void netdata_conf_section_logs(void) {
    static bool run = false;
    if(run) return;
    run = true;

    nd_log_set_facility(config_get(CONFIG_SECTION_LOGS, "facility", "daemon"));

    time_t period = ND_LOG_DEFAULT_THROTTLE_PERIOD;
    size_t logs = ND_LOG_DEFAULT_THROTTLE_LOGS;
    period = config_get_duration_seconds(CONFIG_SECTION_LOGS, "logs flood protection period", period);
    logs = (unsigned long)config_get_number(CONFIG_SECTION_LOGS, "logs to trigger flood protection", (long long int)logs);
    nd_log_set_flood_protection(logs, period);

    const char *netdata_log_level = getenv("NETDATA_LOG_LEVEL");
    netdata_log_level = netdata_log_level ? nd_log_id2priority(nd_log_priority2id(netdata_log_level)) : NDLP_INFO_STR;

    nd_log_set_priority_level(config_get(CONFIG_SECTION_LOGS, "level", netdata_log_level));

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

    nd_log_set_user_settings(NDLS_DEBUG, config_get(CONFIG_SECTION_LOGS, "debug", filename));

    if(os_default_method)
        snprintfz(filename, FILENAME_MAX, "%s", os_default_method);
    else
        snprintfz(filename, FILENAME_MAX, "%s/daemon.log", netdata_configured_log_dir);
    nd_log_set_user_settings(NDLS_DAEMON, config_get(CONFIG_SECTION_LOGS, "daemon", filename));

    if(os_default_method)
        snprintfz(filename, FILENAME_MAX, "%s", os_default_method);
    else
        snprintfz(filename, FILENAME_MAX, "%s/collector.log", netdata_configured_log_dir);
    nd_log_set_user_settings(NDLS_COLLECTORS, config_get(CONFIG_SECTION_LOGS, "collector", filename));

#if defined(OS_WINDOWS)
    // on windows, access log goes to windows events
    snprintfz(filename, FILENAME_MAX, "%s", os_default_method);
#else
    snprintfz(filename, FILENAME_MAX, "%s/access.log", netdata_configured_log_dir);
#endif
    nd_log_set_user_settings(NDLS_ACCESS, config_get(CONFIG_SECTION_LOGS, "access", filename));

    if(os_default_method)
        snprintfz(filename, FILENAME_MAX, "%s", os_default_method);
    else
        snprintfz(filename, FILENAME_MAX, "%s/health.log", netdata_configured_log_dir);
    nd_log_set_user_settings(NDLS_HEALTH, config_get(CONFIG_SECTION_LOGS, "health", filename));

    aclklog_enabled = config_get_boolean(CONFIG_SECTION_CLOUD, "conversation log", CONFIG_BOOLEAN_NO);
    if (aclklog_enabled) {
#if defined(OS_WINDOWS)
        // on windows, aclk log goes to windows events
        snprintfz(filename, FILENAME_MAX, "%s", os_default_method);
#else
        snprintfz(filename, FILENAME_MAX, "%s/aclk.log", netdata_configured_log_dir);
#endif
        nd_log_set_user_settings(NDLS_ACLK, config_get(CONFIG_SECTION_CLOUD, "conversation log file", filename));
    }

    aclk_config_get_query_scope();
}
