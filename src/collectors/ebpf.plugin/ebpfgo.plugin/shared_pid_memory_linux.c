// SPDX-License-Identifier: GPL-3.0-or-later

#include "shared_pid_memory.h"
#include "nd_alloc_shim.h"

#include <errno.h>
#include <fcntl.h>
#include <signal.h>
#include <stdbool.h>
#include <stddef.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <sys/mman.h>
#include <sys/stat.h>
#include <time.h>
#include <unistd.h>

/* Maximum consecutive replace_generation() attempts before giving up permanently. */
#define SHM_PID_REPLACE_MAX_RETRIES 10

struct shared_pid_memory {
    void *mapping;                    /* raw mmap base (for munmap and header access) */
    struct ebpfgo_shm_header *header; /* = mapping; holds per-module validity flags */
    struct ebpf_pid_stat *entries;    /* = (char*)mapping + sizeof(*header) */
    size_t total;
    size_t prev_count; /* entries written in the previous publish cycle */
    uint32_t update_every_s;
    uint32_t publish_timeouts; /* consecutive publish-side sem_timedwait failures */
    uint8_t replace_fail_count; /* consecutive replace_generation() failures after mapping loss */
    int shm_fd;
    sem_t *sem;
    bool shm_name_created;  /* set when this context created/recreated the SHM name; triggers unlink on any exit */
    bool shm_eexist_backoff; /* set when replace_generation backed off on EEXIST; skips shm_unlink on next retry */
    char *shm_name;        /* owned copy of the POSIX SHM object name */
    char *sem_name;        /* owned copy of the POSIX semaphore name */
};

static inline size_t shared_pid_memory_nbytes(const struct shared_pid_memory *ctx)
{
    return sizeof(struct ebpfgo_shm_header) + ctx->total * sizeof(struct ebpf_pid_stat);
}

static uint64_t shared_pid_memory_now_monotonic_usec(void)
{
    struct timespec ts;
    if (clock_gettime(CLOCK_MONOTONIC, &ts) != 0)
        return 0;

    return (uint64_t)ts.tv_sec * 1000000ULL + (uint64_t)ts.tv_nsec / 1000ULL;
}

static void shared_pid_memory_invalidate(struct shared_pid_memory *ctx)
{
    if (!ctx || !ctx->header)
        return;

    bool locked = false;
    if (ctx->sem != SEM_FAILED) {
        if (ebpfgo_shm_sem_wait(ctx->sem))
            locked = true;
    }

    /* The semaphore guards the large entries[] array during publish.  These two
     * header scalars are written atomically (no tearing) and the reader detects
     * staleness via last_publish_ut == 0, so the writes are safe whether or not
     * we hold the semaphore.  This is the shutdown path; publish will not run
     * again on this context. */
    __atomic_store_n(&ctx->header->flags, 0, __ATOMIC_RELEASE);
    __atomic_store_n(&ctx->header->last_publish_ut, 0, __ATOMIC_RELEASE);
    __atomic_store_n(&ctx->header->publisher_pid, 0, __ATOMIC_RELEASE);

    if (locked)
        sem_post(ctx->sem);
}

/* Unlink and recreate both SHM and semaphore objects so that consumers
 * detect the new inode on their next refresh and re-open a semaphore that
 * starts at 1.  Called when sem_timedwait times out in the reused-segment
 * path: a timeout cannot distinguish a slow live holder from a crashed owner,
 * so posting without ownership is unsafe; replacing the generation is the
 * only protocol-correct recovery. */
