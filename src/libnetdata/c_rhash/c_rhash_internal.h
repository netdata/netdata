// SPDX-License-Identifier: GPL-3.0-or-later

#include "c_rhash.h"

struct bin_item {
    uint8_t key_type:4;
    void *key;
    uint8_t value_type:4;
    void *value;

    struct bin_item *next;
};

typedef struct bin_item *c_rhash_bin;

struct c_rhash_s {
    size_t bin_count;
    c_rhash_bin *bins;
};
