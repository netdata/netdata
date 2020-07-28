// SPDX-License-Identifier: GPL-3.0-or-later
#define NETDATA_RRD_INTERNALS

#include "metadatalog.h"

void after_compact_old_records(struct metalog_worker_config* wc)
{
    struct metalog_instance *ctx = wc->ctx;
    int error;

    mlf_flush_records_buffer(wc, &ctx->compaction_state.records_log, &ctx->compaction_state.new_metadata_logfiles);
    uv_run(wc->loop, UV_RUN_DEFAULT);

    error = uv_thread_join(wc->now_compacting_files);
    if (error) {
        error("uv_thread_join(): %s", uv_strerror(error));
    }
    freez(wc->now_compacting_files);
    /* unfreeze command processing */
    wc->now_compacting_files = NULL;

    wc->cleanup_thread_compacting_files = 0;

    /* interrupt event loop */
    uv_stop(wc->loop);

    info("Finished metadata log compaction (id:%"PRIu32").", ctx->current_compaction_id);
}

static void metalog_flush_compaction_records(struct metalog_instance *ctx)
{
    struct metalog_cmd cmd;
    struct completion compaction_completion;

    init_completion(&compaction_completion);

    cmd.opcode = METALOG_COMPACTION_FLUSH;
    cmd.record_io_descr.completion = &compaction_completion;
    metalog_enq_cmd(&ctx->worker_config, &cmd);

    wait_for_completion(&compaction_completion);
    destroy_completion(&compaction_completion);
}

/* The caller must have called metalog_flush_compaction_records() before to synchronize and quiesce the event loop. */
static void compaction_test_quota(struct metalog_worker_config *wc)
{
    struct metalog_instance *ctx = wc->ctx;
    struct logfile_compaction_state *compaction_state;
    struct metadata_logfile *oldmetalogfile, *newmetalogfile;
    unsigned current_size;
    int ret;

    compaction_state = &ctx->compaction_state;
    newmetalogfile = compaction_state->new_metadata_logfiles.last;

    oldmetalogfile = ctx->metadata_logfiles.first;

    current_size = newmetalogfile->pos;
    if (unlikely(current_size >= MAX_METALOGFILE_SIZE && newmetalogfile->starting_fileno < oldmetalogfile->fileno)) {
        /* It's safe to finalize the compacted metadata log file and create a new one since it has already replaced
         * an older one. */

        /* Finalize as the immediately previous file than the currently compacted one. */
        ret = rename_metadata_logfile(newmetalogfile, 0, newmetalogfile->fileno - 1);
        if (ret < 0)
            return;

        ret = add_new_metadata_logfile(ctx, &compaction_state->new_metadata_logfiles,
                                       ctx->metadata_logfiles.first->fileno, ctx->metadata_logfiles.first->fileno);

        if (likely(!ret)) {
            compaction_state->fileno = ctx->metadata_logfiles.first->fileno;
        }
    }
}


