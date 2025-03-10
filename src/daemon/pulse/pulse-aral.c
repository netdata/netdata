// SPDX-License-Identifier: GPL-3.0-or-later

#define PULSE_INTERNALS 1
#include "pulse-aral.h"

struct aral_info {
    const char *name;
    RRDSET *st_memory;
    RRDDIM *rd_malloc_used, *rd_malloc_free, *rd_mmap_used, *rd_mmap_free, *rd_structures, *rd_padding;

    RRDSET *st_utilization;
    RRDDIM *rd_utilization;
};

DEFINE_JUDYL_TYPED(ARAL_STATS, struct aral_info *);

static struct {
    SPINLOCK spinlock;
    ARAL_STATS_JudyLSet idx;
} globals = { 0 };

void pulse_aral_register_statistics(struct aral_statistics *stats, const char *name) {
    if(!name || !stats)
        return;

    spinlock_lock(&globals.spinlock);
    struct aral_info *ai = ARAL_STATS_GET(&globals.idx, (Word_t)stats);
    if(!ai) {
        ai = callocz(1, sizeof(struct aral_info));
        ai->name = strdupz(name);
        ARAL_STATS_SET(&globals.idx, (Word_t)stats, ai);
    }
    spinlock_unlock(&globals.spinlock);
}

void pulse_aral_unregister_statistics(struct aral_statistics *stats) {
    spinlock_lock(&globals.spinlock);
    struct aral_info *ai = ARAL_STATS_GET(&globals.idx, (Word_t)stats);
    if(ai) {
        ARAL_STATS_DEL(&globals.idx, (Word_t)stats);
        freez((void *)ai->name);
        freez(ai);
    }
    spinlock_unlock(&globals.spinlock);
}

void pulse_aral_register(ARAL *ar, const char *name) {
    if(!ar) return;

    if(!name)
        name = aral_name(ar);

    struct aral_statistics *stats = aral_get_statistics(ar);

    pulse_aral_register_statistics(stats, name);
}

void pulse_aral_unregister(ARAL *ar) {
    if(!ar) return;
    struct aral_statistics *stats = aral_get_statistics(ar);
    pulse_aral_unregister_statistics(stats);
}

void pulse_aral_init(void) {
    pulse_aral_register_statistics(aral_by_size_statistics(), "by-size");
    pulse_aral_register_statistics(judy_aral_statistics(), "judy");
    pulse_aral_register_statistics(uuidmap_aral_statistics(), "uuidmap");
}

