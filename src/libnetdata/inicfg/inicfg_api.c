// SPDX-License-Identifier: GPL-3.0-or-later

#include "inicfg_internals.h"

const char *inicfg_get(struct config *root, const char *section, const char *name, const char *default_value) {
    struct config_option *opt = inicfg_get_raw_value(root, section, name, default_value, CONFIG_VALUE_TYPE_TEXT, NULL);
    if(!opt)
        return default_value;

    return string2str(opt->value);
}

const char *inicfg_set(struct config *root, const char *section, const char *name, const char *value) {
    struct config_option *opt = inicfg_set_raw_value(root, section, name, value, CONFIG_VALUE_TYPE_TEXT);
    return string2str(opt->value);
}

bool inicfg_test_boolean_value(const char *s) {
    if(!strcasecmp(s, "yes") || !strcasecmp(s, "true") || !strcasecmp(s, "on")
        || !strcasecmp(s, "auto") || !strcasecmp(s, "on demand"))
        return true;

    return false;
}

int inicfg_get_boolean_by_section(struct config_section *sect, const char *name, int value) {
    struct config_option *opt = inicfg_get_raw_value_of_option_in_section(
        sect, name, (!value) ? "no" : "yes", CONFIG_VALUE_TYPE_BOOLEAN, NULL);
    if(!opt) return value;

    return inicfg_test_boolean_value(string2str(opt->value));
}

int inicfg_get_boolean(struct config *root, const char *section, const char *name, int value) {
    const char *s;
    if(value) s = "yes";
    else s = "no";

    struct config_option *opt = inicfg_get_raw_value(root, section, name, s, CONFIG_VALUE_TYPE_BOOLEAN, NULL);
    if(!opt) return value;
    s = string2str(opt->value);

    return inicfg_test_boolean_value(s);
}

int inicfg_get_boolean_ondemand(struct config *root, const char *section, const char *name, int value) {
    const char *s;

    if(value == CONFIG_BOOLEAN_AUTO)
        s = "auto";

    else if(value == CONFIG_BOOLEAN_NO)
        s = "no";

    else
        s = "yes";

    struct config_option *opt = inicfg_get_raw_value(root, section, name, s, CONFIG_VALUE_TYPE_BOOLEAN_ONDEMAND, NULL);
    if(!opt) return value;

    s = string2str(opt->value);
    if(!strcmp(s, "yes") || !strcmp(s, "true") || !strcmp(s, "on"))
        return CONFIG_BOOLEAN_YES;
    else if(!strcmp(s, "no") || !strcmp(s, "false") || !strcmp(s, "off"))
        return CONFIG_BOOLEAN_NO;
    else if(!strcmp(s, "auto") || !strcmp(s, "on demand"))
        return CONFIG_BOOLEAN_AUTO;

    return value;
}

