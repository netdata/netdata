// SPDX-License-Identifier: GPL-3.0-or-later

#include "libnetdata/libnetdata.h"
#include "libnetdata/required_dummies.h"

#include "windows-events.h"

#define WINDOWS_EVENTS_WORKER_THREADS 5
#define WINDOWS_EVENTS_DEFAULT_TIMEOUT 600
#define WINDOWS_EVENTS_SCAN_EVERY_USEC (5 * 60 * USEC_PER_SEC)
#define WINDOWS_EVENTS_PROGRESS_EVERY_UT (250 * USEC_PER_MS)

#define FUNCTION_PROGRESS_EVERY_ROWS (2000)
#define FUNCTION_DATA_ONLY_CHECK_EVERY_ROWS (1000)
#define ANCHOR_DELTA_UT (10 * USEC_PER_SEC)

netdata_mutex_t stdout_mutex = NETDATA_MUTEX_INITIALIZER;
static bool plugin_should_exit = false;

#define WEVT_FUNCTION_DESCRIPTION    "View, search and analyze the Microsoft Windows Events log."
#define WEVT_FUNCTION_NAME           "windows-events"

// functions needed by LQS

// structures needed by LQS
struct lqs_extension {
    wchar_t *query;

    struct {
        struct {
            size_t completed;
            size_t total;
        } queries;

        struct {
            size_t current_query_total;
            size_t completed;
            size_t total;
        } entries;

        usec_t last_ut;
    } progress;

    // struct {
    //     usec_t start_ut;
    //     usec_t stop_ut;
    //     usec_t first_msg_ut;
    //
    //     uint64_t first_msg_seqnum;
    // } query_file;

    // struct {
    //     uint32_t enable_after_samples;
    //     uint32_t slots;
    //     uint32_t sampled;
    //     uint32_t unsampled;
    //     uint32_t estimated;
    // } samples;

    // struct {
    //     uint32_t enable_after_samples;
    //     uint32_t every;
    //     uint32_t skipped;
    //     uint32_t recalibrate;
    //     uint32_t sampled;
    //     uint32_t unsampled;
    //     uint32_t estimated;
    // } samples_per_file;

