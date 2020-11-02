// SPDX-License-Identifier: GPL-3.0-or-later
#define NETDATA_RRD_INTERNALS

#include "metadatalog.h"

void metalog_commit_delete_chart(RRDSET *st)
{
    char uuid_str[37];

    uuid_unparse_lower(*st->chart_uuid, uuid_str);
    info("metalog_commit_delete_chart %s", uuid_str);
    return;
}

void metalog_commit_delete_dimension(RRDDIM *rd)
{
    char uuid_str[37];

    uuid_unparse_lower(*rd->state->metric_uuid, uuid_str);
    info("metalog_commit_delete_dimension %s", uuid_str);
    return;
}

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
