// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_JOURNALFILE_H
#define NETDATA_JOURNALFILE_H

#include "rrdengine.h"
#include "journalfile_metric_hash_table.h"

/* Forward declarations */
struct rrdengine_instance;
struct rrdengine_datafile;
struct rrdengine_journalfile;

#define WALFILE_PREFIX "journalfile-"
#define WALFILE_EXTENSION ".njf"
#define WALFILE_EXTENSION_V2 ".njfv2"

#define is_descr_journal_v2(descr) ((descr)->extent_entry != NULL)

typedef enum __attribute__ ((__packed__)) {
    JOURNALFILE_FLAG_IS_AVAILABLE          = (1 << 0),
    JOURNALFILE_FLAG_IS_MOUNTED            = (1 << 1),
    JOURNALFILE_FLAG_MOUNTED_FOR_RETENTION = (1 << 2),
    JOURNALFILE_FLAG_METRIC_CRC_CHECK      = (1 << 3),
} JOURNALFILE_FLAGS;

/* only one event loop is supported for now */
struct rrdengine_journalfile {
    struct {
        SPINLOCK spinlock;
        void *data;                    // MMAPed file of journal v2
        uint32_t size;                 // Total file size mapped
        int fd;
    } mmap;

    struct {
        SPINLOCK spinlock;
        JOURNALFILE_FLAGS flags;
        int32_t refcount;
        time_t first_time_s;
        time_t last_time_s;
        time_t not_needed_since_s;
        uint32_t size_of_directory;
    } v2;

    struct {
        Word_t indexed_as;
    } njfv2idx;

    struct {
        SPINLOCK spinlock;
        uint64_t pos;
    } unsafe;

    uv_file file;
    struct rrdengine_datafile *datafile;
};

static inline uint64_t journalfile_current_size(struct rrdengine_journalfile *journalfile) {
    spinlock_lock(&journalfile->unsafe.spinlock);
    uint64_t size = journalfile->unsafe.pos;
    spinlock_unlock(&journalfile->unsafe.spinlock);
    return size;
}

struct wal;

void journalfile_v1_generate_path(struct rrdengine_datafile *datafile, char *str, size_t maxlen);
void journalfile_v2_generate_path(struct rrdengine_datafile *datafile, char *str, size_t maxlen);
struct rrdengine_journalfile *journalfile_alloc_and_init(struct rrdengine_datafile *datafile);
void journalfile_v1_extent_write(struct rrdengine_instance *ctx, struct rrdengine_datafile *datafile, struct wal *wal);
int journalfile_close(struct rrdengine_journalfile *journalfile, struct rrdengine_datafile *datafile);
int journalfile_unlink(struct rrdengine_journalfile *journalfile);
int journalfile_destroy_unsafe(struct rrdengine_journalfile *journalfile, struct rrdengine_datafile *datafile);
int journalfile_create(struct rrdengine_journalfile *journalfile, struct rrdengine_datafile *datafile);
int journalfile_load(struct rrdengine_instance *ctx, struct rrdengine_journalfile *journalfile,
                     struct rrdengine_datafile *datafile);
void journalfile_v2_populate_retention_to_mrg(struct rrdengine_instance *ctx, struct rrdengine_journalfile *journalfile);

void journalfile_migrate_to_v2_callback(Word_t section, unsigned datafile_fileno __maybe_unused, uint8_t type __maybe_unused,
                                        Pvoid_t JudyL_metrics, Pvoid_t JudyL_extents_pos,
                                        size_t number_of_extents, size_t number_of_metrics, size_t number_of_pages, void *user_data);


bool journalfile_v2_data_available(struct rrdengine_journalfile *journalfile);
size_t journalfile_v2_data_size_get(struct rrdengine_journalfile *journalfile);
void journalfile_v2_data_set(struct rrdengine_journalfile *journalfile, int fd, void *journal_data, uint32_t journal_data_size);
struct journal_v2_header *journalfile_v2_data_acquire(struct rrdengine_journalfile *journalfile, size_t *data_size, time_t wanted_first_time_s, time_t wanted_last_time_s);
void journalfile_v2_data_release(struct rrdengine_journalfile *journalfile);
void journalfile_v2_data_unmount_cleanup(time_t now_s);


size_t journalfile_v2_metrics_bytes_count(const struct journal_v2_header *j2_header);
struct journal_metric_list *journalfile_v2_metrics_lookup(const struct journal_v2_header *j2_header, const nd_uuid_t *uuid);

typedef struct {
    bool init;
    Word_t last;
    time_t wanted_start_time_s;
    time_t wanted_end_time_s;
    struct rrdengine_instance *ctx;
    struct journal_v2_header *j2_header_acquired;
} NJFV2IDX_FIND_STATE;

struct rrdengine_datafile *njfv2idx_find_and_acquire_j2_header(NJFV2IDX_FIND_STATE *s);

#endif /* NETDATA_JOURNALFILE_H */
