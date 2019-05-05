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

    if(unlikely(auth_key_id)) freez(auth_key_id);
    if(unlikely(secure_key)) freez(secure_key);
    if(unlikely(stream_name)) freez(stream_name);

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
        if(unlikely(!value)) {
            error("BACKEND: ignoring line %d ('%s') of file '%s', there is no = in it.", line, s, filename);
            continue;
        }
        *value = '\0';
        value++;

        name = trim(name);
        value = trim(value);

        if(unlikely(!name || *name == '#')) {
            error("BACKEND: ignoring line %d of file '%s', name is empty.", line, filename);
            continue;
        }

        if(!value) value = "";

        // strip quotes
        if(*value == '"' || *value == '\'') {
            value++;

            s = value;
            while(*s) s++;
            if(s != value) s--;

            if(*s == '"' || *s == '\'') *s = '\0';
        }

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

    if(unlikely(!auth_key_id || !*auth_key_id || !secure_key || !*secure_key || !stream_name || !*stream_name)) {
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
