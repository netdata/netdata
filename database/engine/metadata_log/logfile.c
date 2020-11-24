// SPDX-License-Identifier: GPL-3.0-or-later
#include <database/sqlite/sqlite_functions.h>
#include "metadatalog.h"
#include "metalogpluginsd.h"


void generate_metadata_logfile_path(struct metadata_logfile *metalogfile, char *str, size_t maxlen)
{
    (void) snprintf(str, maxlen, "%s/" METALOG_PREFIX METALOG_FILE_NUMBER_PRINT_TMPL METALOG_EXTENSION,
                    metalogfile->ctx->rrdeng_ctx->dbfiles_path, metalogfile->starting_fileno, metalogfile->fileno);
}

void metadata_logfile_init(struct metadata_logfile *metalogfile, struct metalog_instance *ctx, unsigned starting_fileno,
                           unsigned fileno)
{
    metalogfile->starting_fileno = starting_fileno;
    metalogfile->fileno = fileno;
    metalogfile->file = (uv_file)0;
    metalogfile->pos = 0;
    metalogfile->next = NULL;
    metalogfile->ctx = ctx;
}

int rename_metadata_logfile(struct metadata_logfile *metalogfile, unsigned new_starting_fileno, unsigned new_fileno)
{
    //struct metalog_instance *ctx = metalogfile->ctx;
    uv_fs_t req;
    int ret;
    char oldpath[RRDENG_PATH_MAX], newpath[RRDENG_PATH_MAX];
    unsigned backup_starting_fileno, backup_fileno;

    backup_starting_fileno = metalogfile->starting_fileno;
    backup_fileno = metalogfile->fileno;
    generate_metadata_logfile_path(metalogfile, oldpath, sizeof(oldpath));
    metalogfile->starting_fileno = new_starting_fileno;
    metalogfile->fileno = new_fileno;
    generate_metadata_logfile_path(metalogfile, newpath, sizeof(newpath));

    info("Renaming metadata log file \"%s\" to \"%s\".", oldpath, newpath);
    ret = uv_fs_rename(NULL, &req, oldpath, newpath, NULL);
    if (ret < 0) {
        error("uv_fs_rename(%s): %s", oldpath, uv_strerror(ret));
        //++ctx->stats.fs_errors; /* this is racy, may miss some errors */
        rrd_stat_atomic_add(&global_fs_errors, 1);
        /* restore previous values */
        metalogfile->starting_fileno = backup_starting_fileno;
        metalogfile->fileno = backup_fileno;
    }
    uv_fs_req_cleanup(&req);

    return ret;
}

int unlink_metadata_logfile(struct metadata_logfile *metalogfile)
{
    //struct metalog_instance *ctx = metalogfile->ctx;
    uv_fs_t req;
    int ret;
    char path[RRDENG_PATH_MAX];

    generate_metadata_logfile_path(metalogfile, path, sizeof(path));

    ret = uv_fs_unlink(NULL, &req, path, NULL);
    if (ret < 0) {
        error("uv_fs_fsunlink(%s): %s", path, uv_strerror(ret));
//        ++ctx->stats.fs_errors;
        rrd_stat_atomic_add(&global_fs_errors, 1);
    }
    uv_fs_req_cleanup(&req);

    return ret;
}

static int check_metadata_logfile_superblock(uv_file file)
{
    int ret;
    struct rrdeng_metalog_sb *superblock;
    uv_buf_t iov;
    uv_fs_t req;

    ret = posix_memalign((void *)&superblock, RRDFILE_ALIGNMENT, sizeof(*superblock));
    if (unlikely(ret)) {
        fatal("posix_memalign:%s", strerror(ret));
    }
    iov = uv_buf_init((void *)superblock, sizeof(*superblock));

    ret = uv_fs_read(NULL, &req, file, &iov, 1, 0, NULL);
    if (ret < 0) {
        error("uv_fs_read: %s", uv_strerror(ret));
        uv_fs_req_cleanup(&req);
        goto error;
    }
    fatal_assert(req.result >= 0);
    uv_fs_req_cleanup(&req);

    if (strncmp(superblock->magic_number, RRDENG_METALOG_MAGIC, RRDENG_MAGIC_SZ)) {
        error("File has invalid superblock.");
        ret = UV_EINVAL;
    } else {
        ret = 0;
    }
    if (superblock->version > RRDENG_METALOG_VER) {
        error("File has unknown version %"PRIu16". Compatibility is not guaranteed.", superblock->version);
    }
error:
    free(superblock);
    return ret;
}

