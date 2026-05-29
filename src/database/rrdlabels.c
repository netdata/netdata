// SPDX-License-Identifier: GPL-3.0-or-later

#include "rrd.h"

// Key OF HS ARRRAY

struct {
    int count;
    Pvoid_t JudyHS;
    SPINLOCK spinlock;
} global_labels = {
    .JudyHS = (Pvoid_t) NULL,
    .spinlock = SPINLOCK_INITIALIZER};

typedef struct label_registry_idx {
    STRING *key;
    STRING *value;
} LABEL_REGISTRY_IDX;

typedef struct labels_registry_entry {
    LABEL_REGISTRY_IDX index;
} RRDLABEL;

// Value of HS array
typedef struct labels_registry_idx_entry {
    RRDLABEL label;
    size_t refcount;
} RRDLABEL_IDX;

typedef struct rrdlabels {
    SPINLOCK spinlock;
    uint32_t version;
    Pvoid_t JudyL;
} RRDLABELS;

#define lfe_start_nolock(label_list, label, ls)                                                                        \
    do {                                                                                                               \
        bool _first_then_next = true;                                                                                  \
        Pvoid_t *_PValue;                                                                                              \
        Word_t _Index = 0;                                                                                             \
        while ((_PValue = JudyLFirstThenNext((label_list)->JudyL, &_Index, &_first_then_next))) {                      \
            (ls) = *(RRDLABEL_SRC *)_PValue;                                                                           \
            (void)(ls);                                                                                                \
            (label) = (void *)_Index;

#define lfe_done_nolock()                                                                                              \
        }                                                                                                              \
    }                                                                                                                  \
    while (0)

#define lfe_start_read(label_list, label, ls)                                                                          \
    do {                                                                                                               \
        spinlock_lock(&(label_list)->spinlock);                                                                        \
        bool _first_then_next = true;                                                                                  \
        Pvoid_t *_PValue;                                                                                              \
        Word_t _Index = 0;                                                                                             \
        while ((_PValue = JudyLFirstThenNext((label_list)->JudyL, &_Index, &_first_then_next))) {                      \
            (ls) = *(RRDLABEL_SRC *)_PValue;                                                                           \
            (void)(ls);                                                                                                \
            (label) = (void *)_Index;

#define lfe_done(label_list)                                                                                           \
        }                                                                                                              \
        spinlock_unlock(&(label_list)->spinlock);                                                                      \
    }                                                                                                                  \
    while (0)

static inline void RRDLABELS_MEMORY_DELTA(struct dictionary_stats *stats, int64_t judy_mem, int64_t item_size) {
    __atomic_fetch_add(&stats->memory.index, judy_mem, __ATOMIC_RELAXED);
    __atomic_fetch_add(&stats->memory.dict, item_size, __ATOMIC_RELAXED);
}

__attribute__((constructor)) void initialize_label_stats(void) {
    dictionary_stats_category_rrdlabels.memory.dict = 0;
    dictionary_stats_category_rrdlabels.memory.index = 0;
    dictionary_stats_category_rrdlabels.memory.values = 0;
}


static ARAL *labels_aral;

static struct aral_statistics label_aral_statistics = { 0 };

void rrdlabels_aral_init(bool with_stats)
{
    labels_aral =
        aral_create("label_stat", sizeof(RRDLABELS), 1, 0, &label_aral_statistics, NULL, NULL, false, false, false);

    if (with_stats)
        pulse_aral_register_statistics(&label_aral_statistics, "labels");
}

void rrdlabels_aral_destroy(bool with_stats)
{
    aral_destroy(labels_aral);
    if (with_stats)
        pulse_aral_unregister_statistics(&label_aral_statistics);
}


// ----------------------------------------------------------------------------
// rrdlabels_create()

RRDLABELS *rrdlabels_create(void)
{
    RRDLABELS *labels = aral_mallocz(labels_aral);
    spinlock_init(&labels->spinlock);
    labels->version = 0;
    labels->JudyL = NULL;
    return labels;
}

static void dup_label(RRDLABEL *label_index)
{
    if (!label_index)
        return;

    spinlock_lock(&global_labels.spinlock);

    Pvoid_t *PValue = JudyHSGet(global_labels.JudyHS, (void *)label_index, sizeof(*label_index));
    if (PValue && *PValue) {
        RRDLABEL_IDX *rrdlabel = *PValue;
        __atomic_add_fetch(&rrdlabel->refcount, 1, __ATOMIC_RELAXED);
    }

    spinlock_unlock(&global_labels.spinlock);
}

static RRDLABEL *add_label_name_value(const char *name, const char *value)
{
    RRDLABEL_IDX *rrdlabel = NULL;
    LABEL_REGISTRY_IDX label_index;
    label_index.key = string_strdupz(name);
    label_index.value = string_strdupz(value);

    spinlock_lock(&global_labels.spinlock);

    JudyAllocThreadPulseReset();

    Pvoid_t *PValue = JudyHSIns(&global_labels.JudyHS, (void *)&label_index, sizeof(label_index), PJE0);

    int64_t judy_mem = JudyAllocThreadPulseGetAndReset();

    if(unlikely(!PValue || PValue == PJERR))
        fatal("RRDLABELS: corrupted judyHS array");

    if (*PValue) {
        rrdlabel = *PValue;
        string_freez(label_index.key);
        string_freez(label_index.value);
        RRDLABELS_MEMORY_DELTA(&dictionary_stats_category_rrdlabels, judy_mem, 0);
    } else {
        rrdlabel = callocz(1, sizeof(*rrdlabel));
        rrdlabel->label.index = label_index;
        *PValue = rrdlabel;
        RRDLABELS_MEMORY_DELTA(&dictionary_stats_category_rrdlabels, judy_mem, sizeof(RRDLABEL_IDX));
        global_labels.count++;
    }
    __atomic_add_fetch(&rrdlabel->refcount, 1, __ATOMIC_RELAXED);

    spinlock_unlock(&global_labels.spinlock);
    return &rrdlabel->label;
}

static void delete_label(RRDLABEL *label)
{
    spinlock_lock(&global_labels.spinlock);

    Pvoid_t *PValue = JudyHSGet(global_labels.JudyHS, &label->index, sizeof(label->index));
    if (PValue && *PValue) {
        RRDLABEL_IDX *rrdlabel = *PValue;
        size_t refcount = __atomic_sub_fetch(&rrdlabel->refcount, 1, __ATOMIC_RELAXED);
        if (refcount == 0) {
            JudyAllocThreadPulseReset();

            int rc = JudyHSDel(&global_labels.JudyHS, (void *)&label->index, sizeof(label->index), PJE0);
            if(rc)
                global_labels.count--;

            int64_t judy_mem = JudyAllocThreadPulseGetAndReset();

            RRDLABELS_MEMORY_DELTA(&dictionary_stats_category_rrdlabels, judy_mem, -(int64_t)sizeof(*rrdlabel));
            string_freez(label->index.key);
            string_freez(label->index.value);
            freez(rrdlabel);
        }
    }
    spinlock_unlock(&global_labels.spinlock);
}

int rrdlabels_registry_count(void) {
    spinlock_lock(&global_labels.spinlock);
    int count = global_labels.count;
    spinlock_unlock(&global_labels.spinlock);
    return count;
}

// ----------------------------------------------------------------------------
// rrdlabels_destroy()

void rrdlabels_flush(RRDLABELS *labels) {
    if (unlikely(!labels))
        return;

    spinlock_lock(&labels->spinlock);

    Pvoid_t *PValue;
    Word_t Index = 0;
    bool first_then_next = true;
    while ((PValue = JudyLFirstThenNext(labels->JudyL, &Index, &first_then_next))) {
        delete_label((RRDLABEL *)Index);
    }
    JudyAllocThreadPulseReset();
    JudyLFreeArray(&labels->JudyL, PJE0);
    int64_t judy_mem = JudyAllocThreadPulseGetAndReset();
    RRDLABELS_MEMORY_DELTA(&dictionary_stats_category_rrdlabels, judy_mem, 0);
    spinlock_unlock(&labels->spinlock);
}

void rrdlabels_destroy(RRDLABELS *labels)
{
    if (unlikely(!labels))
        return;

    rrdlabels_flush(labels);
    aral_freez(labels_aral, labels);
}

//
// Check in labels to see if we have the key specified in label
// same_value indicates if the value should also be matched
//
static RRDLABEL *rrdlabels_find_label_with_key_unsafe(RRDLABELS *labels, RRDLABEL *label, bool same_value)
{
    if (unlikely(!labels))
        return NULL;

    Pvoid_t *PValue;
    Word_t Index = 0;
    bool first_then_next = true;
    RRDLABEL *found = NULL;
    while ((PValue = JudyLFirstThenNext(labels->JudyL, &Index, &first_then_next))) {
        RRDLABEL *lb = (RRDLABEL *)Index;
        if (lb->index.key == label->index.key && ((lb == label) == same_value)) {
            found = (RRDLABEL *)Index;
            break;
        }
    }
    return found;
}

// ----------------------------------------------------------------------------
// rrdlabels_add()

static bool labels_add_already_sanitized(RRDLABELS *labels, const char *key, const char *value, RRDLABEL_SRC ls)
{
    if (unlikely(!labels || !key)) return false;

    RRDLABEL *new_label = add_label_name_value(key, value);

    spinlock_lock(&labels->spinlock);

    RRDLABEL_SRC new_ls = (ls & ~(RRDLABEL_FLAG_NEW | RRDLABEL_FLAG_OLD));

    JudyAllocThreadPulseReset();

    Pvoid_t *PValue = JudyLIns(&labels->JudyL, (Word_t)new_label, PJE0);
    if (!PValue || PValue == PJERR)
        fatal("RRDLABELS: corrupted labels JudyL array");

    int64_t judy_mem = JudyAllocThreadPulseGetAndReset();
    RRDLABELS_MEMORY_DELTA(&dictionary_stats_category_rrdlabels, judy_mem, 0);

    bool changed;
    if(*PValue) {
        new_ls |= RRDLABEL_FLAG_OLD;
        *((RRDLABEL_SRC *)PValue) = new_ls;

        delete_label(new_label);
        changed = false;
    }
    else {
        new_ls |= RRDLABEL_FLAG_NEW;
        *((RRDLABEL_SRC *)PValue) = new_ls;

        RRDLABEL *old_label_with_same_key = rrdlabels_find_label_with_key_unsafe(labels, new_label, false);
        if (old_label_with_same_key) {
            int del_result = JudyLDel(&labels->JudyL, (Word_t) old_label_with_same_key, PJE0);
            (void)del_result;
            judy_mem = JudyAllocThreadPulseGetAndReset();
            RRDLABELS_MEMORY_DELTA(&dictionary_stats_category_rrdlabels, judy_mem, 0);
            delete_label(old_label_with_same_key);
        }
        changed = true;
    }

    labels->version++;

//    judy_mem = JudyAllocThreadPulseGetAndReset();
//    RRDLABELS_MEMORY_DELTA(&dictionary_stats_category_rrdlabels, judy_mem, 0);

    spinlock_unlock(&labels->spinlock);

    return changed;
}

