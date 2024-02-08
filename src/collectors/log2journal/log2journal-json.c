// SPDX-License-Identifier: GPL-3.0-or-later

#include "log2journal.h"

#define JSON_ERROR_LINE_MAX 1024
#define JSON_KEY_MAX 1024
#define JSON_DEPTH_MAX 100

struct log_json_state {
    LOG_JOB *jb;

    const char *line;
    uint32_t pos;
    uint32_t depth;
    char *stack[JSON_DEPTH_MAX];

    char key[JSON_KEY_MAX];
    char msg[JSON_ERROR_LINE_MAX];
};

static inline bool json_parse_object(LOG_JSON_STATE *js);
static inline bool json_parse_array(LOG_JSON_STATE *js);

#define json_current_pos(js) &(js)->line[(js)->pos]
#define json_consume_char(js) ++(js)->pos

static inline void json_process_key_value(LOG_JSON_STATE *js, const char *value, size_t len) {
    log_job_send_extracted_key_value(js->jb, js->key, value, len);
}

static inline void json_skip_spaces(LOG_JSON_STATE *js) {
    const char *s = json_current_pos(js);
    const char *start = s;

    while(isspace(*s)) s++;

    js->pos += s - start;
}

static inline bool json_expect_char_after_white_space(LOG_JSON_STATE *js, const char *expected) {
    json_skip_spaces(js);

    const char *s = json_current_pos(js);
    for(const char *e = expected; *e ;e++) {
        if (*s == *e)
            return true;
    }

    snprintf(js->msg, sizeof(js->msg),
             "JSON PARSER: character '%c' is not one of the expected characters (%s), at pos %u",
             *s ? *s : '?', expected, js->pos);

    return false;
}

static inline bool json_parse_null(LOG_JSON_STATE *js) {
    const char *s = json_current_pos(js);
    if (strncmp(s, "null", 4) == 0) {
        json_process_key_value(js, "null", 4);
        js->pos += 4;
        return true;
    }
    else {
        snprintf(js->msg, sizeof(js->msg),
                 "JSON PARSER: expected 'null', found '%.4s' at position %u", s, js->pos);
        return false;
    }
}

static inline bool json_parse_true(LOG_JSON_STATE *js) {
    const char *s = json_current_pos(js);
    if (strncmp(s, "true", 4) == 0) {
        json_process_key_value(js, "true", 4);
        js->pos += 4;
        return true;
    }
    else {
        snprintf(js->msg, sizeof(js->msg),
                 "JSON PARSER: expected 'true', found '%.4s' at position %u", s, js->pos);
        return false;
    }
}

static inline bool json_parse_false(LOG_JSON_STATE *js) {
    const char *s = json_current_pos(js);
    if (strncmp(s, "false", 5) == 0) {
        json_process_key_value(js, "false", 5);
        js->pos += 5;
        return true;
    }
    else {
        snprintf(js->msg, sizeof(js->msg),
                 "JSON PARSER: expected 'false', found '%.4s' at position %u", s, js->pos);
        return false;
    }
}

static inline bool json_parse_number(LOG_JSON_STATE *js) {
    static __thread char value[8192];

    value[0] = '\0';
    char *d = value;
    const char *s = json_current_pos(js);
    size_t remaining = sizeof(value) - 1; // Reserve space for null terminator

    // Optional minus sign
    if (*s == '-') {
        *d++ = *s++;
        remaining--;
    }

    // Digits before decimal point
    while (*s >= '0' && *s <= '9') {
        if (remaining < 2) {
            snprintf(js->msg, sizeof(js->msg), "JSON PARSER: truncated number value at position %u", js->pos);
            return false;
        }
        *d++ = *s++;
        remaining--;
    }

    // Decimal point and fractional part
    if (*s == '.') {
        *d++ = *s++;
        remaining--;

        while (*s >= '0' && *s <= '9') {
            if (remaining < 2) {
                snprintf(js->msg, sizeof(js->msg), "JSON PARSER: truncated fractional part at position %u", js->pos);
                return false;
            }
            *d++ = *s++;
            remaining--;
        }
    }

    // Exponent part
    if (*s == 'e' || *s == 'E') {
        *d++ = *s++;
        remaining--;

        // Optional sign in exponent
        if (*s == '+' || *s == '-') {
            *d++ = *s++;
            remaining--;
        }

        while (*s >= '0' && *s <= '9') {
            if (remaining < 2) {
                snprintf(js->msg, sizeof(js->msg), "JSON PARSER: truncated exponent at position %u", js->pos);
                return false;
            }
            *d++ = *s++;
            remaining--;
        }
    }

    *d = '\0';
    js->pos += d - value;

    if (d > value) {
        json_process_key_value(js, value, d - value);
        return true;
    } else {
        snprintf(js->msg, sizeof(js->msg), "JSON PARSER: invalid number format at position %u", js->pos);
        return false;
    }
}

