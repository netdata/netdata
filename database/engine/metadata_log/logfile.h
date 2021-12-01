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

/* only one event loop is supported for now */
struct metadata_logfile {
    unsigned fileno; /* Starts at 1 */
    unsigned starting_fileno; /* 0 for normal files, staring number during compaction */
    uv_file file;
    uint64_t pos;
    struct metalog_instance *ctx;
    struct metadata_logfile *next;
};

struct metadata_logfile_list {
    struct metadata_logfile *first; /* oldest */
    struct metadata_logfile *last; /* newest */
};

extern void generate_metadata_logfile_path(struct metadata_logfile *metadatalog, char *str, size_t maxlen);
extern int rename_metadata_logfile(struct metadata_logfile *metalogfile, unsigned new_starting_fileno,
                                   unsigned new_fileno);
extern int unlink_metadata_logfile(struct metadata_logfile *metalogfile);
extern int load_metadata_logfile(struct metalog_instance *ctx, struct metadata_logfile *logfile);
extern int init_metalog_files(struct metalog_instance *ctx);


#endif /* NETDATA_LOGFILE_H */
