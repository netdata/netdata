// SPDX-License-Identifier: GPL-3.0-or-later

#include "windows_plugin.h"
#include "windows-internals.h"

/* Additional IIS counters are table-driven so optional Perflib counters stay independent. */
enum iis_extra_scope {
    IIS_EXTRA_SCOPE_GROUP_DEFAULT = -1,
    IIS_EXTRA_SITE,
    IIS_EXTRA_WORKER,
    IIS_EXTRA_QUEUE,
    IIS_EXTRA_GLOBAL,
    IIS_EXTRA_SCOPE_MODE,
};

enum iis_extra_group_result {
    IIS_EXTRA_GROUP_COLLECTED,
    IIS_EXTRA_GROUP_OBJECT_MISSING,
    IIS_EXTRA_GROUP_EXPIRED,
};

struct iis_extra_dimension_definition {
    const char *key;
    const char *dimension;
    bool incremental;
    bool percentage;
};

struct iis_extra_chart_definition {
    const char *chart;
    const char *context;
    const char *title;
    const char *units;
    size_t priority_offset;
    enum iis_extra_scope scope_override;
    bool area_chart;
    const struct iis_extra_dimension_definition *dimensions;
    size_t dimension_count;
};

struct iis_extra_value {
    COUNTER_DATA first;
    RRDSET *st;
    RRDDIM *rd;
};

#define IIS_EXTRA_PERCENTAGE_PRECISION 10000
#define IIS_EXTRA_ARRAY_SIZE(array) (sizeof(array) / sizeof((array)[0]))
struct iis_extra_instance {
    bool initialized;
    bool seen;
    DICTIONARY *workers;
    struct iis_extra_value values[];
};

struct iis_extra_group {
    DICTIONARY *instances;
    const struct iis_extra_chart_definition *charts;
    size_t chart_count;
    size_t dimension_count;
    size_t instance_size;
    const char *object;
    const char *chart_prefix;
    const char *module_name;
    int priority_base;
    unsigned int missing_object_cycles;
    enum iis_extra_scope scope;
    const char *label;
    RRDSET_TYPE chart_type;
};

static bool
iis_extra_counter_is_incremental(const struct iis_extra_dimension_definition *definition, uint32_t counter_type)
{
    return !definition->percentage && (definition->incremental || perflib_counter_type_is_incremental(counter_type));
}

static void
iis_extra_aggregate_percentage(COUNTER_DATA *aggregate, const COUNTER_DATA *sample, bool *has_previous_sample)
{
    if (!*has_previous_sample) {
        // A fraction needs two observations to calculate interval deltas.
        *has_previous_sample = true;
        return;
    }

    aggregate->current.Data += perflib_counter_delta(
        sample->previous.Data, sample->current.Data, perflib_counter_type_is_32bit(sample->current.CounterType));
    aggregate->current.Time +=
        (LONGLONG)perflib_counter_delta((uint64_t)sample->previous.Time, (uint64_t)sample->current.Time, true);
    aggregate->current.CounterType = sample->current.CounterType;
    aggregate->updated = true;
}

#define IIS_EXTRA_OBJECT_MISSING_CYCLES 12

static const struct iis_extra_dimension_definition web_service_blocked_async_io_dimensions[] = {
    {"Current Blocked Async I/O Requests", "blocked", false, false},
};

static const struct iis_extra_dimension_definition web_service_cgi_requests_dimensions[] = {
    {"Current CGI Requests", "active", false, false},
};

static const struct iis_extra_dimension_definition web_service_users_total_dimensions[] = {
    {"Total Anonymous Users", "anonymous", true, false},
    {"Total NonAnonymous Users", "non_anonymous", true, false},
};

static const struct iis_extra_dimension_definition web_service_blocked_async_io_total_dimensions[] = {
    {"Total Blocked Async I/O Requests", "requests", true, false},
};

static const struct iis_extra_dimension_definition web_service_cgi_requests_total_dimensions[] = {
    {"Total CGI Requests", "requests", true, false},
};

static const struct iis_extra_dimension_definition web_service_rejected_async_io_total_dimensions[] = {
    {"Total Rejected Async I/O Requests", "requests", true, false},
};
static const struct iis_extra_chart_definition web_service_definitions[] = {
    {"blocked_async_io",
     "iis.website_blocked_async_io_requests",
     "Website blocked async I/O requests",
     "requests",
     0,
     IIS_EXTRA_SCOPE_GROUP_DEFAULT,
     false,
     web_service_blocked_async_io_dimensions,
     IIS_EXTRA_ARRAY_SIZE(web_service_blocked_async_io_dimensions)},
    {"cgi_requests",
     "iis.website_cgi_requests",
     "Website active CGI requests",
     "requests",
     1,
     IIS_EXTRA_SCOPE_GROUP_DEFAULT,
     false,
     web_service_cgi_requests_dimensions,
     IIS_EXTRA_ARRAY_SIZE(web_service_cgi_requests_dimensions)},
    {"users_total",
     "iis.website_users_total_rate",
     "Website total user counts",
     "users/s",
     2,
     IIS_EXTRA_SCOPE_GROUP_DEFAULT,
     false,
     web_service_users_total_dimensions,
     IIS_EXTRA_ARRAY_SIZE(web_service_users_total_dimensions)},
    {"blocked_async_io_total",
     "iis.website_blocked_async_io_requests_rate",
     "Website total blocked async I/O requests",
     "requests/s",
     3,
     IIS_EXTRA_SCOPE_GROUP_DEFAULT,
     false,
     web_service_blocked_async_io_total_dimensions,
     IIS_EXTRA_ARRAY_SIZE(web_service_blocked_async_io_total_dimensions)},
    {"cgi_requests_total",
     "iis.website_cgi_requests_rate",
     "Website CGI requests",
     "requests/s",
     4,
     IIS_EXTRA_SCOPE_GROUP_DEFAULT,
     false,
     web_service_cgi_requests_total_dimensions,
     IIS_EXTRA_ARRAY_SIZE(web_service_cgi_requests_total_dimensions)},
    {"rejected_async_io_total",
     "iis.website_rejected_async_io_requests_rate",
     "Website rejected async I/O requests",
     "requests/s",
     6,
     IIS_EXTRA_SCOPE_GROUP_DEFAULT,
     false,
     web_service_rejected_async_io_total_dimensions,
     IIS_EXTRA_ARRAY_SIZE(web_service_rejected_async_io_total_dimensions)},
};

