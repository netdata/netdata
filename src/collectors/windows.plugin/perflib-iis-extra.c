// SPDX-License-Identifier: GPL-3.0-or-later

#include "windows_plugin.h"
#include "windows-internals.h"

/* Additional IIS counters are table-driven so optional Perflib counters stay independent. */
enum iis_extra_scope {
    IIS_EXTRA_SITE,
    IIS_EXTRA_WORKER,
    IIS_EXTRA_QUEUE,
    IIS_EXTRA_GLOBAL,
    IIS_EXTRA_MODE,
};

struct iis_extra_definition {
    const char *key;
    const char *chart;
    const char *context;
    const char *title;
    const char *units;
    const char *dimension;
    const char *label;
    enum iis_extra_scope scope;
    bool incremental;
    RRDSET_TYPE chart_type;
};

struct iis_extra_value {
    COUNTER_DATA first;
    RRDSET *st;
    RRDDIM *rd;
};

#define IIS_EXTRA_MAX 34
#define IIS_EXTRA_ARRAY_SIZE(array) (sizeof(array) / sizeof((array)[0]))
struct iis_extra_instance {
    bool initialized;
    bool seen;
    struct iis_extra_value values[IIS_EXTRA_MAX];
};

struct iis_extra_group {
    DICTIONARY *instances;
    const struct iis_extra_definition *definitions;
    size_t count;
    const char *object;
    const char *chart_prefix;
    const char *module_name;
    int priority_base;
};

#define IIS_EXTRA_DEFINE(key_, chart_, context_, title_, units_, dim_, label_, scope_, incremental_, type_)            \
    {key_, chart_, context_, title_, units_, dim_, label_, scope_, incremental_, type_}
#define IIS_EXTRA(key_, chart_, context_, title_, units_, dim_, label_, scope_, type_)                                 \
    IIS_EXTRA_DEFINE(key_, chart_, context_, title_, units_, dim_, label_, scope_, false, type_)
#define IIS_EXTRA_INCREMENTAL(key_, chart_, context_, title_, units_, dim_, label_, scope_, type_)                     \
    IIS_EXTRA_DEFINE(key_, chart_, context_, title_, units_, dim_, label_, scope_, true, type_)

static const struct iis_extra_definition web_service_definitions[] = {
    IIS_EXTRA(
        "Current Blocked Async I/O Requests",
        "blocked_async_io",
        "iis.website_blocked_async_io_requests",
        "Website blocked async I/O requests",
        "requests",
        "blocked",
        "website",
        IIS_EXTRA_SITE,
        RRDSET_TYPE_LINE),
    IIS_EXTRA(
        "Current CGI Requests",
        "cgi_requests",
        "iis.website_cgi_requests",
        "Website active CGI requests",
        "requests",
        "active",
        "website",
        IIS_EXTRA_SITE,
        RRDSET_TYPE_LINE),
    IIS_EXTRA_INCREMENTAL(
        "Total Anonymous Users",
        "users_total",
        "iis.website_users_total_rate",
        "Website total user counts",
        "users/s",
        "anonymous",
        "website",
        IIS_EXTRA_SITE,
        RRDSET_TYPE_LINE),
    IIS_EXTRA_INCREMENTAL(
        "Total Blocked Async I/O Requests",
        "blocked_async_io_total",
        "iis.website_blocked_async_io_requests_rate",
        "Website total blocked async I/O requests",
        "requests/s",
        "requests",
        "website",
        IIS_EXTRA_SITE,
        RRDSET_TYPE_LINE),
    IIS_EXTRA_INCREMENTAL(
        "Total CGI Requests",
        "cgi_requests_total",
        "iis.website_cgi_requests_rate",
        "Website CGI requests",
        "requests/s",
        "requests",
        "website",
        IIS_EXTRA_SITE,
        RRDSET_TYPE_LINE),
    IIS_EXTRA_INCREMENTAL(
        "Total NonAnonymous Users",
        "users_total",
        "iis.website_users_total_rate",
        "Website total user counts",
        "users/s",
        "non_anonymous",
        "website",
        IIS_EXTRA_SITE,
        RRDSET_TYPE_LINE),
    IIS_EXTRA_INCREMENTAL(
        "Total Rejected Async I/O Requests",
        "rejected_async_io_total",
        "iis.website_rejected_async_io_requests_rate",
        "Website rejected async I/O requests",
        "requests/s",
        "requests",
        "website",
        IIS_EXTRA_SITE,
        RRDSET_TYPE_LINE),
};

