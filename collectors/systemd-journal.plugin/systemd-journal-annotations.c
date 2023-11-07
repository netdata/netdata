// SPDX-License-Identifier: GPL-3.0-or-later

#include "systemd-internals.h"

const char *errno_map[] = {
        [1] = "1 (EPERM)",          // "Operation not permitted",
        [2] = "2 (ENOENT)",         // "No such file or directory",
        [3] = "3 (ESRCH)",          // "No such process",
        [4] = "4 (EINTR)",          // "Interrupted system call",
        [5] = "5 (EIO)",            // "Input/output error",
        [6] = "6 (ENXIO)",          // "No such device or address",
        [7] = "7 (E2BIG)",          // "Argument list too long",
        [8] = "8 (ENOEXEC)",        // "Exec format error",
        [9] = "9 (EBADF)",          // "Bad file descriptor",
        [10] = "10 (ECHILD)",        // "No child processes",
        [11] = "11 (EAGAIN)",        // "Resource temporarily unavailable",
        [12] = "12 (ENOMEM)",        // "Cannot allocate memory",
        [13] = "13 (EACCES)",        // "Permission denied",
        [14] = "14 (EFAULT)",        // "Bad address",
        [15] = "15 (ENOTBLK)",       // "Block device required",
        [16] = "16 (EBUSY)",         // "Device or resource busy",
        [17] = "17 (EEXIST)",        // "File exists",
        [18] = "18 (EXDEV)",         // "Invalid cross-device link",
        [19] = "19 (ENODEV)",        // "No such device",
        [20] = "20 (ENOTDIR)",       // "Not a directory",
        [21] = "21 (EISDIR)",        // "Is a directory",
        [22] = "22 (EINVAL)",        // "Invalid argument",
        [23] = "23 (ENFILE)",        // "Too many open files in system",
        [24] = "24 (EMFILE)",        // "Too many open files",
        [25] = "25 (ENOTTY)",        // "Inappropriate ioctl for device",
        [26] = "26 (ETXTBSY)",       // "Text file busy",
        [27] = "27 (EFBIG)",         // "File too large",
        [28] = "28 (ENOSPC)",        // "No space left on device",
        [29] = "29 (ESPIPE)",        // "Illegal seek",
        [30] = "30 (EROFS)",         // "Read-only file system",
        [31] = "31 (EMLINK)",        // "Too many links",
        [32] = "32 (EPIPE)",         // "Broken pipe",
        [33] = "33 (EDOM)",          // "Numerical argument out of domain",
        [34] = "34 (ERANGE)",        // "Numerical result out of range",
        [35] = "35 (EDEADLK)",       // "Resource deadlock avoided",
        [36] = "36 (ENAMETOOLONG)",  // "File name too long",
        [37] = "37 (ENOLCK)",        // "No locks available",
        [38] = "38 (ENOSYS)",        // "Function not implemented",
        [39] = "39 (ENOTEMPTY)",     // "Directory not empty",
        [40] = "40 (ELOOP)",         // "Too many levels of symbolic links",
        [42] = "42 (ENOMSG)",        // "No message of desired type",
        [43] = "43 (EIDRM)",         // "Identifier removed",
        [44] = "44 (ECHRNG)",        // "Channel number out of range",
        [45] = "45 (EL2NSYNC)",      // "Level 2 not synchronized",
        [46] = "46 (EL3HLT)",        // "Level 3 halted",
        [47] = "47 (EL3RST)",        // "Level 3 reset",
        [48] = "48 (ELNRNG)",        // "Link number out of range",
        [49] = "49 (EUNATCH)",       // "Protocol driver not attached",
        [50] = "50 (ENOCSI)",        // "No CSI structure available",
        [51] = "51 (EL2HLT)",        // "Level 2 halted",
        [52] = "52 (EBADE)",         // "Invalid exchange",
        [53] = "53 (EBADR)",         // "Invalid request descriptor",
        [54] = "54 (EXFULL)",        // "Exchange full",
        [55] = "55 (ENOANO)",        // "No anode",
        [56] = "56 (EBADRQC)",       // "Invalid request code",
        [57] = "57 (EBADSLT)",       // "Invalid slot",
        [59] = "59 (EBFONT)",        // "Bad font file format",
        [60] = "60 (ENOSTR)",        // "Device not a stream",
        [61] = "61 (ENODATA)",       // "No data available",
        [62] = "62 (ETIME)",         // "Timer expired",
        [63] = "63 (ENOSR)",         // "Out of streams resources",
        [64] = "64 (ENONET)",        // "Machine is not on the network",
        [65] = "65 (ENOPKG)",        // "Package not installed",
        [66] = "66 (EREMOTE)",       // "Object is remote",
        [67] = "67 (ENOLINK)",       // "Link has been severed",
        [68] = "68 (EADV)",          // "Advertise error",
        [69] = "69 (ESRMNT)",        // "Srmount error",
        [70] = "70 (ECOMM)",         // "Communication error on send",
        [71] = "71 (EPROTO)",        // "Protocol error",
        [72] = "72 (EMULTIHOP)",     // "Multihop attempted",
        [73] = "73 (EDOTDOT)",       // "RFS specific error",
        [74] = "74 (EBADMSG)",       // "Bad message",
        [75] = "75 (EOVERFLOW)",     // "Value too large for defined data type",
        [76] = "76 (ENOTUNIQ)",      // "Name not unique on network",
        [77] = "77 (EBADFD)",        // "File descriptor in bad state",
        [78] = "78 (EREMCHG)",       // "Remote address changed",
        [79] = "79 (ELIBACC)",       // "Can not access a needed shared library",
        [80] = "80 (ELIBBAD)",       // "Accessing a corrupted shared library",
        [81] = "81 (ELIBSCN)",       // ".lib section in a.out corrupted",
        [82] = "82 (ELIBMAX)",       // "Attempting to link in too many shared libraries",
        [83] = "83 (ELIBEXEC)",      // "Cannot exec a shared library directly",
        [84] = "84 (EILSEQ)",        // "Invalid or incomplete multibyte or wide character",
        [85] = "85 (ERESTART)",      // "Interrupted system call should be restarted",
        [86] = "86 (ESTRPIPE)",      // "Streams pipe error",
        [87] = "87 (EUSERS)",        // "Too many users",
        [88] = "88 (ENOTSOCK)",      // "Socket operation on non-socket",
        [89] = "89 (EDESTADDRREQ)",  // "Destination address required",
        [90] = "90 (EMSGSIZE)",      // "Message too long",
        [91] = "91 (EPROTOTYPE)",    // "Protocol wrong type for socket",
        [92] = "92 (ENOPROTOOPT)",   // "Protocol not available",
        [93] = "93 (EPROTONOSUPPORT)",   // "Protocol not supported",
        [94] = "94 (ESOCKTNOSUPPORT)",   // "Socket type not supported",
        [95] = "95 (ENOTSUP)",       // "Operation not supported",
        [96] = "96 (EPFNOSUPPORT)",  // "Protocol family not supported",
        [97] = "97 (EAFNOSUPPORT)",  // "Address family not supported by protocol",
        [98] = "98 (EADDRINUSE)",    // "Address already in use",
        [99] = "99 (EADDRNOTAVAIL)", // "Cannot assign requested address",
        [100] = "100 (ENETDOWN)",     // "Network is down",
        [101] = "101 (ENETUNREACH)",  // "Network is unreachable",
        [102] = "102 (ENETRESET)",    // "Network dropped connection on reset",
        [103] = "103 (ECONNABORTED)", // "Software caused connection abort",
        [104] = "104 (ECONNRESET)",   // "Connection reset by peer",
        [105] = "105 (ENOBUFS)",      // "No buffer space available",
        [106] = "106 (EISCONN)",      // "Transport endpoint is already connected",
        [107] = "107 (ENOTCONN)",     // "Transport endpoint is not connected",
        [108] = "108 (ESHUTDOWN)",    // "Cannot send after transport endpoint shutdown",
        [109] = "109 (ETOOMANYREFS)", // "Too many references: cannot splice",
        [110] = "110 (ETIMEDOUT)",    // "Connection timed out",
        [111] = "111 (ECONNREFUSED)", // "Connection refused",
        [112] = "112 (EHOSTDOWN)",    // "Host is down",
        [113] = "113 (EHOSTUNREACH)", // "No route to host",
        [114] = "114 (EALREADY)",     // "Operation already in progress",
        [115] = "115 (EINPROGRESS)",  // "Operation now in progress",
        [116] = "116 (ESTALE)",       // "Stale file handle",
        [117] = "117 (EUCLEAN)",      // "Structure needs cleaning",
        [118] = "118 (ENOTNAM)",      // "Not a XENIX named type file",
        [119] = "119 (ENAVAIL)",      // "No XENIX semaphores available",
        [120] = "120 (EISNAM)",       // "Is a named type file",
        [121] = "121 (EREMOTEIO)",    // "Remote I/O error",
        [122] = "122 (EDQUOT)",       // "Disk quota exceeded",
        [123] = "123 (ENOMEDIUM)",    // "No medium found",
        [124] = "124 (EMEDIUMTYPE)",  // "Wrong medium type",
        [125] = "125 (ECANCELED)",    // "Operation canceled",
        [126] = "126 (ENOKEY)",       // "Required key not available",
        [127] = "127 (EKEYEXPIRED)",  // "Key has expired",
        [128] = "128 (EKEYREVOKED)",  // "Key has been revoked",
        [129] = "129 (EKEYREJECTED)", // "Key was rejected by service",
        [130] = "130 (EOWNERDEAD)",   // "Owner died",
        [131] = "131 (ENOTRECOVERABLE)",  // "State not recoverable",
        [132] = "132 (ERFKILL)",      // "Operation not possible due to RF-kill",
        [133] = "133 (EHWPOISON)",    // "Memory page has hardware error",
};

