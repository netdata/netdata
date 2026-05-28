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

#ifdef OS_WINDOWS
static void get_timezone_win_id(char *win_id, DWORD win_size)
{
    win_id[0] = '\0';
    HKEY hKey;
    LSTATUS ret = RegOpenKeyExA(HKEY_LOCAL_MACHINE,
                                "SYSTEM\\CurrentControlSet\\Control\\TimeZoneInformation",
                                0,
                                KEY_READ,
                                &hKey);
    if (ret != ERROR_SUCCESS)
        return;

    DWORD valueType;
    RegQueryValueExA(hKey, "TimeZoneKeyName", NULL, &valueType, (LPBYTE)win_id, &win_size);

    RegCloseKey(hKey);
}

static void get_win_geoiso(char *geo_name, int length) {
    GEOID id = GetUserGeoID(GEOCLASS_NATION);
    int res = GetGeoInfoA(id, GEO_ISO2, geo_name, length, 0);
    if (!res)
        geo_name[0] = '\0';
}

static int map_windows_tz_to_iana(char *out, char *win_id, char *geo_name) {
    if (*win_id == '\0')
        return -1;

    FILE *fp = fopen("c:\\Windows\\Globalization\\Time Zone\\TimezoneMapping.xml", "r");
    if (!fp)
        return -1;

    char buffer[CONFIG_FILE_LINE_MAX + 1];
    char win_id_match[512];
    bool copied = 0;

    snprintfz(win_id_match, sizeof(win_id_match), "\"%s\"", win_id);

    while (fgets(buffer, CONFIG_FILE_LINE_MAX, fp) != NULL) {
        buffer[CONFIG_FILE_LINE_MAX] = '\0';

        char *s = strstr(buffer, win_id_match);
        if (!s) {
            if (!copied)
                continue;
            else // Country codes do not match, but we found the zone
                break;
        }

        //Escape:'  <MapTZ TZID="'
        s = &buffer[15];
        char *end = strchr(s, '"');
        if (!end)
            continue;

        *end = '\0';

        strncpyz(out, s, strlen(s));

        //Escape:" Region="
        char *cmpregion = end+ 10;
        if (!strncmp(cmpregion, geo_name, 2))
            break;

        copied = 1;
    }

    fclose(fp);
    return 0;
}
#endif

// Detect the current IANA timezone name from the system.
// Returns a pointer into the provided buffer, or NULL if detection fails.
const char *detect_system_timezone_name(char *buffer, size_t buffer_size) {
    const char *timezone = NULL;

#ifdef OS_WINDOWS
    char geo_name[128];
    char win_zone[256];
    get_timezone_win_id(win_zone, 256);
    get_win_geoiso(geo_name, 128);
    if (!map_windows_tz_to_iana(buffer, win_zone, geo_name))
        timezone = buffer;
#else
    // read the /etc/localtime symlink first — this is the authoritative source
    // on modern Linux (timedatectl always updates it, but /etc/timezone can lag)
    {
        ssize_t ret = readlink("/etc/localtime", buffer, buffer_size - 1);
        if (ret > 0) {
            buffer[ret] = '\0';

            const char *cmp = "/usr/share/zoneinfo/";
            size_t cmp_len = strlen(cmp);

            char *s = strstr(buffer, cmp);
            if (s && s[cmp_len])
                timezone = &s[cmp_len];
        }
    }

    // fall back to /etc/timezone (Debian/Ubuntu)
    if (!timezone && !read_txt_file("/etc/timezone", buffer, buffer_size)) {
        timezone = buffer;
    }
#endif

    if (timezone && *timezone) {
        // sanitize in-place: keep only alnum, '_', '/', '-', '+'
        char *d = buffer;
        const char *src = timezone;
        const char *end = buffer + buffer_size - 1;
        while (*src && d < end) {
            if (isalnum((uint8_t)*src) || *src == '_' || *src == '/' || *src == '-' || *src == '+')
                *d++ = *src;
            src++;
        }
        *d = '\0';
        timezone = buffer;
    }

    return (timezone && *timezone) ? timezone : NULL;
}

// Set at startup: true when the user explicitly set "timezone" in netdata.conf.
static bool timezone_user_configured = false;

// True when the timezone name came from a proper source (config,
// /etc/localtime, /etc/timezone, TZ env var) rather than from the strftime("%Z")
// fallback which only produces bare abbreviations like "CEST" or "PST".
static bool timezone_is_tzdb_name = false;

