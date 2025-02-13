// SPDX-License-Identifier: GPL-3.0-or-later

#include "nd_log-internals.h"

// --------------------------------------------------------------------------------------------------------------------
// workaround strerror_r()

#if defined(STRERROR_R_CHAR_P)
// GLIBC version of strerror_r
static const char *strerror_result(const char *a, const char *b) { (void)b; return a; }
#elif defined(HAVE_STRERROR_R)
// POSIX version of strerror_r
static const char *strerror_result(int a, const char *b) { (void)a; return b; }
#elif defined(HAVE_C__GENERIC)

// what a trick!
// http://stackoverflow.com/questions/479207/function-overloading-in-c
static const char *strerror_result_int(int a, const char *b) { (void)a; return b; }
static const char *strerror_result_string(const char *a, const char *b) { (void)b; return a; }

#define strerror_result(a, b) _Generic((a), \
    int: strerror_result_int, \
    char *: strerror_result_string \
    )(a, b)

#else
#error "cannot detect the format of function strerror_r()"
#endif

const char *errno2str(int errnum, char *buf, size_t size) {
    return strerror_result(strerror_r(errnum, buf, size), buf);
}

// --------------------------------------------------------------------------------------------------------------------
// logging method

static struct {
    ND_LOG_METHOD method;
    const char *name;
} nd_log_methods[] = {
    { .method = NDLM_DISABLED, .name = "none" },
    { .method = NDLM_DEVNULL, .name = "/dev/null" },
    { .method = NDLM_DEFAULT, .name = "default" },
    { .method = NDLM_JOURNAL, .name = "journal" },
    { .method = NDLM_SYSLOG, .name = "syslog" },
    { .method = NDLM_STDOUT, .name = "stdout" },
    { .method = NDLM_STDERR, .name = "stderr" },
    { .method = NDLM_FILE, .name = "file" },
#if defined(OS_WINDOWS)
#if defined(HAVE_ETW)
    { .method = NDLM_ETW, .name = ETW_NAME },
#endif
#if defined(HAVE_WEL)
        { .method = NDLM_WEL, .name = WEL_NAME },
#endif
#endif
};

ND_LOG_METHOD nd_log_method2id(const char *method) {
    if(!method || !*method)
        return NDLM_DEFAULT;

    size_t entries = sizeof(nd_log_methods) / sizeof(nd_log_methods[0]);
    for(size_t i = 0; i < entries ;i++) {
        if(strcmp(nd_log_methods[i].name, method) == 0)
            return nd_log_methods[i].method;
    }

    return NDLM_FILE;
}

const char *nd_log_id2method(ND_LOG_METHOD method) {
    size_t entries = sizeof(nd_log_methods) / sizeof(nd_log_methods[0]);
    for(size_t i = 0; i < entries ;i++) {
        if(method == nd_log_methods[i].method)
            return nd_log_methods[i].name;
    }

    return "unknown";
}

const char *nd_log_method_for_external_plugins(const char *s) {
    if(s && *s) {
        ND_LOG_METHOD method = nd_log_method2id(s);
        if(IS_VALID_LOG_METHOD_FOR_EXTERNAL_PLUGINS(method))
            return nd_log_id2method(method);
    }

    return nd_log_id2method(NDLM_STDERR);
}

// --------------------------------------------------------------------------------------------------------------------
// facilities
//
// sys/syslog.h (Linux)
// sys/sys/syslog.h (FreeBSD)
// bsd/sys/syslog.h (darwin-xnu)

