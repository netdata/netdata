#include "facets.h"

#define HISTOGRAM_COLUMNS 60

static void facets_row_free(FACETS *facets __maybe_unused, FACET_ROW *row);

// ----------------------------------------------------------------------------

static inline void uint64_to_char(uint64_t num, char *out) {
    static const char id_encoding_characters[64 + 1] = "ABCDEFGHIJKLMNOPQRSTUVWXYZ.abcdefghijklmnopqrstuvwxyz_0123456789";

    int i;
    for(i = 10; i >= 0; --i) {
        out[i] = id_encoding_characters[num & 63];
        num >>= 6;
    }
}

inline void facets_string_hash(const char *src, size_t len, char *out) {
    XXH128_hash_t hash = XXH3_128bits(src, len);

    uint64_to_char(hash.high64, out);
    uint64_to_char(hash.low64, &out[11]);  // Starts right after the first 64-bit encoded string

    out[FACET_STRING_HASH_SIZE - 1] = '\0';
}

// ----------------------------------------------------------------------------

typedef struct facet_value {
    const char *name;

    bool selected;

    uint32_t rows_matching_facet_value;
    uint32_t final_facet_value_counter;

    uint32_t *histogram;
    uint32_t min, max, sum;
} FACET_VALUE;

struct facet_key {
    const char *name;

    DICTIONARY *values;

    FACET_KEY_OPTIONS options;

    bool default_selected_for_values; // the default "selected" for all values in the dictionary

    // members about the current row
    uint32_t key_found_in_row;
    uint32_t key_values_selected_in_row;

    struct {
        char hash[FACET_STRING_HASH_SIZE];
        bool updated;
        BUFFER *b;
    } current_value;

    uint32_t order;

    struct {
        facet_dynamic_row_t cb;
        void *data;
    } dynamic;

    struct {
        facets_key_transformer_t cb;
        void *data;
    } transform;

    struct facet_key *prev, *next;
};

struct facets {
    SIMPLE_PATTERN *visible_keys;
    SIMPLE_PATTERN *excluded_keys;
    SIMPLE_PATTERN *included_keys;

    FACETS_OPTIONS options;
    usec_t anchor;

    SIMPLE_PATTERN *query;          // the full text search pattern
    size_t keys_filtered_by_query;  // the number of fields we do full text search (constant)
    size_t keys_matched_by_query;   // the number of fields matched the full text search (per row)

    DICTIONARY *accepted_params;

    FACET_KEY *keys_ll;
    DICTIONARY *keys;
    FACET_ROW *base;    // double linked list of the selected facets rows

    uint32_t items_to_return;
    uint32_t max_items_to_return;
    uint32_t order;

    struct {
        char *chart;
        bool enabled;
        uint32_t slots;
        usec_t slot_width;
        usec_t after_ut;
        usec_t before_ut;
    } histogram;

    struct {
        FACET_ROW *last_added;

        size_t evaluated;
        size_t matched;

        size_t first;
        size_t forwards;
        size_t backwards;
        size_t skips_before;
        size_t skips_after;
        size_t prepends;
        size_t appends;
        size_t shifts;
    } operations;
};

// ----------------------------------------------------------------------------

static usec_t calculate_histogram_bar_width(usec_t after_ut, usec_t before_ut) {
    // Array of valid durations in seconds
    static time_t valid_durations[] = {
            1,
            15,
            30,
            1 * 60, 2 * 60, 3 * 60, 5 * 60, 10 * 60, 15 * 60, 30 * 60,          // minutes
            1 * 3600, 2 * 3600, 6 * 3600, 8 * 3600, 12 * 3600,                  // hours
            1 * 86400, 2 * 86400, 3 * 86400, 5 * 86400, 7 * 86400, 14 * 86400,  // days
            1 * (30*86400)                                                      // months
    };
    static int array_size = sizeof(valid_durations) / sizeof(valid_durations[0]);

    usec_t duration = before_ut - after_ut;
    usec_t bar_width = 1 * 60;

    for (int i = array_size - 1; i >= 0; --i) {
        if (duration / (valid_durations[i] * 60) >= HISTOGRAM_COLUMNS) {
            bar_width = valid_durations[i] * 60;
            break;
        }
    }

    return bar_width;
}

static inline usec_t facets_histogram_slot_baseline_ut(FACETS *facets, usec_t ut) {
    usec_t delta = ut % facets->histogram.slot_width;
    return ut - delta;
}

void facets_set_histogram(FACETS *facets, const char *chart, usec_t after_ut, usec_t before_ut) {
    facets->histogram.enabled = true;
    facets->histogram.chart = chart ? strdupz(chart) : NULL;
    facets->histogram.slot_width = calculate_histogram_bar_width(after_ut, before_ut);
    facets->histogram.after_ut = facets_histogram_slot_baseline_ut(facets, after_ut);
    facets->histogram.before_ut = facets_histogram_slot_baseline_ut(facets, before_ut) + facets->histogram.slot_width;
    facets->histogram.slots = (facets->histogram.before_ut - facets->histogram.after_ut) / facets->histogram.slot_width + 1;
}

static inline void facets_histogram_update_value(FACETS *facets, FACET_KEY *k __maybe_unused, FACET_VALUE *v, usec_t usec) {
    if(!facets->histogram.enabled)
        return;

    if(unlikely(!v->histogram))
        v->histogram = callocz(facets->histogram.slots, sizeof(*v->histogram));

    usec_t base_ut = facets_histogram_slot_baseline_ut(facets, usec);

    if(base_ut < facets->histogram.after_ut)
        base_ut = facets->histogram.after_ut;

    if(base_ut > facets->histogram.before_ut)
        base_ut = facets->histogram.before_ut;

    uint32_t slot = (base_ut - facets->histogram.after_ut) / facets->histogram.slot_width;

    if(unlikely(slot >= facets->histogram.slots))
        slot = facets->histogram.slots - 1;

    v->histogram[slot]++;
}

static inline void facets_histogram_value_names(BUFFER *wb, FACETS *facets __maybe_unused, FACET_KEY *k, const char *key) {
    buffer_json_member_add_array(wb, key);
    {
        FACET_VALUE *v;
        dfe_start_read(k->values, v) {
            if(unlikely(!v->histogram))
                continue;

            buffer_json_add_array_item_string(wb, v->name);
        }
        dfe_done(v);
    }
    buffer_json_array_close(wb); // key
}

