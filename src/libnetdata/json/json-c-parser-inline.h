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

#define JSONC_PARSE_TXT2CHAR_OR_ERROR_AND_RETURN(jobj, path, member, dst, error, required) do {                 \
    json_object *_j;                                                                                            \
    if (json_object_object_get_ex(jobj, member, &_j) && json_object_is_type(_j, json_type_string)) {            \
        strncpyz(dst, json_object_get_string(_j), sizeof(dst) - 1);                                             \
    }                                                                                                           \
    else {                                                                                                      \
        dst[0] = '\0';                                                                                          \
        if (required) {                                                                                         \
            buffer_sprintf(error, "missing or invalid type for '%s.%s' string", path, member);                  \
            return false;                                                                                       \
        }                                                                                                       \
    }                                                                                                           \
} while(0)

#define JSONC_PARSE_TXT2RFC3339_USEC_OR_ERROR_AND_RETURN(jobj, path, member, dst, error, required) do {         \
    char _datetime[RFC3339_MAX_LENGTH]; _datetime[0] = '\0';                                                    \
    json_object *_j;                                                                                            \
    if (json_object_object_get_ex(jobj, member, &_j) && json_object_is_type(_j, json_type_string)) {            \
        strncpyz(_datetime, json_object_get_string(_j), sizeof(_datetime) - 1);                                 \
        dst = rfc3339_parse_ut(_datetime, NULL);                                                                \
    }                                                                                                           \
    else {                                                                                                      \
        dst = 0;                                                                                                \
        if (required) {                                                                                         \
            buffer_sprintf(error, "missing or invalid type for '%s.%s' string", path, member);                  \
            return false;                                                                                       \
        }                                                                                                       \
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
                /* return false; */                                                                             \
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

#define JSONC_PARSE_SUBOBJECT_CB(jobj, path, member, dst, callback, error, required) do { \
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

#define JSONC_TEMP_VAR(type, line) JSONC_TEMP_VAR_IMPL(type, line)
#define JSONC_TEMP_VAR_IMPL(type, line) _jsonc_temp_##type##line

#define JSONC_PATH_CONCAT(path, sizeof_path, prefix, member, error) do {                                        \
    size_t len = strlen(prefix);                                                                                \
    if(len >= sizeof_path - 1) {                                                                                \
        buffer_sprintf(error, "path too long while adding '%s'", member);                                       \
        return false;                                                                                           \
    }                                                                                                           \
    if(len) {                                                                                                   \
        if(len >= sizeof_path - 2) {                                                                            \
            buffer_sprintf(error, "path too long while adding '.' before '%s'", member);                        \
            return false;                                                                                       \
        }                                                                                                       \
        strncpyz(path + len, ".", sizeof_path - len);                                                           \
        len++;                                                                                                  \
    }                                                                                                           \
    strncpyz(path + len, member, sizeof_path - len);                                                            \
} while(0)

#define JSONC_PATH_CONCAT_INDEX(path, sizeof_path, index, error) do {                                           \
    char _idx_str[32];                                                                                          \
    snprintfz(_idx_str, sizeof(_idx_str), "[%zu]", index);                                                      \
    size_t _path_len = strlen(path);                                                                            \
    if (_path_len + strlen(_idx_str) >= sizeof_path) {                                                          \
        buffer_sprintf(error, "path too long while adding array index");                                        \
        return false;                                                                                           \
    }                                                                                                           \
    strncpyz(path + _path_len, _idx_str, sizeof_path - _path_len);                                              \
} while(0)

#define JSONC_PARSE_SUBOBJECT(jobj, path, member, error, required, block) do { \
    BUILD_BUG_ON(sizeof(path) < 128); /* ensure path is an array of at least 128 bytes */                       \
    json_object *JSONC_TEMP_VAR(_j, __LINE__);                                                                  \
    if (!json_object_object_get_ex(jobj, member, &JSONC_TEMP_VAR(_j, __LINE__))) {                              \
        if(required) {                                                                                          \
            buffer_sprintf(error, "missing '%s.%s' object", *path ? path : "", member);                         \
            return false;                                                                                       \
        }                                                                                                       \
    }                                                                                                           \
    else {                                                                                                      \
        if (!json_object_is_type(JSONC_TEMP_VAR(_j, __LINE__), json_type_object)) {                             \
            if(required) {                                                                                      \
                buffer_sprintf(error, "not an object '%s.%s'", *path ? path : "", member);                      \
                return false;                                                                                   \
            }                                                                                                   \
        }                                                                                                       \
        else {                                                                                                  \
            json_object *JSONC_TEMP_VAR(saved_jobj, __LINE__) = jobj;                                           \
            jobj = JSONC_TEMP_VAR(_j, __LINE__);                                                                \
            char JSONC_TEMP_VAR(saved_path, __LINE__)[strlen(path) + 1];                                        \
            strncpyz(JSONC_TEMP_VAR(saved_path, __LINE__), path, sizeof(JSONC_TEMP_VAR(saved_path, __LINE__))); \
            JSONC_PATH_CONCAT(path, sizeof(path), path, member, error);                                         \
            /* Run the user's code block */                                                                     \
            block                                                                                               \
            /* Restore the previous scope's values */                                                           \
            jobj = JSONC_TEMP_VAR(saved_jobj, __LINE__);                                                        \
            strncpyz(path, JSONC_TEMP_VAR(saved_path, __LINE__), sizeof(path));                                 \
        }                                                                                                       \
    }                                                                                                           \
} while(0)