static inline bool encode_utf8(unsigned codepoint, char **d, size_t *remaining) {
    if (codepoint <= 0x7F) {
        // 1-byte sequence
        if (*remaining < 2) return false; // +1 for the null
        *(*d)++ = (char)codepoint;
        (*remaining)--;
    }
    else if (codepoint <= 0x7FF) {
        // 2-byte sequence
        if (*remaining < 3) return false; // +1 for the null
        *(*d)++ = (char)(0xC0 | ((codepoint >> 6) & 0x1F));
        *(*d)++ = (char)(0x80 | (codepoint & 0x3F));
        (*remaining) -= 2;
    }
    else if (codepoint <= 0xFFFF) {
        // 3-byte sequence
        if (*remaining < 4) return false; // +1 for the null
        *(*d)++ = (char)(0xE0 | ((codepoint >> 12) & 0x0F));
        *(*d)++ = (char)(0x80 | ((codepoint >> 6) & 0x3F));
        *(*d)++ = (char)(0x80 | (codepoint & 0x3F));
        (*remaining) -= 3;
    }
    else if (codepoint <= 0x10FFFF) {
        // 4-byte sequence
        if (*remaining < 5) return false; // +1 for the null
        *(*d)++ = (char)(0xF0 | ((codepoint >> 18) & 0x07));
        *(*d)++ = (char)(0x80 | ((codepoint >> 12) & 0x3F));
        *(*d)++ = (char)(0x80 | ((codepoint >> 6) & 0x3F));
        *(*d)++ = (char)(0x80 | (codepoint & 0x3F));
        (*remaining) -= 4;
    }
    else
        // Invalid code point
        return false;

    return true;
}

size_t parse_surrogate(const char *s, char *d, size_t *remaining) {
    if (s[0] != '\\' || (s[1] != 'u' && s[1] != 'U')) {
        return 0; // Not a valid Unicode escape sequence
    }

    char hex[9] = {0}; // Buffer for the hexadecimal value
    unsigned codepoint;

    if (s[1] == 'u') {
        // Handle \uXXXX
        if (!isxdigit(s[2]) || !isxdigit(s[3]) || !isxdigit(s[4]) || !isxdigit(s[5])) {
            return 0; // Not a valid \uXXXX sequence
        }

        hex[0] = s[2];
        hex[1] = s[3];
        hex[2] = s[4];
        hex[3] = s[5];
        codepoint = (unsigned)strtoul(hex, NULL, 16);

        if (codepoint >= 0xD800 && codepoint <= 0xDBFF) {
            // Possible start of surrogate pair
            if (s[6] == '\\' && s[7] == 'u' && isxdigit(s[8]) && isxdigit(s[9]) &&
                isxdigit(s[10]) && isxdigit(s[11])) {
                // Valid low surrogate
                unsigned low_surrogate = strtoul(&s[8], NULL, 16);
                if (low_surrogate < 0xDC00 || low_surrogate > 0xDFFF) {
                    return 0; // Invalid low surrogate
                }
                codepoint = 0x10000 + ((codepoint - 0xD800) << 10) + (low_surrogate - 0xDC00);
                return encode_utf8(codepoint, &d, remaining) ? 12 : 0; // \uXXXX\uXXXX
            }
        }

        // Single \uXXXX
        return encode_utf8(codepoint, &d, remaining) ? 6 : 0;
    }
    else {
        // Handle \UXXXXXXXX
        for (int i = 2; i < 10; i++) {
            if (!isxdigit(s[i])) {
                return 0; // Not a valid \UXXXXXXXX sequence
            }
            hex[i - 2] = s[i];
        }
        codepoint = (unsigned)strtoul(hex, NULL, 16);
        return encode_utf8(codepoint, &d, remaining) ? 10 : 0; // \UXXXXXXXX
    }
}