static inline void facets_histogram_value_units(BUFFER *wb, FACETS *facets __maybe_unused, FACET_KEY *k, const char *key) {
    buffer_json_member_add_array(wb, key);
    {
        FACET_VALUE *v;
        dfe_start_read(k->values, v) {
            if(unlikely(!v->histogram))
                continue;

            buffer_json_add_array_item_string(wb, "events");
        }
        dfe_done(v);
    }
    buffer_json_array_close(wb); // key
}

static inline void facets_histogram_value_min(BUFFER *wb, FACETS *facets __maybe_unused, FACET_KEY *k, const char *key) {
    buffer_json_member_add_array(wb, key);
    {
        FACET_VALUE *v;
        dfe_start_read(k->values, v) {
            if(unlikely(!v->histogram))
                continue;

            buffer_json_add_array_item_uint64(wb, v->min);
        }
        dfe_done(v);
    }
    buffer_json_array_close(wb); // key
}

static inline void facets_histogram_value_max(BUFFER *wb, FACETS *facets __maybe_unused, FACET_KEY *k, const char *key) {
    buffer_json_member_add_array(wb, key);
    {
        FACET_VALUE *v;
        dfe_start_read(k->values, v) {
                    if(unlikely(!v->histogram))
                        continue;

                    buffer_json_add_array_item_uint64(wb, v->max);
                }
        dfe_done(v);
    }
    buffer_json_array_close(wb); // key
}

static inline void facets_histogram_value_avg(BUFFER *wb, FACETS *facets __maybe_unused, FACET_KEY *k, const char *key) {
    buffer_json_member_add_array(wb, key);
    {
        FACET_VALUE *v;
        dfe_start_read(k->values, v) {
            if(unlikely(!v->histogram))
                continue;

            buffer_json_add_array_item_double(wb, (double)v->sum / (double)facets->histogram.slots);
        }
        dfe_done(v);
    }
    buffer_json_array_close(wb); // key
}

static inline void facets_histogram_value_arp(BUFFER *wb, FACETS *facets __maybe_unused, FACET_KEY *k, const char *key) {
    buffer_json_member_add_array(wb, key);
    {
        FACET_VALUE *v;
        dfe_start_read(k->values, v) {
            if(unlikely(!v->histogram))
                continue;

            buffer_json_add_array_item_uint64(wb, 0);
        }
        dfe_done(v);
    }
    buffer_json_array_close(wb); // key
}

static inline void facets_histogram_value_con(BUFFER *wb, FACETS *facets __maybe_unused, FACET_KEY *k, const char *key, uint32_t sum) {
    buffer_json_member_add_array(wb, key);
    {
        FACET_VALUE *v;
        dfe_start_read(k->values, v) {
            if(unlikely(!v->histogram))
                continue;

            buffer_json_add_array_item_double(wb, (double)v->sum * 100.0 / (double)sum);
        }
        dfe_done(v);
    }
    buffer_json_array_close(wb); // key
}

