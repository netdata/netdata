// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_RRDDIMMEM_H
#define NETDATA_RRDDIMMEM_H

#include "database/rrd.h"

struct mem_collect_handle {
    long slot;
    long entries;
};
struct mem_query_handle {
    long slot;
    long last_slot;
    uint8_t finished;
};

extern RRDDIM* rrddim_init_map(RRDSET *st, const char *id, const char *filename, collected_number multiplier,
                          collected_number divisor, RRD_ALGORITHM algorithm);
extern RRDDIM* rrddim_init_ram(RRDSET *st, const char *id, const char *filename, collected_number multiplier,
                          collected_number divisor, RRD_ALGORITHM algorithm);
extern RRDDIM* rrddim_init_save(RRDSET *st, const char *id, const char *filename, collected_number multiplier,
                          collected_number divisor, RRD_ALGORITHM algorithm);

extern RRDSET* rrdset_init_map(const char *id, const char *fullid, const char *filename, long entries, int update_every);
extern RRDSET* rrdset_init_ram(const char *id, const char *fullid, const char *filename, long entries, int update_every);
extern RRDSET* rrdset_init_save(const char *id, const char *fullid, const char *filename, long entries, int update_every);

STORAGE_ENGINE_INSTANCE* rrddim_storage_engine_instance_new(STORAGE_ENGINE* engine, RRDHOST *host);
void rrddim_storage_engine_instance_exit(STORAGE_ENGINE_INSTANCE* context);
void rrddim_storage_engine_instance_destroy(STORAGE_ENGINE_INSTANCE* context);

extern void rrddim_collect_init(RRDDIM *rd);
extern void rrddim_collect_store_metric(RRDDIM *rd, usec_t point_in_time, storage_number number);
extern int rrddim_collect_finalize(RRDDIM *rd);

extern void rrddim_query_init(RRDDIM *rd, struct rrddim_query_handle *handle, time_t start_time, time_t end_time);
extern storage_number rrddim_query_next_metric(struct rrddim_query_handle *handle, time_t *current_time);
extern int rrddim_query_is_finished(struct rrddim_query_handle *handle);
extern void rrddim_query_finalize(struct rrddim_query_handle *handle);
extern time_t rrddim_query_latest_time(RRDDIM *rd);
extern time_t rrddim_query_oldest_time(RRDDIM *rd);

#endif
