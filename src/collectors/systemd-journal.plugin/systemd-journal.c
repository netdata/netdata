// SPDX-License-Identifier: GPL-3.0-or-later

/*
 * TODO
 * _UDEV_DEVLINK is frequently set more than once per field - support multi-value faces
 *
 */

#include "systemd-internals.h"

#define SYSTEMD_JOURNAL_FUNCTION_DESCRIPTION    "View, search and analyze systemd journal entries."
#define SYSTEMD_JOURNAL_FUNCTION_NAME           "systemd-journal"
#define SYSTEMD_JOURNAL_SAMPLING_SLOTS 1000
#define SYSTEMD_JOURNAL_SAMPLING_RECALIBRATE 10000

#ifdef HAVE_SD_JOURNAL_RESTART_FIELDS
#define LQS_DEFAULT_SLICE_MODE 1
#else
#define LQS_DEFAULT_SLICE_MODE 0
#endif

// functions needed by LQS
static SD_JOURNAL_FILE_SOURCE_TYPE get_internal_source_type(const char *value);

// structures needed by LQS
struct lqs_extension {
    struct {
        usec_t start_ut;
        usec_t stop_ut;
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
};

// prepare LQS
#define LQS_FUNCTION_NAME           SYSTEMD_JOURNAL_FUNCTION_NAME
#define LQS_FUNCTION_DESCRIPTION    SYSTEMD_JOURNAL_FUNCTION_DESCRIPTION
#define LQS_DEFAULT_ITEMS_PER_QUERY 200
#define LQS_DEFAULT_ITEMS_SAMPLING  1000000
#define LQS_SOURCE_TYPE             SD_JOURNAL_FILE_SOURCE_TYPE
#define LQS_SOURCE_TYPE_ALL         SDJF_ALL
#define LQS_SOURCE_TYPE_NONE        SDJF_NONE
#define LQS_PARAMETER_SOURCE_NAME   "Journal Sources" // this is how it is shown to users
#define LQS_FUNCTION_GET_INTERNAL_SOURCE_TYPE(value) get_internal_source_type(value)
#define LQS_FUNCTION_SOURCE_TO_JSON_ARRAY(wb) available_journal_file_sources_to_json_array(wb)
#include "libnetdata/facets/logs_query_status.h"

#include "systemd-journal-sampling.h"

#define FACET_MAX_VALUE_LENGTH                  8192
#define SYSTEMD_JOURNAL_DEFAULT_TIMEOUT         60
#define SYSTEMD_JOURNAL_PROGRESS_EVERY_UT       (250 * USEC_PER_MS)
#define JOURNAL_KEY_ND_JOURNAL_FILE             "ND_JOURNAL_FILE"
#define JOURNAL_KEY_ND_JOURNAL_PROCESS          "ND_JOURNAL_PROCESS"
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

// ----------------------------------------------------------------------------

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
    lqs_query_timeframe(fqs, anchor_delta);
    usec_t start_ut = fqs->query.start_ut;
    usec_t stop_ut = fqs->query.stop_ut;
    bool stop_when_full = fqs->query.stop_when_full;