void rrdlabels_add(RRDLABELS *labels, const char *name, const char *value, RRDLABEL_SRC ls)
{
    (void)rrdlabels_add_changed(labels, name, value, ls);
}

bool rrdlabels_add_changed(RRDLABELS *labels, const char *name, const char *value, RRDLABEL_SRC ls)
{
    if(!labels) {
        netdata_log_error("%s(): called with NULL dictionary.", __FUNCTION__ );
        return false;
    }

    char n[RRDLABELS_MAX_NAME_LENGTH + 1], v[RRDLABELS_MAX_VALUE_LENGTH + 1];
    rrdlabels_sanitize_name(n, name, RRDLABELS_MAX_NAME_LENGTH);
    rrdlabels_sanitize_value(v, value, RRDLABELS_MAX_VALUE_LENGTH);

    if(!*n) {
        netdata_log_error("%s: cannot add name '%s' (value '%s') which is sanitized as empty string", __FUNCTION__, name, value);
        return false;
    }

    return labels_add_already_sanitized(labels, n, v, ls);
}

bool rrdlabels_exist(RRDLABELS *labels, const char *key)
{
    if (!labels)
        return false;

    STRING *this_key = string_strdupz(key);

    RRDLABEL *lb;
    RRDLABEL_SRC ls;

    bool found = false;
    lfe_start_read(labels, lb, ls)
    {
        if (lb->index.key == this_key) {
            found = true;
            break;
        }
    }
    lfe_done(labels);
    string_freez(this_key);
    return found;
}

static const char *get_quoted_string_up_to(char *dst, size_t dst_size, const char *string, char upto1, char upto2) {
    size_t len = 0;
    char *d = dst, quote = 0;
    while(*string && len++ < dst_size) {
        if(unlikely(!quote && (*string == '\'' || *string == '"'))) {
            quote = *string++;
            continue;
        }

        if(unlikely(quote && *string == quote)) {
            quote = 0;
            string++;
            continue;
        }

        if(unlikely(quote && *string == '\\' && string[1])) {
            string++;
            *d++ = *string++;
            continue;
        }

        if(unlikely(!quote && (*string == upto1 || *string == upto2))) break;

        *d++ = *string++;
    }
    *d = '\0';

    if(*string) string++;

    return string;
}

void rrdlabels_add_pair(RRDLABELS *labels, const char *string, RRDLABEL_SRC ls)
{
    if(!labels) {
        netdata_log_error("%s(): called with NULL dictionary.", __FUNCTION__ );
        return;
    }

    char name[RRDLABELS_MAX_NAME_LENGTH + 1];
    string = get_quoted_string_up_to(name, RRDLABELS_MAX_NAME_LENGTH, string, '=', ':');

    char value[RRDLABELS_MAX_VALUE_LENGTH + 1];
    get_quoted_string_up_to(value, RRDLABELS_MAX_VALUE_LENGTH, string, '\0', '\0');

    rrdlabels_add(labels, name, value, ls);
}

// ----------------------------------------------------------------------------

void rrdlabels_value_to_buffer_array_item_or_null(RRDLABELS *labels, BUFFER *wb, const char *key) {
    if(!labels) return;

    STRING *this_key = string_strdupz(key);

    RRDLABEL *lb;
    RRDLABEL_SRC ls;
    lfe_start_read(labels, lb, ls)
    {
        if (lb->index.key == this_key) {
            if (lb->index.value)
                buffer_json_add_array_item_string(wb, string2str(lb->index.value));
            else
                buffer_json_add_array_item_string(wb, NULL);
            break;
        }
    }
    lfe_done(labels);
    string_freez(this_key);
}

void rrdlabels_key_to_buffer_array_item(RRDLABELS *labels, BUFFER *wb)
{
    if(!labels) return;

    RRDLABEL *lb;
    RRDLABEL_SRC ls;
    lfe_start_read(labels, lb, ls) {
        buffer_json_add_array_item_string(wb, string2str(lb->index.key));
    }
    lfe_done(labels);
}

void rrdlabels_key_to_buffer_array_or_string_or_null(RRDLABELS *labels, BUFFER *wb)
{
    if(!labels) {
        buffer_json_add_array_item_string(wb, NULL);
        return;
    }

    size_t items = rrdlabels_entries(labels);
    if(!items) {
        buffer_json_add_array_item_string(wb, NULL);
        return;
    }

    if(items > 1)
        buffer_json_add_array_item_array(wb);

    RRDLABEL *lb;
    RRDLABEL_SRC ls;
    lfe_start_read(labels, lb, ls) {
        buffer_json_add_array_item_string(wb, string2str(lb->index.key));
        if(items == 1)
            break;
    }
    lfe_done(labels);

    if(items > 1)
        buffer_json_array_close(wb);

}

// ----------------------------------------------------------------------------

void rrdlabels_get_value_strcpyz(RRDLABELS *labels, char *dst, size_t dst_len, const char *key) {
    if(!dst || !dst_len)
        return;

    // make sure the output is empty if we can't find it
    *dst = '\0';

    if(!labels)
        return;

    STRING *this_key = string_strdupz(key);

    RRDLABEL *lb;
    RRDLABEL_SRC ls;

    lfe_start_read(labels, lb, ls)
    {
        if (lb->index.key == this_key) {
            if (lb->index.value)
                strncpyz(dst, string2str(lb->index.value), dst_len - 1);
            else
                dst[0] = '\0';
            break;
        }
    }
    lfe_done(labels);
    string_freez(this_key);
}

void rrdlabels_get_value_strdup_or_null(RRDLABELS *labels, char **value, const char *key)
{
    if(!labels) return;

    STRING *this_key = string_strdupz(key);

    RRDLABEL *lb;
    RRDLABEL_SRC ls;
    lfe_start_read(labels, lb, ls)
    {
        if (lb->index.key == this_key) {
            *value = (lb->index.value) ? strdupz(string2str(lb->index.value)) : NULL;
            break;
        }
    }
    lfe_done(labels);
    string_freez(this_key);
}

void rrdlabels_get_value_to_buffer_or_unset(RRDLABELS *labels, BUFFER *wb, const char *key, const char *unset)
{
    if(!labels || !key || !wb) return;

    STRING *this_key = string_strdupz(key);
    RRDLABEL *lb;
    RRDLABEL_SRC ls;

    bool set = false;
    lfe_start_read(labels, lb, ls)
    {
        if (lb->index.key == this_key) {
            if (lb->index.value)
                buffer_strcat(wb, string2str(lb->index.value));
            else
                buffer_strcat(wb, unset);
            set = true;
            break;
        }
    }
    lfe_done(labels);

    if(!set)
        buffer_strcat(wb, unset);

    string_freez(this_key);
}

static void rrdlabels_unmark_all_unsafe(RRDLABELS *labels)
{
    Pvoid_t *PValue;
    Word_t Index = 0;
    bool first_then_next = true;
    while ((PValue = JudyLFirstThenNext(labels->JudyL, &Index, &first_then_next)))
        *((RRDLABEL_SRC *)PValue) &= ~(RRDLABEL_FLAG_OLD | RRDLABEL_FLAG_NEW);
}

void rrdlabels_unmark_all(RRDLABELS *labels)
{
    spinlock_lock(&labels->spinlock);

    rrdlabels_unmark_all_unsafe(labels);

    spinlock_unlock(&labels->spinlock);
}

static size_t rrdlabels_remove_all_unmarked_unsafe(RRDLABELS *labels)
{
    Pvoid_t *PValue;
    Word_t Index = 0;
    bool first_then_next = true;
    size_t removed = 0;

    while ((PValue = JudyLFirstThenNext(labels->JudyL, &Index, &first_then_next))) {
        if (!((*((RRDLABEL_SRC *)PValue)) & (RRDLABEL_FLAG_INTERNAL))) {

            JudyAllocThreadPulseReset();
            (void)JudyLDel(&labels->JudyL, Index, PJE0);
            int64_t judy_mem = JudyAllocThreadPulseGetAndReset();

            RRDLABELS_MEMORY_DELTA(&dictionary_stats_category_rrdlabels, judy_mem, 0);

            delete_label((RRDLABEL *)Index);
            removed++;
            if (labels->JudyL != (Pvoid_t) NULL) {
                Index = 0;
                first_then_next = true;
            }
        }
    }

    return removed;
}

void rrdlabels_remove_all_unmarked(RRDLABELS *labels)
{
    spinlock_lock(&labels->spinlock);
    (void)rrdlabels_remove_all_unmarked_unsafe(labels);
    spinlock_unlock(&labels->spinlock);
}

void rrdlabels_mark_source_as_old(RRDLABELS *labels, RRDLABEL_SRC src_match)
{
    if (!labels) return;

    Pvoid_t *PValue;
    Word_t Index = 0;
    bool first_then_next = true;

    spinlock_lock(&labels->spinlock);

    while ((PValue = JudyLFirstThenNext(labels->JudyL, &Index, &first_then_next))) {
        if ((*((RRDLABEL_SRC *)PValue)) & src_match)
            *((RRDLABEL_SRC *)PValue) |= RRDLABEL_FLAG_OLD;
    }

    spinlock_unlock(&labels->spinlock);
}

bool rrdlabels_remove_all_unmarked_and_changed(RRDLABELS *labels)
{
    if(!labels) return false;

    spinlock_lock(&labels->spinlock);

    size_t added = 0;
    Pvoid_t *PValue;
    Word_t Index = 0;
    bool first_then_next = true;
    while ((PValue = JudyLFirstThenNext(labels->JudyL, &Index, &first_then_next))) {
        if ((*((RRDLABEL_SRC *)PValue)) & RRDLABEL_FLAG_NEW)
            added++;
    }

    size_t removed = rrdlabels_remove_all_unmarked_unsafe(labels);

    spinlock_unlock(&labels->spinlock);

    return (added > 0) || (removed > 0);
}

