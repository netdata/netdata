// SPDX-License-Identifier: GPL-3.0-or-later

#include "windows_plugin.h"
#include "windows-internals.h"

/* Additional IIS counters are table-driven so optional Perflib counters stay independent. */
enum iis_extra_scope {
    IIS_EXTRA_SITE,
    IIS_EXTRA_APP,
    IIS_EXTRA_WORKER,
    IIS_EXTRA_QUEUE,
    IIS_EXTRA_GLOBAL,
    IIS_EXTRA_MODE,
};

struct iis_extra_definition {
    const char *key;
    const char *key2;
    const char *chart;
    const char *context;
    const char *title;
    const char *units;
    const char *dimension;
    const char *label;
    const char *label2;
    const char *label_value;
    enum iis_extra_scope scope;
    bool incremental;
    RRDSET_TYPE chart_type;
};

struct iis_extra_value {
    COUNTER_DATA first;
    COUNTER_DATA second;
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
};

#define IIS_EXTRA_DEFINE(                                                                                              \
    group_, key_, key2_, chart_, context_, title_, units_, dim_, label_, label2_, value_, scope_, incremental_, type_) \
    {key_, key2_, chart_, context_, title_, units_, dim_, label_, label2_, value_, scope_, incremental_, type_}
#define IIS_EXTRA(group_, key_, key2_, chart_, context_, title_, units_, dim_, label_, label2_, value_, scope_, type_) \
    IIS_EXTRA_DEFINE(                                                                                                  \
        group_, key_, key2_, chart_, context_, title_, units_, dim_, label_, label2_, value_, scope_, false, type_)
#define IIS_EXTRA_INCREMENTAL(                                                                                         \
    group_, key_, key2_, chart_, context_, title_, units_, dim_, label_, label2_, value_, scope_, type_)               \
    IIS_EXTRA_DEFINE(                                                                                                  \
        group_, key_, key2_, chart_, context_, title_, units_, dim_, label_, label2_, value_, scope_, true, type_)

