// SPDX-License-Identifier: GPL-3.0-or-later

#include "macos-logs.h"

#include <errno.h>
#include <string.h>

#import <Foundation/Foundation.h>
#import <OSLog/OSLog.h>

#define MACOS_LOGS_PREDICATE_PROBE_ROWS 2000

static inline bool macos_logs_check_stop(const LOGS_QUERY_STATUS *lqs) {
    if(lqs->cancelled && __atomic_load_n(lqs->cancelled, __ATOMIC_RELAXED))
        return true;

    return now_monotonic_usec() > __atomic_load_n(lqs->stop_monotonic_ut, __ATOMIC_RELAXED);
}

static inline usec_t macos_logs_entry_time_ut(OSLogEntry *entry) {
    NSDate *date = entry.date;
    if(!date)
        return 0;

    NSTimeInterval seconds = [date timeIntervalSince1970];
    if(seconds <= 0)
        return 0;

    return (usec_t)(seconds * (NSTimeInterval)USEC_PER_SEC);
}

static inline NSDate *macos_logs_date_from_ut(usec_t ut) {
    return [NSDate dateWithTimeIntervalSince1970:(NSTimeInterval)ut / (NSTimeInterval)USEC_PER_SEC];
}

typedef struct macos_logs_cstring_view {
    const char *value;
    size_t length;
} MACOS_LOGS_CSTRING_VIEW;

static inline MACOS_LOGS_CSTRING_VIEW macos_logs_empty_cstring_view(void) {
    return (MACOS_LOGS_CSTRING_VIEW){ .value = NULL, .length = 0 };
}

static inline MACOS_LOGS_CSTRING_VIEW macos_logs_cstring_view(const char *value, size_t length) {
    if(!value || !*value || !length)
        return macos_logs_empty_cstring_view();

    return (MACOS_LOGS_CSTRING_VIEW){ .value = value, .length = length };
}

#define MACOS_LOGS_CSTRING_LITERAL(value) macos_logs_cstring_view(value, MACOS_LOGS_FIELD_LENGTH(value))

static inline MACOS_LOGS_CSTRING_VIEW macos_logs_string_view(NSString *value) {
    if(!value || value.length == 0)
        return macos_logs_empty_cstring_view();

    const char *utf8 = value.UTF8String;
    // UTF8String is NUL-terminated; strlen avoids a second Foundation encoding pass.
    return macos_logs_cstring_view(utf8, utf8 ? strlen(utf8) : 0); // NOSONAR - NSString.UTF8String is NUL-terminated.
}

static inline const char *macos_logs_string(NSString *value) {
    return macos_logs_string_view(value).value;
}

static inline const char *macos_logs_error_domain(NSError *error) {
    const char *domain = macos_logs_string(error.domain);
    return domain ? domain : "unknown";
}

static inline void macos_logs_add_cstring_view(FACETS *facets, const char *key, size_t key_length,
                                               MACOS_LOGS_CSTRING_VIEW value) {
    if(value.value && *value.value && value.length)
        facets_add_key_value_length(facets, key, key_length, value.value, value.length);
}

static inline void macos_logs_add_facet_cstring(
    FACETS *facets, const char *key, size_t key_length, MACOS_LOGS_FACET_VALUE_CACHE_ID cache_id,
    MACOS_LOGS_CSTRING_VIEW value) {
    if(value.value && *value.value && value.length) {
        facets_add_key_value_length(facets, key, key_length, value.value, value.length);
        macos_logs_cache_facet_value(cache_id, value.value, value.length);
    }
}

static inline void macos_logs_add_string(FACETS *facets, const char *key, size_t key_length, NSString *value) {
    macos_logs_add_cstring_view(facets, key, key_length, macos_logs_string_view(value));
}

static inline void macos_logs_add_facet_string(
    FACETS *facets, const char *key, size_t key_length, MACOS_LOGS_FACET_VALUE_CACHE_ID cache_id, NSString *value) {
    macos_logs_add_facet_cstring(facets, key, key_length, cache_id, macos_logs_string_view(value));
}

static inline void macos_logs_add_uint64(FACETS *facets, const char *key, size_t key_length, uint64_t value) {
    char buffer[32];
    int length = snprintfz(buffer, sizeof(buffer), "%" PRIu64, value);
    macos_logs_add_cstring_view(facets, key, key_length, macos_logs_cstring_view(buffer, length > 0 ? (size_t)length : 0));
}

static inline void macos_logs_add_facet_uint64(
    FACETS *facets, const char *key, size_t key_length, MACOS_LOGS_FACET_VALUE_CACHE_ID cache_id, uint64_t value) {
    char buffer[32];
    int length = snprintfz(buffer, sizeof(buffer), "%" PRIu64, value);
    macos_logs_add_facet_cstring(
        facets, key, key_length, cache_id, macos_logs_cstring_view(buffer, length > 0 ? (size_t)length : 0));
}

static inline MACOS_LOGS_CSTRING_VIEW macos_logs_level_name(OSLogEntryLogLevel level) {
    switch(level) {
        case OSLogEntryLogLevelDebug:
            return MACOS_LOGS_CSTRING_LITERAL("Debug");
        case OSLogEntryLogLevelInfo:
            return MACOS_LOGS_CSTRING_LITERAL("Info");
        case OSLogEntryLogLevelNotice:
            return MACOS_LOGS_CSTRING_LITERAL("Notice");
        case OSLogEntryLogLevelError:
            return MACOS_LOGS_CSTRING_LITERAL("Error");
        case OSLogEntryLogLevelFault:
            return MACOS_LOGS_CSTRING_LITERAL("Fault");
        case OSLogEntryLogLevelUndefined:
        default:
            return MACOS_LOGS_CSTRING_LITERAL("Undefined");
    }
}

