#ifndef JSONC_HELPERS_H
#define JSONC_HELPERS_H

#include "config.h"

#ifdef ENABLE_JSONC

#define JSON_ADD_STRING(name, str, obj)                                                                                \
    {                                                                                                                  \
        tmp = json_object_new_string(str);                                                                             \
        json_object_object_add(obj, name, tmp);                                                                        \
    }

#define JSON_ADD_INT(name, val, obj)                                                                                   \
    {                                                                                                                  \
        tmp = json_object_new_int(val);                                                                                \
        json_object_object_add(obj, name, tmp);                                                                        \
    }

#define JSON_ADD_INT64(name, val, obj)                                                                                 \
    {                                                                                                                  \
        tmp = json_object_new_int64(val);                                                                              \
        json_object_object_add(obj, name, tmp);                                                                        \
    }

#define JSON_ADD_BOOL(name, val, obj)                                                                                  \
    {                                                                                                                  \
        tmp = json_object_new_boolean(val);                                                                            \
        json_object_object_add(obj, name, tmp);                                                                        \
    }

#endif

#endif /* JSONC_HELPERS_H */
