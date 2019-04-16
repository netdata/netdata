// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_RRDDISKPROTOCOL_H
#define NETDATA_RRDDISKPROTOCOL_H

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

#define RRD_NO_COMPRESSION (0)
#define RRD_LZ4 (1)

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
#define PAGE_METRICS    (0)
#define PAGE_LOGS       (1) /* reserved */

/*
 * Data file page descriptor
 */
struct rrdeng_extent_page_descr {
    uint8_t type;

    uint8_t uuid[UUID_SZ];
    uint32_t page_length;
    uint64_t start_time;
    uint64_t end_time;
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

#endif /* NETDATA_RRDDISKPROTOCOL_H */