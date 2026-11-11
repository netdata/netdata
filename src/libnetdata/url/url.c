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

static inline bool url_is_hex_digit(char c) {
    return (c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F');
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
    if(likely(s[0] == '%' && url_is_hex_digit(s[1]) && url_is_hex_digit(s[2])))
        return (char)(((uint8_t)from_hex(s[1]) << 4) | (uint8_t)from_hex(s[2]));
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
    if(unlikely(!size))
        return NULL;

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

// The header lines of a request whose header section is complete: returns the first header line and sets
// *headers_end to the "\r\n\r\n" that ends the section, or returns NULL when the section is not complete yet.
static const char *url_request_header_lines(const char *begin, const char **headers_end) {
    *headers_end = strstr(begin, "\r\n\r\n");
    if(!*headers_end)
        return NULL;

    // the request line always ends before the header section does
    return strstr(begin, "\r\n") + 2;
}

// Finds the next header called `name` (case-insensitive, matched only as a whole field name at the start of a
// header line), from *line up to headers_end. Returns its value without the whitespace around it, sets *value_len,
// and moves *line past that header; returns NULL when there is no further such header.
static const char *url_request_header_value(
    const char **line, const char *headers_end, const char *name, size_t name_len, size_t *value_len) {
    const char *s = *line;

    while(s < headers_end) {
        const char *eol = strstr(s, "\r\n");
        if(!eol || eol > headers_end)
            eol = headers_end;

        if((size_t)(eol - s) > name_len && s[name_len] == ':' && strncasecmp(s, name, name_len) == 0) {
            const char *v = &s[name_len + 1], *ve = eol;
            while(v < ve && (*v == ' ' || *v == '\t')) v++;
            while(ve > v && (ve[-1] == ' ' || ve[-1] == '\t')) ve--;

            *line = eol + 2;
            *value_len = ve - v;
            return v;
        }

        s = eol + 2;
    }

    *line = headers_end;
    return NULL;
}

// The body framing of a POST/PUT request with a complete header section. Fails when Content-Length is missing,
// is not a plain decimal number, or appears more than once with different values (RFC 9112 6.3).
static bool url_request_content_length(const char *begin, size_t *payload_offset, size_t *content_length) {
    const char *headers_end;
    const char *line = url_request_header_lines(begin, &headers_end);
    if(!line)
        return false;

    bool found = false;
    unsigned long long length = 0;
    const char *v;
    size_t v_len;
    while((v = url_request_header_value(&line, headers_end, "Content-Length", 14, &v_len))) {
        if(!v_len || !isdigit((uint8_t)*v))
            return false;

        char *v_end;
        errno_clear();
        unsigned long long parsed = strtoull(v, &v_end, 10);
        if(errno != 0 || v_end != v + v_len || parsed > SIZE_MAX)
            return false;

        if(found && parsed != length)
            return false;

        length = parsed;
        found = true;
    }

    if(!found)
        return false;

    size_t offset = (size_t)(headers_end + 4 - begin);
    if(length > SIZE_MAX - offset)
        return false;

    *payload_offset = offset;
    *content_length = (size_t)length;
    return true;
}

static HTTP_CONTENT_TYPE url_request_content_type(const char *begin) {
    const char *headers_end;
    const char *line = url_request_header_lines(begin, &headers_end);
    size_t ct_len;
    const char *ct = line ? url_request_header_value(&line, headers_end, "Content-Type", 12, &ct_len) : NULL;
    if(!ct)
        return CT_TEXT_PLAIN;

    // the media type, without its parameters
    size_t type_len = 0;
    while(type_len < ct_len && !isspace((uint8_t)ct[type_len]) && ct[type_len] != ';')
        type_len++;

    CLEAN_CHAR_P *ct_copy = mallocz(type_len + 1);
    memcpy(ct_copy, ct, type_len);
    ct_copy[type_len] = '\0';

    return content_type_string2id(ct_copy);
}

inline bool
url_is_request_complete_and_extract_payload(
    const char *begin, const char *end, size_t length, BUFFER **post_payload, size_t *expected_size) {
    if (length < 4)
        return false;

    // The caller rewinds the incremental cursor by up to 4 bytes.
    // After a previous receive of 4 to 7 bytes, end is begin through begin + 3,
    // so searches starting at end - 4 would start before the buffer.
    // Clamp end to begin + 4 so those searches start at begin; larger cursors
    // keep the incremental rescan.
    if (end < begin + 4)
        end = begin + 4;

    if(likely(strncmp(begin, "GET ", 4)) == 0) {
        return strstr(end - 4, "\r\n\r\n");
    }
    else if(unlikely(strncmp(begin, "POST ", 5) == 0 || strncmp(begin, "PUT ", 4) == 0)) {
        if(*expected_size == SIZE_MAX)
            return false;

        if(!*expected_size) {
            // The first call may contain the whole request; later calls begin before newly received bytes.
            const char *headers_search_from = (end == begin + length) ? begin : end;
            if(!strstr(headers_search_from, "\r\n\r\n"))
                return false;

            size_t payload_offset, content_length;
            if(!url_request_content_length(begin, &payload_offset, &content_length))
                goto invalid_content_length;

            *expected_size = payload_offset + content_length;
        }

        if(length != *expected_size)
            return false;

        size_t payload_offset, content_length;
        if(!url_request_content_length(begin, &payload_offset, &content_length))
            return false;

        size_t payload_length = length - payload_offset;
        if(payload_length != content_length)
            return false;

        if(!*post_payload)
            *post_payload = buffer_create(payload_length + 1, NULL);

        buffer_contents_replace(*post_payload, begin + payload_offset, payload_length);
        (*post_payload)->content_type = url_request_content_type(begin);
        return true;

invalid_content_length:
        *expected_size = SIZE_MAX;
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

int url_unittest(void) {
    static const struct {
        const char *url;
        const char *decoded; // NULL when decoding must fail
    } tests[] = {
        { "plain",      "plain" },
        { "a+b",        "a b" },
        { "%41",        "A" },
        { "%2Fapi%2f",  "/api/" },
        { "%C3%A9",     "\xC3\xA9" },
        { "%C3xA9",     NULL },     // continuation byte without '%'
        { "%1g",        NULL },     // non-hex digit
        { "%zz",        NULL },
        { "%4",         NULL },     // truncated escape
        { "%00",        NULL },
        { "%C3%28",     NULL },     // invalid continuation byte
        { "%E2%82",     NULL },     // truncated multibyte character
        { "%C0%AF",     NULL },     // overlong encoding
    };

    int errors = 0;
    for(size_t i = 0; i < sizeof(tests) / sizeof(tests[0]); i++) {
        char out[64];
        const char *got = url_decode_r(out, tests[i].url, sizeof(out));

        bool ok = tests[i].decoded ? (got && strcmp(got, tests[i].decoded) == 0) : (got == NULL);
        if(!ok) {
            fprintf(stderr, "URL: decoding '%s' gave '%s', expected '%s'\n",
                    tests[i].url, got ? got : "(failure)", tests[i].decoded ? tests[i].decoded : "(failure)");
            errors++;
        }
    }

    fprintf(stderr, "URL: %zu decode tests, %d failed\n", sizeof(tests) / sizeof(tests[0]), errors);
    return errors;
}
