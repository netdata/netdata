// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_LOG2JOURNAL_TXT_H
#define NETDATA_LOG2JOURNAL_TXT_H

#include "log2journal.h"

// ----------------------------------------------------------------------------
// A dynamically sized, reusable text buffer,
// allowing us to be fast (no allocations during iterations) while having the
// smallest possible allocations.

typedef struct txt_l2j {
    char *txt;
    uint32_t size;
    uint32_t len;
} TXT_L2J;

static inline void txt_l2j_cleanup(TXT_L2J *t) {
    if(!t)
        return;

    if(t->txt)
        freez(t->txt);

    t->txt = NULL;
    t->size = 0;
    t->len = 0;
}

#define TXT_L2J_ALLOC_ALIGN 1024

static inline size_t txt_l2j_compute_new_size(size_t old_size, size_t required_size) {
    size_t size = (required_size % TXT_L2J_ALLOC_ALIGN == 0) ? required_size : required_size + TXT_L2J_ALLOC_ALIGN;
    size = (size / TXT_L2J_ALLOC_ALIGN) * TXT_L2J_ALLOC_ALIGN;

    if(size < old_size * 2)
        size = old_size * 2;

    return size;
}

static inline void txt_l2j_resize(TXT_L2J *dst, size_t required_size, bool keep) {
    if(required_size <= dst->size)
        return;

    size_t new_size = txt_l2j_compute_new_size(dst->size, required_size);

    if(keep && dst->txt)
        dst->txt = reallocz(dst->txt, new_size);
    else {
        txt_l2j_cleanup(dst);
        dst->txt = mallocz(new_size);
        dst->len = 0;
    }

    dst->size = new_size;
}

static inline void txt_l2j_set(TXT_L2J *dst, const char *s, int32_t len) {
    if(!s || !*s || len == 0) {
        s = "";
        len = 0;
    }

    if(len == -1)
        len = (int32_t)strlen(s);

    txt_l2j_resize(dst, len + 1, false);
    memcpy(dst->txt, s, len);
    dst->txt[len] = '\0';
    dst->len = len;
}

static inline void txt_l2j_append(TXT_L2J *dst, const char *s, int32_t len) {
    if(!dst->txt || !dst->len)
        txt_l2j_set(dst, s, len);

    else {
        if(len == -1)
            len = (int32_t)strlen(s);

        txt_l2j_resize(dst, dst->len + len + 1, true);
        memcpy(&dst->txt[dst->len], s, len);
        dst->len += len;
        dst->txt[dst->len] = '\0';
    }
}

#endif //NETDATA_LOG2JOURNAL_TXT_H
