// SPDX-License-Identifier: GPL-3.0-or-later

/*
 * TODO
 * _UDEV_DEVLINK is frequently set more than once per field - support multi-value faces
 *
 */

#include "collectors/systemd-journal.plugin/provider/netdata_provider.h"
#include "systemd-internals.h"
#include "systemd-journal-function.h"

#define ND_SD_JOURNAL_FUNCTION_DESCRIPTION "View, search and analyze systemd journal entries."
#define ND_SD_JOURNAL_FUNCTION_NAME "systemd-journal"

#define JOURNAL_KEY_ND_JOURNAL_PROCESS "ND_JOURNAL_PROCESS"

// functions needed by LQS
static __always_inline SD_JOURNAL_FILE_SOURCE_TYPE get_internal_source_type(const char *value)
{
    if (strcmp(value, ND_SD_JF_SOURCE_ALL_NAME) == 0)
        return ND_SD_JF_ALL;
    else if (strcmp(value, ND_SD_JF_SOURCE_LOCAL_NAME) == 0)
        return ND_SD_JF_LOCAL_ALL;
    else if (strcmp(value, ND_SD_JF_SOURCE_REMOTES_NAME) == 0)
        return ND_SD_JF_REMOTE_ALL;
    else if (strcmp(value, ND_SD_JF_SOURCE_NAMESPACES_NAME) == 0)
        return ND_SD_JF_LOCAL_NAMESPACE;
    else if (strcmp(value, ND_SD_JF_SOURCE_LOCAL_SYSTEM_NAME) == 0)
        return ND_SD_JF_LOCAL_SYSTEM;
    else if (strcmp(value, ND_SD_JF_SOURCE_LOCAL_USERS_NAME) == 0)
        return ND_SD_JF_LOCAL_USER;
    else if (strcmp(value, ND_SD_JF_SOURCE_LOCAL_OTHER_NAME) == 0)
        return ND_SD_JF_LOCAL_OTHER;

    return ND_SD_JF_NONE;
}

// prepare LQS
#define LQS_FUNCTION_NAME ND_SD_JOURNAL_FUNCTION_NAME
#define LQS_FUNCTION_DESCRIPTION ND_SD_JOURNAL_FUNCTION_DESCRIPTION
#define LQS_DEFAULT_ITEMS_PER_QUERY 200
#define LQS_DEFAULT_ITEMS_SAMPLING 1000000
#define LQS_SOURCE_TYPE SD_JOURNAL_FILE_SOURCE_TYPE
#define LQS_SOURCE_TYPE_ALL ND_SD_JF_ALL
#define LQS_SOURCE_TYPE_NONE ND_SD_JF_NONE
#define LQS_PARAMETER_SOURCE_NAME "Journal Sources" // this is how it is shown to users
#define LQS_FUNCTION_GET_INTERNAL_SOURCE_TYPE(value) get_internal_source_type(value)
#define LQS_FUNCTION_SOURCE_TO_JSON_ARRAY(wb) available_journal_file_sources_to_json_array(wb)

#ifdef HAVE_SD_JOURNAL_RESTART_FIELDS
#define LQS_DEFAULT_SLICE_MODE 1
#else
#define LQS_DEFAULT_SLICE_MODE 0
#endif

#include "libnetdata/facets/logs_query_status.h"

#define SYSTEMD_ALWAYS_VISIBLE_KEYS NULL

#define SYSTEMD_KEYS_EXCLUDED_FROM_FACETS                                                                              \
    "!MESSAGE_ID"                                                                                                      \
    "|*MESSAGE*"                                                                                                       \
    "|*TIMESTAMP*"                                                                                                     \
    "|__*"                                                                                                             \
    ""