// ----------------------------------------------------------------------------
// rrdlabels_walkthrough_read()

int rrdlabels_walkthrough_read(RRDLABELS *labels, int (*callback)(const char *name, const char *value, RRDLABEL_SRC ls, void *data), void *data)
{
    int ret = 0;

    if(unlikely(!labels || !callback)) return 0;

    RRDLABEL *lb;
    RRDLABEL_SRC ls;
    lfe_start_read(labels, lb, ls)
    {
        ret = callback(string2str(lb->index.key), string2str(lb->index.value), ls, data);
        if (ret < 0)
            break;
    }
    lfe_done(labels);

    return ret;
}

int rrdlabels_walkthrough_read_string(RRDLABELS *labels, int (*callback)(STRING *name, STRING *value, RRDLABEL_SRC ls, void *data), void *data)
{
    int ret = 0;

    if(unlikely(!labels || !callback)) return 0;

    RRDLABEL *lb;
    RRDLABEL_SRC ls;
    lfe_start_read(labels, lb, ls)
    {
        ret = callback(lb->index.key, lb->index.value, ls, data);
        if (ret < 0)
            break;
    }
    lfe_done(labels);

    return ret;
}

static SIMPLE_PATTERN_RESULT rrdlabels_walkthrough_read_sp(RRDLABELS *labels, SIMPLE_PATTERN_RESULT (*callback)(const char *name, const char *value, RRDLABEL_SRC ls, void *data), void *data)
{
    SIMPLE_PATTERN_RESULT ret = SP_NOT_MATCHED;

    if(unlikely(!labels || !callback)) return 0;

    RRDLABEL *lb;
    RRDLABEL_SRC ls;
    lfe_start_read(labels, lb, ls)
    {
        ret = callback(string2str(lb->index.key), string2str(lb->index.value), ls, data);
        if (ret != SP_NOT_MATCHED)
            break;
    }
    lfe_done(labels);

    return ret;
}

// ----------------------------------------------------------------------------
// rrdlabels_migrate_to_these()
// migrate an existing label list to a new list

bool rrdlabels_migrate_to_these(RRDLABELS *dst, RRDLABELS *src) {
    if (!dst || !src || (dst == src))
        return false;

    spinlock_lock(&dst->spinlock);
    spinlock_lock(&src->spinlock);

    rrdlabels_unmark_all_unsafe(dst);

    RRDLABEL *label;
    Pvoid_t *PValue;
    size_t added = 0;
    size_t cleaned = 0;

    RRDLABEL_SRC ls;
    lfe_start_nolock(src, label, ls)
    {
        JudyAllocThreadPulseGetAndReset();

        // labels->JudyL is keyed by the deduplicated RRDLABEL pointer produced by
        // add_label_name_value(), which encodes BOTH key and value. A same-key
        // value change in src therefore yields a different pointer than the one
        // dst already has, so JudyLIns lands on an empty slot.
        PValue = JudyLIns(&dst->JudyL, (Word_t)label, PJE0);
        if(unlikely(!PValue || PValue == PJERR))
            fatal("RRDLABELS migrate: corrupted labels array");

        if (!*PValue) {
            // Write through PValue BEFORE any subsequent JudyLDel: Judy
            // invalidates previously-returned PValue pointers when the array
            // is modified. Same ordering as labels_add_already_sanitized().
            *((RRDLABEL_SRC *)PValue) = (ls & ~(RRDLABEL_FLAG_OLD | RRDLABEL_FLAG_NEW)) | RRDLABEL_FLAG_NEW;
            dup_label(label);
            int64_t judy_mem = JudyAllocThreadPulseGetAndReset();
            RRDLABELS_MEMORY_DELTA(&dictionary_stats_category_rrdlabels, judy_mem, 0);
            added++;
        }
        else
            *((RRDLABEL_SRC *)PValue) |= RRDLABEL_FLAG_OLD;

        // Ensure at most one entry per key. The remove-unmarked sweep below
        // preserves RRDLABEL_FLAG_DONT_DELETE, so a stale (key, *) entry
        // would otherwise survive next to the desired (key, value) entry.
        // Runs in BOTH branches because the stale entry may pre-date this
        // iteration (e.g. a duplicate left by a prior buggy path) and is
        // independent of whether the current src label is a fresh insert
        // or an already-present (key, value). The find helper skips the
        // just-inserted/just-found entry via same_value=false.
        for (;;) {
            RRDLABEL *old_label_with_same_key = rrdlabels_find_label_with_key_unsafe(dst, label, false);
            if (!old_label_with_same_key)
                break;
            int del_result = JudyLDel(&dst->JudyL, (Word_t)old_label_with_same_key, PJE0);
            (void)del_result;
            int64_t old_judy_mem = JudyAllocThreadPulseGetAndReset();
            RRDLABELS_MEMORY_DELTA(&dictionary_stats_category_rrdlabels, old_judy_mem, 0);
            delete_label(old_label_with_same_key);
            cleaned++;
        }
    }
    lfe_done_nolock();

    size_t removed = rrdlabels_remove_all_unmarked_unsafe(dst);
    dst->version = src->version;

    spinlock_unlock(&src->spinlock);
    spinlock_unlock(&dst->spinlock);

    // cleaned counts duplicates dropped by the same-key cleanup loop above.
    // Without it, a stale (key,*) duplicate removed while the desired
    // (key,value) was already present would mutate dst silently -- callers
    // gating on this return (e.g. rrdset_update_rrdlabels setting
    // RRDSET_FLAG_PENDING_LABEL_RECHECK) would miss the change.
    return (added > 0) || (removed > 0) || (cleaned > 0);
}

//
//
// Return the common labels count in labels1, labels2
//
size_t rrdlabels_common_count(RRDLABELS *labels1, RRDLABELS *labels2)
{
    if (!labels1 || !labels2)
        return 0;

    if (labels1 == labels2)
        return rrdlabels_entries(labels1);

    RRDLABEL *label;
    RRDLABEL_SRC ls;

    spinlock_lock(&labels1->spinlock);
    spinlock_lock(&labels2->spinlock);

    size_t count = 0;
    lfe_start_nolock(labels2, label, ls)
    {
        RRDLABEL *old_label_with_key = rrdlabels_find_label_with_key_unsafe(labels1, label, true);
        if (old_label_with_key)
            count++;
    }
    lfe_done_nolock();

    spinlock_unlock(&labels2->spinlock);
    spinlock_unlock(&labels1->spinlock);
    return count;
}


void rrdlabels_copy(RRDLABELS *dst, RRDLABELS *src)
{
    if (!dst || !src || (dst == src))
        return;

    RRDLABEL *label;
    RRDLABEL_SRC ls;

    spinlock_lock(&dst->spinlock);
    spinlock_lock(&src->spinlock);

    JudyAllocThreadPulseReset();

    bool update_statistics = false;
    lfe_start_nolock(src, label, ls)
    {
        Pvoid_t *PValue = JudyLIns(&dst->JudyL, (Word_t)label, PJE0);
        if(unlikely(!PValue || PValue == PJERR))
            fatal("RRDLABELS: corrupted labels array");

        if (!*PValue) {
            // Write through PValue BEFORE any subsequent JudyLDel: Judy
            // invalidates previously-returned PValue pointers when the array
            // is modified. Same ordering as labels_add_already_sanitized().
            *((RRDLABEL_SRC *)PValue) = (ls & ~(RRDLABEL_FLAG_OLD)) | RRDLABEL_FLAG_NEW;
            dup_label(label);
            dst->version++;
            update_statistics = true;
        }
        else
            *((RRDLABEL_SRC *)PValue) = (ls & ~(RRDLABEL_FLAG_NEW)) | RRDLABEL_FLAG_OLD;

        // Drop any other entry sharing this key. Runs in BOTH branches so
        // pre-existing same-key duplicates (e.g. left behind by a prior
        // buggy path) get cleaned up even when the current src label is
        // already present in dst. Loop because the find helper returns
        // the first match only. The find skips the just-inserted /
        // just-updated entry via same_value=false.
        for (;;) {
            RRDLABEL *old_label_with_key = rrdlabels_find_label_with_key_unsafe(dst, label, false);
            if (!old_label_with_key)
                break;
            (void)JudyLDel(&dst->JudyL, (Word_t)old_label_with_key, PJE0);
            int64_t judy_mem = JudyAllocThreadPulseGetAndReset();
            RRDLABELS_MEMORY_DELTA(&dictionary_stats_category_rrdlabels, judy_mem, 0);
            delete_label((RRDLABEL *)old_label_with_key);
            // Cleanup is itself a state mutation: bump version and request
            // tail-stats accounting so version-based consumers and the
            // memory pulse stay correct even when no new insert happened.
            dst->version++;
            update_statistics = true;
        }
    }
    lfe_done_nolock();
    if (update_statistics) {
        int64_t judy_mem = JudyAllocThreadPulseGetAndReset();
        RRDLABELS_MEMORY_DELTA(&dictionary_stats_category_rrdlabels, judy_mem, 0);
    }

    spinlock_unlock(&src->spinlock);
    spinlock_unlock(&dst->spinlock);
}


// ----------------------------------------------------------------------------
// rrdlabels_match_simple_pattern()
// returns true when there are keys in the dictionary matching a simple pattern

struct simple_pattern_match_name_value {
    size_t searches;
    SIMPLE_PATTERN *pattern;
    char equal;
};

static SIMPLE_PATTERN_RESULT simple_pattern_match_name_only_callback(const char *name, const char *value, RRDLABEL_SRC ls __maybe_unused, void *data) {
    struct simple_pattern_match_name_value *t = (struct simple_pattern_match_name_value *)data;
    (void)value;

    // we return -1 to stop the walkthrough on first match
    t->searches++;
    SIMPLE_PATTERN_RESULT ret = simple_pattern_matches_extract(t->pattern, name, NULL, 0);
    if (ret == SP_MATCHED_NEGATIVE)
        ret = SP_NOT_MATCHED;
    return ret;
}

