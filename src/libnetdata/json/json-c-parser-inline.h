// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_JSON_C_PARSER_INLINE_H
#define NETDATA_JSON_C_PARSER_INLINE_H

#define JSONC_PARSE_BOOL_OR_ERROR_AND_RETURN(jobj, path, member, dst, error, required) do {                     \
    json_object *_j;                                                                                            \
    if (json_object_object_get_ex(jobj, member, &_j) && json_object_is_type(_j, json_type_boolean))             \
        dst = json_object_get_boolean(_j);                                                                      \
    else if(required) {                                                                                         \
        buffer_sprintf(error, "missing or invalid type for '%s.%s' boolean", path, member);                     \
        return false;                                                                                           \
    }                                                                                                           \
} while(0)

#define JSONC_PARSE_TXT2STRING_OR_ERROR_AND_RETURN(jobj, path, member, dst, error, required) do {               \
    json_object *_j;                                                                                            \
    if (json_object_object_get_ex(jobj, member, &_j) && json_object_is_type(_j, json_type_string)) {            \
        string_freez(dst);                                                                                      \
        dst = string_strdupz(json_object_get_string(_j));                                                       \
    }                                                                                                           \
    else if(required) {                                                                                         \
        buffer_sprintf(error, "missing or invalid type for '%s.%s' string", path, member);                      \
        return false;                                                                                           \
    }                                                                                                           \
} while(0)

#define JSONC_PARSE_TXT2STRDUPZ_OR_ERROR_AND_RETURN(jobj, path, member, dst, error, required) do {              \
    json_object *_j;                                                                                            \
    if (json_object_object_get_ex(jobj, member, &_j) && json_object_is_type(_j, json_type_string)) {            \
        freez((void *)dst);                                                                                     \
        dst = strdupz(json_object_get_string(_j));                                                              \
    }                                                                                                           \
    else if(required) {                                                                                         \
        buffer_sprintf(error, "missing or invalid type for '%s.%s' string", path, member);                      \
        return false;                                                                                           \
    }                                                                                                           \
} while(0)

#define JSONC_PARSE_TXT2UUID_OR_ERROR_AND_RETURN(jobj, path, member, dst, error, required) do {                 \
    json_object *_j;                                                                                            \
    if (json_object_object_get_ex(jobj, member, &_j)) {                                                         \
        if (json_object_is_type(_j, json_type_string)) {                                                        \
            if (uuid_parse(json_object_get_string(_j), dst) != 0) {                                             \
                if(required) {                                                                                  \
                    buffer_sprintf(error, "invalid UUID '%s.%s'", path, member);                                \
                    return false;                                                                               \
                }                                                                                               \
                else                                                                                            \
                    uuid_clear(dst);                                                                            \
            }                                                                                                   \
        }                                                                                                       \
        else if (json_object_is_type(_j, json_type_null)) {                                                     \
            uuid_clear(dst);                                                                                    \
        }                                                                                                       \
        else if (required) {                                                                                    \
            buffer_sprintf(error, "expected UUID or null '%s.%s'", path, member);                               \
            return false;                                                                                       \
        }                                                                                                       \
    }                                                                                                           \
    else if (required) {                                                                                        \
        buffer_sprintf(error, "missing UUID '%s.%s'", path, member);                                            \
        return false;                                                                                           \
    }                                                                                                           \
} while(0)

#define JSONC_PARSE_TXT2BUFFER_OR_ERROR_AND_RETURN(jobj, path, member, dst, error, required) do {               \
    json_object *_j;                                                                                            \
    if (json_object_object_get_ex(jobj, member, &_j) && json_object_is_type(_j, json_type_string)) {            \
        const char *_s = json_object_get_string(_j);                                                            \
        if(!_s || !*_s) {                                                                                       \
            buffer_free(dst);                                                                                   \
            dst = NULL;                                                                                         \
        }                                                                                                       \
        else {                                                                                                  \
            if (dst)                                                                                            \
                buffer_flush(dst);                                                                              \
            else                                                                                                \
                dst = buffer_create(0, NULL);                                                                   \
            if (_s && *_s)                                                                                      \
                buffer_strcat(dst, _s);                                                                         \
        }                                                                                                       \
    }                                                                                                           \
    else if(required) {                                                                                         \
        buffer_sprintf(error, "missing or invalid type for '%s.%s' string", path, member);                      \
        return false;                                                                                           \
    }                                                                                                           \
} while(0)

