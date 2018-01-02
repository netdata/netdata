#include "common.h"

extern void *cgroups_main(void *ptr);

void netdata_cleanup_and_exit(int ret) {
    netdata_exit = 1;

    error_log_limit_unlimited();
    info("EXIT: netdata prepares to exit with code %d...", ret);

    if(ret) {
        // this is bad - exiting due to a fatal condition

        // cleanup/save the database and exit
        info("EXIT: cleaning up the database...");
        rrdhost_cleanup_all();
    }
    else {
        // exit cleanly

        // stop everything
        info("EXIT: stopping master threads...");
        cancel_main_threads();

        // cleanup the database (delete files not needed)
        info("EXIT: cleaning up the database...");
        rrdhost_cleanup_all();

        // free the database
        info("EXIT: freeing database memory...");
        rrdhost_free_all();
    }

    // unlink the pid
    if(pidfile[0]) {
        info("EXIT: removing netdata PID file '%s'...", pidfile);
        if(unlink(pidfile) != 0)
            error("EXIT: cannot unlink pidfile '%s'.", pidfile);
    }

    info("EXIT: all done - netdata is now exiting - bye bye...");
    exit(ret);
}

struct netdata_static_thread static_threads[] = {

#ifdef INTERNAL_PLUGIN_NFACCT
    // nfacct requires root access
    // so, we build it as an external plugin with setuid to root
    {"PLUGIN[nfacct]",       CONFIG_SECTION_PLUGINS,  "nfacct",     1, NULL, NULL, nfacct_main},
#endif

#ifdef NETDATA_INTERNAL_CHECKS
    // debugging plugin
    {"PLUGIN[check]",        CONFIG_SECTION_PLUGINS,  "checks",     0, NULL, NULL, checks_main},
#endif

#if defined(__FreeBSD__)
    // FreeBSD internal plugins
    {"PLUGIN[freebsd]",      CONFIG_SECTION_PLUGINS,  "freebsd",    1, NULL, NULL, freebsd_main},
#elif defined(__APPLE__)
    // macOS internal plugins
    {"PLUGIN[macos]",        CONFIG_SECTION_PLUGINS,  "macos",      1, NULL, NULL, macos_main},
#else
    // linux internal plugins
    {"PLUGIN[proc]",         CONFIG_SECTION_PLUGINS,  "proc",       1, NULL, NULL, proc_main},
    {"PLUGIN[diskspace]",    CONFIG_SECTION_PLUGINS,  "diskspace",  1, NULL, NULL, proc_diskspace_main},
    {"PLUGIN[cgroup]",       CONFIG_SECTION_PLUGINS,  "cgroups",    1, NULL, NULL, cgroups_main},
    {"PLUGIN[tc]",           CONFIG_SECTION_PLUGINS,  "tc",         1, NULL, NULL, tc_main},
#endif /* __FreeBSD__, __APPLE__*/

    // common plugins for all systems
    {"PLUGIN[idlejitter]",   CONFIG_SECTION_PLUGINS,  "idlejitter", 1, NULL, NULL, cpuidlejitter_main},
    {"BACKENDS",            NULL,                    NULL,         1, NULL, NULL, backends_main},
    {"HEALTH",              NULL,                    NULL,         1, NULL, NULL, health_main},
    {"PLUGINSD",            NULL,                    NULL,         1, NULL, NULL, pluginsd_main},
    {"WEB_SERVER[multi]",   NULL,                    NULL,         1, NULL, NULL, socket_listen_main_multi_threaded},
    {"WEB_SERVER[single]",  NULL,                    NULL,         0, NULL, NULL, socket_listen_main_single_threaded},
    {"STREAM",              NULL,                    NULL,         0, NULL, NULL, rrdpush_sender_thread},
    {"STATSD",              NULL,                    NULL,         1, NULL, NULL, statsd_main},

    {NULL,                  NULL,                    NULL,         0, NULL, NULL, NULL}
};

