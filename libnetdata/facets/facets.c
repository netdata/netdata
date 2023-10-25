// SPDX-License-Identifier: GPL-3.0-or-later
#include "facets.h"

#define HISTOGRAM_COLUMNS 150               // the target number of points in a histogram
#define FACETS_KEYS_WITH_VALUES_MAX 200     // the max number of keys that can be facets
#define FACETS_KEYS_IN_ROW_MAX 500          // the max number of keys in a row

#define FACETS_KEYS_HASHTABLE_ENTRIES 127
#define FACETS_VALUES_HASHTABLE_ENTRIES 31

// ----------------------------------------------------------------------------

static const char id_encoding_characters[64 + 1] = "ABCDEFGHIJKLMNOPQRSTUVWXYZ.abcdefghijklmnopqrstuvwxyz_0123456789";
static const uint8_t id_encoding_characters_reverse[256] = {
        ['A'] = 0,  ['B'] = 1,  ['C'] = 2,  ['D'] = 3,
        ['E'] = 4,  ['F'] = 5,  ['G'] = 6,  ['H'] = 7,
        ['I'] = 8,  ['J'] = 9,  ['K'] = 10, ['L'] = 11,
        ['M'] = 12, ['N'] = 13, ['O'] = 14, ['P'] = 15,
        ['Q'] = 16, ['R'] = 17, ['S'] = 18, ['T'] = 19,
        ['U'] = 20, ['V'] = 21, ['W'] = 22, ['X'] = 23,
        ['Y'] = 24, ['Z'] = 25, ['.'] = 26, ['a'] = 27,
        ['b'] = 28, ['c'] = 29, ['d'] = 30, ['e'] = 31,
        ['f'] = 32, ['g'] = 33, ['h'] = 34, ['i'] = 35,
        ['j'] = 36, ['k'] = 37, ['l'] = 38, ['m'] = 39,
        ['n'] = 40, ['o'] = 41, ['p'] = 42, ['q'] = 43,
        ['r'] = 44, ['s'] = 45, ['t'] = 46, ['u'] = 47,
        ['v'] = 48, ['w'] = 49, ['x'] = 50, ['y'] = 51,
        ['z'] = 52, ['_'] = 53, ['0'] = 54, ['1'] = 55,
        ['2'] = 56, ['3'] = 57, ['4'] = 58, ['5'] = 59,
        ['6'] = 60, ['7'] = 61, ['8'] = 62, ['9'] = 63
};

__attribute__((constructor)) void initialize_facets_id_encoding_characters_reverse(void) {

}

#define FACET_STRING_HASH_SIZE 12
#define FACETS_HASH XXH64_hash_t
#define FACETS_HASH_FUNCTION(src, len) XXH3_64bits(src, len)
#define FACETS_HASH_ZERO (FACETS_HASH)0

static inline void facets_hash_to_str(FACETS_HASH num, char *out) {
    out[11] = '\0';
    out[10] = id_encoding_characters[num & 63]; num >>= 6;
    out[9]  = id_encoding_characters[num & 63]; num >>= 6;
    out[8]  = id_encoding_characters[num & 63]; num >>= 6;
    out[7]  = id_encoding_characters[num & 63]; num >>= 6;
    out[6]  = id_encoding_characters[num & 63]; num >>= 6;
    out[5]  = id_encoding_characters[num & 63]; num >>= 6;
    out[4]  = id_encoding_characters[num & 63]; num >>= 6;
    out[3]  = id_encoding_characters[num & 63]; num >>= 6;
    out[2]  = id_encoding_characters[num & 63]; num >>= 6;
    out[1]  = id_encoding_characters[num & 63]; num >>= 6;
    out[0]  = id_encoding_characters[num & 63];
}

static inline FACETS_HASH str_to_facets_hash(const char *str) {
    FACETS_HASH num = 0;
    int shifts = 6 * (FACET_STRING_HASH_SIZE - 2);

    num |= ((FACETS_HASH)(id_encoding_characters_reverse[(uint8_t)(str[0])])) << shifts; shifts -= 6;
    num |= ((FACETS_HASH)(id_encoding_characters_reverse[(uint8_t)(str[1])])) << shifts; shifts -= 6;
    num |= ((FACETS_HASH)(id_encoding_characters_reverse[(uint8_t)(str[2])])) << shifts; shifts -= 6;
    num |= ((FACETS_HASH)(id_encoding_characters_reverse[(uint8_t)(str[3])])) << shifts; shifts -= 6;
    num |= ((FACETS_HASH)(id_encoding_characters_reverse[(uint8_t)(str[4])])) << shifts; shifts -= 6;
    num |= ((FACETS_HASH)(id_encoding_characters_reverse[(uint8_t)(str[5])])) << shifts; shifts -= 6;
    num |= ((FACETS_HASH)(id_encoding_characters_reverse[(uint8_t)(str[6])])) << shifts; shifts -= 6;
    num |= ((FACETS_HASH)(id_encoding_characters_reverse[(uint8_t)(str[7])])) << shifts; shifts -= 6;
    num |= ((FACETS_HASH)(id_encoding_characters_reverse[(uint8_t)(str[8])])) << shifts; shifts -= 6;
    num |= ((FACETS_HASH)(id_encoding_characters_reverse[(uint8_t)(str[9])])) << shifts; shifts -= 6;
    num |= ((FACETS_HASH)(id_encoding_characters_reverse[(uint8_t)(str[10])])) << shifts;

    return num;
}

static const char *hash_to_static_string(FACETS_HASH hash) {
    static __thread char hash_str[FACET_STRING_HASH_SIZE];
    facets_hash_to_str(hash, hash_str);
    return hash_str;
}

static inline bool is_valid_string_hash(const char *s) {
    if(strlen(s) != FACET_STRING_HASH_SIZE - 1) {
        netdata_log_error("The user supplied key '%s' does not have the right length for a facets hash.", s);
        return false;
    }

    uint8_t *t = (uint8_t *)s;
    while(*t) {
        if(id_encoding_characters_reverse[*t] == 0 && *t != id_encoding_characters[0]) {
            netdata_log_error("The user supplied key '%s' contains invalid characters for a facets hash.", s);
            return false;
        }

        t++;
    }

    return true;
}

// ----------------------------------------------------------------------------

typedef uint64_t SIMPLE_HASHTABLE_HASH;
#define SIMPLE_HASHTABLE_HASH_SECOND_HASH_SHIFTS 32

typedef struct simple_hashtable_slot {
    SIMPLE_HASHTABLE_HASH hash;
    void *data;
} SIMPLE_HASHTABLE_SLOT;

typedef struct simple_hashtable {
    size_t size;
    SIMPLE_HASHTABLE_SLOT *hashtable;
} SIMPLE_HASHTABLE;

static void simple_hashtable_init(SIMPLE_HASHTABLE *ht, size_t size) {
    ht->size = size;
    ht->hashtable = callocz(ht->size, sizeof(*ht->hashtable));
}

static void simple_hashtable_free(SIMPLE_HASHTABLE *ht) {
    freez(ht->hashtable);
    ht->hashtable = NULL;
    ht->size = 0;
}

static inline SIMPLE_HASHTABLE_SLOT *simple_hashtable_get_slot(SIMPLE_HASHTABLE *ht, SIMPLE_HASHTABLE_HASH hash) {
    // IMPORTANT:
    // If the hashtable supported deletions, we would need to have a special slot.data value
    // to mark deleted values and assume they are occupied during lookup, but empty during insert.
    // But for our case, we don't need it, since we never delete items from the hashtable.

    size_t slot = hash % ht->size;
    if(!ht->hashtable[slot].data || ht->hashtable[slot].hash == hash)
        return &ht->hashtable[slot];

    slot = ((hash >> SIMPLE_HASHTABLE_HASH_SECOND_HASH_SHIFTS) + 1) % ht->size;
    // Linear probing until we find it
    while (ht->hashtable[slot].data && ht->hashtable[slot].hash != hash)
        slot = (slot + 1) % ht->size;  // Wrap around if necessary

    return &ht->hashtable[slot];
}

static void simple_hashtable_resize_double(SIMPLE_HASHTABLE *ht) {
    SIMPLE_HASHTABLE_SLOT *old = ht->hashtable;
    size_t old_size = ht->size;

    ht->size = (ht->size * 2) + 1;
    ht->hashtable = callocz(ht->size, sizeof(*ht->hashtable));
    for(size_t i = 0 ; i < old_size ; i++) {
        if(!old[i].data)
            continue;

        SIMPLE_HASHTABLE_SLOT *slot = simple_hashtable_get_slot(ht, old[i].hash);
        *slot = old[i];
    }

    freez(old);
}


// ----------------------------------------------------------------------------

typedef struct facet_value {
    FACETS_HASH hash;
    const char *name;
    uint32_t name_len;

    bool selected;
    bool empty;

    uint32_t rows_matching_facet_value;
    uint32_t final_facet_value_counter;
    uint32_t order;

    uint32_t *histogram;
    uint32_t min, max, sum;

    struct facet_value *prev, *next;
} FACET_VALUE;

typedef enum {
    FACET_KEY_VALUE_NONE    = 0,
    FACET_KEY_VALUE_UPDATED = (1 << 0),
    FACET_KEY_VALUE_EMPTY   = (1 << 1),
    FACET_KEY_VALUE_COPIED  = (1 << 2),
} FACET_KEY_VALUE_FLAGS;

#define facet_key_value_updated(k) ((k)->current_value.flags & FACET_KEY_VALUE_UPDATED)
#define facet_key_value_empty(k) ((k)->current_value.flags & FACET_KEY_VALUE_EMPTY)
#define facet_key_value_copied(k) ((k)->current_value.flags & FACET_KEY_VALUE_COPIED)

struct facet_key {
    FACETS *facets;

    FACETS_HASH hash;
    const char *name;

    FACET_KEY_OPTIONS options;

    bool default_selected_for_values; // the default "selected" for all values in the dictionary

    // members about the current row
    uint32_t key_found_in_row;
    uint32_t key_values_selected_in_row;
    uint32_t order;

    struct {
        bool enabled;
        uint32_t used;
        FACET_VALUE *ll;
        SIMPLE_HASHTABLE ht;
    } values;

    struct {
        FACETS_HASH hash;
        FACET_KEY_VALUE_FLAGS flags;
        const char *raw;
        uint32_t raw_len;
        BUFFER *b;
        FACET_VALUE *v;
    } current_value;

    struct {
        FACET_VALUE *v;
    } empty_value;

    struct {
        facet_dynamic_row_t cb;
        void *data;
    } dynamic;

    struct {
        bool view_only;
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

    struct {
        usec_t start_ut;
        usec_t stop_ut;
        FACETS_ANCHOR_DIRECTION direction;
    } anchor;

