// SPDX-License-Identifier: GPL-3.0-or-later

#include "common.h"
#include "buildinfo.h"

struct analytics_data analytics_data;
extern void analytics_exporting_connectors (BUFFER *b);
extern void analytics_exporting_connectors_ssl (BUFFER *b);
extern void analytics_build_info (BUFFER *b);
extern int aclk_connected;

struct collector {
    const char *plugin;
    const char *module;
};

struct array_printer {
    int c;
    BUFFER *both;
};

/*
 * Debug logging
 */
void analytics_log_data(void)
{
    debug(D_ANALYTICS, "NETDATA_CONFIG_STREAM_ENABLED      : [%s]", analytics_data.netdata_config_stream_enabled);
    debug(D_ANALYTICS, "NETDATA_CONFIG_MEMORY_MODE         : [%s]", analytics_data.netdata_config_memory_mode);
    debug(D_ANALYTICS, "NETDATA_CONFIG_EXPORTING_ENABLED   : [%s]", analytics_data.netdata_config_exporting_enabled);
    debug(D_ANALYTICS, "NETDATA_EXPORTING_CONNECTORS       : [%s]", analytics_data.netdata_exporting_connectors);
    debug(D_ANALYTICS, "NETDATA_ALLMETRICS_PROMETHEUS_USED : [%s]", analytics_data.netdata_allmetrics_prometheus_used);
    debug(D_ANALYTICS, "NETDATA_ALLMETRICS_SHELL_USED      : [%s]", analytics_data.netdata_allmetrics_shell_used);
    debug(D_ANALYTICS, "NETDATA_ALLMETRICS_JSON_USED       : [%s]", analytics_data.netdata_allmetrics_json_used);
    debug(D_ANALYTICS, "NETDATA_DASHBOARD_USED             : [%s]", analytics_data.netdata_dashboard_used);
    debug(D_ANALYTICS, "NETDATA_COLLECTORS                 : [%s]", analytics_data.netdata_collectors);
    debug(D_ANALYTICS, "NETDATA_COLLECTORS_COUNT           : [%s]", analytics_data.netdata_collectors_count);
    debug(D_ANALYTICS, "NETDATA_BUILDINFO                  : [%s]", analytics_data.netdata_buildinfo);
    debug(D_ANALYTICS, "NETDATA_CONFIG_PAGE_CACHE_SIZE     : [%s]", analytics_data.netdata_config_page_cache_size);
    debug(D_ANALYTICS, "NETDATA_CONFIG_MULTIDB_DISK_QUOTA  : [%s]", analytics_data.netdata_config_multidb_disk_quota);
    debug(D_ANALYTICS, "NETDATA_CONFIG_HTTPS_ENABLED       : [%s]", analytics_data.netdata_config_https_enabled);
    debug(D_ANALYTICS, "NETDATA_CONFIG_WEB_ENABLED         : [%s]", analytics_data.netdata_config_web_enabled);
    debug(D_ANALYTICS, "NETDATA_CONFIG_RELEASE_CHANNEL     : [%s]", analytics_data.netdata_config_release_channel);
    debug(D_ANALYTICS, "NETDATA_MIRRORED_HOST_COUNT        : [%s]", analytics_data.netdata_mirrored_host_count);
    debug(D_ANALYTICS, "NETDATA_MIRRORED_HOSTS_REACHABLE   : [%s]", analytics_data.netdata_mirrored_hosts_reachable);
    debug(D_ANALYTICS, "NETDATA_MIRRORED_HOSTS_UNREACHABLE : [%s]", analytics_data.netdata_mirrored_hosts_unreachable);
    debug(D_ANALYTICS, "NETDATA_NOTIFICATION_METHODS       : [%s]", analytics_data.netdata_notification_methods);
    debug(D_ANALYTICS, "NETDATA_ALARMS_NORMAL              : [%s]", analytics_data.netdata_alarms_normal);
    debug(D_ANALYTICS, "NETDATA_ALARMS_WARNING             : [%s]", analytics_data.netdata_alarms_warning);
    debug(D_ANALYTICS, "NETDATA_ALARMS_CRITICAL            : [%s]", analytics_data.netdata_alarms_critical);
    debug(D_ANALYTICS, "NETDATA_CHARTS_COUNT               : [%s]", analytics_data.netdata_charts_count);
    debug(D_ANALYTICS, "NETDATA_METRICS_COUNT              : [%s]", analytics_data.netdata_metrics_count);
    debug(D_ANALYTICS, "NETDATA_CONFIG_IS_PARENT           : [%s]", analytics_data.netdata_config_is_parent);
    debug(D_ANALYTICS, "NETDATA_CONFIG_HOSTS_AVAILABLE     : [%s]", analytics_data.netdata_config_hosts_available);
    debug(D_ANALYTICS, "NETDATA_HOST_CLOUD_AVAILABLE       : [%s]", analytics_data.netdata_host_cloud_available);
    debug(D_ANALYTICS, "NETDATA_HOST_ACLK_AVAILABLE        : [%s]", analytics_data.netdata_host_aclk_available);
    debug(D_ANALYTICS, "NETDATA_HOST_ACLK_PROTOCOL         : [%s]", analytics_data.netdata_host_aclk_protocol);
    debug(D_ANALYTICS, "NETDATA_HOST_ACLK_IMPLEMENTATION   : [%s]", analytics_data.netdata_host_aclk_implementation);
    debug(D_ANALYTICS, "NETDATA_HOST_AGENT_CLAIMED         : [%s]", analytics_data.netdata_host_agent_claimed);
    debug(D_ANALYTICS, "NETDATA_HOST_CLOUD_ENABLED         : [%s]", analytics_data.netdata_host_cloud_enabled);
    debug(D_ANALYTICS, "NETDATA_CONFIG_HTTPS_AVAILABLE     : [%s]", analytics_data.netdata_config_https_available);
    debug(D_ANALYTICS, "NETDATA_INSTALL_TYPE               : [%s]", analytics_data.netdata_install_type);
    debug(D_ANALYTICS, "NETDATA_PREBUILT_DISTRO            : [%s]", analytics_data.netdata_prebuilt_distro);
    debug(D_ANALYTICS, "NETDATA_CONFIG_IS_PRIVATE_REGISTRY : [%s]", analytics_data.netdata_config_is_private_registry);
    debug(D_ANALYTICS, "NETDATA_CONFIG_USE_PRIVATE_REGISTRY: [%s]", analytics_data.netdata_config_use_private_registry);
    debug(D_ANALYTICS, "NETDATA_CONFIG_OOM_SCORE           : [%s]", analytics_data.netdata_config_oom_score);
}

