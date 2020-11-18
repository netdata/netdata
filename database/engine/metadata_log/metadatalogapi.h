// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_METADATALOGAPI_H
#define NETDATA_METADATALOGAPI_H

extern void metalog_commit_delete_chart(RRDSET *st);
extern void metalog_delete_dimension_by_uuid(struct metalog_instance *ctx, uuid_t *metric_uuid);

/* must call once before using anything */
extern int metalog_init(struct rrdengine_instance *rrdeng_parent_ctx);

#endif /* NETDATA_METADATALOGAPI_H */
