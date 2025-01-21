// SPDX-License-Identifier: GPL-3.0-or-later

#include "windows_plugin.h"
#include "windows-internals.h"

static void initialize(void) {
    ;
}

static void netdata_ad_atq(PERF_DATA_BLOCK *pDataBlock, PERF_OBJECT_TYPE *pObjectType, int update_every) {
    static COUNTER_DATA atqAverageRequestLatency = { .key = "ATQ Request Latency" };
    static COUNTER_DATA atqOutstandingRequests = { .key = "ATQ Outstanding Queued Requests" };

    static RRDSET *st_atq_average_request_latency = NULL;
    static RRDDIM *rd_atq_average_request_latency = NULL;
    static RRDSET *st_atq_outstanding_requests = NULL;
    static RRDDIM *rd_atq_outstanding_requests = NULL;

    if(perflibGetObjectCounter(pDataBlock, pObjectType, &atqAverageRequestLatency)) {
        if (unlikely(!st_atq_average_request_latency)) {
            st_atq_average_request_latency =  rrdset_create_localhost("ad"
                                                                     , "atq_average_request_latency"
                                                                     , NULL
                                                                     , "queue"
                                                                     , "ad.atq_average_request_latency"
                                                                     , "Average request processing time"
                                                                     , "seconds"
                                                                     , PLUGIN_WINDOWS_NAME
                                                                     , "PerflibAD"
                                                                     , PRIO_AD_AVG_REQUEST_LATENCY
                                                                     , update_every
                                                                     , RRDSET_TYPE_LINE
                                                                     );
            rd_atq_average_request_latency = rrddim_add(st_atq_average_request_latency,
                                                        "time",
                                                        NULL,
                                                        1,
                                                        1000,
                                                        RRD_ALGORITHM_ABSOLUTE);
        }

        rrddim_set_by_pointer(st_atq_average_request_latency,
                              rd_atq_average_request_latency,
                              (collected_number)p->atqAverageRequestLatency.current.Data);
        rrdset_done(st_atq_average_request_latency);
    }

    if(perflibGetObjectCounter(pDataBlock, pObjectType, &atqOutstandingRequests)) {
        if (unlikely(!st_atq_outstanding_requests)) {
            st_atq_outstanding_requests = rrdset_create_localhost("ad"
                                                                  , "atq_outstanding_requests"
                                                                  , NULL
                                                                  , "queue"
                                                                  , "ad.atq_outstanding_requests"
                                                                  , "Outstanding requests"
                                                                  , "requests"
                                                                  , PLUGIN_WINDOWS_NAME
                                                                  , "PerflibAD"
                                                                  , PRIO_AD_OUTSTANDING_REQUEST
                                                                  , update_every
                                                                  , RRDSET_TYPE_LINE);

            rd_atq_outstanding_requests = rrddim_add(st_atq_outstanding_requests,
                                                     "outstanding",
                                                     NULL,
                                                     1,
                                                     1,
                                                     RRD_ALGORITHM_ABSOLUTE);
        }

        rrddim_set_by_pointer(st_atq_outstanding_requests,
                              rd_atq_outstanding_requests,
                              (collected_number)p->atqOutstandingRequests.current.Data);
        rrdset_done(st_atq_outstanding_requests);
    }
}

