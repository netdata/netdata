// SPDX-License-Identifier: GPL-3.0-or-later

#include "api_v1_calls.h"

#ifndef ENABLE_DBENGINE
int web_client_api_request_v1_dbengine_stats(RRDHOST *host __maybe_unused, struct web_client *w __maybe_unused, char *url __maybe_unused) {
    return HTTP_RESP_NOT_FOUND;
}
#else
static void web_client_api_v1_dbengine_stats_for_tier(BUFFER *wb, size_t tier) {
    RRDENG_SIZE_STATS stats = rrdeng_size_statistics(multidb_ctx[tier]);

    buffer_sprintf(wb,
                   "\n\t\t\"default_granularity_secs\":%zu"
                   ",\n\t\t\"sizeof_datafile\":%zu"
                   ",\n\t\t\"sizeof_page_in_cache\":%zu"
                   ",\n\t\t\"sizeof_point_data\":%zu"
                   ",\n\t\t\"sizeof_page_data\":%zu"
                   ",\n\t\t\"pages_per_extent\":%zu"
                   ",\n\t\t\"datafiles\":%zu"
                   ",\n\t\t\"extents\":%zu"
                   ",\n\t\t\"extents_pages\":%zu"
                   ",\n\t\t\"points\":%zu"
                   ",\n\t\t\"metrics\":%zu"
                   ",\n\t\t\"metrics_pages\":%zu"
                   ",\n\t\t\"extents_compressed_bytes\":%zu"
                   ",\n\t\t\"pages_uncompressed_bytes\":%zu"
                   ",\n\t\t\"pages_duration_secs\":%lld"
                   ",\n\t\t\"single_point_pages\":%zu"
                   ",\n\t\t\"first_t\":%ld"
                   ",\n\t\t\"last_t\":%ld"
                   ",\n\t\t\"database_retention_secs\":%lld"
                   ",\n\t\t\"average_compression_savings\":%0.2f"
                   ",\n\t\t\"average_point_duration_secs\":%0.2f"
                   ",\n\t\t\"average_metric_retention_secs\":%0.2f"
                   ",\n\t\t\"ephemeral_metrics_per_day_percent\":%0.2f"
                   ",\n\t\t\"average_page_size_bytes\":%0.2f"
                   ",\n\t\t\"estimated_concurrently_collected_metrics\":%zu"
                   ",\n\t\t\"currently_collected_metrics\":%zu"
                   ",\n\t\t\"disk_space\":%zu"
                   ",\n\t\t\"max_disk_space\":%zu"
                   , stats.default_granularity_secs
                   , stats.sizeof_datafile
                   , stats.sizeof_page_in_cache
                   , stats.sizeof_point_data
                   , stats.sizeof_page_data
                   , stats.pages_per_extent
                   , stats.datafiles
                   , stats.extents
                   , stats.extents_pages
                   , stats.points
                   , stats.metrics
                   , stats.metrics_pages
                   , stats.extents_compressed_bytes
                   , stats.pages_uncompressed_bytes
                   , (long long)stats.pages_duration_secs
                   , stats.single_point_pages
                   , stats.first_time_s
                   , stats.last_time_s
                   , (long long)stats.database_retention_secs
                   , stats.average_compression_savings
                   , stats.average_point_duration_secs
                   , stats.average_metric_retention_secs
                   , stats.ephemeral_metrics_per_day_percent
                   , stats.average_page_size_bytes
                   , stats.estimated_concurrently_collected_metrics
                   , stats.currently_collected_metrics
                   , stats.disk_space
                   , stats.max_disk_space
    );
}

int api_v1_dbengine_stats(RRDHOST *host __maybe_unused, struct web_client *w, char *url __maybe_unused) {
    if (!netdata_ready)
        return HTTP_RESP_SERVICE_UNAVAILABLE;

    BUFFER *wb = w->response.data;
    buffer_flush(wb);

    if(!dbengine_enabled) {
        buffer_strcat(wb, "dbengine is not enabled");
        return HTTP_RESP_NOT_FOUND;
    }

    wb->content_type = CT_APPLICATION_JSON;
    buffer_no_cacheable(wb);
    buffer_strcat(wb, "{");
    for(size_t tier = 0; tier < nd_profile.storage_tiers;tier++) {
        buffer_sprintf(wb, "%s\n\t\"tier%zu\": {", tier?",":"", tier);
        web_client_api_v1_dbengine_stats_for_tier(wb, tier);
        buffer_strcat(wb, "\n\t}");
    }
    buffer_strcat(wb, "\n}");

    return HTTP_RESP_OK;
}
#endif
