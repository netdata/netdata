// SPDX-License-Identifier: GPL-3.0-or-later

#include "shared_dns_memory.h"
#include "nd_alloc_shim.h"

/* ebpfgo_shm_sem_wait and ebpfgo_shm_now_monotonic_usec are defined in
 * apps_ebpf_shared_pid_row.h, included transitively via apps_ebpf_shared_dns_row.h */
#include "apps_ebpf_shared_pid_row.h"

#include <errno.h>
#include <fcntl.h>
#include <semaphore.h>
#include <signal.h>
#include <stdbool.h>
#include <stddef.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <sys/mman.h>
#include <sys/stat.h>
#include <unistd.h>

/* Maximum consecutive replace_generation() attempts before giving up permanently. */
#define SHM_DNS_REPLACE_MAX_RETRIES 10

struct shared_dns_memory {
    struct ebpfgo_dns_shared *data;
    uint32_t update_every_s;
    uint32_t publish_timeouts; /* consecutive publish-side sem_timedwait failures */
    uint8_t replace_fail_count; /* consecutive replace_generation() failures after mapping loss */
    int shm_fd;
    sem_t *sem;
    bool shm_name_created;   /* set when this context created/recreated the SHM name; triggers unlink on any exit */
    bool shm_eexist_backoff; /* set when replace_generation backed off on EEXIST; skips shm_unlink on next retry */
};

static void shared_dns_memory_invalidate(struct shared_dns_memory *ctx)
{
    if (!ctx || !ctx->data)
        return;

    bool locked = false;
    if (ctx->sem != SEM_FAILED) {
        if (ebpfgo_shm_sem_wait(ctx->sem))
            locked = true;
    }

    /* Same rationale as shared_pid_memory_invalidate: these header scalars are
     * written atomically and the reader detects staleness via last_publish_ut,
     * so the writes are safe with or without the semaphore.  Shutdown path only. */
    __atomic_store_n(&ctx->data->hdr.live_count,      0, __ATOMIC_RELEASE);
    __atomic_store_n(&ctx->data->hdr.last_publish_ut, 0, __ATOMIC_RELEASE);

    if (locked)
        sem_post(ctx->sem);
}

/* See pid_shm_replace_generation for the full rationale.  Same generation-
 * replacement approach applied to the DNS SHM and semaphore objects. */
