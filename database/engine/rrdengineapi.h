// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_RRDENGINEAPI_H
#define NETDATA_RRDENGINEAPI_H

#include "rrdengine.h"

struct rrdeng_handle {
    struct rrdeng_page_cache_descr *descr;
    uuid_t *uuid;
    Word_t page_correlation_id;
    struct rrdengine_instance *ctx;
};

extern void *rrdeng_create_page(uuid_t *id, struct rrdeng_page_cache_descr **ret_descr);
extern void rrdeng_commit_page(struct rrdengine_instance *ctx, struct rrdeng_page_cache_descr *descr,
                               Word_t page_correlation_id);
extern void *rrdeng_get_latest_page(struct rrdengine_instance *ctx, uuid_t *id, void **handle);
extern void *rrdeng_get_page(struct rrdengine_instance *ctx, uuid_t *id, usec_t point_in_time, void **handle);
extern void rrdeng_put_page(struct rrdengine_instance *ctx, void *handle);
extern void rrdeng_store_metric_init(struct rrdengine_instance *ctx, RRDDIM *rd, struct rrdeng_handle *handle);
extern void rrdeng_store_metric_next(struct rrdeng_handle *handle, usec_t point_in_time, storage_number number);
extern void rrdeng_store_metric_final(struct rrdeng_handle *handle);
extern void rrdeng_load_metric_init(struct rrdengine_instance *ctx, uuid_t *uuid, struct rrdeng_handle *handle,
                                    usec_t start_time, usec_t end_time);
extern storage_number rrdeng_load_metric_next(struct rrdeng_handle *handle, usec_t point_in_time);
extern void rrdeng_load_metric_final(struct rrdeng_handle *handle);

/* must call once before using anything */
extern int rrdeng_init(struct rrdengine_instance *ctx);

extern int rrdeng_exit(struct rrdengine_instance *ctx);

#endif /* NETDATA_RRDENGINEAPI_H */