static struct {
    int facility;
    const char *name;
} nd_log_facilities[] = {
    { LOG_AUTH, "auth" },
    { LOG_AUTHPRIV, "authpriv" },
    { LOG_CRON, "cron" },
    { LOG_DAEMON, "daemon" },
    { LOG_FTP, "ftp" },
    { LOG_KERN, "kern" },
    { LOG_LPR, "lpr" },
    { LOG_MAIL, "mail" },
    { LOG_NEWS, "news" },
    { LOG_SYSLOG, "syslog" },
    { LOG_USER, "user" },
    { LOG_UUCP, "uucp" },
    { LOG_LOCAL0, "local0" },
    { LOG_LOCAL1, "local1" },
    { LOG_LOCAL2, "local2" },
    { LOG_LOCAL3, "local3" },
    { LOG_LOCAL4, "local4" },
    { LOG_LOCAL5, "local5" },
    { LOG_LOCAL6, "local6" },
    { LOG_LOCAL7, "local7" },

#ifdef __FreeBSD__
    { LOG_CONSOLE, "console" },
    { LOG_NTP, "ntp" },

    // FreeBSD does not consider 'security' as deprecated.
    { LOG_SECURITY, "security" },
#else
    // For all other O/S 'security' is mapped to 'auth'.
    { LOG_AUTH, "security" },
#endif

#ifdef __APPLE__
    { LOG_INSTALL, "install" },
    { LOG_NETINFO, "netinfo" },
    { LOG_RAS,     "ras" },
    { LOG_REMOTEAUTH, "remoteauth" },
    { LOG_LAUNCHD, "launchd" },

#endif
};

int nd_log_facility2id(const char *facility) {
    size_t entries = sizeof(nd_log_facilities) / sizeof(nd_log_facilities[0]);
    for(size_t i = 0; i < entries ;i++) {
        if(strcmp(nd_log_facilities[i].name, facility) == 0)
            return nd_log_facilities[i].facility;
    }

    return LOG_DAEMON;
}

const char *nd_log_id2facility(int facility) {
    size_t entries = sizeof(nd_log_facilities) / sizeof(nd_log_facilities[0]);
    for(size_t i = 0; i < entries ;i++) {
        if(nd_log_facilities[i].facility == facility)
            return nd_log_facilities[i].name;
    }

    return "daemon";
}

// --------------------------------------------------------------------------------------------------------------------
// priorities

static struct {
    ND_LOG_FIELD_PRIORITY priority;
    const char *name;
} nd_log_priorities[] = {
    { .priority = NDLP_EMERG, .name = "emergency" },
    { .priority = NDLP_EMERG, .name = "emerg" },
    { .priority = NDLP_ALERT, .name = "alert" },
    { .priority = NDLP_CRIT, .name = "critical" },
    { .priority = NDLP_CRIT, .name = "crit" },
    { .priority = NDLP_ERR, .name = "error" },
    { .priority = NDLP_ERR, .name = "err" },
    { .priority = NDLP_WARNING, .name = "warning" },
    { .priority = NDLP_WARNING, .name = "warn" },
    { .priority = NDLP_NOTICE, .name = "notice" },
    { .priority = NDLP_INFO, .name = NDLP_INFO_STR },
    { .priority = NDLP_DEBUG, .name = "debug" },
};

int nd_log_priority2id(const char *priority) {
    size_t entries = sizeof(nd_log_priorities) / sizeof(nd_log_priorities[0]);
    for(size_t i = 0; i < entries ;i++) {
        if(strcmp(nd_log_priorities[i].name, priority) == 0)
            return nd_log_priorities[i].priority;
    }

    return NDLP_INFO;
}

const char *nd_log_id2priority(ND_LOG_FIELD_PRIORITY priority) {
    size_t entries = sizeof(nd_log_priorities) / sizeof(nd_log_priorities[0]);
    for(size_t i = 0; i < entries ;i++) {
        if(priority == nd_log_priorities[i].priority)
            return nd_log_priorities[i].name;
    }

    return NDLP_INFO_STR;
}

// --------------------------------------------------------------------------------------------------------------------
// log sources

const char *nd_log_sources[] = {
    [NDLS_UNSET] = "UNSET",
    [NDLS_ACCESS] = "access",
    [NDLS_ACLK] = "aclk",
    [NDLS_COLLECTORS] = "collector",
    [NDLS_DAEMON] = "daemon",
    [NDLS_HEALTH] = "health",
    [NDLS_DEBUG] = "debug",
};

size_t nd_log_source2id(const char *source, ND_LOG_SOURCES def) {
    size_t entries = sizeof(nd_log_sources) / sizeof(nd_log_sources[0]);
    for(size_t i = 0; i < entries ;i++) {
        if(strcmp(nd_log_sources[i], source) == 0)
            return i;
    }

    return def;
}


const char *nd_log_id2source(ND_LOG_SOURCES source) {
    size_t entries = sizeof(nd_log_sources) / sizeof(nd_log_sources[0]);
    if(source < entries)
        return nd_log_sources[source];

    return nd_log_sources[NDLS_COLLECTORS];
}

