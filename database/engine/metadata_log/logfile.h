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

#define MAX_METALOGFILE_SIZE   (1073741824LU)
#define MIN_METALOGFILE_SIZE   (16777216LU)
#define TARGET_METALOGFILES (20)

struct metalog_record_info {
    uint64_t offset;
    uint32_t size;
    struct metadata_logfile *metalogfile;
    struct metalog_record_info *next;
};

struct metalog_records {
    /* the record list is sorted based on disk offset */
    struct metalog_record_info *first;
    struct metalog_record_info *last;
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

/* only one event loop is supported for now */
struct metadata_record_commit_log {
    uint64_t record_id;

    /* outstanding record buffer */
    void *buf;
    unsigned buf_pos;
    unsigned buf_size;
};

extern void mlf_record_insert(struct metalog_record_info *record);
extern void mlf_flush_records_buffer(struct metalog_worker_config *wc);
extern void *mlf_get_records_buffer(struct metalog_worker_config *wc, unsigned size);
extern void metadata_logfile_list_insert(struct metalog_instance *ctx, struct metadata_logfile *metalogfile);
extern void metadata_logfile_list_delete(struct metalog_instance *ctx, struct metadata_logfile *metalogfile);
extern void generate_metadata_logfile_path(struct metadata_logfile *metadatalog, char *str, size_t maxlen);
extern void metadata_logfile_init(struct metadata_logfile *metadatalog, struct metalog_instance *ctx,
                              unsigned tier, unsigned fileno);
extern int close_metadata_logfile(struct metadata_logfile *metadatalog);
extern int destroy_metadata_logfile(struct metadata_logfile *metalogfile);
extern int create_metadata_logfile(struct metadata_logfile *metalogfile);
extern int load_metadata_logfile(struct metalog_instance *ctx, struct metadata_logfile *logfile);
extern void init_metadata_record_log(struct metalog_instance *ctx);
extern int add_new_metadata_logfile(struct metalog_instance *ctx, unsigned tier, unsigned fileno);
extern int init_metalog_files(struct metalog_instance *ctx);
extern void finalize_metalog_files(struct metalog_instance *ctx);


#endif /* NETDATA_LOGFILE_H */