const char *linux_capabilities[] = {
        [CAP_CHOWN] = "CHOWN",
        [CAP_DAC_OVERRIDE] = "DAC_OVERRIDE",
        [CAP_DAC_READ_SEARCH] = "DAC_READ_SEARCH",
        [CAP_FOWNER] = "FOWNER",
        [CAP_FSETID] = "FSETID",
        [CAP_KILL] = "KILL",
        [CAP_SETGID] = "SETGID",
        [CAP_SETUID] = "SETUID",
        [CAP_SETPCAP] = "SETPCAP",
        [CAP_LINUX_IMMUTABLE] = "LINUX_IMMUTABLE",
        [CAP_NET_BIND_SERVICE] = "NET_BIND_SERVICE",
        [CAP_NET_BROADCAST] = "NET_BROADCAST",
        [CAP_NET_ADMIN] = "NET_ADMIN",
        [CAP_NET_RAW] = "NET_RAW",
        [CAP_IPC_LOCK] = "IPC_LOCK",
        [CAP_IPC_OWNER] = "IPC_OWNER",
        [CAP_SYS_MODULE] = "SYS_MODULE",
        [CAP_SYS_RAWIO] = "SYS_RAWIO",
        [CAP_SYS_CHROOT] = "SYS_CHROOT",
        [CAP_SYS_PTRACE] = "SYS_PTRACE",
        [CAP_SYS_PACCT] = "SYS_PACCT",
        [CAP_SYS_ADMIN] = "SYS_ADMIN",
        [CAP_SYS_BOOT] = "SYS_BOOT",
        [CAP_SYS_NICE] = "SYS_NICE",
        [CAP_SYS_RESOURCE] = "SYS_RESOURCE",
        [CAP_SYS_TIME] = "SYS_TIME",
        [CAP_SYS_TTY_CONFIG] = "SYS_TTY_CONFIG",
        [CAP_MKNOD] = "MKNOD",
        [CAP_LEASE] = "LEASE",
        [CAP_AUDIT_WRITE] = "AUDIT_WRITE",
        [CAP_AUDIT_CONTROL] = "AUDIT_CONTROL",
        [CAP_SETFCAP] = "SETFCAP",
        [CAP_MAC_OVERRIDE] = "MAC_OVERRIDE",
        [CAP_MAC_ADMIN] = "MAC_ADMIN",
        [CAP_SYSLOG] = "SYSLOG",
        [CAP_WAKE_ALARM] = "WAKE_ALARM",
        [CAP_BLOCK_SUSPEND] = "BLOCK_SUSPEND",
        [37 /*CAP_AUDIT_READ*/] = "AUDIT_READ",
        [38 /*CAP_PERFMON*/] = "PERFMON",
        [39 /*CAP_BPF*/] = "BPF",
        [40 /* CAP_CHECKPOINT_RESTORE */] = "CHECKPOINT_RESTORE",
};

