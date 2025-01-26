// SPDX-License-Identifier: GPL-3.0-or-later

#define PULSE_INTERNALS 1
#include "pulse-daemon-memory.h"
#include "streaming/stream-replication-sender.h"

#if defined(HAVE_C_MALLOC_INFO) || defined(HAVE_C_MALLINFO2)
#include <malloc.h>
#endif

#ifdef HAVE_C_MALLOC_INFO
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

static bool parse_malloc_info(size_t *arenas, size_t *allocated_arena, size_t *unused_fast, size_t *unused_rest, size_t *allocated_mmap) {
    int found = 0;

    *arenas = 0;
    *allocated_arena = 0;
    *unused_fast = 0;
    *unused_rest = 0;
    *allocated_mmap = 0;

    // Buffer for malloc_info XML
    char *buffer = NULL;
    size_t size = 0;
    FILE *meminfo = open_memstream(&buffer, &size);
    if (!meminfo)
        goto cleanup;

    char *t = malloc(1024);

    // Generate malloc_info XML
    if(malloc_info(0, meminfo) != 0) {
        free(t);
        goto cleanup;
    }

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

    char *fast_pos = strstr(summary_section, fast_key);
    if(!fast_pos || !(size_pos = strstr(fast_pos, "size=\"")))
        goto cleanup;

    *unused_fast = strtoull(size_pos + 6, NULL, 10);
    found++;

    char *rest_pos = strstr(summary_section, rest_key);
    if (!rest_pos || !(size_pos = strstr(rest_pos, "size=\"")))
        goto cleanup;

    *unused_rest = strtoull(size_pos + 6, NULL, 10);
    found++;

    char *mmap_pos = strstr(summary_section, mmap_key);
    if (!mmap_pos || !(size_pos = strstr(mmap_pos, "size=\"")))
        goto cleanup;

    *allocated_mmap = strtoull(size_pos + 6, NULL, 10);
    found++;

    char *system_pos = strstr(summary_section, system_key);
    if (!system_pos || !(size_pos = strstr(system_pos, "size=\"")))
        goto cleanup;

    *allocated_arena = strtoull(size_pos + 6, NULL, 10);
    found++;

cleanup:
    if(meminfo) fclose(meminfo);
    if(buffer) free(buffer);
    return found == 4;
}
#endif // HAVE_C_MALLOC_INFO

