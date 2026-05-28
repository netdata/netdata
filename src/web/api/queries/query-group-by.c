// SPDX-License-Identifier: GPL-3.0-or-later

#include "query-internal.h"

static inline bool group_by_is_separator(char c) {
    return c == ',' || c == '|' || c == ' ';
}

static inline bool group_by_token_matches(const char *token, size_t len, const char *name, size_t name_len) {
    if(!name)
        return false;

    return name_len == len && !strncmp(token, name, len);
}

RRDR_GROUP_BY group_by_parse(const char *group_by_txt) {
    RRDR_GROUP_BY group_by = RRDR_GROUP_BY_NONE;

    if(!group_by_txt || !*group_by_txt)
        return group_by;

    const char *s = group_by_txt;
    while(*s) {
        while(*s && group_by_is_separator(*s))
            s++;

        const char *key = s;
        while(*s && !group_by_is_separator(*s))
            s++;

        size_t len = (size_t)(s - key);
        if(!len) continue;

        if (group_by_token_matches(key, len, "selected", sizeof("selected") - 1))
            group_by |= RRDR_GROUP_BY_SELECTED;

        if (group_by_token_matches(key, len, "dimension", sizeof("dimension") - 1))
            group_by |= RRDR_GROUP_BY_DIMENSION;

        if (group_by_token_matches(key, len, "instance", sizeof("instance") - 1))
            group_by |= RRDR_GROUP_BY_INSTANCE;

        if (group_by_token_matches(key, len, "percentage-of-instance", sizeof("percentage-of-instance") - 1))
            group_by |= RRDR_GROUP_BY_PERCENTAGE_OF_INSTANCE;

        if (group_by_token_matches(key, len, "label", sizeof("label") - 1))
            group_by |= RRDR_GROUP_BY_LABEL;

        if (group_by_token_matches(key, len, "node", sizeof("node") - 1))
            group_by |= RRDR_GROUP_BY_NODE;

        if (group_by_token_matches(key, len, "context", sizeof("context") - 1))
            group_by |= RRDR_GROUP_BY_CONTEXT;

        if (group_by_token_matches(key, len, "units", sizeof("units") - 1))
            group_by |= RRDR_GROUP_BY_UNITS;
    }

    if((group_by & RRDR_GROUP_BY_SELECTED) && (group_by & ~RRDR_GROUP_BY_SELECTED)) {
        internal_error(true, "group-by given by query has 'selected' together with more groupings");
        group_by = RRDR_GROUP_BY_SELECTED; // remove all other groupings
    }

    if(group_by & RRDR_GROUP_BY_PERCENTAGE_OF_INSTANCE)
        group_by = RRDR_GROUP_BY_PERCENTAGE_OF_INSTANCE; // remove all other groupings

    return group_by;
}

void buffer_json_group_by_to_array(BUFFER *wb, RRDR_GROUP_BY group_by) {
    if(group_by == RRDR_GROUP_BY_NONE)
        buffer_json_add_array_item_string(wb, "none");
    else {
        if (group_by & RRDR_GROUP_BY_DIMENSION)
            buffer_json_add_array_item_string(wb, "dimension");

        if (group_by & RRDR_GROUP_BY_INSTANCE)
            buffer_json_add_array_item_string(wb, "instance");

        if (group_by & RRDR_GROUP_BY_PERCENTAGE_OF_INSTANCE)
            buffer_json_add_array_item_string(wb, "percentage-of-instance");

        if (group_by & RRDR_GROUP_BY_LABEL)
            buffer_json_add_array_item_string(wb, "label");

        if (group_by & RRDR_GROUP_BY_NODE)
            buffer_json_add_array_item_string(wb, "node");

        if (group_by & RRDR_GROUP_BY_CONTEXT)
            buffer_json_add_array_item_string(wb, "context");

        if (group_by & RRDR_GROUP_BY_UNITS)
            buffer_json_add_array_item_string(wb, "units");

        if (group_by & RRDR_GROUP_BY_SELECTED)
            buffer_json_add_array_item_string(wb, "selected");
    }
}

RRDR_GROUP_BY_FUNCTION group_by_aggregate_function_parse(const char *s) {
    if(strcmp(s, "average") == 0)
        return RRDR_GROUP_BY_FUNCTION_AVERAGE;

    if(strcmp(s, "avg") == 0)
        return RRDR_GROUP_BY_FUNCTION_AVERAGE;

    if(strcmp(s, "min") == 0)
        return RRDR_GROUP_BY_FUNCTION_MIN;

    if(strcmp(s, "max") == 0)
        return RRDR_GROUP_BY_FUNCTION_MAX;

    if(strcmp(s, "sum") == 0)
        return RRDR_GROUP_BY_FUNCTION_SUM;

    if(strcmp(s, "percentage") == 0)
        return RRDR_GROUP_BY_FUNCTION_PERCENTAGE;

    if(strcmp(s, "extremes") == 0)
        return RRDR_GROUP_BY_FUNCTION_EXTREMES;

    return RRDR_GROUP_BY_FUNCTION_AVERAGE;
}