static void compact_record_by_uuid(struct metalog_instance *ctx, uuid_t *uuid)
{
    GUID_TYPE ret;
    RRDSET *st;
    RRDDIM *rd;
    BUFFER *buffer;
    RRDHOST *host = NULL;

    ret = find_object_by_guid(uuid, NULL, 0);
    switch (ret) {
        case GUID_TYPE_CHAR:
            error_with_guid(uuid, "Ignoring unexpected type GUID_TYPE_CHAR");
            break;
        case GUID_TYPE_CHART:
            st = metalog_get_chart_from_uuid(ctx, uuid);
            if (st) {
                if (ctx->current_compaction_id > st->rrdhost->compaction_id) {
                    error("Forcing compaction of HOST %s from CHART %s", st->rrdhost->hostname, st->id);
                    compact_record_by_uuid(ctx, &st->rrdhost->host_uuid);
                }

                if (ctx->current_compaction_id > st->compaction_id) {
                    st->compaction_id = ctx->current_compaction_id;
                    buffer = metalog_update_chart_buffer(st, ctx->current_compaction_id);
                    metalog_commit_record(ctx, buffer, METALOG_COMMIT_CREATION_RECORD, uuid, 1);
                } else {
                    debug(D_METADATALOG, "Chart has already been compacted, ignoring record.");
                }
            } else {
                debug(D_METADATALOG, "Ignoring nonexistent chart metadata record.");
            }
            break;
        case GUID_TYPE_DIMENSION:
            rd = metalog_get_dimension_from_uuid(ctx, uuid);
            if (rd) {
                if (ctx->current_compaction_id > rd->rrdset->rrdhost->compaction_id) {
                    error("Forcing compaction of HOST %s", rd->rrdset->rrdhost->hostname);
                    compact_record_by_uuid(ctx, &rd->rrdset->rrdhost->host_uuid);
                }
                if (ctx->current_compaction_id > rd->rrdset->compaction_id) {
                    error("Forcing compaction of CHART %s", rd->rrdset->id);
                    compact_record_by_uuid(ctx, rd->rrdset->chart_uuid);
                } else if (ctx->current_compaction_id > rd->state->compaction_id) {
                    rd->state->compaction_id = ctx->current_compaction_id;
                    buffer = metalog_update_dimension_buffer(rd);
                    metalog_commit_record(ctx, buffer, METALOG_COMMIT_CREATION_RECORD, uuid, 1);
                } else {
                    debug(D_METADATALOG, "Dimension has already been compacted, ignoring record.");
                }
            } else {
                debug(D_METADATALOG, "Ignoring nonexistent dimension metadata record.");
            }
            break;
        case GUID_TYPE_HOST:
            host = metalog_get_host_from_uuid(ctx, uuid);
            if (unlikely(!host))
                break;
            if (ctx->current_compaction_id > host->compaction_id) {
                host->compaction_id = ctx->current_compaction_id;
                buffer = metalog_update_host_buffer(host);
                metalog_commit_record(ctx, buffer, METALOG_COMMIT_CREATION_RECORD, uuid, 1);
            } else {
                debug(D_METADATALOG, "Host has already been compacted, ignoring record.");
            }
            break;
        case GUID_TYPE_NOTFOUND:
            debug(D_METADATALOG, "Ignoring nonexistent metadata record.");
            break;
        case GUID_TYPE_NOSPACE:
            error_with_guid(uuid, "Not enough space for object retrieval");
            break;
        default:
            error("Unknown return code %u from find_object_by_guid", ret);
            break;
    }
}

/* Returns 0 on success. */
static int compact_metadata_logfile_records(struct metalog_instance *ctx, struct metadata_logfile *metalogfile)
{
    struct metalog_worker_config* wc = &ctx->worker_config;
    struct logfile_compaction_state *compaction_state;
    struct metalog_record *record;
    struct metalog_record_block *record_block, *prev_record_block;
    int ret;
    unsigned iterated_records;
#define METADATA_LOG_RECORD_BATCH 128 /* Flush I/O and check sizes whenever this many records have been iterated */

    info("Compacting metadata log file \"%s/"METALOG_PREFIX METALOG_FILE_NUMBER_PRINT_TMPL METALOG_EXTENSION"\".",
         ctx->rrdeng_ctx->dbfiles_path, metalogfile->starting_fileno, metalogfile->fileno);

    compaction_state = &ctx->compaction_state;
    record_block = prev_record_block = NULL;
    iterated_records = 0;
    for (record = mlf_record_get_first(metalogfile) ; record != NULL ; record = mlf_record_get_next(metalogfile)) {
        if ((record_block = metalogfile->records.iterator.current) != prev_record_block) {
            if (prev_record_block) { /* Deallocate iterated record blocks */
                rrd_atomic_fetch_add(&ctx->records_nr, -prev_record_block->records_nr);
                freez(prev_record_block);
            }
            prev_record_block = record_block;
        }
        compact_record_by_uuid(ctx, &record->uuid);
        if (0 == ++iterated_records % METADATA_LOG_RECORD_BATCH) {
            metalog_flush_compaction_records(ctx);
            if (compaction_state->throttle) {
                (void)sleep_usec(10000); /* 10 msec throttle compaction */
            }
            compaction_test_quota(wc);
        }
    }
    if (prev_record_block) { /* Deallocate iterated record blocks */
        rrd_atomic_fetch_add(&ctx->records_nr, -prev_record_block->records_nr);
        freez(prev_record_block);
    }

    info("Compacted metadata log file \"%s/"METALOG_PREFIX METALOG_FILE_NUMBER_PRINT_TMPL METALOG_EXTENSION"\".",
         ctx->rrdeng_ctx->dbfiles_path, metalogfile->starting_fileno, metalogfile->fileno);

    metadata_logfile_list_delete(&ctx->metadata_logfiles, metalogfile);
    ret = destroy_metadata_logfile(metalogfile);
    if (!ret) {
        info("Deleted file \"%s/"METALOG_PREFIX METALOG_FILE_NUMBER_PRINT_TMPL METALOG_EXTENSION"\".",
             ctx->rrdeng_ctx->dbfiles_path, metalogfile->starting_fileno, metalogfile->fileno);
        rrd_atomic_fetch_add(&ctx->disk_space, -metalogfile->pos);
    } else {
        error("Failed to delete file \"%s/"METALOG_PREFIX METALOG_FILE_NUMBER_PRINT_TMPL METALOG_EXTENSION"\".",
             ctx->rrdeng_ctx->dbfiles_path, metalogfile->starting_fileno, metalogfile->fileno);
    }
    freez(metalogfile);

    return ret;
}

