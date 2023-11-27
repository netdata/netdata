// SPDX-License-Identifier: GPL-3.0-or-later

#include "log2journal.h"

#define ERROR_LINE_MAX 1024
#define KEY_MAX 1024

struct logfmt_state {
    const char *line;
    size_t pos;
    char msg[ERROR_LINE_MAX];

    char key[KEY_MAX];
    size_t key_start;

    struct log_job *jb;
};

#define logfmt_current_pos(lfs) &(lfs)->line[(lfs)->pos]
#define logfmt_consume_char(lfs) ++(lfs)->pos

static inline void logfmt_process_key_value(LOGFMT_STATE *lfs, const char *value, size_t len) {
    jb_send_extracted_key_value(lfs->jb, lfs->key, value, len);
}

static inline void logfmt_skip_spaces(LOGFMT_STATE *lfs) {
    const char *s = logfmt_current_pos(lfs);
    const char *start = s;

    while(isspace(*s)) s++;

    lfs->pos += s - start;
}

static inline bool logftm_parse_value(LOGFMT_STATE *lfs) {
    static __thread char value[MAX_VALUE_LEN];

    char quote = '\0';
    const char *s = logfmt_current_pos(lfs);
    if(*s == '\"' || *s == '\'') {
        quote = *s;
        logfmt_consume_char(lfs);
    }

    value[0] = '\0';
    char *d = value;
    s = logfmt_current_pos(lfs);
    size_t remaining = sizeof(value);

    char end_char = (char)(quote == '\0' ? ' ' : quote);
    while (*s && *s != end_char) {
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
                default:
                    c = *s++;
                    break;
            }
        }
        else
            c = *s++;

        if(remaining < 2) {
            snprintf(lfs->msg, sizeof(lfs->msg),
                     "LOGFMT PARSER: truncated string value at pos %zu", lfs->pos);
            return false;
        }
        else {
            *d++ = c;
            remaining--;
        }
    }
    *d = '\0';
    lfs->pos += s - logfmt_current_pos(lfs);

    s = logfmt_current_pos(lfs);

    if(quote != '\0') {
        if (*s != quote) {
            snprintf(lfs->msg, sizeof(lfs->msg),
                     "LOGFMT PARSER: missing quote at pos %zu: '%s'",
                     lfs->pos, s);
            return false;
        }
        else
            logfmt_consume_char(lfs);
    }

    if(d > value)
        logfmt_process_key_value(lfs, value, d - value);

    return true;
}

static inline bool logfmt_parse_key(LOGFMT_STATE *lfs) {
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

    logfmt_skip_spaces(lfs);

    char *d = &lfs->key[lfs->key_start];

    size_t remaining = sizeof(lfs->key) - (d - lfs->key);

    const char *s = logfmt_current_pos(lfs);
    char last_c = '\0';
    while(*s && *s != '=') {
        char c;

        if (*s == '\\')
            s++;

        c = valid_journal_key_chars[(unsigned char)*s++];

        if(c == '_' && last_c == '_')
            continue;
        else {
            if(remaining < 2) {
                snprintf(lfs->msg, sizeof(lfs->msg),
                         "LOGFMT PARSER: key buffer full - keys are too long, at pos %zu", lfs->pos);
                return false;
            }
            *d++ = c;
            remaining--;
        }

        last_c = c;
    }
    *d = '\0';
    lfs->pos += s - logfmt_current_pos(lfs);

    s = logfmt_current_pos(lfs);
    if(*s != '=') {
        snprintf(lfs->msg, sizeof(lfs->msg),
                 "LOGFMT PARSER: key is missing the equal sign, at pos %zu", lfs->pos);
        return false;
    }

    logfmt_consume_char(lfs);

    return true;
}

LOGFMT_STATE *logfmt_parser_create(struct log_job *jb) {
    LOGFMT_STATE *lfs = mallocz(sizeof(LOGFMT_STATE));
    memset(lfs, 0, sizeof(LOGFMT_STATE));
    lfs->jb = jb;

    if(jb->prefix)
        lfs->key_start = copy_to_buffer(lfs->key, sizeof(lfs->key), lfs->jb->prefix, strlen(lfs->jb->prefix));

    return lfs;
}

void logfmt_parser_destroy(LOGFMT_STATE *lfs) {
    if(lfs)
        freez(lfs);
}

const char *logfmt_parser_error(LOGFMT_STATE *lfs) {
    return lfs->msg;
}

bool logfmt_parse_document(LOGFMT_STATE *lfs, const char *txt) {
    lfs->line = txt;
    lfs->pos = 0;
    lfs->msg[0] = '\0';

    const char *s;
    do {
        if(!logfmt_parse_key(lfs))
            return false;

        if(!logftm_parse_value(lfs))
            return false;

        logfmt_skip_spaces(lfs);

        s = logfmt_current_pos(lfs);
    } while(*s);

    return true;
}


void logfmt_test(void) {
    struct log_job jb = { .prefix = "NIGNX_" };
    LOGFMT_STATE *logfmt = logfmt_parser_create(&jb);

    logfmt_parse_document(logfmt, "x=1 y=2 z=\"3 \\ 4\" 5  ");

    logfmt_parser_destroy(logfmt);
}
