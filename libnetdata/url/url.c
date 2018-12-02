// SPDX-License-Identifier: GPL-3.0-or-later

#include "../libnetdata.h"

// ----------------------------------------------------------------------------
// URL encode / decode
// code from: http://www.geekhideout.com/urlcode.shtml

/* Converts a hex character to its integer value */
char from_hex(char ch) {
    return (char)(isdigit(ch) ? ch - '0' : tolower(ch) - 'a' + 10);
}

/* Converts an integer value to its hex character*/
char to_hex(char code) {
    static char hex[] = "0123456789abcdef";
    return hex[code & 15];
}

/* Returns a url-encoded version of str */
/* IMPORTANT: be sure to free() the returned string after use */
char *url_encode(char *str) {
    char *buf, *pbuf;

    pbuf = buf = mallocz(strlen(str) * 3 + 1);

    while (*str) {
        if (isalnum(*str) || *str == '-' || *str == '_' || *str == '.' || *str == '~')
            *pbuf++ = *str;

        else if (*str == ' ')
            *pbuf++ = '+';

        else
            *pbuf++ = '%', *pbuf++ = to_hex(*str >> 4), *pbuf++ = to_hex(*str & 15);

        str++;
    }
    *pbuf = '\0';

    pbuf = strdupz(buf);
    freez(buf);
    return pbuf;
}

/* Returns a url-decoded version of str */
/* IMPORTANT: be sure to free() the returned string after use */
char *url_decode(char *str) {
    size_t size = strlen(str) + 1;

    char *buf = mallocz(size);
    return url_decode_r(buf, str, size);
}

char *url_decode_r(char *to, char *url, size_t size) {
    char *s = url,           // source
         *d = to,            // destination
         *e = &to[size - 1]; // destination end

    while(*s && d < e) {
        if(unlikely(*s == '%')) {
            if(likely(s[1] && s[2])) {
                char t = from_hex(s[1]) << 4 | from_hex(s[2]);
                // avoid HTTP header injection
                *d++ = (char)((isprint(t))? t : ' ');
                s += 2;
            }
        }
        else if(unlikely(*s == '+'))
            *d++ = ' ';

        else
            *d++ = *s;

        s++;
    }

    *d = '\0';

    return to;
}
