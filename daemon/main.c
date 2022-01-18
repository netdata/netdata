// SPDX-License-Identifier: GPL-3.0-or-later

#include "common.h"
#include "buildinfo.h"

int netdata_zero_metrics_enabled;
int netdata_anonymous_statistics_enabled;

struct config netdata_config = {
        .first_section = NULL,
        .last_section = NULL,
        .mutex = NETDATA_MUTEX_INITIALIZER,
        .index = {
                .avl_tree = {
                        .root = NULL,
                        .compar = appconfig_section_compare
                },
                .rwlock = AVL_LOCK_INITIALIZER
        }
};

void netdata_cleanup_and_exit(int ret) {
    // enabling this, is wrong
    // because the threads will be cancelled while cleaning up
    // netdata_exit = 1;

    error_log_limit_unlimited();
    info("EXIT: netdata prepares to exit with code %d...", ret);

    send_statistics("EXIT", ret?"ERROR":"OK","-");

    char agent_crash_file[FILENAME_MAX + 1];
    char agent_incomplete_shutdown_file[FILENAME_MAX + 1];
    snprintfz(agent_crash_file, FILENAME_MAX, "%s/.agent_crash", netdata_configured_varlib_dir);
    snprintfz(agent_incomplete_shutdown_file, FILENAME_MAX, "%s/.agent_incomplete_shutdown", netdata_configured_varlib_dir);
    (void) rename(agent_crash_file, agent_incomplete_shutdown_file);

    // cleanup/save the database and exit
    info("EXIT: cleaning up the database...");
    rrdhost_cleanup_all();

    if(!ret) {
        // exit cleanly

        // stop everything
        info("EXIT: stopping static threads...");
#ifdef ENABLE_NEW_CLOUD_PROTOCOL
        aclk_sync_exit_all();
#endif
        cancel_main_threads();

        // free the database
        info("EXIT: freeing database memory...");
#ifdef ENABLE_DBENGINE
        rrdeng_prepare_exit(&multidb_ctx);
#endif
        rrdhost_free_all();
#ifdef ENABLE_DBENGINE
        rrdeng_exit(&multidb_ctx);
#endif
    }
    sql_close_database();

    // unlink the pid
    if(pidfile[0]) {
        info("EXIT: removing netdata PID file '%s'...", pidfile);
        if(unlink(pidfile) != 0)
            error("EXIT: cannot unlink pidfile '%s'.", pidfile);
    }

#ifdef ENABLE_HTTPS
    security_clean_openssl();
#endif
    info("EXIT: all done - netdata is now exiting - bye bye...");
    (void) unlink(agent_incomplete_shutdown_file);
    exit(ret);
}

struct netdata_static_thread static_threads[] = {
    NETDATA_PLUGIN_HOOK_GLOBAL_STATISTICS

    NETDATA_PLUGIN_HOOK_CHECKS
    NETDATA_PLUGIN_HOOK_FREEBSD
    NETDATA_PLUGIN_HOOK_MACOS

    // linux internal plugins
    NETDATA_PLUGIN_HOOK_LINUX_PROC
    NETDATA_PLUGIN_HOOK_LINUX_DISKSPACE
    NETDATA_PLUGIN_HOOK_LINUX_TIMEX
    NETDATA_PLUGIN_HOOK_LINUX_CGROUPS
    NETDATA_PLUGIN_HOOK_LINUX_TC

    NETDATA_PLUGIN_HOOK_IDLEJITTER
    NETDATA_PLUGIN_HOOK_STATSD

#if defined(ENABLE_ACLK) || defined(ACLK_NG)
    NETDATA_ACLK_HOOK
#endif

        // common plugins for all systems
    {"BACKENDS",             NULL,                    NULL,         1, NULL, NULL, backends_main},
    {"EXPORTING",            NULL,                    NULL,         1, NULL, NULL, exporting_main},
    {"WEB_SERVER[static1]",  NULL,                    NULL,         0, NULL, NULL, socket_listen_main_static_threaded},
    {"STREAM",               NULL,                    NULL,         0, NULL, NULL, rrdpush_sender_thread},

    NETDATA_PLUGIN_HOOK_PLUGINSD
    NETDATA_PLUGIN_HOOK_HEALTH
    NETDATA_PLUGIN_HOOK_ANALYTICS
    NETDATA_PLUGIN_HOOK_SERVICE

    {NULL,                   NULL,                    NULL,         0, NULL, NULL, NULL}
};

void web_server_threading_selection(void) {
    web_server_mode = web_server_mode_id(config_get(CONFIG_SECTION_WEB, "mode", web_server_mode_name(web_server_mode)));

    int static_threaded = (web_server_mode == WEB_SERVER_MODE_STATIC_THREADED);

    int i;
    for (i = 0; static_threads[i].name; i++) {
        if (static_threads[i].start_routine == socket_listen_main_static_threaded)
            static_threads[i].enabled = static_threaded;
    }
}

int make_dns_decision(const char *section_name, const char *config_name, const char *default_value, SIMPLE_PATTERN *p)
{
    char *value = config_get(section_name,config_name,default_value);
    if(!strcmp("yes",value))
        return 1;
    if(!strcmp("no",value))
        return 0;
    if(strcmp("heuristic",value))
        error("Invalid configuration option '%s' for '%s'/'%s'. Valid options are 'yes', 'no' and 'heuristic'. Proceeding with 'heuristic'",
              value, section_name, config_name);

    return simple_pattern_is_potential_name(p);
}