int inicfg_set_boolean(struct config *root, const char *section, const char *name, int value) {
    const char *s;
    if(value) s = "yes";
    else s = "no";

    inicfg_set_raw_value(root, section, name, s, CONFIG_VALUE_TYPE_BOOLEAN);

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

time_t inicfg_get_duration_seconds(struct config *root, const char *section, const char *name, time_t default_value) {
    char default_str[128];
    duration_snprintf_time_t(default_str, sizeof(default_str), default_value);

    struct config_option *opt = inicfg_get_raw_value(
        root, section, name, default_str, CONFIG_VALUE_TYPE_DURATION_IN_SECS, reformat_duration_seconds);
    if(!opt)
        return default_value;

    const char *s = string2str(opt->value);

    int result = 0;
    if(!duration_parse_seconds(s, &result)) {
        inicfg_set_raw_value(root, section, name, default_str, CONFIG_VALUE_TYPE_DURATION_IN_SECS);
        netdata_log_error("config option '[%s].%s = %s' is configured with an invalid duration", section, name, s);
        return default_value;
    }

    return ABS(result);
}

time_t inicfg_set_duration_seconds(struct config *root, const char *section, const char *name, time_t value) {
    char str[128];
    duration_snprintf_time_t(str, sizeof(str), value);

    inicfg_set_raw_value(root, section, name, str, CONFIG_VALUE_TYPE_DURATION_IN_SECS);
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

msec_t inicfg_get_duration_ms(struct config *root, const char *section, const char *name, msec_t default_value) {
    char default_str[128];
    duration_snprintf_msec_t(default_str, sizeof(default_str), default_value);

    struct config_option *opt = inicfg_get_raw_value(
        root, section, name, default_str, CONFIG_VALUE_TYPE_DURATION_IN_MS, reformat_duration_ms);
    if(!opt)
        return default_value;

    const char *s = string2str(opt->value);

    smsec_t result = 0;
    if(!duration_parse_msec_t(s, &result)) {
        inicfg_set_raw_value(root, section, name, default_str, CONFIG_VALUE_TYPE_DURATION_IN_MS);
        netdata_log_error("config option '[%s].%s = %s' is configured with an invalid duration", section, name, s);
        return default_value;
    }

    return ABS(result);
}

msec_t inicfg_set_duration_ms(struct config *root, const char *section, const char *name, msec_t value) {
    char str[128];
    duration_snprintf_msec_t(str, sizeof(str), (smsec_t)value);

    inicfg_set_raw_value(root, section, name, str, CONFIG_VALUE_TYPE_DURATION_IN_MS);
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

unsigned inicfg_get_duration_days(struct config *root, const char *section, const char *name, unsigned default_value) {
    char default_str[128];
    duration_snprintf_days(default_str, sizeof(default_str), (int)default_value);

    struct config_option *opt = inicfg_get_raw_value(
        root, section, name, default_str, CONFIG_VALUE_TYPE_DURATION_IN_DAYS, reformat_duration_days);
    if(!opt)
        return default_value;

    const char *s = string2str(opt->value);

    int64_t result = 0;
    if(!duration_parse_days(s, &result)) {
        inicfg_set_raw_value(root, section, name, default_str, CONFIG_VALUE_TYPE_DURATION_IN_DAYS);
        netdata_log_error("config option '[%s].%s = %s' is configured with an invalid duration", section, name, s);
        return default_value;
    }

    return (unsigned)ABS(result);
}

unsigned inicfg_set_duration_days(struct config *root, const char *section, const char *name, unsigned value) {
    char str[128];
    duration_snprintf_days(str, sizeof(str), value);
    inicfg_set_raw_value(root, section, name, str, CONFIG_VALUE_TYPE_DURATION_IN_DAYS);
    return value;
}

long long inicfg_get_number(struct config *root, const char *section, const char *name, long long value) {
    char buffer[100];
    sprintf(buffer, "%lld", value);

    struct config_option *opt = inicfg_get_raw_value(root, section, name, buffer, CONFIG_VALUE_TYPE_INTEGER, NULL);
    if(!opt) return value;

    const char *s = string2str(opt->value);
    return strtoll(s, NULL, 0);
}

NETDATA_DOUBLE inicfg_get_double(struct config *root, const char *section, const char *name, NETDATA_DOUBLE value) {
    char buffer[100];
    sprintf(buffer, "%0.5" NETDATA_DOUBLE_MODIFIER, value);

    struct config_option *opt = inicfg_get_raw_value(root, section, name, buffer, CONFIG_VALUE_TYPE_DOUBLE, NULL);
    if(!opt) return value;

    const char *s = string2str(opt->value);
    return str2ndd(s, NULL);
}

long long inicfg_set_number(struct config *root, const char *section, const char *name, long long value) {
    char buffer[100];
    sprintf(buffer, "%lld", value);

    inicfg_set_raw_value(root, section, name, buffer, CONFIG_VALUE_TYPE_INTEGER);
    return value;
}

NETDATA_DOUBLE inicfg_set_double(struct config *root, const char *section, const char *name, NETDATA_DOUBLE value) {
    char buffer[100];
    sprintf(buffer, "%0.5" NETDATA_DOUBLE_MODIFIER, value);

    inicfg_set_raw_value(root, section, name, buffer, CONFIG_VALUE_TYPE_DOUBLE);
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

uint64_t inicfg_get_size_bytes(struct config *root, const char *section, const char *name, uint64_t default_value) {
    char default_str[128];
    size_snprintf_bytes(default_str, sizeof(default_str), default_value);

    struct config_option *opt =
        inicfg_get_raw_value(root, section, name, default_str, CONFIG_VALUE_TYPE_SIZE_IN_BYTES, reformat_size_bytes);
    if(!opt)
        return default_value;

    const char *s = string2str(opt->value);
    uint64_t result = 0;
    if(!size_parse_bytes(s, &result)) {
        inicfg_set_raw_value(root, section, name, default_str, CONFIG_VALUE_TYPE_SIZE_IN_BYTES);
        netdata_log_error("config option '[%s].%s = %s' is configured with an invalid size", section, name, s);
        return default_value;
    }

    return result;
}

uint64_t inicfg_set_size_bytes(struct config *root, const char *section, const char *name, uint64_t value) {
    char str[128];
    size_snprintf_bytes(str, sizeof(str), value);
    inicfg_set_raw_value(root, section, name, str, CONFIG_VALUE_TYPE_SIZE_IN_BYTES);
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

uint64_t inicfg_get_size_mb(struct config *root, const char *section, const char *name, uint64_t default_value) {
    char default_str[128];
    size_snprintf_mb(default_str, sizeof(default_str), default_value);

    struct config_option *opt =
        inicfg_get_raw_value(root, section, name, default_str, CONFIG_VALUE_TYPE_SIZE_IN_MB, reformat_size_mb);
    if(!opt)
        return default_value;

    const char *s = string2str(opt->value);
    uint64_t result = 0;
    if(!size_parse_mb(s, &result)) {
        inicfg_set_raw_value(root, section, name, default_str, CONFIG_VALUE_TYPE_SIZE_IN_MB);
        netdata_log_error("config option '[%s].%s = %s' is configured with an invalid size", section, name, s);
        return default_value;
    }

    return (unsigned)result;
}

uint64_t inicfg_set_size_mb(struct config *root, const char *section, const char *name, uint64_t value) {
    char str[128];
    size_snprintf_mb(str, sizeof(str), value);
    inicfg_set_raw_value(root, section, name, str, CONFIG_VALUE_TYPE_SIZE_IN_MB);
    return value;
}