static void facets_histogram_generate(FACETS *facets, FACET_KEY *k, BUFFER *wb) {
    size_t dimensions = 0;
    uint32_t min = UINT32_MAX, max = 0, sum = 0, count = 0;

    {
        FACET_VALUE *v;
        dfe_start_read(k->values, v){
            if (unlikely(!v->histogram))
                continue;

            dimensions++;

            v->min = UINT32_MAX;
            v->max = 0;
            v->sum = 0;

            for(uint32_t i = 0; i < facets->histogram.slots ;i++) {
                uint32_t n = v->histogram[i];

                if(n < min)
                    min = n;

                if(n > max)
                    max = n;

                sum += n;
                count++;

                if(n < v->min)
                    v->min = n;

                if(n > v->max)
                    v->max = n;

                v->sum += n;
            }
        }
        dfe_done(v);
    }

    if(!dimensions)
        return;

    buffer_json_member_add_object(wb, "summary");
    {
        buffer_json_member_add_array(wb, "nodes");
        {
            buffer_json_add_array_item_object(wb); // node
            {
                buffer_json_member_add_string(wb, "mg", "default");
                buffer_json_member_add_string(wb, "nm", "facets.histogram");
                buffer_json_member_add_uint64(wb, "ni", 0);
                buffer_json_member_add_object(wb, "st");
                {
                    buffer_json_member_add_uint64(wb, "ai", 0);
                    buffer_json_member_add_uint64(wb, "code", 200);
                    buffer_json_member_add_string(wb, "msg", "");
                }
                buffer_json_object_close(wb); // st
                buffer_json_member_add_object(wb, "is");
                {
                    buffer_json_member_add_uint64(wb, "sl", 1);
                    buffer_json_member_add_uint64(wb, "qr", 1);
                }
                buffer_json_object_close(wb); // is
                buffer_json_member_add_object(wb, "ds");
                {
                    buffer_json_member_add_uint64(wb, "sl", dimensions);
                    buffer_json_member_add_uint64(wb, "qr", dimensions);
                }
                buffer_json_object_close(wb); // ds
                buffer_json_member_add_object(wb, "sts");
                {
                    buffer_json_member_add_uint64(wb, "min", min);
                    buffer_json_member_add_uint64(wb, "max", max);
                    buffer_json_member_add_double(wb, "avg", (double)sum / (double)count);
                    buffer_json_member_add_double(wb, "con", 100.0);
                }
                buffer_json_object_close(wb); // sts
            }
            buffer_json_object_close(wb); // node
        }
        buffer_json_array_close(wb); // nodes

        buffer_json_member_add_array(wb, "contexts");
        {
            buffer_json_add_array_item_object(wb); // context
            {
                buffer_json_member_add_string(wb, "id", "facets.histogram");
                buffer_json_member_add_object(wb, "is");
                {
                    buffer_json_member_add_uint64(wb, "sl", 1);
                    buffer_json_member_add_uint64(wb, "qr", 1);
                }
                buffer_json_object_close(wb); // is
                buffer_json_member_add_object(wb, "ds");
                {
                    buffer_json_member_add_uint64(wb, "sl", dimensions);
                    buffer_json_member_add_uint64(wb, "qr", dimensions);
                }
                buffer_json_object_close(wb); // ds
                buffer_json_member_add_object(wb, "sts");
                {
                    buffer_json_member_add_uint64(wb, "min", min);
                    buffer_json_member_add_uint64(wb, "max", max);
                    buffer_json_member_add_double(wb, "avg", (double)sum / (double)count);
                    buffer_json_member_add_double(wb, "con", 100.0);
                }
                buffer_json_object_close(wb); // sts
            }
            buffer_json_object_close(wb); // context
        }
        buffer_json_array_close(wb); // contexts

        buffer_json_member_add_array(wb, "instances");
        {
            buffer_json_add_array_item_object(wb); // instance
            {
                buffer_json_member_add_string(wb, "id", "facets.histogram");
                buffer_json_member_add_uint64(wb, "ni", 0);
                buffer_json_member_add_object(wb, "ds");
                {
                    buffer_json_member_add_uint64(wb, "sl", dimensions);
                    buffer_json_member_add_uint64(wb, "qr", dimensions);
                }
                buffer_json_object_close(wb); // ds
                buffer_json_member_add_object(wb, "sts");
                {
                    buffer_json_member_add_uint64(wb, "min", min);
                    buffer_json_member_add_uint64(wb, "max", max);
                    buffer_json_member_add_double(wb, "avg", (double)sum / (double)count);
                    buffer_json_member_add_double(wb, "con", 100.0);
                }
                buffer_json_object_close(wb); // sts
            }
            buffer_json_object_close(wb); // instance
        }
        buffer_json_array_close(wb); // instances

        buffer_json_member_add_array(wb, "dimensions");
        {
            size_t pri = 0;
            FACET_VALUE *v;
            dfe_start_read(k->values, v) {
                if(unlikely(!v->histogram))
                    continue;

                buffer_json_add_array_item_object(wb); // dimension
                {
                    buffer_json_member_add_string(wb, "id", v->name);
                    buffer_json_member_add_object(wb, "ds");
                    {
                        buffer_json_member_add_uint64(wb, "sl", 1);
                        buffer_json_member_add_uint64(wb, "qr", 1);
                    }
                    buffer_json_object_close(wb); // ds
                    buffer_json_member_add_object(wb, "sts");
                    {
                        buffer_json_member_add_uint64(wb, "min", v->min);
                        buffer_json_member_add_uint64(wb, "max", v->max);
                        buffer_json_member_add_double(wb, "avg", (double)v->sum / (double)facets->histogram.slots);
                        buffer_json_member_add_double(wb, "con", (double)v->sum * 100.0 / (double)sum);
                    }
                    buffer_json_object_close(wb); // sts
                    buffer_json_member_add_uint64(wb, "pri", pri++);
                }
                buffer_json_object_close(wb); // dimension
            }
            dfe_done(v);
        }
        buffer_json_array_close(wb); // dimensions

        buffer_json_member_add_array(wb, "labels");
        buffer_json_array_close(wb); // labels

        buffer_json_member_add_array(wb, "alerts");
        buffer_json_array_close(wb); // alerts
    }
    buffer_json_object_close(wb); // summary

    buffer_json_member_add_object(wb, "totals");
    {
        buffer_json_member_add_object(wb, "nodes");
        {
            buffer_json_member_add_uint64(wb, "sl", 1);
            buffer_json_member_add_uint64(wb, "qr", 1);
        }
        buffer_json_object_close(wb); // nodes;
        buffer_json_member_add_object(wb, "contexts");
        {
            buffer_json_member_add_uint64(wb, "sl", 1);
            buffer_json_member_add_uint64(wb, "qr", 1);
        }
        buffer_json_object_close(wb); // contexts;
        buffer_json_member_add_object(wb, "dimensions");
        {
            buffer_json_member_add_uint64(wb, "sl", dimensions);
            buffer_json_member_add_uint64(wb, "qr", dimensions);
        }
        buffer_json_object_close(wb); // contexts;
    }
    buffer_json_object_close(wb); // totals

    buffer_json_member_add_object(wb, "result");
    {
        facets_histogram_value_names(wb, facets, k, "labels");

        buffer_json_member_add_object(wb, "point");
        {
            buffer_json_member_add_uint64(wb, "value", 0);
            buffer_json_member_add_uint64(wb, "arp", 1);
            buffer_json_member_add_uint64(wb, "pa", 2);
        }
        buffer_json_object_close(wb); // point

        buffer_json_member_add_array(wb, "data");
        {
            usec_t t = facets->histogram.after_ut;
            for(uint32_t i = 0; i < facets->histogram.slots ;i++) {
                buffer_json_add_array_item_array(wb); // row
                {
                    buffer_json_add_array_item_time_ms(wb, t / USEC_PER_SEC);

                    FACET_VALUE *v;
                    dfe_start_read(k->values, v) {
                        if(unlikely(!v->histogram))
                            continue;

                        buffer_json_add_array_item_array(wb); // point

                        buffer_json_add_array_item_uint64(wb, v->histogram[i]);
                        buffer_json_add_array_item_uint64(wb, 0);
                        buffer_json_add_array_item_uint64(wb, 1);

                        buffer_json_array_close(wb); // point
                    }
                    dfe_done(v);
                }
                buffer_json_array_close(wb); // row

                t += facets->histogram.slot_width;
            }
        }
        buffer_json_array_close(wb); //data
    }
    buffer_json_object_close(wb); // result

    buffer_json_member_add_object(wb, "db");
    {
        buffer_json_member_add_uint64(wb, "tiers", 1);
        buffer_json_member_add_uint64(wb, "update_every", 1);
        buffer_json_member_add_time_t(wb, "first_entry", facets->histogram.after_ut / USEC_PER_SEC);
        buffer_json_member_add_time_t(wb, "last_entry", facets->histogram.before_ut / USEC_PER_SEC);
        buffer_json_member_add_string(wb, "units", "events");
        buffer_json_member_add_object(wb, "dimensions");
        {
            facets_histogram_value_names(wb, facets, k, "ids");
            facets_histogram_value_units(wb, facets, k, "units");

            buffer_json_member_add_object(wb, "sts");
            {
                facets_histogram_value_min(wb, facets, k, "min");
                facets_histogram_value_max(wb, facets, k, "max");
                facets_histogram_value_avg(wb, facets, k, "avg");
                facets_histogram_value_arp(wb, facets, k, "arp");
                facets_histogram_value_con(wb, facets, k, "con", sum);
            }
            buffer_json_object_close(wb); // sts
        }
        buffer_json_object_close(wb); // dimensions

        buffer_json_member_add_array(wb, "per_tier");
        {
            buffer_json_add_array_item_object(wb); // tier0
            {
                buffer_json_member_add_uint64(wb, "tier", 0);
                buffer_json_member_add_uint64(wb, "queries", 1);
                buffer_json_member_add_uint64(wb, "points", count);
                buffer_json_member_add_time_t(wb, "update_every", 1);
                buffer_json_member_add_time_t(wb, "first_entry", facets->histogram.after_ut / USEC_PER_SEC);
                buffer_json_member_add_time_t(wb, "last_entry", facets->histogram.before_ut / USEC_PER_SEC);
            }
            buffer_json_object_close(wb); // tier0
        }
        buffer_json_array_close(wb); // per_tier
    }
    buffer_json_object_close(wb); // db

    buffer_json_member_add_object(wb, "view");
    {
        buffer_json_member_add_string(wb, "title", "Events Distribution");
        buffer_json_member_add_time_t(wb, "update_every", 1);
        buffer_json_member_add_time_t(wb, "after", facets->histogram.after_ut / USEC_PER_SEC);
        buffer_json_member_add_time_t(wb, "before", facets->histogram.before_ut / USEC_PER_SEC);
        buffer_json_member_add_string(wb, "units", "events");
        buffer_json_member_add_string(wb, "chart_type", "stacked");
        buffer_json_member_add_object(wb, "dimensions");
        {
            buffer_json_member_add_array(wb, "grouped_by");
            {
                buffer_json_add_array_item_string(wb, "dimension");
            }
            buffer_json_array_close(wb); // grouped_by

            facets_histogram_value_names(wb, facets, k, "ids");
            facets_histogram_value_names(wb, facets, k, "names");
            facets_histogram_value_units(wb, facets, k, "units");

            buffer_json_member_add_object(wb, "sts");
            {
                facets_histogram_value_min(wb, facets, k, "min");
                facets_histogram_value_max(wb, facets, k, "max");
                facets_histogram_value_avg(wb, facets, k, "avg");
                facets_histogram_value_arp(wb, facets, k, "arp");
                facets_histogram_value_con(wb, facets, k, "con", sum);
            }
            buffer_json_object_close(wb); // sts
        }
        buffer_json_object_close(wb); // dimensions

        buffer_json_member_add_uint64(wb, "min", min);
        buffer_json_member_add_uint64(wb, "max", max);
    }
    buffer_json_object_close(wb); // view

    buffer_json_member_add_array(wb, "agents");
    {
        buffer_json_add_array_item_object(wb); // agent
        {
            buffer_json_member_add_string(wb, "mg", "default");
            buffer_json_member_add_string(wb, "nm", "facets.histogram");
            buffer_json_member_add_time_t(wb, "now", now_realtime_sec());
            buffer_json_member_add_uint64(wb, "ai", 0);
        }
        buffer_json_object_close(wb); // agent
    }
    buffer_json_array_close(wb); // agents
}