static const struct iis_extra_dimension_definition http_queue_current_queue_size_dimensions[] = {
    {"CurrentQueueSize", "queued", false, false},
};

static const struct iis_extra_dimension_definition http_queue_request_rates_dimensions[] = {
    {"RejectedRequests", "rejected", true, false},
    {"ArrivalRate", "requests", false, false},
};

static const struct iis_extra_dimension_definition http_queue_max_queue_item_age_dimensions[] = {
    {"MaxQueueItemAge", "age", false, false},
};

static const struct iis_extra_chart_definition http_queue_definitions[] = {
    {"current_queue_size",
     "windows.http_service_queue_current_queue_size",
     "Current HTTP request queue size",
     "requests",
     0,
     IIS_EXTRA_SCOPE_GROUP_DEFAULT,
     false,
     http_queue_current_queue_size_dimensions,
     IIS_EXTRA_ARRAY_SIZE(http_queue_current_queue_size_dimensions)},
    {"request_rates",
     "windows.http_service_queue_request_rate",
     "HTTP request queue rates",
     "requests/s",
     1,
     IIS_EXTRA_SCOPE_GROUP_DEFAULT,
     false,
     http_queue_request_rates_dimensions,
     IIS_EXTRA_ARRAY_SIZE(http_queue_request_rates_dimensions)},
    {"max_queue_item_age",
     "windows.http_service_queue_max_queue_item_age",
     "Maximum age of an HTTP request in the queue",
     "seconds",
     2,
     IIS_EXTRA_SCOPE_GROUP_DEFAULT,
     false,
     http_queue_max_queue_item_age_dimensions,
     IIS_EXTRA_ARRAY_SIZE(http_queue_max_queue_item_age_dimensions)},
};

static const struct iis_extra_dimension_definition worker_maximum_threads_dimensions[] = {
    {"Maximum Threads Count", "threads", false, false},
};

static const struct iis_extra_dimension_definition worker_active_flushed_entries_dimensions[] = {
    {"Active Flushed Entries", "entries", false, false},
};

static const struct iis_extra_dimension_definition worker_file_cache_max_memory_dimensions[] = {
    {"Maximum File Cache Memory Usage", "used", false, false},
};

static const struct iis_extra_dimension_definition worker_file_cache_flushes_dimensions[] = {
    {"File Cache Flushes", "flushes", true, false},
};

static const struct iis_extra_dimension_definition worker_file_cache_queries_dimensions[] = {
    {"File Cache Hits", "hits", true, false},
    {"File Cache Misses", "misses", true, false},
};

static const struct iis_extra_dimension_definition worker_files_cached_dimensions[] = {
    {"Current Files Cached", "files", false, false},
};

static const struct iis_extra_dimension_definition worker_uri_cache_queries_dimensions[] = {
    {"URI Cache Hits", "hits", true, false},
    {"URI Cache Misses", "misses", true, false},
};

static const struct iis_extra_dimension_definition worker_uris_cached_dimensions[] = {
    {"Current URIs Cached", "uris", false, false},
};

static const struct iis_extra_dimension_definition worker_metadata_cache_queries_dimensions[] = {
    {"Metadata Cache Hits", "hits", true, false},
    {"Metadata Cache Misses", "misses", true, false},
};

static const struct iis_extra_dimension_definition worker_metadata_cached_dimensions[] = {
    {"Current Metadata Cached", "blocks", false, false},
};

static const struct iis_extra_dimension_definition worker_metadata_cache_flushes_dimensions[] = {
    {"Metadata Cache Flushes", "flushes", true, false},
};

static const struct iis_extra_dimension_definition worker_output_cache_items_dimensions[] = {
    {"Output Cache Current Items", "items", false, false},
};

static const struct iis_extra_dimension_definition worker_output_cache_queries_dimensions[] = {
    {"Output Cache Total Hits", "hits", true, false},
    {"Output Cache Total Misses", "misses", true, false},
};

static const struct iis_extra_dimension_definition worker_output_cache_flushed_items_dimensions[] = {
    {"Output Cache Total Flushed Items", "items", true, false},
};

static const struct iis_extra_dimension_definition worker_http_responses_dimensions[] = {
    {"% 401 HTTP Response Sent", "401", false, true},
    {"% 403 HTTP Response Sent", "403", false, true},
    {"% 404 HTTP Response Sent", "404", false, true},
    {"% 500 HTTP Response Sent", "500", false, true},
};

static const struct iis_extra_dimension_definition worker_websocket_active_dimensions[] = {
    {"WebSocket Active Requests", "active", false, false},
};

static const struct iis_extra_dimension_definition worker_websocket_attempts_dimensions[] = {
    {"WebSocket Connection Attempts / Sec", "attempts", false, false},
};

static const struct iis_extra_dimension_definition worker_websocket_accepted_dimensions[] = {
    {"WebSocket Connections Accepted / Sec", "accepted", false, false},
};

static const struct iis_extra_dimension_definition worker_websocket_rejected_dimensions[] = {
    {"WebSocket Connections Rejected / Sec", "rejected", false, false},
};