static const struct iis_extra_definition web_service_definitions[] = {
    IIS_EXTRA(
        web_service,
        "Current Blocked Async I/O Requests",
        NULL,
        "blocked_async_io",
        "iis.website_blocked_async_io_requests",
        "Website blocked async I/O requests",
        "requests",
        "blocked",
        "website",
        NULL,
        NULL,
        IIS_EXTRA_SITE,
        RRDSET_TYPE_LINE),
    IIS_EXTRA(
        web_service,
        "Current CGI Requests",
        NULL,
        "cgi_requests",
        "iis.website_cgi_requests",
        "Website active CGI requests",
        "requests",
        "active",
        "website",
        NULL,
        NULL,
        IIS_EXTRA_SITE,
        RRDSET_TYPE_LINE),
    IIS_EXTRA(
        web_service,
        "Total Anonymous Users",
        NULL,
        "anonymous_users_total",
        "iis.website_anonymous_users_total",
        "Website anonymous users",
        "users",
        "users",
        "website",
        NULL,
        NULL,
        IIS_EXTRA_SITE,
        RRDSET_TYPE_LINE),
    IIS_EXTRA(
        web_service,
        "Total Blocked Async I/O Requests",
        NULL,
        "blocked_async_io_total",
        "iis.website_blocked_async_io_requests_total",
        "Website blocked async I/O requests",
        "requests",
        "requests",
        "website",
        NULL,
        NULL,
        IIS_EXTRA_SITE,
        RRDSET_TYPE_LINE),
    IIS_EXTRA(
        web_service,
        "Total CGI Requests",
        NULL,
        "cgi_requests_total",
        "iis.website_cgi_requests_total",
        "Website CGI requests",
        "requests",
        "requests",
        "website",
        NULL,
        NULL,
        IIS_EXTRA_SITE,
        RRDSET_TYPE_LINE),
    IIS_EXTRA(
        web_service,
        "Total NonAnonymous Users",
        NULL,
        "non_anonymous_users_total",
        "iis.website_non_anonymous_users_total",
        "Website non-anonymous users",
        "users",
        "users",
        "website",
        NULL,
        NULL,
        IIS_EXTRA_SITE,
        RRDSET_TYPE_LINE),
    IIS_EXTRA(
        web_service,
        "Total Rejected Async I/O Requests",
        NULL,
        "rejected_async_io_total",
        "iis.website_rejected_async_io_requests_total",
        "Website rejected async I/O requests",
        "requests",
        "requests",
        "website",
        NULL,
        NULL,
        IIS_EXTRA_SITE,
        RRDSET_TYPE_LINE),
    IIS_EXTRA(
        web_service,
        "Total Other Request Methods",
        NULL,
        "requests_other_total",
        "iis.website_requests_total",
        "Website HTTP requests by method",
        "requests",
        "requests",
        "website",
        "method",
        "other",
        IIS_EXTRA_SITE,
        RRDSET_TYPE_LINE),
    IIS_EXTRA(
        web_service,
        "Total Copy Requests",
        NULL,
        "requests_copy_total",
        "iis.website_requests_total",
        "Website HTTP requests by method",
        "requests",
        "requests",
        "website",
        "method",
        "copy",
        IIS_EXTRA_SITE,
        RRDSET_TYPE_LINE),
    IIS_EXTRA(
        web_service,
        "Total Delete Requests",
        NULL,
        "requests_delete_total",
        "iis.website_requests_total",
        "Website HTTP requests by method",
        "requests",
        "requests",
        "website",
        "method",
        "delete",
        IIS_EXTRA_SITE,
        RRDSET_TYPE_LINE),
    IIS_EXTRA(
        web_service,
        "Total Get Requests",
        NULL,
        "requests_get_total",
        "iis.website_requests_total",
        "Website HTTP requests by method",
        "requests",
        "requests",
        "website",
        "method",
        "get",
        IIS_EXTRA_SITE,
        RRDSET_TYPE_LINE),
    IIS_EXTRA(
        web_service,
        "Total Head Requests",
        NULL,
        "requests_head_total",
        "iis.website_requests_total",
        "Website HTTP requests by method",
        "requests",
        "requests",
        "website",
        "method",
        "head",
        IIS_EXTRA_SITE,
        RRDSET_TYPE_LINE),
    IIS_EXTRA(
        web_service,
        "Total Lock Requests",
        NULL,
        "requests_lock_total",
        "iis.website_requests_total",
        "Website HTTP requests by method",
        "requests",
        "requests",
        "website",
        "method",
        "lock",
        IIS_EXTRA_SITE,
        RRDSET_TYPE_LINE),
    IIS_EXTRA(
        web_service,
        "Total Mkcol Requests",
        NULL,
        "requests_mkcol_total",
        "iis.website_requests_total",
        "Website HTTP requests by method",
        "requests",
        "requests",
        "website",
        "method",
        "mkcol",
        IIS_EXTRA_SITE,
        RRDSET_TYPE_LINE),
    IIS_EXTRA(
        web_service,
        "Total Move Requests",
        NULL,
        "requests_move_total",
        "iis.website_requests_total",
        "Website HTTP requests by method",
        "requests",
        "requests",
        "website",
        "method",
        "move",
        IIS_EXTRA_SITE,
        RRDSET_TYPE_LINE),
    IIS_EXTRA(
        web_service,
        "Total Options Requests",
        NULL,
        "requests_options_total",
        "iis.website_requests_total",
        "Website HTTP requests by method",
        "requests",
        "requests",
        "website",
        "method",
        "options",
        IIS_EXTRA_SITE,
        RRDSET_TYPE_LINE),
    IIS_EXTRA(
        web_service,
        "Total Post Requests",
        NULL,
        "requests_post_total",
        "iis.website_requests_total",
        "Website HTTP requests by method",
        "requests",
        "requests",
        "website",
        "method",
        "post",
        IIS_EXTRA_SITE,
        RRDSET_TYPE_LINE),
    IIS_EXTRA(
        web_service,
        "Total Propfind Requests",
        NULL,
        "requests_propfind_total",
        "iis.website_requests_total",
        "Website HTTP requests by method",
        "requests",
        "requests",
        "website",
        "method",
        "propfind",
        IIS_EXTRA_SITE,
        RRDSET_TYPE_LINE),
    IIS_EXTRA(
        web_service,
        "Total Proppatch Requests",
        NULL,
        "requests_proppatch_total",
        "iis.website_requests_total",
        "Website HTTP requests by method",
        "requests",
        "requests",
        "website",
        "method",
        "proppatch",
        IIS_EXTRA_SITE,
        RRDSET_TYPE_LINE),
    IIS_EXTRA(
        web_service,
        "Total Put Requests",
        NULL,
        "requests_put_total",
        "iis.website_requests_total",
        "Website HTTP requests by method",
        "requests",
        "requests",
        "website",
        "method",
        "put",
        IIS_EXTRA_SITE,
        RRDSET_TYPE_LINE),
    IIS_EXTRA(
        web_service,
        "Total Search Requests",
        NULL,
        "requests_search_total",
        "iis.website_requests_total",
        "Website HTTP requests by method",
        "requests",
        "requests",
        "website",
        "method",
        "search",
        IIS_EXTRA_SITE,
        RRDSET_TYPE_LINE),
    IIS_EXTRA(
        web_service,
        "Total Trace Requests",
        NULL,
        "requests_trace_total",
        "iis.website_requests_total",
        "Website HTTP requests by method",
        "requests",
        "requests",
        "website",
        "method",
        "trace",
        IIS_EXTRA_SITE,
        RRDSET_TYPE_LINE),
    IIS_EXTRA(
        web_service,
        "Total Unlock Requests",
        NULL,
        "requests_unlock_total",
        "iis.website_requests_total",
        "Website HTTP requests by method",
        "requests",
        "requests",
        "website",
        "method",
        "unlock",
        IIS_EXTRA_SITE,
        RRDSET_TYPE_LINE),
};

