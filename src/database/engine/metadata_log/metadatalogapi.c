// SPDX-License-Identifier: GPL-3.0-or-later
#define NETDATA_RRD_INTERNALS

#include "metadatalog.h"

/*
 * Returns 0 on success, negative on error
 */
int metalog_init(struct rrdengine_instance *rrdeng_parent_ctx)
{
    struct metalog_instance *ctx;
    int error;

    ctx = callocz(1, sizeof(*ctx));
    ctx->initialized = 0;
    rrdeng_parent_ctx->metalog_ctx = ctx;

    ctx->rrdeng_ctx = rrdeng_parent_ctx;
    error = init_metalog_files(ctx);
    if (error) {
        goto error_after_init_rrd_files;
    }
    ctx->initialized = 1; /* notify dbengine that the metadata log has finished initializing */
    return 0;

error_after_init_rrd_files:
    freez(ctx);
    return UV_EIO;
}

/* This function is called by dbengine rotation logic when the metric has no writers */
void metalog_delete_dimension_by_uuid(struct metalog_instance *ctx, uuid_t *metric_uuid)
{
    uuid_t  multihost_uuid;

    delete_dimension_uuid(metric_uuid);
    rrdeng_convert_legacy_uuid_to_multihost(ctx->rrdeng_ctx->machine_guid, metric_uuid, &multihost_uuid);
    delete_dimension_uuid(&multihost_uuid);
}
