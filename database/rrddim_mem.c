#include "rrddim_mem.h"

RRDSET* rrdset_init(RRD_MEMORY_MODE memory_mode, const char *id, const char *fullid, const char *filename, long entries, int update_every)
{
    size_t size = sizeof(RRDSET);

    RRDSET *st = (RRDSET *) mymmap(
                (memory_mode == RRD_MEMORY_MODE_RAM) ? NULL : filename
            , size
            , ((memory_mode == RRD_MEMORY_MODE_MAP) ? MAP_SHARED : MAP_PRIVATE)
            , 0
    );

    if(st) {
        memset(&st->avl, 0, sizeof(avl_t));
        memset(&st->avlname, 0, sizeof(avl_t));
        memset(&st->rrdvar_root_index, 0, sizeof(avl_tree_lock));
        memset(&st->dimensions_index, 0, sizeof(avl_tree_lock));
        memset(&st->rrdset_rwlock, 0, sizeof(netdata_rwlock_t));

        st->name = NULL;
        st->config_section = NULL;
        st->type = NULL;
        st->family = NULL;
        st->title = NULL;
        st->units = NULL;
        st->context = NULL;
        st->cache_dir = NULL;
        st->plugin_name = NULL;
        st->module_name = NULL;
        st->dimensions = NULL;
        st->rrdfamily = NULL;
        st->rrdhost = NULL;
        st->next = NULL;
        st->variables = NULL;
        st->alarms = NULL;
        st->flags = 0x00000000;
        st->exporting_flags = NULL;

        if(memory_mode == RRD_MEMORY_MODE_RAM) {
            memset(st, 0, size);
        }
        else {
            time_t now = now_realtime_sec();

            if(strcmp(st->magic, RRDSET_MAGIC) != 0) {
                info("Initializing file %s.", filename);
                memset(st, 0, size);
            }
            else if(strcmp(st->id, fullid) != 0) {
                error("File %s contents are not for chart %s. Clearing it.", filename, fullid);
                // munmap(st, size);
                // st = NULL;
                memset(st, 0, size);
            }
            else if(st->memsize != size || st->entries != entries) {
                error("File %s does not have the desired size. Clearing it.", filename);
                memset(st, 0, size);
            }
            else if(st->update_every != update_every) {
                error("File %s does not have the desired update frequency. Clearing it.", filename);
                memset(st, 0, size);
            }
            else if((now - st->last_updated.tv_sec) > update_every * entries) {
                info("File %s is too old. Clearing it.", filename);
                memset(st, 0, size);
            }
            else if(st->last_updated.tv_sec > now + update_every) {
                error("File %s refers to the future by %zd secs. Resetting it to now.", filename, (ssize_t)(st->last_updated.tv_sec - now));
                st->last_updated.tv_sec = now;
            }

            // make sure the database is aligned
            if(st->last_updated.tv_sec) {
                st->update_every = update_every;
                last_updated_time_align(st);
            }
        }

        // make sure we have the right memory mode
        // even if we cleared the memory
        st->rrd_memory_mode = memory_mode;
    }
    return st;
}


RRDSET* rrdset_init_map(const char *id, const char *fullid, const char *filename, long entries, int update_every)
{
    return rrdset_init(RRD_MEMORY_MODE_MAP, id, fullid, filename, entries, update_every);
}

RRDSET* rrdset_init_ram(const char *id, const char *fullid, const char *filename, long entries, int update_every)
{
    return rrdset_init(RRD_MEMORY_MODE_RAM, id, fullid, filename, entries, update_every);
}

RRDSET* rrdset_init_save(const char *id, const char *fullid, const char *filename, long entries, int update_every)
{
    return rrdset_init(RRD_MEMORY_MODE_SAVE, id, fullid, filename, entries, update_every);
}