#define SYSTEMD_KEYS_INCLUDED_IN_FACETS                                                                                        \
                                                                                                                               \
    /* --- USER JOURNAL FIELDS --- */                                                                                          \
                                                                                                                               \
    /* "|MESSAGE" */                                                                                                           \
    "|MESSAGE_ID"                                                                                                              \
    "|PRIORITY"                                                                                                                \
    "|CODE_FILE" /* "|CODE_LINE" */                                                                                            \
    "|CODE_FUNC"                                                                                                               \
    "|ERRNO" /* "|INVOCATION_ID" */ /* "|USER_INVOCATION_ID" */                                                                \
    "|SYSLOG_FACILITY"                                                                                                         \
    "|SYSLOG_IDENTIFIER" /* "|SYSLOG_PID" */ /* "|SYSLOG_TIMESTAMP" */ /* "|SYSLOG_RAW" */ /* "!DOCUMENTATION" */ /* "|TID" */ \
    "|UNIT"                                                                                                                    \
    "|USER_UNIT"                                                                                                               \
    "|UNIT_RESULT" /* undocumented */                                                                                          \
                                                                                                                               \
    /* --- TRUSTED JOURNAL FIELDS --- */                                                                                       \
                                                                                                                               \
    /* "|_PID" */                                                                                                              \
    "|_UID"                                                                                                                    \
    "|_GID"                                                                                                                    \
    "|_COMM"                                                                                                                   \
    "|_EXE" /* "|_CMDLINE" "|_CAP_EFFECTIVE" "|_AUDIT_SESSION" */                                                              \
    "|_AUDIT_LOGINUID"                                                                                                         \
    "|_SYSTEMD_CGROUP"                                                                                                         \
    "|_SYSTEMD_SLICE"                                                                                                          \
    "|_SYSTEMD_UNIT"                                                                                                           \
    "|_SYSTEMD_USER_UNIT"                                                                                                      \
    "|_SYSTEMD_USER_SLICE"                                                                                                     \
    "|_SYSTEMD_SESSION"                                                                                                        \
    "|_SYSTEMD_OWNER_UID"                                                                                                      \
    "|_SELINUX_CONTEXT" /* "|_SOURCE_REALTIME_TIMESTAMP" */                                                                    \
    "|_BOOT_ID"                                                                                                                \
    "|_MACHINE_ID" /* "|_SYSTEMD_INVOCATION_ID" */                                                                             \
    "|_HOSTNAME"                                                                                                               \
    "|_TRANSPORT" /* "|_STREAM_ID" "|LINE_BREAK" */                                                                            \
    "|_NAMESPACE"                                                                                                              \
    "|_RUNTIME_SCOPE"                                                                                                          \
                                                                                                                               \
    /* --- KERNEL JOURNAL FIELDS --- */                                                                                        \
                                                                                                                               \
    /* "|_KERNEL_DEVICE" */                                                                                                    \
    "|_KERNEL_SUBSYSTEM" /* "|_UDEV_SYSNAME" */                                                                                \
    "|_UDEV_DEVNODE"     /* "|_UDEV_DEVLINK" */                                                                                \
                                                                                                                               \
    /* --- LOGGING ON BEHALF --- */                                                                                            \
                                                                                                                               \
    "|OBJECT_UID"                                                                                                              \
    "|OBJECT_GID"                                                                                                              \
    "|OBJECT_COMM"                                                                                                             \
    "|OBJECT_EXE" /* "|OBJECT_CMDLINE" */ /* "|OBJECT_AUDIT_SESSION" */                                                        \
    "|OBJECT_AUDIT_LOGINUID"                                                                                                   \
    "|OBJECT_SYSTEMD_CGROUP"                                                                                                   \
    "|OBJECT_SYSTEMD_SESSION"                                                                                                  \
    "|OBJECT_SYSTEMD_OWNER_UID"                                                                                                \
    "|OBJECT_SYSTEMD_UNIT"                                                                                                     \
    "|OBJECT_SYSTEMD_USER_UNIT"                                                                                                \
                                                                                                                               \
    /* --- CORE DUMPS --- */                                                                                                   \
                                                                                                                               \
    "|COREDUMP_COMM"                                                                                                           \
    "|COREDUMP_UNIT"                                                                                                           \
    "|COREDUMP_USER_UNIT"                                                                                                      \
    "|COREDUMP_SIGNAL_NAME"                                                                                                    \
    "|COREDUMP_CGROUP"                                                                                                         \
                                                                                                                               \
    /* --- DOCKER --- */                                                                                                       \
                                                                                                                               \
    "|CONTAINER_ID" /* "|CONTAINER_ID_FULL" */                                                                                 \
    "|CONTAINER_NAME"                                                                                                          \
    "|CONTAINER_TAG"                                                                                                           \
    "|IMAGE_NAME" /* undocumented */ /* "|CONTAINER_PARTIAL_MESSAGE" */                                                        \
                                                                                                                               \
    /* --- NETDATA --- */                                                                                                      \
                                                                                                                               \
    "|ND_NIDL_NODE"                                                                                                            \
    "|ND_NIDL_CONTEXT"                                                                                                         \
    "|ND_LOG_SOURCE" /*"|ND_MODULE" */                                                                                         \
    "|ND_ALERT_NAME"                                                                                                           \
    "|ND_ALERT_CLASS"                                                                                                          \
    "|ND_ALERT_COMPONENT"                                                                                                      \
    "|ND_ALERT_TYPE"                                                                                                           \
    "|ND_ALERT_STATUS"                                                                                                         \
                                                                                                                               \
    /* --- NETDATA SNMP TRAPS --- */                                                                                           \
                                                                                                                               \
    "|TRAP_REPORT_TYPE"                                                                                                        \
    "|TRAP_OID"                                                                                                                \
    "|TRAP_NAME"                                                                                                               \
    "|TRAP_CATEGORY"                                                                                                           \
    "|TRAP_SEVERITY"                                                                                                           \
    "|TRAP_PDU_TYPE"                                                                                                           \
    "|TRAP_VERSION"                                                                                                            \
    "|TRAP_SOURCE_IP"                                                                                                          \
    "|TRAP_SOURCE_UDP_PEER"                                                                                                    \
    "|TRAP_DEVICE_VENDOR"                                                                                                      \
    "|TRAP_INTERFACE"                                                                                                          \
    "|TRAP_NEIGHBORS"                                                                                                          \
                                                                                                                               \
    ""