/*
 * Free data
 */
void analytics_free_data(void)
{
    freez(analytics_data.netdata_config_stream_enabled);
    freez(analytics_data.netdata_config_memory_mode);
    freez(analytics_data.netdata_config_exporting_enabled);
    freez(analytics_data.netdata_exporting_connectors);
    freez(analytics_data.netdata_allmetrics_prometheus_used);
    freez(analytics_data.netdata_allmetrics_shell_used);
    freez(analytics_data.netdata_allmetrics_json_used);
    freez(analytics_data.netdata_dashboard_used);
    freez(analytics_data.netdata_collectors);
    freez(analytics_data.netdata_collectors_count);
    freez(analytics_data.netdata_buildinfo);
    freez(analytics_data.netdata_config_page_cache_size);
    freez(analytics_data.netdata_config_multidb_disk_quota);
    freez(analytics_data.netdata_config_https_enabled);
    freez(analytics_data.netdata_config_web_enabled);
    freez(analytics_data.netdata_config_release_channel);
    freez(analytics_data.netdata_mirrored_host_count);
    freez(analytics_data.netdata_mirrored_hosts_reachable);
    freez(analytics_data.netdata_mirrored_hosts_unreachable);
    freez(analytics_data.netdata_notification_methods);
    freez(analytics_data.netdata_alarms_normal);
    freez(analytics_data.netdata_alarms_warning);
    freez(analytics_data.netdata_alarms_critical);
    freez(analytics_data.netdata_charts_count);
    freez(analytics_data.netdata_metrics_count);
    freez(analytics_data.netdata_config_is_parent);
    freez(analytics_data.netdata_config_hosts_available);
    freez(analytics_data.netdata_host_cloud_available);
    freez(analytics_data.netdata_host_aclk_available);
    freez(analytics_data.netdata_host_aclk_protocol);
    freez(analytics_data.netdata_host_aclk_implementation);
    freez(analytics_data.netdata_host_agent_claimed);
    freez(analytics_data.netdata_host_cloud_enabled);
    freez(analytics_data.netdata_config_https_available);
    freez(analytics_data.netdata_install_type);
    freez(analytics_data.netdata_config_is_private_registry);
    freez(analytics_data.netdata_config_use_private_registry);
    freez(analytics_data.netdata_config_oom_score);
    freez(analytics_data.netdata_prebuilt_distro);
}

/*
 * Set a numeric/boolean data with a value
 */
void analytics_set_data(char **name, char *value)
{
    if (*name) {
        analytics_data.data_length -= strlen(*name);
        freez(*name);
    }
    *name = strdupz(value);
    analytics_data.data_length += strlen(*name);
}

/*
 * Set a string data with a value
 */
void analytics_set_data_str(char **name, char *value)
{
    size_t value_string_len;
    if (*name) {
        analytics_data.data_length -= strlen(*name);
        freez(*name);
    }
    value_string_len = strlen(value) + 4;
    *name = mallocz(sizeof(char) * value_string_len);
    snprintfz(*name, value_string_len - 1, "\"%s\"", value);
    analytics_data.data_length += strlen(*name);
}

/*
 * Get data, used by web api v1
 */
void analytics_get_data(char *name, BUFFER *wb)
{
    buffer_strcat(wb, name);
}

/*
 * Log hits on the allmetrics page, with prometheus parameter
 */
void analytics_log_prometheus(void)
{
    if (netdata_anonymous_statistics_enabled == 1 && likely(analytics_data.prometheus_hits < ANALYTICS_MAX_PROMETHEUS_HITS)) {
        analytics_data.prometheus_hits++;
        char b[7];
        snprintfz(b, 6, "%d", analytics_data.prometheus_hits);
        analytics_set_data(&analytics_data.netdata_allmetrics_prometheus_used, b);
    }
}

/*
 * Log hits on the allmetrics page, with shell parameter (or default)
 */
void analytics_log_shell(void)
{
    if (netdata_anonymous_statistics_enabled == 1 && likely(analytics_data.shell_hits < ANALYTICS_MAX_SHELL_HITS)) {
        analytics_data.shell_hits++;
        char b[7];
        snprintfz(b, 6, "%d", analytics_data.shell_hits);
        analytics_set_data(&analytics_data.netdata_allmetrics_shell_used, b);
    }
}

