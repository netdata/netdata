// SPDX-License-Identifier: GPL-3.0-or-later
#include "metadatalog.h"

/* The buffer must not be empty */
static void metalog_commit_record(struct metalog_instance *ctx, BUFFER *buffer, enum metalog_opcode opcode)
{
    struct metalog_cmd cmd;

    assert(buffer_strlen(buffer));
    assert(opcode == METALOG_COMMIT_CREATION_RECORD || opcode == METALOG_COMMIT_DELETION_RECORD);

    cmd.opcode = opcode;
    cmd.record_io_descr.buffer = buffer;
    metalog_enq_cmd(&ctx->worker_config, &cmd);
}

static inline void metalog_commit_creation_record(struct metalog_instance *ctx, BUFFER *buffer)
{
    metalog_commit_record(ctx, buffer, METALOG_COMMIT_CREATION_RECORD);
}

static inline void metalog_commit_deletion_record(struct metalog_instance *ctx, BUFFER *buffer)
{
    metalog_commit_record(ctx, buffer, METALOG_COMMIT_DELETION_RECORD);
}

void metalog_commit_create_host(RRDHOST *host)
{
    struct metalog_instance *ctx;
    BUFFER *buffer;

    /* Metadata are only available with dbengine */
    if (!host->rrdeng_ctx)
        return;

    ctx = host->rrdeng_ctx->metalog_ctx;
    buffer = buffer_create(4096); /* This will be freed after it has been committed to the metadata log buffer */

    rrdhost_rdlock(host);

    buffer_sprintf(buffer,
                   "hostname=%s registry_hostname=%s machine_guid=%s update_every=%d os=%s timezone=%s tags=%s\n",
                   host->hostname,
                   host->registry_hostname,
                   host->machine_guid,
                   default_rrd_update_every,
                   host->os,
                   host->timezone,
                   (host->tags) ? host->tags : "");

    netdata_rwlock_rdlock(&host->labels_rwlock);
    struct label *labels = host->labels;
    while (labels) {
        buffer_sprintf(buffer
            , "LABEL \"%s\" = %d %s\n"
            , labels->key
            , (int)labels->label_source
            , labels->value);

        labels = labels->next;
    }
    netdata_rwlock_unlock(&host->labels_rwlock);

    buffer_strcat(buffer, "OVERWRITE labels\n");

    rrdhost_unlock(host);
    metalog_commit_creation_record(ctx, buffer);
}

/*
 * Returns 0 on success, negative on error
 */
int metalog_init(struct rrdengine_instance *rrdeng_parent_ctx)
{
    struct metalog_instance *ctx;
    int error;

    rrdeng_parent_ctx->metalog_ctx = ctx = callocz(1, sizeof(*ctx));

    ctx->max_disk_space = 32 * 1048576LLU;

    memset(&ctx->worker_config, 0, sizeof(ctx->worker_config));
    ctx->rrdeng_ctx = rrdeng_parent_ctx;
    ctx->worker_config.ctx = ctx;
    init_metadata_record_log(ctx);
    error = init_metalog_files(ctx);
    if (error) {
        goto error_after_init_rrd_files;
    }

    init_completion(&ctx->metalog_completion);
    assert(0 == uv_thread_create(&ctx->worker_config.thread, metalog_worker, &ctx->worker_config));
    /* wait for worker thread to initialize */
    wait_for_completion(&ctx->metalog_completion);
    destroy_completion(&ctx->metalog_completion);
    uv_thread_set_name_np(ctx->worker_config.thread, "METALOG");
    if (ctx->worker_config.error) {
        goto error_after_rrdeng_worker;
    }
    return 0;

error_after_rrdeng_worker:
    finalize_metalog_files(ctx);
error_after_init_rrd_files:
    freez(ctx);
    rrdeng_parent_ctx->metalog_ctx = NULL;
    return UV_EIO;
}

/*
 * Returns 0 on success, 1 on error
 */
int metalog_exit(struct metalog_instance *ctx)
{
    struct metalog_cmd cmd;

    if (NULL == ctx) {
        return 1;
    }

    cmd.opcode = METALOG_SHUTDOWN;
    metalog_enq_cmd(&ctx->worker_config, &cmd);

    assert(0 == uv_thread_join(&ctx->worker_config.thread));

    finalize_metalog_files(ctx);
    freez(ctx);

    return 0;
}
