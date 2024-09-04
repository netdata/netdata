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
    "|ND_ALERT_STATUS"                          \
                                                \
    ""

// ----------------------------------------------------------------------------

typedef struct {
    const char *transaction;

    FACET_KEY_OPTIONS default_facet;

    bool fields_are_ids;
    bool info;
    bool data_only;
    bool slice;
    bool delta;
    bool tail;

    time_t after_s;
    time_t before_s;
    usec_t after_ut;
    usec_t before_ut;

    usec_t anchor;
    usec_t if_modified_since;
    size_t entries;
    FACETS_ANCHOR_DIRECTION direction;

    const char *query;
    const char *histogram;

    SIMPLE_PATTERN *sources;
    SD_JOURNAL_FILE_SOURCE_TYPE source_type;

    size_t filters;
    size_t sampling;

    time_t now_s;
    time_t expires_s;
} LOGS_QUERY_REQUEST;

#define LOGS_QUERY_REQUEST_DEFAULTS(function_transaction, default_slice, default_direction) \
    (LOGS_QUERY_REQUEST) {                                              \
    .transaction = (function_transaction),                              \
    .default_facet = FACET_KEY_OPTION_FACET,                            \
    .info = false,                                                      \
    .data_only = false,                                                 \
    .slice = (default_slice),                                           \
    .delta = false,                                                     \
    .tail = false,                                                      \
    .after_s = 0,                                                       \
    .before_s = 0,                                                      \
    .anchor = 0,                                                        \
    .if_modified_since = 0,                                             \
    .entries = 0,                                                       \
    .direction = (default_direction),                                   \
    .query = NULL,                                                      \
    .histogram = NULL,                                                  \
    .sources = NULL,                                                    \
    .source_type = SDJF_ALL,                                            \
    .filters = 0,                                                       \
    .sampling = SYSTEMD_JOURNAL_DEFAULT_ITEMS_SAMPLING,                 \
}

typedef struct {
    FACETS *facets;

    LOGS_QUERY_REQUEST rq;

    bool *cancelled; // a pointer to the cancelling boolean
    usec_t *stop_monotonic_ut;

    struct {
        usec_t start_ut;
        usec_t stop_ut;
    } anchor;

    usec_t last_modified;

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
} LOGS_QUERY_STATUS;

