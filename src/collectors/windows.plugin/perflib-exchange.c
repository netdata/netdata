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
            rrddim_add(st_exchange_owa_unique_users, "users", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
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

struct netdata_exchange_objects {
    void (*fnct)(PERF_DATA_BLOCK *, PERF_OBJECT_TYPE *, int);
    char *object;
} exchange_obj[] = {
    {.fnct = netdata_exchange_owa, .object = "MSExchange OWA"},
    {.fnct = netdata_exchange_auto_discover, .object = "MSExchangeAutodiscover"},
    {.fnct = netdata_exchange_availability_service, .object = "MSExchange Availability Service"},

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