static const struct iis_extra_definition http_queue_definitions[] = {
    IIS_EXTRA(
        http_queue,
        "CurrentQueueSize",
        NULL,
        "current_queue_size",
        "iis.http_request_queues_current_queue_size",
        "Current HTTP request queue size",
        "requests",
        "queued",
        "site",
        NULL,
        NULL,
        IIS_EXTRA_QUEUE,
        RRDSET_TYPE_LINE),
    IIS_EXTRA(
        http_queue,
        "RejectedRequests",
        NULL,
        "rejected_requests",
        "iis.http_request_queues_rejected_requests",
        "Total HTTP requests rejected from the queue",
        "requests",
        "rejected",
        "site",
        NULL,
        NULL,
        IIS_EXTRA_QUEUE,
        RRDSET_TYPE_LINE),
    IIS_EXTRA(
        http_queue,
        "MaxQueueItemAge",
        NULL,
        "max_queue_item_age",
        "iis.http_request_queues_max_queue_item_age",
        "Maximum age of an HTTP request in the queue",
        "seconds",
        "age",
        "site",
        NULL,
        NULL,
        IIS_EXTRA_QUEUE,
        RRDSET_TYPE_LINE),
    IIS_EXTRA(
        http_queue,
        "ArrivalRate",
        NULL,
        "arrival_rate",
        "iis.http_request_queues_arrival_rate",
        "HTTP request queue arrival rate",
        "requests/s",
        "requests",
        "site",
        NULL,
        NULL,
        IIS_EXTRA_QUEUE,
        RRDSET_TYPE_LINE),
};