static inline MACOS_LOGS_CSTRING_VIEW macos_logs_store_category_name(OSLogEntryStoreCategory category) {
    switch(category) {
        case OSLogEntryStoreCategoryMetadata:
            return MACOS_LOGS_CSTRING_LITERAL("Metadata");
        case OSLogEntryStoreCategoryShortTerm:
            return MACOS_LOGS_CSTRING_LITERAL("ShortTerm");
        case OSLogEntryStoreCategoryLongTermAuto:
            return MACOS_LOGS_CSTRING_LITERAL("LongTermAuto");
        case OSLogEntryStoreCategoryLongTerm1:
            return MACOS_LOGS_CSTRING_LITERAL("LongTerm1");
        case OSLogEntryStoreCategoryLongTerm3:
            return MACOS_LOGS_CSTRING_LITERAL("LongTerm3");
        case OSLogEntryStoreCategoryLongTerm7:
            return MACOS_LOGS_CSTRING_LITERAL("LongTerm7");
        case OSLogEntryStoreCategoryLongTerm14:
            return MACOS_LOGS_CSTRING_LITERAL("LongTerm14");
        case OSLogEntryStoreCategoryLongTerm30:
            return MACOS_LOGS_CSTRING_LITERAL("LongTerm30");
        case OSLogEntryStoreCategoryUndefined:
        default:
            return MACOS_LOGS_CSTRING_LITERAL("Undefined");
    }
}

static inline MACOS_LOGS_CSTRING_VIEW macos_logs_signpost_type_name(OSLogEntrySignpostType type) {
    switch(type) {
        case OSLogEntrySignpostTypeIntervalBegin:
            return MACOS_LOGS_CSTRING_LITERAL("IntervalBegin");
        case OSLogEntrySignpostTypeIntervalEnd:
            return MACOS_LOGS_CSTRING_LITERAL("IntervalEnd");
        case OSLogEntrySignpostTypeEvent:
            return MACOS_LOGS_CSTRING_LITERAL("Event");
        case OSLogEntrySignpostTypeUndefined:
        default:
            return MACOS_LOGS_CSTRING_LITERAL("Undefined");
    }
}

static inline MACOS_LOGS_CSTRING_VIEW macos_logs_entry_type(OSLogEntry *entry) {
    if([entry isKindOfClass:[OSLogEntryLog class]])
        return MACOS_LOGS_CSTRING_LITERAL("Log");
    if([entry isKindOfClass:[OSLogEntryActivity class]])
        return MACOS_LOGS_CSTRING_LITERAL("Activity");
    if([entry isKindOfClass:[OSLogEntrySignpost class]])
        return MACOS_LOGS_CSTRING_LITERAL("Signpost");
    if([entry isKindOfClass:[OSLogEntryBoundary class]])
        return MACOS_LOGS_CSTRING_LITERAL("Boundary");

    return MACOS_LOGS_CSTRING_LITERAL("Entry");
}

static inline bool macos_logs_progress_by_time(
    const LOGS_QUERY_STATUS *lqs, bool forward, usec_t raw_msg_ut, size_t *done) {
    if(forward) {
        if(lqs->query.stop_ut <= lqs->query.start_ut)
            return false;

        if(raw_msg_ut <= lqs->query.start_ut) {
            *done = 0;
            return true;
        }

        if(raw_msg_ut >= lqs->query.stop_ut) {
            *done = MACOS_LOGS_PROGRESS_TOTAL;
            return true;
        }

        long double progress =
            (long double)(raw_msg_ut - lqs->query.start_ut) / (long double)(lqs->query.stop_ut - lqs->query.start_ut);
        *done = (size_t)(progress * (long double)MACOS_LOGS_PROGRESS_TOTAL);
        return true;
    }

    if(lqs->query.start_ut <= lqs->query.stop_ut)
        return false;

    if(raw_msg_ut >= lqs->query.start_ut) {
        *done = 0;
        return true;
    }

    if(raw_msg_ut <= lqs->query.stop_ut) {
        *done = MACOS_LOGS_PROGRESS_TOTAL;
        return true;
    }

    long double progress =
        (long double)(lqs->query.start_ut - raw_msg_ut) / (long double)(lqs->query.start_ut - lqs->query.stop_ut);
    *done = (size_t)(progress * (long double)MACOS_LOGS_PROGRESS_TOTAL);
    return true;
}

static bool macos_logs_filter_requires_detail_fields(FACETS *facets, const char *key) {
    return facets_key_name_is_filter(facets, key);
}

static bool macos_logs_query_requires_detail_fields(LOGS_QUERY_STATUS *lqs) {
    if(lqs->rq.query)
        return true;

    if(lqs->rq.default_facet == FACET_KEY_OPTION_NONE)
        return true;

    return macos_logs_filter_requires_detail_fields(lqs->facets, MACOS_LOGS_FIELD_MESSAGE) ||
           macos_logs_filter_requires_detail_fields(lqs->facets, MACOS_LOGS_FIELD_LEVEL_ID) ||
           macos_logs_filter_requires_detail_fields(lqs->facets, MACOS_LOGS_FIELD_PID) ||
           macos_logs_filter_requires_detail_fields(lqs->facets, MACOS_LOGS_FIELD_THREAD_ID) ||
           macos_logs_filter_requires_detail_fields(lqs->facets, MACOS_LOGS_FIELD_ACTIVITY_ID) ||
           macos_logs_filter_requires_detail_fields(lqs->facets, MACOS_LOGS_FIELD_PARENT_ACTIVITY_ID) ||
           macos_logs_filter_requires_detail_fields(lqs->facets, MACOS_LOGS_FIELD_FORMAT_STRING) ||
           macos_logs_filter_requires_detail_fields(lqs->facets, MACOS_LOGS_FIELD_SIGNPOST_ID);
}