static SIMPLE_PATTERN_RESULT simple_pattern_match_name_and_value_callback(const char *name, const char *value, RRDLABEL_SRC ls __maybe_unused, void *data) {
    struct simple_pattern_match_name_value *t = (struct simple_pattern_match_name_value *)data;

    // we return -1 to stop the walkthrough on first match
    t->searches++;
    if(simple_pattern_matches(t->pattern, name)) return -1;

    char tmp[RRDLABELS_MAX_NAME_LENGTH + RRDLABELS_MAX_VALUE_LENGTH + 2], *dst = &tmp[0];
    const char *v = value;

    // copy the name
    while(*name) *dst++ = *name++;

    // add the equal
    *dst++ = t->equal;

    // add the value
    while(*v) *dst++ = *v++;

    // terminate it
    *dst = '\0';

    t->searches++;
    return simple_pattern_matches_length_extract(t->pattern, tmp, dst - tmp, NULL, 0);
}

SIMPLE_PATTERN_RESULT rrdlabels_match_simple_pattern_parsed(RRDLABELS *labels, SIMPLE_PATTERN *pattern, char equal, size_t *searches) {
    if (!labels) return false;

    struct simple_pattern_match_name_value t = {
        .searches = 0,
        .pattern = pattern,
        .equal = equal
    };

    SIMPLE_PATTERN_RESULT ret = rrdlabels_walkthrough_read_sp(labels, equal?simple_pattern_match_name_and_value_callback:simple_pattern_match_name_only_callback, &t);

    if(searches)
        *searches = t.searches;

    return ret;
}

bool rrdlabels_match_simple_pattern(RRDLABELS *labels, const char *simple_pattern_txt) {
    if (!labels) return false;

    SIMPLE_PATTERN *pattern = simple_pattern_create(simple_pattern_txt, " ,|\t\r\n\f\v", SIMPLE_PATTERN_EXACT, true);
    char equal = '\0';

    const char *s;
    for(s = simple_pattern_txt; *s ; s++) {
        if (*s == '=' || *s == ':') {
            equal = *s;
            break;
        }
    }

    SIMPLE_PATTERN_RESULT ret = rrdlabels_match_simple_pattern_parsed(labels, pattern, equal, NULL);

    simple_pattern_free(pattern);

    return ret == SP_MATCHED_POSITIVE;
}


// ----------------------------------------------------------------------------
// Log all labels

static int rrdlabels_log_label_to_buffer_callback(const char *name, const char *value, void *data) {
    BUFFER *wb = (BUFFER *)data;

    buffer_sprintf(wb, "Label: %s: \"%s\" (", name, value);
    buffer_strcat(wb, "unknown");
    buffer_strcat(wb, ")\n");

    return 1;
}

void rrdlabels_log_to_buffer(RRDLABELS *labels, BUFFER *wb)
{
    RRDLABEL *lb;
    RRDLABEL_SRC ls;
    lfe_start_read(labels, lb, ls)
        rrdlabels_log_label_to_buffer_callback((void *) string2str(lb->index.key), (void *) string2str(lb->index.value), wb);
    lfe_done(labels);
}


// ----------------------------------------------------------------------------
// rrdlabels_to_buffer()

struct labels_to_buffer {
    BUFFER *wb;
    bool (*filter_callback)(const char *name, const char *value, RRDLABEL_SRC ls, void *data);
    void *filter_data;
    void (*name_sanitizer)(char *dst, const char *src, size_t dst_size);
    void (*value_sanitizer)(char *dst, const char *src, size_t dst_size);
    const char *before_each;
    const char *quote;
    const char *equal;
    const char *between_them;
    size_t count;
};

static int label_to_buffer_callback(const RRDLABEL *lb, void *value __maybe_unused, RRDLABEL_SRC ls, void *data)
{

    struct labels_to_buffer *t = (struct labels_to_buffer *)data;

    size_t n_size = (t->name_sanitizer ) ? ( RRDLABELS_MAX_NAME_LENGTH  * 2 ) : 1;
    size_t v_size = (t->value_sanitizer) ? ( RRDLABELS_MAX_VALUE_LENGTH * 2 ) : 1;

    char n[RRDLABELS_MAX_NAME_LENGTH * 2];
    char v[RRDLABELS_MAX_VALUE_LENGTH * 2];

    const char *name = string2str(lb->index.key);

    const char *nn = name, *vv = string2str(lb->index.value);

    if(t->name_sanitizer) {
        t->name_sanitizer(n, name, n_size);
        nn = n;
    }

    if(t->value_sanitizer) {
        t->value_sanitizer(v, string2str(lb->index.value), v_size);
        vv = v;
    }

    if(!t->filter_callback || t->filter_callback(name, string2str(lb->index.value), ls, t->filter_data)) {
        buffer_sprintf(t->wb, "%s%s%s%s%s%s%s%s%s", t->count++?t->between_them:"", t->before_each, t->quote, nn, t->quote, t->equal, t->quote, vv, t->quote);
        return 1;
    }

    return 0;
}


int label_walkthrough_read(RRDLABELS *labels, int (*callback)(const RRDLABEL *item, void *entry, RRDLABEL_SRC ls, void *data), void *data)
{
    int ret = 0;

    if(unlikely(!labels || !callback)) return 0;

    RRDLABEL *lb;
    RRDLABEL_SRC ls;
    lfe_start_read(labels, lb, ls)
    {
        ret = callback((const RRDLABEL *)lb, (void *)string2str(lb->index.value), ls, data);
        if (ret < 0)
            break;
    }
    lfe_done(labels);
    return ret;
}

int rrdlabels_to_buffer(RRDLABELS *labels, BUFFER *wb, const char *before_each, const char *equal, const char *quote, const char *between_them, bool (*filter_callback)(const char *name, const char *value, RRDLABEL_SRC ls, void *data), void *filter_data, void (*name_sanitizer)(char *dst, const char *src, size_t dst_size), void (*value_sanitizer)(char *dst, const char *src, size_t dst_size)) {
    struct labels_to_buffer tmp = {
        .wb = wb,
        .filter_callback = filter_callback,
        .filter_data = filter_data,
        .name_sanitizer = name_sanitizer,
        .value_sanitizer = value_sanitizer,
        .before_each = before_each,
        .equal = equal,
        .quote = quote,
        .between_them = between_them,
        .count = 0
    };
    return label_walkthrough_read(labels, label_to_buffer_callback, (void *)&tmp);
}

void rrdlabels_to_buffer_json_members(RRDLABELS *labels, BUFFER *wb)
{
    RRDLABEL *lb;
    RRDLABEL_SRC ls;
    lfe_start_read(labels, lb, ls)
        buffer_json_member_add_string(wb, string2str(lb->index.key), string2str(lb->index.value));
    lfe_done(labels);
}

size_t rrdlabels_entries(RRDLABELS *labels __maybe_unused)
{
    if (unlikely(!labels))
        return 0;

    size_t count;
    spinlock_lock(&labels->spinlock);
    count = JudyLCount(labels->JudyL, 0, -1, PJE0);
    spinlock_unlock(&labels->spinlock);
    return count;
}

uint32_t rrdlabels_version(RRDLABELS *labels __maybe_unused)
{
    if (unlikely(!labels))
        return 0;

    return labels->version;
}

void rrdset_update_rrdlabels(RRDSET *st, RRDLABELS *new_rrdlabels) {
    bool labels_changed = false;

    if(!st->rrdlabels)
        st->rrdlabels = rrdlabels_create();

    if (new_rrdlabels)
        labels_changed = rrdlabels_migrate_to_these(st->rrdlabels, new_rrdlabels);

    rrdset_flag_set(st, RRDSET_FLAG_METADATA_UPDATE);
    rrdhost_flag_set(st->rrdhost, RRDHOST_FLAG_METADATA_UPDATE);
    rrdset_metadata_updated(st);

    if(labels_changed) {
        rrdset_flag_set(st, RRDSET_FLAG_PENDING_LABEL_RECHECK);
        rrdhost_flag_set(st->rrdhost, RRDHOST_FLAG_PENDING_HEALTH_INITIALIZATION);
    }
}

// ----------------------------------------------------------------------------
// rrdlabels unit test

struct rrdlabels_unittest_add_a_pair {
    const char *pair;
    const char *expected_name;
    const char *expected_value;
    const char *name;
    const char *value;
    int errors;
};

RRDLABEL *rrdlabels_find_label_with_key(RRDLABELS *labels, const char *key, RRDLABEL_SRC *source)
{
    if (!labels || !key)
        return NULL;

    STRING *this_key = string_strdupz(key);

    RRDLABEL *lb = NULL;
    RRDLABEL_SRC ls;

    lfe_start_read(labels, lb, ls)
    {
        if (lb->index.key == this_key) {
            if (source)
                *source = ls;
            break;
        }
    }
    lfe_done(labels);
    string_freez(this_key);
    return lb;
}

static int rrdlabels_unittest_add_a_pair_callback(const char *name, const char *value, RRDLABEL_SRC ls __maybe_unused, void *data) {
    struct rrdlabels_unittest_add_a_pair *t = (struct rrdlabels_unittest_add_a_pair *)data;

    t->name = name;
    t->value = value;

    if(strcmp(name, t->expected_name) != 0) {
        fprintf(stderr, "name is wrong, found \"%s\", expected \"%s\"", name, t->expected_name);
        t->errors++;
    }

    if(value == NULL && t->expected_value == NULL) {
        ;
    }
    else if(value == NULL || t->expected_value == NULL) {
        fprintf(stderr, "value is wrong, found \"%s\", expected \"%s\"", value?value:"(null)", t->expected_value?t->expected_value:"(null)");
        t->errors++;
    }
    else if(strcmp(value, t->expected_value) != 0) {
        fprintf(stderr, "values don't match, found \"%s\", expected \"%s\"", value, t->expected_value);
        t->errors++;
    }

    return 1;
}

static int rrdlabels_unittest_add_a_pair(const char *pair, const char *name, const char *value) {
    RRDLABELS *labels = rrdlabels_create();
    int errors;

    fprintf(stderr, "rrdlabels_add_pair(labels, %s) ... ", pair);

    rrdlabels_add_pair(labels, pair, RRDLABEL_SRC_CONFIG);

    struct rrdlabels_unittest_add_a_pair tmp = {
        .pair = pair,
        .expected_name = name,
        .expected_value = value,
        .errors = 0
    };
    int ret = rrdlabels_walkthrough_read(labels, rrdlabels_unittest_add_a_pair_callback, &tmp);
    errors = tmp.errors;
    if(ret != 1) {
        fprintf(stderr, "failed to get \"%s\" label", name);
        errors++;
    }

    if(!errors)
        fprintf(stderr, " OK, name='%s' and value='%s'\n", tmp.name, tmp.value?tmp.value:"(null)");
    else
        fprintf(stderr, " FAILED\n");

    rrdlabels_destroy(labels);
    return errors;
}

