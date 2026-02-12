// SPDX-License-Identifier: GPL-3.0-or-later

#include "libnetdata/libnetdata.h"
#include "libnetdata/required_dummies.h"

#include "windows-events.h"

netdata_mutex_t stdout_mutex;

static void __attribute__((constructor)) init_mutex(void) {
    netdata_mutex_init(&stdout_mutex);
}

static void __attribute__((destructor)) destroy_mutex(void) {
    netdata_mutex_destroy(&stdout_mutex);
}

static bool plugin_should_exit = false;

#define WEVT_ALWAYS_VISIBLE_KEYS                NULL

#define WEVT_KEYS_EXCLUDED_FROM_FACETS          \
    "|" WEVT_FIELD_MESSAGE                      \
    "|" WEVT_FIELD_XML                          \
    ""

#define WEVT_KEYS_INCLUDED_IN_FACETS            \
    "|" WEVT_FIELD_COMPUTER                     \
    "|" WEVT_FIELD_PROVIDER                     \
    "|" WEVT_FIELD_LEVEL                        \
    "|" WEVT_FIELD_KEYWORDS                     \
    "|" WEVT_FIELD_OPCODE                       \
    "|" WEVT_FIELD_TASK                         \
    "|" WEVT_FIELD_ACCOUNT                      \
    "|" WEVT_FIELD_DOMAIN                       \
    "|" WEVT_FIELD_SID                          \
    ""

#define query_has_fts(lqs) ((lqs)->rq.query != NULL)

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
    FACET_ROW_KEY_VALUE *levelid_rkv = dictionary_get(row->dict, WEVT_FIELD_LEVEL "ID");
    if(!levelid_rkv || levelid_rkv->empty)
        return FACET_ROW_SEVERITY_NORMAL;

    int windows_event_level = str2i(buffer_tostring(levelid_rkv->wb));

    switch (windows_event_level) {
        case WEVT_LEVEL_VERBOSE:
            return FACET_ROW_SEVERITY_DEBUG;

        default:
        case WEVT_LEVEL_INFORMATION:
            return FACET_ROW_SEVERITY_NORMAL;

        case WEVT_LEVEL_WARNING:
            return FACET_ROW_SEVERITY_WARNING;

        case WEVT_LEVEL_ERROR:
        case WEVT_LEVEL_CRITICAL:
            return FACET_ROW_SEVERITY_CRITICAL;
    }
}

struct wevt_bin_data {
    bool rendered;
    WEVT_EVENT ev;
    WEVT_LOG *log;
    EVT_HANDLE hEvent;
    PROVIDER_META_HANDLE *provider;
};

static void wevt_cleanup_bin_data(void *data) {
    struct wevt_bin_data *d = data;

    if(d->hEvent)
        EvtClose(d->hEvent);

    provider_release(d->provider);
    freez(d);
}

static inline void wevt_facets_register_bin_data(WEVT_LOG *log, FACETS *facets, WEVT_EVENT *ev) {
    struct wevt_bin_data *d = mallocz(sizeof(struct wevt_bin_data));

#ifdef NETDATA_INTERNAL_CHECKS
    internal_fatal(strcmp(log->ops.provider.data, provider_get_name(log->provider)) != 0,
                   "Provider name mismatch in data!");

    internal_fatal(!UUIDeq(ev->provider, provider_get_uuid(log->provider)),
                   "Provider UUID mismatch in data!");
#endif

    d->ev = *ev;
    d->log = log;
    d->rendered = false;

    // take the bookmark
    d->hEvent = log->hEvent; log->hEvent = NULL;

    // dup the provider
    d->provider = provider_dup(log->provider);

    facets_row_bin_data_set(facets, wevt_cleanup_bin_data, d);
}