static const struct iis_extra_definition http_queue_definitions[] = {
    IIS_EXTRA(
        "CurrentQueueSize",
        "current_queue_size",
        "windows.http_service_queue_current_queue_size",
        "Current HTTP request queue size",
        "requests",
        "queued",
        "queue",
        IIS_EXTRA_QUEUE,
        RRDSET_TYPE_LINE),
    IIS_EXTRA_INCREMENTAL(
        "RejectedRequests",
        "request_rates",
        "windows.http_service_queue_request_rate",
        "HTTP request queue rates",
        "requests/s",
        "rejected",
        "queue",
        IIS_EXTRA_QUEUE,
        RRDSET_TYPE_LINE),
    IIS_EXTRA(
        "MaxQueueItemAge",
        "max_queue_item_age",
        "windows.http_service_queue_max_queue_item_age",
        "Maximum age of an HTTP request in the queue",
        "seconds",
        "age",
        "queue",
        IIS_EXTRA_QUEUE,
        RRDSET_TYPE_LINE),
    IIS_EXTRA(
        "ArrivalRate",
        "request_rates",
        "windows.http_service_queue_request_rate",
        "HTTP request queue rates",
        "requests/s",
        "requests",
        "queue",
        IIS_EXTRA_QUEUE,
        RRDSET_TYPE_LINE),
};

