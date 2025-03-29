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

#define MIN_DATAFILE_SIZE   (512LU * 1024LU)
#ifndef MAX_DATAFILE_SIZE
#define MAX_DATAFILE_SIZE   (1LLU * 1024LLU * 1024LLU * 1024LLU)
#endif
#if  MIN_DATAFILE_SIZE > MAX_DATAFILE_SIZE
#error MIN_DATAFILE_SIZE > MAX_DATAFILE_SIZE
#endif

#define MAX_DATAFILES (65536 * 4) /* Supports up to 64TiB for now */
#define TARGET_DATAFILES (100)

// When trying to acquire a datafile for deletion and an attempt to evict pages is completed
// the acquire for deletion will return true after this timeout
#define DATAFILE_DELETE_TIMEOUT_SHORT (1)
#define DATAFILE_DELETE_TIMEOUT_LONG (120)

typedef enum __attribute__ ((__packed__)) {
    DATAFILE_ACQUIRE_OPEN_CACHE = 0,
    DATAFILE_ACQUIRE_PAGE_DETAILS,
    DATAFILE_ACQUIRE_RETENTION,

    // terminator
    DATAFILE_ACQUIRE_MAX,
} DATAFILE_ACQUIRE_REASONS;

struct extent_page_details_list;
typedef struct {
    SPINLOCK spinlock;
    struct extent_page_details_list *base;
} EPDL_EXTENT;

#define DATAFILE_MAGIC 0xDA7AF11E

/* only one event loop is supported for now */
struct rrdengine_datafile {
    uint32_t magic1;

    unsigned tier;
    unsigned fileno;
    uv_file file;
    uint64_t pos;
    uv_rwlock_t extent_rwlock;
    struct rrdengine_instance *ctx;
    struct rrdengine_journalfile *journalfile;
    struct rrdengine_datafile *prev;
    struct rrdengine_datafile *next;

    struct {
        SPINLOCK spinlock;
        bool populated;
    } populate_mrg;

    struct {
        SPINLOCK spinlock;
        size_t running;
        size_t flushed_to_open_running;
    } writers;

    struct {
        SPINLOCK spinlock;
        unsigned lockers;
        unsigned lockers_by_reason[DATAFILE_ACQUIRE_MAX];
        bool available;
        time_t time_to_evict;
    } users;

    struct {
        RW_SPINLOCK spinlock;
        Pvoid_t epdl_per_extent;
    } extent_epdl;

    uint32_t magic2;
};

bool datafile_acquire(struct rrdengine_datafile *df, DATAFILE_ACQUIRE_REASONS reason);
void datafile_release_with_trace(struct rrdengine_datafile *df, DATAFILE_ACQUIRE_REASONS reason, const char *func);
#define datafile_release(df, reason) datafile_release_with_trace(df, reason, __FUNCTION__)
bool datafile_acquire_for_deletion(struct rrdengine_datafile *df, bool is_shutdown);

void datafile_list_insert(struct rrdengine_instance *ctx, struct rrdengine_datafile *datafile, bool having_lock);
void datafile_list_delete_unsafe(struct rrdengine_instance *ctx, struct rrdengine_datafile *datafile);
void generate_datafilepath(struct rrdengine_datafile *datafile, char *str, size_t maxlen);
int close_data_file(struct rrdengine_datafile *datafile);
int unlink_data_file(struct rrdengine_datafile *datafile);
int destroy_data_file_unsafe(struct rrdengine_datafile *datafile);
int create_data_file(struct rrdengine_datafile *datafile);
int create_new_datafile_pair(struct rrdengine_instance *ctx, bool having_lock);
int init_data_files(struct rrdengine_instance *ctx);
void finalize_data_files(struct rrdengine_instance *ctx);

NEVERNULL ALWAYS_INLINE
static struct rrdengine_instance *datafile_ctx(struct rrdengine_datafile *datafile) {
    if(unlikely(!datafile->ctx))
        fatal("DBENGINE: datafile %u of tier %u has no ctx", datafile->fileno, datafile->tier);

    if(unlikely(datafile->magic1 != DATAFILE_MAGIC || datafile->magic2 != DATAFILE_MAGIC))
        fatal("DBENGINE: datafile %u of tier %u has invalid magic", datafile->fileno, datafile->tier);

    return datafile->ctx;
}

#endif /* NETDATA_DATAFILE_H */