// --------------------------------------------------------------------------------------------------------------------
// log output formats

static struct {
    ND_LOG_FORMAT format;
    const char *name;
} nd_log_formats[] = {
    { .format = NDLF_JOURNAL, .name = "journal" },
    { .format = NDLF_LOGFMT, .name = "logfmt" },
    { .format = NDLF_JSON, .name = "json" },
#if defined(OS_WINDOWS)
#if defined(HAVE_ETW)
        { .format = NDLF_ETW, .name = ETW_NAME },
#endif
#if defined(HAVE_WEL)
    { .format = NDLF_WEL, .name = WEL_NAME },
#endif
#endif
};

ND_LOG_FORMAT nd_log_format2id(const char *format) {
    if(!format || !*format)
        return NDLF_LOGFMT;

    size_t entries = sizeof(nd_log_formats) / sizeof(nd_log_formats[0]);
    for(size_t i = 0; i < entries ;i++) {
        if(strcmp(nd_log_formats[i].name, format) == 0)
            return nd_log_formats[i].format;
    }

    return NDLF_LOGFMT;
}

const char *nd_log_id2format(ND_LOG_FORMAT format) {
    size_t entries = sizeof(nd_log_formats) / sizeof(nd_log_formats[0]);
    for(size_t i = 0; i < entries ;i++) {
        if(format == nd_log_formats[i].format)
            return nd_log_formats[i].name;
    }

    return "logfmt";
}

// --------------------------------------------------------------------------------------------------------------------

struct nd_log nd_log = {
    .overwrite_process_source = 0,
    .journal = {
        .initialized = false,
        .first_msg = false,
        .fd = -1,
    },
    .journal_direct = {
        .initialized = false,
        .fd = -1,
    },
    .syslog = {
        .initialized = false,
        .facility = LOG_DAEMON,
    },
#if defined(OS_WINDOWS)
    .eventlog =  {
        .initialized =  false,
    },
#endif
    .std_output = {
        .spinlock = SPINLOCK_INITIALIZER,
        .initialized = false,
    },
    .std_error = {
        .spinlock = SPINLOCK_INITIALIZER,
        .initialized = false,
    },
    .sources = {
        [NDLS_UNSET] = {
            .spinlock = SPINLOCK_INITIALIZER,
            .method = NDLM_DISABLED,
            .format = NDLF_JOURNAL,
            .filename = NULL,
            .fd = -1,
            .fp = NULL,
            .min_priority = NDLP_EMERG,
            .limits = ND_LOG_LIMITS_UNLIMITED,
        },
        [NDLS_ACCESS] = {
            .spinlock = SPINLOCK_INITIALIZER,
            .method = NDLM_DEFAULT,
            .format = NDLF_LOGFMT,
            .filename = LOG_DIR "/access.log",
            .fd = -1,
            .fp = NULL,
            .min_priority = NDLP_DEBUG,
            .limits = ND_LOG_LIMITS_UNLIMITED,
        },
        [NDLS_ACLK] = {
            .spinlock = SPINLOCK_INITIALIZER,
            .method = NDLM_FILE,
            .format = NDLF_LOGFMT,
            .filename = LOG_DIR "/aclk.log",
            .fd = -1,
            .fp = NULL,
            .min_priority = NDLP_DEBUG,
            .limits = ND_LOG_LIMITS_UNLIMITED,
        },
        [NDLS_COLLECTORS] = {
            .spinlock = SPINLOCK_INITIALIZER,
            .method = NDLM_DEFAULT,
            .format = NDLF_LOGFMT,
            .filename = LOG_DIR "/collector.log",
            .fd = STDERR_FILENO,
            .fp = NULL,
            .min_priority = NDLP_INFO,
            .limits = ND_LOG_LIMITS_DEFAULT,
        },
        [NDLS_DEBUG] = {
            .spinlock = SPINLOCK_INITIALIZER,
            .method = NDLM_DISABLED,
            .format = NDLF_LOGFMT,
            .filename = LOG_DIR "/debug.log",
            .fd = STDOUT_FILENO,
            .fp = NULL,
            .min_priority = NDLP_DEBUG,
            .limits = ND_LOG_LIMITS_UNLIMITED,
        },
        [NDLS_DAEMON] = {
            .spinlock = SPINLOCK_INITIALIZER,
            .method = NDLM_DEFAULT,
            .filename = LOG_DIR "/daemon.log",
            .format = NDLF_LOGFMT,
            .fd = -1,
            .fp = NULL,
            .min_priority = NDLP_INFO,
            .limits = ND_LOG_LIMITS_DEFAULT,
        },
        [NDLS_HEALTH] = {
            .spinlock = SPINLOCK_INITIALIZER,
            .method = NDLM_DEFAULT,
            .format = NDLF_LOGFMT,
            .filename = LOG_DIR "/health.log",
            .fd = -1,
            .fp = NULL,
            .min_priority = NDLP_DEBUG,
            .limits = ND_LOG_LIMITS_UNLIMITED,
        },
    },
};

