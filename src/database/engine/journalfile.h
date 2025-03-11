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

/*
 * Journal File V2 Format
 *
 * File Layout:
 * +------------------------------------------------+
 * | HEADER SECTION                                 |
 * |   +--------------------------------------------+
 * |   | Header (72 bytes)                          |
 * |   |   - magic, times, counts, offsets          |
 * |   +--------------------------------------------+
 * |   | Padding (to 4096 bytes)                    |
 * +------------------------------------------------+
 * | EXTENT SECTION                                 |
 * |   +--------------------------------------------+
 * |   | Extent Items                               |
 * |   |   - Array of Extent Entries (16 bytes each)|
 * |   +--------------------------------------------+
 * |   | Extent Section Trailer (4 bytes CRC)       |
 * +------------------------------------------------+
 * | METRICS SECTION                                |
 * |   +--------------------------------------------+
 * |   | Metric List                                |
 * |   |   - Array of Metric Entries (36 bytes each)|
 * |   +--------------------------------------------+
 * |   | Metric List Trailer (4 bytes CRC)          |
 * +------------------------------------------------+
 * | PAGE DATA SECTION                              |
 * |   +--------------------------------------------+
 * |   | For each metric:                           |
 * |   |   - Page Header (28 bytes)                 |
 * |   |   - Page Entries (20 bytes each)           |
 * |   |   - Page Trailer (4 bytes CRC)             |
 * +------------------------------------------------+
 * | FILE TRAILER                                   |
 * |   - File CRC (4 bytes)                         |
 * +------------------------------------------------+
 *
 * Data Integrity:
 * - All sections have CRC32 checksums
 * - The file header CRC is stored in the file trailer
 * - Each section (extent list, metric list) has its own trailer with CRC
 * - Each page header includes a CRC of its content
 *
 * Time Representation:
 * - File-level times are stored in microseconds (start_time_ut, end_time_ut)
 * - Page-level times are stored as deltas in seconds relative to the journal start time
 */

// Journal v2 header (72 bytes)
struct journal_v2_header {
    // File type identifier
    //   - 0x01230317: Normal journal file
    //   - 0x00230317: File needs rebuild
    //   - 0x02230317: File should be skipped
    uint32_t magic;

    // --- implicit padding of 4-bytes because the struct is not packed ---

    // Mininimum start time in microseconds
    usec_t start_time_ut;
    // Maximum end time in microseconds
    usec_t end_time_ut;

    // Number of extents
    uint32_t extent_count;
    // Offset to extents section
    uint32_t extent_offset;

    // Number of metrics
    uint32_t metric_count;
    // Offset to metrics section
    uint32_t metric_offset;

    // Total count of pages
    uint32_t page_count;
    // Offset to page data section
    uint32_t page_offset;

    // Offset to extents section CRC
    uint32_t extent_trailer_offset;
    // Offset to metrics section CRC
    uint32_t metric_trailer_offset;

    // Size of original journal file
    uint32_t journal_v1_file_size;
    // Total file size
    uint32_t journal_v2_file_size;

    // Pointer used only when writing to build up the memory-mapped file.
    void *data;
};

// Reserve the first 4 KiB for the journal v2 header
#define JOURNAL_V2_HEADER_PADDING_SZ (RRDENG_BLOCK_SIZE - (sizeof(struct journal_v2_header)))

// Extent section item (16 bytes)
struct journal_extent_list {
    // Extent offset in datafile
    uint64_t datafile_offset;

    // Size of the extent
    uint32_t datafile_size;

    // Index of the data file
    uint16_t file_index;

    // Number of pages in the extent (not all are necesssarily valid)
    uint8_t  pages;

    // --- implicit padding of 1-byte because the struct is not packed ---
};

// Metric section item (36 bytes)
struct journal_metric_list {
    // Unique identifier of the metric
    nd_uuid_t uuid;

    // Number of pages for this metric
    uint32_t entries;

    // Offset to the page data section
    //   Points to: journal_page_header + (entries * journal_page_list)
    uint32_t page_offset;

    // Start time relative to journal start
    uint32_t delta_start_s;

    // End time relative to journal start
    uint32_t delta_end_s;

    // Last update every for this metric in this journal (last page collected)
    uint32_t update_every_s;
};

// Page section item header (28 bytes)
struct journal_page_header {
    // CRC32 of the header
    union {
        uint8_t checksum[CHECKSUM_SZ];
        uint32_t crc;
    };

    // Offset to corresponding metric in metric list
    uint32_t uuid_offset;

    // Number of page items that follow
    uint32_t entries;

    // UUID of the metric
    nd_uuid_t uuid;
};

// Page section item (20 bytes)
struct journal_page_list {
    // Start time relative to journal start
    uint32_t delta_start_s;

    // End time relative to journal start
    uint32_t delta_end_s;

    // Offset into extent section
    uint32_t extent_index;

    // Update frequency
    uint32_t update_every_s;

    // Length of the page
    uint16_t page_length;

    // Page type identifier
    uint8_t type;

    // --- implicit padding of 1-byte because the struct is not packed ---
};

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
