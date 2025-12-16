// SPDX-License-Identifier: GPL-3.0-or-later

#include "windows_plugin.h"
#include "windows-internals.h"

static void netdata_ad_directory(PERF_DATA_BLOCK *pDataBlock, PERF_OBJECT_TYPE *pObjectType, int update_every)
{
    static COUNTER_DATA directoryPercReadsFromDCA = {.key = "DS % Reads from DRA"};
    static COUNTER_DATA directoryPercReadsFromKCC = {.key = "DS % Reads from KCC"};
    static COUNTER_DATA directoryPercReadsFromLSA = {.key = "DS % Reads from LSA"};
    static COUNTER_DATA directoryPercReadsFromNSPI = {.key = "DS % Reads from NSPI"};
    static COUNTER_DATA directoryPercReadsFromNTDSAPI = {.key = "DS % Reads from NTDSAPI"};
    static COUNTER_DATA directoryPercReadsFromSAM = {.key = "DS % Reads from SAM"};
    static COUNTER_DATA directoryPercReadsOther = {.key = "DS % Reads from Other"};

    static COUNTER_DATA directoryPercSearchesFromDCA = {.key = "DS % Searches from DRA"};
    static COUNTER_DATA directoryPercSearchesFromKCC = {.key = "DS % Searches from KCC"};
    static COUNTER_DATA directoryPercSearchesFromLSA = {.key = "DS % Searches from LSA"};
    static COUNTER_DATA directoryPercSearchesFromNSPI = {.key = "DS % Searches from NSPI"};
    static COUNTER_DATA directoryPercSearchesFromNTDSAPI = {.key = "DS % Searches from NTDSAPI"};
    static COUNTER_DATA directoryPercSearchesFromSAM = {.key = "DS % Searches from SAM"};
    static COUNTER_DATA directoryPercSearchesOther = {.key = "DS % Searches from Other"};

    static COUNTER_DATA directoryPercWritesFromDCA = {.key = "DS % Writes from DRA"};
    static COUNTER_DATA directoryPercWritesFromKCC = {.key = "DS % Writes from KCC"};
    static COUNTER_DATA directoryPercWritesFromLSA = {.key = "DS % Writes from LSA"};
    static COUNTER_DATA directoryPercWritesFromNSPI = {.key = "DS % Writes from NSPI"};
    static COUNTER_DATA directoryPercWritesFromNTDSAPI = {.key = "DS % Writes from NTDSAPI"};
    static COUNTER_DATA directoryPercWritesFromSAM = {.key = "DS % Writes from SAM"};
    static COUNTER_DATA directoryPercWritesOther = {.key = "DS % Writes from Other"};

    perflibGetObjectCounter(pDataBlock, pObjectType, &directoryPercReadsFromDCA);
    perflibGetObjectCounter(pDataBlock, pObjectType, &directoryPercReadsFromKCC);
    perflibGetObjectCounter(pDataBlock, pObjectType, &directoryPercReadsFromLSA);
    perflibGetObjectCounter(pDataBlock, pObjectType, &directoryPercReadsFromNSPI);
    perflibGetObjectCounter(pDataBlock, pObjectType, &directoryPercReadsFromNTDSAPI);
    perflibGetObjectCounter(pDataBlock, pObjectType, &directoryPercReadsFromSAM);
    perflibGetObjectCounter(pDataBlock, pObjectType, &directoryPercReadsOther);

    perflibGetObjectCounter(pDataBlock, pObjectType, &directoryPercSearchesFromDCA);
    perflibGetObjectCounter(pDataBlock, pObjectType, &directoryPercSearchesFromKCC);
    perflibGetObjectCounter(pDataBlock, pObjectType, &directoryPercSearchesFromLSA);
    perflibGetObjectCounter(pDataBlock, pObjectType, &directoryPercSearchesFromNSPI);
    perflibGetObjectCounter(pDataBlock, pObjectType, &directoryPercSearchesFromNTDSAPI);
    perflibGetObjectCounter(pDataBlock, pObjectType, &directoryPercSearchesFromSAM);
    perflibGetObjectCounter(pDataBlock, pObjectType, &directoryPercSearchesOther);

    perflibGetObjectCounter(pDataBlock, pObjectType, &directoryPercWritesFromDCA);
    perflibGetObjectCounter(pDataBlock, pObjectType, &directoryPercWritesFromKCC);
    perflibGetObjectCounter(pDataBlock, pObjectType, &directoryPercWritesFromLSA);
    perflibGetObjectCounter(pDataBlock, pObjectType, &directoryPercWritesFromNSPI);
    perflibGetObjectCounter(pDataBlock, pObjectType, &directoryPercWritesFromNTDSAPI);
    perflibGetObjectCounter(pDataBlock, pObjectType, &directoryPercWritesFromSAM);
    perflibGetObjectCounter(pDataBlock, pObjectType, &directoryPercWritesOther);

    static RRDSET *st_directory_operation_total = NULL;
    static RRDDIM *rd_directory_operation_total_read = NULL;
    static RRDDIM *rd_directory_operation_total_write = NULL;
    static RRDDIM *rd_directory_operation_total_search = NULL;

    if (unlikely(!st_directory_operation_total)) {
        st_directory_operation_total = rrdset_create_localhost(
            "ad",
            "directory_operations_read",
            NULL,
            "database",
            "ad.directory_operations",
            "AD directory operations",
            "operations/s",
            PLUGIN_WINDOWS_NAME,
            "PerflibAD",
            PRIO_AD_DIROPERATIONS_TOTAL,
            update_every,
            RRDSET_TYPE_LINE);

        rd_directory_operation_total_read =
            rrddim_add(st_directory_operation_total, "read", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);

        rd_directory_operation_total_write =
            rrddim_add(st_directory_operation_total, "write", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);

        rd_directory_operation_total_search =
            rrddim_add(st_directory_operation_total, "search", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
    }
    collected_number readValue = directoryPercReadsFromDCA.current.Data + directoryPercReadsFromKCC.current.Data +
                                 directoryPercReadsFromLSA.current.Data + directoryPercReadsFromNSPI.current.Data +
                                 directoryPercReadsFromNTDSAPI.current.Data + directoryPercReadsFromSAM.current.Data +
                                 directoryPercReadsOther.current.Data;

    collected_number writeValue = directoryPercWritesFromDCA.current.Data + directoryPercWritesFromKCC.current.Data +
                                  directoryPercWritesFromLSA.current.Data + directoryPercWritesFromNSPI.current.Data +
                                  directoryPercWritesFromNTDSAPI.current.Data +
                                  directoryPercWritesFromSAM.current.Data + directoryPercWritesOther.current.Data;

    collected_number searchValue =
        directoryPercSearchesFromDCA.current.Data + directoryPercSearchesFromKCC.current.Data +
        directoryPercSearchesFromLSA.current.Data + directoryPercSearchesFromNSPI.current.Data +
        directoryPercSearchesFromNTDSAPI.current.Data + directoryPercSearchesFromSAM.current.Data +
        directoryPercSearchesOther.current.Data;

    rrddim_set_by_pointer(st_directory_operation_total, rd_directory_operation_total_read, readValue);

    rrddim_set_by_pointer(st_directory_operation_total, rd_directory_operation_total_write, writeValue);

    rrddim_set_by_pointer(st_directory_operation_total, rd_directory_operation_total_search, searchValue);

    rrdset_done(st_directory_operation_total);
}

static void netdata_ad_cache_lookups(PERF_DATA_BLOCK *pDataBlock, PERF_OBJECT_TYPE *pObjectType, int update_every)
{
    static COUNTER_DATA nameCacheLookupsTotal = {.key = "DS Name Cache hit rate,secondvalue"};

    static RRDSET *st_name_cache_lookups_total = NULL;
    static RRDDIM *rd_name_cache_lookups_total = NULL;

    if (!perflibGetObjectCounter(pDataBlock, pObjectType, &nameCacheLookupsTotal)) {
        return;
    }

    if (unlikely(!st_name_cache_lookups_total)) {
        st_name_cache_lookups_total = rrdset_create_localhost(
                "ad",
                "name_cache_lookups",
                NULL,
                "database",
                "ad.name_cache_lookups",
                "Name cache lookups",
                "lookups/s",
                PLUGIN_WINDOWS_NAME,
                "PerflibAD",
                PRIO_AD_CACHE_LOOKUP_TOTAL,
                update_every,
                RRDSET_TYPE_LINE);

        rd_name_cache_lookups_total =
                rrddim_add(st_name_cache_lookups_total, "lookups", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
    }

    rrddim_set_by_pointer(
            st_name_cache_lookups_total,
            rd_name_cache_lookups_total,
            (collected_number)nameCacheLookupsTotal.current.Data);
    rrdset_done(st_name_cache_lookups_total);
}

static void netdata_ad_cache_hits(PERF_DATA_BLOCK *pDataBlock, PERF_OBJECT_TYPE *pObjectType, int update_every)
{
    static COUNTER_DATA nameCacheHitsTotal = {.key = "DS Name Cache hit rate"};

    static RRDSET *st_name_cache_hits_total = NULL;
    static RRDDIM *rd_name_cache_hits_total = NULL;

    if (!perflibGetObjectCounter(pDataBlock, pObjectType, &nameCacheHitsTotal)) {
        return;
    }

    if (unlikely(!st_name_cache_hits_total)) {
        st_name_cache_hits_total = rrdset_create_localhost(
                "ad",
                "name_cache_hits",
                NULL,
                "database",
                "ad.name_cache_hits",
                "Name cache hits",
                "hits/s",
                PLUGIN_WINDOWS_NAME,
                "PerflibAD",
                PRIO_AD_CACHE_HITS_TOTAL,
                update_every,
                RRDSET_TYPE_LINE);

        rd_name_cache_hits_total =
                rrddim_add(st_name_cache_hits_total, "hits", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
    }

    rrddim_set_by_pointer(
            st_name_cache_hits_total, rd_name_cache_hits_total, (collected_number)nameCacheHitsTotal.current.Data);
    rrdset_done(st_name_cache_hits_total);
}

static void netdata_ad_searches(PERF_DATA_BLOCK *pDataBlock, PERF_OBJECT_TYPE *pObjectType, int update_every)
{
    static COUNTER_DATA ldapSearchesTotal = {.key = "LDAP Searches/sec"};

    static RRDSET *st_ldap_searches_total = NULL;
    static RRDDIM *rd_ldap_searches_total = NULL;

    if (!perflibGetObjectCounter(pDataBlock, pObjectType, &ldapSearchesTotal)) {
        return;
    }

    if (unlikely(!st_ldap_searches_total)) {
        st_ldap_searches_total = rrdset_create_localhost(
                "ad",
                "ldap_searches",
                NULL,
                "ldap",
                "ad.ldap_searches",
                "LDAP client search operations",
                "searches/s",
                PLUGIN_WINDOWS_NAME,
                "PerflibAD",
                PRIO_AD_LDAP_SEARCHES_TOTAL,
                update_every,
                RRDSET_TYPE_LINE);

        rd_ldap_searches_total =
                rrddim_add(st_ldap_searches_total, "searches", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
    }

    rrddim_set_by_pointer(
            st_ldap_searches_total, rd_ldap_searches_total, (collected_number)ldapSearchesTotal.current.Data);
    rrdset_done(st_ldap_searches_total);
}

static void netdata_ad_properties(PERF_DATA_BLOCK *pDataBlock, PERF_OBJECT_TYPE *pObjectType, int update_every)
{
    static COUNTER_DATA replicationInboundPropertiesUpdatedTotal = {.key = "DRA Inbound Properties Applied/sec"};
    static COUNTER_DATA replicationInboundPropertiesFilteredTotal = {.key = "DRA Inbound Properties Filtered/sec"};

    static RRDSET *st_dra_replication_properties_updated = NULL;
    static RRDDIM *rd_dra_replication_properties_updated = NULL;
    static RRDSET *st_dra_replication_properties_filtered = NULL;
    static RRDDIM *rd_dra_replication_properties_filtered = NULL;

    if (!perflibGetObjectCounter(pDataBlock, pObjectType, &replicationInboundPropertiesUpdatedTotal)) {
        goto totalPropertyChart;
    }

    if (unlikely(!st_dra_replication_properties_updated)) {
        st_dra_replication_properties_updated = rrdset_create_localhost(
            "ad",
            "dra_replication_properties_updated",
            NULL,
            "replication",
            "ad.dra_replication_properties_updated",
            "DRA replication properties updated",
            "properties/s",
            PLUGIN_WINDOWS_NAME,
            "PerflibAD",
            PRIO_AD_REPLICATION_PROPERTY_UPDATED,
            update_every,
            RRDSET_TYPE_LINE);

        rd_dra_replication_properties_updated =
            rrddim_add(st_dra_replication_properties_updated, "inbound", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
    }

    rrddim_set_by_pointer(
        st_dra_replication_properties_updated,
        rd_dra_replication_properties_updated,
        (collected_number)replicationInboundPropertiesUpdatedTotal.current.Data);
    rrdset_done(st_dra_replication_properties_updated);

totalPropertyChart:
    if (!perflibGetObjectCounter(pDataBlock, pObjectType, &replicationInboundPropertiesFilteredTotal)) {
        return;
    }

    if (unlikely(!st_dra_replication_properties_filtered)) {
        st_dra_replication_properties_filtered = rrdset_create_localhost(
            "ad",
            "dra_replication_properties_filtered",
            NULL,
            "replication",
            "ad.dra_replication_properties_filtered",
            "DRA replication properties filtered",
            "properties/s",
            PLUGIN_WINDOWS_NAME,
            "PerflibAD",
            PRIO_AD_REPLICATION_PROPERTY_FILTERED,
            update_every,
            RRDSET_TYPE_LINE);

        rd_dra_replication_properties_filtered =
            rrddim_add(st_dra_replication_properties_filtered, "inbound", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
    }

    rrddim_set_by_pointer(
        st_dra_replication_properties_filtered,
        rd_dra_replication_properties_filtered,
        (collected_number)replicationInboundPropertiesFilteredTotal.current.Data);
    rrdset_done(st_dra_replication_properties_filtered);
}

static void netdata_ad_compressed_traffic(PERF_DATA_BLOCK *pDataBlock, PERF_OBJECT_TYPE *pObjectType, int update_every)
{
    static COUNTER_DATA replicationInboundDataInterSiteBytesTotal = {
        .key = "DRA Inbound Bytes Compressed (Between Sites, After Compression)/sec"};
    static COUNTER_DATA replicationOutboundDataInterSiteBytesTotal = {
        .key = "DRA Outbound Bytes Compressed (Between Sites, After Compression)/sec"};

    static COUNTER_DATA replicationDataInboundIntraSiteBytesTotal = {
        .key = "DRA Outbound Bytes Compressed (Between Sites, After Compression)/sec"};
    static COUNTER_DATA replicationDataOutboundIntraSiteBytesTotal = {
        .key = "DRA Outbound Bytes Not Compressed (Within Site)/sec"};

    static RRDSET *st_dra_replication_intersite_compressed_traffic = NULL;
    static RRDDIM *rd_replication_data_intersite_bytes_total_inbound = NULL;
    static RRDDIM *rd_replication_data_intersite_bytes_total_outbound = NULL;

    static RRDSET *st_dra_replication_intrasite_compressed_traffic = NULL;
    static RRDDIM *rd_replication_data_intrasite_bytes_total_inbound = NULL;
    static RRDDIM *rd_replication_data_intrasite_bytes_total_outbound = NULL;

    if (!perflibGetObjectCounter(pDataBlock, pObjectType, &replicationInboundDataInterSiteBytesTotal) ||
        !perflibGetObjectCounter(pDataBlock, pObjectType, &replicationOutboundDataInterSiteBytesTotal)) {
        goto intraChart;
    }

    if (unlikely(!st_dra_replication_intersite_compressed_traffic)) {
        st_dra_replication_intersite_compressed_traffic = rrdset_create_localhost(
            "ad",
            "dra_replication_intersite_compressed_traffic",
            NULL,
            "replication",
            "ad.dra_replication_intersite_compressed_traffic",
            "DRA replication compressed traffic withing site",
            "bytes/s",
            PLUGIN_WINDOWS_NAME,
            "PerflibAD",
            PRIO_AD_REPLICATION_INTERSITE_COMPRESSED,
            update_every,
            RRDSET_TYPE_AREA);

        rd_replication_data_intersite_bytes_total_inbound = rrddim_add(
            st_dra_replication_intersite_compressed_traffic, "inbound", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);

        rd_replication_data_intersite_bytes_total_outbound = rrddim_add(
            st_dra_replication_intersite_compressed_traffic, "outbound", NULL, -1, 1, RRD_ALGORITHM_INCREMENTAL);
    }

    rrddim_set_by_pointer(
        st_dra_replication_intersite_compressed_traffic,
        rd_replication_data_intersite_bytes_total_inbound,
        (collected_number)replicationInboundDataInterSiteBytesTotal.current.Data);
    rrddim_set_by_pointer(
        st_dra_replication_intersite_compressed_traffic,
        rd_replication_data_intersite_bytes_total_outbound,
        (collected_number)replicationOutboundDataInterSiteBytesTotal.current.Data);
    rrdset_done(st_dra_replication_intersite_compressed_traffic);

intraChart:
    if (!perflibGetObjectCounter(pDataBlock, pObjectType, &replicationDataInboundIntraSiteBytesTotal) ||
        !perflibGetObjectCounter(pDataBlock, pObjectType, &replicationDataOutboundIntraSiteBytesTotal)) {
        return;
    }

    if (unlikely(!st_dra_replication_intrasite_compressed_traffic)) {
        st_dra_replication_intrasite_compressed_traffic = rrdset_create_localhost(
            "ad",
            "dra_replication_intrasite_compressed_traffic",
            NULL,
            "replication",
            "ad.dra_replication_intrasite_compressed_traffic",
            "DRA replication compressed traffic between sites",
            "bytes/s",
            PLUGIN_WINDOWS_NAME,
            "PerflibAD",
            PRIO_AD_REPLICATION_INTRASITE_COMPRESSED,
            update_every,
            RRDSET_TYPE_AREA);

        rd_replication_data_intrasite_bytes_total_inbound = rrddim_add(
            st_dra_replication_intrasite_compressed_traffic, "inbound", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);

        rd_replication_data_intrasite_bytes_total_outbound = rrddim_add(
            st_dra_replication_intrasite_compressed_traffic, "outbound", NULL, -1, 1, RRD_ALGORITHM_INCREMENTAL);
    }

    rrddim_set_by_pointer(
        st_dra_replication_intrasite_compressed_traffic,
        rd_replication_data_intrasite_bytes_total_inbound,
        (collected_number)replicationDataInboundIntraSiteBytesTotal.current.Data);
    rrddim_set_by_pointer(
        st_dra_replication_intrasite_compressed_traffic,
        rd_replication_data_intrasite_bytes_total_outbound,
        (collected_number)replicationDataOutboundIntraSiteBytesTotal.current.Data);
    rrdset_done(st_dra_replication_intrasite_compressed_traffic);
}

static void netdata_ad_sync(PERF_DATA_BLOCK *pDataBlock, PERF_OBJECT_TYPE *pObjectType, int update_every)
{
    static COUNTER_DATA replicationSyncPending = {.key = "DRA Pending Replication Synchronizations"};
    static COUNTER_DATA replicationSyncRequestsTotal = {.key = "DRA Sync Requests Made"};
    static COUNTER_DATA replicationPendingSyncs = {.key = "DRA Pending Replication Synchronizations"};

    static RRDSET *st_dra_replication_pending_syncs = NULL;
    static RRDDIM *rd_dra_replication_pending_syncs = NULL;
    static RRDSET *st_dra_replication_sync_requests = NULL;
    static RRDDIM *rd_dra_replication_sync_requests = NULL;
    static RRDSET *st_dra_replication_sync_objects_remaining = NULL;
    static RRDDIM *rd_dra_replication_sync_objects_remaining = NULL;

    if (!perflibGetObjectCounter(pDataBlock, pObjectType, &replicationSyncPending)) {
        goto totalSyncChart;
    }

    if (unlikely(!st_dra_replication_pending_syncs)) {
        st_dra_replication_pending_syncs = rrdset_create_localhost(
            "ad",
            "dra_replication_pending_syncs",
            NULL,
            "replication",
            "ad.dra_replication_pending_syncs",
            "DRA replication pending syncs",
            "syncs",
            PLUGIN_WINDOWS_NAME,
            "PerflibAD",
            PRIO_AD_SYNC_PENDING,
            update_every,
            RRDSET_TYPE_LINE);

        rd_dra_replication_pending_syncs =
            rrddim_add(st_dra_replication_pending_syncs, "pending", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
    }

    rrddim_set_by_pointer(
        st_dra_replication_pending_syncs,
        rd_dra_replication_pending_syncs,
        (collected_number)replicationSyncPending.current.Data);
    rrdset_done(st_dra_replication_pending_syncs);

totalSyncChart:
    if (!perflibGetObjectCounter(pDataBlock, pObjectType, &replicationSyncRequestsTotal)) {
        goto RemainingSyncChart;
    }

    if (unlikely(!st_dra_replication_sync_requests)) {
        st_dra_replication_sync_requests = rrdset_create_localhost(
            "ad",
            "dra_replication_sync_requests",
            NULL,
            "replication",
            "ad.dra_replication_sync_requests",
            "DRA replication sync requests",
            "requests/s",
            PLUGIN_WINDOWS_NAME,
            "PerflibAD",
            PRIO_AD_SYNC_TOTAL,
            update_every,
            RRDSET_TYPE_LINE);

        rd_dra_replication_sync_requests =
            rrddim_add(st_dra_replication_sync_requests, "request", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
    }

    rrddim_set_by_pointer(
        st_dra_replication_sync_requests,
        rd_dra_replication_sync_requests,
        (collected_number)replicationSyncRequestsTotal.current.Data);
    rrdset_done(st_dra_replication_sync_requests);

RemainingSyncChart:
    if (!perflibGetObjectCounter(pDataBlock, pObjectType, &replicationPendingSyncs)) {
        return;
    }

    if (unlikely(!st_dra_replication_sync_objects_remaining)) {
        st_dra_replication_sync_objects_remaining = rrdset_create_localhost(
            "ad",
            "dra_replication_sync_objects_remaining",
            NULL,
            "replication",
            "ad.dra_replication_sync_objects_remaining",
            "DRA replication full sync objects remaining",
            "objects",
            PLUGIN_WINDOWS_NAME,
            "PerflibAD",
            PRIO_AD_SYNC_OBJECTS_REMAINING,
            update_every,
            RRDSET_TYPE_LINE);

        rd_dra_replication_sync_objects_remaining =
            rrddim_add(st_dra_replication_sync_objects_remaining, "object", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
    }

    rrddim_set_by_pointer(
        st_dra_replication_sync_objects_remaining,
        rd_dra_replication_sync_objects_remaining,
        (collected_number)replicationPendingSyncs.current.Data);
    rrdset_done(st_dra_replication_sync_objects_remaining);
}

static void
netdata_ad_service_threads_in_use(PERF_DATA_BLOCK *pDataBlock, PERF_OBJECT_TYPE *pObjectType, int update_every)
{
    static COUNTER_DATA directoryServiceThreads = {.key = "DS Threads in Use"};

    static RRDSET *st_directory_services_threads = NULL;
    static RRDDIM *rd_directory_services_threads = NULL;

    if (!perflibGetObjectCounter(pDataBlock, pObjectType, &directoryServiceThreads)) {
        return;
    }

    if (unlikely(!st_directory_services_threads)) {
        st_directory_services_threads = rrdset_create_localhost(
                "ad",
                "ds_threads",
                NULL,
                "threads",
                "ad.ds_threads",
                "Directory Service threads",
                "threads",
                PLUGIN_WINDOWS_NAME,
                "PerflibAD",
                PRIO_AD_THREADS_IN_USE,
                update_every,
                RRDSET_TYPE_LINE);

        rd_directory_services_threads =
                rrddim_add(st_directory_services_threads, "thread", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
    }

    rrddim_set_by_pointer(
            st_directory_services_threads,
            rd_directory_services_threads,
            (collected_number)directoryServiceThreads.current.Data);
    rrdset_done(st_directory_services_threads);
}

static void netdata_ad_bind_time(PERF_DATA_BLOCK *pDataBlock, PERF_OBJECT_TYPE *pObjectType, int update_every)
{
    static COUNTER_DATA ldapLastBindTimeSecondsTotal = {.key = "DAP Bind Time"};
    static RRDSET *st_ldap_last_bind_time_seconds_total = NULL;
    static RRDDIM *rd_ldap_last_bind_time_seconds_total = NULL;

    if (!perflibGetObjectCounter(pDataBlock, pObjectType, &ldapLastBindTimeSecondsTotal)) {
        return;
    }

    if (unlikely(!st_ldap_last_bind_time_seconds_total)) {
        st_ldap_last_bind_time_seconds_total = rrdset_create_localhost(
                "ad",
                "ldap_last_bind_time",
                NULL,
                "bind",
                "ad.ldap_last_bind_time",
                "LDAP last successful bind time",
                "seconds",
                PLUGIN_WINDOWS_NAME,
                "PerflibAD",
                PRIO_AD_BIND_TIME,
                update_every,
                RRDSET_TYPE_LINE);

        rd_ldap_last_bind_time_seconds_total =
                rrddim_add(st_ldap_last_bind_time_seconds_total, "last_bind", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
    }

    rrddim_set_by_pointer(
            st_ldap_last_bind_time_seconds_total,
            rd_ldap_last_bind_time_seconds_total,
            (collected_number)ldapLastBindTimeSecondsTotal.current.Data);
    rrdset_done(st_ldap_last_bind_time_seconds_total);
}

static void netdata_ad_binds(PERF_DATA_BLOCK *pDataBlock, PERF_OBJECT_TYPE *pObjectType, int update_every)
{
    static COUNTER_DATA bindsTotal = {.key = "DS Server Binds/sec"};

    static RRDSET *st_binds_total = NULL;
    static RRDDIM *rd_binds_total = NULL;

    if (perflibGetObjectCounter(pDataBlock, pObjectType, &bindsTotal)) {
        if (unlikely(!st_binds_total)) {
            st_binds_total = rrdset_create_localhost(
                "ad",
                "binds",
                NULL,
                "bind",
                "ad.binds",
                "Successful binds",
                "bind/s",
                PLUGIN_WINDOWS_NAME,
                "PerflibAD",
                PRIO_AD_BIND_TOTAL,
                update_every,
                RRDSET_TYPE_LINE);

            rd_binds_total = rrddim_add(st_binds_total, "binds", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
        }

        rrddim_set_by_pointer(st_binds_total, rd_binds_total, (collected_number)bindsTotal.current.Data);
        rrdset_done(st_binds_total);
    }
}

static void netdata_ad_bind(PERF_DATA_BLOCK *pDataBlock, PERF_OBJECT_TYPE *pObjectType, int update_every)
{
    netdata_ad_bind_time(pDataBlock, pObjectType, update_every);
    netdata_ad_binds(pDataBlock, pObjectType, update_every);
}

/* TODO: Check why values are growing forever in our setup
static void netdata_ad_atq_queue_requests(PERF_DATA_BLOCK *pDataBlock, PERF_OBJECT_TYPE *pObjectType, int update_every)
{
    static COUNTER_DATA atqOutstandingRequests = { .key = "ATQ Outstanding Queued Requests" };
    static RRDSET *st_atq_outstanding_requests = NULL;
    static RRDDIM *rd_atq_outstanding_requests = NULL;

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
                              (collected_number)atqOutstandingRequests.current.Data);
        rrdset_done(st_atq_outstanding_requests);
    }
}
 */

static void netdata_ad_atq_latency(PERF_DATA_BLOCK *pDataBlock, PERF_OBJECT_TYPE *pObjectType, int update_every)
{
    static COUNTER_DATA atqAverageRequestLatency = {.key = "ATQ Request Latency"};

    static RRDSET *st_atq_average_request_latency = NULL;
    static RRDDIM *rd_atq_average_request_latency = NULL;

    if (perflibGetObjectCounter(pDataBlock, pObjectType, &atqAverageRequestLatency)) {
        if (unlikely(!st_atq_average_request_latency)) {
            st_atq_average_request_latency = rrdset_create_localhost(
                "ad",
                "atq_average_request_latency",
                NULL,
                "queue",
                "ad.atq_average_request_latency",
                "Average request processing time",
                "seconds",
                PLUGIN_WINDOWS_NAME,
                "PerflibAD",
                PRIO_AD_AVG_REQUEST_LATENCY,
                update_every,
                RRDSET_TYPE_LINE);

            rd_atq_average_request_latency =
                rrddim_add(st_atq_average_request_latency, "time", NULL, 1, 1000, RRD_ALGORITHM_ABSOLUTE);
        }

        rrddim_set_by_pointer(
            st_atq_average_request_latency,
            rd_atq_average_request_latency,
            (collected_number)atqAverageRequestLatency.current.Data);
        rrdset_done(st_atq_average_request_latency);
    }
}

static void netdata_ad_op_total(PERF_DATA_BLOCK *pDataBlock, PERF_OBJECT_TYPE *pObjectType, int update_every)
{
    static COUNTER_DATA databaseAddsPerSec = {.key = "Database adds/sec"};
    static COUNTER_DATA databaseDeletesPerSec = {.key = "Database deletes/sec"};
    static COUNTER_DATA databaseModifiesPerSec = {.key = "Database modifys/sec"};
    static COUNTER_DATA databaseRecyclesPerSec = {.key = "Database recycles/sec"};

    static RRDSET *st_database_operation_total = NULL;
    static RRDDIM *rd_database_operation_total_add = NULL;
    static RRDDIM *rd_database_operation_total_delete = NULL;
    static RRDDIM *rd_database_operation_total_modify = NULL;
    static RRDDIM *rd_database_operation_total_recycle = NULL;

    perflibGetObjectCounter(pDataBlock, pObjectType, &databaseAddsPerSec);
    perflibGetObjectCounter(pDataBlock, pObjectType, &databaseDeletesPerSec);
    perflibGetObjectCounter(pDataBlock, pObjectType, &databaseModifiesPerSec);
    perflibGetObjectCounter(pDataBlock, pObjectType, &databaseRecyclesPerSec);

    if (unlikely(!st_database_operation_total)) {
        st_database_operation_total = rrdset_create_localhost(
            "ad",
            "database_operations",
            NULL,
            "database",
            "ad.database_operations",
            "AD database operations",
            "operations/s",
            PLUGIN_WINDOWS_NAME,
            "PerflibAD",
            PRIO_AD_OPERATIONS_TOTAL,
            update_every,
            RRDSET_TYPE_LINE);

        rd_database_operation_total_add =
            rrddim_add(st_database_operation_total, "add", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);

        rd_database_operation_total_delete =
            rrddim_add(st_database_operation_total, "delete", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);

        rd_database_operation_total_modify =
            rrddim_add(st_database_operation_total, "modify", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);

        rd_database_operation_total_recycle =
            rrddim_add(st_database_operation_total, "recycle", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
    }

    rrddim_set_by_pointer(
        st_database_operation_total,
        rd_database_operation_total_add,
        (collected_number)databaseAddsPerSec.current.Data);

    rrddim_set_by_pointer(
        st_database_operation_total,
        rd_database_operation_total_delete,
        (collected_number)databaseDeletesPerSec.current.Data);

    rrddim_set_by_pointer(
        st_database_operation_total,
        rd_database_operation_total_modify,
        (collected_number)databaseModifiesPerSec.current.Data);

    rrddim_set_by_pointer(
        st_database_operation_total,
        rd_database_operation_total_recycle,
        (collected_number)databaseRecyclesPerSec.current.Data);

    rrdset_done(st_database_operation_total);
}

static bool do_AD(PERF_DATA_BLOCK *pDataBlock, int update_every)
{
    PERF_OBJECT_TYPE *pObjectType = perflibFindObjectTypeByName(pDataBlock, "DirectoryServices");
    if (!pObjectType)
        return false;

    static void (*doAD[])(PERF_DATA_BLOCK *, PERF_OBJECT_TYPE *, int) = {
            netdata_ad_directory,
            netdata_ad_cache_lookups,
            netdata_ad_properties,
            netdata_ad_compressed_traffic,
            netdata_ad_sync,
            netdata_ad_cache_hits,
            netdata_ad_service_threads_in_use,
            netdata_ad_bind,
            netdata_ad_searches,
            netdata_ad_atq_latency,
            netdata_ad_op_total,

        // This must be the end
        NULL};

    for (int i = 0; doAD[i]; i++)
        doAD[i](pDataBlock, pObjectType, update_every);

    return true;
}

int do_PerflibAD(int update_every, usec_t dt __maybe_unused)
{
    DWORD id = RegistryFindIDByName("DirectoryServices");
    if (id == PERFLIB_REGISTRY_NAME_NOT_FOUND)
        return -1;

    PERF_DATA_BLOCK *pDataBlock = perflibGetPerformanceData(id);
    if (!pDataBlock)
        return -1;

    if (!do_AD(pDataBlock, update_every))
        return -1;

    return 0;
}