void pulse_daemon_memory_system_do(bool extended) {
    if(!extended) return;

    size_t glibc_mmaps = 0;
    bool have_mallinfo = false;

#ifdef HAVE_C_MALLINFO2
    struct mallinfo2 mi = mallinfo2();
    glibc_mmaps = mi.hblks;
    if(mi.hblkhd || mi.fordblks) {
        static RRDSET *st_mallinfo = NULL;
        static RRDDIM *rd_used_mmap = NULL;
        static RRDDIM *rd_used_arena = NULL;
        static RRDDIM *rd_unused_fragments = NULL;
        static RRDDIM *rd_unused_releasable = NULL;

        if (unlikely(!st_mallinfo)) {
            st_mallinfo = rrdset_create_localhost(
                "netdata",
                "glibc_mallinfo2",
                NULL,
                "Memory Usage",
                NULL,
                "Glibc Mallinfo2 Memory Distribution",
                "bytes",
                "netdata",
                "pulse",
                130130,
                localhost->rrd_update_every,
                RRDSET_TYPE_STACKED);

            rd_unused_releasable = rrddim_add(st_mallinfo, "unused releasable", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
            rd_unused_fragments = rrddim_add(st_mallinfo, "unused fragments", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
            rd_used_arena = rrddim_add(st_mallinfo, "used arena", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
            rd_used_mmap = rrddim_add(st_mallinfo, "used mmap", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
        }

        // size_t total = mi.uordblks;
        size_t used_mmap = mi.hblkhd;
        size_t used_arena = (mi.arena > mi.fordblks) ? mi.arena - mi.fordblks : 0;

        size_t unused_total = mi.fordblks;
        size_t unused_releasable = mi.keepcost;
        // size_t unused_fast = mi.fsmblks;
        size_t unused_fragments = (unused_total > unused_releasable) ? unused_total - unused_releasable : 0;

        rrddim_set_by_pointer(st_mallinfo, rd_unused_releasable, (collected_number)unused_releasable);
        rrddim_set_by_pointer(st_mallinfo, rd_unused_fragments, (collected_number)unused_fragments);
        rrddim_set_by_pointer(st_mallinfo, rd_used_arena, (collected_number)used_arena);
        rrddim_set_by_pointer(st_mallinfo, rd_used_mmap, (collected_number)used_mmap);

        rrdset_done(st_mallinfo);
        have_mallinfo = true;
    }
#endif // HAVE_C_MALLINFO2

#ifdef HAVE_C_MALLOC_INFO
    size_t glibc_arenas, glibc_allocated_arenas, glibc_unused_fast, glibc_unused_rest, glibc_allocated_mmap;
    if(!have_mallinfo && parse_malloc_info(&glibc_arenas, &glibc_allocated_arenas, &glibc_unused_fast, &glibc_unused_rest, &glibc_allocated_mmap)) {
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
                    "arenas",
                    "netdata",
                    "pulse",
                    130120,
                    localhost->rrd_update_every,
                    RRDSET_TYPE_LINE);

                rd_arenas = rrddim_add(st_arenas, "arenas", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
            }

            rrddim_set_by_pointer(st_arenas, rd_arenas, (collected_number)glibc_arenas);
            rrdset_done(st_arenas);
        }

        if (glibc_allocated_arenas || glibc_allocated_mmap) {
            static RRDSET *st_malloc = NULL;
            static RRDDIM *rd_unused_fast = NULL;
            static RRDDIM *rd_unused_rest = NULL;
            static RRDDIM *rd_used_arena = NULL;
            static RRDDIM *rd_used_mmap = NULL;

            if (unlikely(!st_malloc)) {
                st_malloc = rrdset_create_localhost(
                    "netdata",
                    "glibc_malloc_info",
                    NULL,
                    "Memory Usage",
                    NULL,
                    "Glibc Malloc Info",
                    "bytes",
                    "netdata",
                    "pulse",
                    130121,
                    localhost->rrd_update_every,
                    RRDSET_TYPE_STACKED);

                rd_unused_fast = rrddim_add(st_malloc, "unused fast", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
                rd_unused_rest = rrddim_add(st_malloc, "unused rest", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
                rd_used_arena = rrddim_add(st_malloc, "used arena", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
                rd_used_mmap = rrddim_add(st_malloc, "used mmap", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
            }

            size_t unused = glibc_unused_fast + glibc_unused_rest;
            size_t used_arena = (glibc_allocated_arenas > unused) ? glibc_allocated_arenas - unused : 0;

            rrddim_set_by_pointer(st_malloc, rd_unused_fast, (collected_number)glibc_unused_fast);
            rrddim_set_by_pointer(st_malloc, rd_unused_rest, (collected_number)glibc_unused_rest);
            rrddim_set_by_pointer(st_malloc, rd_used_arena, (collected_number)used_arena);
            rrddim_set_by_pointer(st_malloc, rd_used_mmap, (collected_number)glibc_allocated_mmap);
            rrdset_done(st_malloc);
        }
    }
#endif

    size_t netdata_mmaps = __atomic_load_n(&nd_mmap_count, __ATOMIC_RELAXED);
    size_t total_mmaps = netdata_mmaps + glibc_mmaps;
    {
        static RRDSET *st_maps = NULL;
        static RRDDIM *rd_netdata = NULL;
        static RRDDIM *rd_glibc = NULL;

        if (unlikely(!st_maps)) {
            st_maps = rrdset_create_localhost(
                "netdata",
                "memory_maps",
                NULL,
                "Memory Usage",
                NULL,
                "Netdata Memory Maps",
                "maps",
                "netdata",
                "pulse",
                130105,
                localhost->rrd_update_every,
                RRDSET_TYPE_LINE);

            rd_netdata = rrddim_add(st_maps, "netdata", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
            rd_glibc = rrddim_add(st_maps, "glibc", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
        }

        rrddim_set_by_pointer(st_maps, rd_glibc, (collected_number)glibc_mmaps);
        rrddim_set_by_pointer(st_maps, rd_netdata, (collected_number)netdata_mmaps);

        rrdset_done(st_maps);
    }

    static unsigned long long max_map_count = 0;
    static usec_t last_read_ut = 0;
    usec_t now_ut = now_monotonic_usec();
    if(now_ut - last_read_ut >= 60 * USEC_PER_SEC) {
        // read this file once per minute
        read_single_number_file("/proc/sys/vm/max_map_count", &max_map_count);
        last_read_ut = now_ut;
    }

    if (max_map_count && total_mmaps) {
        static RRDSET *st_maps_percent = NULL;
        static RRDDIM *rd_used = NULL;

        if (unlikely(!st_maps_percent)) {
            st_maps_percent = rrdset_create_localhost(
                "netdata",
                "memory_maps_limit",
                NULL,
                "Memory Usage",
                NULL,
                "Netdata Memory Maps Limit",
                "%",
                "netdata",
                "pulse",
                130106,
                localhost->rrd_update_every,
                RRDSET_TYPE_AREA);

            rd_used = rrddim_add(st_maps_percent, "used", NULL, 1, 1000, RRD_ALGORITHM_ABSOLUTE);
        }

        double percent = (double)total_mmaps * 100.0 / (double)max_map_count;

        rrddim_set_by_pointer(st_maps_percent, rd_used, (collected_number)round(percent * 1000.0));
        rrdset_done(st_maps_percent);
    }
}
