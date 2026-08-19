// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_SYSTEMD_JOURNAL_FUNCTION_H
#define NETDATA_SYSTEMD_JOURNAL_FUNCTION_H

#define ND_SD_JOURNAL_SAMPLING_SLOTS 1000
#define ND_SD_JOURNAL_SAMPLING_RECALIBRATE 10000

#define FACET_MAX_VALUE_LENGTH 8192
#define ND_SD_JOURNAL_DEFAULT_TIMEOUT 60
#define ND_SD_JOURNAL_PROGRESS_EVERY_UT (250 * USEC_PER_MS)

#define JOURNAL_DEFAULT_DIRECTION FACETS_ANCHOR_DIRECTION_BACKWARD

#define JD_SOURCE_REALTIME_TIMESTAMP "_SOURCE_REALTIME_TIMESTAMP"

// structures needed by LQS
struct lqs_extension {
    struct {
        usec_t start_ut;
        usec_t stop_ut;
        usec_t first_msg_ut;

        NsdId128 first_msg_writer;
        uint64_t first_msg_seqnum;
    } query_file;

    struct {
        uint32_t enable_after_samples;
        uint32_t slots;
        uint32_t sampled;
        uint32_t unsampled;
        size_t estimated;
    } samples;

    struct {
        uint32_t enable_after_samples;
        uint32_t every;
        uint32_t skipped;
        uint32_t recalibrate;
        uint32_t sampled;
        uint32_t unsampled;
        size_t estimated;
    } samples_per_file;

    struct {
        usec_t start_ut;
        usec_t end_ut;
        usec_t step_ut;
        uint32_t enable_after_samples;
        uint32_t sampled[ND_SD_JOURNAL_SAMPLING_SLOTS];
        uint32_t unsampled[ND_SD_JOURNAL_SAMPLING_SLOTS];
    } samples_per_time_slot;

    // per file progress info
    // size_t cached_count;

    // progress statistics
    usec_t matches_setup_ut;
    size_t rows_useful;
    size_t rows_read;
    size_t bytes_read;
    size_t files_matched;
    size_t file_working;
};

#endif //NETDATA_SYSTEMD_JOURNAL_FUNCTION_H