static size_t macos_logs_process_entry(FACETS *facets, OSLogEntry *entry, bool need_details) {
    size_t bytes = 0;

    if(need_details) {
        MACOS_LOGS_CSTRING_VIEW message = macos_logs_string_view(entry.composedMessage);
        macos_logs_add_cstring_view(
            facets, MACOS_LOGS_FIELD_MESSAGE, MACOS_LOGS_FIELD_LENGTH(MACOS_LOGS_FIELD_MESSAGE), message);
        bytes += message.length;
    }

    macos_logs_add_facet_cstring(
        facets,
        MACOS_LOGS_FIELD_ENTRY_TYPE,
        MACOS_LOGS_FIELD_LENGTH(MACOS_LOGS_FIELD_ENTRY_TYPE),
        MACOS_LOGS_FACET_VALUE_CACHE_ENTRY_TYPE,
        macos_logs_entry_type(entry));
    macos_logs_add_facet_cstring(
        facets,
        MACOS_LOGS_FIELD_STORE_CATEGORY,
        MACOS_LOGS_FIELD_LENGTH(MACOS_LOGS_FIELD_STORE_CATEGORY),
        MACOS_LOGS_FACET_VALUE_CACHE_STORE_CATEGORY,
        macos_logs_store_category_name(entry.storeCategory));

    if([entry isKindOfClass:[OSLogEntryLog class]]) {
        OSLogEntryLog *log_entry = (OSLogEntryLog *)entry;
        OSLogEntryLogLevel level = log_entry.level;

        macos_logs_add_facet_cstring(
            facets,
            MACOS_LOGS_FIELD_LEVEL,
            MACOS_LOGS_FIELD_LENGTH(MACOS_LOGS_FIELD_LEVEL),
            MACOS_LOGS_FACET_VALUE_CACHE_LEVEL,
            macos_logs_level_name(level));
        if(need_details)
            macos_logs_add_uint64(
                facets, MACOS_LOGS_FIELD_LEVEL_ID, MACOS_LOGS_FIELD_LENGTH(MACOS_LOGS_FIELD_LEVEL_ID), (uint64_t)level);
    }
    else {
        macos_logs_add_facet_cstring(
            facets,
            MACOS_LOGS_FIELD_LEVEL,
            MACOS_LOGS_FIELD_LENGTH(MACOS_LOGS_FIELD_LEVEL),
            MACOS_LOGS_FACET_VALUE_CACHE_LEVEL,
            MACOS_LOGS_CSTRING_LITERAL("Undefined"));
        if(need_details)
            macos_logs_add_uint64(
                facets,
                MACOS_LOGS_FIELD_LEVEL_ID,
                MACOS_LOGS_FIELD_LENGTH(MACOS_LOGS_FIELD_LEVEL_ID),
                (uint64_t)OSLogEntryLogLevelUndefined);
    }

    if([entry conformsToProtocol:@protocol(OSLogEntryFromProcess)]) {
        id<OSLogEntryFromProcess> process_entry = (id<OSLogEntryFromProcess>)entry;
        macos_logs_add_facet_string(
            facets,
            MACOS_LOGS_FIELD_PROCESS,
            MACOS_LOGS_FIELD_LENGTH(MACOS_LOGS_FIELD_PROCESS),
            MACOS_LOGS_FACET_VALUE_CACHE_PROCESS,
            process_entry.process);
        macos_logs_add_facet_string(
            facets,
            MACOS_LOGS_FIELD_SENDER,
            MACOS_LOGS_FIELD_LENGTH(MACOS_LOGS_FIELD_SENDER),
            MACOS_LOGS_FACET_VALUE_CACHE_SENDER,
            process_entry.sender);
        if(need_details) {
            macos_logs_add_uint64(
                facets, MACOS_LOGS_FIELD_PID, MACOS_LOGS_FIELD_LENGTH(MACOS_LOGS_FIELD_PID),
                (uint64_t)process_entry.processIdentifier);
            macos_logs_add_uint64(
                facets, MACOS_LOGS_FIELD_THREAD_ID, MACOS_LOGS_FIELD_LENGTH(MACOS_LOGS_FIELD_THREAD_ID),
                process_entry.threadIdentifier);
            macos_logs_add_uint64(
                facets, MACOS_LOGS_FIELD_ACTIVITY_ID, MACOS_LOGS_FIELD_LENGTH(MACOS_LOGS_FIELD_ACTIVITY_ID),
                process_entry.activityIdentifier);
        }
    }

    if([entry conformsToProtocol:@protocol(OSLogEntryWithPayload)]) {
        id<OSLogEntryWithPayload> payload_entry = (id<OSLogEntryWithPayload>)entry;
        macos_logs_add_facet_string(
            facets,
            MACOS_LOGS_FIELD_SUBSYSTEM,
            MACOS_LOGS_FIELD_LENGTH(MACOS_LOGS_FIELD_SUBSYSTEM),
            MACOS_LOGS_FACET_VALUE_CACHE_SUBSYSTEM,
            payload_entry.subsystem);
        macos_logs_add_facet_string(
            facets,
            MACOS_LOGS_FIELD_CATEGORY,
            MACOS_LOGS_FIELD_LENGTH(MACOS_LOGS_FIELD_CATEGORY),
            MACOS_LOGS_FACET_VALUE_CACHE_CATEGORY,
            payload_entry.category);
        macos_logs_add_facet_uint64(
            facets,
            MACOS_LOGS_FIELD_COMPONENT_COUNT,
            MACOS_LOGS_FIELD_LENGTH(MACOS_LOGS_FIELD_COMPONENT_COUNT),
            MACOS_LOGS_FACET_VALUE_CACHE_COMPONENT_COUNT,
            payload_entry.components.count);
        if(need_details)
            macos_logs_add_string(
                facets, MACOS_LOGS_FIELD_FORMAT_STRING, MACOS_LOGS_FIELD_LENGTH(MACOS_LOGS_FIELD_FORMAT_STRING),
                payload_entry.formatString);
    }

    if(need_details && [entry isKindOfClass:[OSLogEntryActivity class]]) {
        OSLogEntryActivity *activity_entry = (OSLogEntryActivity *)entry;
        macos_logs_add_uint64(
            facets,
            MACOS_LOGS_FIELD_PARENT_ACTIVITY_ID,
            MACOS_LOGS_FIELD_LENGTH(MACOS_LOGS_FIELD_PARENT_ACTIVITY_ID),
            activity_entry.parentActivityIdentifier);
    }

    if([entry isKindOfClass:[OSLogEntrySignpost class]]) {
        OSLogEntrySignpost *signpost_entry = (OSLogEntrySignpost *)entry;
        macos_logs_add_facet_string(
            facets,
            MACOS_LOGS_FIELD_SIGNPOST_NAME,
            MACOS_LOGS_FIELD_LENGTH(MACOS_LOGS_FIELD_SIGNPOST_NAME),
            MACOS_LOGS_FACET_VALUE_CACHE_SIGNPOST_NAME,
            signpost_entry.signpostName);
        macos_logs_add_facet_cstring(
            facets,
            MACOS_LOGS_FIELD_SIGNPOST_TYPE,
            MACOS_LOGS_FIELD_LENGTH(MACOS_LOGS_FIELD_SIGNPOST_TYPE),
            MACOS_LOGS_FACET_VALUE_CACHE_SIGNPOST_TYPE,
            macos_logs_signpost_type_name(signpost_entry.signpostType));
        if(need_details)
            macos_logs_add_uint64(
                facets,
                MACOS_LOGS_FIELD_SIGNPOST_ID,
                MACOS_LOGS_FIELD_LENGTH(MACOS_LOGS_FIELD_SIGNPOST_ID),
                signpost_entry.signpostIdentifier);
    }

    return bytes;
}

