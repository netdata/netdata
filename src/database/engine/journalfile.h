// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_JOURNALFILE_H
#define NETDATA_JOURNALFILE_H

#include "rrdengine.h"

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

// Journal v2 structures

#define JOURVAL_V2_MAGIC           (0x01230317)
#define JOURVAL_V2_REBUILD_MAGIC   (0x00230317)
#define JOURVAL_V2_SKIP_MAGIC      (0x02230317)

struct journal_v2_block_trailer {
    union {
        uint8_t checksum[CHECKSUM_SZ]; /* CRC32 */
        uint32_t crc;
    };
};

// Journal V2
// 28 bytes
struct journal_page_header {
    union {
        uint8_t checksum[CHECKSUM_SZ];      // CRC check
        uint32_t crc;
    };
    uint32_t uuid_offset;    // Points back to the UUID list which should point here (UUIDs should much)
    uint32_t entries;        // Entries
    nd_uuid_t   uuid;           // Which UUID this is
};

// 20 bytes
struct journal_page_list {
    uint32_t delta_start_s;    // relative to the start time of journal
    uint32_t delta_end_s;      // relative to delta_start
    uint32_t extent_index;     // Index to the extent (extent list) (bytes from BASE)
    uint32_t update_every_s;
    uint16_t page_length;
    uint8_t type;
};

// UUID_LIST
// 36 bytes
struct journal_metric_list {
    nd_uuid_t uuid;
    uint32_t entries;           // Number of entries
    uint32_t page_offset;       // OFFSET that contains entries * struct( journal_page_list )
    uint32_t delta_start_s;     // Min time of metric
    uint32_t delta_end_s;       // Max time of metric  (to be used to populate page_index)
    uint32_t update_every_s;    // Last update every for this metric in this journal (last page collected)
};

// 16 bytes
struct journal_extent_list {
    uint64_t datafile_offset;   // Datafile offset to find the extent
    uint32_t datafile_size;     // Size of the extent
    uint16_t file_index;        // which file index is this datafile[index]
    uint8_t  pages;             // number of pages (not all are necesssarily valid)
};

// 72 bytes
struct journal_v2_header {
    uint32_t magic;
    usec_t start_time_ut;               // Min start time of journal
    usec_t end_time_ut;                 // Maximum end time of journal
    uint32_t extent_count;              // Count of extents
    uint32_t extent_offset;
    uint32_t metric_count;              // Count of metrics (unique UUIDS)
    uint32_t metric_offset;
    uint32_t page_count;                // Total count of pages (descriptors @ time)
    uint32_t page_offset;
    uint32_t extent_trailer_offset;     // CRC for entent list
    uint32_t metric_trailer_offset;     // CRC for metric list
    uint32_t journal_v1_file_size;      // This is the original journal file
    uint32_t journal_v2_file_size;      // This is the total file size
    void *data;                         // Used when building the index
};

#define JOURNAL_V2_HEADER_PADDING_SZ (RRDENG_BLOCK_SIZE - (sizeof(struct journal_v2_header)))

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