// ----------------------------------------------------------------------------

static inline void facet_value_is_used(FACET_KEY *k, FACET_VALUE *v) {
    if(!k->key_found_in_row)
        v->rows_matching_facet_value++;

    k->key_found_in_row++;

    if(v->selected)
        k->key_values_selected_in_row++;
}

static inline bool facets_key_is_facet(FACETS *facets, FACET_KEY *k) {
    bool included = true, excluded = false;

    if(k->options & (FACET_KEY_OPTION_FACET | FACET_KEY_OPTION_NO_FACET)) {
        if(k->options & FACET_KEY_OPTION_FACET) {
            included = true;
            excluded = false;
        }
        else if(k->options & FACET_KEY_OPTION_NO_FACET) {
            included = false;
            excluded = true;
        }
    }
    else {
        if (facets->included_keys) {
            if (!simple_pattern_matches(facets->included_keys, k->name))
                included = false;
        }

        if (facets->excluded_keys) {
            if (simple_pattern_matches(facets->excluded_keys, k->name))
                excluded = true;
        }
    }

    if(included && !excluded) {
        k->options |= FACET_KEY_OPTION_FACET;
        k->options &= ~FACET_KEY_OPTION_NO_FACET;
        return true;
    }

    k->options |= FACET_KEY_OPTION_NO_FACET;
    k->options &= ~FACET_KEY_OPTION_FACET;
    return false;
}

// ----------------------------------------------------------------------------
// FACET_VALUE dictionary hooks

static void facet_value_insert_callback(const DICTIONARY_ITEM *item __maybe_unused, void *value, void *data) {
    FACET_VALUE *v = value;
    FACET_KEY *k = data;

    if(!v->selected)
        v->selected = k->default_selected_for_values;

    if(v->name) {
        // an actual value, not a filter
        v->name = strdupz(v->name);
        facet_value_is_used(k, v);
    }
}

static bool facet_value_conflict_callback(const DICTIONARY_ITEM *item __maybe_unused, void *old_value, void *new_value, void *data) {
    FACET_VALUE *v = old_value;
    FACET_VALUE *nv = new_value;
    FACET_KEY *k = data;

    if(!v->name && nv->name)
        // an actual value, not a filter
        v->name = strdupz(nv->name);

    if(v->name)
        facet_value_is_used(k, v);

    internal_fatal(v->name && nv->name && strcmp(v->name, nv->name) != 0, "value hash conflict: '%s' and '%s' have the same hash '%s'", v->name, nv->name,
                   dictionary_acquired_item_name(item));

    return false;
}

static void facet_value_delete_callback(const DICTIONARY_ITEM *item __maybe_unused, void *value, void *data __maybe_unused) {
    FACET_VALUE *v = value;
    freez(v->histogram);
    freez((char *)v->name);
}

// ----------------------------------------------------------------------------
// FACET_KEY dictionary hooks

static inline void facet_key_late_init(FACETS *facets, FACET_KEY *k) {
    if(k->values)
        return;

    if(facets_key_is_facet(facets, k)) {
        k->values = dictionary_create_advanced(
                DICT_OPTION_SINGLE_THREADED | DICT_OPTION_DONT_OVERWRITE_VALUE | DICT_OPTION_FIXED_SIZE,
                NULL, sizeof(FACET_VALUE));
        dictionary_register_insert_callback(k->values, facet_value_insert_callback, k);
        dictionary_register_conflict_callback(k->values, facet_value_conflict_callback, k);
        dictionary_register_delete_callback(k->values, facet_value_delete_callback, k);
    }
}