static void compact_old_records(void *arg)
{
    struct metalog_instance *ctx = arg;
    struct metalog_worker_config* wc = &ctx->worker_config;
    struct logfile_compaction_state *compaction_state;
    struct metadata_logfile *metalogfile, *nextmetalogfile, *newmetalogfile;
    int ret;

    compaction_state = &ctx->compaction_state;

    nextmetalogfile = NULL;
    for (metalogfile = ctx->metadata_logfiles.first ;
         metalogfile != compaction_state->last_original_logfile ;
         metalogfile = nextmetalogfile) {
        nextmetalogfile = metalogfile->next;

        newmetalogfile = compaction_state->new_metadata_logfiles.last;
        ret = rename_metadata_logfile(newmetalogfile, newmetalogfile->starting_fileno, metalogfile->fileno);
        if (ret < 0) {
            error("Failed to rename file \"%s/"METALOG_PREFIX METALOG_FILE_NUMBER_PRINT_TMPL METALOG_EXTENSION"\".",
                  ctx->rrdeng_ctx->dbfiles_path, newmetalogfile->starting_fileno, newmetalogfile->fileno);
        }

        ret = compact_metadata_logfile_records(ctx, metalogfile);
        if (ret) {
            error("Metadata log compaction failed, cancelling.");
            break;
        }
    }
    fatal_assert(nextmetalogfile); /* There are always more than 1 metadata log files during compaction */

    newmetalogfile = compaction_state->new_metadata_logfiles.last;
    if (newmetalogfile->starting_fileno != 0) { /* Must rename the last compacted file */
        ret = rename_metadata_logfile(newmetalogfile, 0, nextmetalogfile->fileno - 1);
        if (ret < 0) {
            error("Failed to rename file \"%s/"METALOG_PREFIX METALOG_FILE_NUMBER_PRINT_TMPL METALOG_EXTENSION"\".",
                  ctx->rrdeng_ctx->dbfiles_path, newmetalogfile->starting_fileno, newmetalogfile->fileno);
        }
    }
    /* Connect the compacted files to the metadata log */
    newmetalogfile->next = nextmetalogfile;
    ctx->metadata_logfiles.first = compaction_state->new_metadata_logfiles.first;

    wc->cleanup_thread_compacting_files = 1;
    /* wake up event loop */
    fatal_assert(0 == uv_async_send(&wc->async));
}

/* Returns 0 on success. */
static int init_compaction_state(struct metalog_instance *ctx)
{
    struct metadata_logfile *newmetalogfile;
    struct logfile_compaction_state *compaction_state;
    int ret;

    compaction_state = &ctx->compaction_state;
    compaction_state->new_metadata_logfiles.first = NULL;
    compaction_state->new_metadata_logfiles.last = NULL;
    compaction_state->starting_fileno = ctx->metadata_logfiles.first->fileno;
    compaction_state->fileno = ctx->metadata_logfiles.first->fileno;
    compaction_state->last_original_logfile = ctx->metadata_logfiles.last;
    compaction_state->throttle = 0;

    ret = add_new_metadata_logfile(ctx, &compaction_state->new_metadata_logfiles, compaction_state->starting_fileno,
                                   compaction_state->fileno);
    if (unlikely(ret)) {
        error("Cannot create new metadata log files, compaction aborted.");
        return ret;
    }
    newmetalogfile = compaction_state->new_metadata_logfiles.first;
    fatal_assert(newmetalogfile == compaction_state->new_metadata_logfiles.last);
    init_metadata_record_log(&compaction_state->records_log);

    return 0;
}

