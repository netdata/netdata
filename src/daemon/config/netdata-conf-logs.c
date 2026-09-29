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

#if defined(OS_WINDOWS) && (defined(HAVE_WEL) || defined(HAVE_ETW))
// Upgrade only the exact legacy path generated from the configured log
// directory. A filename suffix alone cannot distinguish a default from an
// explicit user-selected path such as C:/custom/debug.log.
static bool nd_log_is_default_file_path(const char *value, const char *source_name) {
    if (!value || !*value || !netdata_configured_log_dir || !source_name)
        return false;

    size_t dir_len = strlen(netdata_configured_log_dir);
    char expected[FILENAME_MAX + 1];
    snprintfz(expected, sizeof(expected) - 1, "%s%s%s.log", netdata_configured_log_dir,
              dir_len && (netdata_configured_log_dir[dir_len - 1] == '/' ||
                          netdata_configured_log_dir[dir_len - 1] == '\\') ? "" : "/",
              source_name);

    char normalized_value[FILENAME_MAX + 1];
    char normalized_expected[FILENAME_MAX + 1];
    os_translate_path(normalized_value, value, sizeof(normalized_value));
    os_translate_path(normalized_expected, expected, sizeof(normalized_expected));
    for (char *p = normalized_value; *p; p++)
        if (*p == '\\') *p = '/';
    for (char *p = normalized_expected; *p; p++)
        if (*p == '\\') *p = '/';

    return strcasecmp(normalized_value, normalized_expected) == 0;
}
#endif

void netdata_conf_section_logs(void) {
    FUNCTION_RUN_ONCE();

    netdata_conf_section_directories();

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
    const char *s;
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
    s = inicfg_get_log_path_setting(&netdata_config, CONFIG_SECTION_LOGS, "debug", filename);
#if defined(OS_WINDOWS) && (defined(HAVE_WEL) || defined(HAVE_ETW))
    if (nd_log_is_default_file_path(s, "debug")) {
        inicfg_set(&netdata_config, CONFIG_SECTION_LOGS, "debug", os_default_method);
        s = os_default_method;
    }
#endif
    nd_log_set_user_settings(NDLS_DEBUG, s);

    if(os_default_method)
        snprintfz(filename, FILENAME_MAX, "%s", os_default_method);
    else
        snprintfz(filename, FILENAME_MAX, "%s/daemon.log", netdata_configured_log_dir);
    s = inicfg_get_log_path_setting(&netdata_config, CONFIG_SECTION_LOGS, "daemon", filename);
#if defined(OS_WINDOWS) && (defined(HAVE_WEL) || defined(HAVE_ETW))
    if (nd_log_is_default_file_path(s, "daemon")) {
        inicfg_set(&netdata_config, CONFIG_SECTION_LOGS, "daemon", os_default_method);
        s = os_default_method;
    }
#endif
    nd_log_set_user_settings(NDLS_DAEMON, s);

    if(os_default_method)
        snprintfz(filename, FILENAME_MAX, "%s", os_default_method);
    else
        snprintfz(filename, FILENAME_MAX, "%s/collector.log", netdata_configured_log_dir);
    s = inicfg_get_log_path_setting(&netdata_config, CONFIG_SECTION_LOGS, "collector", filename);
#if defined(OS_WINDOWS) && (defined(HAVE_WEL) || defined(HAVE_ETW))
    if (nd_log_is_default_file_path(s, "collector")) {
        inicfg_set(&netdata_config, CONFIG_SECTION_LOGS, "collector", os_default_method);
        s = os_default_method;
    }
#endif
    nd_log_set_user_settings(NDLS_COLLECTORS, s);

#if defined(OS_WINDOWS)
    // on windows, access log goes to windows events
    snprintfz(filename, FILENAME_MAX, "%s", os_default_method);
#else
    snprintfz(filename, FILENAME_MAX, "%s/access.log", netdata_configured_log_dir);
#endif
    s = inicfg_get_log_path_setting(&netdata_config, CONFIG_SECTION_LOGS, "access", filename);
#if defined(OS_WINDOWS) && (defined(HAVE_WEL) || defined(HAVE_ETW))
    if (nd_log_is_default_file_path(s, "access")) {
        inicfg_set(&netdata_config, CONFIG_SECTION_LOGS, "access", os_default_method);
        s = os_default_method;
    }
#endif
    nd_log_set_user_settings(NDLS_ACCESS, s);

    if(os_default_method)
        snprintfz(filename, FILENAME_MAX, "%s", os_default_method);
    else
        snprintfz(filename, FILENAME_MAX, "%s/health.log", netdata_configured_log_dir);
    s = inicfg_get_log_path_setting(&netdata_config, CONFIG_SECTION_LOGS, "health", filename);
#if defined(OS_WINDOWS) && (defined(HAVE_WEL) || defined(HAVE_ETW))
    if (nd_log_is_default_file_path(s, "health")) {
        inicfg_set(&netdata_config, CONFIG_SECTION_LOGS, "health", os_default_method);
        s = os_default_method;
    }
#endif
    nd_log_set_user_settings(NDLS_HEALTH, s);

    // NDLS_ACLK is statically initialized to NDLM_FILE (unlike all other sources which use
    // NDLM_DEFAULT), so it bypasses the normal nd_log_open config path even when aclk
    // conversation logging is disabled.  On Windows, always redirect it to the Windows event
    // log first so that the broken compile-time LOG_DIR path is never used.
#if defined(OS_WINDOWS) && (defined(HAVE_WEL) || defined(HAVE_ETW))
    nd_log_set_user_settings(NDLS_ACLK, os_default_method);
#endif

    aclklog_enabled = inicfg_get_boolean(&netdata_config, CONFIG_SECTION_CLOUD, "conversation log", CONFIG_BOOLEAN_NO);
    if (aclklog_enabled) {
#if defined(OS_WINDOWS)
        // on windows, aclk log goes to windows events
        snprintfz(filename, FILENAME_MAX, "%s", os_default_method);
#else
        snprintfz(filename, FILENAME_MAX, "%s/aclk.log", netdata_configured_log_dir);
#endif
        s = inicfg_get_log_path_setting(&netdata_config, CONFIG_SECTION_CLOUD, "conversation log file", filename);
#if defined(OS_WINDOWS) && (defined(HAVE_WEL) || defined(HAVE_ETW))
        if (nd_log_is_default_file_path(s, "aclk")) {
            inicfg_set(&netdata_config, CONFIG_SECTION_CLOUD, "conversation log file", os_default_method);
            s = os_default_method;
        }
#endif
        nd_log_set_user_settings(NDLS_ACLK, s);
    }

    debug_flags_initialize();
    aclk_config_get_query_scope();
}
