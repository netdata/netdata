// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_RRDENGINELIB_H
#define NETDATA_RRDENGINELIB_H

#include "libnetdata/libnetdata.h"

/* Forward declarations */
struct rrdeng_page_descr;
struct rrdengine_instance;

#define STR_HELPER(x) #x
#define STR(x) STR_HELPER(x)

#define BITS_PER_ULONG (sizeof(unsigned long) * 8)

/* Taken from linux kernel */
#define BUILD_BUG_ON(condition) ((void)sizeof(char[1 - 2*!!(condition)]))

#define ALIGN_BYTES_FLOOR(x) (((x) / RRDENG_BLOCK_SIZE) * RRDENG_BLOCK_SIZE)
#define ALIGN_BYTES_CEILING(x) ((((x) + RRDENG_BLOCK_SIZE - 1) / RRDENG_BLOCK_SIZE) * RRDENG_BLOCK_SIZE)

#define ROUND_USEC_TO_SEC(x) (((x) + USEC_PER_SEC / 2 - 1) / USEC_PER_SEC)

typedef uintptr_t rrdeng_stats_t;

#ifdef __ATOMIC_RELAXED
#define rrd_atomic_fetch_add(p, n) __atomic_fetch_add(p, n, __ATOMIC_RELAXED)
#define rrd_atomic_add_fetch(p, n) __atomic_add_fetch(p, n, __ATOMIC_RELAXED)
#else
#define rrd_atomic_fetch_add(p, n) __sync_fetch_and_add(p, n)
#define rrd_atomic_add_fetch(p, n) __sync_add_and_fetch(p, n)
#endif

#define rrd_stat_atomic_add(p, n) rrd_atomic_fetch_add(p, n)

/* returns -1 if it didn't find the first cleared bit, the position otherwise. Starts from LSB. */
static inline int find_first_zero(unsigned x)
{
    return ffs((int)(~x)) - 1;
}

/* Starts from LSB. */
static inline uint8_t check_bit(unsigned x, size_t pos)
{
    return !!(x & (1 << pos));
}

/* Starts from LSB. val is 0 or 1 */
static inline void modify_bit(unsigned *x, unsigned pos, uint8_t val)
{
    switch(val) {
    case 0:
        *x &= ~(1U << pos);
        break;
    case 1:
        *x |= 1U << pos;
        break;
    default:
        error("modify_bit() called with invalid argument.");
        break;
    }
}

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
    fatal_assert(0 == uv_cond_init(&p->cond));
    fatal_assert(0 == uv_mutex_init(&p->mutex));
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
    fatal_assert(1 == p->completed);
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
extern int open_file_for_io(char *path, int flags, uv_file *file, int direct);
static inline int open_file_direct_io(char *path, int flags, uv_file *file)
{
    return open_file_for_io(path, flags, file, 1);
}
static inline int open_file_buffered_io(char *path, int flags, uv_file *file)
{
    return open_file_for_io(path, flags, file, 0);
}
extern char *get_rrdeng_statistics(struct rrdengine_instance *ctx, char *str, size_t size);
extern int compute_multidb_diskspace();
extern int is_legacy_child(const char *machine_guid);

#endif /* NETDATA_RRDENGINELIB_H */
