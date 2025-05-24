// SPDX-License-Identifier: GPL-3.0-or-later

#include "api_v2_contexts.h"
#include "aclk/aclk_capas.h"
#include "database/rrd-metadata.h"
#include "database/rrd-retention.h"

void build_info_to_json_object(BUFFER *b);

void buffer_json_agents_v2(BUFFER *wb, struct query_timings *timings, time_t now_s, bool info, bool array, CONTEXTS_OPTIONS options) {
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
    buffer_json_member_add_time_t_formatted(wb, "now", now_s, options & CONTEXTS_OPTION_RFC3339);

    if(array)
        buffer_json_member_add_uint64(wb, "ai", 0);

    if(info) {
        buffer_json_member_add_object(wb, "application");
        build_info_to_json_object(wb);
        buffer_json_object_close(wb); // application

        buffer_json_cloud_status(wb, now_s);

        // Get metrics metadata using our reusable function
        RRDSTATS_METADATA metadata = rrdstats_metadata_collect();

        buffer_json_member_add_object(wb, "nodes");
        {
            buffer_json_member_add_uint64(wb, "total", metadata.nodes.total);
            buffer_json_member_add_uint64(wb, "receiving", metadata.nodes.receiving);
            buffer_json_member_add_uint64(wb, "sending", metadata.nodes.sending);
            buffer_json_member_add_uint64(wb, "archived", metadata.nodes.archived);
        }
        buffer_json_object_close(wb); // nodes

        buffer_json_member_add_object(wb, "metrics");
        {
            buffer_json_member_add_uint64(wb, "collected", metadata.metrics.collected);
            buffer_json_member_add_uint64(wb, "available", metadata.metrics.available);
        }
        buffer_json_object_close(wb);

        buffer_json_member_add_object(wb, "instances");
        {
            buffer_json_member_add_uint64(wb, "collected", metadata.instances.collected);
            buffer_json_member_add_uint64(wb, "available", metadata.instances.available);
        }
        buffer_json_object_close(wb);

        buffer_json_member_add_object(wb, "contexts");
        {
            buffer_json_member_add_uint64(wb, "collected", metadata.contexts.collected);
            buffer_json_member_add_uint64(wb, "available", metadata.contexts.available);
            buffer_json_member_add_uint64(wb, "unique", metadata.contexts.unique);
        }
        buffer_json_object_close(wb);

        agent_capabilities_to_json(wb, localhost, "capabilities");

        buffer_json_member_add_object(wb, "api");
        {
            buffer_json_member_add_uint64(wb, "version", aclk_get_http_api_version());
            buffer_json_member_add_boolean(wb, "bearer_protection", netdata_is_protected_by_bearer);
        }
        buffer_json_object_close(wb); // api

        // Get retention information using our new function
        RRDSTATS_RETENTION retention = rrdstats_retention_collect();

        buffer_json_member_add_array(wb, "db_size");
        for (size_t i = 0; i < retention.storage_tiers; i++) {
            RRD_STORAGE_TIER *tier_info = &retention.tiers[i];
            if (!tier_info->backend || tier_info->tier != i)
                continue;

            buffer_json_add_array_item_object(wb);
            buffer_json_member_add_uint64(wb, "tier", tier_info->tier);
            buffer_json_member_add_string(wb, "granularity", tier_info->granularity_human);
            buffer_json_member_add_uint64(wb, "metrics", tier_info->metrics);
            buffer_json_member_add_uint64(wb, "samples", tier_info->samples);

            if(tier_info->disk_used || tier_info->disk_max) {
                buffer_json_member_add_uint64(wb, "disk_used", tier_info->disk_used);
                buffer_json_member_add_uint64(wb, "disk_max", tier_info->disk_max);
                // Format disk_percent to have only 2 decimal places
                double rounded_percent = floor(tier_info->disk_percent * 100.0 + 0.5) / 100.0;
                buffer_json_member_add_double(wb, "disk_percent", rounded_percent);
            }

            if(tier_info->first_time_s < tier_info->last_time_s) {
                buffer_json_member_add_time_t_formatted(wb, "from", tier_info->first_time_s, options & CONTEXTS_OPTION_RFC3339);
                buffer_json_member_add_time_t_formatted(wb, "to", tier_info->last_time_s, options & CONTEXTS_OPTION_RFC3339);
                buffer_json_member_add_time_t(wb, "retention", tier_info->retention);
                buffer_json_member_add_string(wb, "retention_human", tier_info->retention_human);

                if(tier_info->disk_used || tier_info->disk_max) {
                    buffer_json_member_add_time_t(wb, "requested_retention", tier_info->requested_retention);
                    buffer_json_member_add_string(wb, "requested_retention_human", tier_info->requested_retention_human);
                    buffer_json_member_add_time_t(wb, "expected_retention", tier_info->expected_retention);
                    buffer_json_member_add_string(wb, "expected_retention_human", tier_info->expected_retention_human);
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
