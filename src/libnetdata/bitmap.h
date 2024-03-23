// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_BITMAP_H
#define NETDATA_BITMAP_H

#ifdef ENV32BIT

typedef struct bitmapX {
    uint32_t bits;
    uint32_t data[];
} BITMAPX;

typedef struct bitmap256 {
    uint32_t bits;
    uint32_t data[256 / 32];
} BITMAP256;

typedef struct bitmap1024 {
    uint32_t bits;
    uint32_t data[1024 / 32];
} BITMAP1024;

static inline BITMAPX *bitmapX_create(uint32_t bits) {
    BITMAPX *bmp = (BITMAPX *)callocz(1, sizeof(BITMAPX) + sizeof(uint32_t) * ((bits + 31) / 32));
    uint32_t *p = (uint32_t *)&bmp->bits;
    *p = bits;
    return bmp;
}

#define bitmapX_get_bit(ptr, idx) ((ptr)->data[(idx) >> 5] & (1U << ((idx) & 31)))
#define bitmapX_set_bit(ptr, idx, value) do {           \
    register uint32_t _bitmask = 1U << ((idx) & 31);    \
    if (value)                                          \
        (ptr)->data[(idx) >> 5] |= _bitmask;            \
    else                                                \
        (ptr)->data[(idx) >> 5] &= ~_bitmask;           \
} while(0)

#else // 64bit version of bitmaps

typedef struct bitmapX {
    uint32_t bits;
    uint64_t data[];
} BITMAPX;

typedef struct bitmap256 {
    uint32_t bits;
    uint64_t data[256 / 64];
} BITMAP256;

typedef struct bitmap1024 {
    uint32_t bits;
    uint64_t data[1024 / 64];
} BITMAP1024;

static inline BITMAPX *bitmapX_create(uint32_t bits) {
    BITMAPX *bmp = (BITMAPX *)callocz(1, sizeof(BITMAPX) + sizeof(uint64_t) * ((bits + 63) / 64));
    bmp->bits = bits;
    return bmp;
}

#define bitmapX_get_bit(ptr, idx) ((ptr)->data[(idx) >> 6] & (1ULL << ((idx) & 63)))
#define bitmapX_set_bit(ptr, idx, value) do {           \
    register uint64_t _bitmask = 1ULL << ((idx) & 63);  \
    if (value)                                          \
        (ptr)->data[(idx) >> 6] |= _bitmask;            \
    else                                                \
        (ptr)->data[(idx) >> 6] &= ~_bitmask;           \
} while(0)

#endif // 64bit version of bitmaps

#define BITMAPX_INITIALIZER(wanted_bits) { .bits = (wanted_bits), .data = {0} }
#define BITMAP256_INITIALIZER (BITMAP256)BITMAPX_INITIALIZER(256)
#define BITMAP1024_INITIALIZER (BITMAP1024)BITMAPX_INITIALIZER(1024)
#define bitmap256_get_bit(ptr, idx) bitmapX_get_bit((BITMAPX *)ptr, idx)
#define bitmap256_set_bit(ptr, idx, value) bitmapX_set_bit((BITMAPX *)ptr, idx, value)
#define bitmap1024_get_bit(ptr, idx) bitmapX_get_bit((BITMAPX *)ptr, idx)
#define bitmap1024_set_bit(ptr, idx, value) bitmapX_set_bit((BITMAPX *)ptr, idx, value)

#endif //NETDATA_BITMAP_H
