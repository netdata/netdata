// SPDX-License-Identifier: GPL-3.0-or-later

#include "systemd-internals.h"

#ifdef ENABLE_SYSTEMD_DBUS
#include <systemd/sd-bus.h>

#define SYSTEMD_UNITS_MAX_PARAMS 10
#define SYSTEMD_UNITS_DBUS_TYPES "(ssssssouso)"

typedef struct UnitInfo {
    char *id;
    char *type;
    char *description;
    char *load_state;
    char *active_state;
    char *sub_state;
    char *following;
    char *unit_path;
    uint32_t job_id;
    char *job_type;
    char *job_path;

    uint32_t prio;

    struct UnitInfo *prev, *next;
} UnitInfo;

int bus_parse_unit_info(sd_bus_message *message, UnitInfo *u) {
    assert(message);
    assert(u);

    u->type = NULL;

    return sd_bus_message_read(
            message,
            SYSTEMD_UNITS_DBUS_TYPES,
            &u->id,
            &u->description,
            &u->load_state,
            &u->active_state,
            &u->sub_state,
            &u->following,
            &u->unit_path,
            &u->job_id,
            &u->job_type,
            &u->job_path);
}

static void log_dbus_error(int r, const char *msg) {
    netdata_log_error("SYSTEMD_UNITS: %s failed with error %d", msg, r);
}

static int hex_to_int(char c) {
    if (c >= '0' && c <= '9') return c - '0';
    if (c >= 'a' && c <= 'f') return c - 'a' + 10;
    if (c >= 'A' && c <= 'F') return c - 'A' + 10;
    return 0;
}

// un-escape hex sequences (\xNN) in id
static void txt_decode(char *txt) {
    if(!txt || !*txt)
        return;

    char *src = txt, *dst = txt;

    size_t id_len = strlen(src);
    size_t s = 0, d = 0;
    for(; s < id_len ; s++) {
        if(src[s] == '\\' && src[s + 1] == 'x' && isxdigit(src[s + 2]) && isxdigit(src[s + 3])) {
            int value = (hex_to_int(src[s + 2]) << 4) + hex_to_int(src[s + 3]);
            dst[d++] = (char)value;
            s += 3;
        }
        else
            dst[d++] = src[s];
    }
    dst[d] = '\0';
}

static UnitInfo *systemd_units_get_all(void) {
    sd_bus *bus = NULL;
    sd_bus_error error = SD_BUS_ERROR_NULL;
    sd_bus_message *reply = NULL;
    UnitInfo *base = NULL;
    int r;

    r = sd_bus_default_system(&bus);
    if (r < 0) {
        log_dbus_error(r, "sd_bus_default_system()");
        return base;
    }

    // This calls the ListUnits method of the org.freedesktop.systemd1.Manager interface
    // Replace "ListUnits" with "ListUnitsFiltered" to get specific units based on filters
    r = sd_bus_call_method(bus,
            "org.freedesktop.systemd1",           /* service to contact */
            "/org/freedesktop/systemd1",          /* object path */
            "org.freedesktop.systemd1.Manager",   /* interface name */
            "ListUnits",                          /* method name */
            &error,                               /* object to return error in */
            &reply,                               /* return message on success */
            NULL);                                /* input signature */
    if (r < 0) {
        log_dbus_error(r, "sd_bus_call_method()");
        return base;
    }

    r = sd_bus_message_enter_container(reply, SD_BUS_TYPE_ARRAY, SYSTEMD_UNITS_DBUS_TYPES);
    if (r < 0) {
        log_dbus_error(r, "sd_bus_message_enter_container()");
        return base;
    }

    UnitInfo u;
    memset(&u, 0, sizeof(u));
    while ((r = bus_parse_unit_info(reply, &u)) > 0) {
        UnitInfo *i = callocz(1, sizeof(u));

        i->id = strdupz(u.id && *u.id ? u.id : "-");
        txt_decode(i->id);

        char *dot = strrchr(i->id, '.');
        if(dot)
            i->type = strdupz(&dot[1]);
        else
            i->type = strdupz("unknown");

        i->description = strdupz(u.description && *u.description ? u.description : "-");
        txt_decode(i->description);

        i->load_state = strdupz(u.load_state && *u.load_state ? u.load_state : "-");
        i->active_state = strdupz(u.active_state && *u.active_state ? u.active_state : "-");
        i->sub_state = strdupz(u.sub_state && *u.sub_state ? u.sub_state : "-");
        i->following = strdupz(u.following && *u.following ? u.following : "-");
        i->unit_path = strdupz(u.unit_path && *u.unit_path ? u.unit_path : "-");
        i->job_type = strdupz(u.job_type && *u.job_type ? u.job_type : "-");
        i->job_path = strdupz(u.job_path && *u.job_path ? u.job_path : "-");
        i->job_id = u.job_id;

        DOUBLE_LINKED_LIST_APPEND_ITEM_UNSAFE(base, i, prev, next);
        memset(&u, 0, sizeof(u));
    }
    if (r < 0) {
        log_dbus_error(r, "sd_bus_message_read()");
        return base;
    }

    r = sd_bus_message_exit_container(reply);
    if (r < 0) {
        log_dbus_error(r, "sd_bus_message_exit_container()");
        return base;
    }

    return base;
}