static const struct iis_extra_chart_definition worker_definitions[] = {
    {"maximum_threads",
     "iis.w3svc_w3wp_maximum_threads",
     "Maximum worker threads",
     "threads",
     0,
     IIS_EXTRA_SCOPE_GROUP_DEFAULT,
     false,
     worker_maximum_threads_dimensions,
     IIS_EXTRA_ARRAY_SIZE(worker_maximum_threads_dimensions)},
    {"active_flushed_entries",
     "iis.w3svc_w3wp_active_flushed_entries",
     "Worker cache entries pending flush",
     "entries",
     1,
     IIS_EXTRA_SCOPE_GROUP_DEFAULT,
     false,
     worker_active_flushed_entries_dimensions,
     IIS_EXTRA_ARRAY_SIZE(worker_active_flushed_entries_dimensions)},
    {"file_cache_max_memory",
     "iis.w3svc_w3wp_file_cache_max_memory",
     "Maximum worker file cache memory",
     "bytes",
     2,
     IIS_EXTRA_SCOPE_GROUP_DEFAULT,
     false,
     worker_file_cache_max_memory_dimensions,
     IIS_EXTRA_ARRAY_SIZE(worker_file_cache_max_memory_dimensions)},
    {"file_cache_flushes",
     "iis.w3svc_w3wp_file_cache_flushes",
     "Worker file cache flushes",
     "flushes/s",
     3,
     IIS_EXTRA_SCOPE_GROUP_DEFAULT,
     false,
     worker_file_cache_flushes_dimensions,
     IIS_EXTRA_ARRAY_SIZE(worker_file_cache_flushes_dimensions)},
    {"file_cache_queries",
     "iis.w3svc_w3wp_file_cache_queries",
     "Worker file cache hits and misses",
     "queries/s",
     4,
     IIS_EXTRA_SCOPE_GROUP_DEFAULT,
     false,
     worker_file_cache_queries_dimensions,
     IIS_EXTRA_ARRAY_SIZE(worker_file_cache_queries_dimensions)},
    {"files_cached",
     "iis.w3svc_w3wp_files_cached",
     "Files currently in the worker file cache",
     "files",
     6,
     IIS_EXTRA_SCOPE_GROUP_DEFAULT,
     false,
     worker_files_cached_dimensions,
     IIS_EXTRA_ARRAY_SIZE(worker_files_cached_dimensions)},
    {"uri_cache_queries",
     "iis.w3svc_w3wp_uri_cache_queries",
     "Worker URI cache hits and misses",
     "queries/s",
     7,
     IIS_EXTRA_SCOPE_GROUP_DEFAULT,
     false,
     worker_uri_cache_queries_dimensions,
     IIS_EXTRA_ARRAY_SIZE(worker_uri_cache_queries_dimensions)},
    {"uris_cached",
     "iis.w3svc_w3wp_uris_cached",
     "URIs currently in the worker cache",
     "uris",
     9,
     IIS_EXTRA_SCOPE_GROUP_DEFAULT,
     false,
     worker_uris_cached_dimensions,
     IIS_EXTRA_ARRAY_SIZE(worker_uris_cached_dimensions)},
    {"metadata_cache_queries",
     "iis.w3svc_w3wp_metadata_cache_queries",
     "Worker metadata cache hits and misses",
     "queries/s",
     10,
     IIS_EXTRA_SCOPE_GROUP_DEFAULT,
     false,
     worker_metadata_cache_queries_dimensions,
     IIS_EXTRA_ARRAY_SIZE(worker_metadata_cache_queries_dimensions)},
    {"metadata_cached",
     "iis.w3svc_w3wp_metadata_cached",
     "Metadata currently in the worker cache",
     "blocks",
     12,
     IIS_EXTRA_SCOPE_GROUP_DEFAULT,
     false,
     worker_metadata_cached_dimensions,
     IIS_EXTRA_ARRAY_SIZE(worker_metadata_cached_dimensions)},
    {"metadata_cache_flushes",
     "iis.w3svc_w3wp_metadata_cache_flushes",
     "Worker metadata cache flushes",
     "flushes/s",
     13,
     IIS_EXTRA_SCOPE_GROUP_DEFAULT,
     false,
     worker_metadata_cache_flushes_dimensions,
     IIS_EXTRA_ARRAY_SIZE(worker_metadata_cache_flushes_dimensions)},
    {"output_cache_items",
     "iis.w3svc_w3wp_output_cache_items",
     "Items currently in the worker output cache",
     "items",
     14,
     IIS_EXTRA_SCOPE_GROUP_DEFAULT,
     false,
     worker_output_cache_items_dimensions,
     IIS_EXTRA_ARRAY_SIZE(worker_output_cache_items_dimensions)},
    {"output_cache_queries",
     "iis.w3svc_w3wp_output_cache_queries",
     "Worker output cache hits and misses",
     "queries/s",
     15,
     IIS_EXTRA_SCOPE_GROUP_DEFAULT,
     false,
     worker_output_cache_queries_dimensions,
     IIS_EXTRA_ARRAY_SIZE(worker_output_cache_queries_dimensions)},
    {"output_cache_flushed_items",
     "iis.w3svc_w3wp_output_cache_flushed_items",
     "Worker output cache items flushed",
     "items/s",
     17,
     IIS_EXTRA_SCOPE_GROUP_DEFAULT,
     false,
     worker_output_cache_flushed_items_dimensions,
     IIS_EXTRA_ARRAY_SIZE(worker_output_cache_flushed_items_dimensions)},
    {"http_responses",
     "iis.w3svc_w3wp_http_responses",
     "Worker HTTP responses by status",
     "percentage",
     18,
     IIS_EXTRA_SCOPE_GROUP_DEFAULT,
     false,
     worker_http_responses_dimensions,
     IIS_EXTRA_ARRAY_SIZE(worker_http_responses_dimensions)},
    {"websocket_active",
     "iis.w3svc_w3wp_websocket_active_requests",
     "Active worker WebSocket requests",
     "requests",
     22,
     IIS_EXTRA_SCOPE_GROUP_DEFAULT,
     false,
     worker_websocket_active_dimensions,
     IIS_EXTRA_ARRAY_SIZE(worker_websocket_active_dimensions)},
    {"websocket_attempts",
     "iis.w3svc_w3wp_websocket_connection_attempts",
     "Worker WebSocket connection attempts",
     "connections/s",
     23,
     IIS_EXTRA_SCOPE_GROUP_DEFAULT,
     false,
     worker_websocket_attempts_dimensions,
     IIS_EXTRA_ARRAY_SIZE(worker_websocket_attempts_dimensions)},
    {"websocket_accepted",
     "iis.w3svc_w3wp_websocket_connections_accepted",
     "Worker WebSocket connections accepted",
     "connections/s",
     24,
     IIS_EXTRA_SCOPE_GROUP_DEFAULT,
     false,
     worker_websocket_accepted_dimensions,
     IIS_EXTRA_ARRAY_SIZE(worker_websocket_accepted_dimensions)},
    {"websocket_rejected",
     "iis.w3svc_w3wp_websocket_connections_rejected",
     "Worker WebSocket connections rejected",
     "connections/s",
     25,
     IIS_EXTRA_SCOPE_GROUP_DEFAULT,
     false,
     worker_websocket_rejected_dimensions,
     IIS_EXTRA_ARRAY_SIZE(worker_websocket_rejected_dimensions)},
};

