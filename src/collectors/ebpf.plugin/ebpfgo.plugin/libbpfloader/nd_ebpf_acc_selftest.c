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

#include <fcntl.h>
#include <stdint.h>
#include <string.h>
#include <unistd.h>

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

/* Snapshot-shaped item for the merge test: pid first, as
 * nd_ebpf_snapshot_merge_same_pid() requires. */
struct nd_ebpf_snapshot_selftest_item {
    uint32_t pid;
    uint64_t ct;
    uint64_t counter;
    char comm[16];
};
ND_EBPF_ASSERT_PID_FIRST(struct nd_ebpf_snapshot_selftest_item);

static void nd_ebpf_snapshot_selftest_merge(void *dst_v, const void *src_v)
{
    struct nd_ebpf_snapshot_selftest_item *dst = dst_v;
    const struct nd_ebpf_snapshot_selftest_item *src = src_v;

    if (src->ct > dst->ct)
        dst->ct = src->ct;
    dst->counter += src->counter;
    if (!dst->comm[0] && src->comm[0])
        nd_ebpf_copy_comm(dst->comm, sizeof(dst->comm), src->comm, sizeof(src->comm));
}

/* Exercises the shared per-thread row collapse the map walk depends on.
 * Returns 0 on success or a 3x code. */
static int nd_ebpf_snapshot_merge_selftest(void)
{
    /* Unsorted, with three rows for pid 7 and two for pid 3 — the per-thread
     * case the map walk produces. */
    struct nd_ebpf_snapshot_selftest_item items[] = {
        {7, 100, 5, ""},
        {3, 500, 1, "three"},
        {7, 300, 5, "seven"},
        {9, 50, 2, "nine"},
        {3, 400, 1, ""},
        {7, 200, 5, ""},
    };

    size_t n = nd_ebpf_snapshot_merge_same_pid(
        items, sizeof(items) / sizeof(items[0]), sizeof(items[0]),
        nd_ebpf_snapshot_selftest_merge);

    if (n != 3)
        return 30;
    /* Ascending by pid after the merge. */
    if (items[0].pid != 3 || items[1].pid != 7 || items[2].pid != 9)
        return 31;
    /* Counters summed across the collapsed rows. */
    if (items[0].counter != 2 || items[1].counter != 15 || items[2].counter != 2)
        return 32;
    /* ct is the maximum, not the last seen. */
    if (items[0].ct != 500 || items[1].ct != 300 || items[2].ct != 50)
        return 33;
    /* A name found on any row survives, even when the surviving row was empty. */
    if (items[1].comm[0] != 's')
        return 34;

    /* Fewer than two rows is a no-op, not a corruption. */
    struct nd_ebpf_snapshot_selftest_item one = {42, 1, 1, "x"};
    if (nd_ebpf_snapshot_merge_same_pid(&one, 1, sizeof(one), nd_ebpf_snapshot_selftest_merge) != 1)
        return 35;
    if (one.pid != 42 || one.counter != 1)
        return 36;

    return 0;
}

