// SPDX-License-Identifier: GPL-3.0-or-later

#include "inicfg_internals.h"

const char *inicfg_get(struct config *root, const char *section, const char *name, const char *default_value) {
    struct config_option *opt = inicfg_get_raw_value(root, section, name, default_value, CONFIG_VALUE_TYPE_TEXT, NULL);
    if(!opt)
        // the only way for opt to be NULL, is default_value to be NULL too
        return NULL;

    return string2str(opt->value);
}

const char *inicfg_set(struct config *root, const char *section, const char *name, const char *value) {
    struct config_option *opt = inicfg_set_raw_value(root, section, name, value, CONFIG_VALUE_TYPE_TEXT);
    return string2str(opt->value);
}

static STRING *reformat_path(STRING *value) {
#if defined(OS_WINDOWS)
    CLEAN_CONST_CHAR_P *converted = os_translate_windows_to_msys_path(string2str(value));
    if(string_strcmp(value, converted) != 0) {
        string_freez(value);
        return string_strdupz(converted);
    }
    // value unchanged: fall through and return as-is
#else
    // no-op on non-Windows: paths are always POSIX-style
    (void)value;
#endif

    return value;
}

#if defined(OS_WINDOWS)
static const char *inicfg_log_output_etw = "etw";
static const char *inicfg_log_output_wel = "wel";

static bool log_setting_output_is_special(const char *output) {
    return !output || !*output ||
           strcmp(output, "none") == 0 ||
           strcmp(output, "off") == 0 ||
           strcmp(output, "journal") == 0 ||
           strcmp(output, "syslog") == 0 ||
           strcmp(output, "system") == 0 ||
           strcmp(output, "stderr") == 0 ||
           strcmp(output, "stdout") == 0 ||
           strcmp(output, "/dev/null") == 0
#if defined(HAVE_ETW)
           || strcmp(output, inicfg_log_output_etw) == 0
#endif
#if defined(HAVE_WEL)
           || strcmp(output, inicfg_log_output_wel) == 0
#endif
        ;
}

static char *transform_log_path_setting(const char *setting, bool for_display) {
    if(!setting)
        return strdupz("");

    CLEAN_CHAR_P *copy = strdupz(setting);
    char *output = strrchr(copy, '@');
    size_t prefix_len = 0;

    if(output) {
        prefix_len = (size_t)(output - copy);
        *output = '\0';
        output++;
    }
    else
        output = copy;

    if(log_setting_output_is_special(output))
        return strdupz(setting);

    CLEAN_CONST_CHAR_P *translated = for_display
        ? os_translate_msys_to_windows_path(output)
        : os_translate_windows_to_msys_path(output);
    const char *safe_translated;
    if(translated)
        safe_translated = translated;
    else if(output)
        safe_translated = output;
    else
        safe_translated = "";
    size_t safe_translated_len = strlen(safe_translated);

    if(output == copy)
        return strdupz(safe_translated);

    BUFFER *wb = buffer_create(prefix_len + safe_translated_len + 2, NULL);
    buffer_sprintf(wb, "%s@%s", copy, safe_translated);
    char *result = strdupz(buffer_tostring(wb));
    buffer_free(wb);

    return result;
}

static bool windows_native_path_p(const char *value) {
    if(!value || !*value)
        return false;

    return (isalpha((unsigned char)value[0]) && value[1] == ':') ||
           ((value[0] == '\\' && value[1] == '\\') || (value[0] == '/' && value[1] == '/'));
}

static bool windows_path_list_needs_reformat_p(const char *value) {
    if(!value || !*value)
        return false;

    return strchr(value, ';') || strchr(value, '\\') || windows_native_path_p(value);
}

static STRING *reformat_path_list(STRING *value) {
    const char *src = string2str(value);
    size_t src_len = string_strlen(value);
    if(!windows_path_list_needs_reformat_p(src))
        return value;

    BUFFER *wb = buffer_create(src_len + 1, NULL);
    bool first = true;
    const char *segment_start = src;

    while(true) {
        const char *separator = strchr(segment_start, ';');
        size_t segment_len = separator
            ? (size_t)(separator - segment_start)
            : src_len - (size_t)(segment_start - src);

        CLEAN_CHAR_P *segment = strndupz(segment_start, segment_len);
        char *trimmed = trim(segment);
        CLEAN_CONST_CHAR_P *converted = os_translate_windows_to_msys_path(trimmed);

        if(!first)
            buffer_strcat(wb, ":");
        buffer_strcat(wb, converted);
        first = false;

        if(!separator)
            break;

        segment_start = separator + 1;
    }

    if(string_strcmp(value, buffer_tostring(wb)) != 0) {
        string_freez(value);
        value = string_strdupz(buffer_tostring(wb));
    }

    buffer_free(wb);
    return value;
}

static STRING *reformat_quoted_path_list(STRING *value) {
    CLEAN_CHAR_P *copy = strdupz(string2str(value));
    // 256 slots is far more than any realistic plugins list; entries beyond this are silently ignored.
    char *words[256] = { 0 };
    size_t num_words = quoted_strings_splitter_config(copy, words, _countof(words));
    if(!num_words)
        return value;

    BUFFER *wb = buffer_create(string_strlen(value) + 1, NULL);
    for(size_t i = 0; i < num_words; i++) {
        CLEAN_CONST_CHAR_P *converted = os_translate_windows_to_msys_path(words[i]);
        if(i)
            buffer_strcat(wb, " ");
        buffer_sprintf(wb, "\"%s\"", converted);
    }

    if(string_strcmp(value, buffer_tostring(wb)) != 0) {
        string_freez(value);
        value = string_strdupz(buffer_tostring(wb));
    }

    buffer_free(wb);
    return value;
}

