// SPDX-License-Identifier: GPL-3.0-or-later

/* TODO:

   - 1. Netdata Data Collectors in use -> OK
   - 2. Whether streaming is enabled, i.e. whether the daemon receives collected data from another daemon, or sends collected data to another daemon. -> OK
   - 3. Whether the collected agent data are archived in a separate application and the type of that application (e.g. Prometheus, InfluxDB)
        How? Mark when the allmetrics is called?
        TODO: Distinguish between allmetrics and backend exporters
   - 4. The data storage method being used (dbengine, ram, save) -> OK
   - 5. The data retention period -> Implement the calculator in: https://learn.netdata.cloud/docs/store/change-metrics-storage#calculate-the-system-resources-ram-disk-space-needed-to-store-metrics
   - 6. When the agent HTTP user interface was last accessed.
   - 7. Whether SSL is being used to encrypt the HTTP user interface
   - 8. Which alarm notification methods are being used.
        Maybe: parse health_alarm_notify.conf and get specific items if have options
               Run it? and get the env variables? Or pass them directly to the script?

   - 9. Dashboard enabled ([web] mode = none)
   - 10. Default port changed ([web] default port = 19999) -> OK: NETDATA_LISTEN_PORT

   General: Put everything in a struct, keep it there for maybe changes check or to provide them to api/v1
   Do not start the thread is analytics is disabled by the user
   Check if some data can be sent as integers etc...
*/

#include "common.h"

struct collector {
    char *plugin;
    char *module;
};

struct array_printer {
    int c;
    BUFFER *both;
};

extern int aclk_connected;
extern ACLK_POPCORNING_STATE aclk_host_popcorn_check(RRDHOST *host);
extern void analytics_build_info(BUFFER *b);

void analytics_log_data (void) {

    debug(D_ANALYTICS, "NETDATA_BUILDINFO                  : [%s]", analytics_data.NETDATA_BUILDINFO);
    debug(D_ANALYTICS, "NETDATA_CONFIG_STREAM_ENABLED      : [%s]", analytics_data.NETDATA_CONFIG_STREAM_ENABLED);
    debug(D_ANALYTICS, "NETDATA_CONFIG_IS_PARENT           : [%s]", analytics_data.NETDATA_CONFIG_IS_PARENT);
    debug(D_ANALYTICS, "NETDATA_CONFIG_MEMORY_MODE         : [%s]", analytics_data.NETDATA_CONFIG_MEMORY_MODE);
    debug(D_ANALYTICS, "NETDATA_CONFIG_PAGE_CACHE_SIZE     : [%s]", analytics_data.NETDATA_CONFIG_PAGE_CACHE_SIZE);
    debug(D_ANALYTICS, "NETDATA_CONFIG_MULTIDB_DISK_QUOTA  : [%s]", analytics_data.NETDATA_CONFIG_MULTIDB_DISK_QUOTA);
    debug(D_ANALYTICS, "NETDATA_CONFIG_HOSTS_AVAILABLE     : [%s]", analytics_data.NETDATA_CONFIG_HOSTS_AVAILABLE);
    debug(D_ANALYTICS, "NETDATA_CONFIG_ACLK_ENABLED        : [%s]", analytics_data.NETDATA_CONFIG_ACLK_ENABLED);
    debug(D_ANALYTICS, "NETDATA_CONFIG_WEB_ENABLED         : [%s]", analytics_data.NETDATA_CONFIG_WEB_ENABLED);
    debug(D_ANALYTICS, "NETDATA_CONFIG_EXPORTING_ENABLED   : [%s]", analytics_data.NETDATA_CONFIG_EXPORTING_ENABLED);
    debug(D_ANALYTICS, "NETDATA_CONFIG_RELEASE_CHANNEL     : [%s]", analytics_data.NETDATA_CONFIG_RELEASE_CHANNEL);
    debug(D_ANALYTICS, "NETDATA_HOST_ACLK_CONNECTED        : [%s]", analytics_data.NETDATA_HOST_ACLK_CONNECTED);
    debug(D_ANALYTICS, "NETDATA_ALLMETRICS_PROMETHEUS_USED : [%s]", analytics_data.NETDATA_ALLMETRICS_PROMETHEUS_USED);
    debug(D_ANALYTICS, "NETDATA_ALLMETRICS_SHELL_USED      : [%s]", analytics_data.NETDATA_ALLMETRICS_SHELL_USED);
    debug(D_ANALYTICS, "NETDATA_ALLMETRICS_JSON_USED       : [%s]", analytics_data.NETDATA_ALLMETRICS_JSON_USED);
    debug(D_ANALYTICS, "NETDATA_CONFIG_HTTPS_ENABLED       : [%s]", analytics_data.NETDATA_CONFIG_HTTPS_ENABLED);
    debug(D_ANALYTICS, "NETDATA_HOST_CLAIMED               : [%s]", analytics_data.NETDATA_HOST_CLAIMED);
    debug(D_ANALYTICS, "NETDATA_COLLECTORS                 : [%s]", analytics_data.NETDATA_COLLECTORS);
    debug(D_ANALYTICS, "NETDATA_COLLECTORS_COUNT           : [%s]", analytics_data.NETDATA_COLLECTORS_COUNT);
    debug(D_ANALYTICS, "NETDATA_ALARMS_NORMAL              : [%s]", analytics_data.NETDATA_ALARMS_NORMAL);
    debug(D_ANALYTICS, "NETDATA_ALARMS_WARNING             : [%s]", analytics_data.NETDATA_ALARMS_WARNING);
    debug(D_ANALYTICS, "NETDATA_ALARMS_CRITICAL            : [%s]", analytics_data.NETDATA_ALARMS_CRITICAL);
    debug(D_ANALYTICS, "NETDATA_CHARTS_COUNT               : [%s]", analytics_data.NETDATA_CHARTS_COUNT);
    debug(D_ANALYTICS, "NETDATA_METRICS_COUNT              : [%s]", analytics_data.NETDATA_METRICS_COUNT);
    debug(D_ANALYTICS, "NETDATA_NOTIFICATION_METHODS       : [%s]", analytics_data.NETDATA_NOTIFICATION_METHODS);
}

