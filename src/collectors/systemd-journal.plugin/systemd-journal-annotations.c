// SPDX-License-Identifier: GPL-3.0-or-later

#include "systemd-internals.h"

// ----------------------------------------------------------------------------

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

DICTIONARY *boot_ids_to_first_ut = NULL;

void netdata_systemd_journal_transform_boot_id(FACETS *facets __maybe_unused, BUFFER *wb, FACETS_TRANSFORMATION_SCOPE scope __maybe_unused, void *data __maybe_unused) {
    const char *boot_id = buffer_tostring(wb);
    if(*boot_id && isxdigit(*boot_id)) {
        usec_t ut = UINT64_MAX;
        usec_t *p_ut = dictionary_get(boot_ids_to_first_ut, boot_id);
        if(!p_ut) {
#ifndef HAVE_SD_JOURNAL_RESTART_FIELDS
            struct journal_file *jf;
            dfe_start_read(journal_files_registry, jf) {
                const char *files[2] = {
                        [0] = jf_dfe.name,
                        [1] = NULL,
                };

                sd_journal *j = NULL;
                int r = sd_journal_open_files(&j, files, ND_SD_JOURNAL_OPEN_FLAGS);
                if(r < 0 || !j) {
                    internal_error(true, "JOURNAL: while looking for the first timestamp of boot_id '%s', "
                                         "sd_journal_open_files('%s') returned %d",
                                         boot_id, jf_dfe.name, r);
                    continue;
                }

                ut = journal_file_update_annotation_boot_id(j, jf, boot_id);
                sd_journal_close(j);
            }
            dfe_done(jf);
#endif
        }
        else
            ut = *p_ut;

        if(ut && ut != UINT64_MAX) {
            char buffer[RFC3339_MAX_LENGTH];
            rfc3339_datetime_ut(buffer, sizeof(buffer), ut, 0, true);

            switch(scope) {
                default:
                case FACETS_TRANSFORM_DATA:
                case FACETS_TRANSFORM_VALUE:
                    buffer_sprintf(wb, " (%s)  ", buffer);
                    break;

                case FACETS_TRANSFORM_FACET:
                case FACETS_TRANSFORM_FACET_SORT:
                case FACETS_TRANSFORM_HISTOGRAM:
                    buffer_flush(wb);
                    buffer_sprintf(wb, "%s", buffer);
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
        CACHED_USERNAME cu = cached_username_get_by_uid(uid);
        buffer_contents_replace(wb, string2str(cu.username), string_strlen(cu.username));
        cached_username_release(cu);
    }
}

void netdata_systemd_journal_transform_gid(FACETS *facets __maybe_unused, BUFFER *wb, FACETS_TRANSFORMATION_SCOPE scope __maybe_unused, void *data __maybe_unused) {
    if(scope == FACETS_TRANSFORM_FACET_SORT)
        return;

    const char *v = buffer_tostring(wb);
    if(*v && isdigit(*v)) {
        gid_t gid = str2i(buffer_tostring(wb));
        CACHED_GROUPNAME cg = cached_groupname_get_by_gid(gid);
        buffer_contents_replace(wb, string2str(cg.groupname), string_strlen(cg.groupname));
        cached_groupname_release(cg);
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
            char buffer[RFC3339_MAX_LENGTH];
            rfc3339_datetime_ut(buffer, sizeof(buffer), ut, 6, true);
            buffer_sprintf(wb, " (%s)", buffer);
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


// ----------------------------------------------------------------------------

struct message_id_info {
    const char *msg;
};

static DICTIONARY *known_journal_messages_ids = NULL;

#define msgid_into_dict(uuid, message)                             do {    \
        i.msg = message;                                                    \
        dictionary_set(known_journal_messages_ids, uuid, &i, sizeof(i));    \
    } while(0)

static void netdata_systemd_journal_message_ids_init(void) {
    known_journal_messages_ids = dictionary_create(DICT_OPTION_DONT_OVERWRITE_VALUE);
    struct message_id_info i = { 0 };

    // systemd
    // https://github.com/systemd/systemd/blob/main/catalog/systemd.catalog.in
    msgid_into_dict("f77379a8490b408bbe5f6940505a777b", "Journal started");
    msgid_into_dict("d93fb3c9c24d451a97cea615ce59c00b", "Journal stopped");
    msgid_into_dict("a596d6fe7bfa4994828e72309e95d61e", "Journal messages suppressed");
    msgid_into_dict("e9bf28e6e834481bb6f48f548ad13606", "Journal messages missed");
    msgid_into_dict("ec387f577b844b8fa948f33cad9a75e6", "Journal disk space usage");
    msgid_into_dict("fc2e22bc6ee647b6b90729ab34a250b1", "Coredump");
    msgid_into_dict("5aadd8e954dc4b1a8c954d63fd9e1137", "Coredump truncated");
    msgid_into_dict("1f4e0a44a88649939aaea34fc6da8c95", "Backtrace"); // not found in systemd catalog
    msgid_into_dict("8d45620c1a4348dbb17410da57c60c66", "User Session created");
    msgid_into_dict("3354939424b4456d9802ca8333ed424a", "User Session terminated");
    msgid_into_dict("fcbefc5da23d428093f97c82a9290f7b", "Seat started");
    msgid_into_dict("e7852bfe46784ed0accde04bc864c2d5", "Seat removed");
    msgid_into_dict("24d8d4452573402496068381a6312df2", "VM or container started");
    msgid_into_dict("58432bd3bace477cb514b56381b8a758", "VM or container stopped");
    msgid_into_dict("c7a787079b354eaaa9e77b371893cd27", "Time change");
    msgid_into_dict("45f82f4aef7a4bbf942ce861d1f20990", "Timezone change");
    msgid_into_dict("50876a9db00f4c40bde1a2ad381c3a1b", "System configuration issues");
    msgid_into_dict("b07a249cd024414a82dd00cd181378ff", "System start-up completed");
    msgid_into_dict("eed00a68ffd84e31882105fd973abdd1", "User start-up completed");
    msgid_into_dict("6bbd95ee977941e497c48be27c254128", "Sleep start");
    msgid_into_dict("8811e6df2a8e40f58a94cea26f8ebf14", "Sleep stop");
    msgid_into_dict("98268866d1d54a499c4e98921d93bc40", "System shutdown initiated");
    msgid_into_dict("c14aaf76ec284a5fa1f105f88dfb061c", "System factory reset initiated");
    msgid_into_dict("d9ec5e95e4b646aaaea2fd05214edbda", "Container init crashed");
    msgid_into_dict("3ed0163e868a4417ab8b9e210407a96c", "System reboot failed after crash");
    msgid_into_dict("645c735537634ae0a32b15a7c6cba7d4", "Init execution froze");
    msgid_into_dict("5addb3a06a734d3396b794bf98fb2d01", "Init crashed no coredump");
    msgid_into_dict("5c9e98de4ab94c6a9d04d0ad793bd903", "Init crashed no fork");
    msgid_into_dict("5e6f1f5e4db64a0eaee3368249d20b94", "Init crashed unknown signal");
    msgid_into_dict("83f84b35ee264f74a3896a9717af34cb", "Init crashed systemd signal");
    msgid_into_dict("3a73a98baf5b4b199929e3226c0be783", "Init crashed process signal");
    msgid_into_dict("2ed18d4f78ca47f0a9bc25271c26adb4", "Init crashed waitpid failed");
    msgid_into_dict("56b1cd96f24246c5b607666fda952356", "Init crashed coredump failed");
    msgid_into_dict("4ac7566d4d7548f4981f629a28f0f829", "Init crashed coredump");
    msgid_into_dict("38e8b1e039ad469291b18b44c553a5b7", "Crash shell failed to fork");
    msgid_into_dict("872729b47dbe473eb768ccecd477beda", "Crash shell failed to execute");
    msgid_into_dict("658a67adc1c940b3b3316e7e8628834a", "Selinux failed");
    msgid_into_dict("e6f456bd92004d9580160b2207555186", "Battery low warning");
    msgid_into_dict("267437d33fdd41099ad76221cc24a335", "Battery low powering off");
    msgid_into_dict("79e05b67bc4545d1922fe47107ee60c5", "Manager mainloop failed");
    msgid_into_dict("dbb136b10ef4457ba47a795d62f108c9", "Manager no xdgdir path");
    msgid_into_dict("ed158c2df8884fa584eead2d902c1032", "Init failed to drop capability bounding set of usermode");
    msgid_into_dict("42695b500df048298bee37159caa9f2e", "Init failed to drop capability bounding set");
    msgid_into_dict("bfc2430724ab44499735b4f94cca9295", "User manager can't disable new privileges");
    msgid_into_dict("59288af523be43a28d494e41e26e4510", "Manager failed to start default target");
    msgid_into_dict("689b4fcc97b4486ea5da92db69c9e314", "Manager failed to isolate default target");
    msgid_into_dict("5ed836f1766f4a8a9fc5da45aae23b29", "Manager failed to collect passed file descriptors");
    msgid_into_dict("6a40fbfbd2ba4b8db02fb40c9cd090d7", "Init failed to fix up environment variables");
    msgid_into_dict("0e54470984ac419689743d957a119e2e", "Manager failed to allocate");
    msgid_into_dict("d67fa9f847aa4b048a2ae33535331adb", "Manager failed to write Smack");
    msgid_into_dict("af55a6f75b544431b72649f36ff6d62c", "System shutdown critical error");
    msgid_into_dict("d18e0339efb24a068d9c1060221048c2", "Init failed to fork off valgrind");
    msgid_into_dict("7d4958e842da4a758f6c1cdc7b36dcc5", "Unit starting");
    msgid_into_dict("39f53479d3a045ac8e11786248231fbf", "Unit started");
    msgid_into_dict("be02cf6855d2428ba40df7e9d022f03d", "Unit failed");
    msgid_into_dict("de5b426a63be47a7b6ac3eaac82e2f6f", "Unit stopping");
    msgid_into_dict("9d1aaa27d60140bd96365438aad20286", "Unit stopped");
    msgid_into_dict("d34d037fff1847e6ae669a370e694725", "Unit reloading");
    msgid_into_dict("7b05ebc668384222baa8881179cfda54", "Unit reloaded");
    msgid_into_dict("5eb03494b6584870a536b337290809b3", "Unit restart scheduled");
    msgid_into_dict("ae8f7b866b0347b9af31fe1c80b127c0", "Unit resources");
    msgid_into_dict("7ad2d189f7e94e70a38c781354912448", "Unit success");
    msgid_into_dict("0e4284a0caca4bfc81c0bb6786972673", "Unit skipped");
    msgid_into_dict("d9b373ed55a64feb8242e02dbe79a49c", "Unit failure result");
    msgid_into_dict("641257651c1b4ec9a8624d7a40a9e1e7", "Process execution failed");
    msgid_into_dict("98e322203f7a4ed290d09fe03c09fe15", "Unit process exited");
    msgid_into_dict("0027229ca0644181a76c4e92458afa2e", "Syslog forward missed");
    msgid_into_dict("1dee0369c7fc4736b7099b38ecb46ee7", "Mount point is not empty");
    msgid_into_dict("d989611b15e44c9dbf31e3c81256e4ed", "Unit oomd kill"); // not found in systemd catalog
    msgid_into_dict("fe6faa94e7774663a0da52717891d8ef", "Unit out of memory");
    msgid_into_dict("b72ea4a2881545a0b50e200e55b9b06f", "Lid opened");
    msgid_into_dict("b72ea4a2881545a0b50e200e55b9b070", "Lid closed");
    msgid_into_dict("f5f416b862074b28927a48c3ba7d51ff", "System docked");
    msgid_into_dict("51e171bd585248568110144c517cca53", "System undocked");
    msgid_into_dict("b72ea4a2881545a0b50e200e55b9b071", "Power key");
    msgid_into_dict("3e0117101eb243c1b9a50db3494ab10b", "Power key long press");
    msgid_into_dict("9fa9d2c012134ec385451ffe316f97d0", "Reboot key");
    msgid_into_dict("f1c59a58c9d943668965c337caec5975", "Reboot key long press");
    msgid_into_dict("b72ea4a2881545a0b50e200e55b9b072", "Suspend key");
    msgid_into_dict("bfdaf6d312ab4007bc1fe40a15df78e8", "Suspend key long press");
    msgid_into_dict("b72ea4a2881545a0b50e200e55b9b073", "Hibernate key");
    msgid_into_dict("167836df6f7f428e98147227b2dc8945", "Hibernate key long press");
    msgid_into_dict("c772d24e9a884cbeb9ea12625c306c01", "Invalid configuration"); // not found in systemd catalog
    msgid_into_dict("1675d7f172174098b1108bf8c7dc8f5d", "DNSSEC validation failed");
    msgid_into_dict("4d4408cfd0d144859184d1e65d7c8a65", "DNSSEC trust anchor revoked");
    msgid_into_dict("36db2dfa5a9045e1bd4af5f93e1cf057", "DNSSEC turned off");
    msgid_into_dict("b61fdac612e94b9182285b998843061f", "Username unsafe");
    msgid_into_dict("1b3bb94037f04bbf81028e135a12d293", "Mount point path not suitable");
    msgid_into_dict("010190138f494e29a0ef6669749531aa", "Device path not suitable"); // not found in systemd catalog
    msgid_into_dict("b480325f9c394a7b802c231e51a2752c", "Nobody user unsuitable");
    msgid_into_dict("1c0454c1bd2241e0ac6fefb4bc631433", "Systemd udev settle deprecated");
    msgid_into_dict("7c8a41f37b764941a0e1780b1be2f037", "Time initial sync");
    msgid_into_dict("7db73c8af0d94eeb822ae04323fe6ab6", "Time initial bump");
    msgid_into_dict("9e7066279dc8403da79ce4b1a69064b2", "Shutdown scheduled");
    msgid_into_dict("249f6fb9e6e2428c96f3f0875681ffa3", "Shutdown canceled");
    msgid_into_dict("3f7d5ef3e54f4302b4f0b143bb270cab", "TPM PCR Extended");
    msgid_into_dict("f9b0be465ad540d0850ad32172d57c21", "Memory Trimmed");
    msgid_into_dict("a8fa8dacdb1d443e9503b8be367a6adb", "SysV Service Found");
    msgid_into_dict("187c62eb1e7f463bb530394f52cb090f", "Portable Service attached");
    msgid_into_dict("76c5c754d628490d8ecba4c9d042112b", "Portable Service detached");
    msgid_into_dict("9cf56b8baf9546cf9478783a8de42113", "systemd-networkd sysctl changed by foreign process");
    msgid_into_dict("ad7089f928ac4f7ea00c07457d47ba8a", "SRK into TPM authorization failure");
    msgid_into_dict("b2bcbaf5edf948e093ce50bbea0e81ec", "Secure Attention Key (SAK) was pressed");

    // dbus
    // https://github.com/bus1/dbus-broker/blob/main/src/catalog/catalog-ids.h
    msgid_into_dict("7fc63312330b479bb32e598d47cef1a8", "dbus activate no unit");
    msgid_into_dict("ee9799dab1e24d81b7bee7759a543e1b", "dbus activate masked unit");
    msgid_into_dict("a0fa58cafd6f4f0c8d003d16ccf9e797", "dbus broker exited");
    msgid_into_dict("c8c6cde1c488439aba371a664353d9d8", "dbus dirwatch");
    msgid_into_dict("8af3357071af4153af414daae07d38e7", "dbus dispatch stats");
    msgid_into_dict("199d4300277f495f84ba4028c984214c", "dbus no sopeergroup");
    msgid_into_dict("b209c0d9d1764ab38d13b8e00d1784d6", "dbus protocol violation");
    msgid_into_dict("6fa70fa776044fa28be7a21daf42a108", "dbus receive failed");
    msgid_into_dict("0ce0fa61d1a9433dabd67417f6b8e535", "dbus service failed open");
    msgid_into_dict("24dc708d9e6a4226a3efe2033bb744de", "dbus service invalid");
    msgid_into_dict("f15d2347662d483ea9bcd8aa1a691d28", "dbus sighup");

    // gnome
    // https://gitlab.gnome.org/GNOME/gnome-session/-/blob/main/gnome-session/gsm-manager.c
    msgid_into_dict("0ce153587afa4095832d233c17a88001", "Gnome SM startup succeeded");
    msgid_into_dict("10dd2dc188b54a5e98970f56499d1f73", "Gnome SM unrecoverable failure");

    // gnome-shell
    // https://gitlab.gnome.org/GNOME/gnome-shell/-/blob/main/js/ui/main.js#L56
    msgid_into_dict("f3ea493c22934e26811cd62abe8e203a", "Gnome shell started");

    // flathub
    // https://docs.flatpak.org/de/latest/flatpak-command-reference.html
    msgid_into_dict("c7b39b1e006b464599465e105b361485", "Flatpak cache");

    // ???
    msgid_into_dict("75ba3deb0af041a9a46272ff85d9e73e", "Flathub pulls");
    msgid_into_dict("f02bce89a54e4efab3a94a797d26204a", "Flathub pull errors");

    // ??
    msgid_into_dict("dd11929c788e48bdbb6276fb5f26b08a", "Boltd starting");

    // Netdata
    msgid_into_dict("1e6061a9fbd44501b3ccc368119f2b69", "Netdata startup");
    msgid_into_dict("ed4cdb8f1beb4ad3b57cb3cae2d162fa", "Netdata connection from child");
    msgid_into_dict("6e2e3839067648968b646045dbf28d66", "Netdata connection to parent");
    msgid_into_dict("9ce0cb58ab8b44df82c4bf1ad9ee22de", "Netdata alert transition");
    msgid_into_dict("6db0018e83e34320ae2a659d78019fb7", "Netdata alert notification");
    msgid_into_dict("23e93dfccbf64e11aac858b9410d8a82", "Netdata fatal message");
    msgid_into_dict("8ddaf5ba33a74078b609250db1e951f3", "Sensor state transition");
    msgid_into_dict("ec87a56120d5431bace51e2fb8bba243", "Netdata log flood protection");
    msgid_into_dict("acb33cb95778476baac702eb7e4e151d", "Netdata Cloud connection");
    msgid_into_dict("d1f59606dd4d41e3b217a0cfcae8e632", "Netdata extreme cardinality");
    msgid_into_dict("02f47d350af5449197bf7a95b605a468", "Netdata exit reason");
    msgid_into_dict("4fdf40816c124623a032b7fe73beacb8", "Netdata dynamic configuration");
}

void netdata_systemd_journal_transform_message_id(FACETS *facets __maybe_unused, BUFFER *wb, FACETS_TRANSFORMATION_SCOPE scope __maybe_unused, void *data __maybe_unused) {
    const char *message_id = buffer_tostring(wb);
    struct message_id_info *i = dictionary_get(known_journal_messages_ids, message_id);

    if(!i)
        return;

    switch(scope) {
        default:
        case FACETS_TRANSFORM_DATA:
        case FACETS_TRANSFORM_VALUE:
            buffer_sprintf(wb, " (%s)", i->msg);
            break;

        case FACETS_TRANSFORM_FACET:
        case FACETS_TRANSFORM_FACET_SORT:
        case FACETS_TRANSFORM_HISTOGRAM:
            buffer_flush(wb);
            buffer_strcat(wb, i->msg);
            break;
    }
}

// ----------------------------------------------------------------------------

void netdata_systemd_journal_annotations_init(void) {
    cached_usernames_init();
    cached_groupnames_init();
    update_cached_host_users();
    update_cached_host_groups();
    netdata_systemd_journal_message_ids_init();
}

// ----------------------------------------------------------------------------

//static void netdata_systemd_journal_rich_message(FACETS *facets __maybe_unused, BUFFER *json_array, FACET_ROW_KEY_VALUE *rkv, FACET_ROW *row __maybe_unused, void *data __maybe_unused) {
//    buffer_json_add_array_item_object(json_array);
//    buffer_json_member_add_string(json_array, "value", buffer_tostring(rkv->wb));
//    buffer_json_object_close(json_array);
//}