static void facet_key_insert_callback(const DICTIONARY_ITEM *item __maybe_unused, void *value, void *data) {
    FACET_KEY *k = value;
    FACETS *facets = data;

    if(!(k->options & FACET_KEY_OPTION_REORDER))
        k->order = facets->order++;

    if((k->options & FACET_KEY_OPTION_FTS) || (facets->options & FACETS_OPTION_ALL_KEYS_FTS))
        facets->keys_filtered_by_query++;

    if(k->name) {
        // an actual value, not a filter
        k->name = strdupz(k->name);
        facet_key_late_init(facets, k);
    }

    k->current_value.b = buffer_create(0, NULL);

    DOUBLE_LINKED_LIST_APPEND_ITEM_UNSAFE(facets->keys_ll, k, prev, next);
}

static bool facet_key_conflict_callback(const DICTIONARY_ITEM *item __maybe_unused, void *old_value, void *new_value, void *data) {
    FACET_KEY *k = old_value;
    FACET_KEY *nk = new_value;
    FACETS *facets = data;

    if(!k->name && nk->name) {
        // an actual value, not a filter
        k->name = strdupz(nk->name);
        facet_key_late_init(facets, k);
    }

    internal_fatal(k->name && nk->name && strcmp(k->name, nk->name) != 0, "key hash conflict: '%s' and '%s' have the same hash '%s'", k->name, nk->name,
                   dictionary_acquired_item_name(item));

    if(k->options & FACET_KEY_OPTION_REORDER) {
        k->order = facets->order++;
        k->options &= ~FACET_KEY_OPTION_REORDER;
    }

    return false;
}

static void facet_key_delete_callback(const DICTIONARY_ITEM *item __maybe_unused, void *value, void *data __maybe_unused) {
    FACET_KEY *k = value;
    FACETS *facets = data;

    DOUBLE_LINKED_LIST_REMOVE_ITEM_UNSAFE(facets->keys_ll, k, prev, next);

    dictionary_destroy(k->values);
    buffer_free(k->current_value.b);
    freez((char *)k->name);
}

// ----------------------------------------------------------------------------

FACETS *facets_create(uint32_t items_to_return, usec_t anchor, FACETS_OPTIONS options, const char *visible_keys, const char *facet_keys, const char *non_facet_keys) {
    FACETS *facets = callocz(1, sizeof(FACETS));
    facets->options = options;
    facets->keys = dictionary_create_advanced(DICT_OPTION_SINGLE_THREADED|DICT_OPTION_DONT_OVERWRITE_VALUE|DICT_OPTION_FIXED_SIZE, NULL, sizeof(FACET_KEY));
    dictionary_register_insert_callback(facets->keys, facet_key_insert_callback, facets);
    dictionary_register_conflict_callback(facets->keys, facet_key_conflict_callback, facets);
    dictionary_register_delete_callback(facets->keys, facet_key_delete_callback, facets);

    if(facet_keys && *facet_keys)
        facets->included_keys = simple_pattern_create(facet_keys, "|", SIMPLE_PATTERN_EXACT, true);

    if(non_facet_keys && *non_facet_keys)
        facets->excluded_keys = simple_pattern_create(non_facet_keys, "|", SIMPLE_PATTERN_EXACT, true);

    if(visible_keys && *visible_keys)
        facets->visible_keys = simple_pattern_create(visible_keys, "|", SIMPLE_PATTERN_EXACT, true);

    facets->max_items_to_return = items_to_return;
    facets->anchor = anchor;
    facets->order = 1;

    return facets;
}

void facets_destroy(FACETS *facets) {
    dictionary_destroy(facets->accepted_params);
    dictionary_destroy(facets->keys);
    simple_pattern_free(facets->visible_keys);
    simple_pattern_free(facets->included_keys);
    simple_pattern_free(facets->excluded_keys);

    while(facets->base) {
        FACET_ROW *r = facets->base;
        DOUBLE_LINKED_LIST_REMOVE_ITEM_UNSAFE(facets->base, r, prev, next);

        facets_row_free(facets, r);
    }

    freez(facets->histogram.chart);
    freez(facets);
}

void facets_accepted_param(FACETS *facets, const char *param) {
    if(!facets->accepted_params)
        facets->accepted_params = dictionary_create(DICT_OPTION_SINGLE_THREADED|DICT_OPTION_DONT_OVERWRITE_VALUE);

    dictionary_set(facets->accepted_params, param, NULL, 0);
}

inline FACET_KEY *facets_register_key(FACETS *facets, const char *key, FACET_KEY_OPTIONS options) {
    FACET_KEY tk = {
            .name = key,
            .options = options,
            .default_selected_for_values = true,
    };
    char hash[FACET_STRING_HASH_SIZE];
    facets_string_hash(tk.name, strlen(key), hash);
    return dictionary_set(facets->keys, hash, &tk, sizeof(tk));
}

inline FACET_KEY *facets_register_key_transformation(FACETS *facets, const char *key, FACET_KEY_OPTIONS options, facets_key_transformer_t cb, void *data) {
    FACET_KEY *k = facets_register_key(facets, key, options);
    k->transform.cb = cb;
    k->transform.data = data;
    return k;
}

inline FACET_KEY *facets_register_dynamic_key(FACETS *facets, const char *key, FACET_KEY_OPTIONS options, facet_dynamic_row_t cb, void *data) {
    FACET_KEY *k = facets_register_key(facets, key, options);
    k->dynamic.cb = cb;
    k->dynamic.data = data;
    return k;
}

void facets_set_query(FACETS *facets, const char *query) {
    if(!query)
        return;

    facets->query = simple_pattern_create(query, " \t", SIMPLE_PATTERN_SUBSTRING, false);
}

void facets_set_items(FACETS *facets, uint32_t items) {
    facets->max_items_to_return = items;
}

void facets_set_anchor(FACETS *facets, usec_t anchor) {
    facets->anchor = anchor;
}

