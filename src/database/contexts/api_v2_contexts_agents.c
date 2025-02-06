// SPDX-License-Identifier: GPL-3.0-or-later

#include "api_v2_contexts.h"
#include "aclk/aclk_capas.h"

void build_info_to_json_object(BUFFER *b);

static time_t round_retention(time_t retention_seconds) {
    if(retention_seconds > 60 * 86400)
        retention_seconds = HOWMANY(retention_seconds, 86400) * 86400;
    else if(retention_seconds > 86400)
        retention_seconds = HOWMANY(retention_seconds, 3600) * 3600;
    else
        retention_seconds = HOWMANY(retention_seconds, 60) * 60;

    return retention_seconds;
}

void buffer_json_agents_v2(BUFFER *wb, struct query_timings *timings, time_t now_s, bool info, bool array) {
    if(!now_s)
        now_s = now_realtime_sec();

    if(array) {
        buffer_json_member_add_array(wb, "agents");
        buffer_json_add_array_item_object(wb);
    }
    else
        buffer_json_member_add_object(wb, "agent");

    buffer_json_member_add_string(wb, "mg", localhost->machine_guid);
    buffer_json_member_add_uuid(wb, "nd", localhost->node_id.uuid);
    buffer_json_member_add_string(wb, "nm", rrdhost_hostname(localhost));
    buffer_json_member_add_time_t(wb, "now", now_s);

    if(array)
        buffer_json_member_add_uint64(wb, "ai", 0);

    if(info) {
        buffer_json_member_add_object(wb, "application");
        build_info_to_json_object(wb);
        buffer_json_object_close(wb); // application

        buffer_json_cloud_status(wb, now_s);

        size_t collected_metrics = 0;
        size_t collected_instances = 0;
        size_t collected_contexts = 0;
        size_t available_metrics = 0;
        size_t available_instances = 0;
        size_t available_contexts = 0;

        buffer_json_member_add_object(wb, "nodes");
        {
            size_t receiving = 0, archived = 0, sending = 0, total = 0;
            RRDHOST *host;
            dfe_start_read(rrdhost_root_index, host) {
                total++;

                available_metrics += __atomic_load_n(&host->rrdctx.metrics_count, __ATOMIC_RELAXED);
                available_instances += __atomic_load_n(&host->rrdctx.instances_count, __ATOMIC_RELAXED);
                available_contexts += __atomic_load_n(&host->rrdctx.contexts_count, __ATOMIC_RELAXED);

                if(rrdhost_flag_check(host, RRDHOST_FLAG_STREAM_SENDER_CONNECTED))
                    sending++;

                if (rrdhost_is_online(host)) {
                    collected_metrics += __atomic_load_n(&host->collected.metrics_count, __ATOMIC_RELAXED);
                    collected_instances += __atomic_load_n(&host->collected.instances_count, __ATOMIC_RELAXED);
                    collected_contexts += __atomic_load_n(&host->collected.contexts_count, __ATOMIC_RELAXED);

                    if(host != localhost)
                        receiving++;
                }
                else
                    archived++;
            }
            dfe_done(host);

            buffer_json_member_add_uint64(wb, "total", total);
            buffer_json_member_add_uint64(wb, "receiving", receiving);
            buffer_json_member_add_uint64(wb, "sending", sending);
            buffer_json_member_add_uint64(wb, "archived", archived);
        }
        buffer_json_object_close(wb); // nodes

        buffer_json_member_add_object(wb, "metrics");
        {
            buffer_json_member_add_uint64(wb, "collected", collected_metrics);
            buffer_json_member_add_uint64(wb, "available", available_metrics);
        }
        buffer_json_object_close(wb);

        buffer_json_member_add_object(wb, "instances");
        {
            buffer_json_member_add_uint64(wb, "collected", collected_instances);
            buffer_json_member_add_uint64(wb, "available", available_instances);
        }
        buffer_json_object_close(wb);

        buffer_json_member_add_object(wb, "contexts");
        {
            buffer_json_member_add_uint64(wb, "collected", collected_contexts);
            buffer_json_member_add_uint64(wb, "available", available_contexts);
        }
        buffer_json_object_close(wb);

        agent_capabilities_to_json(wb, localhost, "capabilities");

        buffer_json_member_add_object(wb, "api");
        {
            buffer_json_member_add_uint64(wb, "version", aclk_get_http_api_version());
            buffer_json_member_add_boolean(wb, "bearer_protection", netdata_is_protected_by_bearer);
        }
        buffer_json_object_close(wb); // api

        buffer_json_member_add_array(wb, "db_size");
        size_t group_seconds;
        for (size_t tier = 0; tier < nd_profile.storage_tiers; tier++) {
            STORAGE_ENGINE *eng = localhost->db[tier].eng;
            if (!eng) continue;

            group_seconds = get_tier_grouping(tier) * localhost->rrd_update_every;
            uint64_t max = storage_engine_disk_space_max(eng->seb, localhost->db[tier].si);
            uint64_t used = storage_engine_disk_space_used(eng->seb, localhost->db[tier].si);
#ifdef ENABLE_DBENGINE
            if (!max && eng->seb == STORAGE_ENGINE_BACKEND_DBENGINE) {
                max = rrdeng_get_directory_free_bytes_space(multidb_ctx[tier]);
                max += used;
            }
#endif
            time_t first_time_s = storage_engine_global_first_time_s(eng->seb, localhost->db[tier].si);
//            size_t currently_collected_metrics = storage_engine_collected_metrics(eng->seb, localhost->db[tier].si);

            NETDATA_DOUBLE percent;
            if (used && max)
                percent = (NETDATA_DOUBLE) used * 100.0 / (NETDATA_DOUBLE) max;
            else
                percent = 0.0;

            buffer_json_add_array_item_object(wb);
            buffer_json_member_add_uint64(wb, "tier", tier);
            char human_duration[128];
            duration_snprintf_time_t(human_duration, sizeof(human_duration), (stime_t)group_seconds);
            buffer_json_member_add_string(wb, "granularity", human_duration);

            buffer_json_member_add_uint64(wb, "metrics", storage_engine_metrics(eng->seb, localhost->db[tier].si));
            buffer_json_member_add_uint64(wb, "samples", storage_engine_samples(eng->seb, localhost->db[tier].si));

            if(used || max) {
                buffer_json_member_add_uint64(wb, "disk_used", used);
                buffer_json_member_add_uint64(wb, "disk_max", max);
                buffer_json_member_add_double(wb, "disk_percent", percent);
            }

            if(first_time_s < now_s) {
                time_t retention = now_s - first_time_s;

                buffer_json_member_add_time_t(wb, "from", first_time_s);
                buffer_json_member_add_time_t(wb, "to", now_s);
                buffer_json_member_add_time_t(wb, "retention", retention);

                duration_snprintf(human_duration, sizeof(human_duration),
                                  round_retention(retention), "s", false);

                buffer_json_member_add_string(wb, "retention_human", human_duration);

                if(used || max) { // we have disk space information
                    time_t time_retention = 0;
#ifdef ENABLE_DBENGINE
                    time_retention = multidb_ctx[tier]->config.max_retention_s;
#endif
                    time_t space_retention = (time_t)((NETDATA_DOUBLE)(now_s - first_time_s) * 100.0 / percent);
                    time_t actual_retention = MIN(space_retention, time_retention ? time_retention : space_retention);

                    duration_snprintf(
                        human_duration, sizeof(human_duration),
                                            (int)time_retention, "s", false);

                    buffer_json_member_add_time_t(wb, "requested_retention", time_retention);
                    buffer_json_member_add_string(wb, "requested_retention_human", human_duration);

                    duration_snprintf(human_duration, sizeof(human_duration),
                                      (int)round_retention(actual_retention), "s", false);

                    buffer_json_member_add_time_t(wb, "expected_retention", actual_retention);
                    buffer_json_member_add_string(wb, "expected_retention_human", human_duration);
                }
            }
            buffer_json_object_close(wb);
        }
        buffer_json_array_close(wb); // db_size
    }

    if(timings)
        buffer_json_query_timings(wb, "timings", timings);

    buffer_json_object_close(wb);

    if(array)
        buffer_json_array_close(wb);
}
