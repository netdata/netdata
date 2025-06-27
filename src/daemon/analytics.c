// SPDX-License-Identifier: GPL-3.0-or-later

#include "analytics.h"
#include "common.h"
#include "buildinfo.h"

struct analytics_data analytics_data;
extern void analytics_exporting_connectors (BUFFER *b);
extern void analytics_exporting_connectors_ssl (BUFFER *b);
extern void analytics_build_info (BUFFER *b);

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
    netdata_log_debug(D_ANALYTICS, "NETDATA_CONFIG_STREAM_ENABLED      : [%s]", analytics_data.netdata_config_stream_enabled);
    netdata_log_debug(D_ANALYTICS, "NETDATA_CONFIG_MEMORY_MODE         : [%s]", analytics_data.netdata_config_memory_mode);
    netdata_log_debug(D_ANALYTICS, "NETDATA_CONFIG_EXPORTING_ENABLED   : [%s]", analytics_data.netdata_config_exporting_enabled);
    netdata_log_debug(D_ANALYTICS, "NETDATA_EXPORTING_CONNECTORS       : [%s]", analytics_data.netdata_exporting_connectors);
    netdata_log_debug(D_ANALYTICS, "NETDATA_ALLMETRICS_PROMETHEUS_USED : [%s]", analytics_data.netdata_allmetrics_prometheus_used);
    netdata_log_debug(D_ANALYTICS, "NETDATA_ALLMETRICS_SHELL_USED      : [%s]", analytics_data.netdata_allmetrics_shell_used);
    netdata_log_debug(D_ANALYTICS, "NETDATA_ALLMETRICS_JSON_USED       : [%s]", analytics_data.netdata_allmetrics_json_used);
    netdata_log_debug(D_ANALYTICS, "NETDATA_DASHBOARD_USED             : [%s]", analytics_data.netdata_dashboard_used);
    netdata_log_debug(D_ANALYTICS, "NETDATA_COLLECTORS                 : [%s]", analytics_data.netdata_collectors);
    netdata_log_debug(D_ANALYTICS, "NETDATA_COLLECTORS_COUNT           : [%s]", analytics_data.netdata_collectors_count);
    netdata_log_debug(D_ANALYTICS, "NETDATA_BUILDINFO                  : [%s]", analytics_data.netdata_buildinfo);
    netdata_log_debug(D_ANALYTICS, "NETDATA_CONFIG_PAGE_CACHE_SIZE     : [%s]", analytics_data.netdata_config_page_cache_size);
    netdata_log_debug(D_ANALYTICS, "NETDATA_CONFIG_MULTIDB_DISK_QUOTA  : [%s]", analytics_data.netdata_config_multidb_disk_quota);
    netdata_log_debug(D_ANALYTICS, "NETDATA_CONFIG_HTTPS_ENABLED       : [%s]", analytics_data.netdata_config_https_enabled);
    netdata_log_debug(D_ANALYTICS, "NETDATA_CONFIG_WEB_ENABLED         : [%s]", analytics_data.netdata_config_web_enabled);
    netdata_log_debug(D_ANALYTICS, "NETDATA_CONFIG_RELEASE_CHANNEL     : [%s]", analytics_data.netdata_config_release_channel);
    netdata_log_debug(D_ANALYTICS, "NETDATA_MIRRORED_HOST_COUNT        : [%s]", analytics_data.netdata_mirrored_host_count);
    netdata_log_debug(D_ANALYTICS, "NETDATA_MIRRORED_HOSTS_REACHABLE   : [%s]", analytics_data.netdata_mirrored_hosts_reachable);
    netdata_log_debug(D_ANALYTICS, "NETDATA_MIRRORED_HOSTS_UNREACHABLE : [%s]", analytics_data.netdata_mirrored_hosts_unreachable);
    netdata_log_debug(D_ANALYTICS, "NETDATA_NOTIFICATION_METHODS       : [%s]", analytics_data.netdata_notification_methods);
    netdata_log_debug(D_ANALYTICS, "NETDATA_ALARMS_NORMAL              : [%s]", analytics_data.netdata_alarms_normal);
    netdata_log_debug(D_ANALYTICS, "NETDATA_ALARMS_WARNING             : [%s]", analytics_data.netdata_alarms_warning);
    netdata_log_debug(D_ANALYTICS, "NETDATA_ALARMS_CRITICAL            : [%s]", analytics_data.netdata_alarms_critical);
    netdata_log_debug(D_ANALYTICS, "NETDATA_CHARTS_COUNT               : [%s]", analytics_data.netdata_charts_count);
    netdata_log_debug(D_ANALYTICS, "NETDATA_METRICS_COUNT              : [%s]", analytics_data.netdata_metrics_count);
    netdata_log_debug(D_ANALYTICS, "NETDATA_CONFIG_IS_PARENT           : [%s]", analytics_data.netdata_config_is_parent);
    netdata_log_debug(D_ANALYTICS, "NETDATA_CONFIG_HOSTS_AVAILABLE     : [%s]", analytics_data.netdata_config_hosts_available);
    netdata_log_debug(D_ANALYTICS, "NETDATA_HOST_CLOUD_AVAILABLE       : [%s]", analytics_data.netdata_host_cloud_available);
    netdata_log_debug(D_ANALYTICS, "NETDATA_HOST_ACLK_AVAILABLE        : [%s]", analytics_data.netdata_host_aclk_available);
    netdata_log_debug(D_ANALYTICS, "NETDATA_HOST_ACLK_PROTOCOL         : [%s]", analytics_data.netdata_host_aclk_protocol);
    netdata_log_debug(D_ANALYTICS, "NETDATA_HOST_ACLK_IMPLEMENTATION   : [%s]", analytics_data.netdata_host_aclk_implementation);
    netdata_log_debug(D_ANALYTICS, "NETDATA_HOST_AGENT_CLAIMED         : [%s]", analytics_data.netdata_host_agent_claimed);
    netdata_log_debug(D_ANALYTICS, "NETDATA_HOST_CLOUD_ENABLED         : [%s]", analytics_data.netdata_host_cloud_enabled);
    netdata_log_debug(D_ANALYTICS, "NETDATA_CONFIG_HTTPS_AVAILABLE     : [%s]", analytics_data.netdata_config_https_available);
    netdata_log_debug(D_ANALYTICS, "NETDATA_INSTALL_TYPE               : [%s]", analytics_data.netdata_install_type);
    netdata_log_debug(D_ANALYTICS, "NETDATA_PREBUILT_DISTRO            : [%s]", analytics_data.netdata_prebuilt_distro);
    netdata_log_debug(D_ANALYTICS, "NETDATA_CONFIG_IS_PRIVATE_REGISTRY : [%s]", analytics_data.netdata_config_is_private_registry);
    netdata_log_debug(D_ANALYTICS, "NETDATA_CONFIG_USE_PRIVATE_REGISTRY: [%s]", analytics_data.netdata_config_use_private_registry);
    netdata_log_debug(D_ANALYTICS, "NETDATA_CONFIG_OOM_SCORE           : [%s]", analytics_data.netdata_config_oom_score);
}