void analytics_setenv_data (void) {

    setenv ( "NETDATA_BUILDINFO",                 analytics_data.NETDATA_BUILDINFO, 1);
    setenv ( "NETDATA_CONFIG_STREAM_ENABLED",     analytics_data.NETDATA_CONFIG_STREAM_ENABLED, 1);
    setenv ( "NETDATA_CONFIG_IS_PARENT",          analytics_data.NETDATA_CONFIG_IS_PARENT, 1);
    setenv ( "NETDATA_CONFIG_MEMORY_MODE",        analytics_data.NETDATA_CONFIG_MEMORY_MODE, 1);
    setenv ( "NETDATA_CONFIG_PAGE_CACHE_SIZE",    analytics_data.NETDATA_CONFIG_PAGE_CACHE_SIZE, 1);
    setenv ( "NETDATA_CONFIG_MULTIDB_DISK_QUOTA", analytics_data.NETDATA_CONFIG_MULTIDB_DISK_QUOTA, 1);
    setenv ( "NETDATA_CONFIG_HOSTS_AVAILABLE",    analytics_data.NETDATA_CONFIG_HOSTS_AVAILABLE, 1);
    setenv ( "NETDATA_CONFIG_ACLK_ENABLED",       analytics_data.NETDATA_CONFIG_ACLK_ENABLED, 1);
    setenv ( "NETDATA_CONFIG_WEB_ENABLED",        analytics_data.NETDATA_CONFIG_WEB_ENABLED, 1);
    setenv ( "NETDATA_CONFIG_EXPORTING_ENABLED",  analytics_data.NETDATA_CONFIG_EXPORTING_ENABLED, 1);
    setenv ( "NETDATA_CONFIG_RELEASE_CHANNEL",    analytics_data.NETDATA_CONFIG_RELEASE_CHANNEL, 1);
    setenv ( "NETDATA_HOST_ACLK_CONNECTED",       analytics_data.NETDATA_HOST_ACLK_CONNECTED, 1);
    setenv ( "NETDATA_ALLMETRICS_PROMETHEUS_USED",analytics_data.NETDATA_ALLMETRICS_PROMETHEUS_USED, 1);
    setenv ( "NETDATA_ALLMETRICS_SHELL_USED",     analytics_data.NETDATA_ALLMETRICS_SHELL_USED, 1);
    setenv ( "NETDATA_ALLMETRICS_JSON_USED",      analytics_data.NETDATA_ALLMETRICS_JSON_USED, 1);
    setenv ( "NETDATA_CONFIG_HTTPS_ENABLED",      analytics_data.NETDATA_CONFIG_HTTPS_ENABLED, 1);
    setenv ( "NETDATA_HOST_CLAIMED",              analytics_data.NETDATA_HOST_CLAIMED, 1);
    setenv ( "NETDATA_COLLECTORS",                analytics_data.NETDATA_COLLECTORS, 1);
    setenv ( "NETDATA_COLLECTORS_COUNT",          analytics_data.NETDATA_COLLECTORS_COUNT, 1);
    setenv ( "NETDATA_ALARMS_NORMAL",             analytics_data.NETDATA_ALARMS_NORMAL, 1);
    setenv ( "NETDATA_ALARMS_WARNING",            analytics_data.NETDATA_ALARMS_WARNING, 1);
    setenv ( "NETDATA_ALARMS_CRITICAL",           analytics_data.NETDATA_ALARMS_CRITICAL, 1);
    setenv ( "NETDATA_CHARTS_COUNT",              analytics_data.NETDATA_CHARTS_COUNT, 1);
    setenv ( "NETDATA_METRICS_COUNT",             analytics_data.NETDATA_METRICS_COUNT, 1);
    setenv ( "NETDATA_NOTIFICATION_METHODS",      analytics_data.NETDATA_NOTIFICATION_METHODS, 1);
}

