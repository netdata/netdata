// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_METADATALOG_H
#define NETDATA_METADATALOG_H

#ifndef _GNU_SOURCE
#define _GNU_SOURCE
#endif
#include "../rrdengine.h"
#include "metadatalogprotocol.h"
#include "logfile.h"
#include "metadatalogapi.h"
#include "compaction.h"

/* Forward declerations */
struct metalog_instance;
struct parser_user_object;

#define METALOG_FILE_NUMBER_SCAN_TMPL "%5u-%5u"
#define METALOG_FILE_NUMBER_PRINT_TMPL "%5.5u-%5.5u"

/*
 * Debug statistics not used by code logic.
 * They only describe operations since DB engine instance load time.
 */
struct metalog_statistics {
    rrdeng_stats_t io_write_bytes;
    rrdeng_stats_t io_write_requests;
    rrdeng_stats_t io_read_bytes;
    rrdeng_stats_t io_read_requests;
    rrdeng_stats_t io_write_record_bytes;
    rrdeng_stats_t io_write_records;
    rrdeng_stats_t io_read_record_bytes;
    rrdeng_stats_t io_read_records;
    rrdeng_stats_t metadata_logfile_creations;
    rrdeng_stats_t metadata_logfile_deletions;
    rrdeng_stats_t io_errors;
    rrdeng_stats_t fs_errors;
};

struct metalog_instance {
    struct rrdengine_instance *rrdeng_ctx;
    struct parser_user_object *metalog_parser_object;
    uint8_t initialized; /* set to 1 to mark context initialized */
};

#endif /* NETDATA_METADATALOG_H */