/*
 * Free data
 */
void analytics_free_data(void)
{
    freez(analytics_data.netdata_config_stream_enabled);
    analytics_data.netdata_config_stream_enabled = NULL;

    freez(analytics_data.netdata_config_memory_mode);
    analytics_data.netdata_config_memory_mode = NULL;

    freez(analytics_data.netdata_config_exporting_enabled);
    analytics_data.netdata_config_exporting_enabled = NULL;

    freez(analytics_data.netdata_exporting_connectors);
    analytics_data.netdata_exporting_connectors = NULL;

    freez(analytics_data.netdata_allmetrics_prometheus_used);
    analytics_data.netdata_allmetrics_prometheus_used = NULL;

    freez(analytics_data.netdata_allmetrics_shell_used);
    analytics_data.netdata_allmetrics_shell_used = NULL;

    freez(analytics_data.netdata_allmetrics_json_used);
    analytics_data.netdata_allmetrics_json_used = NULL;

    freez(analytics_data.netdata_dashboard_used);
    analytics_data.netdata_dashboard_used = NULL;

    freez(analytics_data.netdata_collectors);
    analytics_data.netdata_collectors = NULL;

    freez(analytics_data.netdata_collectors_count);
    analytics_data.netdata_collectors_count = NULL;

    freez(analytics_data.netdata_buildinfo);
    analytics_data.netdata_buildinfo = NULL;

    freez(analytics_data.netdata_config_page_cache_size);
    analytics_data.netdata_config_page_cache_size = NULL;

    freez(analytics_data.netdata_config_multidb_disk_quota);
    analytics_data.netdata_config_multidb_disk_quota = NULL;

    freez(analytics_data.netdata_config_https_enabled);
    analytics_data.netdata_config_https_enabled = NULL;

    freez(analytics_data.netdata_config_web_enabled);
    analytics_data.netdata_config_web_enabled = NULL;

    freez(analytics_data.netdata_config_release_channel);
    analytics_data.netdata_config_release_channel = NULL;

    freez(analytics_data.netdata_mirrored_host_count);
    analytics_data.netdata_mirrored_host_count = NULL;

    freez(analytics_data.netdata_mirrored_hosts_reachable);
    analytics_data.netdata_mirrored_hosts_reachable = NULL;

    freez(analytics_data.netdata_mirrored_hosts_unreachable);
    analytics_data.netdata_mirrored_hosts_unreachable = NULL;

    freez(analytics_data.netdata_notification_methods);
    analytics_data.netdata_notification_methods = NULL;

    freez(analytics_data.netdata_alarms_normal);
    analytics_data.netdata_alarms_normal = NULL;

    freez(analytics_data.netdata_alarms_warning);
    analytics_data.netdata_alarms_warning = NULL;

    freez(analytics_data.netdata_alarms_critical);
    analytics_data.netdata_alarms_critical = NULL;

    freez(analytics_data.netdata_charts_count);
    analytics_data.netdata_charts_count = NULL;

    freez(analytics_data.netdata_metrics_count);
    analytics_data.netdata_metrics_count = NULL;

    freez(analytics_data.netdata_config_is_parent);
    analytics_data.netdata_config_is_parent = NULL;

    freez(analytics_data.netdata_config_hosts_available);
    analytics_data.netdata_config_hosts_available = NULL;

    freez(analytics_data.netdata_host_cloud_available);
    analytics_data.netdata_host_cloud_available = NULL;

    freez(analytics_data.netdata_host_aclk_available);
    analytics_data.netdata_host_aclk_available = NULL;

    freez(analytics_data.netdata_host_aclk_protocol);
    analytics_data.netdata_host_aclk_protocol = NULL;

    freez(analytics_data.netdata_host_aclk_implementation);
    analytics_data.netdata_host_aclk_implementation = NULL;

    freez(analytics_data.netdata_host_agent_claimed);
    analytics_data.netdata_host_agent_claimed = NULL;

    freez(analytics_data.netdata_host_cloud_enabled);
    analytics_data.netdata_host_cloud_enabled = NULL;

    freez(analytics_data.netdata_config_https_available);
    analytics_data.netdata_config_https_available = NULL;

    freez(analytics_data.netdata_install_type);
    analytics_data.netdata_install_type = NULL;

    freez(analytics_data.netdata_config_is_private_registry);
    analytics_data.netdata_config_is_private_registry = NULL;

    freez(analytics_data.netdata_config_use_private_registry);
    analytics_data.netdata_config_use_private_registry = NULL;

    freez(analytics_data.netdata_config_oom_score);
    analytics_data.netdata_config_oom_score = NULL;

    freez(analytics_data.netdata_prebuilt_distro);
    analytics_data.netdata_prebuilt_distro = NULL;

    freez(analytics_data.netdata_fail_reason);
    analytics_data.netdata_fail_reason = NULL;

}