static bool dns_shm_replace_generation(struct shared_dns_memory *ctx, size_t length)
{
    if (ctx->sem != SEM_FAILED) {
        sem_close(ctx->sem);
        ctx->sem = SEM_FAILED;
    }
    if (ctx->data) {
        munmap(ctx->data, length);
        ctx->data = NULL;
    }
    if (ctx->shm_fd >= 0) {
        close(ctx->shm_fd);
        ctx->shm_fd = -1;
    }
    /* If the previous attempt backed off on EEXIST, the name may belong to a
     * concurrent legitimate publisher.  Skip unlink to avoid destroying their
     * segment.  Otherwise evict whatever stale generation we left behind. */
    if (!ctx->shm_eexist_backoff) {
        (void)sem_unlink(NETDATA_EBPFGO_DNS_SEM_NAME);
        (void)shm_unlink(NETDATA_EBPFGO_DNS_SHM_NAME);
    }
    /* Consume the backoff flag and reset ownership. */
    ctx->shm_eexist_backoff = false;
    ctx->shm_name_created   = false;

    /* O_EXCL ensures we own the new segment.  On any failure the name's
     * ownership is ambiguous — a concurrent publisher may have claimed it in
     * the window between our shm_unlink and this call.  Record the backoff
     * flag regardless of errno so the next retry skips the unlink. */
    ctx->shm_fd = shm_open(NETDATA_EBPFGO_DNS_SHM_NAME, O_CREAT | O_EXCL | O_RDWR, 0640);
    if (ctx->shm_fd < 0) {
        ctx->shm_eexist_backoff = true;
        return false;
    }
    /* Mark created before any further steps; close() unlinks on any failure path. */
    ctx->shm_name_created = true;
    /* Transfer ownership to real UID so consumers can verify the producer. */
    if (fchown(ctx->shm_fd, getuid(), getgid()) != 0)
        return false;

    /* Write publisher_pid BEFORE ftruncate.  Same rationale as
     * pid_shm_replace_generation: pwrite extends the empty fd to
     * sizeof(publisher_pid) bytes with our PID; ftruncate then grows it to
     * length, preserving our PID and zero-filling the rest.  This eliminates
     * the window where size==length && publisher_pid==0 that lets a concurrent
     * shared_dns_memory_open() treat this live fresh segment as dead. */
    {
        uint32_t mypid = (uint32_t)getpid();
        if (pwrite(ctx->shm_fd, &mypid, sizeof(mypid),
                   (off_t)offsetof(struct ebpfgo_shm_header, publisher_pid)) != (ssize_t)sizeof(mypid))
            return false;
    }

    if (ftruncate(ctx->shm_fd, (off_t)length) != 0)
        return false;

    ctx->data = mmap(NULL, length, PROT_READ | PROT_WRITE, MAP_SHARED, ctx->shm_fd, 0);
    if (ctx->data == MAP_FAILED) {
        ctx->data = NULL;
        return false;
    }

    /* publisher_pid was written via pwrite above. */

    /* New SHM is kernel-zero-filled; no explicit memset needed.
     * O_EXCL: we just called sem_unlink so no legitimate semaphore exists.
     * EEXIST means an attacker squatted in the window; evict and retry once. */
    ctx->sem = sem_open(NETDATA_EBPFGO_DNS_SEM_NAME, O_CREAT | O_EXCL, 0660, 1);
    if (ctx->sem == SEM_FAILED && errno == EEXIST) {
        (void)sem_unlink(NETDATA_EBPFGO_DNS_SEM_NAME);
        ctx->sem = sem_open(NETDATA_EBPFGO_DNS_SEM_NAME, O_CREAT | O_EXCL, 0660, 1);
    }
    if (ctx->sem == SEM_FAILED) {
        /* mapping is set but sem is not — next publish would write without a
         * lock.  Null out mapping so the publish guard catches it. */
        munmap(ctx->data, length);
        ctx->data = NULL;
        return false;
    }
    return true;
}

struct shared_dns_memory *shared_dns_memory_open(uint32_t update_every_s)
{
    struct shared_dns_memory *ctx = callocz(1, sizeof(*ctx));
    if (!ctx)
        return NULL;

    ctx->shm_fd = -1;
    ctx->sem = SEM_FAILED;
    ctx->update_every_s = update_every_s;

    /* See shared_pid_memory_linux.c for the full rationale.  O_CREAT|O_EXCL
     * ensures shm_name_created is set only by the actual creator — a racing
     * publisher that opens the same name via the EEXIST fallback does not
     * claim creation and cannot unlink a live publisher's segment on failure. */
    struct stat pre_stat = {0};
    bool reused;
    ctx->shm_fd = shm_open(NETDATA_EBPFGO_DNS_SHM_NAME, O_CREAT | O_EXCL | O_RDWR, 0640);
    if (ctx->shm_fd >= 0) {
        reused = false;
        ctx->shm_name_created = true;
        /* Transfer ownership to real UID so consumers can verify the producer. */
        if (fchown(ctx->shm_fd, getuid(), getgid()) != 0)
            goto fail;
    } else if (errno == EEXIST) {
        ctx->shm_fd = shm_open(NETDATA_EBPFGO_DNS_SHM_NAME, O_RDWR, 0);
        if (ctx->shm_fd < 0)
            goto fail;
        if (fstat(ctx->shm_fd, &pre_stat) != 0)
            goto fail;
        reused = pre_stat.st_size > 0;
    } else {
        goto fail;
    }
    size_t length = sizeof(struct ebpfgo_dns_shared);

    /* Reject a foreign-owned EEXIST segment.  Same rationale as
     * shared_pid_memory_linux.c: attacker-controlled publisher_pid can
     * defeat the liveness check permanently.  Evict and recreate. */
    if (!ctx->shm_name_created && pre_stat.st_uid != getuid()) {
        close(ctx->shm_fd);
        ctx->shm_fd = -1;
        if (!dns_shm_replace_generation(ctx, length))
            goto fail;
        return ctx;
    }

