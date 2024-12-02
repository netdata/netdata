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

uint64_t appconfig_get_size_bytes(struct config *root, const char *section, const char *name, uint64_t default_value) {
    char default_str[128];
    size_snprintf_bytes(default_str, sizeof(default_str), default_value);

    struct config_option *opt =
        appconfig_get_raw_value(root, section, name, default_str, CONFIG_VALUE_TYPE_SIZE_IN_BYTES, reformat_size_bytes);
    if(!opt)
        return default_value;

    const char *s = string2str(opt->value);
    uint64_t result = 0;
    if(!size_parse_bytes(s, &result)) {
        appconfig_set_raw_value(root, section, name, default_str, CONFIG_VALUE_TYPE_SIZE_IN_BYTES);
        netdata_log_error("config option '[%s].%s = %s' is configured with an invalid size", section, name, s);
        return default_value;
    }

    return result;
}

uint64_t appconfig_set_size_bytes(struct config *root, const char *section, const char *name, uint64_t value) {
    char str[128];
    size_snprintf_bytes(str, sizeof(str), value);
    appconfig_set_raw_value(root, section, name, str, CONFIG_VALUE_TYPE_SIZE_IN_BYTES);
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

uint64_t appconfig_get_size_mb(struct config *root, const char *section, const char *name, uint64_t default_value) {
    char default_str[128];
    size_snprintf_mb(default_str, sizeof(default_str), default_value);

    struct config_option *opt =
        appconfig_get_raw_value(root, section, name, default_str, CONFIG_VALUE_TYPE_SIZE_IN_MB, reformat_size_mb);
    if(!opt)
        return default_value;

    const char *s = string2str(opt->value);
    uint64_t result = 0;
    if(!size_parse_mb(s, &result)) {
        appconfig_set_raw_value(root, section, name, default_str, CONFIG_VALUE_TYPE_SIZE_IN_MB);
        netdata_log_error("config option '[%s].%s = %s' is configured with an invalid size", section, name, s);
        return default_value;
    }

    return (unsigned)result;
}

uint64_t appconfig_set_size_mb(struct config *root, const char *section, const char *name, uint64_t value) {
    char str[128];
    size_snprintf_mb(str, sizeof(str), value);
    appconfig_set_raw_value(root, section, name, str, CONFIG_VALUE_TYPE_SIZE_IN_MB);
    return value;
}