static const struct iis_extra_definition worker_definitions[] = {
    IIS_EXTRA(
        "Maximum Threads Count",
        "maximum_threads",
        "iis.w3svc_w3wp_maximum_threads",
        "Maximum worker threads",
        "threads",
        "threads",
        "app",
        IIS_EXTRA_WORKER,
        RRDSET_TYPE_LINE),
    IIS_EXTRA(
        "Active Flushed Entries",
        "active_flushed_entries",
        "iis.w3svc_w3wp_active_flushed_entries",
        "Worker cache entries pending flush",
        "entries",
        "entries",
        "app",
        IIS_EXTRA_WORKER,
        RRDSET_TYPE_LINE),
    IIS_EXTRA(
        "Maximum File Cache Memory Usage",
        "file_cache_max_memory",
        "iis.w3svc_w3wp_file_cache_max_memory",
        "Maximum worker file cache memory",
        "bytes",
        "used",
        "app",
        IIS_EXTRA_WORKER,
        RRDSET_TYPE_LINE),
    IIS_EXTRA_INCREMENTAL(
        "File Cache Flushes",
        "file_cache_flushes",
        "iis.w3svc_w3wp_file_cache_flushes",
        "Worker file cache flushes",
        "flushes/s",
        "flushes",
        "app",
        IIS_EXTRA_WORKER,
        RRDSET_TYPE_LINE),
    IIS_EXTRA_INCREMENTAL(
        "File Cache Hits",
        "file_cache_queries",
        "iis.w3svc_w3wp_file_cache_queries",
        "Worker file cache hits and misses",
        "queries/s",
        "hits",
        "app",
        IIS_EXTRA_WORKER,
        RRDSET_TYPE_LINE),
    IIS_EXTRA_INCREMENTAL(
        "File Cache Misses",
        "file_cache_queries",
        "iis.w3svc_w3wp_file_cache_queries",
        "Worker file cache hits and misses",
        "queries/s",
        "misses",
        "app",
        IIS_EXTRA_WORKER,
        RRDSET_TYPE_LINE),
    IIS_EXTRA(
        "Current Files Cached",
        "files_cached",
        "iis.w3svc_w3wp_files_cached",
        "Files currently in the worker file cache",
        "files",
        "files",
        "app",
        IIS_EXTRA_WORKER,
        RRDSET_TYPE_LINE),
    IIS_EXTRA_INCREMENTAL(
        "URI Cache Hits",
        "uri_cache_queries",
        "iis.w3svc_w3wp_uri_cache_queries",
        "Worker URI cache hits and misses",
        "queries/s",
        "hits",
        "app",
        IIS_EXTRA_WORKER,
        RRDSET_TYPE_LINE),
    IIS_EXTRA_INCREMENTAL(
        "URI Cache Misses",
        "uri_cache_queries",
        "iis.w3svc_w3wp_uri_cache_queries",
        "Worker URI cache hits and misses",
        "queries/s",
        "misses",
        "app",
        IIS_EXTRA_WORKER,
        RRDSET_TYPE_LINE),
    IIS_EXTRA(
        "Current URIs Cached",
        "uris_cached",
        "iis.w3svc_w3wp_uris_cached",
        "URIs currently in the worker cache",
        "uris",
        "uris",
        "app",
        IIS_EXTRA_WORKER,
        RRDSET_TYPE_LINE),
    IIS_EXTRA_INCREMENTAL(
        "Metadata Cache Hits",
        "metadata_cache_queries",
        "iis.w3svc_w3wp_metadata_cache_queries",
        "Worker metadata cache hits and misses",
        "queries/s",
        "hits",
        "app",
        IIS_EXTRA_WORKER,
        RRDSET_TYPE_LINE),
    IIS_EXTRA_INCREMENTAL(
        "Metadata Cache Misses",
        "metadata_cache_queries",
        "iis.w3svc_w3wp_metadata_cache_queries",
        "Worker metadata cache hits and misses",
        "queries/s",
        "misses",
        "app",
        IIS_EXTRA_WORKER,
        RRDSET_TYPE_LINE),
    IIS_EXTRA(
        "Current Metadata Cached",
        "metadata_cached",
        "iis.w3svc_w3wp_metadata_cached",
        "Metadata currently in the worker cache",
        "blocks",
        "blocks",
        "app",
        IIS_EXTRA_WORKER,
        RRDSET_TYPE_LINE),
    IIS_EXTRA_INCREMENTAL(
        "Metadata Cache Flushes",
        "metadata_cache_flushes",
        "iis.w3svc_w3wp_metadata_cache_flushes",
        "Worker metadata cache flushes",
        "flushes/s",
        "flushes",
        "app",
        IIS_EXTRA_WORKER,
        RRDSET_TYPE_LINE),
    IIS_EXTRA(
        "Output Cache Current Items",
        "output_cache_items",
        "iis.w3svc_w3wp_output_cache_items",
        "Items currently in the worker output cache",
        "items",
        "items",
        "app",
        IIS_EXTRA_WORKER,
        RRDSET_TYPE_LINE),
    IIS_EXTRA_INCREMENTAL(
        "Output Cache Total Hits",
        "output_cache_queries",
        "iis.w3svc_w3wp_output_cache_queries",
        "Worker output cache hits and misses",
        "queries/s",
        "hits",
        "app",
        IIS_EXTRA_WORKER,
        RRDSET_TYPE_LINE),
    IIS_EXTRA_INCREMENTAL(
        "Output Cache Total Misses",
        "output_cache_queries",
        "iis.w3svc_w3wp_output_cache_queries",
        "Worker output cache hits and misses",
        "queries/s",
        "misses",
        "app",
        IIS_EXTRA_WORKER,
        RRDSET_TYPE_LINE),
    IIS_EXTRA_INCREMENTAL(
        "Output Cache Total Flushed Items",
        "output_cache_flushed_items",
        "iis.w3svc_w3wp_output_cache_flushed_items",
        "Worker output cache items flushed",
        "items/s",
        "items",
        "app",
        IIS_EXTRA_WORKER,
        RRDSET_TYPE_LINE),
    IIS_EXTRA_INCREMENTAL(
        "% 401 HTTP Response Sent",
        "http_responses",
        "iis.w3svc_w3wp_http_responses",
        "Worker HTTP responses by status",
        "responses/s",
        "401",
        "app",
        IIS_EXTRA_WORKER,
        RRDSET_TYPE_LINE),
    IIS_EXTRA_INCREMENTAL(
        "% 403 HTTP Response Sent",
        "http_responses",
        "iis.w3svc_w3wp_http_responses",
        "Worker HTTP responses by status",
        "responses/s",
        "403",
        "app",
        IIS_EXTRA_WORKER,
        RRDSET_TYPE_LINE),
    IIS_EXTRA_INCREMENTAL(
        "% 404 HTTP Response Sent",
        "http_responses",
        "iis.w3svc_w3wp_http_responses",
        "Worker HTTP responses by status",
        "responses/s",
        "404",
        "app",
        IIS_EXTRA_WORKER,
        RRDSET_TYPE_LINE),
    IIS_EXTRA_INCREMENTAL(
        "% 500 HTTP Response Sent",
        "http_responses",
        "iis.w3svc_w3wp_http_responses",
        "Worker HTTP responses by status",
        "responses/s",
        "500",
        "app",
        IIS_EXTRA_WORKER,
        RRDSET_TYPE_LINE),
    IIS_EXTRA(
        "WebSocket Active Requests",
        "websocket_active",
        "iis.w3svc_w3wp_websocket_active_requests",
        "Active worker WebSocket requests",
        "requests",
        "active",
        "app",
        IIS_EXTRA_WORKER,
        RRDSET_TYPE_LINE),
    IIS_EXTRA(
        "WebSocket Connection Attempts / Sec",
        "websocket_attempts",
        "iis.w3svc_w3wp_websocket_connection_attempts",
        "Worker WebSocket connection attempts",
        "connections/s",
        "attempts",
        "app",
        IIS_EXTRA_WORKER,
        RRDSET_TYPE_LINE),
    IIS_EXTRA(
        "WebSocket Connections Accepted / Sec",
        "websocket_accepted",
        "iis.w3svc_w3wp_websocket_connections_accepted",
        "Worker WebSocket connections accepted",
        "connections/s",
        "accepted",
        "app",
        IIS_EXTRA_WORKER,
        RRDSET_TYPE_LINE),
    IIS_EXTRA(
        "WebSocket Connections Rejected / Sec",
        "websocket_rejected",
        "iis.w3svc_w3wp_websocket_connections_rejected",
        "Worker WebSocket connections rejected",
        "connections/s",
        "rejected",
        "app",
        IIS_EXTRA_WORKER,
        RRDSET_TYPE_LINE),
};