#define JSONC_PARSE_TXT2PATTERN_OR_ERROR_AND_RETURN(jobj, path, member, dst, error, required) do {              \
    json_object *_j;                                                                                            \
    if (json_object_object_get_ex(jobj, member, &_j) && json_object_is_type(_j, json_type_string)) {            \
        string_freez(dst);                                                                                      \
        const char *_v = json_object_get_string(_j);                                                            \
        if(strcmp(_v, "*") == 0)                                                                                \
            dst = NULL;                                                                                         \
        else                                                                                                    \
            dst = string_strdupz(_v);                                                                           \
    }                                                                                                           \
    else if(required) {                                                                                         \
        buffer_sprintf(error, "missing or invalid type for '%s.%s' string", path, member);                      \
        return false;                                                                                           \
    }                                                                                                           \
} while(0)

#define JSONC_PARSE_TXT2EXPRESSION_OR_ERROR_AND_RETURN(jobj, path, member, dst, error, required) do {           \
    json_object *_j;                                                                                            \
    if (json_object_object_get_ex(jobj, member, &_j) && json_object_is_type(_j, json_type_string)) {            \
        const char *_t = json_object_get_string(_j);                                                            \
        if(_t && *_t && strcmp(_t, "*") != 0) {                                                                 \
            const char *_failed_at = NULL;                                                                      \
            int _err = 0;                                                                                       \
            expression_free(dst);                                                                               \
            dst = expression_parse(_t, &_failed_at, &_err);                                                     \
            if(!dst) {                                                                                          \
                buffer_sprintf(error, "expression '%s.%s' has a non-parseable expression '%s': %s at '%s'",     \
                               path, member, _t, expression_strerror(_err), _failed_at);                        \
                return false;                                                                                   \
            }                                                                                                   \
        }                                                                                                       \
    }                                                                                                           \
    else if(required) {                                                                                         \
        buffer_sprintf(error, "missing or invalid type for '%s.%s' expression", path, member);                  \
        return false;                                                                                           \
    }                                                                                                           \
} while(0)

#define JSONC_PARSE_ARRAY_OF_TXT2BITMAP_OR_ERROR_AND_RETURN(jobj, path, member, converter, dst, error, required) do {     \
    json_object *_jarray;                                                                                       \
    if (json_object_object_get_ex(jobj, member, &_jarray) && json_object_is_type(_jarray, json_type_array)) {   \
        size_t _num_options = json_object_array_length(_jarray);                                                \
        dst = 0;                                                                                                \
        for (size_t _i = 0; _i < _num_options; ++_i) {                                                          \
            json_object *_joption = json_object_array_get_idx(_jarray, _i);                                     \
            if (!json_object_is_type(_joption, json_type_string)) {                                             \
                buffer_sprintf(error, "invalid type for '%s.%s' at index %zu", path, member, _i);               \
                return false;                                                                                   \
            }                                                                                                   \
            const char *_option_str = json_object_get_string(_joption);                                         \
            typeof(dst) _bit = converter(_option_str);                                                          \
            if (_bit == 0) {                                                                                    \
                buffer_sprintf(error, "unknown option '%s' in '%s.%s' at index %zu", _option_str, path, member, _i); \
                return false;                                                                                   \
            }                                                                                                   \
            dst |= _bit;                                                                                        \
        }                                                                                                       \
    } else if(required) {                                                                                       \
        buffer_sprintf(error, "missing or invalid type for '%s.%s' array", path, member);                       \
        return false;                                                                                           \
    }                                                                                                           \
} while(0)

#define JSONC_PARSE_TXT2ENUM_OR_ERROR_AND_RETURN(jobj, path, member, converter, dst, error, required) do {      \
    json_object *_j;                                                                                            \
    if (json_object_object_get_ex(jobj, member, &_j) && json_object_is_type(_j, json_type_string))              \
        dst = converter(json_object_get_string(_j));                                                            \
    else if(required) {                                                                                         \
        buffer_sprintf(error, "missing or invalid type (expected text value) for '%s.%s' enum", path, member);  \
        return false;                                                                                           \
    }                                                                                                           \
} while(0)