/*
 * Set a numeric/boolean data with a value
 */
void analytics_set_data(char **name, char *value)
{
    spinlock_lock(&analytics_data.spinlock);
    if (*name) {
        analytics_data.data_length -= strlen(*name);
        freez(*name);
    }
    *name = strdupz(value);
    analytics_data.data_length += strlen(*name);
    spinlock_unlock(&analytics_data.spinlock);
}

/*
 * Set a string data with a value
 */
void analytics_set_data_str(char **name, const char *value)
{
    size_t value_string_len;
    spinlock_lock(&analytics_data.spinlock);
    if (*name) {
        analytics_data.data_length -= strlen(*name);
        freez(*name);
    }
    value_string_len = strlen(value) + 4;
    *name = mallocz(sizeof(char) * value_string_len);
    snprintfz(*name, value_string_len - 1, "\"%s\"", value);
    analytics_data.data_length += strlen(*name);
    spinlock_unlock(&analytics_data.spinlock);
}

/*
 * Log hits on the allmetrics page, with prometheus parameter
 */
void analytics_log_prometheus(void)
{
    if (netdata_anonymous_statistics_enabled && likely(analytics_data.prometheus_hits < ANALYTICS_MAX_PROMETHEUS_HITS)) {
        analytics_data.prometheus_hits++;
        char b[21];
        snprintfz(b, sizeof(b) - 1, "%zu", analytics_data.prometheus_hits);
        analytics_set_data(&analytics_data.netdata_allmetrics_prometheus_used, b);
    }
}

/*
 * Log hits on the allmetrics page, with shell parameter (or default)
 */
void analytics_log_shell(void)
{
    if (netdata_anonymous_statistics_enabled && likely(analytics_data.shell_hits < ANALYTICS_MAX_SHELL_HITS)) {
        analytics_data.shell_hits++;
        char b[21];
        snprintfz(b, sizeof(b) - 1, "%zu", analytics_data.shell_hits);
        analytics_set_data(&analytics_data.netdata_allmetrics_shell_used, b);
    }
}

/*
 * Log hits on the allmetrics page, with json parameter
 */
void analytics_log_json(void)
{
    if (netdata_anonymous_statistics_enabled && likely(analytics_data.json_hits < ANALYTICS_MAX_JSON_HITS)) {
        analytics_data.json_hits++;
        char b[21];
        snprintfz(b, sizeof(b) - 1, "%zu", analytics_data.json_hits);
        analytics_set_data(&analytics_data.netdata_allmetrics_json_used, b);
    }
}

/*
 * Log hits on the dashboard, (when calling HELLO).
 */
void analytics_log_dashboard(void)
{
    if (netdata_anonymous_statistics_enabled && likely(analytics_data.dashboard_hits < ANALYTICS_MAX_DASHBOARD_HITS)) {
        analytics_data.dashboard_hits++;
        char b[21];
        snprintfz(b, sizeof(b) - 1, "%zu", analytics_data.dashboard_hits);
        analytics_set_data(&analytics_data.netdata_dashboard_used, b);
    }
}

/*
 * Called when setting the oom score
 */
void analytics_report_oom_score(long long int score){
    char b[21];
    snprintfz(b, sizeof(b) - 1, "%lld", score);
    analytics_set_data(&analytics_data.netdata_config_oom_score, b);
}

