// SPDX-License-Identifier: GPL-3.0-or-later

#include "appconfig_internals.h"
#include "appconfig_api_sizes.h"

static STRING *reformat_size_bytes(STRING *value) {
    uint64_t result = 0;
    if(!size_parse_bytes(string2str(value), &result))
        return value;

    char buf[128];
    if(size_snprintf_bytes(buf, sizeof(buf), result) > 0 && string_strcmp(value, buf) != 0) {
        string_freez(value);
        return string_strdupz(buf);
    }

    return value;
}

unsigned appconfig_get_size_bytes(struct config *root, const char *section, const char *name, unsigned default_value) {
    char default_str[128];
    size_snprintf_bytes(default_str, sizeof(default_str), (int)default_value);

    const char *s =
        appconfig_get_raw_value(root, section, name, default_str, reformat_size_bytes, CONFIG_VALUE_TYPE_SIZE_IN_BYTES);
    if(!s)
        return default_value;

    uint64_t result = 0;
    if(!size_parse_bytes(s, &result)) {
        appconfig_set(root, section, name, default_str);
        netdata_log_error("config option '[%s].%s = %s' is configured with an invalid size", section, name, s);
        return default_value;
    }

    return (unsigned)result;
}

unsigned appconfig_set_size_bytes(struct config *root, const char *section, const char *name, unsigned value) {
    char str[128];
    size_snprintf_bytes(str, sizeof(str), value);
    appconfig_set(root, section, name, str);
    return value;
}

static STRING *reformat_size_mb(STRING *value) {
    uint64_t result = 0;
    if(!size_parse_mb(string2str(value), &result))
        return value;

    char buf[128];
    if(size_snprintf_mb(buf, sizeof(buf), result) > 0 && string_strcmp(value, buf) != 0) {
        string_freez(value);
        return string_strdupz(buf);
    }

    return value;
}

unsigned appconfig_get_size_mb(struct config *root, const char *section, const char *name, unsigned default_value) {
    char default_str[128];
    size_snprintf_mb(default_str, sizeof(default_str), (int)default_value);

    const char *s =
        appconfig_get_raw_value(root, section, name, default_str, reformat_size_mb, CONFIG_VALUE_TYPE_SIZE_IN_MB);
    if(!s)
        return default_value;

    uint64_t result = 0;
    if(!size_parse_mb(s, &result)) {
        appconfig_set(root, section, name, default_str);
        netdata_log_error("config option '[%s].%s = %s' is configured with an invalid size", section, name, s);
        return default_value;
    }

    return (unsigned)result;
}

unsigned appconfig_set_size_mb(struct config *root, const char *section, const char *name, unsigned value) {
    char str[128];
    size_snprintf_mb(str, sizeof(str), value);
    appconfig_set(root, section, name, str);
    return value;
}
