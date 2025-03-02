// SPDX-License-Identifier: GPL-3.0-or-later

// do not REMOVE this, it is used by systemd-journal includes to prevent saving the file, function, line of the
// source code that makes the calls, allowing our loggers to log the lines of source code that actually log
#define SD_JOURNAL_SUPPRESS_LOCATION

#include "../libnetdata.h"
#include "nd_log-internals.h"

const char *program_name = "";
uint64_t debug_flags = 0;
int aclklog_enabled = 0;

// --------------------------------------------------------------------------------------------------------------------

ALWAYS_INLINE void errno_clear(void) {
    errno = 0;

#if defined(OS_WINDOWS)
    SetLastError(ERROR_SUCCESS);
#endif
}

// --------------------------------------------------------------------------------------------------------------------
// logger router

static ND_LOG_METHOD nd_logger_select_output(ND_LOG_SOURCES source, FILE **fpp, SPINLOCK **spinlock) {
    *spinlock = NULL;
    ND_LOG_METHOD output = nd_log.sources[source].method;

    switch(output) {
        case NDLM_JOURNAL:
            if(unlikely(!nd_log.journal_direct.initialized && !nd_log.journal.initialized)) {
                output = NDLM_FILE;
                *fpp = stderr;
                *spinlock = &nd_log.std_error.spinlock;
            }
            else {
                *fpp = NULL;
                *spinlock = NULL;
            }
            break;

#if defined(OS_WINDOWS) && (defined(HAVE_ETW) || defined(HAVE_WEL))
#if defined(HAVE_ETW)
        case NDLM_ETW:
#endif
#if defined(HAVE_WEL)
        case NDLM_WEL:
#endif
            if(unlikely(!nd_log.eventlog.initialized)) {
                output = NDLM_FILE;
                *fpp = stderr;
                *spinlock = &nd_log.std_error.spinlock;
            }
            else {
                *fpp = NULL;
                *spinlock = NULL;
            }
            break;
#endif

        case NDLM_SYSLOG:
            if(unlikely(!nd_log.syslog.initialized)) {
                output = NDLM_FILE;
                *spinlock = &nd_log.std_error.spinlock;
                *fpp = stderr;
            }
            else {
                *spinlock = NULL;
                *fpp = NULL;
            }
            break;

        case NDLM_FILE:
            if(!nd_log.sources[source].fp) {
                *fpp = stderr;
                *spinlock = &nd_log.std_error.spinlock;
            }
            else {
                *fpp = nd_log.sources[source].fp;
                *spinlock = &nd_log.sources[source].spinlock;
            }
            break;

        case NDLM_STDOUT:
            output = NDLM_FILE;
            *fpp = stdout;
            *spinlock = &nd_log.std_output.spinlock;
            break;

        default:
        case NDLM_DEFAULT:
        case NDLM_STDERR:
            output = NDLM_FILE;
            *fpp = stderr;
            *spinlock = &nd_log.std_error.spinlock;
            break;

        case NDLM_DISABLED:
        case NDLM_DEVNULL:
            output = NDLM_DISABLED;
            *fpp = NULL;
            *spinlock = NULL;
            break;
    }

    return output;
}

// --------------------------------------------------------------------------------------------------------------------

static __thread bool nd_log_event_this = false;

static void nd_log_event(struct log_field *fields, size_t fields_max __maybe_unused) {
    if(!nd_log_event_this)
        return;

    nd_log_event_this = false;

    if(!nd_log.fatal_data_cb)
        return;

    const char *filename = log_field_strdupz(&fields[NDF_FILE]);
    const char *message = log_field_strdupz(&fields[NDF_MESSAGE]);
    const char *function = log_field_strdupz(&fields[NDF_FUNC]);
    const char *stack_trace = log_field_strdupz(&fields[NDF_STACK_TRACE]);
    const char *errno_str = log_field_strdupz(&fields[NDF_ERRNO]);
    long line = log_field_to_int64(&fields[NDF_LINE]);

    nd_log.fatal_data_cb(filename, function, message, errno_str, stack_trace, line);
}