static void wevt_lazy_loading_event_and_xml(struct wevt_bin_data *d, FACET_ROW *row __maybe_unused) {
    if(d->rendered) return;

#ifdef NETDATA_INTERNAL_CHECKS
    const FACET_ROW_KEY_VALUE *provider_rkv = dictionary_get(row->dict, WEVT_FIELD_PROVIDER);
    internal_fatal(!provider_rkv || strcmp(buffer_tostring(provider_rkv->wb), provider_get_name(d->provider)) != 0,
                   "Provider of row does not match the bin data associated with it");

    uint64_t event_record_id = UINT64_MAX;
    const FACET_ROW_KEY_VALUE *event_record_id_rkv = dictionary_get(row->dict, WEVT_FIELD_EVENTRECORDID);
    if(event_record_id_rkv)
        event_record_id = str2uint64_t(buffer_tostring(event_record_id_rkv->wb), NULL);
    internal_fatal(event_record_id != d->ev.id,
                   "Event Record ID of row does not match the bin data associated with it");
#endif

    // the message needs the xml
    EvtFormatMessage_Xml_utf8(&d->log->ops.unicode, d->provider, d->hEvent, &d->log->ops.xml);
    EvtFormatMessage_Event_utf8(&d->log->ops.unicode, d->provider, d->hEvent, &d->log->ops.event);
    d->rendered = true;
}

static void wevt_lazy_load_xml(
        FACETS *facets,
        BUFFER *json_array,
        FACET_ROW_KEY_VALUE *rkv __maybe_unused,
        FACET_ROW *row,
        void *data __maybe_unused) {

    struct wevt_bin_data *d = facets_row_bin_data_get(facets, row);
    if(!d) {
        buffer_json_add_array_item_string(json_array, "Failed to get row BIN DATA from facets");
        return;
    }

    wevt_lazy_loading_event_and_xml(d, row);
    buffer_json_add_array_item_string(json_array, d->log->ops.xml.data);
}

static void wevt_lazy_load_message(
        FACETS *facets,
        BUFFER *json_array,
        FACET_ROW_KEY_VALUE *rkv __maybe_unused,
        FACET_ROW *row,
        void *data __maybe_unused) {

    struct wevt_bin_data *d = facets_row_bin_data_get(facets, row);
    if(!d) {
        buffer_json_add_array_item_string(json_array, "Failed to get row BIN DATA from facets");
        return;
    }

    wevt_lazy_loading_event_and_xml(d, row);

    if(d->log->ops.event.used <= 1) {
        TXT_UTF8 *xml = &d->log->ops.xml;

        buffer_flush(rkv->wb);

        bool added_message = false;
        if(xml->used > 1) {
            const char *message_path[] = {
                    "RenderingInfo",
                    "Message",
                    NULL};

            added_message = buffer_xml_extract_and_print_value(
                    rkv->wb,
                    xml->data, xml->used - 1,
                    NULL,
                    message_path);
        }

        if(!added_message) {
            const FACET_ROW_KEY_VALUE *event_id_rkv = dictionary_get(row->dict, WEVT_FIELD_EVENTID);
            if (event_id_rkv && buffer_strlen(event_id_rkv->wb)) {
                buffer_fast_strcat(rkv->wb, "Event ", 6);
                buffer_fast_strcat(rkv->wb, buffer_tostring(event_id_rkv->wb), buffer_strlen(event_id_rkv->wb));
            } else
                buffer_strcat(rkv->wb, "Unknown Event ");

            const FACET_ROW_KEY_VALUE *provider_rkv = dictionary_get(row->dict, WEVT_FIELD_PROVIDER);
            if (provider_rkv && buffer_strlen(provider_rkv->wb)) {
                buffer_fast_strcat(rkv->wb, " of ", 4);
                buffer_fast_strcat(rkv->wb, buffer_tostring(provider_rkv->wb), buffer_strlen(provider_rkv->wb));
                buffer_putc(rkv->wb, '.');
            } else
                buffer_strcat(rkv->wb, "of unknown Provider.");
        }

        if(xml->used > 1) {
            const char *event_path[] = {
                    "EventData",
                    NULL
            };
            bool added_event_data = buffer_extract_and_print_xml(
                    rkv->wb,
                    xml->data, xml->used - 1,
                    "\n\nRelated event data:\n",
                    event_path);

            const char *user_path[] = {
                    "UserData",
                    NULL
            };
            bool added_user_data = buffer_extract_and_print_xml(
                    rkv->wb,
                    xml->data, xml->used - 1,
                    "\n\nRelated user data:\n",
                    user_path);

            if(!added_event_data && !added_user_data)
                buffer_strcat(rkv->wb, " Without any related data.");
        }

        buffer_json_add_array_item_string(json_array, buffer_tostring(rkv->wb));
    }
    else
        buffer_json_add_array_item_string(json_array, d->log->ops.event.data);
}

