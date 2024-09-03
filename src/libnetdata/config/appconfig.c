// SPDX-License-Identifier: GPL-3.0-or-later

#include "appconfig_internals.h"

int appconfig_exists(struct config *root, const char *section, const char *name) {
    struct config_section *sect = appconfig_section_find(root, section);
    if(!sect) return 0;

    struct config_option *opt = appconfig_option_find(sect, name);
    if(!opt) return 0;

    return 1;
}

const char *appconfig_get(struct config *root, const char *section, const char *name, const char *default_value) {
    return appconfig_get_value_and_reformat(root, section, name, default_value, NULL);
}

long long appconfig_get_number(struct config *root, const char *section, const char *name, long long value) {
    char buffer[100];
    sprintf(buffer, "%lld", value);

    const char *s = appconfig_get(root, section, name, buffer);
    if(!s) return value;

    return strtoll(s, NULL, 0);
}

NETDATA_DOUBLE appconfig_get_double(struct config *root, const char *section, const char *name, NETDATA_DOUBLE value) {
    char buffer[100];
    sprintf(buffer, "%0.5" NETDATA_DOUBLE_MODIFIER, value);

    const char *s = appconfig_get(root, section, name, buffer);
    if(!s) return value;

    return str2ndd(s, NULL);
}

bool appconfig_test_boolean_value(const char *s) {
    if(!strcasecmp(s, "yes") || !strcasecmp(s, "true") || !strcasecmp(s, "on")
       || !strcasecmp(s, "auto") || !strcasecmp(s, "on demand"))
        return true;

    return false;
}

int appconfig_get_boolean_by_section(struct config_section *co, const char *name, int value) {
    const char *s = appconfig_get_value_of_option_in_section(co, name, (!value) ? "no" : "yes", NULL);
    if(!s) return value;

    return appconfig_test_boolean_value(s);
}

int appconfig_get_boolean(struct config *root, const char *section, const char *name, int value) {
    const char *s;
    if(value) s = "yes";
    else s = "no";

    s = appconfig_get(root, section, name, s);
    if(!s) return value;

    return appconfig_test_boolean_value(s);
}

int appconfig_get_boolean_ondemand(struct config *root, const char *section, const char *name, int value) {
    const char *s;

    if(value == CONFIG_BOOLEAN_AUTO)
        s = "auto";

    else if(value == CONFIG_BOOLEAN_NO)
        s = "no";

    else
        s = "yes";

    s = appconfig_get(root, section, name, s);
    if(!s) return value;

    if(!strcmp(s, "yes") || !strcmp(s, "true") || !strcmp(s, "on"))
        return CONFIG_BOOLEAN_YES;
    else if(!strcmp(s, "no") || !strcmp(s, "false") || !strcmp(s, "off"))
        return CONFIG_BOOLEAN_NO;
    else if(!strcmp(s, "auto") || !strcmp(s, "on demand"))
        return CONFIG_BOOLEAN_AUTO;

    return value;
}

const char *appconfig_set_default(struct config *root, const char *section, const char *name, const char *value) {
    struct config_section *sect = appconfig_section_find(root, section);
    if(!sect) return appconfig_set(root, section, name, value);

    struct config_option *opt = appconfig_option_find(sect, name);
    if(!opt) return appconfig_set(root, section, name, value);

    opt->flags |= CONFIG_VALUE_USED;

    if(opt->flags & CONFIG_VALUE_LOADED)
        return string2str(opt->value);

    if(string_strcmp(opt->value, value) != 0) {
        opt->flags |= CONFIG_VALUE_CHANGED;

        string_freez(opt->value);
        opt->value = string_strdupz(value);
    }

    return string2str(opt->value);
}

const char *appconfig_set(struct config *root, const char *section, const char *name, const char *value) {
    struct config_section *sect = appconfig_section_find(root, section);
    if(!sect)
        sect = appconfig_section_create(root, section);

    struct config_option *opt = appconfig_option_find(sect, name);
    if(!opt)
        opt = appconfig_option_create(sect, name, value);

    opt->flags |= CONFIG_VALUE_USED;

    if(string_strcmp(opt->value, value) != 0) {
        opt->flags |= CONFIG_VALUE_CHANGED;

        string_freez(opt->value);
        opt->value = string_strdupz(value);
    }

    return value;
}

long long appconfig_set_number(struct config *root, const char *section, const char *name, long long value) {
    char buffer[100];
    sprintf(buffer, "%lld", value);

    appconfig_set(root, section, name, buffer);
    return value;
}