static const char *syslog_facility_to_name(int facility) {
    switch (facility) {
        case LOG_FAC(LOG_KERN): return "kern";
        case LOG_FAC(LOG_USER): return "user";
        case LOG_FAC(LOG_MAIL): return "mail";
        case LOG_FAC(LOG_DAEMON): return "daemon";
        case LOG_FAC(LOG_AUTH): return "auth";
        case LOG_FAC(LOG_SYSLOG): return "syslog";
        case LOG_FAC(LOG_LPR): return "lpr";
        case LOG_FAC(LOG_NEWS): return "news";
        case LOG_FAC(LOG_UUCP): return "uucp";
        case LOG_FAC(LOG_CRON): return "cron";
        case LOG_FAC(LOG_AUTHPRIV): return "authpriv";
        case LOG_FAC(LOG_FTP): return "ftp";
        case LOG_FAC(LOG_LOCAL0): return "local0";
        case LOG_FAC(LOG_LOCAL1): return "local1";
        case LOG_FAC(LOG_LOCAL2): return "local2";
        case LOG_FAC(LOG_LOCAL3): return "local3";
        case LOG_FAC(LOG_LOCAL4): return "local4";
        case LOG_FAC(LOG_LOCAL5): return "local5";
        case LOG_FAC(LOG_LOCAL6): return "local6";
        case LOG_FAC(LOG_LOCAL7): return "local7";
        default: return NULL;
    }
}