    // struct {
    //     usec_t start_ut;
    //     usec_t end_ut;
    //     usec_t step_ut;
    //     uint32_t enable_after_samples;
    //     uint32_t sampled[SYSTEMD_JOURNAL_SAMPLING_SLOTS];
    //     uint32_t unsampled[SYSTEMD_JOURNAL_SAMPLING_SLOTS];
    // } samples_per_time_slot;

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
#define LQS_DEFAULT_SLICE_MODE      0
#define LQS_FUNCTION_NAME           WEVT_FUNCTION_NAME
#define LQS_FUNCTION_DESCRIPTION    WEVT_FUNCTION_DESCRIPTION
#define LQS_DEFAULT_ITEMS_PER_QUERY 200
#define LQS_DEFAULT_ITEMS_SAMPLING  1000000
#define LQS_SOURCE_TYPE             WEVT_SOURCE_TYPE
#define LQS_SOURCE_TYPE_ALL         WEVTS_ALL
#define LQS_SOURCE_TYPE_NONE        WEVTS_NONE
#define LQS_FUNCTION_GET_INTERNAL_SOURCE_TYPE(value) wevt_internal_source_type(value)
#define LQS_FUNCTION_SOURCE_TO_JSON_ARRAY(wb) wevt_sources_to_json_array(wb)
#include "libnetdata/facets/logs_query_status.h"

#define WEVT_FIELD_COMPUTER             "Computer"
#define WEVT_FIELD_CHANNEL              "Channel"
#define WEVT_FIELD_PROVIDER             "Provider"
#define WEVT_FIELD_SOURCE               "Source"
#define WEVT_FIELD_EVENTRECORDID        "EventRecordID"
#define WEVT_FIELD_EVENTID              "EventID"
#define WEVT_FIELD_LEVEL                "Level"
#define WEVT_FIELD_KEYWORDS             "Keywords"
#define WEVT_FIELD_OPCODE               "Opcode"
#define WEVT_FIELD_USER                 "User"
#define WEVT_FIELD_TASK                 "Task"
#define WEVT_FIELD_PROCESSID            "ProcessID"
#define WEVT_FIELD_THREADID             "ThreadID"
#define WEVT_FIELD_XML                  "XML"
#define WEVT_FIELD_MESSAGE              "Message"

#define WEVT_ALWAYS_VISIBLE_KEYS                NULL

#define WEVT_KEYS_EXCLUDED_FROM_FACETS          \
    "|" WEVT_FIELD_MESSAGE                      \
    "|" WEVT_FIELD_XML                          \
    ""

#define WEVT_KEYS_INCLUDED_IN_FACETS            \
    "|" WEVT_FIELD_PROVIDER                     \
    "|" WEVT_FIELD_SOURCE                       \
    "|" WEVT_FIELD_CHANNEL                      \
    "|" WEVT_FIELD_LEVEL                        \
    "|" WEVT_FIELD_KEYWORDS                     \
    "|" WEVT_FIELD_OPCODE                       \
    "|" WEVT_FIELD_TASK                         \
    "|" WEVT_FIELD_COMPUTER                     \
    "|" WEVT_FIELD_USER                         \
    ""

static inline WEVT_QUERY_STATUS check_stop(const bool *cancelled, const usec_t *stop_monotonic_ut) {
    if(cancelled && __atomic_load_n(cancelled, __ATOMIC_RELAXED)) {
        nd_log(NDLS_COLLECTORS, NDLP_INFO, "Function has been cancelled");
        return WEVT_CANCELLED;
    }

    if(now_monotonic_usec() > __atomic_load_n(stop_monotonic_ut, __ATOMIC_RELAXED)) {
        internal_error(true, "Function timed out");
        return WEVT_TIMED_OUT;
    }

    return WEVT_OK;
}

FACET_ROW_SEVERITY wevt_levelid_to_facet_severity(FACETS *facets __maybe_unused, FACET_ROW *row, void *data __maybe_unused) {
    FACET_ROW_KEY_VALUE *levelid_rkv = dictionary_get(row->dict, WEVT_FIELD_LEVEL);
    if(!levelid_rkv || levelid_rkv->empty)
        return FACET_ROW_SEVERITY_NORMAL;

    int windows_event_level = str2i(buffer_tostring(levelid_rkv->wb));

    switch (windows_event_level) {
        case 5: // Verbose
            return FACET_ROW_SEVERITY_DEBUG;
        default:
        case 4: // Information
            return FACET_ROW_SEVERITY_NORMAL;
        case 3: // Warning
            return FACET_ROW_SEVERITY_WARNING;
        case 2: // Error
        case 1: // Critical
            return FACET_ROW_SEVERITY_CRITICAL;
    }
}

void wevt_render_message(
    FACETS *facets __maybe_unused,
    BUFFER *json_array,
    FACET_ROW_KEY_VALUE *rkv,
    FACET_ROW *row,
    void *data __maybe_unused) {

    buffer_flush(rkv->wb);

    const FACET_ROW_KEY_VALUE *xml_rkv = dictionary_get(row->dict, WEVT_FIELD_XML);

    {
        bool added_message = false;

        if(xml_rkv && buffer_strlen(xml_rkv->wb)) {
            const char *message_path[] = {
                "RenderingInfo",
                "Message",
                NULL};

            added_message = buffer_extract_and_print_value(
                rkv->wb,
                buffer_tostring(xml_rkv->wb),
                buffer_strlen(xml_rkv->wb),
                NULL,
                message_path);
        }

        if(!added_message) {
            const FACET_ROW_KEY_VALUE *event_id_rkv = dictionary_get(row->dict, WEVT_FIELD_EVENTID);
            if (event_id_rkv && buffer_strlen(event_id_rkv->wb)) {
                buffer_fast_strcat(rkv->wb, "EventID ", 8);
                buffer_fast_strcat(rkv->wb, buffer_tostring(event_id_rkv->wb), buffer_strlen(event_id_rkv->wb));
            } else
                buffer_strcat(rkv->wb, "Unknown EventID ");

            const FACET_ROW_KEY_VALUE *provider_rkv = dictionary_get(row->dict, WEVT_FIELD_PROVIDER);
            if (provider_rkv && buffer_strlen(provider_rkv->wb)) {
                buffer_fast_strcat(rkv->wb, " of ", 4);
                buffer_fast_strcat(rkv->wb, buffer_tostring(provider_rkv->wb), buffer_strlen(provider_rkv->wb));
                buffer_putc(rkv->wb, '.');
            } else
                buffer_strcat(rkv->wb, "of unknown Provider.");
        }
    }

    if(xml_rkv && buffer_strlen(xml_rkv->wb)) {
        const char *event_path[] = {
            "EventData",
            NULL
        };
        bool added_event_data = buffer_extract_and_print_xml(
            rkv->wb,
            buffer_tostring(xml_rkv->wb), buffer_strlen(xml_rkv->wb),
            "\n\nRelated event data:\n",
            event_path);

        const char *user_path[] = {
            "UserData",
            NULL
        };
        bool added_user_data = buffer_extract_and_print_xml(
            rkv->wb,
            buffer_tostring(xml_rkv->wb), buffer_strlen(xml_rkv->wb),
            "\n\nRelated user data:\n",
            user_path);

        if(!added_event_data && !added_user_data)
            buffer_strcat(rkv->wb, " Without any related data.");
    }

    buffer_json_add_array_item_string(json_array, buffer_tostring(rkv->wb));
}

static const char *wevt_level_to_name(int level) {
    switch (level) {
        case 5:
            return "Verbose";
        case 4:
            return "Information";
        case 3:
            return "Warning";
        case 2:
            return "Error";
        case 1:
            return "Critical";

        case 0:
            // it seems that this defaults to "Information" for some providers
            // however we need to differentiate to support slicing on it
            return "None";

        default:
            return NULL;
    }
}

static void wevt_transform_level(FACETS *facets __maybe_unused, BUFFER *wb, FACETS_TRANSFORMATION_SCOPE scope __maybe_unused, void *data __maybe_unused) {
    if(scope == FACETS_TRANSFORM_FACET_SORT)
        return;

    const char *v = buffer_tostring(wb);
    if(*v && isdigit((uint8_t)*v)) {
        int priority = str2i(buffer_tostring(wb));
        const char *name = wevt_level_to_name(priority);
        if (name) {
            buffer_flush(wb);
            buffer_strcat(wb, name);
        }
    }
}

static void wevt_register_fields(LOGS_QUERY_STATUS *lqs) {
    FACETS *facets = lqs->facets;
    LOGS_QUERY_REQUEST *rq = &lqs->rq;

    facets_register_row_severity(facets, wevt_levelid_to_facet_severity, NULL);

    facets_register_key_name(
            facets, WEVT_FIELD_COMPUTER,
            rq->default_facet | FACET_KEY_OPTION_VISIBLE);

    facets_register_key_name(
            facets, WEVT_FIELD_CHANNEL,
            rq->default_facet |
            FACET_KEY_OPTION_FTS);

    facets_register_key_name(
            facets, WEVT_FIELD_PROVIDER,
            rq->default_facet |
            FACET_KEY_OPTION_VISIBLE | FACET_KEY_OPTION_FTS);

    facets_register_key_name(
            facets, WEVT_FIELD_SOURCE,
            rq->default_facet |
            FACET_KEY_OPTION_FTS);

    facets_register_key_name(
            facets, WEVT_FIELD_EVENTID,
            rq->default_facet |
            FACET_KEY_OPTION_VISIBLE | FACET_KEY_OPTION_FTS);

    facets_register_key_name_transformation(
        facets,
        WEVT_FIELD_LEVEL,
        rq->default_facet | FACET_KEY_OPTION_TRANSFORM_VIEW | FACET_KEY_OPTION_EXPANDED_FILTER,
        wevt_transform_level,
        NULL);

    facets_register_key_name(
            facets, WEVT_FIELD_USER,
            rq->default_facet | FACET_KEY_OPTION_FTS);

    facets_register_key_name(
            facets, WEVT_FIELD_PROCESSID,
            rq->default_facet | FACET_KEY_OPTION_FTS);

    facets_register_key_name(
            facets, WEVT_FIELD_THREADID,
            rq->default_facet | FACET_KEY_OPTION_FTS);

    facets_register_key_name(
            facets, WEVT_FIELD_TASK,
            rq->default_facet | FACET_KEY_OPTION_FTS);

    facets_register_key_name(
            facets, WEVT_FIELD_PROCESSID,
            rq->default_facet | FACET_KEY_OPTION_FTS);

    facets_register_key_name(
        facets, WEVT_FIELD_THREADID,
        rq->default_facet | FACET_KEY_OPTION_FTS);

    facets_register_key_name(
        facets, WEVT_FIELD_KEYWORDS,
        rq->default_facet | FACET_KEY_OPTION_FTS);

    facets_register_key_name(
        facets, WEVT_FIELD_OPCODE,
        rq->default_facet | FACET_KEY_OPTION_FTS);

    facets_register_key_name(
        facets, WEVT_FIELD_TASK,
        rq->default_facet | FACET_KEY_OPTION_FTS);

    facets_register_dynamic_key_name(
        facets, WEVT_FIELD_MESSAGE,
        FACET_KEY_OPTION_NEVER_FACET | FACET_KEY_OPTION_MAIN_TEXT | FACET_KEY_OPTION_VISIBLE,
        wevt_render_message, NULL);

    facets_register_key_name(
        facets, WEVT_FIELD_XML,
        FACET_KEY_OPTION_NEVER_FACET | FACET_KEY_OPTION_FTS);
}

static inline size_t wevt_process_keywords(WEVT_LOG *log, FACETS *facets, WEVT_EVENT *e) {
    static __thread char keywords_id_str[UINT64_HEX_MAX_LENGTH];
    size_t len = print_uint64_hex_full(keywords_id_str, e->keywords);
    const char *value = keywords_id_str;

    if(log->ops.xml.data && log->ops.xml.used > 1) {
        static const char *path[] = {
            "RenderingInfo",
            "Keywords",
            NULL,
        };

        buffer_flush(log->ops.keywords);
        if(buffer_extract_and_print_value(log->ops.keywords, log->ops.xml.data, log->ops.xml.used - 1, NULL, path)) {
            if (buffer_strlen(log->ops.keywords)) {
                value = buffer_tostring(log->ops.keywords);
                len = buffer_strlen(log->ops.keywords);
            }
        }
    }

    facets_add_key_value_length(facets, WEVT_FIELD_KEYWORDS, sizeof(WEVT_FIELD_KEYWORDS) - 1,
                                value, len);

    return len + 1;
}

static inline size_t wevt_process_opcode(WEVT_LOG *log, FACETS *facets, WEVT_EVENT *e) {
    static __thread char opcode_id_str[UINT64_MAX_LENGTH];
    size_t len = print_uint64(opcode_id_str, e->opcode);
    const char *value = opcode_id_str;

    if(log->ops.xml.data && log->ops.xml.used > 1) {
        static const char *path[] = {
            "RenderingInfo",
            "Opcode",
            NULL,
        };

        buffer_flush(log->ops.opcode);
        if(buffer_extract_and_print_value(log->ops.opcode, log->ops.xml.data, log->ops.xml.used - 1, NULL, path)) {
            if (buffer_strlen(log->ops.opcode)) {
                value = buffer_tostring(log->ops.opcode);
                len = buffer_strlen(log->ops.opcode);
            }
        }
    }

    facets_add_key_value_length(facets, WEVT_FIELD_OPCODE, sizeof(WEVT_FIELD_OPCODE) - 1,
                                value, len);

    return len + 1;
}

static inline size_t wevt_process_task(WEVT_LOG *log, FACETS *facets, WEVT_EVENT *e) {
    static __thread char task_str[UINT64_MAX_LENGTH];
    size_t len = print_uint64(task_str, e->task);
    const char *value = task_str;

    if(log->ops.xml.data && log->ops.xml.used > 1) {
        static const char *path[] = {
            "RenderingInfo",
            "Task",
            NULL,
        };

        buffer_flush(log->ops.task);
        if(buffer_extract_and_print_value(log->ops.task, log->ops.xml.data, log->ops.xml.used - 1, NULL, path)) {
            if (buffer_strlen(log->ops.task)) {
                value = buffer_tostring(log->ops.task);
                len = buffer_strlen(log->ops.task);
            }
        }
    }

    facets_add_key_value_length(facets,
                                WEVT_FIELD_TASK, sizeof(WEVT_FIELD_TASK) - 1,
                                value, len);

    return len + 1;
}

static inline size_t wevt_process_event(WEVT_LOG *log, FACETS *facets, LOGS_QUERY_SOURCE *src, usec_t *msg_ut __maybe_unused, WEVT_EVENT *e) {
    size_t len, bytes = log->ops.content.len;

    {
        bytes += log->ops.provider.used * 2; // unicode is double
        facets_add_key_value_length(
            facets, WEVT_FIELD_PROVIDER, sizeof(WEVT_FIELD_PROVIDER) - 1,
            log->ops.provider.data, log->ops.provider.used - 1);
    }

    {
        bytes += log->ops.source.used * 2;
        facets_add_key_value_length(
            facets, WEVT_FIELD_SOURCE, sizeof(WEVT_FIELD_SOURCE) - 1,
            log->ops.source.data, log->ops.source.used - 1);
    }

    if(log->ops.channel.used > 1) {
        bytes += log->ops.channel.used * 2;
        facets_add_key_value_length(
            facets, WEVT_FIELD_CHANNEL, sizeof(WEVT_FIELD_CHANNEL) - 1,
            log->ops.channel.data, log->ops.channel.used - 1);
    }
    else {
        bytes += src->fullname_len * 2;
        facets_add_key_value_length(
            facets, WEVT_FIELD_CHANNEL, sizeof(WEVT_FIELD_CHANNEL) - 1,
            src->fullname, src->fullname_len);
    }

    {
        static __thread char event_record_id_str[UINT64_MAX_LENGTH];
        len = print_uint64(event_record_id_str, e->id);
        bytes += len;
        facets_add_key_value_length(
            facets, WEVT_FIELD_EVENTRECORDID, sizeof(WEVT_FIELD_EVENTRECORDID) - 1,
            event_record_id_str, len);
    }

    {
        static __thread char level_id_str[UINT64_MAX_LENGTH];
        len = print_uint64(level_id_str, e->level);
        bytes += len;
        facets_add_key_value_length(
            facets, WEVT_FIELD_LEVEL, sizeof(WEVT_FIELD_LEVEL) - 1,
            level_id_str, len);
    }

    {
        bytes += log->ops.computer.used * 2;
        facets_add_key_value_length(
            facets, WEVT_FIELD_COMPUTER, sizeof(WEVT_FIELD_COMPUTER) - 1,
            log->ops.computer.data, log->ops.computer.used - 1);
    }

    bytes += wevt_process_keywords(log, facets, e);
    bytes += wevt_process_opcode(log, facets, e);
    bytes += wevt_process_task(log, facets, e);

    {
        bytes += log->ops.user.used * 2;
        facets_add_key_value_length(
            facets, WEVT_FIELD_USER, sizeof(WEVT_FIELD_USER) - 1,
            log->ops.user.data, log->ops.user.used - 1);
    }

    {
        static __thread char event_id_str[UINT64_MAX_LENGTH];
        len = print_uint64(event_id_str, e->event_id);
        bytes += len;
        facets_add_key_value_length(
            facets, WEVT_FIELD_EVENTID, sizeof(WEVT_FIELD_EVENTID) - 1,
            event_id_str, len);
    }

    if(e->process_id) {
        static __thread char process_id_str[UINT64_MAX_LENGTH];
        len = print_uint64(process_id_str, e->process_id);
        bytes += len;
        facets_add_key_value_length(
            facets, WEVT_FIELD_PROCESSID, sizeof(WEVT_FIELD_PROCESSID) - 1,
            process_id_str, len);
    }

    if(e->thread_id) {
        static __thread char thread_id_str[UINT64_MAX_LENGTH];
        len = print_uint64(thread_id_str, e->thread_id);
        bytes += len;
        facets_add_key_value_length(
            facets, WEVT_FIELD_THREADID, sizeof(WEVT_FIELD_THREADID) - 1,
            thread_id_str, len);
    }

    bytes += log->ops.xml.used * 2;
    facets_add_key_value_length(
        facets, WEVT_FIELD_XML, sizeof(WEVT_FIELD_XML) - 1,
        log->ops.xml.data, log->ops.xml.used - 1);

    return bytes;
}

static void send_progress_update(LOGS_QUERY_STATUS *lqs, size_t current_row_counter, bool flush_current_file) {
    usec_t now_ut = now_monotonic_usec();

    if(current_row_counter > lqs->c.progress.entries.current_query_total) {
        lqs->c.progress.entries.total += current_row_counter - lqs->c.progress.entries.current_query_total;
        lqs->c.progress.entries.current_query_total = current_row_counter;
    }

    if(flush_current_file) {
        lqs->c.progress.entries.total += current_row_counter;
        lqs->c.progress.entries.total -= lqs->c.progress.entries.current_query_total;
        lqs->c.progress.entries.completed += current_row_counter;
        lqs->c.progress.entries.current_query_total = 0;
    }

    size_t completed = lqs->c.progress.entries.completed + current_row_counter;
    if(completed > lqs->c.progress.entries.total)
        lqs->c.progress.entries.total = completed;

    usec_t progress_duration_ut = now_ut - lqs->c.progress.last_ut;
    if(progress_duration_ut >= WINDOWS_EVENTS_PROGRESS_EVERY_UT) {
        lqs->c.progress.last_ut = now_ut;

        netdata_mutex_lock(&stdout_mutex);
        pluginsd_function_progress_to_stdout(lqs->rq.transaction, completed, lqs->c.progress.entries.total);
        netdata_mutex_unlock(&stdout_mutex);
    }
}

static WEVT_QUERY_STATUS wevt_query_backward(
        WEVT_LOG *log, BUFFER *wb __maybe_unused, FACETS *facets,
        LOGS_QUERY_SOURCE *src,
        LOGS_QUERY_STATUS *lqs)
{
    usec_t start_ut = lqs->query.start_ut;
    usec_t stop_ut = lqs->query.stop_ut;
    bool stop_when_full = lqs->query.stop_when_full;

//    lqs->c.query_file.start_ut = start_ut;
//    lqs->c.query_file.stop_ut = stop_ut;

    log->event_query = wevt_query(channel2unicode(src->fullname), lqs->c.query, EvtQueryReverseDirection);
    if(!log->event_query)
        return WEVT_FAILED_TO_SEEK;

    size_t errors_no_timestamp = 0;
    usec_t latest_msg_ut = 0; // the biggest timestamp we have seen so far
    usec_t first_msg_ut = 0; // the first message we got from the db
    size_t row_counter = 0, last_row_counter = 0, rows_useful = 0;
    size_t bytes = 0, last_bytes = 0;

    usec_t last_usec_from = 0;
    usec_t last_usec_to = 0;

    WEVT_QUERY_STATUS status = WEVT_OK;

    facets_rows_begin(facets);
    WEVT_EVENT e;
    while (status == WEVT_OK && wevt_get_next_event(log, &e, true)) {
        usec_t msg_ut = e.created_ns / NSEC_PER_USEC;

        if(unlikely(!msg_ut)) {
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
            // lqs->c.query_file.first_msg_ut = msg_ut;
        }

//        sampling_t sample = is_row_in_sample(log, lqs, src, msg_ut,
//                                             FACETS_ANCHOR_DIRECTION_BACKWARD,
//                                             facets_row_candidate_to_keep(facets, msg_ut));
//
//        if(sample == SAMPLING_FULL) {
            bytes += wevt_process_event(log, facets, src, &msg_ut, &e);

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
                        facets_rows(facets) >= lqs->rq.entries)) {
                // stop the data only query
                usec_t oldest = facets_row_oldest_ut(facets);
                if(oldest && msg_ut < (oldest - lqs->anchor.delta_ut))
                    break;
            }

