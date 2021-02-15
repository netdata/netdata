// SPDX-License-Identifier: GPL-3.0-or-later

#include "common.h"

struct collector {
    char *plugin;
    char *module;
};

struct array_printer {
    int c;
    BUFFER *plugin;
    BUFFER *module;
};

extern int aclk_connected;
extern ACLK_POPCORNING_STATE aclk_host_popcorn_check(RRDHOST *host);

int collector_counter(void *entry, void *data) {

    struct array_printer *ap = (struct array_printer *)data;
    struct collector *col=(struct collector *) entry;

    BUFFER *pl = ap->plugin;
    BUFFER *md = ap->module;

    if (likely(ap->c)) {
        buffer_strcat(pl, "|");
        buffer_strcat(md, "|");
    }
    buffer_strcat(pl, col->plugin);
    buffer_strcat(md, col->module);

    (ap->c)++;
   
    return 0;
}

void *analytics_main(void *ptr) {
    RRDSET *st;
    DICTIONARY *dict = dictionary_create(DICTIONARY_FLAG_SINGLE_THREADED);
    char name[500];
    struct netdata_static_thread *static_thread = (struct netdata_static_thread *)ptr;

    BUFFER *pl = buffer_create(1000);
    BUFFER *md = buffer_create(1000);
    
    debug(D_ANALYTICS, "Analytics thread starts");

    //sleep(10); /* TODO: decide how long to wait... */
    /* Could this from aclk work? What if it is disabled? */
    while (!netdata_exit) {
        if(aclk_host_popcorn_check(localhost) == ACLK_HOST_STABLE) {
            break;
        }
        sleep_usec(USEC_PER_SEC * 1);
    }

    debug(D_ANALYTICS, "Seems stable?");

    setenv("NETDATA_CONFIG_IS_PARENT"             , (localhost->next || configured_as_parent()) ? "true" : "false",        1);

    rrdhost_rdlock(localhost);
    rrdset_foreach_read(st, localhost) {
        if (rrdset_is_available_for_viewers(st)) {
            struct collector col = {
                    .plugin = st->plugin_name ? st->plugin_name : "",
                    .module = st->module_name ? st->module_name : ""
            };
            snprintfz(name, 499, "%s:%s", col.plugin, col.module);
            dictionary_set(dict, name, &col, sizeof(struct collector));
        }
    }
    rrdhost_unlock(localhost);

    struct array_printer ap;
    ap.c = 0;
    ap.plugin = pl;
    ap.module = md;
    
    dictionary_get_all(dict, collector_counter, &ap);
    dictionary_destroy(dict);

    setenv("NETDATA_COLLECTOR_PLUGINS", buffer_tostring(ap.plugin), 1);
    setenv("NETDATA_COLLECTOR_MODULES", buffer_tostring(ap.module), 1);

    {
        char b[7];
        snprintfz(b, 6, "%d", ap.c);
        setenv("NETDATA_COLLECTOR_COUNT"  , b, 1);
    }

    /* free buffers? */

#ifdef ENABLE_ACLK
    setenv("NETDATA_CONFIG_CLOUD_ENABLED"    , "true",  1);
#else
    setenv("NETDATA_CONFIG_CLOUD_ENABLED"    , "false",  1);
#endif
    if (is_agent_claimed())
        setenv("NETDATA_CONFIG_CLAIMED"    , "true",  1);
    else {
        setenv("NETDATA_CONFIG_CLAIMED"    , "false",  1);
    }
#ifdef ENABLE_ACLK
    if (aclk_connected)
        setenv("NETDATA_ACLK_AVAILABLE"    , "true",  1);
    else
#endif
        setenv("NETDATA_ACLK_AVAILABLE"    , "false",  1);

    if (netdata_anonymous_statistics_enabled > 0)
        send_statistics("META", "-", "-");

    /* TODO: either do not exit, or try from main not to try and stop it */
    static_thread->enabled = NETDATA_MAIN_THREAD_EXITED;
    return NULL;
}

void set_late_global_environment() {

    setenv("NETDATA_CONFIG_STREAM_ENABLED"        , default_rrdpush_enabled ? "true" : "false",        1);
    setenv("NETDATA_CONFIG_MEMORY_MODE"           , rrd_memory_mode_name(default_rrd_memory_mode), 1);

#ifdef ENABLE_DBENGINE
    {
        char b[16];
        snprintfz(b, 15, "%d", default_rrdeng_page_cache_mb);
        setenv("NETDATA_CONFIG_PAGE_CACHE_SIZE"        , b,        1);

        snprintfz(b, 15, "%d", default_multidb_disk_quota_mb);
        setenv("NETDATA_CONFIG_MULTIDB_DISK_QUOTA"     , b,        1);
    }
#else
    setenv("NETDATA_CONFIG_PAGE_CACHE_SIZE"       , "N/A", 1);
    setenv("NETDATA_CONFIG_MULTIDB_DISK_QUOTA"    , "N/A", 1);
#endif

#ifdef ENABLE_HTTPS
    setenv("NETDATA_CONFIG_HTTPS_ENABLED"    , "true",  1);
#else
    setenv("NETDATA_CONFIG_HTTPS_ENABLED"    , "false", 1);
#endif

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

#ifdef DISABLE_CLOUD
    setenv("NETDATA_CONFIG_CLOUD_ENABLED"    , "false", 1);
#else
    setenv("NETDATA_CONFIG_CLOUD_ENABLED"    , appconfig_get_boolean(&cloud_config, CONFIG_SECTION_GLOBAL, "enabled", 1) ? "true" : "false", 1);
#endif

    /* Maybe add default values for the late_global_enviroment here */
    /* In case of an exit with error */
    setenv("NETDATA_CONFIG_STREAM_ENABLED"        , "N/A", 1);
    setenv("NETDATA_CONFIG_IS_PARENT"             , "N/A", 1);
    setenv("NETDATA_CONFIG_MEMORY_MODE"           , "N/A", 1);
    setenv("NETDATA_CONFIG_PAGE_CACHE_SIZE"       , "N/A", 1);
    setenv("NETDATA_COLLECTOR_PLUGINS"            , "N/A", 1);
    setenv("NETDATA_COLLECTOR_MODULES"            , "N/A", 1);
    setenv("NETDATA_COLLECTOR_COUNT"              , "N/A", 1);
    setenv("NETDATA_CONFIG_CLOUD_ENABLED"         , "N/A", 1);
    setenv("NETDATA_CONFIG_CLAIMED"               , "N/A", 1);
    setenv("NETDATA_ACLK_AVAILABLE"               , "N/A", 1);


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
    if(!netdata_anonymous_statistics_enabled) return;
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