static int rrdlabels_unittest_add_pairs() {
    fprintf(stderr, "\n%s() tests\n", __FUNCTION__);

    int errors = 0;

    // basic test
    errors += rrdlabels_unittest_add_a_pair("tag=value", "tag", "value");
    errors += rrdlabels_unittest_add_a_pair("tag:value", "tag", "value");

    // test newlines
    errors += rrdlabels_unittest_add_a_pair("   tag   = \t value \r\n", "tag", "value");

    // test spaces in names
    errors += rrdlabels_unittest_add_a_pair("   t   a   g   = value", "t_a_g", "value");

    // test : in values
    errors += rrdlabels_unittest_add_a_pair("tag=:value", "tag", ":value");
    errors += rrdlabels_unittest_add_a_pair("tag::value", "tag", ":value");
    errors += rrdlabels_unittest_add_a_pair("   tag   =   :value ", "tag", ":value");
    errors += rrdlabels_unittest_add_a_pair("   tag   :   :value ", "tag", ":value");
    errors += rrdlabels_unittest_add_a_pair("tag:5", "tag", "5");
    errors += rrdlabels_unittest_add_a_pair("tag:55", "tag", "55");
    errors += rrdlabels_unittest_add_a_pair("tag:aa", "tag", "aa");
    errors += rrdlabels_unittest_add_a_pair("tag:a", "tag", "a");

    // test empty values
    errors += rrdlabels_unittest_add_a_pair("tag", "tag", "[none]");
    errors += rrdlabels_unittest_add_a_pair("tag:", "tag", "[none]");
    errors += rrdlabels_unittest_add_a_pair("tag:\"\"", "tag", "[none]");
    errors += rrdlabels_unittest_add_a_pair("tag:''", "tag", "[none]");
    errors += rrdlabels_unittest_add_a_pair("tag:\r\n", "tag", "[none]");
    errors += rrdlabels_unittest_add_a_pair("tag\r\n", "tag", "[none]");

    // test UTF-8 in values
    errors += rrdlabels_unittest_add_a_pair("tag: country:Ελλάδα", "tag", "country:Ελλάδα");
    errors += rrdlabels_unittest_add_a_pair("\"tag\": \"country:Ελλάδα\"", "tag", "country:Ελλάδα");
    errors += rrdlabels_unittest_add_a_pair("\"tag\": country:\"Ελλάδα\"", "tag", "country:Ελλάδα");
    errors += rrdlabels_unittest_add_a_pair("\"tag=1\": country:\"Gre\\\"ece\"", "tag_1", "country:Gre_ece");
    errors += rrdlabels_unittest_add_a_pair("\"tag=1\" = country:\"Gre\\\"ece\"", "tag_1", "country:Gre_ece");

    errors += rrdlabels_unittest_add_a_pair("\t'LABE=L'\t=\t\"World\" peace", "LABE_L", "World peace");
    errors += rrdlabels_unittest_add_a_pair("\t'LA\\'B:EL'\t=\tcountry:\"World\":\"Europe\":\"Greece\"", "LA_B_EL", "country:World:Europe:Greece");
    errors += rrdlabels_unittest_add_a_pair("\t'LA\\'B:EL'\t=\tcountry\\\"World\"\\\"Europe\"\\\"Greece\"", "LA_B_EL", "country/World/Europe/Greece");

    errors += rrdlabels_unittest_add_a_pair("NAME=\"VALUE\"", "NAME", "VALUE");
    errors += rrdlabels_unittest_add_a_pair("\"NAME\" : \"VALUE\"", "NAME", "VALUE");
    errors += rrdlabels_unittest_add_a_pair("NAME: \"VALUE\"", "NAME", "VALUE");

    return errors;
}

static int rrdlabels_unittest_expect_value(RRDLABELS *labels, const char *key, const char *value, RRDLABEL_SRC required_source)
{
    RRDLABEL_SRC source;
    RRDLABEL *label = rrdlabels_find_label_with_key(labels, key, &source);
    return (!label || strcmp(string2str(label->index.value), value) != 0 || (source != required_source));
}

static int rrdlabels_unittest_double_check()
{
    fprintf(stderr, "\n%s() tests\n", __FUNCTION__);

    int ret = 0;
    RRDLABELS *labels = rrdlabels_create();

    rrdlabels_add(labels, "key1", "value1", RRDLABEL_SRC_CONFIG);
    ret += rrdlabels_unittest_expect_value(labels, "key1", "value1", RRDLABEL_FLAG_NEW | RRDLABEL_SRC_CONFIG);

    rrdlabels_add(labels, "key1", "value2", RRDLABEL_SRC_CONFIG);
    ret += !rrdlabels_unittest_expect_value(labels, "key1", "value2", RRDLABEL_FLAG_OLD | RRDLABEL_SRC_CONFIG);

    rrdlabels_add(labels, "key2", "value1", RRDLABEL_SRC_ACLK|RRDLABEL_SRC_AUTO);
    ret += !rrdlabels_unittest_expect_value(labels, "key1", "value3", RRDLABEL_FLAG_NEW | RRDLABEL_SRC_ACLK);

    ret += (rrdlabels_entries(labels) != 2);

    rrdlabels_destroy(labels);

    if (ret)
        fprintf(stderr, "\n%s() tests failed\n", __FUNCTION__);
    return ret;
}

static int rrdlabels_walkthrough_index_read(RRDLABELS *labels, int (*callback)(const char *name, const char *value, RRDLABEL_SRC ls, size_t index, void *data), void *data)
{
    int ret = 0;

    if(unlikely(!labels || !callback)) return 0;

    RRDLABEL *lb;
    RRDLABEL_SRC ls;
    size_t index = 0;
    lfe_start_read(labels, lb, ls)
    {
        ret = callback(string2str(lb->index.key), string2str(lb->index.value), ls, index, data);
        if (ret < 0)
            break;

        index++;
    }
    lfe_done(labels);

    return ret;
}

static int unittest_dump_labels(const char *name, const char *value, RRDLABEL_SRC ls, size_t index, void *data __maybe_unused)
{
    if (!index && data) {
        fprintf(stderr, "%s\n", (char *) data);
    }
    fprintf(stderr, "LABEL \"%s\" = %d \"%s\"\n", name, ls & (~RRDLABEL_FLAG_INTERNAL), value);
    return 1;
}

void pattern_array_add_lblkey_with_sp(struct pattern_array *pa, const char *key, SIMPLE_PATTERN *sp);
static int rrdlabels_unittest_pattern_check()
{
    fprintf(stderr, "\n%s() tests\n", __FUNCTION__);
    int rc = 0;

    RRDLABELS *labels = NULL;

    labels = rrdlabels_create();

    rrdlabels_add(labels, "_module", "disk_detection", RRDLABEL_SRC_CONFIG);
    rrdlabels_add(labels, "_plugin", "super_plugin", RRDLABEL_SRC_CONFIG);
    rrdlabels_add(labels, "key1", "value1", RRDLABEL_SRC_CONFIG);
    rrdlabels_add(labels, "key2", "caterpillar", RRDLABEL_SRC_CONFIG);
    rrdlabels_add(labels, "key3", "elephant", RRDLABEL_SRC_CONFIG);
    rrdlabels_add(labels, "key4", "value4", RRDLABEL_SRC_CONFIG);

    bool match;
    struct pattern_array *pa = pattern_array_add_key_value(NULL, "_module", "wrong_module", '=');
    match = pattern_array_label_match(pa, labels, '=', NULL);
    // This should not match:  _module in ("wrong_module")
    if (match)
        rc++;

    pattern_array_add_key_value(pa, "_module", "disk_detection", '=');
    match = pattern_array_label_match(pa, labels, '=', NULL);
    // This should match: _module in ("wrong_module","disk_detection")
    if (!match)
        rc++;

    pattern_array_add_key_value(pa, "key1", "wrong_key1_value", '=');
    match = pattern_array_label_match(pa, labels, '=', NULL);
    // This should not match: _module in ("wrong_module","disk_detection") AND key1 in ("wrong_key1_value")
    if (match)
        rc++;

    pattern_array_add_key_value(pa, "key1", "value1", '=');
    match = pattern_array_label_match(pa, labels, '=', NULL);
    // This should match: _module in ("wrong_module","disk_detection") AND key1 in ("wrong_key1_value", "value1")
    if (!match)
        rc++;

    SIMPLE_PATTERN *sp = simple_pattern_create("key2=cat*,!d*", SIMPLE_PATTERN_DEFAULT_WEB_SEPARATORS, SIMPLE_PATTERN_EXACT, true);
    pattern_array_add_lblkey_with_sp(pa, "key2", sp);

    sp = simple_pattern_create("key3=*phant", SIMPLE_PATTERN_DEFAULT_WEB_SEPARATORS, SIMPLE_PATTERN_EXACT, true);
    pattern_array_add_lblkey_with_sp(pa, "key3", sp);

    match = pattern_array_label_match(pa, labels, '=', NULL);
    // This should match: _module in ("wrong_module","disk_detection") AND key1 in ("wrong_key1_value", "value1") AND key2 in ("cat* !d*") AND key3 in ("*phant")
    if (!match)
        rc++;

    rrdlabels_add(labels, "key3", "now_fail", RRDLABEL_SRC_CONFIG);
    match = pattern_array_label_match(pa, labels, '=', NULL);
    // This should not match: _module in ("wrong_module","disk_detection") AND key1 in ("wrong_key1_value", "value1") AND key2 in ("cat* !d*") AND key3 in ("*phant")
    if (match)
        rc++;

    pattern_array_free(pa);
    rrdlabels_destroy(labels);

    return rc;
}

// --------------------------------------------------------------------------------------------------------------------
// Full text search for labels

#include "rrdlabels-aggregated.h"

struct label_full_text_search_data {
    SIMPLE_PATTERN *pattern;
    RRDLABELS_AGGREGATED *agg;
    size_t searches;
    bool found;
};