static void wevt_register_fields(LOGS_QUERY_STATUS *lqs) {
    // the order of the fields here, controls the order of the fields at the table presented

    FACETS *facets = lqs->facets;
    LOGS_QUERY_REQUEST *rq = &lqs->rq;

    facets_register_row_severity(facets, wevt_levelid_to_facet_severity, NULL);

    facets_register_key_name(
            facets, WEVT_FIELD_COMPUTER,
            rq->default_facet | FACET_KEY_OPTION_VISIBLE);

    facets_register_key_name(
            facets, WEVT_FIELD_CHANNEL,
            rq->default_facet | FACET_KEY_OPTION_FTS);

    facets_register_key_name(
            facets, WEVT_FIELD_PROVIDER,
            rq->default_facet | FACET_KEY_OPTION_VISIBLE | FACET_KEY_OPTION_FTS);

    facets_register_key_name(
            facets, WEVT_FIELD_ACCOUNT,
            rq->default_facet | FACET_KEY_OPTION_FTS);

    facets_register_key_name(
            facets, WEVT_FIELD_DOMAIN,
            rq->default_facet | FACET_KEY_OPTION_FTS);

    facets_register_key_name(
            facets, WEVT_FIELD_SID,
            rq->default_facet | FACET_KEY_OPTION_FTS);

    facets_register_key_name(
            facets, WEVT_FIELD_EVENTID,
            rq->default_facet |
            FACET_KEY_OPTION_VISIBLE | FACET_KEY_OPTION_FTS);

    facets_register_key_name(
        facets, WEVT_FIELD_EVENTS_API,
        rq->default_facet |
            FACET_KEY_OPTION_FTS);

    facets_register_key_name(
            facets, WEVT_FIELD_LEVEL,
            rq->default_facet | FACET_KEY_OPTION_FTS | FACET_KEY_OPTION_EXPANDED_FILTER);

    facets_register_key_name(
            facets, WEVT_FIELD_LEVEL "ID",
            FACET_KEY_OPTION_NONE);

    facets_register_key_name(
            facets, WEVT_FIELD_PROCESSID,
            FACET_KEY_OPTION_FTS);

    facets_register_key_name(
            facets, WEVT_FIELD_THREADID,
            FACET_KEY_OPTION_FTS);

    facets_register_key_name(
            facets, WEVT_FIELD_TASK,
            rq->default_facet | FACET_KEY_OPTION_FTS | FACET_KEY_OPTION_VISIBLE);

    facets_register_key_name(
            facets, WEVT_FIELD_TASK "ID",
            FACET_KEY_OPTION_NONE);

    facets_register_key_name(
            facets, WEVT_FIELD_OPCODE,
            rq->default_facet | FACET_KEY_OPTION_FTS | FACET_KEY_OPTION_VISIBLE);

    facets_register_key_name(
            facets, WEVT_FIELD_OPCODE "ID",
            FACET_KEY_OPTION_NONE);

    facets_register_key_name(
        facets, WEVT_FIELD_KEYWORDS,
        rq->default_facet | FACET_KEY_OPTION_FTS);

    facets_register_key_name(
            facets, WEVT_FIELD_KEYWORDS "ID",
            FACET_KEY_OPTION_NONE);

    facets_register_dynamic_key_name(
        facets,
        WEVT_FIELD_MESSAGE,
        FACET_KEY_OPTION_NEVER_FACET | FACET_KEY_OPTION_MAIN_TEXT | FACET_KEY_OPTION_VISIBLE,
        wevt_lazy_load_message,
        NULL);

    facets_register_dynamic_key_name(
        facets,
        WEVT_FIELD_XML,
        FACET_KEY_OPTION_NEVER_FACET | FACET_KEY_OPTION_PRETTY_XML,
        wevt_lazy_load_xml,
        NULL);

    if(query_has_fts(lqs)) {
        facets_register_key_name(
                facets, WEVT_FIELD_EVENT_MESSAGE_HIDDEN,
            FACET_KEY_OPTION_FTS | FACET_KEY_OPTION_HIDDEN | FACET_KEY_OPTION_NEVER_FACET);

        facets_register_key_name(
                facets, WEVT_FIELD_EVENT_XML_HIDDEN,
                FACET_KEY_OPTION_FTS | FACET_KEY_OPTION_HIDDEN | FACET_KEY_OPTION_NEVER_FACET);

        facets_register_key_name(
            facets, WEVT_FIELD_EVENT_DATA_HIDDEN,
            FACET_KEY_OPTION_FTS | FACET_KEY_OPTION_HIDDEN | FACET_KEY_OPTION_NEVER_FACET);
    }

#ifdef NETDATA_INTERNAL_CHECKS
    facets_register_key_name(
        facets, "z_level_source",
        rq->default_facet);

    facets_register_key_name(
        facets, "z_keywords_source",
        rq->default_facet);

    facets_register_key_name(
        facets, "z_opcode_source",
        rq->default_facet);

    facets_register_key_name(
        facets, "z_task_source",
        rq->default_facet);
#endif
}