void nd_log_register_fatal_data_cb(log_event_t cb) {
    nd_log.fatal_data_cb = cb;
}

// --------------------------------------------------------------------------------------------------------------------

void nd_log_register_fatal_final_cb(fatal_event_t cb) {
    nd_log.fatal_final_cb = cb;
}

// --------------------------------------------------------------------------------------------------------------------
// high level logger

static void nd_logger_log_fields(SPINLOCK *spinlock, FILE *fp, bool limit, ND_LOG_FIELD_PRIORITY priority,
                                 ND_LOG_METHOD output, struct nd_log_source *source,
                                 struct log_field *fields, size_t fields_max) {

    nd_log_event(fields, fields_max);

    if(spinlock)
        spinlock_lock(spinlock);

    // check the limits
    if(limit && nd_log_limit_reached(source))
        goto cleanup;

    if(output == NDLM_JOURNAL) {
        if(!nd_logger_journal_direct(fields, fields_max) && !nd_logger_journal_libsystemd(fields, fields_max)) {
            // we can't log to journal, let's log to stderr
            if(spinlock)
                spinlock_unlock(spinlock);

            output = NDLM_FILE;
            spinlock = &nd_log.std_error.spinlock;
            fp = stderr;

            if(spinlock)
                spinlock_lock(spinlock);
        }
    }

#if defined(OS_WINDOWS)
#if defined(HAVE_ETW)
    if(output == NDLM_ETW) {
        if(!nd_logger_etw(source, fields, fields_max)) {
            // we can't log to windows events, let's log to stderr
            if(spinlock)
                spinlock_unlock(spinlock);

            output = NDLM_FILE;
            spinlock = &nd_log.std_error.spinlock;
            fp = stderr;

            if(spinlock)
                spinlock_lock(spinlock);
        }
    }
#endif
#if defined(HAVE_WEL)
    if(output == NDLM_WEL) {
        if(!nd_logger_wel(source, fields, fields_max)) {
            // we can't log to windows events, let's log to stderr
            if(spinlock)
                spinlock_unlock(spinlock);

            output = NDLM_FILE;
            spinlock = &nd_log.std_error.spinlock;
            fp = stderr;

            if(spinlock)
                spinlock_lock(spinlock);
        }
    }
#endif
#endif

    if(output == NDLM_SYSLOG)
        nd_logger_syslog(priority, source->format, fields, fields_max);

    if(output == NDLM_FILE)
        nd_logger_file(fp, source->format, fields, fields_max);


cleanup:
    if(spinlock)
        spinlock_unlock(spinlock);
}

static void nd_logger_unset_all_thread_fields(void) {
    size_t fields_max = THREAD_FIELDS_MAX;
    for(size_t i = 0; i < fields_max ; i++)
        thread_log_fields[i].entry.set = false;
}

static void nd_logger_merge_log_stack_to_thread_fields(void) {
    for(size_t c = 0; c < thread_log_stack_next ;c++) {
        struct log_stack_entry *lgs = thread_log_stack_base[c];

        for(size_t i = 0; lgs[i].id != NDF_STOP ; i++) {
            if(lgs[i].id >= _NDF_MAX || !lgs[i].set)
                continue;

            struct log_stack_entry *e = &lgs[i];
            ND_LOG_STACK_FIELD_TYPE type = lgs[i].type;

            // do not add empty / unset fields
            if((type == NDFT_TXT && (!e->txt || !*e->txt)) ||
                (type == NDFT_BFR && (!e->bfr || !buffer_strlen(e->bfr))) ||
                (type == NDFT_STR && !e->str) ||
                (type == NDFT_UUID && (!e->uuid || uuid_is_null(*e->uuid))) ||
                (type == NDFT_CALLBACK && !e->cb.formatter) ||
                type == NDFT_UNSET)
                continue;

            thread_log_fields[lgs[i].id].entry = *e;
        }
    }
}

