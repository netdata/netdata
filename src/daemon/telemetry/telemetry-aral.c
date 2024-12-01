// SPDX-License-Identifier: GPL-3.0-or-later

#define TELEMETRY_INTERNALS 1
#include "telemetry-aral.h"

struct aral_info {
    const char *name;
    RRDSET *st_memory;
    RRDDIM *rd_used, *rd_free, *rd_structures;

    RRDSET *st_fragmentation;
    RRDDIM *rd_fragmentation;
};

DEFINE_JUDYL_TYPED(ARAL_STATS, struct aral_info *);

static struct {
    SPINLOCK spinlock;
    ARAL_STATS_JudyLSet idx;
} globals = { 0 };

static void telemetry_aral_register_statistics(struct aral_statistics *stats, const char *name) {
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

void telemetry_aral_register(ARAL *ar, const char *name) {
    if(!ar) return;

    if(!name)
        name = aral_name(ar);

    struct aral_statistics *stats = aral_get_statistics(ar);

    telemetry_aral_register_statistics(stats, name);
}

void telemetry_aral_unregister(ARAL *ar) {
    if(!ar) return;
    struct aral_statistics *stats = aral_get_statistics(ar);

    spinlock_lock(&globals.spinlock);
    struct aral_info *ai = ARAL_STATS_GET(&globals.idx, (Word_t)stats);
    if(ai) {
        ARAL_STATS_DEL(&globals.idx, (Word_t)stats);
        freez((void *)ai->name);
        freez(ai);
    }
    spinlock_unlock(&globals.spinlock);
}

void telemerty_aral_init(void) {
    telemetry_aral_register_statistics(aral_by_size_statistics(), "by-size");
}

void telemetry_aral_do(bool extended) {
    if(!extended) return;

    spinlock_lock(&globals.spinlock);
    Word_t s = 0;
    for(struct aral_info *ai = ARAL_STATS_FIRST(&globals.idx, &s);
         ai;
         ai = ARAL_STATS_NEXT(&globals.idx, &s)) {
        struct aral_statistics *stats = (void *)(uintptr_t)s;
        if (!stats)
            continue;

        size_t allocated_bytes = __atomic_load_n(&stats->malloc.allocated_bytes, __ATOMIC_RELAXED) +
                                 __atomic_load_n(&stats->mmap.allocated_bytes, __ATOMIC_RELAXED);

        size_t used_bytes = __atomic_load_n(&stats->malloc.used_bytes, __ATOMIC_RELAXED) +
                            __atomic_load_n(&stats->mmap.used_bytes, __ATOMIC_RELAXED);

        size_t structures_bytes = __atomic_load_n(&stats->structures.allocated_bytes, __ATOMIC_RELAXED);

        size_t free_bytes = (allocated_bytes > used_bytes) ? allocated_bytes - used_bytes : 0;

        NETDATA_DOUBLE fragmentation;
        if(used_bytes && allocated_bytes)
            fragmentation = 100 * (NETDATA_DOUBLE)free_bytes / (NETDATA_DOUBLE)allocated_bytes;
        else
            fragmentation = 0.0;

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
                    "telemetry",
                    910000,
                    localhost->rrd_update_every,
                    RRDSET_TYPE_STACKED);

                rrdlabels_add(ai->st_memory->rrdlabels, "ARAL", ai->name, RRDLABEL_SRC_AUTO);

                ai->rd_free = rrddim_add(ai->st_memory, "free", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
                ai->rd_used = rrddim_add(ai->st_memory, "used", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
                ai->rd_structures = rrddim_add(ai->st_memory, "structures", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
            }

            rrddim_set_by_pointer(ai->st_memory, ai->rd_used, (collected_number)allocated_bytes);
            rrddim_set_by_pointer(ai->st_memory, ai->rd_free, (collected_number)free_bytes);
            rrddim_set_by_pointer(ai->st_memory, ai->rd_structures, (collected_number)structures_bytes);
            rrdset_done(ai->st_memory);
        }

        {
            if (unlikely(!ai->st_fragmentation)) {
                char id[256];

                snprintfz(id, sizeof(id), "aral_%s_fragmentation", ai->name);
                netdata_fix_chart_id(id);

                ai->st_fragmentation = rrdset_create_localhost(
                    "netdata",
                    id,
                    NULL,
                    "ARAL",
                    "netdata.aral_fragmentation",
                    "Array Allocator Memory Fragmentation",
                    "%",
                    "netdata",
                    "telemetry",
                    910001,
                    localhost->rrd_update_every,
                    RRDSET_TYPE_LINE);

                rrdlabels_add(ai->st_fragmentation->rrdlabels, "ARAL", ai->name, RRDLABEL_SRC_AUTO);

                ai->rd_fragmentation = rrddim_add(ai->st_fragmentation, "fragmentation", NULL, 1, 10000, RRD_ALGORITHM_ABSOLUTE);
            }

            rrddim_set_by_pointer(ai->st_fragmentation, ai->rd_fragmentation, (collected_number)(fragmentation * 10000.0));
            rrdset_done(ai->st_fragmentation);
        }
    }

    spinlock_unlock(&globals.spinlock);
}
