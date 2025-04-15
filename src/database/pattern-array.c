// SPDX-License-Identifier: GPL-3.0-or-later

#include "pattern-array.h"

struct pattern_array *pattern_array_allocate()
{
    struct pattern_array *pa = callocz(1, sizeof(*pa));
    return pa;
}

void pattern_array_add_lblkey_with_sp(struct pattern_array *pa, const char *key, SIMPLE_PATTERN *sp)
{
    if (!pa || !key) {
        simple_pattern_free(sp);
        return;
    }

    STRING *string_key = string_strdupz(key);
    Pvoid_t *Pvalue = JudyLIns(&pa->JudyL, (Word_t) string_key, PJE0);
    if (!Pvalue || Pvalue == PJERR ) {
        string_freez(string_key);
        simple_pattern_free(sp);
        return;
    }

    struct pattern_array *pai;
    if (*Pvalue)
        string_freez(string_key);
    else
        *Pvalue = callocz(1, sizeof(*pai));

    pai = *Pvalue;

    Pvalue = JudyLIns(&pai->JudyL, (Word_t) ++pai->key_count, PJE0);
    if (!Pvalue || Pvalue == PJERR) {
        simple_pattern_free(sp);
        return;
    }

    *Pvalue = sp;
}

bool pattern_array_label_match(
    struct pattern_array *pa,
    RRDLABELS *labels,
    char eq,
    size_t *searches)
{
    if (!pa || !labels)
        return true;

    Pvoid_t *Pvalue;
    Word_t Index = 0;
    bool first_then_next = true;
    while ((Pvalue = JudyLFirstThenNext(pa->JudyL, &Index, &first_then_next))) {
        // for each label key in the pattern array

        struct pattern_array *pai = *Pvalue;
        SIMPLE_PATTERN_RESULT match = SP_NOT_MATCHED;
        Word_t Index2 = 0;
        bool first_then_next2 = true;
        while ((Pvalue = JudyLFirstThenNext(pai->JudyL, &Index2, &first_then_next2))) {
            // for each pattern in the label key pattern list
            if (!*Pvalue)
                continue;

            match = rrdlabels_match_simple_pattern_parsed(labels, (SIMPLE_PATTERN *)(*Pvalue), eq, searches);

            if (match != SP_NOT_MATCHED)
                break;
        }
        if (match != SP_MATCHED_POSITIVE)
            return false;
    }
    return true;
}

struct pattern_array *pattern_array_add_key_simple_pattern(struct pattern_array *pa, const char *key, SIMPLE_PATTERN *pattern)
{
    if (unlikely(!pattern || !key))
        return pa;

    if (!pa)
        pa = pattern_array_allocate();

    pattern_array_add_lblkey_with_sp(pa, key, pattern);
    return pa;
}

struct pattern_array *pattern_array_add_simple_pattern(struct pattern_array *pa, SIMPLE_PATTERN *pattern, char sep)
{
    if (unlikely(!pattern))
        return pa;

    if (!pa)
        pa = pattern_array_allocate();

    char *label_key;
    while (pattern && (label_key = simple_pattern_iterate(&pattern))) {
        char key[RRDLABELS_MAX_NAME_LENGTH + 1], *key_sep;

        if (unlikely(!label_key || !(key_sep = strchr(label_key, sep))))
            return pa;

        *key_sep = '\0';
        strncpyz(key, label_key, RRDLABELS_MAX_NAME_LENGTH);
        *key_sep = sep;

        pattern_array_add_lblkey_with_sp(pa, key, string_to_simple_pattern(label_key));
    }
    return pa;
}

struct pattern_array *pattern_array_add_key_value(struct pattern_array *pa, const char *key, const char *value, char sep)
{
    if (unlikely(!key || !value))
        return pa;

    if (!pa)
        pa = pattern_array_allocate();

    char label_key[RRDLABELS_MAX_NAME_LENGTH + RRDLABELS_MAX_VALUE_LENGTH + 2];
    snprintfz(label_key, sizeof(label_key) - 1, "%s%c%s", key, sep, value);
    pattern_array_add_lblkey_with_sp(
        pa, key, simple_pattern_create(label_key, SIMPLE_PATTERN_DEFAULT_WEB_SEPARATORS, SIMPLE_PATTERN_EXACT, true));
    return pa;
}

void pattern_array_free(struct pattern_array *pa)
{
    if (!pa)
        return;

    Pvoid_t *Pvalue;
    Word_t Index = 0;
    bool first = true;
    while ((Pvalue = JudyLFirstThenNext(pa->JudyL, &Index, &first))) {
        struct pattern_array *pai = *Pvalue;

        Word_t Index2 = 0;
        bool first2 = true;
        while ((Pvalue = JudyLFirstThenNext(pai->JudyL, &Index2, &first2))) {
            SIMPLE_PATTERN *sp = (SIMPLE_PATTERN *)*Pvalue;
            simple_pattern_free(sp);
        }

        JudyLFreeArray(&(pai->JudyL), PJE0);
        string_freez((STRING *)Index);
        freez(pai);
    }

    JudyLFreeArray(&(pa->JudyL), PJE0);
    freez(pa);
}

