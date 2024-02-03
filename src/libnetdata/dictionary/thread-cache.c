// SPDX-License-Identifier: GPL-3.0-or-later

#include "thread-cache.h"

static __thread Pvoid_t thread_cache_judy_array = NULL;

void *thread_cache_entry_get_or_set(void *key,
                                    ssize_t key_length,
                                    void *value,
                                    void *(*transform_the_value_before_insert)(void *key, size_t key_length, void *value)
) {
    if(unlikely(!key || !key_length)) return NULL;

    if(key_length == -1)
        key_length = (ssize_t)strlen((char *)key);

    JError_t J_Error;
    Pvoid_t *Rc = JudyHSIns(&thread_cache_judy_array, key, key_length, &J_Error);
    if (unlikely(Rc == PJERR)) {
        fatal("THREAD_CACHE: Cannot insert entry to JudyHS, JU_ERRNO_* == %u, ID == %d",
              JU_ERRNO(&J_Error), JU_ERRID(&J_Error));
    }

    if(*Rc == 0) {
        // new item added

        *Rc = (transform_the_value_before_insert) ? transform_the_value_before_insert(key, key_length, value) : value;
    }

    return *Rc;
}

void thread_cache_destroy(void) {
    if(unlikely(!thread_cache_judy_array)) return;

    JError_t J_Error;
    Word_t ret = JudyHSFreeArray(&thread_cache_judy_array, &J_Error);
    if(unlikely(ret == (Word_t) JERR)) {
        netdata_log_error("THREAD_CACHE: Cannot destroy JudyHS, JU_ERRNO_* == %u, ID == %d",
                          JU_ERRNO(&J_Error), JU_ERRID(&J_Error));
    }

    internal_error(true, "THREAD_CACHE: hash table freed %lu bytes", ret);

    thread_cache_judy_array = NULL;
}