static bool timezone_abbrev_normalize(char *dst, size_t dst_size, const char *src);

bool system_timezone_is_user_configured(void) {
    return timezone_user_configured;
}

bool system_timezone_is_tzdb_name(void) {
    return timezone_is_tzdb_name;
}

void get_system_timezone(void)
{
    char buffer[FILENAME_MAX + 1] = "";
    const char *timezone = NULL;

#ifndef OS_WINDOWS
    // avoid flood calls to stat(/etc/localtime)
    // http://stackoverflow.com/questions/4554271/how-to-avoid-excessive-stat-etc-localtime-calls-in-strftime-on-linux
    const char *tz = getenv("TZ");
    if (!tz || !*tz) {
        setenv("TZ", inicfg_get(&netdata_config, CONFIG_SECTION_ENV_VARS, "TZ", ":/etc/localtime"), 1);
        tz = getenv("TZ");
    }

    // use the TZ variable if it's an explicit IANA name (not a path starting with ':')
    if (tz && *tz && *tz != ':') {
        timezone = tz;
        timezone_is_tzdb_name = true;
        netdata_log_info("TIMEZONE: using TZ variable '%s'", timezone);
    }
#endif

    // Detect from system sources (/etc/localtime symlink, /etc/timezone, Windows API)
    if (!timezone) {
        timezone = detect_system_timezone_name(buffer, sizeof(buffer));
        if (timezone) {
            timezone_is_tzdb_name = true;
            netdata_log_info("TIMEZONE: detected '%s'", timezone);
        }
    }

    // Last resort: use strftime %Z (gives abbreviation, not IANA name).
    // timezone_is_tzdb_name stays false so refresh_system_timezone() won't
    // try to resolve it via tzalloc() or the tzfile parser.
    if (!timezone) {
        time_t t;
        struct tm *tmp, tmbuf;

        t = now_realtime_sec();
        tmp = localtime_r(&t, &tmbuf);

        if (tmp != NULL) {
            if (strftime(buffer, FILENAME_MAX, "%Z", tmp) != 0) {
                buffer[FILENAME_MAX] = '\0';
                timezone = buffer;
                netdata_log_info("TIMEZONE: using strftime(): '%s'", timezone);
            }
        }
    }

    if (!timezone || !*timezone)
        timezone = "unknown";

    // Check if the user explicitly set "timezone" in netdata.conf BEFORE
    // inicfg_get auto-populates it with the detected default.
    timezone_user_configured = inicfg_exists(&netdata_config, CONFIG_SECTION_GLOBAL, "timezone");

    char safe_timezone[FILENAME_MAX + 1];
    const char *default_timezone = timezone;

    if (timezone_is_tzdb_name) {
        if (!timezone_name_is_safe_tzdb_path(timezone)) {
            netdata_log_error("TIMEZONE: detected unsafe tzdb timezone '%s', ignoring", timezone);
            default_timezone = "unknown";
        }
    } else if (!timezone_abbrev_normalize(safe_timezone, sizeof(safe_timezone), timezone)) {
        default_timezone = "unknown";
    } else {
        default_timezone = safe_timezone;
    }

    // inicfg_get returns a config-system-owned pointer, stable for the process lifetime
    const char *configured_tz = inicfg_get(&netdata_config, CONFIG_SECTION_GLOBAL, "timezone", default_timezone);

    // Treat "timezone =" (empty value) as not user-configured,
    // and fall back to the auto-detected timezone.
    if (timezone_user_configured && (!configured_tz || !*configured_tz)) {
        timezone_user_configured = false;
        configured_tz = default_timezone;
    }

    // If the user explicitly configured a timezone, treat it as a valid tzdb name
    // (the user is responsible for providing a valid value).
    if (timezone_user_configured)
        timezone_is_tzdb_name = true;

    // Compute abbreviation and UTC offset, then set all three atomically
    refresh_system_timezone(configured_tz, timezone_is_tzdb_name);
}

static inline uint32_t tzif_read_be32(const unsigned char *src) {
    return ((uint32_t)src[0] << 24) |
           ((uint32_t)src[1] << 16) |
           ((uint32_t)src[2] << 8)  |
           (uint32_t)src[3];
}