typedef enum {
    MACOS_LOGS_PREDICATE_VALUE_STRING,
    MACOS_LOGS_PREDICATE_VALUE_UINT64,
    MACOS_LOGS_PREDICATE_VALUE_LEVEL,
    MACOS_LOGS_PREDICATE_VALUE_STORE_CATEGORY,
    MACOS_LOGS_PREDICATE_VALUE_SIGNPOST_TYPE,
} MACOS_LOGS_PREDICATE_VALUE_TYPE;

typedef enum {
    MACOS_LOGS_PREDICATE_UNKNOWN,
    MACOS_LOGS_PREDICATE_SUPPORTED,
    MACOS_LOGS_PREDICATE_UNSUPPORTED,
} MACOS_LOGS_PREDICATE_STATE;

typedef struct {
    const char *facet_key;
    size_t facet_key_length;
    const char *oslog_key;
    MACOS_LOGS_PREDICATE_VALUE_TYPE value_type;
    MACOS_LOGS_PREDICATE_STATE state;
} MACOS_LOGS_PREDICATE_FIELD;

#define MACOS_LOGS_PREDICATE_FIELD_ENTRY(facet, oslog, type) \
    { facet, MACOS_LOGS_FIELD_LENGTH(facet), oslog, type, MACOS_LOGS_PREDICATE_UNKNOWN }

static const MACOS_LOGS_PREDICATE_FIELD macos_logs_predicate_field_templates[] = {
    MACOS_LOGS_PREDICATE_FIELD_ENTRY(MACOS_LOGS_FIELD_SUBSYSTEM, "subsystem", MACOS_LOGS_PREDICATE_VALUE_STRING),
    MACOS_LOGS_PREDICATE_FIELD_ENTRY(MACOS_LOGS_FIELD_CATEGORY, "category", MACOS_LOGS_PREDICATE_VALUE_STRING),
    MACOS_LOGS_PREDICATE_FIELD_ENTRY(MACOS_LOGS_FIELD_PROCESS, "process", MACOS_LOGS_PREDICATE_VALUE_STRING),
    MACOS_LOGS_PREDICATE_FIELD_ENTRY(MACOS_LOGS_FIELD_SENDER, "sender", MACOS_LOGS_PREDICATE_VALUE_STRING),
    MACOS_LOGS_PREDICATE_FIELD_ENTRY(MACOS_LOGS_FIELD_PID, "processIdentifier", MACOS_LOGS_PREDICATE_VALUE_UINT64),
    MACOS_LOGS_PREDICATE_FIELD_ENTRY(MACOS_LOGS_FIELD_LEVEL, "level", MACOS_LOGS_PREDICATE_VALUE_LEVEL),
    MACOS_LOGS_PREDICATE_FIELD_ENTRY(MACOS_LOGS_FIELD_LEVEL_ID, "level", MACOS_LOGS_PREDICATE_VALUE_UINT64),
    MACOS_LOGS_PREDICATE_FIELD_ENTRY(
        MACOS_LOGS_FIELD_STORE_CATEGORY, "storeCategory", MACOS_LOGS_PREDICATE_VALUE_STORE_CATEGORY),
    MACOS_LOGS_PREDICATE_FIELD_ENTRY(MACOS_LOGS_FIELD_SIGNPOST_NAME, "signpostName", MACOS_LOGS_PREDICATE_VALUE_STRING),
    MACOS_LOGS_PREDICATE_FIELD_ENTRY(
        MACOS_LOGS_FIELD_SIGNPOST_TYPE, "signpostType", MACOS_LOGS_PREDICATE_VALUE_SIGNPOST_TYPE),
};

static NSNumber *macos_logs_uint64_number_from_string(const char *value) {
    if(!value || !*value)
        return nil;

    char *end = NULL;
    errno = 0;
    unsigned long long n = strtoull(value, &end, 10);
    if(errno || !end || *end)
        return nil;

    return [NSNumber numberWithUnsignedLongLong:n];
}

static NSNumber *macos_logs_level_number_from_string(const char *value) {
    if(!value || !*value)
        return nil;

    if(strcasecmp(value, "Undefined") == 0)
        return [NSNumber numberWithInteger:OSLogEntryLogLevelUndefined];
    if(strcasecmp(value, "Debug") == 0)
        return [NSNumber numberWithInteger:OSLogEntryLogLevelDebug];
    if(strcasecmp(value, "Info") == 0)
        return [NSNumber numberWithInteger:OSLogEntryLogLevelInfo];
    if(strcasecmp(value, "Notice") == 0)
        return [NSNumber numberWithInteger:OSLogEntryLogLevelNotice];
    if(strcasecmp(value, "Error") == 0)
        return [NSNumber numberWithInteger:OSLogEntryLogLevelError];
    if(strcasecmp(value, "Fault") == 0)
        return [NSNumber numberWithInteger:OSLogEntryLogLevelFault];

    return macos_logs_uint64_number_from_string(value);
}

