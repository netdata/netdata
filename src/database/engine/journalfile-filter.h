// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_JOURNALFILE_FILTER_H
#define NETDATA_JOURNALFILE_FILTER_H

// Journal time filter (.njtf): a companion file of a tier-0 journal v2 file.
//
// It is a blocked Bloom filter of the keys (metric uuid, hour) for every hour every page of the journal touches.
// Query planning checks the hours of its window before searching the journal: if every hour tests negative, the
// journal holds no page of that metric in the window and is skipped. A Bloom filter has no false negatives, so a
// skipped journal never holds a page today's planner would have accepted; a false positive only costs the search
// done without the filter.
//
// The file is bound to its journal by identity (fileno, journal v2 size, journal header CRC) and its payload is
// CRC-checked once per process before any negative is trusted. Missing, stale, unverified or broken filters, and
// windows touching more than NJTF_MAX_WINDOW_BUCKETS hours, fall back to searching the journal.
//
// Layout: a NJTF_HEADER_SIZE header (struct njtf_header, zero padded), then blocks * NJTF_BLOCK_BYTES of payload.
// Bits are addressed by byte (byte = pos / 8, bit = pos % 8), so the file does not depend on the CPU's byte order.

#define NJTF_EXTENSION           ".njtf"
#define NJTF_TMP_EXTENSION       ".njtf.tmp"
#define NJTF_MAGIC               0x46544a4eU     // "NJTF"
#define NJTF_VERSION             1
#define NJTF_HEADER_SIZE         4096
#define NJTF_BLOCK_BYTES         64
#define NJTF_BLOCK_BITS          (NJTF_BLOCK_BYTES * 8)
#define NJTF_BUCKET_S            3600            // one key per hour a page touches
#define NJTF_BITS_PER_KEY        10              // ~1% false positives per key
#define NJTF_K                   8               // bits set per key, all in one block
#define NJTF_MAX_WINDOW_BUCKETS  25              // windows touching more hours bypass the filter
#define NJTF_MAX_PAGE_BUCKETS    (7 * 24)        // a page spanning more hours makes the journal unfilterable

typedef enum __attribute__((packed)) {
    NJTF_STATE_UNKNOWN = 0,     // not looked at in this process: the builder verifies or builds it
    NJTF_STATE_BUSY,            // the builder is working on it
    NJTF_STATE_VALID,           // verified and mapped: its negatives can be trusted
    NJTF_STATE_MISSING,         // no usable file: the builder builds one
    NJTF_STATE_FAILED,          // cannot be built for this journal in this process
} NJTF_STATE;

typedef enum {
    NJTF_CHECK_SEARCH,          // the filter says the metric may be there: search the journal
    NJTF_CHECK_SKIP,            // the filter rules the journal out
    NJTF_CHECK_BYPASS,          // no usable filter, or the window is too wide: search the journal
} NJTF_CHECK;

struct njtf_header {
    uint32_t magic;
    uint32_t version;
    uint32_t tier;
    uint32_t fileno;
    uint32_t journal_v2_file_size;      // identity: the journal it was built from
    uint32_t journal_v2_header_crc;     // identity: that journal's header CRC (its file trailer)
    uint32_t bucket_s;
    uint32_t bits_per_key;
    uint32_t k;
    uint32_t blocks;                    // payload = blocks * NJTF_BLOCK_BYTES bytes, at NJTF_HEADER_SIZE
    uint64_t seed;
    uint64_t keys;
    uint32_t payload_crc;
    uint32_t header_crc;                // CRC32 of the fields above
};

extern bool dbengine_journal_time_filters;

struct rrdengine_datafile;
struct rrdengine_journalfile;
struct rrdengine_instance;
struct journal_v2_header;

void njtf_generate_path(struct rrdengine_datafile *datafile, char *str, size_t maxlen, bool tmp);

// build the filter of a journal v2 file mapped at jv2 and write it next to the journal; false if it cannot be built
bool njtf_build(struct rrdengine_datafile *datafile, const uint8_t *jv2, size_t jv2_size);

// map and verify the filter of a journal (the caller holds its journal v2 data); true when its negatives can be trusted
bool njtf_verify_and_map(struct rrdengine_journalfile *journalfile, const uint8_t *jv2, size_t jv2_size);

// query planning: may this journal hold pages of uuid within [after_s, before_s]?
// The caller holds the journal v2 data acquired (j2_header, jv2_size).
NJTF_CHECK njtf_check(struct rrdengine_journalfile *journalfile, const struct journal_v2_header *j2_header,
                      size_t jv2_size, const nd_uuid_t *uuid, time_t after_s, time_t before_s);

// unmap the filter of a journal that is being closed or deleted (no user of its journal v2 data remains)
void njtf_release(struct rrdengine_journalfile *journalfile);

// delete the filter files of a datafile that is being deleted
void njtf_unlink(struct rrdengine_datafile *datafile);

// background builder: give one journal of ctx a filter; returns false when there is nothing left to do
bool njtf_backfill_one(struct rrdengine_instance *ctx);

int njtf_unittest(void);
void njtf_test_knob_refresh(void);
int njtf_verify_directory(void);

#endif //NETDATA_JOURNALFILE_FILTER_H