void web_server_threading_selection(void) {
    web_server_mode = web_server_mode_id(config_get(CONFIG_SECTION_WEB, "mode", web_server_mode_name(web_server_mode)));

    int multi_threaded = (web_server_mode == WEB_SERVER_MODE_MULTI_THREADED);
    int single_threaded = (web_server_mode == WEB_SERVER_MODE_SINGLE_THREADED);

    int i;
    for (i = 0; static_threads[i].name; i++) {
        if (static_threads[i].start_routine == socket_listen_main_multi_threaded)
            static_threads[i].enabled = multi_threaded;

        if (static_threads[i].start_routine == socket_listen_main_single_threaded)
            static_threads[i].enabled = single_threaded;
    }
}

void web_server_config_options(void) {
    web_client_timeout = (int) config_get_number(CONFIG_SECTION_WEB, "disconnect idle clients after seconds", DEFAULT_DISCONNECT_IDLE_WEB_CLIENTS_AFTER_SECONDS);

    respect_web_browser_do_not_track_policy = config_get_boolean(CONFIG_SECTION_WEB, "respect do not track policy", respect_web_browser_do_not_track_policy);
    web_x_frame_options = config_get(CONFIG_SECTION_WEB, "x-frame-options response header", "");
    if(!*web_x_frame_options) web_x_frame_options = NULL;

    web_allow_connections_from = simple_pattern_create(config_get(CONFIG_SECTION_WEB, "allow connections from", "localhost *"), NULL, SIMPLE_PATTERN_EXACT);
    web_allow_dashboard_from   = simple_pattern_create(config_get(CONFIG_SECTION_WEB, "allow dashboard from", "localhost *"), NULL, SIMPLE_PATTERN_EXACT);
    web_allow_badges_from      = simple_pattern_create(config_get(CONFIG_SECTION_WEB, "allow badges from", "*"), NULL, SIMPLE_PATTERN_EXACT);
    web_allow_registry_from    = simple_pattern_create(config_get(CONFIG_SECTION_REGISTRY, "allow from", "*"), NULL, SIMPLE_PATTERN_EXACT);
    web_allow_streaming_from   = simple_pattern_create(config_get(CONFIG_SECTION_WEB, "allow streaming from", "*"), NULL, SIMPLE_PATTERN_EXACT);
    web_allow_netdataconf_from = simple_pattern_create(config_get(CONFIG_SECTION_WEB, "allow netdata.conf from", "localhost fd* 10.* 192.168.* 172.16.* 172.17.* 172.18.* 172.19.* 172.20.* 172.21.* 172.22.* 172.23.* 172.24.* 172.25.* 172.26.* 172.27.* 172.28.* 172.29.* 172.30.* 172.31.*"), NULL, SIMPLE_PATTERN_EXACT);

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


int killpid(pid_t pid, int sig)
{
    int ret = -1;
    debug(D_EXIT, "Request to kill pid %d", pid);

    errno = 0;
    if(kill(pid, 0) == -1) {
        switch(errno) {
            case ESRCH:
                error("Request to kill pid %d, but it is not running.", pid);
                break;

            case EPERM:
                error("Request to kill pid %d, but I do not have enough permissions.", pid);
                break;

            default:
                error("Request to kill pid %d, but I received an error.", pid);
                break;
        }
    }
    else {
        errno = 0;
        ret = kill(pid, sig);
        if(ret == -1) {
            switch(errno) {
                case ESRCH:
                    error("Cannot kill pid %d, but it is not running.", pid);
                    break;

                case EPERM:
                    error("Cannot kill pid %d, but I do not have enough permissions.", pid);
                    break;

                default:
                    error("Cannot kill pid %d, but I received an error.", pid);
                    break;
            }
        }
    }

    return ret;
}

void cancel_main_threads() {
    error_log_limit_unlimited();

    int i;
    for (i = 0; static_threads[i].name != NULL ; i++) {
        if(static_threads[i].enabled) {
            info("EXIT: Stopping master thread: %s", static_threads[i].name);
            netdata_thread_cancel(*static_threads[i].thread);
            static_threads[i].enabled = 0;
        }
    }
}

struct option_def option_definitions[] = {
    // opt description                                    arg name       default value
    { 'c', "Configuration file to load.",                 "filename",    CONFIG_DIR "/" CONFIG_FILENAME},
    { 'D', "Do not fork. Run in the foreground.",         NULL,          "run in the background"},
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
            " Copyright (C) 2016-2017, Costa Tsaousis <costa@tsaousis.gr>\n"
            " Released under GNU General Public License v3 or later.\n"
            " All rights reserved.\n"
            "\n"
            " Home Page  : https://my-netdata.io\n"
            " Source Code: https://github.com/firehol/netdata\n"
            " Wiki / Docs: https://github.com/firehol/netdata/wiki\n"
            " Support    : https://github.com/firehol/netdata/issues\n"
            " License    : https://github.com/firehol/netdata/blob/master/LICENSE.md\n"
            "\n"
            " Twitter    : https://twitter.com/linuxnetdata\n"
            " Facebook   : https://www.facebook.com/linuxnetdata/\n"
            "\n"
            " netdata is a https://firehol.org project.\n"
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
            "  -W set section option value\n"
            "                           set netdata.conf option from the command line.\n\n"
            "  -W simple-pattern pattern string\n"
            "                           Check if string matches pattern and exit.\n\n"
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

// TODO: Remove this function with the nix major release.
void remove_option(int opt_index, int *argc, char **argv) {
    int i = opt_index;
    // remove the options.
    do {
        *argc = *argc - 1;
        for(i = opt_index; i < *argc; i++) {
            argv[i] = argv[i+1];
        }
        i = opt_index;
    } while(argv[i][0] != '-' && opt_index >= *argc);
}

static const char *verify_required_directory(const char *dir) {
    if(chdir(dir) == -1)
        fatal("Cannot cd to directory '%s'", dir);

    DIR *d = opendir(dir);
    if(!d)
        fatal("Cannot examine the contents of directory '%s'", dir);
    closedir(d);

    return dir;
}

void log_init(void) {
    char filename[FILENAME_MAX + 1];
    snprintfz(filename, FILENAME_MAX, "%s/debug.log", netdata_configured_log_dir);
    stdout_filename    = config_get(CONFIG_SECTION_GLOBAL, "debug log",  filename);

    snprintfz(filename, FILENAME_MAX, "%s/error.log", netdata_configured_log_dir);
    stderr_filename    = config_get(CONFIG_SECTION_GLOBAL, "error log",  filename);

    snprintfz(filename, FILENAME_MAX, "%s/access.log", netdata_configured_log_dir);
    stdaccess_filename = config_get(CONFIG_SECTION_GLOBAL, "access log", filename);

    error_log_throttle_period_backup =
    error_log_throttle_period = config_get_number(CONFIG_SECTION_GLOBAL, "errors flood protection period", error_log_throttle_period);
    error_log_errors_per_period = (unsigned long)config_get_number(CONFIG_SECTION_GLOBAL, "errors to trigger flood protection", (long long int)error_log_errors_per_period);

    setenv("NETDATA_ERRORS_THROTTLE_PERIOD", config_get(CONFIG_SECTION_GLOBAL, "errors flood protection period"    , ""), 1);
    setenv("NETDATA_ERRORS_PER_PERIOD",      config_get(CONFIG_SECTION_GLOBAL, "errors to trigger flood protection", ""), 1);
}

static void backwards_compatible_config() {
    // allow existing configurations to work with the current version of netdata

    if(config_exists(CONFIG_SECTION_GLOBAL, "multi threaded web server")) {
        int mode = config_get_boolean(CONFIG_SECTION_GLOBAL, "multi threaded web server", 1);
        web_server_mode = (mode)?WEB_SERVER_MODE_MULTI_THREADED:WEB_SERVER_MODE_SINGLE_THREADED;
    }

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
    if(gethostname(buf, HOSTNAME_MAX) == -1)
        error("Cannot get machine hostname.");

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

    netdata_configured_config_dir  = config_get(CONFIG_SECTION_GLOBAL, "config directory",    CONFIG_DIR);
    netdata_configured_log_dir     = config_get(CONFIG_SECTION_GLOBAL, "log directory",       LOG_DIR);
    netdata_configured_web_dir     = config_get(CONFIG_SECTION_GLOBAL, "web files directory", WEB_DIR);
    netdata_configured_cache_dir   = config_get(CONFIG_SECTION_GLOBAL, "cache directory",     CACHE_DIR);
    netdata_configured_varlib_dir  = config_get(CONFIG_SECTION_GLOBAL, "lib directory",       VARLIB_DIR);
    netdata_configured_home_dir    = config_get(CONFIG_SECTION_GLOBAL, "home directory",      CACHE_DIR);

    {
        char plugins_dirs[(FILENAME_MAX * 2) + 1];
        snprintfz(plugins_dirs, FILENAME_MAX * 2, "\"%s\" \"%s/custom-plugins.d\"", PLUGINS_DIR, CONFIG_DIR);
        netdata_configured_plugins_dir_base = strdupz(config_get(CONFIG_SECTION_GLOBAL, "plugins directory",  plugins_dirs));
        quoted_strings_splitter(netdata_configured_plugins_dir_base, plugin_directories, PLUGINSD_MAX_DIRECTORIES, config_isspace);
        netdata_configured_plugins_dir = plugin_directories[0];
    }

    // ------------------------------------------------------------------------
    // get default memory mode for the database

    default_rrd_memory_mode = rrd_memory_mode_id(config_get(CONFIG_SECTION_GLOBAL, "memory mode", rrd_memory_mode_name(default_rrd_memory_mode)));

    // ------------------------------------------------------------------------

    netdata_configured_host_prefix = config_get(CONFIG_SECTION_GLOBAL, "host access prefix", "");

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

static void get_system_timezone(void) {
    // avoid flood calls to stat(/etc/localtime)
    // http://stackoverflow.com/questions/4554271/how-to-avoid-excessive-stat-etc-localtime-calls-in-strftime-on-linux
    const char *tz = getenv("TZ");
    if(!tz || !*tz)
        setenv("TZ", config_get(CONFIG_SECTION_GLOBAL, "TZ environment variable", ":/etc/localtime"), 0);

    char buffer[FILENAME_MAX + 1] = "";
    const char *timezone = NULL;
    ssize_t ret;

    // use the TZ variable
    if(tz && *tz && *tz != ':') {
        timezone = tz;
        // info("TIMEZONE: using TZ variable '%s'", timezone);
    }

    // use the contents of /etc/timezone
    if(!timezone && !read_file("/etc/timezone", buffer, FILENAME_MAX)) {
        timezone = buffer;
        // info("TIMEZONE: using the contents of /etc/timezone: '%s'", timezone);
    }

    // read the link /etc/localtime
    if(!timezone) {
        ret = readlink("/etc/localtime", buffer, FILENAME_MAX);

        if(ret > 0) {
            buffer[ret] = '\0';

            char   *cmp    = "/usr/share/zoneinfo/";
            size_t cmp_len = strlen(cmp);

            char *s = strstr(buffer, cmp);
            if (s && s[cmp_len]) {
                timezone = &s[cmp_len];
                // info("TIMEZONE: using the link of /etc/localtime: '%s'", timezone);
            }
        }
        else
            buffer[0] = '\0';
    }

    // find the timezone from strftime()
    if(!timezone) {
        time_t t;
        struct tm *tmp, tmbuf;

        t = now_realtime_sec();
        tmp = localtime_r(&t, &tmbuf);

        if (tmp != NULL) {
            if(strftime(buffer, FILENAME_MAX, "%Z", tmp) == 0)
                buffer[0] = '\0';
            else {
                buffer[FILENAME_MAX] = '\0';
                timezone = buffer;
                // info("TIMEZONE: using strftime(): '%s'", timezone);
            }
        }
    }

    if(timezone && *timezone) {
        // make sure it does not have illegal characters
        // info("TIMEZONE: fixing '%s'", timezone);

        size_t len = strlen(timezone);
        char tmp[len + 1];
        char *d = tmp;
        *d = '\0';

        while(*timezone) {
            if(isalnum(*timezone) || *timezone == '_' || *timezone == '/')
                *d++ = *timezone++;
            else
                timezone++;
        }
        *d = '\0';
        strncpyz(buffer, tmp, len);
        timezone = buffer;
        // info("TIMEZONE: fixed as '%s'", timezone);
    }

    if(!timezone || !*timezone)
        timezone = "unknown";

    netdata_configured_timezone = config_get(CONFIG_SECTION_GLOBAL, "timezone", timezone);
}

void set_global_environment() {
    {
        char b[16];
        snprintfz(b, 15, "%d", default_rrd_update_every);
        setenv("NETDATA_UPDATE_EVERY", b, 1);
    }

    setenv("NETDATA_HOSTNAME"   , netdata_configured_hostname, 1);
    setenv("NETDATA_CONFIG_DIR" , verify_required_directory(netdata_configured_config_dir),  1);
    setenv("NETDATA_PLUGINS_DIR", verify_required_directory(netdata_configured_plugins_dir), 1);
    setenv("NETDATA_WEB_DIR"    , verify_required_directory(netdata_configured_web_dir),     1);
    setenv("NETDATA_CACHE_DIR"  , verify_required_directory(netdata_configured_cache_dir),   1);
    setenv("NETDATA_LIB_DIR"    , verify_required_directory(netdata_configured_varlib_dir),  1);
    setenv("NETDATA_LOG_DIR"    , verify_required_directory(netdata_configured_log_dir),     1);
    setenv("HOME"               , verify_required_directory(netdata_configured_home_dir),    1);
    setenv("NETDATA_HOST_PREFIX", netdata_configured_host_prefix, 1);

    get_system_timezone();

    // set the path we need
    char path[1024 + 1], *p = getenv("PATH");
    if(!p) p = "/bin:/usr/bin";
    snprintfz(path, 1024, "%s:%s", p, "/sbin:/usr/sbin:/usr/local/bin:/usr/local/sbin");
    setenv("PATH", config_get(CONFIG_SECTION_PLUGINS, "PATH environment variable", path), 1);

    // python options
    p = getenv("PYTHONPATH");
    if(!p) p = "";
    setenv("PYTHONPATH", config_get(CONFIG_SECTION_PLUGINS, "PYTHONPATH environment variable", p), 1);

    // disable buffering for python plugins
    setenv("PYTHONUNBUFFERED", "1", 1);

    // switch to standard locale for plugins
    setenv("LC_ALL", "C", 1);
}

int main(int argc, char **argv) {
    int i;
    int config_loaded = 0;
    int dont_fork = 0;
    size_t default_stacksize;

    // set the name for logging
    program_name = "netdata";

    // parse depercated options
    // TODO: Remove this block with the next major release.
    {
        i = 1;
        while(i < argc) {
            if(strcmp(argv[i], "-pidfile") == 0 && (i+1) < argc) {
                strncpyz(pidfile, argv[i+1], FILENAME_MAX);
                fprintf(stderr, "%s: deprecated option -- %s -- please use -P instead.\n", argv[0], argv[i]);
                remove_option(i, &argc, argv);
            }
            else if(strcmp(argv[i], "-nodaemon") == 0 || strcmp(argv[i], "-nd") == 0) {
                dont_fork = 1;
                fprintf(stderr, "%s: deprecated option -- %s -- please use -D instead.\n ", argv[0], argv[i]);
                remove_option(i, &argc, argv);
            }
            else if(strcmp(argv[i], "-ch") == 0 && (i+1) < argc) {
                config_set(CONFIG_SECTION_GLOBAL, "host access prefix", argv[i+1]);
                fprintf(stderr, "%s: deprecated option -- %s -- please use -s instead.\n", argv[0], argv[i]);
                remove_option(i, &argc, argv);
            }
            else if(strcmp(argv[i], "-l") == 0 && (i+1) < argc) {
                config_set(CONFIG_SECTION_GLOBAL, "history", argv[i+1]);
                fprintf(stderr, "%s: deprecated option -- %s -- This option will be removed with V2.*.\n", argv[0], argv[i]);
                remove_option(i, &argc, argv);
            }
            else i++;
        }
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
                    if(config_load(optarg, 1) != 1) {
                        error("Cannot load configuration file %s.", optarg);
                        return 1;
                    }
                    else {
                        debug(D_OPTIONS, "Configuration loaded from %s.", optarg);
                        config_loaded = 1;
                    }
                    break;
                case 'D':
                    dont_fork = 1;
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

                        if(strcmp(optarg, "unittest") == 0) {
                            if(unit_test_buffer()) return 1;
                            if(unit_test_str2ld()) return 1;
                            //default_rrd_update_every = 1;
                            //default_rrd_memory_mode = RRD_MEMORY_MODE_RAM;
                            //if(!config_loaded) config_load(NULL, 0);
                            get_netdata_configured_variables();
                            default_rrd_update_every = 1;
                            default_rrd_memory_mode = RRD_MEMORY_MODE_RAM;
                            default_health_enabled = 0;
                            rrd_init("unittest");
                            default_rrdpush_enabled = 0;
                            if(run_all_mockup_tests()) return 1;
                            if(unit_test_storage()) return 1;
                            fprintf(stderr, "\n\nALL TESTS PASSED\n\n");
                            return 0;
                        }
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

                            const char *heystack = argv[optind];
                            const char *needle = argv[optind + 1];
                            size_t len = strlen(needle) + 1;
                            char wildcarded[len];

                            SIMPLE_PATTERN *p = simple_pattern_create(heystack, NULL, SIMPLE_PATTERN_EXACT);
                            int ret = simple_pattern_matches_extract(p, needle, wildcarded, len);
                            simple_pattern_free(p);

                            if(ret) {
                                fprintf(stdout, "RESULT: MATCHED - pattern '%s' matches '%s', wildcarded '%s'\n", heystack, needle, wildcarded);
                                return 0;
                            }
                            else {
                                fprintf(stdout, "RESULT: NOT MATCHED - pattern '%s' does not match '%s', wildcarded '%s'\n", heystack, needle, wildcarded);
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
                                config_load(NULL, 0);
                            }

                            backwards_compatible_config();
                            get_netdata_configured_variables();

                            const char *section = argv[optind];
                            const char *key = argv[optind + 1];
                            const char *def = argv[optind + 2];
                            const char *value = config_get(section, key, def);
                            printf("%s\n", value);
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
        config_load(NULL, 0);

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

        // prepare configuration environment variables for the plugins

        get_netdata_configured_variables();
        set_global_environment();

        // work while we are cd into config_dir
        // to allow the plugins refer to their config
        // files using relative filenames
        if(chdir(netdata_configured_config_dir) == -1)
            fatal("Cannot cd to '%s'", netdata_configured_config_dir);
    }

    char *user = NULL;

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


        // --------------------------------------------------------------------
        // load stream.conf
        {
            char filename[FILENAME_MAX + 1];
            snprintfz(filename, FILENAME_MAX, "%s/stream.conf", netdata_configured_config_dir);
            appconfig_load(&stream_config, filename, 0);
        }


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
        // get the user we should run

        // IMPORTANT: this is required before web_files_uid()
        if(getuid() == 0) {
            user = config_get(CONFIG_SECTION_GLOBAL, "run as user", NETDATA_USER);
        }
        else {
            struct passwd *passwd = getpwuid(getuid());
            user = config_get(CONFIG_SECTION_GLOBAL, "run as user", (passwd && passwd->pw_name)?passwd->pw_name:"");
        }

        // --------------------------------------------------------------------
        // create the listening sockets

        web_client_api_v1_init();
        web_server_threading_selection();

        if(web_server_mode != WEB_SERVER_MODE_NONE)
            api_listen_sockets_setup();
    }

    // initialize the log files
    open_all_log_files();

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


    // fork, switch user, create pid file, set process priority
    if(become_daemon(dont_fork, user) == -1)
        fatal("Cannot daemonize myself.");

    info("netdata started on pid %d.", getpid());

    // IMPORTANT: these have to run once, while single threaded
    // but after we have switched user
    web_files_uid();
    web_files_gid();

    netdata_threads_init_after_fork((size_t)config_get_number(CONFIG_SECTION_GLOBAL, "pthread stack size", (long)default_stacksize));

    // ------------------------------------------------------------------------
    // initialize rrd, registry, health, rrdpush, etc.

    rrd_init(netdata_configured_hostname);


    // ------------------------------------------------------------------------
    // enable log flood protection

    error_log_limit_reset();


    // ------------------------------------------------------------------------
    // spawn the threads

    web_server_config_options();

    for (i = 0; static_threads[i].name != NULL ; i++) {
        struct netdata_static_thread *st = &static_threads[i];

        if(st->enabled) {
            st->thread = mallocz(sizeof(netdata_thread_t));
            debug(D_SYSTEM, "Starting thread %s.", st->name);
            netdata_thread_create(st->thread, st->name, NETDATA_THREAD_OPTION_DEFAULT, st->start_routine, st);
        }
        else debug(D_SYSTEM, "Not starting thread %s.", st->name);
    }

    info("netdata initialization completed. Enjoy real-time performance monitoring!");


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
