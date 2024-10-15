// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_TEMPLATE_ENUM_H
#define NETDATA_TEMPLATE_ENUM_H

#define ENUM_STR_MAP_DEFINE(type)                                                                                   \
    static struct  {                                                                                                \
        type id;                                                                                                    \
        const char *name;                                                                                           \
    } type ## _names[]

#define ENUM_STR_DEFINE_FUNCTIONS_EXTERN(type)                                                                      \
    type type ## _2id(const char *str);                                                                             \
    const char *type##_2str(type id);

#define ENUM_STR_DEFINE_FUNCTIONS(type, def, def_str)                                                               \
    type type##_2id(const char *str)                                                                                \
    {                                                                                                               \
        if (!str || !*str)                                                                                          \
            return def;                                                                                             \
                                                                                                                    \
        for (size_t i = 0; type ## _names[i].name; i++) {                                                           \
            if (strcmp(type ## _names[i].name, str) == 0)                                                           \
                return type ## _names[i].id;                                                                        \
        }                                                                                                           \
                                                                                                                    \
        return def;                                                                                                 \
    }                                                                                                               \
                                                                                                                    \
    const char *type##_2str(type id)                                                                                \
    {                                                                                                               \
        for (size_t i = 0; type ## _names[i].name; i++) {                                                           \
            if (id == type ## _names[i].id)                                                                         \
                return type ## _names[i].name;                                                                      \
        }                                                                                                           \
                                                                                                                    \
        return def_str;                                                                                             \
    }

// --------------------------------------------------------------------------------------------------------------------

#define BITMAP_STR_DEFINE_FUNCTIONS_EXTERN(type)                                                                   \
    type type ## _2id_one(const char *str);                                                                        \
    const char *type##_2str_one(type id);                                                                          \
    const char *type##_2json(BUFFER *wb, const char *key, type id);

#define BITMAP_STR_DEFINE_FUNCTIONS(type, def, def_str)                                                             \
    type type##_2id_one(const char *str)                                                                            \
    {                                                                                                               \
        if (!str || !*str)                                                                                          \
            return def;                                                                                             \
                                                                                                                    \
        for (size_t i = 0; type ## _names[i].name; i++) {                                                           \
            if (strcmp(type ## _names[i].name, str) == 0)                                                           \
                return type ## _names[i].id;                                                                        \
        }                                                                                                           \
                                                                                                                    \
        return def;                                                                                                 \
    }                                                                                                               \
                                                                                                                    \
    const char *type##_2str_one(type id)                                                                            \
    {                                                                                                               \
        for (size_t i = 0; type ## _names[i].name; i++) {                                                           \
            if (id == type ## _names[i].id)                                                                         \
                return type ## _names[i].name;                                                                      \
        }                                                                                                           \
                                                                                                                    \
        return def_str;                                                                                             \
    }                                                                                                               \
                                                                                                                    \
    const char *type##_2json(BUFFER *wb, const char *key, type id)                                                  \
    {                                                                                                               \
        buffer_json_member_add_array(wb, key);                                                                      \
        for (size_t i = 0; type ## _names[i].name; i++) {                                                           \
            if ((id & type ## _names[i].id) == type ## _names[i].id)                                                \
                buffer_json_add_array_item_string(wb, type ## _names[i].name);                                      \
        }                                                                                                           \
        buffer_json_array_close(wb);                                                                                \
                                                                                                                    \
        return def_str;                                                                                             \
    }

#endif //NETDATA_TEMPLATE_ENUM_H
