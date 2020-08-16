// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_METADATALOGAPI_H
#define NETDATA_METADATALOGAPI_H

#include "metadatalog.h"

extern BUFFER *metalog_update_host_buffer(RRDHOST *host);
extern void metalog_commit_update_host(RRDHOST *host);
extern BUFFER *metalog_update_chart_buffer(RRDSET *st, uint32_t compaction_id);
extern void metalog_commit_update_chart(RRDSET *st);
extern void metalog_commit_delete_chart(RRDSET *st);
extern BUFFER *metalog_update_dimension_buffer(RRDDIM *rd);
extern void metalog_commit_update_dimension(RRDDIM *rd);
extern void metalog_commit_delete_dimension(RRDDIM *rd);
extern void metalog_upd_objcount(RRDHOST *host, int count);

extern RRDSET *metalog_get_chart_from_uuid(struct metalog_instance *ctx, uuid_t *chart_uuid);
extern RRDDIM *metalog_get_dimension_from_uuid(struct metalog_instance *ctx, uuid_t *metric_uuid);
extern RRDHOST *metalog_get_host_from_uuid(struct metalog_instance *ctx, uuid_t *uuid);
extern void metalog_delete_dimension_by_uuid(struct metalog_instance *ctx, uuid_t *metric_uuid);
extern void metalog_print_dimension_by_uuid(struct metalog_instance *ctx, uuid_t *metric_uuid);

/* must call once before using anything */
extern int metalog_init(struct rrdengine_instance *rrdeng_parent_ctx);
extern int metalog_exit(struct metalog_instance *ctx);
extern void metalog_prepare_exit(struct metalog_instance *ctx);

#endif /* NETDATA_METADATALOGAPI_H */
