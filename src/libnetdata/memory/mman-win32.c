// SPDX-License-Identifier: GPL-3.0-or-later
//
// POSIX mmap()/munmap()/msync()/madvise() implementations for Windows.
//
// See mman-win32.h for the rationale and the supported subset of POSIX
// semantics. The implementation has two backends:
//
//   * Anonymous private memory -> VirtualAlloc / VirtualFree.
//     This is what nd_mmap_advanced uses for MAP_PRIVATE | MAP_ANONYMOUS:
//     onewayalloc arenas, rrddim RAM mode, aral RAM pages.
//
//   * File-backed shared memory -> CreateFileMappingW / MapViewOfFile.
//     This is what nd_mmap_advanced uses for MAP_SHARED with an fd:
//     dbengine journal files (the hot path) and aral save-mode pages.
//
// munmap() must dispatch to the correct cleanup -- UnmapViewOfFile for the
// file-backed view, VirtualFree for the anonymous reservation. There is no
// way to tell them apart from the base address alone, so we keep a small
// JudyL set of addresses that VirtualAlloc returned, guarded by a spinlock.
// Absence from the set means "file-backed".
//
// The mapping HANDLE returned by CreateFileMappingW is closed immediately
// after MapViewOfFile succeeds. Win32 reference-counts the mapping object;
// the view holds it alive until UnmapViewOfFile, after which the kernel
// drops the last reference and reclaims the object. No HANDLE leak, no
// table to maintain.

#include "../libnetdata.h"

#if defined(OS_WINDOWS)

#include <io.h>      // _get_osfhandle
#include <windows.h> // VirtualAlloc, CreateFileMappingW, ...

// ---------------------------------------------------------------------
// anon-allocation tracking

static SPINLOCK g_mman_lock = SPINLOCK_INITIALIZER;
static Pvoid_t  g_anon_set = NULL; // JudyL keyed by base address

static void anon_set_mark(void *addr) {
    spinlock_lock(&g_mman_lock);
    Pvoid_t *p = JudyLIns(&g_anon_set, (Word_t)addr, PJE0);
    if (p && p != PJERR)
        *p = (void *)(uintptr_t)1;
    spinlock_unlock(&g_mman_lock);
}

// Returns true iff `addr` had been recorded as an anonymous (VirtualAlloc)
// region and has now been removed from the set.
static bool anon_set_take(void *addr) {
    spinlock_lock(&g_mman_lock);
    int rc = JudyLDel(&g_anon_set, (Word_t)addr, PJE0);
    spinlock_unlock(&g_mman_lock);
    return rc == 1;
}

// ---------------------------------------------------------------------
// helpers

static DWORD page_protect_from_prot(int prot) {
    if (prot & PROT_WRITE) return PAGE_READWRITE;
    if (prot & PROT_READ)  return PAGE_READONLY;
    return PAGE_NOACCESS;
}

static DWORD view_access_from_prot(int prot) {
    if (prot & PROT_WRITE) return FILE_MAP_READ | FILE_MAP_WRITE;
    if (prot & PROT_READ)  return FILE_MAP_READ;
    return 0;
}

// ---------------------------------------------------------------------
// mmap / munmap

void *mmap(void *addr, size_t length, int prot, int flags, int fd, off_t offset) {
    // MAP_FIXED is not supported -- netdata never asks for it.
    (void)addr;

    if (length == 0) {
        errno = EINVAL;
        return MAP_FAILED;
    }

    // Anonymous: VirtualAlloc backed by the page file.
    if ((flags & MAP_ANONYMOUS) || fd < 0) {
        DWORD page_prot = page_protect_from_prot(prot);
        if (page_prot == PAGE_NOACCESS)
            page_prot = PAGE_READWRITE; // VirtualAlloc with NOACCESS makes the region unusable; promote to RW.

        void *mem = VirtualAlloc(NULL, length, MEM_COMMIT | MEM_RESERVE, page_prot);
        if (!mem) {
            errno = ENOMEM;
            return MAP_FAILED;
        }

        anon_set_mark(mem);
        return mem;
    }

    // File-backed: CreateFileMappingW + MapViewOfFile. The mapping HANDLE
    // is closed immediately; the view keeps it alive.
    HANDLE file = (HANDLE)_get_osfhandle(fd);
    if (file == INVALID_HANDLE_VALUE) {
        errno = EBADF;
        return MAP_FAILED;
    }

    // CreateFileMapping's max-size argument is the offset of the byte
    // past the end of the mapping. Pass 0/0 to use the underlying file
    // size, which matches POSIX mmap of the whole file when length covers
    // the file. We pass the explicit end so we don't depend on the
    // current file length post-truncate.
    uint64_t end = (uint64_t)offset + (uint64_t)length;
    DWORD page_prot = page_protect_from_prot(prot);

    HANDLE mapping = CreateFileMappingW(
        file,
        NULL,
        page_prot,
        (DWORD)(end >> 32),
        (DWORD)(end & 0xFFFFFFFFu),
        NULL);
    if (!mapping) {
        errno = ENOMEM;
        return MAP_FAILED;
    }

    DWORD view_access = view_access_from_prot(prot);
    uint64_t off = (uint64_t)offset;
    void *base = MapViewOfFile(
        mapping,
        view_access,
        (DWORD)(off >> 32),
        (DWORD)(off & 0xFFFFFFFFu),
        length);

    // Whether or not MapViewOfFile succeeded, we can release our handle.
    // The view (if any) keeps the mapping object alive.
    CloseHandle(mapping);

    if (!base) {
        errno = ENOMEM;
        return MAP_FAILED;
    }

    return base;
}

int munmap(void *addr, size_t length) {
    (void)length;

    if (!addr) {
        errno = EINVAL;
        return -1;
    }

    if (anon_set_take(addr)) {
        if (!VirtualFree(addr, 0, MEM_RELEASE)) {
            errno = EINVAL;
            return -1;
        }
        return 0;
    }

    if (!UnmapViewOfFile(addr)) {
        errno = EINVAL;
        return -1;
    }
    return 0;
}

// ---------------------------------------------------------------------
// msync

int msync(void *addr, size_t length, int flags) {
    (void)flags;

    if (!addr) {
        errno = EINVAL;
        return -1;
    }

    if (!FlushViewOfFile(addr, length)) {
        errno = EIO;
        return -1;
    }
    return 0;
}

// ---------------------------------------------------------------------
// madvise -- accepted but not honored. Windows has no broadly-available
// equivalent for SEQUENTIAL/RANDOM/WILLNEED/DONTNEED; what is available
// (PrefetchVirtualMemory, OfferVirtualMemory) is either Windows-8+,
// process-memory-only, or carries semantics that differ enough from POSIX
// that doing the wrong thing here would hide bugs. Returning 0 keeps the
// existing logger-once "madvise failed" log path silent on Windows.

int madvise(void *addr, size_t length, int advice) {
    (void)addr;
    (void)length;
    (void)advice;
    return 0;
}

#endif // OS_WINDOWS
