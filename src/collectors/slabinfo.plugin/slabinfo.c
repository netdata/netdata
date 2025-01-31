// SPDX-License-Identifier: GPL-3.0-or-later

#include "database/rrd.h"
#include "libnetdata/required_dummies.h"

#define PLUGIN_SLABINFO_NAME "slabinfo.plugin"
#define PLUGIN_SLABINFO_PROCFILE "/proc/slabinfo"

#define CHART_TYPE "mem"
#define CHART_FAMILY "slab"
#define CHART_PRIO 3000

// #define slabdebug(...) if (debug) { fprintf(stderr, __VA_ARGS__); }
#define slabdebug(args...) do {     \
    if (debug) {                    \
        fprintf(stderr, "slabinfo.plugin DEBUG (%04d@%-10.10s:%-15.15s)::", __LINE__, __FILE__, __FUNCTION__); \
        fprintf(stderr, ##args);    \
        fprintf(stderr, "\n");      \
    }                               \
} while(0)

int running = 1;
int debug = 0;
size_t lines_discovered = 0;
int redraw_chart = 0;

// ----------------------------------------------------------------------------

// Slabinfo format :
// format 2.1 Was provided by 57ed3eda977a215f054102b460ab0eb5d8d112e6 (2.6.24-rc6) as:
// seq_puts(m, "# name  <active_objs> <num_objs> <objsize> <objperslab> <pagesperslab>");
// seq_puts(m, " : tunables <limit> <batchcount> <sharedfactor>");
// seq_puts(m, " : slabdata <active_slabs> <num_slabs> <sharedavail>");
//
// With max values:
// seq_printf(m, "%-17s %6lu %6lu %6u %4u %4d",
//   cache_name(s), sinfo.active_objs, sinfo.num_objs, s->size, sinfo.objects_per_slab, (1 << sinfo.cache_order));
// seq_printf(m, " : tunables %4u %4u %4u",
//   sinfo.limit, sinfo.batchcount, sinfo.shared);
// seq_printf(m, " : slabdata %6lu %6lu %6lu",
//   sinfo.active_slabs, sinfo.num_slabs, sinfo.shared_avail);
//
// If CONFIG_DEBUG_SLAB is set, it will also add columns from slabinfo_show_stats (for SLAB only):
// seq_printf(m, " : globalstat %7lu %6lu %5lu %4lu %4lu %4lu %4lu %4lu %4lu",
//   allocs, high, grown, reaped, errors, max_freeable, node_allocs, node_frees, overflows);
// seq_printf(m, " : cpustat %6lu %6lu %6lu %6lu",
//   allochit, allocmiss, freehit, freemiss);
//
// Implementation choices:
// - Iterates through a linked list of kmem_cache.
// - Name is a char* from struct kmem_cache (mm/slab.h).
// - max name size found is 24:
//     grep -roP 'kmem_cache_create\(".+"'| awk '{split($0,a,"\""); print a[2],length(a[2]); }' | sort -k2 -n
// - Using uint64 everywhere, as types fits and allows to use standard helpers

struct slabinfo {
    // procfile fields
    const char *name;
    uint64_t active_objs;
    uint64_t num_objs;
    uint64_t obj_size;
    uint64_t obj_per_slab;
    uint64_t pages_per_slab;
    uint64_t tune_limit;
    uint64_t tune_batchcnt;
    uint64_t tune_shared_factor;
    uint64_t data_active_slabs;
    uint64_t data_num_slabs;
    uint64_t data_shared_avail;

    // Calculated fields
    uint64_t mem_usage;
    uint64_t mem_waste;
    uint8_t  obj_filling;

    uint32_t hash;
    struct slabinfo *next;
} *slabinfo_root = NULL, *slabinfo_next = NULL, *slabinfo_last_used = NULL;

// The code is very inspired from "proc_net_dev.c" and "perf_plugin.c"

// Get the existing object, or create a new one
static struct slabinfo *get_slabstruct(const char *name) {
    struct slabinfo *s;

    slabdebug("--> Requested slabstruct %s", name);

    uint32_t hash = simple_hash(name);

    // Search it, from the next to the end
    for (s = slabinfo_next; s; s = s->next) {
        if ((hash = s->hash) && !strcmp(name, s->name)) {
            slabdebug("<-- Found existing slabstruct after %s", slabinfo_last_used->name);
            // Prepare the next run
            slabinfo_next = s->next;
            slabinfo_last_used = s;
            return s;
        }
    }

    // Search it from the beginning to the last position we used
    for (s = slabinfo_root; s != slabinfo_last_used; s = s->next) {
        if (hash == s->hash && !strcmp(name, s->name)) {
            slabdebug("<-- Found existing slabstruct after root %s", slabinfo_root->name);
            slabinfo_next = s->next;
            slabinfo_last_used = s;
            return s;
        }
    }

