// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_METADATALOGAPI_H
#define NETDATA_METADATALOGAPI_H

#include "metadatalog.h"

extern void metalog_commit_update_host(RRDHOST *host);
extern void metalog_commit_update_chart(RRDSET *st);
extern void metalog_commit_delete_chart(RRDSET *st);
extern void metalog_commit_update_dimension(RRDDIM *rd);
extern void metalog_commit_delete_dimension(RRDDIM *rd);

extern RRDSET *metalog_get_chart_from_uuid(struct metalog_instance *ctx, uuid_t *chart_uuid);
extern RRDDIM *metalog_get_dimension_from_uuid(struct metalog_instance *ctx, uuid_t *metric_uuid);
extern void metalog_delete_dimension_by_uuid(struct metalog_instance *ctx, uuid_t *metric_uuid);

/* must call once before using anything */
extern int metalog_init(struct rrdengine_instance *rrdeng_parent_ctx);
extern int metalog_exit(struct metalog_instance *ctx);

#endif /* NETDATA_METADATALOGAPI_H */
