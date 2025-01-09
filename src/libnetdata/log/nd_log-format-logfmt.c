// SPDX-License-Identifier: GPL-3.0-or-later

#include "nd_log-internals.h"

static bool needs_quotes_for_logfmt(const char *s)
{
    static bool safe_for_logfmt[256] = {
        [' '] =  true, ['!'] =  true, ['"'] =  false, ['#'] =  true, ['$'] =  true, ['%'] =  true, ['&'] =  true,
        ['\''] = true, ['('] =  true, [')'] =  true, ['*'] =  true, ['+'] =  true, [','] =  true, ['-'] =  true,
        ['.'] =  true, ['/'] =  true, ['0'] =  true, ['1'] =  true, ['2'] =  true, ['3'] =  true, ['4'] =  true,
        ['5'] =  true, ['6'] =  true, ['7'] =  true, ['8'] =  true, ['9'] =  true, [':'] =  true, [';'] =  true,
        ['<'] =  true, ['='] =  true, ['>'] =  true, ['?'] =  true, ['@'] =  true, ['A'] =  true, ['B'] =  true,
        ['C'] =  true, ['D'] =  true, ['E'] =  true, ['F'] =  true, ['G'] =  true, ['H'] =  true, ['I'] =  true,
        ['J'] =  true, ['K'] =  true, ['L'] =  true, ['M'] =  true, ['N'] =  true, ['O'] =  true, ['P'] =  true,
        ['Q'] =  true, ['R'] =  true, ['S'] =  true, ['T'] =  true, ['U'] =  true, ['V'] =  true, ['W'] =  true,
        ['X'] =  true, ['Y'] =  true, ['Z'] =  true, ['['] =  true, ['\\'] = false, [']'] =  true, ['^'] =  true,
        ['_'] =  true, ['`'] =  true, ['a'] =  true, ['b'] =  true, ['c'] =  true, ['d'] =  true, ['e'] =  true,
        ['f'] =  true, ['g'] =  true, ['h'] =  true, ['i'] =  true, ['j'] =  true, ['k'] =  true, ['l'] =  true,
        ['m'] =  true, ['n'] =  true, ['o'] =  true, ['p'] =  true, ['q'] =  true, ['r'] =  true, ['s'] =  true,
        ['t'] =  true, ['u'] =  true, ['v'] =  true, ['w'] =  true, ['x'] =  true, ['y'] =  true, ['z'] =  true,
        ['{'] =  true, ['|'] =  true, ['}'] =  true, ['~'] =  true, [0x7f] = true,
    };

    if(!*s)
        return true;

    while(*s) {
        if(*s == '=' || isspace((uint8_t)*s) || !safe_for_logfmt[(uint8_t)*s])
            return true;

        s++;
    }

    return false;
}

static void string_to_logfmt(BUFFER *wb, const char *s)
{
    bool spaces = needs_quotes_for_logfmt(s);

    if(spaces)
        buffer_fast_strcat(wb, "\"", 1);

    buffer_json_strcat(wb, s);

    if(spaces)
        buffer_fast_strcat(wb, "\"", 1);
}

void nd_logger_logfmt(BUFFER *wb, struct log_field *fields, size_t fields_max) {

    //  --- FIELD_PARSER_VERSIONS ---
    //
    // IMPORTANT:
    // THERE ARE 6 VERSIONS OF THIS CODE
    //
    // 1. journal (direct socket API),
    // 2. journal (libsystemd API),
    // 3. logfmt,
    // 4. json,
    // 5. convert to uint64
    // 6. convert to int64
    //
    // UPDATE ALL OF THEM FOR NEW FEATURES OR FIXES

    CLEAN_BUFFER *tmp = NULL;

    for (size_t i = 0; i < fields_max; i++) {
        if (!fields[i].entry.set || !fields[i].logfmt)
            continue;

        const char *key = fields[i].logfmt;

        if(fields[i].logfmt_annotator) {
            const char *s = fields[i].logfmt_annotator(&fields[i]);
            if(!s) continue;

            if(buffer_strlen(wb))
                buffer_fast_strcat(wb, " ", 1);

            buffer_strcat(wb, key);
            buffer_fast_strcat(wb, "=", 1);
            string_to_logfmt(wb, s);
        }
        else {
            if(buffer_strlen(wb))
                buffer_fast_strcat(wb, " ", 1);

            switch(fields[i].entry.type) {
                case NDFT_TXT:
                    if(*fields[i].entry.txt) {
                        buffer_strcat(wb, key);
                        buffer_fast_strcat(wb, "=", 1);
                        string_to_logfmt(wb, fields[i].entry.txt);
                    }
                    break;
                case NDFT_STR:
                    buffer_strcat(wb, key);
                    buffer_fast_strcat(wb, "=", 1);
                    string_to_logfmt(wb, string2str(fields[i].entry.str));
                    break;
                case NDFT_BFR:
                    if(buffer_strlen(fields[i].entry.bfr)) {
                        buffer_strcat(wb, key);
                        buffer_fast_strcat(wb, "=", 1);
                        string_to_logfmt(wb, buffer_tostring(fields[i].entry.bfr));
                    }
                    break;
                case NDFT_U64:
                    buffer_strcat(wb, key);
                    buffer_fast_strcat(wb, "=", 1);
                    buffer_print_uint64(wb, fields[i].entry.u64);
                    break;
                case NDFT_I64:
                    buffer_strcat(wb, key);
                    buffer_fast_strcat(wb, "=", 1);
                    buffer_print_int64(wb, fields[i].entry.i64);
                    break;
                case NDFT_DBL:
                    buffer_strcat(wb, key);
                    buffer_fast_strcat(wb, "=", 1);
                    buffer_print_netdata_double(wb, fields[i].entry.dbl);
                    break;
                case NDFT_UUID:
                    if(!uuid_is_null(*fields[i].entry.uuid)) {
                        char u[UUID_COMPACT_STR_LEN];
                        uuid_unparse_lower_compact(*fields[i].entry.uuid, u);
                        buffer_strcat(wb, key);
                        buffer_fast_strcat(wb, "=", 1);
                        buffer_fast_strcat(wb, u, sizeof(u) - 1);
                    }
                    break;
                case NDFT_CALLBACK: {
                    if(!tmp)
                        tmp = buffer_create(1024, NULL);
                    else
                        buffer_flush(tmp);
                    if(fields[i].entry.cb.formatter(tmp, fields[i].entry.cb.formatter_data)) {
                        buffer_strcat(wb, key);
                        buffer_fast_strcat(wb, "=", 1);
                        string_to_logfmt(wb, buffer_tostring(tmp));
                    }
                }
                break;
                default:
                    buffer_strcat(wb, "UNHANDLED");
                    break;
            }
        }
    }
}
