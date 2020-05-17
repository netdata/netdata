// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_METADATALOGAPI_H
#define NETDATA_METADATALOGAPI_H

#include "metadatalog.h"

void metalog_commit_create_host(RRDHOST *host);

/* must call once before using anything */
extern int metalog_init(struct rrdengine_instance *rrdeng_parent_ctx);
extern int metalog_exit(struct metalog_instance *ctx);

#endif /* NETDATA_METADATALOGAPI_H */