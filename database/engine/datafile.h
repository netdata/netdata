// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_DATAFILE_H
#define NETDATA_DATAFILE_H

#include "rrdengine.h"

/* Forward declarations */
struct rrdengine_datafile;
struct rrdengine_journalfile;
struct rrdengine_instance;

#define DATAFILE_PREFIX "datafile-"
#define DATAFILE_EXTENSION ".ndf"

#define MAX_DATAFILE_SIZE   (1073741824LU)
#define MIN_DATAFILE_SIZE   (4194304LU)
#define MAX_DATAFILES (65536) /* Supports up to 64TiB for now */
#define TARGET_DATAFILES (20)

#define DATAFILE_IDEAL_IO_SIZE (1048576U)

struct extent_info {
    uint64_t offset;
    uint32_t size;
    uint8_t number_of_pages;
    struct rrdengine_datafile *datafile;
    struct extent_info *next;
    struct rrdeng_page_descr *pages[];
};

struct rrdengine_df_extents {
    /* the extent list is sorted based on disk offset */
    struct extent_info *first;
    struct extent_info *last;
};

/* only one event loop is supported for now */
struct rrdengine_datafile {
    unsigned tier;
    unsigned fileno;
    uv_file file;
    uint64_t pos;
    struct rrdengine_instance *ctx;
    struct rrdengine_df_extents extents;
    struct rrdengine_journalfile *journalfile;
    struct rrdengine_datafile *next;
};

struct rrdengine_datafile_list {
    struct rrdengine_datafile *first; /* oldest */
    struct rrdengine_datafile *last; /* newest */
};

extern void df_extent_insert(struct extent_info *extent);
extern void datafile_list_insert(struct rrdengine_instance *ctx, struct rrdengine_datafile *datafile);
extern void datafile_list_delete(struct rrdengine_instance *ctx, struct rrdengine_datafile *datafile);
extern void generate_datafilepath(struct rrdengine_datafile *datafile, char *str, size_t maxlen);
extern int close_data_file(struct rrdengine_datafile *datafile);
extern int unlink_data_file(struct rrdengine_datafile *datafile);
extern int destroy_data_file(struct rrdengine_datafile *datafile);
extern int create_data_file(struct rrdengine_datafile *datafile);
extern int create_new_datafile_pair(struct rrdengine_instance *ctx, unsigned tier, unsigned fileno);
extern int init_data_files(struct rrdengine_instance *ctx);
extern void finalize_data_files(struct rrdengine_instance *ctx);

#endif /* NETDATA_DATAFILE_H */