void replay_record(struct metadata_logfile *metalogfile, struct rrdeng_metalog_record_header *header, void *payload)
{
    struct metalog_instance *ctx = metalogfile->ctx;
    char *line, *nextline, *record_end;
    int ret;

    debug(D_METADATALOG, "RECORD contents: %.*s", (int)header->payload_length, (char *)payload);
    record_end = (char *)payload + header->payload_length - 1;
    *record_end = '\0';

    for (line = payload ; line ; line = nextline) {
        nextline = strchr(line, '\n');
        if (nextline) {
            *nextline++ = '\0';
        }
        ret = parser_action(ctx->metalog_parser_object->parser, line);
        debug(D_METADATALOG, "parser_action ret:%d", ret);
        if (ret)
            return; /* skip record due to error */
    };
}

/* This function only works with buffered I/O */
static inline int metalogfile_read(struct metadata_logfile *metalogfile, void *buf, size_t len, uint64_t offset)
{
//    struct metalog_instance *ctx;
    uv_file file;
    uv_buf_t iov;
    uv_fs_t req;
    int ret;

//    ctx = metalogfile->ctx;
    file = metalogfile->file;
    iov = uv_buf_init(buf, len);
    ret = uv_fs_read(NULL, &req, file, &iov, 1, offset, NULL);
    if (unlikely(ret < 0 && ret != req.result)) {
        fatal("uv_fs_read: %s", uv_strerror(ret));
    }
    if (req.result < 0) {
//        ++ctx->stats.io_errors;
        rrd_stat_atomic_add(&global_io_errors, 1);
        error("%s: uv_fs_read - %s - record at offset %"PRIu64"(%u) in metadata logfile %u-%u.", __func__,
              uv_strerror((int)req.result), offset, (unsigned)len, metalogfile->starting_fileno, metalogfile->fileno);
    }
    uv_fs_req_cleanup(&req);
//    ctx->stats.io_read_bytes += len;
//    ++ctx->stats.io_read_requests;

    return ret;
}

/* Return 0 on success */
static int metadata_record_integrity_check(void *record)
{
    int ret;
    uint32_t data_size;
    struct rrdeng_metalog_record_header *header;
    struct rrdeng_metalog_record_trailer *trailer;
    uLong crc;

    header = record;
    data_size = header->header_length + header->payload_length;
    trailer = record + data_size;

    crc = crc32(0L, Z_NULL, 0);
    crc = crc32(crc, record, data_size);
    ret = crc32cmp(trailer->checksum, crc);

    return ret;
}

#define MAX_READ_BYTES (RRDENG_BLOCK_SIZE * 32) /* no record should be over 128KiB in this version */

/*
 * Iterates metadata log file records and creates database objects (host/chart/dimension)
 */
static void iterate_records(struct metadata_logfile *metalogfile)
{
    uint32_t file_size, pos, bytes_remaining, record_size;
    void *buf;
    struct rrdeng_metalog_record_header *header;
    struct metalog_instance *ctx = metalogfile->ctx;
    struct metalog_pluginsd_state *state = ctx->metalog_parser_object->private;
    const size_t min_header_size = offsetof(struct rrdeng_metalog_record_header, header_length) +
                                   sizeof(header->header_length);

    file_size = metalogfile->pos;
    state->metalogfile = metalogfile;

    buf = mallocz(MAX_READ_BYTES);

    for (pos = sizeof(struct rrdeng_metalog_sb) ; pos < file_size ; pos += record_size) {
        bytes_remaining = file_size - pos;
        if (bytes_remaining < min_header_size) {
            error("%s: unexpected end of file in metadata logfile %u-%u.", __func__, metalogfile->starting_fileno,
                  metalogfile->fileno);
            break;
        }
        if (metalogfile_read(metalogfile, buf, min_header_size, pos) < 0)
            break;
        header = (struct rrdeng_metalog_record_header *)buf;
        if (METALOG_STORE_PADDING == header->type) {
            info("%s: Skipping padding in metadata logfile %u-%u.", __func__, metalogfile->starting_fileno,
                 metalogfile->fileno);
            record_size = ALIGN_BYTES_FLOOR(pos + RRDENG_BLOCK_SIZE) - pos;
            continue;
        }
        if (metalogfile_read(metalogfile, buf + min_header_size, sizeof(*header) - min_header_size,
                             pos + min_header_size) < 0)
            break;
        record_size = header->header_length + header->payload_length + sizeof(struct rrdeng_metalog_record_trailer);
        if (header->header_length < min_header_size || record_size > bytes_remaining) {
            error("%s: Corrupted record in metadata logfile %u-%u.", __func__, metalogfile->starting_fileno,
                  metalogfile->fileno);
            break;
        }
        if (record_size > MAX_READ_BYTES) {
            error("%s: Record is too long (%u bytes) in metadata logfile %u-%u.", __func__, record_size,
                  metalogfile->starting_fileno, metalogfile->fileno);
            continue;
        }
        if (metalogfile_read(metalogfile, buf + sizeof(*header), record_size - sizeof(*header),
                             pos + sizeof(*header)) < 0)
            break;
        if (metadata_record_integrity_check(buf)) {
            error("%s: Record at offset %"PRIu32" was read from disk. CRC32 check: FAILED", __func__, pos);
            continue;
        }
        debug(D_METADATALOG, "%s: Record at offset %"PRIu32" was read from disk. CRC32 check: SUCCEEDED", __func__,
              pos);

        replay_record(metalogfile, header, buf + header->header_length);
    }

    freez(buf);
}