    /* Same size-mismatch guard as shared_pid_memory_linux.c: unlink and
     * re-create when a crashed writer left a segment of a different size.
     * Same peek-before-evict: if the publisher is alive the segment is
     * in-flight (pwrite done, ftruncate pending); fail gracefully so the
     * caller retries next cycle. */
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
        (void)shm_unlink(NETDATA_EBPFGO_DNS_SHM_NAME);
        /* O_EXCL: we just called shm_unlink.  On EEXIST back off rather than
         * evicting: cannot distinguish a squatter from a concurrent legitimate
         * publisher without risking destroying a live segment. */
        ctx->shm_fd = shm_open(NETDATA_EBPFGO_DNS_SHM_NAME, O_CREAT | O_EXCL | O_RDWR, 0640);
        if (ctx->shm_fd < 0)
            goto fail;
        reused = false;
        ctx->shm_name_created = true;
        /* Transfer ownership to real UID so consumers can verify the producer. */
        if (fchown(ctx->shm_fd, getuid(), getgid()) != 0)
            goto fail;
    }

    /* For a fresh segment write publisher_pid BEFORE ftruncate.  Same rationale
     * as dns_shm_replace_generation: pwrite extends the fd to sizeof(publisher_pid)
     * bytes with our PID; ftruncate then grows it to length, preserving our PID
     * and zero-filling the rest.  This eliminates the window where
     * size==length && publisher_pid==0 that lets a concurrent opener wipe it.
     * For a reused segment the PID is written inside the semaphore hold below. */
    if (!reused) {
        uint32_t mypid = (uint32_t)getpid();
        if (pwrite(ctx->shm_fd, &mypid, sizeof(mypid),
                   (off_t)offsetof(struct ebpfgo_shm_header, publisher_pid)) != (ssize_t)sizeof(mypid))
            goto fail;
    }

    if (ftruncate(ctx->shm_fd, (off_t)length) != 0)
        goto fail;

    ctx->data = mmap(NULL, length, PROT_READ | PROT_WRITE, MAP_SHARED, ctx->shm_fd, 0);
    if (ctx->data == MAP_FAILED) {
        ctx->data = NULL;
        goto fail;
    }

    /* For a fresh segment (reused=false) no legitimate semaphore exists, so use
     * O_CREAT|O_EXCL to prevent squatting; on EEXIST evict and retry once.
     * For a reused segment the existing semaphore must be joined with O_CREAT
     * (which opens it when already present) so all parties share one object. */
    if (!reused) {
        ctx->sem = sem_open(NETDATA_EBPFGO_DNS_SEM_NAME, O_CREAT | O_EXCL, 0660, 1);
        if (ctx->sem == SEM_FAILED && errno == EEXIST) {
            (void)sem_unlink(NETDATA_EBPFGO_DNS_SEM_NAME);
            ctx->sem = sem_open(NETDATA_EBPFGO_DNS_SEM_NAME, O_CREAT | O_EXCL, 0660, 1);
        }
    } else {
        ctx->sem = sem_open(NETDATA_EBPFGO_DNS_SEM_NAME, O_CREAT, 0660, 1);
    }
    if (ctx->sem == SEM_FAILED)
        goto fail;

    if (reused) {
        if (ebpfgo_shm_sem_wait(ctx->sem)) {
            /* Read the prior publisher's PID and probe liveness — same protocol
             * as shared_pid_memory_linux.c.  Only wipe and claim a dead
             * publisher's segment; leave a live publisher's ring intact. */
            pid_t prev_pid   = (pid_t)__atomic_load_n(&ctx->data->hdr.publisher_pid, __ATOMIC_ACQUIRE);
            bool  prev_alive = (prev_pid > 0) &&
                               (kill(prev_pid, 0) == 0 || errno == EPERM);
            if (!prev_alive) {
                memset(ctx->data, 0, length);
                /* Write publisher_pid inside the semaphore hold so a concurrent
                 * opener never reads a partially-initialised header. */
                ctx->data->hdr.publisher_pid = (uint32_t)getpid();
                /* shm_name_created intentionally NOT set: this context did not
                 * create the SHM name and must not unlink it on close. */
            }
            sem_post(ctx->sem);
        } else {
            /* Timed out: see pid_shm_replace_generation for the rationale.
             * Replace both SHM and semaphore instead of posting without
             * ownership. */
            if (!dns_shm_replace_generation(ctx, length))
                goto fail;
        }
    }
    /* publisher_pid for the fresh path was written before ftruncate above. */
    return ctx;