// --------------------------------------------------------------------------------------------------------------------

__thread struct log_stack_entry *thread_log_stack_base[THREAD_LOG_STACK_MAX];
__thread size_t thread_log_stack_next = 0;
__thread struct log_field thread_log_fields[_NDF_MAX] = {
    // THE ORDER HERE IS IRRELEVANT (but keep them sorted by their number)

    [NDF_STOP] = { // processing will not stop on this - so it is ok to be first
        .journal = NULL,
        .logfmt = NULL,
        .eventlog = NULL,
        .logfmt_annotator = NULL,
    },
    [NDF_TIMESTAMP_REALTIME_USEC] = {
        .journal = NULL,
        .eventlog = "Timestamp",
        .logfmt = "time",
        .logfmt_annotator = timestamp_usec_annotator,
    },
    [NDF_SYSLOG_IDENTIFIER] = {
        .journal = "SYSLOG_IDENTIFIER", // standard journald field
        .eventlog = "Program",
        .logfmt = "comm",
    },
    [NDF_LOG_SOURCE] = {
        .journal = "ND_LOG_SOURCE",
        .eventlog = "NetdataLogSource",
        .logfmt = "source",
    },
    [NDF_PRIORITY] = {
        .journal = "PRIORITY", // standard journald field
        .eventlog = "Level",
        .logfmt = "level",
        .logfmt_annotator = priority_annotator,
    },
    [NDF_ERRNO] = {
        .journal = "ERRNO", // standard journald field
        .eventlog = "UnixErrno",
        .logfmt = "errno",
        .logfmt_annotator = errno_annotator,
    },
    [NDF_WINERROR] = {
#if defined(OS_WINDOWS)
        .journal = "WINERROR",
        .eventlog = "WindowsLastError",
        .logfmt = "winerror",
        .logfmt_annotator = winerror_annotator,
#endif
    },
    [NDF_INVOCATION_ID] = {
        .journal = "INVOCATION_ID", // standard journald field
        .eventlog = "InvocationID",
        .logfmt = NULL,
    },
    [NDF_LINE] = {
        .journal = "CODE_LINE", // standard journald field
        .eventlog = "CodeLine",
        .logfmt = NULL,
    },
    [NDF_FILE] = {
        .journal = "CODE_FILE", // standard journald field
        .eventlog = "CodeFile",
        .logfmt = NULL,
    },
    [NDF_FUNC] = {
        .journal = "CODE_FUNC", // standard journald field
        .eventlog = "CodeFunction",
        .logfmt = NULL,
    },
    [NDF_TID] = {
        .journal = "TID", // standard journald field
        .eventlog = "ThreadID",
        .logfmt = "tid",
    },
    [NDF_THREAD_TAG] = {
        .journal = "THREAD_TAG",
        .eventlog = "ThreadName",
        .logfmt = "thread",
    },
    [NDF_MESSAGE_ID] = {
        .journal = "MESSAGE_ID",
        .eventlog = "MessageID",
        .logfmt = "msg_id",
    },
    [NDF_MODULE] = {
        .journal = "ND_MODULE",
        .eventlog = "Module",
        .logfmt = "module",
    },
    [NDF_NIDL_NODE] = {
        .journal = "ND_NIDL_NODE",
        .eventlog = "Node",
        .logfmt = "node",
    },
    [NDF_NIDL_INSTANCE] = {
        .journal = "ND_NIDL_INSTANCE",
        .eventlog = "Instance",
        .logfmt = "instance",
    },
    [NDF_NIDL_CONTEXT] = {
        .journal = "ND_NIDL_CONTEXT",
        .eventlog = "Context",
        .logfmt = "context",
    },
    [NDF_NIDL_DIMENSION] = {
        .journal = "ND_NIDL_DIMENSION",
        .eventlog = "Dimension",
        .logfmt = "dimension",
    },
    [NDF_SRC_TRANSPORT] = {
        .journal = "ND_SRC_TRANSPORT",
        .eventlog = "SourceTransport",
        .logfmt = "src_transport",
    },
    [NDF_ACCOUNT_ID] = {
        .journal = "ND_ACCOUNT_ID",
        .eventlog = "AccountID",
        .logfmt = "account",
    },
    [NDF_USER_NAME] = {
        .journal = "ND_USER_NAME",
        .eventlog = "UserName",
        .logfmt = "user",
    },
    [NDF_USER_ROLE] = {
        .journal = "ND_USER_ROLE",
        .eventlog = "UserRole",
        .logfmt = "role",
    },
    [NDF_USER_ACCESS] = {
        .journal = "ND_USER_PERMISSIONS",
        .eventlog = "UserPermissions",
        .logfmt = "permissions",
    },
    [NDF_SRC_IP] = {
        .journal = "ND_SRC_IP",
        .eventlog = "SourceIP",
        .logfmt = "src_ip",
    },
    [NDF_SRC_FORWARDED_HOST] = {
        .journal = "ND_SRC_FORWARDED_HOST",
        .eventlog = "SourceForwardedHost",
        .logfmt = "src_forwarded_host",
    },
    [NDF_SRC_FORWARDED_FOR] = {
        .journal = "ND_SRC_FORWARDED_FOR",
        .eventlog = "SourceForwardedFor",
        .logfmt = "src_forwarded_for",
    },
    [NDF_SRC_PORT] = {
        .journal = "ND_SRC_PORT",
        .eventlog = "SourcePort",
        .logfmt = "src_port",
    },
    [NDF_SRC_CAPABILITIES] = {
        .journal = "ND_SRC_CAPABILITIES",
        .eventlog = "SourceCapabilities",
        .logfmt = "src_capabilities",
    },
    [NDF_DST_TRANSPORT] = {
        .journal = "ND_DST_TRANSPORT",
        .eventlog = "DestinationTransport",
        .logfmt = "dst_transport",
    },
    [NDF_DST_IP] = {
        .journal = "ND_DST_IP",
        .eventlog = "DestinationIP",
        .logfmt = "dst_ip",
    },
    [NDF_DST_PORT] = {
        .journal = "ND_DST_PORT",
        .eventlog = "DestinationPort",
        .logfmt = "dst_port",
    },
    [NDF_DST_CAPABILITIES] = {
        .journal = "ND_DST_CAPABILITIES",
        .eventlog = "DestinationCapabilities",
        .logfmt = "dst_capabilities",
    },
    [NDF_REQUEST_METHOD] = {
        .journal = "ND_REQUEST_METHOD",
        .eventlog = "RequestMethod",
        .logfmt = "req_method",
    },
    [NDF_RESPONSE_CODE] = {
        .journal = "ND_RESPONSE_CODE",
        .eventlog = "ResponseCode",
        .logfmt = "code",
    },
    [NDF_CONNECTION_ID] = {
        .journal = "ND_CONNECTION_ID",
        .eventlog = "ConnectionID",
        .logfmt = "conn",
    },
    [NDF_TRANSACTION_ID] = {
        .journal = "ND_TRANSACTION_ID",
        .eventlog = "TransactionID",
        .logfmt = "transaction",
    },
    [NDF_RESPONSE_SENT_BYTES] = {
        .journal = "ND_RESPONSE_SENT_BYTES",
        .eventlog = "ResponseSentBytes",
        .logfmt = "sent_bytes",
    },
    [NDF_RESPONSE_SIZE_BYTES] = {
        .journal = "ND_RESPONSE_SIZE_BYTES",
        .eventlog = "ResponseSizeBytes",
        .logfmt = "size_bytes",
    },
    [NDF_RESPONSE_PREPARATION_TIME_USEC] = {
        .journal = "ND_RESPONSE_PREP_TIME_USEC",
        .eventlog = "ResponsePreparationTimeUsec",
        .logfmt = "prep_ut",
    },
    [NDF_RESPONSE_SENT_TIME_USEC] = {
        .journal = "ND_RESPONSE_SENT_TIME_USEC",
        .eventlog = "ResponseSentTimeUsec",
        .logfmt = "sent_ut",
    },
    [NDF_RESPONSE_TOTAL_TIME_USEC] = {
        .journal = "ND_RESPONSE_TOTAL_TIME_USEC",
        .eventlog = "ResponseTotalTimeUsec",
        .logfmt = "total_ut",
    },
    [NDF_ALERT_ID] = {
        .journal = "ND_ALERT_ID",
        .eventlog = "AlertID",
        .logfmt = "alert_id",
    },
    [NDF_ALERT_UNIQUE_ID] = {
        .journal = "ND_ALERT_UNIQUE_ID",
        .eventlog = "AlertUniqueID",
        .logfmt = "alert_unique_id",
    },
    [NDF_ALERT_TRANSITION_ID] = {
        .journal = "ND_ALERT_TRANSITION_ID",
        .eventlog = "AlertTransitionID",
        .logfmt = "alert_transition_id",
    },
    [NDF_ALERT_EVENT_ID] = {
        .journal = "ND_ALERT_EVENT_ID",
        .eventlog = "AlertEventID",
        .logfmt = "alert_event_id",
    },
    [NDF_ALERT_CONFIG_HASH] = {
        .journal = "ND_ALERT_CONFIG",
        .eventlog = "AlertConfig",
        .logfmt = "alert_config",
    },
    [NDF_ALERT_NAME] = {
        .journal = "ND_ALERT_NAME",
        .eventlog = "AlertName",
        .logfmt = "alert",
    },
    [NDF_ALERT_CLASS] = {
        .journal = "ND_ALERT_CLASS",
        .eventlog = "AlertClass",
        .logfmt = "alert_class",
    },
    [NDF_ALERT_COMPONENT] = {
        .journal = "ND_ALERT_COMPONENT",
        .eventlog = "AlertComponent",
        .logfmt = "alert_component",
    },
    [NDF_ALERT_TYPE] = {
        .journal = "ND_ALERT_TYPE",
        .eventlog = "AlertType",
        .logfmt = "alert_type",
    },
    [NDF_ALERT_EXEC] = {
        .journal = "ND_ALERT_EXEC",
        .eventlog = "AlertExec",
        .logfmt = "alert_exec",
    },
    [NDF_ALERT_RECIPIENT] = {
        .journal = "ND_ALERT_RECIPIENT",
        .eventlog = "AlertRecipient",
        .logfmt = "alert_recipient",
    },
    [NDF_ALERT_VALUE] = {
        .journal = "ND_ALERT_VALUE",
        .eventlog = "AlertValue",
        .logfmt = "alert_value",
    },
    [NDF_ALERT_VALUE_OLD] = {
        .journal = "ND_ALERT_VALUE_OLD",
        .eventlog = "AlertOldValue",
        .logfmt = "alert_value_old",
    },
    [NDF_ALERT_STATUS] = {
        .journal = "ND_ALERT_STATUS",
        .eventlog = "AlertStatus",
        .logfmt = "alert_status",
    },
    [NDF_ALERT_STATUS_OLD] = {
        .journal = "ND_ALERT_STATUS_OLD",
        .eventlog = "AlertOldStatus",
        .logfmt = "alert_value_old",
    },
    [NDF_ALERT_UNITS] = {
        .journal = "ND_ALERT_UNITS",
        .eventlog = "AlertUnits",
        .logfmt = "alert_units",
    },
    [NDF_ALERT_SUMMARY] = {
        .journal = "ND_ALERT_SUMMARY",
        .eventlog = "AlertSummary",
        .logfmt = "alert_summary",
    },
    [NDF_ALERT_INFO] = {
        .journal = "ND_ALERT_INFO",
        .eventlog = "AlertInfo",
        .logfmt = "alert_info",
    },
    [NDF_ALERT_DURATION] = {
        .journal = "ND_ALERT_DURATION",
        .eventlog = "AlertDuration",
        .logfmt = "alert_duration",
    },
    [NDF_ALERT_NOTIFICATION_REALTIME_USEC] = {
        .journal = "ND_ALERT_NOTIFICATION_TIMESTAMP_USEC",
        .eventlog = "AlertNotificationTime",
        .logfmt = "alert_notification_timestamp",
        .logfmt_annotator = timestamp_usec_annotator,
    },
    [NDF_REQUEST] = {
        .journal = "ND_REQUEST",
        .eventlog = "Request",
        .logfmt = "request",
    },
    [NDF_MESSAGE] = {
        .journal = "MESSAGE",
        .eventlog = "Message",
        .logfmt = "msg",
    },
    [NDF_STACK_TRACE] = {
            .journal = "ND_STACK_TRACE",
            .eventlog = "StackTrace",
            .logfmt = NULL,
    },

    // put new items here
};

