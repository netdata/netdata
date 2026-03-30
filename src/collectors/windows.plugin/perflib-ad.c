// SPDX-License-Identifier: GPL-3.0-or-later

#include "windows_plugin.h"
#include "windows-internals.h"

static void netdata_ad_address_book_ambiguous_name_resolution(
    PERF_DATA_BLOCK *pDataBlock, PERF_OBJECT_TYPE *pObjectType, int update_every)
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

    rrddim_set_by_pointer(st_address_book_browse, rd_address_book_browse, (collected_number)addressBookBrowsesPerSec.current.Data);
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

    rrddim_set_by_pointer(st_address_book_find, rd_address_book_find, (collected_number)addressBookMatchesPerSec.current.Data);
    rrdset_done(st_address_book_find);
}

static void netdata_ad_address_book_property_read(
    PERF_DATA_BLOCK *pDataBlock, PERF_OBJECT_TYPE *pObjectType, int update_every)
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

        rd_address_book_property_read = rrddim_add(
            st_address_book_property_read, "property_read", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
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

    rrddim_set_by_pointer(st_address_book_search, rd_address_book_search, (collected_number)addressBookSearchesPerSec.current.Data);
    rrdset_done(st_address_book_search);
}

static void netdata_ad_address_book_proxy_search(
    PERF_DATA_BLOCK *pDataBlock, PERF_OBJECT_TYPE *pObjectType, int update_every)
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
        st_address_book_proxy_search, rd_address_book_proxy_search, (collected_number)addressBookProxyLookupsPerSec.current.Data);
    rrdset_done(st_address_book_proxy_search);
}

static void netdata_ad_address_book_client_sessions(
    PERF_DATA_BLOCK *pDataBlock, PERF_OBJECT_TYPE *pObjectType, int update_every)
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

        rd_address_book_client_sessions = rrddim_add(
            st_address_book_client_sessions, "sessions", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
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

static void netdata_ad_approximate_highest_dnt(
    PERF_DATA_BLOCK *pDataBlock, PERF_OBJECT_TYPE *pObjectType, int update_every)
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

        rd_approximate_highest_dnt =
            rrddim_add(st_approximate_highest_dnt, "dnt", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
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

    rrddim_set_by_pointer(st_search_scope_base, rd_search_scope_base, (collected_number)baseSearchesPerSec.current.Data);
    rrdset_done(st_search_scope_base);
}

static void netdata_ad_search_scope_subtree(PERF_DATA_BLOCK *pDataBlock, PERF_OBJECT_TYPE *pObjectType, int update_every)
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

        rd_search_scope_subtree =
            rrddim_add(st_search_scope_subtree, "subtree", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
    }

    rrddim_set_by_pointer(
        st_search_scope_subtree, rd_search_scope_subtree, (collected_number)subtreeSearchesPerSec.current.Data);
    rrdset_done(st_search_scope_subtree);
}

static void netdata_ad_search_scope_one_level(
    PERF_DATA_BLOCK *pDataBlock, PERF_OBJECT_TYPE *pObjectType, int update_every)
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
        st_name_cache_lookups_total, rd_name_cache_lookups_total, (collected_number)nameCacheLookupsTotal.current.Data);
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

    if (unlikely(!st_dra_replication_intrasite_compressed_traffic)) {
        st_dra_replication_intrasite_compressed_traffic = rrdset_create_localhost(
            "ad",
            "dra_replication_intrasite_compressed_traffic",
            NULL,
            "replication",
            "ad.dra_replication_intrasite_compressed_traffic",
            "DRA replication uncompressed traffic within site",
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

static void netdata_ad_replication_highest_usn_committed(
    PERF_DATA_BLOCK *pDataBlock, PERF_OBJECT_TYPE *pObjectType, int update_every)
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
            "usn",
            PLUGIN_WINDOWS_NAME,
            "PerflibAD",
            PRIO_AD_REPLICATION_HIGHEST_USN_COMMITTED,
            update_every,
            RRDSET_TYPE_LINE);

        rd_replication_highest_usn_committed =
            rrddim_add(st_replication_highest_usn_committed, "committed", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
    }

    // Windows exposes the 64-bit USN as separate high/low perf counters.
    uint64_t committed = ((uint64_t)replicationHighestUSNCommittedHighPart.current.Data << 32) |
                         (uint64_t)replicationHighestUSNCommittedLowPart.current.Data;

    rrddim_set_by_pointer(
        st_replication_highest_usn_committed, rd_replication_highest_usn_committed, (collected_number)committed);
    rrdset_done(st_replication_highest_usn_committed);
}