fail:
    shared_dns_memory_close(ctx);
    return NULL;
}

void shared_dns_memory_publish(
    struct shared_dns_memory *ctx,
    const struct ebpfgo_dns_flow_record *flows,
    uint32_t flow_count)
{
    if (!ctx)
        return;

    if (!ctx->data) {
        /* replace_generation() failed on a previous cycle.  Retry up to
         * SHM_DNS_REPLACE_MAX_RETRIES times so a transient ENOMEM or ENOSPC
         * does not suspend publishing permanently. */
        if (ctx->replace_fail_count >= SHM_DNS_REPLACE_MAX_RETRIES)
            return;

        if (!dns_shm_replace_generation(ctx, sizeof(struct ebpfgo_dns_shared))) {
            if (++ctx->replace_fail_count >= SHM_DNS_REPLACE_MAX_RETRIES)
                fprintf(stderr,
                        "ebpf-go.plugin: dns shm: replace_generation failed %u consecutive times, publishing suspended permanently\n",
                        (unsigned)ctx->replace_fail_count);
            return;
        }

        fprintf(stderr,
                "ebpf-go.plugin: dns shm: recovered after %u failed attempt(s)\n",
                (unsigned)ctx->replace_fail_count);
        ctx->replace_fail_count = 0;
        ctx->publish_timeouts   = 0;
    }

    bool locked = false;
    if (ctx->sem != SEM_FAILED) {
        if (!ebpfgo_shm_sem_wait(ctx->sem)) {
            /* Same wedge recovery as shared_pid_memory_linux.c: after 3 consecutive
             * timeouts assume the semaphore is stuck and replace the generation. */
            if (++ctx->publish_timeouts >= 3) {
                fprintf(stderr,
                        "ebpf-go.plugin: dns shm: semaphore wedged (%u consecutive timeouts), replacing generation\n",
                        ctx->publish_timeouts);
                if (dns_shm_replace_generation(ctx, sizeof(struct ebpfgo_dns_shared)))
                    ctx->publish_timeouts = 0;
                else
                    fprintf(stderr, "ebpf-go.plugin: dns shm: replace_generation failed; dns publishing suspended\n");
            }
            return;
        }
        ctx->publish_timeouts = 0;
        locked = true;
    }

    uint32_t n = flow_count < NETDATA_EBPFGO_DNS_FLOW_RING_CAP
                 ? flow_count : NETDATA_EBPFGO_DNS_FLOW_RING_CAP;
    if (flows && n > 0)
        memcpy(ctx->data->ring, flows, n * sizeof(struct ebpfgo_dns_flow_record));

    /* Write update_every_s and live_count before last_publish_ut so a reader
     * that sees a fresh last_publish_ut also sees consistent metadata. */
    __atomic_store_n(&ctx->data->hdr.update_every_s, ctx->update_every_s, __ATOMIC_RELEASE);
    __atomic_store_n(&ctx->data->hdr.live_count,      n,                  __ATOMIC_RELEASE);
    __atomic_store_n(&ctx->data->hdr.last_publish_ut, ebpfgo_shm_now_monotonic_usec(), __ATOMIC_RELEASE);

    if (locked)
        sem_post(ctx->sem);
}

void shared_dns_memory_close(struct shared_dns_memory *ctx)
{
    if (!ctx)
        return;

    shared_dns_memory_invalidate(ctx);

    if (ctx->sem != SEM_FAILED) {
        sem_close(ctx->sem);
        ctx->sem = SEM_FAILED;
    }

    if (ctx->data) {
        munmap(ctx->data, sizeof(struct ebpfgo_dns_shared));
        ctx->data = NULL;
    }

    if (ctx->shm_fd >= 0) {
        close(ctx->shm_fd);
        ctx->shm_fd = -1;
    }

    /* See shared_pid_memory_linux.c for the full rationale.  Unlink only when
     * this context created/recreated the name; a takeover must not unlink a
     * name it did not create.  sem name is intentionally NOT unlinked. */
    if (ctx->shm_name_created)
        (void)shm_unlink(NETDATA_EBPFGO_DNS_SHM_NAME);

    freez(ctx);
}