void pulse_aral_do(bool extended) {
    if(!extended) return;

    spinlock_lock(&globals.spinlock);
    Word_t s = 0;
    for(struct aral_info *ai = ARAL_STATS_FIRST(&globals.idx, &s);
         ai;
         ai = ARAL_STATS_NEXT(&globals.idx, &s)) {
        struct aral_statistics *stats = (void *)(uintptr_t)s;
        if (!stats)
            continue;

        size_t malloc_allocated_bytes = __atomic_load_n(&stats->malloc.allocated_bytes, __ATOMIC_RELAXED);
        size_t malloc_used_bytes = __atomic_load_n(&stats->malloc.used_bytes, __ATOMIC_RELAXED);
        if(malloc_used_bytes > malloc_allocated_bytes)
            malloc_allocated_bytes = malloc_used_bytes;
        size_t malloc_free_bytes = malloc_allocated_bytes - malloc_used_bytes;

        size_t mmap_allocated_bytes = __atomic_load_n(&stats->mmap.allocated_bytes, __ATOMIC_RELAXED);
        size_t mmap_used_bytes = __atomic_load_n(&stats->mmap.used_bytes, __ATOMIC_RELAXED);
        if(mmap_used_bytes > mmap_allocated_bytes)
            mmap_allocated_bytes = mmap_used_bytes;
        size_t mmap_free_bytes = mmap_allocated_bytes - mmap_used_bytes;

        size_t allocated_total = malloc_allocated_bytes + mmap_allocated_bytes;
        size_t used_total = malloc_used_bytes + mmap_used_bytes;

        size_t structures_bytes = __atomic_load_n(&stats->structures.allocated_bytes, __ATOMIC_RELAXED);

        size_t padding_bytes = __atomic_load_n(&stats->malloc.padding_bytes, __ATOMIC_RELAXED) +
                               __atomic_load_n(&stats->mmap.padding_bytes, __ATOMIC_RELAXED);

        NETDATA_DOUBLE utilization;
        if(allocated_total)
            utilization = 100.0 * (NETDATA_DOUBLE)used_total / (NETDATA_DOUBLE)allocated_total;
        else
            utilization = 100.0;

        {
            if (unlikely(!ai->st_memory)) {
                char id[256];

                snprintfz(id, sizeof(id), "aral_%s_memory", ai->name);
                netdata_fix_chart_id(id);

                ai->st_memory = rrdset_create_localhost(
                    "netdata",
                    id,
                    NULL,
                    "ARAL",
                    "netdata.aral_memory",
                    "Array Allocator Memory Utilization",
                    "bytes",
                    "netdata",
                    "pulse",
                    910000,
                    localhost->rrd_update_every,
                    RRDSET_TYPE_STACKED);

                rrdlabels_add(ai->st_memory->rrdlabels, "ARAL", ai->name, RRDLABEL_SRC_AUTO);

                ai->rd_malloc_free = rrddim_add(ai->st_memory, "malloc free", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
                ai->rd_mmap_free   = rrddim_add(ai->st_memory, "mmap free", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
                ai->rd_malloc_used = rrddim_add(ai->st_memory, "malloc used", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
                ai->rd_mmap_used   = rrddim_add(ai->st_memory, "mmap used", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
                ai->rd_structures  = rrddim_add(ai->st_memory, "structures", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
                ai->rd_padding     = rrddim_add(ai->st_memory, "padding", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
            }

            rrddim_set_by_pointer(ai->st_memory, ai->rd_malloc_used, (collected_number)malloc_used_bytes);
            rrddim_set_by_pointer(ai->st_memory, ai->rd_malloc_free, (collected_number)malloc_free_bytes);
            rrddim_set_by_pointer(ai->st_memory, ai->rd_mmap_used, (collected_number)mmap_used_bytes);
            rrddim_set_by_pointer(ai->st_memory, ai->rd_mmap_free, (collected_number)mmap_free_bytes);
            rrddim_set_by_pointer(ai->st_memory, ai->rd_structures, (collected_number)structures_bytes);
            rrddim_set_by_pointer(ai->st_memory, ai->rd_padding, (collected_number)padding_bytes);
            rrdset_done(ai->st_memory);
        }

        {
            if (unlikely(!ai->st_utilization)) {
                char id[256];

                snprintfz(id, sizeof(id), "aral_%s_utilization", ai->name);
                netdata_fix_chart_id(id);

                ai->st_utilization = rrdset_create_localhost(
                    "netdata",
                    id,
                    NULL,
                    "ARAL",
                    "netdata.aral_utilization",
                    "Array Allocator Memory Utilization",
                    "%",
                    "netdata",
                    "pulse",
                    910001,
                    localhost->rrd_update_every,
                    RRDSET_TYPE_LINE);

                rrdlabels_add(ai->st_utilization->rrdlabels, "ARAL", ai->name, RRDLABEL_SRC_AUTO);

                ai->rd_utilization = rrddim_add(ai->st_utilization, "utilization", NULL, 1, 1000, RRD_ALGORITHM_ABSOLUTE);
            }

            rrddim_set_by_pointer(ai->st_utilization, ai->rd_utilization, (collected_number)(utilization * 1000.0));
            rrdset_done(ai->st_utilization);
        }
    }

    spinlock_unlock(&globals.spinlock);
}
