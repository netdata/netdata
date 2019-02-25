// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_RRDENGINEAPI_H
#define NETDATA_RRDENGINEAPI_H

#include "rrdengine.h"

struct rrdeng_handle {
    struct rrdeng_page_cache_descr *descr;
    uuid_t *uuid;
};

extern void *rrdeng_create_page(uuid_t *id, struct rrdeng_page_cache_descr **ret_descr);
extern void rrdeng_commit_page(struct rrdeng_page_cache_descr *descr);
extern void *rrdeng_get_latest_page(uuid_t *metric, void **handle);
extern void *rrdeng_get_page(uuid_t *metric, usec_t point_in_time, void **handle);
extern void rrdeng_put_page(void *handle);
extern void rrdeng_store_metric_init(RRDDIM *rd, struct rrdeng_handle *handle);
extern void rrdeng_store_metric_next(struct rrdeng_handle *handle, usec_t point_in_time, storage_number number);
extern void rrdeng_store_metric_final(struct rrdeng_handle *handle);
extern void rrdeng_load_metric_init(uuid_t *uuid, struct rrdeng_handle *handle, usec_t start_time, usec_t end_time);
extern storage_number rrdeng_load_metric_next(struct rrdeng_handle *handle, usec_t point_in_time);
extern void rrdeng_load_metric_final(struct rrdeng_handle *handle);

/* must call once before using anything */
extern int rrdeng_init(void);

extern int rrdeng_exit(void);

#endif /* NETDATA_RRDENGINEAPI_H */