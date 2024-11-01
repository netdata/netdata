// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_LOG2JOURNAL_HASHED_KEY_H
#define NETDATA_LOG2JOURNAL_HASHED_KEY_H

#include "log2journal.h"

typedef enum __attribute__((__packed__)) {
    HK_NONE                 = 0,

    // permanent flags - they are set once to optimize various decisions and lookups

    HK_HASHTABLE_ALLOCATED  = (1 << 0), // this is the key object allocated in the hashtable
                                        // objects that do not have this, have a pointer to a key in the hashtable
                                        // objects that have this, value is allocated

    HK_FILTERED             = (1 << 1), // we checked once if this key in filtered
    HK_FILTERED_INCLUDED    = (1 << 2), // the result of the filtering was to include it in the output

    HK_COLLISION_CHECKED    = (1 << 3), // we checked once for collision check of this key

    HK_RENAMES_CHECKED      = (1 << 4), // we checked once if there are renames on this key
    HK_HAS_RENAMES          = (1 << 5), // and we found there is a rename rule related to it

    // ephemeral flags - they are unset at the end of each log line

    HK_VALUE_FROM_LOG       = (1 << 14), // the value of this key has been read from the log (or from injection, duplication)
    HK_VALUE_REWRITTEN      = (1 << 15), // the value of this key has been rewritten due to one of our rewrite rules

} HASHED_KEY_FLAGS;

typedef struct hashed_key {
    const char *key;
    uint32_t len;
    HASHED_KEY_FLAGS flags;
    XXH64_hash_t hash;
    union {
        struct hashed_key *hashtable_ptr;   // HK_HASHTABLE_ALLOCATED is not set
        TXT_L2J value;                      // HK_HASHTABLE_ALLOCATED is set
    };
} HASHED_KEY;

static inline void hashed_key_cleanup(HASHED_KEY *k) {
    if(k->flags & HK_HASHTABLE_ALLOCATED)
        txt_l2j_cleanup(&k->value);
    else
        k->hashtable_ptr = NULL;

    freez((void *)k->key);
    k->key = NULL;
    k->len = 0;
    k->hash = 0;
    k->flags = HK_NONE;
}

static inline void hashed_key_set(HASHED_KEY *k, const char *name, int32_t len) {
    hashed_key_cleanup(k);

    if(len == -1) {
        k->key = strdupz(name);
        k->len = strlen(k->key);
    }
    else {
        k->key = strndupz(name, len);
        k->len = len;
    }

    k->hash = XXH3_64bits(k->key, k->len);
    k->flags = HK_NONE;
}

static inline bool hashed_keys_match(HASHED_KEY *k1, HASHED_KEY *k2) {
    return ((k1 == k2) || (k1->hash == k2->hash && strcmp(k1->key, k2->key) == 0));
}

static inline int compare_keys(struct hashed_key *k1, struct hashed_key *k2) {
    return strcmp(k1->key, k2->key);
}

#endif //NETDATA_LOG2JOURNAL_HASHED_KEY_H
