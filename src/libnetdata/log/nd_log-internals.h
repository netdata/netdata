// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_ND_LOG_INTERNALS_H
#define NETDATA_ND_LOG_INTERNALS_H

#include "../libnetdata.h"

#ifdef __FreeBSD__
#include <sys/endian.h>
#endif

#ifdef __APPLE__
#include <machine/endian.h>
#endif

#if !defined(ENABLE_SENTRY) && defined(HAVE_BACKTRACE)
#include <execinfo.h>
#endif

#ifdef HAVE_SYSTEMD
#include <systemd/sd-journal.h>
#endif

const char *errno2str(int errnum, char *buf, size_t size);

// --------------------------------------------------------------------------------------------------------------------
// ND_LOG_METHOD

typedef enum  __attribute__((__packed__)) {
    NDLM_DISABLED = 0,
    NDLM_DEVNULL,
    NDLM_DEFAULT,
    NDLM_JOURNAL,
    NDLM_SYSLOG,
    NDLM_STDOUT,
    NDLM_STDERR,
    NDLM_FILE,
#if defined(OS_WINDOWS)
#if defined(HAVE_ETW)
    NDLM_ETW,
#endif
#if defined(HAVE_WEL)
    NDLM_WEL,
#endif
#endif
} ND_LOG_METHOD;

// all the log methods are finally mapped to these
#if defined(HAVE_ETW)
#define ETW_CONDITION(ndlo) ((ndlo) == NDLM_ETW)
#else
#define ETW_CONDITION(ndlo) (false)
#endif

#if defined(HAVE_WEL)
#define WEL_CONDITION(ndlo) ((ndlo) == NDLM_WEL)
#else
#define WEL_CONDITION(ndlo) (false)
#endif

#define IS_VALID_LOG_METHOD_FOR_EXTERNAL_PLUGINS(ndlo) ((ndlo) == NDLM_JOURNAL || (ndlo) == NDLM_SYSLOG || (ndlo) == NDLM_STDERR || ETW_CONDITION(ndlo) || WEL_CONDITION(ndlo))
#define IS_FINAL_LOG_METHOD(ndlo) ((ndlo) == NDLM_FILE || (ndlo) == NDLM_JOURNAL || (ndlo) == NDLM_SYSLOG || ETW_CONDITION(ndlo) || WEL_CONDITION(ndlo))

ND_LOG_METHOD nd_log_method2id(const char *method);
const char *nd_log_id2method(ND_LOG_METHOD method);

// --------------------------------------------------------------------------------------------------------------------
// ND_LOG_FORMAT

typedef enum __attribute__((__packed__)) {
    NDLF_JOURNAL,
    NDLF_LOGFMT,
    NDLF_JSON,
#if defined(OS_WINDOWS)
#if defined(HAVE_ETW)
    NDLF_ETW, // Event Tracing for Windows
#endif
#if defined(HAVE_WEL)
    NDLF_WEL, // Windows Event Log
#endif
#endif
} ND_LOG_FORMAT;

#define ETW_NAME "etw"
#define WEL_NAME "wel"

const char *nd_log_id2format(ND_LOG_FORMAT format);
ND_LOG_FORMAT nd_log_format2id(const char *format);

size_t nd_log_source2id(const char *source, ND_LOG_SOURCES def);
const char *nd_log_id2source(ND_LOG_SOURCES source);

const char *nd_log_id2priority(ND_LOG_FIELD_PRIORITY priority);
int nd_log_priority2id(const char *priority);

const char *nd_log_id2facility(int facility);
int nd_log_facility2id(const char *facility);

#include "nd_log_limit.h"

struct nd_log_source {
    SPINLOCK spinlock;
    ND_LOG_METHOD method;
    ND_LOG_FORMAT format;
    const char *filename;
    int fd;
    FILE *fp;

    ND_LOG_FIELD_PRIORITY min_priority;
    const nd_uuid_t *pending_msgid;
    const char *pending_msg;
    struct nd_log_limit limits;

#if defined(OS_WINDOWS)
    ND_LOG_SOURCES source;
    HANDLE hEventLog;
    USHORT channelID;
    UCHAR Opcode;
    USHORT Task;
    ULONGLONG Keyword;
#endif
};

struct nd_log {
    nd_uuid_t invocation_id;