void web_server_config_options(void)
{
    web_client_timeout =
        (int)config_get_number(CONFIG_SECTION_WEB, "disconnect idle clients after seconds", web_client_timeout);
    web_client_first_request_timeout =
        (int)config_get_number(CONFIG_SECTION_WEB, "timeout for first request", web_client_first_request_timeout);
    web_client_streaming_rate_t =
        config_get_number(CONFIG_SECTION_WEB, "accept a streaming request every seconds", web_client_streaming_rate_t);

    respect_web_browser_do_not_track_policy =
        config_get_boolean(CONFIG_SECTION_WEB, "respect do not track policy", respect_web_browser_do_not_track_policy);
    web_x_frame_options = config_get(CONFIG_SECTION_WEB, "x-frame-options response header", "");
    if(!*web_x_frame_options)
        web_x_frame_options = NULL;

    web_allow_connections_from =
        simple_pattern_create(config_get(CONFIG_SECTION_WEB, "allow connections from", "localhost *"),
                              NULL, SIMPLE_PATTERN_EXACT);
    web_allow_connections_dns  =
        make_dns_decision(CONFIG_SECTION_WEB, "allow connections by dns", "heuristic", web_allow_connections_from);
    web_allow_dashboard_from   =
        simple_pattern_create(config_get(CONFIG_SECTION_WEB, "allow dashboard from", "localhost *"),
                              NULL, SIMPLE_PATTERN_EXACT);
    web_allow_dashboard_dns    =
        make_dns_decision(CONFIG_SECTION_WEB, "allow dashboard by dns", "heuristic", web_allow_dashboard_from);
    web_allow_badges_from      =
        simple_pattern_create(config_get(CONFIG_SECTION_WEB, "allow badges from", "*"), NULL, SIMPLE_PATTERN_EXACT);
    web_allow_badges_dns       =
        make_dns_decision(CONFIG_SECTION_WEB, "allow badges by dns", "heuristic", web_allow_badges_from);
    web_allow_registry_from    =
        simple_pattern_create(config_get(CONFIG_SECTION_REGISTRY, "allow from", "*"), NULL, SIMPLE_PATTERN_EXACT);
    web_allow_registry_dns     = make_dns_decision(CONFIG_SECTION_REGISTRY, "allow by dns", "heuristic",
                                                   web_allow_registry_from);
    web_allow_streaming_from   = simple_pattern_create(config_get(CONFIG_SECTION_WEB, "allow streaming from", "*"),
                                                       NULL, SIMPLE_PATTERN_EXACT);
    web_allow_streaming_dns    = make_dns_decision(CONFIG_SECTION_WEB, "allow streaming by dns", "heuristic",
                                                   web_allow_streaming_from);
    // Note the default is not heuristic, the wildcards could match DNS but the intent is ip-addresses.
    web_allow_netdataconf_from = simple_pattern_create(config_get(CONFIG_SECTION_WEB, "allow netdata.conf from",
                                                       "localhost fd* 10.* 192.168.* 172.16.* 172.17.* 172.18.*"
                                                       " 172.19.* 172.20.* 172.21.* 172.22.* 172.23.* 172.24.*"
                                                       " 172.25.* 172.26.* 172.27.* 172.28.* 172.29.* 172.30.*"
                                                       " 172.31.* UNKNOWN"), NULL, SIMPLE_PATTERN_EXACT);
    web_allow_netdataconf_dns  =
        make_dns_decision(CONFIG_SECTION_WEB, "allow netdata.conf by dns", "no", web_allow_netdataconf_from);
    web_allow_mgmt_from        =
        simple_pattern_create(config_get(CONFIG_SECTION_WEB, "allow management from", "localhost"),
                              NULL, SIMPLE_PATTERN_EXACT);
    web_allow_mgmt_dns         =
        make_dns_decision(CONFIG_SECTION_WEB, "allow management by dns","heuristic",web_allow_mgmt_from);


#ifdef NETDATA_WITH_ZLIB
    web_enable_gzip = config_get_boolean(CONFIG_SECTION_WEB, "enable gzip compression", web_enable_gzip);

    char *s = config_get(CONFIG_SECTION_WEB, "gzip compression strategy", "default");
    if(!strcmp(s, "default"))
        web_gzip_strategy = Z_DEFAULT_STRATEGY;
    else if(!strcmp(s, "filtered"))
        web_gzip_strategy = Z_FILTERED;
    else if(!strcmp(s, "huffman only"))
        web_gzip_strategy = Z_HUFFMAN_ONLY;
    else if(!strcmp(s, "rle"))
        web_gzip_strategy = Z_RLE;
    else if(!strcmp(s, "fixed"))
        web_gzip_strategy = Z_FIXED;
    else {
        error("Invalid compression strategy '%s'. Valid strategies are 'default', 'filtered', 'huffman only', 'rle' and 'fixed'. Proceeding with 'default'.", s);
        web_gzip_strategy = Z_DEFAULT_STRATEGY;
    }

    web_gzip_level = (int)config_get_number(CONFIG_SECTION_WEB, "gzip compression level", 3);
    if(web_gzip_level < 1) {
        error("Invalid compression level %d. Valid levels are 1 (fastest) to 9 (best ratio). Proceeding with level 1 (fastest compression).", web_gzip_level);
        web_gzip_level = 1;
    }
    else if(web_gzip_level > 9) {
        error("Invalid compression level %d. Valid levels are 1 (fastest) to 9 (best ratio). Proceeding with level 9 (best compression).", web_gzip_level);
        web_gzip_level = 9;
    }
#endif /* NETDATA_WITH_ZLIB */
}


// killpid kills pid with SIGTERM.
int killpid(pid_t pid) {
    int ret;
    debug(D_EXIT, "Request to kill pid %d", pid);

    errno = 0;
    ret = kill(pid, SIGTERM);
    if (ret == -1) {
        switch(errno) {
            case ESRCH:
                // We wanted the process to exit so just let the caller handle.
                return ret;

            case EPERM:
                error("Cannot kill pid %d, but I do not have enough permissions.", pid);
                break;

            default:
                error("Cannot kill pid %d, but I received an error.", pid);
                break;
        }
    }

    return ret;
}

void cancel_main_threads() {
    error_log_limit_unlimited();

    int i, found = 0;
    usec_t max = 5 * USEC_PER_SEC, step = 100000;
    for (i = 0; static_threads[i].name != NULL ; i++) {
        if(static_threads[i].enabled == NETDATA_MAIN_THREAD_RUNNING) {
            info("EXIT: Stopping main thread: %s", static_threads[i].name);
            netdata_thread_cancel(*static_threads[i].thread);
            found++;
        }
    }

    netdata_exit = 1;

    while(found && max > 0) {
        max -= step;
        info("Waiting %d threads to finish...", found);
        sleep_usec(step);
        found = 0;
        for (i = 0; static_threads[i].name != NULL ; i++) {
            if (static_threads[i].enabled != NETDATA_MAIN_THREAD_EXITED)
                found++;
        }
    }

    if(found) {
        for (i = 0; static_threads[i].name != NULL ; i++) {
            if (static_threads[i].enabled != NETDATA_MAIN_THREAD_EXITED)
                error("Main thread %s takes too long to exit. Giving up...", static_threads[i].name);
        }
    }
    else
        info("All threads finished.");
}

struct option_def option_definitions[] = {
    // opt description                                    arg name       default value
    { 'c', "Configuration file to load.",                 "filename",    CONFIG_DIR "/" CONFIG_FILENAME},
    { 'D', "Do not fork. Run in the foreground.",         NULL,          "run in the background"},
    { 'd', "Fork. Run in the background.",                NULL,          "run in the background"},
    { 'h', "Display this help message.",                  NULL,          NULL},
    { 'P', "File to save a pid while running.",           "filename",    "do not save pid to a file"},
    { 'i', "The IP address to listen to.",                "IP",          "all IP addresses IPv4 and IPv6"},
    { 'p', "API/Web port to use.",                        "port",        "19999"},
    { 's', "Prefix for /proc and /sys (for containers).", "path",        "no prefix"},
    { 't', "The internal clock of netdata.",              "seconds",     "1"},
    { 'u', "Run as user.",                                "username",    "netdata"},
    { 'v', "Print netdata version and exit.",             NULL,          NULL},
    { 'V', "Print netdata version and exit.",             NULL,          NULL},
    { 'W', "See Advanced options below.",                 "options",     NULL},
};

