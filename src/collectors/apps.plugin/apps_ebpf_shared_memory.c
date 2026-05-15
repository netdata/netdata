// SPDX-License-Identifier: GPL-3.0-or-later

#include "apps_plugin.h"

#if defined(OS_LINUX)

#include <fcntl.h>
#include <semaphore.h>
#include <sys/mman.h>
#include <sys/stat.h>
#include <unistd.h>

static struct ebpf_pid_stat *apps_ebpf_shm = NULL;
static struct ebpf_pid_stat *apps_ebpf_snapshot = NULL;
static size_t apps_ebpf_shm_total = 0;
static size_t apps_ebpf_snapshot_total = 0;
static int apps_ebpf_shm_fd = -1;
static sem_t *apps_ebpf_sem = SEM_FAILED;

static int apps_ebpf_compare_pid(const void *a, const void *b)
{
    const struct ebpf_pid_stat *pa = a;
    const struct ebpf_pid_stat *pb = b;

    if (pa->pid < pb->pid)
        return -1;
    if (pa->pid > pb->pid)
        return 1;

    return 0;
}

static bool apps_ebpf_open_shared_memory(void)
{
    if (apps_ebpf_shm)
        return true;

    apps_ebpf_shm_fd = shm_open(NETDATA_EBPFGO_INTEGRATION_NAME, O_RDONLY, 0);
    if (apps_ebpf_shm_fd < 0)
        return false;

    struct stat st;
    if (fstat(apps_ebpf_shm_fd, &st) != 0 || st.st_size <= 0 || (size_t)st.st_size < sizeof(struct ebpf_pid_stat))
        goto fail;

    apps_ebpf_shm_total = (size_t)st.st_size / sizeof(struct ebpf_pid_stat);
    apps_ebpf_shm = nd_mmap(NULL, (size_t)st.st_size, PROT_READ, MAP_SHARED, apps_ebpf_shm_fd, 0);
    if (apps_ebpf_shm == MAP_FAILED) {
        apps_ebpf_shm = NULL;
        goto fail;
    }

    apps_ebpf_sem = sem_open(NETDATA_EBPFGO_SHM_INTEGRATION_NAME, 0);
    if (apps_ebpf_sem == SEM_FAILED) {
        apps_ebpf_sem = SEM_FAILED;
    }

    return true;

fail:
    if (apps_ebpf_shm_fd >= 0) {
        close(apps_ebpf_shm_fd);
        apps_ebpf_shm_fd = -1;
    }
    if (apps_ebpf_shm) {
        nd_munmap(apps_ebpf_shm, apps_ebpf_shm_total * sizeof(struct ebpf_pid_stat));
        apps_ebpf_shm = NULL;
        apps_ebpf_shm_total = 0;
    }
    return false;
}

static bool apps_ebpf_copy_snapshot(void)
{
    if (!apps_ebpf_shm || !apps_ebpf_shm_total)
        return false;

    if (apps_ebpf_snapshot_total != apps_ebpf_shm_total) {
        apps_ebpf_snapshot = reallocz(apps_ebpf_snapshot, apps_ebpf_shm_total * sizeof(*apps_ebpf_snapshot));
        apps_ebpf_snapshot_total = apps_ebpf_shm_total;
    }

    memcpy(apps_ebpf_snapshot, apps_ebpf_shm, apps_ebpf_shm_total * sizeof(*apps_ebpf_snapshot));
    qsort(apps_ebpf_snapshot, apps_ebpf_shm_total, sizeof(*apps_ebpf_snapshot), apps_ebpf_compare_pid);
    return true;
}

bool apps_ebpf_shared_memory_refresh(void)
{
    if (!apps_ebpf_open_shared_memory())
        return false;

    bool locked = false;
    if (apps_ebpf_sem != SEM_FAILED) {
        if (sem_trywait(apps_ebpf_sem) != 0)
            return false;
        locked = true;
    }

    bool ok = apps_ebpf_copy_snapshot();

    if (locked)
        sem_post(apps_ebpf_sem);

    return ok;
}

static const struct ebpf_pid_stat *apps_ebpf_lookup_snapshot(pid_t pid)
{
    if (!apps_ebpf_snapshot || !apps_ebpf_snapshot_total)
        return NULL;

    size_t left = 0;
    size_t right = apps_ebpf_snapshot_total;

    while (left < right) {
        size_t mid = left + (right - left) / 2;
        const struct ebpf_pid_stat *item = &apps_ebpf_snapshot[mid];
        if (item->pid == (uint32_t)pid)
            return item;
        if (item->pid < (uint32_t)pid)
            left = mid + 1;
        else
            right = mid;
    }

    return NULL;
}

bool apps_ebpf_sync_pid_stat(struct pid_stat *p)
{
    if (!p || !apps_ebpf_snapshot)
        return false;

    const struct ebpf_pid_stat *item = apps_ebpf_lookup_snapshot(p->pid);
    if (!item) {
        p->has_ebpf = false;
        memset(&p->ebpf, 0, sizeof(p->ebpf));
        return false;
    }

    memcpy(&p->ebpf, item, sizeof(p->ebpf));
    p->has_ebpf = true;
    return true;
}

#endif