static int label_full_text_search_callback_string(STRING *name, STRING *value, RRDLABEL_SRC ls __maybe_unused, void *data) {
    struct label_full_text_search_data *d = (struct label_full_text_search_data *)data;

    // Now we have STRING* directly, no need to create them
    bool key_matches = false;
    bool value_matches = false;

    // First try to match the key
    if(simple_pattern_matches_string(d->pattern, name)) {
        key_matches = true;
        d->searches++;
    }

    // Then try to match the value
    if(simple_pattern_matches_string(d->pattern, value)) {
        value_matches = true;
        d->searches++;
    }

    // If either matched, add this label to the aggregated structure
    if(key_matches || value_matches) {
        if(!d->agg) {
            d->agg = rrdlabels_aggregated_create();
        }
        rrdlabels_aggregated_add_label(d->agg, string2str(name), string2str(value));
        d->found = true;
    }

    return 0; // Continue walking through labels
}

RRDLABELS_AGGREGATED *rrdlabels_full_text_search(RRDLABELS *labels, SIMPLE_PATTERN *pattern, RRDLABELS_AGGREGATED *agg, size_t *searches) {
    if(!labels || !pattern) return agg;

    struct label_full_text_search_data data = {
        .pattern = pattern,
        .agg = agg,
        .searches = 0,
        .found = false
    };

    rrdlabels_walkthrough_read_string(labels, label_full_text_search_callback_string, &data);

    if(searches)
        *searches = data.searches;

    // If we found matches and didn't have an agg structure, one was created
    // If we had an agg structure, we added to it (or not if no matches)
    // If no matches and no initial agg, data.agg is still NULL
    return data.agg;
}

// --------------------------------------------------------------------------------------------------------------------
// unittest

static int rrdlabels_unittest_migrate_check()
{
    fprintf(stderr, "\n%s() tests\n", __FUNCTION__);

    RRDLABELS *labels1 = NULL;
    RRDLABELS *labels2 = NULL;

    labels1 = rrdlabels_create();
    labels2 = rrdlabels_create();

    rrdlabels_add(labels1, "key1", "value1", RRDLABEL_SRC_CONFIG);
    rrdlabels_add(labels1, "key1", "value2", RRDLABEL_SRC_CONFIG);

    rrdlabels_add(labels2, "new_key1", "value2", RRDLABEL_SRC_CONFIG);
    rrdlabels_add(labels2, "new_key2", "value2", RRDLABEL_SRC_CONFIG);
    rrdlabels_add(labels2, "key1", "value2", RRDLABEL_SRC_CONFIG);

    fprintf(stderr, "Labels1 entries found %zu  (should be 1)\n",  rrdlabels_entries(labels1));
    fprintf(stderr, "Labels2 entries found %zu  (should be 3)\n",  rrdlabels_entries(labels2));

    rrdlabels_migrate_to_these(labels1, labels2);

    int rc = 0;
    rc = rrdlabels_unittest_expect_value(labels1, "key1", "value2", RRDLABEL_FLAG_OLD | RRDLABEL_SRC_CONFIG);
    if (rc) {
        rrdlabels_destroy(labels1);
        rrdlabels_destroy(labels2);
        return rc;
    }

    fprintf(stderr, "labels1 (migrated) entries found %zu (should be 3)\n",  rrdlabels_entries(labels1));
    size_t entries = rrdlabels_entries(labels1);

    rrdlabels_destroy(labels1);
    rrdlabels_destroy(labels2);

    if (entries != 3)
        return 1;

    // Copy test
    labels1 = rrdlabels_create();
    labels2 = rrdlabels_create();

    rrdlabels_add(labels1, "key1", "value1", RRDLABEL_SRC_CONFIG);
    rrdlabels_add(labels1, "key2", "value2", RRDLABEL_SRC_CONFIG);
    rrdlabels_add(labels1, "key3", "value3", RRDLABEL_SRC_CONFIG);
    rrdlabels_add(labels1, "key4", "value4", RRDLABEL_SRC_CONFIG);  // 4 keys
    rrdlabels_walkthrough_index_read(labels1, unittest_dump_labels, "\nlabels1");

    rrdlabels_add(labels2, "key0", "value0", RRDLABEL_SRC_CONFIG);
    rrdlabels_add(labels2, "key1", "value1", RRDLABEL_SRC_CONFIG);
    rrdlabels_add(labels2, "key2", "value2", RRDLABEL_SRC_CONFIG);

    rc = rrdlabels_unittest_expect_value(labels1, "key1", "value1", RRDLABEL_FLAG_NEW | RRDLABEL_SRC_CONFIG);
    if (rc) {
        rrdlabels_destroy(labels1);
        rrdlabels_destroy(labels2);
        return rc;
    }

    rrdlabels_walkthrough_index_read(labels2, unittest_dump_labels, "\nlabels2");

    rrdlabels_copy(labels1, labels2); // labels1 should have 5 keys
    rc = rrdlabels_unittest_expect_value(labels1, "key1", "value1", RRDLABEL_FLAG_OLD  | RRDLABEL_SRC_CONFIG);
    if (rc) {
        rrdlabels_destroy(labels1);
        rrdlabels_destroy(labels2);
        return rc;
    }

    rc = rrdlabels_unittest_expect_value(labels1, "key0", "value0", RRDLABEL_FLAG_NEW | RRDLABEL_SRC_CONFIG);
    if (rc) {
        rrdlabels_destroy(labels1);
        rrdlabels_destroy(labels2);
        return rc;
    }

    rrdlabels_walkthrough_index_read(labels1, unittest_dump_labels, "\nlabels1 after copy from labels2");
    entries = rrdlabels_entries(labels1);

    fprintf(stderr, "labels1 (copied) entries found %zu (should be 5)\n",  rrdlabels_entries(labels1));
    if (entries != 5) {
        rrdlabels_destroy(labels1);
        rrdlabels_destroy(labels2);
        return 1;
    }

    rrdlabels_add(labels1, "key0", "value0", RRDLABEL_SRC_CONFIG);
    rc = rrdlabels_unittest_expect_value(labels1, "key0", "value0", RRDLABEL_FLAG_OLD | RRDLABEL_SRC_CONFIG);

    rrdlabels_destroy(labels1);
    rrdlabels_destroy(labels2);

    return rc;
}

// Exercises the unmark + re-add + remove_all_unmarked cycle that
// reload_host_labels() relies on, plus the rrdlabels_mark_source_as_old()
// preservation path used when the kubernetes loader fails.
static int rrdlabels_unittest_mark_source_as_old(void) {
    fprintf(stderr, "\n%s() tests\n", __FUNCTION__);
    int errors = 0;

    // Subtest A: the prune drops entries that no loader re-added.
    {
        RRDLABELS *l = rrdlabels_create();
        rrdlabels_add(l, "cfg_key",  "cv", RRDLABEL_SRC_CONFIG);
        rrdlabels_add(l, "k8s_key",  "kv", RRDLABEL_SRC_AUTO | RRDLABEL_SRC_K8S);
        rrdlabels_add(l, "aclk_key", "av", RRDLABEL_SRC_AUTO | RRDLABEL_SRC_ACLK);

        rrdlabels_unmark_all(l);
        // simulate a successful config + aclk loader; the k8s loader added nothing
        rrdlabels_add(l, "cfg_key",  "cv", RRDLABEL_SRC_CONFIG);
        rrdlabels_add(l, "aclk_key", "av", RRDLABEL_SRC_AUTO | RRDLABEL_SRC_ACLK);
        rrdlabels_remove_all_unmarked(l);

        if (rrdlabels_exist(l, "k8s_key")) {
            fprintf(stderr, "  FAIL (A): k8s_key should have been pruned (not re-added, not preserved)\n");
            errors++;
        }
        if (!rrdlabels_exist(l, "cfg_key") || !rrdlabels_exist(l, "aclk_key")) {
            fprintf(stderr, "  FAIL (A): re-added labels should have survived the prune\n");
            errors++;
        }
        rrdlabels_destroy(l);
    }

    // Subtest B: mark_source_as_old(K8S) preserves k8s entries through the prune
    // (the k8s-loader-failure preservation path in reload_host_labels()).
    {
        RRDLABELS *l = rrdlabels_create();
        rrdlabels_add(l, "cfg_key",  "cv",  RRDLABEL_SRC_CONFIG);
        rrdlabels_add(l, "k8s_key",  "kv",  RRDLABEL_SRC_AUTO | RRDLABEL_SRC_K8S);
        rrdlabels_add(l, "k8s_key2", "kv2", RRDLABEL_SRC_AUTO | RRDLABEL_SRC_K8S);

        rrdlabels_unmark_all(l);
        // simulate successful config loader; k8s loader failed
        rrdlabels_add(l, "cfg_key", "cv", RRDLABEL_SRC_CONFIG);
        rrdlabels_mark_source_as_old(l, RRDLABEL_SRC_K8S);
        rrdlabels_remove_all_unmarked(l);

        if (!rrdlabels_exist(l, "k8s_key") || !rrdlabels_exist(l, "k8s_key2")) {
            fprintf(stderr, "  FAIL (B): K8S labels should be preserved via mark_source_as_old\n");
            errors++;
        }
        if (!rrdlabels_exist(l, "cfg_key")) {
            fprintf(stderr, "  FAIL (B): cfg_key should still be present after the prune\n");
            errors++;
        }
        rrdlabels_destroy(l);
    }

    // Subtest C: mark_source_as_old(K8S) only preserves labels whose source
    // mask intersects K8S; entries with other-only sources are still pruned.
    {
        RRDLABELS *l = rrdlabels_create();
        rrdlabels_add(l, "aclk_key", "av", RRDLABEL_SRC_AUTO | RRDLABEL_SRC_ACLK);
        rrdlabels_add(l, "cfg_key",  "cv", RRDLABEL_SRC_CONFIG);

        rrdlabels_unmark_all(l);
        rrdlabels_mark_source_as_old(l, RRDLABEL_SRC_K8S);
        rrdlabels_remove_all_unmarked(l);

        if (rrdlabels_exist(l, "aclk_key") || rrdlabels_exist(l, "cfg_key")) {
            fprintf(stderr, "  FAIL (C): non-K8S labels must not be preserved by mark_source_as_old(K8S)\n");
            errors++;
        }
        rrdlabels_destroy(l);
    }

    fprintf(stderr, "%s: %d errors\n", __FUNCTION__, errors);
    return errors;
}

#define UT_EXPECT(_cond, _msg) do {                                            \
    if (!(_cond)) {                                                            \
        fprintf(stderr, "  FAIL: %s\n", (_msg));                               \
        errors++;                                                              \
    }                                                                          \
} while (0)

