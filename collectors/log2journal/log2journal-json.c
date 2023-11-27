// SPDX-License-Identifier: GPL-3.0-or-later

#include "log2journal.h"

#define ERROR_LINE_MAX 1024
#define KEY_MAX 1024
#define JSON_DEPTH_MAX 100

struct log_json_state {
    const char *line;
    size_t pos;
    char msg[ERROR_LINE_MAX];

    char key[KEY_MAX];
    char *key_stack[JSON_DEPTH_MAX];
    size_t depth;

    struct log_job *jb;
};

static inline bool json_parse_object(LOG_JSON_STATE *js);
static inline bool json_parse_array(LOG_JSON_STATE *js);

#define json_current_pos(js) &(js)->line[(js)->pos]
#define json_consume_char(js) ++(js)->pos

static inline void json_process_key_value(LOG_JSON_STATE *js, const char *value, size_t len) {
    jb_send_extracted_key_value(js->jb, js->key, value, len);
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
             "JSON PARSER: character '%c' is not one of the expected characters (%s), at pos %zu",
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
                 "JSON PARSER: expected 'null', found '%.4s' at position %zu", s, js->pos);
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
                 "JSON PARSER: expected 'true', found '%.4s' at position %zu", s, js->pos);
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
                 "JSON PARSER: expected 'false', found '%.4s' at position %zu", s, js->pos);
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
            snprintf(js->msg, sizeof(js->msg), "JSON PARSER: truncated number value at pos %zu", js->pos);
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
                snprintf(js->msg, sizeof(js->msg), "JSON PARSER: truncated fractional part at pos %zu", js->pos);
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
                snprintf(js->msg, sizeof(js->msg), "JSON PARSER: truncated exponent at pos %zu", js->pos);
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
        snprintf(js->msg, sizeof(js->msg), "JSON PARSER: invalid number format at pos %zu", js->pos);
        return false;
    }
}