static bool do_AD(PERF_DATA_BLOCK *pDataBlock, int update_every) {
    PERF_OBJECT_TYPE *pObjectType = perflibFindObjectTypeByName(pDataBlock, "DirectoryServices");
    if (!pObjectType)
        return false;

    static void (*doAD[])(PERF_DATA_BLOCK *, PERF_OBJECT_TYPE *, int) = {
        netdata_ad_atq,

        // This must be the end
        NULL
    };

    for (int i = 0; doAD[i]; i++)
        doAD[i](pDataBlock, pObjectType, update_every);

    // Operations Total
    static COUNTER_DATA databaseAddsPerSec = { .key = "Database adds/sec" };
    static COUNTER_DATA databaseDeletesPerSec = { .key = "Database deletes/sec" };
    static COUNTER_DATA databaseModifiesPerSec = { .key = "Database modifys/sec" };
    static COUNTER_DATA databaseRecyclesPerSec = { .key = "Database recycles/sec" };

    /* TODO : Compare with dumpto confirm
    static COUNTER_DATA directoryReadsPerSec = { .key = "DS Directory Reads/sec" };
    static COUNTER_DATA directoryPercReadsFromDCA = { .key = "DS % Reads from DRA" };
    static COUNTER_DATA directoryPercReadsFromKCC = { .key = "DS % Reads from KCC" };
    static COUNTER_DATA directoryPercReadsFromLSA = { .key = "DS % Reads from LSA" };
    static COUNTER_DATA directoryPercReadsFromNSPI = { .key = "DS % Reads from NSPI" };
    static COUNTER_DATA directoryPercReadsFromNTDSAPI = { .key = "DS % Reads from NTDSAPI" };
    static COUNTER_DATA directoryPercReadsFromSAM = { .key = "DS % Reads from SAM" };
    static COUNTER_DATA directoryPercReadsOther = { .key = "DS % Reads from Other" };
    static COUNTER_DATA directoryPercSearchesFromDCA = { .key = "DS % Searches from DRA" };
    static COUNTER_DATA directoryPercSearchesFromKCC = { .key = "DS % Searches from KCC" };
    static COUNTER_DATA directoryPercSearchesFromLSA = { .key = "DS % Searches from LSA" };
    static COUNTER_DATA directoryPercSearchesFromNSPI = { .key = "DS % Searches from NSPI" };
    static COUNTER_DATA directoryPercSearchesFromNTDSAPI = { .key = "DS % Searches from NTDSAPI" };
    static COUNTER_DATA directoryPercSearchesFromSAM = { .key = "DS % Searches from SAM" };
    static COUNTER_DATA directoryPercSearchesOther = { .key = "DS % Searches from Other" };
    static COUNTER_DATA directoryPercWritesFromDCA = { .key = "DS % Writes from DRA" };
    static COUNTER_DATA directoryPercWritesFromKCC = { .key = "DS % Writes from KCC" };
    static COUNTER_DATA directoryPercWritesFromLSA = { .key = "DS % Writes from LSA" };
    static COUNTER_DATA directoryPercWritesFromNSPI = { .key = "DS % Writes from NSPI" };
    static COUNTER_DATA directoryPercWritesFromNTDSAPI = { .key = "DS % Writes from NTDSAPI" };
    static COUNTER_DATA directoryPercWritesFromSAM = { .key = "DS % Writes from SAM" };
    static COUNTER_DATA directoryPercWritesOther = { .key = "DS % Writes from Other" };
    */

    // Replication
    static COUNTER_DATA replicationInboundObjectsFilteringTotal = { .key = "DRA Inbound Objects Filtered/sec" };
    static COUNTER_DATA replicationInboundPropertiesFilteredTotal = { .key = "DRA Inbound Properties Filtered/sec" };
    static COUNTER_DATA replicationInboundPropertiesUpdatedTotal = { .key = "DRA Inbound Properties Total/sec" };
    static COUNTER_DATA replicationInboundSyncObjectsRemaining = { .key = "DRA Inbound Full Sync Objects Remaining" };

    static COUNTER_DATA replicationDataInterSiteBytesTotal = { .key = "DRA Inbound Bytes Compressed (Between Sites, After Compression)/sec" };
    static COUNTER_DATA replicationDataIntraSiteBytesTotal = { .key = "DRA Inbound Bytes Not Compressed (Within Site)/sec"};

    static COUNTER_DATA replicationPendingSyncs = { .key = "DRA Pending Replication Synchronizations" };

    // Replication sync
    static COUNTER_DATA replicationSyncRequestsTotal = { .key = "DRA Sync Requests Made" };
    static COUNTER_DATA replicationSyncRequestsTotal = { .key = "DRA Sync Requests Successful" };

    static COUNTER_DATA directoryServiceThreads = { .key = "DS Threads in Use" };
    static COUNTER_DATA ldapLastBindTimeSecondsTotal = { .key = "DAP Bind Time" };
    static COUNTER_DATA bindsTotal = { .key = "DS Server Binds/sec" };
    static COUNTER_DATA ldapSearchesTotal = { .key = "LDAP Searches/sec" };
    static COUNTER_DATA nameCacheLookupsTotal = { .key = "DS Name Cache hit rate,secondvalue" };
    static COUNTER_DATA nameCacheHitsTotal  = { .key = "DS Name Cache hit rate" };

    return true;
}

int do_PerflibAD(int update_every, usec_t dt __maybe_unused) {
    static bool initialized = false;

    if(unlikely(!initialized)) {
        initialize();
        initialized = true;
    }

    DWORD id = RegistryFindIDByName("DirectoryServices");
    if(id == PERFLIB_REGISTRY_NAME_NOT_FOUND)
        return -1;

    PERF_DATA_BLOCK *pDataBlock = perflibGetPerformanceData(id);
    if(!pDataBlock) return -1;

    do_AD(pDataBlock, update_every);

    return 0;
}
