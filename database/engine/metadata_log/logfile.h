// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_LOGFILE_H
#define NETDATA_LOGFILE_H

#include "metadatalogprotocol.h"
#include "../rrdengine.h"

/* Forward declarations */
struct rrdengine_metadata_log;
struct rrdengine_instance;
struct rrdengine_worker_config;

#define METALOG_PREFIX "metadatalog-"
#define METALOG_EXTENSION ".mlf"

struct metalog_record_info {
    uint64_t offset;
    uint32_t size;
    uint8_t number_of_pages;
    struct rrdengine_metadata_log *metadatalog;
    struct metalog_record_info *next;
};

struct rrdengine_metalog_records {
    /* the record list is sorted based on disk offset */
    struct metalog_record_info *first;
    struct metalog_record_info *last;
};


/* only one event loop is supported for now */
struct rrdengine_metadata_log {
    unsigned tier;
    unsigned fileno;
    uv_file file;
    uint64_t pos;
    struct rrdengine_instance *ctx;
    struct rrdengine_metalog_records records;
    struct rrdengine_metadata_log *next;
};

struct rrdengine_metadata_log_list {
    struct rrdengine_metadata_log *first; /* oldest */
    struct rrdengine_metadata_log *last; /* newest */
};

/* only one event loop is supported for now */
struct metadata_record_commit_log {
    uint64_t record_id;

    /* outstanding record buffer */
    void *buf;
    unsigned buf_pos;
    unsigned buf_size;
};

extern void generate_metadata_log_path(struct rrdengine_metadata_log *metadatalog, char *str, size_t maxlen);
extern void metadata_log_init(struct rrdengine_metadata_log *metadatalog, struct rrdengine_instance *ctx,
                              unsigned tier, unsigned fileno);
extern int close_metadata_log(struct rrdengine_metadata_log *metadatalog);
extern int destroy_metadata_log(struct rrdengine_metadata_log *metadatalog);
extern int create_metadata_log(struct rrdengine_metadata_log *metadatalog);
extern int load_metadata_log(struct rrdengine_instance *ctx,struct rrdengine_metadata_log *metadatalog);
extern void init_metadata_record_log(struct rrdengine_instance *ctx);


#endif /* NETDATA_LOGFILE_H */