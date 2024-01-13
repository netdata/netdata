#include <iostream>
#include <fstream>

#include "silencers.h"

#include "libnetdata/libnetdata.h"
#include "libnetdata/nlohmann/json.hpp"

#define HEALTH_ALARM_KEY "alarm"
#define HEALTH_CONTEXT_KEY "context"
#define HEALTH_CHART_KEY "chart"
#define HEALTH_HOST_KEY "hosts"

static SILENCE_TYPE silence_type_from_string(const std::string &stype) {
    if (strcmp(stype.data(), "DISABLE") == 0) {
        return STYPE_DISABLE_ALARMS;
    } else if (strcmp(stype.data(), "SILENCE") == 0) {
        return STYPE_SILENCE_NOTIFICATIONS;
    }

    return STYPE_NONE;
}

static const char *silence_type_to_cstr(SILENCE_TYPE stype) {
    switch (stype) {
        case STYPE_NONE:
            return "None";
        case STYPE_DISABLE_ALARMS:
            return "DISABLE";
        case STYPE_SILENCE_NOTIFICATIONS:
            return "SILENCE";
    }
}

bool load_health_silencers(const char *path) {
    nlohmann::json J;

    std::ifstream InputStream(path);
    if (!InputStream.is_open()) {
        netdata_log_error("Failed to open health silencers file %s", path);
        return false;
    }

    try {
        InputStream >> J;
    }
    catch (nlohmann::json::parse_error& Err) {
        netdata_log_error("Failed to parse health silencers file %s", path);
        InputStream.close();
        return false;
    }

    bool InputFailure = InputStream.fail();
    InputStream.close();

    if (InputFailure) {
        netdata_log_error("Failed to read health silencers file %s", path);
        return false;
    }

    bool All = false;
    if (J.contains("all")) {
        if (!J["all"].is_boolean()) {
            netdata_log_error("'all' key in health silencers file %s, should be a boolean", path);
            return false;
        }

        All = J["all"].get<bool>();
    }
    silencers->all_alarms = All ? 1 : 0;

    std::string Type = "NONE";
    if (J.contains("type")) {
        if (!J["type"].is_string())  {
            netdata_log_error("'type' key in health silencers file %s, should be a string", path);
            return false;
        }

        Type = J["type"].get<std::string>();
    }
    silencers->stype = silence_type_from_string(Type);

    if (J.contains("silencers")) {
        if (!J["silencers"].is_array()) {
            netdata_log_error("'silencers' key in health silencers file %s, should be an array of objects", path);
            return false;
        } 

        for (const auto& S: J["silencers"]) {
            SILENCER *silencer = static_cast<SILENCER *>(callocz(1, sizeof(SILENCER)));

            if (S.contains(HEALTH_ALARM_KEY)) {
                if (!S[HEALTH_ALARM_KEY].is_string()) {
                    netdata_log_error("'%s' key in health silencers file %s, should be a string",
                                      HEALTH_ALARM_KEY, path);
                    return false;
                }

                std::string alarms = S[HEALTH_ALARM_KEY];
                silencer->alarms = strdupz(alarms.c_str());
                silencer->alarms_pattern = simple_pattern_create(silencer->alarms, NULL, SIMPLE_PATTERN_EXACT, true);
            }

            if (S.contains(HEALTH_CHART_KEY)) {
                if (!S[HEALTH_CHART_KEY].is_string()) {
                    netdata_log_error("'%s' key in health silencers file %s, should be a string",
                                      HEALTH_CHART_KEY, path);
                    return false;
                }

                std::string charts = S[HEALTH_CHART_KEY];
                silencer->charts = strdupz(charts.c_str());
                silencer->charts_pattern  = simple_pattern_create(silencer->charts, NULL, SIMPLE_PATTERN_EXACT, true);
            }

            if (S.contains(HEALTH_CONTEXT_KEY)) {
                if (!S[HEALTH_CONTEXT_KEY].is_string()) {
                    netdata_log_error("'%s' key in health silencers file %s, should be a string",
                                      HEALTH_CONTEXT_KEY, path);
                    return false;
                }

                std::string contexts = S[HEALTH_CONTEXT_KEY];
                silencer->contexts = strdupz(contexts.c_str());
                silencer->contexts_pattern = simple_pattern_create(silencer->contexts, NULL, SIMPLE_PATTERN_EXACT, true);
            }

            if (S.contains(HEALTH_HOST_KEY)) {
                if (!S[HEALTH_HOST_KEY].is_string()) {
                    netdata_log_error("'%s' key in health silencers file %s, should be a string",
                                      HEALTH_HOST_KEY, path);
                    return false;
                }
                
                std::string hosts = S[HEALTH_HOST_KEY];
                silencer->hosts = strdupz(hosts.c_str());
                silencer->hosts_pattern = simple_pattern_create(silencer->hosts, NULL, SIMPLE_PATTERN_EXACT, true);
            }

            bool add_to_silencers = silencer->alarms ||
                                    silencer->charts ||
                                    silencer->contexts ||
                                    silencer->hosts;

            if (!add_to_silencers) {
                freez(silencer);
                continue;
            }

            silencer->next = silencers->silencers;
            silencers->silencers = silencer;
        }
    }

    return true;
}

