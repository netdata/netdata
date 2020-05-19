// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_METADATALOGPROTOCOL_H
#define NETDATA_METADATALOGPROTOCOL_H

#include "../rrddiskprotocol.h"

#define RRDENG_METALOG_MAGIC "netdata-metadata-log"

#define RRDENG_METALOG_VER "1"

#define RRDENG_METALOG_SB_PADDING_SZ (RRDENG_BLOCK_SIZE - (RRDENG_MAGIC_SZ + RRDENG_VER_SZ + sizeof(uint8_t)))
/*
 * Metadata log persistent super-block
 */
struct rrdeng_metalog_sb {
    char magic_number[RRDENG_MAGIC_SZ];
    char version[RRDENG_VER_SZ];
    uint8_t tier;
    uint8_t padding[RRDENG_METALOG_SB_PADDING_SZ];
} __attribute__ ((packed));

/*
 * Metadata log record types
 */
#define METALOG_STORE_PADDING       (0)
#define METALOG_CREATE_OBJECT       (1)
#define METALOG_DELETE_OBJECT       (2)
#define METALOG_OTHER               (3) /* reserved */

/*
 * Metadata log record header
 */
struct rrdeng_metalog_record_header {
    /* when set to STORE_PADDING jump to start of next block */
    uint8_t type;

    uint32_t header_length;
    uint64_t id;
    uint16_t payload_length;
} __attribute__ ((packed));

/*
 * Metadata log record trailer
 */
struct rrdeng_metalog_record_trailer {
    uint8_t checksum[CHECKSUM_SZ]; /* CRC32 */
} __attribute__ ((packed));

#endif /* NETDATA_METADATALOGPROTOCOL_H */