static const char *syslog_priority_to_name(int priority) {
    switch (priority) {
        case LOG_ALERT: return "alert";
        case LOG_CRIT: return "critical";
        case LOG_DEBUG: return "debug";
        case LOG_EMERG: return "panic";
        case LOG_ERR: return "error";
        case LOG_INFO: return "info";
        case LOG_NOTICE: return "notice";
        case LOG_WARNING: return "warning";
        default: return NULL;
    }
}

FACET_ROW_SEVERITY syslog_priority_to_facet_severity(FACETS *facets __maybe_unused, FACET_ROW *row, void *data __maybe_unused) {
    // same to
    // https://github.com/systemd/systemd/blob/aab9e4b2b86905a15944a1ac81e471b5b7075932/src/basic/terminal-util.c#L1501
    // function get_log_colors()

    FACET_ROW_KEY_VALUE *priority_rkv = dictionary_get(row->dict, "PRIORITY");
    if(!priority_rkv || priority_rkv->empty)
        return FACET_ROW_SEVERITY_NORMAL;

    int priority = str2i(buffer_tostring(priority_rkv->wb));

    if(priority <= LOG_ERR)
        return FACET_ROW_SEVERITY_CRITICAL;

    else if (priority <= LOG_WARNING)
        return FACET_ROW_SEVERITY_WARNING;

    else if(priority <= LOG_NOTICE)
        return FACET_ROW_SEVERITY_NOTICE;

    else if(priority >= LOG_DEBUG)
        return FACET_ROW_SEVERITY_DEBUG;

    return FACET_ROW_SEVERITY_NORMAL;
}

static char *uid_to_username(uid_t uid, char *buffer, size_t buffer_size) {
    static __thread char tmp[1024 + 1];
    struct passwd pw, *result = NULL;

    if (getpwuid_r(uid, &pw, tmp, sizeof(tmp), &result) != 0 || !result || !pw.pw_name || !(*pw.pw_name))
        snprintfz(buffer, buffer_size - 1, "%u", uid);
    else
        snprintfz(buffer, buffer_size - 1, "%u (%s)", uid, pw.pw_name);

    return buffer;
}

static char *gid_to_groupname(gid_t gid, char* buffer, size_t buffer_size) {
    static __thread char tmp[1024];
    struct group grp, *result = NULL;

    if (getgrgid_r(gid, &grp, tmp, sizeof(tmp), &result) != 0 || !result || !grp.gr_name || !(*grp.gr_name))
        snprintfz(buffer, buffer_size - 1, "%u", gid);
    else
        snprintfz(buffer, buffer_size - 1, "%u (%s)", gid, grp.gr_name);

    return buffer;
}

