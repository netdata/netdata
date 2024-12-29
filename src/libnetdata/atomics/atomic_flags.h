// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_ATOMIC_FLAGS_H
#define NETDATA_ATOMIC_FLAGS_H

#define atomic_flags_get(pvalue) \
    __atomic_load_n((pvalue), __ATOMIC_ACQUIRE)

#define atomic_flags_check(pvalue, flag) \
    (__atomic_load_n((pvalue), __ATOMIC_ACQUIRE) & (flag))

#define atomic_flags_set(pvalue, flag) \
    __atomic_or_fetch((pvalue), (flag), __ATOMIC_RELEASE)

#define atomic_flags_clear(pvalue, flag) \
    __atomic_and_fetch((pvalue), ~(flag), __ATOMIC_RELEASE)

// returns the old flags (before applying the changes)
#define atomic_flags_set_and_clear(pvalue, set, clear) ({                    \
    __typeof__(*pvalue) __old = *(pvalue), __new;                            \
    do {                                                                     \
        __new = (__old | (set)) & ~(clear);                                  \
    } while(!__atomic_compare_exchange_n((pvalue), &__old, __new,            \
            false, __ATOMIC_RELEASE, __ATOMIC_RELAXED));                     \
    __old;                                                                   \
})

#endif //NETDATA_ATOMIC_FLAGS_H