int load_metadata_logfile(struct metalog_instance *ctx, struct metadata_logfile *metalogfile)
{
    UNUSED(ctx);
    uv_fs_t req;
    uv_file file;
    int ret, fd, error;
    uint64_t file_size;
    char path[RRDENG_PATH_MAX];

    generate_metadata_logfile_path(metalogfile, path, sizeof(path));
    if (file_is_migrated(path))
        return 0;

    fd = open_file_buffered_io(path, O_RDWR, &file);
    if (fd < 0) {
//        ++ctx->stats.fs_errors;
        rrd_stat_atomic_add(&global_fs_errors, 1);
        return fd;
    }
    info("Loading metadata log \"%s\".", path);

    ret = check_file_properties(file, &file_size, sizeof(struct rrdeng_metalog_sb));
    if (ret)
        goto error;

    ret = check_metadata_logfile_superblock(file);
    if (ret)
        goto error;
//    ctx->stats.io_read_bytes += sizeof(struct rrdeng_jf_sb);
//    ++ctx->stats.io_read_requests;

    metalogfile->file = file;
    metalogfile->pos = file_size;

    iterate_records(metalogfile);

    info("Metadata log \"%s\" migrated to the database (size:%"PRIu64").", path, file_size);
    add_migrated_file(path, file_size);
    return 0;

error:
    error = ret;
    ret = uv_fs_close(NULL, &req, file, NULL);
    if (ret < 0) {
        error("uv_fs_close(%s): %s", path, uv_strerror(ret));
//        ++ctx->stats.fs_errors;
        rrd_stat_atomic_add(&global_fs_errors, 1);
    }
    uv_fs_req_cleanup(&req);
    return error;
}

static int scan_metalog_files_cmp(const void *a, const void *b)
{
    struct metadata_logfile *file1, *file2;
    char path1[RRDENG_PATH_MAX], path2[RRDENG_PATH_MAX];

    file1 = *(struct metadata_logfile **)a;
    file2 = *(struct metadata_logfile **)b;
    generate_metadata_logfile_path(file1, path1, sizeof(path1));
    generate_metadata_logfile_path(file2, path2, sizeof(path2));
    return strcmp(path1, path2);
}