            if(unlikely(row_counter % FUNCTION_PROGRESS_EVERY_ROWS == 0)) {
                status = check_stop(lqs->cancelled, lqs->stop_monotonic_ut);

                if(status == WEVT_OK) {
                    lqs->c.rows_read += row_counter - last_row_counter;
                    last_row_counter = row_counter;

                    lqs->c.bytes_read += bytes - last_bytes;
                    last_bytes = bytes;

                    send_progress_update(lqs, row_counter, false);
                }
            }
//        }
//        else if(sample == SAMPLING_SKIP_FIELDS)
//            facets_row_finished_unsampled(facets, msg_ut);
//        else {
//            sampling_update_running_query_file_estimates(facets, log, lqs, src, msg_ut, FACETS_ANCHOR_DIRECTION_BACKWARD);
//            break;
//        }
    }

    send_progress_update(lqs, row_counter, true);
    lqs->c.rows_read += row_counter - last_row_counter;
    lqs->c.bytes_read += bytes - last_bytes;
    lqs->c.rows_useful += rows_useful;

    if(errors_no_timestamp)
        netdata_log_error("WINDOWS-EVENTS: %zu events did not have timestamps", errors_no_timestamp);

    if(latest_msg_ut > lqs->last_modified)
        lqs->last_modified = latest_msg_ut;

    wevt_query_done(log);

    return status;
}