static const struct iis_extra_dimension_definition cache_active_flushed_entries_dimensions[] = {
    {"Active Flushed Entries", "entries", false, false},
};

static const struct iis_extra_dimension_definition cache_file_cache_memory_dimensions[] = {
    {"Current File Cache Memory Usage", "used", false, false},
    {"Maximum File Cache Memory Usage", "maximum", false, false},
};

static const struct iis_extra_dimension_definition cache_file_cache_flushes_dimensions[] = {
    {"File Cache Flushes", "flushes", true, false},
};

static const struct iis_extra_dimension_definition cache_file_cache_queries_dimensions[] = {
    {"File Cache Hits", "hits", true, false},
    {"File Cache Misses", "misses", true, false},
};

static const struct iis_extra_dimension_definition cache_files_cached_dimensions[] = {
    {"Current Files Cached", "files", false, false},
};

static const struct iis_extra_dimension_definition cache_files_cache_rate_dimensions[] = {
    {"Total Files Cached", "cached", true, false},
    {"Total Flushed Files", "flushed", true, false},
};

static const struct iis_extra_dimension_definition cache_uri_flushes_dimensions[] = {
    {"URI Cache Flushes", "user", true, false},
    {"Kernel: URI Cache Flushes", "kernel", true, false},
};

static const struct iis_extra_dimension_definition cache_uri_queries_dimensions[] = {
    {"URI Cache Hits", "user_hits", true, false},
    {"Kernel: URI Cache Hits", "kernel_hits", true, false},
    {"URI Cache Misses", "user_misses", true, false},
    {"Kernel: URI Cache Misses", "kernel_misses", true, false},
};

static const struct iis_extra_dimension_definition cache_uris_cached_dimensions[] = {
    {"Current URIs Cached", "user", false, false},
    {"Kernel: Current URIs Cached", "kernel", false, false},
};

static const struct iis_extra_dimension_definition cache_uris_cached_total_dimensions[] = {
    {"Total URIs Cached", "user", true, false},
    {"Kernel: Total URIs Cached", "kernel", true, false},
};

static const struct iis_extra_dimension_definition cache_uris_flushed_dimensions[] = {
    {"Total Flushed URIs", "user", true, false},
    {"Kernel: Total Flushed URIs", "kernel", true, false},
};

static const struct iis_extra_dimension_definition cache_metadata_cached_dimensions[] = {
    {"Current Metadata Cached", "blocks", false, false},
};

static const struct iis_extra_dimension_definition cache_metadata_flushes_dimensions[] = {
    {"Metadata Cache Flushes", "flushes", true, false},
};

static const struct iis_extra_dimension_definition cache_metadata_queries_dimensions[] = {
    {"Metadata Cache Hits", "hits", true, false},
    {"Metadata Cache Misses", "misses", true, false},
};

static const struct iis_extra_dimension_definition cache_metadata_cache_rate_dimensions[] = {
    {"Total Metadata Cached", "cached", true, false},
    {"Total Flushed Metadata", "flushed", true, false},
};

static const struct iis_extra_dimension_definition cache_output_items_dimensions[] = {
    {"Output Cache Current Flushed Items", "flushed", false, false},
    {"Output Cache Current Items", "current", false, false},
};

static const struct iis_extra_dimension_definition cache_output_memory_dimensions[] = {
    {"Output Cache Current Memory Usage", "used", false, false},
};

static const struct iis_extra_dimension_definition cache_output_queries_dimensions[] = {
    {"Output Cache Total Hits", "hits", true, false},
    {"Output Cache Total Misses", "misses", true, false},
};

static const struct iis_extra_dimension_definition cache_output_flushed_dimensions[] = {
    {"Output Cache Total Flushed Items", "items", true, false},
};

static const struct iis_extra_dimension_definition cache_output_flushes_dimensions[] = {
    {"Output Cache Total Flushes", "flushes", true, false},
};

