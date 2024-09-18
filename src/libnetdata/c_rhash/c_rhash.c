// SPDX-License-Identifier: GPL-3.0-or-later

#include "../libnetdata.h"
#include "c_rhash_internal.h"

c_rhash c_rhash_new(size_t bin_count) {
    if (!bin_count)
        bin_count = 1000;

    c_rhash hash = callocz(1, sizeof(struct c_rhash_s) + (bin_count * sizeof(struct bin_ll*)) );
    hash->bin_count = bin_count;
    hash->bins = (c_rhash_bin *)((char*)hash + sizeof(struct c_rhash_s));

    return hash;
}

static size_t get_itemtype_len(uint8_t item_type, const void* item_data) {
    switch (item_type) {
        case ITEMTYPE_STRING:
            return strlen(item_data) + 1;
        case ITEMTYPE_UINT64:
            return sizeof(uint64_t);
        case ITEMTYPE_UINT8:
            return 1;
        case ITEMTYPE_OPAQUE_PTR:
            return sizeof(void*);
        default:
            return 0;
    }
}

static int compare_bin_item(struct bin_item *item, uint8_t key_type, const void *key) {
    if (item->key_type != key_type)
        return 1;

    size_t key_value_len = get_itemtype_len(key_type, key);

    if(key_type == ITEMTYPE_STRING) {
        size_t new_key_value_len = get_itemtype_len(item->key_type, item->key);
        if (new_key_value_len != key_value_len)
            return 1;
    }

    if(memcmp(item->key, key, key_value_len) == 0) {
        return 0;
    }

    return 1;
}

static int insert_into_bin(c_rhash_bin *bin, uint8_t key_type, const void *key, uint8_t value_type, const void *value) {
    struct bin_item *prev = NULL;
    while (*bin != NULL) {
        if (!compare_bin_item(*bin, key_type, key)) {
            freez((*bin)->value);
            (*bin)->value_type = value_type;
            (*bin)->value = mallocz(get_itemtype_len(value_type, value));
            memcpy((*bin)->value, value, get_itemtype_len(value_type, value));
            return 0;
        }
        prev = *bin;
        bin = &(*bin)->next;
    }

    if (*bin == NULL)
        *bin = callocz(1, sizeof(struct bin_item));
    if (prev != NULL)
        prev->next = *bin;

    (*bin)->key_type = key_type;
    size_t len = get_itemtype_len(key_type, key);
    (*bin)->key = mallocz(len);
    memcpy((*bin)->key, key, len);

    (*bin)->value_type = value_type;
    len = get_itemtype_len(value_type, value);
    (*bin)->value = mallocz(len);
    memcpy((*bin)->value, value, len);
    return 0;
}

static inline uint32_t get_bin_idx_str(c_rhash hash, const char *key) {
    uint32_t nhash = simple_hash(key);
    return nhash % hash->bin_count;
}

static inline c_rhash_bin *get_binptr_by_str(c_rhash hash, const char *key) {
    return &hash->bins[get_bin_idx_str(hash, key)];
}

int c_rhash_insert_str_ptr(c_rhash hash, const char *key, void *value) {
    c_rhash_bin *bin = get_binptr_by_str(hash, key);

    return insert_into_bin(bin, ITEMTYPE_STRING, key, ITEMTYPE_OPAQUE_PTR, &value);
}

int c_rhash_insert_str_uint8(c_rhash hash, const char *key, uint8_t value) {
    c_rhash_bin *bin = get_binptr_by_str(hash, key);

    return insert_into_bin(bin, ITEMTYPE_STRING, key, ITEMTYPE_UINT8, &value);
}

int c_rhash_insert_uint64_ptr(c_rhash hash, uint64_t key, void *value) {
    c_rhash_bin *bin = &hash->bins[key % hash->bin_count];

    return insert_into_bin(bin, ITEMTYPE_UINT64, &key, ITEMTYPE_OPAQUE_PTR, &value);
}

int c_rhash_get_uint8_by_str(c_rhash hash, const char *key, uint8_t *ret_val) {
    uint32_t nhash = get_bin_idx_str(hash, key);

    struct bin_item *bin = hash->bins[nhash];

    while (bin) {
        if (bin->key_type == ITEMTYPE_STRING) {
            if (!strcmp(bin->key, key)) {
                *ret_val = *(uint8_t*)bin->value;
                return 0;
            }
        }
        bin = bin->next;
    }
    return 1;
}

int c_rhash_get_ptr_by_str(c_rhash hash, const char *key, void **ret_val) {
    uint32_t nhash = get_bin_idx_str(hash, key);

    struct bin_item *bin = hash->bins[nhash];

    while (bin) {
        if (bin->key_type == ITEMTYPE_STRING) {
            if (!strcmp(bin->key, key)) {
                *ret_val = *((void**)bin->value);
                return 0;
            }
        }
        bin = bin->next;
    }
    *ret_val = NULL;
    return 1;
}

int c_rhash_get_ptr_by_uint64(c_rhash hash, uint64_t key, void **ret_val) {
    uint32_t nhash = key % hash->bin_count;

    struct bin_item *bin = hash->bins[nhash];

    while (bin) {
        if (bin->key_type == ITEMTYPE_UINT64) {
            if (*((uint64_t *)bin->key) == key) {
                *ret_val = *((void**)bin->value);
                return 0;
            }
        }
        bin = bin->next;
    }
    *ret_val = NULL;
    return 1;
}

static void c_rhash_destroy_bin(c_rhash_bin bin) {
    struct bin_item *next;
    do {
        next = bin->next;
        freez(bin->key);
        freez(bin->value);
        freez(bin);
        bin = next;
    } while (bin != NULL);
}

int c_rhash_iter_uint64_keys(c_rhash hash, c_rhash_iter_t *iter, uint64_t *key) {
    while (iter->bin < hash->bin_count) {
        if (iter->item != NULL)
            iter->item = iter->item->next;
        if (iter->item == NULL) {
            if (iter->initialized)
                iter->bin++;
            else
                iter->initialized = 1;
            if (iter->bin < hash->bin_count)
                iter->item = hash->bins[iter->bin];
        }
        if (iter->item != NULL && iter->item->key_type == ITEMTYPE_UINT64) {
            *key = *(uint64_t*)iter->item->key;
            return 0;
        }
    }
    return 1;
}

int c_rhash_iter_str_keys(c_rhash hash, c_rhash_iter_t *iter, const char **key) {
    while (iter->bin < hash->bin_count) {
        if (iter->item != NULL)
            iter->item = iter->item->next;
        if (iter->item == NULL) {
            if (iter->initialized)
                iter->bin++;
            else
                iter->initialized = 1;
            if (iter->bin < hash->bin_count)
                iter->item = hash->bins[iter->bin];
        }
        if (iter->item != NULL && iter->item->key_type == ITEMTYPE_STRING) {
            *key = (const char*)iter->item->key;
            return 0;
        }
    }
    return 1;
}

void c_rhash_destroy(c_rhash hash) {
    for (size_t i = 0; i < hash->bin_count; i++) {
        if (hash->bins[i] != NULL)
            c_rhash_destroy_bin(hash->bins[i]);
    }
    freez(hash);
}