static int silencer_to_json(BUFFER *wb, const char *key, char *val, int has_prev)
{
    if (val)
    {
        buffer_sprintf(wb, "%s\n\t\t\t\"%s\": \"%s\"", (has_prev)?",":"", key, val);
        return 1;
    } else {
        return has_prev;
    }
}

void health_silencers2json(BUFFER *wb)
{
    buffer_sprintf(wb, "{\n\t\"all\": %s,"
                       "\n\t\"type\": \"%s\","
                       "\n\t\"silencers\": [",
                   (silencers->all_alarms) ? "true" : "false",
                   silence_type_to_cstr(silencers->stype));

    int num_silencers = 0;

    for(SILENCER *silencer = silencers->silencers; silencer; silencer = silencer->next)
    {
        if(num_silencers)
            buffer_strcat(wb, ",");

        buffer_strcat(wb, "\n\t\t{");

        int num_fields = 0;

        num_fields = silencer_to_json(wb, HEALTH_ALARM_KEY, silencer->alarms, num_fields);
        num_fields = silencer_to_json(wb, HEALTH_CHART_KEY, silencer->charts, num_fields);
        num_fields = silencer_to_json(wb, HEALTH_CONTEXT_KEY, silencer->contexts, num_fields);
        (void) silencer_to_json(wb, HEALTH_HOST_KEY, silencer->hosts, num_fields);

        buffer_strcat(wb, "\n\t\t}");

        num_silencers++;
    }

    if (num_silencers)
        buffer_strcat(wb, "\n\t");

    buffer_strcat(wb, "]\n}\n");
}

SILENCER *health_silencer_add_param(SILENCER *silencer, char *key, char *value)
{
    if (!silencer)
        silencer = static_cast<SILENCER *>(callocz(1, sizeof(SILENCER)));

    if (strcmp(key, HEALTH_ALARM_KEY) == 0) {
        silencer->alarms = strdupz(value);
        silencer->alarms_pattern = simple_pattern_create(silencer->alarms, NULL, SIMPLE_PATTERN_EXACT, true);
    } else if (strcmp(key, HEALTH_CHART_KEY) == 0) {
        silencer->charts = strdupz(value);
        silencer->charts_pattern = simple_pattern_create(silencer->charts, NULL, SIMPLE_PATTERN_EXACT, true);
    } else if (strcmp(key, HEALTH_CONTEXT_KEY) == 0) {
        silencer->contexts = strdupz(value);
        silencer->contexts_pattern = simple_pattern_create(silencer->contexts, NULL, SIMPLE_PATTERN_EXACT, true);
    } else if (strcmp(key, HEALTH_HOST_KEY) == 0) {
        silencer->hosts = strdupz(value);
        silencer->hosts_pattern = simple_pattern_create(silencer->hosts, NULL, SIMPLE_PATTERN_EXACT, true);
    } else {
        netdata_log_error("Unkonwn silencer key: '%s'", key);
    }

    return silencer;
}