    SIMPLE_PATTERN *query;          // the full text search pattern
    size_t keys_filtered_by_query;  // the number of fields we do full text search (constant)

    DICTIONARY *accepted_params;

    struct {
        size_t count;
        FACET_KEY *ll;
        SIMPLE_HASHTABLE ht;
    } keys;

    struct {
        // this is like a stack, of the keys that are used as facets
        size_t used;
        FACET_KEY *array[FACETS_KEYS_WITH_VALUES_MAX];
    } keys_with_values;

    struct {
        // this is like a stack, of the keys that need to clean up between each row
        size_t used;
        FACET_KEY *array[FACETS_KEYS_IN_ROW_MAX];
    } keys_in_row;

    FACET_ROW *base;    // double linked list of the selected facets rows

    uint32_t items_to_return;
    uint32_t max_items_to_return;
    uint32_t order;

    struct {
        FACET_ROW_SEVERITY severity;
        size_t keys_matched_by_query_positive;   // the number of fields matched the full text search (per row)
        size_t keys_matched_by_query_negative;   // the number of fields matched the full text search (per row)
    } current_row;

    struct {
        usec_t after_ut;
        usec_t before_ut;
    } timeframe;

    struct {
        FACET_KEY *key;
        FACETS_HASH hash;
        char *chart;
        bool enabled;
        uint32_t slots;
        usec_t slot_width_ut;
        usec_t after_ut;
        usec_t before_ut;
    } histogram;

    struct {
        facet_row_severity_t cb;
        void *data;
    } severity;

    struct {
        FACET_ROW *last_added;

        size_t first;
        size_t forwards;
        size_t backwards;
        size_t skips_before;
        size_t skips_after;
        size_t prepends;
        size_t appends;
        size_t shifts;

        struct {
            size_t evaluated;
            size_t matched;
            size_t created;
            size_t reused;
        } rows;

        struct {
            size_t registered;
            size_t unique;
            size_t hashtable_increases;
        } keys;

        struct {
            size_t registered;
            size_t transformed;
            size_t dynamic;
            size_t empty;
            size_t indexed;
            size_t inserts;
            size_t conflicts;
            size_t hashtable_increases;
        } values;