// --------------------------------------------------------------------------------------------------------------------

void log_stack_pop(void *ptr) {
    if(!ptr) return;

    struct log_stack_entry *lgs = *(struct log_stack_entry (*)[])ptr;

    if(unlikely(!thread_log_stack_next || lgs != thread_log_stack_base[thread_log_stack_next - 1])) {
        fatal("You cannot pop in the middle of the stack, or an item not in the stack");
        return;
    }

    thread_log_stack_next--;
}

void log_stack_push(struct log_stack_entry *lgs) {
    if(!lgs || thread_log_stack_next >= THREAD_LOG_STACK_MAX) return;
    thread_log_stack_base[thread_log_stack_next++] = lgs;
}

// --------------------------------------------------------------------------------------------------------------------

ND_LOG_FIELD_ID nd_log_field_id_by_journal_name(const char *field, size_t len) {
    for(size_t i = 0; i < THREAD_FIELDS_MAX ;i++) {
        if(thread_log_fields[i].journal && strlen(thread_log_fields[i].journal) == len && strncmp(field, thread_log_fields[i].journal, len) == 0)
            return i;
    }

    return NDF_STOP;
}

// --------------------------------------------------------------------------------------------------------------------

int nd_log_health_fd(void) {
    if(nd_log.sources[NDLS_HEALTH].method == NDLM_FILE && nd_log.sources[NDLS_HEALTH].fd != -1)
        return nd_log.sources[NDLS_HEALTH].fd;

    return STDERR_FILENO;
}

