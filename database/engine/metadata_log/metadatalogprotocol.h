// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_METADATALOGPROTOCOL_H
#define NETDATA_METADATALOGPROTOCOL_H

#include "../rrddiskprotocol.h"

#define RRDENG_METALOG_MAGIC "netdata-metadata-log"

#define RRDENG_METALOG_VER (1)

#define RRDENG_METALOG_SB_PADDING_SZ (RRDENG_BLOCK_SIZE - (RRDENG_MAGIC_SZ + sizeof(uint16_t)))
/*
 * Metadata log persistent super-block
 */
struct rrdeng_metalog_sb {
    char magic_number[RRDENG_MAGIC_SZ];
    uint16_t version;
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
    /* when set to METALOG_STORE_PADDING jump to start of next block */
    uint8_t type;

    uint16_t header_length;
    uint32_t payload_length;
    /******************************************************
     * No fields above this point can ever change.        *
     ******************************************************
     * All fields below this point are subject to change. *
     ******************************************************/
} __attribute__ ((packed));

/*
 * Metadata log record trailer
 */
struct rrdeng_metalog_record_trailer {
    uint8_t checksum[CHECKSUM_SZ]; /* CRC32 */
} __attribute__ ((packed));

#endif /* NETDATA_METADATALOGPROTOCOL_H */
