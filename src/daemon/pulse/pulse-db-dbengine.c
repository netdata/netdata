// SPDX-License-Identifier: GPL-3.0-or-later

#define PULSE_INTERNALS 1
#include "pulse-db-dbengine.h"

int64_t pulse_dbengine_total_memory = 0;

#if defined(ENABLE_DBENGINE)

static usec_t time_and_count_delta_average(struct time_and_count *prev, struct time_and_count *latest) {
    if(latest->count > prev->count && latest->usec > prev->usec)
        return (latest->usec - prev->usec) / (latest->count - prev->count);

    return 0;
}

struct dbengine2_cache_pointers {
    RRDSET *st_cache_hit_ratio;
    RRDDIM *rd_hit_ratio_closest;
    RRDDIM *rd_hit_ratio_exact;

    RRDSET *st_operations;
    RRDDIM *rd_searches_closest;
    RRDDIM *rd_searches_exact;
    RRDDIM *rd_add_hot;
    RRDDIM *rd_add_clean;
    RRDDIM *rd_evictions;
    RRDDIM *rd_flushes;
    RRDDIM *rd_acquires;
    RRDDIM *rd_releases;
    RRDDIM *rd_acquires_for_deletion;

    RRDSET *st_pgc_memory;
    RRDDIM *rd_pgc_memory_free;
    RRDDIM *rd_pgc_memory_clean;
    RRDDIM *rd_pgc_memory_hot;
    RRDDIM *rd_pgc_memory_dirty;
    RRDDIM *rd_pgc_memory_index;
    RRDDIM *rd_pgc_memory_evicting;
    RRDDIM *rd_pgc_memory_flushing;

    struct {
        RRDSET *st_pgc_page_size_heatmap;
        RRDDIM *rd_pgc_page_size_x[PGC_SIZE_HISTOGRAM_ENTRIES];
    } queues[3];

    RRDSET *st_pgc_tm;
    RRDDIM *rd_pgc_tm_current;
    RRDDIM *rd_pgc_tm_wanted;
    RRDDIM *rd_pgc_tm_hot_max;
    RRDDIM *rd_pgc_tm_dirty_max;
    RRDDIM *rd_pgc_tm_hot;
    RRDDIM *rd_pgc_tm_dirty;
    RRDDIM *rd_pgc_tm_referenced;

    RRDSET *st_pgc_pages;
    RRDDIM *rd_pgc_pages_clean;
    RRDDIM *rd_pgc_pages_hot;
    RRDDIM *rd_pgc_pages_dirty;
    RRDDIM *rd_pgc_pages_referenced;

    RRDSET *st_pgc_memory_changes;
    RRDDIM *rd_pgc_memory_new_hot;
    RRDDIM *rd_pgc_memory_new_clean;
    RRDDIM *rd_pgc_memory_clean_evictions;

    RRDSET *st_pgc_memory_migrations;
    RRDDIM *rd_pgc_memory_hot_to_dirty;
    RRDDIM *rd_pgc_memory_dirty_to_clean;

    RRDSET *st_pgc_workers;
    RRDDIM *rd_pgc_workers_evictors;
    RRDDIM *rd_pgc_workers_flushers;
    RRDDIM *rd_pgc_workers_adders;
    RRDDIM *rd_pgc_workers_searchers;
    RRDDIM *rd_pgc_workers_jv2_flushers;
    RRDDIM *rd_pgc_workers_hot2dirty;

    RRDSET *st_pgc_memory_events;
    RRDDIM *rd_pgc_memory_evictions_critical;
    RRDDIM *rd_pgc_memory_evictions_aggressive;
    RRDDIM *rd_pgc_memory_flushes_critical;
    RRDDIM *rd_pgc_waste_evict_thread_signals;
    RRDDIM *rd_pgc_waste_evict_inline_on_add;
    RRDDIM *rd_pgc_waste_evict_inline_on_release;
    RRDDIM *rd_pgc_waste_flush_inline_on_add;
    RRDDIM *rd_pgc_waste_flush_inline_on_release;

    RRDSET *st_pgc_waste;
    RRDDIM *rd_pgc_waste_evict_relocated;
    RRDDIM *rd_pgc_waste_flushes_cancelled;
    RRDDIM *rd_pgc_waste_insert_spins;
    RRDDIM *rd_pgc_waste_evict_spins;
};

