// SPDX-License-Identifier: GPL-3.0-or-later

#include "windows_plugin.h"
#include "windows-internals.h"

static void initialize(void)
{
    ;
}

static void netdata_exchange_owa_current_unique_users(COUNTER_DATA *value, int update_every) {
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
            rrddim_add(st_exchange_owa_unique_users, "users", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
    }

    rrddim_set_by_pointer(
        st_exchange_owa_unique_users,
        rd_exchange_owa_unique_users,
        (collected_number)value->current.Data);
    rrdset_done(st_exchange_owa_unique_users);
}

static void netdata_exchange_owa_request_total(COUNTER_DATA *value, int update_every) {
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
            rrddim_add(st_exchange_owa_request_total, "requests", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
    }

    rrddim_set_by_pointer(
        st_exchange_owa_request_total,
        rd_exchange_owa_request_total,
        (collected_number)value->current.Data);
    rrdset_done(st_exchange_owa_request_total);
}

static
void netdata_exchange_owa(PERF_DATA_BLOCK *pDataBlock, PERF_OBJECT_TYPE *pObjectType, int update_every) {
    static COUNTER_DATA exchangeOWACurrentUniqueUser = {.key = "Current Unique Users" };
    static COUNTER_DATA exchangeOWARequestsTotal = {.key = "Requests/sec" };

    if (!perflibGetObjectCounter(pDataBlock, pObjectType, &exchangeOWACurrentUniqueUser) ||
        !perflibGetObjectCounter(pDataBlock, pObjectType, &exchangeOWARequestsTotal)) {
        return;
    }

    netdata_exchange_owa_current_unique_users(&exchangeOWACurrentUniqueUser, update_every);
    netdata_exchange_owa_request_total(&exchangeOWARequestsTotal, update_every);
}

static void netdata_exchange_active_ping_cmd(COUNTER_DATA *value, int update_every) {
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
            rrddim_add(st_exchange_active_ping_cmds, "ping", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
    }

    rrddim_set_by_pointer(
        st_exchange_active_ping_cmds,
        rd_exchange_active_ping_cmds,
        (collected_number)value->current.Data);
    rrdset_done(st_exchange_active_ping_cmds);
}

static void netdata_exchange_active_requests(COUNTER_DATA *value, int update_every) {
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
            rrddim_add(st_exchange_received_requests, "requests", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
    }

    rrddim_set_by_pointer(
        st_exchange_received_requests,
        rd_exchange_received_requests,
        (collected_number)value->current.Data);
    rrdset_done(st_exchange_received_requests);
}

static void netdata_exchange_sync_cmds(COUNTER_DATA *value, int update_every) {
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

        rd_exchange_sync_cmds =
            rrddim_add(st_exchange_sync_cmds, "sync", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
    }

    rrddim_set_by_pointer(
        st_exchange_sync_cmds,
        rd_exchange_sync_cmds,
        (collected_number)value->current.Data);
    rrdset_done(st_exchange_sync_cmds);
}

static
void netdata_exchange_active_sync(PERF_DATA_BLOCK *pDataBlock, PERF_OBJECT_TYPE *pObjectType, int update_every) {
    static COUNTER_DATA exchangePingCommands = {.key = "Ping Commands Pending" };
    static COUNTER_DATA exchangeSyncCommands = {.key = "Sync Commands/sec" };
    static COUNTER_DATA exchangeActiveRequests = {.key = "Requests/sec" };

    if (!perflibGetObjectCounter(pDataBlock, pObjectType, &exchangePingCommands) ||
        !perflibGetObjectCounter(pDataBlock, pObjectType, &exchangeSyncCommands) ||
        !perflibGetObjectCounter(pDataBlock, pObjectType, &exchangeActiveRequests)) {
        return;
    }

    netdata_exchange_active_ping_cmd(&exchangePingCommands, update_every);
    netdata_exchange_active_requests(&exchangeActiveRequests, update_every);
    netdata_exchange_sync_cmds(&exchangeSyncCommands, update_every);
}

