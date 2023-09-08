// SPDX-License-Identifier: GPL-3.0-or-later
/*
 *  netdata systemd-journal.plugin
 *  Copyright (C) 2023 Netdata Inc.
 *  GPL v3+
 */

// TODO - 1) MARKDOC 2) HELP TEXT

#include "collectors/all.h"
#include "libnetdata/libnetdata.h"
#include "libnetdata/required_dummies.h"

#include <systemd/sd-journal.h>
#include <syslog.h>

#define FACET_MAX_VALUE_LENGTH                  8192

#define SYSTEMD_JOURNAL_FUNCTION_DESCRIPTION    "View, search and analyze systemd journal entries."
#define SYSTEMD_JOURNAL_FUNCTION_NAME           "systemd-journal"
#define SYSTEMD_JOURNAL_DEFAULT_TIMEOUT         30
#define SYSTEMD_JOURNAL_MAX_PARAMS              100
#define SYSTEMD_JOURNAL_DEFAULT_QUERY_DURATION  (3 * 3600)
#define SYSTEMD_JOURNAL_DEFAULT_ITEMS_PER_QUERY 200

#define JOURNAL_PARAMETER_HELP                  "help"
#define JOURNAL_PARAMETER_AFTER                 "after"
#define JOURNAL_PARAMETER_BEFORE                "before"
#define JOURNAL_PARAMETER_ANCHOR                "anchor"
#define JOURNAL_PARAMETER_LAST                  "last"
#define JOURNAL_PARAMETER_QUERY                 "query"
#define JOURNAL_PARAMETER_FACETS                "facets"
#define JOURNAL_PARAMETER_HISTOGRAM             "histogram"
#define JOURNAL_PARAMETER_DIRECTION             "direction"
#define JOURNAL_PARAMETER_IF_MODIFIED_SINCE     "if_modified_since"
#define JOURNAL_PARAMETER_SOURCE                "source"
#define JOURNAL_PARAMETER_INFO                  "info"

#define SYSTEMD_ALWAYS_VISIBLE_KEYS             NULL
#define SYSTEMD_KEYS_EXCLUDED_FROM_FACETS       NULL
#define SYSTEMD_KEYS_INCLUDED_IN_FACETS         \
    "_TRANSPORT"                                \
    "|SYSLOG_IDENTIFIER"                        \
    "|SYSLOG_FACILITY"                          \
    "|PRIORITY"                                 \
    "|_UID"                                     \
    "|_GID"                                     \
    "|_SYSTEMD_UNIT"                            \
    "|_SYSTEMD_SLICE"                           \
    "|_COMM"                                    \
    "|UNIT"                                     \
    "|CONTAINER_NAME"                           \
    "|IMAGE_NAME"                               \
    ""

static netdata_mutex_t mutex = NETDATA_MUTEX_INITIALIZER;
static bool plugin_should_exit = false;

DICTIONARY *uids = NULL;
DICTIONARY *gids = NULL;

// ----------------------------------------------------------------------------