static const struct iis_extra_definition worker_definitions[] = {
    IIS_EXTRA(
        worker,
        "Maximum Threads Count",
        NULL,
        "maximum_threads",
        "iis.w3svc_w3wp_maximum_threads",
        "Maximum worker threads",
        "threads",
        "threads",
        "app",
        "pid",
        NULL,
        IIS_EXTRA_WORKER,
        RRDSET_TYPE_LINE),
    IIS_EXTRA(
        worker,
        "Active Flushed Entries",
        NULL,
        "active_flushed_entries",
        "iis.w3svc_w3wp_active_flushed_entries",
        "Worker cache entries pending flush",
        "entries",
        "entries",
        "app",
        "pid",
        NULL,
        IIS_EXTRA_WORKER,
        RRDSET_TYPE_LINE),
    IIS_EXTRA(
        worker,
        "Maximum File Cache Memory Usage",
        NULL,
        "file_cache_max_memory",
        "iis.w3svc_w3wp_file_cache_max_memory",
        "Maximum worker file cache memory",
        "bytes",
        "used",
        "app",
        "pid",
        NULL,
        IIS_EXTRA_WORKER,
        RRDSET_TYPE_LINE),
    IIS_EXTRA_INCREMENTAL(
        worker,
        "File Cache Flushes",
        NULL,
        "file_cache_flushes",
        "iis.w3svc_w3wp_file_cache_flushes",
        "Worker file cache flushes",
        "flushes/s",
        "flushes",
        "app",
        "pid",
        NULL,
        IIS_EXTRA_WORKER,
        RRDSET_TYPE_LINE),
    IIS_EXTRA_INCREMENTAL(
        worker,
        "File Cache Hits",
        NULL,
        "file_cache_hits",
        "iis.w3svc_w3wp_file_cache_hits",
        "Worker file cache hits",
        "hits/s",
        "hits",
        "app",
        "pid",
        NULL,
        IIS_EXTRA_WORKER,
        RRDSET_TYPE_LINE),
    IIS_EXTRA(
        worker,
        "Current Files Cached",
        NULL,
        "files_cached",
        "iis.w3svc_w3wp_files_cached",
        "Files currently in the worker file cache",
        "files",
        "files",
        "app",
        "pid",
        NULL,
        IIS_EXTRA_WORKER,
        RRDSET_TYPE_LINE),
    IIS_EXTRA_INCREMENTAL(
        worker,
        "URI Cache Hits",
        NULL,
        "uri_cache_hits",
        "iis.w3svc_w3wp_uri_cache_hits",
        "Worker URI cache hits",
        "hits/s",
        "hits",
        "app",
        "pid",
        NULL,
        IIS_EXTRA_WORKER,
        RRDSET_TYPE_LINE),
    IIS_EXTRA_INCREMENTAL(
        worker,
        "URI Cache Misses",
        NULL,
        "uri_cache_misses",
        "iis.w3svc_w3wp_uri_cache_misses",
        "Worker URI cache misses",
        "misses/s",
        "misses",
        "app",
        "pid",
        NULL,
        IIS_EXTRA_WORKER,
        RRDSET_TYPE_LINE),
    IIS_EXTRA(
        worker,
        "Current URIs Cached",
        NULL,
        "uris_cached",
        "iis.w3svc_w3wp_uris_cached",
        "URIs currently in the worker cache",
        "uris",
        "uris",
        "app",
        "pid",
        NULL,
        IIS_EXTRA_WORKER,
        RRDSET_TYPE_LINE),
    IIS_EXTRA_INCREMENTAL(
        worker,
        "Metadata Cache Hits",
        NULL,
        "metadata_cache_hits",
        "iis.w3svc_w3wp_metadata_cache_hits",
        "Worker metadata cache hits",
        "hits/s",
        "hits",
        "app",
        "pid",
        NULL,
        IIS_EXTRA_WORKER,
        RRDSET_TYPE_LINE),
    IIS_EXTRA_INCREMENTAL(
        worker,
        "Metadata Cache Misses",
        NULL,
        "metadata_cache_misses",
        "iis.w3svc_w3wp_metadata_cache_misses",
        "Worker metadata cache misses",
        "misses/s",
        "misses",
        "app",
        "pid",
        NULL,
        IIS_EXTRA_WORKER,
        RRDSET_TYPE_LINE),
    IIS_EXTRA(
        worker,
        "Current Metadata Cached",
        NULL,
        "metadata_cached",
        "iis.w3svc_w3wp_metadata_cached",
        "Metadata currently in the worker cache",
        "blocks",
        "blocks",
        "app",
        "pid",
        NULL,
        IIS_EXTRA_WORKER,
        RRDSET_TYPE_LINE),
    IIS_EXTRA_INCREMENTAL(
        worker,
        "Metadata Cache Flushes",
        NULL,
        "metadata_cache_flushes",
        "iis.w3svc_w3wp_metadata_cache_flushes",
        "Worker metadata cache flushes",
        "flushes/s",
        "flushes",
        "app",
        "pid",
        NULL,
        IIS_EXTRA_WORKER,
        RRDSET_TYPE_LINE),
    IIS_EXTRA(
        worker,
        "Output Cache Current Items",
        NULL,
        "output_cache_items",
        "iis.w3svc_w3wp_output_cache_items",
        "Items currently in the worker output cache",
        "items",
        "items",
        "app",
        "pid",
        NULL,
        IIS_EXTRA_WORKER,
        RRDSET_TYPE_LINE),
    IIS_EXTRA_INCREMENTAL(
        worker,
        "Output Cache Total Hits",
        NULL,
        "output_cache_hits",
        "iis.w3svc_w3wp_output_cache_hits",
        "Worker output cache hits",
        "hits/s",
        "hits",
        "app",
        "pid",
        NULL,
        IIS_EXTRA_WORKER,
        RRDSET_TYPE_LINE),
    IIS_EXTRA_INCREMENTAL(
        worker,
        "Output Cache Total Misses",
        NULL,
        "output_cache_misses",
        "iis.w3svc_w3wp_output_cache_misses",
        "Worker output cache misses",
        "misses/s",
        "misses",
        "app",
        "pid",
        NULL,
        IIS_EXTRA_WORKER,
        RRDSET_TYPE_LINE),
    IIS_EXTRA_INCREMENTAL(
        worker,
        "Output Cache Total Flushed Items",
        NULL,
        "output_cache_flushed_items",
        "iis.w3svc_w3wp_output_cache_flushed_items",
        "Worker output cache items flushed",
        "items/s",
        "items",
        "app",
        "pid",
        NULL,
        IIS_EXTRA_WORKER,
        RRDSET_TYPE_LINE),
    IIS_EXTRA(
        worker,
        "% 401 HTTP Response Sent",
        NULL,
        "responses_401",
        "iis.w3svc_w3wp_http_responses",
        "Worker HTTP responses by status",
        "responses/s",
        "401",
        "app",
        "pid",
        NULL,
        IIS_EXTRA_WORKER,
        RRDSET_TYPE_LINE),
    IIS_EXTRA(
        worker,
        "% 403 HTTP Response Sent",
        NULL,
        "responses_403",
        "iis.w3svc_w3wp_http_responses",
        "Worker HTTP responses by status",
        "responses/s",
        "403",
        "app",
        "pid",
        NULL,
        IIS_EXTRA_WORKER,
        RRDSET_TYPE_LINE),
    IIS_EXTRA(
        worker,
        "% 404 HTTP Response Sent",
        NULL,
        "responses_404",
        "iis.w3svc_w3wp_http_responses",
        "Worker HTTP responses by status",
        "responses/s",
        "404",
        "app",
        "pid",
        NULL,
        IIS_EXTRA_WORKER,
        RRDSET_TYPE_LINE),
    IIS_EXTRA(
        worker,
        "% 500 HTTP Response Sent",
        NULL,
        "responses_500",
        "iis.w3svc_w3wp_http_responses",
        "Worker HTTP responses by status",
        "responses/s",
        "500",
        "app",
        "pid",
        NULL,
        IIS_EXTRA_WORKER,
        RRDSET_TYPE_LINE),
    IIS_EXTRA(
        worker,
        "WebSocket Active Requests",
        NULL,
        "websocket_active",
        "iis.w3svc_w3wp_websocket_active_requests",
        "Active worker WebSocket requests",
        "requests",
        "active",
        "app",
        "pid",
        NULL,
        IIS_EXTRA_WORKER,
        RRDSET_TYPE_LINE),
    IIS_EXTRA(
        worker,
        "WebSocket Connection Attempts / Sec",
        NULL,
        "websocket_attempts",
        "iis.w3svc_w3wp_websocket_connection_attempts",
        "Worker WebSocket connection attempts",
        "connections/s",
        "attempts",
        "app",
        "pid",
        NULL,
        IIS_EXTRA_WORKER,
        RRDSET_TYPE_LINE),
    IIS_EXTRA(
        worker,
        "WebSocket Connections Accepted / Sec",
        NULL,
        "websocket_accepted",
        "iis.w3svc_w3wp_websocket_connections_accepted",
        "Worker WebSocket connections accepted",
        "connections/s",
        "accepted",
        "app",
        "pid",
        NULL,
        IIS_EXTRA_WORKER,
        RRDSET_TYPE_LINE),
    IIS_EXTRA(
        worker,
        "WebSocket Connections Rejected / Sec",
        NULL,
        "websocket_rejected",
        "iis.w3svc_w3wp_websocket_connections_rejected",
        "Worker WebSocket connections rejected",
        "connections/s",
        "rejected",
        "app",
        "pid",
        NULL,
        IIS_EXTRA_WORKER,
        RRDSET_TYPE_LINE),
};

