// SPDX-License-Identifier: GPL-3.0-or-later

#define PULSE_INTERNALS 1
#include "pulse-string.h"

void pulse_string_do(bool extended) {
    if(!extended) return;

    static RRDSET *st_ops = NULL, *st_entries = NULL, *st_mem = NULL;
    static RRDDIM *rd_ops_inserts = NULL, *rd_ops_deletes = NULL;
    static RRDDIM *rd_entries_entries = NULL;
    static RRDDIM *rd_mem = NULL;
    static RRDDIM *rd_mem_idx = NULL;
#ifdef NETDATA_INTERNAL_CHECKS
    static RRDDIM *rd_entries_refs = NULL, *rd_ops_releases = NULL,  *rd_ops_duplications = NULL, *rd_ops_searches = NULL;
#endif

    size_t inserts, deletes, searches, entries, references, memory, memory_index, duplications, releases;

    string_statistics(&inserts, &deletes, &searches, &entries, &references, &memory, &memory_index, &duplications, &releases);

    if (unlikely(!st_ops)) {
        st_ops = rrdset_create_localhost(
            "netdata"
            , "strings_ops"
            , NULL
            , "strings"
            , NULL
            , "Strings operations"
            , "ops/s"
            , "netdata"
            , "pulse"
            , 910000
            , localhost->rrd_update_every
            , RRDSET_TYPE_LINE);

        rd_ops_inserts      = rrddim_add(st_ops, "inserts",      NULL,  1, 1, RRD_ALGORITHM_INCREMENTAL);
        rd_ops_deletes      = rrddim_add(st_ops, "deletes",      NULL, -1, 1, RRD_ALGORITHM_INCREMENTAL);
#ifdef NETDATA_INTERNAL_CHECKS
        rd_ops_searches     = rrddim_add(st_ops, "searches",     NULL,  1, 1, RRD_ALGORITHM_INCREMENTAL);
        rd_ops_duplications = rrddim_add(st_ops, "duplications", NULL,  1, 1, RRD_ALGORITHM_INCREMENTAL);
        rd_ops_releases     = rrddim_add(st_ops, "releases",     NULL, -1, 1, RRD_ALGORITHM_INCREMENTAL);
#endif
    }

    rrddim_set_by_pointer(st_ops, rd_ops_inserts,      (collected_number)inserts);
    rrddim_set_by_pointer(st_ops, rd_ops_deletes,      (collected_number)deletes);
#ifdef NETDATA_INTERNAL_CHECKS
    rrddim_set_by_pointer(st_ops, rd_ops_searches,     (collected_number)searches);
    rrddim_set_by_pointer(st_ops, rd_ops_duplications, (collected_number)duplications);
    rrddim_set_by_pointer(st_ops, rd_ops_releases,     (collected_number)releases);
#endif
    rrdset_done(st_ops);

    if (unlikely(!st_entries)) {
        st_entries = rrdset_create_localhost(
            "netdata"
            , "strings_entries"
            , NULL
            , "strings"
            , NULL
            , "Strings entries"
            , "entries"
            , "netdata"
            , "pulse"
            , 910001
            , localhost->rrd_update_every
            , RRDSET_TYPE_AREA);

        rd_entries_entries  = rrddim_add(st_entries, "entries", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
#ifdef NETDATA_INTERNAL_CHECKS
        rd_entries_refs  = rrddim_add(st_entries, "references", NULL, 1, -1, RRD_ALGORITHM_ABSOLUTE);
#endif
    }

    rrddim_set_by_pointer(st_entries, rd_entries_entries, (collected_number)entries);
#ifdef NETDATA_INTERNAL_CHECKS
    rrddim_set_by_pointer(st_entries, rd_entries_refs, (collected_number)references);
#endif
    rrdset_done(st_entries);

    if (unlikely(!st_mem)) {
        st_mem = rrdset_create_localhost(
            "netdata"
            , "strings_memory"
            , NULL
            , "strings"
            , NULL
            , "Strings memory"
            , "bytes"
            , "netdata"
            , "pulse"
            , 910001
            , localhost->rrd_update_every
            , RRDSET_TYPE_STACKED);

        rd_mem     = rrddim_add(st_mem, "memory", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
        rd_mem_idx = rrddim_add(st_mem, "index", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
    }

    rrddim_set_by_pointer(st_mem, rd_mem, (collected_number)memory);
    rrddim_set_by_pointer(st_mem, rd_mem_idx, (collected_number)memory_index);
    rrdset_done(st_mem);
}