static WEVT_QUERY_STATUS wevt_query_forward(
        WEVT_LOG *log, BUFFER *wb __maybe_unused, FACETS *facets,
        LOGS_QUERY_SOURCE *src,
        LOGS_QUERY_STATUS *lqs)
{
    usec_t start_ut = lqs->query.start_ut;
    usec_t stop_ut = lqs->query.stop_ut;
    bool stop_when_full = lqs->query.stop_when_full;

//    lqs->c.query_file.start_ut = start_ut;
//    lqs->c.query_file.stop_ut = stop_ut;

    log->event_query = wevt_query(channel2unicode(src->fullname), lqs->c.query, EvtQueryForwardDirection);
    if(!log->event_query)
        return WEVT_FAILED_TO_SEEK;

    size_t errors_no_timestamp = 0;
    usec_t latest_msg_ut = 0; // the biggest timestamp we have seen so far
    usec_t first_msg_ut = 0; // the first message we got from the db
    size_t row_counter = 0, last_row_counter = 0, rows_useful = 0;
    size_t bytes = 0, last_bytes = 0;

    usec_t last_usec_from = 0;
    usec_t last_usec_to = 0;

    WEVT_QUERY_STATUS status = WEVT_OK;

    facets_rows_begin(facets);
    WEVT_EVENT e;
    while (status == WEVT_OK && wevt_get_next_event(log, &e, true)) {
        usec_t msg_ut = e.created_ns / NSEC_PER_USEC;

        if(unlikely(!msg_ut)) {
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
            // lqs->c.query_file.first_msg_ut = msg_ut;
        }

//        sampling_t sample = is_row_in_sample(log, lqs, src, msg_ut,
//                                             FACETS_ANCHOR_DIRECTION_FORWARD,
//                                             facets_row_candidate_to_keep(facets, msg_ut));
//
//        if(sample == SAMPLING_FULL) {
            bytes += wevt_process_event(log, facets, src, &msg_ut, &e);

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
                        facets_rows(facets) >= lqs->rq.entries)) {
                // stop the data only query
                usec_t newest = facets_row_newest_ut(facets);
                if(newest && msg_ut > (newest + lqs->anchor.delta_ut))
                    break;
            }

            if(unlikely(row_counter % FUNCTION_PROGRESS_EVERY_ROWS == 0)) {
                status = check_stop(lqs->cancelled, lqs->stop_monotonic_ut);

                if(status == WEVT_OK) {
                    lqs->c.rows_read += row_counter - last_row_counter;
                    last_row_counter = row_counter;

                    lqs->c.bytes_read += bytes - last_bytes;
                    last_bytes = bytes;

                    send_progress_update(lqs, row_counter, false);
                }
            }
