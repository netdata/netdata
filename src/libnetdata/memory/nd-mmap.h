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

extern size_t netdata_mmap_count;
void *netdata_mmap(const char *filename, size_t size, int flags, int ksm, bool read_only, bool dont_dump, int *open_fd);
int netdata_munmap(void *ptr, size_t size);
int memory_file_save(const char *filename, void *mem, size_t size);
extern int enable_ksm;

static inline size_t struct_natural_alignment(size_t size) __attribute__((const));

#define STRUCT_NATURAL_ALIGNMENT (sizeof(uintptr_t) * 2)
static inline size_t struct_natural_alignment(size_t size) {
    if(unlikely(size % STRUCT_NATURAL_ALIGNMENT))
        size = size + STRUCT_NATURAL_ALIGNMENT - (size % STRUCT_NATURAL_ALIGNMENT);

    return size;
}


#endif //NETDATA_ND_MMAP_H