static const struct iis_extra_definition cache_definitions[] = {
    IIS_EXTRA(
        cache,
        "Active Flushed Entries",
        NULL,
        "active_flushed_entries",
        "iis.server_cache_active_flushed_entries",
        "IIS server cache entries pending flush",
        "entries",
        "entries",
        NULL,
        NULL,
        NULL,
        IIS_EXTRA_GLOBAL,
        RRDSET_TYPE_LINE),
    IIS_EXTRA(
        cache,
        "Current File Cache Memory Usage",
        NULL,
        "file_cache_memory",
        "iis.server_file_cache_memory",
        "IIS server file cache memory",
        "bytes",
        "used",
        NULL,
        NULL,
        NULL,
        IIS_EXTRA_GLOBAL,
        RRDSET_TYPE_AREA),
    IIS_EXTRA(
        cache,
        "Maximum File Cache Memory Usage",
        NULL,
        "file_cache_max_memory",
        "iis.server_file_cache_max_memory",
        "IIS server maximum file cache memory",
        "bytes",
        "maximum",
        NULL,
        NULL,
        NULL,
        IIS_EXTRA_GLOBAL,
        RRDSET_TYPE_LINE),
    IIS_EXTRA_INCREMENTAL(
        cache,
        "File Cache Flushes",
        NULL,
        "file_cache_flushes",
        "iis.server_file_cache_flushes",
        "IIS server file cache flushes",
        "flushes/s",
        "flushes",
        NULL,
        NULL,
        NULL,
        IIS_EXTRA_GLOBAL,
        RRDSET_TYPE_LINE),
    IIS_EXTRA_INCREMENTAL(
        cache,
        "File Cache Hits",
        "File Cache Misses",
        "file_cache_queries",
        "iis.server_file_cache_queries",
        "IIS server file cache queries",
        "queries/s",
        "queries",
        NULL,
        NULL,
        NULL,
        IIS_EXTRA_GLOBAL,
        RRDSET_TYPE_LINE),
    IIS_EXTRA_INCREMENTAL(
        cache,
        "File Cache Hits",
        NULL,
        "file_cache_hits",
        "iis.server_file_cache_hits",
        "IIS server file cache hits",
        "hits/s",
        "hits",
        NULL,
        NULL,
        NULL,
        IIS_EXTRA_GLOBAL,
        RRDSET_TYPE_LINE),
    IIS_EXTRA(
        cache,
        "Current Files Cached",
        NULL,
        "files_cached",
        "iis.server_files_cached",
        "Files in the IIS server cache",
        "files",
        "files",
        NULL,
        NULL,
        NULL,
        IIS_EXTRA_GLOBAL,
        RRDSET_TYPE_LINE),
    IIS_EXTRA_INCREMENTAL(
        cache,
        "Total Files Cached",
        NULL,
        "files_cached_total",
        "iis.server_files_cached_total",
        "Files added to the IIS server cache",
        "files/s",
        "files",
        NULL,
        NULL,
        NULL,
        IIS_EXTRA_GLOBAL,
        RRDSET_TYPE_LINE),
    IIS_EXTRA_INCREMENTAL(
        cache,
        "Total Flushed Files",
        NULL,
        "files_flushed_total",
        "iis.server_files_flushed_total",
        "Files removed from the IIS server cache",
        "files/s",
        "files",
        NULL,
        NULL,
        NULL,
        IIS_EXTRA_GLOBAL,
        RRDSET_TYPE_LINE),
    IIS_EXTRA_INCREMENTAL(
        cache,
        "Total Flushed URIs",
        NULL,
        "uri_flushes_user",
        "iis.server_uri_cache_flushes",
        "IIS URI cache flushes",
        "flushes/s",
        "flushes",
        "mode",
        NULL,
        "user",
        IIS_EXTRA_MODE,
        RRDSET_TYPE_LINE),
    IIS_EXTRA_INCREMENTAL(
        cache,
        "Kernel: Total Flushed URIs",
        NULL,
        "uri_flushes_kernel",
        "iis.server_uri_cache_flushes",
        "IIS URI cache flushes",
        "flushes/s",
        "flushes",
        "mode",
        NULL,
        "kernel",
        IIS_EXTRA_MODE,
        RRDSET_TYPE_LINE),
    IIS_EXTRA_INCREMENTAL(
        cache,
        "URI Cache Hits",
        "URI Cache Misses",
        "uri_queries_user",
        "iis.server_uri_cache_queries",
        "IIS URI cache queries",
        "queries/s",
        "queries",
        "mode",
        NULL,
        "user",
        IIS_EXTRA_MODE,
        RRDSET_TYPE_LINE),
    IIS_EXTRA_INCREMENTAL(
        cache,
        "Kernel: URI Cache Hits",
        "Kernel: URI Cache Misses",
        "uri_queries_kernel",
        "iis.server_uri_cache_queries",
        "IIS URI cache queries",
        "queries/s",
        "queries",
        "mode",
        NULL,
        "kernel",
        IIS_EXTRA_MODE,
        RRDSET_TYPE_LINE),
    IIS_EXTRA_INCREMENTAL(
        cache,
        "URI Cache Hits",
        NULL,
        "uri_hits_user",
        "iis.server_uri_cache_hits",
        "IIS URI cache hits",
        "hits/s",
        "hits",
        "mode",
        NULL,
        "user",
        IIS_EXTRA_MODE,
        RRDSET_TYPE_LINE),
    IIS_EXTRA_INCREMENTAL(
        cache,
        "Kernel: URI Cache Hits",
        NULL,
        "uri_hits_kernel",
        "iis.server_uri_cache_hits",
        "IIS URI cache hits",
        "hits/s",
        "hits",
        "mode",
        NULL,
        "kernel",
        IIS_EXTRA_MODE,
        RRDSET_TYPE_LINE),
    IIS_EXTRA(
        cache,
        "Current URIs Cached",
        NULL,
        "uris_cached_user",
        "iis.server_uris_cached",
        "URIs in the IIS cache",
        "uris",
        "uris",
        "mode",
        NULL,
        "user",
        IIS_EXTRA_MODE,
        RRDSET_TYPE_LINE),
    IIS_EXTRA(
        cache,
        "Kernel: Current URIs Cached",
        NULL,
        "uris_cached_kernel",
        "iis.server_uris_cached",
        "URIs in the IIS cache",
        "uris",
        "uris",
        "mode",
        NULL,
        "kernel",
        IIS_EXTRA_MODE,
        RRDSET_TYPE_LINE),
    IIS_EXTRA_INCREMENTAL(
        cache,
        "Total URIs Cached",
        NULL,
        "uris_cached_total_user",
        "iis.server_uris_cached_total",
        "URIs added to the IIS cache",
        "uris/s",
        "uris",
        "mode",
        NULL,
        "user",
        IIS_EXTRA_MODE,
        RRDSET_TYPE_LINE),
    IIS_EXTRA_INCREMENTAL(
        cache,
        "Kernel: Total URIs Cached",
        NULL,
        "uris_cached_total_kernel",
        "iis.server_uris_cached_total",
        "URIs added to the IIS cache",
        "uris/s",
        "uris",
        "mode",
        NULL,
        "kernel",
        IIS_EXTRA_MODE,
        RRDSET_TYPE_LINE),
    IIS_EXTRA_INCREMENTAL(
        cache,
        "Total Flushed URIs",
        NULL,
        "uris_flushed_user",
        "iis.server_uris_flushed_total",
        "URIs removed from the IIS cache",
        "uris/s",
        "uris",
        "mode",
        NULL,
        "user",
        IIS_EXTRA_MODE,
        RRDSET_TYPE_LINE),
    IIS_EXTRA_INCREMENTAL(
        cache,
        "Kernel: Total Flushed URIs",
        NULL,
        "uris_flushed_kernel",
        "iis.server_uris_flushed_total",
        "URIs removed from the IIS cache",
        "uris/s",
        "uris",
        "mode",
        NULL,
        "kernel",
        IIS_EXTRA_MODE,
        RRDSET_TYPE_LINE),
    IIS_EXTRA(
        cache,
        "Current Metadata Cached",
        NULL,
        "metadata_cached",
        "iis.server_metadata_cached",
        "Metadata blocks in the IIS cache",
        "blocks",
        "blocks",
        NULL,
        NULL,
        NULL,
        IIS_EXTRA_GLOBAL,
        RRDSET_TYPE_LINE),
    IIS_EXTRA_INCREMENTAL(
        cache,
        "Metadata Cache Flushes",
        NULL,
        "metadata_flushes",
        "iis.server_metadata_flushes",
        "IIS metadata cache flushes",
        "flushes/s",
        "flushes",
        NULL,
        NULL,
        NULL,
        IIS_EXTRA_GLOBAL,
        RRDSET_TYPE_LINE),
    IIS_EXTRA_INCREMENTAL(
        cache,
        "Metadata Cache Hits",
        "Metadata Cache Misses",
        "metadata_queries",
        "iis.server_metadata_queries",
        "IIS metadata cache queries",
        "queries/s",
        "queries",
        NULL,
        NULL,
        NULL,
        IIS_EXTRA_GLOBAL,
        RRDSET_TYPE_LINE),
    IIS_EXTRA_INCREMENTAL(
        cache,
        "Metadata Cache Hits",
        NULL,
        "metadata_hits",
        "iis.server_metadata_hits",
        "IIS metadata cache hits",
        "hits/s",
        "hits",
        NULL,
        NULL,
        NULL,
        IIS_EXTRA_GLOBAL,
        RRDSET_TYPE_LINE),
    IIS_EXTRA_INCREMENTAL(
        cache,
        "Total Metadata Cached",
        NULL,
        "metadata_cached_total",
        "iis.server_metadata_cached_total",
        "Metadata blocks added to the IIS cache",
        "blocks/s",
        "blocks",
        NULL,
        NULL,
        NULL,
        IIS_EXTRA_GLOBAL,
        RRDSET_TYPE_LINE),
    IIS_EXTRA_INCREMENTAL(
        cache,
        "Total Flushed Metadata",
        NULL,
        "metadata_flushed_total",
        "iis.server_metadata_flushed_total",
        "Metadata blocks removed from the IIS cache",
        "blocks/s",
        "blocks",
        NULL,
        NULL,
        NULL,
        IIS_EXTRA_GLOBAL,
        RRDSET_TYPE_LINE),
    IIS_EXTRA(
        cache,
        "Output Cache Current Flushed Items",
        NULL,
        "output_active_flushed",
        "iis.server_output_cache_active_flushed",
        "Active and flushed IIS output cache items",
        "items",
        "items",
        NULL,
        NULL,
        NULL,
        IIS_EXTRA_GLOBAL,
        RRDSET_TYPE_LINE),
    IIS_EXTRA(
        cache,
        "Output Cache Current Items",
        NULL,
        "output_items",
        "iis.server_output_cache_items",
        "Items in the IIS output cache",
        "items",
        "items",
        NULL,
        NULL,
        NULL,
        IIS_EXTRA_GLOBAL,
        RRDSET_TYPE_LINE),
    IIS_EXTRA(
        cache,
        "Output Cache Current Memory Usage",
        NULL,
        "output_memory",
        "iis.server_output_cache_memory",
        "IIS output cache memory",
        "bytes",
        "used",
        NULL,
        NULL,
        NULL,
        IIS_EXTRA_GLOBAL,
        RRDSET_TYPE_AREA),
    IIS_EXTRA_INCREMENTAL(
        cache,
        "Output Cache Total Hits",
        "Output Cache Total Misses",
        "output_queries",
        "iis.server_output_cache_queries",
        "IIS output cache queries",
        "queries/s",
        "queries",
        NULL,
        NULL,
        NULL,
        IIS_EXTRA_GLOBAL,
        RRDSET_TYPE_LINE),
    IIS_EXTRA_INCREMENTAL(
        cache,
        "Output Cache Total Hits",
        NULL,
        "output_hits",
        "iis.server_output_cache_hits",
        "IIS output cache hits",
        "hits/s",
        "hits",
        NULL,
        NULL,
        NULL,
        IIS_EXTRA_GLOBAL,
        RRDSET_TYPE_LINE),
    IIS_EXTRA_INCREMENTAL(
        cache,
        "Output Cache Total Flushed Items",
        NULL,
        "output_flushed",
        "iis.server_output_cache_flushed",
        "IIS output cache items flushed",
        "items/s",
        "items",
        NULL,
        NULL,
        NULL,
        IIS_EXTRA_GLOBAL,
        RRDSET_TYPE_LINE),
    IIS_EXTRA_INCREMENTAL(
        cache,
        "Output Cache Total Flushes",
        NULL,
        "output_flushes",
        "iis.server_output_cache_flushes",
        "IIS output cache flushes",
        "flushes/s",
        "flushes",
        NULL,
        NULL,
        NULL,
        IIS_EXTRA_GLOBAL,
        RRDSET_TYPE_LINE),
};