//        }
//        else if(sample == SAMPLING_SKIP_FIELDS)
//            facets_row_finished_unsampled(facets, msg_ut);
//        else {
//            sampling_update_running_query_file_estimates(facets, log, lqs, src, msg_ut, FACETS_ANCHOR_DIRECTION_FORWARD);
//            break;
//        }
    }

    send_progress_update(lqs, row_counter, true);
    lqs->c.rows_read += row_counter - last_row_counter;
    lqs->c.bytes_read += bytes - last_bytes;
    lqs->c.rows_useful += rows_useful;

    if(errors_no_timestamp)
        netdata_log_error("WINDOWS-EVENTS: %zu events did not have timestamps", errors_no_timestamp);

    if(latest_msg_ut > lqs->last_modified)
        lqs->last_modified = latest_msg_ut;

    wevt_query_done(log);

    return status;
}

static WEVT_QUERY_STATUS wevt_query_one_channel(
        WEVT_LOG *log,
        BUFFER *wb, FACETS *facets,
        LOGS_QUERY_SOURCE *src,
        LOGS_QUERY_STATUS *lqs) {

    errno_clear();

    WEVT_QUERY_STATUS status;
    if(lqs->rq.direction == FACETS_ANCHOR_DIRECTION_FORWARD)
        status = wevt_query_forward(log, wb, facets, src, lqs);
    else
        status = wevt_query_backward(log, wb, facets, src, lqs);

    return status;
}

static bool source_is_mine(LOGS_QUERY_SOURCE *src, LOGS_QUERY_STATUS *lqs) {
    if((lqs->rq.source_type == WEVTS_NONE && !lqs->rq.sources) || (src->source_type & lqs->rq.source_type) ||
       (lqs->rq.sources && simple_pattern_matches(lqs->rq.sources, string2str(src->source)))) {

        if(!src->msg_last_ut)
            // the file is not scanned yet, or the timestamps have not been updated,
            // so we don't know if it can contribute or not - let's add it.
            return true;

        usec_t anchor_delta = ANCHOR_DELTA_UT;
        usec_t first_ut = src->msg_first_ut - anchor_delta;
        usec_t last_ut = src->msg_last_ut + anchor_delta;

        if(last_ut >= lqs->rq.after_ut && first_ut <= lqs->rq.before_ut)
            return true;
    }

    return false;
}

typedef struct static_utf8_8k {
    char buffer[8192];
    size_t size;
    size_t len;
} STATIC_BUF_8K;

typedef struct static_unicode_16k {
    wchar_t buffer[16384];
    size_t size;
    size_t len;
} STATIC_UNI_16K;

static bool wevt_foreach_selected_value_cb(FACETS *facets __maybe_unused, size_t id, const char *key, const char *value, void *data) {
    STATIC_BUF_8K *b = data;

    b->len += snprintfz(&b->buffer[b->len], b->size - b->len,
                        "%s%s=%s",
                        id ? " or " : "", key, value);

    return b->len < b->size;
}

static wchar_t *wevt_generate_query(LOGS_QUERY_STATUS *lqs, BUFFER *wb) {
    static __thread STATIC_UNI_16K q = {
        .size = sizeof(q.buffer) / sizeof(wchar_t),
        .len = 0,
    };
    static __thread STATIC_BUF_8K b = {
        .size = sizeof(b.buffer) / sizeof(char),
        .len = 0,
    };

    lqs_query_timeframe(lqs, ANCHOR_DELTA_UT);

    usec_t seek_to = lqs->query.start_ut;
    if(lqs->rq.direction == FACETS_ANCHOR_DIRECTION_BACKWARD)
        // windows events queries are limited to millisecond resolution
        // so, in order not to lose data, we have to add
        // a millisecond when the direction is backward
        seek_to += USEC_PER_MS;

    // Convert the microseconds since Unix epoch to FILETIME (used in Windows APIs)
    FILETIME fileTime = os_unix_epoch_ut_to_filetime(seek_to);

    // Convert FILETIME to SYSTEMTIME for use in XPath
    SYSTEMTIME systemTime;
    if (!FileTimeToSystemTime(&fileTime, &systemTime)) {
        nd_log(NDLS_COLLECTORS, NDLP_ERR, "FileTimeToSystemTime() failed");
        return NULL;
    }

    // Format SYSTEMTIME into ISO 8601 format (YYYY-MM-DDTHH:MM:SS.sssZ)
    q.len = swprintf(q.buffer, q.size,
                 L"Event/System[TimeCreated[@SystemTime%ls\"%04d-%02d-%02dT%02d:%02d:%02d.%03dZ\"]",
                 lqs->rq.direction == FACETS_ANCHOR_DIRECTION_BACKWARD ? L"<=" : L">=",
                 systemTime.wYear, systemTime.wMonth, systemTime.wDay,
                 systemTime.wHour, systemTime.wMinute, systemTime.wSecond, systemTime.wMilliseconds);


    if(lqs->rq.slice) {
        b.len = snprintf(b.buffer, b.size, " and (");
        if (facets_foreach_selected_value_in_key(
                lqs->facets,
                WEVT_FIELD_LEVEL,
                sizeof(WEVT_FIELD_LEVEL) - 1,
                used_hashes_registry,
                wevt_foreach_selected_value_cb,
                &b)) {
            b.len += snprintf(&b.buffer[b.len], b.size - b.len, ")");
            if (b.len < b.size) {
                utf82unicode(&q.buffer[q.len], q.size - q.len, b.buffer);
                q.len = wcslen(q.buffer);
            }
        }

        b.len = snprintf(b.buffer, b.size, " and (");
        if (facets_foreach_selected_value_in_key(
                lqs->facets,
                WEVT_FIELD_EVENTID,
                sizeof(WEVT_FIELD_EVENTID) - 1,
                used_hashes_registry,
                wevt_foreach_selected_value_cb,
                &b)) {
            b.len += snprintf(&b.buffer[b.len], b.size - b.len, ")");
            if (b.len < b.size) {
                utf82unicode(&q.buffer[q.len], q.size - q.len, b.buffer);
                q.len = wcslen(q.buffer);
            }
        }
    }

    q.len += swprintf(&q.buffer[q.len], q.size - q.len, L"]");

    buffer_json_member_add_string(wb, "_query", channel2utf8(q.buffer));

    return q.buffer;
}

