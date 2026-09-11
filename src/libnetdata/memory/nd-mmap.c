// SPDX-License-Identifier: GPL-3.0-or-later

#include "../libnetdata.h"

size_t nd_mmap_count = 0;
size_t nd_mmap_size = 0;

#if !defined(MADV_DONTFORK)
#define MADV_DONTFORK 0
#endif

#if !defined(O_NOATIME)
#define O_NOATIME 0
#endif

#if defined(MADV_MERGEABLE)
int enable_ksm = CONFIG_BOOLEAN_AUTO;

// Probe the kernel once for KSM support, by doing the real operation on a
// throw-away private anonymous page. This covers a kernel without CONFIG_KSM,
// a seccomp or LSM denial and a container without a usable /sys, all of which a
// /sys/kernel/mm/ksm/ existence check would miss.
// It uses mmap()/munmap() directly, not nd_mmap()/nd_munmap(): the probe's page
// is not agent memory, so it should not be attributed by workers_memory_call()
// nor briefly counted in nd_mmap_count / nd_mmap_size.
static bool ksm_supported_by_kernel(void) {
    static bool probed = false, supported = false;
    static SPINLOCK spinlock = SPINLOCK_INITIALIZER;

    if(likely(__atomic_load_n(&probed, __ATOMIC_ACQUIRE)))
        return supported;

    bool report = false;
    int report_errno = 0;

    spinlock_lock(&spinlock);
    if(!__atomic_load_n(&probed, __ATOMIC_RELAXED)) {
        size_t len = os_get_system_page_size();
        void *mem = mmap(NULL, len, PROT_READ | PROT_WRITE, MAP_ANONYMOUS | MAP_PRIVATE, -1, 0);
        if(mem == MAP_FAILED) {
            // the probe could not run at all, which says nothing about KSM.
            // don't cache this: the next allocation will probe again.
            spinlock_unlock(&spinlock);
            return false;
        }

        bool rc = (madvise(mem, len, MADV_MERGEABLE) == 0);
        int err = rc ? 0 : errno;
        munmap(mem, len);

        if(!rc && (err == ENOMEM || err == EAGAIN || err == EINTR)) {
            // the attempt was inconclusive: resource pressure or an
            // interruption, not an answer about KSM support. don't cache it.
            spinlock_unlock(&spinlock);
            return false;
        }

        // we have a real answer - cache it
        supported = rc;
        __atomic_store_n(&probed, true, __ATOMIC_RELEASE);

        report = !rc;
        report_errno = err;
    }
    spinlock_unlock(&spinlock);

    // report outside the lock: nd_log() allocates, and this spinlock is not
    // recursive, so logging under it would make any future KSM-eligible
    // allocation inside the logger deadlock
    if(report) {
        errno = report_errno;
        nd_log(NDLS_DAEMON, NDLP_NOTICE,
               "KSM (kernel same-page merging) is not available or not permitted on this system. "
               "Memory deduplication is disabled. "
               "Set '[db] memory deduplication (ksm) = no' in netdata.conf to stop checking.");
    }

    return supported;
}
#else
int enable_ksm = 0;
#endif

static int memory_file_open(const char *filename, size_t size) {
    int fd = open(filename, O_RDWR | O_CREAT | O_NOATIME | O_CLOEXEC, 0664);
    if (fd == -1) {
        netdata_log_error("Cannot create/open file '%s'.", filename);
        return -1;
    }

    if (ftruncate(fd, size) != 0) {
        netdata_log_error("Cannot truncate file '%s' to size %zu.", filename, size);
        close(fd);
        return -1;
    }

    struct stat st;
    if (fstat(fd, &st) != 0 || (size_t)st.st_size < size) {
        netdata_log_error("File '%s' is only %lld bytes, expected %zu!", filename, (long long)st.st_size, size);
        close(fd);
        return -1;
    }

    return fd;
}

static inline bool madvise_log_first_failure(int *logger) {
    return __atomic_exchange_n(logger, 0, __ATOMIC_RELAXED) > 0;
}