void analytics_mirrored_hosts(void)
{
    RRDHOST *host;
    size_t count = 0;
    size_t reachable = 0;
    size_t unreachable = 0;
    char b[21];

    rrd_rdlock();
    rrdhost_foreach_read(host)
    {
        if (rrdhost_flag_check(host, RRDHOST_FLAG_ARCHIVED))
            continue;

        ((host == localhost || !rrdhost_flag_check(host, RRDHOST_FLAG_ORPHAN)) ? reachable++ : unreachable++);

        count++;
    }
    rrd_rdunlock();

    snprintfz(b, sizeof(b) - 1, "%zu", count);
    analytics_set_data(&analytics_data.netdata_mirrored_host_count, b);
    snprintfz(b, sizeof(b) - 1, "%zu", reachable);
    analytics_set_data(&analytics_data.netdata_mirrored_hosts_reachable, b);
    snprintfz(b, sizeof(b) - 1, "%zu", unreachable);
    analytics_set_data(&analytics_data.netdata_mirrored_hosts_unreachable, b);
}

void analytics_exporters(void)
{
    //when no exporters are available, an empty string will be sent
    //decide if something else is more suitable (but probably not null)
    BUFFER *bi = buffer_create(1000, NULL);
    analytics_exporting_connectors(bi);
    analytics_set_data_str(&analytics_data.netdata_exporting_connectors, (char *)buffer_tostring(bi));
    buffer_free(bi);
}

int collector_counter_callb(const DICTIONARY_ITEM *item __maybe_unused, void *entry, void *data) {

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
    DICTIONARY *dict = dictionary_create(DICT_OPTION_SINGLE_THREADED);
    char name[500];
    BUFFER *bt = buffer_create(1000, NULL);

    rrdset_foreach_read(st, localhost) {
        if(!rrdset_is_available_for_viewers(st))
            continue;

        struct collector col = {
            .plugin = rrdset_plugin_name(st),
            .module = rrdset_module_name(st)
        };
        snprintfz(name, sizeof(name) - 1, "%s:%s", col.plugin, col.module);
        dictionary_set(dict, name, &col, sizeof(struct collector));
    }
    rrdset_foreach_done(st);

    struct array_printer ap;
    ap.c = 0;
    ap.both = bt;

    dictionary_walkthrough_read(dict, collector_counter_callb, &ap);
    dictionary_destroy(dict);

    analytics_set_data(&analytics_data.netdata_collectors, (char *)buffer_tostring(ap.both));

    {
        char b[21];
        snprintfz(b, sizeof(b) - 1, "%d", ap.c);
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
        netdata_log_info("Alarm notify script %s not found.", script);
        freez(script);
        return;
    }

    strcat(script, " dump_methods");

    netdata_log_debug(D_ANALYTICS, "Executing %s", script);

    BUFFER *b = buffer_create(1000, NULL);
    int cnt = 0;
    POPEN_INSTANCE *instance = spawn_popen_run(script);
    if (instance) {
        char line[200 + 1];

        while (fgets(line, 200, spawn_popen_stdout(instance)) != NULL) {
            char *end = line;
            while (*end && *end != '\n')
                end++;
            *end = '\0';

            if (likely(cnt))
                buffer_strcat(b, "|");

            buffer_strcat(b, line);

            cnt++;
        }
        spawn_popen_wait(instance);
    }
    freez(script);

    analytics_set_data_str(&analytics_data.netdata_notification_methods, (char *)buffer_tostring(b));

    buffer_free(b);
}

static void analytics_get_install_type(struct rrdhost_system_info *system_info) {
    const char *install_type = rrdhost_system_info_install_type(system_info);
    if(!install_type)
        install_type = "unknown";

    analytics_set_data_str(&analytics_data.netdata_install_type, install_type);

    const char *prebuilt_dist = rrdhost_system_info_prebuilt_dist(system_info);
    if(prebuilt_dist)
        analytics_set_data_str(&analytics_data.netdata_prebuilt_distro, prebuilt_dist);
}

/*
 * Pick up if https is actually used
 */
void analytics_https(void)
{
    BUFFER *b = buffer_create(30, NULL);
    analytics_exporting_connectors_ssl(b);

    buffer_strcat(b, stream_sender_is_connected_with_ssl(localhost) ? "streaming|" : "|");
    buffer_strcat(b, netdata_ssl_web_server_ctx ? "web" : "");

    analytics_set_data_str(&analytics_data.netdata_config_https_available, (char *)buffer_tostring(b));
    buffer_free(b);
}

void analytics_charts(void)
{
    RRDSET *st;
    size_t c = 0;

    rrdset_foreach_read(st, localhost)
        if(rrdset_is_available_for_viewers(st)) c++;
    rrdset_foreach_done(st);

    analytics_data.charts_count = c;
    {
        char b[21];
        snprintfz(b, sizeof(b) - 1, "%zu", c);
        analytics_set_data(&analytics_data.netdata_charts_count, b);
    }
}