static const struct iis_extra_chart_definition cache_definitions[] = {
    {"active_flushed_entries",
     "iis.server_cache_active_flushed_entries",
     "IIS server cache entries pending flush",
     "entries",
     0,
     IIS_EXTRA_SCOPE_GROUP_DEFAULT,
     false,
     cache_active_flushed_entries_dimensions,
     IIS_EXTRA_ARRAY_SIZE(cache_active_flushed_entries_dimensions)},
    {"file_cache_memory",
     "iis.server_file_cache_memory",
     "IIS server file cache memory",
     "bytes",
     1,
     IIS_EXTRA_SCOPE_GROUP_DEFAULT,
     true,
     cache_file_cache_memory_dimensions,
     IIS_EXTRA_ARRAY_SIZE(cache_file_cache_memory_dimensions)},
    {"file_cache_flushes",
     "iis.server_file_cache_flushes",
     "IIS server file cache flushes",
     "flushes/s",
     3,
     IIS_EXTRA_SCOPE_GROUP_DEFAULT,
     false,
     cache_file_cache_flushes_dimensions,
     IIS_EXTRA_ARRAY_SIZE(cache_file_cache_flushes_dimensions)},
    {"file_cache_queries",
     "iis.server_file_cache_queries",
     "IIS server file cache hits and misses",
     "queries/s",
     4,
     IIS_EXTRA_SCOPE_GROUP_DEFAULT,
     false,
     cache_file_cache_queries_dimensions,
     IIS_EXTRA_ARRAY_SIZE(cache_file_cache_queries_dimensions)},
    {"files_cached",
     "iis.server_files_cached",
     "Files in the IIS server cache",
     "files",
     6,
     IIS_EXTRA_SCOPE_GROUP_DEFAULT,
     false,
     cache_files_cached_dimensions,
     IIS_EXTRA_ARRAY_SIZE(cache_files_cached_dimensions)},
    {"files_cache_rate",
     "iis.server_files_cache_rate",
     "IIS server files added to and removed from cache",
     "files/s",
     7,
     IIS_EXTRA_SCOPE_GROUP_DEFAULT,
     false,
     cache_files_cache_rate_dimensions,
     IIS_EXTRA_ARRAY_SIZE(cache_files_cache_rate_dimensions)},
    {"uri_flushes",
     "iis.server_uri_cache_flushes",
     "IIS URI cache flushes",
     "flushes/s",
     9,
     IIS_EXTRA_SCOPE_MODE,
     false,
     cache_uri_flushes_dimensions,
     IIS_EXTRA_ARRAY_SIZE(cache_uri_flushes_dimensions)},
    {"uri_queries",
     "iis.server_uri_cache_queries",
     "IIS URI cache hits and misses",
     "queries/s",
     11,
     IIS_EXTRA_SCOPE_MODE,
     false,
     cache_uri_queries_dimensions,
     IIS_EXTRA_ARRAY_SIZE(cache_uri_queries_dimensions)},
    {"uris_cached",
     "iis.server_uris_cached",
     "URIs in the IIS cache",
     "uris",
     15,
     IIS_EXTRA_SCOPE_MODE,
     false,
     cache_uris_cached_dimensions,
     IIS_EXTRA_ARRAY_SIZE(cache_uris_cached_dimensions)},
    {"uris_cached_total",
     "iis.server_uris_cached_rate",
     "IIS URI cache additions",
     "uris/s",
     17,
     IIS_EXTRA_SCOPE_MODE,
     false,
     cache_uris_cached_total_dimensions,
     IIS_EXTRA_ARRAY_SIZE(cache_uris_cached_total_dimensions)},
    {"uris_flushed",
     "iis.server_uris_flushed_rate",
     "IIS URI cache removals",
     "uris/s",
     19,
     IIS_EXTRA_SCOPE_MODE,
     false,
     cache_uris_flushed_dimensions,
     IIS_EXTRA_ARRAY_SIZE(cache_uris_flushed_dimensions)},
    {"metadata_cached",
     "iis.server_metadata_cached",
     "Metadata blocks in the IIS cache",
     "blocks",
     21,
     IIS_EXTRA_SCOPE_GROUP_DEFAULT,
     false,
     cache_metadata_cached_dimensions,
     IIS_EXTRA_ARRAY_SIZE(cache_metadata_cached_dimensions)},
    {"metadata_flushes",
     "iis.server_metadata_flushes",
     "IIS metadata cache flushes",
     "flushes/s",
     22,
     IIS_EXTRA_SCOPE_GROUP_DEFAULT,
     false,
     cache_metadata_flushes_dimensions,
     IIS_EXTRA_ARRAY_SIZE(cache_metadata_flushes_dimensions)},
    {"metadata_queries",
     "iis.server_metadata_queries",
     "IIS metadata cache hits and misses",
     "queries/s",
     23,
     IIS_EXTRA_SCOPE_GROUP_DEFAULT,
     false,
     cache_metadata_queries_dimensions,
     IIS_EXTRA_ARRAY_SIZE(cache_metadata_queries_dimensions)},
    {"metadata_cache_rate",
     "iis.server_metadata_cache_rate",
     "IIS metadata blocks added to and removed from cache",
     "blocks/s",
     25,
     IIS_EXTRA_SCOPE_GROUP_DEFAULT,
     false,
     cache_metadata_cache_rate_dimensions,
     IIS_EXTRA_ARRAY_SIZE(cache_metadata_cache_rate_dimensions)},
    {"output_items",
     "iis.server_output_cache_items",
     "IIS output cache items",
     "items",
     27,
     IIS_EXTRA_SCOPE_GROUP_DEFAULT,
     false,
     cache_output_items_dimensions,
     IIS_EXTRA_ARRAY_SIZE(cache_output_items_dimensions)},
    {"output_memory",
     "iis.server_output_cache_memory",
     "IIS output cache memory",
     "bytes",
     29,
     IIS_EXTRA_SCOPE_GROUP_DEFAULT,
     true,
     cache_output_memory_dimensions,
     IIS_EXTRA_ARRAY_SIZE(cache_output_memory_dimensions)},
    {"output_queries",
     "iis.server_output_cache_queries",
     "IIS output cache hits and misses",
     "queries/s",
     30,
     IIS_EXTRA_SCOPE_GROUP_DEFAULT,
     false,
     cache_output_queries_dimensions,
     IIS_EXTRA_ARRAY_SIZE(cache_output_queries_dimensions)},
    {"output_flushed",
     "iis.server_output_cache_flushed",
     "IIS output cache items flushed",
     "items/s",
     32,
     IIS_EXTRA_SCOPE_GROUP_DEFAULT,
     false,
     cache_output_flushed_dimensions,
     IIS_EXTRA_ARRAY_SIZE(cache_output_flushed_dimensions)},
    {"output_flushes",
     "iis.server_output_cache_flushes",
     "IIS output cache flushes",
     "flushes/s",
     33,
     IIS_EXTRA_SCOPE_GROUP_DEFAULT,
     false,
     cache_output_flushes_dimensions,
     IIS_EXTRA_ARRAY_SIZE(cache_output_flushes_dimensions)},
};