static void nd_logger(const char *file, const char *function, const unsigned long line,
               ND_LOG_SOURCES source, ND_LOG_FIELD_PRIORITY priority, bool limit,
               int saved_errno, size_t saved_winerror __maybe_unused, const char *fmt, va_list ap) {

    SPINLOCK *spinlock;
    FILE *fp;
    ND_LOG_METHOD output = nd_logger_select_output(source, &fp, &spinlock);
    if(!IS_FINAL_LOG_METHOD(output))
        return;

    // mark all fields as unset
    nd_logger_unset_all_thread_fields();

    // flatten the log stack into the fields
    nd_logger_merge_log_stack_to_thread_fields();

    // set the common fields that are automatically set by the logging subsystem

    if(likely(!thread_log_fields[NDF_STACK_TRACE].entry.set) && priority <= NDLP_WARNING)
        thread_log_fields[NDF_STACK_TRACE].entry = ND_LOG_FIELD_CB(NDF_STACK_TRACE, stack_trace_formatter, NULL);

    if(likely(!thread_log_fields[NDF_INVOCATION_ID].entry.set))
        thread_log_fields[NDF_INVOCATION_ID].entry = ND_LOG_FIELD_UUID(NDF_INVOCATION_ID, &nd_log.invocation_id);

    if(likely(!thread_log_fields[NDF_LOG_SOURCE].entry.set))
        thread_log_fields[NDF_LOG_SOURCE].entry = ND_LOG_FIELD_TXT(NDF_LOG_SOURCE, nd_log_id2source(source));
    else {
        ND_LOG_SOURCES src = source;

        if(thread_log_fields[NDF_LOG_SOURCE].entry.type == NDFT_TXT)
            src = nd_log_source2id(thread_log_fields[NDF_LOG_SOURCE].entry.txt, source);
        else if(thread_log_fields[NDF_LOG_SOURCE].entry.type == NDFT_U64)
            src = thread_log_fields[NDF_LOG_SOURCE].entry.u64;

        if(src != source && src < _NDLS_MAX) {
            source = src;
            output = nd_logger_select_output(source, &fp, &spinlock);
            if(output != NDLM_FILE && output != NDLM_JOURNAL && output != NDLM_SYSLOG)
                return;
        }
    }

    if(likely(!thread_log_fields[NDF_SYSLOG_IDENTIFIER].entry.set))
        thread_log_fields[NDF_SYSLOG_IDENTIFIER].entry = ND_LOG_FIELD_TXT(NDF_SYSLOG_IDENTIFIER, program_name);

    if(likely(!thread_log_fields[NDF_LINE].entry.set)) {
        thread_log_fields[NDF_LINE].entry = ND_LOG_FIELD_U64(NDF_LINE, line);
        thread_log_fields[NDF_FILE].entry = ND_LOG_FIELD_TXT(NDF_FILE, file);
        thread_log_fields[NDF_FUNC].entry = ND_LOG_FIELD_TXT(NDF_FUNC, function);
    }

    if(likely(!thread_log_fields[NDF_PRIORITY].entry.set)) {
        thread_log_fields[NDF_PRIORITY].entry = ND_LOG_FIELD_U64(NDF_PRIORITY, priority);
    }

    if(likely(!thread_log_fields[NDF_TID].entry.set))
        thread_log_fields[NDF_TID].entry = ND_LOG_FIELD_U64(NDF_TID, gettid_cached());

    if(likely(!thread_log_fields[NDF_THREAD_TAG].entry.set)) {
        const char *thread_tag = nd_thread_tag();
        thread_log_fields[NDF_THREAD_TAG].entry = ND_LOG_FIELD_TXT(NDF_THREAD_TAG, thread_tag);

        // TODO: fix the ND_MODULE in logging by setting proper module name in threads
//        if(!thread_log_fields[NDF_MODULE].entry.set)
//            thread_log_fields[NDF_MODULE].entry = ND_LOG_FIELD_CB(NDF_MODULE, thread_tag_to_module, (void *)thread_tag);
    }

    if(likely(!thread_log_fields[NDF_TIMESTAMP_REALTIME_USEC].entry.set))
        thread_log_fields[NDF_TIMESTAMP_REALTIME_USEC].entry = ND_LOG_FIELD_U64(NDF_TIMESTAMP_REALTIME_USEC, now_realtime_usec());

    if(saved_errno != 0 && !thread_log_fields[NDF_ERRNO].entry.set)
        thread_log_fields[NDF_ERRNO].entry = ND_LOG_FIELD_I64(NDF_ERRNO, saved_errno);

    if(saved_winerror != 0 && !thread_log_fields[NDF_WINERROR].entry.set)
        thread_log_fields[NDF_WINERROR].entry = ND_LOG_FIELD_U64(NDF_WINERROR, saved_winerror);

    CLEAN_BUFFER *wb = NULL;
    if(fmt && !thread_log_fields[NDF_MESSAGE].entry.set) {
        wb = buffer_create(1024, NULL);
        buffer_vsprintf(wb, fmt, ap);
        thread_log_fields[NDF_MESSAGE].entry = ND_LOG_FIELD_TXT(NDF_MESSAGE, buffer_tostring(wb));
    }

    nd_logger_log_fields(spinlock, fp, limit, priority, output, &nd_log.sources[source],
                         thread_log_fields, THREAD_FIELDS_MAX);

    if(nd_log.sources[source].pending_msg) {
        // log a pending message

        nd_logger_unset_all_thread_fields();

        thread_log_fields[NDF_TIMESTAMP_REALTIME_USEC].entry = (struct log_stack_entry){
                .set = true,
                .type = NDFT_U64,
                .u64 = now_realtime_usec(),
        };

        thread_log_fields[NDF_LOG_SOURCE].entry = (struct log_stack_entry){
                .set = true,
                .type = NDFT_TXT,
                .txt = nd_log_id2source(source),
        };

        thread_log_fields[NDF_SYSLOG_IDENTIFIER].entry = (struct log_stack_entry){
                .set = true,
                .type = NDFT_TXT,
                .txt = program_name,
        };

        thread_log_fields[NDF_MESSAGE].entry = (struct log_stack_entry){
                .set = true,
                .type = NDFT_TXT,
                .txt = nd_log.sources[source].pending_msg,
        };

        thread_log_fields[NDF_MESSAGE_ID].entry = (struct log_stack_entry){
            .set = nd_log.sources[source].pending_msgid != NULL,
            .type = NDFT_UUID,
            .uuid = nd_log.sources[source].pending_msgid,
        };

        nd_logger_log_fields(spinlock, fp, false, priority, output,
                             &nd_log.sources[source],
                             thread_log_fields, THREAD_FIELDS_MAX);

        freez((void *)nd_log.sources[source].pending_msg);
        nd_log.sources[source].pending_msg = NULL;
        nd_log.sources[source].pending_msgid = NULL;
    }

    errno_clear();
}