static const struct iis_extra_definition cache_definitions[] = {
    IIS_EXTRA(
        "Active Flushed Entries",
        "active_flushed_entries",
        "iis.server_cache_active_flushed_entries",
        "IIS server cache entries pending flush",
        "entries",
        "entries",
        NULL,
        IIS_EXTRA_GLOBAL,
        RRDSET_TYPE_LINE),
    IIS_EXTRA(
        "Current File Cache Memory Usage",
        "file_cache_memory",
        "iis.server_file_cache_memory",
        "IIS server file cache memory",
        "bytes",
        "used",
        NULL,
        IIS_EXTRA_GLOBAL,
        RRDSET_TYPE_AREA),
    IIS_EXTRA(
        "Maximum File Cache Memory Usage",
        "file_cache_memory",
        "iis.server_file_cache_memory",
        "IIS server file cache memory",
        "bytes",
        "maximum",
        NULL,
        IIS_EXTRA_GLOBAL,
        RRDSET_TYPE_AREA),
    IIS_EXTRA_INCREMENTAL(
        "File Cache Flushes",
        "file_cache_flushes",
        "iis.server_file_cache_flushes",
        "IIS server file cache flushes",
        "flushes/s",
        "flushes",
        NULL,
        IIS_EXTRA_GLOBAL,
        RRDSET_TYPE_LINE),
    IIS_EXTRA_INCREMENTAL(
        "File Cache Hits",
        "file_cache_queries",
        "iis.server_file_cache_queries",
        "IIS server file cache hits and misses",
        "queries/s",
        "hits",
        NULL,
        IIS_EXTRA_GLOBAL,
        RRDSET_TYPE_LINE),
    IIS_EXTRA_INCREMENTAL(
        "File Cache Misses",
        "file_cache_queries",
        "iis.server_file_cache_queries",
        "IIS server file cache hits and misses",
        "queries/s",
        "misses",
        NULL,
        IIS_EXTRA_GLOBAL,
        RRDSET_TYPE_LINE),
    IIS_EXTRA(
        "Current Files Cached",
        "files_cached",
        "iis.server_files_cached",
        "Files in the IIS server cache",
        "files",
        "files",
        NULL,
        IIS_EXTRA_GLOBAL,
        RRDSET_TYPE_LINE),
    IIS_EXTRA_INCREMENTAL(
        "Total Files Cached",
        "files_cache_rate",
        "iis.server_files_cache_rate",
        "IIS server files added to and removed from cache",
        "files/s",
        "cached",
        NULL,
        IIS_EXTRA_GLOBAL,
        RRDSET_TYPE_LINE),
    IIS_EXTRA_INCREMENTAL(
        "Total Flushed Files",
        "files_cache_rate",
        "iis.server_files_cache_rate",
        "IIS server files added to and removed from cache",
        "files/s",
        "flushed",
        NULL,
        IIS_EXTRA_GLOBAL,
        RRDSET_TYPE_LINE),
    IIS_EXTRA_INCREMENTAL(
        "URI Cache Flushes",
        "uri_flushes",
        "iis.server_uri_cache_flushes",
        "IIS URI cache flushes",
        "flushes/s",
        "user",
        NULL,
        IIS_EXTRA_MODE,
        RRDSET_TYPE_LINE),
    IIS_EXTRA_INCREMENTAL(
        "Kernel: URI Cache Flushes",
        "uri_flushes",
        "iis.server_uri_cache_flushes",
        "IIS URI cache flushes",
        "flushes/s",
        "kernel",
        NULL,
        IIS_EXTRA_MODE,
        RRDSET_TYPE_LINE),
    IIS_EXTRA_INCREMENTAL(
        "URI Cache Hits",
        "uri_queries",
        "iis.server_uri_cache_queries",
        "IIS URI cache hits and misses",
        "queries/s",
        "user_hits",
        NULL,
        IIS_EXTRA_MODE,
        RRDSET_TYPE_LINE),
    IIS_EXTRA_INCREMENTAL(
        "Kernel: URI Cache Hits",
        "uri_queries",
        "iis.server_uri_cache_queries",
        "IIS URI cache hits and misses",
        "queries/s",
        "kernel_hits",
        NULL,
        IIS_EXTRA_MODE,
        RRDSET_TYPE_LINE),
    IIS_EXTRA_INCREMENTAL(
        "URI Cache Misses",
        "uri_queries",
        "iis.server_uri_cache_queries",
        "IIS URI cache hits and misses",
        "queries/s",
        "user_misses",
        NULL,
        IIS_EXTRA_MODE,
        RRDSET_TYPE_LINE),
    IIS_EXTRA_INCREMENTAL(
        "Kernel: URI Cache Misses",
        "uri_queries",
        "iis.server_uri_cache_queries",
        "IIS URI cache hits and misses",
        "queries/s",
        "kernel_misses",
        NULL,
        IIS_EXTRA_MODE,
        RRDSET_TYPE_LINE),
    IIS_EXTRA(
        "Current URIs Cached",
        "uris_cached",
        "iis.server_uris_cached",
        "URIs in the IIS cache",
        "uris",
        "user",
        NULL,
        IIS_EXTRA_MODE,
        RRDSET_TYPE_LINE),
    IIS_EXTRA(
        "Kernel: Current URIs Cached",
        "uris_cached",
        "iis.server_uris_cached",
        "URIs in the IIS cache",
        "uris",
        "kernel",
        NULL,
        IIS_EXTRA_MODE,
        RRDSET_TYPE_LINE),
    IIS_EXTRA_INCREMENTAL(
        "Total URIs Cached",
        "uris_cached_total",
        "iis.server_uris_cached_rate",
        "IIS URI cache additions",
        "uris/s",
        "user",
        NULL,
        IIS_EXTRA_MODE,
        RRDSET_TYPE_LINE),
    IIS_EXTRA_INCREMENTAL(
        "Kernel: Total URIs Cached",
        "uris_cached_total",
        "iis.server_uris_cached_rate",
        "IIS URI cache additions",
        "uris/s",
        "kernel",
        NULL,
        IIS_EXTRA_MODE,
        RRDSET_TYPE_LINE),
    IIS_EXTRA_INCREMENTAL(
        "Total Flushed URIs",
        "uris_flushed",
        "iis.server_uris_flushed_rate",
        "IIS URI cache removals",
        "uris/s",
        "user",
        NULL,
        IIS_EXTRA_MODE,
        RRDSET_TYPE_LINE),
    IIS_EXTRA_INCREMENTAL(
        "Kernel: Total Flushed URIs",
        "uris_flushed",
        "iis.server_uris_flushed_rate",
        "IIS URI cache removals",
        "uris/s",
        "kernel",
        NULL,
        IIS_EXTRA_MODE,
        RRDSET_TYPE_LINE),
    IIS_EXTRA(
        "Current Metadata Cached",
        "metadata_cached",
        "iis.server_metadata_cached",
        "Metadata blocks in the IIS cache",
        "blocks",
        "blocks",
        NULL,
        IIS_EXTRA_GLOBAL,
        RRDSET_TYPE_LINE),
    IIS_EXTRA_INCREMENTAL(
        "Metadata Cache Flushes",
        "metadata_flushes",
        "iis.server_metadata_flushes",
        "IIS metadata cache flushes",
        "flushes/s",
        "flushes",
        NULL,
        IIS_EXTRA_GLOBAL,
        RRDSET_TYPE_LINE),
    IIS_EXTRA_INCREMENTAL(
        "Metadata Cache Hits",
        "metadata_queries",
        "iis.server_metadata_queries",
        "IIS metadata cache hits and misses",
        "queries/s",
        "hits",
        NULL,
        IIS_EXTRA_GLOBAL,
        RRDSET_TYPE_LINE),
    IIS_EXTRA_INCREMENTAL(
        "Metadata Cache Misses",
        "metadata_queries",
        "iis.server_metadata_queries",
        "IIS metadata cache hits and misses",
        "queries/s",
        "misses",
        NULL,
        IIS_EXTRA_GLOBAL,
        RRDSET_TYPE_LINE),
    IIS_EXTRA_INCREMENTAL(
        "Total Metadata Cached",
        "metadata_cache_rate",
        "iis.server_metadata_cache_rate",
        "IIS metadata blocks added to and removed from cache",
        "blocks/s",
        "cached",
        NULL,
        IIS_EXTRA_GLOBAL,
        RRDSET_TYPE_LINE),
    IIS_EXTRA_INCREMENTAL(
        "Total Flushed Metadata",
        "metadata_cache_rate",
        "iis.server_metadata_cache_rate",
        "IIS metadata blocks added to and removed from cache",
        "blocks/s",
        "flushed",
        NULL,
        IIS_EXTRA_GLOBAL,
        RRDSET_TYPE_LINE),
    IIS_EXTRA(
        "Output Cache Current Flushed Items",
        "output_items",
        "iis.server_output_cache_items",
        "IIS output cache items",
        "items",
        "flushed",
        NULL,
        IIS_EXTRA_GLOBAL,
        RRDSET_TYPE_LINE),
    IIS_EXTRA(
        "Output Cache Current Items",
        "output_items",
        "iis.server_output_cache_items",
        "IIS output cache items",
        "items",
        "current",
        NULL,
        IIS_EXTRA_GLOBAL,
        RRDSET_TYPE_LINE),
    IIS_EXTRA(
        "Output Cache Current Memory Usage",
        "output_memory",
        "iis.server_output_cache_memory",
        "IIS output cache memory",
        "bytes",
        "used",
        NULL,
        IIS_EXTRA_GLOBAL,
        RRDSET_TYPE_AREA),
    IIS_EXTRA_INCREMENTAL(
        "Output Cache Total Hits",
        "output_queries",
        "iis.server_output_cache_queries",
        "IIS output cache hits and misses",
        "queries/s",
        "hits",
        NULL,
        IIS_EXTRA_GLOBAL,
        RRDSET_TYPE_LINE),
    IIS_EXTRA_INCREMENTAL(
        "Output Cache Total Misses",
        "output_queries",
        "iis.server_output_cache_queries",
        "IIS output cache hits and misses",
        "queries/s",
        "misses",
        NULL,
        IIS_EXTRA_GLOBAL,
        RRDSET_TYPE_LINE),
    IIS_EXTRA_INCREMENTAL(
        "Output Cache Total Flushed Items",
        "output_flushed",
        "iis.server_output_cache_flushed",
        "IIS output cache items flushed",
        "items/s",
        "items",
        NULL,
        IIS_EXTRA_GLOBAL,
        RRDSET_TYPE_LINE),
    IIS_EXTRA_INCREMENTAL(
        "Output Cache Total Flushes",
        "output_flushes",
        "iis.server_output_cache_flushes",
        "IIS output cache flushes",
        "flushes/s",
        "flushes",
        NULL,
        IIS_EXTRA_GLOBAL,
        RRDSET_TYPE_LINE),
};

