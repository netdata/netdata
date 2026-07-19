// SPDX-License-Identifier: GPL-3.0-or-later

#include "../libnetdata.h"

// ----------------------------------------------------------------------------
// URL encode / decode
// code from: http://www.geekhideout.com/urlcode.shtml

/* Converts a hex character to its integer value */
char from_hex(char ch) {
    uint8_t uch = (uint8_t)ch;
    return (char)(isdigit(uch) ? uch - '0' : tolower(uch) - 'a' + 10);
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
    size_t max_len = (SIZE_MAX - 1) / 3;
    size_t len = strnlen(str, max_len + 1);

    if(unlikely(len > (SIZE_MAX - 1) / 3))
        fatal("url_encode() cannot allocate encoded string for %zu bytes.", len);

    pbuf = buf = mallocz(len * 3 + 1);

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

static inline char url_utf8_get_byte_length(char c) {
    uint8_t byte = (uint8_t)c;

    if(!IS_UTF8_BYTE(byte))
        return 1;

    char length = 0;
    while(likely(byte & 0x80)) {
        length++;
        byte <<= 1;
    }
    //4 byte is max size for UTF-8 char
    //10XX XXXX is not valid character -> check length == 1
    if(length > 4 || length == 1)
        return -1;

    return length;
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

static inline bool url_decode_percent_byte(const char *s, const char *end, uint8_t *byte) {
    if(unlikely(end - s < 3 || s[0] != '%' || !isxdigit((uint8_t)s[1]) || !isxdigit((uint8_t)s[2])))
        return false;

    *byte = ((uint8_t)from_hex(s[1]) << 4) | (uint8_t)from_hex(s[2]);
    return true;
}

URL_DECODE_STATUS url_decode_r_len(char *to, size_t size, const char *url, size_t url_length, size_t *decoded_length) {
    if(decoded_length)
        *decoded_length = 0;

    if(unlikely(!to || !size))
        return URL_DECODE_DESTINATION_TOO_SMALL;

    if(unlikely(!url))
        return URL_DECODE_MALFORMED;

    const char *s = url;
    const char *end = url + url_length;
    char *d = to;
    char *d_end = &to[size - 1];

    while(s < end) {
        if(unlikely(*s == '%')) {
            uint8_t first_byte;
            if(unlikely(!url_decode_percent_byte(s, end, &first_byte) || !first_byte))
                goto malformed;

            if(IS_UTF8_BYTE(first_byte)) {
                char byte_length = url_utf8_get_byte_length((char)first_byte);
                if(unlikely(byte_length <= 0 || end - s < byte_length * 3))
                    goto malformed;

                if(unlikely(d_end - d < byte_length))
                    goto too_small;

                for(char i = 0; i < byte_length; i++) {
                    uint8_t byte;
                    if(unlikely(!url_decode_percent_byte(s, end, &byte) ||
                                (i == 0 && !IS_UTF8_STARTBYTE(byte)) ||
                                (i != 0 && (byte & 0xc0) != 0x80)))
                        goto malformed;

                    *d++ = (char)byte;
                    s += 3;
                }
            }
            else {
                if(unlikely(!isprint(first_byte)))
                    goto malformed;

                if(unlikely(d == d_end))
                    goto too_small;

                *d++ = (char)first_byte;
                s += 3;
            }
        }
        else {
            if(unlikely(!*s))
                goto malformed;

            if(unlikely(d == d_end))
                goto too_small;

            *d++ = (*s == '+') ? ' ' : *s;
            s++;
        }
    }

    *d = '\0';

    if(unlikely(utf8_check((unsigned char *)to)))
        goto malformed;

    if(decoded_length)
        *decoded_length = (size_t)(d - to);

    return URL_DECODE_OK;

too_small:
    *d = '\0';
    return URL_DECODE_DESTINATION_TOO_SMALL;

malformed:
    *d = '\0';
    return URL_DECODE_MALFORMED;
}

char *url_decode_r(char *to, const char *url, size_t size) {
    if(unlikely(!url || url_decode_r_len(to, size, url, strlen(url), NULL) != URL_DECODE_OK))
        return NULL;

    return to;
}

int url_unittest(void) {
    int errors = 0;
    char decoded[64];
    size_t decoded_length = 0;

    if(url_decode_r_len(decoded, 1, "", 0, &decoded_length) != URL_DECODE_OK ||
       decoded_length != 0 || decoded[0] != '\0')
        errors++;

    if(url_decode_r_len(decoded, sizeof(decoded), "hello+world%21", 14, &decoded_length) != URL_DECODE_OK ||
       decoded_length != 12 || strcmp(decoded, "hello world!") != 0)
        errors++;

    if(url_decode_r_len(decoded, sizeof(decoded), "%E2%82%AC", 9, &decoded_length) != URL_DECODE_OK ||
       decoded_length != 3 || memcmp(decoded, "\xE2\x82\xAC", 3) != 0)
        errors++;

    static const char *malformed[] = { "%", "%GG", "%00", "%0A", "%80", "%C0%AF", "%F4%90%80%80" };
    for(size_t i = 0; i < _countof(malformed); i++) {
        if(url_decode_r_len(decoded, sizeof(decoded), malformed[i], strlen(malformed[i]), NULL) != URL_DECODE_MALFORMED)
            errors++;
    }

    const char embedded_nul[] = { 'a', '\0', 'b' };
    if(url_decode_r_len(decoded, sizeof(decoded), embedded_nul, sizeof(embedded_nul), NULL) != URL_DECODE_MALFORMED)
        errors++;

    const char invalid_utf8[] = { (char)0x80 };
    if(url_decode_r_len(decoded, sizeof(decoded), invalid_utf8, sizeof(invalid_utf8), NULL) != URL_DECODE_MALFORMED)
        errors++;

    if(url_decode_r_len(decoded, 5, "abcd", 4, &decoded_length) != URL_DECODE_OK ||
       decoded_length != 4 || strcmp(decoded, "abcd") != 0 ||
       url_decode_r_len(decoded, 4, "abcd", 4, NULL) != URL_DECODE_DESTINATION_TOO_SMALL ||
       url_decode_r_len(decoded, 2, "%41", 3, &decoded_length) != URL_DECODE_OK ||
       decoded_length != 1 || strcmp(decoded, "A") != 0)
        errors++;

    if(errors)
        fprintf(stderr, "URL: %d test(s) failed\n", errors);

    return errors;
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

        while(*cl == ' ' || *cl == '\t')
            cl++;

        if(!isdigit((uint8_t)*cl))
            return false;

        char *content_length_end;
        errno_clear();
        unsigned long long parsed_content_length = strtoull(cl, &content_length_end, 10);
        if(errno != 0 || parsed_content_length > SIZE_MAX)
            return false;

        while(*content_length_end == ' ' || *content_length_end == '\t')
            content_length_end++;

        if(content_length_end[0] != '\r' || content_length_end[1] != '\n')
            return false;

        size_t content_length = (size_t)parsed_content_length;

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

                CLEAN_CHAR_P *ct_copy = mallocz(ct_len + 1);
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
inline char *url_find_protocol(char *s, const char *end) {
    while(s < end && *s) {
        // find the next space
        while (s < end && *s && *s != ' ') s++;

        if(s >= end || !*s) break;

        // is it SPACE + "HTTP/" ?
        if((size_t)(end - s) >= 6 && !strncmp(s, " HTTP/", 6)) break;
        s++;
    }

    return s;
}