static int wevt_master_query(BUFFER *wb __maybe_unused, LOGS_QUERY_STATUS *lqs __maybe_unused) {
    // make sure the sources list is updated
    wevt_sources_scan();

    lqs->c.query = wevt_generate_query(lqs, wb);
    if(!lqs->c.query)
        return rrd_call_function_error(wb, "failed to generate XPath query", HTTP_RESP_INTERNAL_SERVER_ERROR);

    FACETS *facets = lqs->facets;

    WEVT_QUERY_STATUS status = WEVT_NO_CHANNEL_MATCHED;

    lqs->c.files_matched = 0;
    lqs->c.file_working = 0;
    lqs->c.rows_useful = 0;
    lqs->c.rows_read = 0;
    lqs->c.bytes_read = 0;

    size_t files_used = 0;
    size_t files_max = dictionary_entries(wevt_sources);
    const DICTIONARY_ITEM *file_items[files_max];

    // count the files
    bool files_are_newer = false;
    LOGS_QUERY_SOURCE *src;
    dfe_start_read(wevt_sources, src) {
        if(!source_is_mine(src, lqs))
            continue;

        file_items[files_used++] = dictionary_acquired_item_dup(wevt_sources, src_dfe.item);

        if(src->msg_last_ut > lqs->rq.if_modified_since)
            files_are_newer = true;

        lqs->c.progress.entries.total += src->entries;
    }
    dfe_done(jf);

    lqs->c.files_matched = files_used;

    if(lqs->rq.if_modified_since && !files_are_newer) {
        // release the files
        for(size_t f = 0; f < files_used ;f++)
            dictionary_acquired_item_release(wevt_sources, file_items[f]);

        return rrd_call_function_error(wb, "not modified", HTTP_RESP_NOT_MODIFIED);
    }

    // sort the files, so that they are optimal for facets
    if(files_used >= 2) {
        if (lqs->rq.direction == FACETS_ANCHOR_DIRECTION_BACKWARD)
            qsort(file_items, files_used, sizeof(const DICTIONARY_ITEM *),
                  wevt_sources_dict_items_backward_compar);
        else
            qsort(file_items, files_used, sizeof(const DICTIONARY_ITEM *),
                  wevt_sources_dict_items_forward_compar);
    }

    bool partial = false;
    usec_t query_started_ut = now_monotonic_usec();
    usec_t started_ut = query_started_ut;
    usec_t ended_ut = started_ut;
    usec_t duration_ut, max_duration_ut = 0;

    WEVT_LOG *log = wevt_openlog6();
    if(!log) {
        // release the files
        for(size_t f = 0; f < files_used ;f++)
            dictionary_acquired_item_release(wevt_sources, file_items[f]);

        netdata_log_error("WINDOWS EVENTS: cannot open windows event log");
        return rrd_call_function_error(wb, "cannot open windows events log", HTTP_RESP_INTERNAL_SERVER_ERROR);
    }

    // sampling_query_init(lqs, facets);

    buffer_json_member_add_array(wb, "_channels");
    for(size_t f = 0; f < files_used ;f++) {
        const char *fullname = dictionary_acquired_item_name(file_items[f]);
        src = dictionary_acquired_item_value(file_items[f]);

        if(!source_is_mine(src, lqs))
            continue;

        started_ut = ended_ut;

        // do not even try to do the query if we expect it to pass the timeout
        if(ended_ut + max_duration_ut * 3 >= *lqs->stop_monotonic_ut) {
            partial = true;
            status = WEVT_TIMED_OUT;
            break;
        }

        lqs->c.file_working++;

        size_t rows_useful = lqs->c.rows_useful;
        size_t rows_read = lqs->c.rows_read;
        size_t bytes_read = lqs->c.bytes_read;
        size_t matches_setup_ut = lqs->c.matches_setup_ut;

        // sampling_file_init(lqs, src);

        lqs->c.progress.entries.current_query_total = src->entries;
        WEVT_QUERY_STATUS tmp_status = wevt_query_one_channel(log, wb, facets, src, lqs);

        rows_useful = lqs->c.rows_useful - rows_useful;
        rows_read = lqs->c.rows_read - rows_read;
        bytes_read = lqs->c.bytes_read - bytes_read;
        matches_setup_ut = lqs->c.matches_setup_ut - matches_setup_ut;

        ended_ut = now_monotonic_usec();
        duration_ut = ended_ut - started_ut;

        if(duration_ut > max_duration_ut)
            max_duration_ut = duration_ut;

        buffer_json_add_array_item_object(wb); // channel source
        {
            // information about the file
            buffer_json_member_add_string(wb, "_name", fullname);
            buffer_json_member_add_uint64(wb, "_source_type", src->source_type);
            buffer_json_member_add_string(wb, "_source", string2str(src->source));
            buffer_json_member_add_uint64(wb, "_msg_first_ut", src->msg_first_ut);
            buffer_json_member_add_uint64(wb, "_msg_last_ut", src->msg_last_ut);

            // information about the current use of the file
            buffer_json_member_add_uint64(wb, "duration_ut", ended_ut - started_ut);
            buffer_json_member_add_uint64(wb, "rows_read", rows_read);
            buffer_json_member_add_uint64(wb, "rows_useful", rows_useful);
            buffer_json_member_add_double(wb, "rows_per_second", (double) rows_read / (double) duration_ut * (double) USEC_PER_SEC);
            buffer_json_member_add_uint64(wb, "bytes_read", bytes_read);
            buffer_json_member_add_double(wb, "bytes_per_second", (double) bytes_read / (double) duration_ut * (double) USEC_PER_SEC);
            buffer_json_member_add_uint64(wb, "duration_matches_ut", matches_setup_ut);

            // if(lqs->rq.sampling) {
            //     buffer_json_member_add_object(wb, "_sampling");
            //     {
            //         buffer_json_member_add_uint64(wb, "sampled", lqs->c.samples_per_file.sampled);
            //         buffer_json_member_add_uint64(wb, "unsampled", lqs->c.samples_per_file.unsampled);
            //         buffer_json_member_add_uint64(wb, "estimated", lqs->c.samples_per_file.estimated);
            //     }
            //     buffer_json_object_close(wb); // _sampling
            // }
        }
        buffer_json_object_close(wb); // channel source

        bool stop = false;
        switch(tmp_status) {
            case WEVT_OK:
            case WEVT_NO_CHANNEL_MATCHED:
                status = (status == WEVT_OK) ? WEVT_OK : tmp_status;
                break;

            case WEVT_FAILED_TO_OPEN:
            case WEVT_FAILED_TO_SEEK:
                partial = true;
                if(status == WEVT_NO_CHANNEL_MATCHED)
                    status = tmp_status;
                break;

            case WEVT_CANCELLED:
            case WEVT_TIMED_OUT:
                partial = true;
                stop = true;
                status = tmp_status;
                break;

            case WEVT_NOT_MODIFIED:
                internal_fatal(true, "this should never be returned here");
                break;
        }

        if(stop)
            break;
    }
    buffer_json_array_close(wb); // _channels

    // release the files
    for(size_t f = 0; f < files_used ;f++)
        dictionary_acquired_item_release(wevt_sources, file_items[f]);

    wevt_closelog6(log);

    switch (status) {
        case WEVT_OK:
            if(lqs->rq.if_modified_since && !lqs->c.rows_useful)
                return rrd_call_function_error(wb, "no useful logs, not modified", HTTP_RESP_NOT_MODIFIED);
            break;

        case WEVT_TIMED_OUT:
        case WEVT_NO_CHANNEL_MATCHED:
            break;

        case WEVT_CANCELLED:
            return rrd_call_function_error(wb, "client closed connection", HTTP_RESP_CLIENT_CLOSED_REQUEST);

        case WEVT_NOT_MODIFIED:
            return rrd_call_function_error(wb, "not modified", HTTP_RESP_NOT_MODIFIED);

        case WEVT_FAILED_TO_OPEN:
            return rrd_call_function_error(wb, "failed to open event log", HTTP_RESP_INTERNAL_SERVER_ERROR);

        case WEVT_FAILED_TO_SEEK:
            return rrd_call_function_error(wb, "failed to execute event log query", HTTP_RESP_INTERNAL_SERVER_ERROR);

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

        // if(!journal_files_completed_once()) {
        //     buffer_strcat(msg, "Journals are still being scanned. ");
        //     buffer_strcat(msg_description
        //                   , "LIBRARY SCAN: The journal files are still being scanned, you are probably viewing incomplete data. ");
        //     msg_priority = NDLP_WARNING;
        // }

        if(partial) {
            buffer_strcat(msg, "Query timed-out, incomplete data. ");
            buffer_strcat(msg_description
                          , "QUERY TIMEOUT: The query timed out and may not include all the data of the selected window. ");
            msg_priority = NDLP_WARNING;
        }

        // if(lqs->c.samples.estimated || lqs->c.samples.unsampled) {
        //     double percent = (double) (lqs->c.samples.sampled * 100.0 /
        //                                (lqs->c.samples.estimated + lqs->c.samples.unsampled + lqs->c.samples.sampled));
        //     buffer_sprintf(msg, "%.2f%% real data", percent);
        //     buffer_sprintf(msg_description, "ACTUAL DATA: The filters counters reflect %0.2f%% of the data. ", percent);
        //     msg_priority = MIN(msg_priority, NDLP_NOTICE);
        // }
        //
        // if(lqs->c.samples.unsampled) {
        //     double percent = (double) (lqs->c.samples.unsampled * 100.0 /
        //                                (lqs->c.samples.estimated + lqs->c.samples.unsampled + lqs->c.samples.sampled));
        //     buffer_sprintf(msg, ", %.2f%% unsampled", percent);
        //     buffer_sprintf(msg_description
        //                    , "UNSAMPLED DATA: %0.2f%% of the events exist and have been counted, but their values have not been evaluated, so they are not included in the filters counters. "
        //                    , percent);
        //     msg_priority = MIN(msg_priority, NDLP_NOTICE);
        // }
        //
        // if(lqs->c.samples.estimated) {
        //     double percent = (double) (lqs->c.samples.estimated * 100.0 /
        //                                (lqs->c.samples.estimated + lqs->c.samples.unsampled + lqs->c.samples.sampled));
        //     buffer_sprintf(msg, ", %.2f%% estimated", percent);
        //     buffer_sprintf(msg_description
        //                    , "ESTIMATED DATA: The query selected a large amount of data, so to avoid delaying too much, the presented data are estimated by %0.2f%%. "
        //                    , percent);
        //     msg_priority = MIN(msg_priority, NDLP_NOTICE);
        // }

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
        buffer_json_member_add_string(wb, "help", WEVT_FUNCTION_DESCRIPTION);
    }

    if(!lqs->rq.data_only || lqs->rq.tail)
        buffer_json_member_add_uint64(wb, "last_modified", lqs->last_modified);

    facets_sort_and_reorder_keys(facets);
    facets_report(facets, wb, used_hashes_registry);

    wb->expires = now_realtime_sec() + (lqs->rq.data_only ? 3600 : 0);
    buffer_json_member_add_time_t(wb, "expires", wb->expires);

    // if(lqs->rq.sampling) {
    //     buffer_json_member_add_object(wb, "_sampling");
    //     {
    //         buffer_json_member_add_uint64(wb, "sampled", lqs->c.samples.sampled);
    //         buffer_json_member_add_uint64(wb, "unsampled", lqs->c.samples.unsampled);
    //         buffer_json_member_add_uint64(wb, "estimated", lqs->c.samples.estimated);
    //     }
    //     buffer_json_object_close(wb); // _sampling
    // }

    wb->content_type = CT_APPLICATION_JSON;
    wb->response_code = HTTP_RESP_OK;
    return wb->response_code;
}