static void netdata_ad_replication_highest_usn_issued(
    PERF_DATA_BLOCK *pDataBlock, PERF_OBJECT_TYPE *pObjectType, int update_every)
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
            "usn",
            PLUGIN_WINDOWS_NAME,
            "PerflibAD",
            PRIO_AD_REPLICATION_HIGHEST_USN_ISSUED,
            update_every,
            RRDSET_TYPE_LINE);

        rd_replication_highest_usn_issued =
            rrddim_add(st_replication_highest_usn_issued, "issued", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
    }

    // Windows exposes the 64-bit USN as separate high/low perf counters.
    uint64_t issued = ((uint64_t)replicationHighestUSNIssuedHighPart.current.Data << 32) |
                      (uint64_t)replicationHighestUSNIssuedLowPart.current.Data;

    rrddim_set_by_pointer(
        st_replication_highest_usn_issued, rd_replication_highest_usn_issued, (collected_number)issued);
    rrdset_done(st_replication_highest_usn_issued);
}

static void netdata_ad_replication_highest_usn(
    PERF_DATA_BLOCK *pDataBlock, PERF_OBJECT_TYPE *pObjectType, int update_every)
{
    netdata_ad_replication_highest_usn_committed(pDataBlock, pObjectType, update_every);
    netdata_ad_replication_highest_usn_issued(pDataBlock, pObjectType, update_every);
}

static void netdata_ad_replication_inbound_sync_objects_remaining(
    PERF_DATA_BLOCK *pDataBlock, PERF_OBJECT_TYPE *pObjectType, int update_every)
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

        rd_replication_inbound_sync_objects_remaining = rrddim_add(
            st_replication_inbound_sync_objects_remaining, "remaining", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
    }

    rrddim_set_by_pointer(
        st_replication_inbound_sync_objects_remaining,
        rd_replication_inbound_sync_objects_remaining,
        (collected_number)replicationInboundSyncObjectsRemaining.current.Data);
    rrdset_done(st_replication_inbound_sync_objects_remaining);
}

static void netdata_ad_replication_inbound_link_value_updates_remaining(
    PERF_DATA_BLOCK *pDataBlock, PERF_OBJECT_TYPE *pObjectType, int update_every)
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
    PERF_DATA_BLOCK *pDataBlock, PERF_OBJECT_TYPE *pObjectType, int update_every)
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
    PERF_DATA_BLOCK *pDataBlock, PERF_OBJECT_TYPE *pObjectType, int update_every)
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

static void netdata_ad_replication_pending_operations(
    PERF_DATA_BLOCK *pDataBlock, PERF_OBJECT_TYPE *pObjectType, int update_every)
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

        rd_replication_sync_requests_success = rrddim_add(
            st_replication_sync_requests_success, "success", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
    }

    rrddim_set_by_pointer(
        st_replication_sync_requests_success,
        rd_replication_sync_requests_success,
        (collected_number)replicationSyncRequestsSuccessTotal.current.Data);
    rrdset_done(st_replication_sync_requests_success);
}

static void netdata_ad_sync_result_schema_mismatch_failure(
    PERF_DATA_BLOCK *pDataBlock, PERF_OBJECT_TYPE *pObjectType, int update_every)
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

static void netdata_ad_name_translations_client(
    PERF_DATA_BLOCK *pDataBlock, PERF_OBJECT_TYPE *pObjectType, int update_every)
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

static void netdata_ad_name_translations_server(
    PERF_DATA_BLOCK *pDataBlock, PERF_OBJECT_TYPE *pObjectType, int update_every)
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

static void netdata_ad_change_monitors_registered(
    PERF_DATA_BLOCK *pDataBlock, PERF_OBJECT_TYPE *pObjectType, int update_every)
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

static void netdata_ad_change_monitor_updates_pending(
    PERF_DATA_BLOCK *pDataBlock, PERF_OBJECT_TYPE *pObjectType, int update_every)
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