static void dbengine2_cache_statistics_charts(struct dbengine2_cache_pointers *ptrs, struct pgc_statistics *pgc_stats, struct pgc_statistics *pgc_stats_old __maybe_unused, const char *name, int priority) {

    {
        if (unlikely(!ptrs->st_cache_hit_ratio)) {
            BUFFER *id = buffer_create(100, NULL);
            buffer_sprintf(id, "dbengine_%s_cache_hit_ratio", name);

            BUFFER *family = buffer_create(100, NULL);
            buffer_sprintf(family, "dbengine %s cache", name);

            BUFFER *title = buffer_create(100, NULL);
            buffer_sprintf(title, "Netdata %s Cache Hit Ratio", name);

            ptrs->st_cache_hit_ratio = rrdset_create_localhost(
                "netdata",
                buffer_tostring(id),
                NULL,
                buffer_tostring(family),
                NULL,
                buffer_tostring(title),
                "%",
                "netdata",
                "pulse",
                priority,
                localhost->rrd_update_every,
                RRDSET_TYPE_LINE);

            ptrs->rd_hit_ratio_closest = rrddim_add(ptrs->st_cache_hit_ratio, "closest", NULL, 1, 10000, RRD_ALGORITHM_ABSOLUTE);
            ptrs->rd_hit_ratio_exact = rrddim_add(ptrs->st_cache_hit_ratio, "exact", NULL, 1, 10000, RRD_ALGORITHM_ABSOLUTE);

            buffer_free(id);
            buffer_free(family);
            buffer_free(title);
            priority++;
        }

        size_t closest_percent = 100 * 10000;
        if(pgc_stats->searches_closest > pgc_stats_old->searches_closest)
            closest_percent = (pgc_stats->searches_closest_hits - pgc_stats_old->searches_closest_hits) * 100 * 10000 / (pgc_stats->searches_closest - pgc_stats_old->searches_closest);

        size_t exact_percent = 100 * 10000;
        if(pgc_stats->searches_exact > pgc_stats_old->searches_exact)
            exact_percent = (pgc_stats->searches_exact_hits - pgc_stats_old->searches_exact_hits) * 100 * 10000 / (pgc_stats->searches_exact - pgc_stats_old->searches_exact);

        rrddim_set_by_pointer(ptrs->st_cache_hit_ratio, ptrs->rd_hit_ratio_closest, (collected_number)closest_percent);
        rrddim_set_by_pointer(ptrs->st_cache_hit_ratio, ptrs->rd_hit_ratio_exact, (collected_number)exact_percent);

        rrdset_done(ptrs->st_cache_hit_ratio);
    }

    {
        if (unlikely(!ptrs->st_operations)) {
            BUFFER *id = buffer_create(100, NULL);
            buffer_sprintf(id, "dbengine_%s_cache_operations", name);

            BUFFER *family = buffer_create(100, NULL);
            buffer_sprintf(family, "dbengine %s cache", name);

            BUFFER *title = buffer_create(100, NULL);
            buffer_sprintf(title, "Netdata %s Cache Operations", name);

            ptrs->st_operations = rrdset_create_localhost(
                "netdata",
                buffer_tostring(id),
                NULL,
                buffer_tostring(family),
                NULL,
                buffer_tostring(title),
                "ops/s",
                "netdata",
                "pulse",
                priority,
                localhost->rrd_update_every,
                RRDSET_TYPE_LINE);

            ptrs->rd_searches_closest   = rrddim_add(ptrs->st_operations, "search closest", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
            ptrs->rd_searches_exact     = rrddim_add(ptrs->st_operations, "search exact", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
            ptrs->rd_add_hot            = rrddim_add(ptrs->st_operations, "add hot", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
            ptrs->rd_add_clean          = rrddim_add(ptrs->st_operations, "add clean", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
            ptrs->rd_evictions          = rrddim_add(ptrs->st_operations, "evictions", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
            ptrs->rd_flushes            = rrddim_add(ptrs->st_operations, "flushes", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
            ptrs->rd_acquires           = rrddim_add(ptrs->st_operations, "acquires", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
            ptrs->rd_releases           = rrddim_add(ptrs->st_operations, "releases", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
            ptrs->rd_acquires_for_deletion = rrddim_add(ptrs->st_operations, "del acquires", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);

            buffer_free(id);
            buffer_free(family);
            buffer_free(title);
            priority++;
        }

        rrddim_set_by_pointer(ptrs->st_operations, ptrs->rd_searches_closest, (collected_number)pgc_stats->searches_closest);
        rrddim_set_by_pointer(ptrs->st_operations, ptrs->rd_searches_exact, (collected_number)pgc_stats->searches_exact);
        rrddim_set_by_pointer(ptrs->st_operations, ptrs->rd_add_hot, (collected_number)pgc_stats->queues[PGC_QUEUE_HOT].added_entries);
        rrddim_set_by_pointer(ptrs->st_operations, ptrs->rd_add_clean, (collected_number)(pgc_stats->added_entries - pgc_stats->queues[PGC_QUEUE_HOT].added_entries));
        rrddim_set_by_pointer(ptrs->st_operations, ptrs->rd_evictions, (collected_number)pgc_stats->queues[PGC_QUEUE_CLEAN].removed_entries);
        rrddim_set_by_pointer(ptrs->st_operations, ptrs->rd_flushes, (collected_number)pgc_stats->flushes_completed);
        rrddim_set_by_pointer(ptrs->st_operations, ptrs->rd_acquires, (collected_number)pgc_stats->acquires);
        rrddim_set_by_pointer(ptrs->st_operations, ptrs->rd_releases, (collected_number)pgc_stats->releases);
        rrddim_set_by_pointer(ptrs->st_operations, ptrs->rd_acquires_for_deletion, (collected_number)pgc_stats->acquires_for_deletion);

        rrdset_done(ptrs->st_operations);
    }

    {
        if (unlikely(!ptrs->st_pgc_memory)) {
            BUFFER *id = buffer_create(100, NULL);
            buffer_sprintf(id, "dbengine_%s_cache_memory", name);

            BUFFER *family = buffer_create(100, NULL);
            buffer_sprintf(family, "dbengine %s cache", name);

            BUFFER *title = buffer_create(100, NULL);
            buffer_sprintf(title, "Netdata %s Cache Memory", name);

            ptrs->st_pgc_memory = rrdset_create_localhost(
                "netdata",
                buffer_tostring(id),
                NULL,
                buffer_tostring(family),
                NULL,
                buffer_tostring(title),
                "bytes",
                "netdata",
                "pulse",
                priority,
                localhost->rrd_update_every,
                RRDSET_TYPE_STACKED);

            ptrs->rd_pgc_memory_free     = rrddim_add(ptrs->st_pgc_memory, "free",     NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
            ptrs->rd_pgc_memory_hot      = rrddim_add(ptrs->st_pgc_memory, "hot",      NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
            ptrs->rd_pgc_memory_dirty    = rrddim_add(ptrs->st_pgc_memory, "dirty",    NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
            ptrs->rd_pgc_memory_clean    = rrddim_add(ptrs->st_pgc_memory, "clean",    NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
            ptrs->rd_pgc_memory_index    = rrddim_add(ptrs->st_pgc_memory, "index",    NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
            ptrs->rd_pgc_memory_evicting = rrddim_add(ptrs->st_pgc_memory, "evicting", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
            ptrs->rd_pgc_memory_flushing = rrddim_add(ptrs->st_pgc_memory, "flushing", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);

            buffer_free(id);
            buffer_free(family);
            buffer_free(title);
            priority++;
        }

        collected_number free = (pgc_stats->current_cache_size > pgc_stats->wanted_cache_size) ? 0 :
                                                                                                 (collected_number)(pgc_stats->wanted_cache_size - pgc_stats->current_cache_size);

        rrddim_set_by_pointer(ptrs->st_pgc_memory, ptrs->rd_pgc_memory_free, free);
        rrddim_set_by_pointer(ptrs->st_pgc_memory, ptrs->rd_pgc_memory_hot, (collected_number)pgc_stats->queues[PGC_QUEUE_HOT].size);
        rrddim_set_by_pointer(ptrs->st_pgc_memory, ptrs->rd_pgc_memory_dirty, (collected_number)pgc_stats->queues[PGC_QUEUE_DIRTY].size);
        rrddim_set_by_pointer(ptrs->st_pgc_memory, ptrs->rd_pgc_memory_clean, (collected_number)pgc_stats->queues[PGC_QUEUE_CLEAN].size);
        rrddim_set_by_pointer(ptrs->st_pgc_memory, ptrs->rd_pgc_memory_evicting, (collected_number)pgc_stats->evicting_size);
        rrddim_set_by_pointer(ptrs->st_pgc_memory, ptrs->rd_pgc_memory_flushing, (collected_number)pgc_stats->flushing_size);
        rrddim_set_by_pointer(ptrs->st_pgc_memory, ptrs->rd_pgc_memory_index,(collected_number)(pgc_stats->size - pgc_stats->queues[PGC_QUEUE_CLEAN].size - pgc_stats->queues[PGC_QUEUE_HOT].size - pgc_stats->queues[PGC_QUEUE_DIRTY].size - pgc_stats->evicting_size - pgc_stats->flushing_size));

        rrdset_done(ptrs->st_pgc_memory);
    }

    for(size_t q = 0; q < 3 ;q++) {
        const char *queue;
        switch(q) {
            case PGC_QUEUE_HOT:
                queue = "hot";
                break;

            case PGC_QUEUE_DIRTY:
                queue = "dirty";
                break;

            default:
            case PGC_QUEUE_CLEAN:
                queue = "clean";
                break;
        }

        if (unlikely(!ptrs->queues[q].st_pgc_page_size_heatmap)) {
            CLEAN_BUFFER *ctx = buffer_create(100, NULL);
            buffer_sprintf(ctx, "netdata.dbengine_%s_page_sizes", name);

            CLEAN_BUFFER *id = buffer_create(100, NULL);
            buffer_sprintf(id, "dbengine_%s_%s_page_sizes", name, queue);

            CLEAN_BUFFER *family = buffer_create(100, NULL);
            buffer_sprintf(family, "dbengine %s cache", name);

            CLEAN_BUFFER *title = buffer_create(100, NULL);
            buffer_sprintf(title, "Netdata %s Nominal Page Sizes (without overheads)", name);

            ptrs->queues[q].st_pgc_page_size_heatmap = rrdset_create_localhost(
                "netdata",
                buffer_tostring(id),
                NULL,
                buffer_tostring(family),
                buffer_tostring(ctx),
                buffer_tostring(title),
                "pages",
                "netdata",
                "pulse",
                priority,
                localhost->rrd_update_every,
                RRDSET_TYPE_HEATMAP);

            ptrs->queues[q].rd_pgc_page_size_x[0] = rrddim_add(ptrs->queues[q].st_pgc_page_size_heatmap, "empty", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
            for(size_t i = 1; i < _countof(ptrs->queues[q].rd_pgc_page_size_x) - 1 ;i++) {
                char buf[64];
                snprintfz(buf, sizeof(buf), "%zu", pgc_stats->queues[q].size_histogram.array[i].upto);
                // size_snprintf(&buf[1], sizeof(buf) - 1, pgc_stats->size_histogram.array[i].upto, "B", true);
                ptrs->queues[q].rd_pgc_page_size_x[i] = rrddim_add(ptrs->queues[q].st_pgc_page_size_heatmap, buf, NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
            }
            ptrs->queues[q].rd_pgc_page_size_x[_countof(ptrs->queues[q].rd_pgc_page_size_x) - 1] = rrddim_add(ptrs->queues[q].st_pgc_page_size_heatmap, "+inf", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);

            rrdlabels_add(ptrs->queues[q].st_pgc_page_size_heatmap->rrdlabels, "Cache", name, RRDLABEL_SRC_AUTO);
            rrdlabels_add(ptrs->queues[q].st_pgc_page_size_heatmap->rrdlabels, "Queue", queue, RRDLABEL_SRC_AUTO);

            priority++;
        }

        for(size_t i = 0; i < _countof(ptrs->queues[q].rd_pgc_page_size_x) - 1 ;i++)
            rrddim_set_by_pointer(ptrs->queues[q].st_pgc_page_size_heatmap, ptrs->queues[q].rd_pgc_page_size_x[i], (collected_number)pgc_stats->queues[q].size_histogram.array[i].count);

        rrdset_done(ptrs->queues[q].st_pgc_page_size_heatmap);
    }

    {
        if (unlikely(!ptrs->st_pgc_tm)) {
            BUFFER *id = buffer_create(100, NULL);
            buffer_sprintf(id, "dbengine_%s_target_memory", name);

            BUFFER *family = buffer_create(100, NULL);
            buffer_sprintf(family, "dbengine %s cache", name);

            BUFFER *title = buffer_create(100, NULL);
            buffer_sprintf(title, "Netdata %s Target Cache Memory", name);

            ptrs->st_pgc_tm = rrdset_create_localhost(
                "netdata",
                buffer_tostring(id),
                NULL,
                buffer_tostring(family),
                NULL,
                buffer_tostring(title),
                "bytes",
                "netdata",
                "pulse",
                priority,
                localhost->rrd_update_every,
                RRDSET_TYPE_LINE);

            ptrs->rd_pgc_tm_current    = rrddim_add(ptrs->st_pgc_tm, "current",    NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
            ptrs->rd_pgc_tm_wanted     = rrddim_add(ptrs->st_pgc_tm, "wanted",     NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
            ptrs->rd_pgc_tm_referenced = rrddim_add(ptrs->st_pgc_tm, "referenced", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
            ptrs->rd_pgc_tm_hot_max    = rrddim_add(ptrs->st_pgc_tm, "hot max",    NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
            ptrs->rd_pgc_tm_dirty_max  = rrddim_add(ptrs->st_pgc_tm, "dirty max",  NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
            ptrs->rd_pgc_tm_hot        = rrddim_add(ptrs->st_pgc_tm, "hot",        NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
            ptrs->rd_pgc_tm_dirty      = rrddim_add(ptrs->st_pgc_tm, "dirty",      NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);

            buffer_free(id);
            buffer_free(family);
            buffer_free(title);
            priority++;
        }

        rrddim_set_by_pointer(ptrs->st_pgc_tm, ptrs->rd_pgc_tm_current, (collected_number)pgc_stats->current_cache_size);
        rrddim_set_by_pointer(ptrs->st_pgc_tm, ptrs->rd_pgc_tm_wanted, (collected_number)pgc_stats->wanted_cache_size);
        rrddim_set_by_pointer(ptrs->st_pgc_tm, ptrs->rd_pgc_tm_referenced, (collected_number)pgc_stats->referenced_size);
        rrddim_set_by_pointer(ptrs->st_pgc_tm, ptrs->rd_pgc_tm_hot_max, (collected_number)pgc_stats->queues[PGC_QUEUE_HOT].max_size);
        rrddim_set_by_pointer(ptrs->st_pgc_tm, ptrs->rd_pgc_tm_dirty_max, (collected_number)pgc_stats->queues[PGC_QUEUE_DIRTY].max_size);
        rrddim_set_by_pointer(ptrs->st_pgc_tm, ptrs->rd_pgc_tm_hot, (collected_number)pgc_stats->queues[PGC_QUEUE_HOT].size);
        rrddim_set_by_pointer(ptrs->st_pgc_tm, ptrs->rd_pgc_tm_dirty, (collected_number)pgc_stats->queues[PGC_QUEUE_DIRTY].size);

        rrdset_done(ptrs->st_pgc_tm);
    }

    {
        if (unlikely(!ptrs->st_pgc_pages)) {
            BUFFER *id = buffer_create(100, NULL);
            buffer_sprintf(id, "dbengine_%s_cache_pages", name);

            BUFFER *family = buffer_create(100, NULL);
            buffer_sprintf(family, "dbengine %s cache", name);

            BUFFER *title = buffer_create(100, NULL);
            buffer_sprintf(title, "Netdata %s Cache Pages", name);

            ptrs->st_pgc_pages = rrdset_create_localhost(
                "netdata",
                buffer_tostring(id),
                NULL,
                buffer_tostring(family),
                NULL,
                buffer_tostring(title),
                "pages",
                "netdata",
                "pulse",
                priority,
                localhost->rrd_update_every,
                RRDSET_TYPE_LINE);

            ptrs->rd_pgc_pages_clean   = rrddim_add(ptrs->st_pgc_pages, "clean", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
            ptrs->rd_pgc_pages_hot     = rrddim_add(ptrs->st_pgc_pages, "hot", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
            ptrs->rd_pgc_pages_dirty   = rrddim_add(ptrs->st_pgc_pages, "dirty", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
            ptrs->rd_pgc_pages_referenced = rrddim_add(ptrs->st_pgc_pages, "referenced", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);

            buffer_free(id);
            buffer_free(family);
            buffer_free(title);
            priority++;
        }

        rrddim_set_by_pointer(ptrs->st_pgc_pages, ptrs->rd_pgc_pages_clean, (collected_number)pgc_stats->queues[PGC_QUEUE_CLEAN].entries);
        rrddim_set_by_pointer(ptrs->st_pgc_pages, ptrs->rd_pgc_pages_hot, (collected_number)pgc_stats->queues[PGC_QUEUE_HOT].entries);
        rrddim_set_by_pointer(ptrs->st_pgc_pages, ptrs->rd_pgc_pages_dirty, (collected_number)pgc_stats->queues[PGC_QUEUE_DIRTY].entries);
        rrddim_set_by_pointer(ptrs->st_pgc_pages, ptrs->rd_pgc_pages_referenced, (collected_number)pgc_stats->referenced_entries);

        rrdset_done(ptrs->st_pgc_pages);
    }

    {
        if (unlikely(!ptrs->st_pgc_memory_changes)) {
            BUFFER *id = buffer_create(100, NULL);
            buffer_sprintf(id, "dbengine_%s_cache_memory_changes", name);

            BUFFER *family = buffer_create(100, NULL);
            buffer_sprintf(family, "dbengine %s cache", name);

            BUFFER *title = buffer_create(100, NULL);
            buffer_sprintf(title, "Netdata %s Cache Memory Changes", name);

            ptrs->st_pgc_memory_changes = rrdset_create_localhost(
                "netdata",
                buffer_tostring(id),
                NULL,
                buffer_tostring(family),
                NULL,
                buffer_tostring(title),
                "bytes/s",
                "netdata",
                "pulse",
                priority,
                localhost->rrd_update_every,
                RRDSET_TYPE_AREA);

            ptrs->rd_pgc_memory_new_clean         = rrddim_add(ptrs->st_pgc_memory_changes, "new clean", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
            ptrs->rd_pgc_memory_clean_evictions   = rrddim_add(ptrs->st_pgc_memory_changes, "evictions", NULL, -1, 1, RRD_ALGORITHM_INCREMENTAL);
            ptrs->rd_pgc_memory_new_hot           = rrddim_add(ptrs->st_pgc_memory_changes, "new hot", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);

            buffer_free(id);
            buffer_free(family);
            buffer_free(title);
            priority++;
        }

        rrddim_set_by_pointer(ptrs->st_pgc_memory_changes, ptrs->rd_pgc_memory_new_clean, (collected_number)(pgc_stats->added_size - pgc_stats->queues[PGC_QUEUE_HOT].added_size));
        rrddim_set_by_pointer(ptrs->st_pgc_memory_changes, ptrs->rd_pgc_memory_clean_evictions, (collected_number)pgc_stats->queues[PGC_QUEUE_CLEAN].removed_size);
        rrddim_set_by_pointer(ptrs->st_pgc_memory_changes, ptrs->rd_pgc_memory_new_hot, (collected_number)pgc_stats->queues[PGC_QUEUE_HOT].added_size);

        rrdset_done(ptrs->st_pgc_memory_changes);
    }

    {
        if (unlikely(!ptrs->st_pgc_memory_migrations)) {
            BUFFER *id = buffer_create(100, NULL);
            buffer_sprintf(id, "dbengine_%s_cache_memory_migrations", name);

            BUFFER *family = buffer_create(100, NULL);
            buffer_sprintf(family, "dbengine %s cache", name);

            BUFFER *title = buffer_create(100, NULL);
            buffer_sprintf(title, "Netdata %s Cache Memory Migrations", name);

            ptrs->st_pgc_memory_migrations = rrdset_create_localhost(
                "netdata",
                buffer_tostring(id),
                NULL,
                buffer_tostring(family),
                NULL,
                buffer_tostring(title),
                "bytes/s",
                "netdata",
                "pulse",
                priority,
                localhost->rrd_update_every,
                RRDSET_TYPE_AREA);

            ptrs->rd_pgc_memory_dirty_to_clean    = rrddim_add(ptrs->st_pgc_memory_migrations, "dirty to clean", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
            ptrs->rd_pgc_memory_hot_to_dirty      = rrddim_add(ptrs->st_pgc_memory_migrations, "hot to dirty", NULL, -1, 1, RRD_ALGORITHM_INCREMENTAL);

            buffer_free(id);
            buffer_free(family);
            buffer_free(title);
            priority++;
        }

        rrddim_set_by_pointer(ptrs->st_pgc_memory_migrations, ptrs->rd_pgc_memory_dirty_to_clean, (collected_number)pgc_stats->queues[PGC_QUEUE_DIRTY].removed_size);
        rrddim_set_by_pointer(ptrs->st_pgc_memory_migrations, ptrs->rd_pgc_memory_hot_to_dirty, (collected_number)pgc_stats->queues[PGC_QUEUE_DIRTY].added_size);

        rrdset_done(ptrs->st_pgc_memory_migrations);
    }

    {
        if (unlikely(!ptrs->st_pgc_memory_events)) {
            BUFFER *id = buffer_create(100, NULL);
            buffer_sprintf(id, "dbengine_%s_cache_events", name);

            BUFFER *family = buffer_create(100, NULL);
            buffer_sprintf(family, "dbengine %s cache", name);

            BUFFER *title = buffer_create(100, NULL);
            buffer_sprintf(title, "Netdata %s Cache Events", name);

            ptrs->st_pgc_memory_events = rrdset_create_localhost(
                "netdata",
                buffer_tostring(id),
                NULL,
                buffer_tostring(family),
                NULL,
                buffer_tostring(title),
                "events/s",
                "netdata",
                "pulse",
                priority,
                localhost->rrd_update_every,
                RRDSET_TYPE_AREA);

            ptrs->rd_pgc_memory_evictions_aggressive = rrddim_add(ptrs->st_pgc_memory_events, "evictions aggressive", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
            ptrs->rd_pgc_memory_evictions_critical   = rrddim_add(ptrs->st_pgc_memory_events, "evictions critical", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
            ptrs->rd_pgc_memory_flushes_critical     = rrddim_add(ptrs->st_pgc_memory_events, "flushes critical", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);

            buffer_free(id);
            buffer_free(family);
            buffer_free(title);
            priority++;
        }

        rrddim_set_by_pointer(ptrs->st_pgc_memory_events, ptrs->rd_pgc_memory_evictions_aggressive, (collected_number)pgc_stats->events_cache_needs_space_aggressively);
        rrddim_set_by_pointer(ptrs->st_pgc_memory_events, ptrs->rd_pgc_memory_evictions_critical, (collected_number)pgc_stats->events_cache_under_severe_pressure);
        rrddim_set_by_pointer(ptrs->st_pgc_memory_events, ptrs->rd_pgc_memory_flushes_critical, (collected_number)pgc_stats->events_flush_critical);

        rrdset_done(ptrs->st_pgc_memory_events);
    }

    {
        if (unlikely(!ptrs->st_pgc_waste)) {
            BUFFER *id = buffer_create(100, NULL);
            buffer_sprintf(id, "dbengine_%s_waste_events", name);

            BUFFER *family = buffer_create(100, NULL);
            buffer_sprintf(family, "dbengine %s cache", name);

            BUFFER *title = buffer_create(100, NULL);
            buffer_sprintf(title, "Netdata %s Waste Events", name);

            ptrs->st_pgc_waste = rrdset_create_localhost(
                "netdata",
                buffer_tostring(id),
                NULL,
                buffer_tostring(family),
                NULL,
                buffer_tostring(title),
                "events/s",
                "netdata",
                "pulse",
                priority,
                localhost->rrd_update_every,
                RRDSET_TYPE_LINE);

            ptrs->rd_pgc_waste_evict_relocated          = rrddim_add(ptrs->st_pgc_waste, "evict relocated", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
            ptrs->rd_pgc_waste_flushes_cancelled        = rrddim_add(ptrs->st_pgc_waste, "flushes cancelled", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
            ptrs->rd_pgc_waste_insert_spins             = rrddim_add(ptrs->st_pgc_waste, "insert spins", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
            ptrs->rd_pgc_waste_evict_spins              = rrddim_add(ptrs->st_pgc_waste, "evict useless spins", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
            ptrs->rd_pgc_waste_evict_thread_signals     = rrddim_add(ptrs->st_pgc_waste, "evict thread signals", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
            ptrs->rd_pgc_waste_evict_inline_on_add      = rrddim_add(ptrs->st_pgc_waste, "evict inline on add", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
            ptrs->rd_pgc_waste_evict_inline_on_release  = rrddim_add(ptrs->st_pgc_waste, "evict inline on rel", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
            ptrs->rd_pgc_waste_flush_inline_on_add      = rrddim_add(ptrs->st_pgc_waste, "flush inline on add", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
            ptrs->rd_pgc_waste_flush_inline_on_release  = rrddim_add(ptrs->st_pgc_waste, "flush inline on rel", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);

            buffer_free(id);
            buffer_free(family);
            buffer_free(title);
            priority++;
        }

        rrddim_set_by_pointer(ptrs->st_pgc_waste, ptrs->rd_pgc_waste_evict_relocated, (collected_number)pgc_stats->p2_waste_evict_relocated);
        rrddim_set_by_pointer(ptrs->st_pgc_waste, ptrs->rd_pgc_waste_flushes_cancelled, (collected_number)pgc_stats->p2_waste_flushes_cancelled);
        rrddim_set_by_pointer(ptrs->st_pgc_waste, ptrs->rd_pgc_waste_insert_spins, (collected_number)pgc_stats->p2_waste_insert_spins);
        rrddim_set_by_pointer(ptrs->st_pgc_waste, ptrs->rd_pgc_waste_evict_spins, (collected_number)pgc_stats->p2_waste_evict_useless_spins);
        rrddim_set_by_pointer(ptrs->st_pgc_waste, ptrs->rd_pgc_waste_evict_thread_signals, (collected_number)pgc_stats->p2_waste_evict_thread_signals);
        rrddim_set_by_pointer(ptrs->st_pgc_waste, ptrs->rd_pgc_waste_evict_inline_on_add, (collected_number)pgc_stats->p2_waste_evictions_inline_on_add);
        rrddim_set_by_pointer(ptrs->st_pgc_waste, ptrs->rd_pgc_waste_evict_inline_on_release, (collected_number)pgc_stats->p2_waste_evictions_inline_on_release);
        rrddim_set_by_pointer(ptrs->st_pgc_waste, ptrs->rd_pgc_waste_flush_inline_on_add, (collected_number)pgc_stats->p2_waste_flush_on_add);
        rrddim_set_by_pointer(ptrs->st_pgc_waste, ptrs->rd_pgc_waste_flush_inline_on_release, (collected_number)pgc_stats->p2_waste_flush_on_release);

        rrdset_done(ptrs->st_pgc_waste);
    }

    {
        if (unlikely(!ptrs->st_pgc_workers)) {
            BUFFER *id = buffer_create(100, NULL);
            buffer_sprintf(id, "dbengine_%s_cache_workers", name);

            BUFFER *family = buffer_create(100, NULL);
            buffer_sprintf(family, "dbengine %s cache", name);

            BUFFER *title = buffer_create(100, NULL);
            buffer_sprintf(title, "Netdata %s Cache Workers", name);

            ptrs->st_pgc_workers = rrdset_create_localhost(
                "netdata",
                buffer_tostring(id),
                NULL,
                buffer_tostring(family),
                NULL,
                buffer_tostring(title),
                "workers",
                "netdata",
                "pulse",
                priority,
                localhost->rrd_update_every,
                RRDSET_TYPE_LINE);

            ptrs->rd_pgc_workers_searchers = rrddim_add(ptrs->st_pgc_workers, "searchers", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
            ptrs->rd_pgc_workers_adders    = rrddim_add(ptrs->st_pgc_workers, "adders",    NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
            ptrs->rd_pgc_workers_evictors  = rrddim_add(ptrs->st_pgc_workers, "evictors",  NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
            ptrs->rd_pgc_workers_flushers  = rrddim_add(ptrs->st_pgc_workers, "flushers",  NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
            ptrs->rd_pgc_workers_hot2dirty = rrddim_add(ptrs->st_pgc_workers, "hot2dirty",  NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
            ptrs->rd_pgc_workers_jv2_flushers = rrddim_add(ptrs->st_pgc_workers, "jv2 flushers",  NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);

            buffer_free(id);
            buffer_free(family);
            buffer_free(title);
            priority++;
        }

        rrddim_set_by_pointer(ptrs->st_pgc_workers, ptrs->rd_pgc_workers_searchers, (collected_number)pgc_stats->p2_workers_search);
        rrddim_set_by_pointer(ptrs->st_pgc_workers, ptrs->rd_pgc_workers_adders, (collected_number)pgc_stats->p2_workers_add);
        rrddim_set_by_pointer(ptrs->st_pgc_workers, ptrs->rd_pgc_workers_evictors, (collected_number)pgc_stats->p0_workers_evict);
        rrddim_set_by_pointer(ptrs->st_pgc_workers, ptrs->rd_pgc_workers_flushers, (collected_number)pgc_stats->p2_workers_flush);
        rrddim_set_by_pointer(ptrs->st_pgc_workers, ptrs->rd_pgc_workers_hot2dirty, (collected_number)pgc_stats->p2_workers_hot2dirty);
        rrddim_set_by_pointer(ptrs->st_pgc_workers, ptrs->rd_pgc_workers_jv2_flushers, (collected_number)pgc_stats->p2_workers_jv2_flush);

        rrdset_done(ptrs->st_pgc_workers);
    }
}

void pulse_dbengine_do(bool extended) {
    static struct dbengine2_cache_pointers main_cache_ptrs = {}, open_cache_ptrs = {}, extent_cache_ptrs = {};
    static struct rrdeng_cache_efficiency_stats cache_efficiency_stats = {}, cache_efficiency_stats_old = {};
    static struct pgc_statistics pgc_main_stats = {}, pgc_main_stats_old = {}; (void)pgc_main_stats_old;
    static struct pgc_statistics pgc_open_stats = {}, pgc_open_stats_old = {}; (void)pgc_open_stats_old;
    static struct pgc_statistics pgc_extent_stats = {}, pgc_extent_stats_old = {}; (void)pgc_extent_stats_old;
    static struct mrg_statistics mrg_stats = {}, mrg_stats_old = {}; (void)mrg_stats_old;

    pgc_main_stats_old = pgc_main_stats;
    pgc_main_stats = pgc_get_statistics(main_cache);

    pgc_open_stats_old = pgc_open_stats;
    pgc_open_stats = pgc_get_statistics(open_cache);

    pgc_extent_stats_old = pgc_extent_stats;
    pgc_extent_stats = pgc_get_statistics(extent_cache);

    cache_efficiency_stats_old = cache_efficiency_stats;
    cache_efficiency_stats = rrdeng_get_cache_efficiency_stats();

    mrg_stats_old = mrg_stats;

    struct rrdeng_buffer_sizes dbmem = rrdeng_pulse_memory_sizes();

    int64_t buffers_total_size = (int64_t)dbmem.xt_buf + (int64_t)dbmem.wal;

    int64_t aral_structures_total_size = 0, aral_used_total_size = 0;
    int64_t aral_padding_total_size = 0;
    for(size_t i = 0; i < RRDENG_MEM_MAX ; i++) {
        buffers_total_size += (int64_t)aral_free_bytes_from_stats(dbmem.as[i]);
        aral_structures_total_size += (int64_t)aral_structures_bytes_from_stats(dbmem.as[i]);
        aral_used_total_size += (int64_t)aral_used_bytes_from_stats(dbmem.as[i]);
        aral_padding_total_size += (int64_t)aral_padding_bytes_from_stats(dbmem.as[i]);
    }

    pulse_dbengine_total_memory =
        pgc_main_stats.size + pgc_open_stats.size + pgc_extent_stats.size +
        mrg_stats.size +
        buffers_total_size + aral_structures_total_size + aral_padding_total_size + (int64_t)pgd_padding_bytes();

    // we need all the above for the total dbengine memory as reported by the non-extended netdata memory chart
    if(!main_cache || !main_mrg || !extended)
        return;

    dbengine2_cache_statistics_charts(&main_cache_ptrs, &pgc_main_stats, &pgc_main_stats_old, "main", 135100);
    dbengine2_cache_statistics_charts(&open_cache_ptrs, &pgc_open_stats, &pgc_open_stats_old, "open", 135200);
    dbengine2_cache_statistics_charts(&extent_cache_ptrs, &pgc_extent_stats, &pgc_extent_stats_old, "extent", 135300);
    mrg_get_statistics(main_mrg, &mrg_stats);

    int priority = 135000;
    {
        static RRDSET *st_pgc_memory = NULL;
        static RRDDIM *rd_pgc_memory_main = NULL;
        static RRDDIM *rd_pgc_memory_open = NULL;  // open journal memory
        static RRDDIM *rd_pgc_memory_extent = NULL;  // extent compresses cache memory
        static RRDDIM *rd_pgc_memory_metrics = NULL;  // metric registry memory
        static RRDDIM *rd_pgc_memory_buffers = NULL;
        static RRDDIM *rd_pgc_memory_aral_padding = NULL;
        static RRDDIM *rd_pgc_memory_pgd_padding = NULL;
        static RRDDIM *rd_pgc_memory_aral_structures = NULL;

        if (unlikely(!st_pgc_memory)) {
            st_pgc_memory = rrdset_create_localhost(
                "netdata",
                "dbengine_memory",
                NULL,
                "dbengine memory",
                NULL,
                "Netdata DB Memory",
                "bytes",
                "netdata",
                "pulse",
                priority,
                localhost->rrd_update_every,
                RRDSET_TYPE_STACKED);

            rd_pgc_memory_main    = rrddim_add(st_pgc_memory, "main cache", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
            rd_pgc_memory_open    = rrddim_add(st_pgc_memory, "open cache",    NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
            rd_pgc_memory_extent  = rrddim_add(st_pgc_memory, "extent cache",    NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
            rd_pgc_memory_metrics = rrddim_add(st_pgc_memory, "metrics registry", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
            rd_pgc_memory_buffers = rrddim_add(st_pgc_memory, "buffers", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
            rd_pgc_memory_aral_padding = rrddim_add(st_pgc_memory, "aral padding", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
            rd_pgc_memory_pgd_padding = rrddim_add(st_pgc_memory, "pgd padding", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
            rd_pgc_memory_aral_structures = rrddim_add(st_pgc_memory, "aral structures", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
        }
        priority++;


        rrddim_set_by_pointer(st_pgc_memory, rd_pgc_memory_main, (collected_number)pgc_main_stats.size);
        rrddim_set_by_pointer(st_pgc_memory, rd_pgc_memory_open, (collected_number)pgc_open_stats.size);
        rrddim_set_by_pointer(st_pgc_memory, rd_pgc_memory_extent, (collected_number)pgc_extent_stats.size);
        rrddim_set_by_pointer(st_pgc_memory, rd_pgc_memory_metrics, (collected_number)mrg_stats.size);
        rrddim_set_by_pointer(st_pgc_memory, rd_pgc_memory_buffers, (collected_number)buffers_total_size);
        rrddim_set_by_pointer(st_pgc_memory, rd_pgc_memory_aral_padding, (collected_number)aral_padding_total_size);
        rrddim_set_by_pointer(st_pgc_memory, rd_pgc_memory_pgd_padding, (collected_number)pgd_padding_bytes());
        rrddim_set_by_pointer(st_pgc_memory, rd_pgc_memory_aral_structures, (collected_number)aral_structures_total_size);

        rrdset_done(st_pgc_memory);
    }

    {
        static RRDSET *st_pgc_buffers = NULL;
        static RRDDIM *rd_pgc_buffers_pgc = NULL;
        static RRDDIM *rd_pgc_buffers_pgd = NULL;
        static RRDDIM *rd_pgc_buffers_mrg = NULL;
        static RRDDIM *rd_pgc_buffers_opcodes = NULL;
        static RRDDIM *rd_pgc_buffers_handles = NULL;
        static RRDDIM *rd_pgc_buffers_descriptors = NULL;
        static RRDDIM *rd_pgc_buffers_wal = NULL;
        static RRDDIM *rd_pgc_buffers_workers = NULL;
        static RRDDIM *rd_pgc_buffers_pdc = NULL;
        static RRDDIM *rd_pgc_buffers_xt_io = NULL;
        static RRDDIM *rd_pgc_buffers_xt_buf = NULL;
        static RRDDIM *rd_pgc_buffers_epdl = NULL;
        static RRDDIM *rd_pgc_buffers_deol = NULL;
        static RRDDIM *rd_pgc_buffers_pd = NULL;

        if (unlikely(!st_pgc_buffers)) {
            st_pgc_buffers = rrdset_create_localhost(
                "netdata",
                "dbengine_buffers",
                NULL,
                "dbengine memory",
                NULL,
                "Netdata DB Buffers",
                "bytes",
                "netdata",
                "pulse",
                priority,
                localhost->rrd_update_every,
                RRDSET_TYPE_STACKED);

            rd_pgc_buffers_pgc         = rrddim_add(st_pgc_buffers, "pgc",            NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
            rd_pgc_buffers_pgd         = rrddim_add(st_pgc_buffers, "pgd",            NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
            rd_pgc_buffers_mrg         = rrddim_add(st_pgc_buffers, "mrg",            NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
            rd_pgc_buffers_opcodes     = rrddim_add(st_pgc_buffers, "opcodes",        NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
            rd_pgc_buffers_handles     = rrddim_add(st_pgc_buffers, "query handles",  NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
            rd_pgc_buffers_descriptors = rrddim_add(st_pgc_buffers, "descriptors",    NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
            rd_pgc_buffers_wal         = rrddim_add(st_pgc_buffers, "wal",            NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
            rd_pgc_buffers_workers     = rrddim_add(st_pgc_buffers, "workers",        NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
            rd_pgc_buffers_pdc         = rrddim_add(st_pgc_buffers, "pdc",            NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
            rd_pgc_buffers_pd          = rrddim_add(st_pgc_buffers, "pd",             NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
            rd_pgc_buffers_xt_io       = rrddim_add(st_pgc_buffers, "extent io",      NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
            rd_pgc_buffers_xt_buf      = rrddim_add(st_pgc_buffers, "extent buffers", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
            rd_pgc_buffers_epdl        = rrddim_add(st_pgc_buffers, "epdl",           NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
            rd_pgc_buffers_deol        = rrddim_add(st_pgc_buffers, "deol",           NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
        }
        priority++;

        rrddim_set_by_pointer(st_pgc_buffers, rd_pgc_buffers_pgc, (collected_number)aral_free_bytes_from_stats(dbmem.as[RRDENG_MEM_PGC]));
        rrddim_set_by_pointer(st_pgc_buffers, rd_pgc_buffers_pgd, (collected_number)aral_free_bytes_from_stats(dbmem.as[RRDENG_MEM_PGD]));
        rrddim_set_by_pointer(st_pgc_buffers, rd_pgc_buffers_mrg, (collected_number)aral_free_bytes_from_stats(dbmem.as[RRDENG_MEM_MRG]));
        rrddim_set_by_pointer(st_pgc_buffers, rd_pgc_buffers_opcodes, (collected_number)aral_free_bytes_from_stats(dbmem.as[RRDENG_MEM_OPCODES]));
        rrddim_set_by_pointer(st_pgc_buffers, rd_pgc_buffers_handles, (collected_number)aral_free_bytes_from_stats(dbmem.as[RRDENG_MEM_HANDLES]));
        rrddim_set_by_pointer(st_pgc_buffers, rd_pgc_buffers_descriptors, (collected_number)aral_free_bytes_from_stats(dbmem.as[RRDENG_MEM_DESCRIPTORS]));
        rrddim_set_by_pointer(st_pgc_buffers, rd_pgc_buffers_wal, (collected_number)dbmem.wal);
        rrddim_set_by_pointer(st_pgc_buffers, rd_pgc_buffers_workers, (collected_number)aral_free_bytes_from_stats(dbmem.as[RRDENG_MEM_WORKERS]));
        rrddim_set_by_pointer(st_pgc_buffers, rd_pgc_buffers_pdc, (collected_number)aral_free_bytes_from_stats(dbmem.as[RRDENG_MEM_PDC]));
        rrddim_set_by_pointer(st_pgc_buffers, rd_pgc_buffers_pd, (collected_number)aral_free_bytes_from_stats(dbmem.as[RRDENG_MEM_PD]));
        rrddim_set_by_pointer(st_pgc_buffers, rd_pgc_buffers_xt_io, (collected_number)aral_free_bytes_from_stats(dbmem.as[RRDENG_MEM_XT_IO]));
        rrddim_set_by_pointer(st_pgc_buffers, rd_pgc_buffers_xt_buf, (collected_number)dbmem.xt_buf);
        rrddim_set_by_pointer(st_pgc_buffers, rd_pgc_buffers_epdl, (collected_number)aral_free_bytes_from_stats(dbmem.as[RRDENG_MEM_EPDL]));
        rrddim_set_by_pointer(st_pgc_buffers, rd_pgc_buffers_deol, (collected_number)aral_free_bytes_from_stats(dbmem.as[RRDENG_MEM_DEOL]));

        rrdset_done(st_pgc_buffers);
    }

    {
        static RRDSET *st_mrg_metrics = NULL;
        static RRDDIM *rd_mrg_metrics = NULL;
        static RRDDIM *rd_mrg_acquired = NULL;
        static RRDDIM *rd_mrg_collected = NULL;
        static RRDDIM *rd_mrg_multiple_writers = NULL;

        if (unlikely(!st_mrg_metrics)) {
            st_mrg_metrics = rrdset_create_localhost(
                "netdata",
                "dbengine_metrics",
                NULL,
                "dbengine metrics",
                NULL,
                "Netdata Metrics in Metrics Registry",
                "metrics",
                "netdata",
                "pulse",
                priority,
                localhost->rrd_update_every,
                RRDSET_TYPE_LINE);

            rd_mrg_metrics = rrddim_add(st_mrg_metrics, "all", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
            rd_mrg_acquired = rrddim_add(st_mrg_metrics, "acquired", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
            rd_mrg_collected = rrddim_add(st_mrg_metrics, "collected", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
            rd_mrg_multiple_writers = rrddim_add(st_mrg_metrics, "multi-collected", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
        }
        priority++;

        rrddim_set_by_pointer(st_mrg_metrics, rd_mrg_metrics, (collected_number)mrg_stats.entries);
        rrddim_set_by_pointer(st_mrg_metrics, rd_mrg_acquired, (collected_number)mrg_stats.entries_acquired);
        rrddim_set_by_pointer(st_mrg_metrics, rd_mrg_collected, (collected_number)mrg_stats.writers);
        rrddim_set_by_pointer(st_mrg_metrics, rd_mrg_multiple_writers, (collected_number)mrg_stats.writers_conflicts);

        rrdset_done(st_mrg_metrics);
    }

    {
        static RRDSET *st_mrg_ops = NULL;
        static RRDDIM *rd_mrg_add = NULL;
        static RRDDIM *rd_mrg_del = NULL;
        static RRDDIM *rd_mrg_search = NULL;

        if (unlikely(!st_mrg_ops)) {
            st_mrg_ops = rrdset_create_localhost(
                "netdata",
                "dbengine_metrics_registry_operations",
                NULL,
                "dbengine metrics",
                NULL,
                "Netdata Metrics Registry Operations",
                "metrics",
                "netdata",
                "pulse",
                priority,
                localhost->rrd_update_every,
                RRDSET_TYPE_LINE);

            rd_mrg_add = rrddim_add(st_mrg_ops, "add", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
            rd_mrg_del = rrddim_add(st_mrg_ops, "delete", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
            rd_mrg_search = rrddim_add(st_mrg_ops, "search", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
        }
        priority++;

        rrddim_set_by_pointer(st_mrg_ops, rd_mrg_add, (collected_number)mrg_stats.additions);
        rrddim_set_by_pointer(st_mrg_ops, rd_mrg_del, (collected_number)mrg_stats.deletions);
        rrddim_set_by_pointer(st_mrg_ops, rd_mrg_search, (collected_number)mrg_stats.search_hits + (collected_number)mrg_stats.search_misses);

        rrdset_done(st_mrg_ops);
    }

    {
        static RRDSET *st_mrg_references = NULL;
        static RRDDIM *rd_mrg_references = NULL;

        if (unlikely(!st_mrg_references)) {
            st_mrg_references = rrdset_create_localhost(
                "netdata",
                "dbengine_metrics_registry_references",
                NULL,
                "dbengine metrics",
                NULL,
                "Netdata Metrics Registry References",
                "references",
                "netdata",
                "pulse",
                priority,
                localhost->rrd_update_every,
                RRDSET_TYPE_LINE);

            rd_mrg_references = rrddim_add(st_mrg_references, "references", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
        }
        priority++;

        rrddim_set_by_pointer(st_mrg_references, rd_mrg_references, (collected_number)mrg_stats.current_references);

        rrdset_done(st_mrg_references);
    }

    {
        static RRDSET *st_cache_hit_ratio = NULL;
        static RRDDIM *rd_hit_ratio = NULL;
        static RRDDIM *rd_main_cache_hit_ratio = NULL;
        static RRDDIM *rd_extent_cache_hit_ratio = NULL;
        static RRDDIM *rd_extent_merge_hit_ratio = NULL;

        if (unlikely(!st_cache_hit_ratio)) {
            st_cache_hit_ratio = rrdset_create_localhost(
                "netdata",
                "dbengine_cache_hit_ratio",
                NULL,
                "dbengine query router",
                NULL,
                "Netdata Queries Cache Hit Ratio",
                "%",
                "netdata",
                "pulse",
                priority,
                localhost->rrd_update_every,
                RRDSET_TYPE_LINE);

            rd_hit_ratio = rrddim_add(st_cache_hit_ratio, "overall", NULL, 1, 10000, RRD_ALGORITHM_ABSOLUTE);
            rd_main_cache_hit_ratio = rrddim_add(st_cache_hit_ratio, "main cache", NULL, 1, 10000, RRD_ALGORITHM_ABSOLUTE);
            rd_extent_cache_hit_ratio = rrddim_add(st_cache_hit_ratio, "extent cache", NULL, 1, 10000, RRD_ALGORITHM_ABSOLUTE);
            rd_extent_merge_hit_ratio = rrddim_add(st_cache_hit_ratio, "extent merge", NULL, 1, 10000, RRD_ALGORITHM_ABSOLUTE);
        }
        priority++;

        size_t delta_pages_total = cache_efficiency_stats.pages_total - cache_efficiency_stats_old.pages_total;
        size_t delta_pages_to_load_from_disk = cache_efficiency_stats.pages_to_load_from_disk - cache_efficiency_stats_old.pages_to_load_from_disk;
        size_t delta_extents_loaded_from_disk = cache_efficiency_stats.extents_loaded_from_disk - cache_efficiency_stats_old.extents_loaded_from_disk;

        size_t delta_pages_data_source_main_cache = cache_efficiency_stats.pages_data_source_main_cache - cache_efficiency_stats_old.pages_data_source_main_cache;
        size_t delta_pages_pending_found_in_cache_at_pass4 = cache_efficiency_stats.pages_data_source_main_cache_at_pass4 - cache_efficiency_stats_old.pages_data_source_main_cache_at_pass4;

        size_t delta_pages_data_source_extent_cache = cache_efficiency_stats.pages_data_source_extent_cache - cache_efficiency_stats_old.pages_data_source_extent_cache;
        size_t delta_pages_load_extent_merged = cache_efficiency_stats.pages_load_extent_merged - cache_efficiency_stats_old.pages_load_extent_merged;

        size_t pages_total_hit = delta_pages_total - delta_extents_loaded_from_disk;

        static size_t overall_hit_ratio = 100;
        size_t main_cache_hit_ratio = 0, extent_cache_hit_ratio = 0, extent_merge_hit_ratio = 0;
        if(delta_pages_total) {
            if(pages_total_hit > delta_pages_total)
                pages_total_hit = delta_pages_total;

            overall_hit_ratio = pages_total_hit * 100 * 10000 / delta_pages_total;

            size_t delta_pages_main_cache = delta_pages_data_source_main_cache + delta_pages_pending_found_in_cache_at_pass4;
            if(delta_pages_main_cache > delta_pages_total)
                delta_pages_main_cache = delta_pages_total;

            main_cache_hit_ratio = delta_pages_main_cache * 100 * 10000 / delta_pages_total;
        }

        if(delta_pages_to_load_from_disk) {
            if(delta_pages_data_source_extent_cache > delta_pages_to_load_from_disk)
                delta_pages_data_source_extent_cache = delta_pages_to_load_from_disk;

            extent_cache_hit_ratio = delta_pages_data_source_extent_cache * 100 * 10000 / delta_pages_to_load_from_disk;

            if(delta_pages_load_extent_merged > delta_pages_to_load_from_disk)
                delta_pages_load_extent_merged = delta_pages_to_load_from_disk;

            extent_merge_hit_ratio = delta_pages_load_extent_merged * 100 * 10000 / delta_pages_to_load_from_disk;
        }

        rrddim_set_by_pointer(st_cache_hit_ratio, rd_hit_ratio, (collected_number)overall_hit_ratio);
        rrddim_set_by_pointer(st_cache_hit_ratio, rd_main_cache_hit_ratio, (collected_number)main_cache_hit_ratio);
        rrddim_set_by_pointer(st_cache_hit_ratio, rd_extent_cache_hit_ratio, (collected_number)extent_cache_hit_ratio);
        rrddim_set_by_pointer(st_cache_hit_ratio, rd_extent_merge_hit_ratio, (collected_number)extent_merge_hit_ratio);

        rrdset_done(st_cache_hit_ratio);
    }

    {
        static RRDSET *st_queries = NULL;
        static RRDDIM *rd_total = NULL;
        static RRDDIM *rd_open = NULL;
        static RRDDIM *rd_jv2 = NULL;
        static RRDDIM *rd_planned_with_gaps = NULL;
        static RRDDIM *rd_executed_with_gaps = NULL;

        if (unlikely(!st_queries)) {
            st_queries = rrdset_create_localhost(
                "netdata",
                "dbengine_queries",
                NULL,
                "dbengine query router",
                NULL,
                "Netdata Queries",
                "queries/s",
                "netdata",
                "pulse",
                priority,
                localhost->rrd_update_every,
                RRDSET_TYPE_LINE);

            rd_total = rrddim_add(st_queries, "total", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
            rd_open = rrddim_add(st_queries, "open cache", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
            rd_jv2 = rrddim_add(st_queries, "journal v2", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
            rd_planned_with_gaps = rrddim_add(st_queries, "planned with gaps", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
            rd_executed_with_gaps = rrddim_add(st_queries, "executed with gaps", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
        }
        priority++;

        rrddim_set_by_pointer(st_queries, rd_total, (collected_number)cache_efficiency_stats.prep_time_in_main_cache_lookup.count);
        rrddim_set_by_pointer(st_queries, rd_open, (collected_number)cache_efficiency_stats.prep_time_in_open_cache_lookup.count);
        rrddim_set_by_pointer(st_queries, rd_jv2, (collected_number)cache_efficiency_stats.prep_time_in_journal_v2_lookup.count);
        rrddim_set_by_pointer(st_queries, rd_planned_with_gaps, (collected_number)cache_efficiency_stats.queries_planned_with_gaps);
        rrddim_set_by_pointer(st_queries, rd_executed_with_gaps, (collected_number)cache_efficiency_stats.queries_executed_with_gaps);

        rrdset_done(st_queries);
    }

    {
        static RRDSET *st_queries_running = NULL;
        static RRDDIM *rd_queries = NULL;

        if (unlikely(!st_queries_running)) {
            st_queries_running = rrdset_create_localhost(
                "netdata",
                "dbengine_queries_running",
                NULL,
                "dbengine query router",
                NULL,
                "Netdata Queries Running",
                "queries",
                "netdata",
                "pulse",
                priority,
                localhost->rrd_update_every,
                RRDSET_TYPE_LINE);

            rd_queries = rrddim_add(st_queries_running, "queries", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
        }
        priority++;

        rrddim_set_by_pointer(st_queries_running, rd_queries, (collected_number)cache_efficiency_stats.currently_running_queries);

        rrdset_done(st_queries_running);
    }

    {
        static RRDSET *st_query_pages_metadata_source = NULL;
        static RRDDIM *rd_cache = NULL;
        static RRDDIM *rd_open = NULL;
        static RRDDIM *rd_jv2 = NULL;

        if (unlikely(!st_query_pages_metadata_source)) {
            st_query_pages_metadata_source = rrdset_create_localhost(
                "netdata",
                "dbengine_query_pages_metadata_source",
                NULL,
                "dbengine query router",
                NULL,
                "Netdata Query Pages Metadata Source",
                "pages/s",
                "netdata",
                "pulse",
                priority,
                localhost->rrd_update_every,
                RRDSET_TYPE_STACKED);

            rd_cache = rrddim_add(st_query_pages_metadata_source, "cache hit", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
            rd_jv2   = rrddim_add(st_query_pages_metadata_source, "journal v2 scan", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
            rd_open  = rrddim_add(st_query_pages_metadata_source, "open journal", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
        }
        priority++;

        rrddim_set_by_pointer(st_query_pages_metadata_source, rd_cache, (collected_number)cache_efficiency_stats.pages_meta_source_main_cache);
        rrddim_set_by_pointer(st_query_pages_metadata_source, rd_jv2, (collected_number)cache_efficiency_stats.pages_meta_source_journal_v2);
        rrddim_set_by_pointer(st_query_pages_metadata_source, rd_open, (collected_number)cache_efficiency_stats.pages_meta_source_open_cache);

        rrdset_done(st_query_pages_metadata_source);
    }

    {
        static RRDSET *st_query_pages_data_source = NULL;
        static RRDDIM *rd_pages_main_cache = NULL;
        static RRDDIM *rd_pages_disk = NULL;
        static RRDDIM *rd_pages_extent_cache = NULL;

        if (unlikely(!st_query_pages_data_source)) {
            st_query_pages_data_source = rrdset_create_localhost(
                "netdata",
                "dbengine_query_pages_data_source",
                NULL,
                "dbengine query router",
                NULL,
                "Netdata Query Pages to Data Source",
                "pages/s",
                "netdata",
                "pulse",
                priority,
                localhost->rrd_update_every,
                RRDSET_TYPE_STACKED);

            rd_pages_main_cache = rrddim_add(st_query_pages_data_source, "main cache", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
            rd_pages_disk = rrddim_add(st_query_pages_data_source, "disk", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
            rd_pages_extent_cache = rrddim_add(st_query_pages_data_source, "extent cache", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
        }
        priority++;

        rrddim_set_by_pointer(st_query_pages_data_source, rd_pages_main_cache, (collected_number)cache_efficiency_stats.pages_data_source_main_cache + (collected_number)cache_efficiency_stats.pages_data_source_main_cache_at_pass4);
        rrddim_set_by_pointer(st_query_pages_data_source, rd_pages_disk, (collected_number)cache_efficiency_stats.pages_to_load_from_disk);
        rrddim_set_by_pointer(st_query_pages_data_source, rd_pages_extent_cache, (collected_number)cache_efficiency_stats.pages_data_source_extent_cache);

        rrdset_done(st_query_pages_data_source);
    }

    {
        static RRDSET *st_query_next_page = NULL;
        static RRDDIM *rd_pass4 = NULL;
        static RRDDIM *rd_nowait_failed = NULL;
        static RRDDIM *rd_wait_failed = NULL;
        static RRDDIM *rd_wait_loaded = NULL;
        static RRDDIM *rd_nowait_loaded = NULL;

        if (unlikely(!st_query_next_page)) {
            st_query_next_page = rrdset_create_localhost(
                "netdata",
                "dbengine_query_next_page",
                NULL,
                "dbengine query router",
                NULL,
                "Netdata Query Next Page",
                "pages/s",
                "netdata",
                "pulse",
                priority,
                localhost->rrd_update_every,
                RRDSET_TYPE_STACKED);

            rd_pass4 = rrddim_add(st_query_next_page, "pass4", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
            rd_wait_failed = rrddim_add(st_query_next_page, "failed slow", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
            rd_nowait_failed = rrddim_add(st_query_next_page, "failed fast", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
            rd_wait_loaded = rrddim_add(st_query_next_page, "loaded slow", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
            rd_nowait_loaded = rrddim_add(st_query_next_page, "loaded fast", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
        }
        priority++;

        rrddim_set_by_pointer(st_query_next_page, rd_pass4, (collected_number)cache_efficiency_stats.pages_data_source_main_cache_at_pass4);
        rrddim_set_by_pointer(st_query_next_page, rd_wait_failed, (collected_number)cache_efficiency_stats.page_next_wait_failed);
        rrddim_set_by_pointer(st_query_next_page, rd_nowait_failed, (collected_number)cache_efficiency_stats.page_next_nowait_failed);
        rrddim_set_by_pointer(st_query_next_page, rd_wait_loaded, (collected_number)cache_efficiency_stats.page_next_wait_loaded);
        rrddim_set_by_pointer(st_query_next_page, rd_nowait_loaded, (collected_number)cache_efficiency_stats.page_next_nowait_loaded);

        rrdset_done(st_query_next_page);
    }

    {
        static RRDSET *st_query_page_issues = NULL;
        static RRDDIM *rd_pages_zero_time = NULL;
        static RRDDIM *rd_pages_past_time = NULL;
        static RRDDIM *rd_pages_invalid_size = NULL;
        static RRDDIM *rd_pages_fixed_update_every = NULL;
        static RRDDIM *rd_pages_fixed_entries = NULL;
        static RRDDIM *rd_pages_overlapping = NULL;

        if (unlikely(!st_query_page_issues)) {
            st_query_page_issues = rrdset_create_localhost(
                "netdata",
                "dbengine_query_next_page_issues",
                NULL,
                "dbengine query router",
                NULL,
                "Netdata Query Next Page Issues",
                "pages/s",
                "netdata",
                "pulse",
                priority,
                localhost->rrd_update_every,
                RRDSET_TYPE_STACKED);

            rd_pages_zero_time = rrddim_add(st_query_page_issues, "zero timestamp", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
            rd_pages_invalid_size = rrddim_add(st_query_page_issues, "invalid size", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
            rd_pages_past_time = rrddim_add(st_query_page_issues, "past time", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
            rd_pages_overlapping = rrddim_add(st_query_page_issues, "overlapping", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
            rd_pages_fixed_update_every = rrddim_add(st_query_page_issues, "update every fixed", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
            rd_pages_fixed_entries = rrddim_add(st_query_page_issues, "entries fixed", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
        }
        priority++;

        rrddim_set_by_pointer(st_query_page_issues, rd_pages_zero_time, (collected_number)cache_efficiency_stats.pages_zero_time_skipped);
        rrddim_set_by_pointer(st_query_page_issues, rd_pages_invalid_size, (collected_number)cache_efficiency_stats.pages_invalid_size_skipped);
        rrddim_set_by_pointer(st_query_page_issues, rd_pages_past_time, (collected_number)cache_efficiency_stats.pages_past_time_skipped);
        rrddim_set_by_pointer(st_query_page_issues, rd_pages_overlapping, (collected_number)cache_efficiency_stats.pages_overlapping_skipped);
        rrddim_set_by_pointer(st_query_page_issues, rd_pages_fixed_update_every, (collected_number)cache_efficiency_stats.pages_invalid_update_every_fixed);
        rrddim_set_by_pointer(st_query_page_issues, rd_pages_fixed_entries, (collected_number)cache_efficiency_stats.pages_invalid_entries_fixed);

        rrdset_done(st_query_page_issues);
    }

    {
        static RRDSET *st_query_pages_from_disk = NULL;
        static RRDDIM *rd_compressed = NULL;
        static RRDDIM *rd_invalid = NULL;
        static RRDDIM *rd_uncompressed = NULL;
        static RRDDIM *rd_mmap_failed = NULL;
        static RRDDIM *rd_unavailable = NULL;
        static RRDDIM *rd_unroutable = NULL;
        static RRDDIM *rd_not_found = NULL;
        static RRDDIM *rd_cancelled = NULL;
        static RRDDIM *rd_invalid_extent = NULL;
        static RRDDIM *rd_extent_merged = NULL;

        if (unlikely(!st_query_pages_from_disk)) {
            st_query_pages_from_disk = rrdset_create_localhost(
                "netdata",
                "dbengine_query_pages_disk_load",
                NULL,
                "dbengine query router",
                NULL,
                "Netdata Query Pages Loaded from Disk",
                "pages/s",
                "netdata",
                "pulse",
                priority,
                localhost->rrd_update_every,
                RRDSET_TYPE_LINE);

            rd_compressed = rrddim_add(st_query_pages_from_disk, "ok compressed", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
            rd_invalid = rrddim_add(st_query_pages_from_disk, "fail invalid page", NULL, -1, 1, RRD_ALGORITHM_INCREMENTAL);
            rd_uncompressed = rrddim_add(st_query_pages_from_disk, "ok uncompressed", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
            rd_mmap_failed = rrddim_add(st_query_pages_from_disk, "fail cant mmap", NULL, -1, 1, RRD_ALGORITHM_INCREMENTAL);
            rd_unavailable = rrddim_add(st_query_pages_from_disk, "fail unavailable", NULL, -1, 1, RRD_ALGORITHM_INCREMENTAL);
            rd_unroutable = rrddim_add(st_query_pages_from_disk, "fail unroutable", NULL, -1, 1, RRD_ALGORITHM_INCREMENTAL);
            rd_not_found = rrddim_add(st_query_pages_from_disk, "fail not found", NULL, -1, 1, RRD_ALGORITHM_INCREMENTAL);
            rd_invalid_extent = rrddim_add(st_query_pages_from_disk, "fail invalid extent", NULL, -1, 1, RRD_ALGORITHM_INCREMENTAL);
            rd_extent_merged = rrddim_add(st_query_pages_from_disk, "extent merged", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
            rd_cancelled = rrddim_add(st_query_pages_from_disk, "cancelled", NULL, -1, 1, RRD_ALGORITHM_INCREMENTAL);
        }
        priority++;

        rrddim_set_by_pointer(st_query_pages_from_disk, rd_compressed, (collected_number)cache_efficiency_stats.pages_load_ok_compressed);
        rrddim_set_by_pointer(st_query_pages_from_disk, rd_invalid, (collected_number)cache_efficiency_stats.pages_load_fail_invalid_page_in_extent);
        rrddim_set_by_pointer(st_query_pages_from_disk, rd_uncompressed, (collected_number)cache_efficiency_stats.pages_load_ok_uncompressed);
        rrddim_set_by_pointer(st_query_pages_from_disk, rd_mmap_failed, (collected_number)cache_efficiency_stats.pages_load_fail_cant_mmap_extent);
        rrddim_set_by_pointer(st_query_pages_from_disk, rd_unavailable, (collected_number)cache_efficiency_stats.pages_load_fail_datafile_not_available);
        rrddim_set_by_pointer(st_query_pages_from_disk, rd_unroutable, (collected_number)cache_efficiency_stats.pages_load_fail_unroutable);
        rrddim_set_by_pointer(st_query_pages_from_disk, rd_not_found, (collected_number)cache_efficiency_stats.pages_load_fail_not_found);
        rrddim_set_by_pointer(st_query_pages_from_disk, rd_cancelled, (collected_number)cache_efficiency_stats.pages_load_fail_cancelled);
        rrddim_set_by_pointer(st_query_pages_from_disk, rd_invalid_extent, (collected_number)cache_efficiency_stats.pages_load_fail_invalid_extent);
        rrddim_set_by_pointer(st_query_pages_from_disk, rd_extent_merged, (collected_number)cache_efficiency_stats.pages_load_extent_merged);

        rrdset_done(st_query_pages_from_disk);
    }

    {
        static RRDSET *st_events = NULL;
        static RRDDIM *rd_journal_v2_mapped = NULL;
        static RRDDIM *rd_journal_v2_unmapped = NULL;
        static RRDDIM *rd_datafile_creation = NULL;
        static RRDDIM *rd_datafile_deletion = NULL;
        static RRDDIM *rd_datafile_deletion_spin = NULL;
        static RRDDIM *rd_jv2_indexing = NULL;
        static RRDDIM *rd_retention = NULL;

        if (unlikely(!st_events)) {
            st_events = rrdset_create_localhost(
                "netdata",
                "dbengine_events",
                NULL,
                "dbengine query router",
                NULL,
                "Netdata Database Events",
                "events/s",
                "netdata",
                "pulse",
                priority,
                localhost->rrd_update_every,
                RRDSET_TYPE_LINE);

            rd_journal_v2_mapped = rrddim_add(st_events, "journal v2 mapped", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
            rd_journal_v2_unmapped = rrddim_add(st_events, "journal v2 unmapped", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
            rd_datafile_creation = rrddim_add(st_events, "datafile creation", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
            rd_datafile_deletion = rrddim_add(st_events, "datafile deletion", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
            rd_datafile_deletion_spin = rrddim_add(st_events, "datafile deletion spin", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
            rd_jv2_indexing = rrddim_add(st_events, "journal v2 indexing", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
            rd_retention = rrddim_add(st_events, "retention", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
        }
        priority++;

        rrddim_set_by_pointer(st_events, rd_journal_v2_mapped, (collected_number)cache_efficiency_stats.journal_v2_mapped);
        rrddim_set_by_pointer(st_events, rd_journal_v2_unmapped, (collected_number)cache_efficiency_stats.journal_v2_unmapped);
        rrddim_set_by_pointer(st_events, rd_datafile_creation, (collected_number)cache_efficiency_stats.datafile_creation_started);
        rrddim_set_by_pointer(st_events, rd_datafile_deletion, (collected_number)cache_efficiency_stats.datafile_deletion_started);
        rrddim_set_by_pointer(st_events, rd_datafile_deletion_spin, (collected_number)cache_efficiency_stats.datafile_deletion_spin);
        rrddim_set_by_pointer(st_events, rd_jv2_indexing, (collected_number)cache_efficiency_stats.journal_v2_indexing_started);
        rrddim_set_by_pointer(st_events, rd_retention, (collected_number)cache_efficiency_stats.metrics_retention_started);

        rrdset_done(st_events);
    }

    {
        static RRDSET *st_prep_timings = NULL;
        static RRDDIM *rd_routing_sync = NULL;
        static RRDDIM *rd_routing_syncfirst = NULL;
        static RRDDIM *rd_routing_async = NULL;
        static RRDDIM *rd_main_cache = NULL;
        static RRDDIM *rd_open_cache = NULL;
        static RRDDIM *rd_journal_v2 = NULL;
        static RRDDIM *rd_pass4 = NULL;

        if (unlikely(!st_prep_timings)) {
            st_prep_timings = rrdset_create_localhost(
                "netdata",
                "dbengine_prep_timings",
                NULL,
                "dbengine query router",
                NULL,
                "Netdata Query Planning Timings",
                "usec/s",
                "netdata",
                "pulse",
                priority,
                localhost->rrd_update_every,
                RRDSET_TYPE_STACKED);

            rd_routing_sync = rrddim_add(st_prep_timings, "pdc sync", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
            rd_routing_syncfirst = rrddim_add(st_prep_timings, "pdc syncfirst", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
            rd_routing_async = rrddim_add(st_prep_timings, "pdc async", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
            rd_main_cache = rrddim_add(st_prep_timings, "main cache", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
            rd_open_cache = rrddim_add(st_prep_timings, "open cache", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
            rd_journal_v2 = rrddim_add(st_prep_timings, "journal v2", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
            rd_pass4 = rrddim_add(st_prep_timings, "pass4", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
        }
        priority++;

        rrddim_set_by_pointer(st_prep_timings, rd_routing_sync, (collected_number)cache_efficiency_stats.prep_time_to_route_sync.usec);
        rrddim_set_by_pointer(st_prep_timings, rd_routing_syncfirst, (collected_number)cache_efficiency_stats.prep_time_to_route_syncfirst.usec);
        rrddim_set_by_pointer(st_prep_timings, rd_routing_async, (collected_number)cache_efficiency_stats.prep_time_to_route_async.usec);
        rrddim_set_by_pointer(st_prep_timings, rd_main_cache, (collected_number)cache_efficiency_stats.prep_time_in_main_cache_lookup.usec);
        rrddim_set_by_pointer(st_prep_timings, rd_open_cache, (collected_number)cache_efficiency_stats.prep_time_in_open_cache_lookup.usec);
        rrddim_set_by_pointer(st_prep_timings, rd_journal_v2, (collected_number)cache_efficiency_stats.prep_time_in_journal_v2_lookup.usec);
        rrddim_set_by_pointer(st_prep_timings, rd_pass4, (collected_number)cache_efficiency_stats.prep_time_in_pass4_lookup.usec);

        rrdset_done(st_prep_timings);
    }

    {
        static RRDSET *st_prep_timings = NULL;
        static RRDDIM *rd_routing_sync = NULL;
        static RRDDIM *rd_routing_syncfirst = NULL;
        static RRDDIM *rd_routing_async = NULL;
        static RRDDIM *rd_main_cache = NULL;
        static RRDDIM *rd_open_cache = NULL;
        static RRDDIM *rd_journal_v2 = NULL;
        static RRDDIM *rd_pass4 = NULL;

        if (unlikely(!st_prep_timings)) {
            st_prep_timings = rrdset_create_localhost(
                "netdata",
                "dbengine_prep_average_timings",
                NULL,
                "dbengine query router",
                NULL,
                "Netdata Query Planning Average Timings",
                "usec/s",
                "netdata",
                "pulse",
                priority + 1,
                localhost->rrd_update_every,
                RRDSET_TYPE_STACKED);

            rd_routing_sync = rrddim_add(st_prep_timings, "pdc sync", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
            rd_routing_syncfirst = rrddim_add(st_prep_timings, "pdc syncfirst", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
            rd_routing_async = rrddim_add(st_prep_timings, "pdc async", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
            rd_main_cache = rrddim_add(st_prep_timings, "main cache", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
            rd_open_cache = rrddim_add(st_prep_timings, "open cache", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
            rd_journal_v2 = rrddim_add(st_prep_timings, "journal v2", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
            rd_pass4 = rrddim_add(st_prep_timings, "pass4", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
        }
        priority++;

        rrddim_set_by_pointer(st_prep_timings,rd_routing_sync, (collected_number)time_and_count_delta_average(&cache_efficiency_stats_old.prep_time_to_route_sync, &cache_efficiency_stats.prep_time_to_route_sync));
        rrddim_set_by_pointer(st_prep_timings,rd_routing_syncfirst, (collected_number)time_and_count_delta_average(&cache_efficiency_stats_old.prep_time_to_route_syncfirst, &cache_efficiency_stats.prep_time_to_route_syncfirst));
        rrddim_set_by_pointer(st_prep_timings,rd_routing_async, (collected_number)time_and_count_delta_average(&cache_efficiency_stats_old.prep_time_to_route_async, &cache_efficiency_stats.prep_time_to_route_async));
        rrddim_set_by_pointer(st_prep_timings, rd_main_cache, (collected_number)time_and_count_delta_average(&cache_efficiency_stats_old.prep_time_in_main_cache_lookup, &cache_efficiency_stats.prep_time_in_main_cache_lookup));
        rrddim_set_by_pointer(st_prep_timings, rd_open_cache, (collected_number)time_and_count_delta_average(&cache_efficiency_stats_old.prep_time_in_open_cache_lookup, &cache_efficiency_stats.prep_time_in_open_cache_lookup));
        rrddim_set_by_pointer(st_prep_timings, rd_journal_v2, (collected_number)time_and_count_delta_average(&cache_efficiency_stats_old.prep_time_in_journal_v2_lookup, &cache_efficiency_stats.prep_time_in_journal_v2_lookup));
        rrddim_set_by_pointer(st_prep_timings, rd_pass4, (collected_number)time_and_count_delta_average(&cache_efficiency_stats_old.prep_time_in_pass4_lookup, &cache_efficiency_stats.prep_time_in_pass4_lookup));

        rrdset_done(st_prep_timings);
    }

    {
        static RRDSET *st_query_timings = NULL;
        static RRDDIM *rd_init = NULL;
        static RRDDIM *rd_prep_wait = NULL;
        static RRDDIM *rd_next_page_disk_fast = NULL;
        static RRDDIM *rd_next_page_disk_slow = NULL;
        static RRDDIM *rd_next_page_preload_fast = NULL;
        static RRDDIM *rd_next_page_preload_slow = NULL;

        if (unlikely(!st_query_timings)) {
            st_query_timings = rrdset_create_localhost(
                "netdata",
                "dbengine_query_timings",
                NULL,
                "dbengine query router",
                NULL,
                "Netdata Query Timings",
                "usec/s",
                "netdata",
                "pulse",
                priority,
                localhost->rrd_update_every,
                RRDSET_TYPE_STACKED);

            rd_init = rrddim_add(st_query_timings, "plan", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
            rd_prep_wait = rrddim_add(st_query_timings, "async wait", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
            rd_next_page_disk_fast = rrddim_add(st_query_timings, "next page disk fast", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
            rd_next_page_disk_slow = rrddim_add(st_query_timings, "next page disk slow", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
            rd_next_page_preload_fast = rrddim_add(st_query_timings, "next page preload fast", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
            rd_next_page_preload_slow = rrddim_add(st_query_timings, "next page preload slow", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
        }
        priority++;

        rrddim_set_by_pointer(st_query_timings, rd_init, (collected_number)cache_efficiency_stats.query_time_init.usec);
        rrddim_set_by_pointer(st_query_timings, rd_prep_wait, (collected_number)cache_efficiency_stats.query_time_wait_for_prep.usec);
        rrddim_set_by_pointer(st_query_timings, rd_next_page_disk_fast, (collected_number)cache_efficiency_stats.query_time_to_fast_disk_next_page.usec);
        rrddim_set_by_pointer(st_query_timings, rd_next_page_disk_slow, (collected_number)cache_efficiency_stats.query_time_to_slow_disk_next_page.usec);
        rrddim_set_by_pointer(st_query_timings, rd_next_page_preload_fast, (collected_number)cache_efficiency_stats.query_time_to_fast_preload_next_page.usec);
        rrddim_set_by_pointer(st_query_timings, rd_next_page_preload_slow, (collected_number)cache_efficiency_stats.query_time_to_slow_preload_next_page.usec);

        rrdset_done(st_query_timings);
    }

    {
        static RRDSET *st_query_timings_average = NULL;
        static RRDDIM *rd_init = NULL;
        static RRDDIM *rd_prep_wait = NULL;
        static RRDDIM *rd_next_page_disk_fast = NULL;
        static RRDDIM *rd_next_page_disk_slow = NULL;
        static RRDDIM *rd_next_page_preload_fast = NULL;
        static RRDDIM *rd_next_page_preload_slow = NULL;

        if (unlikely(!st_query_timings_average)) {
            st_query_timings_average = rrdset_create_localhost(
                "netdata",
                "dbengine_query_timings_average",
                NULL,
                "dbengine query router",
                NULL,
                "Netdata Query Average Timings",
                "usec/s",
                "netdata",
                "pulse",
                priority + 1,
                localhost->rrd_update_every,
                RRDSET_TYPE_STACKED);

            rd_init = rrddim_add(st_query_timings_average, "plan", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
            rd_prep_wait = rrddim_add(st_query_timings_average, "async wait", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
            rd_next_page_disk_fast = rrddim_add(st_query_timings_average, "next page disk fast", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
            rd_next_page_disk_slow = rrddim_add(st_query_timings_average, "next page disk slow", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
            rd_next_page_preload_fast = rrddim_add(st_query_timings_average, "next page preload fast", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
            rd_next_page_preload_slow = rrddim_add(st_query_timings_average, "next page preload slow", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
        }
        priority++;

        rrddim_set_by_pointer(st_query_timings_average, rd_init, (collected_number)time_and_count_delta_average(&cache_efficiency_stats_old.query_time_init, &cache_efficiency_stats.query_time_init));
        rrddim_set_by_pointer(st_query_timings_average, rd_prep_wait, (collected_number)time_and_count_delta_average(&cache_efficiency_stats_old.query_time_wait_for_prep, &cache_efficiency_stats.query_time_wait_for_prep));
        rrddim_set_by_pointer(st_query_timings_average, rd_next_page_disk_fast, (collected_number)time_and_count_delta_average(&cache_efficiency_stats_old.query_time_to_fast_disk_next_page, &cache_efficiency_stats.query_time_to_fast_disk_next_page));
        rrddim_set_by_pointer(st_query_timings_average, rd_next_page_disk_slow, (collected_number)time_and_count_delta_average(&cache_efficiency_stats_old.query_time_to_slow_disk_next_page, &cache_efficiency_stats.query_time_to_slow_disk_next_page));
        rrddim_set_by_pointer(st_query_timings_average, rd_next_page_preload_fast, (collected_number)time_and_count_delta_average(&cache_efficiency_stats_old.query_time_to_fast_preload_next_page, &cache_efficiency_stats.query_time_to_fast_preload_next_page));
        rrddim_set_by_pointer(st_query_timings_average, rd_next_page_preload_slow, (collected_number)time_and_count_delta_average(&cache_efficiency_stats_old.query_time_to_slow_preload_next_page, &cache_efficiency_stats.query_time_to_slow_preload_next_page));

        rrdset_done(st_query_timings_average);
    }

    if(netdata_rwlock_tryrdlock(&rrd_rwlock) == 0) {
        priority = 135400;

        RRDHOST *host;
        unsigned long long stats_array[RRDENG_NR_STATS] = {0};
        unsigned long long local_stats_array[RRDENG_NR_STATS];
        unsigned dbengine_contexts = 0, counted_multihost_db[RRD_STORAGE_TIERS] = { 0 }, i;

        rrdhost_foreach_read(host) {
            if (!rrdhost_flag_check(host, RRDHOST_FLAG_ARCHIVED)) {

                /* get localhost's DB engine's statistics for each tier */
                for(size_t tier = 0; tier < nd_profile.storage_tiers;tier++) {
                    if(host->db[tier].mode != RRD_DB_MODE_DBENGINE) continue;
                    if(!host->db[tier].si) continue;

                    if(counted_multihost_db[tier])
                        continue;
                    else
                        counted_multihost_db[tier] = 1;

                    ++dbengine_contexts;
                    rrdeng_get_37_statistics((struct rrdengine_instance *)host->db[tier].si, local_stats_array);
                    for (i = 0; i < RRDENG_NR_STATS; ++i) {
                        /* aggregate statistics across hosts */
                        stats_array[i] += local_stats_array[i];
                    }
                }
            }
        }
        rrd_rdunlock();

        if (dbengine_contexts) {
            /* deduplicate by getting the ones from the last context */
            stats_array[30] = local_stats_array[30];
            stats_array[31] = local_stats_array[31];
            stats_array[32] = local_stats_array[32];
            stats_array[34] = local_stats_array[34];
            stats_array[36] = local_stats_array[36];

            // ----------------------------------------------------------------

            {
                static RRDSET *st_compression = NULL;
                static RRDDIM *rd_savings = NULL;

                if (unlikely(!st_compression)) {
                    st_compression = rrdset_create_localhost(
                        "netdata",
                        "dbengine_compression_ratio",
                        NULL,
                        "dbengine io",
                        NULL,
                        "Netdata DB engine data extents' compression savings ratio",
                        "percentage",
                        "netdata",
                        "pulse",
                        priority,
                        localhost->rrd_update_every,
                        RRDSET_TYPE_LINE);

                    rd_savings = rrddim_add(st_compression, "savings", NULL, 1, 1000, RRD_ALGORITHM_ABSOLUTE);
                }
                priority++;

                unsigned long long ratio;
                unsigned long long compressed_content_size = stats_array[12];
                unsigned long long content_size = stats_array[11];

                if (content_size) {
                    // allow negative savings
                    ratio = ((content_size - compressed_content_size) * 100 * 1000) / content_size;
                } else {
                    ratio = 0;
                }
                rrddim_set_by_pointer(st_compression, rd_savings, ratio);

                rrdset_done(st_compression);
            }

            // ----------------------------------------------------------------

            {
                static RRDSET *st_io_stats = NULL;
                static RRDDIM *rd_reads = NULL;
                static RRDDIM *rd_writes = NULL;

                if (unlikely(!st_io_stats)) {
                    st_io_stats = rrdset_create_localhost(
                        "netdata",
                        "dbengine_io_throughput",
                        NULL,
                        "dbengine io",
                        NULL,
                        "Netdata DB engine I/O throughput",
                        "MiB/s",
                        "netdata",
                        "pulse",
                        priority,
                        localhost->rrd_update_every,
                        RRDSET_TYPE_LINE);

                    rd_reads = rrddim_add(st_io_stats, "reads", NULL, 1, 1024 * 1024, RRD_ALGORITHM_INCREMENTAL);
                    rd_writes = rrddim_add(st_io_stats, "writes", NULL, -1, 1024 * 1024, RRD_ALGORITHM_INCREMENTAL);
                }
                priority++;

                rrddim_set_by_pointer(st_io_stats, rd_reads, (collected_number)stats_array[17]);
                rrddim_set_by_pointer(st_io_stats, rd_writes, (collected_number)stats_array[15]);
                rrdset_done(st_io_stats);
            }

            // ----------------------------------------------------------------

            {
                static RRDSET *st_io_stats = NULL;
                static RRDDIM *rd_reads = NULL;
                static RRDDIM *rd_writes = NULL;

                if (unlikely(!st_io_stats)) {
                    st_io_stats = rrdset_create_localhost(
                        "netdata",
                        "dbengine_io_operations",
                        NULL,
                        "dbengine io",
                        NULL,
                        "Netdata DB engine I/O operations",
                        "operations/s",
                        "netdata",
                        "pulse",
                        priority,
                        localhost->rrd_update_every,
                        RRDSET_TYPE_LINE);

                    rd_reads = rrddim_add(st_io_stats, "reads", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
                    rd_writes = rrddim_add(st_io_stats, "writes", NULL, -1, 1, RRD_ALGORITHM_INCREMENTAL);
                }
                priority++;

                rrddim_set_by_pointer(st_io_stats, rd_reads, (collected_number)stats_array[18]);
                rrddim_set_by_pointer(st_io_stats, rd_writes, (collected_number)stats_array[16]);
                rrdset_done(st_io_stats);
            }

            // ----------------------------------------------------------------

            {
                static RRDSET *st_errors = NULL;
                static RRDDIM *rd_fs_errors = NULL;
                static RRDDIM *rd_io_errors = NULL;
                static RRDDIM *pg_cache_over_half_dirty_events = NULL;

                if (unlikely(!st_errors)) {
                    st_errors = rrdset_create_localhost(
                        "netdata",
                        "dbengine_global_errors",
                        NULL,
                        "dbengine io",
                        NULL,
                        "Netdata DB engine errors",
                        "errors/s",
                        "netdata",
                        "pulse",
                        priority,
                        localhost->rrd_update_every,
                        RRDSET_TYPE_LINE);

                    rd_io_errors = rrddim_add(st_errors, "io_errors", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
                    rd_fs_errors = rrddim_add(st_errors, "fs_errors", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
                    pg_cache_over_half_dirty_events =
                        rrddim_add(st_errors, "pg_cache_over_half_dirty_events", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
                }
                priority++;

                rrddim_set_by_pointer(st_errors, rd_io_errors, (collected_number)stats_array[30]);
                rrddim_set_by_pointer(st_errors, rd_fs_errors, (collected_number)stats_array[31]);
                rrddim_set_by_pointer(st_errors, pg_cache_over_half_dirty_events, (collected_number)stats_array[34]);
                rrdset_done(st_errors);
            }

            // ----------------------------------------------------------------

            {
                static RRDSET *st_fd = NULL;
                static RRDDIM *rd_fd_current = NULL;
                static RRDDIM *rd_fd_max = NULL;

                if (unlikely(!st_fd)) {
                    st_fd = rrdset_create_localhost(
                        "netdata",
                        "dbengine_global_file_descriptors",
                        NULL,
                        "dbengine io",
                        NULL,
                        "Netdata DB engine File Descriptors",
                        "descriptors",
                        "netdata",
                        "pulse",
                        priority,
                        localhost->rrd_update_every,
                        RRDSET_TYPE_LINE);

                    rd_fd_current = rrddim_add(st_fd, "current", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
                    rd_fd_max = rrddim_add(st_fd, "max", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
                }
                priority++;

                rrddim_set_by_pointer(st_fd, rd_fd_current, (collected_number)stats_array[32]);
                /* Careful here, modify this accordingly if the File-Descriptor budget ever changes */
                rrddim_set_by_pointer(st_fd, rd_fd_max, (collected_number)rlimit_nofile.rlim_cur / 4);
                rrdset_done(st_fd);
            }
        }
    }
}

#endif