void analytics_metrics(void)
{
    RRDSET *st;
    size_t dimensions = 0;
    rrdset_foreach_read(st, localhost) {
        if (rrdset_is_available_for_viewers(st)) {
            RRDDIM *rd;
            rrddim_foreach_read(rd, st) {
                if (rrddim_option_check(rd, RRDDIM_OPTION_HIDDEN) || rrddim_flag_check(rd, RRDDIM_FLAG_OBSOLETE))
                    continue;
                dimensions++;
            }
            rrddim_foreach_done(rd);
        }
    }
    rrdset_foreach_done(st);

    analytics_data.metrics_count = dimensions;
    {
        char b[21];
        snprintfz(b, sizeof(b) - 1, "%zu", dimensions);
        analytics_set_data(&analytics_data.netdata_metrics_count, b);
    }
}

void analytics_alarms(void)
{
    size_t alarm_warn = 0, alarm_crit = 0, alarm_normal = 0;
    char b[21];
    RRDCALC *rc;
    foreach_rrdcalc_in_rrdhost_read(localhost, rc) {
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
    foreach_rrdcalc_in_rrdhost_done(rc);

    snprintfz(b, sizeof(b) - 1, "%zu", alarm_normal);
    analytics_set_data(&analytics_data.netdata_alarms_normal, b);
    snprintfz(b, sizeof(b) - 1, "%zu", alarm_warn);
    analytics_set_data(&analytics_data.netdata_alarms_warning, b);
    snprintfz(b, sizeof(b) - 1, "%zu", alarm_crit);
    analytics_set_data(&analytics_data.netdata_alarms_critical, b);
}

/*
 * Misc attributes to get (run from start)
 */
void analytics_misc(void)
{
    analytics_set_data(&analytics_data.netdata_host_cloud_available, "true");
    analytics_set_data_str(&analytics_data.netdata_host_aclk_implementation, "Next Generation");

    analytics_data.exporting_enabled = inicfg_get_boolean(&exporting_config, CONFIG_SECTION_EXPORTING, "enabled", CONFIG_BOOLEAN_NO);
    analytics_set_data(&analytics_data.netdata_config_exporting_enabled,  analytics_data.exporting_enabled ? "true" : "false");

    analytics_set_data(&analytics_data.netdata_config_is_private_registry, "false");
    analytics_set_data(&analytics_data.netdata_config_use_private_registry, "false");

    if (strcmp(
        inicfg_get(&netdata_config, CONFIG_SECTION_REGISTRY, "registry to announce", "https://registry.my-netdata.io"),
        "https://registry.my-netdata.io") != 0)
        analytics_set_data(&analytics_data.netdata_config_use_private_registry, "true");

    //do we need both registry to announce and enabled to indicate that this is a private registry ?
    if (inicfg_get_boolean(&netdata_config, CONFIG_SECTION_REGISTRY, "enabled", CONFIG_BOOLEAN_NO) &&
        web_server_mode != WEB_SERVER_MODE_NONE)
        analytics_set_data(&analytics_data.netdata_config_is_private_registry, "true");
}

void analytics_aclk(void)
{
    if (aclk_online()) {
        analytics_set_data(&analytics_data.netdata_host_aclk_available, "true");
        analytics_set_data_str(&analytics_data.netdata_host_aclk_protocol, "New");
    }
    else
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
    analytics_collectors();
    analytics_alarms();
    analytics_charts();
    analytics_metrics();
    analytics_aclk();
    analytics_mirrored_hosts();
    analytics_alarms_notifications();

    analytics_set_data(
        &analytics_data.netdata_config_is_parent, (rrdhost_hosts_available() > 1 || netdata_conf_is_parent()) ? "true" : "false");

    analytics_set_data(&analytics_data.netdata_host_agent_claimed, is_agent_claimed() ? "true" : "false");

    {
        char b[21];
        snprintfz(b, sizeof(b) - 1, "%zu", analytics_data.prometheus_hits);
        analytics_set_data(&analytics_data.netdata_allmetrics_prometheus_used, b);

        snprintfz(b, sizeof(b) - 1, "%zu", analytics_data.shell_hits);
        analytics_set_data(&analytics_data.netdata_allmetrics_shell_used, b);

        snprintfz(b, sizeof(b) - 1, "%zu", analytics_data.json_hits);
        analytics_set_data(&analytics_data.netdata_allmetrics_json_used, b);

        snprintfz(b, sizeof(b) - 1, "%zu", analytics_data.dashboard_hits);
        analytics_set_data(&analytics_data.netdata_dashboard_used, b);

        snprintfz(b, sizeof(b) - 1, "%zu", rrdhost_hosts_available());
        analytics_set_data(&analytics_data.netdata_config_hosts_available, b);
    }
}

void analytics_main_cleanup(void *pptr)
{
    struct netdata_static_thread *static_thread = CLEANUP_FUNCTION_GET_PTR(pptr);
    if(!static_thread) return;

    static_thread->enabled = NETDATA_MAIN_THREAD_EXITING;

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
    CLEANUP_FUNCTION_REGISTER(analytics_main_cleanup) cleanup_ptr = ptr;
    unsigned int sec = 0;
    heartbeat_t hb;
    heartbeat_init(&hb, USEC_PER_SEC);

    netdata_log_debug(D_ANALYTICS, "Analytics thread starts");

    // first delay after agent start
    while (service_running(SERVICE_ANALYTICS) && likely(sec <= ANALYTICS_INIT_SLEEP_SEC)) {
        heartbeat_next(&hb);
        sec++;
        if (sec == ANALYTICS_INIT_IMMUTABLE_DATA_SEC)
            analytics_gather_immutable_meta_data();
    }

    if (unlikely(!service_running(SERVICE_ANALYTICS)))
        goto cleanup;

    analytics_gather_mutable_meta_data();

    analytics_statistic_t statistic = { "META_START", "-", "-"  };
    analytics_statistic_send(&statistic);
    analytics_log_data();

    sec = 0;
    while (1) {
        heartbeat_next(&hb);
        sec++;

        if (unlikely(!service_running(SERVICE_ANALYTICS)))
            break;

        if (likely(sec < ANALYTICS_HEARTBEAT))
            continue;

        analytics_gather_mutable_meta_data();

        analytics_statistic_t stt = { "META", "-", "-"  };
        analytics_statistic_send(&stt);
        analytics_log_data();

        sec = 0;
    }

cleanup:
    return NULL;
}

/*
 * This is called after the rrdinit
 * These values will be sent on the START event
 */
void set_late_analytics_variables(struct rrdhost_system_info *system_info)
{
    analytics_set_data(&analytics_data.netdata_config_stream_enabled, stream_send.enabled ? "true" : "false");
    analytics_set_data_str(&analytics_data.netdata_config_memory_mode, (char *)rrd_memory_mode_name(default_rrd_memory_mode));
    analytics_set_data(&analytics_data.netdata_host_cloud_enabled, "true");

#ifdef ENABLE_DBENGINE
    {
        char b[16];
        snprintfz(b, sizeof(b) - 1, "%d", default_rrdeng_page_cache_mb);
        analytics_set_data(&analytics_data.netdata_config_page_cache_size, b);

        snprintfz(b, sizeof(b) - 1, "%d", default_multidb_disk_quota_mb);
        analytics_set_data(&analytics_data.netdata_config_multidb_disk_quota, b);
    }
#endif

    analytics_set_data(&analytics_data.netdata_config_https_enabled, "true");

    if (web_server_mode == WEB_SERVER_MODE_NONE)
        analytics_set_data(&analytics_data.netdata_config_web_enabled, "false");
    else
        analytics_set_data(&analytics_data.netdata_config_web_enabled, "true");

    analytics_set_data_str(&analytics_data.netdata_config_release_channel, (char *)get_release_channel());

    {
        BUFFER *bi = buffer_create(1000, NULL);
        analytics_build_info(bi);
        analytics_set_data_str(&analytics_data.netdata_buildinfo, (char *)buffer_tostring(bi));
        buffer_free(bi);
    }

    analytics_get_install_type(system_info);
}

void get_system_timezone(void)
{
    char buffer[FILENAME_MAX + 1] = "";
    const char *timezone = NULL;
    const char *tz = NULL;
#ifdef OS_WINDOWS
    _tzset();
    size_t s;
    _get_tzname(&s, buffer, sizeof(buffer) -1, 0 );
    timezone = buffer;

    /*
    TIME_ZONE_INFORMATION win_tz;
    DWORD tzresult = GetTimeZoneInformation(&win_tz);
    if (tzresult != TIME_ZONE_ID_INVALID) {
        WideCharToMultiByte(CP_UTF8, 0, win_tz.DaylightName, -1, buffer, FILENAME_MAX, NULL, NULL);
    }
    */
#else
    // avoid flood calls to stat(/etc/localtime)
    // http://stackoverflow.com/questions/4554271/how-to-avoid-excessive-stat-etc-localtime-calls-in-strftime-on-linux
    tz = getenv("TZ");
    if (!tz || !*tz)
        setenv("TZ", inicfg_get(&netdata_config, CONFIG_SECTION_ENV_VARS, "TZ", ":/etc/localtime"), 0);
#endif

    ssize_t ret;

    // use the TZ variable
    if (tz && *tz && *tz != ':') {
        timezone = tz;
        netdata_log_info("TIMEZONE: using TZ variable '%s'", timezone);
    }

    // use the contents of /etc/timezone
    if (!timezone && !read_txt_file("/etc/timezone", buffer, sizeof(buffer))) {
        timezone = buffer;
        netdata_log_info("TIMEZONE: using the contents of /etc/timezone");
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
                netdata_log_info("TIMEZONE: using the link of /etc/localtime: '%s'", timezone);
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
                netdata_log_info("TIMEZONE: using strftime(): '%s'", timezone);
            }
        }
    }

    if (timezone && *timezone) {
        // make sure it does not have illegal characters
        // netdata_log_info("TIMEZONE: fixing '%s'", timezone);

        size_t len = strlen(timezone);
        char tmp[len + 1];
        char *d = tmp;
        *d = '\0';

        while (*timezone) {
            if (isalnum((uint8_t)*timezone) || *timezone == '_' || *timezone == '/')
                *d++ = *timezone++;
            else
                timezone++;
        }
        *d = '\0';
        strncpyz(buffer, tmp, len);
        timezone = buffer;
        netdata_log_info("TIMEZONE: fixed as '%s'", timezone);
    }

    if (!timezone || !*timezone)
        timezone = "unknown";

    netdata_configured_timezone = inicfg_get(&netdata_config, CONFIG_SECTION_GLOBAL, "timezone", timezone);

    //get the utc offset, and the timezone as returned by strftime
    //will be sent to the cloud
    //Note: This will need an agent restart to get new offset on time change (dst, etc).
    {
        time_t t;
        struct tm *tmp, tmbuf;
        char zone[FILENAME_MAX + 1];
        char sign[2], hh[3], mm[3];

        t = now_realtime_sec();
#ifdef OS_WINDOWS
        tmp = NULL;
#else
        tmp = localtime_r(&t, &tmbuf);
#endif

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
                hh[0] = isdigit((uint8_t)zone[1]) ? zone[1] : '0';
                hh[1] = isdigit((uint8_t)zone[2]) ? zone[2] : '0';
                hh[2] = '\0';
                mm[0] = isdigit((uint8_t)zone[3]) ? zone[3] : '0';
                mm[1] = isdigit((uint8_t)zone[4]) ? zone[4] : '0';
                mm[2] = '\0';

                netdata_configured_utc_offset = (str2i(hh) * 3600) + (str2i(mm) * 60);
                netdata_configured_utc_offset =
                    sign[0] == '-' ? -netdata_configured_utc_offset : netdata_configured_utc_offset;
            }
        } else {
#ifdef OS_WINDOWS
            long offset;
            if (!_get_timezone(&offset)) {
                offset /= 3600;
            } else {
                offset = 0;
            }

            DWORD tzresult = GetTimeZoneInformation(&win_tz);
            if (tzresult != TIME_ZONE_ID_INVALID) {
                WideCharToMultiByte(CP_UTF8, 0, win_tz.StandardName, -1, buffer, FILENAME_MAX, NULL, NULL);
                netdata_configured_abbrev_timezone = strdupz(buffer);
                netdata_configured_utc_offset = offset;
                return;
            }
#endif
            netdata_configured_abbrev_timezone = strdupz("UTC");
            netdata_configured_utc_offset = 0;
        }
    }
}