/* Returns number of metadata logfiles that were loaded or < 0 on error */
static int scan_metalog_files(struct metalog_instance *ctx)
{
    int ret;
    unsigned starting_no, no, matched_files, i, failed_to_load;
    static uv_fs_t req;
    uv_dirent_t dent;
    struct metadata_logfile **metalogfiles, *metalogfile;
    char *dbfiles_path = ctx->rrdeng_ctx->dbfiles_path;

    ret = uv_fs_scandir(NULL, &req, dbfiles_path, 0, NULL);
    if (ret < 0) {
        fatal_assert(req.result < 0);
        uv_fs_req_cleanup(&req);
        error("uv_fs_scandir(%s): %s", dbfiles_path, uv_strerror(ret));
//        ++ctx->stats.fs_errors;
        rrd_stat_atomic_add(&global_fs_errors, 1);
        return ret;
    }
    info("Found %d files in path %s", ret, dbfiles_path);

    metalogfiles = callocz(MIN(ret, MAX_DATAFILES), sizeof(*metalogfiles));
    for (matched_files = 0 ; UV_EOF != uv_fs_scandir_next(&req, &dent) && matched_files < MAX_DATAFILES ; ) {
        info("Scanning file \"%s/%s\"", dbfiles_path, dent.name);
        ret = sscanf(dent.name, METALOG_PREFIX METALOG_FILE_NUMBER_SCAN_TMPL METALOG_EXTENSION, &starting_no, &no);
        if (2 == ret) {
            info("Matched file \"%s/%s\"", dbfiles_path, dent.name);
            metalogfile = mallocz(sizeof(*metalogfile));
            metadata_logfile_init(metalogfile, ctx, starting_no, no);
            metalogfiles[matched_files++] = metalogfile;
        }
    }
    uv_fs_req_cleanup(&req);

    if (0 == matched_files) {
        freez(metalogfiles);
        return 0;
    }
    if (matched_files == MAX_DATAFILES) {
        error("Warning: hit maximum database engine file limit of %d files", MAX_DATAFILES);
    }
    qsort(metalogfiles, matched_files, sizeof(*metalogfiles), scan_metalog_files_cmp);
    ret = compaction_failure_recovery(ctx, metalogfiles, &matched_files);
    if (ret) { /* If the files are corrupted fail */
        for (i = 0 ; i < matched_files ; ++i) {
            freez(metalogfiles[i]);
        }
        freez(metalogfiles);
        return UV_EINVAL;
    }
    //ctx->last_fileno = metalogfiles[matched_files - 1]->fileno;

    struct plugind cd = {
        .enabled = 1,
        .update_every = 0,
        .pid = 0,
        .serial_failures = 0,
        .successful_collections = 0,
        .obsolete = 0,
        .started_t = INVALID_TIME,
        .next = NULL,
        .version = 0,
    };

    struct metalog_pluginsd_state metalog_parser_state;
    metalog_pluginsd_state_init(&metalog_parser_state, ctx);

    PARSER_USER_OBJECT metalog_parser_object;
    metalog_parser_object.enabled = cd.enabled;
    metalog_parser_object.host = ctx->rrdeng_ctx->host;
    metalog_parser_object.cd = &cd;
    metalog_parser_object.trust_durations = 0;
    metalog_parser_object.private = &metalog_parser_state;

    PARSER *parser = parser_init(metalog_parser_object.host, &metalog_parser_object, NULL, PARSER_INPUT_SPLIT);
    if (unlikely(!parser)) {
        error("Failed to initialize metadata log parser.");
        failed_to_load = matched_files;
        goto after_failed_to_parse;
    }
    parser_add_keyword(parser, PLUGINSD_KEYWORD_HOST, metalog_pluginsd_host);
    parser_add_keyword(parser, PLUGINSD_KEYWORD_GUID, pluginsd_guid);
    parser_add_keyword(parser, PLUGINSD_KEYWORD_CONTEXT, pluginsd_context);
    parser_add_keyword(parser, PLUGINSD_KEYWORD_TOMBSTONE, pluginsd_tombstone);
    parser->plugins_action->dimension_action = &metalog_pluginsd_dimension_action;
    parser->plugins_action->chart_action     = &metalog_pluginsd_chart_action;
    parser->plugins_action->guid_action      = &metalog_pluginsd_guid_action;
    parser->plugins_action->context_action   = &metalog_pluginsd_context_action;
    parser->plugins_action->tombstone_action = &metalog_pluginsd_tombstone_action;
    parser->plugins_action->host_action      = &metalog_pluginsd_host_action;


    metalog_parser_object.parser = parser;
    ctx->metalog_parser_object = &metalog_parser_object;

    for (failed_to_load = 0, i = 0 ; i < matched_files ; ++i) {
        metalogfile = metalogfiles[i];
        db_lock();
        db_execute("BEGIN TRANSACTION;");
        ret = load_metadata_logfile(ctx, metalogfile);
        if (0 != ret) {
            error("Deleting invalid metadata log file \"%s/"METALOG_PREFIX METALOG_FILE_NUMBER_PRINT_TMPL
                      METALOG_EXTENSION"\"", dbfiles_path, metalogfile->starting_fileno, metalogfile->fileno);
            unlink_metadata_logfile(metalogfile);
            ++failed_to_load;
            db_execute("ROLLBACK TRANSACTION;");
        }
        else
            db_execute("COMMIT TRANSACTION;");
        db_unlock();
        freez(metalogfile);
    }
    matched_files -= failed_to_load;
    debug(D_METADATALOG, "PARSER ended");

    parser_destroy(parser);

    size_t count __maybe_unused = metalog_parser_object.count;

    debug(D_METADATALOG, "Parsing count=%u", (unsigned)count);
after_failed_to_parse:

    freez(metalogfiles);

    return matched_files;
}

/* Return 0 on success. */
int init_metalog_files(struct metalog_instance *ctx)
{
    int ret;
    char *dbfiles_path = ctx->rrdeng_ctx->dbfiles_path;

    ret = scan_metalog_files(ctx);
    if (ret < 0) {
        error("Failed to scan path \"%s\".", dbfiles_path);
        return ret;
    }/* else if (0 == ret) {
        ctx->last_fileno = 1;
    }*/

    return 0;
}