static struct iis_extra_group web_service_group = {
    .charts = web_service_definitions,
    .chart_count = IIS_EXTRA_ARRAY_SIZE(web_service_definitions),
    .object = "Web Service",
    .chart_prefix = "iis_site",
    .priority_base = PRIO_IIS_EXTRA_WEBSITE,
    .scope = IIS_EXTRA_SITE,
    .label = "website",
    .chart_type = RRDSET_TYPE_LINE};
static struct iis_extra_group http_queue_group = {
    .charts = http_queue_definitions,
    .chart_count = IIS_EXTRA_ARRAY_SIZE(http_queue_definitions),
    .object = "HTTP Service Request Queues",
    .chart_prefix = "http_service_queue",
    .module_name = "PerflibHttpService",
    .priority_base = PRIO_HTTP_SERVICE_QUEUE,
    .scope = IIS_EXTRA_QUEUE,
    .label = "queue",
    .chart_type = RRDSET_TYPE_LINE};
static struct iis_extra_group worker_group = {
    .charts = worker_definitions,
    .chart_count = IIS_EXTRA_ARRAY_SIZE(worker_definitions),
    .object = "W3SVC_W3WP",
    .chart_prefix = "iis_worker",
    .priority_base = PRIO_IIS_EXTRA_WORKER,
    .scope = IIS_EXTRA_WORKER,
    .label = "app",
    .chart_type = RRDSET_TYPE_LINE};
static struct iis_extra_group cache_group = {
    .charts = cache_definitions,
    .chart_count = IIS_EXTRA_ARRAY_SIZE(cache_definitions),
    .object = "Web Service Cache",
    .chart_prefix = "iis_server_cache",
    .priority_base = PRIO_IIS_EXTRA_CACHE,
    .scope = IIS_EXTRA_GLOBAL,
    .chart_type = RRDSET_TYPE_LINE};

static size_t iis_extra_dimension_count(const struct iis_extra_group *group)
{
    size_t count = 0;
    for (size_t chart_index = 0; chart_index < group->chart_count; chart_index++)
        count += group->charts[chart_index].dimension_count;
    return count;
}

struct iis_extra_iterator {
    size_t chart_index;
    size_t dimension_index;
    size_t flat_index;
};

static void iis_extra_instance_delete_cb(const DICTIONARY_ITEM *item __maybe_unused, void *value, void *data);

static bool iis_extra_iterator_next(
    const struct iis_extra_group *group,
    struct iis_extra_iterator *iterator,
    const struct iis_extra_chart_definition **chart,
    const struct iis_extra_dimension_definition **dimension,
    size_t *flat_index)
{
    while (iterator->chart_index < group->chart_count) {
        const struct iis_extra_chart_definition *current_chart = &group->charts[iterator->chart_index];
        if (iterator->dimension_index >= current_chart->dimension_count) {
            iterator->chart_index++;
            iterator->dimension_index = 0;
            continue;
        }

        *chart = current_chart;
        *dimension = &current_chart->dimensions[iterator->dimension_index++];
        *flat_index = iterator->flat_index++;
        return true;
    }

    return false;
}

static void iis_extra_group_initialize(struct iis_extra_group *group)
{
    group->dimension_count = iis_extra_dimension_count(group);
    group->instance_size = sizeof(struct iis_extra_instance) + group->dimension_count * sizeof(struct iis_extra_value);
    group->instances = dictionary_create_advanced(
        DICT_OPTION_DONT_OVERWRITE_VALUE | DICT_OPTION_FIXED_SIZE, NULL, group->instance_size);
    dictionary_register_delete_callback(group->instances, iis_extra_instance_delete_cb, group);
}

static void iis_extra_instance_delete_cb(const DICTIONARY_ITEM *item __maybe_unused, void *value, void *data)
{
    struct iis_extra_instance *state = value;
    struct iis_extra_group *group = data;
    for (size_t n = 0; n < group->dimension_count; n++)
        rrdset_is_obsolete___safe_from_collector_thread(state->values[n].st);
    dictionary_destroy(state->workers);
}

static void iis_extra_mark_all_unseen(struct iis_extra_group *group)
{
    if (!group->instances)
        return;
    struct iis_extra_instance *state;
    dfe_start_write(group->instances, state) state->seen = false;
    dfe_done(state);
}

static void iis_extra_remove_unseen(struct iis_extra_group *group)
{
    struct iis_extra_instance *state;
    dfe_start_write(group->instances, state)
    {
        if (!state->seen)
            dictionary_del(group->instances, state_dfe.name);
    }
    dfe_done(state);
    dictionary_garbage_collect(group->instances);
}