static NSNumber *macos_logs_store_category_number_from_string(const char *value) {
    if(!value || !*value)
        return nil;

    if(strcasecmp(value, "Undefined") == 0)
        return [NSNumber numberWithInteger:OSLogEntryStoreCategoryUndefined];
    if(strcasecmp(value, "Metadata") == 0)
        return [NSNumber numberWithInteger:OSLogEntryStoreCategoryMetadata];
    if(strcasecmp(value, "ShortTerm") == 0)
        return [NSNumber numberWithInteger:OSLogEntryStoreCategoryShortTerm];
    if(strcasecmp(value, "LongTermAuto") == 0)
        return [NSNumber numberWithInteger:OSLogEntryStoreCategoryLongTermAuto];
    if(strcasecmp(value, "LongTerm1") == 0)
        return [NSNumber numberWithInteger:OSLogEntryStoreCategoryLongTerm1];
    if(strcasecmp(value, "LongTerm3") == 0)
        return [NSNumber numberWithInteger:OSLogEntryStoreCategoryLongTerm3];
    if(strcasecmp(value, "LongTerm7") == 0)
        return [NSNumber numberWithInteger:OSLogEntryStoreCategoryLongTerm7];
    if(strcasecmp(value, "LongTerm14") == 0)
        return [NSNumber numberWithInteger:OSLogEntryStoreCategoryLongTerm14];
    if(strcasecmp(value, "LongTerm30") == 0)
        return [NSNumber numberWithInteger:OSLogEntryStoreCategoryLongTerm30];

    return macos_logs_uint64_number_from_string(value);
}

static NSNumber *macos_logs_signpost_type_number_from_string(const char *value) {
    if(!value || !*value)
        return nil;

    if(strcasecmp(value, "Undefined") == 0)
        return [NSNumber numberWithInteger:OSLogEntrySignpostTypeUndefined];
    if(strcasecmp(value, "IntervalBegin") == 0)
        return [NSNumber numberWithInteger:OSLogEntrySignpostTypeIntervalBegin];
    if(strcasecmp(value, "IntervalEnd") == 0)
        return [NSNumber numberWithInteger:OSLogEntrySignpostTypeIntervalEnd];
    if(strcasecmp(value, "Event") == 0)
        return [NSNumber numberWithInteger:OSLogEntrySignpostTypeEvent];

    return macos_logs_uint64_number_from_string(value);
}

static id macos_logs_predicate_value(MACOS_LOGS_PREDICATE_FIELD *field, const char *value) {
    switch(field->value_type) {
        case MACOS_LOGS_PREDICATE_VALUE_STRING:
            return value && *value ? [NSString stringWithUTF8String:value] : nil;
        case MACOS_LOGS_PREDICATE_VALUE_UINT64:
            return macos_logs_uint64_number_from_string(value);
        case MACOS_LOGS_PREDICATE_VALUE_LEVEL:
            return macos_logs_level_number_from_string(value);
        case MACOS_LOGS_PREDICATE_VALUE_STORE_CATEGORY:
            return macos_logs_store_category_number_from_string(value);
        case MACOS_LOGS_PREDICATE_VALUE_SIGNPOST_TYPE:
            return macos_logs_signpost_type_number_from_string(value);
    }

    return nil;
}

static inline bool macos_logs_string_value_matches(NSString *entry_value, id predicate_value) {
    return entry_value && [predicate_value isKindOfClass:[NSString class]] &&
           [entry_value isEqualToString:(NSString *)predicate_value];
}

static inline bool macos_logs_number_value_matches(id predicate_value, long long entry_value) {
    return [predicate_value isKindOfClass:[NSNumber class]] &&
           [(NSNumber *)predicate_value longLongValue] == entry_value;
}

static bool macos_logs_entry_matches_predicate_value(
    MACOS_LOGS_PREDICATE_FIELD *field, OSLogEntry *entry, id predicate_value) {
    if(strcmp(field->facet_key, MACOS_LOGS_FIELD_STORE_CATEGORY) == 0)
        return macos_logs_number_value_matches(predicate_value, (long long)entry.storeCategory);

    if(strcmp(field->facet_key, MACOS_LOGS_FIELD_LEVEL) == 0 ||
       strcmp(field->facet_key, MACOS_LOGS_FIELD_LEVEL_ID) == 0) {
        OSLogEntryLogLevel level = OSLogEntryLogLevelUndefined;
        if([entry isKindOfClass:[OSLogEntryLog class]])
            level = ((OSLogEntryLog *)entry).level;

        return macos_logs_number_value_matches(predicate_value, (long long)level);
    }

    if([entry conformsToProtocol:@protocol(OSLogEntryFromProcess)]) {
        id<OSLogEntryFromProcess> process_entry = (id<OSLogEntryFromProcess>)entry;

        if(strcmp(field->facet_key, MACOS_LOGS_FIELD_PROCESS) == 0)
            return macos_logs_string_value_matches(process_entry.process, predicate_value);
        if(strcmp(field->facet_key, MACOS_LOGS_FIELD_PID) == 0)
            return macos_logs_number_value_matches(predicate_value, (long long)process_entry.processIdentifier);
        if(strcmp(field->facet_key, MACOS_LOGS_FIELD_SENDER) == 0)
            return macos_logs_string_value_matches(process_entry.sender, predicate_value);
    }

    if([entry conformsToProtocol:@protocol(OSLogEntryWithPayload)]) {
        id<OSLogEntryWithPayload> payload_entry = (id<OSLogEntryWithPayload>)entry;

        if(strcmp(field->facet_key, MACOS_LOGS_FIELD_SUBSYSTEM) == 0)
            return macos_logs_string_value_matches(payload_entry.subsystem, predicate_value);
        if(strcmp(field->facet_key, MACOS_LOGS_FIELD_CATEGORY) == 0)
            return macos_logs_string_value_matches(payload_entry.category, predicate_value);
    }

    if([entry isKindOfClass:[OSLogEntrySignpost class]]) {
        OSLogEntrySignpost *signpost_entry = (OSLogEntrySignpost *)entry;

        if(strcmp(field->facet_key, MACOS_LOGS_FIELD_SIGNPOST_NAME) == 0)
            return macos_logs_string_value_matches(signpost_entry.signpostName, predicate_value);
        if(strcmp(field->facet_key, MACOS_LOGS_FIELD_SIGNPOST_TYPE) == 0)
            return macos_logs_number_value_matches(predicate_value, (long long)signpost_entry.signpostType);
    }

    return false;
}

static bool macos_logs_entry_matches_any_predicate_value(
    MACOS_LOGS_PREDICATE_FIELD *field, OSLogEntry *entry, NSArray *predicate_values) {
    for(id predicate_value in predicate_values) {
        if(macos_logs_entry_matches_predicate_value(field, entry, predicate_value))
            return true;
    }

    return false;
}

