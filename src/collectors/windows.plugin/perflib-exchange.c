// SPDX-License-Identifier: GPL-3.0-or-later

#include "windows_plugin.h"
#include "windows-internals.h"

struct exchange_proxy {
    RRDSET *st_exchange_http_proxy_avg_auth_latency;
    RRDSET *st_exchange_http_proxy_avg_cas_processing_latency;
    RRDSET *st_exchange_http_proxy_mailbox_proxy_failure;
    RRDSET *st_exchange_http_proxy_server_location_avg_latency;
    RRDSET *st_exchange_http_proxy_outstanding_proxy_requests;
    RRDSET *st_exchange_http_proxy_requests_total;

    RRDDIM *rd_exchange_http_proxy_avg_auth_latency;
    RRDDIM *rd_exchange_http_proxy_avg_cas_processing_latency;
    RRDDIM *rd_exchange_http_proxy_mailbox_proxy_failure;
    RRDDIM *rd_exchange_http_proxy_server_location_avg_latency;
    RRDDIM *rd_exchange_http_proxy_outstanding_proxy_requests;
    RRDDIM *rd_exchange_http_proxy_requests_total;

    COUNTER_DATA exchangeProxyAvgAuthLatency;
    COUNTER_DATA exchangeProxyAvgCasProcessingLatency;
    COUNTER_DATA exchangeProxyMailboxProxyFailure;
    COUNTER_DATA exchangeProxyMailboxServerLocator;
    COUNTER_DATA exchangeProxyOutstandingProxy;
    COUNTER_DATA exchangeProxyRequestsTotal;
};

struct exchange_workload {
    RRDSET *st_exchange_workload_active_tasks;
    RRDSET *st_exchange_workload_complete_tasks;
    RRDSET *st_exchange_workload_queued_tasks;
    RRDSET *st_exchange_workload_yielded_tasks;
    RRDSET *st_exchange_workload_activity_status;

    RRDDIM *rd_exchange_workload_active_tasks;
    RRDDIM *rd_exchange_workload_complete_tasks;
    RRDDIM *rd_exchange_workload_queued_tasks;
    RRDDIM *rd_exchange_workload_yielded_tasks;
    RRDDIM *rd_exchange_workload_activity_active_status;
    RRDDIM *rd_exchange_workload_activity_paused_status;

    COUNTER_DATA exchangeWorkloadActiveTasks;
    COUNTER_DATA exchangeWorkloadCompleteTasks;
    COUNTER_DATA exchangeWorkloadQueueTasks;
    COUNTER_DATA exchangeWorkloadYieldedTasks;
    COUNTER_DATA exchangeWorkloadActivityStatus;
};

struct exchange_queue {
    RRDSET *st_exchange_queue_active_mailbox;
    RRDSET *st_exchange_queue_external_active_remote_delivery;
    RRDSET *st_exchange_queue_internal_active_remote_delivery;
    RRDSET *st_exchange_queue_unreachable;
    RRDSET *st_exchange_queue_poison;

    RRDDIM *rd_exchange_queue_active_mailbox;
    RRDDIM *rd_exchange_queue_external_active_remote_delivery;
    RRDDIM *rd_exchange_queue_internal_active_remote_delivery;
    RRDDIM *rd_exchange_queue_unreachable;
    RRDDIM *rd_exchange_queue_poison;

    COUNTER_DATA exchangeTransportQueuesActiveMailboxDelivery;
    COUNTER_DATA exchangeTransportQueuesExternalActiveRemoteDelivery;
    COUNTER_DATA exchangeTransportQueuesInternalActiveRemoteDelivery;
    COUNTER_DATA exchangeTransportQueuesUnreachable;
    COUNTER_DATA exchangeTransportQueuesPoison;
};

DICTIONARY *exchange_proxies;
DICTIONARY *exchange_workloads;
DICTIONARY *exchange_queues;

static void exchange_proxy_initialize_variables(struct exchange_proxy *ep)
{
    ep->exchangeProxyAvgAuthLatency.key = "Average Authentication Latency";
    ep->exchangeProxyAvgCasProcessingLatency.key = "Average ClientAccess Server Processing Latency";
    ep->exchangeProxyMailboxProxyFailure.key = "Mailbox Server Proxy Failure Rate";
    ep->exchangeProxyMailboxServerLocator.key = "MailboxServerLocator Average Latency (Moving Average)";
    ep->exchangeProxyOutstandingProxy.key = "Outstanding Proxy Requests";
    ep->exchangeProxyRequestsTotal.key = "Proxy Requests/Sec";
}

static void
dict_exchange_insert_proxy_cb(const DICTIONARY_ITEM *item __maybe_unused, void *value, void *data __maybe_unused)
{
    struct exchange_proxy *ep = value;

    exchange_proxy_initialize_variables(ep);
}

static void exchange_workload_initialize_variables(struct exchange_workload *ew)
{
    ew->exchangeWorkloadActiveTasks.key = "ActiveTasks";
    ew->exchangeWorkloadCompleteTasks.key = "CompletedTasks";
    ew->exchangeWorkloadQueueTasks.key = "QueuedTasks";
    ew->exchangeWorkloadYieldedTasks.key = "YieldedTasks";
    ew->exchangeWorkloadActivityStatus.key = "Active";
}

static void
dict_exchange_insert_workload_cb(const DICTIONARY_ITEM *item __maybe_unused, void *value, void *data __maybe_unused)
{
    struct exchange_workload *ew = value;

    exchange_workload_initialize_variables(ew);
}

static void exchange_queue_initialize_variables(struct exchange_queue *eq)
{
    eq->exchangeTransportQueuesActiveMailboxDelivery.key = "Active Mailbox Delivery Queue Length";
    eq->exchangeTransportQueuesExternalActiveRemoteDelivery.key = "External Active Remote Delivery Queue Length";
    eq->exchangeTransportQueuesInternalActiveRemoteDelivery.key = "Internal Active Remote Delivery Queue Length";
    eq->exchangeTransportQueuesPoison.key = "Poison Queue Length";
    eq->exchangeTransportQueuesUnreachable.key = "Unreachable Queue Length";
}

static void
dict_exchange_insert_queue_cb(const DICTIONARY_ITEM *item __maybe_unused, void *value, void *data __maybe_unused)
{
    struct exchange_queue *eq = value;

    exchange_queue_initialize_variables(eq);
}

static void initialize(void)
{
    exchange_proxies = dictionary_create_advanced(
        DICT_OPTION_DONT_OVERWRITE_VALUE | DICT_OPTION_FIXED_SIZE, NULL, sizeof(struct exchange_proxy));
    dictionary_register_insert_callback(exchange_proxies, dict_exchange_insert_proxy_cb, NULL);

    exchange_workloads = dictionary_create_advanced(
        DICT_OPTION_DONT_OVERWRITE_VALUE | DICT_OPTION_FIXED_SIZE, NULL, sizeof(struct exchange_workload));
    dictionary_register_insert_callback(exchange_workloads, dict_exchange_insert_workload_cb, NULL);

    exchange_queues = dictionary_create_advanced(
        DICT_OPTION_DONT_OVERWRITE_VALUE | DICT_OPTION_FIXED_SIZE, NULL, sizeof(struct exchange_queue));
    dictionary_register_insert_callback(exchange_queues, dict_exchange_insert_queue_cb, NULL);
}