        struct {
            size_t searches;
        } fts;
    } operations;
};

usec_t facets_row_oldest_ut(FACETS *facets) {
    if(facets->base)
        return facets->base->prev->usec;

    return 0;
}

usec_t facets_row_newest_ut(FACETS *facets) {
    if(facets->base)
        return facets->base->usec;

    return 0;
}

uint32_t facets_rows(FACETS *facets) {
    return facets->items_to_return;
}

// ----------------------------------------------------------------------------

static void facets_row_free(FACETS *facets __maybe_unused, FACET_ROW *row);
static inline void facet_value_is_used(FACET_KEY *k, FACET_VALUE *v);
static inline bool facets_key_is_facet(FACETS *facets, FACET_KEY *k);

// ----------------------------------------------------------------------------
// The FACET_VALUE index within each FACET_KEY

#define foreach_value_in_key(k, v) \
    for((v) = (k)->values.ll; (v) ;(v) = (v)->next)

#define foreach_value_in_key_done(v) do { ; } while(0)

static inline void FACETS_VALUES_INDEX_CREATE(FACET_KEY *k) {
    k->values.ll = NULL;
    k->values.used = 0;
    simple_hashtable_init(&k->values.ht, FACETS_VALUES_HASHTABLE_ENTRIES);
}

static inline void FACETS_VALUES_INDEX_DESTROY(FACET_KEY *k) {
    FACET_VALUE *v = k->values.ll;
    while(v) {
        FACET_VALUE *next = v->next;
        freez(v->histogram);
        freez((void *)v->name);
        freez(v);
        v = next;
    }
    k->values.ll = NULL;
    k->values.used = 0;
    k->values.enabled = false;

    simple_hashtable_free(&k->values.ht);
}

static inline const char *facets_key_get_value(FACET_KEY *k) {
    return facet_key_value_copied(k) ? buffer_tostring(k->current_value.b) : k->current_value.raw;
}

static inline uint32_t facets_key_get_value_length(FACET_KEY *k) {
    return facet_key_value_copied(k) ? buffer_strlen(k->current_value.b) : k->current_value.raw_len;
}

static inline void facets_key_value_copy_to_buffer(FACET_KEY *k) {
    if(!facet_key_value_copied(k)) {
        buffer_contents_replace(k->current_value.b, k->current_value.raw, k->current_value.raw_len);
        k->current_value.flags |= FACET_KEY_VALUE_COPIED;
    }
}

static const char *facets_value_dup(const char *s, uint32_t len) {
    char *d = mallocz(len + 1);

    if(len)
        memcpy(d, s, len);

    d[len] = '\0';

    return d;
}

static inline void FACET_VALUE_ADD_CONFLICT(FACET_KEY *k, FACET_VALUE *v, const FACET_VALUE * const nv) {
    if(!v->name && !v->name_len && nv->name && nv->name_len) {
        // an actual value, not a filter
        v->name = facets_value_dup(nv->name, nv->name_len);
        v->name_len = nv->name_len;
    }

    if(v->name && v->name_len)
        facet_value_is_used(k, v);

    internal_fatal(v->name && nv->name && v->name_len == nv->name_len && memcmp(v->name, nv->name, v->name_len) != 0,
                   "value hash conflict: '%s' and '%s' have the same hash '%s'",
                   v->name, nv->name, hash_to_static_string(v->hash));

    k->facets->operations.values.conflicts++;
}

static inline FACET_VALUE *FACET_VALUE_GET_FROM_INDEX(FACET_KEY *k, FACETS_HASH hash) {
    SIMPLE_HASHTABLE_SLOT *slot = simple_hashtable_get_slot(&k->values.ht, hash);
    return slot->data;
}

static inline FACET_VALUE *FACET_VALUE_ADD_TO_INDEX(FACET_KEY *k, const FACET_VALUE * const tv) {
    SIMPLE_HASHTABLE_SLOT *slot = simple_hashtable_get_slot(&k->values.ht, tv->hash);

    if(slot->data) {
        // already exists

        FACET_VALUE *v = slot->data;
        FACET_VALUE_ADD_CONFLICT(k, v, tv);
        return v;
    }

    // we have to add it

    FACET_VALUE *v = mallocz(sizeof(*v));
    slot->hash = tv->hash;
    slot->data = v;

    memcpy(v, tv, sizeof(*v));

    DOUBLE_LINKED_LIST_APPEND_ITEM_UNSAFE(k->values.ll, v, prev, next);
    k->values.used++;

    if(!v->selected)
        v->selected = k->default_selected_for_values;

    if(v->name && v->name_len) {
        // an actual value, not a filter
        v->name = facets_value_dup(v->name, v->name_len);
        facet_value_is_used(k, v);
    }
    else {
        v->name = NULL;
        v->name_len = 0;
    }

    k->facets->operations.values.inserts++;

    if(unlikely(k->values.used > k->values.ht.size / 2)) {
        simple_hashtable_resize_double(&k->values.ht);
        k->facets->operations.values.hashtable_increases++;
    }

    return v;
}

static inline void FACET_VALUE_ADD_EMPTY_VALUE_TO_INDEX(FACET_KEY *k) {
    static const FACET_VALUE tv = {
            .hash = FACETS_HASH_ZERO,
            .name = FACET_VALUE_UNSET,
            .name_len = sizeof(FACET_VALUE_UNSET) - 1,
    };

    k->current_value.hash = FACETS_HASH_ZERO;

    if(k->empty_value.v) {
        FACET_VALUE_ADD_CONFLICT(k, k->empty_value.v, &tv);
        k->current_value.v = k->empty_value.v;
    }
    else {
        FACET_VALUE *v = FACET_VALUE_ADD_TO_INDEX(k, &tv);
        v->empty = true;
        k->empty_value.v = v;
        k->current_value.v = v;
    }
}

static inline void FACET_VALUE_ADD_CURRENT_VALUE_TO_INDEX(FACET_KEY *k) {
    static __thread FACET_VALUE tv = { 0 };

    internal_fatal(!facet_key_value_updated(k), "trying to add a non-updated value to the index");

    tv.name = facets_key_get_value(k);
    tv.name_len = facets_key_get_value_length(k);
    tv.hash = FACETS_HASH_FUNCTION(tv.name, tv.name_len);

    k->current_value.v = FACET_VALUE_ADD_TO_INDEX(k, &tv);
    k->facets->operations.values.indexed++;
}

static inline void FACET_VALUE_ADD_OR_UPDATE_SELECTED(FACET_KEY *k, FACETS_HASH hash) {
    FACET_VALUE tv = {
            .hash = hash,
            .selected = true,
            .name = NULL,
            .name_len = 0,
    };
    FACET_VALUE_ADD_TO_INDEX(k, &tv);
}

// ----------------------------------------------------------------------------
// The FACET_KEY index within each FACET

#define foreach_key_in_facets(facets, k) \
    for((k) = (facets)->keys.ll; (k) ;(k) = (k)->next)

#define foreach_key_in_facets_done(k) do { ; } while(0)

static inline void facet_key_late_init(FACETS *facets, FACET_KEY *k) {
    if(k->values.enabled)
        return;

    if(facets_key_is_facet(facets, k)) {
        FACETS_VALUES_INDEX_CREATE(k);
        k->values.enabled = true;
        if(facets->keys_with_values.used < FACETS_KEYS_WITH_VALUES_MAX)
            facets->keys_with_values.array[facets->keys_with_values.used++] = k;
    }
}

static inline void FACETS_KEYS_INDEX_CREATE(FACETS *facets) {
    facets->keys.ll = NULL;
    facets->keys.count = 0;
    facets->keys_with_values.used = 0;

    simple_hashtable_init(&facets->keys.ht, FACETS_KEYS_HASHTABLE_ENTRIES);
}

static inline void FACETS_KEYS_INDEX_DESTROY(FACETS *facets) {
    FACET_KEY *k = facets->keys.ll;
    while(k) {
        FACET_KEY *next = k->next;

        FACETS_VALUES_INDEX_DESTROY(k);
        buffer_free(k->current_value.b);
        freez((void *)k->name);
        freez(k);

        k = next;
    }
    facets->keys.ll = NULL;
    facets->keys.count = 0;
    facets->keys_with_values.used = 0;

    simple_hashtable_free(&facets->keys.ht);
}

static inline FACET_KEY *FACETS_KEY_GET_FROM_INDEX(FACETS *facets, FACETS_HASH hash) {
    SIMPLE_HASHTABLE_SLOT *slot = simple_hashtable_get_slot(&facets->keys.ht, hash);
    return slot->data;
}

bool facets_key_name_value_length_is_selected(FACETS *facets, const char *key, size_t key_length, const char *value, size_t value_length) {
    FACETS_HASH hash = FACETS_HASH_FUNCTION(key, key_length);
    FACET_KEY *k = FACETS_KEY_GET_FROM_INDEX(facets, hash);
    if(!k || k->default_selected_for_values)
        return false;

    hash = FACETS_HASH_FUNCTION(value, value_length);
    FACET_VALUE *v = FACET_VALUE_GET_FROM_INDEX(k, hash);
    return (v && v->selected) ? true : false;
}

void facets_add_possible_value_name_to_key(FACETS *facets, const char *key, size_t key_length, const char *value, size_t value_length) {
    FACETS_HASH hash = FACETS_HASH_FUNCTION(key, key_length);
    FACET_KEY *k = FACETS_KEY_GET_FROM_INDEX(facets, hash);
    if(!k) return;

    hash = FACETS_HASH_FUNCTION(value, value_length);
    FACET_VALUE *v = FACET_VALUE_GET_FROM_INDEX(k, hash);
    if(v && v->name && v->name_len) return;

    FACET_VALUE tv = {
            .hash = hash,
            .name = value,
            .name_len = value_length,
    };
    FACET_VALUE_ADD_TO_INDEX(k, &tv);
}

static void facet_key_set_name(FACET_KEY *k, const char *name, size_t name_length) {
    internal_fatal(k->name && name && (strncmp(k->name, name, name_length) != 0 || k->name[name_length] != '\0'),
            "key hash conflict: '%s' and '%s' have the same hash",
            k->name, name);

    if(likely(k->name || !name || !name_length))
        return;

    // an actual value, not a filter

    char buf[name_length + 1];
    memcpy(buf, name, name_length);
    buf[name_length] = '\0';

    internal_fatal(strchr(buf, '='), "found = in key");

    k->name = strdupz(buf);
    facet_key_late_init(k->facets, k);
}

static inline FACET_KEY *FACETS_KEY_CREATE(FACETS *facets, FACETS_HASH hash, const char *name, size_t name_length, FACET_KEY_OPTIONS options) {
    facets->operations.keys.unique++;

    FACET_KEY *k = callocz(1, sizeof(*k));

    k->hash = hash;
    k->facets = facets;
    k->options = options;
    k->current_value.b = buffer_create(sizeof(FACET_VALUE_UNSET), NULL);
    k->default_selected_for_values = true;

    if(!(k->options & FACET_KEY_OPTION_REORDER))
        k->order = facets->order++;

    if((k->options & FACET_KEY_OPTION_FTS) || (facets->options & FACETS_OPTION_ALL_KEYS_FTS))
        facets->keys_filtered_by_query++;

    facet_key_set_name(k, name, name_length);

    DOUBLE_LINKED_LIST_APPEND_ITEM_UNSAFE(facets->keys.ll, k, prev, next);
    facets->keys.count++;

    return k;
}

static inline FACET_KEY *FACETS_KEY_ADD_TO_INDEX(FACETS *facets, FACETS_HASH hash, const char *name, size_t name_length, FACET_KEY_OPTIONS options) {
    facets->operations.keys.registered++;

    SIMPLE_HASHTABLE_SLOT *slot = simple_hashtable_get_slot(&facets->keys.ht, hash);

    if(unlikely(!slot->data)) {
        // we have to add it
        FACET_KEY *k = FACETS_KEY_CREATE(facets, hash, name, name_length, options);

        slot->hash = hash;
        slot->data = k;

        if(facets->keys.count > facets->keys.ht.size / 2) {
            simple_hashtable_resize_double(&facets->keys.ht);
            facets->operations.keys.hashtable_increases++;
        }

        return k;
    }

    // already in the index

    FACET_KEY *k = slot->data;

    facet_key_set_name(k, name, name_length);

    if(unlikely(k->options & FACET_KEY_OPTION_REORDER)) {
        k->order = facets->order++;
        k->options &= ~FACET_KEY_OPTION_REORDER;
    }

    return k;
}

bool facets_key_name_is_filter(FACETS *facets, const char *key) {
    FACETS_HASH hash = FACETS_HASH_FUNCTION(key, strlen(key));
    FACET_KEY *k = FACETS_KEY_GET_FROM_INDEX(facets, hash);
    return (!k || k->default_selected_for_values) ? false : true;
}

bool facets_key_name_is_facet(FACETS *facets, const char *key) {
    size_t key_len = strlen(key);
    FACETS_HASH hash = FACETS_HASH_FUNCTION(key, key_len);
    FACET_KEY *k = FACETS_KEY_ADD_TO_INDEX(facets, hash, key, key_len, 0);
    return (k && (k->options & FACET_KEY_OPTION_FACET));
}

// ----------------------------------------------------------------------------

static usec_t calculate_histogram_bar_width(usec_t after_ut, usec_t before_ut) {
    // Array of valid durations in seconds
    static time_t valid_durations_s[] = {
            1, 2, 5, 10, 15, 30,                                                // seconds
            1 * 60, 2 * 60, 3 * 60, 5 * 60, 10 * 60, 15 * 60, 30 * 60,          // minutes
            1 * 3600, 2 * 3600, 6 * 3600, 8 * 3600, 12 * 3600,                  // hours
            1 * 86400, 2 * 86400, 3 * 86400, 5 * 86400, 7 * 86400, 14 * 86400,  // days
            1 * (30*86400)                                                      // months
    };
    static int array_size = sizeof(valid_durations_s) / sizeof(valid_durations_s[0]);

    usec_t duration_ut = before_ut - after_ut;
    usec_t bar_width_ut = 1 * USEC_PER_SEC;

    for (int i = array_size - 1; i >= 0; --i) {
        if (duration_ut / (valid_durations_s[i] * USEC_PER_SEC) >= HISTOGRAM_COLUMNS) {
            bar_width_ut = valid_durations_s[i] * USEC_PER_SEC;
            break;
        }
    }

    return bar_width_ut;
}

static inline usec_t facets_histogram_slot_baseline_ut(FACETS *facets, usec_t ut) {
    usec_t delta_ut = ut % facets->histogram.slot_width_ut;
    return ut - delta_ut;
}

void facets_set_timeframe_and_histogram_by_id(FACETS *facets, const char *key_id, usec_t after_ut, usec_t before_ut) {
    if(after_ut > before_ut) {
        usec_t t = after_ut;
        after_ut = before_ut;
        before_ut = t;
    }

    facets->histogram.enabled = true;

    if(key_id && *key_id && strlen(key_id) == FACET_STRING_HASH_SIZE - 1) {
        facets->histogram.chart = strdupz(key_id);
        facets->histogram.hash = str_to_facets_hash(facets->histogram.chart);
    }
    else {
        freez(facets->histogram.chart);
        facets->histogram.chart = NULL;
        facets->histogram.hash = FACETS_HASH_ZERO;
    }

    facets->timeframe.after_ut = after_ut;
    facets->timeframe.before_ut = before_ut;

    facets->histogram.slot_width_ut = calculate_histogram_bar_width(after_ut, before_ut);
    facets->histogram.after_ut = facets_histogram_slot_baseline_ut(facets, after_ut);
    facets->histogram.before_ut = facets_histogram_slot_baseline_ut(facets, before_ut) + facets->histogram.slot_width_ut;
    facets->histogram.slots = (facets->histogram.before_ut - facets->histogram.after_ut) / facets->histogram.slot_width_ut + 1;

    internal_fatal(after_ut < facets->histogram.after_ut, "histogram after_ut is not less or equal to wanted after_ut");
    internal_fatal(before_ut > facets->histogram.before_ut, "histogram before_ut is not more or equal to wanted before_ut");

    if(facets->histogram.slots > 1000) {
        facets->histogram.slots = 1000 + 1;
        facets->histogram.slot_width_ut = (facets->histogram.before_ut - facets->histogram.after_ut) / 1000;
    }
}

void facets_set_timeframe_and_histogram_by_name(FACETS *facets, const char *key_name, usec_t after_ut, usec_t before_ut) {
    char hash_str[FACET_STRING_HASH_SIZE];
    FACETS_HASH hash = FACETS_HASH_FUNCTION(key_name, strlen(key_name));
    facets_hash_to_str(hash, hash_str);
    facets_set_timeframe_and_histogram_by_id(facets, hash_str, after_ut, before_ut);
}

static inline void facets_histogram_update_value(FACETS *facets, usec_t usec) {
    if(!facets->histogram.enabled ||
            !facets->histogram.key ||
            !facets->histogram.key->values.enabled ||
            !facet_key_value_updated(facets->histogram.key) ||
            usec < facets->histogram.after_ut ||
            usec > facets->histogram.before_ut)
        return;

    FACET_VALUE *v = facets->histogram.key->current_value.v;

    if(unlikely(!v->histogram))
        v->histogram = callocz(facets->histogram.slots, sizeof(*v->histogram));

    usec_t base_ut = facets_histogram_slot_baseline_ut(facets, usec);

    if(unlikely(base_ut < facets->histogram.after_ut))
        base_ut = facets->histogram.after_ut;

    if(unlikely(base_ut > facets->histogram.before_ut))
        base_ut = facets->histogram.before_ut;

    uint32_t slot = (base_ut - facets->histogram.after_ut) / facets->histogram.slot_width_ut;

    if(unlikely(slot >= facets->histogram.slots))
        slot = facets->histogram.slots - 1;

    v->histogram[slot]++;
}

static inline void facets_histogram_value_names(BUFFER *wb, FACETS *facets __maybe_unused, FACET_KEY *k, const char *key, const char *first_key) {
    BUFFER *tb = NULL;

    buffer_json_member_add_array(wb, key);
    {
        if(first_key)
            buffer_json_add_array_item_string(wb, first_key);

        if(k && k->values.enabled) {
            FACET_VALUE *v;
            foreach_value_in_key(k, v) {
                if (unlikely(!v->histogram))
                    continue;

                if(!v->empty && k->transform.cb && k->transform.view_only) {
                    if(!tb)
                        tb = buffer_create(0, NULL);

                    buffer_contents_replace(tb, v->name, v->name_len);
                    k->transform.cb(facets, tb, FACETS_TRANSFORM_HISTOGRAM, k->transform.data);
                    buffer_json_add_array_item_string(wb, buffer_tostring(tb));
                }
                else
                    buffer_json_add_array_item_string(wb, v->name);
            }
            foreach_value_in_key_done(v);
        }
    }
    buffer_json_array_close(wb); // key

    buffer_free(tb);
}

static inline void facets_histogram_value_units(BUFFER *wb, FACETS *facets __maybe_unused, FACET_KEY *k, const char *key) {
    buffer_json_member_add_array(wb, key);
    {
        if(k && k->values.enabled) {
            FACET_VALUE *v;
            foreach_value_in_key(k, v) {
                if (unlikely(!v->histogram))
                    continue;

                buffer_json_add_array_item_string(wb, "events");
            }
            foreach_value_in_key_done(v);
        }
    }
    buffer_json_array_close(wb); // key
}

static inline void facets_histogram_value_min(BUFFER *wb, FACETS *facets __maybe_unused, FACET_KEY *k, const char *key) {
    buffer_json_member_add_array(wb, key);
    {
        if(k && k->values.enabled) {
            FACET_VALUE *v;
            foreach_value_in_key(k, v) {
                if (unlikely(!v->histogram))
                    continue;

                buffer_json_add_array_item_uint64(wb, v->min);
            }
            foreach_value_in_key_done(v);
        }
    }
    buffer_json_array_close(wb); // key
}

static inline void facets_histogram_value_max(BUFFER *wb, FACETS *facets __maybe_unused, FACET_KEY *k, const char *key) {
    buffer_json_member_add_array(wb, key);
    {
        if(k && k->values.enabled) {
            FACET_VALUE *v;
            foreach_value_in_key(k, v) {
                if (unlikely(!v->histogram))
                    continue;

                buffer_json_add_array_item_uint64(wb, v->max);
            }
            foreach_value_in_key_done(v);
        }
    }
    buffer_json_array_close(wb); // key
}

static inline void facets_histogram_value_avg(BUFFER *wb, FACETS *facets __maybe_unused, FACET_KEY *k, const char *key) {
    buffer_json_member_add_array(wb, key);
    {
        if(k && k->values.enabled) {
            FACET_VALUE *v;
            foreach_value_in_key(k, v) {
                if (unlikely(!v->histogram))
                    continue;

                buffer_json_add_array_item_double(wb, (double) v->sum / (double) facets->histogram.slots);
            }
            foreach_value_in_key_done(v);
        }
    }
    buffer_json_array_close(wb); // key
}

static inline void facets_histogram_value_arp(BUFFER *wb, FACETS *facets __maybe_unused, FACET_KEY *k, const char *key) {
    buffer_json_member_add_array(wb, key);
    {
        if(k && k->values.enabled) {
            FACET_VALUE *v;
            foreach_value_in_key(k, v) {
                if (unlikely(!v->histogram))
                    continue;

                buffer_json_add_array_item_uint64(wb, 0);
            }
            foreach_value_in_key_done(v);
        }
    }
    buffer_json_array_close(wb); // key
}

static inline void facets_histogram_value_con(BUFFER *wb, FACETS *facets __maybe_unused, FACET_KEY *k, const char *key, uint32_t sum) {
    buffer_json_member_add_array(wb, key);
    {
        if(k && k->values.enabled) {
            FACET_VALUE *v;
            foreach_value_in_key(k, v) {
                if (unlikely(!v->histogram))
                    continue;

                buffer_json_add_array_item_double(wb, (double) v->sum * 100.0 / (double) sum);
            }
            foreach_value_in_key_done(v);
        }
    }
    buffer_json_array_close(wb); // key
}

static void facets_histogram_generate(FACETS *facets, FACET_KEY *k, BUFFER *wb) {
    size_t dimensions = 0;
    uint32_t min = UINT32_MAX, max = 0, sum = 0, count = 0;

    if(k && k->values.enabled) {
        FACET_VALUE *v;
        foreach_value_in_key(k, v) {
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
        foreach_value_in_key_done(v);
    }

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

                if(dimensions) {
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
                }

                if(count) {
                    buffer_json_member_add_object(wb, "sts");
                    {
                        buffer_json_member_add_uint64(wb, "min", min);
                        buffer_json_member_add_uint64(wb, "max", max);
                        buffer_json_member_add_double(wb, "avg", (double) sum / (double) count);
                        buffer_json_member_add_double(wb, "con", 100.0);
                    }
                    buffer_json_object_close(wb); // sts
                }
            }
            buffer_json_object_close(wb); // node
        }
        buffer_json_array_close(wb); // nodes

