// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_DATAFILE_H
#define NETDATA_DATAFILE_H

#include "rrdengine.h"

/* Forward declarations */
struct dbengine_datafile;
struct dbengine_journalfile;
struct dbengine_tier;

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

typedef enum __attribute__ ((__packed__)) {
    DATAFILE_ACQUIRE_OPEN_CACHE = 0,
    DATAFILE_ACQUIRE_PAGE_DETAILS,
    DATAFILE_ACQUIRE_RETENTION,
    DATAFILE_ACQUIRE_INDEXING,
    DATAFILE_ACQUIRE_MRG_LOAD,                  // the metrics registry is being populated from its journal

    // terminator
    DATAFILE_ACQUIRE_MAX,
} DATAFILE_ACQUIRE_REASONS;

struct extent_page_details_list;
typedef struct {
    SPINLOCK spinlock;
    struct extent_page_details_list *base;
} EPDL_EXTENT;
void epdl_extent_release(EPDL_EXTENT *e);

#define DATAFILE_MAGIC 0xDA7AF11E

/* only one event loop is supported for now */
struct dbengine_datafile {
    uint32_t magic1;

    unsigned tier;
    unsigned fileno;
    uv_file file;
    uint64_t pos;
    netdata_rwlock_t extent_rwlock;
    struct dbengine_tier *ctx;
    struct dbengine_journalfile *journalfile;

    struct {
        SPINLOCK spinlock;
        bool populated;
    } populate_mrg;

    struct {
        SPINLOCK spinlock;
        size_t running;
        size_t flushed_to_open_running;
        bool failed; // a write to this datafile failed unrecoverably - report it full to force rotation
#ifdef OS_WINDOWS
        time_t last_sync_time;
#endif
    } writers;

    struct {
        SPINLOCK_TRACKED spinlock;  // tracked: 3600s deadlock fatals here are un-triagable without holder identity
        unsigned lockers;
        unsigned lockers_by_reason[DATAFILE_ACQUIRE_MAX];
        bool available;
        bool pending_deletion;
    } users;

    struct {
        RW_SPINLOCK spinlock;
        Pvoid_t epdl_per_extent;
    } extent_epdl;

    uint32_t magic2;
};

bool datafile_acquire(struct dbengine_datafile *df, DATAFILE_ACQUIRE_REASONS reason);
void datafile_release_with_trace(struct dbengine_datafile *df, DATAFILE_ACQUIRE_REASONS reason, const char *func);
#define datafile_release(df, reason) datafile_release_with_trace(df, reason, __FUNCTION__)
bool datafile_acquire_for_deletion(struct dbengine_datafile *df);

void datafile_list_insert(struct dbengine_tier *ctx, struct dbengine_datafile *datafile);
void datafile_list_delete_unsafe(struct dbengine_tier *ctx, struct dbengine_datafile *datafile);
void generate_datafilepath(struct dbengine_datafile *datafile, char *str, size_t maxlen);
int close_data_file(struct dbengine_datafile *datafile);
int unlink_data_file(struct dbengine_datafile *datafile);
int destroy_data_file_unsafe(struct dbengine_datafile *datafile);
int create_data_file(struct dbengine_datafile *datafile);
int create_new_datafile_pair(struct dbengine_tier *ctx);
int init_data_files(struct dbengine_tier *ctx);
void finalize_data_files(struct dbengine_tier *ctx);
void cleanup_datafile_epdl_structures(struct dbengine_datafile *datafile);

NEVERNULL ALWAYS_INLINE
static struct dbengine_tier *datafile_ctx(struct dbengine_datafile *datafile) {
    if(unlikely(!datafile->ctx))
        fatal("DBENGINE: datafile %u of tier %u has no ctx", datafile->fileno, datafile->tier);

    if(unlikely(datafile->magic1 != DATAFILE_MAGIC || datafile->magic2 != DATAFILE_MAGIC))
        fatal("DBENGINE: datafile %u of tier %u has invalid magic", datafile->fileno, datafile->tier);

    return datafile->ctx;
}

#ifdef OS_WINDOWS
void sync_uv_file_data(uv_file file);
#endif

#endif /* NETDATA_DATAFILE_H */
