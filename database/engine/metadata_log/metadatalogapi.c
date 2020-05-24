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

void metalog_commit_update_host(RRDHOST *host)
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
                   "HOST \"%s\" \"%s\" \"%s\" %d \"%s\" \"%s\" \"%s\"\n",
//                 "\"%s\" \"%s\" \"%s\" \"%s\" \"%s\" \"%s\" \"%s\" \"%s\" \"%s\" \"%s\" \"%s\" \"%s\""  /* system */
//                 "\"%s\" \"%s\" \"%s\" \"%s\" \"%s\" \"%s\" \"%s\" \"%s\" \"%s\" \"%s\" \"%s\" \"%s\"", /* info   */
                   host->machine_guid,
                   host->hostname,
                   host->registry_hostname,
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

void metalog_commit_update_chart(RRDSET *st)
{
    struct metalog_instance *ctx;
    BUFFER *buffer;
    RRDHOST *host = st->rrdhost;

    /* Metadata are only available with dbengine */
    if (!host->rrdeng_ctx || RRD_MEMORY_MODE_DBENGINE != st->rrd_memory_mode)
        return;

    ctx = host->rrdeng_ctx->metalog_ctx;
    buffer = buffer_create(1024); /* This will be freed after it has been committed to the metadata log buffer */

    rrdset_rdlock(st);

    buffer_sprintf(buffer, "CONTEXT %s\n", host->machine_guid);

    char uuid_str[37];
    uuid_unparse_lower(*st->chart_uuid, uuid_str);
    buffer_sprintf(buffer, "GUID %s\n", uuid_str); /* TODO: replace this with real GUID when available */

    // properly set the name for the remote end to parse it
    char *name = "";
    if(likely(st->name)) {
        if(unlikely(strcmp(st->id, st->name))) {
            // they differ
            name = strchr(st->name, '.');
            if(name)
                name++;
            else
                name = "";
        }
    }

    // send the chart
    buffer_sprintf(
        buffer
        , "CHART \"%s\" \"%s\" \"%s\" \"%s\" \"%s\" \"%s\" \"%s\" %ld %d \"%s %s %s %s\" \"%s\" \"%s\"\n"
        , st->id
        , name
        , st->title
        , st->units
        , st->family
        , st->context
        , rrdset_type_name(st->chart_type)
        , st->priority
        , st->update_every
        , rrdset_flag_check(st, RRDSET_FLAG_OBSOLETE)?"obsolete":""
        , rrdset_flag_check(st, RRDSET_FLAG_DETAIL)?"detail":""
        , rrdset_flag_check(st, RRDSET_FLAG_STORE_FIRST)?"store_first":""
        , rrdset_flag_check(st, RRDSET_FLAG_HIDDEN)?"hidden":""
        , (st->plugin_name)?st->plugin_name:""
        , (st->module_name)?st->module_name:""
    );

    // send the dimensions
    RRDDIM *rd;
    rrddim_foreach_read(rd, st) {
        char uuid_str[37];

        uuid_unparse_lower(*rd->state->rrdeng_uuid, uuid_str);
        buffer_sprintf(buffer, "GUID %s\n", uuid_str);

        buffer_sprintf(
            buffer
            , "DIMENSION \"%s\" \"%s\" \"%s\" " COLLECTED_NUMBER_FORMAT " " COLLECTED_NUMBER_FORMAT " \"%s %s %s\"\n"
            , rd->id
            , rd->name
            , rrd_algorithm_name(rd->algorithm)
            , rd->multiplier
            , rd->divisor
            , rrddim_flag_check(rd, RRDDIM_FLAG_OBSOLETE)?"obsolete":""
            , rrddim_flag_check(rd, RRDDIM_FLAG_HIDDEN)?"hidden":""
            , rrddim_flag_check(rd, RRDDIM_FLAG_DONT_DETECT_RESETS_OR_OVERFLOWS)?"noreset":""
        );
    }
    rrdset_unlock(st);

    metalog_commit_creation_record(ctx, buffer);
}

void metalog_commit_delete_chart(RRDSET *st)
{
    struct metalog_instance *ctx;
    BUFFER *buffer;
    RRDHOST *host = st->rrdhost;

    /* Metadata are only available with dbengine */
    if (!host->rrdeng_ctx || RRD_MEMORY_MODE_DBENGINE != st->rrd_memory_mode)
        return;

    ctx = host->rrdeng_ctx->metalog_ctx;
    buffer = buffer_create(64); /* This will be freed after it has been committed to the metadata log buffer */

    buffer_sprintf(buffer, "CONTEXT %s\n", host->machine_guid);

    buffer_sprintf(buffer, "TOMBSTONE %s\n", st->id); /* TODO: replace this with real GUID when available */

    metalog_commit_creation_record(ctx, buffer);
}

void metalog_commit_update_dimension(RRDDIM *rd)
{
    struct metalog_instance *ctx;
    BUFFER *buffer;
    RRDSET *st = rd->rrdset;
    RRDHOST *host = st->rrdhost;
    char uuid_str[37];

    /* Metadata are only available with dbengine */
    if (!host->rrdeng_ctx || RRD_MEMORY_MODE_DBENGINE != st->rrd_memory_mode)
        return;

    ctx = host->rrdeng_ctx->metalog_ctx;
    buffer = buffer_create(128); /* This will be freed after it has been committed to the metadata log buffer */

    uuid_unparse_lower(*st->chart_uuid, uuid_str);
    buffer_sprintf(buffer, "CONTEXT %s\n", uuid_str);
    // Activate random GUID
    uuid_unparse_lower(*rd->state->metric_uuid, uuid_str);
    //uuid_unparse_lower(*rd->state->rrdeng_uuid, uuid_str);
    buffer_sprintf(buffer, "GUID %s\n", uuid_str);

    buffer_sprintf(
        buffer
        , "DIMENSION \"%s\" \"%s\" \"%s\" " COLLECTED_NUMBER_FORMAT " " COLLECTED_NUMBER_FORMAT " \"%s %s %s\"\n"
        , rd->id
        , rd->name
        , rrd_algorithm_name(rd->algorithm)
        , rd->multiplier
        , rd->divisor
        , rrddim_flag_check(rd, RRDDIM_FLAG_OBSOLETE)?"obsolete":""
        , rrddim_flag_check(rd, RRDDIM_FLAG_HIDDEN)?"hidden":""
        , rrddim_flag_check(rd, RRDDIM_FLAG_DONT_DETECT_RESETS_OR_OVERFLOWS)?"noreset":""
    );

    metalog_commit_creation_record(ctx, buffer);
}

void metalog_commit_delete_dimension(RRDDIM *rd)
{
    struct metalog_instance *ctx;
    BUFFER *buffer;
    RRDSET *st = rd->rrdset;
    RRDHOST *host = st->rrdhost;
    char uuid_str[37];

    /* Metadata are only available with dbengine */
    if (!host->rrdeng_ctx || RRD_MEMORY_MODE_DBENGINE != st->rrd_memory_mode)
        return;

    ctx = host->rrdeng_ctx->metalog_ctx;
    buffer = buffer_create(64); /* This will be freed after it has been committed to the metadata log buffer */

    buffer_sprintf(buffer, "CONTEXT %s\n", st->id);

    uuid_unparse_lower(*rd->state->rrdeng_uuid, uuid_str);
    buffer_sprintf(buffer, "TOMBSTONE %s\n", uuid_str);

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