_Static_assert(
    IIS_EXTRA_MAX >= IIS_EXTRA_ARRAY_SIZE(web_service_definitions),
    "IIS website table exceeds value storage");
_Static_assert(IIS_EXTRA_MAX >= IIS_EXTRA_ARRAY_SIZE(http_queue_definitions), "HTTP queue table exceeds value storage");
_Static_assert(IIS_EXTRA_MAX >= IIS_EXTRA_ARRAY_SIZE(worker_definitions), "IIS worker table exceeds value storage");
_Static_assert(IIS_EXTRA_MAX >= IIS_EXTRA_ARRAY_SIZE(cache_definitions), "IIS cache table exceeds value storage");

static struct iis_extra_group web_service_group = {
    .definitions = web_service_definitions,
    .count = IIS_EXTRA_ARRAY_SIZE(web_service_definitions),
    .object = "Web Service",
    .chart_prefix = "iis_site",
    .priority_base = PRIO_IIS_EXTRA_WEBSITE};
static struct iis_extra_group http_queue_group = {
    .definitions = http_queue_definitions,
    .count = IIS_EXTRA_ARRAY_SIZE(http_queue_definitions),
    .object = "HTTP Service Request Queues",
    .chart_prefix = "http_service_queue",
    .module_name = "PerflibHttpService",
    .priority_base = PRIO_HTTP_SERVICE_QUEUE};