    // Create a new one
    s = callocz(1, sizeof(struct slabinfo));
    s->name = strdupz(name);
    s->hash = hash;

    // Add it to the current position
    if (slabinfo_root) {
        slabdebug("<-- Creating new slabstruct after %s", slabinfo_last_used->name);
        s->next = slabinfo_last_used->next;
        slabinfo_last_used->next = s;
        slabinfo_last_used = s;
    }
    else {
        slabdebug("<-- Creating new slabstruct as root");
        slabinfo_root = slabinfo_last_used = s;
    }

    return s;
}


// Read a full pass of slabinfo to update the structs
struct slabinfo *read_file_slabinfo() {

    slabdebug("-> Reading procfile %s", PLUGIN_SLABINFO_PROCFILE);

    static procfile *ff = NULL;
    static long slab_pagesize = 0;

    if (unlikely(!slab_pagesize)) {
        slab_pagesize = sysconf(_SC_PAGESIZE);
        slabdebug("   Discovered pagesize: %ld", slab_pagesize);
    }

    if(unlikely(!ff)) {
        ff = procfile_reopen(ff, PLUGIN_SLABINFO_PROCFILE, " ,:" , PROCFILE_FLAG_DEFAULT);
        if(unlikely(!ff)) {
            collector_error("<- Cannot open file '%s", PLUGIN_SLABINFO_PROCFILE);
            exit(1);
        }
    }

    ff = procfile_readall(ff);
    if(unlikely(!ff)) {
        collector_error("<- Cannot read file '%s'", PLUGIN_SLABINFO_PROCFILE);
        exit(0);
    }


    // Iterate on all lines to populate / update the slabinfo struct
    size_t lines = procfile_lines(ff), l;
    if (unlikely(lines != lines_discovered)) {
        lines_discovered = lines;
        redraw_chart = 1;
    }

    slabdebug("   Read %lu lines from procfile", (unsigned long)lines);
    for(l = 2; l < lines; l++) {
        if (unlikely(procfile_linewords(ff, l) < 14)) {
            slabdebug("    Line %zu has only %zu words, skipping", l, (size_t)procfile_linewords(ff,l));
            continue;
        }

        char *name = procfile_lineword(ff, l, 0);
        struct slabinfo *s = get_slabstruct(name);

        s->active_objs    = str2uint64_t(procfile_lineword(ff, l, 1), NULL);
        s->num_objs       = str2uint64_t(procfile_lineword(ff, l, 2), NULL);
        s->obj_size       = str2uint64_t(procfile_lineword(ff, l, 3), NULL);
        s->obj_per_slab   = str2uint64_t(procfile_lineword(ff, l, 4), NULL);
        s->pages_per_slab = str2uint64_t(procfile_lineword(ff, l, 5), NULL);

        s->tune_limit     = str2uint64_t(procfile_lineword(ff, l, 7), NULL);
        s->tune_batchcnt  = str2uint64_t(procfile_lineword(ff, l, 8), NULL);
        s->tune_shared_factor = str2uint64_t(procfile_lineword(ff, l, 9), NULL);

        s->data_active_slabs = str2uint64_t(procfile_lineword(ff, l, 11), NULL);
        s->data_num_slabs    = str2uint64_t(procfile_lineword(ff, l, 12), NULL);
        s->data_shared_avail = str2uint64_t(procfile_lineword(ff, l, 13), NULL);

        uint32_t memperslab = s->pages_per_slab * slab_pagesize;
        // Internal fragmentation: loss per slab, due to objects not being a multiple of pagesize
        //uint32_t lossperslab = memperslab - s->obj_per_slab * s->obj_size;

        // Total usage = slabs * pages per slab * page size
        s->mem_usage = (uint64_t)(s->data_num_slabs * memperslab);

        // Wasted memory (filling): slabs allocated but not filled: sum total slab - sum total objects
        s->mem_waste = s->mem_usage - (uint64_t)(s->active_objs * s->obj_size);
        //if (s->data_num_slabs > 1)
        //    s->mem_waste += s->data_num_slabs * lossperslab;


        // Slab filling efficiency
        if (s->num_objs > 0)
            s->obj_filling = 100 * s->active_objs / s->num_objs;
        else
            s->obj_filling = 0;

        slabdebug("    Updated slab %s: %"PRIu64" %"PRIu64" %"PRIu64" %"PRIu64" %"PRIu64" / %"PRIu64" %"PRIu64" %"PRIu64" / %"PRIu64" %"PRIu64" %"PRIu64" / %"PRIu64" %"PRIu64" %hhu",
            name, s->active_objs, s->num_objs, s->obj_size, s->obj_per_slab, s->pages_per_slab,
            s->tune_limit, s->tune_batchcnt, s->tune_shared_factor,
            s->data_active_slabs, s->data_num_slabs, s->data_shared_avail,
            s->mem_usage, s->mem_waste, s->obj_filling);
    }

