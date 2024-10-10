// SPDX-License-Identifier: GPL-3.0-or-later

#include "nd_log-internals.h"

void nd_logger_json(BUFFER *wb, struct log_field *fields, size_t fields_max) {

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

    buffer_json_initialize(wb, "\"", "\"", 0, true, BUFFER_JSON_OPTIONS_MINIFY);
    CLEAN_BUFFER *tmp = NULL;

    for (size_t i = 0; i < fields_max; i++) {
        if (!fields[i].entry.set || !fields[i].logfmt)
            continue;

        const char *key = fields[i].logfmt;

        const char *s = NULL;
        switch(fields[i].entry.type) {
            case NDFT_TXT:
                s = fields[i].entry.txt;
                break;
            case NDFT_STR:
                s = string2str(fields[i].entry.str);
                break;
            case NDFT_BFR:
                s = buffer_tostring(fields[i].entry.bfr);
                break;
            case NDFT_U64:
                buffer_json_member_add_uint64(wb, key, fields[i].entry.u64);
                break;
            case NDFT_I64:
                buffer_json_member_add_int64(wb, key, fields[i].entry.i64);
                break;
            case NDFT_DBL:
                buffer_json_member_add_double(wb, key, fields[i].entry.dbl);
                break;
            case NDFT_UUID:
                if(!uuid_is_null(*fields[i].entry.uuid)) {
                    char u[UUID_COMPACT_STR_LEN];
                    uuid_unparse_lower_compact(*fields[i].entry.uuid, u);
                    buffer_json_member_add_string(wb, key, u);
                }
                break;
            case NDFT_CALLBACK: {
                if(!tmp)
                    tmp = buffer_create(1024, NULL);
                else
                    buffer_flush(tmp);
                if(fields[i].entry.cb.formatter(tmp, fields[i].entry.cb.formatter_data))
                    s = buffer_tostring(tmp);
                else
                    s = NULL;
            }
            break;
            default:
                s = "UNHANDLED";
                break;
        }

        if(s && *s)
            buffer_json_member_add_string(wb, key, s);
    }

    buffer_json_finalize(wb);
}