static bool pid_shm_replace_generation(struct shared_pid_memory *ctx, size_t length)
{
    if (ctx->sem != SEM_FAILED) {
        sem_close(ctx->sem);
        ctx->sem = SEM_FAILED;
    }
    if (ctx->mapping) {
        munmap(ctx->mapping, length);
        ctx->mapping = NULL;
        ctx->header  = NULL;
        ctx->entries = NULL;
    }
    if (ctx->shm_fd >= 0) {
        close(ctx->shm_fd);
        ctx->shm_fd = -1;
    }
    /* If the previous attempt backed off on EEXIST, the name may belong to a
     * concurrent legitimate publisher.  Skip unlink to avoid destroying their
     * segment.  Otherwise evict whatever stale generation we left behind. */
    if (!ctx->shm_eexist_backoff) {
        (void)sem_unlink(ctx->sem_name);
        (void)shm_unlink(ctx->shm_name);
    }
    /* Consume the backoff flag and reset ownership: we no longer own anything
     * regardless of what happened in the previous attempt. */
    ctx->shm_eexist_backoff  = false;
    ctx->shm_name_created    = false;

    /* O_EXCL ensures we own the new segment.  On any failure the name's
     * ownership is ambiguous — a concurrent publisher may have claimed it in
     * the window between our shm_unlink and this call.  Record the backoff
     * flag regardless of errno so the next retry skips the unlink. */
    ctx->shm_fd = shm_open(ctx->shm_name, O_CREAT | O_EXCL | O_RDWR, 0640);
    if (ctx->shm_fd < 0) {
        ctx->shm_eexist_backoff = true;
        return false;
    }
    /* Mark created before any further steps so the next replace_generation call
     * knows to unlink the name even if we return early. */
    ctx->shm_name_created = true;
    /* Transfer ownership to real UID so consumers can verify the producer. */
    if (fchown(ctx->shm_fd, getuid(), getgid()) != 0) {
        close(ctx->shm_fd);
        ctx->shm_fd = -1;
        return false;
    }

    /* Write publisher_pid BEFORE ftruncate.  pwrite extends the empty fd to
     * sizeof(publisher_pid) bytes with our PID at offset 0; ftruncate then
     * grows it to length, preserving that value and zero-filling the rest.
     * This eliminates the window where size==length && publisher_pid==0 that
     * lets a concurrent shared_pid_memory_open() treat this live fresh segment
     * as a dead one and wipe it.  pwrite and mmap on the same tmpfs fd are
     * coherent on Linux. */
    {
        uint32_t mypid = (uint32_t)getpid();
        if (pwrite(ctx->shm_fd, &mypid, sizeof(mypid),
                   (off_t)offsetof(struct ebpfgo_shm_header, publisher_pid)) != (ssize_t)sizeof(mypid)) {
            close(ctx->shm_fd);
            ctx->shm_fd = -1;
            return false;
        }
    }

    if (ftruncate(ctx->shm_fd, (off_t)length) != 0) {
        close(ctx->shm_fd);
        ctx->shm_fd = -1;
        return false;
    }

    ctx->mapping = mmap(NULL, length, PROT_READ | PROT_WRITE, MAP_SHARED, ctx->shm_fd, 0);
    if (ctx->mapping == MAP_FAILED) {
        ctx->mapping = NULL;
        close(ctx->shm_fd);
        ctx->shm_fd = -1;
        return false;
    }
    ctx->header  = (struct ebpfgo_shm_header *)ctx->mapping;
    ctx->entries = (struct ebpf_pid_stat *)((char *)ctx->mapping + sizeof(*ctx->header));

    /* publisher_pid was written via pwrite above.
     * Restore the prev_count==0 invariant so the first publish after replace
     * skips the conditional memset (new segment is already kernel-zero-filled). */
    ctx->prev_count = 0;

    /* New SHM is kernel-zero-filled; no explicit memset needed.
     * O_EXCL: we just called sem_unlink so no legitimate semaphore exists.
     * EEXIST means an attacker squatted in the window; evict and retry once. */
    ctx->sem = sem_open(ctx->sem_name, O_CREAT | O_EXCL, 0660, 1);
    if (ctx->sem == SEM_FAILED && errno == EEXIST) {
        (void)sem_unlink(ctx->sem_name);
        ctx->sem = sem_open(ctx->sem_name, O_CREAT | O_EXCL, 0660, 1);
    }
    if (ctx->sem == SEM_FAILED) {
        /* mapping is set but sem is not — next publish would write without a
         * lock.  Null out mapping so the publish guard catches it. */
        munmap(ctx->mapping, length);
        ctx->mapping = NULL;
        ctx->header  = NULL;
        ctx->entries = NULL;
        return false;
    }
    return true;
}

