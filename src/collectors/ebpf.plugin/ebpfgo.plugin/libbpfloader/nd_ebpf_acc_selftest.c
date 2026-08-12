//go:build netdata_ebpf_libbpf
// +build netdata_ebpf_libbpf

/* Self-test for the shared per-TGID accumulator.
 *
 * This exists because a missing nd_ebpf_acc_init() shipped once: the table was
 * left with item_size 0, so the first buffer/arena event wrote a TGID into a
 * zero-byte allocation and glibc aborted the plugin with
 * "malloc(): corrupted top size".  Compilation cannot catch that, so the
 * behaviour is asserted here instead and driven from Go by
 * nd_ebpf_acc_selftest_test.go. */

#include <stdint.h>
#include <string.h>

#include "nd_ebpf_runtime_common.h"

/* A stand-in for the per-module accumulator entries.  Same shape as the real
 * ones: a ct, the tgid the table keys on, then module counters. */
struct nd_ebpf_acc_selftest_entry {
    uint64_t ct;
    uint32_t tgid;
    uint32_t uid;
    uint32_t gid;
    char name[16];
    uint64_t counter;
};

/* Returns 0 on success, or a distinct positive code per failed assertion so a
 * failure names itself. */
int netdata_ebpf_acc_selftest(void)
{
    struct nd_ebpf_acc_table t;
    nd_ebpf_acc_init(&t, sizeof(struct nd_ebpf_acc_selftest_entry),
                     offsetof(struct nd_ebpf_acc_selftest_entry, tgid));

    if (t.item_size != sizeof(struct nd_ebpf_acc_selftest_entry))
        return 1;
    if (t.count != 0 || t.items != NULL)
        return 2;

    /* Insert enough TGIDs to force several growths and hash rebuilds. */
    const uint32_t total = 500;
    for (uint32_t i = 1; i <= total; i++) {
        struct nd_ebpf_acc_selftest_entry *e = nd_ebpf_acc_find_or_add(&t, i);
        if (!e)
            return 3;
        if (e->tgid != i)
            return 4;
        if (e->counter != 0 || e->ct != 0)
            return 5; /* a fresh entry must be zeroed */
        e->counter = i;
        e->ct = i * 10;
    }
    if (t.count != total)
        return 6;

    /* Every TGID must be found again, with its counters intact (no aliasing
     * across the growth/rebuild cycles). */
    for (uint32_t i = 1; i <= total; i++) {
        struct nd_ebpf_acc_selftest_entry *e = nd_ebpf_acc_find_or_add(&t, i);
        if (!e)
            return 7;
        if (e->counter != i || e->ct != i * 10)
            return 8;
    }
    if (t.count != total)
        return 9; /* re-finding must not append */

    /* Eviction: swap-with-last plus rebuild must keep every survivor findable. */
    nd_ebpf_acc_evict_tgid(&t, 1);          /* first */
    nd_ebpf_acc_evict_tgid(&t, total);      /* last */
    nd_ebpf_acc_evict_tgid(&t, total / 2);  /* middle */
    if (t.count != total - 3)
        return 10;

    for (uint32_t i = 2; i < total; i++) {
        if (i == total / 2)
            continue;
        struct nd_ebpf_acc_selftest_entry *e = nd_ebpf_acc_find_or_add(&t, i);
        if (!e || e->counter != i)
            return 11;
    }
    if (t.count != total - 3)
        return 12;

    /* Evicting an absent TGID is a no-op, not a corruption. */
    nd_ebpf_acc_evict_tgid(&t, 0xDEADBEEF);
    if (t.count != total - 3)
        return 13;

    nd_ebpf_acc_free(&t);
    if (t.items != NULL || t.htable != NULL || t.count != 0)
        return 14;

    /* The regression itself: an uninitialised table must refuse the write
     * instead of corrupting the heap. */
    struct nd_ebpf_acc_table uninit;
    memset(&uninit, 0, sizeof(uninit));
    if (nd_ebpf_acc_find_or_add(&uninit, 1234) != NULL)
        return 15;

    return 0;
}
