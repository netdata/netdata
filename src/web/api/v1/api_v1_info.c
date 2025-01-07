// SPDX-License-Identifier: GPL-3.0-or-later

#include "api_v1_calls.h"

static void host_collectors(RRDHOST *host, BUFFER *wb) {
    buffer_json_member_add_array(wb, "collectors");

    DICTIONARY *dict = dictionary_create(DICT_OPTION_SINGLE_THREADED|DICT_OPTION_DONT_OVERWRITE_VALUE);
    RRDSET *st;
    char name[500];

    time_t now = now_realtime_sec();

    rrdset_foreach_read(st, host) {
        if (!rrdset_is_available_for_viewers(st))
            continue;

        sprintf(name, "%s:%s", rrdset_plugin_name(st), rrdset_module_name(st));

        bool old = 0;
        bool *set = dictionary_set(dict, name, &old, sizeof(bool));
        if(!*set) {
            *set = true;
            st->last_accessed_time_s = now;
            buffer_json_add_array_item_object(wb);
            buffer_json_member_add_string(wb, "plugin", rrdset_plugin_name(st));
            buffer_json_member_add_string(wb, "module", rrdset_module_name(st));
            buffer_json_object_close(wb);
        }
    }
    rrdset_foreach_done(st);
    dictionary_destroy(dict);

    buffer_json_array_close(wb);
}

static inline void web_client_api_request_v1_info_mirrored_hosts_status(BUFFER *wb, RRDHOST *host) {
    buffer_json_add_array_item_object(wb);

    buffer_json_member_add_string(wb, "hostname", rrdhost_hostname(host));
    buffer_json_member_add_int64(wb, "hops", rrdhost_ingestion_hops(host));
    buffer_json_member_add_boolean(wb, "reachable", (host == localhost || !rrdhost_flag_check(host, RRDHOST_FLAG_ORPHAN)));

    buffer_json_member_add_string(wb, "guid", host->machine_guid);
    buffer_json_member_add_uuid(wb, "node_id", host->node_id.uuid);
    CLAIM_ID claim_id = rrdhost_claim_id_get(host);
    buffer_json_member_add_string(wb, "claim_id", claim_id_is_set(claim_id) ? claim_id.str : NULL);

    buffer_json_object_close(wb);
}

static inline void web_client_api_request_v1_info_mirrored_hosts(BUFFER *wb) {
    RRDHOST *host;

    rrd_rdlock();

    buffer_json_member_add_array(wb, "mirrored_hosts");
    rrdhost_foreach_read(host)
        buffer_json_add_array_item_string(wb, rrdhost_hostname(host));
    buffer_json_array_close(wb);

    buffer_json_member_add_array(wb, "mirrored_hosts_status");
    rrdhost_foreach_read(host) {
        if ((host == localhost || !rrdhost_flag_check(host, RRDHOST_FLAG_ORPHAN))) {
            web_client_api_request_v1_info_mirrored_hosts_status(wb, host);
        }
    }
    rrdhost_foreach_read(host) {
        if ((host != localhost && rrdhost_flag_check(host, RRDHOST_FLAG_ORPHAN))) {
            web_client_api_request_v1_info_mirrored_hosts_status(wb, host);
        }
    }
    buffer_json_array_close(wb);

    rrd_rdunlock();
}

static void web_client_api_request_v1_info_summary_alarm_statuses(RRDHOST *host, BUFFER *wb, const char *key) {
    buffer_json_member_add_object(wb, key);

    size_t normal = 0, warning = 0, critical = 0;
    RRDCALC *rc;
    foreach_rrdcalc_in_rrdhost_read(host, rc) {
        if(unlikely(!rc->rrdset || !rc->rrdset->last_collected_time.tv_sec))
            continue;

        switch(rc->status) {
            case RRDCALC_STATUS_WARNING:
                warning++;
                break;
            case RRDCALC_STATUS_CRITICAL:
                critical++;
                break;
            default:
                normal++;
        }
    }
    foreach_rrdcalc_in_rrdhost_done(rc);

    buffer_json_member_add_uint64(wb, "normal", normal);
    buffer_json_member_add_uint64(wb, "warning", warning);
    buffer_json_member_add_uint64(wb, "critical", critical);

    buffer_json_object_close(wb);
}