#define JSONC_PARSE_ARRAY(jobj, path, member, error, required, block) do {                                      \
    BUILD_BUG_ON(sizeof(path) < 128); /* ensure path is an array of at least 128 bytes */                       \
    json_object *JSONC_TEMP_VAR(_jarray, __LINE__);                                                             \
    if (!json_object_object_get_ex(jobj, member, &JSONC_TEMP_VAR(_jarray, __LINE__))) {                         \
        if (required) {                                                                                         \
           buffer_sprintf(error, "missing '%s.%s' array", *path ? path : "", member);                           \
           return false;                                                                                        \
        }                                                                                                       \
    }                                                                                                           \
    else {                                                                                                      \
        if (!json_object_is_type(JSONC_TEMP_VAR(_jarray, __LINE__), json_type_array)) {                         \
            if (required) {                                                                                     \
                buffer_sprintf(error, "not an array '%s.%s'", *path ? path : "", member);                       \
                return false;                                                                                   \
            }                                                                                                   \
        }                                                                                                       \
        else {                                                                                                  \
            json_object *JSONC_TEMP_VAR(saved_jobj, __LINE__) = jobj;                                           \
            jobj = JSONC_TEMP_VAR(_jarray, __LINE__);                                                           \
            char JSONC_TEMP_VAR(saved_path, __LINE__)[strlen(path) + 1];                                        \
            strncpyz(JSONC_TEMP_VAR(saved_path, __LINE__), path, sizeof(JSONC_TEMP_VAR(saved_path, __LINE__))); \
            JSONC_PATH_CONCAT(path, sizeof(path), path, member, error);                                         \
            /* Run the user's code block */                                                                     \
            block                                                                                               \
            /* Restore the previous scope's values */                                                           \
            jobj = JSONC_TEMP_VAR(saved_jobj, __LINE__);                                                        \
            strncpyz(path, JSONC_TEMP_VAR(saved_path, __LINE__), sizeof(path));                                 \
        }                                                                                                       \
    }                                                                                                           \
} while(0)

#define JSONC_PARSE_ARRAY_ITEM_OBJECT(jobj, path, index, required, block) do {                                  \
    size_t JSONC_TEMP_VAR(_array_len, __LINE__) = json_object_array_length(jobj);                               \
    for (index = 0; index < JSONC_TEMP_VAR(_array_len, __LINE__); index++) {                                    \
        json_object *JSONC_TEMP_VAR(_jitem, __LINE__) = json_object_array_get_idx(jobj, index);                 \
        if (!json_object_is_type(JSONC_TEMP_VAR(_jitem, __LINE__), json_type_object)) {                         \
            if(required) {                                                                                      \
                buffer_sprintf(error, "not an object '%s[%zu]'", *path ? path : "", index);                     \
                return false;                                                                                   \
            }                                                                                                   \
        }                                                                                                       \
        else {                                                                                                  \
            json_object *JSONC_TEMP_VAR(saved_jobj, __LINE__) = jobj;                                           \
            jobj = JSONC_TEMP_VAR(_jitem, __LINE__);                                                            \
            char JSONC_TEMP_VAR(saved_path, __LINE__)[strlen(path) + 1];                                        \
            strncpyz(JSONC_TEMP_VAR(saved_path, __LINE__), path, sizeof(JSONC_TEMP_VAR(saved_path, __LINE__))); \
            JSONC_PATH_CONCAT_INDEX(path, sizeof(path), index, error);                                          \
            /* Run the user's code block */                                                                     \
            block                                                                                               \
            /* Restore the previous scope's values */                                                           \
            jobj = JSONC_TEMP_VAR(saved_jobj, __LINE__);                                                        \
            strncpyz(path, JSONC_TEMP_VAR(saved_path, __LINE__), sizeof(path));                                 \
        }                                                                                                       \
    }                                                                                                           \
} while(0)

typedef bool (*json_parse_function_payload_t)(json_object *jobj, void *data, BUFFER *error);
int rrd_call_function_error(BUFFER *wb, const char *msg, int code);
struct json_object *json_parse_function_payload_or_error(BUFFER *output, BUFFER *payload, int *code, json_parse_function_payload_t cb, void *cb_data);

// return HTTP response code
int json_parse_payload_or_error(BUFFER *payload, BUFFER *error, json_parse_function_payload_t cb, void *cb_data);

#endif //NETDATA_JSON_C_PARSER_INLINE_H
