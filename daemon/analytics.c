#include "common.h"

struct analytics_data analytics_data;

void analytics_setenv_data (void) {
    setenv( "NETDATA_CONFIG_STREAM_ENABLED", analytics_data.NETDATA_CONFIG_STREAM_ENABLED, 1);
    setenv( "NETDATA_CONFIG_MEMORY_MODE",    analytics_data.NETDATA_CONFIG_MEMORY_MODE,    1);
}

void analytics_log_data (void) {
    debug(D_ANALYTICS, "NETDATA_CONFIG_STREAM_ENABLED      : [%s]", analytics_data.NETDATA_CONFIG_STREAM_ENABLED);
    debug(D_ANALYTICS, "NETDATA_CONFIG_MEMORY_MODE         : [%s]", analytics_data.NETDATA_CONFIG_MEMORY_MODE);
}

void analytics_free_data (void) {
    freez(analytics_data.NETDATA_CONFIG_STREAM_ENABLED);
    freez(analytics_data.NETDATA_CONFIG_MEMORY_MODE);
}

void analytics_set_data (char **name, char *value) {
    if (*name) freez(*name);
    *name = strdupz(value);
}

void analytics_set_data_str (char **name, char *value) {
    size_t value_string_len;
    if (*name) freez(*name);
    value_string_len = strlen(value) + 3;
    *name = mallocz(sizeof(char) * value_string_len);
    snprintfz(*name, value_string_len, "\"%s\"", value);
}

void analytics_get_data (char *name, BUFFER *wb) {
    buffer_strcat(wb, name);
}

void analytics_main_cleanup(void *ptr) {
    struct netdata_static_thread *static_thread = (struct netdata_static_thread *)ptr;
    static_thread->enabled = NETDATA_MAIN_THREAD_EXITING;

    debug(D_ANALYTICS, "Cleaning up...");

    static_thread->enabled = NETDATA_MAIN_THREAD_EXITED;
}

void *analytics_main(void *ptr) {
    netdata_thread_cleanup_push(analytics_main_cleanup, ptr);
    int sec = 0;
    heartbeat_t hb;
    heartbeat_init(&hb);
    usec_t step_ut = USEC_PER_SEC;

    debug(D_ANALYTICS, "Analytics thread starts");

    while(!netdata_exit) {
        heartbeat_next(&hb, step_ut);
        sec++;

        if (unlikely(sec >= ANALYTICS_MAX_SLEEP_SEC))
            break;

        if (unlikely(netdata_exit))
            goto cleanup;
    }

    if (unlikely(netdata_exit))
        goto cleanup;

    send_statistics("META", "-", "-");

    analytics_log_data();

 cleanup:
    netdata_thread_cleanup_pop(1);
    return NULL;
}

static const char *verify_required_directory(const char *dir) {
    if(chdir(dir) == -1)
        fatal("Cannot change directory to '%s'", dir);

    DIR *d = opendir(dir);
    if(!d)
        fatal("Cannot examine the contents of directory '%s'", dir);
    closedir(d);

    return dir;
}

/* This is called after the rrdinit */
/* These values will be sent on the START event */
void set_late_global_environment() {

    analytics_set_data (&analytics_data.NETDATA_CONFIG_STREAM_ENABLED, default_rrdpush_enabled ? "true" : "false");
    analytics_set_data_str (&analytics_data.NETDATA_CONFIG_MEMORY_MODE, (char *)rrd_memory_mode_name(default_rrd_memory_mode));

    /* set what we have, to send the START event */
    analytics_setenv_data();
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

    setenv("NETDATA_VERSION"          , program_version, 1);
    setenv("NETDATA_HOSTNAME"         , netdata_configured_hostname, 1);
    setenv("NETDATA_CONFIG_DIR"       , verify_required_directory(netdata_configured_user_config_dir),  1);
    setenv("NETDATA_USER_CONFIG_DIR"  , verify_required_directory(netdata_configured_user_config_dir),  1);
    setenv("NETDATA_STOCK_CONFIG_DIR" , verify_required_directory(netdata_configured_stock_config_dir), 1);
    setenv("NETDATA_PLUGINS_DIR"      , verify_required_directory(netdata_configured_primary_plugins_dir),      1);
    setenv("NETDATA_WEB_DIR"          , verify_required_directory(netdata_configured_web_dir),          1);
    setenv("NETDATA_CACHE_DIR"        , verify_required_directory(netdata_configured_cache_dir),        1);
    setenv("NETDATA_LIB_DIR"          , verify_required_directory(netdata_configured_varlib_dir),       1);
    setenv("NETDATA_LOCK_DIR"         , netdata_configured_lock_dir, 1);
    setenv("NETDATA_LOG_DIR"          , verify_required_directory(netdata_configured_log_dir),          1);
    setenv("HOME"                     , verify_required_directory(netdata_configured_home_dir),         1);
    setenv("NETDATA_HOST_PREFIX"      , netdata_configured_host_prefix, 1);

    analytics_set_data (&analytics_data.NETDATA_CONFIG_STREAM_ENABLED, "null");
    analytics_set_data (&analytics_data.NETDATA_CONFIG_MEMORY_MODE,    "null");

    char *default_port = appconfig_get(&netdata_config, CONFIG_SECTION_WEB, "default port", NULL);
    int clean = 0;
    if (!default_port) {
        default_port = strdupz("19999");
        clean = 1;
    }

    setenv("NETDATA_LISTEN_PORT"      , default_port, 1);
    if(clean)
        freez(default_port);

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

void send_statistics( const char *action, const char *action_result, const char *action_data) {
    static char *as_script;

    if (netdata_anonymous_statistics_enabled == -1) {
        char *optout_file = mallocz(sizeof(char) * (strlen(netdata_configured_user_config_dir) +strlen(".opt-out-from-anonymous-statistics") + 2));
        sprintf(optout_file, "%s/%s", netdata_configured_user_config_dir, ".opt-out-from-anonymous-statistics");
        if (likely(access(optout_file, R_OK) != 0)) {
            as_script = mallocz(sizeof(char) * (strlen(netdata_configured_primary_plugins_dir) + strlen("anonymous-statistics.sh") + 2));
            sprintf(as_script, "%s/%s", netdata_configured_primary_plugins_dir, "anonymous-statistics.sh");
            if (unlikely(access(as_script, R_OK) != 0)) {
               netdata_anonymous_statistics_enabled=0;
               info("Anonymous statistics script %s not found.",as_script);
               freez(as_script);
            } else {
               netdata_anonymous_statistics_enabled=1;
            }
        } else {
            netdata_anonymous_statistics_enabled = 0;
            as_script = NULL;
        }
        freez(optout_file);
    }
    if (!netdata_anonymous_statistics_enabled) return;
    if (!action) return;
    if (!action_result) action_result="";
    if (!action_data) action_data="";
    char *command_to_run=mallocz(sizeof(char) * (strlen(action) + strlen(action_result) + strlen(action_data) + strlen(as_script) + 10));
    pid_t command_pid;

    sprintf(command_to_run,"%s '%s' '%s' '%s'", as_script, action, action_result, action_data);
    info("%s", command_to_run);

    FILE *fp = mypopen(command_to_run, &command_pid);
    if(fp) {
        char buffer[100 + 1];
        while (fgets(buffer, 100, fp) != NULL);
        mypclose(fp, command_pid);
    }
    freez(command_to_run);
}