void function_windows_events(const char *transaction, char *function, usec_t *stop_monotonic_ut, bool *cancelled,
                             BUFFER *payload, HTTP_ACCESS access __maybe_unused,
                             const char *source __maybe_unused, void *data __maybe_unused) {
    bool have_slice = LQS_DEFAULT_SLICE_MODE;

    LOGS_QUERY_STATUS tmp_fqs = {
            .facets = lqs_facets_create(
                    LQS_DEFAULT_ITEMS_PER_QUERY,
                    FACETS_OPTION_ALL_KEYS_FTS,
                    WEVT_ALWAYS_VISIBLE_KEYS,
                    WEVT_KEYS_INCLUDED_IN_FACETS,
                    WEVT_KEYS_EXCLUDED_FROM_FACETS,
                    have_slice),

            .rq = LOGS_QUERY_REQUEST_DEFAULTS(transaction, have_slice, FACETS_ANCHOR_DIRECTION_BACKWARD),

            .cancelled = cancelled,
            .stop_monotonic_ut = stop_monotonic_ut,
    };
    LOGS_QUERY_STATUS *lqs = &tmp_fqs;

    CLEAN_BUFFER *wb = lqs_create_output_buffer();

    // ------------------------------------------------------------------------
    // parse the parameters

    if(lqs_request_parse_and_validate(lqs, wb, function, payload, have_slice, WEVT_FIELD_LEVEL)) {
        wevt_register_fields(lqs);

        // ------------------------------------------------------------------------
        // add versions to the response

        buffer_json_wevt_versions(wb);

        // ------------------------------------------------------------------------
        // run the request

        if (lqs->rq.info)
            lqs_info_response(wb, lqs->facets);
        else {
            wevt_master_query(wb, lqs);
            if (wb->response_code == HTTP_RESP_OK)
                buffer_json_finalize(wb);
        }
    }

    netdata_mutex_lock(&stdout_mutex);
    pluginsd_function_result_to_stdout(transaction, wb);
    netdata_mutex_unlock(&stdout_mutex);

    lqs_cleanup(lqs);
}