    ND_LOG_SOURCES overwrite_process_source;
    log_event_t fatal_hook_cb;
    fatal_event_t fatal_final_cb;

    struct nd_log_source sources[_NDLS_MAX];

    struct {
        bool initialized;
        bool first_msg;
        int fd;             // we don't control this, we just detect it to keep it open
    } journal;

    struct {
        bool initialized;
        int fd;
        char filename[FILENAME_MAX];
    } journal_direct;

    struct {
        bool initialized;
        int facility;
    } syslog;

    struct {
        bool etw; // when set use etw, otherwise wel
        bool initialized;
        bool provider_enabled;  // track etw provider state
        SPINLOCK provider_lock; // Protect etw provider state access
    } eventlog;

    struct {
        SPINLOCK spinlock;
        bool initialized;
    } std_output;

    struct {
        SPINLOCK spinlock;
        bool initialized;
    } std_error;

};

// --------------------------------------------------------------------------------------------------------------------

struct log_field;
typedef const char *(*annotator_t)(struct log_field *lf);

struct log_field {
    const char *journal;
    const char *logfmt;
    const char *eventlog;
    annotator_t logfmt_annotator;
    struct log_stack_entry entry;
};

#define THREAD_LOG_STACK_MAX 50
#define THREAD_FIELDS_MAX (sizeof(thread_log_fields) / sizeof(thread_log_fields[0]))

extern __thread struct log_stack_entry *thread_log_stack_base[THREAD_LOG_STACK_MAX];
extern __thread size_t thread_log_stack_next;
extern __thread struct log_field thread_log_fields[_NDF_MAX];

// --------------------------------------------------------------------------------------------------------------------

extern struct nd_log nd_log;
bool nd_log_replace_existing_fd(struct nd_log_source *e, int new_fd);
void nd_log_open(struct nd_log_source *e, ND_LOG_SOURCES source);
void nd_log_stdin_init(int fd, const char *filename);

// --------------------------------------------------------------------------------------------------------------------
// annotators

struct log_field;
const char *errno_annotator(struct log_field *lf);
const char *priority_annotator(struct log_field *lf);
const char *timestamp_usec_annotator(struct log_field *lf);

extern bool nd_log_forked;
bool stack_trace_formatter(BUFFER *wb, void *data);

#if defined(OS_WINDOWS)
const char *winerror_annotator(struct log_field *lf);
#endif

// --------------------------------------------------------------------------------------------------------------------
// field formatters

uint64_t log_field_to_uint64(struct log_field *lf);
int64_t log_field_to_int64(struct log_field *lf);
const char *log_field_strdupz(struct log_field *lf);

// --------------------------------------------------------------------------------------------------------------------
// common text formatters

void nd_logger_logfmt(BUFFER *wb, struct log_field *fields, size_t fields_max);
void nd_logger_json(BUFFER *wb, struct log_field *fields, size_t fields_max);

// --------------------------------------------------------------------------------------------------------------------
// output to syslog

void nd_log_init_syslog(void);
void nd_log_reset_syslog(void);
bool nd_logger_syslog(int priority, ND_LOG_FORMAT format, struct log_field *fields, size_t fields_max);

// --------------------------------------------------------------------------------------------------------------------
// output to systemd-journal

bool nd_log_journal_systemd_init(void);
bool nd_log_journal_direct_init(const char *path);
bool nd_logger_journal_direct(struct log_field *fields, size_t fields_max);
bool nd_logger_journal_libsystemd(struct log_field *fields, size_t fields_max);

// --------------------------------------------------------------------------------------------------------------------
// output to file

bool nd_logger_file(FILE *fp, ND_LOG_FORMAT format, struct log_field *fields, size_t fields_max);

// --------------------------------------------------------------------------------------------------------------------
// output to windows events log

#if defined(OS_WINDOWS)
#if defined(HAVE_ETW)
bool nd_log_init_etw(void);
bool nd_logger_etw(struct nd_log_source *source, struct log_field *fields, size_t fields_max);
#endif
#if defined(HAVE_WEL)
bool nd_log_init_wel(void);
bool nd_logger_wel(struct nd_log_source *source, struct log_field *fields, size_t fields_max);
#endif
#endif

#endif //NETDATA_ND_LOG_INTERNALS_H