static int web_client_api_request_v1_info_fill_buffer(RRDHOST *host, BUFFER *wb) {
    buffer_json_initialize(wb, "\"", "\"", 0, true, BUFFER_JSON_OPTIONS_DEFAULT);

    buffer_json_member_add_string(wb, "version", rrdhost_program_version(host));
    buffer_json_member_add_string(wb, "uid", host->machine_guid);

    buffer_json_member_add_uint64(wb, "hosts-available", rrdhost_hosts_available());
    web_client_api_request_v1_info_mirrored_hosts(wb);

    web_client_api_request_v1_info_summary_alarm_statuses(host, wb, "alarms");

    rrdhost_system_info_to_json_v1(wb, host->system_info);

    host_labels2json(host, wb, "host_labels");
    host_functions2json(host, wb);
    host_collectors(host, wb);

    buffer_json_member_add_boolean(wb, "cloud-enabled", true);
    buffer_json_member_add_boolean(wb, "cloud-available", true);
    buffer_json_member_add_boolean(wb, "agent-claimed", is_agent_claimed());
    buffer_json_member_add_boolean(wb, "aclk-available", aclk_online());

    buffer_json_member_add_string(wb, "memory-mode", rrd_memory_mode_name(host->rrd_memory_mode));
#ifdef ENABLE_DBENGINE
    buffer_json_member_add_uint64(wb, "multidb-disk-quota", default_multidb_disk_quota_mb);
    buffer_json_member_add_uint64(wb, "page-cache-size", default_rrdeng_page_cache_mb);
#endif // ENABLE_DBENGINE
    buffer_json_member_add_boolean(wb, "web-enabled", web_server_mode != WEB_SERVER_MODE_NONE);
    buffer_json_member_add_boolean(wb, "stream-enabled", stream_send.enabled);

    buffer_json_member_add_boolean(wb, "stream-compression", stream_sender_has_compression(host));

    buffer_json_member_add_boolean(wb, "https-enabled", true);

    buffer_json_member_add_quoted_string(wb, "buildinfo", analytics_data.netdata_buildinfo);
    buffer_json_member_add_quoted_string(wb, "release-channel", analytics_data.netdata_config_release_channel);
    buffer_json_member_add_quoted_string(wb, "notification-methods", analytics_data.netdata_notification_methods);

    buffer_json_member_add_boolean(wb, "exporting-enabled", analytics_data.exporting_enabled);
    buffer_json_member_add_quoted_string(wb, "exporting-connectors", analytics_data.netdata_exporting_connectors);

    buffer_json_member_add_uint64(wb, "allmetrics-prometheus-used", analytics_data.prometheus_hits);
    buffer_json_member_add_uint64(wb, "allmetrics-shell-used", analytics_data.shell_hits);
    buffer_json_member_add_uint64(wb, "allmetrics-json-used", analytics_data.json_hits);
    buffer_json_member_add_uint64(wb, "dashboard-used", analytics_data.dashboard_hits);

    buffer_json_member_add_uint64(wb, "charts-count", analytics_data.charts_count);
    buffer_json_member_add_uint64(wb, "metrics-count", analytics_data.metrics_count);

#if defined(ENABLE_ML)
    buffer_json_member_add_object(wb, "ml-info");
    ml_host_get_info(host, wb);
    buffer_json_object_close(wb);
#endif

    buffer_json_finalize(wb);
    return 0;
}

int api_v1_info(RRDHOST *host, struct web_client *w, char *url) {
    (void)url;
    if (!netdata_ready) return HTTP_RESP_SERVICE_UNAVAILABLE;
    BUFFER *wb = w->response.data;
    buffer_flush(wb);
    wb->content_type = CT_APPLICATION_JSON;

    web_client_api_request_v1_info_fill_buffer(host, wb);

    buffer_no_cacheable(wb);
    return HTTP_RESP_OK;
}
