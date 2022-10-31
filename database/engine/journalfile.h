// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_JOURNALFILE_H
#define NETDATA_JOURNALFILE_H

#include "rrdengine.h"

/* Forward declarations */
struct rrdengine_instance;
struct rrdengine_worker_config;
struct rrdengine_datafile;
struct rrdengine_journalfile;

#define WALFILE_PREFIX "journalfile-"
#define WALFILE_EXTENSION ".njf"
#define WALFILE_EXTENSION_V2 ".njfv2"

#define is_descr_journal_v2(descr) ((descr)->extent == NULL)

/* only one event loop is supported for now */
struct rrdengine_journalfile {
    uv_file file;
    uint64_t pos;
    void *data;
    uint16_t file_index;                        // File index
    void *journal_data;                      // MMAPed file of journal v2
    uint32_t journal_data_size;                 // Total file size mapped
    struct rrdengine_datafile *datafile;
};


// Journal v2 structures

#define JOURVAL_V2_MAGIC   (0x01221019)

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
        uint8_t checksum[4];      // CRC check
        uint32_t crc;
    };
    uint32_t uuid_offset;    // Points back to the UUID list which should point here (UUIDs should much)
    uint32_t entries;        // Entries
    uuid_t   uuid;           // Which UUID this is
};

// 20 bytes
struct journal_page_list {
    uint32_t delta_start_s;    // relative to the start time of journal
    uint32_t delta_end_s;      // relative to delta_start
    uint32_t extent_index;   // Index to the extent (extent list) (bytes from BASE)
    uint16_t update_every_s;
    uint16_t page_length;
    uint8_t type;
};

// UUID_LIST
// 32 bytes
struct journal_metric_list {
    uuid_t uuid;
    uint32_t entries;         // Number of entries
    uint32_t page_offset;     // OFFSET that contains entries * struct( journal_page_list )
    uint32_t delta_start;     // Min time of metric
    uint32_t delta_end;       // Max time of metric  (to be used to populate page_index)
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
    uint32_t total_file_size;           // This is the total file size
    void *data;                         // Used when building the index
};

#define JOURNAL_V2_HEADER_PADDING_SZ (RRDENG_BLOCK_SIZE - (sizeof(struct journal_v2_header)))



/* only one event loop is supported for now */
struct transaction_commit_log {
    uint64_t transaction_id;

    /* outstanding transaction buffer */
    void *buf;
    unsigned buf_pos;
    unsigned buf_size;
};

void generate_journalfilepath(struct rrdengine_datafile *datafile, char *str, size_t maxlen);
void journalfile_init(struct rrdengine_journalfile *journalfile, struct rrdengine_datafile *datafile);
void *wal_get_transaction_buffer(struct rrdengine_worker_config* wc, unsigned size);
void wal_flush_transaction_buffer(struct rrdengine_worker_config* wc);
int close_journal_file(struct rrdengine_journalfile *journalfile, struct rrdengine_datafile *datafile);
int unlink_journal_file(struct rrdengine_journalfile *journalfile);
int destroy_journal_file(struct rrdengine_journalfile *journalfile, struct rrdengine_datafile *datafile);
int create_journal_file(struct rrdengine_journalfile *journalfile, struct rrdengine_datafile *datafile);
int load_journal_file(struct rrdengine_instance *ctx, struct rrdengine_journalfile *journalfile,
                             struct rrdengine_datafile *datafile);
void init_commit_log(struct rrdengine_instance *ctx);


#endif /* NETDATA_JOURNALFILE_H */