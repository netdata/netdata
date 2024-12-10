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

/* Returns an url-encoded version of str */
/* IMPORTANT: be sure to free() the returned string after use */
char *url_encode(const char *str) {
    char *buf, *pbuf;

    pbuf = buf = mallocz(strlen(str) * 3 + 1);

    while (*str) {
        if (isalnum((uint8_t)*str) || *str == '-' || *str == '_' || *str == '.' || *str == '~')
            *pbuf++ = *str;

        else if (*str == ' ')
            *pbuf++ = '+';

        else{
            *pbuf++ = '%';
            *pbuf++ = to_hex((char)(*str >> 4));
            *pbuf++ = to_hex((char)(*str & 15));
        }

        str++;
    }
    *pbuf = '\0';

    pbuf = strdupz(buf);
    freez(buf);
    return pbuf;
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
char url_percent_escape_decode(const char *s) {
    if(likely(s[1] && s[2]))
        return (char)(from_hex(s[1]) << 4 | from_hex(s[2]));
    return 0;
}

/**
 * Get byte length
 *
 * This (utf8 string related) should be moved in separate file in future
 *
 * @param c is the utf8 character
 *  *
 * @return It returns the length of the specific character.
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
char url_decode_multibyte_utf8(const char *s, char *d, const char *d_end) {
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

char *url_decode_r(char *to, const char *url, size_t size) {
    const char *s = url;     // source
    char *d = to,            // destination
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

    if(unlikely( utf8_check((unsigned  char *)to) )) //NULL means success here
        return NULL;

    return to;

fail_cleanup:
    *d = '\0';
    return NULL;
}

inline bool
url_is_request_complete_and_extract_payload(const char *begin, const char *end, size_t length, BUFFER **post_payload) {
    if (begin == end || length < 4)
        return false;

    if(likely(strncmp(begin, "GET ", 4)) == 0) {
        return strstr(end - 4, "\r\n\r\n");
    }
    else if(unlikely(strncmp(begin, "POST ", 5) == 0 || strncmp(begin, "PUT ", 4) == 0)) {
        const char *cl = strcasestr(begin, "Content-Length: ");
        if(!cl) return false;
        cl = &cl[16];

        size_t content_length = str2ul(cl);

        const char *payload = strstr(cl, "\r\n\r\n");
        if(!payload) return false;
        payload += 4;

        size_t payload_length = length - (payload - begin);

        if(payload_length == content_length) {
            if(!*post_payload)
                *post_payload = buffer_create(payload_length + 1, NULL);

            buffer_contents_replace(*post_payload, payload, payload_length);

            // parse the content type
            const char *ct = strcasestr(begin, "Content-Type: ");
            if(ct) {
                ct = &ct[14];
                while (*ct && isspace((uint8_t)*ct)) ct++;
                const char *space = ct;
                while (*space && !isspace((uint8_t)*space) && *space != ';') space++;
                size_t ct_len = space - ct;

                char ct_copy[ct_len + 1];
                memcpy(ct_copy, ct, ct_len);
                ct_copy[ct_len] = '\0';

                (*post_payload)->content_type = content_type_string2id(ct_copy);
            }
            else
                (*post_payload)->content_type = CT_TEXT_PLAIN;

            return true;
        }

        return false;
    }
    else {
        return strstr(end - 4, "\r\n\r\n");
    }
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
