/**
 * \file modp_b64.c
 * <PRE>
 * MODP_B64 - High performance base64 encoder/decoder
 * https://github.com/client9/stringencoders
 *
 * Copyright &copy; 2005-2016 Nick Galbreath
 * All rights reserved.
 * Released under MIT license. See LICENSE for details.
 * </PRE>
 */

/* public header */
#include "modp_b64.h"

#if __BYTE_ORDER__ == __ORDER_LITTLE_ENDIAN__
    #undef WORDS_BIGENDIAN
#else
    #define WORDS_BIGENDIAN 1
#endif

#include "modp_b64_data.h"

#define BADCHAR 0x01FFFFFF

/**
 * you can control if we use padding by commenting out this
 * next line.  However, I highly recommend you use padding and not
 * using it should only be for compatibility with a 3rd party.
 * Also, 'no padding' is not tested!
 */
#define DOPAD 1

/*
 * if we aren't doing padding
 * set the pad character to NULL
 */
#ifndef DOPAD
#undef CHARPAD
#define CHARPAD '\0'
#endif

size_t modp_b64_encode(char* dest, const char* str, size_t len)
{
    size_t i = 0;
    const uint8_t* s = (const uint8_t*)str;
    uint8_t* p = (uint8_t*)dest;

    /* unsigned here is important! */
    /* uint8_t is fastest on G4, amd */
    /* uint32_t is fastest on Intel */
    uint32_t t1, t2, t3;

    if (len > 2) {
        for (i = 0; i < len - 2; i += 3) {
            t1 = s[i];
            t2 = s[i + 1];
            t3 = s[i + 2];
            *p++ = e0[t1];
            *p++ = e1[((t1 & 0x03) << 4) | ((t2 >> 4) & 0x0F)];
            *p++ = e1[((t2 & 0x0F) << 2) | ((t3 >> 6) & 0x03)];
            *p++ = e2[t3];
        }
    }

    switch (len - i) {
    case 0:
        break;
    case 1:
        t1 = s[i];
        *p++ = e0[t1];
        *p++ = e1[(t1 & 0x03) << 4];
        *p++ = CHARPAD;
        *p++ = CHARPAD;
        break;
    default: /* case 2 */
        t1 = s[i];
        t2 = s[i + 1];
        *p++ = e0[t1];
        *p++ = e1[((t1 & 0x03) << 4) | ((t2 >> 4) & 0x0F)];
        *p++ = e2[(t2 & 0x0F) << 2];
        *p++ = CHARPAD;
    }

    *p = '\0';
    return (size_t)(p - (uint8_t*)dest);
}

#ifdef WORDS_BIGENDIAN /* BIG ENDIAN -- SUN / IBM / MOTOROLA */
size_t modp_b64_decode(char* dest, const char* src, size_t len)
{
    size_t i;
    if (len == 0)
        return 0;

#ifdef DOPAD
    /* if padding is used, then the message must be at least
       4 chars and be a multiple of 4.
       there can be at most 2 pad chars at the end */
    if (len < 4 || (len % 4 != 0))
        return -1;
    if (src[len - 1] == CHARPAD) {
        len--;
        if (src[len - 1] == CHARPAD) {
            len--;
        }
    }
#endif /* DOPAD */

    size_t leftover = len % 4;
    size_t chunks = (leftover == 0) ? len / 4 - 1 : len / 4;

    uint8_t* p = (uint8_t*)dest;
    uint32_t x = 0;
    uint32_t* destInt = (uint32_t*)p;
    uint32_t* srcInt = (uint32_t*)src;
    uint32_t y = *srcInt++;
    for (i = 0; i < chunks; ++i) {
        x = d0[y >> 24 & 0xff] | d1[y >> 16 & 0xff] | d2[y >> 8 & 0xff] | d3[y & 0xff];

        if (x >= BADCHAR)
            return -1;
        *destInt = x << 8;
        p += 3;
        destInt = (uint32_t*)p;
        y = *srcInt++;
    }

    switch (leftover) {
    case 0:
        x = d0[y >> 24 & 0xff] | d1[y >> 16 & 0xff] | d2[y >> 8 & 0xff] | d3[y & 0xff];
        if (x >= BADCHAR)
            return -1;
        *p++ = ((uint8_t*)&x)[1];
        *p++ = ((uint8_t*)&x)[2];
        *p = ((uint8_t*)&x)[3];
        return (chunks + 1) * 3;
#ifndef DOPAD
    case 1: /* with padding this is an impossible case */
        x = d3[y >> 24];
        *p = (uint8_t)x;
        break;
#endif
    case 2:
        x = d3[y >> 24] * 64 + d3[(y >> 16) & 0xff];
        *p = (uint8_t)(x >> 4);
        break;
    default: /* case 3 */
        x = (d3[y >> 24] * 64 + d3[(y >> 16) & 0xff]) * 64 + d3[(y >> 8) & 0xff];
        *p++ = (uint8_t)(x >> 10);
        *p = (uint8_t)(x >> 2);
        break;
    }

    if (x >= BADCHAR)
        return -1;
    return 3 * chunks + (6 * leftover) / 8;
}