static int rrdlabels_unittest_change_detection(void) {
    fprintf(stderr, "\n%s() tests\n", __FUNCTION__);
    int errors = 0;

    // ---- rrdlabels_add_changed: new vs same vs value-change ----
    RRDLABELS *l = rrdlabels_create();
    UT_EXPECT(rrdlabels_add_changed(l, "k1", "v1", RRDLABEL_SRC_CONFIG) == true,
              "add of new label should return true");
    UT_EXPECT(rrdlabels_add_changed(l, "k1", "v1", RRDLABEL_SRC_CONFIG) == false,
              "re-add of identical key+value should return false");
    UT_EXPECT(rrdlabels_add_changed(l, "k1", "v2", RRDLABEL_SRC_CONFIG) == true,
              "value change for an existing key should return true");
    UT_EXPECT(rrdlabels_add_changed(l, "k2", "v2", RRDLABEL_SRC_CONFIG) == true,
              "add of a different new key should return true");
    rrdlabels_destroy(l);

    // ---- rrdlabels_migrate_to_these: empty/identical/diff/DONT_DELETE ----
    RRDLABELS *dst = rrdlabels_create();
    RRDLABELS *src = rrdlabels_create();

    // empty dst, empty src => no add, no remove => false
    UT_EXPECT(rrdlabels_migrate_to_these(dst, src) == false,
              "migrate of two empty label sets should return false");

    // populate src; dst empty => added > 0 => true
    rrdlabels_add(src, "k1", "v1", RRDLABEL_SRC_CONFIG);
    rrdlabels_add(src, "k2", "v2", RRDLABEL_SRC_CONFIG);
    UT_EXPECT(rrdlabels_migrate_to_these(dst, src) == true,
              "migrate to empty dst should return true when src has labels");

    // identical content now (dst was populated from src) => false
    UT_EXPECT(rrdlabels_migrate_to_these(dst, src) == false,
              "migrate with identical key/value sets should return false");

    // src adds one => true
    rrdlabels_add(src, "k3", "v3", RRDLABEL_SRC_CONFIG);
    UT_EXPECT(rrdlabels_migrate_to_these(dst, src) == true,
              "migrate that adds a label should return true");

    // src drops one => true (removed > 0)
    {
        RRDLABELS *trimmed = rrdlabels_create();
        rrdlabels_add(trimmed, "k1", "v1", RRDLABEL_SRC_CONFIG);
        rrdlabels_add(trimmed, "k2", "v2", RRDLABEL_SRC_CONFIG);
        UT_EXPECT(rrdlabels_migrate_to_these(dst, trimmed) == true,
                  "migrate that removes a label should return true");
        rrdlabels_destroy(trimmed);
    }
    rrdlabels_destroy(dst);
    rrdlabels_destroy(src);

    // DONT_DELETE: a label that is not in src remains in dst; nothing added or
    // removed, so the function returns false even though dst != src.
    dst = rrdlabels_create();
    src = rrdlabels_create();
    rrdlabels_add(dst, "k1", "v1", RRDLABEL_SRC_CONFIG | RRDLABEL_FLAG_DONT_DELETE);
    UT_EXPECT(rrdlabels_migrate_to_these(dst, src) == false,
              "migrate where the only diff is a DONT_DELETE label should return false");
    UT_EXPECT(rrdlabels_entries(dst) == 1,
              "DONT_DELETE label should be preserved after migrate");
    rrdlabels_destroy(dst);
    rrdlabels_destroy(src);

    // Value change via migrate (no DONT_DELETE): dst ends with a single entry
    // for the key, carrying the new value.
    dst = rrdlabels_create();
    src = rrdlabels_create();
    rrdlabels_add(dst, "k1", "v1", RRDLABEL_SRC_CONFIG);
    rrdlabels_add(src, "k1", "v2", RRDLABEL_SRC_CONFIG);
    UT_EXPECT(rrdlabels_migrate_to_these(dst, src) == true,
              "migrate with a value change should return true");
    UT_EXPECT(rrdlabels_entries(dst) == 1,
              "migrate with a value change should leave one entry per key");
    {
        char *v = NULL;
        rrdlabels_get_value_strdup_or_null(dst, &v, "k1");
        UT_EXPECT(v != NULL && strcmp(v, "v2") == 0,
                  "migrate with a value change should leave the new value in dst");
        freez(v);
    }
    rrdlabels_destroy(dst);
    rrdlabels_destroy(src);

    // Value change via migrate where dst pinned the old value with DONT_DELETE:
    // the stale (key, old-value) must not survive, even though DONT_DELETE
    // would otherwise protect it from the remove-unmarked sweep.
    dst = rrdlabels_create();
    src = rrdlabels_create();
    rrdlabels_add(dst, "k1", "v1", RRDLABEL_SRC_CONFIG | RRDLABEL_FLAG_DONT_DELETE);
    rrdlabels_add(src, "k1", "v2", RRDLABEL_SRC_CONFIG);
    UT_EXPECT(rrdlabels_migrate_to_these(dst, src) == true,
              "migrate with a value change for a DONT_DELETE key should return true");
    UT_EXPECT(rrdlabels_entries(dst) == 1,
              "migrate must not leave a duplicate (key,old-value) when DONT_DELETE was set");
    {
        char *v = NULL;
        rrdlabels_get_value_strdup_or_null(dst, &v, "k1");
        UT_EXPECT(v != NULL && strcmp(v, "v2") == 0,
                  "migrate with a DONT_DELETE value change should leave the new value in dst");
        freez(v);
    }
    rrdlabels_destroy(dst);
    rrdlabels_destroy(src);

    // ---- rrdlabels_copy: same-key cleanup path ----
    // Value change via copy: dst ends with a single entry for the key,
    // carrying the new value from src. Exercises the same-key cleanup loop
    // that mirrors the migrate path.
    dst = rrdlabels_create();
    src = rrdlabels_create();
    rrdlabels_add(dst, "k1", "v1", RRDLABEL_SRC_CONFIG);
    rrdlabels_add(src, "k1", "v2", RRDLABEL_SRC_CONFIG);
    rrdlabels_copy(dst, src);
    UT_EXPECT(rrdlabels_entries(dst) == 1,
              "copy with a value change should leave one entry per key");
    {
        char *v = NULL;
        rrdlabels_get_value_strdup_or_null(dst, &v, "k1");
        UT_EXPECT(v != NULL && strcmp(v, "v2") == 0,
                  "copy with a value change should leave the new value in dst");
        freez(v);
    }
    rrdlabels_destroy(dst);
    rrdlabels_destroy(src);

    // Value change via copy where dst pinned the old value with DONT_DELETE.
    dst = rrdlabels_create();
    src = rrdlabels_create();
    rrdlabels_add(dst, "k1", "v1", RRDLABEL_SRC_CONFIG | RRDLABEL_FLAG_DONT_DELETE);
    rrdlabels_add(src, "k1", "v2", RRDLABEL_SRC_CONFIG);
    rrdlabels_copy(dst, src);
    UT_EXPECT(rrdlabels_entries(dst) == 1,
              "copy must not leave a duplicate (key,old-value) when DONT_DELETE was set");
    {
        char *v = NULL;
        rrdlabels_get_value_strdup_or_null(dst, &v, "k1");
        UT_EXPECT(v != NULL && strcmp(v, "v2") == 0,
                  "copy with a DONT_DELETE value change should leave the new value in dst");
        freez(v);
    }
    rrdlabels_destroy(dst);
    rrdlabels_destroy(src);

    // ---- rrdlabels_remove_all_unmarked_and_changed: CLABEL-commit semantics ----
    // (1) unmark + re-add identical set => no change => false
    l = rrdlabels_create();
    rrdlabels_add(l, "k1", "v1", RRDLABEL_SRC_CONFIG);
    rrdlabels_add(l, "k2", "v2", RRDLABEL_SRC_CONFIG);
    rrdlabels_unmark_all(l);
    rrdlabels_add(l, "k1", "v1", RRDLABEL_SRC_CONFIG);
    rrdlabels_add(l, "k2", "v2", RRDLABEL_SRC_CONFIG);
    UT_EXPECT(rrdlabels_remove_all_unmarked_and_changed(l) == false,
              "remove_all_unmarked_and_changed: identical re-commit should return false");
    rrdlabels_destroy(l);

    // (2) unmark + add a new label => added > 0 => true
    l = rrdlabels_create();
    rrdlabels_add(l, "k1", "v1", RRDLABEL_SRC_CONFIG);
    rrdlabels_unmark_all(l);
    rrdlabels_add(l, "k1", "v1", RRDLABEL_SRC_CONFIG);
    rrdlabels_add(l, "k2", "v2", RRDLABEL_SRC_CONFIG);
    UT_EXPECT(rrdlabels_remove_all_unmarked_and_changed(l) == true,
              "remove_all_unmarked_and_changed: adding a label should return true");
    rrdlabels_destroy(l);

    // (3) unmark + commit a subset => removed > 0 => true
    l = rrdlabels_create();
    rrdlabels_add(l, "k1", "v1", RRDLABEL_SRC_CONFIG);
    rrdlabels_add(l, "k2", "v2", RRDLABEL_SRC_CONFIG);
    rrdlabels_unmark_all(l);
    rrdlabels_add(l, "k1", "v1", RRDLABEL_SRC_CONFIG);
    UT_EXPECT(rrdlabels_remove_all_unmarked_and_changed(l) == true,
              "remove_all_unmarked_and_changed: dropping a label should return true");
    rrdlabels_destroy(l);

    // (4) unmark + change a value => true (the changed entry is NEW)
    l = rrdlabels_create();
    rrdlabels_add(l, "k1", "v1", RRDLABEL_SRC_CONFIG);
    rrdlabels_unmark_all(l);
    rrdlabels_add(l, "k1", "v2", RRDLABEL_SRC_CONFIG);
    UT_EXPECT(rrdlabels_remove_all_unmarked_and_changed(l) == true,
              "remove_all_unmarked_and_changed: value change should return true");
    rrdlabels_destroy(l);

    return errors;
}

#undef UT_EXPECT

struct pattern_array *trim_and_add_key_to_values(struct pattern_array *pa, const char *key, STRING *input);
static int rrdlabels_unittest_check_pattern_list(RRDLABELS *labels, const char *pattern, bool expected) {
    fprintf(stderr, "rrdlabels_match_simple_pattern(labels, \"%s\") ... ", pattern);

    STRING *str = string_strdupz(pattern);
    struct pattern_array *pa = trim_and_add_key_to_values(NULL, NULL, str);

    bool ret = pattern_array_label_match(pa, labels, '=', NULL);

    fprintf(stderr, "%s, got %s expected %s\n", (ret == expected)?"OK":"FAILED", ret?"true":"false", expected?"true":"false");

    string_freez(str);
    pattern_array_free(pa);

    return (ret == expected)?0:1;
}