        buffer_json_member_add_array(wb, "contexts");
        {
            buffer_json_add_array_item_object(wb); // context
            {
                buffer_json_member_add_string(wb, "id", "facets.histogram");

                if(dimensions) {
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
                }

                if(count) {
                    buffer_json_member_add_object(wb, "sts");
                    {
                        buffer_json_member_add_uint64(wb, "min", min);
                        buffer_json_member_add_uint64(wb, "max", max);
                        buffer_json_member_add_double(wb, "avg", (double) sum / (double) count);
                        buffer_json_member_add_double(wb, "con", 100.0);
                    }
                    buffer_json_object_close(wb); // sts
                }
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

                if(dimensions) {
                    buffer_json_member_add_object(wb, "ds");
                    {
                        buffer_json_member_add_uint64(wb, "sl", dimensions);
                        buffer_json_member_add_uint64(wb, "qr", dimensions);
                    }
                    buffer_json_object_close(wb); // ds
                }

                if(count) {
                    buffer_json_member_add_object(wb, "sts");
                    {
                        buffer_json_member_add_uint64(wb, "min", min);
                        buffer_json_member_add_uint64(wb, "max", max);
                        buffer_json_member_add_double(wb, "avg", (double) sum / (double) count);
                        buffer_json_member_add_double(wb, "con", 100.0);
                    }
                    buffer_json_object_close(wb); // sts
                }
            }
            buffer_json_object_close(wb); // instance
        }
        buffer_json_array_close(wb); // instances

        buffer_json_member_add_array(wb, "dimensions");
        if(dimensions && k && k->values.enabled) {
            size_t pri = 0;
            FACET_VALUE *v;
            foreach_value_in_key(k, v) {
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
            foreach_value_in_key_done(v);
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
        buffer_json_object_close(wb); // nodes

        if(dimensions) {
            buffer_json_member_add_object(wb, "contexts");
            {
                buffer_json_member_add_uint64(wb, "sl", 1);
                buffer_json_member_add_uint64(wb, "qr", 1);
            }
            buffer_json_object_close(wb); // contexts
            buffer_json_member_add_object(wb, "instances");
            {
                buffer_json_member_add_uint64(wb, "sl", 1);
                buffer_json_member_add_uint64(wb, "qr", 1);
            }
            buffer_json_object_close(wb); // instances

            buffer_json_member_add_object(wb, "dimensions");
            {
                buffer_json_member_add_uint64(wb, "sl", dimensions);
                buffer_json_member_add_uint64(wb, "qr", dimensions);
            }
            buffer_json_object_close(wb); // dimension
        }
    }
    buffer_json_object_close(wb); // totals

    buffer_json_member_add_object(wb, "result");
    {
        facets_histogram_value_names(wb, facets, k, "labels", "time");

        buffer_json_member_add_object(wb, "point");
        {
            buffer_json_member_add_uint64(wb, "value", 0);
            buffer_json_member_add_uint64(wb, "arp", 1);
            buffer_json_member_add_uint64(wb, "pa", 2);
        }
        buffer_json_object_close(wb); // point

        buffer_json_member_add_array(wb, "data");
        if(k && k->values.enabled) {
            usec_t t = facets->histogram.after_ut;
            for(uint32_t i = 0; i < facets->histogram.slots ;i++) {
                buffer_json_add_array_item_array(wb); // row
                {
                    buffer_json_add_array_item_time_ms(wb, t / USEC_PER_SEC);

                    FACET_VALUE *v;
                    foreach_value_in_key(k, v) {
                        if (unlikely(!v->histogram))
                            continue;

                        buffer_json_add_array_item_array(wb); // point

                        buffer_json_add_array_item_uint64(wb, v->histogram[i]);
                        buffer_json_add_array_item_uint64(wb, 0); // arp - anomaly rate
                        buffer_json_add_array_item_uint64(wb, 0); // pa - point annotation

                        buffer_json_array_close(wb); // point
                    }
                    foreach_value_in_key_done(v);
                }
                buffer_json_array_close(wb); // row

                t += facets->histogram.slot_width_ut;
            }
        }
        buffer_json_array_close(wb); //data
    }
    buffer_json_object_close(wb); // result

    buffer_json_member_add_object(wb, "db");
    {
        buffer_json_member_add_uint64(wb, "tiers", 1);
        buffer_json_member_add_uint64(wb, "update_every", facets->histogram.slot_width_ut / USEC_PER_SEC);
//        we should add these only when we know the retention of the db
//        buffer_json_member_add_time_t(wb, "first_entry", facets->histogram.after_ut / USEC_PER_SEC);
//        buffer_json_member_add_time_t(wb, "last_entry", facets->histogram.before_ut / USEC_PER_SEC);
        buffer_json_member_add_string(wb, "units", "events");
        buffer_json_member_add_object(wb, "dimensions");
        {
            facets_histogram_value_names(wb, facets, k, "ids", NULL);
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
                buffer_json_member_add_time_t(wb, "update_every", facets->histogram.slot_width_ut / USEC_PER_SEC);
//                we should add these only when we know the retention of the db
//                buffer_json_member_add_time_t(wb, "first_entry", facets->histogram.after_ut / USEC_PER_SEC);
//                buffer_json_member_add_time_t(wb, "last_entry", facets->histogram.before_ut / USEC_PER_SEC);
            }
            buffer_json_object_close(wb); // tier0
        }
        buffer_json_array_close(wb); // per_tier
    }
    buffer_json_object_close(wb); // db

