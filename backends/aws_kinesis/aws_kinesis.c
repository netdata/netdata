// SPDX-License-Identifier: GPL-3.0-or-later

#define BACKENDS_INTERNALS
#include "aws_kinesis.h"

#define CONFIG_FILE_LINE_MAX ((CONFIG_MAX_NAME + CONFIG_MAX_VALUE + 1024) * 2)

// ----------------------------------------------------------------------------
// kinesis backend

// read the aws_kinesis.conf file
int read_kinesis_conf(const char *path, char **auth_key_id_p, char **secure_key_p, char **stream_name_p)
{
    char *auth_key_id = *auth_key_id_p;
    char *secure_key = *secure_key_p;
    char *stream_name = *stream_name_p;

    if(auth_key_id) freez(auth_key_id);
    if(secure_key) freez(secure_key);
    if(stream_name) freez(stream_name);

    int line = 0;

    char filename[FILENAME_MAX + 1];
    snprintfz(filename, FILENAME_MAX, "%s/aws_kinesis.conf", path);

    char buffer[CONFIG_FILE_LINE_MAX + 1], *s;

    debug(D_BACKEND, "BACKEND: opening config file '%s'", filename);

    FILE *fp = fopen(filename, "r");
    if(!fp) {
        return 1;
    }

    while(fgets(buffer, CONFIG_FILE_LINE_MAX, fp) != NULL) {
        buffer[CONFIG_FILE_LINE_MAX] = '\0';
        line++;

        s = trim(buffer);
        if(!s || *s == '#') {
            debug(D_BACKEND, "BACKEND: ignoring line %d of file '%s', it is empty.", line, filename);
            continue;
        }

        char *name = s;
        char *value = strchr(s, '=');
        if(!value) {
            error("BACKEND: ignoring line %d ('%s') of file '%s', there is no = in it.", line, s, filename);
            continue;
        }
        *value = '\0';
        value++;

        name = trim(name);
        value = trim(value);

        if(!name || *name == '#') {
            error("BACKEND: ignoring line %d of file '%s', name is empty.", line, filename);
            continue;
        }

        if(!value) value = "";

        // strip quotes
        if(*value == '"' || *value == '\'') value++;
        s = value;
        while(*s) s++;
        if(s != value) s--;
        if(*s == '"' || *s == '\'') *s = '\0';

        if(name[0] == 'a' && !strcmp(name, "auth key id")) {
            auth_key_id = strdupz(value);
        }
        else if(name[0] == 's' && name[1] == 'e' && !strcmp(name, "secure key")) {
            secure_key = strdupz(value);
        }
        else if(name[0] == 's' && name[1] == 't' && !strcmp(name, "stream name")) {
            stream_name = strdupz(value);
        }
    }

    fclose(fp);

    if(!auth_key_id || !*auth_key_id || !secure_key || !*secure_key || !stream_name || !*stream_name) {
        error("BACKEND: mandatory Kinesis parameters are not configured:%s%s%s",
              (auth_key_id && *auth_key_id) ? "" : " auth key id,",
              (secure_key && *secure_key) ? "" : " secure key,",
              (stream_name && *stream_name) ? "" : " stream name");
        return 1;
    }

    *auth_key_id_p = auth_key_id;
    *secure_key_p = secure_key;
    *stream_name_p = stream_name;

    return 0;
}

int format_dimension_collected_kinesis_plaintext(
        BUFFER *b                 // the buffer to write data to
        , const char *prefix        // the prefix to use
        , RRDHOST *host             // the host this chart comes from
        , const char *hostname      // the hostname (to override host->hostname)
        , RRDSET *st                // the chart
        , RRDDIM *rd                // the dimension
        , time_t after              // the start timestamp
        , time_t before             // the end timestamp
        , BACKEND_OPTIONS backend_options // BACKEND_SOURCE_* bitmap
) {
    (void)host;
    (void)after;
    (void)before;
    (void)backend_options;

    const char *tags_pre = "", *tags_post = "", *tags = host->tags;
    if(!tags) tags = "";

    if(*tags) {
        if(*tags == '{' || *tags == '[' || *tags == '"') {
            tags_pre = "\"host_tags\":";
            tags_post = ",";
        }
        else {
            tags_pre = "\"host_tags\":\"";
            tags_post = "\",";
        }
    }

    buffer_sprintf(b, "{"
                      "\"prefix\":\"%s\","
                      "\"hostname\":\"%s\","
                      "%s%s%s"

                      "\"chart_id\":\"%s\","
                      "\"chart_name\":\"%s\","
                      "\"chart_family\":\"%s\","
                      "\"chart_context\": \"%s\","
                      "\"chart_type\":\"%s\","
                      "\"units\": \"%s\","

                      "\"id\":\"%s\","
                      "\"name\":\"%s\","
                      "\"value\":" COLLECTED_NUMBER_FORMAT ","

                      "\"timestamp\": %llu}\n",
            prefix,
            hostname,
            tags_pre, tags, tags_post,

            st->id,
            st->name,
            st->family,
            st->context,
            st->type,
            st->units,

            rd->id,
            rd->name,
            rd->last_collected_value,

            (unsigned long long) rd->last_collected_time.tv_sec
    );

    return 1;
}

int format_dimension_stored_kinesis_plaintext(
        BUFFER *b                 // the buffer to write data to
        , const char *prefix        // the prefix to use
        , RRDHOST *host             // the host this chart comes from
        , const char *hostname      // the hostname (to override host->hostname)
        , RRDSET *st                // the chart
        , RRDDIM *rd                // the dimension
        , time_t after              // the start timestamp
        , time_t before             // the end timestamp
        , BACKEND_OPTIONS backend_options // BACKEND_SOURCE_* bitmap
) {
    (void)host;

    time_t first_t = after, last_t = before;
    calculated_number value = backend_calculate_value_from_stored_data(st, rd, after, before, backend_options, &first_t, &last_t);

    if(!isnan(value)) {
        const char *tags_pre = "", *tags_post = "", *tags = host->tags;
        if(!tags) tags = "";

        if(*tags) {
            if(*tags == '{' || *tags == '[' || *tags == '"') {
                tags_pre = "\"host_tags\":";
                tags_post = ",";
            }
            else {
                tags_pre = "\"host_tags\":\"";
                tags_post = "\",";
            }
        }

        buffer_sprintf(b, "{"
                          "\"prefix\":\"%s\","
                          "\"hostname\":\"%s\","
                          "%s%s%s"

                          "\"chart_id\":\"%s\","
                          "\"chart_name\":\"%s\","
                          "\"chart_family\":\"%s\","
                          "\"chart_context\": \"%s\","
                          "\"chart_type\":\"%s\","
                          "\"units\": \"%s\","

                          "\"id\":\"%s\","
                          "\"name\":\"%s\","
                          "\"value\":" CALCULATED_NUMBER_FORMAT ","

                          "\"timestamp\": %llu}\n",
                prefix,
                hostname,
                tags_pre, tags, tags_post,

                st->id,
                st->name,
                st->family,
                st->context,
                st->type,
                st->units,

                rd->id,
                rd->name,
                value,

                (unsigned long long) last_t
        );

        return 1;
    }
    return 0;
}

int process_kinesis_response(BUFFER *b) {
    return discard_response(b, "kinesis");
}
