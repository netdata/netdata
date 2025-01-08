// SPDX-License-Identifier: GPL-3.0-or-later

#include "../libnetdata.h"

size_t text_sanitize(unsigned char *dst, const unsigned char *src, size_t dst_size, const unsigned char *char_map, bool utf, const char *empty, size_t *multibyte_length) {
    if(unlikely(!dst || !dst_size)) return 0;

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

        if(IS_UTF8_STARTBYTE(c)) {
            size_t utf8_character_bytes = 1;
            bool valid_sequence = true;

            // Determine expected sequence length based on start byte
            if((c & 0xE0) == 0xC0) utf8_character_bytes = 2;      // 2-byte sequence
            else if((c & 0xF0) == 0xE0) utf8_character_bytes = 3; // 3-byte sequence
            else if((c & 0xF8) == 0xF0) utf8_character_bytes = 4; // 4-byte sequence

            if(utf8_character_bytes == 1)
                valid_sequence = false;
            else {
                // make sure all the characters are valid
                for(size_t i = 1; i < utf8_character_bytes; i++) {
                    if(!IS_UTF8_BYTE(src[i]) || IS_UTF8_STARTBYTE(src[i])) {
                        valid_sequence = false;
                        break;
                    }
                }
            }

            if(utf) {
                if (valid_sequence && d + utf8_character_bytes <= end) {
                    // it is a valid utf8 character, and we have room at the destination
                    for (size_t i = 0; i < utf8_character_bytes; i++)
                        *d++ = *src++;
                }
                else {
                    *d++ = hex_digits_lower[(*src & 0xF0) >> 4];
                    if(d <= end) *d++ = hex_digits_lower[(*src & 0x0F)];

                    src++;

                    while(IS_UTF8_BYTE(*src) && !IS_UTF8_STARTBYTE(*src) && d <= end) {
                        *d++ = hex_digits_lower[(*src & 0xF0) >> 4];
                        if(d <= end) *d++ = hex_digits_lower[(*src & 0x0F)];
                        src++;
                    }
                }
            }
            else {
                *d++ = '_'; // this fits, we tested in the while() above

                src++; // skip the utf8 start byte
                // and skip the rest too
                while(IS_UTF8_BYTE(*src) && !IS_UTF8_STARTBYTE(*src))
                    src++;
            }
            last_is_space = false;
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