    buffer_json_member_add_object(wb, "view");
    {
        char title[1024 + 1] = "Events Distribution";
        FACET_KEY *kt = FACETS_KEY_GET_FROM_INDEX(facets, facets->histogram.hash);
        if(kt && kt->name)
            snprintfz(title, 1024, "Events Distribution by %s", kt->name);

        buffer_json_member_add_string(wb, "title", title);
        buffer_json_member_add_time_t(wb, "update_every", facets->histogram.slot_width_ut / USEC_PER_SEC);
        buffer_json_member_add_time_t(wb, "after", facets->histogram.after_ut / USEC_PER_SEC);
        buffer_json_member_add_time_t(wb, "before", facets->histogram.before_ut / USEC_PER_SEC);
        buffer_json_member_add_string(wb, "units", "events");
        buffer_json_member_add_string(wb, "chart_type", "stackedBar");
        buffer_json_member_add_object(wb, "dimensions");
        {
            buffer_json_member_add_array(wb, "grouped_by");
            {
                buffer_json_add_array_item_string(wb, "dimension");
            }
            buffer_json_array_close(wb); // grouped_by

            facets_histogram_value_names(wb, facets, k, "ids", NULL);
            facets_histogram_value_names(wb, facets, k, "names", NULL);
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
    bool included = true, excluded = false, never = false;

    if(k->options & (FACET_KEY_OPTION_FACET | FACET_KEY_OPTION_NO_FACET | FACET_KEY_OPTION_NEVER_FACET)) {
        if(k->options & FACET_KEY_OPTION_FACET) {
            included = true;
            excluded = false;
            never = false;
        }
        else if(k->options & (FACET_KEY_OPTION_NO_FACET | FACET_KEY_OPTION_NEVER_FACET)) {
            included = false;
            excluded = true;
            never = true;
        }
    }
    else {
        if (facets->included_keys) {
            if (!simple_pattern_matches(facets->included_keys, k->name))
                included = false;
        }

        if (facets->excluded_keys) {
            if (simple_pattern_matches(facets->excluded_keys, k->name)) {
                excluded = true;
                never = true;
            }
        }
    }

    if(included && !excluded) {
        k->options |= FACET_KEY_OPTION_FACET;
        k->options &= ~FACET_KEY_OPTION_NO_FACET;
        return true;
    }

    k->options |= FACET_KEY_OPTION_NO_FACET;
    k->options &= ~FACET_KEY_OPTION_FACET;

    if(never)
        k->options |= FACET_KEY_OPTION_NEVER_FACET;

    return false;
}

// ----------------------------------------------------------------------------

FACETS *facets_create(uint32_t items_to_return, FACETS_OPTIONS options, const char *visible_keys, const char *facet_keys, const char *non_facet_keys) {
    FACETS *facets = callocz(1, sizeof(FACETS));
    facets->options = options;
    FACETS_KEYS_INDEX_CREATE(facets);

    if(facet_keys && *facet_keys)
        facets->included_keys = simple_pattern_create(facet_keys, "|", SIMPLE_PATTERN_EXACT, true);

    if(non_facet_keys && *non_facet_keys)
        facets->excluded_keys = simple_pattern_create(non_facet_keys, "|", SIMPLE_PATTERN_EXACT, true);

    if(visible_keys && *visible_keys)
        facets->visible_keys = simple_pattern_create(visible_keys, "|", SIMPLE_PATTERN_EXACT, true);

    facets->max_items_to_return = items_to_return > 1 ? items_to_return : 2;
    facets->anchor.start_ut = 0;
    facets->anchor.stop_ut = 0;
    facets->anchor.direction = FACETS_ANCHOR_DIRECTION_BACKWARD;
    facets->order = 1;

    return facets;
}

void facets_destroy(FACETS *facets) {
    dictionary_destroy(facets->accepted_params);
    FACETS_KEYS_INDEX_DESTROY(facets);
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

static inline FACET_KEY *facets_register_key_name_length(FACETS *facets, const char *key, size_t key_length, FACET_KEY_OPTIONS options) {
    return FACETS_KEY_ADD_TO_INDEX(facets, FACETS_HASH_FUNCTION(key, key_length), key, key_length, options);
}

inline FACET_KEY *facets_register_key_name(FACETS *facets, const char *key, FACET_KEY_OPTIONS options) {
    return facets_register_key_name_length(facets, key, strlen(key), options);
}

inline FACET_KEY *facets_register_key_name_transformation(FACETS *facets, const char *key, FACET_KEY_OPTIONS options, facets_key_transformer_t cb, void *data) {
    FACET_KEY *k = facets_register_key_name(facets, key, options);
    k->transform.cb = cb;
    k->transform.data = data;
    k->transform.view_only = (options & FACET_KEY_OPTION_TRANSFORM_VIEW) ? true : false;
    return k;
}

inline FACET_KEY *facets_register_dynamic_key_name(FACETS *facets, const char *key, FACET_KEY_OPTIONS options, facet_dynamic_row_t cb, void *data) {
    FACET_KEY *k = facets_register_key_name(facets, key, options);
    k->dynamic.cb = cb;
    k->dynamic.data = data;
    return k;
}

void facets_set_query(FACETS *facets, const char *query) {
    if(!query)
        return;

    facets->query = simple_pattern_create(query, "|", SIMPLE_PATTERN_SUBSTRING, false);
}

void facets_set_items(FACETS *facets, uint32_t items) {
    facets->max_items_to_return = items > 1 ? items : 2;
}

void facets_set_anchor(FACETS *facets, usec_t start_ut, usec_t stop_ut, FACETS_ANCHOR_DIRECTION direction) {
    facets->anchor.start_ut = start_ut;
    facets->anchor.stop_ut = stop_ut;
    facets->anchor.direction = direction;

    if((facets->anchor.direction == FACETS_ANCHOR_DIRECTION_BACKWARD && facets->anchor.start_ut && facets->anchor.start_ut < facets->anchor.stop_ut) ||
            (facets->anchor.direction == FACETS_ANCHOR_DIRECTION_FORWARD && facets->anchor.stop_ut && facets->anchor.stop_ut < facets->anchor.start_ut)) {
        internal_error(true, "start and stop anchors are flipped");
        facets->anchor.start_ut = stop_ut;
        facets->anchor.stop_ut = start_ut;
    }
}

void facets_enable_slice_mode(FACETS *facets) {
    facets->options |= FACETS_OPTION_DONT_SEND_EMPTY_VALUE_FACETS | FACETS_OPTION_SORT_FACETS_ALPHABETICALLY;
}

inline FACET_KEY *facets_register_facet_id(FACETS *facets, const char *key_id, FACET_KEY_OPTIONS options) {
    if(!is_valid_string_hash(key_id))
        return NULL;

    FACETS_HASH hash = str_to_facets_hash(key_id);

    internal_error(strcmp(hash_to_static_string(hash), key_id) != 0,
                   "Regenerating the user supplied key, does not produce the same hash string");

    FACET_KEY *k = FACETS_KEY_ADD_TO_INDEX(facets, hash, NULL, 0, options);
    k->options |= FACET_KEY_OPTION_FACET;
    k->options &= ~FACET_KEY_OPTION_NO_FACET;
    facet_key_late_init(facets, k);

    return k;
}

void facets_register_facet_id_filter(FACETS *facets, const char *key_id, char *value_id, FACET_KEY_OPTIONS options) {
    FACET_KEY *k = facets_register_facet_id(facets, key_id, options);
    if(k) {
        if(is_valid_string_hash(value_id)) {
            k->default_selected_for_values = false;
            FACET_VALUE_ADD_OR_UPDATE_SELECTED(k, str_to_facets_hash(value_id));
        }
    }
}

void facets_set_current_row_severity(FACETS *facets, FACET_ROW_SEVERITY severity) {
    facets->current_row.severity = severity;
}

void facets_register_row_severity(FACETS *facets, facet_row_severity_t cb, void *data) {
    facets->severity.cb = cb;
    facets->severity.data = data;
}

void facets_set_additional_options(FACETS *facets, FACETS_OPTIONS options) {
    facets->options |= options;
}

// ----------------------------------------------------------------------------

static inline void facets_key_set_empty_value(FACETS *facets, FACET_KEY *k) {
    if(likely(!facet_key_value_updated(k) && facets->keys_in_row.used < FACETS_KEYS_IN_ROW_MAX))
        facets->keys_in_row.array[facets->keys_in_row.used++] = k;

    k->current_value.flags |= FACET_KEY_VALUE_UPDATED | FACET_KEY_VALUE_EMPTY;

    facets->operations.values.registered++;
    facets->operations.values.empty++;

    // no need to copy the UNSET value
    // empty values are exported as empty
    k->current_value.raw = NULL;
    k->current_value.raw_len = 0;
    k->current_value.b->len = 0;
    k->current_value.flags &= ~FACET_KEY_VALUE_COPIED;

    if(unlikely(k->values.enabled))
        FACET_VALUE_ADD_EMPTY_VALUE_TO_INDEX(k);
    else {
        k->key_found_in_row++;
        k->key_values_selected_in_row++;
    }
}

static inline void facets_key_check_value(FACETS *facets, FACET_KEY *k) {
    if(likely(!facet_key_value_updated(k) && facets->keys_in_row.used < FACETS_KEYS_IN_ROW_MAX))
        facets->keys_in_row.array[facets->keys_in_row.used++] = k;

    k->current_value.flags |= FACET_KEY_VALUE_UPDATED;
    k->current_value.flags &= ~FACET_KEY_VALUE_EMPTY;

    facets->operations.values.registered++;

    if(k->transform.cb && !k->transform.view_only) {
        facets->operations.values.transformed++;
        facets_key_value_copy_to_buffer(k);
        k->transform.cb(facets, k->current_value.b, FACETS_TRANSFORM_VALUE, k->transform.data);
    }

//    bool found = false;
//    if(strstr(buffer_tostring(k->current_value), "fprintd") != NULL)
//        found = true;

    if(facets->query && !facet_key_value_empty(k) && ((k->options & FACET_KEY_OPTION_FTS) || facets->options & FACETS_OPTION_ALL_KEYS_FTS)) {
        facets->operations.fts.searches++;
        facets_key_value_copy_to_buffer(k);
        switch(simple_pattern_matches_extract(facets->query, buffer_tostring(k->current_value.b), NULL, 0)) {
            case SP_MATCHED_POSITIVE:
                facets->current_row.keys_matched_by_query_positive++;
                break;

            case SP_MATCHED_NEGATIVE:
                facets->current_row.keys_matched_by_query_negative++;
                break;

            case SP_NOT_MATCHED:
                break;
        }
    }

    if(k->values.enabled)
        FACET_VALUE_ADD_CURRENT_VALUE_TO_INDEX(k);
    else {
        k->key_found_in_row++;
        k->key_values_selected_in_row++;
    }
}

void facets_add_key_value(FACETS *facets, const char *key, const char *value) {
    FACET_KEY *k = facets_register_key_name(facets, key, 0);
    k->current_value.raw = value;
    k->current_value.raw_len = strlen(value);

    facets_key_check_value(facets, k);
}

void facets_add_key_value_length(FACETS *facets, const char *key, size_t key_len, const char *value, size_t value_len) {
    FACET_KEY *k = facets_register_key_name_length(facets, key, key_len, 0);
    k->current_value.raw = value;
    k->current_value.raw_len = value_len;

    facets_key_check_value(facets, k);
}

// ----------------------------------------------------------------------------
// FACET_ROW dictionary hooks

static void facet_row_key_value_insert_callback(const DICTIONARY_ITEM *item __maybe_unused, void *value, void *data) {
    FACET_ROW_KEY_VALUE *rkv = value;
    FACET_ROW *row = data; (void)row;

    rkv->wb = buffer_create(0, NULL);
    if(!rkv->empty)
        buffer_contents_replace(rkv->wb, rkv->tmp, rkv->tmp_len);
}

static bool facet_row_key_value_conflict_callback(const DICTIONARY_ITEM *item __maybe_unused, void *old_value, void *new_value, void *data) {
    FACET_ROW_KEY_VALUE *rkv = old_value;
    FACET_ROW_KEY_VALUE *n_rkv = new_value;
    FACET_ROW *row = data; (void)row;

    rkv->empty = n_rkv->empty;

    if(!rkv->empty)
        buffer_contents_replace(rkv->wb, n_rkv->tmp, n_rkv->tmp_len);
    else
        buffer_flush(rkv->wb);

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

    if(into) {
        row = into;
        facets->operations.rows.reused++;
    }
    else {
        row = callocz(1, sizeof(FACET_ROW));
        row->dict = dictionary_create_advanced(DICT_OPTION_SINGLE_THREADED|DICT_OPTION_DONT_OVERWRITE_VALUE|DICT_OPTION_FIXED_SIZE, NULL, sizeof(FACET_ROW_KEY_VALUE));
        dictionary_register_insert_callback(row->dict, facet_row_key_value_insert_callback, row);
        dictionary_register_conflict_callback(row->dict, facet_row_key_value_conflict_callback, row);
        dictionary_register_delete_callback(row->dict, facet_row_key_value_delete_callback, row);
        facets->operations.rows.created++;
    }

    row->severity = facets->current_row.severity;
    row->usec = usec;

    FACET_KEY *k;
    foreach_key_in_facets(facets, k) {
        FACET_ROW_KEY_VALUE t = {
                .tmp = NULL,
                .tmp_len = 0,
                .wb = NULL,
                .empty = true,
        };

        if(facet_key_value_updated(k) && !facet_key_value_empty(k)) {
            t.tmp = facets_key_get_value(k);
            t.tmp_len = facets_key_get_value_length(k);
            t.empty = false;
        }

        dictionary_set(row->dict, k->name, &t, sizeof(t));
    }
    foreach_key_in_facets_done(k);

    return row;
}

// ----------------------------------------------------------------------------

static inline FACET_ROW *facets_row_keep_seek_to_position(FACETS *facets, usec_t usec) {
    if(usec < facets->base->prev->usec)
        return facets->base->prev;

    if(usec > facets->base->usec)
        return facets->base;

    FACET_ROW *last = facets->operations.last_added;
    while(last->prev != facets->base->prev && usec > last->prev->usec) {
        last = last->prev;
        facets->operations.backwards++;
    }

    while(last->next && usec < last->next->usec) {
        last = last->next;
        facets->operations.forwards++;
    }

    return last;
}

static void facets_row_keep_first_entry(FACETS *facets, usec_t usec) {
    facets->operations.last_added = facets_row_create(facets, usec, NULL);
    DOUBLE_LINKED_LIST_APPEND_ITEM_UNSAFE(facets->base, facets->operations.last_added, prev, next);
    facets->items_to_return++;
    facets->operations.first++;
}

static inline bool facets_is_entry_within_anchor(FACETS *facets, usec_t usec) {
    if(facets->anchor.start_ut || facets->anchor.stop_ut) {
        // we have an anchor key
        // we don't want to keep rows on the other side of the direction

        switch (facets->anchor.direction) {
            default:
            case FACETS_ANCHOR_DIRECTION_BACKWARD:
                // we need to keep only the smaller timestamps
                if (facets->anchor.start_ut && usec >= facets->anchor.start_ut) {
                    facets->operations.skips_before++;
                    return false;
                }
                if (facets->anchor.stop_ut && usec <= facets->anchor.stop_ut) {
                    facets->operations.skips_after++;
                    return false;
                }
                break;

            case FACETS_ANCHOR_DIRECTION_FORWARD:
                // we need to keep only the bigger timestamps
                if (facets->anchor.start_ut && usec <= facets->anchor.start_ut) {
                    facets->operations.skips_after++;
                    return false;
                }
                if (facets->anchor.stop_ut && usec >= facets->anchor.stop_ut) {
                    facets->operations.skips_before++;
                    return false;
                }
                break;
        }
    }

    return true;
}

static void facets_row_keep(FACETS *facets, usec_t usec) {
    facets->operations.rows.matched++;

    if(unlikely(!facets->base)) {
        // the first row to keep
        facets_row_keep_first_entry(facets, usec);
        return;
    }

    FACET_ROW *closest = facets_row_keep_seek_to_position(facets, usec);
    FACET_ROW *to_replace = NULL;

    if(likely(facets->items_to_return >= facets->max_items_to_return)) {
        // we have enough items to return already

        switch(facets->anchor.direction) {
            default:
            case FACETS_ANCHOR_DIRECTION_BACKWARD:
                if(closest == facets->base->prev && usec < closest->usec) {
                    // this is to the end of the list, belonging to the next page
                    facets->operations.skips_after++;
                    return;
                }

                // it seems we need to remove an item - the last one
                to_replace = facets->base->prev;
                if(closest == to_replace)
                    closest = to_replace->prev;

                break;

            case FACETS_ANCHOR_DIRECTION_FORWARD:
                if(closest == facets->base && usec > closest->usec) {
                    // this is to the beginning of the list, belonging to the next page
                    facets->operations.skips_before++;
                    return;
                }

                // it seems we need to remove an item - the first one
                to_replace = facets->base;
                if(closest == to_replace)
                    closest = to_replace->next;

                break;
        }

        facets->operations.shifts++;
        facets->items_to_return--;
        DOUBLE_LINKED_LIST_REMOVE_ITEM_UNSAFE(facets->base, to_replace, prev, next);
    }

    internal_fatal(!closest, "FACETS: closest cannot be NULL");
    internal_fatal(closest == to_replace, "FACETS: closest cannot be the same as to_replace");

    facets->operations.last_added = facets_row_create(facets, usec, to_replace);

    if(usec < closest->usec) {
        DOUBLE_LINKED_LIST_INSERT_ITEM_AFTER_UNSAFE(facets->base, closest, facets->operations.last_added, prev, next);
        facets->operations.appends++;
    }
    else {
        DOUBLE_LINKED_LIST_INSERT_ITEM_BEFORE_UNSAFE(facets->base, closest, facets->operations.last_added, prev, next);
        facets->operations.prepends++;
    }

    facets->items_to_return++;
}

static inline void facets_reset_key(FACET_KEY *k) {
    k->key_found_in_row = 0;
    k->key_values_selected_in_row = 0;
    k->current_value.flags = FACET_KEY_VALUE_NONE;
    k->current_value.hash = FACETS_HASH_ZERO;
}

static void facets_reset_keys_with_value_and_row(FACETS *facets) {
    size_t entries = facets->keys_in_row.used;

    for(size_t p = 0; p < entries ;p++) {
        FACET_KEY *k = facets->keys_in_row.array[p];
        facets_reset_key(k);
    }

    facets->current_row.severity = FACET_ROW_SEVERITY_NORMAL;
    facets->current_row.keys_matched_by_query_positive = 0;
    facets->current_row.keys_matched_by_query_negative = 0;
    facets->keys_in_row.used = 0;
}

void facets_rows_begin(FACETS *facets) {
    FACET_KEY *k;
    foreach_key_in_facets(facets, k) {
        facets_reset_key(k);
    }
    foreach_key_in_facets_done(k);

    facets->keys_in_row.used = 0;
    facets_reset_keys_with_value_and_row(facets);
}

bool facets_row_finished(FACETS *facets, usec_t usec) {
    facets->operations.rows.evaluated++;

    if(unlikely((facets->query && facets->keys_filtered_by_query &&
                (!facets->current_row.keys_matched_by_query_positive || facets->current_row.keys_matched_by_query_negative)) ||
                (facets->timeframe.before_ut && usec > facets->timeframe.before_ut) ||
                (facets->timeframe.after_ut && usec < facets->timeframe.after_ut))) {
        // this row is not useful
        // 1. not matched by full text search, or
        // 2. not in our timeframe
        facets_reset_keys_with_value_and_row(facets);
        return false;
    }

    bool within_anchor = facets_is_entry_within_anchor(facets, usec);
    if(unlikely(!within_anchor && (facets->options & FACETS_OPTION_DATA_ONLY))) {
        facets_reset_keys_with_value_and_row(facets);
        return false;
    }

    size_t entries = facets->keys_with_values.used;
    size_t total_keys = 0;
    size_t selected_keys = 0;

    for(size_t p = 0; p < entries ;p++) {
        FACET_KEY *k = facets->keys_with_values.array[p];

        if(!facet_key_value_updated(k))
            // put the FACET_VALUE_UNSET value into it
            facets_key_set_empty_value(facets, k);

        total_keys++;

        if(k->key_values_selected_in_row)
            selected_keys++;

        if(unlikely(!facets->histogram.key && facets->histogram.hash == k->hash))
            facets->histogram.key = k;
    }

    if(selected_keys >= total_keys - 1) {
        size_t found = 0;
        (void) found;

        for(size_t p = 0; p < entries; p++) {
            FACET_KEY *k = facets->keys_with_values.array[p];

            size_t counted_by = selected_keys;

            if(counted_by != total_keys && !k->key_values_selected_in_row)
                counted_by++;

            if(counted_by == total_keys) {
                k->current_value.v->final_facet_value_counter++;
                found++;
            }
        }

        internal_fatal(!found, "We should find at least one facet to count this row");
    }

    if(selected_keys == total_keys) {
        // we need to keep this row
        facets_histogram_update_value(facets, usec);

        if(within_anchor)
            facets_row_keep(facets, usec);
    }

    facets_reset_keys_with_value_and_row(facets);

    return selected_keys == total_keys;
}

// ----------------------------------------------------------------------------
// output

static const char *facets_severity_to_string(FACET_ROW_SEVERITY severity) {
    switch(severity) {
        default:
        case FACET_ROW_SEVERITY_NORMAL:
            return "normal";

        case FACET_ROW_SEVERITY_DEBUG:
            return "debug";

        case FACET_ROW_SEVERITY_NOTICE:
            return "notice";

        case FACET_ROW_SEVERITY_WARNING:
            return "warning";

        case FACET_ROW_SEVERITY_CRITICAL:
            return "critical";
    }
}

void facets_accepted_parameters_to_json_array(FACETS *facets, BUFFER *wb, bool with_keys) {
    buffer_json_member_add_array(wb, "accepted_params");
    {
        if(facets->accepted_params) {
            void *t;
            dfe_start_read(facets->accepted_params, t) {
                buffer_json_add_array_item_string(wb, t_dfe.name);
            }
            dfe_done(t);
        }

        if(with_keys) {
            FACET_KEY *k;
            foreach_key_in_facets(facets, k){
                if (!k->values.enabled)
                    continue;

                buffer_json_add_array_item_string(wb, hash_to_static_string(k->hash));
            }
            foreach_key_in_facets_done(k);
        }
    }
    buffer_json_array_close(wb); // accepted_params
}

static int facets_keys_reorder_compar(const void *a, const void *b) {
    const FACET_KEY *ak = *((const FACET_KEY **)a);
    const FACET_KEY *bk = *((const FACET_KEY **)b);

    const char *an = ak->name;
    const char *bn = bk->name;

    if(!an) an = "0";
    if(!bn) bn = "0";

    while(*an && ispunct(*an)) an++;
    while(*bn && ispunct(*bn)) bn++;

    return strcasecmp(an, bn);
}

void facets_sort_and_reorder_keys(FACETS *facets) {
    size_t entries = facets->keys_with_values.used;
    if(!entries)
        return;

    FACET_KEY *keys[entries];
    memcpy(keys, facets->keys_with_values.array, sizeof(FACET_KEY *) * entries);

    qsort(keys, entries, sizeof(FACET_KEY *), facets_keys_reorder_compar);

    for(size_t i = 0; i < entries ;i++)
        keys[i]->order = i + 1;
}

static int facets_key_values_reorder_by_name_compar(const void *a, const void *b) {
    const FACET_VALUE *av = *((const FACET_VALUE **)a);
    const FACET_VALUE *bv = *((const FACET_VALUE **)b);

    const char *an = (av->name && av->name_len) ? av->name : "0";
    const char *bn = (bv->name && bv->name_len) ? bv->name : "0";

    while(*an && ispunct(*an)) an++;
    while(*bn && ispunct(*bn)) bn++;

    int ret = strcasecmp(an, bn);
    return ret;
}

static int facets_key_values_reorder_by_count_compar(const void *a, const void *b) {
    const FACET_VALUE *av = *((const FACET_VALUE **)a);
    const FACET_VALUE *bv = *((const FACET_VALUE **)b);

    if(av->final_facet_value_counter < bv->final_facet_value_counter)
        return 1;

    if(av->final_facet_value_counter > bv->final_facet_value_counter)
        return -1;

    return facets_key_values_reorder_by_name_compar(a, b);
}

static int facets_key_values_reorder_by_name_numeric_compar(const void *a, const void *b) {
    const FACET_VALUE *av = *((const FACET_VALUE **)a);
    const FACET_VALUE *bv = *((const FACET_VALUE **)b);

    const char *an = (av->name && av->name_len) ? av->name : "0";
    const char *bn = (bv->name && bv->name_len) ? bv->name : "0";

    if(strcmp(an, FACET_VALUE_UNSET) == 0) an = "0";
    if(strcmp(bn, FACET_VALUE_UNSET) == 0) bn = "0";

    int64_t ad = str2ll(an, NULL);
    int64_t bd = str2ll(bn, NULL);

    if(ad < bd)
        return -1;

    if(ad > bd)
        return 1;

    return facets_key_values_reorder_by_name_compar(a, b);
}

static uint32_t facets_sort_and_reorder_values_internal(FACET_KEY *k) {
    bool all_values_numeric = true;
    size_t entries = k->values.used;
    FACET_VALUE *values[entries], *v;
    uint32_t used = 0;
    foreach_value_in_key(k, v) {
        if((k->facets->options & FACETS_OPTION_DONT_SEND_EMPTY_VALUE_FACETS) && v->empty)
            continue;

        if(all_values_numeric && !v->empty && v->name && v->name_len) {
            const char *s = v->name;
            while(isdigit(*s)) s++;
            if(*s != '\0')
                all_values_numeric = false;
        }

        values[used++] = v;

        if(used >= entries)
            break;
    }
    foreach_value_in_key_done(v);

    if(!used)
        return 0;

    if(k->facets->options & FACETS_OPTION_SORT_FACETS_ALPHABETICALLY) {
        if(all_values_numeric)
            qsort(values, used, sizeof(FACET_VALUE *), facets_key_values_reorder_by_name_numeric_compar);
        else
            qsort(values, used, sizeof(FACET_VALUE *), facets_key_values_reorder_by_name_compar);
    }
    else
        qsort(values, used, sizeof(FACET_VALUE *), facets_key_values_reorder_by_count_compar);

    for(size_t i = 0; i < used; i++)
        values[i]->order = i + 1;

    return used;
}

static uint32_t facets_sort_and_reorder_values(FACET_KEY *k) {
    if(!k->values.enabled || !k->values.ll || !k->values.used)
        return 0;

    if(!k->transform.cb || !(k->facets->options & FACETS_OPTION_SORT_FACETS_ALPHABETICALLY))
        return facets_sort_and_reorder_values_internal(k);

    // we have a transformation and has to be sorted alphabetically

    BUFFER *tb = buffer_create(0, NULL);
    uint32_t ret = 0;

    size_t entries = k->values.used;
    struct {
        const char *name;
        uint32_t name_len;
    } values[entries];
    FACET_VALUE *v;
    uint32_t used = 0;

    foreach_value_in_key(k, v) {
        if(used >= entries)
            break;

        values[used].name = v->name;
        values[used].name_len = v->name_len;
        used++;

        buffer_contents_replace(tb, v->name, v->name_len);
        k->transform.cb(k->facets, tb, FACETS_TRANSFORM_FACET_SORT, k->transform.data);
        v->name = strdupz(buffer_tostring(tb));
        v->name_len = buffer_strlen(tb);
    }
    foreach_value_in_key_done(v);

    ret = facets_sort_and_reorder_values_internal(k);

    used = 0;
    foreach_value_in_key(k, v) {
        if(used >= entries)
            break;

        freez((void *)v->name);
        v->name = values[used].name;
        v->name_len = values[used].name_len;
        used++;
    }
    foreach_value_in_key_done(v);

    buffer_free(tb);
    return ret;
}

void facets_table_config(BUFFER *wb) {
    buffer_json_member_add_boolean(wb, "show_ids", false);   // do not show the column ids to the user
    buffer_json_member_add_boolean(wb, "has_history", true); // enable date-time picker with after-before

    buffer_json_member_add_object(wb, "pagination");
    {
        buffer_json_member_add_boolean(wb, "enabled", true);
        buffer_json_member_add_string(wb, "key", "anchor");
        buffer_json_member_add_string(wb, "column", "timestamp");
        buffer_json_member_add_string(wb, "units", "timestamp_usec");
    }
    buffer_json_object_close(wb); // pagination
}

static const char *facets_json_key_name_string(FACET_KEY *k, DICTIONARY *used_hashes_registry) {
    if(k->name) {
        if(used_hashes_registry && !k->default_selected_for_values) {
            char hash_str[FACET_STRING_HASH_SIZE];
            facets_hash_to_str(k->hash, hash_str);
            dictionary_set(used_hashes_registry, hash_str, (void *)k->name, strlen(k->name) + 1);
        }

        return k->name;
    }

    // key has no name
    const char *name = "[UNAVAILABLE_FIELD]";

    if(used_hashes_registry) {
        char hash_str[FACET_STRING_HASH_SIZE];
        facets_hash_to_str(k->hash, hash_str);
        const char *s = dictionary_get(used_hashes_registry, hash_str);
        if(s) name = s;
    }

    return name;
}

static const char *facets_json_key_value_string(FACET_KEY *k, FACET_VALUE *v, DICTIONARY *used_hashes_registry) {
    if(v->name && v->name_len) {
        if(used_hashes_registry && !k->default_selected_for_values && v->selected) {
            char hash_str[FACET_STRING_HASH_SIZE];
            facets_hash_to_str(v->hash, hash_str);
            dictionary_set(used_hashes_registry, hash_str, (void *)v->name, v->name_len + 1);
        }

        return v->name;
    }

    // key has no name
    const char *name = "[unavailable field]";

    if(used_hashes_registry) {
        char hash_str[FACET_STRING_HASH_SIZE];
        facets_hash_to_str(v->hash, hash_str);
        const char *s = dictionary_get(used_hashes_registry, hash_str);
        if(s) name = s;
    }

    return name;
}

void facets_report(FACETS *facets, BUFFER *wb, DICTIONARY *used_hashes_registry) {
    if(!(facets->options & FACETS_OPTION_DATA_ONLY)) {
        facets_table_config(wb);
        facets_accepted_parameters_to_json_array(facets, wb, true);
    }

    // ------------------------------------------------------------------------
    // facets

    if(!(facets->options & FACETS_OPTION_DONT_SEND_FACETS)) {
        bool show_facets = false;

        if(facets->options & FACETS_OPTION_DATA_ONLY) {
            if(facets->options & FACETS_OPTION_SHOW_DELTAS) {
                buffer_json_member_add_array(wb, "facets_delta");
                show_facets = true;
            }
        }
        else {
            buffer_json_member_add_array(wb, "facets");
            show_facets = true;
        }

        if(show_facets) {
            BUFFER *tb = NULL;
            FACET_KEY *k;
            foreach_key_in_facets(facets, k) {
                if(!k->values.enabled)
                    continue;

                if(!facets_sort_and_reorder_values(k))
                    // no values for this key
                    continue;

                buffer_json_add_array_item_object(wb); // key
                {
                    buffer_json_member_add_string(wb, "id", hash_to_static_string(k->hash));
                    buffer_json_member_add_string(wb, "name", facets_json_key_name_string(k, used_hashes_registry));

                    if(!k->order) k->order = facets->order++;
                    buffer_json_member_add_uint64(wb, "order", k->order);

                    buffer_json_member_add_array(wb, "options");
                    {
                        FACET_VALUE *v;
                        foreach_value_in_key(k, v) {
                            if((facets->options & FACETS_OPTION_DONT_SEND_EMPTY_VALUE_FACETS) && v->empty)
                                continue;

                            buffer_json_add_array_item_object(wb);
                            {
                                buffer_json_member_add_string(wb, "id", hash_to_static_string(v->hash));

                                if(!v->empty && k->transform.cb && k->transform.view_only) {
                                    if(!tb)
                                        tb = buffer_create(0, NULL);

                                    buffer_contents_replace(tb, v->name, v->name_len);
                                    k->transform.cb(facets, tb, FACETS_TRANSFORM_FACET, k->transform.data);
                                    buffer_json_member_add_string(wb, "name", buffer_tostring(tb));
                                }
                                else
                                    buffer_json_member_add_string(wb, "name", facets_json_key_value_string(k, v, used_hashes_registry));

                                buffer_json_member_add_uint64(wb, "count", v->final_facet_value_counter);
                                buffer_json_member_add_uint64(wb, "order", v->order);
                            }
                            buffer_json_object_close(wb);
                        }
                        foreach_value_in_key_done(v);
                    }
                    buffer_json_array_close(wb); // options
                }
                buffer_json_object_close(wb); // key
            }
            foreach_key_in_facets_done(k);
            buffer_free(tb);
            buffer_json_array_close(wb); // facets
        }
    }

    // ------------------------------------------------------------------------
    // columns

    buffer_json_member_add_object(wb, "columns");
    {
        size_t field_id = 0;
        buffer_rrdf_table_add_field(
                wb, field_id++,
                "timestamp", "Timestamp",
                RRDF_FIELD_TYPE_TIMESTAMP,
                RRDF_FIELD_VISUAL_VALUE,
                RRDF_FIELD_TRANSFORM_DATETIME_USEC, 0, NULL, NAN,
                RRDF_FIELD_SORT_DESCENDING|RRDF_FIELD_SORT_FIXED,
                NULL,
                RRDF_FIELD_SUMMARY_COUNT,
                RRDF_FIELD_FILTER_RANGE,
                RRDF_FIELD_OPTS_WRAP | RRDF_FIELD_OPTS_VISIBLE | RRDF_FIELD_OPTS_UNIQUE_KEY,
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
                RRDR_FIELD_OPTS_DUMMY,
                NULL);

        FACET_KEY *k;
        foreach_key_in_facets(facets, k) {
                    RRDF_FIELD_OPTIONS options = RRDF_FIELD_OPTS_WRAP;
                    bool visible = k->options & (FACET_KEY_OPTION_VISIBLE | FACET_KEY_OPTION_STICKY);

                    if ((facets->options & FACETS_OPTION_ALL_FACETS_VISIBLE && k->values.enabled))
                        visible = true;

                    if (!visible)
                        visible = simple_pattern_matches(facets->visible_keys, k->name);

                    if (visible)
                        options |= RRDF_FIELD_OPTS_VISIBLE;

                    if (k->options & FACET_KEY_OPTION_MAIN_TEXT)
                        options |= RRDF_FIELD_OPTS_FULL_WIDTH | RRDF_FIELD_OPTS_WRAP;

                    const char *hash_str = hash_to_static_string(k->hash);

                    buffer_rrdf_table_add_field(
                            wb, field_id++,
                            hash_str, k->name ? k->name : hash_str,
                            RRDF_FIELD_TYPE_STRING,
                            (k->options & FACET_KEY_OPTION_RICH_TEXT) ? RRDF_FIELD_VISUAL_RICH : RRDF_FIELD_VISUAL_VALUE,
                            RRDF_FIELD_TRANSFORM_NONE, 0, NULL, NAN,
                            RRDF_FIELD_SORT_FIXED,
                            NULL,
                            RRDF_FIELD_SUMMARY_COUNT,
                            (k->options & FACET_KEY_OPTION_NEVER_FACET) ? RRDF_FIELD_FILTER_NONE
                                                                        : RRDF_FIELD_FILTER_FACET,
                            options, FACET_VALUE_UNSET);
                }
        foreach_key_in_facets_done(k);
    }
    buffer_json_object_close(wb); // columns

    // ------------------------------------------------------------------------
    // rows data

    buffer_json_member_add_array(wb, "data");
    {
        usec_t last_usec = 0; (void)last_usec;

        for(FACET_ROW *row = facets->base ; row ;row = row->next) {

            internal_fatal(
                    facets->anchor.start_ut && (
                        (facets->anchor.direction == FACETS_ANCHOR_DIRECTION_BACKWARD && row->usec >= facets->anchor.start_ut) ||
                        (facets->anchor.direction == FACETS_ANCHOR_DIRECTION_FORWARD && row->usec <= facets->anchor.start_ut)
                    ), "Wrong data returned related to %s start anchor!", facets->anchor.direction == FACETS_ANCHOR_DIRECTION_FORWARD ? "forward" : "backward");

            internal_fatal(last_usec && row->usec > last_usec, "Wrong order of data returned!");

            last_usec = row->usec;

            buffer_json_add_array_item_array(wb); // each row
            buffer_json_add_array_item_uint64(wb, row->usec);
            buffer_json_add_array_item_object(wb);
            {
                if(facets->severity.cb)
                    row->severity = facets->severity.cb(facets, row, facets->severity.data);

                buffer_json_member_add_string(wb, "severity", facets_severity_to_string(row->severity));
            }
            buffer_json_object_close(wb);

            FACET_KEY *k;
            foreach_key_in_facets(facets, k) {
                FACET_ROW_KEY_VALUE *rkv = dictionary_get(row->dict, k->name);

                if(unlikely(k->dynamic.cb)) {
                    if(unlikely(!rkv))
                        rkv = dictionary_set(row->dict, k->name, NULL, sizeof(*rkv));

                    k->dynamic.cb(facets, wb, rkv, row, k->dynamic.data);
                    facets->operations.values.dynamic++;
                }
                else {
                    if(!rkv || rkv->empty) {
                        buffer_json_add_array_item_string(wb, NULL);
                    }
                    else if(unlikely(k->transform.cb && k->transform.view_only)) {
                        k->transform.cb(facets, rkv->wb, FACETS_TRANSFORM_DATA, k->transform.data);
                        buffer_json_add_array_item_string(wb, buffer_tostring(rkv->wb));
                    }
                    else
                        buffer_json_add_array_item_string(wb, buffer_tostring(rkv->wb));
                }
            }
            foreach_key_in_facets_done(k);
            buffer_json_array_close(wb); // each row
        }
    }
    buffer_json_array_close(wb); // data

    if(!(facets->options & FACETS_OPTION_DATA_ONLY)) {
        buffer_json_member_add_string(wb, "default_sort_column", "timestamp");
        buffer_json_member_add_array(wb, "default_charts");
        buffer_json_array_close(wb);
    }

    // ------------------------------------------------------------------------
    // histogram

    if(facets->histogram.enabled && !(facets->options & FACETS_OPTION_DONT_SEND_HISTOGRAM)) {
        FACETS_HASH first_histogram_hash = 0;
        buffer_json_member_add_array(wb, "available_histograms");
        {
            FACET_KEY *k;
            foreach_key_in_facets(facets, k) {
                if (!k->values.enabled)
                    continue;

                if(unlikely(!first_histogram_hash))
                    first_histogram_hash = k->hash;

                buffer_json_add_array_item_object(wb);
                buffer_json_member_add_string(wb, "id", hash_to_static_string(k->hash));
                buffer_json_member_add_string(wb, "name", k->name);
                buffer_json_member_add_uint64(wb, "order", k->order);
                buffer_json_object_close(wb);
            }
            foreach_key_in_facets_done(k);
        }
        buffer_json_array_close(wb);

        {
            FACET_KEY *k = FACETS_KEY_GET_FROM_INDEX(facets, facets->histogram.hash);
            if(!k || !k->values.enabled)
                k = FACETS_KEY_GET_FROM_INDEX(facets, first_histogram_hash);

            bool show_histogram = false;

            if(facets->options & FACETS_OPTION_DATA_ONLY) {
                if(facets->options & FACETS_OPTION_SHOW_DELTAS) {
                    buffer_json_member_add_object(wb, "histogram_delta");
                    show_histogram = true;
                }
            }
            else {
                buffer_json_member_add_object(wb, "histogram");
                show_histogram = true;
            }

            if(show_histogram) {
                buffer_json_member_add_string(wb, "id", k ? hash_to_static_string(k->hash) : "");
                buffer_json_member_add_string(wb, "name", k ? k->name : "");
                buffer_json_member_add_object(wb, "chart");
                {
                    facets_histogram_generate(facets, k, wb);
                }
                buffer_json_object_close(wb); // chart
                buffer_json_object_close(wb); // histogram
            }
        }
    }

    // ------------------------------------------------------------------------
    // items

    bool show_items = false;
    if(facets->options & FACETS_OPTION_DATA_ONLY) {
        if(facets->options & FACETS_OPTION_SHOW_DELTAS) {
            buffer_json_member_add_object(wb, "items_delta");
            show_items = true;
        }
    }
    else {
        buffer_json_member_add_object(wb, "items");
        show_items = true;
    }

    if(show_items) {
        buffer_json_member_add_uint64(wb, "evaluated", facets->operations.rows.evaluated);
        buffer_json_member_add_uint64(wb, "matched", facets->operations.rows.matched);
        buffer_json_member_add_uint64(wb, "returned", facets->items_to_return);
        buffer_json_member_add_uint64(wb, "max_to_return", facets->max_items_to_return);
        buffer_json_member_add_uint64(wb, "before", facets->operations.skips_before);
        buffer_json_member_add_uint64(wb, "after", facets->operations.skips_after + facets->operations.shifts);
        buffer_json_object_close(wb); // items
    }

    // ------------------------------------------------------------------------
    // stats

    buffer_json_member_add_object(wb, "_stats");
    {
        buffer_json_member_add_uint64(wb, "first", facets->operations.first);
        buffer_json_member_add_uint64(wb, "forwards", facets->operations.forwards);
        buffer_json_member_add_uint64(wb, "backwards", facets->operations.backwards);
        buffer_json_member_add_uint64(wb, "skips_before", facets->operations.skips_before);
        buffer_json_member_add_uint64(wb, "skips_after", facets->operations.skips_after);
        buffer_json_member_add_uint64(wb, "prepends", facets->operations.prepends);
        buffer_json_member_add_uint64(wb, "appends", facets->operations.appends);
        buffer_json_member_add_uint64(wb, "shifts", facets->operations.shifts);
        buffer_json_member_add_object(wb, "rows");
        {
            buffer_json_member_add_uint64(wb, "created", facets->operations.rows.created);
            buffer_json_member_add_uint64(wb, "reused", facets->operations.rows.reused);
            buffer_json_member_add_uint64(wb, "evaluated", facets->operations.rows.evaluated);
            buffer_json_member_add_uint64(wb, "matched", facets->operations.rows.matched);
        }
        buffer_json_object_close(wb); // rows
        buffer_json_member_add_object(wb, "keys");
        {
            buffer_json_member_add_uint64(wb, "registered", facets->operations.keys.registered);
            buffer_json_member_add_uint64(wb, "unique", facets->operations.keys.unique);
            buffer_json_member_add_uint64(wb, "hashtable_increases", facets->operations.keys.hashtable_increases);
        }
        buffer_json_object_close(wb); // keys
        buffer_json_member_add_object(wb, "values");
        {
            buffer_json_member_add_uint64(wb, "registered", facets->operations.values.registered);
            buffer_json_member_add_uint64(wb, "transformed", facets->operations.values.transformed);
            buffer_json_member_add_uint64(wb, "dynamic", facets->operations.values.dynamic);
            buffer_json_member_add_uint64(wb, "empty", facets->operations.values.empty);
            buffer_json_member_add_uint64(wb, "indexed", facets->operations.values.indexed);
            buffer_json_member_add_uint64(wb, "inserts", facets->operations.values.inserts);
            buffer_json_member_add_uint64(wb, "conflicts", facets->operations.values.conflicts);
            buffer_json_member_add_uint64(wb, "hashtable_increases", facets->operations.values.hashtable_increases);
        }
        buffer_json_object_close(wb); // values
        buffer_json_member_add_object(wb, "fts");
        {
            buffer_json_member_add_uint64(wb, "searches", facets->operations.fts.searches);
        }
        buffer_json_object_close(wb); // fts
    }
    buffer_json_object_close(wb); // items
}
