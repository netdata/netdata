// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_RRDDIMMEM_H
#define NETDATA_RRDDIMMEM_H

#include "rrd.h"

struct mem_collect_handle {
    long slot;
    long entries;
};
struct mem_query_handle {
    long slot;
    long last_slot;
    uint8_t finished;
};

RRDDIM* rrddim_init_map(RRDSET *st, const char *id, const char *filename, collected_number multiplier,
                          collected_number divisor, RRD_ALGORITHM algorithm);
RRDDIM* rrddim_init_ram(RRDSET *st, const char *id, const char *filename, collected_number multiplier,
                          collected_number divisor, RRD_ALGORITHM algorithm);
RRDDIM* rrddim_init_save(RRDSET *st, const char *id, const char *filename, collected_number multiplier,
                          collected_number divisor, RRD_ALGORITHM algorithm);

RRDSET* rrdset_init_map(const char *id, const char *fullid, const char *filename, long entries, int update_every);
RRDSET* rrdset_init_ram(const char *id, const char *fullid, const char *filename, long entries, int update_every);
RRDSET* rrdset_init_save(const char *id, const char *fullid, const char *filename, long entries, int update_every);

void rrddim_destroy(RRDDIM *rd);

void rrddim_collect_init(RRDDIM *rd);
void rrddim_collect_store_metric(RRDDIM *rd, usec_t point_in_time, storage_number number);
int rrddim_collect_finalize(RRDDIM *rd);

void rrddim_query_init(RRDDIM *rd, struct rrddim_query_handle *handle, time_t start_time, time_t end_time);
storage_number rrddim_query_next_metric(struct rrddim_query_handle *handle, time_t *current_time);
int rrddim_query_is_finished(struct rrddim_query_handle *handle);
void rrddim_query_finalize(struct rrddim_query_handle *handle);
time_t rrddim_query_latest_time(RRDDIM *rd);
time_t rrddim_query_oldest_time(RRDDIM *rd);

#endif
