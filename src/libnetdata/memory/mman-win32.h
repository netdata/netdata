// SPDX-License-Identifier: GPL-3.0-or-later
//
// POSIX <sys/mman.h> shim for Windows / UCRT64.
//
// mingw-w64's UCRT64 environment ships no <sys/mman.h>: there are no
// mmap/munmap/msync/madvise symbols, no MAP_* / PROT_* / MS_* / MADV_*
// constants, and no MAP_FAILED sentinel. Cygwin provided all of this via
// the msys-2.0.dll POSIX emulation layer; UCRT64 does not.
//
// This header exposes the subset of <sys/mman.h> that netdata actually
// uses, with POSIX-shaped declarations. The implementations live in
// mman-win32.c and are built on top of Win32 file-mapping
// (CreateFileMappingW / MapViewOfFile) and anonymous-memory (VirtualAlloc)
// primitives.
//
// Only the call patterns netdata exercises are supported -- in particular,
// nd_mmap_advanced is the centralised wrapper and the only patterns it
// passes through are:
//
//   * MAP_PRIVATE | MAP_ANONYMOUS with fd == -1
//       -> anonymous reservation, served by VirtualAlloc.
//   * MAP_SHARED with a valid fd
//       -> file-backed mapping, served by CreateFileMappingW + MapViewOfFile.
//
// MAP_FIXED, mremap(), partial msync ranges, and exec mappings are out of
// scope: netdata doesn't use them, and pretending to support them would
// hide bugs rather than catch them.

#ifndef NETDATA_MMAN_WIN32_H
#define NETDATA_MMAN_WIN32_H

#if defined(OS_WINDOWS)

#include <sys/types.h>
#include <stddef.h>

#ifdef __cplusplus
extern "C" {
#endif

// POSIX page-protection bits. These flag values match Linux for source
// readability; the actual Win32 protection (PAGE_READONLY / PAGE_READWRITE)
// is computed inside mmap().
#ifndef PROT_NONE
#define PROT_NONE  0x0
#endif
#ifndef PROT_READ
#define PROT_READ  0x1
#endif
#ifndef PROT_WRITE
#define PROT_WRITE 0x2
#endif
#ifndef PROT_EXEC
#define PROT_EXEC  0x4
#endif

// POSIX mapping flags. Linux values, again for readability.
#ifndef MAP_SHARED
#define MAP_SHARED    0x01
#endif
#ifndef MAP_PRIVATE
#define MAP_PRIVATE   0x02
#endif
#ifndef MAP_FIXED
#define MAP_FIXED     0x10
#endif
#ifndef MAP_ANONYMOUS
#define MAP_ANONYMOUS 0x20
#endif
#ifndef MAP_ANON
#define MAP_ANON      MAP_ANONYMOUS
#endif

#ifndef MAP_FAILED
#define MAP_FAILED ((void *)-1)
#endif

// msync flags.
#ifndef MS_ASYNC
#define MS_ASYNC      0x1
#endif
#ifndef MS_SYNC
#define MS_SYNC       0x4
#endif
#ifndef MS_INVALIDATE
#define MS_INVALIDATE 0x2
#endif

// madvise hints. The Windows implementation treats all of these as
// best-effort no-ops; the values exist purely so call sites compile
// (nd_mmap.c's madvise_* helpers reference them unconditionally).
#ifndef MADV_NORMAL
#define MADV_NORMAL     0
#endif
#ifndef MADV_RANDOM
#define MADV_RANDOM     1
#endif
#ifndef MADV_SEQUENTIAL
#define MADV_SEQUENTIAL 2
#endif
#ifndef MADV_WILLNEED
#define MADV_WILLNEED   3
#endif
#ifndef MADV_DONTNEED
#define MADV_DONTNEED   4
#endif

void   *mmap(void *addr, size_t length, int prot, int flags, int fd, off_t offset);
int     munmap(void *addr, size_t length);
int     msync(void *addr, size_t length, int flags);
int     madvise(void *addr, size_t length, int advice);

#ifdef __cplusplus
}
#endif

#endif // OS_WINDOWS

#endif // NETDATA_MMAN_WIN32_H
