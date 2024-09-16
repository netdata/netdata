// SPDX-License-Identifier: GPL-3.0-or-later

#include "windows_plugin.h"
#include "windows-internals.h"

struct web_service {
    RRDSET *st;
    RRDDIM *rd;

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
};

struct w3vc_w3wp {
    COUNTER_DATA IISRequestsTotal;
};

struct w3vc_w3wp w3svc_conters;

static inline void initialize_web_service_keys(struct web_service *p) {
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
}

static inline void initialize_w3vc_w3wp_keys(struct w3vc_w3wp *p) {
    p->IISRequestsTotal.key = "Total HTTP Requests Served";
}

void dict_web_service_insert_cb(const DICTIONARY_ITEM *item __maybe_unused, void *value, void *data __maybe_unused) {
    struct web_service *p = value;
    initialize_web_service_keys(p);
}

static DICTIONARY *web_services = NULL;

static void initialize(void) {
    web_services = dictionary_create_advanced(DICT_OPTION_DONT_OVERWRITE_VALUE |
                                                DICT_OPTION_FIXED_SIZE, NULL, sizeof(struct web_service));

    dictionary_register_insert_callback(web_services, dict_web_service_insert_cb, NULL);

    initialize_w3vc_w3wp_keys(&w3svc_conters);
}

static bool do_web_services(PERF_DATA_BLOCK *pDataBlock, int update_every) {
    PERF_OBJECT_TYPE *pObjectType = perflibFindObjectTypeByName(pDataBlock, "Web Service");
    if(!pObjectType) return false;

    PERF_INSTANCE_DEFINITION *pi = NULL;
    for(LONG i = 0; i < pObjectType->NumInstances ; i++) {
        pi = perflibForEachInstance(pDataBlock, pObjectType, pi);
        if(!pi) break;

        if(!getInstanceName(pDataBlock, pObjectType, pi, windows_shared_buffer, sizeof(windows_shared_buffer)))
            strncpyz(windows_shared_buffer, "[unknown]", sizeof(windows_shared_buffer) - 1);

        netdata_fix_chart_name(windows_shared_buffer);
        struct web_service *p = dictionary_set(web_services, windows_shared_buffer, NULL, sizeof(*p));

        perflibGetInstanceCounter(pDataBlock, pObjectType, pi, &p->IISCurrentAnonymousUser);

        /*
        if(!p->st) {
        }

        // Convert to Celsius before to plot

        rrdset_done(p->st);
        */
    }

    return true;
}

int do_PerflibWebService(int update_every, usec_t dt __maybe_unused) {
    static bool initialized = false;

    if(unlikely(!initialized)) {
        initialize();
        initialized = true;
    }

    DWORD id = RegistryFindIDByName("Web Service");
    if(id == PERFLIB_REGISTRY_NAME_NOT_FOUND)
        return -1;

    PERF_DATA_BLOCK *pDataBlock = perflibGetPerformanceData(id);
    if(!pDataBlock) return -1;

    do_web_services(pDataBlock, update_every);

    return 0;
}
