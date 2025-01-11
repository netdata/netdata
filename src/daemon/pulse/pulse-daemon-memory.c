// SPDX-License-Identifier: GPL-3.0-or-later

#define PULSE_INTERNALS 1
#include "pulse-daemon-memory.h"
#include "streaming/stream-replication-sender.h"

#define dictionary_stats_memory_total(stats) \
    ((stats).memory.dict + (stats).memory.values + (stats).memory.index)

struct netdata_buffers_statistics netdata_buffers_statistics = { 0 };

#ifdef HAVE_C_MALLOC_INFO
#include <malloc.h>

// Helper function to find the last occurrence of a substring in a string
static char *find_last(const char *haystack, const char *needle, size_t *found) {
    *found = 0;

    char *last = NULL;
    char *current = strstr(haystack, needle);
    while (current) {
        (*found)++;
        last = current;
        current = strstr(current + 1, needle);
    }
    return last;
}

static bool parse_malloc_info(size_t *arenas, size_t *allocated_memory, size_t *used_fast, size_t *used_rest, size_t *used_mmap, size_t *unused_memory) {
    int found = 0;
    
    *arenas = 0;
    *allocated_memory = 0;
    *used_fast = 0;
    *used_rest = 0;
    *used_mmap = 0;
    *unused_memory = 0;

    // Buffer for malloc_info XML
    char *buffer = NULL;
    size_t size = 0;
    FILE *meminfo = open_memstream(&buffer, &size);
    if (!meminfo)
        goto cleanup;

    char *t = malloc(1024);

    // Generate malloc_info XML
    if(malloc_info(0, meminfo) != 0)
        goto cleanup;

    fflush(meminfo);
    fclose(meminfo);
    meminfo = NULL;

    free(t);

    if(!size || !buffer)
        goto cleanup;

    // make sure it is terminated
    buffer[size - 1] = '\0';

    // Find the last </heap>
    char *last_heap_end = find_last(buffer, "</heap>", arenas);
    if (!last_heap_end)
        goto cleanup;

    // Move past the last </heap>
    char *summary_section = last_heap_end + strlen("</heap>");

    // Parse the summary section using strstr
    const char *fast_key = "<total type=\"fast\"";
    const char *rest_key = "<total type=\"rest\"";
    const char *mmap_key = "<total type=\"mmap\"";
    const char *system_key = "<system type=\"current\"";
    char *size_pos;

    // Extract Used Fast
    char *fast_pos = strstr(summary_section, fast_key);
    if(!fast_pos || !(size_pos = strstr(fast_pos, "size=\"")))
        goto cleanup;

    *used_fast = strtoull(size_pos + 6, NULL, 10);
    found++;

    // Extract Used Rest
    char *rest_pos = strstr(summary_section, rest_key);
    if (!rest_pos || !(size_pos = strstr(rest_pos, "size=\"")))
        goto cleanup;

    *used_rest = strtoull(size_pos + 6, NULL, 10);
    found++;

    // Extract Used Mmap
    char *mmap_pos = strstr(summary_section, mmap_key);
    if (!mmap_pos || !(size_pos = strstr(mmap_pos, "size=\"")))
        goto cleanup;

    *used_mmap = strtoull(size_pos + 6, NULL, 10);
    found++;

    // Extract Allocated Memory
    char *system_pos = strstr(summary_section, system_key);
    if (!system_pos || !(size_pos = strstr(system_pos, "size=\"")))
        goto cleanup;

    *allocated_memory = strtoull(size_pos + 6, NULL, 10);
    found++;

    // Calculate Unused Memory
    *unused_memory = *allocated_memory > (*used_fast + *used_rest + *used_mmap) ?
                         *allocated_memory - (*used_fast + *used_rest + *used_mmap) : 0;

cleanup:
    if(meminfo) fclose(meminfo);
    if(buffer) free(buffer);
    return found == 4;
}
#endif // HAVE_C_MALLOC_INFO