void analytics_free_data (void) {
    freez(analytics_data.NETDATA_BUILDINFO);
    freez(analytics_data.NETDATA_CONFIG_STREAM_ENABLED);
    freez(analytics_data.NETDATA_CONFIG_IS_PARENT);
    freez(analytics_data.NETDATA_CONFIG_MEMORY_MODE);
    freez(analytics_data.NETDATA_CONFIG_PAGE_CACHE_SIZE);
    freez(analytics_data.NETDATA_CONFIG_MULTIDB_DISK_QUOTA);
    freez(analytics_data.NETDATA_CONFIG_HOSTS_AVAILABLE);
    freez(analytics_data.NETDATA_CONFIG_ACLK_ENABLED);
    freez(analytics_data.NETDATA_CONFIG_WEB_ENABLED);
    freez(analytics_data.NETDATA_CONFIG_EXPORTING_ENABLED);
    freez(analytics_data.NETDATA_CONFIG_RELEASE_CHANNEL);
    freez(analytics_data.NETDATA_HOST_ACLK_CONNECTED);
    freez(analytics_data.NETDATA_ALLMETRICS_PROMETHEUS_USED);
    freez(analytics_data.NETDATA_ALLMETRICS_SHELL_USED);
    freez(analytics_data.NETDATA_ALLMETRICS_JSON_USED);
    freez(analytics_data.NETDATA_CONFIG_HTTPS_ENABLED);
    freez(analytics_data.NETDATA_HOST_CLAIMED);
    freez(analytics_data.NETDATA_COLLECTORS);
    freez(analytics_data.NETDATA_COLLECTORS_COUNT);
    freez(analytics_data.NETDATA_ALARMS_NORMAL);
    freez(analytics_data.NETDATA_ALARMS_WARNING);
    freez(analytics_data.NETDATA_ALARMS_CRITICAL);
    freez(analytics_data.NETDATA_CHARTS_COUNT);
    freez(analytics_data.NETDATA_METRICS_COUNT);
    freez(analytics_data.NETDATA_NOTIFICATION_METHODS);
}


void analytics_set_data (char **name, char *value) {

    if (*name) freez(*name);
    *name = strdupz(value);

}

void analytics_get_data (char *name, BUFFER *wb) {

    buffer_strcat(wb, name);

}