static inline void copy_newline(LOG_JSON_STATE *js __maybe_unused, char **d, size_t *remaining) {
    if(*remaining > 3) {
        *(*d)++ = '\\';
        *(*d)++ = 'n';
        (*remaining) -= 2;
    }
}

static inline void copy_tab(LOG_JSON_STATE *js __maybe_unused, char **d, size_t *remaining) {
    if(*remaining > 3) {
        *(*d)++ = '\\';
        *(*d)++ = 't';
        (*remaining) -= 2;
    }
}

static inline bool json_parse_string(LOG_JSON_STATE *js) {
    static __thread char value[JOURNAL_MAX_VALUE_LEN];

    if(!json_expect_char_after_white_space(js, "\""))
        return false;

    json_consume_char(js);

    value[0] = '\0';
    char *d = value;
    const char *s = json_current_pos(js);
    size_t remaining = sizeof(value);

    while (*s && *s != '"') {
        char c;

        if (*s == '\\') {
            s++;

            switch (*s) {
                case 'n':
                    copy_newline(js, &d, &remaining);
                    s++;
                    continue;

                case 't':
                    copy_tab(js, &d, &remaining);
                    s++;
                    continue;

                case 'f':
                case 'b':
                case 'r':
                    c = ' ';
                    s++;
                    break;

                case 'u': {
                        size_t old_remaining = remaining;
                        size_t consumed = parse_surrogate(s - 1, d, &remaining);
                        if (consumed > 0) {
                            s += consumed - 1; // -1 because we already incremented s after '\\'
                            d += old_remaining - remaining;
                            continue;
                        }
                        else {
                            *d++ = '\\';
                            remaining--;
                            c = *s++;
                        }
                    }
                    break;

                default:
                    c = *s++;
                    break;
            }
        }
        else
            c = *s++;

        if(remaining < 2) {
            snprintf(js->msg, sizeof(js->msg),
                     "JSON PARSER: truncated string value at position %u", js->pos);
            return false;
        }
        else {
            *d++ = c;
            remaining--;
        }
    }
    *d = '\0';
    js->pos += s - json_current_pos(js);

    if(!json_expect_char_after_white_space(js, "\""))
        return false;

    json_consume_char(js);

    if(d > value)
        json_process_key_value(js, value, d - value);

    return true;
}

static inline bool json_parse_key_and_push(LOG_JSON_STATE *js) {
    if (!json_expect_char_after_white_space(js, "\""))
        return false;

    if(js->depth >= JSON_DEPTH_MAX - 1) {
        snprintf(js->msg, sizeof(js->msg),
                 "JSON PARSER: object too deep, at position %u", js->pos);
        return false;
    }

    json_consume_char(js);

    char *d = js->stack[js->depth];
    if(js->depth)
        *d++ = '_';

    size_t remaining = sizeof(js->key) - (d - js->key);

    const char *s = json_current_pos(js);
    char last_c = '\0';
    while(*s && *s != '\"') {
        char c;

        if (*s == '\\') {
            s++;
            c = (char)((*s == 'u') ? '_' : journal_key_characters_map[(unsigned char)*s]);
            s += (*s == 'u') ? 5 : 1;
        }
        else
            c = journal_key_characters_map[(unsigned char)*s++];

        if(c == '_' && last_c == '_')
            continue;
        else {
            if(remaining < 2) {
                snprintf(js->msg, sizeof(js->msg),
                         "JSON PARSER: key buffer full - keys are too long, at position %u", js->pos);
                return false;
            }
            *d++ = c;
            remaining--;
        }

        last_c = c;
    }
    *d = '\0';
    js->pos += s - json_current_pos(js);

    if (!json_expect_char_after_white_space(js, "\""))
        return false;

    json_consume_char(js);

    js->stack[++js->depth] = d;

    return true;
}

static inline bool json_key_pop(LOG_JSON_STATE *js) {
    if(js->depth <= 0) {
        snprintf(js->msg, sizeof(js->msg),
                 "JSON PARSER: cannot pop a key at depth %u, at position %u", js->depth, js->pos);
        return false;
    }

    char *k = js->stack[js->depth--];
    *k = '\0';
    return true;
}

