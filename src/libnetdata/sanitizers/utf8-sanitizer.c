// SPDX-License-Identifier: GPL-3.0-or-later

#include "../libnetdata.h"

size_t text_sanitize(unsigned char *dst, const unsigned char *src, size_t dst_size, const unsigned char *char_map, bool utf, const char *empty, size_t *multibyte_length) {
    if(unlikely(!src || !dst || !dst_size)) return 0;

    // skip leading spaces and invalid characters
    while(src && *src && !IS_UTF8_BYTE(*src) && (isspace(*src) || iscntrl(*src) || !isprint(*src)))
        src++;

    if(unlikely(!src || !*src)) {
        strncpyz((char *)dst, empty, dst_size);
        dst[dst_size - 1] = '\0';
        size_t len = strlen((char *)dst);
        if(multibyte_length) *multibyte_length = len;
        return len;
    }

    unsigned char *d = dst;

    // make room for the final string termination
    unsigned char *end = &dst[dst_size - 1];

    // copy while converting, but keep only one space
    // we start wil last_is_space = 1 to skip leading spaces
    int last_is_space = 1;

    size_t mblen = 0;

    while(*src && d < end) {
        unsigned char c = *src;

        if(IS_UTF8_STARTBYTE(c) && IS_UTF8_BYTE(src[1]) && d + 2 <= end) {
            // UTF-8 multi-byte encoded character

            // find how big this character is (2-4 bytes)
            size_t utf_character_size = 2;
            while(utf_character_size < 4 &&
                    d + utf_character_size <= end &&
                    IS_UTF8_BYTE(src[utf_character_size]) &&
                    !IS_UTF8_STARTBYTE(src[utf_character_size]))
                utf_character_size++;

            if(utf) {
                while(utf_character_size) {
                    utf_character_size--;
                    *d++ = *src++;
                }
            }
            else {
                // UTF-8 characters are not allowed.
                // Assume it is an underscore
                // and skip all except the first byte
                *d++ = '_';
                src += (utf_character_size - 1);
            }

            last_is_space = 0;
            mblen++;
            continue;
        }

        c = char_map[c];
        if(c == ' ') {
            // a space character

            if(!last_is_space) {
                // add one space
                *d++ = c;
                mblen++;
            }

            last_is_space++;
        }
        else {
            *d++ = c;
            last_is_space = 0;
            mblen++;
        }

        src++;
    }

    // remove trailing spaces
    while(d > dst && !IS_UTF8_BYTE(*(d - 1)) && *(d - 1) == ' ') {
        d--;
        mblen--;
    }

    // put a termination at the end of what we copied
    *d = '\0';

    // check if dst is all underscores and empty it if it is
    if(*dst == '_') {
        unsigned char *t = dst;
        while (*t == '_') t++;
        if (unlikely(*t == '\0')) {
            *dst = '\0';
            mblen = 0;
        }
    }

    // check if it is empty
    if(unlikely(*dst == '\0')) {
        strncpyz((char *)dst, empty, dst_size);
        dst[dst_size - 1] = '\0';
        mblen = strlen((char *)dst);
        if(multibyte_length) *multibyte_length = mblen;
        return mblen;
    }

    if(multibyte_length) *multibyte_length = mblen;

    return d - dst;
}