void facets_register_facet_filter(FACETS *facets, const char *key_id, char *value_ids, FACET_KEY_OPTIONS options) {
    FACET_KEY tk = {
            .options = options,
    };
    FACET_KEY *k = dictionary_set(facets->keys, key_id, &tk, sizeof(tk));

    k->default_selected_for_values = false;
    k->options |= FACET_KEY_OPTION_FACET;
    k->options &= ~FACET_KEY_OPTION_NO_FACET;
    facet_key_late_init(facets, k);

    FACET_VALUE tv = {
            .selected = true,
    };
    dictionary_set(k->values, value_ids, &tv, sizeof(tv));
}

// ----------------------------------------------------------------------------

static inline void facets_check_value(FACETS *facets __maybe_unused, FACET_KEY *k) {
    if(!k->current_value.updated)
        buffer_flush(k->current_value.b);

    if(k->transform.cb)
        k->transform.cb(facets, k->current_value.b, k->transform.data);

    if(!k->current_value.updated) {
        buffer_fast_strcat(k->current_value.b, FACET_VALUE_UNSET, sizeof(FACET_VALUE_UNSET) - 1);
        k->current_value.updated = true;
    }

//    bool found = false;
//    if(strstr(buffer_tostring(k->current_value), "fprintd") != NULL)
//        found = true;

    if(facets->query && ((k->options & FACET_KEY_OPTION_FTS) || facets->options & FACETS_OPTION_ALL_KEYS_FTS)) {
        if(simple_pattern_matches(facets->query, buffer_tostring(k->current_value.b)))
            facets->keys_matched_by_query++;
    }

    if(k->values) {
        FACET_VALUE tk = {
            .name = buffer_tostring(k->current_value.b),
        };
        facets_string_hash(tk.name, buffer_strlen(k->current_value.b), k->current_value.hash);
        dictionary_set(k->values, k->current_value.hash, &tk, sizeof(tk));
    }
    else {
        k->key_found_in_row++;
        k->key_values_selected_in_row++;
    }
}

void facets_add_key_value(FACETS *facets, const char *key, const char *value) {
    FACET_KEY *k = facets_register_key(facets, key, 0);
    buffer_flush(k->current_value.b);
    buffer_strcat(k->current_value.b, value);
    k->current_value.updated = true;

    facets_check_value(facets, k);
}

void facets_add_key_value_length(FACETS *facets, const char *key, const char *value, size_t value_len) {
    FACET_KEY *k = facets_register_key(facets, key, 0);
    buffer_flush(k->current_value.b);
    buffer_strncat(k->current_value.b, value, value_len);
    k->current_value.updated = true;

    facets_check_value(facets, k);
}

// ----------------------------------------------------------------------------
// FACET_ROW dictionary hooks

static void facet_row_key_value_insert_callback(const DICTIONARY_ITEM *item __maybe_unused, void *value, void *data) {
    FACET_ROW_KEY_VALUE *rkv = value;
    FACET_ROW *row = data; (void)row;

    rkv->wb = buffer_create(0, NULL);
    buffer_strcat(rkv->wb, rkv->tmp && *rkv->tmp ? rkv->tmp : FACET_VALUE_UNSET);
}

static bool facet_row_key_value_conflict_callback(const DICTIONARY_ITEM *item __maybe_unused, void *old_value, void *new_value, void *data) {
    FACET_ROW_KEY_VALUE *rkv = old_value;
    FACET_ROW_KEY_VALUE *n_rkv = new_value;
    FACET_ROW *row = data; (void)row;

    buffer_flush(rkv->wb);
    buffer_strcat(rkv->wb, n_rkv->tmp && *n_rkv->tmp ? n_rkv->tmp : FACET_VALUE_UNSET);

    return false;
}

static void facet_row_key_value_delete_callback(const DICTIONARY_ITEM *item __maybe_unused, void *value, void *data) {
    FACET_ROW_KEY_VALUE *rkv = value;
    FACET_ROW *row = data; (void)row;

    buffer_free(rkv->wb);
}

// ----------------------------------------------------------------------------
// FACET_ROW management

static void facets_row_free(FACETS *facets __maybe_unused, FACET_ROW *row) {
    dictionary_destroy(row->dict);
    freez(row);
}

static FACET_ROW *facets_row_create(FACETS *facets, usec_t usec, FACET_ROW *into) {
    FACET_ROW *row;

    if(into)
        row = into;
    else {
        row = callocz(1, sizeof(FACET_ROW));
        row->dict = dictionary_create_advanced(DICT_OPTION_SINGLE_THREADED|DICT_OPTION_DONT_OVERWRITE_VALUE|DICT_OPTION_FIXED_SIZE, NULL, sizeof(FACET_ROW_KEY_VALUE));
        dictionary_register_insert_callback(row->dict, facet_row_key_value_insert_callback, row);
        dictionary_register_conflict_callback(row->dict, facet_row_key_value_conflict_callback, row);
        dictionary_register_delete_callback(row->dict, facet_row_key_value_delete_callback, row);
    }

    row->usec = usec;

    FACET_KEY *k;
    dfe_start_read(facets->keys, k) {
        FACET_ROW_KEY_VALUE t = {
                .tmp = (k->current_value.updated && buffer_strlen(k->current_value.b)) ?
                        buffer_tostring(k->current_value.b) : FACET_VALUE_UNSET,
                .wb = NULL,
        };
        dictionary_set(row->dict, k->name, &t, sizeof(t));
    }
    dfe_done(k);

    return row;
}

// ----------------------------------------------------------------------------