void netdata_systemd_journal_transform_syslog_facility(FACETS *facets __maybe_unused, BUFFER *wb, FACETS_TRANSFORMATION_SCOPE scope __maybe_unused, void *data __maybe_unused) {
    const char *v = buffer_tostring(wb);
    if(*v && isdigit(*v)) {
        int facility = str2i(buffer_tostring(wb));
        const char *name = syslog_facility_to_name(facility);
        if (name) {
            buffer_flush(wb);
            buffer_strcat(wb, name);
        }
    }
}

void netdata_systemd_journal_transform_priority(FACETS *facets __maybe_unused, BUFFER *wb, FACETS_TRANSFORMATION_SCOPE scope __maybe_unused, void *data __maybe_unused) {
    if(scope == FACETS_TRANSFORM_FACET_SORT)
        return;

    const char *v = buffer_tostring(wb);
    if(*v && isdigit(*v)) {
        int priority = str2i(buffer_tostring(wb));
        const char *name = syslog_priority_to_name(priority);
        if (name) {
            buffer_flush(wb);
            buffer_strcat(wb, name);
        }
    }
}

void netdata_systemd_journal_transform_errno(FACETS *facets __maybe_unused, BUFFER *wb, FACETS_TRANSFORMATION_SCOPE scope __maybe_unused, void *data __maybe_unused) {
    if(scope == FACETS_TRANSFORM_FACET_SORT)
        return;

    const char *v = buffer_tostring(wb);
    if(*v && isdigit(*v)) {
        unsigned err_no = str2u(buffer_tostring(wb));
        if(err_no > 0 && err_no < sizeof(errno_map) / sizeof(*errno_map)) {
            const char *name = errno_map[err_no];
            if(name) {
                buffer_flush(wb);
                buffer_strcat(wb, name);
            }
        }
    }
}

// ----------------------------------------------------------------------------
// UID and GID transformation

#define UID_GID_HASHTABLE_SIZE 10000

struct word_t2str_hashtable_entry {
    struct word_t2str_hashtable_entry *next;
    Word_t hash;
    size_t len;
    char str[];
};

struct word_t2str_hashtable {
    SPINLOCK spinlock;
    size_t size;
    struct word_t2str_hashtable_entry *hashtable[UID_GID_HASHTABLE_SIZE];
};

struct word_t2str_hashtable uid_hashtable = {
        .size = UID_GID_HASHTABLE_SIZE,
};

struct word_t2str_hashtable gid_hashtable = {
        .size = UID_GID_HASHTABLE_SIZE,
};

struct word_t2str_hashtable_entry **word_t2str_hashtable_slot(struct word_t2str_hashtable *ht, Word_t hash) {
    size_t slot = hash % ht->size;
    struct word_t2str_hashtable_entry **e = &ht->hashtable[slot];

    while(*e && (*e)->hash != hash)
        e = &((*e)->next);

    return e;
}

const char *uid_to_username_cached(uid_t uid, size_t *length) {
    spinlock_lock(&uid_hashtable.spinlock);

    struct word_t2str_hashtable_entry **e = word_t2str_hashtable_slot(&uid_hashtable, uid);
    if(!(*e)) {
        static __thread char buf[1024];
        const char *name = uid_to_username(uid, buf, sizeof(buf));
        size_t size = strlen(name) + 1;

        *e = callocz(1, sizeof(struct word_t2str_hashtable_entry) + size);
        (*e)->len = size - 1;
        (*e)->hash = uid;
        memcpy((*e)->str, name, size);
    }

    spinlock_unlock(&uid_hashtable.spinlock);

    *length = (*e)->len;
    return (*e)->str;
}

const char *gid_to_groupname_cached(gid_t gid, size_t *length) {
    spinlock_lock(&gid_hashtable.spinlock);

    struct word_t2str_hashtable_entry **e = word_t2str_hashtable_slot(&gid_hashtable, gid);
    if(!(*e)) {
        static __thread char buf[1024];
        const char *name = gid_to_groupname(gid, buf, sizeof(buf));
        size_t size = strlen(name) + 1;

        *e = callocz(1, sizeof(struct word_t2str_hashtable_entry) + size);
        (*e)->len = size - 1;
        (*e)->hash = gid;
        memcpy((*e)->str, name, size);
    }

    spinlock_unlock(&gid_hashtable.spinlock);

    *length = (*e)->len;
    return (*e)->str;
}