static inline int64_t tzif_read_be64(const unsigned char *src) {
    return (int64_t)(
        ((uint64_t)src[0] << 56) |
        ((uint64_t)src[1] << 48) |
        ((uint64_t)src[2] << 40) |
        ((uint64_t)src[3] << 32) |
        ((uint64_t)src[4] << 24) |
        ((uint64_t)src[5] << 16) |
        ((uint64_t)src[6] << 8)  |
        (uint64_t)src[7]
    );
}

bool timezone_name_is_safe_tzdb_path(const char *timezone) {
    if (!timezone || !*timezone || *timezone == '/')
        return false;

    for (const char *p = timezone; *p; p++) {
        if (!(isalnum((uint8_t)*p) || *p == '_' || *p == '/' || *p == '-' || *p == '+'))
            return false;
    }

    return true;
}

static bool timezone_abbrev_normalize(char *dst, size_t dst_size, const char *src) {
    if (!dst || dst_size < 2 || !src || !*src)
        return false;

    size_t len = 0;
    char *end = dst + dst_size - 1;

    while (*src && dst < end) {
        if (!(isalnum((uint8_t)*src) || *src == '_' || *src == '+' || *src == '-'))
            return false;

        *dst++ = *src++;
        len++;
    }

    *dst = '\0';
    return len != 0 && *src == '\0';
}

static bool timezone_info_from_tm(struct tm *tmp, char *abbrev, size_t abbrev_size, int32_t *offset) {
    if (!tmp)
        return false;

    int32_t new_offset = 0;
    char offset_str[16];

    if (strftime(abbrev, abbrev_size, "%Z", tmp) == 0)
        strncpyz(abbrev, "UTC", abbrev_size - 1);

    if (strftime(offset_str, sizeof(offset_str), "%z", tmp) != 0) {
        char sign = offset_str[0] == '-' || offset_str[0] == '+' ? offset_str[0] : '+';
        int hours = (isdigit((uint8_t)offset_str[1]) ? (offset_str[1] - '0') : 0) * 10 +
                    (isdigit((uint8_t)offset_str[2]) ? (offset_str[2] - '0') : 0);
        int minutes = (isdigit((uint8_t)offset_str[3]) ? (offset_str[3] - '0') : 0) * 10 +
                      (isdigit((uint8_t)offset_str[4]) ? (offset_str[4] - '0') : 0);

        new_offset = (hours * 3600) + (minutes * 60);
        if (sign == '-')
            new_offset = -new_offset;
    }

    *offset = new_offset;
    return true;
}

static bool current_process_timezone_info(time_t t, char *abbrev, size_t abbrev_size, int32_t *offset) {
    struct tm *tmp, tmbuf;
    tmp = localtime_r(&t, &tmbuf);
    return timezone_info_from_tm(tmp, abbrev, abbrev_size, offset);
}

#ifndef OS_WINDOWS
struct tzif_header {
    char magic[4];
    char version;
    char reserved[15];
    unsigned char ttisgmtcnt[4];
    unsigned char ttisstdcnt[4];
    unsigned char leapcnt[4];
    unsigned char timecnt[4];
    unsigned char typecnt[4];
    unsigned char charcnt[4];
};

struct tzif_type {
    int32_t gmtoff;
    uint8_t isdst;
    uint8_t abbrind;
};

static bool tzif_skip_bytes(FILE *fp, size_t bytes) {
    char buffer[256];
    while (bytes) {
        size_t chunk = bytes > sizeof(buffer) ? sizeof(buffer) : bytes;
        if (fread(buffer, 1, chunk, fp) != chunk)
            return false;
        bytes -= chunk;
    }
    return true;
}

// RFC 8536 sane upper bounds — real tzfiles are far smaller,
// but we allow headroom for unusual / future data.
#define TZIF_MAX_TIMECNT    2048
#define TZIF_MAX_TYPECNT     256
#define TZIF_MAX_CHARCNT    2048
#define TZIF_MAX_LEAPCNT      50
#define TZIF_MAX_ISSTDCNT    256
#define TZIF_MAX_ISGMTCNT   256

static bool tzif_validate_header_counts(uint32_t timecnt, uint32_t typecnt, uint32_t charcnt,
                                        uint32_t leapcnt, uint32_t ttisstdcnt, uint32_t ttisgmtcnt) {
    if (timecnt > TZIF_MAX_TIMECNT || typecnt > TZIF_MAX_TYPECNT ||
        charcnt > TZIF_MAX_CHARCNT || leapcnt > TZIF_MAX_LEAPCNT ||
        ttisstdcnt > TZIF_MAX_ISSTDCNT || ttisgmtcnt > TZIF_MAX_ISGMTCNT)
        return false;

    // ttisstdcnt and ttisgmtcnt must be 0 or equal to typecnt per RFC 8536
    if ((ttisstdcnt != 0 && ttisstdcnt != typecnt) ||
        (ttisgmtcnt != 0 && ttisgmtcnt != typecnt))
        return false;

    return true;
}