void systemd_units_free_all(UnitInfo *base) {
    while(base) {
        UnitInfo *u = base;
        DOUBLE_LINKED_LIST_REMOVE_ITEM_UNSAFE(base, u, prev, next);
        freez((void *)u->id);
        freez((void *)u->type);
        freez((void *)u->description);
        freez((void *)u->load_state);
        freez((void *)u->active_state);
        freez((void *)u->sub_state);
        freez((void *)u->following);
        freez((void *)u->unit_path);
        freez((void *)u->job_type);
        freez((void *)u->job_path);
        freez(u);
    }
}

static void netdata_systemd_units_function_help(const char *transaction) {
    BUFFER *wb = buffer_create(0, NULL);
    buffer_sprintf(wb,
            "%s / %s\n"
            "\n"
            "%s\n"
            "\n"
            "The following parameters are supported:\n"
            "\n"
            "   help\n"
            "      Shows this help message.\n"
            "\n"
            "   info\n"
            "      Request initial configuration information about the plugin.\n"
            "      The key entity returned is the required_params array, which includes\n"
            "      all the available systemd journal sources.\n"
            "      When `info` is requested, all other parameters are ignored.\n"
            "\n"
            , program_name
            , SYSTEMD_UNITS_FUNCTION_NAME
            , SYSTEMD_UNITS_FUNCTION_DESCRIPTION
    );

    netdata_mutex_lock(&stdout_mutex);
    pluginsd_function_result_to_stdout(transaction, HTTP_RESP_OK, "text/plain", now_realtime_sec() + 3600, wb);
    netdata_mutex_unlock(&stdout_mutex);

    buffer_free(wb);
}

static void netdata_systemd_units_function_info(const char *transaction) {
    BUFFER *wb = buffer_create(0, NULL);
    buffer_json_initialize(wb, "\"", "\"", 0, true, BUFFER_JSON_OPTIONS_MINIFY);

    buffer_json_member_add_uint64(wb, "status", HTTP_RESP_OK);
    buffer_json_member_add_string(wb, "type", "table");
    buffer_json_member_add_string(wb, "help", SYSTEMD_UNITS_FUNCTION_DESCRIPTION);

    buffer_json_finalize(wb);
    netdata_mutex_lock(&stdout_mutex);
    pluginsd_function_result_to_stdout(transaction, HTTP_RESP_OK, "text/plain", now_realtime_sec() + 3600, wb);
    netdata_mutex_unlock(&stdout_mutex);

    buffer_free(wb);
}

struct sub_state {
    const char *s;
    uint32_t prio;
} sub_states[] = {
        { "failed", 1 },
        { "dead", 2 },
        { "running", 3 },
        { "active", 4 },
        { "auto-restart", 5 },
        { "listening", 6 },
        { "plugged", 7 },
        { "mounted", 8 },
        { "waiting", 9 },
        { "exited", 10 },
};