int collector_counter_callb(void *entry, void *data) {

    struct array_printer *ap = (struct array_printer *)data;
    struct collector *col=(struct collector *) entry;

    BUFFER *bt = ap->both;

    if (likely(ap->c)) {
        buffer_strcat(bt, ",");
    }

    buffer_strcat(bt, "{");
    buffer_strcat(bt, " \"plugin\": \"");
    buffer_strcat(bt, col->plugin);
    buffer_strcat(bt, "\", \"module\":\"");
    buffer_strcat(bt, col->module);
    buffer_strcat(bt, "\" }");

    (ap->c)++;

    return 0;
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

void analytics_alarms (void) {
    int alarm_warn = 0, alarm_crit = 0, alarm_normal = 0;
    char b[10];
    RRDCALC *rc;

    for(rc = localhost->alarms; rc ; rc = rc->next) {
        if(unlikely(!rc->rrdset || !rc->rrdset->last_collected_time.tv_sec))
            continue;

        switch(rc->status) {
        case RRDCALC_STATUS_WARNING:
            alarm_warn++;
            break;
        case RRDCALC_STATUS_CRITICAL:
            alarm_crit++;
            break;
        default:
            alarm_normal++;
        }

    }

    snprintfz(b, 9, "%d", alarm_normal);
    analytics_set_data (&analytics_data.NETDATA_ALARMS_NORMAL, b);
    snprintfz(b, 9, "%d", alarm_warn);
    analytics_set_data (&analytics_data.NETDATA_ALARMS_WARNING, b);
    snprintfz(b, 9, "%d", alarm_crit);
    analytics_set_data (&analytics_data.NETDATA_ALARMS_CRITICAL, b);
}

/* Consider running the already existing function for this */
void analytics_collectors(void) {
    RRDSET *st;
    DICTIONARY *dict = dictionary_create(DICTIONARY_FLAG_SINGLE_THREADED);
    char name[500];
    BUFFER *bt = buffer_create(1000);

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

    struct array_printer ap;
    ap.c = 0;
    ap.both = bt;

    dictionary_get_all(dict, collector_counter_callb, &ap);
    dictionary_destroy(dict);

    analytics_set_data (&analytics_data.NETDATA_COLLECTORS, (char *)buffer_tostring(ap.both));

    {
        char b[7];
        snprintfz(b, 6, "%d", ap.c);
        analytics_set_data (&analytics_data.NETDATA_COLLECTORS_COUNT, b);
    }

}

void analytics_log_prometheus(void) {

    if (likely(analytics_data.prometheus_hits < ANALYTICS_MAX_PROMETHEUS_HITS))
        analytics_data.prometheus_hits++;

}

void analytics_log_shell(void) {

    if (likely(analytics_data.shell_hits < ANALYTICS_MAX_SHELL_HITS))
        analytics_data.shell_hits++;

}

void analytics_log_json(void) {

    if (likely(analytics_data.json_hits < ANALYTICS_MAX_JSON_HITS))
        analytics_data.json_hits++;

}



void analytics_misc(void) {

    analytics_set_data (&analytics_data.NETDATA_CONFIG_IS_PARENT, (localhost->next || configured_as_parent()) ? "true" : "false");

    {
        char b[7];
        snprintfz(b, 6, "%ld", rrd_hosts_available);
        analytics_set_data (&analytics_data.NETDATA_CONFIG_HOSTS_AVAILABLE, b);
    }

#ifdef ENABLE_ACLK
    analytics_set_data (&analytics_data.NETDATA_CONFIG_ACLK_ENABLED, "true");
#else
    analytics_set_data (&analytics_data.NETDATA_CONFIG_ACLK_ENABLED, "false");
#endif
    if (is_agent_claimed())
        analytics_set_data (&analytics_data.NETDATA_HOST_CLAIMED, "true");
    else {
        analytics_set_data (&analytics_data.NETDATA_HOST_CLAIMED, "false");
    }
#ifdef ENABLE_ACLK
    if (aclk_connected)
        analytics_set_data (&analytics_data.NETDATA_HOST_ACLK_CONNECTED, "true");
    else
#endif
        analytics_set_data (&analytics_data.NETDATA_HOST_ACLK_CONNECTED, "false");

    //dont do it like this.... it should be already loaded somewhere...
    analytics_set_data(&analytics_data.NETDATA_CONFIG_EXPORTING_ENABLED, appconfig_get_boolean(&exporting_config, CONFIG_SECTION_EXPORTING, "enabled", 1) ? "true" : "false");

}

void analytics_charts (void) {
    RRDSET *st;
    int c = 0;
    int show_archived = 0;
    rrdset_foreach_read(st, localhost) {
        if ((!show_archived && rrdset_is_available_for_viewers(st)) || (show_archived && rrdset_is_archived(st))) {
            c++;
        }
    }
    {
        char b[7];
        snprintfz(b, 6, "%d", c);
        analytics_set_data (&analytics_data.NETDATA_CHARTS_COUNT, b);
    }

}

void analytics_metrics (void) {
    RRDSET *st;
    long int dimensions = 0;
    RRDDIM *rd;
    rrdset_foreach_read(st, localhost) {
        rrddim_foreach_read(rd, st) {
            if(rrddim_flag_check(rd, RRDDIM_FLAG_HIDDEN) || rrddim_flag_check(rd, RRDDIM_FLAG_OBSOLETE)) continue;
            dimensions++;
        }
    }
    {
        char b[7];
        snprintfz(b, 6, "%ld", dimensions);
        analytics_set_data (&analytics_data.NETDATA_METRICS_COUNT, b);
    }
}

void analytics_alarms_notifications (void) {
    char *script;
    script = mallocz(sizeof(char) * (strlen(netdata_configured_primary_plugins_dir) + strlen("alarm-notify.sh dump_methods") + 2));
    sprintf(script, "%s/%s", netdata_configured_primary_plugins_dir, "alarm-notify.sh");
    if (unlikely(access(script, R_OK) != 0)) {
        info("Alarm notify script %s not found.",script);
        freez(script);
        return;
    }

    strcat(script, " dump_methods");

    pid_t command_pid;

    info("Executing %s", script);
    debug(D_ANALYTICS, "Executing %s", script);

    BUFFER *b = buffer_create(1000);
    int cnt = 0;
    FILE *fp = mypopen(script, &command_pid);
    if(fp) {
        char line[200 + 1];

        while (fgets(line, 200, fp) != NULL) {
            char *end = line;
            while (*end && *end != '\n') end++;
            *end = '\0';

            if (likely(cnt))
                buffer_strcat(b, "|");

            buffer_strcat(b, line);

            cnt++;
        }
        mypclose(fp, command_pid);
    }
    freez(script);

    //check there is something to set
    analytics_set_data (&analytics_data.NETDATA_NOTIFICATION_METHODS, (char *)buffer_tostring(b));

    //TODO Destroy buffer

    //return 0;
}

void analytics_gather_meta_data (void) {

    rrdhost_rdlock(localhost);

    analytics_collectors();
    analytics_alarms();
    analytics_charts();
    analytics_metrics();

    rrdhost_unlock(localhost);

    analytics_misc();
    analytics_alarms_notifications();

    {
        char b[7];
        snprintfz(b, 6, "%d", analytics_data.prometheus_hits);
        analytics_set_data (&analytics_data.NETDATA_ALLMETRICS_PROMETHEUS_USED, b);

        snprintfz(b, 6, "%d", analytics_data.shell_hits);
        analytics_set_data (&analytics_data.NETDATA_ALLMETRICS_SHELL_USED, b);

        snprintfz(b, 6, "%d", analytics_data.json_hits);
        analytics_set_data (&analytics_data.NETDATA_ALLMETRICS_JSON_USED, b);

    }

    analytics_setenv_data();
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

    debug(D_ANALYTICS, "Analytics thread starts");

    while(!netdata_exit) {
        sleep(1);
        ++sec;

        if (sec == ANALYTICS_MAX_SLEEP_SEC)
            break;

        if(unlikely(netdata_exit))
            goto cleanup;
    }

    if(unlikely(netdata_exit))
        goto cleanup;

    debug(D_ANALYTICS, "Stable...");

    analytics_gather_meta_data();
    analytics_log_data();

    send_statistics("META", "-", "-");

    //debug (D_ANALYTICS, "Haha [%s]", analytics_get_data(analytics_data.NETDATA_ALLMETRICS_JSON_USED));

 cleanup:
    netdata_thread_cleanup_pop(1);
    return NULL;
}

/* This is called after the rrdinit */
/* These values will be sent on the START event */
void set_late_global_environment() {

    analytics_set_data (&analytics_data.NETDATA_CONFIG_STREAM_ENABLED, default_rrdpush_enabled ? "true" : "false");
    analytics_set_data (&analytics_data.NETDATA_CONFIG_MEMORY_MODE, (char *)rrd_memory_mode_name(default_rrd_memory_mode));

#ifdef ENABLE_DBENGINE
    {
        char b[16];
        snprintfz(b, 15, "%d", default_rrdeng_page_cache_mb);
        analytics_set_data (&analytics_data.NETDATA_CONFIG_PAGE_CACHE_SIZE, b);

        snprintfz(b, 15, "%d", default_multidb_disk_quota_mb);
        analytics_set_data (&analytics_data.NETDATA_CONFIG_MULTIDB_DISK_QUOTA, b);
    }
#endif

#ifdef ENABLE_HTTPS
    analytics_set_data (&analytics_data.NETDATA_CONFIG_HTTPS_ENABLED, "true");
#else
    analytics_set_data (&analytics_data.NETDATA_CONFIG_HTTPS_ENABLED, "false");
#endif

    if(web_server_mode == WEB_SERVER_MODE_NONE)
        analytics_set_data (&analytics_data.NETDATA_CONFIG_WEB_ENABLED, "false");
    else
        analytics_set_data (&analytics_data.NETDATA_CONFIG_WEB_ENABLED, "true");

    //get release channel //web/api/formatters/charts2json.c
    analytics_set_data(&analytics_data.NETDATA_CONFIG_RELEASE_CHANNEL, (char *)get_release_channel());

    {
        BUFFER *bi = buffer_create(1000);
        analytics_build_info(bi);
        analytics_set_data (&analytics_data.NETDATA_BUILDINFO, (char *)buffer_tostring(bi));
        buffer_free(bi);
    }

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

#ifdef DISABLE_CLOUD
    setenv("NETDATA_CONFIG_CLOUD_ENABLED"    , "false", 1);
#else
    setenv("NETDATA_CONFIG_CLOUD_ENABLED"    , appconfig_get_boolean(&cloud_config, CONFIG_SECTION_GLOBAL, "enabled", 1) ? "true" : "false", 1);
#endif

    /* Initialize values we'll get from late global and the thread to N/A */
    analytics_set_data (&analytics_data.NETDATA_BUILDINFO,                 "N/A");
    analytics_set_data (&analytics_data.NETDATA_CONFIG_STREAM_ENABLED,     "N/A");
    analytics_set_data (&analytics_data.NETDATA_CONFIG_IS_PARENT,          "N/A");
    analytics_set_data (&analytics_data.NETDATA_CONFIG_MEMORY_MODE,        "N/A");
    analytics_set_data (&analytics_data.NETDATA_CONFIG_PAGE_CACHE_SIZE,    "N/A");
    analytics_set_data (&analytics_data.NETDATA_CONFIG_MULTIDB_DISK_QUOTA, "N/A");
    analytics_set_data (&analytics_data.NETDATA_CONFIG_HOSTS_AVAILABLE,    "N/A");
    analytics_set_data (&analytics_data.NETDATA_CONFIG_ACLK_ENABLED,       "N/A");
    analytics_set_data (&analytics_data.NETDATA_CONFIG_WEB_ENABLED,        "N/A");
    analytics_set_data (&analytics_data.NETDATA_CONFIG_EXPORTING_ENABLED,  "N/A");
    analytics_set_data (&analytics_data.NETDATA_CONFIG_RELEASE_CHANNEL,    "N/A");
    analytics_set_data (&analytics_data.NETDATA_HOST_ACLK_CONNECTED,       "N/A");
    analytics_set_data (&analytics_data.NETDATA_HOST_CLAIMED,              "N/A");
    analytics_set_data (&analytics_data.NETDATA_ALLMETRICS_PROMETHEUS_USED,"N/A");
    analytics_set_data (&analytics_data.NETDATA_ALLMETRICS_SHELL_USED,     "N/A");
    analytics_set_data (&analytics_data.NETDATA_ALLMETRICS_JSON_USED,      "N/A");
    analytics_set_data (&analytics_data.NETDATA_CONFIG_HTTPS_ENABLED,      "N/A");
    analytics_set_data (&analytics_data.NETDATA_COLLECTORS,                "\"N/A\""); //must, because this is an array
    analytics_set_data (&analytics_data.NETDATA_COLLECTORS_COUNT,          "N/A");
    analytics_set_data (&analytics_data.NETDATA_ALARMS_NORMAL,             "N/A");
    analytics_set_data (&analytics_data.NETDATA_ALARMS_WARNING,            "N/A");
    analytics_set_data (&analytics_data.NETDATA_ALARMS_CRITICAL,           "N/A");
    analytics_set_data (&analytics_data.NETDATA_CHARTS_COUNT,              "N/A");
    analytics_set_data (&analytics_data.NETDATA_METRICS_COUNT,             "N/A");
    analytics_set_data (&analytics_data.NETDATA_NOTIFICATION_METHODS,      "N/A");
    analytics_data.prometheus_hits = 0;
    analytics_data.shell_hits = 0;
    analytics_data.json_hits = 0;

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