static bool tzif_skip_block(FILE *fp, const struct tzif_header *hdr, size_t time_size) {
    uint32_t ttisgmtcnt = tzif_read_be32(hdr->ttisgmtcnt);
    uint32_t ttisstdcnt = tzif_read_be32(hdr->ttisstdcnt);
    uint32_t leapcnt = tzif_read_be32(hdr->leapcnt);
    uint32_t timecnt = tzif_read_be32(hdr->timecnt);
    uint32_t typecnt = tzif_read_be32(hdr->typecnt);
    uint32_t charcnt = tzif_read_be32(hdr->charcnt);

    if (!tzif_validate_header_counts(timecnt, typecnt, charcnt, leapcnt, ttisstdcnt, ttisgmtcnt))
        return false;

    size_t bytes = (size_t)timecnt * time_size +
                   (size_t)timecnt +
                   (size_t)typecnt * 6 +
                   (size_t)charcnt +
                   (size_t)leapcnt * (time_size + 4) +
                   (size_t)ttisstdcnt +
                   (size_t)ttisgmtcnt;

    return tzif_skip_bytes(fp, bytes);
}

static int tzif_default_type_index(const struct tzif_type *types, uint32_t typecnt) {
    if (!types || !typecnt)
        return -1;

    for (uint32_t i = 0; i < typecnt; i++) {
        if (!types[i].isdst)
            return (int)i;
    }

    return 0;
}

static bool timezone_info_from_tzfile(const char *timezone, time_t t, char *abbrev, size_t abbrev_size, int32_t *offset) {
    if (!timezone_name_is_safe_tzdb_path(timezone))
        return false;

    const char *tzdir = getenv("TZDIR");
    if (!tzdir || !*tzdir)
        tzdir = "/usr/share/zoneinfo";

    char path[FILENAME_MAX + 1];
    snprintfz(path, sizeof(path), "%s/%s", tzdir, timezone);

    FILE *fp = fopen(path, "rb");
    if (!fp)
        return false;

    bool ok = false;
    struct tzif_header hdr;

    if (fread(&hdr, 1, sizeof(hdr), fp) != sizeof(hdr))
        goto cleanup;

    if (memcmp(hdr.magic, "TZif", 4) != 0)
        goto cleanup;

    if (hdr.version >= '2') {
        if (!tzif_skip_block(fp, &hdr, 4))
            goto cleanup;

        if (fread(&hdr, 1, sizeof(hdr), fp) != sizeof(hdr))
            goto cleanup;

        if (memcmp(hdr.magic, "TZif", 4) != 0)
            goto cleanup;
    }

    uint32_t timecnt = tzif_read_be32(hdr.timecnt);
    uint32_t typecnt = tzif_read_be32(hdr.typecnt);
    uint32_t charcnt = tzif_read_be32(hdr.charcnt);
    uint32_t leapcnt = tzif_read_be32(hdr.leapcnt);
    uint32_t ttisstdcnt = tzif_read_be32(hdr.ttisstdcnt);
    uint32_t ttisgmtcnt = tzif_read_be32(hdr.ttisgmtcnt);
    size_t time_size = (hdr.version >= '2') ? 8 : 4;

    if (!typecnt || !charcnt)
        goto cleanup;

    if (!tzif_validate_header_counts(timecnt, typecnt, charcnt, leapcnt, ttisstdcnt, ttisgmtcnt))
        goto cleanup;

    int64_t *transition_times = callocz(timecnt ? timecnt : 1, sizeof(*transition_times));
    uint8_t *transition_types = callocz(timecnt ? timecnt : 1, sizeof(*transition_types));
    struct tzif_type *types = callocz(typecnt, sizeof(*types));
    char *abbrs = callocz(charcnt + 1, sizeof(*abbrs));

    unsigned char timebuf[8];
    unsigned char typebuf[6];

    for (uint32_t i = 0; i < timecnt; i++) {
        if (fread(timebuf, 1, time_size, fp) != time_size)
            goto free_and_cleanup;
        transition_times[i] = (time_size == 8) ? tzif_read_be64(timebuf) : (int32_t)tzif_read_be32(timebuf);
    }

    if (timecnt && fread(transition_types, 1, timecnt, fp) != timecnt)
        goto free_and_cleanup;

    // validate all transition type indices before using them
    for (uint32_t i = 0; i < timecnt; i++) {
        if (transition_types[i] >= typecnt)
            goto free_and_cleanup;
    }

    for (uint32_t i = 0; i < typecnt; i++) {
        if (fread(typebuf, 1, sizeof(typebuf), fp) != sizeof(typebuf))
            goto free_and_cleanup;

        types[i].gmtoff = (int32_t)tzif_read_be32(typebuf);
        types[i].isdst = typebuf[4];
        types[i].abbrind = typebuf[5];
    }

    if (fread(abbrs, 1, charcnt, fp) != charcnt)
        goto free_and_cleanup;
    abbrs[charcnt] = '\0';

    int type_index = -1;
    if (timecnt == 0) {
        type_index = tzif_default_type_index(types, typecnt);
    } else {
        for (uint32_t i = 0; i < timecnt; i++) {
            if ((int64_t)t < transition_times[i])
                break;
            type_index = transition_types[i];
        }

        if (type_index < 0)
            type_index = tzif_default_type_index(types, typecnt);
    }

    if (type_index < 0 || (uint32_t)type_index >= typecnt)
        goto free_and_cleanup;

    if (types[type_index].abbrind >= charcnt)
        goto free_and_cleanup;

    const char *tz_abbrev = &abbrs[types[type_index].abbrind];
    if (!*tz_abbrev)
        tz_abbrev = "UTC";

    strncpyz(abbrev, tz_abbrev, abbrev_size - 1);
    *offset = types[type_index].gmtoff;
    ok = true;

free_and_cleanup:
    freez(abbrs);
    freez(types);
    freez(transition_types);
    freez(transition_times);

cleanup:
    fclose(fp);
    return ok;
}
#endif