static void iis_extra_emit(
    struct iis_extra_group *group,
    struct iis_extra_value *value,
    const struct iis_extra_chart_definition *chart,
    const struct iis_extra_dimension_definition *dimension,
    const char *instance,
    const char *app,
    int update_every)
{
    COUNTER_DATA *counter = &value->first;
    if (!counter->updated || (dimension->percentage && counter->current.Time <= 0))
        return;

    if (unlikely(!value->st)) {
        char id[RRD_ID_LENGTH_MAX + 1];
        char identity[PERFLIB_MAX_NAME_LENGTH];
        strncpyz(identity, app ? app : (instance ? instance : "global"), sizeof(identity) - 1);
        snprintfz(id, sizeof(id), "%s_%s_%s", group->chart_prefix, identity, chart->chart);
        netdata_fix_chart_name(id);
        const char *family;
        enum iis_extra_scope scope =
            chart->scope_override == IIS_EXTRA_SCOPE_GROUP_DEFAULT ? group->scope : chart->scope_override;
        if (scope == IIS_EXTRA_QUEUE)
            family = "http queue";
        else if (scope == IIS_EXTRA_WORKER)
            family = "w3svc w3wp";
        else if (scope == IIS_EXTRA_SITE)
            family = strcmp(chart->chart, "users_total") == 0 ? "users" : "requests";
        else
            family = "cache";
        const char *module_name = group->module_name ? group->module_name : "PerflibWebService";
        int priority = group->priority_base + (int)chart->priority_offset;
        value->st = rrdset_create_localhost(
            scope == IIS_EXTRA_QUEUE ? "windows" : "iis",
            id,
            NULL,
            family,
            chart->context,
            chart->title,
            chart->units,
            PLUGIN_WINDOWS_NAME,
            module_name,
            priority,
            update_every,
            chart->area_chart ? RRDSET_TYPE_AREA : group->chart_type);
        if (dimension->percentage)
            value->rd = rrddim_add(
                value->st, dimension->dimension, NULL, 1, IIS_EXTRA_PERCENTAGE_PRECISION, RRD_ALGORITHM_ABSOLUTE);
        else
            value->rd = dimension->incremental ?
                            rrddim_add(value->st, dimension->dimension, NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL) :
                            perflib_rrddim_add(value->st, dimension->dimension, NULL, 1, 1, counter);
        if (group->label && (app || instance))
            rrdlabels_add(value->st->rrdlabels, group->label, app ? app : instance, RRDLABEL_SRC_AUTO);
    }

    if (dimension->percentage) {
        collected_number percentage =
            (collected_number)(100.0 * (double)counter->current.Data / (double)counter->current.Time *
                               IIS_EXTRA_PERCENTAGE_PRECISION);
        rrddim_set_by_pointer(value->st, value->rd, percentage);
    } else
        perflib_rrddim_set_by_pointer(value->st, value->rd, counter);
}

static void iis_extra_done_charts(struct iis_extra_group *group, struct iis_extra_instance *state)
{
    struct iis_extra_iterator iterator = {0};
    const struct iis_extra_chart_definition *chart;
    const struct iis_extra_dimension_definition *dimension;
    size_t flat_index;
    size_t previous_chart_index = SIZE_MAX;
    RRDSET *chart_st = NULL;

    while (iis_extra_iterator_next(group, &iterator, &chart, &dimension, &flat_index)) {
        (void)chart;
        (void)dimension;
        const size_t chart_index = iterator.chart_index;
        if (chart_index != previous_chart_index) {
            if (chart_st)
                rrdset_done(chart_st);
            previous_chart_index = chart_index;
            chart_st = NULL;
        }
        if (!chart_st)
            chart_st = state->values[flat_index].st;
    }
    if (chart_st)
        rrdset_done(chart_st);
}