static int rrdlabels_unittest_host_chart_labels() {
    fprintf(stderr, "\n%s() tests\n", __FUNCTION__);

    int errors = 0;

    RRDLABELS *labels = rrdlabels_create();
    rrdlabels_add(labels, "_hostname", "hostname1", RRDLABEL_SRC_CONFIG);
    rrdlabels_add(labels, "_os", "linux", RRDLABEL_SRC_CONFIG);
    rrdlabels_add(labels, "_distro", "ubuntu", RRDLABEL_SRC_CONFIG);

    // match a single key
    errors += rrdlabels_unittest_check_pattern_list(labels, "_hostname=*", true);
    errors += rrdlabels_unittest_check_pattern_list(labels, "_hostname=!*", false);

    // conflicting keys (some positive, some negative)
    errors += rrdlabels_unittest_check_pattern_list(labels, "_hostname=* _os=!*", false);
    errors += rrdlabels_unittest_check_pattern_list(labels, "_hostname=!* _os=*", false);

    // the user uses a key that is not there
    errors += rrdlabels_unittest_check_pattern_list(labels, "_not_a_key=*", false);
    errors += rrdlabels_unittest_check_pattern_list(labels, "_not_a_key=!*", false);
    errors += rrdlabels_unittest_check_pattern_list(labels, "_not_a_key=* _hostname=* _os=*", false);
    errors += rrdlabels_unittest_check_pattern_list(labels, "_not_a_key=!* _hostname=* _os=*", false);

    // positive and negative matches on the same key
    errors += rrdlabels_unittest_check_pattern_list(labels, "_hostname=!*invalid* !*bad* *name*", true);
    errors += rrdlabels_unittest_check_pattern_list(labels, "_hostname=*name* !*invalid* !*bad*", true);

    // positive and negative matches on the same key with catch all
    errors += rrdlabels_unittest_check_pattern_list(labels, "_hostname=!*invalid* !*bad* *", true);
    errors += rrdlabels_unittest_check_pattern_list(labels, "_hostname=* !*invalid* !*bad*", true);
    errors += rrdlabels_unittest_check_pattern_list(labels, "_hostname=!*invalid* !*name* *", false);
    errors += rrdlabels_unittest_check_pattern_list(labels, "_hostname=* !*invalid* !*name*", true);
    errors += rrdlabels_unittest_check_pattern_list(labels, "_hostname=*name* !*", true);

    errors += rrdlabels_unittest_check_pattern_list(labels, "_hostname=!*name* _os=l*", false);
    errors += rrdlabels_unittest_check_pattern_list(labels, "_os=l* hostname=!*name*", false);
    errors += rrdlabels_unittest_check_pattern_list(labels, "_hostname=*name* _hostname=*", true);
    errors += rrdlabels_unittest_check_pattern_list(labels, "_hostname=*name* _os=l*", true);
    errors += rrdlabels_unittest_check_pattern_list(labels, "_os=l* _hostname=*name*", true);

    rrdlabels_destroy(labels);

    return errors;
}

static int rrdlabels_unittest_check_simple_pattern(RRDLABELS *labels, const char *pattern, bool expected) {
    fprintf(stderr, "rrdlabels_match_simple_pattern(labels, \"%s\") ... ", pattern);

    bool ret = rrdlabels_match_simple_pattern(labels, pattern);
    fprintf(stderr, "%s, got %s expected %s\n", (ret == expected)?"OK":"FAILED", ret?"true":"false", expected?"true":"false");

    return (ret == expected)?0:1;
}

static int rrdlabels_unittest_simple_pattern() {
    fprintf(stderr, "\n%s() tests\n", __FUNCTION__);

    int errors = 0;

    RRDLABELS *labels = rrdlabels_create();
    rrdlabels_add(labels, "tag1", "value1", RRDLABEL_SRC_CONFIG);
    rrdlabels_add(labels, "tag2", "value2", RRDLABEL_SRC_CONFIG);
    rrdlabels_add(labels, "tag3", "value3", RRDLABEL_SRC_CONFIG);

    errors += rrdlabels_unittest_check_simple_pattern(labels, "*", true);
    errors += rrdlabels_unittest_check_simple_pattern(labels, "tag", false);
    errors += rrdlabels_unittest_check_simple_pattern(labels, "tag*", true);
    errors += rrdlabels_unittest_check_simple_pattern(labels, "*1", true);
    errors += rrdlabels_unittest_check_simple_pattern(labels, "value*", false);
    errors += rrdlabels_unittest_check_simple_pattern(labels, "*=value*", true);
    errors += rrdlabels_unittest_check_simple_pattern(labels, "*:value*", true);
    errors += rrdlabels_unittest_check_simple_pattern(labels, "*2", true);
    errors += rrdlabels_unittest_check_simple_pattern(labels, "*2 *3", true);
    errors += rrdlabels_unittest_check_simple_pattern(labels, "!tag3 *2", true);
    errors += rrdlabels_unittest_check_simple_pattern(labels, "tag1 tag2", true);
    errors += rrdlabels_unittest_check_simple_pattern(labels, "tag1tag2", false);
    errors += rrdlabels_unittest_check_simple_pattern(labels, "invalid1 invalid2 tag3", true);
    errors += rrdlabels_unittest_check_simple_pattern(labels, "!tag1 tag4", false);
    errors += rrdlabels_unittest_check_simple_pattern(labels, "tag1=value1", true);
    errors += rrdlabels_unittest_check_simple_pattern(labels, "tag1=value2", false);
    errors += rrdlabels_unittest_check_simple_pattern(labels, "tag*=value*", true);
    errors += rrdlabels_unittest_check_simple_pattern(labels, "!tag*=value*", false);
    errors += rrdlabels_unittest_check_simple_pattern(labels, "!tag2=something2 tag2=*2", true);

    rrdlabels_destroy(labels);

    return errors;
}

int rrdlabels_unittest_sanitize_value(const char *src, const char *expected) {
    char buf[RRDLABELS_MAX_VALUE_LENGTH + 1];
    size_t len = rrdlabels_sanitize_value(buf, src, RRDLABELS_MAX_VALUE_LENGTH);
    size_t expected_len = strlen(expected);

    int err = 0;
    if(strcmp(buf, expected) != 0) err = 1;
    if(len != expected_len) err = 1;

    fprintf(stderr, "%s(%s): %s, expected '%s', got '%s', expected bytes = %zu, got bytes = %zu\n", __FUNCTION__, src, (err==1)?"FAILED":"OK", expected, buf, expected_len, strlen(buf));
    return err;
}

int rrdlabels_unittest_sanitization() {
    int errors = 0;

    errors += rrdlabels_unittest_sanitize_value("", "[none]");
    errors += rrdlabels_unittest_sanitize_value("1", "1");
    errors += rrdlabels_unittest_sanitize_value("  hello   world   ", "hello world");
    errors += rrdlabels_unittest_sanitize_value("[none]", "[none]");

    // 2-byte UTF-8
    errors += rrdlabels_unittest_sanitize_value(" Ελλάδα ", "Ελλάδα");
    errors += rrdlabels_unittest_sanitize_value("aŰbŲcŴ", "aŰbŲcŴ");
    errors += rrdlabels_unittest_sanitize_value("Ű b Ų c Ŵ", "Ű b Ų c Ŵ");

    // 3-byte UTF-8
    errors += rrdlabels_unittest_sanitize_value("‱", "‱");
    errors += rrdlabels_unittest_sanitize_value("a‱b", "a‱b");
    errors += rrdlabels_unittest_sanitize_value("a ‱ b", "a ‱ b");

    // 4-byte UTF-8
    errors += rrdlabels_unittest_sanitize_value("𩸽", "𩸽");
    errors += rrdlabels_unittest_sanitize_value("a𩸽b", "a𩸽b");
    errors += rrdlabels_unittest_sanitize_value("a 𩸽 b", "a 𩸽 b");

    // mixed multi-byte
    errors += rrdlabels_unittest_sanitize_value("Ű‱𩸽‱Ű", "Ű‱𩸽‱Ű");

    // invalid UTF8 No 1
    const unsigned char invalid1[] = { 0xC3, 0x28, 'A', 'B', 0x0 };
    errors += rrdlabels_unittest_sanitize_value((const char *)invalid1, "c3(AB");

    // invalid UTF8 No 2
    const unsigned char invalid2[] = { 'A', 'B', 0xC3, 0x28, 'C', 'D', 0x0 };
    errors += rrdlabels_unittest_sanitize_value((const char *)invalid2, "ABc3(CD");

    // invalid UTF8 No 3
    const unsigned char invalid3[] = { 'A', 'B', 0xC3, 0x28, 0x0 };
    errors += rrdlabels_unittest_sanitize_value((const char *)invalid3, "ABc3(");

    // invalid UTF8 No 4
    const unsigned char invalid4[] = "clewd修改\xe7\x89";
    errors += rrdlabels_unittest_sanitize_value((const char *)invalid4, "clewd修改e789");

    // invalid UTF8 No 5
    const unsigned char invalid5[] = "app.clewd修改\xe7\x89_fd_open_limits";
    errors += rrdlabels_unittest_sanitize_value((const char *)invalid5, "app.clewd修改e789_fd_open_limits");

    // invalid UTF8 No 6
    const unsigned char invalid6[] = "\260\327\312\300\322\242";
    errors += rrdlabels_unittest_sanitize_value((const char *)invalid6, "d7cac0Ң");

    return errors;
}

int rrdlabels_unittest(void) {
    int errors = 0;

    errors += rrdlabels_unittest_sanitization();
    errors += rrdlabels_unittest_add_pairs();
    errors += rrdlabels_unittest_simple_pattern();
    errors += rrdlabels_unittest_host_chart_labels();
    errors += rrdlabels_unittest_double_check();
    errors += rrdlabels_unittest_migrate_check();
    errors += rrdlabels_unittest_mark_source_as_old();
    errors += rrdlabels_unittest_change_detection();
    errors += rrdlabels_unittest_pattern_check();

    fprintf(stderr, "%d errors found\n", errors);

    // string_destroy();
    return errors;
}