const char *group_by_aggregate_function_to_string(RRDR_GROUP_BY_FUNCTION group_by_function) {
    switch(group_by_function) {
        default:
        case RRDR_GROUP_BY_FUNCTION_AVERAGE:
            return "average";

        case RRDR_GROUP_BY_FUNCTION_MIN:
            return "min";

        case RRDR_GROUP_BY_FUNCTION_MAX:
            return "max";

        case RRDR_GROUP_BY_FUNCTION_SUM:
            return "sum";

        case RRDR_GROUP_BY_FUNCTION_PERCENTAGE:
            return "percentage";

        case RRDR_GROUP_BY_FUNCTION_EXTREMES:
            return "extremes";
    }
}

// ----------------------------------------------------------------------------
// group by

void group_by_label_key_insert_cb(const DICTIONARY_ITEM *item __maybe_unused, void *value, void *data) {
    // add the key to our r->label_keys global keys dictionary
    DICTIONARY *label_keys = data;
    dictionary_set(label_keys, dictionary_acquired_item_name(item), NULL, 0);

    // create a dictionary for the values of this key
    struct group_by_label_key *k = value;
    k->values = dictionary_create_advanced(DICT_OPTION_SINGLE_THREADED | DICT_OPTION_DONT_OVERWRITE_VALUE, NULL, 0);
}

void group_by_label_key_delete_cb(const DICTIONARY_ITEM *item __maybe_unused, void *value, void *data __maybe_unused) {
    struct group_by_label_key *k = value;
    dictionary_destroy(k->values);
}

int rrdlabels_traversal_cb_to_group_by_label_key(const char *name, const char *value, RRDLABEL_SRC ls __maybe_unused, void *data) {
    DICTIONARY *dl = data;
    struct group_by_label_key *k = dictionary_set(dl, name, NULL, sizeof(struct group_by_label_key));
    dictionary_set(k->values, value, NULL, 0);
    return 1;
}

void rrdr_json_group_by_labels(BUFFER *wb, const char *key, RRDR *r, RRDR_OPTIONS options) {
    if(!r->label_keys || !r->dl)
        return;

    buffer_json_member_add_object(wb, key);

    void *t;
    dfe_start_read(r->label_keys, t) {
        buffer_json_member_add_array(wb, t_dfe.name);

        for(size_t d = 0; d < r->d ;d++) {
            if(!rrdr_dimension_should_be_exposed(r->od[d], options))
                continue;

            struct group_by_label_key *k = dictionary_get(r->dl[d], t_dfe.name);
            if(k) {
                buffer_json_add_array_item_array(wb);
                void *tt;
                dfe_start_read(k->values, tt) {
                    buffer_json_add_array_item_string(wb, tt_dfe.name);
                }
                dfe_done(tt);
                buffer_json_array_close(wb);
            }
            else
                buffer_json_add_array_item_string(wb, NULL);
        }

        buffer_json_array_close(wb);
    }
    dfe_done(t);

    buffer_json_object_close(wb); // key
}

void rrd2rrdr_set_timestamps(RRDR *r) {
    QUERY_TARGET *qt = r->internal.qt;

    internal_fatal(qt->window.points != r->n, "QUERY: mismatch to the number of points in qt and r");

    r->view.group = qt->window.group;
    r->view.update_every = (int) query_view_update_every(qt);
    r->view.before = qt->window.before;
    r->view.after = qt->window.after;

    r->time_grouping.points_wanted = qt->window.points;
    r->time_grouping.resampling_group = qt->window.resampling_group;
    r->time_grouping.resampling_divisor = qt->window.resampling_divisor;

    r->rows = qt->window.points;

    size_t points_wanted = qt->window.points;
    time_t after_wanted = qt->window.after;
    time_t before_wanted = qt->window.before; (void)before_wanted;

    time_t view_update_every = r->view.update_every;
    time_t query_granularity = (time_t)(r->view.update_every / r->view.group);

    size_t rrdr_line = 0;
    time_t first_point_end_time = after_wanted + view_update_every - query_granularity;
    time_t now_end_time = first_point_end_time;

    while (rrdr_line < points_wanted) {
        r->t[rrdr_line++] = now_end_time;
        now_end_time += view_update_every;
    }

    internal_fatal(r->t[0] != first_point_end_time, "QUERY: wrong first timestamp in the query");
    internal_error(r->t[points_wanted - 1] != before_wanted,
                   "QUERY: wrong last timestamp in the query, expected %ld, found %ld",
                   before_wanted, r->t[points_wanted - 1]);
}