DICTIONARY *boot_ids_to_first_ut = NULL;

void netdata_systemd_journal_transform_boot_id(FACETS *facets __maybe_unused, BUFFER *wb, FACETS_TRANSFORMATION_SCOPE scope __maybe_unused, void *data __maybe_unused) {
    const char *boot_id = buffer_tostring(wb);
    if(*boot_id && isxdigit(*boot_id)) {
        usec_t ut = UINT64_MAX;
        usec_t *p_ut = dictionary_get(boot_ids_to_first_ut, boot_id);
        if(!p_ut) {
            struct journal_file *jf;
            dfe_start_read(journal_files_registry, jf) {
                const char *files[2] = {
                        [0] = jf_dfe.name,
                        [1] = NULL,
                };

                sd_journal *j = NULL;
                if(sd_journal_open_files(&j, files, ND_SD_JOURNAL_OPEN_FLAGS) < 0 || !j) {
                    internal_error(true, "JOURNAL: cannot open file '%s' to get boot_id", jf_dfe.name);
                    continue;
                }

                char m[100];
                size_t len = snprintfz(m, sizeof(m), "_BOOT_ID=%s", boot_id);

                if(sd_journal_add_match(j, m, len) < 0) {
                    internal_error(true, "JOURNAL: cannot add match '%s' to file '%s'", m, jf_dfe.name);
                    sd_journal_close(j);
                    continue;
                }

                if(sd_journal_seek_head(j) < 0) {
                    internal_error(true, "JOURNAL: cannot seek head to file '%s'", jf_dfe.name);
                    sd_journal_close(j);
                    continue;
                }

                if(sd_journal_next(j) < 0) {
                    internal_error(true, "JOURNAL: cannot get next of file '%s'", jf_dfe.name);
                    sd_journal_close(j);
                    continue;
                }

                usec_t t_ut = 0;
                if(sd_journal_get_realtime_usec(j, &t_ut) < 0 || !t_ut) {
                    internal_error(true, "JOURNAL: cannot get realtime_usec of file '%s'", jf_dfe.name);
                    sd_journal_close(j);
                    continue;
                }

                if(t_ut < ut)
                    ut = t_ut;

                sd_journal_close(j);
            }
            dfe_done(jf);

            dictionary_set(boot_ids_to_first_ut, boot_id, &ut, sizeof(ut));
        }
        else
            ut = *p_ut;

        if(ut != UINT64_MAX) {
            time_t timestamp_sec = (time_t)(ut / USEC_PER_SEC);
            struct tm tm;
            char buffer[30];

            gmtime_r(&timestamp_sec, &tm);
            strftime(buffer, sizeof(buffer), "%Y-%m-%d %H:%M:%S", &tm);

            switch(scope) {
                default:
                case FACETS_TRANSFORM_DATA:
                case FACETS_TRANSFORM_VALUE:
                    buffer_sprintf(wb, " (%s UTC)  ", buffer);
                    break;

                case FACETS_TRANSFORM_FACET:
                case FACETS_TRANSFORM_FACET_SORT:
                case FACETS_TRANSFORM_HISTOGRAM:
                    buffer_flush(wb);
                    buffer_sprintf(wb, "%s UTC", buffer);
                    break;
            }
        }
    }
}

void netdata_systemd_journal_transform_uid(FACETS *facets __maybe_unused, BUFFER *wb, FACETS_TRANSFORMATION_SCOPE scope __maybe_unused, void *data __maybe_unused) {
    if(scope == FACETS_TRANSFORM_FACET_SORT)
        return;

    const char *v = buffer_tostring(wb);
    if(*v && isdigit(*v)) {
        uid_t uid = str2i(buffer_tostring(wb));
        size_t len;
        const char *name = uid_to_username_cached(uid, &len);
        buffer_contents_replace(wb, name, len);
    }
}

void netdata_systemd_journal_transform_gid(FACETS *facets __maybe_unused, BUFFER *wb, FACETS_TRANSFORMATION_SCOPE scope __maybe_unused, void *data __maybe_unused) {
    if(scope == FACETS_TRANSFORM_FACET_SORT)
        return;

    const char *v = buffer_tostring(wb);
    if(*v && isdigit(*v)) {
        gid_t gid = str2i(buffer_tostring(wb));
        size_t len;
        const char *name = gid_to_groupname_cached(gid, &len);
        buffer_contents_replace(wb, name, len);
    }
}