void pulse_daemon_memory_do(bool extended) {
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
        static RRDDIM *rd_labels = NULL; // labels use dictionary like statistics, but it is not ARAL based dictionary
        static RRDDIM *rd_ml = NULL;
        static RRDDIM *rd_strings = NULL;
        static RRDDIM *rd_streaming = NULL;
        static RRDDIM *rd_buffers = NULL;
        static RRDDIM *rd_workers = NULL;
        static RRDDIM *rd_aral = NULL;
        static RRDDIM *rd_judy = NULL;
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
            rd_labels = rrddim_add(st_memory, "labels", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
            rd_ml = rrddim_add(st_memory, "ML", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
            rd_strings = rrddim_add(st_memory, "strings", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
            rd_streaming = rrddim_add(st_memory, "streaming", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
            rd_buffers = rrddim_add(st_memory, "buffers", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
            rd_workers = rrddim_add(st_memory, "workers", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
            rd_aral = rrddim_add(st_memory, "aral", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
            rd_judy = rrddim_add(st_memory, "judy", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
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
                         replication_sender_allocated_buffers() + aral_by_size_free_bytes() + judy_aral_free_bytes();

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
                              (collected_number)dictionary_stats_memory_total(dictionary_stats_category_replication) + (collected_number)replication_sender_allocated_memory());
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

        rrddim_set_by_pointer(st_memory, rd_other,
                              (collected_number)dictionary_stats_memory_total(dictionary_stats_category_other));

        rrdset_done(st_memory);
    }

    // ----------------------------------------------------------------------------------------------------------------

    OS_SYSTEM_MEMORY sm = os_system_memory(true);
    if (sm.ram_total_bytes && dbengine_out_of_memory_protection) {
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
                130102,
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

    // ----------------------------------------------------------------------------------------------------------------

    if(!extended)
        return;

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
                130103,
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

        rrdset_done(st_memory_buffers);
    }

    // ----------------------------------------------------------------------------------------------------------------

#ifdef HAVE_C_MALLOC_INFO
    size_t glibc_arenas, glibc_allocated_memory, glibc_used_fast, glibc_used_rest, glibc_used_mmap, glibc_unused_memory;
    if(parse_malloc_info(&glibc_arenas, &glibc_allocated_memory, &glibc_used_fast, &glibc_used_rest, &glibc_used_mmap, &glibc_unused_memory)) {
        if (glibc_arenas) {
            static RRDSET *st_arenas = NULL;
            static RRDDIM *rd_arenas = NULL;

            if (unlikely(!st_arenas)) {
                st_arenas = rrdset_create_localhost(
                    "netdata",
                    "glibc_arenas",
                    NULL,
                    "Memory Usage",
                    NULL,
                    "Glibc Memory Arenas",
                    "bytes",
                    "netdata",
                    "pulse",
                    130104,
                    localhost->rrd_update_every,
                    RRDSET_TYPE_STACKED);

                rd_arenas = rrddim_add(st_arenas, "arenas", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
            }

            // the sum of all these needs to be above at the total buffers calculation
            rrddim_set_by_pointer(st_arenas, rd_arenas, (collected_number)glibc_arenas);
            rrdset_done(st_arenas);
        }

        if (glibc_allocated_memory > 0 && glibc_used_fast + glibc_used_rest + glibc_used_mmap + glibc_unused_memory > 0) {
            static RRDSET *st_malloc = NULL;
            static RRDDIM *rd_unused = NULL;
            static RRDDIM *rd_fast = NULL;
            static RRDDIM *rd_rest = NULL;
            static RRDDIM *rd_mmap = NULL;

            if (unlikely(!st_malloc)) {
                st_malloc = rrdset_create_localhost(
                    "netdata",
                    "glibc_memory",
                    NULL,
                    "Memory Usage",
                    NULL,
                    "Glibc Memory Usage",
                    "bytes",
                    "netdata",
                    "pulse",
                    130105,
                    localhost->rrd_update_every,
                    RRDSET_TYPE_STACKED);

                rd_unused = rrddim_add(st_malloc, "unused", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
                rd_fast = rrddim_add(st_malloc, "fast", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
                rd_rest = rrddim_add(st_malloc, "rest", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
                rd_mmap = rrddim_add(st_malloc, "mmap", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
            }

            // the sum of all these needs to be above at the total buffers calculation
            rrddim_set_by_pointer(st_malloc, rd_unused, (collected_number)glibc_unused_memory);
            rrddim_set_by_pointer(st_malloc, rd_fast, (collected_number)glibc_used_fast);
            rrddim_set_by_pointer(st_malloc, rd_rest, (collected_number)glibc_used_rest);
            rrddim_set_by_pointer(st_malloc, rd_mmap, (collected_number)glibc_used_mmap);
            rrdset_done(st_malloc);
        }
    }
#endif

}