static struct iis_extra_group worker_group = {
    .definitions = worker_definitions,
    .count = IIS_EXTRA_ARRAY_SIZE(worker_definitions),
    .object = "W3SVC_W3WP",
    .chart_prefix = "iis_worker",
    .priority_base = PRIO_IIS_EXTRA_WORKER};
static struct iis_extra_group cache_group = {
    .definitions = cache_definitions,
    .count = IIS_EXTRA_ARRAY_SIZE(cache_definitions),
    .object = "Web Service Cache",
    .chart_prefix = "iis_server_cache",
    .priority_base = PRIO_IIS_EXTRA_CACHE};

static void iis_extra_instance_delete_cb(const DICTIONARY_ITEM *item __maybe_unused, void *value, void *data)
{
    struct iis_extra_instance *state = value;
    struct iis_extra_group *group = data;
    for (size_t n = 0; n < group->count; n++)
        rrdset_is_obsolete___safe_from_collector_thread(state->values[n].st);
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
    const struct iis_extra_definition *definition,
    const char *instance,
    const char *app,
    int update_every)
{
    COUNTER_DATA *counter = &value->first;
    if (!counter->updated)
        return;

    if (unlikely(!value->st)) {
        char id[RRD_ID_LENGTH_MAX + 1];
        char identity[PERFLIB_MAX_NAME_LENGTH];
        strncpyz(identity, app ? app : (instance ? instance : "global"), sizeof(identity) - 1);
        snprintfz(id, sizeof(id), "%s_%s_%s", group->chart_prefix, identity, definition->chart);
        netdata_fix_chart_name(id);
        const char *family;
        if (definition->scope == IIS_EXTRA_QUEUE)
            family = "http queue";
        else if (definition->scope == IIS_EXTRA_WORKER)
            family = "w3svc w3wp";
        else if (definition->scope == IIS_EXTRA_SITE)
            family = strcmp(definition->chart, "users_total") == 0 ? "users" : "requests";
        else
            family = "cache";
        const char *module_name = group->module_name ? group->module_name : "PerflibWebService";
        size_t chart_index = (size_t)(definition - group->definitions);
        for (size_t n = 0; n < chart_index; n++)
            if (strcmp(group->definitions[n].chart, definition->chart) == 0) {
                chart_index = n;
                break;
            }
        int priority = group->priority_base + (int)chart_index;
        value->st = rrdset_create_localhost(
            definition->scope == IIS_EXTRA_QUEUE ? "windows" : "iis",
            id,
            NULL,
            family,
            definition->context,
            definition->title,
            definition->units,
            PLUGIN_WINDOWS_NAME,
            module_name,
            priority,
            update_every,
            definition->chart_type);
        value->rd = definition->incremental ?
                        rrddim_add(value->st, definition->dimension, NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL) :
                        perflib_rrddim_add(value->st, definition->dimension, NULL, 1, 1, counter);
        if (definition->label && (app || instance))
            rrdlabels_add(value->st->rrdlabels, definition->label, app ? app : instance, RRDLABEL_SRC_AUTO);
    }

    perflib_rrddim_set_by_pointer(value->st, value->rd, counter);
}