void refresh_system_timezone(const char *timezone, bool is_tzdb_name) {
    time_t t;
    char abbrev[64];
    char safe_abbrev[64];
    const char *new_abbrev = "UTC";
    int32_t new_offset = 0;

    // Update global flag when we have confirmed tzdb knowledge.
    // When is_tzdb_name is false, preserve the existing global flag — a prior
    // successful detection may have already promoted it to true.
    if (is_tzdb_name)
        timezone_is_tzdb_name = true;

    t = now_realtime_sec();
    bool ok = false;

#if defined(HAVE_TZALLOC) && defined(HAVE_LOCALTIME_RZ) && defined(HAVE_TZFREE)
    if (is_tzdb_name) {
        timezone_t tz = tzalloc(timezone);
        if (tz) {
            struct tm *tmp, tmbuf;
            tmp = localtime_rz(tz, &t, &tmbuf);
            ok = timezone_info_from_tm(tmp, abbrev, sizeof(abbrev), &new_offset);
            tzfree(tz);
        }
    }
#elif !defined(OS_WINDOWS)
    if (is_tzdb_name)
        ok = timezone_info_from_tzfile(timezone, t, abbrev, sizeof(abbrev), &new_offset);
#endif

    if (!ok)
        ok = current_process_timezone_info(t, abbrev, sizeof(abbrev), &new_offset);

    if (ok && timezone_abbrev_normalize(safe_abbrev, sizeof(safe_abbrev), abbrev))
        new_abbrev = safe_abbrev;

    // Atomically update the system timezone triplet
    system_tz_set(timezone, new_abbrev, new_offset);

    // Update localhost if it exists
    if (localhost) {
        if (rrdhost_update_timezone(localhost, timezone, new_abbrev, new_offset)) {
            // Timezone changed — update the two labels directly, persist, and notify.
            if (localhost->rrdlabels) {
                rrdlabels_add(localhost->rrdlabels, "_timezone", timezone, RRDLABEL_SRC_AUTO);
                rrdlabels_add(localhost->rrdlabels, "_abbrev_timezone", new_abbrev, RRDLABEL_SRC_AUTO);
                rrdhost_flag_set(localhost, RRDHOST_FLAG_METADATA_LABELS | RRDHOST_FLAG_METADATA_UPDATE);
                stream_send_host_labels(localhost);
            }
            aclk_queue_node_info(localhost, false);
        }
    }
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
