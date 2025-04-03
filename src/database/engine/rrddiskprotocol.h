// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_RRDDISKPROTOCOL_H
#define NETDATA_RRDDISKPROTOCOL_H

#include "libnetdata/libnetdata.h"

#define RRDENG_BLOCK_SIZE (4096)
#define RRDFILE_ALIGNMENT RRDENG_BLOCK_SIZE

#define RRDENG_MAGIC_SZ (32)
#define RRDENG_DF_MAGIC "netdata-data-file"
#define RRDENG_JF_MAGIC "netdata-journal-file"

#define RRDENG_VER_SZ (16)
#define RRDENG_DF_VER "1.0"
#define RRDENG_JF_VER "1.0"

#define UUID_SZ (16)
#define CHECKSUM_SZ (4) /* CRC32 */

#define RRDENG_COMPRESSION_NONE (0)
#define RRDENG_COMPRESSION_LZ4  (1)
#define RRDENG_COMPRESSION_ZSTD (2)

#define RRDENG_DF_SB_PADDING_SZ (RRDENG_BLOCK_SIZE - (RRDENG_MAGIC_SZ + RRDENG_VER_SZ + sizeof(uint8_t)))

/*
 * Data file persistent super-block
 */

struct rrdeng_df_sb {
    char magic_number[RRDENG_MAGIC_SZ];
    char version[RRDENG_VER_SZ];
    uint8_t tier;
    uint8_t padding[RRDENG_DF_SB_PADDING_SZ];
} __attribute__ ((packed));

/*
 * Page types
 */

#define RRDENG_PAGE_TYPE_ARRAY_32BIT    (0)
#define RRDENG_PAGE_TYPE_ARRAY_TIER1    (1)
#define RRDENG_PAGE_TYPE_GORILLA_32BIT  (2)
#define RRDENG_PAGE_TYPE_MAX            (2) // Maximum page type (inclusive)

/*
 * Data file page descriptor
 */
struct rrdeng_extent_page_descr {
    uint8_t type;

    uint8_t uuid[UUID_SZ];
    uint32_t page_length;
    uint64_t start_time_ut;
    union {
        struct {
            uint32_t entries;
            uint32_t delta_time_s;
        } gorilla  __attribute__((packed));

        uint64_t end_time_ut;
    };
} __attribute__ ((packed));

/*
 * Data file extent header
 */
struct rrdeng_df_extent_header {
    uint32_t payload_length;
    uint8_t compression_algorithm;
    uint8_t number_of_pages;
    /* #number_of_pages page descriptors follow */
    struct rrdeng_extent_page_descr descr[];
} __attribute__ ((packed));

/*
 * Data file extent trailer
 */
struct rrdeng_df_extent_trailer {
    uint8_t checksum[CHECKSUM_SZ]; /* CRC32 */
} __attribute__ ((packed));

#define RRDENG_JF_SB_PADDING_SZ (RRDENG_BLOCK_SIZE - (RRDENG_MAGIC_SZ + RRDENG_VER_SZ))
/*
 * Journal file super-block
 */
struct rrdeng_jf_sb {
    char magic_number[RRDENG_MAGIC_SZ];
    char version[RRDENG_VER_SZ];
    uint8_t padding[RRDENG_JF_SB_PADDING_SZ];
} __attribute__ ((packed));

/*
 * Transaction record types
 */
#define STORE_PADDING       (0)
#define STORE_DATA          (1)
#define STORE_LOGS          (2) /* reserved */

/*
 * Journal file transaction record header
 */
struct rrdeng_jf_transaction_header {
    /* when set to STORE_PADDING jump to start of next block */
    uint8_t type;

    uint32_t reserved; /* reserved for future use */
    uint64_t id;
    uint16_t payload_length;
} __attribute__ ((packed));

/*
 * Journal file transaction record trailer
 */
struct rrdeng_jf_transaction_trailer {
    uint8_t checksum[CHECKSUM_SZ]; /* CRC32 */
} __attribute__ ((packed));

/*
 * Journal file STORE_DATA action
 */
struct rrdeng_jf_store_data {
    /* data file extent information */
    uint64_t extent_offset;
    uint32_t extent_size;

    uint8_t number_of_pages;
    /* #number_of_pages page descriptors follow */
    struct rrdeng_extent_page_descr descr[];
} __attribute__ ((packed));

// Journal v2 structures

#define JOURVAL_V2_MAGIC           (0x01231317)
#define JOURVAL_V2_REBUILD_MAGIC   (0x00231317)
#define JOURVAL_V2_SKIP_MAGIC      (0x02231317)

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
    uint32_t flags;

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

// Journal v2 supports linear-probing metrics by uuid
#define JOURNAL_V2_FLAG_HASH_TABLE_INDEX (1 << 0)

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

#endif /* NETDATA_RRDDISKPROTOCOL_H */