static bool analytics_script_exists(void) {
    char filename[FILENAME_MAX + 1];
    snprintfz(filename, sizeof(filename), "%s/anonymous-statistics.sh", netdata_configured_primary_plugins_dir);
    return access(filename, R_OK) == 0;
}

bool analytics_check_enabled(void) {
    if(!netdata_anonymous_statistics_enabled)
        return false;

    char filename[FILENAME_MAX + 1];
    snprintfz(filename, sizeof(filename), "%s/.opt-out-from-anonymous-statistics", netdata_configured_user_config_dir);

    if(access(filename, R_OK) != 0) {
        // the file is not there, check the environment variable
        const char *s = getenv("DISABLE_TELEMETRY");
        netdata_anonymous_statistics_enabled = !s || !*s;
    }
    else
        // the file is there, disable telemetry
        netdata_anonymous_statistics_enabled = false;

    return netdata_anonymous_statistics_enabled;
}

void analytics_statistic_send(const analytics_statistic_t *statistic) {
    if (!statistic || !statistic->action || !*statistic->action|| !analytics_check_enabled() || !analytics_script_exists())
        return;

    const char *action_result = statistic->result;
    const char *action_data = statistic->data;

    CLEAN_BUFFER *cmd = buffer_create(0, NULL);
    buffer_sprintf(
        cmd,
        "%s/anonymous-statistics.sh '%s' '%s' '%s' '%s' '%s' '%s' '%s' '%s' '%s' '%s' '%s' '%s' '%s' '%s' '%s' '%s' '%s' '%s' '%s' '%s' '%s' '%s' '%s' '%s' '%s' '%s' '%s' '%s' '%s' '%s' '%s' '%s' '%s' '%s' '%s' '%s' '%s' '%s' '%s' '%s' '%s' '%s' '%s' ",
        netdata_configured_primary_plugins_dir,
        statistic->action,
        action_result ? action_result : "",
        action_data ? action_data : "",
        analytics_data.netdata_config_stream_enabled ? analytics_data.netdata_config_stream_enabled : "",
        analytics_data.netdata_config_memory_mode ? analytics_data.netdata_config_memory_mode : "",
        analytics_data.netdata_config_exporting_enabled ? analytics_data.netdata_config_exporting_enabled : "",
        analytics_data.netdata_exporting_connectors ? analytics_data.netdata_exporting_connectors : "",
        analytics_data.netdata_allmetrics_prometheus_used ? analytics_data.netdata_allmetrics_prometheus_used : "",
        analytics_data.netdata_allmetrics_shell_used ? analytics_data.netdata_allmetrics_shell_used : "",
        analytics_data.netdata_allmetrics_json_used ? analytics_data.netdata_allmetrics_json_used : "",
        analytics_data.netdata_dashboard_used ? analytics_data.netdata_dashboard_used : "",
        analytics_data.netdata_collectors ? analytics_data.netdata_collectors : "",
        analytics_data.netdata_collectors_count ? analytics_data.netdata_collectors_count : "",
        analytics_data.netdata_buildinfo ? analytics_data.netdata_buildinfo : "",
        analytics_data.netdata_config_page_cache_size ? analytics_data.netdata_config_page_cache_size : "",
        analytics_data.netdata_config_multidb_disk_quota ? analytics_data.netdata_config_multidb_disk_quota : "",
        analytics_data.netdata_config_https_enabled ? analytics_data.netdata_config_https_enabled : "",
        analytics_data.netdata_config_web_enabled ? analytics_data.netdata_config_web_enabled : "",
        analytics_data.netdata_config_release_channel ? analytics_data.netdata_config_release_channel : "",
        analytics_data.netdata_mirrored_host_count ? analytics_data.netdata_mirrored_host_count : "",
        analytics_data.netdata_mirrored_hosts_reachable ? analytics_data.netdata_mirrored_hosts_reachable : "",
        analytics_data.netdata_mirrored_hosts_unreachable ? analytics_data.netdata_mirrored_hosts_unreachable : "",
        analytics_data.netdata_notification_methods ? analytics_data.netdata_notification_methods : "",
        analytics_data.netdata_alarms_normal ? analytics_data.netdata_alarms_normal : "",
        analytics_data.netdata_alarms_warning ? analytics_data.netdata_alarms_warning : "",
        analytics_data.netdata_alarms_critical ? analytics_data.netdata_alarms_critical : "",
        analytics_data.netdata_charts_count ? analytics_data.netdata_charts_count : "",
        analytics_data.netdata_metrics_count ? analytics_data.netdata_metrics_count : "",
        analytics_data.netdata_config_is_parent ? analytics_data.netdata_config_is_parent : "",
        analytics_data.netdata_config_hosts_available ? analytics_data.netdata_config_hosts_available : "",
        analytics_data.netdata_host_cloud_available ? analytics_data.netdata_host_cloud_available : "",
        analytics_data.netdata_host_aclk_available ? analytics_data.netdata_host_aclk_available : "",
        analytics_data.netdata_host_aclk_protocol ? analytics_data.netdata_host_aclk_protocol : "",
        analytics_data.netdata_host_aclk_implementation ? analytics_data.netdata_host_aclk_implementation : "",
        analytics_data.netdata_host_agent_claimed ? analytics_data.netdata_host_agent_claimed : "",
        analytics_data.netdata_host_cloud_enabled ? analytics_data.netdata_host_cloud_enabled : "",
        analytics_data.netdata_config_https_available ? analytics_data.netdata_config_https_available : "",
        analytics_data.netdata_install_type ? analytics_data.netdata_install_type : "",
        analytics_data.netdata_config_is_private_registry ? analytics_data.netdata_config_is_private_registry : "",
        analytics_data.netdata_config_use_private_registry ? analytics_data.netdata_config_use_private_registry : "",
        analytics_data.netdata_config_oom_score ? analytics_data.netdata_config_oom_score : "",
        analytics_data.netdata_prebuilt_distro ? analytics_data.netdata_prebuilt_distro : "",
        analytics_data.netdata_fail_reason ? analytics_data.netdata_fail_reason : ""
        );

    nd_log(NDLS_DAEMON, NDLP_DEBUG,
           "%s/anonymous-statistics.sh '%s' '%s' '%s'",
           netdata_configured_primary_plugins_dir, statistic->action,
           action_result ? action_result : "", action_data ? action_data : "");

    POPEN_INSTANCE *instance = spawn_popen_run(buffer_tostring(cmd));
    if (instance) {
        char buffer[4 + 1];
        char *s = fgets(buffer, 4, spawn_popen_stdout(instance));
        int exit_code = spawn_popen_wait(instance);
        if (exit_code)

            nd_log(NDLS_DAEMON, NDLP_NOTICE,
                   "Statistics script returned error: %d",
                   exit_code);

        if (s && strncmp(buffer, "200", 3) != 0)
            nd_log(NDLS_DAEMON, NDLP_NOTICE,
                   "Statistics script returned http code: %s",
                   buffer);

    }
    else
        nd_log(NDLS_DAEMON, NDLP_NOTICE,
               "Failed to run statistics script: %s/anonymous-statistics.sh",
               netdata_configured_primary_plugins_dir);
}

void analytics_reset(void) {
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
    analytics_set_data(&analytics_data.netdata_fail_reason, "null");

    analytics_data.prometheus_hits = 0;
    analytics_data.shell_hits = 0;
    analytics_data.json_hits = 0;
    analytics_data.dashboard_hits = 0;
    analytics_data.charts_count = 0;
    analytics_data.metrics_count = 0;
    analytics_data.exporting_enabled = false;
}

void analytics_init(void)
{
    spinlock_init(&analytics_data.spinlock);
}
