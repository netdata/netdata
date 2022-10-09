// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_RRDENGLOCKING_H
#define NETDATA_RRDENGLOCKING_H

#include "rrdengine.h"

/* Forward declarations */
struct page_cache_descr;

struct page_cache_descr *rrdeng_create_pg_cache_descr(struct rrdengine_instance *ctx);
void rrdeng_destroy_pg_cache_descr(struct rrdengine_instance *ctx, struct page_cache_descr *pg_cache_descr);
void rrdeng_page_descr_mutex_lock(struct rrdengine_instance *ctx, struct rrdeng_page_descr *descr);
void rrdeng_page_descr_mutex_unlock(struct rrdengine_instance *ctx, struct rrdeng_page_descr *descr);
void rrdeng_try_deallocate_pg_cache_descr(struct rrdengine_instance *ctx, struct rrdeng_page_descr *descr);

#endif /* NETDATA_RRDENGLOCKING_H */