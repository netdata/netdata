// SPDX-License-Identifier: GPL-3.0-or-later
/*
 *  netdata systemd-journal.plugin
 *  Copyright (C) 2023 Netdata Inc.
 *  GPL v3+
 */

#include "systemd-internals.h"

/*
 * TODO
 *
 * _UDEV_DEVLINK is frequently set more than once per field - support multi-value faces
 *
 */

#define FACET_MAX_VALUE_LENGTH                  8192

#define SYSTEMD_JOURNAL_FUNCTION_DESCRIPTION    "View, search and analyze systemd journal entries."
#define SYSTEMD_JOURNAL_FUNCTION_NAME           "systemd-journal"
#define SYSTEMD_JOURNAL_DEFAULT_TIMEOUT         60
#define SYSTEMD_JOURNAL_MAX_PARAMS              1000
#define SYSTEMD_JOURNAL_DEFAULT_QUERY_DURATION  (1 * 3600)
#define SYSTEMD_JOURNAL_DEFAULT_ITEMS_PER_QUERY 200
#define SYSTEMD_JOURNAL_DEFAULT_ITEMS_SAMPLING  1000000
#define SYSTEMD_JOURNAL_SAMPLING_SLOTS          1000
#define SYSTEMD_JOURNAL_SAMPLING_RECALIBRATE    10000

#define SYSTEMD_JOURNAL_PROGRESS_EVERY_UT       (250 * USEC_PER_MS)

#define JOURNAL_PARAMETER_HELP                  "help"
#define JOURNAL_PARAMETER_AFTER                 "after"
#define JOURNAL_PARAMETER_BEFORE                "before"
#define JOURNAL_PARAMETER_ANCHOR                "anchor"
#define JOURNAL_PARAMETER_LAST                  "last"
#define JOURNAL_PARAMETER_QUERY                 "query"
#define JOURNAL_PARAMETER_FACETS                "facets"
#define JOURNAL_PARAMETER_HISTOGRAM             "histogram"
#define JOURNAL_PARAMETER_DIRECTION             "direction"
#define JOURNAL_PARAMETER_IF_MODIFIED_SINCE     "if_modified_since"
#define JOURNAL_PARAMETER_DATA_ONLY             "data_only"
#define JOURNAL_PARAMETER_SOURCE                "source"
#define JOURNAL_PARAMETER_INFO                  "info"
#define JOURNAL_PARAMETER_SLICE                 "slice"
#define JOURNAL_PARAMETER_DELTA                 "delta"
#define JOURNAL_PARAMETER_TAIL                  "tail"
#define JOURNAL_PARAMETER_SAMPLING              "sampling"

#define JOURNAL_KEY_ND_JOURNAL_FILE             "ND_JOURNAL_FILE"
#define JOURNAL_KEY_ND_JOURNAL_PROCESS          "ND_JOURNAL_PROCESS"

#define JOURNAL_DEFAULT_SLICE_MODE              true
#define JOURNAL_DEFAULT_DIRECTION               FACETS_ANCHOR_DIRECTION_BACKWARD

#define SYSTEMD_ALWAYS_VISIBLE_KEYS             NULL

#define SYSTEMD_KEYS_EXCLUDED_FROM_FACETS       \
    "!MESSAGE_ID"                               \
    "|*MESSAGE*"                                \
    "|*_RAW"                                    \
    "|*_USEC"                                   \
    "|*_NSEC"                                   \
    "|*TIMESTAMP*"                              \
    "|*_ID"                                     \
    "|*_ID_*"                                   \
    "|__*"                                      \
    ""

#define SYSTEMD_KEYS_INCLUDED_IN_FACETS         \
                                                \
    /* --- USER JOURNAL FIELDS --- */           \
                                                \
    /* "|MESSAGE" */                            \
    "|MESSAGE_ID"                               \
    "|PRIORITY"                                 \
    "|CODE_FILE"                                \
    /* "|CODE_LINE" */                          \
    "|CODE_FUNC"                                \
    "|ERRNO"                                    \
    /* "|INVOCATION_ID" */                      \
    /* "|USER_INVOCATION_ID" */                 \
    "|SYSLOG_FACILITY"                          \
    "|SYSLOG_IDENTIFIER"                        \
    /* "|SYSLOG_PID" */                         \
    /* "|SYSLOG_TIMESTAMP" */                   \
    /* "|SYSLOG_RAW" */                         \
    /* "!DOCUMENTATION" */                      \
    /* "|TID" */                                \
    "|UNIT"                                     \
    "|USER_UNIT"                                \
    "|UNIT_RESULT" /* undocumented */           \
                                                \
                                                \
    /* --- TRUSTED JOURNAL FIELDS --- */        \
                                                \
    /* "|_PID" */                               \
    "|_UID"                                     \
    "|_GID"                                     \
    "|_COMM"                                    \
    "|_EXE"                                     \
    /* "|_CMDLINE" */                           \
    "|_CAP_EFFECTIVE"                           \
    /* "|_AUDIT_SESSION" */                     \
    "|_AUDIT_LOGINUID"                          \
    "|_SYSTEMD_CGROUP"                          \
    "|_SYSTEMD_SLICE"                           \
    "|_SYSTEMD_UNIT"                            \
    "|_SYSTEMD_USER_UNIT"                       \
    "|_SYSTEMD_USER_SLICE"                      \
    "|_SYSTEMD_SESSION"                         \
    "|_SYSTEMD_OWNER_UID"                       \
    "|_SELINUX_CONTEXT"                         \
    /* "|_SOURCE_REALTIME_TIMESTAMP" */         \
    "|_BOOT_ID"                                 \
    "|_MACHINE_ID"                              \
    /* "|_SYSTEMD_INVOCATION_ID" */             \
    "|_HOSTNAME"                                \
    "|_TRANSPORT"                               \
    "|_STREAM_ID"                               \
    /* "|LINE_BREAK" */                         \
    "|_NAMESPACE"                               \
    "|_RUNTIME_SCOPE"                           \
                                                \
                                                \
    /* --- KERNEL JOURNAL FIELDS --- */         \
                                                \
    /* "|_KERNEL_DEVICE" */                     \
    "|_KERNEL_SUBSYSTEM"                        \
    /* "|_UDEV_SYSNAME" */                      \
    "|_UDEV_DEVNODE"                            \
    /* "|_UDEV_DEVLINK" */                      \
                                                \
                                                \
    /* --- LOGGING ON BEHALF --- */             \
                                                \
    "|OBJECT_UID"                               \
    "|OBJECT_GID"                               \
    "|OBJECT_COMM"                              \
    "|OBJECT_EXE"                               \
    /* "|OBJECT_CMDLINE" */                     \
    /* "|OBJECT_AUDIT_SESSION" */               \
    "|OBJECT_AUDIT_LOGINUID"                    \
    "|OBJECT_SYSTEMD_CGROUP"                    \
    "|OBJECT_SYSTEMD_SESSION"                   \
    "|OBJECT_SYSTEMD_OWNER_UID"                 \
    "|OBJECT_SYSTEMD_UNIT"                      \
    "|OBJECT_SYSTEMD_USER_UNIT"                 \
                                                \
                                                \
    /* --- CORE DUMPS --- */                    \
                                                \
    "|COREDUMP_COMM"                            \
    "|COREDUMP_UNIT"                            \
    "|COREDUMP_USER_UNIT"                       \
    "|COREDUMP_SIGNAL_NAME"                     \
    "|COREDUMP_CGROUP"                          \
                                                \
                                                \
    /* --- DOCKER --- */                        \
                                                \
    "|CONTAINER_ID"                             \
    /* "|CONTAINER_ID_FULL" */                  \
    "|CONTAINER_NAME"                           \
    "|CONTAINER_TAG"                            \
    "|IMAGE_NAME" /* undocumented */            \
    /* "|CONTAINER_PARTIAL_MESSAGE" */          \
                                                \
                                                \
    /* --- NETDATA --- */                       \
                                                \
    "|ND_NIDL_NODE"                             \
    "|ND_NIDL_CONTEXT"                          \
    "|ND_LOG_SOURCE"                            \
    /*"|ND_MODULE" */                           \
    "|ND_ALERT_NAME"                            \
    "|ND_ALERT_CLASS"                           \
    "|ND_ALERT_COMPONENT"                       \
    "|ND_ALERT_TYPE"                            \
                                                \
    ""

// ----------------------------------------------------------------------------

