// SPDX-License-Identifier: GPL-3.0-or-later

#include "appconfig_internals.h"
#include "appconfig_api_durations.h"


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

    const char *s = appconfig_get_raw_value(
        root, section, name, default_str, reformat_duration_seconds, CONFIG_VALUE_TYPE_DURATION_IN_SECS);
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

    const char *s = appconfig_get_raw_value(
        root, section, name, default_str, reformat_duration_ms, CONFIG_VALUE_TYPE_DURATION_IN_MS);
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

    const char *s = appconfig_get_raw_value(
        root, section, name, default_str, reformat_duration_days, CONFIG_VALUE_TYPE_DURATION_IN_DAYS);
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