static inline bool json_parse_value(LOG_JSON_STATE *js) {
    if(!json_expect_char_after_white_space(js, "-.0123456789tfn\"{["))
        return false;

    const char *s = json_current_pos(js);
    switch(*s) {
        case '-':
        case '0':
        case '1':
        case '2':
        case '3':
        case '4':
        case '5':
        case '6':
        case '7':
        case '8':
        case '9':
            return json_parse_number(js);

        case 't':
            return json_parse_true(js);

        case 'f':
            return json_parse_false(js);

        case 'n':
            return json_parse_null(js);

        case '"':
            return json_parse_string(js);

        case '{':
            return json_parse_object(js);

        case '[':
            return json_parse_array(js);
    }

    snprintf(js->msg, sizeof(js->msg),
             "JSON PARSER: unexpected character at position %u", js->pos);
    return false;
}

static inline bool json_key_index_and_push(LOG_JSON_STATE *js, size_t index) {
    char *d = js->stack[js->depth];
    if(js->depth > 0) {
        *d++ = '_';
    }

    // Convert index to string manually
    char temp[32];
    char *t = temp + sizeof(temp) - 1; // Start at the end of the buffer
    *t = '\0';

    do {
        *--t = (char)((index % 10) + '0');
        index /= 10;
    } while (index > 0);

    size_t remaining = sizeof(js->key) - (d - js->key);

    // Append the index to the key
    while (*t) {
        if(remaining < 2) {
            snprintf(js->msg, sizeof(js->msg),
                     "JSON PARSER: key buffer full - keys are too long, at position %u", js->pos);
            return false;
        }

        *d++ = *t++;
        remaining--;
    }

    *d = '\0'; // Null-terminate the key
    js->stack[++js->depth] = d;

    return true;
}

static inline bool json_parse_array(LOG_JSON_STATE *js) {
    if(!json_expect_char_after_white_space(js, "["))
        return false;

    json_consume_char(js);

    size_t index = 0;
    do {
        if(!json_key_index_and_push(js, index))
            return false;

        if(!json_parse_value(js))
            return false;

        json_key_pop(js);

        if(!json_expect_char_after_white_space(js, ",]"))
            return false;

        const char *s = json_current_pos(js);
        json_consume_char(js);
        if(*s == ',') {
            index++;
            continue;
        }
        else // }
            break;

    } while(true);

    return true;
}

static inline bool json_parse_object(LOG_JSON_STATE *js) {
    if(!json_expect_char_after_white_space(js, "{"))
        return false;

    json_consume_char(js);

    do {
        if (!json_expect_char_after_white_space(js, "\""))
            return false;

        if(!json_parse_key_and_push(js))
            return false;

        if(!json_expect_char_after_white_space(js, ":"))
            return false;

        json_consume_char(js);

        if(!json_parse_value(js))
            return false;

        json_key_pop(js);

        if(!json_expect_char_after_white_space(js, ",}"))
            return false;

        const char *s = json_current_pos(js);
        json_consume_char(js);
        if(*s == ',')
            continue;
        else // }
            break;

    } while(true);

    return true;
}

LOG_JSON_STATE *json_parser_create(LOG_JOB *jb) {
    LOG_JSON_STATE *js = mallocz(sizeof(LOG_JSON_STATE));
    memset(js, 0, sizeof(LOG_JSON_STATE));
    js->jb = jb;

    if(jb->prefix)
        copy_to_buffer(js->key, sizeof(js->key), js->jb->prefix, strlen(js->jb->prefix));

    js->stack[0] = &js->key[strlen(js->key)];

    return js;
}

void json_parser_destroy(LOG_JSON_STATE *js) {
    if(js)
        freez(js);
}

const char *json_parser_error(LOG_JSON_STATE *js) {
    return js->msg;
}

bool json_parse_document(LOG_JSON_STATE *js, const char *txt) {
    js->line = txt;
    js->pos = 0;
    js->msg[0] = '\0';
    js->stack[0][0] = '\0';
    js->depth = 0;

    if(!json_parse_object(js))
        return false;

    json_skip_spaces(js);
    const char *s = json_current_pos(js);

    if(*s) {
        snprintf(js->msg, sizeof(js->msg),
                 "JSON PARSER: excess characters found after document is finished, at position %u", js->pos);
        return false;
    }

    return true;
}

void json_test(void) {
    LOG_JOB jb = { .prefix = "NIGNX_" };
    LOG_JSON_STATE *json = json_parser_create(&jb);

    json_parse_document(json, "{\"value\":\"\\u\\u039A\\u03B1\\u03BB\\u03B7\\u03BC\\u03AD\\u03C1\\u03B1\"}");

    json_parser_destroy(json);
}
