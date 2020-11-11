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
    delete_dimension_uuid(rd->state->metric_uuid);
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

/* This function is called by dbengine rotation logic when the metric has no writers */
void metalog_delete_dimension_by_uuid(struct metalog_instance *ctx, uuid_t *metric_uuid)
{
    UNUSED(ctx);
    RRDDIM *rd = NULL;
    RRDSET *st = NULL;
    RRDHOST *host = NULL;
//    uint8_t empty_chart;
    char    *host_guid = NULL;
    char    *rd_id = NULL;
    char    *st_id = NULL;
    int     not_found = 1;
    uuid_t  stored_uuid;

    char uuid_str[37];
    uuid_unparse_lower(*metric_uuid, uuid_str);
    info("WARNING: Delete metric %s due to rotation", uuid_str);

    int rc = find_host_chart_dimension(metric_uuid, &host_guid, &st_id, &rd_id, &stored_uuid);

    if (unlikely(rc))
        goto done;

    host = rrdhost_find_by_guid(host_guid, 0);
    if (unlikely(!host))
        goto done;

    info("WARNING UUID %s host %s, chart [%s], dimension [%s]", uuid_str, host->hostname, st_id, rd_id);

    st = rrdset_find(host, st_id);
    if (unlikely(!st))
        goto done;

    rd = rrddim_find(st, st_id);
    if (unlikely(!rd))
        goto done;

    info("WARNING UUID %s host %s, chart [%s], dimension [%s] (found)", uuid_str, host->hostname, st_id, rd_id);
    char uuid_str1[37];
    uuid_unparse_lower(*rd->state->metric_uuid, uuid_str1);
    char uuid_str2[37];
    uuid_unparse_lower(stored_uuid, uuid_str1);

    info("WARNING: delete metric %s due to rotation that matches metric_uuid %s (in db = %s)", uuid_str, uuid_str1, uuid_str2);
    not_found = 0;

done:
    if (unlikely(not_found))
        info("Rotated unknown archived metric.");

    freez(host);
    freez(st_id);
    freez(rd_id);

    // TODO: check the database and delete the UUID

//
//    rd = metalog_get_dimension_from_uuid(ctx, metric_uuid);
//    if (!rd) { /* in the case of legacy UUID convert to multihost and try again */
//        uuid_t multihost_uuid;
//
//        rrdeng_convert_legacy_uuid_to_multihost(ctx->rrdeng_ctx->machine_guid, metric_uuid, &multihost_uuid);
//        rd = metalog_get_dimension_from_uuid(ctx, &multihost_uuid);
//    }
//    if(!rd) {
//        info("Rotated unknown archived metric.");
//        return;
//    }
//    st = rd->rrdset;
//    host = st->rrdhost;
//
//    /* In case there are active metrics in a different database engine do not delete the dimension object */
//    if (unlikely(host->rrd_memory_mode != RRD_MEMORY_MODE_DBENGINE))
//        return;
//
//    /* Since the metric has no writer it will not be commited to the metadata log by rrddim_free_custom().
//     * It must be commited explicitly before calling rrddim_free_custom(). */
//    metalog_commit_delete_dimension(rd);
//
//    rrdset_wrlock(st);
//    rrddim_free_custom(st, rd, 1);
//    empty_chart = (NULL == st->dimensions);
//    rrdset_unlock(st);
//
//    if (empty_chart) {
//        rrdhost_wrlock(host);
//        rrdset_rdlock(st);
//        rrdset_delete_custom(st, 1);
//        rrdset_unlock(st);
//        rrdset_free(st);
//        rrdhost_unlock(host);
//    }
}