void netdata_systemd_journal_transform_cap_effective(FACETS *facets __maybe_unused, BUFFER *wb, FACETS_TRANSFORMATION_SCOPE scope __maybe_unused, void *data __maybe_unused) {
    if(scope == FACETS_TRANSFORM_FACET_SORT)
        return;

    const char *v = buffer_tostring(wb);
    if(*v && isdigit(*v)) {
        uint64_t cap = strtoul(buffer_tostring(wb), NULL, 16);
        if(cap) {
            buffer_fast_strcat(wb, " (", 2);
            for (size_t i = 0, added = 0; i < sizeof(linux_capabilities) / sizeof(linux_capabilities[0]); i++) {
                if (linux_capabilities[i] && (cap & (1ULL << i))) {

                    if (added)
                        buffer_fast_strcat(wb, " | ", 3);

                    buffer_strcat(wb, linux_capabilities[i]);
                    added++;
                }
            }
            buffer_fast_strcat(wb, ")", 1);
        }
    }
}

void netdata_systemd_journal_transform_timestamp_usec(FACETS *facets __maybe_unused, BUFFER *wb, FACETS_TRANSFORMATION_SCOPE scope __maybe_unused, void *data __maybe_unused) {
    if(scope == FACETS_TRANSFORM_FACET_SORT)
        return;

    const char *v = buffer_tostring(wb);
    if(*v && isdigit(*v)) {
        uint64_t ut = str2ull(buffer_tostring(wb), NULL);
        if(ut) {
            time_t timestamp_sec = (time_t)(ut / USEC_PER_SEC);
            struct tm tm;
            char buffer[30];

            gmtime_r(&timestamp_sec, &tm);
            strftime(buffer, sizeof(buffer), "%Y-%m-%d %H:%M:%S", &tm);
            buffer_sprintf(wb, " (%s.%06llu UTC)", buffer, ut % USEC_PER_SEC);
        }
    }
}

// ----------------------------------------------------------------------------

void netdata_systemd_journal_dynamic_row_id(FACETS *facets __maybe_unused, BUFFER *json_array, FACET_ROW_KEY_VALUE *rkv, FACET_ROW *row, void *data __maybe_unused) {
    FACET_ROW_KEY_VALUE *pid_rkv = dictionary_get(row->dict, "_PID");
    const char *pid = pid_rkv ? buffer_tostring(pid_rkv->wb) : FACET_VALUE_UNSET;

    const char *identifier = NULL;
    FACET_ROW_KEY_VALUE *container_name_rkv = dictionary_get(row->dict, "CONTAINER_NAME");
    if(container_name_rkv && !container_name_rkv->empty)
        identifier = buffer_tostring(container_name_rkv->wb);

    if(!identifier) {
        FACET_ROW_KEY_VALUE *syslog_identifier_rkv = dictionary_get(row->dict, "SYSLOG_IDENTIFIER");
        if(syslog_identifier_rkv && !syslog_identifier_rkv->empty)
            identifier = buffer_tostring(syslog_identifier_rkv->wb);

        if(!identifier) {
            FACET_ROW_KEY_VALUE *comm_rkv = dictionary_get(row->dict, "_COMM");
            if(comm_rkv && !comm_rkv->empty)
                identifier = buffer_tostring(comm_rkv->wb);
        }
    }

    buffer_flush(rkv->wb);

    if(!identifier || !*identifier)
        buffer_strcat(rkv->wb, FACET_VALUE_UNSET);
    else if(!pid || !*pid)
        buffer_sprintf(rkv->wb, "%s", identifier);
    else
        buffer_sprintf(rkv->wb, "%s[%s]", identifier, pid);

    buffer_json_add_array_item_string(json_array, buffer_tostring(rkv->wb));
}

static void netdata_systemd_journal_rich_message(FACETS *facets __maybe_unused, BUFFER *json_array, FACET_ROW_KEY_VALUE *rkv, FACET_ROW *row __maybe_unused, void *data __maybe_unused) {
    buffer_json_add_array_item_object(json_array);
    buffer_json_member_add_string(json_array, "value", buffer_tostring(rkv->wb));
    buffer_json_object_close(json_array);
}