static void iis_extra_done_charts(struct iis_extra_group *group, struct iis_extra_instance *state)
{
    for (size_t n = 0; n < group->count; n++) {
        RRDSET *st = state->values[n].st;
        if (!st)
            continue;
        bool already_done = false;
        for (size_t previous = 0; previous < n; previous++)
            if (state->values[previous].st == st) {
                already_done = true;
                break;
            }
        if (!already_done)
            rrdset_done(st);
    }
}

static bool do_iis_extra_group(PERF_DATA_BLOCK *data, struct iis_extra_group *group, int update_every)
{
    if (unlikely(!group->instances)) {
        group->instances = dictionary_create_advanced(
            DICT_OPTION_DONT_OVERWRITE_VALUE | DICT_OPTION_FIXED_SIZE, NULL, sizeof(struct iis_extra_instance));
        dictionary_register_delete_callback(group->instances, iis_extra_instance_delete_cb, group);
    }

    iis_extra_mark_all_unseen(group);

    PERF_OBJECT_TYPE *object = perflibFindObjectTypeByName(data, group->object);
    if (!object) {
        iis_extra_remove_unseen(group);
        return false;
    }

    if (group == &cache_group && object->NumInstances <= 0) {
        struct iis_extra_instance *state = dictionary_set(group->instances, "global", NULL, sizeof(*state));
        if (!state->initialized) {
            for (size_t n = 0; n < group->count; n++) {
                state->values[n].first.key = group->definitions[n].key;
            }
            state->initialized = true;
        }
        state->seen = true;
        for (size_t n = 0; n < group->count; n++) {
            struct iis_extra_value *value = &state->values[n];
            perflibGetObjectCounter(data, object, &value->first);
            iis_extra_emit(group, value, &group->definitions[n], NULL, NULL, update_every);
        }
        iis_extra_done_charts(group, state);
        iis_extra_remove_unseen(group);
        return true;
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
        struct iis_extra_instance *state = dictionary_set(group->instances, state_key, NULL, sizeof(*state));
        const bool first_worker_instance = worker && !state->seen;
        if (first_worker_instance)
            for (size_t n = 0; n < group->count; n++) {
                struct iis_extra_value *value = &state->values[n];
                value->first.previous = value->first.current;
                value->first.current = RAW_DATA_EMPTY;
                value->first.updated = false;
            }
        state->seen = true;
        if (!state->initialized) {
            for (size_t n = 0; n < group->count; n++) {
                state->values[n].first.key = group->definitions[n].key;
                if (group == &worker_group && group->definitions[n].key[0] == '%')
                    state->values[n].first.OverwriteCounterType = PERF_COUNTER_RAWCOUNT;
            }
            state->initialized = true;
        }

        for (size_t n = 0; n < group->count; n++) {
            struct iis_extra_value *value = &state->values[n];
            if (worker) {
                COUNTER_DATA first_sample = value->first;
                first_sample.current = RAW_DATA_EMPTY;
                first_sample.previous = RAW_DATA_EMPTY;
                bool first_updated = perflibGetInstanceCounter(data, object, pi, &first_sample);
                value->first.id = first_sample.id;
                value->first.failures = first_sample.failures;
                value->first.backoff = first_sample.backoff;
                if (first_updated) {
                    value->first.current.CounterType = first_sample.current.CounterType;
                    value->first.current.Time = first_sample.current.Time;
                    value->first.current.Frequency = first_sample.current.Frequency;
                    value->first.current.Data += first_sample.current.Data;
                    value->first.updated = true;
                }
            } else
                perflibGetInstanceCounter(data, object, pi, &value->first);
            if (!worker)
                iis_extra_emit(group, value, &group->definitions[n], instance, app[0] ? app : NULL, update_every);
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
            for (size_t n = 0; n < group->count; n++)
                iis_extra_emit(group, &state->values[n], &group->definitions[n], app, app, update_every);
            iis_extra_done_charts(group, state);
        }
        dfe_done(state);
    }
    iis_extra_remove_unseen(group);
    return true;
}

void do_PerflibWebServiceExtraWeb(PERF_DATA_BLOCK *data, int update_every)
{
    do_iis_extra_group(data, &web_service_group, update_every);
}

void do_PerflibWebServiceExtraWorker(PERF_DATA_BLOCK *data, int update_every)
{
    do_iis_extra_group(data, &worker_group, update_every);
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
