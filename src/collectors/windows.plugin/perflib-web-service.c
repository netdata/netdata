// SPDX-License-Identifier: GPL-3.0-or-later

#include "windows_plugin.h"
#include "windows-internals.h"

struct web_service {
    RRDSET *st_request_rate;
    RRDDIM *rd_request_rate;

    RRDSET *st_request_by_type_rate;
    RRDDIM *rd_request_options_rate;
    RRDDIM *rd_request_get_rate;
    RRDDIM *rd_request_post_rate;
    RRDDIM *rd_request_head_rate;
    RRDDIM *rd_request_put_rate;
    RRDDIM *rd_request_delete_rate;
    RRDDIM *rd_request_trace_rate;
    RRDDIM *rd_request_move_rate;
    RRDDIM *rd_request_copy_rate;
    RRDDIM *rd_request_mkcol_rate;
    RRDDIM *rd_request_propfind_rate;
    RRDDIM *rd_request_proppatch_rate;
    RRDDIM *rd_request_search_rate;
    RRDDIM *rd_request_lock_rate;
    RRDDIM *rd_request_unlock_rate;
    RRDDIM *rd_request_other_rate;

    RRDSET *st_traffic;
    RRDDIM *rd_traffic_received;
    RRDDIM *rd_traffic_sent;

    RRDSET *st_file_transfer;
    RRDDIM *rd_files_received;
    RRDDIM *rd_files_sent;

    RRDSET *st_curr_connections;
    RRDDIM *rd_curr_connections;

    RRDSET *st_connections_attempts;
    RRDDIM *rd_connections_attempts;

    RRDSET *st_user_count;
    RRDDIM *rd_user_anonymous;
    RRDDIM *rd_user_nonanonymous;

    RRDSET *st_isapi_extension_request_count;
    RRDDIM *rd_isapi_extension_request_count;

    RRDSET *st_isapi_extension_request_rate;
    RRDDIM *rd_isapi_extension_request_rate;

    RRDSET *st_error_rate;
    RRDDIM *rd_error_rate_locked;
    RRDDIM *rd_error_rate_not_found;

    RRDSET *st_logon_attempts;
    RRDDIM *rd_logon_attempts;

    RRDSET *st_service_uptime;
    RRDDIM *rd_service_uptime;

    COUNTER_DATA IISCurrentAnonymousUser;
    COUNTER_DATA IISCurrentNonAnonymousUsers;
    COUNTER_DATA IISCurrentConnections;
    COUNTER_DATA IISCurrentISAPIExtRequests;
    COUNTER_DATA IISUptime;

    COUNTER_DATA IISReceivedBytesTotal;
    COUNTER_DATA IISSentBytesTotal;
    COUNTER_DATA IISIPAPIExtRequestsTotal;
    COUNTER_DATA IISConnAttemptsAllInstancesTotal;
    COUNTER_DATA IISFilesReceivedTotal;
    COUNTER_DATA IISFilesSentTotal;
    COUNTER_DATA IISLogonAttemptsTotal;
    COUNTER_DATA IISLockedErrorsTotal;
    COUNTER_DATA IISNotFoundErrorsTotal;

    COUNTER_DATA IISRequestsOptions;
    COUNTER_DATA IISRequestsGet;
    COUNTER_DATA IISRequestsPost;
    COUNTER_DATA IISRequestsHead;
    COUNTER_DATA IISRequestsPut;
    COUNTER_DATA IISRequestsDelete;
    COUNTER_DATA IISRequestsTrace;
    COUNTER_DATA IISRequestsMove;
    COUNTER_DATA IISRequestsCopy;
    COUNTER_DATA IISRequestsMkcol;
    COUNTER_DATA IISRequestsPropfind;
    COUNTER_DATA IISRequestsProppatch;
    COUNTER_DATA IISRequestsSearch;
    COUNTER_DATA IISRequestsLock;
    COUNTER_DATA IISRequestsUnlock;
    COUNTER_DATA IISRequestsOther;
};

struct ws3svc_w3wp_data {
    RRDSET *st_wescv_w3wp_active_threads;
    RRDDIM *rd_wescv_w3wp_active_threads;

    RRDSET *st_wescv_w3wp_requests_total;
    RRDDIM *rd_wescv_w3wp_requests_total;

    RRDSET *st_wescv_w3wp_requests_active;
    RRDDIM *rd_wescv_w3wp_requests_active;

    RRDSET *st_wescv_w3wp_file_cache_mem_usage;
    RRDDIM *rd_wescv_w3wp_file_cache_mem_usage;

    RRDSET *st_wescv_w3wp_files_cache_total;
    RRDDIM *rd_wescv_w3wp_files_cache_total;

    RRDSET *st_wescv_w3wp_files_flushed_total;
    RRDDIM *rd_wescv_w3wp_files_flushed_total;

    RRDSET *st_wescv_w3wp_uri_cache_flushed;
    RRDDIM *rd_wescv_w3wp_uri_cache_flushed;

    RRDSET *st_wescv_w3wp_total_uri_cached;
    RRDDIM *rd_wescv_w3wp_total_uri_cached;

    RRDSET *st_wescv_w3wp_total_metadata_cache;
    RRDDIM *rd_wescv_w3wp_total_metadata_cache;

    RRDSET *st_wescv_w3wp_total_metadata_flushed;
    RRDDIM *rd_wescv_w3wp_total_metadata_flushed;

    RRDSET *st_wescv_w3wp_output_cache_active_flushed_items;
    RRDDIM *rd_wescv_w3wp_output_cache_active_flushed_items;

    RRDSET *st_wescv_w3wp_output_cache_memory_usage;
    RRDDIM *rd_wescv_w3wp_output_cache_memory_usage;

    RRDSET *st_wescv_w3wp_output_cache_flushed_total;
    RRDDIM *rd_wescv_w3wp_output_cache_flushed_total;

    COUNTER_DATA WESCVW3WPActiveThreads;

    COUNTER_DATA WESCVW3WPRequestTotal;
    COUNTER_DATA WESCVW3WPRequestActive;

    COUNTER_DATA WESCVW3WPFileCacheMemUsage;

    COUNTER_DATA WESCVW3WPFilesCachedTotal;
    COUNTER_DATA WESCVW3WPFilesFlushedTotal;

    COUNTER_DATA WESCVW3WPURICachedFlushed;
    COUNTER_DATA WESCVW3WPTotalURICached;

    COUNTER_DATA WESCVW3WPTotalMetadataCached;
    COUNTER_DATA WESCVW3WPTotalMetadataFlushed;

    COUNTER_DATA WESCVW3WPOutputCacheActiveFlushedItens;
    COUNTER_DATA WESCVW3WPOutputCacheMemoryUsage;
    COUNTER_DATA WESCVW3WPOutputCacheFlushesTotal;
};

// AD information
struct iis_app {
    RRDSET *st_app_current_application_pool_state;
    RRDDIM *rd_app_current_application_pool_state_uninitialized;
    RRDDIM *rd_app_current_application_pool_state_initialized;
    RRDDIM *rd_app_current_application_pool_state_running;
    RRDDIM *rd_app_current_application_pool_state_disabling;
    RRDDIM *rd_app_current_application_pool_state_disabled;
    RRDDIM *rd_app_current_application_pool_state_shutdown_pending;
    RRDDIM *rd_app_current_application_pool_state_delete_pending;

    RRDSET *st_app_current_worker_process;
    RRDDIM *rd_app_current_worker_process;

    RRDSET *st_app_maximum_worker_process;
    RRDDIM *rd_app_maximum_worker_process;

    RRDSET *st_app_recent_worker_process_failure;
    RRDDIM *rd_app_recent_worker_process_failure;

    RRDSET *st_app_application_pool_recycles;
    RRDDIM *rd_app_application_pool_recycles;

    RRDSET *st_app_application_pool_uptime;
    RRDDIM *rd_app_application_pool_uptime;

    RRDSET *st_app_worker_process_created;
    RRDDIM *rd_app_worker_process_created;

    RRDSET *st_app_worker_process_failures;
    RRDDIM *rd_app_worker_process_crashes;
    RRDDIM *rd_app_worker_process_ping_failures;
    RRDDIM *rd_app_worker_process_shutdown_failures;
    RRDDIM *rd_app_worker_process_startup_failures;

    COUNTER_DATA APPCurrentApplicationPoolState;
    COUNTER_DATA APPCurrentApplicationPoolUptime;
    COUNTER_DATA APPCurrentWorkerProcess;
    COUNTER_DATA APPMaximumWorkerProcess;
    COUNTER_DATA APPRecentWorkerProcessFailure;
    COUNTER_DATA APPTimeSinceProcessFailure;
    COUNTER_DATA APPApplicationPoolRecycles;
    COUNTER_DATA APPTotalApplicationPoolUptime;
    COUNTER_DATA APPWorkerProcessCreated;
    COUNTER_DATA APPWorkerProcessFailures;
    COUNTER_DATA APPWorkerProcessPingFailures;
    COUNTER_DATA APPWorkerProcessShutdownFailures;
    COUNTER_DATA APPWorkerProcessStartupFailures;
};

static inline void initialize_web_service_keys(struct web_service *p)
{
    p->IISCurrentAnonymousUser.key = "Current Anonymous Users";
    p->IISCurrentNonAnonymousUsers.key = "Current NonAnonymous Users";
    p->IISCurrentConnections.key = "Current Connections";
    p->IISCurrentISAPIExtRequests.key = "Current ISAPI Extension Requests";
    p->IISUptime.key = "Service Uptime";

    p->IISReceivedBytesTotal.key = "Total Bytes Received";
    p->IISSentBytesTotal.key = "Total Bytes Sent";
    p->IISIPAPIExtRequestsTotal.key = "Total ISAPI Extension Requests";
    p->IISConnAttemptsAllInstancesTotal.key = "Total Connection Attempts (all instances)";
    p->IISFilesReceivedTotal.key = "Total Files Received";
    p->IISFilesSentTotal.key = "Total Files Sent";
    p->IISLogonAttemptsTotal.key = "Total Logon Attempts";
    p->IISLockedErrorsTotal.key = "Total Locked Errors";
    p->IISNotFoundErrorsTotal.key = "Total Not Found Errors";

    p->IISRequestsOptions.key = "Options Requests/sec";
    p->IISRequestsGet.key = "Get Requests/sec";
    p->IISRequestsPost.key = "Post Requests/sec";
    p->IISRequestsHead.key = "Head Requests/sec";
    p->IISRequestsPut.key = "Put Requests/sec";
    p->IISRequestsDelete.key = "Delete Requests/sec";
    p->IISRequestsTrace.key = "Trace Requests/sec";
    p->IISRequestsMove.key = "Move Requests/sec";
    p->IISRequestsCopy.key = "Copy Requests/sec";
    p->IISRequestsMkcol.key = "Mkcol Requests/sec";
    p->IISRequestsPropfind.key = "Propfind Requests/sec";
    p->IISRequestsProppatch.key = "Proppatch Requests/sec";
    p->IISRequestsSearch.key = "Search Requests/sec";
    p->IISRequestsLock.key = "Lock Requests/sec";
    p->IISRequestsUnlock.key = "Unlock Requests/sec";
    p->IISRequestsOther.key = "Other Request Methods/sec";
}

void dict_web_service_insert_cb(const DICTIONARY_ITEM *item __maybe_unused, void *value, void *data __maybe_unused)
{
    struct web_service *p = value;
    initialize_web_service_keys(p);
}