static STRING *reformat_log_path_setting(STRING *value) {
    CLEAN_CHAR_P *converted = transform_log_path_setting(string2str(value), false);
    if(string_strcmp(value, converted) != 0) {
        string_freez(value);
        return string_strdupz(converted);
    }

    return value;
}
#else
static STRING *reformat_path_list(STRING *value) {
    return value;
}

static STRING *reformat_quoted_path_list(STRING *value) {
    return value;
}

static STRING *reformat_log_path_setting(STRING *value) {
    return value;
}
#endif

const char *inicfg_get_filename(struct config *root, const char *section, const char *name, const char *default_value) {
    struct config_option *opt = inicfg_get_raw_value(root, section, name, default_value, CONFIG_VALUE_TYPE_FILENAME, reformat_path);
    if(!opt)
        return NULL;

    return string2str(opt->value);
}

const char *inicfg_get_path(struct config *root, const char *section, const char *name, const char *default_value) {
    struct config_option *opt = inicfg_get_raw_value(root, section, name, default_value, CONFIG_VALUE_TYPE_PATH, reformat_path);
    if(!opt)
        return NULL;

    return string2str(opt->value);
}

const char *inicfg_get_path_list(struct config *root, const char *section, const char *name, const char *default_value) {
    struct config_option *opt = inicfg_get_raw_value(root, section, name, default_value, CONFIG_VALUE_TYPE_TEXT, reformat_path_list);
    if(!opt)
        return NULL;

    return string2str(opt->value);
}

const char *inicfg_get_quoted_path_list(struct config *root, const char *section, const char *name, const char *default_value) {
    struct config_option *opt = inicfg_get_raw_value(root, section, name, default_value, CONFIG_VALUE_TYPE_TEXT, reformat_quoted_path_list);
    if(!opt)
        return NULL;

    return string2str(opt->value);
}

const char *inicfg_get_log_path_setting(struct config *root, const char *section, const char *name, const char *default_value) {
    struct config_option *opt = inicfg_get_raw_value(root, section, name, default_value, CONFIG_VALUE_TYPE_TEXT, reformat_log_path_setting);
    if(!opt)
        return NULL;

    return string2str(opt->value);
}

const char *inicfg_log_path_setting_for_display(const char *value, char *dst, size_t dst_size) {
#if defined(OS_WINDOWS)
    if(!dst || dst_size == 0)
        return value ? value : "";

    CLEAN_CHAR_P *converted = transform_log_path_setting(value, true);
    snprintfz(dst, dst_size, "%s", converted);
    return dst;
#else
    (void)dst;
    (void)dst_size;
    return value ? value : "";
#endif
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

static STRING *reformat_duration_days_to_seconds(STRING *value) {
    int64_t result = 0;
    if(!duration_parse(string2str(value), &result, "d", "s"))
        return value;

    char buf[128];
    if(duration_snprintf(buf, sizeof(buf), result, "s", false) > 0 && string_strcmp(value, buf) != 0) {
        string_freez(value);
        return string_strdupz(buf);
    }

    return value;
}

time_t inicfg_get_duration_days_to_seconds(struct config *root, const char *section, const char *name, unsigned default_value_seconds) {
    char default_str[128];
    duration_snprintf(default_str, sizeof(default_str), (int)default_value_seconds, "s", false);

    struct config_option *opt = inicfg_get_raw_value(
        root, section, name, default_str,
        CONFIG_VALUE_TYPE_DURATION_IN_DAYS_TO_SECONDS,
        reformat_duration_days_to_seconds);

    if(!opt)
        return default_value_seconds;

    const char *s = string2str(opt->value);

    int64_t result = 0;
    if(!duration_parse(s, &result, "d", "s")) {
        inicfg_set_raw_value(root, section, name, default_str, CONFIG_VALUE_TYPE_DURATION_IN_DAYS_TO_SECONDS);
        netdata_log_error("config option '[%s].%s = %s' is configured with an invalid duration", section, name, s);
        return default_value_seconds;
    }

    return (unsigned)ABS(result);
}

long long inicfg_get_number(struct config *root, const char *section, const char *name, long long value) {
    char buffer[100];
    sprintf(buffer, "%lld", value);

    struct config_option *opt = inicfg_get_raw_value(root, section, name, buffer, CONFIG_VALUE_TYPE_INTEGER, NULL);
    if(!opt) return value;

    const char *s = string2str(opt->value);
    return strtoll(s, NULL, 0);
}

long long inicfg_get_number_range(struct config *root, const char *section, const char *name, long long value, long long min, long long max) {
    char buffer[100];
    sprintf(buffer, "%lld", value);

    struct config_option *opt = inicfg_get_raw_value(root, section, name, buffer, CONFIG_VALUE_TYPE_INTEGER, NULL);
    if(!opt) return value;

    const char *s = string2str(opt->value);
    long long rc = strtoll(s, NULL, 0);
    long long rc2 = FIT_IN_RANGE(rc, min, max);

    if(rc != rc2) {
        nd_log(NDLS_DAEMON, NDLP_ERR, "CONFIG: out of range [%s].%s = %lld. Acceptable values: %lld to %lld inclusive. Setting it to %lld",
               section, name, rc, min, max, rc2);

        rc = rc2;
        inicfg_set_number(root, section, name, rc);
    }

    return rc;
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