void metalog_do_compaction(struct metalog_worker_config *wc)
{
    struct metalog_instance *ctx = wc->ctx;
    int error;

    if (wc->now_compacting_files) {
        /* already compacting metadata log files */
        return;
    }
    wc->now_compacting_files = mallocz(sizeof(*wc->now_compacting_files));
    wc->cleanup_thread_compacting_files = 0;
    metalog_try_link_new_metadata_logfile(wc);

    error = init_compaction_state(ctx);
    if (unlikely(error)) {
        error("Cannot create new metadata log files, compaction aborted.");
        return;
    }
    ++ctx->current_compaction_id; /* Signify a new compaction */

    info("Starting metadata log compaction (id:%"PRIu32").", ctx->current_compaction_id);
    error = uv_thread_create(wc->now_compacting_files, compact_old_records, ctx);
    if (error) {
        error("uv_thread_create(): %s", uv_strerror(error));
        freez(wc->now_compacting_files);
        wc->now_compacting_files = NULL;
    }

}

/* Return 0 on success. */
int compaction_failure_recovery(struct metalog_instance *ctx, struct metadata_logfile **metalogfiles,
                                 unsigned *matched_files)
{
    int ret;
    unsigned starting_fileno, fileno, i, j, recovered_files;
    struct metadata_logfile *metalogfile = NULL, *compactionfile = NULL, **tmp_metalogfiles;
    char *dbfiles_path = ctx->rrdeng_ctx->dbfiles_path;

    for (i = 0 ; i < *matched_files ; ++i) {
        metalogfile = metalogfiles[i];
        if (0 == metalogfile->starting_fileno)
            continue; /* skip standard metadata log files */
        break; /* this is a compaction temporary file */
    }
    if (i == *matched_files) /* no recovery needed */
        return 0;
    info("Starting metadata log file failure recovery procedure in \"%s\".", dbfiles_path);

    if (*matched_files - i > 1) { /* Can't have more than 1 temporary compaction files */
        error("Metadata log files are in an invalid state. Cannot proceed.");
        return 1;
    }
    compactionfile = metalogfile;
    starting_fileno = compactionfile->starting_fileno;
    fileno = compactionfile->fileno;
    /* scratchpad space to move file pointers around */
    tmp_metalogfiles = callocz(*matched_files, sizeof(*tmp_metalogfiles));

    for (j = 0, recovered_files = 0 ; j < i ; ++j) {
        metalogfile = metalogfiles[j];
        fatal_assert(0 == metalogfile->starting_fileno);
        if (metalogfile->fileno < starting_fileno) {
            tmp_metalogfiles[recovered_files++] = metalogfile;
            continue;
        }
        break; /* reached compaction file serial number */
    }

    if ((j == i) /* Shouldn't be possible, invalid compaction temporary file */ ||
        (metalogfile->fileno == starting_fileno && metalogfile->fileno == fileno)) {
        error("Deleting invalid compaction temporary file \"%s/"METALOG_PREFIX METALOG_FILE_NUMBER_PRINT_TMPL
              METALOG_EXTENSION"\"", dbfiles_path, starting_fileno, fileno);
        unlink_metadata_logfile(compactionfile);
        freez(compactionfile);
        freez(tmp_metalogfiles);
        --*matched_files; /* delete the last one */

        info("Finished metadata log file failure recovery procedure in \"%s\".", dbfiles_path);
        return 0;
    }

    for ( ; j < i ; ++j) { /* continue iterating through normal metadata log files */
        metalogfile = metalogfiles[j];
        fatal_assert(0 == metalogfile->starting_fileno);
        if (metalogfile->fileno < fileno) { /* It has already been compacted */
            error("Deleting invalid metadata log file \"%s/"METALOG_PREFIX METALOG_FILE_NUMBER_PRINT_TMPL
                      METALOG_EXTENSION"\"", dbfiles_path, 0U, metalogfile->fileno);
            unlink_metadata_logfile(metalogfile);
            freez(metalogfile);
            continue;
        }
        tmp_metalogfiles[recovered_files++] = metalogfile;
    }

    /* compaction temporary file is valid */
    tmp_metalogfiles[recovered_files++] = compactionfile;
    ret = rename_metadata_logfile(compactionfile, 0, starting_fileno);
    if (ret < 0) {
        error("Cannot rename temporary compaction files. Cannot proceed.");
        freez(tmp_metalogfiles);
        return 1;
    }

    memcpy(metalogfiles, tmp_metalogfiles, recovered_files * sizeof(*metalogfiles));
    *matched_files = recovered_files;
    freez(tmp_metalogfiles);

    info("Finished metadata log file failure recovery procedure in \"%s\".", dbfiles_path);
    return 0;
}