#define JSONC_PARSE_INT64_OR_ERROR_AND_RETURN(jobj, path, member, dst, error, required) do {                    \
    json_object *_j;                                                                                            \
    if (json_object_object_get_ex(jobj, member, &_j)) {                                                         \
        if (_j != NULL && json_object_is_type(_j, json_type_int))                                               \
            dst = json_object_get_int64(_j);                                                                    \
        else if (_j != NULL && json_object_is_type(_j, json_type_double))                                       \
            dst = (typeof(dst))json_object_get_double(_j);                                                      \
        else if (_j == NULL)                                                                                    \
            dst = 0;                                                                                            \
        else {                                                                                                  \
            buffer_sprintf(error, "not supported type (expected int) for '%s.%s'", path, member);               \
            return false;                                                                                       \
        }                                                                                                       \
    } else if(required) {                                                                                       \
        buffer_sprintf(error, "missing or invalid type (expected int value or null) for '%s.%s'", path, member);\
        return false;                                                                                           \
    }                                                                                                           \
} while(0)

#define JSONC_PARSE_UINT64_OR_ERROR_AND_RETURN(jobj, path, member, dst, error, required) do {                   \
    json_object *_j;                                                                                            \
    if (json_object_object_get_ex(jobj, member, &_j)) {                                                         \
        if (_j != NULL && json_object_is_type(_j, json_type_int))                                               \
            dst = json_object_get_uint64(_j);                                                                   \
        else if (_j != NULL && json_object_is_type(_j, json_type_double))                                       \
            dst = (typeof(dst))json_object_get_double(_j);                                                      \
        else if (_j == NULL)                                                                                    \
            dst = 0;                                                                                            \
        else {                                                                                                  \
            buffer_sprintf(error, "not supported type (expected int) for '%s.%s'", path, member);               \
            return false;                                                                                       \
        }                                                                                                       \
    } else if(required) {                                                                                       \
        buffer_sprintf(error, "missing or invalid type (expected int value or null) for '%s.%s'", path, member);\
        return false;                                                                                           \
    }                                                                                                           \
} while(0)

#define JSONC_PARSE_DOUBLE_OR_ERROR_AND_RETURN(jobj, path, member, dst, error, required) do {                   \
    json_object *_j;                                                                                            \
    if (json_object_object_get_ex(jobj, member, &_j)) {                                                         \
        if (_j != NULL && json_object_is_type(_j, json_type_double))                                            \
            dst = json_object_get_double(_j);                                                                   \
        else if (_j != NULL && json_object_is_type(_j, json_type_int))                                          \
            dst = (typeof(dst))json_object_get_int(_j);                                                         \
        else if (_j == NULL)                                                                                    \
            dst = NAN;                                                                                          \
        else {                                                                                                  \
            buffer_sprintf(error, "not supported type (expected double) for '%s.%s'", path, member);            \
            return false;                                                                                       \
        }                                                                                                       \
    } else if(required) {                                                                                       \
        buffer_sprintf(error, "missing or invalid type (expected double value or null) for '%s.%s'", path, member); \
        return false;                                                                                           \
    }                                                                                                           \
} while(0)

#define JSONC_PARSE_SUBOBJECT(jobj, path, member, dst, callback, error, required) do { \
    json_object *_j;                                                                                            \
    if (json_object_object_get_ex(jobj, member, &_j)) {                                                         \
        char _new_path[strlen(path) + strlen(member) + 2];                                                      \
        snprintfz(_new_path, sizeof(_new_path), "%s%s%s", path, *path?".":"", member);                          \
        if (!callback(_j, _new_path, dst, error, required)) {                                                   \
            return false;                                                                                       \
        }                                                                                                       \
    } else if(required) {                                                                                       \
        buffer_sprintf(error, "missing '%s.%s' object", path, member);                                          \
        return false;                                                                                           \
    }                                                                                                           \
} while(0)

typedef bool (*json_parse_function_payload_t)(json_object *jobj, const char *path, void *data, BUFFER *error);
int rrd_call_function_error(BUFFER *wb, const char *msg, int code);
struct json_object *json_parse_function_payload_or_error(BUFFER *output, BUFFER *payload, int *code, json_parse_function_payload_t cb, void *cb_data);

#endif //NETDATA_JSON_C_PARSER_INLINE_H