NETDATA_DOUBLE appconfig_set_float(struct config *root, const char *section, const char *name, NETDATA_DOUBLE value) {
    char buffer[100];
    sprintf(buffer, "%0.5" NETDATA_DOUBLE_MODIFIER, value);

    appconfig_set(root, section, name, buffer);
    return value;
}

int appconfig_set_boolean(struct config *root, const char *section, const char *name, int value) {
    const char *s;
    if(value) s = "yes";
    else s = "no";

    appconfig_set(root, section, name, s);

    return value;
}

static STRING *reformat_duration_seconds(STRING *value) {
    int result = 0;
    if(!duration_parse_seconds(string2str(value), &result))
        return value;

    char buf[128];
    if(duration_snprintf_time_t(buf, sizeof(buf), result) > 0 && string_strcmp(value, buf) != 0) {
        string_freez(value);
        return string_strdupz(buf);
    }

    return value;
}

time_t appconfig_get_duration_seconds(struct config *root, const char *section, const char *name, time_t default_value) {
    char default_str[128];
    duration_snprintf_time_t(default_str, sizeof(default_str), default_value);

    const char *s = appconfig_get_value_and_reformat(root, section, name, default_str, reformat_duration_seconds);
    if(!s)
        return default_value;

    int result = 0;
    if(!duration_parse_seconds(s, &result)) {
        appconfig_set(root, section, name, default_str);
        netdata_log_error("config option '[%s].%s = %s' is configured with an invalid duration", section, name, s);
        return default_value;
    }

    return ABS(result);
}

time_t appconfig_set_duration_seconds(struct config *root, const char *section, const char *name, time_t value) {
    char str[128];
    duration_snprintf_time_t(str, sizeof(str), value);

    appconfig_set(root, section, name, str);
    return value;
}

static STRING *reformat_duration_ms(STRING *value) {
    int64_t result = 0;
    if(!duration_parse_msec_t(string2str(value), &result))
        return value;

    char buf[128];
    if(duration_snprintf_msec_t(buf, sizeof(buf), result) > 0 && string_strcmp(value, buf) != 0) {
        string_freez(value);
        return string_strdupz(buf);
    }

    return value;
}

msec_t appconfig_get_duration_ms(struct config *root, const char *section, const char *name, msec_t default_value) {
    char default_str[128];
    duration_snprintf_msec_t(default_str, sizeof(default_str), default_value);

    const char *s = appconfig_get_value_and_reformat(root, section, name, default_str, reformat_duration_ms);
    if(!s)
        return default_value;

    smsec_t result = 0;
    if(!duration_parse_msec_t(s, &result)) {
        appconfig_set(root, section, name, default_str);
        netdata_log_error("config option '[%s].%s = %s' is configured with an invalid duration", section, name, s);
        return default_value;
    }

    return ABS(result);
}

msec_t appconfig_set_duration_ms(struct config *root, const char *section, const char *name, msec_t value) {
    char str[128];
    duration_snprintf_msec_t(str, sizeof(str), (smsec_t)value);

    appconfig_set(root, section, name, str);
    return value;
}

static STRING *reformat_duration_days(STRING *value) {
    int64_t result = 0;
    if(!duration_parse_days(string2str(value), &result))
        return value;

    char buf[128];
    if(duration_snprintf_days(buf, sizeof(buf), result) > 0 && string_strcmp(value, buf) != 0) {
        string_freez(value);
        return string_strdupz(buf);
    }

    return value;
}

unsigned appconfig_get_duration_days(struct config *root, const char *section, const char *name, unsigned default_value) {
    char default_str[128];
    duration_snprintf_days(default_str, sizeof(default_str), (int)default_value);

    const char *s = appconfig_get_value_and_reformat(root, section, name, default_str, reformat_duration_days);
    if(!s)
        return default_value;

    int64_t result = 0;
    if(!duration_parse_days(s, &result)) {
        appconfig_set(root, section, name, default_str);
        netdata_log_error("config option '[%s].%s = %s' is configured with an invalid duration", section, name, s);
        return default_value;
    }

    return (unsigned)ABS(result);
}

unsigned appconfig_set_duration_days(struct config *root, const char *section, const char *name, unsigned value) {
    char str[128];
    duration_snprintf_days(str, sizeof(str), value);
    appconfig_set(root, section, name, str);
    return value;
}

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

    const char *s = appconfig_get_value_and_reformat(root, section, name, default_str, reformat_size_bytes);
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

    const char *s = appconfig_get_value_and_reformat(root, section, name, default_str, reformat_size_mb);
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
