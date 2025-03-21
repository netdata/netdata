// SPDX-License-Identifier: GPL-3.0-or-later

#define PULSE_INTERNALS 1
#include "pulse-daemon-memory.h"
#include "streaming/stream-replication-sender.h"

static size_t rrd_slot_memory = 0;

void rrd_slot_memory_added(size_t added) {
    __atomic_add_fetch(&rrd_slot_memory, added, __ATOMIC_RELAXED);
}

void rrd_slot_memory_removed(size_t added) {
    __atomic_sub_fetch(&rrd_slot_memory, added, __ATOMIC_RELAXED);
}

#define dictionary_stats_memory_total(stats) \
    ((stats).memory.dict + (stats).memory.values + (stats).memory.index)

struct netdata_buffers_statistics netdata_buffers_statistics = { 0 };

void pulse_daemon_memory_do(bool extended __maybe_unused) {

    {
        static RRDSET *st_memory = NULL;
        static RRDDIM *rd_db_dbengine = NULL;
        static RRDDIM *rd_db_rrd = NULL;
        static RRDDIM *rd_db_sqlite3 = NULL;

#ifdef DICT_WITH_STATS
        static RRDDIM *rd_collectors = NULL;
        static RRDDIM *rd_rrdhosts = NULL;
        static RRDDIM *rd_rrdsets = NULL;
        static RRDDIM *rd_rrddims = NULL;
        static RRDDIM *rd_contexts = NULL;
        static RRDDIM *rd_health = NULL;
        static RRDDIM *rd_functions = NULL;
        static RRDDIM *rd_replication = NULL;
#else
        static RRDDIM *rd_metadata = NULL;
#endif
        static RRDDIM *rd_uuid = NULL;
        static RRDDIM *rd_labels = NULL; // labels use dictionary like statistics, but it is not ARAL based dictionary
        static RRDDIM *rd_ml = NULL;
        static RRDDIM *rd_strings = NULL;
        static RRDDIM *rd_streaming = NULL;
        static RRDDIM *rd_buffers = NULL;
        static RRDDIM *rd_workers = NULL;
        static RRDDIM *rd_aral = NULL;
        static RRDDIM *rd_judy = NULL;
        static RRDDIM *rd_slots = NULL;
        static RRDDIM *rd_other = NULL;

        if (unlikely(!st_memory)) {
            st_memory = rrdset_create_localhost(
                "netdata",
                "memory",
                NULL,
                "Memory Usage",
                NULL,
                "Netdata Memory",
                "bytes",
                "netdata",
                "pulse",
                130100,
                localhost->rrd_update_every,
                RRDSET_TYPE_STACKED);

            rd_db_dbengine = rrddim_add(st_memory, "dbengine", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
            rd_db_rrd = rrddim_add(st_memory, "rrd", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
            rd_db_sqlite3 = rrddim_add(st_memory, "sqlite3", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);

#ifdef DICT_WITH_STATS
            rd_collectors = rrddim_add(st_memory, "collectors", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
            rd_rrdhosts = rrddim_add(st_memory, "hosts", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
            rd_rrdsets = rrddim_add(st_memory, "rrdset", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
            rd_rrddims = rrddim_add(st_memory, "rrddim", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
            rd_contexts = rrddim_add(st_memory, "contexts", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
            rd_health = rrddim_add(st_memory, "health", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
            rd_functions = rrddim_add(st_memory, "functions", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
            rd_replication = rrddim_add(st_memory, "replication", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
#else
            rd_metadata = rrddim_add(st_memory, "metadata", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
#endif
            rd_uuid = rrddim_add(st_memory, "uuid", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
            rd_labels = rrddim_add(st_memory, "labels", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
            rd_ml = rrddim_add(st_memory, "ML", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
            rd_strings = rrddim_add(st_memory, "strings", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
            rd_streaming = rrddim_add(st_memory, "streaming", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
            rd_buffers = rrddim_add(st_memory, "buffers", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
            rd_workers = rrddim_add(st_memory, "workers", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
            rd_aral = rrddim_add(st_memory, "aral", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
            rd_judy = rrddim_add(st_memory, "judy", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
            rd_slots = rrddim_add(st_memory, "slots", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
            rd_other = rrddim_add(st_memory, "other", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
        }

        // each of these should also be analyzed below at the buffers chart
        size_t buffers =
            netdata_buffers_statistics.query_targets_size + onewayalloc_allocated_memory() +
            netdata_buffers_statistics.rrdset_done_rda_size +
            netdata_buffers_statistics.buffers_aclk +
            netdata_buffers_statistics.buffers_api +
            netdata_buffers_statistics.buffers_functions +
            netdata_buffers_statistics.buffers_sqlite +
            netdata_buffers_statistics.buffers_exporters +
            netdata_buffers_statistics.buffers_health +
            netdata_buffers_statistics.buffers_streaming +
            netdata_buffers_statistics.cbuffers_streaming +
            netdata_buffers_statistics.buffers_web +
            replication_sender_allocated_buffers() +
            aral_by_size_free_bytes() +
            judy_aral_free_bytes() +
            uuidmap_free_bytes();

        sqlite3_int64 sqlite3_memory_used_current = 0, sqlite3_memory_used_highwater = 0;
        sqlite3_status64(SQLITE_STATUS_MEMORY_USED, &sqlite3_memory_used_current, &sqlite3_memory_used_highwater, 1);

        size_t strings_memory = 0, strings_index = 0;
        string_statistics(NULL, NULL, NULL, NULL, NULL, &strings_memory, &strings_index, NULL, NULL);

        rrddim_set_by_pointer(st_memory, rd_db_dbengine, (collected_number)pulse_dbengine_total_memory);
        rrddim_set_by_pointer(st_memory, rd_db_rrd, (collected_number)pulse_rrd_memory_size);
        rrddim_set_by_pointer(st_memory, rd_db_sqlite3, (collected_number)sqlite3_memory_used_highwater);

#ifdef DICT_WITH_STATS
        rrddim_set_by_pointer(st_memory, rd_collectors,
                              (collected_number)dictionary_stats_memory_total(dictionary_stats_category_collectors));

        rrddim_set_by_pointer(st_memory,rd_rrdhosts,
                              (collected_number)dictionary_stats_memory_total(dictionary_stats_category_rrdhost) + (collected_number)netdata_buffers_statistics.rrdhost_allocations_size);

        rrddim_set_by_pointer(st_memory, rd_rrdsets,
                              (collected_number)dictionary_stats_memory_total(dictionary_stats_category_rrdset));

        rrddim_set_by_pointer(st_memory, rd_rrddims,
                              (collected_number)dictionary_stats_memory_total(dictionary_stats_category_rrddim));

        rrddim_set_by_pointer(st_memory, rd_contexts,
                              (collected_number)dictionary_stats_memory_total(dictionary_stats_category_rrdcontext));

        rrddim_set_by_pointer(st_memory, rd_health,
                              (collected_number)dictionary_stats_memory_total(dictionary_stats_category_rrdhealth));

        rrddim_set_by_pointer(st_memory, rd_functions,
                              (collected_number)dictionary_stats_memory_total(dictionary_stats_category_functions));

        rrddim_set_by_pointer(st_memory, rd_replication,
                              (collected_number)dictionary_stats_memory_total(dictionary_stats_category_replication) + replication_sender_allocated_memory());
#else
        uint64_t metadata =
            aral_by_size_used_bytes() +
            dictionary_stats_category_rrdhost.memory.dict + dictionary_stats_category_rrdhost.memory.index +
            dictionary_stats_category_rrdset.memory.dict + dictionary_stats_category_rrdset.memory.index +
            dictionary_stats_category_rrddim.memory.dict + dictionary_stats_category_rrddim.memory.index +
            dictionary_stats_category_rrdcontext.memory.dict + dictionary_stats_category_rrdcontext.memory.index +
            dictionary_stats_category_rrdhealth.memory.dict + dictionary_stats_category_rrdhealth.memory.index +
            dictionary_stats_category_functions.memory.dict + dictionary_stats_category_functions.memory.index +
            dictionary_stats_category_replication.memory.dict + dictionary_stats_category_replication.memory.index +
            netdata_buffers_statistics.rrdhost_allocations_size +
            replication_sender_allocated_memory();

        rrddim_set_by_pointer(st_memory, rd_metadata, (collected_number)metadata);
#endif

        rrddim_set_by_pointer(st_memory, rd_uuid,
                              (collected_number)uuidmap_memory());

        // labels use dictionary like statistics, but it is not ARAL based dictionary
        rrddim_set_by_pointer(st_memory, rd_labels,
                              (collected_number)dictionary_stats_memory_total(dictionary_stats_category_rrdlabels));

        rrddim_set_by_pointer(st_memory, rd_ml,
                              (collected_number)pulse_ml_get_current_memory_usage());

        rrddim_set_by_pointer(st_memory, rd_strings,
                              (collected_number)(strings_memory + strings_index));

        rrddim_set_by_pointer(st_memory, rd_streaming,
                              (collected_number)netdata_buffers_statistics.rrdhost_senders + (collected_number)netdata_buffers_statistics.rrdhost_receivers);

        rrddim_set_by_pointer(st_memory, rd_buffers,
                              (collected_number)buffers);

        rrddim_set_by_pointer(st_memory, rd_workers,
                              (collected_number) workers_allocated_memory());

        rrddim_set_by_pointer(st_memory, rd_aral,
                              (collected_number)aral_by_size_structures_bytes());

        rrddim_set_by_pointer(st_memory, rd_judy,
                              (collected_number) judy_aral_structures());

        rrddim_set_by_pointer(st_memory, rd_slots,
                              (collected_number)__atomic_load_n(&rrd_slot_memory, __ATOMIC_RELAXED));

        rrddim_set_by_pointer(st_memory, rd_other,
                              (collected_number)dictionary_stats_memory_total(dictionary_stats_category_other));

        rrdset_done(st_memory);
    }

    // ----------------------------------------------------------------------------------------------------------------

    {
        static RRDSET *st_memory_buffers = NULL;
        static RRDDIM *rd_queries = NULL;
        static RRDDIM *rd_collectors = NULL;
        static RRDDIM *rd_buffers_aclk = NULL;
        static RRDDIM *rd_buffers_api = NULL;
        static RRDDIM *rd_buffers_functions = NULL;
        static RRDDIM *rd_buffers_sqlite = NULL;
        static RRDDIM *rd_buffers_exporters = NULL;
        static RRDDIM *rd_buffers_health = NULL;
        static RRDDIM *rd_buffers_streaming = NULL;
        static RRDDIM *rd_cbuffers_streaming = NULL;
        static RRDDIM *rd_buffers_replication = NULL;
        static RRDDIM *rd_buffers_web = NULL;
        static RRDDIM *rd_buffers_aral = NULL;
        static RRDDIM *rd_buffers_judy = NULL;
        static RRDDIM *rd_buffers_uuid = NULL;

        if (unlikely(!st_memory_buffers)) {
            st_memory_buffers = rrdset_create_localhost(
                "netdata",
                "memory_buffers",
                NULL,
                "Memory Usage",
                NULL,
                "Netdata Memory Buffers",
                "bytes",
                "netdata",
                "pulse",
                130102,
                localhost->rrd_update_every,
                RRDSET_TYPE_STACKED);

            rd_queries = rrddim_add(st_memory_buffers, "queries", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
            rd_collectors = rrddim_add(st_memory_buffers, "collection", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
            rd_buffers_aclk = rrddim_add(st_memory_buffers, "aclk", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
            rd_buffers_api = rrddim_add(st_memory_buffers, "api", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
            rd_buffers_functions = rrddim_add(st_memory_buffers, "functions", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
            rd_buffers_sqlite = rrddim_add(st_memory_buffers, "sqlite", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
            rd_buffers_exporters = rrddim_add(st_memory_buffers, "exporters", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
            rd_buffers_health = rrddim_add(st_memory_buffers, "health", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
            rd_buffers_streaming = rrddim_add(st_memory_buffers, "streaming", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
            rd_cbuffers_streaming = rrddim_add(st_memory_buffers, "streaming cbuf", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
            rd_buffers_replication = rrddim_add(st_memory_buffers, "replication", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
            rd_buffers_web = rrddim_add(st_memory_buffers, "web", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
            rd_buffers_aral = rrddim_add(st_memory_buffers, "aral-by-size free", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
            rd_buffers_judy = rrddim_add(st_memory_buffers, "aral-judy free", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
            rd_buffers_uuid = rrddim_add(st_memory_buffers, "uuid", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
        }

        // the sum of all these needs to be above at the total buffers calculation
        rrddim_set_by_pointer(st_memory_buffers, rd_queries, (collected_number)netdata_buffers_statistics.query_targets_size + (collected_number) onewayalloc_allocated_memory());
        rrddim_set_by_pointer(st_memory_buffers, rd_collectors, (collected_number)netdata_buffers_statistics.rrdset_done_rda_size);
        rrddim_set_by_pointer(st_memory_buffers, rd_buffers_aclk, (collected_number)netdata_buffers_statistics.buffers_aclk);
        rrddim_set_by_pointer(st_memory_buffers, rd_buffers_api, (collected_number)netdata_buffers_statistics.buffers_api);
        rrddim_set_by_pointer(st_memory_buffers, rd_buffers_functions, (collected_number)netdata_buffers_statistics.buffers_functions);
        rrddim_set_by_pointer(st_memory_buffers, rd_buffers_sqlite, (collected_number)netdata_buffers_statistics.buffers_sqlite);
        rrddim_set_by_pointer(st_memory_buffers, rd_buffers_exporters, (collected_number)netdata_buffers_statistics.buffers_exporters);
        rrddim_set_by_pointer(st_memory_buffers, rd_buffers_health, (collected_number)netdata_buffers_statistics.buffers_health);
        rrddim_set_by_pointer(st_memory_buffers, rd_buffers_streaming, (collected_number)netdata_buffers_statistics.buffers_streaming);
        rrddim_set_by_pointer(st_memory_buffers, rd_cbuffers_streaming, (collected_number)netdata_buffers_statistics.cbuffers_streaming);
        rrddim_set_by_pointer(st_memory_buffers, rd_buffers_web, (collected_number)netdata_buffers_statistics.buffers_web);
        rrddim_set_by_pointer(st_memory_buffers, rd_buffers_replication, (collected_number)replication_sender_allocated_buffers());
        rrddim_set_by_pointer(st_memory_buffers, rd_buffers_aral, (collected_number)aral_by_size_free_bytes());
        rrddim_set_by_pointer(st_memory_buffers, rd_buffers_judy, (collected_number)judy_aral_free_bytes());
        rrddim_set_by_pointer(st_memory_buffers, rd_buffers_uuid, (collected_number)uuidmap_free_bytes());

        rrdset_done(st_memory_buffers);
    }

    // ----------------------------------------------------------------------------------------------------------------

#ifdef ENABLE_DBENGINE
    OS_SYSTEM_MEMORY sm = os_system_memory(true);
    if (OS_SYSTEM_MEMORY_OK(sm) && dbengine_out_of_memory_protection) {
        static RRDSET *st_memory_available = NULL;
        static RRDDIM *rd_available = NULL;

        if (unlikely(!st_memory_available)) {
            st_memory_available = rrdset_create_localhost(
                "netdata",
                "out_of_memory_protection",
                NULL,
                "Memory Usage",
                NULL,
                "Out of Memory Protection",
                "bytes",
                "netdata",
                "pulse",
                130103,
                localhost->rrd_update_every,
                RRDSET_TYPE_AREA);

            rd_available = rrddim_add(st_memory_available, "available", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
        }

        // the sum of all these needs to be above at the total buffers calculation
        rrddim_set_by_pointer(
            st_memory_available, rd_available,
            (collected_number)sm.ram_available_bytes);

        rrdset_done(st_memory_available);
    }
#endif
    // ----------------------------------------------------------------------------------------------------------------
}