static int acc_selftest_body(void)
{
    int rc = 0;
    struct nd_ebpf_acc_table t;
    nd_ebpf_acc_init(&t, sizeof(struct nd_ebpf_acc_selftest_entry),
                     offsetof(struct nd_ebpf_acc_selftest_entry, tgid));

    if (t.item_size != sizeof(struct nd_ebpf_acc_selftest_entry)) {
        rc = 1;
        goto cleanup;
    }
    if (t.count != 0 || t.items != NULL) {
        rc = 2;
        goto cleanup;
    }

    /* Insert enough TGIDs to force several growths and hash rebuilds. */
    const uint32_t total = 500;
    for (uint32_t i = 1; i <= total; i++) {
        struct nd_ebpf_acc_selftest_entry *e = nd_ebpf_acc_find_or_add(&t, i);
        if (!e) {
            rc = 3;
            goto cleanup;
        }
        if (e->tgid != i) {
            rc = 4;
            goto cleanup;
        }
        if (e->counter != 0 || e->ct != 0) {
            rc = 5; /* a fresh entry must be zeroed */
            goto cleanup;
        }
        e->counter = i;
        e->ct = i * 10;
    }
    if (t.count != total) {
        rc = 6;
        goto cleanup;
    }

    /* Every TGID must be found again, with its counters intact (no aliasing
     * across the growth/rebuild cycles). */
    for (uint32_t i = 1; i <= total; i++) {
        struct nd_ebpf_acc_selftest_entry *e = nd_ebpf_acc_find_or_add(&t, i);
        if (!e) {
            rc = 7;
            goto cleanup;
        }
        if (e->counter != i || e->ct != i * 10) {
            rc = 8;
            goto cleanup;
        }
    }
    if (t.count != total) {
        rc = 9; /* re-finding must not append */
        goto cleanup;
    }

    /* Eviction: swap-with-last plus rebuild must keep every survivor findable. */
    nd_ebpf_acc_evict_tgid(&t, 1);          /* first */
    nd_ebpf_acc_evict_tgid(&t, total);      /* last */
    nd_ebpf_acc_evict_tgid(&t, total / 2);  /* middle */
    if (t.count != total - 3) {
        rc = 10;
        goto cleanup;
    }

    for (uint32_t i = 2; i < total; i++) {
        if (i == total / 2)
            continue;
        struct nd_ebpf_acc_selftest_entry *e = nd_ebpf_acc_find_or_add(&t, i);
        if (!e || e->counter != i) {
            rc = 11;
            goto cleanup;
        }
    }
    if (t.count != total - 3) {
        rc = 12;
        goto cleanup;
    }

    /* Evicting an absent TGID is a no-op, not a corruption. */
    nd_ebpf_acc_evict_tgid(&t, 0xDEADBEEF);
    if (t.count != total - 3) {
        rc = 13;
        goto cleanup;
    }

    nd_ebpf_acc_set_max_entries(&t, t.count);
    struct nd_ebpf_acc_selftest_entry *existing = nd_ebpf_acc_find_or_add(&t, 2);
    if (!existing || existing->counter != 2 || t.count != total - 3 || t.dropped != 0) {
        rc = 14;
        goto cleanup;
    }
    if (nd_ebpf_acc_find_or_add(&t, total + 1) != NULL || t.count != total - 3 || t.dropped != 1) {
        rc = 15;
        goto cleanup;
    }

cleanup:
    nd_ebpf_acc_free(&t);
    if (rc)
        return rc;

    if (t.items != NULL || t.htable != NULL || t.count != 0)
        return 16;

    /* The regression itself: an uninitialised table must refuse the write
     * instead of corrupting the heap. */
    struct nd_ebpf_acc_table uninit;
    memset(&uninit, 0, sizeof(uninit));
    if (nd_ebpf_acc_find_or_add(&uninit, 1234) != NULL)
        return 17;

    /* The shared map-walk helper cachestat and dcstat both route through. */
    int merge_rc = nd_ebpf_snapshot_merge_selftest();
    if (merge_rc)
        return merge_rc;

    return 0;
}

/* Two of the assertions above deliberately drive paths that log a diagnostic:
 * the per-TGID accumulator cap, and the uninitialised-table guard.  Those
 * messages are the expected result here, but a passing run then prints
 * error-looking lines that read as failures in CI output.  Silence stderr for the
 * duration; the verdict travels in the return code, never on stderr, so nothing
 * is lost.  If the redirect cannot be set up the test still runs, just noisily. */
int netdata_ebpf_acc_selftest(void)
{
    fflush(stderr);
    int saved = dup(STDERR_FILENO);
    int devnull = open("/dev/null", O_WRONLY | O_CLOEXEC);
    if (saved >= 0 && devnull >= 0)
        dup2(devnull, STDERR_FILENO);

    int rc = acc_selftest_body();

    fflush(stderr);
    if (saved >= 0) {
        dup2(saved, STDERR_FILENO);
        close(saved);
    }
    if (devnull >= 0)
        close(devnull);

    return rc;
}