static bool macos_logs_entry_is_in_query_timeframe(
    const LOGS_QUERY_STATUS *lqs, bool forward, usec_t msg_ut, bool *past_timeframe) {
    *past_timeframe = false;

    if(!msg_ut)
        return false;

    if(forward) {
        if(msg_ut < lqs->query.start_ut)
            return false;
        if(msg_ut > lqs->query.stop_ut) {
            *past_timeframe = true;
            return false;
        }
    }
    else {
        if(msg_ut > lqs->query.start_ut)
            return false;
        if(msg_ut < lqs->query.stop_ut) {
            *past_timeframe = true;
            return false;
        }
    }

    return true;
}

static bool macos_logs_probe_native_predicate(
    LOGS_QUERY_STATUS *lqs, bool forward, MACOS_LOGS_PREDICATE_FIELD *field,
    NSArray *predicate_values, OSLogEnumerator *enumerator, bool *native_matched) {
    size_t rows_checked = 0;
    *native_matched = false;

    while(true) {
        @autoreleasepool {
            OSLogEntry *entry = [enumerator nextObject];
            if(!entry)
                break;

            rows_checked++;
            if(rows_checked > MACOS_LOGS_PREDICATE_PROBE_ROWS)
                break;

            if(macos_logs_check_stop(lqs))
                break;

            bool past_timeframe = false;
            usec_t msg_ut = macos_logs_entry_time_ut(entry);
            if(!macos_logs_entry_is_in_query_timeframe(lqs, forward, msg_ut, &past_timeframe)) {
                if(past_timeframe)
                    break;
                continue;
            }

            if(!macos_logs_entry_matches_any_predicate_value(field, entry, predicate_values)) {
                field->state = MACOS_LOGS_PREDICATE_UNSUPPORTED;
                nd_log(NDLS_COLLECTORS, NDLP_NOTICE,
                       "MACOS-LOGS: OSLog predicate field '%s' returned a non-matching entry; falling back to userspace filtering for this field",
                       field->facet_key);
                return false;
            }

            *native_matched = true;
            return true;
        }
    }

    return true;
}

static bool macos_logs_probe_unfiltered_has_match(
    LOGS_QUERY_STATUS *lqs, bool forward, OSLogStore *store, OSLogEnumeratorOptions options,
    OSLogPosition *position, MACOS_LOGS_PREDICATE_FIELD *field, NSArray *predicate_values, bool *userspace_matched) {
    *userspace_matched = false;

    NSError *error = nil;
    OSLogEnumerator *enumerator = [store entriesEnumeratorWithOptions:options position:position predicate:nil error:&error];
    if(!enumerator) {
        nd_log(NDLS_COLLECTORS, NDLP_NOTICE,
               "MACOS-LOGS: failed to create bounded OSLog predicate probe enumerator, domain '%s', code %ld",
               macos_logs_error_domain(error), (long)error.code);
        return false;
    }

    size_t rows_checked = 0;
    while(true) {
        @autoreleasepool {
            OSLogEntry *entry = [enumerator nextObject];
            if(!entry)
                break;

            rows_checked++;
            if(rows_checked > MACOS_LOGS_PREDICATE_PROBE_ROWS)
                break;

            if(macos_logs_check_stop(lqs))
                break;

            bool past_timeframe = false;
            usec_t msg_ut = macos_logs_entry_time_ut(entry);
            if(!macos_logs_entry_is_in_query_timeframe(lqs, forward, msg_ut, &past_timeframe)) {
                if(past_timeframe)
                    break;
                continue;
            }

            if(macos_logs_entry_matches_any_predicate_value(field, entry, predicate_values)) {
                *userspace_matched = true;
                return true;
            }
        }
    }

    return true;
}

typedef struct {
    MACOS_LOGS_PREDICATE_FIELD *field;
    NSString *oslog_key;
    NSMutableArray *predicates;
    NSMutableArray *values;
} MACOS_LOGS_PREDICATE_VALUE_CB_DATA;

static bool macos_logs_add_selected_value_predicate(
    FACETS *facets __maybe_unused, size_t value_id __maybe_unused, const char *key __maybe_unused,
    const char *value, void *data) {
    MACOS_LOGS_PREDICATE_VALUE_CB_DATA *cb = data;
    id predicate_value = macos_logs_predicate_value(cb->field, value);
    if(!predicate_value)
        return true;

    NSPredicate *predicate = [NSPredicate predicateWithFormat:@"%K == %@", cb->oslog_key, predicate_value];
    if(predicate) {
        [cb->predicates addObject:predicate];
        [cb->values addObject:predicate_value];
    }

    return true;
}

static bool macos_logs_predicate_probe(
    LOGS_QUERY_STATUS *lqs, bool forward, OSLogStore *store, OSLogEnumeratorOptions options,
    OSLogPosition *position, MACOS_LOGS_PREDICATE_FIELD *field, NSPredicate *predicate, NSArray *predicate_values) {
    if(field->state == MACOS_LOGS_PREDICATE_UNSUPPORTED)
        return false;

    NSError *error = nil;
    OSLogEnumerator *probe = [store entriesEnumeratorWithOptions:options position:position predicate:predicate error:&error];
    if(!probe) {
        field->state = MACOS_LOGS_PREDICATE_UNSUPPORTED;
        nd_log(NDLS_COLLECTORS, NDLP_NOTICE,
               "MACOS-LOGS: OSLog predicate field '%s' is not supported, domain '%s', code %ld; falling back to userspace filtering for this field",
               field->facet_key, macos_logs_error_domain(error), (long)error.code);
        return false;
    }

    bool native_matched = false;
    if(!macos_logs_probe_native_predicate(lqs, forward, field, predicate_values, probe, &native_matched))
        return false;

    if(native_matched) {
        field->state = MACOS_LOGS_PREDICATE_SUPPORTED;
        return true;
    }

    bool userspace_matched = false;
    if(!macos_logs_probe_unfiltered_has_match(
            lqs, forward, store, options, position, field, predicate_values, &userspace_matched))
        return false;

    if(userspace_matched) {
        field->state = MACOS_LOGS_PREDICATE_UNSUPPORTED;
        nd_log(NDLS_COLLECTORS, NDLP_NOTICE,
               "MACOS-LOGS: OSLog predicate field '%s' missed an entry found by userspace filtering; falling back to userspace filtering for this field",
               field->facet_key);
        return false;
    }

    // No bounded proof either way. Keep the field unknown and do not push it down
    // for this query; correctness is more important than using an unproven filter.
    return false;
}