static void netdata_ad_directory_search_suboperations(
    PERF_DATA_BLOCK *pDataBlock, PERF_OBJECT_TYPE *pObjectType, int update_every)
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

        rd_directory_search_suboperations_total = rrddim_add(
            st_directory_search_suboperations_total, "suboperations", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
    }

    rrddim_set_by_pointer(
        st_directory_search_suboperations_total,
        rd_directory_search_suboperations_total,
        (collected_number)directorySearchSubOperationsPerSec.current.Data);
    rrdset_done(st_directory_search_suboperations_total);
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
            "bind/s",
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
            "bind/s",
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
            "bind/s",
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
            "bind/s",
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
            "bind/s",
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
            "bind/s",
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
            "bind/s",
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
            "bind/s",
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
            "bind/s",
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

        rd_atq_estimated_delay =
            rrddim_add(st_atq_estimated_delay, "delay", NULL, 1, 1000, RRD_ALGORITHM_ABSOLUTE);
    }

    rrddim_set_by_pointer(st_atq_estimated_delay, rd_atq_estimated_delay, (collected_number)atqEstimatedDelay.current.Data);
    rrdset_done(st_atq_estimated_delay);
}

static void netdata_ad_atq_current_threads_ldap(
    PERF_DATA_BLOCK *pDataBlock, PERF_OBJECT_TYPE *pObjectType, int update_every)
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

static void netdata_ad_atq_current_threads_other(
    PERF_DATA_BLOCK *pDataBlock, PERF_OBJECT_TYPE *pObjectType, int update_every)
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

static void netdata_ad_atq_current_threads(PERF_DATA_BLOCK *pDataBlock, PERF_OBJECT_TYPE *pObjectType, int update_every)
{
    netdata_ad_atq_current_threads_ldap(pDataBlock, pObjectType, update_every);
    netdata_ad_atq_current_threads_other(pDataBlock, pObjectType, update_every);
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

static void netdata_ad_ldap_closed_connections(PERF_DATA_BLOCK *pDataBlock, PERF_OBJECT_TYPE *pObjectType, int update_every)
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

static void netdata_ad_ldap_opened_connections_ldap(
    PERF_DATA_BLOCK *pDataBlock, PERF_OBJECT_TYPE *pObjectType, int update_every)
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

static void netdata_ad_ldap_opened_connections_ldaps(
    PERF_DATA_BLOCK *pDataBlock, PERF_OBJECT_TYPE *pObjectType, int update_every)
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

    rrddim_set_by_pointer(st_ldap_active_threads, rd_ldap_active_threads, (collected_number)ldapActiveThreads.current.Data);
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

static void netdata_ad_ldap_client_sessions(PERF_DATA_BLOCK *pDataBlock, PERF_OBJECT_TYPE *pObjectType, int update_every)
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

        rd_ldap_client_sessions =
            rrddim_add(st_ldap_client_sessions, "sessions", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
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

static void netdata_ad_cleanup_link_values_cleaned(
    PERF_DATA_BLOCK *pDataBlock, PERF_OBJECT_TYPE *pObjectType, int update_every)
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
        st_link_values_cleaned_total, rd_link_values_cleaned_total, (collected_number)linkValuesCleanedPerSec.current.Data);
    rrdset_done(st_link_values_cleaned_total);
}

static void netdata_ad_cleanup_phantom_objects_cleaned(
    PERF_DATA_BLOCK *pDataBlock, PERF_OBJECT_TYPE *pObjectType, int update_every)
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

static void netdata_ad_cleanup_phantom_objects_visited(
    PERF_DATA_BLOCK *pDataBlock, PERF_OBJECT_TYPE *pObjectType, int update_every)
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
    PERF_DATA_BLOCK *pDataBlock, PERF_OBJECT_TYPE *pObjectType, int update_every)
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
    PERF_DATA_BLOCK *pDataBlock, PERF_OBJECT_TYPE *pObjectType, int update_every)
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
        netdata_ad_service_threads_in_use,
        netdata_ad_bind,
        netdata_ad_searches,
        netdata_ad_ldap,
        netdata_ad_cleanup_metrics,
        netdata_ad_atq_queue_requests,
        netdata_ad_atq_estimated_delay,
        netdata_ad_atq_latency,
        netdata_ad_atq_current_threads,
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
