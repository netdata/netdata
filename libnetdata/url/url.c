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

inline HTTP_VALIDATION url_is_request_complete(char *begin,char *end,size_t length) {
    if ( begin == end) {
        return HTTP_VALIDATION_INCOMPLETE;
    }

    if ( length > 3  ) {
        begin = end - 4;
    }

    uint32_t counter = 0;
    do {
        if (*begin == '\r') {
            begin++;
            if ( begin == end )
            {
                break;
            }

            if (*begin == '\n')
            {
                counter++;
            }
        } else if (*begin == '\n') {
            begin++;
            counter++;
        }

        if ( counter == 2) {
            break;
        }
    }
    while (begin != end);

    return (counter == 2)?HTTP_VALIDATION_OK:HTTP_VALIDATION_INCOMPLETE;
}

inline char *url_find_protocol(char *s) {
    while(*s) {
        // find the next space
        while (*s && *s != ' ') s++;

        // is it SPACE + "HTTP/" ?
        if(*s && !strncmp(s, " HTTP/", 6)) break;
        else s++;
    }

    return s;
}

int url_parse_query_string(struct web_fields *names,struct web_fields *values,char *moveme,char *divisor) {
    uint32_t i = 0;
    uint32_t max = WEB_FIELDS_MAX;

    do {
        if ( i == max) {
            error("We are exceeding the maximum number of elements possible(%u) in this query string(%s)",max,moveme);
            break;
        }
        if (divisor) {
            names[i].body = moveme;
            names[i].length = divisor - moveme;//= - begin

            moveme = ++divisor; //value
            values[i].body = moveme;

            (void)divisor;
            divisor = strchr(moveme,'&'); //end of value
            if (divisor) {
                values[i].length = (size_t )(divisor - moveme);
            } else{
                values[i].length = strlen(moveme);
                break;
            }

            moveme = divisor;
            divisor = strchr(++moveme,'='); //end of value
            i++;
        } else {
            break;
        }
    } while (moveme);

    return ++i;
}