#ifdef NETDATA_INTERNAL_CHECKS
static const char *source_to_str(TXT_UTF8 *txt) {
    switch(txt->src) {
        default:
        case TXT_SOURCE_UNKNOWN:
            return "unknown";

        case TXT_SOURCE_EVENT_LOG:
            return "event-log";

        case TXT_SOURCE_PROVIDER:
            return "provider";

        case TXT_SOURCE_FIELD_CACHE:
            return "fields-cache";

        case TXT_SOURCE_HARDCODED:
            return "hardcoded";
    }
}
#endif

static const char *events_api_to_str(WEVT_PROVIDER_PLATFORM platform) {
    switch(platform) {
        case WEVT_PLATFORM_WEL:
            return "Windows Event Log";

        case WEVT_PLATFORM_ETW:
            return "Event Tracing for Windows";

        case WEVT_PLATFORM_TL:
            return "TraceLogging";

        default:
            return "Unknown";
    }
}

static inline size_t wevt_process_event(WEVT_LOG *log, FACETS *facets, LOGS_QUERY_SOURCE *src, usec_t *msg_ut __maybe_unused, WEVT_EVENT *ev) {
    static __thread char uuid_str[UUID_STR_LEN];

    size_t len, bytes = log->ops.raw.system.used + log->ops.raw.user.used;

    if(!UUIDiszero(ev->provider)) {
        uuid_unparse_lower(ev->provider.uuid, uuid_str);
        facets_add_key_value_length(
            facets, WEVT_FIELD_PROVIDER_GUID, sizeof(WEVT_FIELD_PROVIDER_GUID) - 1,
            uuid_str, sizeof(uuid_str) - 1);
    }

    if(!UUIDiszero(ev->activity_id)) {
        uuid_unparse_lower(ev->activity_id.uuid, uuid_str);
        facets_add_key_value_length(
            facets, WEVT_FIELD_ACTIVITY_ID, sizeof(WEVT_FIELD_ACTIVITY_ID) - 1,
            uuid_str, sizeof(uuid_str) - 1);
    }

    if(!UUIDiszero(ev->related_activity_id)) {
        uuid_unparse_lower(ev->related_activity_id.uuid, uuid_str);
        facets_add_key_value_length(
            facets, WEVT_FIELD_RELATED_ACTIVITY_ID, sizeof(WEVT_FIELD_RELATED_ACTIVITY_ID) - 1,
            uuid_str, sizeof(uuid_str) - 1);
    }

    if(ev->qualifiers) {
        static __thread char qualifiers[UINT64_HEX_MAX_LENGTH];
        len = print_uint64_hex(qualifiers, ev->qualifiers);
        bytes += len;
        facets_add_key_value_length(
            facets, WEVT_FIELD_QUALIFIERS, sizeof(WEVT_FIELD_QUALIFIERS) - 1,
            qualifiers, len);
    }

    {
        static __thread char event_record_id_str[UINT64_MAX_LENGTH];
        len = print_uint64(event_record_id_str, ev->id);
        bytes += len;
        facets_add_key_value_length(
            facets, WEVT_FIELD_EVENTRECORDID, sizeof(WEVT_FIELD_EVENTRECORDID) - 1,
            event_record_id_str, len);
    }

    if(ev->version) {
        static __thread char version[UINT64_MAX_LENGTH];
        len = print_uint64(version, ev->version);
        bytes += len;
        facets_add_key_value_length(
            facets, WEVT_FIELD_VERSION, sizeof(WEVT_FIELD_VERSION) - 1,
            version, len);
    }

    if(log->ops.provider.used > 1) {
        bytes += log->ops.provider.used * 2; // unicode is double
        facets_add_key_value_length(
                facets, WEVT_FIELD_PROVIDER, sizeof(WEVT_FIELD_PROVIDER) - 1,
                log->ops.provider.data, log->ops.provider.used - 1);
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

    if(log->ops.level.used > 1) {
        bytes += log->ops.level.used * 2;
        facets_add_key_value_length(
                facets, WEVT_FIELD_LEVEL, sizeof(WEVT_FIELD_LEVEL) - 1,
                log->ops.level.data, log->ops.level.used - 1);
    }

    if(log->ops.computer.used > 1) {
        bytes += log->ops.computer.used * 2;
        facets_add_key_value_length(
            facets, WEVT_FIELD_COMPUTER, sizeof(WEVT_FIELD_COMPUTER) - 1,
            log->ops.computer.data, log->ops.computer.used - 1);
    }

    if(log->ops.opcode.used > 1) {
        bytes += log->ops.opcode.used * 2;
        facets_add_key_value_length(
                facets, WEVT_FIELD_OPCODE, sizeof(WEVT_FIELD_OPCODE) - 1,
                log->ops.opcode.data, log->ops.opcode.used - 1);
    }

    if(log->ops.keywords.used > 1) {
        bytes += log->ops.keywords.used * 2;
        facets_add_key_value_length(
                facets, WEVT_FIELD_KEYWORDS, sizeof(WEVT_FIELD_KEYWORDS) - 1,
                log->ops.keywords.data, log->ops.keywords.used - 1);
    }

    if(log->ops.task.used > 1) {
        bytes += log->ops.task.used * 2;
        facets_add_key_value_length(
                facets, WEVT_FIELD_TASK, sizeof(WEVT_FIELD_TASK) - 1,
                log->ops.task.data, log->ops.task.used - 1);
    }

    if(log->ops.account.used > 1) {
        bytes += log->ops.account.used * 2;
        facets_add_key_value_length(
            facets,
            WEVT_FIELD_ACCOUNT, sizeof(WEVT_FIELD_ACCOUNT) - 1,
            log->ops.account.data, log->ops.account.used - 1);
    }

    if(log->ops.domain.used > 1) {
        bytes += log->ops.domain.used * 2;
        facets_add_key_value_length(
            facets,
            WEVT_FIELD_DOMAIN, sizeof(WEVT_FIELD_DOMAIN) - 1,
            log->ops.domain.data, log->ops.domain.used - 1);
    }

    if(log->ops.sid.used > 1) {
        bytes += log->ops.sid.used * 2;
        facets_add_key_value_length(
            facets,
            WEVT_FIELD_SID, sizeof(WEVT_FIELD_SID) - 1,
            log->ops.sid.data, log->ops.sid.used - 1);
    }

    {
        static __thread char event_id_str[UINT64_MAX_LENGTH];
        len = print_uint64(event_id_str, ev->event_id);
        bytes += len;
        facets_add_key_value_length(
            facets, WEVT_FIELD_EVENTID, sizeof(WEVT_FIELD_EVENTID) - 1,
            event_id_str, len);
    }

    {
        const char *s = events_api_to_str(ev->platform);
        facets_add_key_value_length(
            facets, WEVT_FIELD_EVENTS_API, sizeof(WEVT_FIELD_EVENTS_API) - 1, s, strlen(s));
    }

    if(ev->process_id) {
        static __thread char process_id_str[UINT64_MAX_LENGTH];
        len = print_uint64(process_id_str, ev->process_id);
        bytes += len;
        facets_add_key_value_length(
            facets, WEVT_FIELD_PROCESSID, sizeof(WEVT_FIELD_PROCESSID) - 1,
            process_id_str, len);
    }

    if(ev->thread_id) {
        static __thread char thread_id_str[UINT64_MAX_LENGTH];
        len = print_uint64(thread_id_str, ev->thread_id);
        bytes += len;
        facets_add_key_value_length(
            facets, WEVT_FIELD_THREADID, sizeof(WEVT_FIELD_THREADID) - 1,
            thread_id_str, len);
    }

    {
        static __thread char str[UINT64_MAX_LENGTH];
        len = print_uint64(str, ev->level);
        bytes += len;
        facets_add_key_value_length(
            facets, WEVT_FIELD_LEVEL "ID", sizeof(WEVT_FIELD_LEVEL) + 2 - 1, str, len);
    }

    {
        static __thread char str[UINT64_HEX_MAX_LENGTH];
        len = print_uint64_hex_full(str, ev->keywords);
        bytes += len;
        facets_add_key_value_length(
            facets, WEVT_FIELD_KEYWORDS "ID", sizeof(WEVT_FIELD_KEYWORDS) + 2 - 1, str, len);
    }

    {
        static __thread char str[UINT64_MAX_LENGTH];
        len = print_uint64(str, ev->opcode);
        bytes += len;
        facets_add_key_value_length(
            facets, WEVT_FIELD_OPCODE "ID", sizeof(WEVT_FIELD_OPCODE) + 2 - 1, str, len);
    }

    {
        static __thread char str[UINT64_MAX_LENGTH];
        len = print_uint64(str, ev->task);
        bytes += len;
        facets_add_key_value_length(
            facets, WEVT_FIELD_TASK "ID", sizeof(WEVT_FIELD_TASK) + 2 - 1, str, len);
    }

    if(log->type & WEVT_QUERY_EVENT_DATA) {
        // the query has full text-search
        if(log->ops.event.used > 1) {
            bytes += log->ops.event.used;
            facets_add_key_value_length(
                    facets, WEVT_FIELD_EVENT_MESSAGE_HIDDEN, sizeof(WEVT_FIELD_EVENT_MESSAGE_HIDDEN) - 1,
                    log->ops.event.data, log->ops.event.used - 1);
        }

        if(log->ops.xml.used > 1) {
            bytes += log->ops.xml.used;
            facets_add_key_value_length(
                    facets, WEVT_FIELD_EVENT_XML_HIDDEN, sizeof(WEVT_FIELD_EVENT_XML_HIDDEN) - 1,
                    log->ops.xml.data, log->ops.xml.used - 1);
        }

        if(log->ops.event_data->len) {
            bytes += log->ops.event_data->len;
            facets_add_key_value_length(
                facets, WEVT_FIELD_EVENT_DATA_HIDDEN, sizeof(WEVT_FIELD_EVENT_DATA_HIDDEN) - 1,
                buffer_tostring(log->ops.event_data), buffer_strlen(log->ops.event_data));
        }
    }

    wevt_facets_register_bin_data(log, facets, ev);

#ifdef NETDATA_INTERNAL_CHECKS
    facets_add_key_value(facets, "z_level_source", source_to_str(&log->ops.level));
    facets_add_key_value(facets, "z_keywords_source", source_to_str(&log->ops.keywords));
    facets_add_key_value(facets, "z_opcode_source", source_to_str(&log->ops.opcode));
    facets_add_key_value(facets, "z_task_source", source_to_str(&log->ops.task));
#endif

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

    if(!wevt_query(log, channel2unicode(src->fullname), lqs->c.query, EvtQueryReverseDirection))
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
    while (status == WEVT_OK && wevt_get_next_event(log, &e)) {
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

    if(!wevt_query(log, channel2unicode(src->fullname), lqs->c.query, EvtQueryForwardDirection))
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
    while (status == WEVT_OK && wevt_get_next_event(log, &e)) {
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
    if(
        // no source is requested
        (lqs->rq.source_type == WEVTS_NONE && !lqs->rq.sources) ||

        // matches our internal source types
        (src->source_type & lqs->rq.source_type) ||

        // matches the source name
        (lqs->rq.sources && src->source && simple_pattern_matches(lqs->rq.sources, string2str(src->source))) ||

        // matches the provider (providers start with a special prefix to avoid mix and match)
        (lqs->rq.sources && src->provider && simple_pattern_matches(lqs->rq.sources, string2str(src->provider)))

        ) {

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

static int wevt_master_query(BUFFER *wb __maybe_unused, LOGS_QUERY_STATUS *lqs __maybe_unused) {
    // make sure the sources list is updated
    wevt_sources_scan();

    lqs->c.query = wevt_generate_query_no_xpath(lqs, wb);
    if(!lqs->c.query)
        return rrd_call_function_error(wb, "failed to generate query", HTTP_RESP_INTERNAL_SERVER_ERROR);

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

    WEVT_LOG *log = wevt_openlog6(query_has_fts(lqs) ? WEVT_QUERY_FTS : WEVT_QUERY_NORMAL);
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

    wevt_closelog6(log);

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
                    FACETS_OPTION_ALL_KEYS_FTS | FACETS_OPTION_HASH_IDS,
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
    nd_thread_tag_set("wevt.plugin");
    nd_log_initialize_for_external_plugins("windows-events.plugin");
    netdata_threads_init_for_external_plugins(0);

    // ------------------------------------------------------------------------
    // initialization

    wevt_sources_init();
    provider_cache_init();
    cached_sid_username_init();
    field_cache_init();

    if(!EnableWindowsPrivilege(SE_SECURITY_NAME))
        nd_log(NDLS_COLLECTORS, NDLP_WARNING, "Failed to enable %s privilege", SE_SECURITY_NAME);

    if(!EnableWindowsPrivilege(SE_BACKUP_NAME))
        nd_log(NDLS_COLLECTORS, NDLP_WARNING, "Failed to enable %s privilege", SE_BACKUP_NAME);

    if(!EnableWindowsPrivilege(SE_AUDIT_NAME))
        nd_log(NDLS_COLLECTORS, NDLP_WARNING, "Failed to enable %s privilege", SE_AUDIT_NAME);

    // ------------------------------------------------------------------------
    // debug

    if(argc >= 2 && strcmp(argv[argc - 1], "debug") == 0) {
        wevt_sources_scan();

        struct {
            const char *func;
        } array[] = {
            { "windows-events after:-8640000 before:0 last:200 source:All" },
            //{ "windows-events after:-86400 before:0 direction:backward last:200 facets:HdUoSYab5wV,Cq2r7mRUv4a,LAnVlsIQfeD,BnPLNbA5VWT,KeCITtVD5AD,HytMJ9kj82B,JM3OPW3kHn6,H106l8MXSSr,HREiMN.4Ahu,ClaDGnYSQE7,ApYltST_icg,PtkRm91M0En data_only:false slice:true source:All" },
            //{ "windows-events after:1726055370 before:1726056270 direction:backward last:200 facets:HdUoSYab5wV,Cq2r7mRUv4a,LAnVlsIQfeD,BnPLNbA5VWT,KeCITtVD5AD,HytMJ9kj82B,LT.Xp9I9tiP,No4kPTQbS.g,LQ2LQzfE8EG,PtkRm91M0En,JM3OPW3kHn6,ClaDGnYSQE7,H106l8MXSSr,HREiMN.4Ahu data_only:false source:All HytMJ9kj82B:BlC24d5JBBV,PtVoyIuX.MU,HMj1B38kHTv KeCITtVD5AD:PY1JtCeWwSe,O9kz5J37nNl,JZoJURadhDb" },
            // { "windows-events after:1725636012 before:1726240812 direction:backward last:200 facets:HdUoSYab5wV,Cq2r7mRUv4a,LAnVlsIQfeD,BnPLNbA5VWT,KeCITtVD5AD,HytMJ9kj82B,JM3OPW3kHn6,H106l8MXSSr,HREiMN.4Ahu,ClaDGnYSQE7,ApYltST_icg,PtkRm91M0En data_only:false source:All PtkRm91M0En:LDzHbP5libb" },
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
            functions_evloop_init(WINDOWS_EVENTS_WORKER_THREADS, "WEVT", &stdout_mutex, &plugin_should_exit, NULL);

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

    usec_t send_newline_ut = 0;
    usec_t since_last_scan_ut = WINDOWS_EVENTS_SCAN_EVERY_USEC * 2; // something big to trigger scanning at start
    usec_t since_last_providers_release_ut = 0;
    const bool tty = isatty(fileno(stdout)) == 1;

    heartbeat_t hb;
    heartbeat_init(&hb, USEC_PER_SEC);
    while(!__atomic_load_n(&plugin_should_exit, __ATOMIC_ACQUIRE)) {

        if(since_last_scan_ut > WINDOWS_EVENTS_SCAN_EVERY_USEC) {
            wevt_sources_scan();
            since_last_scan_ut = 0;
        }

        if(since_last_providers_release_ut > WINDOWS_EVENTS_RELEASE_PROVIDERS_HANDLES_EVERY_UT) {
            providers_release_unused_handles();
            since_last_providers_release_ut = 0;
        }

        usec_t dt_ut = heartbeat_next(&hb);
        since_last_providers_release_ut += dt_ut;
        since_last_scan_ut += dt_ut;
        send_newline_ut += dt_ut;

        if(!tty && send_newline_ut > USEC_PER_SEC) {
            send_newline_and_flush(&stdout_mutex);
            send_newline_ut = 0;
        }
    }

    exit(0);
}
