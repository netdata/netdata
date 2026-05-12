// SPDX-License-Identifier: GPL-3.0-or-later

#include "macos-logs.h"

#import <Foundation/Foundation.h>
#import <OSLog/OSLog.h>

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

static inline const char *macos_logs_string(NSString *value) {
    if(!value || value.length == 0)
        return NULL;

    return value.UTF8String;
}

static inline const char *macos_logs_error_domain(NSError *error) {
    const char *domain = macos_logs_string(error.domain);
    return domain ? domain : "unknown";
}

static inline void macos_logs_add_string(FACETS *facets, const char *key, NSString *value) {
    const char *s = macos_logs_string(value);
    if(s)
        facets_add_key_value(facets, key, s);
}

static inline void macos_logs_add_uint64(FACETS *facets, const char *key, uint64_t value) {
    char buffer[32];
    snprintfz(buffer, sizeof(buffer), "%" PRIu64, value);
    facets_add_key_value(facets, key, buffer);
}

static inline const char *macos_logs_level_name(OSLogEntryLogLevel level) {
    switch(level) {
        case OSLogEntryLogLevelDebug:
            return "Debug";
        case OSLogEntryLogLevelInfo:
            return "Info";
        case OSLogEntryLogLevelNotice:
            return "Notice";
        case OSLogEntryLogLevelError:
            return "Error";
        case OSLogEntryLogLevelFault:
            return "Fault";
        case OSLogEntryLogLevelUndefined:
        default:
            return "Undefined";
    }
}

static inline const char *macos_logs_store_category_name(OSLogEntryStoreCategory category) {
    switch(category) {
        case OSLogEntryStoreCategoryMetadata:
            return "Metadata";
        case OSLogEntryStoreCategoryShortTerm:
            return "ShortTerm";
        case OSLogEntryStoreCategoryLongTermAuto:
            return "LongTermAuto";
        case OSLogEntryStoreCategoryLongTerm1:
            return "LongTerm1";
        case OSLogEntryStoreCategoryLongTerm3:
            return "LongTerm3";
        case OSLogEntryStoreCategoryLongTerm7:
            return "LongTerm7";
        case OSLogEntryStoreCategoryLongTerm14:
            return "LongTerm14";
        case OSLogEntryStoreCategoryLongTerm30:
            return "LongTerm30";
        case OSLogEntryStoreCategoryUndefined:
        default:
            return "Undefined";
    }
}

static inline const char *macos_logs_entry_type(OSLogEntry *entry) {
    if([entry isKindOfClass:[OSLogEntryLog class]])
        return "Log";
    if([entry isKindOfClass:[OSLogEntryActivity class]])
        return "Activity";
    if([entry isKindOfClass:[OSLogEntrySignpost class]])
        return "Signpost";
    if([entry isKindOfClass:[OSLogEntryBoundary class]])
        return "Boundary";

    return "Entry";
}

