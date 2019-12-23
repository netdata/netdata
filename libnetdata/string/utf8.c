// SPDX-License-Identifier: GPL-3.0-or-later

#include "../libnetdata.h"

/**
 * Get byte length
 *
 * Measure the utf8 length
 *
 * @param c is the utf8 character
 *  *
 * @return It reurns the length of the specific character.
 */
char url_utf8_get_byte_length(char c) {
    if(!IS_UTF8_BYTE(c))
        return 1;

    char length = 0;
    while(likely(IS_UTF8_BYTE(c))) {
        length++;
        c <<= 1;
    }
    //4 byte is max size for UTF-8 char
    //10XX XXXX is not valid character -> check length == 1
    if(length > 4 || length == 1)
        return -1;

    return length;
}