static enum iis_extra_group_result
do_iis_extra_group(PERF_DATA_BLOCK *data, struct iis_extra_group *group, int update_every)
{
    if (unlikely(!group->instances)) {
        iis_extra_group_initialize(group);
    }

    PERF_OBJECT_TYPE *object = perflibFindObjectTypeByName(data, group->object);
    if (!object) {
        if (++group->missing_object_cycles >= IIS_EXTRA_OBJECT_MISSING_CYCLES) {
            iis_extra_mark_all_unseen(group);
            iis_extra_remove_unseen(group);
            group->missing_object_cycles = 0;
            return IIS_EXTRA_GROUP_EXPIRED;
        }
        return IIS_EXTRA_GROUP_OBJECT_MISSING;
    }
    group->missing_object_cycles = 0;

    iis_extra_mark_all_unseen(group);

    if (group == &cache_group && object->NumInstances <= 0) {
        struct iis_extra_instance *state = dictionary_set(group->instances, "global", NULL, group->instance_size);
        if (!state->initialized) {
            struct iis_extra_iterator iterator = {0};
            const struct iis_extra_chart_definition *chart;
            const struct iis_extra_dimension_definition *dimension;
            size_t n;
            while (iis_extra_iterator_next(group, &iterator, &chart, &dimension, &n))
                state->values[n].first.key = dimension->key;
            state->initialized = true;
        }
        state->seen = true;
        struct iis_extra_iterator iterator = {0};
        const struct iis_extra_chart_definition *chart;
        const struct iis_extra_dimension_definition *dimension;
        size_t n;
        while (iis_extra_iterator_next(group, &iterator, &chart, &dimension, &n)) {
            struct iis_extra_value *value = &state->values[n];
            perflibGetObjectCounter(data, object, &value->first);
            iis_extra_emit(group, value, chart, dimension, NULL, NULL, update_every);
        }
        iis_extra_done_charts(group, state);
        iis_extra_remove_unseen(group);
        return IIS_EXTRA_GROUP_COLLECTED;
    }

    PERF_INSTANCE_DEFINITION *pi = NULL;
    for (LONG i = 0; i < object->NumInstances; i++) {
        pi = perflibForEachInstance(data, object, pi);
        if (!pi)
            break;
        if (!getInstanceName(data, object, pi, windows_shared_buffer, sizeof(windows_shared_buffer)))
            continue;

        char instance_key[PERFLIB_MAX_NAME_LENGTH];
        strncpyz(instance_key, windows_shared_buffer, sizeof(instance_key) - 1);
        char app[PERFLIB_MAX_NAME_LENGTH] = "";
        const char *instance = windows_shared_buffer;
        if (group == &cache_group) {
            if (strcasecmp(instance, "_Total") != 0 && object->NumInstances > 1)
                continue;
            instance = "global";
        } else if (group == &worker_group) {
            char *separator = strchr(windows_shared_buffer, '_');
            if (!separator || separator == windows_shared_buffer || !separator[1])
                continue;
            *separator++ = '\0';
            if (strchr(separator, '#'))
                continue;
            strncpyz(app, separator, sizeof(app) - 1);
            instance = app;
        } else if (strcasecmp(instance, "_Total") == 0 || strncmp(instance, "---", 3) == 0)
            continue;

        const bool worker = group == &worker_group;
        const char *state_key = group == &cache_group ? "global" : (worker ? app : instance_key);
        struct iis_extra_instance *state = dictionary_set(group->instances, state_key, NULL, group->instance_size);
        const bool first_worker_instance = worker && !state->seen;
        if (first_worker_instance) {
            if (!state->workers)
                state->workers = perflib_worker_dictionary_create(group->dimension_count);
            perflib_worker_state_mark_all_unseen(state->workers);
            struct iis_extra_iterator iterator = {0};
            const struct iis_extra_chart_definition *chart;
            const struct iis_extra_dimension_definition *dimension;
            size_t n;
            while (iis_extra_iterator_next(group, &iterator, &chart, &dimension, &n)) {
                struct iis_extra_value *value = &state->values[n];
                value->first.previous = value->first.current;
                if (!iis_extra_counter_is_incremental(dimension, value->first.current.CounterType))
                    value->first.current = RAW_DATA_EMPTY;
                value->first.updated = false;
            }
        }
        state->seen = true;
        if (!state->initialized) {
            struct iis_extra_iterator iterator = {0};
            const struct iis_extra_chart_definition *chart;
            const struct iis_extra_dimension_definition *dimension;
            size_t n;
            while (iis_extra_iterator_next(group, &iterator, &chart, &dimension, &n))
                state->values[n].first.key = dimension->key;
            state->initialized = true;
        }

        PERFLIB_WORKER_STATE *process = NULL;
        bool *has_sample = NULL;
        COUNTER_DATA *process_counters = NULL;
        if (worker) {
            process = perflib_worker_state_get(state->workers, instance_key, group->dimension_count);
            if (!process)
                continue;
            has_sample = perflib_worker_state_has_sample(process);
            process_counters = perflib_worker_state_counters(process);
            if (!process->initialized) {
                struct iis_extra_iterator iterator = {0};
                const struct iis_extra_chart_definition *chart;
                const struct iis_extra_dimension_definition *dimension;
                size_t counter_index;
                while (iis_extra_iterator_next(group, &iterator, &chart, &dimension, &counter_index))
                    process_counters[counter_index].key = dimension->key;
                process->initialized = true;
            }
            process->seen = true;
        }

        struct iis_extra_iterator iterator = {0};
        const struct iis_extra_chart_definition *chart;
        const struct iis_extra_dimension_definition *dimension;
        size_t n;
        while (iis_extra_iterator_next(group, &iterator, &chart, &dimension, &n)) {
            struct iis_extra_value *value = &state->values[n];
            if (worker) {
                COUNTER_DATA *sample = &process_counters[n];
                if (!perflibGetInstanceCounter(data, object, pi, sample)) {
                    has_sample[n] = false;
                    continue;
                }

                if (dimension->percentage)
                    iis_extra_aggregate_percentage(&value->first, sample, &has_sample[n]);
                else {
                    const bool incremental = iis_extra_counter_is_incremental(dimension, sample->current.CounterType);
                    perflib_aggregate_instance_sample(&value->first, sample, &has_sample[n], incremental);
                }
            } else
                perflibGetInstanceCounter(data, object, pi, &value->first);
            if (!worker)
                iis_extra_emit(group, value, chart, dimension, instance, app[0] ? app : NULL, update_every);
        }
        if (!worker)
            iis_extra_done_charts(group, state);
    }
    if (group == &worker_group) {
        struct iis_extra_instance *state;
        dfe_start_write(group->instances, state)
        {
            if (!state->seen)
                continue;
            const char *app = state_dfe.name;
            struct iis_extra_iterator iterator = {0};
            const struct iis_extra_chart_definition *chart;
            const struct iis_extra_dimension_definition *dimension;
            size_t n;
            while (iis_extra_iterator_next(group, &iterator, &chart, &dimension, &n))
                iis_extra_emit(group, &state->values[n], chart, dimension, app, app, update_every);
            iis_extra_done_charts(group, state);

            perflib_worker_state_remove_unseen(state->workers);
        }
        dfe_done(state);
    }
    iis_extra_remove_unseen(group);
    return IIS_EXTRA_GROUP_COLLECTED;
}

void do_PerflibWebServiceExtraWeb(PERF_DATA_BLOCK *data, int update_every)
{
    do_iis_extra_group(data, &web_service_group, update_every);
}

bool do_PerflibWebServiceExtraWorker(PERF_DATA_BLOCK *data, int update_every)
{
    return do_iis_extra_group(data, &worker_group, update_every) == IIS_EXTRA_GROUP_EXPIRED;
}

int do_PerflibHttpService(int update_every, usec_t dt __maybe_unused)
{
    DWORD id = RegistryFindIDByName("HTTP Service Request Queues");
    if (id == PERFLIB_REGISTRY_NAME_NOT_FOUND)
        return -1;

    PERF_DATA_BLOCK *data = perflibGetPerformanceData(id);
    if (!data)
        return 0;

    do_iis_extra_group(data, &http_queue_group, update_every);
    return 0;
}

void do_PerflibWebServiceExtraCache(PERF_DATA_BLOCK *data, int update_every)
{
    do_iis_extra_group(data, &cache_group, update_every);
}