static size_t macos_logs_process_entry(FACETS *facets, OSLogEntry *entry) {
    size_t bytes = 0;

    NSString *message = entry.composedMessage;
    macos_logs_add_string(facets, MACOS_LOGS_FIELD_MESSAGE, message);
    bytes += message ? message.length : 0;

    facets_add_key_value(facets, MACOS_LOGS_FIELD_ENTRY_TYPE, macos_logs_entry_type(entry));
    facets_add_key_value(facets, MACOS_LOGS_FIELD_STORE_CATEGORY, macos_logs_store_category_name(entry.storeCategory));

    if([entry isKindOfClass:[OSLogEntryLog class]]) {
        OSLogEntryLog *log_entry = (OSLogEntryLog *)entry;
        OSLogEntryLogLevel level = log_entry.level;

        facets_add_key_value(facets, MACOS_LOGS_FIELD_LEVEL, macos_logs_level_name(level));
        macos_logs_add_uint64(facets, MACOS_LOGS_FIELD_LEVEL_ID, (uint64_t)level);
    }
    else {
        facets_add_key_value(facets, MACOS_LOGS_FIELD_LEVEL, "Undefined");
        macos_logs_add_uint64(facets, MACOS_LOGS_FIELD_LEVEL_ID, (uint64_t)OSLogEntryLogLevelUndefined);
    }

    if([entry conformsToProtocol:@protocol(OSLogEntryFromProcess)]) {
        id<OSLogEntryFromProcess> process_entry = (id<OSLogEntryFromProcess>)entry;
        macos_logs_add_string(facets, MACOS_LOGS_FIELD_PROCESS, process_entry.process);
        macos_logs_add_uint64(facets, MACOS_LOGS_FIELD_PID, (uint64_t)process_entry.processIdentifier);
        macos_logs_add_string(facets, MACOS_LOGS_FIELD_SENDER, process_entry.sender);
        macos_logs_add_uint64(facets, MACOS_LOGS_FIELD_THREAD_ID, process_entry.threadIdentifier);
        macos_logs_add_uint64(facets, MACOS_LOGS_FIELD_ACTIVITY_ID, process_entry.activityIdentifier);
    }

    if([entry conformsToProtocol:@protocol(OSLogEntryWithPayload)]) {
        id<OSLogEntryWithPayload> payload_entry = (id<OSLogEntryWithPayload>)entry;
        macos_logs_add_string(facets, MACOS_LOGS_FIELD_SUBSYSTEM, payload_entry.subsystem);
        macos_logs_add_string(facets, MACOS_LOGS_FIELD_CATEGORY, payload_entry.category);
    }

    return bytes;
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

    @autoreleasepool {
        NSError *error = nil;
        OSLogStore *store = macos_logs_open_store(&error);
        if(!store) {
            nd_log(NDLS_COLLECTORS, NDLP_ERR,
                   "MACOS-LOGS: failed to open OSLogStore, domain '%s', code %ld",
                   macos_logs_error_domain(error), (long)error.code);
            return MACOS_LOGS_QUERY_OPEN_FAILED;
        }

        bool forward = lqs->rq.direction == FACETS_ANCHOR_DIRECTION_FORWARD;
        usec_t start_ut = forward ? lqs->query.start_ut : lqs->query.stop_ut;
        usec_t stop_ut = forward ? lqs->query.stop_ut : lqs->query.start_ut;
        usec_t position_ut = forward ? start_ut : stop_ut;

        NSDate *position_date = macos_logs_date_from_ut(position_ut);
        OSLogEnumeratorOptions options = forward ? 0 : OSLogEnumeratorReverse;
        OSLogPosition *position = [store positionWithDate:position_date];

        error = nil;
        OSLogEnumerator *enumerator = [store entriesEnumeratorWithOptions:options position:position predicate:nil error:&error];
        if(!enumerator) {
            nd_log(NDLS_COLLECTORS, NDLP_ERR,
                   "MACOS-LOGS: failed to create OSLogEnumerator, domain '%s', code %ld",
                   macos_logs_error_domain(error), (long)error.code);
            return MACOS_LOGS_QUERY_ENUMERATOR_FAILED;
        }

        lqs->c.query_started_ut = now_monotonic_usec();
        lqs->c.progress_last_ut = lqs->c.query_started_ut;
        lqs->c.rows_scanned_limit = MACOS_LOGS_MAX_ROWS_SCANNED;

        size_t row_counter = 0;
        size_t last_row_counter = 0;
        size_t rows_useful = 0;
        size_t bytes = 0;
        size_t last_bytes = 0;
        usec_t last_usec_from = 0;
        usec_t last_usec_to = 0;

        facets_rows_begin(lqs->facets);

        for(OSLogEntry *entry in enumerator) {
            usec_t msg_ut = macos_logs_entry_time_ut(entry);
            if(!msg_ut)
                continue;

            if(forward) {
                if(msg_ut < start_ut)
                    continue;

                if(msg_ut > stop_ut)
                    break;
            }
            else {
                if(msg_ut > stop_ut)
                    continue;

                if(msg_ut < start_ut)
                    break;
            }

            if(msg_ut > lqs->last_modified)
                lqs->last_modified = msg_ut;

            bytes += macos_logs_process_entry(lqs->facets, entry);

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

            if(unlikely(row_counter >= MACOS_LOGS_MAX_ROWS_SCANNED)) {
                lqs->c.rows_read += row_counter - last_row_counter;
                lqs->c.bytes_read += bytes - last_bytes;
                lqs->c.rows_useful += rows_useful;
                lqs->c.query_finished_ut = now_monotonic_usec();
                return MACOS_LOGS_QUERY_SCAN_LIMIT_REACHED;
            }

            if(unlikely(row_counter % MACOS_LOGS_PROGRESS_EVERY_ROWS == 0)) {
                if(macos_logs_check_stop(lqs)) {
                    lqs->c.rows_read += row_counter - last_row_counter;
                    lqs->c.bytes_read += bytes - last_bytes;
                    lqs->c.rows_useful += rows_useful;
                    lqs->c.query_finished_ut = now_monotonic_usec();
                    return (lqs->cancelled && __atomic_load_n(lqs->cancelled, __ATOMIC_RELAXED)) ?
                        MACOS_LOGS_QUERY_CANCELLED : MACOS_LOGS_QUERY_TIMED_OUT;
                }

                usec_t now_ut = now_monotonic_usec();
                if(now_ut - lqs->c.progress_last_ut >= MACOS_LOGS_PROGRESS_EVERY_UT) {
                    lqs->c.progress_last_ut = now_ut;
                    netdata_mutex_lock(&stdout_mutex);
                    pluginsd_function_progress_to_stdout(lqs->rq.transaction, row_counter, MACOS_LOGS_MAX_ROWS_SCANNED);
                    netdata_mutex_unlock(&stdout_mutex);
                }

                lqs->c.rows_read += row_counter - last_row_counter;
                last_row_counter = row_counter;
                lqs->c.bytes_read += bytes - last_bytes;
                last_bytes = bytes;
            }
        }

        lqs->c.rows_read += row_counter - last_row_counter;
        lqs->c.bytes_read += bytes - last_bytes;
        lqs->c.rows_useful += rows_useful;
        lqs->c.query_finished_ut = now_monotonic_usec();

        return MACOS_LOGS_QUERY_OK;
    }
}
