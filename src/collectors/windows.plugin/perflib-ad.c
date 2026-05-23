// SPDX-License-Identifier: GPL-3.0-or-later

#include "windows_plugin.h"
#include "windows-internals.h"

static void netdata_ad_address_book_ambiguous_name_resolution(
    PERF_DATA_BLOCK *pDataBlock,
    PERF_OBJECT_TYPE *pObjectType,
    int update_every)
{
    static COUNTER_DATA addressBookANRPerSec = {.key = "AB ANR/sec"};
    static RRDSET *st_address_book_ambiguous_name_resolution = NULL;
    static RRDDIM *rd_address_book_ambiguous_name_resolution = NULL;

    if (!perflibGetObjectCounter(pDataBlock, pObjectType, &addressBookANRPerSec))
        return;

    if (unlikely(!st_address_book_ambiguous_name_resolution)) {
        st_address_book_ambiguous_name_resolution = rrdset_create_localhost(
            "ad",
            "address_book_ambiguous_name_resolution",
            NULL,
            "address_book",
            "ad.address_book_ambiguous_name_resolution",
            "Address book ambiguous name resolution",
            "operations/s",
            PLUGIN_WINDOWS_NAME,
            "PerflibAD",
            PRIO_AD_ADDRESS_BOOK_AMBIGUOUS_NAME_RESOLUTION,
            update_every,
            RRDSET_TYPE_LINE);

        rd_address_book_ambiguous_name_resolution = rrddim_add(
            st_address_book_ambiguous_name_resolution,
            "ambiguous_name_resolution",
            NULL,
            1,
            1,
            RRD_ALGORITHM_INCREMENTAL);
    }

    rrddim_set_by_pointer(
        st_address_book_ambiguous_name_resolution,
        rd_address_book_ambiguous_name_resolution,
        (collected_number)addressBookANRPerSec.current.Data);
    rrdset_done(st_address_book_ambiguous_name_resolution);
}

