// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_THREAD_CACHE_H
#define NETDATA_THREAD_CACHE_H

#include "../libnetdata.h"

void *thread_cache_entry_get_or_set(void *key,
                                    ssize_t key_length,
                                    void *value,
                                    void *(*transform_the_value_before_insert)(void *key, size_t key_length, void *value));

void thread_cache_destroy(void);

#endif //NETDATA_THREAD_CACHE_H
