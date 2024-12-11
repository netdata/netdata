// SPDX-License-Identifier: GPL-3.0-or-later

#define PULSE_INTERNALS 1
#include "pulse-trace-allocations.h"

#ifdef NETDATA_TRACE_ALLOCATIONS

struct memory_trace_data {
    RRDSET *st_memory;
    RRDSET *st_allocations;
    RRDSET *st_avg_alloc;
    RRDSET *st_ops;
};

static int do_memory_trace_item(void *item, void *data) {
    struct memory_trace_data *tmp = data;
    struct malloc_trace *p = item;

    // ------------------------------------------------------------------------

    if(!p->rd_bytes)
        p->rd_bytes = rrddim_add(tmp->st_memory, p->function, NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);

    collected_number bytes = (collected_number)__atomic_load_n(&p->bytes, __ATOMIC_RELAXED);
    rrddim_set_by_pointer(tmp->st_memory, p->rd_bytes, bytes);

    // ------------------------------------------------------------------------

    if(!p->rd_allocations)
        p->rd_allocations = rrddim_add(tmp->st_allocations, p->function, NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);

    collected_number allocs = (collected_number)__atomic_load_n(&p->allocations, __ATOMIC_RELAXED);
    rrddim_set_by_pointer(tmp->st_allocations, p->rd_allocations, allocs);

    // ------------------------------------------------------------------------

    if(!p->rd_avg_alloc)
        p->rd_avg_alloc = rrddim_add(tmp->st_avg_alloc, p->function, NULL, 1, 100, RRD_ALGORITHM_ABSOLUTE);

    collected_number avg_alloc = (allocs)?(bytes * 100 / allocs):0;
    rrddim_set_by_pointer(tmp->st_avg_alloc, p->rd_avg_alloc, avg_alloc);

    // ------------------------------------------------------------------------

    if(!p->rd_ops)
        p->rd_ops = rrddim_add(tmp->st_ops, p->function, NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);

    collected_number ops = 0;
    ops += (collected_number)__atomic_load_n(&p->malloc_calls, __ATOMIC_RELAXED);
    ops += (collected_number)__atomic_load_n(&p->calloc_calls, __ATOMIC_RELAXED);
    ops += (collected_number)__atomic_load_n(&p->realloc_calls, __ATOMIC_RELAXED);
    ops += (collected_number)__atomic_load_n(&p->strdup_calls, __ATOMIC_RELAXED);
    ops += (collected_number)__atomic_load_n(&p->free_calls, __ATOMIC_RELAXED);
    rrddim_set_by_pointer(tmp->st_ops, p->rd_ops, ops);

    // ------------------------------------------------------------------------

    return 1;
}

void pulse_trace_allocations_do(bool extended) {
    if(!extended) return;

    static struct memory_trace_data tmp = {
        .st_memory = NULL,
        .st_allocations = NULL,
        .st_avg_alloc = NULL,
        .st_ops = NULL,
    };

    if(!tmp.st_memory) {
        tmp.st_memory = rrdset_create_localhost(
            "netdata"
            , "memory_size"
            , NULL
            , "memory"
            , "netdata.memory.size"
            , "Netdata Memory Used by Function"
            , "bytes"
            , "netdata"
            , "pulse"
            , 900000
            , localhost->rrd_update_every
            , RRDSET_TYPE_STACKED
        );
    }

    if(!tmp.st_ops) {
        tmp.st_ops = rrdset_create_localhost(
            "netdata"
            , "memory_operations"
            , NULL
            , "memory"
            , "netdata.memory.operations"
            , "Netdata Memory Operations by Function"
            , "ops/s"
            , "netdata"
            , "pulse"
            , 900001
            , localhost->rrd_update_every
            , RRDSET_TYPE_LINE
        );
    }

    if(!tmp.st_allocations) {
        tmp.st_allocations = rrdset_create_localhost(
            "netdata"
            , "memory_allocations"
            , NULL
            , "memory"
            , "netdata.memory.allocations"
            , "Netdata Memory Allocations by Function"
            , "allocations"
            , "netdata"
            , "pulse"
            , 900002
            , localhost->rrd_update_every
            , RRDSET_TYPE_STACKED
        );
    }

    if(!tmp.st_avg_alloc) {
        tmp.st_avg_alloc = rrdset_create_localhost(
            "netdata"
            , "memory_avg_alloc"
            , NULL
            , "memory"
            , "netdata.memory.avg_alloc"
            , "Netdata Average Allocation Size by Function"
            , "bytes"
            , "netdata"
            , "pulse"
            , 900003
            , localhost->rrd_update_every
            , RRDSET_TYPE_LINE
        );
    }

    malloc_trace_walkthrough(do_memory_trace_item, &tmp);

    rrdset_done(tmp.st_memory);
    rrdset_done(tmp.st_ops);
    rrdset_done(tmp.st_allocations);
    rrdset_done(tmp.st_avg_alloc);
}

#endif