static inline void initialize_app_pool_keys(struct iis_app *p)
{
    p->APPCurrentApplicationPoolState.key = "Current Application Pool State";
    p->APPCurrentApplicationPoolUptime.key = "Current Application Pool Uptime";
    p->APPCurrentWorkerProcess.key = "Current Worker Processes";
    p->APPMaximumWorkerProcess.key = "Maximum Worker Processes";
    p->APPRecentWorkerProcessFailure.key = "Recent Worker Process Failures";
    p->APPTimeSinceProcessFailure.key = "Time Since Last Worker Process Failure";
    p->APPApplicationPoolRecycles.key = "Total Application Pool Recycles";
    p->APPTotalApplicationPoolUptime.key = "Total Application Pool Uptime";
    p->APPWorkerProcessCreated.key = "Total Worker Processes Created";
    p->APPWorkerProcessFailures.key = "Total Worker Process Failures";
    p->APPWorkerProcessPingFailures.key = "Total Worker Process Ping Failures";
    p->APPWorkerProcessShutdownFailures.key = "Total Worker Process Shutdown Failures";
    p->APPWorkerProcessStartupFailures.key = "Total Worker Process Startup Failures";
}

void dict_app_pool_insert_cb(const DICTIONARY_ITEM *item __maybe_unused, void *value, void *data __maybe_unused)
{
    struct iis_app *p = value;
    initialize_app_pool_keys(p);
}

static inline void initialize_w3svc_w3wp_keys(struct ws3svc_w3wp_data *p)
{
    p->WESCVW3WPActiveThreads.key = "Active Threads Count";

    p->WESCVW3WPRequestTotal.key = "Total HTTP Requests Served";
    p->WESCVW3WPRequestActive.key = "Active Requests";

    p->WESCVW3WPFileCacheMemUsage.key = "Current File Cache Memory Usage";

    p->WESCVW3WPFilesCachedTotal.key = "Total Files Cached";
    p->WESCVW3WPFilesFlushedTotal.key = "Total Flushed Files";

    p->WESCVW3WPURICachedFlushed.key = "Total Flushed URIs";
    p->WESCVW3WPTotalURICached.key = "Total URIs Cached";

    p->WESCVW3WPTotalMetadataCached.key = "Total Metadata Cached";
    p->WESCVW3WPTotalMetadataFlushed.key = "Total Flushed Metadata";

    p->WESCVW3WPOutputCacheActiveFlushedItens.key = "Output Cache Current Flushed Items";
    p->WESCVW3WPOutputCacheMemoryUsage.key = "Output Cache Current Memory Usage";
    p->WESCVW3WPOutputCacheFlushesTotal.key = "Output Cache Total Flushes";
}

void dict_wesvc_w3wp_insert_cb(const DICTIONARY_ITEM *item __maybe_unused, void *value, void *data __maybe_unused)
{
    struct ws3svc_w3wp_data *p = value;
    initialize_w3svc_w3wp_keys(p);
}

static DICTIONARY *web_services = NULL;
static DICTIONARY *app_pools = NULL;
static DICTIONARY *w3svc_w3wp_service = NULL;

static void initialize(void)
{
    // IIS
    web_services = dictionary_create_advanced(
        DICT_OPTION_DONT_OVERWRITE_VALUE | DICT_OPTION_FIXED_SIZE, NULL, sizeof(struct web_service));

    dictionary_register_insert_callback(web_services, dict_web_service_insert_cb, NULL);

    // AD (APP_POOL_WAS)
    app_pools = dictionary_create_advanced(
        DICT_OPTION_DONT_OVERWRITE_VALUE | DICT_OPTION_FIXED_SIZE, NULL, sizeof(struct iis_app));

    dictionary_register_insert_callback(app_pools, dict_app_pool_insert_cb, NULL);

    w3svc_w3wp_service = dictionary_create_advanced(
        DICT_OPTION_DONT_OVERWRITE_VALUE | DICT_OPTION_FIXED_SIZE, NULL, sizeof(struct ws3svc_w3wp_data));

    dictionary_register_insert_callback(w3svc_w3wp_service, dict_wesvc_w3wp_insert_cb, NULL);
}