RRDDIM* rrddim_init(RRDSET *st, RRD_MEMORY_MODE memory_mode, const char* filename, int map_mode, collected_number multiplier,
                          collected_number divisor, RRD_ALGORITHM algorithm)
{
    unsigned long size = sizeof(RRDDIM) + (st->entries * sizeof(storage_number));
    RRDDIM* rd = (RRDDIM *)mymmap(filename, size, map_mode, 1);

    if(likely(rd)) {
        // we have a file mapped for rd

        memset(&rd->avl, 0, sizeof(avl_t));
        rd->id = NULL;
        rd->name = NULL;
        rd->cache_filename = NULL;
        rd->variables = NULL;
        rd->next = NULL;
        rd->rrdset = NULL;
        rd->exposed = 0;

        struct timeval now;
        now_realtime_timeval(&now);

        if(memory_mode == RRD_MEMORY_MODE_RAM) {
            memset(rd, 0, size);
        }
        else {
            int reset = 0;

            if(strcmp(rd->magic, RRDDIMENSION_MAGIC) != 0) {
                info("Initializing file %s.", filename);
                memset(rd, 0, size);
                reset = 1;
            }
            else if(rd->memsize != size) {
                error("File %s does not have the desired size, expected %lu but found %lu. Clearing it.", filename, size, rd->memsize);
                memset(rd, 0, size);
                reset = 1;
            }
            else if(rd->update_every != st->update_every) {
                error("File %s does not have the same update frequency, expected %d but found %d. Clearing it.", filename, st->update_every, rd->update_every);
                memset(rd, 0, size);
                reset = 1;
            }
            else if(dt_usec(&now, &rd->last_collected_time) > (rd->entries * rd->update_every * USEC_PER_SEC)) {
                info("File %s is too old (last collected %llu seconds ago, but the database is %ld seconds). Clearing it.", filename, dt_usec(&now, &rd->last_collected_time) / USEC_PER_SEC, rd->entries * rd->update_every);
                memset(rd, 0, size);
                reset = 1;
            }

            if(!reset) {
                if(rd->algorithm != algorithm) {
                    info("File %s does not have the expected algorithm (expected %u '%s', found %u '%s'). Previous values may be wrong.",
                            filename, algorithm, rrd_algorithm_name(algorithm), rd->algorithm, rrd_algorithm_name(rd->algorithm));
                }

                if(rd->multiplier != multiplier) {
                    info("File %s does not have the expected multiplier (expected " COLLECTED_NUMBER_FORMAT ", found " COLLECTED_NUMBER_FORMAT "). Previous values may be wrong.", filename, multiplier, rd->multiplier);
                }

                if(rd->divisor != divisor) {
                    info("File %s does not have the expected divisor (expected " COLLECTED_NUMBER_FORMAT ", found " COLLECTED_NUMBER_FORMAT "). Previous values may be wrong.", filename, divisor, rd->divisor);
                }
            }
        }

        // make sure we have the right memory mode
        // even if we cleared the memory
        rd->rrd_memory_mode = memory_mode;
    }
    return rd;
}

RRDDIM* rrddim_init_map(RRDSET *st, const char *id, const char *filename, collected_number multiplier, collected_number divisor, RRD_ALGORITHM algorithm)
{
    return rrddim_init(st, RRD_MEMORY_MODE_MAP, filename, MAP_SHARED, multiplier, divisor, algorithm);
}
RRDDIM* rrddim_init_ram(RRDSET *st, const char *id, const char *filename, collected_number multiplier, collected_number divisor, RRD_ALGORITHM algorithm)
{
    return rrddim_init(st, RRD_MEMORY_MODE_RAM, NULL, MAP_PRIVATE, multiplier, divisor, algorithm);
}
RRDDIM* rrddim_init_save(RRDSET *st, const char *id, const char *filename, collected_number multiplier, collected_number divisor, RRD_ALGORITHM algorithm)
{
    return rrddim_init(st, RRD_MEMORY_MODE_SAVE, filename, MAP_PRIVATE, multiplier, divisor, algorithm);
}

// ----------------------------------------------------------------------------
// RRDDIM legacy data collection functions

void rrddim_collect_init(RRDDIM *rd) {
    rd->values[rd->rrdset->current_entry] = SN_EMPTY_SLOT;
    rd->state->handle = calloc(1, sizeof(struct mem_collect_handle));
}
void rrddim_collect_store_metric(RRDDIM *rd, usec_t point_in_time, storage_number number) {
    (void)point_in_time;
    rd->values[rd->rrdset->current_entry] = number;
}
int rrddim_collect_finalize(RRDDIM *rd) {
    free((struct mem_collect_handle*)rd->state->handle);
    return 0;
}

// ----------------------------------------------------------------------------
// RRDDIM legacy database query functions

void rrddim_query_init(RRDDIM *rd, struct rrddim_query_handle *handle, time_t start_time, time_t end_time) {
    handle->rd = rd;
    handle->start_time = start_time;
    handle->end_time = end_time;
    struct mem_query_handle* h = calloc(1, sizeof(struct mem_query_handle));
    h->slot = rrdset_time2slot(rd->rrdset, start_time);
    h->last_slot = rrdset_time2slot(rd->rrdset, end_time);
    h->finished = 0;
    handle->handle = (STORAGE_QUERY_HANDLE *)h;
}

storage_number rrddim_query_next_metric(struct rrddim_query_handle *handle, time_t *current_time) {
    RRDDIM *rd = handle->rd;
    struct mem_query_handle* h = (struct mem_query_handle*)handle->handle;
    long entries = rd->rrdset->entries;
    long slot = h->slot;

    (void)current_time;
    if (unlikely(h->slot == h->last_slot))
        h->finished = 1;
    storage_number n = rd->values[slot++];

    if(unlikely(slot >= entries)) slot = 0;
    h->slot = slot;

    return n;
}

int rrddim_query_is_finished(struct rrddim_query_handle *handle) {
    struct mem_query_handle* h = (struct mem_query_handle*)handle->handle;
    return h->finished;
}

void rrddim_query_finalize(struct rrddim_query_handle *handle) {
    freez(handle->handle);
}

time_t rrddim_query_latest_time(RRDDIM *rd) {
    return rrdset_last_entry_t_nolock(rd->rrdset);
}

time_t rrddim_query_oldest_time(RRDDIM *rd) {
    return rrdset_first_entry_t_nolock(rd->rrdset);
}