#include "systemd-journal-execute.h"

static void systemd_journal_register_transformations(LOGS_QUERY_STATUS *lqs)
{
    FACETS *facets = lqs->facets;
    LOGS_QUERY_REQUEST *rq = &lqs->rq;

    // ----------------------------------------------------------------------------------------------------------------
    // register the fields in the order you want them on the dashboard

    facets_register_row_severity(facets, syslog_priority_to_facet_severity, NULL);

    facets_register_key_name(facets, "_HOSTNAME", rq->default_facet | FACET_KEY_OPTION_VISIBLE);

    facets_register_dynamic_key_name(
        facets,
        JOURNAL_KEY_ND_JOURNAL_PROCESS,
        FACET_KEY_OPTION_NEVER_FACET | FACET_KEY_OPTION_VISIBLE,
        nd_sd_journal_dynamic_row_id,
        NULL);

    facets_register_key_name(
        facets,
        "MESSAGE",
        FACET_KEY_OPTION_NEVER_FACET | FACET_KEY_OPTION_MAIN_TEXT | FACET_KEY_OPTION_VISIBLE | FACET_KEY_OPTION_FTS);

    facets_register_key_name_transformation(
        facets,
        "PRIORITY",
        rq->default_facet | FACET_KEY_OPTION_TRANSFORM_VIEW | FACET_KEY_OPTION_EXPANDED_FILTER,
        nd_sd_journal_transform_priority,
        NULL);

    facets_register_key_name_transformation(
        facets,
        "SYSLOG_FACILITY",
        rq->default_facet | FACET_KEY_OPTION_TRANSFORM_VIEW | FACET_KEY_OPTION_EXPANDED_FILTER,
        nd_sd_journal_transform_syslog_facility,
        NULL);

    facets_register_key_name_transformation(
        facets, "ERRNO", rq->default_facet | FACET_KEY_OPTION_TRANSFORM_VIEW, nd_sd_journal_transform_errno, NULL);

    facets_register_key_name(facets, JOURNAL_KEY_ND_JOURNAL_FILE, FACET_KEY_OPTION_NEVER_FACET);

    facets_register_key_name(facets, "SYSLOG_IDENTIFIER", rq->default_facet);

    facets_register_key_name(facets, "UNIT", rq->default_facet);

    facets_register_key_name(facets, "USER_UNIT", rq->default_facet);

    facets_register_key_name_transformation(
        facets,
        "MESSAGE_ID",
        rq->default_facet | FACET_KEY_OPTION_TRANSFORM_VIEW | FACET_KEY_OPTION_EXPANDED_FILTER,
        nd_sd_journal_transform_message_id,
        NULL);

    facets_register_key_name_transformation(
        facets, "_BOOT_ID", rq->default_facet | FACET_KEY_OPTION_TRANSFORM_VIEW, nd_sd_journal_transform_boot_id, NULL);

    facets_register_key_name_transformation(
        facets,
        "_SYSTEMD_OWNER_UID",
        rq->default_facet | FACET_KEY_OPTION_TRANSFORM_VIEW,
        nd_sd_journal_transform_uid,
        NULL);

    facets_register_key_name_transformation(
        facets, "_UID", rq->default_facet | FACET_KEY_OPTION_TRANSFORM_VIEW, nd_sd_journal_transform_uid, NULL);

    facets_register_key_name_transformation(
        facets,
        "OBJECT_SYSTEMD_OWNER_UID",
        rq->default_facet | FACET_KEY_OPTION_TRANSFORM_VIEW,
        nd_sd_journal_transform_uid,
        NULL);

    facets_register_key_name_transformation(
        facets, "OBJECT_UID", rq->default_facet | FACET_KEY_OPTION_TRANSFORM_VIEW, nd_sd_journal_transform_uid, NULL);

    facets_register_key_name_transformation(
        facets, "_GID", rq->default_facet | FACET_KEY_OPTION_TRANSFORM_VIEW, nd_sd_journal_transform_gid, NULL);

    facets_register_key_name_transformation(
        facets, "OBJECT_GID", rq->default_facet | FACET_KEY_OPTION_TRANSFORM_VIEW, nd_sd_journal_transform_gid, NULL);

    facets_register_key_name_transformation(
        facets, "_CAP_EFFECTIVE", FACET_KEY_OPTION_TRANSFORM_VIEW, nd_sd_journal_transform_cap_effective, NULL);

    facets_register_key_name_transformation(
        facets, "_AUDIT_LOGINUID", FACET_KEY_OPTION_TRANSFORM_VIEW, nd_sd_journal_transform_uid, NULL);

    facets_register_key_name_transformation(
        facets, "OBJECT_AUDIT_LOGINUID", FACET_KEY_OPTION_TRANSFORM_VIEW, nd_sd_journal_transform_uid, NULL);

    facets_register_key_name_transformation(
        facets,
        "_SOURCE_REALTIME_TIMESTAMP",
        FACET_KEY_OPTION_TRANSFORM_VIEW,
        nd_sd_journal_transform_timestamp_usec,
        NULL);
}