static void netdata_ad_address_book_browse(PERF_DATA_BLOCK *pDataBlock, PERF_OBJECT_TYPE *pObjectType, int update_every)
{
    static COUNTER_DATA addressBookBrowsesPerSec = {.key = "AB Browses/sec"};
    static RRDSET *st_address_book_browse = NULL;
    static RRDDIM *rd_address_book_browse = NULL;

    if (!perflibGetObjectCounter(pDataBlock, pObjectType, &addressBookBrowsesPerSec))
        return;

    if (unlikely(!st_address_book_browse)) {
        st_address_book_browse = rrdset_create_localhost(
            "ad",
            "address_book_browse",
            NULL,
            "address_book",
            "ad.address_book_browse",
            "Address book browse operations",
            "operations/s",
            PLUGIN_WINDOWS_NAME,
            "PerflibAD",
            PRIO_AD_ADDRESS_BOOK_BROWSE,
            update_every,
            RRDSET_TYPE_LINE);

        rd_address_book_browse = rrddim_add(st_address_book_browse, "browse", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
    }

    rrddim_set_by_pointer(
        st_address_book_browse, rd_address_book_browse, (collected_number)addressBookBrowsesPerSec.current.Data);
    rrdset_done(st_address_book_browse);
}

static void netdata_ad_address_book_find(PERF_DATA_BLOCK *pDataBlock, PERF_OBJECT_TYPE *pObjectType, int update_every)
{
    static COUNTER_DATA addressBookMatchesPerSec = {.key = "AB Matches/sec"};
    static RRDSET *st_address_book_find = NULL;
    static RRDDIM *rd_address_book_find = NULL;

    if (!perflibGetObjectCounter(pDataBlock, pObjectType, &addressBookMatchesPerSec))
        return;

    if (unlikely(!st_address_book_find)) {
        st_address_book_find = rrdset_create_localhost(
            "ad",
            "address_book_find",
            NULL,
            "address_book",
            "ad.address_book_find",
            "Address book matches",
            "operations/s",
            PLUGIN_WINDOWS_NAME,
            "PerflibAD",
            PRIO_AD_ADDRESS_BOOK_FIND,
            update_every,
            RRDSET_TYPE_LINE);

        rd_address_book_find = rrddim_add(st_address_book_find, "find", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
    }

    rrddim_set_by_pointer(
        st_address_book_find, rd_address_book_find, (collected_number)addressBookMatchesPerSec.current.Data);
    rrdset_done(st_address_book_find);
}

static void
netdata_ad_address_book_property_read(PERF_DATA_BLOCK *pDataBlock, PERF_OBJECT_TYPE *pObjectType, int update_every)
{
    static COUNTER_DATA addressBookPropertyReadsPerSec = {.key = "AB Property Reads/sec"};
    static RRDSET *st_address_book_property_read = NULL;
    static RRDDIM *rd_address_book_property_read = NULL;

    if (!perflibGetObjectCounter(pDataBlock, pObjectType, &addressBookPropertyReadsPerSec))
        return;

    if (unlikely(!st_address_book_property_read)) {
        st_address_book_property_read = rrdset_create_localhost(
            "ad",
            "address_book_property_read",
            NULL,
            "address_book",
            "ad.address_book_property_read",
            "Address book property reads",
            "operations/s",
            PLUGIN_WINDOWS_NAME,
            "PerflibAD",
            PRIO_AD_ADDRESS_BOOK_PROPERTY_READ,
            update_every,
            RRDSET_TYPE_LINE);

        rd_address_book_property_read =
            rrddim_add(st_address_book_property_read, "property_read", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
    }

    rrddim_set_by_pointer(
        st_address_book_property_read,
        rd_address_book_property_read,
        (collected_number)addressBookPropertyReadsPerSec.current.Data);
    rrdset_done(st_address_book_property_read);
}

static void netdata_ad_address_book_search(PERF_DATA_BLOCK *pDataBlock, PERF_OBJECT_TYPE *pObjectType, int update_every)
{
    static COUNTER_DATA addressBookSearchesPerSec = {.key = "AB Searches/sec"};
    static RRDSET *st_address_book_search = NULL;
    static RRDDIM *rd_address_book_search = NULL;

    if (!perflibGetObjectCounter(pDataBlock, pObjectType, &addressBookSearchesPerSec))
        return;

    if (unlikely(!st_address_book_search)) {
        st_address_book_search = rrdset_create_localhost(
            "ad",
            "address_book_search",
            NULL,
            "address_book",
            "ad.address_book_search",
            "Address book searches",
            "operations/s",
            PLUGIN_WINDOWS_NAME,
            "PerflibAD",
            PRIO_AD_ADDRESS_BOOK_SEARCH,
            update_every,
            RRDSET_TYPE_LINE);

        rd_address_book_search = rrddim_add(st_address_book_search, "search", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
    }

    rrddim_set_by_pointer(
        st_address_book_search, rd_address_book_search, (collected_number)addressBookSearchesPerSec.current.Data);
    rrdset_done(st_address_book_search);
}

static void
netdata_ad_address_book_proxy_search(PERF_DATA_BLOCK *pDataBlock, PERF_OBJECT_TYPE *pObjectType, int update_every)
{
    static COUNTER_DATA addressBookProxyLookupsPerSec = {.key = "AB Proxy Lookups/sec"};
    static RRDSET *st_address_book_proxy_search = NULL;
    static RRDDIM *rd_address_book_proxy_search = NULL;

    if (!perflibGetObjectCounter(pDataBlock, pObjectType, &addressBookProxyLookupsPerSec))
        return;

    if (unlikely(!st_address_book_proxy_search)) {
        st_address_book_proxy_search = rrdset_create_localhost(
            "ad",
            "address_book_proxy_search",
            NULL,
            "address_book",
            "ad.address_book_proxy_search",
            "Address book proxy searches",
            "operations/s",
            PLUGIN_WINDOWS_NAME,
            "PerflibAD",
            PRIO_AD_ADDRESS_BOOK_PROXY_SEARCH,
            update_every,
            RRDSET_TYPE_LINE);

        rd_address_book_proxy_search =
            rrddim_add(st_address_book_proxy_search, "proxy_search", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
    }

    rrddim_set_by_pointer(
        st_address_book_proxy_search,
        rd_address_book_proxy_search,
        (collected_number)addressBookProxyLookupsPerSec.current.Data);
    rrdset_done(st_address_book_proxy_search);
}

static void
netdata_ad_address_book_client_sessions(PERF_DATA_BLOCK *pDataBlock, PERF_OBJECT_TYPE *pObjectType, int update_every)
{
    static COUNTER_DATA addressBookClientSessions = {.key = "AB Client Sessions"};
    static RRDSET *st_address_book_client_sessions = NULL;
    static RRDDIM *rd_address_book_client_sessions = NULL;

    if (!perflibGetObjectCounter(pDataBlock, pObjectType, &addressBookClientSessions))
        return;

    if (unlikely(!st_address_book_client_sessions)) {
        st_address_book_client_sessions = rrdset_create_localhost(
            "ad",
            "address_book_client_sessions",
            NULL,
            "address_book",
            "ad.address_book_client_sessions",
            "Address book client sessions",
            "sessions",
            PLUGIN_WINDOWS_NAME,
            "PerflibAD",
            PRIO_AD_ADDRESS_BOOK_CLIENT_SESSIONS,
            update_every,
            RRDSET_TYPE_LINE);

        rd_address_book_client_sessions =
            rrddim_add(st_address_book_client_sessions, "sessions", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
    }

    rrddim_set_by_pointer(
        st_address_book_client_sessions,
        rd_address_book_client_sessions,
        (collected_number)addressBookClientSessions.current.Data);
    rrdset_done(st_address_book_client_sessions);
}

static void netdata_ad_address_book(PERF_DATA_BLOCK *pDataBlock, PERF_OBJECT_TYPE *pObjectType, int update_every)
{
    netdata_ad_address_book_ambiguous_name_resolution(pDataBlock, pObjectType, update_every);
    netdata_ad_address_book_browse(pDataBlock, pObjectType, update_every);
    netdata_ad_address_book_find(pDataBlock, pObjectType, update_every);
    netdata_ad_address_book_property_read(pDataBlock, pObjectType, update_every);
    netdata_ad_address_book_search(pDataBlock, pObjectType, update_every);
    netdata_ad_address_book_proxy_search(pDataBlock, pObjectType, update_every);
    netdata_ad_address_book_client_sessions(pDataBlock, pObjectType, update_every);
}

static void
netdata_ad_approximate_highest_dnt(PERF_DATA_BLOCK *pDataBlock, PERF_OBJECT_TYPE *pObjectType, int update_every)
{
    static COUNTER_DATA approximateHighestDNT = {.key = "Approximate highest DNT"};

    static RRDSET *st_approximate_highest_dnt = NULL;
    static RRDDIM *rd_approximate_highest_dnt = NULL;

    if (!perflibGetObjectCounter(pDataBlock, pObjectType, &approximateHighestDNT))
        return;

    if (unlikely(!st_approximate_highest_dnt)) {
        st_approximate_highest_dnt = rrdset_create_localhost(
            "ad",
            "approximate_highest_dnt",
            NULL,
            "database",
            "ad.approximate_highest_dnt",
            "Approximate highest distinguished name tag",
            "dnt",
            PLUGIN_WINDOWS_NAME,
            "PerflibAD",
            PRIO_AD_APPROXIMATE_HIGHEST_DNT,
            update_every,
            RRDSET_TYPE_LINE);

        rd_approximate_highest_dnt = rrddim_add(st_approximate_highest_dnt, "dnt", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
    }

    rrddim_set_by_pointer(
        st_approximate_highest_dnt, rd_approximate_highest_dnt, (collected_number)approximateHighestDNT.current.Data);
    rrdset_done(st_approximate_highest_dnt);
}

static void netdata_ad_directory(PERF_DATA_BLOCK *pDataBlock, PERF_OBJECT_TYPE *pObjectType, int update_every)
{
    static COUNTER_DATA directoryPercReadsFromDCA = {.key = "DS % Reads from DRA"};
    static COUNTER_DATA directoryPercReadsFromKCC = {.key = "DS % Reads from KCC"};
    static COUNTER_DATA directoryPercReadsFromLSA = {.key = "DS % Reads from LSA"};
    static COUNTER_DATA directoryPercReadsFromNSPI = {.key = "DS % Reads from NSPI"};
    static COUNTER_DATA directoryPercReadsFromNTDSAPI = {.key = "DS % Reads from NTDSAPI"};
    static COUNTER_DATA directoryPercReadsFromSAM = {.key = "DS % Reads from SAM"};
    static COUNTER_DATA directoryPercReadsOther = {.key = "DS % Reads Other"};

    static COUNTER_DATA directoryPercSearchesFromDCA = {.key = "DS % Searches from DRA"};
    static COUNTER_DATA directoryPercSearchesFromKCC = {.key = "DS % Searches from KCC"};
    static COUNTER_DATA directoryPercSearchesFromLSA = {.key = "DS % Searches from LSA"};
    static COUNTER_DATA directoryPercSearchesFromNSPI = {.key = "DS % Searches from NSPI"};
    static COUNTER_DATA directoryPercSearchesFromNTDSAPI = {.key = "DS % Searches from NTDSAPI"};
    static COUNTER_DATA directoryPercSearchesFromSAM = {.key = "DS % Searches from SAM"};
    static COUNTER_DATA directoryPercSearchesFromLDAP = {.key = "DS % Searches from LDAP"};
    static COUNTER_DATA directoryPercSearchesOther = {.key = "DS % Searches Other"};

    static COUNTER_DATA directoryPercWritesFromDCA = {.key = "DS % Writes from DRA"};
    static COUNTER_DATA directoryPercWritesFromKCC = {.key = "DS % Writes from KCC"};
    static COUNTER_DATA directoryPercWritesFromLSA = {.key = "DS % Writes from LSA"};
    static COUNTER_DATA directoryPercWritesFromNSPI = {.key = "DS % Writes from NSPI"};
    static COUNTER_DATA directoryPercWritesFromNTDSAPI = {.key = "DS % Writes from NTDSAPI"};
    static COUNTER_DATA directoryPercWritesFromSAM = {.key = "DS % Writes from SAM"};
    static COUNTER_DATA directoryPercWritesFromLDAP = {.key = "DS % Writes from LDAP"};
    static COUNTER_DATA directoryPercWritesOther = {.key = "DS % Writes Other"};

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
    perflibGetObjectCounter(pDataBlock, pObjectType, &directoryPercSearchesFromLDAP);
    perflibGetObjectCounter(pDataBlock, pObjectType, &directoryPercSearchesOther);

    perflibGetObjectCounter(pDataBlock, pObjectType, &directoryPercWritesFromDCA);
    perflibGetObjectCounter(pDataBlock, pObjectType, &directoryPercWritesFromKCC);
    perflibGetObjectCounter(pDataBlock, pObjectType, &directoryPercWritesFromLSA);
    perflibGetObjectCounter(pDataBlock, pObjectType, &directoryPercWritesFromNSPI);
    perflibGetObjectCounter(pDataBlock, pObjectType, &directoryPercWritesFromNTDSAPI);
    perflibGetObjectCounter(pDataBlock, pObjectType, &directoryPercWritesFromSAM);
    perflibGetObjectCounter(pDataBlock, pObjectType, &directoryPercWritesFromLDAP);
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
                                  directoryPercWritesFromSAM.current.Data + directoryPercWritesFromLDAP.current.Data +
                                  directoryPercWritesOther.current.Data;

    collected_number searchValue =
        directoryPercSearchesFromDCA.current.Data + directoryPercSearchesFromKCC.current.Data +
        directoryPercSearchesFromLSA.current.Data + directoryPercSearchesFromNSPI.current.Data +
        directoryPercSearchesFromNTDSAPI.current.Data + directoryPercSearchesFromSAM.current.Data +
        directoryPercSearchesFromLDAP.current.Data + directoryPercSearchesOther.current.Data;

    rrddim_set_by_pointer(st_directory_operation_total, rd_directory_operation_total_read, readValue);

    rrddim_set_by_pointer(st_directory_operation_total, rd_directory_operation_total_write, writeValue);

    rrddim_set_by_pointer(st_directory_operation_total, rd_directory_operation_total_search, searchValue);

    rrdset_done(st_directory_operation_total);
}

static void netdata_ad_search_scope_base(PERF_DATA_BLOCK *pDataBlock, PERF_OBJECT_TYPE *pObjectType, int update_every)
{
    static COUNTER_DATA baseSearchesPerSec = {.key = "Base searches/sec"};
    static RRDSET *st_search_scope_base = NULL;
    static RRDDIM *rd_search_scope_base = NULL;

    if (!perflibGetObjectCounter(pDataBlock, pObjectType, &baseSearchesPerSec))
        return;

    if (unlikely(!st_search_scope_base)) {
        st_search_scope_base = rrdset_create_localhost(
            "ad",
            "searches_base",
            NULL,
            "search",
            "ad.searches_base",
            "Directory base searches",
            "searches/s",
            PLUGIN_WINDOWS_NAME,
            "PerflibAD",
            PRIO_AD_SEARCHES_SCOPE_BASE,
            update_every,
            RRDSET_TYPE_LINE);

        rd_search_scope_base = rrddim_add(st_search_scope_base, "base", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
    }

    rrddim_set_by_pointer(
        st_search_scope_base, rd_search_scope_base, (collected_number)baseSearchesPerSec.current.Data);
    rrdset_done(st_search_scope_base);
}

static void
netdata_ad_search_scope_subtree(PERF_DATA_BLOCK *pDataBlock, PERF_OBJECT_TYPE *pObjectType, int update_every)
{
    static COUNTER_DATA subtreeSearchesPerSec = {.key = "Subtree searches/sec"};
    static RRDSET *st_search_scope_subtree = NULL;
    static RRDDIM *rd_search_scope_subtree = NULL;

    if (!perflibGetObjectCounter(pDataBlock, pObjectType, &subtreeSearchesPerSec))
        return;

    if (unlikely(!st_search_scope_subtree)) {
        st_search_scope_subtree = rrdset_create_localhost(
            "ad",
            "searches_subtree",
            NULL,
            "search",
            "ad.searches_subtree",
            "Directory subtree searches",
            "searches/s",
            PLUGIN_WINDOWS_NAME,
            "PerflibAD",
            PRIO_AD_SEARCHES_SCOPE_SUBTREE,
            update_every,
            RRDSET_TYPE_LINE);

        rd_search_scope_subtree = rrddim_add(st_search_scope_subtree, "subtree", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
    }

    rrddim_set_by_pointer(
        st_search_scope_subtree, rd_search_scope_subtree, (collected_number)subtreeSearchesPerSec.current.Data);
    rrdset_done(st_search_scope_subtree);
}

static void
netdata_ad_search_scope_one_level(PERF_DATA_BLOCK *pDataBlock, PERF_OBJECT_TYPE *pObjectType, int update_every)
{
    static COUNTER_DATA oneLevelSearchesPerSec = {.key = "Onelevel searches/sec"};
    static RRDSET *st_search_scope_one_level = NULL;
    static RRDDIM *rd_search_scope_one_level = NULL;

    if (!perflibGetObjectCounter(pDataBlock, pObjectType, &oneLevelSearchesPerSec))
        return;

    if (unlikely(!st_search_scope_one_level)) {
        st_search_scope_one_level = rrdset_create_localhost(
            "ad",
            "searches_one_level",
            NULL,
            "search",
            "ad.searches_one_level",
            "Directory one-level searches",
            "searches/s",
            PLUGIN_WINDOWS_NAME,
            "PerflibAD",
            PRIO_AD_SEARCHES_SCOPE_ONE_LEVEL,
            update_every,
            RRDSET_TYPE_LINE);

        rd_search_scope_one_level =
            rrddim_add(st_search_scope_one_level, "one_level", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
    }

    rrddim_set_by_pointer(
        st_search_scope_one_level, rd_search_scope_one_level, (collected_number)oneLevelSearchesPerSec.current.Data);
    rrdset_done(st_search_scope_one_level);
}

static void netdata_ad_search_scope(PERF_DATA_BLOCK *pDataBlock, PERF_OBJECT_TYPE *pObjectType, int update_every)
{
    netdata_ad_search_scope_base(pDataBlock, pObjectType, update_every);
    netdata_ad_search_scope_subtree(pDataBlock, pObjectType, update_every);
    netdata_ad_search_scope_one_level(pDataBlock, pObjectType, update_every);
}

static void netdata_ad_cache_lookups(PERF_DATA_BLOCK *pDataBlock, PERF_OBJECT_TYPE *pObjectType, int update_every)
{
    static COUNTER_DATA nameCacheLookupsTotal = {.key = "DS Name Cache hit rate"};

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
        st_name_cache_lookups_total, rd_name_cache_lookups_total, (collected_number)nameCacheLookupsTotal.current.Data);
    rrdset_done(st_name_cache_lookups_total);
}

static void netdata_ad_cache_hits(PERF_DATA_BLOCK *pDataBlock, PERF_OBJECT_TYPE *pObjectType, int update_every)
{
    static COUNTER_DATA nameCacheHitsTotal = {.key = "DS Name Cache hit rate,secondvalue"};

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

        rd_name_cache_hits_total = rrddim_add(st_name_cache_hits_total, "hits", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
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

        rd_ldap_searches_total = rrddim_add(st_ldap_searches_total, "searches", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
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
        .key = "DRA Inbound Bytes Not Compressed (Within Site)/sec"};
    static COUNTER_DATA replicationDataOutboundIntraSiteBytesTotal = {
        .key = "DRA Outbound Bytes Not Compressed (Within Site)/sec"};

    static RRDSET *st_dra_replication_intersite_compressed_traffic = NULL;
    static RRDDIM *rd_replication_data_intersite_bytes_total_inbound = NULL;
    static RRDDIM *rd_replication_data_intersite_bytes_total_outbound = NULL;

    static RRDSET *st_dra_replication_intrasite_uncompressed_traffic = NULL;
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
            "DRA replication compressed traffic between sites",
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

    if (unlikely(!st_dra_replication_intrasite_uncompressed_traffic)) {
        st_dra_replication_intrasite_uncompressed_traffic = rrdset_create_localhost(
            "ad",
            "dra_replication_intrasite_uncompressed_traffic",
            NULL,
            "replication",
            "ad.dra_replication_intrasite_uncompressed_traffic",
            "DRA replication uncompressed traffic within site",
            "bytes/s",
            PLUGIN_WINDOWS_NAME,
            "PerflibAD",
            PRIO_AD_REPLICATION_INTRASITE_UNCOMPRESSED,
            update_every,
            RRDSET_TYPE_AREA);

        rd_replication_data_intrasite_bytes_total_inbound = rrddim_add(
            st_dra_replication_intrasite_uncompressed_traffic, "inbound", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);

        rd_replication_data_intrasite_bytes_total_outbound = rrddim_add(
            st_dra_replication_intrasite_uncompressed_traffic, "outbound", NULL, -1, 1, RRD_ALGORITHM_INCREMENTAL);
    }

    rrddim_set_by_pointer(
        st_dra_replication_intrasite_uncompressed_traffic,
        rd_replication_data_intrasite_bytes_total_inbound,
        (collected_number)replicationDataInboundIntraSiteBytesTotal.current.Data);
    rrddim_set_by_pointer(
        st_dra_replication_intrasite_uncompressed_traffic,
        rd_replication_data_intrasite_bytes_total_outbound,
        (collected_number)replicationDataOutboundIntraSiteBytesTotal.current.Data);
    rrdset_done(st_dra_replication_intrasite_uncompressed_traffic);
}

static void netdata_ad_replication_highest_usn_committed(
    PERF_DATA_BLOCK *pDataBlock,
    PERF_OBJECT_TYPE *pObjectType,
    int update_every)
{
    static COUNTER_DATA replicationHighestUSNCommittedHighPart = {.key = "DRA Highest USN Committed (High part)"};
    static COUNTER_DATA replicationHighestUSNCommittedLowPart = {.key = "DRA Highest USN Committed (Low part)"};
    static RRDSET *st_replication_highest_usn_committed = NULL;
    static RRDDIM *rd_replication_highest_usn_committed = NULL;

    if (!perflibGetObjectCounter(pDataBlock, pObjectType, &replicationHighestUSNCommittedHighPart) ||
        !perflibGetObjectCounter(pDataBlock, pObjectType, &replicationHighestUSNCommittedLowPart))
        return;

    if (unlikely(!st_replication_highest_usn_committed)) {
        st_replication_highest_usn_committed = rrdset_create_localhost(
            "ad",
            "replication_highest_usn_committed",
            NULL,
            "replication",
            "ad.replication_highest_usn_committed",
            "Highest replication committed update sequence number",
            "usn/s",
            PLUGIN_WINDOWS_NAME,
            "PerflibAD",
            PRIO_AD_REPLICATION_HIGHEST_USN_COMMITTED,
            update_every,
            RRDSET_TYPE_LINE);

        rd_replication_highest_usn_committed =
            rrddim_add(st_replication_highest_usn_committed, "committed", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
    }

    // Windows exposes the 64-bit USN as separate high/low perf counters.
    uint64_t committed = ((uint64_t)replicationHighestUSNCommittedHighPart.current.Data << 32) |
                         (uint64_t)replicationHighestUSNCommittedLowPart.current.Data;

    rrddim_set_by_pointer(
        st_replication_highest_usn_committed, rd_replication_highest_usn_committed, (collected_number)committed);
    rrdset_done(st_replication_highest_usn_committed);
}

static void
netdata_ad_replication_highest_usn_issued(PERF_DATA_BLOCK *pDataBlock, PERF_OBJECT_TYPE *pObjectType, int update_every)
{
    static COUNTER_DATA replicationHighestUSNIssuedHighPart = {.key = "DRA Highest USN Issued (High part)"};
    static COUNTER_DATA replicationHighestUSNIssuedLowPart = {.key = "DRA Highest USN Issued (Low part)"};
    static RRDSET *st_replication_highest_usn_issued = NULL;
    static RRDDIM *rd_replication_highest_usn_issued = NULL;

    if (!perflibGetObjectCounter(pDataBlock, pObjectType, &replicationHighestUSNIssuedHighPart) ||
        !perflibGetObjectCounter(pDataBlock, pObjectType, &replicationHighestUSNIssuedLowPart))
        return;

    if (unlikely(!st_replication_highest_usn_issued)) {
        st_replication_highest_usn_issued = rrdset_create_localhost(
            "ad",
            "replication_highest_usn_issued",
            NULL,
            "replication",
            "ad.replication_highest_usn_issued",
            "Highest replication issued update sequence number",
            "usn/s",
            PLUGIN_WINDOWS_NAME,
            "PerflibAD",
            PRIO_AD_REPLICATION_HIGHEST_USN_ISSUED,
            update_every,
            RRDSET_TYPE_LINE);

        rd_replication_highest_usn_issued =
            rrddim_add(st_replication_highest_usn_issued, "issued", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
    }

    // Windows exposes the 64-bit USN as separate high/low perf counters.
    uint64_t issued = ((uint64_t)replicationHighestUSNIssuedHighPart.current.Data << 32) |
                      (uint64_t)replicationHighestUSNIssuedLowPart.current.Data;

    rrddim_set_by_pointer(
        st_replication_highest_usn_issued, rd_replication_highest_usn_issued, (collected_number)issued);
    rrdset_done(st_replication_highest_usn_issued);
}

static void
netdata_ad_replication_highest_usn(PERF_DATA_BLOCK *pDataBlock, PERF_OBJECT_TYPE *pObjectType, int update_every)
{
    netdata_ad_replication_highest_usn_committed(pDataBlock, pObjectType, update_every);
    netdata_ad_replication_highest_usn_issued(pDataBlock, pObjectType, update_every);
}

static void netdata_ad_replication_inbound_sync_objects_remaining(
    PERF_DATA_BLOCK *pDataBlock,
    PERF_OBJECT_TYPE *pObjectType,
    int update_every)
{
    static COUNTER_DATA replicationInboundSyncObjectsRemaining = {.key = "DRA Inbound Full Sync Objects Remaining"};
    static RRDSET *st_replication_inbound_sync_objects_remaining = NULL;
    static RRDDIM *rd_replication_inbound_sync_objects_remaining = NULL;

    if (!perflibGetObjectCounter(pDataBlock, pObjectType, &replicationInboundSyncObjectsRemaining))
        return;

    if (unlikely(!st_replication_inbound_sync_objects_remaining)) {
        st_replication_inbound_sync_objects_remaining = rrdset_create_localhost(
            "ad",
            "replication_inbound_sync_objects_remaining",
            NULL,
            "replication",
            "ad.replication_inbound_sync_objects_remaining",
            "Replication inbound sync objects remaining",
            "objects",
            PLUGIN_WINDOWS_NAME,
            "PerflibAD",
            PRIO_AD_REPLICATION_INBOUND_SYNC_OBJECTS_REMAINING,
            update_every,
            RRDSET_TYPE_LINE);

        rd_replication_inbound_sync_objects_remaining =
            rrddim_add(st_replication_inbound_sync_objects_remaining, "remaining", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
    }

    rrddim_set_by_pointer(
        st_replication_inbound_sync_objects_remaining,
        rd_replication_inbound_sync_objects_remaining,
        (collected_number)replicationInboundSyncObjectsRemaining.current.Data);
    rrdset_done(st_replication_inbound_sync_objects_remaining);
}

static void netdata_ad_replication_inbound_link_value_updates_remaining(
    PERF_DATA_BLOCK *pDataBlock,
    PERF_OBJECT_TYPE *pObjectType,
    int update_every)
{
    static COUNTER_DATA replicationInboundLinkValueUpdatesRemaining = {
        .key = "DRA Inbound Link Value Updates Remaining in Packet"};
    static RRDSET *st_replication_inbound_link_value_updates_remaining = NULL;
    static RRDDIM *rd_replication_inbound_link_value_updates_remaining = NULL;

    if (!perflibGetObjectCounter(pDataBlock, pObjectType, &replicationInboundLinkValueUpdatesRemaining))
        return;

    if (unlikely(!st_replication_inbound_link_value_updates_remaining)) {
        st_replication_inbound_link_value_updates_remaining = rrdset_create_localhost(
            "ad",
            "replication_inbound_link_value_updates_remaining",
            NULL,
            "replication",
            "ad.replication_inbound_link_value_updates_remaining",
            "Replication inbound link value updates remaining",
            "updates",
            PLUGIN_WINDOWS_NAME,
            "PerflibAD",
            PRIO_AD_REPLICATION_INBOUND_LINK_VALUE_UPDATES_REMAINING,
            update_every,
            RRDSET_TYPE_LINE);

        rd_replication_inbound_link_value_updates_remaining = rrddim_add(
            st_replication_inbound_link_value_updates_remaining, "remaining", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
    }

    rrddim_set_by_pointer(
        st_replication_inbound_link_value_updates_remaining,
        rd_replication_inbound_link_value_updates_remaining,
        (collected_number)replicationInboundLinkValueUpdatesRemaining.current.Data);
    rrdset_done(st_replication_inbound_link_value_updates_remaining);
}

static void netdata_ad_replication_inbound_objects_updated(
    PERF_DATA_BLOCK *pDataBlock,
    PERF_OBJECT_TYPE *pObjectType,
    int update_every)
{
    static COUNTER_DATA replicationInboundObjectsUpdatedTotal = {.key = "DRA Inbound Objects Applied/sec"};
    static RRDSET *st_replication_inbound_objects_updated = NULL;
    static RRDDIM *rd_replication_inbound_objects_updated = NULL;

    if (!perflibGetObjectCounter(pDataBlock, pObjectType, &replicationInboundObjectsUpdatedTotal))
        return;

    if (unlikely(!st_replication_inbound_objects_updated)) {
        st_replication_inbound_objects_updated = rrdset_create_localhost(
            "ad",
            "replication_inbound_objects_updated",
            NULL,
            "replication",
            "ad.replication_inbound_objects_updated",
            "Replication inbound objects updated",
            "objects/s",
            PLUGIN_WINDOWS_NAME,
            "PerflibAD",
            PRIO_AD_REPLICATION_INBOUND_OBJECTS_UPDATED,
            update_every,
            RRDSET_TYPE_LINE);

        rd_replication_inbound_objects_updated =
            rrddim_add(st_replication_inbound_objects_updated, "updated", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
    }

    rrddim_set_by_pointer(
        st_replication_inbound_objects_updated,
        rd_replication_inbound_objects_updated,
        (collected_number)replicationInboundObjectsUpdatedTotal.current.Data);
    rrdset_done(st_replication_inbound_objects_updated);
}

static void netdata_ad_replication_inbound_objects_filtered(
    PERF_DATA_BLOCK *pDataBlock,
    PERF_OBJECT_TYPE *pObjectType,
    int update_every)
{
    static COUNTER_DATA replicationInboundObjectsFilteredTotal = {.key = "DRA Inbound Objects Filtered/sec"};
    static RRDSET *st_replication_inbound_objects_filtered = NULL;
    static RRDDIM *rd_replication_inbound_objects_filtered = NULL;

    if (!perflibGetObjectCounter(pDataBlock, pObjectType, &replicationInboundObjectsFilteredTotal))
        return;

    if (unlikely(!st_replication_inbound_objects_filtered)) {
        st_replication_inbound_objects_filtered = rrdset_create_localhost(
            "ad",
            "replication_inbound_objects_filtered",
            NULL,
            "replication",
            "ad.replication_inbound_objects_filtered",
            "Replication inbound objects filtered",
            "objects/s",
            PLUGIN_WINDOWS_NAME,
            "PerflibAD",
            PRIO_AD_REPLICATION_INBOUND_OBJECTS_FILTERED,
            update_every,
            RRDSET_TYPE_LINE);

        rd_replication_inbound_objects_filtered =
            rrddim_add(st_replication_inbound_objects_filtered, "filtered", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
    }

    rrddim_set_by_pointer(
        st_replication_inbound_objects_filtered,
        rd_replication_inbound_objects_filtered,
        (collected_number)replicationInboundObjectsFilteredTotal.current.Data);
    rrdset_done(st_replication_inbound_objects_filtered);
}

static void netdata_ad_replication_inbound(PERF_DATA_BLOCK *pDataBlock, PERF_OBJECT_TYPE *pObjectType, int update_every)
{
    netdata_ad_replication_inbound_sync_objects_remaining(pDataBlock, pObjectType, update_every);
    netdata_ad_replication_inbound_link_value_updates_remaining(pDataBlock, pObjectType, update_every);
    netdata_ad_replication_inbound_objects_updated(pDataBlock, pObjectType, update_every);
    netdata_ad_replication_inbound_objects_filtered(pDataBlock, pObjectType, update_every);
}

static void netdata_ad_sync(PERF_DATA_BLOCK *pDataBlock, PERF_OBJECT_TYPE *pObjectType, int update_every)
{
    static COUNTER_DATA replicationSyncPending = {.key = "DRA Pending Replication Synchronizations"};
    static COUNTER_DATA replicationSyncRequestsTotal = {.key = "DRA Sync Requests Made"};

    static RRDSET *st_dra_replication_pending_syncs = NULL;
    static RRDDIM *rd_dra_replication_pending_syncs = NULL;
    static RRDSET *st_dra_replication_sync_requests = NULL;
    static RRDDIM *rd_dra_replication_sync_requests = NULL;

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
        return;
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
}

static void
netdata_ad_replication_pending_operations(PERF_DATA_BLOCK *pDataBlock, PERF_OBJECT_TYPE *pObjectType, int update_every)
{
    static COUNTER_DATA replicationPendingOperations = {.key = "DRA Pending Replication Operations"};

    static RRDSET *st_replication_pending_operations = NULL;
    static RRDDIM *rd_replication_pending_operations = NULL;

    if (!perflibGetObjectCounter(pDataBlock, pObjectType, &replicationPendingOperations))
        return;

    if (unlikely(!st_replication_pending_operations)) {
        st_replication_pending_operations = rrdset_create_localhost(
            "ad",
            "replication_pending_operations",
            NULL,
            "replication",
            "ad.replication_pending_operations",
            "Replication pending operations",
            "operations",
            PLUGIN_WINDOWS_NAME,
            "PerflibAD",
            PRIO_AD_REPLICATION_PENDING_OPERATIONS,
            update_every,
            RRDSET_TYPE_LINE);

        rd_replication_pending_operations =
            rrddim_add(st_replication_pending_operations, "pending", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
    }

    rrddim_set_by_pointer(
        st_replication_pending_operations,
        rd_replication_pending_operations,
        (collected_number)replicationPendingOperations.current.Data);
    rrdset_done(st_replication_pending_operations);
}

static void netdata_ad_sync_result_success(PERF_DATA_BLOCK *pDataBlock, PERF_OBJECT_TYPE *pObjectType, int update_every)
{
    static COUNTER_DATA replicationSyncRequestsSuccessTotal = {.key = "DRA Sync Requests Successful"};
    static RRDSET *st_replication_sync_requests_success = NULL;
    static RRDDIM *rd_replication_sync_requests_success = NULL;

    if (!perflibGetObjectCounter(pDataBlock, pObjectType, &replicationSyncRequestsSuccessTotal))
        return;

    if (unlikely(!st_replication_sync_requests_success)) {
        st_replication_sync_requests_success = rrdset_create_localhost(
            "ad",
            "replication_sync_requests_success",
            NULL,
            "replication",
            "ad.replication_sync_requests_success",
            "Replication sync requests successful",
            "requests/s",
            PLUGIN_WINDOWS_NAME,
            "PerflibAD",
            PRIO_AD_SYNC_SUCCESS_TOTAL,
            update_every,
            RRDSET_TYPE_LINE);

        rd_replication_sync_requests_success =
            rrddim_add(st_replication_sync_requests_success, "success", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
    }

    rrddim_set_by_pointer(
        st_replication_sync_requests_success,
        rd_replication_sync_requests_success,
        (collected_number)replicationSyncRequestsSuccessTotal.current.Data);
    rrdset_done(st_replication_sync_requests_success);
}

static void netdata_ad_sync_result_schema_mismatch_failure(
    PERF_DATA_BLOCK *pDataBlock,
    PERF_OBJECT_TYPE *pObjectType,
    int update_every)
{
    static COUNTER_DATA replicationSyncRequestsSchemaMismatchFailureTotal = {
        .key = "DRA Sync Failures on Schema Mismatch"};
    static RRDSET *st_replication_sync_requests_schema_mismatch_failure = NULL;
    static RRDDIM *rd_replication_sync_requests_schema_mismatch_failure = NULL;

    if (!perflibGetObjectCounter(pDataBlock, pObjectType, &replicationSyncRequestsSchemaMismatchFailureTotal))
        return;

    if (unlikely(!st_replication_sync_requests_schema_mismatch_failure)) {
        st_replication_sync_requests_schema_mismatch_failure = rrdset_create_localhost(
            "ad",
            "replication_sync_requests_schema_mismatch_failure",
            NULL,
            "replication",
            "ad.replication_sync_requests_schema_mismatch_failure",
            "Replication sync requests failed due to schema mismatch",
            "requests/s",
            PLUGIN_WINDOWS_NAME,
            "PerflibAD",
            PRIO_AD_SYNC_SCHEMA_MISMATCH_FAILURE_TOTAL,
            update_every,
            RRDSET_TYPE_LINE);

        rd_replication_sync_requests_schema_mismatch_failure = rrddim_add(
            st_replication_sync_requests_schema_mismatch_failure, "failure", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
    }

    rrddim_set_by_pointer(
        st_replication_sync_requests_schema_mismatch_failure,
        rd_replication_sync_requests_schema_mismatch_failure,
        (collected_number)replicationSyncRequestsSchemaMismatchFailureTotal.current.Data);
    rrdset_done(st_replication_sync_requests_schema_mismatch_failure);
}

static void netdata_ad_sync_result(PERF_DATA_BLOCK *pDataBlock, PERF_OBJECT_TYPE *pObjectType, int update_every)
{
    netdata_ad_sync_result_success(pDataBlock, pObjectType, update_every);
    netdata_ad_sync_result_schema_mismatch_failure(pDataBlock, pObjectType, update_every);
}

static void
netdata_ad_name_translations_client(PERF_DATA_BLOCK *pDataBlock, PERF_OBJECT_TYPE *pObjectType, int update_every)
{
    static COUNTER_DATA clientNameTranslationsPerSec = {.key = "DS Client Name Translations/sec"};
    static RRDSET *st_name_translations_client = NULL;
    static RRDDIM *rd_name_translations_client = NULL;

    if (!perflibGetObjectCounter(pDataBlock, pObjectType, &clientNameTranslationsPerSec))
        return;

    if (unlikely(!st_name_translations_client)) {
        st_name_translations_client = rrdset_create_localhost(
            "ad",
            "name_translations_client",
            NULL,
            "directory",
            "ad.name_translations_client",
            "Client name translations",
            "translations/s",
            PLUGIN_WINDOWS_NAME,
            "PerflibAD",
            PRIO_AD_NAME_TRANSLATIONS_CLIENT,
            update_every,
            RRDSET_TYPE_LINE);

        rd_name_translations_client =
            rrddim_add(st_name_translations_client, "client", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
    }

    rrddim_set_by_pointer(
        st_name_translations_client,
        rd_name_translations_client,
        (collected_number)clientNameTranslationsPerSec.current.Data);
    rrdset_done(st_name_translations_client);
}

static void
netdata_ad_name_translations_server(PERF_DATA_BLOCK *pDataBlock, PERF_OBJECT_TYPE *pObjectType, int update_every)
{
    static COUNTER_DATA serverNameTranslationsPerSec = {.key = "DS Server Name Translations/sec"};
    static RRDSET *st_name_translations_server = NULL;
    static RRDDIM *rd_name_translations_server = NULL;

    if (!perflibGetObjectCounter(pDataBlock, pObjectType, &serverNameTranslationsPerSec))
        return;

    if (unlikely(!st_name_translations_server)) {
        st_name_translations_server = rrdset_create_localhost(
            "ad",
            "name_translations_server",
            NULL,
            "directory",
            "ad.name_translations_server",
            "Server name translations",
            "translations/s",
            PLUGIN_WINDOWS_NAME,
            "PerflibAD",
            PRIO_AD_NAME_TRANSLATIONS_SERVER,
            update_every,
            RRDSET_TYPE_LINE);

        rd_name_translations_server =
            rrddim_add(st_name_translations_server, "server", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
    }

    rrddim_set_by_pointer(
        st_name_translations_server,
        rd_name_translations_server,
        (collected_number)serverNameTranslationsPerSec.current.Data);
    rrdset_done(st_name_translations_server);
}

static void netdata_ad_name_translations(PERF_DATA_BLOCK *pDataBlock, PERF_OBJECT_TYPE *pObjectType, int update_every)
{
    netdata_ad_name_translations_client(pDataBlock, pObjectType, update_every);
    netdata_ad_name_translations_server(pDataBlock, pObjectType, update_every);
}

static void
netdata_ad_change_monitors_registered(PERF_DATA_BLOCK *pDataBlock, PERF_OBJECT_TYPE *pObjectType, int update_every)
{
    static COUNTER_DATA changeMonitorsRegistered = {.key = "DS Monitor List Size"};
    static RRDSET *st_change_monitors_registered = NULL;
    static RRDDIM *rd_change_monitors_registered = NULL;

    if (!perflibGetObjectCounter(pDataBlock, pObjectType, &changeMonitorsRegistered))
        return;

    if (unlikely(!st_change_monitors_registered)) {
        st_change_monitors_registered = rrdset_create_localhost(
            "ad",
            "change_monitors_registered",
            NULL,
            "directory",
            "ad.change_monitors_registered",
            "Registered change monitors",
            "monitors",
            PLUGIN_WINDOWS_NAME,
            "PerflibAD",
            PRIO_AD_CHANGE_MONITORS_REGISTERED,
            update_every,
            RRDSET_TYPE_LINE);

        rd_change_monitors_registered =
            rrddim_add(st_change_monitors_registered, "registered", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
    }

    rrddim_set_by_pointer(
        st_change_monitors_registered,
        rd_change_monitors_registered,
        (collected_number)changeMonitorsRegistered.current.Data);
    rrdset_done(st_change_monitors_registered);
}

static void
netdata_ad_change_monitor_updates_pending(PERF_DATA_BLOCK *pDataBlock, PERF_OBJECT_TYPE *pObjectType, int update_every)
{
    static COUNTER_DATA changeMonitorUpdatesPending = {.key = "DS Notify Queue Size"};
    static RRDSET *st_change_monitor_updates_pending = NULL;
    static RRDDIM *rd_change_monitor_updates_pending = NULL;

    if (!perflibGetObjectCounter(pDataBlock, pObjectType, &changeMonitorUpdatesPending))
        return;

    if (unlikely(!st_change_monitor_updates_pending)) {
        st_change_monitor_updates_pending = rrdset_create_localhost(
            "ad",
            "change_monitor_updates_pending",
            NULL,
            "directory",
            "ad.change_monitor_updates_pending",
            "Pending change monitor updates",
            "updates",
            PLUGIN_WINDOWS_NAME,
            "PerflibAD",
            PRIO_AD_CHANGE_MONITOR_UPDATES_PENDING,
            update_every,
            RRDSET_TYPE_LINE);

        rd_change_monitor_updates_pending =
            rrddim_add(st_change_monitor_updates_pending, "pending", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
    }

    rrddim_set_by_pointer(
        st_change_monitor_updates_pending,
        rd_change_monitor_updates_pending,
        (collected_number)changeMonitorUpdatesPending.current.Data);
    rrdset_done(st_change_monitor_updates_pending);
}

static void netdata_ad_change_monitors(PERF_DATA_BLOCK *pDataBlock, PERF_OBJECT_TYPE *pObjectType, int update_every)
{
    netdata_ad_change_monitors_registered(pDataBlock, pObjectType, update_every);
    netdata_ad_change_monitor_updates_pending(pDataBlock, pObjectType, update_every);
}

static void
netdata_ad_directory_search_suboperations(PERF_DATA_BLOCK *pDataBlock, PERF_OBJECT_TYPE *pObjectType, int update_every)
{
    static COUNTER_DATA directorySearchSubOperationsPerSec = {.key = "DS Search sub-operations/sec"};

    static RRDSET *st_directory_search_suboperations_total = NULL;
    static RRDDIM *rd_directory_search_suboperations_total = NULL;

    if (!perflibGetObjectCounter(pDataBlock, pObjectType, &directorySearchSubOperationsPerSec))
        return;

    if (unlikely(!st_directory_search_suboperations_total)) {
        st_directory_search_suboperations_total = rrdset_create_localhost(
            "ad",
            "directory_search_suboperations",
            NULL,
            "search",
            "ad.directory_search_suboperations",
            "Directory search suboperations",
            "operations/s",
            PLUGIN_WINDOWS_NAME,
            "PerflibAD",
            PRIO_AD_DIRECTORY_SEARCH_SUBOPERATIONS_TOTAL,
            update_every,
            RRDSET_TYPE_LINE);

        rd_directory_search_suboperations_total =
            rrddim_add(st_directory_search_suboperations_total, "suboperations", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
    }

    rrddim_set_by_pointer(
        st_directory_search_suboperations_total,
        rd_directory_search_suboperations_total,
        (collected_number)directorySearchSubOperationsPerSec.current.Data);
    rrdset_done(st_directory_search_suboperations_total);
}

static void netdata_ad_security_descriptor_propagation_events(
    PERF_DATA_BLOCK *pDataBlock,
    PERF_OBJECT_TYPE *pObjectType,
    int update_every)
{
    static COUNTER_DATA securityDescriptorSubOperationsPerSec = {.key = "DS Security Descriptor sub-operations/sec"};
    static RRDSET *st_security_descriptor_propagation_events = NULL;
    static RRDDIM *rd_security_descriptor_propagation_events = NULL;

    if (!perflibGetObjectCounter(pDataBlock, pObjectType, &securityDescriptorSubOperationsPerSec))
        return;

    if (unlikely(!st_security_descriptor_propagation_events)) {
        st_security_descriptor_propagation_events = rrdset_create_localhost(
            "ad",
            "security_descriptor_propagation_events",
            NULL,
            "directory",
            "ad.security_descriptor_propagation_events",
            "Security descriptor propagation events",
            "events/s",
            PLUGIN_WINDOWS_NAME,
            "PerflibAD",
            PRIO_AD_SECURITY_DESCRIPTOR_PROPAGATION_EVENTS_TOTAL,
            update_every,
            RRDSET_TYPE_LINE);

        rd_security_descriptor_propagation_events =
            rrddim_add(st_security_descriptor_propagation_events, "events", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
    }

    rrddim_set_by_pointer(
        st_security_descriptor_propagation_events,
        rd_security_descriptor_propagation_events,
        (collected_number)securityDescriptorSubOperationsPerSec.current.Data);
    rrdset_done(st_security_descriptor_propagation_events);
}

static void netdata_ad_security_descriptor_propagation_events_queued(
    PERF_DATA_BLOCK *pDataBlock,
    PERF_OBJECT_TYPE *pObjectType,
    int update_every)
{
    static COUNTER_DATA securityDescriptorPropagationsEvents = {.key = "DS Security Descriptor Propagations Events"};
    static RRDSET *st_security_descriptor_propagation_events_queued = NULL;
    static RRDDIM *rd_security_descriptor_propagation_events_queued = NULL;

    if (!perflibGetObjectCounter(pDataBlock, pObjectType, &securityDescriptorPropagationsEvents))
        return;

    if (unlikely(!st_security_descriptor_propagation_events_queued)) {
        st_security_descriptor_propagation_events_queued = rrdset_create_localhost(
            "ad",
            "security_descriptor_propagation_events_queued",
            NULL,
            "directory",
            "ad.security_descriptor_propagation_events_queued",
            "Security descriptor propagation events queued",
            "events",
            PLUGIN_WINDOWS_NAME,
            "PerflibAD",
            PRIO_AD_SECURITY_DESCRIPTOR_PROPAGATION_EVENTS_QUEUED,
            update_every,
            RRDSET_TYPE_LINE);

        rd_security_descriptor_propagation_events_queued =
            rrddim_add(st_security_descriptor_propagation_events_queued, "queued", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
    }

    rrddim_set_by_pointer(
        st_security_descriptor_propagation_events_queued,
        rd_security_descriptor_propagation_events_queued,
        (collected_number)securityDescriptorPropagationsEvents.current.Data);
    rrdset_done(st_security_descriptor_propagation_events_queued);
}

static void netdata_ad_security_descriptor_propagation_access_wait(
    PERF_DATA_BLOCK *pDataBlock,
    PERF_OBJECT_TYPE *pObjectType,
    int update_every)
{
    static COUNTER_DATA securityDescriptorPropagatorAverageExclusionTime = {
        .key = "DS Security Descriptor Propagator Average Exclusion Time"};
    static RRDSET *st_security_descriptor_propagation_access_wait = NULL;
    static RRDDIM *rd_security_descriptor_propagation_access_wait = NULL;

    if (!perflibGetObjectCounter(pDataBlock, pObjectType, &securityDescriptorPropagatorAverageExclusionTime))
        return;

    if (unlikely(!st_security_descriptor_propagation_access_wait)) {
        st_security_descriptor_propagation_access_wait = rrdset_create_localhost(
            "ad",
            "security_descriptor_propagation_access_wait",
            NULL,
            "directory",
            "ad.security_descriptor_propagation_access_wait",
            "Security descriptor propagation access wait",
            "seconds",
            PLUGIN_WINDOWS_NAME,
            "PerflibAD",
            PRIO_AD_SECURITY_DESCRIPTOR_PROPAGATION_ACCESS_WAIT_TOTAL_SECONDS,
            update_every,
            RRDSET_TYPE_LINE);

        rd_security_descriptor_propagation_access_wait =
            rrddim_add(st_security_descriptor_propagation_access_wait, "wait", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
    }

    rrddim_set_by_pointer(
        st_security_descriptor_propagation_access_wait,
        rd_security_descriptor_propagation_access_wait,
        (collected_number)securityDescriptorPropagatorAverageExclusionTime.current.Data);
    rrdset_done(st_security_descriptor_propagation_access_wait);
}

static void netdata_ad_security_descriptor_propagation_items_queued(
    PERF_DATA_BLOCK *pDataBlock,
    PERF_OBJECT_TYPE *pObjectType,
    int update_every)
{
    static COUNTER_DATA securityDescriptorPropagatorRuntimeQueue = {
        .key = "DS Security Descriptor Propagator Runtime Queue"};
    static RRDSET *st_security_descriptor_propagation_items_queued = NULL;
    static RRDDIM *rd_security_descriptor_propagation_items_queued = NULL;

    if (!perflibGetObjectCounter(pDataBlock, pObjectType, &securityDescriptorPropagatorRuntimeQueue))
        return;

    if (unlikely(!st_security_descriptor_propagation_items_queued)) {
        st_security_descriptor_propagation_items_queued = rrdset_create_localhost(
            "ad",
            "security_descriptor_propagation_items_queued",
            NULL,
            "directory",
            "ad.security_descriptor_propagation_items_queued",
            "Security descriptor propagation items queued",
            "items",
            PLUGIN_WINDOWS_NAME,
            "PerflibAD",
            PRIO_AD_SECURITY_DESCRIPTOR_PROPAGATION_ITEMS_QUEUED_TOTAL,
            update_every,
            RRDSET_TYPE_LINE);

        rd_security_descriptor_propagation_items_queued =
            rrddim_add(st_security_descriptor_propagation_items_queued, "queued", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
    }

    rrddim_set_by_pointer(
        st_security_descriptor_propagation_items_queued,
        rd_security_descriptor_propagation_items_queued,
        (collected_number)securityDescriptorPropagatorRuntimeQueue.current.Data);
    rrdset_done(st_security_descriptor_propagation_items_queued);
}

static void
netdata_ad_security_descriptor_propagation(PERF_DATA_BLOCK *pDataBlock, PERF_OBJECT_TYPE *pObjectType, int update_every)
{
    netdata_ad_security_descriptor_propagation_events(pDataBlock, pObjectType, update_every);
    netdata_ad_security_descriptor_propagation_events_queued(pDataBlock, pObjectType, update_every);
    netdata_ad_security_descriptor_propagation_access_wait(pDataBlock, pObjectType, update_every);
    netdata_ad_security_descriptor_propagation_items_queued(pDataBlock, pObjectType, update_every);
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
    static COUNTER_DATA ldapLastBindTimeSecondsTotal = {.key = "LDAP Bind Time"};
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
            rrddim_add(st_ldap_last_bind_time_seconds_total, "last_bind", NULL, 1, 1000, RRD_ALGORITHM_ABSOLUTE);
    }

    rrddim_set_by_pointer(
        st_ldap_last_bind_time_seconds_total,
        rd_ldap_last_bind_time_seconds_total,
        (collected_number)ldapLastBindTimeSecondsTotal.current.Data);
    rrdset_done(st_ldap_last_bind_time_seconds_total);
}

static void netdata_ad_binds_digest(PERF_DATA_BLOCK *pDataBlock, PERF_OBJECT_TYPE *pObjectType, int update_every)
{
    static COUNTER_DATA digestBindsPerSec = {.key = "Digest Binds/sec"};
    static RRDSET *st_binds_digest = NULL;
    static RRDDIM *rd_binds_digest = NULL;

    if (!perflibGetObjectCounter(pDataBlock, pObjectType, &digestBindsPerSec))
        return;

    if (unlikely(!st_binds_digest)) {
        st_binds_digest = rrdset_create_localhost(
            "ad",
            "binds_digest",
            NULL,
            "bind",
            "ad.binds_digest",
            "Digest binds",
            "binds/s",
            PLUGIN_WINDOWS_NAME,
            "PerflibAD",
            PRIO_AD_BIND_DIGEST,
            update_every,
            RRDSET_TYPE_LINE);

        rd_binds_digest = rrddim_add(st_binds_digest, "digest", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
    }

    rrddim_set_by_pointer(st_binds_digest, rd_binds_digest, (collected_number)digestBindsPerSec.current.Data);
    rrdset_done(st_binds_digest);
}

static void netdata_ad_binds_ds_client(PERF_DATA_BLOCK *pDataBlock, PERF_OBJECT_TYPE *pObjectType, int update_every)
{
    static COUNTER_DATA dsClientBindsPerSec = {.key = "DS Client Binds/sec"};
    static RRDSET *st_binds_ds_client = NULL;
    static RRDDIM *rd_binds_ds_client = NULL;

    if (!perflibGetObjectCounter(pDataBlock, pObjectType, &dsClientBindsPerSec))
        return;

    if (unlikely(!st_binds_ds_client)) {
        st_binds_ds_client = rrdset_create_localhost(
            "ad",
            "binds_ds_client",
            NULL,
            "bind",
            "ad.binds_ds_client",
            "DS client binds",
            "binds/s",
            PLUGIN_WINDOWS_NAME,
            "PerflibAD",
            PRIO_AD_BIND_DS_CLIENT,
            update_every,
            RRDSET_TYPE_LINE);

        rd_binds_ds_client = rrddim_add(st_binds_ds_client, "ds_client", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
    }

    rrddim_set_by_pointer(st_binds_ds_client, rd_binds_ds_client, (collected_number)dsClientBindsPerSec.current.Data);
    rrdset_done(st_binds_ds_client);
}

static void netdata_ad_binds_ds_server(PERF_DATA_BLOCK *pDataBlock, PERF_OBJECT_TYPE *pObjectType, int update_every)
{
    static COUNTER_DATA dsServerBindsPerSec = {.key = "DS Server Binds/sec"};
    static RRDSET *st_binds_ds_server = NULL;
    static RRDDIM *rd_binds_ds_server = NULL;

    if (!perflibGetObjectCounter(pDataBlock, pObjectType, &dsServerBindsPerSec))
        return;

    if (unlikely(!st_binds_ds_server)) {
        st_binds_ds_server = rrdset_create_localhost(
            "ad",
            "binds_ds_server",
            NULL,
            "bind",
            "ad.binds_ds_server",
            "DS server binds",
            "binds/s",
            PLUGIN_WINDOWS_NAME,
            "PerflibAD",
            PRIO_AD_BIND_DS_SERVER,
            update_every,
            RRDSET_TYPE_LINE);

        rd_binds_ds_server = rrddim_add(st_binds_ds_server, "ds_server", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
    }

    rrddim_set_by_pointer(st_binds_ds_server, rd_binds_ds_server, (collected_number)dsServerBindsPerSec.current.Data);
    rrdset_done(st_binds_ds_server);
}

static void netdata_ad_binds_external(PERF_DATA_BLOCK *pDataBlock, PERF_OBJECT_TYPE *pObjectType, int update_every)
{
    static COUNTER_DATA externalBindsPerSec = {.key = "External Binds/sec"};
    static RRDSET *st_binds_external = NULL;
    static RRDDIM *rd_binds_external = NULL;

    if (!perflibGetObjectCounter(pDataBlock, pObjectType, &externalBindsPerSec))
        return;

    if (unlikely(!st_binds_external)) {
        st_binds_external = rrdset_create_localhost(
            "ad",
            "binds_external",
            NULL,
            "bind",
            "ad.binds_external",
            "External binds",
            "binds/s",
            PLUGIN_WINDOWS_NAME,
            "PerflibAD",
            PRIO_AD_BIND_EXTERNAL,
            update_every,
            RRDSET_TYPE_LINE);

        rd_binds_external = rrddim_add(st_binds_external, "external", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
    }

    rrddim_set_by_pointer(st_binds_external, rd_binds_external, (collected_number)externalBindsPerSec.current.Data);
    rrdset_done(st_binds_external);
}

static void netdata_ad_binds_fast(PERF_DATA_BLOCK *pDataBlock, PERF_OBJECT_TYPE *pObjectType, int update_every)
{
    static COUNTER_DATA fastBindsPerSec = {.key = "Fast Binds/sec"};
    static RRDSET *st_binds_fast = NULL;
    static RRDDIM *rd_binds_fast = NULL;

    if (!perflibGetObjectCounter(pDataBlock, pObjectType, &fastBindsPerSec))
        return;

    if (unlikely(!st_binds_fast)) {
        st_binds_fast = rrdset_create_localhost(
            "ad",
            "binds_fast",
            NULL,
            "bind",
            "ad.binds_fast",
            "Fast binds",
            "binds/s",
            PLUGIN_WINDOWS_NAME,
            "PerflibAD",
            PRIO_AD_BIND_FAST,
            update_every,
            RRDSET_TYPE_LINE);

        rd_binds_fast = rrddim_add(st_binds_fast, "fast", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
    }

    rrddim_set_by_pointer(st_binds_fast, rd_binds_fast, (collected_number)fastBindsPerSec.current.Data);
    rrdset_done(st_binds_fast);
}

static void netdata_ad_binds_negotiate(PERF_DATA_BLOCK *pDataBlock, PERF_OBJECT_TYPE *pObjectType, int update_every)
{
    static COUNTER_DATA negotiatedBindsPerSec = {.key = "Negotiated Binds/sec"};
    static RRDSET *st_binds_negotiate = NULL;
    static RRDDIM *rd_binds_negotiate = NULL;

    if (!perflibGetObjectCounter(pDataBlock, pObjectType, &negotiatedBindsPerSec))
        return;

    if (unlikely(!st_binds_negotiate)) {
        st_binds_negotiate = rrdset_create_localhost(
            "ad",
            "binds_negotiate",
            NULL,
            "bind",
            "ad.binds_negotiate",
            "Negotiated binds",
            "binds/s",
            PLUGIN_WINDOWS_NAME,
            "PerflibAD",
            PRIO_AD_BIND_NEGOTIATE,
            update_every,
            RRDSET_TYPE_LINE);

        rd_binds_negotiate = rrddim_add(st_binds_negotiate, "negotiate", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
    }

    rrddim_set_by_pointer(st_binds_negotiate, rd_binds_negotiate, (collected_number)negotiatedBindsPerSec.current.Data);
    rrdset_done(st_binds_negotiate);
}

static void netdata_ad_binds_ntlm(PERF_DATA_BLOCK *pDataBlock, PERF_OBJECT_TYPE *pObjectType, int update_every)
{
    static COUNTER_DATA ntlmBindsPerSec = {.key = "NTLM Binds/sec"};
    static RRDSET *st_binds_ntlm = NULL;
    static RRDDIM *rd_binds_ntlm = NULL;

    if (!perflibGetObjectCounter(pDataBlock, pObjectType, &ntlmBindsPerSec))
        return;

    if (unlikely(!st_binds_ntlm)) {
        st_binds_ntlm = rrdset_create_localhost(
            "ad",
            "binds_ntlm",
            NULL,
            "bind",
            "ad.binds_ntlm",
            "NTLM binds",
            "binds/s",
            PLUGIN_WINDOWS_NAME,
            "PerflibAD",
            PRIO_AD_BIND_NTLM,
            update_every,
            RRDSET_TYPE_LINE);

        rd_binds_ntlm = rrddim_add(st_binds_ntlm, "ntlm", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
    }

    rrddim_set_by_pointer(st_binds_ntlm, rd_binds_ntlm, (collected_number)ntlmBindsPerSec.current.Data);
    rrdset_done(st_binds_ntlm);
}

static void netdata_ad_binds_simple(PERF_DATA_BLOCK *pDataBlock, PERF_OBJECT_TYPE *pObjectType, int update_every)
{
    static COUNTER_DATA simpleBindsPerSec = {.key = "Simple Binds/sec"};
    static RRDSET *st_binds_simple = NULL;
    static RRDDIM *rd_binds_simple = NULL;

    if (!perflibGetObjectCounter(pDataBlock, pObjectType, &simpleBindsPerSec))
        return;

    if (unlikely(!st_binds_simple)) {
        st_binds_simple = rrdset_create_localhost(
            "ad",
            "binds_simple",
            NULL,
            "bind",
            "ad.binds_simple",
            "Simple binds",
            "binds/s",
            PLUGIN_WINDOWS_NAME,
            "PerflibAD",
            PRIO_AD_BIND_SIMPLE,
            update_every,
            RRDSET_TYPE_LINE);

        rd_binds_simple = rrddim_add(st_binds_simple, "simple", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
    }

    rrddim_set_by_pointer(st_binds_simple, rd_binds_simple, (collected_number)simpleBindsPerSec.current.Data);
    rrdset_done(st_binds_simple);
}

static void netdata_ad_binds_ldap(PERF_DATA_BLOCK *pDataBlock, PERF_OBJECT_TYPE *pObjectType, int update_every)
{
    static COUNTER_DATA ldapSuccessfulBindsPerSec = {.key = "LDAP Successful Binds/sec"};
    static RRDSET *st_binds_ldap = NULL;
    static RRDDIM *rd_binds_ldap = NULL;

    if (!perflibGetObjectCounter(pDataBlock, pObjectType, &ldapSuccessfulBindsPerSec))
        return;

    if (unlikely(!st_binds_ldap)) {
        st_binds_ldap = rrdset_create_localhost(
            "ad",
            "binds_ldap",
            NULL,
            "bind",
            "ad.binds_ldap",
            "LDAP successful binds",
            "binds/s",
            PLUGIN_WINDOWS_NAME,
            "PerflibAD",
            PRIO_AD_BIND_LDAP,
            update_every,
            RRDSET_TYPE_LINE);

        rd_binds_ldap = rrddim_add(st_binds_ldap, "ldap", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
    }

    rrddim_set_by_pointer(st_binds_ldap, rd_binds_ldap, (collected_number)ldapSuccessfulBindsPerSec.current.Data);
    rrdset_done(st_binds_ldap);
}

static void netdata_ad_binds(PERF_DATA_BLOCK *pDataBlock, PERF_OBJECT_TYPE *pObjectType, int update_every)
{
    netdata_ad_binds_digest(pDataBlock, pObjectType, update_every);
    netdata_ad_binds_ds_client(pDataBlock, pObjectType, update_every);
    netdata_ad_binds_ds_server(pDataBlock, pObjectType, update_every);
    netdata_ad_binds_external(pDataBlock, pObjectType, update_every);
    netdata_ad_binds_fast(pDataBlock, pObjectType, update_every);
    netdata_ad_binds_negotiate(pDataBlock, pObjectType, update_every);
    netdata_ad_binds_ntlm(pDataBlock, pObjectType, update_every);
    netdata_ad_binds_simple(pDataBlock, pObjectType, update_every);
    netdata_ad_binds_ldap(pDataBlock, pObjectType, update_every);
}

static void netdata_ad_bind(PERF_DATA_BLOCK *pDataBlock, PERF_OBJECT_TYPE *pObjectType, int update_every)
{
    netdata_ad_bind_time(pDataBlock, pObjectType, update_every);
    netdata_ad_binds(pDataBlock, pObjectType, update_every);
}

static void netdata_ad_atq_queue_requests(PERF_DATA_BLOCK *pDataBlock, PERF_OBJECT_TYPE *pObjectType, int update_every)
{
    static COUNTER_DATA atqOutstandingRequests = {.key = "ATQ Outstanding Queued Requests"};
    static RRDSET *st_atq_outstanding_requests = NULL;
    static RRDDIM *rd_atq_outstanding_requests = NULL;

    if (perflibGetObjectCounter(pDataBlock, pObjectType, &atqOutstandingRequests)) {
        if (unlikely(!st_atq_outstanding_requests)) {
            st_atq_outstanding_requests = rrdset_create_localhost(
                "ad",
                "atq_outstanding_requests",
                NULL,
                "queue",
                "ad.atq_outstanding_requests",
                "Outstanding requests",
                "requests",
                PLUGIN_WINDOWS_NAME,
                "PerflibAD",
                PRIO_AD_OUTSTANDING_REQUEST,
                update_every,
                RRDSET_TYPE_LINE);

            rd_atq_outstanding_requests =
                rrddim_add(st_atq_outstanding_requests, "outstanding", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
        }

        rrddim_set_by_pointer(
            st_atq_outstanding_requests,
            rd_atq_outstanding_requests,
            (collected_number)atqOutstandingRequests.current.Data);
        rrdset_done(st_atq_outstanding_requests);
    }
}

static void netdata_ad_atq_estimated_delay(PERF_DATA_BLOCK *pDataBlock, PERF_OBJECT_TYPE *pObjectType, int update_every)
{
    static COUNTER_DATA atqEstimatedDelay = {.key = "ATQ Estimated Queue Delay"};

    static RRDSET *st_atq_estimated_delay = NULL;
    static RRDDIM *rd_atq_estimated_delay = NULL;

    if (!perflibGetObjectCounter(pDataBlock, pObjectType, &atqEstimatedDelay))
        return;

    if (unlikely(!st_atq_estimated_delay)) {
        st_atq_estimated_delay = rrdset_create_localhost(
            "ad",
            "atq_estimated_delay",
            NULL,
            "queue",
            "ad.atq_estimated_delay",
            "Estimated queue delay",
            "seconds",
            PLUGIN_WINDOWS_NAME,
            "PerflibAD",
            PRIO_AD_ATQ_ESTIMATED_DELAY,
            update_every,
            RRDSET_TYPE_LINE);

        rd_atq_estimated_delay = rrddim_add(st_atq_estimated_delay, "delay", NULL, 1, 1000, RRD_ALGORITHM_ABSOLUTE);
    }

    rrddim_set_by_pointer(
        st_atq_estimated_delay, rd_atq_estimated_delay, (collected_number)atqEstimatedDelay.current.Data);
    rrdset_done(st_atq_estimated_delay);
}

static void
netdata_ad_atq_current_threads_ldap(PERF_DATA_BLOCK *pDataBlock, PERF_OBJECT_TYPE *pObjectType, int update_every)
{
    static COUNTER_DATA atqThreadsLDAP = {.key = "ATQ Threads LDAP"};
    static RRDSET *st_atq_current_threads_ldap = NULL;
    static RRDDIM *rd_atq_current_threads_ldap = NULL;

    if (!perflibGetObjectCounter(pDataBlock, pObjectType, &atqThreadsLDAP))
        return;

    if (unlikely(!st_atq_current_threads_ldap)) {
        st_atq_current_threads_ldap = rrdset_create_localhost(
            "ad",
            "atq_current_threads_ldap",
            NULL,
            "threads",
            "ad.atq_current_threads_ldap",
            "Current ATQ LDAP threads",
            "threads",
            PLUGIN_WINDOWS_NAME,
            "PerflibAD",
            PRIO_AD_ATQ_CURRENT_THREADS_LDAP,
            update_every,
            RRDSET_TYPE_LINE);

        rd_atq_current_threads_ldap =
            rrddim_add(st_atq_current_threads_ldap, "ldap", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
    }

    rrddim_set_by_pointer(
        st_atq_current_threads_ldap, rd_atq_current_threads_ldap, (collected_number)atqThreadsLDAP.current.Data);
    rrdset_done(st_atq_current_threads_ldap);
}

static void
netdata_ad_atq_current_threads_other(PERF_DATA_BLOCK *pDataBlock, PERF_OBJECT_TYPE *pObjectType, int update_every)
{
    static COUNTER_DATA atqThreadsOther = {.key = "ATQ Threads Other"};
    static RRDSET *st_atq_current_threads_other = NULL;
    static RRDDIM *rd_atq_current_threads_other = NULL;

    if (!perflibGetObjectCounter(pDataBlock, pObjectType, &atqThreadsOther))
        return;

    if (unlikely(!st_atq_current_threads_other)) {
        st_atq_current_threads_other = rrdset_create_localhost(
            "ad",
            "atq_current_threads_other",
            NULL,
            "threads",
            "ad.atq_current_threads_other",
            "Current ATQ other threads",
            "threads",
            PLUGIN_WINDOWS_NAME,
            "PerflibAD",
            PRIO_AD_ATQ_CURRENT_THREADS_OTHER,
            update_every,
            RRDSET_TYPE_LINE);

        rd_atq_current_threads_other =
            rrddim_add(st_atq_current_threads_other, "other", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
    }

    rrddim_set_by_pointer(
        st_atq_current_threads_other, rd_atq_current_threads_other, (collected_number)atqThreadsOther.current.Data);
    rrdset_done(st_atq_current_threads_other);
}

static void
netdata_ad_atq_current_threads_total(PERF_DATA_BLOCK *pDataBlock, PERF_OBJECT_TYPE *pObjectType, int update_every)
{
    static COUNTER_DATA atqThreadsTotal = {.key = "ATQ Threads Total"};
    static RRDSET *st_atq_current_threads_total = NULL;
    static RRDDIM *rd_atq_current_threads_total = NULL;

    if (!perflibGetObjectCounter(pDataBlock, pObjectType, &atqThreadsTotal))
        return;

    if (unlikely(!st_atq_current_threads_total)) {
        st_atq_current_threads_total = rrdset_create_localhost(
            "ad",
            "atq_current_threads_total",
            NULL,
            "threads",
            "ad.atq_current_threads_total",
            "Current ATQ total threads",
            "threads",
            PLUGIN_WINDOWS_NAME,
            "PerflibAD",
            PRIO_AD_ATQ_CURRENT_THREADS_TOTAL,
            update_every,
            RRDSET_TYPE_LINE);

        rd_atq_current_threads_total =
            rrddim_add(st_atq_current_threads_total, "total", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
    }

    rrddim_set_by_pointer(
        st_atq_current_threads_total, rd_atq_current_threads_total, (collected_number)atqThreadsTotal.current.Data);
    rrdset_done(st_atq_current_threads_total);
}

static void netdata_ad_atq_current_threads(PERF_DATA_BLOCK *pDataBlock, PERF_OBJECT_TYPE *pObjectType, int update_every)
{
    netdata_ad_atq_current_threads_ldap(pDataBlock, pObjectType, update_every);
    netdata_ad_atq_current_threads_other(pDataBlock, pObjectType, update_every);
    netdata_ad_atq_current_threads_total(pDataBlock, pObjectType, update_every);
}

static void netdata_ad_atq_latency(PERF_DATA_BLOCK *pDataBlock, PERF_OBJECT_TYPE *pObjectType, int update_every)
{
    static COUNTER_DATA atqAverageRequestLatency = {.key = "ATQ Request Latency"};

    static RRDSET *st_atq_average_request_latency = NULL;
    static RRDDIM *rd_atq_average_request_latency = NULL;

    if (!perflibGetObjectCounter(pDataBlock, pObjectType, &atqAverageRequestLatency))
        return;

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

static void netdata_ad_directory_reads(PERF_DATA_BLOCK *pDataBlock, PERF_OBJECT_TYPE *pObjectType, int update_every)
{
    static COUNTER_DATA directoryReadsPerSec = {.key = "DS Directory Reads/sec"};
    static RRDSET *st_directory_reads = NULL;
    static RRDDIM *rd_directory_reads = NULL;

    if (!perflibGetObjectCounter(pDataBlock, pObjectType, &directoryReadsPerSec))
        return;

    if (unlikely(!st_directory_reads)) {
        st_directory_reads = rrdset_create_localhost(
            "ad",
            "directory_reads",
            NULL,
            "directory",
            "ad.directory_reads",
            "Directory reads",
            "reads/s",
            PLUGIN_WINDOWS_NAME,
            "PerflibAD",
            PRIO_AD_DIRECTORY_READS,
            update_every,
            RRDSET_TYPE_LINE);

        rd_directory_reads = rrddim_add(st_directory_reads, "read", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
    }

    rrddim_set_by_pointer(st_directory_reads, rd_directory_reads, (collected_number)directoryReadsPerSec.current.Data);
    rrdset_done(st_directory_reads);
}

static void netdata_ad_directory_searches(PERF_DATA_BLOCK *pDataBlock, PERF_OBJECT_TYPE *pObjectType, int update_every)
{
    static COUNTER_DATA directorySearchesPerSec = {.key = "DS Directory Searches/sec"};
    static RRDSET *st_directory_searches = NULL;
    static RRDDIM *rd_directory_searches = NULL;

    if (!perflibGetObjectCounter(pDataBlock, pObjectType, &directorySearchesPerSec))
        return;

    if (unlikely(!st_directory_searches)) {
        st_directory_searches = rrdset_create_localhost(
            "ad",
            "directory_searches",
            NULL,
            "directory",
            "ad.directory_searches",
            "Directory searches",
            "searches/s",
            PLUGIN_WINDOWS_NAME,
            "PerflibAD",
            PRIO_AD_DIRECTORY_SEARCHES,
            update_every,
            RRDSET_TYPE_LINE);

        rd_directory_searches = rrddim_add(st_directory_searches, "search", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
    }

    rrddim_set_by_pointer(
        st_directory_searches, rd_directory_searches, (collected_number)directorySearchesPerSec.current.Data);
    rrdset_done(st_directory_searches);
}

static void netdata_ad_directory_writes(PERF_DATA_BLOCK *pDataBlock, PERF_OBJECT_TYPE *pObjectType, int update_every)
{
    static COUNTER_DATA directoryWritesPerSec = {.key = "DS Directory Writes/sec"};
    static RRDSET *st_directory_writes = NULL;
    static RRDDIM *rd_directory_writes = NULL;

    if (!perflibGetObjectCounter(pDataBlock, pObjectType, &directoryWritesPerSec))
        return;

    if (unlikely(!st_directory_writes)) {
        st_directory_writes = rrdset_create_localhost(
            "ad",
            "directory_writes",
            NULL,
            "directory",
            "ad.directory_writes",
            "Directory writes",
            "writes/s",
            PLUGIN_WINDOWS_NAME,
            "PerflibAD",
            PRIO_AD_DIRECTORY_WRITES,
            update_every,
            RRDSET_TYPE_LINE);

        rd_directory_writes = rrddim_add(st_directory_writes, "write", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
    }

    rrddim_set_by_pointer(
        st_directory_writes, rd_directory_writes, (collected_number)directoryWritesPerSec.current.Data);
    rrdset_done(st_directory_writes);
}

static void netdata_ad_replication_inbound_object_updates_remaining(
    PERF_DATA_BLOCK *pDataBlock,
    PERF_OBJECT_TYPE *pObjectType,
    int update_every)
{
    static COUNTER_DATA replicationInboundObjectUpdatesRemainingInPacket = {
        .key = "DRA Inbound Object Updates Remaining in Packet"};
    static RRDSET *st_replication_inbound_object_updates_remaining = NULL;
    static RRDDIM *rd_replication_inbound_object_updates_remaining = NULL;

    if (!perflibGetObjectCounter(pDataBlock, pObjectType, &replicationInboundObjectUpdatesRemainingInPacket))
        return;

    if (unlikely(!st_replication_inbound_object_updates_remaining)) {
        st_replication_inbound_object_updates_remaining = rrdset_create_localhost(
            "ad",
            "replication_inbound_object_updates_remaining",
            NULL,
            "replication",
            "ad.replication_inbound_object_updates_remaining",
            "Replication inbound object updates remaining",
            "updates",
            PLUGIN_WINDOWS_NAME,
            "PerflibAD",
            PRIO_AD_REPLICATION_INBOUND_OBJECT_UPDATES_REMAINING,
            update_every,
            RRDSET_TYPE_LINE);

        rd_replication_inbound_object_updates_remaining = rrddim_add(
            st_replication_inbound_object_updates_remaining, "remaining", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
    }

    rrddim_set_by_pointer(
        st_replication_inbound_object_updates_remaining,
        rd_replication_inbound_object_updates_remaining,
        (collected_number)replicationInboundObjectUpdatesRemainingInPacket.current.Data);
    rrdset_done(st_replication_inbound_object_updates_remaining);
}

static void netdata_ad_replication_inbound_values_dns_only(
    PERF_DATA_BLOCK *pDataBlock,
    PERF_OBJECT_TYPE *pObjectType,
    int update_every)
{
    static COUNTER_DATA replicationInboundValuesDNsOnlyPerSec = {.key = "DRA Inbound Values (DNs only)/sec"};
    static RRDSET *st_replication_inbound_values_dns_only = NULL;
    static RRDDIM *rd_replication_inbound_values_dns_only = NULL;

    if (!perflibGetObjectCounter(pDataBlock, pObjectType, &replicationInboundValuesDNsOnlyPerSec))
        return;

    if (unlikely(!st_replication_inbound_values_dns_only)) {
        st_replication_inbound_values_dns_only = rrdset_create_localhost(
            "ad",
            "replication_inbound_values_dns_only",
            NULL,
            "replication",
            "ad.replication_inbound_values_dns_only",
            "DRA replication inbound values DNs only",
            "values/s",
            PLUGIN_WINDOWS_NAME,
            "PerflibAD",
            PRIO_AD_REPLICATION_INBOUND_VALUES_DNS_ONLY,
            update_every,
            RRDSET_TYPE_LINE);

        rd_replication_inbound_values_dns_only =
            rrddim_add(st_replication_inbound_values_dns_only, "dns_only", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
    }

    rrddim_set_by_pointer(
        st_replication_inbound_values_dns_only,
        rd_replication_inbound_values_dns_only,
        (collected_number)replicationInboundValuesDNsOnlyPerSec.current.Data);
    rrdset_done(st_replication_inbound_values_dns_only);
}

static void
netdata_ad_replication_inbound_values_total(PERF_DATA_BLOCK *pDataBlock, PERF_OBJECT_TYPE *pObjectType, int update_every)
{
    static COUNTER_DATA replicationInboundValuesTotalPerSec = {.key = "DRA Inbound Values Total/sec"};
    static RRDSET *st_replication_inbound_values_total = NULL;
    static RRDDIM *rd_replication_inbound_values_total = NULL;

    if (!perflibGetObjectCounter(pDataBlock, pObjectType, &replicationInboundValuesTotalPerSec))
        return;

    if (unlikely(!st_replication_inbound_values_total)) {
        st_replication_inbound_values_total = rrdset_create_localhost(
            "ad",
            "replication_inbound_values_total",
            NULL,
            "replication",
            "ad.replication_inbound_values_total",
            "DRA replication inbound values total",
            "values/s",
            PLUGIN_WINDOWS_NAME,
            "PerflibAD",
            PRIO_AD_REPLICATION_INBOUND_VALUES_TOTAL,
            update_every,
            RRDSET_TYPE_LINE);

        rd_replication_inbound_values_total =
            rrddim_add(st_replication_inbound_values_total, "total", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
    }

    rrddim_set_by_pointer(
        st_replication_inbound_values_total,
        rd_replication_inbound_values_total,
        (collected_number)replicationInboundValuesTotalPerSec.current.Data);
    rrdset_done(st_replication_inbound_values_total);
}

static void netdata_ad_replication_outbound_objects_filtered(
    PERF_DATA_BLOCK *pDataBlock,
    PERF_OBJECT_TYPE *pObjectType,
    int update_every)
{
    static COUNTER_DATA replicationOutboundObjectsFilteredPerSec = {.key = "DRA Outbound Objects Filtered/sec"};
    static RRDSET *st_replication_outbound_objects_filtered = NULL;
    static RRDDIM *rd_replication_outbound_objects_filtered = NULL;

    if (!perflibGetObjectCounter(pDataBlock, pObjectType, &replicationOutboundObjectsFilteredPerSec))
        return;

    if (unlikely(!st_replication_outbound_objects_filtered)) {
        st_replication_outbound_objects_filtered = rrdset_create_localhost(
            "ad",
            "replication_outbound_objects_filtered",
            NULL,
            "replication",
            "ad.replication_outbound_objects_filtered",
            "Replication outbound objects filtered",
            "objects/s",
            PLUGIN_WINDOWS_NAME,
            "PerflibAD",
            PRIO_AD_REPLICATION_OUTBOUND_OBJECTS_FILTERED,
            update_every,
            RRDSET_TYPE_LINE);

        rd_replication_outbound_objects_filtered =
            rrddim_add(st_replication_outbound_objects_filtered, "filtered", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
    }

    rrddim_set_by_pointer(
        st_replication_outbound_objects_filtered,
        rd_replication_outbound_objects_filtered,
        (collected_number)replicationOutboundObjectsFilteredPerSec.current.Data);
    rrdset_done(st_replication_outbound_objects_filtered);
}

static void
netdata_ad_replication_outbound_objects(PERF_DATA_BLOCK *pDataBlock, PERF_OBJECT_TYPE *pObjectType, int update_every)
{
    static COUNTER_DATA replicationOutboundObjectsPerSec = {.key = "DRA Outbound Objects/sec"};
    static RRDSET *st_replication_outbound_objects = NULL;
    static RRDDIM *rd_replication_outbound_objects = NULL;

    if (!perflibGetObjectCounter(pDataBlock, pObjectType, &replicationOutboundObjectsPerSec))
        return;

    if (unlikely(!st_replication_outbound_objects)) {
        st_replication_outbound_objects = rrdset_create_localhost(
            "ad",
            "replication_outbound_objects",
            NULL,
            "replication",
            "ad.replication_outbound_objects",
            "Replication outbound objects",
            "objects/s",
            PLUGIN_WINDOWS_NAME,
            "PerflibAD",
            PRIO_AD_REPLICATION_OUTBOUND_OBJECTS,
            update_every,
            RRDSET_TYPE_LINE);

        rd_replication_outbound_objects =
            rrddim_add(st_replication_outbound_objects, "objects", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
    }

    rrddim_set_by_pointer(
        st_replication_outbound_objects,
        rd_replication_outbound_objects,
        (collected_number)replicationOutboundObjectsPerSec.current.Data);
    rrdset_done(st_replication_outbound_objects);
}

static void
netdata_ad_replication_outbound_properties(PERF_DATA_BLOCK *pDataBlock, PERF_OBJECT_TYPE *pObjectType, int update_every)
{
    static COUNTER_DATA replicationOutboundPropertiesPerSec = {.key = "DRA Outbound Properties/sec"};
    static RRDSET *st_replication_outbound_properties = NULL;
    static RRDDIM *rd_replication_outbound_properties = NULL;

    if (!perflibGetObjectCounter(pDataBlock, pObjectType, &replicationOutboundPropertiesPerSec))
        return;

    if (unlikely(!st_replication_outbound_properties)) {
        st_replication_outbound_properties = rrdset_create_localhost(
            "ad",
            "replication_outbound_properties",
            NULL,
            "replication",
            "ad.replication_outbound_properties",
            "Replication outbound properties",
            "properties/s",
            PLUGIN_WINDOWS_NAME,
            "PerflibAD",
            PRIO_AD_REPLICATION_OUTBOUND_PROPERTIES,
            update_every,
            RRDSET_TYPE_LINE);

        rd_replication_outbound_properties =
            rrddim_add(st_replication_outbound_properties, "properties", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
    }

    rrddim_set_by_pointer(
        st_replication_outbound_properties,
        rd_replication_outbound_properties,
        (collected_number)replicationOutboundPropertiesPerSec.current.Data);
    rrdset_done(st_replication_outbound_properties);
}

static void netdata_ad_replication_outbound_values_dns_only(
    PERF_DATA_BLOCK *pDataBlock,
    PERF_OBJECT_TYPE *pObjectType,
    int update_every)
{
    static COUNTER_DATA replicationOutboundValuesDNsOnlyPerSec = {.key = "DRA Outbound Values (DNs only)/sec"};
    static RRDSET *st_replication_outbound_values_dns_only = NULL;
    static RRDDIM *rd_replication_outbound_values_dns_only = NULL;

    if (!perflibGetObjectCounter(pDataBlock, pObjectType, &replicationOutboundValuesDNsOnlyPerSec))
        return;

    if (unlikely(!st_replication_outbound_values_dns_only)) {
        st_replication_outbound_values_dns_only = rrdset_create_localhost(
            "ad",
            "replication_outbound_values_dns_only",
            NULL,
            "replication",
            "ad.replication_outbound_values_dns_only",
            "DRA replication outbound values DNs only",
            "values/s",
            PLUGIN_WINDOWS_NAME,
            "PerflibAD",
            PRIO_AD_REPLICATION_OUTBOUND_VALUES_DNS_ONLY,
            update_every,
            RRDSET_TYPE_LINE);

        rd_replication_outbound_values_dns_only =
            rrddim_add(st_replication_outbound_values_dns_only, "dns_only", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
    }

    rrddim_set_by_pointer(
        st_replication_outbound_values_dns_only,
        rd_replication_outbound_values_dns_only,
        (collected_number)replicationOutboundValuesDNsOnlyPerSec.current.Data);
    rrdset_done(st_replication_outbound_values_dns_only);
}

static void netdata_ad_replication_outbound_values_total(
    PERF_DATA_BLOCK *pDataBlock,
    PERF_OBJECT_TYPE *pObjectType,
    int update_every)
{
    static COUNTER_DATA replicationOutboundValuesTotalPerSec = {.key = "DRA Outbound Values Total/sec"};
    static RRDSET *st_replication_outbound_values_total = NULL;
    static RRDDIM *rd_replication_outbound_values_total = NULL;

    if (!perflibGetObjectCounter(pDataBlock, pObjectType, &replicationOutboundValuesTotalPerSec))
        return;

    if (unlikely(!st_replication_outbound_values_total)) {
        st_replication_outbound_values_total = rrdset_create_localhost(
            "ad",
            "replication_outbound_values_total",
            NULL,
            "replication",
            "ad.replication_outbound_values_total",
            "DRA replication outbound values total",
            "values/s",
            PLUGIN_WINDOWS_NAME,
            "PerflibAD",
            PRIO_AD_REPLICATION_OUTBOUND_VALUES_TOTAL,
            update_every,
            RRDSET_TYPE_LINE);

        rd_replication_outbound_values_total =
            rrddim_add(st_replication_outbound_values_total, "total", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
    }

    rrddim_set_by_pointer(
        st_replication_outbound_values_total,
        rd_replication_outbound_values_total,
        (collected_number)replicationOutboundValuesTotalPerSec.current.Data);
    rrdset_done(st_replication_outbound_values_total);
}

static void netdata_ad_replication_threads_getting_nc_changes(
    PERF_DATA_BLOCK *pDataBlock,
    PERF_OBJECT_TYPE *pObjectType,
    int update_every)
{
    static COUNTER_DATA draThreadsGettingNCChanges = {.key = "DRA Threads Getting NC Changes"};
    static RRDSET *st_replication_threads_getting_nc_changes = NULL;
    static RRDDIM *rd_replication_threads_getting_nc_changes = NULL;

    if (!perflibGetObjectCounter(pDataBlock, pObjectType, &draThreadsGettingNCChanges))
        return;

    if (unlikely(!st_replication_threads_getting_nc_changes)) {
        st_replication_threads_getting_nc_changes = rrdset_create_localhost(
            "ad",
            "replication_threads_getting_nc_changes",
            NULL,
            "replication",
            "ad.replication_threads_getting_nc_changes",
            "DRA threads getting NC changes",
            "threads",
            PLUGIN_WINDOWS_NAME,
            "PerflibAD",
            PRIO_AD_REPLICATION_THREADS_GETTING_NC_CHANGES,
            update_every,
            RRDSET_TYPE_LINE);

        rd_replication_threads_getting_nc_changes =
            rrddim_add(st_replication_threads_getting_nc_changes, "threads", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
    }

    rrddim_set_by_pointer(
        st_replication_threads_getting_nc_changes,
        rd_replication_threads_getting_nc_changes,
        (collected_number)draThreadsGettingNCChanges.current.Data);
    rrdset_done(st_replication_threads_getting_nc_changes);
}

static void netdata_ad_replication_threads_getting_nc_changes_holding_semaphore(
    PERF_DATA_BLOCK *pDataBlock,
    PERF_OBJECT_TYPE *pObjectType,
    int update_every)
{
    static COUNTER_DATA draThreadsGettingNCChangesHoldingSemaphore = {
        .key = "DRA Threads Getting NC Changes Holding Semaphore"};
    static RRDSET *st_replication_threads_getting_nc_changes_holding_semaphore = NULL;
    static RRDDIM *rd_replication_threads_getting_nc_changes_holding_semaphore = NULL;

    if (!perflibGetObjectCounter(pDataBlock, pObjectType, &draThreadsGettingNCChangesHoldingSemaphore))
        return;

    if (unlikely(!st_replication_threads_getting_nc_changes_holding_semaphore)) {
        st_replication_threads_getting_nc_changes_holding_semaphore = rrdset_create_localhost(
            "ad",
            "replication_threads_getting_nc_changes_holding_semaphore",
            NULL,
            "replication",
            "ad.replication_threads_getting_nc_changes_holding_semaphore",
            "DRA threads getting NC changes holding semaphore",
            "threads",
            PLUGIN_WINDOWS_NAME,
            "PerflibAD",
            PRIO_AD_REPLICATION_THREADS_GETTING_NC_CHANGES_HOLDING_SEMAPHORE,
            update_every,
            RRDSET_TYPE_LINE);

        rd_replication_threads_getting_nc_changes_holding_semaphore = rrddim_add(
            st_replication_threads_getting_nc_changes_holding_semaphore, "threads", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
    }

    rrddim_set_by_pointer(
        st_replication_threads_getting_nc_changes_holding_semaphore,
        rd_replication_threads_getting_nc_changes_holding_semaphore,
        (collected_number)draThreadsGettingNCChangesHoldingSemaphore.current.Data);
    rrdset_done(st_replication_threads_getting_nc_changes_holding_semaphore);
}

static void
netdata_ad_transitive_operations(PERF_DATA_BLOCK *pDataBlock, PERF_OBJECT_TYPE *pObjectType, int update_every)
{
    static COUNTER_DATA transitiveOperationsPerSec = {.key = "Transitive operations/sec"};
    static RRDSET *st_transitive_operations = NULL;
    static RRDDIM *rd_transitive_operations = NULL;

    if (!perflibGetObjectCounter(pDataBlock, pObjectType, &transitiveOperationsPerSec))
        return;

    if (unlikely(!st_transitive_operations)) {
        st_transitive_operations = rrdset_create_localhost(
            "ad",
            "transitive_operations",
            NULL,
            "database",
            "ad.transitive_operations",
            "Transitive operations",
            "operations/s",
            PLUGIN_WINDOWS_NAME,
            "PerflibAD",
            PRIO_AD_TRANSITIVE_OPERATIONS,
            update_every,
            RRDSET_TYPE_LINE);

        rd_transitive_operations =
            rrddim_add(st_transitive_operations, "operations", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
    }

    rrddim_set_by_pointer(
        st_transitive_operations, rd_transitive_operations, (collected_number)transitiveOperationsPerSec.current.Data);
    rrdset_done(st_transitive_operations);
}

static void
netdata_ad_transitive_suboperations(PERF_DATA_BLOCK *pDataBlock, PERF_OBJECT_TYPE *pObjectType, int update_every)
{
    static COUNTER_DATA transitiveSubOperationsPerSec = {.key = "Transitive suboperations/sec"};
    static RRDSET *st_transitive_suboperations = NULL;
    static RRDDIM *rd_transitive_suboperations = NULL;

    if (!perflibGetObjectCounter(pDataBlock, pObjectType, &transitiveSubOperationsPerSec))
        return;

    if (unlikely(!st_transitive_suboperations)) {
        st_transitive_suboperations = rrdset_create_localhost(
            "ad",
            "transitive_suboperations",
            NULL,
            "database",
            "ad.transitive_suboperations",
            "Transitive suboperations",
            "suboperations/s",
            PLUGIN_WINDOWS_NAME,
            "PerflibAD",
            PRIO_AD_TRANSITIVE_SUBOPERATIONS,
            update_every,
            RRDSET_TYPE_LINE);

        rd_transitive_suboperations =
            rrddim_add(st_transitive_suboperations, "suboperations", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
    }

    rrddim_set_by_pointer(
        st_transitive_suboperations,
        rd_transitive_suboperations,
        (collected_number)transitiveSubOperationsPerSec.current.Data);
    rrdset_done(st_transitive_suboperations);
}

static void
netdata_ad_replication_inbound_bytes_total(PERF_DATA_BLOCK *pDataBlock, PERF_OBJECT_TYPE *pObjectType, int update_every)
{
    static COUNTER_DATA replicationInboundBytesTotalPerSec = {.key = "DRA Inbound Bytes Total/sec"};
    static RRDSET *st_replication_inbound_bytes_total = NULL;
    static RRDDIM *rd_replication_inbound_bytes_total = NULL;

    if (!perflibGetObjectCounter(pDataBlock, pObjectType, &replicationInboundBytesTotalPerSec))
        return;

    if (unlikely(!st_replication_inbound_bytes_total)) {
        st_replication_inbound_bytes_total = rrdset_create_localhost(
            "ad",
            "replication_inbound_bytes_total",
            NULL,
            "replication",
            "ad.replication_inbound_bytes_total",
            "DRA replication inbound bytes total",
            "bytes/s",
            PLUGIN_WINDOWS_NAME,
            "PerflibAD",
            PRIO_AD_REPLICATION_INBOUND_BYTES_TOTAL,
            update_every,
            RRDSET_TYPE_LINE);

        rd_replication_inbound_bytes_total =
            rrddim_add(st_replication_inbound_bytes_total, "inbound", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
    }

    rrddim_set_by_pointer(
        st_replication_inbound_bytes_total,
        rd_replication_inbound_bytes_total,
        (collected_number)replicationInboundBytesTotalPerSec.current.Data);
    rrdset_done(st_replication_inbound_bytes_total);
}

static void
netdata_ad_replication_outbound_bytes_total(PERF_DATA_BLOCK *pDataBlock, PERF_OBJECT_TYPE *pObjectType, int update_every)
{
    static COUNTER_DATA replicationOutboundBytesTotalPerSec = {.key = "DRA Outbound Bytes Total/sec"};
    static RRDSET *st_replication_outbound_bytes_total = NULL;
    static RRDDIM *rd_replication_outbound_bytes_total = NULL;

    if (!perflibGetObjectCounter(pDataBlock, pObjectType, &replicationOutboundBytesTotalPerSec))
        return;

    if (unlikely(!st_replication_outbound_bytes_total)) {
        st_replication_outbound_bytes_total = rrdset_create_localhost(
            "ad",
            "replication_outbound_bytes_total",
            NULL,
            "replication",
            "ad.replication_outbound_bytes_total",
            "DRA replication outbound bytes total",
            "bytes/s",
            PLUGIN_WINDOWS_NAME,
            "PerflibAD",
            PRIO_AD_REPLICATION_OUTBOUND_BYTES_TOTAL,
            update_every,
            RRDSET_TYPE_LINE);

        rd_replication_outbound_bytes_total =
            rrddim_add(st_replication_outbound_bytes_total, "outbound", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
    }

    rrddim_set_by_pointer(
        st_replication_outbound_bytes_total,
        rd_replication_outbound_bytes_total,
        (collected_number)replicationOutboundBytesTotalPerSec.current.Data);
    rrdset_done(st_replication_outbound_bytes_total);
}

static void
netdata_ad_ldap_closed_connections(PERF_DATA_BLOCK *pDataBlock, PERF_OBJECT_TYPE *pObjectType, int update_every)
{
    static COUNTER_DATA ldapClosedConnectionsPerSec = {.key = "LDAP Closed Connections/sec"};
    static RRDSET *st_ldap_closed_connections_total = NULL;
    static RRDDIM *rd_ldap_closed_connections_total = NULL;

    if (!perflibGetObjectCounter(pDataBlock, pObjectType, &ldapClosedConnectionsPerSec))
        return;

    if (unlikely(!st_ldap_closed_connections_total)) {
        st_ldap_closed_connections_total = rrdset_create_localhost(
            "ad",
            "ldap_closed_connections",
            NULL,
            "ldap",
            "ad.ldap_closed_connections",
            "LDAP closed connections",
            "connections/s",
            PLUGIN_WINDOWS_NAME,
            "PerflibAD",
            PRIO_AD_LDAP_CLOSED_CONNECTIONS_TOTAL,
            update_every,
            RRDSET_TYPE_LINE);

        rd_ldap_closed_connections_total =
            rrddim_add(st_ldap_closed_connections_total, "closed", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
    }

    rrddim_set_by_pointer(
        st_ldap_closed_connections_total,
        rd_ldap_closed_connections_total,
        (collected_number)ldapClosedConnectionsPerSec.current.Data);
    rrdset_done(st_ldap_closed_connections_total);
}

static void
netdata_ad_ldap_opened_connections_ldap(PERF_DATA_BLOCK *pDataBlock, PERF_OBJECT_TYPE *pObjectType, int update_every)
{
    static COUNTER_DATA ldapNewConnectionsPerSec = {.key = "LDAP New Connections/sec"};
    static RRDSET *st_ldap_opened_connections_ldap = NULL;
    static RRDDIM *rd_ldap_opened_connections_ldap = NULL;

    if (!perflibGetObjectCounter(pDataBlock, pObjectType, &ldapNewConnectionsPerSec))
        return;

    if (unlikely(!st_ldap_opened_connections_ldap)) {
        st_ldap_opened_connections_ldap = rrdset_create_localhost(
            "ad",
            "ldap_opened_connections_ldap",
            NULL,
            "ldap",
            "ad.ldap_opened_connections_ldap",
            "LDAP opened connections",
            "connections/s",
            PLUGIN_WINDOWS_NAME,
            "PerflibAD",
            PRIO_AD_LDAP_OPENED_CONNECTIONS_LDAP,
            update_every,
            RRDSET_TYPE_LINE);

        rd_ldap_opened_connections_ldap =
            rrddim_add(st_ldap_opened_connections_ldap, "ldap", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
    }

    rrddim_set_by_pointer(
        st_ldap_opened_connections_ldap,
        rd_ldap_opened_connections_ldap,
        (collected_number)ldapNewConnectionsPerSec.current.Data);
    rrdset_done(st_ldap_opened_connections_ldap);
}

static void
netdata_ad_ldap_opened_connections_ldaps(PERF_DATA_BLOCK *pDataBlock, PERF_OBJECT_TYPE *pObjectType, int update_every)
{
    static COUNTER_DATA ldapNewSSLConnectionsPerSec = {.key = "LDAP New SSL Connections/sec"};
    static RRDSET *st_ldap_opened_connections_ldaps = NULL;
    static RRDDIM *rd_ldap_opened_connections_ldaps = NULL;

    if (!perflibGetObjectCounter(pDataBlock, pObjectType, &ldapNewSSLConnectionsPerSec))
        return;

    if (unlikely(!st_ldap_opened_connections_ldaps)) {
        st_ldap_opened_connections_ldaps = rrdset_create_localhost(
            "ad",
            "ldap_opened_connections_ldaps",
            NULL,
            "ldap",
            "ad.ldap_opened_connections_ldaps",
            "LDAPS opened connections",
            "connections/s",
            PLUGIN_WINDOWS_NAME,
            "PerflibAD",
            PRIO_AD_LDAP_OPENED_CONNECTIONS_LDAPS,
            update_every,
            RRDSET_TYPE_LINE);

        rd_ldap_opened_connections_ldaps =
            rrddim_add(st_ldap_opened_connections_ldaps, "ldaps", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
    }

    rrddim_set_by_pointer(
        st_ldap_opened_connections_ldaps,
        rd_ldap_opened_connections_ldaps,
        (collected_number)ldapNewSSLConnectionsPerSec.current.Data);
    rrdset_done(st_ldap_opened_connections_ldaps);
}

static void netdata_ad_ldap_active_threads(PERF_DATA_BLOCK *pDataBlock, PERF_OBJECT_TYPE *pObjectType, int update_every)
{
    static COUNTER_DATA ldapActiveThreads = {.key = "LDAP Active Threads"};
    static RRDSET *st_ldap_active_threads = NULL;
    static RRDDIM *rd_ldap_active_threads = NULL;

    if (!perflibGetObjectCounter(pDataBlock, pObjectType, &ldapActiveThreads))
        return;

    if (unlikely(!st_ldap_active_threads)) {
        st_ldap_active_threads = rrdset_create_localhost(
            "ad",
            "ldap_active_threads",
            NULL,
            "threads",
            "ad.ldap_active_threads",
            "LDAP active threads",
            "threads",
            PLUGIN_WINDOWS_NAME,
            "PerflibAD",
            PRIO_AD_LDAP_ACTIVE_THREADS,
            update_every,
            RRDSET_TYPE_LINE);

        rd_ldap_active_threads = rrddim_add(st_ldap_active_threads, "active", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
    }

    rrddim_set_by_pointer(
        st_ldap_active_threads, rd_ldap_active_threads, (collected_number)ldapActiveThreads.current.Data);
    rrdset_done(st_ldap_active_threads);
}

static void netdata_ad_ldap_udp_operations(PERF_DATA_BLOCK *pDataBlock, PERF_OBJECT_TYPE *pObjectType, int update_every)
{
    static COUNTER_DATA ldapUDPOperationsPerSec = {.key = "LDAP UDP operations/sec"};
    static RRDSET *st_ldap_udp_operations_total = NULL;
    static RRDDIM *rd_ldap_udp_operations_total = NULL;

    if (!perflibGetObjectCounter(pDataBlock, pObjectType, &ldapUDPOperationsPerSec))
        return;

    if (unlikely(!st_ldap_udp_operations_total)) {
        st_ldap_udp_operations_total = rrdset_create_localhost(
            "ad",
            "ldap_udp_operations",
            NULL,
            "ldap",
            "ad.ldap_udp_operations",
            "LDAP UDP operations",
            "operations/s",
            PLUGIN_WINDOWS_NAME,
            "PerflibAD",
            PRIO_AD_LDAP_UDP_OPERATIONS_TOTAL,
            update_every,
            RRDSET_TYPE_LINE);

        rd_ldap_udp_operations_total =
            rrddim_add(st_ldap_udp_operations_total, "udp", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
    }

    rrddim_set_by_pointer(
        st_ldap_udp_operations_total,
        rd_ldap_udp_operations_total,
        (collected_number)ldapUDPOperationsPerSec.current.Data);
    rrdset_done(st_ldap_udp_operations_total);
}

static void netdata_ad_ldap_writes(PERF_DATA_BLOCK *pDataBlock, PERF_OBJECT_TYPE *pObjectType, int update_every)
{
    static COUNTER_DATA ldapWritesPerSec = {.key = "LDAP Writes/sec"};
    static RRDSET *st_ldap_writes_total = NULL;
    static RRDDIM *rd_ldap_writes_total = NULL;

    if (!perflibGetObjectCounter(pDataBlock, pObjectType, &ldapWritesPerSec))
        return;

    if (unlikely(!st_ldap_writes_total)) {
        st_ldap_writes_total = rrdset_create_localhost(
            "ad",
            "ldap_writes",
            NULL,
            "ldap",
            "ad.ldap_writes",
            "LDAP writes",
            "writes/s",
            PLUGIN_WINDOWS_NAME,
            "PerflibAD",
            PRIO_AD_LDAP_WRITES_TOTAL,
            update_every,
            RRDSET_TYPE_LINE);

        rd_ldap_writes_total = rrddim_add(st_ldap_writes_total, "writes", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
    }

    rrddim_set_by_pointer(st_ldap_writes_total, rd_ldap_writes_total, (collected_number)ldapWritesPerSec.current.Data);
    rrdset_done(st_ldap_writes_total);
}

static void
netdata_ad_ldap_client_sessions(PERF_DATA_BLOCK *pDataBlock, PERF_OBJECT_TYPE *pObjectType, int update_every)
{
    static COUNTER_DATA ldapClientSessions = {.key = "LDAP Client Sessions"};
    static RRDSET *st_ldap_client_sessions = NULL;
    static RRDDIM *rd_ldap_client_sessions = NULL;

    if (!perflibGetObjectCounter(pDataBlock, pObjectType, &ldapClientSessions))
        return;

    if (unlikely(!st_ldap_client_sessions)) {
        st_ldap_client_sessions = rrdset_create_localhost(
            "ad",
            "ldap_client_sessions",
            NULL,
            "ldap",
            "ad.ldap_client_sessions",
            "LDAP client sessions",
            "sessions",
            PLUGIN_WINDOWS_NAME,
            "PerflibAD",
            PRIO_AD_LDAP_CLIENT_SESSIONS,
            update_every,
            RRDSET_TYPE_LINE);

        rd_ldap_client_sessions = rrddim_add(st_ldap_client_sessions, "sessions", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
    }

    rrddim_set_by_pointer(
        st_ldap_client_sessions, rd_ldap_client_sessions, (collected_number)ldapClientSessions.current.Data);
    rrdset_done(st_ldap_client_sessions);
}

static void netdata_ad_ldap(PERF_DATA_BLOCK *pDataBlock, PERF_OBJECT_TYPE *pObjectType, int update_every)
{
    netdata_ad_ldap_closed_connections(pDataBlock, pObjectType, update_every);
    netdata_ad_ldap_opened_connections_ldap(pDataBlock, pObjectType, update_every);
    netdata_ad_ldap_opened_connections_ldaps(pDataBlock, pObjectType, update_every);
    netdata_ad_ldap_active_threads(pDataBlock, pObjectType, update_every);
    netdata_ad_ldap_udp_operations(pDataBlock, pObjectType, update_every);
    netdata_ad_ldap_writes(pDataBlock, pObjectType, update_every);
    netdata_ad_ldap_client_sessions(pDataBlock, pObjectType, update_every);
}

static void
netdata_ad_cleanup_link_values_cleaned(PERF_DATA_BLOCK *pDataBlock, PERF_OBJECT_TYPE *pObjectType, int update_every)
{
    static COUNTER_DATA linkValuesCleanedPerSec = {.key = "Link Values Cleaned/sec"};
    static RRDSET *st_link_values_cleaned_total = NULL;
    static RRDDIM *rd_link_values_cleaned_total = NULL;

    if (!perflibGetObjectCounter(pDataBlock, pObjectType, &linkValuesCleanedPerSec))
        return;

    if (unlikely(!st_link_values_cleaned_total)) {
        st_link_values_cleaned_total = rrdset_create_localhost(
            "ad",
            "link_values_cleaned",
            NULL,
            "cleanup",
            "ad.link_values_cleaned",
            "Link values cleaned",
            "values/s",
            PLUGIN_WINDOWS_NAME,
            "PerflibAD",
            PRIO_AD_LINK_VALUES_CLEANED_TOTAL,
            update_every,
            RRDSET_TYPE_LINE);

        rd_link_values_cleaned_total =
            rrddim_add(st_link_values_cleaned_total, "cleaned", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
    }

    rrddim_set_by_pointer(
        st_link_values_cleaned_total,
        rd_link_values_cleaned_total,
        (collected_number)linkValuesCleanedPerSec.current.Data);
    rrdset_done(st_link_values_cleaned_total);
}

static void
netdata_ad_cleanup_phantom_objects_cleaned(PERF_DATA_BLOCK *pDataBlock, PERF_OBJECT_TYPE *pObjectType, int update_every)
{
    static COUNTER_DATA phantomsCleanedPerSec = {.key = "Phantoms Cleaned/sec"};
    static RRDSET *st_phantom_objects_cleaned_total = NULL;
    static RRDDIM *rd_phantom_objects_cleaned_total = NULL;

    if (!perflibGetObjectCounter(pDataBlock, pObjectType, &phantomsCleanedPerSec))
        return;

    if (unlikely(!st_phantom_objects_cleaned_total)) {
        st_phantom_objects_cleaned_total = rrdset_create_localhost(
            "ad",
            "phantom_objects_cleaned",
            NULL,
            "cleanup",
            "ad.phantom_objects_cleaned",
            "Phantom objects cleaned",
            "objects/s",
            PLUGIN_WINDOWS_NAME,
            "PerflibAD",
            PRIO_AD_PHANTOM_OBJECTS_CLEANED_TOTAL,
            update_every,
            RRDSET_TYPE_LINE);

        rd_phantom_objects_cleaned_total =
            rrddim_add(st_phantom_objects_cleaned_total, "cleaned", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
    }

    rrddim_set_by_pointer(
        st_phantom_objects_cleaned_total,
        rd_phantom_objects_cleaned_total,
        (collected_number)phantomsCleanedPerSec.current.Data);
    rrdset_done(st_phantom_objects_cleaned_total);
}

static void
netdata_ad_cleanup_phantom_objects_visited(PERF_DATA_BLOCK *pDataBlock, PERF_OBJECT_TYPE *pObjectType, int update_every)
{
    static COUNTER_DATA phantomsVisitedPerSec = {.key = "Phantoms Visited/sec"};
    static RRDSET *st_phantom_objects_visited_total = NULL;
    static RRDDIM *rd_phantom_objects_visited_total = NULL;

    if (!perflibGetObjectCounter(pDataBlock, pObjectType, &phantomsVisitedPerSec))
        return;

    if (unlikely(!st_phantom_objects_visited_total)) {
        st_phantom_objects_visited_total = rrdset_create_localhost(
            "ad",
            "phantom_objects_visited",
            NULL,
            "cleanup",
            "ad.phantom_objects_visited",
            "Phantom objects visited",
            "objects/s",
            PLUGIN_WINDOWS_NAME,
            "PerflibAD",
            PRIO_AD_PHANTOM_OBJECTS_VISITED_TOTAL,
            update_every,
            RRDSET_TYPE_LINE);

        rd_phantom_objects_visited_total =
            rrddim_add(st_phantom_objects_visited_total, "visited", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
    }

    rrddim_set_by_pointer(
        st_phantom_objects_visited_total,
        rd_phantom_objects_visited_total,
        (collected_number)phantomsVisitedPerSec.current.Data);
    rrdset_done(st_phantom_objects_visited_total);
}

static void netdata_ad_cleanup_tombstoned_objects_collected(
    PERF_DATA_BLOCK *pDataBlock,
    PERF_OBJECT_TYPE *pObjectType,
    int update_every)
{
    static COUNTER_DATA tombstonesGarbageCollectedPerSec = {.key = "Tombstones Garbage Collected/sec"};
    static RRDSET *st_tombstoned_objects_collected_total = NULL;
    static RRDDIM *rd_tombstoned_objects_collected_total = NULL;

    if (!perflibGetObjectCounter(pDataBlock, pObjectType, &tombstonesGarbageCollectedPerSec))
        return;

    if (unlikely(!st_tombstoned_objects_collected_total)) {
        st_tombstoned_objects_collected_total = rrdset_create_localhost(
            "ad",
            "tombstoned_objects_collected",
            NULL,
            "cleanup",
            "ad.tombstoned_objects_collected",
            "Tombstoned objects collected",
            "objects/s",
            PLUGIN_WINDOWS_NAME,
            "PerflibAD",
            PRIO_AD_TOMBSTONED_OBJECTS_COLLECTED_TOTAL,
            update_every,
            RRDSET_TYPE_LINE);

        rd_tombstoned_objects_collected_total =
            rrddim_add(st_tombstoned_objects_collected_total, "collected", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
    }

    rrddim_set_by_pointer(
        st_tombstoned_objects_collected_total,
        rd_tombstoned_objects_collected_total,
        (collected_number)tombstonesGarbageCollectedPerSec.current.Data);
    rrdset_done(st_tombstoned_objects_collected_total);
}

static void netdata_ad_cleanup_tombstoned_objects_visited(
    PERF_DATA_BLOCK *pDataBlock,
    PERF_OBJECT_TYPE *pObjectType,
    int update_every)
{
    static COUNTER_DATA tombstonesVisitedPerSec = {.key = "Tombstones Visited/sec"};
    static RRDSET *st_tombstoned_objects_visited_total = NULL;
    static RRDDIM *rd_tombstoned_objects_visited_total = NULL;

    if (!perflibGetObjectCounter(pDataBlock, pObjectType, &tombstonesVisitedPerSec))
        return;

    if (unlikely(!st_tombstoned_objects_visited_total)) {
        st_tombstoned_objects_visited_total = rrdset_create_localhost(
            "ad",
            "tombstoned_objects_visited",
            NULL,
            "cleanup",
            "ad.tombstoned_objects_visited",
            "Tombstoned objects visited",
            "objects/s",
            PLUGIN_WINDOWS_NAME,
            "PerflibAD",
            PRIO_AD_TOMBSTONED_OBJECTS_VISITED_TOTAL,
            update_every,
            RRDSET_TYPE_LINE);

        rd_tombstoned_objects_visited_total =
            rrddim_add(st_tombstoned_objects_visited_total, "visited", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
    }

    rrddim_set_by_pointer(
        st_tombstoned_objects_visited_total,
        rd_tombstoned_objects_visited_total,
        (collected_number)tombstonesVisitedPerSec.current.Data);
    rrdset_done(st_tombstoned_objects_visited_total);
}

static void netdata_ad_cleanup_metrics(PERF_DATA_BLOCK *pDataBlock, PERF_OBJECT_TYPE *pObjectType, int update_every)
{
    netdata_ad_cleanup_link_values_cleaned(pDataBlock, pObjectType, update_every);
    netdata_ad_cleanup_phantom_objects_cleaned(pDataBlock, pObjectType, update_every);
    netdata_ad_cleanup_phantom_objects_visited(pDataBlock, pObjectType, update_every);
    netdata_ad_cleanup_tombstoned_objects_collected(pDataBlock, pObjectType, update_every);
    netdata_ad_cleanup_tombstoned_objects_visited(pDataBlock, pObjectType, update_every);
}

static void
netdata_ad_sam_group_membership_evaluations(PERF_DATA_BLOCK *pDataBlock, PERF_OBJECT_TYPE *pObjectType, int update_every)
{
    static COUNTER_DATA samGlobalGroupMembershipEvaluationsPerSec = {
        .key = "SAM Global Group Membership Evaluations/sec"};
    static COUNTER_DATA samDomainLocalGroupMembershipEvaluationsPerSec = {
        .key = "SAM Domain Local Group Membership Evaluations/sec"};
    static COUNTER_DATA samUniversalGroupMembershipEvaluationsPerSec = {
        .key = "SAM Universal Group Membership Evaluations/sec"};

    static RRDSET *st_sam_group_membership_evaluations = NULL;
    static RRDDIM *rd_sam_group_membership_evaluations_global = NULL;
    static RRDDIM *rd_sam_group_membership_evaluations_domain_local = NULL;
    static RRDDIM *rd_sam_group_membership_evaluations_universal = NULL;

    perflibGetObjectCounter(pDataBlock, pObjectType, &samGlobalGroupMembershipEvaluationsPerSec);
    perflibGetObjectCounter(pDataBlock, pObjectType, &samDomainLocalGroupMembershipEvaluationsPerSec);
    perflibGetObjectCounter(pDataBlock, pObjectType, &samUniversalGroupMembershipEvaluationsPerSec);

    if (unlikely(!st_sam_group_membership_evaluations)) {
        st_sam_group_membership_evaluations = rrdset_create_localhost(
            "ad",
            "sam_group_membership_evaluations",
            NULL,
            "sam",
            "ad.sam_group_membership_evaluations",
            "SAM group membership evaluations",
            "evaluations/s",
            PLUGIN_WINDOWS_NAME,
            "PerflibAD",
            PRIO_AD_SAM_GROUP_MEMBERSHIP_EVALUATIONS_TOTAL,
            update_every,
            RRDSET_TYPE_LINE);

        rd_sam_group_membership_evaluations_global =
            rrddim_add(st_sam_group_membership_evaluations, "global", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
        rd_sam_group_membership_evaluations_domain_local =
            rrddim_add(st_sam_group_membership_evaluations, "domain_local", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
        rd_sam_group_membership_evaluations_universal =
            rrddim_add(st_sam_group_membership_evaluations, "universal", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
    }

    rrddim_set_by_pointer(
        st_sam_group_membership_evaluations,
        rd_sam_group_membership_evaluations_global,
        (collected_number)samGlobalGroupMembershipEvaluationsPerSec.current.Data);
    rrddim_set_by_pointer(
        st_sam_group_membership_evaluations,
        rd_sam_group_membership_evaluations_domain_local,
        (collected_number)samDomainLocalGroupMembershipEvaluationsPerSec.current.Data);
    rrddim_set_by_pointer(
        st_sam_group_membership_evaluations,
        rd_sam_group_membership_evaluations_universal,
        (collected_number)samUniversalGroupMembershipEvaluationsPerSec.current.Data);
    rrdset_done(st_sam_group_membership_evaluations);
}

static void netdata_ad_sam_group_membership_global_catalog_evaluations(
    PERF_DATA_BLOCK *pDataBlock,
    PERF_OBJECT_TYPE *pObjectType,
    int update_every)
{
    static COUNTER_DATA samGCEvaluationsPerSec = {.key = "SAM GC Evaluations/sec"};
    static RRDSET *st_sam_group_membership_global_catalog_evaluations = NULL;
    static RRDDIM *rd_sam_group_membership_global_catalog_evaluations = NULL;

    if (!perflibGetObjectCounter(pDataBlock, pObjectType, &samGCEvaluationsPerSec))
        return;

    if (unlikely(!st_sam_group_membership_global_catalog_evaluations)) {
        st_sam_group_membership_global_catalog_evaluations = rrdset_create_localhost(
            "ad",
            "sam_group_membership_global_catalog_evaluations",
            NULL,
            "sam",
            "ad.sam_group_membership_global_catalog_evaluations",
            "SAM group membership global catalog evaluations",
            "evaluations/s",
            PLUGIN_WINDOWS_NAME,
            "PerflibAD",
            PRIO_AD_SAM_GROUP_MEMBERSHIP_GLOBAL_CATALOG_EVALUATIONS_TOTAL,
            update_every,
            RRDSET_TYPE_LINE);

        rd_sam_group_membership_global_catalog_evaluations = rrddim_add(
            st_sam_group_membership_global_catalog_evaluations, "global_catalog", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
    }

    rrddim_set_by_pointer(
        st_sam_group_membership_global_catalog_evaluations,
        rd_sam_group_membership_global_catalog_evaluations,
        (collected_number)samGCEvaluationsPerSec.current.Data);
    rrdset_done(st_sam_group_membership_global_catalog_evaluations);
}

static void netdata_ad_sam_group_membership_evaluations_nontransitive(
    PERF_DATA_BLOCK *pDataBlock,
    PERF_OBJECT_TYPE *pObjectType,
    int update_every)
{
    static COUNTER_DATA samNonTransitiveMembershipEvaluationsPerSec = {
        .key = "SAM Non-Transitive Membership Evaluations/sec"};
    static RRDSET *st_sam_group_membership_evaluations_nontransitive = NULL;
    static RRDDIM *rd_sam_group_membership_evaluations_nontransitive = NULL;

    if (!perflibGetObjectCounter(pDataBlock, pObjectType, &samNonTransitiveMembershipEvaluationsPerSec))
        return;

    if (unlikely(!st_sam_group_membership_evaluations_nontransitive)) {
        st_sam_group_membership_evaluations_nontransitive = rrdset_create_localhost(
            "ad",
            "sam_group_membership_evaluations_nontransitive",
            NULL,
            "sam",
            "ad.sam_group_membership_evaluations_nontransitive",
            "SAM non-transitive membership evaluations",
            "evaluations/s",
            PLUGIN_WINDOWS_NAME,
            "PerflibAD",
            PRIO_AD_SAM_GROUP_MEMBERSHIP_EVALUATIONS_NONTRANSITIVE_TOTAL,
            update_every,
            RRDSET_TYPE_LINE);

        rd_sam_group_membership_evaluations_nontransitive = rrddim_add(
            st_sam_group_membership_evaluations_nontransitive, "nontransitive", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
    }

    rrddim_set_by_pointer(
        st_sam_group_membership_evaluations_nontransitive,
        rd_sam_group_membership_evaluations_nontransitive,
        (collected_number)samNonTransitiveMembershipEvaluationsPerSec.current.Data);
    rrdset_done(st_sam_group_membership_evaluations_nontransitive);
}

static void netdata_ad_sam_group_membership_evaluations_transitive(
    PERF_DATA_BLOCK *pDataBlock,
    PERF_OBJECT_TYPE *pObjectType,
    int update_every)
{
    static COUNTER_DATA samTransitiveMembershipEvaluationsPerSec = {.key = "SAM Transitive Membership Evaluations/sec"};
    static RRDSET *st_sam_group_membership_evaluations_transitive = NULL;
    static RRDDIM *rd_sam_group_membership_evaluations_transitive = NULL;

    if (!perflibGetObjectCounter(pDataBlock, pObjectType, &samTransitiveMembershipEvaluationsPerSec))
        return;

    if (unlikely(!st_sam_group_membership_evaluations_transitive)) {
        st_sam_group_membership_evaluations_transitive = rrdset_create_localhost(
            "ad",
            "sam_group_membership_evaluations_transitive",
            NULL,
            "sam",
            "ad.sam_group_membership_evaluations_transitive",
            "SAM transitive membership evaluations",
            "evaluations/s",
            PLUGIN_WINDOWS_NAME,
            "PerflibAD",
            PRIO_AD_SAM_GROUP_MEMBERSHIP_EVALUATIONS_TRANSITIVE_TOTAL,
            update_every,
            RRDSET_TYPE_LINE);

        rd_sam_group_membership_evaluations_transitive = rrddim_add(
            st_sam_group_membership_evaluations_transitive, "transitive", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
    }

    rrddim_set_by_pointer(
        st_sam_group_membership_evaluations_transitive,
        rd_sam_group_membership_evaluations_transitive,
        (collected_number)samTransitiveMembershipEvaluationsPerSec.current.Data);
    rrdset_done(st_sam_group_membership_evaluations_transitive);
}

static void
netdata_ad_sam_group_evaluation_latency(PERF_DATA_BLOCK *pDataBlock, PERF_OBJECT_TYPE *pObjectType, int update_every)
{
    static COUNTER_DATA samAccountGroupEvaluationLatency = {.key = "SAM Account Group Evaluation Latency"};
    static COUNTER_DATA samResourceGroupEvaluationLatency = {.key = "SAM Resource Group Evaluation Latency"};

    static RRDSET *st_sam_group_evaluation_latency = NULL;
    static RRDDIM *rd_sam_group_evaluation_latency_account_group = NULL;
    static RRDDIM *rd_sam_group_evaluation_latency_resource_group = NULL;

    perflibGetObjectCounter(pDataBlock, pObjectType, &samAccountGroupEvaluationLatency);
    perflibGetObjectCounter(pDataBlock, pObjectType, &samResourceGroupEvaluationLatency);

    if (unlikely(!st_sam_group_evaluation_latency)) {
        st_sam_group_evaluation_latency = rrdset_create_localhost(
            "ad",
            "sam_group_evaluation_latency",
            NULL,
            "sam",
            "ad.sam_group_evaluation_latency",
            "SAM group evaluation latency",
            "latency",
            PLUGIN_WINDOWS_NAME,
            "PerflibAD",
            PRIO_AD_SAM_GROUP_EVALUATION_LATENCY,
            update_every,
            RRDSET_TYPE_LINE);

        rd_sam_group_evaluation_latency_account_group =
            rrddim_add(st_sam_group_evaluation_latency, "account_group", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
        rd_sam_group_evaluation_latency_resource_group =
            rrddim_add(st_sam_group_evaluation_latency, "resource_group", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
    }

    rrddim_set_by_pointer(
        st_sam_group_evaluation_latency,
        rd_sam_group_evaluation_latency_account_group,
        (collected_number)samAccountGroupEvaluationLatency.current.Data);
    rrddim_set_by_pointer(
        st_sam_group_evaluation_latency,
        rd_sam_group_evaluation_latency_resource_group,
        (collected_number)samResourceGroupEvaluationLatency.current.Data);
    rrdset_done(st_sam_group_evaluation_latency);
}

static void
netdata_ad_sam_computer_creation_requests(PERF_DATA_BLOCK *pDataBlock, PERF_OBJECT_TYPE *pObjectType, int update_every)
{
    static COUNTER_DATA samMachineCreationAttemptsPerSec = {.key = "SAM Machine Creation Attempts/sec"};
    static RRDSET *st_sam_computer_creation_requests = NULL;
    static RRDDIM *rd_sam_computer_creation_requests = NULL;

    if (!perflibGetObjectCounter(pDataBlock, pObjectType, &samMachineCreationAttemptsPerSec))
        return;

    if (unlikely(!st_sam_computer_creation_requests)) {
        st_sam_computer_creation_requests = rrdset_create_localhost(
            "ad",
            "sam_computer_creation_requests",
            NULL,
            "sam",
            "ad.sam_computer_creation_requests",
            "SAM computer creation requests",
            "requests/s",
            PLUGIN_WINDOWS_NAME,
            "PerflibAD",
            PRIO_AD_SAM_COMPUTER_CREATION_REQUESTS_TOTAL,
            update_every,
            RRDSET_TYPE_LINE);

        rd_sam_computer_creation_requests =
            rrddim_add(st_sam_computer_creation_requests, "request", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
    }

    rrddim_set_by_pointer(
        st_sam_computer_creation_requests,
        rd_sam_computer_creation_requests,
        (collected_number)samMachineCreationAttemptsPerSec.current.Data);
    rrdset_done(st_sam_computer_creation_requests);
}

static void netdata_ad_sam_computer_creation_successful_requests(
    PERF_DATA_BLOCK *pDataBlock,
    PERF_OBJECT_TYPE *pObjectType,
    int update_every)
{
    static COUNTER_DATA samSuccessfulComputerCreationsPerSecIncludesAllRequests = {
        .key = "SAM Successful Computer Creations/sec: Includes all requests"};
    static RRDSET *st_sam_computer_creation_successful_requests = NULL;
    static RRDDIM *rd_sam_computer_creation_successful_requests = NULL;

    if (!perflibGetObjectCounter(pDataBlock, pObjectType, &samSuccessfulComputerCreationsPerSecIncludesAllRequests))
        return;

    if (unlikely(!st_sam_computer_creation_successful_requests)) {
        st_sam_computer_creation_successful_requests = rrdset_create_localhost(
            "ad",
            "sam_computer_creation_successful_requests",
            NULL,
            "sam",
            "ad.sam_computer_creation_successful_requests",
            "SAM successful computer creation requests",
            "requests/s",
            PLUGIN_WINDOWS_NAME,
            "PerflibAD",
            PRIO_AD_SAM_COMPUTER_CREATION_SUCCESSFUL_REQUESTS_TOTAL,
            update_every,
            RRDSET_TYPE_LINE);

        rd_sam_computer_creation_successful_requests =
            rrddim_add(st_sam_computer_creation_successful_requests, "success", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
    }

    rrddim_set_by_pointer(
        st_sam_computer_creation_successful_requests,
        rd_sam_computer_creation_successful_requests,
        (collected_number)samSuccessfulComputerCreationsPerSecIncludesAllRequests.current.Data);
    rrdset_done(st_sam_computer_creation_successful_requests);
}

static void
netdata_ad_sam_user_creation_requests(PERF_DATA_BLOCK *pDataBlock, PERF_OBJECT_TYPE *pObjectType, int update_every)
{
    static COUNTER_DATA samUserCreationAttemptsPerSec = {.key = "SAM User Creation Attempts/sec"};
    static RRDSET *st_sam_user_creation_requests = NULL;
    static RRDDIM *rd_sam_user_creation_requests = NULL;

    if (!perflibGetObjectCounter(pDataBlock, pObjectType, &samUserCreationAttemptsPerSec))
        return;

    if (unlikely(!st_sam_user_creation_requests)) {
        st_sam_user_creation_requests = rrdset_create_localhost(
            "ad",
            "sam_user_creation_requests",
            NULL,
            "sam",
            "ad.sam_user_creation_requests",
            "SAM user creation requests",
            "requests/s",
            PLUGIN_WINDOWS_NAME,
            "PerflibAD",
            PRIO_AD_SAM_USER_CREATION_REQUESTS_TOTAL,
            update_every,
            RRDSET_TYPE_LINE);

        rd_sam_user_creation_requests =
            rrddim_add(st_sam_user_creation_requests, "request", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
    }

    rrddim_set_by_pointer(
        st_sam_user_creation_requests,
        rd_sam_user_creation_requests,
        (collected_number)samUserCreationAttemptsPerSec.current.Data);
    rrdset_done(st_sam_user_creation_requests);
}

static void netdata_ad_sam_user_creation_successful_requests(
    PERF_DATA_BLOCK *pDataBlock,
    PERF_OBJECT_TYPE *pObjectType,
    int update_every)
{
    static COUNTER_DATA samSuccessfulUserCreationsPerSec = {.key = "SAM Successful User Creations/sec"};
    static RRDSET *st_sam_user_creation_successful_requests = NULL;
    static RRDDIM *rd_sam_user_creation_successful_requests = NULL;

    if (!perflibGetObjectCounter(pDataBlock, pObjectType, &samSuccessfulUserCreationsPerSec))
        return;

    if (unlikely(!st_sam_user_creation_successful_requests)) {
        st_sam_user_creation_successful_requests = rrdset_create_localhost(
            "ad",
            "sam_user_creation_successful_requests",
            NULL,
            "sam",
            "ad.sam_user_creation_successful_requests",
            "SAM successful user creation requests",
            "requests/s",
            PLUGIN_WINDOWS_NAME,
            "PerflibAD",
            PRIO_AD_SAM_USER_CREATION_SUCCESSFUL_REQUESTS_TOTAL,
            update_every,
            RRDSET_TYPE_LINE);

        rd_sam_user_creation_successful_requests =
            rrddim_add(st_sam_user_creation_successful_requests, "success", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
    }

    rrddim_set_by_pointer(
        st_sam_user_creation_successful_requests,
        rd_sam_user_creation_successful_requests,
        (collected_number)samSuccessfulUserCreationsPerSec.current.Data);
    rrdset_done(st_sam_user_creation_successful_requests);
}

static void
netdata_ad_sam_query_display_requests(PERF_DATA_BLOCK *pDataBlock, PERF_OBJECT_TYPE *pObjectType, int update_every)
{
    static COUNTER_DATA samDisplayInformationQueriesPerSec = {.key = "SAM Display Information Queries/sec"};
    static RRDSET *st_sam_query_display_requests = NULL;
    static RRDDIM *rd_sam_query_display_requests = NULL;

    if (!perflibGetObjectCounter(pDataBlock, pObjectType, &samDisplayInformationQueriesPerSec))
        return;

    if (unlikely(!st_sam_query_display_requests)) {
        st_sam_query_display_requests = rrdset_create_localhost(
            "ad",
            "sam_query_display_requests",
            NULL,
            "sam",
            "ad.sam_query_display_requests",
            "SAM query display requests",
            "requests/s",
            PLUGIN_WINDOWS_NAME,
            "PerflibAD",
            PRIO_AD_SAM_QUERY_DISPLAY_REQUESTS_TOTAL,
            update_every,
            RRDSET_TYPE_LINE);

        rd_sam_query_display_requests =
            rrddim_add(st_sam_query_display_requests, "query", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
    }

    rrddim_set_by_pointer(
        st_sam_query_display_requests,
        rd_sam_query_display_requests,
        (collected_number)samDisplayInformationQueriesPerSec.current.Data);
    rrdset_done(st_sam_query_display_requests);
}

static void netdata_ad_sam_enumerations(PERF_DATA_BLOCK *pDataBlock, PERF_OBJECT_TYPE *pObjectType, int update_every)
{
    static COUNTER_DATA samEnumerationsPerSec = {.key = "SAM Enumerations/sec"};
    static RRDSET *st_sam_enumerations = NULL;
    static RRDDIM *rd_sam_enumerations = NULL;

    if (!perflibGetObjectCounter(pDataBlock, pObjectType, &samEnumerationsPerSec))
        return;

    if (unlikely(!st_sam_enumerations)) {
        st_sam_enumerations = rrdset_create_localhost(
            "ad",
            "sam_enumerations",
            NULL,
            "sam",
            "ad.sam_enumerations",
            "SAM enumerations",
            "enumerations/s",
            PLUGIN_WINDOWS_NAME,
            "PerflibAD",
            PRIO_AD_SAM_ENUMERATIONS_TOTAL,
            update_every,
            RRDSET_TYPE_LINE);

        rd_sam_enumerations = rrddim_add(st_sam_enumerations, "enumeration", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
    }

    rrddim_set_by_pointer(
        st_sam_enumerations, rd_sam_enumerations, (collected_number)samEnumerationsPerSec.current.Data);
    rrdset_done(st_sam_enumerations);
}

static void
netdata_ad_sam_membership_changes(PERF_DATA_BLOCK *pDataBlock, PERF_OBJECT_TYPE *pObjectType, int update_every)
{
    static COUNTER_DATA samMembershipChangesPerSec = {.key = "SAM Membership Changes/sec"};
    static RRDSET *st_sam_membership_changes = NULL;
    static RRDDIM *rd_sam_membership_changes = NULL;

    if (!perflibGetObjectCounter(pDataBlock, pObjectType, &samMembershipChangesPerSec))
        return;

    if (unlikely(!st_sam_membership_changes)) {
        st_sam_membership_changes = rrdset_create_localhost(
            "ad",
            "sam_membership_changes",
            NULL,
            "sam",
            "ad.sam_membership_changes",
            "SAM membership changes",
            "changes/s",
            PLUGIN_WINDOWS_NAME,
            "PerflibAD",
            PRIO_AD_SAM_MEMBERSHIP_CHANGES_TOTAL,
            update_every,
            RRDSET_TYPE_LINE);

        rd_sam_membership_changes =
            rrddim_add(st_sam_membership_changes, "change", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
    }

    rrddim_set_by_pointer(
        st_sam_membership_changes,
        rd_sam_membership_changes,
        (collected_number)samMembershipChangesPerSec.current.Data);
    rrdset_done(st_sam_membership_changes);
}

static void
netdata_ad_sam_password_changes(PERF_DATA_BLOCK *pDataBlock, PERF_OBJECT_TYPE *pObjectType, int update_every)
{
    static COUNTER_DATA samPasswordChangesPerSec = {.key = "SAM Password Changes/sec"};
    static RRDSET *st_sam_password_changes = NULL;
    static RRDDIM *rd_sam_password_changes = NULL;

    if (!perflibGetObjectCounter(pDataBlock, pObjectType, &samPasswordChangesPerSec))
        return;

    if (unlikely(!st_sam_password_changes)) {
        st_sam_password_changes = rrdset_create_localhost(
            "ad",
            "sam_password_changes",
            NULL,
            "sam",
            "ad.sam_password_changes",
            "SAM password changes",
            "changes/s",
            PLUGIN_WINDOWS_NAME,
            "PerflibAD",
            PRIO_AD_SAM_PASSWORD_CHANGES_TOTAL,
            update_every,
            RRDSET_TYPE_LINE);

        rd_sam_password_changes = rrddim_add(st_sam_password_changes, "change", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
    }

    rrddim_set_by_pointer(
        st_sam_password_changes, rd_sam_password_changes, (collected_number)samPasswordChangesPerSec.current.Data);
    rrdset_done(st_sam_password_changes);
}

static void netdata_ad_sam(PERF_DATA_BLOCK *pDataBlock, PERF_OBJECT_TYPE *pObjectType, int update_every)
{
    netdata_ad_sam_group_membership_evaluations(pDataBlock, pObjectType, update_every);
    netdata_ad_sam_group_membership_global_catalog_evaluations(pDataBlock, pObjectType, update_every);
    netdata_ad_sam_group_membership_evaluations_nontransitive(pDataBlock, pObjectType, update_every);
    netdata_ad_sam_group_membership_evaluations_transitive(pDataBlock, pObjectType, update_every);
    netdata_ad_sam_group_evaluation_latency(pDataBlock, pObjectType, update_every);
    netdata_ad_sam_computer_creation_requests(pDataBlock, pObjectType, update_every);
    netdata_ad_sam_computer_creation_successful_requests(pDataBlock, pObjectType, update_every);
    netdata_ad_sam_user_creation_requests(pDataBlock, pObjectType, update_every);
    netdata_ad_sam_user_creation_successful_requests(pDataBlock, pObjectType, update_every);
    netdata_ad_sam_query_display_requests(pDataBlock, pObjectType, update_every);
    netdata_ad_sam_enumerations(pDataBlock, pObjectType, update_every);
    netdata_ad_sam_membership_changes(pDataBlock, pObjectType, update_every);
    netdata_ad_sam_password_changes(pDataBlock, pObjectType, update_every);
}

static bool do_AD(PERF_DATA_BLOCK *pDataBlock, int update_every)
{
    PERF_OBJECT_TYPE *pObjectType = perflibFindObjectTypeByName(pDataBlock, "DirectoryServices");
    if (!pObjectType)
        return false;

    static void (*doAD[])(PERF_DATA_BLOCK *, PERF_OBJECT_TYPE *, int) = {
        netdata_ad_address_book,
        netdata_ad_approximate_highest_dnt,
        netdata_ad_directory,
        netdata_ad_search_scope,
        netdata_ad_cache_lookups,
        netdata_ad_properties,
        netdata_ad_compressed_traffic,
        netdata_ad_replication_highest_usn,
        netdata_ad_replication_inbound,
        netdata_ad_sync,
        netdata_ad_replication_pending_operations,
        netdata_ad_sync_result,
        netdata_ad_cache_hits,
        netdata_ad_name_translations,
        netdata_ad_change_monitors,
        netdata_ad_directory_search_suboperations,
        netdata_ad_security_descriptor_propagation,
        netdata_ad_service_threads_in_use,
        netdata_ad_bind,
        netdata_ad_searches,
        netdata_ad_ldap,
        netdata_ad_cleanup_metrics,
        netdata_ad_sam,
        netdata_ad_atq_queue_requests,
        netdata_ad_atq_estimated_delay,
        netdata_ad_atq_latency,
        netdata_ad_atq_current_threads,
        netdata_ad_op_total,
        netdata_ad_directory_reads,
        netdata_ad_directory_searches,
        netdata_ad_directory_writes,
        netdata_ad_replication_inbound_object_updates_remaining,
        netdata_ad_replication_inbound_values_dns_only,
        netdata_ad_replication_inbound_values_total,
        netdata_ad_replication_outbound_objects_filtered,
        netdata_ad_replication_outbound_objects,
        netdata_ad_replication_outbound_properties,
        netdata_ad_replication_outbound_values_dns_only,
        netdata_ad_replication_outbound_values_total,
        netdata_ad_replication_threads_getting_nc_changes,
        netdata_ad_replication_threads_getting_nc_changes_holding_semaphore,
        netdata_ad_transitive_operations,
        netdata_ad_transitive_suboperations,
        netdata_ad_replication_inbound_bytes_total,
        netdata_ad_replication_outbound_bytes_total,

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
