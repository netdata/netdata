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

//decode %XX character or return 0 if cannot
char url_percent_escape_decode(char *s) {
    if(likely(s[1] && s[2]))
        return from_hex(s[1]) << 4 | from_hex(s[2]);
    return 0;
}

//this (utf8 string related) should be moved in separate file in future
char url_utf8_get_byte_length(char c) {
    if(!IS_UTF8_BYTE(c))
        return 1;

    char length = 0;
    while(likely(c & 0x80)) {
        length++;
        c <<= 1;
    }
    //4 byte is max size for UTF-8 char
    //10XX XXXX is not valid character -> check length == 1
    if(length > 4 || length == 1)
        return -1;

    return length;
}

//decode % encoded UTF-8 characters and copy them to *d
//return count of bytes written to *d
char url_decode_multibyte_utf8(char *s, char *d, char *d_end) {
    char first_byte = url_percent_escape_decode(s);

    if(unlikely(!first_byte || !IS_UTF8_STARTBYTE(first_byte)))
        return 0;

    char byte_length = url_utf8_get_byte_length(first_byte);

    if(unlikely(byte_length <= 0 || d+byte_length >= d_end))
        return 0;

    char to_read = byte_length;
    while(to_read > 0) {
        char c = url_percent_escape_decode(s);

        if(unlikely( !IS_UTF8_BYTE(c) ))
            return 0;
        if((to_read != byte_length) && IS_UTF8_STARTBYTE(c)) 
            return 0;

        *d++ = c;
        s+=3;
        to_read--;
    }

    return byte_length;
}

/*
 * The utf8_check() function scans the '\0'-terminated string starting
 * at s. It returns a pointer to the first byte of the first malformed
 * or overlong UTF-8 sequence found, or NULL if the string contains
 * only correct UTF-8. It also spots UTF-8 sequences that could cause
 * trouble if converted to UTF-16, namely surrogate characters
 * (U+D800..U+DFFF) and non-Unicode positions (U+FFFE..U+FFFF). This
 * routine is very likely to find a malformed sequence if the input
 * uses any other encoding than UTF-8. It therefore can be used as a
 * very effective heuristic for distinguishing between UTF-8 and other
 * encodings.
 *
 * Markus Kuhn <http://www.cl.cam.ac.uk/~mgk25/> -- 2005-03-30
 * License: http://www.cl.cam.ac.uk/~mgk25/short-license.html
 */

unsigned char *utf8_check(unsigned char *s)
{
    while (*s)
    {
        if (*s < 0x80)
            /* 0xxxxxxx */
            s++;
        else if ((s[0] & 0xe0) == 0xc0)
        {
            /* 110XXXXx 10xxxxxx */
            if ((s[1] & 0xc0) != 0x80 ||
                (s[0] & 0xfe) == 0xc0) /* overlong? */
                return s;
            else
                s += 2;
        }
        else if ((s[0] & 0xf0) == 0xe0)
        {
            /* 1110XXXX 10Xxxxxx 10xxxxxx */
            if ((s[1] & 0xc0) != 0x80 ||
                (s[2] & 0xc0) != 0x80 ||
                (s[0] == 0xe0 && (s[1] & 0xe0) == 0x80) || /* overlong? */
                (s[0] == 0xed && (s[1] & 0xe0) == 0xa0) || /* surrogate? */
                (s[0] == 0xef && s[1] == 0xbf &&
                 (s[2] & 0xfe) == 0xbe)) /* U+FFFE or U+FFFF? */
                return s;
            else
                s += 3;
        }
        else if ((s[0] & 0xf8) == 0xf0)
        {
            /* 11110XXX 10XXxxxx 10xxxxxx 10xxxxxx */
            if ((s[1] & 0xc0) != 0x80 ||
                (s[2] & 0xc0) != 0x80 ||
                (s[3] & 0xc0) != 0x80 ||
                (s[0] == 0xf0 && (s[1] & 0xf0) == 0x80) ||    /* overlong? */
                (s[0] == 0xf4 && s[1] > 0x8f) || s[0] > 0xf4) /* > U+10FFFF? */
                return s;
            else
                s += 4;
        }
        else
            return s;
    }

    return NULL;
}

char *url_decode_r(char *to, char *url, size_t size) {
    char *s = url,           // source
         *d = to,            // destination
         *e = &to[size - 1]; // destination end

    while(*s && d < e) {
        if(unlikely(*s == '%')) {
            char t = url_percent_escape_decode(s);
            if(IS_UTF8_BYTE(t)) {
                char bytes_written = url_decode_multibyte_utf8(s, d, e);
                if(likely(bytes_written)){
                    d += bytes_written;
                    s += (bytes_written * 3)-1;
                }
                else {
                    goto fail_cleanup;
                }
            }
            else if(likely(t) && isprint(t)) {
                // avoid HTTP header injection
                *d++ = t;
                s += 2;
            }
            else
                goto fail_cleanup;
        }
        else if(unlikely(*s == '+'))
            *d++ = ' ';

        else
            *d++ = *s;

        s++;
    }

    *d = '\0';

    if(unlikely( utf8_check(to) )) //NULL means sucess here
        return NULL;

    return to;

fail_cleanup:
    *d = '\0';
    return NULL;
}

/**
 * Is request complete?
 *
 * The URL will be ready top be parsed when the last characters are the string "\r\n\r\n",
 * this function checks the last characters of the message.
 *
 * @param begin is the first character of the sequence to analyse.
 * @param end is the last character of the sequence
 * @param length is the length of the total of bytes read, it is not the difference between end and begin.
 *
 * @return It returns HTTP_VALIDATION_OK when the message is complete and HTTP_VALIDATION_INCOMPLETE otherwise.
 */
inline HTTP_VALIDATION url_is_request_complete(char *begin,char *end,size_t length) {
    //Message cannot be complete when first and last address are the same
    if ( begin == end) {
        return HTTP_VALIDATION_INCOMPLETE;
    }

    //We always set as begin the first character of the message, but we wanna check
    //the last characters, so I am adjusting it case it is possible.
    if ( length > 3  ) {
        begin = end - 4;
    }

    //I wanna have a pair of '\r\n', so I will count them.
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
        } else {
            counter = 0;
            begin++;
        }

        //Case I already have the total expected, I stop it.
        if ( counter == 2) {
            break;
        }
    }
    while (begin != end);

    return (counter == 2)?HTTP_VALIDATION_OK:HTTP_VALIDATION_INCOMPLETE;
}

/**
 * Find protocol
 *
 * Search for the string ' HTTP/' in the message given.
 *
 * @param s is the start of the user request.
 * @return
 */
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

/**
 * Parse query string
 *
 * Parse the query string sent by the client.
 *
 * @param names is the vector to store the variable names
 * @param values is the vector to store the values of the variables.
 * @param moveme the query string vector
 * @param divisor the place of the first equal
 *
 * @return It returns the number of variables processed
 */
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
