// SPDX-License-Identifier: GPL-3.0-or-later

#include "nd_log-internals.h"

int64_t log_field_to_int64(struct log_field *lf) {

    //  --- FIELD_PARSER_VERSIONS ---
    //
    // IMPORTANT:
    // THERE ARE MULTIPLE VERSIONS OF THIS CODE
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
    const char *s = NULL;

    switch(lf->entry.type) {
        default:
        case NDFT_UUID:
        case NDFT_UNSET:
            return 0;

        case NDFT_TXT:
            s = lf->entry.txt;
            break;

        case NDFT_STR:
            s = string2str(lf->entry.str);
            break;

        case NDFT_BFR:
            s = buffer_tostring(lf->entry.bfr);
            break;

        case NDFT_CALLBACK:
            tmp = buffer_create(0, NULL);

            if(lf->entry.cb.formatter(tmp, lf->entry.cb.formatter_data))
                s = buffer_tostring(tmp);
            else
                s = NULL;
            break;

        case NDFT_U64:
            return (int64_t)lf->entry.u64;

        case NDFT_I64:
            return (int64_t)lf->entry.i64;

        case NDFT_DBL:
            return (int64_t)lf->entry.dbl;
    }

    if(s && *s)
        return str2ll(s, NULL);

    return 0;
}

uint64_t log_field_to_uint64(struct log_field *lf) {

    //  --- FIELD_PARSER_VERSIONS ---
    //
    // IMPORTANT:
    // THERE ARE MULTIPLE VERSIONS OF THIS CODE
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
    const char *s = NULL;

    switch(lf->entry.type) {
        default:
        case NDFT_UUID:
        case NDFT_UNSET:
            return 0;

        case NDFT_TXT:
            s = lf->entry.txt;
            break;

        case NDFT_STR:
            s = string2str(lf->entry.str);
            break;

        case NDFT_BFR:
            s = buffer_tostring(lf->entry.bfr);
            break;

        case NDFT_CALLBACK:
            tmp = buffer_create(0, NULL);

            if(lf->entry.cb.formatter(tmp, lf->entry.cb.formatter_data))
                s = buffer_tostring(tmp);
            else
                s = NULL;
            break;

        case NDFT_U64:
            return lf->entry.u64;

        case NDFT_I64:
            return lf->entry.i64;

        case NDFT_DBL:
            return (uint64_t) lf->entry.dbl;
    }

    if(s && *s)
        return str2uint64_t(s, NULL);

    return 0;
}

const char *log_field_strdupz(struct log_field *lf) {

    //  --- FIELD_PARSER_VERSIONS ---
    //
    // IMPORTANT:
    // THERE ARE MULTIPLE VERSIONS OF THIS CODE
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
    const char *s = NULL;
    char buf[DOUBLE_MAX_LENGTH];

    if(lf->logfmt_annotator)
         s = lf->logfmt_annotator(lf);
    else {
        switch (lf->entry.type) {
            default:
            case NDFT_UNSET:
                return NULL;

            case NDFT_UUID:
                uuid_unparse_lower_compact(*lf->entry.uuid, buf);
                s = buf;
                break;

            case NDFT_TXT:
                s = lf->entry.txt;
                break;

            case NDFT_STR:
                s = string2str(lf->entry.str);
                break;

            case NDFT_BFR:
                s = buffer_tostring(lf->entry.bfr);
                break;

            case NDFT_CALLBACK:
                tmp = buffer_create(0, NULL);

                if (lf->entry.cb.formatter(tmp, lf->entry.cb.formatter_data))
                    s = buffer_tostring(tmp);
                else
                    s = NULL;
                break;

            case NDFT_U64:
                print_uint64(buf, lf->entry.u64);
                s = buf;
                break;

            case NDFT_I64:
                print_int64(buf, lf->entry.i64);
                s = buf;
                break;

            case NDFT_DBL:
                print_netdata_double(buf, lf->entry.dbl);
                break;
        }
    }

    if(s && *s)
        return strdupz(s);

    return NULL;
}