static void facets_row_keep(FACETS *facets, usec_t usec) {
    facets->operations.matched++;

    if(usec < facets->anchor) {
        facets->operations.skips_before++;
        return;
    }

    if(unlikely(!facets->base)) {
        facets->operations.last_added = facets_row_create(facets, usec, NULL);
        DOUBLE_LINKED_LIST_APPEND_ITEM_UNSAFE(facets->base, facets->operations.last_added, prev, next);
        facets->items_to_return++;
        facets->operations.first++;
        return;
    }

    if(likely(usec > facets->base->prev->usec))
        facets->operations.last_added = facets->base->prev;

    FACET_ROW *last = facets->operations.last_added;
    while(last->prev != facets->base->prev && usec > last->prev->usec) {
        last = last->prev;
        facets->operations.backwards++;
    }

    while(last->next && usec < last->next->usec) {
        last = last->next;
        facets->operations.forwards++;
    }

    if(facets->items_to_return >= facets->max_items_to_return) {
        if(last == facets->base->prev && usec < last->usec) {
            facets->operations.skips_after++;
            return;
        }
    }

    facets->items_to_return++;

    if(usec > last->usec) {
        if(facets->items_to_return > facets->max_items_to_return) {
            facets->items_to_return--;
            facets->operations.shifts++;
            facets->operations.last_added = facets->base->prev;
            DOUBLE_LINKED_LIST_REMOVE_ITEM_UNSAFE(facets->base, facets->operations.last_added, prev, next);
            facets->operations.last_added = facets_row_create(facets, usec, facets->operations.last_added);
        }
        DOUBLE_LINKED_LIST_PREPEND_ITEM_UNSAFE(facets->base, facets->operations.last_added, prev, next);
        facets->operations.prepends++;
    }
    else {
        facets->operations.last_added = facets_row_create(facets, usec, NULL);
        DOUBLE_LINKED_LIST_APPEND_ITEM_UNSAFE(facets->base, facets->operations.last_added, prev, next);
        facets->operations.appends++;
    }

    while(facets->items_to_return > facets->max_items_to_return) {
        // we have to remove something

        FACET_ROW *tmp = facets->base->prev;
        DOUBLE_LINKED_LIST_REMOVE_ITEM_UNSAFE(facets->base, tmp, prev, next);
        facets->items_to_return--;

        if(unlikely(facets->operations.last_added == tmp))
            facets->operations.last_added = facets->base->prev;

        facets_row_free(facets, tmp);
        facets->operations.shifts++;
    }
}

void facets_rows_begin(FACETS *facets) {
    FACET_KEY *k;
    // dfe_start_read(facets->keys, k) {
    for(k = facets->keys_ll ; k ; k = k->next) {
        k->key_found_in_row = 0;
        k->key_values_selected_in_row = 0;
        k->current_value.updated = false;
        k->current_value.hash[0] = '\0';
    }
    // dfe_done(k);

    facets->keys_matched_by_query = 0;
}

void facets_row_finished(FACETS *facets, usec_t usec) {
    if(facets->query && facets->keys_filtered_by_query && !facets->keys_matched_by_query)
        goto cleanup;

    facets->operations.evaluated++;

    uint32_t total_keys = 0;
    uint32_t selected_by = 0;

    FACET_KEY *k;
    // dfe_start_read(facets->keys, k) {
    for(k = facets->keys_ll ; k ; k = k->next) {
        if(!k->key_found_in_row) {
            // put the FACET_VALUE_UNSET value into it
            facets_check_value(facets, k);
        }

        internal_fatal(!k->key_found_in_row, "all keys should be found in the row at this point");
        internal_fatal(k->key_found_in_row != 1, "all keys should be matched exactly once at this point");
        internal_fatal(k->key_values_selected_in_row > 1, "key values are selected in row more than once");

        k->key_found_in_row = 1;

        total_keys += k->key_found_in_row;
        selected_by += (k->key_values_selected_in_row) ? 1 : 0;
    }
    // dfe_done(k);

    if(selected_by >= total_keys - 1) {
        uint32_t found = 0;

        // dfe_start_read(facets->keys, k){
        for(k = facets->keys_ll ; k ; k = k->next) {
            uint32_t counted_by = selected_by;

            if (counted_by != total_keys && !k->key_values_selected_in_row)
                counted_by++;

            if(counted_by == total_keys) {
                if(k->values) {
                    if(!k->current_value.hash[0])
                        facets_string_hash(buffer_tostring(k->current_value.b), buffer_strlen(k->current_value.b), k->current_value.hash);

                    FACET_VALUE *v = dictionary_get(k->values, k->current_value.hash);
                    v->final_facet_value_counter++;

                    if(selected_by == total_keys)
                        facets_histogram_update_value(facets, k, v, usec);
                }

                found++;
            }
        }
        // dfe_done(k);

        internal_fatal(!found, "We should find at least one facet to count this row");
        (void)found;
    }

    if(selected_by == total_keys)
        facets_row_keep(facets, usec);

cleanup:
    facets_rows_begin(facets);
}

// ----------------------------------------------------------------------------
// output

