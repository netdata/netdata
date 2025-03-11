// SPDX-License-Identifier: GPL-3.0-or-later

#include "judy-malloc.h"

// --------------------------------------------------------------------------------------------------------------------
// Judy using ARAL

#define MAX_JUDY_SIZE_TO_ARAL 24
static bool judy_sizes_config[MAX_JUDY_SIZE_TO_ARAL + 1] = {
    [3] = true,
    [4] = true,
    [5] = true,
    [6] = true,
    [7] = true,
    [8] = true,
    [10] = true,
    [11] = true,
    [15] = true,
    [23] = true,
};
static ARAL *judy_sizes_aral[MAX_JUDY_SIZE_TO_ARAL + 1] = { 0 };

struct aral_statistics judy_sizes_aral_statistics = { 0 };

static void aral_judy_init(void) {
    for(size_t Words = 0; Words <= MAX_JUDY_SIZE_TO_ARAL; Words++)
        if(judy_sizes_config[Words]) {
            char buf[30+1];
            snprintfz(buf, sizeof(buf) - 1, "judy-%zu", Words * sizeof(Word_t));
            judy_sizes_aral[Words] = aral_create(
                buf,
                Words * sizeof(Word_t),
                0,
                0,
                &judy_sizes_aral_statistics,
                NULL, NULL, false, false, false);
        }
}

size_t judy_aral_free_bytes(void) {
    return aral_free_bytes_from_stats(&judy_sizes_aral_statistics);
}

size_t judy_aral_structures(void) {
    return aral_structures_bytes_from_stats(&judy_sizes_aral_statistics);
}

struct aral_statistics *judy_aral_statistics(void) {
    return &judy_sizes_aral_statistics;
}

static ARAL *judy_size_aral(Word_t Words) {
    if(Words <= MAX_JUDY_SIZE_TO_ARAL)
        return judy_sizes_aral[Words];

    return NULL;
}

// --------------------------------------------------------------------------------------------------------------------
// Judy memory tracking

static __thread int64_t judy_allocated = 0;

ALWAYS_INLINE void JudyAllocThreadPulseReset(void) {
    judy_allocated = 0;
}

ALWAYS_INLINE int64_t JudyAllocThreadPulseGetAndReset(void) {
    int64_t rc = judy_allocated;
    judy_allocated = 0;
    return rc;
}

// --------------------------------------------------------------------------------------------------------------------
// Judy dedicated jemalloc arena

static unsigned jemalloc_arena_index = 0;
static bool jemalloc_initialized = false;

#ifdef HAVE_JEMALLOC_ARENA_API
#include <jemalloc/jemalloc.h>
static void jemalloc_init(void) {
    // Create shared arena
    size_t sz = sizeof(unsigned);
    if (mallctl("arenas.create", &jemalloc_arena_index, &sz, NULL, 0) != 0)
        return;

    // Disable thread cache for direct arena access
    int cache_enabled = 0;
    if (mallctl("thread.tcache.enabled", NULL, NULL, &cache_enabled, sizeof(bool)) != 0)
        return;

    jemalloc_initialized = true;
}

static void *jemalloc_malloc(Word_t Words) {
    return mallocx(Words * sizeof(Word_t), MALLOCX_ARENA(jemalloc_arena_index));
}

static void jemalloc_free(void * PWord, Word_t Words __maybe_unused) {
    if(PWord)
        dallocx(PWord, MALLOCX_ARENA(jemalloc_arena_index));
}
#endif

// --------------------------------------------------------------------------------------------------------------------
// Judy API

inline Word_t JudyMalloc(Word_t Words)
{
    Word_t Addr;

#ifdef HAVE_JEMALLOC_ARENA_API
    if(jemalloc_initialized)
        Addr = (Word_t)jemalloc_malloc(Words);
    else
#endif
    {
        ARAL *ar = judy_size_aral(Words);
        if (ar)
            Addr = (Word_t)aral_mallocz(ar);
        else
            Addr = (Word_t)mallocz(Words * sizeof(Word_t));
    }

    judy_allocated += Words * sizeof(Word_t);

    return(Addr);
}

inline void JudyFree(void * PWord, Word_t Words) {
#ifdef HAVE_JEMALLOC_ARENA_API
    if(jemalloc_initialized)
        jemalloc_free(PWord, Words);
    else
#endif
    {
        ARAL *ar = judy_size_aral(Words);
        if (ar)
            aral_freez(ar, PWord);
        else
            freez(PWord);
    }

    judy_allocated -= Words * sizeof(Word_t);
}

Word_t JudyMallocVirtual(Word_t Words) {
    return JudyMalloc(Words);
}

void JudyFreeVirtual(void * PWord, Word_t Words) {
    JudyFree(PWord, Words);
}

// --------------------------------------------------------------------------------------------------------------------
// initialization

void libjudy_malloc_init(void) {
    // IMPORTANT: this is not called on external plugins
    // the allocator should run even if this is not called

#ifdef HAVE_JEMALLOC_ARENA_API
    jemalloc_init();
    if(!jemalloc_initialized)
#endif
        aral_judy_init();
}