static NSPredicate *macos_logs_predicate_for_field(
    LOGS_QUERY_STATUS *lqs, bool forward, OSLogStore *store, OSLogEnumeratorOptions options,
    OSLogPosition *position, MACOS_LOGS_PREDICATE_FIELD *field) {
    if(field->state == MACOS_LOGS_PREDICATE_UNSUPPORTED)
        return nil;

    NSMutableArray *value_predicates = [NSMutableArray array];
    NSMutableArray *predicate_values = [NSMutableArray array];
    NSString *oslog_key = [NSString stringWithUTF8String:field->oslog_key];
    if(!oslog_key)
        return nil;

    MACOS_LOGS_PREDICATE_VALUE_CB_DATA cb = {
        .field = field,
        .oslog_key = oslog_key,
        .predicates = value_predicates,
        .values = predicate_values,
    };

    if(!facets_foreach_selected_value_in_key(
            lqs->facets, field->facet_key, field->facet_key_length, used_hashes_registry,
            macos_logs_add_selected_value_predicate, &cb) ||
       !value_predicates.count || !predicate_values.count)
        return nil;

    NSPredicate *predicate = value_predicates.count == 1 ?
                                 [value_predicates objectAtIndex:0] :
                                 [NSCompoundPredicate orPredicateWithSubpredicates:value_predicates];

    return macos_logs_predicate_probe(lqs, forward, store, options, position, field, predicate, predicate_values) ?
               predicate : nil;
}

static NSPredicate *macos_logs_build_predicate(
    LOGS_QUERY_STATUS *lqs, OSLogStore *store, OSLogEnumeratorOptions options, OSLogPosition *position) {
    if(!lqs->rq.slice || !lqs->rq.filters)
        return nil;

    MACOS_LOGS_PREDICATE_FIELD predicate_fields[
        sizeof(macos_logs_predicate_field_templates) / sizeof(macos_logs_predicate_field_templates[0])];
    memcpy(predicate_fields, macos_logs_predicate_field_templates, sizeof(predicate_fields));

    NSMutableArray *field_predicates = [NSMutableArray array];
    size_t fields = sizeof(predicate_fields) / sizeof(predicate_fields[0]);
    for(size_t i = 0; i < fields; i++) {
        NSPredicate *predicate = macos_logs_predicate_for_field(
            lqs, lqs->rq.direction == FACETS_ANCHOR_DIRECTION_FORWARD, store, options, position,
            &predicate_fields[i]);
        if(predicate)
            [field_predicates addObject:predicate];
    }

    if(!field_predicates.count)
        return nil;

    return field_predicates.count == 1 ?
               [field_predicates objectAtIndex:0] :
               [NSCompoundPredicate andPredicateWithSubpredicates:field_predicates];
}

static OSLogStore *macos_logs_open_store(NSError **error) {
    OSLogStore *store = nil;

#if defined(MAC_OS_VERSION_12_0) && MAC_OS_X_VERSION_MAX_ALLOWED >= MAC_OS_VERSION_12_0
    if(@available(macOS 12.0, *))
        store = [OSLogStore storeWithScope:OSLogStoreSystem error:error];
#endif

    if(!store)
        store = [OSLogStore localStoreAndReturnError:error];

    return store;
}

static bool macos_logs_source_selected(LOGS_QUERY_STATUS *lqs) {
    if(lqs->rq.source_type & MACOS_LOGS_SOURCE_ALL)
        return true;

    if(lqs->rq.sources) {
        if(simple_pattern_matches(lqs->rq.sources, "all") ||
           simple_pattern_matches(lqs->rq.sources, "system") ||
           simple_pattern_matches(lqs->rq.sources, "macos-unified-log"))
            return true;
    }

    return false;
}