int main(int argc __maybe_unused, char **argv __maybe_unused) {
    clocks_init();
    nd_thread_tag_set("wevt.plugin");
    nd_log_initialize_for_external_plugins("windows-events.plugin");

    // ------------------------------------------------------------------------
    // initialization

    wevt_sources_init();

    // ------------------------------------------------------------------------
    // debug

    if(argc >= 2 && strcmp(argv[argc - 1], "debug") == 0) {
        wevt_sources_scan();

        struct {
            const char *func;
        } array[] = {
            //{ "windows-events after:-86400 before:0 direction:backward last:200 facets:HdUoSYab5wV,Cq2r7mRUv4a,LAnVlsIQfeD,BnPLNbA5VWT,KeCITtVD5AD,HytMJ9kj82B,JM3OPW3kHn6,H106l8MXSSr,HREiMN.4Ahu,ClaDGnYSQE7,ApYltST_icg,PtkRm91M0En data_only:false slice:true source:All" },
            //{ "windows-events after:1726055370 before:1726056270 direction:backward last:200 facets:HdUoSYab5wV,Cq2r7mRUv4a,LAnVlsIQfeD,BnPLNbA5VWT,KeCITtVD5AD,HytMJ9kj82B,LT.Xp9I9tiP,No4kPTQbS.g,LQ2LQzfE8EG,PtkRm91M0En,JM3OPW3kHn6,ClaDGnYSQE7,H106l8MXSSr,HREiMN.4Ahu data_only:false source:All HytMJ9kj82B:BlC24d5JBBV,PtVoyIuX.MU,HMj1B38kHTv KeCITtVD5AD:PY1JtCeWwSe,O9kz5J37nNl,JZoJURadhDb" },
            { "windows-events after:1725636012 before:1726240812 direction:backward last:200 facets:HdUoSYab5wV,Cq2r7mRUv4a,LAnVlsIQfeD,BnPLNbA5VWT,KeCITtVD5AD,HytMJ9kj82B,JM3OPW3kHn6,H106l8MXSSr,HREiMN.4Ahu,ClaDGnYSQE7,ApYltST_icg,PtkRm91M0En data_only:false source:All PtkRm91M0En:LDzHbP5libb" },
            //{ "windows-events after:1725650386 before:1725736786 anchor:1725652420809461 direction:forward last:200 facets:HWNGeY7tg6c,LAnVlsIQfeD,BnPLNbA5VWT,Cq2r7mRUv4a,KeCITtVD5AD,I_Amz_APBm3,HytMJ9kj82B,LT.Xp9I9tiP,No4kPTQbS.g,LQ2LQzfE8EG,PtkRm91M0En,JM3OPW3kHn6 if_modified_since:1725736649011085 data_only:true delta:true tail:true source:all Cq2r7mRUv4a:PPc9fUy.q6o No4kPTQbS.g:Dwo9PhK27v3 HytMJ9kj82B:KbbznGjt_9r LAnVlsIQfeD:OfU1t5cpjgG JM3OPW3kHn6:CS_0g5AEpy2" },
            //{ "windows-events info after:1725650420 before:1725736820" },
            //{ "windows-events after:1725650420 before:1725736820 last:200 facets:HWNGeY7tg6c,LAnVlsIQfeD,BnPLNbA5VWT,Cq2r7mRUv4a,KeCITtVD5AD,I_Amz_APBm3,HytMJ9kj82B,LT.Xp9I9tiP,No4kPTQbS.g,LQ2LQzfE8EG,PtkRm91M0En,JM3OPW3kHn6 source:all Cq2r7mRUv4a:PPc9fUy.q6o No4kPTQbS.g:Dwo9PhK27v3 HytMJ9kj82B:KbbznGjt_9r LAnVlsIQfeD:OfU1t5cpjgG JM3OPW3kHn6:CS_0g5AEpy2" },
            //{ "windows-events after:1725650430 before:1725736830 last:200 facets:HWNGeY7tg6c,LAnVlsIQfeD,BnPLNbA5VWT,Cq2r7mRUv4a,KeCITtVD5AD,I_Amz_APBm3,HytMJ9kj82B,LT.Xp9I9tiP,No4kPTQbS.g,LQ2LQzfE8EG,PtkRm91M0En,JM3OPW3kHn6 source:all Cq2r7mRUv4a:PPc9fUy.q6o No4kPTQbS.g:Dwo9PhK27v3 HytMJ9kj82B:KbbznGjt_9r LAnVlsIQfeD:OfU1t5cpjgG JM3OPW3kHn6:CS_0g5AEpy2" },
            { NULL },
        };

        for(int i = 0; array[i].func ;i++) {
            bool cancelled = false;
            usec_t stop_monotonic_ut = now_monotonic_usec() + 600 * USEC_PER_SEC;
            //char buf[] = "windows-events after:-86400 before:0 direction:backward last:200 data_only:false slice:true source:all";
            function_windows_events("123", (char *)array[i].func, &stop_monotonic_ut, &cancelled, NULL, HTTP_ACCESS_ALL, NULL, NULL);
        }
        printf("\n\nAll done!\n\n");
        fflush(stdout);
        exit(1);
    }

    // ------------------------------------------------------------------------
    // the event loop for functions

    struct functions_evloop_globals *wg =
            functions_evloop_init(WINDOWS_EVENTS_WORKER_THREADS, "WEVT", &stdout_mutex, &plugin_should_exit);

    functions_evloop_add_function(wg,
                                  WEVT_FUNCTION_NAME,
                                  function_windows_events,
                                  WINDOWS_EVENTS_DEFAULT_TIMEOUT,
                                  NULL);

    // ------------------------------------------------------------------------
    // register functions to netdata

    netdata_mutex_lock(&stdout_mutex);

    fprintf(stdout, PLUGINSD_KEYWORD_FUNCTION " GLOBAL \"%s\" %d \"%s\" \"logs\" "HTTP_ACCESS_FORMAT" %d\n",
            WEVT_FUNCTION_NAME, WINDOWS_EVENTS_DEFAULT_TIMEOUT, WEVT_FUNCTION_DESCRIPTION,
            (HTTP_ACCESS_FORMAT_CAST)(HTTP_ACCESS_SIGNED_ID | HTTP_ACCESS_SAME_SPACE | HTTP_ACCESS_SENSITIVE_DATA),
            RRDFUNCTIONS_PRIORITY_DEFAULT);

    fflush(stdout);
    netdata_mutex_unlock(&stdout_mutex);

    // ------------------------------------------------------------------------

    const usec_t step_ut = 100 * USEC_PER_MS;
    usec_t send_newline_ut = 0;
    usec_t since_last_scan_ut = WINDOWS_EVENTS_SCAN_EVERY_USEC * 2; // something big to trigger scanning at start
    const bool tty = isatty(fileno(stdout)) == 1;

    heartbeat_t hb;
    heartbeat_init(&hb);
    while(!plugin_should_exit) {

        if(since_last_scan_ut > WINDOWS_EVENTS_SCAN_EVERY_USEC) {
            wevt_sources_scan();
            since_last_scan_ut = 0;
        }

        usec_t dt_ut = heartbeat_next(&hb, step_ut);
        since_last_scan_ut += dt_ut;
        send_newline_ut += dt_ut;

        if(!tty && send_newline_ut > USEC_PER_SEC) {
            send_newline_and_flush(&stdout_mutex);
            send_newline_ut = 0;
        }
    }

    exit(0);
}