struct active_state {
    const char *s;
    uint32_t prio;
} active_states[] = {
        { "failed", 1 },
        { "active", 2 },
        { "activating", 3 },
        { "inactive", 4 },
};

static void systemd_unit_priority(UnitInfo *u, size_t units) {
    uint32_t sub_states_count = sizeof(sub_states) / sizeof(struct sub_state);
    uint32_t active_states_count = sizeof(active_states) / sizeof(struct active_state);

    uint32_t sub_state = sub_states_count + 1;
    for(size_t i = 0; i < sub_states_count ; i++) {
        if(strcmp(u->sub_state, sub_states[i].s) == 0) {
            sub_state = sub_states[i].prio;
            break;
        }
    }

    uint32_t active_state = active_states_count + 1;
    for(size_t i = 0; i < active_states_count ; i++) {
        if(strcmp(u->active_state, active_states[i].s) == 0) {
            active_state = active_states[i].prio;
            break;
        }
    }

    uint32_t prio = active_state * sub_states_count + sub_state;
    u->prio = (prio * units) + u->prio;
}

FACET_ROW_SEVERITY system_unit_severity(UnitInfo *u) {

    // load state
    if(strcmp(u->load_state, "not-found") == 0)
        return FACET_ROW_SEVERITY_WARNING;

    if(strcmp(u->load_state, "masked") == 0)
        return FACET_ROW_SEVERITY_DEBUG;

    // active state
    if(strcmp(u->active_state, "failed") == 0)
        return FACET_ROW_SEVERITY_CRITICAL;

    // sub state
    if(strcmp(u->sub_state, "failed") == 0)
        return FACET_ROW_SEVERITY_WARNING;

    if(strcmp(u->sub_state, "dead") == 0)
        return FACET_ROW_SEVERITY_NOTICE;

    if(strcmp(u->sub_state, "waiting") == 0 || strcmp(u->sub_state, "exited") == 0)
        return FACET_ROW_SEVERITY_DEBUG;

    return FACET_ROW_SEVERITY_NORMAL;
}

int unit_info_compar(const void *a, const void *b) {
    UnitInfo *u1 = *((UnitInfo **)a);
    UnitInfo *u2 = *((UnitInfo **)b);

    return strcasecmp(u1->id, u2->id);
}

void systemd_units_assign_priority(UnitInfo *base) {
    size_t units = 0, c = 0, prio = 0;
    for(UnitInfo *u = base; u ; u = u->next)
        units++;

    UnitInfo *array[units];
    for(UnitInfo *u = base; u ; u = u->next)
        array[c++] = u;

    qsort(array, units, sizeof(UnitInfo *), unit_info_compar);

    for(c = 0; c < units ; c++) {
        array[c]->prio = prio++;
        systemd_unit_priority(array[c], units);
    }
}