    fqs->c.query_file.start_ut = start_ut;
    fqs->c.query_file.stop_ut = stop_ut;

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
            fqs->c.query_file.first_msg_ut = msg_ut;

#ifdef HAVE_SD_JOURNAL_GET_SEQNUM
            if(sd_journal_get_seqnum(j, &fqs->c.query_file.first_msg_seqnum, &fqs->c.query_file.first_msg_writer) < 0) {
                fqs->c.query_file.first_msg_seqnum = 0;
                fqs->c.query_file.first_msg_writer = SD_ID128_NULL;
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
                FUNCTION_PROGRESS_UPDATE_ROWS(fqs->c.rows_read, row_counter - last_row_counter);
                last_row_counter = row_counter;

                FUNCTION_PROGRESS_UPDATE_BYTES(fqs->c.bytes_read, bytes - last_bytes);
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

    FUNCTION_PROGRESS_UPDATE_ROWS(fqs->c.rows_read, row_counter - last_row_counter);
    FUNCTION_PROGRESS_UPDATE_BYTES(fqs->c.bytes_read, bytes - last_bytes);

    fqs->c.rows_useful += rows_useful;

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
    lqs_query_timeframe(fqs, anchor_delta);
    usec_t start_ut = fqs->query.start_ut;
    usec_t stop_ut = fqs->query.stop_ut;
    bool stop_when_full = fqs->query.stop_when_full;

    fqs->c.query_file.start_ut = start_ut;
    fqs->c.query_file.stop_ut = stop_ut;

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
            fqs->c.query_file.first_msg_ut = msg_ut;
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
                FUNCTION_PROGRESS_UPDATE_ROWS(fqs->c.rows_read, row_counter - last_row_counter);
                last_row_counter = row_counter;

                FUNCTION_PROGRESS_UPDATE_BYTES(fqs->c.bytes_read, bytes - last_bytes);
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

    FUNCTION_PROGRESS_UPDATE_ROWS(fqs->c.rows_read, row_counter - last_row_counter);
    FUNCTION_PROGRESS_UPDATE_BYTES(fqs->c.bytes_read, bytes - last_bytes);

    fqs->c.rows_useful += rows_useful;

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
static bool netdata_systemd_filtering_by_journal(sd_journal *j, FACETS *facets, LOGS_QUERY_STATUS *lqs) {
    const char *field = NULL;
    const void *data = NULL;
    size_t data_length;
    size_t added_keys = 0;
    size_t failures = 0;
    size_t filters_added = 0;

    SD_JOURNAL_FOREACH_FIELD(j, field) { // for each key
        bool interesting;

        if(lqs->rq.data_only)
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
        lqs_log_error(lqs, "failed to setup journal filter, will run the full query.");
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

        fqs->c.matches_setup_ut += (ended - started);
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

    lqs->c.files_matched = 0;
    lqs->c.file_working = 0;
    lqs->c.rows_useful = 0;
    lqs->c.rows_read = 0;
    lqs->c.bytes_read = 0;

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

    lqs->c.files_matched = files_used;

    if(lqs->rq.if_modified_since && !files_are_newer) {
        // release the files
        for(size_t f = 0; f < files_used ;f++)
            dictionary_acquired_item_release(journal_files_registry, file_items[f]);

        return rrd_call_function_error(wb, "No new data since the previous call.", HTTP_RESP_NOT_MODIFIED);
    }

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

        lqs->c.file_working++;
        // fqs->cached_count = 0;

        size_t fs_calls = fstat_thread_calls;
        size_t fs_cached = fstat_thread_cached_responses;
        size_t rows_useful = lqs->c.rows_useful;
        size_t rows_read = lqs->c.rows_read;
        size_t bytes_read = lqs->c.bytes_read;
        size_t matches_setup_ut = lqs->c.matches_setup_ut;

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

        rows_useful = lqs->c.rows_useful - rows_useful;
        rows_read = lqs->c.rows_read - rows_read;
        bytes_read = lqs->c.bytes_read - bytes_read;
        matches_setup_ut = lqs->c.matches_setup_ut - matches_setup_ut;
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
                    buffer_json_member_add_uint64(wb, "sampled", lqs->c.samples_per_file.sampled);
                    buffer_json_member_add_uint64(wb, "unsampled", lqs->c.samples_per_file.unsampled);
                    buffer_json_member_add_uint64(wb, "estimated", lqs->c.samples_per_file.estimated);
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
            if(lqs->rq.if_modified_since && !lqs->c.rows_useful)
                return rrd_call_function_error(wb, "No additional useful data since the previous call.", HTTP_RESP_NOT_MODIFIED);
            break;

        case ND_SD_JOURNAL_TIMED_OUT:
        case ND_SD_JOURNAL_NO_FILE_MATCHED:
            break;

        case ND_SD_JOURNAL_CANCELLED:
            return rrd_call_function_error(wb, "Request cancelled.", HTTP_RESP_CLIENT_CLOSED_REQUEST);

        case ND_SD_JOURNAL_NOT_MODIFIED:
            return rrd_call_function_error(wb, "No new data since the previous call.", HTTP_RESP_NOT_MODIFIED);

        case ND_SD_JOURNAL_FAILED_TO_OPEN:
            return rrd_call_function_error(wb, "Failed to open systemd journal file.", HTTP_RESP_INTERNAL_SERVER_ERROR);

        case ND_SD_JOURNAL_FAILED_TO_SEEK:
            return rrd_call_function_error(wb, "Failed to seek in systemd journal file.", HTTP_RESP_INTERNAL_SERVER_ERROR);

        default:
            return rrd_call_function_error(wb, "Unknown status", HTTP_RESP_INTERNAL_SERVER_ERROR);
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

        if(lqs->c.samples.estimated || lqs->c.samples.unsampled) {
            double percent = (double) (lqs->c.samples.sampled * 100.0 /
                                       (lqs->c.samples.estimated + lqs->c.samples.unsampled + lqs->c.samples.sampled));
            buffer_sprintf(msg, "%.2f%% real data", percent);
            buffer_sprintf(msg_description, "ACTUAL DATA: The filters counters reflect %0.2f%% of the data. ", percent);
            msg_priority = MIN(msg_priority, NDLP_NOTICE);
        }

        if(lqs->c.samples.unsampled) {
            double percent = (double) (lqs->c.samples.unsampled * 100.0 /
                                       (lqs->c.samples.estimated + lqs->c.samples.unsampled + lqs->c.samples.sampled));
            buffer_sprintf(msg, ", %.2f%% unsampled", percent);
            buffer_sprintf(msg_description
                           , "UNSAMPLED DATA: %0.2f%% of the events exist and have been counted, but their values have not been evaluated, so they are not included in the filters counters. "
                           , percent);
            msg_priority = MIN(msg_priority, NDLP_NOTICE);
        }

        if(lqs->c.samples.estimated) {
            double percent = (double) (lqs->c.samples.estimated * 100.0 /
                                       (lqs->c.samples.estimated + lqs->c.samples.unsampled + lqs->c.samples.sampled));
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

    wb->expires = now_realtime_sec() + (lqs->rq.data_only ? 3600 : 0);
    buffer_json_member_add_time_t(wb, "expires", wb->expires);

    buffer_json_member_add_object(wb, "_fstat_caching");
    {
        buffer_json_member_add_uint64(wb, "calls", fstat_thread_calls);
        buffer_json_member_add_uint64(wb, "cached", fstat_thread_cached_responses);
    }
    buffer_json_object_close(wb); // _fstat_caching

    if(lqs->rq.sampling) {
        buffer_json_member_add_object(wb, "_sampling");
        {
            buffer_json_member_add_uint64(wb, "sampled", lqs->c.samples.sampled);
            buffer_json_member_add_uint64(wb, "unsampled", lqs->c.samples.unsampled);
            buffer_json_member_add_uint64(wb, "estimated", lqs->c.samples.estimated);
        }
        buffer_json_object_close(wb); // _sampling
    }

    wb->content_type = CT_APPLICATION_JSON;
    wb->response_code = HTTP_RESP_OK;
    return wb->response_code;
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
            LQS_DEFAULT_ITEMS_PER_QUERY,
            FACETS_OPTION_ALL_KEYS_FTS | FACETS_OPTION_HASH_IDS,
            SYSTEMD_ALWAYS_VISIBLE_KEYS,
            SYSTEMD_KEYS_INCLUDED_IN_FACETS,
            SYSTEMD_KEYS_EXCLUDED_FROM_FACETS,
            have_slice),

        .rq = LOGS_QUERY_REQUEST_DEFAULTS(transaction, LQS_DEFAULT_SLICE_MODE, JOURNAL_DEFAULT_DIRECTION),

        .cancelled = cancelled,
        .stop_monotonic_ut = stop_monotonic_ut,
    };
    LOGS_QUERY_STATUS *lqs = &tmp_fqs;

    CLEAN_BUFFER *wb = lqs_create_output_buffer();

    // ------------------------------------------------------------------------
    // parse the parameters

    if(lqs_request_parse_and_validate(lqs, wb, function, payload, have_slice, "PRIORITY")) {
        systemd_journal_register_transformations(lqs);

        // ------------------------------------------------------------------------
        // add versions to the response

        buffer_json_journal_versions(wb);

        // ------------------------------------------------------------------------
        // run the request

        if (lqs->rq.info)
            lqs_info_response(wb, lqs->facets);
        else {
            netdata_systemd_journal_query(wb, lqs);
            if (wb->response_code == HTTP_RESP_OK)
                buffer_json_finalize(wb);
        }
    }

    netdata_mutex_lock(&stdout_mutex);
    pluginsd_function_result_to_stdout(transaction, wb);
    netdata_mutex_unlock(&stdout_mutex);

    lqs_cleanup(lqs);
}
