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
#else
int enable_ksm = 0;
#endif

static int memory_file_open(const char *filename, size_t size) {
    // netdata_log_info("memory_file_open('%s', %zu", filename, size);

    int fd = open(filename, O_RDWR | O_CREAT | O_NOATIME | O_CLOEXEC, 0664);
    if (fd != -1) {
        if (lseek(fd, size, SEEK_SET) == (off_t) size) {
            if (write(fd, "", 1) == 1) {
                if (ftruncate(fd, size))
                    netdata_log_error("Cannot truncate file '%s' to size %zu. Will use the larger file.", filename, size);
            }
            else
                netdata_log_error("Cannot write to file '%s' at position %zu.", filename, size);
        }
        else
            netdata_log_error("Cannot seek file '%s' to size %zu.", filename, size);
    }
    else
        netdata_log_error("Cannot create/open file '%s'.", filename);

    return fd;
}

inline int madvise_sequential(void *mem, size_t len) {
    static int logger = 1;
    int ret = madvise(mem, len, MADV_SEQUENTIAL);

    if (ret != 0 && logger-- > 0)
        netdata_log_error("madvise(MADV_SEQUENTIAL) of size %zu, failed.", len);
    return ret;
}

inline int madvise_random(void *mem, size_t len) {
    static int logger = 1;
    int ret = madvise(mem, len, MADV_RANDOM);

    if (ret != 0 && logger-- > 0)
        netdata_log_error("madvise(MADV_RANDOM) of size %zu, failed.", len);
    return ret;
}

inline int madvise_dontfork(void *mem, size_t len) {
    static int logger = 1;
    int ret = madvise(mem, len, MADV_DONTFORK);

    if (ret != 0 && logger-- > 0)
        netdata_log_error("madvise(MADV_DONTFORK) of size %zu, failed.", len);
    return ret;
}

inline int madvise_willneed(void *mem, size_t len) {
    static int logger = 1;
    int ret = madvise(mem, len, MADV_WILLNEED);

    if (ret != 0 && logger-- > 0)
        netdata_log_error("madvise(MADV_WILLNEED) of size %zu, failed.", len);
    return ret;
}

inline int madvise_dontneed(void *mem, size_t len) {
    static int logger = 1;
    int ret = madvise(mem, len, MADV_DONTNEED);

    if (ret != 0 && logger-- > 0)
        netdata_log_error("madvise(MADV_DONTNEED) of size %zu, failed.", len);
    return ret;
}

inline int madvise_dontdump(void *mem __maybe_unused, size_t len __maybe_unused) {
#if __linux__
    static int logger = 1;
    int ret = madvise(mem, len, MADV_DONTDUMP);

    if (ret != 0 && logger-- > 0)
        netdata_log_error("madvise(MADV_DONTDUMP) of size %zu, failed.", len);
    return ret;
#else
    return 0;
#endif
}

inline int madvise_mergeable(void *mem __maybe_unused, size_t len __maybe_unused) {
#ifdef MADV_MERGEABLE
    static int logger = 1;
    int ret = madvise(mem, len, MADV_MERGEABLE);

    if (ret != 0 && logger-- > 0)
        netdata_log_error("madvise(MADV_MERGEABLE) of size %zu, failed.", len);
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
        malloc_trace_mmap(size);
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

    // don't enable ksm is the global setting is disabled
    if(unlikely(!enable_ksm)) ksm = 0;

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