static struct iis_extra_group web_service_group = {
    .definitions = web_service_definitions,
    .count = IIS_EXTRA_ARRAY_SIZE(web_service_definitions),
    .object = "Web Service",
    .chart_prefix = "iis_site"};
static struct iis_extra_group http_queue_group = {
    .definitions = http_queue_definitions,
    .count = IIS_EXTRA_ARRAY_SIZE(http_queue_definitions),
    .object = "HTTP Service Request Queues",
    .chart_prefix = "iis_http_queue"};
static struct iis_extra_group worker_group = {
    .definitions = worker_definitions,
    .count = IIS_EXTRA_ARRAY_SIZE(worker_definitions),
    .object = "W3SVC_W3WP",
    .chart_prefix = "iis_worker"};
static struct iis_extra_group cache_group = {
    .definitions = cache_definitions,
    .count = IIS_EXTRA_ARRAY_SIZE(cache_definitions),
    .object = "Web Service Cache",
    .chart_prefix = "iis_server_cache"};

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
    const char *pid,
    int update_every)
{
    COUNTER_DATA combined;
    COUNTER_DATA *counter = &value->first;
    if (definition->key2) {
        if (!value->first.updated || !value->second.updated)
            return;
        combined = value->first;
        combined.current.Data += value->second.current.Data;
        combined.previous.Data += value->second.previous.Data;
        combined.current.CounterType = PERF_COUNTER_BULK_COUNT;
        counter = &combined;
    } else if (!counter->updated)
        return;

    if (unlikely(!value->st)) {
        char id[RRD_ID_LENGTH_MAX + 1];
        char identity[PERFLIB_MAX_NAME_LENGTH];
        if (pid)
            snprintfz(identity, sizeof(identity), "%s_%s", pid, app ? app : instance);
        else
            strncpyz(identity, app ? app : (instance ? instance : "global"), sizeof(identity) - 1);
        snprintfz(id, sizeof(id), "%s_%s_%s", group->chart_prefix, identity, definition->chart);
        netdata_fix_chart_name(id);
        value->st = rrdset_create_localhost(
            "iis",
            id,
            NULL,
            definition->label ? "iis" : "iis server",
            definition->context,
            definition->title,
            definition->units,
            PLUGIN_WINDOWS_NAME,
            "PerflibWebService",
            PRIO_WEBSITE_IIS_REQUESTS_RATE + 100 + (int)(definition - group->definitions),
            update_every,
            definition->chart_type);
        value->rd = definition->incremental ?
                        rrddim_add(value->st, definition->dimension, NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL) :
                        perflib_rrddim_add(value->st, definition->dimension, NULL, 1, 1, counter);
        if (definition->label && (app || instance || definition->label_value)) {
            const char *label_value =
                (definition->label_value && !definition->label2) ? definition->label_value : (app ? app : instance);
            rrdlabels_add(value->st->rrdlabels, definition->label, label_value, RRDLABEL_SRC_AUTO);
        }
        if (definition->label2 && pid)
            rrdlabels_add(value->st->rrdlabels, definition->label2, pid, RRDLABEL_SRC_AUTO);
        if (definition->label_value && definition->label2) {
            const char *value_label =
                (pid && strcmp(definition->label2, "pid") == 0) ? "status_code" : definition->label2;
            rrdlabels_add(value->st->rrdlabels, value_label, definition->label_value, RRDLABEL_SRC_AUTO);
        }
    }

    perflib_rrddim_set_by_pointer(value->st, value->rd, counter);
    rrdset_done(value->st);
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
                if (group->definitions[n].key2)
                    state->values[n].second.key = group->definitions[n].key2;
            }
            state->initialized = true;
        }
        state->seen = true;
        for (size_t n = 0; n < group->count; n++) {
            struct iis_extra_value *value = &state->values[n];
            perflibGetObjectCounter(data, object, &value->first);
            if (group->definitions[n].key2)
                perflibGetObjectCounter(data, object, &value->second);
            iis_extra_emit(group, value, &group->definitions[n], NULL, NULL, NULL, update_every);
        }
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
        char pid[32] = "";
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
            strncpyz(pid, windows_shared_buffer, sizeof(pid) - 1);
            strncpyz(app, separator, sizeof(app) - 1);
            instance = app;
        } else if (strcasecmp(instance, "_Total") == 0 || strncmp(instance, "---", 3) == 0)
            continue;

        struct iis_extra_instance *state = dictionary_set(group->instances, instance_key, NULL, sizeof(*state));
        state->seen = true;
        if (!state->initialized) {
            for (size_t n = 0; n < group->count; n++) {
                state->values[n].first.key = group->definitions[n].key;
                if (group->definitions[n].key2)
                    state->values[n].second.key = group->definitions[n].key2;
            }
            state->initialized = true;
        }

        for (size_t n = 0; n < group->count; n++) {
            struct iis_extra_value *value = &state->values[n];
            perflibGetInstanceCounter(data, object, pi, &value->first);
            if (group->definitions[n].key2)
                perflibGetInstanceCounter(data, object, pi, &value->second);
            iis_extra_emit(
                group, value, &group->definitions[n], instance, app[0] ? app : NULL, pid[0] ? pid : NULL, update_every);
        }
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

void do_PerflibWebServiceExtraHTTPQueue(PERF_DATA_BLOCK *data, int update_every)
{
    do_iis_extra_group(data, &http_queue_group, update_every);
}

void do_PerflibWebServiceExtraCache(PERF_DATA_BLOCK *data, int update_every)
{
    do_iis_extra_group(data, &cache_group, update_every);
}