static inline void netdata_webservice_traffic(
    PERF_DATA_BLOCK *pDataBlock,
    PERF_OBJECT_TYPE *pObjectType,
    PERF_INSTANCE_DEFINITION *pi,
    struct web_service *p,
    int update_every)
{
    if (perflibGetInstanceCounter(pDataBlock, pObjectType, pi, &p->IISReceivedBytesTotal) &&
        perflibGetInstanceCounter(pDataBlock, pObjectType, pi, &p->IISSentBytesTotal)) {
        if (!p->st_traffic) {
            char id[RRD_ID_LENGTH_MAX + 1];
            snprintfz(id, RRD_ID_LENGTH_MAX, "website_%s_traffic", windows_shared_buffer);
            netdata_fix_chart_name(id);
            p->st_traffic = rrdset_create_localhost(
                "iis",
                id,
                NULL,
                "traffic",
                "iis.website_traffic",
                "Website traffic",
                "bytes/s",
                PLUGIN_WINDOWS_NAME,
                "PerflibWebService",
                PRIO_WEBSITE_IIS_TRAFFIC,
                update_every,
                RRDSET_TYPE_AREA);

            p->rd_traffic_received = rrddim_add(p->st_traffic, "received", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
            p->rd_traffic_sent = rrddim_add(p->st_traffic, "sent", NULL, -1, 1, RRD_ALGORITHM_INCREMENTAL);
            rrdlabels_add(p->st_traffic->rrdlabels, "website", windows_shared_buffer, RRDLABEL_SRC_AUTO);
        }

        rrddim_set_by_pointer(
            p->st_traffic, p->rd_traffic_received, (collected_number)p->IISReceivedBytesTotal.current.Data);
        rrddim_set_by_pointer(p->st_traffic, p->rd_traffic_sent, (collected_number)p->IISSentBytesTotal.current.Data);

        rrdset_done(p->st_traffic);
    }
}

static inline void netdata_webservice_file_transfer_rate(
    PERF_DATA_BLOCK *pDataBlock,
    PERF_OBJECT_TYPE *pObjectType,
    PERF_INSTANCE_DEFINITION *pi,
    struct web_service *p,
    int update_every)
{
    if (perflibGetInstanceCounter(pDataBlock, pObjectType, pi, &p->IISFilesReceivedTotal) &&
        perflibGetInstanceCounter(pDataBlock, pObjectType, pi, &p->IISFilesSentTotal)) {
        if (!p->st_file_transfer) {
            char id[RRD_ID_LENGTH_MAX + 1];
            snprintfz(id, RRD_ID_LENGTH_MAX, "website_%s_ftp_file_transfer_rate", windows_shared_buffer);
            netdata_fix_chart_name(id);
            p->st_file_transfer = rrdset_create_localhost(
                "iis",
                id,
                NULL,
                "traffic",
                "iis.website_ftp_file_transfer_rate",
                "Website FTP file transfer rate",
                "files/s",
                PLUGIN_WINDOWS_NAME,
                "PerflibWebService",
                PRIO_WEBSITE_IIS_FTP_FILE_TRANSFER_RATE,
                update_every,
                RRDSET_TYPE_LINE);

            p->rd_files_received = rrddim_add(p->st_file_transfer, "received", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
            p->rd_files_sent = rrddim_add(p->st_file_transfer, "sent", NULL, -1, 1, RRD_ALGORITHM_INCREMENTAL);
            rrdlabels_add(p->st_file_transfer->rrdlabels, "website", windows_shared_buffer, RRDLABEL_SRC_AUTO);
        }

        rrddim_set_by_pointer(
            p->st_file_transfer, p->rd_files_received, (collected_number)p->IISFilesReceivedTotal.current.Data);
        rrddim_set_by_pointer(
            p->st_file_transfer, p->rd_files_sent, (collected_number)p->IISFilesSentTotal.current.Data);

        rrdset_done(p->st_file_transfer);
    }
}

static inline void netdata_webservice_active_connection(
    PERF_DATA_BLOCK *pDataBlock,
    PERF_OBJECT_TYPE *pObjectType,
    PERF_INSTANCE_DEFINITION *pi,
    struct web_service *p,
    int update_every)
{
    if (perflibGetInstanceCounter(pDataBlock, pObjectType, pi, &p->IISCurrentConnections)) {
        if (!p->st_curr_connections) {
            char id[RRD_ID_LENGTH_MAX + 1];
            snprintfz(id, RRD_ID_LENGTH_MAX, "website_%s_active_connections_count", windows_shared_buffer);
            netdata_fix_chart_name(id);
            p->st_curr_connections = rrdset_create_localhost(
                "iis",
                id,
                NULL,
                "connections",
                "iis.website_active_connections_count",
                "Website active connections",
                "connections",
                PLUGIN_WINDOWS_NAME,
                "PerflibWebService1",
                PRIO_WEBSITE_IIS_ACTIVE_CONNECTIONS_COUNT,
                update_every,
                RRDSET_TYPE_LINE);

            p->rd_curr_connections = rrddim_add(p->st_curr_connections, "active", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
            rrdlabels_add(p->st_curr_connections->rrdlabels, "website", windows_shared_buffer, RRDLABEL_SRC_AUTO);
        }

        rrddim_set_by_pointer(
            p->st_curr_connections, p->rd_curr_connections, (collected_number)p->IISCurrentConnections.current.Data);

        rrdset_done(p->st_curr_connections);
    }
}

static inline void netdata_webservice_connection_attempt_rate(
    PERF_DATA_BLOCK *pDataBlock,
    PERF_OBJECT_TYPE *pObjectType,
    PERF_INSTANCE_DEFINITION *pi,
    struct web_service *p,
    int update_every)
{
    if (perflibGetInstanceCounter(pDataBlock, pObjectType, pi, &p->IISConnAttemptsAllInstancesTotal)) {
        if (!p->st_connections_attempts) {
            char id[RRD_ID_LENGTH_MAX + 1];
            snprintfz(id, RRD_ID_LENGTH_MAX, "website_%s_connection_attemptts_rate", windows_shared_buffer);
            netdata_fix_chart_name(id);
            p->st_connections_attempts = rrdset_create_localhost(
                "iis",
                id,
                NULL,
                "connections",
                "iis.website_connection_attemptts_rate",
                "Website connections attemptts",
                "attemptts/s",
                PLUGIN_WINDOWS_NAME,
                "PerflibWebService",
                PRIO_WEBSITE_IIS_CONNECTIONS_ATTEMPT,
                update_every,
                RRDSET_TYPE_LINE);

            p->rd_connections_attempts =
                rrddim_add(p->st_connections_attempts, "connection", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);

            rrdlabels_add(p->st_connections_attempts->rrdlabels, "website", windows_shared_buffer, RRDLABEL_SRC_AUTO);
        }

        rrddim_set_by_pointer(
            p->st_connections_attempts,
            p->rd_connections_attempts,
            (collected_number)p->IISConnAttemptsAllInstancesTotal.current.Data);

        rrdset_done(p->st_connections_attempts);
    }
}

static inline void netdata_webservice_user_count(
    PERF_DATA_BLOCK *pDataBlock,
    PERF_OBJECT_TYPE *pObjectType,
    PERF_INSTANCE_DEFINITION *pi,
    struct web_service *p,
    int update_every)
{
    if (perflibGetInstanceCounter(pDataBlock, pObjectType, pi, &p->IISCurrentAnonymousUser) &&
        perflibGetInstanceCounter(pDataBlock, pObjectType, pi, &p->IISCurrentNonAnonymousUsers)) {
        if (!p->st_user_count) {
            char id[RRD_ID_LENGTH_MAX + 1];
            snprintfz(id, RRD_ID_LENGTH_MAX, "website_%s_users_count", windows_shared_buffer);
            netdata_fix_chart_name(id);
            p->st_user_count = rrdset_create_localhost(
                "iis",
                id,
                NULL,
                "requests",
                "iis.website_users_count",
                "Website users with pending requests",
                "users",
                PLUGIN_WINDOWS_NAME,
                "PerflibWebService",
                PRIO_WEBSITE_IIS_USERS,
                update_every,
                RRDSET_TYPE_STACKED);

            p->rd_user_anonymous = rrddim_add(p->st_user_count, "anonymous", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
            p->rd_user_nonanonymous = rrddim_add(p->st_user_count, "non_anonymous", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);

            rrdlabels_add(p->st_user_count->rrdlabels, "website", windows_shared_buffer, RRDLABEL_SRC_AUTO);
        }

        rrddim_set_by_pointer(
            p->st_user_count, p->rd_user_anonymous, (collected_number)p->IISCurrentAnonymousUser.current.Data);

        rrddim_set_by_pointer(
            p->st_user_count, p->rd_user_nonanonymous, (collected_number)p->IISCurrentNonAnonymousUsers.current.Data);

        rrdset_done(p->st_user_count);
    }
}

static inline void netdata_webservice_isapi_extension_request_count(
    PERF_DATA_BLOCK *pDataBlock,
    PERF_OBJECT_TYPE *pObjectType,
    PERF_INSTANCE_DEFINITION *pi,
    struct web_service *p,
    int update_every)
{
    if (perflibGetInstanceCounter(pDataBlock, pObjectType, pi, &p->IISCurrentISAPIExtRequests)) {
        if (!p->st_isapi_extension_request_count) {
            char id[RRD_ID_LENGTH_MAX + 1];
            snprintfz(id, RRD_ID_LENGTH_MAX, "website_%s_isapi_extension_requests_count", windows_shared_buffer);
            netdata_fix_chart_name(id);
            p->st_isapi_extension_request_count = rrdset_create_localhost(
                "iis",
                id,
                NULL,
                "requests",
                "iis.website_isapi_extension_requests_count",
                "ISAPI extension requests",
                "requests",
                PLUGIN_WINDOWS_NAME,
                "PerflibWebService",
                PRIO_WEBSITE_IIS_ISAPI_EXT_REQUEST_COUNT,
                update_every,
                RRDSET_TYPE_LINE);

            p->rd_isapi_extension_request_count =
                rrddim_add(p->st_isapi_extension_request_count, "isapi", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);

            rrdlabels_add(
                p->st_isapi_extension_request_count->rrdlabels, "website", windows_shared_buffer, RRDLABEL_SRC_AUTO);
        }

        rrddim_set_by_pointer(
            p->st_isapi_extension_request_count,
            p->rd_isapi_extension_request_count,
            (collected_number)p->IISCurrentISAPIExtRequests.current.Data);

        rrdset_done(p->st_isapi_extension_request_count);
    }
}

static inline void netdata_webservice_isapi_extension_request_rate(
    PERF_DATA_BLOCK *pDataBlock,
    PERF_OBJECT_TYPE *pObjectType,
    PERF_INSTANCE_DEFINITION *pi,
    struct web_service *p,
    int update_every)
{
    if (perflibGetInstanceCounter(pDataBlock, pObjectType, pi, &p->IISIPAPIExtRequestsTotal)) {
        if (!p->st_isapi_extension_request_rate) {
            char id[RRD_ID_LENGTH_MAX + 1];
            snprintfz(id, RRD_ID_LENGTH_MAX, "website_%s_isapi_extension_requests_rate", windows_shared_buffer);
            netdata_fix_chart_name(id);
            p->st_isapi_extension_request_rate = rrdset_create_localhost(
                "iis",
                id,
                NULL,
                "requests",
                "iis.website_isapi_extension_requests_rate",
                "Website extensions request",
                "requests/s",
                PLUGIN_WINDOWS_NAME,
                "PerflibWebService",
                PRIO_WEBSITE_IIS_ISAPI_EXT_REQUEST_RATE,
                update_every,
                RRDSET_TYPE_LINE);

            p->rd_isapi_extension_request_rate =
                rrddim_add(p->st_isapi_extension_request_rate, "isapi", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);

            rrdlabels_add(
                p->st_isapi_extension_request_rate->rrdlabels, "website", windows_shared_buffer, RRDLABEL_SRC_AUTO);
        }

        rrddim_set_by_pointer(
            p->st_isapi_extension_request_rate,
            p->rd_isapi_extension_request_rate,
            (collected_number)p->IISIPAPIExtRequestsTotal.current.Data);

        rrdset_done(p->st_isapi_extension_request_rate);
    }
}

static inline void netdata_webservice_errors_rate(
    PERF_DATA_BLOCK *pDataBlock,
    PERF_OBJECT_TYPE *pObjectType,
    PERF_INSTANCE_DEFINITION *pi,
    struct web_service *p,
    int update_every)
{
    if (perflibGetInstanceCounter(pDataBlock, pObjectType, pi, &p->IISLockedErrorsTotal) &&
        perflibGetInstanceCounter(pDataBlock, pObjectType, pi, &p->IISNotFoundErrorsTotal)) {
        if (!p->st_error_rate) {
            char id[RRD_ID_LENGTH_MAX + 1];
            snprintfz(id, RRD_ID_LENGTH_MAX, "website_%s_errors_rate", windows_shared_buffer);
            netdata_fix_chart_name(id);
            p->st_error_rate = rrdset_create_localhost(
                "iis",
                id,
                NULL,
                "requests",
                "iis.website_errors_rate",
                "Website errors",
                "errors/s",
                PLUGIN_WINDOWS_NAME,
                "PerflibWebService",
                PRIO_WEBSITE_IIS_USERS,
                update_every,
                RRDSET_TYPE_STACKED);

            p->rd_error_rate_locked =
                rrddim_add(p->st_error_rate, "document_locked", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
            p->rd_error_rate_not_found =
                rrddim_add(p->st_error_rate, "document_not_found", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);

            rrdlabels_add(p->st_error_rate->rrdlabels, "website", windows_shared_buffer, RRDLABEL_SRC_AUTO);
        }

        rrddim_set_by_pointer(
            p->st_error_rate, p->rd_error_rate_locked, (collected_number)p->IISLockedErrorsTotal.current.Data);

        rrddim_set_by_pointer(
            p->st_error_rate, p->rd_error_rate_not_found, (collected_number)p->IISNotFoundErrorsTotal.current.Data);

        rrdset_done(p->st_error_rate);
    }
}

static inline void netdata_webservice_logon_attempt_rate(
    PERF_DATA_BLOCK *pDataBlock,
    PERF_OBJECT_TYPE *pObjectType,
    PERF_INSTANCE_DEFINITION *pi,
    struct web_service *p,
    int update_every)
{
    if (perflibGetInstanceCounter(pDataBlock, pObjectType, pi, &p->IISLogonAttemptsTotal)) {
        if (!p->st_logon_attempts) {
            char id[RRD_ID_LENGTH_MAX + 1];
            snprintfz(id, RRD_ID_LENGTH_MAX, "website_%s_logon_attemptts_rate", windows_shared_buffer);
            netdata_fix_chart_name(id);
            p->st_logon_attempts = rrdset_create_localhost(
                "iis",
                id,
                NULL,
                "logon",
                "iis.website_logon_attemptts_rate",
                "Website logon attemptts",
                "attemptts/s",
                PLUGIN_WINDOWS_NAME,
                "PerflibWebService",
                PRIO_WEBSITE_IIS_LOGON_ATTEMPTS,
                update_every,
                RRDSET_TYPE_LINE);

            p->rd_logon_attempts = rrddim_add(p->st_logon_attempts, "logon", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);

            rrdlabels_add(p->st_logon_attempts->rrdlabels, "website", windows_shared_buffer, RRDLABEL_SRC_AUTO);
        }

        rrddim_set_by_pointer(
            p->st_logon_attempts, p->rd_logon_attempts, (collected_number)p->IISLogonAttemptsTotal.current.Data);

        rrdset_done(p->st_logon_attempts);
    }
}

static inline void netdata_webservice_uptime(
    PERF_DATA_BLOCK *pDataBlock,
    PERF_OBJECT_TYPE *pObjectType,
    PERF_INSTANCE_DEFINITION *pi,
    struct web_service *p,
    int update_every)
{
    if (perflibGetInstanceCounter(pDataBlock, pObjectType, pi, &p->IISUptime)) {
        if (!p->st_service_uptime) {
            char id[RRD_ID_LENGTH_MAX + 1];
            snprintfz(id, RRD_ID_LENGTH_MAX, "website_%s_uptime", windows_shared_buffer);
            netdata_fix_chart_name(id);
            p->st_service_uptime = rrdset_create_localhost(
                "iis",
                id,
                NULL,
                "uptime",
                "iis.website_uptime",
                "Website uptime",
                "seconds",
                PLUGIN_WINDOWS_NAME,
                "PerflibWebService",
                PRIO_WEBSITE_IIS_UPTIME,
                update_every,
                RRDSET_TYPE_LINE);

            p->rd_service_uptime = rrddim_add(p->st_service_uptime, "uptime", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);

            rrdlabels_add(p->st_service_uptime->rrdlabels, "website", windows_shared_buffer, RRDLABEL_SRC_AUTO);
        }

        rrddim_set_by_pointer(p->st_service_uptime, p->rd_service_uptime, (collected_number)p->IISUptime.current.Data);

        rrdset_done(p->st_service_uptime);
    }
}

static inline void netdata_webservice_requests(
    PERF_DATA_BLOCK *pDataBlock,
    PERF_OBJECT_TYPE *pObjectType,
    PERF_INSTANCE_DEFINITION *pi,
    struct web_service *p,
    int update_every)
{
    if (perflibGetInstanceCounter(pDataBlock, pObjectType, pi, &p->IISRequestsOptions) &&
        perflibGetInstanceCounter(pDataBlock, pObjectType, pi, &p->IISRequestsGet) &&
        perflibGetInstanceCounter(pDataBlock, pObjectType, pi, &p->IISRequestsPost) &&
        perflibGetInstanceCounter(pDataBlock, pObjectType, pi, &p->IISRequestsHead) &&
        perflibGetInstanceCounter(pDataBlock, pObjectType, pi, &p->IISRequestsPut) &&
        perflibGetInstanceCounter(pDataBlock, pObjectType, pi, &p->IISRequestsDelete) &&
        perflibGetInstanceCounter(pDataBlock, pObjectType, pi, &p->IISRequestsTrace) &&
        perflibGetInstanceCounter(pDataBlock, pObjectType, pi, &p->IISRequestsMove) &&
        perflibGetInstanceCounter(pDataBlock, pObjectType, pi, &p->IISRequestsCopy) &&
        perflibGetInstanceCounter(pDataBlock, pObjectType, pi, &p->IISRequestsMkcol) &&
        perflibGetInstanceCounter(pDataBlock, pObjectType, pi, &p->IISRequestsPropfind) &&
        perflibGetInstanceCounter(pDataBlock, pObjectType, pi, &p->IISRequestsProppatch) &&
        perflibGetInstanceCounter(pDataBlock, pObjectType, pi, &p->IISRequestsSearch) &&
        perflibGetInstanceCounter(pDataBlock, pObjectType, pi, &p->IISRequestsLock) &&
        perflibGetInstanceCounter(pDataBlock, pObjectType, pi, &p->IISRequestsUnlock) &&
        perflibGetInstanceCounter(pDataBlock, pObjectType, pi, &p->IISRequestsOther)) {
        char id[RRD_ID_LENGTH_MAX + 1];
        if (!p->st_request_rate) {
            snprintfz(id, RRD_ID_LENGTH_MAX, "website_%s_requests_rate", windows_shared_buffer);
            netdata_fix_chart_name(id);
            p->st_request_rate = rrdset_create_localhost(
                "iis",
                id,
                NULL,
                "requests",
                "iis.website_requests_rate",
                "Website requests rate",
                "requests/s",
                PLUGIN_WINDOWS_NAME,
                "PerflibWebService",
                PRIO_WEBSITE_IIS_REQUESTS_RATE,
                update_every,
                RRDSET_TYPE_LINE);

            p->rd_request_rate = rrddim_add(p->st_request_rate, "requests", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);

            rrdlabels_add(p->st_request_rate->rrdlabels, "website", windows_shared_buffer, RRDLABEL_SRC_AUTO);
        }

        uint64_t requests =
            p->IISRequestsOptions.current.Data + p->IISRequestsGet.current.Data + p->IISRequestsPost.current.Data +
            p->IISRequestsHead.current.Data + p->IISRequestsPut.current.Data + p->IISRequestsDelete.current.Data +
            p->IISRequestsTrace.current.Data + p->IISRequestsMove.current.Data + p->IISRequestsCopy.current.Data +
            p->IISRequestsMkcol.current.Data + p->IISRequestsPropfind.current.Data +
            p->IISRequestsProppatch.current.Data + p->IISRequestsSearch.current.Data + p->IISRequestsLock.current.Data +
            p->IISRequestsUnlock.current.Data + p->IISRequestsOther.current.Data;

        rrddim_set_by_pointer(p->st_request_rate, p->rd_request_rate, (collected_number)requests);

        rrdset_done(p->st_request_rate);

        if (!p->st_request_by_type_rate) {
            snprintfz(id, RRD_ID_LENGTH_MAX, "website_%s_requests_by_type_rate", windows_shared_buffer);
            netdata_fix_chart_name(id);
            p->st_request_by_type_rate = rrdset_create_localhost(
                "iis",
                id,
                NULL,
                "requests",
                "iis.website_requests_by_type_rate",
                "Website requests rate",
                "requests/s",
                PLUGIN_WINDOWS_NAME,
                "PerflibWebService",
                PRIO_WEBSITE_IIS_REQUESTS_BY_TYPE_RATE,
                update_every,
                RRDSET_TYPE_STACKED);

            p->rd_request_options_rate =
                rrddim_add(p->st_request_by_type_rate, "options", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
            p->rd_request_get_rate =
                rrddim_add(p->st_request_by_type_rate, "get", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
            p->rd_request_post_rate =
                rrddim_add(p->st_request_by_type_rate, "post", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
            p->rd_request_head_rate =
                rrddim_add(p->st_request_by_type_rate, "head", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
            p->rd_request_put_rate =
                rrddim_add(p->st_request_by_type_rate, "put", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
            p->rd_request_delete_rate =
                rrddim_add(p->st_request_by_type_rate, "delete", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
            p->rd_request_trace_rate =
                rrddim_add(p->st_request_by_type_rate, "trace", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
            p->rd_request_move_rate =
                rrddim_add(p->st_request_by_type_rate, "move", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
            p->rd_request_copy_rate =
                rrddim_add(p->st_request_by_type_rate, "copy", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
            p->rd_request_mkcol_rate =
                rrddim_add(p->st_request_by_type_rate, "mkcol", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
            p->rd_request_propfind_rate =
                rrddim_add(p->st_request_by_type_rate, "propfind", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
            p->rd_request_proppatch_rate =
                rrddim_add(p->st_request_by_type_rate, "proppatch", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
            p->rd_request_search_rate =
                rrddim_add(p->st_request_by_type_rate, "search", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
            p->rd_request_lock_rate =
                rrddim_add(p->st_request_by_type_rate, "lock", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
            p->rd_request_unlock_rate =
                rrddim_add(p->st_request_by_type_rate, "unlock", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
            p->rd_request_other_rate =
                rrddim_add(p->st_request_by_type_rate, "other", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);

            rrdlabels_add(p->st_request_by_type_rate->rrdlabels, "website", windows_shared_buffer, RRDLABEL_SRC_AUTO);
        }

        rrddim_set_by_pointer(
            p->st_request_by_type_rate,
            p->rd_request_options_rate,
            (collected_number)p->IISRequestsOptions.current.Data);
        rrddim_set_by_pointer(
            p->st_request_by_type_rate, p->rd_request_get_rate, (collected_number)p->IISRequestsGet.current.Data);
        rrddim_set_by_pointer(
            p->st_request_by_type_rate, p->rd_request_post_rate, (collected_number)p->IISRequestsPost.current.Data);
        rrddim_set_by_pointer(
            p->st_request_by_type_rate, p->rd_request_head_rate, (collected_number)p->IISRequestsHead.current.Data);
        rrddim_set_by_pointer(
            p->st_request_by_type_rate, p->rd_request_put_rate, (collected_number)p->IISRequestsPut.current.Data);
        rrddim_set_by_pointer(
            p->st_request_by_type_rate, p->rd_request_delete_rate, (collected_number)p->IISRequestsDelete.current.Data);
        rrddim_set_by_pointer(
            p->st_request_by_type_rate, p->rd_request_trace_rate, (collected_number)p->IISRequestsTrace.current.Data);
        rrddim_set_by_pointer(
            p->st_request_by_type_rate, p->rd_request_move_rate, (collected_number)p->IISRequestsMove.current.Data);
        rrddim_set_by_pointer(
            p->st_request_by_type_rate, p->rd_request_copy_rate, (collected_number)p->IISRequestsCopy.current.Data);
        rrddim_set_by_pointer(
            p->st_request_by_type_rate, p->rd_request_mkcol_rate, (collected_number)p->IISRequestsMkcol.current.Data);
        rrddim_set_by_pointer(
            p->st_request_by_type_rate,
            p->rd_request_propfind_rate,
            (collected_number)p->IISRequestsPropfind.current.Data);
        rrddim_set_by_pointer(
            p->st_request_by_type_rate,
            p->rd_request_proppatch_rate,
            (collected_number)p->IISRequestsProppatch.current.Data);
        rrddim_set_by_pointer(
            p->st_request_by_type_rate, p->rd_request_search_rate, (collected_number)p->IISRequestsSearch.current.Data);
        rrddim_set_by_pointer(
            p->st_request_by_type_rate, p->rd_request_lock_rate, (collected_number)p->IISRequestsLock.current.Data);
        rrddim_set_by_pointer(
            p->st_request_by_type_rate, p->rd_request_unlock_rate, (collected_number)p->IISRequestsUnlock.current.Data);
        rrddim_set_by_pointer(
            p->st_request_by_type_rate, p->rd_request_other_rate, (collected_number)p->IISRequestsOther.current.Data);

        rrdset_done(p->st_request_by_type_rate);
    }
}

static bool do_web_services(PERF_DATA_BLOCK *pDataBlock, int update_every)
{
    PERF_OBJECT_TYPE *pObjectType = perflibFindObjectTypeByName(pDataBlock, "Web Service");
    if (!pObjectType)
        return false;

    PERF_INSTANCE_DEFINITION *pi = NULL;
    for (LONG i = 0; i < pObjectType->NumInstances; i++) {
        pi = perflibForEachInstance(pDataBlock, pObjectType, pi);
        if (!pi)
            break;

        if (!getInstanceName(pDataBlock, pObjectType, pi, windows_shared_buffer, sizeof(windows_shared_buffer)))
            strncpyz(windows_shared_buffer, "[unknown]", sizeof(windows_shared_buffer) - 1);

        // We are not ploting _Total here, because cloud will group the sites
        if (strcasecmp(windows_shared_buffer, "_Total") == 0) {
            continue;
        }

        struct web_service *p = dictionary_set(web_services, windows_shared_buffer, NULL, sizeof(*p));

        netdata_webservice_traffic(pDataBlock, pObjectType, pi, p, update_every);
        netdata_webservice_file_transfer_rate(pDataBlock, pObjectType, pi, p, update_every);
        netdata_webservice_active_connection(pDataBlock, pObjectType, pi, p, update_every);
        netdata_webservice_connection_attempt_rate(pDataBlock, pObjectType, pi, p, update_every);
        netdata_webservice_user_count(pDataBlock, pObjectType, pi, p, update_every);
        netdata_webservice_isapi_extension_request_count(pDataBlock, pObjectType, pi, p, update_every);
        netdata_webservice_isapi_extension_request_rate(pDataBlock, pObjectType, pi, p, update_every);
        netdata_webservice_errors_rate(pDataBlock, pObjectType, pi, p, update_every);
        netdata_webservice_logon_attempt_rate(pDataBlock, pObjectType, pi, p, update_every);
        netdata_webservice_uptime(pDataBlock, pObjectType, pi, p, update_every);
        netdata_webservice_requests(pDataBlock, pObjectType, pi, p, update_every);
    }

    return true;
}

static RRDDIM *app_pool_select_dim(struct iis_app *p, uint32_t selector)
{
    switch (selector) {
        case 1:
            return p->rd_app_current_application_pool_state_uninitialized;
        case 2:
            return p->rd_app_current_application_pool_state_initialized;
        case 3:
            return p->rd_app_current_application_pool_state_running;
        case 4:
            return p->rd_app_current_application_pool_state_disabling;
        case 5:
            return p->rd_app_current_application_pool_state_disabled;
        case 6:
            return p->rd_app_current_application_pool_state_shutdown_pending;
        case 7:
        default:
            return p->rd_app_current_application_pool_state_delete_pending;
    }
}

static inline void app_pool_current_state(
    struct iis_app *p,
    PERF_DATA_BLOCK *pDataBlock,
    PERF_OBJECT_TYPE *pObjectType,
    PERF_INSTANCE_DEFINITION *pi,
    int update_every)
{
    if (perflibGetInstanceCounter(pDataBlock, pObjectType, pi, &p->APPCurrentApplicationPoolState)) {
        if (!p->st_app_current_application_pool_state) {
            char id[RRD_ID_LENGTH_MAX + 1];
            snprintfz(id, RRD_ID_LENGTH_MAX, "application_pool_%s_current_state", windows_shared_buffer);
            netdata_fix_chart_name(id);
            p->st_app_current_application_pool_state = rrdset_create_localhost(
                "iis",
                id,
                NULL,
                "app pool status",
                "iis.application_pool_current_status",
                "IIS App Pool current status",
                "status",
                PLUGIN_WINDOWS_NAME,
                "PerflibWebService",
                PRIO_IIS_APP_POOL_STATE,
                update_every,
                RRDSET_TYPE_LINE);

            p->rd_app_current_application_pool_state_uninitialized = rrddim_add(
                p->st_app_current_application_pool_state, "uninitialized", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);

            p->rd_app_current_application_pool_state_initialized =
                rrddim_add(p->st_app_current_application_pool_state, "initialized", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);

            p->rd_app_current_application_pool_state_running =
                rrddim_add(p->st_app_current_application_pool_state, "running", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);

            p->rd_app_current_application_pool_state_disabling =
                rrddim_add(p->st_app_current_application_pool_state, "disabling", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);

            p->rd_app_current_application_pool_state_disabled =
                rrddim_add(p->st_app_current_application_pool_state, "disabled", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);

            p->rd_app_current_application_pool_state_shutdown_pending = rrddim_add(
                p->st_app_current_application_pool_state, "shutdown_pending", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);

            p->rd_app_current_application_pool_state_delete_pending = rrddim_add(
                p->st_app_current_application_pool_state, "delete_pending", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);

            rrdlabels_add(
                p->st_app_current_application_pool_state->rrdlabels,
                "app_pool",
                windows_shared_buffer,
                RRDLABEL_SRC_AUTO);
        }

#define NETDATA_APP_COOL_TOTAL_STATES (7)
        uint32_t current_state = (uint32_t)p->APPCurrentApplicationPoolState.current.Data;
        for (uint32_t i = 1; i <= NETDATA_APP_COOL_TOTAL_STATES; i++) {
            RRDDIM *dim = app_pool_select_dim(p, i);
            uint32_t value = (current_state == i) ? 1 : 0;

            rrddim_set_by_pointer(p->st_app_current_application_pool_state, dim, (collected_number)value);
        }

        rrdset_done(p->st_app_current_application_pool_state);
    }
}

static inline void app_pool_current_worker_processes(
    struct iis_app *p,
    PERF_DATA_BLOCK *pDataBlock,
    PERF_OBJECT_TYPE *pObjectType,
    PERF_INSTANCE_DEFINITION *pi,
    int update_every)
{
    if (perflibGetInstanceCounter(pDataBlock, pObjectType, pi, &p->APPCurrentWorkerProcess)) {
        if (!p->st_app_current_worker_process) {
            char id[RRD_ID_LENGTH_MAX + 1];
            snprintfz(id, RRD_ID_LENGTH_MAX, "application_pool_%s_current_worker_processes", windows_shared_buffer);
            netdata_fix_chart_name(id);
            p->st_app_current_worker_process = rrdset_create_localhost(
                "iis",
                id,
                NULL,
                "app pool worker processes",
                "iis.application_pool_current_worker_processes",
                "IIS App Pool worker processes currently running",
                "processes",
                PLUGIN_WINDOWS_NAME,
                "PerflibWebService",
                PRIO_IIS_APP_POOL_WORKER_PROCESSES_CURRENT,
                update_every,
                RRDSET_TYPE_LINE);

            p->rd_app_current_worker_process =
                rrddim_add(p->st_app_current_worker_process, "running", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
            rrdlabels_add(
                p->st_app_current_worker_process->rrdlabels, "app_pool", windows_shared_buffer, RRDLABEL_SRC_AUTO);
        }

        rrddim_set_by_pointer(
            p->st_app_current_worker_process,
            p->rd_app_current_worker_process,
            (collected_number)p->APPCurrentWorkerProcess.current.Data);

        rrdset_done(p->st_app_current_worker_process);
    }
}

static inline void app_pool_maximum_worker_processes(
    struct iis_app *p,
    PERF_DATA_BLOCK *pDataBlock,
    PERF_OBJECT_TYPE *pObjectType,
    PERF_INSTANCE_DEFINITION *pi,
    int update_every)
{
    if (perflibGetInstanceCounter(pDataBlock, pObjectType, pi, &p->APPMaximumWorkerProcess)) {
        if (!p->st_app_maximum_worker_process) {
            char id[RRD_ID_LENGTH_MAX + 1];
            snprintfz(id, RRD_ID_LENGTH_MAX, "application_pool_%s_maximum_worker_processes", windows_shared_buffer);
            netdata_fix_chart_name(id);
            p->st_app_maximum_worker_process = rrdset_create_localhost(
                "iis",
                id,
                NULL,
                "app pool worker processes",
                "iis.application_pool_maximum_worker_processes",
                "IIS App Pool maximum created worker processes",
                "processes",
                PLUGIN_WINDOWS_NAME,
                "PerflibWebService",
                PRIO_IIS_APP_POOL_WORKER_PROCESSES_MAX,
                update_every,
                RRDSET_TYPE_LINE);

            p->rd_app_maximum_worker_process =
                rrddim_add(p->st_app_maximum_worker_process, "created", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
            rrdlabels_add(
                p->st_app_maximum_worker_process->rrdlabels, "app_pool", windows_shared_buffer, RRDLABEL_SRC_AUTO);
        }

        rrddim_set_by_pointer(
            p->st_app_maximum_worker_process,
            p->rd_app_maximum_worker_process,
            (collected_number)p->APPMaximumWorkerProcess.current.Data);

        rrdset_done(p->st_app_maximum_worker_process);
    }
}

static inline void app_pool_recent_worker_process_failures(
    struct iis_app *p,
    PERF_DATA_BLOCK *pDataBlock,
    PERF_OBJECT_TYPE *pObjectType,
    PERF_INSTANCE_DEFINITION *pi,
    int update_every)
{
    if (perflibGetInstanceCounter(pDataBlock, pObjectType, pi, &p->APPRecentWorkerProcessFailure)) {
        if (!p->st_app_recent_worker_process_failure) {
            char id[RRD_ID_LENGTH_MAX + 1];
            snprintfz(
                id, RRD_ID_LENGTH_MAX, "application_pool_%s_recent_worker_process_failures", windows_shared_buffer);
            netdata_fix_chart_name(id);
            p->st_app_recent_worker_process_failure = rrdset_create_localhost(
                "iis",
                id,
                NULL,
                "app pool failures",
                "iis.application_pool_recent_worker_process_failures",
                "IIS App Pool worker process failures during the rapid-fail protection interval",
                "failures/s",
                PLUGIN_WINDOWS_NAME,
                "PerflibWebService",
                PRIO_IIS_APP_POOL_WORKER_PROCESS_RECENT_FAILURES,
                update_every,
                RRDSET_TYPE_LINE);

            p->rd_app_recent_worker_process_failure =
                rrddim_add(p->st_app_recent_worker_process_failure, "failures", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
            rrdlabels_add(
                p->st_app_recent_worker_process_failure->rrdlabels,
                "app_pool",
                windows_shared_buffer,
                RRDLABEL_SRC_AUTO);
        }

        rrddim_set_by_pointer(
            p->st_app_recent_worker_process_failure,
            p->rd_app_recent_worker_process_failure,
            (collected_number)p->APPRecentWorkerProcessFailure.current.Data);

        rrdset_done(p->st_app_recent_worker_process_failure);
    }
}

static inline void app_pool_recycles(
    struct iis_app *p,
    PERF_DATA_BLOCK *pDataBlock,
    PERF_OBJECT_TYPE *pObjectType,
    PERF_INSTANCE_DEFINITION *pi,
    int update_every)
{
    if (perflibGetInstanceCounter(pDataBlock, pObjectType, pi, &p->APPApplicationPoolRecycles)) {
        if (!p->st_app_application_pool_recycles) {
            char id[RRD_ID_LENGTH_MAX + 1];
            snprintfz(id, RRD_ID_LENGTH_MAX, "application_pool_%s_recycles", windows_shared_buffer);
            netdata_fix_chart_name(id);
            p->st_app_application_pool_recycles = rrdset_create_localhost(
                "iis",
                id,
                NULL,
                "app pool recycles",
                "iis.application_pool_recycles",
                "IIS App Pool recycles",
                "recycles/s",
                PLUGIN_WINDOWS_NAME,
                "PerflibWebService",
                PRIO_IIS_APP_POOL_RECYCLES,
                update_every,
                RRDSET_TYPE_LINE);

            p->rd_app_application_pool_recycles =
                rrddim_add(p->st_app_application_pool_recycles, "recycles", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
            rrdlabels_add(
                p->st_app_application_pool_recycles->rrdlabels, "app_pool", windows_shared_buffer, RRDLABEL_SRC_AUTO);
        }

        rrddim_set_by_pointer(
            p->st_app_application_pool_recycles,
            p->rd_app_application_pool_recycles,
            (collected_number)p->APPApplicationPoolRecycles.current.Data);

        rrdset_done(p->st_app_application_pool_recycles);
    }
}

static inline void app_pool_uptime(
    struct iis_app *p,
    PERF_DATA_BLOCK *pDataBlock,
    PERF_OBJECT_TYPE *pObjectType,
    PERF_INSTANCE_DEFINITION *pi,
    int update_every)
{
    if (perflibGetInstanceCounter(pDataBlock, pObjectType, pi, &p->APPTotalApplicationPoolUptime)) {
        if (p->APPTotalApplicationPoolUptime.current.Frequency != 0) {
            if (!p->st_app_application_pool_uptime) {
                char id[RRD_ID_LENGTH_MAX + 1];
                snprintfz(id, RRD_ID_LENGTH_MAX, "application_pool_%s_uptime", windows_shared_buffer);
                netdata_fix_chart_name(id);
                p->st_app_application_pool_uptime = rrdset_create_localhost(
                    "iis",
                    id,
                    NULL,
                    "app pool uptime",
                    "iis.application_pool_uptime",
                    "IIS App Pool uptime",
                    "seconds",
                    PLUGIN_WINDOWS_NAME,
                    "PerflibWebService",
                    PRIO_IIS_APP_POOL_TOTAL_UPTIME,
                    update_every,
                    RRDSET_TYPE_LINE);

                p->rd_app_application_pool_uptime =
                    rrddim_add(p->st_app_application_pool_uptime, "uptime", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
                rrdlabels_add(
                    p->st_app_application_pool_uptime->rrdlabels, "app_pool", windows_shared_buffer, RRDLABEL_SRC_AUTO);
            }

            time_t uptime = (time_t)(p->APPTotalApplicationPoolUptime.current.Time /
                                     p->APPTotalApplicationPoolUptime.current.Frequency);

            rrddim_set_by_pointer(
                p->st_app_application_pool_uptime, p->rd_app_application_pool_uptime, (collected_number)uptime);

            rrdset_done(p->st_app_application_pool_uptime);
        }
    }
}

static inline void app_pool_worker_processes_created(
    struct iis_app *p,
    PERF_DATA_BLOCK *pDataBlock,
    PERF_OBJECT_TYPE *pObjectType,
    PERF_INSTANCE_DEFINITION *pi,
    int update_every)
{
    if (perflibGetInstanceCounter(pDataBlock, pObjectType, pi, &p->APPWorkerProcessCreated)) {
        if (!p->st_app_worker_process_created) {
            char id[RRD_ID_LENGTH_MAX + 1];
            snprintfz(id, RRD_ID_LENGTH_MAX, "application_pool_%s_worker_processes_created", windows_shared_buffer);
            netdata_fix_chart_name(id);
            p->st_app_worker_process_created = rrdset_create_localhost(
                "iis",
                id,
                NULL,
                "app pool worker processes",
                "iis.application_pool_worker_processes_created",
                "IIS App Pool worker processes created",
                "processes/s",
                PLUGIN_WINDOWS_NAME,
                "PerflibWebService",
                PRIO_IIS_APP_POOL_TOTAL_WORKER_PROCESSES_CREATED,
                update_every,
                RRDSET_TYPE_LINE);

            p->rd_app_worker_process_created =
                rrddim_add(p->st_app_worker_process_created, "created", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
            rrdlabels_add(
                p->st_app_worker_process_created->rrdlabels, "app_pool", windows_shared_buffer, RRDLABEL_SRC_AUTO);
        }

        rrddim_set_by_pointer(
            p->st_app_worker_process_created,
            p->rd_app_worker_process_created,
            (collected_number)p->APPWorkerProcessCreated.current.Data);

        rrdset_done(p->st_app_worker_process_created);
    }
}

static inline void app_pool_worker_process_failures(
    struct iis_app *p,
    PERF_DATA_BLOCK *pDataBlock,
    PERF_OBJECT_TYPE *pObjectType,
    PERF_INSTANCE_DEFINITION *pi,
    int update_every)
{
    if (perflibGetInstanceCounter(pDataBlock, pObjectType, pi, &p->APPWorkerProcessFailures) &&
        perflibGetInstanceCounter(pDataBlock, pObjectType, pi, &p->APPWorkerProcessPingFailures) &&
        perflibGetInstanceCounter(pDataBlock, pObjectType, pi, &p->APPWorkerProcessShutdownFailures) &&
        perflibGetInstanceCounter(pDataBlock, pObjectType, pi, &p->APPWorkerProcessStartupFailures)) {
        if (!p->st_app_worker_process_failures) {
            char id[RRD_ID_LENGTH_MAX + 1];
            snprintfz(id, RRD_ID_LENGTH_MAX, "application_pool_%s_worker_process_failures", windows_shared_buffer);
            netdata_fix_chart_name(id);
            p->st_app_worker_process_failures = rrdset_create_localhost(
                "iis",
                id,
                NULL,
                "app pool failures",
                "iis.application_pool_worker_process_failures",
                "IIS App Pool worker process failures",
                "failures/s",
                PLUGIN_WINDOWS_NAME,
                "PerflibWebService",
                PRIO_IIS_APP_POOL_TOTAL_WORKER_PROCESS_FAILURES,
                update_every,
                RRDSET_TYPE_LINE);

            p->rd_app_worker_process_crashes =
                rrddim_add(p->st_app_worker_process_failures, "crash", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
            p->rd_app_worker_process_ping_failures =
                rrddim_add(p->st_app_worker_process_failures, "ping", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
            p->rd_app_worker_process_startup_failures =
                rrddim_add(p->st_app_worker_process_failures, "startup", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
            p->rd_app_worker_process_shutdown_failures =
                rrddim_add(p->st_app_worker_process_failures, "shutdown", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
            rrdlabels_add(
                p->st_app_worker_process_failures->rrdlabels, "app_pool", windows_shared_buffer, RRDLABEL_SRC_AUTO);
        }

        rrddim_set_by_pointer(
            p->st_app_worker_process_failures,
            p->rd_app_worker_process_crashes,
            (collected_number)p->APPWorkerProcessFailures.current.Data);

        rrddim_set_by_pointer(
            p->st_app_worker_process_failures,
            p->rd_app_worker_process_ping_failures,
            (collected_number)p->APPWorkerProcessPingFailures.current.Data);

        rrddim_set_by_pointer(
            p->st_app_worker_process_failures,
            p->rd_app_worker_process_startup_failures,
            (collected_number)p->APPWorkerProcessStartupFailures.current.Data);

        rrddim_set_by_pointer(
            p->st_app_worker_process_failures,
            p->rd_app_worker_process_shutdown_failures,
            (collected_number)p->APPWorkerProcessShutdownFailures.current.Data);

        rrdset_done(p->st_app_worker_process_failures);
    }
}

static bool do_app_pool(PERF_DATA_BLOCK *pDataBlock, int update_every)
{
    PERF_OBJECT_TYPE *pObjectType = perflibFindObjectTypeByName(pDataBlock, "APP_POOL_WAS");
    if (!pObjectType)
        return false;

    PERF_INSTANCE_DEFINITION *pi = NULL;
    for (LONG i = 0; i < pObjectType->NumInstances; i++) {
        pi = perflibForEachInstance(pDataBlock, pObjectType, pi);
        if (!pi)
            break;

        if (!getInstanceName(pDataBlock, pObjectType, pi, windows_shared_buffer, sizeof(windows_shared_buffer)))
            strncpyz(windows_shared_buffer, "[unknown]", sizeof(windows_shared_buffer) - 1);

        // We are not ploting _Total here, because cloud will group the sites
        if (strcasecmp(windows_shared_buffer, "_Total") == 0) {
            continue;
        }

        struct iis_app *p = dictionary_set(app_pools, windows_shared_buffer, NULL, sizeof(*p));
        app_pool_current_state(p, pDataBlock, pObjectType, pi, update_every);

        app_pool_current_worker_processes(p, pDataBlock, pObjectType, pi, update_every);
        app_pool_maximum_worker_processes(p, pDataBlock, pObjectType, pi, update_every);
        app_pool_worker_processes_created(p, pDataBlock, pObjectType, pi, update_every);

        app_pool_recent_worker_process_failures(p, pDataBlock, pObjectType, pi, update_every);
        app_pool_worker_process_failures(p, pDataBlock, pObjectType, pi, update_every);

        app_pool_recycles(p, pDataBlock, pObjectType, pi, update_every);
        app_pool_uptime(p, pDataBlock, pObjectType, pi, update_every);
    }

    return true;
}

static int iis_web_service(char *name, int update_every, typeof(bool(PERF_DATA_BLOCK *, int)) *routine)
{
    DWORD id = RegistryFindIDByName(name);
    if (id == PERFLIB_REGISTRY_NAME_NOT_FOUND)
        return -1;

    PERF_DATA_BLOCK *pDataBlock = perflibGetPerformanceData(id);
    if (!pDataBlock)
        return -1;

    routine(pDataBlock, update_every);
    return 0;
}

static inline void w3svc_w3wp_active_threads(
    struct ws3svc_w3wp_data *p,
    PERF_DATA_BLOCK *pDataBlock,
    PERF_OBJECT_TYPE *pObjectType,
    PERF_INSTANCE_DEFINITION *pi,
    int update_every,
    char *app_name)
{
    if (perflibGetInstanceCounter(pDataBlock, pObjectType, pi, &p->WESCVW3WPActiveThreads)) {
        if (!p->st_wescv_w3wp_active_threads) {
            char id[RRD_ID_LENGTH_MAX + 1];
            snprintfz(id, RRD_ID_LENGTH_MAX, "w3svc_w3wp_%s_active_threads", app_name);
            netdata_fix_chart_name(id);
            p->st_wescv_w3wp_active_threads = rrdset_create_localhost(
                "iis",
                id,
                NULL,
                "w3svc w3wp",
                "iis.w3svc_w3wp_active_threads",
                "Threads actively processing requests in the worker process",
                "threads",
                PLUGIN_WINDOWS_NAME,
                "PerflibWebService",
                PRIO_W3SVC_W3WP_ACTIVE_THREADS,
                update_every,
                RRDSET_TYPE_LINE);

            p->rd_wescv_w3wp_active_threads =
                rrddim_add(p->st_wescv_w3wp_active_threads, "active", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
            rrdlabels_add(p->st_wescv_w3wp_active_threads->rrdlabels, "app", app_name, RRDLABEL_SRC_AUTO);
        }

        rrddim_set_by_pointer(
            p->st_wescv_w3wp_active_threads,
            p->rd_wescv_w3wp_active_threads,
            (collected_number)p->WESCVW3WPActiveThreads.current.Data);

        rrdset_done(p->st_wescv_w3wp_active_threads);
    }
}

static inline void w3svc_w3wp_requests_total(
    struct ws3svc_w3wp_data *p,
    PERF_DATA_BLOCK *pDataBlock,
    PERF_OBJECT_TYPE *pObjectType,
    PERF_INSTANCE_DEFINITION *pi,
    int update_every,
    char *app_name)
{
    if (perflibGetInstanceCounter(pDataBlock, pObjectType, pi, &p->WESCVW3WPRequestTotal)) {
        if (!p->st_wescv_w3wp_requests_total) {
            char id[RRD_ID_LENGTH_MAX + 1];
            snprintfz(id, RRD_ID_LENGTH_MAX, "w3svc_w3wp_%s_requests_total", app_name);
            netdata_fix_chart_name(id);
            p->st_wescv_w3wp_requests_total = rrdset_create_localhost(
                "iis",
                id,
                NULL,
                "w3svc w3wp",
                "iis.w3svc_w3wp_requests_total",
                "HTTP requests served by the worker process.",
                "requests/s",
                PLUGIN_WINDOWS_NAME,
                "PerflibWebService",
                PRIO_W3SVC_W3WP_REQUESTS_TOTAL,
                update_every,
                RRDSET_TYPE_LINE);

            p->rd_wescv_w3wp_requests_total =
                rrddim_add(p->st_wescv_w3wp_requests_total, "requests", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
            rrdlabels_add(p->st_wescv_w3wp_requests_total->rrdlabels, "app", app_name, RRDLABEL_SRC_AUTO);
        }

        rrddim_set_by_pointer(
            p->st_wescv_w3wp_requests_total,
            p->rd_wescv_w3wp_requests_total,
            (collected_number)p->WESCVW3WPRequestTotal.current.Data);

        rrdset_done(p->st_wescv_w3wp_requests_total);
    }
}

static inline void w3svc_w3wp_requests_active(
    struct ws3svc_w3wp_data *p,
    PERF_DATA_BLOCK *pDataBlock,
    PERF_OBJECT_TYPE *pObjectType,
    PERF_INSTANCE_DEFINITION *pi,
    int update_every,
    char *app_name)
{
    if (perflibGetInstanceCounter(pDataBlock, pObjectType, pi, &p->WESCVW3WPRequestActive)) {
        if (!p->st_wescv_w3wp_requests_active) {
            char id[RRD_ID_LENGTH_MAX + 1];
            snprintfz(id, RRD_ID_LENGTH_MAX, "w3svc_w3wp_%s_requests_active", app_name);
            netdata_fix_chart_name(id);
            p->st_wescv_w3wp_requests_active = rrdset_create_localhost(
                "iis",
                id,
                NULL,
                "w3svc w3wp",
                "iis.w3svc_w3wp_requests_active",
                "Current number of requests being processed by the worker process",
                "requests",
                PLUGIN_WINDOWS_NAME,
                "PerflibWebService",
                PRIO_W3SVC_W3WP_REQUESTS_ACTIVE,
                update_every,
                RRDSET_TYPE_LINE);

            p->rd_wescv_w3wp_requests_active =
                rrddim_add(p->st_wescv_w3wp_requests_active, "active", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
            rrdlabels_add(p->st_wescv_w3wp_requests_active->rrdlabels, "app", app_name, RRDLABEL_SRC_AUTO);
        }

        rrddim_set_by_pointer(
            p->st_wescv_w3wp_requests_active,
            p->rd_wescv_w3wp_requests_active,
            (collected_number)p->WESCVW3WPRequestActive.current.Data);

        rrdset_done(p->st_wescv_w3wp_requests_active);
    }
}

static inline void w3svc_w3wp_file_cache_mem_usage(
    struct ws3svc_w3wp_data *p,
    PERF_DATA_BLOCK *pDataBlock,
    PERF_OBJECT_TYPE *pObjectType,
    PERF_INSTANCE_DEFINITION *pi,
    int update_every,
    char *app_name)
{
    if (perflibGetInstanceCounter(pDataBlock, pObjectType, pi, &p->WESCVW3WPFileCacheMemUsage)) {
        if (!p->st_wescv_w3wp_file_cache_mem_usage) {
            char id[RRD_ID_LENGTH_MAX + 1];
            snprintfz(id, RRD_ID_LENGTH_MAX, "w3svc_w3wp_%s_file_cache_mem_usage", app_name);
            netdata_fix_chart_name(id);
            p->st_wescv_w3wp_file_cache_mem_usage = rrdset_create_localhost(
                "iis",
                id,
                NULL,
                "w3svc w3wp",
                "iis.w3svc_w3wp_file_cache_mem_usage",
                "Current memory usage by the worker process.",
                "bytes",
                PLUGIN_WINDOWS_NAME,
                "PerflibWebService",
                PRIO_W3SVC_W3WP_FILE_CACHE_MEM_USAGE,
                update_every,
                RRDSET_TYPE_AREA);

            p->rd_wescv_w3wp_file_cache_mem_usage =
                rrddim_add(p->st_wescv_w3wp_file_cache_mem_usage, "used", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
            rrdlabels_add(p->st_wescv_w3wp_file_cache_mem_usage->rrdlabels, "app", app_name, RRDLABEL_SRC_AUTO);
        }

        rrddim_set_by_pointer(
            p->st_wescv_w3wp_file_cache_mem_usage,
            p->rd_wescv_w3wp_file_cache_mem_usage,
            (collected_number)p->WESCVW3WPFileCacheMemUsage.current.Data);

        rrdset_done(p->st_wescv_w3wp_file_cache_mem_usage);
    }
}

static inline void w3svc_w3wp_files_cached_total(
    struct ws3svc_w3wp_data *p,
    PERF_DATA_BLOCK *pDataBlock,
    PERF_OBJECT_TYPE *pObjectType,
    PERF_INSTANCE_DEFINITION *pi,
    int update_every,
    char *app_name)
{
    if (perflibGetInstanceCounter(pDataBlock, pObjectType, pi, &p->WESCVW3WPFilesCachedTotal)) {
        if (!p->st_wescv_w3wp_files_cache_total) {
            char id[RRD_ID_LENGTH_MAX + 1];
            snprintfz(id, RRD_ID_LENGTH_MAX, "w3svc_w3wp_%s_files_cache_total", app_name);
            netdata_fix_chart_name(id);
            p->st_wescv_w3wp_files_cache_total = rrdset_create_localhost(
                "iis",
                id,
                NULL,
                "w3svc w3wp",
                "iis.w3svc_w3wp_files_cache_total",
                "Files whose contents were ever added to the cache",
                "files/s",
                PLUGIN_WINDOWS_NAME,
                "PerflibWebService",
                PRIO_W3SVC_W3WP_FILE_CACHE_TOTAL,
                update_every,
                RRDSET_TYPE_LINE);

            p->rd_wescv_w3wp_files_cache_total =
                rrddim_add(p->st_wescv_w3wp_files_cache_total, "cached_files", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
            rrdlabels_add(p->st_wescv_w3wp_files_cache_total->rrdlabels, "app", app_name, RRDLABEL_SRC_AUTO);
        }

        rrddim_set_by_pointer(
            p->st_wescv_w3wp_files_cache_total,
            p->rd_wescv_w3wp_files_cache_total,
            (collected_number)p->WESCVW3WPFilesCachedTotal.current.Data);

        rrdset_done(p->st_wescv_w3wp_files_cache_total);
    }
}

static inline void w3svc_w3wp_files_flushed_total(
    struct ws3svc_w3wp_data *p,
    PERF_DATA_BLOCK *pDataBlock,
    PERF_OBJECT_TYPE *pObjectType,
    PERF_INSTANCE_DEFINITION *pi,
    int update_every,
    char *app_name)
{
    if (perflibGetInstanceCounter(pDataBlock, pObjectType, pi, &p->WESCVW3WPFilesFlushedTotal)) {
        if (!p->st_wescv_w3wp_files_flushed_total) {
            char id[RRD_ID_LENGTH_MAX + 1];
            snprintfz(id, RRD_ID_LENGTH_MAX, "w3svc_w3wp_%s_files_flushed_total", app_name);
            netdata_fix_chart_name(id);
            p->st_wescv_w3wp_files_flushed_total = rrdset_create_localhost(
                "iis",
                id,
                NULL,
                "w3svc w3wp",
                "iis.w3svc_w3wp_files_flushed_total",
                "File handles that have been removed from the cache",
                "flushes/s",
                PLUGIN_WINDOWS_NAME,
                "PerflibWebService",
                PRIO_W3SVC_W3WP_FILE_FLUSHED_TOTAL,
                update_every,
                RRDSET_TYPE_LINE);

            p->rd_wescv_w3wp_files_flushed_total =
                rrddim_add(p->st_wescv_w3wp_files_flushed_total, "file_handles", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
            rrdlabels_add(p->st_wescv_w3wp_files_flushed_total->rrdlabels, "app", app_name, RRDLABEL_SRC_AUTO);
        }

        rrddim_set_by_pointer(
            p->st_wescv_w3wp_files_flushed_total,
            p->rd_wescv_w3wp_files_flushed_total,
            (collected_number)p->WESCVW3WPFilesFlushedTotal.current.Data);

        rrdset_done(p->st_wescv_w3wp_files_flushed_total);
    }
}

static inline void w3svc_w3wp_uri_cached_flushed(
    struct ws3svc_w3wp_data *p,
    PERF_DATA_BLOCK *pDataBlock,
    PERF_OBJECT_TYPE *pObjectType,
    PERF_INSTANCE_DEFINITION *pi,
    int update_every,
    char *app_name)
{
    if (perflibGetInstanceCounter(pDataBlock, pObjectType, pi, &p->WESCVW3WPURICachedFlushed)) {
        if (!p->st_wescv_w3wp_uri_cache_flushed) {
            char id[RRD_ID_LENGTH_MAX + 1];
            snprintfz(id, RRD_ID_LENGTH_MAX, "w3svc_w3wp_%s_uri_cache_flushed", app_name);
            netdata_fix_chart_name(id);
            p->st_wescv_w3wp_uri_cache_flushed = rrdset_create_localhost(
                "iis",
                id,
                NULL,
                "w3svc w3wp",
                "iis.w3svc_w3wp_uri_cache_flushed",
                "URI cache flushes",
                "flushes/s",
                PLUGIN_WINDOWS_NAME,
                "PerflibWebService",
                PRIO_W3SVC_W3WP_URI_FLUSHED_TOTAL,
                update_every,
                RRDSET_TYPE_LINE);

            p->rd_wescv_w3wp_uri_cache_flushed =
                rrddim_add(p->st_wescv_w3wp_uri_cache_flushed, "cached_uris", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
            rrdlabels_add(p->st_wescv_w3wp_uri_cache_flushed->rrdlabels, "app", app_name, RRDLABEL_SRC_AUTO);
        }

        rrddim_set_by_pointer(
            p->st_wescv_w3wp_uri_cache_flushed,
            p->rd_wescv_w3wp_uri_cache_flushed,
            (collected_number)p->WESCVW3WPURICachedFlushed.current.Data);

        rrdset_done(p->st_wescv_w3wp_uri_cache_flushed);
    }
}

static inline void w3svc_w3wp_total_uri_cached(
    struct ws3svc_w3wp_data *p,
    PERF_DATA_BLOCK *pDataBlock,
    PERF_OBJECT_TYPE *pObjectType,
    PERF_INSTANCE_DEFINITION *pi,
    int update_every,
    char *app_name)
{
    if (perflibGetInstanceCounter(pDataBlock, pObjectType, pi, &p->WESCVW3WPTotalURICached)) {
        if (!p->st_wescv_w3wp_total_uri_cached) {
            char id[RRD_ID_LENGTH_MAX + 1];
            snprintfz(id, RRD_ID_LENGTH_MAX, "w3svc_w3wp_%s_total_uri_cached", app_name);
            netdata_fix_chart_name(id);
            p->st_wescv_w3wp_total_uri_cached = rrdset_create_localhost(
                "iis",
                id,
                NULL,
                "w3svc w3wp",
                "iis.w3svc_w3wp_total_uri_cached",
                "URI information blocks added to the cache",
                "blocks/s",
                PLUGIN_WINDOWS_NAME,
                "PerflibWebService",
                PRIO_W3SVC_W3WP_URI_CACHED_TOTAL,
                update_every,
                RRDSET_TYPE_LINE);

            p->rd_wescv_w3wp_total_uri_cached =
                rrddim_add(p->st_wescv_w3wp_total_uri_cached, "uri_cache_blocks", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
            rrdlabels_add(p->st_wescv_w3wp_total_uri_cached->rrdlabels, "app", app_name, RRDLABEL_SRC_AUTO);
        }

        rrddim_set_by_pointer(
            p->st_wescv_w3wp_total_uri_cached,
            p->rd_wescv_w3wp_total_uri_cached,
            (collected_number)p->WESCVW3WPTotalURICached.current.Data);

        rrdset_done(p->st_wescv_w3wp_total_uri_cached);
    }
}

static inline void w3svc_w3wp_total_metadata_cached(
    struct ws3svc_w3wp_data *p,
    PERF_DATA_BLOCK *pDataBlock,
    PERF_OBJECT_TYPE *pObjectType,
    PERF_INSTANCE_DEFINITION *pi,
    int update_every,
    char *app_name)
{
    if (perflibGetInstanceCounter(pDataBlock, pObjectType, pi, &p->WESCVW3WPTotalMetadataCached)) {
        if (!p->st_wescv_w3wp_total_metadata_cache) {
            char id[RRD_ID_LENGTH_MAX + 1];
            snprintfz(id, RRD_ID_LENGTH_MAX, "w3svc_w3wp_%s_total_metadata_cache", app_name);
            netdata_fix_chart_name(id);
            p->st_wescv_w3wp_total_metadata_cache = rrdset_create_localhost(
                "iis",
                id,
                NULL,
                "w3svc w3wp",
                "iis.w3svc_w3wp_total_metadata_cached",
                "Metadata information blocks added to the user-mode cache",
                "blocks/s",
                PLUGIN_WINDOWS_NAME,
                "PerflibWebService",
                PRIO_W3SVC_W3WP_METADATA_CACHED,
                update_every,
                RRDSET_TYPE_LINE);

            p->rd_wescv_w3wp_total_metadata_cache =
                rrddim_add(p->st_wescv_w3wp_total_metadata_cache, "metadata_blocks", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
            rrdlabels_add(p->st_wescv_w3wp_total_metadata_cache->rrdlabels, "app", app_name, RRDLABEL_SRC_AUTO);
        }

        rrddim_set_by_pointer(
            p->st_wescv_w3wp_total_metadata_cache,
            p->rd_wescv_w3wp_total_metadata_cache,
            (collected_number)p->WESCVW3WPTotalMetadataCached.current.Data);

        rrdset_done(p->st_wescv_w3wp_total_metadata_cache);
    }
}

static inline void w3svc_w3wp_total_metadata_flushed(
    struct ws3svc_w3wp_data *p,
    PERF_DATA_BLOCK *pDataBlock,
    PERF_OBJECT_TYPE *pObjectType,
    PERF_INSTANCE_DEFINITION *pi,
    int update_every,
    char *app_name)
{
    if (perflibGetInstanceCounter(pDataBlock, pObjectType, pi, &p->WESCVW3WPTotalMetadataFlushed)) {
        if (!p->st_wescv_w3wp_total_metadata_flushed) {
            char id[RRD_ID_LENGTH_MAX + 1];
            snprintfz(id, RRD_ID_LENGTH_MAX, "w3svc_w3wp_%s_total_metadata_flushed", app_name);
            netdata_fix_chart_name(id);
            p->st_wescv_w3wp_total_metadata_flushed = rrdset_create_localhost(
                "iis",
                id,
                NULL,
                "w3svc w3wp",
                "iis.w3svc_w3wp_total_metadata_flushed",
                "User-mode metadata cache flushed",
                "flushes/s",
                PLUGIN_WINDOWS_NAME,
                "PerflibWebService",
                PRIO_W3SVC_W3WP_METADATA_FLUSHED,
                update_every,
                RRDSET_TYPE_LINE);

            p->rd_wescv_w3wp_total_metadata_flushed =
                rrddim_add(p->st_wescv_w3wp_total_metadata_flushed, "metadata_blocks", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
            rrdlabels_add(p->st_wescv_w3wp_total_metadata_flushed->rrdlabels, "app", app_name, RRDLABEL_SRC_AUTO);
        }

        rrddim_set_by_pointer(
            p->st_wescv_w3wp_total_metadata_flushed,
            p->rd_wescv_w3wp_total_metadata_flushed,
            (collected_number)p->WESCVW3WPTotalMetadataFlushed.current.Data);

        rrdset_done(p->st_wescv_w3wp_total_metadata_flushed);
    }
}

static inline void w3svc_w3wp_output_cache_active_flushed_items(
    struct ws3svc_w3wp_data *p,
    PERF_DATA_BLOCK *pDataBlock,
    PERF_OBJECT_TYPE *pObjectType,
    PERF_INSTANCE_DEFINITION *pi,
    int update_every,
    char *app_name)
{
    if (perflibGetInstanceCounter(pDataBlock, pObjectType, pi, &p->WESCVW3WPOutputCacheActiveFlushedItens)) {
        if (!p->st_wescv_w3wp_output_cache_active_flushed_items) {
            char id[RRD_ID_LENGTH_MAX + 1];
            snprintfz(id, RRD_ID_LENGTH_MAX, "w3svc_w3wp_%s_output_cache_active_flushed_items", app_name);
            netdata_fix_chart_name(id);
            p->st_wescv_w3wp_output_cache_active_flushed_items = rrdset_create_localhost(
                "iis",
                id,
                NULL,
                "w3svc w3wp",
                "iis.w3svc_w3wp_output_cache_active_flushed_items",
                "Items flushed but still in memory for active responses",
                "items",
                PLUGIN_WINDOWS_NAME,
                "PerflibWebService",
                PRIO_W3SVC_W3WP_OUTPUT_CACHE_ACTIVE_FLUSH,
                update_every,
                RRDSET_TYPE_LINE);

            p->rd_wescv_w3wp_output_cache_active_flushed_items = rrddim_add(
                p->st_wescv_w3wp_output_cache_active_flushed_items, "used", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
            rrdlabels_add(
                p->st_wescv_w3wp_output_cache_active_flushed_items->rrdlabels, "app", app_name, RRDLABEL_SRC_AUTO);
        }

        rrddim_set_by_pointer(
            p->st_wescv_w3wp_output_cache_active_flushed_items,
            p->rd_wescv_w3wp_output_cache_active_flushed_items,
            (collected_number)p->WESCVW3WPOutputCacheActiveFlushedItens.current.Data);

        rrdset_done(p->st_wescv_w3wp_output_cache_active_flushed_items);
    }
}

static inline void w3svc_w3wp_output_cache_memory_usage(
    struct ws3svc_w3wp_data *p,
    PERF_DATA_BLOCK *pDataBlock,
    PERF_OBJECT_TYPE *pObjectType,
    PERF_INSTANCE_DEFINITION *pi,
    int update_every,
    char *app_name)
{
    if (perflibGetInstanceCounter(pDataBlock, pObjectType, pi, &p->WESCVW3WPOutputCacheMemoryUsage)) {
        if (!p->st_wescv_w3wp_output_cache_memory_usage) {
            char id[RRD_ID_LENGTH_MAX + 1];
            snprintfz(id, RRD_ID_LENGTH_MAX, "w3svc_w3wp_%s_output_cache_memory_usage", app_name);
            netdata_fix_chart_name(id);
            p->st_wescv_w3wp_output_cache_memory_usage = rrdset_create_localhost(
                "iis",
                id,
                NULL,
                "w3svc w3wp",
                "iis.w3svc_w3wp_output_cache_memory_usage",
                "Current number of bytes used by output cache",
                "bytes",
                PLUGIN_WINDOWS_NAME,
                "PerflibWebService",
                PRIO_W3SVC_W3WP_OUTPUT_CACHE_MEMORY_USAGE,
                update_every,
                RRDSET_TYPE_AREA);

            p->rd_wescv_w3wp_output_cache_memory_usage =
                rrddim_add(p->st_wescv_w3wp_output_cache_memory_usage, "used", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
            rrdlabels_add(p->st_wescv_w3wp_output_cache_memory_usage->rrdlabels, "app", app_name, RRDLABEL_SRC_AUTO);
        }

        rrddim_set_by_pointer(
            p->st_wescv_w3wp_output_cache_memory_usage,
            p->rd_wescv_w3wp_output_cache_memory_usage,
            (collected_number)p->WESCVW3WPOutputCacheMemoryUsage.current.Data);

        rrdset_done(p->st_wescv_w3wp_output_cache_memory_usage);
    }
}

static inline void w3svc_w3wp_output_cache_flushed_total(
    struct ws3svc_w3wp_data *p,
    PERF_DATA_BLOCK *pDataBlock,
    PERF_OBJECT_TYPE *pObjectType,
    PERF_INSTANCE_DEFINITION *pi,
    int update_every,
    char *app_name)
{
    if (perflibGetInstanceCounter(pDataBlock, pObjectType, pi, &p->WESCVW3WPOutputCacheFlushesTotal)) {
        if (!p->st_wescv_w3wp_output_cache_flushed_total) {
            char id[RRD_ID_LENGTH_MAX + 1];
            snprintfz(id, RRD_ID_LENGTH_MAX, "w3svc_w3wp_%s_output_cache_flushed_total", app_name);
            netdata_fix_chart_name(id);
            p->st_wescv_w3wp_output_cache_flushed_total = rrdset_create_localhost(
                "iis",
                id,
                NULL,
                "w3svc w3wp",
                "iis.w3svc_w3wp_output_cache_flushed_total",
                "Flushes of output cache",
                "flushes/s",
                PLUGIN_WINDOWS_NAME,
                "PerflibWebService",
                PRIO_W3SVC_W3WP_OUTPUT_CACHE_FLUSHED_TOTAL,
                update_every,
                RRDSET_TYPE_LINE);

            p->rd_wescv_w3wp_output_cache_flushed_total = rrddim_add(
                p->st_wescv_w3wp_output_cache_flushed_total, "output_cache_entries", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
            rrdlabels_add(p->st_wescv_w3wp_output_cache_flushed_total->rrdlabels, "app", app_name, RRDLABEL_SRC_AUTO);
        }

        rrddim_set_by_pointer(
            p->st_wescv_w3wp_output_cache_flushed_total,
            p->rd_wescv_w3wp_output_cache_flushed_total,
            (collected_number)p->WESCVW3WPOutputCacheFlushesTotal.current.Data);

        rrdset_done(p->st_wescv_w3wp_output_cache_flushed_total);
    }
}

static bool do_W3SCV_W3WP(PERF_DATA_BLOCK *pDataBlock, int update_every)
{
    PERF_OBJECT_TYPE *pObjectType = perflibFindObjectTypeByName(pDataBlock, "W3SVC_W3WP");
    if (!pObjectType)
        return false;

    PERF_INSTANCE_DEFINITION *pi = NULL;
    for (LONG i = 0; i < pObjectType->NumInstances; i++) {
        pi = perflibForEachInstance(pDataBlock, pObjectType, pi);
        if (!pi)
            break;

        if (!getInstanceName(pDataBlock, pObjectType, pi, windows_shared_buffer, sizeof(windows_shared_buffer)))
            strncpyz(windows_shared_buffer, "[unknown]", sizeof(windows_shared_buffer) - 1);

        // We are not ploting _Total here, because cloud will group the sites
        if (strcasecmp(windows_shared_buffer, "_Total") == 0) {
            continue;
        }

        struct ws3svc_w3wp_data *p = dictionary_set(w3svc_w3wp_service, windows_shared_buffer, NULL, sizeof(*p));
        // Instance example: "11084_MSExchangeOABAppPool"
        char *app = strchr(windows_shared_buffer, '_');
        if (!app) {
            continue;
        }

        *app++ = '\0';

        w3svc_w3wp_active_threads(p, pDataBlock, pObjectType, pi, update_every, app);

        w3svc_w3wp_requests_total(p, pDataBlock, pObjectType, pi, update_every, app);
        w3svc_w3wp_requests_active(p, pDataBlock, pObjectType, pi, update_every, app);

        w3svc_w3wp_file_cache_mem_usage(p, pDataBlock, pObjectType, pi, update_every, app);

        w3svc_w3wp_files_cached_total(p, pDataBlock, pObjectType, pi, update_every, app);
        w3svc_w3wp_files_flushed_total(p, pDataBlock, pObjectType, pi, update_every, app);

        w3svc_w3wp_uri_cached_flushed(p, pDataBlock, pObjectType, pi, update_every, app);
        w3svc_w3wp_total_uri_cached(p, pDataBlock, pObjectType, pi, update_every, app);

        w3svc_w3wp_total_metadata_cached(p, pDataBlock, pObjectType, pi, update_every, app);
        w3svc_w3wp_total_metadata_flushed(p, pDataBlock, pObjectType, pi, update_every, app);

        w3svc_w3wp_output_cache_active_flushed_items(p, pDataBlock, pObjectType, pi, update_every, app);
        w3svc_w3wp_output_cache_memory_usage(p, pDataBlock, pObjectType, pi, update_every, app);
        w3svc_w3wp_output_cache_flushed_total(p, pDataBlock, pObjectType, pi, update_every, app);
    }

    return true;
}

int do_PerflibWebService(int update_every __maybe_unused, usec_t dt __maybe_unused)
{
    static bool initialized = false;

    if (unlikely(!initialized)) {
        initialize();
        initialized = true;
    }

    int ret = 0;
#define TOTAL_NUMBER_OF_FAILURES (3)
    if (iis_web_service("Web Service", update_every, do_web_services))
        ret++;
    if (iis_web_service("APP_POOL_WAS", update_every, do_app_pool))
        ret++;
    if (iis_web_service("W3SVC_W3WP", update_every, do_W3SCV_W3WP))
        ret++;

    return (ret == TOTAL_NUMBER_OF_FAILURES) ? -1 : 0;
}