/*
 * Log hits on the allmetrics page, with json parameter
 */
void analytics_log_json(void)
{
    if (netdata_anonymous_statistics_enabled == 1 && likely(analytics_data.json_hits < ANALYTICS_MAX_JSON_HITS)) {
        analytics_data.json_hits++;
        char b[7];
        snprintfz(b, 6, "%d", analytics_data.json_hits);
        analytics_set_data(&analytics_data.netdata_allmetrics_json_used, b);
    }
}

/*
 * Log hits on the dashboard, (when calling HELLO).
 */
void analytics_log_dashboard(void)
{
    if (netdata_anonymous_statistics_enabled == 1 && likely(analytics_data.dashboard_hits < ANALYTICS_MAX_DASHBOARD_HITS)) {
        analytics_data.dashboard_hits++;
        char b[7];
        snprintfz(b, 6, "%d", analytics_data.dashboard_hits);
        analytics_set_data(&analytics_data.netdata_dashboard_used, b);
    }
}

/*
 * Called when setting the oom score
 */
void analytics_report_oom_score(long long int score){
    char b[7];
    snprintfz(b, 6, "%d", (int)score);
    analytics_set_data(&analytics_data.netdata_config_oom_score, b);
}

void analytics_mirrored_hosts(void)
{
    RRDHOST *host;
    int count = 0;
    int reachable = 0;
    int unreachable = 0;
    char b[11];

    rrd_rdlock();
    rrdhost_foreach_read(host)
    {
        if (rrdhost_flag_check(host, RRDHOST_FLAG_ARCHIVED))
            continue;

        netdata_mutex_lock(&host->receiver_lock);
        ((host->receiver || host == localhost) ? reachable++ : unreachable++);
        netdata_mutex_unlock(&host->receiver_lock);

        count++;
    }
    rrd_unlock();

    snprintfz(b, 10, "%d", count);
    analytics_set_data(&analytics_data.netdata_mirrored_host_count, b);
    snprintfz(b, 10, "%d", reachable);
    analytics_set_data(&analytics_data.netdata_mirrored_hosts_reachable, b);
    snprintfz(b, 10, "%d", unreachable);
    analytics_set_data(&analytics_data.netdata_mirrored_hosts_unreachable, b);
}

void analytics_exporters(void)
{
    //when no exporters are available, an empty string will be sent
    //decide if something else is more suitable (but probably not null)
    BUFFER *bi = buffer_create(1000);
    analytics_exporting_connectors(bi);
    analytics_set_data_str(&analytics_data.netdata_exporting_connectors, (char *)buffer_tostring(bi));
    buffer_free(bi);
}