int help(int exitcode) {
    FILE *stream;
    if(exitcode == 0)
        stream = stdout;
    else
        stream = stderr;

    int num_opts = sizeof(option_definitions) / sizeof(struct option_def);
    int i;
    int max_len_arg = 0;

    // Compute maximum argument length
    for( i = 0; i < num_opts; i++ ) {
        if(option_definitions[i].arg_name) {
            int len_arg = (int)strlen(option_definitions[i].arg_name);
            if(len_arg > max_len_arg) max_len_arg = len_arg;
        }
    }

    if(max_len_arg > 30) max_len_arg = 30;
    if(max_len_arg < 20) max_len_arg = 20;

    fprintf(stream, "%s", "\n"
            " ^\n"
            " |.-.   .-.   .-.   .-.   .  netdata                                         \n"
            " |   '-'   '-'   '-'   '-'   real-time performance monitoring, done right!   \n"
            " +----+-----+-----+-----+-----+-----+-----+-----+-----+-----+-----+-----+--->\n"
            "\n"
            " Copyright (C) 2016-2020, Netdata, Inc. <info@netdata.cloud>\n"
            " Released under GNU General Public License v3 or later.\n"
            " All rights reserved.\n"
            "\n"
            " Home Page  : https://netdata.cloud\n"
            " Source Code: https://github.com/netdata/netdata\n"
            " Docs       : https://learn.netdata.cloud\n"
            " Support    : https://github.com/netdata/netdata/issues\n"
            " License    : https://github.com/netdata/netdata/blob/master/LICENSE.md\n"
            "\n"
            " Twitter    : https://twitter.com/linuxnetdata\n"
            " Facebook   : https://www.facebook.com/linuxnetdata/\n"
            "\n"
            "\n"
    );

    fprintf(stream, " SYNOPSIS: netdata [options]\n");
    fprintf(stream, "\n");
    fprintf(stream, " Options:\n\n");

    // Output options description.
    for( i = 0; i < num_opts; i++ ) {
        fprintf(stream, "  -%c %-*s  %s", option_definitions[i].val, max_len_arg, option_definitions[i].arg_name ? option_definitions[i].arg_name : "", option_definitions[i].description);
        if(option_definitions[i].default_value) {
            fprintf(stream, "\n   %c %-*s  Default: %s\n", ' ', max_len_arg, "", option_definitions[i].default_value);
        } else {
            fprintf(stream, "\n");
        }
        fprintf(stream, "\n");
    }

    fprintf(stream, "\n Advanced options:\n\n"
            "  -W stacksize=N           Set the stacksize (in bytes).\n\n"
            "  -W debug_flags=N         Set runtime tracing to debug.log.\n\n"
            "  -W unittest              Run internal unittests and exit.\n\n"
            "  -W sqlite-check          Check metadata database integrity and exit.\n\n"
            "  -W sqlite-fix            Check metadata database integrity, fix if needed and exit.\n\n"
            "  -W sqlite-compact        Reclaim metadata database unused space and exit.\n\n"
#ifdef ENABLE_DBENGINE
            "  -W createdataset=N       Create a DB engine dataset of N seconds and exit.\n\n"
            "  -W stresstest=A,B,C,D,E,F\n"
            "                           Run a DB engine stress test for A seconds,\n"
            "                           with B writers and C readers, with a ramp up\n"
            "                           time of D seconds for writers, a page cache\n"
            "                           size of E MiB, an optional disk space limit\n"
            "                           of F MiB and exit.\n\n"
#endif
            "  -W set section option value\n"
            "                           set netdata.conf option from the command line.\n\n"
            "  -W simple-pattern pattern string\n"
            "                           Check if string matches pattern and exit.\n\n"
            "  -W \"claim -token=TOKEN -rooms=ROOM1,ROOM2\"\n"
            "                           Claim the agent to the workspace rooms pointed to by TOKEN and ROOM*.\n\n"
    );

    fprintf(stream, "\n Signals netdata handles:\n\n"
            "  - HUP                    Close and reopen log files.\n"
            "  - USR1                   Save internal DB to disk.\n"
            "  - USR2                   Reload health configuration.\n"
            "\n"
    );

    fflush(stream);
    return exitcode;
}

#ifdef ENABLE_HTTPS
static void security_init(){
    char filename[FILENAME_MAX + 1];
    snprintfz(filename, FILENAME_MAX, "%s/ssl/key.pem",netdata_configured_user_config_dir);
    security_key    = config_get(CONFIG_SECTION_WEB, "ssl key",  filename);

    snprintfz(filename, FILENAME_MAX, "%s/ssl/cert.pem",netdata_configured_user_config_dir);
    security_cert    = config_get(CONFIG_SECTION_WEB, "ssl certificate",  filename);

    tls_version    = config_get(CONFIG_SECTION_WEB, "tls version",  "1.3");
    tls_ciphers    = config_get(CONFIG_SECTION_WEB, "tls ciphers",  "none");

    security_openssl_library();
}
#endif

static void log_init(void) {
    char filename[FILENAME_MAX + 1];
    snprintfz(filename, FILENAME_MAX, "%s/debug.log", netdata_configured_log_dir);
    stdout_filename    = config_get(CONFIG_SECTION_GLOBAL, "debug log",  filename);

    snprintfz(filename, FILENAME_MAX, "%s/error.log", netdata_configured_log_dir);
    stderr_filename    = config_get(CONFIG_SECTION_GLOBAL, "error log",  filename);

    snprintfz(filename, FILENAME_MAX, "%s/access.log", netdata_configured_log_dir);
    stdaccess_filename = config_get(CONFIG_SECTION_GLOBAL, "access log", filename);

    char deffacility[8];
    snprintfz(deffacility,7,"%s","daemon");
    facility_log = config_get(CONFIG_SECTION_GLOBAL, "facility log",  deffacility);

    error_log_throttle_period = config_get_number(CONFIG_SECTION_GLOBAL, "errors flood protection period", error_log_throttle_period);
    error_log_errors_per_period = (unsigned long)config_get_number(CONFIG_SECTION_GLOBAL, "errors to trigger flood protection", (long long int)error_log_errors_per_period);
    error_log_errors_per_period_backup = error_log_errors_per_period;

    setenv("NETDATA_ERRORS_THROTTLE_PERIOD", config_get(CONFIG_SECTION_GLOBAL, "errors flood protection period"    , ""), 1);
    setenv("NETDATA_ERRORS_PER_PERIOD",      config_get(CONFIG_SECTION_GLOBAL, "errors to trigger flood protection", ""), 1);
}

char *initialize_lock_directory_path(char *prefix)
{
    char filename[FILENAME_MAX + 1];
    snprintfz(filename, FILENAME_MAX, "%s/lock", prefix);

    return config_get(CONFIG_SECTION_GLOBAL, "lock directory", filename);
}

static void backwards_compatible_config() {
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

    config_move(CONFIG_SECTION_GLOBAL, "web files owner",
                CONFIG_SECTION_WEB,    "web files owner");

    config_move(CONFIG_SECTION_GLOBAL, "web files group",
                CONFIG_SECTION_WEB,    "web files group");

    config_move(CONFIG_SECTION_BACKEND, "opentsdb host tags",
                CONFIG_SECTION_BACKEND, "host tags");
}