    return slabinfo_root;
}



unsigned int do_slab_stats(int update_every) {

    static unsigned int loops = 0;
    struct slabinfo *sactive = NULL, *s = NULL;

    // Main processing loop
    while (running) {

        sactive = read_file_slabinfo();

        // Init Charts
        if (unlikely(redraw_chart)) {
            redraw_chart = 0;
            // Memory Usage
            printf("CHART %s.%s '' 'Memory Usage' 'B' '%s' '' line %d %d %s\n"
                , CHART_TYPE
                , "slabmemory"
                , CHART_FAMILY
                , CHART_PRIO
                , update_every
                , PLUGIN_SLABINFO_NAME
            );
            for (s = sactive; s; s = s->next) {
                printf("DIMENSION %s '' absolute 1 1\n", s->name);
            }

            // Slab active usage (filling)
            printf("CHART %s.%s '' 'Object Filling' '%%' '%s' '' line %d %d %s\n"
                , CHART_TYPE
                , "slabfilling"
                , CHART_FAMILY
                , CHART_PRIO + 1
                , update_every
                , PLUGIN_SLABINFO_NAME
            );
            for (s = sactive; s; s = s->next) {
                printf("DIMENSION %s '' absolute 1 1\n", s->name);
            }

            // Memory waste
            printf("CHART %s.%s '' 'Memory waste' 'B' '%s' '' line %d %d %s\n"
                , CHART_TYPE
                , "slabwaste"
                , CHART_FAMILY
                , CHART_PRIO + 2
                , update_every
                , PLUGIN_SLABINFO_NAME
            );
            for (s = sactive; s; s = s->next) {
                printf("DIMENSION %s '' absolute 1 1\n", s->name);
            }
        }


        //
        // Memory usage
        //
        printf("BEGIN %s.%s\n"
            , CHART_TYPE
            , "slabmemory"
        );
        for (s = sactive; s; s = s->next) {
            printf("SET %s = %"PRIu64"\n"
                , s->name
                , s->mem_usage
            );
        }
        printf("END\n");

        //
        // Slab active usage
        //
        printf("BEGIN %s.%s\n"
            , CHART_TYPE
            , "slabfilling"
        );
        for (s = sactive; s; s = s->next) {
            printf("SET %s = %u\n"
                , s->name
                , s->obj_filling
            );
        }
        printf("END\n");

        //
        // Memory waste
        //
        printf("BEGIN %s.%s\n"
            , CHART_TYPE
            , "slabwaste"
        );
        for (s = sactive; s; s = s->next) {
            printf("SET %s = %"PRIu64"\n"
                , s->name
                , s->mem_waste
            );
        }
        printf("END\n");

        fprintf(stdout, "\n");
        fflush(stdout);
        if (ferror(stdout) && errno == EPIPE) {
            netdata_log_error("error writing to stdout: EPIPE. Exiting...");
            return loops;
        }

        loops++;

        sleep(update_every);
    }

    return loops;
}




// ----------------------------------------------------------------------------
// main

void usage(void) {
    fprintf(stderr, "%s\n", program_name);
    exit(1);
}

int main(int argc, char **argv) {
    nd_log_initialize_for_external_plugins("slabinfo.plugin");
    netdata_threads_init_for_external_plugins(0);

    program_name = argv[0];
    int update_every = 1, i, n, freq = 0;

    for (i = 1; i < argc; i++) {
        // Frequency parsing
        if(isdigit(*argv[i]) && !freq) {
            n = (int) str2l(argv[i]);
            if (n > 0) {
                if (n >= UPDATE_EVERY_MAX) {
                    collector_error("Invalid interval value: %s", argv[i]);
                    exit(1);
                }
                freq = n;
            }
        }
        else if (strcmp("debug", argv[i]) == 0) {
            debug = 1;
            continue;
        }
        else {
            fprintf(stderr,
                "netdata slabinfo.plugin %s\n"
                "This program is a data collector plugin for netdata.\n"
                "\n"
                "Available command line options:\n"
                "\n"
                "  COLLECTION_FREQUENCY    data collection frequency in seconds\n"
                "                          minimum: %d\n"
                "\n"
                "  debug                   enable verbose output\n"
                "                          default: disabled\n"
                "\n",
                NETDATA_VERSION,
                update_every
            );
            exit(1);
        }
    }

    if(freq >= update_every)
        update_every = freq;
    else if(freq)
        collector_error("update frequency %d seconds is too small for slabinfo. Using %d.", freq, update_every);


    // Call the main function. Time drift to be added
    do_slab_stats(update_every);

    return 0;
}