static ND_LOG_SOURCES nd_log_validate_source(ND_LOG_SOURCES source) {
    if(source >= _NDLS_MAX)
        source = NDLS_DAEMON;

    if(nd_log.overwrite_process_source)
        source = nd_log.overwrite_process_source;

    return source;
}

// --------------------------------------------------------------------------------------------------------------------
// public API for loggers

void netdata_logger(ND_LOG_SOURCES source, ND_LOG_FIELD_PRIORITY priority, const char *file, const char *function, unsigned long line, const char *fmt, ... )
{
    int saved_errno = errno;

    size_t saved_winerror = 0;
#if defined(OS_WINDOWS)
    saved_winerror = GetLastError();
#endif

    source = nd_log_validate_source(source);

    if (source != NDLS_DEBUG && priority > nd_log.sources[source].min_priority)
        return;

    va_list args;
    va_start(args, fmt);
    nd_logger(file, function, line, source, priority,
              source == NDLS_DAEMON || source == NDLS_COLLECTORS,
              saved_errno, saved_winerror, fmt, args);
    va_end(args);
}

void netdata_logger_with_limit(ERROR_LIMIT *erl, ND_LOG_SOURCES source, ND_LOG_FIELD_PRIORITY priority, const char *file __maybe_unused, const char *function __maybe_unused, const unsigned long line __maybe_unused, const char *fmt, ... ) {
    int saved_errno = errno;

    size_t saved_winerror = 0;
#if defined(OS_WINDOWS)
    saved_winerror = GetLastError();
#endif

    source = nd_log_validate_source(source);

    if (source != NDLS_DEBUG && priority > nd_log.sources[source].min_priority)
        return;

    if(erl->sleep_ut)
        sleep_usec(erl->sleep_ut);

    spinlock_lock(&erl->spinlock);

    erl->count++;
    time_t now = now_boottime_sec();
    if(now - erl->last_logged < erl->log_every) {
        spinlock_unlock(&erl->spinlock);
        return;
    }

    spinlock_unlock(&erl->spinlock);

    va_list args;
    va_start(args, fmt);
    nd_logger(file, function, line, source, priority,
            source == NDLS_DAEMON || source == NDLS_COLLECTORS,
            saved_errno, saved_winerror, fmt, args);
    va_end(args);
    erl->last_logged = now;
    erl->count = 0;
}