int nd_log_collectors_fd(void) {
    if(nd_log.sources[NDLS_COLLECTORS].method == NDLM_FILE && nd_log.sources[NDLS_COLLECTORS].fd != -1)
        return nd_log.sources[NDLS_COLLECTORS].fd;

    return STDERR_FILENO;
}

// --------------------------------------------------------------------------------------------------------------------

void log_date(char *buffer, size_t len, time_t now) {
    if(unlikely(!buffer || !len))
        return;

    time_t t = now;
    struct tm *tmp, tmbuf;

    tmp = localtime_r(&t, &tmbuf);

    if (unlikely(!tmp)) {
        buffer[0] = '\0';
        return;
    }

    if (unlikely(strftime(buffer, len, "%Y-%m-%d %H:%M:%S", tmp) == 0))
        buffer[0] = '\0';

    buffer[len - 1] = '\0';
}

// --------------------------------------------------------------------------------------------------------------------

bool nd_log_replace_existing_fd(struct nd_log_source *e, int new_fd) {
    if(new_fd == -1 || e->fd == -1 ||
        (e->fd == STDOUT_FILENO && nd_log.std_output.initialized) ||
        (e->fd == STDERR_FILENO && nd_log.std_error.initialized))
        return false;

    if(new_fd != e->fd) {
        int t = dup2(new_fd, e->fd);

        bool ret = true;
        if (t == -1) {
            netdata_log_error("Cannot dup2() new fd %d to old fd %d for '%s'", new_fd, e->fd, e->filename);
            ret = false;
        }
        else
            close(new_fd);

        if(e->fd == STDOUT_FILENO)
            nd_log.std_output.initialized = true;
        else if(e->fd == STDERR_FILENO)
            nd_log.std_error.initialized = true;

        return ret;
    }

    return false;
}