inline int madvise_sequential(void *mem, size_t len) {
    static int logger = 1;
    int ret = madvise(mem, len, MADV_SEQUENTIAL);

    if (ret != 0 && madvise_log_first_failure(&logger))
        nd_log(NDLS_DAEMON, NDLP_NOTICE, "madvise(MADV_SEQUENTIAL) of size %zu, failed.", len);
    return ret;
}

inline int madvise_random(void *mem, size_t len) {
    static int logger = 1;
    int ret = madvise(mem, len, MADV_RANDOM);

    if (ret != 0 && madvise_log_first_failure(&logger))
        nd_log(NDLS_DAEMON, NDLP_NOTICE, "madvise(MADV_RANDOM) of size %zu, failed.", len);
    return ret;
}

inline int madvise_dontfork(void *mem, size_t len) {
    static int logger = 1;
    int ret = madvise(mem, len, MADV_DONTFORK);

    if (ret != 0 && madvise_log_first_failure(&logger))
        nd_log(NDLS_DAEMON, NDLP_NOTICE, "madvise(MADV_DONTFORK) of size %zu, failed.", len);
    return ret;
}

inline int madvise_willneed(void *mem, size_t len) {
    static int logger = 1;
    int ret = madvise(mem, len, MADV_WILLNEED);

    if (ret != 0 && madvise_log_first_failure(&logger))
        nd_log(NDLS_DAEMON, NDLP_NOTICE, "madvise(MADV_WILLNEED) of size %zu, failed.", len);
    return ret;
}

inline int madvise_dontneed(void *mem, size_t len) {
    static int logger = 1;
    int ret = madvise(mem, len, MADV_DONTNEED);

    if (ret != 0 && madvise_log_first_failure(&logger))
        nd_log(NDLS_DAEMON, NDLP_NOTICE, "madvise(MADV_DONTNEED) of size %zu, failed.", len);
    return ret;
}

inline int madvise_dontdump(void *mem __maybe_unused, size_t len __maybe_unused) {
#if __linux__
    static int logger = 1;
    int ret = madvise(mem, len, MADV_DONTDUMP);

    if (ret != 0 && madvise_log_first_failure(&logger))
        nd_log(NDLS_DAEMON, NDLP_NOTICE, "madvise(MADV_DONTDUMP) of size %zu, failed.", len);
    return ret;
#else
    return 0;
#endif
}

inline int madvise_mergeable(void *mem __maybe_unused, size_t len __maybe_unused) {
#ifdef MADV_MERGEABLE
    static int logger = 1;
    int ret = madvise(mem, len, MADV_MERGEABLE);

    if (ret != 0 && madvise_log_first_failure(&logger))
        nd_log(NDLS_DAEMON, NDLP_NOTICE, "madvise(MADV_MERGEABLE) of size %zu, failed.", len);
    return ret;
#else
    return 0;
#endif
}

#define THP_SIZE (2 * 1024 * 1024) // 2 MiB THP size
#define THP_MASK (THP_SIZE - 1)    // Mask for alignment check

inline int madvise_thp(void *mem __maybe_unused, size_t len __maybe_unused) {
#ifdef MADV_HUGEPAGE
    // Check if the size is at least THP size and aligned
    if (len >= THP_SIZE && ((uintptr_t)mem & THP_MASK) == 0) {
        return madvise(mem, len, MADV_HUGEPAGE);
    }
#endif
    return 0; // Do nothing if THP is not supported or size is too small
}

int nd_munmap(void *ptr, size_t size) {
#ifdef NETDATA_TRACE_ALLOCATIONS
    malloc_trace_munmap(size);
#endif

    workers_memory_call(WORKERS_MEMORY_CALL_MUNMAP);
    int rc = munmap(ptr, size);

    if(rc == 0) {
        __atomic_sub_fetch(&nd_mmap_count, 1, __ATOMIC_RELAXED);
        __atomic_sub_fetch(&nd_mmap_size, size, __ATOMIC_RELAXED);
    }

    return rc;
}