void netdata_logger_fatal(const char *file, const char *function, const unsigned long line, const char *fmt, ... ) {
    static size_t already_in_fatal = 0;

    size_t recursion = __atomic_add_fetch(&already_in_fatal, 1, __ATOMIC_SEQ_CST);
    if(recursion > 1) {
        // exit immediately, nothing more to be done
        sleep(2); // give the first fatal the chance to be written
        fprintf(stderr, "\nRECURSIVE FATAL STATEMENTS, latest from %s() of %lu@%s, EXITING NOW! 23e93dfccbf64e11aac858b9410d8a82\n",
                function, line, file);
        fflush(stderr);

#ifdef ENABLE_SENTRY
        abort();
#else
        _exit(1);
#endif
    }

    // send this event to deamon_status_file
    nd_log_event_this = true;

    int saved_errno = errno;
    size_t saved_winerror = 0;
#if defined(OS_WINDOWS)
    saved_winerror = GetLastError();
#endif

    // make sure the msg id does not leak
    {
        ND_LOG_STACK lgs[] = {
            ND_LOG_FIELD_UUID(NDF_MESSAGE_ID, &netdata_fatal_msgid),
            ND_LOG_FIELD_END(),
        };
        ND_LOG_STACK_PUSH(lgs);

        ND_LOG_SOURCES source = NDLS_DAEMON;
        source = nd_log_validate_source(source);

        va_list args;
        va_start(args, fmt);
        nd_logger(file, function, line, source, NDLP_ALERT, true, saved_errno, saved_winerror, fmt, args);
        va_end(args);
    }

    char date[LOG_DATE_LENGTH];
    log_date(date, LOG_DATE_LENGTH, now_realtime_sec());

    char action_data[70+1];
    snprintfz(action_data, 70, "%04lu@%-10.10s:%-15.15s/%d", line, file, function, saved_errno);

    const char *thread_tag = nd_thread_tag();
    const char *tag_to_send = thread_tag;

    // anonymize thread names
    if(strncmp(thread_tag, THREAD_TAG_STREAM_RECEIVER, strlen(THREAD_TAG_STREAM_RECEIVER)) == 0)
        tag_to_send = THREAD_TAG_STREAM_RECEIVER;
    if(strncmp(thread_tag, THREAD_TAG_STREAM_SENDER, strlen(THREAD_TAG_STREAM_SENDER)) == 0)
        tag_to_send = THREAD_TAG_STREAM_SENDER;

    char action_result[200+1];
    snprintfz(action_result, 60, "%s:%s:%s", program_name, tag_to_send, function);

#ifdef NETDATA_INTERNAL_CHECKS
    // abort();
#endif

    if(nd_log.fatal_final_cb)
        nd_log.fatal_final_cb();

    exit(1);
}