static void get_netdata_configured_variables() {
    backwards_compatible_config();

    // ------------------------------------------------------------------------
    // get the hostname

    char buf[HOSTNAME_MAX + 1];
    if(gethostname(buf, HOSTNAME_MAX) == -1){
        error("Cannot get machine hostname.");
    }

    netdata_configured_hostname = config_get(CONFIG_SECTION_GLOBAL, "hostname", buf);
    debug(D_OPTIONS, "hostname set to '%s'", netdata_configured_hostname);

    // ------------------------------------------------------------------------
    // get default database size

    default_rrd_history_entries = (int) config_get_number(CONFIG_SECTION_GLOBAL, "history", align_entries_to_pagesize(default_rrd_memory_mode, RRD_DEFAULT_HISTORY_ENTRIES));

    long h = align_entries_to_pagesize(default_rrd_memory_mode, default_rrd_history_entries);
    if(h != default_rrd_history_entries) {
        config_set_number(CONFIG_SECTION_GLOBAL, "history", h);
        default_rrd_history_entries = (int)h;
    }

    if(default_rrd_history_entries < 5 || default_rrd_history_entries > RRD_HISTORY_ENTRIES_MAX) {
        error("Invalid history entries %d given. Defaulting to %d.", default_rrd_history_entries, RRD_DEFAULT_HISTORY_ENTRIES);
        default_rrd_history_entries = RRD_DEFAULT_HISTORY_ENTRIES;
    }

    // ------------------------------------------------------------------------
    // get default database update frequency

    default_rrd_update_every = (int) config_get_number(CONFIG_SECTION_GLOBAL, "update every", UPDATE_EVERY);
    if(default_rrd_update_every < 1 || default_rrd_update_every > 600) {
        error("Invalid data collection frequency (update every) %d given. Defaulting to %d.", default_rrd_update_every, UPDATE_EVERY_MAX);
        default_rrd_update_every = UPDATE_EVERY;
    }

    // ------------------------------------------------------------------------
    // get system paths

    netdata_configured_user_config_dir  = config_get(CONFIG_SECTION_GLOBAL, "config directory",       netdata_configured_user_config_dir);
    netdata_configured_stock_config_dir = config_get(CONFIG_SECTION_GLOBAL, "stock config directory", netdata_configured_stock_config_dir);
    netdata_configured_log_dir          = config_get(CONFIG_SECTION_GLOBAL, "log directory",          netdata_configured_log_dir);
    netdata_configured_web_dir          = config_get(CONFIG_SECTION_GLOBAL, "web files directory",    netdata_configured_web_dir);
    netdata_configured_cache_dir        = config_get(CONFIG_SECTION_GLOBAL, "cache directory",        netdata_configured_cache_dir);
    netdata_configured_varlib_dir       = config_get(CONFIG_SECTION_GLOBAL, "lib directory",          netdata_configured_varlib_dir);
    char *env_home=getenv("HOME");
    netdata_configured_home_dir         = config_get(CONFIG_SECTION_GLOBAL, "home directory",         env_home?env_home:netdata_configured_home_dir);

    netdata_configured_lock_dir = initialize_lock_directory_path(netdata_configured_varlib_dir);

    {
        pluginsd_initialize_plugin_directories();
        netdata_configured_primary_plugins_dir = plugin_directories[PLUGINSD_STOCK_PLUGINS_DIRECTORY_PATH];
    }

    // ------------------------------------------------------------------------
    // get default memory mode for the database

    default_rrd_memory_mode = rrd_memory_mode_id(config_get(CONFIG_SECTION_GLOBAL, "memory mode", rrd_memory_mode_name(default_rrd_memory_mode)));
#ifdef ENABLE_DBENGINE
    // ------------------------------------------------------------------------
    // get default Database Engine page cache size in MiB

    default_rrdeng_page_cache_mb = (int) config_get_number(CONFIG_SECTION_GLOBAL, "page cache size", default_rrdeng_page_cache_mb);
    if(default_rrdeng_page_cache_mb < RRDENG_MIN_PAGE_CACHE_SIZE_MB) {
        error("Invalid page cache size %d given. Defaulting to %d.", default_rrdeng_page_cache_mb, RRDENG_MIN_PAGE_CACHE_SIZE_MB);
        default_rrdeng_page_cache_mb = RRDENG_MIN_PAGE_CACHE_SIZE_MB;
    }

    // ------------------------------------------------------------------------
    // get default Database Engine disk space quota in MiB

    default_rrdeng_disk_quota_mb = (int) config_get_number(CONFIG_SECTION_GLOBAL, "dbengine disk space", default_rrdeng_disk_quota_mb);
    if(default_rrdeng_disk_quota_mb < RRDENG_MIN_DISK_SPACE_MB) {
        error("Invalid dbengine disk space %d given. Defaulting to %d.", default_rrdeng_disk_quota_mb, RRDENG_MIN_DISK_SPACE_MB);
        default_rrdeng_disk_quota_mb = RRDENG_MIN_DISK_SPACE_MB;
    }

    default_multidb_disk_quota_mb = (int) config_get_number(CONFIG_SECTION_GLOBAL, "dbengine multihost disk space", compute_multidb_diskspace());
    if(default_multidb_disk_quota_mb < RRDENG_MIN_DISK_SPACE_MB) {
        error("Invalid multidb disk space %d given. Defaulting to %d.", default_multidb_disk_quota_mb, default_rrdeng_disk_quota_mb);
        default_multidb_disk_quota_mb = default_rrdeng_disk_quota_mb;
    }
#else
    if (default_rrd_memory_mode == RRD_MEMORY_MODE_DBENGINE) {
       error_report("RRD_MEMORY_MODE_DBENGINE is not supported in this platform. The agent will use memory mode ram instead.");
       default_rrd_memory_mode = RRD_MEMORY_MODE_RAM;
    }
#endif
    // ------------------------------------------------------------------------

    netdata_configured_host_prefix = config_get(CONFIG_SECTION_GLOBAL, "host access prefix", "");
    verify_netdata_host_prefix();

    // --------------------------------------------------------------------
    // get KSM settings

#ifdef MADV_MERGEABLE
    enable_ksm = config_get_boolean(CONFIG_SECTION_GLOBAL, "memory deduplication (ksm)", enable_ksm);
#endif

    // --------------------------------------------------------------------
    // get various system parameters

    get_system_HZ();
    get_system_cpus();
    get_system_pid_max();


}

static int load_netdata_conf(char *filename, char overwrite_used) {
    errno = 0;

    int ret = 0;

    if(filename && *filename) {
        ret = config_load(filename, overwrite_used, NULL);
        if(!ret)
            error("CONFIG: cannot load config file '%s'.", filename);
    }
    else {
        filename = strdupz_path_subpath(netdata_configured_user_config_dir, "netdata.conf");

        ret = config_load(filename, overwrite_used, NULL);
        if(!ret) {
            info("CONFIG: cannot load user config '%s'. Will try the stock version.", filename);
            freez(filename);

            filename = strdupz_path_subpath(netdata_configured_stock_config_dir, "netdata.conf");
            ret = config_load(filename, overwrite_used, NULL);
            if(!ret)
                info("CONFIG: cannot load stock config '%s'. Running with internal defaults.", filename);
        }

        freez(filename);
    }

    return ret;
}

