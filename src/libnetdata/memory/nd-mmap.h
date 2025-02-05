// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_ND_MMAP_H
#define NETDATA_ND_MMAP_H

#include "../common.h"

int madvise_sequential(void *mem, size_t len);
int madvise_random(void *mem, size_t len);
int madvise_dontfork(void *mem, size_t len);
int madvise_willneed(void *mem, size_t len);
int madvise_dontneed(void *mem, size_t len);
int madvise_dontdump(void *mem, size_t len);
int madvise_mergeable(void *mem, size_t len);
int madvise_thp(void *mem, size_t len);

extern size_t nd_mmap_count;
extern size_t nd_mmap_size;
extern int enable_ksm;

void *nd_mmap_advanced(const char *filename, size_t size, int flags, int ksm, bool read_only, bool dont_dump, int *open_fd);
void *nd_mmap(void *addr, size_t len, int prot, int flags, int fd, off_t offset);
int nd_munmap(void *ptr, size_t size);

#endif //NETDATA_ND_MMAP_H
