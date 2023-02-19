// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_RRDENGINELIB_H
#define NETDATA_RRDENGINELIB_H

#include "libnetdata/libnetdata.h"

/* Forward declarations */
struct rrdengine_instance;

#define STR_HELPER(x) #x
#define STR(x) STR_HELPER(x)

#define BITS_PER_ULONG (sizeof(unsigned long) * 8)

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

static inline int crc32cmp(void *crcp, uLong crc)
{
    return (*(uint32_t *)crcp != crc);
}

static inline void crc32set(void *crcp, uLong crc)
{
    *(uint32_t *)crcp = crc;
}

int check_file_properties(uv_file file, uint64_t *file_size, size_t min_size);
int open_file_for_io(char *path, int flags, uv_file *file, int direct);
static inline int open_file_direct_io(char *path, int flags, uv_file *file)
{
    return open_file_for_io(path, flags, file, 1);
}
static inline int open_file_buffered_io(char *path, int flags, uv_file *file)
{
    return open_file_for_io(path, flags, file, 0);
}
int compute_multidb_diskspace();
int is_legacy_child(const char *machine_guid);

#endif /* NETDATA_RRDENGINELIB_H */