// coverity[ +tainted_string_sanitize_content : arg-0 ]
static inline void coverity_remove_taint(char *s)
{
    (void)s;
}

int get_system_info(struct rrdhost_system_info *system_info) {
    char *script;
    script = mallocz(sizeof(char) * (strlen(netdata_configured_primary_plugins_dir) + strlen("system-info.sh") + 2));
    sprintf(script, "%s/%s", netdata_configured_primary_plugins_dir, "system-info.sh");
    if (unlikely(access(script, R_OK) != 0)) {
        info("System info script %s not found.",script);
        freez(script);
        return 1;
    }

    pid_t command_pid;

    info("Executing %s", script);

    FILE *fp = mypopen(script, &command_pid);
    if(fp) {
        char line[200 + 1];
        // Removed the double strlens, if the Coverity tainted string warning reappears I'll revert.
        // One time init code, but I'm curious about the warning...
        while (fgets(line, 200, fp) != NULL) {
            char *value=line;
            while (*value && *value != '=') value++;
            if (*value=='=') {
                *value='\0';
                value++;
                char *end = value;
                while (*end && *end != '\n') end++;
                *end = '\0';    // Overwrite newline if present
                coverity_remove_taint(line);    // I/O is controlled result of system_info.sh - not tainted
                coverity_remove_taint(value);

                if(unlikely(rrdhost_set_system_info_variable(system_info, line, value))) {
                    info("Unexpected environment variable %s=%s", line, value);
                }
                else {
                    info("%s=%s", line, value);
                    setenv(line, value, 1);
                }
            }
        }
        mypclose(fp, command_pid);
    }
    freez(script);
    return 0;
}

void set_silencers_filename() {
    char filename[FILENAME_MAX + 1];
    snprintfz(filename, FILENAME_MAX, "%s/health.silencers.json", netdata_configured_varlib_dir);
    silencers_filename = config_get(CONFIG_SECTION_HEALTH, "silencers file", filename);
}

/* Any config setting that can be accessed without a default value i.e. configget(...,...,NULL) *MUST*
   be set in this procedure to be called in all the relevant code paths.
*/
void post_conf_load(char **user)
{
    // --------------------------------------------------------------------
    // get the user we should run

    // IMPORTANT: this is required before web_files_uid()
    if(getuid() == 0) {
        *user = config_get(CONFIG_SECTION_GLOBAL, "run as user", NETDATA_USER);
    }
    else {
        struct passwd *passwd = getpwuid(getuid());
        *user = config_get(CONFIG_SECTION_GLOBAL, "run as user", (passwd && passwd->pw_name)?passwd->pw_name:"");
    }

    // --------------------------------------------------------------------
    // Check if the cloud is enabled
#if defined( DISABLE_CLOUD ) || !defined( ENABLE_ACLK )
    netdata_cloud_setting = 0;
#else
    netdata_cloud_setting = appconfig_get_boolean(&cloud_config, CONFIG_SECTION_GLOBAL, "enabled", 1);
#endif
    // This must be set before any point in the code that accesses it. Do not move it from this function.
    appconfig_get(&cloud_config, CONFIG_SECTION_GLOBAL, "cloud base url", DEFAULT_CLOUD_BASE_URL);
}