void function_systemd_units(const char *transaction, char *function, int timeout, bool *cancelled) {
    char *words[SYSTEMD_UNITS_MAX_PARAMS] = { NULL };
    size_t num_words = quoted_strings_splitter_pluginsd(function, words, SYSTEMD_UNITS_MAX_PARAMS);
    for(int i = 1; i < SYSTEMD_UNITS_MAX_PARAMS ;i++) {
        char *keyword = get_word(words, num_words, i);
        if(!keyword) break;

        if(strcmp(keyword, "info") == 0) {
            netdata_systemd_units_function_info(transaction);
            return;
        }
        else if(strcmp(keyword, "help") == 0) {
            netdata_systemd_units_function_help(transaction);
            return;
        }
    }

    UnitInfo *base = systemd_units_get_all();
    systemd_units_assign_priority(base);

    BUFFER *wb = buffer_create(0, NULL);
    buffer_json_initialize(wb, "\"", "\"", 0, true, BUFFER_JSON_OPTIONS_MINIFY);

    buffer_json_member_add_uint64(wb, "status", HTTP_RESP_OK);
    buffer_json_member_add_string(wb, "type", "table");
    buffer_json_member_add_time_t(wb, "update_every", 10);
    buffer_json_member_add_string(wb, "help", SYSTEMD_UNITS_FUNCTION_DESCRIPTION);
    buffer_json_member_add_array(wb, "data");

    for(UnitInfo *u = base; u ;u = u->next) {
        buffer_json_add_array_item_array(wb);
        {
            buffer_json_add_array_item_string(wb, u->id);

            buffer_json_add_array_item_object(wb);
            {
                buffer_json_member_add_string(wb, "severity", facets_severity_to_string(system_unit_severity(u)));
            }
            buffer_json_object_close(wb);

            buffer_json_add_array_item_string(wb, u->type);
            buffer_json_add_array_item_string(wb, u->description);
            buffer_json_add_array_item_string(wb, u->load_state);
            buffer_json_add_array_item_string(wb, u->active_state);
            buffer_json_add_array_item_string(wb, u->sub_state);
            buffer_json_add_array_item_string(wb, u->following);
            buffer_json_add_array_item_string(wb, u->unit_path);
            buffer_json_add_array_item_uint64(wb, u->job_id);
            buffer_json_add_array_item_string(wb, u->job_type);
            buffer_json_add_array_item_string(wb, u->job_path);
            buffer_json_add_array_item_uint64(wb, u->prio);
            buffer_json_add_array_item_uint64(wb, 1); // count
        }
        buffer_json_array_close(wb);
    }

    buffer_json_array_close(wb); // data

    buffer_json_member_add_object(wb, "columns");
    {
        size_t field_id = 0;

        buffer_rrdf_table_add_field(wb, field_id++, "id", "Unit ID",
                RRDF_FIELD_TYPE_STRING, RRDF_FIELD_VISUAL_VALUE, RRDF_FIELD_TRANSFORM_NONE,
                0, NULL, NAN, RRDF_FIELD_SORT_ASCENDING, NULL,
                RRDF_FIELD_SUMMARY_COUNT, RRDF_FIELD_FILTER_NONE,
                RRDF_FIELD_OPTS_VISIBLE | RRDF_FIELD_OPTS_UNIQUE_KEY | RRDF_FIELD_OPTS_WRAP | RRDF_FIELD_OPTS_FULL_WIDTH,
                NULL);

        buffer_rrdf_table_add_field(
                wb, field_id++,
                "rowOptions", "rowOptions",
                RRDF_FIELD_TYPE_NONE,
                RRDR_FIELD_VISUAL_ROW_OPTIONS,
                RRDF_FIELD_TRANSFORM_NONE, 0, NULL, NAN,
                RRDF_FIELD_SORT_FIXED,
                NULL,
                RRDF_FIELD_SUMMARY_COUNT,
                RRDF_FIELD_FILTER_NONE,
                RRDF_FIELD_OPTS_DUMMY,
                NULL);

        buffer_rrdf_table_add_field(wb, field_id++, "type", "Unit Type",
                RRDF_FIELD_TYPE_STRING, RRDF_FIELD_VISUAL_VALUE, RRDF_FIELD_TRANSFORM_NONE,
                0, NULL, NAN, RRDF_FIELD_SORT_ASCENDING, NULL,
                RRDF_FIELD_SUMMARY_COUNT, RRDF_FIELD_FILTER_MULTISELECT,
                RRDF_FIELD_OPTS_VISIBLE | RRDF_FIELD_OPTS_EXPANDED_FILTER,
                NULL);

        buffer_rrdf_table_add_field(wb, field_id++, "description", "Unit Description",
                RRDF_FIELD_TYPE_STRING, RRDF_FIELD_VISUAL_VALUE, RRDF_FIELD_TRANSFORM_NONE,
                0, NULL, NAN, RRDF_FIELD_SORT_ASCENDING, NULL,
                RRDF_FIELD_SUMMARY_COUNT, RRDF_FIELD_FILTER_NONE,
                RRDF_FIELD_OPTS_VISIBLE | RRDF_FIELD_OPTS_WRAP | RRDF_FIELD_OPTS_FULL_WIDTH,
                NULL);

        buffer_rrdf_table_add_field(wb, field_id++, "loadState", "Unit Load State",
                RRDF_FIELD_TYPE_STRING, RRDF_FIELD_VISUAL_VALUE, RRDF_FIELD_TRANSFORM_NONE,
                0, NULL, NAN, RRDF_FIELD_SORT_ASCENDING, NULL,
                RRDF_FIELD_SUMMARY_COUNT, RRDF_FIELD_FILTER_MULTISELECT,
                RRDF_FIELD_OPTS_VISIBLE | RRDF_FIELD_OPTS_EXPANDED_FILTER,
                NULL);

        buffer_rrdf_table_add_field(wb, field_id++, "activeState", "Unit Active State",
                RRDF_FIELD_TYPE_STRING, RRDF_FIELD_VISUAL_VALUE, RRDF_FIELD_TRANSFORM_NONE,
                0, NULL, NAN, RRDF_FIELD_SORT_ASCENDING, NULL,
                RRDF_FIELD_SUMMARY_COUNT, RRDF_FIELD_FILTER_MULTISELECT,
                RRDF_FIELD_OPTS_VISIBLE | RRDF_FIELD_OPTS_EXPANDED_FILTER,
                NULL);

        buffer_rrdf_table_add_field(wb, field_id++, "subState", "Unit Sub State",
                RRDF_FIELD_TYPE_STRING, RRDF_FIELD_VISUAL_VALUE, RRDF_FIELD_TRANSFORM_NONE,
                0, NULL, NAN, RRDF_FIELD_SORT_ASCENDING, NULL,
                RRDF_FIELD_SUMMARY_COUNT, RRDF_FIELD_FILTER_MULTISELECT,
                RRDF_FIELD_OPTS_VISIBLE | RRDF_FIELD_OPTS_EXPANDED_FILTER,
                NULL);

        buffer_rrdf_table_add_field(wb, field_id++, "following", "Unit Following",
                RRDF_FIELD_TYPE_STRING, RRDF_FIELD_VISUAL_VALUE, RRDF_FIELD_TRANSFORM_NONE,
                0, NULL, NAN, RRDF_FIELD_SORT_ASCENDING, NULL,
                RRDF_FIELD_SUMMARY_COUNT, RRDF_FIELD_FILTER_NONE,
                RRDF_FIELD_OPTS_WRAP,
                NULL);

        buffer_rrdf_table_add_field(wb, field_id++, "path", "Unit Path",
                RRDF_FIELD_TYPE_STRING, RRDF_FIELD_VISUAL_VALUE, RRDF_FIELD_TRANSFORM_NONE,
                0, NULL, NAN, RRDF_FIELD_SORT_ASCENDING, NULL,
                RRDF_FIELD_SUMMARY_COUNT, RRDF_FIELD_FILTER_NONE,
                RRDF_FIELD_OPTS_VISIBLE | RRDF_FIELD_OPTS_WRAP | RRDF_FIELD_OPTS_FULL_WIDTH,
                NULL);

        buffer_rrdf_table_add_field(wb, field_id++, "jobId", "Unit Job ID",
                RRDF_FIELD_TYPE_INTEGER, RRDF_FIELD_VISUAL_VALUE, RRDF_FIELD_TRANSFORM_NONE,
                0, NULL, NAN, RRDF_FIELD_SORT_ASCENDING, NULL,
                RRDF_FIELD_SUMMARY_COUNT, RRDF_FIELD_FILTER_NONE,
                RRDF_FIELD_OPTS_NONE,
                NULL);

        buffer_rrdf_table_add_field(wb, field_id++, "jobType", "Unit Job Type",
                RRDF_FIELD_TYPE_STRING, RRDF_FIELD_VISUAL_VALUE, RRDF_FIELD_TRANSFORM_NONE,
                0, NULL, NAN, RRDF_FIELD_SORT_ASCENDING, NULL,
                RRDF_FIELD_SUMMARY_COUNT, RRDF_FIELD_FILTER_MULTISELECT,
                RRDF_FIELD_OPTS_NONE,
                NULL);

        buffer_rrdf_table_add_field(wb, field_id++, "jobPath", "Unit Job Path",
                RRDF_FIELD_TYPE_STRING, RRDF_FIELD_VISUAL_VALUE, RRDF_FIELD_TRANSFORM_NONE,
                0, NULL, NAN, RRDF_FIELD_SORT_ASCENDING, NULL,
                RRDF_FIELD_SUMMARY_COUNT, RRDF_FIELD_FILTER_NONE,
                RRDF_FIELD_OPTS_NONE,
                NULL);

        buffer_rrdf_table_add_field(wb, field_id++, "priority", "Priority",
                RRDF_FIELD_TYPE_INTEGER, RRDF_FIELD_VISUAL_VALUE, RRDF_FIELD_TRANSFORM_NONE,
                0, NULL, NAN, RRDF_FIELD_SORT_ASCENDING, NULL,
                RRDF_FIELD_SUMMARY_COUNT, RRDF_FIELD_FILTER_NONE,
                RRDF_FIELD_OPTS_NONE,
                NULL);

        buffer_rrdf_table_add_field(wb, field_id++, "count", "Count",
                RRDF_FIELD_TYPE_INTEGER, RRDF_FIELD_VISUAL_VALUE, RRDF_FIELD_TRANSFORM_NONE,
                0, NULL, NAN, RRDF_FIELD_SORT_ASCENDING, NULL,
                RRDF_FIELD_SUMMARY_COUNT, RRDF_FIELD_FILTER_NONE,
                RRDF_FIELD_OPTS_NONE,
                NULL);
    }

    buffer_json_object_close(wb); // columns
    buffer_json_member_add_string(wb, "default_sort_column", "priority");

    buffer_json_member_add_object(wb, "charts");
    {
        buffer_json_member_add_object(wb, "count");
        {
            buffer_json_member_add_string(wb, "name", "count");
            buffer_json_member_add_string(wb, "type", "stacked-bar");
            buffer_json_member_add_array(wb, "columns");
            {
                buffer_json_add_array_item_string(wb, "count");
            }
            buffer_json_array_close(wb);
        }
        buffer_json_object_close(wb);
    }
    buffer_json_object_close(wb); // charts

    buffer_json_member_add_array(wb, "default_charts");
    {
        buffer_json_add_array_item_array(wb);
        buffer_json_add_array_item_string(wb, "count");
        buffer_json_add_array_item_string(wb, "activeState");
        buffer_json_array_close(wb);
        buffer_json_add_array_item_array(wb);
        buffer_json_add_array_item_string(wb, "count");
        buffer_json_add_array_item_string(wb, "subState");
        buffer_json_array_close(wb);
    }
    buffer_json_array_close(wb);

    buffer_json_member_add_object(wb, "group_by");
    {
        buffer_json_member_add_object(wb, "type");
        {
            buffer_json_member_add_string(wb, "name", "Top Down Tree");
            buffer_json_member_add_array(wb, "columns");
            {
                buffer_json_add_array_item_string(wb, "type");
                buffer_json_add_array_item_string(wb, "loadState");
                buffer_json_add_array_item_string(wb, "activeState");
                buffer_json_add_array_item_string(wb, "subState");
            }
            buffer_json_array_close(wb);
        }
        buffer_json_object_close(wb);

        buffer_json_member_add_object(wb, "subState");
        {
            buffer_json_member_add_string(wb, "name", "Bottom Up Tree");
            buffer_json_member_add_array(wb, "columns");
            {
                buffer_json_add_array_item_string(wb, "subState");
                buffer_json_add_array_item_string(wb, "activeState");
                buffer_json_add_array_item_string(wb, "loadState");
                buffer_json_add_array_item_string(wb, "type");
            }
            buffer_json_array_close(wb);
        }
        buffer_json_object_close(wb);
    }
    buffer_json_object_close(wb); // group_by

    buffer_json_member_add_time_t(wb, "expires", now_realtime_sec() + 1);
    buffer_json_finalize(wb);

    netdata_mutex_lock(&stdout_mutex);
    pluginsd_function_result_to_stdout(transaction, HTTP_RESP_OK, "application/json", now_realtime_sec() + 3600, wb);
    netdata_mutex_unlock(&stdout_mutex);

    buffer_free(wb);
    systemd_units_free_all(base);
}

#endif // ENABLE_SYSTEMD_DBUS