struct shared_pid_memory *shared_pid_memory_open(const char *shm_name, const char *sem_name,
                                                  size_t total, uint32_t update_every_s)
{
    if (!total || !shm_name || !sem_name)
        return NULL;

    struct shared_pid_memory *ctx = callocz(1, sizeof(*ctx));
    if (!ctx)
        return NULL;

    ctx->shm_name = strdup(shm_name);
    ctx->sem_name = strdup(sem_name);
    if (!ctx->shm_name || !ctx->sem_name) {
        freez(ctx->shm_name);
        freez(ctx->sem_name);
        freez(ctx);
        return NULL;
    }

    ctx->shm_fd = -1;
    ctx->sem = SEM_FAILED;
    ctx->update_every_s = update_every_s;

    /* close() calls shm_unlink so /dev/shm is not littered, but this also
     * means a graceful restart creates a fresh SHM object (new inode).
     * Consumers detect the inode change on their next refresh and reconnect
     * within one collection cycle.  The "reused" branch below fires only
     * after a crash (unlink did not run), where the prior segment still exists.
     *
     * We use O_CREAT|O_EXCL so shm_name_created is set only by the actual
     * creator — a racing second publisher that opens the same name sees EEXIST
     * and never sets shm_name_created, so a later failure on its side cannot
     * unlink a live publisher's segment.  On EEXIST we open without claiming
     * creation.  A non-zero pre-ftruncate size (reused=true) means a prior
     * publisher left rows that must be cleared — the kernel only zero-fills
     * fresh segments.  Clearing preserves the optimisation that avoids a
     * 17.5 MB page-fault storm on the first publish after creation. */
    struct stat pre_stat = {0};
    bool reused;
    ctx->shm_fd = shm_open(ctx->shm_name, O_CREAT | O_EXCL | O_RDWR, 0640);
    if (ctx->shm_fd >= 0) {
        reused = false;
        ctx->shm_name_created = true;
        /* Transfer ownership to real UID so consumers can verify the producer. */
        if (fchown(ctx->shm_fd, getuid(), getgid()) != 0)
            goto fail;
    } else if (errno == EEXIST) {
        ctx->shm_fd = shm_open(ctx->shm_name, O_RDWR, 0);
        if (ctx->shm_fd < 0)
            goto fail;
        if (fstat(ctx->shm_fd, &pre_stat) != 0)
            goto fail;
        reused = pre_stat.st_size > 0;
    } else {
        goto fail;
    }

    ctx->total = total;
    size_t length = shared_pid_memory_nbytes(ctx);

    /* Reject a foreign-owned EEXIST segment: publisher_pid in the header is
     * attacker-controlled when the segment is not ours.  An attacker can set
     * publisher_pid = 1 (init) so kill(1,0) always returns 0, permanently
     * blocking the takeover path.  Evict and recreate instead. */
    if (!ctx->shm_name_created && pre_stat.st_uid != getuid()) {
        close(ctx->shm_fd);
        ctx->shm_fd = -1;
        if (!pid_shm_replace_generation(ctx, length))
            goto fail;
        return ctx;
    }

    /* If a crashed writer left a segment whose size differs from what this
     * run needs, unlink and re-create to get a new inode.  Consumers detect
     * the inode change on their next refresh and remap, preventing a SIGBUS
     * from a stale larger mapping accessing pages past the shrunk file end.
     * Exception: if the publisher_pid field is readable and the publisher is
     * alive the segment is in-flight (pwrite completed, ftruncate pending).
     * Evicting now would orphan the initialiser's fd; fail gracefully so the
     * caller retries next cycle after ftruncate finishes. */
    if (reused && pre_stat.st_size != (off_t)length) {
        if (pre_stat.st_size == (off_t)(offsetof(struct ebpfgo_shm_header, publisher_pid) + sizeof(uint32_t))) {
            uint32_t peek_pid = 0;
            if (pread(ctx->shm_fd, &peek_pid, sizeof(peek_pid),
                      (off_t)offsetof(struct ebpfgo_shm_header, publisher_pid)) == (ssize_t)sizeof(peek_pid) &&
                peek_pid > 0 &&
                (kill((pid_t)peek_pid, 0) == 0 || errno == EPERM)) {
                close(ctx->shm_fd);
                ctx->shm_fd = -1;
                goto fail;
            }
        }
        close(ctx->shm_fd);
        ctx->shm_fd = -1;
        (void)shm_unlink(ctx->shm_name);
        /* O_EXCL: we just called shm_unlink.  On EEXIST back off rather than
         * evicting: cannot distinguish a squatter from a concurrent legitimate
         * publisher without risking destroying a live segment. */
        ctx->shm_fd = shm_open(ctx->shm_name, O_CREAT | O_EXCL | O_RDWR, 0640);
        if (ctx->shm_fd < 0)
            goto fail;
        reused = false;
        ctx->shm_name_created = true;
        /* Transfer ownership to real UID so consumers can verify the producer. */
        if (fchown(ctx->shm_fd, getuid(), getgid()) != 0)
            goto fail;
    }

    /* For a fresh segment write publisher_pid BEFORE ftruncate.  Same rationale
     * as pid_shm_replace_generation: pwrite extends the fd to sizeof(publisher_pid)
     * bytes with our PID; ftruncate then grows the segment to length, preserving
     * our PID at offset 0 and zero-filling the rest.  This eliminates the window
     * where size==length && publisher_pid==0 that lets a concurrent opener treat
     * this live fresh segment as a dead one and wipe it.
     * For a reused segment the PID is written inside the semaphore hold below. */
    if (!reused) {
        uint32_t mypid = (uint32_t)getpid();
        if (pwrite(ctx->shm_fd, &mypid, sizeof(mypid),
                   (off_t)offsetof(struct ebpfgo_shm_header, publisher_pid)) != (ssize_t)sizeof(mypid))
            goto fail;
    }

    if (ftruncate(ctx->shm_fd, (off_t)length) != 0)
        goto fail;

    ctx->mapping = mmap(NULL, length, PROT_READ | PROT_WRITE, MAP_SHARED, ctx->shm_fd, 0);
    if (ctx->mapping == MAP_FAILED) {
        ctx->mapping = NULL;
        goto fail;
    }
    ctx->header  = (struct ebpfgo_shm_header *)ctx->mapping;
    ctx->entries = (struct ebpf_pid_stat *)((char *)ctx->mapping + sizeof(*ctx->header));

    /* For a fresh segment (reused=false) no legitimate semaphore exists;
     * O_CREAT|O_EXCL prevents squatting.  On EEXIST evict and retry once.
     * For a reused segment the semaphore already exists and must be joined
     * with O_CREAT so all parties share the same underlying object. */
    if (!reused) {
        ctx->sem = sem_open(ctx->sem_name, O_CREAT | O_EXCL, 0660, 1);
        if (ctx->sem == SEM_FAILED && errno == EEXIST) {
            (void)sem_unlink(ctx->sem_name);
            ctx->sem = sem_open(ctx->sem_name, O_CREAT | O_EXCL, 0660, 1);
        }
    } else {
        ctx->sem = sem_open(ctx->sem_name, O_CREAT, 0660, 1);
    }
    if (ctx->sem == SEM_FAILED)
        goto fail;

    if (reused) {
        if (ebpfgo_shm_sem_wait(ctx->sem)) {
            /* Read the prior publisher's PID under the semaphore.
             * kill(pid, 0) probes liveness: 0 → alive, EPERM → alive but no
             * permission, ESRCH → dead.  Take over only when the prior
             * publisher is confirmed dead: zero the segment, write our PID,
             * and claim ownership.  If still alive, release without
             * modification — the segment belongs to that publisher. */
            pid_t prev_pid   = (pid_t)__atomic_load_n(&ctx->header->publisher_pid, __ATOMIC_ACQUIRE);
            bool  prev_alive = (prev_pid > 0) &&
                               (kill(prev_pid, 0) == 0 || errno == EPERM);
            if (!prev_alive) {
                memset(ctx->mapping, 0, length);
                ctx->header->publisher_pid = (uint32_t)getpid();
                /* shm_name_created stays false: this context did not create the
                 * SHM name and must not unlink it on close — the original
                 * creator may have restarted and be using the same name. */
            }
            sem_post(ctx->sem);
        } else {
            /* Timed out: a slow live holder and a crashed owner both produce
             * the same timeout; posting without ownership is unsafe.  Replace
             * both SHM and semaphore so consumers detect the new inode on
             * their next refresh and re-open a semaphore that starts at 1. */
            if (!pid_shm_replace_generation(ctx, length))
                goto fail;
        }
    }
    /* publisher_pid for the fresh path was written before ftruncate above. */
    return ctx;

fail:
    shared_pid_memory_close(ctx);
    return NULL;
}

