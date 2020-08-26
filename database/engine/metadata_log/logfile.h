// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_LOGFILE_H
#define NETDATA_LOGFILE_H

#include "metadatalogprotocol.h"
#include "../rrdengine.h"

/* Forward declarations */
struct metadata_logfile;
struct metalog_worker_config;

#define METALOG_PREFIX "metadatalog-"
#define METALOG_EXTENSION ".mlf"

#define MAX_METALOGFILE_SIZE   (524288LU)

/* Deletions are ignored during compaction, so only creation UUIDs are stored */
struct metalog_record {
    uuid_t uuid;
};

#define MAX_METALOG_RECORDS_PER_BLOCK   (1024LU)
struct metalog_record_block {
    uint64_t file_offset;
    uint32_t io_size;

    struct metalog_record record_array[MAX_METALOG_RECORDS_PER_BLOCK];
    uint16_t records_nr;

    struct metalog_record_block *next;
};

struct metalog_records {
    /* the record block list is sorted based on disk offset */
    struct metalog_record_block *first;
    struct metalog_record_block *last;
    struct {
        struct metalog_record_block *current;
        uint16_t record_i;
    } iterator;
};

/* only one event loop is supported for now */
struct metadata_logfile {
    unsigned fileno; /* Starts at 1 */
    unsigned starting_fileno; /* 0 for normal files, staring number during compaction */
    uv_file file;
    uint64_t pos;
    struct metalog_instance *ctx;
    struct metalog_records records;
    struct metadata_logfile *next;
};

struct metadata_logfile_list {
    struct metadata_logfile *first; /* oldest */
    struct metadata_logfile *last; /* newest */
};

struct metadata_record_commit_log {
    uint64_t record_id;

    /* outstanding record buffer */
    void *buf;
    unsigned buf_pos;
    unsigned buf_size;
};

extern void mlf_record_insert(struct metadata_logfile *metalogfile, struct metalog_record *record);
extern struct metalog_record *mlf_record_get_first(struct metadata_logfile *metalogfile);
extern struct metalog_record *mlf_record_get_next(struct metadata_logfile *metalogfile);
extern void mlf_flush_records_buffer(struct metalog_worker_config *wc, struct metadata_record_commit_log *records_log,
                                     struct metadata_logfile_list *metadata_logfiles);
extern void *mlf_get_records_buffer(struct metalog_worker_config *wc, struct metadata_record_commit_log *records_log,
                                    struct metadata_logfile_list *metadata_logfiles, unsigned size);
extern void metadata_logfile_list_insert(struct metadata_logfile_list *metadata_logfiles,
                                         struct metadata_logfile *metalogfile);
extern void metadata_logfile_list_delete(struct metadata_logfile_list *metadata_logfiles,
                                         struct metadata_logfile *metalogfile);
extern void generate_metadata_logfile_path(struct metadata_logfile *metadatalog, char *str, size_t maxlen);
extern void metadata_logfile_init(struct metadata_logfile *metadatalog, struct metalog_instance *ctx,
                              unsigned tier, unsigned fileno);
extern int rename_metadata_logfile(struct metadata_logfile *metalogfile, unsigned new_starting_fileno,
                                   unsigned new_fileno);
extern int close_metadata_logfile(struct metadata_logfile *metadatalog);
extern int fsync_metadata_logfile(struct metadata_logfile *metalogfile);
extern int unlink_metadata_logfile(struct metadata_logfile *metalogfile);
extern int destroy_metadata_logfile(struct metadata_logfile *metalogfile);
extern int create_metadata_logfile(struct metadata_logfile *metalogfile);
extern int load_metadata_logfile(struct metalog_instance *ctx, struct metadata_logfile *logfile);
extern void init_metadata_record_log(struct metadata_record_commit_log *records_log);
extern int add_new_metadata_logfile(struct metalog_instance *ctx, struct metadata_logfile_list *logfile_list,
                                    unsigned tier, unsigned fileno);
extern int init_metalog_files(struct metalog_instance *ctx);
extern void finalize_metalog_files(struct metalog_instance *ctx);


#endif /* NETDATA_LOGFILE_H */