static void log_fqs(LOGS_QUERY_STATUS *fqs, const char *msg) {
    netdata_log_error("ERROR: %s, on query "
                      "timeframe [%"PRIu64" - %"PRIu64"], "
                      "anchor [%"PRIu64" - %"PRIu64"], "
                      "if_modified_since %"PRIu64", "
                      "data_only:%s, delta:%s, tail:%s, direction:%s"
                      , msg
                      , fqs->rq.after_ut, fqs->rq.before_ut
                      , fqs->anchor.start_ut, fqs->anchor.stop_ut
                      , fqs->rq.if_modified_since
                      , fqs->rq.data_only ? "true" : "false"
                      , fqs->rq.delta ? "true" : "false"
                      , fqs->rq.tail ? "tail" : "false"
                      , fqs->rq.direction == FACETS_ANCHOR_DIRECTION_FORWARD ? "forward" : "backward");
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

static void sampling_query_init(LOGS_QUERY_STATUS *fqs, FACETS *facets) {
    if(!fqs->rq.sampling)
        return;

    if(!fqs->rq.slice) {
        // the user is doing a full data query
        // disable sampling
        fqs->rq.sampling = 0;
        return;
    }

    if(fqs->rq.data_only) {
        // the user is doing a data query
        // disable sampling
        fqs->rq.sampling = 0;
        return;
    }

    if(!fqs->files_matched) {
        // no files have been matched
        // disable sampling
        fqs->rq.sampling = 0;
        return;
    }

    fqs->samples.slots = facets_histogram_slots(facets);
    if(fqs->samples.slots < 2) fqs->samples.slots = 2;
    if(fqs->samples.slots > SYSTEMD_JOURNAL_SAMPLING_SLOTS)
        fqs->samples.slots = SYSTEMD_JOURNAL_SAMPLING_SLOTS;

    if(!fqs->rq.after_ut || !fqs->rq.before_ut || fqs->rq.after_ut >= fqs->rq.before_ut) {
        // we don't have enough information for sampling
        fqs->rq.sampling = 0;
        return;
    }

    usec_t delta = fqs->rq.before_ut - fqs->rq.after_ut;
    usec_t step = delta / facets_histogram_slots(facets) - 1;
    if(step < 1) step = 1;

    fqs->samples_per_time_slot.start_ut = fqs->rq.after_ut;
    fqs->samples_per_time_slot.end_ut = fqs->rq.before_ut;
    fqs->samples_per_time_slot.step_ut = step;

    // the minimum number of rows to enable sampling
    fqs->samples.enable_after_samples = fqs->rq.sampling / 2;

    size_t files_matched = fqs->files_matched;
    if(!files_matched)
        files_matched = 1;

    // the minimum number of rows per file to enable sampling
    fqs->samples_per_file.enable_after_samples = (fqs->rq.sampling / 4) / files_matched;
    if(fqs->samples_per_file.enable_after_samples < fqs->rq.entries)
        fqs->samples_per_file.enable_after_samples = fqs->rq.entries;

    // the minimum number of rows per time slot to enable sampling
    fqs->samples_per_time_slot.enable_after_samples = (fqs->rq.sampling / 4) / fqs->samples.slots;
    if(fqs->samples_per_time_slot.enable_after_samples < fqs->rq.entries)
        fqs->samples_per_time_slot.enable_after_samples = fqs->rq.entries;
}

static void sampling_file_init(LOGS_QUERY_STATUS *fqs, struct journal_file *jf __maybe_unused) {
    fqs->samples_per_file.sampled = 0;
    fqs->samples_per_file.unsampled = 0;
    fqs->samples_per_file.estimated = 0;
    fqs->samples_per_file.every = 0;
    fqs->samples_per_file.skipped = 0;
    fqs->samples_per_file.recalibrate = 0;
}

static size_t sampling_file_lines_scanned_so_far(LOGS_QUERY_STATUS *fqs) {
    size_t sampled = fqs->samples_per_file.sampled + fqs->samples_per_file.unsampled;
    if(!sampled) sampled = 1;
    return sampled;
}

static void sampling_running_file_query_overlapping_timeframe_ut(
    LOGS_QUERY_STATUS *fqs, struct journal_file *jf, FACETS_ANCHOR_DIRECTION direction,
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

static double sampling_running_file_query_progress_by_time(
    LOGS_QUERY_STATUS *fqs, struct journal_file *jf,
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

static usec_t sampling_running_file_query_remaining_time(
    LOGS_QUERY_STATUS *fqs, struct journal_file *jf,
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

static size_t sampling_running_file_query_estimate_remaining_lines_by_time(
    LOGS_QUERY_STATUS *fqs,
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

static size_t sampling_running_file_query_estimate_remaining_lines(sd_journal *j __maybe_unused,
    LOGS_QUERY_STATUS *fqs, struct journal_file *jf, FACETS_ANCHOR_DIRECTION direction, usec_t msg_ut) {
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

static void sampling_decide_file_sampling_every(sd_journal *j,
    LOGS_QUERY_STATUS *fqs, struct journal_file *jf, FACETS_ANCHOR_DIRECTION direction, usec_t msg_ut) {
    size_t files_matched = fqs->files_matched;
    if(!files_matched) files_matched = 1;

    size_t remaining_lines = sampling_running_file_query_estimate_remaining_lines(j, fqs, jf, direction, msg_ut);
    size_t wanted_samples = (fqs->rq.sampling / 2) / files_matched;
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

static inline sampling_t is_row_in_sample(sd_journal *j,
    LOGS_QUERY_STATUS *fqs, struct journal_file *jf, usec_t msg_ut, FACETS_ANCHOR_DIRECTION direction, bool candidate_to_keep) {
    if(!fqs->rq.sampling || candidate_to_keep)
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

static void sampling_update_running_query_file_estimates(FACETS *facets, sd_journal *j,
    LOGS_QUERY_STATUS *fqs, struct journal_file *jf, usec_t msg_ut, FACETS_ANCHOR_DIRECTION direction) {
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
        struct journal_file *jf,
    LOGS_QUERY_STATUS *fqs) {

    usec_t anchor_delta = __atomic_load_n(&jf->max_journal_vs_realtime_delta_ut, __ATOMIC_RELAXED);

    usec_t start_ut = ((fqs->rq.data_only && fqs->anchor.start_ut) ? fqs->anchor.start_ut : fqs->rq.before_ut) + anchor_delta;
    usec_t stop_ut = (fqs->rq.data_only && fqs->anchor.stop_ut) ? fqs->anchor.stop_ut : fqs->rq.after_ut;
    bool stop_when_full = (fqs->rq.data_only && !fqs->anchor.stop_ut);

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
                        facets_rows(facets) >= fqs->rq.entries)) {
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
        struct journal_file *jf,
    LOGS_QUERY_STATUS *fqs) {

    usec_t anchor_delta = __atomic_load_n(&jf->max_journal_vs_realtime_delta_ut, __ATOMIC_RELAXED);

    usec_t start_ut = (fqs->rq.data_only && fqs->anchor.start_ut) ? fqs->anchor.start_ut : fqs->rq.after_ut;
    usec_t stop_ut = ((fqs->rq.data_only && fqs->anchor.stop_ut) ? fqs->anchor.stop_ut : fqs->rq.before_ut) + anchor_delta;
    bool stop_when_full = (fqs->rq.data_only && !fqs->anchor.stop_ut);

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
                        facets_rows(facets) >= fqs->rq.entries)) {
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
static bool netdata_systemd_filtering_by_journal(sd_journal *j, FACETS *facets, LOGS_QUERY_STATUS *fqs) {
    const char *field = NULL;
    const void *data = NULL;
    size_t data_length;
    size_t added_keys = 0;
    size_t failures = 0;
    size_t filters_added = 0;

    SD_JOURNAL_FOREACH_FIELD(j, field) { // for each key
        bool interesting;

        if(fqs->rq.data_only)
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
        struct journal_file *jf,
    LOGS_QUERY_STATUS *fqs) {

    sd_journal *j = NULL;
    errno_clear();

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
    if(fqs->rq.slice) {
        usec_t started = now_monotonic_usec();

        matches_filters = netdata_systemd_filtering_by_journal(j, facets, fqs) || !fqs->rq.filters;
        usec_t ended = now_monotonic_usec();

        fqs->matches_setup_ut += (ended - started);
    }
#endif // HAVE_SD_JOURNAL_RESTART_FIELDS

    if(matches_filters) {
        if(fqs->rq.direction == FACETS_ANCHOR_DIRECTION_FORWARD)
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

static bool jf_is_mine(struct journal_file *jf, LOGS_QUERY_STATUS *fqs) {

    if((fqs->rq.source_type == SDJF_NONE && !fqs->rq.sources) || (jf->source_type & fqs->rq.source_type) ||
       (fqs->rq.sources && simple_pattern_matches(fqs->rq.sources, string2str(jf->source)))) {

        if(!jf->msg_last_ut)
            // the file is not scanned yet, or the timestamps have not been updated,
            // so we don't know if it can contribute or not - let's add it.
            return true;

        usec_t anchor_delta = JOURNAL_VS_REALTIME_DELTA_MAX_UT;
        usec_t first_ut = jf->msg_first_ut - anchor_delta;
        usec_t last_ut = jf->msg_last_ut + anchor_delta;

        if(last_ut >= fqs->rq.after_ut && first_ut <= fqs->rq.before_ut)
            return true;
    }

    return false;
}

static int netdata_systemd_journal_query(BUFFER *wb, LOGS_QUERY_STATUS *lqs) {
    FACETS *facets = lqs->facets;

    ND_SD_JOURNAL_STATUS status = ND_SD_JOURNAL_NO_FILE_MATCHED;
    struct journal_file *jf;

    lqs->files_matched = 0;
    lqs->file_working = 0;
    lqs->rows_useful = 0;
    lqs->rows_read = 0;
    lqs->bytes_read = 0;

    size_t files_used = 0;
    size_t files_max = dictionary_entries(journal_files_registry);
    const DICTIONARY_ITEM *file_items[files_max];

    // count the files
    bool files_are_newer = false;
    dfe_start_read(journal_files_registry, jf) {
        if(!jf_is_mine(jf, lqs))
            continue;

        file_items[files_used++] = dictionary_acquired_item_dup(journal_files_registry, jf_dfe.item);

        if(jf->msg_last_ut > lqs->rq.if_modified_since)
            files_are_newer = true;
    }
    dfe_done(jf);

    lqs->files_matched = files_used;

    if(lqs->rq.if_modified_since && !files_are_newer)
        return rrd_call_function_error(wb, "not modified", HTTP_RESP_NOT_MODIFIED);

    // sort the files, so that they are optimal for facets
    if(files_used >= 2) {
        if (lqs->rq.direction == FACETS_ANCHOR_DIRECTION_BACKWARD)
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

    sampling_query_init(lqs, facets);

    buffer_json_member_add_array(wb, "_journal_files");
    for(size_t f = 0; f < files_used ;f++) {
        const char *filename = dictionary_acquired_item_name(file_items[f]);
        jf = dictionary_acquired_item_value(file_items[f]);

        if(!jf_is_mine(jf, lqs))
            continue;

        started_ut = ended_ut;

        // do not even try to do the query if we expect it to pass the timeout
        if(ended_ut + max_duration_ut * 3 >= *lqs->stop_monotonic_ut) {
            partial = true;
            status = ND_SD_JOURNAL_TIMED_OUT;
            break;
        }

        lqs->file_working++;
        // fqs->cached_count = 0;

        size_t fs_calls = fstat_thread_calls;
        size_t fs_cached = fstat_thread_cached_responses;
        size_t rows_useful = lqs->rows_useful;
        size_t rows_read = lqs->rows_read;
        size_t bytes_read = lqs->bytes_read;
        size_t matches_setup_ut = lqs->matches_setup_ut;

        sampling_file_init(lqs, jf);

        ND_SD_JOURNAL_STATUS tmp_status = netdata_systemd_journal_query_one_file(filename, wb, facets, jf, lqs);

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

        rows_useful = lqs->rows_useful - rows_useful;
        rows_read = lqs->rows_read - rows_read;
        bytes_read = lqs->bytes_read - bytes_read;
        matches_setup_ut = lqs->matches_setup_ut - matches_setup_ut;
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
            pluginsd_function_progress_to_stdout(lqs->rq.transaction, f + 1, files_used);
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

            if(lqs->rq.sampling) {
                buffer_json_member_add_object(wb, "_sampling");
                {
                    buffer_json_member_add_uint64(wb, "sampled", lqs->samples_per_file.sampled);
                    buffer_json_member_add_uint64(wb, "unsampled", lqs->samples_per_file.unsampled);
                    buffer_json_member_add_uint64(wb, "estimated", lqs->samples_per_file.estimated);
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
            if(lqs->rq.if_modified_since && !lqs->rows_useful)
                return rrd_call_function_error(wb, "no useful logs, not modified", HTTP_RESP_NOT_MODIFIED);
            break;

        case ND_SD_JOURNAL_TIMED_OUT:
        case ND_SD_JOURNAL_NO_FILE_MATCHED:
            break;

        case ND_SD_JOURNAL_CANCELLED:
            return rrd_call_function_error(wb, "client closed connection", HTTP_RESP_CLIENT_CLOSED_REQUEST);

        case ND_SD_JOURNAL_NOT_MODIFIED:
            return rrd_call_function_error(wb, "not modified", HTTP_RESP_NOT_MODIFIED);

        case ND_SD_JOURNAL_FAILED_TO_OPEN:
            return rrd_call_function_error(wb, "failed to open journal", HTTP_RESP_INTERNAL_SERVER_ERROR);

        case ND_SD_JOURNAL_FAILED_TO_SEEK:
            return rrd_call_function_error(wb, "failed to seek in journal", HTTP_RESP_INTERNAL_SERVER_ERROR);

        default:
            return rrd_call_function_error(wb, "unknown status", HTTP_RESP_INTERNAL_SERVER_ERROR);
    }

    buffer_json_member_add_uint64(wb, "status", HTTP_RESP_OK);
    buffer_json_member_add_boolean(wb, "partial", partial);
    buffer_json_member_add_string(wb, "type", "table");

    // build a message for the query
    if(!lqs->rq.data_only) {
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

        if(lqs->samples.estimated || lqs->samples.unsampled) {
            double percent = (double) (lqs->samples.sampled * 100.0 /
                                       (lqs->samples.estimated + lqs->samples.unsampled + lqs->samples.sampled));
            buffer_sprintf(msg, "%.2f%% real data", percent);
            buffer_sprintf(msg_description, "ACTUAL DATA: The filters counters reflect %0.2f%% of the data. ", percent);
            msg_priority = MIN(msg_priority, NDLP_NOTICE);
        }

        if(lqs->samples.unsampled) {
            double percent = (double) (lqs->samples.unsampled * 100.0 /
                                       (lqs->samples.estimated + lqs->samples.unsampled + lqs->samples.sampled));
            buffer_sprintf(msg, ", %.2f%% unsampled", percent);
            buffer_sprintf(msg_description
                           , "UNSAMPLED DATA: %0.2f%% of the events exist and have been counted, but their values have not been evaluated, so they are not included in the filters counters. "
                           , percent);
            msg_priority = MIN(msg_priority, NDLP_NOTICE);
        }

        if(lqs->samples.estimated) {
            double percent = (double) (lqs->samples.estimated * 100.0 /
                                       (lqs->samples.estimated + lqs->samples.unsampled + lqs->samples.sampled));
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

    if(!lqs->rq.data_only) {
        buffer_json_member_add_time_t(wb, "update_every", 1);
        buffer_json_member_add_string(wb, "help", SYSTEMD_JOURNAL_FUNCTION_DESCRIPTION);
    }

    if(!lqs->rq.data_only || lqs->rq.tail)
        buffer_json_member_add_uint64(wb, "last_modified", lqs->last_modified);

    facets_sort_and_reorder_keys(facets);
    facets_report(facets, wb, used_hashes_registry);

    buffer_json_member_add_time_t(wb, "expires", now_realtime_sec() + (lqs->rq.data_only ? 3600 : 0));

    buffer_json_member_add_object(wb, "_fstat_caching");
    {
        buffer_json_member_add_uint64(wb, "calls", fstat_thread_calls);
        buffer_json_member_add_uint64(wb, "cached", fstat_thread_cached_responses);
    }
    buffer_json_object_close(wb); // _fstat_caching

    if(lqs->rq.sampling) {
        buffer_json_member_add_object(wb, "_sampling");
        {
            buffer_json_member_add_uint64(wb, "sampled", lqs->samples.sampled);
            buffer_json_member_add_uint64(wb, "unsampled", lqs->samples.unsampled);
            buffer_json_member_add_uint64(wb, "estimated", lqs->samples.estimated);
        }
        buffer_json_object_close(wb); // _sampling
    }

    buffer_json_finalize(wb);

    wb->content_type = CT_APPLICATION_JSON;
    wb->response_code = HTTP_RESP_OK;
    return wb->response_code;
}

static void logs_function_help(BUFFER *wb) {
    buffer_reset(wb);
    wb->content_type = CT_TEXT_PLAIN;
    wb->response_code = HTTP_RESP_OK;

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
}

static SD_JOURNAL_FILE_SOURCE_TYPE get_internal_source_type(const char *value) {
    if(strcmp(value, SDJF_SOURCE_ALL_NAME) == 0)
        return SDJF_ALL;
    else if(strcmp(value, SDJF_SOURCE_LOCAL_NAME) == 0)
        return SDJF_LOCAL_ALL;
    else if(strcmp(value, SDJF_SOURCE_REMOTES_NAME) == 0)
        return SDJF_REMOTE_ALL;
    else if(strcmp(value, SDJF_SOURCE_NAMESPACES_NAME) == 0)
        return SDJF_LOCAL_NAMESPACE;
    else if(strcmp(value, SDJF_SOURCE_LOCAL_SYSTEM_NAME) == 0)
        return SDJF_LOCAL_SYSTEM;
    else if(strcmp(value, SDJF_SOURCE_LOCAL_USERS_NAME) == 0)
        return SDJF_LOCAL_USER;
    else if(strcmp(value, SDJF_SOURCE_LOCAL_OTHER_NAME) == 0)
        return SDJF_LOCAL_OTHER;

    return SDJF_NONE;
}

static FACETS_ANCHOR_DIRECTION get_direction(const char *value) {
    return strcasecmp(value, "forward") == 0 ? FACETS_ANCHOR_DIRECTION_FORWARD : FACETS_ANCHOR_DIRECTION_BACKWARD;
}

struct logs_query_data {
    const char *transaction;
    FACETS *facets;
    LOGS_QUERY_REQUEST *q;
    BUFFER *wb;
};

static bool lqs_request_parse_json_payload(json_object *jobj, const char *path, void *data, BUFFER *error) {
    struct logs_query_data *qd = data;
    LOGS_QUERY_REQUEST *q = qd->q;
    BUFFER *wb = qd->wb;
    FACETS *facets = qd->facets;
    // const char *transaction = qd->transaction;

    buffer_flush(error);

    JSONC_PARSE_BOOL_OR_ERROR_AND_RETURN(jobj, path, JOURNAL_PARAMETER_INFO, q->info, error, false);
    JSONC_PARSE_BOOL_OR_ERROR_AND_RETURN(jobj, path, JOURNAL_PARAMETER_DELTA, q->delta, error, false);
    JSONC_PARSE_BOOL_OR_ERROR_AND_RETURN(jobj, path, JOURNAL_PARAMETER_TAIL, q->tail, error, false);
    JSONC_PARSE_BOOL_OR_ERROR_AND_RETURN(jobj, path, JOURNAL_PARAMETER_SLICE, q->slice, error, false);
    JSONC_PARSE_BOOL_OR_ERROR_AND_RETURN(jobj, path, JOURNAL_PARAMETER_DATA_ONLY, q->data_only, error, false);
    JSONC_PARSE_UINT64_OR_ERROR_AND_RETURN(jobj, path, JOURNAL_PARAMETER_SAMPLING, q->sampling, error, false);
    JSONC_PARSE_INT64_OR_ERROR_AND_RETURN(jobj, path, JOURNAL_PARAMETER_AFTER, q->after_s, error, false);
    JSONC_PARSE_INT64_OR_ERROR_AND_RETURN(jobj, path, JOURNAL_PARAMETER_BEFORE, q->before_s, error, false);
    JSONC_PARSE_UINT64_OR_ERROR_AND_RETURN(jobj, path, JOURNAL_PARAMETER_IF_MODIFIED_SINCE, q->if_modified_since, error, false);
    JSONC_PARSE_UINT64_OR_ERROR_AND_RETURN(jobj, path, JOURNAL_PARAMETER_ANCHOR, q->anchor, error, false);
    JSONC_PARSE_UINT64_OR_ERROR_AND_RETURN(jobj, path, JOURNAL_PARAMETER_LAST, q->entries, error, false);
    JSONC_PARSE_TXT2ENUM_OR_ERROR_AND_RETURN(jobj, path, JOURNAL_PARAMETER_DIRECTION, get_direction, q->direction, error, false);
    JSONC_PARSE_TXT2STRDUPZ_OR_ERROR_AND_RETURN(jobj, path, JOURNAL_PARAMETER_QUERY, q->query, error, false);
    JSONC_PARSE_TXT2STRDUPZ_OR_ERROR_AND_RETURN(jobj, path, JOURNAL_PARAMETER_HISTOGRAM, q->histogram, error, false);

    json_object *sources;
    if (json_object_object_get_ex(jobj, JOURNAL_PARAMETER_SOURCE, &sources)) {
        if (json_object_get_type(sources) != json_type_array) {
            buffer_sprintf(error, "member '%s' is not an array", JOURNAL_PARAMETER_SOURCE);
            return false;
        }

        buffer_json_member_add_array(wb, JOURNAL_PARAMETER_SOURCE);

        CLEAN_BUFFER *sources_list = buffer_create(0, NULL);

        q->source_type = SDJF_NONE;

        size_t sources_len = json_object_array_length(sources);
        for (size_t i = 0; i < sources_len; i++) {
            json_object *src = json_object_array_get_idx(sources, i);

            if (json_object_get_type(src) != json_type_string) {
                buffer_sprintf(error, "sources array item %zu is not a string", i);
                return false;
            }

            const char *value = json_object_get_string(src);
            buffer_json_add_array_item_string(wb, value);

            SD_JOURNAL_FILE_SOURCE_TYPE t = get_internal_source_type(value);
            if(t != SDJF_NONE) {
                q->source_type |= t;
                value = NULL;
            }
            else {
                // else, match the source, whatever it is
                if(buffer_strlen(sources_list))
                    buffer_putc(sources_list, '|');

                buffer_strcat(sources_list, value);
            }
        }

        if(buffer_strlen(sources_list)) {
            simple_pattern_free(q->sources);
            q->sources = simple_pattern_create(buffer_tostring(sources_list), "|", SIMPLE_PATTERN_EXACT, false);
        }

        buffer_json_array_close(wb); // source
    }

    json_object *fcts;
    if (json_object_object_get_ex(jobj, JOURNAL_PARAMETER_FACETS, &fcts)) {
        if (json_object_get_type(sources) != json_type_array) {
            buffer_sprintf(error, "member '%s' is not an array", JOURNAL_PARAMETER_FACETS);
            return false;
        }

        q->default_facet = FACET_KEY_OPTION_NONE;
        facets_reset_and_disable_all_facets(facets);

        buffer_json_member_add_array(wb, JOURNAL_PARAMETER_FACETS);

        size_t facets_len = json_object_array_length(fcts);
        for (size_t i = 0; i < facets_len; i++) {
            json_object *fct = json_object_array_get_idx(fcts, i);

            if (json_object_get_type(fct) != json_type_string) {
                buffer_sprintf(error, "facets array item %zu is not a string", i);
                return false;
            }

            const char *value = json_object_get_string(fct);
            facets_register_facet(facets, value, FACET_KEY_OPTION_FACET|FACET_KEY_OPTION_FTS|FACET_KEY_OPTION_REORDER);
            buffer_json_add_array_item_string(wb, value);
        }

        buffer_json_array_close(wb); // facets
    }

    json_object *selections;
    if (json_object_object_get_ex(jobj, "selections", &selections)) {
        if (json_object_get_type(selections) != json_type_object) {
            buffer_sprintf(error, "member 'selections' is not an object");
            return false;
        }

        buffer_json_member_add_object(wb, "selections");

        json_object_object_foreach(selections, key, val) {
            if (json_object_get_type(val) != json_type_array) {
                buffer_sprintf(error, "selection '%s' is not an array", key);
                return false;
            }

            buffer_json_member_add_array(wb, key);

            size_t values_len = json_object_array_length(val);
            for (size_t i = 0; i < values_len; i++) {
                json_object *value_obj = json_object_array_get_idx(val, i);

                if (json_object_get_type(value_obj) != json_type_string) {
                    buffer_sprintf(error, "selection '%s' array item %zu is not a string", key, i);
                    return false;
                }

                const char *value = json_object_get_string(value_obj);

                // Call facets_register_facet_id_filter for each value
                facets_register_facet_filter(
                    facets, key, value, FACET_KEY_OPTION_FACET | FACET_KEY_OPTION_FTS | FACET_KEY_OPTION_REORDER);

                buffer_json_add_array_item_string(wb, value);
                q->filters++;
            }

            buffer_json_array_close(wb); // key
        }

        buffer_json_object_close(wb); // selections
    }

    q->fields_are_ids = false;
    return true;
}

static bool lqs_request_parse_POST(LOGS_QUERY_STATUS *lqs, BUFFER *wb, BUFFER *payload, const char *transaction) {
    FACETS *facets = lqs->facets;
    LOGS_QUERY_REQUEST *rq = &lqs->rq;

    struct logs_query_data qd = {
        .transaction = transaction,
        .facets = facets,
        .q = rq,
        .wb = wb,
    };

    int code;
    CLEAN_JSON_OBJECT *jobj =
        json_parse_function_payload_or_error(wb, payload, &code, lqs_request_parse_json_payload, &qd);
    wb->response_code = code;

    return (jobj && code == HTTP_RESP_OK);
}

static bool lqs_request_parse_GET(LOGS_QUERY_STATUS *lqs, BUFFER *wb, char *function) {
    FACETS *facets = lqs->facets;
    LOGS_QUERY_REQUEST *rq = &lqs->rq;

    buffer_json_member_add_object(wb, "_request");

    char *words[SYSTEMD_JOURNAL_MAX_PARAMS] = { NULL };
    size_t num_words = quoted_strings_splitter_pluginsd(function, words, SYSTEMD_JOURNAL_MAX_PARAMS);
    for(int i = 1; i < SYSTEMD_JOURNAL_MAX_PARAMS ;i++) {
        char *keyword = get_word(words, num_words, i);
        if(!keyword) break;

        if(strcmp(keyword, JOURNAL_PARAMETER_HELP) == 0) {
            logs_function_help(wb);
            return false;
        }
        else if(strcmp(keyword, JOURNAL_PARAMETER_INFO) == 0) {
            rq->info = true;
        }
        else if(strncmp(keyword, JOURNAL_PARAMETER_DELTA ":", sizeof(JOURNAL_PARAMETER_DELTA ":") - 1) == 0) {
            char *v = &keyword[sizeof(JOURNAL_PARAMETER_DELTA ":") - 1];

            if(strcmp(v, "false") == 0 || strcmp(v, "no") == 0 || strcmp(v, "0") == 0)
                rq->delta = false;
            else
                rq->delta = true;
        }
        else if(strncmp(keyword, JOURNAL_PARAMETER_TAIL ":", sizeof(JOURNAL_PARAMETER_TAIL ":") - 1) == 0) {
            char *v = &keyword[sizeof(JOURNAL_PARAMETER_TAIL ":") - 1];

            if(strcmp(v, "false") == 0 || strcmp(v, "no") == 0 || strcmp(v, "0") == 0)
                rq->tail = false;
            else
                rq->tail = true;
        }
        else if(strncmp(keyword, JOURNAL_PARAMETER_SAMPLING ":", sizeof(JOURNAL_PARAMETER_SAMPLING ":") - 1) == 0) {
            rq->sampling = str2ul(&keyword[sizeof(JOURNAL_PARAMETER_SAMPLING ":") - 1]);
        }
        else if(strncmp(keyword, JOURNAL_PARAMETER_DATA_ONLY ":", sizeof(JOURNAL_PARAMETER_DATA_ONLY ":") - 1) == 0) {
            char *v = &keyword[sizeof(JOURNAL_PARAMETER_DATA_ONLY ":") - 1];

            if(strcmp(v, "false") == 0 || strcmp(v, "no") == 0 || strcmp(v, "0") == 0)
                rq->data_only = false;
            else
                rq->data_only = true;
        }
        else if(strncmp(keyword, JOURNAL_PARAMETER_SLICE ":", sizeof(JOURNAL_PARAMETER_SLICE ":") - 1) == 0) {
            char *v = &keyword[sizeof(JOURNAL_PARAMETER_SLICE ":") - 1];

            if(strcmp(v, "false") == 0 || strcmp(v, "no") == 0 || strcmp(v, "0") == 0)
                rq->slice = false;
            else
                rq->slice = true;
        }
        else if(strncmp(keyword, JOURNAL_PARAMETER_SOURCE ":", sizeof(JOURNAL_PARAMETER_SOURCE ":") - 1) == 0) {
            const char *value = &keyword[sizeof(JOURNAL_PARAMETER_SOURCE ":") - 1];

            buffer_json_member_add_array(wb, JOURNAL_PARAMETER_SOURCE);

            CLEAN_BUFFER *sources_list = buffer_create(0, NULL);

            rq->source_type = SDJF_NONE;
            while(value) {
                char *sep = strchr(value, ',');
                if(sep)
                    *sep++ = '\0';

                buffer_json_add_array_item_string(wb, value);

                SD_JOURNAL_FILE_SOURCE_TYPE t = get_internal_source_type(value);
                if(t != SDJF_NONE) {
                    rq->source_type |= t;
                    value = NULL;
                }
                else {
                    // else, match the source, whatever it is
                    if(buffer_strlen(sources_list))
                        buffer_putc(sources_list, '|');

                    buffer_strcat(sources_list, value);
                }

                value = sep;
            }

            if(buffer_strlen(sources_list)) {
                simple_pattern_free(rq->sources);
                rq->sources = simple_pattern_create(buffer_tostring(sources_list), "|", SIMPLE_PATTERN_EXACT, false);
            }

            buffer_json_array_close(wb); // source
        }
        else if(strncmp(keyword, JOURNAL_PARAMETER_AFTER ":", sizeof(JOURNAL_PARAMETER_AFTER ":") - 1) == 0) {
            rq->after_s = str2l(&keyword[sizeof(JOURNAL_PARAMETER_AFTER ":") - 1]);
        }
        else if(strncmp(keyword, JOURNAL_PARAMETER_BEFORE ":", sizeof(JOURNAL_PARAMETER_BEFORE ":") - 1) == 0) {
            rq->before_s = str2l(&keyword[sizeof(JOURNAL_PARAMETER_BEFORE ":") - 1]);
        }
        else if(strncmp(keyword, JOURNAL_PARAMETER_IF_MODIFIED_SINCE ":", sizeof(JOURNAL_PARAMETER_IF_MODIFIED_SINCE ":") - 1) == 0) {
            rq->if_modified_since = str2ull(&keyword[sizeof(JOURNAL_PARAMETER_IF_MODIFIED_SINCE ":") - 1], NULL);
        }
        else if(strncmp(keyword, JOURNAL_PARAMETER_ANCHOR ":", sizeof(JOURNAL_PARAMETER_ANCHOR ":") - 1) == 0) {
            rq->anchor = str2ull(&keyword[sizeof(JOURNAL_PARAMETER_ANCHOR ":") - 1], NULL);
        }
        else if(strncmp(keyword, JOURNAL_PARAMETER_DIRECTION ":", sizeof(JOURNAL_PARAMETER_DIRECTION ":") - 1) == 0) {
            rq->direction = get_direction(&keyword[sizeof(JOURNAL_PARAMETER_DIRECTION ":") - 1]);
        }
        else if(strncmp(keyword, JOURNAL_PARAMETER_LAST ":", sizeof(JOURNAL_PARAMETER_LAST ":") - 1) == 0) {
            rq->entries = str2ul(&keyword[sizeof(JOURNAL_PARAMETER_LAST ":") - 1]);
        }
        else if(strncmp(keyword, JOURNAL_PARAMETER_QUERY ":", sizeof(JOURNAL_PARAMETER_QUERY ":") - 1) == 0) {
            freez((void *)rq->query);
            rq->query= strdupz(&keyword[sizeof(JOURNAL_PARAMETER_QUERY ":") - 1]);
        }
        else if(strncmp(keyword, JOURNAL_PARAMETER_HISTOGRAM ":", sizeof(JOURNAL_PARAMETER_HISTOGRAM ":") - 1) == 0) {
            freez((void *)rq->histogram);
            rq->histogram = strdupz(&keyword[sizeof(JOURNAL_PARAMETER_HISTOGRAM ":") - 1]);
        }
        else if(strncmp(keyword, JOURNAL_PARAMETER_FACETS ":", sizeof(JOURNAL_PARAMETER_FACETS ":") - 1) == 0) {
            rq->default_facet = FACET_KEY_OPTION_NONE;
            facets_reset_and_disable_all_facets(facets);

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

                    facets_register_facet_filter_id(
                        facets, keyword, value,
                        FACET_KEY_OPTION_FACET | FACET_KEY_OPTION_FTS | FACET_KEY_OPTION_REORDER);

                    buffer_json_add_array_item_string(wb, value);
                    rq->filters++;

                    value = sep;
                }

                buffer_json_array_close(wb); // keyword
            }
        }
    }

    rq->fields_are_ids = true;
    return true;
}

static void lqs_info_response(BUFFER *wb, FACETS *facets) {
    // the buffer already has the request in it
    // DO NOT FLUSH IT

    buffer_json_member_add_uint64(wb, "v", 3);
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

    wb->content_type = CT_APPLICATION_JSON;
    wb->response_code = HTTP_RESP_OK;
}

static BUFFER *lqs_create_output_buffer(void) {
    BUFFER *wb = buffer_create(0, NULL);
    buffer_flush(wb);
    buffer_json_initialize(wb, "\"", "\"", 0, true, BUFFER_JSON_OPTIONS_MINIFY);
    return wb;
}

static FACETS *lqs_facets_create(uint32_t items_to_return, FACETS_OPTIONS options, const char *visible_keys, const char *facet_keys, const char *non_facet_keys, bool have_slice) {
    FACETS *facets = facets_create(items_to_return, options,
                                   visible_keys, facet_keys, non_facet_keys);

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

    if(have_slice)
        facets_accepted_param(facets, JOURNAL_PARAMETER_SLICE);

    return facets;
}

static bool lqs_request_parse_and_validate(LOGS_QUERY_STATUS *lqs, BUFFER *wb, char *function, BUFFER *payload, bool have_slice, const char *default_histogram) {
    LOGS_QUERY_REQUEST *rq = &lqs->rq;
    FACETS *facets = lqs->facets;

    if( (payload && !lqs_request_parse_POST(lqs, wb, payload, rq->transaction)) ||
        (!payload && !lqs_request_parse_GET(lqs, wb, function)) )
        return false;

    // ----------------------------------------------------------------------------------------------------------------
    // validate parameters

    if(rq->query && !*rq->query) {
        freez((void *)rq->query);
        rq->query = NULL;
    }

    if(rq->histogram && !*rq->histogram) {
        freez((void *)rq->histogram);
        rq->histogram = NULL;
    }

    if(!rq->data_only)
        rq->delta = false;

    if(!rq->data_only || !rq->if_modified_since)
        rq->tail = false;

    rq->now_s = now_realtime_sec();
    rq->expires_s = rq->now_s + 1;
    wb->expires = rq->expires_s;

    if(!rq->after_s && !rq->before_s) {
        rq->before_s = rq->now_s;
        rq->after_s = rq->before_s - SYSTEMD_JOURNAL_DEFAULT_QUERY_DURATION;
    }
    else
        rrdr_relative_window_to_absolute(&rq->after_s, &rq->before_s, rq->now_s);

    if(rq->after_s > rq->before_s) {
        time_t tmp = rq->after_s;
        rq->after_s = rq->before_s;
        rq->before_s = tmp;
    }

    if(rq->after_s == rq->before_s)
        rq->after_s = rq->before_s - SYSTEMD_JOURNAL_DEFAULT_QUERY_DURATION;

    rq->after_ut = rq->after_s * USEC_PER_SEC;
    rq->before_ut = (rq->before_s * USEC_PER_SEC) + USEC_PER_SEC - 1;

    if(!rq->entries)
        rq->entries = SYSTEMD_JOURNAL_DEFAULT_ITEMS_PER_QUERY;

    // ----------------------------------------------------------------------------------------------------------------
    // validate the anchor

    lqs->last_modified = 0;
    lqs->anchor.start_ut = lqs->rq.anchor;
    lqs->anchor.stop_ut = 0;

    if(lqs->anchor.start_ut && lqs->rq.tail) {
        // a tail request
        // we need the top X entries from BEFORE
        // but, we need to calculate the facets and the
        // histogram up to the anchor
        lqs->rq.direction = FACETS_ANCHOR_DIRECTION_BACKWARD;
        lqs->anchor.start_ut = 0;
        lqs->anchor.stop_ut = lqs->rq.anchor;
    }

    if(lqs->rq.anchor && lqs->rq.anchor < lqs->rq.after_ut) {
        log_fqs(lqs, "received anchor is too small for query timeframe, ignoring anchor");
        lqs->rq.anchor = 0;
        lqs->anchor.start_ut = 0;
        lqs->anchor.stop_ut = 0;
        lqs->rq.direction = FACETS_ANCHOR_DIRECTION_BACKWARD;
    }
    else if(lqs->rq.anchor > lqs->rq.before_ut) {
        log_fqs(lqs, "received anchor is too big for query timeframe, ignoring anchor");
        lqs->rq.anchor = 0;
        lqs->anchor.start_ut = 0;
        lqs->anchor.stop_ut = 0;
        lqs->rq.direction = FACETS_ANCHOR_DIRECTION_BACKWARD;
    }

    facets_set_anchor(facets, lqs->anchor.start_ut, lqs->anchor.stop_ut, lqs->rq.direction);

    facets_set_additional_options(facets,
                                  ((lqs->rq.data_only) ? FACETS_OPTION_DATA_ONLY : 0) |
                                      ((lqs->rq.delta) ? FACETS_OPTION_SHOW_DELTAS : 0));

    facets_set_items(facets, lqs->rq.entries);
    facets_set_query(facets, lqs->rq.query);

    if(lqs->rq.slice && have_slice)
        facets_enable_slice_mode(facets);
    else
        lqs->rq.slice = false;

    if(lqs->rq.histogram) {
        if(lqs-rq->fields_are_ids)
            facets_set_timeframe_and_histogram_by_id(facets, lqs->rq.histogram, lqs->rq.after_ut, lqs->rq.before_ut);
        else
            facets_set_timeframe_and_histogram_by_name(facets, lqs->rq.histogram, lqs->rq.after_ut, lqs->rq.before_ut);
    }
    else if(default_histogram)
        facets_set_timeframe_and_histogram_by_name(facets, default_histogram, lqs->rq.after_ut, lqs->rq.before_ut);

    // complete the request object
    buffer_json_member_add_boolean(wb, JOURNAL_PARAMETER_INFO, false);
    buffer_json_member_add_boolean(wb, JOURNAL_PARAMETER_SLICE, lqs->rq.slice);
    buffer_json_member_add_boolean(wb, JOURNAL_PARAMETER_DATA_ONLY, lqs->rq.data_only);
    buffer_json_member_add_boolean(wb, JOURNAL_PARAMETER_DELTA, lqs->rq.delta);
    buffer_json_member_add_boolean(wb, JOURNAL_PARAMETER_TAIL, lqs->rq.tail);
    buffer_json_member_add_uint64(wb, JOURNAL_PARAMETER_SAMPLING, lqs->rq.sampling);
    buffer_json_member_add_uint64(wb, "source_type", lqs->rq.source_type);
    buffer_json_member_add_uint64(wb, JOURNAL_PARAMETER_AFTER, lqs->rq.after_ut / USEC_PER_SEC);
    buffer_json_member_add_uint64(wb, JOURNAL_PARAMETER_BEFORE, lqs->rq.before_ut / USEC_PER_SEC);
    buffer_json_member_add_uint64(wb, "if_modified_since", lqs->rq.if_modified_since);
    buffer_json_member_add_uint64(wb, JOURNAL_PARAMETER_ANCHOR, lqs->rq.anchor);
    buffer_json_member_add_string(wb, JOURNAL_PARAMETER_DIRECTION, lqs->rq.direction == FACETS_ANCHOR_DIRECTION_FORWARD ? "forward" : "backward");
    buffer_json_member_add_uint64(wb, JOURNAL_PARAMETER_LAST, lqs->rq.entries);
    buffer_json_member_add_string(wb, JOURNAL_PARAMETER_QUERY, lqs->rq.query);
    buffer_json_member_add_string(wb, JOURNAL_PARAMETER_HISTOGRAM, lqs->rq.histogram);
    buffer_json_object_close(wb); // request

    return true;
}

static void systemd_journal_register_transformations(LOGS_QUERY_STATUS *lqs) {
    FACETS *facets = lqs->facets;
    LOGS_QUERY_REQUEST *rq = &lqs->rq;

    // ----------------------------------------------------------------------------------------------------------------
    // register the fields in the order you want them on the dashboard

    facets_register_row_severity(facets, syslog_priority_to_facet_severity, NULL);

    facets_register_key_name(
        facets, "_HOSTNAME", rq->default_facet | FACET_KEY_OPTION_VISIBLE);

    facets_register_dynamic_key_name(
        facets, JOURNAL_KEY_ND_JOURNAL_PROCESS,
        FACET_KEY_OPTION_NEVER_FACET | FACET_KEY_OPTION_VISIBLE,
        netdata_systemd_journal_dynamic_row_id, NULL);

    facets_register_key_name(
        facets, "MESSAGE",
        FACET_KEY_OPTION_NEVER_FACET | FACET_KEY_OPTION_MAIN_TEXT |
            FACET_KEY_OPTION_VISIBLE | FACET_KEY_OPTION_FTS);

    //    facets_register_dynamic_key_name(
    //        facets, "MESSAGE",
    //        FACET_KEY_OPTION_NEVER_FACET | FACET_KEY_OPTION_MAIN_TEXT | FACET_KEY_OPTION_RICH_TEXT |
    //            FACET_KEY_OPTION_VISIBLE | FACET_KEY_OPTION_FTS,
    //        netdata_systemd_journal_rich_message, NULL);

    facets_register_key_name_transformation(
        facets, "PRIORITY",
        rq->default_facet | FACET_KEY_OPTION_TRANSFORM_VIEW |
            FACET_KEY_OPTION_EXPANDED_FILTER,
        netdata_systemd_journal_transform_priority, NULL);

    facets_register_key_name_transformation(
        facets, "SYSLOG_FACILITY",
        rq->default_facet | FACET_KEY_OPTION_TRANSFORM_VIEW |
            FACET_KEY_OPTION_EXPANDED_FILTER,
        netdata_systemd_journal_transform_syslog_facility, NULL);

    facets_register_key_name_transformation(
        facets, "ERRNO",
        rq->default_facet | FACET_KEY_OPTION_TRANSFORM_VIEW,
        netdata_systemd_journal_transform_errno, NULL);

    facets_register_key_name(
        facets, JOURNAL_KEY_ND_JOURNAL_FILE,
        FACET_KEY_OPTION_NEVER_FACET);

    facets_register_key_name(
        facets, "SYSLOG_IDENTIFIER", rq->default_facet);

    facets_register_key_name(
        facets, "UNIT", rq->default_facet);

    facets_register_key_name(
        facets, "USER_UNIT", rq->default_facet);

    facets_register_key_name_transformation(
        facets, "MESSAGE_ID",
        rq->default_facet | FACET_KEY_OPTION_TRANSFORM_VIEW |
            FACET_KEY_OPTION_EXPANDED_FILTER,
        netdata_systemd_journal_transform_message_id, NULL);

    facets_register_key_name_transformation(
        facets, "_BOOT_ID",
        rq->default_facet | FACET_KEY_OPTION_TRANSFORM_VIEW,
        netdata_systemd_journal_transform_boot_id, NULL);

    facets_register_key_name_transformation(
        facets, "_SYSTEMD_OWNER_UID",
        rq->default_facet | FACET_KEY_OPTION_TRANSFORM_VIEW,
        netdata_systemd_journal_transform_uid, NULL);

    facets_register_key_name_transformation(
        facets, "_UID",
        rq->default_facet | FACET_KEY_OPTION_TRANSFORM_VIEW,
        netdata_systemd_journal_transform_uid, NULL);

    facets_register_key_name_transformation(
        facets, "OBJECT_SYSTEMD_OWNER_UID",
        rq->default_facet | FACET_KEY_OPTION_TRANSFORM_VIEW,
        netdata_systemd_journal_transform_uid, NULL);

    facets_register_key_name_transformation(
        facets, "OBJECT_UID",
        rq->default_facet | FACET_KEY_OPTION_TRANSFORM_VIEW,
        netdata_systemd_journal_transform_uid, NULL);

    facets_register_key_name_transformation(
        facets, "_GID",
        rq->default_facet | FACET_KEY_OPTION_TRANSFORM_VIEW,
        netdata_systemd_journal_transform_gid, NULL);

    facets_register_key_name_transformation(
        facets, "OBJECT_GID",
        rq->default_facet | FACET_KEY_OPTION_TRANSFORM_VIEW,
        netdata_systemd_journal_transform_gid, NULL);

    facets_register_key_name_transformation(
        facets, "_CAP_EFFECTIVE",
        FACET_KEY_OPTION_TRANSFORM_VIEW,
        netdata_systemd_journal_transform_cap_effective, NULL);

    facets_register_key_name_transformation(
        facets, "_AUDIT_LOGINUID",
        FACET_KEY_OPTION_TRANSFORM_VIEW,
        netdata_systemd_journal_transform_uid, NULL);

    facets_register_key_name_transformation(
        facets, "OBJECT_AUDIT_LOGINUID",
        FACET_KEY_OPTION_TRANSFORM_VIEW,
        netdata_systemd_journal_transform_uid, NULL);

    facets_register_key_name_transformation(
        facets, "_SOURCE_REALTIME_TIMESTAMP",
        FACET_KEY_OPTION_TRANSFORM_VIEW,
        netdata_systemd_journal_transform_timestamp_usec, NULL);
}

void function_systemd_journal(const char *transaction, char *function, usec_t *stop_monotonic_ut, bool *cancelled,
                              BUFFER *payload, HTTP_ACCESS access __maybe_unused,
                              const char *source __maybe_unused, void *data __maybe_unused) {
    fstat_thread_calls = 0;
    fstat_thread_cached_responses = 0;

#ifdef HAVE_SD_JOURNAL_RESTART_FIELDS
    bool have_slice = true;
#else
    bool have_slice = false;
#endif // HAVE_SD_JOURNAL_RESTART_FIELDS

    LOGS_QUERY_STATUS tmp_fqs = {
        .facets = lqs_facets_create(
            SYSTEMD_JOURNAL_DEFAULT_ITEMS_PER_QUERY,
            FACETS_OPTION_ALL_KEYS_FTS,
            SYSTEMD_ALWAYS_VISIBLE_KEYS,
            SYSTEMD_KEYS_INCLUDED_IN_FACETS,
            SYSTEMD_KEYS_EXCLUDED_FROM_FACETS,
            have_slice),

        .rq = LOGS_QUERY_REQUEST_DEFAULTS(transaction, JOURNAL_DEFAULT_SLICE_MODE, JOURNAL_DEFAULT_DIRECTION),

        .cancelled = cancelled,
        .stop_monotonic_ut = stop_monotonic_ut,
    };
    LOGS_QUERY_STATUS *lqs = &tmp_fqs;

    CLEAN_BUFFER *wb = lqs_create_output_buffer();

    // ------------------------------------------------------------------------
    // parse the parameters

    if(!lqs_request_parse_and_validate(lqs, wb, function, payload, have_slice, "PRIORITY"))
        goto output;

    systemd_journal_register_transformations(lqs);

    // ------------------------------------------------------------------------
    // add versions to the response

    buffer_json_journal_versions(wb);

    // ------------------------------------------------------------------------
    // run the request

    if(lqs->rq.info) {
        lqs_info_response(wb, lqs->facets);
        goto output;
    }

    netdata_systemd_journal_query(wb, lqs);

output:
    netdata_mutex_lock(&stdout_mutex);
    pluginsd_function_result_to_stdout(transaction, wb);
    netdata_mutex_unlock(&stdout_mutex);

    freez((void *)lqs->rq.query);
    freez((void *)lqs->rq.histogram);
    simple_pattern_free(lqs->rq.sources);
    facets_destroy(lqs->facets);
}
