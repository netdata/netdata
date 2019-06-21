// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_RRDENGINELIB_H
#define NETDATA_RRDENGINELIB_H

#include "rrdengine.h"

/* Forward declarations */
struct rrdeng_page_descr;

#define STR_HELPER(x) #x
#define STR(x) STR_HELPER(x)

#define BITS_PER_ULONG (sizeof(unsigned long) * 8)

#ifndef UUID_STR_LEN
#define UUID_STR_LEN (37)
#endif

/* Taken from linux kernel */
#define BUILD_BUG_ON(condition) ((void)sizeof(char[1 - 2*!!(condition)]))

#define ALIGN_BYTES_FLOOR(x) (((x) / RRDENG_BLOCK_SIZE) * RRDENG_BLOCK_SIZE)
#define ALIGN_BYTES_CEILING(x) ((((x) + RRDENG_BLOCK_SIZE - 1) / RRDENG_BLOCK_SIZE) * RRDENG_BLOCK_SIZE)

typedef uintptr_t rrdeng_stats_t;

#ifdef __ATOMIC_RELAXED
#define rrd_stat_atomic_add(p, n) do {(void) __atomic_fetch_add(p, n, __ATOMIC_RELAXED);} while(0)
#else
#define rrd_stat_atomic_add(p, n) do {(void) __sync_fetch_and_add(p, n);} while(0)
#endif

#define RRDENG_PATH_MAX (4096)

/* returns old *ptr value */
static inline unsigned long ulong_compare_and_swap(volatile unsigned long *ptr,
                                                   unsigned long oldval, unsigned long newval)
{
    return __sync_val_compare_and_swap(ptr, oldval, newval);
}

#ifndef O_DIRECT
/* Workaround for OS X */
#define O_DIRECT (0)
#endif

struct completion {
    uv_mutex_t mutex;
    uv_cond_t cond;
    volatile unsigned completed;
};

static inline void init_completion(struct completion *p)
{
    p->completed = 0;
    assert(0 == uv_cond_init(&p->cond));
    assert(0 == uv_mutex_init(&p->mutex));
}

static inline void destroy_completion(struct completion *p)
{
    uv_cond_destroy(&p->cond);
    uv_mutex_destroy(&p->mutex);
}

static inline void wait_for_completion(struct completion *p)
{
    uv_mutex_lock(&p->mutex);
    while (0 == p->completed) {
        uv_cond_wait(&p->cond, &p->mutex);
    }
    assert(1 == p->completed);
    uv_mutex_unlock(&p->mutex);
}

static inline void complete(struct completion *p)
{
    uv_mutex_lock(&p->mutex);
    p->completed = 1;
    uv_mutex_unlock(&p->mutex);
    uv_cond_broadcast(&p->cond);
}

static inline int crc32cmp(void *crcp, uLong crc)
{
    return (*(uint32_t *)crcp != crc);
}

static inline void crc32set(void *crcp, uLong crc)
{
    *(uint32_t *)crcp = crc;
}

extern void print_page_cache_descr(struct rrdeng_page_descr *page_cache_descr);
extern void print_page_descr(struct rrdeng_page_descr *descr);
extern int check_file_properties(uv_file file, uint64_t *file_size, size_t min_size);
extern int open_file_direct_io(char *path, int flags, uv_file *file);
extern char *get_rrdeng_statistics(struct rrdengine_instance *ctx, char *str, size_t size);

#endif /* NETDATA_RRDENGINELIB_H */