typedef struct function_query_status {
    bool *cancelled; // a pointer to the cancelling boolean
    usec_t *stop_monotonic_ut;

    // request
    const char *transaction;

    SD_JOURNAL_FILE_SOURCE_TYPE source_type;
    SIMPLE_PATTERN *sources;
    usec_t after_ut;
    usec_t before_ut;

    struct {
        usec_t start_ut;
        usec_t stop_ut;
    } anchor;

    FACETS_ANCHOR_DIRECTION direction;
    size_t entries;
    usec_t if_modified_since;
    bool delta;
    bool tail;
    bool data_only;
    bool slice;
    size_t sampling;
    size_t filters;
    usec_t last_modified;
    const char *query;
    const char *histogram;

    struct {
        usec_t start_ut;     // the starting time of the query - we start from this
        usec_t stop_ut;      // the ending time of the query - we stop at this
        usec_t first_msg_ut;

        sd_id128_t first_msg_writer;
        uint64_t first_msg_seqnum;
    } query_file;

    struct {
        uint32_t enable_after_samples;
        uint32_t slots;
        uint32_t sampled;
        uint32_t unsampled;
        uint32_t estimated;
    } samples;

    struct {
        uint32_t enable_after_samples;
        uint32_t every;
        uint32_t skipped;
        uint32_t recalibrate;
        uint32_t sampled;
        uint32_t unsampled;
        uint32_t estimated;
    } samples_per_file;

    struct {
        usec_t start_ut;
        usec_t end_ut;
        usec_t step_ut;
        uint32_t enable_after_samples;
        uint32_t sampled[SYSTEMD_JOURNAL_SAMPLING_SLOTS];
        uint32_t unsampled[SYSTEMD_JOURNAL_SAMPLING_SLOTS];
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
} FUNCTION_QUERY_STATUS;

static void log_fqs(FUNCTION_QUERY_STATUS *fqs, const char *msg) {
    netdata_log_error("ERROR: %s, on query "
                      "timeframe [%"PRIu64" - %"PRIu64"], "
                      "anchor [%"PRIu64" - %"PRIu64"], "
                      "if_modified_since %"PRIu64", "
                      "data_only:%s, delta:%s, tail:%s, direction:%s"
                      , msg
                      , fqs->after_ut, fqs->before_ut
                      , fqs->anchor.start_ut, fqs->anchor.stop_ut
                      , fqs->if_modified_since
                      , fqs->data_only ? "true" : "false"
                      , fqs->delta ? "true" : "false"
                      , fqs->tail ? "tail" : "false"
                      , fqs->direction == FACETS_ANCHOR_DIRECTION_FORWARD ? "forward" : "backward");
}

static inline bool netdata_systemd_journal_seek_to(sd_journal *j, usec_t timestamp) {
    if(sd_journal_seek_realtime_usec(j, timestamp) < 0) {
        netdata_log_error("SYSTEMD-JOURNAL: Failed to seek to %" PRIu64, timestamp);
        if(sd_journal_seek_tail(j) < 0) {
            netdata_log_error("SYSTEMD-JOURNAL: Failed to seek to journal's tail");
            return false;
        }
    }

    return true;
}

#define JD_SOURCE_REALTIME_TIMESTAMP "_SOURCE_REALTIME_TIMESTAMP"

// ----------------------------------------------------------------------------
// sampling support

static void sampling_query_init(FUNCTION_QUERY_STATUS *fqs, FACETS *facets) {
    if(!fqs->sampling)
        return;

    if(!fqs->slice) {
        // the user is doing a full data query
        // disable sampling
        fqs->sampling = 0;
        return;
    }

    if(fqs->data_only) {
        // the user is doing a data query
        // disable sampling
        fqs->sampling = 0;
        return;
    }

    if(!fqs->files_matched) {
        // no files have been matched
        // disable sampling
        fqs->sampling = 0;
        return;
    }

    fqs->samples.slots = facets_histogram_slots(facets);
    if(fqs->samples.slots < 2) fqs->samples.slots = 2;
    if(fqs->samples.slots > SYSTEMD_JOURNAL_SAMPLING_SLOTS)
        fqs->samples.slots = SYSTEMD_JOURNAL_SAMPLING_SLOTS;

    if(!fqs->after_ut || !fqs->before_ut || fqs->after_ut >= fqs->before_ut) {
        // we don't have enough information for sampling
        fqs->sampling = 0;
        return;
    }

    usec_t delta = fqs->before_ut - fqs->after_ut;
    usec_t step = delta / facets_histogram_slots(facets) - 1;
    if(step < 1) step = 1;

    fqs->samples_per_time_slot.start_ut = fqs->after_ut;
    fqs->samples_per_time_slot.end_ut = fqs->before_ut;
    fqs->samples_per_time_slot.step_ut = step;

    // the minimum number of rows to enable sampling
    fqs->samples.enable_after_samples = fqs->sampling / 2;

    size_t files_matched = fqs->files_matched;
    if(!files_matched)
        files_matched = 1;

    // the minimum number of rows per file to enable sampling
    fqs->samples_per_file.enable_after_samples = (fqs->sampling / 4) / files_matched;
    if(fqs->samples_per_file.enable_after_samples < fqs->entries)
        fqs->samples_per_file.enable_after_samples = fqs->entries;

    // the minimum number of rows per time slot to enable sampling
    fqs->samples_per_time_slot.enable_after_samples = (fqs->sampling / 4) / fqs->samples.slots;
    if(fqs->samples_per_time_slot.enable_after_samples < fqs->entries)
        fqs->samples_per_time_slot.enable_after_samples = fqs->entries;
}

static void sampling_file_init(FUNCTION_QUERY_STATUS *fqs, struct journal_file *jf __maybe_unused) {
    fqs->samples_per_file.sampled = 0;
    fqs->samples_per_file.unsampled = 0;
    fqs->samples_per_file.estimated = 0;
    fqs->samples_per_file.every = 0;
    fqs->samples_per_file.skipped = 0;
    fqs->samples_per_file.recalibrate = 0;
}

static size_t sampling_file_lines_scanned_so_far(FUNCTION_QUERY_STATUS *fqs) {
    size_t sampled = fqs->samples_per_file.sampled + fqs->samples_per_file.unsampled;
    if(!sampled) sampled = 1;
    return sampled;
}

static void sampling_running_file_query_overlapping_timeframe_ut(
        FUNCTION_QUERY_STATUS *fqs, struct journal_file *jf, FACETS_ANCHOR_DIRECTION direction,
                usec_t msg_ut, usec_t *after_ut, usec_t *before_ut) {

    // find the overlap of the query and file timeframes
    // taking into account the first message we encountered

    usec_t oldest_ut, newest_ut;
    if(direction == FACETS_ANCHOR_DIRECTION_FORWARD) {
        // the first message we know (oldest)
        oldest_ut = fqs->query_file.first_msg_ut ? fqs->query_file.first_msg_ut : jf->msg_first_ut;
        if(!oldest_ut) oldest_ut = fqs->query_file.start_ut;

        if(jf->msg_last_ut)
            newest_ut = MIN(fqs->query_file.stop_ut, jf->msg_last_ut);
        else if(jf->file_last_modified_ut)
            newest_ut = MIN(fqs->query_file.stop_ut, jf->file_last_modified_ut);
        else
            newest_ut = fqs->query_file.stop_ut;

        if(msg_ut < oldest_ut)
            oldest_ut = msg_ut - 1;
    }
    else /* BACKWARD */ {
        // the latest message we know (newest)
        newest_ut = fqs->query_file.first_msg_ut ? fqs->query_file.first_msg_ut : jf->msg_last_ut;
        if(!newest_ut) newest_ut = fqs->query_file.start_ut;

        if(jf->msg_first_ut)
            oldest_ut = MAX(fqs->query_file.stop_ut, jf->msg_first_ut);
        else
            oldest_ut = fqs->query_file.stop_ut;

        if(newest_ut < msg_ut)
            newest_ut = msg_ut + 1;
    }

    *after_ut = oldest_ut;
    *before_ut = newest_ut;
}

static double sampling_running_file_query_progress_by_time(FUNCTION_QUERY_STATUS *fqs, struct journal_file *jf,
                                                           FACETS_ANCHOR_DIRECTION direction, usec_t msg_ut) {

    usec_t after_ut, before_ut, elapsed_ut;
    sampling_running_file_query_overlapping_timeframe_ut(fqs, jf, direction, msg_ut, &after_ut, &before_ut);

    if(direction == FACETS_ANCHOR_DIRECTION_FORWARD)
        elapsed_ut = msg_ut - after_ut;
    else
        elapsed_ut = before_ut - msg_ut;

    usec_t total_ut = before_ut - after_ut;
    double progress = (double)elapsed_ut / (double)total_ut;

    return progress;
}

static usec_t sampling_running_file_query_remaining_time(FUNCTION_QUERY_STATUS *fqs, struct journal_file *jf,
                                                         FACETS_ANCHOR_DIRECTION direction, usec_t msg_ut,
                                                         usec_t *total_time_ut, usec_t *remaining_start_ut,
                                                         usec_t *remaining_end_ut) {
    usec_t after_ut, before_ut;
    sampling_running_file_query_overlapping_timeframe_ut(fqs, jf, direction, msg_ut, &after_ut, &before_ut);

    // since we have a timestamp in msg_ut
    // this timestamp can extend the overlap
    if(msg_ut <= after_ut)
        after_ut = msg_ut - 1;

    if(msg_ut >= before_ut)
        before_ut = msg_ut + 1;

    // return the remaining duration
    usec_t remaining_from_ut, remaining_to_ut;
    if(direction == FACETS_ANCHOR_DIRECTION_FORWARD) {
        remaining_from_ut = msg_ut;
        remaining_to_ut = before_ut;
    }
    else {
        remaining_from_ut = after_ut;
        remaining_to_ut = msg_ut;
    }

    usec_t remaining_ut = remaining_to_ut - remaining_from_ut;

    if(total_time_ut)
        *total_time_ut = (before_ut > after_ut) ? before_ut - after_ut : 1;

    if(remaining_start_ut)
        *remaining_start_ut = remaining_from_ut;

    if(remaining_end_ut)
        *remaining_end_ut = remaining_to_ut;

    return remaining_ut;
}

static size_t sampling_running_file_query_estimate_remaining_lines_by_time(FUNCTION_QUERY_STATUS *fqs,
                                                                           struct journal_file *jf,
                                                                           FACETS_ANCHOR_DIRECTION direction,
                                                                           usec_t msg_ut) {
    size_t scanned_lines = sampling_file_lines_scanned_so_far(fqs);

    // Calculate the proportion of time covered
    usec_t total_time_ut, remaining_start_ut, remaining_end_ut;
    usec_t remaining_time_ut = sampling_running_file_query_remaining_time(fqs, jf, direction, msg_ut, &total_time_ut,
                                                                          &remaining_start_ut, &remaining_end_ut);
    if (total_time_ut == 0) total_time_ut = 1;

    double proportion_by_time = (double) (total_time_ut - remaining_time_ut) / (double) total_time_ut;

    if (proportion_by_time == 0 || proportion_by_time > 1.0 || !isfinite(proportion_by_time))
        proportion_by_time = 1.0;

    // Estimate the total number of lines in the file
    size_t expected_matching_logs_by_time = (size_t)((double)scanned_lines / proportion_by_time);

    if(jf->messages_in_file && expected_matching_logs_by_time > jf->messages_in_file)
        expected_matching_logs_by_time = jf->messages_in_file;

    // Calculate the estimated number of remaining lines
    size_t remaining_logs_by_time = expected_matching_logs_by_time - scanned_lines;
    if (remaining_logs_by_time < 1) remaining_logs_by_time = 1;

//    nd_log(NDLS_COLLECTORS, NDLP_INFO,
//           "JOURNAL ESTIMATION: '%s' "
//           "scanned_lines=%zu [sampled=%zu, unsampled=%zu, estimated=%zu], "
//           "file [%"PRIu64" - %"PRIu64", duration %"PRId64", known lines in file %zu], "
//           "query [%"PRIu64" - %"PRIu64", duration %"PRId64"], "
//           "first message read from the file at %"PRIu64", current message at %"PRIu64", "
//           "proportion of time %.2f %%, "
//           "expected total lines in file %zu, "
//           "remaining lines %zu, "
//           "remaining time %"PRIu64" [%"PRIu64" - %"PRIu64", duration %"PRId64"]"
//           , jf->filename
//           , scanned_lines, fqs->samples_per_file.sampled, fqs->samples_per_file.unsampled, fqs->samples_per_file.estimated
//           , jf->msg_first_ut, jf->msg_last_ut, jf->msg_last_ut - jf->msg_first_ut, jf->messages_in_file
//           , fqs->query_file.start_ut, fqs->query_file.stop_ut, fqs->query_file.stop_ut - fqs->query_file.start_ut
//           , fqs->query_file.first_msg_ut, msg_ut
//           , proportion_by_time * 100.0
//           , expected_matching_logs_by_time
//           , remaining_logs_by_time
//           , remaining_time_ut, remaining_start_ut, remaining_end_ut, remaining_end_ut - remaining_start_ut
//           );

    return remaining_logs_by_time;
}

static size_t sampling_running_file_query_estimate_remaining_lines(sd_journal *j __maybe_unused, FUNCTION_QUERY_STATUS *fqs, struct journal_file *jf, FACETS_ANCHOR_DIRECTION direction, usec_t msg_ut) {
    size_t remaining_logs_by_seqnum = 0;

#ifdef HAVE_SD_JOURNAL_GET_SEQNUM
    size_t expected_matching_logs_by_seqnum = 0;
    double proportion_by_seqnum = 0.0;
    uint64_t current_msg_seqnum;
    sd_id128_t current_msg_writer;
    if(!fqs->query_file.first_msg_seqnum || sd_journal_get_seqnum(j, &current_msg_seqnum, &current_msg_writer) < 0) {
        fqs->query_file.first_msg_seqnum = 0;
        fqs->query_file.first_msg_writer = SD_ID128_NULL;
    }
    else if(jf->messages_in_file) {
        size_t scanned_lines = sampling_file_lines_scanned_so_far(fqs);

        double proportion_of_all_lines_so_far;
        if(direction == FACETS_ANCHOR_DIRECTION_FORWARD)
            proportion_of_all_lines_so_far = (double)scanned_lines / (double)(current_msg_seqnum - jf->first_seqnum);
        else
            proportion_of_all_lines_so_far = (double)scanned_lines / (double)(jf->last_seqnum - current_msg_seqnum);

        if(proportion_of_all_lines_so_far > 1.0)
            proportion_of_all_lines_so_far = 1.0;

        expected_matching_logs_by_seqnum = (size_t)(proportion_of_all_lines_so_far * (double)jf->messages_in_file);

        proportion_by_seqnum = (double)scanned_lines / (double)expected_matching_logs_by_seqnum;

        if (proportion_by_seqnum == 0 || proportion_by_seqnum > 1.0 || !isfinite(proportion_by_seqnum))
            proportion_by_seqnum = 1.0;

        remaining_logs_by_seqnum = expected_matching_logs_by_seqnum - scanned_lines;
        if(!remaining_logs_by_seqnum) remaining_logs_by_seqnum = 1;
    }
#endif

    if(remaining_logs_by_seqnum)
        return remaining_logs_by_seqnum;

    return sampling_running_file_query_estimate_remaining_lines_by_time(fqs, jf, direction, msg_ut);
}

static void sampling_decide_file_sampling_every(sd_journal *j, FUNCTION_QUERY_STATUS *fqs, struct journal_file *jf, FACETS_ANCHOR_DIRECTION direction, usec_t msg_ut) {
    size_t files_matched = fqs->files_matched;
    if(!files_matched) files_matched = 1;

    size_t remaining_lines = sampling_running_file_query_estimate_remaining_lines(j, fqs, jf, direction, msg_ut);
    size_t wanted_samples = (fqs->sampling / 2) / files_matched;
    if(!wanted_samples) wanted_samples = 1;

    fqs->samples_per_file.every = remaining_lines / wanted_samples;

    if(fqs->samples_per_file.every < 1)
        fqs->samples_per_file.every = 1;
}

typedef enum {
    SAMPLING_STOP_AND_ESTIMATE = -1,
    SAMPLING_FULL = 0,
    SAMPLING_SKIP_FIELDS = 1,
} sampling_t;

static inline sampling_t is_row_in_sample(sd_journal *j, FUNCTION_QUERY_STATUS *fqs, struct journal_file *jf, usec_t msg_ut, FACETS_ANCHOR_DIRECTION direction, bool candidate_to_keep) {
    if(!fqs->sampling || candidate_to_keep)
        return SAMPLING_FULL;

    if(unlikely(msg_ut < fqs->samples_per_time_slot.start_ut))
        msg_ut = fqs->samples_per_time_slot.start_ut;
    if(unlikely(msg_ut > fqs->samples_per_time_slot.end_ut))
        msg_ut = fqs->samples_per_time_slot.end_ut;

    size_t slot = (msg_ut - fqs->samples_per_time_slot.start_ut) / fqs->samples_per_time_slot.step_ut;
    if(slot >= fqs->samples.slots)
        slot = fqs->samples.slots - 1;

    bool should_sample = false;

    if(fqs->samples.sampled < fqs->samples.enable_after_samples ||
        fqs->samples_per_file.sampled < fqs->samples_per_file.enable_after_samples ||
        fqs->samples_per_time_slot.sampled[slot] < fqs->samples_per_time_slot.enable_after_samples)
        should_sample = true;

    else if(fqs->samples_per_file.recalibrate >= SYSTEMD_JOURNAL_SAMPLING_RECALIBRATE || !fqs->samples_per_file.every) {
        // this is the first to be unsampled for this file
        sampling_decide_file_sampling_every(j, fqs, jf, direction, msg_ut);
        fqs->samples_per_file.recalibrate = 0;
        should_sample = true;
    }
    else {
        // we sample 1 every fqs->samples_per_file.every
        if(fqs->samples_per_file.skipped >= fqs->samples_per_file.every) {
            fqs->samples_per_file.skipped = 0;
            should_sample = true;
        }
        else
            fqs->samples_per_file.skipped++;
    }

    if(should_sample) {
        fqs->samples.sampled++;
        fqs->samples_per_file.sampled++;
        fqs->samples_per_time_slot.sampled[slot]++;

        return SAMPLING_FULL;
    }

    fqs->samples_per_file.recalibrate++;

    fqs->samples.unsampled++;
    fqs->samples_per_file.unsampled++;
    fqs->samples_per_time_slot.unsampled[slot]++;

    if(fqs->samples_per_file.unsampled > fqs->samples_per_file.sampled) {
        double progress_by_time = sampling_running_file_query_progress_by_time(fqs, jf, direction, msg_ut);

        if(progress_by_time > SYSTEMD_JOURNAL_ENABLE_ESTIMATIONS_FILE_PERCENTAGE)
            return SAMPLING_STOP_AND_ESTIMATE;
    }

    return SAMPLING_SKIP_FIELDS;
}

static void sampling_update_running_query_file_estimates(FACETS *facets, sd_journal *j, FUNCTION_QUERY_STATUS *fqs, struct journal_file *jf, usec_t msg_ut, FACETS_ANCHOR_DIRECTION direction) {
    usec_t total_time_ut, remaining_start_ut, remaining_end_ut;
    sampling_running_file_query_remaining_time(fqs, jf, direction, msg_ut, &total_time_ut, &remaining_start_ut,
                                               &remaining_end_ut);
    size_t remaining_lines = sampling_running_file_query_estimate_remaining_lines(j, fqs, jf, direction, msg_ut);
    facets_update_estimations(facets, remaining_start_ut, remaining_end_ut, remaining_lines);
    fqs->samples.estimated += remaining_lines;
    fqs->samples_per_file.estimated += remaining_lines;
}

// ----------------------------------------------------------------------------

static inline size_t netdata_systemd_journal_process_row(sd_journal *j, FACETS *facets, struct journal_file *jf, usec_t *msg_ut) {
    const void *data;
    size_t length, bytes = 0;

    facets_add_key_value_length(facets, JOURNAL_KEY_ND_JOURNAL_FILE, sizeof(JOURNAL_KEY_ND_JOURNAL_FILE) - 1, jf->filename, jf->filename_len);

    SD_JOURNAL_FOREACH_DATA(j, data, length) {
        const char *key, *value;
        size_t key_length, value_length;

        if(!parse_journal_field(data, length, &key, &key_length, &value, &value_length))
            continue;

#ifdef NETDATA_INTERNAL_CHECKS
        usec_t origin_journal_ut = *msg_ut;
#endif
        if(unlikely(key_length == sizeof(JD_SOURCE_REALTIME_TIMESTAMP) - 1 &&
            memcmp(key, JD_SOURCE_REALTIME_TIMESTAMP, sizeof(JD_SOURCE_REALTIME_TIMESTAMP) - 1) == 0)) {
            usec_t ut = str2ull(value, NULL);
            if(ut && ut < *msg_ut) {
                usec_t delta = *msg_ut - ut;
                *msg_ut = ut;

                if(delta > JOURNAL_VS_REALTIME_DELTA_MAX_UT)
                    delta = JOURNAL_VS_REALTIME_DELTA_MAX_UT;

                // update max_journal_vs_realtime_delta_ut if the delta increased
                usec_t expected = jf->max_journal_vs_realtime_delta_ut;
                do {
                    if(delta <= expected)
                        break;
                } while(!__atomic_compare_exchange_n(&jf->max_journal_vs_realtime_delta_ut, &expected, delta, false, __ATOMIC_RELAXED, __ATOMIC_RELAXED));

                internal_error(delta > expected,
                               "increased max_journal_vs_realtime_delta_ut from %"PRIu64" to %"PRIu64", "
                               "journal %"PRIu64", actual %"PRIu64" (delta %"PRIu64")"
                               , expected, delta, origin_journal_ut, *msg_ut, origin_journal_ut - (*msg_ut));
            }
        }

        bytes += length;
        facets_add_key_value_length(facets, key, key_length, value, value_length <= FACET_MAX_VALUE_LENGTH ? value_length : FACET_MAX_VALUE_LENGTH);
    }

    return bytes;
}

#define FUNCTION_PROGRESS_UPDATE_ROWS(rows_read, rows) __atomic_fetch_add(&(rows_read), rows, __ATOMIC_RELAXED)
#define FUNCTION_PROGRESS_UPDATE_BYTES(bytes_read, bytes) __atomic_fetch_add(&(bytes_read), bytes, __ATOMIC_RELAXED)
#define FUNCTION_PROGRESS_EVERY_ROWS (1ULL << 13)
#define FUNCTION_DATA_ONLY_CHECK_EVERY_ROWS (1ULL << 7)

static inline ND_SD_JOURNAL_STATUS check_stop(const bool *cancelled, const usec_t *stop_monotonic_ut) {
    if(cancelled && __atomic_load_n(cancelled, __ATOMIC_RELAXED)) {
        internal_error(true, "Function has been cancelled");
        return ND_SD_JOURNAL_CANCELLED;
    }

    if(now_monotonic_usec() > __atomic_load_n(stop_monotonic_ut, __ATOMIC_RELAXED)) {
        internal_error(true, "Function timed out");
        return ND_SD_JOURNAL_TIMED_OUT;
    }

    return ND_SD_JOURNAL_OK;
}

ND_SD_JOURNAL_STATUS netdata_systemd_journal_query_backward(
        sd_journal *j, BUFFER *wb __maybe_unused, FACETS *facets,
        struct journal_file *jf, FUNCTION_QUERY_STATUS *fqs) {

    usec_t anchor_delta = __atomic_load_n(&jf->max_journal_vs_realtime_delta_ut, __ATOMIC_RELAXED);

    usec_t start_ut = ((fqs->data_only && fqs->anchor.start_ut) ? fqs->anchor.start_ut : fqs->before_ut) + anchor_delta;
    usec_t stop_ut = (fqs->data_only && fqs->anchor.stop_ut) ? fqs->anchor.stop_ut : fqs->after_ut;
    bool stop_when_full = (fqs->data_only && !fqs->anchor.stop_ut);

    fqs->query_file.start_ut = start_ut;
    fqs->query_file.stop_ut = stop_ut;

    if(!netdata_systemd_journal_seek_to(j, start_ut))
        return ND_SD_JOURNAL_FAILED_TO_SEEK;

    size_t errors_no_timestamp = 0;
    usec_t latest_msg_ut = 0; // the biggest timestamp we have seen so far
    usec_t first_msg_ut = 0; // the first message we got from the db
    size_t row_counter = 0, last_row_counter = 0, rows_useful = 0;
    size_t bytes = 0, last_bytes = 0;

    usec_t last_usec_from = 0;
    usec_t last_usec_to = 0;

    ND_SD_JOURNAL_STATUS status = ND_SD_JOURNAL_OK;

    facets_rows_begin(facets);
    while (status == ND_SD_JOURNAL_OK && sd_journal_previous(j) > 0) {
        usec_t msg_ut = 0;
        if(sd_journal_get_realtime_usec(j, &msg_ut) < 0 || !msg_ut) {
            errors_no_timestamp++;
            continue;
        }

        if (unlikely(msg_ut > start_ut))
            continue;

        if (unlikely(msg_ut < stop_ut))
            break;

        if(unlikely(msg_ut > latest_msg_ut))
            latest_msg_ut = msg_ut;

        if(unlikely(!first_msg_ut)) {
            first_msg_ut = msg_ut;
            fqs->query_file.first_msg_ut = msg_ut;

#ifdef HAVE_SD_JOURNAL_GET_SEQNUM
            if(sd_journal_get_seqnum(j, &fqs->query_file.first_msg_seqnum, &fqs->query_file.first_msg_writer) < 0) {
                fqs->query_file.first_msg_seqnum = 0;
                fqs->query_file.first_msg_writer = SD_ID128_NULL;
            }
#endif
        }

        sampling_t sample = is_row_in_sample(j, fqs, jf, msg_ut,
                                        FACETS_ANCHOR_DIRECTION_BACKWARD,
                                        facets_row_candidate_to_keep(facets, msg_ut));

        if(sample == SAMPLING_FULL) {
            bytes += netdata_systemd_journal_process_row(j, facets, jf, &msg_ut);

            // make sure each line gets a unique timestamp
            if(unlikely(msg_ut >= last_usec_from && msg_ut <= last_usec_to))
                msg_ut = --last_usec_from;
            else
                last_usec_from = last_usec_to = msg_ut;

            if(facets_row_finished(facets, msg_ut))
                rows_useful++;

            row_counter++;
            if(unlikely((row_counter % FUNCTION_DATA_ONLY_CHECK_EVERY_ROWS) == 0 &&
                        stop_when_full &&
                        facets_rows(facets) >= fqs->entries)) {
                // stop the data only query
                usec_t oldest = facets_row_oldest_ut(facets);
                if(oldest && msg_ut < (oldest - anchor_delta))
                    break;
            }

            if(unlikely(row_counter % FUNCTION_PROGRESS_EVERY_ROWS == 0)) {
                FUNCTION_PROGRESS_UPDATE_ROWS(fqs->rows_read, row_counter - last_row_counter);
                last_row_counter = row_counter;

                FUNCTION_PROGRESS_UPDATE_BYTES(fqs->bytes_read, bytes - last_bytes);
                last_bytes = bytes;

                status = check_stop(fqs->cancelled, fqs->stop_monotonic_ut);
            }
        }
        else if(sample == SAMPLING_SKIP_FIELDS)
            facets_row_finished_unsampled(facets, msg_ut);
        else {
            sampling_update_running_query_file_estimates(facets, j, fqs, jf, msg_ut, FACETS_ANCHOR_DIRECTION_BACKWARD);
            break;
        }
    }

    FUNCTION_PROGRESS_UPDATE_ROWS(fqs->rows_read, row_counter - last_row_counter);
    FUNCTION_PROGRESS_UPDATE_BYTES(fqs->bytes_read, bytes - last_bytes);

    fqs->rows_useful += rows_useful;

    if(errors_no_timestamp)
        netdata_log_error("SYSTEMD-JOURNAL: %zu lines did not have timestamps", errors_no_timestamp);

    if(latest_msg_ut > fqs->last_modified)
        fqs->last_modified = latest_msg_ut;

    return status;
}

ND_SD_JOURNAL_STATUS netdata_systemd_journal_query_forward(
        sd_journal *j, BUFFER *wb __maybe_unused, FACETS *facets,
        struct journal_file *jf, FUNCTION_QUERY_STATUS *fqs) {

    usec_t anchor_delta = __atomic_load_n(&jf->max_journal_vs_realtime_delta_ut, __ATOMIC_RELAXED);

    usec_t start_ut = (fqs->data_only && fqs->anchor.start_ut) ? fqs->anchor.start_ut : fqs->after_ut;
    usec_t stop_ut = ((fqs->data_only && fqs->anchor.stop_ut) ? fqs->anchor.stop_ut : fqs->before_ut) + anchor_delta;
    bool stop_when_full = (fqs->data_only && !fqs->anchor.stop_ut);

    fqs->query_file.start_ut = start_ut;
    fqs->query_file.stop_ut = stop_ut;

    if(!netdata_systemd_journal_seek_to(j, start_ut))
        return ND_SD_JOURNAL_FAILED_TO_SEEK;

    size_t errors_no_timestamp = 0;
    usec_t latest_msg_ut = 0; // the biggest timestamp we have seen so far
    usec_t first_msg_ut = 0; // the first message we got from the db
    size_t row_counter = 0, last_row_counter = 0, rows_useful = 0;
    size_t bytes = 0, last_bytes = 0;

    usec_t last_usec_from = 0;
    usec_t last_usec_to = 0;

    ND_SD_JOURNAL_STATUS status = ND_SD_JOURNAL_OK;

    facets_rows_begin(facets);
    while (status == ND_SD_JOURNAL_OK && sd_journal_next(j) > 0) {
        usec_t msg_ut = 0;
        if(sd_journal_get_realtime_usec(j, &msg_ut) < 0 || !msg_ut) {
            errors_no_timestamp++;
            continue;
        }

        if (unlikely(msg_ut < start_ut))
            continue;

        if (unlikely(msg_ut > stop_ut))
            break;

        if(likely(msg_ut > latest_msg_ut))
            latest_msg_ut = msg_ut;

        if(unlikely(!first_msg_ut)) {
            first_msg_ut = msg_ut;
            fqs->query_file.first_msg_ut = msg_ut;
        }

        sampling_t sample = is_row_in_sample(j, fqs, jf, msg_ut,
                                        FACETS_ANCHOR_DIRECTION_FORWARD,
                                        facets_row_candidate_to_keep(facets, msg_ut));

        if(sample == SAMPLING_FULL) {
            bytes += netdata_systemd_journal_process_row(j, facets, jf, &msg_ut);

            // make sure each line gets a unique timestamp
            if(unlikely(msg_ut >= last_usec_from && msg_ut <= last_usec_to))
                msg_ut = ++last_usec_to;
            else
                last_usec_from = last_usec_to = msg_ut;

            if(facets_row_finished(facets, msg_ut))
                rows_useful++;

            row_counter++;
            if(unlikely((row_counter % FUNCTION_DATA_ONLY_CHECK_EVERY_ROWS) == 0 &&
                        stop_when_full &&
                        facets_rows(facets) >= fqs->entries)) {
                // stop the data only query
                usec_t newest = facets_row_newest_ut(facets);
                if(newest && msg_ut > (newest + anchor_delta))
                    break;
            }

            if(unlikely(row_counter % FUNCTION_PROGRESS_EVERY_ROWS == 0)) {
                FUNCTION_PROGRESS_UPDATE_ROWS(fqs->rows_read, row_counter - last_row_counter);
                last_row_counter = row_counter;

                FUNCTION_PROGRESS_UPDATE_BYTES(fqs->bytes_read, bytes - last_bytes);
                last_bytes = bytes;

                status = check_stop(fqs->cancelled, fqs->stop_monotonic_ut);
            }
        }
        else if(sample == SAMPLING_SKIP_FIELDS)
            facets_row_finished_unsampled(facets, msg_ut);
        else {
            sampling_update_running_query_file_estimates(facets, j, fqs, jf, msg_ut, FACETS_ANCHOR_DIRECTION_FORWARD);
            break;
        }
    }

    FUNCTION_PROGRESS_UPDATE_ROWS(fqs->rows_read, row_counter - last_row_counter);
    FUNCTION_PROGRESS_UPDATE_BYTES(fqs->bytes_read, bytes - last_bytes);

    fqs->rows_useful += rows_useful;

    if(errors_no_timestamp)
        netdata_log_error("SYSTEMD-JOURNAL: %zu lines did not have timestamps", errors_no_timestamp);

    if(latest_msg_ut > fqs->last_modified)
        fqs->last_modified = latest_msg_ut;

    return status;
}

bool netdata_systemd_journal_check_if_modified_since(sd_journal *j, usec_t seek_to, usec_t last_modified) {
    // return true, if data have been modified since the timestamp

    if(!last_modified || !seek_to)
        return false;

    if(!netdata_systemd_journal_seek_to(j, seek_to))
        return false;

    usec_t first_msg_ut = 0;
    while (sd_journal_previous(j) > 0) {
        usec_t msg_ut;
        if(sd_journal_get_realtime_usec(j, &msg_ut) < 0)
            continue;

        first_msg_ut = msg_ut;
        break;
    }

    return first_msg_ut != last_modified;
}

#ifdef HAVE_SD_JOURNAL_RESTART_FIELDS
static bool netdata_systemd_filtering_by_journal(sd_journal *j, FACETS *facets, FUNCTION_QUERY_STATUS *fqs) {
    const char *field = NULL;
    const void *data = NULL;
    size_t data_length;
    size_t added_keys = 0;
    size_t failures = 0;
    size_t filters_added = 0;

    SD_JOURNAL_FOREACH_FIELD(j, field) { // for each key
        bool interesting;

        if(fqs->data_only)
            interesting = facets_key_name_is_filter(facets, field);
        else
            interesting = facets_key_name_is_facet(facets, field);

        if(interesting) {
            if(sd_journal_query_unique(j, field) >= 0) {
                bool added_this_key = false;
                size_t added_values = 0;

                SD_JOURNAL_FOREACH_UNIQUE(j, data, data_length) { // for each value of the key
                    const char *key, *value;
                    size_t key_length, value_length;

                    if(!parse_journal_field(data, data_length, &key, &key_length, &value, &value_length))
                        continue;

                    facets_add_possible_value_name_to_key(facets, key, key_length, value, value_length);

                    if(!facets_key_name_value_length_is_selected(facets, key, key_length, value, value_length))
                        continue;

                    if(added_keys && !added_this_key) {
                        if(sd_journal_add_conjunction(j) < 0) // key AND key AND key
                            failures++;

                        added_this_key = true;
                        added_keys++;
                    }
                    else if(added_values)
                        if(sd_journal_add_disjunction(j) < 0) // value OR value OR value
                            failures++;

                    if(sd_journal_add_match(j, data, data_length) < 0)
                        failures++;

                    if(!added_keys) {
                        added_keys++;
                        added_this_key = true;
                    }

                    added_values++;
                    filters_added++;
                }
            }
        }
    }

    if(failures) {
        log_fqs(fqs, "failed to setup journal filter, will run the full query.");
        sd_journal_flush_matches(j);
        return true;
    }

    return filters_added ? true : false;
}
#endif // HAVE_SD_JOURNAL_RESTART_FIELDS

static ND_SD_JOURNAL_STATUS netdata_systemd_journal_query_one_file(
        const char *filename, BUFFER *wb, FACETS *facets,
        struct journal_file *jf, FUNCTION_QUERY_STATUS *fqs) {

    sd_journal *j = NULL;
    errno = 0;

    fstat_cache_enable_on_thread();

    const char *paths[2] = {
            [0] = filename,
            [1] = NULL,
    };

    if(sd_journal_open_files(&j, paths, ND_SD_JOURNAL_OPEN_FLAGS) < 0 || !j) {
        netdata_log_error("JOURNAL: cannot open file '%s' for query", filename);
        fstat_cache_disable_on_thread();
        return ND_SD_JOURNAL_FAILED_TO_OPEN;
    }

    ND_SD_JOURNAL_STATUS status;
    bool matches_filters = true;

#ifdef HAVE_SD_JOURNAL_RESTART_FIELDS
    if(fqs->slice) {
        usec_t started = now_monotonic_usec();

        matches_filters = netdata_systemd_filtering_by_journal(j, facets, fqs) || !fqs->filters;
        usec_t ended = now_monotonic_usec();

        fqs->matches_setup_ut += (ended - started);
    }
#endif // HAVE_SD_JOURNAL_RESTART_FIELDS

    if(matches_filters) {
        if(fqs->direction == FACETS_ANCHOR_DIRECTION_FORWARD)
            status = netdata_systemd_journal_query_forward(j, wb, facets, jf, fqs);
        else
            status = netdata_systemd_journal_query_backward(j, wb, facets, jf, fqs);
    }
    else
        status = ND_SD_JOURNAL_NO_FILE_MATCHED;

    sd_journal_close(j);
    fstat_cache_disable_on_thread();

    return status;
}

static bool jf_is_mine(struct journal_file *jf, FUNCTION_QUERY_STATUS *fqs) {

    if((fqs->source_type == SDJF_NONE && !fqs->sources) || (jf->source_type & fqs->source_type) ||
       (fqs->sources && simple_pattern_matches(fqs->sources, string2str(jf->source)))) {

        if(!jf->msg_last_ut)
            // the file is not scanned yet, or the timestamps have not been updated,
            // so we don't know if it can contribute or not - let's add it.
            return true;

        usec_t anchor_delta = JOURNAL_VS_REALTIME_DELTA_MAX_UT;
        usec_t first_ut = jf->msg_first_ut - anchor_delta;
        usec_t last_ut = jf->msg_last_ut + anchor_delta;

        if(last_ut >= fqs->after_ut && first_ut <= fqs->before_ut)
            return true;
    }

    return false;
}

static int netdata_systemd_journal_query(BUFFER *wb, FACETS *facets, FUNCTION_QUERY_STATUS *fqs) {
    ND_SD_JOURNAL_STATUS status = ND_SD_JOURNAL_NO_FILE_MATCHED;
    struct journal_file *jf;

    fqs->files_matched = 0;
    fqs->file_working = 0;
    fqs->rows_useful = 0;
    fqs->rows_read = 0;
    fqs->bytes_read = 0;

    size_t files_used = 0;
    size_t files_max = dictionary_entries(journal_files_registry);
    const DICTIONARY_ITEM *file_items[files_max];

    // count the files
    bool files_are_newer = false;
    dfe_start_read(journal_files_registry, jf) {
        if(!jf_is_mine(jf, fqs))
            continue;

        file_items[files_used++] = dictionary_acquired_item_dup(journal_files_registry, jf_dfe.item);

        if(jf->msg_last_ut > fqs->if_modified_since)
            files_are_newer = true;
    }
    dfe_done(jf);

    fqs->files_matched = files_used;

    if(fqs->if_modified_since && !files_are_newer) {
        buffer_flush(wb);
        return HTTP_RESP_NOT_MODIFIED;
    }

    // sort the files, so that they are optimal for facets
    if(files_used >= 2) {
        if (fqs->direction == FACETS_ANCHOR_DIRECTION_BACKWARD)
            qsort(file_items, files_used, sizeof(const DICTIONARY_ITEM *),
                  journal_file_dict_items_backward_compar);
        else
            qsort(file_items, files_used, sizeof(const DICTIONARY_ITEM *),
                  journal_file_dict_items_forward_compar);
    }

    bool partial = false;
    usec_t query_started_ut = now_monotonic_usec();
    usec_t started_ut = query_started_ut;
    usec_t ended_ut = started_ut;
    usec_t duration_ut = 0, max_duration_ut = 0;
    usec_t progress_duration_ut = 0;

    sampling_query_init(fqs, facets);

    buffer_json_member_add_array(wb, "_journal_files");
    for(size_t f = 0; f < files_used ;f++) {
        const char *filename = dictionary_acquired_item_name(file_items[f]);
        jf = dictionary_acquired_item_value(file_items[f]);

        if(!jf_is_mine(jf, fqs))
            continue;

        started_ut = ended_ut;

        // do not even try to do the query if we expect it to pass the timeout
        if(ended_ut + max_duration_ut * 3 >= *fqs->stop_monotonic_ut) {
            partial = true;
            status = ND_SD_JOURNAL_TIMED_OUT;
            break;
        }

        fqs->file_working++;
        // fqs->cached_count = 0;

        size_t fs_calls = fstat_thread_calls;
        size_t fs_cached = fstat_thread_cached_responses;
        size_t rows_useful = fqs->rows_useful;
        size_t rows_read = fqs->rows_read;
        size_t bytes_read = fqs->bytes_read;
        size_t matches_setup_ut = fqs->matches_setup_ut;

        sampling_file_init(fqs, jf);

        ND_SD_JOURNAL_STATUS tmp_status = netdata_systemd_journal_query_one_file(filename, wb, facets, jf, fqs);

//        nd_log(NDLS_COLLECTORS, NDLP_INFO,
//               "JOURNAL ESTIMATION FINAL: '%s' "
//               "total lines %zu [sampled=%zu, unsampled=%zu, estimated=%zu], "
//               "file [%"PRIu64" - %"PRIu64", duration %"PRId64", known lines in file %zu], "
//               "query [%"PRIu64" - %"PRIu64", duration %"PRId64"], "
//               , jf->filename
//               , fqs->samples_per_file.sampled + fqs->samples_per_file.unsampled + fqs->samples_per_file.estimated
//               , fqs->samples_per_file.sampled, fqs->samples_per_file.unsampled, fqs->samples_per_file.estimated
//               , jf->msg_first_ut, jf->msg_last_ut, jf->msg_last_ut - jf->msg_first_ut, jf->messages_in_file
//               , fqs->query_file.start_ut, fqs->query_file.stop_ut, fqs->query_file.stop_ut - fqs->query_file.start_ut
//        );

        rows_useful = fqs->rows_useful - rows_useful;
        rows_read = fqs->rows_read - rows_read;
        bytes_read = fqs->bytes_read - bytes_read;
        matches_setup_ut = fqs->matches_setup_ut - matches_setup_ut;
        fs_calls = fstat_thread_calls - fs_calls;
        fs_cached = fstat_thread_cached_responses - fs_cached;

        ended_ut = now_monotonic_usec();
        duration_ut = ended_ut - started_ut;

        if(duration_ut > max_duration_ut)
            max_duration_ut = duration_ut;

        progress_duration_ut += duration_ut;
        if(progress_duration_ut >= SYSTEMD_JOURNAL_PROGRESS_EVERY_UT) {
            progress_duration_ut = 0;
            netdata_mutex_lock(&stdout_mutex);
            pluginsd_function_progress_to_stdout(fqs->transaction, f + 1, files_used);
            netdata_mutex_unlock(&stdout_mutex);
        }

        buffer_json_add_array_item_object(wb); // journal file
        {
            // information about the file
            buffer_json_member_add_string(wb, "_filename", filename);
            buffer_json_member_add_uint64(wb, "_source_type", jf->source_type);
            buffer_json_member_add_string(wb, "_source", string2str(jf->source));
            buffer_json_member_add_uint64(wb, "_last_modified_ut", jf->file_last_modified_ut);
            buffer_json_member_add_uint64(wb, "_msg_first_ut", jf->msg_first_ut);
            buffer_json_member_add_uint64(wb, "_msg_last_ut", jf->msg_last_ut);
            buffer_json_member_add_uint64(wb, "_journal_vs_realtime_delta_ut", jf->max_journal_vs_realtime_delta_ut);

            // information about the current use of the file
            buffer_json_member_add_uint64(wb, "duration_ut", ended_ut - started_ut);
            buffer_json_member_add_uint64(wb, "rows_read", rows_read);
            buffer_json_member_add_uint64(wb, "rows_useful", rows_useful);
            buffer_json_member_add_double(wb, "rows_per_second", (double) rows_read / (double) duration_ut * (double) USEC_PER_SEC);
            buffer_json_member_add_uint64(wb, "bytes_read", bytes_read);
            buffer_json_member_add_double(wb, "bytes_per_second", (double) bytes_read / (double) duration_ut * (double) USEC_PER_SEC);
            buffer_json_member_add_uint64(wb, "duration_matches_ut", matches_setup_ut);
            buffer_json_member_add_uint64(wb, "fstat_query_calls", fs_calls);
            buffer_json_member_add_uint64(wb, "fstat_query_cached_responses", fs_cached);

            if(fqs->sampling) {
                buffer_json_member_add_object(wb, "_sampling");
                {
                    buffer_json_member_add_uint64(wb, "sampled", fqs->samples_per_file.sampled);
                    buffer_json_member_add_uint64(wb, "unsampled", fqs->samples_per_file.unsampled);
                    buffer_json_member_add_uint64(wb, "estimated", fqs->samples_per_file.estimated);
                }
                buffer_json_object_close(wb); // _sampling
            }
        }
        buffer_json_object_close(wb); // journal file

        bool stop = false;
        switch(tmp_status) {
            case ND_SD_JOURNAL_OK:
            case ND_SD_JOURNAL_NO_FILE_MATCHED:
                status = (status == ND_SD_JOURNAL_OK) ? ND_SD_JOURNAL_OK : tmp_status;
                break;

            case ND_SD_JOURNAL_FAILED_TO_OPEN:
            case ND_SD_JOURNAL_FAILED_TO_SEEK:
                partial = true;
                if(status == ND_SD_JOURNAL_NO_FILE_MATCHED)
                    status = tmp_status;
                break;

            case ND_SD_JOURNAL_CANCELLED:
            case ND_SD_JOURNAL_TIMED_OUT:
                partial = true;
                stop = true;
                status = tmp_status;
                break;

            case ND_SD_JOURNAL_NOT_MODIFIED:
                internal_fatal(true, "this should never be returned here");
                break;
        }

        if(stop)
            break;
    }
    buffer_json_array_close(wb); // _journal_files

    // release the files
    for(size_t f = 0; f < files_used ;f++)
        dictionary_acquired_item_release(journal_files_registry, file_items[f]);

    switch (status) {
        case ND_SD_JOURNAL_OK:
            if(fqs->if_modified_since && !fqs->rows_useful) {
                buffer_flush(wb);
                return HTTP_RESP_NOT_MODIFIED;
            }
            break;

        case ND_SD_JOURNAL_TIMED_OUT:
        case ND_SD_JOURNAL_NO_FILE_MATCHED:
            break;

        case ND_SD_JOURNAL_CANCELLED:
            buffer_flush(wb);
            return HTTP_RESP_CLIENT_CLOSED_REQUEST;

        case ND_SD_JOURNAL_NOT_MODIFIED:
            buffer_flush(wb);
            return HTTP_RESP_NOT_MODIFIED;

        default:
        case ND_SD_JOURNAL_FAILED_TO_OPEN:
        case ND_SD_JOURNAL_FAILED_TO_SEEK:
            buffer_flush(wb);
            return HTTP_RESP_INTERNAL_SERVER_ERROR;
    }

    buffer_json_member_add_uint64(wb, "status", HTTP_RESP_OK);
    buffer_json_member_add_boolean(wb, "partial", partial);
    buffer_json_member_add_string(wb, "type", "table");

    // build a message for the query
    if(!fqs->data_only) {
        CLEAN_BUFFER *msg = buffer_create(0, NULL);
        CLEAN_BUFFER *msg_description = buffer_create(0, NULL);
        ND_LOG_FIELD_PRIORITY msg_priority = NDLP_INFO;

        if(!journal_files_completed_once()) {
            buffer_strcat(msg, "Journals are still being scanned. ");
            buffer_strcat(msg_description
                          , "LIBRARY SCAN: The journal files are still being scanned, you are probably viewing incomplete data. ");
            msg_priority = NDLP_WARNING;
        }

        if(partial) {
            buffer_strcat(msg, "Query timed-out, incomplete data. ");
            buffer_strcat(msg_description
                          , "QUERY TIMEOUT: The query timed out and may not include all the data of the selected window. ");
            msg_priority = NDLP_WARNING;
        }

        if(fqs->samples.estimated || fqs->samples.unsampled) {
            double percent = (double) (fqs->samples.sampled * 100.0 /
                                       (fqs->samples.estimated + fqs->samples.unsampled + fqs->samples.sampled));
            buffer_sprintf(msg, "%.2f%% real data", percent);
            buffer_sprintf(msg_description, "ACTUAL DATA: The filters counters reflect %0.2f%% of the data. ", percent);
            msg_priority = MIN(msg_priority, NDLP_NOTICE);
        }

        if(fqs->samples.unsampled) {
            double percent = (double) (fqs->samples.unsampled * 100.0 /
                                       (fqs->samples.estimated + fqs->samples.unsampled + fqs->samples.sampled));
            buffer_sprintf(msg, ", %.2f%% unsampled", percent);
            buffer_sprintf(msg_description
                           , "UNSAMPLED DATA: %0.2f%% of the events exist and have been counted, but their values have not been evaluated, so they are not included in the filters counters. "
                           , percent);
            msg_priority = MIN(msg_priority, NDLP_NOTICE);
        }

        if(fqs->samples.estimated) {
            double percent = (double) (fqs->samples.estimated * 100.0 /
                                       (fqs->samples.estimated + fqs->samples.unsampled + fqs->samples.sampled));
            buffer_sprintf(msg, ", %.2f%% estimated", percent);
            buffer_sprintf(msg_description
                           , "ESTIMATED DATA: The query selected a large amount of data, so to avoid delaying too much, the presented data are estimated by %0.2f%%. "
                           , percent);
            msg_priority = MIN(msg_priority, NDLP_NOTICE);
        }

        buffer_json_member_add_object(wb, "message");
        if(buffer_tostring(msg)) {
            buffer_json_member_add_string(wb, "title", buffer_tostring(msg));
            buffer_json_member_add_string(wb, "description", buffer_tostring(msg_description));
            buffer_json_member_add_string(wb, "status", nd_log_id2priority(msg_priority));
        }
        // else send an empty object if there is nothing to tell
        buffer_json_object_close(wb); // message
    }

    if(!fqs->data_only) {
        buffer_json_member_add_time_t(wb, "update_every", 1);
        buffer_json_member_add_string(wb, "help", SYSTEMD_JOURNAL_FUNCTION_DESCRIPTION);
    }

    if(!fqs->data_only || fqs->tail)
        buffer_json_member_add_uint64(wb, "last_modified", fqs->last_modified);

    facets_sort_and_reorder_keys(facets);
    facets_report(facets, wb, used_hashes_registry);

    buffer_json_member_add_time_t(wb, "expires", now_realtime_sec() + (fqs->data_only ? 3600 : 0));

    buffer_json_member_add_object(wb, "_fstat_caching");
    {
        buffer_json_member_add_uint64(wb, "calls", fstat_thread_calls);
        buffer_json_member_add_uint64(wb, "cached", fstat_thread_cached_responses);
    }
    buffer_json_object_close(wb); // _fstat_caching

    if(fqs->sampling) {
        buffer_json_member_add_object(wb, "_sampling");
        {
            buffer_json_member_add_uint64(wb, "sampled", fqs->samples.sampled);
            buffer_json_member_add_uint64(wb, "unsampled", fqs->samples.unsampled);
            buffer_json_member_add_uint64(wb, "estimated", fqs->samples.estimated);
        }
        buffer_json_object_close(wb); // _sampling
    }

    buffer_json_finalize(wb);

    return HTTP_RESP_OK;
}

static void netdata_systemd_journal_function_help(const char *transaction) {
    BUFFER *wb = buffer_create(0, NULL);
    buffer_sprintf(wb,
            "%s / %s\n"
            "\n"
            "%s\n"
            "\n"
            "The following parameters are supported:\n"
            "\n"
            "   "JOURNAL_PARAMETER_HELP"\n"
            "      Shows this help message.\n"
            "\n"
            "   "JOURNAL_PARAMETER_INFO"\n"
            "      Request initial configuration information about the plugin.\n"
            "      The key entity returned is the required_params array, which includes\n"
            "      all the available systemd journal sources.\n"
            "      When `"JOURNAL_PARAMETER_INFO"` is requested, all other parameters are ignored.\n"
            "\n"
            "   "JOURNAL_PARAMETER_DATA_ONLY":true or "JOURNAL_PARAMETER_DATA_ONLY":false\n"
            "      Quickly respond with data requested, without generating a\n"
            "      `histogram`, `facets` counters and `items`.\n"
            "\n"
            "   "JOURNAL_PARAMETER_DELTA":true or "JOURNAL_PARAMETER_DELTA":false\n"
            "      When doing data only queries, include deltas for histogram, facets and items.\n"
            "\n"
            "   "JOURNAL_PARAMETER_TAIL":true or "JOURNAL_PARAMETER_TAIL":false\n"
            "      When doing data only queries, respond with the newest messages,\n"
            "      and up to the anchor, but calculate deltas (if requested) for\n"
            "      the duration [anchor - before].\n"
            "\n"
            "   "JOURNAL_PARAMETER_SLICE":true or "JOURNAL_PARAMETER_SLICE":false\n"
            "      When it is turned on, the plugin is executing filtering via libsystemd,\n"
            "      utilizing all the available indexes of the journal files.\n"
            "      When it is off, only the time constraint is handled by libsystemd and\n"
            "      all filtering is done by the plugin.\n"
            "      The default is: %s\n"
            "\n"
            "   "JOURNAL_PARAMETER_SOURCE":SOURCE\n"
            "      Query only the specified journal sources.\n"
            "      Do an `"JOURNAL_PARAMETER_INFO"` query to find the sources.\n"
            "\n"
            "   "JOURNAL_PARAMETER_BEFORE":TIMESTAMP_IN_SECONDS\n"
            "      Absolute or relative (to now) timestamp in seconds, to start the query.\n"
            "      The query is always executed from the most recent to the oldest log entry.\n"
            "      If not given the default is: now.\n"
            "\n"
            "   "JOURNAL_PARAMETER_AFTER":TIMESTAMP_IN_SECONDS\n"
            "      Absolute or relative (to `before`) timestamp in seconds, to end the query.\n"
            "      If not given, the default is %d.\n"
            "\n"
            "   "JOURNAL_PARAMETER_LAST":ITEMS\n"
            "      The number of items to return.\n"
            "      The default is %d.\n"
            "\n"
            "   "JOURNAL_PARAMETER_SAMPLING":ITEMS\n"
            "      The number of log entries to sample to estimate facets counters and histogram.\n"
            "      The default is %d.\n"
            "\n"
            "   "JOURNAL_PARAMETER_ANCHOR":TIMESTAMP_IN_MICROSECONDS\n"
            "      Return items relative to this timestamp.\n"
            "      The exact items to be returned depend on the query `"JOURNAL_PARAMETER_DIRECTION"`.\n"
            "\n"
            "   "JOURNAL_PARAMETER_DIRECTION":forward or "JOURNAL_PARAMETER_DIRECTION":backward\n"
            "      When set to `backward` (default) the items returned are the newest before the\n"
            "      `"JOURNAL_PARAMETER_ANCHOR"`, (or `"JOURNAL_PARAMETER_BEFORE"` if `"JOURNAL_PARAMETER_ANCHOR"` is not set)\n"
            "      When set to `forward` the items returned are the oldest after the\n"
            "      `"JOURNAL_PARAMETER_ANCHOR"`, (or `"JOURNAL_PARAMETER_AFTER"` if `"JOURNAL_PARAMETER_ANCHOR"` is not set)\n"
            "      The default is: %s\n"
            "\n"
            "   "JOURNAL_PARAMETER_QUERY":SIMPLE_PATTERN\n"
            "      Do a full text search to find the log entries matching the pattern given.\n"
            "      The plugin is searching for matches on all fields of the database.\n"
            "\n"
            "   "JOURNAL_PARAMETER_IF_MODIFIED_SINCE":TIMESTAMP_IN_MICROSECONDS\n"
            "      Each successful response, includes a `last_modified` field.\n"
            "      By providing the timestamp to the `"JOURNAL_PARAMETER_IF_MODIFIED_SINCE"` parameter,\n"
            "      the plugin will return 200 with a successful response, or 304 if the source has not\n"
            "      been modified since that timestamp.\n"
            "\n"
            "   "JOURNAL_PARAMETER_HISTOGRAM":facet_id\n"
            "      Use the given `facet_id` for the histogram.\n"
            "      This parameter is ignored in `"JOURNAL_PARAMETER_DATA_ONLY"` mode.\n"
            "\n"
            "   "JOURNAL_PARAMETER_FACETS":facet_id1,facet_id2,facet_id3,...\n"
            "      Add the given facets to the list of fields for which analysis is required.\n"
            "      The plugin will offer both a histogram and facet value counters for its values.\n"
            "      This parameter is ignored in `"JOURNAL_PARAMETER_DATA_ONLY"` mode.\n"
            "\n"
            "   facet_id:value_id1,value_id2,value_id3,...\n"
            "      Apply filters to the query, based on the facet IDs returned.\n"
            "      Each `facet_id` can be given once, but multiple `facet_ids` can be given.\n"
            "\n"
            , program_name
            , SYSTEMD_JOURNAL_FUNCTION_NAME
            , SYSTEMD_JOURNAL_FUNCTION_DESCRIPTION
            , JOURNAL_DEFAULT_SLICE_MODE ? "true" : "false" // slice
            , -SYSTEMD_JOURNAL_DEFAULT_QUERY_DURATION
            , SYSTEMD_JOURNAL_DEFAULT_ITEMS_PER_QUERY
            , SYSTEMD_JOURNAL_DEFAULT_ITEMS_SAMPLING
            , JOURNAL_DEFAULT_DIRECTION == FACETS_ANCHOR_DIRECTION_BACKWARD ? "backward" : "forward"
    );

    netdata_mutex_lock(&stdout_mutex);
    pluginsd_function_result_to_stdout(transaction, HTTP_RESP_OK, "text/plain", now_realtime_sec() + 3600, wb);
    netdata_mutex_unlock(&stdout_mutex);

    buffer_free(wb);
}

void function_systemd_journal(const char *transaction, char *function, usec_t *stop_monotonic_ut, bool *cancelled,
                              BUFFER *payload __maybe_unused, HTTP_ACCESS access __maybe_unused,
                              const char *source __maybe_unused, void *data __maybe_unused) {
    fstat_thread_calls = 0;
    fstat_thread_cached_responses = 0;

    BUFFER *wb = buffer_create(0, NULL);
    buffer_flush(wb);
    buffer_json_initialize(wb, "\"", "\"", 0, true, BUFFER_JSON_OPTIONS_MINIFY);

    FUNCTION_QUERY_STATUS tmp_fqs = {
            .cancelled = cancelled,
            .stop_monotonic_ut = stop_monotonic_ut,
    };
    FUNCTION_QUERY_STATUS *fqs = NULL;

    FACETS *facets = facets_create(50, FACETS_OPTION_ALL_KEYS_FTS,
                                   SYSTEMD_ALWAYS_VISIBLE_KEYS,
                                   SYSTEMD_KEYS_INCLUDED_IN_FACETS,
                                   SYSTEMD_KEYS_EXCLUDED_FROM_FACETS);

    facets_accepted_param(facets, JOURNAL_PARAMETER_INFO);
    facets_accepted_param(facets, JOURNAL_PARAMETER_SOURCE);
    facets_accepted_param(facets, JOURNAL_PARAMETER_AFTER);
    facets_accepted_param(facets, JOURNAL_PARAMETER_BEFORE);
    facets_accepted_param(facets, JOURNAL_PARAMETER_ANCHOR);
    facets_accepted_param(facets, JOURNAL_PARAMETER_DIRECTION);
    facets_accepted_param(facets, JOURNAL_PARAMETER_LAST);
    facets_accepted_param(facets, JOURNAL_PARAMETER_QUERY);
    facets_accepted_param(facets, JOURNAL_PARAMETER_FACETS);
    facets_accepted_param(facets, JOURNAL_PARAMETER_HISTOGRAM);
    facets_accepted_param(facets, JOURNAL_PARAMETER_IF_MODIFIED_SINCE);
    facets_accepted_param(facets, JOURNAL_PARAMETER_DATA_ONLY);
    facets_accepted_param(facets, JOURNAL_PARAMETER_DELTA);
    facets_accepted_param(facets, JOURNAL_PARAMETER_TAIL);
    facets_accepted_param(facets, JOURNAL_PARAMETER_SAMPLING);

#ifdef HAVE_SD_JOURNAL_RESTART_FIELDS
    facets_accepted_param(facets, JOURNAL_PARAMETER_SLICE);
#endif // HAVE_SD_JOURNAL_RESTART_FIELDS

    // register the fields in the order you want them on the dashboard

    facets_register_row_severity(facets, syslog_priority_to_facet_severity, NULL);

    facets_register_key_name(facets, "_HOSTNAME",
                             FACET_KEY_OPTION_FACET | FACET_KEY_OPTION_VISIBLE);

    facets_register_dynamic_key_name(facets, JOURNAL_KEY_ND_JOURNAL_PROCESS,
                                     FACET_KEY_OPTION_NEVER_FACET | FACET_KEY_OPTION_VISIBLE,
                                     netdata_systemd_journal_dynamic_row_id, NULL);

    facets_register_key_name(facets, "MESSAGE",
                                     FACET_KEY_OPTION_NEVER_FACET | FACET_KEY_OPTION_MAIN_TEXT |
                                     FACET_KEY_OPTION_VISIBLE | FACET_KEY_OPTION_FTS);

//    facets_register_dynamic_key_name(facets, "MESSAGE",
//                             FACET_KEY_OPTION_NEVER_FACET | FACET_KEY_OPTION_MAIN_TEXT | FACET_KEY_OPTION_RICH_TEXT |
//                             FACET_KEY_OPTION_VISIBLE | FACET_KEY_OPTION_FTS,
//                             netdata_systemd_journal_rich_message, NULL);

    facets_register_key_name_transformation(facets, "PRIORITY",
                                            FACET_KEY_OPTION_FACET | FACET_KEY_OPTION_TRANSFORM_VIEW |
                                                    FACET_KEY_OPTION_EXPANDED_FILTER,
                                            netdata_systemd_journal_transform_priority, NULL);

    facets_register_key_name_transformation(facets, "SYSLOG_FACILITY",
                                            FACET_KEY_OPTION_FACET | FACET_KEY_OPTION_TRANSFORM_VIEW |
                                                    FACET_KEY_OPTION_EXPANDED_FILTER,
                                            netdata_systemd_journal_transform_syslog_facility, NULL);

    facets_register_key_name_transformation(facets, "ERRNO",
                                            FACET_KEY_OPTION_FACET | FACET_KEY_OPTION_TRANSFORM_VIEW,
                                            netdata_systemd_journal_transform_errno, NULL);

    facets_register_key_name(facets, JOURNAL_KEY_ND_JOURNAL_FILE,
                             FACET_KEY_OPTION_NEVER_FACET);

    facets_register_key_name(facets, "SYSLOG_IDENTIFIER",
                             FACET_KEY_OPTION_FACET);

    facets_register_key_name(facets, "UNIT",
                             FACET_KEY_OPTION_FACET);

    facets_register_key_name(facets, "USER_UNIT",
                             FACET_KEY_OPTION_FACET);

    facets_register_key_name_transformation(facets, "MESSAGE_ID",
                                            FACET_KEY_OPTION_FACET | FACET_KEY_OPTION_TRANSFORM_VIEW |
                                            FACET_KEY_OPTION_EXPANDED_FILTER,
                                            netdata_systemd_journal_transform_message_id, NULL);

    facets_register_key_name_transformation(facets, "_BOOT_ID",
                                            FACET_KEY_OPTION_FACET | FACET_KEY_OPTION_TRANSFORM_VIEW,
                                            netdata_systemd_journal_transform_boot_id, NULL);

    facets_register_key_name_transformation(facets, "_SYSTEMD_OWNER_UID",
                                            FACET_KEY_OPTION_FACET | FACET_KEY_OPTION_TRANSFORM_VIEW,
                                            netdata_systemd_journal_transform_uid, NULL);

    facets_register_key_name_transformation(facets, "_UID",
                                            FACET_KEY_OPTION_FACET | FACET_KEY_OPTION_TRANSFORM_VIEW,
                                            netdata_systemd_journal_transform_uid, NULL);

    facets_register_key_name_transformation(facets, "OBJECT_SYSTEMD_OWNER_UID",
                                            FACET_KEY_OPTION_FACET | FACET_KEY_OPTION_TRANSFORM_VIEW,
                                            netdata_systemd_journal_transform_uid, NULL);

    facets_register_key_name_transformation(facets, "OBJECT_UID",
                                            FACET_KEY_OPTION_FACET | FACET_KEY_OPTION_TRANSFORM_VIEW,
                                            netdata_systemd_journal_transform_uid, NULL);

    facets_register_key_name_transformation(facets, "_GID",
                                            FACET_KEY_OPTION_FACET | FACET_KEY_OPTION_TRANSFORM_VIEW,
                                            netdata_systemd_journal_transform_gid, NULL);

    facets_register_key_name_transformation(facets, "OBJECT_GID",
                                            FACET_KEY_OPTION_FACET | FACET_KEY_OPTION_TRANSFORM_VIEW,
                                            netdata_systemd_journal_transform_gid, NULL);

    facets_register_key_name_transformation(facets, "_CAP_EFFECTIVE",
                                            FACET_KEY_OPTION_TRANSFORM_VIEW,
                                            netdata_systemd_journal_transform_cap_effective, NULL);

    facets_register_key_name_transformation(facets, "_AUDIT_LOGINUID",
                                            FACET_KEY_OPTION_TRANSFORM_VIEW,
                                            netdata_systemd_journal_transform_uid, NULL);

    facets_register_key_name_transformation(facets, "OBJECT_AUDIT_LOGINUID",
                                            FACET_KEY_OPTION_TRANSFORM_VIEW,
                                            netdata_systemd_journal_transform_uid, NULL);

    facets_register_key_name_transformation(facets, "_SOURCE_REALTIME_TIMESTAMP",
                                            FACET_KEY_OPTION_TRANSFORM_VIEW,
                                            netdata_systemd_journal_transform_timestamp_usec, NULL);

    // ------------------------------------------------------------------------
    // parse the parameters

    bool info = false, data_only = false, slice = JOURNAL_DEFAULT_SLICE_MODE, delta = false, tail = false;
    time_t after_s = 0, before_s = 0;
    usec_t anchor = 0;
    usec_t if_modified_since = 0;
    size_t last = 0;
    FACETS_ANCHOR_DIRECTION direction = JOURNAL_DEFAULT_DIRECTION;
    const char *query = NULL;
    const char *chart = NULL;
    SIMPLE_PATTERN *sources = NULL;
    SD_JOURNAL_FILE_SOURCE_TYPE source_type = SDJF_ALL;
    size_t filters = 0;
    size_t sampling = SYSTEMD_JOURNAL_DEFAULT_ITEMS_SAMPLING;

    buffer_json_member_add_object(wb, "_request");

    char *words[SYSTEMD_JOURNAL_MAX_PARAMS] = { NULL };
    size_t num_words = quoted_strings_splitter_pluginsd(function, words, SYSTEMD_JOURNAL_MAX_PARAMS);
    for(int i = 1; i < SYSTEMD_JOURNAL_MAX_PARAMS ;i++) {
        char *keyword = get_word(words, num_words, i);
        if(!keyword) break;

        if(strcmp(keyword, JOURNAL_PARAMETER_HELP) == 0) {
            netdata_systemd_journal_function_help(transaction);
            goto cleanup;
        }
        else if(strcmp(keyword, JOURNAL_PARAMETER_INFO) == 0) {
            info = true;
        }
        else if(strncmp(keyword, JOURNAL_PARAMETER_DELTA ":", sizeof(JOURNAL_PARAMETER_DELTA ":") - 1) == 0) {
            char *v = &keyword[sizeof(JOURNAL_PARAMETER_DELTA ":") - 1];

            if(strcmp(v, "false") == 0 || strcmp(v, "no") == 0 || strcmp(v, "0") == 0)
                delta = false;
            else
                delta = true;
        }
        else if(strncmp(keyword, JOURNAL_PARAMETER_TAIL ":", sizeof(JOURNAL_PARAMETER_TAIL ":") - 1) == 0) {
            char *v = &keyword[sizeof(JOURNAL_PARAMETER_TAIL ":") - 1];

            if(strcmp(v, "false") == 0 || strcmp(v, "no") == 0 || strcmp(v, "0") == 0)
                tail = false;
            else
                tail = true;
        }
        else if(strncmp(keyword, JOURNAL_PARAMETER_SAMPLING ":", sizeof(JOURNAL_PARAMETER_SAMPLING ":") - 1) == 0) {
            sampling = str2ul(&keyword[sizeof(JOURNAL_PARAMETER_SAMPLING ":") - 1]);
        }
        else if(strncmp(keyword, JOURNAL_PARAMETER_DATA_ONLY ":", sizeof(JOURNAL_PARAMETER_DATA_ONLY ":") - 1) == 0) {
            char *v = &keyword[sizeof(JOURNAL_PARAMETER_DATA_ONLY ":") - 1];

            if(strcmp(v, "false") == 0 || strcmp(v, "no") == 0 || strcmp(v, "0") == 0)
                data_only = false;
            else
                data_only = true;
        }
        else if(strncmp(keyword, JOURNAL_PARAMETER_SLICE ":", sizeof(JOURNAL_PARAMETER_SLICE ":") - 1) == 0) {
            char *v = &keyword[sizeof(JOURNAL_PARAMETER_SLICE ":") - 1];

            if(strcmp(v, "false") == 0 || strcmp(v, "no") == 0 || strcmp(v, "0") == 0)
                slice = false;
            else
                slice = true;
        }
        else if(strncmp(keyword, JOURNAL_PARAMETER_SOURCE ":", sizeof(JOURNAL_PARAMETER_SOURCE ":") - 1) == 0) {
            const char *value = &keyword[sizeof(JOURNAL_PARAMETER_SOURCE ":") - 1];

            buffer_json_member_add_array(wb, JOURNAL_PARAMETER_SOURCE);

            BUFFER *sources_list = buffer_create(0, NULL);

            source_type = SDJF_NONE;
            while(value) {
                char *sep = strchr(value, ',');
                if(sep)
                    *sep++ = '\0';

                buffer_json_add_array_item_string(wb, value);

                if(strcmp(value, SDJF_SOURCE_ALL_NAME) == 0) {
                    source_type |= SDJF_ALL;
                    value = NULL;
                }
                else if(strcmp(value, SDJF_SOURCE_LOCAL_NAME) == 0) {
                    source_type |= SDJF_LOCAL_ALL;
                    value = NULL;
                }
                else if(strcmp(value, SDJF_SOURCE_REMOTES_NAME) == 0) {
                    source_type |= SDJF_REMOTE_ALL;
                    value = NULL;
                }
                else if(strcmp(value, SDJF_SOURCE_NAMESPACES_NAME) == 0) {
                    source_type |= SDJF_LOCAL_NAMESPACE;
                    value = NULL;
                }
                else if(strcmp(value, SDJF_SOURCE_LOCAL_SYSTEM_NAME) == 0) {
                    source_type |= SDJF_LOCAL_SYSTEM;
                    value = NULL;
                }
                else if(strcmp(value, SDJF_SOURCE_LOCAL_USERS_NAME) == 0) {
                    source_type |= SDJF_LOCAL_USER;
                    value = NULL;
                }
                else if(strcmp(value, SDJF_SOURCE_LOCAL_OTHER_NAME) == 0) {
                    source_type |= SDJF_LOCAL_OTHER;
                    value = NULL;
                }
                else {
                    // else, match the source, whatever it is
                    if(buffer_strlen(sources_list))
                        buffer_strcat(sources_list, ",");

                    buffer_strcat(sources_list, value);
                }

                value = sep;
            }

            if(buffer_strlen(sources_list)) {
                simple_pattern_free(sources);
                sources = simple_pattern_create(buffer_tostring(sources_list), ",", SIMPLE_PATTERN_EXACT, false);
            }

            buffer_free(sources_list);

            buffer_json_array_close(wb); // source
        }
        else if(strncmp(keyword, JOURNAL_PARAMETER_AFTER ":", sizeof(JOURNAL_PARAMETER_AFTER ":") - 1) == 0) {
            after_s = str2l(&keyword[sizeof(JOURNAL_PARAMETER_AFTER ":") - 1]);
        }
        else if(strncmp(keyword, JOURNAL_PARAMETER_BEFORE ":", sizeof(JOURNAL_PARAMETER_BEFORE ":") - 1) == 0) {
            before_s = str2l(&keyword[sizeof(JOURNAL_PARAMETER_BEFORE ":") - 1]);
        }
        else if(strncmp(keyword, JOURNAL_PARAMETER_IF_MODIFIED_SINCE ":", sizeof(JOURNAL_PARAMETER_IF_MODIFIED_SINCE ":") - 1) == 0) {
            if_modified_since = str2ull(&keyword[sizeof(JOURNAL_PARAMETER_IF_MODIFIED_SINCE ":") - 1], NULL);
        }
        else if(strncmp(keyword, JOURNAL_PARAMETER_ANCHOR ":", sizeof(JOURNAL_PARAMETER_ANCHOR ":") - 1) == 0) {
            anchor = str2ull(&keyword[sizeof(JOURNAL_PARAMETER_ANCHOR ":") - 1], NULL);
        }
        else if(strncmp(keyword, JOURNAL_PARAMETER_DIRECTION ":", sizeof(JOURNAL_PARAMETER_DIRECTION ":") - 1) == 0) {
            direction = strcasecmp(&keyword[sizeof(JOURNAL_PARAMETER_DIRECTION ":") - 1], "forward") == 0 ? FACETS_ANCHOR_DIRECTION_FORWARD : FACETS_ANCHOR_DIRECTION_BACKWARD;
        }
        else if(strncmp(keyword, JOURNAL_PARAMETER_LAST ":", sizeof(JOURNAL_PARAMETER_LAST ":") - 1) == 0) {
            last = str2ul(&keyword[sizeof(JOURNAL_PARAMETER_LAST ":") - 1]);
        }
        else if(strncmp(keyword, JOURNAL_PARAMETER_QUERY ":", sizeof(JOURNAL_PARAMETER_QUERY ":") - 1) == 0) {
            query= &keyword[sizeof(JOURNAL_PARAMETER_QUERY ":") - 1];
        }
        else if(strncmp(keyword, JOURNAL_PARAMETER_HISTOGRAM ":", sizeof(JOURNAL_PARAMETER_HISTOGRAM ":") - 1) == 0) {
            chart = &keyword[sizeof(JOURNAL_PARAMETER_HISTOGRAM ":") - 1];
        }
        else if(strncmp(keyword, JOURNAL_PARAMETER_FACETS ":", sizeof(JOURNAL_PARAMETER_FACETS ":") - 1) == 0) {
            char *value = &keyword[sizeof(JOURNAL_PARAMETER_FACETS ":") - 1];
            if(*value) {
                buffer_json_member_add_array(wb, JOURNAL_PARAMETER_FACETS);

                while(value) {
                    char *sep = strchr(value, ',');
                    if(sep)
                        *sep++ = '\0';

                    facets_register_facet_id(facets, value, FACET_KEY_OPTION_FACET|FACET_KEY_OPTION_FTS|FACET_KEY_OPTION_REORDER);
                    buffer_json_add_array_item_string(wb, value);

                    value = sep;
                }

                buffer_json_array_close(wb); // JOURNAL_PARAMETER_FACETS
            }
        }
        else {
            char *value = strchr(keyword, ':');
            if(value) {
                *value++ = '\0';

                buffer_json_member_add_array(wb, keyword);

                while(value) {
                    char *sep = strchr(value, ',');
                    if(sep)
                        *sep++ = '\0';

                    facets_register_facet_id_filter(facets, keyword, value, FACET_KEY_OPTION_FACET|FACET_KEY_OPTION_FTS|FACET_KEY_OPTION_REORDER);
                    buffer_json_add_array_item_string(wb, value);
                    filters++;

                    value = sep;
                }

                buffer_json_array_close(wb); // keyword
            }
        }
    }

    // ------------------------------------------------------------------------
    // put this request into the progress db

    fqs = &tmp_fqs;

    // ------------------------------------------------------------------------
    // validate parameters

    time_t now_s = now_realtime_sec();
    time_t expires = now_s + 1;

    if(!after_s && !before_s) {
        before_s = now_s;
        after_s = before_s - SYSTEMD_JOURNAL_DEFAULT_QUERY_DURATION;
    }
    else
        rrdr_relative_window_to_absolute(&after_s, &before_s, now_s);

    if(after_s > before_s) {
        time_t tmp = after_s;
        after_s = before_s;
        before_s = tmp;
    }

    if(after_s == before_s)
        after_s = before_s - SYSTEMD_JOURNAL_DEFAULT_QUERY_DURATION;

    if(!last)
        last = SYSTEMD_JOURNAL_DEFAULT_ITEMS_PER_QUERY;


    // ------------------------------------------------------------------------
    // set query time-frame, anchors and direction

    fqs->transaction = transaction;
    fqs->after_ut = after_s * USEC_PER_SEC;
    fqs->before_ut = (before_s * USEC_PER_SEC) + USEC_PER_SEC - 1;
    fqs->if_modified_since = if_modified_since;
    fqs->data_only = data_only;
    fqs->delta = (fqs->data_only) ? delta : false;
    fqs->tail = (fqs->data_only && fqs->if_modified_since) ? tail : false;
    fqs->sources = sources;
    fqs->source_type = source_type;
    fqs->entries = last;
    fqs->last_modified = 0;
    fqs->filters = filters;
    fqs->query = (query && *query) ? query : NULL;
    fqs->histogram = (chart && *chart) ? chart : NULL;
    fqs->direction = direction;
    fqs->anchor.start_ut = anchor;
    fqs->anchor.stop_ut = 0;
    fqs->sampling = sampling;

    if(fqs->anchor.start_ut && fqs->tail) {
        // a tail request
        // we need the top X entries from BEFORE
        // but, we need to calculate the facets and the
        // histogram up to the anchor
        fqs->direction = direction = FACETS_ANCHOR_DIRECTION_BACKWARD;
        fqs->anchor.start_ut = 0;
        fqs->anchor.stop_ut = anchor;
    }

    if(anchor && anchor < fqs->after_ut) {
        log_fqs(fqs, "received anchor is too small for query timeframe, ignoring anchor");
        anchor = 0;
        fqs->anchor.start_ut = 0;
        fqs->anchor.stop_ut = 0;
        fqs->direction = direction = FACETS_ANCHOR_DIRECTION_BACKWARD;
    }
    else if(anchor > fqs->before_ut) {
        log_fqs(fqs, "received anchor is too big for query timeframe, ignoring anchor");
        anchor = 0;
        fqs->anchor.start_ut = 0;
        fqs->anchor.stop_ut = 0;
        fqs->direction = direction = FACETS_ANCHOR_DIRECTION_BACKWARD;
    }

    facets_set_anchor(facets, fqs->anchor.start_ut, fqs->anchor.stop_ut, fqs->direction);

    facets_set_additional_options(facets,
                                  ((fqs->data_only) ? FACETS_OPTION_DATA_ONLY : 0) |
                                  ((fqs->delta) ? FACETS_OPTION_SHOW_DELTAS : 0));

    // ------------------------------------------------------------------------
    // set the rest of the query parameters


    facets_set_items(facets, fqs->entries);
    facets_set_query(facets, fqs->query);

#ifdef HAVE_SD_JOURNAL_RESTART_FIELDS
    fqs->slice = slice;
    if(slice)
        facets_enable_slice_mode(facets);
#else
    fqs->slice = false;
#endif

    if(fqs->histogram)
        facets_set_timeframe_and_histogram_by_id(facets, fqs->histogram, fqs->after_ut, fqs->before_ut);
    else
        facets_set_timeframe_and_histogram_by_name(facets, "PRIORITY", fqs->after_ut, fqs->before_ut);


    // ------------------------------------------------------------------------
    // complete the request object

    buffer_json_member_add_boolean(wb, JOURNAL_PARAMETER_INFO, false);
    buffer_json_member_add_boolean(wb, JOURNAL_PARAMETER_SLICE, fqs->slice);
    buffer_json_member_add_boolean(wb, JOURNAL_PARAMETER_DATA_ONLY, fqs->data_only);
    buffer_json_member_add_boolean(wb, JOURNAL_PARAMETER_DELTA, fqs->delta);
    buffer_json_member_add_boolean(wb, JOURNAL_PARAMETER_TAIL, fqs->tail);
    buffer_json_member_add_uint64(wb, JOURNAL_PARAMETER_SAMPLING, fqs->sampling);
    buffer_json_member_add_uint64(wb, "source_type", fqs->source_type);
    buffer_json_member_add_uint64(wb, JOURNAL_PARAMETER_AFTER, fqs->after_ut / USEC_PER_SEC);
    buffer_json_member_add_uint64(wb, JOURNAL_PARAMETER_BEFORE, fqs->before_ut / USEC_PER_SEC);
    buffer_json_member_add_uint64(wb, "if_modified_since", fqs->if_modified_since);
    buffer_json_member_add_uint64(wb, JOURNAL_PARAMETER_ANCHOR, anchor);
    buffer_json_member_add_string(wb, JOURNAL_PARAMETER_DIRECTION, fqs->direction == FACETS_ANCHOR_DIRECTION_FORWARD ? "forward" : "backward");
    buffer_json_member_add_uint64(wb, JOURNAL_PARAMETER_LAST, fqs->entries);
    buffer_json_member_add_string(wb, JOURNAL_PARAMETER_QUERY, fqs->query);
    buffer_json_member_add_string(wb, JOURNAL_PARAMETER_HISTOGRAM, fqs->histogram);
    buffer_json_object_close(wb); // request

    buffer_json_journal_versions(wb);

    // ------------------------------------------------------------------------
    // run the request

    int response;

    if(info) {
        facets_accepted_parameters_to_json_array(facets, wb, false);
        buffer_json_member_add_array(wb, "required_params");
        {
            buffer_json_add_array_item_object(wb);
            {
                buffer_json_member_add_string(wb, "id", "source");
                buffer_json_member_add_string(wb, "name", "source");
                buffer_json_member_add_string(wb, "help", "Select the SystemD Journal source to query");
                buffer_json_member_add_string(wb, "type", "multiselect");
                buffer_json_member_add_array(wb, "options");
                {
                    available_journal_file_sources_to_json_array(wb);
                }
                buffer_json_array_close(wb); // options array
            }
            buffer_json_object_close(wb); // required params object
        }
        buffer_json_array_close(wb); // required_params array

        facets_table_config(wb);

        buffer_json_member_add_uint64(wb, "status", HTTP_RESP_OK);
        buffer_json_member_add_string(wb, "type", "table");
        buffer_json_member_add_string(wb, "help", SYSTEMD_JOURNAL_FUNCTION_DESCRIPTION);
        buffer_json_finalize(wb);
        response = HTTP_RESP_OK;
        goto output;
    }

    response = netdata_systemd_journal_query(wb, facets, fqs);

    // ------------------------------------------------------------------------
    // handle error response

    if(response != HTTP_RESP_OK) {
        netdata_mutex_lock(&stdout_mutex);
        pluginsd_function_json_error_to_stdout(transaction, response, "failed");
        netdata_mutex_unlock(&stdout_mutex);
        goto cleanup;
    }

output:
    netdata_mutex_lock(&stdout_mutex);
    pluginsd_function_result_to_stdout(transaction, response, "application/json", expires, wb);
    netdata_mutex_unlock(&stdout_mutex);

cleanup:
    simple_pattern_free(sources);
    facets_destroy(facets);
    buffer_free(wb);
}
