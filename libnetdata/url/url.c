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

/**
 * URL Decode
 *
 * Returns a url-decoded version of str
 * IMPORTANT: be sure to free() the returned string after use
 *
 * @param str the string that will be decode
 *
 * @return a pointer for the url decoded.
 */
char *url_decode(char *str) {
    size_t size = strlen(str) + 1;

    char *buf = mallocz(size);
    return url_decode_r(buf, str, size);
}

/**
 *  Percentage escape decode
 *
 *  Decode %XX character or return 0 if cannot
 *
 *  @param s the string to decode
 *
 *  @return The character decoded on success and 0 otherwise
 */
char url_percent_escape_decode(char *s) {
    if(likely(s[1] && s[2]))
        return from_hex(s[1]) << 4 | from_hex(s[2]);
    return 0;
}

/**
 * Get byte length
 *
 * This (utf8 string related) should be moved in separate file in future
 *
 * @param c is the utf8 character
 *  *
 * @return It reurns the length of the specific character.
 */
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

/**
 * Decode Multibyte UTF8
 *
 * Decode % encoded UTF-8 characters and copy them to *d
 *
 * @param s first address
 * @param d
 * @param d_end last address
 *
 * @return count of bytes written to *d
 */
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

    if(unlikely( utf8_check((unsigned  char *)to) )) //NULL means sucess here
        return NULL;

    return to;

fail_cleanup:
    *d = '\0';
    return NULL;
}

/**
 * Is request complete?
 *
 * Check whether the request is complete.
 * This function cannot check all the requests METHODS, for example, case you are working with POST, it will fail.
 *
 * @param begin is the first character of the sequence to analyse.
 * @param end is the last character of the sequence
 * @param length is the length of the total of bytes read, it is not the difference between end and begin.
 *
 * @return It returns 1 when the request is complete and 0 otherwise.
 */
inline int url_is_request_complete(char *begin, char *end, size_t length) {

    if ( begin == end) {
        //Message cannot be complete when first and last address are the same
        return 0;
    }

    //This math to verify  the last is valid, because we are discarding the POST
    if (length > 4) {
        begin = end - 4;
    }

    return (strstr(begin, "\r\n\r\n"))?1:0;
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
 * Map query string
 *
 * Map the query string fields that will be decoded.
 * This functions must be called after to check the presence of query strings,
 * here we are assuming that you already tested this.
 *
 * @param out the pointer to pointers that will be used to map
 * @param url the input url that we are decoding.
 *
 * @return It returns the number of total variables in the query string.
 */
int url_map_query_string(char **out, char *url) {
    (void)out;
    (void)url;
    int count = 0;

    //First we try to parse considering that there was not URL encode process
    char *moveme = url;
    char *ptr;

    //We always we have at least one here, so I can set this.
    out[count++] = moveme;
    while(moveme) {
        ptr = strchr((moveme+1), '&');
        if(ptr) {
            out[count++] = ptr;
        }

        moveme = ptr;
    }

    //I could not find any '&', so I am assuming now it is like '%26'
    if (count == 1) {
        moveme = url;
        while(moveme) {
            ptr = strchr((moveme+1), '%');
            if(ptr) {
                char *test = (ptr+1);
                if (!strncmp(test, "3f", 2) || !strncmp(test, "3F", 2)) {
                    out[count++] = ptr;
                }
            }
            moveme = ptr;
        }
    }

    return count;
}

/**
 * Parse query string
 *
 * Parse the query string mapped and store it inside output.
 *
 * @param output is a vector where I will store the string.
 * @param max is the maximum length of the output
 * @param map the map done by the function url_map_query_string.
 * @param total the total number of variables inside map
 *
 * @return It returns 0 on success and -1 otherwise
 */
int url_parse_query_string(char *output, size_t max, char **map, int total) {
    if(!total) {
        return 0;
    }

    int counter, next;
    size_t length;
    char *end;
    char *begin = map[0];
    char save;
    size_t copied = 0;
    for(counter = 0, next=1 ; next <= total ; ++counter, ++next) {
        if (next != total) {
            end = map[next];
            length = (size_t) (end - begin);
            save = *end;
            *end = 0x00;
        } else {
            length = strlen(begin);
            end = NULL;
        }
        length++;

        if (length > (max - copied)) {
            error("Parsing query string: we cannot parse a query string so big");
            break;
        }

        if(!url_decode_r(output, begin, length)) {
            return -1;
        }
        length = strlen(output);
        copied += length;
        output += length;

        begin = end;
        if (begin) {
            *begin = save;
        }
    }

    return 0;
}