int shared_pid_memory_publish(struct shared_pid_memory *ctx, const struct ebpf_pid_stat *entries, size_t count, uint32_t flags)
{
    if (!ctx)
        return -1;

    if (!ctx->mapping) {
        /* replace_generation() failed on a previous cycle.  Retry up to
         * SHM_PID_REPLACE_MAX_RETRIES times so a transient ENOMEM or ENOSPC
         * does not suspend publishing permanently. */
        if (ctx->replace_fail_count >= SHM_PID_REPLACE_MAX_RETRIES)
            return -1;

        if (!pid_shm_replace_generation(ctx, shared_pid_memory_nbytes(ctx))) {
            if (++ctx->replace_fail_count >= SHM_PID_REPLACE_MAX_RETRIES)
                fprintf(stderr,
                        "ebpf-go.plugin: pid shm: replace_generation failed %u consecutive times, publishing suspended permanently\n",
                        (unsigned)ctx->replace_fail_count);
            return -1;
        }

        fprintf(stderr,
                "ebpf-go.plugin: pid shm: recovered after %u failed attempt(s)\n",
                (unsigned)ctx->replace_fail_count);
        ctx->replace_fail_count = 0;
        ctx->publish_timeouts   = 0;
    }

    if (count > ctx->total)
        count = ctx->total;

    bool locked = false;
    if (ctx->sem != SEM_FAILED) {
        if (!ebpfgo_shm_sem_wait(ctx->sem)) {
            /* A consumer killed while holding the semaphore wedges it permanently
             * (POSIX semaphores are not released on process death).  After 3
             * consecutive timeouts assume the semaphore is stuck and replace the
             * generation so consumers detect the new inode and reconnect. */
            if (++ctx->publish_timeouts >= 3) {
                fprintf(stderr,
                        "ebpf-go.plugin: pid shm: semaphore wedged (%u consecutive timeouts), replacing generation\n",
                        ctx->publish_timeouts);
                if (pid_shm_replace_generation(ctx, shared_pid_memory_nbytes(ctx)))
                    ctx->publish_timeouts = 0;
                else
                    fprintf(stderr, "ebpf-go.plugin: pid shm: replace_generation failed; pid publishing suspended\n");
            }
            return -1;
        }
        ctx->publish_timeouts = 0;
        locked = true;
    }

    /* Set per-module validity flags and publish interval for this cycle.
     * update_every_s lets the reader compute a correctly-sized stale window
     * instead of using a hardcoded constant that breaks when update_every > 10. */
    __atomic_store_n(&ctx->header->update_every_s, ctx->update_every_s, __ATOMIC_RELEASE);
    __atomic_store_n(&ctx->header->flags, flags, __ATOMIC_RELEASE);

    if (entries && count)
        memcpy(ctx->entries, entries, count * sizeof(struct ebpf_pid_stat));

    /* Zero only the slots vacated since the previous cycle.  POSIX
     * guarantees the shm_open segment is zero-filled at create time, so
     * on the first call prev_count==0 and this is a no-op. */
    if (ctx->prev_count > count)
        memset(ctx->entries + count, 0,
               (ctx->prev_count - count) * sizeof(struct ebpf_pid_stat));

    ctx->prev_count = count;
    /* live_count is stored after entries are written so a reader that sees the
     * updated last_publish_ut is guaranteed to also see the correct live_count. */
    __atomic_store_n(&ctx->header->live_count, (uint32_t)count, __ATOMIC_RELEASE);
    __atomic_store_n(&ctx->header->last_publish_ut, shared_pid_memory_now_monotonic_usec(), __ATOMIC_RELEASE);

    if (locked)
        sem_post(ctx->sem);

    return 0;
}

void shared_pid_memory_close(struct shared_pid_memory *ctx)
{
    if (!ctx)
        return;

    shared_pid_memory_invalidate(ctx);

    if (ctx->sem != SEM_FAILED)
        sem_close(ctx->sem);

    if (ctx->mapping)
        munmap(ctx->mapping, shared_pid_memory_nbytes(ctx));

    if (ctx->shm_fd >= 0)
        close(ctx->shm_fd);

    /* Unlink only when this context created (or recreated) the SHM name.  A
     * takeover of a pre-existing crashed segment (shm_name_created=false) must
     * NOT unlink: the creator may have restarted and called replace_generation,
     * and unlinking then would remove the new-generation name while the creator
     * is still live.  The sem name is intentionally NOT unlinked: consumers and
     * the next writer must share the same underlying kernel object. */
    if (ctx->shm_name_created)
        (void)shm_unlink(ctx->shm_name);

    freez(ctx->shm_name);
    freez(ctx->sem_name);
    freez(ctx);
}