int main(int argc, char **argv) {
    int i;
    int config_loaded = 0;
    int dont_fork = 0;
    size_t default_stacksize;
    char *user = NULL;


    netdata_ready=0;
    // set the name for logging
    program_name = "netdata";

    if (argc > 1 && strcmp(argv[1], SPAWN_SERVER_COMMAND_LINE_ARGUMENT) == 0) {
        // don't run netdata, this is the spawn server
        spawn_server();
        exit(0);
    }

    // parse options
    {
        int num_opts = sizeof(option_definitions) / sizeof(struct option_def);
        char optstring[(num_opts * 2) + 1];

        int string_i = 0;
        for( i = 0; i < num_opts; i++ ) {
            optstring[string_i] = option_definitions[i].val;
            string_i++;
            if(option_definitions[i].arg_name) {
                optstring[string_i] = ':';
                string_i++;
            }
        }
        // terminate optstring
        optstring[string_i] ='\0';
        optstring[(num_opts *2)] ='\0';

        int opt;
        while( (opt = getopt(argc, argv, optstring)) != -1 ) {
            switch(opt) {
                case 'c':
                    if(load_netdata_conf(optarg, 1) != 1) {
                        error("Cannot load configuration file %s.", optarg);
                        return 1;
                    }
                    else {
                        debug(D_OPTIONS, "Configuration loaded from %s.", optarg);
                        post_conf_load(&user);
                        load_cloud_conf(1);
                        config_loaded = 1;
                    }
                    break;
                case 'D':
                    dont_fork = 1;
                    break;
                case 'd':
                    dont_fork = 0;
                    break;
                case 'h':
                    return help(0);
                case 'i':
                    config_set(CONFIG_SECTION_WEB, "bind to", optarg);
                    break;
                case 'P':
                    strncpy(pidfile, optarg, FILENAME_MAX);
                    pidfile[FILENAME_MAX] = '\0';
                    break;
                case 'p':
                    config_set(CONFIG_SECTION_GLOBAL, "default port", optarg);
                    break;
                case 's':
                    config_set(CONFIG_SECTION_GLOBAL, "host access prefix", optarg);
                    break;
                case 't':
                    config_set(CONFIG_SECTION_GLOBAL, "update every", optarg);
                    break;
                case 'u':
                    config_set(CONFIG_SECTION_GLOBAL, "run as user", optarg);
                    break;
                case 'v':
                case 'V':
                    printf("%s %s\n", program_name, program_version);
                    return 0;
                case 'W':
                    {
                        char* stacksize_string = "stacksize=";
                        char* debug_flags_string = "debug_flags=";
                        char* claim_string = "claim";
#ifdef ENABLE_DBENGINE
                        char* createdataset_string = "createdataset=";
                        char* stresstest_string = "stresstest=";
#endif
                        if(strcmp(optarg, "sqlite-check") == 0) {
                            sql_init_database(DB_CHECK_INTEGRITY);
                            return 0;
                        }

                        if(strcmp(optarg, "sqlite-fix") == 0) {
                            sql_init_database(DB_CHECK_FIX_DB);
                            return 0;
                        }

                        if(strcmp(optarg, "sqlite-compact") == 0) {
                            sql_init_database(DB_CHECK_RECLAIM_SPACE);
                            return 0;
                        }

                        if(strcmp(optarg, "unittest") == 0) {
                            if(unit_test_buffer()) return 1;
                            if(unit_test_str2ld()) return 1;
                            // No call to load the config file on this code-path
                            post_conf_load(&user);
                            get_netdata_configured_variables();
                            default_rrd_update_every = 1;
                            default_rrd_memory_mode = RRD_MEMORY_MODE_RAM;
                            default_health_enabled = 0;
                            registry_init();
                            if(rrd_init("unittest", NULL)) {
                                fprintf(stderr, "rrd_init failed for unittest\n");
                                return 1;
                            }
                            default_rrdpush_enabled = 0;
                            if(run_all_mockup_tests()) return 1;
                            if(unit_test_storage()) return 1;
#ifdef ENABLE_DBENGINE
                            if(test_dbengine()) return 1;
#endif
                            if(test_sqlite()) return 1;
                            fprintf(stderr, "\n\nALL TESTS PASSED\n\n");
                            return 0;
                        }
#ifdef ENABLE_ML_TESTS
                        else if(strcmp(optarg, "mltest") == 0) {
                            return test_ml(argc, argv);
                        }
#endif
#ifdef ENABLE_DBENGINE
                        else if(strncmp(optarg, createdataset_string, strlen(createdataset_string)) == 0) {
                            optarg += strlen(createdataset_string);
                            unsigned history_seconds = strtoul(optarg, NULL, 0);
                            generate_dbengine_dataset(history_seconds);
                            return 0;
                        }
                        else if(strncmp(optarg, stresstest_string, strlen(stresstest_string)) == 0) {
                            char *endptr;
                            unsigned test_duration_sec = 0, dset_charts = 0, query_threads = 0, ramp_up_seconds = 0,
                            page_cache_mb = 0, disk_space_mb = 0;

                            optarg += strlen(stresstest_string);
                            test_duration_sec = (unsigned)strtoul(optarg, &endptr, 0);
                            if (',' == *endptr)
                                dset_charts = (unsigned)strtoul(endptr + 1, &endptr, 0);
                            if (',' == *endptr)
                                query_threads = (unsigned)strtoul(endptr + 1, &endptr, 0);
                            if (',' == *endptr)
                                ramp_up_seconds = (unsigned)strtoul(endptr + 1, &endptr, 0);
                            if (',' == *endptr)
                                page_cache_mb = (unsigned)strtoul(endptr + 1, &endptr, 0);
                            if (',' == *endptr)
                                disk_space_mb = (unsigned)strtoul(endptr + 1, &endptr, 0);

                            dbengine_stress_test(test_duration_sec, dset_charts, query_threads, ramp_up_seconds,
                                                 page_cache_mb, disk_space_mb);
                            return 0;
                        }
#endif
                        else if(strcmp(optarg, "simple-pattern") == 0) {
                            if(optind + 2 > argc) {
                                fprintf(stderr, "%s", "\nUSAGE: -W simple-pattern 'pattern' 'string'\n\n"
                                        " Checks if 'pattern' matches the given 'string'.\n"
                                        " - 'pattern' can be one or more space separated words.\n"
                                        " - each 'word' can contain one or more asterisks.\n"
                                        " - words starting with '!' give negative matches.\n"
                                        " - words are processed left to right\n"
                                        "\n"
                                        "Examples:\n"
                                        "\n"
                                        " > match all veth interfaces, except veth0:\n"
                                        "\n"
                                        "   -W simple-pattern '!veth0 veth*' 'veth12'\n"
                                        "\n"
                                        "\n"
                                        " > match all *.ext files directly in /path/:\n"
                                        "   (this will not match *.ext files in a subdir of /path/)\n"
                                        "\n"
                                        "   -W simple-pattern '!/path/*/*.ext /path/*.ext' '/path/test.ext'\n"
                                        "\n"
                                );
                                return 1;
                            }

                            const char *haystack = argv[optind];
                            const char *needle = argv[optind + 1];
                            size_t len = strlen(needle) + 1;
                            char wildcarded[len];

                            SIMPLE_PATTERN *p = simple_pattern_create(haystack, NULL, SIMPLE_PATTERN_EXACT);
                            int ret = simple_pattern_matches_extract(p, needle, wildcarded, len);
                            simple_pattern_free(p);

                            if(ret) {
                                fprintf(stdout, "RESULT: MATCHED - pattern '%s' matches '%s', wildcarded '%s'\n", haystack, needle, wildcarded);
                                return 0;
                            }
                            else {
                                fprintf(stdout, "RESULT: NOT MATCHED - pattern '%s' does not match '%s', wildcarded '%s'\n", haystack, needle, wildcarded);
                                return 1;
                            }
                        }
                        else if(strncmp(optarg, stacksize_string, strlen(stacksize_string)) == 0) {
                            optarg += strlen(stacksize_string);
                            config_set(CONFIG_SECTION_GLOBAL, "pthread stack size", optarg);
                        }
                        else if(strncmp(optarg, debug_flags_string, strlen(debug_flags_string)) == 0) {
                            optarg += strlen(debug_flags_string);
                            config_set(CONFIG_SECTION_GLOBAL, "debug flags",  optarg);
                            debug_flags = strtoull(optarg, NULL, 0);
                        }
                        else if(strcmp(optarg, "set") == 0) {
                            if(optind + 3 > argc) {
                                fprintf(stderr, "%s", "\nUSAGE: -W set 'section' 'key' 'value'\n\n"
                                        " Overwrites settings of netdata.conf.\n"
                                        "\n"
                                        " These options interact with: -c netdata.conf\n"
                                        " If -c netdata.conf is given on the command line,\n"
                                        " before -W set... the user may overwrite command\n"
                                        " line parameters at netdata.conf\n"
                                        " If -c netdata.conf is given after (or missing)\n"
                                        " -W set... the user cannot overwrite the command line\n"
                                        " parameters."
                                        "\n"
                                );
                                return 1;
                            }
                            const char *section = argv[optind];
                            const char *key = argv[optind + 1];
                            const char *value = argv[optind + 2];
                            optind += 3;

                            // set this one as the default
                            // only if it is not already set in the config file
                            // so the caller can use -c netdata.conf before or
                            // after this parameter to prevent or allow overwriting
                            // variables at netdata.conf
                            config_set_default(section, key,  value);

                            // fprintf(stderr, "SET section '%s', key '%s', value '%s'\n", section, key, value);
                        }
                        else if(strcmp(optarg, "set2") == 0) {
                            if(optind + 4 > argc) {
                                fprintf(stderr, "%s", "\nUSAGE: -W set 'conf_file' 'section' 'key' 'value'\n\n"
                                        " Overwrites settings of netdata.conf or cloud.conf\n"
                                        "\n"
                                        " These options interact with: -c netdata.conf\n"
                                        " If -c netdata.conf is given on the command line,\n"
                                        " before -W set... the user may overwrite command\n"
                                        " line parameters at netdata.conf\n"
                                        " If -c netdata.conf is given after (or missing)\n"
                                        " -W set... the user cannot overwrite the command line\n"
                                        " parameters."
                                        " conf_file can be \"cloud\" or \"netdata\".\n"
                                        "\n"
                                );
                                return 1;
                            }
                            const char *conf_file = argv[optind]; /* "cloud" is cloud.conf, otherwise netdata.conf */
                            struct config *tmp_config = strcmp(conf_file, "cloud") ? &netdata_config : &cloud_config;
                            const char *section = argv[optind + 1];
                            const char *key = argv[optind + 2];
                            const char *value = argv[optind + 3];
                            optind += 4;

                            // set this one as the default
                            // only if it is not already set in the config file
                            // so the caller can use -c netdata.conf before or
                            // after this parameter to prevent or allow overwriting
                            // variables at netdata.conf
                            appconfig_set_default(tmp_config, section, key,  value);

                            // fprintf(stderr, "SET section '%s', key '%s', value '%s'\n", section, key, value);
                        }
                        else if(strcmp(optarg, "get") == 0) {
                            if(optind + 3 > argc) {
                                fprintf(stderr, "%s", "\nUSAGE: -W get 'section' 'key' 'value'\n\n"
                                        " Prints settings of netdata.conf.\n"
                                        "\n"
                                        " These options interact with: -c netdata.conf\n"
                                        " -c netdata.conf has to be given before -W get.\n"
                                        "\n"
                                );
                                return 1;
                            }

                            if(!config_loaded) {
                                fprintf(stderr, "warning: no configuration file has been loaded. Use -c CONFIG_FILE, before -W get. Using default config.\n");
                                load_netdata_conf(NULL, 0);
                                post_conf_load(&user);
                            }

                            get_netdata_configured_variables();

                            const char *section = argv[optind];
                            const char *key = argv[optind + 1];
                            const char *def = argv[optind + 2];
                            const char *value = config_get(section, key, def);
                            printf("%s\n", value);
                            return 0;
                        }
                        else if(strcmp(optarg, "get2") == 0) {
                            if(optind + 4 > argc) {
                                fprintf(stderr, "%s", "\nUSAGE: -W get2 'conf_file' 'section' 'key' 'value'\n\n"
                                        " Prints settings of netdata.conf or cloud.conf\n"
                                        "\n"
                                        " These options interact with: -c netdata.conf\n"
                                        " -c netdata.conf has to be given before -W get2.\n"
                                        " conf_file can be \"cloud\" or \"netdata\".\n"
                                        "\n"
                                );
                                return 1;
                            }

                            if(!config_loaded) {
                                fprintf(stderr, "warning: no configuration file has been loaded. Use -c CONFIG_FILE, before -W get. Using default config.\n");
                                load_netdata_conf(NULL, 0);
                                post_conf_load(&user);
                                load_cloud_conf(1);
                            }

                            get_netdata_configured_variables();

                            const char *conf_file = argv[optind]; /* "cloud" is cloud.conf, otherwise netdata.conf */
                            struct config *tmp_config = strcmp(conf_file, "cloud") ? &netdata_config : &cloud_config;
                            const char *section = argv[optind + 1];
                            const char *key = argv[optind + 2];
                            const char *def = argv[optind + 3];
                            const char *value = appconfig_get(tmp_config, section, key, def);
                            printf("%s\n", value);
                            return 0;
                        }
                        else if(strncmp(optarg, claim_string, strlen(claim_string)) == 0) {
                            /* will trigger a claiming attempt when the agent is initialized */
                            claiming_pending_arguments = optarg + strlen(claim_string);
                        }
                        else if(strcmp(optarg, "buildinfo") == 0) {
                            printf("Version: %s %s\n", program_name, program_version);
                            print_build_info();
                            return 0;
                        }
                        else if(strcmp(optarg, "buildinfojson") == 0) {
                            print_build_info_json();
                            return 0;
                        }
                        else {
                            fprintf(stderr, "Unknown -W parameter '%s'\n", optarg);
                            return help(1);
                        }
                    }
                    break;

                default: /* ? */
                    fprintf(stderr, "Unknown parameter '%c'\n", opt);
                    return help(1);
            }
        }
    }

#ifdef _SC_OPEN_MAX
    // close all open file descriptors, except the standard ones
    // the caller may have left open files (lxc-attach has this issue)
    {
        int fd;
        for(fd = (int) (sysconf(_SC_OPEN_MAX) - 1); fd > 2; fd--)
            if(fd_is_valid(fd)) close(fd);
    }
#endif

    if(!config_loaded)
    {
        load_netdata_conf(NULL, 0);
        post_conf_load(&user);
        load_cloud_conf(0);
    }


    // ------------------------------------------------------------------------
    // initialize netdata
    {
        char *pmax = config_get(CONFIG_SECTION_GLOBAL, "glibc malloc arena max for plugins", "1");
        if(pmax && *pmax)
            setenv("MALLOC_ARENA_MAX", pmax, 1);

#if defined(HAVE_C_MALLOPT)
        i = (int)config_get_number(CONFIG_SECTION_GLOBAL, "glibc malloc arena max for netdata", 1);
        if(i > 0)
            mallopt(M_ARENA_MAX, 1);
#endif
        test_clock_boottime();
        test_clock_monotonic_coarse();

        // prepare configuration environment variables for the plugins

        get_netdata_configured_variables();
        set_global_environment();

        // work while we are cd into config_dir
        // to allow the plugins refer to their config
        // files using relative filenames
        if(chdir(netdata_configured_user_config_dir) == -1)
            fatal("Cannot cd to '%s'", netdata_configured_user_config_dir);

        // Get execution path before switching user to avoid permission issues
        get_netdata_execution_path();
    }

    {
        // --------------------------------------------------------------------
        // get the debugging flags from the configuration file

        char *flags = config_get(CONFIG_SECTION_GLOBAL, "debug flags",  "0x0000000000000000");
        setenv("NETDATA_DEBUG_FLAGS", flags, 1);

        debug_flags = strtoull(flags, NULL, 0);
        debug(D_OPTIONS, "Debug flags set to '0x%" PRIX64 "'.", debug_flags);

        if(debug_flags != 0) {
            struct rlimit rl = { RLIM_INFINITY, RLIM_INFINITY };
            if(setrlimit(RLIMIT_CORE, &rl) != 0)
                error("Cannot request unlimited core dumps for debugging... Proceeding anyway...");

#ifdef HAVE_SYS_PRCTL_H
            prctl(PR_SET_DUMPABLE, 1, 0, 0, 0);
#endif
        }


        // --------------------------------------------------------------------
        // get log filenames and settings
        log_init();
        error_log_limit_unlimited();
        // initialize the log files
        open_all_log_files();

        get_system_timezone();
        // --------------------------------------------------------------------
        // get the certificate and start security
#ifdef ENABLE_HTTPS
        security_init();
#endif

        // --------------------------------------------------------------------
        // This is the safest place to start the SILENCERS structure
        set_silencers_filename();
        health_initialize_global_silencers();

        // --------------------------------------------------------------------
        // Initialize ML configuration
        ml_init();

        // --------------------------------------------------------------------
        // setup process signals

        // block signals while initializing threads.
        // this causes the threads to block signals.
        signals_block();

        // setup the signals we want to use
        signals_init();

        // setup threads configs
        default_stacksize = netdata_threads_init();


        // --------------------------------------------------------------------
        // check which threads are enabled and initialize them

        for (i = 0; static_threads[i].name != NULL ; i++) {
            struct netdata_static_thread *st = &static_threads[i];

            if(st->config_name)
                st->enabled = config_get_boolean(st->config_section, st->config_name, st->enabled);

            if(st->enabled && st->init_routine)
                st->init_routine();
        }


        // --------------------------------------------------------------------
        // create the listening sockets

        web_client_api_v1_init();
        web_server_threading_selection();

        if(web_server_mode != WEB_SERVER_MODE_NONE)
            api_listen_sockets_setup();
    }

#ifdef NETDATA_INTERNAL_CHECKS
    if(debug_flags != 0) {
        struct rlimit rl = { RLIM_INFINITY, RLIM_INFINITY };
        if(setrlimit(RLIMIT_CORE, &rl) != 0)
            error("Cannot request unlimited core dumps for debugging... Proceeding anyway...");
#ifdef HAVE_SYS_PRCTL_H
        prctl(PR_SET_DUMPABLE, 1, 0, 0, 0);
#endif
    }
#endif /* NETDATA_INTERNAL_CHECKS */

    // get the max file limit
    if(getrlimit(RLIMIT_NOFILE, &rlimit_nofile) != 0)
        error("getrlimit(RLIMIT_NOFILE) failed");
    else
        info("resources control: allowed file descriptors: soft = %zu, max = %zu", (size_t)rlimit_nofile.rlim_cur, (size_t)rlimit_nofile.rlim_max);

    // fork, switch user, create pid file, set process priority
    if(become_daemon(dont_fork, user) == -1)
        fatal("Cannot daemonize myself.");

    info("netdata started on pid %d.", getpid());

    // IMPORTANT: these have to run once, while single threaded
    // but after we have switched user
    web_files_uid();
    web_files_gid();

    netdata_threads_init_after_fork((size_t)config_get_number(CONFIG_SECTION_GLOBAL, "pthread stack size", (long)default_stacksize));

    // initialize internal registry
    registry_init();
    // fork the spawn server
    spawn_init();
    /*
     * Libuv uv_spawn() uses SIGCHLD internally:
     * https://github.com/libuv/libuv/blob/cc51217a317e96510fbb284721d5e6bc2af31e33/src/unix/process.c#L485
     * and inadvertently replaces the netdata signal handler which was setup during initialization.
     * Thusly, we must explicitly restore the signal handler for SIGCHLD.
     * Warning: extreme care is needed when mixing and matching POSIX and libuv.
     */
    signals_restore_SIGCHLD();

    // ------------------------------------------------------------------------
    // initialize rrd, registry, health, rrdpush, etc.

    netdata_anonymous_statistics_enabled=-1;
    struct rrdhost_system_info *system_info = calloc(1, sizeof(struct rrdhost_system_info));
    get_system_info(system_info);
    system_info->hops = 0;

    if(rrd_init(netdata_configured_hostname, system_info))
        fatal("Cannot initialize localhost instance with name '%s'.", netdata_configured_hostname);

    char agent_crash_file[FILENAME_MAX + 1];
    char agent_incomplete_shutdown_file[FILENAME_MAX + 1];
    snprintfz(agent_incomplete_shutdown_file, FILENAME_MAX, "%s/.agent_incomplete_shutdown", netdata_configured_varlib_dir);
    int incomplete_shutdown_detected = (unlink(agent_incomplete_shutdown_file) == 0);
    snprintfz(agent_crash_file, FILENAME_MAX, "%s/.agent_crash", netdata_configured_varlib_dir);
    int crash_detected = (unlink(agent_crash_file) == 0);
    int fd = open(agent_crash_file, O_WRONLY | O_CREAT | O_TRUNC, 444);
    if (fd >= 0)
        close(fd);


    // ------------------------------------------------------------------------
    // Claim netdata agent to a cloud endpoint

    if (claiming_pending_arguments)
         claim_agent(claiming_pending_arguments);
    load_claiming_state();

    // ------------------------------------------------------------------------
    // enable log flood protection

    error_log_limit_reset();

    // Load host labels
    reload_host_labels();

    // ------------------------------------------------------------------------
    // spawn the threads

    web_server_config_options();

    netdata_zero_metrics_enabled = config_get_boolean_ondemand(CONFIG_SECTION_GLOBAL, "enable zero metrics", CONFIG_BOOLEAN_NO);

    set_late_global_environment();

    for (i = 0; static_threads[i].name != NULL ; i++) {
        struct netdata_static_thread *st = &static_threads[i];

        if(st->enabled) {
            st->thread = mallocz(sizeof(netdata_thread_t));
            debug(D_SYSTEM, "Starting thread %s.", st->name);
            netdata_thread_create(st->thread, st->name, NETDATA_THREAD_OPTION_DEFAULT, st->start_routine, st);
        }
        else debug(D_SYSTEM, "Not starting thread %s.", st->name);
    }

    // ------------------------------------------------------------------------
    // Initialize netdata agent command serving from cli and signals

    commands_init();

    info("netdata initialization completed. Enjoy real-time performance monitoring!");
    netdata_ready = 1;

    send_statistics("START", "-",  "-");
    if (crash_detected)
        send_statistics("CRASH", "-", "-");
    if (incomplete_shutdown_detected)
        send_statistics("INCOMPLETE_SHUTDOWN", "-", "-");

    //check if ANALYTICS needs to start
    if (netdata_anonymous_statistics_enabled == 1) {
        for (i = 0; static_threads[i].name != NULL; i++) {
            if (!strncmp(static_threads[i].name, "ANALYTICS", 9)) {
                struct netdata_static_thread *st = &static_threads[i];
                st->thread = mallocz(sizeof(netdata_thread_t));
                st->enabled = 1;
                debug(D_SYSTEM, "Starting thread %s.", st->name);
                netdata_thread_create(st->thread, st->name, NETDATA_THREAD_OPTION_DEFAULT, st->start_routine, st);
            }
        }
    }

    // ------------------------------------------------------------------------
    // Report ACLK build failure
#ifndef ENABLE_ACLK
    error("This agent doesn't have ACLK.");
    char filename[FILENAME_MAX + 1];
    snprintfz(filename, FILENAME_MAX, "%s/.aclk_report_sent", netdata_configured_varlib_dir);
    if (netdata_anonymous_statistics_enabled > 0 && access(filename, F_OK)) { // -1 -> not initialized
        send_statistics("ACLK_DISABLED", "-", "-");
#ifdef ACLK_NO_LWS
        send_statistics("BUILD_FAIL_LWS", "-", "-");
#endif
#ifdef ACLK_NO_LIBMOSQ
        send_statistics("BUILD_FAIL_MOSQ", "-", "-");
#endif
        int fd = open(filename, O_WRONLY | O_CREAT | O_TRUNC, 444);
        if (fd == -1)
            error("Cannot create file '%s'. Please fix this.", filename);
        else
            close(fd);
    }
#endif

    // ------------------------------------------------------------------------
    // unblock signals

    signals_unblock();

    // ------------------------------------------------------------------------
    // Handle signals

    signals_handle();

    // should never reach this point
    // but we need it for rpmlint #2752
    return 1;
}