MACOS_LOGS_QUERY_STATUS macos_logs_query_oslog(LOGS_QUERY_STATUS *lqs) {
    if(!macos_logs_source_selected(lqs))
        return MACOS_LOGS_QUERY_OK;

    if(macos_logs_check_stop(lqs))
        return (lqs->cancelled && __atomic_load_n(lqs->cancelled, __ATOMIC_RELAXED)) ?
                   MACOS_LOGS_QUERY_CANCELLED : MACOS_LOGS_QUERY_TIMED_OUT;

    @try {
        @autoreleasepool {
        if(macos_logs_check_stop(lqs))
            return (lqs->cancelled && __atomic_load_n(lqs->cancelled, __ATOMIC_RELAXED)) ?
                       MACOS_LOGS_QUERY_CANCELLED : MACOS_LOGS_QUERY_TIMED_OUT;

        NSError *error = nil;
        OSLogStore *store = macos_logs_open_store(&error);
        if(!store) {
            nd_log(NDLS_COLLECTORS, NDLP_ERR,
                   "MACOS-LOGS: failed to open OSLogStore, domain '%s', code %ld",
                   macos_logs_error_domain(error), (long)error.code);
            return MACOS_LOGS_QUERY_OPEN_FAILED;
        }

        bool forward = lqs->rq.direction == FACETS_ANCHOR_DIRECTION_FORWARD;
        // lqs_query_timeframe() already orients these per direction:
        //   forward:  start_ut = oldest edge, stop_ut = newest edge (ascending)
        //   backward: start_ut = newest edge, stop_ut = oldest edge (descending)
        // So use them directly as the iteration start/end; do NOT re-swap.
        usec_t position_ut = lqs->query.start_ut;

        NSDate *position_date = macos_logs_date_from_ut(position_ut);
        OSLogEnumeratorOptions options = forward ? 0 : OSLogEnumeratorReverse;
        OSLogPosition *position = [store positionWithDate:position_date];

        NSPredicate *predicate = macos_logs_build_predicate(lqs, store, options, position);

        error = nil;
        OSLogEnumerator *enumerator = [store entriesEnumeratorWithOptions:options position:position predicate:predicate error:&error];
        if(!enumerator && predicate) {
            nd_log(NDLS_COLLECTORS, NDLP_NOTICE,
                   "MACOS-LOGS: OSLogEnumerator failed with native predicate, domain '%s', code %ld; retrying without native predicate",
                   macos_logs_error_domain(error), (long)error.code);
            error = nil;
            enumerator = [store entriesEnumeratorWithOptions:options position:position predicate:nil error:&error];
        }
        if(!enumerator) {
            nd_log(NDLS_COLLECTORS, NDLP_ERR,
                   "MACOS-LOGS: failed to create OSLogEnumerator, domain '%s', code %ld",
                   macos_logs_error_domain(error), (long)error.code);
            return MACOS_LOGS_QUERY_ENUMERATOR_FAILED;
        }

        lqs->c.query_started_ut = now_monotonic_usec();
        lqs->c.progress_last_ut = lqs->c.query_started_ut;

        size_t row_counter = 0;
        size_t last_row_counter = 0;
        size_t rows_useful = 0;
        size_t last_rows_useful = 0;
        size_t bytes = 0;
        size_t last_bytes = 0;
        size_t last_progress_done = 0;
        usec_t last_usec_from = 0;
        usec_t last_usec_to = 0;
        bool query_requires_detail_fields = macos_logs_query_requires_detail_fields(lqs);

        facets_rows_begin(lqs->facets);

        while(true) {
            @autoreleasepool {
                OSLogEntry *entry = [enumerator nextObject];
                if(!entry)
                    break;

                usec_t msg_ut = macos_logs_entry_time_ut(entry);
                if(!msg_ut)
                    continue;

                usec_t raw_msg_ut = msg_ut;

                if(forward) {
                    if(msg_ut < lqs->query.start_ut)
                        continue;
                    if(msg_ut > lqs->query.stop_ut)
                        break;
                }
                else {
                    if(msg_ut > lqs->query.start_ut)
                        continue;
                    if(msg_ut < lqs->query.stop_ut)
                        break;
                }

                if(msg_ut > lqs->last_modified)
                    lqs->last_modified = msg_ut;

                bool need_details =
                    query_requires_detail_fields || facets_row_candidate_to_keep(lqs->facets, msg_ut);
                bytes += macos_logs_process_entry(lqs->facets, entry, need_details);

                if(forward) {
                    if(unlikely(msg_ut >= last_usec_from && msg_ut <= last_usec_to))
                        msg_ut = ++last_usec_to;
                    else
                        last_usec_from = last_usec_to = msg_ut;
                }
                else {
                    if(unlikely(msg_ut >= last_usec_from && msg_ut <= last_usec_to))
                        msg_ut = --last_usec_from;
                    else
                        last_usec_from = last_usec_to = msg_ut;
                }

                if(facets_row_finished(lqs->facets, msg_ut))
                    rows_useful++;

                row_counter++;
                if(unlikely((row_counter % MACOS_LOGS_DATA_ONLY_CHECK_EVERY_ROWS) == 0 &&
                            lqs->query.stop_when_full &&
                            facets_rows(lqs->facets) >= lqs->rq.entries)) {
                    if(forward) {
                        usec_t newest = facets_row_newest_ut(lqs->facets);
                        if(newest && msg_ut > newest + lqs->anchor.delta_ut)
                            break;
                    }
                    else {
                        usec_t oldest = facets_row_oldest_ut(lqs->facets);
                        if(oldest && msg_ut < oldest - lqs->anchor.delta_ut)
                            break;
                    }
                }

                if(unlikely(row_counter % MACOS_LOGS_PROGRESS_EVERY_ROWS == 0)) {
                    if(macos_logs_check_stop(lqs)) {
                        lqs->c.rows_read += row_counter - last_row_counter;
                        lqs->c.bytes_read += bytes - last_bytes;
                        lqs->c.rows_useful += rows_useful - last_rows_useful;
                        lqs->c.query_finished_ut = now_monotonic_usec();
                        return (lqs->cancelled && __atomic_load_n(lqs->cancelled, __ATOMIC_RELAXED)) ?
                            MACOS_LOGS_QUERY_CANCELLED : MACOS_LOGS_QUERY_TIMED_OUT;
                    }

                    usec_t now_ut = now_monotonic_usec();
                    if(now_ut - lqs->c.progress_last_ut >= MACOS_LOGS_PROGRESS_EVERY_UT) {
                        lqs->c.progress_last_ut = now_ut;
                        netdata_mutex_lock(&stdout_mutex);
                        size_t progress_done = 0;
                        if(macos_logs_progress_by_time(lqs, forward, raw_msg_ut, &progress_done)) {
                            if(progress_done < last_progress_done)
                                progress_done = last_progress_done;
                            else
                                last_progress_done = progress_done;

                            pluginsd_function_progress_to_stdout(
                                lqs->rq.transaction, progress_done, MACOS_LOGS_PROGRESS_TOTAL);
                        }
                        else
                            pluginsd_function_progress_to_stdout(lqs->rq.transaction, row_counter, 0);
                        netdata_mutex_unlock(&stdout_mutex);
                    }

                    lqs->c.rows_read += row_counter - last_row_counter;
                    last_row_counter = row_counter;
                    lqs->c.bytes_read += bytes - last_bytes;
                    last_bytes = bytes;
                    lqs->c.rows_useful += rows_useful - last_rows_useful;
                    last_rows_useful = rows_useful;
                }
            }
        }

        lqs->c.rows_read += row_counter - last_row_counter;
        lqs->c.bytes_read += bytes - last_bytes;
        lqs->c.rows_useful += rows_useful - last_rows_useful;
        lqs->c.query_finished_ut = now_monotonic_usec();

        return MACOS_LOGS_QUERY_OK;
        }
    } @catch(NSException *exception) {
        const char *exception_name = macos_logs_string(exception.name);
        const char *exception_reason = macos_logs_string(exception.reason);
        nd_log(NDLS_COLLECTORS, NDLP_ERR,
               "MACOS-LOGS: OSLog query raised Objective-C exception '%s': %s",
               exception_name ? exception_name : "unknown",
               exception_reason ? exception_reason : "no reason");
        lqs->c.query_finished_ut = now_monotonic_usec();
        return MACOS_LOGS_QUERY_ENUMERATOR_FAILED;
    }
}