BUFFER *function_systemd_journal_result(
    const char *transaction,
    char *function,
    usec_t *stop_monotonic_ut,
    bool *cancelled,
    BUFFER *payload,
    HTTP_ACCESS access __maybe_unused,
    const char *source __maybe_unused,
    void *data __maybe_unused)
{
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

    BUFFER *wb = lqs_create_output_buffer();

    // ------------------------------------------------------------------------
    // parse the parameters

    if (lqs_request_parse_and_validate(lqs, wb, function, payload, have_slice, "PRIORITY")) {
        systemd_journal_register_transformations(lqs);

        // ------------------------------------------------------------------------
        // add versions to the response

        buffer_json_journal_versions(wb);

        // ------------------------------------------------------------------------
        // run the request

        if (lqs->rq.info)
            lqs_info_response(wb, lqs->facets);
        else {
            nd_sd_journal_query(wb, lqs);
            if (wb->response_code == HTTP_RESP_OK)
                buffer_json_finalize(wb);
        }
    }

    lqs_cleanup(lqs);

    return wb;
}

void function_systemd_journal(
    const char *transaction,
    char *function,
    usec_t *stop_monotonic_ut,
    bool *cancelled,
    BUFFER *payload,
    HTTP_ACCESS access,
    const char *source,
    void *data)
{
    BUFFER *wb = function_systemd_journal_result(
        transaction, function, stop_monotonic_ut, cancelled, payload, access, source, data);

    netdata_mutex_lock(&stdout_mutex);
    pluginsd_function_result_to_stdout(transaction, wb);
    netdata_mutex_unlock(&stdout_mutex);

    buffer_free(wb);
}
