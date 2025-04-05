// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_PATTERN_ARRAY_H
#define NETDATA_PATTERN_ARRAY_H

#include "libnetdata/libnetdata.h"
#include "rrdlabels.h"

struct pattern_array_item {
    Word_t size;
    Pvoid_t JudyL;
};

struct pattern_array {
    Word_t key_count;
    Pvoid_t JudyL;
};

struct pattern_array *pattern_array_allocate();
struct pattern_array *
pattern_array_add_key_value(struct pattern_array *pa, const char *key, const char *value, char sep);
bool pattern_array_label_match(
    struct pattern_array *pa,
    RRDLABELS *labels,
    char eq,
    size_t *searches);
struct pattern_array *pattern_array_add_simple_pattern(struct pattern_array *pa, SIMPLE_PATTERN *pattern, char sep);
struct pattern_array *
pattern_array_add_key_simple_pattern(struct pattern_array *pa, const char *key, SIMPLE_PATTERN *pattern);
void pattern_array_free(struct pattern_array *pa);

#endif //NETDATA_PATTERN_ARRAY_H
