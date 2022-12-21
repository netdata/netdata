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

#ifndef MAX_DATAFILE_SIZE
#define MAX_DATAFILE_SIZE   (1073741824LU)
#endif
#if  MIN_DATAFILE_SIZE > MAX_DATAFILE_SIZE
#error MIN_DATAFILE_SIZE > MAX_DATAFILE_SIZE
#endif

#define MIN_DATAFILE_SIZE   (4194304LU)
#define MAX_DATAFILES (65536) /* Supports up to 64TiB for now */
#define TARGET_DATAFILES (20)

#define DATAFILE_IDEAL_IO_SIZE (1048576U)

/* only one event loop is supported for now */
struct rrdengine_datafile {
    unsigned tier;
    unsigned fileno;
    uv_file file;
    uint64_t pos;
    uv_rwlock_t extent_rwlock;
    struct rrdengine_instance *ctx;
    struct rrdengine_journalfile *journalfile;
    struct rrdengine_datafile *prev;
    struct rrdengine_datafile *next;

    // exclusive access to extents
    struct {
        SPINLOCK spinlock;
        unsigned lockers;
        Pvoid_t extents_JudyL;
    } extent_exclusive_access;

    struct {
        SPINLOCK spinlock;
        unsigned lockers;
        bool available;
    } users;
};

void datafile_acquire_dup(struct rrdengine_datafile *df);
bool datafile_acquire(struct rrdengine_datafile *df);
void datafile_release(struct rrdengine_datafile *df);
bool datafile_acquire_for_deletion(struct rrdengine_datafile *df, bool wait);

struct rrdengine_datafile_list {
    uv_rwlock_t rwlock;
    struct rrdengine_datafile *first; /* oldest - the newest with ->first->prev */
};

void datafile_list_insert(struct rrdengine_instance *ctx, struct rrdengine_datafile *datafile);
void datafile_list_delete_unsafe(struct rrdengine_instance *ctx, struct rrdengine_datafile *datafile);
void generate_datafilepath(struct rrdengine_datafile *datafile, char *str, size_t maxlen);
int close_data_file(struct rrdengine_datafile *datafile);
int unlink_data_file(struct rrdengine_datafile *datafile);
int destroy_data_file_unsafe(struct rrdengine_datafile *datafile);
int create_data_file(struct rrdengine_datafile *datafile);
int create_new_datafile_pair(struct rrdengine_instance *ctx, unsigned tier, unsigned fileno);
int init_data_files(struct rrdengine_instance *ctx);
void finalize_data_files(struct rrdengine_instance *ctx);

#endif /* NETDATA_DATAFILE_H */