int systemd_journal_query(BUFFER *wb, FACETS *facets, usec_t after_ut, usec_t before_ut, usec_t if_modified_since, usec_t stop_monotonic_ut) {
    sd_journal *j = NULL;
    int r;

    if(*netdata_configured_host_prefix) {
#ifdef HAVE_SD_JOURNAL_OS_ROOT
        // Give our host prefix to systemd journal
        r = sd_journal_open_directory(&j, netdata_configured_host_prefix, SD_JOURNAL_OS_ROOT);
#else
        char buf[FILENAME_MAX + 1];
        snprintfz(buf, FILENAME_MAX, "%s/var/log/journal", netdata_configured_host_prefix);
        r = sd_journal_open_directory(&j, buf, 0);
#endif
    }
    else {
        // Open the system journal for reading
        r = sd_journal_open(&j, 0);
    }

    if (r < 0) {
        netdata_log_error("SYSTEMD-JOURNAL: Failed to open SystemD Journal, with error %d", r);
        return HTTP_RESP_INTERNAL_SERVER_ERROR;
    }

    facets_rows_begin(facets);

    uint64_t first_msg_ut = 0;
    bool timed_out = false;
    size_t row_counter = 0;
    sd_journal_seek_realtime_usec(j, before_ut);
    SD_JOURNAL_FOREACH_BACKWARDS(j) {
            row_counter++;

            uint64_t msg_ut;
            sd_journal_get_realtime_usec(j, &msg_ut);

            if(unlikely(!first_msg_ut)) {
                if(msg_ut == if_modified_since) {
                    sd_journal_close(j);
                    return HTTP_RESP_NOT_MODIFIED;
                }

                first_msg_ut = msg_ut;
            }

            if (msg_ut < after_ut)
                break;

            const void *data;
            size_t length;
            SD_JOURNAL_FOREACH_DATA(j, data, length) {
                const char *key = data;
                const char *equal = strchr(key, '=');
                if(unlikely(!equal))
                    continue;

                const char *value = ++equal;
                size_t key_length = value - key; // including '\0'

                char key_copy[key_length];
                memcpy(key_copy, key, key_length - 1);
                key_copy[key_length - 1] = '\0';

                size_t value_length = length - key_length; // without '\0'
                facets_add_key_value_length(facets, key_copy, value, value_length <= FACET_MAX_VALUE_LENGTH ? value_length : FACET_MAX_VALUE_LENGTH);
            }

            facets_row_finished(facets, msg_ut);

            if((row_counter % 100) == 0 && now_monotonic_usec() > stop_monotonic_ut) {
                timed_out = true;
                break;
            }
        }

    sd_journal_close(j);

    buffer_json_member_add_uint64(wb, "status", HTTP_RESP_OK);
    buffer_json_member_add_boolean(wb, "partial", timed_out);
    buffer_json_member_add_string(wb, "type", "table");
    buffer_json_member_add_time_t(wb, "update_every", 1);
    buffer_json_member_add_string(wb, "help", SYSTEMD_JOURNAL_FUNCTION_DESCRIPTION);
    buffer_json_member_add_uint64(wb, "last_modified", first_msg_ut);

    facets_report(facets, wb);

    buffer_json_member_add_time_t(wb, "expires", now_realtime_sec());
    buffer_json_finalize(wb);

    return HTTP_RESP_OK;
}

static void systemd_journal_function_help(const char *transaction) {
    pluginsd_function_result_begin_to_stdout(transaction, HTTP_RESP_OK, "text/plain", now_realtime_sec() + 3600);
    fprintf(stdout,
            "%s / %s\n"
            "\n"
            "%s\n"
            "\n"
            "The following filters are supported:\n"
            "\n"
            "   help\n"
            "      Shows this help message.\n"
            "\n"
            "   before:TIMESTAMP\n"
            "      Absolute or relative (to now) timestamp in seconds, to start the query.\n"
            "      The query is always executed from the most recent to the oldest log entry.\n"
            "      If not given the default is: now.\n"
            "\n"
            "   after:TIMESTAMP\n"
            "      Absolute or relative (to `before`) timestamp in seconds, to end the query.\n"
            "      If not given, the default is %d.\n"
            "\n"
            "   last:ITEMS\n"
            "      The number of items to return.\n"
            "      The default is %d.\n"
            "\n"
            "   anchor:NUMBER\n"
            "      The `timestamp` of the item last received, to return log entries after that.\n"
            "      If not given, the query will return the top `ITEMS` from the most recent.\n"
            "\n"
            "   facet_id:value_id1,value_id2,value_id3,...\n"
            "      Apply filters to the query, based on the facet IDs returned.\n"
            "      Each `facet_id` can be given once, but multiple `facet_ids` can be given.\n"
            "\n"
            "Filters can be combined. Each filter can be given only one time.\n"
            , program_name
            , SYSTEMD_JOURNAL_FUNCTION_NAME
            , SYSTEMD_JOURNAL_FUNCTION_DESCRIPTION
            , -SYSTEMD_JOURNAL_DEFAULT_QUERY_DURATION
            , SYSTEMD_JOURNAL_DEFAULT_ITEMS_PER_QUERY
    );
    pluginsd_function_result_end_to_stdout();
}

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

