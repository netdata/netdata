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

    RRDSET *st_connections_attemps;
    RRDDIM *rd_connections_attemps;

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

    RRDSET *st_logon_attemps;
    RRDDIM *rd_logon_attemps;

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

static DICTIONARY *web_services = NULL;

static void initialize(void)
{
    web_services = dictionary_create_advanced(
        DICT_OPTION_DONT_OVERWRITE_VALUE | DICT_OPTION_FIXED_SIZE, NULL, sizeof(struct web_service));

    dictionary_register_insert_callback(web_services, dict_web_service_insert_cb, NULL);
}

static bool do_web_services(PERF_DATA_BLOCK *pDataBlock, int update_every)
{
    char id[RRD_ID_LENGTH_MAX + 1];
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

        if (perflibGetInstanceCounter(pDataBlock, pObjectType, pi, &p->IISReceivedBytesTotal) &&
            perflibGetInstanceCounter(pDataBlock, pObjectType, pi, &p->IISSentBytesTotal)) {
            if (!p->st_traffic) {
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
            rrddim_set_by_pointer(
                p->st_traffic, p->rd_traffic_sent, (collected_number)p->IISSentBytesTotal.current.Data);

            rrdset_done(p->st_traffic);
        }

        if (perflibGetInstanceCounter(pDataBlock, pObjectType, pi, &p->IISFilesReceivedTotal) &&
            perflibGetInstanceCounter(pDataBlock, pObjectType, pi, &p->IISFilesSentTotal)) {
            if (!p->st_file_transfer) {
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

                p->rd_files_received =
                    rrddim_add(p->st_file_transfer, "received", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
                p->rd_files_sent = rrddim_add(p->st_file_transfer, "sent", NULL, -1, 1, RRD_ALGORITHM_INCREMENTAL);
                rrdlabels_add(p->st_file_transfer->rrdlabels, "website", windows_shared_buffer, RRDLABEL_SRC_AUTO);
            }

            rrddim_set_by_pointer(
                p->st_file_transfer, p->rd_files_received, (collected_number)p->IISFilesReceivedTotal.current.Data);
            rrddim_set_by_pointer(
                p->st_file_transfer, p->rd_files_sent, (collected_number)p->IISFilesSentTotal.current.Data);

            rrdset_done(p->st_file_transfer);
        }

        if (perflibGetInstanceCounter(pDataBlock, pObjectType, pi, &p->IISCurrentConnections)) {
            if (!p->st_curr_connections) {
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

                p->rd_curr_connections =
                    rrddim_add(p->st_curr_connections, "active", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
                rrdlabels_add(p->st_curr_connections->rrdlabels, "website", windows_shared_buffer, RRDLABEL_SRC_AUTO);
            }

            rrddim_set_by_pointer(
                p->st_curr_connections,
                p->rd_curr_connections,
                (collected_number)p->IISCurrentConnections.current.Data);

            rrdset_done(p->st_curr_connections);
        }

        if (perflibGetInstanceCounter(pDataBlock, pObjectType, pi, &p->IISConnAttemptsAllInstancesTotal)) {
            if (!p->st_connections_attemps) {
                snprintfz(id, RRD_ID_LENGTH_MAX, "website_%s_connection_attempts_rate", windows_shared_buffer);
                netdata_fix_chart_name(id);
                p->st_connections_attemps = rrdset_create_localhost(
                    "iis",
                    id,
                    NULL,
                    "connections",
                    "iis.website_connection_attempts_rate",
                    "Website connections attempts",
                    "attempts/s",
                    PLUGIN_WINDOWS_NAME,
                    "PerflibWebService",
                    PRIO_WEBSITE_IIS_CONNECTIONS_ATTEMP,
                    update_every,
                    RRDSET_TYPE_LINE);

                p->rd_connections_attemps =
                    rrddim_add(p->st_connections_attemps, "connection", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);

                rrdlabels_add(
                    p->st_connections_attemps->rrdlabels, "website", windows_shared_buffer, RRDLABEL_SRC_AUTO);
            }

            rrddim_set_by_pointer(
                p->st_connections_attemps,
                p->rd_connections_attemps,
                (collected_number)p->IISCurrentConnections.current.Data);

            rrdset_done(p->st_connections_attemps);
        }

        if (perflibGetInstanceCounter(pDataBlock, pObjectType, pi, &p->IISCurrentAnonymousUser) &&
            perflibGetInstanceCounter(pDataBlock, pObjectType, pi, &p->IISCurrentNonAnonymousUsers)) {
            if (!p->st_user_count) {
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
                p->rd_user_nonanonymous =
                    rrddim_add(p->st_user_count, "non_anonymous", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);

                rrdlabels_add(p->st_user_count->rrdlabels, "website", windows_shared_buffer, RRDLABEL_SRC_AUTO);
            }

            rrddim_set_by_pointer(
                p->st_user_count, p->rd_user_anonymous, (collected_number)p->IISCurrentAnonymousUser.current.Data);

            rrddim_set_by_pointer(
                p->st_user_count,
                p->rd_user_nonanonymous,
                (collected_number)p->IISCurrentNonAnonymousUsers.current.Data);

            rrdset_done(p->st_user_count);
        }

        if (perflibGetInstanceCounter(pDataBlock, pObjectType, pi, &p->IISCurrentISAPIExtRequests)) {
            if (!p->st_isapi_extension_request_count) {
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

        if (perflibGetInstanceCounter(pDataBlock, pObjectType, pi, &p->IISIPAPIExtRequestsTotal)) {
            if (!p->st_isapi_extension_request_rate) {
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

        if (perflibGetInstanceCounter(pDataBlock, pObjectType, pi, &p->IISLockedErrorsTotal) &&
            perflibGetInstanceCounter(pDataBlock, pObjectType, pi, &p->IISNotFoundErrorsTotal)) {
            if (!p->st_error_rate) {
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

        if (perflibGetInstanceCounter(pDataBlock, pObjectType, pi, &p->IISLogonAttemptsTotal)) {
            if (!p->st_logon_attemps) {
                snprintfz(id, RRD_ID_LENGTH_MAX, "website_%s_logon_attempts_rate", windows_shared_buffer);
                netdata_fix_chart_name(id);
                p->st_logon_attemps = rrdset_create_localhost(
                    "iis",
                    id,
                    NULL,
                    "logon",
                    "iis.website_logon_attempts_rate",
                    "Website logon attempts",
                    "attempts/s",
                    PLUGIN_WINDOWS_NAME,
                    "PerflibWebService",
                    PRIO_WEBSITE_IIS_LOGON_ATTEMPTS,
                    update_every,
                    RRDSET_TYPE_LINE);

                p->rd_logon_attemps = rrddim_add(p->st_logon_attemps, "logon", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);

                rrdlabels_add(p->st_logon_attemps->rrdlabels, "website", windows_shared_buffer, RRDLABEL_SRC_AUTO);
            }

            rrddim_set_by_pointer(
                p->st_logon_attemps, p->rd_logon_attemps, (collected_number)p->IISLogonAttemptsTotal.current.Data);

            rrdset_done(p->st_logon_attemps);
        }

        if (perflibGetInstanceCounter(pDataBlock, pObjectType, pi, &p->IISUptime)) {
            if (!p->st_service_uptime) {
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

            rrddim_set_by_pointer(
                p->st_service_uptime, p->rd_service_uptime, (collected_number)p->IISUptime.current.Data);

            rrdset_done(p->st_service_uptime);
        }

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
                p->IISRequestsProppatch.current.Data + p->IISRequestsSearch.current.Data +
                p->IISRequestsLock.current.Data + p->IISRequestsUnlock.current.Data + p->IISRequestsOther.current.Data;

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

                rrdlabels_add(
                    p->st_request_by_type_rate->rrdlabels, "website", windows_shared_buffer, RRDLABEL_SRC_AUTO);
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
                p->st_request_by_type_rate,
                p->rd_request_delete_rate,
                (collected_number)p->IISRequestsDelete.current.Data);
            rrddim_set_by_pointer(
                p->st_request_by_type_rate,
                p->rd_request_trace_rate,
                (collected_number)p->IISRequestsTrace.current.Data);
            rrddim_set_by_pointer(
                p->st_request_by_type_rate, p->rd_request_move_rate, (collected_number)p->IISRequestsMove.current.Data);
            rrddim_set_by_pointer(
                p->st_request_by_type_rate, p->rd_request_copy_rate, (collected_number)p->IISRequestsCopy.current.Data);
            rrddim_set_by_pointer(
                p->st_request_by_type_rate,
                p->rd_request_mkcol_rate,
                (collected_number)p->IISRequestsMkcol.current.Data);
            rrddim_set_by_pointer(
                p->st_request_by_type_rate,
                p->rd_request_propfind_rate,
                (collected_number)p->IISRequestsPropfind.current.Data);
            rrddim_set_by_pointer(
                p->st_request_by_type_rate,
                p->rd_request_proppatch_rate,
                (collected_number)p->IISRequestsProppatch.current.Data);
            rrddim_set_by_pointer(
                p->st_request_by_type_rate,
                p->rd_request_search_rate,
                (collected_number)p->IISRequestsSearch.current.Data);
            rrddim_set_by_pointer(
                p->st_request_by_type_rate, p->rd_request_lock_rate, (collected_number)p->IISRequestsLock.current.Data);
            rrddim_set_by_pointer(
                p->st_request_by_type_rate,
                p->rd_request_unlock_rate,
                (collected_number)p->IISRequestsUnlock.current.Data);
            rrddim_set_by_pointer(
                p->st_request_by_type_rate,
                p->rd_request_other_rate,
                (collected_number)p->IISRequestsOther.current.Data);

            rrdset_done(p->st_request_by_type_rate);
        }
    }

    return true;
}

int do_PerflibWebService(int update_every, usec_t dt __maybe_unused)
{
    static bool initialized = false;

    if (unlikely(!initialized)) {
        initialize();
        initialized = true;
    }

    DWORD id = RegistryFindIDByName("Web Service");
    if (id == PERFLIB_REGISTRY_NAME_NOT_FOUND)
        return -1;

    PERF_DATA_BLOCK *pDataBlock = perflibGetPerformanceData(id);
    if (!pDataBlock)
        return -1;

    do_web_services(pDataBlock, update_every);

    return 0;
}