int collector_counter_callb(const char *name, void *entry, void *data) {
    (void)name;

    struct array_printer *ap = (struct array_printer *)data;
    struct collector *col = (struct collector *)entry;

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

/*
 * Create a JSON array of available collectors, same as in api/v1/info
 */
void analytics_collectors(void)
{
    RRDSET *st;
    DICTIONARY *dict = dictionary_create(DICTIONARY_FLAG_SINGLE_THREADED);
    char name[500];
    BUFFER *bt = buffer_create(1000);

    rrdset_foreach_read(st, localhost)
    {
        if (rrdset_is_available_for_viewers(st)) {
            struct collector col = {
                .plugin = rrdset_plugin_name(st),
                .module = rrdset_module_name(st)
            };
            snprintfz(name, 499, "%s:%s", col.plugin, col.module);
            dictionary_set(dict, name, &col, sizeof(struct collector));
        }
    }

    struct array_printer ap;
    ap.c = 0;
    ap.both = bt;

    dictionary_walkthrough_read(dict, collector_counter_callb, &ap);
    dictionary_destroy(dict);

    analytics_set_data(&analytics_data.netdata_collectors, (char *)buffer_tostring(ap.both));

    {
        char b[7];
        snprintfz(b, 6, "%d", ap.c);
        analytics_set_data(&analytics_data.netdata_collectors_count, b);
    }

    buffer_free(bt);
}

/*
 * Run alarm-notify.sh script using the dump_methods parameter
 * SEND_CUSTOM is always available
 */
void analytics_alarms_notifications(void)
{
    char *script;
    script = mallocz(
        sizeof(char) * (strlen(netdata_configured_primary_plugins_dir) + strlen("alarm-notify.sh dump_methods") + 2));
    sprintf(script, "%s/%s", netdata_configured_primary_plugins_dir, "alarm-notify.sh");
    if (unlikely(access(script, R_OK) != 0)) {
        info("Alarm notify script %s not found.", script);
        freez(script);
        return;
    }

    strcat(script, " dump_methods");

    pid_t command_pid;

    debug(D_ANALYTICS, "Executing %s", script);

    BUFFER *b = buffer_create(1000);
    int cnt = 0;
    FILE *fp = mypopen(script, &command_pid);
    if (fp) {
        char line[200 + 1];

        while (fgets(line, 200, fp) != NULL) {
            char *end = line;
            while (*end && *end != '\n')
                end++;
            *end = '\0';

            if (likely(cnt))
                buffer_strcat(b, "|");

            buffer_strcat(b, line);

            cnt++;
        }
        mypclose(fp, command_pid);
    }
    freez(script);

    analytics_set_data_str(&analytics_data.netdata_notification_methods, (char *)buffer_tostring(b));

    buffer_free(b);
}

void analytics_get_install_type(void)
{
    if (localhost->system_info->install_type == NULL) {
        analytics_set_data_str(&analytics_data.netdata_install_type, "unknown");
    } else {
        analytics_set_data_str(&analytics_data.netdata_install_type, localhost->system_info->install_type);
    }

    if (localhost->system_info->prebuilt_dist != NULL) {
        analytics_set_data_str(&analytics_data.netdata_prebuilt_distro, localhost->system_info->prebuilt_dist);
    }
}

/*
 * Pick up if https is actually used
 */
void analytics_https(void)
{
    BUFFER *b = buffer_create(30);
#ifdef ENABLE_HTTPS
    analytics_exporting_connectors_ssl(b);
    buffer_strcat(b, netdata_client_ctx && localhost->ssl.flags == NETDATA_SSL_HANDSHAKE_COMPLETE && __atomic_load_n(&localhost->rrdpush_sender_connected, __ATOMIC_SEQ_CST) ? "streaming|" : "|");
    buffer_strcat(b, netdata_srv_ctx ? "web" : "");
#else
    buffer_strcat(b, "||");
#endif

    analytics_set_data_str(&analytics_data.netdata_config_https_available, (char *)buffer_tostring(b));
    buffer_free(b);
}

void analytics_charts(void)
{
    RRDSET *st;
    int c = 0;
    rrdset_foreach_read(st, localhost)
    {
        if (rrdset_is_available_for_viewers(st)) {
            c++;
        }
    }
    {
        char b[7];
        snprintfz(b, 6, "%d", c);
        analytics_set_data(&analytics_data.netdata_charts_count, b);
    }
}

void analytics_metrics(void)
{
    RRDSET *st;
    long int dimensions = 0;
    RRDDIM *rd;
    rrdset_foreach_read(st, localhost)
    {
        rrdset_rdlock(st);

        if (rrdset_is_available_for_viewers(st)) {
            rrddim_foreach_read(rd, st)
            {
                if (rrddim_flag_check(rd, RRDDIM_FLAG_HIDDEN) || rrddim_flag_check(rd, RRDDIM_FLAG_OBSOLETE))
                    continue;
                dimensions++;
            }
        }

        rrdset_unlock(st);
    }
    {
        char b[7];
        snprintfz(b, 6, "%ld", dimensions);
        analytics_set_data(&analytics_data.netdata_metrics_count, b);
    }
}

void analytics_alarms(void)
{
    int alarm_warn = 0, alarm_crit = 0, alarm_normal = 0;
    char b[10];
    RRDCALC *rc;
    for (rc = localhost->alarms; rc; rc = rc->next) {
        if (unlikely(!rc->rrdset || !rc->rrdset->last_collected_time.tv_sec))
            continue;

        switch (rc->status) {
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
    analytics_set_data(&analytics_data.netdata_alarms_normal, b);
    snprintfz(b, 9, "%d", alarm_warn);
    analytics_set_data(&analytics_data.netdata_alarms_warning, b);
    snprintfz(b, 9, "%d", alarm_crit);
    analytics_set_data(&analytics_data.netdata_alarms_critical, b);
}

/*
 * Misc attributes to get (run from start)
 */
void analytics_misc(void)
{
#ifdef ENABLE_ACLK
    analytics_set_data(&analytics_data.netdata_host_cloud_available, "true");
    analytics_set_data_str(&analytics_data.netdata_host_aclk_implementation, "Next Generation");
#else
    analytics_set_data(&analytics_data.netdata_host_cloud_available, "false");
    analytics_set_data_str(&analytics_data.netdata_host_aclk_implementation, "");
#endif

    analytics_set_data(&analytics_data.netdata_config_exporting_enabled, appconfig_get_boolean(&exporting_config, CONFIG_SECTION_EXPORTING, "enabled", CONFIG_BOOLEAN_NO) ? "true" : "false");

    analytics_set_data(&analytics_data.netdata_config_is_private_registry, "false");
    analytics_set_data(&analytics_data.netdata_config_use_private_registry, "false");

    if (strcmp(
        config_get(CONFIG_SECTION_REGISTRY, "registry to announce", "https://registry.my-netdata.io"),
        "https://registry.my-netdata.io"))
        analytics_set_data(&analytics_data.netdata_config_use_private_registry, "true");

    //do we need both registry to announce and enabled to indicate that this is a private registry ?
    if (config_get_boolean(CONFIG_SECTION_REGISTRY, "enabled", CONFIG_BOOLEAN_NO) &&
        web_server_mode != WEB_SERVER_MODE_NONE)
        analytics_set_data(&analytics_data.netdata_config_is_private_registry, "true");
}

void analytics_aclk(void)
{
#ifdef ENABLE_ACLK
    if (aclk_connected) {
        analytics_set_data(&analytics_data.netdata_host_aclk_available, "true");
        analytics_set_data_str(&analytics_data.netdata_host_aclk_protocol, "New");
    }
    else
#endif
        analytics_set_data(&analytics_data.netdata_host_aclk_available, "false");
}

/*
 * Get the meta data, called from the thread once after the original delay
 * These are values that won't change during agent runtime, and therefore
 * don't try to read them on each META event send
 */
void analytics_gather_immutable_meta_data(void)
{
    analytics_misc();
    analytics_exporters();
    analytics_https();
}

/*
 * Get the meta data, called from the thread on every heartbeat, and right before the EXIT event
 * These are values that can change between agent restarts, and therefore
 * try to read them on each META event send
 */
void analytics_gather_mutable_meta_data(void)
{
    rrdhost_rdlock(localhost);

    analytics_collectors();
    analytics_alarms();
    analytics_charts();
    analytics_metrics();
    analytics_aclk();

    rrdhost_unlock(localhost);

    analytics_mirrored_hosts();
    analytics_alarms_notifications();

    analytics_set_data(
        &analytics_data.netdata_config_is_parent, (localhost->next || configured_as_parent()) ? "true" : "false");

    char *claim_id = get_agent_claimid();
    analytics_set_data(&analytics_data.netdata_host_agent_claimed, claim_id ? "true" : "false");
    freez(claim_id);

    {
        char b[7];
        snprintfz(b, 6, "%d", analytics_data.prometheus_hits);
        analytics_set_data(&analytics_data.netdata_allmetrics_prometheus_used, b);

        snprintfz(b, 6, "%d", analytics_data.shell_hits);
        analytics_set_data(&analytics_data.netdata_allmetrics_shell_used, b);

        snprintfz(b, 6, "%d", analytics_data.json_hits);
        analytics_set_data(&analytics_data.netdata_allmetrics_json_used, b);

        snprintfz(b, 6, "%d", analytics_data.dashboard_hits);
        analytics_set_data(&analytics_data.netdata_dashboard_used, b);

        snprintfz(b, 6, "%zu", rrd_hosts_available);
        analytics_set_data(&analytics_data.netdata_config_hosts_available, b);
    }
}

void analytics_main_cleanup(void *ptr)
{
    struct netdata_static_thread *static_thread = (struct netdata_static_thread *)ptr;
    static_thread->enabled = NETDATA_MAIN_THREAD_EXITING;

    debug(D_ANALYTICS, "Cleaning up...");
    analytics_free_data();

    static_thread->enabled = NETDATA_MAIN_THREAD_EXITED;
}

/*
 * The analytics thread. Sleep for ANALYTICS_INIT_SLEEP_SEC,
 * gather the data, and then go to a loop where every ANALYTICS_HEARTBEAT
 * it will send a new META event after gathering data that could be changed
 * while the agent is running
 */
void *analytics_main(void *ptr)
{
    netdata_thread_cleanup_push(analytics_main_cleanup, ptr);
    unsigned int sec = 0;
    heartbeat_t hb;
    heartbeat_init(&hb);
    usec_t step_ut = USEC_PER_SEC;

    debug(D_ANALYTICS, "Analytics thread starts");

    //first delay after agent start
    while (!netdata_exit && likely(sec <= ANALYTICS_INIT_SLEEP_SEC)) {
        heartbeat_next(&hb, step_ut);
        sec++;
    }

    if (unlikely(netdata_exit))
        goto cleanup;

    analytics_gather_immutable_meta_data();
    analytics_gather_mutable_meta_data();
    send_statistics("META_START", "-", "-");
    analytics_log_data();

    sec = 0;
    while (1) {
        heartbeat_next(&hb, step_ut * 2);
        sec += 2;

        if (unlikely(netdata_exit))
            break;

        if (likely(sec < ANALYTICS_HEARTBEAT))
            continue;

        analytics_gather_mutable_meta_data();
        send_statistics("META", "-", "-");
        analytics_log_data();
        sec = 0;
    }

cleanup:
    netdata_thread_cleanup_pop(1);
    return NULL;
}

static const char *verify_required_directory(const char *dir)
{
    if (chdir(dir) == -1)
        fatal("Cannot change directory to '%s'", dir);

    DIR *d = opendir(dir);
    if (!d)
        fatal("Cannot examine the contents of directory '%s'", dir);
    closedir(d);

    return dir;
}

/*
 * This is called after the rrdinit
 * These values will be sent on the START event
 */
void set_late_global_environment()
{
    analytics_set_data(&analytics_data.netdata_config_stream_enabled, default_rrdpush_enabled ? "true" : "false");
    analytics_set_data_str(&analytics_data.netdata_config_memory_mode, (char *)rrd_memory_mode_name(default_rrd_memory_mode));

#ifdef DISABLE_CLOUD
    analytics_set_data(&analytics_data.netdata_host_cloud_enabled, "false");
#else
    analytics_set_data(
        &analytics_data.netdata_host_cloud_enabled,
        appconfig_get_boolean(&cloud_config, CONFIG_SECTION_GLOBAL, "enabled", CONFIG_BOOLEAN_YES) ? "true" : "false");
#endif

#ifdef ENABLE_DBENGINE
    {
        char b[16];
        snprintfz(b, 15, "%d", default_rrdeng_page_cache_mb);
        analytics_set_data(&analytics_data.netdata_config_page_cache_size, b);

        snprintfz(b, 15, "%d", default_multidb_disk_quota_mb);
        analytics_set_data(&analytics_data.netdata_config_multidb_disk_quota, b);
    }
#endif

#ifdef ENABLE_HTTPS
    analytics_set_data(&analytics_data.netdata_config_https_enabled, "true");
#else
    analytics_set_data(&analytics_data.netdata_config_https_enabled, "false");
#endif

    if (web_server_mode == WEB_SERVER_MODE_NONE)
        analytics_set_data(&analytics_data.netdata_config_web_enabled, "false");
    else
        analytics_set_data(&analytics_data.netdata_config_web_enabled, "true");

    analytics_set_data_str(&analytics_data.netdata_config_release_channel, (char *)get_release_channel());

    {
        BUFFER *bi = buffer_create(1000);
        analytics_build_info(bi);
        analytics_set_data_str(&analytics_data.netdata_buildinfo, (char *)buffer_tostring(bi));
        buffer_free(bi);
    }

    analytics_get_install_type();
}

void get_system_timezone(void)
{
    // avoid flood calls to stat(/etc/localtime)
    // http://stackoverflow.com/questions/4554271/how-to-avoid-excessive-stat-etc-localtime-calls-in-strftime-on-linux
    const char *tz = getenv("TZ");
    if (!tz || !*tz)
        setenv("TZ", config_get(CONFIG_SECTION_ENV_VARS, "TZ", ":/etc/localtime"), 0);

    char buffer[FILENAME_MAX + 1] = "";
    const char *timezone = NULL;
    ssize_t ret;

    // use the TZ variable
    if (tz && *tz && *tz != ':') {
        timezone = tz;
        info("TIMEZONE: using TZ variable '%s'", timezone);
    }

    // use the contents of /etc/timezone
    if (!timezone && !read_file("/etc/timezone", buffer, FILENAME_MAX)) {
        timezone = buffer;
        info("TIMEZONE: using the contents of /etc/timezone");
    }

    // read the link /etc/localtime
    if (!timezone) {
        ret = readlink("/etc/localtime", buffer, FILENAME_MAX);

        if (ret > 0) {
            buffer[ret] = '\0';

            char *cmp = "/usr/share/zoneinfo/";
            size_t cmp_len = strlen(cmp);

            char *s = strstr(buffer, cmp);
            if (s && s[cmp_len]) {
                timezone = &s[cmp_len];
                info("TIMEZONE: using the link of /etc/localtime: '%s'", timezone);
            }
        } else
            buffer[0] = '\0';
    }

    // find the timezone from strftime()
    if (!timezone) {
        time_t t;
        struct tm *tmp, tmbuf;

        t = now_realtime_sec();
        tmp = localtime_r(&t, &tmbuf);

        if (tmp != NULL) {
            if (strftime(buffer, FILENAME_MAX, "%Z", tmp) == 0)
                buffer[0] = '\0';
            else {
                buffer[FILENAME_MAX] = '\0';
                timezone = buffer;
                info("TIMEZONE: using strftime(): '%s'", timezone);
            }
        }
    }

    if (timezone && *timezone) {
        // make sure it does not have illegal characters
        // info("TIMEZONE: fixing '%s'", timezone);

        size_t len = strlen(timezone);
        char tmp[len + 1];
        char *d = tmp;
        *d = '\0';

        while (*timezone) {
            if (isalnum(*timezone) || *timezone == '_' || *timezone == '/')
                *d++ = *timezone++;
            else
                timezone++;
        }
        *d = '\0';
        strncpyz(buffer, tmp, len);
        timezone = buffer;
        info("TIMEZONE: fixed as '%s'", timezone);
    }

    if (!timezone || !*timezone)
        timezone = "unknown";

    netdata_configured_timezone = config_get(CONFIG_SECTION_GLOBAL, "timezone", timezone);

    //get the utc offset, and the timezone as returned by strftime
    //will be sent to the cloud
    //Note: This will need an agent restart to get new offset on time change (dst, etc).
    {
        time_t t;
        struct tm *tmp, tmbuf;
        char zone[FILENAME_MAX + 1];
        char sign[2], hh[3], mm[3];

        t = now_realtime_sec();
        tmp = localtime_r(&t, &tmbuf);

        if (tmp != NULL) {
            if (strftime(zone, FILENAME_MAX, "%Z", tmp) == 0) {
                netdata_configured_abbrev_timezone = strdupz("UTC");
            } else
                netdata_configured_abbrev_timezone = strdupz(zone);

            if (strftime(zone, FILENAME_MAX, "%z", tmp) == 0) {
                netdata_configured_utc_offset = 0;
            } else {
                sign[0] = zone[0] == '-' || zone[0] == '+' ? zone[0] : '0';
                sign[1] = '\0';
                hh[0] = isdigit(zone[1]) ? zone[1] : '0';
                hh[1] = isdigit(zone[2]) ? zone[2] : '0';
                hh[2] = '\0';
                mm[0] = isdigit(zone[3]) ? zone[3] : '0';
                mm[1] = isdigit(zone[4]) ? zone[4] : '0';
                mm[2] = '\0';

                netdata_configured_utc_offset = (str2i(hh) * 3600) + (str2i(mm) * 60);
                netdata_configured_utc_offset =
                    sign[0] == '-' ? -netdata_configured_utc_offset : netdata_configured_utc_offset;
            }
        } else {
            netdata_configured_abbrev_timezone = strdupz("UTC");
            netdata_configured_utc_offset = 0;
        }
    }
}

void set_global_environment()
{
    {
        char b[16];
        snprintfz(b, 15, "%d", default_rrd_update_every);
        setenv("NETDATA_UPDATE_EVERY", b, 1);
    }

    setenv("NETDATA_VERSION", program_version, 1);
    setenv("NETDATA_HOSTNAME", netdata_configured_hostname, 1);
    setenv("NETDATA_CONFIG_DIR", verify_required_directory(netdata_configured_user_config_dir), 1);
    setenv("NETDATA_USER_CONFIG_DIR", verify_required_directory(netdata_configured_user_config_dir), 1);
    setenv("NETDATA_STOCK_CONFIG_DIR", verify_required_directory(netdata_configured_stock_config_dir), 1);
    setenv("NETDATA_PLUGINS_DIR", verify_required_directory(netdata_configured_primary_plugins_dir), 1);
    setenv("NETDATA_WEB_DIR", verify_required_directory(netdata_configured_web_dir), 1);
    setenv("NETDATA_CACHE_DIR", verify_required_directory(netdata_configured_cache_dir), 1);
    setenv("NETDATA_LIB_DIR", verify_required_directory(netdata_configured_varlib_dir), 1);
    setenv("NETDATA_LOCK_DIR", netdata_configured_lock_dir, 1);
    setenv("NETDATA_LOG_DIR", verify_required_directory(netdata_configured_log_dir), 1);
    setenv("HOME", verify_required_directory(netdata_configured_home_dir), 1);
    setenv("NETDATA_HOST_PREFIX", netdata_configured_host_prefix, 1);

    {
        BUFFER *user_plugins_dirs = buffer_create(FILENAME_MAX);

        for (size_t i = 1; i < PLUGINSD_MAX_DIRECTORIES && plugin_directories[i]; i++) {
            if (i > 1)
                buffer_strcat(user_plugins_dirs, " ");
            buffer_strcat(user_plugins_dirs, plugin_directories[i]);
        }

        setenv("NETDATA_USER_PLUGINS_DIRS", buffer_tostring(user_plugins_dirs), 1);

        buffer_free(user_plugins_dirs);
    }

    analytics_data.data_length = 0;
    analytics_set_data(&analytics_data.netdata_config_stream_enabled, "null");
    analytics_set_data(&analytics_data.netdata_config_memory_mode, "null");
    analytics_set_data(&analytics_data.netdata_config_exporting_enabled, "null");
    analytics_set_data(&analytics_data.netdata_exporting_connectors, "null");
    analytics_set_data(&analytics_data.netdata_allmetrics_prometheus_used, "null");
    analytics_set_data(&analytics_data.netdata_allmetrics_shell_used, "null");
    analytics_set_data(&analytics_data.netdata_allmetrics_json_used, "null");
    analytics_set_data(&analytics_data.netdata_dashboard_used, "null");
    analytics_set_data(&analytics_data.netdata_collectors, "null");
    analytics_set_data(&analytics_data.netdata_collectors_count, "null");
    analytics_set_data(&analytics_data.netdata_buildinfo, "null");
    analytics_set_data(&analytics_data.netdata_config_page_cache_size, "null");
    analytics_set_data(&analytics_data.netdata_config_multidb_disk_quota, "null");
    analytics_set_data(&analytics_data.netdata_config_https_enabled, "null");
    analytics_set_data(&analytics_data.netdata_config_web_enabled, "null");
    analytics_set_data(&analytics_data.netdata_config_release_channel, "null");
    analytics_set_data(&analytics_data.netdata_mirrored_host_count, "null");
    analytics_set_data(&analytics_data.netdata_mirrored_hosts_reachable, "null");
    analytics_set_data(&analytics_data.netdata_mirrored_hosts_unreachable, "null");
    analytics_set_data(&analytics_data.netdata_notification_methods, "null");
    analytics_set_data(&analytics_data.netdata_alarms_normal, "null");
    analytics_set_data(&analytics_data.netdata_alarms_warning, "null");
    analytics_set_data(&analytics_data.netdata_alarms_critical, "null");
    analytics_set_data(&analytics_data.netdata_charts_count, "null");
    analytics_set_data(&analytics_data.netdata_metrics_count, "null");
    analytics_set_data(&analytics_data.netdata_config_is_parent, "null");
    analytics_set_data(&analytics_data.netdata_config_hosts_available, "null");
    analytics_set_data(&analytics_data.netdata_host_cloud_available, "null");
    analytics_set_data(&analytics_data.netdata_host_aclk_implementation, "null");
    analytics_set_data(&analytics_data.netdata_host_aclk_available, "null");
    analytics_set_data(&analytics_data.netdata_host_aclk_protocol, "null");
    analytics_set_data(&analytics_data.netdata_host_agent_claimed, "null");
    analytics_set_data(&analytics_data.netdata_host_cloud_enabled, "null");
    analytics_set_data(&analytics_data.netdata_config_https_available, "null");
    analytics_set_data(&analytics_data.netdata_install_type, "null");
    analytics_set_data(&analytics_data.netdata_config_is_private_registry, "null");
    analytics_set_data(&analytics_data.netdata_config_use_private_registry, "null");
    analytics_set_data(&analytics_data.netdata_config_oom_score, "null");
    analytics_set_data(&analytics_data.netdata_prebuilt_distro, "null");

    analytics_data.prometheus_hits = 0;
    analytics_data.shell_hits = 0;
    analytics_data.json_hits = 0;
    analytics_data.dashboard_hits = 0;

    char *default_port = appconfig_get(&netdata_config, CONFIG_SECTION_WEB, "default port", NULL);
    int clean = 0;
    if (!default_port) {
        default_port = strdupz("19999");
        clean = 1;
    }

    setenv("NETDATA_LISTEN_PORT", default_port, 1);
    if (clean)
        freez(default_port);

    // set the path we need
    char path[1024 + 1], *p = getenv("PATH");
    if (!p)
        p = "/bin:/usr/bin";
    snprintfz(path, 1024, "%s:%s", p, "/sbin:/usr/sbin:/usr/local/bin:/usr/local/sbin");
    setenv("PATH", config_get(CONFIG_SECTION_ENV_VARS, "PATH", path), 1);

    // python options
    p = getenv("PYTHONPATH");
    if (!p)
        p = "";
    setenv("PYTHONPATH", config_get(CONFIG_SECTION_ENV_VARS, "PYTHONPATH", p), 1);

    // disable buffering for python plugins
    setenv("PYTHONUNBUFFERED", "1", 1);

    // switch to standard locale for plugins
    setenv("LC_ALL", "C", 1);
}

void send_statistics(const char *action, const char *action_result, const char *action_data)
{
    static char *as_script;

    if (netdata_anonymous_statistics_enabled == -1) {
        char *optout_file = mallocz(
            sizeof(char) *
            (strlen(netdata_configured_user_config_dir) + strlen(".opt-out-from-anonymous-statistics") + 2));
        sprintf(optout_file, "%s/%s", netdata_configured_user_config_dir, ".opt-out-from-anonymous-statistics");
        if (likely(access(optout_file, R_OK) != 0)) {
            as_script = mallocz(
                sizeof(char) *
                (strlen(netdata_configured_primary_plugins_dir) + strlen("anonymous-statistics.sh") + 2));
            sprintf(as_script, "%s/%s", netdata_configured_primary_plugins_dir, "anonymous-statistics.sh");
            if (unlikely(access(as_script, R_OK) != 0)) {
                netdata_anonymous_statistics_enabled = 0;
                info("Anonymous statistics script %s not found.", as_script);
                freez(as_script);
            } else {
                netdata_anonymous_statistics_enabled = 1;
            }
        } else {
            netdata_anonymous_statistics_enabled = 0;
            as_script = NULL;
        }
        freez(optout_file);
    }
    if (!netdata_anonymous_statistics_enabled)
        return;
    if (!action)
        return;
    if (!action_result)
        action_result = "";
    if (!action_data)
        action_data = "";
    char *command_to_run = mallocz(
        sizeof(char) * (strlen(action) + strlen(action_result) + strlen(action_data) + strlen(as_script) +
                        analytics_data.data_length + (ANALYTICS_NO_OF_ITEMS * 3) + 15));
    pid_t command_pid;

    sprintf(
        command_to_run,
        "%s '%s' '%s' '%s' '%s' '%s' '%s' '%s' '%s' '%s' '%s' '%s' '%s' '%s' '%s' '%s' '%s' '%s' '%s' '%s' '%s' '%s' '%s' '%s' '%s' '%s' '%s' '%s' '%s' '%s' '%s' '%s' '%s' '%s' '%s' '%s' '%s' '%s' '%s' '%s' '%s' '%s' '%s' ",
        as_script,
        action,
        action_result,
        action_data,
        analytics_data.netdata_config_stream_enabled,
        analytics_data.netdata_config_memory_mode,
        analytics_data.netdata_config_exporting_enabled,
        analytics_data.netdata_exporting_connectors,
        analytics_data.netdata_allmetrics_prometheus_used,
        analytics_data.netdata_allmetrics_shell_used,
        analytics_data.netdata_allmetrics_json_used,
        analytics_data.netdata_dashboard_used,
        analytics_data.netdata_collectors,
        analytics_data.netdata_collectors_count,
        analytics_data.netdata_buildinfo,
        analytics_data.netdata_config_page_cache_size,
        analytics_data.netdata_config_multidb_disk_quota,
        analytics_data.netdata_config_https_enabled,
        analytics_data.netdata_config_web_enabled,
        analytics_data.netdata_config_release_channel,
        analytics_data.netdata_mirrored_host_count,
        analytics_data.netdata_mirrored_hosts_reachable,
        analytics_data.netdata_mirrored_hosts_unreachable,
        analytics_data.netdata_notification_methods,
        analytics_data.netdata_alarms_normal,
        analytics_data.netdata_alarms_warning,
        analytics_data.netdata_alarms_critical,
        analytics_data.netdata_charts_count,
        analytics_data.netdata_metrics_count,
        analytics_data.netdata_config_is_parent,
        analytics_data.netdata_config_hosts_available,
        analytics_data.netdata_host_cloud_available,
        analytics_data.netdata_host_aclk_available,
        analytics_data.netdata_host_aclk_protocol,
        analytics_data.netdata_host_aclk_implementation,
        analytics_data.netdata_host_agent_claimed,
        analytics_data.netdata_host_cloud_enabled,
        analytics_data.netdata_config_https_available,
        analytics_data.netdata_install_type,
        analytics_data.netdata_config_is_private_registry,
        analytics_data.netdata_config_use_private_registry,
        analytics_data.netdata_config_oom_score,
        analytics_data.netdata_prebuilt_distro);

    info("%s '%s' '%s' '%s'", as_script, action, action_result, action_data);

    FILE *fp = mypopen(command_to_run, &command_pid);
    if (fp) {
        char buffer[4 + 1];
        char *s = fgets(buffer, 4, fp);
        int exit_code = mypclose(fp, command_pid);
        if (exit_code)
            error("Execution of anonymous statistics script returned %d.", exit_code);
        if (s && strncmp(buffer, "200", 3))
            error("Execution of anonymous statistics script returned http code %s.", buffer);
    } else {
        error("Failed to run anonymous statistics script %s.", as_script);
    }
    freez(command_to_run);
}