void *nd_mmap(void *addr, size_t len, int prot, int flags, int fd, off_t offset) {
    workers_memory_call(WORKERS_MEMORY_CALL_MMAP);

    void *rc = mmap(addr, len, prot, flags, fd, offset);

    if(rc != MAP_FAILED) {
        __atomic_add_fetch(&nd_mmap_count, 1, __ATOMIC_RELAXED);
        __atomic_add_fetch(&nd_mmap_size, len, __ATOMIC_RELAXED);

#ifdef NETDATA_TRACE_ALLOCATIONS
        malloc_trace_mmap(len);
#endif
    }

    return rc;
}

void *nd_mmap_advanced(const char *filename, size_t size, int flags, int ksm, bool read_only, bool dont_dump, int *open_fd) {
    // netdata_log_info("netdata_mmap('%s', %zu", filename, size);

    // MAP_SHARED is used in memory mode map
    // MAP_PRIVATE is used in memory mode ram and save

    if(unlikely(!(flags & MAP_SHARED) && !(flags & MAP_PRIVATE)))
        fatal("Neither MAP_SHARED or MAP_PRIVATE were given to nd_mmap_advanced()");

    if(unlikely((flags & MAP_SHARED) && (flags & MAP_PRIVATE)))
        fatal("Both MAP_SHARED and MAP_PRIVATE were given to nd_mmap_advanced()");

    if(unlikely((flags & MAP_SHARED) && (!filename || !*filename)))
        fatal("MAP_SHARED requested, without a filename to nd_mmap_advanced()");

    // resolve the 3 states of the global ksm setting:
    // no   - never offer memory to ksm
    // auto - offer it only when the kernel supports it (default)
    // yes  - always offer it, even if the probe says it is unsupported
    // on platforms without MADV_MERGEABLE there is nothing to resolve:
    // enable_ksm is a compile-time CONFIG_BOOLEAN_NO and the option is not
    // parsed, so the first case always wins and the probe does not exist.
    if(ksm) {
        switch(enable_ksm) {
            case CONFIG_BOOLEAN_NO:
                ksm = 0;
                break;

#if defined(MADV_MERGEABLE)
            case CONFIG_BOOLEAN_AUTO:
                ksm = ksm_supported_by_kernel() ? 1 : 0;
                break;
#endif

            default:
                break;
        }
    }

    // KSM only merges anonymous (private) pages, never pagecache (file) pages
    // but MAP_PRIVATE without MAP_ANONYMOUS it fails too, so we need it always
    if((flags & MAP_PRIVATE)) flags |= MAP_ANONYMOUS;

    int fd = -1;
    void *mem = MAP_FAILED;

    errno_clear();

    if(filename && *filename) {
        // open/create the file to be used
        fd = memory_file_open(filename, size);
        if(fd == -1) goto cleanup;
    }

    int fd_for_mmap = fd;
    if(fd != -1 && (flags & MAP_PRIVATE)) {
        // this is MAP_PRIVATE allocation
        // no need for mmap() to use our fd
        // we will copy the file into the memory allocated
        fd_for_mmap = -1;
    }

    mem = nd_mmap(NULL, size, read_only ? PROT_READ : PROT_READ | PROT_WRITE, flags, fd_for_mmap, 0);
    if (mem != MAP_FAILED) {
        // if we have a file open, but we didn't give it to mmap(),
        // we have to read the file into the memory block we allocated
        if(fd != -1 && fd_for_mmap == -1) {
            if (lseek(fd, 0, SEEK_SET) == 0) {
                if (read(fd, mem, size) != (ssize_t) size)
                    netdata_log_info("Cannot read from file '%s'", filename);
            }
            else netdata_log_info("Cannot seek to beginning of file '%s'.", filename);
        }

        madvise_thp(mem, size);
        // madvise_sequential(mem, size);
        // madvise_dontfork(mem, size); // aral is initialized before we daemonize
        if(dont_dump) madvise_dontdump(mem, size);
        // if(flags & MAP_SHARED) madvise_willneed(mem, size);
        if(ksm) madvise_mergeable(mem, size);
    }

cleanup:
    if(fd != -1) {
        if (open_fd)
            *open_fd = fd;
        else
            close(fd);
    }

    if(mem == MAP_FAILED)
        return NULL;

    errno_clear();
    return mem;
}