void facets_report(FACETS *facets, BUFFER *wb) {
    buffer_json_member_add_boolean(wb, "show_ids", false);
    buffer_json_member_add_boolean(wb, "has_history", true);

    buffer_json_member_add_object(wb, "pagination");
    buffer_json_member_add_boolean(wb, "enabled", true);
    buffer_json_member_add_string(wb, "key", "anchor");
    buffer_json_member_add_string(wb, "column", "timestamp");
    buffer_json_object_close(wb);

    buffer_json_member_add_array(wb, "accepted_params");
    {
        if(facets->accepted_params) {
            void *t;
            dfe_start_read(facets->accepted_params, t) {
                buffer_json_add_array_item_string(wb, t_dfe.name);
            }
            dfe_done(t);
        }

        FACET_KEY *k;
        dfe_start_read(facets->keys, k) {
            if(!k->values)
                continue;

            buffer_json_add_array_item_string(wb, k_dfe.name);
        }
        dfe_done(k);
    }
    buffer_json_array_close(wb); // accepted_params

    buffer_json_member_add_array(wb, "facets");
    {
        FACET_KEY *k;
        dfe_start_read(facets->keys, k) {
            if(!k->values)
                continue;

            buffer_json_add_array_item_object(wb); // key
            {
                buffer_json_member_add_string(wb, "id", k_dfe.name);
                buffer_json_member_add_string(wb, "name", k->name);

                if(!k->order)
                    k->order = facets->order++;

                buffer_json_member_add_uint64(wb, "order", k->order);
                buffer_json_member_add_array(wb, "options");
                {
                    FACET_VALUE *v;
                    dfe_start_read(k->values, v) {
                        buffer_json_add_array_item_object(wb);
                        {
                            buffer_json_member_add_string(wb, "id", v_dfe.name);
                            buffer_json_member_add_string(wb, "name", v->name);
                            buffer_json_member_add_uint64(wb, "count", v->final_facet_value_counter);
                        }
                        buffer_json_object_close(wb);
                    }
                    dfe_done(v);
                }
                buffer_json_array_close(wb); // options
            }
            buffer_json_object_close(wb); // key
        }
        dfe_done(k);
    }
    buffer_json_array_close(wb); // facets

    buffer_json_member_add_object(wb, "columns");
    {
        size_t field_id = 0;
        buffer_rrdf_table_add_field(
                wb, field_id++,
                "timestamp", "Timestamp",
                RRDF_FIELD_TYPE_TIMESTAMP,
                RRDF_FIELD_VISUAL_VALUE,
                RRDF_FIELD_TRANSFORM_DATETIME_USEC, 0, NULL, NAN,
                RRDF_FIELD_SORT_DESCENDING,
                NULL,
                RRDF_FIELD_SUMMARY_COUNT,
                RRDF_FIELD_FILTER_RANGE,
                RRDF_FIELD_OPTS_VISIBLE | RRDF_FIELD_OPTS_UNIQUE_KEY,
                NULL);

        FACET_KEY *k;
        dfe_start_read(facets->keys, k) {
            RRDF_FIELD_OPTIONS options = RRDF_FIELD_OPTS_NONE;
            bool visible = k->options & (FACET_KEY_OPTION_VISIBLE|FACET_KEY_OPTION_STICKY);

            if((facets->options & FACETS_OPTION_ALL_FACETS_VISIBLE && k->values))
                visible = true;

            if(!visible)
                visible = simple_pattern_matches(facets->visible_keys, k->name);

            if(visible)
                options |= RRDF_FIELD_OPTS_VISIBLE;

            if(k->options & FACET_KEY_OPTION_MAIN_TEXT)
                options |= RRDF_FIELD_OPTS_FULL_WIDTH | RRDF_FIELD_OPTS_WRAP;

            buffer_rrdf_table_add_field(
                    wb, field_id++,
                    k_dfe.name, k->name ? k->name : k_dfe.name,
                    RRDF_FIELD_TYPE_STRING,
                    RRDF_FIELD_VISUAL_VALUE,
                    RRDF_FIELD_TRANSFORM_NONE, 0, NULL, NAN,
                    RRDF_FIELD_SORT_ASCENDING,
                    NULL,
                    RRDF_FIELD_SUMMARY_COUNT,
                    k->values ? RRDF_FIELD_FILTER_FACET : RRDF_FIELD_FILTER_NONE,
                    options,
                    FACET_VALUE_UNSET);
        }
        dfe_done(k);
    }
    buffer_json_object_close(wb); // columns

    buffer_json_member_add_array(wb, "data");
    {
        for(FACET_ROW *row = facets->base ; row ;row = row->next) {
            buffer_json_add_array_item_array(wb); // each row
            buffer_json_add_array_item_uint64(wb, row->usec);

            FACET_KEY *k;
            dfe_start_read(facets->keys, k)
            {
                FACET_ROW_KEY_VALUE *rkv = dictionary_get(row->dict, k->name);

                if(unlikely(k->dynamic.cb)) {
                    if(unlikely(!rkv))
                        rkv = dictionary_set(row->dict, k->name, NULL, sizeof(*rkv));

                    k->dynamic.cb(facets, wb, rkv, row, k->dynamic.data);
                }
                else
                	buffer_json_add_array_item_string(wb, rkv ? buffer_tostring(rkv->wb) : FACET_VALUE_UNSET);
            }
            dfe_done(k);
            buffer_json_array_close(wb); // each row
        }
    }
    buffer_json_array_close(wb); // data

    buffer_json_member_add_string(wb, "default_sort_column", "timestamp");
    buffer_json_member_add_array(wb, "default_charts");
    buffer_json_array_close(wb);

    if(facets->histogram.enabled) {
        const char *first_histogram = NULL;
        buffer_json_member_add_array(wb, "available_histograms");
        {
            FACET_KEY *k;
            dfe_start_read(facets->keys, k) {
                if (!k->values)
                    continue;

                if(unlikely(!first_histogram))
                    first_histogram = k_dfe.name;

                buffer_json_add_array_item_object(wb);
                buffer_json_member_add_string(wb, "id", k_dfe.name);
                buffer_json_member_add_string(wb, "name", k->name);
                buffer_json_object_close(wb);
            }
            dfe_done(k);
        }
        buffer_json_array_close(wb);

        {
            const char *id = facets->histogram.chart;
            FACET_KEY *k = dictionary_get(facets->keys, id);
            if(!k || !k->values) {
                id = first_histogram;
                k = dictionary_get(facets->keys, id);
            }

            if(k && k->values) {
                buffer_json_member_add_object(wb, "histogram");
                {
                    buffer_json_member_add_string(wb, "id", id);
                    buffer_json_member_add_string(wb, "name", k->name);
                    buffer_json_member_add_object(wb, "chart");
                    facets_histogram_generate(facets, k, wb);
                    buffer_json_object_close(wb);
                }
                buffer_json_object_close(wb); // histogram
            }
        }
    }

    buffer_json_member_add_object(wb, "items");
    {
        buffer_json_member_add_uint64(wb, "evaluated", facets->operations.evaluated);
        buffer_json_member_add_uint64(wb, "matched", facets->operations.matched);
        buffer_json_member_add_uint64(wb, "returned", facets->items_to_return);
        buffer_json_member_add_uint64(wb, "max_to_return", facets->max_items_to_return);
        buffer_json_member_add_uint64(wb, "before", facets->operations.skips_before);
        buffer_json_member_add_uint64(wb, "after", facets->operations.skips_after + facets->operations.shifts);
    }
    buffer_json_object_close(wb); // items

    buffer_json_member_add_object(wb, "stats");
    {
        buffer_json_member_add_uint64(wb, "first", facets->operations.first);
        buffer_json_member_add_uint64(wb, "forwards", facets->operations.forwards);
        buffer_json_member_add_uint64(wb, "backwards", facets->operations.backwards);
        buffer_json_member_add_uint64(wb, "skips_before", facets->operations.skips_before);
        buffer_json_member_add_uint64(wb, "skips_after", facets->operations.skips_after);
        buffer_json_member_add_uint64(wb, "prepends", facets->operations.prepends);
        buffer_json_member_add_uint64(wb, "appends", facets->operations.appends);
        buffer_json_member_add_uint64(wb, "shifts", facets->operations.shifts);
    }
    buffer_json_object_close(wb); // items

}