static
void netdata_exchange_auto_discover(PERF_DATA_BLOCK *pDataBlock, PERF_OBJECT_TYPE *pObjectType, int update_every) {
    static COUNTER_DATA exchangeAutoDiscoverRequestsTotal = {.key = "Requests/sec" };

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

static
void netdata_exchange_availability_service(PERF_DATA_BLOCK *pDataBlock, PERF_OBJECT_TYPE *pObjectType, int update_every) {
    static COUNTER_DATA exchangeAvailServiceRequests = {.key = "Availability Requests (sec)" };

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

static void netdata_exchange_rpc_avg_latency(COUNTER_DATA *value, int update_every) {
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
        st_exchange_rpc_avg_latency,
        rd_exchange_rpc_avg_latency,
        (collected_number)value->current.Data);
    rrdset_done(st_exchange_rpc_avg_latency);
}

static void netdata_exchange_rpc_requests(COUNTER_DATA *value, int update_every) {
    static RRDSET *st_exchange_rpc_requests = NULL;
    static RRDDIM *rd_exchange_rpc_requests = NULL;

    if (unlikely(!st_exchange_rpc_requests)) {
        st_exchange_rpc_requests = rrdset_create_localhost(
            "exchange",
            "rpc_requests_total",
            NULL,
            "rpc",
            "exchange.rpc_requests_total",
            "Clients requests currently being processed.",
            "requests",
            PLUGIN_WINDOWS_NAME,
            "PerflibExchange",
            PRIO_EXCHANGE_RPC_REQUESTS,
            update_every,
            RRDSET_TYPE_LINE);

        rd_exchange_rpc_requests =
            rrddim_add(st_exchange_rpc_requests, "requests", NULL, 1, 1000, RRD_ALGORITHM_ABSOLUTE);
    }

    rrddim_set_by_pointer(
        st_exchange_rpc_requests,
        rd_exchange_rpc_requests,
        (collected_number)value->current.Data);
    rrdset_done(st_exchange_rpc_requests);
}

static void netdata_exchange_rpc_active_user_count(COUNTER_DATA *value, int update_every) {
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
            rrddim_add(st_exchange_active_user_account, "users", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
    }

    rrddim_set_by_pointer(
        st_exchange_active_user_account,
        rd_exchange_active_user_account,
        (collected_number)value->current.Data);
    rrdset_done(st_exchange_active_user_account);
}

static void netdata_exchange_rpc_connection_count(COUNTER_DATA *value, int update_every) {
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
        st_exchange_rpc_connection_count,
        rd_exchange_rpc_connection_count,
        (collected_number)value->current.Data);
    rrdset_done(st_exchange_rpc_connection_count);
}

static void netdata_exchange_rpc_op_per_sec(COUNTER_DATA *value, int update_every) {
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
        st_exchange_rpc_opc_per_sec,
        rd_exchange_rpc_opc_per_sec,
        (collected_number)value->current.Data);
    rrdset_done(st_exchange_rpc_opc_per_sec);
}

static void netdata_exchange_rpc_user_count(COUNTER_DATA *value, int update_every) {
    static RRDSET *st_exchange_rpc_user_count = NULL;
    static RRDDIM *rd_exchange_rpc_user_count = NULL;

    if (unlikely(!st_exchange_rpc_user_count)) {
        st_exchange_rpc_user_count = rrdset_create_localhost(
            "exchange",
            "rpc_user",
            NULL,
            "rpc",
            "exchange.rpc_user",
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
        st_exchange_rpc_user_count,
        rd_exchange_rpc_user_count,
        (collected_number)value->current.Data);
    rrdset_done(st_exchange_rpc_user_count);
}

static
void netdata_exchange_rpc(PERF_DATA_BLOCK *pDataBlock, PERF_OBJECT_TYPE *pObjectType, int update_every) {
    static COUNTER_DATA exchangeRPCAveragedLatency = {.key = "RPC Averaged Latency" };
    static COUNTER_DATA exchangeRPCRequest = {.key = "RPC Requests" };
    static COUNTER_DATA exchangeRPCActiveUserCount = {.key = "Active User Count" };
    static COUNTER_DATA exchangeRPCConnectionCount = {.key = "Connection Count" };
    static COUNTER_DATA exchangeRPCOperationPerSec = {.key = "RPC Operations/sec" };
    static COUNTER_DATA exchangeRPCUserCount = {.key = "User Count" };

    if (perflibGetObjectCounter(pDataBlock, pObjectType, &exchangeRPCAveragedLatency))
        netdata_exchange_rpc_avg_latency(&exchangeRPCAveragedLatency, update_every);

    if (perflibGetObjectCounter(pDataBlock, pObjectType, &exchangeRPCRequest))
        netdata_exchange_rpc_requests(&exchangeRPCRequest, update_every);

    if (perflibGetObjectCounter(pDataBlock, pObjectType, &exchangeRPCActiveUserCount))
        netdata_exchange_rpc_active_user_count(&exchangeRPCRequest, update_every);

    if (perflibGetObjectCounter(pDataBlock, pObjectType, &exchangeRPCConnectionCount))
        netdata_exchange_rpc_connection_count(&exchangeRPCConnectionCount, update_every);

    if (perflibGetObjectCounter(pDataBlock, pObjectType, &exchangeRPCOperationPerSec))
        netdata_exchange_rpc_op_per_sec(&exchangeRPCOperationPerSec, update_every);

    if (perflibGetObjectCounter(pDataBlock, pObjectType, &exchangeRPCUserCount))
        netdata_exchange_rpc_user_count(&exchangeRPCUserCount, update_every);
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

    // This is the end of the loop
    {.fnct = NULL, .object = NULL}};

int do_PerflibExchange(int update_every, usec_t dt __maybe_unused)
{
    static bool initialized = false;

    if (unlikely(!initialized)) {
        initialize();
        initialized = true;
    }

    for (int i = 0; exchange_obj[i].fnct ; i++) {
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