static void netdata_exchange_owa_current_unique_users(COUNTER_DATA *value, int update_every)
{
    static RRDSET *st_exchange_owa_unique_users = NULL;
    static RRDDIM *rd_exchange_owa_unique_users = NULL;

    if (unlikely(!st_exchange_owa_unique_users)) {
        st_exchange_owa_unique_users = rrdset_create_localhost(
            "exchange",
            "owa_current_unique_users",
            NULL,
            "owa",
            "exchange.owa_current_unique_users",
            "Unique users currently logged on to Outlook Web App",
            "users",
            PLUGIN_WINDOWS_NAME,
            "PerflibExchange",
            PRIO_EXCHANGE_OWA_UNIQUE_USERS,
            update_every,
            RRDSET_TYPE_LINE);

        rd_exchange_owa_unique_users =
            rrddim_add(st_exchange_owa_unique_users, "logged-in", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
    }

    rrddim_set_by_pointer(
        st_exchange_owa_unique_users, rd_exchange_owa_unique_users, (collected_number)value->current.Data);
    rrdset_done(st_exchange_owa_unique_users);
}

static void netdata_exchange_owa_request_total(COUNTER_DATA *value, int update_every)
{
    static RRDSET *st_exchange_owa_request_total = NULL;
    static RRDDIM *rd_exchange_owa_request_total = NULL;

    if (unlikely(!st_exchange_owa_request_total)) {
        st_exchange_owa_request_total = rrdset_create_localhost(
            "exchange",
            "owa_requests_total",
            NULL,
            "owa",
            "exchange.owa_requests_total",
            "Requests handled by Outlook Web App.",
            "requests/s",
            PLUGIN_WINDOWS_NAME,
            "PerflibExchange",
            PRIO_EXCHANGE_OWA_REQUESTS_TOTAL,
            update_every,
            RRDSET_TYPE_LINE);

        rd_exchange_owa_request_total =
            rrddim_add(st_exchange_owa_request_total, "handled", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
    }

    rrddim_set_by_pointer(
        st_exchange_owa_request_total, rd_exchange_owa_request_total, (collected_number)value->current.Data);
    rrdset_done(st_exchange_owa_request_total);
}

static void netdata_exchange_owa(PERF_DATA_BLOCK *pDataBlock, PERF_OBJECT_TYPE *pObjectType, int update_every)
{
    static COUNTER_DATA exchangeOWACurrentUniqueUser = {.key = "Current Unique Users"};
    static COUNTER_DATA exchangeOWARequestsTotal = {.key = "Requests/sec"};

    if (perflibGetObjectCounter(pDataBlock, pObjectType, &exchangeOWACurrentUniqueUser))
        netdata_exchange_owa_current_unique_users(&exchangeOWACurrentUniqueUser, update_every);

    if (perflibGetObjectCounter(pDataBlock, pObjectType, &exchangeOWARequestsTotal))
        netdata_exchange_owa_request_total(&exchangeOWARequestsTotal, update_every);
}

static void netdata_exchange_active_ping_cmd(COUNTER_DATA *value, int update_every)
{
    static RRDSET *st_exchange_active_ping_cmds = NULL;
    static RRDDIM *rd_exchange_active_ping_cmds = NULL;

    if (unlikely(!st_exchange_active_ping_cmds)) {
        st_exchange_active_ping_cmds = rrdset_create_localhost(
            "exchange",
            "activesync_ping_cmds_pending",
            NULL,
            "sync",
            "exchange.activesync_ping_cmds_pending",
            "Ping commands pending in queue.",
            "commands",
            PLUGIN_WINDOWS_NAME,
            "PerflibExchange",
            PRIO_EXCHANGE_ACTIVE_SYNC_PING_CMDS_PENDING,
            update_every,
            RRDSET_TYPE_LINE);

        rd_exchange_active_ping_cmds =
            rrddim_add(st_exchange_active_ping_cmds, "pending", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
    }

    rrddim_set_by_pointer(
        st_exchange_active_ping_cmds, rd_exchange_active_ping_cmds, (collected_number)value->current.Data);
    rrdset_done(st_exchange_active_ping_cmds);
}

static void netdata_exchange_active_requests(COUNTER_DATA *value, int update_every)
{
    static RRDSET *st_exchange_received_requests = NULL;
    static RRDDIM *rd_exchange_received_requests = NULL;

    if (unlikely(!st_exchange_received_requests)) {
        st_exchange_received_requests = rrdset_create_localhost(
            "exchange",
            "activesync_requests",
            NULL,
            "sync",
            "exchange.activesync_requests",
            "HTTP requests received from ASP.NET.",
            "requests/s",
            PLUGIN_WINDOWS_NAME,
            "PerflibExchange",
            PRIO_EXCHANGE_ACTIVE_SYNC_REQUESTS,
            update_every,
            RRDSET_TYPE_LINE);

        rd_exchange_received_requests =
            rrddim_add(st_exchange_received_requests, "received", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
    }

    rrddim_set_by_pointer(
        st_exchange_received_requests, rd_exchange_received_requests, (collected_number)value->current.Data);
    rrdset_done(st_exchange_received_requests);
}

static void netdata_exchange_sync_cmds(COUNTER_DATA *value, int update_every)
{
    static RRDSET *st_exchange_sync_cmds = NULL;
    static RRDDIM *rd_exchange_sync_cmds = NULL;

    if (unlikely(!st_exchange_sync_cmds)) {
        st_exchange_sync_cmds = rrdset_create_localhost(
            "exchange",
            "activesync_sync_cmds",
            NULL,
            "sync",
            "exchange.activesync_sync_cmds",
            "Sync commands processed.",
            "commands/s",
            PLUGIN_WINDOWS_NAME,
            "PerflibExchange",
            PRIO_EXCHANGE_ACTIVE_SYNC_CMDS,
            update_every,
            RRDSET_TYPE_LINE);

        rd_exchange_sync_cmds = rrddim_add(st_exchange_sync_cmds, "processed", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
    }

    rrddim_set_by_pointer(st_exchange_sync_cmds, rd_exchange_sync_cmds, (collected_number)value->current.Data);
    rrdset_done(st_exchange_sync_cmds);
}

static void netdata_exchange_active_sync(PERF_DATA_BLOCK *pDataBlock, PERF_OBJECT_TYPE *pObjectType, int update_every)
{
    static COUNTER_DATA exchangePingCommands = {.key = "Ping Commands Pending"};
    static COUNTER_DATA exchangeSyncCommands = {.key = "Sync Commands/sec"};
    static COUNTER_DATA exchangeActiveRequests = {.key = "Requests/sec"};

    if (perflibGetObjectCounter(pDataBlock, pObjectType, &exchangePingCommands))
        netdata_exchange_active_ping_cmd(&exchangePingCommands, update_every);

    if (perflibGetObjectCounter(pDataBlock, pObjectType, &exchangeSyncCommands))
        netdata_exchange_sync_cmds(&exchangeSyncCommands, update_every);

    if (perflibGetObjectCounter(pDataBlock, pObjectType, &exchangeActiveRequests))
        netdata_exchange_active_requests(&exchangeActiveRequests, update_every);
}

static void netdata_exchange_auto_discover(PERF_DATA_BLOCK *pDataBlock, PERF_OBJECT_TYPE *pObjectType, int update_every)
{
    static COUNTER_DATA exchangeAutoDiscoverRequestsTotal = {.key = "Requests/sec"};

    static RRDSET *st_exchange_auto_discover_request_total = NULL;
    static RRDDIM *rd_exchange_auto_discover_request_total = NULL;

    if (!perflibGetObjectCounter(pDataBlock, pObjectType, &exchangeAutoDiscoverRequestsTotal)) {
        return;
    }

    if (unlikely(!st_exchange_auto_discover_request_total)) {
        st_exchange_auto_discover_request_total = rrdset_create_localhost(
            "exchange",
            "autodiscover_requests",
            NULL,
            "requests",
            "exchange.autodiscover_requests",
            "Autodiscover service requests processed.",
            "requests/s",
            PLUGIN_WINDOWS_NAME,
            "PerflibExchange",
            PRIO_EXCHANGE_AUTO_DISCOVER_REQUESTS,
            update_every,
            RRDSET_TYPE_LINE);

        rd_exchange_auto_discover_request_total =
            rrddim_add(st_exchange_auto_discover_request_total, "processed", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
    }

    rrddim_set_by_pointer(
        st_exchange_auto_discover_request_total,
        rd_exchange_auto_discover_request_total,
        (collected_number)exchangeAutoDiscoverRequestsTotal.current.Data);
    rrdset_done(st_exchange_auto_discover_request_total);
}

static void
netdata_exchange_availability_service(PERF_DATA_BLOCK *pDataBlock, PERF_OBJECT_TYPE *pObjectType, int update_every)
{
    static COUNTER_DATA exchangeAvailServiceRequests = {.key = "Availability Requests (sec)"};

    static RRDSET *st_exchange_avail_service_requests = NULL;
    static RRDDIM *rd_exchange_avail_service_requests = NULL;

    if (!perflibGetObjectCounter(pDataBlock, pObjectType, &exchangeAvailServiceRequests)) {
        return;
    }

    if (unlikely(!st_exchange_avail_service_requests)) {
        st_exchange_avail_service_requests = rrdset_create_localhost(
            "exchange",
            "avail_service_requests",
            NULL,
            "requests",
            "exchange.avail_service_requests",
            "Requests serviced.",
            "requests/s",
            PLUGIN_WINDOWS_NAME,
            "PerflibExchange",
            PRIO_EXCHANGE_AUTO_AVAILABILITY_SERVICES,
            update_every,
            RRDSET_TYPE_LINE);

        rd_exchange_avail_service_requests =
            rrddim_add(st_exchange_avail_service_requests, "serviced", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
    }

    rrddim_set_by_pointer(
        st_exchange_avail_service_requests,
        rd_exchange_avail_service_requests,
        (collected_number)exchangeAvailServiceRequests.current.Data);
    rrdset_done(st_exchange_avail_service_requests);
}

static void netdata_exchange_rpc_avg_latency(COUNTER_DATA *value, int update_every)
{
    static RRDSET *st_exchange_rpc_avg_latency = NULL;
    static RRDDIM *rd_exchange_rpc_avg_latency = NULL;

    if (unlikely(!st_exchange_rpc_avg_latency)) {
        st_exchange_rpc_avg_latency = rrdset_create_localhost(
            "exchange",
            "rpc_avg_latency",
            NULL,
            "rpc",
            "exchange.rpc_avg_latency",
            "Average latency.",
            "seconds",
            PLUGIN_WINDOWS_NAME,
            "PerflibExchange",
            PRIO_EXCHANGE_RPC_AVG_LATENCY,
            update_every,
            RRDSET_TYPE_LINE);

        rd_exchange_rpc_avg_latency =
            rrddim_add(st_exchange_rpc_avg_latency, "latency", NULL, 1, 1000, RRD_ALGORITHM_ABSOLUTE);
    }

    rrddim_set_by_pointer(
        st_exchange_rpc_avg_latency, rd_exchange_rpc_avg_latency, (collected_number)value->current.Data);
    rrdset_done(st_exchange_rpc_avg_latency);
}

static void netdata_exchange_rpc_requests(COUNTER_DATA *value, int update_every)
{
    static RRDSET *st_exchange_rpc_requests = NULL;
    static RRDDIM *rd_exchange_rpc_requests = NULL;

    if (unlikely(!st_exchange_rpc_requests)) {
        st_exchange_rpc_requests = rrdset_create_localhost(
            "exchange",
            "rpc_requests_total",
            NULL,
            "rpc",
            "exchange.rpc_requests",
            "Clients requests currently being processed.",
            "requests",
            PLUGIN_WINDOWS_NAME,
            "PerflibExchange",
            PRIO_EXCHANGE_RPC_REQUESTS,
            update_every,
            RRDSET_TYPE_LINE);

        rd_exchange_rpc_requests =
            rrddim_add(st_exchange_rpc_requests, "processed", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
    }

    rrddim_set_by_pointer(st_exchange_rpc_requests, rd_exchange_rpc_requests, (collected_number)value->current.Data);
    rrdset_done(st_exchange_rpc_requests);
}

static void netdata_exchange_rpc_active_user_count(COUNTER_DATA *value, int update_every)
{
    static RRDSET *st_exchange_active_user_account = NULL;
    static RRDDIM *rd_exchange_active_user_account = NULL;

    if (unlikely(!st_exchange_active_user_account)) {
        st_exchange_active_user_account = rrdset_create_localhost(
            "exchange",
            "rpc_active_user",
            NULL,
            "rpc",
            "exchange.rpc_active_user",
            "Active unique users in the last 2 minutes.",
            "users",
            PLUGIN_WINDOWS_NAME,
            "PerflibExchange",
            PRIO_EXCHANGE_RPC_ACTIVE_USERS_COUNT,
            update_every,
            RRDSET_TYPE_LINE);

        rd_exchange_active_user_account =
            rrddim_add(st_exchange_active_user_account, "active", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
    }

    rrddim_set_by_pointer(
        st_exchange_active_user_account, rd_exchange_active_user_account, (collected_number)value->current.Data);
    rrdset_done(st_exchange_active_user_account);
}

static void netdata_exchange_rpc_connection_count(COUNTER_DATA *value, int update_every)
{
    static RRDSET *st_exchange_rpc_connection_count = NULL;
    static RRDDIM *rd_exchange_rpc_connection_count = NULL;

    if (unlikely(!st_exchange_rpc_connection_count)) {
        st_exchange_rpc_connection_count = rrdset_create_localhost(
            "exchange",
            "rpc_connection",
            NULL,
            "rpc",
            "exchange.rpc_connection",
            "Client connections.",
            "connections",
            PLUGIN_WINDOWS_NAME,
            "PerflibExchange",
            PRIO_EXCHANGE_RPC_CONNECTION_COUNT,
            update_every,
            RRDSET_TYPE_LINE);

        rd_exchange_rpc_connection_count =
            rrddim_add(st_exchange_rpc_connection_count, "connections", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
    }

    rrddim_set_by_pointer(
        st_exchange_rpc_connection_count, rd_exchange_rpc_connection_count, (collected_number)value->current.Data);
    rrdset_done(st_exchange_rpc_connection_count);
}

static void netdata_exchange_rpc_op_per_sec(COUNTER_DATA *value, int update_every)
{
    static RRDSET *st_exchange_rpc_opc_per_sec = NULL;
    static RRDDIM *rd_exchange_rpc_opc_per_sec = NULL;

    if (unlikely(!st_exchange_rpc_opc_per_sec)) {
        st_exchange_rpc_opc_per_sec = rrdset_create_localhost(
            "exchange",
            "rpc_operations",
            NULL,
            "rpc",
            "exchange.rpc_operations",
            "RPC operations.",
            "operations/s",
            PLUGIN_WINDOWS_NAME,
            "PerflibExchange",
            PRIO_EXCHANGE_RPC_OPERATIONS_TOTAL,
            update_every,
            RRDSET_TYPE_LINE);

        rd_exchange_rpc_opc_per_sec =
            rrddim_add(st_exchange_rpc_opc_per_sec, "operations", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
    }

    rrddim_set_by_pointer(
        st_exchange_rpc_opc_per_sec, rd_exchange_rpc_opc_per_sec, (collected_number)value->current.Data);
    rrdset_done(st_exchange_rpc_opc_per_sec);
}

static void netdata_exchange_rpc_user_count(COUNTER_DATA *value, int update_every)
{
    static RRDSET *st_exchange_rpc_user_count = NULL;
    static RRDDIM *rd_exchange_rpc_user_count = NULL;

    if (unlikely(!st_exchange_rpc_user_count)) {
        st_exchange_rpc_user_count = rrdset_create_localhost(
            "exchange",
            "rpc_user",
            NULL,
            "rpc",
            "exchange.rpc_user_count",
            "RPC users.",
            "users",
            PLUGIN_WINDOWS_NAME,
            "PerflibExchange",
            PRIO_EXCHANGE_RPC_USER_COUNT,
            update_every,
            RRDSET_TYPE_LINE);

        rd_exchange_rpc_user_count =
            rrddim_add(st_exchange_rpc_user_count, "users", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
    }

    rrddim_set_by_pointer(
        st_exchange_rpc_user_count, rd_exchange_rpc_user_count, (collected_number)value->current.Data);
    rrdset_done(st_exchange_rpc_user_count);
}

static void netdata_exchange_rpc(PERF_DATA_BLOCK *pDataBlock, PERF_OBJECT_TYPE *pObjectType, int update_every)
{
    static COUNTER_DATA exchangeRPCAveragedLatency = {.key = "RPC Averaged Latency"};
    static COUNTER_DATA exchangeRPCRequest = {.key = "RPC Requests"};
    static COUNTER_DATA exchangeRPCActiveUserCount = {.key = "Active User Count"};
    static COUNTER_DATA exchangeRPCConnectionCount = {.key = "Connection Count"};
    static COUNTER_DATA exchangeRPCOperationPerSec = {.key = "RPC Operations/sec"};
    static COUNTER_DATA exchangeRPCUserCount = {.key = "User Count"};

    if (perflibGetObjectCounter(pDataBlock, pObjectType, &exchangeRPCAveragedLatency))
        netdata_exchange_rpc_avg_latency(&exchangeRPCAveragedLatency, update_every);

    if (perflibGetObjectCounter(pDataBlock, pObjectType, &exchangeRPCRequest))
        netdata_exchange_rpc_requests(&exchangeRPCRequest, update_every);

    if (perflibGetObjectCounter(pDataBlock, pObjectType, &exchangeRPCActiveUserCount))
        netdata_exchange_rpc_active_user_count(&exchangeRPCActiveUserCount, update_every);

    if (perflibGetObjectCounter(pDataBlock, pObjectType, &exchangeRPCConnectionCount))
        netdata_exchange_rpc_connection_count(&exchangeRPCConnectionCount, update_every);

    if (perflibGetObjectCounter(pDataBlock, pObjectType, &exchangeRPCOperationPerSec))
        netdata_exchange_rpc_op_per_sec(&exchangeRPCOperationPerSec, update_every);

    if (perflibGetObjectCounter(pDataBlock, pObjectType, &exchangeRPCUserCount))
        netdata_exchange_rpc_user_count(&exchangeRPCUserCount, update_every);
}

static void netdata_exchange_proxy_avg_auth_latency(struct exchange_proxy *ep, char *proxy, int update_every)
{
    if (unlikely(!ep->st_exchange_http_proxy_avg_auth_latency)) {
        char id[RRD_ID_LENGTH_MAX + 1];
        snprintfz(id, RRD_ID_LENGTH_MAX, "exchange_proxy_%s_avg_auth_latency", proxy);

        ep->st_exchange_http_proxy_avg_auth_latency = rrdset_create_localhost(
            "exchange",
            id,
            NULL,
            "proxy",
            "exchange.http_proxy_avg_auth_latency",
            "Average time spent authenticating CAS requests.",
            "seconds",
            PLUGIN_WINDOWS_NAME,
            "PerflibExchange",
            PRIO_EXCHANGE_PROXY_AVG_AUTH_LATENCY,
            update_every,
            RRDSET_TYPE_LINE);

        ep->rd_exchange_http_proxy_avg_auth_latency =
            rrddim_add(ep->st_exchange_http_proxy_avg_auth_latency, "latency", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);

        rrdlabels_add(ep->st_exchange_http_proxy_avg_auth_latency->rrdlabels, "http_proxy", proxy, RRDLABEL_SRC_AUTO);
    }

    rrddim_set_by_pointer(
        ep->st_exchange_http_proxy_avg_auth_latency,
        ep->rd_exchange_http_proxy_avg_auth_latency,
        (collected_number)ep->exchangeProxyAvgAuthLatency.current.Data);
    rrdset_done(ep->st_exchange_http_proxy_avg_auth_latency);
}

static void netdata_exchange_proxy_avg_cas_processing_latency(struct exchange_proxy *ep, char *proxy, int update_every)
{
    if (unlikely(!ep->st_exchange_http_proxy_avg_cas_processing_latency)) {
        char id[RRD_ID_LENGTH_MAX + 1];
        snprintfz(id, RRD_ID_LENGTH_MAX, "exchange_proxy_%s_avg_cas_processing_latency_sec", proxy);

        ep->st_exchange_http_proxy_avg_cas_processing_latency = rrdset_create_localhost(
            "exchange",
            id,
            NULL,
            "proxy",
            "exchange.http_proxy_avg_cas_processing_latency_sec",
            "Average latency (sec) of CAS processing time.",
            "seconds",
            PLUGIN_WINDOWS_NAME,
            "PerflibExchange",
            PRIO_EXCHANGE_PROXY_AVG_CAS_PROCESSING_LATENCY,
            update_every,
            RRDSET_TYPE_LINE);

        ep->rd_exchange_http_proxy_avg_cas_processing_latency = rrddim_add(
            ep->st_exchange_http_proxy_avg_cas_processing_latency, "latency", NULL, 1, 1000, RRD_ALGORITHM_ABSOLUTE);

        rrdlabels_add(
            ep->st_exchange_http_proxy_avg_cas_processing_latency->rrdlabels, "http_proxy", proxy, RRDLABEL_SRC_AUTO);
    }

    rrddim_set_by_pointer(
        ep->st_exchange_http_proxy_avg_cas_processing_latency,
        ep->rd_exchange_http_proxy_avg_cas_processing_latency,
        (collected_number)ep->exchangeProxyAvgCasProcessingLatency.current.Data);
    rrdset_done(ep->st_exchange_http_proxy_avg_cas_processing_latency);
}

static void netdata_exchange_proxy_mailbox_proxy_failure(struct exchange_proxy *ep, char *proxy, int update_every)
{
    if (unlikely(!ep->st_exchange_http_proxy_mailbox_proxy_failure)) {
        char id[RRD_ID_LENGTH_MAX + 1];
        snprintfz(id, RRD_ID_LENGTH_MAX, "exchange_proxy_%s_mailbox_proxy_failure_rate", proxy);

        ep->st_exchange_http_proxy_mailbox_proxy_failure = rrdset_create_localhost(
            "exchange",
            id,
            NULL,
            "proxy",
            "exchange.http_proxy_mailbox_proxy_failure_rate",
            "Percentage of failures between this CAS and MBX servers.",
            "percentage",
            PLUGIN_WINDOWS_NAME,
            "PerflibExchange",
            PRIO_EXCHANGE_PROXY_MAILBOX_PROXY_FAILURE,
            update_every,
            RRDSET_TYPE_LINE);

        ep->rd_exchange_http_proxy_mailbox_proxy_failure = rrddim_add(
            ep->st_exchange_http_proxy_mailbox_proxy_failure, "failures", NULL, 1, 1000, RRD_ALGORITHM_ABSOLUTE);

        rrdlabels_add(
            ep->st_exchange_http_proxy_mailbox_proxy_failure->rrdlabels, "http_proxy", proxy, RRDLABEL_SRC_AUTO);
    }

    rrddim_set_by_pointer(
        ep->st_exchange_http_proxy_mailbox_proxy_failure,
        ep->rd_exchange_http_proxy_mailbox_proxy_failure,
        (collected_number)ep->exchangeProxyMailboxProxyFailure.current.Data);
    rrdset_done(ep->st_exchange_http_proxy_mailbox_proxy_failure);
}

static void netdata_exchange_proxy_mailbox_server_locator(struct exchange_proxy *ep, char *proxy, int update_every)
{
    if (unlikely(!ep->st_exchange_http_proxy_server_location_avg_latency)) {
        char id[RRD_ID_LENGTH_MAX + 1];
        snprintfz(id, RRD_ID_LENGTH_MAX, "exchange_proxy_%s_mailbox_server_locator_avg_latency_sec", proxy);

        ep->st_exchange_http_proxy_server_location_avg_latency = rrdset_create_localhost(
            "exchange",
            id,
            NULL,
            "proxy",
            "exchange.http_proxy_mailbox_server_locator_avg_latency_sec",
            "Average latency of MailboxServerLocator web service calls.",
            "seconds",
            PLUGIN_WINDOWS_NAME,
            "PerflibExchange",
            PRIO_EXCHANGE_PROXY_SERVER_LOCATIOR_AVG_LATENCY,
            update_every,
            RRDSET_TYPE_LINE);

        ep->rd_exchange_http_proxy_server_location_avg_latency = rrddim_add(
            ep->st_exchange_http_proxy_server_location_avg_latency, "latency", NULL, 1, 1000, RRD_ALGORITHM_ABSOLUTE);

        rrdlabels_add(
            ep->st_exchange_http_proxy_server_location_avg_latency->rrdlabels, "http_proxy", proxy, RRDLABEL_SRC_AUTO);
    }

    rrddim_set_by_pointer(
        ep->st_exchange_http_proxy_server_location_avg_latency,
        ep->rd_exchange_http_proxy_server_location_avg_latency,
        (collected_number)ep->exchangeProxyMailboxServerLocator.current.Data);
    rrdset_done(ep->st_exchange_http_proxy_server_location_avg_latency);
}

static void netdata_exchange_proxy_outstanding_proxy(struct exchange_proxy *ep, char *proxy, int update_every)
{
    if (unlikely(!ep->st_exchange_http_proxy_outstanding_proxy_requests)) {
        char id[RRD_ID_LENGTH_MAX + 1];
        snprintfz(id, RRD_ID_LENGTH_MAX, "exchange_proxy_%s_outstanding_proxy_requests", proxy);

        ep->st_exchange_http_proxy_outstanding_proxy_requests = rrdset_create_localhost(
            "exchange",
            id,
            NULL,
            "proxy",
            "exchange.http_proxy_outstanding_proxy_requests",
            "Concurrent outstanding proxy requests.",
            "requests",
            PLUGIN_WINDOWS_NAME,
            "PerflibExchange",
            PRIO_EXCHANGE_PROXY_OUTSTANDING_PROXY_REQUESTS,
            update_every,
            RRDSET_TYPE_LINE);

        ep->rd_exchange_http_proxy_outstanding_proxy_requests = rrddim_add(
            ep->st_exchange_http_proxy_outstanding_proxy_requests, "requests", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);

        rrdlabels_add(
            ep->st_exchange_http_proxy_outstanding_proxy_requests->rrdlabels, "http_proxy", proxy, RRDLABEL_SRC_AUTO);
    }

    rrddim_set_by_pointer(
        ep->st_exchange_http_proxy_outstanding_proxy_requests,
        ep->rd_exchange_http_proxy_outstanding_proxy_requests,
        (collected_number)ep->exchangeProxyOutstandingProxy.current.Data);
    rrdset_done(ep->st_exchange_http_proxy_outstanding_proxy_requests);
}

static void netdata_exchange_proxy_request_total(struct exchange_proxy *ep, char *proxy, int update_every)
{
    if (unlikely(!ep->st_exchange_http_proxy_requests_total)) {
        char id[RRD_ID_LENGTH_MAX + 1];
        snprintfz(id, RRD_ID_LENGTH_MAX, "exchange_proxy_%s_requests_total", proxy);

        ep->st_exchange_http_proxy_requests_total = rrdset_create_localhost(
            "exchange",
            id,
            NULL,
            "proxy",
            "exchange.http_proxy_requests",
            "Number of proxy requests processed each second.",
            "requests",
            PLUGIN_WINDOWS_NAME,
            "PerflibExchange",
            PRIO_EXCHANGE_PROXY_PROXY_REQUESTS_TOTAL,
            update_every,
            RRDSET_TYPE_LINE);

        ep->rd_exchange_http_proxy_requests_total =
            rrddim_add(ep->st_exchange_http_proxy_requests_total, "requests", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);

        rrdlabels_add(ep->st_exchange_http_proxy_requests_total->rrdlabels, "http_proxy", proxy, RRDLABEL_SRC_AUTO);
    }

    rrddim_set_by_pointer(
        ep->st_exchange_http_proxy_requests_total,
        ep->rd_exchange_http_proxy_requests_total,
        (collected_number)ep->exchangeProxyRequestsTotal.current.Data);
    rrdset_done(ep->st_exchange_http_proxy_requests_total);
}

static void netdata_exchange_proxy(PERF_DATA_BLOCK *pDataBlock, PERF_OBJECT_TYPE *pObjectType, int update_every)
{
    PERF_INSTANCE_DEFINITION *pi = NULL;
    for (LONG i = 0; i < pObjectType->NumInstances; i++) {
        pi = perflibForEachInstance(pDataBlock, pObjectType, pi);
        if (!pi)
            break;

        if (!getInstanceName(pDataBlock, pObjectType, pi, windows_shared_buffer, sizeof(windows_shared_buffer)))
            strncpyz(windows_shared_buffer, "[unknown]", sizeof(windows_shared_buffer) - 1);

        if (strcasecmp(windows_shared_buffer, "_Total") == 0)
            continue;

        struct exchange_proxy *ep = dictionary_set(exchange_proxies, windows_shared_buffer, NULL, sizeof(*ep));
        if (!ep)
            continue;

        if (perflibGetObjectCounter(pDataBlock, pObjectType, &ep->exchangeProxyAvgAuthLatency))
            netdata_exchange_proxy_avg_auth_latency(ep, windows_shared_buffer, update_every);

        if (perflibGetObjectCounter(pDataBlock, pObjectType, &ep->exchangeProxyAvgCasProcessingLatency))
            netdata_exchange_proxy_avg_cas_processing_latency(ep, windows_shared_buffer, update_every);

        if (perflibGetObjectCounter(pDataBlock, pObjectType, &ep->exchangeProxyMailboxProxyFailure))
            netdata_exchange_proxy_mailbox_proxy_failure(ep, windows_shared_buffer, update_every);

        if (perflibGetObjectCounter(pDataBlock, pObjectType, &ep->exchangeProxyMailboxServerLocator))
            netdata_exchange_proxy_mailbox_server_locator(ep, windows_shared_buffer, update_every);

        if (perflibGetObjectCounter(pDataBlock, pObjectType, &ep->exchangeProxyOutstandingProxy))
            netdata_exchange_proxy_outstanding_proxy(ep, windows_shared_buffer, update_every);

        if (perflibGetObjectCounter(pDataBlock, pObjectType, &ep->exchangeProxyRequestsTotal))
            netdata_exchange_proxy_request_total(ep, windows_shared_buffer, update_every);
    }
}

static void netdata_exchange_workload_active_tasks(struct exchange_workload *ew, char *workload, int update_every)
{
    if (unlikely(!ew->st_exchange_workload_active_tasks)) {
        char id[RRD_ID_LENGTH_MAX + 1];
        snprintfz(id, RRD_ID_LENGTH_MAX, "exchange_workload_%s_tasks", workload);

        ew->st_exchange_workload_active_tasks = rrdset_create_localhost(
            "exchange",
            id,
            NULL,
            "workload",
            "exchange.workload_active_tasks",
            "Workload active tasks.",
            "tasks",
            PLUGIN_WINDOWS_NAME,
            "PerflibExchange",
            PRIO_EXCHANGE_WORKLOAD_ACTIVE_TASKS,
            update_every,
            RRDSET_TYPE_LINE);

        ew->rd_exchange_workload_active_tasks =
            rrddim_add(ew->st_exchange_workload_active_tasks, "active", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);

        rrdlabels_add(ew->st_exchange_workload_active_tasks->rrdlabels, "workload", workload, RRDLABEL_SRC_AUTO);
    }

    rrddim_set_by_pointer(
        ew->st_exchange_workload_active_tasks,
        ew->rd_exchange_workload_active_tasks,
        (collected_number)ew->exchangeWorkloadActiveTasks.current.Data);
    rrdset_done(ew->st_exchange_workload_active_tasks);
}

static void netdata_exchange_workload_completed_tasks(struct exchange_workload *ew, char *workload, int update_every)
{
    if (unlikely(!ew->st_exchange_workload_complete_tasks)) {
        char id[RRD_ID_LENGTH_MAX + 1];
        snprintfz(id, RRD_ID_LENGTH_MAX, "exchange_workload_%s_completed_tasks", workload);

        ew->st_exchange_workload_complete_tasks = rrdset_create_localhost(
            "exchange",
            id,
            NULL,
            "workload",
            "exchange.workload_completed_tasks",
            "Workload completed tasks.",
            "tasks/s",
            PLUGIN_WINDOWS_NAME,
            "PerflibExchange",
            PRIO_EXCHANGE_WORKLOAD_COMPLETE_TASKS,
            update_every,
            RRDSET_TYPE_LINE);

        ew->rd_exchange_workload_complete_tasks =
            rrddim_add(ew->st_exchange_workload_complete_tasks, "completed", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);

        rrdlabels_add(ew->st_exchange_workload_complete_tasks->rrdlabels, "completed", workload, RRDLABEL_SRC_AUTO);
    }

    rrddim_set_by_pointer(
        ew->st_exchange_workload_complete_tasks,
        ew->rd_exchange_workload_complete_tasks,
        (collected_number)ew->exchangeWorkloadCompleteTasks.current.Data);
    rrdset_done(ew->st_exchange_workload_complete_tasks);
}

static void netdata_exchange_workload_queued_tasks(struct exchange_workload *ew, char *workload, int update_every)
{
    if (unlikely(!ew->st_exchange_workload_queued_tasks)) {
        char id[RRD_ID_LENGTH_MAX + 1];
        snprintfz(id, RRD_ID_LENGTH_MAX, "exchange_workload_%s_queued_tasks", workload);

        ew->st_exchange_workload_queued_tasks = rrdset_create_localhost(
            "exchange",
            id,
            NULL,
            "workload",
            "exchange.workload_queued_tasks",
            "Workload queued tasks.",
            "tasks/s",
            PLUGIN_WINDOWS_NAME,
            "PerflibExchange",
            PRIO_EXCHANGE_WORKLOAD_QUEUE_TASKS,
            update_every,
            RRDSET_TYPE_LINE);

        ew->rd_exchange_workload_queued_tasks =
            rrddim_add(ew->st_exchange_workload_queued_tasks, "queued", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);

        rrdlabels_add(ew->st_exchange_workload_queued_tasks->rrdlabels, "workload", workload, RRDLABEL_SRC_AUTO);
    }

    rrddim_set_by_pointer(
        ew->st_exchange_workload_queued_tasks,
        ew->rd_exchange_workload_queued_tasks,
        (collected_number)ew->exchangeWorkloadQueueTasks.current.Data);
    rrdset_done(ew->st_exchange_workload_queued_tasks);
}

static void netdata_exchange_workload_yielded_tasks(struct exchange_workload *ew, char *workload, int update_every)
{
    if (unlikely(!ew->st_exchange_workload_yielded_tasks)) {
        char id[RRD_ID_LENGTH_MAX + 1];
        snprintfz(id, RRD_ID_LENGTH_MAX, "exchange_workload_%s_yielded_tasks", workload);

        ew->st_exchange_workload_yielded_tasks = rrdset_create_localhost(
            "exchange",
            id,
            NULL,
            "workload",
            "exchange.workload_yielded_tasks",
            "Workload yielded tasks.",
            "tasks/s",
            PLUGIN_WINDOWS_NAME,
            "PerflibExchange",
            PRIO_EXCHANGE_WORKLOAD_YIELDED_TASKS,
            update_every,
            RRDSET_TYPE_LINE);

        ew->rd_exchange_workload_yielded_tasks =
            rrddim_add(ew->st_exchange_workload_yielded_tasks, "yielded", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);

        rrdlabels_add(ew->st_exchange_workload_yielded_tasks->rrdlabels, "workload", workload, RRDLABEL_SRC_AUTO);
    }

    rrddim_set_by_pointer(
        ew->st_exchange_workload_yielded_tasks,
        ew->rd_exchange_workload_yielded_tasks,
        (collected_number)ew->exchangeWorkloadYieldedTasks.current.Data);
    rrdset_done(ew->st_exchange_workload_yielded_tasks);
}

static void netdata_exchange_workload_activity_status(struct exchange_workload *ew, char *workload, int update_every)
{
    if (unlikely(!ew->st_exchange_workload_activity_status)) {
        char id[RRD_ID_LENGTH_MAX + 1];
        snprintfz(id, RRD_ID_LENGTH_MAX, "exchange_workload_%s_activity_status", workload);

        ew->st_exchange_workload_activity_status = rrdset_create_localhost(
            "exchange",
            id,
            NULL,
            "workload",
            "exchange.workload_activity_status",
            "Workload activity status.",
            "status",
            PLUGIN_WINDOWS_NAME,
            "PerflibExchange",
            PRIO_EXCHANGE_WORKLOAD_ACTIVITY,
            update_every,
            RRDSET_TYPE_LINE);

        ew->rd_exchange_workload_activity_active_status =
            rrddim_add(ew->st_exchange_workload_activity_status, "active", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);

        ew->rd_exchange_workload_activity_paused_status =
            rrddim_add(ew->st_exchange_workload_activity_status, "paused", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);

        rrdlabels_add(ew->st_exchange_workload_activity_status->rrdlabels, "workload", workload, RRDLABEL_SRC_AUTO);
    }

    bool value = (bool)ew->exchangeWorkloadActivityStatus.current.Data;
    rrddim_set_by_pointer(
        ew->st_exchange_workload_activity_status,
        ew->rd_exchange_workload_activity_active_status,
        (collected_number)value);
    rrdset_done(ew->st_exchange_workload_activity_status);

    value = !value;
    rrddim_set_by_pointer(
        ew->st_exchange_workload_activity_status,
        ew->rd_exchange_workload_activity_paused_status,
        (collected_number)value);
    rrdset_done(ew->st_exchange_workload_activity_status);
}

static void netdata_exchange_workload(PERF_DATA_BLOCK *pDataBlock, PERF_OBJECT_TYPE *pObjectType, int update_every)
{
    PERF_INSTANCE_DEFINITION *pi = NULL;
    for (LONG i = 0; i < pObjectType->NumInstances; i++) {
        pi = perflibForEachInstance(pDataBlock, pObjectType, pi);
        if (!pi)
            break;

        if (!getInstanceName(pDataBlock, pObjectType, pi, windows_shared_buffer, sizeof(windows_shared_buffer)))
            strncpyz(windows_shared_buffer, "[unknown]", sizeof(windows_shared_buffer) - 1);

        if (strcasecmp(windows_shared_buffer, "_Total") == 0)
            continue;

        struct exchange_workload *ew = dictionary_set(exchange_workloads, windows_shared_buffer, NULL, sizeof(*ew));
        if (!ew)
            continue;

        if (perflibGetObjectCounter(pDataBlock, pObjectType, &ew->exchangeWorkloadActiveTasks))
            netdata_exchange_workload_active_tasks(ew, windows_shared_buffer, update_every);

        if (perflibGetObjectCounter(pDataBlock, pObjectType, &ew->exchangeWorkloadCompleteTasks))
            netdata_exchange_workload_completed_tasks(ew, windows_shared_buffer, update_every);

        if (perflibGetObjectCounter(pDataBlock, pObjectType, &ew->exchangeWorkloadQueueTasks))
            netdata_exchange_workload_queued_tasks(ew, windows_shared_buffer, update_every);

        if (perflibGetObjectCounter(pDataBlock, pObjectType, &ew->exchangeWorkloadYieldedTasks))
            netdata_exchange_workload_yielded_tasks(ew, windows_shared_buffer, update_every);

        if (perflibGetObjectCounter(pDataBlock, pObjectType, &ew->exchangeWorkloadActivityStatus))
            netdata_exchange_workload_activity_status(ew, windows_shared_buffer, update_every);
    }
}

static void netdata_exchange_queue_active_mailbox(struct exchange_queue *eq, char *mailbox, int update_every)
{
    if (unlikely(!eq->st_exchange_queue_active_mailbox)) {
        char id[RRD_ID_LENGTH_MAX + 1];
        snprintfz(id, RRD_ID_LENGTH_MAX, "exchange_transport_queues_%s_active_mailbox_delivery", mailbox);

        eq->st_exchange_queue_active_mailbox = rrdset_create_localhost(
            "exchange",
            id,
            NULL,
            "queue",
            "exchange.transport_queues_active_mail_box_delivery",
            "Active Mailbox Delivery Queue length.",
            "messages",
            PLUGIN_WINDOWS_NAME,
            "PerflibExchange",
            PRIO_EXCHANGE_QUEUE_ACTIVE_MAILBOX_DELIVERY,
            update_every,
            RRDSET_TYPE_LINE);

        eq->rd_exchange_queue_active_mailbox =
            rrddim_add(eq->st_exchange_queue_active_mailbox, "active", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);

        rrdlabels_add(eq->st_exchange_queue_active_mailbox->rrdlabels, "mailbox", mailbox, RRDLABEL_SRC_AUTO);
    }

    rrddim_set_by_pointer(
        eq->st_exchange_queue_active_mailbox,
        eq->rd_exchange_queue_active_mailbox,
        (collected_number)eq->exchangeTransportQueuesActiveMailboxDelivery.current.Data);
    rrdset_done(eq->st_exchange_queue_active_mailbox);
}

static void netdata_exchange_queue_external_active_remote_delivery(struct exchange_queue *eq, char *mailbox, int update_every)
{
    if (unlikely(!eq->st_exchange_queue_external_active_remote_delivery)) {
        char id[RRD_ID_LENGTH_MAX + 1];
        snprintfz(id, RRD_ID_LENGTH_MAX, "exchange_transport_queues_%s_external_active_remote_delivery", mailbox);

        eq->st_exchange_queue_external_active_remote_delivery = rrdset_create_localhost(
            "exchange",
            id,
            NULL,
            "queue",
            "exchange.transport_queues_external_active_remote_delivery",
            "External Active Remote Delivery Queue length.",
            "messages",
            PLUGIN_WINDOWS_NAME,
            "PerflibExchange",
            PRIO_EXCHANGE_QUEUE_EXTERNAL_ACTIVE_REMOTE_DELIVERY,
            update_every,
            RRDSET_TYPE_LINE);

        eq->rd_exchange_queue_external_active_remote_delivery =
            rrddim_add(eq->st_exchange_queue_external_active_remote_delivery, "active", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);

        rrdlabels_add(eq->st_exchange_queue_external_active_remote_delivery->rrdlabels, "mailbox", mailbox, RRDLABEL_SRC_AUTO);
    }

    rrddim_set_by_pointer(
        eq->st_exchange_queue_external_active_remote_delivery,
        eq->rd_exchange_queue_external_active_remote_delivery,
        (collected_number)eq->exchangeTransportQueuesExternalActiveRemoteDelivery.current.Data);
    rrdset_done(eq->st_exchange_queue_external_active_remote_delivery);
}

static void netdata_exchange_queue_internal_active_remote_delivery(struct exchange_queue *eq, char *mailbox, int update_every)
{
    if (unlikely(!eq->st_exchange_queue_internal_active_remote_delivery)) {
        char id[RRD_ID_LENGTH_MAX + 1];
        snprintfz(id, RRD_ID_LENGTH_MAX, "exchange_transport_queues_%s_internal_active_remote_delivery", mailbox);

        eq->st_exchange_queue_internal_active_remote_delivery = rrdset_create_localhost(
                "exchange",
                id,
                NULL,
                "queue",
                "exchange.transport_queues_internal_active_remote_delivery",
                "Internal Active Remote Delivery Queue length.",
                "messages",
                PLUGIN_WINDOWS_NAME,
                "PerflibExchange",
                PRIO_EXCHANGE_QUEUE_INTERNAL_ACTIVE_REMOTE_DELIVERY,
                update_every,
                RRDSET_TYPE_LINE);

        eq->rd_exchange_queue_internal_active_remote_delivery =
                rrddim_add(eq->st_exchange_queue_internal_active_remote_delivery, "active", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);

        rrdlabels_add(eq->st_exchange_queue_internal_active_remote_delivery->rrdlabels, "mailbox", mailbox, RRDLABEL_SRC_AUTO);
    }

    rrddim_set_by_pointer(
            eq->st_exchange_queue_internal_active_remote_delivery,
            eq->rd_exchange_queue_internal_active_remote_delivery,
            (collected_number)eq->exchangeTransportQueuesInternalActiveRemoteDelivery.current.Data);
    rrdset_done(eq->st_exchange_queue_internal_active_remote_delivery);
}

static void netdata_exchange_queue_unreachable(struct exchange_queue *eq, char *mailbox, int update_every)
{
    if (unlikely(!eq->st_exchange_queue_unreachable)) {
        char id[RRD_ID_LENGTH_MAX + 1];
        snprintfz(id, RRD_ID_LENGTH_MAX, "exchange_transport_queues_%s_unreachable", mailbox);

        eq->st_exchange_queue_unreachable = rrdset_create_localhost(
                "exchange",
                id,
                NULL,
                "queue",
                "exchange.transport_queues_unreachable",
                "Unreachable Queue length.",
                "messages",
                PLUGIN_WINDOWS_NAME,
                "PerflibExchange",
                PRIO_EXCHANGE_QUEUE_UNREACHABLE,
                update_every,
                RRDSET_TYPE_LINE);

        eq->rd_exchange_queue_unreachable =
                rrddim_add(eq->st_exchange_queue_unreachable, "unreachable", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);

        rrdlabels_add(eq->st_exchange_queue_unreachable->rrdlabels, "mailbox", mailbox, RRDLABEL_SRC_AUTO);
    }

    rrddim_set_by_pointer(
            eq->st_exchange_queue_unreachable,
            eq->rd_exchange_queue_unreachable,
            (collected_number)eq->exchangeTransportQueuesUnreachable.current.Data);
    rrdset_done(eq->st_exchange_queue_unreachable);
}

static void netdata_exchange_queue_poison(struct exchange_queue *eq, char *mailbox, int update_every)
{
    if (unlikely(!eq->st_exchange_queue_poison)) {
        char id[RRD_ID_LENGTH_MAX + 1];
        snprintfz(id, RRD_ID_LENGTH_MAX, "exchange_transport_queues_%s_poison", mailbox);

        eq->st_exchange_queue_poison = rrdset_create_localhost(
                "exchange",
                id,
                NULL,
                "queue",
                "exchange.transport_queues_poison",
                "Poison Queue Length.",
                "messages",
                PLUGIN_WINDOWS_NAME,
                "PerflibExchange",
                PRIO_EXCHANGE_QUEUE_POISON,
                update_every,
                RRDSET_TYPE_LINE);

        eq->rd_exchange_queue_poison =
                rrddim_add(eq->st_exchange_queue_poison, "poison", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);

        rrdlabels_add(eq->st_exchange_queue_poison->rrdlabels, "mailbox", mailbox, RRDLABEL_SRC_AUTO);
    }

    rrddim_set_by_pointer(
            eq->st_exchange_queue_poison,
            eq->rd_exchange_queue_poison,
            (collected_number)eq->exchangeTransportQueuesPoison.current.Data);
    rrdset_done(eq->st_exchange_queue_poison);
}

static void netdata_exchange_queues(PERF_DATA_BLOCK *pDataBlock, PERF_OBJECT_TYPE *pObjectType, int update_every)
{
    PERF_INSTANCE_DEFINITION *pi = NULL;
    for (LONG i = 0; i < pObjectType->NumInstances; i++) {
        pi = perflibForEachInstance(pDataBlock, pObjectType, pi);
        if (!pi)
            break;

        if (!getInstanceName(pDataBlock, pObjectType, pi, windows_shared_buffer, sizeof(windows_shared_buffer)))
            strncpyz(windows_shared_buffer, "[unknown]", sizeof(windows_shared_buffer) - 1);

        // Remove 'queue' from instance name
        char *ptr = strchr(windows_shared_buffer, ' ');
        if (ptr)
            *ptr = '\0';

        if (strcasecmp(windows_shared_buffer, "_Total") == 0 || strcasecmp(windows_shared_buffer, "total") == 0)
            continue;

        struct exchange_queue *eq = dictionary_set(exchange_queues, windows_shared_buffer, NULL, sizeof(*eq));
        if (!eq)
            continue;

        if (perflibGetObjectCounter(pDataBlock, pObjectType, &eq->exchangeTransportQueuesActiveMailboxDelivery))
            netdata_exchange_queue_active_mailbox(eq, windows_shared_buffer, update_every);

        if (perflibGetObjectCounter(pDataBlock, pObjectType, &eq->exchangeTransportQueuesExternalActiveRemoteDelivery))
            netdata_exchange_queue_external_active_remote_delivery(eq, windows_shared_buffer, update_every);

        if (perflibGetObjectCounter(pDataBlock, pObjectType, &eq->exchangeTransportQueuesInternalActiveRemoteDelivery))
            netdata_exchange_queue_internal_active_remote_delivery(eq, windows_shared_buffer, update_every);

        if (perflibGetObjectCounter(pDataBlock, pObjectType, &eq->exchangeTransportQueuesUnreachable))
            netdata_exchange_queue_unreachable(eq, windows_shared_buffer, update_every);

        if (perflibGetObjectCounter(pDataBlock, pObjectType, &eq->exchangeTransportQueuesPoison))
            netdata_exchange_queue_poison(eq, windows_shared_buffer, update_every);
    }
}

struct netdata_exchange_objects {
    void (*fnct)(PERF_DATA_BLOCK *, PERF_OBJECT_TYPE *, int);
    char *object;
} exchange_obj[] = {
    {.fnct = netdata_exchange_owa, .object = "MSExchange OWA"},
    {.fnct = netdata_exchange_active_sync, .object = "MSExchange ActiveSync"},
    {.fnct = netdata_exchange_auto_discover, .object = "MSExchangeAutodiscover"},
    {.fnct = netdata_exchange_availability_service, .object = "MSExchange Availability Service"},
    {.fnct = netdata_exchange_rpc, .object = "MSExchange RpcClientAccess"},
    {.fnct = netdata_exchange_proxy, .object = "MSExchange HttpProxy"},
    {.fnct = netdata_exchange_workload, .object = "MSExchange WorkloadManagement Workloads"},
    {.fnct = netdata_exchange_queues, .object = "MSExchangeTransport Queues"},

    // This is the end of the loop
    {.fnct = NULL, .object = NULL}};

int do_PerflibExchange(int update_every, usec_t dt __maybe_unused)
{
    static bool initialized = false;

    if (unlikely(!initialized)) {
        initialize();
        initialized = true;
    }

    for (int i = 0; exchange_obj[i].fnct; i++) {
        DWORD id = RegistryFindIDByName(exchange_obj[i].object);
        if (id == PERFLIB_REGISTRY_NAME_NOT_FOUND)
            continue;

        PERF_DATA_BLOCK *pDataBlock = perflibGetPerformanceData(id);
        if (!pDataBlock)
            continue;

        PERF_OBJECT_TYPE *pObjectType = perflibFindObjectTypeByName(pDataBlock, exchange_obj[i].object);
        if (!pObjectType)
            continue;

        exchange_obj[i].fnct(pDataBlock, pObjectType, update_every);
    }

    return 0;
}
