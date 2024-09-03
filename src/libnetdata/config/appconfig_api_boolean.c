// SPDX-License-Identifier: GPL-3.0-or-later

#include "appconfig_internals.h"
#include "appconfig_api_boolean.h"

bool appconfig_test_boolean_value(const char *s) {
    if(!strcasecmp(s, "yes") || !strcasecmp(s, "true") || !strcasecmp(s, "on")
        || !strcasecmp(s, "auto") || !strcasecmp(s, "on demand"))
        return true;

    return false;
}

int appconfig_get_boolean_by_section(struct config_section *co, const char *name, int value) {
    const char *s = appconfig_get_raw_value_of_option_in_section(
        co, name, (!value) ? "no" : "yes", NULL, CONFIG_VALUE_TYPE_BOOLEAN);
    if(!s) return value;

    return appconfig_test_boolean_value(s);
}

int appconfig_get_boolean(struct config *root, const char *section, const char *name, int value) {
    const char *s;
    if(value) s = "yes";
    else s = "no";

    s = appconfig_get_raw_value(root, section, name, s, NULL, CONFIG_VALUE_TYPE_BOOLEAN);
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

    s = appconfig_get_raw_value(root, section, name, s, NULL, CONFIG_VALUE_TYPE_BOOLEAN_ONDEMAND);
    if(!s) return value;

    if(!strcmp(s, "yes") || !strcmp(s, "true") || !strcmp(s, "on"))
        return CONFIG_BOOLEAN_YES;
    else if(!strcmp(s, "no") || !strcmp(s, "false") || !strcmp(s, "off"))
        return CONFIG_BOOLEAN_NO;
    else if(!strcmp(s, "auto") || !strcmp(s, "on demand"))
        return CONFIG_BOOLEAN_AUTO;

    return value;
}

int appconfig_set_boolean(struct config *root, const char *section, const char *name, int value) {
    const char *s;
    if(value) s = "yes";
    else s = "no";

    appconfig_set(root, section, name, s);

    return value;
}
