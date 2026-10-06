// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef DBENGINE_RETENTION_SUMMARY_H
#define DBENGINE_RETENTION_SUMMARY_H

#include "rrdengine.h"

// Immutable per-tier retention summaries of fixed blocks of journal v2 files.
//
// Block N covers datafiles [N * 24, N * 24 + 23]. Once all 24 have a sealed
// journal v2, a background builder merges their metric lists into one file:
// per metric {first, last, update_every}, sorted by UUID, plus the identity of
// every journal it was built from. At startup a summary whose 24 identities
// match the loaded journals replaces their 24 metric-list walks; anything else
// (missing, damaged, stale, unknown version) falls back to the per-file walk.

#define RETENTION_SUMMARY_FILES_PER_BLOCK 24
#define RETENTION_SUMMARY_PREFIX "retsum-"
#define RETENTION_SUMMARY_EXTENSION ".nrs"
#define RETENTION_SUMMARY_TMP_EXTENSION ".nrs.tmp"
#define RETENTION_SUMMARY_MAGIC 0x3153524e // "NRS1"
#define RETENTION_SUMMARY_VERSION 1
#define RETENTION_SUMMARY_CHUNK_RECORDS 65536
#define RETENTION_SUMMARY_HEADER_SIZE 4096

// what a summary records about each journal v2 it was built from
struct retention_summary_file_identity {
    uint32_t fileno;
    uint32_t metric_count;
    uint64_t v2_size;
    uint64_t start_time_ut;         // the journal v2 header start time
    uint64_t end_time_ut;
    uint64_t samples;               // the samples of the file (its descriptor total, or the estimate)
    uint32_t header_crc;            // the file trailer: the CRC of the journal v2 header
    uint32_t metric_list_crc;       // the metric-list trailer
    uint32_t samples_estimated;
    uint32_t reserved;
};

struct retention_summary_header {
    uint32_t magic;
    uint32_t version;
    uint32_t files_per_block;
    uint32_t tier;
    uint32_t block_id;
    uint32_t chunk_records;
    uint64_t record_count;
    int64_t base_time_s;            // record times are deltas from this
    int64_t max_last_time_s;        // the newest last time of any record
    struct retention_summary_file_identity files[RETENTION_SUMMARY_FILES_PER_BLOCK];
    uint32_t reserved;
    uint32_t crc;                   // CRC32 of the fields above
};

// The per-file walk applies each file's entry with mrg_metric_expand_retention():
// first = min, last = max, and update_every changes only when an entry raises the
// last time AND has a non-zero update_every (single-point pages have 0). So the
// update_every a metric ends with depends on the last time it already had: it is
// the one of the newest entry that raised the last time with a non-zero
// update_every. A record keeps that entry's last time too, and the loader applies
// it first, then extends the last time - exactly what the walk would do.
struct retention_summary_record {
    nd_uuid_t uuid;
    uint32_t first_delta_s;
    uint32_t last_delta_s;
    uint32_t update_every_s;            // 0 when no entry raising the last time had one
    uint32_t update_every_last_delta_s; // the last time of the entry that set update_every_s
};

// File layout:
//   header (padded to RETENTION_SUMMARY_HEADER_SIZE)
//   records[record_count], sorted by UUID
//   chunk_crc[ceil(record_count / chunk_records)], the CRC32 of each chunk of records
//   trailer: CRC32 of chunk_crc[]

typedef struct retention_summary_block RETENTION_SUMMARY_BLOCK;

// Startup, before the per-file MRG population: deletes temporary and unusable
// summaries, and claims (marks as populated and acquires) the datafiles of every
// summary that matches the loaded journals. Returns the number of claimed blocks in *blocks.
size_t retention_summary_claim(struct rrdengine_instance *ctx, RETENTION_SUMMARY_BLOCK ***blocks);

// Loads one claimed block into the MRG (falls back to the per-file walk of its
// datafiles if the records are damaged), releases its datafiles and frees it.
size_t retention_summary_apply(RETENTION_SUMMARY_BLOCK *block);

// Builds the summary of one complete block that has none, newest first.
// Returns true when it tried a block, false when there was none to build.
bool retention_summary_build_one(struct rrdengine_instance *ctx);

// A datafile was deleted: its block summary no longer describes the block.
void retention_summary_datafile_deleted(struct rrdengine_instance *ctx, unsigned fileno);

// Releases the in-memory summary state of a ctx.
void retention_summary_ctx_cleanup(struct rrdengine_instance *ctx);

int retention_summary_unittest(void);

#endif // DBENGINE_RETENTION_SUMMARY_H