static bool encode_utf8(unsigned codepoint, char **d, size_t *remaining) {
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

static inline bool json_parse_string(LOG_JSON_STATE *js) {
    static __thread char value[MAX_VALUE_LEN];

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
                    c = '\n';
                    s++;
                    break;
                case 't':
                    c = '\t';
                    s++;
                    break;
                case 'b':
                    c = '\b';
                    s++;
                    break;
                case 'f':
                    c = '\f';
                    s++;
                    break;
                case 'r':
                    c = '\r';
                    s++;
                    break;
                case 'u':
                    if(isxdigit(s[1]) && isxdigit(s[2]) && isxdigit(s[3]) && isxdigit(s[4])) {
                        char b[5] = {
                                [0] = s[1],
                                [1] = s[2],
                                [2] = s[3],
                                [3] = s[4],
                                [4] = '\0',
                        };
                        unsigned codepoint = strtoul(b, NULL, 16);
                        if(encode_utf8(codepoint, &d, &remaining)) {
                            s += 5;
                            continue;
                        }
                        else {
                            *d++ = '\\';
                            remaining--;
                            c = *s++;
                        }
                    }
                    else {
                        *d++ = '\\';
                        remaining--;
                        c = *s++;
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
                     "JSON PARSER: truncated string value at pos %zu", js->pos);
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
    static const char valid_journal_key_chars[256] = {
            // control characters
            [0] = '\0', [1] = '_', [2] = '_', [3] = '_', [4] = '_', [5] = '_', [6] = '_', [7] = '_',
            [8] = '_', [9] = '_', [10] = '_', [11] = '_', [12] = '_', [13] = '_', [14] = '_', [15] = '_',
            [16] = '_', [17] = '_', [18] = '_', [19] = '_', [20] = '_', [21] = '_', [22] = '_', [23] = '_',
            [24] = '_', [25] = '_', [26] = '_', [27] = '_', [28] = '_', [29] = '_', [30] = '_', [31] = '_',

            // symbols
            [' '] = '_', ['!'] = '_', ['"'] = '_', ['#'] = '_', ['$'] = '_', ['%'] = '_', ['&'] = '_', ['\''] = '_',
            ['('] = '_', [')'] = '_', ['*'] = '_', ['+'] = '_', [','] = '_', ['-'] = '_', ['.'] = '_', ['/'] = '_',

            // numbers
            ['0'] = '0', ['1'] = '1', ['2'] = '2', ['3'] = '3', ['4'] = '4', ['5'] = '5', ['6'] = '6', ['7'] = '7',
            ['8'] = '8', ['9'] = '9',

            // symbols
            [':'] = '_', [';'] = '_', ['<'] = '_', ['='] = '_', ['>'] = '_', ['?'] = '_', ['@'] = '_',

            // capitals
            ['A'] = 'A', ['B'] = 'B', ['C'] = 'C', ['D'] = 'D', ['E'] = 'E', ['F'] = 'F', ['G'] = 'G', ['H'] = 'H',
            ['I'] = 'I', ['J'] = 'J', ['K'] = 'K', ['L'] = 'L', ['M'] = 'M', ['N'] = 'N', ['O'] = 'O', ['P'] = 'P',
            ['Q'] = 'Q', ['R'] = 'R', ['S'] = 'S', ['T'] = 'T', ['U'] = 'U', ['V'] = 'V', ['W'] = 'W', ['X'] = 'X',
            ['Y'] = 'Y', ['Z'] = 'Z',

            // symbols
            ['['] = '_', ['\\'] = '_', [']'] = '_', ['^'] = '_', ['_'] = '_', ['`'] = '_',

            // lower to upper
            ['a'] = 'A', ['b'] = 'B', ['c'] = 'C', ['d'] = 'D', ['e'] = 'E', ['f'] = 'F', ['g'] = 'G', ['h'] = 'H',
            ['i'] = 'I', ['j'] = 'J', ['k'] = 'K', ['l'] = 'L', ['m'] = 'M', ['n'] = 'N', ['o'] = 'O', ['p'] = 'P',
            ['q'] = 'Q', ['r'] = 'R', ['s'] = 'S', ['t'] = 'T', ['u'] = 'U', ['v'] = 'V', ['w'] = 'W', ['x'] = 'X',
            ['y'] = 'Y', ['z'] = 'Z',

            // symbols
            ['{'] = '_', ['|'] = '_', ['}'] = '_', ['~'] = '_', [127] = '_', // Delete (DEL)

            // Extended ASCII characters (128-255) set to underscore
            [128] = '_', [129] = '_', [130] = '_', [131] = '_', [132] = '_', [133] = '_', [134] = '_', [135] = '_',
            [136] = '_', [137] = '_', [138] = '_', [139] = '_', [140] = '_', [141] = '_', [142] = '_', [143] = '_',
            [144] = '_', [145] = '_', [146] = '_', [147] = '_', [148] = '_', [149] = '_', [150] = '_', [151] = '_',
            [152] = '_', [153] = '_', [154] = '_', [155] = '_', [156] = '_', [157] = '_', [158] = '_', [159] = '_',
            [160] = '_', [161] = '_', [162] = '_', [163] = '_', [164] = '_', [165] = '_', [166] = '_', [167] = '_',
            [168] = '_', [169] = '_', [170] = '_', [171] = '_', [172] = '_', [173] = '_', [174] = '_', [175] = '_',
            [176] = '_', [177] = '_', [178] = '_', [179] = '_', [180] = '_', [181] = '_', [182] = '_', [183] = '_',
            [184] = '_', [185] = '_', [186] = '_', [187] = '_', [188] = '_', [189] = '_', [190] = '_', [191] = '_',
            [192] = '_', [193] = '_', [194] = '_', [195] = '_', [196] = '_', [197] = '_', [198] = '_', [199] = '_',
            [200] = '_', [201] = '_', [202] = '_', [203] = '_', [204] = '_', [205] = '_', [206] = '_', [207] = '_',
            [208] = '_', [209] = '_', [210] = '_', [211] = '_', [212] = '_', [213] = '_', [214] = '_', [215] = '_',
            [216] = '_', [217] = '_', [218] = '_', [219] = '_', [220] = '_', [221] = '_', [222] = '_', [223] = '_',
            [224] = '_', [225] = '_', [226] = '_', [227] = '_', [228] = '_', [229] = '_', [230] = '_', [231] = '_',
            [232] = '_', [233] = '_', [234] = '_', [235] = '_', [236] = '_', [237] = '_', [238] = '_', [239] = '_',
            [240] = '_', [241] = '_', [242] = '_', [243] = '_', [244] = '_', [245] = '_', [246] = '_', [247] = '_',
            [248] = '_', [249] = '_', [250] = '_', [251] = '_', [252] = '_', [253] = '_', [254] = '_', [255] = '_',
    };

    if (!json_expect_char_after_white_space(js, "\""))
        return false;

    if(js->depth >= JSON_DEPTH_MAX - 1) {
        snprintf(js->msg, sizeof(js->msg),
                 "JSON PARSER: object too deep, at pos %zu", js->pos);
        return false;
    }

    json_consume_char(js);

    char *d = js->key_stack[js->depth];
    if(js->depth)
        *d++ = '_';

    size_t remaining = sizeof(js->key) - (d - js->key);

    const char *s = json_current_pos(js);
    char last_c = '\0';
    while(*s && *s != '\"') {
        char c;

        if (*s == '\\') {
            s++;
            c = (char)((*s == 'u') ? '_' : valid_journal_key_chars[(unsigned char)*s]);
            s += (*s == 'u') ? 5 : 1;
        }
        else
            c = valid_journal_key_chars[(unsigned char)*s++];

        if(c == '_' && last_c == '_')
            continue;
        else {
            if(remaining < 2) {
                snprintf(js->msg, sizeof(js->msg),
                         "JSON PARSER: key buffer full - keys are too long, at pos %zu", js->pos);
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

    js->key_stack[++js->depth] = d;

    return true;
}

static inline bool json_key_pop(LOG_JSON_STATE *js) {
    if(js->depth <= 0) {
        snprintf(js->msg, sizeof(js->msg),
                 "JSON PARSER: cannot pop a key at depth %zu, at pos %zu", js->depth, js->pos);
        return false;
    }

    char *k = js->key_stack[js->depth--];
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
             "JSON PARSER: unexpected character at pos %zu", js->pos);
    return false;
}

static inline bool json_key_index_and_push(LOG_JSON_STATE *js, size_t index) {
    char *d = js->key_stack[js->depth];
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
                     "JSON PARSER: key buffer full - keys are too long, at pos %zu", js->pos);
            return false;
        }

        *d++ = *t++;
        remaining--;
    }

    *d = '\0'; // Null-terminate the key
    js->key_stack[++js->depth] = d;

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

LOG_JSON_STATE *json_parser_create(struct log_job *jb) {
    LOG_JSON_STATE *js = mallocz(sizeof(LOG_JSON_STATE));
    memset(js, 0, sizeof(LOG_JSON_STATE));
    js->jb = jb;

    if(jb->prefix)
        copy_to_buffer(js->key, sizeof(js->key), js->jb->prefix, strlen(js->jb->prefix));

    js->key_stack[0] = &js->key[strlen(js->key)];

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
    js->key_stack[0][0] = '\0';
    js->depth = 0;

    if(!json_parse_object(js))
        return false;

    json_skip_spaces(js);
    const char *s = json_current_pos(js);

    if(*s) {
        snprintf(js->msg, sizeof(js->msg),
                 "JSON PARSER: excess characters found after document is finished, at pos %zu", js->pos);
        return false;
    }

    return true;
}

void json_test(void) {
    struct log_job jb = { .prefix = "NIGNX_" };
    LOG_JSON_STATE *json = json_parser_create(&jb);

    json_parse_document(json, "{\"value\":\"\\u\\u039A\\u03B1\\u03BB\\u03B7\\u03BC\\u03AD\\u03C1\\u03B1\"}");

    json_parser_destroy(json);
}