#else /* LITTLE  ENDIAN -- INTEL AND FRIENDS */

size_t modp_b64_decode(char* dest, const char* src, size_t len)
{
    size_t i;
    size_t leftover;
    size_t chunks;

    uint8_t* p;
    uint32_t x;
    uint32_t* destInt;
    const uint32_t* srcInt = (const uint32_t*)src;
    uint32_t y = *srcInt++;

    if (len == 0)
        return 0;

#ifdef DOPAD
    /*
     * if padding is used, then the message must be at least
     * 4 chars and be a multiple of 4
     */
    if (len < 4 || (len % 4 != 0)) {
        return (size_t)-1; /* error */
    }
    /* there can be at most 2 pad chars at the end */
    if (src[len - 1] == CHARPAD) {
        len--;
        if (src[len - 1] == CHARPAD) {
            len--;
        }
    }
#endif

    leftover = len % 4;
    chunks = (leftover == 0) ? len / 4 - 1 : len / 4;

    p = (uint8_t*)dest;
    x = 0;
    destInt = (uint32_t*)p;
    srcInt = (const uint32_t*)src;
    y = *srcInt++;
    for (i = 0; i < chunks; ++i) {
        x = d0[y & 0xff] | d1[(y >> 8) & 0xff] | d2[(y >> 16) & 0xff] | d3[(y >> 24) & 0xff];

        if (x >= BADCHAR) {
            return (size_t)-1;
        }
        *destInt = x;
        p += 3;
        destInt = (uint32_t*)p;
        y = *srcInt++;
    }

    switch (leftover) {
    case 0:
        x = d0[y & 0xff] | d1[(y >> 8) & 0xff] | d2[(y >> 16) & 0xff] | d3[(y >> 24) & 0xff];

        if (x >= BADCHAR) {
            return (size_t)-1;
        }
        *p++ = ((uint8_t*)(&x))[0];
        *p++ = ((uint8_t*)(&x))[1];
        *p = ((uint8_t*)(&x))[2];
        return (chunks + 1) * 3;
#ifndef DOPAD
    case 1: /* with padding this is an impossible case */
        x = d0[y & 0xff];
        *p = *((uint8_t*)(&x)); /* i.e. first char/byte in int */
        break;
#endif
    case 2: /* case 2, 1  output byte */
        x = d0[y & 0xff] | d1[y >> 8 & 0xff];
        *p = *((uint8_t*)(&x)); /* i.e. first char */
        break;
    default: /* case 3, 2 output bytes */
        x = d0[y & 0xff] | d1[y >> 8 & 0xff] | d2[y >> 16 & 0xff]; /* 0x3c */
        *p++ = ((uint8_t*)(&x))[0];
        *p = ((uint8_t*)(&x))[1];
        break;
    }

    if (x >= BADCHAR) {
        return (size_t)-1;
    }

    return 3 * chunks + (6 * leftover) / 8;
}

#endif /* if bigendian / else / endif */