static char *uid_to_username(uid_t uid, char *buffer, size_t buffer_size) {
    struct passwd pw, *result;
    char tmp[1024 + 1];

    if (getpwuid_r(uid, &pw, tmp, 1024, &result) != 0 || result == NULL)
        return NULL;

    snprintfz(buffer, buffer_size - 1, "%u (%s)", uid, pw.pw_name);
    return buffer;
}

static char *gid_to_groupname(gid_t gid, char* buffer, size_t buffer_size) {
    struct group grp, *result;
    char tmp[1024 + 1];

    if (getgrgid_r(gid, &grp, tmp, 1024, &result) != 0 || result == NULL)
        return NULL;

    snprintfz(buffer, buffer_size - 1, "%u (%s)", gid, grp.gr_name);
    return buffer;
}

static void systemd_journal_transform_syslog_facility(FACETS *facets __maybe_unused, BUFFER *wb, void *data __maybe_unused) {
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

static void systemd_journal_transform_priority(FACETS *facets __maybe_unused, BUFFER *wb, void *data __maybe_unused) {
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

static void systemd_journal_transform_uid(FACETS *facets __maybe_unused, BUFFER *wb, void *data) {
    DICTIONARY *cache = data;
    const char *v = buffer_tostring(wb);
    if(*v && isdigit(*v)) {
        const char *sv = dictionary_get(cache, v);
        if(!sv) {
            char buf[1024 + 1];
            int uid = str2i(buffer_tostring(wb));
            const char *name = uid_to_username(uid, buf, 1024);
            if (!name)
                name = v;

            sv = dictionary_set(cache, v, (void *)name, strlen(name) + 1);
        }

        buffer_flush(wb);
        buffer_strcat(wb, sv);
    }
}

static void systemd_journal_transform_gid(FACETS *facets __maybe_unused, BUFFER *wb, void *data) {
    DICTIONARY *cache = data;
    const char *v = buffer_tostring(wb);
    if(*v && isdigit(*v)) {
        const char *sv = dictionary_get(cache, v);
        if(!sv) {
            char buf[1024 + 1];
            int gid = str2i(buffer_tostring(wb));
            const char *name = gid_to_groupname(gid, buf, 1024);
            if (!name)
                name = v;

            sv = dictionary_set(cache, v, (void *)name, strlen(name) + 1);
        }

        buffer_flush(wb);
        buffer_strcat(wb, sv);
    }
}

static void systemd_journal_dynamic_row_id(FACETS *facets __maybe_unused, BUFFER *json_array, FACET_ROW_KEY_VALUE *rkv, FACET_ROW *row, void *data __maybe_unused) {
    FACET_ROW_KEY_VALUE *pid_rkv = dictionary_get(row->dict, "_PID");
    const char *pid = pid_rkv ? buffer_tostring(pid_rkv->wb) : FACET_VALUE_UNSET;

    FACET_ROW_KEY_VALUE *syslog_identifier_rkv = dictionary_get(row->dict, "SYSLOG_IDENTIFIER");
    const char *identifier = syslog_identifier_rkv ? buffer_tostring(syslog_identifier_rkv->wb) : FACET_VALUE_UNSET;

    if(strcmp(identifier, FACET_VALUE_UNSET) == 0) {
        FACET_ROW_KEY_VALUE *comm_rkv = dictionary_get(row->dict, "_COMM");
        identifier = comm_rkv ? buffer_tostring(comm_rkv->wb) : FACET_VALUE_UNSET;
    }

    buffer_flush(rkv->wb);

    if(strcmp(pid, FACET_VALUE_UNSET) == 0)
        buffer_strcat(rkv->wb, identifier);
    else
        buffer_sprintf(rkv->wb, "%s[%s]", identifier, pid);

    buffer_json_add_array_item_string(json_array, buffer_tostring(rkv->wb));
}

static void function_systemd_journal(const char *transaction, char *function, char *line_buffer __maybe_unused, int line_max __maybe_unused, int timeout __maybe_unused) {
    BUFFER *wb = buffer_create(0, NULL);
    buffer_flush(wb);
    buffer_json_initialize(wb, "\"", "\"", 0, true, BUFFER_JSON_OPTIONS_NEWLINE_ON_ARRAY_ITEMS);

    FACETS *facets = facets_create(50, FACETS_OPTION_ALL_KEYS_FTS,
                                   SYSTEMD_ALWAYS_VISIBLE_KEYS,
                                   SYSTEMD_KEYS_INCLUDED_IN_FACETS,
                                   SYSTEMD_KEYS_EXCLUDED_FROM_FACETS);

    facets_accepted_param(facets, JOURNAL_PARAMETER_INFO);
    facets_accepted_param(facets, JOURNAL_PARAMETER_SOURCE);
    facets_accepted_param(facets, JOURNAL_PARAMETER_AFTER);
    facets_accepted_param(facets, JOURNAL_PARAMETER_BEFORE);
    facets_accepted_param(facets, JOURNAL_PARAMETER_ANCHOR);
    facets_accepted_param(facets, JOURNAL_PARAMETER_DIRECTION);
    facets_accepted_param(facets, JOURNAL_PARAMETER_LAST);
    facets_accepted_param(facets, JOURNAL_PARAMETER_QUERY);
    facets_accepted_param(facets, JOURNAL_PARAMETER_FACETS);
    facets_accepted_param(facets, JOURNAL_PARAMETER_HISTOGRAM);
    facets_accepted_param(facets, JOURNAL_PARAMETER_IF_MODIFIED_SINCE);

    // register the fields in the order you want them on the dashboard

    facets_register_dynamic_key_name(facets, "ND_JOURNAL_PROCESS",
                                     FACET_KEY_OPTION_NO_FACET | FACET_KEY_OPTION_VISIBLE | FACET_KEY_OPTION_FTS,
                                     systemd_journal_dynamic_row_id, NULL);

    facets_register_key_name(facets, "MESSAGE",
                             FACET_KEY_OPTION_NO_FACET | FACET_KEY_OPTION_MAIN_TEXT | FACET_KEY_OPTION_VISIBLE |
                             FACET_KEY_OPTION_FTS);

    facets_register_key_name_transformation(facets, "PRIORITY", FACET_KEY_OPTION_FACET | FACET_KEY_OPTION_FTS,
                                            systemd_journal_transform_priority, NULL);

    facets_register_key_name_transformation(facets, "SYSLOG_FACILITY", FACET_KEY_OPTION_FACET | FACET_KEY_OPTION_FTS,
                                            systemd_journal_transform_syslog_facility, NULL);

    facets_register_key_name(facets, "SYSLOG_IDENTIFIER", FACET_KEY_OPTION_FACET | FACET_KEY_OPTION_FTS);
    facets_register_key_name(facets, "UNIT", FACET_KEY_OPTION_FACET | FACET_KEY_OPTION_FTS);
    facets_register_key_name(facets, "USER_UNIT", FACET_KEY_OPTION_FACET | FACET_KEY_OPTION_FTS);

    facets_register_key_name_transformation(facets, "_UID", FACET_KEY_OPTION_FACET | FACET_KEY_OPTION_FTS,
                                            systemd_journal_transform_uid, uids);

    facets_register_key_name_transformation(facets, "_GID", FACET_KEY_OPTION_FACET | FACET_KEY_OPTION_FTS,
                                            systemd_journal_transform_gid, gids);

    bool info = false;
    time_t after_s = 0, before_s = 0;
    usec_t anchor = 0;
    usec_t if_modified_since = 0;
    size_t last = 0;
    FACETS_ANCHOR_DIRECTION direction = FACETS_ANCHOR_DIRECTION_BACKWARD;
    const char *query = NULL;
    const char *chart = NULL;
    const char *source = NULL;

    buffer_json_member_add_object(wb, "request");

    char *words[SYSTEMD_JOURNAL_MAX_PARAMS] = { NULL };
    size_t num_words = quoted_strings_splitter_pluginsd(function, words, SYSTEMD_JOURNAL_MAX_PARAMS);
    for(int i = 1; i < SYSTEMD_JOURNAL_MAX_PARAMS ;i++) {
        char *keyword = get_word(words, num_words, i);
        if(!keyword) break;

        if(strcmp(keyword, JOURNAL_PARAMETER_HELP) == 0) {
            systemd_journal_function_help(transaction);
            goto cleanup;
        }
        else if(strcmp(keyword, JOURNAL_PARAMETER_INFO) == 0) {
            info = true;
        }
        else if(strncmp(keyword, JOURNAL_PARAMETER_SOURCE ":", sizeof(JOURNAL_PARAMETER_SOURCE ":") - 1) == 0) {
            source = &keyword[sizeof(JOURNAL_PARAMETER_SOURCE ":") - 1];
        }
        else if(strncmp(keyword, JOURNAL_PARAMETER_AFTER ":", sizeof(JOURNAL_PARAMETER_AFTER ":") - 1) == 0) {
            after_s = str2l(&keyword[sizeof(JOURNAL_PARAMETER_AFTER ":") - 1]);
        }
        else if(strncmp(keyword, JOURNAL_PARAMETER_BEFORE ":", sizeof(JOURNAL_PARAMETER_BEFORE ":") - 1) == 0) {
            before_s = str2l(&keyword[sizeof(JOURNAL_PARAMETER_BEFORE ":") - 1]);
        }
        else if(strncmp(keyword, JOURNAL_PARAMETER_IF_MODIFIED_SINCE ":", sizeof(JOURNAL_PARAMETER_IF_MODIFIED_SINCE ":") - 1) == 0) {
            if_modified_since = str2ull(&keyword[sizeof(JOURNAL_PARAMETER_IF_MODIFIED_SINCE ":") - 1], NULL);
        }
        else if(strncmp(keyword, JOURNAL_PARAMETER_ANCHOR ":", sizeof(JOURNAL_PARAMETER_ANCHOR ":") - 1) == 0) {
            anchor = str2ull(&keyword[sizeof(JOURNAL_PARAMETER_ANCHOR ":") - 1], NULL);
        }
        else if(strncmp(keyword, JOURNAL_PARAMETER_DIRECTION ":", sizeof(JOURNAL_PARAMETER_DIRECTION ":") - 1) == 0) {
            direction = strcasecmp(&keyword[sizeof(JOURNAL_PARAMETER_DIRECTION ":") - 1], "forward") ? FACETS_ANCHOR_DIRECTION_FORWARD : FACETS_ANCHOR_DIRECTION_BACKWARD;
        }
        else if(strncmp(keyword, JOURNAL_PARAMETER_LAST ":", sizeof(JOURNAL_PARAMETER_LAST ":") - 1) == 0) {
            last = str2ul(&keyword[sizeof(JOURNAL_PARAMETER_LAST ":") - 1]);
        }
        else if(strncmp(keyword, JOURNAL_PARAMETER_QUERY ":", sizeof(JOURNAL_PARAMETER_QUERY ":") - 1) == 0) {
            query= &keyword[sizeof(JOURNAL_PARAMETER_QUERY ":") - 1];
        }
        else if(strncmp(keyword, JOURNAL_PARAMETER_HISTOGRAM ":", sizeof(JOURNAL_PARAMETER_HISTOGRAM ":") - 1) == 0) {
            chart = &keyword[sizeof(JOURNAL_PARAMETER_HISTOGRAM ":") - 1];
        }
        else if(strncmp(keyword, JOURNAL_PARAMETER_FACETS ":", sizeof(JOURNAL_PARAMETER_FACETS ":") - 1) == 0) {
            char *value = &keyword[sizeof(JOURNAL_PARAMETER_FACETS ":") - 1];
            if(*value) {
                buffer_json_member_add_array(wb, JOURNAL_PARAMETER_FACETS);

                while(value) {
                    char *sep = strchr(value, ',');
                    if(sep)
                        *sep++ = '\0';

                    facets_register_facet_id(facets, value, FACET_KEY_OPTION_FACET|FACET_KEY_OPTION_FTS|FACET_KEY_OPTION_REORDER);
                    buffer_json_add_array_item_string(wb, value);

                    value = sep;
                }

                buffer_json_array_close(wb); // JOURNAL_PARAMETER_FACETS
            }
        }
        else {
            char *value = strchr(keyword, ':');
            if(value) {
                *value++ = '\0';

                buffer_json_member_add_array(wb, keyword);

                while(value) {
                    char *sep = strchr(value, ',');
                    if(sep)
                        *sep++ = '\0';

                    facets_register_facet_id_filter(facets, keyword, value, FACET_KEY_OPTION_FACET|FACET_KEY_OPTION_FTS|FACET_KEY_OPTION_REORDER);
                    buffer_json_add_array_item_string(wb, value);

                    value = sep;
                }

                buffer_json_array_close(wb); // keyword
            }
        }
    }

    time_t expires = now_realtime_sec() + 1;
    time_t now_s;

    if(!after_s && !before_s) {
        now_s = now_realtime_sec();
        before_s = now_s;
        after_s = before_s - SYSTEMD_JOURNAL_DEFAULT_QUERY_DURATION;
    }
    else
        rrdr_relative_window_to_absolute(&after_s, &before_s, &now_s, false);

    if(after_s > before_s) {
        time_t tmp = after_s;
        after_s = before_s;
        before_s = tmp;
    }

    if(after_s == before_s)
        after_s = before_s - SYSTEMD_JOURNAL_DEFAULT_QUERY_DURATION;

    if(!last)
        last = SYSTEMD_JOURNAL_DEFAULT_ITEMS_PER_QUERY;

    buffer_json_member_add_string(wb, "source", source ? source : "default");
    buffer_json_member_add_time_t(wb, "after", after_s);
    buffer_json_member_add_time_t(wb, "before", before_s);
    buffer_json_member_add_uint64(wb, "if_modified_since", if_modified_since);
    buffer_json_member_add_uint64(wb, "anchor", anchor);
    buffer_json_member_add_string(wb, "direction", direction == FACETS_ANCHOR_DIRECTION_FORWARD ? "forward" : "backward");
    buffer_json_member_add_uint64(wb, "last", last);
    buffer_json_member_add_string(wb, "query", query);
    buffer_json_member_add_string(wb, "chart", chart);
    buffer_json_member_add_time_t(wb, "timeout", timeout);
    buffer_json_object_close(wb); // request

    int response;

    if(info) {
        facets_accepted_parameters_to_json_array(facets, wb, false);
        buffer_json_member_add_array(wb, "sources");
        buffer_json_add_array_item_string(wb, "default");
        buffer_json_array_close(wb); // sources
        buffer_json_member_add_uint64(wb, "status", HTTP_RESP_OK);
        buffer_json_member_add_string(wb, "type", "table");
        buffer_json_member_add_string(wb, "help", SYSTEMD_JOURNAL_FUNCTION_DESCRIPTION);
        buffer_json_finalize(wb);
        response = HTTP_RESP_OK;
        goto output;
    }

    facets_set_items(facets, last);
    facets_set_anchor(facets, anchor, direction);
    facets_set_query(facets, query);
    facets_set_histogram(facets, chart ? chart : "PRIORITY", after_s * USEC_PER_SEC, before_s * USEC_PER_SEC);

    response = systemd_journal_query(wb, facets, after_s * USEC_PER_SEC, before_s * USEC_PER_SEC,
                                     if_modified_since, now_monotonic_usec() + (timeout - 1) * USEC_PER_SEC);

    if(response != HTTP_RESP_OK) {
        pluginsd_function_json_error_to_stdout(transaction, response, "failed");
        goto cleanup;
    }

output:
    pluginsd_function_result_begin_to_stdout(transaction, response, "application/json", expires);
    fwrite(buffer_tostring(wb), buffer_strlen(wb), 1, stdout);
    pluginsd_function_result_end_to_stdout();

cleanup:
    facets_destroy(facets);
    buffer_free(wb);
}

static void *reader_main(void *arg __maybe_unused) {
    char buffer[PLUGINSD_LINE_MAX + 1];

    char *s = NULL;
    while(!plugin_should_exit && (s = fgets(buffer, PLUGINSD_LINE_MAX, stdin))) {

        char *words[PLUGINSD_MAX_WORDS] = { NULL };
        size_t num_words = quoted_strings_splitter_pluginsd(buffer, words, PLUGINSD_MAX_WORDS);

        const char *keyword = get_word(words, num_words, 0);

        if(keyword && strcmp(keyword, PLUGINSD_KEYWORD_FUNCTION) == 0) {
            char *transaction = get_word(words, num_words, 1);
            char *timeout_s = get_word(words, num_words, 2);
            char *function = get_word(words, num_words, 3);

            if(!transaction || !*transaction || !timeout_s || !*timeout_s || !function || !*function) {
                netdata_log_error("Received incomplete %s (transaction = '%s', timeout = '%s', function = '%s'). Ignoring it.",
                                  keyword,
                                  transaction?transaction:"(unset)",
                                  timeout_s?timeout_s:"(unset)",
                                  function?function:"(unset)");
            }
            else {
                int timeout = str2i(timeout_s);
                if(timeout <= 0) timeout = SYSTEMD_JOURNAL_DEFAULT_TIMEOUT;

                netdata_mutex_lock(&mutex);

                if(strncmp(function, SYSTEMD_JOURNAL_FUNCTION_NAME, strlen(SYSTEMD_JOURNAL_FUNCTION_NAME)) == 0)
                    function_systemd_journal(transaction, function, buffer, PLUGINSD_LINE_MAX + 1, timeout);
                else
                    pluginsd_function_json_error_to_stdout(transaction, HTTP_RESP_NOT_FOUND,
                                                           "No function with this name found in systemd-journal.plugin.");

                fflush(stdout);
                netdata_mutex_unlock(&mutex);
            }
        }
        else
            netdata_log_error("Received unknown command: %s", keyword?keyword:"(unset)");
    }

    if(!s || feof(stdin) || ferror(stdin)) {
        plugin_should_exit = true;
        netdata_log_error("Received error on stdin.");
    }

    exit(1);
}

int main(int argc __maybe_unused, char **argv __maybe_unused) {
    stderror = stderr;
    clocks_init();

    program_name = "systemd-journal.plugin";

    // disable syslog
    error_log_syslog = 0;

    // set errors flood protection to 100 logs per hour
    error_log_errors_per_period = 100;
    error_log_throttle_period = 3600;

    uids = dictionary_create(0);
    gids = dictionary_create(0);

    netdata_configured_host_prefix = getenv("NETDATA_HOST_PREFIX");
    if(verify_netdata_host_prefix() == -1) exit(1);

    // ------------------------------------------------------------------------
    // debug

    if(argc == 2 && strcmp(argv[1], "debug") == 0) {
        char buf[] = "systemd-journal after:-864000 before:0 last:500";
        function_systemd_journal("123", buf, "", 0, 30);
        exit(1);
    }

    // ------------------------------------------------------------------------

    netdata_thread_t reader_thread;
    netdata_thread_create(&reader_thread, "SDJ_READER", NETDATA_THREAD_OPTION_DONT_LOG, reader_main, NULL);

    // ------------------------------------------------------------------------

    time_t started_t = now_monotonic_sec();

    size_t iteration;
    usec_t step = 1000 * USEC_PER_MS;
    bool tty = isatty(fileno(stderr)) == 1;

    netdata_mutex_lock(&mutex);
    fprintf(stdout, PLUGINSD_KEYWORD_FUNCTION " GLOBAL \"%s\" %d \"%s\"\n",
            SYSTEMD_JOURNAL_FUNCTION_NAME, SYSTEMD_JOURNAL_DEFAULT_TIMEOUT, SYSTEMD_JOURNAL_FUNCTION_DESCRIPTION);

    heartbeat_t hb;
    heartbeat_init(&hb);
    for(iteration = 0; 1 ; iteration++) {
        netdata_mutex_unlock(&mutex);
        heartbeat_next(&hb, step);
        netdata_mutex_lock(&mutex);

        if(!tty)
            fprintf(stdout, "\n");

        fflush(stdout);

        time_t now = now_monotonic_sec();
        if(now - started_t > 86400)
            break;
    }

    dictionary_destroy(uids);
    dictionary_destroy(gids);

    exit(0);
}
